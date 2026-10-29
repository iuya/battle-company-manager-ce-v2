package core

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// The paths and the progression of a lean document read as the typed values the engine uses today (docs/definitions.md).
// They are pure functions of the document, the vanilla book and the glossary.

// LeanProgression is the progression rules of a lean document: the thresholds and the warrior's table of campaign.progression, the
// limits of campaign.characteristicLimits, the paths (each with its chart, limits, offers and starting choices), the minor special
// rules and the magical powers a hero can learn. A path that is not heroic is handed out by a company or an option and is not
// for the player to choose: the end of the road keeps only its rule, the custom path is hidden.
func LeanProgression(d *lean.Document, vanilla *VanillaBook, gl Glossary) (ProgressionRules, error) {
	p := d.Campaign.Progression
	rules := ProgressionRules{
		heroStats: vanilla.HeroStats(), LowerIsBetter: map[string]bool{}, Peak: map[string]map[string]int{}, NoPromotionBy: map[string]int{},
		Threshold: p.Threshold, WarriorDice: p.Warrior.Roll, HeroDice: p.Hero.Roll, HeroRolls: p.Hero.RollsPerProgression, HeroChoose: p.Hero.Choose,
		NoPromotionTo: p.Warrior.NoPromotion.Maximum,
	}
	if rules.HeroChoose != 1 { // the choice flow keeps one result
		return ProgressionRules{}, errors.Join(ErrInvalidRuleset, fmt.Errorf("campaign.progression.hero.choose: %d is not supported, a hero keeps one result", rules.HeroChoose))
	}
	rules.NoPromotionBy["courage"], rules.NoPromotionBy["intelligence"] = p.Warrior.NoPromotion.Courage, p.Warrior.NoPromotion.Intelligence
	for _, w := range p.Warrior.Table {
		rules.Warrior = append(rules.Warrior, WarriorResult{Min: w.Roll.Lo, Max: w.Roll.Hi, Key: w.Key, Name: w.Name, Text: w.Text})
	}
	for _, stat := range []string{"courage", "intelligence"} {
		if rules.NoPromotionBy[stat] > 0 {
			rules.NoPromotion = append(rules.NoPromotion, stat)
		}
	}
	if err := leanLimits(d.Campaign.CharacteristicLimits, &rules); err != nil {
		return ProgressionRules{}, err
	}

	text := LeanRuleText(d.Rules, vanilla)
	tags := leanTags(d.Rules)
	for _, path := range d.Paths {
		if path.Kind == "endOfPath" {
			if rules.EndOfPath == nil {
				rules.EndOfPath = map[string]string{}
			}
			rules.EndOfPath[path.Name] = path.Rule
		}
		hidden := path.Kind != "" && path.Kind != "heroic"
		if hidden && path.Kind != "custom" {
			continue
		}
		pp := PathProgression{Name: path.Name, Limits: map[string]map[string]int(nil), PathDice: cmp.Or(path.Dice, rules.HeroDice), LimitPerCompany: path.Limit, Description: path.Description}
		if err := leanRoundTrip(path.Maximums, &pp.Limits); err != nil {
			return ProgressionRules{}, fmt.Errorf("paths[%s].maximums: %w", path.Name, err)
		}
		if path.Start != nil {
			pp.Start = map[string]int{"might": path.Start.Might, "will": path.Start.Will, "fate": path.Start.Fate}
		}
		if path.HeroicAction != nil {
			pp.StartAction = *path.HeroicAction
		}
		pp.StartPower = len(path.Spells) > 0 // a path with a spell list begins with a power of its choice
		for _, row := range path.Rows {
			opts, err := leanOptions(path.Name, row.Options)
			if err != nil {
				return ProgressionRules{}, err
			}
			rowText := LeanRowText(row, opts, text, tags)
			for roll := row.Roll.Lo; roll <= row.Roll.Hi; roll++ {
				pp.Rows = append(pp.Rows, ProgRow{Roll: roll, Text: rowText, Options: joinStatOptions(opts)})
			}
		}
		offers := DerivePathOffers(PathOfferInput{Description: path.Description, StartAction: pp.StartAction, StartPower: pp.StartPower, Mastery: path.Mastery, Signature: path.Signature, Rows: pp.Rows})
		pp.Summary, pp.Mastery, pp.SpecialRules = offers.Summary, offers.Mastery, offers.SpecialRules
		if hidden {
			rules.Hidden = append(rules.Hidden, pp)
			rules.CustomPath = path.Name
		} else {
			rules.Paths = append(rules.Paths, pp)
		}
	}
	var bad []string
	for name, v := range rules.Peak[willMaxOverridesKey] { // the Will maximum a path has of its own
		switch {
		case !slices.ContainsFunc(d.Paths, func(p lean.Path) bool { return p.Name == name }):
			bad = append(bad, fmt.Sprintf("campaign.characteristicLimits.peak.willMaxOverrides: %q is not a path", name))
		case v < 1 || v > 10:
			bad = append(bad, fmt.Sprintf("campaign.characteristicLimits.peak.willMaxOverrides[%s]: %d is outside 1..10", name, v))
		}
	}
	if len(bad) > 0 {
		slices.Sort(bad)
		return ProgressionRules{}, errors.Join(ErrInvalidRuleset, errors.New(strings.Join(bad, "; ")))
	}
	rules.CastFloor, rules.CastMax = LeanCasting(d.Magic)
	rules.Powers = LeanPowers(d.Magic, vanilla)
	rules.MinorRules, rules.MinorParams = LeanMinorRules(d.Rules, text)
	return rules, nil
}

// leanRoundTrip copies a free value (decoded JSON) into a typed one.
func leanRoundTrip(from, into any) error {
	if from == nil {
		return nil
	}
	b, err := json.Marshal(from)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, into)
}

// leanLimits reads campaign.characteristicLimits: the stats where lower is better, the overall peak of each stat (with the Will maximum
// some paths have of their own) and the cap on the total improvement of Courage and Intelligence.
func leanLimits(free lean.Free, rules *ProgressionRules) error {
	var l struct {
		LowerIsBetter []string                   `json:"lowerIsBetter"`
		Peak          map[string]json.RawMessage `json:"peak"`
		Increases     struct {
			Cap *struct {
				Value        int  `json:"value"`
				IncludesGear bool `json:"includesGear"`
			} `json:"courageIntelligenceTotalCap"`
		} `json:"increases"`
	}
	if err := leanRoundTrip(free, &l); err != nil {
		return errors.Join(ErrInvalidRuleset, fmt.Errorf("campaign.characteristicLimits: %w", err))
	}
	for _, k := range l.LowerIsBetter {
		rules.LowerIsBetter[k] = true
	}
	for stat, raw := range l.Peak {
		var limits map[string]int
		if json.Unmarshal(raw, &limits) == nil && len(limits) > 0 {
			rules.Peak[stat] = limits
		}
	}
	if c := l.Increases.Cap; c != nil {
		if c.Value < 1 || c.Value > 10 {
			return errors.Join(ErrInvalidRuleset, fmt.Errorf("campaign.characteristicLimits.increases.courageIntelligenceTotalCap.value: %d is out of range 1..10", c.Value))
		}
		rules.CITotalCap, rules.CITotalGear = c.Value, c.IncludesGear
	}
	return nil
}

// leanTags are the tags (Active, Passive) of the rules of rules[] by name.
func leanTags(rules []lean.Rule) map[string]string {
	out := map[string]string{}
	for _, r := range rules {
		if r.Tag != "" {
			out[lean.NameKey(r.Name)] = r.Tag
		}
	}
	return out
}

// leanOptions turns the options of a lean row into the engine's: a stat is one option (a result that improves one of several stats lists
// them side by side), a rule is a special rule, and the open choices are the minor special rule, the
// heroic action, the magical power and the casting value. A named minor rule, heroic action or power is not something the engine can
// offer yet and is refused.
func leanOptions(path string, opts []lean.PathOption) ([]ProgOption, error) {
	var out []ProgOption
	for _, o := range opts {
		switch {
		case o.Stat != "":
			out = append(out, ProgOption{Type: "improveStat", Stats: []string{o.Stat}, Amount: max(1, o.Amount), Maximum: o.Max, RollAgain: o.RollAgain})
		case o.Rule != nil:
			out = append(out, ProgOption{Type: "specialRule", Rule: o.Rule.Name, Amount: 1})
		case o.AnyMinorRule:
			out = append(out, ProgOption{Type: "minorSpecialRule", Amount: 1, MaxTotal: o.MaxTotal})
		case o.AnyHeroicAction:
			out = append(out, ProgOption{Type: "heroicAction", Amount: 1, MaxTotal: o.MaxTotal})
		case o.AnyPower:
			out = append(out, ProgOption{Type: "magicalPower", Amount: 1})
		case o.CastingValue != 0:
			out = append(out, ProgOption{Type: "improveCastingValue", Amount: max(1, o.CastingValue)})
		case len(o.OneOf) > 0:
			sub, err := leanOptions(path, o.OneOf)
			if err != nil {
				return nil, err
			}
			out = append(out, ProgOption{Type: "oneOf", Amount: 1, OneOf: sub})
		default:
			return nil, errors.Join(ErrInvalidRuleset, fmt.Errorf("paths[%s]: a row option names a minor rule, a heroic action or a power; the engine offers the open choices anyMinorRule, anyHeroicAction and anyPower", path))
		}
	}
	return out, nil
}

// joinStatOptions folds the stats a row lists side by side (same amount, ceiling and re-roll) into one option that improves one of them,
// the form the level-up flow and the pages read, also inside a choice between options.
func joinStatOptions(opts []ProgOption) []ProgOption {
	var out []ProgOption
	for _, o := range opts {
		o.OneOf = joinStatOptions(o.OneOf)
		if n := len(out); n > 0 && o.Type == "improveStat" && out[n-1].Type == "improveStat" && out[n-1].Amount == o.Amount && out[n-1].Maximum == o.Maximum && out[n-1].RollAgain == o.RollAgain {
			out[n-1].Stats = append(slices.Clone(out[n-1].Stats), o.Stats...)
			continue
		}
		out = append(out, o)
	}
	return out
}

// LeanRowText is the text a progression row shows: the one the file writes, else, for a row that grants exactly one rule, the text of the
// rule ("Spy (Active) - ..."), else, for a row that only improves characteristics, "Improve Attacks or Wounds by 1.". It is empty for a
// row whose words cannot be derived and the file leaves out.
func LeanRowText(row lean.PathRow, opts []ProgOption, text func(string) (string, bool), tags map[string]string) string {
	if row.Text != "" {
		return row.Text
	}
	if len(row.Options) == 1 && row.Options[0].Rule != nil {
		name := row.Options[0].Rule.Name
		if d := row.Options[0].Rule.Def; d != nil && d.Definition != "" { // a rule defined where the row names it
			return RuleRowText(name, d.Tag, d.Definition)
		}
		if def, ok := text(name); ok {
			return RuleRowText(name, tags[lean.NameKey(name)], def)
		}
		return ""
	}
	return StatRowText(opts)
}

// LeanPathRules are the special rules each path unlocks, with their descriptions, by path name: the rules of the options of its rows in
// chart order, the description being the text of the row after its " - ".
func LeanPathRules(d *lean.Document, vanilla *VanillaBook) map[string][]UnitRule {
	out := map[string][]UnitRule{}
	text := LeanRuleText(d.Rules, vanilla)
	tags := leanTags(d.Rules)
	for _, p := range d.Paths {
		for _, row := range p.Rows {
			opts, _ := leanOptions(p.Name, row.Options)
			t := LeanRowText(row, opts, text, tags)
			for _, o := range row.Options {
				if o.Rule == nil {
					continue
				}
				desc := t
				if _, after, ok := strings.Cut(desc, " - "); ok {
					desc = after
				}
				out[p.Name] = append(out[p.Name], UnitRule{Name: o.Rule.Name, Description: desc, Source: RuleFromPath})
			}
		}
	}
	return out
}

// LeanHeroicActionText is what a heroic action says: the definition of the rule of that name in rules[], else the glossary's term of the name
// or an alias of it. It is false for a name neither knows.
func LeanHeroicActionText(d *lean.Document, vanilla *VanillaBook, gl Glossary, name string) (string, bool) {
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
			return tidy(strings.Join(paras, " ")), true
		}
	}
	return LeanRuleText(nil, vanilla)(name)
}

// LeanPathTerms are the glossary terms the paths of a lean document make: one for every path (its description, the heroic action it starts
// with, its limits and its chart) and one for every special rule a path unlocks. The term of a starting heroic action is the glossary's own.
func LeanPathTerms(d *lean.Document, vanilla *VanillaBook, gl Glossary) []ExtraTerm {
	const src = "Battle Companies"
	var out []ExtraTerm
	text := LeanRuleText(d.Rules, vanilla)
	tags := leanTags(d.Rules)
	for _, p := range d.Paths {
		def := strings.TrimSpace(cmp.Or(p.Description, p.Note))
		if def == "" {
			def = "A heroic path of Battle Companies: the chart below is what a hero of this path rolls on when it levels up."
		}
		if p.HeroicAction != nil {
			def += "\n\nStarting heroic action: " + *p.HeroicAction + "."
		}
		var limits map[string]map[string]int
		_ = leanRoundTrip(p.Maximums, &limits)
		data := &PathData{Limits: pathLimits(limits), PathDice: p.Dice}
		type unlocked struct{ rule, text string }
		var rules []unlocked
		for _, row := range p.Rows {
			opts, _ := leanOptions(p.Name, row.Options)
			t := LeanRowText(row, opts, text, tags)
			data.Rows = append(data.Rows, PathChartRow{Roll: row.Roll.Lo, Text: tidy(t)})
			for _, o := range row.Options {
				if o.Rule != nil {
					rt := t
					if _, after, ok := strings.Cut(rt, " - "); ok {
						rt = after
					}
					rules = append(rules, unlocked{o.Rule.Name, rt})
				}
			}
		}
		out = append(out, ExtraTerm{Name: p.Name, Category: "paths", Definition: def, Source: src, Path: data})
		for _, r := range rules {
			if tidy(r.text) != "" {
				out = append(out, ExtraTerm{Name: r.rule, Category: "path rules", Definition: strings.TrimSpace(r.text + "\n\nGained by heroes of the " + p.Name + "."), Source: src})
			}
		}
	}
	return out
}
