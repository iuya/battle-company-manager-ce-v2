package core

import (
	"errors"
	"testing"
)

func (f *companyFixture) cemetery(t *testing.T) *CemeteryService {
	t.Helper()
	f.update()
	return NewCemeteryService(f.campaigns, f.companies, staticRules{shippedRules(t)})
}

func TestBuryMovesTheUnitToTheCemeteryAsItWas(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.cemetery(t)
	f.state.Influence = 20
	f.state.Order = []int64{3, 2, 1}
	arm := NewArmouryService(f.campaigns, f.companies, staticRules{shippedRules(t)})
	if err := arm.BuyCreature(f.ctx, f.user, 5, 1, 2, "Baggage Pony", ""); err != nil {
		t.Fatal(err)
	}
	if err := arm.Carry(f.ctx, f.user, 5, 1, 2, CarryAction{Kind: "store", Item: "Spear"}); err != nil {
		t.Fatal(err)
	}
	before := f.detail(t)
	hador := f.unit(t, 2)
	if err := s.Bury(f.ctx, 5, 1, 2, "Dead", "Match 3"); err != nil {
		t.Fatal(err)
	}
	after := f.detail(t)
	if len(after.Roster) != len(before.Roster)-1 || len(after.Cemetery) != 1 {
		t.Fatalf("roster %d -> %d, cemetery %d", len(before.Roster), len(after.Roster), len(after.Cemetery))
	}
	fallen := after.Cemetery[0]
	if fallen.Unit.ID != 2 || fallen.Fate != "Dead" || fallen.Match != "Match 3" || fallen.Unit.Points != hador.Points || fallen.Unit.Level != hador.Level || fallen.Unit.Path == nil || len(fallen.Unit.Equipment) != len(hador.Equipment) {
		t.Errorf("the snapshot keeps what the unit was: %+v", fallen.Unit)
	}
	for _, u := range after.Roster {
		if u.ID == 2 {
			t.Error("the unit left the roster")
		}
	}
	if len(after.Creatures) != 0 || after.Points >= before.Points {
		t.Errorf("the creature is freed and the points go: %d creatures, %d -> %d points", len(after.Creatures), before.Points, after.Points)
	}
	if len(f.state.Pack) != 1 || f.state.Pack[0] != "Spear" {
		t.Errorf("what it left in the pack stays: %v", f.state.Pack)
	}
	for _, id := range f.state.Order {
		if id == 2 {
			t.Error("the saved order forgets the unit")
		}
	}
	if err := s.Bury(f.ctx, 5, 1, 2, "Dead", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a unit is buried once: %v", err)
	}
	if err := s.Bury(f.ctx, 5, 1, 999, "Dead", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown unit: %v", err)
	}
}

func TestTheFallenCountTowardsNoLimit(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.cemetery(t)
	f.companyOf("Moria", 30, warriors("Moria Goblin Warrior", 3))
	f.state.Units = append(f.state.Units, UnitState{ID: 50, Base: "Cave Troll", Role: RoleWarrior})
	env := newRecruitEnv(shippedRules(t), f.settings, f.state)
	if why, kind := env.block(Reinforcement{Type: "unit", Unit: "Cave Troll", Rare: 1}); why == "" || kind != blockRare {
		t.Fatalf("the company has its Cave Troll: %q %q", why, kind)
	}
	if err := s.Bury(f.ctx, 5, 1, 50, "Dead", "Match 1"); err != nil {
		t.Fatal(err)
	}
	env = newRecruitEnv(shippedRules(t), f.settings, f.state)
	if why, _ := env.block(Reinforcement{Type: "unit", Unit: "Cave Troll", Rare: 1}); why != "" {
		t.Errorf("a fallen Cave Troll does not count against the rare limit: %s", why)
	}
	d := f.detail(t)
	if d.ModelCount() != 3 {
		t.Errorf("nor against the model maximum: %d", d.ModelCount())
	}
}

func TestSaveCemeteryOrder(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.cemetery(t)
	for _, id := range []int64{4, 5, 6} {
		if err := s.Bury(f.ctx, 5, 1, id, "Dead", "Match 1"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SaveCemeteryOrder(f.ctx, f.user, 5, 1, []int64{6, 4, 6}); err != nil {
		t.Fatal(err)
	}
	if d := f.detail(t); d.Cemetery[0].Unit.ID != 6 || d.Cemetery[1].Unit.ID != 4 || d.Cemetery[2].Unit.ID != 5 {
		t.Errorf("the saved order leads, the rest follows: %d %d %d", d.Cemetery[0].Unit.ID, d.Cemetery[1].Unit.ID, d.Cemetery[2].Unit.ID)
	}
	if err := s.SaveCemeteryOrder(f.ctx, f.user, 5, 1, []int64{6, 1}); !errors.Is(err, ErrRefused) {
		t.Errorf("a unit of the roster cannot be put among the fallen: %v", err)
	}
	companies := NewCompanyService(f.campaigns, f.companies, staticRules{shippedRules(t)})
	if err := companies.SaveFormation(f.ctx, f.user, 5, 1, []int64{4}); !errors.Is(err, ErrRefused) {
		t.Errorf("nor a fallen one among the units of the company: %v", err)
	}
	if err := s.SaveCemeteryOrder(f.ctx, User{ID: 8}, 5, 1, []int64{6}); !errors.Is(err, ErrNotFound) {
		t.Errorf("non-owner: %v", err)
	}
	if len(f.state.Graveyard) != 2 {
		t.Errorf("refusals change nothing: %v", f.state.Graveyard)
	}
}

func TestAHeroRetiresOnItsSecondArmOrLegWound(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.cemetery(t)
	rs := shippedRules(t)
	def, _ := rs.BC.Injuries().Def("armWound")
	if def.RetireAt != 2 {
		t.Fatalf("the file retires a hero at its second arm wound: %d", def.RetireAt)
	}
	f.state.Units[1].Injuries = nil
	retired, err := s.Injure(f.ctx, 5, 1, 2, "armWound", "Match 4")
	if err != nil || retired || f.state.Units[1].Injuries["armWound"] != 1 {
		t.Fatalf("the first wound stays: %v %v %v", retired, err, f.state.Units[1].Injuries)
	}
	retired, err = s.Injure(f.ctx, 5, 1, 2, "armWound", "Match 5")
	if err != nil || !retired {
		t.Fatalf("the second one retires the hero: %v %v", retired, err)
	}
	d := f.detail(t)
	if len(d.Cemetery) != 1 || d.Cemetery[0].Fate != "Retired" || d.Cemetery[0].Match != "Match 5" || d.Cemetery[0].Unit.ID != 2 {
		t.Errorf("the hero rests among the fallen: %+v", d.Cemetery)
	}
	if _, err := s.Injure(f.ctx, 5, 1, 6, "armWound", "x"); !errors.Is(err, ErrRefused) {
		t.Errorf("warriors keep no injuries: %v", err)
	}
	if _, err := s.Injure(f.ctx, 5, 1, 1, "paperCut", "x"); !errors.Is(err, ErrRefused) {
		t.Errorf("unknown injury: %v", err)
	}
	if _, err := s.Injure(f.ctx, 5, 1, 999, "armWound", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown unit: %v", err)
	}
	if retired, err := s.Injure(f.ctx, 5, 1, 1, "brokenHonour", "Match 5"); err != nil || retired {
		t.Errorf("broken honour does not retire a hero: %v %v", retired, err)
	}
}
