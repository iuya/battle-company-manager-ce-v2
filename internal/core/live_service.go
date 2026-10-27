package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
)

// LiveService runs the live mode of a match (docs/live-match.md): it computes the starting values, folds the log, validates and
// records actions, undoes entries, and gives the aftermath its defaults. The truth is in the liveRepository (Postgres); nothing
// is cached here, so a restart loses nothing.
type LiveService struct {
	companyDeps
	live    liveRepository
	matches matchRepository
	hub     *LiveHub
	ato     atoRepository // optional (WithATO): the Against the Odds selection overlaid on the live state
	ends    endCanceller  // optional (WithEnds): cancels a pending end of the match when the game goes on
}

// endCanceller is what the live mode needs of the match service: a game action made while the end of the match is pending cancels the
// proposal (docs/live-match.md, "Ending a match while live"). It reports whether one was pending.
type endCanceller interface {
	CancelPendingEnd(ctx context.Context, user User, id, matchID int64, why string) (bool, error)
}

// WithEnds gives the service the match service that cancels a pending end.
func (s *LiveService) WithEnds(c endCanceller) *LiveService { s.ends = c; return s }

// NewLiveService builds the service.
func NewLiveService(live liveRepository, campaigns campaignRepository, companies companyRepository, matches matchRepository, rules ruleSource) *LiveService {
	return &LiveService{companyDeps: companyDeps{campaigns: campaigns, companies: companies, rules: rules, dice: RandomDice{}}, live: live, matches: matches}
}

// WithATO lets the live state show what the match's Against the Odds selection adds: the items a Tool Kit loaned and the temporary
// Wanderer. The selection is read every time, never copied into the live data.
func (s *LiveService) WithATO(r atoRepository) *LiveService { s.ato = r; return s }

// WithHub gives the service the hub Subscribe uses.
func (s *LiveService) WithHub(h *LiveHub) *LiveService { s.hub = h; return s }

// liveContext is what a call needs to know about the match and the viewer.
type liveContext struct {
	camp    CampaignDetail
	match   Match
	details map[int64]CompanyDetail // by company id, for the names
	owners  map[int64]int64         // company id -> owner user id
	rules   *RuleSet                // the campaign's rules (any company's: they share the file)
	cfg     LiveFoldConfig          // the Break Point settings of the companies (percent, models per unit), from the rosters and the rules
	extra   []LiveUnitInit          // units that exist only in the overlay (the temporary Wanderer)
	temp    *TemporaryWanderer      // the temporary Wanderer of the match, nil without
	tempCo  int64                   // the company that took it
}

func (s *LiveService) context(ctx context.Context, user User, id, matchID int64) (liveContext, error) {
	camp, err := s.campaigns.Get(ctx, user.ID, id)
	if err != nil {
		return liveContext{}, err
	}
	m, err := s.matches.Get(ctx, id, matchID)
	if err != nil {
		return liveContext{}, err
	}
	return s.build(ctx, id, camp, m)
}

// build is context for a match already read; camp is the zero value when no viewer is involved (MatchDefaults).
func (s *LiveService) build(ctx context.Context, id int64, camp CampaignDetail, m Match) (liveContext, error) {
	lc := liveContext{camp: camp, match: m, details: map[int64]CompanyDetail{}, owners: map[int64]int64{},
		cfg: LiveFoldConfig{Break: map[int64]BreakConfig{}, BreakModels: map[int64]int{}, NoStandFast: map[int64]bool{}, Owned: map[int64][]string{}}}
	for _, side := range m.Sides {
		for _, co := range side.Companies {
			lc.owners[co.ID] = co.OwnerID
			d, rs, err := s.detail(ctx, id, co.ID)
			if err != nil {
				return liveContext{}, err
			}
			lc.details[co.ID] = d
			lc.breakConfig(rs, d)
			lc.rules = rs
		}
	}
	if s.ato != nil && lc.rules != nil {
		rec, err := s.ato.Get(ctx, id, m.ID)
		if err != nil {
			return liveContext{}, err
		}
		lc.overlay(lc.rules.BC.ATO(), rec)
	}
	return lc, nil
}

// overlay adds what the Against the Odds record puts into the match: the loaned items on their models and the temporary Wanderer as a
// unit of the company that took it. The unit is not stored anywhere: it is made here, from the record and the rules file, every time.
func (lc *liveContext) overlay(rules ATORules, rec ATORecord) {
	if rec.CompanyID == 0 || rec.Selection.Empty() {
		return
	}
	lc.cfg.AfterGame = map[string]bool{}
	for _, k := range rules.Kits {
		for _, it := range k.Items {
			if it.AfterGame {
				lc.cfg.AfterGame[strings.ToLower(it.Item)] = true
			}
		}
	}
	lc.cfg.Loaned = append(lc.cfg.Loaned, rules.LoanedFor(rec, rec.CompanyID)...)
	if w, ok := rules.Wanderer(rec.Selection.Wanderer); ok && rec.Selection.Has(atoWanderer) {
		id := TemporaryUnitID(rec.CompanyID)
		rating := 0
		if b, ok := rules.Bonus(atoWanderer); ok {
			rating = b.Rating
		}
		lc.extra = append(lc.extra, LiveUnitInit{UnitID: id, CompanyID: rec.CompanyID, Wounds: w.Wounds, Might: w.Might, Will: w.Will, Fate: w.Fate,
			CountsAsKill: true, Temporary: true})
		lc.cfg.NoStandFast[id] = true
		lc.temp = &TemporaryWanderer{UnitID: id, Name: w.Name, Summary: w.Summary, Rating: rating, Courage: w.Courage}
		lc.tempCo = rec.CompanyID
	}
}

// inits are the starting values the fold takes: the stored ones and the overlay's.
func (lc liveContext) inits(snap LiveSnapshot) []LiveUnitInit {
	if len(lc.extra) == 0 {
		return snap.Units
	}
	return append(slices.Clone(snap.Units), lc.extra...)
}

// breakConfig fills the Break Point settings of a company: its percent (its own company rule or the file's default), the models it
// has besides its units (the creatures), how many models each unit counts as, who cannot call Stand Fast, and the exceptions of the
// match's scenario. Nothing is stored: it is worked out from the roster and the file every time, like the names.
func (lc *liveContext) breakConfig(rs *RuleSet, d CompanyDetail) {
	rules := rs.BC.BreakRules()
	cfg := BreakConfig{Percent: breakPointPercent(rs, d.Faction), Extra: len(d.Creatures)}
	scenario := ""
	if lc.match.Scenario != nil && lc.match.Scenario.Kind == ScenarioRulebook {
		scenario = lc.match.Scenario.Name
	}
	for _, e := range rules.ExceptionsFor(scenario) {
		cfg.Exceptions = append(cfg.Exceptions, e.Text())
	}
	lc.cfg.Break[d.ID] = cfg
	for _, u := range d.Roster {
		for _, e := range u.Equipment {
			lc.cfg.Owned[u.ID] = append(lc.cfg.Owned[u.ID], e.Name)
		}
		n := u.ModelCount()
		if len(u.Parts) > 0 { // a unit made of several models (Warg Marauder): one until the owner decides how it counts (bead 36y.18)
			n = 1
		}
		lc.cfg.BreakModels[u.ID] = n
		noStandFast := !u.IsHero() || u.Wanderer || slices.ContainsFunc(u.LostAbilities, func(l InjuryLoss) bool {
			return strings.Contains(strings.ToLower(strings.ReplaceAll(l.Ability, " ", "")), "standfast")
		})
		lc.cfg.NoStandFast[u.ID] = noStandFast
	}
}

func (lc liveContext) company(companyID int64) (CampaignCompany, bool) {
	for _, side := range lc.match.Sides {
		for _, co := range side.Companies {
			if co.ID == companyID {
				return co, true
			}
		}
	}
	return CampaignCompany{}, false
}

// names gives a state the names of the units and the companies of the match.
func (lc liveContext) names(st *LiveState) {
	units := map[int64]string{}
	for _, d := range lc.details {
		for _, u := range d.Roster {
			units[u.ID] = u.Title()
		}
	}
	if lc.temp != nil {
		units[lc.temp.UnitID] = lc.temp.Name
	}
	st.UnitName = func(id int64) string { return units[id] }
	st.CompanyName = func(id int64) string { return lc.details[id].Name }
}

// access is what the user may do with the company now: nothing once the match or the campaign is over.
func (lc liveContext) access(user User, co CampaignCompany) LiveAccess {
	if lc.camp.CanPlay() != nil {
		return LiveNone
	}
	return LiveAccessFor(user, lc.match, co, lc.camp.OrganiserID)
}

// initsOf is the starting values of the units of a company: the profile's Wounds, Might, Will and Fate (promotions and path boosts
// count), a mount when the unit has one, Hero or Monster for the kills, and whether it takes part.
func initsOf(d CompanyDetail) []LiveUnitInit {
	var out []LiveUnitInit
	for _, u := range d.Roster {
		in := LiveUnitInit{UnitID: u.ID, CompanyID: d.ID, Absent: Absent(u), CountsAsKill: u.IsHero() || u.Kind == KindMonster}
		for _, a := range u.Stats {
			switch a.Name {
			case "Wounds":
				in.Wounds = a.Value
			case "Might":
				in.Might = a.Value
			case "Will":
				in.Will = a.Value
			case "Fate":
				in.Fate = a.Value
			}
		}
		for _, e := range u.Equipment {
			if slices.Contains(e.Classes(), EquipMount) {
				in.HasMount = true
				for _, a := range e.MountStats {
					if a.Name == "Wounds" && a.Value > 1 {
						in.MountWounds = a.Value
					}
				}
			}
		}
		if u.Kind == KindCavalry || u.Kind == KindMonstrousCavalry {
			in.HasMount = true
		}
		out = append(out, in)
	}
	return out
}

// Init stores the starting values of the units of the match; it does nothing when they are stored.
func (s *LiveService) Init(ctx context.Context, campaignID, matchID int64) error {
	m, err := s.matches.Get(ctx, campaignID, matchID)
	if err != nil {
		return err
	}
	return s.initMatch(ctx, campaignID, m)
}

func (s *LiveService) initMatch(ctx context.Context, campaignID int64, m Match) error {
	var inits []LiveUnitInit
	for _, side := range m.Sides {
		for _, co := range side.Companies {
			d, _, err := s.detail(ctx, campaignID, co.ID)
			if err != nil {
				return err
			}
			inits = append(inits, initsOf(d)...)
		}
	}
	return s.live.Init(ctx, campaignID, m.ID, inits)
}

// snapshot reads the live data of a match, storing the starting values first when a match opened before the feature has none.
func (s *LiveService) snapshot(ctx context.Context, campaignID int64, m Match) (LiveSnapshot, error) {
	snap, err := s.live.Read(ctx, campaignID, m.ID)
	if err != nil {
		return snap, err
	}
	if snap.Initialised || m.Ended {
		return snap, nil
	}
	if err := s.initMatch(ctx, campaignID, m); err != nil {
		return snap, err
	}
	return s.live.Read(ctx, campaignID, m.ID)
}

func (lc liveContext) state(snap LiveSnapshot) LiveState {
	st := FoldLiveWith(lc.inits(snap), snap.Entries, lc.cfg)
	lc.names(&st)
	return st
}

func (lc liveContext) view(snap LiveSnapshot, user User, co CampaignCompany) LiveCompany {
	st := lc.state(snap)
	out := LiveCompany{MatchID: snap.MatchID, CompanyID: co.ID, MatchNumber: lc.match.Number, Seq: snap.Seq, Units: st.Company(co.ID), Access: lc.access(user, co), Break: st.Break(co.ID), Turn: st.Turn()}
	if lc.temp != nil && lc.tempCo == co.ID {
		w := *lc.temp
		out.Wanderer = &w
	}
	for _, in := range snap.Units {
		if in.CompanyID == co.ID && in.Absent {
			out.Absent = append(out.Absent, in.UnitID)
		}
	}
	return out
}

// Live is the live state of a company in an open match. ErrNotFound when the company is in no open match (the page is the usual one).
func (s *LiveService) Live(ctx context.Context, user User, id, companyID int64) (LiveCompany, error) {
	st, err := s.companies.Read(ctx, id, companyID)
	if err != nil {
		return LiveCompany{}, err
	}
	if st.OpenMatch == nil {
		return LiveCompany{}, ErrNotFound
	}
	lc, err := s.context(ctx, user, id, st.OpenMatch.MatchID)
	if err != nil {
		return LiveCompany{}, err
	}
	co, ok := lc.company(companyID)
	if !ok || lc.match.Ended {
		return LiveCompany{}, ErrNotFound
	}
	snap, err := s.snapshot(ctx, id, lc.match)
	if err != nil {
		return LiveCompany{}, err
	}
	return lc.view(snap, user, co), nil
}

// Apply records an action of a unit of the company. ErrRefused when the user may not change the company, the match or the campaign
// is over, or the state does not allow it; ErrInvalidLive for bad input.
func (s *LiveService) Apply(ctx context.Context, user User, id, matchID, companyID int64, a LiveAction) (LiveCompany, LiveEntry, error) {
	lc, err := s.context(ctx, user, id, matchID)
	if err != nil {
		return LiveCompany{}, LiveEntry{}, err
	}
	co, ok := lc.company(companyID)
	if !ok {
		return LiveCompany{}, LiveEntry{}, ErrNotFound
	}
	if a.Kind == LiveTurn && lc.match.EndProposal != nil {
		return LiveCompany{}, LiveEntry{}, Refuse("Cancel the ending first: the match is about to end.")
	}
	if a.Kind == LiveMVPVote { // the voter is an opponent, not an editor of the company voted on
		a.TargetCompanyID = companyID
		err = lc.mayVote(user, co)
	} else {
		err = lc.writable(user, co)
	}
	if err != nil {
		return LiveCompany{}, LiveEntry{}, err
	}
	var secretText string
	if a.Kind.Secret() {
		if secretText, err = s.checkSecretRules(ctx, id, lc, user, companyID, a); err != nil {
			return LiveCompany{}, LiveEntry{}, err
		}
	}
	if _, err := s.snapshot(ctx, id, lc.match); err != nil {
		return LiveCompany{}, LiveEntry{}, err
	}
	entry, err := s.live.Update(ctx, id, matchID, user, func(snap LiveSnapshot) (LiveChange, error) {
		if snap.Ended {
			return LiveChange{}, Refuse("The match has ended.")
		}
		st := lc.state(snap)
		res, touches, text, err := st.CheckAs(a, companyID, LiveVoter{ID: user.ID, Name: user.DisplayName})
		if err != nil {
			return LiveChange{}, err
		}
		if secretText != "" {
			text = secretText
		}
		entryCompany := companyID
		if a.Kind == LiveTurn { // a turn belongs to the match, not to the company that pressed the button
			entryCompany = 0
		}
		return LiveChange{Append: &LiveEntry{MatchID: matchID, CompanyID: entryCompany, UnitID: a.UnitID, Touches: touches, Action: res, Text: text}}, nil
	})
	if err != nil {
		return LiveCompany{}, LiveEntry{}, err
	}
	if err := s.cancelPendingEnd(ctx, lc, user, id, matchID, a.Kind, entry.Text); err != nil {
		return LiveCompany{}, entry, err
	}
	snap, err := s.live.Read(ctx, id, matchID)
	if err != nil {
		return LiveCompany{}, entry, err
	}
	return lc.view(snap, user, co), entry, nil
}

// cancelPendingEnd drops a pending end of the match after a game action (not an end check): the game is never played under a confirmed
// end. The match service writes the "Ending cancelled: ..." line of the log through RecordEnd.
func (s *LiveService) cancelPendingEnd(ctx context.Context, lc liveContext, user User, id, matchID int64, kind LiveKind, what string) error {
	if s.ends == nil || lc.match.EndProposal == nil || kind.EndCheckKind() {
		return nil
	}
	_, err := s.ends.CancelPendingEnd(ctx, user, id, matchID, "Ending cancelled: "+what)
	return err
}

// RecordEnd writes the log line of a change of a pending end (EndChange from the match service): an informational entry in the
// colour of the company concerned (the author's company when none is named). The final confirmation has no line (the match history
// has it), and a match without live data has no log.
func (s *LiveService) RecordEnd(ctx context.Context, ch EndChange) {
	kind := map[string]LiveKind{"endProposed": LiveEndProposed, "endConfirmed": LiveEndConfirmed, "endWithdrawn": LiveEndWithdrawn,
		"endCancelled": LiveEndCancelled, "endOverride": LiveEndOverride}[ch.Kind]
	if kind == "" {
		return
	}
	lc, err := s.context(ctx, ch.By, ch.CampaignID, ch.MatchID)
	if err != nil {
		slog.Warn("end of match: no log line", "match", ch.MatchID, "err", err)
		return
	}
	company := int64(0)
	if len(ch.CompanyIDs) > 0 {
		company = ch.CompanyIDs[0]
	} else {
		for c, owner := range lc.owners {
			if owner == ch.By.ID && (company == 0 || c < company) {
				company = c
			}
		}
	}
	snap, err := s.live.Read(ctx, ch.CampaignID, ch.MatchID)
	if err != nil || !snap.Initialised {
		return // nothing was ever played live: there is no log to add to
	}
	_, err = s.live.Update(ctx, ch.CampaignID, ch.MatchID, ch.By, func(LiveSnapshot) (LiveChange, error) {
		return LiveChange{Append: &LiveEntry{MatchID: ch.MatchID, CompanyID: company, Touches: ch.CompanyIDs, Action: LiveAction{Kind: kind}, Text: ch.Text}}, nil
	})
	if err != nil {
		slog.Warn("end of match: no log line", "match", ch.MatchID, "err", err)
	}
}

// sideOf is the (0-based) side a company plays on, -1 when it is not in the match.
func (lc liveContext) sideOf(companyID int64) int {
	for i, side := range lc.match.Sides {
		for _, co := range side.Companies {
			if co.ID == companyID {
				return i
			}
		}
	}
	return -1
}

// opposing says whether the user owns a company on a team other than the company's.
func (lc liveContext) opposing(user User, companyID int64) bool {
	side := lc.sideOf(companyID)
	for c, owner := range lc.owners {
		if owner == user.ID && lc.sideOf(c) != side {
			return true
		}
	}
	return false
}

// mayVote says why the user cannot vote for the MVP of the company (nil when they can): a player of the match, from another team.
func (lc liveContext) mayVote(user User, co CampaignCompany) error {
	if err := lc.camp.CanPlay(); err != nil {
		return err
	}
	if lc.match.Ended {
		return Refuse("The match has ended.")
	}
	owns := false
	for _, owner := range lc.owners {
		owns = owns || owner == user.ID
	}
	switch {
	case !owns:
		return Refuse("You have no company in this match.")
	case !lc.opposing(user, co.ID):
		return Refuse("You can only vote for companies of the other teams.")
	}
	return nil
}

// writable says why the user cannot change the company's live state (nil when they can).
func (lc liveContext) writable(user User, co CampaignCompany) error {
	if err := lc.camp.CanPlay(); err != nil {
		return err
	}
	if lc.match.Ended {
		return Refuse("The match has ended.")
	}
	if LiveAccessFor(user, lc.match, co, lc.camp.OrganiserID) != LiveEdit {
		return Refuse("Only the owner of %s can change it.", co.Name)
	}
	return nil
}

// Undo removes an entry of the log and returns the state of the company the viewer is on. ErrLiveDepends when a later entry
// needs it; ErrNotFound when it is gone already.
func (s *LiveService) Undo(ctx context.Context, user User, id, matchID, entryID, companyID int64) (LiveCompany, error) {
	lc, err := s.context(ctx, user, id, matchID)
	if err != nil {
		return LiveCompany{}, err
	}
	co, ok := lc.company(companyID)
	if !ok {
		return LiveCompany{}, ErrNotFound
	}
	if err := lc.camp.CanPlay(); err != nil {
		return LiveCompany{}, err
	}
	if lc.match.Ended {
		return LiveCompany{}, Refuse("The match has ended.")
	}
	if _, err := s.snapshot(ctx, id, lc.match); err != nil {
		return LiveCompany{}, err
	}
	var undone LiveEntry
	_, err = s.live.Update(ctx, id, matchID, user, func(snap LiveSnapshot) (LiveChange, error) {
		if snap.Ended {
			return LiveChange{}, Refuse("The match has ended.")
		}
		i := slices.IndexFunc(snap.Entries, func(e LiveEntry) bool { return e.ID == entryID })
		if i < 0 {
			return LiveChange{}, ErrNotFound
		}
		undone = snap.Entries[i]
		if !CanUndoEntry(user, snap.Entries[i], lc.owners, lc.camp.OrganiserID) {
			return LiveChange{}, Refuse("Only the players involved can undo this.")
		}
		rest := slices.Delete(slices.Clone(snap.Entries), i, i+1)
		if err := ValidateLogWith(lc.inits(snap), rest, lc.cfg); err != nil {
			return LiveChange{}, err
		}
		return LiveChange{Undo: entryID}, nil
	})
	if err != nil {
		return LiveCompany{}, err
	}
	if err := s.cancelPendingEnd(ctx, lc, user, id, matchID, undone.Action.Kind, "undo of "+undone.Text); err != nil {
		return LiveCompany{}, err
	}
	snap, err := s.live.Read(ctx, id, matchID)
	if err != nil {
		return LiveCompany{}, err
	}
	return lc.view(snap, user, co), nil
}

// Log lists the not-undone entries, newest first, readable by every member of the campaign, also after the match (the log is kept
// for good). CanUndo is set for the user.
func (s *LiveService) Log(ctx context.Context, user User, id, matchID, before int64, limit int) ([]LiveEntry, error) {
	lc, err := s.context(ctx, user, id, matchID)
	if err != nil {
		return nil, err
	}
	if !lc.match.Ended {
		if _, err := s.snapshot(ctx, id, lc.match); err != nil {
			return nil, err
		}
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	list, err := s.live.Log(ctx, id, matchID, before, limit)
	if err != nil {
		return nil, err
	}
	open := !lc.match.Ended && lc.camp.CanPlay() == nil
	if !lc.match.Ended { // the secret objectives of the other players do not exist for this viewer until the match ends
		list = slices.DeleteFunc(list, func(e LiveEntry) bool { return e.Action.Kind.Secret() && e.ByUserID != user.ID })
	}
	for i := range list {
		list[i].CanUndo = open && CanUndoEntry(user, list[i], lc.owners, lc.camp.OrganiserID)
	}
	return list, nil
}

// AftermathDefaults is the aftermath sheet the live state gives a company: Field units survived, Retreated units fled, casualties
// are casualties; the gains are the flags and counts earned, and everybody took part. The second result is false when the match has
// no live data (recorded after the fact, or opened before the feature and never looked at).
func (s *LiveService) AftermathDefaults(ctx context.Context, id, matchID, companyID int64) ([]ModelSheet, bool, error) {
	st, ok, err := s.readState(ctx, id, matchID)
	if err != nil || !ok {
		return nil, false, err
	}
	var out []ModelSheet
	_, winner, _ := st.MVPTally(companyID) // the most votes; a tie leaves the choice to the owner
	for _, u := range st.Company(companyID) {
		if u.Temporary { // the temporary Wanderer of Against the Odds is not on the sheet: no roll, no experience, no progression
			continue
		}
		sheet := ModelSheet{UnitID: u.UnitID, Outcome: OutcomeSurvived, Gains: map[string]int{participatedKey: 1}}
		switch u.Zone {
		case ZoneRetreated:
			sheet.Outcome = OutcomeFled
		case ZoneCasualty:
			sheet.Outcome = OutcomeCasualty
		}
		for k, v := range u.Gains {
			if v > 0 {
				sheet.Gains[k] = v
			}
		}
		if st.HasObjective(u.UnitID) {
			sheet.Gains[GainObjective] = 1
		}
		if u.UnitID == winner {
			sheet.Gains[GainMVP] = 1
		}
		out = append(out, sheet)
	}
	return out, true, nil
}

// Subscribe feeds the SSE handler: a stream of coalesced changes of the match. ErrNotFound for a stranger or an unknown match,
// ErrRefused when the match is not open or the caps are reached.
func (s *LiveService) Subscribe(ctx context.Context, user User, id, matchID int64) (LiveStream, error) {
	if s.hub == nil {
		return nil, fmt.Errorf("live hub not wired")
	}
	if _, err := s.campaigns.Get(ctx, user.ID, id); err != nil {
		return nil, err
	}
	m, err := s.matches.Get(ctx, id, matchID)
	if err != nil {
		return nil, err
	}
	if m.Ended {
		return nil, Refuse("The match has ended.")
	}
	var ids []int64
	for _, side := range m.Sides {
		for _, co := range side.Companies {
			ids = append(ids, co.ID)
		}
	}
	snap, err := s.live.Read(ctx, id, matchID)
	if err != nil {
		return nil, err
	}
	stream, err := s.hub.subscribe(user.ID, matchID, ids, snap.Seq)
	if err != nil {
		return nil, err
	}
	context.AfterFunc(ctx, stream.Close) // a cancelled request frees its slot
	return stream, nil
}

// EndChecks is the card of the end of the game: who got the objective, the MVP ballots and the tally (docs/live-match.md "End checks").
type EndChecks struct {
	Objective []CompanyObjectives // the companies the viewer may edit (the owner's, or all for an administrator or the organiser)
	MVP       []MVPBallot         // one per company of another team than a company the viewer owns
	Tally     []MVPTally          // every company of the match
	Pending   []string            // what the End match reminder lists
}

// CompanyObjectives lists the units of a company that may get the objective (in the Field or Retreated).
type CompanyObjectives struct {
	CompanyID int64
	Name      string
	Units     []EndCheckUnit
}

// EndCheckUnit is a unit on an end-check list.
type EndCheckUnit struct {
	UnitID  int64
	Title   string
	Type    string
	Zone    Zone
	Wounds  LiveStat
	Has     bool // got the objective
	Current bool // the viewer's MVP vote
}

// MVPBallot is the vote of the viewer for one opposing company.
type MVPBallot struct {
	CompanyID int64
	Name      string
	Units     []EndCheckUnit // the units that took part (Field, Retreated, casualties)
	Current   int64          // the unit the viewer voted for, 0 for none
}

// MVPTally is the count of the votes for one company.
type MVPTally struct {
	CompanyID int64
	Name      string
	Lines     []TallyLine
	Winner    int64   // 0 on a tie or without votes
	Tied      []int64 // the units sharing the most votes when it is a tie
}

// TallyLine is the votes of one unit.
type TallyLine struct {
	UnitID int64
	Title  string
	Votes  int
}

// EndChecks lists the end-of-game checks for the viewer. ErrNotFound when the match is not open.
func (s *LiveService) EndChecks(ctx context.Context, user User, id, matchID int64) (EndChecks, error) {
	lc, err := s.context(ctx, user, id, matchID)
	if err != nil {
		return EndChecks{}, err
	}
	if lc.match.Ended {
		return EndChecks{}, ErrNotFound
	}
	snap, err := s.snapshot(ctx, id, lc.match)
	if err != nil {
		return EndChecks{}, err
	}
	st := lc.state(snap)
	titles := map[int64]CompanyUnit{}
	for _, d := range lc.details {
		for _, u := range d.Roster {
			titles[u.ID] = u
		}
	}
	if lc.temp != nil {
		titles[lc.temp.UnitID] = CompanyUnit{ID: lc.temp.UnitID, Name: lc.temp.Name, Type: "Temporary Wanderer", Role: RoleHero}
	}
	line := func(u LiveUnit) EndCheckUnit {
		cu := titles[u.UnitID]
		return EndCheckUnit{UnitID: u.UnitID, Title: cu.Title(), Type: cu.Type, Zone: u.Zone, Wounds: u.Wounds, Has: st.HasObjective(u.UnitID)}
	}
	var out EndChecks
	for _, side := range lc.match.Sides {
		for _, co := range side.Companies {
			if LiveAccessFor(user, lc.match, co, lc.camp.OrganiserID) == LiveEdit && lc.camp.CanPlay() == nil {
				c := CompanyObjectives{CompanyID: co.ID, Name: lc.details[co.ID].Name}
				any := false
				for _, u := range st.Company(co.ID) {
					if u.Zone == ZoneField || u.Zone == ZoneRetreated {
						l := line(u)
						c.Units = append(c.Units, l)
						any = any || l.Has
					}
				}
				out.Objective = append(out.Objective, c)
				if !any {
					out.Pending = append(out.Pending, fmt.Sprintf("No unit of %s has the objective yet", c.Name))
				}
			}
			if lc.opposing(user, co.ID) {
				b := MVPBallot{CompanyID: co.ID, Name: lc.details[co.ID].Name, Current: st.Vote(user.ID, co.ID)}
				for _, u := range st.Company(co.ID) {
					l := line(u)
					l.Current = u.UnitID == b.Current
					b.Units = append(b.Units, l)
				}
				out.MVP = append(out.MVP, b)
				if b.Current == 0 {
					out.Pending = append(out.Pending, fmt.Sprintf("you have not voted for %s", b.Name))
				}
			}
			counts, winner, tied := st.MVPTally(co.ID)
			t := MVPTally{CompanyID: co.ID, Name: lc.details[co.ID].Name, Winner: winner, Tied: tied}
			for _, c := range counts {
				t.Lines = append(t.Lines, TallyLine{UnitID: c.UnitID, Title: titles[c.UnitID].Title(), Votes: c.Votes})
			}
			out.Tally = append(out.Tally, t)
		}
	}
	return out, nil
}

// secretObjective finds a secret scoring objective of the match's scenario.
func (s *LiveService) secretObjective(ctx context.Context, campaignID int64, m Match, key string) (ScoringObjective, error) {
	if m.Scenario == nil || m.Scenario.Kind != ScenarioRulebook {
		return ScoringObjective{}, Refuse("Choose the scenario first: secret objectives belong to it.")
	}
	rs, _, err := s.ruleset(ctx, campaignID)
	if err != nil {
		return ScoringObjective{}, err
	}
	sc, ok := rs.BC.Scenario(m.Scenario.Name)
	if !ok {
		return ScoringObjective{}, Refuse("The rules do not know this scenario.")
	}
	for _, o := range sc.ScoringObjectives {
		if o.Key == key && o.Secret {
			return o, nil
		}
	}
	return ScoringObjective{}, Refuse("That is not a secret objective of %s.", m.Scenario.Name)
}

// checkSecretRules applies the rules of a secret objective entry that need the scenario and the rosters, and words the entry (only its
// author sees it before the end, and everybody after).
func (s *LiveService) checkSecretRules(ctx context.Context, campaignID int64, lc liveContext, user User, companyID int64, a LiveAction) (string, error) {
	o, err := s.secretObjective(ctx, campaignID, lc.match, a.ObjectiveKey)
	if err != nil {
		return "", err
	}
	me := user.DisplayName
	if me == "" {
		me = "A player"
	}
	company := lc.details[companyID]
	switch a.Kind {
	case LiveSecretDone:
		if a.Tier < 0 || a.Tier > len(o.Options) {
			return "", invalid("Choose one of the tiers of this objective.")
		}
		if a.Tier == 0 {
			return fmt.Sprintf("%s withdrew the report of %s", me, o.Label), nil
		}
		tier := o.Options[a.Tier-1]
		return fmt.Sprintf("%s scored %d for %s", me, tier.Points, o.Label), nil
	}
	n := o.Nomination
	if n == nil {
		return "", Refuse("%s needs nothing noted down.", o.Label)
	}
	note := strings.TrimSpace(a.Note)
	if a.UnitID == 0 && note == "" {
		return fmt.Sprintf("%s cleared the note for %s", me, o.Label), nil
	}
	if n.From == "terrain" {
		if a.UnitID != 0 || note == "" {
			return "", invalid("Describe the terrain piece.")
		}
		return fmt.Sprintf("%s noted the terrain piece %q for %s", me, note, o.Label), nil
	}
	if note != "" || a.UnitID == 0 {
		return "", invalid("Choose a Hero.")
	}
	var unit CompanyUnit
	var owner int64
	for cid, d := range lc.details {
		for _, u := range d.Roster {
			if u.ID == a.UnitID {
				unit, owner = u, cid
			}
		}
	}
	if owner == 0 || Absent(unit) {
		return "", Refuse("That unit takes no part in this match.")
	}
	if !unit.IsHero() {
		return "", Refuse("%s is not a Hero.", unit.Title())
	}
	switch n.From {
	case "ownHeroes":
		if owner != companyID {
			return "", Refuse("Note down one of your own Heroes.")
		}
	case "enemyHeroes":
		if lc.sideOf(owner) == lc.sideOf(companyID) {
			return "", Refuse("Note down a Hero of an opposing company.")
		}
	}
	if n.AllowLeader == "onlyHero" && unit.Role == RoleLeader {
		heroes := 0
		for _, u := range lc.details[owner].Roster {
			if u.IsHero() && !Absent(u) {
				heroes++
			}
		}
		if heroes > 1 {
			return "", Refuse("The Leader may only be noted down when the company has no other Hero.")
		}
	}
	_ = company
	return fmt.Sprintf("%s noted %s (%s) for %s", me, unit.Title(), lc.details[owner].Name, o.Label), nil
}

// SecretLine is one secret objective of a company, as the viewer may see it.
type SecretLine struct {
	CompanyID int64
	Company   string
	Key       string
	Label     string // a short name: "Own hero survives"
	Text      string // the objective
	Nominee   string // the Hero or terrain piece noted down
	Tier      int    // 0: not reported
	Points    int
}

// SecretView is what the viewer may see of the secret objectives of a match: their own while it is open (and who else has some, by
// company name, never what), everybody's once it has ended.
type SecretView struct {
	Revealed bool
	Lines    []SecretLine
	Others   []string // companies that have secret objectives the viewer cannot see yet
}

// SecretObjectives lists the secret objectives the viewer may see. Privacy is applied here, on read.
func (s *LiveService) SecretObjectives(ctx context.Context, user User, id, matchID int64) (SecretView, error) {
	lc, err := s.context(ctx, user, id, matchID)
	if err != nil {
		return SecretView{}, err
	}
	snap, err := s.live.Read(ctx, id, matchID)
	if err != nil {
		return SecretView{}, err
	}
	view := SecretView{Revealed: lc.match.Ended}
	authors := map[int64]map[int64]bool{}
	for _, e := range snap.Entries {
		if e.Action.Kind.Secret() {
			if authors[e.CompanyID] == nil {
				authors[e.CompanyID] = map[int64]bool{}
			}
			authors[e.CompanyID][e.ByUserID] = true
		}
	}
	st := lc.state(snap)
	var sc ScenarioInfo
	if lc.match.Scenario != nil && lc.match.Scenario.Kind == ScenarioRulebook {
		if rs, _, err := s.ruleset(ctx, id); err == nil {
			sc, _ = rs.BC.Scenario(lc.match.Scenario.Name)
		}
	}
	titles := map[int64]string{}
	for _, d := range lc.details {
		for _, u := range Distinguish(d.Roster) { // the nominee of "one of your Heroes" is told apart from a twin
			titles[u.ID] = u.Title()
		}
	}
	for _, side := range lc.match.Sides {
		for _, co := range side.Companies {
			if len(authors[co.ID]) == 0 {
				continue
			}
			if !lc.match.Ended && !authors[co.ID][user.ID] {
				view.Others = append(view.Others, lc.details[co.ID].Name)
				continue
			}
			for _, o := range sc.ScoringObjectives {
				n, ok := st.SecretOf(co.ID, o.Key)
				if !o.Secret || !ok {
					continue
				}
				l := SecretLine{CompanyID: co.ID, Company: lc.details[co.ID].Name, Key: o.Key, Label: o.Label, Text: o.Text, Tier: n.Tier}
				switch {
				case n.Nominee != 0:
					l.Nominee = titles[n.Nominee]
				default:
					l.Nominee = n.Terrain
				}
				if n.Tier >= 1 && n.Tier <= len(o.Options) {
					l.Points = o.Options[n.Tier-1].Points
				}
				view.Lines = append(view.Lines, l)
			}
		}
	}
	return view, nil
}

// SecretScore is the Victory Points the secret objectives a company reported add up to, for the result form to prefill once the match
// is over (ok is false when it has not ended or the company reported none).
func (s *LiveService) SecretScore(ctx context.Context, user User, id, matchID, companyID int64) (points int, lines []SecretLine, ok bool, err error) {
	view, err := s.SecretObjectives(ctx, user, id, matchID)
	if err != nil || !view.Revealed {
		return 0, nil, false, err
	}
	for _, l := range view.Lines {
		if l.CompanyID == companyID && l.Tier > 0 {
			points += l.Points
			lines = append(lines, l)
		}
	}
	return points, lines, len(lines) > 0, nil
}

// MVPHint is the tally of the MVP votes for a company, nil when nobody voted or the match has no live data. The aftermath sheet shows it
// next to the MVP flag (the winner is prefilled by AftermathDefaults).
func (s *LiveService) MVPHint(ctx context.Context, id, matchID, companyID int64) (*MVPHint, error) {
	st, ok, err := s.readState(ctx, id, matchID)
	if err != nil || !ok {
		return nil, err
	}
	counts, winner, tied := st.MVPTally(companyID)
	if len(counts) == 0 {
		return nil, nil
	}
	titles := map[int64]string{}
	if d, _, err := s.detail(ctx, id, companyID); err == nil {
		for _, u := range d.Roster {
			titles[u.ID] = u.Title()
		}
	}
	h := &MVPHint{Winner: winner, Tied: tied}
	for _, c := range counts {
		h.Lines = append(h.Lines, TallyLine{UnitID: c.UnitID, Title: titles[c.UnitID], Votes: c.Votes})
	}
	return h, nil
}

// RollCourage rolls the Courage test of a unit of a Broken company: two dice (or the two the player typed or threw, as the campaign's
// settings allow) against the unit's Courage; the file says how many dice and what passes. It writes nothing: the player records the
// result with Apply (broken for a pass, retreat with Why "courage" for a fail). ErrRefused when the user may not change the company, the
// unit is not in the battle or the campaign does not allow that kind of roll; ErrNotFound for a unit in no company of the match.
func (s *LiveService) RollCourage(ctx context.Context, user User, id, matchID, unitID int64, entered []int, thrown bool) (CourageRoll, error) {
	lc, err := s.context(ctx, user, id, matchID)
	if err != nil {
		return CourageRoll{}, err
	}
	var co CampaignCompany
	var unit CompanyUnit
	for cid, d := range lc.details {
		if u, ok := rosterUnit(d.Roster, unitID); ok {
			unit, co = u, mustCompany(lc, cid)
		}
	}
	if unit.ID == 0 && lc.temp != nil && lc.temp.UnitID == unitID {
		unit = CompanyUnit{ID: unitID, Name: lc.temp.Name, Role: RoleHero, Stats: []Attribute{{Name: "Courage", Value: lc.temp.Courage}}}
		co = mustCompany(lc, lc.tempCo)
	}
	if unit.ID == 0 {
		return CourageRoll{}, ErrNotFound
	}
	if err := lc.writable(user, co); err != nil {
		return CourageRoll{}, err
	}
	rs, settings, err := s.ruleset(ctx, id)
	if err != nil {
		return CourageRoll{}, err
	}
	if msg := settings.RollRefusal(entered != nil, thrown); msg != "" {
		return CourageRoll{}, Refuse("%s", msg)
	}
	rules := rs.BC.BreakRules()
	if rules.CourageDice != "2d6" || rules.PassIf != "totalAtLeastCourage" {
		return CourageRoll{}, fmt.Errorf("the rules file asks for a courage test the app does not know (%q, %q)", rules.CourageDice, rules.PassIf)
	}
	snap, err := s.snapshot(ctx, id, lc.match)
	if err != nil {
		return CourageRoll{}, err
	}
	live, ok := lc.state(snap).Unit(unitID)
	if !ok || live.Zone != ZoneField {
		return CourageRoll{}, Refuse("%s is not in the battle.", unit.Title())
	}
	courage := 0
	for _, a := range unit.Stats {
		if a.Name == "Courage" {
			courage = a.Value
		}
	}
	if courage == 0 {
		return CourageRoll{}, Refuse("%s has no Courage to test.", unit.Title())
	}
	dice, err := castDice(s.dice, entered)
	if err != nil { // the live mode answers bad input with ErrInvalidLive (422), not the aftermath's error (which would be a 500 here)
		return CourageRoll{}, invalid("Enter the two dice, each from 1 to 6.")
	}
	total := dice[0] + dice[1]
	return CourageRoll{Dice: dice, Total: total, Courage: courage, Passed: total >= courage}, nil
}

func mustCompany(lc liveContext, id int64) CampaignCompany {
	co, _ := lc.company(id)
	return co
}

// readState folds the live data of a match as the pages see it (the Against the Odds overlay and the Break Point settings included),
// for the callers that have no viewer. false when the match has no live data.
func (s *LiveService) readState(ctx context.Context, id, matchID int64) (LiveState, bool, error) {
	snap, err := s.live.Read(ctx, id, matchID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return LiveState{}, false, nil
		}
		return LiveState{}, false, err
	}
	if !snap.Initialised {
		return LiveState{}, false, nil
	}
	m, err := s.matches.Get(ctx, id, matchID)
	if err != nil {
		return LiveState{}, false, err
	}
	lc, err := s.build(ctx, id, CampaignDetail{}, m)
	if err != nil {
		return LiveState{}, false, err
	}
	return lc.state(snap), true, nil
}

// MatchDefaults is what the live log says about the match's result form: the companies that fled (All is Lost) and those whose
// opponents are all Broken at the end. The second result is false when the match has no live data.
func (s *LiveService) MatchDefaults(ctx context.Context, id, matchID int64) (LiveMatchDefaults, bool, error) {
	st, ok, err := s.readState(ctx, id, matchID)
	if err != nil || !ok {
		return LiveMatchDefaults{}, false, err
	}
	m, err := s.matches.Get(ctx, id, matchID)
	if err != nil {
		return LiveMatchDefaults{}, false, err
	}
	out := LiveMatchDefaults{Fled: map[int64]bool{}, EnemyBroken: map[int64]bool{}}
	for i, side := range m.Sides {
		for _, co := range side.Companies {
			if st.Break(co.ID).Fled {
				out.Fled[co.ID] = true
			}
			enemies, broken := 0, 0
			for j, other := range m.Sides {
				if j == i {
					continue
				}
				for _, oc := range other.Companies {
					enemies++
					if st.Break(oc.ID).Broken {
						broken++
					}
				}
			}
			if enemies > 0 && broken == enemies {
				out.EnemyBroken[co.ID] = true
			}
		}
	}
	return out, true, nil
}
