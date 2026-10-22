package core

import "slices"

// elvesOnly applies the Battle Companies armoury rule "Elven-made weaponry uses their generic equivalent values, but are
// restricted to Elf models unless otherwise noted": an item that is the elven version of a generic one (Elven spear) is for
// models of the Elf race. It says why the unit cannot take the item; empty when it can (a unit whose profile is unknown is not restricted).
func (sh shop) elvesOnly(u CompanyUnit, o GearOffer) string {
	a, ok := sh.rs.BC.Annotation(o.Name)
	if !ok || !slices.Contains(a.Modifiers, "elven") || o.Exclusive {
		return ""
	}
	v, known := sh.profileOf(u.Type)
	if !known || slices.Contains(v.Race, "Elf") {
		return ""
	}
	return "Elven-made weaponry is for Elves"
}
