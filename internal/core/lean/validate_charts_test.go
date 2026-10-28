package lean

import (
	"reflect"
	"testing"
)

func TestRangesText(t *testing.T) {
	for in, want := range map[string][]int{"": nil, "1": {1}, "1-3": {1, 2, 3}, "1-3, 5": {1, 2, 3, 5}, "2, 4, 6-7": {2, 4, 6, 7}} {
		if got := rangesText(want); got != in {
			t.Errorf("rangesText(%v) = %q, want %q", want, got, in)
		}
	}
}

func TestChartCoverage(t *testing.T) {
	d6, threeD6 := DiceSpec{1, 6}, DiceSpec{3, 6}
	mk := func(rolls ...Roll) []rolled {
		out := make([]rolled, len(rolls))
		for i, r := range rolls {
			out[i] = rolled{r, "rows[" + string(rune('0'+i)) + "].roll"}
		}
		return out
	}
	for _, c := range []struct {
		name  string
		rows  []rolled
		spec  DiceSpec
		form  string
		lines []string
	}{
		{"exact d6", mk(Roll{1, 1}, Roll{2, 3}, Roll{4, 6}), d6, "chart", nil},
		{"exact 3d6", mk(Roll{3, 10}, Roll{11, 18}), threeD6, "path", nil},
		{"one missing", mk(Roll{1, 5}), d6, "chart", []string{"rows: rolls cover 1-5, 6 is missing (a D6 chart must cover 1-6 exactly once)"}},
		{"several missing", mk(Roll{2, 2}, Roll{4, 4}), d6, "chart", []string{"rows: rolls cover 2, 4; 1, 3, 5-6 are missing (a D6 chart must cover 1-6 exactly once)"}},
		{"nothing covered", mk(Roll{3, 2}), d6, "chart", []string{`rows[0].roll: "3-2" is empty`, "rows: rolls cover nothing; 1-6 are missing (a D6 chart must cover 1-6 exactly once)"}},
		{"overlap", mk(Roll{1, 3}, Roll{3, 6}), d6, "chart", []string{"rows[1].roll: roll 3 is covered twice (by rows[0] and rows[1]); each total belongs to exactly one row"}},
		{"outside", mk(Roll{1, 6}, Roll{7, 8}), d6, "chart", []string{"rows[1].roll: roll 7-8 is outside 1-6, the totals of D6"}},
		{"path form", mk(Roll{1, 2}), d6, "path", []string{"rows: rolls 1..6 need an option each; 3-6 are missing"}},
		{"table form", mk(Roll{1, 2}), d6, "table", []string{"rows: rows cover 1-2; 3-6 are missing (a table keyed by D6 must cover 1-6 exactly once)"}},
		{"absent table is optional", nil, d6, "table", nil},
		{"absent chart is an error", nil, d6, "chart", []string{"rows: rolls cover nothing; 1-6 are missing (a D6 chart must cover 1-6 exactly once)"}},
		{"invalid spec is skipped", mk(Roll{1, 1}), DiceSpec{}, "chart", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			var rep Report
			v := &validator{rep: NewReporter("f", &rep)}
			v.chart("rows", c.rows, c.spec, c.form)
			if got := issueLines(rep); !reflect.DeepEqual(got, c.lines) {
				t.Errorf("got  %q\nwant %q", got, c.lines)
			}
		})
	}
}

func TestParameterChecks(t *testing.T) {
	mutate := func(f func(c *Campaign)) Campaign {
		doc, _ := normalised(t, minimalWith(t, ``))
		f(&doc.Campaign)
		return doc.Campaign
	}
	for _, c := range []struct {
		name string
		f    func(c *Campaign)
		want []string
	}{
		{"defaults are fine", func(c *Campaign) {}, nil},
		{"choose above rolls", func(c *Campaign) { c.Progression.Hero.Choose = 3 }, []string{"campaign.progression.hero.choose: 3 is more than rollsPerProgression 2"}},
		{"officers above models", func(c *Campaign) { c.Company.MaxModels = 3; c.Company.Leaders = 2 }, []string{"campaign.company.sergeants: leaders 2 + sergeants 2 is more than maxModels 3"}},
		{"no officer", func(c *Campaign) { c.Company.Leaders = 0; c.Company.Sergeants = 0 }, []string{"campaign.company.leaders: 0 with sergeants 0; a company needs at least one officer"}},
		{"duplicate gain key", func(c *Campaign) { c.Experience.Gains[1].Key = "participated" }, []string{`campaign.experience.gains[1].key: "participated" is already used at campaign.experience.gains[0]`}},
		{"scenario die of two", func(c *Campaign) { c.ScenarioSelection.Die = DiceSpec{2, 6} }, []string{`campaign.scenarioSelection.die: "2d6" has 2 dice; the scenario die is a single die`}},
	} {
		t.Run(c.name, func(t *testing.T) {
			camp := mutate(c.f)
			var rep Report
			v := &validator{doc: &Document{Campaign: camp}, rep: NewReporter("f", &rep)}
			v.parameters(&camp)
			v.scenarioTables(&camp.ScenarioSelection)
			if got := issueLines(rep); !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %q\nwant %q", got, c.want)
			}
		})
	}
}
