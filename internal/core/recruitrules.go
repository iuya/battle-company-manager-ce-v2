package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// Adjustment directions of campaign.recruitment.adjust.direction.
const (
	AdjustUp   = "up"
	AdjustDown = "down"
	AdjustBoth = "both"
)

// AdjustRules is how influence moves the die of a recruiting roll (campaign.recruitment.adjust), docs/definitions.md.
type AdjustRules struct {
	Max          int    // most points of adjustment per recruiting attempt, over both charts together (maxPerAttempt)
	CostPerPoint int    // influence per point, either direction
	Direction    string // AdjustUp, AdjustDown or AdjustBoth
	Die          bool   // true: the points move the die (appliesTo "die"), the only kind the engine knows
}

// CanRaise and CanLower say which way a point may go.
func (a AdjustRules) CanRaise() bool { return a.Direction == AdjustUp || a.Direction == AdjustBoth }
func (a AdjustRules) CanLower() bool { return a.Direction == AdjustDown || a.Direction == AdjustBoth }

// Budget is how many points a roll can still be moved up and down: the attempt's points that are left and the die range. What the
// company can afford is the caller's to apply (CostPerPoint).
func (a AdjustRules) Budget(used, roll, sides int) (up, down int) {
	left := max(0, a.Max-used)
	if a.CanRaise() {
		up = max(0, min(left, sides-roll))
	}
	if a.CanLower() {
		down = max(0, min(left, roll-1))
	}
	return up, down
}

// RareRules says which models of a profile count towards Rare N (campaign.recruitment.rare).
type RareRules struct {
	Injured, Dead, Promoted bool
}

// PerkRange is how far a perk moves a die (campaign.aftermathEffects[].range).
type PerkRange struct{ Low, High int }

// Allows reports whether the move is within the range.
func (p PerkRange) Allows(n int) bool { return n >= p.Low && n <= p.High }

// RecruitRules are the parameters of recruiting that the engine reads from the Battle Companies file.
type RecruitRules struct {
	Adjust            AdjustRules
	Sides             int // the reinforcement die (campaign.recruitment.die, "d6"); every chart covers 1..Sides
	Rare              RareRules
	Rendezvous        PerkRange // campaign.aftermathEffects[key=rendezvous].range
	VantagePoint      PerkRange // campaign.aftermathEffects[key=vantagePoint].range
	StartingInfluence int       // campaign.company.startingInfluence
}

// ParseRecruitRules reads and validates the recruiting parameters. The die and the starting influence are optional while the
// shipped file does not carry them yet (REVIEW: bead battle-company-manager-ce-v2-dfk): then d6 and 0, today's values.
func ParseRecruitRules(content []byte) (RecruitRules, error) {
	var doc struct {
		Campaign struct {
			Company struct {
				StartingInfluence *int `json:"startingInfluence"`
			} `json:"company"`
			Recruitment struct {
				Die    *string `json:"die"`
				Adjust *struct {
					Direction     string `json:"direction"`
					CostPerPoint  *int   `json:"costPerPoint"`
					MaxPerAttempt *int   `json:"maxPerAttempt"`
					AppliesTo     string `json:"appliesTo"`
				} `json:"adjust"`
				Rare *struct {
					Injured  *bool `json:"injuredUnitsCount"`
					Dead     *bool `json:"deadUnitsCount"`
					Promoted *bool `json:"promotedUnitsCount"`
				} `json:"rare"`
			} `json:"recruitment"`
			AftermathEffects []struct {
				Key   string  `json:"key"`
				Range *[2]int `json:"range"`
			} `json:"aftermathEffects"`
		} `json:"campaign"`
	}
	if err := json.Unmarshal(content, &doc); err != nil {
		return RecruitRules{}, fmt.Errorf("campaign.recruitment: %w", err)
	}
	if !bytes.Contains(content, []byte(`"campaign"`)) { // a file without a campaign section (a fixture, a vanilla-only test) has no parameters to read
		return RecruitRules{Sides: 6}, nil
	}
	c := doc.Campaign
	var r RecruitRules
	var problems []string
	bad := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	inRange := func(path string, v *int, lo, hi int) int {
		switch {
		case v == nil:
			bad("%s is missing", path)
			return 0
		case *v < lo || *v > hi:
			bad("%s: %d is outside %d..%d", path, *v, lo, hi)
		}
		return *v
	}

	a := c.Recruitment.Adjust
	if a == nil {
		bad("campaign.recruitment.adjust is missing")
	} else {
		r.Adjust.Max = inRange("campaign.recruitment.adjust.maxPerAttempt", a.MaxPerAttempt, 0, 20)
		r.Adjust.CostPerPoint = inRange("campaign.recruitment.adjust.costPerPoint", a.CostPerPoint, 0, 10)
		switch a.Direction {
		case AdjustUp, AdjustDown, AdjustBoth:
			r.Adjust.Direction = a.Direction
		default:
			bad("campaign.recruitment.adjust.direction: %q is not one of \"up\", \"down\", \"both\"", a.Direction)
		}
		if a.AppliesTo != "" && a.AppliesTo != "die" {
			bad("campaign.recruitment.adjust.appliesTo: %q is not \"die\"", a.AppliesTo)
		}
		r.Adjust.Die = true
	}

	r.Sides = 6
	if d := c.Recruitment.Die; d != nil {
		spec, err := lean.ParseDice(*d)
		switch {
		case err != nil:
			bad("campaign.recruitment.die: %v", err)
		case spec.Count != 1:
			bad("campaign.recruitment.die: %q must be one die", *d)
		default:
			r.Sides = spec.Sides
		}
	}

	if rare := c.Recruitment.Rare; rare == nil || rare.Injured == nil || rare.Dead == nil || rare.Promoted == nil {
		bad("campaign.recruitment.rare needs injuredUnitsCount, deadUnitsCount and promotedUnitsCount")
	} else {
		r.Rare = RareRules{Injured: *rare.Injured, Dead: *rare.Dead, Promoted: *rare.Promoted}
	}

	for _, e := range c.AftermathEffects {
		var dst *PerkRange
		switch e.Key {
		case PerkRendezvous:
			dst = &r.Rendezvous
		case PerkVantagePoint:
			dst = &r.VantagePoint
		default:
			continue
		}
		path := "campaign.aftermathEffects[" + e.Key + "].range"
		switch {
		case e.Range == nil:
			bad("%s is missing", path)
		case e.Range[0] < -6 || e.Range[1] > 6 || e.Range[0] > e.Range[1]:
			bad("%s: [%d, %d] must lie within -6..6 with low <= high", path, e.Range[0], e.Range[1])
		default:
			*dst = PerkRange{e.Range[0], e.Range[1]}
		}
	}
	for _, k := range []struct {
		key string
		r   PerkRange
	}{{PerkRendezvous, r.Rendezvous}, {PerkVantagePoint, r.VantagePoint}} {
		found := false
		for _, e := range c.AftermathEffects {
			found = found || e.Key == k.key
		}
		if !found {
			bad("campaign.aftermathEffects[%s] is missing", k.key)
		}
	}

	if s := c.Company.StartingInfluence; s != nil {
		r.StartingInfluence = inRange("campaign.company.startingInfluence", s, 0, 100)
	}
	if len(problems) > 0 {
		return RecruitRules{}, fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return r, nil
}

// checkChart says whether the rows of a chart cover 1..sides exactly once.
func checkChart(path string, rows []ChartRow, sides int) error {
	seen := make([]int, sides+1)
	for _, row := range rows {
		for n := row.Min; n <= row.Max; n++ {
			if n < 1 || n > sides {
				return fmt.Errorf("%s: row %d-%d is outside 1..%d", path, row.Min, row.Max, sides)
			}
			seen[n]++
		}
	}
	for n := 1; n <= sides; n++ {
		if seen[n] != 1 {
			return fmt.Errorf("%s: %d is covered %d times, expected once (the die is 1..%d)", path, n, seen[n], sides)
		}
	}
	return nil
}

// Describe says the range in words: "+1 or -1", "-2 to +2".
func (p PerkRange) Describe() string {
	switch {
	case p.Low == -p.High && p.High == 1:
		return "+1 or -1"
	case p.Low == p.High:
		return fmt.Sprintf("%+d", p.Low)
	}
	return fmt.Sprintf("%+d to %+d", p.Low, p.High)
}
