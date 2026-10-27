package core

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func poisonRules(u CompanyUnit) []string {
	var out []string
	for _, r := range u.Rules {
		if strings.HasPrefix(r.Name, "Poisoned Attacks") {
			out = append(out, r.Name)
		}
	}
	return out
}

func builtUnit(t *testing.T, company string, s UnitState) CompanyUnit {
	t.Helper()
	rs := shippedRules(t)
	c, ok := rs.BC.Company(company)
	if !ok {
		t.Fatalf("no company %q", company)
	}
	return buildUnit(rs, c, s, CampaignSettings{})
}

// Envenom Weapon chooses ONE weapon of any class its annotation lists (grantsRuleForChosenWeapon.weaponClasses): the unit gains
// Poisoned Attacks (<weapon>) once and the weapon stays the plain item.
func TestEnvenomWeaponTakesAnyWeaponClassOfTheData(t *testing.T) {
	rs := shippedRules(t)
	a, _ := rs.BC.Annotation("Envenom Weapon (X)")
	want := []EquipmentClass{EquipMelee, EquipTwoHanded, EquipSpear, EquipLance, EquipBow, EquipCrossbow, EquipThrowing, EquipMissile}
	if a.ChosenWeapon == nil || a.ChosenWeapon.Rule != "Poisoned Attacks" || !slices.Equal(a.ChosenWeapon.WeaponClasses, want) {
		t.Fatalf("Envenom Weapon annotation = %+v", a.ChosenWeapon)
	}
	for _, c := range want {
		if !rs.BC.CanBePoisoned(Equipment{Class: c}) {
			t.Errorf("a %s can be envenomed", c)
		}
	}
	for _, c := range []EquipmentClass{EquipArmour, EquipShield, EquipMount, EquipOther} {
		if rs.BC.CanBePoisoned(Equipment{Class: c}) {
			t.Errorf("a %s cannot be envenomed", c)
		}
	}
	for item, class := range map[string]EquipmentClass{"Hand weapon": EquipMelee, "Two-handed weapon": EquipTwoHanded, "Spear": EquipSpear, "Lance": EquipLance,
		"Bow": EquipBow, "Crossbow": EquipCrossbow, "Throwing weapon": EquipThrowing} {
		t.Run(string(class), func(t *testing.T) {
			u := builtUnit(t, "Mordor", UnitState{ID: 1, Base: "Mordor Orc Warrior", Role: RoleWarrior, Gear: []string{item}, Bought: []string{"Envenom Weapon (" + item + ")"}})
			rule := "Poisoned Attacks (" + item + ")"
			if got := poisonRules(u); !slices.Equal(got, []string{rule}) {
				t.Fatalf("rules %v, want %q once", got, rule)
			}
			for _, e := range u.Equipment {
				switch {
				case strings.HasPrefix(e.Name, "Envenom Weapon"):
					if e.Modifies != item || !slices.Contains(e.Choices, item) || e.Size != "small" {
						t.Errorf("the modifier is for %q: %+v", item, e)
					}
				case e.Name == item && (e.Poisoned || e.Modifies != ""):
					t.Errorf("the weapon stays the plain item: %+v", e)
				}
			}
			if slices.Contains(u.PoisonableWeapons, item) {
				t.Errorf("%q is poisoned and cannot be chosen again: %v", item, u.PoisonableWeapons)
			}
		})
	}
}

// Without a weapon chosen, or with a weapon the unit does not carry, the modifier gives nothing and offers the weapons to choose from.
func TestEnvenomWeaponWaitsForItsWeapon(t *testing.T) {
	for _, bought := range []string{"Envenom Weapon (X)", "Envenom Weapon (Lance)"} {
		u := builtUnit(t, "Mordor", UnitState{ID: 1, Base: "Mordor Orc Warrior", Role: RoleWarrior, Gear: []string{"Spear"}, Bought: []string{bought}})
		if got := poisonRules(u); len(got) != 0 {
			t.Errorf("%s: no weapon chosen, no rule: %v", bought, got)
		}
		for _, e := range u.Equipment {
			if strings.HasPrefix(e.Name, "Envenom Weapon") && (e.Modifies != "" || !slices.Contains(e.Choices, "Spear") || !slices.Contains(e.Choices, "Hand weapon")) {
				t.Errorf("%s: %+v", bought, e)
			}
		}
		if !slices.Contains(u.PoisonableWeapons, "Spear") {
			t.Errorf("%s: %v", bought, u.PoisonableWeapons)
		}
	}
}

// A weapon already poisoned (by the profile, the Blowpipe) cannot be chosen, and a unit with the rule from the profile and from the
// item shows it once.
func TestAPoisonedWeaponCannotBeEnvenomedAndTheRuleShowsOnce(t *testing.T) {
	haradrim := builtUnit(t, "Harad", UnitState{ID: 1, Base: "Haradrim Warrior", Role: RoleWarrior, Gear: []string{"Bow"}, Bought: []string{"Envenom Weapon (Bow)"}})
	if got := poisonRules(haradrim); len(got) != 1 {
		t.Errorf("the profile's rule shows once: %v", got)
	}
	if slices.Contains(haradrim.PoisonableWeapons, "Bow") || !slices.Contains(haradrim.PoisonableWeapons, "Hand weapon") {
		t.Errorf("the Bow is poisoned already: %v", haradrim.PoisonableWeapons)
	}
	for _, e := range haradrim.Equipment {
		if strings.HasPrefix(e.Name, "Envenom Weapon") && (e.Modifies != "" || slices.Contains(e.Choices, "Bow")) {
			t.Errorf("Envenom Weapon cannot take the poisoned Bow: %+v", e)
		}
	}
	mahud := builtUnit(t, "Harad", UnitState{ID: 2, Base: "Mahûd Warrior", Role: RoleWarrior, Gear: []string{"Blowpipe"}, Bought: []string{"Envenom Weapon (Blowpipe)"}})
	if got := poisonRules(mahud); len(got) != 1 {
		t.Errorf("the Blowpipe and the profile give one rule: %v", got)
	}
	if slices.Contains(mahud.PoisonableWeapons, "Blowpipe") {
		t.Errorf("the Blowpipe is poisoned already: %v", mahud.PoisonableWeapons)
	}
	other := builtUnit(t, "Harad", UnitState{ID: 3, Base: "Haradrim Warrior", Role: RoleWarrior, Gear: []string{"Bow"}, Bought: []string{"Envenom Weapon (Hand weapon)"}})
	if got := poisonRules(other); len(got) != 2 {
		t.Errorf("the Bow (profile) and the Hand weapon (Envenom): %v", got)
	}
}

// The chosen weapon is stored in the name of the modifier; the armoury and the annotations know it by its generic name.
func TestAChosenWeaponModifierIsStillTheArmouryItem(t *testing.T) {
	rs := shippedRules(t)
	c, _ := rs.BC.Company("Mordor")
	if base, weapon, ok := rs.BC.ChosenWeapon("Envenom Weapon (Elf Bow)"); !ok || base != "Envenom Weapon (X)" || weapon != "Elf Bow" {
		t.Errorf("chosen = %q %q %v", base, weapon, ok)
	}
	if _, _, ok := rs.BC.ChosenWeapon("Envenom Weapon (X)"); ok {
		t.Error("(X) has no weapon yet")
	}
	if _, _, ok := rs.BC.ChosenWeapon("Shield (Light)"); ok {
		t.Error("a shield is no weapon modifier")
	}
	if got := ChosenWeaponName("Envenom Weapon (X)", "Bow"); got != "Envenom Weapon (Bow)" {
		t.Errorf("name = %q", got)
	}
	plain, _ := rs.BC.Offer(c, "Envenom Weapon (X)")
	chosen, ok := rs.BC.Offer(c, "Envenom Weapon (Bow)")
	if !ok || chosen.Name != plain.Name || chosen.Cost != plain.Cost || chosen.Size != "small" {
		t.Errorf("offer %+v / %+v", chosen, plain)
	}
}

// A promotion to a profile that no longer lists the chosen weapon makes the modifier a modifier without a weapon again; one whose
// profile lists it keeps the choice. The unit never keeps a rule for a weapon it does not carry.
func TestPromotionClearsAnEnvenomChoiceItsProfileNoLongerLists(t *testing.T) {
	rs := shippedRules(t)
	c, _ := rs.BC.Company("City of Umbar")
	for _, tc := range []struct {
		name, to string
		gear     []string
		bought   []string
		want     []string
	}{
		{"the weapon is lost: the choice is cleared", "Corsair Crossbowman", []string{"Bow"}, []string{"Envenom Weapon (Bow)"}, []string{"Envenom Weapon (X)"}},
		{"the weapon is listed: the choice stays", "Corsair Crossbowman", []string{"Crossbow"}, []string{"Envenom Weapon (Crossbow)"}, []string{"Envenom Weapon (Crossbow)"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			us := UnitState{Role: RoleWarrior, Gear: tc.gear, Bought: tc.bought}
			dropUnlisted(&levelEnv{rs: rs, bcC: c}, &us, tc.to, nil)
			got := slices.DeleteFunc(slices.Clone(us.Bought), func(g string) bool { return !strings.HasPrefix(g, "Envenom") })
			if !slices.Equal(got, tc.want) {
				t.Errorf("bought %v, want %v", us.Bought, tc.want)
			}
		})
	}
}

func envenomOf(t *testing.T, u CompanyUnit) Equipment {
	t.Helper()
	for _, e := range u.Equipment {
		if strings.HasPrefix(e.Name, "Envenom Weapon") {
			return e
		}
	}
	t.Fatalf("no Envenom Weapon on %s: %v", u.Title(), gearNames(u))
	return Equipment{}
}

// Buying Envenom Weapon with a weapon chosen stores the choice in the item; buying it without one leaves the choice to
// ChooseModifierWeapon, which also chooses again.
func TestBuyingAndChoosingTheWeaponOfAnEnvenomWeapon(t *testing.T) {
	newUnit := func() (*companyFixture, *ArmouryService) {
		f := newCompanyFixture(t)
		f.companyOf("Mordor", 12, []UnitState{{ID: 7, Base: "Mordor Orc Warrior", Role: RoleWarrior, Bought: []string{"Spear"}}})
		return f, f.armoury(t)
	}
	t.Run("bought with its weapon", func(t *testing.T) {
		f, s := newUnit()
		if err := s.BuyEquipmentFor(f.ctx, f.user, 5, 7, "Envenom Weapon (X)", "", "spear"); err != nil {
			t.Fatal(err)
		}
		u := f.unit(t, 7)
		if e := envenomOf(t, u); e.Name != "Envenom Weapon (Spear)" || e.Modifies != "Spear" {
			t.Errorf("%+v", e)
		}
		if got := poisonRules(u); !slices.Equal(got, []string{"Poisoned Attacks (Spear)"}) {
			t.Errorf("rules %v", got)
		}
	})
	t.Run("bought without a weapon, chosen then chosen again", func(t *testing.T) {
		f, s := newUnit()
		if err := s.BuyEquipment(f.ctx, f.user, 5, 7, "Envenom Weapon (X)", ""); err != nil {
			t.Fatal(err)
		}
		e := envenomOf(t, f.unit(t, 7))
		if e.Name != "Envenom Weapon (X)" || e.Modifies != "" || !slices.Equal(e.Choices, []string{"Hand weapon", "Spear"}) {
			t.Fatalf("pending: %+v", e)
		}
		if err := s.ChooseModifierWeapon(f.ctx, f.user, 5, 7, e.Name, "Hand weapon"); err != nil {
			t.Fatal(err)
		}
		e = envenomOf(t, f.unit(t, 7))
		if e.Modifies != "Hand weapon" || !slices.Equal(poisonRules(f.unit(t, 7)), []string{"Poisoned Attacks (Hand weapon)"}) {
			t.Fatalf("chosen: %+v", e)
		}
		if err := s.ChooseModifierWeapon(f.ctx, f.user, 5, 7, e.Name, "Spear"); err != nil {
			t.Fatal(err)
		}
		if e = envenomOf(t, f.unit(t, 7)); e.Modifies != "Spear" || e.Name != "Envenom Weapon (Spear)" {
			t.Errorf("chosen again: %+v", e)
		}
		if got := poisonRules(f.unit(t, 7)); !slices.Equal(got, []string{"Poisoned Attacks (Spear)"}) {
			t.Errorf("one rule: %v", got)
		}
	})
	t.Run("refusals", func(t *testing.T) {
		f, s := newUnit()
		if err := s.BuyEquipmentFor(f.ctx, f.user, 5, 7, "Envenom Weapon (X)", "", "Crossbow"); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "Hand weapon, Spear") {
			t.Errorf("a weapon the unit lacks is refused with the choices: %v", err)
		}
		if err := s.BuyEquipmentFor(f.ctx, f.user, 5, 7, "Map", "", "Spear"); !errors.Is(err, ErrRefused) {
			t.Errorf("Map is for no weapon: %v", err)
		}
		if len(f.state.Units[0].Bought) != 1 || f.state.Influence != 12 {
			t.Errorf("nothing was bought or paid: %+v %d", f.state.Units[0].Bought, f.state.Influence)
		}
		_ = s.BuyEquipment(f.ctx, f.user, 5, 7, "Envenom Weapon (X)", "")
		if err := s.ChooseModifierWeapon(f.ctx, f.user, 5, 7, "Envenom Weapon (X)", "Crossbow"); !errors.Is(err, ErrRefused) {
			t.Errorf("not a weapon of the unit: %v", err)
		}
		if err := s.ChooseModifierWeapon(f.ctx, f.user, 5, 7, "Shield", "Spear"); !errors.Is(err, ErrNotFound) {
			t.Errorf("not a weapon modifier: %v", err)
		}
		if err := s.ChooseModifierWeapon(f.ctx, f.user, 5, 99, "Envenom Weapon (X)", "Spear"); !errors.Is(err, ErrNotFound) {
			t.Errorf("unknown unit: %v", err)
		}
	})
}

// Through the real LevelService: the Corsair loses the Bow he envenomed, so the modifier is a modifier without a weapon again and
// the unit has no Poisoned Attacks (Bow); he may choose again among the weapons he has now.
func TestPromotionLosingTheEnvenomedWeaponThroughTheLevelService(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	f.companyOf("City of Umbar", 5, []UnitState{{ID: 90, Base: "Corsair of Umbar", Role: RoleWarrior, Gear: []string{"Bow"}, Bought: []string{"Envenom Weapon (Bow)"}, XP: 5}})
	if got := poisonRules(f.unit(t, 90)); !slices.Equal(got, []string{"Poisoned Attacks (Bow)"}) {
		t.Fatalf("before: %v", got)
	}
	promoteTo(t, f, s, 90, "Corsair Crossbowman")
	u := f.unit(t, 90)
	e := envenomOf(t, u)
	if e.Name != "Envenom Weapon (X)" || e.Modifies != "" || len(poisonRules(u)) != 0 {
		t.Errorf("the choice is cleared: %+v %v", e, poisonRules(u))
	}
	if !slices.Contains(e.Choices, "Crossbow") || slices.Contains(e.Choices, "Bow") {
		t.Errorf("choices among the new weapons: %v", e.Choices)
	}
}
