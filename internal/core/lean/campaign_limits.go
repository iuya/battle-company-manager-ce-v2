package lean

// CharacteristicLimits is `campaign.characteristicLimits`: which characteristics improve downwards, the highest each one goes, the average of a
// profile (the base of the hero rating) and the cap on the total improvement of Courage and Intelligence. core.LeanProgression and core.LeanRating
// read it.
type CharacteristicLimits struct {
	LowerIsBetter []string               `json:"lowerIsBetter,omitempty" enum:"movement|fight|shoot|strength|defence|attack|wounds|courage|intelligence|might|will|fate" noun:"characteristic"`
	Peak          PeakLimits             `json:"peak,omitempty"`
	Average       map[string]int         `json:"average,omitempty"`
	Increases     CharacteristicIncrease `json:"increases,omitempty"`
}

// StatLimit is how far one characteristic may be improved. A key that is absent sets no limit of that kind: max is the highest value of a
// characteristic where higher is better, best the best (lowest) value of one where lower is better, maxIncrease how many points above the starting
// profile (without gear) it may go. The same shape is used by a path's maximums.
type StatLimit struct {
	Max         *int `json:"max,omitempty" min:"1" max:"20"`
	Best        *int `json:"best,omitempty" min:"1" max:"20"`
	MaxIncrease *int `json:"maxIncrease,omitempty" min:"0" max:"20"`
}

// PeakLimits are the overall limits of the characteristics, one entry per characteristic, and the Will maximum some paths have of their own.
type PeakLimits struct {
	Movement         *StatLimit     `json:"movement,omitempty"`
	Fight            *StatLimit     `json:"fight,omitempty"`
	Shoot            *StatLimit     `json:"shoot,omitempty"`
	Strength         *StatLimit     `json:"strength,omitempty"`
	Defence          *StatLimit     `json:"defence,omitempty"`
	Attack           *StatLimit     `json:"attack,omitempty"`
	Wounds           *StatLimit     `json:"wounds,omitempty"`
	Courage          *StatLimit     `json:"courage,omitempty"`
	Intelligence     *StatLimit     `json:"intelligence,omitempty"`
	Might            *StatLimit     `json:"might,omitempty"`
	Will             *StatLimit     `json:"will,omitempty"`
	Fate             *StatLimit     `json:"fate,omitempty"`
	WillMaxOverrides map[string]int `json:"willMaxOverrides,omitempty"`
}

// ByStat lists the limits that are set, by characteristic.
func (p PeakLimits) ByStat() map[string]StatLimit {
	out := map[string]StatLimit{}
	for stat, l := range map[string]*StatLimit{"movement": p.Movement, "fight": p.Fight, "shoot": p.Shoot, "strength": p.Strength, "defence": p.Defence,
		"attack": p.Attack, "wounds": p.Wounds, "courage": p.Courage, "intelligence": p.Intelligence, "might": p.Might, "will": p.Will, "fate": p.Fate} {
		if l != nil {
			out[stat] = *l
		}
	}
	return out
}

// CharacteristicIncrease limits the improvements a hero can add up.
type CharacteristicIncrease struct {
	CourageIntelligenceTotalCap *TotalCap `json:"courageIntelligenceTotalCap,omitempty"`
}

// TotalCap is the most Courage and Intelligence may be improved in total, and whether the improvement from gear counts in it.
type TotalCap struct {
	Value        int  `json:"value" required:"true" min:"1" max:"10"`
	IncludesGear bool `json:"includesGear,omitempty"`
}
