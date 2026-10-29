package core

import (
	"math"
	"slices"
	"strconv"
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
	Definition     string            // what the item does, in words (shown on hover)
	MountProfile   string            // for a mount: the name of its vanilla entry, which has its stat block
	Base           string            // the generic item this one is a version of (Elven spear: Spear); the tooltip already explains the modifiers
	GrantsSlots    SlotCounts        // equipment slots the item adds to its bearer (the Backpack: 3 small)
	Modifiers      []string          // keys of data.gearAnnotations.modifiers (elven, masterForged, poisoned)
	PricedAs       string            // an Elven-made item sold at the price of its generic equivalent (derived from Base + the elven modifier)
	Related        []string          // related but distinct items (Elf Bow and Longbow share a profile): never the same item
	RequiresMount  bool              // only a mounted model can use it (a lance, Barding): struck through in a live match when the mount is lost
	WeaponModifier bool              // not an item but a modifier bought for one weapon (Envenom Weapon)
	ChosenWeapon   *ChosenWeaponRule // the rule the modifier gives the UNIT for the weapon the player chooses (Poisoned Attacks)
	Situational    []StatChange      // bonuses that hold only in a game situation (Elven Glaive attacks): item text, never added to the stats
	StatChanges    []StatChange      // armour: what it adds to a characteristic (data.gearAnnotations statChanges)
	Defence        *DefenceBonus     // a shield: the Defence it adds and what cancels it
}

// ChosenWeaponRule is data.gearAnnotations grantsRuleForChosenWeapon: the player chooses ONE item of the unit whose class is among
// WeaponClasses; the unit gains the special rule `Rule (<weapon name>)` and the weapon itself stays the plain item.
type ChosenWeaponRule struct {
	Rule          string
	WeaponClasses []EquipmentClass
}

// StatChange is a change to a characteristic.
type StatChange struct {
	Stat  string // the stat key (defence, strength...)
	Delta int
	// Condition is set when the change holds only under a condition: it is then a note on the item and never added to the unit's
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
func isSteedArmour(name string) bool { return sameName(name, "Barding") }

// LimitRule is one of the company limits of the Battle Companies file (data limits): its id, share of the models and label.
type LimitRule struct {
	ID      string
	Label   string
	Percent float64
	RoundUp bool
	// Classes are the item classes the limit counts (limits[].classes); Keyword is the unit keyword it counts instead (limits[]
	// type "keyword": Cavalry). Exactly one of them is set.
	Classes []EquipmentClass
	Keyword string
}

// counts says whether an item of the given classes counts against the limit.
func (r LimitRule) counts(classes []EquipmentClass) bool {
	return slices.ContainsFunc(classes, func(c EquipmentClass) bool { return slices.Contains(r.Classes, c) })
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
// model exempted from a limit is left out of its count and of its base; an exempt item is not counted. What the exemptions leave
// out is still tallied (Exempt), so the page can show the company's whole number of bows, throwing weapons or cavalry.
func computeLimits(rules []LimitRule, c BCCompany, units []CompanyUnit, stored ...Equipment) []CompanyLimit {
	var out []CompanyLimit
	for _, rule := range rules {
		percent := rule.Percent
		if p, ok := c.LimitOverrides[rule.ID]; ok {
			percent = p
		}
		base, used, exempt := 0, 0, 0
		for _, u := range units {
			unitExempt := slices.ContainsFunc(c.LimitExemptions, func(e LimitExemption) bool { return e.Unit != "" && e.Unit == u.Type && e.covers(rule.ID) })
			if !unitExempt {
				base++
			}
			if rule.Keyword == KeywordCavalry && u.isCavalry() {
				if unitExempt {
					exempt++
				} else {
					used++
				}
			}
			for _, e := range u.Equipment {
				if !rule.counts(e.Classes()) {
					continue
				}
				if unitExempt || c.exempts(e.Name, rule.ID) {
					exempt++
				} else {
					used++
				}
			}
		}
		for _, e := range stored { // gear in the Baggage Pony's pack still counts towards the item limits
			if !rule.counts(e.Classes()) {
				continue
			}
			if c.exempts(e.Name, rule.ID) {
				exempt++
			} else {
				used++
			}
		}
		out = append(out, CompanyLimit{ID: rule.ID, Label: rule.Label, Used: used, Exempt: exempt, Max: LimitMax(percent, base, rule.RoundUp), Next: LimitMax(percent, base+1, rule.RoundUp),
			LimitClasses: rule.Classes, LimitKeyword: rule.Keyword})
	}
	return out
}

// exempts says whether the company's rules leave the item out of the limit's count.
func (c BCCompany) exempts(item, limit string) bool {
	return slices.ContainsFunc(c.LimitExemptions, func(x LimitExemption) bool {
		return x.Item != "" && sameName(x.Item, item) && x.covers(limit)
	})
}

// LimitMax is the most models a limit allows in a group of n models: the share of n, rounded up when the rule says so (Battle
// Companies: "a third of the models" is 2 of 6 and 3 of 7) and down otherwise. A model joining the company counts in n, so a limit
// that is full at n models may have room at n+1 (CompanyLimit.Next).
func LimitMax(percent float64, n int, roundUp bool) int {
	if roundUp {
		return int(math.Ceil(percent/100*float64(n) - 1e-9))
	}
	return int(math.Floor(percent/100*float64(n) + 1e-9))
}
