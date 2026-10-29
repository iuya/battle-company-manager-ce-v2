package core

import (
	"slices"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// LeanTTSModels reads the Tabletop Simulator mappings of a lean document: `tts` is a property of
// the thing it describes, not six name-keyed tables.
//
//   - gear[] of kind wanderer or creature with tts.model are the stand-ins (the kind is the item's kind);
//   - any other gear[] entry with tts.model is a mount that fights: tts.model is the figure, the attacks are the vanilla ones (the
//     entry is an annotation of the vanilla mount, "Warg" or "Fell Warg", so nothing else is written);
//   - units[] entries with tts: foot and states make the unit a rider (cavalry); options are the option spellings of the mod; defaultOptions
//     the options a bare unit is requested with; marker an alias option left out of the key when it stands alone.
//
// The lists come in the order of gear[] and units[], and a mount comes before the next one it hides (the export shows the first mount a unit
// carries). A company's own units are not read: the mappings name vanilla units, which the shared units[] annotate.
func LeanTTSModels(d *lean.Document, vanilla *VanillaBook) TTSModels {
	var out TTSModels
	for _, g := range d.Gear {
		if g.TTS == nil || g.TTS.Model == "" {
			continue
		}
		switch g.Kind {
		case "wanderer", "creature":
			out.StandIns = append(out.StandIns, TTSStandIn{Kind: g.Kind, Name: g.Name, Model: g.TTS.Model, Alternatives: slices.Clone(g.TTS.Alternatives)})
		default:
			m := TTSMount{Gear: g.Name, Model: g.TTS.Model, Alternatives: slices.Clone(g.TTS.Alternatives)}
			if vanilla != nil {
				if vg, ok := vanilla.Gear(g.Name); ok {
					m.Attacks = vg.Stats["attack"]
				}
			}
			out.Mounts = append(out.Mounts, m)
		}
	}
	slices.SortStableFunc(out.StandIns, func(a, b TTSStandIn) int { // the wanderers, then the creatures
		return strings.Compare(b.Kind, a.Kind)
	})
	for _, u := range d.Units {
		t := u.TTS
		if t == nil {
			continue
		}
		name := u.Name
		if name == "" {
			name = u.Extends
		}
		if t.Foot != "" || t.States != nil {
			out.Riders = append(out.Riders, TTSRider{Unit: name, FootModel: t.Foot, States: t.States != nil && *t.States})
		}
		for _, from := range slices.Sorted(mapKeys(t.Options)) {
			out.Names = append(out.Names, TTSName{Unit: name, From: from, To: t.Options[from]})
		}
		if len(t.DefaultOptions) > 0 {
			out.DefaultOptions = append(out.DefaultOptions, TTSDefaultOptions{Unit: name, Options: slices.Clone(t.DefaultOptions)})
		}
		if t.Marker != "" {
			out.MarkerOptions = append(out.MarkerOptions, TTSMarkerOption{Unit: name, Option: t.Marker})
		}
	}
	return out
}
