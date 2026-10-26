package core

import "testing"

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
