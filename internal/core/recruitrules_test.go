package core

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"
)

// recruiterWith is f.recruiter on a rule set whose recruiting parameters mutate changes.
func (f *companyFixture) recruiterWith(t *testing.T, mutate func(doc map[string]any), dice ...int) *RecruitService {
	t.Helper()
	f.update()
	f.settings.ManualRolls = true
	rs := customRuleset(t, mutate)
	return NewRecruitService(f.campaigns, f.companies, staticRules{rs}, &FixedDice{Rolls: dice})
}

func setAdjust(t *testing.T, max, cost float64, direction string) func(doc map[string]any) {
	return func(doc map[string]any) {
		a := jsonAt(t, doc, "campaign", "recruitment", "adjust").(map[string]any)
		a["maxPerAttempt"], a["costPerPoint"], a["direction"] = max, cost, direction
	}
}

func TestAdjustmentFollowsTheRulesFile(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(t *testing.T) func(map[string]any)
		roll      int
		influence int
		adjust    int
		wantFinal string // text in the summary
		wantLeft  int    // influence left after the roll (3) and the adjustment
		refused   bool
	}{
		{"default: +3 at 1 each", func(t *testing.T) func(map[string]any) { return nil }, 1, 9, 3, "raised to a 4", 3, false},
		{"default: lowering is ignored", func(t *testing.T) func(map[string]any) { return nil }, 4, 9, -2, "You rolled a 4: ", 6, false},
		{"cap 5: +5 from a 1 is a 6", func(t *testing.T) func(map[string]any) { return setAdjust(t, 5, 1, "up") }, 1, 20, 5, "raised to a 6", 12, false},
		{"cap 5: +6 is held to the cap", func(t *testing.T) func(map[string]any) { return setAdjust(t, 5, 1, "up") }, 1, 20, 6, "raised to a 6", 12, false},
		{"cost 2: +2 is 4 influence", func(t *testing.T) func(map[string]any) { return setAdjust(t, 3, 2, "up") }, 2, 20, 2, "raised to a 4", 13, false},
		{"cost 2: not enough influence", func(t *testing.T) func(map[string]any) { return setAdjust(t, 3, 2, "up") }, 2, 6, 2, "", 0, true},
		{"both: lowering is paid", func(t *testing.T) func(map[string]any) { return setAdjust(t, 3, 2, "both") }, 5, 20, -2, "lowered to a 3", 13, false},
		{"down: raising is ignored", func(t *testing.T) func(map[string]any) { return setAdjust(t, 3, 1, "down") }, 3, 20, 2, "You rolled a 3: ", 17, false},
		{"down: lowering stops at 1", func(t *testing.T) func(map[string]any) { return setAdjust(t, 5, 1, "down") }, 2, 20, -4, "lowered to a 1", 16, false},
		{"free adjustment (cost 0)", func(t *testing.T) func(map[string]any) { return setAdjust(t, 3, 0, "both") }, 3, 3, 3, "raised to a 6", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCompanyFixture(t)
			s := f.recruiterWith(t, tt.mutate(t))
			f.state.Influence = tt.influence
			res, err := recruit(t, s, f, tt.roll, tt.adjust)
			if tt.refused {
				if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "costs 4 influence") {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil || !strings.Contains(res.Summary, tt.wantFinal) {
				t.Fatalf("%+v %v", res, err)
			}
			if f.state.Influence != tt.wantLeft {
				t.Errorf("influence left = %d, want %d", f.state.Influence, tt.wantLeft)
			}
		})
	}
}

// The points of an attempt are shared by both charts, whichever way they went.
func TestAdjustedPointsCountBothWaysAcrossTheCharts(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.recruiterWith(t, setAdjust(t, 3, 1, "both"))
	f.state.Influence = 20
	if err := s.RollRecruit(f.ctx, f.user, 5, 1, 5, false); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.RecruitState(f.ctx, f.user, 5, 1); st == nil || st.Budget != 1 || st.BudgetDown != 3 || st.Adjust.Max != 3 || st.Sides != 6 {
		t.Fatalf("a 5: up 1, down 3: %+v", st)
	}
	if res, err := s.ConfirmRecruit(f.ctx, f.user, 5, 1, 1, ""); err != nil || !res.Next { // a 6: the special chart, 1 of 3 points used
		t.Fatalf("%+v %v", res, err)
	}
	if st, _ := s.RecruitState(f.ctx, f.user, 5, 1); st == nil || st.Stage != "special" || st.BudgetDown != 0 || st.Budget != 2 {
		// not rolled yet: nothing to lower (roll 0), two points left to raise
		t.Errorf("special chart: %+v", st)
	}
	if err := s.RollRecruit(f.ctx, f.user, 5, 1, 4, false); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.RecruitState(f.ctx, f.user, 5, 1); st == nil || st.Budget != 2 || st.BudgetDown != 2 {
		t.Errorf("a 4 with 2 points left: %+v", st)
	}
}

func TestRendezvousFollowsItsRange(t *testing.T) {
	for _, tt := range []struct {
		name    string
		low     float64
		high    float64
		perk    int
		roll    int
		wantErr bool
		want    string
	}{
		{"default +1", -1, 1, 1, 3, false, "Rendezvous +1 = 4"},
		{"default refuses +2", -1, 1, 2, 3, true, "+1 or -1"},
		{"wider range takes +2", -2, 2, 2, 3, false, "Rendezvous +2 = 5"},
		{"wider range takes -2", -2, 2, -2, 4, false, "Rendezvous -2 = 2"},
		{"the die stays on its faces", -2, 2, 2, 5, true, "cannot be moved"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newCompanyFixture(t)
			s := f.recruiterWith(t, func(doc map[string]any) {
				for _, e := range jsonAt(t, doc, "campaign", "aftermathEffects").([]any) {
					if m := e.(map[string]any); m["key"] == "rendezvous" {
						m["range"] = []any{tt.low, tt.high}
					}
				}
			})
			f.state.Influence = 20
			f.state.Perks = []Perk{{ID: 1, Effect: PerkRendezvous}}
			err := s.RollRecruitWithPerk(f.ctx, f.user, 5, 1, tt.roll, false, tt.perk)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidRoll) || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			res, err := s.ConfirmRecruit(f.ctx, f.user, 5, 1, 0, "")
			if err != nil || !strings.Contains(res.Summary, tt.want) {
				t.Errorf("%+v %v", res, err)
			}
		})
	}
}

func TestRareCountingFollowsTheFlags(t *testing.T) {
	roster := []CompanyUnit{
		{Type: "Hunter"},
		{Type: "Hunter", Injured: true},
		{Type: "Ranger", PromotedFrom: "Hunter"},
	}
	fallen := []FallenUnit{{Unit: CompanyUnit{Type: "Hunter"}, Fate: "Dead"}, {Unit: CompanyUnit{Type: "Hunter"}, Fate: "Retired"}}
	isHunter := func(t string) bool { return t == "Hunter" }
	tests := []struct {
		name  string
		rules RareRules
		want  int
	}{
		{"shipped: injured count, dead and promoted do not", RareRules{Injured: true}, 2},
		{"injured do not count", RareRules{}, 1},
		{"dead count", RareRules{Injured: true, Dead: true}, 3},
		{"promoted count", RareRules{Injured: true, Promoted: true}, 3},
		{"all count", RareRules{Injured: true, Dead: true, Promoted: true}, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rareCount(tt.rules, roster, fallen, isHunter); got != tt.want {
				t.Errorf("count = %d, want %d", got, tt.want)
			}
		})
	}
	if why, _ := rareBlock(RareRules{Injured: true}, roster, fallen, "Hunter", 2); why == "" {
		t.Error("two Hunters in the roster fill Rare 2")
	}
	if why, _ := rareBlock(RareRules{}, roster, fallen, "Hunter", 2); why != "" {
		t.Errorf("the injured one does not count when the rules say so: %s", why)
	}
	if why, _ := rareBlock(RareRules{}, roster, fallen, "Hunter", 0); why != "" {
		t.Errorf("no Rare limit: %s", why)
	}
}

func TestStartingInfluenceComesFromTheRules(t *testing.T) {
	rs := customRuleset(t, func(doc map[string]any) {
		jsonAt(t, doc, "campaign", "company").(map[string]any)["startingInfluence"] = 2.0
	})
	profile, _ := rs.BC.Profile("Minas Tirith")
	f := newCompanyFixture(t)
	f.campaigns.EXPECT().Get(gomock.Any(), int64(10), int64(5)).Return(CampaignDetail{}, nil).AnyTimes()
	var got NewCompany
	f.companies.EXPECT().Create(gomock.Any(), int64(5), User{ID: 10}, gomock.Any()).DoAndReturn(func(_ context.Context, _ int64, _ User, c NewCompany) (CampaignCompany, error) {
		got = c
		return CampaignCompany{ID: 1, Name: c.Name, Faction: c.Faction}, nil
	})
	svc := NewCompanyService(f.campaigns, f.companies, staticRules{rs})
	if _, err := svc.CreateCompany(f.ctx, User{ID: 10}, 5, CompanyInput{Name: "Guard", Faction: profile.Name}); err != nil {
		t.Fatal(err)
	}
	if got.Influence != 2 {
		t.Errorf("a new company starts with %d influence, the file says 2", got.Influence)
	}
}

// A promoted unit remembers the profile it was recruited as, so Rare can count it where the rules say so.
func TestPromotedUnitKnowsItsOrigin(t *testing.T) {
	rs := shippedRules(t)
	st := CompanyState{Company: CampaignCompany{ID: 1, Faction: "Minas Tirith"}, Units: []UnitState{
		{ID: 1, Base: "Warrior of Minas Tirith", Role: RoleWarrior, Progress: &Progress{Base: "Citadel Guard"}},
		{ID: 2, Base: "Warrior of Minas Tirith", Role: RoleWarrior},
	}}
	d := BuildDetail(rs, CampaignSettings{}, st)
	if d.Roster[0].Type != "Citadel Guard" || d.Roster[0].PromotedFrom != "Warrior of Minas Tirith" || d.Roster[1].PromotedFrom != "" {
		t.Errorf("roster = %q from %q, %q from %q", d.Roster[0].Type, d.Roster[0].PromotedFrom, d.Roster[1].Type, d.Roster[1].PromotedFrom)
	}
}

func TestPerkRangeInWords(t *testing.T) {
	for r, want := range map[PerkRange]string{{-1, 1}: "+1 or -1", {-2, 2}: "-2 to +2", {1, 1}: "+1", {0, 3}: "+0 to +3"} {
		if got := r.Describe(); got != want {
			t.Errorf("%+v = %q, want %q", r, got, want)
		}
	}
}
