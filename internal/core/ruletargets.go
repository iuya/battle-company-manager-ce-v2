package core

import (
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Who a rule works against (docs/unit-keywords.md "Rule targets"): the Battle Companies file classifies every rule whose parameter is a
// keyword (data.ruleTargets). Hatred (Orc) and Ancient Enemies (Elf) work against ENEMY models with that keyword, Terror against every enemy
// model, Fury (X) for friendly models, some rules only for the bearer. The tooltip of such a rule says so ("Works against"), and in a live
// match an enemy unit that a rule of yours names gets a marker. A rule set without the table (a custom ruleset) has no targets: plain
// tooltips and no markers.

// RuleDirection says who a rule acts on.
type RuleDirection string

const (
	TargetEnemyKeyword    RuleDirection = "enemy-keyword"    // enemy models that have the keyword in the brackets (Hatred (X), Ancient Enemies (X))
	TargetEnemyAny        RuleDirection = "enemy-any"        // every enemy model (Terror)
	TargetFriendlyKeyword RuleDirection = "friendly-keyword" // friendly models that have the keyword (Fury (X))
	TargetOwn             RuleDirection = "own"              // the bearer only
)

// RuleTarget is one row of data.ruleTargets: a family of rules written with a keyword in brackets (Hatred (X)), or one rule by name.
type RuleTarget struct {
	Rule      string // "Hatred (X)" for a family, the rule's name otherwise
	Base      string // a family's name without its parameter ("Hatred"); empty for a rule by name
	Direction RuleDirection
	Target    string   // the text of "works against", {X} stands for the keywords
	Keywords  []string // a rule by name: the keywords its text names
	All       bool     // keywordMatch "all": a model needs every keyword; otherwise any of them is enough
	Needs     []string // the rule only works whilst the bearer has one of these keywords
}

// RuleTargets is the table. The zero value (and nil) knows no rule.
type RuleTargets struct {
	names    map[string]RuleTarget // rules by name, folded
	patterns map[string]RuleTarget // families by base name, folded
}

// NewRuleTargets builds the table from its rows.
func NewRuleTargets(rows []RuleTarget) RuleTargets {
	t := RuleTargets{names: map[string]RuleTarget{}, patterns: map[string]RuleTarget{}}
	for _, r := range rows {
		if r.Base != "" {
			t.patterns[foldRule(r.Base)] = r
		} else if r.Rule != "" {
			t.names[foldRule(r.Rule)] = r
		}
	}
	return t
}

// foldRule is a rule name compared without case, quotes and spacing differences.
func foldRule(s string) string {
	s = strings.NewReplacer("\"", "", "\u201c", "", "\u201d", "", "\u2019", "'").Replace(s)
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// Empty says there is no table (a ruleset without data.ruleTargets).
func (t RuleTargets) Empty() bool { return len(t.names)+len(t.patterns) == 0 }

// Rows lists the table, families first, each by name.
func (t RuleTargets) Rows() []RuleTarget {
	var out []RuleTarget
	for _, m := range []map[string]RuleTarget{t.patterns, t.names} {
		var part []RuleTarget
		for _, r := range m {
			part = append(part, r)
		}
		sort.Slice(part, func(i, j int) bool { return part[i].Rule < part[j].Rule })
		out = append(out, part...)
	}
	return out
}

var ruleBrackets = regexp.MustCompile(`\(([^)]*)\)`)

// ParseRuleName splits a rule written with parameters into its name and its keyword alternatives. "Hatred (Goblin)(Orc)(Uruk-Hai)" is
// "Hatred" and three alternatives of one keyword each; one bracket that lists several keywords ("Fury (Mordor, Orc)") is one alternative
// that needs all of them. The words are spelled as the keyword table spells them. A bracket that holds no word, or a number or a distance
// ("Terror (2)", "Harbinger of Evil (12\")") is not a keyword and is skipped.
func ParseRuleName(label string) (string, [][]string) {
	label = strings.TrimSpace(label)
	i := strings.Index(label, "(")
	if i < 0 {
		return label, nil
	}
	var alts [][]string
	for _, m := range ruleBrackets.FindAllStringSubmatch(label[i:], -1) {
		if strings.ContainsAny(m[1], "0123456789\"") {
			continue
		}
		words := normalizeKeywords(splitKeywordList(m[1]))
		if len(words) > 0 {
			alts = append(alts, words)
		}
	}
	return strings.TrimSpace(label[:i]), alts
}

func splitKeywordList(s string) []string {
	s = strings.NewReplacer(" and ", ",", " & ", ",", "+", ",").Replace(s)
	return strings.Split(s, ",")
}

// RuleWorks is what the table says of one rule as it is written on a card.
type RuleWorks struct {
	Works   string     // who it works against or for, for the tooltip ("enemy models with the Orc keyword"); empty for an own rule
	Against [][]string // for a rule that works against enemy models with a keyword: alternatives, a model needs all the words of one
	Needs   []string   // the bearer must have one of these keywords for the rule to work
	Friends bool       // the rule works for friendly models, not against enemy ones
}

// Describe gives what the table says of a rule: the family it belongs to when its brackets name keywords (Hatred (Orc)), else the rule by
// name (Terror, Terror (2)). Nothing for a rule the table does not know and for an "own" rule.
func (t RuleTargets) Describe(label string) RuleWorks {
	name, alts := ParseRuleName(label)
	if row, ok := t.names[foldRule(label)]; ok {
		return describeRow(row, nil)
	}
	if row, ok := t.patterns[foldRule(name)]; ok && len(alts) > 0 {
		return describeRow(row, alts)
	}
	if row, ok := t.names[foldRule(name)]; ok {
		return describeRow(row, nil)
	}
	return RuleWorks{}
}

// describeRow applies a row; alts are the keyword alternatives of a family's brackets, a rule by name has its own keywords.
func describeRow(row RuleTarget, alts [][]string) RuleWorks {
	if row.Direction == TargetOwn {
		return RuleWorks{}
	}
	if alts == nil && len(row.Keywords) > 0 {
		if row.All {
			alts = [][]string{row.Keywords}
		} else {
			for _, w := range row.Keywords {
				alts = append(alts, []string{w})
			}
		}
	}
	out := RuleWorks{Needs: row.Needs, Friends: row.Direction == TargetFriendlyKeyword}
	out.Works = strings.ReplaceAll(row.Target, "{X}", joinAlternatives(alts, row.All))
	if row.Direction == TargetEnemyKeyword {
		out.Against = alts
	}
	return out
}

// joinAlternatives writes the keywords for a sentence: "Goblin, Orc or Uruk-hai" (any), "Mordor and Orc" (all).
func joinAlternatives(alts [][]string, all bool) string {
	var words []string
	for _, a := range alts {
		words = append(words, a...)
	}
	if all || len(alts) == 1 {
		return joinWords(words, "and")
	}
	return joinWords(words, "or")
}

func joinWords(w []string, last string) string {
	switch len(w) {
	case 0:
		return ""
	case 1:
		return w[0]
	}
	return strings.Join(w[:len(w)-1], ", ") + " " + last + " " + w[len(w)-1]
}

// annotateRules sets Works and Against on each rule of the list.
func (t RuleTargets) annotateRules(rules []UnitRule) {
	for i := range rules {
		w := t.Describe(rules[i].Name)
		rules[i].Works, rules[i].Against, rules[i].Needs, rules[i].Friends = w.Works, w.Against, w.Needs, w.Friends
	}
}

// annotateUnit sets Works and Against on the rules of a unit and of its path.
func (t RuleTargets) annotateUnit(u *CompanyUnit) {
	if t.Empty() {
		return
	}
	t.annotateRules(u.Rules)
	if u.Path != nil {
		t.annotateRules(u.Path.Rules)
	}
}

// TargetUnit is a unit of a running match as the rule matching sees it: in the Field, with the keywords it has right now.
type TargetUnit struct {
	ID       int64
	Name     string
	Keywords []string // lower-cased, without the keywords turned off (a dismounted rider has no Cavalry), the mount's apart
	Rules    []UnitRule
}

// NewTargetUnit is the unit as the matching sees it, or false when it takes no part: only a unit still in the Field counts. A rider whose
// mount is lost has the keywords of a model on foot (OnFoot).
func NewTargetUnit(u CompanyUnit, l LiveUnit) (TargetUnit, bool) {
	if l.UnitID == 0 || l.Zone != ZoneField {
		return TargetUnit{}, false
	}
	k := u.Keywords
	if l.HasMount && !l.Mounted {
		k = k.OnFoot()
	}
	t := TargetUnit{ID: u.ID, Name: u.Title()}
	for _, w := range k.Model {
		if !w.Off {
			t.Keywords = append(t.Keywords, strings.ToLower(w.Name))
		}
	}
	for _, r := range slices.Concat(u.Rules, pathRules(u)) {
		if r.Mount == "" { // a rule the mount has is not the rider's
			t.Rules = append(t.Rules, r)
		}
	}
	return t, true
}

func pathRules(u CompanyUnit) []UnitRule {
	if u.Path == nil {
		return nil
	}
	return u.Path.Rules
}

// has says whether the unit has every keyword of one alternative.
func (t TargetUnit) has(alt []string) bool {
	return len(alt) > 0 && !slices.ContainsFunc(alt, func(w string) bool { return !slices.Contains(t.Keywords, strings.ToLower(w)) })
}

// RuleMatch says that a rule of your units names a keyword of an enemy unit.
type RuleMatch struct {
	Rule    string   // the rule as written: "Hatred (Orc)"
	Holders []string // your units in the Field that have it, in the order given
	Keyword string   // the keyword(s) of the enemy unit the rule names: "Orc", "Mordor and Orc"
}

// Text is the sentence of the marker's tooltip.
func (m RuleMatch) Text() string {
	who := joinWords(m.Holders, "and")
	verb := "has"
	if len(m.Holders) > 1 {
		verb = "have"
	}
	return "Your " + who + " " + verb + " " + m.Rule + ". This unit has the " + m.Keyword + " keyword, so the rule works against it."
}

// Label is the marker text: "Hatred (Orc) applies".
func (m RuleMatch) Label() string { return m.Rule + " applies" }

// RuleMatches finds, for each enemy unit, the enemy-keyword rules (Hatred (X), Ancient Enemies (X)) of your units that name one of its
// keywords. Both lists hold only units in the Field (NewTargetUnit). The result is keyed by the enemy unit's ID; a unit nobody's rule
// names has no entry. Rules are listed in the order of first appearance in mine, each with all the units of yours that carry it.
func RuleMatches(mine, theirs []TargetUnit) map[int64][]RuleMatch {
	out := map[int64][]RuleMatch{}
	for _, e := range theirs {
		var found []RuleMatch
		for _, m := range mine {
			for _, r := range m.Rules {
				k := slices.IndexFunc(r.Against, e.has)
				if k < 0 || (len(r.Needs) > 0 && !slices.ContainsFunc(r.Needs, func(w string) bool { return m.has([]string{w}) })) {
					continue
				}
				i := slices.IndexFunc(found, func(f RuleMatch) bool { return strings.EqualFold(f.Rule, r.Name) })
				if i < 0 {
					found = append(found, RuleMatch{Rule: r.Name, Keyword: joinWords(r.Against[k], "and")})
					i = len(found) - 1
				}
				if !slices.Contains(found[i].Holders, m.Name) {
					found[i].Holders = append(found[i].Holders, m.Name)
				}
			}
		}
		if len(found) > 0 {
			out[e.ID] = found
		}
	}
	return out
}
