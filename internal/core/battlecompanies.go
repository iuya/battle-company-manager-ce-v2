package core

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// BCBook gives access to the Battle Companies rules the company pages need: how many models a company may have, what
// recruiting costs and what it can give, and what the armoury sells. Like VanillaBook it exposes the part of the rules
// that the app reads.
type BCBook struct {
	shape         CompanyRules // campaign.company: officers, hero start, model limits
	companies     map[string]BCCompany
	armoury       []GearOffer
	keywords      map[string]string
	parameterised map[string]string // the definition of each rule with a parameter, by lower-cased name
	ruleTargets   RuleTargets       // data.ruleTargets: who each keyword rule works against (empty in a custom ruleset)
	rating        RatingRules
	annotations   map[string]GearAnnotation // by lower-cased name and alias
	weaponRules   []GearAnnotation          // the annotations that give a rule to a weapon the player chooses (Envenom Weapon (X))
	limitRules    []LimitRule
	fillIns       []unitFillIn
	scenarios     []ScenarioInfo
	selection     ScenarioSelection
	labels        map[string]string // rule key -> the label it is shown under (data.keywords label)
	armourUp      ArmourUpgrade
	injuries      InjuryRules
	recruit       RecruitRules
	creatureRules CreatureRules
	creatures     []CreatureOffer
	wandererRules WandererRules
	wanderers     []WandererOffer
	progression   ProgressionRules
	profiles      []CompanyProfile
	profileOnce   sync.Once
	profileSets   *profileSet
	advancements  map[string][]Advancement
	pathRules     map[string][]UnitRule // the special rules each path unlocks, in order, with their text
	readAs        map[string]string     // unit|option (lower-cased) -> the armoury item an option is read as (data.optionSales readAs)
	unsold        map[string]string     // unit|option (lower-cased) -> "status: reason" of the options no armoury sells (data.optionSales)
	breakRules    BreakRules
	xpGains       map[string]int // experience.gains[].amount by key
	ato           ATORules
	ttsModels     TTSModels       // data.ttsModels
	auras         AuraTable       // data.auras
	reserved      map[string]bool // unit|option (lower-cased) of the options reserved to a named hero's army list
	aftermath     AftermathRules
}

// RatingRules are the Battle Companies rules that turn a hero's profile into a rating (its points).
type RatingRules struct {
	PerPoint       map[string]int // rating per point of a characteristic, by key ("might": 5, "attack": 10)
	PerSpecialRule int
	heroStats      map[string]bool  // the heroes-only characteristics: they count from zero, whatever the unit's own profile has
	Wargear        WargearThreshold // when a hero pays the hero rating for its wargear
	// PerMinorSpecialRule is the rating of a minor special rule, MaxMinorSpecialRule the most all of a hero's minor rules add.
	PerMinorSpecialRule, MaxMinorSpecialRule int
	Average                                  map[string]int // the average Battle Company profile: advancements up to it add no rating
}

// BCCompany is a Battle Company's page: its limits and its reinforcement charts.
// DisplayName is the company's name as the pages spell it.
func (b BCCompany) DisplayName() string { return cmp.Or(b.Label, b.Name) }

type BCCompany struct {
	Name              string
	Label             string // how the pages spell the name when it differs from the key (Lothlórien); empty: the name
	Alignment         string // good or evil
	MaxModels         int
	ReinforcementCost int // influence per roll on the reinforcement chart (it differs per company)
	Reinforcements    []ChartRow
	SpecialChart      []ChartRow
	SpecialUnits      []SpecialUnit // units that can be bought directly, at their own influence cost
	UniqueWargear     []GearOffer
	RecruitNotes      []string // what the company page says about its reinforcements
	LimitExemptions   []LimitExemption
	LimitOverrides    map[string]lean.Share // a company's own share for a limit, by limit id (Osgiliath: bows 1/2)
}

// ChartRow is one line of a D6 chart: rolls Min to Max give Result.
type ChartRow struct {
	Min, Max int
	Result   Reinforcement
}

// Reinforcement is what a roll on a reinforcement chart gives.
type Reinforcement struct {
	Type    string // "nothing", "unit", "specialChart" (go on to the special chart), "choice" (one of the results above, see Text) or "lostStartingUnit"
	Text    string // what a "choice" result says (display text)
	Scope   string // a "choice" result: "above" (the rows above the roll) or "any" (any row of both charts)
	Picks   int    // a "choice" result: how many results the player takes
	Unit    string
	Count   int
	Wargear []string
	Choice  bool         // the player picks the unit's options
	Blocks  bool         // "nothing" that also blocks further rolls
	Rare    int          // at most this many of the unit in the company at once (0: no limit)
	Options []ChoiceUnit // for "unitChoice": the units the player may pick one of
	Alt     string       // a second unit the player may take instead of Unit, at no cost (Fornost: a Hobbit Archer on 2 to 6)
}

// ChoiceUnit is one of the units a "unitChoice" result lets the player pick: a profile of its own (a unit with its options
// selected, under the company's label), with the number of that profile the company may have.
type ChoiceUnit struct {
	Unit     string
	Label    string
	Selected []string
	Rare     int
}

// SpecialUnit is a unit a company can recruit directly for its own influence cost (Moria's Cave Troll costs 15).
type SpecialUnit struct {
	Unit string // as the company calls it

	Cost   int
	Rare   int
	Choice bool
}

// GearOffer is something the armoury sells: wargear (no slot) or equipment (a small or large slot).
type GearOffer struct {
	Name           string
	AlsoCalled     []string // other spellings of the same item (data.gearAnnotations aliases: Warspear); never a different item
	Classes        []EquipmentClass
	Cost           int      // influence
	Size           string   // "small" or "large" for equipment, empty for wargear
	Restrictions   []string // the badges the page shows for who may take it: hero, cavalry, evil, unique (made from Who; nothing reads them to decide)
	Gold           Rating   // what it costs in Gold, for a warrior and for a hero (its rating); zero when it cannot be bought with Gold
	Rating         Rating   // what the item adds to a model's rating (its points)
	Description    string
	Grants         []string // special rules the item gives its bearer (the mount's when GrantedTo is "mount")
	GrantedTo      string
	Stats          string    // the missile profile, `24" · S2`
	Exclusive      bool      // only the company has it (its unique wargear)
	Who            *lean.Who // who may take it (Admits decides); none: whoever the profile options allow
	Limit          int       // the most models of the company that may carry a unique item; 0 for no limit
	WeaponModifier bool      // a modifier bought for one weapon, not an item (Envenom Weapon)
	ArmourStage    int       // above 0: the offer is the upgrade to this armour stage (see ArmourUpgrade); it replaces the armour the model wears
}

// Rating is what a piece of wargear or equipment adds to the rating (the points) of a warrior and of a hero.
type Rating struct{ Warrior, Hero int }

// For is the rating the item adds to a unit of that kind.
func (r Rating) For(hero bool) int {
	if hero {
		return r.Hero
	}
	return r.Warrior
}

// Roll finds the result of a D6 (or other) roll on a chart.
func Roll(chart []ChartRow, roll int) (Reinforcement, bool) {
	for _, r := range chart {
		if roll >= r.Min && roll <= r.Max {
			return r.Result, true
		}
	}
	return Reinforcement{}, false
}

// Aftermath is what the War's Aftermath reads from the file: the experience and influence gains, the injury charts, the Against the Odds bonuses
// and the rewards of the scenarios.
func (b *BCBook) Aftermath() AftermathRules { return b.aftermath }

// Scenarios lists the scenarios of the file by their place on the selection tables, the unplaced last.
func (b *BCBook) Scenarios() []ScenarioInfo { return b.scenarios }

// Shape is the company shape of the file (campaign.company): the officers of a company, what a new hero starts with and the model limits.
func (b *BCBook) Shape() CompanyRules { return b.shape }

// HeroBonus is what a hero starts with (campaign.company.startingHeroStats), plus extra.
func (b *BCBook) HeroBonus(extra map[string]int) map[string]int {
	return b.shape.HeroStart.Bonus(extra)
}

// Selection is how the dice pick a scenario (campaign.scenarioSelection).
func (b *BCBook) Selection() ScenarioSelection { return b.selection }

// Creatures and Wanderers are what the armoury also sells besides gear.
func (b *BCBook) Creatures() (CreatureRules, []CreatureOffer) { return b.creatureRules, b.creatures }
func (b *BCBook) Wanderers() (WandererRules, []WandererOffer) { return b.wandererRules, b.wanderers }

// Progression is the level-up system; Advancements are the promotions a company offers.
func (b *BCBook) Progression() ProgressionRules          { return b.progression }
func (b *BCBook) Advancements(c BCCompany) []Advancement { return b.advancements[c.Name] }

// Injuries are the injury chart and the treatments of the file.
func (b *BCBook) Injuries() InjuryRules { return b.injuries }

// Recruiting are the recruiting parameters of the file: the adjustment of rolls with influence, the die, Rare, the perks that move
// a die and the starting influence.
func (b *BCBook) Recruiting() RecruitRules { return b.recruit }

// Annotation finds what the file says about a piece of gear by its name or an alias, ignoring case.
func (b *BCBook) Annotation(name string) (GearAnnotation, bool) {
	a, ok := b.annotations[NameKey(name)]
	if !ok {
		if base, _, chosen := b.ChosenWeapon(name); chosen {
			a, ok = b.annotations[NameKey(base)]
		}
	}
	return a, ok
}

// Limits counts a company's limits (bows, throwing weapons, cavalry) with its exemptions and its own shares.
func (b *BCBook) Limits(c BCCompany, units []CompanyUnit, stored ...Equipment) []CompanyLimit {
	return computeLimits(b.limitRules, c, units, stored...)
}

// Company finds a company by its Battle Companies name ("Minas Tirith").
func (b *BCBook) Company(name string) (BCCompany, bool) {
	if c, ok := b.companies[name]; ok {
		return c, true
	}
	for key, c := range b.companies { // a company stored under an older spelling ("Lake-Town" before "Lake-town") still resolves
		if sameName(key, name) {
			return c, true
		}
	}
	return BCCompany{}, false
}

// Armoury is everything a company's heroes and warriors can buy: the common wargear and equipment, and the company's own
// unique wargear.
func (b *BCBook) Armoury(c BCCompany) []GearOffer {
	return slices.Concat(b.armoury, c.UniqueWargear)
}

// Offer finds an item the armoury sells by name (or by one of its other names), ignoring case.
func (b *BCBook) Offer(c BCCompany, name string) (GearOffer, bool) {
	for _, o := range b.Armoury(c) {
		if namesItem(o.Name, o.AlsoCalled, name) {
			return o, true
		}
	}
	if base, _, chosen := b.ChosenWeapon(name); chosen {
		return b.Offer(c, base)
	}
	return GearOffer{}, false
}

// Keyword finds the definition of a Battle Companies special rule.
func (b *BCBook) Keyword(name string) (string, bool) {
	if d, ok := b.keywords[name]; ok {
		return d, true
	}
	return b.ParameterisedKeyword(name)
}

// RuleTargets is the table of who the keyword rules work against; empty when the file has none.
func (b *BCBook) RuleTargets() RuleTargets {
	if b == nil {
		return RuleTargets{}
	}
	return b.ruleTargets
}

// DefinedKeyword is the definition of a rule the file defines under exactly this name, not one a parameterised rule gives it.
func (b *BCBook) DefinedKeyword(name string) (string, bool) { d, ok := b.keywords[name]; return d, ok }

// ParameterisedKeyword is the definition of a rule written with a parameter ("Hatred (Spirit)", "Poisoned Attacks (bow)", or the
// bare "Poisoned Attacks"): the definition of the parameterised rule of that name (data.parameterisedRules).
func (b *BCBook) ParameterisedKeyword(name string) (string, bool) {
	d, ok := b.parameterised[NameKey(parameter.ReplaceAllString(name, ""))]
	return d, ok
}

// ErrRefused is the error for something the rules do not allow right now (not enough influence, no free slot...); the
// reason is joined to it and is fit to show.
var ErrRefused = errors.New("not allowed")

// Refuse builds an ErrRefused with a reason.
func Refuse(format string, args ...any) error {
	return errors.Join(ErrRefused, fmt.Errorf(format, args...))
}

// GearMatchesOption says whether an armoury item is one of the items a profile option names ("Shield and Spear" gives a shield and
// a spear). Warriors may only buy wargear that matches one of their profile's options. The match is by identity (ItemKey and
// the item's own aliases): a Pike is not a Spear, a Longbow is not an Elf bow, a Crossbow is not a Bow, whatever their classes.
func GearMatchesOption(offer GearOffer, option string) bool {
	for _, part := range OptionItems(option) {
		if namesItem(offer.Name, offer.AlsoCalled, part) {
			return true
		}
	}
	return false
}

func reservedKey(unit, option string) string {
	return NameKey(unit) + "|" + NameKey(option)
}

// Reserved reports that an option of a unit belongs to one named hero's army list (derived from the vanilla file): a Battle Company
// has no such list, so a "choice of options" never offers it (a company's unitAliases can still select it by name).
func (b *BCBook) Reserved(unit, option string) bool {
	return b != nil && b.reserved[reservedKey(unit, option)]
}

// Unsold says that no armoury sells an option of a unit and why (data.optionSales: "notSold: ..." or "granted: ..."), so a
// profile option without a matching armoury item is not a data defect.
func (b *BCBook) Unsold(unit, option string) (string, bool) {
	if b == nil {
		return "", false
	}
	r, ok := b.unsold[reservedKey(unit, option)]
	return r, ok
}

// ReadAs is the armoury item a profile option is read as ("Troll Spear" is the Spear), when the data says so.
func (b *BCBook) ReadAs(unit, option string) (string, bool) {
	if b == nil {
		return "", false
	}
	item, ok := b.readAs[reservedKey(unit, option)]
	return item, ok
}

// ChoosableOptions are the options of a profile a "choice of options" recruit may take: the ones a Battle Companies armoury item
// stands for. Not those no armoury sells (data.optionSales notSold or granted: banners, war horns and
// drums, Twin Blades...) nor those reserved to a named hero's army list (data.reservedOptions). An option read as an armoury item
// (readAs) is choosable. The wargear of the profile itself is not an option: it is added on recruit whether sold or not
// (data.profileWargearNotSold lists what can never be bought).
func (b *BCBook) ChoosableOptions(unit string, options []ChoiceOption) []ChoiceOption {
	var out []ChoiceOption
	for _, o := range options {
		if _, unsold := b.Unsold(unit, o.Name); unsold || b.Reserved(unit, o.Name) {
			continue
		}
		out = append(out, o)
	}
	return out
}

// HeroRating is the rating (the points) of a hero before its wargear and equipment, by the Battle Companies rules: the
// points of the Warrior profile it was built from, plus the rating of what it gained: 5 per point of Might, Will and Fate,
// 5 per point of Movement, Fight, Strength and Defence and 10 per point of Attack and Wounds above the Warrior's profile
// and above the average Battle Company profile (advancements up to it add nothing), 5 per special rule gained and 5 per
// minor special rule gained, the minor rules together at most 10. Shoot, Courage, Intelligence, heroic actions and magical
// powers add nothing. stats are the hero's current values by characteristic key.
func (b *BCBook) HeroRating(base VanillaUnit, stats map[string]int, specialRules, minorRules int) int {
	minor := minorRules * b.rating.PerMinorSpecialRule
	if b.rating.MaxMinorSpecialRule > 0 {
		minor = min(minor, b.rating.MaxMinorSpecialRule)
	}
	rating := base.Points + specialRules*b.rating.PerSpecialRule + minor
	for stat, per := range b.rating.PerPoint {
		value := stats[stat]
		floor := max(base.Stats[stat], b.rating.Average[stat])
		if b.rating.heroStats[stat] {
			floor = 0
		}
		rating += per * max(0, value-floor)
	}
	return rating
}

// unitFillIn gives a vanilla unit the fields it leaves empty (data.unitFillIns): race, faction and unit type drive company limits.
type unitFillIn struct {
	unit                     string
	race, faction, unitTypes []string
}

// RuleLabel is the name a rule is shown under: its label when the file gives one (Keen Sight (Falcon) is shown as Keen Sight), else
// its name. The name is the key (identity); the app never shows a key that has a label.
func (b *BCBook) RuleLabel(name string) string {
	if l, ok := b.labels[name]; ok {
		return l
	}
	return name
}

// Scenario finds a scenario of the file by name.
func (b *BCBook) Scenario(name string) (ScenarioInfo, bool) {
	for _, sc := range b.scenarios {
		if sc.Name == name {
			return sc, true
		}
	}
	return ScenarioInfo{}, false
}
