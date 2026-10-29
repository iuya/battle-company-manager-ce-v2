package core

import "testing"

// An officer chosen at company creation is not a warrior that rolled a 6: it keeps its profile (Arnor's Rangers of Arnor stay
// Rangers of Arnor) and gains the point of Might, Will and Fate; the company's hero promotion belongs to the level-up roll.
func TestStartingOfficersKeepTheirProfileAndGainMightWillAndFate(t *testing.T) {
	rs := shippedRules(t)
	for _, p := range rs.BC.Profiles() {
		heroes := p.DefaultHeroes()
		units := StartingUnits(p, heroes)
		list := p.StartList()
		for _, h := range heroes {
			u := units[h.Index]
			if u.Base != list[h.Index].Unit {
				t.Errorf("%s: officer %s became %s", p.Name, list[h.Index].Unit, u.Base)
			}
			if gain := u.Bonus["might"] == 1; gain != p.StartGains(u.Base) {
				t.Errorf("%s: %s gains Might, Will and Fate %v, the company says %v", p.Name, u.Base, gain, p.StartGains(u.Base))
			}
		}
	}
}

func TestArnorRangersOfArnorChosenAsOfficersStayRangersOfArnor(t *testing.T) {
	rs := shippedRules(t)
	p, _ := rs.BC.Profile("Arnor")
	var idx []int
	for i, s := range p.StartList() {
		if s.Unit == "Ranger of Arnor" {
			idx = append(idx, i)
		}
	}
	var heroes []StartHero
	for n, i := range idx {
		role := RoleSergeant
		if n == 0 {
			role = RoleLeader
		}
		heroes = append(heroes, StartHero{Index: i, Role: role})
	}
	units := StartingUnits(p, heroes)
	for _, i := range idx {
		if units[i].Base != "Ranger of Arnor" || units[i].Bonus["might"] != 1 {
			t.Errorf("officer %+v", units[i])
		}
	}
	st := CompanyState{Company: CampaignCompany{ID: 1, Faction: "Arnor"}, Gold: p.StartGold, Units: units}
	d := BuildDetail(rs, CampaignSettings{}, st)
	if d.Points != 101 || d.Points+p.StartGold != 105 {
		t.Errorf("101 points and 4 gold are 105: %d + %d", d.Points, p.StartGold)
	}
	if got := unitByID(t, d, int64(idx[0]+1)).Points; got != 23 {
		t.Errorf("a Ranger of Arnor officer is 8 + 15: %d", got)
	}
}

func TestWanderersStartWithTwoHeroesInTheMakingBesidesTheOfficers(t *testing.T) {
	rs := shippedRules(t)
	p, _ := rs.BC.Profile("Wanderers in the Wild")
	units := StartingUnits(p, p.DefaultHeroes())
	heroes := 0
	for i, u := range units {
		if u.Role != RoleWarrior {
			heroes++
		}
		if p.StartList()[i].Role == RoleHeroInTheMaking && (u.Role != RoleHero || u.Bonus["might"] != 1) {
			t.Errorf("%s starts as a Hero in the Making: %+v", u.Base, u)
		}
		if p.StartList()[i].Role == RoleHeroInTheMaking && p.CanLeadStart(p.StartList()[i]) {
			t.Errorf("a Hero in the Making is not one of the officers the player picks")
		}
	}
	if heroes != 5 {
		t.Errorf("3 officers and 2 heroes in the making: %d", heroes)
	}
}

// A warrior that takes the place of a lost officer is not a warrior that rolled a 6 either: no promotion.
func TestChoosePathLeavesALegacyOfficersProfileAsItIs(t *testing.T) {
	f := newCompanyFixture(t)
	l := f.leveller(t)
	f.companyOf("Arnor", 5, []UnitState{{ID: 90, Base: "Ranger of Arnor", Role: RoleLeader, Bonus: HeroBonus(nil)}, {ID: 91, Base: "Ranger of the North", Role: RoleSergeant}})
	for _, id := range []int64{90, 91} {
		before := f.unit(t, id).Type
		if err := l.ChoosePath(f.ctx, f.user, 5, 1, id, "Path of the Soldier", ""); err != nil {
			t.Fatal(err)
		}
		if got := f.unit(t, id).Type; got != before {
			t.Errorf("choosing a path changes nothing but the path: %s to %s", before, got)
		}
	}
}
