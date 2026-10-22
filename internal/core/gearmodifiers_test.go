package core

import (
	"slices"
	"strings"
	"testing"
)

// An item is a base item plus modifiers (data.gearAnnotations.modifiers): the tooltip explains the modifier, the identity stays exact.
func TestItemModifiersExplainedInTheTooltip(t *testing.T) {
	rs := shippedRules(t)
	spear, ok := rs.BC.Annotation("Elven spear")
	if !ok || spear.Base != "Spear" || !slices.Equal(spear.Modifiers, []string{"elven"}) || spear.PricedAs != "Spear" {
		t.Fatalf("Elven spear = %+v", spear)
	}
	for _, want := range []string{"Elven weapon:", "3+", "1-4", "neither side gains the benefit"} {
		if !strings.Contains(spear.Definition, want) {
			t.Errorf("the Elven spear tooltip lacks %q: %s", want, spear.Definition)
		}
	}
	bow, _ := rs.BC.Annotation("Bow with poisoned arrows")
	if bow.Base != "Bow" || bow.VariantOf != "Bow" || !strings.Contains(bow.Definition, "Poisoned:") {
		t.Errorf("poisoned bow = %+v", bow)
	}
	if plain, _ := rs.BC.Annotation("Spear"); plain.Base != "" || strings.Contains(plain.Definition, "Elven") {
		t.Errorf("a plain Spear has no modifier: %+v", plain)
	}
}

// Unique wargear belongs to its company alone, at the price of that company's page (not the price of its base).
func TestUniqueElvenSpearsBelongToTheirCompany(t *testing.T) {
	rs := shippedRules(t)
	for item, tc := range map[string]struct {
		company string
		cost    int
		rating  Rating
	}{
		"Elven-made Glaive": {"Halls of Thranduil", 1, Rating{2, 5}},
		"Wood Elf Spear":    {"Lothlorien", 1, Rating{1, 5}},
	} {
		for name, c := range rs.BC.companies {
			o, offered := rs.BC.Offer(c, item)
			if offered != (name == tc.company) {
				t.Errorf("%s: %s offered = %v", name, item, offered)
			}
			if offered && (o.Cost != tc.cost || o.Rating != tc.rating || !o.Exclusive) {
				t.Errorf("%s: %s costs %d rated %v, want %d %v", name, item, o.Cost, o.Rating, tc.cost, tc.rating)
			}
		}
	}
}

// The Mattock is its own item: only a profile whose option names it can buy it, and no other item is "also called" Mattock.
func TestMattockIsOnlyForTheProfilesThatHaveIt(t *testing.T) {
	rs := shippedRules(t)
	for name, c := range rs.BC.companies {
		sh := shop{rs: rs, bcC: c}
		for _, o := range rs.BC.Armoury(c) {
			if !strings.EqualFold(o.Name, "Mattock") && slices.ContainsFunc(o.AlsoCalled, func(n string) bool { return strings.EqualFold(n, "Mattock") }) {
				t.Errorf("%s: %s is also called Mattock", name, o.Name)
			}
		}
		for _, ty := range []string{"Mordor Uruk-Hai", "Uruk-hai Warrior", "Dwarf Warrior"} {
			if slices.Contains(offeredWargear(sh, CompanyUnit{Type: ty, Role: RoleWarrior}), "Mattock") {
				t.Errorf("%s: %s is offered a Mattock", name, ty)
			}
		}
	}
	co, _ := rs.BC.Company("Iron Hills")
	if !slices.Contains(offeredWargear(shop{rs: rs, bcC: co}, CompanyUnit{Type: "Iron Hills Warrior", Role: RoleWarrior}), "Mattock") {
		t.Error("an Iron Hills Warrior can buy a Mattock")
	}
}

// "Two-handed axe" (Wild Man of Dunland, Dwarf Warrior) is the Two-handed weapon option.
func TestTwoHandedAxeIsTheTwoHandedWeapon(t *testing.T) {
	rs := shippedRules(t)
	co, _ := rs.BC.Company("Burning of the Westfold")
	got := offeredWargear(shop{rs: rs, bcC: co}, CompanyUnit{Type: "Wild Man of Dunland", Role: RoleWarrior})
	if !slices.Contains(got, "Two-handed weapon") {
		t.Errorf("a Wild Man of Dunland can buy the two-handed axe: %v", got)
	}
}
