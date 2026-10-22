package core

import (
	"encoding/json"
	"strings"
)

// CreatureRules are the Battle Companies rules for creatures (data.creatures.rules): animals bought for the officers.
type CreatureRules struct {
	AttachTo      []string // the roles that may have one: leader, sergeant
	MaxPerHero    int
	MaxPerCompany int
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
}

// ArmouryWanderer is a wanderer of the armoury and whether it can be hired now.
type ArmouryWanderer struct {
	WandererOffer
	PayInfluence, PayGold bool
	StatBlock             []Attribute
	Reason                string
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

// ParseCreaturesAndWanderers reads data.creatures and data.wanderers of the Battle Companies file.
func ParseCreaturesAndWanderers(content []byte) (CreatureRules, []CreatureOffer, WandererRules, []WandererOffer, error) {
	type item map[string]json.RawMessage
	var doc struct {
		Data struct {
			Creatures struct {
				Rules struct {
					AttachTo      []string `json:"attachTo"`
					MaxPerHero    int      `json:"maxPerHero"`
					MaxPerCompany int      `json:"maxPerCompany"`
				} `json:"rules"`
				Items []item `json:"items"`
			} `json:"creatures"`
			Wanderers struct {
				Rules struct {
					MaxAtOnce int `json:"maxAtOnce"`
				} `json:"rules"`
				Items []item `json:"items"`
			} `json:"wanderers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(content, &doc); err != nil {
		return CreatureRules{}, nil, WandererRules{}, nil, err
	}
	stats := func(raw item) map[string]int {
		out := map[string]int{}
		for _, k := range []string{"movement", "fight", "shoot", "strength", "defence", "attack", "wounds", "courage", "intelligence", "might", "will", "fate"} {
			var v int
			if raw[k] != nil && json.Unmarshal(raw[k], &v) == nil && (v != 0 || k == "attack" || k == "wounds") {
				out[k] = v
			}
		}
		return out
	}
	str := func(raw item, key string) (s string) { _ = json.Unmarshal(raw[key], &s); return }
	num := func(raw item, key string) (n int) { _ = json.Unmarshal(raw[key], &n); return }
	list := func(raw item, key string) (l []string) { _ = json.Unmarshal(raw[key], &l); return }

	cr := CreatureRules{AttachTo: doc.Data.Creatures.Rules.AttachTo, MaxPerHero: doc.Data.Creatures.Rules.MaxPerHero, MaxPerCompany: doc.Data.Creatures.Rules.MaxPerCompany}
	var creatures []CreatureOffer
	for _, raw := range doc.Data.Creatures.Items {
		c := CreatureOffer{Name: str(raw, "name"), Cost: num(raw, "influence"), Gold: num(raw, "gold"), Points: num(raw, "points"), Stats: stats(raw), Rules: list(raw, "specialRules"), Notes: list(raw, "notes")}
		if types := list(raw, "unitType"); len(types) > 0 {
			c.Kind = strings.Join(types, " ")
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
		if c.Name != "" {
			creatures = append(creatures, c)
		}
	}
	wr := WandererRules{MaxAtOnce: doc.Data.Wanderers.Rules.MaxAtOnce}
	var wanderers []WandererOffer
	for _, raw := range doc.Data.Wanderers.Items {
		w := WandererOffer{Name: str(raw, "name"), Cost: num(raw, "influence"), Gold: num(raw, "gold"), Points: num(raw, "points"), UnitTypes: list(raw, "unitType"), Stats: stats(raw),
			HeroicActions: list(raw, "heroicActions"), Rules: list(raw, "specialRules"), Wargear: list(raw, "wargear"), Note: str(raw, "keywordNote")}
		if w.Name != "" {
			wanderers = append(wanderers, w)
		}
	}
	return cr, creatures, wr, wanderers, nil
}
