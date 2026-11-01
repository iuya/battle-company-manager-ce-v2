package core

import (
	"fmt"
	"slices"
	"strings"
)

// What the armoury lists (issue 57): every purchase the company could ever make, whether or not a unit can make it now, and only
// those. "Possible" is decided here, in one place, from the same sources as a hero's options (heroOptions): some unit that can
// exist in the company (the units it has, and the profiles its charts, advancements, promotions and starting company give) could
// buy the item under the rules. What stands in the way only for now (influence, a free slot, a company limit, "already carried",
// the count of a unique item, the roster of the day) does not make an item impossible.

// reachRole says how a profile is reachable in a company.
type reachRole int

const (
	reachRecruit   reachRole = iota // a charts' result, an advancement's target or a starting unit: a warrior can be this
	reachPromotion                  // what a promotion makes of a unit: only a hero is this
)

// reachable calls unit for every profile a unit of the company can be or become and wargear for every piece of wargear an advancement
// or the starting company gives, in the order heroOptions reads them.
func (sh shop) reachable(unit func(name string, role reachRole), wargear func(name string)) {
	for _, chart := range [][]ChartRow{sh.bcC.Reinforcements, sh.bcC.SpecialChart} {
		for _, r := range chart {
			unit(r.Result.Unit, reachRecruit)
			for _, c := range r.Result.Options {
				unit(c.Unit, reachRecruit)
			}
		}
	}
	for _, adv := range sh.rs.BC.Advancements(sh.bcC) { // "including those that could be promoted"
		for _, to := range adv.To {
			unit(to.Unit, reachRecruit)
			for _, w := range to.Wargear {
				wargear(w)
			}
		}
	}
	if profile, ok := sh.rs.BC.Profile(sh.bcC.Name); ok {
		for _, s := range profile.Start {
			unit(s.Unit, reachRecruit)
			for _, w := range s.Wargear {
				wargear(w)
			}
		}
		for _, p := range profile.Promotions {
			unit(p.Becomes, reachPromotion)
		}
	}
}

// candidates are the units that can exist in the company with nothing on them: a warrior of every profile it recruits and a hero of
// every profile a hero can be (a profile a promotion replaces is no hero: the Ranger of Arnor becomes a Ranger of the North). Heroes
// come first.
func (sh shop) candidates() []CompanyUnit {
	var recruit, promoted []string
	sh.reachable(func(name string, role reachRole) {
		list := &recruit
		if role == reachPromotion {
			list = &promoted
		}
		if name != "" && !slices.Contains(*list, name) {
			*list = append(*list, name)
		}
	}, func(string) {})
	var replaced []string
	if profile, ok := sh.rs.BC.Profile(sh.bcC.Name); ok {
		for _, p := range profile.Promotions {
			replaced = append(replaced, p.From)
		}
	}
	var out []CompanyUnit
	add := func(name string, role UnitRole) {
		v, ok := sh.profileOf(name)
		if !ok {
			return
		}
		kind := KindOf(v.UnitTypes)
		if profile, ok := sh.rs.BC.Profile(sh.bcC.Name); ok {
			kind = KindOf(profile.UnitTypes(name, v.UnitTypes))
		}
		out = append(out, CompanyUnit{Type: name, Role: role, Kind: kind})
	}
	for _, name := range recruit {
		if !slices.Contains(replaced, name) {
			add(name, RoleHero)
		}
	}
	for _, name := range promoted {
		add(name, RoleHero)
	}
	for _, name := range recruit {
		add(name, RoleWarrior)
	}
	return out
}

// startStage is the armour stage a profile wears before anything is bought.
func (sh shop) startStage(unit string) int {
	v, ok := sh.profileOf(unit)
	if !ok {
		return 0
	}
	up := sh.rs.BC.ArmourUpgrade()
	best := 0
	for _, w := range v.Wargear {
		if s, ok := up.StageOf(w); ok {
			best = max(best, s)
		}
	}
	return best
}

// armourReaches lists the candidates that could climb to the stage of the upgrade, one stage at a time: they wear less than it now and
// the stages between are ones they may have (a warrior: its profile's options; a hero: what the company's units can have).
func (sh shop) armourReaches(o GearOffer, cands []CompanyUnit) []CompanyUnit {
	var out []CompanyUnit
	for _, c := range cands {
		from := sh.startStage(c.Type)
		if from >= o.ArmourStage {
			continue
		}
		pool := sh.armourPool(c)
		reaches := true
		for stage := from + 1; stage <= o.ArmourStage; stage++ {
			if len(pool[stage]) == 0 {
				reaches = false
				break
			}
		}
		if reaches {
			out = append(out, c)
		}
	}
	return out
}

// takers lists the candidates that could buy the offer: for an armour upgrade those that can climb to it, for anything else those no
// rule rules out, with nothing carried and an empty roster (the other units of the day are no part of whether the item is possible).
func (sh shop) takers(o GearOffer, cands []CompanyUnit) []CompanyUnit {
	if o.ArmourStage > 0 {
		return sh.armourReaches(o, cands)
	}
	fresh := sh
	fresh.d.Roster = nil
	var out []CompanyUnit
	for _, c := range cands {
		if _, blocker := fresh.whyNot(c, o); blocker != BlockedByRule {
			out = append(out, c)
		}
	}
	return out
}

// entries are the items of the armoury, each with who can buy it now and whether it is possible in the company.
func (sh shop) entries() []ArmouryEntry {
	var cands []CompanyUnit
	var out []ArmouryEntry
	for _, offer := range sh.offers() {
		entry := ArmouryEntry{GearOffer: offer, Icon: IconFor(offer.Name, offer.Classes, offer.Size)}
		onlyRules := true // every unit is stopped by a rule, not by influence, a slot or a limit
		for _, u := range sh.d.Roster {
			reason, blocker := sh.whyNot(u, offer)
			opt := UnitOption{ID: u.ID, Title: u.Title(), Type: u.Type, Role: u.Role, Kind: u.Kind, Reason: reason, Blocker: blocker, GoldPrice: sh.goldPrice(u, offer)}
			opt.PayInfluence, opt.PayGold = canPay(sh.d, offer.Cost, opt.GoldPrice)
			entry.Units = append(entry.Units, opt)
			onlyRules = onlyRules && blocker == BlockedByRule
		}
		entry.Possible = len(entry.Units) == 0 || !onlyRules
		if onlyRules && len(entry.Units) > 0 {
			if cands == nil {
				cands = sh.candidates()
			}
			later := sh.takers(offer, cands)
			entry.Possible = len(later) > 0
			if entry.Possible {
				entry.Why = sh.whyNotNow(entry, later)
			}
		} else if !entry.CanBuy() {
			entry.Why = sh.whyNotNow(entry, nil)
		}
		out = append(out, entry)
	}
	return out
}

// maxNamed is how many unit types the "why" sentence names before it says "and others".
const maxNamed = 3

// whyNotNow is the sentence for an item no unit can take now: what stops each kind of unit of the company, then who could take it
// later (later: the candidates that could).
func (sh shop) whyNotNow(e ArmouryEntry, later []CompanyUnit) string {
	type group struct {
		reason string
		types  []string
	}
	var groups []*group
	for _, u := range e.Units {
		if u.Able() {
			continue
		}
		i := slices.IndexFunc(groups, func(g *group) bool { return g.reason == u.Reason })
		if i < 0 {
			groups = append(groups, &group{reason: u.Reason})
			i = len(groups) - 1
		}
		if !slices.Contains(groups[i].types, u.Type) {
			groups[i].types = append(groups[i].types, u.Type)
		}
	}
	var parts []string
	for _, g := range groups {
		parts = append(parts, LowerFirst(g.reason)+" ("+namedTypes(g.types)+")")
	}
	var b strings.Builder
	b.WriteString("No unit can take it now")
	if len(parts) > 0 {
		b.WriteString(": " + strings.Join(parts, "; "))
	}
	b.WriteString(".")
	var names []string
	for _, c := range later {
		if !slices.Contains(names, c.Type) {
			names = append(names, c.Type)
		}
	}
	if len(names) > 0 {
		list := namedTypes(names, "or")
		fmt.Fprintf(&b, " %s %s may take it", article(list), list)
		if e.ArmourStage > 1 && !slices.ContainsFunc(later, func(c CompanyUnit) bool { return sh.startStage(c.Type) >= e.ArmourStage-1 }) {
			if stages := sh.rs.BC.ArmourUpgrade().Stages; e.ArmourStage-1 < len(stages) && len(stages[e.ArmourStage-1]) > 0 {
				b.WriteString(" after " + stages[e.ArmourStage-1][0])
			}
		}
		b.WriteString(".")
	}
	return b.String()
}

// namedTypes lists up to maxNamed names, "a, b and c" (joiner "or" for alternatives), then says there are others.
func namedTypes(names []string, joiner ...string) string {
	word := "and"
	if len(joiner) > 0 {
		word = joiner[0]
	}
	more := len(names) > maxNamed
	if more {
		names = names[:maxNamed]
	}
	switch {
	case more:
		return strings.Join(names, ", ") + " " + word + " others"
	case len(names) > 1:
		return strings.Join(names[:len(names)-1], ", ") + " " + word + " " + names[len(names)-1]
	}
	return strings.Join(names, "")
}

// article is "An" before a vowel, "A" otherwise.
func article(word string) string {
	if word != "" && strings.ContainsRune("AEIOU", rune(word[0])) {
		return "An"
	}
	return "A"
}
