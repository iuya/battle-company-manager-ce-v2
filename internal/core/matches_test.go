package core

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"
)

type matchesFixture struct {
	svc       *Matches
	campaigns *MockcampaignRepository
	companies *MockcompanyRepository
	matches   *MockmatchRepository
	rulesets  *MockrulesetRepository
	match     Match // the stored match
	state     CompanyState
	ctx       context.Context
}

const matchOrganiser = 99

func newMatchesFixture(t *testing.T) *matchesFixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	f := &matchesFixture{campaigns: NewMockcampaignRepository(ctrl), companies: NewMockcompanyRepository(ctrl), matches: NewMockmatchRepository(ctrl),
		rulesets: NewMockrulesetRepository(ctrl), state: gondor(), ctx: context.Background()}
	f.match = testMatch()
	f.match.Revision = 3
	f.campaigns.EXPECT().Hashes(gomock.Any(), int64(5)).Return(RulesetHashes{BattleCompanies: "bc"}, CampaignSettings{}, nil).AnyTimes()
	f.campaigns.EXPECT().Hashes(gomock.Any(), int64(404)).Return(RulesetHashes{}, CampaignSettings{}, ErrNotFound).AnyTimes()
	f.campaigns.EXPECT().Get(gomock.Any(), gomock.Any(), int64(5)).Return(CampaignDetail{CampaignSummary: CampaignSummary{ID: 5, Status: CampaignActive}, OrganiserID: matchOrganiser}, nil).AnyTimes()
	f.companies.EXPECT().Read(gomock.Any(), int64(5), gomock.Any()).DoAndReturn(func(_ context.Context, _, id int64) (CompanyState, error) {
		return f.state.Clone(), nil
	}).AnyTimes()
	roster := func(context.Context, int64) ([]CompanyState, error) { // every company id the tests use, all with the fixture's roster
		var out []CompanyState
		for id := int64(1); id <= 9; id++ {
			st := f.state.Clone()
			st.Company.ID = id
			out = append(out, st)
		}
		return out, nil
	}
	f.companies.EXPECT().ReadRoster(gomock.Any(), int64(5)).DoAndReturn(roster).AnyTimes()
	f.matches.EXPECT().Get(gomock.Any(), int64(5), int64(1)).DoAndReturn(func(context.Context, int64, int64) (Match, error) { return f.match, nil }).AnyTimes()
	f.matches.EXPECT().Events(gomock.Any(), int64(5), int64(1)).Return([]MatchEvent{{Kind: "ended", By: "Ann"}}, nil).AnyTimes()
	f.matches.EXPECT().Update(gomock.Any(), int64(5), int64(1), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ int64, _ User, fn func(*Match) error) (Match, error) {
			m := f.match
			if err := fn(&m); err != nil {
				return Match{}, err
			}
			m.Revision++
			f.match = m
			return m, nil
		}).AnyTimes()
	f.rulesets.EXPECT().Get(gomock.Any(), "bc").DoAndReturn(func(context.Context, string) (StoredRuleset, error) {
		return StoredRuleset{Content: readDef(t, "battle_companies/battle_companies.json")}, nil
	}).AnyTimes()
	f.svc = NewMatches(f.campaigns, f.companies, f.matches, f.rulesets, staticRules{shippedRules(t)})
	return f
}

func (f *matchesFixture) player() User { return User{ID: 10, Role: RoleUser} }

func (f *matchesFixture) firstScenario(t *testing.T) ScenarioInfo {
	t.Helper()
	list, err := f.svc.Scenarios(f.ctx, 5)
	if err != nil || len(list) < 18 {
		t.Fatalf("scenarios = %d, %v", len(list), err)
	}
	return list[0]
}

func TestMatchesReadFillsRatingAndHistory(t *testing.T) {
	f := newMatchesFixture(t)
	m, err := f.svc.Match(f.ctx, 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.History) != 1 || m.History[0].By != "Ann" {
		t.Errorf("history = %+v", m.History)
	}
	if m.Sides[0].Companies[0].Points == 0 || m.Sides[1].Companies[1].Points == 0 {
		t.Errorf("companies are rated: %+v", m.Sides)
	}
	if f.match.Sides[0].Companies[0].Points != 0 {
		t.Error("the stored match is not changed")
	}
	list := []Match{testMatch()}
	f.matches.EXPECT().List(gomock.Any(), int64(5)).Return(list, nil)
	got, err := f.svc.Matches(f.ctx, 5)
	if err != nil || got[0].Sides[1].Companies[0].Points == 0 {
		t.Fatalf("Matches = %+v, %v", got, err)
	}
	if _, err := f.svc.Matches(f.ctx, 404); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown campaign: %v", err)
	}
}

func TestMatchesSetScenarioAndAccess(t *testing.T) {
	f := newMatchesFixture(t)
	sc := f.firstScenario(t)
	in := ScenarioInput{Kind: ScenarioRulebook, Name: sc.Name}
	tests := []struct {
		name     string
		user     User
		revision int
		in       ScenarioInput
		ended    bool
		want     error
	}{
		{"participant", User{ID: 10}, 3, in, false, nil},
		{"organiser", User{ID: matchOrganiser}, 3, in, false, nil},
		{"stranger", User{ID: 7}, 3, in, false, ErrRefused},
		{"participant, ended", User{ID: 10}, 3, in, true, ErrRefused},
		{"admin, ended", User{ID: 5, Role: RoleAdmin}, 3, in, true, nil},
		{"stale", User{ID: 10}, 2, in, false, ErrMatchChanged},
		{"unknown rulebook scenario", User{ID: 10}, 3, ScenarioInput{Kind: ScenarioRulebook, Name: "Nope"}, false, ErrInvalidMatch},
		{"invalid", User{ID: 10}, 3, ScenarioInput{Kind: ScenarioCustom}, false, ErrInvalidMatch},
		{"custom", User{ID: 10}, 3, ScenarioInput{Kind: ScenarioCustom, Name: "Tavern brawl", Description: "d"}, false, nil},
	}
	for _, tt := range tests {
		f.match = testMatch()
		f.match.Revision, f.match.Ended = 3, tt.ended
		m, err := f.svc.SetScenario(f.ctx, tt.user, 5, 1, tt.revision, tt.in)
		if tt.want == nil {
			if err != nil || m.Scenario == nil || m.Scenario.Name != tt.in.Name || m.Revision != 4 {
				t.Errorf("%s: %+v, %v", tt.name, m, err)
			}
		} else if !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.want)
		} else if f.match.Revision != 3 || f.match.Scenario != nil {
			t.Errorf("%s: nothing is stored on failure", tt.name)
		}
	}
}

func TestMatchesChangingTheScenarioClearsObjectives(t *testing.T) {
	f := newMatchesFixture(t)
	f.match.Scenario = &MatchScenario{Kind: ScenarioCustom, Name: "x"}
	f.match.Result.Companies = []CompanyResult{{CompanyID: 1, Fled: true, Objectives: map[string]int{"k": 1}}}
	m, err := f.svc.ClearScenario(f.ctx, f.player(), 5, 1, 3)
	if err != nil || m.Scenario != nil {
		t.Fatalf("ClearScenario = %+v, %v", m.Scenario, err)
	}
	if c := m.Result.Companies[0]; !c.Fled || c.Objectives != nil {
		t.Errorf("objectives go, fled stays: %+v", c)
	}
}

func TestMatchesSaveResult(t *testing.T) {
	f := newMatchesFixture(t)
	vp := 12
	good := MatchResult{Decided: true, Winner: 2, Sides: []SideResult{{VictoryPoints: &vp}, {}}, Companies: []CompanyResult{{CompanyID: 3, Fled: true}}, Note: " fun "}
	m, err := f.svc.SaveResult(f.ctx, f.player(), 5, 1, 3, good)
	if err != nil || m.Winner != 2 || m.Result.Note != "fun" || !m.Result.Company(3).Fled {
		t.Fatalf("SaveResult = %+v, %v", m.Result, err)
	}
	bad := good
	bad.Winner = 7
	if _, err := f.svc.SaveResult(f.ctx, f.player(), 5, 1, 4, bad); !errors.Is(err, ErrInvalidMatch) {
		t.Errorf("bad winner: %v", err)
	}
	if _, err := f.svc.SaveResult(f.ctx, f.player(), 5, 1, 3, good); !errors.Is(err, ErrMatchChanged) {
		t.Errorf("stale: %v", err)
	}
	f.match.Scenario = &MatchScenario{Kind: ScenarioCustom, Name: "x"}
	withObj := good
	withObj.Companies = []CompanyResult{{CompanyID: 1, Objectives: map[string]int{"allCargo": 1}}}
	if _, err := f.svc.SaveResult(f.ctx, f.player(), 5, 1, 4, withObj); !errors.Is(err, ErrInvalidMatch) {
		t.Errorf("a one-off scenario has no objectives: %v", err)
	}
}

func TestMatchesSaveResultKnowsRulebookObjectives(t *testing.T) {
	f := newMatchesFixture(t)
	list, _ := f.svc.Scenarios(f.ctx, 5)
	var sc ScenarioInfo
	for _, s := range list {
		if len(s.Objectives) > 0 {
			sc = s
			break
		}
	}
	if sc.Name == "" {
		t.Skip("the rules file has no scenario with objectives")
	}
	f.match.Scenario = &MatchScenario{Kind: ScenarioRulebook, Name: sc.Name}
	r := MatchResult{Sides: make([]SideResult, 2), Companies: []CompanyResult{{CompanyID: 1, Objectives: map[string]int{sc.Objectives[0].Key: 1}}}}
	m, err := f.svc.SaveResult(f.ctx, f.player(), 5, 1, 3, r)
	if err != nil || m.Result.Company(1).Objectives[sc.Objectives[0].Key] != 1 {
		t.Fatalf("SaveResult = %+v, %v", m.Result, err)
	}
}

func TestMatchesEndAndReopen(t *testing.T) {
	f := newMatchesFixture(t)
	admin := User{ID: 5, Role: RoleAdmin}
	if _, err := f.svc.endMatch(f.ctx, f.player(), 5, 1, 3); !errors.Is(err, ErrRefused) {
		t.Errorf("no scenario, no result: %v", err)
	}
	f.match.Scenario = &MatchScenario{Kind: ScenarioCustom, Name: "x"}
	f.match.Result = MatchResult{Decided: true, Winner: 1, Sides: make([]SideResult, 2)}
	m, err := f.svc.endMatch(f.ctx, f.player(), 5, 1, 3)
	if err != nil || !m.Ended || m.EndedOn == nil || m.Revision != 4 {
		t.Fatalf("endMatch = %+v, %v", m, err)
	}
	if _, err := f.svc.ReopenMatch(f.ctx, f.player(), 5, 1, 4); !errors.Is(err, ErrRefused) {
		t.Errorf("players cannot reopen: %v", err)
	}
	started := true
	f.svc.AftermathStarted = func(context.Context, int64, int64) (bool, error) { return started, nil }
	if _, err := f.svc.ReopenMatch(f.ctx, admin, 5, 1, 4); !errors.Is(err, ErrRefused) {
		t.Errorf("aftermath started: %v", err)
	}
	started = false
	if _, err := f.svc.ReopenMatch(f.ctx, admin, 5, 1, 3); !errors.Is(err, ErrMatchChanged) {
		t.Errorf("stale: %v", err)
	}
	m, err = f.svc.ReopenMatch(f.ctx, admin, 5, 1, 4)
	if err != nil || m.Ended || m.EndedOn != nil {
		t.Fatalf("ReopenMatch = %+v, %v", m, err)
	}
	if _, err := f.svc.ReopenMatch(f.ctx, admin, 5, 1, 5); !errors.Is(err, ErrRefused) {
		t.Errorf("not ended: %v", err)
	}
}

func TestMatchesCreate(t *testing.T) {
	f := newMatchesFixture(t)
	sides := []NewSide{{Companies: []int64{1}}, {Companies: []int64{2}}}
	want := []NewSide{{Name: "Side 1", Companies: []int64{1}}, {Name: "Side 2", Companies: []int64{2}}}
	f.matches.EXPECT().Create(gomock.Any(), int64(5), f.player(), want).Return(f.match, nil)
	if m, err := f.svc.CreateMatch(f.ctx, f.player(), 5, sides); err != nil || m.ID != 1 {
		t.Fatalf("CreateMatch = %+v, %v", m, err)
	}
	if _, err := f.svc.CreateMatch(f.ctx, f.player(), 5, sides[:1]); !errors.Is(err, ErrInvalidMatch) {
		t.Errorf("one side: %v", err)
	}
	f.state.Units[1].Path = ""
	if _, err := f.svc.CreateMatch(f.ctx, f.player(), 5, sides); !errors.Is(err, ErrRefused) {
		t.Errorf("an officer without a path cannot play: %v", err)
	}
	f.state.Units[1].Path = "Path of the Warlord"
	f.state.Recruit.Officers = &OfficerStep{Unit: "Ranger of the North"}
	if _, err := f.svc.CreateMatch(f.ctx, f.player(), 5, sides); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "arrange the officers") {
		t.Errorf("a pending officer arrangement cannot play: %v", err)
	}
}

func TestMatchesGrants(t *testing.T) {
	f := newMatchesFixture(t)
	org := User{ID: matchOrganiser, DisplayName: "Org"}
	in := GrantInput{Gold: 50, Experience: 2, UnitID: 3, Note: " Rescued the pony "}
	f.matches.EXPECT().AddGrant(gomock.Any(), int64(5), org, gomock.Any()).DoAndReturn(func(_ context.Context, _ int64, _ User, g Grant) (Grant, error) {
		if g.Kind != GrantOrganiser || g.Gold != 50 || g.Note != "Rescued the pony" || g.By != "Org" || g.CompanyID != 1 || g.UnitID != 3 {
			t.Errorf("grant = %+v", g)
		}
		g.ID = 8
		return g, nil
	})
	if g, err := f.svc.GrantReward(f.ctx, org, 5, 1, in); err != nil || g.ID != 8 {
		t.Fatalf("GrantReward = %+v, %v", g, err)
	}
	tests := []struct {
		name string
		user User
		in   GrantInput
		want error
	}{
		{"a player", f.player(), in, ErrRefused},
		{"no note", org, GrantInput{Gold: 1}, ErrInvalidCompany},
		{"unit not on roster", org, GrantInput{Gold: 1, Experience: 1, UnitID: 999, Note: "x"}, ErrRefused},
		{"below zero", org, GrantInput{Gold: -1000, Note: "x"}, ErrRefused},
		{"influence below zero", org, GrantInput{Influence: -50, Note: "x"}, ErrRefused},
	}
	for _, tt := range tests {
		if _, err := f.svc.GrantReward(f.ctx, tt.user, 5, 1, tt.in); !errors.Is(err, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, err, tt.want)
		}
	}
	f.matches.EXPECT().Grants(gomock.Any(), int64(5), int64(1)).Return([]Grant{{ID: 8}}, nil)
	if got, err := f.svc.Grants(f.ctx, 5, 1); err != nil || len(got) != 1 {
		t.Errorf("Grants = %v, %v", got, err)
	}
}

func TestMatchesFinishedCampaignIsReadOnly(t *testing.T) {
	f := newMatchesFixture(t)
	f.campaigns.EXPECT().Get(gomock.Any(), gomock.Any(), int64(6)).Return(CampaignDetail{CampaignSummary: CampaignSummary{ID: 6, Status: CampaignFinished}, OrganiserID: matchOrganiser}, nil).AnyTimes()
	f.campaigns.EXPECT().Hashes(gomock.Any(), int64(6)).Return(RulesetHashes{BattleCompanies: "bc"}, CampaignSettings{}, nil).AnyTimes()
	admin := User{ID: 5, Role: RoleAdmin}
	sides := []NewSide{{Companies: []int64{1}}, {Companies: []int64{2}}}
	calls := map[string]error{}
	_, calls["create"] = f.svc.CreateMatch(f.ctx, f.player(), 6, sides)
	_, calls["scenario"] = f.svc.SetScenario(f.ctx, f.player(), 6, 1, 3, ScenarioInput{Kind: ScenarioCustom, Name: "x"})
	_, calls["clear"] = f.svc.ClearScenario(f.ctx, f.player(), 6, 1, 3)
	_, calls["result"] = f.svc.SaveResult(f.ctx, f.player(), 6, 1, 3, MatchResult{})
	_, calls["end"] = f.svc.endMatch(f.ctx, f.player(), 6, 1, 3)
	_, calls["reopen"] = f.svc.ReopenMatch(f.ctx, admin, 6, 1, 3)
	_, calls["admin result"] = f.svc.SaveResult(f.ctx, admin, 6, 1, 3, MatchResult{})
	_, calls["side roles"] = f.svc.SetSideRoles(f.ctx, f.player(), 6, 1, 3, []string{"attacker", ""})
	_, calls["swap roles"] = f.svc.SwapSideRoles(f.ctx, f.player(), 6, 1, 3)
	for name, err := range calls {
		if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "The campaign is finished.") {
			t.Errorf("%s in a finished campaign: %v", name, err)
		}
	}
}

func TestMatchesEndedMatchSaysSo(t *testing.T) {
	f := newMatchesFixture(t)
	f.match.Ended = true
	_, err := f.svc.SaveResult(f.ctx, f.player(), 5, 1, 3, MatchResult{})
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "This match has ended") {
		t.Errorf("a participant after the end: %v", err)
	}
	if got := (Match{Ended: false}).NoAccessReason(); !strings.Contains(got, "Only the players") {
		t.Errorf("open: %q", got)
	}
}

func TestMatchesPausedCampaignFinishesOpenMatchesButStartsNone(t *testing.T) {
	f := newMatchesFixture(t)
	f.campaigns.EXPECT().Get(gomock.Any(), gomock.Any(), int64(7)).Return(CampaignDetail{CampaignSummary: CampaignSummary{ID: 7, Status: CampaignPaused}, OrganiserID: matchOrganiser}, nil).AnyTimes()
	f.campaigns.EXPECT().Hashes(gomock.Any(), int64(7)).Return(RulesetHashes{BattleCompanies: "bc"}, CampaignSettings{}, nil).AnyTimes()
	f.matches.EXPECT().Get(gomock.Any(), int64(7), int64(1)).DoAndReturn(func(context.Context, int64, int64) (Match, error) { return f.match, nil }).AnyTimes()
	f.matches.EXPECT().Events(gomock.Any(), int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.matches.EXPECT().Update(gomock.Any(), int64(7), int64(1), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ int64, _ User, fn func(*Match) error) (Match, error) {
			m := f.match
			if err := fn(&m); err != nil {
				return Match{}, err
			}
			return m, nil
		}).AnyTimes()
	f.companies.EXPECT().Read(gomock.Any(), int64(7), gomock.Any()).DoAndReturn(func(context.Context, int64, int64) (CompanyState, error) { return f.state.Clone(), nil }).AnyTimes()
	f.companies.EXPECT().ReadRoster(gomock.Any(), int64(7)).DoAndReturn(func(context.Context, int64) ([]CompanyState, error) {
		a, b := f.state.Clone(), f.state.Clone()
		a.Company.ID, b.Company.ID = 1, 2
		return []CompanyState{a, b}, nil
	}).AnyTimes()
	_, err := f.svc.CreateMatch(f.ctx, f.player(), 7, []NewSide{{Companies: []int64{1}}, {Companies: []int64{2}}})
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "This campaign is paused.") {
		t.Errorf("a new match in a paused campaign: %v", err)
	}
	if _, err := f.svc.SetScenario(f.ctx, f.player(), 7, 1, 3, ScenarioInput{Kind: ScenarioCustom, Name: "Duel"}); err != nil {
		t.Errorf("an open match is edited while paused: %v", err)
	}
	if _, err := f.svc.endMatch(f.ctx, f.player(), 7, 1, 3); errors.Is(err, ErrCampaignPaused) {
		t.Errorf("an open match can be ended while paused: %v", err)
	}
}

func TestSideRolesValidationSwapAndEnd(t *testing.T) {
	if _, err := ValidateSides([]NewSide{{Companies: []int64{1}, Role: "attacker"}, {Companies: []int64{2}, Role: "attacker"}}); !errors.Is(err, ErrInvalidMatch) {
		t.Errorf("two attackers: %v", err)
	}
	if _, err := ValidateSides([]NewSide{{Companies: []int64{1}, Role: "king"}, {Companies: []int64{2}}}); !errors.Is(err, ErrInvalidMatch) {
		t.Errorf("unknown role: %v", err)
	}
	if _, err := ValidateSides([]NewSide{{Companies: []int64{1}, Role: "attacker"}, {Companies: []int64{2}, Role: "defender"}}); err != nil {
		t.Errorf("both roles: %v", err)
	}

	f := newMatchesFixture(t)
	p := f.player()
	rev := func() int { return f.match.Revision }
	if _, err := f.svc.SetSideRoles(f.ctx, p, 5, 1, rev(), []string{"attacker"}); !errors.Is(err, ErrInvalidMatch) {
		t.Errorf("one role per side: %v", err)
	}
	if _, err := f.svc.SetSideRoles(f.ctx, p, 5, 1, rev(), []string{"defender", "defender"}); !errors.Is(err, ErrInvalidMatch) {
		t.Errorf("duplicate: %v", err)
	}
	m, err := f.svc.SetSideRoles(f.ctx, p, 5, 1, rev(), []string{"attacker", ""})
	if err != nil || m.RoleOf(1) != "attacker" || m.RoleOf(2) != "" || m.RoleOf(3) != "" {
		t.Fatalf("set: %v %+v", err, m.Sides)
	}
	m, err = f.svc.SwapSideRoles(f.ctx, p, 5, 1, rev())
	if err != nil || m.RoleOf(1) != "defender" || m.RoleOf(2) != "attacker" {
		t.Fatalf("swap with one tag: %v %+v", err, m.Sides)
	}
	m, _ = f.svc.SwapSideRoles(f.ctx, p, 5, 1, rev())
	if m.RoleOf(1) != "attacker" || m.RoleOf(2) != "defender" {
		t.Errorf("swap back: %+v", m.Sides)
	}

	// a scenario with roles needs both tags to end (two teams only)
	list, _ := f.svc.Scenarios(f.ctx, 5)
	var roles, plain string
	for _, sc := range list {
		if sc.HasRoles() && roles == "" {
			roles = sc.Name
		}
		if !sc.HasRoles() && plain == "" {
			plain = sc.Name
		}
	}
	f.match.Sides = []MatchSide{{Name: "A", Companies: f.match.Sides[0].Companies}, {Name: "B", Companies: f.match.Sides[1].Companies}}
	f.match.Scenario = &MatchScenario{Kind: ScenarioRulebook, Name: roles}
	f.match.Result = MatchResult{Decided: true, Winner: 1}
	f.match.Winner = 1
	if _, err := f.svc.endMatch(f.ctx, p, 5, 1, rev()); !errors.Is(err, ErrInvalidMatch) || !strings.Contains(err.Error(), "which team attacks") {
		t.Errorf("ending without tags: %v", err)
	}
	if _, err := f.svc.SetSideRoles(f.ctx, p, 5, 1, rev(), []string{"attacker", "defender"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.endMatch(f.ctx, p, 5, 1, rev()); err != nil {
		t.Errorf("tagged: %v", err)
	}
	if _, err := f.svc.SetSideRoles(f.ctx, p, 5, 1, rev(), []string{"", ""}); !errors.Is(err, ErrRefused) {
		t.Errorf("roles of an ended match: %v", err)
	}
	f.match.Ended = false
	f.match.Scenario = &MatchScenario{Kind: ScenarioRulebook, Name: plain}
	f.match.Sides[0].Role, f.match.Sides[1].Role = "", ""
	if _, err := f.svc.endMatch(f.ctx, p, 5, 1, rev()); err != nil {
		t.Errorf("a scenario without roles ends without tags: %v", err)
	}
}

func TestScenariosKnowTheirScoringObjectivesBySide(t *testing.T) {
	f := newMatchesFixture(t)
	list, _ := f.svc.Scenarios(f.ctx, 5)
	var stand, fog ScenarioInfo
	for _, sc := range list {
		switch sc.Name {
		case "Stand Your Ground":
			stand = sc
		case "Fog of War":
			fog = sc
		}
	}
	if !stand.HasRoles() || len(stand.ScoringObjectives) == 0 {
		t.Fatalf("Stand Your Ground: %+v", stand.About.Roles)
	}
	if len(stand.ObjectivesFor("")) != len(stand.ScoringObjectives) {
		t.Error("without a tag all objectives are listed")
	}
	for _, o := range stand.ObjectivesFor("attacker") {
		if o.Side == "defender" {
			t.Errorf("a defender's objective for the attacker: %+v", o)
		}
	}
	secret := 0
	for _, o := range fog.ScoringObjectives {
		if o.Secret {
			secret++
			if o.Nomination == nil && len(o.Options) == 0 {
				t.Errorf("secret objective without data: %+v", o)
			}
		}
	}
	if secret == 0 {
		t.Error("Fog of War has secret objectives")
	}
}

// The organiser manages the campaign like an administrator: reopens an ended match and corrects it.
func TestOrganiserReopensAndEditsEndedMatches(t *testing.T) {
	f := newMatchesFixture(t)
	org := User{ID: matchOrganiser, Role: RoleUser}
	f.match.Ended = true
	f.match.Revision = 4
	if _, err := f.svc.SaveResult(f.ctx, f.player(), 5, 1, 4, MatchResult{}); !errors.Is(err, ErrRefused) {
		t.Errorf("a player cannot edit an ended match: %v", err)
	}
	if _, err := f.svc.SaveResult(f.ctx, org, 5, 1, 4, MatchResult{Decided: true, Winner: 1, Sides: make([]SideResult, 2)}); err != nil {
		t.Errorf("the organiser corrects an ended match: %v", err)
	}
	m, err := f.svc.ReopenMatch(f.ctx, org, 5, 1, 5)
	if err != nil || m.Ended {
		t.Errorf("the organiser reopens: %+v %v", m, err)
	}
	if _, err := f.svc.ReopenMatch(f.ctx, User{ID: 77}, 5, 1, 6); !errors.Is(err, ErrRefused) {
		t.Errorf("a stranger to the match: %v", err)
	}
}

func TestObjectiveLabel(t *testing.T) {
	for in, want := range map[string]string{"ownHeroSurvives": "Own hero survives", "enemyHeroHarmed": "Enemy hero harmed", "terrainPiece": "Terrain piece", "enemyLeader": "Enemy leader"} {
		if got := ObjectiveLabel(in); got != want {
			t.Errorf("%s: %q", in, got)
		}
	}
}
