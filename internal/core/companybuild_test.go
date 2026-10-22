package core

import (
	"context"
	"slices"
	"sync"
	"testing"
)

// fileRepo serves the files the app ships with to the loader.
type fileRepo struct {
	rulesetRepository
	files map[string]StoredRuleset
}

func (f fileRepo) Get(_ context.Context, hash string) (StoredRuleset, error) {
	if s, ok := f.files[hash]; ok {
		return s, nil
	}
	return StoredRuleset{}, ErrNotFound
}

var (
	shippedOnce sync.Once
	shippedSet  *RuleSet
	shippedErr  error
)

// shippedRules is the RuleSet of the default files, built once for all tests.
func shippedRules(t *testing.T) *RuleSet {
	t.Helper()
	shippedOnce.Do(func() {
		files := map[string]StoredRuleset{}
		var h RulesetHashes
		for _, f := range []struct {
			kind RulesetKind
			path string
			set  *string
		}{
			{KindVanilla, "vanilla/data2024.json", &h.Vanilla},
			{KindBattleCompanies, "battle_companies/battle_companies.json", &h.BattleCompanies},
			{KindGlossary, "glossary/mesbg_glossary.json", &h.Glossary},
		} {
			content := readDef(t, f.path)
			hash, err := RulesetHash(content)
			if err != nil {
				shippedErr = err
				return
			}
			*f.set = hash
			files[hash] = StoredRuleset{RulesetInfo: RulesetInfo{Kind: f.kind, Hash: hash}, Content: content}
		}
		shippedSet, shippedErr = NewRules(fileRepo{files: files}, nil).For(context.Background(), h)
	})
	if shippedErr != nil {
		t.Fatal(shippedErr)
	}
	return shippedSet
}

// gondor is the Minas Tirith demo company of the fake backend as a stored state.
func gondor() CompanyState {
	return CompanyState{
		Company:   CampaignCompany{ID: 1, Name: "The White Company", Faction: "Minas Tirith"},
		Influence: 12,
		Units: []UnitState{
			{ID: 1, Name: "Beregond", Base: "Warrior of Minas Tirith", Role: RoleLeader, Bonus: HeroBonus(map[string]int{"fight": 1, "attack": 1}), Bought: []string{"Throwing Spear", "Badge of Courage", "Mountain Boots"}, Path: "Path of the Warlord", PathRules: 2, Actions: []string{"Heroic Strength"}, BoughtActions: []string{"Heroic Challenge"}, Level: 2, XP: 11},
			{ID: 2, Name: "Hador", Base: "Warrior of Minas Tirith", Role: RoleSergeant, Bonus: HeroBonus(nil), Bought: []string{"Spear"}, Path: "Path of the Shieldbearer", PathRules: 1, Actions: []string{"Heroic Defence"}, XP: 15},
			{ID: 3, Name: "Baranor", Base: "Warrior of Minas Tirith", Role: RoleSergeant, Bonus: HeroBonus(map[string]int{"shoot": -1}), Bought: []string{"Bow"}, Path: "Path of the Marksman", PathRules: 2, Actions: []string{"Heroic Accuracy"}, Level: 1, XP: 7},
			{ID: 4, Name: "Faradir", Base: "Citadel Guard", Role: RoleHero, Bonus: HeroBonus(map[string]int{"courage": -1}), Injuries: map[string]int{"armWound": 1, "legWound": 1}, Injured: true, Bought: []string{"Crossbow", "Concealing Cloak"}, Path: "Path of the Ranger", PathRules: 2, BoughtActions: []string{"Heroic Shoot"}, Level: 3, XP: 17},
			{ID: 5, Name: "Elphir", Base: "Knight of Minas Tirith", Role: RoleHero, Bonus: HeroBonus(map[string]int{"attack": 1}), Injuries: map[string]int{"brokenHonour": 2}, Bought: []string{"Barding"}, Path: "Path of the Duelist", PathRules: 1, Actions: []string{"Heroic Combat"}, Level: 1, XP: 5},
			{ID: 6, Name: "Cirion", Base: "Warrior of Minas Tirith", Role: RoleWarrior, Bought: []string{"Bow"}, XP: 3},
			{ID: 7, Name: "Dorlas", Base: "Warrior of Minas Tirith", Role: RoleWarrior, XP: 5, Injured: true},
			{ID: 8, Base: "Warrior of Minas Tirith", Role: RoleWarrior, Bought: []string{"Throwing Spear"}, XP: 6},
			{ID: 9, Name: "Mablung", Base: "Warrior of Minas Tirith", Role: RoleWarrior, Bought: []string{"Company Standard"}, Level: 1, XP: 8},
			{ID: 10, Base: "Warrior of Minas Tirith", Role: RoleWarrior},
			{ID: 11, Name: "Ioreth", Base: "Warrior of Minas Tirith", Role: RoleWarrior, Gear: []string{"Guardian of the King"}, XP: 1},
		},
	}
}

// mordor is the Mordor demo company.
func mordor() CompanyState {
	return CompanyState{
		Company: CampaignCompany{ID: 1, Name: "The Black Hand", Faction: "Mordor"},
		Units: []UnitState{
			{ID: 1, Name: "Grishnákh", Base: "Mordor Orc Warrior", Role: RoleLeader, Bonus: HeroBonus(map[string]int{"strength": 1}), Bought: []string{"Shield", "Trophy Pelt", "Ring of Warding"}, Path: "Path of the Tactician", PathRules: 2, Actions: []string{"Heroic March"}, Level: 2, XP: 10},
			{ID: 2, Name: "Snaga", Base: "Orc Tracker", Role: RoleSergeant, Bonus: HeroBonus(nil), Bought: []string{"Envenom Weapon (X)"}, Path: "Path of the Sharpshooter", PathRules: 1, Level: 1, XP: 6},
			{ID: 3, Name: "Ugluk the Younger", Base: "Uruk-hai Warrior", Role: RoleSergeant, Bonus: HeroBonus(nil), Bought: []string{"Throwing Weapon"}, Path: "Path of the Berserker", PathRules: 1, Actions: []string{"Heroic Strike"}, XP: 10},
			{ID: 4, Name: "Gorbag", Base: "Mordor Troll Chieftain", Role: RoleHero, Path: "Path of the Soldier", PathRules: 1, Level: 2, XP: 12},
			{ID: 5, Name: "Muzgûr", Base: "Muzgûr, Orc Shaman", Role: RoleHero, Path: "Path of the Sorcerer", PathRules: 2, BoughtActions: []string{"Heroic Channelling"}, Spells: []PathSpell{{"Black Dart", 3, 12}, {"Aura of Command", 4, 6}}, Level: 1, XP: 9},
			{ID: 6, Name: "Sharku", Base: "Sharku, Warg Rider Captain", Role: RoleHero, Path: "Path of the Warlord", PathRules: 1, XP: 2},
			{ID: 7, Name: "Fang", Base: "Wild Warg Chieftain", Role: RoleHero, Path: "Path of the Berserker", PathRules: 1, Level: 1, XP: 5},
			{ID: 8, Name: "Oliphaunt", Base: "Mumak War Leader", Role: RoleWarrior, Kind: KindMonstrousCavalry},
			{ID: 9, Base: "Werewolf", Role: RoleWarrior},
			{ID: 10, Name: "Muzgash", Base: "Orc Tracker", Role: RoleWarrior, Bought: []string{"Throwing Weapon"}, XP: 4, Injured: true},
		},
	}
}

func unitByID(t *testing.T, d CompanyDetail, id int64) CompanyUnit {
	t.Helper()
	for _, u := range d.Roster {
		if u.ID == id {
			return u
		}
	}
	t.Fatalf("unit %d is missing", id)
	return CompanyUnit{}
}

func ruleNamed(u CompanyUnit, name string) (UnitRule, bool) {
	for _, r := range u.Rules {
		if r.Name == name {
			return r, true
		}
	}
	return UnitRule{}, false
}

func TestBuildDetailResolvesTheDemoCompaniesAgainstTheRealRules(t *testing.T) {
	rs := shippedRules(t)
	for name, st := range map[string]CompanyState{"Minas Tirith": gondor(), "Mordor": mordor()} {
		d := BuildDetail(rs, CampaignSettings{}, st)
		if len(d.Roster) < 8 || d.Roster[0].Role != RoleLeader {
			t.Errorf("%s: roster %d, first %s", name, len(d.Roster), d.Roster[0].Role)
		}
		for _, u := range d.Roster {
			if len(u.Stats) < 9 || u.Points == 0 || len(u.Equipment) == 0 {
				t.Errorf("%s %s (%s): %d stats, %d points, %d items", name, u.Title(), u.Type, len(u.Stats), u.Points, len(u.Equipment))
			}
			hasM := false
			for _, a := range u.Stats {
				hasM = hasM || a.HeroOnly
			}
			if u.IsHero() != (u.Path != nil) {
				t.Errorf("%s %s: every hero, and only heroes, has a path", name, u.Title())
			}
			if u.Path != nil && len(u.Path.Rules) == 0 {
				t.Errorf("%s %s: the path %q gave no rules", name, u.Title(), u.Path.Name)
			}
			if u.Path != nil {
				for _, sp := range u.Path.Powers {
					if !sp.Spell || sp.Casting == 0 || sp.Range == 0 {
						t.Errorf("%s %s: path power %q lacks its casting value or range", name, u.Title(), sp.Name)
					}
				}
			}
			for _, r := range u.Rules {
				if r.Source == RuleFromPath {
					t.Errorf("%s %s: the path's rule %q is also among the unit's rules", name, u.Title(), r.Name)
				}
			}
			if hasM && !u.IsHero() {
				t.Errorf("%s %s: a regular unit shows hero attributes", name, u.Title())
			}
		}
	}
}

func TestBuildDetailPointsFollowTheBattleCompaniesRating(t *testing.T) {
	d := BuildDetail(shippedRules(t), CampaignSettings{}, gondor())
	// A warrior: its MESBG points (8) plus the option its wargear amounts to, plus its equipment.
	for id, want := range map[int64]int{
		10: 8,                                // a plain Warrior of Minas Tirith
		6:  8 + 1,                            // Cirion carries a bow, option "Bow" +1
		9:  8 + 15,                           // Mablung carries a Company Standard (equipment, rating 15)
		2:  8 + 15 + 5,                       // Hador: the warrior profile, Might, Will and Fate, and the rule his path gave; the spear is the profile's option
		1:  8 + 15 + 5 + 10 + 10 + 5 + 5 + 3, // Beregond: + Fight 5, Attack 2, two path rules, gear at the hero rating
	} {
		if got := unitByID(t, d, id).Points; got != want {
			t.Errorf("unit %d is worth %d, want %d", id, got, want)
		}
	}
}

func TestBuildDetailInjuredUnitsLeaveTheRatingButNotFullPoints(t *testing.T) {
	d := BuildDetail(shippedRules(t), CampaignSettings{}, gondor())
	total, injured, injuredPoints := 0, 0, 0
	for _, u := range d.Roster {
		total += u.Points
		if u.Injured {
			injured++
			injuredPoints += u.Points
		}
	}
	if injured != 2 || d.InjuredUnits != 2 || injuredPoints == 0 {
		t.Fatalf("two units are injured: %d, %d", injured, d.InjuredUnits)
	}
	if d.Points != total-injuredPoints || d.FullPoints != total {
		t.Errorf("Points %d (want %d), FullPoints %d (want %d)", d.Points, total-injuredPoints, d.FullPoints, total)
	}
	// A hero sent to the healers misses the next game as well.
	st := gondor()
	st.Units[4].SentToHealers = true
	if got := BuildDetail(shippedRules(t), CampaignSettings{}, st); got.InjuredUnits != 3 || !unitByID(t, got, 5).Injured {
		t.Errorf("sent to the healers: %d injured", got.InjuredUnits)
	}
}

func TestBuildDetailDoesNotChangeTheState(t *testing.T) {
	st := gondor()
	st.Units[0].Progress = &Progress{Taken: 1, Bonus: map[string]int{"fight": 1}, Rules: []string{"x"}}
	before := st.Clone()
	BuildDetail(shippedRules(t), CampaignSettings{}, st)
	if !equalStates(before, st) {
		t.Error("BuildDetail changed the state it was given")
	}
}

func equalStates(a, b CompanyState) bool {
	if len(a.Units) != len(b.Units) {
		return false
	}
	for i := range a.Units {
		x, y := a.Units[i], b.Units[i]
		if x.Level != y.Level || x.Path != y.Path || !slices.Equal(x.Bought, y.Bought) || !slices.Equal(x.Gear, y.Gear) ||
			len(x.Bonus) != len(y.Bonus) || len(x.Injuries) != len(y.Injuries) || (x.Progress == nil) != (y.Progress == nil) ||
			(x.Progress != nil && (x.Progress.Taken != y.Progress.Taken || len(x.Progress.Bonus) != len(y.Progress.Bonus))) {
			return false
		}
	}
	return true
}

func TestBuildDetailLevelUpsChangeTheProfile(t *testing.T) {
	rs := shippedRules(t)
	st := gondor()
	base := unitByID(t, BuildDetail(rs, CampaignSettings{}, st), 10)
	st.Units[9].Progress = &Progress{Taken: 1, Bonus: map[string]int{"fight": 1}}
	got := unitByID(t, BuildDetail(rs, CampaignSettings{}, st), 10)
	stat := func(u CompanyUnit, name string) Attribute {
		for _, a := range u.Stats {
			if a.Name == name {
				return a
			}
		}
		return Attribute{}
	}
	if got.Level != base.Level+1 || stat(got, "Fight").Value != stat(base, "Fight").Value+1 || stat(got, "Fight").Boost != 1 {
		t.Errorf("a level-up gave +1 Fight: level %d->%d, fight %+v -> %+v", base.Level, got.Level, stat(base, "Fight"), stat(got, "Fight"))
	}
}

func TestBuildDetailTheDataDrivesWhatTheCardsShow(t *testing.T) {
	rs := shippedRules(t)
	d1 := BuildDetail(rs, CampaignSettings{}, gondor())
	d2 := BuildDetail(rs, CampaignSettings{}, mordor())
	elphir := unitByID(t, d1, 5)
	var order []string
	for _, e := range elphir.Equipment {
		order = append(order, e.Name)
	}
	if len(order) < 3 || order[0] != "Horse" || order[1] != "Barding" {
		t.Errorf("the mount, then its armour: %v", order)
	}
	if r, ok := ruleNamed(elphir, "Expert Rider"); !ok || r.Mount != "" || r.From != "Barding" {
		t.Errorf("barding gives the rider Expert Rider: %+v %v", r, ok)
	}
	for _, e := range unitByID(t, d1, 11).Equipment {
		if e.Name == "Guardian of the King" && (!e.Upgrade || e.Drop == "") {
			t.Errorf("a unit upgrade: %+v", e)
		}
	}
	snaga := unitByID(t, d2, 2)
	modified := false
	for _, e := range snaga.Equipment {
		if e.Name == "Envenom Weapon (X)" && e.Modifies != "" {
			modified = true
		}
	}
	if _, ok := ruleNamed(snaga, "Poisoned Attacks (X)"); !modified || !ok {
		t.Errorf("a poisoned copy of the weapon: modified=%v rule=%v", modified, ok)
	}
	for _, e := range unitByID(t, d1, 3).Equipment {
		if e.Name == "Bow" && e.Stats != `24" · S2` {
			t.Errorf("a bow's profile: %q", e.Stats)
		}
	}
}

func TestBuildDetailTheOptionalRuleLetsHeroesDropProfileGear(t *testing.T) {
	rs := shippedRules(t)
	dropOf := func(settings CampaignSettings) (profile string, drop string) {
		d := BuildDetail(rs, settings, mordor())
		for _, e := range unitByID(t, d, 2).Equipment {
			if e.Profile && !e.Upgrade {
				return e.Name, e.Drop
			}
		}
		t.Fatal("Snaga carries profile gear")
		return
	}
	_, without := dropOf(CampaignSettings{})
	_, with := dropOf(CampaignSettings{HeroesDropProfileGear: true})
	if without == "" || with != "" {
		t.Errorf("a hero may drop profile gear only with the optional rule: without=%q with=%q", without, with)
	}
}

func TestBuildDetailHeroInjuriesChangeItsProfileAndGear(t *testing.T) {
	d := BuildDetail(shippedRules(t), CampaignSettings{}, gondor())
	faradir := unitByID(t, d, 4)
	if len(faradir.Injuries) != 2 {
		t.Fatalf("injuries: %+v", faradir.Injuries)
	}
	moved := false
	for _, a := range faradir.Stats {
		if a.Name == "Movement" {
			moved = a.Boost == -1
		}
	}
	if !moved {
		t.Errorf("a leg wound takes 1 off Movement: %+v", faradir.Stats)
	}
	disabled := ""
	for _, e := range faradir.Equipment {
		if e.Name == "Crossbow" {
			disabled = e.Disabled
		}
	}
	if disabled != "Arm Wound" {
		t.Errorf("an arm wound stops a hero firing a crossbow: %q", disabled)
	}
	elphir := unitByID(t, d, 5)
	_, fearful := ruleNamed(elphir, "Fearful")
	if !fearful || len(elphir.LostAbilities) == 0 {
		t.Errorf("a second broken honour makes a hero Fearful and takes abilities away: %+v", elphir.LostAbilities)
	}
}

func TestBuildDetailSavedOrderTakesPriority(t *testing.T) {
	rs := shippedRules(t)
	ids := func(st CompanyState) []int64 {
		var out []int64
		for _, u := range BuildDetail(rs, CampaignSettings{}, st).Roster {
			out = append(out, u.ID)
		}
		return out
	}
	st := gondor()
	if got := ids(st); got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("the default order is leader, sergeants, heroes, then the regular units: %v", got)
	}
	st.Order = []int64{6, 2, 3, 4, 5, 1}
	got := ids(st)
	if got[0] != 6 || got[5] != 1 || got[6] != 7 || got[len(got)-1] != 11 {
		t.Errorf("the saved order, then the others in the default order: %v", got)
	}
	st.Units = append(st.Units, UnitState{ID: 100, Base: "Warrior of Minas Tirith", Role: RoleWarrior})
	if after := ids(st); after[0] != 6 || after[len(after)-1] != 100 {
		t.Errorf("a new recruit joins at the end: %v", after)
	}
}

func TestBuildDetailCreaturesGoToOfficersAndTakeNoSlots(t *testing.T) {
	rs := shippedRules(t)
	before := BuildDetail(rs, CampaignSettings{}, gondor())
	st := gondor()
	st.Units[0].Creature = "Baggage Pony"
	after := BuildDetail(rs, CampaignSettings{}, st)
	if len(after.Creatures) != 1 || after.Creatures[0].OwnerID != 1 || after.Pack == nil || after.Pack.Capacity != 15 {
		t.Fatalf("the pony is the leader's and gives the pack: %+v", after.Creatures)
	}
	if after.Units != before.Units || len(after.Limits) != len(before.Limits) || after.Points != before.Points+5 || after.FullPoints != before.FullPoints+5 {
		t.Errorf("a creature is no unit: units %d->%d, points %d->%d", before.Units, after.Units, before.Points, after.Points)
	}
	if unitByID(t, after, 1).Creature == nil {
		t.Error("the owner's card shows it")
	}
}

func TestBuildDetailGearInThePackStillCountsTowardsTheBowLimit(t *testing.T) {
	rs := shippedRules(t)
	used := func(st CompanyState) int {
		for _, l := range BuildDetail(rs, CampaignSettings{}, st).Limits {
			if l.ID == "bow" {
				return l.Used
			}
		}
		return -1
	}
	st := gondor()
	st.Units[0].Creature = "Baggage Pony"
	before := used(st)
	st.Units[2].Bought = nil // Baranor stores his bow in the pack
	st.Pack = []string{"Bow"}
	if used(st) != before {
		t.Errorf("the stored bow still counts: %d then %d", before, used(st))
	}
}

func TestBuildDetailTheFallenRestInTheirOwnCemetery(t *testing.T) {
	rs := shippedRules(t)
	st := gondor()
	bcCo, _ := rs.BC.Company(st.Company.Faction)
	for i, f := range []struct {
		spec  UnitState
		fate  string
		match string
	}{
		{UnitState{ID: 201, Name: "Hurin", Base: "Warrior of Minas Tirith", Role: RoleHero, Bonus: HeroBonus(map[string]int{"fight": 1}), Bought: []string{"Spear"}, Path: "Path of the Soldier", PathRules: 1, Level: 2, XP: 11}, "Dead: the Hero's adventure ended there", "Match 2"},
		{UnitState{ID: 202, Base: "Warrior of Minas Tirith", Gear: []string{"Bow"}, Level: 1, XP: 6}, "Dead", "Match 3"},
	} {
		_ = i
		st.Cemetery = append(st.Cemetery, FallenUnit{Unit: buildUnit(rs, bcCo, f.spec, CampaignSettings{}), Fate: f.fate, Match: f.match})
	}
	d := BuildDetail(rs, CampaignSettings{}, st)
	if len(d.Cemetery) != 2 || d.Cemetery[0].Unit.ID != 201 {
		t.Fatalf("cemetery: %+v", d.Cemetery)
	}
	for _, f := range d.Cemetery {
		for _, u := range d.Roster {
			if u.ID == f.Unit.ID {
				t.Errorf("a fallen unit is not in the roster: %d", u.ID)
			}
		}
		if f.Fate == "" || f.Unit.Points == 0 {
			t.Errorf("it shows how it ended and what it was: %+v", f)
		}
	}
	st.Graveyard = []int64{202, 201}
	if d := BuildDetail(rs, CampaignSettings{}, st); d.Cemetery[0].Unit.ID != 202 {
		t.Errorf("the saved order is kept: %d", d.Cemetery[0].Unit.ID)
	}
}

func TestBuildDetailAWargMarauderIsOneCavalryModelThatCountsAsThree(t *testing.T) {
	rs := shippedRules(t)
	st := CompanyState{Company: CampaignCompany{ID: 1, Faction: "Mirkwood"}, Units: []UnitState{{ID: 1, Base: "Warg Marauder", Role: RoleWarrior}}}
	d := BuildDetail(rs, CampaignSettings{}, st)
	m := d.Roster[0]
	if m.ModelCount() != 3 || len(m.Parts) != 3 || m.Kind != KindCavalry || !m.OwnPath {
		t.Errorf("three riders and a mount on one cavalry card, on its own path: %+v", m)
	}
	if m.Path == nil || m.Path.Name != "On Their Own Path" || len(m.Path.Rules) != 1 || m.Path.Rules[0].Description == "" {
		t.Errorf("the end of the path is explained: %+v", m.Path)
	}
	if d.ModelCount() != 3 || d.Units != 3 {
		t.Errorf("it takes three places of the company's models: %d / %d", d.ModelCount(), d.Units)
	}
}

func TestBuildDetailAHobbitProfileWithItsOptionIsWorthItsPoints(t *testing.T) {
	rs := shippedRules(t)
	bcCo, ok := rs.BC.Company("Wanderers in the Wild")
	if !ok {
		t.Skip("no Wanderers in the Wild company")
	}
	var chosen ChoiceUnit
	for _, o := range bcCo.Reinforcements[1].Result.Options {
		if o.Label == "Tookish Hunter" {
			chosen = o
		}
	}
	res := optionResult(chosen)
	st := CompanyState{Company: CampaignCompany{ID: 1, Faction: bcCo.Name}, Units: []UnitState{{ID: 1, Base: res.Unit, Gear: res.Wargear, Role: RoleWarrior}}}
	d := BuildDetail(rs, CampaignSettings{}, st)
	if hunter := d.Roster[0]; hunter.Type != "Tookish Hunter" || hunter.Points != 6 {
		t.Fatalf("a Tookish Hunter is a Hobbit Archer (5) with its option (1): %+v", hunter)
	}
	if why, _ := rareBlock(d.Roster, "Tookish Hunter", 1); why == "" {
		t.Error("only one Tookish Hunter")
	}
}

func TestCompanyBlockKeepsToKeywordLimitsAndBalance(t *testing.T) {
	rs := shippedRules(t)
	roster := func(types ...string) []CompanyUnit {
		var out []CompanyUnit
		for _, ty := range types {
			out = append(out, CompanyUnit{Type: ty})
		}
		return out
	}
	helm, _ := rs.BC.Company("Helm's Deep")
	mixed := func(elves, men int) []CompanyUnit {
		var types []string
		for range elves {
			types = append(types, "Galadhrim Warrior")
		}
		for range men {
			types = append(types, "Warrior of Rohan")
		}
		return roster(types...)
	}
	// the limit rounds up and the Elf that joins counts in numerator and denominator: 33% of 7 is 3, of 4 is 2
	if why, _ := companyBlock(rs, helm, mixed(2, 4), "Galadhrim Warrior"); why != "" {
		t.Errorf("a 3rd Elf in a company of 7 may join: %s", why)
	}
	if why, _ := companyBlock(rs, helm, mixed(3, 3), "Galadhrim Warrior"); why == "" {
		t.Error("a 4th Elf in a company of 7 may not join")
	}
	if why, _ := companyBlock(rs, helm, mixed(3, 8), "Galadhrim Warrior"); why != "" {
		t.Errorf("a 4th Elf in a company of 12 may join: %s", why)
	}
	if why, _ := companyBlock(rs, helm, mixed(3, 3), "Warrior of Rohan"); why != "" {
		t.Errorf("others are not counted: %s", why)
	}
	defenders, _ := rs.BC.Company("Defenders of the North")
	if why, kind := companyBlock(rs, defenders, roster("Warrior of Dale", "Iron Hills Warrior"), "Iron Hills Warrior"); why == "" || kind != blockBalance {
		t.Errorf("Dwarves may not outnumber Dale: %q", why)
	}
	if why, _ := companyBlock(rs, defenders, roster("Warrior of Dale", "Warrior of Dale", "Iron Hills Warrior"), "Iron Hills Warrior"); why != "" {
		t.Errorf("as many as Dale is allowed: %s", why)
	}
}

func TestStartingUnitsAndWithPath(t *testing.T) {
	rs := shippedRules(t)
	profile, ok := rs.BC.Profile("Minas Tirith")
	if !ok {
		t.Skip("no profile")
	}
	heroes := profile.DefaultHeroes()
	units := StartingUnits(profile, heroes)
	if len(units) != len(profile.StartList()) || units[0].ID != 1 {
		t.Fatalf("starting units: %d", len(units))
	}
	chosen := 0
	for _, u := range units {
		if u.Role != RoleWarrior {
			chosen++
			if u.Bonus["might"] != 1 && profile.StartGains(u.Base) {
				t.Errorf("an officer starts with a point of Might, Will and Fate: %+v", u.Bonus)
			}
		}
	}
	if chosen == 0 {
		t.Error("a starting company has officers")
	}
	paths := rs.BC.Progression()
	u := WithPath(UnitState{ID: 1, Base: "Warrior of Minas Tirith", Role: RoleLeader}, "Path of the Warlord", "", paths)
	if u.Path != "Path of the Warlord" {
		t.Errorf("path: %q", u.Path)
	}
	d := BuildDetail(rs, CampaignSettings{}, CompanyState{Company: CampaignCompany{Faction: "Minas Tirith"}, Units: []UnitState{u}})
	if d.Roster[0].PathPending || d.Roster[0].Path == nil || d.Roster[0].Path.Name != "Path of the Warlord" {
		t.Errorf("the officer has its path: %+v", d.Roster[0].Path)
	}
	pending := BuildDetail(rs, CampaignSettings{}, CompanyState{Company: CampaignCompany{Faction: "Minas Tirith"}, Units: []UnitState{{ID: 1, Base: "Warrior of Minas Tirith", Role: RoleLeader, Bonus: HeroBonus(nil)}}})
	if !pending.Roster[0].PathPending {
		t.Error("an officer without a path is pending")
	}
}

func TestCompanyStateCloneIsDeep(t *testing.T) {
	st := gondor()
	st.Units[0].Progress = &Progress{Bonus: map[string]int{"fight": 1}, Rules: []string{"a"}}
	c := st.Clone()
	c.Units[0].Bought[0] = "changed"
	c.Units[0].Bonus["fight"] = 99
	c.Units[0].Progress.Bonus["fight"] = 99
	c.Units[0].Progress.Rules[0] = "b"
	if st.Units[0].Bought[0] == "changed" || st.Units[0].Bonus["fight"] == 99 || st.Units[0].Progress.Bonus["fight"] == 99 || st.Units[0].Progress.Rules[0] == "b" {
		t.Error("Clone shares memory with the original")
	}
}

func TestLancesAndBardingRequireAMountAndTheWarSpearDoesNot(t *testing.T) {
	rs := shippedRules(t)
	for name, want := range map[string]bool{"Lance": true, "Elven lance": true, "Barding": true, "War spear": false, "Spear": false, "Horse": false} {
		e, _ := equipmentFor(rs, BCCompany{}, name, true, false, false)
		if e.RequiresMount != want {
			t.Errorf("%s: RequiresMount = %v, want %v", name, e.RequiresMount, want)
		}
	}
}

// A company stored as "Lake-Town" (the spelling before the official Lake-town) still resolves.
func TestCompanyNamesResolveIgnoringCase(t *testing.T) {
	rs := shippedRules(t)
	want, ok := rs.BC.Company("Lake-town")
	if !ok {
		t.Fatal("the rules have Lake-town")
	}
	for _, spelling := range []string{"Lake-Town", "LAKE-TOWN", "lake-town"} {
		got, ok := rs.BC.Company(spelling)
		if !ok || got.Name != want.Name {
			t.Errorf("%s: %+v %v", spelling, got.Name, ok)
		}
		if p, ok := rs.BC.Profile(spelling); !ok || p.Name != "Lake-town" {
			t.Errorf("profile %s: %v", spelling, ok)
		}
	}
	if _, ok := rs.BC.Company("Lake Town"); ok {
		t.Error("only the case differs")
	}
}
