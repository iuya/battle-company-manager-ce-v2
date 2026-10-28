// Package lean is the typed model, strict decoder, name resolution and validation of the lean Battle Companies format
// (schemaVersion 2, docs/definitions-lean-design.md). It is pure: it imports only the standard library, never package core, so
// core can adapt it and the web layer never needs to know it.
//
// model.go is the contract. Every struct field carries a json tag and, where the format needs them, validation tags read by the
// binder (bind.go):
//
//	required:"true"   the key must be present (and not null)
//	enum:"a|b|c"      closed vocabulary (strings)
//	min:"n" max:"n"   inclusive range of an int or a float; for a string or a list the length
//	rest:"true"       on a map[string]any field: keys the struct does not name are kept there instead of being errors
//
// A field of type Free (any) is accepted as is: the section is read by a later slice, which types it. Types that implement Binder
// (DiceSpec, Roll, Rating, RuleRef, Scalar) read polymorphic values.
package lean

// Free is a value the format does not type yet (its shape is fixed by the slice that moves its reader to the lean model).
type Free = any

// Document is the whole Battle Companies file (design sections 1 and 2): the header and the sections.
type Document struct {
	SchemaVersion int      `json:"schemaVersion" required:"true" min:"1" max:"1000"` // separate from Version: 2 for this format
	Name          string   `json:"name" required:"true" min:"1"`
	Version       Scalar   `json:"version" required:"true"` // the rulebook version, "1.1"
	Credit        Free     `json:"credit,omitempty"`
	Extends       string   `json:"extends,omitempty"` // the vanilla file it builds on (header marker)
	Campaign      Campaign `json:"campaign" required:"true"`

	Rules     []Rule     `json:"rules,omitempty"`
	Magic     *Magic     `json:"magic,omitempty"`
	Gear      []Gear     `json:"gear,omitempty"`
	Paths     []Path     `json:"paths,omitempty"`
	Scenarios []Scenario `json:"scenarios,omitempty"`
	Companies []Company  `json:"companies,omitempty"`
	Units     []Unit     `json:"units,omitempty"` // shared units
	Limits    []Limit    `json:"limits,omitempty"`
	Creatures *Followers `json:"creatures,omitempty"`
	Wanderers *Followers `json:"wanderers,omitempty"`
	Vanilla   *Vanilla   `json:"vanilla,omitempty"`
	Auras     *Auras     `json:"auras,omitempty"`
}

// Characteristics is the closed vocabulary of stats (design 4.4 `stat`, 12.2 F1/D3).
const Characteristics = "movement|fight|shoot|strength|defence|attack|wounds|courage|intelligence|might|will|fate"

// Company is a Battle Company (design 4.1). The 25 old effects[] kinds are typed fields (table 4.1.1).
type Company struct {
	Name           string        `json:"name" required:"true" min:"1"`
	ID             string        `json:"id,omitempty"`
	Aka            []string      `json:"aka,omitempty"`
	Alignment      string        `json:"alignment" required:"true" enum:"good|evil"`
	Description    string        `json:"description,omitempty"`
	Epigraph       *Epigraph     `json:"epigraph,omitempty"`
	Faction        string        `json:"faction,omitempty"` // default: the name
	Oath           *Oath         `json:"oath,omitempty"`
	Extends        string        `json:"extends,omitempty"`
	MaxModels      int           `json:"maxModels,omitempty" min:"1" max:"50"` // default campaign.company.maxModels; Elite 12
	BreakPoint     int           `json:"breakPoint,omitempty" min:"1" max:"100"`
	Start          *Start        `json:"start,omitempty"`
	Reinforcements *Chart        `json:"reinforcements,omitempty"`
	Special        *SpecialChart `json:"special,omitempty"`
	Advancements   []Advancement `json:"advancements,omitempty"`
	HeroUpgrades   []HeroUpgrade `json:"heroUpgrades,omitempty"`
	Units          []Unit        `json:"units,omitempty"` // company-local units
	Rules          []Rule        `json:"rules,omitempty"` // company-local rule definitions, hoisted
	Gear           []Gear        `json:"gear,omitempty"`  // unique wargear
	Promotions     []Promotion   `json:"promotions,omitempty"`
	Heroes         *Heroes       `json:"heroes,omitempty"`
	Recruit        *Recruit      `json:"recruit,omitempty"`
	Limits         *CompanyLimit `json:"limits,omitempty"`
	OwnPath        *OwnPath      `json:"ownPath,omitempty"`
	Substitutions  Free          `json:"substitutions,omitempty"` // ruleSubstitution, shape fixed by slice 7
	Exchanges      Free          `json:"exchanges,omitempty"`     // ruleExchange, slice 7
	Granted        Free          `json:"granted,omitempty"`       // grantedRule, slice 7
	Notes          []string      `json:"notes,omitempty"`
	SpecialRules   []string      `json:"specialRules,omitempty"`
}

// Epigraph is a quote with its comment (design 4.1).
type Epigraph struct {
	Quote string `json:"quote"`
	Text  string `json:"text,omitempty"`
}

// Oath is the vow and text of a company; {name} is replaced by the company's name (design 3).
type Oath struct {
	Vow  string `json:"vow,omitempty"`
	Text string `json:"text"`
}

// Start is the starting force: gold and the units (design 4.1).
type Start struct {
	Gold  int         `json:"gold" min:"0" max:"1000"`
	Units []StartUnit `json:"units" required:"true"`
}

// StartUnit is one line of the starting force.
type StartUnit struct {
	Unit  string   `json:"unit" required:"true" min:"1"`
	Count int      `json:"count,omitempty" min:"1" max:"50"` // default 1
	With  []string `json:"with,omitempty"`
}

// Chart is the reinforcement chart (design 4.1): the influence cost and the rows by roll.
type Chart struct {
	Influence int        `json:"influence" min:"0" max:"100"`
	Rows      []ChartRow `json:"rows" required:"true"`
}

// SpecialChart is the special units chart (design 4.1).
type SpecialChart struct {
	Rows []ChartRow `json:"rows" required:"true"`
}

// ChartRow is one row of a reinforcement or special chart (design 3, "company start / chart rows").
type ChartRow struct {
	Roll    Roll     `json:"roll" required:"true"`
	Unit    string   `json:"unit,omitempty"`
	Nothing bool     `json:"nothing,omitempty"`
	Blocks  bool     `json:"blocks,omitempty"` // a nothing row that blocks further rolls
	Count   int      `json:"count,omitempty" min:"1" max:"50"`
	Rare    int      `json:"rare,omitempty" min:"0" max:"50"`
	With    []string `json:"with,omitempty"`
}

// Advancement says which units a unit may be promoted to (design 4.1).
type Advancement struct {
	From string   `json:"from" required:"true"`
	To   []string `json:"to" required:"true"`
}

// HeroUpgrade names a rule a hero may take; an inline Definition defines it (design 4.1).
type HeroUpgrade struct {
	Name       string `json:"name" required:"true" min:"1"`
	Flavour    string `json:"flavour,omitempty"`
	Definition string `json:"definition,omitempty"` // inline rule definition, hoisted into the rule index
	Source     string `json:"source,omitempty"`
}

// Promotion is a hero promotion of a unit (design 4.1.1 heroPromotion).
type Promotion struct {
	From       string   `json:"from" required:"true"`
	On         string   `json:"on" required:"true" enum:"noChange|promoted|heroInTheMaking"`
	Becomes    string   `json:"becomes" required:"true"`
	Keep       []string `json:"keep,omitempty"`
	Drop       []string `json:"drop,omitempty"`
	StatGain   bool     `json:"statGain,omitempty"`
	PathChoice string   `json:"pathChoice,omitempty"`
	Text       string   `json:"text,omitempty"`
}

// Heroes restricts who may be a starting hero (design 4.1.1).
type Heroes struct {
	Allowed       []string `json:"allowed,omitempty"`
	Excluded      []string `json:"excluded,omitempty"`
	NoStartGain   []string `json:"noStartGain,omitempty"`
	RecruitAsHero []string `json:"recruitAsHero,omitempty"`
}

// Recruit holds the recruitment exceptions of a company (design 4.1.1).
type Recruit struct {
	Fallback Free `json:"fallback,omitempty"`
	Replace  Free `json:"replace,omitempty"`
	Balance  Free `json:"balance,omitempty"`
}

// CompanyLimit overrides the limits of one company (design 4.1.1 limitExemption, limitOverride, companyLimit).
type CompanyLimit struct {
	Exempt   Free `json:"exempt,omitempty"`
	Override Free `json:"override,omitempty"`
	Company  Free `json:"company,omitempty"`
}

// OwnPath lists the units On Their Own Path (design section 9 verdict 3).
type OwnPath struct {
	Units []string `json:"units" required:"true"`
	Path  string   `json:"path,omitempty"` // default "On Their Own Path"
}

// Unit is a company-local or shared unit (design 4.2): a base plus deltas, a brand-new unit, or an annotation of a lower-layer unit.
type Unit struct {
	Name        string     `json:"name,omitempty"` // default: Extends (an annotation)
	ID          string     `json:"id,omitempty"`
	Aka         []string   `json:"aka,omitempty"`
	Extends     string     `json:"extends,omitempty"`
	With        []string   `json:"with,omitempty"` // options pre-selected
	AddRules    []string   `json:"addRules,omitempty"`
	AddKeywords []string   `json:"addKeywords,omitempty"`
	Note        string     `json:"note,omitempty"`
	Types       *TypeDelta `json:"types,omitempty"`
	Composite   Free       `json:"composite,omitempty"`
	Includes    Free       `json:"includes,omitempty"`
	Models      Free       `json:"models,omitempty"`
	OwnPath     *OwnPath   `json:"ownPath,omitempty"`
	TTS         *UnitTTS   `json:"tts,omitempty"`
	Overrides   bool       `json:"overrides,omitempty"`
	Profile     Free       `json:"profile,omitempty"` // a brand-new unit (no extends): full definition
	Options     Free       `json:"options,omitempty"`
	Points      int        `json:"points,omitempty" min:"0" max:"10000"`
	Definition  Free       `json:"definition,omitempty"`
}

// TypeDelta adds and removes unit types (design 4.2).
type TypeDelta struct {
	Add    []string `json:"add,omitempty"`
	Remove []string `json:"remove,omitempty"`
}

// UnitTTS is the Tabletop Simulator figure of a unit (design 4.6).
type UnitTTS struct {
	Model          string   `json:"model,omitempty"`
	Foot           string   `json:"foot,omitempty"`
	States         *bool    `json:"states,omitempty"`
	Options        Free     `json:"options,omitempty"`
	DefaultOptions []string `json:"defaultOptions,omitempty"`
	Alternatives   Free     `json:"alternatives,omitempty"`
	Marker         string   `json:"marker,omitempty"`
}

// GearTTS is the Tabletop Simulator figure of an item (design 4.6).
type GearTTS struct {
	Name  string `json:"name,omitempty"`
	Model string `json:"model,omitempty"`
}

// Gear is an item (design 4.3): wargear, equipment, creature or wanderer; a company-local one is unique wargear.
type Gear struct {
	Name                  string       `json:"name" required:"true" min:"1"`
	ID                    string       `json:"id,omitempty"`
	Aka                   []string     `json:"aka,omitempty"`
	Kind                  string       `json:"kind,omitempty" enum:"wargear|equipment|creature|wanderer"`
	Type                  string       `json:"type,omitempty"`
	Classes               []string     `json:"classes,omitempty" enum:"melee|spear|two_handed|mount|bow|armour|throwing|shield|instrument|consumable|lance|banner|missile|crossbow|other" noun:"gear class"`
	Extends               string       `json:"extends,omitempty"`
	Overrides             bool         `json:"overrides,omitempty"`
	Influence             int          `json:"influence,omitempty" min:"0" max:"100"`
	Rating                Rating       `json:"rating,omitempty"`
	Gold                  *Rating      `json:"gold,omitempty"` // default: the rating
	Size                  string       `json:"size,omitempty" enum:"small|large"`
	Definition            string       `json:"definition,omitempty"`
	Grants                []string     `json:"grants,omitempty"`
	GrantsForChosenWeapon *ChosenGrant `json:"grantsForChosenWeapon,omitempty"`
	GrantsSlots           *Slots       `json:"grantsSlots,omitempty"`
	StatChanges           []StatChange `json:"statChanges,omitempty"`
	Abilities             *Abilities   `json:"abilities,omitempty"`
	Ranged                bool         `json:"ranged,omitempty"`
	PricedAs              string       `json:"pricedAs,omitempty"`
	Base                  string       `json:"base,omitempty"`
	RequiresMount         bool         `json:"requiresMount,omitempty"`
	Modifiers             []string     `json:"modifiers,omitempty"`
	Aura                  *Aura        `json:"aura,omitempty"`
	TTS                   *GearTTS     `json:"tts,omitempty"`
	Who                   *Who         `json:"who,omitempty"`
	Limit                 int          `json:"limit,omitempty" min:"0" max:"99"`
	Profile               Free         `json:"profile,omitempty"` // creatures and wanderers
	Note                  string       `json:"note,omitempty"`
}

// GearClasses is the closed vocabulary of item classes (design 12.4).
const GearClasses = "melee|spear|two_handed|mount|bow|armour|throwing|shield|instrument|consumable|lance|banner|missile|crossbow|other"

// ChosenGrant is the rule an item grants for the weapon its bearer chooses (Envenom Weapon).
type ChosenGrant struct {
	Rule          string   `json:"rule" required:"true"`
	WeaponClasses []string `json:"weaponClasses" required:"true" enum:"melee|spear|two_handed|mount|bow|armour|throwing|shield|instrument|consumable|lance|banner|missile|crossbow|other" noun:"gear class"`
}

// Slots is a pair of equipment slot counts (design 12.2 B6, B7).
type Slots struct {
	Small int `json:"small,omitempty" min:"0" max:"10"`
	Large int `json:"large,omitempty" min:"0" max:"10"`
}

// StatChange is a characteristic changed by an item or an injury (design 4.3, 12.2 D3).
type StatChange struct {
	Stat   string `json:"stat" required:"true" enum:"movement|fight|shoot|strength|defence|attack|wounds|courage|intelligence|might|will|fate" noun:"characteristic"`
	Amount int    `json:"amount" required:"true" min:"-20" max:"20"`
}

// Abilities are the two item abilities the engine reads (design 4.3).
type Abilities struct {
	DefenceBonus int `json:"defenceBonus,omitempty" min:"-10" max:"10"`
	AttacksBonus int `json:"attacksBonus,omitempty" min:"-10" max:"10"`
}

// Who is the one shape of access restriction of an item (design 4.3).
type Who struct {
	Hero      *bool    `json:"hero,omitempty"`
	Race      []string `json:"race,omitempty"`
	Units     []string `json:"units,omitempty"`
	UnitTypes []string `json:"unitTypes,omitempty"`
	Alignment string   `json:"alignment,omitempty" enum:"good|evil"`
	Text      string   `json:"text,omitempty"`
}

// Limit is a company-wide limit on a class of items or a keyword (design 12.2 B8, B9).
type Limit struct {
	ID                string   `json:"id" required:"true" min:"1"`
	Type              string   `json:"type,omitempty" enum:"bow|throwingWeapon|keyword|class"`
	Value             string   `json:"value,omitempty"`
	Property          string   `json:"property,omitempty"`
	MaximumPercentage float64  `json:"maximumPercentage" required:"true" min:"0.01" max:"100"`
	RoundUp           bool     `json:"roundUp" required:"true"`
	Classes           []string `json:"classes,omitempty" enum:"melee|spear|two_handed|mount|bow|armour|throwing|shield|instrument|consumable|lance|banner|missile|crossbow|other" noun:"gear class"`
	Label             string   `json:"label,omitempty"`
}

// Followers holds the rules of creatures or wanderers (design 12.2 B10).
type Followers struct {
	Rules FollowerRules `json:"rules" required:"true"`
}

// FollowerRules are the counts of creatures (per hero) and wanderers (at once).
type FollowerRules struct {
	MaxPerHero int `json:"maxPerHero,omitempty" min:"0" max:"99"`
	MaxAtOnce  int `json:"maxAtOnce,omitempty" min:"0" max:"99"`
}

// Rule is a special rule or keyword (design 4.5).
type Rule struct {
	Name       string     `json:"name" required:"true" min:"1"`
	ID         string     `json:"id,omitempty"`
	Aka        []string   `json:"aka,omitempty"`
	Definition string     `json:"definition,omitempty"`
	Source     string     `json:"source,omitempty"`
	Tag        string     `json:"tag,omitempty" enum:"Active|Passive"`
	Minor      bool       `json:"minor,omitempty"`
	Overrides  bool       `json:"overrides,omitempty"`
	Parameter  *Parameter `json:"parameter,omitempty"`
	Target     *Target    `json:"target,omitempty"`
	Aura       *Aura      `json:"aura,omitempty"`
	Note       string     `json:"note,omitempty"`
}

// Parameter says a rule takes a bracketed argument (design 4.5).
type Parameter struct {
	Kind  string `json:"kind" required:"true" enum:"keyword|weapon|rollTarget|number|companyHero"`
	Picks string `json:"picks,omitempty"`
}

// Target says who a rule works against or for (design 4.5).
type Target struct {
	AppliesTo      string   `json:"appliesTo,omitempty" enum:"enemy-keyword|enemy-any|friendly-keyword|own"` // default own
	Keywords       []string `json:"keywords,omitempty"`
	Match          string   `json:"match,omitempty" enum:"any|all"` // default any
	BearerKeywords []string `json:"bearerKeywords,omitempty"`
	Text           string   `json:"text,omitempty"`
}

// Aura is a ring of influence around the bearer, on a rule or an item (design 4.5).
type Aura struct {
	Radius   int      `json:"radius" required:"true" min:"1" max:"48"`
	Kind     string   `json:"kind" required:"true" enum:"banner|command|support|enemy|utility"`
	Affects  string   `json:"affects" required:"true" enum:"friendly|enemy|any|self"`
	When     string   `json:"when" required:"true" enum:"always|declared|conditional|target|cast"`
	LOS      bool     `json:"los"`
	Keywords []string `json:"keywords,omitempty"`
	Tip      string   `json:"tip,omitempty"`
	Concept  string   `json:"concept,omitempty"`
}

// Auras holds the aura vocabulary tables (design 4.5): kinds and groups, once.
type Auras struct {
	Kinds  []AuraKind  `json:"kinds,omitempty"`
	Groups []AuraGroup `json:"groups,omitempty"`
}

// AuraKind is a kind of aura with its colour and label.
type AuraKind struct {
	ID      string `json:"id" required:"true"`
	Label   string `json:"label,omitempty"`
	Colour  string `json:"colour,omitempty"`
	RGB     string `json:"rgb,omitempty"`
	Text    string `json:"text,omitempty"`
	Default bool   `json:"default,omitempty"`
}

// AuraGroup is a group of auras by when they apply.
type AuraGroup struct {
	ID      string `json:"id" required:"true"`
	Label   string `json:"label,omitempty"`
	Text    string `json:"text,omitempty"`
	Default bool   `json:"default,omitempty"`
}

// Magic holds what Battle Companies says about magical powers (design 5, "magicalPowers").
type Magic struct {
	Powers      map[string]Power `json:"powers,omitempty"`
	CastingRule string           `json:"castingRule,omitempty"`
}

// Power is the range and casting value Battle Companies gives a vanilla power.
type Power struct {
	Range   string `json:"range,omitempty"`
	Casting string `json:"casting,omitempty"`
}

// Path is a heroic path (design 4.4).
type Path struct {
	Name         string    `json:"name" required:"true" min:"1"`
	ID           string    `json:"id,omitempty"`
	Aka          []string  `json:"aka,omitempty"`
	Kind         string    `json:"kind,omitempty" enum:"heroic|custom|endOfPath"` // default heroic
	Extends      string    `json:"extends,omitempty"`
	Dice         DiceSpec  `json:"dice,omitempty"` // default campaign.progression.hero.roll
	Description  string    `json:"description,omitempty"`
	Maximums     Free      `json:"maximums,omitempty"`
	Mastery      string    `json:"mastery,omitempty"`
	HeroicAction *string   `json:"heroicAction,omitempty"`
	Signature    []string  `json:"signature,omitempty"`
	Spells       []string  `json:"spells,omitempty"`
	Rows         []PathRow `json:"rows,omitempty"`
	Limit        int       `json:"limit,omitempty" min:"0" max:"99"`
	Note         string    `json:"note,omitempty"`
}

// PathRow is the result of one roll on a path (design 4.4); Text is derived when absent.
type PathRow struct {
	Roll    Roll         `json:"roll" required:"true"`
	Options []PathOption `json:"options" required:"true"`
	Text    string       `json:"text,omitempty"`
}

// PathOption is a closed alternative: exactly one of Stat, Rule, MinorRule, HeroicAction, Power, CastingValue or OneOf is set.
type PathOption struct {
	Stat         string       `json:"stat,omitempty" enum:"movement|fight|shoot|strength|defence|attack|wounds|courage|intelligence|might|will|fate" noun:"characteristic"`
	Amount       int          `json:"amount,omitempty" min:"1" max:"20"`
	Rule         *RuleRef     `json:"rule,omitempty"`
	MinorRule    *RuleRef     `json:"minorRule,omitempty"`
	HeroicAction string       `json:"heroicAction,omitempty"`
	Power        string       `json:"power,omitempty"`
	CastingValue int          `json:"castingValue,omitempty" min:"1" max:"10"`
	MaxTotal     int          `json:"maxTotal,omitempty" min:"1" max:"99"`
	OneOf        []PathOption `json:"oneOf,omitempty"`
}

// Scenario is a scenario (design 4.7).
type Scenario struct {
	Name              string    `json:"name" required:"true" min:"1"`
	ID                string    `json:"id,omitempty"`
	Aka               []string  `json:"aka,omitempty"`
	Side              string    `json:"side,omitempty" enum:"both|attacker|defender"` // default both
	Description       string    `json:"description,omitempty"`
	Outline           string    `json:"outline,omitempty"`   // the short lead-in some scenarios print before the description
	UsesRules         []string  `json:"usesRules,omitempty"` // rules applied without text of their own (Maelstrom of Battle): shown by name
	Armies            string    `json:"armies,omitempty"`
	Layout            string    `json:"layout,omitempty"`
	StartingPositions string    `json:"startingPositions,omitempty"`
	Objectives        string    `json:"objectives,omitempty"`
	SpecialRules      Free      `json:"specialRules,omitempty"`
	Roles             Free      `json:"roles,omitempty"`
	Rewards           Free      `json:"rewards,omitempty"`
	Scoring           []Scoring `json:"scoring,omitempty"`
}

// Scoring is one scoring objective of a scenario (design 4.7, 6).
type Scoring struct {
	Key         string          `json:"key" required:"true" min:"1"`
	Side        string          `json:"side,omitempty" enum:"both|attacker|defender"`
	Secret      bool            `json:"secret,omitempty"`
	Kind        string          `json:"kind,omitempty"`
	Item        string          `json:"item,omitempty"`
	Count       int             `json:"count,omitempty" min:"0" max:"99"`
	Instruction string          `json:"instruction,omitempty"`
	Nomination  *Nomination     `json:"nomination,omitempty"`
	Options     []ScoringOption `json:"options,omitempty"`
}

// Nomination is what a player notes down at the start for a secret objective (design 4.7): From is where the nominee comes
// from, AllowLeader limits whether the Leader may be picked (onlyHero: only when the company has a single Hero), Where
// places a terrain piece.
type Nomination struct {
	From        string `json:"from" required:"true" enum:"ownHeroes|enemyHeroes|terrain"`
	AllowLeader string `json:"allowLeader,omitempty" enum:"onlyHero"`
	Where       string `json:"where,omitempty" enum:"opponentHalf"`
}

// ScoringOption is a tier of a scoring objective: its sentence and the Victory Points.
type ScoringOption struct {
	Text   string `json:"text" required:"true"`
	Points int    `json:"points" required:"true" min:"0" max:"99"`
}

// Vanilla is what Battle Companies says about vanilla things (design 4.9).
type Vanilla struct {
	Pins    []Pin                  `json:"pins,omitempty"`
	Units   map[string]VanillaFill `json:"units,omitempty"`
	Mounts  map[string]VanillaFill `json:"mounts,omitempty"`
	Options *VanillaOptions        `json:"options,omitempty"`
}

// Pin records which occurrence of a duplicated vanilla name is meant (first wins otherwise).
type Pin struct {
	Kind       string `json:"kind" required:"true" enum:"hero|warrior|unit|item|rule|mount"`
	Name       string `json:"name" required:"true"`
	Occurrence int    `json:"occurrence" required:"true" min:"1" max:"20"`
}

// VanillaFill supplies fields a vanilla unit or mount lacks; it applies only where the vanilla field is empty.
type VanillaFill struct {
	Race     []string `json:"race,omitempty"`
	Faction  []string `json:"faction,omitempty"`
	UnitType []string `json:"unitType,omitempty"`
	BaseSize []string `json:"baseSize,omitempty"`
}

// VanillaOptions are the rules about vanilla options (design 4.9, 5).
type VanillaOptions struct {
	NotSold    []string          `json:"notSold,omitempty"`
	Exceptions []OptionException `json:"exceptions,omitempty"`
}

// OptionException is an option the notSold words would hide, or read as another item.
type OptionException struct {
	Unit    string `json:"unit" required:"true"`
	Option  string `json:"option" required:"true"`
	ReadAs  string `json:"readAs,omitempty"`
	OwnItem string `json:"ownItem,omitempty"`
	Granted bool   `json:"granted,omitempty"`
}
