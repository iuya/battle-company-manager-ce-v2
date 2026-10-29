package core

import (
	"slices"
	"strconv"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// The rules of a lean document read as the typed values the engine uses today (docs/definitions.md).
// They are pure functions of the document and the vanilla book.

// LeanRuleText finds what a rule says: the definition of the entry in rules[], else the vanilla text of the name (an annotation adds
// properties to a vanilla rule and writes no text). It is false for a name no layer defines.
func LeanRuleText(rules []lean.Rule, vanilla *VanillaBook) func(name string) (string, bool) {
	own := map[string]string{}
	for _, r := range rules {
		if r.Definition != "" {
			own[lean.NameKey(r.Name)] = r.Definition
		}
	}
	return func(name string) (string, bool) {
		if d, ok := own[lean.NameKey(name)]; ok {
			return d, true
		}
		if vanilla == nil {
			return "", false
		}
		if d, ok := vanilla.keywords[name]; ok {
			return d, true
		}
		for n, d := range vanilla.keywords {
			if lean.NameKey(n) == lean.NameKey(name) {
				return d, true
			}
		}
		return "", false
	}
}

// LeanRuleTargets is the table of who each rule works against or for: a rule with a keyword parameter is a family ("Hatred (X)"),
// any other rule with a target is one rule by name.
func LeanRuleTargets(rules []lean.Rule) RuleTargets {
	var rows []RuleTarget
	for _, r := range rules {
		t := r.Target
		if t == nil {
			continue
		}
		row := RuleTarget{Direction: RuleDirection(t.AppliesTo), Target: t.Text, All: t.Match == "all"}
		if r.Parameter != nil && r.Parameter.Kind == "keyword" {
			row.Rule, row.Base = r.Name+" (X)", r.Name
		} else {
			row.Rule, row.Keywords, row.Needs = r.Name, t.Keywords, t.BearerKeywords
		}
		rows = append(rows, row)
		if b := r.BareTarget; b != nil { // the same rule written without its bracket works on others (Terror: all enemy models)
			rows = append(rows, RuleTarget{Rule: r.Name, Direction: RuleDirection(b.AppliesTo), Target: b.Text, All: b.Match == "all", Keywords: b.Keywords, Needs: b.BearerKeywords})
		}
	}
	return NewRuleTargets(rows)
}

// LeanMinorRules are the minor special rules a hero may take, in file order, and the ones that need a choice when taken, with the
// text the rule says where X stands for the choice. text is LeanRuleText.
func LeanMinorRules(rules []lean.Rule, text func(string) (string, bool)) ([]string, map[string]ParamRule) {
	var names []string
	var params map[string]ParamRule
	for _, r := range rules {
		if !r.Minor {
			continue
		}
		names = append(names, r.Name)
		if r.Parameter != nil {
			if params == nil {
				params = map[string]ParamRule{}
			}
			d, _ := text(r.Name)
			params[r.Name] = ParamRule{Kind: r.Parameter.Kind, Definition: strings.Join(strings.Fields(d), " ")}
		}
	}
	return names, params
}

// LeanPowers are the powers a Path of the Sorcerer hero can learn: the names of magic.powers in alphabetical order, each with the
// text and duration of the vanilla power, the range and casting value the file gives, and the vanilla variants ("Fury (Orc)").
func LeanPowers(m *lean.Magic, vanilla *VanillaBook) []PowerOffer {
	if m == nil {
		return nil
	}
	var out []PowerOffer
	for _, name := range lean.SortedKeys(m.Powers) {
		p := m.Powers[name]
		o := PowerOffer{Name: name, Range: leanNumber(p.Range), Casting: leanNumber(p.Casting)}
		if vp, ok := vanilla.Power(name); ok {
			o.Duration, o.Definition = vp.Duration, vp.Definition
		}
		for _, v := range vanilla.powerOrder {
			if strings.HasPrefix(v, name+" (") {
				o.Variants = append(o.Variants, v)
			}
		}
		out = append(out, o)
	}
	return out
}

// leanNumber reads "6", "6\"" or "5+" as a number; "self" and anything else is 0.
func leanNumber(s string) int {
	n, _ := strconv.Atoi(strings.TrimRight(strings.TrimSpace(s), `+"`))
	return n
}

// LeanRuleLabels are the names rules are shown under when they differ from the key (Keen Sight (Falcon) is shown as Keen Sight).
func LeanRuleLabels(rules []lean.Rule) map[string]string {
	out := map[string]string{}
	for _, r := range rules {
		if r.Label != "" && r.Label != r.Name {
			out[r.Name] = r.Label
		}
	}
	return out
}

// LeanCasting are the limits of the casting rule: a casting value is never improved below floor, and a power is improved at most max
// times. A file that says nothing has floor 1 and max 1.
func LeanCasting(m *lean.Magic) (floor, max int) {
	if m == nil {
		return 1, 1
	}
	return max0(m.Floor), max0(m.MaxPerPower)
}

func max0(n int) int { return max(1, n) }

// LeanAuraTable is the aura table of a lean document: the kinds and groups once, and one entry for every ring: the rings of items
// first (document order), then those of rules, then those of magical powers (alphabetical). A ring found on a rule that has a
// parameter is a family (its pattern "Name (X)"); its names are its owner's name and aliases. A ring is labelled by its own label,
// else by the name its owner is shown under; replaces names rings by label and becomes their entry ids.
func LeanAuraTable(d *lean.Document) AuraTable {
	var t AuraTable
	if d.Auras != nil {
		for _, k := range d.Auras.Kinds {
			t.Kinds = append(t.Kinds, AuraKindDef{ID: k.ID, Label: k.Label, Colour: k.Colour, RGB: k.RGB, Text: k.Text, Default: k.Default})
		}
		for _, g := range d.Auras.Groups {
			t.Groups = append(t.Groups, AuraGroupDef{ID: g.ID, Label: g.Label, Text: g.Text, Default: g.Default})
		}
	}
	used := map[string]bool{}
	byLabel := map[string][]string{}
	var replaces [][]string // per entry, the labels it replaces, resolved after every id exists
	add := func(typ, owner string, names []string, pattern string, a lean.Aura) {
		label := owner
		if a.Label != "" {
			label = a.Label
		}
		id := lean.Slug(owner) + "-" + strconv.Itoa(a.Radius)
		for n := 2; used[id]; n++ {
			id = lean.Slug(owner) + "-" + strconv.Itoa(a.Radius) + "-" + strconv.Itoa(n)
		}
		used[id] = true
		byLabel[lean.NameKey(label)] = append(byLabel[lean.NameKey(label)], id)
		if a.From != "" && a.From != "rule" {
			typ = a.From
		}
		e := AuraEntry{ID: id, Source: AuraSource{Type: typ, Names: names, Pattern: pattern}, Label: label, MergeLabels: a.MergeLabels,
			Radius: float64(a.Radius), Kind: AuraKind(a.Kind), Affects: a.Affects, When: AuraWhen(a.When), Group: a.Group, LOS: a.LOS, Keywords: a.Keywords,
			Bearer: a.Bearer, Tip: a.Tip, Concept: a.Concept}
		t.Entries = append(t.Entries, newAuraEntry(e))
		replaces = append(replaces, a.Replaces)
	}
	for _, g := range d.Gear {
		for _, a := range g.Auras {
			add("item", g.Name, slices.Concat([]string{g.Name}, g.Aka, a.Items), "", a) // a.Items: the other items that draw the same ring
		}
	}
	for _, r := range d.Rules {
		shown := r.Name
		if r.Label != "" {
			shown = r.Label
		}
		pattern := ""
		if r.Parameter != nil {
			pattern = r.Name + " (X)"
		}
		for _, a := range r.Auras {
			add("rule", shown, slices.Concat([]string{r.Name}, r.Aka), pattern, a)
		}
	}
	if d.Magic != nil {
		for _, name := range lean.SortedKeys(d.Magic.Powers) {
			for _, a := range d.Magic.Powers[name].Auras {
				add("power", name, []string{name}, "", a)
			}
		}
	}
	for i, labels := range replaces {
		for _, l := range labels {
			t.Entries[i].Replaces = append(t.Entries[i].Replaces, byLabel[lean.NameKey(l)]...)
		}
	}
	return t
}
