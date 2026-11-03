package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// endChecksFixture is what the live mode lists for frodo, the owner of Rangers: who of Rangers has the objective experience (Hador does)
// and the ballot for the MVP of Raiders (Snaga has his vote).
func endChecksFixture() core.EndChecks {
	return core.EndChecks{
		Objective: []core.CompanyObjectives{{CompanyID: 1, Name: "Rangers", Units: []core.EndCheckUnit{
			{UnitID: 5, Title: "Hador", Type: "Warrior", Zone: core.ZoneField, Has: true},
			{UnitID: 6, Title: "Aldor", Type: "Warrior", Zone: core.ZoneRetreated}}}},
		MVP: []core.MVPBallot{{CompanyID: 2, Name: "Raiders", Current: 21, Units: []core.EndCheckUnit{
			{UnitID: 20, Title: "Grashnak", Type: "Captain", Wounds: core.LiveStat{Current: 2}},
			{UnitID: 21, Title: "Snaga", Type: "Orc", Current: true}}}},
		Tally:       []core.MVPTally{{CompanyID: 2, Name: "Raiders", Lines: []core.TallyLine{{UnitID: 21, Title: "Snaga", Votes: 2}}}},
		ObjectiveXP: 1, MVPXP: 3,
	}
}

const sheetPath = matchPath + "/end/sheet"

func sheetFixture(t *testing.T, m core.Match) *fixture {
	t.Helper()
	f := matchWrite(t, m)
	f.camp.EXPECT().EndChecks(gomock.Any(), frodo, int64(7), int64(3)).Return(endChecksFixture(), nil).AnyTimes()
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return([]core.ScenarioInfo{cargoScenario}, nil).AnyTimes()
	return f
}

func TestEndMatchSheetAsksTheWinnerTheMVPAndTheObjective(t *testing.T) {
	m := rulebookMatch()
	m.Result = core.MatchResult{Companies: []core.CompanyResult{{CompanyID: 1, Objectives: map[string]int{"relics": 2}}}}
	f := sheetFixture(t, m)
	body := htmxGet(f, sheetPath).Body.String()
	for _, want := range []string{"End match", `name="winner" value="1"`, `name="winner" value="2"`, `name="winner" value="draw"`, ">Rangers</span>", ">Raiders</span>",
		"MVP of Raiders", `name="mvp-2"`, `value="21"`, `x-data="unitPick"`, "Snaga", "Grashnak", "3 experience",
		"Objective · Rangers", `name="objective-5" value="1" checked`, `name="objective-6" value="1" >`, "retreated",
		`name="tick-1-allCargo"`, `name="tick-1-relics"`, `value="2"`, `name="note"`, `hx-post="/campaigns/7/matches/3/end/propose"`, `name="revision" value="4"`} {
		if !strings.Contains(body, want) {
			t.Errorf("sheet lacks %q", want)
		}
	}
	for _, bad := range []string{"No MVP", "Victory Points", "Fled", "Not decided yet", `name="tick-2-`} {
		if strings.Contains(body, bad) {
			t.Errorf("the sheet has %q", bad)
		}
	}
}

func TestEndMatchSheetOfAScenarioWithoutRewardsHasNoTicks(t *testing.T) {
	f := matchWrite(t, readyToEnd())
	f.camp.EXPECT().EndChecks(gomock.Any(), frodo, int64(7), int64(3)).Return(endChecksFixture(), nil)
	if body := htmxGet(f, sheetPath).Body.String(); strings.Contains(body, "tick-") || !strings.Contains(body, "MVP of Raiders") {
		t.Error("a one-off scenario has no reward ticks")
	}
}

func TestEndMatchSheetOfAPracticeMatchHasNoMVP(t *testing.T) {
	m := readyToEnd()
	m.Practice = true
	f := matchWrite(t, m)
	f.camp.EXPECT().EndChecks(gomock.Any(), frodo, int64(7), int64(3)).Return(endChecksFixture(), nil)
	body := htmxGet(f, sheetPath).Body.String()
	if strings.Contains(body, "MVP of") || !strings.Contains(body, "No aftermath will follow.") || !strings.Contains(body, "(recorded only)") {
		t.Error("a practice match awards nothing: no MVP vote")
	}
}

func TestEndMatchSheetIsForThePlayers(t *testing.T) {
	f := matchWrite(t, outsiderMatch())
	if rec := htmxGet(f, sheetPath); rec.Code != http.StatusForbidden {
		t.Errorf("an outsider: %d", rec.Code)
	}
}

func TestEndMatchNeedsTheWinnerAndTheMVP(t *testing.T) {
	f := sheetFixture(t, readyToEnd())
	rec := f.do(f.signedIn(form(matchPath+"/end/propose", url.Values{"revision": {"4"}, "mvp-2": {"20"}, "note": {"keep me"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Choose who won, or a draw.") || !strings.Contains(rec.Body.String(), "keep me") {
		t.Errorf("no winner: %d", rec.Code)
	}
	rec = f.do(f.signedIn(form(matchPath+"/end/propose", url.Values{"revision": {"4"}, "winner": {"1"}, "mvp-2": {"99"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Choose the MVP of Raiders.") {
		t.Errorf("no MVP: %d %s", rec.Code, rec.Body.String())
	}
}

// End match: the votes and the objective switches that changed are live entries, then the result and the proposal, in one request.
func TestEndMatchSavesTheVotesTheObjectiveAndTheResultAndProposes(t *testing.T) {
	m := rulebookMatch()
	f := sheetFixture(t, m)
	gomock.InOrder(
		f.camp.EXPECT().Apply(gomock.Any(), frodo, int64(7), int64(3), int64(2), core.LiveAction{Kind: core.LiveMVPVote, UnitID: 20}).Return(core.LiveCompany{}, core.LiveEntry{}, nil),
		f.camp.EXPECT().Apply(gomock.Any(), frodo, int64(7), int64(3), int64(1), core.LiveAction{Kind: core.LiveObjective, UnitID: 5}).Return(core.LiveCompany{}, core.LiveEntry{}, nil),
		f.camp.EXPECT().Apply(gomock.Any(), frodo, int64(7), int64(3), int64(1), core.LiveAction{Kind: core.LiveObjective, UnitID: 6, On: true}).Return(core.LiveCompany{}, core.LiveEntry{}, nil),
	)
	want := core.MatchResult{Decided: true, Winner: 1, Note: "close", Companies: []core.CompanyResult{{CompanyID: 1, Objectives: map[string]int{"allCargo": 1}}}}
	saved := m
	saved.Revision, saved.Result = 5, want
	f.camp.EXPECT().SaveResult(gomock.Any(), frodo, int64(7), int64(3), 4, want).Return(saved, nil)
	f.camp.EXPECT().ProposeEnd(gomock.Any(), frodo, int64(7), int64(3), 5).Return(saved, nil)
	r := f.signedIn(form(matchPath+"/end/propose", url.Values{"revision": {"4"}, "winner": {"1"}, "mvp-2": {"20"}, "objective-6": {"1"}, "tick-1-allCargo": {"1"}, "note": {"close"}}))
	r.Header.Set("HX-Request", "true")
	rec := f.do(r)
	if rec.Code != http.StatusNoContent || rec.Header().Get("HX-Redirect") != matchPath {
		t.Errorf("proposed: %d %q\n%s", rec.Code, rec.Header().Get("HX-Redirect"), rec.Body.String())
	}
}

func TestEndMatchRefusedShowsTheSheetAgain(t *testing.T) {
	m := readyToEnd()
	f := sheetFixture(t, m)
	f.camp.EXPECT().ProposeEnd(gomock.Any(), frodo, int64(7), int64(3), 4).Return(core.Match{}, core.Refuse("Vote the MVP of Raiders first."))
	rec := f.do(f.signedIn(form(matchPath+"/end/propose", url.Values{"revision": {"4"}, "winner": {"1"}, "mvp-2": {"21"}, "objective-5": {"1"}})))
	body := rec.Body.String()
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(body, "Vote the MVP of Raiders first.") || !strings.Contains(body, `id="end-sheet"`) {
		t.Errorf("refused: %d\n%s", rec.Code, body)
	}
	// a stale sheet is answered with the current state, nothing is written
	f = sheetFixture(t, m)
	if rec := f.do(f.signedIn(form(matchPath+"/end/propose", url.Values{"revision": {"2"}, "winner": {"1"}, "mvp-2": {"21"}}))); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Somebody changed this match meanwhile") {
		t.Errorf("stale: %d", rec.Code)
	}
}

// waitingOnFrodo is a pending end Hador proposed (Raiders won) that frodo, the owner of Rangers, still has to confirm.
func waitingOnFrodo() core.Match {
	m := pendingEnd()
	m.Result = core.MatchResult{Decided: true, Winner: 2}
	m.EndProposal.ProposedBy, m.EndProposal.ProposedByID = "Hador", 50
	m.EndProposal.Companies[0].Confirmed, m.EndProposal.Companies[1].Confirmed = false, true
	return m
}

func TestConfirmSheetShowsTheProposalAndAsksTheVotes(t *testing.T) {
	f := sheetFixture(t, waitingOnFrodo())
	body := htmxGet(f, sheetPath).Body.String()
	for _, want := range []string{"Confirm the end", "Hador proposes: <strong>Raiders won</strong>", "Change result", `name="winner" value="2" checked`, "MVP of Raiders", "Objective · Rangers",
		`hx-post="/campaigns/7/matches/3/end/confirm"`, ">Confirm</button>"} {
		if !strings.Contains(body, want) {
			t.Errorf("confirm sheet lacks %q", want)
		}
	}
}

func TestConfirmSavesTheVoteAndConfirms(t *testing.T) {
	m := waitingOnFrodo()
	f := sheetFixture(t, m)
	f.camp.EXPECT().Apply(gomock.Any(), frodo, int64(7), int64(3), int64(2), core.LiveAction{Kind: core.LiveMVPVote, UnitID: 20}).Return(core.LiveCompany{}, core.LiveEntry{}, nil)
	f.camp.EXPECT().ConfirmEnd(gomock.Any(), frodo, int64(7), int64(3), 4).Return(m, nil) // the result is unchanged: no save
	r := f.signedIn(form(matchPath+"/end/confirm", url.Values{"revision": {"4"}, "winner": {"2"}, "mvp-2": {"20"}, "objective-5": {"1"}}))
	r.Header.Set("HX-Request", "true")
	if rec := f.do(r); rec.Code != http.StatusNoContent || rec.Header().Get("HX-Redirect") != matchPath {
		t.Errorf("confirmed: %d\n%s", rec.Code, rec.Body.String())
	}
	// Change result: the new winner is saved first (the other confirmations are reset by the service), then confirmed
	g := sheetFixture(t, m)
	changed := m
	changed.Revision = 5
	g.camp.EXPECT().SaveResult(gomock.Any(), frodo, int64(7), int64(3), 4, core.MatchResult{Decided: true, Winner: 0}).Return(changed, nil)
	g.camp.EXPECT().ConfirmEnd(gomock.Any(), frodo, int64(7), int64(3), 5).Return(changed, nil)
	if rec := g.do(g.signedIn(form(matchPath+"/end/confirm", url.Values{"revision": {"4"}, "winner": {"draw"}, "mvp-2": {"21"}, "objective-5": {"1"}}))); rec.Code != http.StatusSeeOther {
		t.Errorf("changed result: %d\n%s", rec.Code, rec.Body.String())
	}
}

func TestEndMatchFromTheLiveBannerOpensTheSameSheet(t *testing.T) {
	f := sheetFixture(t, readyToEnd())
	body := htmxGet(f, sheetPath+"?company=1").Body.String()
	if !strings.Contains(body, `name="company" value="1"`) || !strings.Contains(body, "MVP of Raiders") {
		t.Errorf("banner sheet: %s", body)
	}
}

func TestSheetCancelClosesTheWindow(t *testing.T) {
	f := sheetFixture(t, readyToEnd())
	if body := htmxGet(f, sheetPath).Body.String(); !strings.Contains(body, `$dispatch('close-modal')`) {
		t.Error("Cancel closes the window of the page it opened on")
	}
}

func TestEndMatchSheetPrefillsEnemyBrokenFromTheLiveMatch(t *testing.T) {
	m := rulebookMatch()
	m.Scenario = &core.MatchScenario{Kind: core.ScenarioRulebook, Name: "Hold"}
	f := matchWrite(t, m)
	f.camp.EXPECT().EndChecks(gomock.Any(), frodo, int64(7), int64(3)).Return(endChecksFixture(), nil)
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return([]core.ScenarioInfo{{Name: "Hold", Objectives: []core.ScenarioObjective{{Key: "enemyBroken", Label: "Enemy Broken", Max: 1}}}}, nil).AnyTimes()
	f.liveDefaults = &core.LiveMatchDefaults{EnemyBroken: map[int64]bool{1: true}}
	if body := htmxGet(f, sheetPath).Body.String(); !strings.Contains(body, `name="tick-1-enemyBroken" value="1" checked`) {
		t.Error("every opposing company Broken at the end ticks Enemy Broken")
	}
}
