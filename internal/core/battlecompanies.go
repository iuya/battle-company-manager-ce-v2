package core

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
)

// BCBook gives access to the Battle Companies rules the company pages need: how many models a company may have, what
// recruiting costs and what it can give, and what the armoury sells. Like VanillaBook it is the part of the future rules
// loader that the app needs now.
type BCBook struct {
	defaultMaxModels int
	eliteMaxModels   int
	companies        map[string]BCCompany
	armoury          []GearOffer
	keywords         map[string]string
	parameterised    map[string]string // the definition of each rule with a parameter, by lower-cased name
	rating           RatingRules
	annotations      map[string]GearAnnotation // by lower-cased name and alias
	spell            map[string]string         // lower-cased wargear name -> the spelling the pages use (see canonicaliseWargear)
	limitRules       []LimitRule
	fillIns          []unitFillIn
	mountFillIns     []unitFillIn // data.mountFillIns: unit is the gear name
	scenarios        []ScenarioInfo
	labels           map[string]string // rule key -> the label it is shown under (data.keywords label)
	armourUp         ArmourUpgrade
	injuries         InjuryRules
	creatureRules    CreatureRules
	creatures        []CreatureOffer
	wandererRules    WandererRules
	wanderers        []WandererOffer
	progression      ProgressionRules
	profiles         []CompanyProfile
	profileOnce      sync.Once
	profileSets      *profileSet
	advancements     map[string][]Advancement
	pathRules        map[string][]UnitRule // the special rules each path unlocks, in order, with their text
	readAs           map[string]string     // unit|option (lower-cased) -> the armoury item an option is read as (data.optionSales readAs)
	unsold           map[string]string     // unit|option (lower-cased) -> "status: reason" of the options no armoury sells (data.optionSales)
	breakRules       BreakRules
	ato              ATORules
	reserved         map[string]bool // unit|option (lower-cased) of the options reserved to a named hero's army list
}

// RatingRules are the Battle Companies rules that turn a hero's profile into a rating (its points).
type RatingRules struct {
	PerPoint       map[string]int // rating per point of a characteristic, by key ("might": 5, "attack": 10)
	PerSpecialRule int
	Average        map[string]int // the average Battle Company profile: advancements up to it add no rating
}

// BCCompany is a Battle Company's page: its limits and its reinforcement charts.
type BCCompany struct {
	Name              string
	Alignment         string // good or evil
	MaxModels         int
	ReinforcementCost int // influence per roll on the reinforcement chart (it differs per company)
	Reinforcements    []ChartRow
	SpecialChart      []ChartRow
	SpecialUnits      []SpecialUnit // units that can be bought directly, at their own influence cost
	UniqueWargear     []GearOffer
	RecruitNotes      []string // what the company page says about its reinforcements
	LimitExemptions   []LimitExemption
	LimitOverrides    map[string]float64 // a company's own share for a limit, by limit id (Osgiliath: bows 50)
}

// ChartRow is one line of a D6 chart: rolls Min to Max give Result.
type ChartRow struct {
	Min, Max int
	Result   Reinforcement
}

// Reinforcement is what a roll on a reinforcement chart gives.
type Reinforcement struct {
	Type    string // "nothing", "unit", "specialChart" (go on to the special chart), "choice" (one of the results above, see Text) or "lostStartingUnit"
	Text    string // what a "choice" result says
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
	Restrictions   []string // hero, cavalry, evil, unique...
	Gold           Rating   // what it costs in Gold, for a warrior and for a hero (its rating); zero when it cannot be bought with Gold
	Rating         Rating   // what the item adds to a model's rating (its points)
	Description    string
	Grants         []string // special rules the item gives its bearer (the mount's when GrantedTo is "mount")
	GrantedTo      string
	VariantOf      string    // a specific version of another item that fills its place in a profile option (Bow with poisoned arrows)
	Stats          string    // the missile profile, `24" · S2`
	Exclusive      bool      // only the company has it (its unique wargear)
	Only           string    // who may take a unique item ("Hobbit Heroes", "Corsair"); empty: whoever the profile options allow
	Limit          int       // the most models of the company that may carry a unique item; 0 for no limit
	WeaponModifier bool      // a modifier bought for one weapon, not an item (Envenom Weapon)
	OnlyUnits      []string  // exact vanilla unit names that may take a unique item (War Camel: Mahûd Warrior); Only is then display text only
	OnlyKind       *KindRule // a kind of model that may take it (Pony: Hobbit heroes)
	ArmourStage    int       // above 0: the offer is the upgrade to this armour stage (see ArmourUpgrade); it replaces the armour the model wears
}

// KindRule restricts a unique item to a kind of model: a race, and optionally heroes only.
type KindRule struct {
	Race       string
	HeroesOnly bool
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

// ParseBattleCompanies reads the parts of the Battle Companies file the book needs.
func ParseBattleCompanies(content []byte) (*BCBook, error) {
	type result struct {
		Type    string           `json:"type"`
		Unit    string           `json:"unit"`
		Count   int              `json:"count"`
		Wargear []string         `json:"wargear"`
		Choice  bool             `json:"choice"`
		Blocks  bool             `json:"blocksFurtherRolls"`
		Text    string           `json:"text"`
		Rare    int              `json:"rare"`
		Options []choiceUnitJSON `json:"options"`
		Label   string           `json:"label"`
	}
	type row struct {
		Roll   []int  `json:"roll"`
		Result result `json:"result"`
	}
	type gear struct {
		Name         string   `json:"name"`
		Names        []string `json:"names"`
		Type         string   `json:"type"`
		Size         string   `json:"size"`
		Influence    int      `json:"influence"`
		Restrictions []string `json:"restrictions"`
		Definition   string   `json:"definition"`
		Grants       []string `json:"grantsSpecialRules"`
		Rating       struct {
			Warrior int `json:"warrior"`
			Hero    int `json:"hero"`
		} `json:"rating"`
		Gold struct {
			Warrior int `json:"warrior"`
			Hero    int `json:"hero"`
		} `json:"gold"`
	}
	var doc struct {
		Campaign struct {
			Company struct {
				MaxModels      int `json:"maxModels"`
				EliteMaxModels int `json:"eliteMaxModels"`
			} `json:"company"`
			Rating struct {
				Hero struct {
					PerPoint []struct {
						Stat   string `json:"stat"`
						Rating int    `json:"rating"`
					} `json:"perPoint"`
					PerSpecialRule int `json:"perSpecialRule"`
				} `json:"hero"`
			} `json:"rating"`
			CharacteristicLimits struct {
				Average map[string]any `json:"average"`
			} `json:"characteristicLimits"`
			BreakPoint struct {
				Default struct {
					Percent int `json:"percent"`
				} `json:"default"`
				WhenBroken struct {
					Courage struct {
						Dice   string `json:"dice"`
						PassIf string `json:"passIf"`
					} `json:"courageTest"`
				} `json:"whenBroken"`
				Exceptions []struct {
					Scenario string `json:"scenario"`
					Setting  string `json:"scenarioSetting"`
					Rule     string `json:"rule"`
					Effect   string `json:"effect"`
					Who      string `json:"who"`
					Text     string `json:"text"`
				} `json:"scenarioExceptions"`
			} `json:"breakPoint"`
		} `json:"campaign"`
		Data struct {
			Companies []struct {
				Name           string `json:"name"`
				Alignment      string `json:"alignment"`
				MaxModels      int    `json:"maxModels"`
				Reinforcements struct {
					Influence int   `json:"influence"`
					Table     []row `json:"table"`
				} `json:"reinforcements"`
				SpecialChart struct {
					Table []row `json:"table"`
				} `json:"specialChart"`
				UnitAliases []struct {
					Label    string   `json:"label"`
					AddRules []string `json:"addSpecialRules"`
					Selected []string `json:"selectedOptions"`
				} `json:"unitAliases"`
				SpecialUnits []struct {
					Unit      string `json:"unit"`
					Label     string `json:"label"`
					Rare      int    `json:"rare"`
					Influence int    `json:"influence"`
					Choice    bool   `json:"choice"`
				} `json:"specialUnits"`
				Notes []struct {
					Section string `json:"section"`
					Text    string `json:"text"`
				} `json:"notes"`
				LimitExemptions []struct {
					Unit   string   `json:"unit"`
					Item   string   `json:"item"`
					Limits []string `json:"limits"`
					Text   string   `json:"text"`
				} `json:"limitExemptions"`
				LimitOverrides map[string]float64 `json:"limitOverrides"`
				UniqueWargear  []struct {
					Name        string   `json:"name"`
					Definition  string   `json:"definition"`
					Influence   int      `json:"influence"`
					Restriction string   `json:"restriction"`
					Units       []string `json:"restrictedToUnits"`
					Kind        *struct {
						Race       string `json:"race"`
						HeroesOnly bool   `json:"heroesOnly"`
					} `json:"restrictedToKind"`
					Limit  int `json:"limit"`
					Rating struct {
						Warrior int `json:"warrior"`
						Hero    int `json:"hero"`
					} `json:"rating"`
					Gold struct {
						Warrior int `json:"warrior"`
						Hero    int `json:"hero"`
					} `json:"gold"`
				} `json:"uniqueWargear"`
			} `json:"companies"`
			Wargear struct {
				Items         []gear `json:"items"`
				ArmourUpgrade struct {
					Stages    [][]string `json:"stages"`
					Influence int        `json:"influencePerUpgrade"`
					Rating    struct{ Warrior, Hero int }
					Gold      struct{ Warrior, Hero int }
				} `json:"armourUpgrade"`
			} `json:"wargear"`
			UnitFillIns struct {
				Items []struct {
					Unit     string   `json:"unit"`
					Race     []string `json:"race"`
					Faction  []string `json:"faction"`
					UnitType []string `json:"unitType"`
				} `json:"items"`
			} `json:"unitFillIns"`
			MountFillIns struct {
				Items []struct {
					Gear     string   `json:"gear"`
					Race     []string `json:"race"`
					Faction  []string `json:"faction"`
					UnitType []string `json:"unitType"`
				} `json:"items"`
			} `json:"mountFillIns"`
			Equipment     struct{ Items []gear } `json:"equipment"`
			Parameterised []struct {
				Name       string `json:"name"`
				Definition string `json:"definition"`
			} `json:"parameterisedRules"`
			Keywords []struct {
				Name       string `json:"name"`
				Definition string `json:"definition"`
				Label      string `json:"label"`
			} `json:"keywords"`
			ReservedOptions struct {
				Items []struct {
					Unit   string `json:"unit"`
					Option string `json:"option"`
				} `json:"items"`
			} `json:"reservedOptions"`
			OptionSales struct {
				Items []struct {
					Unit   string `json:"unit"`
					Option string `json:"option"`
					Status string `json:"status"`
					Reason string `json:"reason"`
					Item   string `json:"armouryItem"`
					Own    string `json:"item"` // ownItem: the option is an item of its own (a Troll Spear)
				} `json:"items"`
			} `json:"optionSales"`
			GearAnnotations struct {
				Items []struct {
					Name           string   `json:"name"`
					Classes        []string `json:"classes"`
					Aliases        []string `json:"aliases"`
					Grants         []string `json:"grants"`
					GrantedTo      string   `json:"grantedTo"`
					WeaponModifier bool     `json:"weaponModifier"`
					RequiresMount  bool     `json:"requiresMount"`
					Definition     string   `json:"definition"`
					VariantOf      string   `json:"variantOf"`
					PricedAs       string   `json:"pricedAs"`
					Base           string   `json:"base"`
					Modifiers      []string `json:"modifiers"`
					StatChanges    []struct {
						Stat        string `json:"stat"`
						Delta       int    `json:"delta"`
						Conditional bool   `json:"conditional"`
						Condition   string `json:"condition"`
					} `json:"statChanges"`
					Abilities []struct {
						Key                     string          `json:"key"`
						Value                   json.RawMessage `json:"value"`
						NotWhileEngaged         bool            `json:"notWhileEngaged"`
						Conditional             bool            `json:"conditional"` // situational: item text only, never added to the stats
						Condition               string          `json:"condition"`
						Against                 string          `json:"against"`
						CancelledByClasses      []string        `json:"cancelledByClasses"`
						CancelledByItems        []string        `json:"cancelledByItems"`
						CancelledByTwoHandedUse bool            `json:"cancelledByTwoHandedUse"`
						LiftedByRules           []struct {
							Rule         string   `json:"rule"`
							Cancels      []string `json:"cancels"`
							WhileMounted bool     `json:"whileMounted"`
						} `json:"liftedByRules"`
					} `json:"abilities"`
					RelatedTo []struct {
						Name string `json:"name"`
					} `json:"relatedTo"`
					Profile *struct {
						Name string `json:"name"`
					} `json:"profile"`
					Ranged *struct {
						Range    int `json:"range"`
						Strength int `json:"strength"`
					} `json:"ranged"`
				} `json:"items"`
			} `json:"gearAnnotations"`
		} `json:"data"`
		Limits []struct {
			ID      string  `json:"id"`
			Label   string  `json:"label"`
			Percent float64 `json:"maximumPercentage"`
			RoundUp bool    `json:"roundUp"`
		} `json:"limits"`
	}
	if err := json.Unmarshal(content, &doc); err != nil {
		return nil, errors.Join(ErrInvalidRuleset, errors.New("the Battle Companies file is not valid JSON"))
	}
	b := &BCBook{defaultMaxModels: doc.Campaign.Company.MaxModels, eliteMaxModels: doc.Campaign.Company.EliteMaxModels,
		companies: map[string]BCCompany{}, keywords: map[string]string{}}
	if r, err := ParseATO(content); err == nil {
		b.ato = r
	}
	b.breakRules.DefaultPercent = doc.Campaign.BreakPoint.Default.Percent
	b.breakRules.CourageDice, b.breakRules.PassIf = strings.ToLower(doc.Campaign.BreakPoint.WhenBroken.Courage.Dice), doc.Campaign.BreakPoint.WhenBroken.Courage.PassIf
	for _, e := range doc.Campaign.BreakPoint.Exceptions {
		b.breakRules.Exceptions = append(b.breakRules.Exceptions, BreakException{Scenario: e.Scenario, Setting: e.Setting, Rule: e.Rule, Effect: e.Effect, Who: e.Who, Prose: e.Text})
	}
	rows := func(in []row, ownNames map[string]bool) []ChartRow {
		var out []ChartRow
		for _, r := range in {
			if len(r.Roll) == 2 {
				unit := r.Result.Unit
				if ownNames[strings.ToLower(r.Result.Label)] { // Helminga: the company's name carries the option the alias selects
					unit = r.Result.Label
				}
				out = append(out, ChartRow{Min: r.Roll[0], Max: r.Roll[1], Result: Reinforcement{Type: r.Result.Type, Unit: unit,
					Count: max(1, r.Result.Count), Wargear: r.Result.Wargear, Choice: r.Result.Choice, Blocks: r.Result.Blocks, Text: r.Result.Text, Rare: r.Result.Rare, Options: choiceUnits(r.Result.Options)}})
			}
		}
		return out
	}
	b.reserved = map[string]bool{}
	for _, r := range doc.Data.ReservedOptions.Items {
		b.reserved[reservedKey(r.Unit, r.Option)] = true
	}
	b.unsold, b.readAs = map[string]string{}, map[string]string{}
	for _, r := range doc.Data.OptionSales.Items {
		switch r.Status {
		case "readAs": // sold, as another armoury item (a Hand-and-a-half hammer is the Hand-and-a-half Weapon): choosable, not "unsold"
			b.readAs[reservedKey(r.Unit, r.Option)] = r.Item
			continue
		case "ownItem": // an item of its own (the Troll Spear, a spear with an additional rule): choosable under its own name
			b.readAs[reservedKey(r.Unit, r.Option)] = cmp.Or(r.Own, r.Option)
			continue
		}
		b.unsold[reservedKey(r.Unit, r.Option)] = r.Status + ": " + r.Reason
	}
	b.annotations = map[string]GearAnnotation{}
	for _, it := range doc.Data.GearAnnotations.Items {
		a := GearAnnotation{Name: it.Name, Aliases: it.Aliases, Grants: it.Grants, GrantedTo: it.GrantedTo, WeaponModifier: it.WeaponModifier, RequiresMount: it.RequiresMount, Definition: strings.Join(strings.Fields(it.Definition), " ")}
		a.VariantOf, a.PricedAs, a.Base, a.Modifiers = it.VariantOf, it.PricedAs, it.Base, it.Modifiers
		for _, c := range it.StatChanges {
			sc := StatChange{Stat: c.Stat, Delta: c.Delta}
			if c.Conditional {
				if sc.Condition = strings.TrimSpace(c.Condition); sc.Condition == "" {
					sc.Condition = "under a condition the item's text gives"
				}
			}
			a.StatChanges = append(a.StatChanges, sc)
		}
		for _, ab := range it.Abilities {
			var v int
			if ab.Key == "attacksBonus" && ab.Conditional && json.Unmarshal(ab.Value, &v) == nil { // the Elven Glaive: text on the card
				a.Situational = append(a.Situational, StatChange{Stat: "attacks", Delta: v, Condition: strings.TrimSpace(ab.Condition)})
				continue
			}
			if ab.Key != "defenceBonus" || json.Unmarshal(ab.Value, &v) != nil {
				continue
			}
			d := &DefenceBonus{Value: v, Conditional: ab.Conditional, Condition: strings.TrimSpace(ab.Condition), NotWhileEngaged: ab.NotWhileEngaged, Against: ab.Against, CancelledByItems: ab.CancelledByItems, TwoHandedUse: ab.CancelledByTwoHandedUse}
			for _, c := range ab.CancelledByClasses {
				d.CancelledByClasses = append(d.CancelledByClasses, EquipmentClass(c))
			}
			for _, l := range ab.LiftedByRules {
				d.Lifted = append(d.Lifted, DefenceLift{Rule: l.Rule, Cancels: l.Cancels, WhileMounted: l.WhileMounted})
			}
			a.Defence = d
		}
		for _, r := range it.RelatedTo {
			a.Related = append(a.Related, r.Name)
		}
		if it.Profile != nil {
			a.MountProfile = it.Profile.Name
		}
		for _, c := range it.Classes {
			a.Classes = append(a.Classes, EquipmentClass(c))
		}
		if it.Ranged != nil {
			a.Range, a.Strength = it.Ranged.Range, it.Ranged.Strength
		}
		for _, n := range append([]string{it.Name}, it.Aliases...) {
			if _, seen := b.annotations[strings.ToLower(n)]; !seen {
				b.annotations[strings.ToLower(n)] = a
			}
		}
	}
	for _, l := range doc.Limits {
		b.limitRules = append(b.limitRules, LimitRule{ID: l.ID, Label: limitLabel(l.ID, l.Label), Percent: l.Percent, RoundUp: l.RoundUp})
	}
	if len(b.limitRules) == 0 {
		b.limitRules = defaultLimitRules
	}
	b.rating = RatingRules{PerPoint: map[string]int{}, PerSpecialRule: doc.Campaign.Rating.Hero.PerSpecialRule, Average: map[string]int{}}
	for _, p := range doc.Campaign.Rating.Hero.PerPoint {
		b.rating.PerPoint[p.Stat] = p.Rating
	}
	for k, v := range doc.Campaign.CharacteristicLimits.Average {
		if n, ok := v.(float64); ok {
			b.rating.Average[k] = int(n)
		}
	}
	for _, c := range doc.Data.Companies {
		ownNames := map[string]bool{} // the company's own names that change a unit (an option selected, a rule added), not just call it differently
		for _, a := range c.UnitAliases {
			if len(a.Selected) > 0 || len(a.AddRules) > 0 {
				ownNames[strings.ToLower(a.Label)] = true
			}
		}
		bc := BCCompany{Name: c.Name, Alignment: c.Alignment, MaxModels: c.MaxModels, ReinforcementCost: c.Reinforcements.Influence,
			Reinforcements: rows(c.Reinforcements.Table, ownNames), SpecialChart: rows(c.SpecialChart.Table, ownNames)}
		if bc.MaxModels == 0 {
			bc.MaxModels = b.defaultMaxModels
		}
		for _, x := range c.LimitExemptions {
			bc.LimitExemptions = append(bc.LimitExemptions, LimitExemption{Unit: x.Unit, Item: x.Item, Limits: x.Limits, Text: x.Text})
		}
		bc.LimitOverrides = map[string]float64{}
		for k, v := range c.LimitOverrides {
			bc.LimitOverrides[k] = v
		}
		for _, n := range c.Notes {
			if n.Section == "reinforcements" {
				bc.RecruitNotes = append(bc.RecruitNotes, n.Text)
			}
		}
		for _, s := range c.SpecialUnits {
			name := s.Unit
			if s.Label != "" { // the company's own name for the profile (Goblin Hulk); a unit alias resolves it
				name = s.Label
			}
			bc.SpecialUnits = append(bc.SpecialUnits, SpecialUnit{Unit: name, Cost: s.Influence, Rare: s.Rare, Choice: s.Choice})
		}
		for _, u := range c.UniqueWargear {
			only, kind := u.Restriction, (*KindRule)(nil)
			if u.Kind != nil {
				kind = &KindRule{Race: u.Kind.Race, HeroesOnly: u.Kind.HeroesOnly}
				if only == "" { // display text
					only = strings.TrimSpace(u.Kind.Race + map[bool]string{true: " Heroes"}[u.Kind.HeroesOnly])
				}
			}
			bc.UniqueWargear = append(bc.UniqueWargear, b.offerFrom(GearOffer{Exclusive: true, Only: only, OnlyUnits: u.Units, OnlyKind: kind, Limit: u.Limit, Name: u.Name, Description: u.Definition, Cost: u.Influence, Rating: Rating{u.Rating.Warrior, u.Rating.Hero}, Gold: Rating{u.Gold.Warrior, u.Gold.Hero}}, nil, ""))
		}
		b.companies[c.Name] = bc
	}
	offer := func(g gear, name string, also []string) GearOffer {
		return b.offerFrom(GearOffer{Name: name, AlsoCalled: also, Cost: g.Influence, Size: g.Size, Restrictions: g.Restrictions,
			Rating: Rating{g.Rating.Warrior, g.Rating.Hero}, Gold: Rating{g.Gold.Warrior, g.Gold.Hero}, Description: g.Definition, Grants: g.Grants}, also, name)
	}
	for _, f := range doc.Data.UnitFillIns.Items {
		b.fillIns = append(b.fillIns, unitFillIn{f.Unit, f.Race, f.Faction, f.UnitType})
	}
	for _, f := range doc.Data.MountFillIns.Items {
		b.mountFillIns = append(b.mountFillIns, unitFillIn{f.Gear, f.Race, f.Faction, f.UnitType})
	}
	up := doc.Data.Wargear.ArmourUpgrade
	b.armourUp = ArmourUpgrade{Stages: up.Stages, Cost: up.Influence, Rating: Rating{up.Rating.Warrior, up.Rating.Hero}, Gold: Rating{up.Gold.Warrior, up.Gold.Hero}}
	for _, g := range doc.Data.Wargear.Items { // a row is a price group: every name in it is its own item
		for _, n := range g.Names {
			name, also := n, []string(nil)
			if a, ok := b.annotations[strings.ToLower(n)]; ok { // the item's own name and spellings, not its row-mates
				name, also = a.Name, a.Aliases
			}
			if !slices.ContainsFunc(b.armoury, func(o GearOffer) bool { return strings.EqualFold(o.Name, name) }) {
				b.armoury = append(b.armoury, offer(g, name, also))
			}
		}
	}
	for _, it := range doc.Data.GearAnnotations.Items { // Elven-made weaponry: its own item at its generic equivalent's price
		if it.PricedAs == "" {
			continue
		}
		at := slices.IndexFunc(b.armoury, func(o GearOffer) bool { return namesItem(o.Name, o.AlsoCalled, it.PricedAs) })
		if at < 0 || slices.ContainsFunc(b.armoury, func(o GearOffer) bool { return namesItem(o.Name, o.AlsoCalled, it.Name) }) {
			continue
		}
		base := b.armoury[at]
		o := GearOffer{Name: it.Name, AlsoCalled: it.Aliases, Cost: base.Cost, Size: base.Size, Restrictions: base.Restrictions, Rating: base.Rating, Gold: base.Gold, Description: base.Description}
		b.armoury = append(b.armoury, b.offerFrom(o, it.Aliases, it.Name))
	}
	for _, g := range doc.Data.Equipment.Items {
		b.armoury = append(b.armoury, offer(g, g.Name, nil))
	}
	b.parameterised = map[string]string{}
	for _, pr := range doc.Data.Parameterised {
		if d := strings.Join(strings.Fields(pr.Definition), " "); d != "" {
			b.parameterised[strings.ToLower(pr.Name)] = d
		}
	}
	b.labels = map[string]string{}
	for _, k := range doc.Data.Keywords {
		if _, seen := b.keywords[k.Name]; !seen {
			b.keywords[k.Name] = k.Definition
		}
		if k.Label != "" && k.Label != k.Name {
			b.labels[k.Name] = k.Label
		}
	}
	defer b.canonicaliseWargear(nil) // runs after parseProfiles: the charts and the profiles are all there
	defer b.parseProfiles(content)
	b.pathRules = parsePathRules(content)
	inj, err := ParseInjuries(content)
	if err != nil {
		return nil, err
	}
	b.injuries = inj
	if b.progression, b.advancements, err = ParseProgression(content); err != nil {
		return nil, err
	}
	if b.creatureRules, b.creatures, b.wandererRules, b.wanderers, err = ParseCreaturesAndWanderers(content); err != nil {
		return nil, err
	}
	b.scenarios, _ = ParseScenarios(content) // a file without scenarios simply has none
	return b, nil
}

// Creatures and Wanderers are what the armoury also sells besides gear.
func (b *BCBook) Creatures() (CreatureRules, []CreatureOffer) { return b.creatureRules, b.creatures }
func (b *BCBook) Wanderers() (WandererRules, []WandererOffer) { return b.wandererRules, b.wanderers }

// Progression is the level-up system; Advancements are the promotions a company offers.
func (b *BCBook) Progression() ProgressionRules          { return b.progression }
func (b *BCBook) Advancements(c BCCompany) []Advancement { return b.advancements[c.Name] }

// Injuries are the injury chart and the treatments of the file.
func (b *BCBook) Injuries() InjuryRules { return b.injuries }

// offerFrom fills an offer's classes, missile profile and granted rules from the gear annotations of the file: by the
// item's name, then by the other names it is sold under. An item nobody annotated is "other" (the cards then draw a
// generic icon) and keeps the rules the armoury lists for it.
func (b *BCBook) offerFrom(o GearOffer, also []string, _ string) GearOffer {
	for _, n := range append([]string{o.Name}, also...) {
		if a, ok := b.annotations[strings.ToLower(n)]; ok {
			o.Classes, o.Stats, o.GrantedTo, o.WeaponModifier, o.VariantOf = a.Classes, a.Stats(), a.GrantedTo, a.WeaponModifier, a.VariantOf
			if a.Definition != "" { // the full text of the item, as the rules file annotates it
				o.Description = a.Definition
			}
			if len(a.Grants) > 0 {
				o.Grants = a.Grants
			}
			return o
		}
	}
	o.Classes = []EquipmentClass{EquipOther}
	return o
}

// Annotation finds what the file says about a piece of gear by its name or an alias, ignoring case.
func (b *BCBook) Annotation(name string) (GearAnnotation, bool) {
	a, ok := b.annotations[strings.ToLower(strings.TrimSpace(name))]
	return a, ok
}

// Limits counts a company's limits (bows, throwing weapons, cavalry) with its exemptions and its own shares.
func (b *BCBook) Limits(c BCCompany, units []CompanyUnit, stored ...Equipment) []CompanyLimit {
	return computeLimits(b.limitRules, c, units, stored...)
}

func limitLabel(id, fromFile string) string {
	for _, d := range defaultLimitRules {
		if d.ID == id {
			return d.Label
		}
	}
	return fromFile
}

// Company finds a company by its Battle Companies name ("Minas Tirith").
func (b *BCBook) Company(name string) (BCCompany, bool) {
	if c, ok := b.companies[name]; ok {
		return c, true
	}
	for key, c := range b.companies { // a company stored under an older spelling ("Lake-Town" before "Lake-town") still resolves
		if strings.EqualFold(key, name) {
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
	return GearOffer{}, false
}

// Keyword finds the definition of a Battle Companies special rule.
func (b *BCBook) Keyword(name string) (string, bool) {
	if d, ok := b.keywords[name]; ok {
		return d, true
	}
	return b.ParameterisedKeyword(name)
}

// DefinedKeyword is the definition of a rule the file defines under exactly this name, not one a parameterised rule gives it.
func (b *BCBook) DefinedKeyword(name string) (string, bool) { d, ok := b.keywords[name]; return d, ok }

// ParameterisedKeyword is the definition of a rule written with a parameter ("Hatred (Spirit)", "Poisoned Attacks (bow)", or the
// bare "Poisoned Attacks"): the definition of the parameterised rule of that name (data.parameterisedRules).
func (b *BCBook) ParameterisedKeyword(name string) (string, bool) {
	d, ok := b.parameterised[strings.ToLower(strings.TrimSpace(parameter.ReplaceAllString(name, "")))]
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
		if namesItem(offer.Name, offer.AlsoCalled, part) || (offer.VariantOf != "" && namesItem(offer.VariantOf, nil, part)) {
			return true
		}
	}
	return false
}

func reservedKey(unit, option string) string {
	return strings.ToLower(unit) + "|" + strings.ToLower(option)
}

// Reserved reports that an option of a unit belongs to one named hero's army list (data.reservedOptions): a Battle Company
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
// stands for (user decision 2026-10-21). Not those no armoury sells (data.optionSales notSold or granted: banners, war horns and
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
// and above the average Battle Company profile (advancements up to it add nothing), and 5 per special rule gained.
// Shoot, Courage, Intelligence, heroic actions and magical powers add nothing. stats are the hero's current values
// by characteristic key.
func (b *BCBook) HeroRating(base VanillaUnit, stats map[string]int, specialRules int) int {
	rating := base.Points + specialRules*b.rating.PerSpecialRule
	for stat, per := range b.rating.PerPoint {
		value := stats[stat]
		floor := max(base.Stats[stat], b.rating.Average[stat])
		if stat == "might" || stat == "will" || stat == "fate" {
			floor = 0
		}
		rating += per * max(0, value-floor)
	}
	return rating
}

type choiceUnitJSON struct {
	Unit     string   `json:"unit"`
	Label    string   `json:"label"`
	Selected []string `json:"selectedOptions"`
	Rare     int      `json:"rare"`
}

func choiceUnits(in []choiceUnitJSON) []ChoiceUnit {
	var out []ChoiceUnit
	for _, o := range in {
		out = append(out, ChoiceUnit{Unit: o.Unit, Label: o.Label, Selected: o.Selected, Rare: o.Rare})
	}
	return out
}

// canonicaliseWargear spells the wargear the charts, starting companies, promotions and composite models give in the
// armoury's spelling ("shield" is the Shield, "Orc bow" the Orc Bow), so that exact comparisons and the pages agree whatever
// case the file used. The armoury's spelling comes first, then the annotations', then the vanilla names (vanillaNames, given
// when the vanilla book is known); a name none of them knows is left as written.
func (b *BCBook) canonicaliseWargear(vanillaNames []string) {
	spell := map[string]string{}
	add := func(n string) {
		if k := strings.ToLower(strings.TrimSpace(n)); k != "" {
			if _, seen := spell[k]; !seen {
				spell[k] = n
			}
		}
	}
	for _, o := range b.armoury {
		add(o.Name)
	}
	for _, o := range b.armoury { // another name of the item (a plural, a hyphenated spelling) reads as the item itself
		for _, a := range o.AlsoCalled {
			if !strings.EqualFold(a, o.Name) {
				if _, seen := spell[strings.ToLower(strings.TrimSpace(a))]; !seen {
					spell[strings.ToLower(strings.TrimSpace(a))] = o.Name
				}
			}
		}
	}
	for _, c := range b.companies { // a company's unique wargear is spelled as its page spells it (Elven-made Glaive, not the annotation's Elven Glaive)
		for _, u := range c.UniqueWargear {
			add(u.Name)
			if a, ok := b.annotations[strings.ToLower(u.Name)]; ok {
				for _, n := range append([]string{a.Name}, a.Aliases...) {
					if k := strings.ToLower(strings.TrimSpace(n)); k != "" {
						spell[k] = u.Name
					}
				}
			}
		}
	}
	byName := make([]GearAnnotation, 0, len(b.annotations))
	for _, a := range b.annotations {
		byName = append(byName, a)
	}
	slices.SortFunc(byName, func(x, y GearAnnotation) int { return strings.Compare(x.Name, y.Name) })
	for _, a := range byName { // a true synonym that is no armoury name (a vanilla typo, a plural) reads as the annotated item
		for _, al := range a.Aliases {
			if !strings.EqualFold(al, a.Name) {
				if _, seen := spell[strings.ToLower(strings.TrimSpace(al))]; !seen {
					spell[strings.ToLower(strings.TrimSpace(al))] = a.Name
				}
			}
		}
	}
	names := make([]string, 0, len(b.annotations)+len(vanillaNames))
	for _, a := range b.annotations {
		names = append(names, a.Name)
		names = append(names, a.Aliases...)
	}
	names = append(names, vanillaNames...)
	slices.Sort(names) // capitalised spellings sort first, and the choice never depends on map order
	for _, n := range names {
		add(n)
	}
	b.spell = spell
	fix := func(w []string) {
		for i, n := range w {
			if c, ok := spell[strings.ToLower(strings.TrimSpace(n))]; ok {
				w[i] = c
			}
		}
	}
	for _, c := range b.companies { // the rows share their slices with the profiles' copies of the company
		for _, chart := range [][]ChartRow{c.Reinforcements, c.SpecialChart} {
			for _, r := range chart {
				fix(r.Result.Wargear)
				for _, o := range r.Result.Options {
					fix(o.Selected)
				}
			}
		}
	}
	for i := range b.profiles {
		p := &b.profiles[i]
		for _, s := range p.Start {
			fix(s.Wargear)
		}
		for _, c := range p.Composites {
			for _, part := range c.Parts {
				fix(part.Wargear)
			}
		}
	}
	for _, list := range b.advancements {
		for _, a := range list {
			for _, t := range a.To {
				fix(t.Wargear)
			}
		}
	}
}

// SpellWargearLikeVanilla spells the wargear of the charts as the vanilla book does where the armoury has no such item.
func (b *BCBook) SpellWargearLikeVanilla(v *VanillaBook) {
	var names []string
	for n := range v.gear {
		names = append(names, n)
	}
	for _, u := range v.units {
		names = append(names, u.Wargear...)
	}
	b.canonicaliseWargear(names)
	// The profiles' own wargear (Heavy armour / Heavy Armour, Short bow / Short Bow) is spelled the same way, so that a
	// chip reads alike whichever unit it comes from.
	for name, u := range v.units {
		u.Wargear = slices.Clone(u.Wargear)
		for i, w := range u.Wargear {
			if c, ok := b.spell[strings.ToLower(strings.TrimSpace(w))]; ok {
				u.Wargear[i] = c
			}
		}
		v.units[name] = u
	}
}

// unitFillIn gives a vanilla unit the fields it leaves empty (data.unitFillIns): race, faction and unit type drive company limits.
type unitFillIn struct {
	unit                     string
	race, faction, unitTypes []string
}

// ApplyUnitFillIns sets, on the vanilla book, the fields the Battle Companies file fills in where vanilla leaves them empty. A
// field vanilla does define is never changed.
func (b *BCBook) ApplyUnitFillIns(v *VanillaBook) {
	for _, f := range b.fillIns {
		u, ok := v.units[f.unit]
		if !ok {
			continue
		}
		if len(u.Race) == 0 {
			u.Race = slices.Clone(f.race)
		}
		if len(u.Faction) == 0 {
			u.Faction = slices.Clone(f.faction)
		}
		if len(u.UnitTypes) == 0 {
			u.UnitTypes = slices.Clone(f.unitTypes)
		}
		v.units[f.unit] = u
	}
	for _, f := range b.mountFillIns { // the same for the keywords of a mount
		g, ok := v.gear[strings.ToLower(f.unit)]
		if !ok {
			continue
		}
		if len(g.Race) == 0 {
			g.Race = slices.Clone(f.race)
		}
		if len(g.Faction) == 0 {
			g.Faction = slices.Clone(f.faction)
		}
		if len(g.UnitTypes) == 0 {
			g.UnitTypes = slices.Clone(f.unitTypes)
		}
		v.gear[strings.ToLower(f.unit)] = g
	}
	v.collectRaces()
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
