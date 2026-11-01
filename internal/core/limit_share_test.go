package core

import (
	"reflect"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// The limit shares are exact integer fractions (docs/customizing-rulesets.md, "Shares"); these tests pin the integer arithmetic against
// a plain reference and check that a file still written with percents gives the very same limits as the fractions.

// referenceLimit is the most models a share of n allows, found by counting up or down instead of by the formula.
func referenceLimit(num, den, n int, roundUp bool) int {
	if roundUp {
		k := 0
		for k*den < n*num { // the smallest k with k/n >= num/den
			k++
		}
		return k
	}
	k := n
	for k*den > n*num { // the largest k with k/n <= num/den
		k--
	}
	return k
}

func TestLimitMaxIsExactIntegerArithmetic(t *testing.T) {
	for _, s := range []lean.Share{{Num: 1, Den: 3}, {Num: 1, Den: 2}, {Num: 2, Den: 3}, {Num: 3, Den: 4}} {
		for _, roundUp := range []bool{true, false} {
			for n := 1; n <= 100; n++ {
				if got, want := LimitMax(s, n, roundUp), referenceLimit(s.Num, s.Den, n, roundUp); got != want {
					t.Errorf("LimitMax(%v, %d, roundUp=%v) = %d, want %d", s, n, roundUp, got, want)
				}
			}
		}
	}
	// the cases the old float formula needed a fudge for: a third of every multiple of three is exact
	for n := 3; n <= 99; n += 3 {
		if up, down := LimitMax(lean.Share{Num: 1, Den: 3}, n, true), LimitMax(lean.Share{Num: 1, Den: 3}, n, false); up != n/3 || down != n/3 {
			t.Errorf("a third of %d: up %d, down %d, want %d", n, up, down, n/3)
		}
	}
	if got := LimitMax(lean.Share{Num: 1, Den: 3}, 6, false); got != 2 {
		t.Errorf("a third of 6 rounded down is 2, got %d", got)
	}
	if got := LimitMax(lean.Share{}, 6, true); got != 0 {
		t.Errorf("an unset share allows nothing, got %d", got)
	}
}

// limitsOfSizes is every limit of the rule set for companies of 1 to 40 plain models.
func limitsOfSizes(rs *RuleSet) [][]CompanyLimit {
	var out [][]CompanyLimit
	for n := 1; n <= 40; n++ {
		out = append(out, computeLimits(rs.BC.limitRules, BCCompany{}, make([]CompanyUnit, n)))
	}
	return out
}

func TestPercentLimitsGiveTheLimitsOfTheFractions(t *testing.T) {
	fractions := limitsOfSizes(shippedRules(t))
	legacy := customRuleset(t, func(doc map[string]any) {
		for _, id := range []string{"bow", "throwing", "cavalry"} {
			limitPercent(t, doc, id, 33.33) // what the file wrote before the fractions
		}
	})
	if got := limitsOfSizes(legacy); !reflect.DeepEqual(got, fractions) {
		t.Error("33.33 percent gives other limits than 1/3 for a company of 1 to 40 models")
	}
}

func TestOverrideAsFractionOrPercentGivesTheSameLimits(t *testing.T) {
	osgiliath := func(rs *RuleSet) BCCompany {
		c, ok := rs.BC.Company("Osgiliath & Ithilien")
		if !ok {
			t.Fatal("no Osgiliath & Ithilien")
		}
		return c
	}
	setOverride := func(v any) *RuleSet {
		return customRuleset(t, func(doc map[string]any) {
			for _, c := range doc["companies"].([]any) {
				if m := c.(map[string]any); m["name"] == "Osgiliath & Ithilien" {
					m["limits"].(map[string]any)["override"] = map[string]any{"bow": v}
				}
			}
		})
	}
	var want []int
	for _, v := range []any{"1/2", 50.0, "2/4"} {
		rs := setOverride(v)
		c := osgiliath(rs)
		if got := c.LimitOverrides["bow"]; got != (lean.Share{Num: 1, Den: 2}) {
			t.Errorf("override %v = %v, want 1/2", v, got)
		}
		var maxes []int
		for n := 1; n <= 30; n++ {
			maxes = append(maxes, computeLimits(rs.BC.limitRules, c, make([]CompanyUnit, n))[0].Max)
		}
		if want == nil {
			want = maxes
		} else if !reflect.DeepEqual(maxes, want) {
			t.Errorf("override %v gives %v, want %v", v, maxes, want)
		}
	}
}

func TestBreakPointAsFractionOrPercent(t *testing.T) {
	for _, v := range []any{"33/50", 66.0} {
		rs := customRuleset(t, func(doc map[string]any) {
			for _, c := range doc["companies"].([]any) {
				if m := c.(map[string]any); m["name"] == "Muster of Isengard" {
					m["breakPoint"] = v
				}
			}
		})
		if got := breakPointPercent(rs, "Muster of Isengard"); got != 66 {
			t.Errorf("Muster of Isengard breakPoint %v = %d percent, want 66", v, got)
		}
	}
	for _, v := range []any{"2/5", 40.0} {
		rs := customRuleset(t, func(doc map[string]any) {
			d := campaignAt(t, doc, "campaign", "breakPoint")["default"].(map[string]any)
			delete(d, "share")
			delete(d, "percent")
			if s, ok := v.(string); ok {
				d["share"] = s
			} else {
				d["percent"] = v
			}
		})
		if got := rs.BC.BreakRules().DefaultPercent; got != 40 {
			t.Errorf("default Break Point %v = %d percent, want 40", v, got)
		}
	}
}

func TestKeywordShareAsFractionOrPercent(t *testing.T) {
	for _, v := range []any{"33/100", 33.0} {
		rs := customRuleset(t, func(doc map[string]any) {
			for _, c := range doc["companies"].([]any) {
				if m := c.(map[string]any); m["name"] == "Helm's Deep" {
					k := m["limits"].(map[string]any)["company"].([]any)[0].(map[string]any)
					delete(k, "share")
					delete(k, "percent")
					if s, ok := v.(string); ok {
						k["share"] = s
					} else {
						k["percent"] = v
					}
				}
			}
		})
		p, _ := rs.BC.Profile("Helm's Deep")
		if len(p.KeywordLimits) != 1 || p.KeywordLimits[0].Share != (lean.Share{Num: 33, Den: 100}) {
			t.Errorf("Elf share %v: %+v", v, p.KeywordLimits)
		}
	}
}
