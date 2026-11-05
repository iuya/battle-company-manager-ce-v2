package core

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"
)

// docs/matches.md, "New match" and "Ending a match": the scenario at creation, the scenario fixed once the game started, and the mandatory MVP votes.

// ownedATOFixture is newATOFixture with the companies' owners on the company records (Rangers: 10, Raiders: 20), as the new match form
// reads them.
func ownedATOFixture(t *testing.T) *atoFixture {
	f := newATOFixture(t)
	inner := f.companies
	f.companies = NewMockcompanyRepository(gomock.NewController(t))
	owners := map[int64]int64{1: 10, 2: 20}
	f.companies.EXPECT().Read(gomock.Any(), int64(5), gomock.Any()).DoAndReturn(func(ctx context.Context, c, id int64) (CompanyState, error) {
		st, err := inner.Read(ctx, c, id)
		st.Company.OwnerID = owners[id]
		return st, err
	}).AnyTimes()
	f.companies.EXPECT().ReadRoster(gomock.Any(), int64(5)).DoAndReturn(func(ctx context.Context, c int64) ([]CompanyState, error) {
		list, err := inner.ReadRoster(ctx, c)
		for i := range list {
			list[i].Company.OwnerID = owners[list[i].Company.ID]
		}
		return list, err
	}).AnyTimes()
	f.svc = NewMatches(f.campaigns, f.companies, f.matches, staticRules{shippedRules(t)}).WithATO(f.repo)
	return f
}

func TestCreateMatchStoresTheScenario(t *testing.T) {
	f := ownedATOFixture(t)
	scenarios := mustScenarios(t, f)
	sides := []NewSide{{Companies: []int64{1}}, {Companies: []int64{2}}}
	var got NewMatch
	f.matches.EXPECT().Create(gomock.Any(), int64(5), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ int64, _ User, in NewMatch) (Match, error) {
		got = in
		return f.match, nil
	}).AnyTimes()
	create := func(user int64, in NewMatch) error {
		got = NewMatch{}
		_, err := f.svc.CreateMatch(f.ctx, User{ID: user}, 5, in)
		return err
	}
	// whoever creates it: the player above as much as the one below
	for _, user := range []int64{10, 20} {
		if err := create(user, NewMatch{Sides: sides, Scenario: &ScenarioInput{Kind: ScenarioRulebook, Name: scenarios[0].Name}}); err != nil {
			t.Fatal(err)
		}
		if got.Scenario == nil || got.Scenario.Name != scenarios[0].Name {
			t.Errorf("user %d: the scenario is stored with the match: %+v", user, got.Scenario)
		}
	}
	for name, in := range map[string]NewMatch{
		"unknown scenario":       {Sides: sides, Scenario: &ScenarioInput{Kind: ScenarioRulebook, Name: "Nope"}},
		"one-off without a name": {Sides: sides, Scenario: &ScenarioInput{Kind: ScenarioCustom}},
	} {
		if err := create(10, in); !errors.Is(err, ErrInvalidMatch) {
			t.Errorf("%s: %v", name, err)
		}
		if got.Sides != nil {
			t.Errorf("%s: nothing is created", name)
		}
	}
}

func TestTheScenarioIsFixedOnceTheGameStarted(t *testing.T) {
	f := newMatchesFixture(t)
	sc := f.firstScenario(t)
	started := false
	f.svc.GameStarted = func(context.Context, int64, int64) (bool, error) { return started, nil }
	in := ScenarioInput{Kind: ScenarioRulebook, Name: sc.Name}
	if _, err := f.svc.SetScenario(f.ctx, f.player(), 5, 1, 3, in); err != nil {
		t.Fatalf("before the game: %v", err)
	}
	started = true
	if _, err := f.svc.SetScenario(f.ctx, f.player(), 5, 1, 4, in); !errors.Is(err, ErrRefused) {
		t.Errorf("SetScenario after the first live entry: %v", err)
	}
	if _, err := f.svc.ClearScenario(f.ctx, f.player(), 5, 1, 4); !errors.Is(err, ErrRefused) {
		t.Errorf("ClearScenario after the first live entry: %v", err)
	}
	if ok, err := f.svc.MatchStarted(f.ctx, 5, 1); !ok || err != nil {
		t.Errorf("MatchStarted = %v, %v", ok, err)
	}
	f.match.Scenario = nil // a match that started without a scenario can still get one: it cannot end without it
	if m, err := f.svc.SetScenario(f.ctx, f.player(), 5, 1, 4, in); err != nil || m.Scenario == nil {
		t.Errorf("a started match without a scenario: %v", err)
	}
}

func TestLiveMissingVotes(t *testing.T) {
	f := newLiveFixture(t) // Rangers (owner 10) against Raiders (20) and Wolves (30)
	missing := func(owner int64) []MissingVote {
		t.Helper()
		out, err := f.svc.MissingVotes(f.ctx, 5, 1, owner)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got := missing(0); len(got) != 4 {
		t.Fatalf("every owner votes every opposing company: %+v", got)
	}
	if got := missing(liveOwner1); len(got) != 2 || got[0].CompanyID != 2 || got[1].CompanyID != 3 {
		t.Errorf("Rangers' owner: %+v", got)
	}
	for _, company := range []int64{2, 3} {
		lc, _ := f.svc.Live(f.ctx, f.user(liveOwner1), 5, company)
		if _, _, err := f.svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, company, LiveAction{Kind: LiveMVPVote, UnitID: lc.Units[0].UnitID}); err != nil {
			t.Fatal(err)
		}
	}
	if got := missing(liveOwner1); len(got) != 0 {
		t.Errorf("voted: %+v", got)
	}
	if got := missing(0); len(got) != 2 || got[0].OwnerID != liveOwner2 || got[1].OwnerID != liveOwner3 {
		t.Errorf("the others still vote: %+v", got)
	}
	f.match.Practice = true
	if got := missing(0); len(got) != 0 {
		t.Errorf("a practice match needs no vote: %+v", got)
	}
}
