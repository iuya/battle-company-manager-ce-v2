package core

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

// amFixture is the aftermath service over mocks: campaign 5, match 1 (number 3) won by the side of company 1, owned by user 9.
type amFixture struct {
	svc       *AftermathService
	campaigns *MockcampaignRepository
	companies *MockcompanyRepository
	matches   *MockmatchRepository
	rulesets  *MockrulesetRepository
	repo      *MockaftermathRepository
	state     CompanyState
	rec       AftermathRecord
	match     Match
	dice      *FixedDice
	ctx       context.Context
	settings  CampaignSettings // the campaign's settings (typed dice by default)
}

const (
	amOwner     = 9
	amOrganiser = 99
)

func newAmFixture(t *testing.T) *amFixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	f := &amFixture{campaigns: NewMockcampaignRepository(ctrl), companies: NewMockcompanyRepository(ctrl), matches: NewMockmatchRepository(ctrl),
		rulesets: NewMockrulesetRepository(ctrl), repo: NewMockaftermathRepository(ctrl), state: gondor(), dice: &FixedDice{Rolls: []int{1}}, ctx: context.Background(), settings: CampaignSettings{ManualRolls: true}}
	f.match = Match{ID: 1, Number: 3, Ended: true, Winner: 1, Scenario: &MatchScenario{Kind: ScenarioRulebook, Name: "Rescue"},
		Result: MatchResult{Decided: true, Winner: 1},
		Sides: []MatchSide{{Name: "A", Companies: []CampaignCompany{{ID: 1, OwnerID: amOwner, Name: "The White Company"}}},
			{Name: "B", Companies: []CampaignCompany{{ID: 2, OwnerID: 20, Name: "Raiders"}}}}}
	f.rec = AftermathRecord{MatchID: 1, CompanyID: 1, MatchEnded: true, Status: AftermathPending}
	f.campaigns.EXPECT().Hashes(gomock.Any(), int64(5)).DoAndReturn(func(context.Context, int64) (RulesetHashes, CampaignSettings, error) {
		return RulesetHashes{BattleCompanies: "bc"}, f.settings, nil
	}).AnyTimes()
	f.campaigns.EXPECT().Get(gomock.Any(), gomock.Any(), int64(5)).DoAndReturn(func(_ context.Context, user, _ int64) (CampaignDetail, error) {
		if user == 7 {
			return CampaignDetail{}, ErrNotFound
		}
		d := CampaignDetail{CampaignSummary: CampaignSummary{ID: 5}, OrganiserID: amOrganiser}
		if user == amOwner {
			d.Company = &CampaignCompany{ID: 1}
		}
		return d, nil
	}).AnyTimes()
	f.matches.EXPECT().Get(gomock.Any(), int64(5), int64(1)).DoAndReturn(func(context.Context, int64, int64) (Match, error) { return f.match, nil }).AnyTimes()
	f.rulesets.EXPECT().Get(gomock.Any(), "bc").DoAndReturn(func(context.Context, string) (StoredRuleset, error) {
		return StoredRuleset{Content: readDef(t, "battle_companies/battle_companies.json")}, nil
	}).AnyTimes()
	f.companies.EXPECT().Read(gomock.Any(), int64(5), int64(1)).DoAndReturn(func(context.Context, int64, int64) (CompanyState, error) { return f.state.Clone(), nil }).AnyTimes()
	f.repo.EXPECT().Get(gomock.Any(), int64(5), int64(1), int64(1)).DoAndReturn(func(context.Context, int64, int64, int64) (AftermathRecord, error) { return f.rec, nil }).AnyTimes()
	f.repo.EXPECT().Update(gomock.Any(), int64(5), int64(1), int64(1), gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _, _ int64, fn func(*CompanyState, *AftermathRecord) error) error {
			st, rec := f.state.Clone(), f.rec
			rec.NewGrants, rec.NewPerks = nil, nil
			if err := fn(&st, &rec); err != nil {
				return err
			}
			f.state, f.rec = st, rec
			return nil
		}).AnyTimes()
	f.svc = NewAftermathService(f.campaigns, f.companies, f.matches, f.rulesets, f.repo, staticRules{shippedRules(t)}, f.dice)
	return f
}

func (f *amFixture) owner() User { return User{ID: amOwner} }

func (f *amFixture) unit(t *testing.T, id int64) CompanyUnit {
	t.Helper()
	d := BuildDetail(shippedRules(t), CampaignSettings{}, f.state)
	return mustRosterUnit(t, d.Roster, id)
}

func mustRosterUnit(t *testing.T, roster []CompanyUnit, id int64) CompanyUnit {
	t.Helper()
	u, ok := rosterUnit(roster, id)
	if !ok {
		t.Fatalf("unit %d is not on the roster", id)
	}
	return u
}

func casualties(ids ...int64) AftermathInput {
	var in AftermathInput
	for _, id := range ids {
		in.Models = append(in.Models, ModelSheet{UnitID: id, Outcome: OutcomeCasualty})
	}
	return in
}

func (f *amFixture) start(t *testing.T, in AftermathInput) Aftermath {
	t.Helper()
	a, err := f.svc.StartAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision, in)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	return a
}

func (f *amFixture) roll(t *testing.T, unit int64, a, b int) Aftermath {
	t.Helper()
	out, err := f.svc.RollCasualty(f.ctx, f.owner(), 5, 1, 1, unit, []int{a, b}, false)
	if err != nil {
		t.Fatalf("roll %d: %v", unit, err)
	}
	return out
}

func TestAftermathStartRecoversAbsentUnitsAndCreditsRerolls(t *testing.T) {
	f := newAmFixture(t)
	if !f.unit(t, 4).Injured || !f.unit(t, 7).Injured {
		t.Fatal("the fixture has two injured units")
	}
	in := casualties(6)
	in.AgainstTheOdds = []string{"reroll"}
	a := f.start(t, in)
	if a.Status != AftermathInjuries || a.Rerolls != 2 || f.state.Rerolls != 2 || !slices.Equal(a.Absent, []int64{4, 7}) {
		t.Fatalf("start: status %s, rerolls %d/%d, absent %v", a.Status, a.Rerolls, f.state.Rerolls, a.Absent)
	}
	if f.unit(t, 4).Injured || f.unit(t, 7).Injured {
		t.Error("the units that missed the game recover when it starts")
	}
	// the sheet can be unlocked again before the first roll, which undoes the start
	a, err := f.svc.ReopenSheet(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision)
	if err != nil || a.Status != AftermathSheet || f.state.Rerolls != 0 || !f.unit(t, 4).Injured || !f.unit(t, 7).Injured {
		t.Fatalf("reopen: %+v %v (rerolls %d)", a, err, f.state.Rerolls)
	}
}

func TestAftermathWithoutCasualtiesGoesStraightToLevelUps(t *testing.T) {
	f := newAmFixture(t)
	xp := f.unit(t, 6).XP
	a := f.start(t, AftermathInput{})
	if a.Status != AftermathLevelUps {
		t.Fatalf("status %s", a.Status)
	}
	if got := f.unit(t, 6).XP; got != xp+1 {
		t.Errorf("participation is 1 experience: %d -> %d", xp, got)
	}
	if got := f.unit(t, 7).XP; got != 5 {
		t.Errorf("a unit that missed the game earns nothing: %d", got)
	}
	influence := f.state.Influence
	a, err := f.svc.FinishAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision)
	if err != nil || a.Status != AftermathDone {
		t.Fatalf("finish: %+v %v", a, err)
	}
	// Rescue: the Leader was not removed, so survivalOfTheFittest gave it +1 experience; Participating 2 + Win 2 Influence
	if f.state.Influence != influence+4 || f.rec.FinishedAt == nil {
		t.Errorf("influence %d -> %d", influence, f.state.Influence)
	}
	if len(f.rec.NewGrants) != 1 || f.rec.NewGrants[0].Experience != 1 || f.rec.NewGrants[0].UnitID != 1 || f.rec.NewGrants[0].Kind != GrantScenario || f.rec.NewGrants[0].MatchID != 1 {
		t.Errorf("the scenario experience is recorded as a grant: %+v", f.rec.NewGrants)
	}
	if _, err := f.svc.FinishAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision); !errors.Is(err, ErrRefused) {
		t.Errorf("finish twice: %v", err)
	}
}

func TestAftermathLeaderFallsAndSuccessionIsAskedFor(t *testing.T) {
	f := newAmFixture(t)
	f.start(t, casualties(1))
	a := f.roll(t, 1, 1, 1) // 2: Dead
	if a.Status != AftermathInjuries || a.Succession == nil || !a.Succession.NeedLeader || !slices.Equal(a.Succession.LeaderOptions, []int64{2, 3}) {
		t.Fatalf("the Leader fell: %+v", a.Succession)
	}
	if len(f.state.Cemetery) != 1 || f.state.Cemetery[0].Fate != "Dead" || f.state.Cemetery[0].Match != "Match 3, Rescue" {
		t.Fatalf("cemetery: %+v", f.state.Cemetery)
	}
	if _, err := f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{Kind: "succession", Answer: "5"}); !errors.Is(err, ErrInvalidAftermath) {
		t.Fatalf("only a Sergeant may lead: %v", err)
	}
	a, err := f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{Kind: "succession", Answer: "2"})
	if err != nil || a.Succession == nil || !a.Succession.NeedSergeant || a.Status != AftermathInjuries {
		t.Fatalf("a Sergeant place is open next: %+v %v", a.Succession, err)
	}
	if f.unit(t, 2).Role != RoleLeader {
		t.Error("the Sergeant leads now")
	}
	xp := f.unit(t, 3).XP
	a, err = f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{Kind: "sergeant", Answer: "5"})
	if err != nil || a.Status != AftermathLevelUps || f.unit(t, 5).Role != RoleSergeant {
		t.Fatalf("sergeant: %+v %v", a, err)
	}
	if f.unit(t, 3).XP <= xp {
		t.Error("experience is given when the succession is settled")
	}
	influence := f.state.Influence
	if _, err := f.svc.FinishAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision); err != nil {
		t.Fatal(err)
	}
	if f.state.Influence != influence+4 || len(f.rec.NewGrants) != 0 {
		t.Errorf("no Rescue reward without the Leader: influence +%d, grants %+v", f.state.Influence-influence, f.rec.NewGrants)
	}
}

func TestAftermathHeroInjuries(t *testing.T) {
	t.Run("a second arm wound forces retirement", func(t *testing.T) {
		f := newAmFixture(t)
		f.state.Units[2].Injuries = map[string]int{"armWound": 1} // Baranor
		f.start(t, casualties(3))
		a := f.roll(t, 3, 1, 2)
		if len(f.state.Cemetery) != 1 || f.state.Cemetery[0].Fate != "Retired" || f.state.Cemetery[0].Unit.ID != 3 {
			t.Fatalf("%s, cemetery %+v", a.Status, f.state.Cemetery)
		}
		if a.Status != AftermathInjuries || a.Succession == nil || !a.Succession.NeedSergeant { // a retired Sergeant is replaced like a dead one
			t.Fatalf("a Sergeant place is open: %+v", a.Succession)
		}
	})
	t.Run("the first one is kept", func(t *testing.T) {
		f := newAmFixture(t)
		f.start(t, casualties(3))
		f.roll(t, 3, 1, 2)
		if u := f.unit(t, 3); len(u.Injuries) != 1 || u.Injuries[0].Key != "armWound" || u.Injuries[0].Count != 1 {
			t.Fatalf("injuries: %+v", u.Injuries)
		}
	})
	t.Run("protection by the valar gives a point of Fate", func(t *testing.T) {
		f := newAmFixture(t)
		f.start(t, casualties(3))
		f.roll(t, 3, 5, 6)
		if f.state.Units[2].Progress == nil || f.state.Units[2].Progress.Bonus["fate"] != 1 {
			t.Fatalf("progress: %+v", f.state.Units[2].Progress)
		}
	})
	t.Run("a scratch asks, and a second roll stands", func(t *testing.T) {
		f := newAmFixture(t)
		f.dice.Rolls = []int{4}
		f.start(t, casualties(3))
		a := f.roll(t, 3, 3, 3) // 6: T'is Just a Scratch
		if a.Status != AftermathInjuries || a.Casualties[0].Decision == nil || a.Casualties[0].Decision.Kind != "scratch" {
			t.Fatalf("decision: %+v", a.Casualties)
		}
		if _, err := f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{UnitID: 3, Kind: "scratch", Answer: "maybe"}); !errors.Is(err, ErrInvalidAftermath) {
			t.Fatalf("bad answer: %v", err)
		}
		if _, err := f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{UnitID: 3, Kind: "scratch", Answer: "reroll"}); err != nil {
			t.Fatal(err)
		}
		a = f.roll(t, 3, 6, 6) // 12: Wounds of a Hero: the D6 is asked for (a typed-dice campaign)
		if d := a.Casualties[0].Decision; d == nil || d.Kind != "bonus" || a.Status != AftermathInjuries {
			t.Fatalf("the bonus die waits: %+v", a.Casualties[0])
		}
		if _, err := f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{UnitID: 3, Kind: "bonus", Answer: "7"}); !errors.Is(err, ErrInvalidAftermath) {
			t.Fatalf("a D6 is 1 to 6: %v", err)
		}
		a, err := f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{UnitID: 3, Kind: "bonus", Answer: "4"}) // D6 influence (4)
		if err != nil || a.Casualties[0].Bonus != 4 {
			t.Fatalf("typed die: %+v %v", a.Casualties[0], err)
		}
		if a.Status != AftermathLevelUps {
			t.Fatalf("%s", a.Status)
		}
		influence := f.state.Influence
		if _, err := f.svc.FinishAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision); err != nil {
			t.Fatal(err)
		}
		if f.state.Influence != influence+4+f.dice.Rolls[0] {
			t.Errorf("influence %d -> %d, want +%d", influence, f.state.Influence, 4+f.dice.Rolls[0])
		}
	})
	t.Run("a 12 on the hero chart gives the D6 influence, rolled by the server without typed dice", func(t *testing.T) {
		f := newAmFixture(t)
		f.settings = CampaignSettings{}
		f.dice.Rolls = []int{6} // 6 + 6 = 12, and the D6 is a 6
		f.start(t, casualties(3))
		a, err := f.svc.RollCasualty(f.ctx, f.owner(), 5, 1, 1, 3, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		c := a.Casualties[0]
		if c.Total != 12 || c.ResultKey != "woundsOfAHero" || c.Bonus != 6 || c.Decision != nil {
			t.Fatalf("a 12: %+v", c)
		}
		var got int
		for _, l := range a.Preview.Influence {
			if l.Key == "woundsOfAHero" {
				got = l.Amount
			}
		}
		if got != 6 {
			t.Errorf("the preview shows the extra influence: %+v", a.Preview.Influence)
		}
	})
	t.Run("a scratch kept misses the next game", func(t *testing.T) {
		f := newAmFixture(t)
		f.start(t, casualties(3))
		f.roll(t, 3, 3, 3)
		if _, err := f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{UnitID: 3, Kind: "scratch", Answer: "keep"}); err != nil {
			t.Fatal(err)
		}
		if !f.unit(t, 3).Injured {
			t.Error("it misses the next game")
		}
	})
	t.Run("full recovery heals one injury of the player's choice", func(t *testing.T) {
		f := newAmFixture(t)
		f.state.Units[2].Injuries = map[string]int{"armWound": 1, "legWound": 1}
		f.start(t, casualties(3))
		a := f.roll(t, 3, 4, 4) // 8: full recovery
		if d := a.Casualties[0].Decision; d == nil || d.Kind != "heal" || len(d.Options) != 2 {
			t.Fatalf("decision: %+v", a.Casualties[0].Decision)
		}
		if _, err := f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{UnitID: 3, Kind: "heal", Answer: "brokenHonour"}); !errors.Is(err, ErrInvalidAftermath) {
			t.Fatalf("not an injury it has: %v", err)
		}
		if _, err := f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{UnitID: 3, Kind: "heal", Answer: "legWound"}); err != nil {
			t.Fatal(err)
		}
		if u := f.unit(t, 3); len(u.Injuries) != 1 || u.Injuries[0].Key != "armWound" {
			t.Fatalf("injuries: %+v", u.Injuries)
		}
	})
}

func TestAftermathWarriorResultsAndRerolls(t *testing.T) {
	f := newAmFixture(t)
	a := f.start(t, AftermathInput{AgainstTheOdds: []string{"reroll"}, Models: casualties(6, 8, 9).Models})
	f.roll(t, 8, 2, 3)                                                                                                         // 5: injured
	f.roll(t, 9, 6, 6)                                                                                                         // 12: Lesson Learned (+D3 = 1 with a die of 1)
	if _, err := f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{UnitID: 9, Kind: "bonus", Answer: "2"}); err != nil { // D3: 2 -> 1
		t.Fatal(err)
	}
	a = f.roll(t, 6, 3, 3)
	if a.Status != AftermathInjuries || f.rec.Work.Applied {
		t.Fatal("with re-rolls left the player decides when to move on")
	}
	f.dice.Rolls, f.dice.n = []int{1, 1}, 0
	if _, err := f.svc.RerollCasualty(f.ctx, f.owner(), 5, 1, 1, 6, nil, false); err != nil { // 6 -> 2: Dead
		t.Fatal(err)
	}
	if f.state.Rerolls != 1 {
		t.Errorf("one re-roll is spent: %d", f.state.Rerolls)
	}
	xp9 := f.unit(t, 9).XP
	a, err := f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{Kind: "continue"})
	if err != nil || a.Status != AftermathLevelUps {
		t.Fatalf("continue: %+v %v", a.Status, err)
	}
	if len(f.state.Cemetery) != 1 || f.state.Cemetery[0].Unit.ID != 6 || !f.unit(t, 8).Injured || f.unit(t, 9).XP != xp9+1 {
		t.Errorf("effects: cemetery %+v, injured %v, xp %d -> %d", f.state.Cemetery, f.unit(t, 8).Injured, xp9, f.unit(t, 9).XP)
	}
	if _, err := f.svc.RerollCasualty(f.ctx, f.owner(), 5, 1, 1, 8, nil, false); !errors.Is(err, ErrRefused) {
		t.Errorf("no re-roll once settled: %v", err)
	}
	if _, err := f.svc.FinishAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision); err != nil {
		t.Fatal(err)
	}
	if f.state.Rerolls != 0 {
		t.Errorf("the bonus expires at the end: %d", f.state.Rerolls)
	}
}

func TestAftermathLastRerollClosesTheStep(t *testing.T) {
	f := newAmFixture(t)
	f.start(t, AftermathInput{AgainstTheOdds: []string{"reroll"}, Models: casualties(8, 9).Models})
	f.roll(t, 8, 2, 3)
	f.roll(t, 9, 3, 3)
	f.dice.Rolls, f.dice.n = []int{4, 4, 4, 4}, 0
	if _, err := f.svc.RerollCasualty(f.ctx, f.owner(), 5, 1, 1, 8, nil, false); err != nil {
		t.Fatal(err)
	}
	if f.rec.Status != AftermathInjuries {
		t.Fatalf("one re-roll is left, the player decides: %v", f.rec.Status)
	}
	a, err := f.svc.RerollCasualty(f.ctx, f.owner(), 5, 1, 1, 9, nil, false)
	if err != nil || a.Status != AftermathLevelUps || f.state.Rerolls != 0 {
		t.Fatalf("with no re-roll left nothing waits for a Continue: %v %v rerolls %d", a.Status, err, f.state.Rerolls)
	}
}

func TestAftermathPermissionsAndRefusals(t *testing.T) {
	f := newAmFixture(t)
	other, org, admin := User{ID: 8}, User{ID: amOrganiser}, User{ID: 5, Role: RoleAdmin}
	if _, err := f.svc.Aftermath(f.ctx, User{ID: 7}, 5, 1, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("stranger: %v", err)
	}
	if _, err := f.svc.Aftermath(f.ctx, f.owner(), 5, 1, 99); !errors.Is(err, ErrNotFound) {
		t.Errorf("company not in the match: %v", err)
	}
	a, err := f.svc.Aftermath(f.ctx, other, 5, 1, 1)
	if err != nil || a.Access != AftermathNone || len(a.Input.Models) == 0 || a.Status != AftermathPending || a.Preview.InfluenceTotal() != 4 {
		t.Fatalf("another player reads: %+v %v", a, err)
	}
	if _, err := f.svc.SaveAftermath(f.ctx, other, 5, 1, 1, 0, a.Input); !errors.Is(err, ErrRefused) {
		t.Errorf("another player writes: %v", err)
	}
	for _, u := range []User{f.owner(), org, admin} {
		if _, err := f.svc.SaveAftermath(f.ctx, u, 5, 1, 1, f.rec.Revision, a.Input); err != nil {
			t.Errorf("user %d: %v", u.ID, err)
		}
	}
	if _, err := f.svc.SaveAftermath(f.ctx, f.owner(), 5, 1, 1, 0, a.Input); !errors.Is(err, ErrAftermathChanged) {
		t.Errorf("stale: %v", err)
	}
	bad := AftermathInput{Models: []ModelSheet{{UnitID: 99}}}
	if _, err := f.svc.SaveAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision, bad); !errors.Is(err, ErrInvalidAftermath) {
		t.Errorf("unknown unit: %v", err)
	}
	if _, err := f.svc.RollCasualty(f.ctx, f.owner(), 5, 1, 1, 0, nil, false); !errors.Is(err, ErrRefused) {
		t.Errorf("nothing started: %v", err)
	}
	f.match.Ended = false
	if _, err := f.svc.Aftermath(f.ctx, f.owner(), 5, 1, 1); !errors.Is(err, ErrRefused) {
		t.Errorf("open match: %v", err)
	}
}

func TestWaiveAftermath(t *testing.T) {
	f := newAmFixture(t)
	if _, err := f.svc.WaiveAftermath(f.ctx, f.owner(), 5, 1, 1, "x"); !errors.Is(err, ErrRefused) {
		t.Errorf("the owner cannot waive: %v", err)
	}
	if _, err := f.svc.WaiveAftermath(f.ctx, User{ID: amOrganiser}, 5, 1, 1, strings.Repeat("x", 201)); !errors.Is(err, ErrInvalidAftermath) {
		t.Errorf("note too long: %v", err)
	}
	f.start(t, AftermathInput{AgainstTheOdds: []string{"reroll"}})
	f.state.Influence = 3
	a, err := f.svc.WaiveAftermath(f.ctx, User{ID: amOrganiser}, 5, 1, 1, "gone")
	if err != nil || a.Status != AftermathWaived || f.state.Influence != 3 || f.state.Rerolls != 0 {
		t.Fatalf("waive: %+v %v", a, err)
	}
	if _, err := f.svc.WaiveAftermath(f.ctx, User{ID: amOrganiser}, 5, 1, 1, "again"); !errors.Is(err, ErrRefused) {
		t.Errorf("waived once: %v", err)
	}
}

func TestCompanyActionsAreLockedWhileAnAftermathIsPending(t *testing.T) {
	rules := staticRules{shippedRules(t)}
	f := newCompanyFixture(t)
	f.update()
	f.state.Aftermaths = []AftermathRef{{MatchID: 1, MatchNumber: 3, CompanyID: 1, Status: AftermathPending}}
	arm := NewArmouryService(f.campaigns, f.companies, rules)
	for name, err := range map[string]error{
		"buy":      arm.BuyEquipment(f.ctx, f.user, 5, 6, "Shield", ""),
		"drop":     arm.DropEquipment(f.ctx, f.user, 5, 6, "Bow"),
		"creature": arm.BuyCreature(f.ctx, f.user, 5, 2, "Baggage Pony", ""),
		"wanderer": arm.HireWanderer(f.ctx, f.user, 5, "Anybody", ""),
		"dismiss":  arm.DismissWanderer(f.ctx, f.user, 5, 6),
		"recruit":  NewRecruitService(f.campaigns, f.companies, rules, &FixedDice{Rolls: []int{3}}).RollRecruit(f.ctx, f.user, 5, 0, false),
	} {
		if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "Finish the aftermath of Match 3 first") {
			t.Errorf("%s: %v", name, err)
		}
	}
	_, err := NewTreatmentService(f.campaigns, f.companies, rules, &FixedDice{}).TreatInjury(f.ctx, f.user, 5, 7, Treatment{Method: "warrior"})
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "Finish the aftermath") {
		t.Errorf("treat: %v", err)
	}
	if err := NewCompanyService(f.campaigns, f.companies, rules).RenameCompany(f.ctx, f.user, 5, "Still allowed"); err != nil {
		t.Errorf("renaming: %v", err)
	}
	f.state.Aftermaths = nil
	if err := arm.DropEquipment(f.ctx, f.user, 5, 6, "Bow"); errors.Is(err, ErrRefused) && strings.Contains(err.Error(), "Finish the aftermath") {
		t.Errorf("unlocked: %v", err)
	}
}

func TestScenarioRewardsAtFinish(t *testing.T) {
	scenario := func(f *amFixture, name string, objectives map[string]int) {
		f.match.Scenario = &MatchScenario{Kind: ScenarioRulebook, Name: name}
		f.match.Result.Companies = []CompanyResult{{CompanyID: 1, Objectives: objectives}}
	}
	t.Run("a victory leaves a perk", func(t *testing.T) {
		f := newAmFixture(t)
		scenario(f, "Chance Encounter", nil)
		f.start(t, AftermathInput{})
		if _, err := f.svc.FinishAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision); err != nil {
			t.Fatal(err)
		}
		if len(f.rec.NewPerks) != 1 || f.rec.NewPerks[0].Effect != "rendezvous" || f.rec.NewPerks[0].MatchID != 1 {
			t.Fatalf("perks: %+v", f.rec.NewPerks)
		}
	})
	t.Run("a defeat does not", func(t *testing.T) {
		f := newAmFixture(t)
		scenario(f, "Chance Encounter", nil)
		f.match.Winner = 2
		f.start(t, AftermathInput{})
		if _, err := f.svc.FinishAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision); err != nil {
			t.Fatal(err)
		}
		if len(f.rec.NewPerks) != 0 {
			t.Fatalf("perks: %+v", f.rec.NewPerks)
		}
	})
	t.Run("the relic perk is bound to its bearer", func(t *testing.T) {
		f := newAmFixture(t)
		scenario(f, "Relic of Ages Past", map[string]int{"possessesRelic": 1})
		in := DefaultSheet(BuildDetail(shippedRules(t), CampaignSettings{}, f.state).Roster)
		in.Models[2].Relic = true
		bearer := in.Models[2].UnitID
		f.start(t, in)
		if _, err := f.svc.FinishAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision); err != nil {
			t.Fatal(err)
		}
		if len(f.rec.NewPerks) != 1 || f.rec.NewPerks[0].UnitID != bearer || f.rec.NewPerks[0].When != "nextGame" {
			t.Fatalf("perks: %+v (bearer %d)", f.rec.NewPerks, bearer)
		}
	})
	t.Run("cargo is Influence and a grant", func(t *testing.T) {
		f := newAmFixture(t)
		scenario(f, "Baggage Train", map[string]int{"allCargo": 1})
		f.start(t, AftermathInput{})
		influence := f.state.Influence
		if _, err := f.svc.FinishAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision); err != nil {
			t.Fatal(err)
		}
		if f.state.Influence != influence+5 || len(f.rec.NewGrants) != 1 || f.rec.NewGrants[0].Influence != 1 {
			t.Fatalf("influence +%d, grants %+v", f.state.Influence-influence, f.rec.NewGrants)
		}
	})
	t.Run("free equipment is chosen, checked and given", func(t *testing.T) {
		f := newAmFixture(t)
		scenario(f, "Scavenge the Battlefield", map[string]int{"allObjectiveMarkers": 1})
		f.start(t, AftermathInput{})
		if _, err := f.svc.FinishAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision); !errors.Is(err, ErrRefused) {
			t.Fatalf("finish needs the choice: %v", err)
		}
		offers, err := f.svc.FreeEquipmentOffers(f.ctx, 5, 1, 2)
		if err != nil || len(offers) == 0 {
			t.Fatalf("offers: %v %v", offers, err)
		}
		for _, o := range offers {
			if o.Cost > 2 || len(o.Units) == 0 {
				t.Errorf("%s costs %d and has %d units", o.Name, o.Cost, len(o.Units))
			}
		}
		all, _ := f.svc.FreeEquipmentOffers(f.ctx, 5, 1, 99)
		if len(all) <= len(offers) {
			t.Errorf("a bigger value offers more: %d vs %d", len(all), len(offers))
		}
		decide := func(answer string) error {
			_, err := f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{Kind: "reward", Key: "toTheVictorTheSpoils", Answer: answer})
			return err
		}
		if err := decide("Nothing|1"); !errors.Is(err, ErrInvalidAftermath) {
			t.Errorf("unknown item: %v", err)
		}
		pricey := all[slices.IndexFunc(all, func(o FreeEquipment) bool { return o.Cost > 2 })]
		if err := decide(pricey.Name + "|" + strconv.FormatInt(pricey.Units[0].ID, 10)); !errors.Is(err, ErrInvalidAftermath) {
			t.Errorf("worth more than allowed: %v", err)
		}
		item := offers[0]
		if err := decide(item.Name + "|" + strconv.FormatInt(item.Units[0].ID, 10)); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.FinishAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision); err != nil {
			t.Fatal(err)
		}
		if u, _ := stateUnit(&f.state, item.Units[0].ID); !slices.Contains(u.Bought, item.Name) {
			t.Errorf("%s was not given: %+v", item.Name, u.Bought)
		}
	})
}

func TestUsePerkIsForTheOwner(t *testing.T) {
	f := newAmFixture(t)
	f.repo.EXPECT().UsePerk(gomock.Any(), int64(5), int64(1), int64(4)).Return(nil)
	if err := f.svc.UsePerk(f.ctx, f.owner(), 5, 1, 4); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.UsePerk(f.ctx, User{ID: 8}, 5, 1, 4); !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrRefused) {
		t.Errorf("another player: %v", err)
	}
	if err := f.svc.UsePerk(f.ctx, User{ID: amOrganiser}, 5, 1, 4); !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrRefused) {
		t.Errorf("the organiser has no company: %v", err)
	}
}

func TestScenarioObjectivesSayWhichOnesAreHeldByAModel(t *testing.T) {
	list, err := ParseScenarios(readDef(t, "battle_companies/battle_companies.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, sc := range list {
		for _, o := range sc.Objectives {
			if want := o.Key == "possessesRelic"; o.Model != want {
				t.Errorf("%s / %s: Model = %v", sc.Name, o.Key, o.Model)
			}
		}
	}
}

func TestAftermathRulesComeFromTheCampaignsFile(t *testing.T) {
	f := newAmFixture(t)
	rules, err := f.svc.AftermathRules(f.ctx, 5)
	if err != nil || len(rules.Gains) == 0 {
		t.Fatalf("rules of the campaign's file: %v, %v", rules, err)
	}
}

func TestLockMessageNamesEveryPendingAftermath(t *testing.T) {
	ref := func(n ...int) (out []AftermathRef) {
		for _, x := range n {
			out = append(out, AftermathRef{MatchNumber: x})
		}
		return
	}
	for _, c := range []struct {
		refs []AftermathRef
		want string
	}{
		{nil, ""},
		{ref(3), "Finish the aftermath of Match 3 first."},
		{ref(3, 4), "Finish the aftermaths of Matches 3 and 4 first."},
		{ref(3, 4, 10), "Finish the aftermaths of Matches 3, 4 and 10 first."},
	} {
		if got := LockMessage(c.refs); got != c.want {
			t.Errorf("%v: %q, want %q", c.refs, got, c.want)
		}
	}
	if err := (CompanyState{Aftermaths: ref(3, 4)}).Locked(); err == nil || !strings.Contains(err.Error(), "Matches 3 and 4") {
		t.Errorf("Locked: %v", err)
	}
}

func TestAftermathRerollAndBonusDieCanBeTyped(t *testing.T) {
	f := newAmFixture(t)
	f.start(t, AftermathInput{AgainstTheOdds: []string{"reroll"}, Models: casualties(8, 9).Models})
	f.roll(t, 8, 2, 3) // 5: injured
	f.roll(t, 9, 3, 3) // 6: recovery
	if _, err := f.svc.RerollCasualty(f.ctx, f.owner(), 5, 1, 1, 8, []int{7, 1}, false); !errors.Is(err, ErrInvalidAftermath) {
		t.Fatalf("a die is 1 to 6: %v", err)
	}
	a, err := f.svc.RerollCasualty(f.ctx, f.owner(), 5, 1, 1, 8, []int{6, 6}, false) // typed: 12, Lesson Learned
	if err != nil || a.Casualties[0].Total != 12 {
		t.Fatalf("typed re-roll: %+v %v", a.Casualties[0], err)
	}
	if d := a.Casualties[0].Decision; d == nil || d.Kind != "bonus" {
		t.Fatalf("the re-rolled result asks for its own bonus die: %+v", a.Casualties[0])
	}
	f.dice.Rolls, f.dice.n = []int{6}, 0
	a, err = f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{UnitID: 8, Kind: "bonus", Answer: "0"}) // roll it for me: a 6 is a 3 on a D3
	if err != nil || a.Casualties[0].Bonus != 3 {
		t.Fatalf("server die: %+v %v", a.Casualties[0], err)
	}
	if _, err := f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{UnitID: 8, Kind: "bonus", Answer: "1"}); !errors.Is(err, ErrRefused) {
		t.Errorf("answered twice: %v", err)
	}
}

type stubLive struct {
	sheets []ModelSheet
	ok     bool
	calls  int
}

func (s *stubLive) AftermathDefaults(context.Context, int64, int64, int64) ([]ModelSheet, bool, error) {
	s.calls++
	return s.sheets, s.ok, nil
}

func TestAftermathDefaultSheetComesFromTheLiveMatch(t *testing.T) {
	liveSheets := []ModelSheet{
		{UnitID: 1, Outcome: OutcomeSurvived, Gains: map[string]int{participatedKey: 1, GainWoundedEnemy: 1, GainFatePrevented: 1, GainBrokenPassed: 1, GainSlainHeroMons: 2}},
		{UnitID: 5, Outcome: OutcomeFled, Gains: map[string]int{participatedKey: 1}},
		{UnitID: 6, Outcome: OutcomeCasualty, Gains: map[string]int{participatedKey: 1}},
	}
	// no live data, or no live service: exactly today's default
	for name, wire := range map[string]func(*amFixture){"no service": func(*amFixture) {}, "no data": func(f *amFixture) { f.svc.WithLive(&stubLive{}) }} {
		f := newAmFixture(t)
		wire(f)
		a, err := f.svc.Aftermath(f.ctx, f.owner(), 5, 1, 1)
		if err != nil || a.FromLive {
			t.Fatalf("%s: %+v %v", name, a.FromLive, err)
		}
		want := DefaultSheet(BuildDetail(shippedRules(t), CampaignSettings{ManualRolls: true}, f.state).Roster)
		if len(a.Input.Models) != len(want.Models) || a.Input.Models[0].Outcome != OutcomeSurvived || a.Preview.Rolls != 0 {
			t.Errorf("%s: %+v", name, a.Input)
		}
	}

	f := newAmFixture(t)
	live := &stubLive{sheets: liveSheets, ok: true}
	f.svc.WithLive(live)
	a, err := f.svc.Aftermath(f.ctx, f.owner(), 5, 1, 1)
	if err != nil || !a.FromLive {
		t.Fatalf("%v %v", a.FromLive, err)
	}
	m1, _ := a.Input.Model(1)
	m5, _ := a.Input.Model(5)
	m6, _ := a.Input.Model(6)
	m2, _ := a.Input.Model(2)
	if m1.Gains[GainSlainHeroMons] != 2 || m1.Gains[GainWoundedEnemy] != 1 || m1.Gains[GainFatePrevented] != 1 || m1.Gains[GainBrokenPassed] != 1 || m1.Gains[participatedKey] != 1 {
		t.Errorf("gains: %+v", m1)
	}
	if m5.Outcome != OutcomeFled || m6.Outcome != OutcomeCasualty || m2.Outcome != OutcomeSurvived {
		t.Errorf("outcomes: %v %v %v (a roster unit missing from the live result keeps the old default)", m5.Outcome, m6.Outcome, m2.Outcome)
	}
	if a.Preview.Rolls != 1 {
		t.Errorf("only the casualty rolls for injuries, the fled unit does not: %d", a.Preview.Rolls)
	}
	byHand := Preview(AftermathContext{Rules: f.mustRules(t), Outcome: "win", Scenario: "Rescue", Result: f.match.Result.Company(1), Roster: BuildDetail(shippedRules(t), CampaignSettings{ManualRolls: true}, f.state).Roster}, a.Input)
	if len(byHand.Experience) != len(a.Preview.Experience) || byHand.Rolls != a.Preview.Rolls {
		t.Errorf("the preview of a default is the preview of its sheet: %+v vs %+v", byHand, a.Preview)
	}
	if st := f.start(t, a.Input); st.FromLive {
		t.Error("a started sheet is not from live")
	}
	calls := live.calls
	if _, err := f.svc.Aftermath(f.ctx, f.owner(), 5, 1, 1); err != nil || live.calls != calls {
		t.Errorf("a started aftermath never reads the live log again: %d -> %d, %v", calls, live.calls, err)
	}
}

func (f *amFixture) mustRules(t *testing.T) AftermathRules {
	t.Helper()
	r, err := f.svc.aftermathRules(f.ctx, "bc")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestVantagePointMovesTheSecondScenarioDie(t *testing.T) {
	f := newAmFixture(t)
	f.match.Ended = false
	perks := []Perk{{ID: 3, Effect: PerkVantagePoint, When: "nextScenarioTableRoll"}}
	f.repo.EXPECT().Perks(gomock.Any(), int64(5), int64(1)).DoAndReturn(func(context.Context, int64, int64) ([]Perk, error) { return perks, nil }).AnyTimes()
	used := 0
	f.repo.EXPECT().UsePerk(gomock.Any(), int64(5), int64(1), int64(3)).DoAndReturn(func(context.Context, int64, int64, int64) error { used++; return nil }).AnyTimes()
	if _, _, err := f.svc.UseVantagePoint(f.ctx, f.owner(), 5, 1, 1, 6, 1); !errors.Is(err, ErrInvalidRoll) {
		t.Errorf("past six: %v", err)
	}
	if _, _, err := f.svc.UseVantagePoint(f.ctx, f.owner(), 5, 1, 1, 3, 2); !errors.Is(err, ErrInvalidRoll) {
		t.Errorf("+2: %v", err)
	}
	if _, _, err := f.svc.UseVantagePoint(f.ctx, User{ID: 8}, 5, 1, 1, 3, 1); !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrRefused) {
		t.Errorf("someone else: %v", err)
	}
	if used != 0 {
		t.Fatal("nothing is used by a refused call")
	}
	die, line, err := f.svc.UseVantagePoint(f.ctx, f.owner(), 5, 1, 1, 3, 1)
	if err != nil || die != 4 || line != "3, Vantage Point +1 = 4" || used != 1 {
		t.Errorf("got %d %q %v (used %d)", die, line, err, used)
	}
	perks = nil
	if _, _, err := f.svc.UseVantagePoint(f.ctx, f.owner(), 5, 1, 1, 3, 1); !errors.Is(err, ErrRefused) {
		t.Errorf("no perk left: %v", err)
	}
	f.match.Ended = true
	perks = []Perk{{ID: 3, Effect: PerkVantagePoint}}
	if _, _, err := f.svc.UseVantagePoint(f.ctx, f.owner(), 5, 1, 1, 3, 1); !errors.Is(err, ErrRefused) {
		t.Errorf("ended match: %v", err)
	}
}

type tallyLive struct{ stubLive }

func (tallyLive) MVPHint(context.Context, int64, int64, int64) (*MVPHint, error) {
	return &MVPHint{Tied: []int64{1, 5}, Lines: []TallyLine{{UnitID: 1, Votes: 1}, {UnitID: 5, Votes: 1}}}, nil
}

func TestAftermathViewCarriesTheMVPTally(t *testing.T) {
	f := newAmFixture(t)
	f.svc.WithLive(&tallyLive{stubLive{sheets: []ModelSheet{{UnitID: 1, Outcome: OutcomeSurvived, Gains: map[string]int{participatedKey: 1}}}, ok: true}})
	a, err := f.svc.Aftermath(f.ctx, f.owner(), 5, 1, 1)
	if err != nil || a.MVP == nil || len(a.MVP.Tied) != 2 || !a.FromLive {
		t.Fatalf("%+v %v", a.MVP, err)
	}
	f2 := newAmFixture(t)
	f2.svc.WithLive(&stubLive{})
	if a, _ := f2.svc.Aftermath(f2.ctx, f2.owner(), 5, 1, 1); a.MVP != nil {
		t.Error("no live data, no tally")
	}
}

func TestAftermathLinesCarryTheMatchAndADate(t *testing.T) {
	f := newAmFixture(t)
	ended := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	f.match.EndedOn = &ended
	f.rec.Lines = []AftermathLine{{Kind: "note", Text: "a line"}}
	a, err := f.svc.Aftermath(f.ctx, f.owner(), 5, 1, 1)
	if err != nil || len(a.Lines) != 1 {
		t.Fatalf("Aftermath = %+v, %v", a, err)
	}
	if l := a.Lines[0]; l.Match != 3 || l.MatchID != 1 || !l.On.Equal(ended) {
		t.Errorf("without a start the line is dated by the end of the match: %+v", l)
	}
	started, finished := ended.Add(time.Hour), ended.Add(2*time.Hour)
	f.rec.StartedAt = &started
	if a, _ = f.svc.Aftermath(f.ctx, f.owner(), 5, 1, 1); !a.Lines[0].On.Equal(started) {
		t.Errorf("started: %+v", a.Lines[0])
	}
	f.rec.FinishedAt = &finished
	if a, _ = f.svc.Aftermath(f.ctx, f.owner(), 5, 1, 1); !a.Lines[0].On.Equal(finished) {
		t.Errorf("finished: %+v", a.Lines[0])
	}
}

func TestAftermathDeadSergeantIsReplacedByAHeroOptionally(t *testing.T) {
	f := newAmFixture(t)
	f.start(t, casualties(3)) // Cene, a Sergeant; Dal (4) is a Hero
	a := f.roll(t, 3, 1, 1)   // Dead
	if a.Status != AftermathInjuries || a.Succession == nil || a.Succession.NeedLeader || !a.Succession.NeedSergeant || !slices.Equal(a.Succession.SergeantOptions, []int64{4, 5}) {
		t.Fatalf("the Leader lives, a Sergeant place is open: %+v", a.Succession)
	}
	a, err := f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{Kind: "sergeant", Answer: ""})
	if err != nil || a.Status != AftermathLevelUps || a.Succession != nil {
		t.Fatalf("a Hero may be passed over: %+v %v", a, err)
	}
}

func TestAftermathDeadSergeantNeedsAWarriorWhenNoHeroIsLeft(t *testing.T) {
	f := newAmFixture(t)
	for i := range f.state.Units {
		if f.state.Units[i].ID == 4 || f.state.Units[i].ID == 5 {
			f.state.Units[i].Role = RoleWarrior
		}
	}
	f.start(t, casualties(3))
	a := f.roll(t, 3, 1, 1)
	if a.Succession == nil || !a.Succession.NeedSergeant || len(a.Succession.Fallback) == 0 || len(a.Succession.SergeantOptions) != 0 {
		t.Fatalf("a Warrior must step up: %+v", a.Succession)
	}
	if _, err := f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{Kind: "sergeant", Answer: ""}); !errors.Is(err, ErrInvalidAftermath) {
		t.Errorf("the place is not optional: %v", err)
	}
	if _, err := f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{Kind: "sergeant", Answer: "1"}); !errors.Is(err, ErrInvalidAftermath) {
		t.Errorf("only the Warriors with the most Experience: %v", err)
	}
	pick := strconv.FormatInt(a.Succession.Fallback[0], 10)
	a, err := f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{Kind: "sergeant", Answer: pick})
	if err != nil || a.Status != AftermathLevelUps {
		t.Fatalf("%+v %v", a, err)
	}
	id, _ := strconv.ParseInt(pick, 10, 64)
	if u := f.unit(t, id); u.Role != RoleSergeant {
		t.Errorf("the Warrior is a Sergeant now: %+v", u)
	}
	for _, us := range f.state.Units {
		if us.ID == id && us.Bonus["might"] < 1 {
			t.Errorf("treated as having rolled Hero in the Making: %+v", us.Bonus)
		}
	}
}

// Two officers lost in the same match: the Leader and a Sergeant. The remaining Sergeant leads, then a Hero may fill the place.
func TestAftermathLeaderAndSergeantLostTogether(t *testing.T) {
	f := newAmFixture(t)
	f.start(t, casualties(1, 3))
	f.roll(t, 1, 1, 1)
	a := f.roll(t, 3, 1, 1)
	if a.Succession == nil || !a.Succession.NeedLeader || !slices.Equal(a.Succession.LeaderOptions, []int64{2}) {
		t.Fatalf("the Sergeant left leads: %+v", a.Succession)
	}
	a, err := f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{Kind: "succession", Answer: "2"})
	if err != nil || a.Succession == nil || !a.Succession.NeedSergeant || a.Succession.NeedLeader || len(a.Succession.SergeantOptions) == 0 {
		t.Fatalf("a Sergeant is chosen next: %+v %v", a.Succession, err)
	}
	a, err = f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{Kind: "sergeant", Answer: "4"})
	if err != nil || f.unit(t, 4).Role != RoleSergeant || f.unit(t, 2).Role != RoleLeader {
		t.Fatalf("%+v %v", a, err)
	}
	// still fewer than two Sergeants and another Hero: the place stays optional
	if a.Succession == nil || !a.Succession.NeedSergeant {
		t.Fatalf("the second place is still offered: %+v", a.Succession)
	}
	if a, err = f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{Kind: "sergeant", Answer: ""}); err != nil || a.Status != AftermathLevelUps {
		t.Fatalf("it can be left empty: %+v %v", a, err)
	}
}

// Both Sergeants lost with no Hero left: two Warriors step up, one question after the other, then the place is no longer open.
func TestAftermathBothSergeantsLostAndNoHeroLeft(t *testing.T) {
	f := newAmFixture(t)
	for i := range f.state.Units {
		if id := f.state.Units[i].ID; id == 4 || id == 5 {
			f.state.Units[i].Role = RoleWarrior
		}
	}
	f.start(t, casualties(2, 3))
	f.roll(t, 2, 1, 1)
	a := f.roll(t, 3, 1, 1)
	for i := 0; i < 2; i++ {
		if a.Succession == nil || len(a.Succession.Fallback) == 0 {
			t.Fatalf("question %d: a Warrior must step up: %+v", i+1, a.Succession)
		}
		var err error
		a, err = f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{Kind: "sergeant", Answer: strconv.FormatInt(a.Succession.Fallback[0], 10)})
		if err != nil {
			t.Fatal(err)
		}
	}
	if a.Status != AftermathLevelUps || a.Succession != nil {
		t.Errorf("both places are filled: %+v", a)
	}
}

func TestAHeroInTheMakingNamedSergeantStaysSergeant(t *testing.T) {
	rs := shippedRules(t)
	for _, tt := range []struct{ role, want UnitRole }{{RoleWarrior, RoleHero}, {RoleSergeant, RoleSergeant}, {RoleLeader, RoleLeader}} {
		s := UnitState{Role: tt.role}
		applyProgress(rs, &s, &Progress{Hero: true, Path: "Path of the Soldier"})
		if s.Role != tt.want {
			t.Errorf("a hero holding the %s place is %s, want %s", tt.role, s.Role, tt.want)
		}
	}
}
