package core

import (
	"slices"
	"strconv"
	"strings"
)

// What the lean format does not store about a path: the picker's tooltip with
// the lists behind it (`offers`), and the text of a row whose words follow from its options. The loaders use these helpers too,
// so a path file that omits them reads exactly as one that spells them out (TestPathOffersAreDerived).

// PathOffers is what taking a path gives a hero.
type PathOffers struct {
	StartAction    string   // the heroic action the hero gets at once
	StartPower     bool     // and a magical power of its choice
	Mastery        string   // its Mastery rule, which improves a heroic action and comes with the signature characteristics
	SpecialRules   []string // its own special rules, the Mastery rule left out, in the order of the chart
	SignatureStats []string
	Choices        []string // what else a level-up can give: minorSpecialRule, heroicAction, magicalPower, improveCastingValue
	Summary        string   // the sentence the path picker shows
}

// PathOfferInput is what the offers are a pure function of: the chart, the starting heroic action and power, and the Mastery rule with
// its signature characteristics. The Description is the summary of a path with no chart (the end of the road).
type PathOfferInput struct {
	Description string
	StartAction string
	StartPower  bool
	Mastery     string
	Signature   []string
	Rows        []ProgRow
}

// flatOptions lists the options of a row with the members of every oneOf in place of the oneOf.
func flatOptions(opts []ProgOption) []ProgOption {
	var out []ProgOption
	for _, o := range opts {
		if o.Type == "oneOf" {
			out = append(out, flatOptions(o.OneOf)...)
			continue
		}
		out = append(out, o)
	}
	return out
}

// RowMastery finds the Mastery rule of a chart and the characteristics that come with it: the rule named "Heroic X Mastery" in a row, and
// the stats that row also improves. It is what a lean file states in `mastery` and `signature`, so the file can be checked against
// its own chart, and what the offers are made from.
func RowMastery(rows []ProgRow) (mastery string, signature []string) {
	for _, r := range rows {
		row := flatOptions(r.Options)
		for _, o := range row {
			if o.Type == "specialRule" && strings.HasPrefix(o.Rule, "Heroic ") && strings.HasSuffix(o.Rule, " Mastery") {
				mastery = o.Rule
				signature = nil
				for _, s := range row {
					if s.Type == "improveStat" {
						signature = append(signature, s.Stats...)
					}
				}
				break
			}
		}
	}
	return mastery, signature
}

var choiceWords = []struct{ key, words string }{
	{"minorSpecialRule", "a Minor Special Rule"}, {"heroicAction", "a new Heroic Action"}, {"magicalPower", "a Magical Power"}, {"improveCastingValue", "a better Casting Value"}}

// listWords writes names as "a", "a and b", "a, b and c".
func listWords(names []string) string {
	if len(names) <= 1 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// DerivePathOffers makes the offers of a path from its chart and the fields it states.
func DerivePathOffers(in PathOfferInput) PathOffers {
	if len(in.Rows) == 0 { // the end of the road has no chart: its description is all there is to say
		return PathOffers{Summary: in.Description}
	}
	o := PathOffers{StartAction: in.StartAction, StartPower: in.StartPower, Mastery: in.Mastery, SignatureStats: in.Signature}
	var all []ProgOption
	for _, r := range in.Rows {
		row := flatOptions(r.Options)
		all = append(all, row...)
		for _, x := range row {
			if x.Type == "specialRule" && x.Rule != in.Mastery && !slices.Contains(o.SpecialRules, x.Rule) {
				o.SpecialRules = append(o.SpecialRules, x.Rule)
			}
		}
	}
	for _, c := range choiceWords {
		if slices.ContainsFunc(all, func(x ProgOption) bool { return x.Type == c.key }) {
			o.Choices = append(o.Choices, c.key)
		}
	}
	var parts []string
	switch {
	case o.StartAction != "":
		s := "Adds the Heroic Action " + o.StartAction
		if o.StartPower {
			s += " and a Magical Power of your choice to the Spell List."
		} else {
			s += "."
		}
		parts = append(parts, s)
	case o.Mastery != "":
		parts = append(parts, "Adds no Heroic Action.")
	}
	if o.Mastery != "" {
		action := strings.TrimSuffix(o.Mastery, " Mastery")
		if o.StartAction != "" {
			parts = append(parts, o.Mastery+" improves "+action+".")
		} else {
			parts = append(parts, o.Mastery+" improves the "+action+" every Hero has.")
		}
	}
	if len(o.SpecialRules) > 0 {
		parts = append(parts, "Path special rules: "+listWords(o.SpecialRules)+".")
	}
	if len(o.SignatureStats) > 0 {
		var labels []string
		for _, s := range o.SignatureStats {
			labels = append(labels, statLabel(s))
		}
		parts = append(parts, "Signature characteristic: "+listWords(labels)+".")
	}
	if len(o.Choices) > 0 {
		var words []string
		for _, c := range o.Choices {
			for _, w := range choiceWords {
				if w.key == c {
					words = append(words, w.words)
				}
			}
		}
		parts = append(parts, "Level-ups can also give "+listWords(words)+".")
	}
	o.Summary = strings.Join(parts, " ")
	return o
}

// StatRowText is the text of a row that only improves one of several characteristics by the same amount, with no ceiling of its own and no
// second result: "Improve Attacks or Wounds by 1." (three or more: "Improve Might, Will, or Fate by 1."). It is empty for any other row.
func StatRowText(opts []ProgOption) string {
	flat := flatOptions(opts)
	var names []string
	for _, o := range flat {
		if o.Type != "improveStat" || len(o.Stats) != 1 || o.Maximum != 0 || o.RollAgain || o.Amount != flat[0].Amount || statTerm(o.Stats[0]) == "" {
			return ""
		}
		names = append(names, statTerm(o.Stats[0]))
	}
	if len(names) == 0 {
		return ""
	}
	var list string
	switch len(names) {
	case 1:
		list = names[0]
	case 2:
		list = names[0] + " or " + names[1]
	default:
		list = strings.Join(names[:len(names)-1], ", ") + ", or " + names[len(names)-1]
	}
	return "Improve " + list + " by " + strconv.Itoa(flat[0].Amount) + "."
}
