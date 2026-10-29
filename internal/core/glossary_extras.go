package core

import (
	"encoding/json"
	"fmt"
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

// PathData is a path as the glossary shows it: the limits of its heroes and the results of its level-up chart.
type PathData struct {
	Limits   []PathLimit
	Rows     []PathChartRow
	PathDice DiceSpec // the dice the chart is rolled with (paths[].dice, else progression.hero.roll); zero when the file names none
}

// PathLimit is the limit of one characteristic: "Fight", "+3 / 9".
type PathLimit struct{ Stat, Abbr, Term, Limit string } // Term is the glossary term of the characteristic

// PathChartRow is a result of the chart.
type PathChartRow struct {
	Roll int
	Text string
}

// statPhrases is the one table of how the pages word each characteristic, keyed by the vanilla stat key and in the vanilla order (a test
// checks both against the vanilla file): Label is the short name ("Move"), Abbr the column head, Term how a chart row says it ("Move Value").
var statPhrases = []struct{ key, label, abbr, term string }{{"movement", "Move", "Mv", "Move Value"}, {"fight", "Fight", "FV", "Fight Value"}, {"shoot", "Shoot", "SV", "Shoot Value"},
	{"strength", "Strength", "S", "Strength"}, {"defence", "Defence", "D", "Defence"}, {"attack", "Attacks", "A", "Attacks"}, {"wounds", "Wounds", "W", "Wounds"},
	{"courage", "Courage", "C", "Courage"}, {"intelligence", "Intelligence", "I", "Intelligence"}, {"might", "Might", "Might", "Might"}, {"will", "Will", "Will", "Will"}, {"fate", "Fate", "Fate", "Fate"}}

// statLabel and statTerm are the short name and the chart row name of a characteristic; empty for a key the table does not know.
func statLabel(key string) string { return statPhrase(key).label }
func statTerm(key string) string  { return statPhrase(key).term }

func statPhrase(key string) (p struct{ key, label, abbr, term string }) {
	for _, s := range statPhrases {
		if s.key == key {
			return s
		}
	}
	return p
}

// statLine writes a profile in words: "Move 8, Fight 2, Strength 3...".
func statLine(raw map[string]json.RawMessage) string {
	var parts []string
	for _, s := range statPhrases {
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

// gearTermText is the text of the glossary term of a piece of gear: what it says, then its price in the Battle Companies armoury and the
// slot it takes. It is false when there is nothing to say.
func gearTermText(def string, influence int, gold Rating, size string) (string, bool) {
	var price []string
	if influence > 0 {
		price = append(price, fmt.Sprintf("%d influence", influence))
	}
	if gold.Warrior+gold.Hero > 0 {
		price = append(price, fmt.Sprintf("%d Gold for a warrior, %d for a hero", gold.Warrior, gold.Hero))
	}
	extra := ""
	if size != "" {
		extra = fmt.Sprintf(" It takes a %s equipment slot.", size)
	}
	if len(price) > 0 {
		extra = " Battle Companies armory: " + strings.Join(price, " or ") + "." + extra
	}
	if def == "" && extra == "" {
		return "", false
	}
	return strings.TrimSpace(def + "\n\n" + strings.TrimSpace(extra)), true
}

// creatureTermText is the text of the glossary term of a creature (its raw item: stats, rules, price, notes).
func creatureTermText(c map[string]json.RawMessage) string {
	def := statLine(c)
	if r := rawStrings(c, "specialRules"); len(r) > 0 {
		def += "\n\nSpecial rules: " + strings.Join(r, ", ") + "."
	}
	def += fmt.Sprintf("\n\nA creature for a leader or a sergeant. It costs %d influence and is worth %d points.", rawInt(c, "influence"), rawInt(c, "points"))
	for _, n := range rawStrings(c, "notes") {
		def += "\n\n" + n
	}
	return def
}

// wandererTermText is the text of the glossary term of a wanderer.
func wandererTermText(w map[string]json.RawMessage) string {
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
	return def
}

func tidy(s string) string { return strings.Join(strings.Fields(s), " ") }

// pathLimits lists a path's characteristic limits in the order of a profile: "+3" above the starting profile, "9" at most,
// "3+" at best (where a low number is better), joined when a stat has more than one.
func pathLimits(max map[string]map[string]int) []PathLimit {
	var out []PathLimit
	for _, s := range statPhrases {
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
