package lean

// AftermathEffect is an entry of `campaign.aftermathEffects`: a reward of a scenario, an Against the Odds bonus paid in the aftermath, or a
// consumable item. The kinds influence, experience and freeEquipment pay a scenario reward; injuryRollModifier and injuryRollReplacement are the
// consumables; rendezvous and vantagePoint (by key) move a die with their range. The other keys are the rule in data for the reader of the file.
type AftermathEffect struct {
	Key                   string      `json:"key" required:"true" min:"1"`
	Kind                  string      `json:"kind" required:"true" enum:"rerolls|influence|experiencePerModel|injuryRollModifier|injuryRollReplacement|reinforcementRollModifier|freeEquipment|experience|scenarioTableModifier|magicalPower"`
	Text                  string      `json:"text,omitempty"`
	Amount                int         `json:"amount,omitempty" min:"0" max:"100"`
	MaxInfluenceValue     int         `json:"maxInfluenceValue,omitempty" min:"0" max:"100"`
	Range                 []int       `json:"range,omitempty" min:"2" max:"2" emin:"-6" emax:"6"`
	Scenario              string      `json:"scenario,omitempty" min:"1"`
	Source                string      `json:"source,omitempty"`
	Who                   string      `json:"who,omitempty" min:"1"`
	To                    string      `json:"to,omitempty" enum:"leader"`
	When                  string      `json:"when,omitempty" enum:"beforeInjuryRolls|firstReinforcementRollThisAftermath|nextScenarioTableRoll|nextGame"`
	Count                 int         `json:"count,omitempty" min:"1" max:"20"`
	Rolls                 int         `json:"rolls,omitempty" min:"1" max:"20"`
	UsableIn              []string    `json:"usableIn,omitempty" enum:"game|warsAftermath" noun:"phase"`
	Gains                 *ATOReward  `json:"gains,omitempty"`
	ActivatedBy           ActivatedBy `json:"activatedBy,omitempty"`
	Applies               string      `json:"applies,omitempty" enum:"eachInjuryRoll"`
	Choice                string      `json:"choice,omitempty" enum:"player"`
	Clamp                 []int       `json:"clamp,omitempty" min:"2" max:"2" emin:"0" emax:"100"`
	Consumes              string      `json:"consumes,omitempty" enum:"oneItem|thisItem"`
	IncludesHeroReroll    bool        `json:"includesHeroReroll,omitempty"`
	IncludesHealOneInjury bool        `json:"includesHealOneInjury,omitempty"`
	Modifier              int         `json:"modifier,omitempty" min:"-12" max:"12"`
	Stackable             bool        `json:"stackable,omitempty"`
	UsesPerAftermath      int         `json:"usesPerAftermath,omitempty" min:"1" max:"20"`
	NoRoll                bool        `json:"noRoll,omitempty"`
	Result                string      `json:"result,omitempty" min:"1"`
	Die                   string      `json:"die,omitempty" enum:"second"`
	IgnoresWill           bool        `json:"ignoresWill,omitempty"`
	Power                 string      `json:"power,omitempty" min:"1"`
}

// ActivatedBy says who may use a consumable and when.
type ActivatedBy struct {
	Carrier     string   `json:"carrier,omitempty" enum:"hero|anyModel"`
	FledModelIs string   `json:"fledModelIs,omitempty" enum:"notCasualty"`
	Requires    []string `json:"requires,omitempty" enum:"participated|casualty|notCasualty" noun:"condition"`
}
