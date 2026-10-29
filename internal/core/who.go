package core

import (
	"slices"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// The one selector of the data for "which models" is lean.Who; these build the Subject it tests (docs/definitions.md, "Who").

// subjectOf is a model of a company as lean.Who sees it: the names it goes by, whether it is a hero, the races, factions and unit
// types of its profile (a company's own name for a profile counts as that profile), Cavalry for a model that rides, and the alignment
// of the company.
func subjectOf(rs *RuleSet, profile CompanyProfile, alignment string, u CompanyUnit) lean.Subject {
	v, _, known := rs.ResolveUnit(profile, u.Type)
	s := vanillaSubject(v, known)
	s.Names = []string{u.Type}
	if known {
		s.Names = append(s.Names, v.Name)
	}
	s.Hero, s.Alignment = u.IsHero(), alignment
	if u.isCavalry() && !slices.Contains(s.Keywords, KeywordCavalry) {
		s.Keywords = append(s.Keywords, KeywordCavalry)
	}
	return s
}

// vanillaSubject is a vanilla profile as lean.Who sees it (hero by its unit types).
func vanillaSubject(v VanillaUnit, known bool) lean.Subject {
	s := lean.Subject{Names: []string{v.Name}, Known: known, Race: v.Race, Hero: slices.Contains(v.UnitTypes, "Hero")}
	s.Keywords = slices.Concat(v.Race, v.Faction, v.UnitTypes)
	return s
}
