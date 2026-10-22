package core

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// recruiter is a recruit service over the fixture's company, with the dice fixed and typed rolls allowed.
func (f *companyFixture) recruiter(t *testing.T, dice ...int) *RecruitService {
	t.Helper()
	f.update()
	f.settings.ManualRolls = true
	return NewRecruitService(f.campaigns, f.companies, staticRules{shippedRules(t)}, &FixedDice{Rolls: dice})
}

// companyOf makes the fixture a company of that faction with these units.
func (f *companyFixture) companyOf(faction string, influence int, units []UnitState) {
	f.state = CompanyState{Company: CampaignCompany{ID: 1, Name: "Test", Faction: faction}, Influence: influence, Units: units}
}

func recruit(t *testing.T, s *RecruitService, f *companyFixture, roll, adjust int) (RecruitResult, error) {
	t.Helper()
	if err := s.RollRecruit(f.ctx, f.user, 5, roll, false); err != nil {
		return RecruitResult{}, err
	}
	return s.ConfirmRecruit(f.ctx, f.user, 5, adjust, "")
}

func TestRecruitingRollsOnTheChartAndCostsInfluence(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.recruiter(t)
	before := BuildDetail(shippedRules(t), f.settings, f.state)
	if before.Recruit.Cost != 3 || before.MaxUnits != 15 || len(before.Recruit.Chart) == 0 || len(before.Recruit.SpecialChart) == 0 {
		t.Errorf("Minas Tirith recruits for 3 influence up to 15 units, with a special chart of its own: %+v", before.Recruit)
	}
	if err := s.RollRecruit(f.ctx, f.user, 5, 4, false); err != nil {
		t.Fatal(err)
	}
	if f.state.Influence != 9 || len(f.state.Units) != len(before.Roster) {
		t.Errorf("the price is paid, the unit has not joined: influence %d, units %d", f.state.Influence, len(f.state.Units))
	}
	if state, _ := s.RecruitState(f.ctx, f.user, 5); state == nil || state.Roll != 4 || state.Budget != 2 || state.Stage != "chart" {
		t.Errorf("the recruit in progress: %+v", state)
	}
	if err := s.RollRecruit(f.ctx, f.user, 5, 2, false); !errors.Is(err, ErrRefused) {
		t.Errorf("no second roll before confirming: %v", err)
	}
	res, err := s.ConfirmRecruit(f.ctx, f.user, 5, 0, "") // a 4: a Warrior of Minas Tirith with a spear and shield
	if err != nil || !strings.Contains(res.Summary, "Warrior of Minas Tirith") {
		t.Fatalf("%+v %v", res, err)
	}
	if len(f.state.Units) != len(before.Roster)+1 || f.state.Influence != 9 {
		t.Errorf("units %d -> %d, influence 12 -> %d", len(before.Roster), len(f.state.Units), f.state.Influence)
	}
	joined := f.state.Units[len(f.state.Units)-1]
	if joined.Role != RoleWarrior || joined.Base != "Warrior of Minas Tirith" || !slices.Equal(joined.Gear, []string{"Spear", "Shield"}) && len(joined.Gear) == 0 {
		t.Errorf("the recruit: %+v", joined)
	}
	if state, _ := s.RecruitState(f.ctx, f.user, 5); state != nil {
		t.Error("the recruit is over")
	}
	f.state.Influence = 2
	if err := s.RollRecruit(f.ctx, f.user, 5, 4, false); !errors.Is(err, ErrRefused) {
		t.Errorf("not enough influence: %v", err)
	}
	if f.state.Influence != 2 || f.state.Recruit.Active() {
		t.Error("a refused roll costs nothing and starts nothing")
	}
	if err := s.RollRecruit(f.ctx, User{ID: 8}, 5, 4, false); !errors.Is(err, ErrNotFound) {
		t.Errorf("non-owner: %v", err)
	}
}

func TestTheAppRollsWhenNoRollIsGiven(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.recruiter(t, 5)
	f.settings.ManualRolls = false // the server's own roll is always fine
	if err := s.RollRecruit(f.ctx, f.user, 5, 0, false); err != nil {
		t.Fatal(err)
	}
	if f.state.Recruit.Roll == nil || f.state.Recruit.Roll.Roll != 5 {
		t.Errorf("the dice gave a 5: %+v", f.state.Recruit.Roll)
	}
}

func TestRollsNeedTheCampaignSettings(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.recruiter(t)
	f.settings = CampaignSettings{ShakeTheTable: true} // typed rolls off
	if err := s.RollRecruit(f.ctx, f.user, 5, 4, false); !errors.Is(err, ErrRefused) {
		t.Errorf("a typed recruit roll needs the setting: %v", err)
	}
	if err := s.RollRecruit(f.ctx, f.user, 5, 4, true); err != nil {
		t.Errorf("a thrown roll with Shake the table: %v", err)
	}
	f2 := newCompanyFixture(t)
	s2 := f2.recruiter(t)
	f2.settings = CampaignSettings{}
	if err := s2.RollRecruit(f2.ctx, f2.user, 5, 4, true); !errors.Is(err, ErrRefused) {
		t.Errorf("a thrown roll without it: %v", err)
	}
}

func TestAdjustingARollAfterSeeingItCostsInfluence(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.recruiter(t)
	f.state.Influence = 5
	if err := s.RollRecruit(f.ctx, f.user, 5, 1, false); err != nil { // a 1 is nothing: the player sees it first...
		t.Fatal(err)
	}
	res, err := s.ConfirmRecruit(f.ctx, f.user, 5, 2, "") // ...and buys +2: a 3, a Warrior of Minas Tirith with a bow
	if err != nil || !strings.Contains(res.Summary, "raised to a 3") || !strings.Contains(res.Summary, "(Bow)") {
		t.Fatalf("%+v %v", res, err)
	}
	if f.state.Influence != 0 {
		t.Errorf("5 influence, 3 for the roll and 2 for the adjustment: %d left", f.state.Influence)
	}
	f.state.Influence = 5
	if err := s.RollRecruit(f.ctx, f.user, 5, 3, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfirmRecruit(f.ctx, f.user, 5, 3, ""); !errors.Is(err, ErrRefused) {
		t.Errorf("2 influence left, +3 is more than it can pay: %v", err)
	}
	if state, _ := s.RecruitState(f.ctx, f.user, 5); state == nil || f.state.Influence != 2 {
		t.Error("a refused confirmation leaves the recruit in progress, the roll paid")
	}
	// the die is only ever raised
	res, err = s.ConfirmRecruit(f.ctx, f.user, 5, -3, "")
	if err != nil || !strings.Contains(res.Summary, "You rolled a 3: ") {
		t.Errorf("a negative adjustment is ignored: %+v %v", res, err)
	}
}

func TestAdjustingNeverPaysPastASix(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.recruiter(t)
	f.state.Influence = 6
	if err := s.RollRecruit(f.ctx, f.user, 5, 5, false); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.RecruitState(f.ctx, f.user, 5); st == nil || st.Budget != 1 {
		t.Errorf("a 5 can be raised by 1 at most: %+v", st)
	}
	if _, err := s.ConfirmRecruit(f.ctx, f.user, 5, 3, ""); err != nil {
		t.Fatal(err)
	}
	if f.state.Influence != 2 { // 6 - 3 (roll) - 1 (the only point the die can move)
		t.Errorf("influence for points past a 6 is not taken: %d left", f.state.Influence)
	}
}

func TestNothingStillCosts(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.recruiter(t)
	if res, err := recruit(t, s, f, 1, 0); err != nil || !strings.Contains(res.Summary, "nothing (−3 influence)") {
		t.Errorf("a 1 gives nothing but still costs: %+v %v", res, err)
	}
}

func TestTheSpecialChartIsASecondRollInTheSameRecruit(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.recruiter(t)
	f.state.Influence = 5
	if err := s.RollRecruit(f.ctx, f.user, 5, 5, false); err != nil { // a 5, adjusted +1 to a 6: the special chart
		t.Fatal(err)
	}
	res, err := s.ConfirmRecruit(f.ctx, f.user, 5, 1, "")
	if err != nil || !res.Next {
		t.Fatalf("a 6 leads on to the special chart: %+v %v", res, err)
	}
	state, _ := s.RecruitState(f.ctx, f.user, 5)
	if state == nil || state.Stage != "special" || state.Rolled || state.Budget != 2 {
		t.Fatalf("the special chart waits for its roll, with the adjustment left over: %+v", state)
	}
	if _, err := s.ConfirmRecruit(f.ctx, f.user, 5, 0, ""); !errors.Is(err, ErrRefused) {
		t.Errorf("nothing to confirm before rolling: %v", err)
	}
	if err := s.RollRecruit(f.ctx, f.user, 5, 4, false); err != nil { // a 4 on the special chart: a Knight of Minas Tirith
		t.Fatal(err)
	}
	res, err = s.ConfirmRecruit(f.ctx, f.user, 5, 0, "")
	if err != nil || res.Next || !strings.Contains(res.Summary, "Knight of Minas Tirith") || !strings.Contains(res.Summary, "special chart") {
		t.Fatalf("%+v %v", res, err)
	}
	if f.state.Influence != 1 { // 5 - 3 (roll) - 1 (adjustment); the second roll is free
		t.Errorf("influence %d", f.state.Influence)
	}
}

func TestAChoiceOfOptionsWaitsForTheChoice(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.recruiter(t)
	units := len(f.state.Units)
	res, err := recruit(t, s, f, 5, 0) // a 5: a Warrior of Minas Tirith, you choose its options
	if err != nil || res.Choice == nil || res.Choice.Unit != "Warrior of Minas Tirith" {
		t.Fatalf("%+v %v", res, err)
	}
	for _, o := range res.Choice.Options {
		if strings.Contains(strings.ToLower(o.Name), "banner") || strings.Contains(strings.ToLower(o.Name), "horn") {
			t.Errorf("no banner or war horn options: %q", o.Name)
		}
	}
	if len(res.Choice.Options) == 0 {
		t.Fatal("the profile has options")
	}
	if len(f.state.Units) != units {
		t.Errorf("the unit joins only once chosen: %d units", len(f.state.Units))
	}
	if pending, _ := s.PendingChoice(f.ctx, f.user, 5); pending == nil || pending.Unit != "Warrior of Minas Tirith" {
		t.Error("a choice is pending")
	}
	if err := s.RollRecruit(f.ctx, f.user, 5, 2, false); !errors.Is(err, ErrRefused) {
		t.Errorf("no new recruit while one waits: %v", err)
	}
	if _, err := s.ChooseOptions(f.ctx, f.user, 5, "Nonsense"); !errors.Is(err, ErrRefused) {
		t.Errorf("an option the profile lacks: %v", err)
	}
	chosen := res.Choice.Options[0].Name
	if _, err := s.ChooseOptions(f.ctx, f.user, 5, chosen); err != nil {
		t.Fatal(err)
	}
	if len(f.state.Units) != units+1 {
		t.Errorf("the unit has joined: %d units", len(f.state.Units))
	}
	if pending, _ := s.PendingChoice(f.ctx, f.user, 5); pending != nil {
		t.Error("nothing is pending any more")
	}
	if _, err := s.ChooseOptions(f.ctx, f.user, 5, ""); !errors.Is(err, ErrRefused) {
		t.Errorf("nothing to choose: %v", err)
	}
}

func TestPendingRecruitSurvivesBetweenRequests(t *testing.T) {
	// the state is all there is: a second service instance (another request, another server) continues the recruit
	f := newCompanyFixture(t)
	s := f.recruiter(t)
	if err := s.RollRecruit(f.ctx, f.user, 5, 6, false); err != nil {
		t.Fatal(err)
	}
	other := NewRecruitService(f.campaigns, f.companies, staticRules{shippedRules(t)}, &FixedDice{})
	if res, err := other.ConfirmRecruit(f.ctx, f.user, 5, 0, ""); err != nil || !res.Next {
		t.Errorf("continued: %+v %v", res, err)
	}
}

func TestARareUnitCannotJoinOnceTheCompanyHasAsManyAsItMayHave(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.recruiter(t)
	f.companyOf("Moria", 30, warriors("Moria Goblin Warrior", 5))
	d := BuildDetail(shippedRules(t), f.settings, f.state)
	shown := false
	for _, sp := range d.Recruit.Special {
		shown = shown || sp.Unit == "Cave Troll" && sp.Rare == 1 && sp.Blocked == ""
	}
	if !shown {
		t.Fatal("the first Cave Troll may join, and the special unit says it is rare")
	}
	if _, err := s.RecruitSpecial(f.ctx, f.user, 5, "Cave Troll"); err != nil {
		t.Fatal(err)
	}
	if f.state.Influence != 15 {
		t.Errorf("a Cave Troll costs 15 influence: %d left", f.state.Influence)
	}
	if f.state.Recruit.Choice == nil {
		t.Error("a Cave Troll comes with a choice of options")
	}
	if _, err := s.ChooseOptions(f.ctx, f.user, 5, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecruitSpecial(f.ctx, f.user, 5, "Cave Troll"); !errors.Is(err, ErrRefused) {
		t.Errorf("a second one may not: %v", err)
	}
	if _, err := s.RecruitSpecial(f.ctx, f.user, 5, "Mumak"); !errors.Is(err, ErrNotFound) {
		t.Errorf("not one of the company's special units: %v", err)
	}
	f.state.Influence = 3
	f.state.Units = f.state.Units[:5]
	if _, err := s.RecruitSpecial(f.ctx, f.user, 5, "Cave Troll"); !errors.Is(err, ErrRefused) {
		t.Errorf("not enough influence: %v", err)
	}
}

func TestALowerResultIsOfferedOnlyWhenTheRollCannotJoin(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.recruiter(t)
	// Defenders of the North: a Dwarf for every Dale, another Dwarf may not join
	f.companyOf("Defenders of the North", 12, []UnitState{
		{ID: 1, Base: "Warrior of Dale", Role: RoleWarrior}, {ID: 2, Base: "Iron Hills Warrior", Role: RoleWarrior}})
	rs := shippedRules(t)
	defenders, _ := rs.BC.Company("Defenders of the North")
	var dwarfRoll int
	for _, row := range defenders.Reinforcements {
		if row.Result.Type == "unit" && row.Result.Unit == "Iron Hills Warrior" {
			dwarfRoll = row.Max
		}
	}
	if dwarfRoll < 2 {
		t.Fatal("the chart gives an Iron Hills Warrior above the lowest result")
	}
	res, err := recruit(t, s, f, dwarfRoll, 0)
	if err != nil || !res.Lower {
		t.Fatalf("a Dwarf may not outnumber Dale: %+v %v", res, err)
	}
	if f.state.Influence != 9 {
		t.Errorf("the roll is paid: %d", f.state.Influence)
	}
	state, _ := s.RecruitState(f.ctx, f.user, 5)
	if state == nil || state.Lower == nil || len(state.Lower.Rows) == 0 {
		t.Fatalf("the lower results are offered: %+v", state)
	}
	free := -1
	for _, r := range state.Lower.Rows {
		if r.Roll == strconv.Itoa(dwarfRoll) || strings.HasSuffix(r.Roll, "–"+strconv.Itoa(dwarfRoll)) {
			t.Errorf("only lower results are offered: %s", r.Roll)
		}
		if r.Blocked == "" && r.Text != "Nothing" && free < 0 {
			free = r.Index
		}
	}
	if free < 0 {
		t.Fatal("a lower result that can join exists")
	}
	if err := s.RollRecruit(f.ctx, f.user, 5, 2, false); !errors.Is(err, ErrRefused) {
		t.Errorf("no new recruit while the player chooses: %v", err)
	}
	if _, err := s.ChooseLower(f.ctx, f.user, 5, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown row: %v", err)
	}
	units := len(f.state.Units)
	res, err = s.ChooseLower(f.ctx, f.user, 5, free)
	if err != nil || !strings.Contains(res.Summary, "you choose the lower result") {
		t.Fatalf("%+v %v", res, err)
	}
	if len(f.state.Units) != units+1 && f.state.Recruit.Choice == nil {
		t.Errorf("the lower result joined: %d units", len(f.state.Units))
	}
	if f.state.Recruit.Lower != nil {
		t.Error("the choice is made")
	}
	if _, err := s.ChooseLower(f.ctx, f.user, 5, 0); !errors.Is(err, ErrRefused) {
		t.Errorf("nothing waits: %v", err)
	}
}

func TestAVaultWardenTeamJoinsAsTwoUnitsOfTheirOwn(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.recruiter(t)
	f.companyOf("Durin's Folk", 30, warriors("Iron Hills Warrior", 3))
	var teamRoll int
	rs := shippedRules(t)
	co, _ := rs.BC.Company("Durin's Folk")
	for _, row := range co.SpecialChart {
		if row.Result.Unit == "Vault Warden Team" {
			teamRoll = row.Min
		}
	}
	f.state.Recruit.Roll = &RecruitStep{Stage: "special"}
	if err := s.RollRecruit(f.ctx, f.user, 5, teamRoll, false); err != nil {
		t.Fatal(err)
	}
	res, err := s.ConfirmRecruit(f.ctx, f.user, 5, 0, "")
	if err != nil || !strings.Contains(res.Summary, "Iron Shield Bearer and Foe Spear Wielder") {
		t.Fatalf("both join: %+v %v", res, err)
	}
	d := BuildDetail(rs, f.settings, f.state)
	points := map[string]int{}
	for _, u := range d.Roster {
		points[u.Type] = u.Points
	}
	if points["Iron Shield Bearer"] != 13 || points["Foe Spear Wielder"] != 12 {
		t.Errorf("they are worth 13 and 12 points: %v", points)
	}

	// the team is rare 1: a second team cannot join while the first is in the company
	f.state.Recruit.Roll = &RecruitStep{Stage: "special"}
	if err := s.RollRecruit(f.ctx, f.user, 5, teamRoll, false); err != nil {
		t.Fatal(err)
	}
	units := len(f.state.Units)
	if res, err := s.ConfirmRecruit(f.ctx, f.user, 5, 0, ""); err != nil || !res.Lower || len(f.state.Units) != units {
		t.Fatalf("a second Vault Warden Team is refused as rare: %+v %v units %d to %d", res, err, units, len(f.state.Units))
	}
	f.state.Recruit.Lower = nil

	// over the maximum it cannot join and any result of both charts can be taken instead
	f.companyOf("Durin's Folk", 30, warriors("Iron Hills Warrior", 11)) // the maximum is 12: one place is left
	f.state.Recruit.Roll = &RecruitStep{Stage: "special"}
	if err := s.RollRecruit(f.ctx, f.user, 5, teamRoll, false); err != nil {
		t.Fatal(err)
	}
	res, err = s.ConfirmRecruit(f.ctx, f.user, 5, 0, "")
	if err != nil || !res.Lower {
		t.Fatalf("a team of two does not fit in the last place: %+v %v", res, err)
	}
	charts := map[string]bool{}
	for _, e := range f.state.Recruit.Lower.Entries {
		charts[e.Chart] = true
	}
	if !charts["Reinforcements"] || !charts["Special chart"] {
		t.Errorf("any result of both charts is offered: %v", charts)
	}
}

func TestAMissingTeamMemberCanBeTakenInsteadOfASpecialChartResult(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.recruiter(t)
	f.companyOf("Durin's Folk", 30, append(warriors("Iron Hills Warrior", 3), UnitState{ID: 9, Base: "Iron Shield Bearer", Role: RoleWarrior, Points: 13}))
	f.state.Recruit.Roll = &RecruitStep{Stage: "special"}
	if err := s.RollRecruit(f.ctx, f.user, 5, 1, false); err != nil {
		t.Fatal(err)
	}
	state, _ := s.RecruitState(f.ctx, f.user, 5)
	if state == nil || state.Replace != "Foe Spear Wielder" {
		t.Fatalf("the Foe Spear Wielder is missing: %+v", state)
	}
	units := len(f.state.Units)
	res, err := s.ConfirmRecruit(f.ctx, f.user, 5, 0, "member")
	if err != nil || !strings.Contains(res.Summary, "Foe Spear Wielder") || len(f.state.Units) != units+1 {
		t.Fatalf("%+v %v", res, err)
	}
	if f.state.Units[units].Points != 0 && f.state.Units[units].Base != "Foe Spear Wielder" {
		t.Errorf("the replacement: %+v", f.state.Units[units])
	}
	// complete again: nothing is missing
	f.state.Recruit.Roll = &RecruitStep{Stage: "special"}
	_ = s.RollRecruit(f.ctx, f.user, 5, 1, false)
	if state, _ := s.RecruitState(f.ctx, f.user, 5); state == nil || state.Replace != "" {
		t.Errorf("nothing is missing now: %+v", state)
	}
}

func TestFornostMayTakeAHobbitArcherInsteadOnA2To6(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.recruiter(t)
	f.companyOf("Fornost", 12, warriors("Warrior of Arnor", 3))
	d := BuildDetail(shippedRules(t), f.settings, f.state)
	for _, r := range d.Recruit.Chart {
		if (r.Min >= 2) != (r.Alt == "Hobbit Archer") {
			t.Errorf("the Hobbit Archer is the other option of 2 to 6 only: %+v", r)
		}
	}
	if err := s.RollRecruit(f.ctx, f.user, 5, 4, false); err != nil {
		t.Fatal(err)
	}
	res, err := s.ConfirmRecruit(f.ctx, f.user, 5, 0, "alt")
	if err != nil || !strings.Contains(res.Summary, "Hobbit Archer") || f.state.Units[3].Base != "Hobbit Archer" || f.state.Influence != 12-3 {
		t.Fatalf("%+v %v influence %d", res, err, f.state.Influence)
	}
}

func TestAWargMarauderTakesThreePlaces(t *testing.T) {
	f := newCompanyFixture(t)
	f.companyOf("Mirkwood", 12, warriors("Mirkwood Hunting Spider", 11)) // 15 places: 4 left, 3 are taken by a Marauder
	rs := shippedRules(t)
	env := newRecruitEnv(rs, f.settings, f.state)
	if why, _ := env.block(Reinforcement{Type: "unit", Unit: "Warg Marauder"}); why != "" {
		t.Fatalf("it fits in the four places left: %s", why)
	}
	f.state.Units = append(f.state.Units, warriors("Mirkwood Hunting Spider", 2)...)
	env = newRecruitEnv(rs, f.settings, f.state)
	if why, kind := env.block(Reinforcement{Type: "unit", Unit: "Warg Marauder"}); why == "" || kind != blockModels {
		t.Errorf("two places left, it needs three: %q %q", why, kind)
	}
}

func TestAResultOfTwoModelsAddsBothAndNeedsRoomForBoth(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.recruiter(t)
	f.companyOf("The Shire", 12, warriors("Hobbit Militia", 3))
	res, err := recruit(t, s, f, 5, 0) // a 5: two Hobbit Militia
	if err != nil || len(f.state.Units) != 5 || !strings.Contains(res.Summary, "2 × Hobbit Militia") {
		t.Fatalf("%+v %v units %d", res, err, len(f.state.Units))
	}
	f.companyOf("The Shire", 12, warriors("Hobbit Militia", 24)) // the maximum is 25
	if res, err := recruit(t, s, f, 5, 0); err != nil || !res.Lower {
		t.Errorf("two models do not fit in the last place: %+v %v", res, err)
	}
}

func TestAPairThatDoesNotFitOffersASingleOfTheSameUnit(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.recruiter(t)
	f.companyOf("Lake-town", 12, warriors("Lake-town Militia", 19)) // the maximum is 20: one place is left
	res, err := recruit(t, s, f, 6, 0)                              // 2 x Lake-town Guard
	if err != nil || !res.Lower {
		t.Fatalf("a pair does not fit in the last place: %+v %v", res, err)
	}
	single := false
	for _, e := range f.state.Recruit.Lower.Entries {
		if e.Res.Unit == "Lake-town Guard" && e.Res.Count == 1 {
			single = true
		}
	}
	if !single {
		t.Errorf("a single Lake-town Guard (a 4) is a lower result that fits: %+v", f.state.Recruit.Lower.Entries)
	}
}

func TestAChoiceResultLetsThePlayerTakeAnyResultOfTheChart(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.recruiter(t)
	f.companyOf("The Shire", 12, warriors("Hobbit Militia", 3)) // a 6: choose any 2 Hobbits from above
	res, err := recruit(t, s, f, 6, 0)
	if err != nil || !res.Lower || f.state.Recruit.Lower == nil || f.state.Recruit.Lower.Picks != 2 {
		t.Fatalf("a choice of two results: %+v %v %+v", res, err, f.state.Recruit.Lower)
	}
	units := len(f.state.Units)
	first := -1
	for i, e := range f.state.Recruit.Lower.Entries {
		if e.Res.Unit == "Hobbit Archer" {
			first = i
		}
	}
	if first < 0 {
		t.Fatal("the chart's Hobbit Archer is offered")
	}
	res, err = s.ChooseLower(f.ctx, f.user, 5, first)
	if err == nil && (strings.Contains(res.Summary, "lower result") || !strings.Contains(res.Summary, "Hobbit Archer joins")) {
		t.Errorf("a picked result is not a lower one and the pick is named: %q", res.Summary)
	}
	if err != nil || len(f.state.Units) != units+1 || f.state.Recruit.Lower == nil || f.state.Recruit.Lower.Picks != 1 {
		t.Fatalf("the first pick joins, one is left: %+v %v", res, err)
	}
	res, err = s.ChooseLower(f.ctx, f.user, 5, first)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Summary, "Hobbit Archer joins the company") || !strings.Contains(res.Summary, ". Then a Hobbit Archer joins") || strings.Contains(res.Summary, "and;") {
		t.Errorf("both picks are named in order: %q", res.Summary)
	}
	if res, _ := s.RecruitState(f.ctx, f.user, 5); res != nil {
		t.Errorf("the recruit is over: %+v", res)
	}
	if len(f.state.Units) != units+2 || f.state.Recruit.Lower != nil || f.state.Influence != 10 {
		t.Errorf("both joined for the one price: units %d, influence %d", len(f.state.Units), f.state.Influence)
	}
}

func TestAChoiceWithNothingToChooseEndsTheRecruit(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.recruiter(t)
	var units []UnitState
	for _, base := range []string{"Hobbit Militia", "Hobbit Shirriff", "Tookish Hunter", "Battlin' Brandybuck", "Warrior of Numenor", "Warrior of Numenor", "Warrior of Numenor", "Dwarf Ranger", "Lothlorien Warrior"} {
		units = append(units, UnitState{ID: int64(len(units) + 1), Base: base, Role: RoleWarrior})
	}
	f.companyOf("Wanderers in the Wild", 12, units) // every rare unit of the chart is in the company: a 6 (choice of the above) has nothing to give
	res, err := recruit(t, s, f, 6, 0)
	if err != nil || res.Lower || f.state.Recruit.Lower != nil || !strings.Contains(res.Summary, "no result on the chart can join") {
		t.Fatalf("the recruit ends with nothing, it does not wait for a pick that cannot be made: %+v %v %+v", res, err, f.state.Recruit.Lower)
	}
	if f.state.Influence != 9 {
		t.Errorf("the roll was paid: %d", f.state.Influence)
	}
}

func TestAHobbitRollOffersTheHobbitProfilesTheCompanyLacks(t *testing.T) {
	f := newCompanyFixture(t)
	f.companyOf("Wanderers in the Wild", 12, []UnitState{{ID: 1, Base: "Hobbit Militia", Role: RoleWarrior}, {ID: 2, Base: "Hobbit Shirriff", Role: RoleWarrior}})
	rs := shippedRules(t)
	d := BuildDetail(rs, f.settings, f.state)
	row := d.Recruit.Chart[1]
	if len(row.Options) != 4 {
		t.Fatalf("four hobbit profiles on a 2: %+v", row)
	}
	have := map[string]bool{}
	for _, u := range d.Roster {
		have[u.Type] = true
	}
	for _, o := range row.Options {
		if (o.Blocked != "") != have[o.Text] {
			t.Errorf("%s is blocked exactly when the company has it: %q (has: %v)", o.Text, o.Blocked, have[o.Text])
		}
	}
	env := newRecruitEnv(rs, f.settings, f.state)
	bcCo, _ := rs.BC.Company("Wanderers in the Wild")
	res := bcCo.Reinforcements[1].Result
	if res.Type != "unitChoice" {
		t.Fatalf("%+v", res)
	}
	picked := env.pickOption(res, "opt:2")
	if picked.Unit != "Tookish Hunter" || len(picked.Wargear) != 1 {
		t.Errorf("a Tookish Hunter is a Hobbit Archer with its option: %+v", picked)
	}
}

// A special unit the company names itself (the Goblin Hulk, a Gundabad Ogre) joins, and is shown, under that name.
func TestSpecialUnitIsRecruitedUnderTheCompanysName(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.recruiter(t)
	f.companyOf("Goblin-Town", 30, warriors("Goblin Warrior", 5))
	d := BuildDetail(shippedRules(t), f.settings, f.state)
	if len(d.Recruit.Special) != 1 || d.Recruit.Special[0].Unit != "Goblin Hulk" {
		t.Fatalf("special units: %+v", d.Recruit.Special)
	}
	if _, err := s.RecruitSpecial(f.ctx, f.user, 5, "Goblin Hulk"); err != nil {
		t.Fatal(err)
	}
	last := f.state.Units[len(f.state.Units)-1]
	if last.Base != "Goblin Hulk" {
		t.Fatalf("joined as %q", last.Base)
	}
	d = BuildDetail(shippedRules(t), f.settings, f.state)
	var hulk *CompanyUnit
	for i := range d.Roster {
		if d.Roster[i].ID == last.ID {
			hulk = &d.Roster[i]
		}
	}
	if hulk == nil || hulk.Points != 60 || !hulk.OwnPath {
		t.Errorf("the Hulk has the Ogre's profile: %+v", hulk)
	}
	if hulk.Type != "Goblin Hulk" {
		t.Errorf("the Hulk is called %q", hulk.Type)
	}
	if _, err := s.RecruitSpecial(f.ctx, f.user, 5, "Goblin Hulk"); !errors.Is(err, ErrRefused) {
		t.Errorf("rare 1: a second one may not join: %v", err)
	}
}

// A team exists while a member is on the roster; the cemetery is not consulted (register G9).
func TestMissingTeamMemberLooksAtTheRosterOnly(t *testing.T) {
	fallen := func(base string) FallenUnit { return FallenUnit{Unit: CompanyUnit{Type: base}, Fate: "Dead"} }
	for _, tt := range []struct {
		name    string
		units   []UnitState
		dead    []FallenUnit
		replace string
	}{
		{"only the Foe Spear Wielder died", []UnitState{{ID: 9, Base: "Iron Shield Bearer", Role: RoleWarrior}}, []FallenUnit{fallen("Foe Spear Wielder")}, "Foe Spear Wielder"},
		{"the whole team died: a fresh team can be rolled", nil, []FallenUnit{fallen("Foe Spear Wielder"), fallen("Iron Shield Bearer")}, ""},
		{"a team never recruited", nil, nil, ""},
		{"the Shield Bearer died and the Wielder lives", []UnitState{{ID: 9, Base: "Foe Spear Wielder", Role: RoleWarrior}}, []FallenUnit{fallen("Iron Shield Bearer")}, "Iron Shield Bearer"},
		{"the team is whole", []UnitState{{ID: 9, Base: "Foe Spear Wielder", Role: RoleWarrior}, {ID: 10, Base: "Iron Shield Bearer", Role: RoleWarrior}}, nil, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newCompanyFixture(t)
			s := f.recruiter(t)
			f.companyOf("Durin's Folk", 30, append(warriors("Iron Hills Warrior", 3), tt.units...))
			f.state.Cemetery = tt.dead
			f.state.Recruit.Roll = &RecruitStep{Stage: "special"}
			if err := s.RollRecruit(f.ctx, f.user, 5, 1, false); err != nil {
				t.Fatal(err)
			}
			state, _ := s.RecruitState(f.ctx, f.user, 5)
			if state == nil || state.Replace != tt.replace {
				t.Errorf("missing member %+v, want %q", state, tt.replace)
			}
		})
	}
}

// "Choice of any" offers the units of every chart, normal and special, and any roll (register G7).
func TestChoiceOfAnyOffersEveryChartAndEveryRoll(t *testing.T) {
	rs := shippedRules(t)
	rohan, _ := rs.BC.Company("Rohan")
	normal := func(res []LowerEntry, chart string) int {
		n := 0
		for _, e := range res {
			if e.Chart == chart {
				n++
			}
		}
		return n
	}
	for _, tt := range []struct {
		name, text, stage string
		roll              int
		special, normal   bool
	}{
		{"any on the special chart", "Choice of Any", "special", 6, true, true},
		{"any on the normal chart", "Choice of any", "chart", 6, true, true},
		{"above on the normal chart", "Choice of above", "chart", 5, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			env := recruitEnv{rs: rs, bcC: rohan}
			got := env.choiceEntries(tt.stage, tt.roll, Reinforcement{Type: "choice", Text: tt.text})
			if (normal(got, "Special chart") > 0) != tt.special || (normal(got, "Reinforcements") > 0) != tt.normal {
				t.Errorf("charts offered: reinforcements %d, special %d", normal(got, "Reinforcements"), normal(got, "Special chart"))
			}
			if !tt.special {
				return
			}
			for _, row := range append(slices.Clone(rohan.Reinforcements), rohan.SpecialChart...) { // every unit result of every row can be taken
				for _, o := range rowResults(row) {
					if o.Type == "unit" && !slices.ContainsFunc(got, func(e LowerEntry) bool { return e.Res.Unit == o.Unit }) {
						t.Errorf("%s is not offered", o.Unit)
					}
				}
			}
		})
	}
}

// The reinforcement price leaves the balance when the roll is made and cannot be undone; the extra influence leaves it at the confirm (register G8).
func TestRollCostIsSpentAtOnceAndRaisingAtConfirm(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.recruiter(t)
	if err := s.RollRecruit(f.ctx, f.user, 5, 2, false); err != nil {
		t.Fatal(err)
	}
	if f.state.Influence != 9 {
		t.Fatalf("the price (3) is gone at the roll: %d", f.state.Influence)
	}
	if d := BuildDetail(shippedRules(t), f.settings, f.state); d.Influence != 9 {
		t.Errorf("the page's balance during the recruit: %d", d.Influence)
	}
	if _, err := s.ConfirmRecruit(f.ctx, f.user, 5, 2, ""); err != nil {
		t.Fatal(err)
	}
	if f.state.Influence != 7 {
		t.Errorf("raising by 2 costs 2 more: %d", f.state.Influence)
	}
}

// With the whole team in the cemetery a normal roll recruits a fresh team: rare 1 counts the active roster only (register G9).
func TestAWholeTeamInTheCemeteryCanBeRecruitedAgain(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.recruiter(t)
	f.companyOf("Durin's Folk", 30, warriors("Iron Hills Warrior", 3))
	f.state.Cemetery = []FallenUnit{{Unit: CompanyUnit{Type: "Foe Spear Wielder"}, Fate: "Dead"}, {Unit: CompanyUnit{Type: "Iron Shield Bearer"}, Fate: "Dead"}}
	co, _ := shippedRules(t).BC.Company("Durin's Folk")
	teamRoll := 0
	for _, row := range co.SpecialChart {
		if row.Result.Unit == "Vault Warden Team" {
			teamRoll = row.Min
		}
	}
	f.state.Recruit.Roll = &RecruitStep{Stage: "special"}
	if err := s.RollRecruit(f.ctx, f.user, 5, teamRoll, false); err != nil {
		t.Fatal(err)
	}
	if state, _ := s.RecruitState(f.ctx, f.user, 5); state == nil || state.Replace != "" {
		t.Fatalf("no substitution is offered: %+v", state)
	}
	if res, err := s.ConfirmRecruit(f.ctx, f.user, 5, 0, ""); err != nil || res.Lower || !strings.Contains(res.Summary, "Iron Shield Bearer and Foe Spear Wielder") {
		t.Errorf("a full team joins: %+v %v", res, err)
	}
}
