package core

import (
	"errors"
	"sync"
	"testing"
)

func TestScriptedDiceOrderAndFallback(t *testing.T) {
	d := NewScriptedDice(&FixedDice{Rolls: []int{4}})
	d.Push(3, 5)
	d.Push(2)
	if got := []int{d.Die(6), d.D6(), d.Die(3)}; got[0] != 3 || got[1] != 5 || got[2] != 2 {
		t.Errorf("queued results in order, any die size: %v", got)
	}
	if d.Len() != 0 {
		t.Errorf("Len = %d", d.Len())
	}
	if got := d.Die(6); got != 4 {
		t.Errorf("an empty queue falls back: %d", got)
	}
	d.Push(1, 2, 3)
	d.Clear()
	if d.Len() != 0 || d.Die(6) != 4 {
		t.Error("Clear empties the queue")
	}
}

func TestScriptedDiceRollSumsTheQueue(t *testing.T) {
	d := NewScriptedDice(nil)
	d.Push(6, 5)
	if got := d.Roll(NdM(2, 6)); got != 11 {
		t.Errorf("2d6 = %d", got)
	}
}

func TestScriptedDiceRejectsAResultThatDoesNotFit(t *testing.T) {
	for _, tc := range []struct {
		name     string
		v, sides int
	}{{"above", 4, 3}, {"zero", 0, 6}, {"negative", -1, 6}} {
		t.Run(tc.name, func(t *testing.T) {
			d := NewScriptedDice(nil)
			d.Push(tc.v, 2)
			var err error
			func() {
				defer func() {
					if r := recover(); r != nil {
						err, _ = r.(error)
					}
				}()
				d.Die(tc.sides)
			}()
			var se ScriptError
			if !errors.As(err, &se) || se.Value != tc.v || se.Sides != tc.sides {
				t.Fatalf("want a ScriptError, got %v", err)
			}
			if d.Len() != 1 || d.Die(6) != 2 {
				t.Error("the bad result is consumed, the next one waits")
			}
		})
	}
}

func TestScriptedDiceIsSafeForConcurrentUse(t *testing.T) {
	d := NewScriptedDice(nil)
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(2)
		go func() { defer wg.Done(); d.Push(1, 2, 3) }()
		go func() { defer wg.Done(); d.Die(6); d.Len() }()
	}
	wg.Wait()
}
