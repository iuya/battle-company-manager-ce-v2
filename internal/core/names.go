package core

import (
	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// Names of the rules data. The lean format (docs/definitions.md, "Normalisation: `extends`, defaults, names") gives every unit, item, rule,
// path and company one stable `name` plus `aka` for its other spellings, resolved once into an index; nothing is fuzzy. The app resolves a name in these places only:
//
//   - sameName says whether two names of the data are the same name (every site that compared two names asks it);
//   - CompanyProfile.AliasFor finds a company's own name for a unit (its label);
//   - RuleSet.ResolveUnit finds the vanilla profile a unit name stands for in a company (a vanilla name, else a label);
//   - BCBook.Company finds a company by its name, whatever the case it was stored in (the aka of a company);
//   - ItemKey / namesItem / SameItem (gearidentity.go) stay the fuzzy bridge to the vanilla file's own spellings of items (plural s, "made", "&").
//
// The lean index (NameKey, aka, the entity's own scope first) resolves a unit to the same vanilla unit as ResolveUnit.

// sameName says whether two names of the data are the same name: equal when case is ignored. The lean format compares by lean.NameKey
// (case and punctuation) and `aka`; the two agree on every name of the shipped data.
func sameName(a, b string) bool { return NameKey(a) == NameKey(b) }

// NameKey is the one key of a name in a map or a set: lower case, accents and apostrophes dropped, runs of spaces and punctuation one
// space (lean.NameKey). The maps keyed by a name of the rules data use it on both sides, so "Lothlorien" finds "Lothlórien" and
// "Lake-town" finds "lake town". Gear identity stays ItemKey (plural s, "made").
func NameKey(s string) string { return lean.NameKey(s) }

// aliasesFor lists the aliases whose label is label, in the order of the profile (a label is unique in a file; the lean format makes it an error).
func (p CompanyProfile) aliasesFor(label string) func(yield func(UnitAlias) bool) {
	return func(yield func(UnitAlias) bool) {
		for _, a := range p.Aliases {
			if sameName(a.Label, label) && !yield(a) {
				return
			}
		}
	}
}

// AliasFor finds the company's own name for a unit: the alias whose label is label (a company's "Ranger of Ithilien", "Grimhammer").
func (p CompanyProfile) AliasFor(label string) (UnitAlias, bool) {
	for a := range p.aliasesFor(label) {
		return a, true
	}
	return UnitAlias{}, false
}

// ResolveUnit finds the vanilla profile a unit name stands for in a company, as buildUnit does: a vanilla name, else the label of one of
// the company's aliases (the alias is returned with it).
func (r *RuleSet) ResolveUnit(p CompanyProfile, name string) (VanillaUnit, *UnitAlias, bool) {
	if v, ok := r.Vanilla.Unit(name); ok {
		return v, nil, true
	}
	for a := range p.aliasesFor(name) {
		if v, ok := r.Vanilla.Unit(a.Unit); ok {
			return v, &a, true
		}
	}
	return VanillaUnit{}, nil, false
}
