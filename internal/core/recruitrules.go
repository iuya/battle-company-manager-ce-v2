package core

import (
	"fmt"
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
