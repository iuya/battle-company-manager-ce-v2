package core

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// Aura ranges (docs/aura-ranges.md, data.auras of the Battle Companies file, docs/definitions.md): the fixed radii in inches around a
// figure inside which a rule or an item of the bearer works. Pure functions over a built unit and the rules: no repository, no state.

// AuraKind is the colour group of an aura (data.auras.kinds): banner, command, support, enemy or utility.
type AuraKind string

// AuraWhen says when an aura is in force.
type AuraWhen string

const (
	AuraAlways      AuraWhen = "always"      // passive, or a rule of every Hero
	AuraDeclared    AuraWhen = "declared"    // an Active rule, a Heroic Action or a once-per-game rule
	AuraConditional AuraWhen = "conditional" // only after a game event
	AuraCast        AuraWhen = "cast"        // a Magical Power in effect
	AuraTarget      AuraWhen = "target"      // the radius only limits the choice of one model
)

// Groups of data.auras.groups: the switches of the dialog.
const (
	AuraGroupAlways      = "always"
	AuraGroupDeclared    = "declared"
	AuraGroupConditional = "conditional"
	AuraGroupCast        = "cast"
	AuraGroupTarget      = "target"
	AuraGroupLarge       = "large"
	AuraGroupOptional    = "optional"
)

// Per figure and per payload limits of the rings (the helper has its own, larger, hard limits: 6 and 200).
const (
	AurasPerFigure  = 6
	AurasPerPayload = 150
)

// Aura is the ring a figure draws: one or several entries of data.auras merged by kind, radius and tier.
type Aura struct {
	ID       string   // the data.auras entry id; a merged aura carries the first entry's
	Entries  []string // the ids of all the entries merged into it, in table order
	Kind     AuraKind
	Radius   float64  // inches from the edge of the base; 0 < r <= 48
	Affects  string   // "friendly", "enemy", "any", "self"
	When     AuraWhen // the strongest of the merged entries (always, declared, conditional), else the tier (cast, target)
	Group    string   // the dialog switch (data.auras.groups) of the strongest entry
	LOS      bool
	Label    string   // the labels joined with ", "
	Rules    []string // the rules, items, powers and actions of the unit that give it (for the tooltip)
	Tip      string   // the tips of the merged entries joined with a space
	Concept  string   // glossary term for the chip popup: the first entry's
	Keywords []string // the affected models' keywords, kept only when every merged entry has one
}

// AuraKindDef is a data.auras.kinds row.
type AuraKindDef struct {
	ID, Label, Colour, RGB, Text string
	Default                      bool
}

// AuraGroupDef is a data.auras.groups row.
type AuraGroupDef struct {
	ID, Label, Text string
	Default         bool
}

// AuraSource says how an entry is found on a unit (data.auras.entries[].source).
type AuraSource struct {
	Type    string   // item, rule, power, heroicAction or hero
	Names   []string // the names it goes by
	Pattern string   // a parameterised family: "Fury (X)"
}

// AuraEntry is a row of data.auras.entries.
type AuraEntry struct {
	ID          string
	Source      AuraSource
	Label       string
	MergeLabels []string
	Radius      float64
	Kind        AuraKind
	Affects     string
	When        AuraWhen
	Group       string
	LOS         bool
	Keywords    []string
	Bearer      string // "Hero": only a Hero bearer gets the entry
	Replaces    []string
	Tip         string
	Concept     string

	keys   map[string]bool // ItemKey of the names
	family string          // ItemKey of the pattern up to its bracket
}

// AuraTable is data.auras: empty (no rings, no error) for a ruleset that has none.
type AuraTable struct {
	Kinds   []AuraKindDef
	Groups  []AuraGroupDef
	Entries []AuraEntry
}

// Empty says there are no aura entries.
func (t AuraTable) Empty() bool { return len(t.Entries) == 0 }

// ParseAuras reads data.auras of a Battle Companies file (empty when it has none).
func ParseAuras(bc []byte) (AuraTable, error) {
	var doc struct {
		Data struct {
			Auras *struct {
				Kinds []struct {
					ID      string `json:"id"`
					Label   string `json:"label"`
					Colour  string `json:"colour"`
					RGB     string `json:"rgb"`
					Text    string `json:"text"`
					Default bool   `json:"default"`
				} `json:"kinds"`
				Groups []struct {
					ID      string `json:"id"`
					Label   string `json:"label"`
					Text    string `json:"text"`
					Default bool   `json:"default"`
				} `json:"groups"`
				Entries []struct {
					ID     string `json:"id"`
					Source struct {
						Type    string   `json:"type"`
						Names   []string `json:"names"`
						Pattern string   `json:"pattern"`
					} `json:"source"`
					Label       string   `json:"label"`
					MergeLabels []string `json:"mergeLabels"`
					Radius      float64  `json:"radius"`
					Kind        string   `json:"kind"`
					Affects     string   `json:"affects"`
					When        string   `json:"when"`
					Group       string   `json:"group"`
					LOS         bool     `json:"los"`
					Keywords    []string `json:"keywords"`
					Bearer      string   `json:"bearer"`
					Replaces    []string `json:"replaces"`
					Tip         string   `json:"tip"`
					Concept     string   `json:"concept"`
				} `json:"entries"`
			} `json:"auras"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bc, &doc); err != nil {
		return AuraTable{}, err
	}
	var t AuraTable
	a := doc.Data.Auras
	if a == nil {
		return t, nil
	}
	for _, k := range a.Kinds {
		t.Kinds = append(t.Kinds, AuraKindDef{ID: k.ID, Label: k.Label, Colour: k.Colour, RGB: k.RGB, Text: k.Text, Default: k.Default})
	}
	for _, g := range a.Groups {
		t.Groups = append(t.Groups, AuraGroupDef{ID: g.ID, Label: g.Label, Text: g.Text, Default: g.Default})
	}
	for _, e := range a.Entries {
		if e.Radius <= 0 {
			return AuraTable{}, fmt.Errorf("data.auras: entry %q has no radius", e.ID)
		}
		x := AuraEntry{ID: e.ID, Source: AuraSource{Type: e.Source.Type, Names: e.Source.Names, Pattern: e.Source.Pattern}, Label: e.Label, MergeLabels: e.MergeLabels,
			Radius: e.Radius, Kind: AuraKind(e.Kind), Affects: e.Affects, When: AuraWhen(e.When), Group: e.Group, LOS: e.LOS, Keywords: e.Keywords, Bearer: e.Bearer,
			Replaces: e.Replaces, Tip: e.Tip, Concept: e.Concept, keys: map[string]bool{}}
		if x.Group == "" {
			x.Group = string(x.When)
		}
		for _, n := range e.Source.Names {
			x.keys[ItemKey(n)] = true
		}
		if p, _, ok := strings.Cut(e.Source.Pattern, "("); ok {
			x.family = ItemKey(p)
		}
		t.Entries = append(t.Entries, x)
	}
	return t, nil
}

// Auras is the aura table of the rules.
func (b *BCBook) Auras() AuraTable { return b.auras }

// auraMatch is an entry found on a unit and the name of the rule, item, power or action of the unit that gave it.
type auraMatch struct {
	e    AuraEntry
	from string
}

// unitFacts are the names a unit can be matched by.
type unitFacts struct {
	items   map[string]string // ItemKey -> the name shown
	rules   map[string]string
	powers  map[string]string
	actions map[string]string
	hero    bool
}

func addFact(m map[string]string, name string) {
	if k := ItemKey(name); k != "" {
		if _, ok := m[k]; !ok {
			m[k] = name
		}
	}
}

func (r *RuleSet) unitFacts(u CompanyUnit) unitFacts {
	f := unitFacts{items: map[string]string{}, rules: map[string]string{}, powers: map[string]string{}, actions: map[string]string{}}
	for _, e := range u.Equipment {
		if e.Disabled != "" || e.Upgrade {
			continue
		}
		names := append([]string{e.Name}, e.Spellings...)
		names = append(names, OptionItems(e.Name)...)
		for _, n := range slices.Clone(names) {
			if a, ok := r.BC.Annotation(n); ok {
				names = append(names, a.Name)
				names = append(names, a.Aliases...)
			}
		}
		for _, n := range names {
			addFact(f.items, n)
		}
	}
	addRule := func(rule UnitRule) {
		if rule.Mount != "" {
			return
		}
		addFact(f.rules, rule.Shown())
		addFact(f.rules, rule.Name)
	}
	for _, rule := range u.Rules {
		addRule(rule)
		if rule.Spell {
			addFact(f.powers, rule.Shown())
			addFact(f.powers, rule.Name)
		}
	}
	if u.Path != nil {
		for _, rule := range u.Path.Rules {
			addRule(rule)
		}
		for _, p := range u.Path.Powers {
			addFact(f.powers, p.Shown())
			addFact(f.powers, p.Name)
		}
		for _, a := range u.Path.HeroicActions {
			addFact(f.actions, a.Name)
		}
	}
	lostStandFast := slices.ContainsFunc(u.LostAbilities, func(l InjuryLoss) bool { return strings.Contains(ItemKey(l.Ability), "standfast") })
	f.hero = u.IsHero() && !u.Wanderer && !lostStandFast
	return f
}

// find is the first fact that names the entry, "" when none does.
func (e AuraEntry) find(facts map[string]string) (string, bool) {
	for _, n := range e.Source.Names {
		if v, ok := facts[ItemKey(n)]; ok {
			return v, true
		}
	}
	if e.family != "" {
		for k, v := range facts {
			if strings.HasPrefix(k, e.family) && strings.Contains(v, "(") && ItemKey(strings.SplitN(v, "(", 2)[0]) == e.family {
				return v, true
			}
		}
	}
	return "", false
}

// auraMatches are the entries a unit has, in table order, with `replaces` applied.
func (r *RuleSet) auraMatches(u CompanyUnit) []auraMatch {
	t := r.BC.auras
	if t.Empty() {
		return nil
	}
	facts := r.unitFacts(u)
	return t.match(facts, facts.hero)
}

func (t AuraTable) match(f unitFacts, hero bool) []auraMatch {
	var out []auraMatch
	for _, e := range t.Entries {
		var from string
		var ok bool
		switch e.Source.Type {
		case "item":
			from, ok = e.find(f.items)
		case "rule":
			from, ok = e.find(f.rules)
		case "power":
			from, ok = e.find(f.powers)
		case "heroicAction":
			from, ok = e.find(f.actions)
		case "hero":
			from, ok = "Hero", hero
		}
		if !ok {
			continue
		}
		if e.Bearer == "Hero" && !hero {
			continue
		}
		out = append(out, auraMatch{e: e, from: from})
	}
	drop := map[string]bool{}
	for _, m := range out {
		for _, id := range m.e.Replaces {
			drop[id] = true
		}
	}
	return slices.DeleteFunc(out, func(m auraMatch) bool { return drop[m.e.ID] })
}

// UnitAuras are the auras of a unit (the rider, never the mount): unfiltered, merged by kind and radius, sorted by kind then radius. Pure
// over the built unit; it does not depend on the mounted state, nor on the live state.
func (r *RuleSet) UnitAuras(u CompanyUnit) []Aura {
	return r.mergeAuras(r.auraMatches(u))
}

// CreatureAuras are the auras of a creature figure (Keen Sight of a Falcon), from the creature's own rules.
func (r *RuleSet) CreatureAuras(c CompanyCreature) []Aura {
	t := r.BC.auras
	if t.Empty() {
		return nil
	}
	return r.mergeAuras(t.match(creatureFacts(c), false))
}

// AuraSelection is the dialog's switches; the zero value draws nothing.
type AuraSelection struct {
	Draw  bool
	Kinds map[AuraKind]bool // nil: every kind
	// The groups of data.auras.groups; the group "always" is on whenever Draw is.
	Declared, Conditional, Cast, Target, Large, Optional bool
	// Base adds the base radius of the vanilla unit to a figure (payload field b); off by default: the helper measures the figure (docs/aura-ranges.md Q7).
	Base bool
}

// DefaultAuraSelection is the selection the data defaults describe (data.auras kinds and groups; spec section 9).
func (r *RuleSet) DefaultAuraSelection() AuraSelection {
	sel := AuraSelection{Draw: true}
	t := r.BC.auras
	for _, g := range t.Groups {
		switch g.ID {
		case AuraGroupDeclared:
			sel.Declared = g.Default
		case AuraGroupConditional:
			sel.Conditional = g.Default
		case AuraGroupCast:
			sel.Cast = g.Default
		case AuraGroupTarget:
			sel.Target = g.Default
		case AuraGroupLarge:
			sel.Large = g.Default
		case AuraGroupOptional:
			sel.Optional = g.Default
		}
	}
	for _, k := range t.Kinds {
		if !k.Default {
			if sel.Kinds == nil {
				sel.Kinds = map[AuraKind]bool{}
				for _, k2 := range t.Kinds {
					sel.Kinds[AuraKind(k2.ID)] = k2.Default
				}
			}
			break
		}
	}
	return sel
}

// Group says whether the group is on in the selection.
func (s AuraSelection) group(g string) bool {
	switch g {
	case AuraGroupDeclared:
		return s.Declared
	case AuraGroupConditional:
		return s.Conditional
	case AuraGroupCast:
		return s.Cast
	case AuraGroupTarget:
		return s.Target
	case AuraGroupLarge:
		return s.Large
	case AuraGroupOptional:
		return s.Optional
	}
	return true // always, and a group the code does not know
}

func (s AuraSelection) kind(k AuraKind) bool {
	if s.Kinds == nil {
		return true
	}
	return s.Kinds[k]
}

// withGroup is the selection with one group switched.
func (s AuraSelection) withGroup(g string, on bool) AuraSelection {
	switch g {
	case AuraGroupDeclared:
		s.Declared = on
	case AuraGroupConditional:
		s.Conditional = on
	case AuraGroupCast:
		s.Cast = on
	case AuraGroupTarget:
		s.Target = on
	case AuraGroupLarge:
		s.Large = on
	case AuraGroupOptional:
		s.Optional = on
	}
	return s
}

// wants says whether an entry is drawn in the selection: the switch is on, its group is on and its kind is on.
func (s AuraSelection) wants(e AuraEntry, kinds bool) bool {
	return s.Draw && s.group(e.Group) && (!kinds || s.kind(e.Kind))
}

// auraTier is what keeps an entry from merging with another: the always, declared and conditional groups share the tier "base" so a Stand
// Fast ring and a Heroic Action shout of the same radius are one ring; a Magical Power or a targeting range never merges into them.
func auraTier(e AuraEntry) string {
	switch e.Group {
	case AuraGroupAlways, AuraGroupDeclared, AuraGroupConditional:
		return "base"
	}
	return e.Group
}

var auraWhenRank = map[AuraWhen]int{AuraAlways: 0, AuraDeclared: 1, AuraConditional: 2, AuraCast: 3, AuraTarget: 4}

func (r *RuleSet) kindRank(k AuraKind) int {
	for i, d := range r.BC.auras.Kinds {
		if AuraKind(d.ID) == k {
			return i
		}
	}
	return len(r.BC.auras.Kinds)
}

// mergeAuras merges the matches of one figure: per (kind, radius, tier); labels in table order (the label, then its merge labels), the
// strongest `when`, keywords kept only if all have them. Sorted by kind then radius.
func (r *RuleSet) mergeAuras(ms []auraMatch) []Aura {
	type key struct {
		kind   AuraKind
		radius float64
		tier   string
	}
	var order []key
	by := map[key]*Aura{}
	kw := map[key]bool{} // every merged entry so far has keywords
	for _, m := range ms {
		e := m.e
		k := key{e.Kind, e.Radius, auraTier(e)}
		a, ok := by[k]
		if !ok {
			a = &Aura{ID: e.ID, Kind: e.Kind, Radius: e.Radius, Affects: e.Affects, When: e.When, Group: e.Group, LOS: e.LOS, Concept: e.Concept}
			by[k] = a
			order = append(order, k)
			kw[k] = true
		} else {
			if auraWhenRank[e.When] < auraWhenRank[a.When] {
				a.When, a.Group = e.When, e.Group
			}
			a.LOS = a.LOS || e.LOS
			if a.Affects != e.Affects {
				a.Affects = "any"
			}
		}
		for _, l := range append([]string{e.Label}, e.MergeLabels...) {
			if l != "" && !slices.Contains(strings.Split(a.Label, ", "), l) {
				if a.Label != "" {
					a.Label += ", "
				}
				a.Label += l
			}
		}
		a.Entries = append(a.Entries, e.ID)
		if !slices.Contains(a.Rules, m.from) {
			a.Rules = append(a.Rules, m.from)
		}
		if e.Tip != "" {
			if a.Tip != "" {
				a.Tip += " "
			}
			a.Tip += e.Tip
		}
		if len(e.Keywords) == 0 {
			kw[k] = false
		} else if kw[k] {
			for _, w := range e.Keywords {
				if !slices.Contains(a.Keywords, w) {
					a.Keywords = append(a.Keywords, w)
				}
			}
		}
	}
	var out []Aura
	for _, k := range order {
		a := *by[k]
		if !kw[k] {
			a.Keywords = nil
		}
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if ki, kj := r.kindRank(out[i].Kind), r.kindRank(out[j].Kind); ki != kj {
			return ki < kj
		}
		return out[i].Radius < out[j].Radius
	})
	return out
}

// selectedAuras are the auras of a unit or creature under a selection: the entries the selection wants, merged. replaces is applied first
// (a Voice of Command removes the 6" command ring even when it is the only one of its group that is on).
func (r *RuleSet) selectedAuras(ms []auraMatch, sel AuraSelection) []Aura {
	var keep []auraMatch
	for _, m := range ms {
		if sel.wants(m.e, true) {
			keep = append(keep, m)
		}
	}
	return r.mergeAuras(keep)
}

// AuraKindSummary is a kind of the dialog: what it is, and the rings it has in the company.
type AuraKindSummary struct {
	Kind             AuraKind
	Label, Text, RGB string
	Colour           string
	Rings            int       // rings of the kind in the selection's groups (whether or not the kind itself is on)
	Radii            []float64 // their distinct radii, ascending
	On               bool      // the kind is on in the selection
}

// AuraSummary is what the dialog shows for a company and a selection.
type AuraSummary struct {
	Kinds   []AuraKindSummary // only the kinds the company has (counted before the limits)
	Rings   int               // rings in the payload
	Figures int               // figures with at least one ring
	LeftOut int               // rings left out by the limits
	// Counts (before the limits): how many rings each switch (declared, conditional, cast, target, large, optional) adds when turned on, or removes when it is on.
	Counts map[string]int
	Notes  []TTSNote
}

// CompanyAuraSummary is the summary of a company's own list (every unit once, rider and on-foot version as the list has them).
func (r *RuleSet) CompanyAuraSummary(d CompanyDetail, sel AuraSelection) AuraSummary {
	return r.AuraSummaryFor(d, r.TTSList(d, TTSOptions{FootVersions: true, MountFigures: true}), sel)
}

// AuraSummaryFor is the summary for the figures of exp (the list the player spawns, a live list included: fallen units have no rings).
func (r *RuleSet) AuraSummaryFor(d CompanyDetail, exp TTSExport, sel AuraSelection) AuraSummary {
	s := AuraSummary{Counts: map[string]int{}}
	rings, notes := r.figureRings(d, exp, sel)
	s.Notes = notes.notes
	s.LeftOut = notes.leftOut
	for _, f := range rings {
		if len(f.Rings) > 0 {
			s.Figures++
			s.Rings += len(f.Rings)
		}
	}
	// the kinds the company has: all groups on, all kinds on
	all := AuraSelection{Draw: true, Declared: true, Conditional: true, Cast: true, Target: true, Large: true, Optional: true}
	have, _ := r.figureRingsCap(d, exp, all, false)
	present := map[AuraKind]bool{}
	for _, f := range have {
		for _, a := range f.Rings {
			present[a.Kind] = true
		}
	}
	groupsOnly := sel
	groupsOnly.Kinds = nil
	byKind, _ := r.figureRingsCap(d, exp, groupsOnly, false)
	for _, k := range r.BC.auras.Kinds {
		kind := AuraKind(k.ID)
		if !present[kind] {
			continue
		}
		ks := AuraKindSummary{Kind: kind, Label: k.Label, Text: k.Text, RGB: k.RGB, Colour: k.Colour, On: sel.kind(kind)}
		for _, f := range byKind {
			for _, a := range f.Rings {
				if a.Kind == kind {
					ks.Rings++
					if !slices.Contains(ks.Radii, a.Radius) {
						ks.Radii = append(ks.Radii, a.Radius)
					}
				}
			}
		}
		slices.Sort(ks.Radii)
		s.Kinds = append(s.Kinds, ks)
	}
	count := func(x AuraSelection) int {
		n := 0
		rs, _ := r.figureRingsCap(d, exp, x, false)
		for _, f := range rs {
			n += len(f.Rings)
		}
		return n
	}
	if sel.Draw {
		for _, g := range []string{AuraGroupDeclared, AuraGroupConditional, AuraGroupCast, AuraGroupTarget, AuraGroupLarge, AuraGroupOptional} {
			s.Counts[g] = count(sel.withGroup(g, true)) - count(sel.withGroup(g, false))
		}
	}
	return s
}
