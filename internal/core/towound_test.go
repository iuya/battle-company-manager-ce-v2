package core

import (
	"errors"
	"os"
	"testing"
)

// The shipped glossary carries the rulebook's To Wound Chart. The expected rolls below are the ones the owner knows by heart from the printed chart
// (page 44), written down here independently of the file.
func TestToWoundChartOfTheShippedGlossary(t *testing.T) {
	content, err := os.ReadFile("../../definitions/glossary/mesbg_glossary.json")
	if err != nil {
		t.Fatal(err)
	}
	g, err := BuildGlossary(RulesetInfo{}, content)
	if err != nil {
		t.Fatal(err)
	}
	if g.ToWound == nil {
		t.Fatal("the glossary has no To Wound chart")
	}
	c := *g.ToWound
	if len(c.Strengths) != 10 || len(c.Defences) != 10 || len(c.Notes) == 0 || c.Title == "" {
		t.Fatalf("chart = %d strengths, %d defences, %d notes, title %q", len(c.Strengths), len(c.Defences), len(c.Notes), c.Title)
	}
	for _, w := range []struct {
		s, d int
		want string
	}{
		{3, 3, "4+"}, {4, 6, "5+"}, {1, 1, "4+"}, {6, 6, "4+"}, {10, 10, "4+"},
		{4, 3, "4+"}, {5, 3, "3+"}, {7, 5, "3+"}, {3, 1, "3+"}, {3, 2, "4+"}, {2, 6, "6+"}, {1, 6, "6+/4+"}, {1, 8, "6+/6+"}, {1, 9, "-"}, {3, 10, "6+/6+"}, {7, 10, "6+"}, {9, 10, "5+"},
	} {
		if got := c.Roll(w.s, w.d); got != w.want {
			t.Errorf("S%d vs D%d = %q, want %q", w.s, w.d, got, w.want)
		}
	}
	if c.Roll(0, 3) != "" || c.Roll(3, 11) != "" {
		t.Error("a Strength or Defence outside the chart has a roll")
	}
}

func TestParseToWoundRejectsABrokenChart(t *testing.T) {
	if _, ok, err := ParseToWound([]byte(`{"terms":[]}`)); ok || err != nil {
		t.Errorf("a file without the chart: ok=%v err=%v", ok, err)
	}
	for name, doc := range map[string]string{
		"short grid":   `{"reference":{"toWound":{"strengths":[1,2],"defences":[1],"cells":[["4+"]]}}}`,
		"unknown cell": `{"reference":{"toWound":{"strengths":[1],"defences":[1],"cells":[["2+"]]}}}`,
	} {
		if _, ok, err := ParseToWound([]byte(doc)); ok || !errors.Is(err, ErrInvalidRuleset) {
			t.Errorf("%s: ok=%v err=%v", name, ok, err)
		}
		if g, err := BuildGlossary(RulesetInfo{}, []byte(doc)); err != nil || g.ToWound != nil {
			t.Errorf("%s: the glossary must build without the chart (err %v)", name, err)
		}
	}
}
