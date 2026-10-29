package core

import (
	"testing"
)

func TestScenarioByRollsUsesTheRealFile(t *testing.T) {
	list, sel := shippedRules(t).BC.Scenarios(), shippedRules(t).BC.Selection()
	cases := []struct {
		first, second int
		want          string
	}{
		{1, 1, "Chance Encounter"}, {2, 6, "Rescue"}, {3, 1, "Relic of Ages Past"}, {4, 4, "Plunder the Supplies"},
		{5, 1, "Secure the Area"}, {6, 6, "Baggage Train"},
	}
	for _, c := range cases {
		got, ok := ScenarioByRolls(sel, list, c.first, c.second)
		if !ok || got.Name != c.want {
			t.Errorf("%d,%d: %q %v, want %q", c.first, c.second, got.Name, ok, c.want)
		}
	}
	// every pair of dice gives one of the 18 scenarios, each exactly once per table position
	seen := map[string]int{}
	for a := 1; a <= 6; a++ {
		for b := 1; b <= 6; b++ {
			sc, ok := ScenarioByRolls(sel, list, a, b)
			if !ok {
				t.Fatalf("%d,%d gives nothing", a, b)
			}
			seen[sc.Name]++
		}
	}
	if len(seen) != 18 {
		t.Errorf("%d different scenarios, want 18", len(seen))
	}
	for _, bad := range [][2]int{{0, 1}, {7, 1}, {1, 0}, {1, 7}} {
		if _, ok := ScenarioByRolls(sel, list, bad[0], bad[1]); ok {
			t.Errorf("%v must not map", bad)
		}
	}
}

func TestTopDog(t *testing.T) {
	side := func(points ...int) MatchSide {
		var s MatchSide
		for _, p := range points {
			s.Companies = append(s.Companies, CampaignCompany{Points: p})
		}
		return s
	}
	if top, under := (Match{Sides: []MatchSide{side(100), side(120, 10)}}).TopDog(); top != 1 || under != 0 {
		t.Errorf("sums of the side: %d %d", top, under)
	}
	if top, _ := (Match{Sides: []MatchSide{side(100), side(100)}}).TopDog(); top != -1 {
		t.Error("a tie has no top dog")
	}
	if top, _ := (Match{Sides: []MatchSide{side(1), side(2), side(3)}}).TopDog(); top != -1 {
		t.Error("three sides have no top dog")
	}
	full := MatchSide{Companies: []CampaignCompany{{Points: 90, FullPoints: 110}}}
	if full.Rating() != 110 {
		t.Error("full points count")
	}
}
