package core

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// The three forms of a unit in a lean document (docs/customizing-rulesets.md, "Units"), and where each one is resolved:
//
//   - an annotation of a vanilla unit (no extends, no profile): the properties ride on the vanilla profile (tts, types, models...);
//   - a unit with `extends` and a delta: a UnitAlias of the company, built by leanUnitAliases for the company's own units and for the shared
//     (top-level) units alike, the company's own first;
//   - a brand-new unit (a `profile`, no base): a VanillaUnit in the same shape a vanilla profile has, registered by RegisterLeanUnits.
//
// RuleSet.ResolveUnit (names.go) is the one place that finds the profile a name stands for: the company's own, then the shared units, then vanilla.

// newUnitProfile says that a unit is a definition of its own (a brand-new unit, or a re-definition with `overrides`) rather than a delta on
// a base: it has a profile and no other base. A unit that extends one with a profile inherited it through `extends`.
func newUnitProfile(u lean.Unit) bool {
	return u.Profile != nil && (u.Extends == "" || lean.NameKey(u.Extends) == lean.NameKey(u.Name))
}

// leanUnitProfile reads the profile of a brand-new unit in the vanilla file's own shape: the flat characteristics, race, faction, unitType,
// specialRules, wargear, heroicActions, magicalPowers and baseSize of `profile`, with the unit's points, options and includes.
func leanUnitProfile(book *VanillaBook, u lean.Unit) VanillaUnit {
	raw := map[string]json.RawMessage{}
	if b, err := json.Marshal(u.Profile); err == nil {
		_ = json.Unmarshal(b, &raw)
	}
	set := func(key string, v any) {
		if b, err := json.Marshal(v); err == nil {
			raw[key] = b
		}
	}
	set("points", u.Points)
	if u.Options != nil {
		set("options", u.Options)
	}
	if len(u.Includes) > 0 {
		set("includes", u.Includes)
	}
	return book.readUnit(u.Name, raw)
}

// RegisterLeanUnits adds the brand-new units of the document to the book, so the whole app treats them as vanilla profiles: the shared units, and a
// company's own units when their name is free (a name two companies define differently stays with its company, CompanyProfile.NewUnits).
// A unit that re-defines a vanilla unit (`overrides`) replaces it. A unit's `aka` names find it as well.
func RegisterLeanUnits(book *VanillaBook, d *lean.Document) {
	add := func(u lean.Unit, shared bool) {
		v := leanUnitProfile(book, u)
		for _, name := range append([]string{u.Name}, u.Aka...) {
			if _, taken := book.units[name]; shared || !taken {
				book.units[name] = v
			}
		}
	}
	for _, u := range d.Units {
		if newUnitProfile(u) {
			add(u, true)
		}
	}
	for _, c := range d.Companies {
		for _, u := range c.Units {
			if newUnitProfile(u) {
				add(u, false)
			}
		}
	}
	book.collectRaces()
}

// leanUnitAliases are the units a company can field that build on another unit, by name (and aka): the company's own, then the shared ones
// it does not shadow. Each has its Extends followed to the unit that has a profile (a vanilla or brand-new one), so an alias is always one step
// from a profile; the options and rules the chain gave were merged into the unit when the file was read.
func leanUnitAliases(d *lean.Document, c lean.Company) []lean.Unit {
	chain := func(base string) string {
		for range 16 {
			next, ok := lean.Unit{}, false
			for _, list := range [][]lean.Unit{c.Units, d.Units} {
				for _, u := range list {
					if lean.NameKey(u.Name) == lean.NameKey(base) || slices.ContainsFunc(u.Aka, func(a string) bool { return lean.NameKey(a) == lean.NameKey(base) }) {
						next, ok = u, true
						break
					}
				}
				if ok {
					break
				}
			}
			if !ok || next.Extends == "" || lean.NameKey(next.Extends) == lean.NameKey(next.Name) {
				if ok {
					return next.Name
				}
				return base
			}
			base = next.Extends
		}
		return base
	}
	var out []lean.Unit
	seen := map[string]bool{}
	for _, list := range [][]lean.Unit{c.Units, d.Units} {
		for _, u := range list {
			if u.Extends == "" || lean.NameKey(u.Extends) == lean.NameKey(u.Name) || seen[lean.NameKey(u.Name)] {
				continue
			}
			seen[lean.NameKey(u.Name)] = true
			u.Extends = chain(u.Extends)
			out = append(out, u)
		}
	}
	return out
}

// checkNewUnits reports what is wrong with the profile of a brand-new unit: the checker cannot read it from the types, as the profile is written
// the way the vanilla file writes one. A unit nobody fields is valid.
func checkNewUnits(file string, d *lean.Document, ix *lean.Index, lower *vanillaLower, rep *lean.Report) {
	if lower == nil || lower.book == nil {
		return
	}
	check := func(u lean.Unit, path string) {
		if !newUnitProfile(u) {
			return
		}
		pp := lean.Join(path, "profile")
		fail := func(at, msg, hint string) { rep.Add(lean.Issue{File: file, Path: at, Message: msg, Hint: hint}) }
		prof, ok := u.Profile.(map[string]any)
		if !ok {
			fail(pp, "a unit profile is an object of the characteristics and words of a vanilla profile", `write {"movement": 6, "fight": 4, ..., "race": ["Man"], "unitType": ["Warrior", "Infantry"]}`)
			return
		}
		stats := lower.book.StatKeys()
		words := []string{"race", "faction", "unitType", "specialRules", "wargear", "heroicActions", "magicalPowers", "baseSize"}
		known := slices.Concat(stats, words)
		for _, k := range lean.SortedKeys(prof) {
			if !slices.Contains(known, k) {
				fail(lean.Join(pp, k), "unknown field "+fmt.Sprintf("%q", k), "known fields: "+strings.Join(known, ", "))
			}
		}
		for _, s := range stats {
			v, has := prof[s]
			_, isNum := v.(json.Number)
			switch {
			case has && (isNum || v == "-" && s == "shoot"):
			case !has && lower.book.HeroStat(s):
			case !has:
				fail(lean.Join(pp, s), fmt.Sprintf("a unit needs its %s", s), "every characteristic that is not a hero's (Might, Will, Fate) is a number; shoot may be \"-\"")
			default:
				fail(lean.Join(pp, s), fmt.Sprintf("%s is a number", s), "")
			}
		}
		for _, w := range []string{"race", "unitType"} {
			if l, _ := prof[w].([]any); len(l) == 0 {
				fail(lean.Join(pp, w), fmt.Sprintf("a unit needs its %s", w), "a list of words, for example race [\"Man\"], unitType [\"Warrior\", \"Infantry\"]")
			}
		}
		named := func(key string, kind lean.Kind, plain bool) {
			list, _ := prof[key].([]any)
			for i, x := range list {
				name, _ := x.(string)
				base, _, _ := strings.Cut(name, " (")
				_, inFile := ix.Lookup(kind, name, "")
				if name == "" || inFile || lower.Has(kind, name) || (!plain && lower.Has(kind, base)) {
					continue
				}
				rep.Add(lean.Issue{File: file, Path: lean.Idx(lean.Join(pp, key), i), Message: fmt.Sprintf("no %s %q in this file, vanilla or the glossary", kind, name),
					Closest: lean.Closest(lower.Names(kind), name)})
			}
		}
		named("specialRules", lean.KindRule, false)
		named("wargear", lean.KindGear, true)
	}
	for i, u := range d.Units {
		check(u, lean.Idx("units", i))
	}
	for ci, c := range d.Companies {
		for i, u := range c.Units {
			check(u, lean.Idx(lean.Join(lean.Idx("companies", ci), "units"), i))
		}
	}
}
