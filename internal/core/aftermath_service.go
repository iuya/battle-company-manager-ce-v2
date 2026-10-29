package core

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// AftermathService runs the War's Aftermath of an ended match (docs/aftermath.md): the sheet, the injury rolls and their
// effects, succession, experience, influence and the scenario rewards. Every step is one aftermathRepository.Update, which holds
// the company row, so the company and its aftermath change together. The pure rules are in aftermath.go. The service implements
// those methods of web.campaigns.
type AftermathService struct {
	companyDeps
	matches    matchRepository
	aftermaths aftermathRepository
	live       liveDefaults  // optional: the default sheet of a match played live
	ato        atoRepository // optional (WithATO): the Against the Odds selection of the match
}

// NewAftermathService builds the service. rules is the loader (*Rules); dice rolls the casualties.
func NewAftermathService(campaigns campaignRepository, companies companyRepository, matches matchRepository,
	aftermaths aftermathRepository, rules ruleSource, dice Dice) *AftermathService {
	return &AftermathService{companyDeps: companyDeps{campaigns: campaigns, companies: companies, rules: rules, dice: dice},
		matches: matches, aftermaths: aftermaths}
}

// liveDefaults is what the aftermath needs from the live mode (implemented by LiveService): the sheet of a company as the match
// log leaves it; false when the match has no live data.
type liveDefaults interface {
	AftermathDefaults(ctx context.Context, id, matchID, companyID int64) ([]ModelSheet, bool, error)
}

// liveTally is the optional second thing the aftermath asks of the live mode: the MVP vote tally of a company (LiveService has it).
type liveTally interface {
	MVPHint(ctx context.Context, id, matchID, companyID int64) (*MVPHint, error)
}

// WithATO lets the aftermath read the match's Against the Odds selection: the bonuses prefill the sheet, the items a Tool Kit loaned
// can be elected like carried ones.
func (s *AftermathService) WithATO(r atoRepository) *AftermathService { s.ato = r; return s }

// WithLive lets the unsaved default sheet come from the live match instead of "everyone survived".
func (s *AftermathService) WithLive(l liveDefaults) *AftermathService { s.live = l; return s }

func (s *AftermathService) aftermathRules(ctx context.Context, h RulesetHashes) (AftermathRules, error) {
	rs, err := s.rules.For(ctx, h)
	if err != nil {
		return AftermathRules{}, err
	}
	return rs.BC.Aftermath(), nil
}

// AftermathRules is what the aftermath pages read from the campaign's Battle Companies file; core.ErrNotFound for an unknown campaign.
func (s *AftermathService) AftermathRules(ctx context.Context, id int64) (AftermathRules, error) {
	h, _, err := s.campaigns.Hashes(ctx, id)
	if err != nil {
		return AftermathRules{}, err
	}
	return s.aftermathRules(ctx, h)
}

// amEnv is everything a step needs, loaded before the transaction.
type amEnv struct {
	campaignID, matchID, companyID int64
	user                           User
	match                          Match
	company                        CampaignCompany
	organiser                      int64
	rs                             *RuleSet
	settings                       CampaignSettings
	rules                          AftermathRules
	outcome                        string
	fled                           bool
	access                         AftermathAccess
	finished                       bool         // the campaign is finished: the aftermath can be read, not changed
	loaned                         []LoanedItem // items loaned to units for this match (a Healer's Kit); electable like carried ones
	atoBonuses                     []string     // the Against the Odds bonuses the company took, in the file's order, that matter in the aftermath
}

// load finds the match and the company and works out who the viewer is. An open match refuses the aftermath.
func (s *AftermathService) load(ctx context.Context, user User, id, matchID, companyID int64) (*amEnv, error) {
	camp, err := s.campaigns.Get(ctx, user.ID, id)
	if err != nil {
		return nil, err
	}
	m, err := s.matches.Get(ctx, id, matchID)
	if err != nil {
		return nil, err
	}
	outcome, fled, ok := CompanyOutcome(m, companyID)
	if !ok {
		return nil, ErrNotFound
	}
	if !m.Ended {
		return nil, Refuse("End the match first.")
	}
	var co CampaignCompany
	for _, side := range m.Sides {
		for _, c := range side.Companies {
			if c.ID == companyID {
				co = c
			}
		}
	}
	h, settings, err := s.campaigns.Hashes(ctx, id)
	if err != nil {
		return nil, err
	}
	rs, err := s.rules.For(ctx, h)
	if err != nil {
		return nil, err
	}
	rules, err := s.aftermathRules(ctx, h)
	if err != nil {
		return nil, err
	}
	env := &amEnv{campaignID: id, matchID: matchID, companyID: companyID, user: user, match: m, company: co, organiser: camp.OrganiserID,
		rs: rs, settings: settings, rules: rules, outcome: outcome, fled: fled, access: AftermathAccessFor(user, m, co, camp.OrganiserID), finished: camp.Status == CampaignFinished}
	if s.ato != nil {
		rec, err := s.ato.Get(ctx, id, matchID)
		if err != nil {
			return nil, err
		}
		if rec.CompanyID == companyID {
			env.loaned = rs.BC.ATO().LoanedFor(rec, companyID)
			for _, key := range rec.Selection.Bonuses {
				if b, ok := rules.Bonus(key); ok && b.Effective() {
					env.atoBonuses = append(env.atoBonuses, key)
				}
			}
		}
	}
	return env, nil
}

func (e *amEnv) requireEdit() error {
	if e.access != AftermathEdit {
		return Refuse("Only the owner of %s, the organiser of the campaign or an administrator can change this aftermath.", e.company.Name)
	}
	return nil
}

func (e *amEnv) scenarioName() string {
	if e.match.Scenario != nil && e.match.Scenario.Kind == ScenarioRulebook {
		return e.match.Scenario.Name
	}
	return ""
}

func (e *amEnv) where() string {
	if n := e.scenarioName(); n != "" {
		return fmt.Sprintf("Match %d, %s", e.match.Number, n)
	}
	return fmt.Sprintf("Match %d", e.match.Number)
}

// detail is the company's detail with the units titled so that they can be told apart (Distinguish): the aftermath's lines,
// casualties and options are written with those titles.
func (e *amEnv) detail(st CompanyState) CompanyDetail {
	d := BuildDetail(e.rs, e.settings, st)
	d.Roster = Distinguish(d.Roster)
	return d
}

// context is the pure rules' view of the match: the roster is the one the sheet was written for once started.
func (e *amEnv) context(st CompanyState, rec AftermathRecord) AftermathContext {
	roster := rec.Work.Roster
	if roster == nil {
		roster = e.detail(st).Roster
	}
	return AftermathContext{Rules: e.rules, Outcome: e.outcome, Fled: e.fled, Scenario: e.scenarioName(),
		Result: e.match.Result.Company(e.companyID), Roster: roster}
}

func line(rec *AftermathRecord, kind, text string, unit int64, amount int) {
	rec.Lines = append(rec.Lines, AftermathLine{Kind: kind, Text: text, UnitID: unit, Amount: amount})
}

// chartRow finds the row of the chart a casualty rolled.
func (e *amEnv) chartRow(cas Casualty) InjuryRow {
	for _, row := range e.rules.Chart(cas.Hero) {
		if row.Key == cas.ResultKey && row.Covers(cas.Total) {
			return row
		}
	}
	return InjuryRow{}
}

// view builds what the page shows.
func (s *AftermathService) view(ctx context.Context, e *amEnv, st CompanyState, rec AftermathRecord) Aftermath {
	status := rec.Status
	if status == "" {
		status = AftermathPending
	}
	a := Aftermath{MatchID: e.matchID, CompanyID: e.companyID, Status: status, Revision: rec.Revision, Outcome: e.outcome, Fled: e.fled,
		Absent: slices.Clone(rec.Work.Absent), Casualties: slices.Clone(rec.Work.Casualties), Lines: slices.Clone(rec.Lines),
		Rerolls: rec.Work.Rerolls, WaivedNote: rec.Work.WaivedNote, Access: e.access}
	on := time.Time{}
	switch {
	case rec.FinishedAt != nil:
		on = *rec.FinishedAt
	case rec.StartedAt != nil:
		on = *rec.StartedAt
	case e.match.EndedOn != nil:
		on = *e.match.EndedOn
	}
	for i := range a.Lines {
		a.Lines[i].Match, a.Lines[i].MatchID, a.Lines[i].On = e.match.Number, e.matchID, on
	}
	d := e.detail(st)
	if !status.Started() && status != AftermathWaived {
		for _, u := range d.Roster {
			if Absent(u) {
				a.Absent = append(a.Absent, u.ID)
			}
		}
	}
	in := rec.Input
	if len(in.Models) == 0 && !status.Started() {
		in = DefaultSheet(d.Roster)
		if s.live != nil { // the log of a live match decides outcomes and gains; a saved or started sheet never reads it again
			if sheets, ok, err := s.live.AftermathDefaults(ctx, e.campaignID, e.matchID, e.companyID); err == nil && ok {
				for i, m := range in.Models {
					if j := slices.IndexFunc(sheets, func(x ModelSheet) bool { return x.UnitID == m.UnitID }); j >= 0 {
						in.Models[i] = sheets[j]
					}
				}
				a.FromLive = true
				if t, ok := s.live.(liveTally); ok {
					if hint, err := t.MVPHint(ctx, e.campaignID, e.matchID, e.companyID); err == nil {
						a.MVP = hint
					}
				}
			}
		}
	}
	if len(rec.Input.Models) == 0 && !status.Started() && len(e.atoBonuses) > 0 && len(in.AgainstTheOdds) == 0 {
		in.AgainstTheOdds = slices.Clone(e.atoBonuses) // the bonuses the company took before the game; the player can still change them
		a.FromMatch = true
	}
	a.Input = in
	if !status.Started() && status != AftermathWaived {
		a.ItemChoices = in.ItemChoices(d.Roster, e.loaned, e.rules)
	}
	if status == AftermathWaived || (len(in.Models) == 0 && status != AftermathDone) {
		return a
	}
	amc := e.context(st, rec)
	a.Preview = Preview(amc, in)
	for _, cas := range rec.Work.Casualties {
		if row := e.chartRow(cas); row.BonusKind == "influence" && cas.Bonus > 0 && cas.Kind != CasualtyDead && cas.Kind != CasualtyRetired {
			a.Preview.Influence = append(a.Preview.Influence, AmountLine{Text: row.Name, Amount: cas.Bonus, UnitID: cas.UnitID, Key: row.Key})
		}
	}
	for _, rw := range a.Preview.Rewards {
		if rw.Decision && rec.Work.Answers["reward:"+rw.Key] == "" {
			a.Pending = append(a.Pending, rw)
		}
	}
	if status == AftermathInjuries && rec.Work.Applied {
		a.Succession = successionOpen(d.Roster, rec, e.rs.BC.Shape())
		a.NoOneToLead = a.Succession == nil && rec.Work.LeaderFell && !slices.ContainsFunc(d.Roster, func(u CompanyUnit) bool { return u.Role == RoleLeader })
	}
	return a
}

// write runs a step on the company and its record, bumps the revision and returns the new view.
func (s *AftermathService) write(ctx context.Context, user User, id, matchID, companyID int64, edit bool,
	fn func(e *amEnv, st *CompanyState, rec *AftermathRecord) error) (Aftermath, error) {
	e, err := s.load(ctx, user, id, matchID, companyID)
	if err != nil {
		return Aftermath{}, err
	}
	if e.finished {
		return Aftermath{}, Refuse("The campaign is finished.")
	}
	if edit {
		if err := e.requireEdit(); err != nil {
			return Aftermath{}, err
		}
	}
	var out Aftermath
	err = s.aftermaths.Update(ctx, id, matchID, companyID, func(st *CompanyState, rec *AftermathRecord) error {
		if !rec.MatchEnded {
			return Refuse("End the match first.")
		}
		if rec.Status == "" {
			rec.Status = AftermathPending
		}
		if err := fn(e, st, rec); err != nil {
			return err
		}
		rec.Revision++
		out = s.view(ctx, e, *st, *rec)
		return nil
	})
	return out, err
}

// Aftermath loads the state (a computed default sheet when none is saved).
func (s *AftermathService) Aftermath(ctx context.Context, user User, id, matchID, companyID int64) (Aftermath, error) {
	e, err := s.load(ctx, user, id, matchID, companyID)
	if err != nil {
		return Aftermath{}, err
	}
	rec, err := s.aftermaths.Get(ctx, id, matchID, companyID)
	if err != nil {
		return Aftermath{}, err
	}
	st, err := s.companies.Read(ctx, id, companyID)
	if err != nil {
		return Aftermath{}, err
	}
	return s.view(ctx, e, st, rec), nil
}

// AftermathStatuses lists every company's status in the match.
func (s *AftermathService) AftermathStatuses(ctx context.Context, id, matchID int64) ([]AftermathRef, error) {
	return s.aftermaths.Statuses(ctx, id, matchID)
}

// PendingAftermaths lists the aftermaths a company still has to finish (the lock), oldest first.
func (s *AftermathService) PendingAftermaths(ctx context.Context, id, companyID int64) ([]AftermathRef, error) {
	return s.aftermaths.Pending(ctx, id, companyID)
}

// Started says whether a company has started the aftermath of the match: it cannot be reopened. It is what
// Matches.AftermathStarted is set to.
func (s *AftermathService) Started(ctx context.Context, id, matchID int64) (bool, error) {
	return s.aftermaths.Started(ctx, id, matchID)
}

// SaveAftermath stores the sheet as a draft.
func (s *AftermathService) SaveAftermath(ctx context.Context, user User, id, matchID, companyID int64, revision int, in AftermathInput) (Aftermath, error) {
	return s.sheet(ctx, user, id, matchID, companyID, revision, in, false)
}

// StartAftermath stores the sheet, locks it and starts the injuries.
func (s *AftermathService) StartAftermath(ctx context.Context, user User, id, matchID, companyID int64, revision int, in AftermathInput) (Aftermath, error) {
	return s.sheet(ctx, user, id, matchID, companyID, revision, in, true)
}

func (s *AftermathService) sheet(ctx context.Context, user User, id, matchID, companyID int64, revision int, in AftermathInput, start bool) (Aftermath, error) {
	return s.write(ctx, user, id, matchID, companyID, true, func(e *amEnv, st *CompanyState, rec *AftermathRecord) error {
		switch {
		case rec.Status != AftermathPending && rec.Status != AftermathSheet:
			return Refuse("The sheet is locked: the aftermath has started.")
		case revision != rec.Revision:
			return ErrAftermathChanged
		}
		roster := e.detail(*st).Roster
		in, err := in.ValidateWith(roster, e.loaned, e.rules, e.match.Result.Company(companyID))
		if err != nil {
			return err
		}
		rec.Input, rec.Status = in, AftermathSheet
		if !start {
			return nil
		}
		// Start: the units that missed this game recover, the re-rolls are credited, one casualty per unit
		w := &rec.Work
		*w = AftermathWork{Roster: slices.Clone(roster), Undo: map[int64][2]bool{}, Second: map[int64]bool{}, Answers: map[string]string{}}
		for _, u := range roster {
			if !Absent(u) {
				continue
			}
			us, _ := stateUnit(st, u.ID)
			w.Absent = append(w.Absent, u.ID)
			w.Undo[u.ID] = [2]bool{us.Injured, us.SentToHealers}
			us.Injured, us.SentToHealers = false, false
			line(rec, "injury", u.Title()+" missed this game and recovers", u.ID, 0)
		}
		w.PrevRerolls = st.Rerolls
		w.Rerolls = RerollsFor(e.rules, in)
		st.Rerolls += w.Rerolls
		for _, m := range in.Models {
			if m.Outcome == OutcomeCasualty {
				u, _ := rosterUnit(roster, m.UnitID)
				cas := Casualty{UnitID: m.UnitID, Name: u.Title(), Hero: u.IsHero()}
				if e.rules.noRoll(m.UsesItem) { // Wondrous Cram: no roll, the chart's result stands
					if err := replaceRoll(e, st, &cas, e.rules.Effects[m.UsesItem]); err != nil {
						return err
					}
				}
				w.Casualties = append(w.Casualties, cas)
			}
		}
		rec.Status = AftermathInjuries
		now := time.Now()
		rec.StartedAt = &now
		return s.advance(e, st, rec)
	})
}

// ReopenSheet unlocks the sheet again; only before the first roll.
func (s *AftermathService) ReopenSheet(ctx context.Context, user User, id, matchID, companyID int64, revision int) (Aftermath, error) {
	return s.write(ctx, user, id, matchID, companyID, true, func(e *amEnv, st *CompanyState, rec *AftermathRecord) error {
		w := rec.Work
		switch {
		case revision != rec.Revision:
			return ErrAftermathChanged
		case rec.Status != AftermathInjuries || w.Applied || slices.ContainsFunc(w.Casualties, func(c Casualty) bool { return c.Rolled }):
			return Refuse("The sheet can only be unlocked before the first injury roll.")
		}
		for uid, prev := range w.Undo {
			if us, ok := stateUnit(st, uid); ok {
				us.Injured, us.SentToHealers = prev[0], prev[1]
			}
		}
		st.Rerolls = w.PrevRerolls
		rec.Status, rec.Work, rec.Lines, rec.StartedAt = AftermathSheet, AftermathWork{}, nil, nil
		return nil
	})
}

// castDice rolls the injury dice (campaign.injuries.roll), or checks the dice the player entered: one per die, each 1..Sides.
func castDice(d Dice, spec DiceSpec, entered []int) ([]int, error) {
	if entered == nil {
		return RollDice(d, spec), nil
	}
	if len(entered) != spec.Count || slices.ContainsFunc(entered, func(n int) bool { return n < 1 || n > spec.Sides }) {
		return nil, invalidAftermath(fmt.Sprintf("enter %s, each from 1 to %d", diceWords(spec.Count, "die", "dice"), spec.Sides))
	}
	return slices.Clone(entered), nil
}

func diceWords(n int, one, many string) string {
	words := []string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten"}
	if n == 1 {
		return "one " + one
	}
	return "the " + words[min(n, len(words)-1)] + " " + many
}

func invalidAftermath(msg string) error { return fmt.Errorf("%w: %s", ErrInvalidAftermath, msg) }

// roll resolves one casualty with the dice cast and rolls its bonus dice.
func (s *AftermathService) roll(e *amEnv, st *CompanyState, rec *AftermathRecord, i int, dice []int) {
	cas := &rec.Work.Casualties[i]
	cu := CasualtyUnit{ID: cas.UnitID, Hero: cas.Hero, Injuries: map[string]int{}}
	if us, ok := stateUnit(st, cas.UnitID); ok {
		cu.Injuries = cloneMap(us.Injuries)
		if cu.Injuries == nil {
			cu.Injuries = map[string]int{}
		}
	}
	resolve := ResolveCasualty
	if rec.Work.Second[cas.UnitID] {
		resolve = ResolveScratchReroll
	}
	total, shown := RollModifierOf(e.rules, rec.Input).Apply(sumOf(dice)) // Healing Herbs: +1, held to the clamp
	eff, _ := resolve(e.rules.Chart(cas.Hero), total, cu)                 // the chart covers Min..Max of the injury dice
	cas.Rolled, cas.Dice, cas.Total, cas.Modifier = true, dice, total, shown
	cas.ResultKey, cas.ResultTxt, cas.Text = eff.Row.Key, eff.Row.Name, eff.Row.Text
	cas.Kind, cas.InjuryKey, cas.Count, cas.Decision = eff.Kind, eff.InjuryKey, eff.Count, eff.Decision
	cas.Bonus = 0
	if eff.Row.BonusDice != "" && eff.Decision == nil {
		if e.settings.ManualRolls { // the player has a die of their own: ask for it (Decide, kind "bonus")
			cas.Decision = &CasualtyDecision{Kind: "bonus"}
		} else {
			cas.Bonus = eff.Bonus(s.dice)
		}
	}
}

// replaceRoll makes the casualty roll-less: it counts as having rolled the result the effect names (Wondrous Cram: Full Recovery).
// Its total is the first of the result's rows so that the chart reads back the same row; a hero's Full Recovery still offers to
// heal an injury. The bonus dice of the row (none for Full Recovery) are not rolled.
func replaceRoll(e *amEnv, st *CompanyState, cas *Casualty, eff RewardEffect) error {
	chart := e.rules.Chart(cas.Hero)
	i := slices.IndexFunc(chart, func(r InjuryRow) bool { return r.Key == eff.Result })
	if i < 0 {
		return invalidAftermath(fmt.Sprintf("the injuries chart has no %q result for %s", eff.Result, e.rules.itemName(eff.Key)))
	}
	cu := CasualtyUnit{ID: cas.UnitID, Hero: cas.Hero, Injuries: map[string]int{}}
	if us, ok := stateUnit(st, cas.UnitID); ok && us.Injuries != nil {
		cu.Injuries = cloneMap(us.Injuries)
	}
	res, _ := ResolveCasualty(chart, chart[i].Min, cu)
	cas.Rolled, cas.Dice, cas.Total, cas.Item = true, nil, chart[i].Min, eff.Key
	cas.ResultKey, cas.ResultTxt, cas.Text = res.Row.Key, res.Row.Name, res.Row.Text
	cas.Kind, cas.InjuryKey, cas.Count, cas.Decision = res.Kind, res.InjuryKey, res.Count, res.Decision
	return nil
}

// consumeItems removes the consumables the sheet elected from the units that carried them (a loaned one is simply spent) and
// writes one line each. Called at Finish, never before: a waived aftermath consumes nothing.
func consumeItems(e *amEnv, st *CompanyState, rec *AftermathRecord) {
	for _, m := range rec.Input.Models {
		eff, ok := e.rules.Effects[m.UsesItem]
		if m.UsesItem == "" || !ok {
			continue
		}
		u, _ := rosterUnit(rec.Work.Roster, m.UnitID)
		var names []string
		for item, key := range e.rules.ItemEffects {
			if key == eff.Key {
				names = append(names, item)
			}
		}
		if us, found := stateUnit(st, m.UnitID); found {
			for _, list := range []*[]string{&us.Bought, &us.Gear} {
				if i := slices.IndexFunc(*list, func(n string) bool { return slices.Contains(names, NameKey(n)) }); i >= 0 {
					*list = slices.Delete(*list, i, i+1)
					break
				}
			}
		}
		text := fmt.Sprintf("%s used %s", u.Title(), e.rules.itemName(eff.Key))
		if eff.NoRoll {
			for _, cas := range rec.Work.Casualties {
				if cas.UnitID == m.UnitID {
					text += ": " + cas.ResultTxt
				}
			}
		}
		line(rec, "injury", text, m.UnitID, 0)
	}
}

// decideBonus answers the bonus dice of Lesson Learned (a D3) or Wounds of a Hero (a D6), whatever the rules file says: "0" lets
// the server roll, a number is the roll the player made (see TypedMax and FromTyped: a D3 is rolled on a D6).
func (s *AftermathService) decideBonus(e *amEnv, rec *AftermathRecord, ans AftermathAnswer) error {
	w := &rec.Work
	i := slices.IndexFunc(w.Casualties, func(c Casualty) bool { return c.UnitID == ans.UnitID })
	if i < 0 {
		return ErrNotFound
	}
	cas := &w.Casualties[i]
	if w.Applied || cas.Decision == nil || cas.Decision.Kind != "bonus" {
		return Refuse("%s has no such question open.", cas.Name)
	}
	eff := CasualtyEffect{Row: e.chartRow(*cas)}
	spec := eff.Row.BonusSpec
	n, err := strconv.Atoi(ans.Answer)
	if err != nil || n < 0 || n > TypedMax(spec) || n > 0 && n < spec.Min() {
		return invalidAftermath(fmt.Sprintf("enter the roll, from %d to %d, or 0 to roll it for you", spec.Min(), TypedMax(spec)))
	}
	if n > 0 {
		if msg := e.settings.RollRefusal(true, false); msg != "" {
			return Refuse("%s", msg)
		}
		cas.Bonus, _ = FromTyped(spec, n)
	} else {
		cas.Bonus = s.dice.Roll(spec)
	}
	cas.Decision = nil
	return nil
}

// RollCasualty rolls one casualty (unitID) or all the unrolled ones (unitID 0, no entered dice).
func (s *AftermathService) RollCasualty(ctx context.Context, user User, id, matchID, companyID, unitID int64, entered []int, thrown bool) (Aftermath, error) {
	return s.write(ctx, user, id, matchID, companyID, true, func(e *amEnv, st *CompanyState, rec *AftermathRecord) error {
		if msg := e.settings.RollRefusal(entered != nil, thrown); msg != "" {
			return Refuse("%s", msg)
		}
		if unitID == 0 && entered != nil {
			return Refuse("Roll all takes no typed dice: roll the casualties one by one.")
		}
		if rec.Status != AftermathInjuries || rec.Work.Applied {
			return Refuse("There are no injury rolls to make.")
		}
		rolled := false
		for i := range rec.Work.Casualties {
			cas := &rec.Work.Casualties[i]
			if (unitID != 0 && cas.UnitID != unitID) || cas.Rolled {
				continue
			}
			dice, err := castDice(s.dice, e.rules.InjuryDice, entered)
			if err != nil {
				return err
			}
			s.roll(e, st, rec, i, dice)
			rolled = true
		}
		if !rolled {
			if unitID != 0 && !slices.ContainsFunc(rec.Work.Casualties, func(c Casualty) bool { return c.UnitID == unitID }) {
				return ErrNotFound
			}
			return Refuse("Nothing to roll.")
		}
		return s.advance(e, st, rec)
	})
}

// RerollCasualty spends one Against the Odds re-roll on a rolled casualty that is not final.
func (s *AftermathService) RerollCasualty(ctx context.Context, user User, id, matchID, companyID, unitID int64, entered []int, thrown bool) (Aftermath, error) {
	return s.write(ctx, user, id, matchID, companyID, true, func(e *amEnv, st *CompanyState, rec *AftermathRecord) error {
		if msg := e.settings.RollRefusal(entered != nil, thrown); msg != "" {
			return Refuse("%s", msg)
		}
		w := &rec.Work
		i := slices.IndexFunc(w.Casualties, func(c Casualty) bool { return c.UnitID == unitID })
		switch {
		case i < 0:
			return ErrNotFound
		case rec.Status != AftermathInjuries || w.Applied:
			return Refuse("The injuries are settled; a result can no longer be re-rolled.")
		case w.Casualties[i].Item != "":
			return Refuse("%s used %s: there was no roll to re-roll.", w.Casualties[i].Name, e.rules.itemName(w.Casualties[i].Item))
		case !w.Casualties[i].Rolled:
			return Refuse("Roll it first.")
		case w.Rerolls <= 0 || st.Rerolls <= 0:
			return Refuse("No Against the Odds re-rolls left.")
		}
		dice, err := castDice(s.dice, e.rules.InjuryDice, entered)
		if err != nil {
			return err
		}
		cas := &w.Casualties[i]
		cas.Previous = append(cas.Previous, cas.ResultTxt)
		cas.Rerolled = true
		s.roll(e, st, rec, i, dice)
		w.Rerolls--
		st.Rerolls--
		line(rec, "injury", fmt.Sprintf("%s: re-rolled, now %s", cas.Name, cas.ResultTxt), cas.UnitID, 0)
		return s.advance(e, st, rec) // the last re-roll leaves nothing to wait for: the step closes
	})
}

// Decide answers an open decision.
func (s *AftermathService) Decide(ctx context.Context, user User, id, matchID, companyID int64, ans AftermathAnswer) (Aftermath, error) {
	return s.write(ctx, user, id, matchID, companyID, true, func(e *amEnv, st *CompanyState, rec *AftermathRecord) error {
		if rec.Status != AftermathInjuries && rec.Status != AftermathLevelUps {
			return Refuse("There is nothing to decide now.")
		}
		var err error
		switch ans.Kind {
		case "scratch", "heal":
			err = decideCasualty(rec, ans)
		case "bonus":
			err = s.decideBonus(e, rec, ans)
		case "succession", "sergeant":
			err = decideSuccession(e, st, rec, ans)
		case "reward":
			err = s.decideReward(e, st, rec, ans)
		case "continue":
			w := rec.Work
			if rec.Status != AftermathInjuries || w.Applied || slices.ContainsFunc(w.Casualties, func(c Casualty) bool { return !c.Rolled || c.Decision != nil }) {
				return Refuse("Roll every casualty and answer the questions first.")
			}
			rec.Work.Continued = true
		default:
			err = invalidAftermath("unknown decision")
		}
		if err != nil {
			return err
		}
		return s.advance(e, st, rec)
	})
}

func decideCasualty(rec *AftermathRecord, ans AftermathAnswer) error {
	w := &rec.Work
	i := slices.IndexFunc(w.Casualties, func(c Casualty) bool { return c.UnitID == ans.UnitID })
	if i < 0 {
		return ErrNotFound
	}
	cas := &w.Casualties[i]
	if w.Applied || cas.Decision == nil || cas.Decision.Kind != ans.Kind {
		return Refuse("%s has no such question open.", cas.Name)
	}
	switch ans.Kind {
	case "scratch":
		switch ans.Answer {
		case "keep":
			cas.Kind, cas.Decision = CasualtyInjured, nil
		case "reroll":
			cas.Previous = append(cas.Previous, cas.ResultTxt)
			cas.Rolled, cas.Decision, cas.Dice, cas.Total, cas.ResultKey, cas.ResultTxt, cas.Text = false, nil, nil, 0, "", "", ""
			if w.Second == nil {
				w.Second = map[int64]bool{}
			}
			w.Second[cas.UnitID] = true
		default:
			return invalidAftermath("answer keep or reroll")
		}
	case "heal":
		if ans.Answer != "" && !slices.ContainsFunc(cas.Decision.Options, func(o DecisionOption) bool { return o.Key == ans.Answer }) {
			return invalidAftermath("that injury cannot be healed")
		}
		cas.InjuryKey, cas.Decision = ans.Answer, nil
	}
	return nil
}

func (s *AftermathService) decideReward(e *amEnv, st *CompanyState, rec *AftermathRecord, ans AftermathAnswer) error {
	if rec.Status != AftermathLevelUps {
		return Refuse("Rewards are chosen once the injuries are settled.")
	}
	rewards := Rewards(e.context(*st, *rec), rec.Input)
	i := slices.IndexFunc(rewards, func(x RewardApply) bool { return x.Key == ans.Key && x.Decision })
	if i < 0 {
		return ErrNotFound
	}
	item, unit, _ := strings.Cut(ans.Answer, "|")
	uid, _ := strconv.ParseInt(unit, 10, 64)
	if err := checkFreeEquipment(e.rs, e.settings, *st, rewards[i].MaxValue, item, uid); err != nil {
		return err
	}
	if rec.Work.Answers == nil {
		rec.Work.Answers = map[string]string{}
	}
	rec.Work.Answers["reward:"+ans.Key] = item + "|" + unit
	return nil
}

// successionOpen says what the company still has to decide after its Leader fell or a Sergeant was lost, or nil.
func successionOpen(roster []CompanyUnit, rec AftermathRecord, shape CompanyRules) *Succession {
	w := rec.Work
	if !w.LeaderFell && w.SergeantsToFill == 0 {
		return nil
	}
	if w.LeaderFell {
		if sc := NextSuccession(roster, true, shape.Leaders); sc.NeedLeader {
			return &sc
		}
	}
	if !w.NoSergeant {
		if sc := NextSergeant(roster, w.SergeantsToFill > 0, shape.Sergeants); sc.NeedSergeant {
			return &sc
		}
	}
	return nil
}

func decideSuccession(e *amEnv, st *CompanyState, rec *AftermathRecord, ans AftermathAnswer) error {
	roster := e.detail(*st).Roster
	open := successionOpen(roster, *rec, e.rs.BC.Shape())
	if !rec.Work.Applied || open == nil {
		return Refuse("Nobody has to be chosen now.")
	}
	uid, _ := strconv.ParseInt(ans.Answer, 10, 64)
	us, found := stateUnit(st, uid)
	title := ""
	if u, ok := rosterUnit(roster, uid); ok {
		title = u.Title()
	}
	switch {
	case ans.Kind == "succession" && open.NeedLeader:
		if len(open.Fallback) > 0 { // no hero left: a warrior is treated as having rolled Hero in the Making
			if !found || !slices.Contains(open.Fallback, uid) {
				return invalidAftermath("choose one of the Warriors with the most Experience")
			}
			us.Role = RoleLeader
			us.Bonus = mergeBonus(us.Bonus, e.rs.BC.HeroBonus(nil)) // an officer without a path: it chooses one in the path window
			line(rec, "succession", title+" becomes the Leader (Hero in the Making: choose its path)", uid, 0)
			return nil
		}
		if !found || !slices.Contains(open.LeaderOptions, uid) {
			return invalidAftermath("choose one of the heroes offered")
		}
		us.Role = RoleLeader
		line(rec, "succession", title+" becomes the Leader", uid, 0)
		return nil
	case ans.Kind == "sergeant" && !open.NeedLeader && open.NeedSergeant:
		if len(open.Fallback) > 0 { // no hero left to replace a Sergeant who died: a Warrior is treated as having rolled Hero in the Making
			if !found || !slices.Contains(open.Fallback, uid) {
				return invalidAftermath("choose one of the Warriors with the most Experience")
			}
			us.Role = RoleSergeant
			us.Bonus = mergeBonus(us.Bonus, e.rs.BC.HeroBonus(nil))
			rec.Work.SergeantsToFill = max(0, rec.Work.SergeantsToFill-1)
			line(rec, "succession", title+" becomes a Sergeant (Hero in the Making: choose its path)", uid, 0)
			return nil
		}
		if ans.Answer == "" {
			rec.Work.NoSergeant = true
			rec.Work.SergeantsToFill = 0
			return nil
		}
		if !found || !slices.Contains(open.SergeantOptions, uid) {
			return invalidAftermath("choose one of the heroes offered")
		}
		us.Role = RoleSergeant
		rec.Work.SergeantsToFill = max(0, rec.Work.SergeantsToFill-1)
		line(rec, "succession", title+" becomes a Sergeant", uid, 0)
		return nil
	}
	return Refuse("That is not what the company has to choose now.")
}

// advance moves the injuries step on when it can: all casualties rolled and no question open (and, with re-rolls left, the
// player moved on), then the effects are applied, then the Leader's succession settled, then the experience goes to the units.
func (s *AftermathService) advance(e *amEnv, st *CompanyState, rec *AftermathRecord) error {
	if rec.Status != AftermathInjuries {
		return nil
	}
	w := &rec.Work
	if !w.Applied {
		for _, cas := range w.Casualties {
			if !cas.Rolled || cas.Decision != nil {
				return nil
			}
		}
		if w.Rerolls > 0 && len(w.Casualties) > 0 && !w.Continued {
			return nil
		}
		if err := s.applyInjuries(e, st, rec); err != nil {
			return err
		}
	}
	if successionOpen(e.detail(*st).Roster, *rec, e.rs.BC.Shape()) != nil {
		return nil
	}
	applyExperience(e, st, rec)
	rec.Status = AftermathLevelUps
	return nil
}

// applyInjuries makes the casualties' results real.
func (s *AftermathService) applyInjuries(e *amEnv, st *CompanyState, rec *AftermathRecord) error {
	w := &rec.Work
	for i := range w.Casualties {
		cas := &w.Casualties[i]
		cas.Final = true
		row := e.chartRow(*cas)
		us, ok := stateUnit(st, cas.UnitID)
		if !ok {
			return ErrNotFound
		}
		wasLeader, wasSergeant := us.Role == RoleLeader, us.Role == RoleSergeant
		switch cas.Kind {
		case CasualtyDead, CasualtyRetired:
			fate := "Dead"
			if cas.Kind == CasualtyRetired {
				fate = "Retired"
				if us.Injuries == nil {
					us.Injuries = map[string]int{}
				}
				us.Injuries[cas.InjuryKey]++
			}
			if err := bury(e.rs, e.settings, st, cas.UnitID, fate, e.where()); err != nil {
				return err
			}
			if wasLeader {
				w.LeaderFell = true
			}
			if wasSergeant {
				w.SergeantsToFill++
			}
			text := cas.Name + ": " + fate
			if cas.Kind == CasualtyRetired {
				text = fmt.Sprintf("%s: forced to retire (%s number %d)", cas.Name, row.Name, us.Injuries[cas.InjuryKey])
			}
			line(rec, "injury", text, cas.UnitID, 0)
			continue
		case CasualtyInjured, CasualtyScratch:
			us.Injured = true
			line(rec, "injury", cas.Name+": "+cas.ResultTxt+", misses the next game", cas.UnitID, 0)
		case CasualtyInjury:
			if us.Injuries == nil {
				us.Injuries = map[string]int{}
			}
			us.Injuries[cas.InjuryKey]++
			line(rec, "injury", fmt.Sprintf("%s: %s (%d)", cas.Name, cas.ResultTxt, us.Injuries[cas.InjuryKey]), cas.UnitID, 0)
		case CasualtyHeal:
			if cas.InjuryKey != "" {
				if n := us.Injuries[cas.InjuryKey]; n > 1 {
					us.Injuries[cas.InjuryKey] = n - 1
				} else {
					delete(us.Injuries, cas.InjuryKey)
				}
				line(rec, "injury", cas.Name+": Full Recovery, heals "+cas.InjuryKey, cas.UnitID, 0)
			} else {
				line(rec, "injury", cas.Name+": Full Recovery", cas.UnitID, 0)
			}
		default:
			line(rec, "injury", cas.Name+": "+cas.ResultTxt, cas.UnitID, 0)
		}
		for _, stat := range slices.Sorted(maps.Keys(row.StatGains)) {
			gainStat(e, st, rec, cas, stat, row.StatGains[stat])
		}
		if row.BonusKind == "experience" && cas.Bonus > 0 {
			us, _ = stateUnit(st, cas.UnitID)
			us.XP += cas.Bonus
			line(rec, "experience", fmt.Sprintf("%s: %s, +%d Experience", cas.Name, row.Name, cas.Bonus), cas.UnitID, cas.Bonus)
		}
	}
	w.Applied = true
	return nil
}

// gainStat gives a hero permanent points of a stat (Fate for Protection by the Valar), up to the maximum of its path.
func gainStat(e *amEnv, st *CompanyState, rec *AftermathRecord, cas *Casualty, stat string, points int) {
	name := strings.ToUpper(stat[:1]) + stat[1:]
	d := e.detail(*st)
	u, ok := rosterUnit(d.Roster, cas.UnitID)
	us, found := stateUnit(st, cas.UnitID)
	if !ok || !found || u.Path == nil {
		return
	}
	rules := e.rs.BC.Progression()
	path, _ := rules.Path(u.Path.Name)
	sheet := sheetOf(u, us.Progress, d.Roster, nil)
	for range points {
		if _, can := rules.improvedStat(path, sheet, stat, 1, 0); !can {
			line(rec, "injury", cas.Name+" is at the most "+name+" its path allows", cas.UnitID, 0)
			return
		}
		if us.Progress == nil {
			us.Progress = &Progress{}
		}
		if us.Progress.Bonus == nil {
			us.Progress.Bonus = map[string]int{}
		}
		if rules.LowerIsBetter[stat] { // Courage and Shoot improve by going down
			us.Progress.Bonus[stat]--
		} else {
			us.Progress.Bonus[stat]++
		}
		sheet = sheetOf(mustUnit(e.detail(*st).Roster, cas.UnitID), us.Progress, d.Roster, nil)
		line(rec, "injury", cas.Name+" gains 1 "+name+" point", cas.UnitID, 0)
	}
}

func mustUnit(roster []CompanyUnit, id int64) CompanyUnit {
	u, _ := rosterUnit(roster, id)
	return u
}

// applyExperience gives every unit still on the roster its experience.
func applyExperience(e *amEnv, st *CompanyState, rec *AftermathRecord) {
	for _, g := range Experience(e.context(*st, *rec), rec.Input) {
		us, ok := stateUnit(st, g.UnitID)
		if !ok || g.Total == 0 {
			continue
		}
		us.XP += g.Total
		u, _ := rosterUnit(rec.Work.Roster, g.UnitID)
		line(rec, "experience", fmt.Sprintf("%s: +%d Experience", u.Title(), g.Total), g.UnitID, g.Total)
	}
}

// FinishAftermath applies Influence and the scenario rewards, clears the re-rolls and unlocks the company.
func (s *AftermathService) FinishAftermath(ctx context.Context, user User, id, matchID, companyID int64, revision int) (Aftermath, error) {
	return s.write(ctx, user, id, matchID, companyID, true, func(e *amEnv, st *CompanyState, rec *AftermathRecord) error {
		switch {
		case revision != rec.Revision:
			return ErrAftermathChanged
		case rec.Status != AftermathLevelUps:
			return Refuse("The injuries must be settled before the aftermath can finish.")
		}
		ctxA := e.context(*st, *rec)
		rewards := Rewards(ctxA, rec.Input)
		for _, rw := range rewards {
			if !rw.Decision {
				continue
			}
			answer := rec.Work.Answers["reward:"+rw.Key]
			if answer == "" {
				return Refuse("Choose %s first.", rw.Name)
			}
			item, unit, _ := strings.Cut(answer, "|")
			uid, _ := strconv.ParseInt(unit, 10, 64)
			if err := checkFreeEquipment(e.rs, e.settings, *st, rw.MaxValue, item, uid); err != nil {
				return Refuse("%s: %s Choose again.", rw.Name, strings.TrimSuffix(err.Error(), "."))
			}
		}
		influence := 0
		for _, l := range Influence(ctxA, rec.Input) {
			influence += l.Amount
			line(rec, "influence", fmt.Sprintf("+%d Influence: %s", l.Amount, l.Text), 0, l.Amount)
		}
		for _, cas := range rec.Work.Casualties {
			if row := e.chartRow(cas); row.BonusKind == "influence" && cas.Bonus > 0 && cas.Kind != CasualtyDead && cas.Kind != CasualtyRetired {
				influence += cas.Bonus
				line(rec, "influence", fmt.Sprintf("+%d Influence: %s", cas.Bonus, row.Name), cas.UnitID, cas.Bonus)
			}
		}
		st.Influence += influence
		now := time.Now()
		for _, rw := range rewards {
			grant := Grant{CompanyID: companyID, Kind: GrantScenario, MatchID: matchID, UnitID: rw.UnitID, Note: rw.Name, On: now}
			switch rw.Kind {
			case "influence": // already in the sum above
				grant.Influence = rw.Amount
			case "experience": // given with the experience step
				grant.Experience = rw.Amount
			case "freeEquipment":
				item, unit, _ := strings.Cut(rec.Work.Answers["reward:"+rw.Key], "|")
				uid, _ := strconv.ParseInt(unit, 10, 64)
				us, _ := stateUnit(st, uid)
				us.Bought = append(us.Bought, item)
				line(rec, "reward", fmt.Sprintf("%s: %s takes %s for free", rw.Name, mustUnit(e.detail(*st).Roster, uid).Title(), item), uid, 0)
				continue
			default:
				rec.NewPerks = append(rec.NewPerks, Perk{MatchID: matchID, UnitID: rw.UnitID, Effect: rw.Key, Name: rw.Name, Text: rw.Text, When: rw.When})
				line(rec, "perk", rw.Name+": "+rw.Text, rw.UnitID, 0)
				continue
			}
			rec.NewGrants = append(rec.NewGrants, grant)
			line(rec, "reward", rw.Name, rw.UnitID, rw.Amount)
		}
		consumeItems(e, st, rec)
		st.Rerolls = 0
		rec.Work.Rerolls = 0
		rec.Status = AftermathDone
		rec.FinishedAt = &now
		return nil
	})
}

// WaiveAftermath is for the organiser and administrators: no effects, unlocks the company, records the note (1-200).
func (s *AftermathService) WaiveAftermath(ctx context.Context, user User, id, matchID, companyID int64, note string) (Aftermath, error) {
	return s.write(ctx, user, id, matchID, companyID, false, func(e *amEnv, st *CompanyState, rec *AftermathRecord) error {
		if !CanWaive(user, e.organiser) {
			return Refuse("Only the organiser of the campaign or an administrator can waive an aftermath.")
		}
		note = strings.TrimSpace(note)
		if n := utf8.RuneCountInString(note); n == 0 || n > 200 {
			return invalidAftermath("the note must be between 1 and 200 characters")
		}
		if !rec.Status.Locks() {
			return Refuse("This aftermath is already %s.", rec.Status)
		}
		if rec.Status.Started() {
			st.Rerolls = 0 // the credited re-rolls expire with it
		}
		rec.Status, rec.Work.WaivedNote = AftermathWaived, note
		now := time.Now()
		rec.FinishedAt = &now
		line(rec, "note", fmt.Sprintf("Waived by %s: %s", user.DisplayName, note), 0, 0)
		return nil
	})
}
