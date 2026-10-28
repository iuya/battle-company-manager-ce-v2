package lean

// Campaign holds the engine tables and every rule parameter of design section 12 (decision 12): a value the engine uses is a field
// here, required, with its range; there is no code default. Ids (A1, B3 ...) are those of docs/engine-parameters-audit.md and of
// the table in design 12.2; ParameterIDs below lists where each lives. Tables whose reader moves in a later slice are Free or
// carry a `rest` map so the shipped keys survive until that slice types them.
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

	CharacteristicLimits Free `json:"characteristicLimits,omitempty"`
	AgainstTheOdds       Free `json:"againstTheOdds,omitempty"` // H4: typed by slice P5
	Gold                 Free `json:"gold,omitempty"`
	MatchInput           Free `json:"matchInput,omitempty"` // prose shown to players (design 4.8, slice 10)
	Options              Free `json:"options,omitempty"`
}

// CompanyRules is `campaign.company` (design 4.8, 12.2 A8, B1, B3, B4, B6).
type CompanyRules struct {
	MaxModels         int      `json:"maxModels" required:"true" min:"1" max:"50"`          // B4
	Leaders           int      `json:"leaders" required:"true" min:"1" max:"3"`             // B1
	Sergeants         int      `json:"sergeants" required:"true" min:"0" max:"6"`           // B1
	HeroStart         HeroGain `json:"heroStart" required:"true"`                           // B3
	StartingInfluence int      `json:"startingInfluence" required:"true" min:"0" max:"100"` // A8
	EquipmentSlots    SlotPair `json:"equipmentSlots" required:"true"`                      // B6
	Oaths             Free     `json:"oaths,omitempty"`                                     // fallback oath texts by alignment (design 3)
}

// HeroGain is what a new hero starts with (12.2 B3).
type HeroGain struct {
	Might int `json:"might" required:"true" min:"0" max:"5"`
	Will  int `json:"will" required:"true" min:"0" max:"5"`
	Fate  int `json:"fate" required:"true" min:"0" max:"5"`
}

// SlotPair is the equipment slots of a model (12.2 B6).
type SlotPair struct {
	Small int `json:"small" required:"true" min:"0" max:"10"`
	Large int `json:"large" required:"true" min:"0" max:"10"`
}

// Recruitment is `campaign.recruitment`: parameters only, the prose is dropped (decision 14).
type Recruitment struct {
	Die    DiceSpec `json:"die" required:"true"` // A4: Count 1; the charts cover 1..Sides
	Adjust Adjust   `json:"adjust" required:"true"`
	Rare   RareRule `json:"rare" required:"true"`
}

// Adjust is how Influence moves a recruiting die (12.2 A1 to A3).
type Adjust struct {
	MaxPerAttempt int    `json:"maxPerAttempt" required:"true" min:"0" max:"20"` // A1
	Direction     string `json:"direction" required:"true" enum:"up|down|both"`  // A2
	CostPerPoint  int    `json:"costPerPoint" required:"true" min:"0" max:"10"`  // A3
}

// RareRule says which models of the profile count towards Rare N (12.2 A9).
type RareRule struct {
	InjuredCount  bool `json:"injuredCount" required:"true"`
	DeadCount     bool `json:"deadCount" required:"true"`
	PromotedCount bool `json:"promotedCount" required:"true"`
}

// Influence is `campaign.influence`: the gains (read) and the adjustRoll entry's player-facing text.
type Influence struct {
	Gains InfluenceGains `json:"gains" required:"true"`
	Spend Free           `json:"spend,omitempty"`
}

// InfluenceGains are the Influence of the War's Aftermath (12.2).
type InfluenceGains struct {
	Participating int `json:"participating" required:"true" min:"0" max:"20"`
	Draw          int `json:"draw" required:"true" min:"0" max:"20"`
	Win           int `json:"win" required:"true" min:"0" max:"20"`
}

// Progression is `campaign.progression` (12.2 C1 to C5).
type Progression struct {
	Threshold int             `json:"threshold" required:"true" min:"1" max:"50"` // C1
	Warrior   WarriorProgress `json:"warrior" required:"true"`
	Hero      HeroProgress    `json:"hero" required:"true"`
	Rest      map[string]any  `json:"-" rest:"true"` // xp, timing, influenceAdjust ... read by later slices
}

// WarriorProgress is the warrior progression table.
type WarriorProgress struct {
	Roll        DiceSpec       `json:"roll" required:"true"` // C2: the table rows cover it
	Table       []TableRow     `json:"table,omitempty"`
	NoPromotion NoPromotion    `json:"noPromotion" required:"true"` // C5
	Rest        map[string]any `json:"-" rest:"true"`
}

// NoPromotion is what a warrior with no promotion gains (12.2 C5).
type NoPromotion struct {
	Courage      int `json:"courage" required:"true" min:"0" max:"5"`
	Intelligence int `json:"intelligence" required:"true" min:"0" max:"5"`
	Maximum      int `json:"maximum" required:"true" min:"1" max:"10"`
}

// HeroProgress is the hero progression (12.2 C3, C4).
type HeroProgress struct {
	Roll                DiceSpec       `json:"roll" required:"true"`                                // C4
	RollsPerProgression int            `json:"rollsPerProgression" required:"true" min:"1" max:"6"` // C3
	Choose              int            `json:"choose" required:"true" min:"1" max:"6"`              // C3
	Rest                map[string]any `json:"-" rest:"true"`
}

// TableRow is a row of a table keyed by a roll; the other keys of the row are read by the slice that owns the table.
type TableRow struct {
	Roll Roll           `json:"roll" required:"true"`
	Key  string         `json:"key,omitempty"`
	Name string         `json:"name,omitempty"`
	Text string         `json:"text,omitempty"`
	Rest map[string]any `json:"-" rest:"true"`
}

// Injuries is `campaign.injuries` (12.2 D1 to D3).
type Injuries struct {
	Roll    DiceSpec       `json:"roll" required:"true"` // D1
	Warrior []InjuryRow    `json:"warrior,omitempty"`
	Hero    []InjuryRow    `json:"hero,omitempty"`
	Rest    map[string]any `json:"-" rest:"true"` // treatment, retirement, rollModifier ...
}

// InjuryRow is a row of an injury chart.
type InjuryRow struct {
	Roll              Roll           `json:"roll" required:"true"`
	Key               string         `json:"key,omitempty"`
	Name              string         `json:"name,omitempty"`
	Text              string         `json:"text,omitempty"`
	GrantsExperience  *DiceGrant     `json:"grantsExperience,omitempty"` // D2
	StatChanges       []StatChange   `json:"statChanges,omitempty"`      // D3
	CancelsMissedGame bool           `json:"cancelsMissedGame,omitempty"`
	Rest              map[string]any `json:"-" rest:"true"`
}

// DiceGrant is Experience rolled on a die (12.2 D2).
type DiceGrant struct {
	Dice DiceSpec `json:"dice" required:"true"`
}

// Experience is `campaign.experience`: the gains (12.2 D2). The threshold lives in progression.threshold.
type Experience struct {
	Gains []ExperienceGain `json:"gains" required:"true"`
	Note  string           `json:"note,omitempty"`
}

// ExperienceGain is one way to earn Experience.
type ExperienceGain struct {
	Key        string   `json:"key" required:"true" min:"1"`
	Label      string   `json:"label,omitempty"`
	Amount     int      `json:"amount,omitempty" min:"0" max:"20"` // H7
	Dice       DiceSpec `json:"dice,omitempty"`                    // D2
	Repeatable bool     `json:"repeatable,omitempty"`
}

// BreakPoint is `campaign.breakPoint` (12.2 E1, E4, E6).
type BreakPoint struct {
	Default    BreakDefault   `json:"default" required:"true"`
	Threshold  BreakThreshold `json:"threshold" required:"true"`
	WhenBroken WhenBroken     `json:"whenBroken" required:"true"`
	Rest       map[string]any `json:"-" rest:"true"` // startingModels, casualty ... fixed choices documented, prose
}

// BreakDefault is the Break Point of a company with none of its own (E6).
type BreakDefault struct {
	Percent int    `json:"percent" required:"true" min:"1" max:"100"`
	Of      string `json:"of,omitempty"`
}

// BreakThreshold is the comparison against the Break Point (E1).
type BreakThreshold struct {
	Comparison string `json:"comparison" required:"true" enum:"gt|gte"`
}

// WhenBroken is what happens to a broken company (E4).
type WhenBroken struct {
	CourageTest CourageTest    `json:"courageTest" required:"true"`
	Rest        map[string]any `json:"-" rest:"true"`
}

// CourageTest is the test a model of a broken company takes (E4).
type CourageTest struct {
	Dice DiceSpec       `json:"dice" required:"true"`
	Rest map[string]any `json:"-" rest:"true"`
}

// RatingRules is `campaign.rating` (12.2 F1).
type RatingRules struct {
	Hero HeroRating     `json:"hero" required:"true"`
	Rest map[string]any `json:"-" rest:"true"`
}

// HeroRating is how a hero is rated.
type HeroRating struct {
	Wargear WargearRating  `json:"wargear" required:"true"`
	Rest    map[string]any `json:"-" rest:"true"` // perPoint, perSpecialRule ... (H4, slice P5)
}

// WargearRating decides when a hero pays the hero rating for gear.
type WargearRating struct {
	Threshold WargearThreshold `json:"threshold" required:"true"`
	Text      string           `json:"text,omitempty"`
}

// WargearThreshold: a hero whose Stats sum to SumAtLeast pays the hero rating (F1).
type WargearThreshold struct {
	Stats      []string `json:"stats" required:"true" min:"1" enum:"movement|fight|shoot|strength|defence|attack|wounds|courage|intelligence|might|will|fate" noun:"characteristic"`
	SumAtLeast int      `json:"sumAtLeast" required:"true" min:"0" max:"20"`
}

// ScenarioSelection is `campaign.scenarioSelection` (12.2 F5).
type ScenarioSelection struct {
	Die    DiceSpec        `json:"die" required:"true"` // Count 1
	Tables []ScenarioTable `json:"tables" required:"true"`
	Rest   map[string]any  `json:"-" rest:"true"` // text, layout, maelstromOfBattle
}

// ScenarioTable is one table of scenarios picked by the first die.
type ScenarioTable struct {
	Roll      Roll     `json:"roll" required:"true"`
	Scenarios []string `json:"scenarios" required:"true"`
}

// AftermathEffect is an entry of `campaign.aftermathEffects`; Range is read for rendezvous and vantagePoint (12.2 A7).
type AftermathEffect struct {
	Key   string         `json:"key" required:"true" min:"1"`
	Range []int          `json:"range,omitempty" min:"2" max:"2" emin:"-6" emax:"6"`
	Rest  map[string]any `json:"-" rest:"true"`
}

// ParameterIDs maps each engine parameter id of design 12.2 to the json path (under campaign unless it starts with a top-level
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
