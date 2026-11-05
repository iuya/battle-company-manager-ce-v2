package web

import (
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func atoCard() core.ATO {
	return core.ATO{MatchID: 3, CompanyID: 1, CompanyName: "Rangers", Eligible: true, OwnRating: 240, OppRating: 325, OppName: "Raiders", Gap: 85, Revision: 4, Access: core.ATOEdit, Payable: true, Pending: true, HasScenario: true,
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
	body := htmxGet(f, testMatchPath+"/ato").Body.String()
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
	c.Spent, c.Decided, c.Pending = 45, true, false
	f.camp.EXPECT().AgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3)).Return(c, nil)
	body := htmxGet(f, testMatchPath+"/ato").Body.String()
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
	if body := htmxGet(f2, testMatchPath+"/ato").Body.String(); !strings.Contains(body, "The game has started. The choice can no longer be changed.") {
		t.Error("locked reason")
	}
}

func TestATOCardNotEligibleAndHiddenStates(t *testing.T) {
	f := atoFixture(t)
	f.camp.EXPECT().AgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3)).Return(core.ATO{Why: "Against the Odds: nobody. Both companies are rated 240."}, nil)
	if body := htmxGet(f, testMatchPath+"/ato").Body.String(); !strings.Contains(body, "nobody. Both companies are rated 240.") || strings.Contains(body, "<form") {
		t.Error("not eligible: one line, no controls")
	}
	// an ended match with nothing chosen shows nothing
	f2 := matchWrite(t, endedMatch())
	c := atoCard()
	c.Access = core.ATONone
	f2.camp.EXPECT().AgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3)).Return(c, nil)
	if body := htmxGet(f2, testMatchPath+"/ato").Body.String(); strings.Contains(body, "Against the Odds") {
		t.Errorf("ended and unchosen: hidden, got %s", body)
	}
}

func TestATOSaveSendsTheWholeSelectionAndAnswersTheMatch(t *testing.T) {
	f := atoFixture(t)
	f.camp.EXPECT().AgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3)).Return(atoCard(), nil)
	want := core.ATOSelection{Bonuses: []string{"toolKit", "ambush"}, Kits: []string{"healer"}, Ambush: "Rescue", Items: []core.KitAssignment{
		{Kit: "healer", Item: "Healing Herbs", Index: 1, UnitID: 10}, {Kit: "healer", Item: "Envenom Weapon (X)", Index: 0, UnitID: 11, Detail: "Elven blade"}}}
	f.camp.EXPECT().SaveAgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3), 4, want).Return(atoCard(), nil)
	r := form(testMatchPath+"/ato", url.Values{"revision": {"4"}, "bonus": {"toolKit", "ambush"}, "kit": {"healer"}, "ambush": {"Rescue"}, "wanderer": {"Radagast"},
		"assign-healer-0-0": {"0"}, "assign-healer-0-1": {"10"}, "assign-healer-1-0": {"11"}, "detail-healer-1-0": {" Elven blade "}})
	r.Header.Set("HX-Request", "true")
	rec := f.do(f.signedIn(r))
	if rec.Code != 200 || rec.Header().Get("HX-Retarget") != "#match" || !strings.Contains(rec.Body.String(), "Against the Odds confirmed.") || !strings.Contains(rec.Body.String(), `id="match"`) {
		t.Errorf("save: %d %v", rec.Code, rec.Header())
	}
}

func TestATOSaveRefusalsComeBackOnTheCard(t *testing.T) {
	f := atoFixture(t)
	f.camp.EXPECT().AgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3)).Return(atoCard(), nil).Times(4)
	f.camp.EXPECT().SaveAgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3), 4, gomock.Any()).Return(core.ATO{}, core.Refuse("The game has started.")).Times(1)
	f.camp.EXPECT().SaveAgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3), 4, gomock.Any()).Return(core.ATO{}, core.ErrMatchChanged).Times(1)
	post := func() *httptest.ResponseRecorder {
		r := form(testMatchPath+"/ato", url.Values{"revision": {"4"}, "bonus": {"reroll"}})
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
	r := form(testMatchPath+"/ato", url.Values{"revision": {"4"}, "bonus": {"reroll"}})
	r.Header.Set("HX-Request", "true")
	if rec := f.do(f.signedIn(r)); rec.Code != 403 {
		t.Errorf("viewer write: %d", rec.Code)
	}
}

// The card by where the decision stands (docs/matches.md, "Against the Odds before the game").
func TestATOCardConfirmWaitAndChange(t *testing.T) {
	card := func(edit func(*core.ATO)) string {
		f := atoFixture(t)
		c := atoCard()
		edit(&c)
		f.camp.EXPECT().AgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3)).Return(c, nil)
		return htmxGet(f, testMatchPath+"/ato").Body.String()
	}
	body := card(func(*core.ATO) {})
	for _, want := range []string{"data-ato-confirm", "Play without bonuses", `hx-sync="#ato-slot:replace"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the lower player's card lacks %q", want)
		}
	}
	if strings.Contains(body, ">Save<") || strings.Contains(body, "Clear") || strings.Contains(body, "disabled>Play") {
		t.Errorf("one button, enabled: %s", body)
	}
	body = card(func(c *core.ATO) { c.HasScenario = false })
	if !strings.Contains(body, "Choose the scenario first") || !regexp.MustCompile(`data-ato-confirm disabled`).MatchString(body) {
		t.Error("without a scenario nothing can be confirmed")
	}
	body = card(func(c *core.ATO) { c.Access = core.ATONone })
	if !strings.Contains(body, "Waiting for Rangers to choose") || strings.Contains(body, "<form") {
		t.Errorf("the rival waits, read only: %s", body)
	}
	body = card(func(c *core.ATO) { c.Decided, c.Pending = true, false })
	if !strings.Contains(body, "Rangers took no bonus.") || !strings.Contains(body, "data-ato-change") || strings.Contains(body, "data-ato-confirm") {
		t.Errorf("confirmed: the summary and Change: %s", body)
	}
	body = card(func(c *core.ATO) { c.Decided, c.Pending, c.Access = true, false, core.ATONone })
	if strings.Contains(body, "data-ato-change") {
		t.Error("the rival cannot change it")
	}
	body = card(func(c *core.ATO) { c.Payable, c.Pending, c.Gap = false, false, 5 })
	if !strings.Contains(body, "A gap of 5 pays for no bonus") || strings.Contains(body, "<form") {
		t.Errorf("nothing to choose: %s", body)
	}
}

func TestATOChangeAnswersTheMatchAndARefusalTheCard(t *testing.T) {
	f := atoFixture(t)
	f.camp.EXPECT().ReopenAgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3), 4).Return(atoCard(), nil)
	r := form(testMatchPath+"/ato/change", url.Values{"revision": {"4"}})
	r.Header.Set("HX-Request", "true")
	if rec := f.do(f.signedIn(r)); rec.Code != 200 || rec.Header().Get("HX-Retarget") != "#match" {
		t.Errorf("change: %d %v", rec.Code, rec.Header())
	}
	g := atoFixture(t)
	g.camp.EXPECT().ReopenAgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3), 4).Return(core.ATO{}, core.Refuse("The game has started: Against the Odds can no longer change."))
	g.camp.EXPECT().AgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3)).Return(atoCard(), nil)
	r = form(testMatchPath+"/ato/change", url.Values{"revision": {"4"}})
	r.Header.Set("HX-Request", "true")
	if rec := g.do(g.signedIn(r)); rec.Code != 422 || !strings.Contains(rec.Body.String(), "can no longer change") || !strings.Contains(rec.Body.String(), `id="ato"`) {
		t.Errorf("refused: %d", rec.Code)
	}
}

func TestPendingDecisionDisablesEndMatchWithTheReason(t *testing.T) {
	f := matchWriteOwning(t, readyToEnd())
	f.atoStatus = core.ATOStatus{Pending: true, CompanyName: "Raiders", OwnerID: 50}
	body := f.do(f.signedIn(httptest.NewRequest("GET", testMatchPath, nil))).Body.String()
	if !strings.Contains(body, "Waiting for Raiders to choose Against the Odds.") || !strings.Contains(body, `id="end-reason"`) {
		t.Errorf("End match says what it waits for")
	}
}

func TestAmbushFoldsTheScenarioCardToReadOnly(t *testing.T) {
	m := editableMatch()
	m.Scenario = &core.MatchScenario{Kind: core.ScenarioRulebook, Name: "Rescue"}
	f := matchWriteOwning(t, m)
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return(testScenarios, nil).AnyTimes()
	f.atoStatus = core.ATOStatus{Ambush: "Rescue"}
	body := f.do(f.signedIn(httptest.NewRequest("GET", testMatchPath, nil))).Body.String()
	if !strings.Contains(body, "Ambush! chose this scenario") || strings.Contains(body, "data-change-scenario") {
		t.Errorf("the scenario changes only from the card")
	}
}

func TestScenarioChangeSaysTheDecisionReopened(t *testing.T) {
	m := editableMatch()
	m.ATODecided = true
	f := matchWrite(t, m)
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return(testScenarios, nil).AnyTimes()
	done := editableMatch()
	done.Scenario = &core.MatchScenario{Kind: core.ScenarioRulebook, Name: "Rescue"}
	f.camp.EXPECT().SetScenario(gomock.Any(), frodo, int64(7), int64(3), 4, gomock.Any()).Return(done, nil)
	r := form(testMatchPath+"/scenario", url.Values{"revision": {"4"}, "kind": {"rulebook"}, "name": {"Rescue"}})
	r.Header.Set("HX-Request", "true")
	if body := f.do(f.signedIn(r)).Body.String(); !strings.Contains(body, "Against the Odds has to be confirmed again.") {
		t.Errorf("the notice is missing")
	}
}

func TestYourTurnAsksTheLowerRatedOwnerToChoose(t *testing.T) {
	f := newFixture(t, Config{})
	d := campaignDetail()
	d.Companies = []core.CampaignCompany{{ID: 1, Name: "Rangers", OwnerID: frodo.ID}}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().Companies(gomock.Any(), int64(7)).Return(twoCompanies, nil).AnyTimes()
	f.camp.EXPECT().Matches(gomock.Any(), int64(7)).Return([]core.Match{editableMatch()}, nil).AnyTimes()
	f.atoStatus = core.ATOStatus{Pending: true, CompanyName: "Rangers", OwnerID: frodo.ID}
	for _, path := range []string{"/campaigns/7/matches", "/campaigns/7"} {
		if body := f.do(f.signedIn(httpGet(path))).Body.String(); !strings.Contains(body, "Choose Against the Odds for Match 3") {
			t.Errorf("%s lacks the line", path)
		}
	}
	f.atoStatus.OwnerID = 50 // the rival's: nothing waits on this viewer
	if body := f.do(f.signedIn(httpGet("/campaigns/7/matches"))).Body.String(); strings.Contains(body, "Choose Against the Odds") {
		t.Error("the rival has nothing to choose")
	}
}

// While the match waits, End match of the live banner is disabled and the banner says why; its refresh brings the button back out of band.
func TestLiveBannerEndMatchWaitsForTheDecision(t *testing.T) {
	f := matchWriteOwning(t, editableMatch())
	f.atoStatus = core.ATOStatus{Pending: true, CompanyName: "Raiders", OwnerID: 50}
	body := htmxGet(f, testMatchPath+"/live/end/banner?company=1").Body.String()
	for _, want := range []string{"data-ato-banner", "Waiting for Raiders to choose Against the Odds.", "See the match page", `id="live-end-button"`, `hx-swap-oob="true"`, "disabled title="} {
		if !strings.Contains(body, want) {
			t.Errorf("the banner lacks %q: %s", want, body)
		}
	}
	f.atoStatus = core.ATOStatus{}
	body = htmxGet(f, testMatchPath+"/live/end/banner?company=1").Body.String()
	if strings.Contains(body, "data-ato-banner") || strings.Contains(body, "disabled title=") || !strings.Contains(body, "data-end-match") {
		t.Errorf("decided: End match is back: %s", body)
	}
}
