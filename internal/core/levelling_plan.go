package core

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"
)

// LevelPlan says how a hero's two results go together when one of them (a RollAgain option) also takes the other. The window marks an
// option and shows what fits with it, so that core's refusal after the press is never reached. Marks, Conflicts and Alone are all the
// window needs: which option is marked in each result ("" none), which pairs (first result's key|second result's key) do not fit and
// why, and which RollAgain options leave nothing to take from the other result, so they apply at once.
type LevelPlan struct {
	Marks     [2]string         `json:"marks"`
	Both      bool              `json:"both"` // both results take the other too: one mark in each, applied by a button
	Conflicts map[string]string `json:"conflicts"`
	Alone     []string          `json:"alone"`
}

// levelPlan works out the plan of a hero's two results; nil when no option takes the other result or when nothing could be taken with it.
func (env *levelEnv) levelPlan(st CompanyState, roll LevelRoll) *LevelPlan {
	if roll.Kind != "hero" || roll.Again || roll.Done || len(roll.Offers) != 2 {
		return nil
	}
	offers := [2][]LevelChoice{roll.Offers[0].Choices, roll.Offers[1].Choices}
	again := func(c LevelChoice) bool { return c.RollAgain }
	first, second := slices.ContainsFunc(offers[0], again), slices.ContainsFunc(offers[1], again)
	if !first && !second {
		return nil
	}
	plan := &LevelPlan{Conflicts: env.planConflicts(st, roll, offers), Alone: []string{}}
	var marked bool
	switch {
	case first && second:
		marked = plan.markBothSides(offers)
	case first:
		marked = plan.markOneSide(offers, 0)
	default:
		marked = plan.markOneSide(offers, 1)
	}
	if !marked {
		return nil
	}
	return plan
}

// planConflicts says which pairs of options do not go together, and why: a pair fits only when one of them takes the other result and
// that result still offers the other option once it is applied.
func (env *levelEnv) planConflicts(st CompanyState, roll LevelRoll, offers [2][]LevelChoice) map[string]string {
	type asked struct {
		side int
		key  string
	}
	cache := map[asked]map[string]bool{}
	after := func(side int, key string) map[string]bool { // what the other result offers once this option is applied
		a := asked{side, key}
		if _, ok := cache[a]; !ok {
			offered, err := env.offerAfter(st, key, roll.Offers[1-side].Roll)
			if err != nil { // the plan only helps the window, and this runs inside every step of a level-up: an error here must not stop one
				slog.Error("level-up plan: treating the option as not fitting", "unit", env.unit.ID, "option", key, "err", err)
			}
			cache[a] = offered
		}
		return cache[a]
	}
	conflicts := map[string]string{}
	for _, x := range offers[0] {
		for _, y := range offers[1] {
			fits := false
			switch {
			case x.RollAgain:
				fits = after(0, x.Key)[y.Key]
			case y.RollAgain:
				fits = after(1, y.Key)[x.Key]
			}
			if !fits {
				conflicts[x.Key+"|"+y.Key] = conflictReason(x, y)
			}
		}
	}
	return conflicts
}

// markBothSides puts the first pair that fits in the default order as the marks of two results that each take the other; false when none fits.
func (p *LevelPlan) markBothSides(offers [2][]LevelChoice) bool {
	for _, a := range markOrder(offers[0], statCourage, statIntelligence) {
		for _, b := range markOrder(offers[1], statIntelligence, statCourage) {
			if _, no := p.Conflicts[a.Key+"|"+b.Key]; !no {
				p.Marks, p.Both = [2]string{a.Key, b.Key}, true
				return true
			}
		}
	}
	return false
}

// markOneSide marks the first option of the side that takes the other result and has something to take from it; the ones with nothing
// to take apply alone. False when none can be marked.
func (p *LevelPlan) markOneSide(offers [2][]LevelChoice, side int) bool {
	for _, x := range markOrder(offers[side], statCourage, statIntelligence) {
		if !x.RollAgain {
			continue
		}
		fits := slices.ContainsFunc(offers[1-side], func(y LevelChoice) bool {
			k := x.Key + "|" + y.Key
			if side == 1 {
				k = y.Key + "|" + x.Key
			}
			_, no := p.Conflicts[k]
			return !no
		})
		switch {
		case !fits:
			p.Alone = append(p.Alone, x.Key)
		case p.Marks[side] == "":
			p.Marks[side] = x.Key
		}
	}
	return p.Marks[side] != ""
}

// markOrder lists the options in the order they are marked by default: the stats named, then the other options that take the other
// result, then the rest.
func markOrder(choices []LevelChoice, stats ...string) []LevelChoice {
	rank := func(c LevelChoice) int {
		if i := slices.Index(stats, optionStat(c.Key)); i >= 0 {
			return i
		}
		if c.RollAgain {
			return len(stats)
		}
		return len(stats) + 1
	}
	out := slices.Clone(choices)
	slices.SortStableFunc(out, func(a, b LevelChoice) int { return rank(a) - rank(b) })
	return out
}

// optionStat is the stat a choice's key improves, empty for anything else.
func optionStat(key string) string {
	if rest, ok := strings.CutPrefix(key, "stat:"); ok {
		stat, _, _ := strings.Cut(rest, ":")
		return stat
	}
	return ""
}

// offerAfter lists the keys a result of the path would offer once the option is applied, on a copy of the company. None and no error
// is a normal outcome: the result gives the unit nothing any more. An error means the option could not be worked out.
func (env *levelEnv) offerAfter(st CompanyState, key string, sum int) (map[string]bool, error) {
	c := st.Clone()
	us, ok := stateUnit(&c, env.unit.ID)
	if !ok {
		return nil, fmt.Errorf("unit %d: %w", env.unit.ID, ErrNotFound)
	}
	if us.Progress == nil {
		us.Progress = &Progress{}
	}
	rules := env.rs.BC.Progression()
	if err := rules.Apply(us.Progress, key); err != nil {
		return nil, err
	}
	e, err := newLevelEnv(env.rs, env.settings, c, env.unit.ID)
	if err != nil {
		return nil, err
	}
	offer, ok := rules.OfferFor(sheetOf(e.unit, us.Progress, e.d.Roster, upgradesOf(e.rs, e.bcC, e.unit)), levelLists(e.rs), sum)
	if !ok {
		return nil, nil
	}
	out := make(map[string]bool, len(offer.Choices))
	for _, ch := range offer.Choices {
		out[ch.Key] = true
	}
	return out, nil
}

// conflictReason says why two options cannot be taken together. The same stat twice stops at what improvedStat allows: the path's
// limit, the overall peak and, for Courage and Intelligence, the total cap of each (CITotalCap).
func conflictReason(x, y LevelChoice) string {
	a, b := optionStat(x.Key), optionStat(y.Key)
	switch {
	case !x.RollAgain && !y.RollAgain:
		return "Neither of these takes the other result, so they cannot go together."
	case a != "" && a == b:
		return fmt.Sprintf("%s cannot improve twice: it would pass its limit.", Capitalise(a))
	}
	return "It cannot be taken together with the option marked in the other result."
}
