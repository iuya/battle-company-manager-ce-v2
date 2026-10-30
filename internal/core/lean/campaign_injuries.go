package lean

// Injuries is `campaign.injuries`: the die, the two charts, the treatments and the prose around them. The chart rows and the treatments
// are read by the engine (core.LeanInjuries, core.LeanAftermath); applies, deathOfAHero, healing, retirement and rollModifier are rule
// text shown to the players or kept for the people who read the file.
type Injuries struct {
	Roll         DiceSpec           `json:"roll" required:"true"` // D1
	Applies      string             `json:"applies,omitempty"`
	DeathOfAHero string             `json:"deathOfAHero,omitempty"`
	Healing      InjuryHealingText  `json:"healing,omitempty"`
	Warrior      []InjuryRow        `json:"warrior,omitempty"`
	Hero         []InjuryRow        `json:"hero,omitempty"`
	Retirement   InjuryRetirement   `json:"retirement,omitempty"`
	RollModifier InjuryRollModifier `json:"rollModifier,omitempty"`
	Treatment    InjuryTreatment    `json:"treatment,omitempty"`
}

// InjuryHealingText is the rulebook text of how injuries are healed.
type InjuryHealingText struct {
	Hero    string `json:"hero,omitempty"`
	Warrior string `json:"warrior,omitempty"`
}

// InjuryRetirement says what happens to a hero that suffers a persistent injury for the retireAtCount-th time.
type InjuryRetirement struct {
	Text   string `json:"text,omitempty"`
	Result string `json:"result,omitempty" min:"1"` // the key of the chart row it counts as
}

// InjuryRollModifier is the prose about modifiers of the injury roll (Healing Herbs); the modifier itself is an aftermathEffects entry.
type InjuryRollModifier struct {
	Text            string `json:"text,omitempty"`
	AppliesTo       string `json:"appliesTo,omitempty" enum:"total2d6"`
	Clamp           []int  `json:"clamp,omitempty" min:"2" max:"2" emin:"0" emax:"100"`
	IncludesRerolls bool   `json:"includesRerolls,omitempty"`
	ReplacedBy      string `json:"replacedBy,omitempty"`
}

// InjuryEffects is the closed vocabulary of the effect codes of a chart row.
const InjuryEffects = "remove|missNextGame|fullRecovery|healOneInjury|reduceMovement|noShield|noTwoHandedWeapon|noPike|noBow|noStandFast|noHeroicActionsOnCompany"

// InjuryRow is a row of an injury chart. The engine reads effects (remove, missNextGame, fullRecovery) to decide what the result does, persistent,
// retireAtCount, statChanges, disables and grantsAtCount for a hero's lasting injuries, heals for Full Recovery, optionalReroll for T'is Just a Scratch.
type InjuryRow struct {
	Roll              Roll                  `json:"roll" required:"true"`
	Key               string                `json:"key,omitempty"`
	Name              string                `json:"name,omitempty"`
	Text              string                `json:"text,omitempty"`
	Effects           []string              `json:"effects,omitempty" enum:"remove|missNextGame|fullRecovery|healOneInjury|reduceMovement|noShield|noTwoHandedWeapon|noPike|noBow|noStandFast|noHeroicActionsOnCompany" noun:"injury effect"`
	GrantsExperience  *DiceGrant            `json:"grantsExperience,omitempty"` // D2
	GrantsInfluence   *DiceGrant            `json:"grantsInfluence,omitempty"`
	Experience        DiceSpec              `json:"experience,omitempty"`  // the die of grantsExperience, written for the reader of the file
	Influence         DiceSpec              `json:"influence,omitempty"`   // the die of grantsInfluence, written for the reader of the file
	StatChanges       []InjuryChange        `json:"statChanges,omitempty"` // D3
	Stat              map[string]int        `json:"stat,omitempty"`        // statChanges in short, for the reader of the file
	StatNote          string                `json:"stat_note,omitempty"`
	CancelsMissedGame bool                  `json:"cancelsMissedGame,omitempty"`
	Persistent        bool                  `json:"persistent,omitempty"`
	RetireAtCount     int                   `json:"retireAtCount,omitempty" min:"1" max:"20"`
	Healable          bool                  `json:"healable,omitempty"`
	HealableBy        []string              `json:"healableBy,omitempty" min:"1"`
	ClearsAfterGames  int                   `json:"clearsAfterGames,omitempty" min:"1" max:"20"`
	OptionalReroll    bool                  `json:"optionalReroll,omitempty"`
	Reroll            *InjuryReroll         `json:"reroll,omitempty"`
	Disables          []InjuryDisable       `json:"disables,omitempty"`
	GrantsAtCount     map[string]CountGrant `json:"grantsAtCount,omitempty"`
	Heals             *InjuryHeals          `json:"heals,omitempty"`
}

// InjuryChange is a characteristic an injury changes: Amount points, once or Per count of the injury.
type InjuryChange struct {
	Stat              string `json:"stat" required:"true" enum:"movement|fight|shoot|strength|defence|attack|wounds|courage|intelligence|might|will|fate" noun:"characteristic"`
	Amount            int    `json:"amount" required:"true" min:"-20" max:"20"`
	Per               string `json:"per,omitempty" enum:"count"`
	Minimum           *int   `json:"minimum,omitempty" nullable:"true" min:"0" max:"20"`
	Permanent         bool   `json:"permanent,omitempty"`
	CapAtPathMaximum  bool   `json:"capAtPathMaximum,omitempty"`
	RaisesPathMaximum bool   `json:"raisesPathMaximum,omitempty"`
}

// DiceGrant is Experience or Influence rolled on a die.
type DiceGrant struct {
	Dice DiceSpec `json:"dice" required:"true"`
}

// InjuryReroll is the optional second roll of T'is Just a Scratch.
type InjuryReroll struct {
	Dice                       DiceSpec `json:"dice" required:"true"`
	Optional                   bool     `json:"optional,omitempty"`
	SecondResultStands         bool     `json:"secondResultStands,omitempty"`
	IfScratchAgainMissNextGame bool     `json:"ifScratchAgainMissNextGame,omitempty"`
}

// InjuryAbilities is the closed vocabulary of the abilities an injury can take away from a hero.
const InjuryAbilities = "standFast|heroicActionsOnCompany"

// InjuryDisable is what a lasting injury takes from a hero: the benefit of gear of some classes or by name, or an ability.
type InjuryDisable struct {
	Kind    string   `json:"kind" required:"true" enum:"gearClass|gearItem|ability"`
	Classes []string `json:"classes,omitempty" enum:"melee|spear|two_handed|mount|bow|armour|throwing|shield|instrument|consumable|lance|banner|missile|crossbow|other" noun:"gear class"`
	Names   []string `json:"names,omitempty" min:"1"`
	Key     string   `json:"key,omitempty" enum:"standFast|heroicActionsOnCompany" noun:"ability"`
}

// CountGrant is the special rule a hero gets from a given count of an injury on.
type CountGrant struct {
	SpecialRule string `json:"specialRule" required:"true" min:"1"`
}

// InjuryHeals is what a healing result removes: Count injuries of the listed keys, chosen by the player.
type InjuryHeals struct {
	Choice string   `json:"choice" required:"true" enum:"player"`
	Count  int      `json:"count" required:"true" min:"1" max:"10"`
	Keys   []string `json:"keys" required:"true" min:"1"`
}

// InjuryTreatment is how Influence treats injuries (Battle Companies, Step 3): the hero methods, the warrior's and what removing an injury does.
type InjuryTreatment struct {
	Currency      string           `json:"currency,omitempty" enum:"influence"`
	Phase         string           `json:"phase,omitempty" enum:"companyManagement"`
	Hero          []HeroTreatment  `json:"hero,omitempty"`
	Warrior       WarriorTreatment `json:"warrior,omitempty"`
	RemovalEffect TreatmentRemoval `json:"removalEffect,omitempty"`
}

// HeroTreatment is one way to treat a hero: an attempt on a die, or the healers.
type HeroTreatment struct {
	Key             string        `json:"key" required:"true" enum:"attempt|healers" noun:"treatment"`
	Text            string        `json:"text,omitempty"`
	Cost            int           `json:"cost" required:"true" min:"0" max:"20"`
	Roll            *TreatRoll    `json:"roll,omitempty" nullable:"true"`
	Healable        []string      `json:"healable,omitempty" min:"1"`
	MaxAttempts     *int          `json:"maxAttempts,omitempty" nullable:"true" min:"1" max:"100"`
	MissNextGame    bool          `json:"missNextGame,omitempty"`
	RemovesInjuries int           `json:"removesInjuries,omitempty" min:"0" max:"10"`
	OnFailure       *TreatFailure `json:"onFailure,omitempty"`
}

// TreatRoll is the die of a treatment attempt: the total must reach Target, and extra Influence raises it.
type TreatRoll struct {
	Dice                     DiceSpec      `json:"dice" required:"true"`
	Compare                  string        `json:"compare,omitempty" enum:"gte"`
	Target                   int           `json:"target" required:"true" min:"1" max:"100"`
	Modifier                 TreatModifier `json:"modifier,omitempty"`
	NaturalMaxIsAlwaysEnough bool          `json:"naturalMaxIsAlwaysEnough,omitempty"`
}

// TreatModifier is how extra Influence raises a treatment roll, like a Might point.
type TreatModifier struct {
	Currency  string `json:"currency,omitempty" enum:"influence"`
	Direction string `json:"direction,omitempty" enum:"up"`
	PerPoint  int    `json:"perPoint,omitempty" min:"0" max:"10"`
	Ceiling   int    `json:"ceiling,omitempty" min:"0" max:"100"`
	MaxPoints *int   `json:"maxPoints,omitempty" nullable:"true" min:"0" max:"100"`
	Like      string `json:"like,omitempty"`
}

// TreatFailure is what a failed attempt costs.
type TreatFailure struct {
	CostKept bool   `json:"costKept,omitempty"`
	Effect   string `json:"effect,omitempty" enum:"none"`
}

// WarriorTreatment is the treatment of a warrior: Cost Influence removes the Injured condition.
type WarriorTreatment struct {
	Key      string     `json:"key,omitempty"`
	Text     string     `json:"text,omitempty"`
	Cost     int        `json:"cost,omitempty" min:"0" max:"20"`
	Roll     *TreatRoll `json:"roll,omitempty" nullable:"true"`
	Removes  string     `json:"removes,omitempty" enum:"injured"`
	Requires string     `json:"requires,omitempty" enum:"injured"`
}

// TreatmentRemoval is what removing an injury does to the hero's counts.
type TreatmentRemoval struct {
	Choice         string `json:"choice,omitempty" enum:"player"`
	CountDelta     int    `json:"countDelta,omitempty" min:"-10" max:"0"`
	DerivedEffects string `json:"derivedEffects,omitempty" enum:"recomputeFromCounts"`
}
