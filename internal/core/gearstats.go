package core

import (
	"fmt"
	"slices"
	"strings"
)

// StatMod is one change the gear makes to a characteristic of a unit, relative to what its vanilla profile already includes: a
// shield adds +1 Defence unless something cancels it, armour its stages, a unit upgrade its modifier (Helmingas: Strength +1).
type StatMod struct {
	Stat  string // the stat key (defence...)
	Delta int
	From  string // the item
	Note  string // why it does not apply or only partly (cancelled by a bow; used two-handed in that Combat)
}

// gearMod is what one item contributes, and the note when it does not apply.
type gearMod StatMod

// gearContributions are the characteristic changes of a set of items. A profile lists its shield and armour already counted in its
// Defence, so callers compare the contributions of the unit's gear with those of its profile's wargear.
func gearContributions(rs *RuleSet, items []Equipment, hasRule func(string) bool, mounted bool) []gearMod {
	var out []gearMod
	// armour: only the best one counts
	var bestArmour *gearMod
	for _, e := range items {
		if e.Disabled != "" {
			continue
		}
		a, ok := rs.BC.Annotation(e.Name)
		if !ok {
			continue
		}
		if slices.Contains(a.Classes, EquipArmour) {
			for _, c := range a.StatChanges {
				if c.Condition != "" { // a conditional bonus is text on the item
					out = append(out, gearMod{Stat: c.Stat, From: e.Name, Note: fmt.Sprintf("%+d %s %s (not counted)", c.Delta, c.Stat, c.Condition)})
					continue
				}
				if bestArmour == nil || c.Delta > bestArmour.Delta {
					bestArmour = &gearMod{Stat: c.Stat, Delta: c.Delta, From: e.Name}
				}
			}
			continue
		}
	}
	if bestArmour != nil {
		out = append(out, *bestArmour)
	}
	// shields do not stack: the best active one counts
	var bestShield *gearMod
	for _, e := range items {
		if e.Disabled != "" {
			continue
		}
		a, ok := rs.BC.Annotation(e.Name)
		if !ok || a.Defence == nil {
			continue
		}
		d := a.Defence
		if d.Conditional && d.Against == "" && !d.NotWhileEngaged { // situational, worded by the data: item text, not a change of the Defence characteristic
			out = append(out, gearMod{Stat: "defence", From: e.Name, Note: fmt.Sprintf("%+d Defence: %s", d.Value, d.Condition)})
			continue
		}
		if d.Against != "" { // +1 against Strikes only: a note on the card, not a change of the Defence characteristic
			out = append(out, gearMod{Stat: "defence", From: e.Name, Note: fmt.Sprintf("%+d Defence against %s only", d.Value, d.Against)})
			continue
		}
		m := gearMod{Stat: "defence", Delta: d.Value, From: e.Name}
		if cause := shieldCancelledBy(d, e, items, hasRule, mounted); cause != "" {
			m.Delta, m.Note = 0, "cancelled by "+cause
		} else if d.NotWhileEngaged { // the bonus holds only when not Engaged: item text, not part of the Defence characteristic
			out = append(out, gearMod{Stat: "defence", From: e.Name, Note: fmt.Sprintf("%+d Defence, but not while Engaged in Combat (apply it yourself)", d.Value)})
			continue
		} else if d.TwoHandedUse {
			for _, o := range items {
				if o.Disabled == "" && slices.Contains(o.Classes(), EquipTwoHanded) && slices.Contains(o.Classes(), EquipMelee) {
					m.Note = "not while " + o.Name + " is used two-handed"
				}
			}
		}
		if bestShield == nil || m.Delta > bestShield.Delta {
			bestShield = &m
		}
	}
	if bestShield != nil {
		out = append(out, *bestShield)
	}
	// situational bonuses of other items (the Elven Glaive's Attacks): text on the card
	for _, e := range items {
		if a, ok := rs.BC.Annotation(e.Name); ok && e.Disabled == "" {
			for _, c := range a.Situational {
				out = append(out, gearMod{Stat: c.Stat, From: e.Name, Note: fmt.Sprintf("%+d %s: %s", c.Delta, c.Stat, c.Condition)})
			}
		}
	}
	// unit upgrades and other items with a plain modifier (vanilla modifiesStats)
	for _, e := range items {
		if e.Disabled != "" || e.Size != "" {
			continue
		}
		if a, ok := rs.BC.Annotation(e.Name); ok && (a.Defence != nil || slices.Contains(a.Classes, EquipArmour) || slices.Contains(a.Classes, EquipShield)) {
			continue
		}
		g, ok := rs.Vanilla.Gear(e.Name)
		if !ok || strings.EqualFold(g.Type, "armour") || strings.EqualFold(g.Type, "shield") || ParseOption(e.Name).Exchange {
			continue
		}
		for _, c := range g.Modifies {
			if c.Condition != "" {
				out = append(out, gearMod{Stat: c.Stat, From: e.Name, Note: fmt.Sprintf("%+d %s %s (not counted)", c.Delta, c.Stat, c.Condition)})
				continue
			}
			out = append(out, gearMod{Stat: c.Stat, Delta: c.Delta, From: e.Name})
		}
	}
	return out
}

// shieldCancelledBy names what cancels the shield's bonus (a bow, a two-handed weapon, a pike), unless a rule of the unit lifts that
// cancellation. A hand-and-a-half weapon is two-handed only when used so in a Combat, which the player decides offline.
func shieldCancelledBy(d *DefenceBonus, shield Equipment, items []Equipment, hasRule func(string) bool, mounted bool) string {
	lifted := func(class, name string) bool {
		for _, l := range d.Lifted {
			if hasRule(l.Rule) && (!l.WhileMounted || mounted) &&
				slices.ContainsFunc(l.Cancels, func(c string) bool { return strings.EqualFold(c, class) || (name != "" && strings.EqualFold(c, name)) }) {
				return true
			}
		}
		return false
	}
	for _, o := range items {
		if o.Disabled != "" || sameName(o.Name, shield.Name) {
			continue
		}
		if slices.ContainsFunc(d.CancelledByItems, func(n string) bool { return sameName(n, o.Name) }) && !lifted("", o.Name) {
			return o.Name
		}
		for _, c := range d.CancelledByClasses {
			if !slices.Contains(o.Classes(), c) {
				continue
			}
			if c == EquipTwoHanded && slices.Contains(o.Classes(), EquipMelee) { // hand-and-a-half
				continue
			}
			if !lifted(string(c), "") {
				return o.Name
			}
		}
	}
	return ""
}

// applyGearStats adds to the unit's characteristics what its gear changes relative to the gear its vanilla profile comes with
// (the profile already counts that: a profile that lists a shield has the shield's +1), and lists the changes in StatMods.
func applyGearStats(rs *RuleSet, bcCo BCCompany, base VanillaUnit, u *CompanyUnit, hero bool) {
	hasRule := func(name string) bool {
		return slices.ContainsFunc(u.Rules, func(r UnitRule) bool { return sameName(r.Name, name) }) ||
			(u.Path != nil && slices.ContainsFunc(u.Path.Rules, func(r UnitRule) bool { return sameName(r.Name, name) }))
	}
	mounted := u.isCavalry()
	var own []Equipment
	for _, w := range base.Wargear {
		e, _ := equipmentFor(rs, bcCo, w, false, true, hero)
		own = append(own, e)
	}
	cur := gearContributions(rs, u.Equipment, hasRule, mounted)
	was := gearContributions(rs, own, hasRule, mounted)
	type key struct{ stat, from string }
	net := map[key]*StatMod{}
	var order []key
	add := func(m gearMod, sign int) {
		k := key{m.Stat, m.From}
		if net[k] == nil {
			net[k] = &StatMod{Stat: m.Stat, From: m.From}
			order = append(order, k)
		}
		net[k].Delta += sign * m.Delta
		if sign > 0 && m.Note != "" {
			net[k].Note = m.Note
		}
	}
	for _, m := range cur {
		add(m, 1)
	}
	for _, m := range was {
		add(m, -1)
	}
	total := map[string]int{}
	for _, k := range order {
		if m := net[k]; m.Delta != 0 || m.Note != "" {
			u.StatMods = append(u.StatMods, *m)
			total[k.stat] += m.Delta
		}
	}
	for _, def := range rs.Vanilla.Stats() {
		d := total[def.Key]
		if d == 0 {
			continue
		}
		for i := range u.Stats {
			if u.Stats[i].Name == def.Name {
				u.Stats[i].Value += d
				u.Stats[i].Gear = d
			}
		}
	}
}
