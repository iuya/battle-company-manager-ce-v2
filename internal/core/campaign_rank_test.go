package core

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestRank(t *testing.T) {
	rows := Rank([]CampaignCompany{
		{Name: "b", Points: 300}, {Name: "a", Points: 300}, {Name: "top", Points: 400}, {Name: "last", Points: 100},
	})
	var names []string
	var ranks []int
	for _, r := range rows {
		names, ranks = append(names, r.Name), append(ranks, r.Rank)
	}
	if want := []string{"top", "a", "b", "last"}; !slices.Equal(names, want) {
		t.Errorf("order: got %v, want %v", names, want)
	}
	if want := []int{1, 2, 2, 4}; !slices.Equal(ranks, want) {
		t.Errorf("ranks: got %v, want %v", ranks, want)
	}
}

func TestValidateSides(t *testing.T) {
	got, err := ValidateSides([]NewSide{{Name: " Gondor ", Companies: []int64{1}}, {Companies: []int64{2, 3}}, {Companies: []int64{4}}})
	if err != nil {
		t.Fatalf("three sides: %v", err)
	}
	if got[0].Name != "Gondor" || got[1].Name != "Side 2" || got[2].Name != "Side 3" {
		t.Errorf("names: %+v", got)
	}
	long := strings.Repeat("a", 41)
	for name, sides := range map[string][]NewSide{
		"one side":           {{Companies: []int64{1}}},
		"empty side":         {{Companies: []int64{1}}, {}},
		"same company twice": {{Companies: []int64{1}}, {Companies: []int64{1}}},
		"twice on one side":  {{Companies: []int64{1, 1}}, {Companies: []int64{2}}},
		"long name":          {{Name: long, Companies: []int64{1}}, {Companies: []int64{2}}},
		"too many sides":     make([]NewSide, 9),
	} {
		if _, err := ValidateSides(sides); !errors.Is(err, ErrInvalidMatch) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestMatchTone(t *testing.T) {
	m := Match{Ended: true, Winner: 2, Sides: make([]MatchSide, 3)}
	if m.Tone(0) != "loss" || m.Tone(1) != "win" || m.Tone(2) != "loss" {
		t.Error("winner and losers")
	}
	m.Winner = 0
	if m.Tone(0) != "draw" || m.Tone(1) != "draw" {
		t.Error("draw")
	}
	if (Match{Sides: make([]MatchSide, 2)}).Tone(0) != "" {
		t.Error("open match")
	}
}

func TestCompanyInputValidate(t *testing.T) {
	if in, err := (CompanyInput{Name: " Rangers ", Faction: "Gondor"}).Validate(); err != nil || in.Name != "Rangers" {
		t.Errorf("valid: %+v, %v", in, err)
	}
	for _, in := range []CompanyInput{{Faction: "Gondor"}, {Name: "x"}} {
		if _, err := in.Validate(); !errors.Is(err, ErrInvalidCompany) {
			t.Errorf("%+v: got %v", in, err)
		}
	}
}
