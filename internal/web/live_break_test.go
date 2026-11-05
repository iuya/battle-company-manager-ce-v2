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
	lc.Break = core.LiveBreak{Percent: 50, StartingModels: 13, Casualties: 7, Threshold100: 650, Broken: true,
		Exceptions: []string{"Models on the hill do not test."}}
}

func livePage(f *fixture) string {
	return f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1", nil))).Body.String()
}

func TestBreakPanelForTheOwnerHasNoTurns(t *testing.T) {
	f := liveSetupWith(t, core.LiveEdit, func(lc *core.LiveCompany) {
		lc.Break = core.LiveBreak{Percent: 50, StartingModels: 13, Casualties: 6, Threshold100: 650, Exceptions: []string{"Models on the hill do not test."}}
	})
	body := livePage(f)
	for _, want := range []string{"1 more to Break", "The company breaks at 7 casualties: more than 50% of its 13 starting models", `"kind":"breakAdjust","field":"start","delta":"-1"`, `"field":"casualties","delta":"1"`, `id="break-note"`,
		"Models on the hill do not test.", `aria-label="6 casualties of 13 starting models"`, "Hador"} {
		if !strings.Contains(body, want) {
			t.Errorf("break panel lacks %q", want)
		}
	}
	for _, banned := range []string{"6.5 of", `data-end-mark`, "to 0%", `id="live-broken"`, "Test due", "Start turn", "Turn not started", `"kind":"turn"`, "Will break"} {
		if strings.Contains(body, banned) {
			t.Errorf("not Broken and no turns: no %q", banned)
		}
	}
}

func TestBreakPanelForAViewerHasNoButtons(t *testing.T) {
	f := liveSetupWith(t, core.LiveNone, func(lc *core.LiveCompany) {
		lc.Break = core.LiveBreak{Percent: 66, StartingModels: 13, Casualties: 2, Threshold100: 858}
	})
	body := livePage(f)
	if !strings.Contains(body, "7 more to Break") || !strings.Contains(body, "this company's own rule") {
		t.Error("viewer reads the panel")
	}
	for _, banned := range []string{`"kind":"breakAdjust"`, `id="break-note"`} {
		if strings.Contains(body, banned) {
			t.Errorf("a viewer has no %q", banned)
		}
	}
}

// The scenario's 25%: whole models only, shown when the scenario ends there, and models that left the table bring neither it nor the Break closer.
func TestBreakPanelCountsToTheScenarioEnd(t *testing.T) {
	f := liveSetupWith(t, core.LiveEdit, func(lc *core.LiveCompany) {
		lc.Break = core.LiveBreak{Percent: 50, StartingModels: 15, Casualties: 3, LeftTable: 2, Threshold100: 750, EndLossPercent: 75}
	})
	body := livePage(f)
	for _, want := range []string{"5 more to Break", "9 more to 25%", `data-end-mark`, "This scenario ends at 12 casualties, with the company down to 25% of its 15 starting models", "models that left the board do not (2 so far)", `aria-label="3 casualties of 15 starting models, 2 left the table"`,
		"left: 46.67%" /* the 8th casualty breaks */, "left: 73.33%" /* the 12th casualty reaches it, whoever left the table */} {
		if !strings.Contains(body, want) {
			t.Errorf("panel lacks %q", want)
		}
	}
	if strings.Contains(body, "7.5 of") {
		t.Error("no half models on the panel")
	}
	if got := strings.Count(body, "break-cell-left"); got != 2 {
		t.Errorf("%d cells for models that left the table, want 2", got)
	}
	g := liveSetupWith(t, core.LiveEdit, func(lc *core.LiveCompany) {
		lc.Break = core.LiveBreak{Percent: 50, StartingModels: 4, Casualties: 3, Threshold100: 200, Broken: true, EndLossPercent: 75}
	})
	body = livePage(g)
	for _, want := range []string{">Broken<", "25% reached", "break-mark-top", "break-mark-bottom"} {
		if !strings.Contains(body, want) {
			t.Errorf("small company reaching both lacks %q", want)
		}
	}
}

func TestBrokenBannerAndChipsWithoutAllIsLost(t *testing.T) {
	f := liveSetupWith(t, core.LiveEdit, brokenState)
	body := livePage(f)
	for _, want := range []string{`id="live-broken"`, "The company is Broken</h3>", "Every model tests Courage before it activates, each turn. Models in combat, or that cannot activate, do not test.</p>", `class="courage-failed"`, "boot-prints", `hx-vals='{"kind":"retreat","why":"courage","unit":"5"}'`, `<span class="sr-only"> (Hador)</span>`} {
		if !strings.Contains(body, want) {
			t.Errorf("broken page lacks %q", want)
		}
	}
	for _, banned := range []string{"Test due", "Flee the battlefield", `"kind":"allIsLost"`, "Who has tested this turn", "since turn", "/sheet/courage", ">Courage test<"} {
		if strings.Contains(body, banned) {
			t.Errorf("no All is Lost offer and no turns: %q", banned)
		}
	}
	// once tested the chip says how, and the mark stays offered (the test is taken again every turn)
	g := liveSetupWith(t, core.LiveEdit, func(lc *core.LiveCompany) { brokenState(lc); lc.Units[0].CourageHow = "passed" })
	body = livePage(g)
	if !strings.Contains(body, `class="courage-failed"`) || !strings.Contains(body, ">Tested: passed<") {
		t.Error("tested: the chip says so and the failed Courage mark is still there")
	}
	v := liveSetupWith(t, core.LiveNone, brokenState)
	if body = livePage(v); !strings.Contains(body, `id="live-broken"`) || strings.Contains(body, `class="courage-failed"`) {
		t.Error("a viewer sees the banner but cannot mark the test")
	}
}

// The mark posts the retreat straight away: no window and no confirmation. Outside a Broken company it is not offered.
func TestFailedCourageMarkRecordsTheRetreat(t *testing.T) {
	f := liveSetupWith(t, core.LiveEdit, brokenState)
	body := livePage(f)
	if mark := body[strings.Index(body, `class="courage-failed"`):strings.Index(body, "Failed Courage<")]; strings.Contains(mark, "hx-confirm") || strings.Contains(mark, "openModal") {
		t.Errorf("the mark asks for nothing: %s", mark)
	}
	f.camp.EXPECT().Apply(gomock.Any(), frodo, int64(7), int64(3), int64(1), core.LiveAction{Kind: core.LiveRetreat, UnitID: 5, Why: "courage"}).Return(core.LiveCompany{}, core.LiveEntry{ID: 41, Text: "done"}, nil)
	if rec := f.do(f.signedIn(form("/campaigns/7/matches/3/live/1/actions", url.Values{"kind": {"retreat"}, "why": {"courage"}, "unit": {"5"}}))); rec.Code != 200 {
		t.Errorf("the mark's action: %d", rec.Code)
	}
	if body := livePage(liveSetupWith(t, core.LiveEdit, nil)); strings.Contains(body, `class="courage-failed"`) {
		t.Error("a company that is not Broken has no mark")
	}
}

func TestCourageWindowIsGone(t *testing.T) {
	f := liveSetupWith(t, core.LiveEdit, brokenState)
	if rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/3/live/1/units/5/sheet/courage", nil))); rec.Code != 404 {
		t.Errorf("the sheet: %d", rec.Code)
	}
	if rec := f.do(f.signedIn(form("/campaigns/7/matches/3/live/1/units/5/courage/roll", url.Values{"roll": {"3"}, "roll2": {"4"}}))); rec.Code != 404 {
		t.Errorf("the roll: %d", rec.Code)
	}
}

func TestLiveActionsCarryTheBreakPointFields(t *testing.T) {
	f := liveSetupWith(t, core.LiveEdit, brokenState)
	for _, c := range []struct {
		v    url.Values
		want core.LiveAction
	}{
		{url.Values{"kind": {"breakAdjust"}, "field": {"start"}, "delta": {"-1"}, "note": {"Pony"}}, core.LiveAction{Kind: core.LiveBreakAdjust, Field: "start", Delta: -1, Note: "Pony"}},
		{url.Values{"kind": {"broken"}, "unit": {"5"}}, core.LiveAction{Kind: core.LiveBroken, UnitID: 5}},
		{url.Values{"kind": {"objective"}, "unit": {"5"}, "on": {"1"}}, core.LiveAction{Kind: core.LiveObjective, UnitID: 5, On: true}},
		{url.Values{"kind": {"objective"}, "unit": {"5"}, "on": {"0"}}, core.LiveAction{Kind: core.LiveObjective, UnitID: 5}},
		{url.Values{"kind": {"itemUsed"}, "unit": {"5"}, "item": {"Dwarven Brew"}, "index": {"1"}}, core.LiveAction{Kind: core.LiveItemUsed, UnitID: 5, Item: "Dwarven Brew", Index: 1}},
	} {
		f.camp.EXPECT().Apply(gomock.Any(), frodo, int64(7), int64(3), int64(1), c.want).Return(core.LiveCompany{}, core.LiveEntry{ID: 41, Text: "done"}, nil)
		if rec := f.do(f.signedIn(form("/campaigns/7/matches/3/live/1/actions", c.v))); rec.Code != 200 {
			t.Errorf("%v: %d", c.want.Kind, rec.Code)
		}
	}
	for _, kind := range []string{"turn", "allIsLost"} { // there are no turns and no All is Lost action any more
		rec := f.do(f.signedIn(form("/campaigns/7/matches/3/live/1/actions", url.Values{"kind": {kind}, "turn": {"4"}, "flee": {"1"}})))
		if rec.Code != 422 || !strings.Contains(rec.Body.String(), "Unknown action.") {
			t.Errorf("%s: %d", kind, rec.Code)
		}
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
	f.camp.EXPECT().UnitLog(gomock.Any(), frodo, int64(7), int64(3), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes() // the window lists what the unit recorded
	items := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/3/live/1/units/5/sheet/item", nil))).Body.String()
	for _, want := range []string{`value="itemUsed"`, `name="item" value="Dwarven Brew"`, "Healing Herbs and Wondrous Cram act in the aftermath"} {
		if !strings.Contains(items, want) {
			t.Errorf("item sheet lacks %q", want)
		}
	}
	if strings.Contains(items, `value="Torching Brand"`) || strings.Contains(items, `value="Healing Herbs"`) {
		t.Error("spent and aftermath items are not offered")
	}
}

func TestCourageChipSaysHowItWasResolved(t *testing.T) {
	for how, want := range map[string]string{"passed": "Tested: passed", "failed": "Tested: failed"} {
		f := liveSetupWith(t, core.LiveEdit, func(lc *core.LiveCompany) { brokenState(lc); lc.Units[0].CourageHow = how })
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

// live.js finds the +/- buttons again by id to give them the focus back after a swap of the zones, and the server sends the panel closed.
func TestBreakPanelButtonsSurviveTheSwap(t *testing.T) {
	f := liveSetupWith(t, core.LiveEdit, func(lc *core.LiveCompany) {
		lc.Break = core.LiveBreak{Percent: 50, StartingModels: 13, Casualties: 2, Threshold100: 650}
	})
	body := livePage(f)
	for _, id := range []string{"break-start-less", "break-start-more", "break-casualties-less", "break-casualties-more"} {
		if strings.Count(body, `id="`+id+`"`) != 1 {
			t.Errorf("button %s: want one with that id", id)
		}
	}
	if !strings.Contains(body, `id="live-break-panel"`) || strings.Contains(body, `id="live-break-panel" open`) {
		t.Error("the server sends the panel closed: only the page knows how the user left it")
	}
}
