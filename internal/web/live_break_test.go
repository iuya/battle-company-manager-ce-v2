package web

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func brokenState(lc *core.LiveCompany) {
	lc.Turn = 4
	lc.Units[0].CourageTurn = 3 // Hador has not tested this turn
	lc.Break = core.LiveBreak{Percent: 50, StartingModels: 13, Casualties: 7, Threshold100: 650, Broken: true, BrokenAtTurn: 4, Offer: true,
		Exceptions: []string{"Models on the hill do not test."}}
}

func livePage(f *fixture) string {
	return f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1", nil))).Body.String()
}

func TestBreakPanelAndTurnButtonForTheOwner(t *testing.T) {
	f := liveSetupWith(t, core.LiveEdit, func(lc *core.LiveCompany) {
		lc.Turn = 3
		lc.Break = core.LiveBreak{Percent: 50, StartingModels: 13, Casualties: 7, Threshold100: 650, WillBreak: true, Exceptions: []string{"Models on the hill do not test."}}
	})
	body := livePage(f)
	for _, want := range []string{"Turn 3", "Start turn 4", `"kind":"turn","turn":"4"`, "Break Point 6.5 of 13 · Casualties 7 · 50%", "Will break next turn", "will be Broken at the start of the next turn (7 casualties, Break Point 6.5)",
		`"kind":"breakAdjust","field":"start","delta":"-1"`, `"field":"casualties","delta":"1"`, `id="break-note"`, "Models on the hill do not test.", `role="progressbar"`, "Hador"} {
		if !strings.Contains(body, want) {
			t.Errorf("break panel lacks %q", want)
		}
	}
	if strings.Contains(body, `id="live-broken"`) || strings.Contains(body, "Test due") {
		t.Error("not Broken: no banner, no chips")
	}
}

func TestBreakPanelForAViewerHasNoButtons(t *testing.T) {
	f := liveSetupWith(t, core.LiveNone, func(lc *core.LiveCompany) {
		lc.Break = core.LiveBreak{Percent: 66, StartingModels: 13, Casualties: 2, Threshold100: 858}
	})
	body := livePage(f)
	if !strings.Contains(body, "Break Point 8.5 of 13") || !strings.Contains(body, "this company's own rule") || !strings.Contains(body, "Turn not started") {
		t.Error("viewer reads the panel")
	}
	for _, banned := range []string{"Start turn", `"kind":"breakAdjust"`, `id="break-note"`} {
		if strings.Contains(body, banned) {
			t.Errorf("a viewer has no %q", banned)
		}
	}
}

func TestBrokenBannerChipsAndAllIsLostOffer(t *testing.T) {
	f := liveSetupWith(t, core.LiveEdit, brokenState)
	body := livePage(f)
	for _, want := range []string{`id="live-broken"`, "The company is Broken (turn 4)", "Every model tests Courage before it does anything else", "All is Lost: flee or fight on?", `"kind":"allIsLost","flee":"1"`, `"flee":"0"`, "Flee the battlefield",
		"Who has tested this turn", ">Test due<", "Courage test", "/units/5/sheet/courage", "Broken since turn 4"} {
		if !strings.Contains(body, want) {
			t.Errorf("broken page lacks %q", want)
		}
	}
	// once resolved this turn the chip changes and the primary button goes
	g := liveSetupWith(t, core.LiveEdit, func(lc *core.LiveCompany) { brokenState(lc); lc.Units[0].CourageTurn = 4 })
	body = livePage(g)
	if strings.Contains(body, "/units/5/sheet/courage") || !strings.Contains(body, "Courage resolved") {
		t.Error("resolved this turn: chip says so and no Courage test button")
	}
	// the offer closes at the next turn; a viewer cannot decide
	h := liveSetupWith(t, core.LiveEdit, func(lc *core.LiveCompany) { brokenState(lc); lc.Break.Offer = false; lc.Break.Fled = true })
	if body = livePage(h); strings.Contains(body, "Flee the battlefield") || !strings.Contains(body, "fled the battlefield") {
		t.Error("no offer after it closed; the fled company says so")
	}
	v := liveSetupWith(t, core.LiveNone, brokenState)
	if body = livePage(v); strings.Contains(body, "Flee the battlefield") || !strings.Contains(body, "The owner of the company can decide") || !strings.Contains(body, `id="live-broken"`) {
		t.Error("a viewer sees the banner but cannot decide")
	}
}

func TestCourageSheetOffersRollRecordAndPasses(t *testing.T) {
	f := liveSetupWith(t, core.LiveEdit, brokenState)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/3/live/1/units/5/sheet/courage", nil))).Body.String()
	for _, want := range []string{"Courage test: Hador", "/units/5/courage/roll", "Roll 2D6", "Record: passed", "Record: failed", `name="standFast"`, `name="reason" value="standFast"`, "Auto-pass: Heroic Resolve", "Exempt: in combat", "Exempt: cannot cannotActivate"} {
		if want == "Exempt: cannot cannotActivate" {
			want = "Exempt: cannot activate"
		}
		if !strings.Contains(body, want) {
			t.Errorf("courage sheet lacks %q", want)
		}
	}
	if strings.Contains(body, `name="roll"`) {
		t.Error("typed dice only with ManualRolls")
	}
}

func TestCourageRollShowsTheResultAndRefusals(t *testing.T) {
	f := liveSetupWith(t, core.LiveEdit, brokenState)
	f.camp.EXPECT().RollCourage(gomock.Any(), frodo, int64(7), int64(3), int64(5), []int(nil), false).Return(core.CourageRoll{Dice: []int{3, 4}, Total: 7, Courage: 4, Passed: true}, nil)
	rec := f.do(f.signedIn(form("/campaigns/7/matches/3/live/1/units/5/courage/roll", url.Values{"roll": {"0"}, "roll2": {"0"}})))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "against Courage 4") || !strings.Contains(body, "3 &#43; 4") || !strings.Contains(body, ">passed<") || !strings.Contains(body, "Roll again") {
		t.Errorf("roll: %d\n%s", rec.Code, body)
	}
	f.camp.EXPECT().RollCourage(gomock.Any(), frodo, int64(7), int64(3), int64(5), []int{2, 5}, false).Return(core.CourageRoll{}, core.Refuse("Typing in a roll is not allowed in this campaign."))
	rec = f.do(f.signedIn(form("/campaigns/7/matches/3/live/1/units/5/courage/roll", url.Values{"roll": {"2"}, "roll2": {"5"}})))
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "Typing in a roll is not allowed") {
		t.Errorf("refused roll: %d", rec.Code)
	}
}

func TestLiveActionsCarryTheBreakPointFields(t *testing.T) {
	f := liveSetupWith(t, core.LiveEdit, brokenState)
	for _, c := range []struct {
		v    url.Values
		want core.LiveAction
	}{
		{url.Values{"kind": {"turn"}, "turn": {"5"}}, core.LiveAction{Kind: core.LiveTurn, Turn: 5}},
		{url.Values{"kind": {"breakAdjust"}, "field": {"start"}, "delta": {"-1"}, "note": {"Pony"}}, core.LiveAction{Kind: core.LiveBreakAdjust, Field: "start", Delta: -1, Note: "Pony"}},
		{url.Values{"kind": {"courageSkip"}, "unit": {"5"}, "reason": {"engaged"}}, core.LiveAction{Kind: core.LiveCourageSkip, UnitID: 5, Reason: "engaged"}},
		{url.Values{"kind": {"allIsLost"}, "flee": {"1"}}, core.LiveAction{Kind: core.LiveAllIsLost, Flee: true}},
		{url.Values{"kind": {"broken"}, "unit": {"5"}, "standFast": {"1"}}, core.LiveAction{Kind: core.LiveBroken, UnitID: 5, StandFast: true}},
		{url.Values{"kind": {"objective"}, "unit": {"5"}, "on": {"1"}}, core.LiveAction{Kind: core.LiveObjective, UnitID: 5, On: true}},
		{url.Values{"kind": {"objective"}, "unit": {"5"}, "on": {"0"}}, core.LiveAction{Kind: core.LiveObjective, UnitID: 5}},
		{url.Values{"kind": {"itemUsed"}, "unit": {"5"}, "item": {"Dwarven Brew"}, "index": {"1"}}, core.LiveAction{Kind: core.LiveItemUsed, UnitID: 5, Item: "Dwarven Brew", Index: 1}},
	} {
		f.camp.EXPECT().Apply(gomock.Any(), frodo, int64(7), int64(3), int64(1), c.want).Return(core.LiveCompany{}, core.LiveEntry{ID: 41, Text: "done"}, nil)
		if rec := f.do(f.signedIn(form("/campaigns/7/matches/3/live/1/actions", c.v))); rec.Code != 200 {
			t.Errorf("%v: %d", c.want.Kind, rec.Code)
		}
	}
	f.camp.EXPECT().Apply(gomock.Any(), frodo, int64(7), int64(3), int64(1), core.LiveAction{Kind: core.LiveTurn, Turn: 4}).Return(core.LiveCompany{}, core.LiveEntry{}, core.Refuse("Turn 4 has already started."))
	rec := f.do(f.signedIn(form("/campaigns/7/matches/3/live/1/actions", url.Values{"kind": {"turn"}, "turn": {"4"}})))
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "Turn 4 has already started.") {
		t.Errorf("stale turn: %d", rec.Code)
	}
}

func TestLoanedItemsTemporaryWandererAndItemUsed(t *testing.T) {
	f := liveSetupWith(t, core.LiveEdit, func(lc *core.LiveCompany) {
		lc.Wanderer = &core.TemporaryWanderer{UnitID: -1, Name: "Radagast", Summary: "F4/2 Courage 6", Rating: 45, Courage: 6}
		lc.Units = append(lc.Units, core.LiveUnit{UnitID: -1, Zone: core.ZoneField, Temporary: true, Wounds: core.LiveStat{Current: 3, Start: 3}, Might: core.LiveStat{Current: 3, Start: 3}})
		lc.Units[0].Loaned = []core.LoanedItem{
			{UnitID: 5, Item: "Dwarven Brew", Kit: "Raider's Kit", Index: 0},
			{UnitID: 5, Item: "Torching Brand", Kit: "Raider's Kit", Index: 0, Used: true},
			{UnitID: 5, Item: "Healing Herbs", Kit: "Healer's Kit", AfterGame: true}}
		lc.Units[0].UsedItems = []string{"Hand weapon"}
	})
	body := livePage(f)
	for _, want := range []string{`aria-label="Loaned items"`, "Dwarven Brew", "Loaned (Raider&#39;s Kit)", `aria-label="Torching Brand, used"`, "use after the game", "Radagast", "Temporary", "Taken with Against the Odds", "F4/2 Courage 6", "<span class=\"font-semibold text-fg\">45</span> pts"} {
		if !strings.Contains(body, want) {
			t.Errorf("card lacks %q", want)
		}
	}
	if strings.Contains(body, "/units/-1/dismiss") {
		t.Error("the temporary Wanderer cannot be dismissed")
	}
	f.camp.EXPECT().Log(gomock.Any(), frodo, int64(7), int64(3), int64(0), 200).Return(nil, nil).AnyTimes() // the window lists what the unit recorded
	menu := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/3/live/1/units/5/sheet/menu", nil))).Body.String()
	if !strings.Contains(menu, "Item used") {
		t.Error("the menu offers Item used for a unit with an unspent loaned item")
	}
	items := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/3/live/1/units/5/sheet/item", nil))).Body.String()
	for _, want := range []string{`value="itemUsed"`, `name="item" value="Dwarven Brew"`, "Healing Herbs and Wondrous Cram act in the aftermath"} {
		if !strings.Contains(items, want) {
			t.Errorf("item sheet lacks %q", want)
		}
	}
	if strings.Contains(items, `value="Torching Brand"`) || strings.Contains(items, `value="Healing Herbs"`) {
		t.Error("spent and aftermath items are not offered")
	}
	// the temporary Wanderer has its own sheet, and Stand Fast is not offered for it
	sheet := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/3/live/1/units/-1/sheet/broken", nil))).Body.String()
	if !strings.Contains(sheet, "Radagast") || strings.Contains(sheet, "Call Stand Fast") {
		t.Errorf("the Wanderer's sheet has no Stand Fast: %s", sheet)
	}
}

func TestResultFormIsPrefilledFromTheLiveMatch(t *testing.T) {
	m := editableMatch()
	m.Scenario = &core.MatchScenario{Kind: core.ScenarioRulebook, Name: "Hold"}
	f2 := matchWrite(t, m)
	f2.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return([]core.ScenarioInfo{{Name: "Hold", Objectives: []core.ScenarioObjective{{Key: "enemyBroken", Label: "Enemy Broken", Max: 1}}}}, nil).AnyTimes()
	f2.liveDefaults = &core.LiveMatchDefaults{Fled: map[int64]bool{1: true}, EnemyBroken: map[int64]bool{2: true}}
	body := f2.do(f2.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	for _, want := range []string{`<span class="badge badge-gold">From the live match</span>`, "The other side claims victory (All is Lost)", `name="fled-1" value="1"`, `&#34;fled&#34;:true`, "Every opposing company was Broken at the end of the game"} {
		if !strings.Contains(body, want) {
			t.Errorf("result form lacks %q", want)
		}
	}
	if n := strings.Count(body, "Every opposing company was Broken"); n != 1 {
		t.Errorf("the enemyBroken tick is prefilled once (for company 2), got %d", n)
	}
	// without live data nothing is prefilled
	g := matchWrite(t, m)
	g.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return([]core.ScenarioInfo{{Name: "Hold", Objectives: []core.ScenarioObjective{{Key: "enemyBroken", Label: "Enemy Broken", Max: 1}}}}, nil).AnyTimes()
	if body := g.do(g.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String(); strings.Contains(body, "Every opposing company was Broken") || strings.Contains(body, "badge badge-gold\">From the live match") {
		t.Error("no live data, no prefill")
	}
}

func TestCourageChipSaysHowItWasResolved(t *testing.T) {
	for how, want := range map[string]string{"passed": "Tested: passed", "failed": "Tested: failed", "heroicResolve": "Auto-pass", "scenario": "Auto-pass", "engaged": "Exempt", "cannotActivate": "Exempt", "": "Courage resolved"} {
		f := liveSetupWith(t, core.LiveEdit, func(lc *core.LiveCompany) { brokenState(lc); lc.Units[0].CourageTurn = 4; lc.Units[0].CourageHow = how })
		if body := livePage(f); !strings.Contains(body, ">"+want+"<") {
			t.Errorf("how %q: chip lacks %q", how, want)
		}
	}
}

func TestCompanyPageShowsItsBreakPointRule(t *testing.T) {
	f := liveSetupWith(t, core.LiveNone, nil)
	if body := livePage(f); strings.Contains(body, "company-break-point") {
		t.Error("no percent known: no line")
	}
	g := newFixture(t, Config{})
	d, detail := mineDetail()
	detail.BreakPointPercent = 66
	g.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	g.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	g.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	g.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	g.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	g.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	if body := g.do(g.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1", nil))).Body.String(); !strings.Contains(body, "Break Point: 66% of the starting models") {
		t.Error("the company's Break Point rule is shown")
	}
}
