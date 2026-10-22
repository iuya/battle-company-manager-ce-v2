package core

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// ExtraTerm is a glossary entry made from a campaign's rules files (the vanilla file and the Battle Companies file): special
// rules, magical powers, wargear, equipment, paths, creatures... Terms of the glossary file itself come first and win when a
// name is the same.
type ExtraTerm struct {
	Name       string
	Category   string
	Aliases    []string
	Definition string
	Source     string    // the file it comes from, shown on the term
	Path       *PathData // for a path: its limits and its 2D6 chart, shown as tables
	// Weak: the term is only added when the glossary file does not already hold the name or an alias. Used for gear, where the
	// rulebook's own entry (Spear, Bow, Crossbow...) is the better explanation and a Battle Companies price list must not replace it.
	Weak bool
}

// PathData is a path as the glossary shows it: the limits of its heroes and the results of its 2D6 chart.
type PathData struct {
	Limits []PathLimit
	Rows   []PathChartRow
}

// PathLimit is the limit of one characteristic: "Fight", "+3 / 9".
type PathLimit struct{ Stat, Abbr, Term, Limit string } // Term is the glossary term of the characteristic

// PathChartRow is a result of the chart.
type PathChartRow struct {
	Roll int
	Text string
}

var statOrder = []struct{ key, label, abbr, term string }{{"movement", "Move", "Mv", "Move Value"}, {"fight", "Fight", "FV", "Fight Value"}, {"shoot", "Shoot", "SV", "Shoot Value"},
	{"strength", "Strength", "S", "Strength"}, {"defence", "Defence", "D", "Defence"}, {"attack", "Attacks", "A", "Attacks"}, {"wounds", "Wounds", "W", "Wounds"},
	{"courage", "Courage", "C", "Courage"}, {"intelligence", "Intelligence", "I", "Intelligence"}, {"might", "Might", "Might", "Might"}, {"will", "Will", "Will", "Will"}, {"fate", "Fate", "Fate", "Fate"}}

// statLine writes a profile in words: "Move 8, Fight 2, Strength 3...".
func statLine(raw map[string]json.RawMessage) string {
	var parts []string
	for _, s := range statOrder {
		var v int
		if raw[s.key] != nil && json.Unmarshal(raw[s.key], &v) == nil && (v != 0 || s.key == "attack") {
			parts = append(parts, fmt.Sprintf("%s %d", s.label, v))
		}
	}
	return strings.Join(parts, ", ")
}

func rawStr(raw map[string]json.RawMessage, key string) (s string) {
	_ = json.Unmarshal(raw[key], &s)
	return
}
func rawStrings(raw map[string]json.RawMessage, key string) (l []string) {
	_ = json.Unmarshal(raw[key], &l)
	return
}
func rawInt(raw map[string]json.RawMessage, key string) (n int) {
	_ = json.Unmarshal(raw[key], &n)
	return
}

func tidy(s string) string { return strings.Join(strings.Fields(s), " ") }

// GlossaryExtras lists the terms the rules files of a campaign define: from the Battle Companies file first (it wins over the
// vanilla one) and then from the vanilla file. Either may be empty.
func GlossaryExtras(vanilla, bc []byte) []ExtraTerm {
	var out []ExtraTerm
	type variant struct{ label, def, source string }
	var variants []variant // rules that share a display label with a rule of another key (Keen Sight (Falcon) shows as Keen Sight)
	add := func(name, category, def, source string, aliases ...string) {
		if name = strings.TrimSpace(name); name != "" && tidy(def) != "" {
			out = append(out, ExtraTerm{Name: name, Category: category, Aliases: aliases, Definition: strings.TrimSpace(def), Source: source})
		}
	}
	if len(bc) > 0 {
		const src = "Battle Companies"
		var doc struct {
			Campaign struct {
				Injuries struct {
					Hero    []map[string]json.RawMessage `json:"hero"`
					Warrior []map[string]json.RawMessage `json:"warrior"`
				} `json:"injuries"`
			} `json:"campaign"`
			Data struct {
				Keywords []map[string]json.RawMessage `json:"keywords"`
				Paths    []struct {
					Name        string `json:"name"`
					Description string `json:"description"`
					StartNote   string `json:"startingNote"`
					StartAction *struct {
						Name       string `json:"name"`
						Definition string `json:"definition"`
					} `json:"startingHeroicAction"`
					Maximums    map[string]map[string]int `json:"maximums"`
					Progression []struct {
						Roll    int    `json:"roll"`
						Text    string `json:"text"`
						Options []struct {
							Type string `json:"type"`
							Rule string `json:"specialRule"`
						} `json:"options"`
					} `json:"progression"`
				} `json:"paths"`
				MagicalPowers struct {
					Items []map[string]json.RawMessage `json:"items"`
				} `json:"magicalPowers"`
				Parameterised []map[string]json.RawMessage                 `json:"parameterisedRules"`
				Wargear       struct{ Items []map[string]json.RawMessage } `json:"wargear"`
				Equipment     struct{ Items []map[string]json.RawMessage } `json:"equipment"`
				Creatures     struct{ Items []map[string]json.RawMessage } `json:"creatures"`
				Wanderers     struct{ Items []map[string]json.RawMessage } `json:"wanderers"`
				GearNotes     struct {
					Items []map[string]json.RawMessage `json:"items"`
				} `json:"gearAnnotations"`
			} `json:"data"`
		}
		if json.Unmarshal(bc, &doc) == nil {
			for _, k := range doc.Data.Keywords {
				if label := rawStr(k, "label"); label != "" && label != rawStr(k, "name") { // the app shows the label, never the key
					variants = append(variants, variant{label, rawStr(k, "definition"), rawStr(k, "source")})
					continue
				}
				add(rawStr(k, "name"), "special rules", rawStr(k, "definition"), src)
			}
			for _, pr := range doc.Data.Parameterised { // rules written with a parameter, as Battle Companies words them
				if pattern := rawStr(pr, "pattern"); pattern != "" && rawStr(pr, "definition") != "" {
					add(rawStr(pr, "name"), "special rules", "Written as "+pattern+".\n\n"+rawStr(pr, "definition"), src)
				}
			}
			for _, p := range doc.Data.Paths {
				def := strings.TrimSpace(p.Description)
				if def == "" { // some paths have no description of their own
					def = strings.TrimSpace(p.StartNote)
				}
				if def == "" {
					def = "A heroic path of Battle Companies: the chart below is what a hero of this path rolls on when it levels up."
				}
				if p.StartAction != nil {
					def += "\n\nStarting heroic action: " + p.StartAction.Name + "."
					add(p.StartAction.Name, "heroic actions", p.StartAction.Definition, src)
				}
				data := &PathData{Limits: pathLimits(p.Maximums)}
				for _, row := range p.Progression {
					data.Rows = append(data.Rows, PathChartRow{Roll: row.Roll, Text: tidy(row.Text)})
				}
				add(p.Name, "paths", def, src)
				if n := len(out); n > 0 && out[n-1].Name == p.Name {
					out[n-1].Path = data
				}
				for _, row := range p.Progression {
					for _, o := range row.Options {
						if o.Type == "specialRule" && o.Rule != "" {
							text := row.Text
							if _, after, ok := strings.Cut(text, " - "); ok {
								text = after
							}
							add(o.Rule, "path rules", text+"\n\nGained by heroes of the "+p.Name+".", src)
						}
					}
				}
			}
			for _, pw := range doc.Data.MagicalPowers.Items {
				def := rawStr(pw, "definition")
				if rawInt(pw, "casting") > 0 {
					rng := "self"
					var inches int
					if json.Unmarshal(pw["range"], &inches) == nil && inches > 0 {
						rng = fmt.Sprintf("%d\"", inches)
					}
					def += fmt.Sprintf("\n\nBattle Companies Sorcerer: casting value %d+, range %s. Duration: %s.", rawInt(pw, "casting"), rng, rawStr(pw, "duration"))
				}
				add(rawStr(pw, "name"), "magic", def, src)
			}
			type note struct {
				name    string
				aliases []string
				def     string
			}
			notes := map[string]note{} // by lower-cased name or alias: what each item says on its own
			for _, a := range doc.Data.GearNotes.Items {
				n := note{name: rawStr(a, "name"), aliases: rawStrings(a, "aliases"), def: tidy(rawStr(a, "definition"))}
				for _, k := range append([]string{n.name}, n.aliases...) {
					if _, seen := notes[strings.ToLower(k)]; !seen {
						notes[strings.ToLower(k)] = n
					}
				}
			}
			// gearTerm adds ONE term for ONE item. A row of the wargear table is a price group, not an item: its names are
			// separate items with their own texts, so the caller calls it once per name.
			gearTerm := func(it map[string]json.RawMessage, name string, category string, weak bool) {
				if name == "" {
					return
				}
				def, aliases := "", []string(nil)
				if n, ok := notes[strings.ToLower(name)]; ok {
					def, aliases, name = n.def, n.aliases, n.name
				}
				if def == "" {
					def = tidy(rawStr(it, "definition"))
				}
				var price []string
				if n := rawInt(it, "influence"); n > 0 {
					price = append(price, fmt.Sprintf("%d influence", n))
				}
				var gold struct{ Warrior, Hero int }
				if json.Unmarshal(it["gold"], &gold) == nil && gold.Warrior+gold.Hero > 0 {
					price = append(price, fmt.Sprintf("%d Gold for a warrior, %d for a hero", gold.Warrior, gold.Hero))
				}
				extra := ""
				if size := rawStr(it, "size"); size != "" {
					extra = fmt.Sprintf(" It takes a %s equipment slot.", size)
				}
				if len(price) > 0 {
					extra = " Battle Companies armory: " + strings.Join(price, " or ") + "." + extra
				}
				if def == "" && extra == "" {
					return
				}
				add(name, category, strings.TrimSpace(def+"\n\n"+strings.TrimSpace(extra)), src, aliases...)
				if weak && len(out) > 0 && out[len(out)-1].Name == name {
					out[len(out)-1].Weak = true
				}
			}
			for _, it := range doc.Data.Wargear.Items {
				for _, n := range rawStrings(it, "names") {
					gearTerm(it, n, "wargear", true)
				}
			}
			for _, it := range doc.Data.Equipment.Items {
				gearTerm(it, rawStr(it, "name"), "equipment", false)
			}
			for _, c := range doc.Data.Creatures.Items {
				def := statLine(c)
				if r := rawStrings(c, "specialRules"); len(r) > 0 {
					def += "\n\nSpecial rules: " + strings.Join(r, ", ") + "."
				}
				def += fmt.Sprintf("\n\nA creature for a leader or a sergeant. It costs %d influence and is worth %d points.", rawInt(c, "influence"), rawInt(c, "points"))
				for _, n := range rawStrings(c, "notes") {
					def += "\n\n" + n
				}
				add(rawStr(c, "name"), "creatures", def, src)
			}
			for _, w := range doc.Data.Wanderers.Items {
				def := statLine(w)
				if a := rawStrings(w, "wargear"); len(a) > 0 {
					def += "\n\nWargear: " + strings.Join(a, ", ") + "."
				}
				if a := rawStrings(w, "heroicActions"); len(a) > 0 {
					def += "\n\nHeroic actions: " + strings.Join(a, ", ") + "."
				}
				if r := rawStrings(w, "specialRules"); len(r) > 0 {
					def += "\n\nSpecial rules: " + strings.Join(r, ", ") + "."
				}
				def += fmt.Sprintf("\n\nA mercenary hero for a company. It costs %d influence and is worth %d points.", rawInt(w, "influence"), rawInt(w, "points"))
				add(rawStr(w, "name"), "wanderers", def, src)
			}
			for _, chart := range [][]map[string]json.RawMessage{doc.Campaign.Injuries.Hero, doc.Campaign.Injuries.Warrior} {
				for _, in := range chart {
					add(rawStr(in, "name"), "injuries", rawStr(in, "text"), src)
				}
			}
		}
	}
	if len(vanilla) > 0 {
		const src = "Vanilla"
		var doc struct {
			Data struct {
				Keywords      []map[string]json.RawMessage `json:"keywords"`
				MagicalPowers []map[string]json.RawMessage `json:"magicalPowers"`
				Gear          []map[string]json.RawMessage `json:"gear"`
			} `json:"data"`
		}
		if json.Unmarshal(vanilla, &doc) == nil {
			for _, k := range doc.Data.Keywords {
				add(rawStr(k, "name"), "special rules", rawStr(k, "definition"), src)
			}
			for _, p := range doc.Data.MagicalPowers {
				def := rawStr(p, "definition")
				if d := rawStr(p, "duration"); d != "" {
					def += "\n\nDuration: " + d + "."
				}
				add(rawStr(p, "name"), "magic", def, src)
			}
			for _, g := range doc.Data.Gear {
				name := rawStr(g, "name")
				if strings.HasPrefix(name, "Exchange") || strings.HasPrefix(name, "Replace") {
					continue
				}
				category := "wargear"
				if rawStr(g, "type") == "mount" {
					category = "mounts"
					def := statLine(g)
					if r := rawStrings(g, "specialRules"); len(r) > 0 {
						def += "\n\nSpecial rules: " + strings.Join(r, ", ") + "."
					}
					add(name, category, def, src)
					continue
				}
				add(name, category, rawStr(g, "definition"), src)
			}
		}
	}
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

// pathLimits lists a path's characteristic limits in the order of a profile: "+3" above the starting profile, "9" at most,
// "3+" at best (where a low number is better), joined when a stat has more than one.
func pathLimits(max map[string]map[string]int) []PathLimit {
	var out []PathLimit
	for _, s := range statOrder {
		l, ok := max[s.key]
		if !ok {
			continue
		}
		var bits []string
		if v, ok := l["maxIncrease"]; ok {
			bits = append(bits, fmt.Sprintf("(+%d)", v)) // a number of increases, not a total
		}
		if v, ok := l["max"]; ok {
			bits = append(bits, fmt.Sprint(v))
		}
		if v, ok := l["best"]; ok {
			bits = append(bits, fmt.Sprintf("%d+", v))
		}
		if len(bits) > 0 {
			out = append(out, PathLimit{Stat: s.label, Abbr: s.abbr, Term: s.term, Limit: strings.Join(bits, " / ")})
		}
	}
	return out
}
