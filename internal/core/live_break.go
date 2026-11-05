package core

import (
	"fmt"
	"strings"
)

// The company Break Point in play (docs/live-match.md, "Company break point in play"): the counts, the moment a company becomes Broken
// (as soon as its casualties exceed the Break Point; there are no turns) and the courage tests of a Broken company. Everything here is part of the pure fold
// of the live log; the numbers that come from the rules file (the percent, the scenario exceptions) and from the rosters (how many
// models a unit counts as) are given in a LiveFoldConfig.

// BreakRules are the Break Point rules of the Battle Companies file (campaign.breakPoint).
type BreakRules struct {
	DefaultPercent int
	// Comparison is breakPoint.threshold.comparison: false is "gt" (Broken when the casualties are strictly greater than the Break
	// Point, exactly the Break Point is not Broken), true is "gte". The Break Point is never rounded (a fixed engine rule): the test is
	// done on integers, casualties * 100 against percent * startingModels.
	AtLeast     bool
	CourageDice string   // whenBroken.courageTest.dice as written, lower case ("2d6")
	CourageSpec DiceSpec // CourageDice parsed; the zero spec when the file names none
	PassIf      string   // whenBroken.courageTest.passIf, "totalAtLeastCourage"
	Exceptions  []BreakException
}

// BreakException is a scenario or scenario setting that changes the courage tests or the count (campaign.breakPoint.scenarioExceptions).
type BreakException struct {
	Scenario, Setting string // exactly one is set
	Rule, Effect, Who string
	Prose             string // the data's own player-facing sentence; empty: Text words it from the codes
}

// breakEffectText says what an exception does, in words, from the effect and "who" codes of the data (the data has no prose for them).
var breakEffectText = map[string]string{
	"noCourageTest":                      "%s do not take Courage tests",
	"autoPassCourage":                    "%s pass Courage tests automatically",
	"notCountedForBreakPoint":            "%s do not count for the Break Point",
	"yetToArriveCountAsOnTheBattlefield": "%s count as on the battlefield",
}

var breakWhoText = map[string]string{
	"modelsOnTheHill":          "models on the hill",
	"defenderWithin6OfTheGate": "the Defender's models within 6\" of the Gate",
	"defenderBaggagePonies":    "the Defender's Baggage Ponies",
	"neutralModels":            "neutral models",
	"ownModelsNotArrived":      "models yet to arrive",
}

// Text is the exception as a sentence: the data's `text`, else worded from the codes ("Defend the Hill: models on the hill do not take Courage tests").
func (e BreakException) Text() string {
	if p := strings.TrimSpace(e.Prose); p != "" {
		return p
	}
	who := breakWhoText[e.Who]
	if who == "" {
		who = e.Who
	}
	effect := breakEffectText[e.Effect]
	if effect == "" {
		effect = "%s: " + e.Effect
	}
	name := e.Rule
	if name == "" {
		name = e.Setting
	}
	return name + ": " + fmt.Sprintf(effect, who)
}

// ExceptionsFor lists the exceptions of a scenario (by its name) and of the scenario settings that are on.
func (r BreakRules) ExceptionsFor(scenario string, settings ...string) []BreakException {
	var out []BreakException
	for _, e := range r.Exceptions {
		switch {
		case e.Scenario != "" && e.Scenario == scenario:
			out = append(out, e)
		case e.Setting != "":
			for _, s := range settings {
				if strings.EqualFold(s, e.Setting) {
					out = append(out, e)
				}
			}
		}
	}
	return out
}

// ATO are the Against the Odds rules of the file (empty when the file has none).
func (b *BCBook) ATO() ATORules { return b.ato }

// ExperienceGain is the amount of an experience gain of the file (experience.gains[key].amount); 0 when the file has none.
func (b *BCBook) ExperienceGain(key string) int { return b.xpGains[key] }

// BreakRules are the Break Point rules of the file.
func (b *BCBook) BreakRules() BreakRules { return b.breakRules }

// BreakConfig is what the fold needs to know about one company that is not in the log.
type BreakConfig struct {
	AtLeast        bool     // breakPoint.threshold.comparison is "gte"
	Percent        int      // the share of the starting models that breaks it (company effect or the file's default); 0: never Broken
	Extra          int      // models counted besides the units: the company's creatures, a temporary Wanderer, models yet to arrive
	Exceptions     []string // the scenario's exceptions, as sentences
	EndLossPercent int      // the game ends once the company has lost this share of its starting models; 0: the scenario has no such end
}

// LiveFoldConfig is what the fold takes besides the starting values and the log.
type LiveFoldConfig struct {
	Break       map[int64]BreakConfig // by company
	BreakModels map[int64]int         // by unit: how many models it counts as (1 when missing)
	NoStandFast map[int64]bool        // units that cannot call Stand Fast (Wanderers, a Hero with Broken Honour)
	Loaned      []LoanedItem          // the items a Tool Kit loaned to units for this match (Against the Odds)
	Owned       map[int64][]string    // by unit: the names of the equipment it carries (an owned item can be marked used)
	AfterGame   map[string]bool       // lower-case names of the items that act in the aftermath only (Healing Herbs, Wondrous Cram)
}

// LiveBreak is a company's Break Point state at this point of the log.
type LiveBreak struct {
	Percent        int
	StartingModels int
	Casualties     int
	Threshold100   int  // Percent * StartingModels: the Break Point is Threshold100 / 100
	Broken         bool // the casualties exceed the Break Point now; an undo that takes them back under makes it whole again
	AtLeast        bool // the comparison of the rules file: reaching the Break Point is enough to break (see BreakConfig)
	LeftTable      int  // models that left the board: no casualties, but no longer in the game
	EndLossPercent int  // the share of the starting models whose loss ends the scenario; 0: it has no such end
	Exceptions     []string
}

// BreakAt is how many casualties make the company Broken, always a whole number of models: with 15 models and 50% the Break Point is
// 7.5 and the 8th casualty breaks it; with 14 it is 7 and, as only exceeding it counts, the 8th as well.
func (b LiveBreak) BreakAt() int {
	if b.AtLeast {
		return (b.Threshold100 + 99) / 100
	}
	return b.Threshold100/100 + 1
}

// ToBreak is how many more casualties make the company Broken; 0 when it already is.
func (b LiveBreak) ToBreak() int { return max(0, b.BreakAt()-b.Casualties) }

// EndAt is how many casualties reach the scenario's end, a fixed "at least" against its share of the starting models and never rounded:
// the smallest whole number with casualties * 100 >= share * models. With 16 models and 75% the 12th does, where the Break Point of 50%
// takes the 9th.
func (b LiveBreak) EndAt() int { return (b.EndLossPercent*b.StartingModels + 99) / 100 }

// EndLeftPercent is the scenario's end as the players read it, the share that remains: 25 for an end at 75% lost.
func (b LiveBreak) EndLeftPercent() int { return 100 - b.EndLossPercent }

// ToEnd is how many more casualties reach the scenario's end; 0 when they have. Casualties count those who fled a failed Courage test;
// models that left the board voluntarily do not count, as for the Break Point.
func (b LiveBreak) ToEnd() int { return max(0, b.EndAt()-b.Casualties) }

// Courage reasons of a courageSkip.
var courageSkipReasons = map[string]string{
	"standFast":      "Stand Fast",
	"heroicResolve":  "Heroic Resolve",
	"chariot":        "a Chariot",
	"scenario":       "a scenario rule",
	"engaged":        "in combat",
	"cannotActivate": "cannot activate",
}

// exempt reasons are not passes: the model does not test at all.
func courageExempt(reason string) bool { return reason == "engaged" || reason == "cannotActivate" }

type breakState struct {
	adjStart, adjCasualties int
}

// casualtiesOf counts the models of the company that fell: Field casualties and the units that fled after failing a Courage test.
// A unit that left the board is not one, nor is a mount lost alone.
func (s LiveState) casualtiesOf(company int64) int {
	n := 0
	for _, id := range s.order {
		u := s.units[id]
		if u.company != company || u.absent {
			continue
		}
		if z, _ := u.zone(); z == ZoneCasualty || (z == ZoneRetreated && u.retreat == reasonFailedCourage) {
			n += s.modelsOf(id)
		}
	}
	return n + s.brk(company).adjCasualties
}

// leftTableOf counts the models of the company that left the board without being casualties.
func (s LiveState) leftTableOf(company int64) int {
	n := 0
	for _, id := range s.order {
		u := s.units[id]
		if u.company != company || u.absent {
			continue
		}
		if z, _ := u.zone(); z == ZoneRetreated && u.retreat != reasonFailedCourage {
			n += s.modelsOf(id)
		}
	}
	return n
}

func (s LiveState) modelsOf(unit int64) int {
	if n := s.cfg.BreakModels[unit]; n > 0 {
		return n
	}
	return 1
}

func (s LiveState) startingModels(company int64) int {
	n := s.cfg.Break[company].Extra + s.brk(company).adjStart
	for _, id := range s.order {
		if u := s.units[id]; u.company == company && !u.absent {
			n += s.modelsOf(id)
		}
	}
	return n
}

// exceeded says whether the casualties break a company of models at the start: more than the Break Point (comparison gt) or at least
// it (gte), on integers so the Break Point is not rounded.
func (c BreakConfig) exceeded(casualties, models int) bool {
	if c.AtLeast {
		return casualties*100 >= c.Percent*models
	}
	return casualties*100 > c.Percent*models
}

// Break is a company's Break Point state: Broken as soon as the casualties exceed the Break Point (docs/live-match.md, "Company Break Point in play").
func (s LiveState) Break(company int64) LiveBreak {
	cfg := s.cfg.Break[company]
	b := LiveBreak{Percent: cfg.Percent, StartingModels: s.startingModels(company), Casualties: s.casualtiesOf(company), AtLeast: cfg.AtLeast, LeftTable: s.leftTableOf(company), EndLossPercent: cfg.EndLossPercent, Exceptions: cfg.Exceptions}
	b.Threshold100 = b.Percent * b.StartingModels
	b.Broken = b.Percent > 0 && cfg.exceeded(b.Casualties, b.StartingModels)
	return b
}

// fleeBattlefield removes every unit of the company that is still in the Field: they leave the board (not casualties).
func (s LiveState) fleeBattlefield(company int64) {
	for _, id := range s.order {
		u := s.units[id]
		if u.company != company || u.absent {
			continue
		}
		if z, _ := u.zone(); z == ZoneField {
			u.retreat = "Left the board"
		}
	}
}

func (s LiveState) brk(company int64) breakState {
	if st := s.breaks[company]; st != nil {
		return *st
	}
	return breakState{}
}

func (s LiveState) knowsCompany(company int64) bool {
	_, ok := s.breaks[company]
	return ok
}

func (s LiveState) companyName(id int64) string {
	if s.CompanyName != nil {
		if n := s.CompanyName(id); n != "" {
			return n
		}
	}
	return fmt.Sprintf("company %d", id)
}

func (s LiveState) checkBreakAdjust(a LiveAction, company int64) (LiveAction, []int64, string, error) {
	if !s.knowsCompany(company) {
		return a, nil, "", Refuse("That company is not in this match.")
	}
	if a.Delta == 0 || a.Delta < -9 || a.Delta > 9 {
		return a, nil, "", invalid("The correction is from -9 to 9 models, not 0.")
	}
	name := s.companyName(company)
	var what string
	switch a.Field {
	case "start":
		if s.startingModels(company)+a.Delta < 1 {
			return a, nil, "", Refuse("%s cannot start with fewer than one model.", name)
		}
		what = "starting model"
	case "casualties":
		if s.casualtiesOf(company)+a.Delta < 0 {
			return a, nil, "", Refuse("%s cannot have fewer than no casualties.", name)
		}
		what = "casualty"
	default:
		return a, nil, "", invalid("Correct the starting models or the casualties.")
	}
	n, more := a.Delta, "more"
	if n < 0 {
		n, more = -n, "fewer"
	}
	if a.Field == "casualties" {
		what = Plural(n, what, "casualties")
	} else {
		what = Plural(n, what, what+"s")
	}
	text := fmt.Sprintf("%s: %d %s %s", name, n, more, what)
	if note := strings.TrimSpace(a.Note); note != "" {
		text += " (" + note + ")"
	}
	return a, nil, text, nil
}
