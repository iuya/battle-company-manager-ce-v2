package main

import (
	"fmt"
	"strconv"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// newMatch opens a match between the sides (each a list of companies); the first company's name is the side's name.
func (w *world) newMatch(c *wcamp, sides ...[]*wco) core.Match {
	return w.openMatch(c, false, sides...)
}

// newPracticeMatch opens a practice match (no rewards, no aftermath) between the sides.
func (w *world) newPracticeMatch(c *wcamp, sides ...[]*wco) core.Match {
	return w.openMatch(c, true, sides...)
}

func (w *world) openMatch(c *wcamp, practice bool, sides ...[]*wco) core.Match {
	var ns []core.NewSide
	for _, s := range sides {
		side := core.NewSide{Name: s[0].name}
		for _, co := range s {
			side.Companies = append(side.Companies, co.id)
		}
		ns = append(ns, side)
	}
	m, err := w.c().CreateMatch(w.ctx, c.org, c.id, core.NewMatch{Sides: ns, Practice: practice})
	w.must("create match in "+c.name, err)
	return m
}

// rulebook sets a scenario of the rules file.
func (w *world) rulebook(c *wcamp, m core.Match, name string) core.Match {
	m, err := w.c().SetScenario(w.ctx, c.org, c.id, m.ID, m.Revision, core.ScenarioInput{Kind: core.ScenarioRulebook, Name: name})
	w.must("scenario "+name, err)
	return m
}

// oneOff sets a scenario the players invented.
func (w *world) oneOff(c *wcamp, m core.Match, name, description, victory string) core.Match {
	m, err := w.c().SetScenario(w.ctx, c.org, c.id, m.ID, m.Revision, core.ScenarioInput{Kind: core.ScenarioCustom, Name: name, Description: description, Victory: victory})
	w.must("one-off scenario "+name, err)
	return m
}

// scenarioInfo is what the rules file says about a scenario (its tickable objectives).
func (w *world) scenarioInfo(c *wcamp, name string) core.ScenarioInfo {
	if w.scenarios == nil {
		w.scenarios = map[string]core.ScenarioInfo{}
	}
	if len(w.scenarios) == 0 {
		list, err := w.c().Scenarios(w.ctx, c.id)
		w.must("scenarios", err)
		for _, s := range list {
			w.scenarios[s.Name] = s
		}
	}
	return w.scenarios[name]
}

// outcome describes the result the script reports.
type outcome struct {
	winner int // 1-based side; 0 with decided: a draw
	draw   bool
	none   bool  // nothing reported
	vp     []int // victory points per side (empty: not reported)
	fled   []*wco
	ticks  map[*wco]map[string]int
	note   string
}

// report saves the result of the match.
func (w *world) report(c *wcamp, m core.Match, o outcome) core.Match {
	r := core.MatchResult{Decided: !o.none, Winner: o.winner, Note: o.note, Sides: make([]core.SideResult, len(m.Sides))}
	if o.draw {
		r.Winner = 0
	}
	for i, v := range o.vp {
		v := v
		r.Sides[i].VictoryPoints = &v
	}
	seen := map[int64]bool{}
	for _, f := range o.fled {
		seen[f.id] = true
		r.Companies = append(r.Companies, core.CompanyResult{CompanyID: f.id, Fled: true, Objectives: o.ticks[f]})
	}
	for co, t := range o.ticks {
		if !seen[co.id] {
			r.Companies = append(r.Companies, core.CompanyResult{CompanyID: co.id, Objectives: t})
		}
	}
	m, err := w.c().SaveResult(w.ctx, c.org, c.id, m.ID, m.Revision, r)
	w.must("result of match "+fmt.Sprint(m.Number), err)
	return m
}

// endMatch ends the match; its aftermaths are then to play.
func (w *world) endMatch(c *wcamp, m core.Match) core.Match {
	if m.Scenario != nil && len(m.Sides) == 2 && w.scenarioInfo(c, m.Scenario.Name).HasRoles() && (m.Sides[0].Role == "" || m.Sides[1].Role == "") {
		var err error // a scenario with an Attacker and a Defender needs both teams tagged before it ends
		m, err = w.c().SetSideRoles(w.ctx, c.org, c.id, m.ID, m.Revision, []string{core.RoleAttacker, core.RoleDefender})
		w.must("tag the teams", err)
	}
	m, err := endNow(w.ctx, w.c(), c.org, c.id, m.ID, m.Revision)
	w.must("end match", err)
	return m
}

// played makes an ended match: scenario, result, end.
func (w *world) played(c *wcamp, scenario string, o outcome, sides ...[]*wco) core.Match {
	m := w.newMatch(c, sides...)
	m = w.rulebook(c, m, scenario)
	m = w.report(c, m, o)
	return w.endMatch(c, m)
}

// ---------------------------------------------------------------------------------------------------------------------
// Aftermath

func (w *world) am(co *wco, m core.Match) core.Aftermath {
	a, err := w.c().Aftermath(w.ctx, co.camp.org, co.camp.id, m.ID, co.id)
	w.must("aftermath of "+co.name, err)
	return a
}

// sheet is the default sheet of the company (everybody survived and took part); edit it with set.
func (w *world) sheet(co *wco) core.AftermathInput { return core.DefaultSheet(w.detail(co).Roster) }

// set changes what happened to one unit on the sheet.
func set(in *core.AftermathInput, unit int64, outcome core.ModelOutcome, gains map[string]int) {
	for i := range in.Models {
		if in.Models[i].UnitID == unit {
			in.Models[i].Outcome = outcome
			if gains != nil {
				g := map[string]int{"participated": 1}
				for k, v := range gains {
					g[k] = v
				}
				in.Models[i].Gains = g
			}
			return
		}
	}
}

func (w *world) saveSheet(co *wco, m core.Match, in core.AftermathInput) core.Aftermath {
	a, err := w.c().SaveAftermath(w.ctx, co.camp.org, co.camp.id, m.ID, co.id, w.am(co, m).Revision, in)
	w.must("save the sheet of "+co.name, err)
	return a
}

func (w *world) startAftermath(co *wco, m core.Match, in core.AftermathInput) core.Aftermath {
	a, err := w.c().StartAftermath(w.ctx, co.camp.org, co.camp.id, m.ID, co.id, w.am(co, m).Revision, in)
	w.must("start the aftermath of "+co.name, err)
	return a
}

// cast rolls the injury of one casualty with the two dice (and the bonus die of the chart's result, if it has one).
func (w *world) cast(co *wco, m core.Match, unit int64, d1, d2, bonus int) core.Aftermath {
	var entered []int
	w.rollWith(bonus, bonus)
	if co.camp.settings.ManualRolls {
		entered = []int{d1, d2}
	} else {
		w.rollWith(d1, d2, bonus, bonus)
	}
	a, err := w.c().RollCasualty(w.ctx, co.camp.org, co.camp.id, m.ID, co.id, unit, entered, false)
	w.must("roll the injury in "+co.name, err)
	for _, c := range a.Casualties { // a typed-dice campaign asks for the bonus die (Lesson Learned, Wounds of a Hero) as well
		if c.UnitID == unit && c.Decision != nil && c.Decision.Kind == "bonus" {
			return w.decide(co, m, core.AftermathAnswer{UnitID: unit, Kind: "bonus", Answer: strconv.Itoa(bonus)})
		}
	}
	return a
}

func (w *world) decide(co *wco, m core.Match, ans core.AftermathAnswer) core.Aftermath {
	a, err := w.c().Decide(w.ctx, co.camp.org, co.camp.id, m.ID, co.id, ans)
	w.must(fmt.Sprintf("answer %s in %s", ans.Kind, co.name), err)
	return a
}

func (w *world) finishAftermath(co *wco, m core.Match) core.Aftermath {
	a, err := w.c().FinishAftermath(w.ctx, co.camp.org, co.camp.id, m.ID, co.id, w.am(co, m).Revision)
	w.must("finish the aftermath of "+co.name, err)
	return a
}

// settle plays an aftermath through: the sheet, the dice of the casualties (unit -> two dice), then "continue" and finish
// when nothing waits on the player. It returns the aftermath where it stopped.
func (w *world) settle(co *wco, m core.Match, in core.AftermathInput, dice map[int64][3]int, finish bool) core.Aftermath {
	a := w.startAftermath(co, m, in)
	for _, mod := range in.Models {
		if mod.Outcome != core.OutcomeCasualty {
			continue
		}
		d := dice[mod.UnitID]
		a = w.cast(co, m, mod.UnitID, d[0], d[1], d[2])
	}
	if finish {
		if a.Status == core.AftermathInjuries {
			a = w.decide(co, m, core.AftermathAnswer{Kind: "continue"})
		}
		a = w.finishAftermath(co, m)
	}
	return a
}
