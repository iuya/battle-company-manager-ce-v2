package web

import (
	"net/url"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The dice of the rules file size the forms, the headings and the dice tray: no page
// has a number of dice or of faces of its own.

func TestScenarioRollFollowsTheDieOfTheRules(t *testing.T) {
	m := editableMatch()
	f := rollSetup(t, m, core.CampaignSettings{ManualRolls: true})
	f.selection = core.ScenarioSelection{Die: core.NdM(1, 8), Tables: []core.ScenarioTableRange{{Min: 1, Max: 3}, {Min: 4, Max: 5}, {Min: 6, Max: 8}}}
	var sides []int
	seq := []int{8, 1}
	f.srv.die = func(n int) int { sides = append(sides, n); v := seq[0]; seq = seq[1:]; return v }
	done := m
	done.Scenario = &core.MatchScenario{Kind: core.ScenarioRulebook, Name: "Rescue"}
	f.camp.EXPECT().SetScenario(gomock.Any(), frodo, int64(7), int64(3), 4, core.ScenarioInput{Kind: core.ScenarioRulebook, Name: "Rescue"}).Return(done, nil)
	rec := f.do(f.signedIn(form(testMatchPath+"/scenario/roll", url.Values{"revision": {"4"}})))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "Rolled <strong>8</strong> (table 3) and <strong>1</strong>") || len(sides) != 2 || sides[0] != 8 || sides[1] != 8 {
		t.Errorf("an 8 and a 1 on d8 dice pick table 3: %d %v %s", rec.Code, sides, body)
	}
}

func TestPickerGroupsShowTheFirstDieRangeOfTheFile(t *testing.T) {
	groups := groupScenarios([]core.ScenarioInfo{{Name: "A", Table: 1, Roll: 1, FirstDie: [2]int{1, 4}}, {Name: "B", Table: 2, Roll: 1, FirstDie: [2]int{5, 5}}, {Name: "C"}})
	var titles []string
	for _, g := range groups {
		titles = append(titles, g.Title)
	}
	if got := strings.Join(titles, "|"); got != "Table 1 (first die 1-4)|Table 2 (first die 5)|Other scenarios" {
		t.Errorf("titles = %s", got)
	}
}

func TestInjuryFormIsSizedByTheInjuryDice(t *testing.T) {
	f := rollSetup(t, endedMatch(), core.CampaignSettings{ManualRolls: true})
	rules := amRules
	rules.InjuryDice = core.NdM(3, 8)
	f.camp.EXPECT().AftermathRules(gomock.Any(), int64(7)).Return(rules, nil).AnyTimes()
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(injuriesState(core.Casualty{UnitID: 10, Name: "Aldor"}), nil)
	body := f.do(f.signedIn(httpGet(amPath))).Body.String()
	for _, want := range []string{"Roll 3D8", `name="roll"`, `name="roll2"`, `name="roll3"`, "Die 3", `data-per-lane="3"`, `data-sides="8"`, `<option value="8">I rolled a 8</option>`} {
		if !strings.Contains(body, want) {
			t.Errorf("the injury form lacks %q", want)
		}
	}
	if strings.Contains(body, `name="roll4"`) || strings.Contains(body, `<option value="9">`) || strings.Contains(body, "2D6") {
		t.Error("the form has a die or a face the rules do not have")
	}
}

func TestChartHeaderShowsTheInjuryDice(t *testing.T) {
	f := amFixture(t)
	rules := amRules
	rules.InjuryDice = core.NdM(1, 12)
	rules.Warrior = []core.InjuryRow{{Min: 1, Max: 12, Key: "dead", Name: "Dead", Kind: core.CasualtyDead}}
	f2 := matchWrite(t, endedMatch())
	_ = f
	f2.camp.EXPECT().AftermathRules(gomock.Any(), int64(7)).Return(rules, nil).AnyTimes()
	f2.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(core.CompanyDetail{Roster: amUnits()}, nil).AnyTimes()
	f2.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(injuriesState(core.Casualty{UnitID: 10, Name: "Aldor", Rolled: true, Dice: []int{9}, Total: 9, ResultKey: "dead", ResultTxt: "Dead", Kind: core.CasualtyDead}), nil)
	body := f2.do(f2.signedIn(httpGet(amPath))).Body.String()
	if !strings.Contains(body, `>D12</th>`) || strings.Contains(body, "2D6") {
		t.Error("the chart header is the injury dice of the rules")
	}
}

func TestBonusDieMaxFollowsTheBonusDice(t *testing.T) {
	rules := amRules
	rules.Warrior = []core.InjuryRow{
		{Key: "ll", Name: "Lesson Learned", BonusDice: "d3", BonusSpec: core.NdM(1, 3), BonusKind: "experience"},
		{Key: "d10", Name: "Big", BonusDice: "d10", BonusSpec: core.NdM(1, 10), BonusKind: "influence"},
	}
	v := aftermathView{Rules: rules}
	if got := v.BonusMax(core.Casualty{ResultKey: "ll"}); got != 6 {
		t.Errorf("a D3 is rolled on a D6: %d", got)
	}
	if got := v.BonusMax(core.Casualty{ResultKey: "d10"}); got != 10 {
		t.Errorf("a D10 is typed as it is: %d", got)
	}
	if text := v.BonusDieText(core.Casualty{ResultKey: "ll"}); !strings.Contains(text, "On a D3 a die of 1-2 reads 1, 3-4 reads 2 and 5-6 reads 3.") {
		t.Errorf("the D3 hint = %q", text)
	}
	if text := v.BonusDieText(core.Casualty{ResultKey: "d10"}); strings.Contains(text, "reads") {
		t.Errorf("a D10 needs no hint: %q", text)
	}
}

func TestEnteredRollsReadsAsManyDiceAsTheFormCarries(t *testing.T) {
	read := func(v url.Values) []int {
		r := form("/x", v)
		return enteredRolls(r)
	}
	tests := []struct {
		name string
		form url.Values
		want []int
	}{
		{"nothing typed", url.Values{"roll": {"0"}, "roll2": {"0"}}, nil},
		{"absent", url.Values{}, nil},
		{"two dice", url.Values{"roll": {"3"}, "roll2": {"5"}}, []int{3, 5}},
		{"four dice", url.Values{"roll": {"1"}, "roll2": {"2"}, "roll3": {"3"}, "roll4": {"4"}}, []int{1, 2, 3, 4}},
		{"a die left on Roll for me is passed on as 0", url.Values{"roll": {"3"}, "roll2": {"0"}}, []int{3, 0}},
		{"a gap stops the reading", url.Values{"roll": {"3"}, "roll3": {"5"}}, []int{3}},
		{"not a number", url.Values{"roll": {"x"}}, nil},
	}
	for _, tt := range tests {
		got := read(tt.form)
		if len(got) != len(tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
			}
		}
	}
}

func TestLevelUpFormFollowsTheDiceOfTheRules(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	d.Settings.ManualRolls = true
	detail.Roster[0].XP, detail.Roster[0].LevelEvery = 5, 5
	detail.Roster[0].Role = core.RoleHero
	detail.Roster[0].Path = &core.UnitPath{Name: "Path of the Duelist"}
	detail.RuleDice.Level = core.LevelDice{Warrior: core.NdM(1, 6), Hero: core.NdM(2, 6), Pairs: 3, Paths: map[string]core.DiceSpec{"Path of the Duelist": core.NdM(1, 12)}}
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().LevelUpState(gomock.Any(), frodo, int64(7), int64(1), int64(5)).Return(nil, nil)
	body := f.do(f.signedIn(httpGet("/campaigns/7/my-company/1/units/5/levelup"))).Body.String()
	for _, want := range []string{"rolls 3 x D12", `data-sides="12"`, `data-per-lane="1"`, `data-lanes="3"`, `name="roll3"`, "Pair 3", `<option value="12">I rolled 12</option>`} {
		if !strings.Contains(body, want) {
			t.Errorf("the hero's level-up form lacks %q", want)
		}
	}
	if strings.Contains(body, `name="roll4"`) || strings.Contains(body, `<option value="13">`) {
		t.Error("more results or faces than the rules have")
	}
}

func TestCourageSheetFollowsTheCourageDice(t *testing.T) {
	saved := testRuleDice
	t.Cleanup(func() { testRuleDice = saved })
	testRuleDice.Courage = core.NdM(3, 8)
	f := liveSetupWith(t, core.LiveEdit, brokenState)
	body := f.do(f.signedIn(httpGet("/campaigns/7/matches/3/live/1/units/5/sheet/courage"))).Body.String()
	if !strings.Contains(body, "Roll 3D8") || !strings.Contains(body, "tests Courage (3D8, at least its Courage)") || strings.Contains(body, "2D6") {
		t.Errorf("the courage window prints the dice of the rules file: %s", body)
	}
}
