package core

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func ids(units []CompanyUnit) []int64 {
	var out []int64
	for _, u := range units {
		out = append(out, u.ID)
	}
	return out
}

func TestOrderUnitsDefault(t *testing.T) {
	units := []CompanyUnit{
		{ID: 1, Role: RoleWarrior}, {ID: 2, Role: RoleHero}, {ID: 3, Role: RoleSergeant}, {ID: 4, Role: RoleWarrior},
		{ID: 5, Role: RoleLeader}, {ID: 6, Role: RoleSergeant},
	}
	if got, want := ids(OrderUnits(units, nil)), []int64{5, 3, 6, 2, 1, 4}; !slices.Equal(got, want) {
		t.Errorf("default order: got %v, want %v", got, want)
	}
}

func TestOrderUnitsHonoursTheSavedOrder(t *testing.T) {
	units := []CompanyUnit{{ID: 1, Role: RoleLeader}, {ID: 2, Role: RoleWarrior}, {ID: 3, Role: RoleWarrior}, {ID: 4, Role: RoleHero}}
	// the player moved 3 to the top; 4 is new (not saved yet) and follows in the default order
	if got, want := ids(OrderUnits(units, []int64{3, 1, 2})), []int64{3, 1, 2, 4}; !slices.Equal(got, want) {
		t.Errorf("saved order: got %v, want %v", got, want)
	}
}

func TestCompanyLimits(t *testing.T) {
	bow := Equipment{Name: "Bow", Class: EquipBow}
	units := []CompanyUnit{
		{Kind: KindInfantry, Equipment: []Equipment{bow}}, {Kind: KindInfantry, Equipment: []Equipment{bow, {Name: "Throwing axe", Class: EquipThrowing}}},
		{Kind: KindCavalry}, {Kind: KindMonstrousCavalry}, {Kind: KindInfantry}, {Kind: KindInfantry}, {Kind: KindInfantry},
	}
	limits := shippedLimits(t, units) // 7 units: a third rounded up is 3
	want := []CompanyLimit{{ID: "bow", Label: "Bows", Used: 2, Max: 3, Next: 3}, {ID: "throwing", Label: "Throwing weapons", Used: 1, Max: 3, Next: 3}, {ID: "cavalry", Label: "Cavalry", Used: 2, Max: 3, Next: 3}}
	for i := range limits {
		limits[i].LimitClasses, limits[i].LimitKeyword = nil, ""
	}
	if !reflect.DeepEqual(limits, want) {
		t.Errorf("got %+v, want %+v", limits, want)
	}
	if s := (CompanyLimit{Used: 4, Max: 3, Exempt: 1}).Segments(); s != 5 { // over the limit the bar grows to show every counted one
		t.Errorf("segments over the limit: %d", s)
	}
	if s := (CompanyLimit{}).Segments(); s != 1 {
		t.Errorf("an empty limit still draws one segment: %d", s)
	}
	if limits[0].Full() || !(CompanyLimit{Used: 3, Max: 3}).Full() {
		t.Error("Full")
	}
}

func TestARiderWithABoughtMountCountsAsCavalry(t *testing.T) {
	horse := Equipment{Name: "Horse", Class: EquipMount, Bought: true}
	units := []CompanyUnit{{Kind: KindInfantry, Equipment: []Equipment{horse}}, {Kind: KindCavalry, Equipment: []Equipment{horse}}, {Kind: KindInfantry}}
	if l := shippedLimits(t, units)[2]; l.Used != 2 || l.Max != 1 {
		t.Errorf("the mounted warrior and the cavalry unit count once each: %+v", l)
	}
}

func TestUnitTitle(t *testing.T) {
	if (CompanyUnit{Name: " Hador ", Type: "Warrior"}).Title() != " Hador " || (CompanyUnit{Type: "Warrior"}).Title() != "Warrior" {
		t.Error("title is the name, or the type without one")
	}
}

func TestSortEquipment(t *testing.T) {
	items := []Equipment{
		{Name: "Bow", Class: EquipBow}, {Name: "Heavy armour", Class: EquipArmour}, {Name: "Banner", Class: EquipBanner}, {Name: "Dagger", Class: EquipMelee, Bought: true},
		{Name: "Shield", Class: EquipShield}, {Name: "Horse", Class: EquipMount}, {Name: "Sword", Class: EquipMelee}, {Name: "Throwing axe", Class: EquipThrowing},
	}
	var names []string
	for _, e := range SortEquipment(items) {
		names = append(names, e.Name)
	}
	want := []string{"Horse", "Sword", "Dagger", "Shield", "Heavy armour", "Bow", "Throwing axe", "Banner"}
	if !slices.Equal(names, want) {
		t.Errorf("got %v, want %v", names, want)
	}
}

func TestSortRules(t *testing.T) {
	rules := []UnitRule{
		{Name: "From a sword", Source: RuleFromEquipment}, {Name: "From the path", Source: RuleFromPath}, {Name: "Spell", Source: RuleFromUnit, Spell: true}, {Name: "Own", Source: RuleFromUnit},
	}
	var names []string
	for _, r := range SortRules(rules) {
		names = append(names, r.Name)
	}
	if want := []string{"Own", "Spell", "From the path", "From a sword"}; !slices.Equal(names, want) {
		t.Errorf("got %v, want %v", names, want)
	}
}

func TestLevelUps(t *testing.T) {
	for _, c := range []struct{ level, xp, inLevel, pending int }{{0, 3, 3, 0}, {0, 5, 5, 1}, {0, 15, 15, 3}, {1, 15, 10, 2}, {3, 17, 2, 0}} {
		u := CompanyUnit{Level: c.level, XP: c.xp, LevelEvery: 5}
		if u.XPInLevel() != c.inLevel || u.PendingLevelUps() != c.pending {
			t.Errorf("level %d xp %d: got %d in level, %d pending; want %d, %d", c.level, c.xp, u.XPInLevel(), u.PendingLevelUps(), c.inLevel, c.pending)
		}
	}
}

func TestKindOf(t *testing.T) {
	for types, want := range map[string]UnitKind{"Hero,Infantry": KindInfantry, "Warrior,Cavalry": KindCavalry, "Warrior,Monster,Infantry": KindMonster, "Hero,Beast": KindBeast, "Cavalry,Monster": KindMonstrousCavalry, "War Beast": KindBeast} {
		if got := KindOf(strings.Split(types, ",")); got != want {
			t.Errorf("%s: got %s, want %s", types, got, want)
		}
	}
}

func TestDualItemsSortAndCountEverywhere(t *testing.T) {
	// a spear-bow is placed with the melee weapons (its best class) and counts as a bow for the limit
	units := []CompanyUnit{{Kind: KindInfantry, Equipment: SortEquipment([]Equipment{
		{Name: "Armour", Class: EquipArmour}, {Name: "Spear-bow", Class: EquipBow, Also: []EquipmentClass{EquipSpear}}, {Name: "Shield", Class: EquipShield},
	})}}
	if units[0].Equipment[0].Name != "Spear-bow" {
		t.Errorf("order: %+v", units[0].Equipment)
	}
	if got := shippedLimits(t, units)[0]; got.Used != 1 {
		t.Errorf("bows used: %+v", got)
	}
}

func TestNewClassesSortAndCount(t *testing.T) {
	items := []Equipment{
		{Name: "Scroll", Class: EquipConsumable, Size: "small"}, {Name: "Blowpipe", Class: EquipMissile}, {Name: "Barding", Class: EquipArmour},
		{Name: "Heavy Armour", Class: EquipArmour}, {Name: "Lance", Class: EquipLance}, {Name: "Sword", Class: EquipMelee}, {Name: "Horse", Class: EquipMount},
		{Name: "Shield", Class: EquipShield}, {Name: "Bow", Class: EquipBow},
	}
	var names []string
	for _, e := range SortEquipment(items) {
		names = append(names, e.Name)
	}
	// the mount, the armour of the steed, melee weapons with the lance, shield, the rider's armour, ranged, the rest
	want := []string{"Horse", "Barding", "Lance", "Sword", "Shield", "Heavy Armour", "Blowpipe", "Bow", "Scroll"}
	if !slices.Equal(names, want) {
		t.Errorf("got %v, want %v", names, want)
	}
	// a missile (the blowpipe) counts against no limit
	units := []CompanyUnit{{Kind: KindInfantry, Equipment: []Equipment{{Name: "Blowpipe", Class: EquipMissile}}}}
	if l := shippedLimits(t, units); l[0].Used != 0 || l[1].Used != 0 {
		t.Errorf("a missile counts against neither the bow nor the throwing limit: %+v", l)
	}
}

func TestLimitExemptionsLeaveModelsOutOfTheCountAndTheBase(t *testing.T) {
	bow := Equipment{Name: "Bow", Class: EquipBow}
	var units []CompanyUnit
	for i := 0; i < 2; i++ {
		units = append(units, CompanyUnit{Type: "Warrior of Rohan", Kind: KindInfantry})
	}
	for i := 0; i < 10; i++ {
		units = append(units, CompanyUnit{Type: "Rider of Rohan", Kind: KindCavalry, Equipment: []Equipment{bow}})
	}
	rohan := BCCompany{LimitExemptions: []LimitExemption{{Unit: "Rider of Rohan", Limits: []string{"bow", "cavalry"}}}}
	for _, l := range computeLimits(shippedRules(t).BC.limitRules, rohan, units) {
		switch l.ID {
		case "bow", "cavalry":
			if l.Used != 0 || l.Max != 1 || l.Exempt != 10 || l.Total() != 10 || l.Segments() != 11 {
				t.Errorf("%s: the riders do not count (but are tallied as exempt) and 2 warriors make a maximum of 1: %+v", l.ID, l)
			}
		case "throwing":
			if l.Max != 4 { // nobody is exempt from this one: 12 models
				t.Errorf("throwing: %+v", l)
			}
		}
	}
	// an exempt item (Sharkey's Rogues: whips) is not counted but its bearer stays in the base
	whip := Equipment{Name: "Whip", Class: EquipThrowing, Also: []EquipmentClass{EquipMelee}}
	rogues := BCCompany{LimitExemptions: []LimitExemption{{Item: "Whip", Limits: []string{"throwing"}}}}
	units = []CompanyUnit{{Equipment: []Equipment{whip}}, {Equipment: []Equipment{whip}}, {}}
	if l := computeLimits(shippedRules(t).BC.limitRules, rogues, units)[1]; l.Used != 0 || l.Exempt != 2 || l.Max != 1 {
		t.Errorf("whips are free: %+v", l)
	}
	// a company's own share for a limit (Osgiliath: bows 50%)
	osgiliath := BCCompany{LimitOverrides: map[string]float64{"bow": 50}}
	if l := computeLimits(shippedRules(t).BC.limitRules, osgiliath, make([]CompanyUnit, 6))[0]; l.Max != 3 {
		t.Errorf("50%% of 6: %+v", l)
	}
}

func TestSlots(t *testing.T) {
	u := CompanyUnit{BaseSlots: SlotCounts{Large: 1, Small: 1}, Equipment: []Equipment{{Name: "Sword"}, {Name: "Badge of Courage", Size: "small"}, {Name: "Mountain Boots", Size: "large"}}}
	if s := u.Slots(); s.LargeUsed != 1 || s.LargeMax != 1 || s.SmallUsed != 1 || s.SmallMax != 1 || s.LargeFree() || s.SmallFree() {
		t.Errorf("a model carries one large and one small item: %+v", s)
	}
	u.Equipment = []Equipment{{Name: "Backpack", Size: "large", GrantsSlots: SlotCounts{Small: 3}}, {Name: "Map", Size: "small"}}
	if s := u.Slots(); s.SmallMax != 4 || !s.SmallFree() || s.LargeFree() {
		t.Errorf("a backpack holds three more small items: %+v", s)
	}
}

func TestCanRecruit(t *testing.T) {
	d := CompanyDetail{Influence: 4, MaxUnits: 3, Roster: make([]CompanyUnit, 2)}
	if ok, _ := d.CanRecruit(3); !ok {
		t.Error("room and influence")
	}
	if ok, why := d.CanRecruit(5); ok || !strings.Contains(why, "5 influence") {
		t.Errorf("not enough influence: %v %q", ok, why)
	}
	d.Roster = make([]CompanyUnit, 3)
	if ok, why := d.CanRecruit(1); ok || !strings.Contains(why, "full") {
		t.Errorf("a full company: %v %q", ok, why)
	}
}

func TestIconFor(t *testing.T) {
	for _, c := range []struct {
		name    string
		classes []EquipmentClass
		size    string
		want    string
	}{
		{"Ring of Warding", []EquipmentClass{EquipOther}, "small", "ring"},
		{"Scroll of Courage", []EquipmentClass{EquipOther}, "small", "scroll"},
		{"Mountain Boots", []EquipmentClass{EquipOther}, "large", "boots"},
		{"Seeing Stone", []EquipmentClass{EquipOther}, "small", "orb"},
		{"Traps & Snares", []EquipmentClass{EquipOther}, "large", "snare"},
		{"Axe of Lossarnach", []EquipmentClass{EquipMelee}, "", "axe"},
		{"Fell Warg", []EquipmentClass{EquipMount}, "", "warg"},
		{"War Camel", []EquipmentClass{EquipMount}, "", "camel"},
		{"War goat", []EquipmentClass{EquipMount}, "", "goat"},
		{"Horse", []EquipmentClass{EquipMount}, "", ""},
		{"Armoured horse", []EquipmentClass{EquipMount}, "", "armoured-horse"},
		{"Galadhrim Steed", []EquipmentClass{EquipMount}, "", "steed"},
		{"Pony", []EquipmentClass{EquipMount}, "", "pony"},
		{"Hand-and-a-half hammer", []EquipmentClass{EquipMelee, EquipTwoHanded}, "", "hammer"},
		{"Elven-made Hand-and-a-half sword", []EquipmentClass{EquipMelee, EquipTwoHanded}, "", "hand-and-a-half"},
		{"Mattock", []EquipmentClass{EquipTwoHanded}, "", "hammer"},
		{"Twin Blades", []EquipmentClass{EquipMelee}, "", "twin-blades"},
		{"Razor-sharp teeth and talons (hand weapons)", []EquipmentClass{EquipMelee}, "", "claws"},
		{"Beaks and Claws (hand weapon)", []EquipmentClass{EquipMelee}, "", "claws"},
		{"Elven Glaive", []EquipmentClass{EquipSpear}, "", "glaive"},
		{"Easterling Halberd", []EquipmentClass{EquipSpear, EquipTwoHanded}, "", "halberd"},
		{"Throwing Spears", []EquipmentClass{EquipThrowing}, "", "thrown-spear"},
		{"Smoke Bombs", []EquipmentClass{EquipThrowing}, "", "smoke-bomb"},
		{"Troll Chain", []EquipmentClass{EquipThrowing}, "", "chain"},
		{"Whip", []EquipmentClass{EquipMelee, EquipThrowing}, "", "whip"},
		{"Light Armour", []EquipmentClass{EquipArmour}, "", "light-armour"},
		{"Heavy Dwarf Armour", []EquipmentClass{EquipArmour}, "", "heavy-armour"},
		{"Armour", []EquipmentClass{EquipArmour}, "", ""},
		{"Small Shield", []EquipmentClass{EquipShield}, "", "small-shield"},
		{"Pavise", []EquipmentClass{EquipShield}, "", "pavise"},
		{"Elven hand weapon", []EquipmentClass{EquipMelee}, "", ""},
		{"Sword", []EquipmentClass{EquipMelee}, "", ""},                         // drawn by its class
		{"Odd Trinket", []EquipmentClass{EquipOther}, "small", "generic-small"}, // a new item still gets an icon
		{"Strange Crate", []EquipmentClass{EquipOther}, "large", "generic-large"},
		{"Mystery", []EquipmentClass{EquipOther}, "", ""},
	} {
		if got := IconFor(c.name, c.classes, c.size); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestChoosableOptionsAreTheOnesTheArmourySells(t *testing.T) {
	rs := shippedRules(t)
	names := func(unit string) []string {
		v, ok := rs.Vanilla.Unit(unit)
		if !ok {
			t.Fatalf("no %s", unit)
		}
		var out []string
		for _, o := range rs.BC.ChoosableOptions(v.Name, v.Options) {
			out = append(out, o.Name)
		}
		return out
	}
	// the Knight of Dol Amroth: banners and war horns are not sold, the armoured horse and lance is
	if got := names("Knight of Dol Amroth"); !slices.Contains(got, "Armoured horse and lance") || slices.ContainsFunc(got, func(n string) bool {
		return strings.Contains(strings.ToLower(n), "banner") || strings.Contains(strings.ToLower(n), "horn")
	}) {
		t.Errorf("Knight of Dol Amroth: %v", got)
	}
	// the Cave Troll's hammer is read as an armoury item and its Troll Spear is an item of its own; and stay choosable
	got := names("Cave Troll")
	for _, want := range []string{"Troll Spear", "Hand-and-a-half hammer"} {
		if !slices.Contains(got, want) {
			t.Errorf("Cave Troll cannot choose %s: %v", want, got)
		}
		if _, unsold := rs.BC.Unsold("Cave Troll", want); unsold {
			t.Errorf("%s is read as an armoury item, not unsold", want)
		}
	}
	if item, ok := rs.BC.ReadAs("Cave Troll", "Troll Spear"); !ok || item != "Troll Spear" {
		t.Errorf("the Troll Spear is an item of its own: %q", item)
	}
	if item, ok := rs.BC.ReadAs("Cave Troll", "Hand-and-a-half hammer"); !ok || item != "Hand-and-a-half Weapon" {
		t.Errorf("the hammer is read as the Hand-and-a-half Weapon: %q", item)
	}
}

func TestDropReason(t *testing.T) {
	options := []ChoiceOption{{"War horn and Shield", 26}, {"Banner", 25}, {"Shield and Spear", 2}, {"Bow", 1}, {"Shield", 1}}
	armour := Equipment{Name: "Heavy Armour", Class: EquipArmour, Profile: true}
	shield := Equipment{Name: "Shield", Class: EquipShield}
	spear := Equipment{Name: "Spear", Class: EquipSpear}
	bow := Equipment{Name: "Bow", Class: EquipBow}
	badge := Equipment{Name: "Badge of Courage", Class: EquipOther, Size: "small"}
	warrior := CompanyUnit{Role: RoleWarrior, Equipment: []Equipment{armour, shield, spear, badge}}
	hero := CompanyUnit{Role: RoleHero, Equipment: []Equipment{armour, bow, badge}}
	for _, c := range []struct {
		name  string
		unit  CompanyUnit
		item  Equipment
		optal bool
		ok    bool
	}{
		{"warrior drops the spear: a shield is an option", warrior, spear, false, true},
		{"warrior drops the shield: a spear alone is not", warrior, shield, false, false},
		{"warrior never drops its profile's armour", warrior, armour, false, false},
		{"warrior drops equipment freely", warrior, badge, false, true},
		{"warrior with just a bow drops it: back to the plain profile", CompanyUnit{Role: RoleWarrior, Equipment: []Equipment{armour, bow}}, bow, false, true},
		{"hero drops bought wargear", hero, bow, false, true},
		{"hero keeps its profile's armour", hero, armour, false, false},
		{"hero drops its profile's armour with the optional rule", hero, armour, true, true},
		{"hero drops equipment", hero, badge, false, true},
	} {
		got := DropReason(c.unit, c.item, options, c.optal)
		if (got == "") != c.ok {
			t.Errorf("%s: reason %q, want allowed=%v", c.name, got, c.ok)
		}
	}
}

func TestRatingFor(t *testing.T) {
	r := Rating{Warrior: 1, Hero: 5}
	if r.For(false) != 1 || r.For(true) != 5 {
		t.Error("a warrior and a hero have their own rating")
	}
}

func TestProgressionRulesAreReadFromTheFile(t *testing.T) {
	book := shippedRules(t).BC
	r := book.Progression()
	if r.Threshold != 5 || len(r.Warrior) != 3 || len(r.Paths) < 10 || !r.LowerIsBetter["courage"] {
		t.Fatalf("progression: %+v", r.Threshold)
	}
	if w, _ := r.WarriorFor(4); w.Key != "promoted" {
		t.Errorf("a 4 promotes: %+v", w)
	}
	// a Tactician at the limit of Attack cannot take "Attacks or Wounds" on Attack but can on Wounds
	sheet := HeroSheet{Path: "Path of the Tactician", Stats: map[string]SheetStat{"attack": {Value: 2, Boost: 1}, "wounds": {Value: 1}}, Rules: map[string]bool{}, Actions: map[string]bool{}}
	offer, ok := r.OfferFor(sheet, LevelLists{}, 4)
	if !ok || len(offer.Choices) != 1 || !strings.HasPrefix(offer.Choices[0].Key, "stat:wounds") {
		t.Errorf("only Wounds can rise: %+v", offer)
	}
	// Courage improves downwards and stops at the path's best
	sheet.Stats["courage"] = SheetStat{Value: 7}
	sheet.Stats["intelligence"] = SheetStat{Value: 7}
	offer, ok = r.OfferFor(sheet, LevelLists{}, 5)
	if !ok || len(offer.Choices) != 2 || !offer.Choices[0].RollAgain {
		t.Errorf("a 5 improves Courage or Intelligence and rolls again: %+v", offer)
	}
	if offer.Choices[0].Key != "stat:courage:1" {
		t.Errorf("Courage 7+ becomes 6+: %q", offer.Choices[0].Key)
	}
	var p Progress
	if err := r.Apply(&p, offer.Choices[0].Key); err != nil || p.Bonus["courage"] != -1 {
		t.Errorf("applying it: %+v %v", p.Bonus, err)
	}
	if err := r.Apply(&p, "stat:attack:-1"); err != nil || p.Bonus["attack"] != 1 {
		t.Errorf("a normal stat goes up: %+v", p.Bonus)
	}
	// a rule the hero already has is not offered again
	sheet.Rules["spy"] = true
	if _, ok := r.OfferFor(sheet, LevelLists{}, 2); ok {
		t.Error("Spy is already owned")
	}
}

func TestAHeroMustTakeTheOnlyResultItCanTake(t *testing.T) {
	book := shippedRules(t).BC
	r := book.Progression()
	sheet := HeroSheet{Path: "Path of the Tactician", Stats: map[string]SheetStat{"attack": {Value: 3, Boost: 2}, "wounds": {Value: 2, Boost: 1}}, Rules: map[string]bool{}, Actions: map[string]bool{}}
	// a 4 (Attack or Wounds) cannot be taken at the maximum; a 2 (Spy) can: the 2 stands and the 4 is not rerolled
	roll := r.RollHero(sheet, LevelLists{}, []int{4, 2}, &FixedDice{Rolls: []int{1}})
	if len(roll.Offers) != 1 || roll.Offers[0].Roll != 2 {
		t.Fatalf("only the 2 is offered: %+v", roll.Offers)
	}
	if len(roll.Pairs) != 2 || roll.Pairs[0].Usable || roll.Pairs[0].Sum != 4 || roll.Pairs[0].Text == "" || !roll.Pairs[1].Usable {
		t.Errorf("the result that cannot be taken is kept, with what it was: %+v", roll.Pairs)
	}
	// neither can be taken: both are rolled again until one can
	calls := 0
	roll = r.RollHero(sheet, LevelLists{}, []int{4, 4}, countingDice{&calls}) // 1+1 = 2: Spy
	if len(roll.Offers) == 0 || calls == 0 {
		t.Errorf("unavailable results are rolled again: %+v", roll)
	}
}

func TestSorcererRowsOfferPowersAndCastingImprovements(t *testing.T) {
	book := shippedRules(t).BC
	r := book.Progression()
	if len(r.Powers) != 31 || r.CastFloor != 2 || r.CastMax != 2 {
		t.Fatalf("the spell list: %d powers, floor %d, max %d", len(r.Powers), r.CastFloor, r.CastMax)
	}
	sheet := HeroSheet{Path: "Path of the Sorcerer", Stats: map[string]SheetStat{"will": {Value: 1}}, Rules: map[string]bool{}, Actions: map[string]bool{},
		Powers: map[string]int{"black dart": 0, "renew": 2}, Casting: map[string]int{"black dart": 6, "renew": 3}, PowerNames: map[string]string{"black dart": "Black Dart", "renew": "Renew"}}
	offer, ok := r.OfferFor(sheet, LevelLists{}, 3)
	if !ok {
		t.Fatal("row 3 offers something")
	}
	powers, casts := 0, []string{}
	for _, c := range offer.Choices {
		switch {
		case strings.HasPrefix(c.Key, "power:"):
			powers++
			if c.Key == "power:Black Dart" || c.Key == "power:Renew" {
				t.Errorf("a known power is not offered again: %s", c.Key)
			}
		case strings.HasPrefix(c.Key, "cast:"):
			casts = append(casts, c.Key)
		}
	}
	if powers != 35 || len(casts) != 1 || casts[0] != "cast:black dart" {
		t.Errorf("35 new powers (Fury once per species); only Black Dart can improve (Renew is at its 2 improvements): %d %v", powers, casts)
	}
}

func TestCampaignGlossaryHasEverythingTheRulesFilesDefine(t *testing.T) {
	read := func(p string) []byte {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Skip("no rules file")
		}
		return b
	}
	vanilla, bc, gl := read("../../definitions/vanilla/data2024.json"), read("../../definitions/battle_companies/battle_companies.json"), read("../../definitions/glossary/mesbg_glossary.json")
	base, err := BuildGlossary(RulesetInfo{}, gl)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadRuleSet(vanilla, bc, gl, nil)
	if err != nil {
		t.Fatal(err)
	}
	g := loaded.Glossary
	if len(g.Terms) < len(base.Terms)+300 {
		t.Errorf("the campaign glossary adds the rules files' terms: %d vs %d", len(g.Terms), len(base.Terms))
	}
	for name, category := range map[string]string{"Spy": "special rules", "Black Dart": "magic", "Baggage Pony": "creatures", "Arm Wound": "injuries", "Heavy Armour": "wargear", "Wandering Swordsman": "wanderers", "Path of the Sorcerer": "paths", "Heroic Channeling": "heroic actions"} {
		term, ok := g.Find(name)
		if !ok {
			t.Errorf("%s is missing", name)
		} else if term.Category != category {
			t.Errorf("%s is in %q, want %q", name, term.Category, category)
		}
	}
	for _, name := range []string{"Path of the Tactician", "Path of the Sharpshooter", "Path of the Soldier"} { // paths without a description of their own
		if p, ok := g.Find(name); !ok || p.Path == nil || len(p.Path.Rows) == 0 {
			t.Errorf("%s has a glossary entry with its chart", name)
		}
	}
	if path, ok := g.Find("Path of the Duelist"); !ok || path.Path == nil || len(path.Path.Rows) != 11 || path.Path.Rows[0].Roll != 2 || path.Path.Rows[10].Roll != 12 || len(path.Path.Limits) < 5 {
		t.Errorf("a path term holds its limits and its roll table as data: %+v", path.Path)
	} else if links := func() (n int) {
		for _, r := range path.Path.Rows {
			for _, seg := range r.Segments {
				if seg.TermID != "" {
					n++
				}
			}
		}
		return
	}(); links < 5 {
		t.Errorf("the results of the chart link to the rules they name: %d links", links)
	}
	seen := map[string]bool{}
	for _, term := range g.Terms {
		if seen[strings.ToLower(term.Name)] {
			t.Errorf("%s appears twice", term.Name)
		}
		seen[strings.ToLower(term.Name)] = true
	}
}

func TestBattleCompaniesDefinitionsWinInTheCampaignGlossary(t *testing.T) {
	gl := []byte(`{"terms":[{"name":"Heavy Armour","category":"wargear","definition":"Rulebook text."},{"name":"Fear","category":"special rules","definition":"Glossary fear."}]}`)
	extras := []ExtraTerm{
		{Name: "Heavy Armour", Category: "wargear", Definition: "Battle Companies text.", Source: "Battle Companies"},
		{Name: "Fear", Category: "special rules", Definition: "Vanilla fear.", Source: "Vanilla"},
		{Name: "Spy", Category: "special rules", Definition: "Vanilla spy.", Source: "Vanilla"},
		{Name: "Spy", Category: "path rules", Definition: "Battle Companies spy.", Source: "Battle Companies"},
	}
	g, err := BuildGlossaryWith(RulesetInfo{}, gl, extras)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"Heavy Armour": "Battle Companies text.", "Fear": "Glossary fear."}
	for name, def := range want {
		term, ok := g.Find(name)
		if !ok || term.Summary(100) != def {
			t.Errorf("%s: %+v", name, term.Summary(100))
		}
	}
	if len(g.Terms) != 3 {
		t.Errorf("no duplicates: %d terms", len(g.Terms))
	}
}

func TestCompanyProfilesHaveEverythingForTheBrowser(t *testing.T) {
	read := func(p string) []byte {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Skip("no rules file")
		}
		return b
	}
	book := shippedRules(t).BC
	vanilla, _ := ParseVanilla(read("../../definitions/vanilla/data2024.json"))
	profiles := book.Profiles()
	if len(profiles) != 37 {
		t.Fatalf("every company: %d", len(profiles))
	}
	rohan, _ := book.Profile("Rohan")
	rohan = rohan.WithPreviews(vanilla)
	if rohan.Description == "" || len(rohan.Start) == 0 || rohan.StartGold == 0 || len(rohan.Advancements) == 0 || len(rohan.HeroUpgrades) == 0 {
		t.Errorf("Rohan has its description, starting company, promotions and hero upgrade: %+v", rohan.Start)
	}
	if rohan.HeroUpgrades[0].Text == "" {
		t.Error("the hero upgrade has its rule text")
	}
	pv := rohan.Preview("Warrior of Rohan", []string{"shield"})
	if pv.ID == "" || pv.Points == 0 || len(pv.Stats) == 0 || !slices.Contains(pv.Wargear, "Shield") {
		t.Errorf("the unit named in a chart has a card: %+v", pv)
	}
	if rohan.Preview("Rohan Royal Guard", nil).ID == "" {
		t.Error("a unit of the special chart has a card")
	}
}

func TestLimitMaxRoundsUpAndCountsTheNewModel(t *testing.T) {
	for _, c := range []struct {
		percent float64
		n, want int
	}{{33.33, 6, 2}, {33.33, 7, 3}, {33.33, 9, 3}, {33.33, 10, 4}, {33, 6, 2}, {33, 9, 3}, {33, 12, 4}, {33.33, 0, 0}, {50, 5, 3}} {
		if got := LimitMax(c.percent, c.n, true); got != c.want {
			t.Errorf("LimitMax(%v, %d) = %d, want %d", c.percent, c.n, got, c.want)
		}
	}
	bow := Equipment{Name: "Bow", Class: EquipBow}
	units := []CompanyUnit{{Equipment: []Equipment{bow}}, {Equipment: []Equipment{bow}}, {}, {}, {}, {}} // 6 units, 2 bows
	l := shippedLimits(t, units)[0]
	if !l.Full() || !l.RoomForNew() || l.Max != 2 || l.Next != 3 {
		t.Errorf("6 units with 2 bows: the 7th may be a bow unit (limit 3): %+v", l)
	}
	units = append(units, CompanyUnit{Equipment: []Equipment{bow}}) // 7 units, 3 bows
	if l := shippedLimits(t, units)[0]; l.RoomForNew() || l.Max != 3 {
		t.Errorf("7 units with 3 bows: the 8th may not (limit 3 of 8 rounds up to 3): %+v", l)
	}
}

// Every sentence of a company's "Only this company has" and limits sections is printed once: a rule that a substitution,
// exchange, free special rule or limit card already shows is not repeated as a "Company rule" card.
func TestCompanyPageNeverRepeatsASentence(t *testing.T) {
	book := shippedRules(t).BC
	key := func(t string) string { return strings.ToLower(strings.Join(strings.Fields(t), " ")) }
	for _, p := range book.Profiles() {
		var shown []string
		for _, r := range p.OwnRules() {
			shown = append(shown, r)
		}
		for _, g := range p.Granted {
			shown = append(shown, g.Text)
		}
		for _, e := range p.Exchanges {
			shown = append(shown, e.Text)
		}
		shown = append(shown, p.Substitutions...)
		shown = append(shown, p.LimitLines()...)
		for i, a := range shown {
			for _, b := range shown[i+1:] {
				if ka, kb := key(a), key(b); ka != "" && (ka == kb || strings.Contains(ka, kb) || strings.Contains(kb, ka)) {
					t.Errorf("%s prints a sentence twice: %q / %q", p.Name, a, b)
				}
			}
		}
	}
	fief, _ := book.Profile("Fiefdoms")
	if got := fief.OwnRules(); len(got) != 0 || len(fief.Substitutions) != 1 {
		t.Errorf("Fiefdoms shows its Pride of Dol Amroth sentence once, as the substitution: rules %v, substitutions %v", got, fief.Substitutions)
	}
}

// The Last Alliance lists the Rivendell Warrior (the document's 2018 "High Elf Warrior"): no card for a unit with its own name.
func TestLastAllianceHasNoUnitWithItsOwnName(t *testing.T) {
	book := shippedRules(t).BC
	p, ok := book.Profile("The Last Alliance")
	if !ok {
		t.Fatal("no Last Alliance")
	}
	if len(p.Aliases) != 0 {
		t.Errorf("aliases: %+v", p.Aliases)
	}
}

func TestCompanyEpigraphOnlyArnor(t *testing.T) {
	book := shippedRules(t).BC
	for _, p := range book.Profiles() {
		if p.Name == "Arnor" {
			if p.Epigraph == nil || p.Epigraph.Quote != "Leave all that can be spared behind. We travel light." || !strings.HasPrefix(p.Epigraph.Text, "As Arnor fell") {
				t.Errorf("Arnor epigraph = %+v", p.Epigraph)
			}
		} else if p.Epigraph != nil {
			t.Errorf("%s has an epigraph: %+v", p.Name, p.Epigraph)
		}
	}
}

// countingDice rolls ones and counts the dice it rolled.
type countingDice struct{ n *int }

func (c countingDice) D6() int             { return c.Die(6) }
func (c countingDice) Die(int) int         { *c.n++; return 1 }
func (c countingDice) Roll(s DiceSpec) int { return sumDice(c, s) }
