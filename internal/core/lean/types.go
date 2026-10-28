package lean

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// The polymorphic value types (design 3.1, 4.1, 4.3, 4.4). Each reads its forms in BindJSON and writes a canonical form in
// MarshalJSON.

// Roll is the roll of a chart row: a number, a range "lo-hi" or a pair [lo, hi]. An empty range ("3-2", Lo > Hi) is bound as
// written and reported by the validation, which can name the row.
type Roll struct{ Lo, Hi int }

// Covers reports whether the total is inside the roll.
func (r Roll) Covers(n int) bool { return n >= r.Lo && n <= r.Hi }

// Empty reports a range with Lo above Hi.
func (r Roll) Empty() bool { return r.Lo > r.Hi }

// String is `3` or `2-3`, the form the messages quote.
func (r Roll) String() string {
	if r.Lo == r.Hi {
		return strconv.Itoa(r.Lo)
	}
	return fmt.Sprintf("%d-%d", r.Lo, r.Hi)
}

const maxRollValue = 1000

func (r *Roll) BindJSON(v any, at string, rep *Reporter) {
	const want = `expected a number, a range like "2-3" or a pair [lo, hi]`
	num := func(x any) (int, bool) {
		n, ok := x.(json.Number)
		if !ok {
			return 0, false
		}
		i, err := strconv.Atoi(n.String())
		if err != nil || i < 0 || i > maxRollValue {
			return 0, false
		}
		return i, true
	}
	switch x := v.(type) {
	case json.Number:
		n, ok := num(x)
		if !ok {
			rep.Errorf(at, "%s is not a roll; expected a whole number 0..%d", x, maxRollValue)
			return
		}
		*r = Roll{n, n}
	case string:
		lo, hi, found := strings.Cut(strings.TrimSpace(x), "-")
		a, ea := strconv.Atoi(strings.TrimSpace(lo))
		b, eb := a, error(nil)
		if found {
			b, eb = strconv.Atoi(strings.TrimSpace(hi))
		}
		if ea != nil || eb != nil || a < 0 || b < 0 || a > maxRollValue || b > maxRollValue {
			rep.Errorf(at, "%q is not a roll; %s", x, want)
			return
		}
		*r = Roll{a, b}
	case []any:
		if len(x) != 2 {
			rep.Errorf(at, "a roll pair needs 2 numbers, has %d; %s", len(x), want)
			return
		}
		a, oka := num(x[0])
		b, okb := num(x[1])
		if !oka || !okb {
			rep.Errorf(at, "a roll pair holds two whole numbers 0..%d; %s", maxRollValue, want)
			return
		}
		*r = Roll{a, b}
	default:
		rep.Errorf(at, "%s, got %s", want, describe(v))
	}
}

func (r Roll) MarshalJSON() ([]byte, error) {
	if r.Lo == r.Hi {
		return json.Marshal(r.Lo)
	}
	return json.Marshal(r.String())
}

// Rating is the rating of an item: a number for warrior and hero alike, or {warrior, hero}.
type Rating struct{ Warrior, Hero int }

func (g *Rating) BindJSON(v any, at string, rep *Reporter) {
	f := &field{min: ptr(0.0), max: ptr(1000.0)}
	switch x := v.(type) {
	case json.Number:
		var n int
		bindValue(reflectInt(&n), x, at, rep, f)
		*g = Rating{n, n}
	case map[string]any:
		var o struct {
			Warrior int `json:"warrior" required:"true" min:"0" max:"1000"`
			Hero    int `json:"hero" required:"true" min:"0" max:"1000"`
		}
		BindValue(&o, x, at, rep)
		*g = Rating{o.Warrior, o.Hero}
	default:
		rep.Errorf(at, "expected a number or {warrior, hero}, got %s", describe(v))
	}
}

func (g Rating) MarshalJSON() ([]byte, error) {
	if g.Warrior == g.Hero {
		return json.Marshal(g.Warrior)
	}
	return json.Marshal(map[string]int{"warrior": g.Warrior, "hero": g.Hero})
}

// RuleRef names a rule or defines it inline (design 4.4, 4.1): a string, or a rule object that the loader hoists.
type RuleRef struct {
	Name string
	Def  *Rule
}

func (g *RuleRef) BindJSON(v any, at string, rep *Reporter) {
	switch x := v.(type) {
	case string:
		if strings.TrimSpace(x) == "" {
			rep.Errorf(at, "must not be empty")
			return
		}
		*g = RuleRef{Name: x}
	case map[string]any:
		var def Rule
		BindValue(&def, x, at, rep)
		*g = RuleRef{Name: def.Name, Def: &def}
	default:
		rep.Errorf(at, "expected a rule name or a rule definition object, got %s", describe(v))
	}
}

func (g RuleRef) MarshalJSON() ([]byte, error) {
	if g.Def == nil {
		return json.Marshal(g.Name)
	}
	return json.Marshal(encodeTree(reflectOf(g.Def)))
}

// Scalar is a string or a number read as text (the rulebook version "1.1" or 1.1).
type Scalar string

func (s *Scalar) BindJSON(v any, at string, rep *Reporter) {
	switch x := v.(type) {
	case string:
		if x == "" {
			rep.Errorf(at, "must not be empty")
			return
		}
		*s = Scalar(x)
	case json.Number:
		*s = Scalar(x.String())
	default:
		rep.Errorf(at, "expected a string or a number, got %s", describe(v))
	}
}

func (s Scalar) MarshalJSON() ([]byte, error) { return json.Marshal(string(s)) }

// BindJSON reads a dice spec string; the error names the reason (design 3.1).
func (d *DiceSpec) BindJSON(v any, at string, rep *Reporter) {
	s, ok := v.(string)
	if !ok {
		rep.Errorf(at, `expected a dice spec string like "d6" or "2d6", got %s`, describe(v))
		return
	}
	got, err := ParseDice(s)
	if err != nil {
		rep.Errorf(at, "%s", err.Error())
		return
	}
	*d = got
}

func (d DiceSpec) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

func ptr[T any](v T) *T { return &v }
