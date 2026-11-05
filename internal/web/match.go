package web

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The match page (docs/matches.md). Every card lives inside #match, and every write answers with the whole of it again, so
// the header badge, the scenario card and the result always agree with each other. Only the current phase is open
// (docs/matches.md, "The match page"): Preparing (no live entry yet), Playing, Ending (an end is pending) and Ended.

// The phases of the match page.
const (
	phasePreparing = "preparing"
	phasePlaying   = "playing"
	phaseEnding    = "ending"
	phaseEnded     = "ended"
)

// scenarioGroup is one table of the scenario picker.
type scenarioGroup struct {
	Title     string
	Scenarios []core.ScenarioInfo
}

// SamePlayerNotes are the "Same player" notes of the match page (docs/matches.md, "Several companies of one player"): a player who owns two or more of its
// companies. Never a refusal; the match counts like any other. Empty when no player has several.
func (v matchView) SamePlayerNotes() []string {
	var notes []string
	for _, sp := range core.SamePlayers(v.Match.Sides) {
		switch {
		case sp.AcrossSides && sp.Companies == 2:
			notes = append(notes, fmt.Sprintf("Same player: %s plays both sides.", sp.Owner))
		case sp.AcrossSides:
			notes = append(notes, fmt.Sprintf("Same player: %s has %d companies in this match, on more than one side.", sp.Owner, sp.Companies))
		case sp.Companies == 2:
			notes = append(notes, fmt.Sprintf("Same player: both of these companies are %s's.", sp.Owner))
		default:
			notes = append(notes, fmt.Sprintf("Same player: all %d of these companies are %s's.", sp.Companies, sp.Owner))
		}
	}
	return notes
}

type matchView struct {
	campaignBase
	Match    core.Match
	Draft    core.Match       // the match with the result the form shows: the saved one, or the one a failed save posted
	Access   core.MatchAccess // what the viewer may do; the page only uses it to hide controls
	Info     *core.ScenarioInfo
	Combined int // the rating of the companies in the match added, for the scenario tables that look it up

	Phase   string                   // preparing, playing, ending or ended
	Mine    []core.CampaignCompany   // the viewer's companies in the match ("Go to my company")
	Breaks  map[int64]core.LiveBreak // the Break Point of every company while the match is played (the scoreboard marks the Broken ones)
	Started bool                     // a live entry exists: the scenario and Against the Odds are fixed

	Cards *matchCards // the lazy cards drawn inline on the full page; nil on answers to writes
	// LiveSince is the state of the open match (core.LiveGeneration) that the cards were built from; the page sends it to its event
	// stream, which then skips the initial refetch of the cards. Empty when unknown: the stream starts with a full sync.
	LiveSince string

	Notice    string // shown on top of the match
	NoticeErr bool

	Mode        string // the scenario card: "" shows the scenario, "pick" the picker, "custom" the one-off form
	Groups      []scenarioGroup
	ScenarioIn  core.ScenarioInput // what the one-off form shows
	ScenarioErr map[string]string  // error text by field of the one-off form (name, description, victory); "" for the form
	ResultErr   string
	// AftermathApplied: a company started its aftermath or had it waived, so the match cannot be reopened and an
	// administrator's change to the result does not redo what the aftermath did.
	AftermathApplied bool
	ATO              core.ATOStatus // where the Against the Odds decision stands (open matches)
	EndReason        string         // why the match cannot be ended yet; empty when it can
	End              core.EndAccess // what the viewer may do about the pending or proposed end
	OverrideNote     string         // the override dialog's note after a refusal; the dialog then stays open
	OverrideErr      string
	Roll             *scenarioRoll // the dice of the last Roll again, with the Vantage Point offers; nil otherwise

	Aftermath    []aftermathRow // the Aftermath card of an ended match, one row per company
	AftermathErr string         // why the card could not be loaded
}

// scenarioRoll is the dice of Roll again on the match page and the scenario they set.
type scenarioRoll struct {
	First, Second int
	Error         string
	Line          string          // "3, Vantage Point +1 = 4" after the perk moved the second die
	Vantage       []vantageOption // companies of the viewer in this match with an unused Vantage Point (offered after a roll)
	Sides         int             // the size of the dice
	Table         int
}

// vantageOption is a company whose Vantage Point the viewer may use on the roll.
type vantageOption struct {
	CompanyID int64
	Name      string
}

// CanEdit is whether the viewer may change an open match; AdminOnEnded whether they are an administrator looking at an
// ended one (the form is then behind a button).
func (v matchView) CanEdit() bool      { return v.Access == core.MatchEdit }
func (v matchView) AdminOnEnded() bool { return v.Access == core.MatchEditEnded }

// ScenarioOpen is whether the scenario may still change (Change scenario is offered): an editor, before the game starts, or at any time
// while an open match has none (an older match that started without one could not end otherwise).
func (v matchView) ScenarioOpen() bool {
	return v.CanEdit() && !v.Match.Ended && (!v.Started || v.Match.Scenario == nil) && v.ATO.Ambush == ""
}

// Broken says whether the company is Broken now (the scoreboard while the match is played).
func (v matchView) Broken(companyID int64) *core.LiveBreak {
	if b, ok := v.Breaks[companyID]; ok && b.Broken && !v.Match.Ended {
		return &b
	}
	return nil
}

// ProposedResult says what the pending end reports: "Rohan won" or "Draw".
func (v matchView) ProposedResult() string { return proposedResult(v.Match) }

// WinnerValue is what the winner buttons start on: "" not reported, "draw", or the 1-based side.
func (v matchView) WinnerValue() string { return winnerValue(v.Draft.Result) }

func winnerValue(r core.MatchResult) string {
	switch {
	case !r.Decided:
		return ""
	case r.Winner == 0:
		return "draw"
	default:
		return strconv.Itoa(r.Winner)
	}
}

// Objectives are the ticks of the scenario, per company.
func (v matchView) Objectives() []core.ScenarioObjective {
	if v.Info == nil {
		return nil
	}
	return v.Info.Objectives
}

// Editor is whether the viewer may change the match (an administrator on an ended one included).
func (v matchView) Editor() bool { return v.Access != core.MatchNoAccess }

// matchCtx loads the campaign and the match of the request; it answers 404 itself when either is unknown.
func (s *Server) matchCtx(w http.ResponseWriter, r *http.Request) (campaignBase, core.Match, bool) {
	base, ok := s.loadCampaign(w, r, "matches")
	if !ok {
		return base, core.Match{}, false
	}
	mid, ok := s.idParam(w, r, "match")
	if !ok {
		return base, core.Match{}, false
	}
	m, err := s.campaigns.Match(r.Context(), base.Campaign.ID, mid)
	if s.fail(w, r, err) {
		return base, core.Match{}, false
	}
	return base, m, true
}

// noAccessReason is what a viewer who may not change the match is told: a finished campaign first, then the match's own reason.
func noAccessReason(base campaignBase, m core.Match) string {
	if base.Campaign.Status == core.CampaignFinished {
		return "The campaign is finished."
	}
	return m.NoAccessReason()
}

// matchAccess is the viewer's access to the match; a finished campaign is read only for everybody.
func matchAccess(base campaignBase, m core.Match, u core.User) core.MatchAccess {
	if base.Campaign.Status == core.CampaignFinished {
		return core.MatchNoAccess
	}
	return m.Access(u, base.Campaign.OrganiserID)
}

// newMatchView gathers what the cards need; the scenario list is read only when a card shows it.
func (s *Server) newMatchView(r *http.Request, base campaignBase, m core.Match, mode string) (matchView, error) {
	v := matchView{campaignBase: base, Match: m, Draft: m, Mode: mode, Access: matchAccess(base, m, *userFrom(r.Context()))}
	for _, c := range m.AllCompanies() {
		if base.Campaign.Owns(c.ID) {
			v.Mine = append(v.Mine, c)
		}
	}
	if m.Ended {
		v.Aftermath, v.AftermathErr = s.aftermathRows(r, base, m)
		for _, row := range v.Aftermath {
			v.AftermathApplied = v.AftermathApplied || (!m.Practice && (row.Ref.Status.Started() || row.Ref.Status == core.AftermathWaived))
		}
	} else {
		started, err := s.campaigns.MatchStarted(r.Context(), base.Campaign.ID, m.ID)
		if err != nil {
			return v, err
		}
		v.Started = started
		if started {
			if v.Breaks, err = s.campaigns.MatchBreaks(r.Context(), base.Campaign.ID, m.ID); err != nil {
				return v, err
			}
		}
	}
	switch {
	case m.Ended:
		v.Phase = phaseEnded
	case m.EndProposal != nil:
		v.Phase = phaseEnding
	case v.Started:
		v.Phase = phasePlaying
	default:
		v.Phase = phasePreparing
	}
	if !m.Ended {
		st, err := s.campaigns.ATOStatus(r.Context(), base.Campaign.ID, m)
		if err != nil {
			return v, err
		}
		v.ATO = st
	}
	switch {
	case m.CanEnd() != nil && m.Scenario == nil:
		v.EndReason = "Choose the scenario before ending the match."
	case v.ATO.Pending:
		v.EndReason = v.ATO.Why()
	}
	v.End = endAccess(r, base, m)
	if mode == "custom" && m.Scenario != nil && m.Scenario.Kind == core.ScenarioCustom {
		v.ScenarioIn = *m.Scenario
	}
	rulebook := m.Scenario != nil && m.Scenario.Kind == core.ScenarioRulebook
	if rulebook || mode == "pick" {
		list, err := s.campaigns.Scenarios(r.Context(), base.Campaign.ID)
		if err != nil {
			return v, err
		}
		if rulebook {
			if v.Info = core.FindScenario(list, m.Scenario.Name); v.Info != nil {
				for _, c := range m.AllCompanies() {
					v.Combined += c.Points
				}
			}
		}
		if mode == "pick" {
			v.Groups = groupScenarios(list)
		}
	}
	return v, nil
}

// groupScenarios splits the rules file's scenarios by table, in the order the file gives them.
func groupScenarios(list []core.ScenarioInfo) []scenarioGroup {
	title := func(sc core.ScenarioInfo) string {
		if sc.Table == 0 {
			return "Other scenarios"
		}
		return fmt.Sprintf("Table %d (first die %s)", sc.Table, rangeText(sc.FirstDie[0], sc.FirstDie[1]))
	}
	var out []scenarioGroup
	for _, sc := range list {
		i := slices.IndexFunc(out, func(g scenarioGroup) bool { return g.Title == title(sc) })
		if i < 0 {
			out = append(out, scenarioGroup{Title: title(sc)})
			i = len(out) - 1
		}
		out[i].Scenarios = append(out[i].Scenarios, sc)
	}
	return out
}

// rangeText writes a range of rolls: "3", "1-2".
func rangeText(lo, hi int) string {
	if lo == hi {
		return strconv.Itoa(lo)
	}
	return fmt.Sprintf("%d-%d", lo, hi)
}

func (s *Server) handleMatch(w http.ResponseWriter, r *http.Request) {
	base, m, ok := s.matchCtx(w, r)
	if !ok {
		return
	}
	v, err := s.newMatchView(r, base, m, "")
	if s.fail(w, r, err) {
		return
	}
	if !m.Ended && v.Campaign.Status != core.CampaignFinished { // read before the cards are built (they share the read scope's snapshot)
		if gen, err := s.campaigns.LiveGeneration(r.Context(), *userFrom(r.Context()), base.Campaign.ID, m.ID); err == nil {
			v.LiveSince = gen
		}
	}
	s.loadMatchCards(r, &v)
	s.render.page(w, r, http.StatusOK, "match", v)
}

// answer renders the whole match again after a write (or a refused one) with a notice on top.
func (s *Server) answer(w http.ResponseWriter, r *http.Request, status int, base campaignBase, m core.Match, mode, notice string, isErr bool, edit func(*matchView)) {
	v, err := s.newMatchView(r, base, m, mode)
	if s.fail(w, r, err) {
		return
	}
	v.Notice, v.NoticeErr = notice, isErr
	if edit != nil {
		edit(&v)
	}
	s.render.partial(w, status, "match", "match_body", v)
}

// writeFailed answers an error of a write. ok is true when it was one of the expected ones and has been answered. A viewer
// who cannot edit gets a plain 403; a stale form gets the current match with a notice; anything else the backend refused
// (the match cannot be ended yet) or found invalid is shown on the card again.
func (s *Server) writeFailed(w http.ResponseWriter, r *http.Request, base campaignBase, m core.Match, mode string, err error, edit func(*matchView)) bool {
	switch {
	case errors.Is(err, core.ErrMatchChanged):
		cur, merr := s.campaigns.Match(r.Context(), base.Campaign.ID, m.ID)
		if s.fail(w, r, merr) {
			return true
		}
		s.answer(w, r, http.StatusOK, base, cur, "", "Somebody changed this match while you were editing, so what you see now is its current state. Check it and make your change again.", true, nil)
	case errors.Is(err, core.ErrRefused):
		if matchAccess(base, m, *userFrom(r.Context())) == core.MatchNoAccess {
			s.refuse(w, r, http.StatusForbidden, reason(err))
			return true
		}
		s.answer(w, r, http.StatusUnprocessableEntity, base, m, mode, reason(err), true, edit)
	case errors.Is(err, core.ErrInvalidMatch):
		s.answer(w, r, http.StatusUnprocessableEntity, base, m, mode, "", false, edit)
	default:
		return s.fail(w, r, err)
	}
	return true
}

// revisionOf reads the revision the form was rendered with.
func revisionOf(r *http.Request) int {
	n, _ := strconv.Atoi(r.PostFormValue("revision"))
	return n
}

// handleScenarioCard answers the scenario card in the mode asked for (?mode=pick or custom; none shows it plainly again).
func (s *Server) handleScenarioCard(w http.ResponseWriter, r *http.Request) {
	base, m, ok := s.matchCtx(w, r)
	if !ok {
		return
	}
	mode := r.URL.Query().Get("mode")
	if mode != "" && mode != "pick" && mode != "custom" {
		s.notFound(w, r)
		return
	}
	if mode != "" && matchAccess(base, m, *userFrom(r.Context())) == core.MatchNoAccess {
		s.refuse(w, r, http.StatusForbidden, noAccessReason(base, m))
		return
	}
	v, err := s.newMatchView(r, base, m, mode)
	if s.fail(w, r, err) {
		return
	}
	s.render.partial(w, http.StatusOK, "match", "scenario_card", v)
}

// handleSetScenario chooses a rulebook scenario (kind=rulebook, name) or saves the one-off form (kind=custom).
func (s *Server) handleSetScenario(w http.ResponseWriter, r *http.Request) {
	base, m, ok := s.matchCtx(w, r)
	if !ok {
		return
	}
	in := core.ScenarioInput{Kind: core.ScenarioKind(r.PostFormValue("kind")), Name: r.PostFormValue("name"),
		Description: r.PostFormValue("description"), Victory: r.PostFormValue("victory")}
	mode := "pick"
	if in.Kind == core.ScenarioCustom {
		mode = "custom"
	}
	updated, err := s.campaigns.SetScenario(r.Context(), *userFrom(r.Context()), base.Campaign.ID, m.ID, revisionOf(r), in)
	if err != nil {
		s.writeFailed(w, r, base, m, mode, err, func(v *matchView) {
			v.ScenarioIn, v.ScenarioErr = in, scenarioErrors(err)
		})
		return
	}
	s.answer(w, r, http.StatusOK, base, updated, "", atoReopened(m, updated), false, nil)
}

// atoReopened is the notice of a change of the scenario that took back a confirmed Against the Odds decision.
func atoReopened(before, after core.Match) string {
	if before.ATODecided && !after.ATODecided {
		return "The scenario changed: Against the Odds has to be confirmed again."
	}
	return ""
}

func (s *Server) handleClearScenario(w http.ResponseWriter, r *http.Request) {
	base, m, ok := s.matchCtx(w, r)
	if !ok {
		return
	}
	updated, err := s.campaigns.ClearScenario(r.Context(), *userFrom(r.Context()), base.Campaign.ID, m.ID, revisionOf(r))
	if err != nil {
		s.writeFailed(w, r, base, m, "", err, nil)
		return
	}
	s.answer(w, r, http.StatusOK, base, updated, "", atoReopened(m, updated), false, nil)
}

// reasons are the messages of an error built with errors.Join(sentinel, reason...), without the sentinel.
func reasons(err error) []string {
	var out []string
	for _, fr := range fieldReasons(err) {
		out = append(out, fr.Msg)
	}
	return out
}

// fieldReasons are the reasons of an error built with errors.Join(sentinel, reason...), each with the form field it is about.
func fieldReasons(err error) []core.FieldError {
	var out []core.FieldError
	u, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return []core.FieldError{{Field: core.FieldOf(err), Msg: sentence(err.Error())}}
	}
	for _, e := range u.Unwrap()[1:] {
		out = append(out, core.FieldError{Field: core.FieldOf(e), Msg: sentence(e.Error())})
	}
	return out
}

// scenarioErrors files the messages of ScenarioInput.Validate under the field they are about.
func scenarioErrors(err error) map[string]string {
	out := map[string]string{}
	for _, fr := range fieldReasons(err) {
		field, msg := fr.Field, fr.Msg
		if out[field] != "" {
			msg = out[field] + " " + msg
		}
		out[field] = msg
	}
	return out
}

// winnerFromForm reads the winner buttons: "draw" or a 1-based side. ok is false when nothing (or something unknown) was chosen.
func winnerFromForm(r *http.Request, m core.Match) (decided bool, winner int, ok bool) {
	switch w := r.PostFormValue("winner"); w {
	case "":
		return false, 0, false
	case "draw":
		return true, 0, true
	default:
		n, err := strconv.Atoi(w)
		if err != nil || n < 1 || n > len(m.Sides) {
			return false, 0, false
		}
		return true, n, true
	}
}

// ticksFromForm reads the scenario reward ticks of the companies (tick-{company}-{key}; a number for an objective counted more than
// once) into the result, replacing what was reported for those companies only.
func ticksFromForm(r *http.Request, res core.MatchResult, companies []core.CampaignCompany, objectives []core.ScenarioObjective) (core.MatchResult, []string) {
	var problems []string
	res.Companies = slices.Clone(res.Companies)
	for _, c := range companies {
		cr := core.CompanyResult{CompanyID: c.ID}
		for _, o := range objectives {
			v := strings.TrimSpace(r.PostFormValue("tick-" + itoa(int(c.ID)) + "-" + o.Key))
			if v == "" {
				continue
			}
			n, err := strconv.Atoi(v)
			if err != nil {
				problems = append(problems, o.Label+" must be a whole number")
				continue
			}
			if n != 0 {
				if cr.Objectives == nil {
					cr.Objectives = map[string]int{}
				}
				cr.Objectives[o.Key] = n
			}
		}
		res.Companies = slices.DeleteFunc(res.Companies, func(x core.CompanyResult) bool { return x.CompanyID == c.ID })
		if len(cr.Objectives) > 0 {
			res.Companies = append(res.Companies, cr)
		}
	}
	return res, problems
}

// scenarioObjectives are the reward ticks of the match's rulebook scenario (none for a one-off).
func (s *Server) scenarioObjectives(r *http.Request, campaignID int64, m core.Match) ([]core.ScenarioObjective, error) {
	if m.Scenario == nil || m.Scenario.Kind != core.ScenarioRulebook {
		return nil, nil
	}
	list, err := s.campaigns.Scenarios(r.Context(), campaignID)
	if err != nil {
		return nil, err
	}
	if sc := core.FindScenario(list, m.Scenario.Name); sc != nil {
		return sc.Objectives, nil
	}
	return nil, nil
}

// handleSaveResult is the result form of an ended match, for the organiser or an administrator who corrects it: the winner, the reward
// ticks and the note. An open match reports its result in the End match sheet.
func (s *Server) handleSaveResult(w http.ResponseWriter, r *http.Request) {
	base, m, ok := s.matchCtx(w, r)
	if !ok {
		return
	}
	if matchAccess(base, m, *userFrom(r.Context())) == core.MatchNoAccess {
		s.refuse(w, r, http.StatusForbidden, noAccessReason(base, m))
		return
	}
	objectives, err := s.scenarioObjectives(r, base.Campaign.ID, m)
	if s.fail(w, r, err) {
		return
	}
	res := core.MatchResult{Note: r.PostFormValue("note"), Companies: m.Result.Companies}
	var problems []string
	var chosen bool
	if res.Decided, res.Winner, chosen = winnerFromForm(r, m); !chosen {
		problems = append(problems, "Choose who won, or a draw")
	}
	res, more := ticksFromForm(r, res, m.AllCompanies(), objectives)
	problems = append(problems, more...)
	show := func(msg string) func(*matchView) {
		return func(v *matchView) {
			v.Draft.Result, v.ResultErr = res, msg
		}
	}
	if len(problems) > 0 {
		s.answer(w, r, http.StatusUnprocessableEntity, base, m, "", "", false, show(strings.Join(problems, ". ")))
		return
	}
	updated, err := s.campaigns.SaveResult(r.Context(), *userFrom(r.Context()), base.Campaign.ID, m.ID, revisionOf(r), res)
	if err != nil {
		if errors.Is(err, core.ErrInvalidMatch) {
			s.writeFailed(w, r, base, m, "", err, show(strings.Join(reasons(err), ". ")))
			return
		}
		s.writeFailed(w, r, base, m, "", err, show(reason(err)))
		return
	}
	s.answer(w, r, http.StatusOK, base, updated, "", "Result saved.", false, nil)
}

// endAccess is what the viewer may do about the end of the match; a finished campaign allows nothing.
func endAccess(r *http.Request, base campaignBase, m core.Match) core.EndAccess {
	if base.Campaign.Status == core.CampaignFinished {
		return core.EndAccess{Reason: "The campaign is finished."}
	}
	return core.EndAccessFor(*userFrom(r.Context()), m, m.AllCompanies(), base.Campaign.OrganiserID)
}

// handleEndCard answers the pending-end card (and the End match button, out of band) for the live refresh of the match page.
func (s *Server) handleEndCard(w http.ResponseWriter, r *http.Request) {
	base, m, ok := s.matchCtx(w, r)
	if !ok {
		return
	}
	v, err := s.newMatchView(r, base, m, "")
	if s.fail(w, r, err) {
		return
	}
	s.render.partial(w, http.StatusOK, "match", "end_card_slot", v)
}

// The answers to a pending end of a match (docs/matches.md): withdraw one's companies, cancel it ("Keep playing") and the organiser's
// override. Propose and Confirm come from the End match and Confirm sheets (end_sheet.go).
type endStep int

const (
	endWithdraw endStep = iota
	endCancel
	endOverride
)

func (s *Server) handleEnd(step endStep) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		base, m, ok := s.matchCtx(w, r)
		if !ok {
			return
		}
		ctx, user, id, rev := r.Context(), *userFrom(r.Context()), base.Campaign.ID, revisionOf(r)
		banner := r.PostFormValue("view") == "banner" // asked from the live banner of a company page: answer the banner
		company := formID(r, "company")
		answer := func(status int, cur core.Match, notice string, isErr bool, note, noteErr string) {
			if banner {
				s.answerEndBanner(w, r, status, base, cur, company, notice, isErr, note, noteErr)
				return
			}
			s.answer(w, r, status, base, cur, "", notice, isErr, func(v *matchView) { v.OverrideNote, v.OverrideErr = note, noteErr })
		}
		var (
			updated core.Match
			err     error
			notice  string
		)
		switch step {
		case endWithdraw:
			updated, err = s.campaigns.WithdrawEnd(ctx, user, id, m.ID, rev)
			notice = "You took back your confirmation."
		case endCancel:
			updated, err = s.campaigns.CancelEnd(ctx, user, id, m.ID, rev)
			notice = "Ending cancelled: the match goes on."
		case endOverride:
			note := strings.TrimSpace(r.PostFormValue("note"))
			updated, err = s.campaigns.OverrideEnd(ctx, user, id, m.ID, rev, note)
			if errors.Is(err, core.ErrInvalidMatch) { // the note is missing or too long: the dialog stays open
				answer(http.StatusUnprocessableEntity, m, "", false, note, reason(err))
				return
			}
			notice = "The match ended without the confirmation of the players still waiting."
		}
		if errors.Is(err, core.ErrInvalidMatch) {
			answer(http.StatusUnprocessableEntity, m, strings.Join(reasons(err), ". "), true, "", "")
			return
		}
		if banner && err != nil {
			s.bannerFailed(w, r, base, m, company, err)
			return
		}
		if err != nil {
			s.writeFailed(w, r, base, m, "", err, nil)
			return
		}
		answer(http.StatusOK, updated, notice, false, "", "")
	}
}

// rollScenario rolls both dice of the selection (always the server's dice) until they point at a scenario of the list, a few tries at
// most: a pair of dice the file places no scenario on is rolled again. found is false after the last try, whose dice it gives.
func (s *Server) rollScenario(sel core.ScenarioSelection, list []core.ScenarioInfo) (sc core.ScenarioInfo, first, second int, found bool) {
	for range 10 {
		first, second = s.die(sel.Die.Sides), s.die(sel.Die.Sides)
		if sc, found = core.ScenarioByRolls(sel, list, first, second); found {
			break
		}
	}
	return sc, first, second, found
}

// handleScenarioRoll is Roll again on the match page (docs/matches.md, "New match"): the server rolls both dice of the scenario
// selection and the scenario they point at is set at once. With vantage (a company of the viewer) it is the Vantage Point perk on the
// roll on show instead: the second die moves by adjust (+1 or -1) and the scenario follows.
func (s *Server) handleScenarioRoll(w http.ResponseWriter, r *http.Request) {
	base, m, ok := s.matchCtx(w, r)
	if !ok {
		return
	}
	user := *userFrom(r.Context())
	if matchAccess(base, m, user) == core.MatchNoAccess {
		s.refuse(w, r, http.StatusForbidden, noAccessReason(base, m))
		return
	}
	ctx, id := r.Context(), base.Campaign.ID
	list, err := s.campaigns.Scenarios(ctx, id)
	if s.fail(w, r, err) {
		return
	}
	sel, err := s.campaigns.ScenarioSelection(ctx, id)
	if s.fail(w, r, err) {
		return
	}
	roll := &scenarioRoll{Sides: sel.Die.Sides}
	keep := func(v *matchView) { v.Roll = roll }
	var sc core.ScenarioInfo
	var found bool
	if vantage := formID(r, "vantage"); vantage != 0 {
		d1, d2 := formInt(r, "roll"), formInt(r, "roll2")
		if !sel.Valid(d1) || !sel.Valid(d2) {
			s.answer(w, r, http.StatusUnprocessableEntity, base, m, "", fmt.Sprintf("A die shows 1 to %d.", sel.Die.Sides), true, nil)
			return
		}
		moved, line, err := s.campaigns.UseVantagePoint(ctx, user, id, m.ID, vantage, d2, formInt(r, "adjust"))
		if errors.Is(err, core.ErrRefused) || errors.Is(err, core.ErrInvalidRoll) {
			roll.First, roll.Second = d1, d2
			s.answer(w, r, http.StatusUnprocessableEntity, base, m, "", reason(err), true, keep)
			return
		}
		if s.fail(w, r, err) {
			return
		}
		roll.First, roll.Second, roll.Line = d1, moved, line
		sc, found = core.ScenarioByRolls(sel, list, d1, moved)
	} else {
		if sel.Die.Sides <= 0 {
			s.answer(w, r, http.StatusUnprocessableEntity, base, m, "", "The rules of this campaign have no scenario dice.", true, nil)
			return
		}
		sc, roll.First, roll.Second, found = s.rollScenario(sel, list)
	}
	roll.Table = sel.Table(roll.First)
	if !found {
		s.answer(w, r, http.StatusUnprocessableEntity, base, m, "", "The rules of this campaign place no scenario on that roll: choose one.", true, keep)
		return
	}
	updated, err := s.campaigns.SetScenario(ctx, user, id, m.ID, revisionOf(r), core.ScenarioInput{Kind: core.ScenarioRulebook, Name: sc.Name})
	if err != nil {
		s.writeFailed(w, r, base, m, "", err, nil)
		return
	}
	for _, co := range updated.AllCompanies() {
		if co.OwnerID != user.ID || m.Ended {
			continue
		}
		if d, err := s.campaigns.CompanyDetail(ctx, id, co.ID); err == nil {
			if _, ok := core.UnusedPerk(d.Perks, core.PerkVantagePoint); ok {
				roll.Vantage = append(roll.Vantage, vantageOption{co.ID, co.Name})
			}
		}
	}
	s.answer(w, r, http.StatusOK, base, updated, "", atoReopened(m, updated), false, keep)
}

// handleReopenMatch is the way back for the organiser and administrators into an ended match; everybody else gets a 403. The reopening is
// recorded in the match's history.
func (s *Server) handleReopenMatch(w http.ResponseWriter, r *http.Request) {
	base, m, ok := s.matchCtx(w, r)
	if !ok {
		return
	}
	if u := userFrom(r.Context()); !core.CanManageCampaign(*u, base.Campaign.OrganiserID) {
		s.refuse(w, r, http.StatusForbidden, "Only the organiser of the campaign or an administrator can reopen a match.")
		return
	}
	updated, err := s.campaigns.ReopenMatch(r.Context(), *userFrom(r.Context()), base.Campaign.ID, m.ID, revisionOf(r))
	if err != nil {
		s.writeFailed(w, r, base, m, "", err, nil)
		return
	}
	s.answer(w, r, http.StatusOK, base, updated, "", "Match reopened.", false, nil)
}

// InProgress is whether the game is being played (or its end is pending): the scenario and the log stay open.
func (v matchView) InProgress() bool { return v.Phase == phasePlaying || v.Phase == phaseEnding }
