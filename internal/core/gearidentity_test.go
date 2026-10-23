package core

import (
	"slices"
	"strings"
	"testing"
)

func TestItemKeyIgnoresSpellingNotIdentity(t *testing.T) {
	same := [][2]string{{"Elven-made Glaive", "Elven Glaive"}, {"Warspear", "War spear"}, {"Throwing Spears", "throwing spear"}, {"Uruk-Hai Bow", "uruk-hai bow"}}
	for _, p := range same {
		if ItemKey(p[0]) != ItemKey(p[1]) {
			t.Errorf("%q and %q are one item", p[0], p[1])
		}
	}
	distinct := [][2]string{{"Pike", "Spear"}, {"Elf Bow", "Longbow"}, {"Crossbow", "Bow"}, {"Light Shield", "Shield"}, {"War spear", "Spear"}, {"Elven pike", "Pike"}}
	for _, p := range distinct {
		if ItemKey(p[0]) == ItemKey(p[1]) {
			t.Errorf("%q and %q are different items", p[0], p[1])
		}
	}
}

func TestOptionItems(t *testing.T) {
	cases := map[string][]string{
		"Shield and Spear":                    {"Shield", "Spear"},
		"Shield & Throwing Spears":            {"Shield", "Throwing Spears"},
		"Horse, Elf bow":                      {"Horse", "Elf bow"},
		"War horn, shield,":                   {"War horn", "shield"},
		"Exchange lance for banner":           {"lance", "banner"},
		"Replace pike with banner":            {"pike", "banner"},
		"Shield (without bow)":                {"Shield"},
		"Swap shield for war drum (Isengard)": {"shield", "war drum"},
	}
	for in, want := range cases {
		if got := OptionItems(in); !slices.Equal(got, want) {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestGearMatchesOptionByIdentity(t *testing.T) {
	spear := GearOffer{Name: "Spear", Classes: []EquipmentClass{EquipSpear}}
	pike := GearOffer{Name: "Pike", Classes: []EquipmentClass{EquipSpear}}
	shield := GearOffer{Name: "Shield", Classes: []EquipmentClass{EquipShield}}
	light := GearOffer{Name: "Light Shield", Classes: []EquipmentClass{EquipShield}}
	bow := GearOffer{Name: "Bow", Classes: []EquipmentClass{EquipBow}}
	longbow := GearOffer{Name: "Longbow", Classes: []EquipmentClass{EquipBow}}
	elf := GearOffer{Name: "Elf Bow", Classes: []EquipmentClass{EquipBow}}
	cross := GearOffer{Name: "Crossbow", Classes: []EquipmentClass{EquipCrossbow}}
	war := GearOffer{Name: "War spear", AlsoCalled: []string{"Warspear"}, Classes: []EquipmentClass{EquipSpear, EquipLance}}
	yes := func(o GearOffer, opt string) {
		t.Helper()
		if !GearMatchesOption(o, opt) {
			t.Errorf("%s is an item of %q", o.Name, opt)
		}
	}
	no := func(o GearOffer, opt string) {
		t.Helper()
		if GearMatchesOption(o, opt) {
			t.Errorf("%s is not an item of %q", o.Name, opt)
		}
	}
	yes(spear, "Shield and Spear")
	yes(shield, "Shield and Spear")
	no(bow, "Shield and Spear")
	no(pike, "Shield and Spear")
	no(light, "Shield and Spear")
	no(spear, "Pike")
	yes(pike, "Pike")
	yes(bow, "Bow")
	no(longbow, "Bow")
	no(elf, "Bow")
	no(cross, "Bow")
	no(bow, "Elf bow")
	no(longbow, "Elf bow")
	yes(elf, "Horse, Elf bow")
	no(spear, "Throwing spears")
	no(spear, "War spear")
	no(spear, "Exchange war spear for mattock")
	yes(war, "Exchange war spear for mattock")
	yes(war, "War spear")
	yes(light, "Light Shield and Spear")
	no(shield, "Light Shield")
	yes(spear, "Exchange spear for crossbow")
	yes(cross, "Exchange spear for crossbow")
	no(bow, "Exchange spear for crossbow")
}

// Every item of the armoury table is its own offer (a price row is not one item), with its own text.
func TestEveryWargearNameIsItsOwnOffer(t *testing.T) {
	rs := shippedRules(t)
	co, _ := rs.BC.Company("Minas Tirith")
	offers := map[string]GearOffer{}
	for _, o := range rs.BC.Armoury(co) {
		offers[o.Name] = o
	}
	for _, n := range []string{"Pike", "Spear", "Lance", "War spear", "Shield", "Light Shield", "Elf Bow", "Longbow", "Esgaroth Bow", "Dwarf Longbow", "Crossbow", "Bow"} {
		o, ok := offers[n]
		if !ok {
			t.Errorf("%s is not offered on its own", n)
			continue
		}
		if len(o.AlsoCalled) > 1 || (len(o.AlsoCalled) == 1 && ItemKey(o.AlsoCalled[0]) != ItemKey("Warspear") && ItemKey(o.AlsoCalled[0]) != ItemKey("Throwing Spear")) {
			t.Errorf("%s is called %v: aliases are spellings, not row-mates", n, o.AlsoCalled)
		}
		a, annotated := rs.BC.Annotation(n)
		if !annotated || o.Description != a.Definition {
			t.Errorf("%s: the offer carries the item's own text", n)
		}
	}
	if !strings.Contains(offers["Pike"].Description, "second") && !strings.Contains(offers["Pike"].Description, "another friendly model armed with a pike") {
		t.Errorf("the Pike text mentions supporting through a second pike: %q", offers["Pike"].Description)
	}
	if !strings.Contains(offers["Crossbow"].Description, "may not fire in the same turn") || !strings.Contains(offers["Crossbow"].Stats, "S4") {
		t.Errorf("crossbow: %q %q", offers["Crossbow"].Description, offers["Crossbow"].Stats)
	}
	if !strings.Contains(offers["Longbow"].Description, "half its Move Value") {
		t.Errorf("a bow may shoot after moving up to half its Move: %q", offers["Longbow"].Description)
	}
}

// What a Warrior of Minas Tirith may buy: only items that are exactly an option of its profile.
func TestMinasTirithWarriorsBuyOnlyTheirOptions(t *testing.T) {
	rs := shippedRules(t)
	co, ok := rs.BC.Company("Minas Tirith")
	if !ok {
		t.Fatal("no Minas Tirith")
	}
	u, ok := rs.Vanilla.Unit("Warrior of Minas Tirith")
	if !ok {
		t.Fatal("no Warrior of Minas Tirith")
	}
	var can []string
	for _, o := range rs.BC.Armoury(co) {
		if o.Size != "" {
			continue
		}
		if slices.ContainsFunc(u.Options, func(opt ChoiceOption) bool { return GearMatchesOption(o, opt.Name) }) {
			can = append(can, o.Name)
		}
	}
	for _, bad := range []string{"Pike", "Elf Bow", "Esgaroth Bow", "Longbow", "Crossbow", "Light Shield", "Lance"} {
		if slices.Contains(can, bad) {
			t.Errorf("a Warrior of Minas Tirith cannot buy %s (offered: %v)", bad, can)
		}
	}
	for _, good := range []string{"Spear", "Shield", "Bow"} {
		if !slices.Contains(can, good) {
			t.Errorf("a Warrior of Minas Tirith can buy %s (offered: %v)", good, can)
		}
	}
	t.Logf("Warrior of Minas Tirith options %v can buy %v", u.Options, can)
}

// A gear chip looks its name up in the glossary: each item finds its own entry, never that of a row-mate of the price table.
func TestGearTooltipsAreTheItemsOwn(t *testing.T) {
	rs := shippedRules(t)
	want := map[string]string{ // what the chip finds -> the glossary term it must be
		"Spear": "Spear", "Pike": "Pike", "Lance": "Lance", "War spear": "War Spear", "Shield": "Shields", "Light Shield": "Light Shield",
		"Bow": "Bow", "Crossbow": "Crossbow", "Longbow": "Longbow", "Elf Bow": "Elf Bow", "Esgaroth Bow": "Esgaroth Bow", "Dwarf Longbow": "Dwarf Longbow",
	}
	for chip, term := range want {
		got, ok := rs.Glossary.Find(chip)
		if !ok || got.Name != term {
			t.Errorf("%s: the tooltip is %q (found %v), want the term %q", chip, got.Name, ok, term)
		}
	}
	text := func(chip string) string { g, _ := rs.Glossary.Find(chip); return g.Text() }
	if strings.Contains(text("Spear"), "second") || !strings.Contains(text("Pike"), "another friendly model armed with a pike") {
		t.Error("spear and pike keep their own texts")
	}
	if !strings.Contains(text("Crossbow"), "cannot make a Shooting Attack with it in the same turn") && !strings.Contains(text("Crossbow"), "may not fire") {
		t.Errorf("crossbow: %q", text("Crossbow"))
	}
	for _, bow := range []string{"Longbow", "Elf Bow", "Esgaroth Bow", "Dwarf Longbow"} {
		if !strings.Contains(text(bow), "half its Move Value") {
			t.Errorf("%s: a bow may shoot after moving up to half its Move: %q", bow, text(bow))
		}
	}
	if !strings.Contains(text("Elf Bow"), "Strength") || strings.Contains(text("Longbow"), "Crossbow") {
		t.Error("each bow carries its own profile")
	}
}

// Items no file describes still have a tooltip: natural weapons are hand weapons, Twin Blades is a vanilla special rule, and
// a mount's chip shows its profile.
func TestGearWithoutOwnTextFindsATooltip(t *testing.T) {
	rs := shippedRules(t)
	co, _ := rs.BC.Company("Harad")
	for _, name := range []string{"Hand weapon", "Hand weapons", "Axe", "Sword", "Fangs (hand weapons)", "Claws and teeth (hand weapon)"} {
		if e, _ := equipmentFor(rs, co, name, false, false, false); !strings.Contains(e.Description, "single hand") {
			t.Errorf("%s: tooltip %q should be the Hand Weapon entry", name, e.Description)
		}
	}
	if e, _ := equipmentFor(rs, co, "Twin Blades", false, false, false); !strings.Contains(e.Description, "Attacks to 2") {
		t.Errorf("Twin Blades: %q", e.Description)
	}
	for _, name := range []string{"Elven cloak", "Elven Cloak"} { // profile gear the armoury does not sell: the glossary's wargear entry
		if e, _ := equipmentFor(rs, co, name, false, false, false); !strings.Contains(e.Description, "Stalk Unseen") || !strings.Contains(e.Description, "To Hit") {
			t.Errorf("%s: tooltip %q should be the Elven Cloak entry", name, e.Description)
		}
	}
	for _, name := range []string{"Horse", "Warg", "Fell Warg", "Armoured horse", "Goat"} {
		if tip := gearTip(rs, co, name); tip.Text == "" {
			t.Errorf("%s has no tooltip (mount profile expected)", name)
		}
	}
}
