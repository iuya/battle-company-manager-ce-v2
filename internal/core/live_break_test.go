package core

import (
	"errors"
	"fmt"
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

func TestBreakCountsAreWholeModels(t *testing.T) {
	tests := []struct {
		name                           string
		b                              LiveBreak
		breakAt, toBreak, endAt, toEnd int
	}{
		{"15 models: the Break Point is 7.5, the 8th breaks; the end is 11.25, the 12th reaches it", LiveBreak{StartingModels: 15, Threshold100: 750, Casualties: 3, EndLossPercent: 75}, 8, 5, 12, 9},
		{"14 models: exactly the Break Point is not Broken", LiveBreak{StartingModels: 14, Threshold100: 700, Casualties: 7, EndLossPercent: 75}, 8, 1, 11, 4},
		{"16 models: strictly over the Break Point at the 9th, exactly 3/4 is the end at the 12th", LiveBreak{StartingModels: 16, Threshold100: 800, Casualties: 12, EndLossPercent: 75}, 9, 0, 12, 0},
		{"16 models: one short of the end", LiveBreak{StartingModels: 16, Threshold100: 800, Casualties: 11, EndLossPercent: 75}, 9, 0, 12, 1},
		{"16 models: exactly the Break Point is not Broken yet", LiveBreak{StartingModels: 16, Threshold100: 800, Casualties: 8, EndLossPercent: 75}, 9, 1, 12, 4},
		{"models that left bring neither closer", LiveBreak{StartingModels: 15, Threshold100: 750, Casualties: 3, LeftTable: 4, EndLossPercent: 75}, 8, 5, 12, 9},
		{"at least the Break Point (gte), rounded up to a whole model", LiveBreak{StartingModels: 15, Threshold100: 750, Casualties: 3, AtLeast: true}, 8, 5, 0, 0},
		{"gte on a whole Break Point", LiveBreak{StartingModels: 14, Threshold100: 700, Casualties: 7, AtLeast: true}, 7, 0, 0, 0},
		{"three models: every one must be lost", LiveBreak{StartingModels: 3, Threshold100: 150, EndLossPercent: 75}, 2, 2, 3, 3},
		{"never below zero", LiveBreak{StartingModels: 4, Threshold100: 200, Casualties: 9, EndLossPercent: 75}, 3, 0, 3, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := tt.b
			if b.BreakAt() != tt.breakAt || b.ToBreak() != tt.toBreak || b.EndAt() != tt.endAt || b.ToEnd() != tt.toEnd {
				t.Errorf("BreakAt %d, ToBreak %d, EndAt %d, ToEnd %d; want %d %d %d %d", b.BreakAt(), b.ToBreak(), b.EndAt(), b.ToEnd(), tt.breakAt, tt.toBreak, tt.endAt, tt.toEnd)
			}
		})
	}
}

// The players read the end as the share that remains, derived from the share lost.
func TestEndLeftPercentIsTheShareThatRemains(t *testing.T) {
	if got := (LiveBreak{EndLossPercent: 75}).EndLeftPercent(); got != 25 {
		t.Errorf("3/4 lost leaves %d%%, want 25", got)
	}
}

// A failed courage test is a casualty, for the Break Point and for the scenario's end; a model that left the board counts for neither.
func TestBreakCountsWhoLeftTheTable(t *testing.T) {
	b := &bp{t: t, inits: breakInits(12), cfg: LiveFoldConfig{Break: map[int64]BreakConfig{1: {Percent: 50, EndLossPercent: 75}}}}
	b.do(1, LiveAction{Kind: LiveRetreat, UnitID: 1, Why: "left"})
	b.do(1, LiveAction{Kind: LiveRetreat, UnitID: 2, Why: "courage"})
	b.do(1, LiveAction{Kind: LiveDied, UnitID: 3})
	got := b.state().Break(1)
	if got.LeftTable != 1 || got.Casualties != 2 || got.EndLossPercent != 75 {
		t.Errorf("left %d casualties %d end %d%%, want 1, 2 and 75", got.LeftTable, got.Casualties, got.EndLossPercent)
	}
	if got.ToEnd() != 7 { // the 9th of 12 is the end, with the two casualties and not the one who left
		t.Errorf("%d more to the end, want 7", got.ToEnd())
	}
	b.do(1, LiveAction{Kind: LiveReturn, UnitID: 1})
	if got := b.state().Break(1); got.LeftTable != 0 || got.ToEnd() != 7 {
		t.Errorf("returned: left %d, %d more to the end", got.LeftTable, got.ToEnd())
	}
}

// 16 models, played through the log: the 9th casualty breaks the company and the 12th ends the game, one of them a failed Courage test,
// with models that left the board on the side.
func TestScenarioEndThroughTheLog(t *testing.T) {
	b := &bp{t: t, inits: breakInits(16), cfg: LiveFoldConfig{Break: map[int64]BreakConfig{1: {Percent: 50, EndLossPercent: 75}}}}
	b.do(1, LiveAction{Kind: LiveRetreat, UnitID: 16, Why: "left"})
	b.do(1, LiveAction{Kind: LiveRetreat, UnitID: 15, Why: "left"})
	for id := int64(1); id <= 8; id++ {
		b.do(1, LiveAction{Kind: LiveDied, UnitID: id})
	}
	if got := b.state().Break(1); got.Broken || got.ToEnd() != 4 {
		t.Fatalf("8 casualties: Broken %v, %d more to the end; want whole and 4", got.Broken, got.ToEnd())
	}
	b.do(1, LiveAction{Kind: LiveRetreat, UnitID: 9, Why: "courage"})
	if got := b.state().Break(1); !got.Broken || got.ToEnd() != 3 {
		t.Fatalf("the 9th, a failed Courage test: Broken %v, %d more to the end; want Broken and 3", got.Broken, got.ToEnd())
	}
	b.do(1, LiveAction{Kind: LiveDied, UnitID: 10})
	b.do(1, LiveAction{Kind: LiveDied, UnitID: 11})
	if got := b.state().Break(1); got.ToEnd() != 1 {
		t.Fatalf("11 casualties: %d more to the end, want 1", got.ToEnd())
	}
	b.do(1, LiveAction{Kind: LiveDied, UnitID: 12})
	if got := b.state().Break(1); got.ToEnd() != 0 || got.LeftTable != 2 {
		t.Errorf("the 12th: %d more to the end, %d left; want 0 and 2", got.ToEnd(), got.LeftTable)
	}
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
			got := b.state().Break(1)
			if got.Broken != tt.broken || got.Casualties != tt.casualties || got.StartingModels != tt.models || fmt.Sprintf("%d.%d", got.Threshold100/100, (got.Threshold100%100)/10) != tt.breakPoint {
				t.Errorf("%+v, want broken=%v %s", got, tt.broken, tt.breakPoint)
			}
		})
	}
}

func TestBrokenAsSoonAsTheCasualtiesExceedTheBreakPoint(t *testing.T) {
	b := newBP(t, 13, 50)
	b.fall(6)
	if b.state().Break(1).Broken {
		t.Fatal("6 of 13 is not above the Break Point")
	}
	b.fall(1)
	if !b.state().Break(1).Broken {
		t.Fatal("the seventh casualty breaks the company at once: there are no turns")
	}
	// a unit returns to the battle: the casualties drop under the Break Point and the company is whole again
	b.do(1, LiveAction{Kind: LiveReturn, UnitID: 1})
	if got := b.state().Break(1); got.Broken || got.Casualties != 6 {
		t.Errorf("back under the Break Point: %+v", got)
	}
	// undoing the casualty that broke it is the fold without it
	b.do(1, LiveAction{Kind: LiveDied, UnitID: 1})
	rest := b.entries[:len(b.entries)-1]
	if err := ValidateLogWith(b.inits, rest, b.cfg); err != nil {
		t.Fatalf("the log without the last casualty is valid: %v", err)
	}
	if got := FoldLiveWith(b.inits, rest, b.cfg).Break(1); got.Broken {
		t.Errorf("after the undo: %+v", got)
	}
	if b.state().Break(2).Broken {
		t.Error("the other company has no casualties")
	}
}

func TestOlderTurnAndAllIsLostEntries(t *testing.T) {
	b := newBP(t, 13, 50)
	b.fall(7)
	if _, err := b.try(0, LiveAction{Kind: liveTurnLegacy, Turn: 1}); !errors.Is(err, ErrInvalidLive) {
		t.Errorf("a turn entry can no longer be written: %v", err)
	}
	if _, err := b.try(1, LiveAction{Kind: liveAllIsLostLegacy, Flee: true}); !errors.Is(err, ErrInvalidLive) {
		t.Errorf("All is Lost can no longer be written: %v", err)
	}
	// an older log still folds: the turn is ignored, the company that fled left the board
	old := append(append([]LiveEntry{}, b.entries...),
		LiveEntry{ID: 90, Seq: 90, CompanyID: 0, Action: LiveAction{Kind: liveTurnLegacy, Turn: 1}},
		LiveEntry{ID: 91, Seq: 91, CompanyID: 1, Action: LiveAction{Kind: liveAllIsLostLegacy, Flee: true}})
	s := FoldLiveWith(b.inits, old, b.cfg)
	if got := s.Break(1); !got.Broken || got.Casualties != 7 {
		t.Errorf("the turn changes nothing and leaving the board adds no casualties: %+v", got)
	}
	for i := int64(8); i <= 13; i++ {
		if x, _ := s.Unit(i); x.Zone != ZoneRetreated || x.Reason != "Left the board" {
			t.Errorf("unit %d should have left the board: %+v", i, x)
		}
	}
	if err := ValidateLogWith(b.inits, old, b.cfg); err != nil {
		t.Errorf("an older entry must not stop an undo elsewhere in the log: %v", err)
	}
}

func TestOlderSecretNotesFoldToNothing(t *testing.T) {
	b := newBP(t, 13, 50)
	b.fall(3)
	want := b.state().Break(1)
	old := append(append([]LiveEntry{}, b.entries...),
		LiveEntry{ID: 90, Seq: 90, CompanyID: 1, UnitID: 8, Action: LiveAction{Kind: liveSecretNominateLegacy, UnitID: 8}},
		LiveEntry{ID: 91, Seq: 91, CompanyID: 1, Action: LiveAction{Kind: liveSecretDoneLegacy}})
	s := FoldLiveWith(b.inits, old, b.cfg)
	if got := s.Break(1); got.Broken != want.Broken || got.Casualties != want.Casualties {
		t.Errorf("the notes change nothing: %+v, want %+v", got, want)
	}
	if x, _ := s.Unit(8); x.Zone != ZoneField {
		t.Errorf("the noted unit stays on the field: %+v", x)
	}
	if err := ValidateLogWith(b.inits, old, b.cfg); err != nil {
		t.Errorf("an older note must not stop an undo elsewhere in the log: %v", err)
	}
	for _, kind := range []LiveKind{liveSecretNominateLegacy, liveSecretDoneLegacy} {
		if _, err := b.try(1, LiveAction{Kind: kind, UnitID: 8}); !errors.Is(err, ErrInvalidLive) {
			t.Errorf("%s can no longer be written: %v", kind, err)
		}
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
	if got.StartingModels != 6 || got.Casualties != 4 || !got.Broken {
		t.Errorf("adjusted (4 of 6 is above the Break Point): %+v", got)
	}
	// undo of the adjustment is just the fold without it
	if got := FoldLiveWith(b.inits, b.entries[:len(b.entries)-2], b.cfg).Break(1); got.StartingModels != 7 || got.Casualties != 2 || got.Broken {
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
	if got := FoldLive(b.inits, nil).Break(1); got.Percent != 0 || got.Broken {
		t.Errorf("without a config nothing ever breaks: %+v", got)
	}
}

func TestCourageTests(t *testing.T) {
	b := newBP(t, 4, 50)
	b.cfg.NoStandFast = map[int64]bool{2: true}
	b.do(1, LiveAction{Kind: LiveDied, UnitID: 3})
	b.do(1, LiveAction{Kind: LiveDied, UnitID: 4})
	if b.state().Break(1).Broken {
		t.Fatal("setup: 2 of 4 is not above half")
	}
	b.do(1, LiveAction{Kind: LiveDied, UnitID: 2})
	if !b.state().Break(1).Broken {
		t.Fatal("setup: 3 of 4 breaks it")
	}
	u := func(id int64) LiveUnit { x, _ := b.state().Unit(id); return x }
	if u(1).CourageHow != "" {
		t.Fatalf("not tested yet: %+v", u(1))
	}
	b.do(1, LiveAction{Kind: LiveBroken, UnitID: 1, StandFast: true})
	if x := u(1); x.CourageHow != "passed" || x.Gains[GainBrokenPassed] != 1 {
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
	if x := u(1); x.CourageHow != "heroicResolve" {
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
	// a failed test: the unit flees as a casualty
	b2 := newBP(t, 4, 50)
	b2.do(1, LiveAction{Kind: LiveRetreat, UnitID: 1, Why: "courage"})
	if x, _ := b2.state().Unit(1); x.CourageHow != "failed" || x.Zone != ZoneRetreated {
		t.Errorf("failed courage: %+v", x)
	}
	// a skip never earns the experience flag
	b2.do(1, LiveAction{Kind: LiveCourageSkip, UnitID: 2, Reason: "standFast"})
	if x, _ := b2.state().Unit(2); x.Gains[GainBrokenPassed] != 0 {
		t.Errorf("an automatic pass earns nothing: %+v", x)
	}
}

func TestCanUndoAnOlderTurnEntry(t *testing.T) {
	owners := map[int64]int64{1: 10, 2: 20}
	turn := LiveEntry{CompanyID: 0, Action: LiveAction{Kind: liveTurnLegacy, Turn: 1}}
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

func TestLiveServiceBreakPoint(t *testing.T) {
	f := newLiveFixture(t)
	lc, err := f.svc.Live(f.ctx, f.user(liveOwner1), 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	present := 0
	for _, u := range lc.Units {
		present += u.BreakModels
	}
	if lc.Break.Percent != 50 || lc.Break.StartingModels != present || present < 5 || lc.Break.Broken {
		t.Fatalf("the starting count comes from the roster: %+v (%d present)", lc.Break, present)
	}
	if _, _, err := f.svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, 1, LiveAction{Kind: liveTurnLegacy, Turn: 1}); !errors.Is(err, ErrInvalidLive) {
		t.Errorf("there are no turns: %v", err)
	}
	// a Warrior cannot call Stand Fast
	var warrior int64
	for _, u := range lc.Units {
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
	for _, side := range []struct{ company, owner int64 }{{2, liveOwner2}, {3, liveOwner3}} { // the other team is two companies, in this order
		theirs, _ := f.svc.Live(f.ctx, f.user(side.owner), 5, side.company)
		for _, u := range theirs.Units {
			if _, _, err := f.svc.Apply(f.ctx, f.user(side.owner), 5, 1, side.company, LiveAction{Kind: LiveDied, UnitID: u.UnitID}); err != nil {
				t.Fatal(err)
			}
		}
		if side.company == 2 {
			d, _, _ := f.svc.MatchDefaults(f.ctx, 5, 1)
			if len(d.EnemyBroken) != 0 {
				t.Fatal("setup")
			}
		}
	}
	d, ok, err := f.svc.MatchDefaults(f.ctx, 5, 1)
	if err != nil || !ok || !d.EnemyBroken[1] || d.EnemyBroken[2] || d.EnemyBroken[3] {
		t.Errorf("every company of the other team is Broken as soon as it falls: %+v %v %v", d, ok, err)
	}
	if lc, _ := f.svc.Live(f.ctx, f.user(liveOwner2), 5, 2); !lc.Break.Broken {
		t.Fatalf("company 2: %+v", lc.Break)
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
