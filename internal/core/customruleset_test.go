package core

import (
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
)

// customRuleset loads the shipped rules files, lets mutate change the Battle Companies document (a generic map: set one
// parameter, delete one key) and builds a RuleSet through LoadRuleSet, the path production uses. It is the harness of
// TestCustomRuleset (docs/testing.md, "Custom rulesets"); the rows of the engine parameters use it.
func customRuleset(t *testing.T, mutate func(doc map[string]any)) *RuleSet {
	t.Helper()
	rs, err := customRulesetErr(t, mutate)
	if err != nil {
		t.Fatalf("custom ruleset does not load: %v", err)
	}
	return rs
}

// customRulesetErr is customRuleset for a file the loader may refuse: it returns the loader's error.
func customRulesetErr(t *testing.T, mutate func(doc map[string]any)) (*RuleSet, error) {
	t.Helper()
	read := func(p string) []byte {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Skip("definitions not available: ", err)
		}
		return b
	}
	bc := read("../../definitions/battle_companies/battle_companies.json")
	vanilla := read("../../definitions/vanilla/data2024.json")
	glossary := read("../../definitions/glossary/mesbg_glossary.json")
	if mutate != nil {
		var doc map[string]any
		if err := json.Unmarshal(bc, &doc); err != nil {
			t.Fatal(err)
		}
		mutate(doc)
		var err error
		if bc, err = json.Marshal(doc); err != nil {
			t.Fatal(err)
		}
	}
	return LoadRuleSet(vanilla, bc, glossary, slog.New(slog.DiscardHandler))
}

// customAftermath is the aftermath rules of the shipped file with mutate applied.
func customAftermath(t *testing.T, mutate func(doc map[string]any)) (AftermathRules, error) {
	t.Helper()
	rs, err := customRulesetErr(t, mutate)
	if err != nil {
		return AftermathRules{}, err
	}
	return rs.BC.Aftermath(), nil
}

// jsonAt walks keys (map keys, or slice indexes as ints) down a decoded document.
func jsonAt(t *testing.T, v any, path ...any) any {
	t.Helper()
	for _, p := range path {
		switch k := p.(type) {
		case string:
			m, ok := v.(map[string]any)
			if !ok || m[k] == nil && !has(m, k) {
				t.Fatalf("no key %q", k)
			}
			v = m[k]
		case int:
			s, ok := v.([]any)
			if !ok || k >= len(s) {
				t.Fatalf("no index %d", k)
			}
			v = s[k]
		}
	}
	return v
}

func has(m map[string]any, k string) bool { _, ok := m[k]; return ok }

// TestCustomRuleset has one row per engine parameter (docs/testing.md, "Custom rulesets"); each
// row mutates the shipped file and asserts the new behaviour through the built RuleSet.
func TestCustomRuleset(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, doc map[string]any)
		check  func(t *testing.T, rs *RuleSet)
	}{
		{
			name:   "unchanged file: the treatment value of the shipped rules",
			mutate: func(t *testing.T, doc map[string]any) {},
			check: func(t *testing.T, rs *RuleSet) {
				if got := rs.BC.Injuries().Attempt.PerPoint; got != 1 {
					t.Errorf("attempt per point = %d, want 1", got)
				}
			},
		},
		{
			name: "injuries.treatment attempt perPoint is read from data",
			mutate: func(t *testing.T, doc map[string]any) {
				hero := jsonAt(t, doc, "campaign", "injuries", "treatment", "hero").([]any)
				for _, h := range hero {
					m := h.(map[string]any)
					if m["key"] == "attempt" {
						m["roll"].(map[string]any)["modifier"].(map[string]any)["perPoint"] = 2.0
					}
				}
			},
			check: func(t *testing.T, rs *RuleSet) {
				if got := rs.BC.Injuries().Attempt.PerPoint; got != 2 {
					t.Errorf("attempt per point = %d, want 2", got)
				}
				if got := rs.BC.Injuries().Healers.PerPoint; got == 2 {
					t.Errorf("the healers method changed too: %d", got)
				}
			},
		},
		{
			name:   "P4 unchanged file: the dice of the shipped rules",
			mutate: func(t *testing.T, doc map[string]any) {},
			check: func(t *testing.T, rs *RuleSet) {
				d6, twoD6 := NdM(1, 6), NdM(2, 6)
				p, b := rs.BC.Progression(), rs.BC.BreakRules()
				if p.WarriorDice != d6 || p.HeroDice != twoD6 || p.HeroRolls != 2 || p.HeroChoose != 1 {
					t.Errorf("progression dice = %s %s rolls %d choose %d", p.WarriorDice, p.HeroDice, p.HeroRolls, p.HeroChoose)
				}
				if path, ok := p.Path("Path of the Tactician"); !ok || path.PathDice != twoD6 {
					t.Errorf("path dice = %v %v", path.PathDice, ok)
				}
				if b.CourageSpec != twoD6 || rs.BC.Selection().Die != d6 || len(rs.BC.Selection().Tables) != 3 {
					t.Errorf("courage %s, selection %+v", b.CourageSpec, rs.BC.Selection())
				}
			},
		},
		{
			name: "P4 D1: injuries.roll d12 with charts that start at 1",
			mutate: func(t *testing.T, doc map[string]any) {
				inj := jsonAt(t, doc, "campaign", "injuries").(map[string]any)
				inj["roll"] = "d12"
				for _, chart := range []string{"warrior", "hero"} {
					first := jsonAt(t, inj, chart, 0).(map[string]any)
					first["roll"].([]any)[0] = 1.0 // the 2d6 charts start at 2; a d12 can show a 1
				}
			},
			check: func(t *testing.T, rs *RuleSet) {
				ar, err := customAftermath(t, func(doc map[string]any) {
					inj := jsonAt(t, doc, "campaign", "injuries").(map[string]any)
					inj["roll"] = "d12"
					for _, chart := range []string{"warrior", "hero"} {
						jsonAt(t, inj, chart, 0).(map[string]any)["roll"].([]any)[0] = 1.0
					}
				})
				if err != nil || ar.InjuryDice != (NdM(1, 12)) {
					t.Fatalf("aftermath rules: %v %v", ar.InjuryDice, err)
				}
				if row, ok := ResolveCasualty(ar.Warrior, 1, CasualtyUnit{}); !ok || row.Row.Key != "dead" {
					t.Errorf("a 1 on the d12 reads the first row: %+v %v", row.Row.Key, ok)
				}
			},
		},
		{
			name: "P4 E4: the Courage test is rolled on 3d6",
			mutate: func(t *testing.T, doc map[string]any) {
				jsonAt(t, doc, "campaign", "breakPoint", "whenBroken", "courageTest").(map[string]any)["dice"] = "3d6"
			},
			check: func(t *testing.T, rs *RuleSet) {
				if got := rs.BC.BreakRules().CourageSpec; got != (NdM(3, 6)) {
					t.Errorf("courage dice = %s, want 3d6", got)
				}
			},
		},
		{
			name: "P4 D2: Lesson Learned grants a D4 of experience",
			mutate: func(t *testing.T, doc map[string]any) {
				for _, r := range jsonAt(t, doc, "campaign", "injuries", "warrior").([]any) {
					if m := r.(map[string]any); m["key"] == "lessonLearned" {
						m["grantsExperience"].(map[string]any)["dice"] = "d4"
					}
				}
			},
			check: func(t *testing.T, rs *RuleSet) {
				ar, err := customAftermath(t, func(doc map[string]any) {
					for _, r := range jsonAt(t, doc, "campaign", "injuries", "warrior").([]any) {
						if m := r.(map[string]any); m["key"] == "lessonLearned" {
							m["grantsExperience"].(map[string]any)["dice"] = "d4"
						}
					}
				})
				if err != nil {
					t.Fatal(err)
				}
				for _, row := range ar.Warrior {
					if row.Key == "lessonLearned" && (row.BonusSpec != NdM(1, 4) || row.BonusDice != "d4") {
						t.Errorf("bonus dice = %q %s, want d4", row.BonusDice, row.BonusSpec)
					}
				}
			},
		},
		{
			name:   "P1 unchanged file: the recruiting parameters of the shipped rules",
			mutate: func(t *testing.T, doc map[string]any) {},
			check: func(t *testing.T, rs *RuleSet) {
				want := RecruitRules{Adjust: AdjustRules{Max: 3, CostPerPoint: 1, Direction: AdjustUp, Die: true}, Sides: 6, Rare: RareRules{Injured: true},
					Rendezvous: PerkRange{-1, 1}, VantagePoint: PerkRange{-1, 1}}
				if got := rs.BC.Recruiting(); got != want {
					t.Errorf("recruiting = %+v, want %+v", got, want)
				}
			},
		},
		{
			name: "P1 A1-A3: cap 5, both directions, 2 influence per point",
			mutate: func(t *testing.T, doc map[string]any) {
				a := jsonAt(t, doc, "campaign", "recruitment", "adjust").(map[string]any)
				a["maxPerAttempt"], a["direction"], a["costPerPoint"] = 5.0, "both", 2.0
			},
			check: func(t *testing.T, rs *RuleSet) {
				a := rs.BC.Recruiting().Adjust
				if a.Max != 5 || a.Direction != AdjustBoth || a.CostPerPoint != 2 || !a.CanLower() || !a.CanRaise() {
					t.Errorf("adjust = %+v", a)
				}
				if up, down := a.Budget(0, 3, 6); up != 3 || down != 2 {
					t.Errorf("from a 3 on a d6: up %d, down %d, want 3 and 2 (the die stays on its faces)", up, down)
				}
				if up, down := a.Budget(4, 3, 6); up != 1 || down != 1 {
					t.Errorf("4 of 5 points used: up %d, down %d, want 1 and 1", up, down)
				}
			},
		},
		{
			name: "P1 A2: direction down cannot raise",
			mutate: func(t *testing.T, doc map[string]any) {
				jsonAt(t, doc, "campaign", "recruitment", "adjust").(map[string]any)["direction"] = "down"
			},
			check: func(t *testing.T, rs *RuleSet) {
				if up, down := rs.BC.Recruiting().Adjust.Budget(0, 3, 6); up != 0 || down != 2 {
					t.Errorf("up %d, down %d, want 0 and 2", up, down)
				}
			},
		},
		{
			name: "P1 Rare: injuredCount false",
			mutate: func(t *testing.T, doc map[string]any) {
				jsonAt(t, doc, "campaign", "recruitment", "rare").(map[string]any)["injuredCount"] = false
			},
			check: func(t *testing.T, rs *RuleSet) {
				if got := rs.BC.Recruiting().Rare; got.Injured || got.Dead || got.Promoted {
					t.Errorf("rare = %+v", got)
				}
			},
		},
		{
			name: "P1 A7: Rendezvous moves a die by up to 2",
			mutate: func(t *testing.T, doc map[string]any) {
				for _, e := range jsonAt(t, doc, "campaign", "aftermathEffects").([]any) {
					if m := e.(map[string]any); m["key"] == "rendezvous" {
						m["range"] = []any{-2.0, 2.0}
					}
				}
			},
			check: func(t *testing.T, rs *RuleSet) {
				r := rs.BC.Recruiting()
				if r.Rendezvous != (PerkRange{-2, 2}) || r.VantagePoint != (PerkRange{-1, 1}) {
					t.Errorf("rendezvous %+v, vantage point %+v", r.Rendezvous, r.VantagePoint)
				}
			},
		},
		{
			name: "P1 A8: starting influence 2",
			mutate: func(t *testing.T, doc map[string]any) {
				jsonAt(t, doc, "campaign", "company").(map[string]any)["startingInfluence"] = 2.0
			},
			check: func(t *testing.T, rs *RuleSet) {
				if got := rs.BC.Recruiting().StartingInfluence; got != 2 {
					t.Errorf("starting influence = %d, want 2", got)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs := customRuleset(t, func(doc map[string]any) { tt.mutate(t, doc) })
			tt.check(t, rs)
		})
	}
}

// TestCustomRulesetRefusesBadRecruitingParameters: a value outside its range, or a chart that does not fit the die, is an error
// that names the path.
func TestCustomRulesetRefusesBadRecruitingParameters(t *testing.T) {
	adjust := func(doc map[string]any) map[string]any {
		return jsonAt(t, doc, "campaign", "recruitment", "adjust").(map[string]any)
	}
	tests := []struct {
		name   string
		mutate func(doc map[string]any)
		want   string
	}{
		{"cap too high", func(doc map[string]any) { adjust(doc)["maxPerAttempt"] = 21.0 }, "campaign.recruitment.adjust.maxPerAttempt: 21 is outside 0..20"},
		{"negative cost", func(doc map[string]any) { adjust(doc)["costPerPoint"] = -1.0 }, "campaign.recruitment.adjust.costPerPoint: -1 is outside 0..10"},
		{"cap missing", func(doc map[string]any) { delete(adjust(doc), "maxPerAttempt") }, "campaign.recruitment.adjust.maxPerAttempt is missing"},
		{"unknown direction", func(doc map[string]any) { adjust(doc)["direction"] = "sideways" }, `campaign.recruitment.adjust.direction: "sideways"`},
		{"adjust section missing", func(doc map[string]any) { delete(jsonAt(t, doc, "campaign", "recruitment").(map[string]any), "adjust") }, "campaign.recruitment.adjust is missing"},
		{"rare flag missing", func(doc map[string]any) {
			delete(jsonAt(t, doc, "campaign", "recruitment", "rare").(map[string]any), "deadCount")
		}, "campaign.recruitment.rare.deadCount: missing"},
		{"starting influence too high", func(doc map[string]any) {
			jsonAt(t, doc, "campaign", "company").(map[string]any)["startingInfluence"] = 101.0
		}, "campaign.company.startingInfluence: 101 is outside 0..100"},
		{"a die the charts do not cover", func(doc map[string]any) { jsonAt(t, doc, "campaign", "recruitment").(map[string]any)["die"] = "d8" }, "a D8 chart must cover 1-8 exactly once"},
		{"two dice", func(doc map[string]any) { jsonAt(t, doc, "campaign", "recruitment").(map[string]any)["die"] = "2d6" }, "a recruiting die is a single die"},
		{"not a dice spec", func(doc map[string]any) { jsonAt(t, doc, "campaign", "recruitment").(map[string]any)["die"] = "2d6x" }, "campaign.recruitment.die:"},
		{"perk range inverted", func(doc map[string]any) {
			for _, e := range jsonAt(t, doc, "campaign", "aftermathEffects").([]any) {
				if m := e.(map[string]any); m["key"] == "vantagePoint" {
					m["range"] = []any{1.0, -1.0}
				}
			}
		}, "low is above high"},
		{"perk range too wide", func(doc map[string]any) {
			for _, e := range jsonAt(t, doc, "campaign", "aftermathEffects").([]any) {
				if m := e.(map[string]any); m["key"] == "rendezvous" {
					m["range"] = []any{-7.0, 1.0}
				}
			}
		}, "-7 is out of range -6..6"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := customRulesetErr(t, tt.mutate)
			if err == nil || !strings.Contains(err.Error(), leanWording(tt.want)) {
				t.Errorf("error = %v, want one naming %q", err, leanWording(tt.want))
			}
		})
	}
}

// TestCustomRulesetRefusesDiceTheTablesDoNotCover: a dice string that is not a spec, or a table keyed by a roll that does not cover
// Min..Max of its dice, is an error of the file that names the path.
func TestCustomRulesetRefusesDiceTheTablesDoNotCover(t *testing.T) {
	set := func(path ...any) func(v any) func(doc map[string]any) {
		return func(v any) func(doc map[string]any) {
			return func(doc map[string]any) {
				parent := jsonAt(t, doc, path[:len(path)-1]...)
				switch p := parent.(type) {
				case map[string]any:
					p[path[len(path)-1].(string)] = v
				}
			}
		}
	}
	tests := []struct {
		name   string
		mutate func(doc map[string]any)
		want   string
	}{
		{"injury dice 3d6: the charts stop at 12", set("campaign", "injuries", "roll")("3d6"), "campaign.injuries.warrior"},
		{"injury dice d6: the charts run to 12", set("campaign", "injuries", "roll")("d6"), "is outside 1-6"},
		{"injury dice d12 with charts that start at 2", set("campaign", "injuries", "roll")("d12"), "a table keyed by D12 must cover 1-12 exactly once"},
		{"injury dice not a spec", set("campaign", "injuries", "roll")("2d6x"), `campaign.injuries.roll: "2d6x" is not a dice spec`},
		{"injury dice d66", set("campaign", "injuries", "roll")("d66"), "d66"},
		{"injury dice missing", func(doc map[string]any) { delete(jsonAt(t, doc, "campaign", "injuries").(map[string]any), "roll") }, "campaign.injuries.roll is missing"},
		{"warrior dice d8: the table stops at 6", set("campaign", "progression", "warrior", "roll")("d8"), "a table keyed by D8 must cover 1-8 exactly once"},
		{"warrior dice 2d6: the table has no 7 to 12", set("campaign", "progression", "warrior", "roll")("2d6"), "is outside 2-12"},
		{"warrior dice with a table above the dice", set("campaign", "progression", "warrior", "roll")("d4"), "is outside 1-4"},
		{"hero dice 3d6: a path without dice of its own stops at 12", func(doc map[string]any) {
			jsonAt(t, doc, "campaign", "progression", "hero").(map[string]any)["roll"] = "3d6"
			delete(jsonAt(t, doc, "paths", 0).(map[string]any), "dice")
		}, "is outside 3-18"},
		{"hero rolls 0", set("campaign", "progression", "hero", "rollsPerProgression")(0.0), "rollsPerProgression: 0 is outside 1..6"},
		{"hero rolls 7", set("campaign", "progression", "hero", "rollsPerProgression")(7.0), "rollsPerProgression: 7 is outside 1..6"},
		{"hero chooses two", set("campaign", "progression", "hero", "choose")(2.0), "campaign.progression.hero.choose: 2 is not supported"},
		{"a path with dice its rows do not cover", func(doc map[string]any) {
			jsonAt(t, doc, "paths", 0).(map[string]any)["dice"] = "d12"
		}, "need an option each"},
		{"courage dice not a spec", set("campaign", "breakPoint", "whenBroken", "courageTest", "dice")("2d6+1"), "campaign.breakPoint.whenBroken.courageTest.dice"},
		{"scenario die d8: the tables cover 1..6", set("campaign", "scenarioSelection", "die")("d8"), "each table lists exactly 8"},
		{"scenario die two dice", set("campaign", "scenarioSelection", "die")("2d6"), "the scenario die is a single die"},
		{"bonus dice not a spec", func(doc map[string]any) {
			for _, r := range jsonAt(t, doc, "campaign", "injuries", "warrior").([]any) {
				if m := r.(map[string]any); m["key"] == "lessonLearned" {
					m["grantsExperience"].(map[string]any)["dice"] = "d3d3"
				}
			}
		}, "is not a dice spec"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := customRulesetErr(t, tt.mutate)
			if err == nil || !strings.Contains(err.Error(), leanWording(tt.want)) {
				t.Errorf("error = %v, want one containing %q", err, leanWording(tt.want))
			}
		})
	}
}
