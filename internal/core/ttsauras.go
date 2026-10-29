package core

import (
	"fmt"
)

// The aura rings of the names payload (ArmyNames v2, docs/aura-ranges.md section 4.2, docs/tabletop-simulator.md): which rings go on which figure.

// TTSNamesVersionAuras is the payload version that carries rings: written only when at least one ring is.
const TTSNamesVersionAuras = 2

// TTSAurasLeftOut is the note kind for rings the limits left out.
const TTSAurasLeftOut = "aurasLeftOut"

// TTSRing is one ring of a figure in the payload: radius in inches from the base edge, kind id, when (a always, d declared, x conditional,
// c cast, t target) and the labels.
type TTSRing struct {
	R float64 `json:"r"`
	K string  `json:"k"`
	W string  `json:"w"`
	N string  `json:"n"`
}

var ttsWhen = map[AuraWhen]string{AuraAlways: "a", AuraDeclared: "d", AuraConditional: "x", AuraCast: "c", AuraTarget: "t"}

func ringsOf(as []Aura) []TTSRing {
	if len(as) == 0 {
		return nil
	}
	out := make([]TTSRing, len(as))
	for i, a := range as {
		w := ttsWhen[a.When]
		if w == "" {
			w = "a"
		}
		out[i] = TTSRing{R: a.Radius, K: string(a.Kind), W: w, N: ttsSafe(a.Label)}
	}
	return out
}

// figAuras are the rings of one figure of the export.
type figAuras struct {
	Rings []Aura
}

type ringStats struct {
	notes   []TTSNote
	leftOut int
}

// figureRings are the rings of every figure of exp in spawn order (one entry per figure, empty when it has none) under the selection:
// the rider and the on-foot version of a unit, a creature's own figure; never a mount, never a fallen unit. At most AurasPerFigure per
// figure (kinds in the order of the data, then the smaller radius first) and AurasPerPayload in all; what is over goes in the stats.
func (r *RuleSet) figureRings(d CompanyDetail, exp TTSExport, sel AuraSelection) ([]figAuras, ringStats) {
	return r.figureRingsCap(d, exp, sel, true)
}

// figureRingsCap is figureRings with the limits optional: the dialog counts the rings before the limits (capped false), the payload after.
func (r *RuleSet) figureRingsCap(d CompanyDetail, exp TTSExport, sel AuraSelection, capped bool) ([]figAuras, ringStats) {
	var out []figAuras
	var st ringStats
	t := r.BC.auras
	n := &ttsNamer{rs: r, d: d, models: r.BC.TTSModels(), units: map[int64]CompanyUnit{}}
	for _, u := range d.Roster {
		n.units[u.ID] = u
	}
	matches := map[int64][]auraMatch{}
	given := map[string]bool{} // a unit made of several figures (a bundle) gives its rings to the first
	total, perFigure := 0, 0
	for _, w := range exp.Warbands {
		for _, l := range w.Lines {
			for _, id := range l.UnitIDs {
				var f figAuras
				u, ok := n.units[id]
				switch {
				case !ok || !sel.Draw || t.Empty() || w.Fallen || l.Extra == "mount":
				case l.Creature:
					if c, found := n.creature(u, l); found {
						f.Rings = r.selectedAuras(t.match(creatureFacts(c), false), sel)
					}
				default:
					key := fmt.Sprintf("%d|%s", id, l.Extra)
					if given[key] {
						break
					}
					given[key] = true
					ms, seen := matches[id]
					if !seen {
						ms = r.auraMatches(u)
						matches[id] = ms
					}
					f.Rings = r.selectedAuras(ms, sel)
				}
				if !capped {
					out = append(out, f)
					continue
				}
				if len(f.Rings) > AurasPerFigure {
					perFigure += len(f.Rings) - AurasPerFigure
					f.Rings = f.Rings[:AurasPerFigure]
				}
				if room := AurasPerPayload - total; len(f.Rings) > room {
					st.leftOut += len(f.Rings) - room
					f.Rings = f.Rings[:room]
				}
				total += len(f.Rings)
				out = append(out, f)
			}
		}
	}
	st.leftOut += perFigure
	if st.leftOut > 0 {
		st.notes = append(st.notes, TTSNote{Kind: TTSAurasLeftOut, Text: fmt.Sprintf("%d aura rings were left out (limit %d per figure, %d in all).", st.leftOut, AurasPerFigure, AurasPerPayload)})
	}
	return out, st
}

func creatureFacts(c CompanyCreature) unitFacts {
	f := unitFacts{items: map[string]string{}, rules: map[string]string{}, powers: map[string]string{}, actions: map[string]string{}}
	for _, name := range c.Rules {
		addFact(f.rules, name)
	}
	return f
}
