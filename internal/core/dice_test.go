package core

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestRandomDiceStaysOnItsFaces(t *testing.T) {
	var d Dice = RandomDice{}
	for _, spec := range []DiceSpec{NdM(1, 6), NdM(2, 6), NdM(3, 4), NdM(1, 12), NdM(2, 20), NdM(10, 100)} {
		lo, hi := 1<<30, 0
		for range 2000 {
			n := d.Roll(spec)
			lo, hi = min(lo, n), max(hi, n)
			if n < spec.Min() || n > spec.Max() {
				t.Fatalf("%s rolled %d, outside %d..%d", spec, n, spec.Min(), spec.Max())
			}
		}
		if spec.Count == 1 && (lo != 1 || hi != spec.Sides) {
			t.Errorf("%s: 2000 rolls showed %d..%d, want every face", spec, lo, hi)
		}
	}
	for range 500 {
		if n := d.Die(12); n < 1 || n > 12 {
			t.Fatalf("d12 rolled %d", n)
		}
	}
}

func TestFixedDiceFeedsEveryDieOfARoll(t *testing.T) {
	tests := []struct {
		name  string
		rolls []int
		spec  DiceSpec
		want  int
	}{
		{"2d6 adds two results", []int{3, 4}, NdM(2, 6), 7},
		{"d12 keeps a result above six", []int{11}, NdM(1, 12), 11},
		{"d3 folds a d6 face", []int{5}, NdM(1, 3), 3},
		{"d3 folds 1-2 to 1", []int{2}, NdM(1, 3), 1},
		{"d2 folds 4-6 to 2", []int{4}, NdM(1, 2), 2},
		{"d4 is not a divisor of six: kept", []int{4}, NdM(1, 4), 4},
		{"the last result repeats", []int{2}, NdM(3, 6), 6},
		{"no results: ones", nil, NdM(2, 6), 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (&FixedDice{Rolls: tt.rolls}).Roll(tt.spec); got != tt.want {
				t.Errorf("Roll(%s) = %d, want %d", tt.spec, got, tt.want)
			}
		})
	}
	if got := RollDice(&FixedDice{Rolls: []int{2, 5, 9}}, NdM(3, 12)); !slices.Equal(got, []int{2, 5, 9}) {
		t.Errorf("RollDice = %v, want the three dice one by one", got)
	}
}

func TestCheckCovers(t *testing.T) {
	d12, twoD6 := NdM(1, 12), NdM(2, 6)
	tests := []struct {
		name   string
		spec   DiceSpec
		ranges [][2]int
		want   string // "" = covered
	}{
		{"2d6 exactly", twoD6, [][2]int{{2, 3}, {4, 11}, {12, 12}}, ""},
		{"d12 exactly", d12, [][2]int{{1, 6}, {7, 12}}, ""},
		{"a gap", twoD6, [][2]int{{2, 3}, {5, 12}}, "4 is covered 0 times"},
		{"an overlap", twoD6, [][2]int{{2, 7}, {7, 12}}, "7 is covered 2 times"},
		{"above the dice", twoD6, [][2]int{{2, 13}}, "outside 2..12"},
		{"below the dice", twoD6, [][2]int{{1, 12}}, "outside 2..12"},
		{"inverted", twoD6, [][2]int{{12, 2}}, "outside 2..12"},
		{"d12 table on 2d6 dice misses the low end", twoD6, [][2]int{{1, 12}}, "outside"},
		{"nothing", twoD6, nil, "2 is covered 0 times"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckCovers("table", tt.spec, tt.ranges)
			switch {
			case tt.want == "" && err != nil:
				t.Errorf("unexpected error: %v", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Errorf("error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}

func TestTypedRollsOfDiceWithoutAPhysicalForm(t *testing.T) {
	tests := []struct {
		spec     DiceSpec
		typedMax int
		typed    int
		want     int
		ok       bool
	}{
		{NdM(1, 6), 6, 4, 4, true},
		{NdM(1, 6), 6, 7, 0, false},
		{NdM(1, 3), 6, 6, 3, true}, // a d3 is rolled on a d6
		{NdM(1, 3), 6, 3, 2, true},
		{NdM(1, 3), 6, 1, 1, true},
		{NdM(1, 2), 6, 4, 2, true},
		{NdM(1, 4), 4, 4, 4, true}, // no way to share six faces out evenly: typed as it is
		{NdM(1, 12), 12, 12, 12, true},
		{NdM(2, 6), 12, 12, 12, true}, // a sum is typed as a sum
		{NdM(2, 6), 12, 1, 0, false},  // below the lowest total
	}
	for _, tt := range tests {
		if got := TypedMax(tt.spec); got != tt.typedMax {
			t.Errorf("TypedMax(%s) = %d, want %d", tt.spec, got, tt.typedMax)
		}
		if got, ok := FromTyped(tt.spec, tt.typed); got != tt.want || ok != tt.ok {
			t.Errorf("FromTyped(%s, %d) = %d, %v; want %d, %v", tt.spec, tt.typed, got, ok, tt.want, tt.ok)
		}
	}
}

func TestCastDiceFollowsTheDiceOfTheRules(t *testing.T) {
	tests := []struct {
		name    string
		spec    DiceSpec
		entered []int
		want    []int
		err     string
	}{
		{"rolled by the app", NdM(3, 6), nil, []int{1, 1, 1}, ""},
		{"typed 2d6", NdM(2, 6), []int{3, 5}, []int{3, 5}, ""},
		{"typed d12", NdM(1, 12), []int{11}, []int{11}, ""},
		{"too few dice", NdM(3, 6), []int{3, 5}, nil, "enter the three dice, each from 1 to 6"},
		{"too many dice", NdM(1, 12), []int{3, 5}, nil, "enter one die, each from 1 to 12"},
		{"a face the die has not", NdM(2, 6), []int{3, 7}, nil, "each from 1 to 6"},
		{"zero", NdM(2, 6), []int{0, 3}, nil, "each from 1 to 6"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := castDice(&FixedDice{Rolls: []int{1}}, tt.spec, tt.entered)
			if tt.err != "" {
				if !errors.Is(err, ErrInvalidAftermath) || !strings.Contains(err.Error(), tt.err) {
					t.Errorf("error = %v, want ErrInvalidAftermath containing %q", err, tt.err)
				}
				return
			}
			if err != nil || !slices.Equal(got, tt.want) {
				t.Errorf("castDice = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestBonusRollsTheDiceOfTheRow(t *testing.T) {
	row := func(dice string, spec DiceSpec) CasualtyEffect {
		return CasualtyEffect{Row: InjuryRow{BonusDice: dice, BonusSpec: spec}}
	}
	tests := []struct {
		name string
		eff  CasualtyEffect
		fix  []int
		want int
	}{
		{"none", CasualtyEffect{}, []int{6}, 0},
		{"d3 from a six", row("d3", NdM(1, 3)), []int{6}, 3},
		{"d6", row("d6", NdM(1, 6)), []int{4}, 4},
		{"d4", row("d4", NdM(1, 4)), []int{4}, 4},
		{"2d6", row("2d6", NdM(2, 6)), []int{3, 4}, 7},
	}
	for _, tt := range tests {
		if got := tt.eff.Bonus(&FixedDice{Rolls: tt.fix}); got != tt.want {
			t.Errorf("%s: Bonus = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestParseScenarioSelection(t *testing.T) {
	sixOf := func(prefix string) string {
		return `["` + prefix + `1","` + prefix + `2","` + prefix + `3","` + prefix + `4","` + prefix + `5","` + prefix + `6"]`
	}
	eightOf := func(prefix string) string {
		return `["` + prefix + `1","` + prefix + `2","` + prefix + `3","` + prefix + `4","` + prefix + `5","` + prefix + `6","` + prefix + `7","` + prefix + `8"]`
	}
	doc := func(die, tables string) []byte {
		return []byte(`{"campaign":{"scenarioSelection":{` + die + `"tables":[` + tables + `]}}}`)
	}
	tests := []struct {
		name    string
		content []byte
		wantDie DiceSpec
		tables  int
		err     string
	}{
		{"the shipped shape, no die key yet", doc(``, `{"roll":[1,2],"scenarios":`+sixOf("a")+`},{"roll":[3,4],"scenarios":`+sixOf("b")+`},{"roll":[5,6],"scenarios":`+sixOf("c")+`}`), NdM(1, 6), 3, ""},
		{"a d8 with two tables of eight", doc(`"die":"d8",`, `{"roll":[1,4],"scenarios":`+eightOf("a")+`},{"roll":[5,8],"scenarios":`+eightOf("b")+`}`), NdM(1, 8), 2, ""},
		{"no tables", []byte(`{"campaign":{}}`), NdM(1, 6), 0, ""},
		{"ranges leave a face out", doc(`"die":"d8",`, `{"roll":[1,4],"scenarios":`+eightOf("a")+`},{"roll":[5,7],"scenarios":`+eightOf("b")+`}`), DiceSpec{}, 0, "8 is covered 0 times"},
		{"six scenarios on a d8 table", doc(`"die":"d8",`, `{"roll":[1,8],"scenarios":`+sixOf("a")+`}`), DiceSpec{}, 0, "6 scenarios, expected 8"},
		{"two dice", doc(`"die":"2d6",`, `{"roll":[2,12],"scenarios":[]}`), DiceSpec{}, 0, "must be one die"},
		{"not a spec", doc(`"die":"d6x",`, ``), DiceSpec{}, 0, "campaign.scenarioSelection.die"},
		{"a range of one number", doc(``, `{"roll":[1],"scenarios":`+sixOf("a")+`}`), DiceSpec{}, 0, "roll must be [low, high]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sel, err := ParseScenarioSelection(tt.content)
			if tt.err != "" {
				if !errors.Is(err, ErrInvalidRuleset) || !strings.Contains(err.Error(), tt.err) {
					t.Errorf("error = %v, want ErrInvalidRuleset containing %q", err, tt.err)
				}
				return
			}
			if err != nil || sel.Die != tt.wantDie || len(sel.Tables) != tt.tables {
				t.Errorf("selection = %+v, %v; want die %s and %d tables", sel, err, tt.wantDie, tt.tables)
			}
		})
	}
}

func TestScenarioSelectionPicksByTheRangesOfTheFile(t *testing.T) {
	sel := ScenarioSelection{Die: NdM(1, 8), Tables: []ScenarioTableRange{{1, 3}, {4, 8}}}
	for d, want := range map[int]int{1: 1, 3: 1, 4: 2, 8: 2, 0: 0, 9: 0} {
		if got := sel.Table(d); got != want {
			t.Errorf("Table(%d) = %d, want %d", d, got, want)
		}
	}
	list := []ScenarioInfo{{Name: "A", Table: 1, Roll: 7}, {Name: "B", Table: 2, Roll: 8}}
	if sc, ok := ScenarioByRolls(sel, list, 2, 7); !ok || sc.Name != "A" {
		t.Errorf("2,7 = %q %v, want A", sc.Name, ok)
	}
	if sc, ok := ScenarioByRolls(sel, list, 8, 8); !ok || sc.Name != "B" {
		t.Errorf("8,8 = %q %v, want B", sc.Name, ok)
	}
	for _, bad := range [][2]int{{9, 1}, {1, 9}, {0, 1}} {
		if _, ok := ScenarioByRolls(sel, list, bad[0], bad[1]); ok {
			t.Errorf("%v must not map", bad)
		}
	}
}

func TestRollHeroRollsAsManyResultsAsTheRulesAsk(t *testing.T) {
	r := ProgressionRules{HeroRolls: 3, HeroDice: NdM(2, 6), Paths: []PathProgression{{Name: "P", PathDice: NdM(1, 12),
		Rows: []ProgRow{{Roll: 5, Text: "five", Options: []ProgOption{{Type: "specialRule", Rule: "Spy"}}}}}}}
	sheet := HeroSheet{Path: "P", Stats: map[string]SheetStat{}, Rules: map[string]bool{}, Actions: map[string]bool{}}
	roll := r.RollHero(sheet, LevelLists{}, []int{5, 12, 3}, &FixedDice{Rolls: []int{1}})
	if len(roll.Pairs) != 3 || roll.Pairs[0].Sum != 5 || roll.Pairs[1].Sum != 12 || roll.Pairs[2].Sum != 3 {
		t.Fatalf("three results, typed as they are: %+v", roll.Pairs)
	}
	if len(roll.Offers) != 1 || roll.Offers[0].Roll != 5 {
		t.Errorf("only the 5 has a row: %+v", roll.Offers)
	}
	// a typed 13 is outside the d12 of the path: rolled by the app instead
	roll = r.RollHero(sheet, LevelLists{}, []int{13, 13, 13}, &FixedDice{Rolls: []int{5}})
	if len(roll.Pairs) != 3 || len(roll.Pairs[0].Dice) != 1 || roll.Pairs[0].Sum != 5 {
		t.Errorf("a result outside the dice is rolled again: %+v", roll.Pairs)
	}
}
