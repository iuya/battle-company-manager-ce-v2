package core

import (
	"slices"
)

// PartialBundle is a row that names one piece of a profile option of several items ("Shield and Spear") and not the others.
type PartialBundle struct {
	Option  string   // the vanilla option
	Points  int      // its points as a bundle
	Named   []string // the wargear the row names
	Missing []string // what the unit gets besides
}

// itemAs describes a wargear name as the identity checks want it.
func itemAs(rs *RuleSet, bcCo BCCompany, name string) GearOffer {
	e, _ := equipmentFor(rs, bcCo, name, false, false, false)
	return GearOffer{Name: name, AlsoCalled: e.Spellings, Classes: e.Classes()}
}

// completeBundle applies the rule that a chart row, a promotion or the starting company which names one piece of a multi-item profile
// option gives the unit the whole option: Iron Hills Warrior (Spear) carries Shield and Spear and is worth
// the option's points as a bundle. It returns the wargear to add (the pieces the unit does not carry already) and what it found.
// An exact single-item option for the wargear wins; options no armoury sells (banners) and exchanges are not bundles to complete; of
// several bundles holding the piece the cheapest is taken.
func completeBundle(rs *RuleSet, bcCo BCCompany, base VanillaUnit, gear, carried []string) (added []string, found *PartialBundle) {
	if len(gear) == 0 {
		return nil, nil
	}
	have := append(slices.Clone(carried), gear...)
	have = append(have, base.Wargear...)
	items := make([]GearOffer, len(have))
	for i, h := range have {
		items[i] = itemAs(rs, bcCo, h)
	}
	named := func(part string) bool {
		return slices.ContainsFunc(items, func(o GearOffer) bool { return GearMatchesOption(o, part) })
	}
	best := -1
	for i, o := range base.Options {
		parts := OptionItems(o.Name)
		if len(parts) < 2 || ParseOption(o.Name).Exchange {
			continue
		}
		if _, unsold := rs.BC.Unsold(base.Name, o.Name); unsold || rs.BC.Reserved(base.Name, o.Name) {
			continue
		}
		fromGear := false
		for _, g := range gear {
			offer := itemAs(rs, bcCo, g)
			for _, part := range parts {
				fromGear = fromGear || GearMatchesOption(offer, part)
			}
		}
		if !fromGear {
			continue
		}
		if best < 0 || o.Points < base.Options[best].Points {
			best = i
		}
	}
	if best < 0 {
		return nil, nil
	}
	// a single-item option that is exactly the named wargear wins (a Bow is a Bow, not a bundle with a Bow in it)
	for _, o := range base.Options {
		if parts := OptionItems(o.Name); len(parts) == 1 && !ParseOption(o.Name).Exchange {
			for _, g := range gear {
				if GearMatchesOption(itemAs(rs, bcCo, g), parts[0]) {
					return nil, nil
				}
			}
		}
	}
	opt := base.Options[best]
	pb := &PartialBundle{Option: opt.Name, Points: opt.Points, Named: slices.Clone(gear)}
	for _, part := range OptionItems(opt.Name) {
		if !named(part) {
			added = append(added, part)
			pb.Missing = append(pb.Missing, part)
		}
	}
	if len(added) == 0 {
		return nil, nil
	}
	return added, pb
}
