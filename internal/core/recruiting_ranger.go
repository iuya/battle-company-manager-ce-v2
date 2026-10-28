package core

import (
	"fmt"
	"slices"
	"strings"
)

// The Shire's Ranger of the North replacement (docs/frontend.md, "Ranger of the North replacement"): when the company's Ranger
// has been slain, any successful reinforcement result may be swapped for a new one. He starts as the Leader (he may be put in a
// Sergeant place instead) and never leaves the officer places, so the officers are reshuffled: the old Leader and Sergeants fill
// the other places and, when there are three of them, one stops being an officer and is an ordinary Hero. The rule is data (the
// effect rangerReplacement of the company); this file is the management-layer behaviour.

// RangerReplacement is the effect rangerReplacement of a company: the unit that replaces a result, how many the company may have
// at once and the places it may take.
type RangerReplacement struct {
	Unit  string
	Max   int
	Roles []UnitRole // the file's order (Leader, Sergeant)
}

// RangerID stands for the new Ranger in an OfficerArrangement and an OfficerPanel: he has no unit yet.
const RangerID int64 = 0

// RangerOffer is what a rolled result may be swapped for.
type RangerOffer struct {
	Unit string
}

// OfficerCard is a unit of the officer panel.
type OfficerCard struct {
	ID    int64 // RangerID for the new Ranger
	Title string
	Role  UnitRole
	Type  string
}

// OfficerPanel is what the player arranges after choosing the Ranger: the Ranger (pre-placed as Leader), the other officers (the
// Leader and Sergeants, active Heroes), the Heroes who are not officers and how many of the officers must stop being one.
type OfficerPanel struct {
	Unit       string
	Leader     OfficerCard // the Ranger, ID RangerID
	Officers   []OfficerCard
	Candidates []OfficerCard // only used when there are fewer than three other officers
	Sergeants  int           // the Sergeant places of a company (campaign.company.sergeants)
	Leaders    int           // and its Leader places
	Drop       int           // max(0, len(Officers) - the Sergeants a company has)
}

// OfficerArrangement is the player's arrangement: the unit for the Leader place and for the Sergeant places (at most as many as the company has Sergeants). The
// new Ranger is RangerID; he is in exactly one of the officer places.
type OfficerArrangement struct {
	Leader    int64
	Sergeants []int64
}

// rangerOffer is nil unless the company has the rule (and it lets him lead), the cemetery holds one of its unit (however it
// fell: dead, or a hero forced to retire) and the roster has fewer than Max of them.
func (env recruitEnv) rangerOffer() *RangerOffer {
	profile, ok := env.rs.BC.Profile(env.bcC.Name)
	if !ok || profile.RangerReplacement == nil {
		return nil
	}
	rr := profile.RangerReplacement
	if !slices.Contains(rr.Roles, RoleLeader) {
		return nil
	}
	if !slices.ContainsFunc(env.d.Cemetery, func(f FallenUnit) bool { return f.Unit.Type == rr.Unit }) {
		return nil
	}
	n := 0
	for _, u := range env.d.Roster {
		if u.Type == rr.Unit {
			n++
		}
	}
	if n >= rr.Max {
		return nil
	}
	return &RangerOffer{Unit: rr.Unit}
}

// officerPanel is the arrangement the player starts from.
func (env recruitEnv) officerPanel(unit string) *OfficerPanel {
	p := &OfficerPanel{Unit: unit, Leader: OfficerCard{ID: RangerID, Title: unit, Role: RoleLeader, Type: unit}}
	titled := Distinguish(env.d.Roster)
	for i, u := range env.d.Roster {
		card := OfficerCard{ID: u.ID, Title: titled[i].Title(), Role: u.Role, Type: u.Type}
		switch u.Role {
		case RoleLeader, RoleSergeant:
			p.Officers = append(p.Officers, card)
		case RoleHero:
			p.Candidates = append(p.Candidates, card)
		}
	}
	// the Leader first, then the Sergeants
	slices.SortStableFunc(p.Officers, func(a, b OfficerCard) int { return a.Role.order() - b.Role.order() })
	p.Sergeants, p.Leaders = env.rs.BC.Shape().Sergeants, env.rs.BC.Shape().Leaders
	p.Drop = max(0, len(p.Officers)-p.Sergeants)
	return p
}

// parkRanger swaps the rolled result for the Ranger: the roll is paid, the Ranger has to be able to join, and the recruit waits for
// ArrangeOfficers. Nothing changes in the roster yet.
func (env recruitEnv) parkRanger(st *CompanyState, prefix string, paid, roll int) (RecruitResult, error) {
	offer := env.rangerOffer()
	if offer == nil {
		return RecruitResult{}, Refuse("The Ranger of the North cannot be recruited now.")
	}
	if why, _ := env.block(Reinforcement{Type: "unit", Unit: offer.Unit, Count: 1}); why != "" {
		return RecruitResult{}, Refuse("The %s cannot join the company: %s.", offer.Unit, why)
	}
	st.Recruit.Officers = &OfficerStep{Unit: offer.Unit, Paid: paid, Roll: roll}
	return RecruitResult{Roll: roll, Summary: fmt.Sprintf("%syou take the %s instead%s: arrange the officers.", prefix, offer.Unit, influenceText(paid)),
		Officers: env.officerPanel(offer.Unit)}, nil
}

// arrange validates the arrangement against the panel and applies it to the state. Nothing is changed on an error.
func (env recruitEnv) arrange(st *CompanyState, step OfficerStep, a OfficerArrangement) (RecruitResult, error) {
	panel := env.officerPanel(step.Unit)
	known := map[int64]OfficerCard{RangerID: panel.Leader}
	isOfficer := map[int64]bool{}
	for _, c := range panel.Officers {
		known[c.ID], isOfficer[c.ID] = c, true
	}
	for _, c := range panel.Candidates {
		known[c.ID] = c
	}
	if sergeants := env.rs.BC.Shape().Sergeants; len(a.Sergeants) > sergeants {
		return RecruitResult{}, Refuse("A company has at most %d Sergeants.", sergeants)
	}
	places := append([]int64{a.Leader}, a.Sergeants...)
	seen := map[int64]bool{}
	named := 0
	for _, id := range places {
		c, ok := known[id]
		switch {
		case !ok:
			return RecruitResult{}, Refuse("That unit cannot be appointed.")
		case seen[id]:
			return RecruitResult{}, Refuse("%s is named twice.", c.Title)
		}
		seen[id] = true
		if isOfficer[id] {
			named++
		}
	}
	if !seen[RangerID] {
		return RecruitResult{}, Refuse("The %s takes command: he cannot be left out.", step.Unit)
	}
	if want := len(panel.Officers) - panel.Drop; named != want {
		if panel.Drop > 0 {
			return RecruitResult{}, Refuse("Exactly %d of the other officers keep their place; the rest stop being officers.", want)
		}
		return RecruitResult{}, Refuse("The other officers keep their places.")
	}
	// apply
	roleOf := map[int64]UnitRole{a.Leader: RoleLeader}
	for _, id := range a.Sergeants {
		roleOf[id] = RoleSergeant
	}
	var dropped []string
	for _, c := range panel.Officers {
		if _, keeps := roleOf[c.ID]; !keeps {
			dropped = append(dropped, c.Title)
		}
	}
	for i := range st.Units {
		u := &st.Units[i]
		if role, ok := roleOf[u.ID]; ok {
			u.Role = role
		} else if isOfficer[u.ID] {
			u.Role = RoleHero // keeps its path, level, XP, gear and injuries
		}
	}
	st.Units = append(st.Units, UnitState{Base: step.Unit, Role: roleOf[RangerID]})
	var names []string
	for _, id := range places {
		role := "Sergeant"
		if id == a.Leader {
			role = "Leader"
		}
		names = append(names, fmt.Sprintf("%s as %s", known[id].Title, role))
	}
	text := fmt.Sprintf("Officers: %s.", strings.Join(names, ", "))
	if len(dropped) > 0 {
		text += fmt.Sprintf(" %s is no longer an officer.", strings.Join(dropped, " and "))
	}
	st.Recruit.Officers = nil
	return RecruitResult{Roll: step.Roll, Summary: text}, nil
}

// rangerMayReturn reports (the unit's name, true) when the company has the replacement rule, the cemetery holds its unit and the roster has fewer than Max.
func rangerMayReturn(rs *RuleSet, company string, roster []CompanyUnit, cemetery []FallenUnit) (unit string, ok bool) {
	profile, ok := rs.BC.Profile(company)
	if !ok || profile.RangerReplacement == nil {
		return "", false
	}
	rr := profile.RangerReplacement
	if !slices.ContainsFunc(cemetery, func(f FallenUnit) bool { return f.Unit.Type == rr.Unit }) || len(rr.Roles) == 0 {
		return "", false
	}
	n := 0
	for _, u := range roster {
		if u.Type == rr.Unit {
			n++
		}
	}
	return rr.Unit, n < rr.Max
}
