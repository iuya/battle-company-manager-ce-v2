package core

import (
	"encoding/json"
	"errors"
)

// TTSModels is data.ttsModels of the Battle Companies file: the fixed mappings from Battle Companies to the model names of the
// Tabletop Simulator MESBG FTC mod (docs/tabletop-simulator.md). Short names only; the mod's catalogue is never stored.
type TTSModels struct {
	StandIns []TTSStandIn // wanderers and creatures, which the mod has no figure for
	Mounts   []TTSMount   // bought mounts that fight (vanilla Attacks above 0) and the model of their figure
	Riders   []TTSRider   // the cavalry units
	Names    []TTSName    // option spellings that differ between the vanilla file and the mod
}

// TTSStandIn: Kind is "wanderer" or "creature", Name the Battle Companies name, Model the mod model shown instead ("Name" or "Name: options").
type TTSStandIn struct {
	Kind, Name, Model string
	Alternatives      []string
}

// TTSMount: Gear is the vanilla gear name of the mount, Model the figure to spawn for it, Attacks its vanilla Attacks.
type TTSMount struct {
	Gear, Model  string
	Alternatives []string
	Attacks      int
}

// TTSRider: Unit is a vanilla cavalry unit; FootModel the model of the rider on foot ("" when the mod has none); States says
// that every model of the unit carries the Tabletop Simulator state that dismounts it (no extra figure is needed then).
type TTSRider struct {
	Unit, FootModel string
	States          bool
}

// TTSName: in Unit, the vanilla option From is written To for the mod.
type TTSName struct{ Unit, From, To string }

// ParseTTSModels reads data.ttsModels of a Battle Companies file (empty when it has none).
func ParseTTSModels(bc []byte) (TTSModels, error) {
	var doc struct {
		Data struct {
			TTS struct {
				StandIns []struct {
					Kind         string   `json:"kind"`
					Name         string   `json:"name"`
					Model        string   `json:"model"`
					Alternatives []string `json:"alternatives"`
				} `json:"standIns"`
				Mounts []struct {
					Gear         string   `json:"gear"`
					Model        string   `json:"model"`
					Alternatives []string `json:"alternatives"`
					Attacks      int      `json:"attacks"`
				} `json:"mounts"`
				Riders []struct {
					Unit      string `json:"unit"`
					FootModel string `json:"footModel"`
					States    bool   `json:"states"`
				} `json:"riders"`
				Names []struct {
					Unit string `json:"unit"`
					From string `json:"from"`
					To   string `json:"to"`
				} `json:"names"`
			} `json:"ttsModels"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bc, &doc); err != nil {
		return TTSModels{}, errors.Join(ErrInvalidRuleset, errors.New("the Battle Companies file is not valid JSON"))
	}
	var out TTSModels
	for _, s := range doc.Data.TTS.StandIns {
		out.StandIns = append(out.StandIns, TTSStandIn{s.Kind, s.Name, s.Model, s.Alternatives})
	}
	for _, m := range doc.Data.TTS.Mounts {
		out.Mounts = append(out.Mounts, TTSMount{m.Gear, m.Model, m.Alternatives, m.Attacks})
	}
	for _, r := range doc.Data.TTS.Riders {
		out.Riders = append(out.Riders, TTSRider{r.Unit, r.FootModel, r.States})
	}
	for _, n := range doc.Data.TTS.Names {
		out.Names = append(out.Names, TTSName{n.Unit, n.From, n.To})
	}
	return out, nil
}

// TTSModels gives the fixed mappings to the Tabletop Simulator mod's model names.
func (b *BCBook) TTSModels() TTSModels { return b.ttsModels }
