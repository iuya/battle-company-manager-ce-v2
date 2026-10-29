package core

import (
	"slices"
	"sort"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// What the lean format does not store about gear, derived from the vanilla file and from the
// items themselves. TestLeanGear checks each helper.

// ClassesOfVanillaType is the class an item has when nobody annotated it, from the type word of the vanilla file (or of the Battle
// Companies wargear row): a "bow" is a bow, a "throwingWeapon" a throwing weapon, an "upgrade" a unit upgrade ("other"). A word that
// does not say the class alone (Weapon: melee? spear? two-handed?) gives none, and the item must name its classes.
func ClassesOfVanillaType(typ string) []EquipmentClass {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "mount":
		return []EquipmentClass{EquipMount}
	case "bow":
		return []EquipmentClass{EquipBow}
	case "armour":
		return []EquipmentClass{EquipArmour}
	case "shield":
		return []EquipmentClass{EquipShield}
	case "banner":
		return []EquipmentClass{EquipBanner}
	case "throwingweapon":
		return []EquipmentClass{EquipThrowing}
	case "blowpipe":
		return []EquipmentClass{EquipMissile}
	case "horn":
		return []EquipmentClass{EquipHorn}
	case "spear":
		return []EquipmentClass{EquipSpear}
	case "two-handed weapon":
		return []EquipmentClass{EquipTwoHanded}
	case "upgrade", "stat":
		return []EquipmentClass{EquipOther}
	}
	return nil
}

// OptionNotSold says whether a vanilla option is one the armoury never sells because the Battle Companies equipment replaces it: its
// name contains one of the words, ignoring case ("Banner", "Exchange lance for war horn": the Company Standard, Horn and Drum are sold
// instead).
func OptionNotSold(option string, words []string) bool {
	low := strings.ToLower(option)
	return slices.ContainsFunc(words, func(w string) bool { return w != "" && strings.Contains(low, strings.ToLower(w)) })
}

// ReservedOption is a vanilla option that belongs to the army list of named heroes only.
type ReservedOption struct {
	Unit, Option string
	AvailableIn  []string
}

// ReservedOptions lists the options of the vanilla units whose availableIn names only heroes (the King's Guard of Gil-galad's list,
// the Lorien Guard of Galadriel's and Celeborn's): a Battle Company has no such army list, so a "choice of options" never offers them.
// Sorted by unit, then option; an option that names an army or a scenario as well is not listed.
func (b *VanillaBook) ReservedOptions() []ReservedOption {
	var out []ReservedOption
	for name, u := range b.units {
		for _, o := range u.Options {
			av := b.available[reservedKey(name, o.Name)]
			if !allHeroes(av, b.heroes) {
				continue
			}
			if !slices.ContainsFunc(out, func(r ReservedOption) bool { return r.Unit == name && r.Option == o.Name }) {
				out = append(out, ReservedOption{Unit: name, Option: o.Name, AvailableIn: av})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Unit != out[j].Unit {
			return out[i].Unit < out[j].Unit
		}
		return out[i].Option < out[j].Option
	})
	return out
}

func allHeroes(names []string, heroes map[string]bool) bool {
	for _, n := range names {
		if !heroes[n] {
			return false
		}
	}
	return len(names) > 0
}

// ProfileWargear is a piece of wargear the vanilla profiles of some units carry that no armoury sells.
type ProfileWargear struct {
	Item  string
	Units []string
}

// ProfileWargearNotSold lists, for the vanilla units a company uses, the wargear of their profiles that no armoury item names: a model
// of the unit always starts with it (it is added on recruit) but it can never be bought and is not an option to choose. sold says
// whether some armoury item is that item (by ItemKey). Sorted by item, ignoring case; the units of an item in alphabetical order.
func (b *VanillaBook) ProfileWargearNotSold(units []string, sold func(item string) bool) []ProfileWargear {
	var out []ProfileWargear
	at := map[string]int{}
	for _, name := range slices.Compact(slices.Sorted(slices.Values(units))) {
		u, ok := b.units[name]
		if !ok {
			continue
		}
		for _, w := range u.Wargear {
			if sold(w) {
				continue
			}
			i, seen := at[w]
			if !seen {
				i = len(out)
				at[w] = i
				out = append(out, ProfileWargear{Item: w})
			}
			out[i].Units = append(out[i].Units, name)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Item) < strings.ToLower(out[j].Item) })
	return out
}

// fillIns are the fields Battle Companies gives a vanilla unit or mount that the vanilla file leaves empty.
type fillIns struct {
	units  map[string]lean.VanillaFill
	mounts map[string]lean.VanillaFill
}

// applyFillIns sets, on the vanilla book, the race, faction and unit type of the units and mounts where vanilla leaves them empty. A
// field vanilla does define is never changed. The base size of a unit fill-in is not applied: nothing reads it today.
func applyFillIns(v *VanillaBook, f fillIns) {
	for name, fill := range f.units {
		u, ok := v.units[name]
		if !ok {
			continue
		}
		if len(u.Race) == 0 {
			u.Race = slices.Clone(fill.Race)
		}
		if len(u.Faction) == 0 {
			u.Faction = slices.Clone(fill.Faction)
		}
		if len(u.UnitTypes) == 0 {
			u.UnitTypes = slices.Clone(fill.UnitType)
		}
		v.units[name] = u
	}
	for name, fill := range f.mounts { // the same for the keywords of a mount
		g, ok := v.gear[NameKey(name)]
		if !ok {
			continue
		}
		if len(g.Race) == 0 {
			g.Race = slices.Clone(fill.Race)
		}
		if len(g.Faction) == 0 {
			g.Faction = slices.Clone(fill.Faction)
		}
		if len(g.UnitTypes) == 0 {
			g.UnitTypes = slices.Clone(fill.UnitType)
		}
		v.gear[NameKey(name)] = g
	}
	v.collectRaces()
}

// ApplyLeanFillIns is applyFillIns for the vanilla section of a lean document.
func ApplyLeanFillIns(v *VanillaBook, d *lean.Vanilla) {
	if d != nil {
		applyFillIns(v, fillIns{units: d.Units, mounts: d.Mounts})
	}
}

// OptionSales is what the vanilla.options rule says about each vanilla option: the notSold words, and the exceptions.
type OptionSales struct {
	words  []string
	reason string
	ex     map[string]lean.OptionException
}

// LeanOptionSales reads the vanilla.options section of a lean document.
func LeanOptionSales(v *lean.Vanilla) OptionSales {
	s := OptionSales{ex: map[string]lean.OptionException{}}
	if v == nil || v.Options == nil {
		return s
	}
	s.words, s.reason = v.Options.NotSold, v.Options.Reason
	for _, e := range v.Options.Exceptions {
		s.ex[reservedKey(e.Unit, e.Option)] = e
	}
	return s
}

// Unsold says that no armoury sells the option: its name holds a notSold word, or it is an exception that says so; an option read as
// an armoury item (readAs, ownItem) is sold as that item.
func (s OptionSales) Unsold(unit, option string) bool {
	if e, ok := s.ex[reservedKey(unit, option)]; ok {
		return e.Granted || e.NotSold
	}
	return OptionNotSold(option, s.words)
}

// Reason is "notSold: why" or "granted: why" for an unsold option, as the armoury shows it.
func (s OptionSales) Reason(unit, option string) string {
	if e, ok := s.ex[reservedKey(unit, option)]; ok {
		kind := "notSold"
		if e.Granted {
			kind = "granted"
		}
		return kind + ": " + e.Reason
	}
	return "notSold: " + s.reason
}

// ReadAs is the armoury item an option is read as.
func (s OptionSales) ReadAs(unit, option string) (string, bool) {
	e, ok := s.ex[reservedKey(unit, option)]
	switch {
	case ok && e.ReadAs != "":
		return e.ReadAs, true
	case ok && e.OwnItem != "":
		return e.OwnItem, true
	}
	return "", false
}
