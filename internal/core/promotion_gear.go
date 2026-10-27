package core

import "slices"

// What a promoted warrior carries on (owner rule 2026-10-26). A warrior may only carry what its profile lists, so on a level-up
// promotion he drops the wargear the new profile does not list: an option (under any spelling the armoury knows or
// the armoury knows it by), its own wargear, an option its company label selects, or what the promotion itself gives. Equipment
// (slot items: banners, herbs, cloaks...) always stays. A promotion that makes the unit a hero (Hero in the Making, the hero
// promotion of a path) keeps everything, as does a hero by any other road: dropUnlisted is only called for a warrior's promotion.

// listedNames are the item names a profile lists: every item of every option, its wargear, the options its company label selects,
// what the promotion gives and the armoury items its options are read as.
func listedNames(rs *RuleSet, bcC BCCompany, newBase string, given []string) []string {
	company, _ := rs.BC.Profile(bcC.Name)
	v, alias, ok := resolveUnit(rs, company, newBase)
	if !ok {
		return nil
	}
	var parts []string
	add := func(names ...string) {
		for _, n := range names {
			parts = append(parts, OptionItems(n)...)
		}
	}
	for _, o := range v.Options {
		add(o.Name)
		if item, ok := rs.BC.ReadAs(v.Name, o.Name); ok {
			add(item)
		}
	}
	add(v.Wargear...)
	add(given...)
	if alias != nil {
		add(alias.Selected...)
	}
	return parts
}

// sameSpelling says whether two names are one item: equal once case, punctuation, the plural s and spacing are ignored (ItemKey).
func sameSpelling(a, b string) bool { return ItemKey(a) == ItemKey(b) }

// knownAs are the names an item is known by: its own, the armoury's other names and the annotation's aliases.
func knownAs(rs *RuleSet, bcC BCCompany, g string) []string {
	names := []string{g}
	if offer, sold := rs.BC.Offer(bcC, g); sold {
		names = append(names, offer.Name)
		names = append(names, offer.AlsoCalled...)
	}
	if a, ok := rs.BC.Annotation(g); ok {
		names = append(names, a.Name)
		names = append(names, a.Aliases...)
	}
	return names
}

// namedIn says whether any of the names is one of the listed items.
func namedIn(names, listed []string) bool {
	return slices.ContainsFunc(listed, func(l string) bool {
		return slices.ContainsFunc(names, func(n string) bool { return n != "" && sameSpelling(n, l) })
	})
}

// keepsOnPromotion says whether a warrior promoted to a profile listing `listed` keeps the item g: equipment always, wargear when the
// profile lists it under any name it is known by (its own, the armoury's other names and the annotation's aliases).
func keepsOnPromotion(rs *RuleSet, bcC BCCompany, listed []string, g string) bool {
	if offer, sold := rs.BC.Offer(bcC, g); sold && offer.Size != "" {
		return true
	}
	return namedIn(knownAs(rs, bcC, g), listed)
}

// carriedItems are the items a stored gear entry stands for, each judged on its own. A known item is itself; an option label
// names its items, and an exchange ("Exchange hand-and-a-half weapon for bow") carries only the item it gives: the other was
// exchanged away.
func carriedItems(rs *RuleSet, bcC BCCompany, g string) (items []string, exchange bool) {
	if _, sold := rs.BC.Offer(bcC, g); sold {
		return []string{g}, false
	}
	if _, ok := rs.BC.Annotation(g); ok {
		return []string{g}, false
	}
	if isExchange(g) {
		if parts := optionParts(g); len(parts) > 0 {
			return parts, true
		}
	}
	if parts := OptionItems(g); len(parts) > 1 || (len(parts) == 1 && !sameSpelling(parts[0], g)) {
		return parts, false
	}
	return []string{g}, false
}

// dropUnlisted takes from a promoted warrior the wargear its new profile does not list. The gear is gone for good, as with a
// drop; a hero made of the unit may buy it again from the armoury. A stored option label is read as the items it carries, each
// judged on its own against what the new profile lists as wargear or option; when the new profile has an exchange option that gives
// the carried item, that exchange is already chosen and the entry becomes that option. A weapon modifier (Envenom Weapon) whose
// weapon the new profile no longer lists is a modifier without a weapon again: the player chooses anew.
func dropUnlisted(env *levelEnv, us *UnitState, newBase string, given []string) {
	rs, bcC := env.rs, env.bcC
	listed := listedNames(rs, bcC, newBase, given)
	var options []ChoiceOption
	var wargear []string
	if company, ok := rs.BC.Profile(bcC.Name); ok {
		if v, _, ok := resolveUnit(rs, company, newBase); ok {
			options, wargear = v.Options, v.Wargear
		}
	}
	keep := func(list []string) []string {
		var out []string
		add := func(g string) {
			if !slices.Contains(out, g) {
				out = append(out, g)
			}
		}
		for _, g := range list {
			if base, weapon, ok := rs.BC.ChosenWeapon(g); ok {
				if keepsOnPromotion(rs, bcC, listed, weapon) {
					add(g)
				} else {
					add(base)
				}
				continue
			}
			items, exchange := carriedItems(rs, bcC, g)
			if len(items) == 1 && items[0] == g {
				if keepsOnPromotion(rs, bcC, listed, g) {
					out = append(out, g)
				}
				continue
			}
			for _, it := range items {
				if !keepsOnPromotion(rs, bcC, listed, it) {
					continue
				}
				names := knownAs(rs, bcC, it)
				switch i := slices.IndexFunc(options, func(o ChoiceOption) bool { return isExchange(o.Name) && namedIn(names, optionParts(o.Name)) }); {
				case exchange && i >= 0:
					add(options[i].Name)
				case exchange && namedIn(names, wargear):
					// the profile carries it already
				default:
					add(it)
				}
			}
		}
		return out
	}
	us.Gear, us.Bought = keep(us.Gear), keep(us.Bought)
}
