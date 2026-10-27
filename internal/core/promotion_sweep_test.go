package core

import (
	"slices"
	"sort"
	"strings"
	"testing"
)

// gearOfUnit lists every way a warrior of a company's unit comes to carry an item (the strings the company state stores).
func gearOfUnit(rs *RuleSet, c BCCompany, from string) map[string]string {
	company, _ := rs.BC.Profile(c.Name)
	out := map[string]string{}
	put := func(item, source string) {
		if item = strings.TrimSpace(item); item != "" {
			if _, dup := out[item]; !dup {
				out[item] = source
			}
		}
	}
	v, alias, ok := resolveUnit(rs, company, from)
	if !ok {
		return out
	}
	for _, o := range rs.BC.ChoosableOptions(v.Name, v.Options) {
		for _, part := range strings.Split(o.Name, " and ") {
			put(part, "option chosen on recruit")
		}
	}
	for _, o := range v.Options {
		for _, offer := range rs.BC.Armoury(c) {
			if offer.Size == "" && GearMatchesOption(offer, o.Name) {
				put(offer.Name, "bought")
			}
		}
	}
	if alias != nil {
		for _, s := range alias.Selected {
			put(s, "alias option")
		}
	}
	for _, rows := range [][]ChartRow{c.Reinforcements, c.SpecialChart} {
		for _, r := range rows {
			if r.Result.Unit == from {
				for _, g := range r.Result.Wargear {
					put(g, "chart row wargear")
				}
			}
		}
	}
	for _, s := range company.Start {
		if s.Unit == from {
			for _, g := range s.Wargear {
				put(g, "starting company")
			}
		}
	}
	for _, a := range rs.BC.Advancements(c) {
		for _, t := range a.To {
			if t.Unit == from || t.Label == from {
				for _, g := range t.Wargear {
					put(g, "earlier promotion")
				}
			}
		}
	}
	return out
}

// TestEveryPromotionAppliesTheOneRule runs every promotion of every company (all the gear a warrior could carry from the old profile)
// and checks the one rule, with no list of approved drops: equipment stays; every carried item is judged on its own and stays exactly
// when the new profile lists it (as wargear, in an option, or as what an exchange option gives) under a name the armoury recognises as
// the same item. An option label stands for the items it carries (an exchange carries the item it gives, the other was exchanged
// away); a carried item that is listed stays either as itself, as the exchange option of the new profile that gives it (that
// exchange is already chosen), or as the new profile's own wargear. Nothing else is kept, and nothing panics.
func TestEveryPromotionAppliesTheOneRule(t *testing.T) {
	rs := shippedRules(t)
	rows := 0
	for _, n := range sortedCompanies(rs) {
		c := rs.BC.companies[n]
		company, _ := rs.BC.Profile(c.Name)
		for _, a := range rs.BC.Advancements(c) {
			var items []string
			for g := range gearOfUnit(rs, c, a.FromUnit) {
				items = append(items, g)
			}
			for _, to := range a.To {
				base := promotedName(rs, c, to)
				listed := listedNames(rs, c, base, to.Wargear)
				target, _, _ := resolveUnit(rs, company, base)
				us := UnitState{Base: a.FromUnit, Role: RoleWarrior, Gear: slices.Clone(items), Bought: slices.Clone(items)}
				dropUnlisted(&levelEnv{rs: rs, bcC: c}, &us, base, to.Wargear)
				// held says whether the unit carries the item after the promotion, in what it stores or in the profile it now has
				held := func(stored []string, it string, viaProfile bool) bool {
					for _, g := range stored {
						if SameItem(g, nil, it, nil) || sameSpelling(g, it) {
							return true
						}
						if isExchange(g) && slices.ContainsFunc(optionParts(g), func(p string) bool { return sameSpelling(p, it) }) {
							return true
						}
					}
					return viaProfile && slices.ContainsFunc(target.Wargear, func(w string) bool { return sameSpelling(w, it) })
				}
				for _, g := range items {
					rows++
					carried, _ := carriedItems(rs, c, g)
					for _, it := range carried {
						want := false
						if offer, sold := rs.BC.Offer(c, it); sold && offer.Size != "" {
							want = true
						}
						for _, l := range listed {
							if SameItem(it, nil, l, nil) || sameSpelling(it, l) {
								want = true
							}
							if offer, sold := rs.BC.Offer(c, it); sold && (GearMatchesOption(offer, l) || SameItem(offer.Name, offer.AlsoCalled, l, nil)) {
								want = true
							}
						}
						single := len(carried) == 1 && carried[0] == g
						for kind, stored := range map[string][]string{"gear": us.Gear, "bought": us.Bought} {
							got := held(stored, it, !single)
							if single {
								got = slices.Contains(stored, g)
							}
							if got != want {
								t.Errorf("%s: %s -> %s: %s %q (item %q) kept=%v, the rule says %v", n, a.FromUnit, base, kind, g, it, got, want)
							}
						}
					}
				}
				for _, stored := range [][]string{us.Gear, us.Bought} {
					for _, g := range stored {
						if !slices.Contains(items, g) && !slices.ContainsFunc(target.Options, func(o ChoiceOption) bool { return o.Name == g }) && !slices.ContainsFunc(items, func(x string) bool {
							parts, _ := carriedItems(rs, c, x)
							return slices.ContainsFunc(parts, func(p string) bool { return p == g })
						}) {
							t.Errorf("%s: %s -> %s: %q was invented by the promotion", n, a.FromUnit, base, g)
						}
					}
				}
			}
		}
	}
	if rows == 0 {
		t.Fatal("no promotion swept")
	}
}

func sortedCompanies(rs *RuleSet) []string {
	var names []string
	for n := range rs.BC.companies {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
