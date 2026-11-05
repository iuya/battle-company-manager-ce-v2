package core

import (
	"bytes"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
)

const courageKey, intelligenceKey = "stat:courage:1", "stat:intelligence:1"

// aHeroOn is a Galadhrim hero (Courage 5, room to improve) on the path, with the roll typed.
func aHeroOn(t *testing.T, f *companyFixture, s *LevelService, path string, bonus map[string]int, rolls ...int) LevelRoll {
	t.Helper()
	f.companyOf("Lothlorien", 5, []UnitState{{ID: 90, Base: "Galadhrim Warrior", Role: RoleHero, Path: path, XP: 5, Progress: &Progress{Bonus: bonus}}})
	r, err := s.RollLevelUp(f.ctx, f.user, 5, 1, 90, rolls, false)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func choiceKeys(o LevelOffer) []string {
	var out []string
	for _, c := range o.Choices {
		out = append(out, c.Key)
	}
	return out
}

// planLog collects what the default logger writes during the test.
func planLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	logger, buf := capture()
	old := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(old) })
	return buf
}

func TestAnOptionThatCannotBeWorkedOutIsLoggedAndTheRestOfThePlanStands(t *testing.T) {
	const broken = "stat:courage:oops"
	f := newCompanyFixture(t)
	s := f.leveller(t)
	r := aHeroOn(t, f, s, "Path of the Tactician", nil, 5, 8)
	stored := f.state.Units[len(f.state.Units)-1].Pending.Level
	stored.Offers[0].Choices = append([]LevelChoice{{Key: broken, Label: "Broken", RollAgain: true}}, stored.Offers[0].Choices...)
	logged := planLog(t)

	got, err := s.LevelUpState(f.ctx, f.user, 5, 1, 90)
	if err != nil || got == nil || got.Plan == nil {
		t.Fatalf("the window keeps its plan: %+v %v", got, err)
	}
	if got.Plan.Marks[0] != courageKey || !slices.Contains(got.Plan.Alone, broken) {
		t.Errorf("Courage is still marked, the broken option goes alone: %+v", got.Plan)
	}
	for _, y := range r.Offers[1].Choices {
		if got.Plan.Conflicts[broken+"|"+y.Key] == "" {
			t.Errorf("the broken option does not fit %s: %+v", y.Key, got.Plan.Conflicts)
		}
	}
	if n := strings.Count(logged.String(), "level-up plan"); n != 1 || !strings.Contains(logged.String(), broken) {
		t.Errorf("logged once, naming the option: %q", logged.String())
	}
	if _, err := s.ChooseLevelUp(f.ctx, f.user, 5, 1, 90, []string{broken}); err == nil {
		t.Error("pressing it fails: choosing applies the same key")
	}
}

func TestAPlanWithNothingToWorkOutLeavesTheWindowWithoutMarks(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	aHeroOn(t, f, s, "Path of the Tactician", nil, 5, 8)
	stored := f.state.Units[len(f.state.Units)-1].Pending.Level
	stored.Offers[0].Choices = []LevelChoice{{Key: "stat:courage:oops", Label: "Broken", RollAgain: true}}
	logged := planLog(t)
	if got, err := s.LevelUpState(f.ctx, f.user, 5, 1, 90); err != nil || got == nil || got.Plan != nil {
		t.Fatalf("the roll still shows, with no plan: %+v %v", got, err)
	}
	if !strings.Contains(logged.String(), "stat:courage:oops") {
		t.Errorf("the loss of the plan is logged: %q", logged.String())
	}
}

func TestARollAgainThatLeavesNothingToTakeIsNotLogged(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	logged := planLog(t)
	aHeroOn(t, f, s, "Path of the Warlord", map[string]int{"courage": -1, "intelligence": -3}, 5, 5)
	if logged.Len() != 0 {
		t.Errorf("nothing went wrong: %q", logged.String())
	}
}

func TestRollAgainBesideAnotherResultMarksCourageAndAppliesBothInOneRequest(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	r := aHeroOn(t, f, s, "Path of the Tactician", nil, 5, 8)
	if r.Plan == nil || r.Plan.Both || r.Plan.Marks != [2]string{courageKey, ""} {
		t.Fatalf("Courage is marked: %+v", r.Plan)
	}
	if len(r.Plan.Conflicts) != 0 || len(r.Plan.Alone) != 0 {
		t.Errorf("everything fits: %+v", r.Plan)
	}
	if got, _ := s.LevelUpState(f.ctx, f.user, 5, 1, 90); got == nil || got.Plan == nil || got.Plan.Marks != r.Plan.Marks {
		t.Errorf("reopening the window shows the same: %+v", got)
	}
	done, err := s.ChooseLevelUp(f.ctx, f.user, 5, 1, 90, []string{r.Plan.Marks[0], r.Offers[1].Choices[0].Key})
	if err != nil || !done.Done {
		t.Fatalf("both in one request: %+v %v", done, err)
	}
	if p := f.state.Units[len(f.state.Units)-1].Progress; p.Bonus["courage"] != -1 || p.Taken != 1 {
		t.Errorf("both results, one roll used: %+v", p)
	}
}

func TestTheMarkFallsToIntelligenceWhenCourageIsAtItsLimit(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	r := aHeroOn(t, f, s, "Path of the Warlord", map[string]int{"courage": -2}, 5, 8) // Courage 5+ to its best, 3+
	if slices.Contains(choiceKeys(r.Offers[0]), courageKey) {
		t.Fatalf("Courage is at its limit: %v", choiceKeys(r.Offers[0]))
	}
	if r.Plan == nil || r.Plan.Marks[0] != intelligenceKey {
		t.Errorf("Intelligence is marked: %+v", r.Plan)
	}
}

func TestRollAgainWithNothingInTheOtherResultAppliesAtOnce(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	f.companyOf("Lothlorien", 5, []UnitState{{ID: 90, Base: "Galadhrim Warrior", Role: RoleHero, Path: "Path of the Tactician", XP: 5, Progress: &Progress{Rules: []string{"Spy"}}}})
	r, err := s.RollLevelUp(f.ctx, f.user, 5, 1, 90, []int{5, 2}, false) // the 2 is Spy, which it has
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Offers) != 1 || r.Plan != nil {
		t.Fatalf("one result and no marks: %+v", r)
	}
	if done, err := s.ChooseLevelUp(f.ctx, f.user, 5, 1, 90, []string{courageKey}); err != nil || !done.Done {
		t.Errorf("it applies at once: %+v %v", done, err)
	}
}

func TestRollAgainWithNothingLeftAfterItIsAPlainOption(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	// Courage one step from its peak and Intelligence at its own: the only thing a second 5 could give is the same Courage, which is gone
	r := aHeroOn(t, f, s, "Path of the Warlord", map[string]int{"courage": -1, "intelligence": -3}, 5, 5)
	if r.Plan != nil {
		t.Errorf("nothing fits with anything, nothing is marked: %+v", r.Plan)
	}
	if done, err := s.ChooseLevelUp(f.ctx, f.user, 5, 1, 90, []string{courageKey}); err != nil || !done.Done {
		t.Errorf("it applies at once: %+v %v", done, err)
	}
}

func TestANormalOptionBesideRollAgainAppliesAlone(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	r := aHeroOn(t, f, s, "Path of the Warlord", nil, 7, 8)
	if r.Plan == nil || r.Plan.Both || r.Plan.Marks[0] != courageKey || r.Plan.Marks[1] != "" {
		t.Fatalf("the RollAgain option is still marked: %+v", r.Plan)
	}
	normal := ""
	for _, c := range r.Offers[0].Choices {
		if !c.RollAgain {
			normal = c.Key
		}
	}
	if done, err := s.ChooseLevelUp(f.ctx, f.user, 5, 1, 90, []string{normal}); err != nil || !done.Done {
		t.Fatalf("only that option: %+v %v", done, err)
	}
	if p := f.state.Units[len(f.state.Units)-1].Progress; len(p.Rules) != 1 || p.Bonus["fight"] != 0 {
		t.Errorf("no extra, the other result is lost: %+v", p)
	}
}

func TestBothResultsTakingTheOtherMarkOneEach(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	r := aHeroOn(t, f, s, "Path of the Tactician", nil, 5, 5)
	if r.Plan == nil || !r.Plan.Both || r.Plan.Marks != [2]string{courageKey, intelligenceKey} {
		t.Fatalf("Courage in the first, Intelligence in the second: %+v", r.Plan)
	}
	if done, err := s.ChooseLevelUp(f.ctx, f.user, 5, 1, 90, []string{courageKey, intelligenceKey}); err != nil || !done.Done {
		t.Fatalf("Apply both: %+v %v", done, err)
	}
	if p := f.state.Units[len(f.state.Units)-1].Progress; p.Bonus["courage"] != -1 || p.Bonus["intelligence"] != -1 {
		t.Errorf("both stats: %+v", p.Bonus)
	}
}

func TestTheWarlordsFiveAndSevenTakeTheRuleAndTheStat(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	r := aHeroOn(t, f, s, "Path of the Warlord", nil, 5, 7)
	if r.Plan == nil || !r.Plan.Both {
		t.Fatalf("a mark in each: %+v", r.Plan)
	}
	rule := ""
	for _, c := range r.Offers[1].Choices {
		if !c.RollAgain {
			rule = c.Key
		}
	}
	if _, no := r.Plan.Conflicts[courageKey+"|"+rule]; no {
		t.Errorf("the normal option of the 7 can take the mark: %+v", r.Plan.Conflicts)
	}
	if done, err := s.ChooseLevelUp(f.ctx, f.user, 5, 1, 90, []string{courageKey, rule}); err != nil || !done.Done {
		t.Fatalf("the 5's stat and the 7's rule: %+v %v", done, err)
	}
	if p := f.state.Units[len(f.state.Units)-1].Progress; p.Bonus["courage"] != -1 || len(p.Rules) != 1 {
		t.Errorf("it gets both: %+v", p)
	}
}

func TestTwoNormalOptionsNeverGoTogether(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	r := aHeroOn(t, f, s, "Path of the Warlord", nil, 7, 7)
	if r.Plan == nil || !r.Plan.Both {
		t.Fatalf("a mark in each: %+v", r.Plan)
	}
	rule := "rule:Heroic Resolve Mastery"
	if why := r.Plan.Conflicts[rule+"|"+rule]; why == "" {
		t.Errorf("the mastery twice does not fit: %+v", r.Plan.Conflicts)
	}
}

func TestOptionsThatDoNotFitTheMarkSayWhy(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	// Courage 4+ has one step left to its peak of 3+
	r := aHeroOn(t, f, s, "Path of the Tactician", map[string]int{"courage": -1}, 5, 5)
	if r.Plan == nil || !r.Plan.Both {
		t.Fatalf("a mark in each: %+v", r)
	}
	if why := r.Plan.Conflicts[courageKey+"|"+courageKey]; !strings.Contains(why, "Courage cannot improve twice") {
		t.Errorf("the same stat: %q", why)
	}
	if why := r.Plan.Conflicts[courageKey+"|"+intelligenceKey]; why != "" {
		t.Errorf("the other stat fits: %q", why)
	}
	if r.Plan.Conflicts[r.Plan.Marks[0]+"|"+r.Plan.Marks[1]] != "" {
		t.Errorf("the default marks fit: %+v", r.Plan)
	}
}

func TestARerollRecalculatesTheMarks(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t, 4, 4) // the re-rolled pair: an 8
	r := aHeroOn(t, f, s, "Path of the Tactician", nil, 5, 5)
	f.state.Rerolls = 1
	if r.Plan == nil || !r.Plan.Both {
		t.Fatalf("a mark in each: %+v", r.Plan)
	}
	again, err := s.RerollLevelUp(f.ctx, f.user, 5, 1, 90, 1)
	if err != nil {
		t.Fatal(err)
	}
	if again.Plan == nil || again.Plan.Both || again.Plan.Marks != [2]string{courageKey, ""} {
		t.Errorf("one mark now: %+v", again.Plan)
	}
}

func TestStatOptionsCarryTheirProgress(t *testing.T) {
	stat := func(r LevelRoll, key string) StatGain {
		t.Helper()
		for _, o := range r.Offers {
			for _, c := range o.Choices {
				if c.Key == key {
					if c.Stat == nil {
						t.Fatalf("%s has no progress", key)
					}
					return *c.Stat
				}
			}
		}
		t.Fatalf("no %s in %+v", key, r.Offers)
		return StatGain{}
	}
	t.Run("one taken of three", func(t *testing.T) {
		f := newCompanyFixture(t)
		s := f.leveller(t)
		g := stat(aHeroOn(t, f, s, "Path of the Tactician", map[string]int{"fight": 1}, 8, 8), "stat:fight:-1")
		if g.Stat != "Fight" || g.Taken != 1 || g.Gain != 1 || g.Limit != 3 {
			t.Errorf("%+v", g)
		}
		if !strings.Contains(g.Detail, "→") || !strings.Contains(g.Detail, "path limit +3") || !strings.Contains(g.Detail, "peak 9") {
			t.Errorf("the exact values: %q", g.Detail)
		}
	})
	t.Run("lower is better counts steps of improvement", func(t *testing.T) {
		f := newCompanyFixture(t)
		s := f.leveller(t)
		g := stat(aHeroOn(t, f, s, "Path of the Tactician", map[string]int{"courage": -1}, 5, 8), courageKey)
		if g.Taken != 1 || g.Gain != 1 || g.Limit != 2 { // 5+ to the peak of 3+, below the path's limit of 3 steps
			t.Errorf("%+v", g)
		}
		if !strings.Contains(g.Detail, "+ →") {
			t.Errorf("Courage reads as 4+ → 3+: %q", g.Detail)
		}
	})
	t.Run("a peak below the path limit", func(t *testing.T) {
		f := newCompanyFixture(t)
		s := f.leveller(t)
		// Fight 5 on a path that allows +4: the peak of 9 is exactly at the limit; with +1 more bonus the peak decides
		g := stat(aHeroOn(t, f, s, "Path of the Duelist", map[string]int{"fight": 3}, 8, 8), "stat:fight:-1")
		if g.Limit != g.Taken+(9-(5+3)) {
			t.Errorf("the peak of 9 limits it: %+v", g)
		}
	})
	t.Run("an option of +2 with room for one counts one", func(t *testing.T) {
		rules := shippedRules(t).BC.Progression()
		path, _ := rules.Path("Path of the Tactician")
		sheet := HeroSheet{Stats: map[string]SheetStat{"fight": {Value: 6, Boost: 2}}}
		to, ok := rules.improvedStat(path, sheet, "fight", 2, 0)
		if !ok || to != 7 {
			t.Fatalf("cut to one: %d %v", to, ok)
		}
		if g := rules.statGain(path, sheet, "fight", 0, to); g.Gain != 1 || g.Taken != 2 || g.Limit != 3 {
			t.Errorf("%+v", g)
		}
	})
}

// --- becoming a hero and choosing a path are two steps

func TestAHeroInTheMakingBecomesAHeroWithNoPathAndChoosesLater(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	r, err := s.RollLevelUp(f.ctx, f.user, 5, 1, 8, []int{6}, false)
	if err != nil || r.Done || r.Result != "heroInTheMaking" {
		t.Fatalf("%+v %v", r, err)
	}
	if slices.ContainsFunc(r.Offers[0].Choices, func(c LevelChoice) bool { return strings.HasPrefix(c.Key, "path:") }) {
		t.Errorf("no paths on this screen: %+v", r.Offers[0].Choices)
	}
	done, err := s.ChooseLevelUp(f.ctx, f.user, 5, 1, 8, []string{"hero"})
	if err != nil || !done.Done {
		t.Fatalf("finishing needs no path: %+v %v", done, err)
	}
	u := f.unit(t, 8)
	if !u.IsHero() || !u.PathPending || u.Level != 1 {
		t.Fatalf("a hero with no path: %+v", u)
	}
	boost := func(u CompanyUnit) map[string]int {
		out := map[string]int{}
		for _, a := range u.Stats {
			if a.HeroOnly {
				out[a.Name] = a.Boost
			}
		}
		return out
	}
	before := boost(u)
	if err := s.ChoosePath(f.ctx, f.user, 5, 1, 8, "Path of the Sorcerer", "Black Dart"); err != nil {
		t.Fatal(err)
	}
	u = f.unit(t, 8)
	if u.PathPending || u.Path == nil || u.Path.Name != "Path of the Sorcerer" {
		t.Fatalf("on its path: %+v", u.Path)
	}
	hasPower, hasAction := false, false
	for _, p := range u.Path.Powers {
		hasPower = hasPower || (p.Name == "Black Dart" && p.Casting == 6 && p.Range == 6)
	}
	for _, a := range u.Path.HeroicActions {
		hasAction = hasAction || a.Name == "Heroic Channeling"
	}
	if !hasPower || !hasAction {
		t.Errorf("Black Dart and Heroic Channeling: %+v %+v", u.Path.Powers, u.Path.HeroicActions)
	}
	if after := boost(u); after["Might"] != before["Might"] || before["Might"] != 1 {
		t.Errorf("one point of Might, not two: %v then %v", before, after)
	}
	if err := s.ChoosePath(f.ctx, f.user, 5, 1, 8, "Path of the Duelist", ""); !errors.Is(err, ErrRefused) {
		t.Errorf("a path is chosen once: %v", err)
	}
}

func TestPromotionIsPossibleOnlyUntilTheHeroIsAccepted(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	f.companyOf("Minas Tirith", 5, []UnitState{{ID: 8, Base: "Warrior of Minas Tirith", Role: RoleWarrior, Gear: []string{"Bow"}, XP: 5}})
	r, err := s.RollLevelUp(f.ctx, f.user, 5, 1, 8, []int{6}, false)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(r.Offers[0].Choices, func(c LevelChoice) bool { return strings.HasPrefix(c.Key, "promo:") })
	if i < 0 {
		t.Fatalf("the promotion can be taken instead: %+v", r.Offers[0].Choices)
	}
	promo := r.Offers[0].Choices[i].Key
	if _, err := s.ChooseLevelUp(f.ctx, f.user, 5, 1, 8, []string{"hero"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChooseLevelUp(f.ctx, f.user, 5, 1, 8, []string{promo}); !errors.Is(err, ErrRefused) {
		t.Errorf("too late: %v", err)
	}
}

func TestALimitedPathIsCheckedWhenThePathIsChosen(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	f.state.Units = append(f.state.Units, UnitState{ID: 90, Base: "Warrior of Minas Tirith", Role: RoleWarrior, XP: 5})
	for _, id := range []int64{8, 90} {
		if _, err := s.RollLevelUp(f.ctx, f.user, 5, 1, id, []int{6}, false); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ChooseLevelUp(f.ctx, f.user, 5, 1, id, []string{"hero"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.ChoosePath(f.ctx, f.user, 5, 1, 8, "Path of the Sorcerer", "Black Dart"); err != nil {
		t.Fatal(err)
	}
	if err := s.ChoosePath(f.ctx, f.user, 5, 1, 90, "Path of the Sorcerer", "Black Dart"); !errors.Is(err, ErrRefused) {
		t.Errorf("the second Sorcerer is refused: %v", err)
	}
	offer, err := s.PathOptions(f.ctx, f.user, 5, 1, 90)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(offer.Paths, func(p PathOption) bool { return p.Name == "Path of the Sorcerer" })
	if i < 0 || offer.Paths[i].Blocked == "" {
		t.Errorf("the path window says why: %+v", offer.Paths)
	}
}

func TestAPathWithItsOwnStartReplacesTheBonusOfBecomingAHero(t *testing.T) {
	rs := shippedRules(t)
	env := levelEnv{rs: rs}
	p := &Progress{Hero: true, Bonus: map[string]int{"might": 1, "will": 1, "fate": 1}, StartBonus: map[string]int{"might": 1, "will": 1, "fate": 1}}
	env.takePath(p, "Path of Their Own Making") // 0 Might, 0 Will, 1 Fate
	if p.Bonus["might"] != 0 || p.Bonus["will"] != 0 || p.Bonus["fate"] != 1 || p.StartBonus != nil {
		t.Errorf("never both: %+v", p.Bonus)
	}
}

func TestAHeroInTheMakingStoredWithItsPathsIsBroughtUpToDate(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	stored := LevelRoll{Kind: "warrior", D6: 6, Result: "heroInTheMaking", Offers: []LevelOffer{{Roll: 6, Choices: []LevelChoice{
		{Key: "path:Path of the Duelist", Label: "Path of the Duelist"}, {Key: "promo:0:0", Label: "Promotion"}}}}}
	for i := range f.state.Units {
		if f.state.Units[i].ID == 8 {
			f.state.Units[i].Pending.Level = &stored
		}
	}
	got, err := s.LevelUpState(f.ctx, f.user, 5, 1, 8)
	if err != nil || got == nil {
		t.Fatalf("%+v %v", got, err)
	}
	if keys := choiceKeys(got.Offers[0]); !slices.Equal(keys, []string{"hero", "promo:0:0"}) {
		t.Errorf("the screen as it is now: %v", keys)
	}
	if _, err := s.ChooseLevelUp(f.ctx, f.user, 5, 1, 8, []string{"hero"}); err != nil {
		t.Errorf("it finishes as a new one does: %v", err)
	}
}
