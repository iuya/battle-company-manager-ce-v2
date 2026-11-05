package web

import (
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The End match and Confirm sheets (docs/matches.md, "Ending a match"). End match asks the
// winner (each side by its companies, or a draw), the MVP of every opposing company (D6, D7), the objective review of the viewer's own
// companies (D17), the scenario reward ticks when the scenario has rewards (D18) and a note, and proposes the end in one request. The
// other owners answer with the Confirm sheet: their MVP votes, the review and ticks of their own companies, and Confirm; Change result
// there edits the winner (the confirmations of the others are reset, as for any change of the result).

// endSheetView is one of the two sheets. It is shown in the window of the match page or of a company page in the live match.
type endSheetView struct {
	Campaign           int64
	Match              core.Match
	Confirm            bool  // the Confirm sheet (an end is pending)
	Company            int64 // the company page it was opened from; 0 on the match page
	Winner             string
	Note               string
	Ballots            []core.MVPBallot         // the viewer's MVP ballots, one per opposing company
	Votes              map[int64]int64          // the unit chosen per ballot (the saved vote, or what a failed submit posted)
	Objectives         []core.CompanyObjectives // the viewer's companies: who has the objective experience
	Has                map[int64]bool           // per unit: the objective switch as it shows
	Rewards            []core.ScenarioObjective // the scenario's reward ticks (empty for most scenarios)
	Tickers            []core.CampaignCompany   // the companies whose ticks the viewer sets
	Ticks              map[string]int           // "company/key" -> times
	Errors             []string
	ObjectiveXP, MVPXP int
}

// Tick is how many times the company completed the reward objective as the sheet shows it.
func (v endSheetView) Tick(companyID int64, key string) int {
	return v.Ticks[tickKey(companyID, key)]
}

// tickKey is where a company's reward tick is kept.
func tickKey(companyID int64, key string) string { return strconv.FormatInt(companyID, 10) + "/" + key }

// Vote is the unit chosen as MVP of the ballot's company (0 for none yet).
func (v endSheetView) Vote(companyID int64) int64 { return v.Votes[companyID] }

// VoteName is the title of the chosen unit, for the combobox (empty for none).
func (v endSheetView) VoteName(b core.MVPBallot) string {
	for _, u := range b.Units {
		if u.UnitID == v.Votes[b.CompanyID] {
			return u.Title
		}
	}
	return ""
}

// UnitsJSON feeds the MVP combobox of a ballot (static/js/match_form.js, unitPick): id, title and a line of detail per unit.
func (v endSheetView) UnitsJSON(b core.MVPBallot) string {
	type unit struct {
		ID     int64  `json:"id"`
		Title  string `json:"title"`
		Detail string `json:"detail"`
	}
	list := make([]unit, 0, len(b.Units))
	for _, u := range b.Units {
		detail := string(u.Zone)
		if u.Type != "" && u.Type != u.Title {
			detail = u.Type + " · " + detail
		}
		list = append(list, unit{u.UnitID, u.Title, detail})
	}
	out, _ := json.Marshal(list)
	return string(out)
}

// ProposedResult says what the pending end reports: "Rohan won" or "Draw".
func (v endSheetView) ProposedResult() string { return proposedResult(v.Match) }

func proposedResult(m core.Match) string {
	if w := m.ReportedWinner(); w != "" {
		return w + " won"
	}
	if m.Result.Decided {
		return "Draw"
	}
	return ""
}

// PostURL is where the sheet is submitted.
func (v endSheetView) PostURL() string {
	step := "propose"
	if v.Confirm {
		step = "confirm"
	}
	return matchPath(v.Campaign, v.Match.ID) + "/end/" + step
}

// endSheet builds the sheet for the viewer from the match and the end checks of the live mode.
func (s *Server) endSheet(r *http.Request, base campaignBase, m core.Match, confirm bool) (endSheetView, error) {
	v := endSheetView{Campaign: base.Campaign.ID, Match: m, Confirm: confirm, Company: formID(r, "company"), Winner: winnerValue(m.Result), Note: m.Result.Note,
		Votes: map[int64]int64{}, Has: map[int64]bool{}, Ticks: map[string]int{}}
	if v.Company == 0 {
		v.Company = queryID(r, "company")
	}
	checks, err := s.campaigns.EndChecks(r.Context(), *userFrom(r.Context()), base.Campaign.ID, m.ID)
	if err != nil && !errors.Is(err, core.ErrNotFound) {
		return v, err
	}
	v.ObjectiveXP, v.MVPXP = checks.ObjectiveXP, checks.MVPXP
	if !m.Practice {
		v.Ballots = checks.MVP
	}
	for _, b := range v.Ballots {
		v.Votes[b.CompanyID] = b.Current
	}
	v.Objectives = checks.Objective
	for _, c := range checks.Objective {
		for _, u := range c.Units {
			v.Has[u.UnitID] = u.Has
		}
	}
	if v.Rewards, err = s.scenarioObjectives(r, base.Campaign.ID, m); err != nil {
		return v, err
	}
	for _, c := range checks.Objective { // the viewer sets the ticks of the companies they may edit
		for _, co := range m.AllCompanies() {
			if co.ID == c.CompanyID {
				v.Tickers = append(v.Tickers, co)
			}
		}
	}
	reported := map[int64]bool{}
	for _, c := range m.Result.Companies {
		reported[c.CompanyID] = true
		for k, n := range c.Objectives {
			v.Ticks[tickKey(c.CompanyID, k)] = n
		}
	}
	if slices.ContainsFunc(v.Rewards, func(o core.ScenarioObjective) bool { return o.Key == "enemyBroken" }) {
		if def, ok, err := s.campaigns.MatchDefaults(r.Context(), base.Campaign.ID, m.ID); err == nil && ok { // every opposing company Broken
			for id := range def.EnemyBroken {
				if !reported[id] {
					v.Ticks[tickKey(id, "enemyBroken")] = 1
				}
			}
		}
	}
	return v, nil
}

// handleEndSheet answers the End match sheet, or the Confirm sheet when an end is pending (?company= is the company page it opens on).
func (s *Server) handleEndSheet(w http.ResponseWriter, r *http.Request) {
	base, m, ok := s.matchCtx(w, r)
	if !ok {
		return
	}
	user := *userFrom(r.Context())
	if matchAccess(base, m, user) != core.MatchEdit {
		s.refuse(w, r, http.StatusForbidden, noAccessReason(base, m))
		return
	}
	v, err := s.endSheet(r, base, m, m.EndProposal != nil)
	if s.fail(w, r, err) {
		return
	}
	if v.Confirm && !m.EndProposal.WaitingOn(user.ID) {
		v.Errors = append(v.Errors, "You have no company waiting to confirm.")
	}
	s.render.partial(w, http.StatusOK, "match", "end_sheet", v)
}

// handleProposeEnd is the submit of the End match sheet, handleConfirmEnd of the Confirm sheet.
func (s *Server) handleProposeEnd(w http.ResponseWriter, r *http.Request) {
	s.submitEndSheet(w, r, false)
}
func (s *Server) handleConfirmEnd(w http.ResponseWriter, r *http.Request) {
	s.submitEndSheet(w, r, true)
}

// submitEndSheet takes the votes, the objective changes and the result, then proposes or confirms the end, in one request. A refusal
// answers the sheet again (422) with what was posted; success goes to the match page.
func (s *Server) submitEndSheet(w http.ResponseWriter, r *http.Request, confirm bool) {
	base, m, ok := s.matchCtx(w, r)
	if !ok {
		return
	}
	ctx, user, id := r.Context(), *userFrom(r.Context()), base.Campaign.ID
	if matchAccess(base, m, user) != core.MatchEdit {
		s.refuse(w, r, http.StatusForbidden, noAccessReason(base, m))
		return
	}
	v, err := s.endSheet(r, base, m, confirm)
	if s.fail(w, r, err) {
		return
	}
	again := func(msgs ...string) {
		v.Errors = append(v.Errors, msgs...)
		s.render.partial(w, http.StatusUnprocessableEntity, "match", "end_sheet", v)
	}
	if revisionOf(r) != m.Revision {
		if v, err = s.endSheet(r, base, m, m.EndProposal != nil); s.fail(w, r, err) {
			return
		}
		again("Somebody changed this match meanwhile, so this is its current state. Check it and send it again.")
		return
	}
	switch {
	case confirm && m.EndProposal == nil:
		again("Nobody has proposed to end this match.")
		return
	case !confirm && m.EndProposal != nil:
		again("The end of this match has already been proposed.")
		return
	}
	v.keepPosted(r, confirm)
	res, problems := v.postedResult(r, confirm)
	if missing := v.missingMVPs(); len(missing) > 0 && len(problems) == 0 {
		problems = []string{"Choose the MVP of " + strings.Join(missing, " and ") + "."}
	}
	if len(problems) > 0 {
		again(problems...)
		return
	}
	if err := s.applyEndEntries(r, id, v); err != nil {
		s.sheetRefused(w, r, err, again)
		return
	}
	rev := m.Revision
	if !sameResult(res, m.Result) {
		saved, err := s.campaigns.SaveResult(ctx, user, id, m.ID, rev, res)
		if err != nil {
			s.sheetRefused(w, r, err, again)
			return
		}
		rev = saved.Revision
	}
	if confirm {
		_, err = s.campaigns.ConfirmEnd(ctx, user, id, m.ID, rev)
	} else {
		_, err = s.campaigns.ProposeEnd(ctx, user, id, m.ID, rev)
	}
	if err != nil {
		s.sheetRefused(w, r, err, again)
		return
	}
	redirect(w, r, matchPath(id, m.ID))
}

// keepPosted puts what was posted on the sheet, so a refusal shows it again.
func (v *endSheetView) keepPosted(r *http.Request, confirm bool) {
	v.Note = r.PostFormValue("note")
	if w := r.PostFormValue("winner"); w != "" || !confirm {
		v.Winner = w
	}
	for _, b := range v.Ballots {
		v.Votes[b.CompanyID] = formID(r, "mvp-"+itoa(int(b.CompanyID)))
	}
	for _, c := range v.Objectives {
		for _, u := range c.Units {
			v.Has[u.UnitID] = r.PostFormValue("objective-"+itoa(int(u.UnitID))) == "1"
		}
	}
	for _, c := range v.Tickers {
		for _, o := range v.Rewards {
			v.Ticks[tickKey(c.ID, o.Key)] = formInt(r, "tick-"+itoa(int(c.ID))+"-"+o.Key)
		}
	}
}

// postedResult is the result the sheet reports and what is wrong with it. The Confirm sheet keeps the winner unless Change result posted one.
func (v endSheetView) postedResult(r *http.Request, confirm bool) (core.MatchResult, []string) {
	res := v.Match.Result
	res.Note = v.Note
	if !confirm || r.PostFormValue("winner") != "" {
		var chosen bool
		if res.Decided, res.Winner, chosen = winnerFromForm(r, v.Match); !chosen {
			return res, []string{"Choose who won, or a draw."}
		}
	}
	return ticksFromForm(r, res, v.Tickers, v.Rewards)
}

// missingMVPs names the opposing companies whose ballot has no unit of theirs chosen.
func (v endSheetView) missingMVPs() []string {
	var missing []string
	for _, b := range v.Ballots {
		if !slices.ContainsFunc(b.Units, func(u core.EndCheckUnit) bool { return u.UnitID == v.Votes[b.CompanyID] }) {
			missing = append(missing, b.Name)
		}
	}
	return missing
}

// applyEndEntries writes the live entries of the sheet: the votes and the objective switches that changed (they never cancel a pending end).
func (s *Server) applyEndEntries(r *http.Request, id int64, v endSheetView) error {
	ctx, user, matchID := r.Context(), *userFrom(r.Context()), v.Match.ID
	for _, b := range v.Ballots {
		if vote := v.Votes[b.CompanyID]; vote != b.Current {
			if _, _, err := s.campaigns.Apply(ctx, user, id, matchID, b.CompanyID, core.LiveAction{Kind: core.LiveMVPVote, UnitID: vote}); err != nil {
				return err
			}
		}
	}
	for _, c := range v.Objectives {
		for _, u := range c.Units {
			if on := v.Has[u.UnitID]; on != u.Has {
				if _, _, err := s.campaigns.Apply(ctx, user, id, matchID, c.CompanyID, core.LiveAction{Kind: core.LiveObjective, UnitID: u.UnitID, On: on}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// sheetRefused answers a refused step of a sheet submit: the reason on the sheet, or the error page for anything unexpected.
func (s *Server) sheetRefused(w http.ResponseWriter, r *http.Request, err error, again func(...string)) {
	switch {
	case errors.Is(err, core.ErrMatchChanged):
		again("Somebody changed this match meanwhile. Check the sheet and send it again.")
	case errors.Is(err, core.ErrInvalidMatch):
		again(reasons(err)...)
	case errors.Is(err, core.ErrRefused), errors.Is(err, core.ErrInvalidLive):
		again(reason(err))
	default:
		s.fail(w, r, err)
	}
}

// sameResult says whether two results report the same thing (the order of the companies aside).
func sameResult(a, b core.MatchResult) bool {
	if a.Decided != b.Decided || a.Winner != b.Winner || strings.TrimSpace(a.Note) != strings.TrimSpace(b.Note) {
		return false
	}
	ticks := func(r core.MatchResult) map[string]int {
		out := map[string]int{}
		for _, c := range r.Companies {
			for k, n := range c.Objectives {
				if n != 0 {
					out[tickKey(c.CompanyID, k)] = n
				}
			}
		}
		return out
	}
	return maps.Equal(ticks(a), ticks(b))
}
