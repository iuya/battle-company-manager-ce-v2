package lean

import (
	"reflect"
	"strings"
)

// The extends engine: a company, unit, gear item or path may name a base; the result is the base's
// fields with the child's own on top. Scalars override, lists and nested objects replace as a whole. A field "written" is a field
// that is not the zero value, so a child cannot override `true` with `false` (no shipped field needs it). The merge skips what must
// stay the child's own (name, id, aka, definition text ...). A base that is not in the file (vanilla, the glossary) is left to the
// resolution pass; a loop of bases is an error here.

type extendsKind struct {
	label string   // "company", "unit" ...
	skip  []string // Go field names never inherited
}

var (
	extCompany = extendsKind{"company", []string{"Name", "ID", "Aka", "Extends", "Faction", "Description", "Epigraph", "Notes", "SpecialRules"}}
	extUnit    = extendsKind{"unit", []string{"Name", "ID", "Aka", "Extends", "Note", "Overrides"}}
	extGear    = extendsKind{"gear item", []string{"Name", "ID", "Aka", "Extends", "Overrides", "Definition", "Note", "TTS"}}
	extPath    = extendsKind{"path", []string{"Name", "ID", "Aka", "Extends", "Note"}}
)

// mergeBase fills the zero fields of child from a copy of base, except the skipped ones.
func mergeBase(child, base reflect.Value, skip []string) {
	t := child.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() || hasString(skip, f.Name) {
			continue
		}
		if child.Field(i).IsZero() {
			child.Field(i).Set(deepCopy(base.Field(i)))
		}
	}
}

func hasString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// extendsList resolves the extends of items[i] for every i. path is the JSON path of the list ("companies"); outer is a list
// searched after items for a base (the shared units for a company-local unit), already resolved. name, extends and aka read the
// fields of an item.
func extendsList[T any](items []T, outer []T, path string, kind extendsKind, name, extends func(*T) string, aka func(*T) []string, rep *Reporter) {
	state := make([]int, len(items)) // 0 new, 1 visiting, 2 done
	find := func(list []T, want string, self int, selfInList bool) int {
		key := NameKey(want)
		for j := range list {
			if selfInList && j == self {
				continue
			}
			if NameKey(name(&list[j])) == key {
				return j
			}
			for _, a := range aka(&list[j]) {
				if NameKey(a) == key {
					return j
				}
			}
		}
		return -1
	}
	var visit func(i int, chain []string)
	visit = func(i int, chain []string) {
		if state[i] == 2 {
			return
		}
		state[i] = 1
		defer func() { state[i] = 2 }()
		item := &items[i]
		ext := extends(item)
		if ext == "" {
			return
		}
		base := -1
		if NameKey(name(item)) != NameKey(ext) { // an entry named like its base annotates a lower layer: never a base in this list
			base = find(items, ext, i, true)
		}
		var b *T
		if base >= 0 {
			if state[base] == 1 {
				loop := append(append([]string(nil), chain...), name(&items[base]))
				rep.Errorf(Join(Idx(path, i), "extends"), "%q extends itself through %s; extends must not form a loop", ext, strings.Join(loop, " -> "))
				return
			}
			visit(base, append(chain, name(&items[base])))
			b = &items[base]
		} else if j := find(outer, ext, -1, false); j >= 0 {
			b = &outer[j]
		}
		if b == nil {
			return // vanilla or missing: the resolution pass decides
		}
		mergeBase(reflect.ValueOf(item).Elem(), reflect.ValueOf(b).Elem(), kind.skip)
	}
	for i := range items {
		visit(i, []string{name(&items[i])})
	}
}

// applyExtends resolves every extends of the document: shared units and gear first, then companies, their local units and gear.
func applyExtends(doc *Document, rep *Reporter) {
	unitName := func(u *Unit) string { return u.Name }
	unitExt := func(u *Unit) string { return u.Extends }
	unitAka := func(u *Unit) []string { return u.Aka }
	gearName := func(g *Gear) string { return g.Name }
	gearExt := func(g *Gear) string { return g.Extends }
	gearAka := func(g *Gear) []string { return g.Aka }

	nameUnits(doc.Units, "units", rep)
	extendsList(doc.Units, nil, "units", extUnit, unitName, unitExt, unitAka, rep)
	extendsList(doc.Gear, nil, "gear", extGear, gearName, gearExt, gearAka, rep)
	extendsList(doc.Paths, nil, "paths", extPath, func(p *Path) string { return p.Name }, func(p *Path) string { return p.Extends }, func(p *Path) []string { return p.Aka }, rep)
	// a company's own units and gear resolve before the company inherits them
	for i := range doc.Companies {
		c := &doc.Companies[i]
		cp := Idx("companies", i)
		nameUnits(c.Units, Join(cp, "units"), rep)
		extendsList(c.Units, doc.Units, Join(cp, "units"), extUnit, unitName, unitExt, unitAka, rep)
		extendsList(c.Gear, doc.Gear, Join(cp, "gear"), extGear, gearName, gearExt, gearAka, rep)
	}
	extendsList(doc.Companies, nil, "companies", extCompany, func(c *Company) string { return c.Name }, func(c *Company) string { return c.Extends }, func(c *Company) []string { return c.Aka }, rep)
}

// nameUnits gives an annotation without a name the name of its base, and reports a unit with neither.
func nameUnits(units []Unit, path string, rep *Reporter) {
	for i := range units {
		u := &units[i]
		if u.Name == "" {
			if u.Extends == "" {
				rep.Errorf(Idx(path, i), `a unit needs a "name" or an "extends"`)
				continue
			}
			u.Name = u.Extends
		}
	}
}
