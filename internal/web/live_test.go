package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// liveOwnerSetup serves frodo's company 1 in the open Match 3 with units 5 (field), 6 (retreated), 7 (casualty) and 8 (missed).
func liveOwnerSetup(t *testing.T, access core.LiveAccess, sides ...core.MatchSide) *fixture {
	t.Helper()
	return liveSetupWith(t, access, nil, sides...)
}

// liveSetupWith is liveOwnerSetup with a change to the live state the mock serves.
func liveSetupWith(t *testing.T, access core.LiveAccess, change func(*core.LiveCompany), sides ...core.MatchSide) *fixture {
	t.Helper()
	return liveSetupMatch(t, access, change, nil, sides...)
}

// liveSetupMatch also changes the match the mock serves (a pending end, say).
func liveSetupMatch(t *testing.T, access core.LiveAccess, change func(*core.LiveCompany), matchChange func(*core.Match), sides ...core.MatchSide) *fixture {
	t.Helper()
	return liveSetupRoster(t, access, change, matchChange, nil, sides...)
}

// liveSetupRoster is liveSetupMatch with its own roster for the viewer's company (nil: Hador, Aldor, Baran and Cirion).
func liveSetupRoster(t *testing.T, access core.LiveAccess, change func(*core.LiveCompany), matchChange func(*core.Match), roster []core.CompanyUnit, sides ...core.MatchSide) *fixture {
	t.Helper()
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	detail.OpenMatch = &core.OpenMatchRef{MatchID: 3, Number: 3}
	detail.Roster = []core.CompanyUnit{
		{ID: 5, Name: "Hador", Type: "Warrior", Role: core.RoleSergeant, Kind: core.KindInfantry, Points: 8},
		{ID: 6, Name: "Aldor", Type: "Warrior", Role: core.RoleWarrior, Kind: core.KindInfantry, Points: 8},
		{ID: 7, Name: "Baran", Type: "Warrior", Role: core.RoleWarrior, Kind: core.KindInfantry, Points: 8},
		{ID: 8, Name: "Cirion", Type: "Warrior", Role: core.RoleWarrior, Kind: core.KindInfantry, Points: 8, Injured: true},
	}
	if roster != nil {
		detail.Roster = roster
	}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	m := core.Match{ID: 3, Number: 3, Revision: 6, Sides: sides, Scenario: &core.MatchScenario{Kind: core.ScenarioRulebook, Name: "Chance Encounter"}}
	if matchChange != nil {
		matchChange(&m)
	}
	f.camp.EXPECT().Match(gomock.Any(), int64(7), int64(3)).Return(m, nil).AnyTimes()
	lc := core.LiveCompany{MatchID: 3, CompanyID: 1, MatchNumber: 3, Seq: 12, Access: access,
		Units: []core.LiveUnit{
			{UnitID: 5, Zone: core.ZoneField, Wounds: core.LiveStat{Current: 1, Start: 1}},
			{UnitID: 6, Zone: core.ZoneRetreated, Reason: "Failed courage", Wounds: core.LiveStat{Current: 1, Start: 1}},
			{UnitID: 7, Zone: core.ZoneCasualty, Reason: "Wounded by Grashnak (Raiders)", Wounds: core.LiveStat{Current: 0, Start: 1}},
		}, Absent: []int64{8}}
	if change != nil {
		change(&lc)
	}
	f.camp.EXPECT().Live(gomock.Any(), frodo, int64(7), int64(1)).Return(lc, nil).AnyTimes()
	return f
}

func TestLiveCompanyPageHasBannerZonesAndNoRosterControls(t *testing.T) {
	f := liveOwnerSetup(t, core.LiveEdit)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company", nil))).Body.String()
	for _, want := range []string{`id="live-banner"`, "Match 3", "is running", "Chance Encounter", "Match log", "locked until the match ends",
		`id="live-zones"`, `data-seq="12"`, "Retreated", "Failed courage", "Field casualties", "Wounded by Grashnak (Raiders)", "Missed this game", "Cirion",
		"Return to the battle", `/matches/3/live/1/actions`} {
		if !strings.Contains(body, want) {
			t.Errorf("live page lacks %q", want)
		}
	}
	for _, banned := range []string{"Recruit ·", "Change formation", "Buy equipment", "Buy wargear", "Heal "} {
		if strings.Contains(body, banned) {
			t.Errorf("a company in a running match has no %q", banned)
		}
	}
	// the order the user asked for: field, retreated, casualties, missed, then the cemetery
	order := []string{`id="live-zones"`, `aria-label="Retreated"`, `aria-label="Field casualties"`, `aria-label="Missed this game"`}
	last := -1
	for _, o := range order {
		i := strings.Index(body, o)
		if i < last {
			t.Errorf("%q is out of order", o)
		}
		last = i
	}
	if strings.Count(body, `class="card unit-slot`) != 3 {
		t.Error("the retreated, casualty and missed units are compact cards")
	}
}

func TestLiveCompanyPageForAViewerHasNoControls(t *testing.T) {
	f := liveOwnerSetup(t, core.LiveNone)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company", nil))).Body.String()
	if !strings.Contains(body, `id="live-banner"`) || !strings.Contains(body, "Retreated") {
		t.Error("a viewer sees the live page")
	}
	for _, banned := range []string{"Match log</button>", "Return to the battle"} {
		if strings.Contains(body, banned) {
			t.Errorf("a viewer has no %q", banned)
		}
	}
}

func TestLiveFragmentIsTheZonesOnly(t *testing.T) {
	f := liveOwnerSetup(t, core.LiveEdit)
	rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/1/live", nil)))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.HasPrefix(strings.TrimSpace(body), `<div id="live-zones"`) || strings.Contains(body, "<html") || strings.Contains(body, `id="live-banner"`) {
		t.Errorf("%d %.200s", rec.Code, body)
	}
	g := newFixture(t, Config{})
	d, detail := mineDetail()
	g.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	g.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
	g.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil)
	if rec := g.do(g.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/1/live", nil))); rec.Code != http.StatusNotFound {
		t.Errorf("a company in no match has no live fragment: %d", rec.Code)
	}
}

func TestACompanyInNoOpenMatchIsNotLive(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company", nil))).Body.String()
	if strings.Contains(body, "live-banner") || strings.Contains(body, "live-zones") || !strings.Contains(body, "Recruit ·") {
		t.Error("the usual page")
	}
}

func TestLiveStatPressIsOneActionAndAnswersZonesWithAToast(t *testing.T) {
	f := liveOwnerSetup(t, core.LiveEdit)
	f.camp.EXPECT().Apply(gomock.Any(), frodo, int64(7), int64(3), int64(1), core.LiveAction{Kind: core.LiveStatChange, UnitID: 5, Stat: "wounds", Delta: -1}).
		Return(core.LiveCompany{}, core.LiveEntry{ID: 40, Text: "Hador: Wounds -1 (manual)"}, nil).Times(2) // two presses, two entries: the answers are not deduplicated
	for i := 0; i < 2; i++ {
		rec := f.do(f.signedIn(form("/campaigns/7/matches/3/live/1/actions", url.Values{"kind": {"stat"}, "unit": {"5"}, "stat": {"wounds"}, "delta": {"-1"}})))
		body := rec.Body.String()
		if rec.Code != 200 || !strings.Contains(body, `id="live-zones"`) || !strings.Contains(body, "Hador: Wounds -1 (manual)") || !strings.Contains(body, `/live/log/40/undo`) || strings.Contains(body, "<html") {
			t.Fatalf("%d %.300s", rec.Code, body)
		}
	}
}

func TestLiveSetToSendsTheValueAndARefusalIsShownWithTheState(t *testing.T) {
	f := liveOwnerSetup(t, core.LiveEdit)
	three := 3
	f.camp.EXPECT().Apply(gomock.Any(), frodo, int64(7), int64(3), int64(1), core.LiveAction{Kind: core.LiveStatChange, UnitID: 5, Stat: "fate", Set: &three}).
		Return(core.LiveCompany{}, core.LiveEntry{}, core.Refuse("Fate cannot go above 1."))
	rec := f.do(f.signedIn(form("/campaigns/7/matches/3/live/1/actions", url.Values{"kind": {"stat"}, "unit": {"5"}, "stat": {"fate"}, "set": {"3"}})))
	body := rec.Body.String()
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(body, "Fate cannot go above 1.") || !strings.Contains(body, `id="live-zones"`) || strings.Contains(body, "/undo") {
		t.Errorf("%d %.300s", rec.Code, body)
	}
}

func TestLiveActionByAViewerIs403AndAnUnknownKindIsRefused(t *testing.T) {
	f := liveOwnerSetup(t, core.LiveNone)
	f.camp.EXPECT().Apply(gomock.Any(), frodo, int64(7), int64(3), int64(1), gomock.Any()).Return(core.LiveCompany{}, core.LiveEntry{}, core.Refuse("Only the owner of Rangers can change it."))
	rec := f.do(f.signedIn(form("/campaigns/7/matches/3/live/1/actions", url.Values{"kind": {"broken"}, "unit": {"5"}})))
	if rec.Code != http.StatusForbidden {
		t.Errorf("viewer: %d", rec.Code)
	}
	g := liveOwnerSetup(t, core.LiveEdit)
	rec = g.do(g.signedIn(form("/campaigns/7/matches/3/live/1/actions", url.Values{"kind": {"explode"}, "unit": {"5"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Unknown action") {
		t.Errorf("unknown kind: %d %.200s", rec.Code, rec.Body.String())
	}
}

func TestLiveUndoAnswersZonesAndShowsTheDependencyRefusal(t *testing.T) {
	f := liveOwnerSetup(t, core.LiveEdit)
	f.camp.EXPECT().Undo(gomock.Any(), frodo, int64(7), int64(3), int64(40), int64(1)).Return(core.LiveCompany{}, nil)
	rec := f.do(f.signedIn(form("/campaigns/7/matches/3/live/log/40/undo", url.Values{"company": {"1"}})))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Undone.") {
		t.Errorf("%d %.200s", rec.Code, rec.Body.String())
	}
	f.camp.EXPECT().Undo(gomock.Any(), frodo, int64(7), int64(3), int64(41), int64(1)).Return(core.LiveCompany{}, core.ErrLiveDepends)
	rec = f.do(f.signedIn(form("/campaigns/7/matches/3/live/log/41/undo", url.Values{"company": {"1"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "later change depends on it") {
		t.Errorf("%d %.300s", rec.Code, rec.Body.String())
	}
}

func TestTrackersAreButtonsForTheOwnerAndNumbersForAViewer(t *testing.T) {
	f := liveOwnerSetup(t, core.LiveEdit)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company", nil))).Body.String()
	for _, want := range []string{`data-tracker="wounds"`, `Wounds of Hador down by one`, `"delta":"-1"`, `name="set"`, `max="1"`} {
		if !strings.Contains(body, want) {
			t.Errorf("owner lacks %q", want)
		}
	}
	g := liveOwnerSetup(t, core.LiveNone)
	body = g.do(g.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company", nil))).Body.String()
	if !strings.Contains(body, `data-tracker="wounds"`) || strings.Contains(body, "down by one") || strings.Contains(body, `name="set"`) {
		t.Error("a viewer reads the trackers, nothing more")
	}
}

func TestLiveSheetsOfTheActionsMenu(t *testing.T) {
	f := liveOwnerSetup(t, core.LiveEdit)
	get := func(kind string) (int, string) {
		rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/3/live/1/units/5/sheet/"+kind, nil)))
		return rec.Code, rec.Body.String()
	}
	code, body := get("menu")
	if code != 200 || !strings.Contains(body, "Wounded a unit") || !strings.Contains(body, "Passed a Broken courage roll") || strings.Contains(body, "Passed a fate roll") || strings.Contains(body, "Mount removed") {
		t.Errorf("menu: %d %s", code, body) // no Fate left to spend, no mount
	}
	for kind, want := range map[string]string{"broken": `name="kind" value="broken"`, "retreat": "Failed a courage roll", "died": "Fell off a cliff"} {
		if code, body := get(kind); code != 200 || !strings.Contains(body, want) || !strings.Contains(body, `hx-target="#live-zones"`) {
			t.Errorf("%s: %d", kind, code)
		}
	}
	if code, _ := get("nonsense"); code != 404 {
		t.Errorf("unknown sheet: %d", code)
	}
	g := liveOwnerSetup(t, core.LiveNone)
	if rec := g.do(g.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/3/live/1/units/5/sheet/menu", nil))); rec.Code != http.StatusForbidden {
		t.Errorf("a viewer gets no sheet: %d", rec.Code)
	}
}

func TestWoundSheetListsOnlyUnitsInTheField(t *testing.T) {
	f := liveOwnerSetup(t, core.LiveEdit,
		core.MatchSide{Name: "Gondor", Companies: []core.CampaignCompany{{ID: 1, Name: "Rangers"}}},
		core.MatchSide{Name: "Mordor", Companies: []core.CampaignCompany{{ID: 2, Name: "Raiders"}}})
	f.camp.EXPECT().Live(gomock.Any(), frodo, int64(7), int64(2)).Return(core.LiveCompany{MatchID: 3, CompanyID: 2, Units: []core.LiveUnit{
		{UnitID: 20, Zone: core.ZoneField, Wounds: core.LiveStat{Current: 3, Start: 3}, Fate: core.LiveStat{Current: 2, Start: 2}, HasMount: true, Mounted: true, MountWounds: core.LiveStat{Current: 1, Start: 1}},
		{UnitID: 21, Zone: core.ZoneCasualty, Wounds: core.LiveStat{Start: 1}}}}, nil)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(2)).Return(core.CompanyDetail{Roster: []core.CompanyUnit{
		{ID: 20, Name: "Grashnak", Type: "Orc Captain", Role: core.RoleLeader}, {ID: 21, Name: "Dead", Type: "Orc"}}}, nil)
	rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/3/live/1/units/5/sheet/wound", nil)))
	body := rec.Body.String()
	for _, want := range []string{"Grashnak", `data-fate="2"`, `data-mounted="1"`, `data-kill="1"`, "Which part", "Saved by Fate", "Raiders"} {
		if !strings.Contains(body, want) {
			t.Errorf("wound sheet lacks %q", want)
		}
	}
	if strings.Contains(body, "Dead") || strings.Contains(body, "Friendly fire") {
		t.Error("only units in the field; nobody of the own side is left to be friendly fire")
	}
}

func TestUnitThatLostItsMountIsOnFootWithMountGearStruckThrough(t *testing.T) {
	d, detail := mineDetail()
	detail.OpenMatch = &core.OpenMatchRef{MatchID: 3, Number: 3}
	detail.Roster = []core.CompanyUnit{{ID: 5, Name: "Hador", Type: "Rider", Role: core.RoleSergeant, Kind: core.KindCavalry, Points: 20, Keywords: rohanRider(),
		Equipment: []core.Equipment{{Name: "Horse", Class: core.EquipMount}, {Name: "Lance", Class: core.EquipMelee, RequiresMount: true}, {Name: "Sword", Class: core.EquipMelee}},
		Rules:     []core.UnitRule{{Name: "Cavalry", Source: core.RuleFromUnit}, {Name: "Timid", Mount: "Horse", Source: core.RuleFromUnit}, {Name: "Bodyguard", Source: core.RuleFromUnit}}}}
	g := newFixture(t, Config{})
	g.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	g.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	g.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	g.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	g.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	g.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	g.camp.EXPECT().Match(gomock.Any(), int64(7), int64(3)).Return(core.Match{ID: 3, Number: 3}, nil).AnyTimes()
	live := func(mounted bool) core.LiveCompany {
		return core.LiveCompany{MatchID: 3, CompanyID: 1, MatchNumber: 3, Access: core.LiveEdit, Units: []core.LiveUnit{{UnitID: 5, Zone: core.ZoneField, HasMount: true, Mounted: mounted, Wounds: core.LiveStat{Current: 1, Start: 1}}}}
	}
	g.camp.EXPECT().Live(gomock.Any(), frodo, int64(7), int64(1)).Return(live(false), nil)
	body := g.do(g.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company", nil))).Body.String()
	for _, want := range []string{"On foot", `aria-label="Horse, the mount is lost"`, `aria-label="Lance, needs a mount"`, `aria-label="Cavalry, needs a mount"`, `aria-label="Timid, needs a mount"`} {
		if !strings.Contains(body, want) {
			t.Errorf("lacks %q", want)
		}
	}
	if strings.Contains(body, `aria-label="Sword,`) || strings.Contains(body, `aria-label="Bodyguard,`) {
		t.Error("what needs no mount stays as it is")
	}
	if row := keywordRowOf(body); !strings.Contains(row, ">Man<") || strings.Contains(row, ">Cavalry<") || strings.Contains(body, "kw-off") || strings.Contains(body, "Mount: Horse</dt>") {
		t.Errorf("on foot: the keywords do not change (Cavalry is the icon's, the mount is gear): %s", row)
	}
	g.camp.EXPECT().Live(gomock.Any(), frodo, int64(7), int64(1)).Return(live(true), nil)
	body = g.do(g.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company", nil))).Body.String()
	if row := keywordRowOf(body); !strings.Contains(row, ">Man<") || strings.Contains(row, ">Cavalry<") || strings.Contains(body, "Mount: Horse</dt>") {
		t.Errorf("mounted: the same row: %s", row)
	}
	if strings.Contains(body, "On foot") || strings.Contains(body, "needs a mount") {
		t.Error("with the mount back nothing is struck through")
	}
}

func logEntries(n int, from int64) []core.LiveEntry {
	var out []core.LiveEntry
	for i := 0; i < n; i++ {
		out = append(out, core.LiveEntry{ID: from - int64(i), Seq: from - int64(i), CompanyID: 1, Text: "Hador: Wounds -1 (manual)", By: "Frodo", On: time.Date(2026, 10, 20, 14, 32, 0, 0, time.UTC), CanUndo: i == 0})
	}
	return out
}

func TestLiveLogSheetListsEntriesNewestFirstWithUndoForWhoMay(t *testing.T) {
	f := liveOwnerSetup(t, core.LiveEdit, core.MatchSide{Name: "Gondor", Companies: []core.CampaignCompany{{ID: 1, Name: "Rangers"}}})
	f.camp.EXPECT().Log(gomock.Any(), frodo, int64(7), int64(3), int64(0), liveLogPage).Return(logEntries(3, 12), nil)
	rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/3/live/log?company=1", nil)))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "Match log") || !strings.Contains(body, "Hador: Wounds -1 (manual)") || strings.Count(body, "/live/log/12/undo") != 1 || strings.Contains(body, "/live/log/11/undo") {
		t.Errorf("%d %s", rec.Code, body)
	}
	if !strings.Contains(body, `"company":"1"`) || strings.Contains(body, "Show older") || !strings.Contains(body, "14:32") {
		t.Errorf("sheet: %s", body)
	}
}

func TestLiveLogPagesAndStaysAfterTheMatchEnds(t *testing.T) {
	f := liveOwnerSetup(t, core.LiveNone)
	f.camp.EXPECT().Log(gomock.Any(), frodo, int64(7), int64(3), int64(0), liveLogPage).Return(logEntries(liveLogPage, 100), nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/3/live/log?card=1", nil))).Body.String()
	if !strings.Contains(body, "Show older entries") || !strings.Contains(body, "before=71") {
		t.Errorf("a full page offers the older ones: %s", body)
	}
	f.camp.EXPECT().Log(gomock.Any(), frodo, int64(7), int64(3), int64(71), liveLogPage).Return(logEntries(2, 70), nil)
	rows := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/3/live/log?before=71&card=1", nil))).Body.String()
	if strings.Contains(rows, "<section") || !strings.Contains(rows, "<li") || strings.Contains(rows, "Show older") {
		t.Errorf("an older page is rows only: %s", rows)
	}
	// an ended match shows its log read only, and says so when there is none
	g := newFixture(t, Config{})
	d, _ := mineDetail()
	g.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	g.camp.EXPECT().Match(gomock.Any(), int64(7), int64(3)).Return(core.Match{ID: 3, Number: 3, Ended: true}, nil).AnyTimes()
	g.camp.EXPECT().Log(gomock.Any(), frodo, int64(7), int64(3), int64(0), liveLogPage).Return(logEntries(2, 5), nil)
	body = g.do(g.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/3/live/log?card=1", nil))).Body.String()
	if strings.Contains(body, "/undo") || !strings.Contains(body, "Hador: Wounds -1 (manual)") {
		t.Errorf("ended: %s", body)
	}
	g.camp.EXPECT().Log(gomock.Any(), frodo, int64(7), int64(3), int64(0), liveLogPage).Return(nil, nil)
	if body := g.do(g.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/3/live/log?card=1", nil))).Body.String(); !strings.Contains(body, "recorded after the fact") {
		t.Errorf("no log: %s", body)
	}
}

func TestUndoFromTheLogAnswersTheLogAndTheZonesOutOfBand(t *testing.T) {
	f := liveOwnerSetup(t, core.LiveEdit, core.MatchSide{Name: "Gondor", Companies: []core.CampaignCompany{{ID: 1, Name: "Rangers"}}})
	f.camp.EXPECT().Undo(gomock.Any(), frodo, int64(7), int64(3), int64(12), int64(1)).Return(core.LiveCompany{}, nil)
	f.camp.EXPECT().Log(gomock.Any(), frodo, int64(7), int64(3), int64(0), liveLogPage).Return(logEntries(1, 11), nil).Times(2)
	rec := f.do(f.signedIn(form("/campaigns/7/matches/3/live/log/12/undo", url.Values{"company": {"1"}, "from": {"log"}})))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, `id="live-log"`) || !strings.Contains(body, `id="live-zones" hx-swap-oob="true"`) {
		t.Errorf("%d %.400s", rec.Code, body)
	}
	f.camp.EXPECT().Undo(gomock.Any(), frodo, int64(7), int64(3), int64(11), int64(1)).Return(core.LiveCompany{}, core.ErrLiveDepends)
	rec = f.do(f.signedIn(form("/campaigns/7/matches/3/live/log/11/undo", url.Values{"company": {"1"}, "from": {"log"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "later change depends on it") || strings.Contains(rec.Body.String(), "live-zones") {
		t.Errorf("%d %.300s", rec.Code, rec.Body.String())
	}
}

func TestCreateMatchRefusedForABusyCompanyLinksToItsMatch(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil).AnyTimes()
	f.camp.EXPECT().Companies(gomock.Any(), int64(7)).Return([]core.CampaignCompany{{ID: 1, Name: "Rangers"}, {ID: 2, Name: "Raiders"}}, nil).AnyTimes()
	f.camp.EXPECT().CreateMatch(gomock.Any(), frodo, int64(7), gomock.Any()).Return(core.Match{}, &core.CompanyBusyError{Company: "Rangers", OpenMatchRef: core.OpenMatchRef{MatchID: 41, Number: 6}})
	rec := f.do(f.signedIn(form("/campaigns/7/matches", url.Values{"sides": {"2"}, "side-1": {"1"}, "side-2": {"2"}})))
	body := rec.Body.String()
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(body, "Rangers is already in Match 6.") || !strings.Contains(body, `href="/campaigns/7/matches/41"`) || !strings.Contains(body, "Go to Match 6") {
		t.Errorf("%d %s", rec.Code, body)
	}
	g := newFixture(t, Config{})
	g.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil).AnyTimes()
	g.camp.EXPECT().Companies(gomock.Any(), int64(7)).Return([]core.CampaignCompany{{ID: 1, Name: "Rangers"}, {ID: 2, Name: "Raiders"}}, nil).AnyTimes()
	g.camp.EXPECT().CreateMatch(gomock.Any(), frodo, int64(7), gomock.Any()).Return(core.Match{}, core.Refuse("Finish the aftermath of Match 5 first."))
	rec = g.do(g.signedIn(form("/campaigns/7/matches", url.Values{"sides": {"2"}, "side-1": {"1"}, "side-2": {"2"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Finish the aftermath of Match 5 first.") || strings.Contains(rec.Body.String(), "Go to Match") {
		t.Errorf("pending aftermath: %d", rec.Code)
	}
}

func TestAftermathSheetFromTheLiveMatchCarriesTheBadge(t *testing.T) {
	f := amFixture(t)
	st := amState(core.AftermathPending, core.AftermathEdit)
	st.FromLive = true
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(st, nil)
	body := f.do(f.signedIn(httpGet(amPath))).Body.String()
	if !strings.Contains(body, "From the live match") {
		t.Error("the prefilled sheet says where it comes from")
	}
	g := amFixture(t)
	g.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(amState(core.AftermathPending, core.AftermathEdit), nil)
	if body := g.do(g.signedIn(httpGet(amPath))).Body.String(); strings.Contains(body, "From the live match") {
		t.Error("no badge for a sheet that is not from the live match")
	}
}

// keywordRowOf is the compact keyword row of the first card in a page.
func keywordRowOf(body string) string {
	i := strings.Index(body, `aria-label="Keywords"`)
	j := strings.Index(body[i:], `class="kw-full"`)
	if i < 0 || j < 0 {
		return ""
	}
	return body[i : i+j]
}
