package web

import (
	"slices"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The milestones of the aftermath (docs/aftermath.md, "The aftermath page"): the marks of a unit card in the field, here one press on the
// sheet and read only on the results of each unit.

// gainLooks draws the experience gains of the rules file as milestones: the file's label is a sentence, so it goes in the tooltip. A gain
// a custom ruleset adds keeps its label and draws gainLookOther.
var gainLooks = map[string]struct{ Icon, Label string }{
	core.GainWoundedEnemy:  {"machete", "Wounded"},
	core.GainSlainHeroMons: {"crowned-skull", "Slain"},
	core.GainFatePrevented: {"shield-reflect", "Fate"},
	core.GainBrokenPassed:  {"mighty-force", "Courage"},
	core.GainObjective:     {"flying-flag", "Objective"},
	core.GainMVP:           {"medal", "MVP"},
}

const gainLookOther = "diamond"

// sheetMilestone is one gain of a unit on the sheet. M is what milestone_face draws: On always, as the state of the checkbox lights it and
// shows its tick (a repeatable gain counts instead).
type sheetMilestone struct {
	M          milestoneView
	Key        string
	Repeatable bool
	Count      int
}

// RelicFace is the milestone of the unit that holds the Relic.
func (v aftermathView) RelicFace() milestoneView {
	return milestoneView{Icon: "gem-pendant", Label: "Relic", Tip: "Holds the Relic", On: true}
}

// SheetMilestones are the gains the sheet offers a unit: Fate only to a unit that has Fate to spend, as on its card in the field, unless the
// sheet already gives it to the unit.
func (v aftermathView) SheetMilestones(u core.CompanyUnit, m core.ModelSheet) []sheetMilestone {
	var out []sheetMilestone
	for _, g := range v.Gains() {
		n := m.Gains[g.Key]
		if g.Key == core.GainFatePrevented && !u.HasHeroStats() && n == 0 {
			continue
		}
		icon, label := gainLookOther, g.Label
		if l, ok := gainLooks[g.Key]; ok {
			icon, label = l.Icon, l.Label
		}
		out = append(out, sheetMilestone{Key: g.Key, Repeatable: g.Repeatable, Count: n, M: milestoneView{Icon: icon, Label: label, Tip: g.Label, On: true}})
	}
	return out
}

// unitResult is what the match did to a unit, shown once the sheet is locked.
type unitResult struct {
	Unit                core.CompanyUnit
	Played              bool
	Status, Badge, Icon string
	Next                string // that it recovers and can play the next match, for a unit that did not play
	Foot                unitFoot
	Milestones          []milestoneView // every one the unit could earn, lit if it did
}

// unitFoot is what a card's level and experience bar draws: U with the experience it had before this match, and what the match gives it.
type unitFoot struct {
	U       core.CompanyUnit
	MatchXP int
}

// Foot is the foot of a unit's card. Before the experience is given it is the preview of the sheet (so a save of the sheet changes it). Once it
// has been given the unit already counts it, so it is taken out for the bar to draw it in the same way.
func (v aftermathView) Foot(u core.CompanyUnit) unitFoot {
	c, isCasualty := v.casualty(u.ID)
	xp := v.unitXP(u.ID, isCasualty && v.Fallen(c))
	if v.Step() >= 3 {
		u.XP = max(0, u.XP-xp)
	}
	return unitFoot{U: u, MatchXP: xp}
}

// ResultOf is the result of one unit of the sheet.
func (v aftermathView) ResultOf(id int64) unitResult {
	for _, m := range v.A.Input.Models {
		if m.UnitID == id {
			return v.unitResult(m)
		}
	}
	return unitResult{}
}

func (v aftermathView) casualty(id int64) (core.Casualty, bool) {
	i := slices.IndexFunc(v.A.Casualties, func(c core.Casualty) bool { return c.UnitID == id })
	if i < 0 {
		return core.Casualty{}, false
	}
	return v.A.Casualties[i], true
}

func (v aftermathView) unitResult(m core.ModelSheet) unitResult {
	r := unitResult{Status: v.OutcomeLabel(m.Outcome), Badge: v.OutcomeBadge(m.Outcome), Played: m.Took()}
	c, isCasualty := v.casualty(m.UnitID)
	if isCasualty {
		r.Unit, r.Icon = v.CasualtyUnit(c), "death-skull"
		if c.Rolled && !v.Animate[c.UnitID] { // while its dice roll, the card does not give the result away
			r.Status = c.ResultTxt
			switch c.Kind { // the skull is for the unit that fell
			case core.CasualtyDead, core.CasualtyRetired:
			case core.CasualtyInjured, core.CasualtyInjury, core.CasualtyScratch:
				r.Badge, r.Icon = "badge-warn", ""
			default:
				r.Badge, r.Icon = "badge-success", ""
			}
		}
	} else {
		r.Unit = v.Roster[m.UnitID]
		if m.Outcome == core.OutcomeFled {
			r.Icon = "boot-prints"
		}
		if !m.Took() {
			r.Status, r.Badge, r.Icon, r.Next = "Did not take part", "badge-muted", "", "Can play the next match"
		}
	}
	r.Foot = v.Foot(r.Unit)
	r.Milestones = v.resultMilestones(r.Unit, m)
	return r
}

// resultMilestones are the milestones of the sheet with none to press: lit, with their count, those the unit earned.
func (v aftermathView) resultMilestones(u core.CompanyUnit, m core.ModelSheet) []milestoneView {
	var out []milestoneView
	for _, g := range v.SheetMilestones(u, m) {
		g.M.On = g.Count > 0
		if g.Repeatable && g.Count > 0 {
			g.M.Count, g.M.CountTip = g.Count, g.M.Tip
		}
		out = append(out, g.M)
	}
	if v.RelicTicked() {
		f := v.RelicFace()
		f.On = m.Relic
		out = append(out, f)
	}
	return out
}

// unitXP is the experience the match gives a unit. Once it has been given (Rewards, Done) it is the lines of the record; before, the preview
// of the sheet (nothing for a unit that fell) and what a result has already added (Lesson Learned).
func (v aftermathView) unitXP(id int64, fell bool) int {
	given := 0
	for _, l := range v.A.Lines {
		if l.Kind == "experience" && l.UnitID == id {
			given += l.Amount
		}
	}
	if v.Step() >= 3 || fell {
		return given
	}
	for _, g := range v.A.Preview.Experience {
		if g.UnitID == id {
			return g.Total + given
		}
	}
	return given
}

// UnitResults lists the units on the cards of the company once the sheet is locked: while the casualties are rolled, the units that are not
// casualties (those have their own cards); afterwards the whole company, the casualties first. Last come the units that did not play, which
// recover and can play the next match. Nothing for a waived aftermath, where nothing happened.
func (v aftermathView) UnitResults() []unitResult {
	if v.Step() == 0 || v.A.Status == core.AftermathWaived {
		return nil
	}
	var casualties, played, out []unitResult
	for _, m := range v.A.Input.Models {
		r := v.unitResult(m)
		_, isCasualty := v.casualty(m.UnitID)
		switch {
		case r.Unit.ID == 0: // no longer in the company
		case isCasualty:
			if v.Step() != 1 {
				casualties = append(casualties, r)
			}
		case r.Played:
			played = append(played, r)
		default:
			out = append(out, r)
		}
	}
	out = append(append(casualties, played...), out...)
	for _, u := range v.Absent {
		out = append(out, unitResult{Unit: u, Status: "Missed this game", Badge: "badge-muted", Next: "Recovered: can play the next match",
			Foot: v.Foot(u), Milestones: v.resultMilestones(u, core.ModelSheet{UnitID: u.ID})})
	}
	return out
}
