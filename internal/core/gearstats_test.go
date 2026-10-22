package core

import (
	"slices"
	"strings"
	"testing"
)

func defenceOf(u CompanyUnit) int {
	for _, a := range u.Stats {
		if a.Name == "Defence" {
			return a.Value
		}
	}
	return -1
}

func buildFor(t *testing.T, company string, s UnitState) CompanyUnit {
	t.Helper()
	rs := shippedRules(t)
	bcCo, _ := rs.BC.Company(company)
	return buildUnit(rs, bcCo, s, CampaignSettings{})
}

// A profile that lists a shield already counts it; the gear only changes Defence relative to the profile.
func TestShieldAndArmourChangeDefenceRelativeToTheProfile(t *testing.T) {
	plain := buildFor(t, "Rohan", UnitState{ID: 1, Base: "Warrior of Rohan", Role: RoleWarrior})
	shielded := buildFor(t, "Rohan", UnitState{ID: 1, Base: "Warrior of Rohan", Role: RoleWarrior, Gear: []string{"Shield"}})
	if defenceOf(shielded) != defenceOf(plain)+1 {
		t.Errorf("a shield adds 1 Defence: %d -> %d (%v)", defenceOf(plain), defenceOf(shielded), shielded.StatMods)
	}
	bow := buildFor(t, "Rohan", UnitState{ID: 1, Base: "Warrior of Rohan", Role: RoleWarrior, Gear: []string{"Shield", "Bow"}})
	if defenceOf(bow) != defenceOf(plain) {
		t.Errorf("a bow cancels the shield: %d vs %d (%v)", defenceOf(bow), defenceOf(plain), bow.StatMods)
	}
	hero := buildFor(t, "Rohan", UnitState{ID: 1, Base: "Warrior of Rohan", Role: RoleHero, Bonus: HeroBonus(nil), Gear: []string{"Shield"}, Bought: []string{"Two-handed weapon"}})
	if hero.Points <= 0 || defenceOf(hero) != defenceOf(plain)+0 {
		t.Errorf("a two-handed weapon takes the shield's bonus: %d vs %d (%v)", defenceOf(hero), defenceOf(plain), hero.StatMods)
	}
}

func TestExpertRiderKeepsTheShieldWithItsBow(t *testing.T) {
	// the Rider of Rohan's profile lists Shield and Bow and its Defence counts the shield (Expert Rider lifts the bow's cancellation)
	rider := buildFor(t, "Rohan", UnitState{ID: 1, Base: "Rider of Rohan", Role: RoleWarrior})
	noBow := buildFor(t, "Rohan", UnitState{ID: 1, Base: "Rider of Rohan", Role: RoleHero, Bonus: HeroBonus(nil), Dropped: []string{"Bow"}})
	if defenceOf(noBow) != defenceOf(rider) {
		t.Errorf("dropping the bow changes nothing for an Expert Rider: %d vs %d (%v)", defenceOf(noBow), defenceOf(rider), noBow.StatMods)
	}
	noShield := buildFor(t, "Rohan", UnitState{ID: 1, Base: "Rider of Rohan", Role: RoleHero, Bonus: HeroBonus(nil), Dropped: []string{"Shield"}})
	if defenceOf(noShield) != defenceOf(rider)-1 {
		t.Errorf("dropping the shield costs 1: %d vs %d", defenceOf(noShield), defenceOf(rider))
	}
	twoHanded := buildFor(t, "Rohan", UnitState{ID: 1, Base: "Rider of Rohan", Role: RoleHero, Bonus: HeroBonus(nil), Bought: []string{"Two-handed weapon"}})
	if defenceOf(twoHanded) != defenceOf(rider)-1 {
		t.Errorf("a two-handed weapon cancels the shield even for an Expert Rider: %d vs %d (%v)", defenceOf(twoHanded), defenceOf(rider), twoHanded.StatMods)
	}
}

func TestArmourUpgradeChangesDefenceByOneStage(t *testing.T) {
	armoured := buildFor(t, "Rohan", UnitState{ID: 1, Base: "Warrior of Rohan", Role: RoleHero, Bonus: HeroBonus(nil), Dropped: []string{"Armour"}})
	heavy := buildFor(t, "Rohan", UnitState{ID: 1, Base: "Warrior of Rohan", Role: RoleHero, Bonus: HeroBonus(nil), Dropped: []string{"Armour"}, Bought: []string{"Heavy Armour"}})
	plain := buildFor(t, "Rohan", UnitState{ID: 1, Base: "Warrior of Rohan", Role: RoleHero, Bonus: HeroBonus(nil)})
	t.Log(armourNames(plain), defenceOf(plain), defenceOf(armoured), defenceOf(heavy))
	if defenceOf(heavy)-defenceOf(armoured) != 2 {
		t.Errorf("heavy armour is +2 over none: %d vs %d", defenceOf(heavy), defenceOf(armoured))
	}
}

// Across every company: the gear's changes listed in StatMods are exactly what was added to the characteristics.
func TestGearStatModsAddUpInEveryCompany(t *testing.T) {
	rs := shippedRules(t)
	units := 0
	for _, p := range rs.BC.Profiles() {
		bcCo, _ := rs.BC.Company(p.Name)
		var specs []UnitState
		for _, s := range p.Start {
			specs = append(specs, UnitState{Base: s.Unit, Role: RoleWarrior, Gear: s.Wargear})
		}
		for _, row := range append(append([]ChartRow{}, p.Reinforcements...), p.SpecialChart...) {
			specs = append(specs, UnitState{Base: row.Result.Unit, Role: RoleWarrior, Gear: row.Result.Wargear})
		}
		for _, s := range specs {
			u := buildUnit(rs, bcCo, s, CampaignSettings{})
			sum := map[string]int{}
			for _, m := range u.StatMods {
				sum[m.Stat] += m.Delta
			}
			for _, a := range u.Stats {
				key := ""
				for _, d := range rs.Vanilla.Stats() {
					if d.Name == a.Name {
						key = d.Key
					}
				}
				if a.Gear != sum[key] {
					t.Errorf("%s / %s: %s gear %d but the mods add up to %d", p.Name, s.Base, a.Name, a.Gear, sum[key])
				}
			}
			units++
		}
	}
	if units == 0 {
		t.Fatal("no units")
	}
}

func TestUnitFillInsFillOnlyWhatVanillaLeavesEmpty(t *testing.T) {
	rs := shippedRules(t)
	v, ok := rs.Vanilla.Unit("Hunter Orc Warrior")
	if !ok || len(v.Race) == 0 || len(v.UnitTypes) == 0 {
		t.Errorf("the Hunter Orc Warrior gets its race and unit types: %+v", v)
	}
	if k, _ := rs.Vanilla.Unit("Knight of Dol Amroth"); !hasKeyword(k, "Man") {
		t.Errorf("a unit vanilla defines is untouched: %+v", k)
	}
}

func TestBattlinBrandybuckHasStrength3(t *testing.T) {
	strength := func(u CompanyUnit) int {
		for _, a := range u.Stats {
			if a.Name == "Strength" {
				return a.Value
			}
		}
		return -1
	}
	rs := shippedRules(t)
	p, _ := rs.BC.Profile("The Shire")
	var label string
	for _, a := range p.Aliases {
		if a.Label == "Battlin' Brandybuck" {
			label = a.Label
		}
	}
	if label == "" {
		t.Skip("no Battlin' Brandybuck in the Shire")
	}
	bb := buildFor(t, "The Shire", UnitState{ID: 1, Base: label, Role: RoleWarrior})
	militia := buildFor(t, "The Shire", UnitState{ID: 1, Base: "Hobbit Militia", Role: RoleWarrior})
	if strength(bb) != strength(militia)+1 || strength(bb) != 3 {
		t.Errorf("Battlin' Brandybuck has Strength 3: %d vs militia %d (%v)", strength(bb), strength(militia), bb.StatMods)
	}
}

func TestRuleKeyAndLabel(t *testing.T) {
	rs := shippedRules(t)
	if got := rs.BC.RuleLabel("Keen Sight (Falcon)"); got != "Keen Sight" {
		t.Errorf("label of the Falcon's rule: %q", got)
	}
	if got := rs.BC.RuleLabel("Hatred (Orc)"); got != "Hatred (Orc)" {
		t.Errorf("a rule without a label is shown by its name: %q", got)
	}
	if (UnitRule{Name: "Keen Sight (Falcon)", Label: "Keen Sight"}).Shown() != "Keen Sight" || (UnitRule{Name: "X"}).Shown() != "X" {
		t.Error("Shown")
	}
	for _, e := range GlossaryExtras(readDef(t, "vanilla/data2024.json"), readDef(t, "battle_companies/battle_companies.json")) {
		if e.Name == "Keen Sight (Falcon)" {
			t.Error("the glossary shows the label, never the key")
		}
	}
}

// The Small Shield's +1 holds against Strikes only: the profile's Defence does not change, the card says so.
func TestSmallShieldIsANoteNotADefenceChange(t *testing.T) {
	plain := buildFor(t, "Rohan", UnitState{ID: 1, Base: "Warrior of Rohan", Role: RoleWarrior})
	small := buildFor(t, "Rohan", UnitState{ID: 1, Base: "Warrior of Rohan", Role: RoleHero, Bonus: HeroBonus(nil), Bought: []string{"Small Shield"}})
	if defenceOf(small) != defenceOf(plain) {
		t.Fatalf("Defence %d vs %d", defenceOf(small), defenceOf(plain))
	}
	var note string
	for _, m := range small.StatMods {
		if m.From == "Small Shield" {
			note = m.Note
		}
	}
	if !strings.Contains(note, "against strikes") {
		t.Errorf("the card notes it: %+v", small.StatMods)
	}
}

// R8: a conditional bonus (the Pavise's +3, not while Engaged) is text on the item, never added to the unit's Defence.
func TestPaviseIsANoteNotADefenceChange(t *testing.T) {
	plain := buildFor(t, "Rohan", UnitState{ID: 1, Base: "Warrior of Rohan", Role: RoleWarrior})
	pavise := buildFor(t, "Rohan", UnitState{ID: 1, Base: "Warrior of Rohan", Role: RoleHero, Bonus: HeroBonus(nil), Bought: []string{"Pavise"}})
	// the Warrior of Rohan's profile carries a Shield; the Pavise is a second shield and the best active one counts, so only the note differs
	if defenceOf(pavise) != defenceOf(plain) {
		t.Fatalf("Defence %d vs %d: a conditional +3 is not counted (%v)", defenceOf(pavise), defenceOf(plain), pavise.StatMods)
	}
	var note string
	for _, m := range pavise.StatMods {
		if m.From == "Pavise" {
			if m.Delta != 0 {
				t.Errorf("no change is added: %+v", m)
			}
			note = m.Note
		}
	}
	if !strings.Contains(note, "+3 Defence") || !strings.Contains(note, "not while Engaged") {
		t.Errorf("the card says what the bonus is: %+v", pavise.StatMods)
	}
}

// Top-6: with Phalanx a pike and a shield carry no penalty; a bow brings the shield's penalty back (because of the bow, not the pike).
func TestPhalanxPikeAndShieldKeepTheShieldButABowTakesItAway(t *testing.T) {
	rs := shippedRules(t)
	bcCo, _ := rs.BC.Company("Rohan")
	eq := func(names ...string) []Equipment {
		var out []Equipment
		for _, n := range names {
			e, _ := equipmentFor(rs, bcCo, n, true, false, true)
			out = append(out, e)
		}
		return out
	}
	shield := func(items []Equipment, rules ...string) gearMod {
		has := func(r string) bool { return slices.Contains(rules, r) }
		for _, m := range gearContributions(rs, items, has, false) {
			if m.From == "Shield" {
				return m
			}
		}
		t.Fatal("no shield contribution")
		return gearMod{}
	}
	if m := shield(eq("Shield", "Pike"), "Phalanx"); m.Delta != 1 || m.Note != "" {
		t.Errorf("Phalanx: pike and shield keep +1: %+v", m)
	}
	if m := shield(eq("Shield", "Pike")); m.Delta != 0 || !strings.Contains(m.Note, "Pike") {
		t.Errorf("without Phalanx the pike cancels the shield: %+v", m)
	}
	if m := shield(eq("Shield", "Pike", "Bow"), "Phalanx"); m.Delta != 0 || !strings.Contains(m.Note, "Bow") {
		t.Errorf("Phalanx does not lift the bow's cancellation: %+v", m)
	}
}

// The Elven Glaive's +1 Attacks (Engaged, not Supported) is text on the card, not a stat change.
func TestSituationalAttacksBonusIsATextNote(t *testing.T) {
	rs := shippedRules(t)
	a, ok := rs.BC.Annotation("Elven Glaive")
	if !ok || len(a.Situational) != 1 || !strings.Contains(a.Situational[0].Condition, "Engaged") {
		t.Fatalf("the glaive's situational bonus: %+v", a.Situational)
	}
	bcCo, _ := rs.BC.Company("Rohan")
	e, _ := equipmentFor(rs, bcCo, "Elven Glaive", true, false, true)
	var note string
	for _, m := range gearContributions(rs, []Equipment{e}, func(string) bool { return false }, false) {
		if m.From == "Elven Glaive" {
			if m.Delta != 0 {
				t.Errorf("never a stat change: %+v", m)
			}
			note = m.Note
		}
	}
	if !strings.Contains(note, "+1 attacks") || !strings.Contains(note, "not Supported") {
		t.Errorf("note %q", note)
	}
}
