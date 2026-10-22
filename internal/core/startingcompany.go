package core

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// StartHero is a starting unit the player made the leader or a sergeant of the company. It chooses its path later, from the
// company page (like a hero whose promotion was left half done), before the company's next match.
type StartHero struct {
	Index int // the place of the unit in the expanded starting company (StartList)
	Role  UnitRole
}

// StartList is the starting company with one entry per model (a unit with a count of two is two entries).
func (p CompanyProfile) StartList() []StartUnit {
	var out []StartUnit
	for _, s := range p.Start {
		n := s.Count
		s.Count = 1
		for i := 0; i < n; i++ {
			out = append(out, s)
		}
	}
	return out
}

// CanLead says whether a starting unit may be made a hero: the company may allow only some units, or exclude some.
// Units On Their Own Path (Cave Troll, Crebain...) never lead: they earn no experience, so they cannot be officers or Heroes in the
// Making; the homebrew "Path of Their Own Making" makes them heroes of their own path when the company is built (buildUnit).
func (p CompanyProfile) CanLead(unit string) bool {
	if p.OnOwnPath(unit) {
		return false
	}
	has := func(names []string) bool {
		return slices.ContainsFunc(names, func(n string) bool { return strings.Contains(strings.ToLower(unit), strings.ToLower(n)) })
	}
	if len(p.StartHeroes.Allowed) > 0 && !has(p.StartHeroes.Allowed) {
		return false
	}
	return !has(p.StartHeroes.Excluded)
}

// StartGains says whether a starting hero gets the usual point of Might, Will and Fate (some units already have them).
func (p CompanyProfile) StartGains(unit string) bool {
	return !slices.ContainsFunc(p.NoStartGain, func(n string) bool { return strings.EqualFold(n, unit) })
}

// DefaultHeroes is the leader and the two sergeants the company suggests: the roles its file gives, completed with the first
// units that can lead.
func (p CompanyProfile) DefaultHeroes() []StartHero {
	list := p.StartList()
	var out []StartHero
	taken := map[int]bool{}
	want := map[UnitRole]int{RoleLeader: 1, RoleSergeant: 2}
	for i, s := range list {
		if (s.Role == RoleLeader || s.Role == RoleSergeant) && want[s.Role] > 0 && p.CanLead(s.Unit) {
			want[s.Role]--
			taken[i] = true
			out = append(out, StartHero{Index: i, Role: s.Role})
		}
	}
	for _, role := range []UnitRole{RoleLeader, RoleSergeant} {
		for i, s := range list {
			if want[role] == 0 {
				break
			}
			if !taken[i] && p.CanLead(s.Unit) {
				want[role]--
				taken[i] = true
				out = append(out, StartHero{Index: i, Role: role})
			}
		}
	}
	slices.SortFunc(out, func(a, b StartHero) int { return a.Index - b.Index })
	return out
}

// fixedLeader is the place of the starting unit the rules make the Leader: the Shire's Ranger of the North (the company's
// replacement unit, which its file lists as the leader) starts as the Leader and cannot be demoted or left out of the officers.
func (p CompanyProfile) fixedLeader(list []StartUnit) (int, bool) {
	if p.RangerReplacement == nil {
		return 0, false
	}
	for i, s := range list {
		if s.Role == RoleLeader && strings.EqualFold(s.Unit, p.RangerReplacement.Unit) {
			return i, true
		}
	}
	return 0, false
}

func (p CompanyProfile) fixedLeaderMessage(unit string) string {
	return "The " + unit + " must start as the Leader of the " + p.Name + " company."
}

// ValidateStart checks the player's choice of leader and sergeants against the company; with no choice it gives the company's
// suggestion. The result is sorted by place.
func (p CompanyProfile) ValidateStart(heroes []StartHero) ([]StartHero, error) {
	if len(heroes) == 0 {
		heroes = p.DefaultHeroes()
	}
	list := p.StartList()
	fail := func(format string, a ...any) ([]StartHero, error) {
		return nil, errors.Join(ErrInvalidCompany, fmt.Errorf(format, a...))
	}
	var leaders, sergeants int
	seen := map[int]bool{}
	for _, h := range heroes {
		switch {
		case h.Index < 0 || h.Index >= len(list):
			return fail("that unit is not in the starting company")
		case seen[h.Index]:
			return fail("a unit cannot be two officers")
		case !p.CanLead(list[h.Index].Unit):
			return fail("%s cannot be an officer of the %s company", list[h.Index].Unit, p.Name)
		}
		seen[h.Index] = true
		switch h.Role {
		case RoleLeader:
			leaders++
		case RoleSergeant:
			sergeants++
		default:
			return fail("a starting officer is the leader or a sergeant")
		}
	}
	if leaders != 1 || sergeants != 2 {
		return fail("choose one leader and two sergeants")
	}
	if i, ok := p.fixedLeader(list); ok {
		if !slices.ContainsFunc(heroes, func(h StartHero) bool { return h.Index == i && h.Role == RoleLeader }) {
			return fail("%s", p.fixedLeaderMessage(list[i].Unit))
		}
	}
	out := slices.Clone(heroes)
	slices.SortFunc(out, func(a, b StartHero) int { return a.Index - b.Index })
	return out, nil
}
