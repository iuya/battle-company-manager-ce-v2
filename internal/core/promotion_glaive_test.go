package core

import (
	"slices"
	"strings"
	"testing"
)

func gearNames(u CompanyUnit) []string {
	var out []string
	for _, e := range u.Equipment {
		out = append(out, e.Name)
	}
	return out
}

func hasGlaive(names []string) bool {
	return slices.ContainsFunc(names, func(n string) bool { return SameItem(n, nil, "Elven-made Glaive", []string{"Elven Glaive"}) })
}

// Owner rule (2026-10-26): a Mirkwood Elf Warrior with the Elven-made glaive promoted to the Mirkwood Elf Knight loses it, because
// a warrior may only carry what its profile lists and the Knight's profile has no glaive. The dropped gear is simply gone.
func TestAPromotedWarriorLosesTheGlaiveTheKnightDoesNotHave(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	f.companyOf("Halls of Thranduil", 5, []UnitState{{ID: 90, Base: "Mirkwood Elf Warrior", Role: RoleWarrior, Gear: []string{"Elven-made Glaive"}, XP: 5}})
	if before := f.unit(t, 90); !hasGlaive(gearNames(before)) {
		t.Fatalf("the warrior carries the glaive: %v", gearNames(before))
	}
	roll, err := s.RollLevelUp(f.ctx, f.user, 5, 90, []int{4}, false)
	if err != nil {
		t.Fatal(err)
	}
	key := ""
	for _, o := range roll.Offers {
		for _, ch := range o.Choices {
			if strings.HasPrefix(ch.Key, "promo:") && ch.Label == "Mirkwood Elf Knight" {
				key = ch.Key
			}
		}
	}
	if key == "" {
		t.Fatalf("the Knight promotion is not offered: %+v", roll.Offers)
	}
	if _, err := s.ChooseLevelUp(f.ctx, f.user, 5, 90, []string{key}); err != nil {
		t.Fatal(err)
	}
	u := f.unit(t, 90)
	if u.Type != "Mirkwood Elf Knight" || u.IsHero() {
		t.Fatalf("a warrior Knight: %q hero=%v", u.Type, u.IsHero())
	}
	if hasGlaive(gearNames(u)) {
		t.Errorf("the Knight lost the glaive, it carries %v", gearNames(u))
	}
	if !slices.Contains(gearNames(u), "Horse") {
		t.Errorf("the Knight keeps its own wargear: %v", gearNames(u))
	}
	t.Logf("stored gear after the promotion: Gear=%v Bought=%v Dropped=%v Progress.Gear=%v", f.state.Units[0].Gear, f.state.Units[0].Bought, f.state.Units[0].Dropped, f.state.Units[0].Progress.Gear)
}

func glaiveItem(t *testing.T, u UnitState) (ArmouryItem, bool) {
	t.Helper()
	st := CompanyState{Company: CampaignCompany{ID: 1, Name: "T", Faction: "Halls of Thranduil"}, Influence: 20, Gold: 50, Units: []UnitState{u}}
	items, err := newShop(shippedRules(t), CampaignSettings{}, st).items(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Size == "" && GearMatchesOption(it.GearOffer, "Elven Glaive") {
			return it, true
		}
	}
	return ArmouryItem{}, false
}

// The same Knight as a hero buys the glaive from the armoury (a hero may buy the options of any profile of its company); a warrior
// Knight, whose profile has no options, may not.
func TestOnlyAHeroKnightMayBuyTheElvenGlaive(t *testing.T) {
	cases := []struct {
		name string
		unit UnitState
		want bool
	}{
		{"hero knight", UnitState{ID: 1, Base: "Mirkwood Elf Knight", Role: RoleHero, Bonus: HeroBonus(nil), Progress: &Progress{Hero: true, Path: "Warrior"}}, true},
		{"warrior knight", UnitState{ID: 1, Base: "Mirkwood Elf Knight", Role: RoleWarrior}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			it, found := glaiveItem(t, c.unit)
			if c.want {
				if !found || !it.Buyable() || it.Cost <= 0 {
					t.Fatalf("the glaive is a legal purchase with a price: found=%v %+v", found, it)
				}
				t.Logf("price: influence %d gold %d", it.Cost, it.GoldPrice)
			} else if found && it.Buyable() {
				t.Fatalf("a warrior Knight cannot buy it: %+v", it)
			}
		})
	}
}

func TestAHeroKnightBuysTheElvenGlaive(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.armoury(t)
	f.companyOf("Halls of Thranduil", 20, []UnitState{{ID: 7, Base: "Mirkwood Elf Knight", Role: RoleHero, Bonus: HeroBonus(nil), Progress: &Progress{Hero: true, Path: "Warrior"}}})
	it, ok := glaiveItem(t, f.state.Units[0])
	if !ok {
		t.Fatal("not offered")
	}
	if err := s.BuyEquipment(f.ctx, f.user, 5, 7, it.Name, ""); err != nil {
		t.Fatal(err)
	}
	if f.state.Influence != 20-it.Cost || !hasGlaive(f.state.Units[0].Bought) {
		t.Errorf("influence %d, bought %v", f.state.Influence, f.state.Units[0].Bought)
	}
	if !hasGlaive(gearNames(f.unit(t, 7))) {
		t.Errorf("the hero carries it: %v", gearNames(f.unit(t, 7)))
	}
	if err := s.BuyEquipment(f.ctx, f.user, 5, 7, it.Name, ""); err == nil {
		t.Error("bought twice")
	}
}

// The promotion's own required wargear is kept where the new profile lists it: the Elf bow of a Mirkwood Ranger.
func TestAPromotedWarriorKeepsWhatTheNewProfileLists(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	f.companyOf("Halls of Thranduil", 5, []UnitState{{ID: 90, Base: "Mirkwood Elf Warrior", Role: RoleWarrior, Gear: []string{"Elf bow"}, XP: 5}})
	roll, err := s.RollLevelUp(f.ctx, f.user, 5, 90, []int{4}, false)
	if err != nil {
		t.Fatal(err)
	}
	key := ""
	for _, o := range roll.Offers {
		for _, ch := range o.Choices {
			if strings.HasPrefix(ch.Key, "promo:") && ch.Label == "Mirkwood Ranger" {
				key = ch.Key
			}
		}
	}
	if key == "" {
		t.Fatalf("no Ranger promotion: %+v", roll.Offers)
	}
	if _, err := s.ChooseLevelUp(f.ctx, f.user, 5, 90, []string{key}); err != nil {
		t.Fatal(err)
	}
	if got := gearNames(f.unit(t, 90)); !slices.ContainsFunc(got, func(n string) bool { return SameItem(n, nil, "Elf bow", nil) }) {
		t.Errorf("the Ranger keeps the bow: %v", got)
	}
}
