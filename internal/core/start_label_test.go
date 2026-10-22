package core

import "testing"

// Wanderers in the Wild start with a Tookish Hunter and a Battlin' Brandybuck: the Hobbit Archer and Hobbit Militia with the option
// their own name selects (a point more), not plain ones.
func TestStartingUnitsKeepTheCompanysOwnName(t *testing.T) {
	rs := shippedRules(t)
	p, ok := rs.BC.Profile("Wanderers in the Wild")
	if !ok {
		t.Fatal("no profile")
	}
	want := map[string]int{"Tookish Hunter": 6, "Battlin' Brandybuck": 5}
	seen := map[string]bool{}
	for _, s := range p.Start {
		pts, ok := want[s.Unit]
		if !ok {
			continue
		}
		seen[s.Unit] = true
		u := buildUnit(rs, rs.BC.companies["Wanderers in the Wild"], UnitState{Base: s.Unit, Role: RoleWarrior}, CampaignSettings{})
		if u.Points != pts {
			t.Errorf("%s is worth %d, want %d", s.Unit, u.Points, pts)
		}
	}
	for n := range want {
		if !seen[n] {
			t.Errorf("the start list has no %s: %+v", n, p.Start)
		}
	}
}

// A Hobbit under a company's own name (Tookish Hunter, Battlin' Brandybuck) can take the Hobbit-only hero upgrade.
func TestHobbitUpgradeIsOpenToACompanyNamedHobbit(t *testing.T) {
	rs := shippedRules(t)
	c := rs.BC.companies["Wanderers in the Wild"]
	for _, name := range []string{"Hobbit Militia", "Tookish Hunter", "Battlin' Brandybuck"} {
		got := upgradesOf(rs, c, CompanyUnit{Type: name, Role: RoleSergeant})
		if len(got) != 2 {
			t.Errorf("%s can take %d hero upgrades, want 2 (a Hobbit takes both)", name, len(got))
		}
	}
	if got := upgradesOf(rs, c, CompanyUnit{Type: "Warrior of Numenor", Role: RoleSergeant}); len(got) != 1 {
		t.Errorf("a Man can take %d upgrades, want 1", len(got))
	}
}

// Every recruit of the Wanderers in the Wild joins as a Hero in the Making (+1 Might, Will and Fate); other companies recruit warriors.
func TestWanderersRecruitsJoinAsHeroesInTheMaking(t *testing.T) {
	rs := shippedRules(t)
	for _, tc := range []struct {
		company string
		role    UnitRole
	}{{"Wanderers in the Wild", RoleHero}, {"The Shire", RoleWarrior}} {
		env := recruitEnv{rs: rs, bcC: rs.BC.companies[tc.company]}
		u := env.newRecruit("Warrior of Numenor", nil)
		if u.Role != tc.role {
			t.Errorf("%s: a recruit joins as %q, want %q", tc.company, u.Role, tc.role)
		}
		if tc.role == RoleHero && (u.Bonus["might"] != 1 || u.Bonus["will"] != 1 || u.Bonus["fate"] != 1) {
			t.Errorf("%s: recruit bonus %v, want 1 Might, Will and Fate", tc.company, u.Bonus)
		}
	}
}
