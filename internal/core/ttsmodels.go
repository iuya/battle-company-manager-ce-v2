package core

import ()

// TTSModels is data.ttsModels of the Battle Companies file: the fixed mappings from Battle Companies to the model names of the
// Tabletop Simulator MESBG FTC mod (docs/tabletop-simulator.md). Short names only; the mod's catalogue is never stored.
type TTSModels struct {
	StandIns []TTSStandIn // wanderers and creatures, which the mod has no figure for
	Mounts   []TTSMount   // bought mounts that fight (vanilla Attacks above 0) and the model of their figure
	Riders   []TTSRider   // the cavalry units
	Names    []TTSName    // option spellings that differ between the vanilla file and the mod
	// DefaultOptions: units the bag has no bare figure of; MarkerOptions: alias markers left out of the key when they stand alone.
	DefaultOptions []TTSDefaultOptions
	MarkerOptions  []TTSMarkerOption
}

// TTSDefaultOptions: a Unit that carries no option is requested with Options (the bag has no bare figure of it).
type TTSDefaultOptions struct {
	Unit    string
	Options []string
}

// TTSMarkerOption: Option of Unit only names the unit (a company alias selects it); as the only option it is left out of the key.
type TTSMarkerOption struct{ Unit, Option string }

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

// TTSModels gives the fixed mappings to the Tabletop Simulator mod's model names.
func (b *BCBook) TTSModels() TTSModels { return b.ttsModels }
