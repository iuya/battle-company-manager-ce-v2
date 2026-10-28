package lean

import (
	"fmt"
	"strings"
)

// SupportedSchemaVersion is the only schemaVersion this build reads (design section 2).
const SupportedSchemaVersion = 2

// VersionIssue is the report line for a file of another schemaVersion (design section 7, "version").
func VersionIssue(file string, got int) Issue {
	return Issue{File: file, Message: fmt.Sprintf("schemaVersion %d is not supported (this app reads %d); the file must be regenerated or converted", got, SupportedSchemaVersion),
		Hint: "the Battle Companies file must say \"schemaVersion\": 2"}
}

// Validate runs the semantic rules that need no engine (design sections 7 and 12.7) on a normalised, resolved document: structure
// here, charts, paths and parameter ranges in validate_charts.go. It reports into rep and changes nothing.
func Validate(doc *Document, ix *Index, lower Lower, rep *Reporter) {
	v := &validator{doc: doc, ix: ix, lower: lower, rep: rep}
	v.structure()
	v.charts()
}

type validator struct {
	doc   *Document
	ix    *Index
	lower Lower
	rep   *Reporter
}

var pinKinds = map[string]Kind{"hero": KindUnit, "warrior": KindUnit, "unit": KindUnit, "item": KindGear, "rule": KindRule, "mount": KindMount}

func (v *validator) structure() {
	d := v.doc
	if d.Vanilla != nil {
		v.pins(d.Vanilla)
	}
	for i := range d.Scenarios {
		v.scenario(&d.Scenarios[i], Idx("scenarios", i))
	}
	for i := range d.Rules {
		v.rule(&d.Rules[i], Idx("rules", i))
	}
	for i := range d.Gear {
		v.gear(&d.Gear[i], Idx("gear", i))
	}
	for i := range d.Paths {
		v.pathStructure(&d.Paths[i], Idx("paths", i))
	}
	for i := range d.Companies {
		v.company(&d.Companies[i], Idx("companies", i))
	}
	if d.Auras != nil {
		v.uniqueIDs(len(d.Auras.Kinds), func(i int) string { return d.Auras.Kinds[i].ID }, "auras.kinds")
		v.uniqueIDs(len(d.Auras.Groups), func(i int) string { return d.Auras.Groups[i].ID }, "auras.groups")
	}
	v.uniqueIDs(len(d.Limits), func(i int) string { return d.Limits[i].ID }, "limits")
}

func (v *validator) uniqueIDs(n int, id func(int) string, path string) {
	seen := map[string]int{}
	for i := 0; i < n; i++ {
		k := NameKey(id(i))
		if k == "" {
			continue
		}
		if first, dup := seen[k]; dup {
			v.rep.Add(Issue{Path: Join(Idx(path, i), "id"), Message: fmt.Sprintf("%q is already used at %s; ids are unique", id(i), Idx(path, first))})
			continue
		}
		seen[k] = i
	}
}

// pins warns when a pin names an occurrence the vanilla file does not have (design 7, "vanilla drift").
func (v *validator) pins(van *Vanilla) {
	if v.lower == nil {
		return
	}
	for i, p := range van.Pins {
		kind, ok := pinKinds[p.Kind]
		if !ok {
			continue
		}
		if v.lower.Duplicates(kind, p.Name) < p.Occurrence {
			v.rep.Warnf(Join(Idx("vanilla.pins", i), "occurrence"), "occurrence %d does not exist for %q in this vanilla file", p.Occurrence, p.Name)
		}
	}
}

func (v *validator) scenario(s *Scenario, path string) {
	seen := map[string]int{}
	for i, sc := range s.Scoring {
		p := Idx(Join(path, "scoring"), i)
		if first, dup := seen[sc.Key]; dup {
			v.rep.Errorf(Join(p, "key"), "%q is listed twice (also at %s)", sc.Key, Idx(Join(path, "scoring"), first))
		} else {
			seen[sc.Key] = i
		}
		if n := sc.Nomination; n != nil {
			np := Join(p, "nomination")
			if n.From == "terrain" && n.AllowLeader != "" {
				v.rep.Errorf(Join(np, "allowLeader"), "allowLeader belongs to a nomination of heroes, not of terrain")
			}
			if n.From != "terrain" && n.Where != "" {
				v.rep.Errorf(Join(np, "where"), "where belongs to a nomination of terrain")
			}
		}
		for j, o := range sc.Options {
			if strings.TrimSpace(o.Text) == "" {
				v.rep.Errorf(Join(Idx(Join(p, "options"), j), "text"), "must not be empty")
			}
		}
	}
}

func (v *validator) rule(r *Rule, path string) {
	if r.Target != nil && strings.HasSuffix(r.Target.AppliesTo, "-keyword") && len(r.Target.Keywords) == 0 && r.Parameter == nil {
		v.rep.Add(Issue{Path: Join(Join(path, "target"), "keywords"), Message: fmt.Sprintf("a target of %q needs keywords, or the rule needs a parameter that picks them", r.Target.AppliesTo),
			Hint: "list the keywords, or add \"parameter\": {\"kind\": \"keyword\"}"})
	}
}

func (v *validator) gear(g *Gear, path string) {
	e, ok := v.ix.Lookup(KindGear, g.Name, "")
	if ok && e.Path == path && e.Class == ClassNew && g.Kind == "" {
		v.rep.Add(Issue{Path: Join(path, "kind"), Message: "missing; a new item needs a kind", Hint: "known: wargear, equipment, creature, wanderer"})
	}
}

func (v *validator) pathStructure(p *Path, path string) {
	forEachOption(p.Rows, path, func(o *PathOption, op string) {
		set := 0
		for _, b := range []bool{o.Stat != "", o.Rule != nil, o.MinorRule != nil, o.HeroicAction != "", o.Power != "", o.CastingValue != 0, len(o.OneOf) > 0} {
			if b {
				set++
			}
		}
		if set != 1 {
			v.rep.Add(Issue{Path: op, Message: fmt.Sprintf("an option sets exactly one of stat, rule, minorRule, heroicAction, power, castingValue, oneOf; this one sets %d", set)})
		}
		if o.MaxTotal != 0 && o.MinorRule == nil && o.HeroicAction == "" {
			v.rep.Errorf(Join(op, "maxTotal"), "maxTotal belongs to a minorRule or a heroicAction option")
		}
		if o.Amount != 0 && o.Stat == "" {
			v.rep.Errorf(Join(op, "amount"), "amount belongs to a stat option")
		}
	})
	v.rowText(p, path)
}

// forEachOption visits every option of the rows, oneOf members included, with its path.
func forEachOption(rows []PathRow, path string, fn func(o *PathOption, p string)) {
	var walk func(opts []PathOption, p string)
	walk = func(opts []PathOption, p string) {
		for i := range opts {
			op := Idx(p, i)
			fn(&opts[i], op)
			walk(opts[i].OneOf, Join(op, "oneOf"))
		}
	}
	for i := range rows {
		walk(rows[i].Options, Join(Idx(Join(path, "rows"), i), "options"))
	}
}

// rowText warns when a written row text differs from the composed one (design 5, 7 "path text"): "{name} ({tag}) - {definition}"
// for a row with one rule option whose definition this file holds.
func (v *validator) rowText(p *Path, path string) {
	for i := range p.Rows {
		row := &p.Rows[i]
		if row.Text == "" || len(row.Options) != 1 || row.Options[0].Rule == nil {
			continue
		}
		ref := row.Options[0].Rule
		def, tag, ok := v.ruleText(ref)
		if !ok {
			continue
		}
		composed := ref.Name
		if tag != "" {
			composed += " (" + tag + ")"
		}
		composed += " - " + def
		if strings.TrimSpace(row.Text) != composed {
			v.rep.Add(Issue{Path: Join(Idx(Join(path, "rows"), i), "text"), Severity: Warn,
				Message: fmt.Sprintf("differs from the composed %q", ellipsis(composed, 40)), Hint: "remove \"text\" unless the difference is intended"})
		}
	}
}

func (v *validator) ruleText(ref *RuleRef) (def, tag string, ok bool) {
	if ref.Def != nil {
		return ref.Def.Definition, ref.Def.Tag, ref.Def.Definition != ""
	}
	if e, found := v.ix.Lookup(KindRule, ref.Name, ""); found {
		for i := range v.doc.Rules {
			if Idx("rules", i) == e.Path {
				r := &v.doc.Rules[i]
				return r.Definition, r.Tag, r.Definition != ""
			}
		}
	}
	return "", "", false
}

func ellipsis(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

func (v *validator) company(c *Company, path string) {
	if c.Start == nil {
		v.rep.Errorf(Join(path, "start"), "missing")
	}
	if c.Reinforcements == nil {
		v.rep.Errorf(Join(path, "reinforcements"), "missing")
	}
	if c.Start != nil {
		v.officers(c, path)
		models := 0
		for _, u := range c.Start.Units {
			models += u.Count
		}
		if c.MaxModels > 0 && models > c.MaxModels {
			v.rep.Errorf(Join(path, "start"), "the starting force has %d models, more than maxModels %d", models, c.MaxModels)
		}
	}
	for _, rows := range []struct {
		name string
		rows []ChartRow
	}{{"reinforcements", reinfRows(c)}, {"special", specialRows(c)}} {
		for i, row := range rows.rows {
			rp := Idx(Join(Join(path, rows.name), "rows"), i)
			switch {
			case row.Nothing && row.Unit != "":
				v.rep.Errorf(rp, "a row is either \"nothing\" or a \"unit\", not both")
			case !row.Nothing && row.Unit == "":
				v.rep.Errorf(rp, "a row needs a \"unit\" or \"nothing\": true")
			}
			if row.Blocks && !row.Nothing {
				v.rep.Errorf(Join(rp, "blocks"), "blocks belongs to a \"nothing\" row")
			}
			if row.Rare > 0 && row.Unit == "" {
				v.rep.Errorf(Join(rp, "rare"), "rare belongs to a row with a unit")
			}
		}
	}
	for i := range c.Rules {
		v.rule(&c.Rules[i], Idx(Join(path, "rules"), i))
	}
	for i := range c.Gear {
		g := &c.Gear[i]
		if e, ok := v.ix.Lookup(KindGear, g.Name, c.Name); ok && e.Scope == c.Name && e.Class == ClassNew && g.Kind == "" {
			v.rep.Add(Issue{Path: Join(Idx(Join(path, "gear"), i), "kind"), Message: "missing; a new item needs a kind", Hint: "known: wargear, equipment, creature, wanderer"})
		}
	}
}

func reinfRows(c *Company) []ChartRow {
	if c.Reinforcements == nil {
		return nil
	}
	return c.Reinforcements.Rows
}

func specialRows(c *Company) []ChartRow {
	if c.Special == nil {
		return nil
	}
	return c.Special.Rows
}

// officers reports a company with nobody who may lead (design 7, "officers").
func (v *validator) officers(c *Company, path string) {
	h := c.Heroes
	if h == nil || len(c.Start.Units) == 0 {
		return
	}
	var may int
	for _, su := range c.Start.Units {
		k := NameKey(su.Unit)
		if inList(h.Excluded, k) {
			continue
		}
		if len(h.Allowed) > 0 && !inList(h.Allowed, k) {
			continue
		}
		may++
	}
	if may == 0 {
		why := "heroes.excluded names every starting unit"
		if len(h.Allowed) > 0 {
			why = "heroes.allowed names none of the starting units"
		}
		v.rep.Add(Issue{Path: Join(path, "start"), Message: "the company has no unit that may lead (" + why + ")", Hint: "a company needs at least one starting unit that may become an officer"})
	}
}

func inList(list []string, key string) bool {
	for _, x := range list {
		if NameKey(x) == key {
			return true
		}
	}
	return false
}
