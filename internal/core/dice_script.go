package core

import (
	"fmt"
	"sync"
)

// ScriptedDice is the dice of a development server that an end-to-end test scripts (docs/testing.md, "Scripting the server's dice"):
// a FIFO queue of results, each used by the next roll whatever its sides, then the fallback (RandomDice) once the queue is empty.
// A result outside 1..sides when a roll consumes it is a bug of the script, not of the app: the roll panics with a ScriptError, which
// the HTTP recoverer turns into a 500 the test sees, and the value is gone. Safe for concurrent use. A production server never has one.
type ScriptedDice struct {
	fallback Dice
	mu       sync.Mutex
	queue    []int
}

// ScriptError is the panic value of a scripted result that does not fit the die rolled.
type ScriptError struct{ Value, Sides int }

func (e ScriptError) Error() string {
	return fmt.Sprintf("scripted die %d does not fit a d%d (want 1..%d)", e.Value, e.Sides, e.Sides)
}

// NewScriptedDice returns a script with an empty queue over fallback (RandomDice when nil).
func NewScriptedDice(fallback Dice) *ScriptedDice {
	if fallback == nil {
		fallback = RandomDice{}
	}
	return &ScriptedDice{fallback: fallback}
}

// Push appends results to the queue.
func (d *ScriptedDice) Push(values ...int) {
	d.mu.Lock()
	d.queue = append(d.queue, values...)
	d.mu.Unlock()
}

// Clear empties the queue.
func (d *ScriptedDice) Clear() {
	d.mu.Lock()
	d.queue = nil
	d.mu.Unlock()
}

// Len is how many results wait.
func (d *ScriptedDice) Len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.queue)
}

func (d *ScriptedDice) D6() int { return d.Die(6) }

// Die takes the next result of the queue, or rolls the fallback when it is empty. A result outside 1..sides panics (ScriptError).
func (d *ScriptedDice) Die(sides int) int {
	d.mu.Lock()
	if len(d.queue) == 0 {
		d.mu.Unlock()
		return d.fallback.Die(sides)
	}
	v := d.queue[0]
	d.queue = d.queue[1:]
	d.mu.Unlock()
	if v < 1 || v > sides {
		panic(ScriptError{Value: v, Sides: sides})
	}
	return v
}

func (d *ScriptedDice) Roll(spec DiceSpec) int { return sumDice(d, spec) }
