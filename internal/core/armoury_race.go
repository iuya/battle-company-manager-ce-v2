package core

import (
	"slices"
	"strings"
)

// elvesOnly applies the Battle Companies armoury rule "Elven-made weaponry uses their generic equivalent values, but are
// restricted to Elf models unless otherwise noted": an item that is the elven version of a generic one (Elven spear) is for
// models of the Elf race. It says why the unit cannot take the item; empty when it can (a unit whose profile is unknown is not restricted).
func (sh shop) elvesOnly(u CompanyUnit, o GearOffer) string {
	a, ok := sh.rs.BC.Annotation(o.Name)
	if !ok || a.OnlyRace == "" || o.Exclusive {
		return ""
	}
	v, known := sh.profileOf(u.Type)
	if !known || slices.ContainsFunc(v.Race, func(r string) bool { return strings.EqualFold(r, a.OnlyRace) }) {
		return ""
	}
	return "Elven-made weaponry is for " + racePlural(a.OnlyRace)
}

// racePlural is the race word as the players say it for a group ("Elf" is "Elves", "Dwarf" "Dwarves", "Goblin" "Goblins").
func racePlural(race string) string {
	if strings.HasSuffix(race, "f") {
		return strings.TrimSuffix(race, "f") + "ves"
	}
	return race + "s"
}
