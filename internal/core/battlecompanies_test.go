package core

import (
	"testing"
)

func TestBattleCompaniesBook(t *testing.T) {
	b := shippedRules(t).BC
	mt, ok := b.Company("Minas Tirith")
	if !ok || mt.ReinforcementCost != 3 || mt.MaxModels != 15 || len(mt.Reinforcements) == 0 || len(mt.SpecialChart) == 0 {
		t.Errorf("Minas Tirith: %+v", mt)
	}
	if r, ok := Roll(mt.Reinforcements, 4); !ok || r.Type != "unit" || r.Unit != "Warrior of Minas Tirith" {
		t.Errorf("a 4 on the chart: %+v %v", r, ok)
	}
	if r, _ := Roll(mt.Reinforcements, 1); r.Type != "nothing" {
		t.Errorf("a 1: %+v", r)
	}
	if r, _ := Roll(mt.Reinforcements, 6); r.Type != "specialChart" {
		t.Errorf("a 6 goes to the special chart: %+v", r)
	}
	moria, _ := b.Company("Moria")
	if len(moria.SpecialUnits) != 1 || moria.SpecialUnits[0].Unit != "Cave Troll" || moria.SpecialUnits[0].Cost != 15 {
		t.Errorf("Moria's special unit: %+v", moria.SpecialUnits)
	}
	if some, _ := b.Company("Lothlorien"); some.MaxModels != 12 {
		t.Errorf("Lothlorien's limit: %d", some.MaxModels)
	}
	var sizes = map[string]int{}
	for _, o := range b.Armoury(mt) {
		sizes[o.Size]++
		if o.Cost == 0 || len(o.Classes) == 0 {
			t.Errorf("an offer without cost or classes: %+v", o)
		}
	}
	if sizes["small"] == 0 || sizes["large"] == 0 || sizes[""] == 0 {
		t.Errorf("the armoury sells wargear and small and large equipment: %v", sizes)
	}
	badge, ok := b.Offer(mt, "badge of courage")
	if !ok || badge.Size != "small" || len(badge.Grants) != 1 || badge.Grants[0] != "Fearless" {
		t.Errorf("Badge of Courage: %+v", badge)
	}
	if _, ok := b.Offer(mt, "Dwarf Bow"); !ok {
		t.Error("an item is found by its other names too")
	}
}

func TestGearAnnotationsFromTheFile(t *testing.T) {
	b := shippedRules(t).BC
	classes := func(name string) []EquipmentClass {
		a, ok := b.Annotation(name)
		if !ok {
			t.Skipf("%s is not annotated (the data is changing)", name)
		}
		return a.Classes
	}
	if got := classes("Lance"); len(got) != 1 || got[0] != EquipLance {
		t.Errorf("Lance: %v", got)
	}
	if got := classes("War spear"); len(got) != 2 || got[0] != EquipSpear || got[1] != EquipLance {
		t.Errorf("War spear: %v", got)
	}
	if got := classes("Warspear"); len(got) != 2 {
		t.Errorf("an alias finds the same annotation: %v", got)
	}
	if got := classes("Blowpipe"); len(got) != 1 || got[0] != EquipMissile {
		t.Errorf("Blowpipe: %v", got)
	}
	if got := classes("Healing Herbs"); got[0] != EquipConsumable {
		t.Errorf("Healing Herbs: %v", got)
	}
	if got := classes("Axe of Lossarnach"); len(got) != 2 || got[0] != EquipSpear || got[1] != EquipTwoHanded {
		t.Errorf("Axe of Lossarnach: %v", got)
	}
	if a, _ := b.Annotation("Pony"); a.GrantedTo != "mount" || len(a.Grants) != 1 || a.Grants[0] != "Timid" {
		t.Errorf("Pony: %+v", a)
	}
	if a, _ := b.Annotation("Barding"); a.GrantedTo != "" || len(a.Grants) != 2 {
		t.Errorf("barding's rules go to the rider: %+v", a)
	}
	if a, _ := b.Annotation("Envenom Weapon (X)"); !a.WeaponModifier {
		t.Errorf("Envenom Weapon is a modifier: %+v", a)
	}
	if a, _ := b.Annotation("Bow"); a.Stats() != `24" · S2` {
		t.Errorf("Bow: %q", a.Stats())
	}
	if a, ok := b.Annotation("Helmingas"); ok && !IsUnitUpgrade(a, "") {
		t.Errorf("Helmingas is a unit upgrade: %+v", a)
	}
	rohan, _ := b.Company("Rohan")
	if len(rohan.LimitExemptions) == 0 || rohan.LimitExemptions[0].Unit != "Rider of Rohan" {
		t.Errorf("Rohan's riders are exempt: %+v", rohan.LimitExemptions)
	}
	osg, _ := b.Company("Osgiliath & Ithilien")
	if osg.LimitOverrides["bow"] != 50 {
		t.Errorf("Osgiliath's bow share: %+v", osg.LimitOverrides)
	}
	// an offer takes its classes from the annotations, not from its name
	for _, o := range b.Armoury(rohan) {
		if o.Name == "Blowpipe" && (len(o.Classes) != 1 || o.Classes[0] != EquipMissile) {
			t.Errorf("the armoury's blowpipe: %+v", o.Classes)
		}
	}
}

func TestOptionsReservedToANamedHeroAreNotOffered(t *testing.T) {
	rs := shippedRules(t)
	b, v := rs.BC, rs.Vanilla // the reserved options come from the vanilla file, not from the Battle Companies file
	_ = v
	if !b.Reserved("Rivendell Warrior", "King's Guard") || b.Reserved("Rivendell Warrior", "Shield") {
		t.Error("King's Guard is reserved to Gil-galad's list")
	}
	for _, o := range choiceFor(rs, "Rivendell Warrior").Options {
		if o.Name == "King's Guard" {
			t.Errorf("offered a reserved option: %+v", o)
		}
	}
	if len(choiceFor(rs, "Rivendell Warrior").Options) == 0 {
		t.Error("the other options stay")
	}
}
