package core

import (
	"reflect"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

func TestRuleRowText(t *testing.T) {
	cases := []struct{ name, tag, def, want string }{
		{"Spy", "Active", "At the beginning.", "Spy (Active) - At the beginning."},
		{"Lead by Example", "", "If this Hero kills.", "Lead by Example - If this Hero kills."},
		{"Hatred", "Passive", "", "Hatred (Passive) - "},
	}
	for _, c := range cases {
		if got := RuleRowText(c.name, c.tag, c.def); got != c.want {
			t.Errorf("RuleRowText(%q, %q, %q) = %q, want %q", c.name, c.tag, c.def, got, c.want)
		}
	}
}

func TestLeanNumber(t *testing.T) {
	for in, want := range map[string]int{"6": 6, "12": 12, "5+": 5, `6"`: 6, " 4+ ": 4, "self": 0, "": 0} {
		if got := leanNumber(in); got != want {
			t.Errorf("leanNumber(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestLeanPowersTakeTheirTextFromVanilla(t *testing.T) {
	vb := &VanillaBook{powers: map[string]VanillaPower{
		"Fury":          {"Fury", "Exhaustion", "Vanilla Fury."},
		"Fury (Beast)":  {"Fury (Beast)", "Exhaustion", "Beast."},
		"Fury (Spider)": {"Fury (Spider)", "Exhaustion", "Spider."},
		"Curse":         {"Curse", "Instant", "Vanilla Curse."},
	}, powerOrder: []string{"Fury", "Fury (Beast)", "Curse", "Fury (Spider)"}}
	got := LeanPowers(&lean.Magic{Powers: map[string]lean.Power{"Fury": {Range: "self", Casting: "4+"}, "Curse": {Range: `12"`, Casting: "3+"}}}, vb)
	want := []PowerOffer{
		{Name: "Curse", Casting: 3, Range: 12, Duration: "Instant", Definition: "Vanilla Curse."},
		{Name: "Fury", Casting: 4, Duration: "Exhaustion", Definition: "Vanilla Fury.", Variants: []string{"Fury (Beast)", "Fury (Spider)"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
