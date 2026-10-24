package core

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// cardOf is what a card shows of a hero that matters for comparing two ways of making one.
func cardOf(u CompanyUnit) string {
	var b strings.Builder
	fmt.Fprintf(&b, "type=%s points=%d;", u.Type, u.Points)
	for _, a := range u.Stats {
		fmt.Fprintf(&b, " %s=%d", a.Name, a.Value)
	}
	var rules, gear []string
	for _, r := range u.Rules {
		rules = append(rules, r.Name)
	}
	for _, e := range u.Equipment {
		gear = append(gear, e.Name)
	}
	slices.Sort(rules)
	slices.Sort(gear)
	fmt.Fprintf(&b, "; rules=%v; gear=%v", rules, gear)
	if u.Path != nil {
		var pr, ha []string
		for _, r := range u.Path.Rules {
			pr = append(pr, r.Name)
		}
		for _, a := range u.Path.HeroicActions {
			ha = append(ha, a.Name)
		}
		slices.Sort(pr)
		slices.Sort(ha)
		fmt.Fprintf(&b, "; path=%s %v %v", u.Path.Name, pr, ha)
	}
	return b.String()
}

// Choosing a unit as an officer of the starting company is the same as the unit having rolled a 6 on the warrior table and
// become a hero: for every unit of every company that may lead, the card of the officer (after it chose its path) equals the
// card of the warrior that rolled the 6 and chose the same path.
func TestAnOfficerOfTheStartingCompanyIsAHeroInTheMakingInEveryCompany(t *testing.T) {
	rs := shippedRules(t)
	const path = "Path of the Soldier"
	compared := 0
	for _, p := range rs.BC.Profiles() {
		list := p.StartList()
		for i, s := range list {
			if !p.CanLead(s.Unit) {
				continue
			}
			t.Run(p.Name+"/"+s.Unit+"/"+fmt.Sprint(i), func(t *testing.T) {
				officer := StartingUnits(p, []StartHero{{Index: i, Role: RoleSergeant}})[i]
				officer = WithPath(officer, path, "", rs.BC.Progression())
				fo := newCompanyFixture(t)
				fo.companyOf(p.Name, 5, []UnitState{officer})
				fo.state.Units[0].ID = 90
				got := cardOf(fo.unit(t, 90))

				f := newCompanyFixture(t)
				l := f.leveller(t)
				warrior := StartingUnits(p, nil)[i]
				warrior.Role, warrior.ID, warrior.XP = RoleWarrior, 90, 5
				f.companyOf(p.Name, 5, []UnitState{warrior})
				if _, err := l.RollLevelUp(f.ctx, f.user, 5, 90, []int{6}, false); err != nil {
					t.Fatal(err)
				}
				if _, err := l.ChooseLevelUp(f.ctx, f.user, 5, 90, []string{"path:" + path}); err != nil {
					t.Fatal(err)
				}
				want := cardOf(f.unit(t, 90))
				if got != want {
					t.Errorf("officer and Hero in the Making differ:\n officer: %s\n rolled 6: %s", got, want)
				}
			})
			compared++
		}
	}
	if compared < 37 {
		t.Fatalf("only %d units compared", compared)
	}
}

// Every unit of a company that has a heroPromotion becomes the promoted unit when made an officer, by every road to officer.
func TestAnOfficerGetsTheCompanysPromotionWhenItBecomesAHero(t *testing.T) {
	rs := shippedRules(t)
	n := 0
	for _, p := range rs.BC.Profiles() {
		for _, promo := range p.Promotions {
			n++
			t.Run(p.Name+"/"+promo.From, func(t *testing.T) {
				us := UnitState{Base: promo.From, Role: RoleSergeant, Gear: []string{"Spear", "Armour", "Bow"}}
				p.BecomeHero(&us)
				if us.Base != promo.Becomes {
					t.Errorf("becomes %s, not %s", us.Base, promo.Becomes)
				}
				for _, lost := range promo.Drop {
					if slices.Contains(us.Gear, lost) {
						t.Errorf("%s is lost: %v", lost, us.Gear)
					}
				}
				if !slices.Contains(us.Gear, "Bow") {
					t.Errorf("the other wargear stays: %v", us.Gear)
				}
				if got := us.Bonus["might"]; (got == 1) != promo.StatGain {
					t.Errorf("hero stat gain %v but bonus %v", promo.StatGain, us.Bonus)
				}
				again := UnitState{Base: promo.From, Progress: &Progress{Base: promo.From}}
				p.BecomeHero(&again)
				if again.Progress.Base != promo.Becomes {
					t.Errorf("a unit promoted by a level-up keeps the promotion in its progress: %+v", again.Progress)
				}
			})
		}
		other := UnitState{Base: "Nobody Special", Role: RoleSergeant}
		p.BecomeHero(&other)
		if other.Base != "Nobody Special" {
			t.Errorf("%s: no promotion, no change: %s", p.Name, other.Base)
		}
	}
	if n == 0 {
		t.Fatal("no company has a promotion")
	}
}

func TestAnArnorRangerOfficerIsARangerOfTheNorthFromTheStart(t *testing.T) {
	rs := shippedRules(t)
	p, _ := rs.BC.Profile("Arnor")
	var idx []int
	for i, s := range p.StartList() {
		if s.Unit == "Ranger of Arnor" {
			idx = append(idx, i)
		}
	}
	units := StartingUnits(p, []StartHero{{Index: idx[0], Role: RoleLeader}})
	if units[idx[0]].Base != "Ranger of the North" || units[idx[1]].Base != "Ranger of Arnor" {
		t.Errorf("only the officer is promoted: %s, %s", units[idx[0]].Base, units[idx[1]].Base)
	}
	if len(units[idx[0]].Bonus) != 0 {
		t.Errorf("a Ranger of the North has its Might, Will and Fate: %v", units[idx[0]].Bonus)
	}
}

func TestAnOfficerMadeBeforeTheFixIsPromotedWhenItChoosesItsPath(t *testing.T) {
	f := newCompanyFixture(t)
	l := f.leveller(t)
	f.companyOf("Arnor", 5, []UnitState{{ID: 90, Base: "Ranger of Arnor", Role: RoleLeader, Bonus: HeroBonus(nil)}})
	if u := f.unit(t, 90); u.Type != "Ranger of Arnor" || !u.PathPending {
		t.Fatalf("legacy: %+v", u.Type)
	}
	if err := l.ChoosePath(f.ctx, f.user, 5, 90, "Path of the Soldier", ""); err != nil {
		t.Fatal(err)
	}
	u := f.unit(t, 90)
	if u.Type != "Ranger of the North" {
		t.Errorf("promoted: %s", u.Type)
	}
	for _, a := range u.Stats {
		if a.HeroOnly && a.Boost != 0 {
			t.Errorf("no extra Might, Will or Fate: %+v", a)
		}
	}
}
