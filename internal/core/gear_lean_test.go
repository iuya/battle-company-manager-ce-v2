package core

import (
	"reflect"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

func yes() *bool { return &[]bool{true}[0] }

func TestLeanClasses(t *testing.T) {
	v, err := ParseVanilla([]byte(`{"data":{"gear":[{"name":"Dwarf Bow","type":"bow"},{"name":"Mace","type":"Weapon"},{"name":"Cloak"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		g    lean.Gear
		want []EquipmentClass
	}{
		{"named classes win", lean.Gear{Name: "Dwarf Bow", Classes: []string{"crossbow"}}, []EquipmentClass{EquipCrossbow}},
		{"the type of the entry", lean.Gear{Name: "Sling", Type: "throwingWeapon"}, []EquipmentClass{EquipThrowing}},
		{"the vanilla type of the same name", lean.Gear{Name: "dwarf bow"}, []EquipmentClass{EquipBow}},
		{"a type that does not say the class", lean.Gear{Name: "Mace"}, []EquipmentClass{EquipOther}},
		{"no type at all", lean.Gear{Name: "Cloak"}, []EquipmentClass{EquipOther}},
		{"nobody knows it", lean.Gear{Name: "Whistle"}, []EquipmentClass{EquipOther}},
	} {
		if got := LeanClasses(c.g, v); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// TestLeanBadges: the words the armoury page shows come from the who of the item and its limit; nothing decides by them.
func TestLeanBadges(t *testing.T) {
	for _, c := range []struct {
		name string
		g    lean.Gear
		want []string
	}{
		{"none", lean.Gear{}, nil},
		{"heroes", lean.Gear{Who: &lean.Who{Hero: yes()}}, []string{"hero"}},
		{"cavalry", lean.Gear{Who: &lean.Who{Keywords: []string{"Cavalry"}}}, []string{"cavalry"}},
		{"a race is no badge", lean.Gear{Who: &lean.Who{Race: []string{"Elf"}}}, nil},
		{"evil companies", lean.Gear{Who: &lean.Who{Alignment: "evil"}}, []string{"evil"}},
		{"a hero, and one in the company", lean.Gear{Who: &lean.Who{Hero: yes()}, Limit: 1}, []string{"hero", "unique"}},
		{"a limit of two is no uniqueness", lean.Gear{Limit: 2}, nil},
	} {
		if got := leanBadges(c.g); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestLeanAnnotation(t *testing.T) {
	v, err := ParseVanilla([]byte(`{"data":{"gear":[{"name":"Horse","type":"mount","definition":"A horse."}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	horse := LeanAnnotation(lean.Gear{Name: "Horse", Grants: []string{"Timid"}}, v, true)
	if horse.GrantedTo != "mount" || horse.MountProfile != "Horse" || horse.Definition != "A horse." || !reflect.DeepEqual(horse.Classes, []EquipmentClass{EquipMount}) {
		t.Errorf("a mount: its rules are the mount's, its profile its own name, its text vanilla's: %+v", horse)
	}
	goat := LeanAnnotation(lean.Gear{Name: "Goat", Type: "mount", Mount: "War goat"}, v, true)
	if goat.GrantedTo != "" || goat.MountProfile != "War goat" {
		t.Errorf("a mount with another profile and no rules: %+v", goat)
	}
	envenom := LeanAnnotation(lean.Gear{Name: "Envenom Weapon (X)", Classes: []string{"other"}, GrantsForChosenWeapon: &lean.ChosenGrant{Rule: "Poisoned Attacks", WeaponClasses: []string{"melee", "bow"}}}, v, true)
	if !envenom.WeaponModifier || envenom.ChosenWeapon == nil || envenom.ChosenWeapon.Rule != "Poisoned Attacks" ||
		!reflect.DeepEqual(envenom.ChosenWeapon.WeaponClasses, []EquipmentClass{EquipMelee, EquipBow}) {
		t.Errorf("a weapon modifier: %+v", envenom)
	}
	armour := LeanAnnotation(lean.Gear{Name: "Armour", Type: "armour", StatChanges: []lean.StatChange{{Stat: "defence", Amount: 1}, {Stat: "movement", Amount: -1, Condition: " when mounted "}}}, v, true)
	if len(armour.StatChanges) != 2 || armour.StatChanges[0].Condition != "" || armour.StatChanges[1].Condition != "when mounted" {
		t.Errorf("stat changes, one conditional: %+v", armour.StatChanges)
	}
	shield := LeanAnnotation(lean.Gear{Name: "Shield", Type: "shield", Abilities: &lean.Abilities{
		DefenceBonus: &lean.DefenceBonus{Value: 1, CancelledByClasses: []string{"bow"}, CancelledByItems: []string{"Pike"}, TwoHandedUse: true,
			LiftedByRules: []lean.DefenceLift{{Rule: "Expert Rider", Cancels: []string{"bow"}, WhileMounted: true}}}}}, v, true)
	d := shield.Defence
	if d == nil || d.Value != 1 || !d.TwoHandedUse || !reflect.DeepEqual(d.CancelledByClasses, []EquipmentClass{EquipBow}) || !reflect.DeepEqual(d.CancelledByItems, []string{"Pike"}) ||
		!reflect.DeepEqual(d.Lifted, []DefenceLift{{Rule: "Expert Rider", Cancels: []string{"bow"}, WhileMounted: true}}) {
		t.Errorf("a shield: %+v", d)
	}
	glaive := LeanAnnotation(lean.Gear{Name: "Elven Glaive", Abilities: &lean.Abilities{AttacksBonus: &lean.AttacksBonus{Value: 1, Condition: "while engaged"}}}, v, true)
	if !reflect.DeepEqual(glaive.Situational, []StatChange{{Stat: "attacks", Delta: 1, Condition: "while engaged"}}) {
		t.Errorf("a situational bonus is a note: %+v", glaive.Situational)
	}
	bow := LeanAnnotation(lean.Gear{Name: "Bow", Type: "bow", Ranged: &lean.Ranged{Range: 24, Strength: 2}, Who: &lean.Who{Race: []string{"Elf"}}}, v, true)
	if bow.Stats() != `24" · S2` {
		t.Errorf("a bow: %s", bow.Stats())
	}
}

func TestLeanGear(t *testing.T) {
	v, err := ParseVanilla([]byte(`{"stats":[{"name":"Movement"},{"name":"Fight"}],"data":{"gear":[{"name":"Spear","type":"spear","definition":"A spear."}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	two := lean.Rating{Warrior: 2, Hero: 5}
	doc := &lean.Document{
		Gear: []lean.Gear{
			{Name: "Spear", Kind: "wargear", Influence: 1, Rating: lean.Rating{Warrior: 1, Hero: 5}},
			{Name: "Elven spear", Kind: "wargear", PricedAs: "Spear", Classes: []string{"spear"}, Who: &lean.Who{Race: []string{"Elf"}}},
			{Name: "Axe", Kind: "wargear", Unsold: true},
			{Name: "Seeing Stone", Kind: "equipment", Size: "small", Influence: 4, Rating: lean.Rating{Warrior: 10, Hero: 10}, Who: &lean.Who{Hero: yes()}, Limit: 1},
			{Name: "Healing Herbs", Kind: "equipment", Size: "small", Influence: 1, Rating: lean.Rating{Warrior: 1, Hero: 1}, AftermathEffect: "healingHerbs"},
			{Name: "Falcon", Kind: "creature", Influence: 3, Rating: lean.Rating{Warrior: 10, Hero: 10}, Profile: map[string]any{"unitType": []any{"Beast"}, "movement": 12.0}},
			{Name: "Wandering Sage", Kind: "wanderer", Influence: 9, Rating: lean.Rating{Warrior: 60, Hero: 60}, Profile: map[string]any{"heroicActions": []any{"Heroic Resolve"}, "might": 1.0}},
		},
		Companies: []lean.Company{{Name: "Umbar", Gear: []lean.Gear{
			{Name: "Smoke Bombs", Kind: "wargear", Influence: 2, Rating: lean.Rating{Warrior: 10, Hero: 10}, Limit: 1, Who: &lean.Who{Units: []string{"Corsair"}, Text: "Corsair"}},
		}}, {Name: "Rohan"}},
		ArmourUp: &lean.ArmourUpgrade{Stages: [][]string{{"None"}, {"Armour"}}, Influence: 1, Rating: lean.Rating{Warrior: 1, Hero: 5}, Gold: &two},
	}
	got := LeanGear(doc, v)
	if n := len(got.Offers); n != 4 {
		t.Fatalf("%d offers, want the spear, the Elven spear, the Stone and the herbs: %+v", n, got.Offers)
	}
	elven := got.Offers[1]
	if elven.Cost != 1 || elven.Rating != (Rating{1, 5}) || elven.Gold != (Rating{1, 5}) || !reflect.DeepEqual(elven.Classes, []EquipmentClass{EquipSpear}) {
		t.Errorf("an item priced as another takes its price: %+v", elven)
	}
	if st := got.Offers[2]; !reflect.DeepEqual(st.Restrictions, []string{"hero", "unique"}) || st.Gold != (Rating{10, 10}) {
		t.Errorf("a shared item: %+v", st)
	}
	if got.Offers[0].Description != "A spear." {
		t.Errorf("the text of an item is vanilla's when the entry has none: %q", got.Offers[0].Description)
	}
	if _, ok := got.Annotations["axe"]; !ok {
		t.Error("an unsold item is still annotated")
	}
	if len(got.Armoury("Rohan")) != 4 || len(got.Armoury("Umbar")) != 5 {
		t.Errorf("a company's armoury is the common items and its own: %d and %d", len(got.Armoury("Rohan")), len(got.Armoury("Umbar")))
	}
	smoke := got.Local["Umbar"][0]
	if !smoke.Exclusive || smoke.Limit != 1 || smoke.Who.Label() != "Corsair" || !reflect.DeepEqual(smoke.Who.Units, []string{"Corsair"}) || smoke.Restrictions != nil {
		t.Errorf("a unique item: %+v", smoke)
	}
	if got.ItemEffects["healing herbs"] != "healingHerbs" {
		t.Errorf("item effects: %v", got.ItemEffects)
	}
	if len(got.Creatures) != 1 || got.Creatures[0].Name != "Falcon" || got.Creatures[0].Cost != 3 || got.Creatures[0].Points != 10 || got.Creatures[0].Stats["movement"] != 12 {
		t.Errorf("creatures: %+v", got.Creatures)
	}
	if len(got.Wanderers) != 1 || got.Wanderers[0].Cost != 9 || got.Wanderers[0].Points != 60 || got.Wanderers[0].HeroicActions[0] != "Heroic Resolve" {
		t.Errorf("wanderers: %+v", got.Wanderers)
	}
	if got.ArmourUp.Cost != 1 || got.ArmourUp.Gold != (Rating{2, 5}) || len(got.ArmourUp.Stages) != 2 {
		t.Errorf("armour upgrade: %+v", got.ArmourUp)
	}
}

func TestLeanLimitRules(t *testing.T) {
	got := LeanLimitRules([]lean.Limit{
		{ID: "bow", Type: "bow", MaximumPercentage: 33.33, RoundUp: true, Classes: []string{"bow", "crossbow"}, Label: "bows (heroes included)"},
		{ID: "cavalry", Type: "keyword", Value: "Cavalry", MaximumPercentage: 33.33, RoundUp: true, Label: "Cavalry"},
	})
	want := []LimitRule{
		{ID: "bow", Label: "Bows", Share: lean.Share{Num: 3333, Den: 10000}, RoundUp: true, Classes: []EquipmentClass{EquipBow, EquipCrossbow}},
		{ID: "cavalry", Label: "Cavalry", Share: lean.Share{Num: 3333, Den: 10000}, RoundUp: true, Keyword: "Cavalry"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}
