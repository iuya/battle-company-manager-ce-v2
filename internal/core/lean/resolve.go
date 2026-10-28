package lean

import (
	"fmt"
	"sort"
	"strings"
)

// The name resolution pass (design 1, 4.10, 7): every name the file writes is looked up, in order, in the entity's own scope
// (a company's units and gear), the shared lists of the file, and the lower layers; every entry is classified against the layers
// below it. Nothing is fuzzy: names compare by NameKey, aliases (aka) resolve to the entity.

// Class says what an entry is relative to the layers below it (design 4.10).
type Class int

const (
	ClassNew        Class = iota // a name nothing below has, with a definition (or a base)
	ClassExtends                 // a new name built on a base
	ClassAnnotation              // a name that exists below, adding properties only
	ClassShadow                  // a name that exists below, replacing it (`overrides: true`)
)

func (c Class) String() string {
	return [...]string{"new", "extends", "annotation", "shadow"}[c]
}

// Entry is one named thing of the file.
type Entry struct {
	Kind  Kind
	Name  string
	Path  string // JSON path of the entry
	Scope string // "" for the shared lists, else the name of the company that owns it
	Class Class
}

// HoistedRule is a rule defined inline (a company's rules, a path row) and lifted into the rule index.
type HoistedRule struct {
	Name       string
	Definition string
	Path       string
}

// Index is the result of the pass: what each name of the file resolves to.
type Index struct {
	entries map[scopeKind]map[string]Entry // NameKey -> entry (aka included)
	order   map[scopeKind][]Entry          // file order, one per entity
	Hoisted []HoistedRule
}

type scopeKind struct {
	scope string // NameKey of the company, "" shared
	kind  Kind
}

// Lookup finds a name of a kind visible from a company (scope is the company's name; "" for the shared lists only). A company's
// own entries win over the shared ones. It does not look at the lower layers: the caller asks Lower.
func (ix *Index) Lookup(kind Kind, name, scope string) (Entry, bool) {
	key := NameKey(name)
	if scope != "" {
		if e, ok := ix.entries[scopeKind{NameKey(scope), kind}][key]; ok {
			return e, true
		}
	}
	e, ok := ix.entries[scopeKind{"", kind}][key]
	return e, ok
}

// Entries lists the entities of a kind in file order for one scope ("" shared).
func (ix *Index) Entries(kind Kind, scope string) []Entry {
	return ix.order[scopeKind{NameKey(scope), kind}]
}

// ent is the common view of an entity for duplicate and classification checks.
type ent struct {
	name, path, extends string
	aka                 []string
	hasDef, overrides   bool
	canBeNew            bool // enough to create a new thing even without a definition text (an item with a kind)
}

type resolver struct {
	doc   *Document
	lower Lower
	rep   *Reporter
	ix    *Index
}

// Resolve checks every name of a normalised document against the file and the lower layers, reports what does not resolve, and
// returns the index. rep collects the errors; the index is complete even when errors were found.
func Resolve(doc *Document, lower Lower, rep *Reporter) *Index {
	r := &resolver{doc: doc, lower: lower, rep: rep, ix: &Index{entries: map[scopeKind]map[string]Entry{}, order: map[scopeKind][]Entry{}}}
	r.declare()
	r.references()
	return r.ix
}

// ---------------------------------------------------------------- declarations

func (r *resolver) declare() {
	d := r.doc
	r.declareKind(KindUnit, "", "", unitEnts(d.Units, "units"), "vanilla")
	r.declareKind(KindGear, "", "", gearEnts(d.Gear, "gear"), "vanilla")
	r.declareKind(KindRule, "", "", ruleEnts(d.Rules, "rules"), "vanilla or the glossary")
	r.declareKind(KindPath, "", "", pathEnts(d.Paths), "")
	r.declareKind(KindScenario, "", "", scenarioEnts(d.Scenarios), "")
	r.declareKind(KindCompany, "", "", companyEnts(d.Companies), "")
	for i := range d.Companies {
		c := &d.Companies[i]
		cp := Idx("companies", i)
		r.declareKind(KindUnit, c.Name, c.Name, unitEnts(c.Units, Join(cp, "units")), "the shared units or vanilla")
		r.declareKind(KindGear, c.Name, c.Name, gearEnts(c.Gear, Join(cp, "gear")), "the shared gear or vanilla")
	}
	r.declarePowers()
	r.hoist()
}

func (r *resolver) declarePowers() {
	if r.doc.Magic == nil {
		return
	}
	sk := scopeKind{"", KindPower}
	r.ix.entries[sk] = map[string]Entry{}
	for _, name := range SortedKeys(r.doc.Magic.Powers) {
		e := Entry{Kind: KindPower, Name: name, Path: Join("magic.powers", name), Class: ClassNew}
		if r.lowerHas(KindPower, name) {
			e.Class = ClassAnnotation
		}
		r.ix.entries[sk][NameKey(name)] = e
		r.ix.order[sk] = append(r.ix.order[sk], e)
	}
}

func unitEnts(us []Unit, path string) []ent {
	out := make([]ent, len(us))
	for i, u := range us {
		out[i] = ent{name: u.Name, path: Idx(path, i), extends: u.Extends, aka: u.Aka, overrides: u.Overrides,
			hasDef: u.Profile != nil || u.Options != nil || u.Points != 0 || u.Definition != nil}
	}
	return out
}

func gearEnts(gs []Gear, path string) []ent {
	out := make([]ent, len(gs))
	for i, g := range gs {
		// An item's kind, prices and classes are properties an annotation may add to a vanilla item; only its text or profile
		// redefines it.
		out[i] = ent{name: g.Name, path: Idx(path, i), extends: g.Extends, aka: g.Aka, overrides: g.Overrides,
			hasDef: g.Definition != "" || g.Profile != nil, canBeNew: g.Kind != ""}
	}
	return out
}

func ruleEnts(rs []Rule, path string) []ent {
	out := make([]ent, len(rs))
	for i, x := range rs {
		out[i] = ent{name: x.Name, path: Idx(path, i), aka: x.Aka, overrides: x.Overrides, hasDef: x.Definition != ""}
	}
	return out
}

func pathEnts(ps []Path) []ent {
	out := make([]ent, len(ps))
	for i, p := range ps {
		out[i] = ent{name: p.Name, path: Idx("paths", i), extends: p.Extends, aka: p.Aka, hasDef: true}
	}
	return out
}

func scenarioEnts(ss []Scenario) []ent {
	out := make([]ent, len(ss))
	for i, s := range ss {
		out[i] = ent{name: s.Name, path: Idx("scenarios", i), aka: s.Aka, hasDef: true}
	}
	return out
}

func companyEnts(cs []Company) []ent {
	out := make([]ent, len(cs))
	for i, c := range cs {
		out[i] = ent{name: c.Name, path: Idx("companies", i), extends: c.Extends, aka: c.Aka, hasDef: true}
	}
	return out
}

// declareKind registers the entities of one list, reports duplicates and classifies each against the layers below. below names
// the lower layers for messages ("" when the kind has none).
func (r *resolver) declareKind(kind Kind, scopeName, company string, ents []ent, below string) {
	sk := scopeKind{NameKey(scopeName), kind}
	if r.ix.entries[sk] == nil {
		r.ix.entries[sk] = map[string]Entry{}
	}
	seen := r.ix.entries[sk]
	for _, e := range ents {
		key := NameKey(e.name)
		if key == "" {
			if e.name != "" {
				r.rep.Errorf(Join(e.path, "name"), "%q has no letter or digit; a name needs at least one", e.name)
			}
			continue
		}
		cls := r.classify(kind, e, below, scopeName)
		entry := Entry{Kind: kind, Name: e.name, Path: e.path, Scope: scopeName, Class: cls}
		if prev, dup := seen[key]; dup {
			r.rep.Add(Issue{Path: Join(e.path, "name"), Message: fmt.Sprintf("%q is already defined at %s; names are unique", e.name, prev.Path), Hint: "use another name or put the old spelling in aka"})
		} else {
			seen[key] = entry
			r.ix.order[sk] = append(r.ix.order[sk], entry)
		}
		for ai, a := range e.aka {
			ak := NameKey(a)
			apath := Idx(Join(e.path, "aka"), ai)
			switch prev, dup := seen[ak]; {
			case ak == "":
				r.rep.Errorf(apath, "%q has no letter or digit", a)
			case ak == key:
				r.rep.Errorf(apath, "%q is the name itself; an alias spells it differently", a)
			case dup:
				r.rep.Add(Issue{Path: apath, Message: fmt.Sprintf("%q is already defined at %s; names and aliases are unique", a, prev.Path), Hint: "use another alias"})
			default:
				seen[ak] = entry
				if r.lowerHas(kind, a) && cls != ClassAnnotation && below != "" {
					r.rep.Add(Issue{Path: apath, Message: fmt.Sprintf("%q is a %s name; an alias must not shadow a different %s", a, lowerWord(kind), kind), Hint: "pick another alias, or annotate the " + string(kind) + " instead"})
				}
			}
		}
	}
}

func lowerWord(kind Kind) string {
	if kind == KindRule {
		return "vanilla or glossary"
	}
	return "vanilla"
}

func (r *resolver) lowerHas(kind Kind, name string) bool {
	return r.lower != nil && r.lower.Has(kind, name)
}

// outerHas looks for a name in the layers below an entry of scope: the shared list (when the entry is company-local) then the
// lower layers.
func (r *resolver) outerHas(kind Kind, name, scope string) bool {
	if scope != "" {
		if _, ok := r.ix.entries[scopeKind{"", kind}][NameKey(name)]; ok {
			return true
		}
	}
	return r.lowerHas(kind, name)
}

func (r *resolver) classify(kind Kind, e ent, below, scope string) Class {
	if below == "" { // path, scenario, company: always defined here
		if e.extends != "" {
			return ClassExtends
		}
		return ClassNew
	}
	exists := r.outerHas(kind, e.name, scope)
	selfBase := e.extends != "" && NameKey(e.extends) == NameKey(e.name)
	switch {
	case exists && e.overrides:
		return ClassShadow
	case exists && (selfBase || (!e.hasDef && e.extends == "")):
		return ClassAnnotation
	case exists && e.hasDef:
		r.rep.Add(Issue{Path: e.path, Message: fmt.Sprintf("%q exists in %s; add \"overrides\": true or drop the definition to annotate it", e.name, below),
			Hint: "an entry that only adds properties to something below has no definition"})
		return ClassShadow
	case exists:
		r.rep.Add(Issue{Path: e.path, Message: fmt.Sprintf("%q exists in %s; add \"overrides\": true to replace it or choose another name", e.name, below)})
		return ClassShadow
	case e.extends != "":
		return ClassExtends
	case e.hasDef || e.canBeNew:
		return ClassNew
	}
	if kind == KindRule {
		r.rep.Add(Issue{Path: e.path, Message: fmt.Sprintf("%q has no definition here, in Battle Companies, vanilla or the glossary", e.name), Hint: "write its definition, or check the spelling of the name"})
	} else {
		r.rep.Add(Issue{Path: e.path, Message: fmt.Sprintf("%q is not defined in %s and has no definition or \"extends\" here", e.name, below),
			Hint: "give it a definition or an \"extends\", or check the spelling of the name"})
	}
	return ClassNew
}

// hoist collects the inline rule definitions into the rule index; the same name twice with different text is an error.
func (r *resolver) hoist() {
	seen := map[string]HoistedRule{}
	add := func(name, text, path string) {
		key := NameKey(name)
		if key == "" || text == "" {
			return
		}
		if prev, ok := seen[key]; ok {
			if prev.Definition != text {
				r.rep.Add(Issue{Path: path, Message: fmt.Sprintf("rule %q is defined twice with different text (here and at %s)", name, prev.Path), Hint: "define a rule once, in rules[], and name it where it is used"})
			}
			return
		}
		seen[key] = HoistedRule{Name: name, Definition: text, Path: path}
		r.ix.Hoisted = append(r.ix.Hoisted, seen[key])
	}
	for i, x := range r.doc.Rules {
		add(x.Name, x.Definition, Idx("rules", i))
	}
	for ci := range r.doc.Companies {
		c := &r.doc.Companies[ci]
		cp := Idx("companies", ci)
		for i, x := range c.Rules {
			add(x.Name, x.Definition, Idx(Join(cp, "rules"), i))
		}
		for i, h := range c.HeroUpgrades {
			add(h.Name, h.Definition, Join(Idx(Join(cp, "heroUpgrades"), i), "definition"))
		}
	}
	for pi := range r.doc.Paths {
		p := &r.doc.Paths[pi]
		forEachRuleRef(p.Rows, Idx("paths", pi), func(ref *RuleRef, path string) {
			if ref.Def != nil {
				add(ref.Def.Name, ref.Def.Definition, path)
			}
		})
	}
}

// forEachRuleRef visits the rule references of the option lists of path rows.
func forEachRuleRef(rows []PathRow, path string, fn func(ref *RuleRef, path string)) {
	var walk func(opts []PathOption, p string)
	walk = func(opts []PathOption, p string) {
		for i := range opts {
			op := Idx(p, i)
			if opts[i].Rule != nil {
				fn(opts[i].Rule, Join(op, "rule"))
			}
			if opts[i].MinorRule != nil {
				fn(opts[i].MinorRule, Join(op, "minorRule"))
			}
			walk(opts[i].OneOf, Join(op, "oneOf"))
		}
	}
	for i := range rows {
		walk(rows[i].Options, Join(Idx(Join(path, "rows"), i), "options"))
	}
}

// ---------------------------------------------------------------- references

// view is the scope a reference is resolved in.
type view struct {
	company string // "" outside a company
}

func (r *resolver) references() {
	d := r.doc
	for i := range d.Units {
		r.unitRefs(&d.Units[i], Idx("units", i), view{})
	}
	for i := range d.Gear {
		r.gearRefs(&d.Gear[i], Idx("gear", i), view{})
	}
	for i := range d.Rules {
		r.ruleRefs(&d.Rules[i], Idx("rules", i))
	}
	for i := range d.Paths {
		r.pathRefs(&d.Paths[i], Idx("paths", i))
	}
	for i := range d.Companies {
		r.companyRefs(&d.Companies[i], Idx("companies", i))
	}
	r.scenarioSelectionRefs()
	r.whoRefs()
}

// found reports whether name resolves for a kind from view v: the company's scope, the shared lists, the lower layers.
func (r *resolver) found(kind Kind, name string, v view) bool {
	if _, ok := r.ix.Lookup(kind, name, v.company); ok {
		return true
	}
	return r.lowerHas(kind, name)
}

// candidates lists every name of a kind visible from v, for the closest match.
func (r *resolver) candidates(kind Kind, v view) []string {
	var out []string
	if v.company != "" {
		for _, e := range r.ix.Entries(kind, v.company) {
			out = append(out, e.Name)
		}
	}
	for _, e := range r.ix.Entries(kind, "") {
		out = append(out, e.Name)
	}
	if r.lower != nil {
		out = append(out, r.lower.Names(kind)...)
	}
	sort.Strings(out)
	return out
}

// known reports whether the lower layers (or the file) know the kind at all; a kind nobody lists cannot be checked.
func (r *resolver) known(kind Kind) bool {
	return r.lower != nil && len(r.lower.Names(kind)) > 0
}

// mustResolve reports an unresolved name with its closest match; where is the phrase "in this company, the shared units or vanilla".
func (r *resolver) mustResolve(kind Kind, name, path string, v view, where, hint string) bool {
	if name == "" || r.found(kind, name, v) {
		return true
	}
	iss := Issue{Path: path, Message: fmt.Sprintf("no %s %q in %s", kind, name, where), Hint: hint}
	iss.Closest = Closest(r.candidates(kind, v), name)
	r.rep.Add(iss)
	return false
}

func (r *resolver) whereUnit(v view) string {
	if v.company != "" {
		return "this company, the shared units or vanilla"
	}
	return "the shared units or vanilla"
}

func (r *resolver) unit(name, path string, v view) {
	r.mustResolve(KindUnit, name, path, v, r.whereUnit(v), "")
}

func (r *resolver) rule(name, path string, v view) {
	if name == "" {
		return
	}
	if _, ok := r.hoistedAs(name); ok {
		return
	}
	r.mustResolve(KindRule, name, path, v, "rules[], vanilla or the glossary", "define it in rules[]")
}

func (r *resolver) hoistedAs(name string) (HoistedRule, bool) {
	key := NameKey(name)
	for _, h := range r.ix.Hoisted {
		if NameKey(h.Name) == key {
			return h, true
		}
	}
	return HoistedRule{}, false
}

// checked resolves a name of a kind the lower layers may not list (keywords, races, powers): when nobody lists the kind and the
// file defines none, there is nothing to check against.
func (r *resolver) checked(kind Kind, name, path string, v view, where, hint string) {
	if !r.known(kind) && len(r.ix.order[scopeKind{"", kind}]) == 0 {
		return
	}
	r.mustResolve(kind, name, path, v, where, hint)
}

// optionOrGear reports a "with" entry that is neither an option of the unit nor a gear item.
func (r *resolver) optionOrGear(name, unit, path string, v view) {
	if name == "" || r.found(KindGear, name, v) {
		return
	}
	opts, free := r.optionsOf(unit, v, 0)
	if free {
		return
	}
	for _, o := range opts {
		if NameKey(o) == NameKey(name) {
			return
		}
	}
	iss := Issue{Path: path, Message: fmt.Sprintf("%q is not an option of %q nor a gear item", name, unit), Hint: "options come from the vanilla profile; items from gear[]"}
	iss.Closest = Closest(append(append([]string(nil), opts...), r.candidates(KindGear, v)...), name)
	r.rep.Add(iss)
}

// optionsOf follows a unit's extends chain to a vanilla unit and returns its options. free is true when the unit is defined here
// with a profile of its own (its options are not listed) or when nothing is known about it.
func (r *resolver) optionsOf(unit string, v view, depth int) (opts []string, free bool) {
	if depth > 16 {
		return nil, true
	}
	if e, ok := r.ix.Lookup(KindUnit, unit, v.company); ok {
		u := r.unitByPath(e.Path)
		if u == nil {
			return nil, true
		}
		if u.Profile != nil || u.Options != nil {
			return nil, true
		}
		base := u.Extends
		if base == "" {
			base = u.Name
		}
		if NameKey(base) == NameKey(u.Name) {
			return r.lower.Options(base), r.lower == nil
		}
		return r.optionsOf(base, v, depth+1)
	}
	if r.lower == nil {
		return nil, true
	}
	return r.lower.Options(unit), false
}

func (r *resolver) unitByPath(path string) *Unit {
	var ci, i int
	if n, _ := fmt.Sscanf(path, "units[%d]", &i); n == 1 && !strings.Contains(path, "companies") {
		return &r.doc.Units[i]
	}
	if n, _ := fmt.Sscanf(path, "companies[%d].units[%d]", &ci, &i); n == 2 {
		return &r.doc.Companies[ci].Units[i]
	}
	return nil
}

func (r *resolver) unitRefs(u *Unit, path string, v view) {
	if u.Extends != "" && NameKey(u.Extends) != NameKey(u.Name) {
		r.mustResolve(KindUnit, u.Extends, Join(path, "extends"), view{company: v.company}, r.whereUnit(v), "")
	} else if u.Extends != "" && !r.found(KindUnit, u.Extends, v) {
		r.mustResolve(KindUnit, u.Extends, Join(path, "extends"), v, r.whereUnit(v), "")
	}
	for i, n := range u.With {
		r.optionOrGear(n, u.Name, Idx(Join(path, "with"), i), v)
	}
	for i, n := range u.AddRules {
		r.rule(n, Idx(Join(path, "addRules"), i), v)
	}
	for i, k := range u.AddKeywords {
		r.checked(KindKeyword, k, Idx(Join(path, "addKeywords"), i), v, "vanilla", "")
	}
}

func (r *resolver) gearRefs(g *Gear, path string, v view) {
	if g.Extends != "" {
		r.mustResolve(KindGear, g.Extends, Join(path, "extends"), v, "the shared gear or vanilla", "")
	}
	for i, n := range g.Grants {
		r.rule(n, Idx(Join(path, "grants"), i), v)
	}
	if g.GrantsForChosenWeapon != nil {
		r.rule(g.GrantsForChosenWeapon.Rule, Join(Join(path, "grantsForChosenWeapon"), "rule"), v)
	}
	if g.Who != nil {
		for i, n := range g.Who.Units {
			r.unit(n, Idx(Join(Join(path, "who"), "units"), i), v)
		}
	}
	r.auraRefs(g.Aura, Join(path, "aura"), v)
}

func (r *resolver) auraRefs(a *Aura, path string, v view) {
	if a == nil {
		return
	}
	for i, k := range a.Keywords {
		r.checked(KindKeyword, k, Idx(Join(path, "keywords"), i), v, "vanilla", "")
	}
}

func (r *resolver) ruleRefs(x *Rule, path string) {
	v := view{}
	if x.Target != nil {
		for i, k := range x.Target.Keywords {
			r.checked(KindKeyword, k, Idx(Join(Join(path, "target"), "keywords"), i), v, "vanilla", "")
		}
		for i, k := range x.Target.BearerKeywords {
			r.checked(KindKeyword, k, Idx(Join(Join(path, "target"), "bearerKeywords"), i), v, "vanilla", "")
		}
	}
	r.auraRefs(x.Aura, Join(path, "aura"), v)
}

func (r *resolver) pathRefs(p *Path, path string) {
	v := view{}
	if p.Extends != "" {
		r.mustResolve(KindPath, p.Extends, Join(path, "extends"), v, "the paths of this file", "")
	}
	if p.Mastery != "" {
		r.rule(p.Mastery, Join(path, "mastery"), v)
	}
	if p.HeroicAction != nil {
		r.rule(*p.HeroicAction, Join(path, "heroicAction"), v)
	}
	for i, n := range p.Spells {
		r.checked(KindPower, n, Idx(Join(path, "spells"), i), v, "the magic powers of this file or vanilla", "")
	}
	forEachRuleRef(p.Rows, path, func(ref *RuleRef, rp string) {
		if ref.Def == nil {
			r.rule(ref.Name, rp, v)
		}
	})
	var walk func(opts []PathOption, p string)
	walk = func(opts []PathOption, base string) {
		for i := range opts {
			op := Idx(base, i)
			if opts[i].Power != "" {
				r.checked(KindPower, opts[i].Power, Join(op, "power"), v, "the magic powers of this file or vanilla", "")
			}
			if opts[i].HeroicAction != "" {
				r.rule(opts[i].HeroicAction, Join(op, "heroicAction"), v)
			}
			walk(opts[i].OneOf, Join(op, "oneOf"))
		}
	}
	for i := range p.Rows {
		walk(p.Rows[i].Options, Join(Idx(Join(path, "rows"), i), "options"))
	}
}

func (r *resolver) companyRefs(c *Company, cp string) {
	v := view{company: c.Name}
	if c.Extends != "" {
		r.mustResolve(KindCompany, c.Extends, Join(cp, "extends"), v, "the companies of this file", "")
	}
	for i := range c.Units {
		r.unitRefs(&c.Units[i], Idx(Join(cp, "units"), i), v)
	}
	for i := range c.Gear {
		r.gearRefs(&c.Gear[i], Idx(Join(cp, "gear"), i), v)
	}
	for i := range c.Rules {
		r.ruleRefs(&c.Rules[i], Idx(Join(cp, "rules"), i))
	}
	if c.Start != nil {
		for i, su := range c.Start.Units {
			sp := Idx(Join(Join(cp, "start"), "units"), i)
			r.unit(su.Unit, Join(sp, "unit"), v)
			for j, w := range su.With {
				r.optionOrGear(w, su.Unit, Idx(Join(sp, "with"), j), v)
			}
		}
	}
	chart := func(rows []ChartRow, rp string) {
		for i, row := range rows {
			p := Idx(rp, i)
			if row.Unit != "" {
				r.unit(row.Unit, Join(p, "unit"), v)
			}
			for j, w := range row.With {
				r.optionOrGear(w, row.Unit, Idx(Join(p, "with"), j), v)
			}
		}
	}
	if c.Reinforcements != nil {
		chart(c.Reinforcements.Rows, Join(Join(cp, "reinforcements"), "rows"))
	}
	if c.Special != nil {
		chart(c.Special.Rows, Join(Join(cp, "special"), "rows"))
	}
	for i, a := range c.Advancements {
		ap := Idx(Join(cp, "advancements"), i)
		r.unit(a.From, Join(ap, "from"), v)
		for j, t := range a.To {
			r.unit(t, Idx(Join(ap, "to"), j), v)
		}
	}
	for i, h := range c.HeroUpgrades {
		if h.Definition == "" {
			r.rule(h.Name, Join(Idx(Join(cp, "heroUpgrades"), i), "name"), v)
		}
	}
	for i, pr := range c.Promotions {
		pp := Idx(Join(cp, "promotions"), i)
		r.unit(pr.From, Join(pp, "from"), v)
		r.unit(pr.Becomes, Join(pp, "becomes"), v)
		for j, k := range pr.Keep {
			r.optionOrGear(k, pr.From, Idx(Join(pp, "keep"), j), v)
		}
		for j, k := range pr.Drop {
			r.optionOrGear(k, pr.From, Idx(Join(pp, "drop"), j), v)
		}
	}
	if c.OwnPath != nil {
		for i, n := range c.OwnPath.Units {
			r.unit(n, Idx(Join(Join(cp, "ownPath"), "units"), i), v)
		}
	}
	if c.Heroes != nil {
		for _, l := range []struct {
			name  string
			names []string
		}{{"allowed", c.Heroes.Allowed}, {"excluded", c.Heroes.Excluded}, {"noStartGain", c.Heroes.NoStartGain}, {"recruitAsHero", c.Heroes.RecruitAsHero}} {
			for i, n := range l.names {
				r.unit(n, Idx(Join(Join(cp, "heroes"), l.name), i), v)
			}
		}
	}
}

func (r *resolver) scenarioSelectionRefs() {
	for ti, t := range r.doc.Campaign.ScenarioSelection.Tables {
		for si, n := range t.Scenarios {
			p := Idx(Join(Idx("campaign.scenarioSelection.tables", ti), "scenarios"), si)
			if _, ok := r.ix.Lookup(KindScenario, n, ""); ok {
				continue
			}
			iss := Issue{Path: p, Message: fmt.Sprintf("no scenario %q in scenarios[]", n), Hint: "define it in scenarios[]"}
			iss.Closest = Closest(r.candidates(KindScenario, view{}), n)
			r.rep.Add(iss)
		}
	}
}

func (r *resolver) whoRefs() {
	// races named by gear: only checked when the lower layers list races
	check := func(g []Gear, base string) {
		for i := range g {
			if g[i].Who == nil {
				continue
			}
			for j, n := range g[i].Who.Race {
				r.checked(KindRace, n, Idx(Join(Join(Idx(base, i), "who"), "race"), j), view{}, "vanilla", "")
			}
		}
	}
	check(r.doc.Gear, "gear")
	for ci := range r.doc.Companies {
		check(r.doc.Companies[ci].Gear, Join(Idx("companies", ci), "gear"))
	}
}
