package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// LeanMaxPoints is the most a player can score on a scoring objective: its best tier, times the count of a "for each" objective.
func LeanMaxPoints(kind string, count int, options []lean.ScoringOption) int {
	best := 0
	for _, o := range options {
		best = max(best, o.Points)
	}
	switch kind {
	case "eachTiers", "each":
		return count * best
	}
	return best
}

// LeanScenarios reads the scenarios of a lean document in the order of its list: name, place on the scenario selection tables, the
// text of its Victory Point conditions (one entry per objective) and its objectives: the rewards whose condition the players tick
// (companyFlag, and modelFlag shown as one tick per company), each with the reward of its campaign.aftermathEffects[] entry. Rewards
// that follow from the result (victory, leaderNotRemoved) are not ticks. They are sorted by the place on the selection tables, the
// unplaced last.
func LeanScenarios(d *lean.Document) ([]ScenarioInfo, error) {
	type effect struct {
		kind, text string
		amount     int
	}
	effects := map[string]effect{}
	for _, e := range d.Campaign.AftermathEffects {
		var x effect
		x.kind, _ = e.Rest["kind"].(string)
		x.text, _ = e.Rest["text"].(string)
		x.amount = restInt(e.Rest["amount"])
		effects[e.Key] = x
	}
	var out []ScenarioInfo
	for _, sc := range d.Scenarios {
		info := ScenarioInfo{Name: sc.Name}
		for t, table := range d.Campaign.ScenarioSelection.Tables {
			if i := slices.Index(table.Scenarios, sc.Name); i >= 0 {
				info.Table, info.Roll = t+1, i+1
				info.FirstDie = [2]int{table.Roll.Lo, table.Roll.Hi}
			}
		}
		info.About = ScenarioAbout{Outline: sc.Outline, Description: sc.Description}
		for _, sec := range []struct{ title, text string }{{"Armies", sc.Armies}, {"Layout", sc.Layout}, {"Starting positions", sc.StartingPositions}, {"How the game ends", sc.Objectives}} {
			if strings.TrimSpace(sec.text) != "" {
				info.About.Sections = append(info.About.Sections, ScenarioText{Title: sec.title, Text: sec.text})
			}
		}
		var rules []struct {
			Name       string `json:"name"`
			Definition string `json:"definition"`
			Table      *struct {
				Lookup  string   `json:"lookup"`
				Columns []string `json:"columns"`
				Rows    []struct {
					Min   int      `json:"min"`
					Max   *int     `json:"max"`
					Cells []string `json:"cells"`
				} `json:"rows"`
			} `json:"table"`
		}
		var roles []struct {
			Name string `json:"name"`
		}
		var rewards []struct {
			Effect    string `json:"effect"`
			Name      string `json:"name"`
			Who       string `json:"who"`
			Text      string `json:"text"`
			Condition struct {
				Kind  string `json:"kind"`
				Key   string `json:"key"`
				Label string `json:"label"`
			} `json:"condition"`
		}
		for _, free := range []struct {
			from, into any
		}{{sc.SpecialRules, &rules}, {sc.Roles, &roles}, {sc.Rewards, &rewards}} {
			if err := leanRoundTrip(free.from, free.into); err != nil {
				return nil, errors.Join(ErrInvalidRuleset, fmt.Errorf("scenarios[%s]: %w", sc.Name, err))
			}
		}
		for _, r := range rules {
			text := ScenarioText{Title: r.Name, Text: r.Definition}
			if r.Table != nil {
				t := &ScenarioRuleTable{Lookup: r.Table.Lookup, Columns: r.Table.Columns}
				for _, row := range r.Table.Rows {
					max := -1
					if row.Max != nil {
						max = *row.Max
					}
					t.Rows = append(t.Rows, ScenarioTableRow{Min: row.Min, Max: max, Cells: row.Cells})
				}
				if spec, err := lean.ParseDice(r.Table.Lookup); err == nil {
					t.TableDice = spec
					var spans [][2]int
					for _, row := range t.Rows {
						hi := row.Max
						if hi < 0 {
							hi = spec.Max()
						}
						spans = append(spans, [2]int{row.Min, hi})
					}
					if err := CheckCovers(fmt.Sprintf("scenarios[%s].specialRules[%s].table", sc.Name, r.Name), spec, spans); err != nil {
						return nil, errors.Join(ErrInvalidRuleset, err)
					}
				}
				text.Table = t
			}
			info.About.Rules = append(info.About.Rules, text)
		}
		info.About.Uses = sc.UsesRules
		for _, r := range roles {
			info.About.Roles = append(info.About.Roles, r.Name)
		}
		for _, o := range sc.Scoring {
			sentences := make([]string, len(o.Options))
			for i, opt := range o.Options {
				sentences[i] = opt.Text
			}
			so := ScoringObjective{Label: ObjectiveLabel(o.Key), Key: o.Key, Text: ComposeObjectiveText(o.Instruction, sentences), Side: o.Side, Kind: o.Kind, Instruction: o.Instruction, Secret: o.Secret, MaxPoints: LeanMaxPoints(o.Kind, o.Count, o.Options)}
			info.Scoring = append(info.Scoring, so.Text)
			if n := o.Nomination; n != nil {
				so.Nomination = &ObjectiveNomination{From: n.From, AllowLeader: n.AllowLeader, Where: n.Where}
			}
			for _, opt := range o.Options {
				so.Options = append(so.Options, ScoringTier{Text: opt.Text, Points: opt.Points})
			}
			slices.SortStableFunc(so.Options, func(a, b ScoringTier) int { return a.Points - b.Points }) // the tiers ascend whatever order the sentences are printed in
			info.ScoringObjectives = append(info.ScoringObjectives, so)
		}
		for _, rw := range rewards {
			if rw.Condition.Kind != "companyFlag" && rw.Condition.Kind != "modelFlag" { // a reward that follows from the result: shown, not ticked
				who := map[string]string{"victor": "The winner", "eachCompanyWhoseLeaderSurvives": "Each company whose Leader is not removed"}[rw.Who]
				info.Prizes = append(info.Prizes, ScenarioPrize{Name: rw.Name, Who: who, Text: rw.Text})
				continue
			}
			if rw.Condition.Key == "" {
				continue
			}
			reward := ScenarioReward{Kind: "other", Text: rw.Text}
			if e, ok := effects[rw.Effect]; ok {
				reward.Text = e.text
				switch e.kind {
				case "influence", "experience":
					reward.Kind, reward.Amount = e.kind, e.amount
				case "freeEquipment":
					reward.Kind = "equipment"
				}
			}
			info.Objectives = append(info.Objectives, ScenarioObjective{Key: rw.Condition.Key, Label: rw.Condition.Label, Max: 1, Model: rw.Condition.Kind == "modelFlag", Rewards: []ScenarioReward{reward}})
		}
		out = append(out, info)
	}
	slices.SortStableFunc(out, func(a, b ScenarioInfo) int {
		if a.Table != b.Table {
			if a.Table == 0 || b.Table == 0 { // unplaced scenarios last
				return b.Table - a.Table
			}
			return a.Table - b.Table
		}
		return a.Roll - b.Roll
	})
	return out, nil
}

// restInt reads a whole number out of a value kept in a Rest map (the decoder keeps numbers as written).
func restInt(v any) int {
	switch n := v.(type) {
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}
