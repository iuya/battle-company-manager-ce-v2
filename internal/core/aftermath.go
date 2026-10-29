package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// The War's Aftermath of a match: see docs/aftermath.md. This file holds the types, the rules parser and the pure rules
// (no storage, no clock): a backend feeds them the match, the roster and the sheet, and applies what they return.

var (
	// ErrAftermathChanged is returned by a write made on a stale Aftermath.Revision.
	ErrAftermathChanged = errors.New("the aftermath changed")
	// ErrInvalidAftermath carries the reasons a sheet or an answer was rejected (joined, fit to show).
	ErrInvalidAftermath = errors.New("invalid aftermath")
)

// AftermathStatus is how far a company's aftermath in a match has got.
type AftermathStatus string

const (
	AftermathPending  AftermathStatus = "pending"  // nothing saved
	AftermathSheet    AftermathStatus = "sheet"    // a draft is saved
	AftermathInjuries AftermathStatus = "injuries" // started: casualties being rolled
	AftermathLevelUps AftermathStatus = "levelups" // injuries and experience applied
	AftermathDone     AftermathStatus = "done"
	AftermathWaived   AftermathStatus = "waived"
)

// Locks says whether a company in this state may not recruit, buy, drop, heal or change its roster.
func (s AftermathStatus) Locks() bool { return s != AftermathDone && s != AftermathWaived }

// Started says whether the sheet is locked in (the first step is over).
func (s AftermathStatus) Started() bool {
	return s == AftermathInjuries || s == AftermathLevelUps || s == AftermathDone
}

// ModelOutcome is what happened to a unit at the table.
type ModelOutcome string

const (
	OutcomeSurvived ModelOutcome = "survived"
	OutcomeCasualty ModelOutcome = "casualty"
	OutcomeFled     ModelOutcome = "fled" // left the board: no injury roll
)

// Limits of the sheet.
const (
	maxGainCount      = 9
	possessesRelicKey = "possessesRelic" // the modelFlag objective of Relic of Ages Past
	participatedKey   = "participated"
	mvpKey            = "mvp"
)

// ---------------------------------------------------------------------------------------------------------------------
// Rules

// XPGain is one way to earn experience (campaign.experience.gains).
type XPGain struct {
	Key, Label string
	Amount     int
	Repeatable bool
}

// OddsBonus is an Against the Odds bonus and what it does in the aftermath. Only influence, experience and reroll have an
// effect here; the others have all-zero numbers.
type OddsBonus struct {
	Key, Name, Text                string
	Rating                         int
	InfluenceWin, InfluenceOther   int // Influence for a Win / otherwise
	ExperienceWin, ExperienceOther int // Experience per model for a Win / otherwise
	Rerolls                        int
}

// Effective says whether the bonus does anything in the aftermath.
func (b OddsBonus) Effective() bool {
	return b.InfluenceWin != 0 || b.InfluenceOther != 0 || b.ExperienceWin != 0 || b.ExperienceOther != 0 || b.Rerolls != 0
}

// CasualtyKind is what a row of the injuries chart does.
type CasualtyKind string

const (
	CasualtyDead      CasualtyKind = "dead"      // leaves the roster for the cemetery
	CasualtyRetired   CasualtyKind = "retired"   // a hero forced to retire by the count of an injury (not a chart kind)
	CasualtyInjured   CasualtyKind = "injured"   // misses the next game
	CasualtyRecovered CasualtyKind = "recovered" // full recovery, perhaps with a bonus
	CasualtyInjury    CasualtyKind = "injury"    // a persistent injury: its count rises
	CasualtyScratch   CasualtyKind = "scratch"   // misses the next game, or roll again once
	CasualtyHeal      CasualtyKind = "heal"      // full recovery and heal one persistent injury
)

// InjuryRow is a row of the injuries chart of warriors or heroes.
type InjuryRow struct {
	Min, Max   int
	Key, Name  string
	Text       string
	Kind       CasualtyKind
	RetireAt   int            // for an injury: the count that forces retirement (0: never)
	Heals      []string       // for a heal: the injury keys the hero may heal
	BonusDice  string         // the bonus dice as written, lower case ("d3", "d6")
	BonusSpec  DiceSpec       // BonusDice parsed (grantsExperience.dice, grantsInfluence.dice)
	BonusKind  string         // "experience" or "influence"
	Fate       int            // the Fate share of StatGains, kept for the pages that name it
	StatGains  map[string]int // permanent points of each stat gained (to the path's maximum), by stat key
	Persistent bool
}

// Covers says whether the roll (a total of the injury dice) is on this row.
func (r InjuryRow) Covers(total int) bool { return total >= r.Min && total <= r.Max }

// RewardEffect is an entry of campaign.aftermathEffects: what a scenario reward does.
type RewardEffect struct {
	Key, Kind, Text string
	Amount          int
	MaxValue        int    // freeEquipment: the most Influence the item may cost
	When            string // perks: when it is used

	// Items (kind injuryRollModifier | injuryRollReplacement): a consumable a model elects to use before the injury rolls.
	Modifier           int      // injuryRollModifier: added to the 2d6 total of every injury roll
	ClampMin, ClampMax int      // the modified total is held to this range (0, 0: no clamp)
	Stackable          bool     // false: only one of them counts per aftermath
	Carrier            string   // activatedBy.carrier: "hero" | "anyModel"
	Requires           []string // activatedBy.requires: participated | casualty | notCasualty
	Result             string   // injuryRollReplacement: the chart key the model counts as having rolled
	NoRoll             bool     // injuryRollReplacement: the model does not roll
}

// Effect kinds of the consumables.
const (
	kindRollModifier    = "injuryRollModifier"
	kindRollReplacement = "injuryRollReplacement"
)

// RewardCondition says when a scenario reward applies.
type RewardCondition struct {
	Kind, Key, Label string // victory | companyFlag | modelFlag | leaderNotRemoved
}

// RewardRule is a reward of a scenario (data.scenarioDetails[].rewards[]).
type RewardRule struct {
	Effect, Name, Text, Who string
	Condition               RewardCondition
}

// AftermathRules is everything the aftermath reads from the Battle Companies file.
type AftermathRules struct {
	Gains                    []XPGain
	Participating, Draw, Win int // Influence
	LevelEvery               int // the experience that earns one level-up roll (campaign.progression.threshold)
	Bonuses                  []OddsBonus
	Warrior, Hero            []InjuryRow
	InjuryDice               DiceSpec                // campaign.injuries.roll, "2d6": the injury charts cover Min..Max of it
	Effects                  map[string]RewardEffect // by key
	ItemEffects              map[string]string       // lower-case equipment name -> aftermathEffects key (Healing Herbs, Wondrous Cram)
	Scenarios                map[string][]RewardRule // by scenario name
}

// Gain finds an experience gain by key.
func (r AftermathRules) Gain(key string) (XPGain, bool) {
	i := slices.IndexFunc(r.Gains, func(g XPGain) bool { return g.Key == key })
	if i < 0 {
		return XPGain{}, false
	}
	return r.Gains[i], true
}

// Bonus finds an Against the Odds bonus by key.
func (r AftermathRules) Bonus(key string) (OddsBonus, bool) {
	i := slices.IndexFunc(r.Bonuses, func(b OddsBonus) bool { return b.Key == key })
	if i < 0 {
		return OddsBonus{}, false
	}
	return r.Bonuses[i], true
}

// Chart is the injuries chart for a warrior or a hero.
func (r AftermathRules) Chart(hero bool) []InjuryRow {
	if hero {
		return r.Hero
	}
	return r.Warrior
}

// LeanAftermath reads the aftermath rules of a lean document: experience gains, influence gains, Against the Odds bonuses, the
// injuries charts, the scenario rewards and the items that activate an effect. The charts and the bonuses are the campaign tables as
// the file stores them (lean Rest/Free) until a parameter slice types them.
func LeanAftermath(d *lean.Document, vanilla *VanillaBook) (AftermathRules, error) {
	type chartRow struct {
		Roll       jsonRoll `json:"roll"`
		Key        string   `json:"key"`
		Name       string   `json:"name"`
		Text       string   `json:"text"`
		Effects    []string `json:"effects"`
		Persistent bool     `json:"persistent"`
		RetireAt   int      `json:"retireAtCount"`
		Optional   bool     `json:"optionalReroll"`
		Heals      *struct {
			Keys []string `json:"keys"`
		} `json:"heals"`
		GrantsXP *struct {
			Dice string `json:"dice"`
		} `json:"grantsExperience"`
		GrantsInfluence *struct {
			Dice string `json:"dice"`
		} `json:"grantsInfluence"`
		StatChanges []struct {
			Stat   string `json:"stat"`
			Amount int    `json:"amount"`
		} `json:"statChanges"`
	}
	type gains struct {
		Win   int `json:"win"`
		Other int `json:"other"`
	}
	var c struct {
		Progression struct {
			Threshold int `json:"threshold"`
		} `json:"progression"`
		Experience struct {
			Gains []struct {
				Key        string `json:"key"`
				Label      string `json:"label"`
				Amount     int    `json:"amount"`
				Repeatable bool   `json:"repeatable"`
			} `json:"gains"`
		} `json:"experience"`
		Influence struct {
			Gains struct {
				Participating int `json:"participating"`
				Draw          int `json:"draw"`
				Win           int `json:"win"`
			} `json:"gains"`
		} `json:"influence"`
		Injuries struct {
			Roll    string     `json:"roll"`
			Warrior []chartRow `json:"warrior"`
			Hero    []chartRow `json:"hero"`
		} `json:"injuries"`
		AgainstTheOdds struct {
			Bonuses []struct {
				Key    string `json:"key"`
				Name   string `json:"name"`
				Text   string `json:"text"`
				Rating int    `json:"rating"`
				Count  int    `json:"count"`
				Gains  struct {
					Influence *gains `json:"influence"`
					XP        *gains `json:"experiencePerModel"`
				} `json:"gains"`
			} `json:"bonuses"`
		} `json:"againstTheOdds"`
		AftermathEffects []struct {
			Key        string `json:"key"`
			Kind       string `json:"kind"`
			Amount     int    `json:"amount"`
			MaxValue   int    `json:"maxInfluenceValue"`
			When       string `json:"when"`
			Text       string `json:"text"`
			Scenario   string `json:"scenario"`
			SourceBase string `json:"source"`
			Modifier   int    `json:"modifier"`
			Clamp      []int  `json:"clamp"`
			Stackable  bool   `json:"stackable"`
			NoRoll     bool   `json:"noRoll"`
			Result     string `json:"result"`
			Activated  struct {
				Carrier  string   `json:"carrier"`
				Requires []string `json:"requires"`
			} `json:"activatedBy"`
		} `json:"aftermathEffects"`
	}
	raw, err := lean.Encode(d.Campaign)
	if err != nil {
		return AftermathRules{}, err
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return AftermathRules{}, err
	}
	rules := AftermathRules{
		LevelEvery:    c.Progression.Threshold,
		Participating: c.Influence.Gains.Participating, Draw: c.Influence.Gains.Draw, Win: c.Influence.Gains.Win,
		Effects: map[string]RewardEffect{}, ItemEffects: map[string]string{}, Scenarios: map[string][]RewardRule{},
	}
	for _, g := range c.Experience.Gains {
		rules.Gains = append(rules.Gains, XPGain{Key: g.Key, Label: g.Label, Amount: g.Amount, Repeatable: g.Repeatable})
	}
	for _, b := range c.AgainstTheOdds.Bonuses {
		ob := OddsBonus{Key: b.Key, Name: b.Name, Text: b.Text, Rating: b.Rating, Rerolls: b.Count}
		if g := b.Gains.Influence; g != nil {
			ob.InfluenceWin, ob.InfluenceOther = g.Win, g.Other
		}
		if g := b.Gains.XP; g != nil {
			ob.ExperienceWin, ob.ExperienceOther = g.Win, g.Other
		}
		if b.Key != "reroll" {
			ob.Rerolls = 0
		}
		rules.Bonuses = append(rules.Bonuses, ob)
	}
	convert := func(rows []chartRow, hero bool) ([]InjuryRow, error) {
		var out []InjuryRow
		for _, r := range rows {
			row := InjuryRow{Min: r.Roll[0], Max: r.Roll[1], Key: r.Key, Name: r.Name, Text: r.Text, Persistent: r.Persistent, RetireAt: r.RetireAt}
			switch {
			case slices.Contains(r.Effects, "remove"):
				row.Kind = CasualtyDead
			case r.Optional:
				row.Kind = CasualtyScratch
			case r.Persistent:
				row.Kind = CasualtyInjury
			case r.Heals != nil:
				row.Kind = CasualtyHeal
				row.Heals = r.Heals.Keys
			case slices.Contains(r.Effects, "missNextGame"):
				row.Kind = CasualtyInjured
			case slices.Contains(r.Effects, "fullRecovery"):
				row.Kind = CasualtyRecovered
			case len(r.Effects) == 0 && r.Key == "dead": // a row written without the shipped file's effect codes is read by its key
				row.Kind = CasualtyDead
			case len(r.Effects) == 0 && r.Key == "injured":
				row.Kind = CasualtyInjured
			case len(r.Effects) == 0:
				row.Kind = CasualtyRecovered
			default:
				return nil, fmt.Errorf("the injuries chart row %q does nothing the aftermath knows", r.Key)
			}
			bonus := func(kind, path, dice string) error {
				spec, err := ParseDiceAt("campaign.injuries."+map[bool]string{false: "warrior", true: "hero"}[hero]+"["+r.Key+"]."+path+".dice", dice)
				row.BonusKind, row.BonusDice, row.BonusSpec = kind, strings.ToLower(dice), spec
				return err
			}
			if r.GrantsXP != nil {
				if err := bonus("experience", "grantsExperience", r.GrantsXP.Dice); err != nil {
					return nil, err
				}
			}
			if r.GrantsInfluence != nil {
				if err := bonus("influence", "grantsInfluence", r.GrantsInfluence.Dice); err != nil {
					return nil, err
				}
			}
			if !r.Persistent { // a persistent injury keeps its stat changes as the injury itself; a kept row gains the stat for good
				for j, sc := range r.StatChanges {
					path := fmt.Sprintf("campaign.injuries.%s[%s].statChanges[%d]", map[bool]string{false: "warrior", true: "hero"}[hero], r.Key, j)
					switch {
					case !slices.Contains(vanilla.StatKeys(), sc.Stat):
						return nil, fmt.Errorf("%s.stat: %q is not a characteristic; known: %s", path, sc.Stat, strings.Join(vanilla.StatKeys(), ", "))
					case sc.Amount <= 0:
						return nil, fmt.Errorf("%s.amount: %d must be a gain; a loss belongs to a persistent injury", path, sc.Amount)
					}
					if row.StatGains == nil {
						row.StatGains = map[string]int{}
					}
					row.StatGains[sc.Stat] += sc.Amount
					if sc.Stat == "fate" {
						row.Fate = sc.Amount
					}
				}
			}
			out = append(out, row)
		}
		// the chart must cover Min..Max of the injury dice exactly once
		var spans [][2]int
		for _, r := range out {
			spans = append(spans, [2]int{r.Min, r.Max})
		}
		if err := CheckCovers("campaign.injuries."+map[bool]string{false: "warrior", true: "hero"}[hero], rules.InjuryDice, spans); err != nil {
			return nil, fmt.Errorf("the injuries chart must cover every roll of the injury dice once: %w", err)
		}
		return out, nil
	}
	if rules.InjuryDice, err = diceAt("campaign.injuries.roll", c.Injuries.Roll); err != nil {
		return AftermathRules{}, errors.Join(ErrInvalidRuleset, err)
	}
	if rules.Warrior, err = convert(c.Injuries.Warrior, false); err != nil {
		return AftermathRules{}, errors.Join(ErrInvalidRuleset, err)
	}
	if rules.Hero, err = convert(c.Injuries.Hero, true); err != nil {
		return AftermathRules{}, errors.Join(ErrInvalidRuleset, err)
	}
	for _, e := range c.AftermathEffects {
		eff := RewardEffect{Key: e.Key, Kind: e.Kind, Text: e.Text, Amount: e.Amount, MaxValue: e.MaxValue, When: e.When,
			Modifier: e.Modifier, Stackable: e.Stackable, Carrier: e.Activated.Carrier, Requires: e.Activated.Requires, Result: e.Result, NoRoll: e.NoRoll}
		if len(e.Clamp) == 2 {
			eff.ClampMin, eff.ClampMax = e.Clamp[0], e.Clamp[1]
		}
		rules.Effects[e.Key] = eff
	}
	for _, it := range d.Gear {
		if it.AftermathEffect == "" {
			continue
		}
		if _, ok := rules.Effects[it.AftermathEffect]; !ok {
			return AftermathRules{}, errors.Join(ErrInvalidRuleset, fmt.Errorf("the item %q names the aftermath effect %q, which does not exist", it.Name, it.AftermathEffect))
		}
		rules.ItemEffects[NameKey(it.Name)] = it.AftermathEffect
	}
	for _, sc := range d.Scenarios {
		var rewards []struct {
			Effect    string `json:"effect"`
			Name      string `json:"name"`
			Text      string `json:"text"`
			Who       string `json:"who"`
			Condition struct {
				Kind  string `json:"kind"`
				Key   string `json:"key"`
				Label string `json:"label"`
			} `json:"condition"`
		}
		if err := leanRoundTrip(sc.Rewards, &rewards); err != nil {
			return AftermathRules{}, err
		}
		for _, rw := range rewards {
			rules.Scenarios[sc.Name] = append(rules.Scenarios[sc.Name], RewardRule{Effect: rw.Effect, Name: rw.Name, Text: rw.Text, Who: rw.Who,
				Condition: RewardCondition{Kind: rw.Condition.Kind, Key: rw.Condition.Key, Label: rw.Condition.Label}})
		}
	}
	return rules, nil
}

// ---------------------------------------------------------------------------------------------------------------------
// The sheet

// ModelSheet is what happened to one unit at the table.
type ModelSheet struct {
	UnitID  int64
	Outcome ModelOutcome
	Gains   map[string]int // experience gains key -> times earned; flags 0 or 1, repeatable gains a count (0-9)
	Relic   bool           // the bearer of the scenario's modelFlag objective
	// UsesItem is the consumable the unit elects to use before the injury rolls: "" or an aftermathEffects key (healingHerbs,
	// wondrousCram). It must be carried (or loaned) and allowed by the effect's activatedBy; see ItemChoices.
	UsesItem string `json:",omitempty"`
}

// Took says whether the unit took part (the "participated" gain).
func (m ModelSheet) Took() bool { return m.Gains[participatedKey] > 0 }

// AftermathInput is the whole sheet; saved as one document.
type AftermathInput struct {
	AgainstTheOdds []string     // the bonus keys the company took
	Models         []ModelSheet // one per unit of the roster that did not miss this game
}

// Model finds a unit's sheet.
func (in AftermathInput) Model(unitID int64) (ModelSheet, bool) {
	i := slices.IndexFunc(in.Models, func(m ModelSheet) bool { return m.UnitID == unitID })
	if i < 0 {
		return ModelSheet{}, false
	}
	return in.Models[i], true
}

// Taken says whether the company took the Against the Odds bonus.
func (in AftermathInput) Taken(key string) bool { return slices.Contains(in.AgainstTheOdds, key) }

// Absent says whether a unit misses this game: it is not on the sheet, recovers when the aftermath starts and earns nothing.
func Absent(u CompanyUnit) bool { return u.Injured }

// DefaultSheet is the sheet when none is saved: everyone who did not miss the game took part and survived.
func DefaultSheet(roster []CompanyUnit) AftermathInput {
	var in AftermathInput
	for _, u := range roster {
		if Absent(u) {
			continue
		}
		in.Models = append(in.Models, ModelSheet{UnitID: u.ID, Outcome: OutcomeSurvived, Gains: map[string]int{participatedKey: 1}})
	}
	return in
}

// Validate checks the sheet against the roster, the rules file and the company's reported result (result says whether the
// company ticked that it holds the Relic) and returns it cleaned: a unit with no entry gets the default one, zero gains are
// dropped, the order is the roster's. A rejection wraps ErrInvalidAftermath and its message is fit to show.
func (in AftermathInput) Validate(roster []CompanyUnit, rules AftermathRules, result CompanyResult) (AftermathInput, error) {
	return in.ValidateWith(roster, nil, rules, result)
}

// ValidateWith is Validate with the items loaned to units for this match (a Healer's Kit): they can be elected as if carried.
func (in AftermathInput) ValidateWith(roster []CompanyUnit, loaned []LoanedItem, rules AftermathRules, result CompanyResult) (AftermathInput, error) {
	var errs []error
	bad := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	byID := map[int64]CompanyUnit{}
	for _, u := range roster {
		byID[u.ID] = u
	}
	given := map[int64]ModelSheet{}
	for _, m := range in.Models {
		u, ok := byID[m.UnitID]
		switch {
		case !ok:
			bad("a unit on the sheet is not on the roster")
		case Absent(u):
			bad("%s missed this game and is not on the sheet", u.Title())
		case given[m.UnitID].UnitID != 0:
			bad("%s is on the sheet twice", u.Title())
		default:
			given[m.UnitID] = m
		}
	}
	var out AftermathInput
	mvps, relics := 0, 0
	for _, u := range roster {
		if Absent(u) {
			continue
		}
		m, ok := given[u.ID]
		if !ok {
			m = ModelSheet{UnitID: u.ID, Outcome: OutcomeSurvived, Gains: map[string]int{participatedKey: 1}}
		}
		switch m.Outcome {
		case OutcomeSurvived, OutcomeCasualty, OutcomeFled:
		case "":
			m.Outcome = OutcomeSurvived
		default:
			bad("%s: unknown outcome %q", u.Title(), m.Outcome)
		}
		clean := map[string]int{}
		for key, n := range m.Gains {
			if n == 0 {
				continue
			}
			g, known := rules.Gain(key)
			max := 1
			if g.Repeatable {
				max = maxGainCount
			}
			switch {
			case !known:
				bad("%s: %q is not a way to earn experience", u.Title(), key)
			case n < 0 || n > max:
				bad("%s: %q can be earned 0 to %d times", u.Title(), g.Label, max)
			default:
				clean[key] = n
			}
		}
		if len(clean) > 0 && clean[participatedKey] == 0 {
			bad("%s did not take part, so it cannot earn experience", u.Title())
		}
		if clean[mvpKey] > 0 {
			mvps++
		}
		if m.Relic {
			relics++
			if m.Outcome == OutcomeCasualty {
				bad("%s is a casualty and cannot hold the Relic", u.Title())
			}
		}
		if len(clean) == 0 {
			clean = nil
		}
		out.Models = append(out.Models, ModelSheet{UnitID: u.ID, Outcome: m.Outcome, Gains: clean, Relic: m.Relic, UsesItem: m.UsesItem})
	}
	if mvps > 1 {
		bad("only one unit can be the MVP")
	}
	ticked := result.Objectives[possessesRelicKey] > 0
	switch {
	case ticked && relics != 1:
		bad("the result says the company holds the Relic: choose the unit that has it")
	case !ticked && relics > 0:
		bad("the result does not say the company holds the Relic")
	}
	errs = append(errs, validateItems(out, roster, loaned, rules)...)
	seen := map[string]bool{}
	for _, key := range in.AgainstTheOdds {
		if _, ok := rules.Bonus(key); !ok {
			bad("%q is not an Against the Odds bonus", key)
		} else if !seen[key] {
			seen[key] = true
			out.AgainstTheOdds = append(out.AgainstTheOdds, key)
		}
	}
	if len(errs) > 0 {
		return in, errors.Join(append([]error{ErrInvalidAftermath}, errs...)...)
	}
	return out, nil
}

// ---------------------------------------------------------------------------------------------------------------------
// What the match was for the company

// AftermathContext is what the pure rules need to know about a company's match.
type AftermathContext struct {
	Rules    AftermathRules
	Outcome  string // "win" | "draw" | "loss"
	Fled     bool   // All is Lost
	Scenario string // the rulebook scenario's name; "" for a one-off or none
	Result   CompanyResult
	Roster   []CompanyUnit
}

// CompanyOutcome is "win", "draw" or "loss" for the company in an ended match, and whether it fled (All is Lost, which counts
// as a loss). ok is false when the company is not in the match.
func CompanyOutcome(m Match, companyID int64) (outcome string, fled, ok bool) {
	for i, s := range m.Sides {
		for _, c := range s.Companies {
			if c.ID != companyID {
				continue
			}
			fled = m.Result.Company(companyID).Fled
			switch {
			case fled:
				return "loss", true, true
			case m.Winner == 0:
				return "draw", false, true
			case m.Winner == i+1:
				return "win", false, true
			}
			return "loss", false, true
		}
	}
	return "", false, false
}

// AmountLine is a reason and an amount, for the preview and the history ("Winning +2").
type AmountLine struct {
	Text   string
	Amount int
	UnitID int64  // for experience lines
	Key    string // the rule behind it
}

// UnitGain is the experience a unit gets and why.
type UnitGain struct {
	UnitID int64
	Total  int
	Lines  []AmountLine
}

// leader is the roster's leader, 0 when none.
func leaderOf(roster []CompanyUnit) int64 {
	for _, u := range roster {
		if u.Role == RoleLeader {
			return u.ID
		}
	}
	return 0
}

func rosterUnit(roster []CompanyUnit, id int64) (CompanyUnit, bool) {
	i := slices.IndexFunc(roster, func(u CompanyUnit) bool { return u.ID == id })
	if i < 0 {
		return CompanyUnit{}, false
	}
	return roster[i], true
}

// RewardApply is a scenario reward that applies to the company.
type RewardApply struct {
	Key, Name, Text string
	Kind            string // influence | experience | freeEquipment | perk
	Amount          int
	UnitID          int64  // experience: the unit; perk: the relic bearer, 0 for the company
	MaxValue        int    // freeEquipment
	When            string // perk
	Decision        bool   // needs the player's choice at Finish
}

// Rewards are the scenario rewards that apply to the company, in the file's order. A fled company gets none; a "victory"
// reward needs a win; a model flag needs the unit on the sheet that holds it.
func Rewards(ctx AftermathContext, in AftermathInput) []RewardApply {
	if ctx.Fled || ctx.Scenario == "" {
		return nil
	}
	var out []RewardApply
	for _, rule := range ctx.Rules.Scenarios[ctx.Scenario] {
		var unit int64
		switch rule.Condition.Kind {
		case "victory":
			if ctx.Outcome != "win" {
				continue
			}
		case "companyFlag":
			if ctx.Result.Objectives[rule.Condition.Key] == 0 {
				continue
			}
		case "modelFlag":
			i := slices.IndexFunc(in.Models, func(m ModelSheet) bool { return m.Relic })
			if rule.Condition.Key != possessesRelicKey || i < 0 || ctx.Result.Objectives[rule.Condition.Key] == 0 {
				continue
			}
			unit = in.Models[i].UnitID
		case "leaderNotRemoved":
			lead := leaderOf(ctx.Roster)
			m, ok := in.Model(lead)
			if !ok || m.Outcome != OutcomeSurvived {
				continue
			}
			unit = lead
		default:
			continue
		}
		eff, ok := ctx.Rules.Effects[rule.Effect]
		if !ok {
			continue
		}
		r := RewardApply{Key: rule.Effect, Name: rule.Name, Text: eff.Text, UnitID: unit, Amount: eff.Amount, MaxValue: eff.MaxValue, When: eff.When}
		switch eff.Kind {
		case "influence", "experience":
			r.Kind = eff.Kind
		case "freeEquipment":
			r.Kind, r.Decision = "freeEquipment", true
		default:
			r.Kind = "perk"
		}
		if r.Kind == "experience" && unit == 0 {
			continue
		}
		out = append(out, r)
	}
	return out
}

// Experience is what each unit of the sheet earns: the sum of its gains plus the Against the Odds experience bonus plus the
// scenario's experience rewards. A unit that did not take part earns nothing; neither does one On Its Own Path. Units that are
// lost to a casualty roll are listed too (the backend skips the dead).
func Experience(ctx AftermathContext, in AftermathInput) []UnitGain {
	var out []UnitGain
	rewards := Rewards(ctx, in)
	for _, m := range in.Models {
		u, ok := rosterUnit(ctx.Roster, m.UnitID)
		if !ok {
			continue
		}
		g := UnitGain{UnitID: m.UnitID}
		if m.Took() && !u.OwnPath && !u.Wanderer { // neither earns experience (BC: Wanderers gain none, units On Their Own Path none)
			for _, gain := range ctx.Rules.Gains {
				if n := m.Gains[gain.Key]; n > 0 {
					g.Lines = append(g.Lines, AmountLine{Text: gain.Label, Amount: n * gain.Amount, UnitID: m.UnitID, Key: gain.Key})
				}
			}
			for _, key := range in.AgainstTheOdds {
				b, _ := ctx.Rules.Bonus(key)
				amount := b.ExperienceOther
				if ctx.Outcome == "win" {
					amount = b.ExperienceWin
				}
				if amount != 0 {
					g.Lines = append(g.Lines, AmountLine{Text: "Against the Odds: " + b.Name, Amount: amount, UnitID: m.UnitID, Key: "odds:" + b.Key})
				}
			}
			for _, r := range rewards {
				if r.Kind == "experience" && r.UnitID == m.UnitID {
					g.Lines = append(g.Lines, AmountLine{Text: r.Name, Amount: r.Amount, UnitID: m.UnitID, Key: r.Key})
				}
			}
		}
		for _, l := range g.Lines {
			g.Total += l.Amount
		}
		out = append(out, g)
	}
	return out
}

// Influence is the breakdown of the Influence the company earns at Finish: participating always; a Draw or a Win; the Against
// the Odds bonus; scenario rewards in Influence. A company that fled (All is Lost) gets participating and its Against the Odds bonuses (it is a loss, and Rewards gives it no scenario rewards). The Influence of
// Wounds of a Hero comes from the casualty rolls and is added by the backend.
func Influence(ctx AftermathContext, in AftermathInput) []AmountLine {
	lines := []AmountLine{{Text: "Participating", Amount: ctx.Rules.Participating, Key: "participating"}}
	switch ctx.Outcome { // a fled company's outcome is a loss: no Win or Draw, but it keeps its Against the Odds bonuses
	case "win":
		lines = append(lines, AmountLine{Text: "Win", Amount: ctx.Rules.Win, Key: "win"})
	case "draw":
		lines = append(lines, AmountLine{Text: "Draw", Amount: ctx.Rules.Draw, Key: "draw"})
	}
	for _, key := range in.AgainstTheOdds {
		b, _ := ctx.Rules.Bonus(key)
		amount := b.InfluenceOther
		if ctx.Outcome == "win" {
			amount = b.InfluenceWin
		}
		if amount != 0 {
			lines = append(lines, AmountLine{Text: "Against the Odds: " + b.Name, Amount: amount, Key: "odds:" + b.Key})
		}
	}
	for _, r := range Rewards(ctx, in) {
		if r.Kind == "influence" {
			lines = append(lines, AmountLine{Text: r.Name, Amount: r.Amount, Key: r.Key})
		}
	}
	return lines
}

// RerollsFor is how many Against the Odds re-rolls the company's bonuses give.
func RerollsFor(rules AftermathRules, in AftermathInput) int {
	n := 0
	for _, key := range in.AgainstTheOdds {
		b, _ := rules.Bonus(key)
		n += b.Rerolls
	}
	return n
}

// AftermathPreview is what the sheet step shows after every save.
type AftermathPreview struct {
	Experience []UnitGain
	Influence  []AmountLine
	Rolls      int           // casualty rolls to come (a Wondrous Cram casualty does not roll)
	Items      []AmountLine  // consumables in use ("Healing Herbs: +1 to every injury roll", "Wondrous Cram on Aldor: no roll")
	Rewards    []RewardApply // scenario rewards and perks that will apply
}

// InfluenceTotal sums the Influence lines.
func (p AftermathPreview) InfluenceTotal() int {
	n := 0
	for _, l := range p.Influence {
		n += l.Amount
	}
	return n
}

// Preview works out the whole preview of a sheet.
func Preview(ctx AftermathContext, in AftermathInput) AftermathPreview {
	p := AftermathPreview{Experience: Experience(ctx, in), Influence: Influence(ctx, in), Rewards: Rewards(ctx, in)}
	p.Items = itemLines(ctx, in)
	for _, m := range in.Models {
		if m.Outcome == OutcomeCasualty && !ctx.Rules.noRoll(m.UsesItem) {
			p.Rolls++
		}
	}
	return p
}

// ---------------------------------------------------------------------------------------------------------------------
// Casualties

// CasualtyUnit is what the injury roll needs to know about the unit.
type CasualtyUnit struct {
	ID       int64
	Hero     bool
	Injuries map[string]int // persistent injuries the hero has, by key
}

// DecisionOption is one answer to a decision: a unit or an injury.
type DecisionOption struct{ Key, Label string }

// CasualtyDecision is an open question. Kind: "scratch" (roll again or accept), "heal" (options are the hero's injury keys,
// an empty answer is none), "succession" (options are unit ids to promote), "sergeant" (the same, optional).
type CasualtyDecision struct {
	Kind    string
	Options []DecisionOption
}

// CasualtyEffect is what the chart says happens to a unit.
type CasualtyEffect struct {
	Row       InjuryRow
	Kind      CasualtyKind
	InjuryKey string // for an injury or a retirement
	Count     int    // the new count of that injury
	Decision  *CasualtyDecision
}

// ResolveCasualty looks the 2D6 total up in the chart. A persistent injury raises its count and forces the hero to retire at
// its retireAtCount (Kind retired, as Dead); a Full Recovery that can heal offers the choice only when the hero has an injury to
// heal. ok is false when the chart has no such row.
func ResolveCasualty(chart []InjuryRow, total int, unit CasualtyUnit) (CasualtyEffect, bool) {
	i := slices.IndexFunc(chart, func(r InjuryRow) bool { return r.Covers(total) })
	if i < 0 {
		return CasualtyEffect{}, false
	}
	row := chart[i]
	e := CasualtyEffect{Row: row, Kind: row.Kind}
	switch row.Kind {
	case CasualtyInjury:
		e.InjuryKey = row.Key
		e.Count = unit.Injuries[row.Key] + 1
		if row.RetireAt > 0 && e.Count >= row.RetireAt {
			e.Kind = CasualtyRetired
		}
	case CasualtyScratch:
		e.Decision = &CasualtyDecision{Kind: "scratch", Options: []DecisionOption{{Key: "reroll", Label: "Roll again"}, {Key: "keep", Label: "Miss the next game"}}}
	case CasualtyHeal:
		var opts []DecisionOption
		for _, key := range row.Heals {
			if unit.Injuries[key] > 0 {
				label := key
				for _, r := range chart { // the chart's name ("Arm Wound"), not the key
					if r.Key == key && r.Name != "" {
						label = r.Name
					}
				}
				opts = append(opts, DecisionOption{Key: key, Label: label})
			}
		}
		if len(opts) == 0 {
			e.Kind = CasualtyRecovered
		} else {
			e.Decision = &CasualtyDecision{Kind: "heal", Options: opts}
		}
	}
	return e, true
}

// ResolveScratchReroll is the second roll of T'is Just a Scratch: whatever it gives stands, even if worse, and another Scratch
// means the hero misses the next game.
func ResolveScratchReroll(chart []InjuryRow, total int, unit CasualtyUnit) (CasualtyEffect, bool) {
	e, ok := ResolveCasualty(chart, total, unit)
	if ok && e.Kind == CasualtyScratch {
		e.Kind, e.Decision = CasualtyInjured, nil
	}
	return e, ok
}

// Bonus rolls the bonus dice of a result (campaign.injuries rows grantsExperience / grantsInfluence: Lesson Learned D3, Wounds of a Hero D6). 0 when it has none.
func (e CasualtyEffect) Bonus(d Dice) int {
	if e.Row.BonusDice == "" {
		return 0
	}
	return d.Roll(e.Row.BonusSpec)
}

// Succession is who must be chosen after the Leader fell or a Sergeant was lost. Heroes are units with a role other than warrior.
type Succession struct {
	NeedLeader      bool
	LeaderOptions   []int64 // the remaining Sergeants (other heroes when there are none)
	NeedSergeant    bool    // the company has fewer Sergeants than the rules give it
	Sergeants       int     // the Sergeants a company has (campaign.company.sergeants), set with NeedSergeant
	SergeantOptions []int64 // the other heroes
	// Fallback is set when no hero is left (for the Leader, or for a lost Sergeant): the Warriors with the most Experience (ties:
	// highest points); one entry means it is treated as having rolled Hero in the Making, several mean the player decides. The
	// chosen Warrior chooses its path in the path window.
	Fallback []int64
}

// NextSuccession says what the company has to decide after its Leader fell (leaderFell), looking at the roster that is left
// (the fallen already removed, the roles as they are now) and the number of Leaders the rules give a company
// (campaign.company.leaders). Without a fallen Leader nothing is needed.
func NextSuccession(roster []CompanyUnit, leaderFell bool, leaders int) Succession {
	var s Succession
	if !leaderFell {
		return s
	}
	var sergeants, others []int64
	led := 0
	for _, u := range roster {
		switch u.Role {
		case RoleLeader:
			led++
		case RoleSergeant:
			sergeants = append(sergeants, u.ID)
		case RoleHero:
			others = append(others, u.ID)
		}
	}
	if led >= leaders {
		return Succession{} // the company has its leaders again: somebody already took over
	}
	switch {
	case len(sergeants) > 0:
		s.NeedLeader, s.LeaderOptions = true, sergeants
	case len(others) > 0:
		s.NeedLeader, s.LeaderOptions = true, others
	default:
		if s.Fallback = mostExperienced(roster); len(s.Fallback) == 0 {
			return Succession{} // nobody is left to lead
		}
		s.NeedLeader = true
	}
	return s
}

// NextSergeant says whether the company, with its Leader, has fewer Sergeants than the rules give it (campaign.company.sergeants) and which heroes could fill the place.
// replace is true when a Sergeant died or retired and has not been replaced: with no hero left, the Warriors with the most
// Experience are then offered (Fallback; the place is not optional), as for a lost Leader.
func NextSergeant(roster []CompanyUnit, replace bool, sergeants int) Succession {
	s := Succession{Sergeants: sergeants}
	n := 0
	for _, u := range roster {
		switch u.Role {
		case RoleSergeant:
			n++
		case RoleHero:
			s.SergeantOptions = append(s.SergeantOptions, u.ID)
		}
	}
	if n >= sergeants {
		return s
	}
	switch {
	case len(s.SergeantOptions) > 0:
		s.NeedSergeant = true
	case replace:
		if s.Fallback = mostExperienced(roster); len(s.Fallback) > 0 {
			s.NeedSergeant = true
		}
	}
	return s
}

func mostExperienced(roster []CompanyUnit) []int64 {
	var best []CompanyUnit
	for _, u := range roster {
		if u.Role != RoleWarrior {
			continue
		}
		switch {
		case len(best) == 0 || u.XP > best[0].XP || (u.XP == best[0].XP && u.Points > best[0].Points):
			best = []CompanyUnit{u}
		case u.XP == best[0].XP && u.Points == best[0].Points:
			best = append(best, u)
		}
	}
	out := make([]int64, len(best))
	for i, u := range best {
		out[i] = u.ID
	}
	return out
}

// ---------------------------------------------------------------------------------------------------------------------
// State of an aftermath as the page shows it

// Casualty is one injury roll and what came of it.
type Casualty struct {
	UnitID    int64
	Name      string // the unit's title, for the page
	Hero      bool
	Rolled    bool
	Dice      []int  // the two dice (last roll)
	Total     int    // the chart total: the dice plus Modifier, held to the effect's clamp
	Modifier  int    // what Healing Herbs added to the dice ("9 +1 = 10"); 0 without
	Item      string // "wondrousCram" (an aftermathEffects key): the unit did not roll, it counts as the effect's result
	ResultKey string // injuries chart key; "" until rolled
	ResultTxt string // the chart's name for the result
	Text      string // the chart's text for the result
	Kind      CasualtyKind
	InjuryKey string
	Count     int
	Bonus     int // the D3 of Lesson Learned or the D6 of Wounds of a Hero, rolled by the server
	Rerolled  bool
	Previous  []string // the results given up for an Against the Odds re-roll or a Scratch roll
	Decision  *CasualtyDecision
	Final     bool
}

// AftermathAnswer answers a decision. UnitID 0 with Key is a reward decision.
type AftermathAnswer struct {
	UnitID int64  // the casualty (scratch, heal); 0 for the rest
	Kind   string // "scratch" | "heal" | "succession" | "sergeant" | "reward" | "continue"
	Key    string // the reward key for "reward"
	Answer string // scratch: "reroll" or "keep"; heal: an injury key or ""; succession and sergeant: a unit id ("" skips the sergeant); reward: "<item>|<unit id>"
}

// AftermathLine is one line of the audit trail ("Aldor: Injured", "+2 Influence for a Win").
type AftermathLine struct {
	Kind, Text string // Kind: injury, experience, influence, reward, perk, succession, note
	UnitID     int64
	Amount     int
	// Set when the lines are read (never stored): the match the line belongs to and when it was written. Match is the match number
	// and MatchID its id (link to /campaigns/{id}/matches/{MatchID}); On is when the aftermath was finished, else started, else when
	// the match ended. They are set on both Aftermath.Lines and a company's History.
	Match   int
	MatchID int64     `json:"-"`
	On      time.Time `json:"-"`
}

// AftermathAccess is what a viewer may do with an aftermath.
type AftermathAccess int

const (
	AftermathNone AftermathAccess = iota // read only
	AftermathEdit
)

// AftermathAccessFor: the owner of the company, the campaign organiser and administrators edit the aftermath of an ended match.
func AftermathAccessFor(u User, m Match, c CampaignCompany, organiserID int64) AftermathAccess {
	if !m.Ended {
		return AftermathNone
	}
	admin := u.Role == RoleAdmin || u.Role == RoleSuperuser
	if admin || (organiserID != 0 && u.ID == organiserID) || (c.OwnerID != 0 && c.OwnerID == u.ID) {
		return AftermathEdit
	}
	return AftermathNone
}

// CanWaive says whether u may waive an aftermath: only the organiser and administrators.
func CanWaive(u User, organiserID int64) bool {
	return CanManageCampaign(u, organiserID)
}

// Aftermath is a company's aftermath in a match, as the page shows it.
type Aftermath struct {
	MatchID, CompanyID int64
	Status             AftermathStatus
	Revision           int
	Outcome            string // "win" | "draw" | "loss"
	Fled               bool   // All is Lost
	Absent             []int64
	Input              AftermathInput
	Preview            AftermathPreview // recomputed on read while pending/sheet; the applied numbers afterwards
	Casualties         []Casualty
	Succession         *Succession   // open when the Leader fell
	NoOneToLead        bool          // the Leader fell and nobody is left to take over (no Hero and no Warrior): nothing to decide
	Pending            []RewardApply // rewards that need a choice at Finish
	Lines              []AftermathLine
	Rerolls            int // Against the Odds re-rolls left
	WaivedNote         string
	Access             AftermathAccess
	FromLive           bool // the unsaved default sheet comes from the live match ("From the live match" badge)
	// FromMatch says the Against the Odds bonuses of the unsaved sheet come from the match's selection ("From the match" badge).
	FromMatch bool
	// ItemChoices lists, per unit of the sheet, the consumables it may elect now (carried or loaned by a Healer's Kit); empty once started.
	ItemChoices map[int64][]string
	// MVP is the live vote tally for the company (nil without live data or votes): the winner is prefilled on the sheet; on a tie the
	// owner chooses among Tied (the sheet shows "Tied: Aldor, Brand").
	MVP *MVPHint
	// Practice is set for a practice match: the aftermath is done and empty, the page shows the short "nothing to roll" state.
	Practice bool
}

// MVPHint is the tally of the MVP votes for one company, for the aftermath sheet.
type MVPHint struct {
	Winner int64       // the unit with the most votes, 0 on a tie
	Tied   []int64     // the units sharing the most votes when it is a tie
	Lines  []TallyLine // every unit that got a vote, most first
}

// AftermathRef is a company's aftermath status without the details, for the match page and the lock.
type AftermathRef struct {
	MatchID     int64
	MatchNumber int
	CompanyID   int64
	CompanyName string
	Status      AftermathStatus
}

// Distinguish returns the roster with every unit titled so that the player can tell them apart at the table: a Leader or
// Sergeant carries its role ("Mordor Orc Warrior (Leader)"), and units that would still share a title are numbered in the
// company's order ("Warrior of Rohan 2"). It is for the aftermath's cards, options and lines only: it never reaches the stored
// units, and a name the player gave keeps standing as the title.
func Distinguish(roster []CompanyUnit) []CompanyUnit {
	out := slices.Clone(roster)
	for i := range out {
		switch out[i].Role {
		case RoleLeader:
			out[i].Name = out[i].Title() + " (Leader)"
		case RoleSergeant:
			out[i].Name = out[i].Title() + " (Sergeant)"
		}
	}
	count := map[string]int{}
	for _, u := range out {
		count[u.Title()]++
	}
	seen := map[string]int{}
	for i := range out {
		t := out[i].Title()
		if count[t] > 1 {
			seen[t]++
			out[i].Name = fmt.Sprintf("%s %d", t, seen[t])
		}
	}
	return out
}
