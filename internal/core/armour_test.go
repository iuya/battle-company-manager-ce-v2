package core

import (
	"slices"
	"testing"
)

func armourShop(t *testing.T, company string) (shop, CompanyState) {
	t.Helper()
	rs := shippedRules(t)
	p, _ := rs.BC.Profile(company)
	st := CompanyState{Company: CampaignCompany{ID: 1, Name: company, Faction: company}, Influence: 20, Gold: 20, Units: StartingUnits(p, p.DefaultHeroes())}
	for i := range st.Units {
		st.Units[i].ID = int64(i + 1)
	}
	return newShop(rs, CampaignSettings{}, st), st
}

func upgradeOffer(t *testing.T, sh shop, stage int) GearOffer {
	t.Helper()
	for _, o := range sh.rs.BC.ArmourUpgrade().offers() {
		if o.ArmourStage == stage {
			return o
		}
	}
	t.Fatalf("no armour upgrade to stage %d", stage)
	return GearOffer{}
}

func armourNames(u CompanyUnit) []string {
	var out []string
	for _, e := range u.Equipment {
		if slices.Contains(e.Classes(), EquipArmour) {
			out = append(out, e.Name)
		}
	}
	return out
}

func TestArmourUpgradeReplacesTheArmourWithTheNextStage(t *testing.T) {
	sh, st := armourShop(t, "Rohan") // the sergeants wear Armour; the company fields Heavy Armour units, so a hero may buy it
	hero, _ := sh.unit(2)
	heavy := upgradeOffer(t, sh, 2)
	if why, _ := sh.whyNot(hero, heavy); why != "" {
		t.Fatalf("a Rohan hero in Armour may upgrade to Heavy Armour: %s", why)
	}
	if why, _ := sh.whyNot(hero, upgradeOffer(t, sh, 1)); why == "" {
		t.Error("it already wears Armour")
	}
	us, _ := stateUnit(&st, 2)
	before := hero.Points
	upgradeArmour(sh, us, hero, heavy)
	got, _ := newShop(sh.rs, CampaignSettings{}, st).unit(2)
	if names := armourNames(got); len(names) != 1 || !slices.Contains(append([]string{heavy.Name}, heavy.AlsoCalled...), names[0]) {
		t.Errorf("the armour is replaced by the next stage, not added: %v", names)
	}
	if got.Points != before+1 {
		t.Errorf("an upgrade adds 1 rating to a hero whose Attacks + Wounds are below 3: %d -> %d", before, got.Points)
	}
}

func TestArmourUpgradeStagesAreOneAtATimeAndLimitedToTheCompany(t *testing.T) {
	sh, _ := armourShop(t, "The Shire") // an unarmoured Hobbit hero; no unit of the company has Heavy Armour
	hero, _ := sh.unit(2)
	if sh.rs.BC.ArmourUpgrade().UnitStage(hero) != 0 {
		t.Fatalf("the hero is unarmoured: %v", armourNames(hero))
	}
	if why, _ := sh.whyNot(hero, upgradeOffer(t, sh, 1)); why != "" {
		t.Errorf("a hero may buy Armour (the Ranger of the North has it): %s", why)
	}
	if why, _ := sh.whyNot(hero, upgradeOffer(t, sh, 2)); why == "" {
		t.Error("Heavy Armour is not one stage up from no armour")
	}
	warrior := CompanyUnit{ID: 99, Type: "Hobbit Militia", Role: RoleWarrior}
	if why, blocker := sh.whyNot(warrior, upgradeOffer(t, sh, 1)); why == "" || blocker != BlockedByRule {
		t.Errorf("a warrior needs an Armour option in its profile: %q", why)
	}
}

// Whatever stage a unit is at, the offers never let it skip one or go down, for every company.
func TestArmourUpgradesAreOfferedOneStageAtATimeInEveryCompany(t *testing.T) {
	rs := shippedRules(t)
	up := rs.BC.ArmourUpgrade()
	if len(up.Stages) != 3 || up.Cost != 1 {
		t.Fatalf("armour upgrade rule not loaded: %+v", up)
	}
	n := 0
	for _, p := range rs.BC.Profiles() {
		sh, _ := armourShop(t, p.Name)
		for _, u := range sh.d.Roster {
			for _, o := range up.offers() {
				if why, _ := sh.whyNot(u, o); why == "" {
					n++
					if up.UnitStage(u) != o.ArmourStage-1 {
						t.Errorf("%s: %s at stage %d is offered stage %d", p.Name, u.Type, up.UnitStage(u), o.ArmourStage)
					}
				}
			}
		}
	}
	if n == 0 {
		t.Error("no company offers any armour upgrade")
	}
}

// A hero may buy any wargear of a member of its company, start units included (the Shire's Ranger of the North has a spear).
func TestHeroPoolIncludesStartUnits(t *testing.T) {
	sh, _ := armourShop(t, "The Shire")
	hero, _ := sh.unit(2)
	if hero.Type != "Hobbit Militia" {
		t.Fatalf("expected a Hobbit Militia sergeant, got %s", hero.Type)
	}
	spear, ok := sh.rs.BC.Offer(sh.bcC, "Spear")
	if !ok {
		t.Fatal("no Spear in the armoury")
	}
	if why, blocker := sh.whyNot(hero, spear); blocker == BlockedByRule {
		t.Errorf("a Shire hero may buy the Ranger of the North's Spear: %s", why)
	}
}
