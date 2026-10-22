package core

import (
	"errors"
	"strings"
	"testing"
)

// breakInits: company 1 has n units (ids 1..n, one Wound each), company 2 has three (ids 101-103).
func breakInits(n int) []LiveUnitInit {
	var out []LiveUnitInit
	for i := 1; i <= n; i++ {
		out = append(out, LiveUnitInit{UnitID: int64(i), CompanyID: 1, Wounds: 1})
	}
	for i := 101; i <= 103; i++ {
		out = append(out, LiveUnitInit{UnitID: int64(i), CompanyID: 2, Wounds: 1})
	}
	return out
}

// bp plays the steps through Check like the service and keeps the log; step.by 0 is a match-level entry.
type bp struct {
	t       *testing.T
	inits   []LiveUnitInit
	cfg     LiveFoldConfig
	entries []LiveEntry
}

func newBP(t *testing.T, n, percent int) *bp {
	cfg := LiveFoldConfig{Break: map[int64]BreakConfig{1: {Percent: percent}, 2: {Percent: percent}}, NoStandFast: map[int64]bool{}}
	return &bp{t: t, inits: breakInits(n), cfg: cfg}
}

func (b *bp) state() LiveState {
	s := FoldLiveWith(b.inits, b.entries, b.cfg)
	s.CompanyName = func(id int64) string { return map[int64]string{1: "Alpha", 2: "Beta"}[id] }
	return s
}

func (b *bp) try(by int64, a LiveAction) (string, error) {
	res, touches, text, err := b.state().Check(a, by)
	if err != nil {
		return "", err
	}
	b.entries = append(b.entries, LiveEntry{ID: int64(len(b.entries) + 1), Seq: int64(len(b.entries) + 1), CompanyID: by, UnitID: a.UnitID, Action: res, Touches: touches, Text: text})
	return text, nil
}

func (b *bp) do(by int64, a LiveAction) string {
	b.t.Helper()
	text, err := b.try(by, a)
	if err != nil {
		b.t.Fatalf("%s: %v", a.Kind, err)
	}
	return text
}

func (b *bp) fall(n int) { // n more units of company 1 die
	b.t.Helper()
	have := b.state().casualtiesOf(1)
	for i := 1; i <= n; i++ {
		b.do(1, LiveAction{Kind: LiveDied, UnitID: int64(have + i)})
	}
}

func (b *bp) turn() string {
	b.t.Helper()
	return b.do(0, LiveAction{Kind: LiveTurn, Turn: b.state().Turn() + 1})
}

func TestBreakPointEdges(t *testing.T) {
	tests := []struct {
		name       string
		models     int
		percent    int
		casualties int
		broken     bool
		breakPoint string
	}{
		{"13 models, 6 casualties is exactly half: not Broken", 13, 50, 6, false, "6.5"},
		{"13 models, 7 casualties", 13, 50, 7, true, "6.5"},
		{"12 models, 6 casualties is exactly half", 12, 50, 6, false, "6.0"},
		{"12 models, 7 casualties", 12, 50, 7, true, "6.0"},
		{"Muster of Isengard 66%: 8 of 13", 13, 66, 8, false, "8.5"},
		{"Muster of Isengard 66%: 9 of 13", 13, 66, 9, true, "8.5"},
		{"no casualties", 13, 50, 0, false, "6.5"},
		{"a percent of 0 never breaks", 13, 0, 13, false, "0.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newBP(t, tt.models, tt.percent)
			b.fall(tt.casualties)
			if got := b.state().Break(1); got.Broken || got.Casualties != tt.casualties || got.StartingModels != tt.models {
				t.Fatalf("before the turn: %+v", got)
			}
			if got := b.state().Break(1); got.WillBreak != tt.broken {
				t.Errorf("WillBreak = %v, want %v", got.WillBreak, tt.broken)
			}
			b.turn()
			got := b.state().Break(1)
			if got.Broken != tt.broken || got.BreakPoint() != tt.breakPoint {
				t.Errorf("after the turn: %+v (Break Point %s), want broken=%v %s", got, got.BreakPoint(), tt.broken, tt.breakPoint)
			}
			if tt.broken && (got.BrokenAtTurn != 1 || !got.Offer || got.WillBreak) {
				t.Errorf("broken state: %+v", got)
			}
		})
	}
}

func TestBrokenOnlyAtTheTurnBoundaryAndForGood(t *testing.T) {
	b := newBP(t, 13, 50)
	b.fall(7)
	if b.state().Break(1).Broken {
		t.Fatal("casualties alone never break a company: only a turn entry tests")
	}
	text := b.turn()
	if text != "Turn 1 started: Alpha is Broken" {
		t.Errorf("text: %q", text)
	}
	// a unit returns to the battle: the casualties drop, the company stays Broken
	b.do(1, LiveAction{Kind: LiveReturn, UnitID: 1})
	b.do(1, LiveAction{Kind: LiveReturn, UnitID: 2})
	b.turn()
	got := b.state().Break(1)
	if !got.Broken || got.BrokenAtTurn != 1 || got.Casualties != 5 || got.Offer {
		t.Errorf("Broken is permanent: %+v", got)
	}
	if b.state().Break(2).Broken {
		t.Error("the other company has no casualties")
	}
	if tx := b.turn(); strings.Contains(tx, "Broken") {
		t.Errorf("a company already Broken is not announced again: %q", tx)
	}
}

func TestUndoingTheTurnUnbreaksTheCompany(t *testing.T) {
	b := newBP(t, 13, 50)
	b.fall(7)
	b.turn()
	if !b.state().Break(1).Broken {
		t.Fatal("setup")
	}
	rest := b.entries[:len(b.entries)-1] // the log without the turn entry
	if err := ValidateLogWith(b.inits, rest, b.cfg); err != nil {
		t.Fatalf("the log without the turn is valid: %v", err)
	}
	if got := FoldLiveWith(b.inits, rest, b.cfg).Break(1); got.Broken || !got.WillBreak {
		t.Errorf("after the undo: %+v", got)
	}
	// but a casualty cannot be undone from under an All is Lost that depends on... see TestAllIsLost; here a later turn depends on the first
	b.turn()
	if err := ValidateLogWith(b.inits, append(append([]LiveEntry{}, b.entries[:7]...), b.entries[8:]...), b.cfg); !errors.Is(err, ErrLiveDepends) {
		t.Errorf("turn 2 needs turn 1: %v", err)
	}
}

func TestTurnEntriesAreCheckedForStaleness(t *testing.T) {
	b := newBP(t, 13, 50)
	if _, err := b.try(0, LiveAction{Kind: LiveTurn, Turn: 2}); !errors.Is(err, ErrInvalidLive) {
		t.Errorf("the first turn is 1: %v", err)
	}
	b.turn()
	if _, err := b.try(0, LiveAction{Kind: LiveTurn, Turn: 1}); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "Turn 1 has already started") {
		t.Errorf("a second press is refused: %v", err)
	}
	if _, err := b.try(0, LiveAction{Kind: LiveTurn, Turn: 3}); !errors.Is(err, ErrInvalidLive) {
		t.Errorf("a turn cannot be skipped: %v", err)
	}
	if b.state().Turn() != 1 {
		t.Errorf("turn %d", b.state().Turn())
	}
}

func TestBreakPointCountsFromTheLog(t *testing.T) {
	b := newBP(t, 13, 50)
	b.do(1, LiveAction{Kind: LiveRetreat, UnitID: 1, Why: "courage"})
	b.do(1, LiveAction{Kind: LiveRetreat, UnitID: 2, Why: "left"})
	b.do(1, LiveAction{Kind: LiveWound, UnitID: 3, Targets: []WoundTarget{{UnitID: 101, Caused: 1}}}) // a company-2 unit falls: not company 1's
	if got := b.state().Break(1); got.Casualties != 1 {
		t.Errorf("failed courage counts, leaving the board does not: %+v", got)
	}
	b.do(2, LiveAction{Kind: LiveWound, UnitID: 102, Targets: []WoundTarget{{UnitID: 4, Caused: 1}}})
	if got := b.state().Break(1); got.Casualties != 2 {
		t.Errorf("a Field casualty counts: %+v", got)
	}
	b.do(1, LiveAction{Kind: LiveReturn, UnitID: 4})
	if got := b.state().Break(1); got.Casualties != 1 {
		t.Errorf("return to the battle subtracts again: %+v", got)
	}
	if got := b.state().Break(2); got.Casualties != 1 || got.StartingModels != 3 {
		t.Errorf("company 2: %+v", got)
	}
}

func TestBreakPointModelsAndExtras(t *testing.T) {
	b := newBP(t, 4, 50)
	b.cfg.Break[1] = BreakConfig{Percent: 50, Extra: 2} // a creature and a temporary Wanderer
	b.cfg.BreakModels = map[int64]int{1: 2}             // unit 1 counts as two models (a War Beast and its Commander)
	if got := b.state().Break(1); got.StartingModels != 4+1+2 {
		t.Errorf("starting models: %+v", got)
	}
	b.do(1, LiveAction{Kind: LiveDied, UnitID: 1})
	if got := b.state().Break(1); got.Casualties != 2 {
		t.Errorf("a unit that counts as two falls as two: %+v", got)
	}
	// the manual adjustment
	text := b.do(1, LiveAction{Kind: LiveBreakAdjust, Field: "start", Delta: -1, Note: "pony"})
	if text != "Alpha: 1 fewer starting model (pony)" {
		t.Errorf("text: %q", text)
	}
	b.do(1, LiveAction{Kind: LiveBreakAdjust, Field: "casualties", Delta: 2})
	got := b.state().Break(1)
	if got.StartingModels != 6 || got.Casualties != 4 {
		t.Errorf("adjusted: %+v", got)
	}
	// undo of the adjustment is just the fold without it
	if got := FoldLiveWith(b.inits, b.entries[:len(b.entries)-2], b.cfg).Break(1); got.StartingModels != 7 || got.Casualties != 2 {
		t.Errorf("without the adjustments: %+v", got)
	}
	for _, bad := range []LiveAction{
		{Kind: LiveBreakAdjust, Field: "start", Delta: 0},
		{Kind: LiveBreakAdjust, Field: "start", Delta: 10},
		{Kind: LiveBreakAdjust, Field: "models", Delta: 1},
		{Kind: LiveBreakAdjust, Field: "casualties", Delta: -9},
		{Kind: LiveBreakAdjust, Field: "start", Delta: -9},
	} {
		if _, err := b.try(1, bad); err == nil {
			t.Errorf("%+v should be refused", bad)
		}
	}
	if _, err := b.try(9, LiveAction{Kind: LiveBreakAdjust, Field: "start", Delta: 1}); !errors.Is(err, ErrRefused) {
		t.Errorf("a company that is not in the match: %v", err)
	}
	if got := FoldLive(b.inits, nil).Break(1); got.Percent != 0 || got.WillBreak {
		t.Errorf("without a config nothing ever breaks: %+v", got)
	}
}

func TestCourageTests(t *testing.T) {
	b := newBP(t, 4, 50)
	b.cfg.NoStandFast = map[int64]bool{2: true}
	b.fall(0)
	b.do(1, LiveAction{Kind: LiveDied, UnitID: 3})
	b.do(1, LiveAction{Kind: LiveDied, UnitID: 4})
	b.turn() // Broken at turn 1 (2 of 4 is not above half: 3 needed) -> not Broken, so tests are optional
	if b.state().Break(1).Broken {
		t.Fatal("setup: 2 of 4")
	}
	b.do(1, LiveAction{Kind: LiveDied, UnitID: 2})
	b.turn() // turn 2: 3 of 4
	if !b.state().Break(1).Broken {
		t.Fatal("setup: Broken at turn 2")
	}
	u := func(id int64) LiveUnit { x, _ := b.state().Unit(id); return x }
	if u(1).CourageTurn != 0 {
		t.Fatalf("not resolved yet: %+v", u(1))
	}
	b.do(1, LiveAction{Kind: LiveBroken, UnitID: 1, StandFast: true})
	if x := u(1); x.CourageHow != "passed" {
		t.Errorf("CourageHow after a pass: %q", x.CourageHow)
	}
	if x := u(1); x.CourageTurn != 2 || x.StandFastTurn != 2 || x.Gains[GainBrokenPassed] != 1 {
		t.Errorf("a rolled pass: %+v", x)
	}
	if _, err := b.try(1, LiveAction{Kind: LiveBroken, UnitID: 1, StandFast: true}); err != nil {
		t.Errorf("a hero passing again is allowed: %v", err)
	}
	// courageSkip: resolves without the experience flag
	text := b.do(1, LiveAction{Kind: LiveCourageSkip, UnitID: 1, Reason: "heroicResolve"})
	if text != "unit 1 passes courage automatically (Heroic Resolve)" {
		t.Errorf("text: %q", text)
	}
	if x := u(1); x.CourageTurn != 2 || x.CourageHow != "heroicResolve" {
		t.Errorf("%+v", x)
	}
	s := b.state()
	s.UnitName = func(id int64) string { return map[int64]string{3: "Cene"}[id] }
	if _, _, tx, err := s.Check(LiveAction{Kind: LiveCourageSkip, UnitID: 1, Reason: "engaged"}, 1); err != nil || !strings.Contains(tx, "takes no courage test (in combat)") {
		t.Errorf("exempt: %q %v", tx, err)
	}
	if _, err := b.try(1, LiveAction{Kind: LiveCourageSkip, UnitID: 1, Reason: "magic"}); !errors.Is(err, ErrInvalidLive) {
		t.Errorf("unknown reason: %v", err)
	}
	if _, err := b.try(1, LiveAction{Kind: LiveCourageSkip, UnitID: 3, Reason: "chariot"}); !errors.Is(err, ErrRefused) {
		t.Errorf("a unit outside the Field: %v", err)
	}
	b.do(1, LiveAction{Kind: LiveDied, UnitID: 1})
	if _, err := b.try(1, LiveAction{Kind: LiveBroken, UnitID: 2, StandFast: true}); !errors.Is(err, ErrRefused) {
		t.Errorf("a Warrior or Wanderer cannot call Stand Fast: %v", err)
	}
	// a failed test: the unit flees as a casualty and its courage is resolved this turn
	b2 := newBP(t, 4, 50)
	b2.turn()
	b2.do(1, LiveAction{Kind: LiveRetreat, UnitID: 1, Why: "courage"})
	if x, _ := b2.state().Unit(1); x.CourageTurn != 1 || x.CourageHow != "failed" || x.Zone != ZoneRetreated {
		t.Errorf("failed courage: %+v", x)
	}
	// a skip never earns the experience flag
	b2.do(1, LiveAction{Kind: LiveCourageSkip, UnitID: 2, Reason: "standFast"})
	if x, _ := b2.state().Unit(2); x.Gains[GainBrokenPassed] != 0 {
		t.Errorf("an automatic pass earns nothing: %+v", x)
	}
}

func TestAllIsLost(t *testing.T) {
	b := newBP(t, 13, 50)
	if _, err := b.try(1, LiveAction{Kind: LiveAllIsLost, Flee: true}); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "nothing to flee from") {
		t.Errorf("no offer yet: %v", err)
	}
	b.fall(7)
	b.turn()
	if got := b.state().Break(1); !got.Offer || got.Fled {
		t.Fatalf("the offer is open the turn the company breaks: %+v", got)
	}
	if _, err := b.try(2, LiveAction{Kind: LiveAllIsLost, Flee: true}); !errors.Is(err, ErrRefused) {
		t.Errorf("the other company has no offer: %v", err)
	}
	if text := b.do(1, LiveAction{Kind: LiveAllIsLost, Flee: true}); text != "Alpha fled the battlefield (All is Lost)" {
		t.Errorf("text: %q", text)
	}
	got := b.state().Break(1)
	if !got.Fled || got.Offer {
		t.Fatalf("fled: %+v", got)
	}
	for i := int64(8); i <= 13; i++ {
		if x, _ := b.state().Unit(i); x.Zone != ZoneRetreated || x.Reason != "Left the board" {
			t.Errorf("unit %d should have left the board: %+v", i, x)
		}
	}
	if got.Casualties != 7 {
		t.Errorf("leaving the board adds no casualties: %+v", got)
	}
	if _, err := b.try(1, LiveAction{Kind: LiveAllIsLost, Flee: false}); !errors.Is(err, ErrRefused) {
		t.Errorf("once only: %v", err)
	}
	// undo the flee: the units are back and the offer is open again
	if got := FoldLiveWith(b.inits, b.entries[:len(b.entries)-1], b.cfg).Break(1); got.Fled || !got.Offer {
		t.Errorf("after the undo: %+v", got)
	}
}

func TestAllIsLostFightOnAndTheOfferExpires(t *testing.T) {
	b := newBP(t, 13, 50)
	b.fall(7)
	b.turn()
	if text := b.do(1, LiveAction{Kind: LiveAllIsLost, Flee: false}); text != "Alpha fights on" {
		t.Errorf("text: %q", text)
	}
	if got := b.state().Break(1); got.Offer || got.Fled {
		t.Errorf("fought on: %+v", got)
	}
	if x, _ := b.state().Unit(8); x.Zone != ZoneField {
		t.Errorf("fighting on keeps the units: %+v", x)
	}
	// and an unanswered offer is gone at the next turn
	b2 := newBP(t, 13, 50)
	b2.fall(7)
	b2.turn()
	b2.turn()
	if got := b2.state().Break(1); got.Offer || !got.Broken {
		t.Errorf("the offer lasts until the next turn: %+v", got)
	}
	if _, err := b2.try(1, LiveAction{Kind: LiveAllIsLost, Flee: true}); !errors.Is(err, ErrRefused) {
		t.Errorf("too late: %v", err)
	}
}

func TestCanUndoATurnEntry(t *testing.T) {
	owners := map[int64]int64{1: 10, 2: 20}
	turn := LiveEntry{CompanyID: 0, Action: LiveAction{Kind: LiveTurn, Turn: 1}}
	for _, tt := range []struct {
		user User
		want bool
	}{{User{ID: 10}, true}, {User{ID: 20}, true}, {User{ID: 30}, false}, {User{ID: 99, Role: RoleAdmin}, true}} {
		if got := CanUndoEntry(tt.user, turn, owners, 50); got != tt.want {
			t.Errorf("user %d: %v", tt.user.ID, got)
		}
	}
}

func TestBreakRulesFromTheFile(t *testing.T) {
	rs := shippedRules(t)
	rules := rs.BC.BreakRules()
	if rules.DefaultPercent != 50 {
		t.Errorf("default percent %d", rules.DefaultPercent)
	}
	if p, ok := rs.BC.Profile("Muster of Isengard"); !ok || p.BreakPoint != 66 {
		t.Errorf("Muster of Isengard: %+v", p.BreakPoint)
	}
	for faction, want := range map[string]int{"Muster of Isengard": 66, "Gondor": 50, "No such company": 50} {
		if got := breakPointPercent(rs, faction); got != want {
			t.Errorf("breakPointPercent(%q) = %d, want %d", faction, got, want)
		}
	}
	ex := rules.ExceptionsFor("Stand Your Ground")
	if len(ex) != 1 || ex[0].Prose == "" || ex[0].Text() != ex[0].Prose {
		t.Errorf("exceptions: %+v", ex)
	}
	if got := rules.ExceptionsFor("Nothing", "maelstrom of battle"); len(got) != 1 || !strings.Contains(got[0].Text(), "not arrived yet count as being on the battlefield") {
		t.Errorf("setting: %+v", got)
	}
}

func TestLiveServiceTurnAndBreakPoint(t *testing.T) {
	f := newLiveFixture(t)
	lc, err := f.svc.Live(f.ctx, f.user(liveOwner1), 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	present := 0
	for _, u := range lc.Units {
		present += u.BreakModels
	}
	if lc.Break.Percent != 50 || lc.Break.StartingModels != present || present < 5 || lc.Break.Broken || lc.Turn != 0 {
		t.Fatalf("the starting count comes from the roster: %+v (%d present)", lc.Break, present)
	}
	// a stranger and a company-less user cannot press the button; either owner of a company in the match can
	if _, _, err := f.svc.Apply(f.ctx, f.user(7), 5, 1, 1, LiveAction{Kind: LiveTurn, Turn: 1}); !errors.Is(err, ErrRefused) {
		t.Errorf("a stranger: %v", err)
	}
	lc, e, err := f.svc.Apply(f.ctx, f.user(liveOwner2), 5, 1, 2, LiveAction{Kind: LiveTurn, Turn: 1})
	if err != nil || e.CompanyID != 0 || e.UnitID != 0 || e.Text != "Turn 1 started" || lc.Turn != 1 {
		t.Fatalf("turn 1: %+v %+v %v", lc, e, err)
	}
	// the other player pressing the same turn a moment later is refused, and sees the turn on their own company
	if _, _, err := f.svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, 1, LiveAction{Kind: LiveTurn, Turn: 1}); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "already started") {
		t.Errorf("stale: %v", err)
	}
	mine, _ := f.svc.Live(f.ctx, f.user(liveOwner1), 5, 1)
	if mine.Turn != 1 {
		t.Errorf("the turn is match level: %d", mine.Turn)
	}
	// the first owner can undo the second owner's turn entry (it belongs to the whole match)
	if _, err := f.svc.Undo(f.ctx, f.user(liveOwner1), 5, 1, e.ID, 1); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if mine, _ = f.svc.Live(f.ctx, f.user(liveOwner1), 5, 1); mine.Turn != 0 {
		t.Errorf("after the undo: %d", mine.Turn)
	}
	// a Warrior cannot call Stand Fast
	var warrior int64
	for _, u := range mine.Units {
		if u.UnitID%100 >= 6 { // the gondor() warriors have ids 6.. in their company
			warrior = u.UnitID
			break
		}
	}
	if warrior == 0 {
		t.Skip("no warrior in the fixture")
	}
	if _, _, err := f.svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, 1, LiveAction{Kind: LiveBroken, UnitID: warrior, StandFast: true}); !errors.Is(err, ErrRefused) {
		t.Errorf("stand fast by a warrior: %v", err)
	}
}

func TestRollCourage(t *testing.T) {
	f := newLiveFixture(t)
	lc, err := f.svc.Live(f.ctx, f.user(liveOwner1), 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	unit := firstUnit(lc)
	var courage int
	for _, a := range f.unitStats(t, 1, unit) {
		if a.Name == "Courage" {
			courage = a.Value
		}
	}
	if courage == 0 {
		t.Fatal("the fixture's units have a Courage")
	}
	f.svc.dice = &FixedDice{Rolls: []int{courage - 1, 1}}
	r, err := f.svc.RollCourage(f.ctx, f.user(liveOwner1), 5, 1, unit, nil, false)
	if err != nil || r.Total != courage || r.Courage != courage || !r.Passed || len(r.Dice) != 2 {
		t.Fatalf("a total equal to the Courage passes: %+v %v", r, err)
	}
	f.svc.dice = &FixedDice{Rolls: []int{1, 1}}
	if r, err = f.svc.RollCourage(f.ctx, f.user(liveOwner1), 5, 1, unit, nil, false); err != nil || r.Passed || r.Total != 2 {
		t.Fatalf("2 fails: %+v %v", r, err)
	}
	if snap, _ := f.repo.Read(f.ctx, 5, 1); len(snap.Entries) != 0 {
		t.Error("a roll writes nothing")
	}
	// refusals
	if _, err := f.svc.RollCourage(f.ctx, f.user(liveOwner2), 5, 1, unit, nil, false); !errors.Is(err, ErrRefused) {
		t.Errorf("another company's owner: %v", err)
	}
	if _, err := f.svc.RollCourage(f.ctx, f.user(liveOwner1), 5, 1, unit, []int{3, 4}, false); !errors.Is(err, ErrRefused) {
		t.Errorf("typed dice are refused when the campaign does not allow them: %v", err)
	}
	if _, err := f.svc.RollCourage(f.ctx, f.user(liveOwner1), 5, 1, unit, nil, true); !errors.Is(err, ErrRefused) {
		t.Errorf("thrown dice are refused when the campaign does not allow them: %v", err)
	}
	if _, err := f.svc.RollCourage(f.ctx, f.user(liveOwner1), 5, 1, 424242, nil, false); !errors.Is(err, ErrNotFound) {
		t.Errorf("an unknown unit: %v", err)
	}
	if _, _, err := f.svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, 1, LiveAction{Kind: LiveDied, UnitID: unit}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.RollCourage(f.ctx, f.user(liveOwner1), 5, 1, unit, nil, false); !errors.Is(err, ErrRefused) {
		t.Errorf("a unit that is out of the battle does not test: %v", err)
	}
}

func (f *liveFixture) unitStats(t *testing.T, company, unit int64) []Attribute {
	t.Helper()
	d, _, err := f.svc.detail(f.ctx, 5, company)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := rosterUnit(d.Roster, unit)
	return u.Stats
}

func TestMatchDefaults(t *testing.T) {
	f := newLiveFixture(t)
	if _, ok, err := f.svc.MatchDefaults(f.ctx, 5, 1); err != nil || ok {
		t.Fatalf("no live data yet: %v %v", ok, err)
	}
	for company, owner := range map[int64]int64{2: liveOwner2, 3: liveOwner3} { // the other team is two companies
		theirs, _ := f.svc.Live(f.ctx, f.user(owner), 5, company)
		for _, u := range theirs.Units {
			if _, _, err := f.svc.Apply(f.ctx, f.user(owner), 5, 1, company, LiveAction{Kind: LiveDied, UnitID: u.UnitID}); err != nil {
				t.Fatal(err)
			}
		}
		if company == 2 {
			d, _, _ := f.svc.MatchDefaults(f.ctx, 5, 1)
			if len(d.EnemyBroken) != 0 {
				t.Fatal("setup")
			}
		}
	}
	d, ok, err := f.svc.MatchDefaults(f.ctx, 5, 1)
	if err != nil || !ok || len(d.Fled) != 0 || len(d.EnemyBroken) != 0 {
		t.Fatalf("not Broken before a turn: %+v %v %v", d, ok, err)
	}
	if _, _, err := f.svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, 1, LiveAction{Kind: LiveTurn, Turn: 1}); err != nil {
		t.Fatal(err)
	}
	d, _, _ = f.svc.MatchDefaults(f.ctx, 5, 1)
	if !d.EnemyBroken[1] || d.EnemyBroken[2] || d.EnemyBroken[3] || len(d.Fled) != 0 {
		t.Errorf("every company of the other team is Broken: %+v", d)
	}
	if lc, _ := f.svc.Live(f.ctx, f.user(liveOwner2), 5, 2); !lc.Break.Broken || !lc.Break.Offer {
		t.Fatalf("company 2: %+v", lc.Break)
	}
	if _, _, err := f.svc.Apply(f.ctx, f.user(liveOwner2), 5, 1, 2, LiveAction{Kind: LiveAllIsLost, Flee: true}); err != nil {
		t.Fatal(err)
	}
	if d, _, _ = f.svc.MatchDefaults(f.ctx, 5, 1); !d.Fled[2] || d.Fled[1] {
		t.Errorf("company 2 fled: %+v", d)
	}
}

func TestBreakExceptionTextPrefersTheDataSentence(t *testing.T) {
	e := BreakException{Scenario: "S", Rule: "Defend the Hill", Effect: "noCourageTest", Who: "modelsOnTheHill"}
	if got := e.Text(); got != "Defend the Hill: models on the hill do not take Courage tests" {
		t.Errorf("fallback: %q", got)
	}
	e.Prose = "  Models on the hill never test.  "
	if got := e.Text(); got != "Models on the hill never test." {
		t.Errorf("data text: %q", got)
	}
}
