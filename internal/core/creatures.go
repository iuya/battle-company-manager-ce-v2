package core

import (
	"encoding/json"
	"strings"
)

// CreatureRules are the Battle Companies rules for creatures (data.creatures.rules): animals bought for the officers.
type CreatureRules struct {
	AttachTo   []string // the roles that may have one: leader, sergeant
	MaxPerHero int      // data.creatures.rules.maxPerHero; the company-wide count follows from one creature per officer
}

// CarryFeature is what the Baggage Pony gives the whole company: a pack the members can store gear in and take it from.
type CarryFeature struct {
	Capacity int
	Excludes []EquipmentClass // what cannot be carried (mounts)
}

// CreatureOffer is a creature the armoury sells.
type CreatureOffer struct {
	Name           string
	Cost           int // influence
	Gold           int // or this much Gold (its points)
	Points         int // its rating
	Kind           string
	UnitTypes      []string // the creature's unit types (Beast, Infantry, Warrior): its only keywords
	Keywords       UnitKeywords
	Stats          map[string]int // by stat key
	Rules          []string
	Notes          []string
	Carry          *CarryFeature
	CourageOfOwner bool // its Courage is its owner's (a hunting dog)
}

// WandererRules are the Battle Companies rules for wanderers (data.wanderers.rules).
type WandererRules struct {
	MaxAtOnce int
}

// WandererOffer is a mercenary hero the armoury sells. It joins the company as a unit and counts towards its limit.
type WandererOffer struct {
	Name          string
	Cost          int
	Gold          int // or this much Gold (its points)
	Points        int
	UnitTypes     []string
	Stats         map[string]int
	HeroicActions []string
	Rules         []string
	Wargear       []string
	Note          string
	Keywords      UnitKeywords
}

// AsUnit is the wanderer as a unit profile, like those of the vanilla file.
func (w WandererOffer) AsUnit() VanillaUnit {
	return VanillaUnit{Name: w.Name, Points: w.Points, Stats: w.Stats, UnitTypes: w.UnitTypes, SpecialRules: w.Rules, HeroicActions: w.HeroicActions, Wargear: w.Wargear}
}

// CompanyCreature is a creature a company owns: whose it is and how it looks.
type CompanyCreature struct {
	Name      string
	OwnerID   int64
	OwnerName string
	Points    int
	Stats     []Attribute
	Rules     []string
	Notes     []string
	Carry     *CarryFeature
}

// PackItem is a piece of gear stored with the Baggage Pony.
type PackItem struct {
	Name  string
	Class EquipmentClass
	Icon  string
	Size  string
}

// Pack is what the company's Baggage Pony carries.
type Pack struct {
	Capacity int
	Items    []PackItem
}

// Free is how many pieces more it can carry.
func (p Pack) Free() int { return p.Capacity - len(p.Items) }

// CarryAction is what a member does with the pack: store one of its items, retrieve one, or swap them.
type CarryAction struct {
	Kind string // "store", "retrieve" or "swap"
	Item string // the member's item (store, swap)
	Take string // the pack's item (retrieve, swap)
}

// UnitCreatureRef is the creature a unit owns, as the card shows it.
type UnitCreatureRef struct {
	Name  string
	Stats []Attribute
	Rules []string
	Notes []string
}

// ArmouryCreature is a creature of the armoury with, for each officer, whether they can take it now.
type ArmouryCreature struct {
	CreatureOffer
	PayInfluence, PayGold bool // how the company could pay for it now
	StatBlock             []Attribute
	Owners                []UnitOption
	Reason                string // why nobody can (company-wide), empty otherwise
	Possible              bool   // the rules let an officer of the company have a creature at all
}

// ArmouryWanderer is a wanderer of the armoury and whether it can be hired now.
type ArmouryWanderer struct {
	WandererOffer
	PayInfluence, PayGold bool
	StatBlock             []Attribute
	Reason                string
	Blocker               Blocker // what stops the hire; empty when it can be hired
	Possible              bool    // the company could hire it with a roster in which nothing else stands in the way (influence, room and today's units aside)
}

// Hirable says the wanderer can be hired now.
func (a ArmouryWanderer) Hirable() bool { return a.Reason == "" }

// CanTake says some officer can take the creature now.
func (a ArmouryCreature) CanTake() bool {
	for _, o := range a.Owners {
		if o.Able() {
			return true
		}
	}
	return false
}

// followerStats reads the characteristics of a creature or wanderer: a zero is left out except Attacks and Wounds.
func followerStats(raw map[string]json.RawMessage, vanilla *VanillaBook) map[string]int {
	out := map[string]int{}
	for _, k := range vanilla.StatKeys() {
		var v int
		if raw[k] != nil && json.Unmarshal(raw[k], &v) == nil && (v != 0 || k == "attack" || k == "wounds") {
			out[k] = v
		}
	}
	return out
}

// creatureFrom reads a creature of data.creatures.items (or of a lean gear entry written in that shape).
func creatureFrom(raw map[string]json.RawMessage, vanilla *VanillaBook) CreatureOffer {
	c := CreatureOffer{Name: rawStr(raw, "name"), Cost: rawInt(raw, "influence"), Gold: rawInt(raw, "gold"), Points: rawInt(raw, "points"), Stats: followerStats(raw, vanilla),
		Rules: rawStrings(raw, "specialRules"), Notes: rawStrings(raw, "notes")}
	if types := rawStrings(raw, "unitType"); len(types) > 0 {
		c.Kind = strings.Join(types, " ")
		c.UnitTypes = types
	}
	var courage struct {
		Same bool `json:"sameAsOwner"`
	}
	if json.Unmarshal(raw["courage"], &courage) == nil {
		c.CourageOfOwner = courage.Same
	}
	var feature struct {
		Key      string   `json:"key"`
		Capacity int      `json:"capacity"`
		Excludes []string `json:"excludes"`
	}
	if json.Unmarshal(raw["companyFeature"], &feature) == nil && feature.Key == "carryGear" {
		c.Carry = &CarryFeature{Capacity: feature.Capacity}
		for _, e := range feature.Excludes {
			c.Carry.Excludes = append(c.Carry.Excludes, EquipmentClass(e))
		}
	}
	return c
}

// wandererFrom reads a wanderer of data.wanderers.items (or of a lean gear entry written in that shape).
func wandererFrom(raw map[string]json.RawMessage, vanilla *VanillaBook) WandererOffer {
	return WandererOffer{Name: rawStr(raw, "name"), Cost: rawInt(raw, "influence"), Gold: rawInt(raw, "gold"), Points: rawInt(raw, "points"), UnitTypes: rawStrings(raw, "unitType"),
		Stats: followerStats(raw, vanilla), HeroicActions: rawStrings(raw, "heroicActions"), Rules: rawStrings(raw, "specialRules"), Wargear: rawStrings(raw, "wargear"), Note: rawStr(raw, "keywordNote")}
}
