package core

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ScenarioSelection is how a scenario is picked by dice (campaign.scenarioSelection): the first die picks one of the tables by its
// range, the second picks the scenario on it. Die is the size of both (scenarioSelection.die, d6); every table lists Die.Sides
// scenarios and the ranges partition 1..Sides.
type ScenarioSelection struct {
	Die    DiceSpec
	Tables []ScenarioTableRange // by table, in the order of the file (Table 1 is the first)
}

// ScenarioTableRange is the first-die range [Min, Max] that picks a table.
type ScenarioTableRange struct{ Min, Max int }

// ParseScenarioSelection reads campaign.scenarioSelection. The die is "d6" while the shipped file does not carry the key yet
// (REVIEW: the definitions-curator adds scenarioSelection.die, bead battle-company-manager-ce-v2-60m7); a file with no tables has an
// empty selection. A bad die, ranges that do not partition the die, or a table with the wrong number of scenarios is an error.
func ParseScenarioSelection(content []byte) (ScenarioSelection, error) {
	var doc struct {
		Campaign struct {
			Selection struct {
				Die    string `json:"die"`
				Tables []struct {
					Roll      []int    `json:"roll"`
					Scenarios []string `json:"scenarios"`
				} `json:"tables"`
			} `json:"scenarioSelection"`
		} `json:"campaign"`
	}
	if err := json.Unmarshal(content, &doc); err != nil {
		return ScenarioSelection{}, err
	}
	c := doc.Campaign.Selection
	sel := ScenarioSelection{Die: D6Spec}
	if c.Die != "" {
		spec, err := ParseDiceAt("campaign.scenarioSelection.die", c.Die)
		switch {
		case err != nil:
			return ScenarioSelection{}, errors.Join(ErrInvalidRuleset, err)
		case spec.Count != 1:
			return ScenarioSelection{}, errors.Join(ErrInvalidRuleset, fmt.Errorf("campaign.scenarioSelection.die: %q must be one die", c.Die))
		}
		sel.Die = spec
	}
	if len(c.Tables) == 0 {
		return sel, nil
	}
	var spans [][2]int
	for i, t := range c.Tables {
		if len(t.Roll) != 2 {
			return ScenarioSelection{}, errors.Join(ErrInvalidRuleset, fmt.Errorf("campaign.scenarioSelection.tables[%d].roll must be [low, high]", i))
		}
		spans = append(spans, [2]int{t.Roll[0], t.Roll[1]})
		sel.Tables = append(sel.Tables, ScenarioTableRange{t.Roll[0], t.Roll[1]})
		if len(t.Scenarios) != sel.Die.Sides {
			return ScenarioSelection{}, errors.Join(ErrInvalidRuleset, fmt.Errorf("campaign.scenarioSelection.tables[%d]: %d scenarios, expected %d (one for each face of the second die, %s)", i, len(t.Scenarios), sel.Die.Sides, sel.Die))
		}
	}
	if err := CheckCovers("campaign.scenarioSelection.tables", sel.Die, spans); err != nil {
		return ScenarioSelection{}, errors.Join(ErrInvalidRuleset, err)
	}
	return sel, nil
}

// Table is the table (1, 2, ...) the first die picks; 0 for a die outside 1..Sides or when the file has no tables.
func (s ScenarioSelection) Table(d int) int {
	for i, t := range s.Tables {
		if d >= t.Min && d <= t.Max {
			return i + 1
		}
	}
	return 0
}

// Valid says whether a die shows a face (1..Sides).
func (s ScenarioSelection) Valid(d int) bool { return d >= 1 && d <= s.Die.Sides }

// ScenarioByRolls maps the two dice of the scenario selection onto a scenario of the list: the first die picks the table, the
// second the scenario on it (its Roll). It reports false when a die is outside 1..Sides or the rules file places no scenario there.
func ScenarioByRolls(sel ScenarioSelection, list []ScenarioInfo, first, second int) (ScenarioInfo, bool) {
	table := sel.Table(first)
	if table == 0 || !sel.Valid(first) || !sel.Valid(second) {
		return ScenarioInfo{}, false
	}
	for _, sc := range list {
		if sc.Table == table && sc.Roll == second {
			return sc, true
		}
	}
	return ScenarioInfo{}, false
}

// Rating is the Battle Company Rating of a side: the points of its companies (injured units counted at full value when the
// company knows it).
func (s MatchSide) Rating() int {
	n := 0
	for _, c := range s.Companies {
		if c.FullPoints > 0 {
			n += c.FullPoints
		} else {
			n += c.Points
		}
	}
	return n
}

// TopDog says which side of a two-sided match has the higher rating (the Top Dog, 0-based index) and which the Underdog.
// A tie, or a match that does not have exactly two sides, has neither: both are -1.
func (m Match) TopDog() (top, under int) {
	if len(m.Sides) != 2 || m.Sides[0].Rating() == m.Sides[1].Rating() {
		return -1, -1
	}
	if m.Sides[0].Rating() > m.Sides[1].Rating() {
		return 0, 1
	}
	return 1, 0
}
