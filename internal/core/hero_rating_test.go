package core

import (
	"slices"
	"testing"
)

// BC "Calculate Rating": a hero pays for wargear and equipment that is neither in its base profile nor in its profile's Options; below
// Attacks + Wounds 3 at the warrior rating, from 3 on at the hero rating (all such gear recalculated at that moment). Wargear of the
// base profile or of its Options "makes no adjustment, it is as it is in their book": the profile's wargear is free and an option
// costs its MESBG points, for a hero as for a warrior.
func TestHeroRatingOfGearOutsideTheProfileAndItsOptions(t *testing.T) {
	rs := shippedRules(t)
	rating := func(name string) Rating {
		o, ok := rs.BC.Offer(rs.mustCompany(t, "Rohan"), name)
		if !ok {
			t.Fatalf("no %s in the armoury", name)
		}
		return o.Rating
	}
	points := func(role UnitRole, bonus map[string]int, bought ...string) int {
		s := UnitState{ID: 1, Base: "Warrior of Rohan", Role: role, Bonus: bonus, Bought: bought}
		if role == RoleWarrior {
			s.Bonus = nil
		}
		return buildUnit(rs, rs.mustCompany(t, "Rohan"), s, CampaignSettings{}).Points
	}
	vanilla, _ := rs.Vanilla.Unit("Warrior of Rohan")
	bowOption := 0
	for _, o := range vanilla.Options {
		if o.Name == "Bow" {
			bowOption = o.Points
		}
	}
	if bowOption == 0 {
		t.Fatal("the Warrior of Rohan has a Bow option")
	}
	grown := HeroBonus(map[string]int{"attack": 1}) // Attacks 2 + Wounds 1 = 3
	young := HeroBonus(nil)                         // Attacks 1 + Wounds 1 = 2
	for _, tc := range []struct {
		name  string
		bonus map[string]int
		item  string
		want  func(Rating) int
	}{
		{"outside the options, below 3", young, "Horse", func(r Rating) int { return r.Warrior }},
		{"outside the options, from 3", grown, "Horse", func(r Rating) int { return r.Hero }},
		{"in the options: as in the book, below 3", young, "Bow", func(Rating) int { return bowOption }},
		{"in the options: as in the book, from 3", grown, "Bow", func(Rating) int { return bowOption }},
		{"equipment is never in the options, below 3", young, "Company Standard", func(r Rating) int { return r.Warrior }},
		{"equipment, from 3", grown, "Company Standard", func(r Rating) int { return r.Hero }},
	} {
		without, with := points(RoleHero, tc.bonus), points(RoleHero, tc.bonus, tc.item)
		if got := with - without; got != tc.want(rating(tc.item)) {
			t.Errorf("%s: %s adds %d, want %d", tc.name, tc.item, got, tc.want(rating(tc.item)))
		}
	}
	// the Rohan warrior's Armour upgrade to Heavy Armour: +1 below 3, +5 from 3 (the armour stage rating)
	for name, tc := range map[string]struct {
		bonus map[string]int
		want  int
	}{"below 3": {young, 1}, "from 3": {grown, 5}} {
		without, with := points(RoleHero, tc.bonus), points(RoleHero, tc.bonus, "Heavy Armour")
		if with-without != tc.want {
			t.Errorf("heavy armour %s adds %d, want %d", name, with-without, tc.want)
		}
	}
	// a warrior uses the points of its option and its equipment's warrior rating
	v, _ := rs.Vanilla.Unit("Warrior of Rohan")
	want := 0
	for _, o := range v.Options {
		if o.Name == "Bow" {
			want = o.Points
		}
	}
	if got := points(RoleWarrior, nil, "Bow") - points(RoleWarrior, nil); want == 0 || got != want {
		t.Errorf("a warrior's Bow is its option's points: %d, want %d", got, want)
	}
}

// A hero's armour that is an option of its profile makes no adjustment (the Ranger of the North's Armour option); only the stages
// above the profile and its options are charged.
func TestHeroArmourInsideTheOptionsIsFree(t *testing.T) {
	rs := shippedRules(t)
	bcCo := rs.mustCompany(t, "The Shire")
	build := func(bonus map[string]int, drop []string, bought ...string) CompanyUnit {
		return buildUnit(rs, bcCo, UnitState{ID: 1, Base: "Ranger of the North", Role: RoleHero, Bonus: bonus, Dropped: drop, Bought: bought}, CampaignSettings{})
	}
	base := build(HeroBonus(nil), nil)
	if len(armourNames(base)) == 0 {
		t.Skip("the Ranger of the North has no armour here")
	}
	// Heavy Armour is not an option of the Ranger: one stage above its Armour
	heavy := build(HeroBonus(nil), []string{"Armour"}, "Heavy Armour")
	if d := heavy.Points - base.Points; d < 1 {
		t.Errorf("heavy armour outside the options is charged: %d", d)
	}
}

func (rs *RuleSet) mustCompany(t *testing.T, name string) BCCompany {
	t.Helper()
	c, ok := rs.BC.Company(name)
	if !ok {
		t.Fatalf("no company %s", name)
	}
	return c
}

// BC "Calculate Rating" (S1417-1418): a hero gains 5 for every minor special rule, together at most 10 (data: perMinorSpecialRule,
// maxMinorSpecialRuleRating). A minor rule with a target (Poisoned Attacks (weapon), Combat Synergy - hero) is a minor rule too; a rule an
// item grants (Envenom Weapon's Poisoned Attacks) is not: it is the item's, rated as equipment.
func TestHeroRatingOfMinorSpecialRules(t *testing.T) {
	rs := shippedRules(t)
	co := rs.mustCompany(t, "Rohan")
	rating := rs.BC.rating
	if rating.PerMinorSpecialRule != 5 || rating.MaxMinorSpecialRule != 10 {
		t.Fatalf("the data says %d per minor rule, at most %d", rating.PerMinorSpecialRule, rating.MaxMinorSpecialRule)
	}
	hero := func(id int64, p *Progress, bought ...string) UnitState {
		return UnitState{ID: id, Base: "Warrior of Rohan", Role: RoleHero, Bonus: HeroBonus(nil), Path: "Path of the Soldier", Gear: []string{"Spear"}, Bought: bought, Progress: p}
	}
	points := func(p *Progress, bought ...string) int {
		d := BuildDetail(rs, CampaignSettings{}, CompanyState{Company: CampaignCompany{Faction: co.Name}, Units: []UnitState{hero(1, p, bought...), hero(2, nil)}})
		for _, u := range d.Roster {
			if u.ID == 1 {
				return u.Points
			}
		}
		t.Fatal("no unit 1")
		return 0
	}
	base := points(nil)
	for _, tc := range []struct {
		name  string
		minor []string
		want  int
	}{
		{"none", nil, 0},
		{"one untargeted", []string{"Resistant to Magic"}, 5},
		{"two", []string{"Resistant to Magic", "Fearless"}, 10},
		{"three are capped at 10", []string{"Resistant to Magic", "Fearless", "Stalwart"}, 10},
		{"a weapon target counts", []string{"Poisoned Attacks|weapon:Spear"}, 5},
		{"a weapon target and an untargeted rule", []string{"Poisoned Attacks|weapon:Spear", "Fearless"}, 10},
		{"a unit target counts", []string{"Combat Synergy|unit:2"}, 5},
		{"all kinds, capped", []string{"Poisoned Attacks|weapon:Spear", "Fearless", "Combat Synergy|unit:2"}, 10},
	} {
		if got := points(&Progress{Hero: true, Path: "Path of the Soldier", Minor: tc.minor}) - base; got != tc.want {
			t.Errorf("%s: adds %d, want %d", tc.name, got, tc.want)
		}
	}
	// a path rule rates 5 and is not capped with the minor rules
	if got := points(&Progress{Hero: true, Path: "Path of the Soldier", Rules: []string{rs.BC.pathRules["Path of the Soldier"][0].Name}, Minor: []string{"Fearless", "Stalwart", "Resistant to Magic"}}) - base; got != 15 {
		t.Errorf("a path rule and three minor rules add %d, want 5 + 10", got)
	}
	// Envenom Weapon: its Poisoned Attacks is the item's, not a minor rule: only the item's own rating is added
	env := "Envenom Weapon (Spear)"
	withD := BuildDetail(rs, CampaignSettings{}, CompanyState{Company: CampaignCompany{Faction: "Rohan"}, Units: []UnitState{hero(1, nil, env), hero(2, nil)}})
	with := withD.Roster[slices.IndexFunc(withD.Roster, func(u CompanyUnit) bool { return u.ID == 1 })]
	sum := func(u CompanyUnit) (n int) {
		for _, e := range u.Equipment {
			n += e.Rating
		}
		return n
	}
	baseD := BuildDetail(rs, CampaignSettings{}, CompanyState{Company: CampaignCompany{Faction: "Rohan"}, Units: []UnitState{hero(1, nil), hero(2, nil)}})
	itemRating := sum(with) - sum(baseD.Roster[slices.IndexFunc(baseD.Roster, func(u CompanyUnit) bool { return u.ID == 1 })])
	if with.Points-base != itemRating {
		t.Errorf("Envenom Weapon adds %d, its own rating is %d: the rule it grants must not count as a minor rule", with.Points-base, itemRating)
	}
}
