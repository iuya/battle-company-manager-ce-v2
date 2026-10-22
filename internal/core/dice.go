package core

import "math/rand/v2"

// Dice rolls the dice of the rules. Services take it as a dependency so tests can fix the results.
type Dice interface {
	// D6 returns 1 to 6.
	D6() int
}

// RandomDice rolls with the runtime's random source.
type RandomDice struct{}

func (RandomDice) D6() int { return rand.IntN(6) + 1 }

// FixedDice returns the given results in turn, then repeats the last one. For tests and the seed command.
type FixedDice struct {
	Rolls []int
	n     int
}

func (d *FixedDice) D6() int {
	if len(d.Rolls) == 0 {
		return 1
	}
	i := min(d.n, len(d.Rolls)-1)
	d.n++
	return d.Rolls[i]
}
