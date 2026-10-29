package core

import (
	"errors"
	"strings"
	"testing"
)

func testMatch() Match {
	co := func(id, owner int64, name string) CampaignCompany {
		return CampaignCompany{ID: id, OwnerID: owner, Name: name}
	}
	return Match{ID: 1, Sides: []MatchSide{
		{Name: "Side 1", Companies: []CampaignCompany{co(1, 10, "Rangers")}},
		{Name: "Side 2", Companies: []CampaignCompany{co(2, 20, "Raiders"), co(3, 30, "Wolves")}},
	}}
}

func TestMatchAccess(t *testing.T) {
	const organiser = 99
	user := func(id int64, role Role) User { return User{ID: id, Role: role} }
	tests := []struct {
		name  string
		ended bool
		u     User
		want  MatchAccess
	}{
		{"participant, open", false, user(10, RoleUser), MatchEdit},
		{"participant on the second side, open", false, user(30, RoleUser), MatchEdit},
		{"organiser, open", false, user(organiser, RoleUser), MatchEdit},
		{"admin, open", false, user(5, RoleAdmin), MatchEdit},
		{"superuser, open", false, user(6, RoleSuperuser), MatchEdit},
		{"outsider, open", false, user(7, RoleUser), MatchNoAccess},
		{"participant, ended", true, user(10, RoleUser), MatchNoAccess},
		{"organiser, ended", true, user(organiser, RoleUser), MatchEditEnded},
		{"admin, ended", true, user(5, RoleAdmin), MatchEditEnded},
		{"outsider, ended", true, user(7, RoleUser), MatchNoAccess},
	}
	for _, tt := range tests {
		m := testMatch()
		m.Ended = tt.ended
		if got := m.Access(tt.u, organiser); got != tt.want {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
	if got := testMatch().Access(User{ID: 0}, 0); got != MatchNoAccess {
		t.Errorf("a user with id 0 is not the organiser of a campaign without one: %v", got)
	}
}

func TestMatchCanEnd(t *testing.T) {
	rulebook := &MatchScenario{Kind: ScenarioRulebook, Name: "Seize Ground"}
	tests := []struct {
		name   string
		change func(m *Match)
		reason string // "" when it can end
	}{
		{"ready", func(m *Match) {}, ""},
		{"a draw is a decision", func(m *Match) { m.Result.Winner = 0 }, ""},
		{"no scenario", func(m *Match) { m.Scenario = nil }, "scenario"},
		{"nothing reported", func(m *Match) { m.Result = MatchResult{} }, "who won"},
		{"the winner fled", func(m *Match) { m.Result.Companies = []CompanyResult{{CompanyID: 1, Fled: true}} }, "Rangers fled"},
		{"the loser fled", func(m *Match) { m.Result.Companies = []CompanyResult{{CompanyID: 2, Fled: true}} }, ""},
		{"already ended", func(m *Match) { m.Ended = true }, "already ended"},
	}
	for _, tt := range tests {
		m := testMatch()
		m.Scenario = rulebook
		m.Result = MatchResult{Decided: true, Winner: 1}
		tt.change(&m)
		err := m.CanEnd()
		switch {
		case tt.reason == "" && err != nil:
			t.Errorf("%s: %v", tt.name, err)
		case tt.reason != "" && !errors.Is(err, ErrRefused):
			t.Errorf("%s: want a refusal, got %v", tt.name, err)
		case tt.reason != "" && !strings.Contains(err.Error(), tt.reason):
			t.Errorf("%s: %q does not mention %q", tt.name, err, tt.reason)
		}
	}
}

func TestScenarioInputValidate(t *testing.T) {
	long := func(n int) string { return strings.Repeat("é", n) }
	tests := []struct {
		name string
		in   ScenarioInput
		ok   bool
		want ScenarioInput
	}{
		{"rulebook keeps only its name", ScenarioInput{Kind: ScenarioRulebook, Name: " Seize Ground ", Description: "x", Victory: "y"}, true, ScenarioInput{Kind: ScenarioRulebook, Name: "Seize Ground"}},
		{"custom trimmed", ScenarioInput{Kind: ScenarioCustom, Name: " Ambush ", Description: " d ", Victory: " v "}, true, ScenarioInput{Kind: ScenarioCustom, Name: "Ambush", Description: "d", Victory: "v"}},
		{"name 60", ScenarioInput{Kind: ScenarioCustom, Name: long(60)}, true, ScenarioInput{Kind: ScenarioCustom, Name: long(60)}},
		{"name 61", ScenarioInput{Kind: ScenarioCustom, Name: long(61)}, false, ScenarioInput{}},
		{"no name", ScenarioInput{Kind: ScenarioCustom, Name: "  "}, false, ScenarioInput{}},
		{"description 1000", ScenarioInput{Kind: ScenarioCustom, Name: "a", Description: long(1000)}, true, ScenarioInput{Kind: ScenarioCustom, Name: "a", Description: long(1000)}},
		{"description 1001", ScenarioInput{Kind: ScenarioCustom, Name: "a", Description: long(1001)}, false, ScenarioInput{}},
		{"victory 1001", ScenarioInput{Kind: ScenarioCustom, Name: "a", Victory: long(1001)}, false, ScenarioInput{}},
		{"unknown kind", ScenarioInput{Name: "a"}, false, ScenarioInput{}},
	}
	for _, tt := range tests {
		got, err := tt.in.Validate()
		if tt.ok != (err == nil) {
			t.Errorf("%s: err = %v", tt.name, err)
			continue
		}
		if err != nil && !errors.Is(err, ErrInvalidMatch) {
			t.Errorf("%s: %v is not ErrInvalidMatch", tt.name, err)
		}
		if tt.ok && got != tt.want {
			t.Errorf("%s: %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

func TestValidateResult(t *testing.T) {
	info := &ScenarioInfo{Name: "Seize Ground", Objectives: []ScenarioObjective{{Key: "relic", Max: 2}}}
	vp := func(n int) *int { return &n }
	base := func() MatchResult {
		return MatchResult{Decided: true, Winner: 2, Sides: []SideResult{{VictoryPoints: vp(3)}, {}}}
	}
	m := testMatch()
	m.Scenario = &MatchScenario{Kind: ScenarioRulebook, Name: "Seize Ground"}
	tests := []struct {
		name   string
		change func(r *MatchResult)
		ok     bool
	}{
		{"valid", func(r *MatchResult) {}, true},
		{"objective within max", func(r *MatchResult) {
			r.Companies = []CompanyResult{{CompanyID: 1, Objectives: map[string]int{"relic": 2, "other": 0}}}
		}, true},
		{"objective above max", func(r *MatchResult) {
			r.Companies = []CompanyResult{{CompanyID: 1, Objectives: map[string]int{"relic": 3}}}
		}, false},
		{"unknown objective", func(r *MatchResult) {
			r.Companies = []CompanyResult{{CompanyID: 1, Objectives: map[string]int{"nope": 1}}}
		}, false},
		{"company not in the match", func(r *MatchResult) { r.Companies = []CompanyResult{{CompanyID: 9}} }, false},
		{"company twice", func(r *MatchResult) { r.Companies = []CompanyResult{{CompanyID: 1}, {CompanyID: 1}} }, false},
		{"winner out of range", func(r *MatchResult) { r.Winner = 3 }, false},
		{"wrong number of sides", func(r *MatchResult) { r.Sides = r.Sides[:1] }, false},
		{"VP 99", func(r *MatchResult) { r.Sides[0].VictoryPoints = vp(99) }, true},
		{"VP 100", func(r *MatchResult) { r.Sides[0].VictoryPoints = vp(100) }, false},
		{"VP negative", func(r *MatchResult) { r.Sides[1].VictoryPoints = vp(-1) }, false},
		{"note 500", func(r *MatchResult) { r.Note = strings.Repeat("a", 500) }, true},
		{"note 501", func(r *MatchResult) { r.Note = strings.Repeat("a", 501) }, false},
	}
	for _, tt := range tests {
		r := base()
		tt.change(&r)
		_, err := m.ValidateResult(r, info)
		if tt.ok != (err == nil) {
			t.Errorf("%s: err = %v", tt.name, err)
		} else if err != nil && !errors.Is(err, ErrInvalidMatch) {
			t.Errorf("%s: %v is not ErrInvalidMatch", tt.name, err)
		}
	}
	// an undecided result carries no winner; a custom scenario has no objectives
	got, _ := m.ValidateResult(MatchResult{Winner: 2, Sides: make([]SideResult, 2)}, info)
	if got.Winner != 0 || got.Decided {
		t.Errorf("undecided result: %+v", got)
	}
	m.Scenario = &MatchScenario{Kind: ScenarioCustom, Name: "x"}
	r := base()
	r.Companies = []CompanyResult{{CompanyID: 1, Objectives: map[string]int{"relic": 1}}}
	if _, err := m.ValidateResult(r, nil); err == nil {
		t.Error("a one-off scenario has no objectives")
	}
}

func TestParseScenariosReadsTheObjectivesOfTheRealFile(t *testing.T) {
	list := shippedRules(t).BC.Scenarios()
	by := map[string]ScenarioInfo{}
	for _, sc := range list {
		by[sc.Name] = sc
	}
	cargo := by["Baggage Train"].Objectives
	if len(cargo) != 1 || cargo[0].Key != "allCargo" || cargo[0].Max != 1 || cargo[0].Label == "" ||
		len(cargo[0].Rewards) != 1 || cargo[0].Rewards[0].Kind != "influence" || cargo[0].Rewards[0].Amount != 1 {
		t.Errorf("Baggage Train: %+v", cargo)
	}
	spoils := by["Scavenge the Battlefield"].Objectives
	if len(spoils) != 1 || spoils[0].Key != "allObjectiveMarkers" || spoils[0].Rewards[0].Kind != "equipment" || spoils[0].Rewards[0].Text == "" {
		t.Errorf("Scavenge the Battlefield: %+v", spoils)
	}
	if relic := by["Relic of Ages Past"].Objectives; len(relic) != 1 || relic[0].Key != "possessesRelic" || relic[0].Rewards[0].Kind != "other" {
		t.Errorf("Relic of Ages Past: %+v", relic)
	}
	// the rulebook text and the rewards that follow from the result
	if p := by["Chance Encounter"].Prizes; len(p) != 1 || p[0].Name != "Rendezvous" || p[0].Who != "The winner" || p[0].Text == "" {
		t.Errorf("Chance Encounter prizes: %+v", p)
	}
	if p := by["Rescue"].Prizes; len(p) != 1 || p[0].Name != "Survival of the Fittest" || p[0].Who == "" {
		t.Errorf("Rescue prizes: %+v", p)
	}
	if a := by["Stand Your Ground"].About; len(a.Roles) != 2 || a.Roles[0] != "Attacker" || len(a.Rules) != 1 {
		t.Errorf("Stand Your Ground about: %+v", a)
	}
	if a := by["Seize the Outpost"].About; a.Outline == "" || len(a.Rules) != 2 || len(a.Sections) < 3 {
		t.Errorf("Seize the Outpost about: %+v", a)
	}
	for _, sc := range list {
		if sc.About.Description == "" || len(sc.About.Sections) == 0 {
			t.Errorf("%s has no rulebook text: %+v", sc.Name, sc.About)
		}
	}
	// rewards that follow from the result are not ticks
	for _, name := range []string{"Chance Encounter", "Secure the Area", "Rescue", "Seize Ground"} {
		if n := len(by[name].Objectives); n != 0 {
			t.Errorf("%s has %d ticks", name, n)
		}
	}
}

func TestMatchSideLabelNamesTheCompaniesOfAnUnnamedSide(t *testing.T) {
	two := []CampaignCompany{{Name: "Rangers"}, {Name: "Raiders"}}
	tests := []struct {
		side MatchSide
		want string
	}{
		{MatchSide{Name: "Side 2", Companies: two[:1]}, "Rangers"},
		{MatchSide{Name: "Side 1", Companies: two}, "Rangers + Raiders"},
		{MatchSide{Name: "Gondor", Companies: two}, "Gondor"},
		{MatchSide{Name: "Side 3"}, "Side 3"},
	}
	for _, tt := range tests {
		if got := tt.side.Label(); got != tt.want {
			t.Errorf("Label(%+v) = %q, want %q", tt.side, got, tt.want)
		}
	}
	m := Match{Ended: true, Winner: 1, Sides: []MatchSide{{Name: "Side 1", Companies: two[:1]}, {Name: "Side 2", Companies: two[1:]}}}
	if m.WinnerName() != "Rangers" {
		t.Errorf("WinnerName = %q", m.WinnerName())
	}
}

func TestScenarioRuleTableRowFor(t *testing.T) {
	tb := ScenarioRuleTable{Rows: []ScenarioTableRow{{Min: 0, Max: 300}, {Min: 301, Max: 700}, {Min: 701, Max: -1}}}
	for v, want := range map[int]int{0: 0, 300: 0, 301: 1, 700: 1, 701: 2, 5000: 2, -1: -1} {
		if got := tb.RowFor(v); got != want {
			t.Errorf("RowFor(%d) = %d, want %d", v, got, want)
		}
	}
}
