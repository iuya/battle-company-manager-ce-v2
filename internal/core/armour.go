package core

import (
	"fmt"
	"slices"
	"strings"
)

// ArmourUpgrade is the Battle Companies rule "Upgrade Armour by one stage" (data.wargear.armourUpgrade): stages of armour, each
// upgrade costs influence (or Gold) and adds rating. Buying one replaces the armour the model wears with the next stage.
type ArmourUpgrade struct {
	Stages [][]string // the names of each stage, lowest first (None and Light, Armour, Heavy and Dwarf armour)
	Cost   int        // influence per upgrade
	Rating Rating     // rating per upgrade
	Gold   Rating     // Gold per upgrade
}

// ArmourUpgrade is the armour upgrade rule of the file.
func (b *BCBook) ArmourUpgrade() ArmourUpgrade { return b.armourUp }

// StageOf is the stage an armour is at: 0 for no armour or Light Armour, up to the last stage. ok is false for a name that is no
// armour (a "Heavy Dwarf Armour" is read as a heavy armour).
func (a ArmourUpgrade) StageOf(name string) (stage int, ok bool) {
	name = strings.TrimSpace(name)
	for i, names := range a.Stages {
		if slices.ContainsFunc(names, func(n string) bool { return sameName(n, name) }) {
			return i, true
		}
	}
	if l := strings.ToLower(name); strings.Contains(l, "heavy") && strings.Contains(l, "armour") && len(a.Stages) > 0 {
		return len(a.Stages) - 1, true
	}
	return 0, false
}

// UnitStage is the armour stage a unit wears now (0 when it wears none).
func (a ArmourUpgrade) UnitStage(u CompanyUnit) int {
	best := 0
	for _, e := range u.Equipment {
		if slices.Contains(e.Classes(), EquipArmour) {
			if s, ok := a.StageOf(e.Name); ok {
				best = max(best, s)
			}
		}
	}
	return best
}

// offers are the armour upgrades as armoury items, one per stage above the first. An upgrade is bought like any item and is
// told apart by ArmourStage.
func (a ArmourUpgrade) offers() []GearOffer {
	var out []GearOffer
	for i := 1; i < len(a.Stages); i++ {
		if len(a.Stages[i]) == 0 {
			continue
		}
		out = append(out, GearOffer{Name: a.Stages[i][0], AlsoCalled: a.Stages[i][1:], Classes: []EquipmentClass{EquipArmour}, Cost: a.Cost,
			Rating: a.Rating, Gold: a.Gold, ArmourStage: i, Description: fmt.Sprintf("Upgrades the armour by one stage, to %s; the armour worn now is replaced.", a.Stages[i][0])})
	}
	return out
}

// offers is what the armoury sells this company: its items and the armour upgrades.
func (sh shop) offers() []GearOffer {
	return append(slices.Clone(sh.rs.BC.Armoury(sh.bcC)), sh.rs.BC.ArmourUpgrade().offers()...)
}

// armourNames are the names of the armour of a stage that the company's units can have (its pool), empty if none can.
func (sh shop) armourPool(u CompanyUnit) map[int][]string {
	key := u.Type + "|" + string(u.Role)
	if sh.memo != nil {
		if pool, ok := sh.memo.pool[key]; ok {
			return pool
		}
	}
	up := sh.rs.BC.ArmourUpgrade()
	pool := map[int][]string{}
	options, _ := sh.optionsOf(u)
	for _, o := range options {
		if ParseOption(o.Name).Exchange {
			continue
		}
		for _, part := range OptionItems(o.Name) {
			if s, ok := up.StageOf(part); ok && s > 0 && !slices.ContainsFunc(pool[s], func(n string) bool { return sameName(n, part) }) {
				pool[s] = append(pool[s], part)
			}
		}
	}
	if sh.memo != nil {
		sh.memo.pool[key] = pool
	}
	return pool
}

// armourTarget is the armour the unit gets from the upgrade offer (a Dwarf takes Dwarf Armour where its company's units have it).
func (sh shop) armourTarget(u CompanyUnit, o GearOffer) string {
	names := append([]string{o.Name}, o.AlsoCalled...)
	pool := sh.armourPool(u)[o.ArmourStage]
	for _, n := range names {
		if slices.ContainsFunc(pool, func(p string) bool { return sameName(p, n) }) {
			return n
		}
	}
	return o.Name
}

// armourReason says why the unit cannot take the armour upgrade now (empty when it can). The unit must wear the stage below the
// upgrade, and the stage must be one the unit may have: a warrior needs it as an option of its profile; a hero only the stages that units of its company can have, so a company whose units never wear heavy armour
// never sells it to a hero.
func (sh shop) armourReason(u CompanyUnit, o GearOffer) (string, Blocker) {
	up := sh.rs.BC.ArmourUpgrade()
	switch now := up.UnitStage(u); {
	case now >= o.ArmourStage:
		return "Already wears this armour or better", BlockedByRule
	case now < o.ArmourStage-1:
		return "Armour is upgraded one stage at a time", BlockedByRule
	}
	if len(sh.armourPool(u)[o.ArmourStage]) == 0 {
		if u.IsHero() {
			return "No unit of the company can have this armour", BlockedByRule
		}
		return "Not an option of its profile", BlockedByRule
	}
	if inf, gold := canPay(sh.d, o.Cost, o.Gold.For(u.IsHero())); !inf && !gold {
		return costReason(sh.d, o.Cost, o.Gold.For(u.IsHero())), BlockedByInfluence
	}
	return "", ""
}

// upgradeArmour records the purchase of an armour upgrade on the unit: the armour worn is dropped (replaced, not kept) and the
// next stage bought.
func upgradeArmour(sh shop, st *UnitState, u CompanyUnit, o GearOffer) {
	for _, e := range u.Equipment {
		if slices.Contains(e.Classes(), EquipArmour) {
			if s, ok := sh.rs.BC.ArmourUpgrade().StageOf(e.Name); ok && s > 0 {
				st.Dropped = append(st.Dropped, e.Name)
			}
		}
	}
	st.Bought = append(st.Bought, sh.armourTarget(u, o))
}
