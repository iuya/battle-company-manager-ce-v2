package core

import (
	"slices"
	"strings"
	"testing"
)

// promoteTo rolls a promotion for the unit and takes the one called label.
func promoteTo(t *testing.T, f *companyFixture, s *LevelService, id int64, label string) {
	t.Helper()
	roll, err := s.RollLevelUp(f.ctx, f.user, 5, id, []int{4}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range roll.Offers {
		for _, ch := range o.Choices {
			if strings.HasPrefix(ch.Key, "promo:") && strings.HasPrefix(ch.Label, label) {
				if _, err := s.ChooseLevelUp(f.ctx, f.user, 5, id, []string{ch.Key}); err != nil {
					t.Fatal(err)
				}
				return
			}
		}
	}
	t.Fatalf("no promotion to %q offered: %+v", label, roll.Offers)
}

func hasItem(names []string, item string) bool {
	return slices.ContainsFunc(names, func(n string) bool { return SameItem(n, nil, item, nil) })
}

// Through the real LevelService: what a promoted warrior keeps and loses, by the identity of the items.
func TestPromotionWargearThroughTheLevelService(t *testing.T) {
	cases := []struct {
		name, faction, from, to string
		gear, bought            []string
		kept, lost              []string
	}{
		{"Corsair bow lost for the crossbow", "City of Umbar", "Corsair of Umbar", "Corsair Crossbowman", []string{"Bow"}, nil, []string{"Crossbow"}, []string{"Bow"}},
		{"bought gear is judged too", "City of Umbar", "Corsair of Umbar", "Corsair Crossbowman", []string{"Bow"}, []string{"Spear"}, []string{"Crossbow"}, []string{"Bow", "Spear"}},
		{"Elf bow stays for the Mirkwood Ranger", "Halls of Thranduil", "Mirkwood Elf Warrior", "Mirkwood Ranger", []string{"Elf bow"}, nil, []string{"Elf Bow"}, nil},
		{"the Wood Elf spear is its own item, not the Elven spear", "Lothlorien", "Lothlorien Warrior", "Galadhrim Warrior", []string{"Wood Elf spear"}, nil, nil, []string{"Wood Elf Spear"}},
		{"the Elven throwing weapons are not listed by the Galadhrim", "Lothlorien", "Lothlorien Warrior", "Galadhrim Warrior", []string{"Elven throwing weapons"}, nil, nil, []string{"Elven throwing weapons"}},
		{"a Haradrim bow is a plain Bow and stays", "Harad", "Haradrim Warrior", "Haradrim Raider", []string{"Bow"}, nil, []string{"Bow"}, nil},
		{"a Khandish Warrior who exchanged his hand-and-a-half weapon for a bow carries the bow", "Grand Army of the South", "Khandish Warrior", "Khandish Horseman", []string{"Exchange hand-and-a-half weapon for bow"}, nil, []string{"Bow", "Hand weapon"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newCompanyFixture(t)
			s := f.leveller(t)
			f.companyOf(c.faction, 5, []UnitState{{ID: 90, Base: c.from, Role: RoleWarrior, Gear: c.gear, Bought: c.bought, XP: 5}})
			promoteTo(t, f, s, 90, c.to)
			u := f.unit(t, 90)
			if u.Type != c.to || u.IsHero() {
				t.Fatalf("promoted warrior expected: %q hero=%v", u.Type, u.IsHero())
			}
			got := gearNames(u)
			for _, k := range c.kept {
				if !hasItem(got, k) {
					t.Errorf("%s should carry %q: %v", c.to, k, got)
				}
			}
			for _, l := range c.lost {
				if hasItem(got, l) {
					t.Errorf("%s should have lost %q: %v", c.to, l, got)
				}
			}
		})
	}
}

// Equipment (slot items) is not wargear: a banner or herbs bought by a warrior survive a promotion that loses his bow.
func TestPromotionKeepsBoughtEquipment(t *testing.T) {
	rs := shippedRules(t)
	c, _ := rs.BC.Company("City of Umbar")
	var equipment string
	for _, o := range rs.BC.Armoury(c) {
		if o.Size == "small" {
			equipment = o.Name
			break
		}
	}
	if equipment == "" {
		t.Fatal("no small equipment in the armoury")
	}
	f := newCompanyFixture(t)
	s := f.leveller(t)
	f.companyOf("City of Umbar", 5, []UnitState{{ID: 90, Base: "Corsair of Umbar", Role: RoleWarrior, Gear: []string{"Bow"}, Bought: []string{equipment}, XP: 5}})
	promoteTo(t, f, s, 90, "Corsair Crossbowman")
	if got := f.state.Units[0].Bought; !slices.Contains(got, equipment) {
		t.Errorf("%q is equipment and stays: %v", equipment, got)
	}
	if got := f.state.Units[0].Gear; slices.Contains(got, "Bow") {
		t.Errorf("the bow is lost: %v", got)
	}
}

// The dropped gear no longer counts: the promoted Crossbowman's points are those of a Crossbowman recruited without a bow, and a
// bought Spear that the promotion loses is not paid for twice.
func TestDroppedWargearNoLongerCountsInThePoints(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	f.companyOf("City of Umbar", 5, []UnitState{
		{ID: 90, Base: "Corsair of Umbar", Role: RoleWarrior, Gear: []string{"Bow"}, Bought: []string{"Spear", "Light Shield"}, XP: 5},
		{ID: 91, Base: "Corsair Crossbowman", Role: RoleWarrior},
	})
	before := f.unit(t, 90).Points
	promoteTo(t, f, s, 90, "Corsair Crossbowman")
	u, plain := f.unit(t, 90), f.unit(t, 91)
	if u.Points != plain.Points {
		t.Errorf("the lost gear still counts: %d points against %d (before the promotion %d)", u.Points, plain.Points, before)
	}
}

// A unit that becomes a hero keeps everything (Hero in the Making): the glaive of a Mirkwood Elf Warrior stays on the hero.
func TestAHeroInTheMakingKeepsItsWargear(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	f.companyOf("Halls of Thranduil", 5, []UnitState{{ID: 90, Base: "Mirkwood Elf Warrior", Role: RoleWarrior, Gear: []string{"Elven-made Glaive", "Shield"}, XP: 5}})
	roll, err := s.RollLevelUp(f.ctx, f.user, 5, 90, []int{6}, false)
	if err != nil {
		t.Fatal(err)
	}
	key := ""
	for _, o := range roll.Offers {
		for _, ch := range o.Choices {
			if strings.HasPrefix(ch.Key, "path:") && key == "" {
				key = ch.Key
			}
		}
	}
	if key == "" {
		t.Fatalf("no path offered: %+v", roll.Offers)
	}
	if _, err := s.ChooseLevelUp(f.ctx, f.user, 5, 90, []string{key}); err != nil {
		t.Fatal(err)
	}
	u := f.unit(t, 90)
	if !u.IsHero() {
		t.Fatal("a hero now")
	}
	if got := gearNames(u); !hasGlaive(got) || !hasItem(got, "Shield") {
		t.Errorf("the hero keeps his gear: %v", got)
	}
}

// Poison is a rule of the unit, not of the Bow: the Raider keeps the Bow and has the rule because his profile lists it; a profile
// that lists the Bow without the rule leaves the promoted warrior with a plain Bow.
func TestPoisonFollowsTheProfileNotTheBow(t *testing.T) {
	poisoned := func(u CompanyUnit) bool {
		return slices.ContainsFunc(u.Rules, func(r UnitRule) bool { return strings.HasPrefix(r.Name, "Poisoned Attacks") })
	}
	for _, c := range []struct {
		name, faction, from, to string
		gear                    []string
		poison                  bool
	}{
		{"the Haradrim Raider lists Poisoned Attacks (bow)", "Harad", "Haradrim Warrior", "Haradrim Raider", []string{"Bow"}, true},
		{"the Khandish Horseman lists the Bow without the rule", "Grand Army of the South", "Khandish Warrior", "Khandish Horseman", []string{"Bow"}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newCompanyFixture(t)
			s := f.leveller(t)
			f.companyOf(c.faction, 5, []UnitState{{ID: 90, Base: c.from, Role: RoleWarrior, Gear: c.gear, XP: 5}})
			promoteTo(t, f, s, 90, c.to)
			u := f.unit(t, 90)
			if !hasItem(gearNames(u), "Bow") {
				t.Errorf("the plain Bow stays: %v", gearNames(u))
			}
			if poisoned(u) != c.poison {
				t.Errorf("poisonous = %v, want %v: %+v", poisoned(u), c.poison, u.Rules)
			}
		})
	}
}

// Carried items are judged one by one against what the new profile lists. A stored exchange label carries the item it gives (the
// other was exchanged away); an exchange option of the new profile that gives that item means the exchange is already chosen.
func TestAnExchangeLabelIsJudgedItemByItem(t *testing.T) {
	rs := shippedRules(t)
	umbar, _ := rs.BC.Company("City of Umbar")
	grand, _ := rs.BC.Company("Grand Army of the South")
	cases := []struct {
		name string
		co   BCCompany
		to   string
		gear []string
		want []string
	}{
		{"the Charioteer offers the same exchange: it is already chosen", grand, "Khandish Charioteer", []string{"Exchange hand-and-a-half weapon for bow"}, []string{"Exchange hand-and-a-half weapon for bow"}},
		{"the Horseman lists both items as wargear: the profile carries them", grand, "Khandish Horseman", []string{"Exchange hand-and-a-half weapon for bow"}, nil},
		{"another exchange giving the crossbow is the one chosen", umbar, "Corsair Bo'sun", []string{"Exchange two-handed weapon for crossbow"}, []string{"Exchange spear for crossbow"}},
		{"the exchanged-away item is not carried", umbar, "Corsair Crossbowman", []string{"Exchange two-handed weapon for crossbow"}, nil},
		{"a profile without the given item loses it", umbar, "Corsair of Umbar", []string{"Exchange two-handed weapon for crossbow"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, bought := range []bool{false, true} {
				us := UnitState{Role: RoleWarrior}
				if bought {
					us.Bought = slices.Clone(c.gear)
				} else {
					us.Gear = slices.Clone(c.gear)
				}
				dropUnlisted(&levelEnv{rs: rs, bcC: c.co}, &us, c.to, nil)
				got := us.Gear
				if bought {
					got = us.Bought
				}
				if !slices.Equal(got, c.want) {
					t.Errorf("%v -> %v, want %v", c.gear, got, c.want)
				}
			}
		})
	}
}
