package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// editableMatch is a match frodo plays in; the campaign's organiser is somebody else.
func editableMatch() core.Match {
	return core.Match{ID: 3, Number: 3, Revision: 4, Sides: []core.MatchSide{
		{Name: "Gondor", Companies: []core.CampaignCompany{{ID: 1, Name: "Rangers", OwnerID: frodo.ID}}},
		{Name: "Mordor", Companies: []core.CampaignCompany{{ID: 2, Name: "Raiders", OwnerID: 50}}},
	}}
}

// outsiderMatch is one frodo is not in.
func outsiderMatch() core.Match {
	m := editableMatch()
	m.Sides[0].Companies[0].OwnerID = 51
	return m
}

const matchPath = "/campaigns/7/matches/3"

// matchWrite sets up a campaign and match for a request that loads them (twice when reloaded is set, as a stale write does).
func matchWrite(t *testing.T, m core.Match) *fixture {
	t.Helper()
	f := newFixture(t, Config{})
	d := campaignDetail()
	d.OrganiserID = 99
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().Match(gomock.Any(), int64(7), m.ID).Return(m, nil).AnyTimes()
	return f
}

var twoCompanies = []core.CampaignCompany{{ID: 1, Name: "Rangers"}, {ID: 2, Name: "Raiders"}}

// testSelection is the scenario selection of the shipped rules: two d6, the first picks one of three tables.
var testSelection = core.ScenarioSelection{Die: core.DiceSpec{Count: 1, Sides: 6}, Tables: []core.ScenarioTableRange{{Min: 1, Max: 2}, {Min: 3, Max: 4}, {Min: 5, Max: 6}}}

var testScenarios = []core.ScenarioInfo{
	{Name: "Seize Ground", Table: 1, Roll: 1, FirstDie: [2]int{1, 2}, Scoring: []string{"Hold the objectives"}},
	{Name: "Chance Encounter", Table: 1, Roll: 2, FirstDie: [2]int{1, 2}},
	{Name: "Rescue", Table: 3, Roll: 1, FirstDie: [2]int{5, 6}},
	{Name: "Homebrew", Table: 0},
}

func htmxGet(f *fixture, path string) *httptest.ResponseRecorder {
	r := f.signedIn(httptest.NewRequest("GET", path, nil))
	r.Header.Set("HX-Request", "true")
	return f.do(r)
}

func TestScenarioPickerListsTheScenariosByTable(t *testing.T) {
	f := matchWrite(t, editableMatch())
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return(testScenarios, nil)
	rec := htmxGet(f, matchPath+"/scenario?mode=pick")
	body := rec.Body.String()
	if rec.Code != 200 || strings.Contains(body, "<html") {
		t.Fatalf("a fragment was expected: %d", rec.Code)
	}
	for _, want := range []string{"Table 1 (first die 1-2)", "Table 3 (first die 5-6)", "Other scenarios", `value="Seize Ground"`, `value="Rescue"`, `name="revision" value="4"`, "Hold the objectives", `type="search"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "Table 2") {
		t.Error("a table without scenarios must not be listed")
	}
}

// A rules file can carry any text, and Alpine evaluates attribute values as JavaScript after the browser decodes the HTML
// entities, so names from the file must reach those expressions as string literals.
func TestScenarioPickerKeepsRulesTextOutOfScripts(t *testing.T) {
	m := editableMatch()
	m.Scenario = &core.MatchScenario{Kind: core.ScenarioRulebook, Name: "Evil', z: alert(1), y: '"}
	f := matchWrite(t, m)
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return([]core.ScenarioInfo{{Name: "Evil', z: alert(1), y: '", Table: 1, Roll: 1}}, nil)
	body := htmxGet(f, matchPath+"/scenario?mode=pick").Body.String()
	if !strings.Contains(body, `picked: &#34;Evil&#39;, z: alert(1), y: &#39;&#34;`) {
		t.Errorf("the scenario name should be a JSON string in the expression")
	}
}

func TestScenarioPickerIsForEditorsOnly(t *testing.T) {
	f := matchWrite(t, outsiderMatch())
	if rec := htmxGet(f, matchPath+"/scenario?mode=pick"); rec.Code != http.StatusForbidden {
		t.Errorf("an outsider asked for the picker: %d", rec.Code)
	}
	if rec := htmxGet(f, matchPath+"/scenario?mode=nonsense"); rec.Code != http.StatusNotFound {
		t.Errorf("an unknown mode: %d", rec.Code)
	}
}

func TestMatchPageOffersScenarioControlsToEditorsOnly(t *testing.T) {
	m := editableMatch()
	m.Scenario = &core.MatchScenario{Kind: core.ScenarioCustom, Name: "Ambush"}
	f := matchWrite(t, m)
	body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	for _, want := range []string{"Change scenario", "scenario/remove", `hx-get="/campaigns/7/matches/3/scenario?mode=pick"`} {
		if !strings.Contains(body, want) {
			t.Errorf("an editor misses %q", want)
		}
	}
	f = matchWrite(t, outsiderMatch())
	body = f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	for _, bad := range []string{"Choose scenario", "Change scenario", "scenario/remove", "mode=pick"} {
		if strings.Contains(body, bad) {
			t.Errorf("a viewer outside the match sees %q", bad)
		}
	}
}

func TestSetScenarioPostsTheNameAndRevision(t *testing.T) {
	m := editableMatch()
	f := matchWrite(t, m)
	done := m
	done.Revision = 5
	done.Scenario = &core.MatchScenario{Kind: core.ScenarioRulebook, Name: "Rescue"}
	f.camp.EXPECT().SetScenario(gomock.Any(), frodo, int64(7), int64(3), 4, core.ScenarioInput{Kind: core.ScenarioRulebook, Name: "Rescue"}).Return(done, nil)
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return(testScenarios, nil)
	rec := f.do(f.signedIn(form(matchPath+"/scenario", url.Values{"kind": {"rulebook"}, "name": {"Rescue"}, "revision": {"4"}})))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, `id="match"`) || !strings.Contains(body, "Rescue") || !strings.Contains(body, `value="5"`) {
		t.Errorf("the whole match was expected, with the new revision: %d\n%s", rec.Code, body)
	}
}

func TestSetScenarioRefusals(t *testing.T) {
	in := core.ScenarioInput{Kind: core.ScenarioRulebook, Name: "Rescue"}
	post := url.Values{"kind": {"rulebook"}, "name": {"Rescue"}, "revision": {"4"}}

	// an outsider: 403, nothing rendered
	f := matchWrite(t, outsiderMatch())
	f.camp.EXPECT().SetScenario(gomock.Any(), frodo, int64(7), int64(3), 4, in).Return(core.Match{}, core.Refuse("Only the players in this match can change it."))
	if rec := f.do(f.signedIn(form(matchPath+"/scenario", post))); rec.Code != http.StatusForbidden {
		t.Errorf("outsider: %d", rec.Code)
	}

	// an ended match for a non-admin
	ended := outsiderMatch()
	ended.Ended = true
	f = matchWrite(t, ended)
	f.camp.EXPECT().SetScenario(gomock.Any(), frodo, int64(7), int64(3), 4, in).Return(core.Match{}, core.Refuse("This match has ended; ask an administrator."))
	if rec := f.do(f.signedIn(form(matchPath+"/scenario", post))); rec.Code != http.StatusForbidden {
		t.Errorf("ended: %d", rec.Code)
	}

	// an unknown name: the picker again with the reason, 422 so htmx swaps it
	f = matchWrite(t, editableMatch())
	bad := core.ScenarioInput{Kind: core.ScenarioRulebook, Name: "Nowhere"}
	f.camp.EXPECT().SetScenario(gomock.Any(), frodo, int64(7), int64(3), 4, bad).Return(core.Match{}, errors.Join(core.ErrInvalidMatch, errors.New("the campaign's rules have no scenario called Nowhere")))
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return(testScenarios, nil)
	rec := f.do(f.signedIn(form(matchPath+"/scenario", url.Values{"kind": {"rulebook"}, "name": {"Nowhere"}, "revision": {"4"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "no scenario called Nowhere") || !strings.Contains(rec.Body.String(), `type="search"`) {
		t.Errorf("invalid name: %d\n%s", rec.Code, rec.Body.String())
	}
}

func TestStaleRevisionReloadsTheMatchWithANotice(t *testing.T) {
	m := editableMatch()
	f := matchWrite(t, m)
	f.camp.EXPECT().SetScenario(gomock.Any(), frodo, int64(7), int64(3), 2, gomock.Any()).Return(core.Match{}, core.ErrMatchChanged)
	rec := f.do(f.signedIn(form(matchPath+"/scenario", url.Values{"kind": {"rulebook"}, "name": {"Rescue"}, "revision": {"2"}})))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "changed this match while you were editing") || !strings.Contains(body, `id="match"`) {
		t.Errorf("stale: %d\n%s", rec.Code, body)
	}
}

func TestClearScenario(t *testing.T) {
	m := editableMatch()
	m.Scenario = &core.MatchScenario{Kind: core.ScenarioCustom, Name: "Ambush"}
	f := matchWrite(t, m)
	cleared := editableMatch()
	cleared.Revision = 5
	f.camp.EXPECT().ClearScenario(gomock.Any(), frodo, int64(7), int64(3), 4).Return(cleared, nil)
	rec := f.do(f.signedIn(form(matchPath+"/scenario/remove", url.Values{"revision": {"4"}})))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "No scenario yet.") {
		t.Errorf("remove: %d", rec.Code)
	}
}

func TestOneOffFormIsPrefilledAndEscaped(t *testing.T) {
	m := editableMatch()
	m.Scenario = &core.MatchScenario{Kind: core.ScenarioCustom, Name: `Ambush "<script>"`, Description: "line one\nline <two>", Victory: "last <b>"}
	f := matchWrite(t, m)
	body := htmxGet(f, matchPath+"/scenario?mode=custom").Body.String()
	for _, want := range []string{`value="Ambush &#34;&lt;script&gt;&#34;"`, "line &lt;two&gt;", "last &lt;b&gt;", `maxlength="60"`, `maxlength="1000"`, "Players are trusted", "kept with this match only", `name="kind" value="custom"`, `name="revision" value="4"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "<script>") {
		t.Error("the text must be escaped")
	}
	// an empty match offers an empty form; a rulebook scenario does not prefill it
	m.Scenario = &core.MatchScenario{Kind: core.ScenarioRulebook, Name: "Rescue"}
	f = matchWrite(t, m)
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return(testScenarios, nil)
	if body := htmxGet(f, matchPath+"/scenario?mode=custom").Body.String(); strings.Contains(body, `value="Rescue"`) {
		t.Error("the one-off form must not carry the rulebook name")
	}
}

func TestSaveOneOffScenario(t *testing.T) {
	m := editableMatch()
	f := matchWrite(t, m)
	in := core.ScenarioInput{Kind: core.ScenarioCustom, Name: "Ambush", Description: "d <script>", Victory: "v"}
	done := m
	done.Revision = 5
	done.Scenario = &in
	f.camp.EXPECT().SetScenario(gomock.Any(), frodo, int64(7), int64(3), 4, in).Return(done, nil)
	rec := f.do(f.signedIn(form(matchPath+"/scenario", url.Values{"kind": {"custom"}, "name": {"Ambush"}, "description": {"d <script>"}, "victory": {"v"}, "revision": {"4"}})))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "One-off") || !strings.Contains(body, "d &lt;script&gt;") || strings.Contains(body, "d <script>") {
		t.Errorf("saved: %d\n%s", rec.Code, body)
	}
}

func TestOneOffValidationErrorsSitBesideTheFields(t *testing.T) {
	f := matchWrite(t, editableMatch())
	_, verr := core.ScenarioInput{Kind: core.ScenarioCustom, Name: "", Description: strings.Repeat("x", 1001)}.Validate()
	f.camp.EXPECT().SetScenario(gomock.Any(), frodo, int64(7), int64(3), 4, gomock.Any()).Return(core.Match{}, verr)
	rec := f.do(f.signedIn(form(matchPath+"/scenario", url.Values{"kind": {"custom"}, "name": {" "}, "description": {strings.Repeat("x", 1001)}, "victory": {"keep me"}, "revision": {"4"}})))
	body := rec.Body.String()
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d", rec.Code)
	}
	for _, want := range []string{`id="scenario-name-err"`, "The name must be between 1 and 60 characters", `id="scenario-description-err"`, "at most 1000 characters", "keep me", `aria-invalid="true"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, `id="scenario-victory-err"`) {
		t.Error("the field without a problem must not show an error")
	}
}

func TestOneOffScenarioIsNotOfferedByThePicker(t *testing.T) {
	// the picker lists the campaign's rules file only; a one-off of another match is match data and never reaches it
	other := editableMatch()
	other.Scenario = &core.MatchScenario{Kind: core.ScenarioCustom, Name: "Secret Ambush"}
	f := matchWrite(t, editableMatch())
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return(testScenarios, nil)
	if body := htmxGet(f, matchPath+"/scenario?mode=pick").Body.String(); strings.Contains(body, other.Scenario.Name) {
		t.Error("a one-off scenario appeared in the picker")
	}
}

// signedInAs is signedIn for another user.
func (f *fixture) signedInAs(u core.User, r *http.Request) *http.Request {
	f.auth.EXPECT().Authenticate(gomock.Any(), "tok").Return(u, time.Time{}, nil).AnyTimes()
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "tok"})
	return r
}

var gandalf = core.User{ID: 99, DisplayName: "Gandalf"} // the organiser of the campaign the tests use
var sauron = core.User{ID: 77, DisplayName: "Admin", Role: core.RoleAdmin}

func readyToEnd() core.Match {
	m := editableMatch()
	m.Scenario = &core.MatchScenario{Kind: core.ScenarioCustom, Name: "Ambush"}
	m.Result = core.MatchResult{Decided: true, Winner: 1, Sides: make([]core.SideResult, 2)}
	return m
}

func TestResultFormForAnEditor(t *testing.T) {
	f := matchWrite(t, editableMatch())
	body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	for _, want := range []string{
		`hx-post="/campaigns/7/matches/3/result"`, `name="winner" value="1"`, `name="winner" value="2"`, `name="winner" value="draw"`, `name="winner" value=""`,
		`name="vp-1"`, `inputmode="numeric"`, `name="fled-1"`, `name="fled-2"`, "All is Lost", `name="note"`, `maxlength="500"`, "Save result", `data-winner=""`,
		`name="revision" value="4"`, `/campaigns/7/matches/3/live/end/reminder?revision=4`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "Edit the result (organiser or administrator)") || strings.Contains(body, "Ended ") {
		t.Error("an open match has no administrator edit")
	}
	// no radio is preselected on the server side: an unreported match is not a draw
	if strings.Contains(body, `value="draw" checked`) || strings.Contains(body, `name="winner" value="1" checked`) {
		t.Error("a winner is preselected")
	}
}

func TestDrawAndNotReportedAreDistinguished(t *testing.T) {
	m := editableMatch()
	m.Result = core.MatchResult{Decided: true, Winner: 0, Sides: make([]core.SideResult, 2)}
	f := matchWrite(t, m)
	if body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String(); !strings.Contains(body, `data-winner="draw"`) {
		t.Error("a reported draw must start on Draw")
	}
	m.Result = core.MatchResult{Decided: true, Winner: 2, Sides: make([]core.SideResult, 2)}
	f = matchWrite(t, m)
	if body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String(); !strings.Contains(body, `data-winner="2"`) {
		t.Error("a reported winner must start on that side")
	}
}

func TestOrganiserAndAdminSeeTheResultFormOfAnOpenMatch(t *testing.T) {
	for _, u := range []core.User{gandalf, sauron} {
		f := newFixture(t, Config{})
		d := campaignDetail()
		d.OrganiserID = gandalf.ID
		f.camp.EXPECT().Get(gomock.Any(), u.ID, int64(7)).Return(d, nil)
		f.camp.EXPECT().Match(gomock.Any(), int64(7), int64(3)).Return(outsiderMatch(), nil)
		body := f.do(f.signedInAs(u, httptest.NewRequest("GET", matchPath, nil))).Body.String()
		for _, want := range []string{"Save result", "Propose to end", "Choose scenario"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s misses %q", u.DisplayName, want)
			}
		}
	}
}

func TestSaveResultPostsTheWholeReport(t *testing.T) {
	m := editableMatch()
	f := matchWrite(t, m)
	vp1, vp2 := 8, 0
	want := core.MatchResult{Decided: true, Winner: 2, Note: "close <one>",
		Sides:     []core.SideResult{{VictoryPoints: &vp1}, {VictoryPoints: &vp2}},
		Companies: []core.CompanyResult{{CompanyID: 1, Fled: true}}}
	done := m
	done.Revision = 5
	done.Result = want
	f.camp.EXPECT().SaveResult(gomock.Any(), frodo, int64(7), int64(3), 4, want).Return(done, nil)
	rec := f.do(f.signedIn(form(matchPath+"/result", url.Values{"winner": {"2"}, "vp-1": {"8"}, "vp-2": {"0"}, "fled-1": {"1"}, "note": {"close <one>"}, "revision": {"4"}})))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "Result saved.") || !strings.Contains(body, `id="match"`) || !strings.Contains(body, `value="5"`) || !strings.Contains(body, "close &lt;one&gt;") {
		t.Errorf("saved: %d\n%s", rec.Code, body)
	}
}

func TestSaveResultKeepsTheTypedValuesWhenRejected(t *testing.T) {
	f := matchWrite(t, editableMatch())
	rec := f.do(f.signedIn(form(matchPath+"/result", url.Values{"winner": {"1"}, "vp-1": {"lots"}, "note": {"keep this"}, "revision": {"4"}})))
	body := rec.Body.String()
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(body, "whole number") || !strings.Contains(body, "keep this") {
		t.Errorf("not a whole number: %d\n%s", rec.Code, body)
	}

	f = matchWrite(t, editableMatch())
	f.camp.EXPECT().SaveResult(gomock.Any(), frodo, int64(7), int64(3), 4, gomock.Any()).Return(core.Match{}, errors.Join(core.ErrInvalidMatch, errors.New("Victory Points must be between 0 and 99")))
	rec = f.do(f.signedIn(form(matchPath+"/result", url.Values{"winner": {"1"}, "vp-1": {"120"}, "note": {"keep this"}, "revision": {"4"}})))
	body = rec.Body.String()
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(body, "between 0 and 99") || !strings.Contains(body, `value="120"`) || !strings.Contains(body, "keep this") {
		t.Errorf("rejected: %d\n%s", rec.Code, body)
	}
}

func TestResultWritesByOutsidersAreForbidden(t *testing.T) {
	f := matchWrite(t, outsiderMatch())
	if rec := f.do(f.signedIn(form(matchPath+"/result", url.Values{"winner": {"1"}, "revision": {"4"}}))); rec.Code != http.StatusForbidden {
		t.Errorf("save: %d", rec.Code)
	}
	f = matchWrite(t, outsiderMatch())
	f.camp.EXPECT().ProposeEnd(gomock.Any(), frodo, int64(7), int64(3), 4).Return(core.Match{}, core.Refuse("Only the players in this match can change it."))
	if rec := f.do(f.signedIn(form(matchPath+"/end/propose", url.Values{"revision": {"4"}}))); rec.Code != http.StatusForbidden {
		t.Errorf("end: %d", rec.Code)
	}
	// and the page shows them no controls
	f = matchWrite(t, outsiderMatch())
	body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	for _, bad := range []string{"Save result", "Propose to end", `name="winner"`, "Edit the result (organiser or administrator)"} {
		if strings.Contains(body, bad) {
			t.Errorf("an outsider sees %q", bad)
		}
	}
}

func TestStaleResultShowsTheNotice(t *testing.T) {
	f := matchWrite(t, editableMatch())
	f.camp.EXPECT().SaveResult(gomock.Any(), frodo, int64(7), int64(3), 2, gomock.Any()).Return(core.Match{}, core.ErrMatchChanged)
	rec := f.do(f.signedIn(form(matchPath+"/result", url.Values{"winner": {"1"}, "revision": {"2"}})))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "changed this match while you were editing") || !strings.Contains(rec.Body.String(), `name="revision" value="4"`) {
		t.Errorf("stale: %d", rec.Code)
	}
}

func TestProposeEndButtonIsDisabledWithTheReason(t *testing.T) {
	f := matchWrite(t, editableMatch())
	body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	for _, want := range []string{"disabled", "Choose the scenario before ending the match.", "Propose to end"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	f = matchWrite(t, readyToEnd())
	body = f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	if strings.Contains(body, `id="end-reason"`) || strings.Contains(body, "aria-describedby=\"end-reason\"") {
		t.Error("a match that can end shows no reason")
	}
	if !strings.Contains(body, "confirm(") {
		t.Error("ending asks for a confirmation")
	}
}

func TestProposeEndUpdatesTheHeaderAndTurnsTheFormReadOnly(t *testing.T) {
	m := readyToEnd()
	f := matchWrite(t, m)
	now := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	done := m
	done.Ended, done.EndedOn, done.Winner, done.Revision = true, &now, 1, 5
	f.camp.EXPECT().ProposeEnd(gomock.Any(), frodo, int64(7), int64(3), 4).Return(done, nil)
	rec := f.do(f.signedIn(form(matchPath+"/end/propose", url.Values{"revision": {"4"}})))
	body := rec.Body.String()
	for _, want := range []string{"Match ended.", "Gondor won", "Ended 19 October 2026", "This match has ended."} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, bad := range []string{"Save result", `name="winner"`, "Propose to end", "Edit the result (organiser or administrator)"} {
		if strings.Contains(body, bad) {
			t.Errorf("an ended match still shows %q to a player", bad)
		}
	}
}

func TestProposeEndRefusedShowsTheReason(t *testing.T) {
	f := matchWrite(t, editableMatch())
	f.camp.EXPECT().ProposeEnd(gomock.Any(), frodo, int64(7), int64(3), 4).Return(core.Match{}, core.Refuse("Choose the scenario before ending the match."))
	rec := f.do(f.signedIn(form(matchPath+"/end/propose", url.Values{"revision": {"4"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Choose the scenario before ending the match.") {
		t.Errorf("refused: %d", rec.Code)
	}
}

func TestAdministratorEditsAnEndedMatchBehindAButton(t *testing.T) {
	for _, tt := range []struct {
		name      string
		refs      []core.AftermathRef
		warning   string
		reopenBtn bool
	}{
		{"no aftermath started", []core.AftermathRef{{CompanyID: 1, Status: core.AftermathPending}}, "No aftermath has started yet", true},
		{"an aftermath started", []core.AftermathRef{{CompanyID: 1, Status: core.AftermathInjuries}}, "A company has already played its aftermath; changes here do not redo it.", false},
		{"an aftermath waived", []core.AftermathRef{{CompanyID: 1, Status: core.AftermathWaived}}, "changes here do not redo it.", false},
	} {
		m := readyToEnd()
		now := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
		m.Ended, m.EndedOn, m.Winner = true, &now, 1
		m.Sides[0].Companies[0].OwnerID = 51
		f := newFixture(t, Config{})
		f.aftermaths = tt.refs
		f.camp.EXPECT().Aftermath(gomock.Any(), gomock.Any(), int64(7), int64(3), int64(1)).Return(core.Aftermath{}, nil).AnyTimes()
		d := campaignDetail()
		d.OrganiserID = 99
		f.camp.EXPECT().Get(gomock.Any(), sauron.ID, int64(7)).Return(d, nil)
		f.camp.EXPECT().Match(gomock.Any(), int64(7), int64(3)).Return(m, nil)
		body := f.do(f.signedInAs(sauron, httptest.NewRequest("GET", matchPath, nil))).Body.String()
		for _, want := range []string{"Edit the result (organiser or administrator)", tt.warning, "Save result", `hx-post="/campaigns/7/matches/3/result"`} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: missing %q", tt.name, want)
			}
		}
		if got := strings.Contains(body, "Reopen match"); got != tt.reopenBtn {
			t.Errorf("%s: Reopen match shown = %v, want %v", tt.name, got, tt.reopenBtn)
		}
		if strings.Contains(body, ">Propose to end<") {
			t.Error("an ended match cannot be ended again")
		}
	}
}

func TestResultFormWarnsAboutAFledWinner(t *testing.T) {
	m := editableMatch()
	m.Result = core.MatchResult{Decided: true, Winner: 1, Sides: make([]core.SideResult, 2), Companies: []core.CompanyResult{{CompanyID: 1, Fled: true}}}
	f := matchWrite(t, m)
	body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	// the form carries the state the browser warns from, and ending is refused with the same reason
	for _, want := range []string{"&#34;fled&#34;:true", "x-text=\"conflict()\"", "Rangers fled the battlefield, so Gondor cannot be the winner."} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}

var cargoScenario = core.ScenarioInfo{Name: "Baggage Train", Table: 3, Roll: 3, Objectives: []core.ScenarioObjective{
	{Key: "allCargo", Label: "Scored all Cargo", Max: 1, Rewards: []core.ScenarioReward{{Kind: "influence", Amount: 1, Text: "A company that scores all Cargo earns 1 bonus Influence."}}},
	{Key: "relics", Label: "Relics carried off", Max: 3, Rewards: []core.ScenarioReward{{Kind: "other", Text: "Each relic is worth a story."}}},
}}

func rulebookMatch() core.Match {
	m := editableMatch()
	m.Scenario = &core.MatchScenario{Kind: core.ScenarioRulebook, Name: "Baggage Train"}
	return m
}

func TestResultFormShowsTheObjectivesWithTheirRewards(t *testing.T) {
	m := rulebookMatch()
	m.Result = core.MatchResult{Sides: make([]core.SideResult, 2), Companies: []core.CompanyResult{{CompanyID: 1, Objectives: map[string]int{"allCargo": 1, "relics": 2}}}}
	f := matchWrite(t, m)
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return([]core.ScenarioInfo{cargoScenario}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	for _, want := range []string{`name="obj-1-allCargo"`, `name="obj-2-allCargo"`, `name="obj-1-relics"`, `max="3"`, `value="2"`, "Scored all Cargo",
		"Completing this earns: 1 Influence", "Completing this earns: Each relic is worth a story."} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if !strings.Contains(body, `value="1" checked`) {
		t.Error("the reported tick is not checked")
	}
}

func TestScenariosWithoutObjectivesShowNone(t *testing.T) {
	m := rulebookMatch()
	f := matchWrite(t, m)
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return([]core.ScenarioInfo{{Name: "Baggage Train"}}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	if strings.Contains(body, "obj-") || strings.Contains(body, "Completing this earns") {
		t.Error("objectives shown for a scenario without any")
	}
	f = matchWrite(t, editableMatch()) // a match without scenario, and a one-off one
	if body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String(); strings.Contains(body, "obj-") {
		t.Error("objectives without a scenario")
	}
}

func TestSaveResultPostsTheObjectives(t *testing.T) {
	m := rulebookMatch()
	f := matchWrite(t, m)
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return([]core.ScenarioInfo{cargoScenario}, nil).AnyTimes()
	want := core.MatchResult{Sides: make([]core.SideResult, 2), Companies: []core.CompanyResult{
		{CompanyID: 1, Objectives: map[string]int{"allCargo": 1, "relics": 2}}, {CompanyID: 2, Fled: true}}}
	done := m
	done.Revision, done.Result = 5, want
	f.camp.EXPECT().SaveResult(gomock.Any(), frodo, int64(7), int64(3), 4, want).Return(done, nil)
	rec := f.do(f.signedIn(form(matchPath+"/result", url.Values{"revision": {"4"}, "obj-1-allCargo": {"1"}, "obj-1-relics": {"2"}, "obj-2-relics": {"0"}, "fled-2": {"1"}, "obj-1-bogus": {"1"}})))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, `value="2"`) || !strings.Contains(body, "Result saved.") {
		t.Errorf("saved: %d\n%s", rec.Code, body)
	}
}

func TestSaveResultShowsAnObjectiveCountOverTheMaximum(t *testing.T) {
	m := rulebookMatch()
	f := matchWrite(t, m)
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return([]core.ScenarioInfo{cargoScenario}, nil).AnyTimes()
	f.camp.EXPECT().SaveResult(gomock.Any(), frodo, int64(7), int64(3), 4, gomock.Any()).Return(core.Match{}, errors.Join(core.ErrInvalidMatch, errors.New(`"relics" can be completed 0 to 3 times`)))
	rec := f.do(f.signedIn(form(matchPath+"/result", url.Values{"revision": {"4"}, "obj-1-relics": {"9"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "0 to 3 times") || !strings.Contains(rec.Body.String(), `value="9"`) {
		t.Errorf("over max: %d", rec.Code)
	}
}

func TestReadOnlyReportListsTheCompletedObjectives(t *testing.T) {
	m := rulebookMatch()
	m.Sides[0].Companies[0].OwnerID = 51
	m.Result = core.MatchResult{Sides: make([]core.SideResult, 2), Companies: []core.CompanyResult{{CompanyID: 1, Objectives: map[string]int{"allCargo": 1, "relics": 2}}}}
	f := matchWrite(t, m)
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return([]core.ScenarioInfo{cargoScenario}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	if !strings.Contains(body, "Rangers") || !strings.Contains(body, "completed:") || !strings.Contains(body, "Scored all Cargo.") || !strings.Contains(body, "Relics carried off (x2)") {
		t.Errorf("report: %s", body)
	}
}

func TestMatchListsShowTheScenario(t *testing.T) {
	list := []core.Match{
		{ID: 1, Number: 1, Scenario: &core.MatchScenario{Kind: core.ScenarioRulebook, Name: "Seize Ground"}, Sides: editableMatch().Sides},
		{ID: 2, Number: 2, Scenario: &core.MatchScenario{Kind: core.ScenarioCustom, Name: "Ambush <b>"}, Sides: editableMatch().Sides},
		{ID: 3, Number: 3, Sides: editableMatch().Sides},
	}
	for _, path := range []string{"/campaigns/7/matches", "/campaigns/7"} {
		f := newFixture(t, Config{})
		f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
		f.camp.EXPECT().Companies(gomock.Any(), int64(7)).Return(twoCompanies, nil).AnyTimes()
		f.camp.EXPECT().Matches(gomock.Any(), int64(7)).Return(list, nil)
		body := f.do(f.signedIn(httptest.NewRequest("GET", path, nil))).Body.String()
		if !strings.Contains(body, "Scenario: Seize Ground") || !strings.Contains(body, "One-off: Ambush &lt;b&gt;") {
			t.Errorf("%s: scenarios missing", path)
		}
		if n := strings.Count(body, "Scenario: ") + strings.Count(body, "One-off: "); n != 2 {
			t.Errorf("%s: %d scenario lines, want 2 (none for a match without scenario)", path, n)
		}
	}
}

func rollSetup(t *testing.T, m core.Match, settings core.CampaignSettings) *fixture {
	t.Helper()
	f := newFixture(t, Config{})
	d := campaignDetail()
	d.OrganiserID = 99
	d.Settings = settings
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().Match(gomock.Any(), int64(7), m.ID).Return(m, nil).AnyTimes()
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return(testScenarios, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), gomock.Any()).Return(core.CompanyDetail{}, nil).AnyTimes()
	return f
}

func TestScenarioRollPreselectsOnlyAndShowsTopDog(t *testing.T) {
	m := editableMatch()
	m.Sides[0].Companies[0].Points = 120
	m.Sides[1].Companies[0].Points = 100
	f := rollSetup(t, m, core.CampaignSettings{})
	seq := []int{1, 2}
	f.srv.die = func(int) int { n := seq[0]; seq = seq[1:]; return n }
	rec := f.do(f.signedIn(form(matchPath+"/scenario/roll", url.Values{"roll": {"0"}, "roll2": {"0"}})))
	body := rec.Body.String()
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, body)
	}
	for _, want := range []string{"Chance Encounter", `data-pick="Chance Encounter"`, `data-dice="1,2"`, `data-animate="true"`, "Top Dog", "Underdog", "rating 120"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q\n%s", want, body)
		}
	}
	if strings.Contains(body, `name="roll"`) && strings.Contains(body, "I rolled a") {
		t.Error("typed rolls are off")
	}
}

func TestScenarioRollRefusesTypedAndThrownRollsUnlessAllowed(t *testing.T) {
	f := rollSetup(t, editableMatch(), core.CampaignSettings{})
	rec := f.do(f.signedIn(form(matchPath+"/scenario/roll", url.Values{"roll": {"4"}, "roll2": {"0"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "does not let players enter their own rolls") || strings.Contains(rec.Body.String(), "data-pick") {
		t.Errorf("typed: %d %s", rec.Code, rec.Body.String())
	}
	rec = f.do(f.signedIn(form(matchPath+"/scenario/roll", url.Values{"roll": {"4"}, "roll2": {"2"}, "thrown": {"1"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "throw the dice in their browser") {
		t.Errorf("thrown: %d", rec.Code)
	}
	f = rollSetup(t, editableMatch(), core.CampaignSettings{ManualRolls: true})
	rec = f.do(f.signedIn(form(matchPath+"/scenario/roll", url.Values{"roll": {"6"}, "roll2": {"1"}})))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `data-pick="Rescue"`) || !strings.Contains(rec.Body.String(), `data-animate="false"`) {
		t.Errorf("typed 6,1: %d %s", rec.Code, rec.Body.String())
	}
	f = rollSetup(t, editableMatch(), core.CampaignSettings{ShakeTheTable: true})
	rec = f.do(f.signedIn(form(matchPath+"/scenario/roll", url.Values{"roll": {"5"}, "roll2": {"1"}, "thrown": {"1"}})))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `data-pick="Rescue"`) {
		t.Errorf("thrown: %d", rec.Code)
	}
	f = rollSetup(t, editableMatch(), core.CampaignSettings{ManualRolls: true})
	if rec = f.do(f.signedIn(form(matchPath+"/scenario/roll", url.Values{"roll": {"9"}}))); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("a 9 on a D6: %d", rec.Code)
	}
}

func TestScenarioRollIsForEditorsOnly(t *testing.T) {
	f := rollSetup(t, outsiderMatch(), core.CampaignSettings{})
	if rec := f.do(f.signedIn(form(matchPath+"/scenario/roll", url.Values{}))); rec.Code != http.StatusForbidden {
		t.Errorf("an outsider rolled: %d", rec.Code)
	}
}

func TestScenarioPickerOffersTheRoll(t *testing.T) {
	f := rollSetup(t, editableMatch(), core.CampaignSettings{ShakeTheTable: true})
	body := htmxGet(f, matchPath+"/scenario?mode=pick").Body.String()
	for _, want := range []string{"Roll for it", "Roll the dice", "throwScenario", `scenario/roll"`, "Use this scenario"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func endedBy(m core.Match) core.Match {
	on := time.Date(2026, 10, 19, 14, 5, 0, 0, time.UTC)
	m.Ended, m.EndedOn, m.Winner = true, &on, 1
	m.History = []core.MatchEvent{{Kind: "ended", By: "Frodo", On: on}, {Kind: "reopened", By: "Sauron", On: on.Add(time.Hour)}}
	return m
}

func TestReopenIsOfferedToAdministratorsOnlyAndHistoryIsListed(t *testing.T) {
	m := endedBy(readyToEnd())
	m.Sides[0].Companies[0].OwnerID = 51
	f := newFixture(t, Config{})
	d := campaignDetail()
	d.OrganiserID = 99
	f.camp.EXPECT().Get(gomock.Any(), sauron.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().Match(gomock.Any(), int64(7), int64(3)).Return(m, nil)
	body := f.do(f.signedInAs(sauron, httptest.NewRequest("GET", matchPath, nil))).Body.String()
	for _, want := range []string{"Reopen match", `hx-post="/campaigns/7/matches/3/reopen"`, `name="revision" value="4"`, "History", "Ended</span> by Frodo", "Reopened</span> by Sauron", "19 October 2026, 14:05"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Index(body, "by Frodo") > strings.Index(body, "by Sauron") {
		t.Error("history lists the oldest first, newest last")
	}
	// a player of the match sees the history but not the button
	f = matchWrite(t, endedBy(readyToEnd()))
	body = f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	if strings.Contains(body, "Reopen match") || !strings.Contains(body, "by Frodo") {
		t.Error("players see the history, not the button")
	}
}

func TestReopenMatchIsForbiddenToEveryoneButAdministrators(t *testing.T) {
	f := matchWrite(t, endedBy(readyToEnd())) // frodo plays in it; no call to the backend is expected
	if rec := f.do(f.signedIn(form(matchPath+"/reopen", url.Values{"revision": {"4"}}))); rec.Code != http.StatusForbidden {
		t.Errorf("a player reopened: %d", rec.Code)
	}
}

func TestReopenMatchAnswersTheOpenMatch(t *testing.T) {
	m := endedBy(readyToEnd())
	f := newFixture(t, Config{})
	d := campaignDetail()
	d.OrganiserID = 99
	f.camp.EXPECT().Get(gomock.Any(), sauron.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().Match(gomock.Any(), int64(7), int64(3)).Return(m, nil)
	open := m
	open.Ended, open.EndedOn, open.Revision = false, nil, 5
	f.camp.EXPECT().ReopenMatch(gomock.Any(), sauron, int64(7), int64(3), 4).Return(open, nil)
	rec := f.do(f.signedInAs(sauron, form(matchPath+"/reopen", url.Values{"revision": {"4"}})))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "Match reopened.") || !strings.Contains(body, "Propose to end") || strings.Contains(body, "Reopen match") {
		t.Errorf("%d %s", rec.Code, body)
	}
}

// finishedMatch is a match frodo plays in, in a finished campaign.
func finishedMatch(t *testing.T, m core.Match) *fixture {
	t.Helper()
	f := newFixture(t, Config{})
	d := campaignDetail()
	d.OrganiserID = 99
	d.Status = core.CampaignFinished
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().Match(gomock.Any(), int64(7), m.ID).Return(m, nil).AnyTimes()
	f.camp.EXPECT().Matches(gomock.Any(), int64(7)).Return([]core.Match{m}, nil).AnyTimes()
	f.camp.EXPECT().Companies(gomock.Any(), int64(7)).Return(twoCompanies, nil).AnyTimes()
	return f
}

func TestFinishedCampaignTakesNoNewMatchesAndNoEdits(t *testing.T) {
	f := finishedMatch(t, editableMatch())
	if body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches", nil))).Body.String(); strings.Contains(body, "New match") {
		t.Error("the list offers New match in a finished campaign")
	}
	if rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/new", nil))); rec.Code != http.StatusForbidden {
		t.Errorf("new match page: %d", rec.Code)
	}
	if rec := f.do(f.signedIn(form("/campaigns/7/matches", url.Values{"sides": {"2"}, "side-1": {"1"}, "side-2": {"2"}}))); rec.Code != http.StatusForbidden ||
		!strings.Contains(rec.Body.String(), "The campaign is finished.") {
		t.Errorf("create: %d %s", rec.Code, rec.Body.String())
	}
	rec := f.do(f.signedIn(form(matchPath+"/result", url.Values{"winner": {"1"}, "revision": {"4"}})))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "The campaign is finished.") {
		t.Errorf("result: %d %s", rec.Code, rec.Body.String())
	}
	body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	for _, bad := range []string{"Save result", "Propose to end", `name="winner"`, "Choose scenario"} {
		if strings.Contains(body, bad) {
			t.Errorf("a finished campaign's match offers %q", bad)
		}
	}
}

func TestEndedMatchTellsAPlayerToAskTheOrganiserOrAnAdministrator(t *testing.T) {
	m := editableMatch()
	m.Ended = true
	f := matchWrite(t, m)
	rec := f.do(f.signedIn(form(matchPath+"/result", url.Values{"winner": {"1"}, "revision": {"4"}})))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "This match has ended; ask the organiser or an administrator.") {
		t.Errorf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestShellShowsFailedHtmxRequests(t *testing.T) {
	f := matchWrite(t, editableMatch())
	body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	for _, want := range []string{`@htmx:response-error.window`, `@htmx:send-error.window`, `x-text="err"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the shell lacks %q: failures of htmx requests would be silent", want)
		}
	}
}

func TestMatchPageShowsTheRulebookTextAndThePrizes(t *testing.T) {
	m := editableMatch()
	m.Scenario = &core.MatchScenario{Kind: core.ScenarioRulebook, Name: "Chance Encounter"}
	f := matchWrite(t, m)
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return([]core.ScenarioInfo{{Name: "Chance Encounter",
		Scoring: []string{"Score points"},
		About: core.ScenarioAbout{Description: "Two companies <meet>.", Roles: []string{"Attacker", "Defender"},
			Sections: []core.ScenarioText{{Title: "Layout", Text: "Dense terrain."}},
			Rules:    []core.ScenarioText{{Title: "Defend the Hill", Text: "No Courage tests."}}},
		Prizes: []core.ScenarioPrize{{Name: "Rendezvous", Who: "The winner", Text: "Plus or minus one."}}}}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	for _, want := range []string{"About this scenario", "Two companies &lt;meet&gt;.", "Dense terrain.", "Special rules", "Defend the Hill",
		"What the result earns", "Rendezvous", "The winner", "an Attacker and a Defender"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestResultFormCarriesTheFieldsOfItsHints(t *testing.T) {
	m := editableMatch()
	m.Scenario = &core.MatchScenario{Kind: core.ScenarioRulebook, Name: "Relic of Ages Past"}
	f := matchWrite(t, m)
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return([]core.ScenarioInfo{{Name: "Relic of Ages Past",
		Objectives: []core.ScenarioObjective{{Key: "possessesRelic", Label: "Holds the Relic", Max: 1, Model: true}}}}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	for _, want := range []string{`data-vp`, `data-holds="Holds the Relic"`, `data-company="Rangers"`, `Worth a second look (nothing is blocked):`, `@input="refresh()"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestUnnamedSidesShowTheirCompanies(t *testing.T) {
	m := editableMatch()
	m.Sides[0].Name, m.Sides[1].Name = "Side 1", "Side 2"
	f := matchWrite(t, m)
	body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	if strings.Contains(body, "Side 1") || strings.Contains(body, "Side 2") {
		t.Error("the page still says Side 1 / Side 2")
	}
	f = matchWrite(t, m)
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return(testScenarios, nil)
	body = htmxGet(f, matchPath+"/scenario?mode=pick").Body.String()
	if strings.Contains(body, "Side 1") || !strings.Contains(body, "Raiders") {
		t.Error("the Roll for it panel does not name the companies")
	}
}

func TestRemovingAScenarioAsksFirst(t *testing.T) {
	for kind, want := range map[core.ScenarioKind]string{core.ScenarioCustom: "Remove this one-off scenario?", core.ScenarioRulebook: "Remove the scenario?"} {
		m := editableMatch()
		m.Scenario = &core.MatchScenario{Kind: kind, Name: "Ford"}
		f := matchWrite(t, m)
		if kind == core.ScenarioRulebook {
			f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return([]core.ScenarioInfo{{Name: "Ford"}}, nil)
		}
		body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
		if !strings.Contains(body, "confirm('"+want) {
			t.Errorf("%s: Remove does not ask %q", kind, want)
		}
	}
}

func TestACampaignWithoutTwoCompaniesOffersNoNewMatch(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil).AnyTimes()
	f.camp.EXPECT().Matches(gomock.Any(), int64(7)).Return(nil, nil)
	f.camp.EXPECT().Companies(gomock.Any(), int64(7)).Return(twoCompanies[:1], nil).AnyTimes()
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches", nil))).Body.String()
	if strings.Contains(body, ">New match<") || !strings.Contains(body, "A match needs at least two companies and this campaign has 1.") {
		t.Errorf("list: %s", body)
	}
	body = f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/new", nil))).Body.String()
	if strings.Contains(body, "Create match") || !strings.Contains(body, "A match needs at least two companies and this campaign has 1.") {
		t.Errorf("new match page offers a form")
	}
}

func TestMatchPageShowsScenarioRuleTablesAndMarksTheCombinedRating(t *testing.T) {
	m := editableMatch()
	m.Sides[0].Companies[0].Points = 250
	m.Sides[1].Companies[0].Points = 200 // 450 together
	m.Scenario = &core.MatchScenario{Kind: core.ScenarioRulebook, Name: "Tame the Beast"}
	f := matchWrite(t, m)
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return([]core.ScenarioInfo{{Name: "Tame the Beast",
		About: core.ScenarioAbout{Uses: []string{"Dark of Night"}, Rules: []core.ScenarioText{
			{Title: "There's Always a Bigger Beast", Text: "Rule of thumb:", Table: &core.ScenarioRuleTable{Lookup: "combinedRating", Columns: []string{"Combined Rating", "Monster Budget"},
				Rows: []core.ScenarioTableRow{{Min: 0, Max: 400, Cells: []string{"0-400", "140 points"}}, {Min: 401, Max: 500, Cells: []string{"401-500", "200 points"}}, {Min: 501, Max: -1, Cells: []string{"501+", "300 points"}}}}},
			{Title: "Strange Aura", Text: "Roll.", Table: &core.ScenarioRuleTable{Lookup: "d6", TableDice: core.DiceSpec{Count: 1, Sides: 6}, Columns: []string{"D6", "Result"}, Rows: []core.ScenarioTableRow{{Min: 1, Max: 6, Cells: []string{"1-6", "Fearless"}}}}}}}}}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	for _, want := range []string{"Monster Budget", "200 points", `aria-current="true"`, "add up to 450 rating", "Roll a D6", "Dark of Night"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Count(body, `aria-current="true"`) != 1 {
		t.Error("only the row of the combined rating is marked")
	}
}

func TestRolesCardAndSwap(t *testing.T) {
	m := editableMatch()
	m.Sides[0].Role = "attacker"
	f := matchWrite(t, m)
	body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	for _, want := range []string{`id="roles"`, "Save roles", "Swap roles", `name="role-2"`, "Attacker"} {
		if !strings.Contains(body, want) {
			t.Errorf("match page lacks %q", want)
		}
	}
	swapped := m
	swapped.Sides = []core.MatchSide{m.Sides[0], m.Sides[1]}
	swapped.Sides[0].Role, swapped.Sides[1].Role = "", "attacker"
	f.camp.EXPECT().SwapSideRoles(gomock.Any(), frodo, int64(7), int64(3), 4).Return(swapped, nil)
	rec := f.do(f.signedIn(form(matchPath+"/roles/swap", url.Values{"revision": {"4"}})))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Roles swapped.") {
		t.Errorf("swap: %d", rec.Code)
	}
	f.camp.EXPECT().SetSideRoles(gomock.Any(), frodo, int64(7), int64(3), 4, []string{"defender", ""}).Return(m, nil)
	if rec := f.do(f.signedIn(form(matchPath+"/roles", url.Values{"revision": {"4"}, "role-1": {"defender"}, "role-2": {""}}))); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Roles saved.") {
		t.Errorf("save: %d", rec.Code)
	}
	f.camp.EXPECT().SetSideRoles(gomock.Any(), frodo, int64(7), int64(3), 4, gomock.Any()).Return(core.Match{}, errors.Join(core.ErrInvalidMatch, errors.New("only one side can be the attacker")))
	if rec := f.do(f.signedIn(form(matchPath+"/roles", url.Values{"revision": {"4"}, "role-1": {"attacker"}, "role-2": {"attacker"}}))); rec.Code != 422 || !strings.Contains(rec.Body.String(), "ne side can be the attacker") {
		t.Errorf("invalid: %d", rec.Code)
	}
}

func TestRolesHiddenForAScenarioWithoutRoles(t *testing.T) {
	m := editableMatch()
	m.Sides[0].Role = "attacker"
	m.Scenario = &core.MatchScenario{Kind: core.ScenarioCustom, Name: "Homebrew"}
	f := matchWrite(t, m)
	body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	if strings.Contains(body, `id="roles"`) || strings.Contains(body, "Swap roles") {
		t.Error("a scenario without roles hides the tags")
	}
}

func TestVantagePointIsOfferedAfterTheRollAndMovesTheSecondDie(t *testing.T) {
	m := editableMatch()
	d := campaignDetail()
	d.OrganiserID = 99
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().Match(gomock.Any(), int64(7), m.ID).Return(m, nil).AnyTimes()
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return(testScenarios, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(core.CompanyDetail{Perks: []core.Perk{{ID: 9, Effect: core.PerkVantagePoint}}}, nil).Times(1)
	seq := []int{1, 1}
	f.srv.die = func(int) int { n := seq[0]; seq = seq[1:]; return n }
	body := f.do(f.signedIn(form(matchPath+"/scenario/roll", url.Values{"roll": {"0"}, "roll2": {"0"}}))).Body.String()
	for _, want := range []string{"Vantage Point", `name="vantage" value="1"`, `name="adjust" value="1"`, `name="adjust" value="-1" class="btn btn-secondary !min-h-9" disabled`} {
		if !strings.Contains(body, want) {
			t.Errorf("roll panel lacks %q", want)
		}
	}
	f.camp.EXPECT().UseVantagePoint(gomock.Any(), frodo, int64(7), int64(3), int64(1), 1, 1).Return(2, "1, Vantage Point +1 = 2", nil)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(core.CompanyDetail{}, nil).Times(1)
	rec := f.do(f.signedIn(form(matchPath+"/scenario/roll", url.Values{"roll": {"1"}, "roll2": {"1"}, "vantage": {"1"}, "adjust": {"1"}})))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "1, Vantage Point &#43;1 = 2") || strings.Contains(rec.Body.String(), `name="vantage"`) {
		t.Errorf("adjust: %d %s", rec.Code, rec.Body.String())
	}
}

func TestOrganiserEditsAndReopensAnEndedMatchLikeAnAdministrator(t *testing.T) {
	m := readyToEnd()
	now := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	m.Ended, m.EndedOn, m.Winner = true, &now, 1
	m.Sides[0].Companies[0].OwnerID = 51
	m.Sides[1].Companies[0].OwnerID = 52
	f := newFixture(t, Config{})
	f.aftermaths = []core.AftermathRef{{CompanyID: 1, Status: core.AftermathPending}}
	f.camp.EXPECT().Aftermath(gomock.Any(), gomock.Any(), int64(7), int64(3), gomock.Any()).Return(core.Aftermath{}, nil).AnyTimes()
	d := campaignDetail()
	d.OrganiserID = frodo.ID
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().Match(gomock.Any(), int64(7), int64(3)).Return(m, nil).AnyTimes()
	body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	for _, want := range []string{"Edit the result (organiser or administrator)", "Reopen match", "Save result"} {
		if !strings.Contains(body, want) {
			t.Errorf("the organiser lacks %q", want)
		}
	}
}
