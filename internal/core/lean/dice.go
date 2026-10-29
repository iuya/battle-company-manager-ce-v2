package lean

import (
	"fmt"
	"strconv"
	"strings"
)

// Limits of a dice specification.
const (
	MaxDiceCount = 10
	MinDiceSides = 2
	MaxDiceSides = 100
)

// DiceSpec is a dice specification [N]dM: Count dice of Sides faces. The zero value is invalid.
type DiceSpec struct{ Count, Sides int }

// ParseDice reads "d6", "D6", "2d6", "10d100". No spaces, no modifier, no concatenated dice: "d66" is refused (a 66-sided die is not a thing, the rulebook means two dice).
func ParseDice(s string) (DiceSpec, error) {
	bad := func(format string, a ...any) (DiceSpec, error) {
		return DiceSpec{}, fmt.Errorf(format, a...)
	}
	notSpec := fmt.Errorf("%q is not a dice spec; expected [N]dM, N 1..%d, M %d..%d", s, MaxDiceCount, MinDiceSides, MaxDiceSides)
	i := strings.IndexAny(s, "dD")
	if i < 0 || strings.ContainsAny(s[i+1:], "dD") {
		return DiceSpec{}, notSpec
	}
	count, sides := 1, 0
	if i > 0 {
		n, ok := digits(s[:i])
		if !ok {
			return DiceSpec{}, notSpec
		}
		count = n
	}
	n, ok := digits(s[i+1:])
	if !ok {
		return DiceSpec{}, notSpec
	}
	sides = n
	if count < 1 || count > MaxDiceCount {
		return bad("%q: count %d is outside 1..%d", s, count, MaxDiceCount)
	}
	if sides == 66 { // the rulebook's d66 (two dice read as tens and units) is not a die with 66 faces
		return bad("%q: d66 (concatenated dice) is not supported", s)
	}
	if sides < MinDiceSides || sides > MaxDiceSides {
		return bad("%q: sides %d is outside %d..%d", s, sides, MinDiceSides, MaxDiceSides)
	}
	return DiceSpec{Count: count, Sides: sides}, nil
}

// digits parses a non-empty run of ASCII digits no longer than 6 (longer is an out-of-range number anyway).
func digits(s string) (int, bool) {
	if s == "" || len(s) > 6 {
		if s != "" && len(s) > 6 && strings.Trim(s, "0123456789") == "" {
			return 1 << 30, true // a number too large: reported by the range check
		}
		return 0, false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

// Valid reports whether the spec is inside the limits.
func (d DiceSpec) Valid() bool {
	return d.Count >= 1 && d.Count <= MaxDiceCount && d.Sides >= MinDiceSides && d.Sides <= MaxDiceSides
}

// Min is the lowest total (every die shows 1).
func (d DiceSpec) Min() int { return d.Count }

// Max is the highest total.
func (d DiceSpec) Max() int { return d.Count * d.Sides }

// String is the canonical lower case form, "d6" for a single die; it parses back to an equal spec.
func (d DiceSpec) String() string {
	if d.Count == 1 {
		return "d" + strconv.Itoa(d.Sides)
	}
	return strconv.Itoa(d.Count) + "d" + strconv.Itoa(d.Sides)
}

// Display is the form the pages print: "D6", "2D6" (the canonical form in capitals).
func (d DiceSpec) Display() string { return strings.ToUpper(d.String()) }

// Sum is the lowest and highest total as "2-12", for a header over a table keyed by the roll.
func (d DiceSpec) Sum() string { return strconv.Itoa(d.Min()) + "-" + strconv.Itoa(d.Max()) }
