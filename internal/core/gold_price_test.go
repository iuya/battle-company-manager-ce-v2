package core

import "testing"

// A warrior is worth its MESBG points, so it pays in Gold what its profile's option costs, not the armoury's generic price
// ("Dwarf Rangers pay 1pt for a Longbow-equivalent"); a hero pays the armoury's hero price.
func TestWarriorGoldPriceIsTheOptionsPoints(t *testing.T) {
	rs := shippedRules(t)
	c := rs.BC.companies["Durin's Folk"]
	sh := shop{rs: rs, bcC: c, d: CompanyDetail{Influence: 20, Gold: 20}}
	for _, tc := range []struct {
		unit, item string
		role       UnitRole
		want       int
	}{
		{"Dwarf Ranger", "Dwarf Longbow", RoleWarrior, 1},
		{"Dwarf Ranger", "Throwing weapon", RoleWarrior, 1},
		{"Dwarf Warrior", "Dwarf Bow", RoleWarrior, 1},
		{"Dwarf Warrior", "Shield", RoleWarrior, 1},
		{"Dwarf Ranger", "Dwarf Longbow", RoleSergeant, 5},
	} {
		o, ok := rs.BC.Offer(c, tc.item)
		if !ok {
			t.Fatalf("no %s", tc.item)
		}
		if got := sh.goldPrice(CompanyUnit{Type: tc.unit, Role: tc.role}, o); got != tc.want {
			t.Errorf("%s %s buying %s: %d gold, want %d", tc.role, tc.unit, tc.item, got, tc.want)
		}
	}
}
