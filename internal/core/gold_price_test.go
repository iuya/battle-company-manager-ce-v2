package core

import (
	"strings"
	"testing"
)

// Every unit, warrior or hero, pays in Gold the armoury's first value (the warrior rating); the hero value is only what an item adds
// to a hero's rating. The exception is wargear that is one of the unit's own profile's options on its own: it costs that option's
// points ("Dwarf Rangers pay 1pt for a Longbow-equivalent"), so a Dwarf Ranger pays 1 for a Dwarf Longbow and a Dwarf Warrior or a
// hero of another profile pays the armoury's 2.
func TestGoldPriceIsTheFirstValueOrTheOptionsPoints(t *testing.T) {
	rs := shippedRules(t)
	for _, tc := range []struct {
		company, unit, item string
		role                UnitRole
		want                int
	}{
		{"Durin's Folk", "Dwarf Ranger", "Dwarf Longbow", RoleWarrior, 1},
		{"Durin's Folk", "Dwarf Ranger", "Throwing weapon", RoleWarrior, 1},
		{"Durin's Folk", "Dwarf Warrior", "Dwarf Bow", RoleWarrior, 1},
		{"Durin's Folk", "Dwarf Warrior", "Shield", RoleWarrior, 1},
		{"Durin's Folk", "Dwarf Warrior", "Dwarf Longbow", RoleWarrior, 2}, // not its option: the armoury's first value
		{"Durin's Folk", "Dwarf Ranger", "Dwarf Longbow", RoleSergeant, 1}, // a hero of the profile pays the option's points too
		{"Durin's Folk", "Dwarf Warrior", "Dwarf Longbow", RoleSergeant, 2},
		{"Arnor", "Ranger of Arnor", "Torching Brand", RoleLeader, 1}, // equipment: the first value (the hero column is 5)
		{"Arnor", "Ranger of Arnor", "Torching Brand", RoleWarrior, 1},
		{"Arnor", "Ranger of Arnor", "Spear", RoleLeader, 1}, // its option's points, as for a warrior
		{"Arnor", "Ranger of Arnor", "Spear", RoleWarrior, 1},
		{"Arnor", "Warrior of Arnor", "Bow", RoleSergeant, 1}, // a hero never pays the hero column
	} {
		c := rs.BC.companies[tc.company]
		sh := shop{rs: rs, bcC: c, d: CompanyDetail{Influence: 20, Gold: 20}}
		o, ok := rs.BC.Offer(c, tc.item)
		if !ok {
			t.Fatalf("no %s in %s", tc.item, tc.company)
		}
		if got := sh.goldPrice(CompanyUnit{Type: tc.unit, Role: tc.role}, o); got != tc.want {
			t.Errorf("%s %s buying %s: %d gold, want %d (armoury %d / %d)", tc.role, tc.unit, tc.item, got, tc.want, o.Gold.Warrior, o.Gold.Hero)
		}
	}
}

// An armour upgrade costs the first value for every unit.
func TestArmourUpgradeGoldPriceIsTheFirstValue(t *testing.T) {
	rs := shippedRules(t)
	c := rs.BC.companies["Arnor"]
	sh := shop{rs: rs, bcC: c, d: CompanyDetail{Influence: 20, Gold: 20}}
	n := 0
	for _, o := range sh.offers() {
		if o.ArmourStage == 0 {
			continue
		}
		n++
		for _, role := range []UnitRole{RoleWarrior, RoleLeader} {
			if got := sh.goldPrice(CompanyUnit{Type: "Ranger of Arnor", Role: role}, o); got != o.Gold.Warrior {
				t.Errorf("%s buying %s: %d gold, want the first value %d", role, o.Name, got, o.Gold.Warrior)
			}
		}
	}
	if n == 0 {
		t.Fatal("no armour upgrade offered")
	}
}

// The refusal reason quotes the price the buy charges: a company with exactly the Gold can buy, one Gold less is refused naming that price.
func TestGoldAffordabilityFollowsThePrice(t *testing.T) {
	rs := shippedRules(t)
	c := rs.BC.companies["Arnor"]
	u := CompanyUnit{Type: "Ranger of Arnor", Role: RoleLeader, BaseSlots: SlotCounts{Large: 2, Small: 2}}
	o, ok := rs.BC.Offer(c, "Torching Brand")
	if !ok || o.Cost == 0 {
		t.Fatalf("Torching Brand: %v, cost %d", ok, o.Cost)
	}
	price := (shop{rs: rs, bcC: c}).goldPrice(u, o)
	for _, tc := range []struct {
		gold   int
		refuse bool
	}{{price, false}, {price - 1, true}} {
		sh := shop{rs: rs, bcC: c, d: CompanyDetail{Influence: o.Cost - 1, Gold: tc.gold}}
		reason, blocker := sh.whyNot(u, o)
		if tc.refuse {
			want := costReason(sh.d, o.Cost, price)
			if blocker != BlockedByInfluence || reason != want || !strings.Contains(reason, "gold") {
				t.Errorf("with %d gold: %q (%s), want %q", tc.gold, reason, blocker, want)
			}
		} else if reason != "" && blocker == BlockedByInfluence {
			t.Errorf("with exactly %d gold: refused (%s)", tc.gold, reason)
		}
	}
}
