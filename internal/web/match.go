package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The match page (docs/matches.md). Every card lives inside #match, and every write answers with the whole of it again, so
// the header badge, the scenario card and the result form always agree with each other.

// scenarioGroup is one table of the scenario picker.
type scenarioGroup struct {
	Title     string
	Scenarios []core.ScenarioInfo
}

// SamePlayerNotes are the "Same player" notes of the match page (docs/multi-company-spec.md, 4.3): a player who owns two or more of its
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
	Combined int  // the rating of the companies in the match added, for the scenario tables that look it up
	Fled     bool // somebody fled

	Cards *matchCards // the lazy cards drawn inline on the full page; nil on answers to writes

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
	EndReason        string         // why the match cannot be ended yet; empty when it can
	End              core.EndAccess // what the viewer may do about the pending or proposed end
	OverrideNote     string         // the override dialog's note after a refusal; the dialog then stays open
	OverrideErr      string
	Roll             *scenarioRoll          // the picker's "Roll for it" panel: nil until a roll is asked for
	Selection        core.ScenarioSelection // how the dice pick a scenario (set in the picker): the size of the dice and the first die's table ranges

	VPFromLive map[int]bool // sides whose Victory Points the form prefilled from the secret objectives of the live match (1-based side)
	// FledFromLive and ObjFromLive ("company/objective key") mark what the result form prefilled from the live match (36y.17).
	FledFromLive map[int64]bool
	ObjFromLive  map[string]bool
	Aftermath    []aftermathRow // the Aftermath card of an ended match, one row per company
	AftermathErr string         // why the card could not be loaded
}

// scenarioRoll is the picker's roll helper: the two dice of scenarioSelection and the scenario they point at.
type scenarioRoll struct {
	First, Second int
	Scenario      *core.ScenarioInfo // nil when the rules file places no scenario there
	Animate       bool               // the server rolled: the dice show it before the result is revealed
	Error         string
	Line          string          // "3, Vantage Point +1 = 4" after the perk moved the second die
	Vantage       []vantageOption // companies of the viewer in this match with an unused Vantage Point (offered after a roll)
}

// vantageOption is a company whose Vantage Point the viewer may use on the roll.
type vantageOption struct {
	CompanyID int64
	Name      string
}

// Dice is the dice as the roll panel's data attribute reads them; empty before a roll.
func (v matchView) Dice() string {
	if v.Roll == nil || v.Roll.Error != "" {
		return ""
	}
	return itoa(v.Roll.First) + "," + itoa(v.Roll.Second)
}

// RollAnimates is whether the roll panel shows the dice rolling before it reveals the result.
func (v matchView) RollAnimates() bool { return v.Roll != nil && v.Roll.Animate && v.Roll.Error == "" }

// RollSides describes who is the Top Dog and the Underdog by rating; empty when the match has no two sides to compare.
func (v matchView) RollSides() []rollSide {
	if len(v.Match.Sides) != 2 {
		return nil
	}
	top, under := v.Match.TopDog()
	out := make([]rollSide, 2)
	for i, sd := range v.Match.Sides {
		out[i] = rollSide{Name: sd.Label(), Rating: sd.Rating(), Top: i == top, Under: i == under}
	}
	return out
}

type rollSide struct {
	Name       string
	Rating     int
	Top, Under bool
}

// CanEdit is whether the viewer may change an open match; AdminOnEnded whether they are an administrator looking at an
// ended one (the form is then behind a button).
func (v matchView) CanEdit() bool      { return v.Access == core.MatchEdit }
func (v matchView) AdminOnEnded() bool { return v.Access == core.MatchEditEnded }

// ResultJSON feeds the result form's warning (static/js/match_result.js): who fled, and the side of each company.
func (v matchView) ResultJSON() string {
	type co struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Side int    `json:"side"`
		Fled bool   `json:"fled"`
	}
	var list []co
	for i, sd := range v.Match.Sides {
		for _, c := range sd.Companies {
			list = append(list, co{c.ID, c.Name, i + 1, v.Draft.Result.Company(c.ID).Fled})
		}
	}
	b, _ := json.Marshal(map[string]any{"companies": list, "sides": sideNames(v.Match)})
	return string(b)
}

func sideNames(m core.Match) []string {
	out := make([]string, len(m.Sides))
	for i, sd := range m.Sides {
		out[i] = sd.Label()
	}
	return out
}

// WinnerValue is what the winner radio group starts on: "" not reported, "draw", or the 1-based side.
func (v matchView) WinnerValue() string {
	switch r := v.Draft.Result; {
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

// ShowRoles is whether the Attacker/Defender tags are offered: while the scenario is not chosen, or when it has roles.
func (v matchView) ShowRoles() bool {
	switch sc := v.Match.Scenario; {
	case sc == nil:
		return true
	case sc.Kind == core.ScenarioRulebook:
		return v.Info != nil && v.Info.HasRoles()
	}
	return false
}

// AnyRole is whether a side carries a tag.
func (v matchView) AnyRole() bool {
	for _, sd := range v.Match.Sides {
		if sd.Role != "" {
			return true
		}
	}
	return false
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
	for _, c := range m.Result.Companies {
		v.Fled = v.Fled || c.Fled
	}
	if m.Ended {
		v.Aftermath, v.AftermathErr = s.aftermathRows(r, base, m)
		for _, row := range v.Aftermath {
			v.AftermathApplied = v.AftermathApplied || (!m.Practice && (row.Ref.Status.Started() || row.Ref.Status == core.AftermathWaived))
		}
	}
	if m.CanEnd() != nil {
		v.EndReason = reason(m.CanEnd())
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
			if i := slices.IndexFunc(list, func(sc core.ScenarioInfo) bool { return sc.Name == m.Scenario.Name }); i >= 0 {
				v.Info = &list[i]
				for _, side := range m.Sides {
					for _, c := range side.Companies {
						v.Combined += c.Points
					}
				}
			}
		}
		if mode == "pick" {
			if v.Selection, err = s.campaigns.ScenarioSelection(r.Context(), base.Campaign.ID); err != nil {
				return v, err
			}
			v.Groups = groupScenarios(list)
		}
	}
	if m.Ended && v.Access != core.MatchNoAccess && hasSecret(v.Info) {
		s.prefillSecretScores(r, &v, base.Campaign.ID)
	}
	if v.Access != core.MatchNoAccess {
		s.prefillLiveDefaults(r, &v)
	}
	return v, nil
}

// prefillLiveDefaults puts what the live log says on the result form for a company whose result was not reported yet: Fled when it chose
// All is Lost, and the enemyBroken objective when every opposing company is Broken at the end. Nothing is saved until the editor saves.
func (s *Server) prefillLiveDefaults(r *http.Request, v *matchView) {
	def, ok, err := s.campaigns.MatchDefaults(r.Context(), v.Campaign.ID, v.Match.ID)
	if err != nil || !ok {
		return
	}
	reported := map[int64]bool{}
	for _, c := range v.Match.Result.Companies {
		reported[c.CompanyID] = true
	}
	hasObjective := v.Info != nil && slices.ContainsFunc(v.Info.Objectives, func(o core.ScenarioObjective) bool { return o.Key == "enemyBroken" })
	companies := slices.Clone(v.Draft.Result.Companies)
	for _, side := range v.Match.Sides {
		for _, co := range side.Companies {
			if reported[co.ID] {
				continue
			}
			fled, broken := def.Fled[co.ID], hasObjective && def.EnemyBroken[co.ID]
			if !fled && !broken {
				continue
			}
			c := core.CompanyResult{CompanyID: co.ID, Fled: fled, Objectives: map[string]int{}}
			if fled {
				if v.FledFromLive == nil {
					v.FledFromLive = map[int64]bool{}
				}
				v.FledFromLive[co.ID] = true
			}
			if broken {
				c.Objectives["enemyBroken"] = 1
				if v.ObjFromLive == nil {
					v.ObjFromLive = map[string]bool{}
				}
				v.ObjFromLive[fmt.Sprintf("%d/enemyBroken", co.ID)] = true
			}
			companies = append(companies, c)
		}
	}
	v.Draft.Result.Companies = companies
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

// handleSetScenario chooses a rulebook scenario (kind=rulebook, name).
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
	s.answer(w, r, http.StatusOK, base, updated, "", "", false, nil)
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
	s.answer(w, r, http.StatusOK, base, updated, "", "", false, nil)
}

// reasons are the messages of an error built with errors.Join(sentinel, reason...), without the sentinel.
func reasons(err error) []string {
	u, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return []string{sentence(err.Error())}
	}
	var out []string
	for _, e := range u.Unwrap()[1:] {
		out = append(out, sentence(e.Error()))
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

// resultFromForm reads the result form. Problems of the text boxes are returned with the result as far as it could be read,
// so the form can show what was typed.
func resultFromForm(r *http.Request, m core.Match, objectives []core.ScenarioObjective) (core.MatchResult, []string) {
	var problems []string
	res := core.MatchResult{Sides: make([]core.SideResult, len(m.Sides)), Note: r.PostFormValue("note")}
	switch w := r.PostFormValue("winner"); w {
	case "":
	case "draw":
		res.Decided = true
	default:
		n, err := strconv.Atoi(w)
		if err != nil || n < 1 || n > len(m.Sides) {
			problems = append(problems, "Choose one of the sides, a draw, or leave the winner open")
		} else {
			res.Decided, res.Winner = true, n
		}
	}
	for i := range m.Sides {
		v := strings.TrimSpace(r.PostFormValue("vp-" + itoa(i+1)))
		if v == "" {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			problems = append(problems, "Victory Points must be a whole number")
			continue
		}
		res.Sides[i].VictoryPoints = &n
	}
	for _, sd := range m.Sides {
		for _, c := range sd.Companies {
			cr := core.CompanyResult{CompanyID: c.ID, Fled: r.PostFormValue("fled-"+itoa(int(c.ID))) != ""}
			for _, o := range objectives {
				v := strings.TrimSpace(r.PostFormValue("obj-" + itoa(int(c.ID)) + "-" + o.Key))
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
			if cr.Fled || len(cr.Objectives) > 0 {
				res.Companies = append(res.Companies, cr)
			}
		}
	}
	return res, problems
}

func (s *Server) handleSaveResult(w http.ResponseWriter, r *http.Request) {
	base, m, ok := s.matchCtx(w, r)
	if !ok {
		return
	}
	if matchAccess(base, m, *userFrom(r.Context())) == core.MatchNoAccess {
		s.refuse(w, r, http.StatusForbidden, noAccessReason(base, m))
		return
	}
	var objectives []core.ScenarioObjective
	if m.Scenario != nil && m.Scenario.Kind == core.ScenarioRulebook {
		list, err := s.campaigns.Scenarios(r.Context(), base.Campaign.ID)
		if s.fail(w, r, err) {
			return
		}
		for _, sc := range list {
			if sc.Name == m.Scenario.Name {
				objectives = sc.Objectives
			}
		}
	}
	res, problems := resultFromForm(r, m, objectives)
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
	var companies []core.CampaignCompany
	for _, side := range m.Sides {
		companies = append(companies, side.Companies...)
	}
	return core.EndAccessFor(*userFrom(r.Context()), m, companies, base.Campaign.OrganiserID)
}

// handleEndCard answers the pending-end card (and the header button, out of band) for the live refresh of the match page.
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

// The five answers to the end of a match (docs/matches.md): propose it, confirm or withdraw one's companies, cancel it ("Keep playing") and
// the organiser's override.
type endStep int

const (
	endPropose endStep = iota
	endConfirm
	endWithdraw
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
		case endPropose:
			updated, err = s.campaigns.ProposeEnd(ctx, user, id, m.ID, rev)
			notice = "You proposed to end the match. It ends when everyone has confirmed."
		case endConfirm:
			updated, err = s.campaigns.ConfirmEnd(ctx, user, id, m.ID, rev)
			notice = "You confirmed the end."
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
		if errors.Is(err, core.ErrInvalidMatch) { // e.g. the roles are not chosen yet
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
		if step == endPropose || step == endConfirm {
			if updated.Ended {
				notice = "Match ended."
			}
		}
		answer(http.StatusOK, updated, notice, false, "", "")
	}
}

// handleScenarioRoll rolls the two dice of the scenario selection for the picker (or takes the ones the player brings,
// as the campaign's settings allow) and answers the roll panel with the scenario they point at. It changes nothing: the
// picker only preselects the scenario and the player confirms it.
func (s *Server) handleScenarioRoll(w http.ResponseWriter, r *http.Request) {
	base, m, ok := s.matchCtx(w, r)
	if !ok {
		return
	}
	if matchAccess(base, m, *userFrom(r.Context())) == core.MatchNoAccess {
		s.refuse(w, r, http.StatusForbidden, noAccessReason(base, m))
		return
	}
	v, err := s.newMatchView(r, base, m, "pick")
	if s.fail(w, r, err) {
		return
	}
	d1, _ := strconv.Atoi(r.PostFormValue("roll"))
	d2, _ := strconv.Atoi(r.PostFormValue("roll2"))
	thrown := r.PostFormValue("thrown") != ""
	vantage := formID(r, "vantage") // a Vantage Point adjustment of the roll on show: the dice are the ones already rolled
	roll := &scenarioRoll{}
	v.Roll = roll
	status := http.StatusOK
	user := *userFrom(r.Context())
	var list []core.ScenarioInfo
	for _, g := range v.Groups {
		list = append(list, g.Scenarios...)
	}
	sel := v.Selection
	dieErr := fmt.Sprintf("A die shows 1 to %d.", sel.Die.Sides)
	switch msg := base.Campaign.Settings.RollRefusal(d1 > 0 || d2 > 0, thrown); {
	case vantage != 0:
		if !sel.Valid(d1) || !sel.Valid(d2) {
			roll.Error, status = dieErr, http.StatusUnprocessableEntity
			break
		}
		moved, line, err := s.campaigns.UseVantagePoint(r.Context(), user, base.Campaign.ID, m.ID, vantage, d2, formInt(r, "adjust"))
		if err != nil {
			if errors.Is(err, core.ErrRefused) || errors.Is(err, core.ErrInvalidRoll) {
				roll.Error, status = reason(err), http.StatusUnprocessableEntity
				roll.First, roll.Second = d1, d2
				break
			}
			s.fail(w, r, err)
			return
		}
		roll.First, roll.Second, roll.Line = d1, moved, line
		if sc, found := core.ScenarioByRolls(sel, list, d1, moved); found {
			roll.Scenario = &sc
		}
	case msg != "":
		roll.Error, status = msg, http.StatusUnprocessableEntity
	case d1 < 0 || d1 > sel.Die.Sides || d2 < 0 || d2 > sel.Die.Sides:
		roll.Error, status = dieErr, http.StatusUnprocessableEntity
	default:
		roll.Animate = d1 == 0 || d2 == 0 // a die the server rolls is shown rolling; the ones the player brings are not
		if d1 == 0 {
			d1 = s.die(sel.Die.Sides)
		}
		if d2 == 0 {
			d2 = s.die(sel.Die.Sides)
		}
		roll.First, roll.Second = d1, d2
		if sc, found := core.ScenarioByRolls(sel, list, d1, d2); found {
			roll.Scenario = &sc
		}
	}
	if roll.First > 0 && m.Ended == false {
		for _, side := range m.Sides {
			for _, co := range side.Companies {
				if co.OwnerID != user.ID {
					continue
				}
				if d, err := s.campaigns.CompanyDetail(r.Context(), base.Campaign.ID, co.ID); err == nil {
					if _, ok := core.UnusedPerk(d.Perks, core.PerkVantagePoint); ok {
						roll.Vantage = append(roll.Vantage, vantageOption{co.ID, co.Name})
					}
				}
			}
		}
	}
	s.render.partial(w, status, "match", "scenario_roll", v)
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

// handleSetRoles saves the Attacker/Defender tag of every side (role-1, role-2, ...).
func (s *Server) handleSetRoles(w http.ResponseWriter, r *http.Request) {
	base, m, ok := s.matchCtx(w, r)
	if !ok {
		return
	}
	roles := make([]string, len(m.Sides))
	for i := range roles {
		roles[i] = r.PostFormValue("role-" + itoa(i+1))
	}
	updated, err := s.campaigns.SetSideRoles(r.Context(), *userFrom(r.Context()), base.Campaign.ID, m.ID, revisionOf(r), roles)
	if errors.Is(err, core.ErrInvalidMatch) {
		s.answer(w, r, http.StatusUnprocessableEntity, base, m, "", strings.Join(reasons(err), ". "), true, nil)
		return
	}
	if err != nil {
		s.writeFailed(w, r, base, m, "", err, nil)
		return
	}
	s.answer(w, r, http.StatusOK, base, updated, "", "Roles saved.", false, nil)
}

// handleSwapRoles swaps Attacker and Defender between the two teams.
func (s *Server) handleSwapRoles(w http.ResponseWriter, r *http.Request) {
	base, m, ok := s.matchCtx(w, r)
	if !ok {
		return
	}
	updated, err := s.campaigns.SwapSideRoles(r.Context(), *userFrom(r.Context()), base.Campaign.ID, m.ID, revisionOf(r))
	if err != nil {
		s.writeFailed(w, r, base, m, "", err, nil)
		return
	}
	s.answer(w, r, http.StatusOK, base, updated, "", "Roles swapped.", false, nil)
}

// prefillSecretScores puts the Victory Points the revealed secret objectives of a side add up to on the result form where none were
// reported yet (the badge "From the live match" says so; nothing is saved until the editor saves).
func (s *Server) prefillSecretScores(r *http.Request, v *matchView, id int64) {
	user := *userFrom(r.Context())
	sides := slices.Clone(v.Draft.Result.Sides)
	for len(sides) < len(v.Match.Sides) {
		sides = append(sides, core.SideResult{})
	}
	for i, side := range v.Match.Sides {
		if sides[i].VictoryPoints != nil {
			continue
		}
		total, any := 0, false
		for _, co := range side.Companies {
			if pts, _, ok, err := s.campaigns.SecretScore(r.Context(), user, id, v.Match.ID, co.ID); err == nil && ok {
				total, any = total+pts, true
			}
		}
		if any {
			sides[i].VictoryPoints = &total
			if v.VPFromLive == nil {
				v.VPFromLive = map[int]bool{}
			}
			v.VPFromLive[i+1] = true
		}
	}
	v.Draft.Result.Sides = sides
}
