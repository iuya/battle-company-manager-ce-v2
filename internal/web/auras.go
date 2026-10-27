package web

import (
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The aura chips of the unit cards (docs/aura-ranges.md section 4.6, docs/frontend.md): a row of "Aura 6"" chips under the special rules, one per
// merged aura of the unit (core.Unit.Auras, from core.RuleSet.UnitAuras). Nothing is computed here but the words.

// auraMax is how many chips the row shows before the "+N" button.
const auraMax = 3

// auraRowView is what the aura_row template draws: the chips of the compact row and the ones behind "+N".
type auraRowView struct {
	Shown, Rest []core.Aura
}

// auraRow splits a unit's auras for the template; a unit without any has no row.
func auraRow(as []core.Aura, max int) auraRowView {
	if max <= 0 {
		max = auraMax
	}
	if len(as) <= max {
		return auraRowView{Shown: as}
	}
	return auraRowView{Shown: as[:max], Rest: as[max:]}
}

// auraText is the chip's text: `Aura 6"`.
func auraText(a core.Aura) string { return "Aura " + auraInches(a.Radius) + `"` }

// auraLetter is the kind's first letter, the ring's cue that does not rely on colour.
func auraLetter(a core.Aura) string {
	if a.Kind == "" {
		return ""
	}
	return strings.ToUpper(string(a.Kind)[:1])
}

// auraThick is whether the kind is drawn with the thicker line (the enemy kind).
func auraThick(a core.Aura) bool { return a.Kind == "enemy" }

// auraOff is whether the dialog leaves the aura off by default: a spell, a targeting range, a ring over 12 inches or an optional one.
func auraOff(a core.Aura) bool {
	switch a.Group {
	case core.AuraGroupCast, core.AuraGroupTarget, core.AuraGroupLarge, core.AuraGroupOptional:
		return true
	}
	return false
}

func auraAffects(a core.Aura) string {
	var who string
	switch a.Affects {
	case "friendly":
		who = "friendly models"
	case "enemy":
		who = "enemy models"
	case "self":
		who = "the bearer, near other friendly models"
	default:
		who = "any model"
	}
	if len(a.Keywords) > 0 {
		who += " (" + strings.Join(a.Keywords, ", ") + ")"
	}
	return who
}

func auraWhen(a core.Aura) string {
	switch a.When {
	case core.AuraDeclared:
		return "when declared"
	case core.AuraConditional:
		return "after an event"
	case core.AuraCast:
		return "while the spell is in effect"
	case core.AuraTarget:
		return "limits a choice"
	}
	return "always"
}

// auraMeta is the second line of the chip's tooltip: "Command · friendly models · always · needs line of sight".
func auraMeta(a core.Aura) string {
	parts := []string{strings.ToUpper(string(a.Kind)[:1]) + string(a.Kind)[1:], auraAffects(a), auraWhen(a)}
	if a.LOS {
		parts = append(parts, "needs line of sight")
	}
	if auraOff(a) {
		parts = append(parts, "off by default in Tabletop Simulator")
	}
	return strings.Join(parts, " · ")
}

// auraLabel is the chip's accessible name.
func auraLabel(a core.Aura) string {
	s := "Aura, " + auraInches(a.Radius) + " inches"
	if a.Label != "" {
		s += ": " + a.Label
	}
	s += "; works for " + auraAffects(a) + ", " + auraWhen(a)
	if a.LOS {
		s += ", needs line of sight"
	}
	if auraOff(a) {
		s += ", off by default in Tabletop Simulator"
	}
	return s
}
