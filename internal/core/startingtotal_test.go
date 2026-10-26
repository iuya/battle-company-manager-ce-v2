package core

import "testing"

// startingTotal is the number the rules give every starting company: the points of the company once its officers are chosen plus
// the starting gold are 105 (Battle Companies: 100 points of warriors and officers and the gold the company has left).
const startingTotal = 105

// startingTotalExceptions lists the companies whose total is not 105 and why. The total must be the one given here for every
// choice of officers. Defenders of the North: the Battle Companies book lists 1 Iron Hills Dwarf with shield & spear, 2 with
// mattock, 1 Warrior of Dale with shield & spear and 2 with Esgaroth bow, 0 gold; at the points of the current MESBG book (Iron
// Hills Warrior 10, Warrior of Dale 7) that is 61 points of warriors, one over the 60 the starting company should add up to
// (BC "Creating Custom Companies": 60 points of Rating and Gold before Might, Will and Fate, plus 5 points each for 3 heroes).
var startingTotalExceptions = map[string]int{"Defenders of the North": 106}

func startPoints(t *testing.T, rs *RuleSet, p CompanyProfile, heroes []StartHero) (points int) {
	t.Helper()
	st := CompanyState{Company: CampaignCompany{ID: 1, Name: p.Name, Faction: p.Name}, Influence: StartingInfluence, Gold: p.StartGold, Units: StartingUnits(p, heroes)}
	return BuildDetail(rs, CampaignSettings{}, st).Points
}

// Points plus starting gold are 105 for every company once the officers are chosen: for the suggested officers and for every
// allowed choice of one leader and two sergeants.
func TestStartingCompanyPointsPlusGoldAre105InEveryCompany(t *testing.T) {
	rs := shippedRules(t)
	for _, p := range rs.BC.Profiles() {
		t.Run(p.Name, func(t *testing.T) {
			want := startingTotal
			if n, ok := startingTotalExceptions[p.Name]; ok {
				want = n
			}
			pts := startPoints(t, rs, p, p.DefaultHeroes())
			t.Logf("%s: points %d + gold %d = %d", p.Name, pts, p.StartGold, pts+p.StartGold)
			if pts+p.StartGold != want {
				t.Errorf("suggested officers: %d points + %d gold = %d, not %d", pts, p.StartGold, pts+p.StartGold, want)
			}
			var cand []int
			for i, s := range p.StartList() {
				if p.CanLeadStart(s) {
					cand = append(cand, i)
				}
			}
			var combos [][]StartHero
			for _, l := range cand {
				for a := 0; a < len(cand); a++ {
					for b := a + 1; b < len(cand); b++ {
						if cand[a] == l || cand[b] == l {
							continue
						}
						h := []StartHero{{l, RoleLeader}, {cand[a], RoleSergeant}, {cand[b], RoleSergeant}}
						if _, err := p.ValidateStart(h); err != nil { // the rules refuse an officer choice that leaves a hero by profile a warrior
							if len(p.heroesByProfile()) == 0 {
								t.Errorf("officers %v refused: %v", h, err)
							}
							continue
						}
						combos = append(combos, h)
					}
				}
			}
			stride := max(1, len(combos)/120) // every allowed choice for a small company, an even sample of at most about 120 for a big one
			bad := 0
			for i := 0; i < len(combos); i += stride {
				if got := startPoints(t, rs, p, combos[i]) + p.StartGold; got != want {
					if bad++; bad <= 3 {
						t.Errorf("officers %v: total %d, not %d", combos[i], got, want)
					}
				}
			}
		})
	}
}

// The Shire's Ranger of the North (and Helm's Deep's) has its Might, Will and Fate already: left out of the officers, three others
// would gain 15 points each and the company would add up to 120. The rules refuse such a choice.
func TestAHeroByProfileMustBeAnOfficer(t *testing.T) {
	rs := shippedRules(t)
	for _, name := range []string{"The Shire", "Helm's Deep"} {
		p, _ := rs.BC.Profile(name)
		if len(p.heroesByProfile()) == 0 {
			t.Fatalf("%s: no hero by profile", name)
		}
		var h []StartHero
		for i, s := range p.StartList() {
			if len(h) < 3 && p.CanLeadStart(s) && p.StartGains(s.Unit) {
				role := RoleSergeant
				if len(h) == 0 {
					role = RoleLeader
				}
				h = append(h, StartHero{Index: i, Role: role})
			}
		}
		if len(h) == 3 {
			if _, err := p.ValidateStart(h); err == nil {
				t.Errorf("%s: officers without the Ranger of the North are accepted", name)
			}
		}
	}
}
