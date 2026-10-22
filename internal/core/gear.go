package core

import (
	"math"
	"slices"
	"strconv"
	"strings"
)

// GearAnnotation is what the Battle Companies file says about one piece of gear (data.gearAnnotations): what it is, what it
// grants, and its missile profile.
type GearAnnotation struct {
	Name           string
	Classes        []EquipmentClass // the first is the main one
	Aliases        []string
	Grants         []string // special rules the bearer gets (the mount's when GrantedTo is "mount")
	GrantedTo      string   // "mount" when the rules belong to the mount; empty for the bearer
	Range          int      // inches, for bows, crossbows, throwing weapons and missiles
	Strength       int
	Definition     string        // what the item does, in words (shown on hover)
	MountProfile   string        // for a mount: the name of its vanilla entry, which has its stat block
	Base           string        // the generic item this one is a version of (Elven spear: Spear); the tooltip already explains the modifiers
	Modifiers      []string      // keys of data.gearAnnotations.modifiers (elven, masterForged, poisoned)
	PricedAs       string        // an Elven-made item sold at the price of its generic equivalent (derived from Base + the elven modifier)
	VariantOf      string        // a specific version of another item that fills its place in a profile option
	Related        []string      // related but distinct items (Elf Bow and Longbow share a profile): never the same item
	RequiresMount  bool          // only a mounted model can use it (a lance, Barding): struck through in a live match when the mount is lost
	WeaponModifier bool          // not an item but a modifier bought for one weapon (Envenom Weapon)
	Situational    []StatChange  // bonuses that hold only in a game situation (Elven Glaive attacks): item text, never added to the stats
	StatChanges    []StatChange  // armour: what it adds to a characteristic (data.gearAnnotations statChanges)
	Defence        *DefenceBonus // a shield: the Defence it adds and what cancels it
}

// StatChange is a change to a characteristic.
type StatChange struct {
	Stat  string // the stat key (defence, strength...)
	Delta int
	// Condition is set when the change holds only under a condition (R8): it is then a note on the item and never added to the unit's
	// characteristics, the player applies it. The conservative default: any change the data marks conditional.
	Condition string
}

// DefenceBonus is a shield's Defence bonus (the defenceBonus ability): it is lost while the model carries an item of a cancelling
// class or a cancelling item, unless a rule lifts that cancellation.
type DefenceBonus struct {
	Value              int
	NotWhileEngaged    bool             // the Pavise's bonus does not apply in combat
	Conditional        bool             // situational (the data's conditional flag): item text only, never added to the Defence
	Condition          string           // the condition as the data words it
	Against            string           // a bonus that holds against one kind of attack only (Small Shield: "strikes"): it never changes the profile's Defence, the card notes it
	CancelledByClasses []EquipmentClass // bow, crossbow, two_handed
	CancelledByItems   []string         // Pike
	TwoHandedUse       bool             // a hand-and-a-half weapon used two-handed also cancels it; decided per fight offline, so the app only shows it
	Lifted             []DefenceLift
}

// DefenceLift is a rule that lifts a cancellation of a shield's bonus (Expert Rider: the bow's, while mounted; Phalanx: the pike's).
type DefenceLift struct {
	Rule         string
	Cancels      []string // classes or item names whose cancellation the rule lifts
	WhileMounted bool
}

// Stats is the missile profile as shown next to the item: `24" · S2`; empty for items without one.
func (a GearAnnotation) Stats() string {
	if a.Range == 0 {
		return ""
	}
	return formatStats(a.Range, a.Strength)
}

func formatStats(rng, strength int) string {
	return strconv.Itoa(rng) + `" · S` + strconv.Itoa(strength)
}

// IsUnitUpgrade says whether an annotated entry is a named unit upgrade (Helmingas, Hearthguard, Tookish Hunter...) rather
// than gear: it is classed "other" and takes no slot. The cards show these as badges.
func IsUnitUpgrade(a GearAnnotation, size string) bool {
	return len(a.Classes) == 1 && a.Classes[0] == EquipOther && size == "" && !a.WeaponModifier
}

// isSteedArmour says whether the item armours the mount, not the rider (barding).
func isSteedArmour(name string) bool { return strings.EqualFold(name, "Barding") }

// LimitRule is one of the company limits of the Battle Companies file (data limits): its id, share of the models and label.
type LimitRule struct {
	ID      string
	Label   string
	Percent float64
	RoundUp bool
}

var defaultLimitRules = []LimitRule{
	{ID: "bow", Label: "Bows", Percent: 33.33, RoundUp: true},
	{ID: "throwing", Label: "Throwing weapons", Percent: 33.33, RoundUp: true},
	{ID: "cavalry", Label: "Cavalry", Percent: 33.33, RoundUp: true},
}

// LimitExemption is a company rule that some models or items do not count against some limits.
type LimitExemption struct {
	Unit   string // a unit type whose models are left out of the count and of the base the maximum is worked out on
	Item   string // an item that is not counted
	Limits []string
	Text   string
}

func (e LimitExemption) covers(limit string) bool { return slices.Contains(e.Limits, limit) }

// computeLimits counts each limit by class and derives its maximum from the models that count: bow and crossbow items for the
// Bow limit, throwing items for the Throwing limit (missiles count against neither) and the Cavalry keyword for Cavalry. A
// model exempted from a limit is left out of its count and of its base; an exempt item is not counted.
func computeLimits(rules []LimitRule, c BCCompany, units []CompanyUnit, stored ...Equipment) []CompanyLimit {
	var out []CompanyLimit
	for _, rule := range rules {
		percent := rule.Percent
		if p, ok := c.LimitOverrides[rule.ID]; ok {
			percent = p
		}
		base, used := 0, 0
		for _, u := range units {
			if slices.ContainsFunc(c.LimitExemptions, func(e LimitExemption) bool { return e.Unit != "" && e.Unit == u.Type && e.covers(rule.ID) }) {
				continue
			}
			base++
			if rule.ID == "cavalry" && u.isCavalry() {
				used++
			}
			for _, e := range u.Equipment {
				if slices.ContainsFunc(c.LimitExemptions, func(x LimitExemption) bool {
					return x.Item != "" && strings.EqualFold(x.Item, e.Name) && x.covers(rule.ID)
				}) {
					continue
				}
				switch rule.ID {
				case "bow":
					if slices.ContainsFunc(e.Classes(), EquipmentClass.isRangedLimit) {
						used++
					}
				case "throwing":
					if slices.Contains(e.Classes(), EquipThrowing) {
						used++
					}
				}
			}
		}
		for _, e := range stored { // gear in the Baggage Pony's pack still counts towards the ranged limits
			if slices.ContainsFunc(c.LimitExemptions, func(x LimitExemption) bool {
				return x.Item != "" && strings.EqualFold(x.Item, e.Name) && x.covers(rule.ID)
			}) {
				continue
			}
			switch rule.ID {
			case "bow":
				if slices.ContainsFunc(e.Classes(), EquipmentClass.isRangedLimit) {
					used++
				}
			case "throwing":
				if slices.Contains(e.Classes(), EquipThrowing) {
					used++
				}
			}
		}
		out = append(out, CompanyLimit{ID: rule.ID, Label: rule.Label, Used: used, Max: LimitMax(percent, base), Next: LimitMax(percent, base+1)})
	}
	return out
}

// LimitMax is the most models a limit allows in a group of n models: the share of n, always rounded up (Battle Companies:
// "a third of the models" is 2 of 6 and 3 of 7). A model joining the company counts in n, so a limit that is full at n models
// may have room at n+1 (CompanyLimit.Next).
func LimitMax(percent float64, n int) int {
	return int(math.Ceil(percent/100*float64(n) - 1e-9))
}
