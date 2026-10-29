package core

import (
	"fmt"
	"math/rand/v2"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// DiceSpec is a dice specification [N]dM of the rules file (docs/definitions.md, "Engine parameters"), parsed once at load.
type DiceSpec = lean.DiceSpec

// NdM is the spec of count dice of sides faces (for tests and for code that builds a spec from numbers it has checked).
func NdM(count, sides int) DiceSpec { return DiceSpec{Count: count, Sides: sides} }

// D6Spec is one six-sided die, the spec for the rolls whose rule does not name dice.
var D6Spec = DiceSpec{Count: 1, Sides: 6}

// Dice rolls the dice of the rules. Services take it as a dependency so tests can fix the results.
type Dice interface {
	// D6 returns 1 to 6.
	D6() int
	// Die returns 1 to sides.
	Die(sides int) int
	// Roll returns the sum of spec.Count dice of spec.Sides faces: Min..Max of the spec.
	Roll(spec DiceSpec) int
}

// RollDice rolls the individual dice of a spec, each 1..Sides (the 3D dice, the typed entry of each die).
func RollDice(d Dice, spec DiceSpec) []int {
	out := make([]int, spec.Count)
	for i := range out {
		out[i] = d.Die(spec.Sides)
	}
	return out
}

// RandomDice rolls with the runtime's random source.
type RandomDice struct{}

func (RandomDice) D6() int           { return rand.IntN(6) + 1 }
func (RandomDice) Die(sides int) int { return rand.IntN(sides) + 1 }
func (r RandomDice) Roll(spec DiceSpec) int {
	return sumDice(r, spec)
}

func sumDice(d Dice, spec DiceSpec) int {
	n := 0
	for range spec.Count {
		n += d.Die(spec.Sides)
	}
	return n
}

// FixedDice returns the given results in turn, then repeats the last one. For tests and the seed command. Each result stands for
// one die: a six-sided face, kept as it is for any die of six or more sides (a result above the sides is the test's to avoid) and folded on a d2 or d3.
type FixedDice struct {
	Rolls []int
	n     int
}

func (d *FixedDice) D6() int { return d.Die(6) }

func (d *FixedDice) Die(sides int) int {
	v := 1
	if len(d.Rolls) > 0 {
		v = d.Rolls[min(d.n, len(d.Rolls)-1)]
		d.n++
	}
	if sides < 6 && 6%sides == 0 { // a result is a six-sided face: on a d3 it folds as a typed one does (1-2 reads 1, 3-4 reads 2, 5-6 reads 3)
		v = (v-1)/(6/sides) + 1
	}
	return v
}

func (d *FixedDice) Roll(spec DiceSpec) int { return sumDice(d, spec) }

// ParseDiceAt reads a dice string of the rules file; the error names the path of the key.
func ParseDiceAt(path, s string) (DiceSpec, error) {
	spec, err := lean.ParseDice(s)
	if err != nil {
		return DiceSpec{}, fmt.Errorf("%s: %w", path, err)
	}
	return spec, nil
}

// CheckCovers says whether the ranges [lo, hi] (one per table row) cover every total of the spec exactly once and nothing outside
// it. path names the table in the error.
func CheckCovers(path string, spec DiceSpec, ranges [][2]int) error {
	lo, hi := spec.Min(), spec.Max()
	seen := make([]int, hi+1)
	for _, r := range ranges {
		if r[0] > r[1] || r[0] < lo || r[1] > hi {
			return fmt.Errorf("%s: row %d-%d is outside %d..%d (the dice are %s)", path, r[0], r[1], lo, hi, spec)
		}
		for n := r[0]; n <= r[1]; n++ {
			seen[n]++
		}
	}
	for n := lo; n <= hi; n++ {
		if seen[n] != 1 {
			return fmt.Errorf("%s: %d is covered %d times, expected once (the dice are %s: %d..%d)", path, n, seen[n], spec, lo, hi)
		}
	}
	return nil
}

// TypedMax is the highest number a player types for a roll of the spec that has no physical die of its own (a d3, a d2): they roll a
// six-sided die and the faces are shared out, so 6; any other spec is typed as it is rolled, its total (spec.Max()).
func TypedMax(spec DiceSpec) int {
	if folds(spec) {
		return 6
	}
	return spec.Max()
}

// FromTyped turns a typed number into the roll it stands for: on a d3 a six-sided die of 1-2 reads 1, 3-4 reads 2 and 5-6 reads 3.
// ok is false for a number outside 1..TypedMax (below Min for a sum).
func FromTyped(spec DiceSpec, n int) (roll int, ok bool) {
	if n < spec.Min() || n > TypedMax(spec) {
		return 0, false
	}
	if folds(spec) {
		return (n-1)/(6/spec.Sides) + 1, true
	}
	return n, true
}

func folds(spec DiceSpec) bool { return spec.Count == 1 && spec.Sides < 6 && 6%spec.Sides == 0 }

// sumOf adds up the dice.
func sumOf(dice []int) int {
	n := 0
	for _, d := range dice {
		n += d
	}
	return n
}

// diceAt is ParseDiceAt for a key the file must carry: an empty string is "missing".
func diceAt(path, s string) (DiceSpec, error) {
	if s == "" {
		return DiceSpec{}, fmt.Errorf("%s is missing", path)
	}
	return ParseDiceAt(path, s)
}
