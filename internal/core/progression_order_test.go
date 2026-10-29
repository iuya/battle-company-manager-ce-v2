package core

import (
	"reflect"
	"testing"
)

// The casting-value choices follow the order of the powers in the data, not map order.
func TestImproveCastingChoicesFollowDataOrder(t *testing.T) {
	rules := ProgressionRules{
		CastFloor: 2, CastMax: 3,
		Powers: []PowerOffer{{Name: "Zeta"}, {Name: "Alpha"}, {Name: "Fury", Variants: []string{"Fury (Mid)", "Fury (Low)"}}, {Name: "Mu"}},
	}
	sheet := HeroSheet{
		Powers:  map[string]int{"alpha": 0, "zeta": 0, "mu": 0, "fury (low)": 0, "fury (mid)": 0, "unlisted b": 0, "unlisted a": 0},
		Casting: map[string]int{"alpha": 5, "zeta": 5, "mu": 5, "fury (low)": 5, "fury (mid)": 5, "unlisted b": 5, "unlisted a": 5},
	}
	want := []string{"cast:zeta", "cast:alpha", "cast:fury (mid)", "cast:fury (low)", "cast:mu", "cast:unlisted a", "cast:unlisted b"}
	for i := 0; i < 30; i++ {
		var got []string
		for _, c := range rules.choicesFor(PathProgression{}, sheet, LevelLists{}, ProgOption{Type: "improveCastingValue"}, false) {
			got = append(got, c.Key)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d: got %v want %v", i, got, want)
		}
	}
}
