package core

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
)

// UnitRole is a unit's place in the company hierarchy.
type UnitRole string

const (
	RoleLeader   UnitRole = "leader"
	RoleSergeant UnitRole = "sergeant"
	RoleHero     UnitRole = "hero"    // any other hero
	RoleWarrior  UnitRole = "warrior" // a regular unit
)

// order is the default position of a role in the company's list.
func (r UnitRole) order() int {
	switch r {
	case RoleLeader:
		return 0
	case RoleSergeant:
		return 1
	case RoleHero:
		return 2
	}
	return 3
}

// UnitKind is what sort of model a unit is, shown as an icon on its card.
type UnitKind string

const (
	KindInfantry         UnitKind = "infantry"
	KindCavalry          UnitKind = "cavalry"
	KindBeast            UnitKind = "beast"
	KindMonster          UnitKind = "monster"
	KindMonstrousCavalry UnitKind = "monstrous_cavalry"
)

// KindOf derives the kind from vanilla unit types ("Cavalry", "Monster", "Beast", ...).
func KindOf(unitTypes []string) UnitKind {
	has := func(t string) bool { return slices.Contains(unitTypes, t) }
	switch {
	case has("Cavalry") && has("Monster"):
		return KindMonstrousCavalry
	case has("Cavalry"):
		return KindCavalry
	case has("Monster"):
		return KindMonster
	case has("Beast") || has("War Beast"):
		return KindBeast
	}
	return KindInfantry
}

func (k UnitKind) Label() string {
	switch k {
	case KindCavalry:
		return "Cavalry"
	case KindBeast:
		return "Beast"
	case KindMonster:
		return "Monster"
	case KindMonstrousCavalry:
		return "Monstrous cavalry"
	}
	return "Infantry"
}

// EquipmentClass is what sort of item a piece of equipment is. It decides the icon and colour, where the item sorts in
// a unit's equipment list, and which company limit it counts against.
type EquipmentClass string

const (
	EquipMount      EquipmentClass = "mount"
	EquipMelee      EquipmentClass = "melee" // hand weapons, swords, axes...
	EquipTwoHanded  EquipmentClass = "two_handed"
	EquipSpear      EquipmentClass = "spear" // spears and pikes
	EquipLance      EquipmentClass = "lance" // a cavalry spear
	EquipShield     EquipmentClass = "shield"
	EquipArmour     EquipmentClass = "armour"
	EquipBow        EquipmentClass = "bow" // bows of every kind
	EquipCrossbow   EquipmentClass = "crossbow"
	EquipThrowing   EquipmentClass = "throwing"
	EquipMissile    EquipmentClass = "missile" // a ranged weapon that counts against no limit (the blowpipe)
	EquipBanner     EquipmentClass = "banner"
	EquipHorn       EquipmentClass = "instrument" // war horns, drums
	EquipConsumable EquipmentClass = "consumable" // scrolls, herbs, brew, cram
	EquipOther      EquipmentClass = "other"
)

// knownEquipmentClasses is the vocabulary of item classes (the classes a rules file may name).
var knownEquipmentClasses = []EquipmentClass{EquipMount, EquipMelee, EquipTwoHanded, EquipSpear, EquipLance, EquipShield, EquipArmour, EquipBow, EquipCrossbow,
	EquipThrowing, EquipMissile, EquipBanner, EquipHorn, EquipConsumable, EquipOther}

// EquipmentClasses is the vocabulary of item classes.
func EquipmentClasses() []EquipmentClass { return slices.Clone(knownEquipmentClasses) }

// rank is the position of a class in a unit's equipment list: mount first (with its armour right after), then melee
// weapons (lances included), shield, armour, ranged weapons, and everything else last, so units can be compared at a glance.
func (c EquipmentClass) rank() int {
	switch c {
	case EquipMount:
		return 0
	case EquipMelee, EquipTwoHanded, EquipSpear, EquipLance:
		return 2
	case EquipShield:
		return 3
	case EquipArmour:
		return 4
	case EquipBow, EquipCrossbow, EquipThrowing, EquipMissile:
		return 5
	}
	return 6
}

// hasWord reports whether text contains one of the words, as a whole word ("Mountain Boots" is not a mount). A word
// ending in "!" must match exactly ("mount!"), "bow" also matches the end of a word (Longbow), and the others match the
// start of one ("blade" in "bladed").
func hasWord(text string, words ...string) bool {
	for _, w := range words {
		if wordPattern(w).MatchString(text) {
			return true
		}
	}
	return false
}

// wordPatterns caches the compiled pattern of each word hasWord is asked for (compiling one per call cost a fifth of a page).
var wordPatterns sync.Map // word -> *regexp.Regexp

func wordPattern(w string) *regexp.Regexp {
	if re, ok := wordPatterns.Load(w); ok {
		return re.(*regexp.Regexp)
	}
	pattern := `\b` + regexp.QuoteMeta(w)
	if w == "bow" {
		pattern = `bow\b`
	} else if exact, ok := strings.CutSuffix(w, "!"); ok {
		pattern = `\b` + regexp.QuoteMeta(exact) + `\b`
	}
	re, _ := wordPatterns.LoadOrStore(w, regexp.MustCompile(`(?i)`+pattern))
	return re.(*regexp.Regexp)
}

// iconRules maps words in an item's name to the icon that draws it. The first match wins. Items that match none are drawn
// by their class (a sword for a hand weapon...) or, when they only take a slot, by the generic small or large icon.
var iconRules = []struct {
	icon  string
	words []string
}{
	{"backpack", []string{"backpack"}}, {"badge", []string{"badge", "medal"}}, {"barding", []string{"barding"}},
	{"rope", []string{"rope"}}, {"drum", []string{"drum"}}, {"horn", []string{"horn"}}, {"banner", []string{"standard", "banner"}},
	{"instrument", []string{"instrument", "flute", "lute", "pipe!"}}, {"cloak", []string{"cloak", "cape"}}, {"brew", []string{"brew", "ale", "mead"}},
	{"herbs", []string{"herb", "athelas"}}, {"talisman", []string{"talisman", "amulet", "charm"}}, {"map", []string{"map"}},
	{"boots", []string{"boots"}}, {"ring", []string{"ring"}}, {"scroll", []string{"scroll"}}, {"orb", []string{"seeing stone", "palantir", "orb"}},
	{"torch", []string{"brand", "torch"}}, {"snare", []string{"snares", "traps", "trap!"}}, {"pelt", []string{"pelt", "hide", "fur"}},
	{"cram", []string{"cram", "lembas", "bread"}}, {"belt", []string{"belt"}}, {"vial", []string{"envenom", "poison", "venom"}},
	{"blowpipe", []string{"blowpipe"}}, {"axe", []string{"axe"}},
	{"warg", []string{"warg"}}, {"camel", []string{"camel"}}, {"goat", []string{"goat"}}, // mounts that are not horses
	{"armoured-horse", []string{"armoured horse"}}, {"steed", []string{"galadhrim steed"}}, {"pony", []string{"pony"}},
	// weapons and armour drawn more precisely than by their class
	{"hammer", []string{"hammer", "mattock"}}, {"hand-and-a-half", []string{"hand-and-a-half"}}, {"twin-blades", []string{"twin blades"}},
	{"claws", []string{"claws", "teeth", "fangs", "talons", "beaks"}}, {"glaive", []string{"glaive"}}, {"halberd", []string{"halberd"}},
	{"thrown-spear", []string{"throwing spears"}}, {"smoke-bomb", []string{"smoke bomb"}}, {"chain", []string{"chain"}}, {"whip", []string{"whip"}},
	{"light-armour", []string{"light armour"}}, {"heavy-armour", []string{"heavy armour", "heavy dwarf armour"}},
	{"small-shield", []string{"small shield", "light shield"}}, {"pavise", []string{"pavise"}},
}

var iconsByName memo

// iconByName is the first icon rule the name matches, or "".
func iconByName(name string) string {
	for _, r := range iconRules {
		if hasWord(name, r.words...) {
			return r.icon
		}
	}
	return ""
}

// IconFor chooses the icon of an item: a specific one by its name (a ring, a scroll, boots, ...), else "" to draw it by its
// classes, else the generic "small" or "large" icon for equipment that only takes a slot (so a new item in the rules data
// always has an icon).
func IconFor(name string, classes []EquipmentClass, size string) string {
	if icon := iconsByName.get(name, iconByName); icon != "" {
		return icon
	}
	if len(classes) > 0 && classes[0] != EquipOther {
		return ""
	}
	switch size {
	case "small":
		return "generic-small"
	case "large":
		return "generic-large"
	}
	return ""
}

// IconKeys are the keys IconFor can return, so the page can check it draws every one.
func IconKeys() []string {
	keys := []string{"generic-small", "generic-large"}
	for _, r := range iconRules {
		keys = append(keys, r.icon)
	}
	return keys
}

// Equipment is something a unit carries: part of its starting options (Bought false) or bought during the campaign.
type Equipment struct {
	Name        string
	Class       EquipmentClass   // the main class
	Also        []EquipmentClass // other things the item is (a blade-spear is a spear and a blade)
	Bought      bool
	Icon        string      // a specific icon (IconFor); empty to draw the item by its classes
	Upgrade     bool        // a named unit upgrade (Helmingas, Hearthguard...), not gear: shown as a badge
	Modifies    string      // for a weapon modifier (Envenom Weapon): the weapon the player chose for it; empty while none is chosen
	Choices     []string    // for a weapon modifier: the weapons of the unit it could be given to now (the chosen one included)
	Profile     bool        // part of the unit's vanilla profile (a Warrior of Minas Tirith's armour), not an option or a purchase
	Rating      int         // what the item adds to the unit's points (0 for profile items, which are already in them)
	Drop        string      // why the unit cannot drop it (see DropReason); empty when it can
	GrantsSlots SlotCounts  // the equipment slots the item adds (data.gearAnnotations grantsSlots: the Backpack)
	Size        string      // "small" or "large" for equipment that takes a slot; empty for wargear, which takes none
	Stats       string      // what the item does in numbers, e.g. `24" · S2` for a bow; empty for most items
	Description string      // the item's rules, if it has any (shown on hover)
	Poisoned    bool        // it carries Poisoned Attacks (a level-up gave it to this weapon)
	Exclusive   bool        // only this company has it
	MountStats  []Attribute // for a mount: its stat block
	MountRules  []string    // and its special rules
	Spellings   []string    // other spellings of the same item (the rules file's aliases), for identity checks
	Disabled    string      // the injury that stops the hero benefiting from it (Arm Wound); empty when it works
	// RequiresMount marks gear only a mounted model can use (a lance): the live match strikes it through when the mount is lost. The rules
	// file's `requiresMount` flag feeds it.
	RequiresMount bool
}

// Classes is every class of the item, the main one first.
// isCavalry says whether the model counts as Cavalry: its profile says so, or it rides a mount it carries (a Horse bought for a Warrior).
func (u CompanyUnit) isCavalry() bool {
	return u.Kind == KindCavalry || u.Kind == KindMonstrousCavalry || slices.ContainsFunc(u.Equipment, func(e Equipment) bool { return slices.Contains(e.Classes(), EquipMount) })
}

func (e Equipment) Classes() []EquipmentClass { return append([]EquipmentClass{e.Class}, e.Also...) }

// rank is the item's place in the list: that of its best-placed class.
func (e Equipment) rank() int {
	if isSteedArmour(e.Name) {
		return 1 // barding armours the steed, not the rider: it sits right after the mount
	}
	best := e.Class.rank()
	for _, c := range e.Also {
		best = min(best, c.rank())
	}
	return best
}

// SortEquipment orders a unit's equipment the same way for every unit: mount, melee weapons, shield, armour, ranged
// weapons, then the rest; within a group the starting items come before the bought ones.
func SortEquipment(items []Equipment) []Equipment {
	out := slices.Clone(items)
	slices.SortStableFunc(out, func(a, b Equipment) int {
		if d := a.rank() - b.rank(); d != 0 {
			return d
		}
		switch {
		case a.Bought == b.Bought:
			return 0
		case a.Bought:
			return 1
		}
		return -1
	})
	return out
}

// RuleSource is where a unit's special rule comes from.
type RuleSource string

const (
	RuleFromUnit      RuleSource = "unit"
	RuleFromPath      RuleSource = "path" // heroes only: the path they chose
	RuleFromEquipment RuleSource = "equipment"
	RuleFromCompany   RuleSource = "company" // a hero upgrade only this company has
)

func (s RuleSource) rank() int {
	switch s {
	case RuleFromUnit:
		return 0
	case RuleFromPath:
		return 1
	}
	return 2
}

// UnitRule is a special rule (or magical power) of a unit.
type UnitRule struct {
	Name        string
	Description string
	Source      RuleSource
	Spell       bool
	Casting     int    // a spell's casting value (4 means 4+)
	Range       int    // a spell's range in inches
	From        string // for rules from equipment: the item that grants it
	Mount       string // for rules granted to the mount (Pony: Timid), the mount's name; empty for the bearer's own
	Label       string // the name to show when it differs from Name (the key): see Shown
	// Works is who the rule works against, for its tooltip ("enemy models with the Orc keyword"); Against holds, for a rule that works against
	// enemy models with a keyword, the alternatives of keywords (one bracket each). Both come from data.ruleTargets and stay empty without it.
	Works   string
	Against [][]string
	Needs   []string // the rule only works whilst the bearer has one of these keywords (Slay their Horses!: Cavalry)
	Friends bool     // Works names friendly models (Fury (X)): it works for them, not against
}

// WorksLabel is the lead of the Works text: "Works against" an enemy, "Works for" friendly models.
func (r UnitRule) WorksLabel() string {
	if r.Friends {
		return "Works for"
	}
	return "Works against"
}

// TipText is the description for a tooltip: with a line on who the rule works against, when the rule set says so.
func (r UnitRule) TipText() string {
	if r.Works == "" {
		return r.Description
	}
	return strings.TrimSpace(r.Description + "\n\n" + r.WorksLine())
}

// WorksLine is "Works against: enemy models with the Orc keyword."; empty without Works.
func (r UnitRule) WorksLine() string {
	if r.Works == "" {
		return ""
	}
	return r.WorksLabel() + ": " + r.Works + "."
}

// Shown is the name of the rule on cards, chips and tooltips: its label, else its name. Name stays the key.
func (r UnitRule) Shown() string {
	if r.Label != "" {
		return r.Label
	}
	return r.Name
}

// SortRules puts a unit's rules in order: its own rules, then its spells, then those of its path, then those that come
// from equipment.
func SortRules(rules []UnitRule) []UnitRule {
	out := slices.Clone(rules)
	key := func(r UnitRule) int {
		k := r.Source.rank() * 2
		if r.Spell {
			k++
		}
		return k
	}
	slices.SortStableFunc(out, func(a, b UnitRule) int { return key(a) - key(b) })
	return out
}

// ActionSource says how a hero got a heroic action.
type ActionSource string

const (
	ActionFromUnit ActionSource = "unit"   // part of the unit's vanilla profile
	ActionFromPath ActionSource = "path"   // unlocked by the hero's path
	ActionBought   ActionSource = "bought" // bought later in the campaign
)

// HeroicAction is a heroic action a hero can declare ("Heroic Strike"). Its rules text lives in the glossary.
type HeroicAction struct {
	Name   string
	Source ActionSource
}

// UnitPath is the path a hero has chosen: every hero, the leader and the sergeants included, has one.
type UnitPath struct {
	Name          string
	HeroicActions []HeroicAction // unlocked so far, including the ones bought later
	Rules         []UnitRule     // special rules gained from the path; they are not repeated among the unit's own rules
	Powers        []UnitRule     // magical powers learned through the path (the Path of the Sorcerer); Spell is true
	// MinorRules is the number of minor special rules the hero took at level-up, with a target or without (they rate 5 each, at most 10 in all);
	// a rule an item grants (Envenom Weapon's Poisoned Attacks) is not one of them. minorListed is how many of them are in Rules.
	MinorRules  int
	minorListed int
}

// Attribute is one value of a unit's profile (Mv 6", FV 4, SV 4+, ...). Boost is what the company added to the vanilla
// value (from progression), shown highlighted.
type Attribute struct {
	Abbr     string
	Name     string
	Value    int
	Trail    string
	Boost    int
	Gear     int // what the unit's gear added (a shield bought +1, a two-handed weapon taking it away -1), already in Value
	HeroOnly bool
}

// KeywordCavalry is the unit keyword the Cavalry limit counts (limits[] of type keyword).
const KeywordCavalry = "Cavalry"

// CompanyUnit is one model of a battle company. Units are nameable: Name is the player's name for it (may be empty) and
// Type is what it actually is ("Warrior of Minas Tirith").
type CompanyUnit struct {
	ID   int64
	Name string
	Type string
	// PromotedFrom is the profile the unit was recruited as when a promotion changed its Type; empty otherwise. Rare N counts it
	// where the rules say promoted models count (campaign.recruitment.rare).
	PromotedFrom string
	Role         UnitRole
	Kind         UnitKind
	Points       int
	Stats        []Attribute
	Equipment    []Equipment // sorted with SortEquipment
	// PoisonableWeapons are the weapons of the unit that Poisoned Attacks could still be given to (an Envenom Weapon, a level-up): the
	// carried weapons whose class is among the weaponClasses of Envenom Weapon, minus those a Poisoned Attacks (<weapon>) rule names.
	PoisonableWeapons []string
	Rules             []UnitRule       // sorted with SortRules; the rules of a hero's path are in Path instead
	Path              *UnitPath        // heroes only
	Creature          *UnitCreatureRef // the creature this officer owns
	Wanderer          bool             // a mercenary hired from the armoury
	StatMods          []StatMod        // what the gear changes in the characteristics, relative to the vanilla profile (already in Stats)
	Injuries          []UnitInjury     // a hero's persistent injuries (arm wound, leg wound, broken honour) with their counts
	LostAbilities     []InjuryLoss     // what the injuries took away, by the data key of the ability (AbilityStandFast, AbilityHeroicActionsOnCompany)
	Injured           bool             // misses the next game: it does not count towards the company's rating
	AtHealers         bool             // sent to the healers: its missed game is the price of the healing, field treatment cannot cancel it
	Level             int
	XP                int          // all the experience the unit has earned; each level-up roll uses LevelEvery of it
	BaseSlots         SlotCounts   // the equipment slots a model has before its items add any (campaign.company.equipmentSlots)
	LevelEvery        int          // the experience that earns one level-up roll (campaign.progression.threshold)
	PathPending       bool         // a hero that has not chosen its path yet: the company cannot play a match until it does
	OwnPath           bool         // On Its Own Path: it earns no experience and never levels up
	Models            int          // how many models it counts as against the company's maximum; 0 is one
	Parts             []UnitPart   // a unit made of several models on one card (Warg Marauder: its riders and mount)
	Keywords          UnitKeywords // race, faction and unit types (and the mount's apart), see docs/unit-keywords.md
	Auras             []Aura       // the aura ranges of the unit, unfiltered, merged by kind and radius (UnitAuras): the chips of the card, docs/aura-ranges.md
}

// UnitPart is a part of a unit made of several models: a rider, a mount.
type UnitPart struct {
	Role    string
	Name    string
	Count   int
	Wargear []string
}

// ModelCount is how many models the unit counts as against the company's maximum.
func (u CompanyUnit) ModelCount() int { return max(1, u.Models) }

// ModelCount is how many models the company has, which is what its maximum counts (a Warg Marauder is three).
func (d CompanyDetail) ModelCount() int {
	n := 0
	for _, u := range d.Roster {
		n += u.ModelCount()
	}
	return n
}

// XPInLevel is the experience earned towards the next level-up; it can be more than LevelEvery when level-ups are waiting.
func (u CompanyUnit) XPInLevel() int { return max(0, u.XP-u.Level*u.LevelEvery) }

// PendingLevelUps is how many level-up rolls the unit can make now. A unit only gains a level once its roll is made,
// so one that earned 15 XP at level 0 has 3 rolls waiting. A Wanderer and a unit On Its Own Path never level up: none waits.
func (u CompanyUnit) PendingLevelUps() int {
	if u.Wanderer || u.OwnPath {
		return 0
	}
	if u.LevelEvery <= 0 { // a unit that was not built from a ruleset has no level-ups
		return 0
	}
	return u.XPInLevel() / u.LevelEvery
}

// LockMessage is the reason a locked company gives (see LockMessage), empty when it is not locked.
func (d CompanyDetail) LockMessage() string {
	if d.OpenMatch != nil {
		return LockMessageOpenMatch(*d.OpenMatch)
	}
	return LockMessage(d.Aftermaths)
}

// Title is the unit's name, or its type when it has none.
func (u CompanyUnit) Title() string {
	if strings.TrimSpace(u.Name) != "" {
		return u.Name
	}
	return u.Type
}

// Slots is how many of a unit's equipment slots are used. The slots a model has and the ones an item adds come from the
// rules file (campaign.company.equipmentSlots, data.gearAnnotations grantsSlots).
type Slots struct {
	LargeUsed, LargeMax int
	SmallUsed, SmallMax int
}

func (s Slots) LargeFree() bool { return s.LargeUsed < s.LargeMax }
func (s Slots) SmallFree() bool { return s.SmallUsed < s.SmallMax }

// Slots counts the unit's equipment against its slots.
func (u CompanyUnit) Slots() Slots {
	s := Slots{LargeMax: u.BaseSlots.Large, SmallMax: u.BaseSlots.Small}
	for _, e := range u.Equipment {
		switch e.Size {
		case "large":
			s.LargeUsed++
		case "small":
			s.SmallUsed++
		}
		s.LargeMax += e.GrantsSlots.Large
		s.SmallMax += e.GrantsSlots.Small
	}
	return s
}

// HasHeroStats says whether the unit shows Might, Will and Fate.
func (u CompanyUnit) HasHeroStats() bool {
	for _, a := range u.Stats {
		if a.HeroOnly {
			return true
		}
	}
	return false
}

func (u CompanyUnit) IsHero() bool { return u.Role != RoleWarrior }

// CompanyLimit is how much of a capped resource a company uses (Battle Companies caps bows, throwing weapons and cavalry
// at a third of the company, rounded up: limits[] of the rules file).
type CompanyLimit struct {
	ID    string // the limit's id in the rules file (bow, throwing, cavalry)
	Label string
	Used  int
	// Exempt is how many the company has that its rules leave out of the count (Riders of Rohan and the Bow limit, Whips and the
	// Throwing limit): Used + Exempt is the company's whole number.
	Exempt int
	Max    int // the most the company's models now allow
	Next   int // the most when one more model has joined (it counts in numerator and denominator): where a recruit is checked
	// LimitClasses are the item classes the limit counts and LimitKeyword the unit keyword it counts instead (limits[].classes, type keyword).
	LimitClasses []EquipmentClass
	LimitKeyword string
}

// RoomForNew says whether a model that counts once against the limit can join the company.
func (l CompanyLimit) RoomForNew() bool { return l.Used+1 <= l.Next }

func (l CompanyLimit) Full() bool { return l.Used >= l.Max }

// Total is the company's whole number, counted or not.
func (l CompanyLimit) Total() int { return l.Used + l.Exempt }

// Segments is how many slots the page's bar has: one per allowed model (or per counted one when over the limit) and one per
// exempt one, at least one so an empty limit still draws.
func (l CompanyLimit) Segments() int { return max(1, max(l.Used, l.Max)+l.Exempt) }

// CompanyDetail is a company with everything its page shows.
type CompanyDetail struct {
	CampaignCompany
	Influence int
	Gold      int
	Limits    []CompanyLimit
	MaxUnits  int           // the most models the company may have (it differs per company)
	Recruit   RecruitInfo   // what recruiting costs and can give
	Roster    []CompanyUnit // the units, in the company's order (see OrderUnits)
	// InjuredUnits is how many units are injured; their points are left out of the company's rating (Points).
	InjuredUnits int
	Settings     CampaignSettings
	// BreakPointPercent is the share of the starting models whose loss breaks the company: its own breakPoint rule or the file's default (never 0).
	BreakPointPercent int
	Rerolls           int               // Against the Odds re-rolls the company can still use
	Creatures         []CompanyCreature // the creatures the officers own (not units: they take no roster slot)
	Cemetery          []FallenUnit      // the units that died: they left the roster, and cannot come back to it
	// RangerMayReturn: the company's replacement unit (the Shire's Ranger of the North) is in the cemetery and none is on the roster, so a
	// reinforcement may bring one back (RecruitRoll.Ranger); the cemetery tags the fallen card.
	RangerMayReturn bool
	RangerUnit      string // the replacement unit's name when RangerMayReturn
	// OfficersPending: a Ranger of the North was chosen at a recruit and the player has yet to arrange the officers (PendingOfficers,
	// ArrangeOfficers); the company is locked until then.
	OfficersPending bool
	Pack            *Pack       // what the Baggage Pony carries; nil when the company has none
	Injuries        InjuryRules // what treating injuries costs and does (the rules file)
	// RuleDice are the dice the pages roll for the company outside the aftermath: the level-ups and the Courage test of a Broken company.
	RuleDice RuleDice
	// Aftermaths are the aftermaths of ended matches the company still has to finish: it is locked until then. Perks and History
	// come from the aftermath storage (CompanyService.WithAftermaths): the scenario perks, unused first, and the latest 20 lines
	// of the company's aftermaths.
	Aftermaths []AftermathRef
	// OpenMatch is the open match the company is in (nil: none): the page shows the live view and the company is locked.
	OpenMatch *OpenMatchRef
	Perks     []Perk
	History   []AftermathLine
}

// RuleDice are dice of the rules file the pages need to throw and to ask for.
type RuleDice struct {
	Level   LevelDice
	Courage DiceSpec // campaign.breakPoint.whenBroken.courageTest.dice
	// The Against the Odds Reroll bonus (campaign.againstTheOdds.bonuses[key=reroll]): what it costs in Rating and how many re-rolls it gives.
	RerollRating, RerollCount int
}

// RecruitInfo is what a company can do to get new units: roll on its reinforcement chart for a cost in influence (it
// differs per company; a 6 may lead to a special chart), and buy special units directly for their own cost.
type RecruitInfo struct {
	Cost         int
	Chart        []RecruitRow
	SpecialChart []RecruitRow // the chart a "special chart" result leads to; empty when the company has none
	Special      []SpecialRecruit
	Notes        []string     // what the company's page says about its reinforcements
	Rules        RecruitRules // the recruiting parameters of the rules file: the window prints its numbers from here
}

// RecruitRow describes one line of a chart for the recruit window.
type RecruitRow struct {
	Min, Max int
	Text     string
	Alt      string      // the unit the player may take instead, when the company gives two options on this roll
	Rare     int         // the company may have at most this many of the unit at once (0: no limit)
	Options  []RowOption // for a result that lets the player pick one unit among several (Wanderers' Hobbits)
}

// RowOption is one of the units a chart row lets the player pick, with the reason it cannot be picked now.
type RowOption struct {
	Index   int
	Text    string
	Blocked string
}

// Roll is the roll as the window shows it: "4" or "1–2".
func (r RecruitRow) Roll() string {
	if r.Min == r.Max {
		return fmt.Sprint(r.Min)
	}
	return fmt.Sprintf("%d–%d", r.Min, r.Max)
}

// RecruitRoll is a recruit in progress: the reinforcement price is paid and the roll made, and the player may now adjust
// it with influence before confirming. A "special chart" result sends the recruit on to a second roll.
type RecruitRoll struct {
	Stage  string // "chart" or "special"
	Rolled bool   // false on the special chart until the player rolls on it
	Roll   int
	// PerkAdjust is the Rendezvous bonus (+1 or -1) already applied to the die and PerkRaw the die before it: the window badges it
	// ("5, Rendezvous +1 = 6"); both are zero without the perk.
	PerkAdjust, PerkRaw int
	Budget, BudgetDown  int           // how many points the roll can still be raised and lowered by, at the rules' price (campaign.recruitment.adjust); 0 where the direction is not allowed
	Adjust              AdjustRules   // the rules of the adjustment, for the window's text
	Sides               int           // faces of the reinforcement die
	Lower               *RecruitLower // set when the result cannot be recruited: the player chooses a lower one
	// Replace is set on a rolled special chart result when the company is missing a member of a team (Vault Warden Team): the
	// player may confirm with pick "member" to take that member instead of the result, at no extra cost.
	Replace string
	// Ranger is set on a rolled, successful result of the chart when the company may swap it for its replacement unit (the
	// Shire's Ranger of the North, see RangerOffer); nil otherwise. ConfirmRecruit takes it as pick "ranger".
	Ranger *RangerOffer
}

// RecruitLower is the choice of a lower result a player makes when the result they got cannot join the company.
type RecruitLower struct {
	Roll   int    // the result that cannot be recruited
	Reason string // why
	Rows   []LowerRow
	// Picks is set when the result was a "choice" of the chart rather than one that cannot join: how many results the player
	// takes (1, or more for "choose any 2"). 0 means the result cannot join.
	Picks int
}

// LowerRow is a result the player may take instead.
type LowerRow struct {
	Index   int    // its place in the list, what ChooseLower takes
	Chart   string // "Reinforcements" or "Special chart"
	Roll    string // its roll on the chart
	Text    string
	Blocked string // why it cannot be taken either; empty when it can
}

// SpecialRecruit is a unit that can be bought directly.
type SpecialRecruit struct {
	Unit    string
	Cost    int
	Rare    int
	Blocked string // why the company cannot take it now (rare ones, limits); empty when it can
}

// CanRecruit says whether a company can take another unit now, and if not why.
func (d CompanyDetail) CanRecruit(cost int) (bool, string) {
	switch {
	case d.ModelCount() >= d.MaxUnits:
		return false, fmt.Sprintf("The company is full (%d of %d models).", d.ModelCount(), d.MaxUnits)
	case d.Influence < cost:
		return false, fmt.Sprintf("That costs %d influence and the company has %d.", cost, d.Influence)
	}
	return true, ""
}

// RecruitResult tells what a recruit did. When the result is a "choice of options", Choice is set and the unit only joins
// once the player has chosen (ChooseOptions).
type RecruitResult struct {
	Roll    int
	Summary string
	Choice  *RecruitChoice
	// Next is true when the result sent the recruit on to the special chart: the recruit is not over, the player rolls again.
	Next bool
	// Lower is set when the result cannot join the company: the recruit waits for the player to choose a lower result.
	Lower bool
	// Officers is set when the Ranger of the North was chosen: the officers have to be arranged (ArrangeOfficers).
	Officers *OfficerPanel
}

// RecruitChoice is a recruit waiting for the player to pick its options.
type RecruitChoice struct {
	Unit    string
	Options []ChoiceOption // what its profile offers, minus banners, war horns and war drums
}

// ChoiceOption is one option of a unit's profile.
type ChoiceOption struct {
	Name   string
	Points int
}

// UnitOption is a unit of the company that could take an item, or the reason it cannot.
type UnitOption struct {
	ID                    int64
	Title                 string
	Type                  string
	Role                  UnitRole
	Kind                  UnitKind
	Reason                string // empty when the unit can buy the item
	Blocker               Blocker
	GoldPrice             int  // what it costs this unit in Gold (0: not for Gold)
	PayInfluence, PayGold bool // how the company could pay for it now
}

func (o UnitOption) Able() bool { return o.Reason == "" }

// ArmouryEntry is an item of the company's armoury (the Armory tab lists all of them, whoever can take them) with, for each
// unit, whether it can buy it now.
type ArmouryEntry struct {
	GearOffer
	Icon  string
	Units []UnitOption
}

// CanBuy says whether some unit can buy the item now.
func (e ArmouryEntry) CanBuy() bool {
	for _, u := range e.Units {
		if u.Able() {
			return true
		}
	}
	return false
}

// Blocker says what stops a unit from buying an item. A lack of influence, of a free slot or of room under a company limit
// is worth telling the player (they can fix it, even by dropping equipment); a rule (not a hero, not cavalry, an option
// the unit does not have...) means the item is not offered at all.
type Blocker string

const (
	BlockedByInfluence Blocker = "influence"
	BlockedBySlots     Blocker = "slots"
	BlockedByLimit     Blocker = "limit" // a company limit (bows, throwing weapons) is full: dropping an item elsewhere frees it
	BlockedByRule      Blocker = "rule"
)

// ArmouryItem is something a unit could buy, with whether it can right now and, if not, why.
type ArmouryItem struct {
	GearOffer
	Icon                  string
	Reason                string  // empty when the unit can buy it
	Blocker               Blocker // empty when it can
	GoldPrice             int     // what it costs this unit in Gold (0: not for Gold)
	PayInfluence, PayGold bool    // how the company could pay for it now
}

func (a ArmouryItem) Buyable() bool { return a.Reason == "" }

// Offered says whether the item is shown at all: the ones a rule forbids are left out.
func (a ArmouryItem) Offered() bool { return a.Blocker != BlockedByRule }

// OrderUnits puts units in the company's order. By default the leader comes first, then the sergeants, then the other
// heroes, then the regular units (each group in its given order). saved holds unit ids in the order the player chose
// (persisted by the backend); units it names come first in that order, new units follow in the default order.
func OrderUnits(units []CompanyUnit, saved []int64) []CompanyUnit {
	out := slices.Clone(units)
	slices.SortStableFunc(out, func(a, b CompanyUnit) int { return a.Role.order() - b.Role.order() })
	if len(saved) == 0 {
		return out
	}
	rank := map[int64]int{}
	for i, id := range saved {
		rank[id] = i + 1
	}
	slices.SortStableFunc(out, func(a, b CompanyUnit) int {
		ra, rb := rank[a.ID], rank[b.ID]
		switch {
		case ra == 0 && rb == 0:
			return 0
		case ra == 0:
			return 1
		case rb == 0:
			return -1
		}
		return ra - rb
	})
	return out
}

// DropReason applies the Battle Companies rules for dropping what a model carries and says why it cannot (empty when it can):
//   - equipment (the slot items) can always be dropped; it is lost and its rating goes;
//   - a hero can freely drop any wargear it chose or bought; wargear that is part of its profile only with the optional
//     rule that lets heroes discard it (heroesMayDropProfileGear);
//   - a warrior may drop wargear only if what it then carries is another option of its standard profile (or its plain
//     profile); wargear of the profile itself cannot be dropped.
//
// options are the options of the unit's vanilla profile. The rating that goes with the item is already in the points.
func DropReason(u CompanyUnit, item Equipment, options []ChoiceOption, heroesMayDropProfileGear bool) string {
	switch {
	case item.Size != "":
		return ""
	case item.Profile:
		if u.IsHero() && heroesMayDropProfileGear {
			return ""
		}
		if u.IsHero() {
			return "Part of its profile (an optional rule lets heroes discard it)"
		}
		return "Part of its profile"
	case u.IsHero():
		return ""
	}
	var rest []Equipment // the wargear it would still carry besides its profile's
	dropped := false
	for _, e := range u.Equipment {
		if e.Size != "" || e.Profile {
			continue
		}
		if !dropped && e.Name == item.Name {
			dropped = true
			continue
		}
		rest = append(rest, e)
	}
	if len(rest) == 0 || len(options) == 0 {
		return ""
	}
	for _, opt := range options {
		if matchesOption(rest, opt.Name) {
			return ""
		}
	}
	return "A warrior may only drop wargear if what is left is another option of its profile"
}

// OptionFor finds the option of a profile that is exactly what the wargear is (a shield and a spear are "Shield and
// Spear"); ok is false when none is, and for no wargear at all (the plain profile) the zero option.
func OptionFor(items []Equipment, options []ChoiceOption) (ChoiceOption, bool) {
	if len(items) == 0 {
		return ChoiceOption{}, true
	}
	for _, o := range options {
		if matchesOption(items, o.Name) {
			return o, true
		}
	}
	return ChoiceOption{}, false
}

// OptionsCover prices a warrior's wargear by the options of its profile (the price rule: vanilla base cost plus the options bought,
// nothing else). The wargear that is exactly an option costs that option; otherwise the options whole in the wargear are
// added up (a warrior may buy every option at once), and what is left over costs the cheapest option that has it as a part (a
// Knight of Dol Amroth with an armoured horse is the option "Armoured horse and lance"). It returns the points of the options
// and the wargear that no option explains.
func OptionsCover(items []Equipment, options []ChoiceOption) (points int, rest []Equipment) {
	if len(items) == 0 {
		return 0, nil
	}
	if o, ok := OptionFor(items, options); ok {
		return o.Points, nil
	}
	isItem := func(it Equipment, part string) bool {
		return GearMatchesOption(GearOffer{Name: it.Name, AlsoCalled: it.Spellings, Classes: it.Classes()}, part)
	}
	// take(items, option) returns the indexes of items that fill every part of the option, or nil.
	take := func(free []bool, option string) []int {
		var got []int
		taken := make([]bool, len(free))
		for _, part := range OptionItems(option) {
			found := false
			for i, it := range items {
				if free[i] && !taken[i] && isItem(it, part) {
					taken[i], found = true, true
					got = append(got, i)
					break
				}
			}
			if !found {
				return nil
			}
		}
		return got
	}
	free := make([]bool, len(items))
	for i := range free {
		free[i] = true
	}
	for { // the biggest option whole in what is left, the cheapest of those
		best, bestGot := -1, []int(nil)
		for oi, o := range options {
			if ParseOption(o.Name).Exchange {
				continue
			}
			got := take(free, o.Name)
			if len(got) == 0 {
				continue
			}
			if best < 0 || len(got) > len(bestGot) || (len(got) == len(bestGot) && o.Points < options[best].Points) {
				best, bestGot = oi, got
			}
		}
		if best < 0 {
			break
		}
		points += options[best].Points
		for _, i := range bestGot {
			free[i] = false
		}
	}
	var left []int
	for i := range items {
		if free[i] {
			left = append(left, i)
		}
	}
	if len(left) == 0 {
		return points, nil
	}
	cheapest := -1 // an option that has all that is left as parts of it
	for oi, o := range options {
		parts := OptionItems(o.Name)
		all := true
		for _, i := range left {
			if !slices.ContainsFunc(parts, func(part string) bool { return isItem(items[i], part) }) {
				all = false
			}
		}
		if all && (cheapest < 0 || o.Points < options[cheapest].Points) && !ParseOption(o.Name).Exchange {
			cheapest = oi
		}
	}
	if cheapest >= 0 {
		return points + options[cheapest].Points, nil
	}
	for _, i := range left {
		rest = append(rest, items[i])
	}
	return points, rest
}

// matchesOption says whether the wargear is exactly what an option gives ("Shield and Spear" is a shield and a spear).
func matchesOption(items []Equipment, option string) bool {
	parts := OptionItems(option) // "War horn, shield, and throwing spears" is three items, not two
	if len(parts) != len(items) {
		return false
	}
	used := make([]bool, len(items))
	for _, part := range parts {
		found := false
		for i, it := range items {
			if !used[i] && GearMatchesOption(GearOffer{Name: it.Name, AlsoCalled: it.Spellings, Classes: it.Classes()}, part) {
				used[i], found = true, true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// FallenUnit is a unit that died (or retired): it left the roster for good and rests in the cemetery as it was when it fell.
type FallenUnit struct {
	Unit  CompanyUnit // the unit at the moment it fell: its profile, wargear, path and level
	Fate  string      // what ended it ("Dead", a hero forced to retire)
	Match string      // where it fell
}

// OrderFallen puts the cemetery in the order the player chose (unit ids; units not named keep their place at the end).
func OrderFallen(fallen []FallenUnit, saved []int64) []FallenUnit {
	out := slices.Clone(fallen)
	rank := map[int64]int{}
	for i, id := range saved {
		rank[id] = i + 1
	}
	slices.SortStableFunc(out, func(a, b FallenUnit) int {
		ra, rb := rank[a.Unit.ID], rank[b.Unit.ID]
		switch {
		case ra == 0 && rb == 0:
			return 0
		case ra == 0:
			return 1
		case rb == 0:
			return -1
		}
		return ra - rb
	})
	return out
}

// PathOffer is what a hero that has no path yet chooses from.
type PathOffer struct {
	Unit   CompanyUnit
	Paths  []PathOption
	Powers []LevelChoice // the first magical powers a Sorcerer can begin with
}

// PathOption is a path a hero could take, or the reason it cannot.
type PathOption struct {
	Name        string
	StartAction string // the heroic action the path gives at once
	StartPower  bool   // the hero also chooses a first magical power
	Blocked     string
	Description string   // the path in a sentence (may be empty)
	Summary     string   // what it offers (heroic action, Mastery rule, rules, signature characteristic): the tooltip
	Mastery     string   // its Mastery rule
	Rules       []string // its own special rules
}
