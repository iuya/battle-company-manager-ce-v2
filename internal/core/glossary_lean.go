package core

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// LeanGlossaryExtras lists the terms the rules files of a campaign define: from the lean Battle Companies document first (it wins over
// the vanilla one), then from the vanilla file, and last the variants of rules that share a display label with another rule. Terms of
// the glossary file itself come first in the glossary and win when a name is the same (BuildGlossaryWith).
func LeanGlossaryExtras(vanilla []byte, d *lean.Document, book *VanillaBook, gl Glossary) []ExtraTerm {
	const src = "Battle Companies"
	var out []ExtraTerm
	add := func(name, category, def, source string, aliases ...string) {
		if name = strings.TrimSpace(name); name != "" && tidy(def) != "" {
			out = append(out, ExtraTerm{Name: name, Category: category, Aliases: aliases, Definition: strings.TrimSpace(def), Source: source})
		}
	}
	type variant struct{ label, def, source string }
	var variants []variant // rules that share a display label with a rule of another key (Keen Sight (Falcon) shows as Keen Sight)

	// the rules; the ones written with a parameter come after, worded as Battle Companies words them
	for _, r := range d.Rules {
		if r.Parameter != nil || r.Definition == "" {
			continue
		}
		if r.Label != "" && r.Label != r.Name { // the app shows the label, never the key
			variants = append(variants, variant{r.Label, r.Definition, r.Source})
			continue
		}
		add(r.Name, "special rules", r.Definition, src)
	}
	for _, r := range d.Rules {
		if r.Parameter == nil || r.Definition == "" {
			continue
		}
		pattern := r.Name + " (X)"
		if r.Parameter.Kind == "rollTarget" {
			pattern = r.Name + " (X+)"
		}
		add(r.Name, "special rules", "Written as "+pattern+".\n\n"+r.Definition, src)
	}

	for _, t := range LeanPathTerms(d, book, gl) {
		if t.Category == "paths" { // the heroic action a path starts with is explained by the file, as Battle Companies words it
			if i := slices.IndexFunc(d.Paths, func(p lean.Path) bool { return p.Name == t.Name }); i >= 0 && d.Paths[i].HeroicAction != nil {
				name := *d.Paths[i].HeroicAction
				if def, ok := heroicActionDefinition(d, gl, name); ok {
					add(name, "heroic actions", def, src)
				}
			}
		}
		out = append(out, t)
	}

	if d.Magic != nil { // a power Battle Companies does not reword says what vanilla says; the sorcerer's range and casting value follow
		for _, name := range lean.SortedKeys(d.Magic.Powers) {
			p := d.Magic.Powers[name]
			var def, duration string
			if vp, ok := book.Power(name); ok {
				def, duration = vp.Definition, vp.Duration
			}
			if casting := leanNumber(p.Casting); casting > 0 {
				rng := "self"
				if inches := leanNumber(p.Range); inches > 0 {
					rng = Inches(float64(inches)) + `"`
				}
				def += fmt.Sprintf("\n\nBattle Companies Sorcerer: casting value %d+, range %s. Duration: %s.", casting, rng, duration)
			}
			add(name, "magic", def, src)
		}
		for _, name := range d.Magic.Listed {
			if vp, ok := book.Power(name); ok {
				add(name, "magic", vp.Definition, src)
			}
		}
	}

	// the gear, in this order: the wargear, the equipment, the creatures, the wanderers
	var gear []ExtraTerm
	for _, t := range LeanGearTerms(d, book) {
		if i := slices.IndexFunc(d.Gear, func(g lean.Gear) bool { return g.Name == t.Name }); i >= 0 && d.Gear[i].PricedAs != "" {
			continue // Elven-made weaponry has no price list of its own
		}
		gear = append(gear, t)
	}
	for _, category := range []string{"wargear", "equipment", "creatures", "wanderers"} {
		for _, t := range gear {
			if t.Category == category {
				out = append(out, t)
			}
		}
	}

	for _, chart := range [][]lean.InjuryRow{d.Campaign.Injuries.Hero, d.Campaign.Injuries.Warrior} {
		for _, in := range chart {
			add(in.Name, "injuries", in.Text, src)
		}
	}

	out = append(out, vanillaTerms(vanilla)...)

	for _, v := range variants { // the variants join the term of their label, whichever file defines it
		i := slices.IndexFunc(out, func(t ExtraTerm) bool { return t.Name == v.label })
		who := v.source
		if who == "" {
			who = "Battle Companies"
		}
		text := "Variant (" + who + "): " + strings.TrimSpace(v.def)
		if i >= 0 {
			out[i].Definition += "\n\n" + text
		} else {
			add(v.label, "special rules", text, "Battle Companies")
		}
	}
	return out
}

// vanillaTerms are the glossary terms of the vanilla file: its special rules, powers, wargear and mounts. Either may be empty.
func vanillaTerms(vanilla []byte) []ExtraTerm {
	if len(vanilla) == 0 {
		return nil
	}
	var doc struct {
		Data struct {
			Keywords      []map[string]json.RawMessage `json:"keywords"`
			MagicalPowers []map[string]json.RawMessage `json:"magicalPowers"`
			Gear          []map[string]json.RawMessage `json:"gear"`
		} `json:"data"`
	}
	if json.Unmarshal(vanilla, &doc) != nil {
		return nil
	}
	const src = "Vanilla"
	var out []ExtraTerm
	add := func(name, category, def string) {
		if name = strings.TrimSpace(name); name != "" && tidy(def) != "" {
			out = append(out, ExtraTerm{Name: name, Category: category, Definition: strings.TrimSpace(def), Source: src})
		}
	}
	for _, k := range doc.Data.Keywords {
		add(rawStr(k, "name"), "special rules", rawStr(k, "definition"))
	}
	for _, p := range doc.Data.MagicalPowers {
		def := rawStr(p, "definition")
		if d := rawStr(p, "duration"); d != "" {
			def += "\n\nDuration: " + d + "."
		}
		add(rawStr(p, "name"), "magic", def)
	}
	for _, g := range doc.Data.Gear {
		name := rawStr(g, "name")
		if strings.HasPrefix(name, "Exchange") || strings.HasPrefix(name, "Replace") {
			continue
		}
		if rawStr(g, "type") == "mount" {
			def := statLine(g)
			if r := rawStrings(g, "specialRules"); len(r) > 0 {
				def += "\n\nSpecial rules: " + strings.Join(r, ", ") + "."
			}
			add(name, "mounts", def)
			continue
		}
		add(name, "wargear", rawStr(g, "definition"))
	}
	return out
}

// heroicActionDefinition is what the file says a starting heroic action does: the rule of that name in rules[], else the paragraphs of the
// glossary's own term.
func heroicActionDefinition(d *lean.Document, gl Glossary, name string) (string, bool) {
	if def, ok := LeanRuleText(d.Rules, nil)(name); ok {
		return def, true
	}
	key := lean.NameKey(name)
	for _, t := range gl.Terms {
		if lean.NameKey(t.Name) == key || slices.ContainsFunc(t.Aliases, func(a string) bool { return lean.NameKey(a) == key }) {
			var paras []string
			for _, para := range t.Paragraphs {
				var words []string
				for _, seg := range para {
					words = append(words, seg.Text)
				}
				paras = append(paras, strings.Join(words, ""))
			}
			return strings.Join(paras, "\n\n"), true
		}
	}
	return "", false
}
