package core

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
)

// Progression is the level-up system of Battle Companies, read from the rules file (campaign.progression, data.paths,
// campaign.characteristicLimits). A unit that earns 5 experience rolls once: a warrior on a D6 (promotion, or a hero in the
// making), a hero twice on 2D6 on its path, choosing one of the two results.

// WarriorResult is a row of the warrior's D6 progression table.
type WarriorResult struct {
	Min, Max int
	Key      string // noChange, promoted, heroInTheMaking
	Name     string
	Text     string
}

// ProgOption is one thing a path row gives: a rule, a stat, a minor rule, a heroic action, a magical power...
type ProgOption struct {
	Type        string // specialRule, improveStat, minorSpecialRule, heroicAction, magicalPower, improveCastingValue, oneOf
	Stats       []string
	Amount      int
	Maximum     int  // an extra ceiling for the stat (Will 6 for the Tactician)
	RollAgain   bool // the hero also takes the other result
	Rule        string
	MaxTotal    int // most minor rules or heroic actions a hero can gain this way
	MaxPerPower int // most times a power's casting value can be improved
	OneOf       []ProgOption
}

// ProgRow is a row of a path's 2D6 chart.
type ProgRow struct {
	Roll    int
	Text    string
	Options []ProgOption
}

// PathProgression is a path's chart and the limits of its heroes.
type PathProgression struct {
	Name            string
	Limits          map[string]map[string]int // by stat: max, maxIncrease, best
	Rows            []ProgRow
	StartAction     string         // the heroic action a hero gets when it takes the path
	StartPower      bool           // and a magical power of its choice
	LimitPerCompany int            // how many heroes of the company may take the path (0: no limit)
	Description     string         // the rulebook's intro sentence for the path (may be empty)
	Summary         string         // what the path offers, ready-made: the path picker's tooltip
	Mastery         string         // its Mastery rule
	SpecialRules    []string       // its own special rules (without the Mastery rule)
	Start           map[string]int // Might, Will and Fate a unit starts on the path with, when it is not the usual 1 each (the homebrew path)
}

// Advancement is a promotion a company offers: a warrior with this profile and wargear can become one of the units.
type Advancement struct {
	FromUnit    string
	FromWargear []string
	To          []AdvancementTarget
}

// AdvancementTarget is a unit a promotion leads to, with the wargear it gets.
type AdvancementTarget struct {
	Unit    string
	Label   string // the company's own name for the promoted unit (Ranger of Ithilien); a unit alias of the company
	Wargear []string
}

// AlreadyIs says whether the unit is already what the promotion would make it: the same profile carrying all the wargear
// it adds (a Knight of Dol Amroth on its armoured horse cannot be promoted onto an armoured horse again).
func (t AdvancementTarget) AlreadyIs(u CompanyUnit) bool {
	if t.Unit != u.Type || len(t.Wargear) == 0 {
		return false
	}
	return !slices.ContainsFunc(t.Wargear, func(w string) bool {
		return !slices.ContainsFunc(u.Equipment, func(e Equipment) bool { return strings.EqualFold(e.Name, w) })
	})
}

// Name is how the company calls the promoted unit.
func (t AdvancementTarget) Name() string {
	if t.Label != "" {
		return t.Label
	}
	return t.Unit
}

// Matches says whether the unit can take the promotion: the same profile, carrying the wargear it asks for (by name or class).
func (a Advancement) Matches(u CompanyUnit) bool {
	if a.FromUnit != u.Type {
		return false
	}
	if !slices.ContainsFunc(a.To, func(t AdvancementTarget) bool { return !t.AlreadyIs(u) }) {
		return false
	}
	for _, w := range a.FromWargear {
		if !slices.ContainsFunc(u.Equipment, func(e Equipment) bool {
			return SameItem(e.Name, e.Spellings, w, nil) || slices.ContainsFunc(e.Classes(), func(c EquipmentClass) bool { return strings.EqualFold(string(c), w) })
		}) {
			return false
		}
	}
	return true
}

// ProgressionRules are the progression rules of the Battle Companies file.
type ProgressionRules struct {
	Threshold     int
	Warrior       []WarriorResult
	NoPromotion   []string // the stats a warrior with no promotion improves
	NoPromotionTo int      // up to this value (3 means 3+)
	Paths         []PathProgression
	CustomPath    string            // the homebrew path the option customOnTheirOwnPath hands to units On Their Own Path
	Hidden        []PathProgression // paths a company or an option hands out (not for the player to choose): the homebrew path of the units On Their Own Path
	EndOfPath     map[string]string // the paths a company hands out to units that never level up, with the special rule each has
	Peak          map[string]map[string]int
	LowerIsBetter map[string]bool
	MinorRules    []string
	MinorParams   map[string]ParamRule // minor rules that need a target chosen when they are taken (Poisoned Attacks: a weapon)
	Powers        []PowerOffer         // what a Path of the Sorcerer hero can learn
	CastFloor     int                  // a casting value is never improved below this (2 means 2+)
	CastMax       int                  // most times a power\'s casting value can be improved
}

// ParamRule is a minor special rule with a parameter X: "weapon" (one of the unit's weapons) or "companyHero" (another hero of
// the company). Definition is the Battle Companies text, where X stands for the choice.
type ParamRule struct {
	Kind       string
	Definition string
}

// SheetTarget is a hero of the company a rule can be aimed at. Targets are kept by id, so that renaming a unit changes nothing.
type SheetTarget struct {
	ID   int64
	Name string
}

// PowerOffer is a magical power a hero can learn, with the range and casting value it has when learned.
type PowerOffer struct {
	Name       string
	Duration   string
	Definition string
	Range      int      // inches; 0 is self
	Casting    int      // the X of X+
	Variants   []string // a power learned with a choice (Fury: its species) is learned as one of these
}

// Path finds a path's chart.
func (r ProgressionRules) Path(name string) (PathProgression, bool) {
	i := slices.IndexFunc(r.Paths, func(p PathProgression) bool { return p.Name == name })
	if i < 0 {
		if j := slices.IndexFunc(r.Hidden, func(p PathProgression) bool { return p.Name == name }); j >= 0 {
			return r.Hidden[j], true
		}
		return PathProgression{}, false
	}
	return r.Paths[i], true
}

// WarriorFor gives the row of the warrior table for a D6.
func (r ProgressionRules) WarriorFor(roll int) (WarriorResult, bool) {
	i := slices.IndexFunc(r.Warrior, func(w WarriorResult) bool { return roll >= w.Min && roll <= w.Max })
	if i < 0 {
		return WarriorResult{}, false
	}
	return r.Warrior[i], true
}

// ParseProgression reads the progression rules of the Battle Companies file, and the promotions of each company.
func ParseProgression(content []byte) (ProgressionRules, map[string][]Advancement, error) {
	type opt struct {
		Type        string            `json:"type"`
		Stats       []string          `json:"stats"`
		Amount      int               `json:"amount"`
		Maximum     int               `json:"maximum"`
		RollAgain   bool              `json:"rollAgain"`
		Rule        string            `json:"specialRule"`
		MaxTotal    int               `json:"maxTotal"`
		MaxPerPower int               `json:"maxPerPower"`
		Options     []json.RawMessage `json:"options"`
	}
	var doc struct {
		Campaign struct {
			Progression struct {
				Threshold int `json:"threshold"`
				Warrior   struct {
					Table []struct {
						Roll []int  `json:"roll"`
						Key  string `json:"key"`
						Name string `json:"name"`
						Text string `json:"text"`
					} `json:"table"`
					NoPromotion struct {
						Courage      int `json:"courage"`
						Intelligence int `json:"intelligence"`
						Maximum      int `json:"maximum"`
					} `json:"noPromotion"`
				} `json:"warrior"`
			} `json:"progression"`
			CharacteristicLimits struct {
				LowerIsBetter []string                   `json:"lowerIsBetter"`
				Peak          map[string]json.RawMessage `json:"peak"`
			} `json:"characteristicLimits"`
		} `json:"campaign"`
		Data struct {
			Companies []struct {
				Name         string `json:"name"`
				Advancements []struct {
					From struct {
						Unit    string   `json:"unit"`
						Wargear []string `json:"wargear"`
					} `json:"from"`
					To []struct {
						Unit    string   `json:"unit"`
						Label   string   `json:"label"`
						Wargear []string `json:"wargear"`
					} `json:"to"`
				} `json:"advancements"`
			} `json:"companies"`
			Paths []struct {
				Name          string                    `json:"name"`
				Selectable    *bool                     `json:"selectable"`
				StartingStats map[string]int            `json:"startingStats"`
				Kind          string                    `json:"kind"`
				SpecialRule   string                    `json:"specialRule"`
				Maximums      map[string]map[string]int `json:"maximums"`
				StartAction   *struct {
					Name string `json:"name"`
				} `json:"startingHeroicAction"`
				StartPower  json.RawMessage `json:"startingMagicalPower"`
				Limit       int             `json:"limitPerCompany"`
				Description string          `json:"description"`
				Offers      struct {
					Summary      string   `json:"summary"`
					MasteryRule  string   `json:"masteryRule"`
					SpecialRules []string `json:"specialRules"`
				} `json:"offers"`
				Progression []struct {
					Roll    int               `json:"roll"`
					Text    string            `json:"text"`
					Options []json.RawMessage `json:"options"`
				} `json:"progression"`
			} `json:"paths"`
			MagicalPowers struct {
				CastingRule struct {
					Floor       int `json:"floor"`
					Improvement struct {
						MaxPerPower int `json:"maxPerPower"`
					} `json:"improvement"`
				} `json:"castingRule"`
				Items []struct {
					Name       string          `json:"name"`
					Duration   string          `json:"duration"`
					Definition string          `json:"definition"`
					Learnable  bool            `json:"learnable"`
					Range      json.RawMessage `json:"range"`
					Casting    int             `json:"casting"`
					Parameter  string          `json:"parameter"`
				} `json:"items"`
			} `json:"magicalPowers"`
			MinorSpecialRules []struct {
				Rule string `json:"specialRule"`
				Kind string `json:"parameterKind"`
			} `json:"minorSpecialRules"`
			ParameterisedRules []struct {
				Name       string `json:"name"`
				Definition string `json:"definition"`
			} `json:"parameterisedRules"`
		} `json:"data"`
	}
	if err := json.Unmarshal(content, &doc); err != nil {
		return ProgressionRules{}, nil, err
	}
	var parse func(raw json.RawMessage) ProgOption
	parse = func(raw json.RawMessage) ProgOption {
		var o opt
		_ = json.Unmarshal(raw, &o)
		out := ProgOption{Type: o.Type, Stats: o.Stats, Amount: max(1, o.Amount), Maximum: o.Maximum, RollAgain: o.RollAgain, Rule: o.Rule, MaxTotal: o.MaxTotal, MaxPerPower: o.MaxPerPower}
		for _, sub := range o.Options {
			out.OneOf = append(out.OneOf, parse(sub))
		}
		return out
	}
	p := doc.Campaign.Progression
	rules := ProgressionRules{Threshold: max(1, p.Threshold), NoPromotionTo: p.Warrior.NoPromotion.Maximum, LowerIsBetter: map[string]bool{}, Peak: map[string]map[string]int{}}
	for _, w := range p.Warrior.Table {
		if len(w.Roll) == 2 {
			rules.Warrior = append(rules.Warrior, WarriorResult{Min: w.Roll[0], Max: w.Roll[1], Key: w.Key, Name: w.Name, Text: w.Text})
		}
	}
	if p.Warrior.NoPromotion.Courage > 0 {
		rules.NoPromotion = append(rules.NoPromotion, "courage")
	}
	if p.Warrior.NoPromotion.Intelligence > 0 {
		rules.NoPromotion = append(rules.NoPromotion, "intelligence")
	}
	for _, k := range doc.Campaign.CharacteristicLimits.LowerIsBetter {
		rules.LowerIsBetter[k] = true
	}
	for stat, raw := range doc.Campaign.CharacteristicLimits.Peak {
		var limits map[string]int
		if json.Unmarshal(raw, &limits) == nil && len(limits) > 0 {
			rules.Peak[stat] = limits
		}
	}
	for _, path := range doc.Data.Paths {
		if path.Kind == "endOfPath" {
			if rules.EndOfPath == nil {
				rules.EndOfPath = map[string]string{}
			}
			rules.EndOfPath[path.Name] = path.SpecialRule
		}
		hidden := path.Selectable != nil && !*path.Selectable // a path a company or an option hands out: not for the player to choose
		if hidden && path.Kind != "custom" {
			continue
		}
		pp := PathProgression{Name: path.Name, Limits: path.Maximums, Start: path.StartingStats}
		if path.StartAction != nil {
			pp.StartAction = path.StartAction.Name
		}
		pp.StartPower = len(path.StartPower) > 0 && string(path.StartPower) != "null"
		pp.LimitPerCompany = path.Limit
		pp.Description, pp.Summary, pp.Mastery, pp.SpecialRules = path.Description, path.Offers.Summary, path.Offers.MasteryRule, path.Offers.SpecialRules
		for _, row := range path.Progression {
			r := ProgRow{Roll: row.Roll, Text: row.Text}
			for _, o := range row.Options {
				r.Options = append(r.Options, parse(o))
			}
			pp.Rows = append(pp.Rows, r)
		}
		if hidden {
			rules.Hidden = append(rules.Hidden, pp)
			if path.Kind == "custom" {
				rules.CustomPath = path.Name
			}
		} else {
			rules.Paths = append(rules.Paths, pp)
		}
	}
	rules.CastFloor, rules.CastMax = max(1, doc.Data.MagicalPowers.CastingRule.Floor), max(1, doc.Data.MagicalPowers.CastingRule.Improvement.MaxPerPower)
	for _, pw := range doc.Data.MagicalPowers.Items {
		if !pw.Learnable {
			continue
		}
		var inches int
		_ = json.Unmarshal(pw.Range, &inches) // "self" stays 0
		rules.Powers = append(rules.Powers, PowerOffer{Name: pw.Name, Duration: pw.Duration, Definition: pw.Definition, Range: inches, Casting: pw.Casting})
	}
	for _, pw := range doc.Data.MagicalPowers.Items { // "Fury (Orc)" is a way of learning "Fury"
		if pw.Learnable {
			continue
		}
		for i := range rules.Powers {
			if strings.HasPrefix(pw.Name, rules.Powers[i].Name+" (") {
				rules.Powers[i].Variants = append(rules.Powers[i].Variants, pw.Name)
			}
		}
	}
	for _, m := range doc.Data.MinorSpecialRules {
		rules.MinorRules = append(rules.MinorRules, m.Rule)
		if m.Kind != "" {
			if rules.MinorParams == nil {
				rules.MinorParams = map[string]ParamRule{}
			}
			pr := ParamRule{Kind: m.Kind}
			for _, d := range doc.Data.ParameterisedRules {
				if d.Name == m.Rule {
					pr.Definition = strings.Join(strings.Fields(d.Definition), " ")
				}
			}
			rules.MinorParams[m.Rule] = pr
		}
	}
	advancements := map[string][]Advancement{}
	for _, c := range doc.Data.Companies {
		for _, a := range c.Advancements {
			adv := Advancement{FromUnit: a.From.Unit, FromWargear: a.From.Wargear}
			for _, t := range a.To {
				adv.To = append(adv.To, AdvancementTarget{Unit: t.Unit, Label: t.Label, Wargear: t.Wargear})
			}
			advancements[c.Name] = append(advancements[c.Name], adv)
		}
	}
	return rules, advancements, nil
}

// SheetStat is a stat of a hero now and how much the company added to it.
type SheetStat struct{ Value, Boost int }

// HeroSheet is what the rules need to know about a hero to see what a result would give it.
type HeroSheet struct {
	Path          string
	Stats         map[string]SheetStat // by stat key (movement, fight, ..., might, will, fate)
	Rules         map[string]bool      // lower-cased special rules it has
	Actions       map[string]bool      // lower-cased heroic actions it has
	MinorGained   int                  // minor special rules gained through progression
	ActionsGained int                  // heroic actions gained through progression
	Powers        map[string]int       // magical powers learned (lower-cased), with how many times their casting value was improved
	Casting       map[string]int       // and their casting value now
	Upgrades      []HeroUpgrade        // the company hero upgrades this hero does not have yet
	Weapons       []string             // the weapons a rule such as Poisoned Attacks could be given to (not those already poisoned)
	Heroes        []SheetTarget        // the other heroes of the company a rule such as Combat Synergy could be aimed at
	PowerNames    map[string]string    // their names as written
}

// LevelLists are the things a hero can pick from when a result lets it choose.
type LevelLists struct {
	MinorRules    []string
	HeroicActions []string
	Powers        []string
	ActionName    func(string) string // canonical spelling of a heroic action (nil: as written), to tell the ones a hero has from the ones on offer
}

// LevelChoice is one concrete thing a result can give. Key says what (stat:attack, rule:Spy, minor:Cave Dweller,
// action:Heroic March, power:Fire Ball, cast:Fire Ball, path:Path of the Duelist, promo:0).
type LevelChoice struct {
	Key       string
	Label     string
	Group     string // choices that belong together (a long list of minor rules)
	Hint      string // what it is, shown on hover (a power's definition)
	RollAgain bool
}

// LevelOffer is one of the results a hero may choose from.
type LevelOffer struct {
	Pair    int // the hero\'s first or second roll (1 or 2); 0 for a warrior
	Roll    int // the 2D6 result
	Text    string
	Choices []LevelChoice
}

// improvedStat says what a stat becomes when raised by amount, within the path's limits and the overall peak; ok is false when
// it cannot be raised at all. A stat where lower is better (Courage, Shoot) goes down.
func (r ProgressionRules) improvedStat(path PathProgression, sheet HeroSheet, stat string, amount, extraMax int) (value int, ok bool) {
	cur, has := sheet.Stats[stat]
	if !has {
		return 0, false
	}
	lower := r.LowerIsBetter[stat]
	for step := max(1, amount); step >= 1; step-- {
		v := cur.Value + step
		if lower {
			v = cur.Value - step
		}
		fits := func(limits map[string]int) bool {
			if m, has := limits["max"]; has && !lower && v > m {
				return false
			}
			if b, has := limits["best"]; has && lower && v < b {
				return false
			}
			if mi, has := limits["maxIncrease"]; has {
				base := cur.Value - cur.Boost
				gained := v - base
				if lower {
					gained = base - v
				}
				if gained > mi {
					return false
				}
			}
			return true
		}
		if extraMax > 0 && !lower && v > extraMax {
			continue
		}
		if fits(path.Limits[stat]) && fits(r.Peak[stat]) {
			return v, true
		}
	}
	return 0, false
}

func statName(stat string) string {
	if stat == "" {
		return ""
	}
	return strings.ToUpper(stat[:1]) + stat[1:]
}

// choicesFor lists what an option gives a hero now (nothing when it cannot take it).
func (r ProgressionRules) choicesFor(path PathProgression, sheet HeroSheet, lists LevelLists, o ProgOption, rollAgain bool) []LevelChoice {
	var out []LevelChoice
	switch o.Type {
	case "specialRule":
		if !sheet.Rules[strings.ToLower(o.Rule)] {
			out = append(out, LevelChoice{Key: "rule:" + o.Rule, Label: o.Rule})
		}
	case "improveStat":
		for _, s := range o.Stats {
			if v, ok := r.improvedStat(path, sheet, s, o.Amount, o.Maximum); ok {
				from := sheet.Stats[s].Value
				label := fmt.Sprintf("%s +%d", statName(s), o.Amount)
				if _, lower := r.LowerIsBetter[s]; lower {
					label = fmt.Sprintf("%s %d+ → %d+", statName(s), from, v)
				} else if s != "might" && s != "will" && s != "fate" {
					label = fmt.Sprintf("%s %d → %d", statName(s), from, v)
				}
				out = append(out, LevelChoice{Key: "stat:" + s + ":" + fmt.Sprint(from-v), Label: label, RollAgain: o.RollAgain || rollAgain})
			}
		}
	case "minorSpecialRule":
		if o.MaxTotal == 0 || sheet.MinorGained < o.MaxTotal {
			for _, m := range lists.MinorRules {
				switch pr, parameterised := r.MinorParams[m]; {
				case !parameterised:
					if !sheet.Rules[strings.ToLower(m)] {
						out = append(out, LevelChoice{Key: "minor:" + m, Label: m, Group: "A minor special rule"})
					}
				case pr.Kind == "weapon": // one choice per weapon that does not have it yet
					for _, w := range sheet.Weapons {
						out = append(out, LevelChoice{Key: "minor:" + m + "|weapon:" + w, Label: m + " (" + w + ")", Group: "A minor special rule"})
					}
				case pr.Kind == "companyHero": // one choice per other hero, picked by id
					for _, h := range sheet.Heroes {
						out = append(out, LevelChoice{Key: fmt.Sprintf("%s%s|unit:%d", "minor:", m, h.ID), Label: m + " - " + h.Name, Group: "A minor special rule"})
					}
				}
			}
		}
	case "heroicAction":
		if o.MaxTotal == 0 || sheet.ActionsGained < o.MaxTotal {
			owned := map[string]bool{}
			for a := range sheet.Actions {
				owned[strings.ToLower(lists.canonAction(a))] = true
			}
			for _, a := range lists.HeroicActions {
				if !owned[strings.ToLower(lists.canonAction(a))] {
					out = append(out, LevelChoice{Key: "action:" + a, Label: a, Group: "A heroic action"})
				}
			}
		}
	case "magicalPower":
		out = append(out, r.PowerChoices(sheet)...)
	case "improveCastingValue":
		for name, n := range sheet.Powers {
			cur := sheet.Casting[name]
			if n < r.CastMax && cur > r.CastFloor {
				out = append(out, LevelChoice{Key: "cast:" + name, Label: fmt.Sprintf("%s %d+ → %d+", sheet.PowerName(name), cur, cur-1), Group: "Improve a casting value"})
			}
		}
	case "oneOf":
		for _, sub := range o.OneOf {
			out = append(out, r.choicesFor(path, sheet, lists, sub, rollAgain)...)
		}
	}
	return out
}

// OfferFor says what a 2D6 result of the hero's path would give it now; ok is false when nothing in the row can be taken.
func (r ProgressionRules) OfferFor(sheet HeroSheet, lists LevelLists, sum int) (LevelOffer, bool) {
	path, found := r.Path(sheet.Path)
	if !found {
		return LevelOffer{}, false
	}
	for _, row := range path.Rows {
		if row.Roll != sum {
			continue
		}
		offer := LevelOffer{Roll: sum, Text: row.Text}
		for _, o := range row.Options {
			offer.Choices = append(offer.Choices, r.choicesFor(path, sheet, lists, o, false)...)
		}
		return offer, len(offer.Choices) > 0
	}
	return LevelOffer{}, false
}

// Progress is what a unit has gained through progression, on top of its profile: the app keeps one per unit.
type Progress struct {
	Taken    int            // level-up rolls made
	Bonus    map[string]int // added to the profile by stat key (negative where lower is better)
	Rules    []string       // special rules gained from the path
	Minor    []string       // minor special rules gained
	Actions  []string       // heroic actions gained
	Powers   []string       // magical powers learned
	Cast     map[string]int // casting value improvements by lower-cased power
	Path     string         // the path a new hero picked
	Upgrades []string       // company hero upgrades taken
	Drop     []string       // wargear a promotion made the unit lose
	Start    string         // the heroic action its path gave when it became a hero
	Base     string         // the unit it was promoted to
	Gear     []string       // wargear a promotion added
	Hero     bool           // became a hero
}

// Apply gives a hero what a choice says.
func (r ProgressionRules) Apply(p *Progress, key string) error {
	kind, rest, _ := strings.Cut(key, ":")
	switch kind {
	case "upgrade":
		p.Upgrades = append(p.Upgrades, rest)
	case "rule":
		p.Rules = append(p.Rules, rest)
	case "minor":
		p.Minor = append(p.Minor, rest)
	case "action":
		p.Actions = append(p.Actions, rest)
	case "power":
		p.Powers = append(p.Powers, rest)
	case "cast":
		if p.Cast == nil {
			p.Cast = map[string]int{}
		}
		p.Cast[strings.ToLower(rest)]++
	case "stat":
		stat, delta, _ := strings.Cut(rest, ":")
		var d int
		if _, err := fmt.Sscan(delta, &d); err != nil || stat == "" {
			return fmt.Errorf("bad choice %q", key)
		}
		if p.Bonus == nil {
			p.Bonus = map[string]int{}
		}
		p.Bonus[stat] -= d // delta is "from - to": the stat went up (or down where lower is better) by -d
	default:
		return fmt.Errorf("bad choice %q", key)
	}
	return nil
}

// LevelRoll is a level-up roll in progress: what was rolled and what the player may choose from.
type LevelRoll struct {
	Kind    string     // "warrior" or "hero"
	D6      int        // a warrior's roll
	Pairs   []HeroPair // a hero's two rolls, with their dice (none for a pair the player entered)
	Result  string     // a warrior's result key
	Notes   []string
	Offers  []LevelOffer // what the player chooses from (one offer for a warrior)
	Upgrade *LevelOffer  // the company hero upgrades: a hero may take one instead of either result
	Again   bool         // the choice made asks for the other result too: only the remaining offer is shown
	Done    bool
	Stage   string // "startingPower": a new Sorcerer chooses its first power
	Summary string
}

// HeroPair is one of a hero's two rolls of 2D6.
type HeroPair struct {
	Text   string // what the result is on the path\'s chart, also when the hero cannot take it
	Dice   []int  // the dice the app rolled; empty when the player entered the sum
	Sum    int
	Usable bool // the hero can take something from it
}

// RollHero rolls a hero's two pairs: a pair the player entered (their own dice) is used as it is. A result the hero cannot take is
// not offered, and the player must take the other one if it can be taken. Only when neither can be taken are the unavailable
// results rolled again (by the app) until one can. If nothing is possible the hero has learned all there is.
func (r ProgressionRules) RollHero(sheet HeroSheet, lists LevelLists, entered []int, rnd func() int) LevelRoll {
	roll := LevelRoll{Kind: "hero"}
	type pair struct {
		sum   int
		dice  []int
		offer LevelOffer
		ok    bool
	}
	pairs := make([]pair, 2)
	for i := range pairs {
		if i < len(entered) && entered[i] >= 2 && entered[i] <= 12 {
			pairs[i].sum = entered[i]
		}
	}
	valid := 0
	for try := 0; try < 60; try++ {
		valid = 0
		for i := range pairs {
			if pairs[i].sum == 0 {
				pairs[i].dice = []int{rnd(), rnd()}
				pairs[i].sum = pairs[i].dice[0] + pairs[i].dice[1]
			}
			pairs[i].offer, pairs[i].ok = r.OfferFor(sheet, lists, pairs[i].sum)
			if pairs[i].ok {
				valid++
			}
		}
		if valid > 0 {
			break
		}
		for i := range pairs { // neither can be taken: roll them again
			roll.Notes = append(roll.Notes, fmt.Sprintf("A %d gives this hero nothing it can take: rolled again.", pairs[i].sum))
			pairs[i].sum, pairs[i].dice = 0, nil
		}
	}
	for i, pr := range pairs {
		roll.Pairs = append(roll.Pairs, HeroPair{Dice: pr.dice, Sum: pr.sum, Usable: pr.ok, Text: pr.offer.Text})
		switch {
		case pr.ok:
			pr.offer.Pair = i + 1
			roll.Offers = append(roll.Offers, pr.offer)
		}
	}
	if len(sheet.Upgrades) > 0 { // the company's hero upgrade may replace either result, whether or not the results can be taken
		offer := LevelOffer{Text: "A hero may swap a result for the upgrade of its company."}
		for _, u := range sheet.Upgrades {
			label := u.Name
			if u.Restriction != "" {
				label += " (" + u.Restriction + " only)"
			}
			offer.Choices = append(offer.Choices, LevelChoice{Key: "upgrade:" + u.Name, Label: label, Group: "Company-exclusive hero upgrade", Hint: u.Text})
		}
		roll.Upgrade = &offer
	}
	if len(roll.Offers) == 0 && roll.Upgrade == nil {
		roll.Notes = nil
		roll.Done, roll.Summary = true, "This hero has learned all that it needs to know: nothing on its path is left to gain."
	}
	return roll
}

// D6 rolls a die.
func D6() int { return rand.IntN(6) + 1 }

// Label is how a power is listed: its name, casting value and range.
func (p PowerOffer) Label() string {
	rng := "self"
	if p.Range > 0 {
		rng = fmt.Sprintf("%d\"", p.Range)
	}
	return fmt.Sprintf("%s · %d+ · %s", p.Name, p.Casting, rng)
}

// Power finds a power by name, ignoring case.
func (r ProgressionRules) Power(name string) (PowerOffer, bool) {
	for _, p := range r.Powers {
		if strings.EqualFold(p.Name, name) {
			return p, true
		}
		for _, v := range p.Variants {
			if strings.EqualFold(v, name) {
				p.Name = v
				return p, true
			}
		}
	}
	return PowerOffer{}, false
}

// PowerName is the proper-cased name of a power the hero knows.
func (h HeroSheet) PowerName(lower string) string {
	if n, ok := h.PowerNames[lower]; ok {
		return n
	}
	return lower
}

// PowerChoices lists the powers a hero could learn now: those it does not know, a power with a choice (Fury) once per way of
// learning it.
func (r ProgressionRules) PowerChoices(sheet HeroSheet) []LevelChoice {
	var out []LevelChoice
	for _, p := range r.Powers {
		names := []string{p.Name}
		if len(p.Variants) > 0 {
			names = p.Variants
		}
		for _, n := range names {
			if _, known := sheet.Powers[strings.ToLower(n)]; known {
				continue
			}
			learn := p
			learn.Name = n
			out = append(out, LevelChoice{Key: "power:" + n, Label: learn.Label(), Group: "A magical power", Hint: p.Definition})
		}
	}
	return out
}

func (l LevelLists) canonAction(a string) string {
	if l.ActionName == nil {
		return a
	}
	return l.ActionName(a)
}
