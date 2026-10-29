package core

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// The gear of a lean document read as the typed values the engine uses today (docs/definitions.md).
// They are pure functions of the document and the vanilla book.

// LeanGearTable is everything the armoury, the cards and the glossary read about items.
type LeanGearTable struct {
	Offers      []GearOffer               // what every company's armoury sells, in the order of gear[]
	Local       map[string][]GearOffer    // a company's own unique wargear
	Annotations map[string]GearAnnotation // by lower-cased name and alias, shared gear first, the first wins
	ArmourUp    ArmourUpgrade
	ItemEffects map[string]string // lower-cased item name -> the aftermath effect it activates
	Creatures   []CreatureOffer
	Wanderers   []WandererOffer
}

// Armoury is what a company's heroes and warriors can buy: the common items and the company's own unique wargear.
func (t LeanGearTable) Armoury(company string) []GearOffer {
	return slices.Concat(t.Offers, t.Local[company])
}

// LeanClasses are the classes of an item: the ones it names, else the one its type word says (the type of the entry, else the type of
// the vanilla item of the same name), else "other".
func LeanClasses(g lean.Gear, vanilla *VanillaBook) []EquipmentClass {
	if len(g.Classes) > 0 {
		out := make([]EquipmentClass, len(g.Classes))
		for i, c := range g.Classes {
			out[i] = EquipmentClass(c)
		}
		return out
	}
	typ := g.Type
	if typ == "" && vanilla != nil {
		if vg, ok := vanilla.Gear(g.Name); ok {
			typ = vg.Type
		}
	}
	if cs := ClassesOfVanillaType(typ); cs != nil {
		return cs
	}
	return []EquipmentClass{EquipOther}
}

// leanText is what an item says: its own definition, else the vanilla text of the item of that name; whitespace folded, as the
// annotations read it.
func leanText(g lean.Gear, vanilla *VanillaBook) string {
	if g.Definition != "" {
		return tidy(g.Definition)
	}
	if vanilla != nil {
		if vg, ok := vanilla.Gear(g.Name); ok {
			return tidy(vg.Definition)
		}
	}
	return ""
}

// LeanAnnotation is what the file says about one item as the engine reads it.
func LeanAnnotation(g lean.Gear, vanilla *VanillaBook, shared bool) GearAnnotation {
	a := GearAnnotation{Name: g.Name, Classes: LeanClasses(g, vanilla), Aliases: g.Aka, Grants: g.Grants, Definition: leanText(g, vanilla), Base: g.Base,
		Modifiers: g.Modifiers, PricedAs: g.PricedAs, Related: g.Related, RequiresMount: g.RequiresMount}
	if slices.Contains(a.Classes, EquipMount) {
		if len(g.Grants) > 0 {
			a.GrantedTo = "mount" // a mount's rules belong to the mount
		}
		a.MountProfile = g.Mount
		if a.MountProfile == "" {
			a.MountProfile = g.Name
		}
	}
	if s := g.GrantsSlots; s != nil {
		a.GrantsSlots = SlotCounts{Large: s.Large, Small: s.Small}
	}
	if c := g.GrantsForChosenWeapon; c != nil {
		a.WeaponModifier = true
		a.ChosenWeapon = &ChosenWeaponRule{Rule: c.Rule}
		for _, cl := range c.WeaponClasses {
			a.ChosenWeapon.WeaponClasses = append(a.ChosenWeapon.WeaponClasses, EquipmentClass(cl))
		}
	}
	for _, c := range g.StatChanges {
		a.StatChanges = append(a.StatChanges, StatChange{Stat: c.Stat, Delta: c.Amount, Condition: strings.TrimSpace(c.Condition)})
	}
	if ab := g.Abilities; ab != nil {
		if d := ab.DefenceBonus; d != nil {
			db := &DefenceBonus{Value: d.Value, Conditional: d.Conditional, Condition: strings.TrimSpace(d.Condition), NotWhileEngaged: d.NotWhileEngaged, Against: d.Against,
				CancelledByItems: d.CancelledByItems, TwoHandedUse: d.TwoHandedUse}
			for _, c := range d.CancelledByClasses {
				db.CancelledByClasses = append(db.CancelledByClasses, EquipmentClass(c))
			}
			for _, l := range d.LiftedByRules {
				db.Lifted = append(db.Lifted, DefenceLift{Rule: l.Rule, Cancels: l.Cancels, WhileMounted: l.WhileMounted})
			}
			a.Defence = db
		}
		if b := ab.AttacksBonus; b != nil {
			a.Situational = append(a.Situational, StatChange{Stat: "attacks", Delta: b.Value, Condition: strings.TrimSpace(b.Condition)})
		}
	}
	if r := g.Ranged; r != nil {
		a.Range, a.Strength = r.Range, r.Strength
	}
	return a
}

// leanBadges are the words the armoury page shows for who may take an item: those of its who (hero, cavalry, evil or good) and
// unique for an item of which the company has one. They are display only: Who.Admits decides.
func leanBadges(g lean.Gear) []string {
	out := g.Who.Badges()
	if g.Limit == 1 {
		out = append(out, "unique")
	}
	return out
}

// leanRating is a rating of the file as the engine's Rating.
func leanRating(r lean.Rating) Rating { return Rating{r.Warrior, r.Hero} }

// leanGold is what an item costs in Gold: its own gold, else its rating.
func leanGold(g lean.Gear) Rating {
	if g.Gold != nil {
		return leanRating(*g.Gold)
	}
	return leanRating(g.Rating)
}

// LeanGear reads the items of a lean document: the shared armoury (wargear and equipment that are sold), each company's unique
// wargear, the annotation of every item, the armour upgrade, the consumables of the aftermath, the creatures and the wanderers.
func LeanGear(d *lean.Document, vanilla *VanillaBook) LeanGearTable {
	t := LeanGearTable{Local: map[string][]GearOffer{}, Annotations: map[string]GearAnnotation{}, ItemEffects: map[string]string{}}
	annotate := func(g lean.Gear, shared bool) GearAnnotation {
		a := LeanAnnotation(g, vanilla, shared)
		for _, n := range append([]string{g.Name}, g.Aka...) {
			if _, seen := t.Annotations[NameKey(n)]; !seen {
				t.Annotations[NameKey(n)] = a
			}
		}
		return a
	}
	offer := func(g lean.Gear, a GearAnnotation) GearOffer {
		o := GearOffer{Name: g.Name, AlsoCalled: g.Aka, Cost: g.Influence, Size: g.Size, Restrictions: leanBadges(g), Who: g.Who, Rating: leanRating(g.Rating), Gold: leanGold(g),
			Description: a.Definition, Grants: g.Grants, Classes: a.Classes, Stats: a.Stats(), GrantedTo: a.GrantedTo, WeaponModifier: a.WeaponModifier}
		if g.PricedAs != "" && g.Influence == 0 && g.Rating == (lean.Rating{}) { // Elven-made weaponry: its generic equivalent's price, size and limits
			if at := slices.IndexFunc(t.Offers, func(b GearOffer) bool { return namesItem(b.Name, b.AlsoCalled, g.PricedAs) }); at >= 0 {
				b := t.Offers[at]
				o.Cost, o.Size, o.Restrictions, o.Rating, o.Gold = b.Cost, b.Size, b.Restrictions, b.Rating, b.Gold
				if o.Who == nil {
					o.Who = b.Who
				}
			}
		}
		return o
	}
	for _, g := range d.Gear {
		if onlyTTS(g) { // the Tabletop Simulator figure of a vanilla item (tts_lean.go): not a statement about the item
			continue
		}
		a := annotate(g, true)
		if g.AftermathEffect != "" {
			t.ItemEffects[NameKey(g.Name)] = g.AftermathEffect
		}
		switch g.Kind {
		case "wargear", "equipment":
			if !g.Unsold {
				t.Offers = append(t.Offers, offer(g, a))
			}
		case "creature":
			t.Creatures = append(t.Creatures, creatureFrom(followerItem(g), vanilla))
		case "wanderer":
			t.Wanderers = append(t.Wanderers, wandererFrom(followerItem(g), vanilla))
		}
	}
	for _, c := range d.Companies {
		for _, g := range c.Gear {
			a := annotate(g, false)
			o := offer(g, a)
			o.Exclusive, o.Limit = true, g.Limit
			o.Restrictions = nil
			t.Local[c.Name] = append(t.Local[c.Name], o)
		}
	}
	if up := d.ArmourUp; up != nil {
		t.ArmourUp = ArmourUpgrade{Stages: up.Stages, Cost: up.Influence, Rating: leanRating(up.Rating)}
		if up.Gold != nil {
			t.ArmourUp.Gold = leanRating(*up.Gold)
		}
	}
	return t
}

// onlyTTS says that an entry adds nothing but a Tabletop Simulator figure to the vanilla item of its name (the loader fills in the id and
// a zero price, which say nothing).
func onlyTTS(g lean.Gear) bool {
	if g.TTS == nil || len(g.Aka) > 0 || len(g.Classes) > 0 || g.Rating != (lean.Rating{}) || (g.Gold != nil && *g.Gold != (lean.Rating{})) {
		return false
	}
	h := g
	h.Name, h.ID, h.TTS, h.Gold, h.Aka, h.Classes = "", "", nil, nil, nil, nil
	return reflect.ValueOf(h).IsZero()
}

// followerItem is a creature or a wanderer of gear[] in the shape of a vanilla item, so that one reader (creatureFrom, wandererFrom)
// reads both: its profile, with the name, the influence and the rating (the points) of the entry.
func followerItem(g lean.Gear) map[string]json.RawMessage {
	raw := map[string]json.RawMessage{}
	if m, ok := g.Profile.(map[string]any); ok {
		for k, v := range m {
			raw[k], _ = json.Marshal(v)
		}
	}
	raw["name"], _ = json.Marshal(g.Name)
	raw["influence"], _ = json.Marshal(g.Influence)
	raw["points"], _ = json.Marshal(g.Rating.Warrior)
	raw["gold"], _ = json.Marshal(leanGold(g).Warrior)
	return raw
}

// LeanFollowerRules are the rules of creatures and wanderers: the roles that may have a creature, how many each hero may, and how
// many wanderers at once.
func LeanFollowerRules(d *lean.Document) (CreatureRules, WandererRules) {
	var cr CreatureRules
	var wr WandererRules
	if c := d.Creatures; c != nil {
		cr = CreatureRules{AttachTo: c.Rules.AttachTo, MaxPerHero: c.Rules.MaxPerHero}
	}
	if w := d.Wanderers; w != nil {
		wr = WandererRules{MaxAtOnce: w.Rules.MaxAtOnce}
	}
	return cr, wr
}

// LeanLimitRules are the company limits of the file.
func LeanLimitRules(limits []lean.Limit) []LimitRule {
	var out []LimitRule
	for _, l := range limits {
		r := LimitRule{ID: l.ID, Label: limitLabel(l.Label), Percent: l.MaximumPercentage, RoundUp: l.RoundUp}
		if l.Type == "keyword" {
			r.Keyword = l.Value
		} else {
			for _, c := range l.Classes {
				r.Classes = append(r.Classes, EquipmentClass(c))
			}
		}
		out = append(out, r)
	}
	return out
}

// LeanGearTerms are the glossary terms the gear of a lean document makes (the armoury's price list under each item, and a creature's
// or wanderer's profile), in the order of gear[]. A term for wargear is weak: the rulebook's own entry wins.
func LeanGearTerms(d *lean.Document, vanilla *VanillaBook) []ExtraTerm {
	const src = "Battle Companies"
	t := LeanGear(d, vanilla)
	var out []ExtraTerm
	sold := 0
	for _, g := range d.Gear {
		switch g.Kind {
		case "wargear", "equipment":
			if g.Unsold {
				continue
			}
			o := t.Offers[sold]
			sold++
			if text, ok := gearTermText(o.Description, o.Cost, o.Gold, o.Size); ok {
				term := ExtraTerm{Name: o.Name, Category: g.Kind, Aliases: o.AlsoCalled, Definition: text, Source: src, Weak: g.Kind == "wargear"}
				out = append(out, term)
			}
		case "creature":
			out = append(out, ExtraTerm{Name: g.Name, Category: "creatures", Definition: strings.TrimSpace(creatureTermText(followerItem(g))), Source: src})
		case "wanderer":
			out = append(out, ExtraTerm{Name: g.Name, Category: "wanderers", Definition: strings.TrimSpace(wandererTermText(followerItem(g))), Source: src})
		}
	}
	return out
}
