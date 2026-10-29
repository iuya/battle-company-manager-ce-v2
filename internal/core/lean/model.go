// Package lean is the typed model, strict decoder, name resolution and validation of the lean Battle Companies format
// (schemaVersion 2, docs/definitions.md). It is pure: it imports only the standard library, never package core, so
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

// Free is a value the format does not type yet (its shape is whatever its reader needs).
type Free = any

// Document is the whole Battle Companies file: the header and the sections.
type Document struct {
	SchemaVersion int      `json:"schemaVersion" required:"true" min:"1" max:"1000"` // separate from Version: 2 for this format
	Name          string   `json:"name" required:"true" min:"1"`
	Version       Scalar   `json:"version" required:"true"` // the rulebook version, "1.1"
	Credit        Free     `json:"credit,omitempty"`
	Extends       string   `json:"extends,omitempty"` // the vanilla file it builds on (header marker)
	Campaign      Campaign `json:"campaign" required:"true"`

	Rules     []Rule         `json:"rules,omitempty"`
	Magic     *Magic         `json:"magic,omitempty"`
	Gear      []Gear         `json:"gear,omitempty"`
	Paths     []Path         `json:"paths,omitempty"`
	Scenarios []Scenario     `json:"scenarios,omitempty"`
	Companies []Company      `json:"companies,omitempty"`
	Units     []Unit         `json:"units,omitempty"` // shared units
	Limits    []Limit        `json:"limits,omitempty"`
	ArmourUp  *ArmourUpgrade `json:"armourUpgrade,omitempty"`
	Creatures *Followers     `json:"creatures,omitempty"`
	Wanderers *Followers     `json:"wanderers,omitempty"`
	Vanilla   *Vanilla       `json:"vanilla,omitempty"`
	Auras     *Auras         `json:"auras,omitempty"`
}

// Characteristics is the closed vocabulary of stats.
const Characteristics = "movement|fight|shoot|strength|defence|attack|wounds|courage|intelligence|might|will|fate"

// Company is a Battle Company. The 25 old effects[] kinds are typed fields.
type Company struct {
	Name           string         `json:"name" required:"true" min:"1"`
	ID             string         `json:"id,omitempty"`
	Aka            []string       `json:"aka,omitempty"`
	Alignment      string         `json:"alignment" required:"true" enum:"good|evil"`
	Description    string         `json:"description,omitempty"`
	Epigraph       *Epigraph      `json:"epigraph,omitempty"`
	Faction        string         `json:"faction,omitempty"` // default: the name
	Label          string         `json:"label,omitempty"`   // how the pages spell the name when the key is plainer (Lothlórien for Lothlorien); the same name once accents are folded
	Oath           *Oath          `json:"oath,omitempty"`
	Extends        string         `json:"extends,omitempty"`
	MaxModels      int            `json:"maxModels,omitempty" min:"1" max:"50"` // default campaign.company.maxModels; Elite 12
	BreakPoint     int            `json:"breakPoint,omitempty" min:"1" max:"100"`
	Start          *Start         `json:"start,omitempty"`
	Reinforcements *Chart         `json:"reinforcements,omitempty"`
	Special        *SpecialChart  `json:"special,omitempty"`
	Advancements   []Advancement  `json:"advancements,omitempty"`
	HeroUpgrades   []HeroUpgrade  `json:"heroUpgrades,omitempty"`
	Units          []Unit         `json:"units,omitempty"` // company-local units
	Rules          []Rule         `json:"rules,omitempty"` // company-local rule definitions, hoisted
	Gear           []Gear         `json:"gear,omitempty"`  // unique wargear
	Promotions     []Promotion    `json:"promotions,omitempty"`
	Heroes         *Heroes        `json:"heroes,omitempty"`
	Recruit        *Recruit       `json:"recruit,omitempty"`
	Limits         *CompanyLimit  `json:"limits,omitempty"`
	OwnPath        *OwnPath       `json:"ownPath,omitempty"`
	Substitutions  []Substitution `json:"substitutions,omitempty"` // ruleSubstitution
	Exchanges      []Exchange     `json:"exchanges,omitempty"`     // ruleExchange
	Granted        []Granted      `json:"granted,omitempty"`       // grantedRule
	ElvenWeaponry  *Who           `json:"elvenWeaponry,omitempty"` // the models that may also buy and wear Elven-made weaponry (an empty object: every model); none: only Elves
	Notes          []CompanyNote  `json:"notes,omitempty"`
	SpecialRules   []string       `json:"specialRules,omitempty"` // only the sentences with no typed form: a typed object keeps its own text
}

// Epigraph is a quote with its comment.
type Epigraph struct {
	Quote string `json:"quote"`
	Text  string `json:"text,omitempty"`
}

// Oath is the vow and text of a company; {name} is replaced by the company's name.
type Oath struct {
	Vow  string `json:"vow,omitempty"`
	Text string `json:"text"`
}

// Start is the starting force: gold and the units.
type Start struct {
	Gold  int         `json:"gold" min:"0" max:"1000"`
	Units []StartUnit `json:"units" required:"true"`
}

// StartUnit is one line of the starting force. Unit is the name the company calls it (an own unit that selects an option or adds a
// rule, or the vanilla name); With is the wargear it starts with.
type StartUnit struct {
	Unit  string   `json:"unit" required:"true" min:"1"`
	Count int      `json:"count,omitempty" min:"1" max:"50"` // default 1
	With  []string `json:"with,omitempty"`
	Role  string   `json:"role,omitempty" enum:"leader|sergeant|heroInTheMaking"` // the place the company suggests for it; none for a plain warrior
}

// Chart is the reinforcement chart: the influence cost and the rows by roll.
type Chart struct {
	Influence int        `json:"influence" min:"0" max:"100"`
	Rows      []ChartRow `json:"rows" required:"true"`
}

// SpecialChart is the special units chart and the units a company may buy directly (Buy).
type SpecialChart struct {
	Rows []ChartRow `json:"rows" required:"true"`
	Buy  []BuyUnit  `json:"buy,omitempty"` // specialUnit: a unit bought directly for its own influence cost, outside the chart
}

// BuyUnit is a unit a company can recruit directly for its own influence cost (Moria's Cave Troll costs 15). Unit is the name the
// company calls it (the Goblin Hulk is an own unit built on the Gundabad Ogre).
type BuyUnit struct {
	Unit      string `json:"unit" required:"true" min:"1"`
	Influence int    `json:"influence" required:"true" min:"0" max:"100"`
	Rare      int    `json:"rare,omitempty" min:"0" max:"50"`
	Pick      bool   `json:"pick,omitempty"` // the player picks the unit's options
}

// ChartRow is one row of a reinforcement or special chart.
//
// A row is one of: a unit, nothing, special (the roll goes on to the special chart), choose (the player takes any result of the
// chart; the text says which) or options (the player picks one of several units).
type ChartRow struct {
	Roll    Roll          `json:"roll" required:"true"`
	Unit    string        `json:"unit,omitempty"`
	Nothing bool          `json:"nothing,omitempty"`
	Blocks  bool          `json:"blocks,omitempty"` // a nothing row that blocks further rolls
	Special bool          `json:"special,omitempty"`
	Choose  string        `json:"choose,omitempty"`                 // "Choice of above": the words of a choice-of-results row (display text)
	Scope   string        `json:"scope,omitempty" enum:"above|any"` // a choose row: the player takes a result of the rows above the roll, or of any row of both charts
	Picks   int           `json:"picks,omitempty" min:"1" max:"10"` // a choose row: how many results the player takes (default 1)
	Options []ChartChoice `json:"options,omitempty"`                // the units the player may pick one of
	Text    string        `json:"text,omitempty"`                   // what an options row says
	Pick    bool          `json:"pick,omitempty"`                   // the player picks the unit's options ("choice of options")
	Count   int           `json:"count,omitempty" min:"1" max:"50"`
	Rare    int           `json:"rare,omitempty" min:"0" max:"50"`
	With    []string      `json:"with,omitempty"`
}

// ChartChoice is one of the units an options row lets the player pick, with the number of it the company may have.
type ChartChoice struct {
	Unit string `json:"unit" required:"true" min:"1"`
	Rare int    `json:"rare,omitempty" min:"0" max:"50"`
}

// Advancement says which units a unit may be promoted to. With is the wargear the unit must carry to be promoted.
type Advancement struct {
	From string      `json:"from" required:"true"`
	With []string    `json:"with,omitempty"`
	To   []AdvanceTo `json:"to" required:"true"`
}

// AdvanceTo is a unit a promotion leads to, with the wargear it gets: a name, or {unit, with}.
type AdvanceTo struct {
	Unit string   `json:"unit"`
	With []string `json:"with,omitempty"`
}

// HeroUpgrade names a rule a hero may take; an inline Definition defines it.
type HeroUpgrade struct {
	Name       string `json:"name" required:"true" min:"1"`
	Flavour    string `json:"flavour,omitempty"`
	Definition string `json:"definition,omitempty"` // inline rule definition, hoisted into the rule index
	Source     string `json:"source,omitempty"`
	Who        *Who   `json:"who,omitempty"`  // the models that can take it (Fiefdoms: keywords Dol Amroth); none: any
	Note       string `json:"note,omitempty"` // why the restriction is read so (a review note)
}

// Promotion is a hero promotion of a unit.
type Promotion struct {
	From       string `json:"from" required:"true"`
	On         string `json:"on" required:"true" enum:"noChange|promoted|heroInTheMaking"`
	Becomes    string `json:"becomes" required:"true"`
	StatGain   bool   `json:"statGain,omitempty"`
	PathChoice string `json:"pathChoice,omitempty"`
	Forgo      bool   `json:"forgo,omitempty"` // the player may keep the unit as it is
	Text       string `json:"text,omitempty"`
}

// Heroes restricts who may be a starting hero.
type Heroes struct {
	Who           *Who      `json:"who,omitempty"`           // startingHeroRestriction: the starting units that may be made heroes (units: only these; except: all but these)
	NoStartGain   []string  `json:"noStartGain,omitempty"`   // starting units that keep their own Might, Will and Fate when made heroes
	RecruitAsHero *HeroGain `json:"recruitAsHero,omitempty"` // every recruit joins as a Hero in the Making with these gains
}

// Recruit holds the recruitment exceptions of a company.
type Recruit struct {
	Fallback []Fallback `json:"fallback,omitempty"` // recruitFallback
	Replace  []Replace  `json:"replace,omitempty"`  // rangerReplacement and reinforcementReplacement
	Balance  []Balance  `json:"balance,omitempty"`  // raceBalance
}

// Fallback lets a recruit that would break a limit take another result (Vault Warden Team: over the model maximum, any result of both
// charts). Charts name "reinforcements" and "specialChart".
type Fallback struct {
	Unit       string   `json:"unit" required:"true" min:"1"`
	When       string   `json:"when" required:"true" enum:"wouldExceedMaxModels|memberMissing"`
	Charts     []string `json:"charts,omitempty" enum:"reinforcements|specialChart"`
	Choose     string   `json:"choose,omitempty"`
	Cost       int      `json:"cost,omitempty" min:"0" max:"100"`
	SwapRollOn string   `json:"swapRollOn,omitempty" enum:"reinforcements|specialChart"`
	AnyRoll    bool     `json:"anyRoll,omitempty"`
}

// Replace is a recruit that may be swapped: the Shire's Ranger of the North once the company's one is slain (Unit, Max, Roles, When), or
// a unit offered at no cost instead of the chart's on some rolls of a chart (Chart, Roll, Alternative; Fornost: a Hobbit Archer on 2 to 6).
type Replace struct {
	Unit         string   `json:"unit,omitempty"`
	Max          int      `json:"max,omitempty" min:"1" max:"50"`
	Roles        []string `json:"roles,omitempty" enum:"leader|sergeant"`
	When         string   `json:"when,omitempty"`
	Chart        string   `json:"chart,omitempty" enum:"reinforcements|specialChart"`
	Roll         *Roll    `json:"roll,omitempty"`
	Alternative  string   `json:"alternative,omitempty"`
	PlayerChoice bool     `json:"playerChoice,omitempty"`
	Cost         int      `json:"cost,omitempty" min:"0" max:"100"`
}

// Balance says a keyword may not outnumber another (Defenders of the North: Dwarves not more than Dale models).
type Balance struct {
	Keyword     string `json:"keyword" required:"true" min:"1"`
	NotMoreThan string `json:"notMoreThan" required:"true" min:"1"`
	OnExceed    string `json:"onExceed,omitempty"`
}

// CompanyLimit overrides the limits of one company.
type CompanyLimit struct {
	Exempt   []Exempt           `json:"exempt,omitempty"`   // models or items that do not count towards limits
	Override map[string]float64 `json:"override,omitempty"` // a limit's own share for this company, by limit id (Osgiliath: bow 50)
	Company  []KeywordLimit     `json:"company,omitempty"`  // no more than a share of the company may have a keyword (Helm's Deep: Elf 33)
}

// Exempt says a unit or an item does not count towards some limits (Riders of Rohan: bow and cavalry).
type Exempt struct {
	Unit   string   `json:"unit,omitempty"`
	Item   string   `json:"item,omitempty"`
	Limits []string `json:"limits" required:"true"`
	Text   string   `json:"text,omitempty"`
}

// KeywordLimit caps the share of a company that may have a keyword.
type KeywordLimit struct {
	Keyword string  `json:"keyword" required:"true" min:"1"`
	Percent float64 `json:"percent" required:"true" min:"0" max:"100"`
	Text    string  `json:"text,omitempty"`
}

// Substitution says models of the company treat some model as another for a rule (Osgiliath: the Leader is Faramir for Loyal to the
// Captain).
type Substitution struct {
	AppliesTo string    `json:"appliesTo" required:"true" min:"1"`
	Target    SubTarget `json:"target" required:"true"`
	TreatedAs string    `json:"treatedAs" required:"true" min:"1"`
	Rule      string    `json:"rule,omitempty"`
	Text      string    `json:"text" required:"true" min:"1"`
}

// SubTarget is the model that is treated as another: a role of the company, or any hero with a rule.
type SubTarget struct {
	Role     string `json:"role,omitempty" enum:"leader|sergeant"`
	Hero     bool   `json:"hero,omitempty"`
	WithRule string `json:"withRule,omitempty"`
}

// Exchange is a rule models of the company swap for another (Sharkey's Rogues: Hatred (Rohan) for Hatred (Hobbit)).
type Exchange struct {
	Remove string `json:"remove" required:"true" min:"1"`
	Add    string `json:"add" required:"true" min:"1"`
	Text   string `json:"text,omitempty"`
}

// Granted is a special rule the company gives some of its models for free.
type Granted struct {
	Rule string `json:"rule" required:"true" min:"1"`
	Who  *Who   `json:"who,omitempty"` // the models that get it for free; none: every model of the company
	Cost int    `json:"cost,omitempty" min:"0" max:"100"`
	Text string `json:"text,omitempty"`
}

// CompanyNote is a note of a company page, with the section of the page it belongs to.
type CompanyNote struct {
	Section string   `json:"section" required:"true" enum:"startingCompany|reinforcements|specialChart|advancements|eliteCompany"`
	Text    string   `json:"text" required:"true" min:"1"`
	About   []string `json:"about,omitempty"`
}

// OwnPath lists the units On Their Own Path.
type OwnPath struct {
	Units []string `json:"units" required:"true"`
	Path  string   `json:"path,omitempty"` // default "On Their Own Path"
}

// Unit is a company-local or shared unit: a base plus deltas, a brand-new unit, or an annotation of a lower-layer unit.
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
	Composite   *Composite `json:"composite,omitempty"`               // one model on the table that counts as several (Warg Marauder)
	Includes    []Included `json:"includes,omitempty"`                // the units a unit is made of, with their points (Vault Warden Team)
	Models      int        `json:"models,omitempty" min:"1" max:"50"` // how many models a recruit counts as against the maximum
	OwnPath     *OwnPath   `json:"ownPath,omitempty"`
	TTS         *UnitTTS   `json:"tts,omitempty"`
	Overrides   bool       `json:"overrides,omitempty"`
	Profile     Free       `json:"profile,omitempty"` // a brand-new unit (no extends): full definition
	Options     Free       `json:"options,omitempty"`
	Points      int        `json:"points,omitempty" min:"0" max:"10000"`
	Definition  Free       `json:"definition,omitempty"`
}

// Composite is one model on the table that counts as several for the company (the Warg Marauder: three riders on a Warg).
type Composite struct {
	Parts           []CompositePart `json:"parts" required:"true"`
	Counts          *CompositeCount `json:"counts,omitempty"`
	OnePhysicalCard bool            `json:"onePhysicalCard,omitempty"`
	Casualty        *Casualty       `json:"casualty,omitempty"`
	Rules           []string        `json:"rules,omitempty"` // the special rules that explain how it plays
}

// CompositePart is a part of a composite model: the riders and the mount.
type CompositePart struct {
	Role  string   `json:"role" required:"true" min:"1"`
	Unit  string   `json:"unit,omitempty"`
	Mount string   `json:"mount,omitempty"`
	Count int      `json:"count,omitempty" min:"1" max:"50"` // default 1
	With  []string `json:"with,omitempty"`
}

// CompositeCount is what a composite model counts as: roster slots (against the company's maximum and its roster) and the limits.
type CompositeCount struct {
	Slots        int `json:"slots,omitempty" min:"1" max:"50"`
	MaxModels    int `json:"maxModels,omitempty" min:"0" max:"50"`
	BowLimit     int `json:"bowLimit,omitempty" min:"0" max:"50"`
	CavalryLimit int `json:"cavalryLimit,omitempty" min:"0" max:"50"`
}

// Casualty says when a composite model is injured and the injury table it rolls on.
type Casualty struct {
	InjuredWhen string `json:"injuredWhen,omitempty"`
	Table       string `json:"table,omitempty"`
	WholeModel  bool   `json:"wholeModel,omitempty"`
}

// Included is a unit a unit is made of, with the points it is worth.
type Included struct {
	Name   string `json:"name" required:"true" min:"1"`
	Points int    `json:"points" required:"true" min:"0" max:"10000"`
}

// TypeDelta adds and removes unit types.
type TypeDelta struct {
	Add    []string `json:"add,omitempty"`
	Remove []string `json:"remove,omitempty"`
}

// UnitTTS is the Tabletop Simulator figure of a unit. A vanilla unit BC cannot edit gets it through an annotation (an
// entry of units[] with that name and only tts).
type UnitTTS struct {
	Model          string            `json:"model,omitempty"`          // the model the mod knows the unit by, when it differs from the unit's name
	Foot           string            `json:"foot,omitempty"`           // a cavalry unit: the model of the rider on foot (absent: the mod has none)
	States         *bool             `json:"states,omitempty"`         // a cavalry unit: every model of it in the mod carries the state that dismounts the figure
	Options        map[string]string `json:"options,omitempty"`        // option spellings that differ between the vanilla file and the mod: vanilla spelling -> the mod's
	DefaultOptions []string          `json:"defaultOptions,omitempty"` // the options the unit is requested with when it carries none (the bag has no bare figure)
	Alternatives   []string          `json:"alternatives,omitempty"`   // other models the mod has for the unit
	Marker         string            `json:"marker,omitempty"`         // an alias marker option (Lorien Guard) left out of the key when it is the unit's only option
}

// GearTTS is the Tabletop Simulator figure of an item: a wanderer or creature the mod has no 1:1 figure of, or a mount that
// fights (vanilla Attacks above 0). Model is the figure to spawn ("Name" or "Name: options"); the mod's attacks of a mount are the vanilla ones.
type GearTTS struct {
	Name         string   `json:"name,omitempty"` // the name the mod knows the item by, when it differs
	Model        string   `json:"model,omitempty"`
	Alternatives []string `json:"alternatives,omitempty"` // other models the mod has that stand in as well
}

// Gear is an item: wargear, equipment, creature or wanderer; a company-local one is unique wargear.
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
	Ranged                *Ranged      `json:"ranged,omitempty"`   // the missile profile of a bow, crossbow, throwing weapon or missile: 24" and Strength 2
	PricedAs              string       `json:"pricedAs,omitempty"` // sold at the price of this item (Elven-made weaponry: its generic equivalent)
	Base                  string       `json:"base,omitempty"`     // the generic item this one is a version of (Elven spear: Spear)
	RequiresMount         bool         `json:"requiresMount,omitempty"`
	Modifiers             []string     `json:"modifiers,omitempty"`
	Related               []string     `json:"related,omitempty"`         // related but distinct items (Elf Bow and Longbow share a profile): never the same item
	Mount                 string       `json:"mount,omitempty"`           // a mount: the vanilla entry that has its stat block, when it is not the item's own name (Goat: War goat)
	Unsold                bool         `json:"unsold,omitempty"`          // defined for its classes and text, but no armoury sells it (profile wargear, natural weapons)
	AftermathEffect       string       `json:"aftermathEffect,omitempty"` // a consumable of the aftermath: the key of the campaign.aftermathEffects entry it activates
	Auras                 []Aura       `json:"auras,omitempty"`           // the rings the item draws (War Horn has two)
	TTS                   *GearTTS     `json:"tts,omitempty"`
	Who                   *Who         `json:"who,omitempty"`
	Limit                 int          `json:"limit,omitempty" min:"0" max:"99"`
	Profile               Free         `json:"profile,omitempty"` // creatures and wanderers
	Note                  string       `json:"note,omitempty"`
}

// GearClasses is the closed vocabulary of item classes.
const GearClasses = "melee|spear|two_handed|mount|bow|armour|throwing|shield|instrument|consumable|lance|banner|missile|crossbow|other"

// ChosenGrant is the rule an item grants for the weapon its bearer chooses (Envenom Weapon).
type ChosenGrant struct {
	Rule          string   `json:"rule" required:"true"`
	WeaponClasses []string `json:"weaponClasses" required:"true" enum:"melee|spear|two_handed|mount|bow|armour|throwing|shield|instrument|consumable|lance|banner|missile|crossbow|other" noun:"gear class"`
}

// Slots is a pair of equipment slot counts.
type Slots struct {
	Small int `json:"small,omitempty" min:"0" max:"10"`
	Large int `json:"large,omitempty" min:"0" max:"10"`
}

// StatChange is a characteristic changed by an item or an injury.
type StatChange struct {
	Stat      string `json:"stat" required:"true" enum:"movement|fight|shoot|strength|defence|attack|wounds|courage|intelligence|might|will|fate" noun:"characteristic"`
	Amount    int    `json:"amount" required:"true" min:"-20" max:"20"`
	Condition string `json:"condition,omitempty"` // set: the change holds only under this condition, so it is a note and never added to the profile
}

// Ranged is the missile profile of an item, in inches and Strength.
type Ranged struct {
	Range    int `json:"range" required:"true" min:"1" max:"99"`
	Strength int `json:"strength" required:"true" min:"0" max:"20"`
}

// Abilities are the two item abilities the engine reads; every other ability is item text only.
type Abilities struct {
	DefenceBonus *DefenceBonus `json:"defenceBonus,omitempty"`
	AttacksBonus *AttacksBonus `json:"attacksBonus,omitempty"`
}

// DefenceBonus is the Defence a shield adds and what cancels it: a bow, crossbow or two-handed weapon (Classes), a named item (Items),
// a hand-and-a-half weapon used two-handed (TwoHandedUse), unless a rule lifts the cancellation (Lifted). Conditional or Against
// make it a note on the item that the Defence never includes.
type DefenceBonus struct {
	Value              int           `json:"value" required:"true" min:"-10" max:"10"`
	Conditional        bool          `json:"conditional,omitempty"`
	Condition          string        `json:"condition,omitempty"`
	NotWhileEngaged    bool          `json:"notWhileEngaged,omitempty"`
	Against            string        `json:"against,omitempty"`
	CancelledByClasses []string      `json:"cancelledByClasses,omitempty" enum:"melee|spear|two_handed|mount|bow|armour|throwing|shield|instrument|consumable|lance|banner|missile|crossbow|other" noun:"gear class"`
	CancelledByItems   []string      `json:"cancelledByItems,omitempty"`
	TwoHandedUse       bool          `json:"cancelledByTwoHandedUse,omitempty"`
	LiftedByRules      []DefenceLift `json:"liftedByRules,omitempty"`
}

// DefenceLift is a rule that lifts a cancellation of the bonus (Expert Rider: the bow's, while mounted; Phalanx: the pike's).
type DefenceLift struct {
	Rule         string   `json:"rule" required:"true"`
	Cancels      []string `json:"cancels,omitempty"`
	WhileMounted bool     `json:"whileMounted,omitempty"`
}

// AttacksBonus is a bonus to Attacks that holds only in a game situation: item text, never added to the profile.
type AttacksBonus struct {
	Value     int    `json:"value" required:"true" min:"-10" max:"10"`
	Condition string `json:"condition,omitempty"`
}

// Limit is a company-wide limit on a class of items or a keyword.
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

// ArmourUpgrade is the Battle Companies rule "Upgrade Armour by one stage": Stages lists the names of each stage, lowest first; each
// upgrade costs Influence (or Gold, default the rating) and adds Rating.
type ArmourUpgrade struct {
	Stages    [][]string `json:"stages" required:"true"`
	Influence int        `json:"influence" required:"true" min:"0" max:"100"`
	Rating    Rating     `json:"rating" required:"true"`
	Gold      *Rating    `json:"gold,omitempty"`
}

// Followers holds the rules of creatures or wanderers.
type Followers struct {
	Rules FollowerRules `json:"rules" required:"true"`
}

// FollowerRules are the counts of creatures (per hero) and wanderers (at once).
type FollowerRules struct {
	MaxPerHero int      `json:"maxPerHero,omitempty" min:"0" max:"99"`
	MaxAtOnce  int      `json:"maxAtOnce,omitempty" min:"0" max:"99"`
	AttachTo   []string `json:"attachTo,omitempty" enum:"leader|sergeant"` // creatures: the roles that may have one
}

// Rule is a special rule or keyword.
type Rule struct {
	Name       string     `json:"name" required:"true" min:"1"`
	ID         string     `json:"id,omitempty"`
	Aka        []string   `json:"aka,omitempty"`
	Definition string     `json:"definition,omitempty"`
	Source     string     `json:"source,omitempty"`
	Label      string     `json:"label,omitempty"` // the name shown when it is not the key (Keen Sight (Falcon) is shown as Keen Sight)
	Tag        string     `json:"tag,omitempty" enum:"Active|Passive"`
	Minor      bool       `json:"minor,omitempty"`
	Overrides  bool       `json:"overrides,omitempty"`
	Parameter  *Parameter `json:"parameter,omitempty"`
	Target     *Target    `json:"target,omitempty"`
	BareTarget *Target    `json:"bareTarget,omitempty"` // the target of the rule written without its bracket (Terror: all enemies; Terror (X): the keyword)
	Auras      []Aura     `json:"auras,omitempty"`      // the rings the rule draws (Enchanting Song has two)
	Note       string     `json:"note,omitempty"`
}

// Parameter says a rule takes a bracketed argument.
type Parameter struct {
	Kind  string `json:"kind" required:"true" enum:"keyword|weapon|rollTarget|number|companyHero"`
	Picks string `json:"picks,omitempty"`
}

// Target says who a rule works against or for.
type Target struct {
	AppliesTo      string   `json:"appliesTo,omitempty" enum:"enemy-keyword|enemy-any|friendly-keyword|own"` // default own
	Keywords       []string `json:"keywords,omitempty"`
	Match          string   `json:"match,omitempty" enum:"any|all"` // default any
	BearerKeywords []string `json:"bearerKeywords,omitempty"`
	Text           string   `json:"text,omitempty"`
}

// Aura is a ring of influence around the bearer, on a rule, an item or a magical power. It sits on the thing that
// draws it; the vocabulary tables (kinds, groups) are the top-level Auras.
type Aura struct {
	Label       string   `json:"label,omitempty"`       // the name of this ring when it is not the owner's (Heroic March draws "At the Double")
	MergeLabels []string `json:"mergeLabels,omitempty"` // other labels shown as this ring (Stand Fast: With Me!, Loose)
	Radius      int      `json:"radius" required:"true" min:"1" max:"48"`
	Kind        string   `json:"kind" required:"true" enum:"banner|command|support|enemy|utility"`
	Affects     string   `json:"affects" required:"true" enum:"friendly|enemy|any|self"`
	When        string   `json:"when" required:"true" enum:"always|declared|conditional|target|cast"`
	Group       string   `json:"group,omitempty"` // the dialog switch (auras.groups id); default: when
	Items       []string `json:"items,omitempty"` // on an item: other items that draw the same ring (Company Horn draws the War Horn's)
	LOS         bool     `json:"los"`
	Keywords    []string `json:"keywords,omitempty"`
	Bearer      string   `json:"bearer,omitempty" enum:"Hero"`                       // only a Hero bearer gets the ring
	From        string   `json:"from,omitempty" enum:"rule|heroicAction|hero|power"` // on a rule: what draws it; default rule (hero: every Hero has it; power: Fury, the rule of a power with a choice)
	Replaces    []string `json:"replaces,omitempty"`                                 // labels of the rings this one supersedes (Voice of Command)
	Tip         string   `json:"tip,omitempty"`
	Concept     string   `json:"concept,omitempty"`
}

// Auras holds the aura vocabulary tables: kinds and groups, once.
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

// Magic holds what Battle Companies says about magical powers.
type Magic struct {
	Powers      map[string]Power `json:"powers,omitempty"`
	CastingRule string           `json:"castingRule,omitempty"`
	Floor       int              `json:"floor,omitempty" min:"1" max:"6"`       // a casting value is never improved below this (2 is 2+); default 1
	MaxPerPower int              `json:"maxPerPower,omitempty" min:"1" max:"6"` // improvements of one power per hero; default 1
	Listed      []string         `json:"listed,omitempty"`                      // vanilla powers Battle Companies lists though nobody learns them: the glossary shows them as its own
}

// Power is the range and casting value Battle Companies gives a vanilla power.
type Power struct {
	Range   string `json:"range,omitempty"`
	Casting string `json:"casting,omitempty"`
	Auras   []Aura `json:"auras,omitempty"` // the rings the power draws while it is in effect (Aura of Command)
}

// Path is a heroic path.
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
	Start        *HeroGain `json:"start,omitempty"` // Might, Will and Fate a unit starts on the path with, when it is not the usual heroStart (the homebrew path)
	Rule         string    `json:"rule,omitempty"`  // the rule of a path that is the end of the road (kind endOfPath): At Path's End
}

// PathRow is the result of one roll on a path; Text is derived when absent.
type PathRow struct {
	Roll    Roll         `json:"roll" required:"true"`
	Options []PathOption `json:"options" required:"true"`
	Text    string       `json:"text,omitempty"`
}

// PathOption is a closed alternative: exactly one of Stat, Rule, MinorRule, HeroicAction, Power, CastingValue, OneOf or one of the
// open choices AnyMinorRule, AnyHeroicAction and AnyPower is set. Max and RollAgain belong to a stat option, MaxTotal to a minor rule
// or a heroic action.
type PathOption struct {
	Stat            string       `json:"stat,omitempty" enum:"movement|fight|shoot|strength|defence|attack|wounds|courage|intelligence|might|will|fate" noun:"characteristic"`
	Amount          int          `json:"amount,omitempty" min:"1" max:"20"`
	Rule            *RuleRef     `json:"rule,omitempty"`
	MinorRule       *RuleRef     `json:"minorRule,omitempty"`
	HeroicAction    string       `json:"heroicAction,omitempty"`
	Power           string       `json:"power,omitempty"`
	CastingValue    int          `json:"castingValue,omitempty" min:"1" max:"10"`
	MaxTotal        int          `json:"maxTotal,omitempty" min:"1" max:"99"`
	Max             int          `json:"max,omitempty" min:"1" max:"10"` // a stat option: the highest the stat may reach through this result (Will 6 for the Tactician)
	RollAgain       bool         `json:"rollAgain,omitempty"`            // a stat option: the hero also takes its other result
	AnyMinorRule    bool         `json:"anyMinorRule,omitempty"`         // a minor special rule of the minor list, the hero's choice
	AnyHeroicAction bool         `json:"anyHeroicAction,omitempty"`      // a heroic action the hero does not have yet, its choice
	AnyPower        bool         `json:"anyPower,omitempty"`             // a magical power of the path's spells the hero does not know yet, its choice
	OneOf           []PathOption `json:"oneOf,omitempty"`
}

// Scenario is a scenario.
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

// Scoring is one scoring objective of a scenario.
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

// Nomination is what a player notes down at the start for a secret objective: From is where the nominee comes
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

// Vanilla is what Battle Companies says about vanilla things.
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
}

// VanillaOptions are the rules about vanilla options.
type VanillaOptions struct {
	NotSold    []string          `json:"notSold,omitempty"`
	Reason     string            `json:"reason,omitempty"` // why the words are not sold; the armoury shows it
	Exceptions []OptionException `json:"exceptions,omitempty"`
}

// OptionException is an option the notSold words would hide, or read as another item.
type OptionException struct {
	Unit    string `json:"unit" required:"true"`
	Option  string `json:"option" required:"true"`
	ReadAs  string `json:"readAs,omitempty"`  // sold as this armoury item (Hand-and-a-half hammer: Hand-and-a-half Weapon)
	OwnItem string `json:"ownItem,omitempty"` // an item of its own that no armoury sells (Troll Spear), choosable under its own name
	Granted bool   `json:"granted,omitempty"` // the model comes with it from the charts or the starting company and no armoury sells it
	Reason  string `json:"reason,omitempty"`  // why it is not sold (granted, notSold); the armoury shows it
	NotSold bool   `json:"notSold,omitempty"` // no armoury sells it though its name is none of the notSold words (the unique wargear of one company)
}
