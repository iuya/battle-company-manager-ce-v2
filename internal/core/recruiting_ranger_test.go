package core

import (
	"slices"
	"strings"
	"testing"
)

const rangerUnit = "Ranger of the North"

// shireWithFallenRanger is a Shire company whose Ranger of the North is in the cemetery: a Leader, one Sergeant and warriors.
func shireWithFallenRanger(f *companyFixture, fallen bool) {
	units := []UnitState{{ID: 1, Base: "Hobbit Shirriff", Role: RoleLeader, Path: "x"}, {ID: 2, Base: "Hobbit Shirriff", Role: RoleSergeant, Path: "x"}}
	units = append(units, warriors("Hobbit Militia", 3)...)
	for i := 2; i < len(units); i++ {
		units[i].ID = int64(10 + i)
	}
	f.companyOf("The Shire", 12, units)
	if fallen {
		f.state.Cemetery = []FallenUnit{{Unit: CompanyUnit{ID: 99, Type: rangerUnit}, Fate: "Dead"}}
	}
}

func rollOnly(t *testing.T, f *companyFixture, s *RecruitService, roll int) *RecruitRoll {
	t.Helper()
	if err := s.RollRecruit(f.ctx, f.user, 5, 1, roll, false); err != nil {
		t.Fatal(err)
	}
	state, err := s.RecruitState(f.ctx, f.user, 5, 1)
	if err != nil || state == nil {
		t.Fatalf("RecruitState = %+v, %v", state, err)
	}
	return state
}

func TestRangerOfferConditions(t *testing.T) {
	for _, c := range []struct {
		name  string
		setup func(f *companyFixture)
		roll  int
		want  bool
	}{
		{"fallen Ranger, a successful row", func(f *companyFixture) { shireWithFallenRanger(f, true) }, 4, true},
		{"fallen Ranger, a two-model row", func(f *companyFixture) { shireWithFallenRanger(f, true) }, 5, true},
		{"row 1, nobody steps forward", func(f *companyFixture) { shireWithFallenRanger(f, true) }, 1, false},
		{"no Ranger has fallen", func(f *companyFixture) { shireWithFallenRanger(f, false) }, 4, false},
		{"a Ranger is in the roster already", func(f *companyFixture) {
			shireWithFallenRanger(f, true)
			f.state.Units = append(f.state.Units, UnitState{ID: 50, Base: rangerUnit, Role: RoleHero, Path: "x"})
		}, 4, false},
		{"another company", func(f *companyFixture) {
			f.companyOf("Minas Tirith", 12, warriors("Warrior of Minas Tirith", 3))
			f.state.Cemetery = []FallenUnit{{Unit: CompanyUnit{Type: rangerUnit}}}
		}, 4, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newCompanyFixture(t)
			s := f.recruiter(t)
			c.setup(f)
			got := rollOnly(t, f, s, c.roll).Ranger
			if (got != nil) != c.want {
				t.Fatalf("offer = %+v, want offered %v", got, c.want)
			}
		})
	}
}

// threeOfficers adds a second Sergeant (id 3) to the Shire company: Leader 1, Sergeants 2 and 3, a Hero 4 who is no officer.
func threeOfficers(f *companyFixture) {
	shireWithFallenRanger(f, true)
	f.state.Units = append(f.state.Units,
		UnitState{ID: 3, Base: "Hobbit Shirriff", Role: RoleSergeant, Path: "y", Level: 2, XP: 7, Injuries: map[string]int{"armWound": 1}, Bought: []string{"Elven cloak"}},
		UnitState{ID: 4, Base: "Hobbit Shirriff", Role: RoleHero, Path: "z"})
}

func stateUnitByID(st CompanyState, id int64) UnitState {
	for _, u := range st.Units {
		if u.ID == id {
			return u
		}
	}
	return UnitState{}
}

func rolesOf(st CompanyState) (leaders, sergeants int) {
	for _, u := range st.Units {
		switch u.Role {
		case RoleLeader:
			leaders++
		case RoleSergeant:
			sergeants++
		}
	}
	return
}

func parkTheRanger(t *testing.T, f *companyFixture, s *RecruitService) RecruitResult {
	t.Helper()
	rollOnly(t, f, s, 5) // two Hobbit Militia
	before := f.state.Influence
	res, err := s.ConfirmRecruit(f.ctx, f.user, 5, 1, 0, "ranger")
	if err != nil || res.Officers == nil || !strings.Contains(res.Summary, rangerUnit) {
		t.Fatalf("%+v %v", res, err)
	}
	if f.state.Influence != before || f.state.Recruit.Roll != nil || f.state.Recruit.Officers == nil {
		t.Fatalf("the swap is free and the arrangement waits: influence %d -> %d, %+v", before, f.state.Influence, f.state.Recruit)
	}
	return res
}

func TestPickingTheRangerParksTheArrangement(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.recruiter(t)
	threeOfficers(f)
	units := len(f.state.Units)
	res := parkTheRanger(t, f, s)
	p := res.Officers
	if len(f.state.Units) != units {
		t.Error("nothing changes in the roster until the officers are arranged")
	}
	if p.Unit != rangerUnit || p.Leader.ID != RangerID || p.Leader.Role != RoleLeader || len(p.Officers) != 3 || p.Drop != 1 || len(p.Candidates) != 1 || p.Candidates[0].ID != 4 {
		t.Errorf("panel: %+v", p)
	}
	if p.Officers[0].Role != RoleLeader || p.Officers[0].ID != 1 {
		t.Errorf("the Leader comes first: %+v", p.Officers)
	}
	got, err := s.PendingOfficers(f.ctx, f.user, 5, 1)
	if err != nil || got == nil || got.Drop != 1 {
		t.Errorf("PendingOfficers = %+v, %v", got, err)
	}
	if f.state.Recruit.Officers == nil || !f.state.Recruit.Active() {
		t.Error("an arrangement waiting is a recruit in progress")
	}
	if err := f.state.Locked(); err == nil || !strings.Contains(err.Error(), "Arrange the officers") {
		t.Errorf("the company is locked: %v", err)
	}
	if d := BuildDetail(shippedRules(t), f.settings, f.state); !d.OfficersPending {
		t.Error("the detail says officers are pending")
	}
	if err := s.RollRecruit(f.ctx, f.user, 5, 1, 4, false); err == nil {
		t.Error("no other recruit while the arrangement waits")
	}
}

func TestArrangeOfficers(t *testing.T) {
	for _, c := range []struct {
		name    string
		a       OfficerArrangement
		leader  int64 // unit id, RangerID for the Ranger
		dropped int64
	}{
		{"the Ranger leads, the old Leader and a Sergeant stay, one Sergeant drops", OfficerArrangement{Leader: RangerID, Sergeants: []int64{1, 2}}, RangerID, 3},
		{"the Ranger as a Sergeant, a Sergeant leads, the old Leader drops", OfficerArrangement{Leader: 2, Sergeants: []int64{RangerID, 3}}, 2, 1},
		{"the Ranger as a Sergeant, the old Leader stays", OfficerArrangement{Leader: 1, Sergeants: []int64{3, RangerID}}, 1, 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newCompanyFixture(t)
			s := f.recruiter(t)
			threeOfficers(f)
			parkTheRanger(t, f, s)
			res, err := s.ArrangeOfficers(f.ctx, f.user, 5, 1, c.a)
			if err != nil || f.state.Recruit.Officers != nil || !strings.Contains(res.Summary, "no longer an officer") {
				t.Fatalf("%+v %v", res, err)
			}
			leaders, sergeants := rolesOf(f.state)
			if leaders != 1 || sergeants != 2 {
				t.Errorf("exactly one Leader and two Sergeants, got %d and %d: %+v", leaders, sergeants, f.state.Units)
			}
			var ranger UnitState
			for _, u := range f.state.Units {
				if u.Base == rangerUnit {
					ranger = u
				}
			}
			if ranger.Bonus != nil || ranger.Path != "" || (c.leader == RangerID) != (ranger.Role == RoleLeader) || ranger.Role == RoleHero || ranger.Role == RoleWarrior {
				t.Errorf("the Ranger is an officer with no bonus and no path: %+v", ranger)
			}
			if c.leader != RangerID && stateUnitByID(f.state, c.leader).Role != RoleLeader {
				t.Errorf("unit %d should lead", c.leader)
			}
			d := stateUnitByID(f.state, c.dropped)
			orig := map[int64]UnitState{1: {Path: "x"}, 2: {Path: "x"}, 3: {Path: "y", Level: 2, XP: 7}}[c.dropped]
			if d.Role != RoleHero || d.Path != orig.Path || d.Level != orig.Level || d.XP != orig.XP {
				t.Errorf("the dropped officer is a plain Hero keeping its progress: %+v", d)
			}
			if c.dropped == 3 && (d.Injuries["armWound"] != 1 || len(d.Bought) != 1) {
				t.Errorf("injuries and gear stay: %+v", d)
			}
			if stateUnitByID(f.state, 4).Role != RoleHero {
				t.Error("a Hero who was no officer stays one")
			}
			if err := f.state.Locked(); err != nil {
				t.Errorf("unlocked again: %v", err)
			}
		})
	}
}

func TestArrangeOfficersRefusals(t *testing.T) {
	for _, c := range []struct {
		name string
		a    OfficerArrangement
		want string
	}{
		{"the Ranger left out", OfficerArrangement{Leader: 1, Sergeants: []int64{2, 3}}, "cannot be left out"},
		{"the Ranger named twice", OfficerArrangement{Leader: RangerID, Sergeants: []int64{RangerID, 1}}, "named twice"},
		{"a duplicate id", OfficerArrangement{Leader: RangerID, Sergeants: []int64{1, 1}}, "named twice"},
		{"three Sergeants", OfficerArrangement{Leader: RangerID, Sergeants: []int64{1, 2, 3}}, "at most 2"},
		{"an unknown id", OfficerArrangement{Leader: RangerID, Sergeants: []int64{1, 99}}, "cannot be appointed"},
		{"a warrior", OfficerArrangement{Leader: RangerID, Sergeants: []int64{1, 12}}, "cannot be appointed"},
		{"a non-officer Hero while three officers exist", OfficerArrangement{Leader: RangerID, Sergeants: []int64{1, 4}}, "keep their place"},
		{"nobody dropped", OfficerArrangement{Leader: 1, Sergeants: []int64{RangerID}}, "keep their place"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newCompanyFixture(t)
			s := f.recruiter(t)
			threeOfficers(f)
			parkTheRanger(t, f, s)
			units := slices.Clone(f.state.Units)
			if _, err := s.ArrangeOfficers(f.ctx, f.user, 5, 1, c.a); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q, got %v", c.want, err)
			}
			if f.state.Recruit.Officers == nil || len(f.state.Units) != len(units) {
				t.Error("nothing is stored on an error")
			}
			for i, u := range f.state.Units {
				if u.Role != units[i].Role {
					t.Errorf("roles unchanged: %+v", u)
				}
			}
		})
	}
	t.Run("nothing waits", func(t *testing.T) {
		f := newCompanyFixture(t)
		s := f.recruiter(t)
		threeOfficers(f)
		if _, err := s.ArrangeOfficers(f.ctx, f.user, 5, 1, OfficerArrangement{Leader: RangerID}); err == nil || !strings.Contains(err.Error(), "waiting") {
			t.Errorf("%v", err)
		}
	})
}

func TestArrangeOfficersWithFewerThanThreeOfficers(t *testing.T) {
	for _, c := range []struct {
		name      string
		officers  int // other officers in the company (Leader first)
		a         OfficerArrangement
		sergeants int
		wantErr   string
	}{
		{"two officers: both keep a place, nobody drops", 2, OfficerArrangement{Leader: RangerID, Sergeants: []int64{1, 2}}, 2, ""},
		{"one officer: a Sergeant place stays empty", 1, OfficerArrangement{Leader: RangerID, Sergeants: []int64{1}}, 1, ""},
		{"one officer and a Hero from the pool", 1, OfficerArrangement{Leader: 1, Sergeants: []int64{RangerID, 4}}, 2, ""},
		{"one officer left out", 1, OfficerArrangement{Leader: RangerID, Sergeants: []int64{4}}, 0, "keep their place"},
		{"no officer: the Ranger leads alone", 0, OfficerArrangement{Leader: RangerID}, 0, ""},
		{"no officer: Heroes from the pool", 0, OfficerArrangement{Leader: RangerID, Sergeants: []int64{4}}, 1, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newCompanyFixture(t)
			s := f.recruiter(t)
			threeOfficers(f)
			// keep only the first c.officers officers; the others become plain Heroes
			n := 0
			for i := range f.state.Units {
				if r := f.state.Units[i].Role; r == RoleLeader || r == RoleSergeant {
					if n++; n > c.officers {
						f.state.Units[i].Role = RoleHero
					}
				}
			}
			parkTheRanger(t, f, s)
			_, err := s.ArrangeOfficers(f.ctx, f.user, 5, 1, c.a)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("want %q, got %v", c.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if leaders, sergeants := rolesOf(f.state); leaders != 1 || sergeants != c.sergeants {
				t.Errorf("%d Leader and %d Sergeants, got %d and %d", 1, c.sergeants, leaders, sergeants)
			}
		})
	}
}

func TestRangerPickRefusals(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.recruiter(t)
	shireWithFallenRanger(f, false)
	rollOnly(t, f, s, 4)
	if _, err := s.ConfirmRecruit(f.ctx, f.user, 5, 1, 0, "ranger"); err == nil || !strings.Contains(err.Error(), "cannot be recruited now") {
		t.Errorf("no offer: %v", err)
	}
	for _, old := range []string{"ranger:leader", "ranger:sergeant:2"} {
		if _, err := s.ConfirmRecruit(f.ctx, f.user, 5, 1, 0, old); err != nil && f.state.Recruit.Officers != nil {
			t.Errorf("%s parked an arrangement", old)
		}
	}
	shireWithFallenRanger(f, true)
	f.state.Recruit = RecruitProgress{}
	rollOnly(t, f, s, 1)
	if _, err := s.ConfirmRecruit(f.ctx, f.user, 5, 1, 0, "ranger"); err == nil {
		t.Error("row 1 cannot be swapped")
	}
	// a result that could not join is still swappable and the lower panel does not open
	f2 := newCompanyFixture(t)
	s2 := f2.recruiter(t)
	shireWithFallenRanger(f2, true)
	f2.state.Units = append(f2.state.Units, warriors("Hobbit Militia", 19)...)
	for i := range f2.state.Units {
		f2.state.Units[i].ID = int64(i + 1)
	}
	rollOnly(t, f2, s2, 5) // two models do not fit in the last place
	res, err := s2.ConfirmRecruit(f2.ctx, f2.user, 5, 1, 0, "ranger")
	if err != nil || res.Lower || f2.state.Recruit.Lower != nil || f2.state.Recruit.Officers == nil {
		t.Errorf("the Ranger fits: %+v %v", res, err)
	}
	// with no room at all it is refused and the roll stays to confirm
	f3 := newCompanyFixture(t)
	s3 := f3.recruiter(t)
	shireWithFallenRanger(f3, true)
	f3.state.Units = append(f3.state.Units, warriors("Hobbit Militia", 19)...)
	for i := range f3.state.Units {
		f3.state.Units[i].ID = int64(i + 1)
	}
	rollOnly(t, f3, s3, 4)
	f3.state.Units = append(f3.state.Units, UnitState{ID: 100, Base: "Hobbit Militia", Role: RoleWarrior})
	if _, err := s3.ConfirmRecruit(f3.ctx, f3.user, 5, 1, 0, "ranger"); err == nil || !strings.Contains(err.Error(), "cannot join") || f3.state.Recruit.Roll == nil {
		t.Errorf("no room for the Ranger: %v", err)
	}
}

func TestShireProfileParsesTheRangerReplacement(t *testing.T) {
	p, ok := shippedRules(t).BC.Profile("The Shire")
	if !ok || p.RangerReplacement == nil {
		t.Fatalf("the Shire has the rule: %+v", p.RangerReplacement)
	}
	if r := p.RangerReplacement; r.Unit != rangerUnit || r.Max != 1 || len(r.Roles) != 2 || r.Roles[0] != RoleLeader || r.Roles[1] != RoleSergeant {
		t.Errorf("%+v", r)
	}
	if o, ok := shippedRules(t).BC.Profile("Minas Tirith"); ok && o.RangerReplacement != nil {
		t.Error("only the Shire has it")
	}
}

func TestRangerMayReturnOnTheCompanyDetail(t *testing.T) {
	f := newCompanyFixture(t)
	shireWithFallenRanger(f, false)
	if f.detail(t).RangerMayReturn {
		t.Error("no Ranger in the cemetery: nothing may return")
	}
	f = newCompanyFixture(t)
	shireWithFallenRanger(f, true)
	if !f.detail(t).RangerMayReturn {
		t.Error("the Ranger fell and none is on the roster: one may return")
	}
}
