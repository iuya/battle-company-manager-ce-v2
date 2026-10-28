package core

import (
	"fmt"
	"strings"
)

// The company Break Point in play (docs/live-match.md, "Company break point in play"): the counts, the turn boundary at which a
// company becomes Broken, the courage tests of a Broken company and the All is Lost offer. Everything here is part of the pure fold
// of the live log; the numbers that come from the rules file (the percent, the scenario exceptions) and from the rosters (how many
// models a unit counts as) are given in a LiveFoldConfig.

// BreakRules are the Break Point rules of the Battle Companies file (campaign.breakPoint).
type BreakRules struct {
	DefaultPercent int
	// Comparison is breakPoint.threshold.comparison (E1): false is "gt" (Broken when the casualties are strictly greater than the Break
	// Point, exactly the Break Point is not Broken), true is "gte". The Break Point is never rounded (a fixed engine rule): the test is
	// done on integers, casualties * 100 against percent * startingModels.
	AtLeast     bool
	CourageDice string   // whenBroken.courageTest.dice as written, lower case ("2d6")
	CourageSpec DiceSpec // CourageDice parsed (E4); the zero spec when the file names none
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
	AtLeast    bool     // breakPoint.threshold.comparison is "gte"
	Percent    int      // the share of the starting models that breaks it (company effect or the file's default); 0: never Broken
	Extra      int      // models counted besides the units: the company's creatures, a temporary Wanderer, models yet to arrive
	Exceptions []string // the scenario's exceptions, as sentences
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
	Threshold100   int // Percent * StartingModels: the Break Point is Threshold100 / 100
	Broken         bool
	BrokenAtTurn   int  // 0 when not Broken
	WillBreak      bool // the casualties are above the Break Point and the company is not Broken yet: it will be at the next turn
	Offer          bool // the All is Lost offer is open
	Fled           bool // it chose to flee
	Exceptions     []string
}

// BreakPoint is the Break Point as a number with one decimal ("6.5").
func (b LiveBreak) BreakPoint() string {
	return fmt.Sprintf("%d.%d", b.Threshold100/100, (b.Threshold100%100)/10)
}

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

type liveMeta struct{ turn int } // match-level state, shared by the copies of a LiveState value

type breakState struct {
	adjStart, adjCasualties int
	broken                  bool
	brokenAt                int
	decided                 bool // an allIsLost entry exists
	fled                    bool
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
		if z, _ := u.zone(); z == ZoneCasualty || (z == ZoneRetreated && u.retreat == "Failed courage") {
			n += s.modelsOf(id)
		}
	}
	return n + s.brk(company).adjCasualties
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

// Turn is the last started turn (0 before the first).
func (s LiveState) Turn() int { return s.m.turn }

// Break is a company's Break Point state.
func (s LiveState) Break(company int64) LiveBreak {
	cfg, st := s.cfg.Break[company], s.brk(company)
	b := LiveBreak{Percent: cfg.Percent, StartingModels: s.startingModels(company), Casualties: s.casualtiesOf(company), Broken: st.broken,
		BrokenAtTurn: st.brokenAt, Fled: st.fled, Exceptions: cfg.Exceptions}
	b.Threshold100 = b.Percent * b.StartingModels
	b.WillBreak = !b.Broken && b.Percent > 0 && cfg.exceeded(b.Casualties, b.StartingModels)
	b.Offer = b.Broken && st.brokenAt == s.m.turn && !st.decided
	return b
}

// startTurn is the fold of a turn entry: the number advances and every company that fails the test (the integer test, once
// per turn, at this entry) becomes Broken for good.
func (s LiveState) startTurn(n int) {
	s.m.turn = n
	for company, st := range s.breaks {
		cfg := s.cfg.Break[company]
		if st.broken || cfg.Percent <= 0 {
			continue
		}
		if cfg.exceeded(s.casualtiesOf(company), s.startingModels(company)) {
			st.broken, st.brokenAt = true, n
		}
	}
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

// checkTurn validates a turn entry and words it, naming the companies it breaks.
func (s LiveState) checkTurn(a LiveAction) (LiveAction, []int64, string, error) {
	switch {
	case a.Turn <= s.m.turn:
		return a, nil, "", Refuse("Turn %d has already started.", a.Turn)
	case a.Turn != s.m.turn+1:
		return a, nil, "", invalid("The next turn is %d.", s.m.turn+1)
	}
	sim := s.clone()
	sim.startTurn(a.Turn)
	text := fmt.Sprintf("Turn %d started", a.Turn)
	for _, id := range s.companyIDs() {
		if !s.brk(id).broken && sim.brk(id).broken {
			text += fmt.Sprintf(": %s is Broken", s.companyName(id))
		}
	}
	return a, nil, text, nil
}

func (s LiveState) companyIDs() []int64 {
	var ids []int64
	seen := map[int64]bool{}
	for _, id := range s.order {
		c := s.units[id].company
		if !seen[c] {
			seen[c] = true
			ids = append(ids, c)
		}
	}
	return ids
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
	if a.Field == "casualties" && n != 1 {
		what = "casualties"
	} else if n != 1 {
		what += "s"
	}
	text := fmt.Sprintf("%s: %d %s %s", name, n, more, what)
	if note := strings.TrimSpace(a.Note); note != "" {
		text += " (" + note + ")"
	}
	return a, nil, text, nil
}

func (s LiveState) checkAllIsLost(a LiveAction, company int64) (LiveAction, []int64, string, error) {
	if !s.knowsCompany(company) {
		return a, nil, "", Refuse("That company is not in this match.")
	}
	if !s.Break(company).Offer {
		return a, nil, "", Refuse("There is nothing to flee from yet.")
	}
	name := s.companyName(company)
	if a.Flee {
		return a, nil, name + " fled the battlefield (All is Lost)", nil
	}
	return a, nil, name + " fights on", nil
}
