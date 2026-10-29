package core

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// Progression is the level-up system of Battle Companies, read from the rules file (campaign.progression, data.paths,
// campaign.characteristicLimits). A unit that earns campaign.progression.threshold experience rolls once: a warrior on a D6 (promotion, or a hero in the
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
	Type      string // specialRule, improveStat, minorSpecialRule, heroicAction, magicalPower, improveCastingValue, oneOf
	Stats     []string
	Amount    int
	Maximum   int  // an extra ceiling for the stat (Will 6 for the Tactician)
	RollAgain bool // the hero also takes the other result
	Rule      string
	MaxTotal  int // most minor rules or heroic actions a hero can gain this way
	OneOf     []ProgOption
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
	PathDice        DiceSpec                  // the dice of the chart (paths[].dice, else progression.hero.roll): Rows cover Min..Max of it
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
		return !slices.ContainsFunc(u.Equipment, func(e Equipment) bool { return sameName(e.Name, w) })
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

// Stats the progression caps by name.
const (
	statCourage      = "courage"
	statIntelligence = "intelligence"
)

// ProgressionRules are the progression rules of the Battle Companies file.
type ProgressionRules struct {
	Threshold     int
	heroStats     map[string]bool // the heroes-only characteristics (they only grow: shown as +1, not as a before and after)
	WarriorDice   DiceSpec        // progression.warrior.roll: Warrior covers Min..Max of it
	HeroDice      DiceSpec        // progression.hero.roll: the dice of a path that names none
	HeroRolls     int             // progression.hero.rollsPerProgression: how many results a hero rolls for one level-up
	HeroChoose    int             // progression.hero.choose: how many of them it keeps (the engine keeps one)
	Warrior       []WarriorResult
	NoPromotion   []string       // the stats a warrior with no promotion improves
	NoPromotionBy map[string]int // by how much each improves (warrior.noPromotion.courage / .intelligence, C5)
	NoPromotionTo int            // up to this value (3 means 3+)
	Paths         []PathProgression
	CustomPath    string            // the homebrew path the option customOnTheirOwnPath hands to units On Their Own Path
	Hidden        []PathProgression // paths a company or an option hands out (not for the player to choose): the homebrew path of the units On Their Own Path
	EndOfPath     map[string]string // the paths a company hands out to units that never level up, with the special rule each has
	Peak          map[string]map[string]int
	// CITotalCap is the most Courage and Intelligence may be improved in total, from the unmodified profile (characteristicLimits.increases.
	// courageIntelligenceTotalCap), CITotalGear whether the improvement from gear counts in it. 0: no cap.
	CITotalCap    int
	CITotalGear   bool
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

// LevelDice are the dice of a level-up as the pages show them (progression.warrior.roll, progression.hero.roll, paths[].dice).
type LevelDice struct {
	Warrior DiceSpec
	Hero    DiceSpec            // a path that names no dice
	Pairs   int                 // results a hero rolls for one level-up (progression.hero.rollsPerProgression)
	Paths   map[string]DiceSpec // by path name
}

// HeroFor is the dice of the hero's chart on that path.
func (l LevelDice) HeroFor(path string) DiceSpec {
	if d, ok := l.Paths[path]; ok {
		return d
	}
	return l.Hero
}

// LevelDice is the dice of the level-ups, for the pages.
func (r ProgressionRules) LevelDice() LevelDice {
	l := LevelDice{Warrior: r.WarriorDice, Hero: r.HeroDice, Pairs: r.HeroRolls, Paths: map[string]DiceSpec{}}
	for _, p := range append(slices.Clone(r.Paths), r.Hidden...) {
		l.Paths[p.Name] = p.PathDice
	}
	return l
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

// SheetStat is a stat of a hero now and how much the company added to it.
type SheetStat struct{ Value, Boost int }

// HeroSheet is what the rules need to know about a hero to see what a result would give it.
type HeroSheet struct {
	Path          string
	Stats         map[string]SheetStat // by stat key (movement, fight, ..., might, will, fate)
	Gear          map[string]int       // what the hero's gear added to each stat (already in Stats), signed like the value
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

// willMaxOverridesKey is the entry of campaign.characteristicLimits.peak that maps a path to its own Will maximum in place of the
// peak's; the loader keeps it in ProgressionRules.Peak next to the stats.
const willMaxOverridesKey = "willMaxOverrides"

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
		if lower && (stat == statCourage || stat == statIntelligence) && r.CITotalCap > 0 {
			base := cur.Value - cur.Boost // the profile with its gear
			if r.CITotalGear {
				base -= sheet.Gear[stat] // the profile without it: the gear's improvement counts in the total
			}
			if base-v > r.CITotalCap {
				continue
			}
		}
		peak := r.Peak[stat]
		if override, has := r.Peak[willMaxOverridesKey][path.Name]; has && stat == "will" {
			peak = map[string]int{"max": override} // the path's own Will maximum replaces the overall peak's
		}
		if fits(path.Limits[stat]) && fits(peak) {
			return v, true
		}
	}
	return 0, false
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
				label := fmt.Sprintf("%s +%d", Capitalise(s), o.Amount)
				if _, lower := r.LowerIsBetter[s]; lower {
					label = fmt.Sprintf("%s %d+ → %d+", Capitalise(s), from, v)
				} else if !r.heroStats[s] {
					label = fmt.Sprintf("%s %d → %d", Capitalise(s), from, v)
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
		for _, name := range r.knownPowersInOrder(sheet) {
			n := sheet.Powers[name]
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

// OfferFor says what a result of the hero's path would give it now; ok is false when nothing in the row can be taken.
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
	Auto    bool // not stored: a warrior's Hero in the Making whose path is not chosen by the player (On Their Own Path)
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
func (r ProgressionRules) RollHero(sheet HeroSheet, lists LevelLists, entered []int, dice Dice) LevelRoll {
	roll := LevelRoll{Kind: "hero"}
	type pair struct {
		sum   int
		dice  []int
		offer LevelOffer
		ok    bool
	}
	spec := r.HeroDice
	if p, ok := r.Path(sheet.Path); ok {
		spec = p.PathDice
	}
	pairs := make([]pair, r.HeroRolls)
	for i := range pairs {
		if i < len(entered) && entered[i] >= spec.Min() && entered[i] <= spec.Max() {
			pairs[i].sum = entered[i]
		}
	}
	valid := 0
	for try := 0; try < 60; try++ {
		valid = 0
		for i := range pairs {
			if pairs[i].sum == 0 {
				pairs[i].dice = RollDice(dice, spec)
				pairs[i].sum = sumOf(pairs[i].dice)
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
			if r := u.Restriction(); r != "" {
				label += " (" + r + " only)"
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

// Label is how a power is listed: its name, casting value and range.
func (p PowerOffer) Label() string {
	rng := "self"
	if p.Range > 0 {
		rng = Inches(float64(p.Range)) + `"`
	}
	return fmt.Sprintf("%s · %d+ · %s", p.Name, p.Casting, rng)
}

// Power finds a power by name, ignoring case.
func (r ProgressionRules) Power(name string) (PowerOffer, bool) {
	for _, p := range r.Powers {
		if sameName(p.Name, name) {
			return p, true
		}
		for _, v := range p.Variants {
			if sameName(v, name) {
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

// knownPowersInOrder lists the lower-cased powers the hero knows in the order of the data (ProgressionRules.Powers, a power with
// variants by its variants), then any the data does not list, sorted by name, so the offer never depends on map order.
func (r ProgressionRules) knownPowersInOrder(sheet HeroSheet) []string {
	out := make([]string, 0, len(sheet.Powers))
	seen := map[string]bool{}
	add := func(n string) {
		k := strings.ToLower(n)
		if _, known := sheet.Powers[k]; known && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	for _, p := range r.Powers {
		add(p.Name)
		for _, v := range p.Variants {
			add(v)
		}
	}
	var rest []string
	for k := range sheet.Powers {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

func (l LevelLists) canonAction(a string) string {
	if l.ActionName == nil {
		return a
	}
	return l.ActionName(a)
}

// soleRule is the special rule a progression row grants when that is all it grants, "" for any other row.
func soleRule(options []json.RawMessage) string {
	if len(options) != 1 {
		return ""
	}
	var o struct {
		Type string `json:"type"`
		Rule string `json:"specialRule"`
	}
	if json.Unmarshal(options[0], &o) != nil || o.Type != "specialRule" {
		return ""
	}
	return o.Rule
}
