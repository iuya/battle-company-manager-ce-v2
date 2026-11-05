package core

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"
)

// docs/matches.md, "Against the Odds before the game": the lower rated player always decides, and the game waits for it.

// gateFixture is the Against the Odds fixture with the match store answering from f.match, as the service reads it.
func gateFixture(t *testing.T) *atoFixture {
	f := ownedATOFixture(t)
	f.match.Sides[0].Companies[0].Points, f.match.Sides[1].Companies[0].Points = 0, 0
	f.matches.EXPECT().Get(gomock.Any(), int64(5), int64(1)).DoAndReturn(func(context.Context, int64, int64) (Match, error) { return f.match, nil }).AnyTimes()
	f.matches.EXPECT().Events(gomock.Any(), int64(5), int64(1)).Return(nil, nil).AnyTimes()
	f.matches.EXPECT().Update(gomock.Any(), int64(5), int64(1), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ int64, _ User, fn func(*Match) error) (Match, error) {
			m := f.match
			if err := fn(&m); err != nil {
				return Match{}, err
			}
			m.Revision++
			f.match = m
			return m, nil
		}).AnyTimes()
	return f
}

func blocks(t *testing.T, f *atoFixture) string {
	t.Helper()
	why, err := f.svc.ATOBlocks(f.ctx, 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	return why
}

func TestTheGameWaitsForTheLowerRatedPlayer(t *testing.T) {
	f := gateFixture(t)
	const want = "Waiting for Rangers to choose Against the Odds."
	if got := blocks(t, f); got != want {
		t.Fatalf("an undecided match with a gap: %q", got)
	}
	if _, err := f.svc.ProposeEnd(f.ctx, User{ID: 20, DisplayName: "Rival"}, 5, 1, 3); !errors.Is(err, ErrRefused) {
		t.Errorf("the rival cannot end it: %v", err)
	}
	if c := f.card(t, 20); !c.Pending || c.Decided || c.Access != ATONone {
		t.Errorf("the rival reads a waiting card: %+v", c)
	}

	// the lower rated owner confirms, whoever created the match
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 3, ATOSelection{}); err != nil {
		t.Fatal(err)
	}
	if !f.match.ATODecided || blocks(t, f) != "" {
		t.Errorf("playing without bonuses is a decision: %+v", f.match.ATODecided)
	}
	if c := f.card(t, 20); c.Pending || !c.Decided {
		t.Errorf("the rival's card: %+v", c)
	}
}

func TestTheOrganiserCanConfirmForTheLowerRatedPlayer(t *testing.T) {
	f := gateFixture(t)
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: matchOrganiser}, 5, 1, 3, ATOSelection{Bonuses: []string{"reroll"}}); err != nil {
		t.Fatal(err)
	}
	if !f.match.ATODecided || !f.rec.Selection.Has("reroll") {
		t.Errorf("confirmed for them: %v %+v", f.match.ATODecided, f.rec)
	}
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 20}, 5, 1, 4, ATOSelection{}); !errors.Is(err, ErrRefused) {
		t.Errorf("not the rival: %v", err)
	}
}

func TestChangeReopensTheDecisionUntilTheGameStarts(t *testing.T) {
	f := gateFixture(t)
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 3, ATOSelection{Bonuses: []string{"reroll"}}); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		user int64
		rev  int
		want error
	}{"the rival": {20, 4, ErrRefused}, "a stale revision": {10, 3, ErrMatchChanged}} {
		if _, err := f.svc.ReopenAgainstTheOdds(f.ctx, User{ID: tc.user}, 5, 1, tc.rev); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
	f.started = true
	if _, err := f.svc.ReopenAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 4); !errors.Is(err, ErrRefused) {
		t.Errorf("the game has started: %v", err)
	}
	f.started = false // every entry undone: it can change again
	c, err := f.svc.ReopenAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 4)
	if err != nil || f.match.ATODecided || !c.Pending || !c.Selection.Has("reroll") {
		t.Fatalf("reopened, keeping the selection: %+v %v", c, err)
	}
	if blocks(t, f) == "" {
		t.Error("the game waits again")
	}
}

func TestNothingBlocksWhenThereIsNothingToDecide(t *testing.T) {
	f := gateFixture(t)
	f.started = true
	if got := blocks(t, f); got != "" {
		t.Errorf("a game that started never stops, whatever the ratings do: %q", got)
	}
	f.started = false
	f.match.Ended = true
	if got := blocks(t, f); got != "" {
		t.Errorf("an ended match: %q", got)
	}
	f.match.Ended = false

	// not one company a side
	three := f.match
	three.Sides = append([]MatchSide{}, f.match.Sides...)
	three.Sides[1].Companies = append(append([]CampaignCompany{}, three.Sides[1].Companies...), CampaignCompany{ID: 3, OwnerID: 30})
	saved := f.match
	f.match = three
	if got := blocks(t, f); got != "" {
		t.Errorf("not a one against one: %q", got)
	}
	f.match = saved

	// equal ratings: the same company twice has the same Rating
	equal := f.match
	equal.Sides = []MatchSide{{Companies: []CampaignCompany{{ID: 1, OwnerID: 10, Name: "Rangers"}}}, {Companies: []CampaignCompany{{ID: 1, OwnerID: 20, Name: "Rangers"}}}}
	f.match = equal
	if got := blocks(t, f); got != "" {
		t.Errorf("equal ratings: %q", got)
	}
}

func TestAPracticeMatchFollowsTheSameRules(t *testing.T) {
	f := gateFixture(t)
	f.match.Practice = true
	if blocks(t, f) == "" { // the fixture's gap pays for Table bonuses (Tool Kit, Wanderer, Ambush!)
		t.Error("a practice match waits too")
	}
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 3, ATOSelection{Bonuses: []string{"reroll"}}); !errors.Is(err, ErrRefused) {
		t.Errorf("no reward bonus in practice: %v", err)
	}
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 3, ATOSelection{}); err != nil || blocks(t, f) != "" {
		t.Errorf("confirmed: %v", err)
	}
}

func TestAPAyableGap(t *testing.T) {
	rules := realATO(t)
	for _, tc := range []struct {
		name        string
		gap         int
		practice    bool
		hasWanderer bool
		want        bool
	}{
		{"a gap below the cheapest bonus", 14, false, false, false},
		{"exactly the cheapest bonus: the rules allow equal", 15, false, false, true},
		{"practice has no reward bonus", 29, true, false, false},
		{"practice can pay for a Tool Kit", 30, true, false, true},
	} {
		if got := atoPayable(rules, tc.gap, tc.practice, tc.hasWanderer); got != tc.want {
			t.Errorf("%s: %v", tc.name, got)
		}
	}
	only := ATORules{Inclusive: true, Bonuses: []ATOBonusRule{{Key: atoWanderer, Rating: 45}}}
	if atoPayable(only, 60, false, true) || !atoPayable(only, 60, false, false) {
		t.Error("a company with a Wanderer cannot take another")
	}
	if atoPayable(ATORules{Bonuses: []ATOBonusRule{{Key: "reroll", Rating: 15}}}, 15, false, false) {
		t.Error("without inclusive the price must stay under the gap")
	}
}

func TestChangingTheScenarioReopensTheDecision(t *testing.T) {
	f := gateFixture(t)
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 3, ATOSelection{}); err != nil {
		t.Fatal(err)
	}
	// the same one again changes nothing
	same := *f.match.Scenario
	if _, err := f.svc.SetScenario(f.ctx, User{ID: 10}, 5, 1, f.match.Revision, same); err != nil || !f.match.ATODecided {
		t.Fatalf("the same scenario: %v %v", err, f.match.ATODecided)
	}
	next := ScenarioInput{Kind: ScenarioCustom, Name: "The ford"}
	if _, err := f.svc.SetScenario(f.ctx, User{ID: 10}, 5, 1, f.match.Revision, next); err != nil || f.match.ATODecided {
		t.Fatalf("another scenario reopens it: %v %v", err, f.match.ATODecided)
	}
	f.match.ATODecided = true
	if _, err := f.svc.ClearScenario(f.ctx, User{ID: 10}, 5, 1, f.match.Revision); err != nil || f.match.ATODecided {
		t.Fatalf("removing it too: %v %v", err, f.match.ATODecided)
	}
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, f.match.Revision, ATOSelection{}); !errors.Is(err, ErrRefused) {
		t.Errorf("without a scenario nothing is confirmed: %v", err)
	}
	if !f.card(t, 10).Pending || f.card(t, 10).HasScenario {
		t.Errorf("the card says so: %+v", f.card(t, 10))
	}
}

func TestAmbushFixesTheScenarioToTheCard(t *testing.T) {
	f := gateFixture(t)
	other := mustScenarios(t, f)[0].Name
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 3, ATOSelection{Bonuses: []string{"ambush"}, Ambush: other}); err != nil {
		t.Fatal(err)
	}
	before := f.match.Revision
	if _, err := f.svc.SetScenario(f.ctx, User{ID: 10}, 5, 1, before, ScenarioInput{Kind: ScenarioCustom, Name: "The ford"}); !errors.Is(err, ErrRefused) {
		t.Errorf("no change by hand: %v", err)
	}
	if _, err := f.svc.ClearScenario(f.ctx, User{ID: 10}, 5, 1, before); !errors.Is(err, ErrRefused) {
		t.Errorf("no removal by hand: %v", err)
	}
	rated, err := f.svc.Match(f.ctx, 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	st, err := f.svc.ATOStatus(f.ctx, 5, rated)
	if err != nil || st.Ambush != other || st.Pending {
		t.Errorf("the status names it: %+v %v", st, err)
	}
	if f.match.Scenario.Name != other || !f.match.ATODecided {
		t.Errorf("unchanged: %+v", f.match)
	}
}

// LiveService.Apply refuses while the gate answers a reason, and does not ask once the match is decided.
func TestLiveApplyAsksTheGate(t *testing.T) {
	f := newLiveFixture(t)
	lc, err := f.svc.Live(f.ctx, f.user(liveOwner1), 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	asked := 0
	f.svc.WithATOGate(gateFunc(func() (string, error) { asked++; return "Waiting for Rangers to choose Against the Odds.", nil }))
	for name, a := range map[string]LiveAction{
		"a game action":    {Kind: LiveBroken, UnitID: firstUnit(lc)},
		"the end of match": {Kind: LiveObjective, UnitID: firstUnit(lc)},
	} {
		if _, _, err := f.svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, 1, a); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "Waiting for Rangers") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if asked != 2 {
		t.Errorf("the gate was asked %d times", asked)
	}
	f.svc.WithATOGate(gateFunc(func() (string, error) { return "", nil }))
	if _, e, err := f.svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, 1, LiveAction{Kind: LiveBroken, UnitID: firstUnit(lc)}); err != nil || e.Text == "" {
		t.Errorf("decided: %+v %v", e, err)
	}
}

type gateFunc func() (string, error)

func (g gateFunc) ATOBlocks(context.Context, int64, int64) (string, error) { return g() }
