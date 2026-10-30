package lean

// Campaign holds the engine tables and every rule parameter of the format (docs/definitions.md, "Engine parameters"): a value the engine uses is a field
// here, required, with its range; there is no code default. ParameterIDs below lists where each parameter lives. Only gold and influence.spend are open values.
type Campaign struct {
	Company           CompanyRules      `json:"company" required:"true"`
	Recruitment       Recruitment       `json:"recruitment" required:"true"`
	Influence         Influence         `json:"influence" required:"true"`
	Progression       Progression       `json:"progression" required:"true"`
	Injuries          Injuries          `json:"injuries" required:"true"`
	Experience        Experience        `json:"experience" required:"true"`
	BreakPoint        BreakPoint        `json:"breakPoint" required:"true"`
	Rating            RatingRules       `json:"rating" required:"true"`
	ScenarioSelection ScenarioSelection `json:"scenarioSelection" required:"true"`
	AftermathEffects  []AftermathEffect `json:"aftermathEffects,omitempty"`

	CharacteristicLimits CharacteristicLimits `json:"characteristicLimits,omitempty"`
	AgainstTheOdds       AgainstTheOdds       `json:"againstTheOdds,omitempty"`
	Gold                 Free                 `json:"gold,omitempty"` // open data: where Gold comes from; the app does not read it
	MatchInput           MatchInput           `json:"matchInput,omitempty"`
	Options              []CampaignOption     `json:"options,omitempty"`
}

// CompanyRules is `campaign.company`.
type CompanyRules struct {
	MaxModels         int      `json:"maxModels" required:"true" min:"1" max:"50"`          // B4
	Leaders           int      `json:"leaders" required:"true" min:"1" max:"3"`             // B1
	Sergeants         int      `json:"sergeants" required:"true" min:"0" max:"6"`           // B1
	HeroStart         HeroGain `json:"heroStart" required:"true"`                           // B3
	StartingInfluence int      `json:"startingInfluence" required:"true" min:"0" max:"100"` // A8
	EquipmentSlots    SlotPair `json:"equipmentSlots" required:"true"`                      // B6
	Oaths             Oaths    `json:"oaths,omitempty"`                                     // fallback oath texts by alignment
}

// HeroGain is what a new hero starts with.
type HeroGain struct {
	Might int `json:"might" required:"true" min:"0" max:"5"`
	Will  int `json:"will" required:"true" min:"0" max:"5"`
	Fate  int `json:"fate" required:"true" min:"0" max:"5"`
}

// SlotPair is the equipment slots of a model.
type SlotPair struct {
	Small int `json:"small" required:"true" min:"0" max:"10"`
	Large int `json:"large" required:"true" min:"0" max:"10"`
}

// Recruitment is `campaign.recruitment`: parameters only, the prose is dropped.
type Recruitment struct {
	Die    DiceSpec `json:"die" required:"true"` // A4: Count 1; the charts cover 1..Sides
	Adjust Adjust   `json:"adjust" required:"true"`
	Rare   RareRule `json:"rare" required:"true"`
}

// Adjust is how Influence moves a recruiting die.
type Adjust struct {
	MaxPerAttempt int    `json:"maxPerAttempt" required:"true" min:"0" max:"20"` // A1
	Direction     string `json:"direction" required:"true" enum:"up|down|both"`  // A2
	CostPerPoint  int    `json:"costPerPoint" required:"true" min:"0" max:"10"`  // A3
}

// RareRule says which models of the profile count towards Rare N.
type RareRule struct {
	InjuredCount  bool `json:"injuredCount" required:"true"`
	DeadCount     bool `json:"deadCount" required:"true"`
	PromotedCount bool `json:"promotedCount" required:"true"`
}

// Influence is `campaign.influence`: the gains (read) and spend, free prose about what Influence buys that the app does not read.
type Influence struct {
	Gains InfluenceGains `json:"gains" required:"true"`
	Spend Free           `json:"spend,omitempty"`
}

// InfluenceGains are the Influence of the War's Aftermath.
type InfluenceGains struct {
	Participating int `json:"participating" required:"true" min:"0" max:"20"`
	Draw          int `json:"draw" required:"true" min:"0" max:"20"`
	Win           int `json:"win" required:"true" min:"0" max:"20"`
}

// Progression is `campaign.progression`.
type Progression struct {
	Threshold int             `json:"threshold" required:"true" min:"1" max:"50"` // C1
	Warrior   WarriorProgress `json:"warrior" required:"true"`
	Hero      HeroProgress    `json:"hero" required:"true"`
}

// WarriorProgress is the warrior progression table.
type WarriorProgress struct {
	Roll        DiceSpec    `json:"roll" required:"true"` // C2: the table rows cover it
	Table       []TableRow  `json:"table,omitempty"`
	NoPromotion NoPromotion `json:"noPromotion" required:"true"` // C5
}

// NoPromotion is what a warrior with no promotion gains.
type NoPromotion struct {
	Courage      int `json:"courage" required:"true" min:"0" max:"5"`
	Intelligence int `json:"intelligence" required:"true" min:"0" max:"5"`
	Maximum      int `json:"maximum" required:"true" min:"1" max:"10"`
}

// HeroProgress is the hero progression.
type HeroProgress struct {
	Roll                DiceSpec `json:"roll" required:"true"`                                // C4
	RollsPerProgression int      `json:"rollsPerProgression" required:"true" min:"1" max:"6"` // C3
	Choose              int      `json:"choose" required:"true" min:"1" max:"6"`              // C3
}

// TableRow is a row of a table keyed by a roll; the other keys of the row are read by the slice that owns the table.
type TableRow struct {
	Roll Roll   `json:"roll" required:"true"`
	Key  string `json:"key,omitempty"`
	Name string `json:"name,omitempty"`
	Text string `json:"text,omitempty"`
}

// Experience is `campaign.experience`: the gains. The threshold lives in progression.threshold.
type Experience struct {
	Gains []ExperienceGain `json:"gains" required:"true"`
	Note  string           `json:"note,omitempty"`
}

// ExperienceGain is one way to earn Experience.
type ExperienceGain struct {
	Key        string   `json:"key" required:"true" min:"1"`
	Label      string   `json:"label,omitempty"`
	Amount     int      `json:"amount,omitempty" min:"0" max:"20"`
	Dice       DiceSpec `json:"dice,omitempty"` // D2
	Repeatable bool     `json:"repeatable,omitempty"`
}

// BreakPoint is `campaign.breakPoint`.
type BreakPoint struct {
	Default            BreakDefault     `json:"default" required:"true"`
	Threshold          BreakThreshold   `json:"threshold" required:"true"`
	WhenBroken         WhenBroken       `json:"whenBroken" required:"true"`
	ScenarioExceptions []BreakException `json:"scenarioExceptions,omitempty"`
}

// BreakDefault is the Break Point of a company with none of its own.
type BreakDefault struct {
	Percent int    `json:"percent" required:"true" min:"1" max:"100"`
	Of      string `json:"of,omitempty" enum:"startingModels"`
}

// BreakThreshold is the comparison against the Break Point.
type BreakThreshold struct {
	Comparison string `json:"comparison" required:"true" enum:"gt|gte"`
}

// WhenBroken is what happens to a broken company.
type WhenBroken struct {
	CourageTest CourageTest `json:"courageTest" required:"true"`
}

// CourageTest is the test a model of a broken company takes.
type CourageTest struct {
	Dice   DiceSpec `json:"dice" required:"true"`
	PassIf string   `json:"passIf,omitempty" enum:"totalAtLeastCourage"`
}

// BreakException is a scenario, or a scenario setting, that changes the Courage tests or the count of a Broken company. The live match
// shows its text; effect and who word it when there is no text.
type BreakException struct {
	Text            string `json:"text,omitempty"`
	Scenario        string `json:"scenario,omitempty" min:"1"`
	ScenarioSetting string `json:"scenarioSetting,omitempty" min:"1"`
	Rule            string `json:"rule,omitempty"`
	Effect          string `json:"effect,omitempty" enum:"noCourageTest|autoPassCourage|notCountedForBreakPoint|yetToArriveCountAsOnTheBattlefield"`
	Who             string `json:"who,omitempty" enum:"modelsOnTheHill|defenderWithin6OfTheGate|defenderBaggagePonies|neutralModels|ownModelsNotArrived"`
}

// RatingRules is `campaign.rating`.
type RatingRules struct {
	Hero HeroRating `json:"hero" required:"true"`
}

// HeroRating is how a hero is rated: Rating per point of each characteristic above the shared profile, per special rule and per minor special
// rule, and when the gear is paid at the hero rating. Absent numbers are 0.
type HeroRating struct {
	Wargear                   WargearRating `json:"wargear" required:"true"`
	PerPoint                  []RatingPoint `json:"perPoint,omitempty"`
	PerSpecialRule            int           `json:"perSpecialRule,omitempty" min:"0" max:"100"`
	PerMinorSpecialRule       int           `json:"perMinorSpecialRule,omitempty" min:"0" max:"100"`
	MaxMinorSpecialRuleRating int           `json:"maxMinorSpecialRuleRating,omitempty" min:"0" max:"1000"`
}

// RatingPoint is the Rating one point of a characteristic is worth in a hero.
type RatingPoint struct {
	Stat   string `json:"stat" required:"true" enum:"movement|fight|shoot|strength|defence|attack|wounds|courage|intelligence|might|will|fate" noun:"characteristic"`
	Rating int    `json:"rating" required:"true" min:"0" max:"100"`
}

// WargearRating decides when a hero pays the hero rating for gear.
type WargearRating struct {
	Threshold WargearThreshold `json:"threshold" required:"true"`
	Text      string           `json:"text,omitempty"`
}

// WargearThreshold: a hero whose Stats sum to SumAtLeast pays the hero rating.
type WargearThreshold struct {
	Stats      []string `json:"stats" required:"true" min:"1" enum:"movement|fight|shoot|strength|defence|attack|wounds|courage|intelligence|might|will|fate" noun:"characteristic"`
	SumAtLeast int      `json:"sumAtLeast" required:"true" min:"0" max:"20"`
}

// ScenarioSelection is `campaign.scenarioSelection`.
type ScenarioSelection struct {
	Die    DiceSpec        `json:"die" required:"true"` // Count 1
	Tables []ScenarioTable `json:"tables" required:"true"`
}

// ScenarioTable is one table of scenarios picked by the first die.
type ScenarioTable struct {
	Roll      Roll     `json:"roll" required:"true"`
	Scenarios []string `json:"scenarios" required:"true"`
}

// ParameterIDs maps each engine parameter id of the format to the json path (under campaign unless it starts with a top-level
// section) where the model holds it. The model test checks that every path resolves to a required field with a range, so the
// table is the index of the contract.
var ParameterIDs = map[string]string{
	"A1":  "campaign.recruitment.adjust.maxPerAttempt",
	"A2":  "campaign.recruitment.adjust.direction",
	"A3":  "campaign.recruitment.adjust.costPerPoint",
	"A4":  "campaign.recruitment.die",
	"A7":  "campaign.aftermathEffects[].range",
	"A8":  "campaign.company.startingInfluence",
	"A9":  "campaign.recruitment.rare.injuredCount",
	"B1":  "campaign.company.leaders",
	"B3":  "campaign.company.heroStart",
	"B4":  "campaign.company.maxModels",
	"B6":  "campaign.company.equipmentSlots",
	"B7":  "gear[].grantsSlots",
	"B8":  "limits[].maximumPercentage",
	"B9":  "limits[].roundUp",
	"B10": "creatures.rules.maxPerHero",
	"C1":  "campaign.progression.threshold",
	"C2":  "campaign.progression.warrior.roll",
	"C3":  "campaign.progression.hero.rollsPerProgression",
	"C4":  "campaign.progression.hero.roll",
	"C5":  "campaign.progression.warrior.noPromotion.maximum",
	"D1":  "campaign.injuries.roll",
	"D2":  "campaign.experience.gains[].dice",
	"D3":  "campaign.injuries.warrior[].statChanges[].stat",
	"E1":  "campaign.breakPoint.threshold.comparison",
	"E4":  "campaign.breakPoint.whenBroken.courageTest.dice",
	"E6":  "campaign.breakPoint.default.percent",
	"F1":  "campaign.rating.hero.wargear.threshold.sumAtLeast",
	"F5":  "campaign.scenarioSelection.die",
	"G3":  "gear[].who.race",
}
