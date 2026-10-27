package core

import (
	"slices"
	"strings"
)

// Poison (owner decision 2026-10-27): the special rule Poisoned Attacks (<weapon>) belongs to a UNIT, never to the wargear. It
// comes from the unit's profile, from the Blowpipe (an item that grants it itself), from a level-up (Poisoned Attacks|weapon:<name>)
// and from Envenom Weapon (data.gearAnnotations grantsRuleForChosenWeapon): the player chooses one weapon of the unit and the unit
// gains the rule for it. A weapon already named by such a rule cannot be chosen again.

// ChosenWeapon says whether the name is a weapon modifier with its weapon chosen ("Envenom Weapon (Bow)" for the annotation
// "Envenom Weapon (X)"): the name of the modifier as the armoury knows it and the weapon. "Envenom Weapon (X)" itself has no weapon yet.
func (b *BCBook) ChosenWeapon(name string) (base, weapon string, ok bool) {
	name = strings.TrimSpace(name)
	for _, a := range b.weaponRules {
		prefix, found := strings.CutSuffix(a.Name, "X)")
		if !found || !strings.HasSuffix(name, ")") || len(name) <= len(prefix)+1 || !strings.EqualFold(name[:len(prefix)], prefix) {
			continue
		}
		if w := strings.TrimSpace(name[len(prefix) : len(name)-1]); w != "" && w != "X" {
			return a.Name, w, true
		}
	}
	return "", "", false
}

// ChosenWeaponName is the stored name of a weapon modifier with its weapon chosen: "Envenom Weapon (X)" and "Bow" make
// "Envenom Weapon (Bow)".
func ChosenWeaponName(modifier, weapon string) string {
	return strings.TrimSuffix(modifier, "X)") + weapon + ")"
}

// weaponRuleOf is the annotation's chosen-weapon rule for the Poisoned Attacks rule, if the data has one.
func (b *BCBook) weaponRuleOf(rule string) (ChosenWeaponRule, bool) {
	for _, a := range b.weaponRules {
		if strings.EqualFold(a.ChosenWeapon.Rule, rule) {
			return *a.ChosenWeapon, true
		}
	}
	return ChosenWeaponRule{}, false
}

// PoisonRule is the name of the rule Envenom Weapon and the level-up give a chosen weapon.
const PoisonRule = "Poisoned Attacks"

// CanBePoisoned says whether the item is a weapon the unit can be given Poisoned Attacks for: its class is among the weaponClasses
// of the annotation of Envenom Weapon, and it is no package that came with the unit.
func (b *BCBook) CanBePoisoned(e Equipment) bool {
	cw, ok := b.weaponRuleOf(PoisonRule)
	if !ok || e.Upgrade {
		return false
	}
	return slices.ContainsFunc(e.Classes(), func(c EquipmentClass) bool { return slices.Contains(cw.WeaponClasses, c) })
}

// poisonedWeapons are the weapons named by the Poisoned Attacks (<weapon>) rules among the lists, by item identity.
func poisonedWeapons(rules ...[]UnitRule) []string {
	var out []string
	for _, list := range rules {
		for _, r := range list {
			if w, ok := strings.CutPrefix(r.Name, PoisonRule+" ("); ok {
				out = append(out, strings.TrimSuffix(w, ")"))
			}
		}
	}
	return out
}

// isPoisoned says whether a weapon is named by one of the poisoned weapons.
func isPoisoned(weapon string, poisoned []string) bool {
	return slices.ContainsFunc(poisoned, func(p string) bool { return sameSpelling(p, weapon) })
}

// applyWeaponModifiers resolves the weapon modifiers a unit carries (Envenom Weapon): the weapons it could be given to, the one chosen
// when it is still carried and not poisoned by something else, and the rule `Poisoned Attacks (<weapon>)` the unit gains for it. The
// rule is added to the grants once (the profile's own rule, the Blowpipe's and a level-up's are the same rule and show once). It
// fills the unit's PoisonableWeapons last.
func applyWeaponModifiers(rs *RuleSet, book *VanillaBook, u *CompanyUnit, grants []UnitRule) []UnitRule {
	var pathRules []UnitRule
	if u.Path != nil {
		pathRules = u.Path.Rules
	}
	poisoned := poisonedWeapons(u.Rules, pathRules, grants)
	var weapons []string // the carried weapons the rule could be given to, by name once
	for _, e := range u.Equipment {
		if rs.BC.CanBePoisoned(e) && !isPoisoned(e.Name, poisoned) && !slices.ContainsFunc(weapons, func(w string) bool { return sameSpelling(w, e.Name) }) {
			weapons = append(weapons, e.Name)
		}
	}
	for i := range u.Equipment {
		e := &u.Equipment[i]
		a, ok := rs.BC.Annotation(e.Name)
		if !ok || a.ChosenWeapon == nil {
			continue
		}
		e.Choices = slices.Clone(weapons)
		_, weapon, chosen := rs.BC.ChosenWeapon(e.Name)
		j := slices.IndexFunc(weapons, func(w string) bool { return sameSpelling(w, weapon) })
		if !chosen || j < 0 {
			continue
		}
		e.Modifies = weapons[j]
		rule := a.ChosenWeapon.Rule + " (" + e.Modifies + ")"
		desc, ok := book.Keyword(rule)
		if !ok {
			desc, _ = rs.BC.Keyword(rule)
		}
		grants = append(grants, UnitRule{Name: rule, Description: desc, Source: RuleFromEquipment, From: e.Name})
		weapons = slices.Delete(weapons, j, j+1)
	}
	u.PoisonableWeapons = weapons
	return grants
}
