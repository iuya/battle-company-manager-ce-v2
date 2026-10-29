package core

import (
	"reflect"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

func TestClassesOfVanillaType(t *testing.T) {
	for typ, want := range map[string][]EquipmentClass{
		"mount": {EquipMount}, "Mount": {EquipMount}, " bow ": {EquipBow}, "armour": {EquipArmour}, "Shield": {EquipShield}, "shield": {EquipShield},
		"banner": {EquipBanner}, "Banner": {EquipBanner}, "throwingWeapon": {EquipThrowing}, "blowpipe": {EquipMissile}, "horn": {EquipHorn},
		"spear": {EquipSpear}, "Spear": {EquipSpear}, "Two-handed weapon": {EquipTwoHanded}, "upgrade": {EquipOther}, "stat": {EquipOther},
		"Weapon": nil, "weapon": nil, "siegeEngine": nil, "": nil,
	} {
		if got := ClassesOfVanillaType(typ); !reflect.DeepEqual(got, want) {
			t.Errorf("ClassesOfVanillaType(%q) = %v, want %v", typ, got, want)
		}
	}
}

func TestOptionNotSold(t *testing.T) {
	words := []string{"banner", "war horn", "war drum"}
	for _, c := range []struct {
		option string
		want   bool
	}{
		{"Banner", true}, {"Exchange lance for banner", true}, {"War Horn", true}, {"Exchange shield for war drum", true}, {"Standard bearer", false},
		{"Shield", false}, {"", false}, {"Hand-and-a-half hammer", false},
	} {
		if got := OptionNotSold(c.option, words); got != c.want {
			t.Errorf("OptionNotSold(%q) = %v, want %v", c.option, got, c.want)
		}
	}
	if OptionNotSold("Banner", nil) || OptionNotSold("Banner", []string{""}) {
		t.Error("no word, no unsold option")
	}
}

// miniVanilla is a vanilla file of two heroes and three warriors for the derivations.
const miniVanilla = `{"data":{
"heroes":[{"name":"Galadriel","wargear":["Elven cloak"]},{"name":"Celeborn","wargear":[]}],
"warriors":[
 {"name":"Galadhrim Warrior","wargear":["Armour","Hand weapon"],"options":[
   {"name":"Lorien Guard","availableIn":["Galadriel","Celeborn"]},
   {"name":"Cloaks","availableIn":["Galadriel","The Last Alliance"]},
   {"name":"Bow"}]},
 {"name":"Dwarf Ranger","wargear":["Heavy armour","Hand weapon"],"options":[{"name":"Banner","availableIn":["Galadriel"]},{"name":"Banner","availableIn":["Galadriel"]}]},
 {"name":"Hunter Orc Warrior","wargear":["Hand weapon","Armour"]}]}}`

func TestVanillaReservedOptions(t *testing.T) {
	v, err := ParseVanilla([]byte(miniVanilla))
	if err != nil {
		t.Fatal(err)
	}
	want := []ReservedOption{
		{Unit: "Dwarf Ranger", Option: "Banner", AvailableIn: []string{"Galadriel"}},
		{Unit: "Galadhrim Warrior", Option: "Lorien Guard", AvailableIn: []string{"Galadriel", "Celeborn"}},
	}
	if got := v.ReservedOptions(); !reflect.DeepEqual(got, want) {
		t.Errorf("ReservedOptions = %v\nwant %v", got, want) // an option that also names an army is not reserved; a repeated one counts once
	}
}

func TestVanillaProfileWargearNotSold(t *testing.T) {
	v, err := ParseVanilla([]byte(miniVanilla))
	if err != nil {
		t.Fatal(err)
	}
	sold := func(item string) bool { return ItemKey(item) == ItemKey("Hand weapons") } // the plural is the same item
	for _, c := range []struct {
		name  string
		units []string
		want  []ProfileWargear
	}{
		{"every unit a company uses", []string{"Hunter Orc Warrior", "Galadhrim Warrior", "Dwarf Ranger"},
			[]ProfileWargear{{Item: "Armour", Units: []string{"Galadhrim Warrior", "Hunter Orc Warrior"}}, {Item: "Heavy armour", Units: []string{"Dwarf Ranger"}}}},
		{"a unit twice and a unit vanilla lacks count once or not at all", []string{"Dwarf Ranger", "Dwarf Ranger", "Nobody"},
			[]ProfileWargear{{Item: "Heavy armour", Units: []string{"Dwarf Ranger"}}}},
		{"no unit, no item", nil, nil},
	} {
		if got := v.ProfileWargearNotSold(c.units, sold); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestApplyFillIns(t *testing.T) {
	v, err := ParseVanilla([]byte(`{"data":{"warriors":[
 {"name":"Hunter Orc Warrior"},
 {"name":"Vault Warden Team","race":["Dwarf"],"faction":["Erebor"],"unitType":["Warrior"]}],
"gear":[{"name":"Brego","type":"mount"},{"name":"Elk","type":"mount","race":["Elk"]}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	ApplyLeanFillIns(v, &lean.Vanilla{
		Units: map[string]lean.VanillaFill{
			"Hunter Orc Warrior": {Race: []string{"Orc"}, Faction: []string{"Gundabad"}, UnitType: []string{"Warrior", "Infantry"}},
			"Vault Warden Team":  {Race: []string{"Man"}, Faction: []string{"Gondor"}, UnitType: []string{"Hero"}},
			"Nobody":             {Race: []string{"Orc"}},
		},
		Mounts: map[string]lean.VanillaFill{"Brego": {Race: []string{"Horse"}, Faction: []string{"See Rider"}, UnitType: []string{"Mount"}}, "Elk": {Race: []string{"Boar"}, UnitType: []string{"Mount"}}},
	})
	hunter, _ := v.Unit("Hunter Orc Warrior")
	if !reflect.DeepEqual(hunter.Race, []string{"Orc"}) || !reflect.DeepEqual(hunter.Faction, []string{"Gundabad"}) || !reflect.DeepEqual(hunter.UnitTypes, []string{"Warrior", "Infantry"}) {
		t.Errorf("a unit vanilla leaves empty is filled: %+v", hunter)
	}
	team, _ := v.Unit("Vault Warden Team")
	if !reflect.DeepEqual(team.Race, []string{"Dwarf"}) || !reflect.DeepEqual(team.UnitTypes, []string{"Warrior"}) {
		t.Errorf("a field vanilla defines is never changed: %+v", team)
	}
	brego, _ := v.Gear("Brego")
	elk, _ := v.Gear("Elk")
	if !reflect.DeepEqual(brego.Race, []string{"Horse"}) || !reflect.DeepEqual(elk.Race, []string{"Elk"}) || !reflect.DeepEqual(elk.UnitTypes, []string{"Mount"}) {
		t.Errorf("mounts: %+v %+v", brego, elk)
	}
	if !v.IsRace("Orc") || !v.IsRace("Dwarf") {
		t.Error("the races of the filled-in units count as races")
	}
}

func TestLeanOptionSales(t *testing.T) {
	s := LeanOptionSales(&lean.Vanilla{Options: &lean.VanillaOptions{
		NotSold: []string{"banner", "war horn"},
		Exceptions: []lean.OptionException{
			{Unit: "Cave Troll", Option: "Hand-and-a-half hammer", ReadAs: "Hand-and-a-half Weapon"},
			{Unit: "Cave Troll", Option: "Troll Spear", OwnItem: "Troll Spear"},
			{Unit: "Ruffian", Option: "Whip", Granted: true},
			{Unit: "Lothlorien Warrior", Option: "Wood Elf spear", NotSold: true},
			{Unit: "Moria Goblin", Option: "Banner", ReadAs: "Company Standard"}, // an exception beats the word
		}}})
	for _, c := range []struct {
		unit, option string
		unsold       bool
		read         string
	}{
		{"Angmar Orc Warrior", "Banner", true, ""}, {"Black Numenorean Knight", "Exchange lance for war horn", true, ""},
		{"Cave Troll", "Hand-and-a-half hammer", false, "Hand-and-a-half Weapon"}, {"Cave Troll", "Troll Spear", false, "Troll Spear"},
		{"Ruffian", "Whip", true, ""}, {"Lothlorien Warrior", "Wood Elf spear", true, ""}, {"Moria Goblin", "Banner", false, "Company Standard"},
		{"Rohan Royal Guard", "Shield", false, ""},
	} {
		if got := s.Unsold(c.unit, c.option); got != c.unsold {
			t.Errorf("%s / %s: unsold %v, want %v", c.unit, c.option, got, c.unsold)
		}
		if item, ok := s.ReadAs(c.unit, c.option); item != c.read || ok != (c.read != "") {
			t.Errorf("%s / %s: read as %q %v, want %q", c.unit, c.option, item, ok, c.read)
		}
	}
	if (LeanOptionSales(nil)).Unsold("A", "Banner") {
		t.Error("a file without the rule sells everything")
	}
}
