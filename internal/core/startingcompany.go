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

// RoleHeroInTheMaking is the role a company's file gives a starting unit that is already a Hero in the Making (Wanderers in the
// Wild): it starts as a hero with a point of Might, Will and Fate and its path to choose, and is not one of the officers the player picks.
const RoleHeroInTheMaking UnitRole = "heroInTheMaking"

// CanLeadStart says whether a unit of the starting company may be one of the officers the player picks.
func (p CompanyProfile) CanLeadStart(s StartUnit) bool {
	return s.Role != RoleHeroInTheMaking && p.CanLead(s.Unit)
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
		if (s.Role == RoleLeader || s.Role == RoleSergeant) && want[s.Role] > 0 && p.CanLeadStart(s) {
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
			if !taken[i] && p.CanLeadStart(s) {
				want[role]--
				taken[i] = true
				out = append(out, StartHero{Index: i, Role: role})
			}
		}
	}
	slices.SortFunc(out, func(a, b StartHero) int { return a.Index - b.Index })
	return out
}

// heroesByProfile are the places of the starting units that already have their own Might, Will and Fate (a Ranger of the North:
// startingHeroNoStatGain) and may lead: they are officers, or the company would be worth 15 points more than the rules say.
func (p CompanyProfile) heroesByProfile() []int {
	var out []int
	for i, s := range p.StartList() {
		if !p.StartGains(s.Unit) && p.CanLeadStart(s) {
			out = append(out, i)
		}
	}
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
	return "The " + unit + " must start as the Leader of " + theCompany(p.Name) + "."
}

func (p CompanyProfile) heroByProfileMessage(unit string) string {
	return unit + " must be one of the officers of " + theCompany(p.Name) + " (it is already a hero)."
}

// OfficerLockKind says how the rules bind a starting unit in the choice of officers.
type OfficerLockKind string

const (
	OfficerFree   OfficerLockKind = ""        // the player decides
	OfficerLeader OfficerLockKind = "leader"  // must start as the Leader
	OfficerAny    OfficerLockKind = "officer" // must be an officer, Leader or Sergeant
	OfficerBarred OfficerLockKind = "barred"  // cannot be an officer
)

// OfficerLock is what the rules say about one starting unit in the choice of officers, with the reason in words. It is the single
// source of truth of the creation page and of ValidateStart: the page blocks what this forbids, the server refuses it.
type OfficerLock struct {
	Kind   OfficerLockKind
	Reason string // a short sentence for the card ("Locked: ..."); empty when the unit is free
}

// OfficerLocks gives, for every place of the starting company (StartList), how the rules bind the unit in the choice of officers.
func (p CompanyProfile) OfficerLocks() []OfficerLock {
	list := p.StartList()
	out := make([]OfficerLock, len(list))
	leader, hasLeader := p.fixedLeader(list)
	for i, s := range list {
		switch {
		case s.Role == RoleHeroInTheMaking:
			out[i] = OfficerLock{OfficerBarred, s.Unit + " starts as a Hero in the Making with a point of Might, Will and Fate; it chooses its path after the company is made."}
		case p.OnOwnPath(s.Unit):
			out[i] = OfficerLock{OfficerBarred, s.Unit + " is On Their Own Path: it earns no experience, so it cannot be an officer."}
		case !p.CanLeadStart(s):
			out[i] = OfficerLock{OfficerBarred, "The " + p.Name + " company cannot make " + s.Unit + " an officer."}
		case hasLeader && i == leader:
			out[i] = OfficerLock{OfficerLeader, "The rules make " + s.Unit + " the Leader of " + theCompany(p.Name) + "."}
		case !p.StartGains(s.Unit):
			out[i] = OfficerLock{OfficerAny, s.Unit + " is already a hero (it has its own Might, Will and Fate), so it is one of the officers: Leader or Sergeant."}
		}
	}
	return out
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
	locks := p.OfficerLocks()
	var leaders, sergeants int
	seen := map[int]bool{}
	for _, h := range heroes {
		switch {
		case h.Index < 0 || h.Index >= len(list):
			return fail("that unit is not in the starting company")
		case seen[h.Index]:
			return fail("a unit cannot be two officers")
		case locks[h.Index].Kind == OfficerBarred:
			return fail("%s cannot be an officer of %s", list[h.Index].Unit, theCompany(p.Name))
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
	for i, l := range locks {
		leads := slices.ContainsFunc(heroes, func(h StartHero) bool { return h.Index == i && h.Role == RoleLeader })
		officer := slices.ContainsFunc(heroes, func(h StartHero) bool { return h.Index == i })
		switch {
		case l.Kind == OfficerLeader && !leads:
			return fail("%s", p.fixedLeaderMessage(list[i].Unit))
		case l.Kind == OfficerAny && !officer: // a hero by its profile is worth its Might, Will and Fate as a warrior too
			return fail("%s", p.heroByProfileMessage(list[i].Unit))
		}
	}
	out := slices.Clone(heroes)
	slices.SortFunc(out, func(a, b StartHero) int { return a.Index - b.Index })
	return out, nil
}

// theCompany writes "the Rohan company" or "the Shire company" without doubling the article when the company's name
// already starts with "The" (The Shire).
func theCompany(name string) string {
	n := strings.TrimSpace(name)
	if strings.HasPrefix(strings.ToLower(n), "the ") {
		n = n[4:]
	}
	return "the " + n + " company"
}
