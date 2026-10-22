package web

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func atoCard() core.ATO {
	return core.ATO{MatchID: 3, CompanyID: 1, CompanyName: "Rangers", Eligible: true, OwnRating: 240, OppRating: 325, OppName: "Raiders", Gap: 85, Revision: 4, Access: core.ATOEdit,
		Options: []core.ATOOption{
			{Key: "reroll", Name: "Reroll", Text: "Two rerolls.", Rating: 15},
			{Key: "toolKit", Name: "Tool Kit", Text: "A kit.", Rating: 30},
			{Key: "wanderer", Name: "Wanderer", Text: "A Wanderer.", Rating: 45, Disabled: true, Reason: "The company already has a Wanderer."},
			{Key: "ambush", Name: "Ambush!", Text: "Choose the scenario.", Rating: 60}},
		Kits:      []core.ATOKit{{Key: "healer", Name: "Healer's Kit", Items: []core.ATOKitItem{{Item: "Healing Herbs", Count: 2, HeroOnly: true}, {Item: "Envenom Weapon (X)", Count: 1, Detail: true}}}},
		Wanderers: []core.ATOWandererRule{{Name: "Radagast", Summary: "Might 3", OwnPrice: 100}},
		Models:    []core.ATOModel{{UnitID: 10, Name: "Aldor", Type: "Captain", Hero: true}, {UnitID: 11, Name: "Brand", Type: "Warrior"}}}
}

func atoFixture(t *testing.T) *fixture {
	f := matchWrite(t, editableMatch())
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return(testScenarios, nil).AnyTimes()
	return f
}

func TestATOCardForTheLowerCompanyShowsBudgetRowsAndAssignments(t *testing.T) {
	f := atoFixture(t)
	f.camp.EXPECT().AgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3)).Return(atoCard(), nil)
	body := htmxGet(f, matchPath+"/ato").Body.String()
	for _, want := range []string{`id="ato"`, "Rangers is <strong>85</strong> below Raiders (240 vs 325)", "Spent <span x-text=\"spent\"></span> / 85", `name="bonus" value="toolKit"`, "The company already has a Wanderer.",
		`name="kit" value="healer"`, `name="assign-healer-0-1"`, "Heroes only", "Not given to anybody yet", `name="detail-healer-1-0"`, `name="wanderer" value="Radagast"`, `name="ambush"`, "Seize Ground", `name="revision" value="4"`, "No bonus chosen."} {
		if !strings.Contains(body, want) {
			t.Errorf("card lacks %q", want)
		}
	}
	// Brand is not a Hero: the Herbs selects list Aldor only.
	i := strings.Index(body, `id="assign-healer-0-0"`)
	sel := body[i : i+strings.Index(body[i:], "</select>")]
	if !strings.Contains(sel, "Aldor") || strings.Contains(sel, "Brand") {
		t.Errorf("a Hero-only item lists Heroes only: %s", sel)
	}
}

func TestATOCardIsReadOnlyForTheOpponentAndWhenLocked(t *testing.T) {
	f := atoFixture(t)
	c := atoCard()
	c.Access = core.ATONone
	c.Selection = core.ATOSelection{Bonuses: []string{"reroll", "toolKit"}, Kits: []string{"healer"}, Items: []core.KitAssignment{{Kit: "healer", Item: "Healing Herbs", Index: 0, UnitID: 10}}}
	c.Options[0].Taken, c.Options[1].Taken = true, true
	c.Spent = 45
	f.camp.EXPECT().AgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3)).Return(c, nil)
	body := htmxGet(f, matchPath+"/ato").Body.String()
	for _, want := range []string{"Rangers took: Reroll; Tool Kit (Healer&#39;s Kit).", "Spent 45 of 85", "Healing Herbs goes to Aldor", "Only the lower rated company chooses bonuses."} {
		if !strings.Contains(body, want) {
			t.Errorf("read only card lacks %q", want)
		}
	}
	if strings.Contains(body, "<form") {
		t.Error("no form for a viewer")
	}
	c.Locked, c.LockedWhy = true, "The game has started."
	f2 := atoFixture(t)
	f2.camp.EXPECT().AgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3)).Return(c, nil)
	if body := htmxGet(f2, matchPath+"/ato").Body.String(); !strings.Contains(body, "The game has started. The choice can no longer be changed.") {
		t.Error("locked reason")
	}
}

func TestATOCardNotEligibleAndHiddenStates(t *testing.T) {
	f := atoFixture(t)
	f.camp.EXPECT().AgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3)).Return(core.ATO{Why: "Against the Odds: nobody. Both companies are rated 240."}, nil)
	if body := htmxGet(f, matchPath+"/ato").Body.String(); !strings.Contains(body, "nobody. Both companies are rated 240.") || strings.Contains(body, "<form") {
		t.Error("not eligible: one line, no controls")
	}
	// an ended match with nothing chosen shows nothing
	f2 := matchWrite(t, endedMatch())
	c := atoCard()
	c.Access = core.ATONone
	f2.camp.EXPECT().AgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3)).Return(c, nil)
	if body := htmxGet(f2, matchPath+"/ato").Body.String(); strings.Contains(body, "Against the Odds") {
		t.Errorf("ended and unchosen: hidden, got %s", body)
	}
}

func TestATOSaveSendsTheWholeSelectionAndAnswersTheMatch(t *testing.T) {
	f := atoFixture(t)
	f.camp.EXPECT().AgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3)).Return(atoCard(), nil)
	want := core.ATOSelection{Bonuses: []string{"toolKit", "ambush"}, Kits: []string{"healer"}, Ambush: "Rescue", Items: []core.KitAssignment{
		{Kit: "healer", Item: "Healing Herbs", Index: 1, UnitID: 10}, {Kit: "healer", Item: "Envenom Weapon (X)", Index: 0, UnitID: 11, Detail: "Elven blade"}}}
	f.camp.EXPECT().SaveAgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3), 4, want).Return(atoCard(), nil)
	r := form(matchPath+"/ato", url.Values{"revision": {"4"}, "bonus": {"toolKit", "ambush"}, "kit": {"healer"}, "ambush": {"Rescue"}, "wanderer": {"Radagast"},
		"assign-healer-0-0": {"0"}, "assign-healer-0-1": {"10"}, "assign-healer-1-0": {"11"}, "detail-healer-1-0": {" Elven blade "}})
	r.Header.Set("HX-Request", "true")
	rec := f.do(f.signedIn(r))
	if rec.Code != 200 || rec.Header().Get("HX-Retarget") != "#match" || !strings.Contains(rec.Body.String(), "Against the Odds saved.") || !strings.Contains(rec.Body.String(), `id="match"`) {
		t.Errorf("save: %d %v", rec.Code, rec.Header())
	}
}

func TestATOSaveRefusalsComeBackOnTheCard(t *testing.T) {
	f := atoFixture(t)
	f.camp.EXPECT().AgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3)).Return(atoCard(), nil).Times(4)
	f.camp.EXPECT().SaveAgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3), 4, gomock.Any()).Return(core.ATO{}, core.Refuse("The game has started.")).Times(1)
	f.camp.EXPECT().SaveAgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3), 4, gomock.Any()).Return(core.ATO{}, core.ErrMatchChanged).Times(1)
	post := func() *httptest.ResponseRecorder {
		r := form(matchPath+"/ato", url.Values{"revision": {"4"}, "bonus": {"reroll"}})
		r.Header.Set("HX-Request", "true")
		return f.do(f.signedIn(r))
	}
	if rec := post(); rec.Code != 422 || !strings.Contains(rec.Body.String(), "The game has started.") || !strings.Contains(rec.Body.String(), `id="ato"`) {
		t.Errorf("refused: %d", rec.Code)
	}
	if rec := post(); !strings.Contains(rec.Body.String(), "Somebody changed this match") {
		t.Errorf("stale: %d", rec.Code)
	}
}

func TestATOSaveIsForbiddenToViewers(t *testing.T) {
	f := atoFixture(t)
	c := atoCard()
	c.Access = core.ATONone
	f.camp.EXPECT().AgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3)).Return(c, nil)
	r := form(matchPath+"/ato", url.Values{"revision": {"4"}, "bonus": {"reroll"}})
	r.Header.Set("HX-Request", "true")
	if rec := f.do(f.signedIn(r)); rec.Code != 403 {
		t.Errorf("viewer write: %d", rec.Code)
	}
}
