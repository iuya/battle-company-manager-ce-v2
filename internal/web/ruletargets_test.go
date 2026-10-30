package web

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func orcKeywords() core.UnitKeywords {
	return core.UnitKeywords{Model: []core.UnitKeyword{kw("Orc", core.KeywordRace, "Keywords"), kw("Mordor", core.KeywordFaction, "Keywords"), kw("Warrior", core.KeywordType, "Warrior")}}
}

func hater(name string, id int64) core.CompanyUnit {
	return core.CompanyUnit{ID: id, Name: name, Type: "Warrior of Minas Tirith", Role: core.RoleSergeant, Kind: core.KindInfantry,
		Keywords: core.UnitKeywords{Model: []core.UnitKeyword{kw("Man", core.KeywordRace, "Keywords"), kw("Gondor", core.KeywordFaction, "Keywords")}},
		Rules: []core.UnitRule{{Name: "Hatred (Orc)", Description: "A bonus to wound against the keyword.", Works: "enemy models with the Orc keyword", Against: [][]string{{"Orc"}}},
			{Name: "Terror", Description: "Fear.", Works: "enemy models in general"}}}
}

const markerRe = `data-rule-match="Hatred \(Orc\)"`

func TestWoundSheetShowsEnemyKeywordsAndWhichRulesApply(t *testing.T) {
	f := liveSetupRoster(t, core.LiveEdit, nil, nil, []core.CompanyUnit{hater("Hador", 5)},
		core.MatchSide{Name: "Gondor", Companies: []core.CampaignCompany{{ID: 1, Name: "Rangers"}}},
		core.MatchSide{Name: "Mordor", Companies: []core.CampaignCompany{{ID: 2, Name: "Raiders"}}})
	f.camp.EXPECT().Live(gomock.Any(), frodo, int64(7), int64(2)).Return(core.LiveCompany{MatchID: 3, CompanyID: 2, Units: []core.LiveUnit{
		{UnitID: 20, Zone: core.ZoneField, Wounds: core.LiveStat{Current: 3, Start: 3}},
		{UnitID: 22, Zone: core.ZoneField, Wounds: core.LiveStat{Current: 3, Start: 3}},
		{UnitID: 23, Zone: core.ZoneCasualty, Wounds: core.LiveStat{Start: 1}}}}, nil)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(2)).Return(core.CompanyDetail{Roster: []core.CompanyUnit{
		{ID: 20, Name: "Grashnak", Type: "Orc Captain", Role: core.RoleLeader, Keywords: orcKeywords()},
		{ID: 22, Name: "Hunter", Type: "Haradrim", Keywords: core.UnitKeywords{Model: []core.UnitKeyword{kw("Man", core.KeywordRace, "Keywords"), kw("Harad", core.KeywordFaction, "Keywords")}}},
		{ID: 23, Name: "Dead", Type: "Orc", Keywords: orcKeywords()}}}, nil)
	f.camp.EXPECT().Log(gomock.Any(), frodo, int64(7), int64(3), int64(0), 200).Return(nil, nil).AnyTimes() // the window lists what the unit recorded
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/3/live/1/units/5/sheet/wound", nil))).Body.String()
	if got := len(regexp.MustCompile(markerRe).FindAllString(body, -1)); got != 1 {
		t.Fatalf("%d Hatred (Orc) markers, want 1 (Grashnak only; Terror names no keyword, the Haradrim is no Orc)\n%s", got, body)
	}
	i := strings.Index(body, "Grashnak")
	j := strings.Index(body, "Hunter")
	if i < 0 || j < i {
		t.Fatal("the rows are not in order")
	}
	grashnak := body[i:j]
	for _, want := range []string{"Hatred (Orc) applies", `data-keywords-flat`, `aria-label="Race keyword: Orc"`, `aria-label="Faction keyword: Mordor"`} {
		if !strings.Contains(grashnak, want) {
			t.Errorf("Grashnak's row lacks %q: %s", want, grashnak)
		}
	}
	if strings.Contains(body[j:], "applies") || !strings.Contains(body[j:], `aria-label="Faction keyword: Harad"`) {
		t.Error("the Haradrim shows his keywords and no marker")
	}
	if strings.Contains(body, "Dead") {
		t.Error("a casualty is not listed")
	}
}

func TestOpponentsPageMarksTheUnitsYourRulesNameAndTheRefreshKeepsThem(t *testing.T) {
	f := liveSetupRoster(t, core.LiveEdit, nil, nil, []core.CompanyUnit{hater("Hador", 5)},
		core.MatchSide{Name: "Gondor", Companies: []core.CampaignCompany{{ID: 1, Name: "Rangers"}}},
		core.MatchSide{Name: "Mordor", Companies: []core.CampaignCompany{{ID: 2, Name: "Raiders"}}})
	other := core.CompanyDetail{CampaignCompany: core.CampaignCompany{ID: 2, Name: "Raiders"}, OpenMatch: &core.OpenMatchRef{MatchID: 3, Number: 3}, Roster: []core.CompanyUnit{
		{ID: 20, Name: "Grashnak", Type: "Orc Captain", Role: core.RoleLeader, Kind: core.KindInfantry, Keywords: orcKeywords()},
		{ID: 21, Name: "Snaga", Type: "Orc Warrior", Role: core.RoleWarrior, Kind: core.KindInfantry, Keywords: orcKeywords()},
		{ID: 22, Name: "Hunter", Type: "Haradrim", Role: core.RoleWarrior, Kind: core.KindInfantry, Keywords: core.UnitKeywords{Model: []core.UnitKeyword{kw("Harad", core.KeywordFaction, "Keywords")}}}}}
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(2)).Return(other, nil).AnyTimes()
	f.camp.EXPECT().Live(gomock.Any(), frodo, int64(7), int64(2)).Return(core.LiveCompany{MatchID: 3, CompanyID: 2, Units: []core.LiveUnit{
		{UnitID: 20, Zone: core.ZoneField, Wounds: core.LiveStat{Current: 3, Start: 3}},
		{UnitID: 21, Zone: core.ZoneRetreated, Reason: "Left the board", Wounds: core.LiveStat{Current: 1, Start: 1}},
		{UnitID: 22, Zone: core.ZoneField, Wounds: core.LiveStat{Current: 1, Start: 1}}}}, nil).AnyTimes()
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/2/live", nil))).Body.String() // what the live refresh swaps in
	if got := len(regexp.MustCompile(markerRe).FindAllString(body, -1)); got != 1 {
		t.Fatalf("%d markers, want 1: the Orc in the field (the retreated Orc is not in the field, the Haradrim is no Orc)\n%s", got, body)
	}
	for _, want := range []string{`data-rule-matches`, `aria-label="Rules of your units that apply to this unit"`, `data-tip-title="Hatred (Orc) applies"`, `data-tip-meta="Your Hador"`,
		"Your Hador has Hatred (Orc). This unit has the Orc keyword, so the rule works against it.", `data-concept="Hatred (Orc)"`, `tabindex="0"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the refresh lacks %q", want)
		}
	}
	// the retreated Orc's compact card shows its keywords, and no marker
	i := strings.Index(body, `aria-label="Retreated"`)
	if i < 0 || !strings.Contains(body[i:], `aria-label="Race keyword: Orc"`) || strings.Contains(body[i:], "applies") {
		t.Error("the compact card of the retreated enemy shows its keyword chips and no marker")
	}
	// the viewer's own side sees no markers on its own company
	own := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/1/live", nil))).Body.String()
	if strings.Contains(own, "data-rule-match") {
		t.Error("nothing of your own side is an enemy")
	}
}

func TestNoRuleTargetsNoMarkersAndPlainTooltips(t *testing.T) {
	h := hater("Hador", 5)
	for i := range h.Rules { // a custom ruleset: the rules carry no targets
		h.Rules[i].Works, h.Rules[i].Against = "", nil
	}
	f := liveSetupRoster(t, core.LiveEdit, nil, nil, []core.CompanyUnit{h},
		core.MatchSide{Name: "Gondor", Companies: []core.CampaignCompany{{ID: 1, Name: "Rangers"}}},
		core.MatchSide{Name: "Mordor", Companies: []core.CampaignCompany{{ID: 2, Name: "Raiders"}}})
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(2)).Return(core.CompanyDetail{CampaignCompany: core.CampaignCompany{ID: 2, Name: "Raiders"}, OpenMatch: &core.OpenMatchRef{MatchID: 3}, Roster: []core.CompanyUnit{{ID: 20, Name: "Grashnak", Type: "Orc", Kind: core.KindInfantry, Keywords: orcKeywords()}}}, nil).AnyTimes()
	f.camp.EXPECT().Live(gomock.Any(), frodo, int64(7), int64(2)).Return(core.LiveCompany{MatchID: 3, CompanyID: 2, Units: []core.LiveUnit{{UnitID: 20, Zone: core.ZoneField, Wounds: core.LiveStat{Current: 3, Start: 3}}}}, nil).AnyTimes()
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/2/live", nil))).Body.String()
	if strings.Contains(body, "data-rule-match") || strings.Contains(body, "applies") || !strings.Contains(body, "Orc") {
		t.Error("without rule targets the enemy shows its chips and no marker")
	}
	own := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1", nil))).Body.String()
	if strings.Contains(own, "data-tip-works") || strings.Contains(own, "Works against") {
		t.Error("plain tooltips without the table")
	}
}

func TestRuleTooltipsSayWhoTheRuleWorksAgainst(t *testing.T) {
	h := hater("Hador", 5)
	h.Rules = append(h.Rules, core.UnitRule{Name: "Fury (Orc)", Description: "Furious.", Works: "friendly models with the Orc keywords", Friends: true})
	f := liveSetupRoster(t, core.LiveEdit, nil, nil, []core.CompanyUnit{h})
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1", nil))).Body.String()
	for _, want := range []string{
		"A bonus to wound against the keyword.\n\nWorks against: enemy models with the Orc keyword.",
		`data-tip-works="Works against: enemy models with the Orc keyword."`,
		"Fear.\n\nWorks against: enemy models in general.",
		"Furious.\n\nWorks for: friendly models with the Orc keywords.",
		`data-works>Works against: enemy models with the Orc keyword.</p>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("lacks %q", want)
		}
	}
}
