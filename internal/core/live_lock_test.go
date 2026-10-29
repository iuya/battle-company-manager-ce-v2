package core

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

// While the company's match is running every roster action is refused with the match number, exactly as for a pending aftermath;
// renaming units and the formation stay allowed.
func TestCompanyActionsAreLockedWhileAMatchIsRunning(t *testing.T) {
	rules := staticRules{shippedRules(t)}
	f := newCompanyFixture(t)
	f.update()
	f.state.OpenMatch = &OpenMatchRef{MatchID: 1, Number: 3}
	arm := NewArmouryService(f.campaigns, f.companies, rules)
	lvl := NewLevelService(f.campaigns, f.companies, rules, &FixedDice{Rolls: []int{3}})
	rec := NewRecruitService(f.campaigns, f.companies, rules, &FixedDice{Rolls: []int{3}})
	_, rollErr := lvl.RollLevelUp(f.ctx, f.user, 5, 1, 1, nil, false)
	_, chooseErr := lvl.ChooseLevelUp(f.ctx, f.user, 5, 1, 1, []string{"x"})
	_, rerollErr := lvl.RerollLevelUp(f.ctx, f.user, 5, 1, 1, 0)
	_, specialErr := rec.RecruitSpecial(f.ctx, f.user, 5, 1, "Anybody")
	_, treatErr := NewTreatmentService(f.campaigns, f.companies, rules, &FixedDice{}).TreatInjury(f.ctx, f.user, 5, 1, 7, Treatment{Method: "warrior"})
	for name, err := range map[string]error{
		"buy":      arm.BuyEquipment(f.ctx, f.user, 5, 1, 6, "Shield", ""),
		"drop":     arm.DropEquipment(f.ctx, f.user, 5, 1, 6, "Bow"),
		"creature": arm.BuyCreature(f.ctx, f.user, 5, 1, 2, "Baggage Pony", ""),
		"wanderer": arm.HireWanderer(f.ctx, f.user, 5, 1, "Anybody", ""),
		"dismiss":  arm.DismissWanderer(f.ctx, f.user, 5, 1, 6),
		"carry":    arm.Carry(f.ctx, f.user, 5, 1, 6, CarryAction{Kind: "store", Item: "Bow"}),
		"recruit":  rec.RollRecruit(f.ctx, f.user, 5, 1, 0, false),
		"special":  specialErr,
		"treat":    treatErr,
		"level":    rollErr,
		"choose":   chooseErr,
		"reroll":   rerollErr,
		"path":     lvl.ChoosePath(f.ctx, f.user, 5, 1, 2, "Path of the Warlord", ""),
	} {
		if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "Match 3 is running; finish it first.") {
			t.Errorf("%s: %v", name, err)
		}
	}
	// the open match message wins over an aftermath
	f.state.Aftermaths = []AftermathRef{{MatchID: 1, MatchNumber: 2, CompanyID: 1, Status: AftermathPending}}
	if err := arm.DropEquipment(f.ctx, f.user, 5, 1, 6, "Bow"); err == nil || !strings.Contains(err.Error(), "Match 3 is running") {
		t.Errorf("both set: %v", err)
	}
	f.state.Aftermaths = nil

	svc := NewCompanyService(f.campaigns, f.companies, rules)
	if err := svc.RenameUnit(f.ctx, f.user, 5, 1, 6, "Cirion the Bold"); err != nil {
		t.Errorf("renaming a unit: %v", err)
	}
	var order []int64
	for _, u := range f.state.Units {
		order = append(order, u.ID)
	}
	if err := svc.SaveFormation(f.ctx, f.user, 5, 1, order); err != nil {
		t.Errorf("formation: %v", err)
	}
	if err := NewCemeteryService(f.campaigns, f.companies, rules).SaveCemeteryOrder(f.ctx, f.user, 5, 1, nil); err != nil {
		t.Errorf("cemetery order: %v", err)
	}
	f.state.OpenMatch = nil
	if err := arm.DropEquipment(f.ctx, f.user, 5, 1, 6, "Bow"); err != nil && strings.Contains(err.Error(), "is running") {
		t.Errorf("unlocked: %v", err)
	}
}

func TestCreateMatchGuards(t *testing.T) {
	f := newMatchesFixture(t)
	sides := []NewSide{{Name: "A", Companies: []int64{1}}, {Name: "B", Companies: []int64{2}}}
	var created, inited bool
	f.matches.EXPECT().Create(gomock.Any(), int64(5), gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, int64, User, []NewSide) (Match, error) {
		created = true
		return f.match, nil
	}).AnyTimes()
	f.svc.LiveInit = func(context.Context, int64, int64) error { inited = true; return errors.New("database down") }

	f.svc.CompanyAftermaths = func(_ context.Context, _, companyID int64) ([]AftermathRef, error) {
		if companyID == 2 {
			return []AftermathRef{{MatchID: 9, MatchNumber: 4, CompanyID: 2}}, nil
		}
		return nil, nil
	}
	_, err := f.svc.CreateMatch(f.ctx, f.player(), 5, sides)
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "Finish the aftermath of Match 4 first.") || created {
		t.Errorf("a pending aftermath: %v (created %v)", err, created)
	}
	f.svc.CompanyAftermaths = func(context.Context, int64, int64) ([]AftermathRef, error) { return nil, nil }
	if _, err := f.svc.CreateMatch(f.ctx, f.player(), 5, sides); err != nil || !created || !inited {
		t.Errorf("created %v, live init %v, err %v (a failing live init does not fail the match)", created, inited, err)
	}
}

func TestCreateMatchPassesTheBusyCompanyError(t *testing.T) {
	f := newMatchesFixture(t)
	busy := &CompanyBusyError{Company: "Rangers", OpenMatchRef: OpenMatchRef{MatchID: 2, Number: 2}}
	f.matches.EXPECT().Create(gomock.Any(), int64(5), gomock.Any(), gomock.Any()).Return(Match{}, busy).AnyTimes()
	_, err := f.svc.CreateMatch(f.ctx, f.player(), 5, []NewSide{{Name: "A", Companies: []int64{1}}, {Name: "B", Companies: []int64{2}}})
	var got *CompanyBusyError
	if !errors.Is(err, ErrRefused) || !errors.As(err, &got) || got.Number != 2 {
		t.Errorf("%v", err)
	}
}

// A finished campaign is frozen: no company use case writes, whoever asks.
func TestFinishedCampaignRefusesEveryCompanyWrite(t *testing.T) {
	rules := staticRules{shippedRules(t)}
	ctrl := gomock.NewController(t)
	campaigns := NewMockcampaignRepository(ctrl)
	companies := NewMockcompanyRepository(ctrl)
	campaigns.EXPECT().Hashes(gomock.Any(), int64(5)).Return(RulesetHashes{}, CampaignSettings{}, nil).AnyTimes()
	campaigns.EXPECT().Get(gomock.Any(), int64(9), int64(5)).Return(CampaignDetail{CampaignSummary: CampaignSummary{ID: 5, Status: CampaignFinished, Companies: []CampaignCompany{{ID: 1}}}}, nil).AnyTimes()
	// Update and Read are never reached: every use case stops at the campaign
	ctx, user := context.Background(), User{ID: 9}
	arm := NewArmouryService(campaigns, companies, rules)
	lvl := NewLevelService(campaigns, companies, rules, &FixedDice{Rolls: []int{3}})
	rec := NewRecruitService(campaigns, companies, rules, &FixedDice{Rolls: []int{3}})
	co := NewCompanyService(campaigns, companies, rules)
	_, rollLevel := lvl.RollLevelUp(ctx, user, 5, 1, 1, nil, false)
	_, chooseLevel := lvl.ChooseLevelUp(ctx, user, 5, 1, 1, []string{"x"})
	_, rerollLevel := lvl.RerollLevelUp(ctx, user, 5, 1, 1, 0)
	_, special := rec.RecruitSpecial(ctx, user, 5, 1, "Anybody")
	_, treat := NewTreatmentService(campaigns, companies, rules, &FixedDice{}).TreatInjury(ctx, user, 5, 1, 7, Treatment{Method: "warrior"})
	for name, err := range map[string]error{
		"buy": arm.BuyEquipment(ctx, user, 5, 1, 6, "Shield", ""), "drop": arm.DropEquipment(ctx, user, 5, 1, 6, "Bow"),
		"creature": arm.BuyCreature(ctx, user, 5, 1, 2, "Baggage Pony", ""), "wanderer": arm.HireWanderer(ctx, user, 5, 1, "Anybody", ""),
		"dismiss": arm.DismissWanderer(ctx, user, 5, 1, 6), "carry": arm.Carry(ctx, user, 5, 1, 6, CarryAction{Kind: "store", Item: "Bow"}),
		"recruit": rec.RollRecruit(ctx, user, 5, 1, 0, false), "special": special, "treat": treat,
		"level": rollLevel, "choose": chooseLevel, "reroll": rerollLevel, "path": lvl.ChoosePath(ctx, user, 5, 1, 1, "Path of the Warlord", ""),
		"rename company": co.RenameCompany(ctx, user, 5, 1, "New name"), "rename unit": co.RenameUnit(ctx, user, 5, 1, 1, "Bold"),
		"formation": co.SaveFormation(ctx, user, 5, 1, []int64{1}), "cemetery order": NewCemeteryService(campaigns, companies, rules).SaveCemeteryOrder(ctx, user, 5, 1, nil),
		"perk": (&AftermathService{companyDeps: companyDeps{campaigns: campaigns}}).UsePerk(ctx, user, 5, 1, 1),
	} {
		if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "The campaign is finished.") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Rendezvous moves the first reinforcement roll by one, once; the perk is used in the same transaction as the roll.
func TestRendezvousMovesTheFirstReinforcementRoll(t *testing.T) {
	rules := staticRules{shippedRules(t)}
	for _, tc := range []struct {
		name         string
		perks        []Perk
		roll, adjust int
		err          error
		wantRoll     int
	}{
		{"plus one", []Perk{{ID: 7, Effect: PerkRendezvous, When: "firstReinforcementRollThisAftermath"}}, 5, 1, nil, 6},
		{"minus one", []Perk{{ID: 7, Effect: PerkRendezvous}}, 5, -1, nil, 4},
		{"no bonus asked", []Perk{{ID: 7, Effect: PerkRendezvous}}, 5, 0, nil, 5},
		{"no perk", nil, 5, 1, ErrRefused, 0},
		{"another perk", []Perk{{ID: 8, Effect: PerkVantagePoint}}, 5, 1, ErrRefused, 0},
		{"a used perk", []Perk{{ID: 9, Effect: PerkRendezvous, UsedOn: new(time.Time)}}, 5, 1, ErrRefused, 0},
		{"past six", []Perk{{ID: 7, Effect: PerkRendezvous}}, 6, 1, ErrInvalidRoll, 0},
		{"below one", []Perk{{ID: 7, Effect: PerkRendezvous}}, 1, -1, ErrInvalidRoll, 0},
		{"two", []Perk{{ID: 7, Effect: PerkRendezvous}}, 3, 2, ErrInvalidRoll, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCompanyFixture(t)
			f.update()
			f.settings = CampaignSettings{ManualRolls: true}
			f.state.Perks, f.state.Influence = tc.perks, 12
			err := NewRecruitService(f.campaigns, f.companies, rules, &FixedDice{}).RollRecruitWithPerk(f.ctx, f.user, 5, 1, tc.roll, false, tc.adjust)
			if tc.err != nil {
				if !errors.Is(err, tc.err) || f.state.Recruit.Roll != nil || len(f.state.PerksUsed) != 0 {
					t.Errorf("got %v, recruit %+v, used %v", err, f.state.Recruit.Roll, f.state.PerksUsed)
				}
				return
			}
			step := f.state.Recruit.Roll
			if err != nil || step == nil || step.Roll != tc.wantRoll {
				t.Fatalf("got %v %+v, want a %d", err, step, tc.wantRoll)
			}
			if tc.adjust != 0 {
				rr, _ := NewRecruitService(f.campaigns, f.companies, rules, &FixedDice{}).RecruitState(f.ctx, f.user, 5, 1)
				if rr == nil || rr.PerkAdjust != tc.adjust || rr.PerkRaw != tc.roll || rr.Roll != tc.wantRoll {
					t.Errorf("the recruit window badges the moved die: %+v", rr)
				}
			}
			if (tc.adjust != 0) != (len(f.state.PerksUsed) == 1) || (tc.adjust != 0 && (step.PerkRaw != tc.roll || step.PerkAdjust != tc.adjust)) {
				t.Errorf("the perk is used with the roll: %v %+v", f.state.PerksUsed, step)
			}
		})
	}
}
