package lean

import (
	"fmt"
	"strconv"
	"strings"
)

// SupportedSchemaVersion is the only schemaVersion this build reads.
const SupportedSchemaVersion = 2

// VersionIssue is the report line for a file of another schemaVersion.
func VersionIssue(file string, got int) Issue {
	return Issue{File: file, Message: fmt.Sprintf("schemaVersion %d is not supported (this app reads %d); the file must be regenerated or converted", got, SupportedSchemaVersion),
		Hint: "the Battle Companies file must say \"schemaVersion\": 2"}
}

// Validate runs the semantic rules that need no engine on a normalised, resolved document: structure
// here, charts, paths and parameter ranges in validate_charts.go. It reports into rep and changes nothing.
func Validate(doc *Document, ix *Index, lower Lower, rep *Reporter) {
	v := &validator{doc: doc, ix: ix, lower: lower, rep: rep}
	v.structure()
	v.shares()
	v.charts()
}

// ringLabels are the labels of every ring in the document (what a replaces entry may name), by name key.
func (v *validator) ringLabels() map[string]string {
	if v.labels != nil {
		return v.labels
	}
	v.labels = map[string]string{}
	add := func(owner string, as []Aura) {
		for _, a := range as {
			l := ringLabel(owner, a)
			v.labels[NameKey(l)] = l
		}
	}
	for _, r := range v.doc.Rules {
		add(ruleShown(&r), r.Auras)
	}
	for _, g := range v.doc.Gear {
		add(g.Name, g.Auras)
	}
	if m := v.doc.Magic; m != nil {
		for n, p := range m.Powers {
			add(n, p.Auras)
		}
	}
	for _, c := range v.doc.Companies {
		for _, r := range c.Rules {
			add(ruleShown(&r), r.Auras)
		}
		for _, g := range c.Gear {
			add(g.Name, g.Auras)
		}
	}
	return v.labels
}

func ruleShown(r *Rule) string {
	if r.Label != "" {
		return r.Label
	}
	return r.Name
}

// ringLabel is the label of a ring: its own, else the name its owner is shown under.
func ringLabel(owner string, a Aura) string {
	if a.Label != "" {
		return a.Label
	}
	return owner
}

// auras checks the rings of one owner (a rule, an item or a power): their labels tell them apart, the group is one of the tables,
// replaces names rings of the document. fromOK is false for an item or a power, whose kind of source is where the ring sits.
func (v *validator) auras(owner string, as []Aura, path string, fromOK bool) {
	seen := map[string]int{}
	for i := range as {
		a := &as[i]
		ap := Idx(path, i)
		l := ringLabel(owner, *a)
		if first, dup := seen[NameKey(l)]; dup {
			v.rep.Add(Issue{Path: ap, Message: fmt.Sprintf("the ring is also labelled %q at %s; rings of one owner need different labels", l, Idx(path, first)),
				Hint: `add "label": "..." to tell the rings apart`})
		} else {
			seen[NameKey(l)] = i
		}
		if a.From != "" && !fromOK {
			v.rep.Add(Issue{Path: Join(ap, "from"), Message: "only the ring of a rule has a source kind; an item's ring is an item ring and a power's a power ring"})
		}
		for j, m := range a.MergeLabels {
			if strings.TrimSpace(m) == "" {
				v.rep.Errorf(Idx(Join(ap, "mergeLabels"), j), "must not be empty")
			}
		}
		if a.Group != "" && v.doc.Auras != nil && len(v.doc.Auras.Groups) > 0 {
			var ids []string
			found := false
			for _, g := range v.doc.Auras.Groups {
				ids = append(ids, g.ID)
				found = found || g.ID == a.Group
			}
			if !found {
				v.rep.Add(Issue{Path: Join(ap, "group"), Message: fmt.Sprintf("no aura group %q in auras.groups", a.Group), Closest: Closest(ids, a.Group), Hint: "known: " + strings.Join(ids, ", ")})
			}
		}
		for j, name := range a.Replaces {
			rp := Idx(Join(ap, "replaces"), j)
			labels := v.ringLabels()
			switch {
			case NameKey(name) == NameKey(l):
				v.rep.Errorf(rp, "a ring cannot replace itself")
			default:
				if _, ok := labels[NameKey(name)]; !ok {
					var all []string
					for _, x := range labels {
						all = append(all, x)
					}
					v.rep.Add(Issue{Path: rp, Message: fmt.Sprintf("no ring labelled %q in this file", name), Closest: Closest(all, name),
						Hint: "replaces names the label of another ring (its label, else the name of its rule, item or power)"})
				}
			}
		}
	}
}

type validator struct {
	doc   *Document
	ix    *Index
	lower Lower
	rep   *Reporter

	labels map[string]string // ringLabels, built when first needed
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
	if m := d.Magic; m != nil {
		v.magic(m)
	}
}

// magic checks the casting limits against the powers: no power is cast on a value the floor forbids.
func (v *validator) magic(m *Magic) {
	for _, name := range SortedKeys(m.Powers) {
		p := m.Powers[name]
		pp := Join("magic.powers", name)
		v.auras(name, p.Auras, Join(pp, "auras"), false)
		if m.Floor == 0 {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(p.Casting), "+")); err == nil && n < m.Floor {
			v.rep.Add(Issue{Path: Join(pp, "casting"), Message: fmt.Sprintf("%q is below magic.floor %d: a casting value is never better than the floor", p.Casting, m.Floor),
				Hint: "raise the casting value or lower magic.floor"})
		}
	}
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

// pins warns when a pin names an occurrence the vanilla file does not have.
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
	if t := r.Target; t != nil && strings.HasSuffix(t.AppliesTo, "-keyword") && len(t.Keywords) == 0 && r.Parameter == nil {
		// a friendly rule may say who it works for in words only (Lordly Quality: the bearer's faction keyword)
		if t.AppliesTo == "friendly-keyword" && strings.TrimSpace(t.Text) != "" {
			// accepted: the text names the models
		} else {
			hint := "list the keywords, or add \"parameter\": {\"kind\": \"keyword\"}"
			if t.AppliesTo == "friendly-keyword" {
				hint += ", or say in text who it works for"
			}
			v.rep.Add(Issue{Path: Join(Join(path, "target"), "keywords"), Message: fmt.Sprintf("a target of %q needs keywords, or the rule needs a parameter that picks them", t.AppliesTo), Hint: hint})
		}
	}
	if b := r.BareTarget; b != nil {
		bp := Join(path, "bareTarget")
		switch {
		case r.Parameter == nil:
			v.rep.Add(Issue{Path: bp, Message: "bareTarget is the target of the rule written without its bracket, so the rule needs a parameter", Hint: "add \"parameter\": {\"kind\": \"keyword\"}, or use target"})
		case r.Target == nil:
			v.rep.Add(Issue{Path: bp, Message: "bareTarget needs the target of the bracket form beside it", Hint: "add \"target\" for the rule written with its bracket"})
		}
		if strings.HasSuffix(b.AppliesTo, "-keyword") && len(b.Keywords) == 0 {
			v.rep.Add(Issue{Path: Join(bp, "keywords"), Message: fmt.Sprintf("the bare form has no keyword to pick, so a target of %q needs keywords", b.AppliesTo), Hint: "list the keywords, or use \"enemy-any\""})
		}
	}
	v.auras(ruleShown(r), r.Auras, Join(path, "auras"), true)
}

func (v *validator) gear(g *Gear, path string) {
	v.auras(g.Name, g.Auras, Join(path, "auras"), false)
	e, ok := v.ix.Lookup(KindGear, g.Name, "")
	if ok && e.Path == path && e.Class == ClassNew && g.Kind == "" {
		v.rep.Add(Issue{Path: Join(path, "kind"), Message: "missing; a new item needs a kind", Hint: "known: wargear, equipment, creature, wanderer"})
	}
}

func (v *validator) pathStructure(p *Path, path string) {
	forEachOption(p.Rows, path, func(o *PathOption, op string) {
		set := 0
		for _, b := range []bool{o.Stat != "", o.Rule != nil, o.MinorRule != nil, o.HeroicAction != "", o.Power != "", o.CastingValue != 0, len(o.OneOf) > 0, o.AnyMinorRule, o.AnyHeroicAction, o.AnyPower} {
			if b {
				set++
			}
		}
		if set != 1 {
			v.rep.Add(Issue{Path: op, Message: fmt.Sprintf("an option sets exactly one of stat, rule, minorRule, heroicAction, power, castingValue, oneOf, anyMinorRule, anyHeroicAction, anyPower; this one sets %d", set)})
		}
		if o.MaxTotal != 0 && o.MinorRule == nil && o.HeroicAction == "" && !o.AnyMinorRule && !o.AnyHeroicAction {
			v.rep.Errorf(Join(op, "maxTotal"), "maxTotal belongs to a minorRule or a heroicAction option")
		}
		if o.Max != 0 && o.Stat == "" {
			v.rep.Errorf(Join(op, "max"), "max belongs to a stat option")
		}
		if o.RollAgain && o.Stat == "" {
			v.rep.Errorf(Join(op, "rollAgain"), "rollAgain belongs to a stat option")
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

// rowText warns when a written row text differs from the composed one: "{name} ({tag}) - {definition}"
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
	if c.Label != "" && NameKey(c.Label) != NameKey(c.Name) {
		v.rep.Errorf(Join(path, "label"), "%q is a label for %q only when it is the same name apart from case, accents and punctuation; a new name is a new company", c.Label, c.Name)
	}
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
			v.chartRow(row, rp, rows.name == "special")
		}
	}
	v.companyLimits(c, path)
	if rc := c.Recruit; rc != nil {
		for i, x := range rc.Replace {
			xp := Idx(Join(Join(path, "recruit"), "replace"), i)
			ranger, alt := x.Unit != "" || len(x.Roles) > 0, x.Alternative != ""
			switch {
			case ranger && alt, !ranger && !alt:
				v.rep.Add(Issue{Path: xp, Message: "a replacement is either a unit with roles (a unit that takes a place once it is slain) or an alternative on a roll of a chart", Hint: "set unit and roles, or chart, roll and alternative"})
			case alt && (x.Chart == "" || x.Roll == nil):
				v.rep.Errorf(xp, "an alternative needs the chart and the roll it replaces")
			case ranger && (x.Unit == "" || len(x.Roles) == 0):
				v.rep.Errorf(xp, "a replacement unit needs its roles")
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

// chartRow checks that a row is exactly one of a unit, nothing, special, choose and options, and that the other keys fit that kind.
func (v *validator) chartRow(row ChartRow, rp string, special bool) {
	kinds := 0
	for _, set := range []bool{row.Unit != "", row.Nothing, row.Special, row.Choose != "", len(row.Options) > 0} {
		if set {
			kinds++
		}
	}
	switch {
	case row.Nothing && row.Unit != "":
		v.rep.Errorf(rp, "a row is either \"nothing\" or a \"unit\", not both")
	case kinds == 0:
		v.rep.Errorf(rp, "a row needs a \"unit\", \"nothing\": true, \"special\": true, \"choose\" or \"options\"")
	case kinds > 1:
		v.rep.Errorf(rp, "a row is one of unit, nothing, special, choose and options; this one sets %d", kinds)
	}
	if row.Choose != "" && row.Scope == "" {
		v.rep.Errorf(Join(rp, "scope"), "a choose row says what the player may choose from: scope \"above\" or \"any\"")
	}
	if row.Choose == "" && (row.Scope != "" || row.Picks != 0) {
		v.rep.Errorf(Join(rp, "scope"), "scope and picks belong to a choose row")
	}
	if row.Blocks && !row.Nothing {
		v.rep.Errorf(Join(rp, "blocks"), "blocks belongs to a \"nothing\" row")
	}
	if row.Rare > 0 && row.Unit == "" {
		v.rep.Errorf(Join(rp, "rare"), "rare belongs to a row with a unit")
	}
	if row.Pick && row.Unit == "" {
		v.rep.Errorf(Join(rp, "pick"), "pick belongs to a row with a unit")
	}
	if row.Special && special {
		v.rep.Errorf(Join(rp, "special"), "special sends a roll on to the special chart, so it belongs to the reinforcement chart")
	}
	if row.Text != "" && len(row.Options) == 0 {
		v.rep.Errorf(Join(rp, "text"), "text belongs to a row with options")
	}
	if len(row.With) > 0 && row.Unit == "" {
		v.rep.Errorf(Join(rp, "with"), "with belongs to a row with a unit")
	}
}

// companyLimits checks that the limits a company exempts or changes exist in limits[] (when the file lists any).
func (v *validator) companyLimits(c *Company, path string) {
	l := c.Limits
	if l == nil || len(v.doc.Limits) == 0 {
		return
	}
	known := make([]string, len(v.doc.Limits))
	for i, x := range v.doc.Limits {
		known[i] = x.ID
	}
	check := func(id, at string) {
		for _, k := range known {
			if NameKey(k) == NameKey(id) {
				return
			}
		}
		iss := Issue{Path: at, Message: fmt.Sprintf("no limit %q in limits[]", id), Hint: "known: " + strings.Join(known, ", ")}
		iss.Closest = Closest(known, id)
		v.rep.Add(iss)
	}
	for i, x := range l.Exempt {
		for j, id := range x.Limits {
			check(id, Idx(Join(Idx(Join(Join(path, "limits"), "exempt"), i), "limits"), j))
		}
		if x.Unit == "" && x.Item == "" {
			v.rep.Errorf(Idx(Join(Join(path, "limits"), "exempt"), i), "an exemption names a unit or an item")
		}
	}
	for _, id := range SortedKeys(l.Override) {
		check(id, Join(Join(Join(path, "limits"), "override"), id))
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

// officers reports a company with nobody who may lead.
func (v *validator) officers(c *Company, path string) {
	h := c.Heroes
	if h == nil || len(c.Start.Units) == 0 {
		return
	}
	var may int
	for _, su := range c.Start.Units {
		if h.Who.Admits(Subject{Names: []string{su.Unit}, Known: true}) == "" {
			may++
		}
	}
	if may == 0 {
		why := "heroes.who.except names every starting unit"
		if h.Who != nil && len(h.Who.Units) > 0 {
			why = "heroes.who.units names none of the starting units"
		}
		v.rep.Add(Issue{Path: Join(path, "start"), Message: "the company has no unit that may lead (" + why + ")", Hint: "a company needs at least one starting unit that may become an officer"})
	}
	if w := h.Who; w != nil && (w.Hero != nil || len(w.Race) > 0 || len(w.Keywords) > 0 || w.Alignment != "") {
		v.rep.Add(Issue{Path: Join(Join(path, "heroes"), "who"), Message: "the starting heroes are chosen by unit name: only units and except count here", Hint: "list the units that may lead (units) or the ones that may not (except)"})
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
