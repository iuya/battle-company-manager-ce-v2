package lean

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// Share is a part of a whole as an exact integer fraction: a limit's most ("1/3" of the company), a keyword's share, a Break Point.
// The zero value (Den 0) is "not written". A share is written as a fraction string "1/3", "1/2", "2/3"; a bare number is a percent
// and is converted to the exact fraction over 100 in lowest terms (50 is 1/2, 66 is 33/50, 33.33 is 3333/10000), which keeps the
// older percent keys working. The core works on the numerator and denominator with integers only.
type Share struct{ Num, Den int }

// MaxShareDen is the largest denominator a share may have; it keeps every product of the core far from overflow.
const MaxShareDen = 1_000_000

// IsSet reports that a share was written.
func (s Share) IsSet() bool { return s.Den > 0 }

// String is the fraction "1/3"; an unset share is "".
func (s Share) String() string {
	if !s.IsSet() {
		return ""
	}
	return strconv.Itoa(s.Num) + "/" + strconv.Itoa(s.Den)
}

// Percent is the share in percent, for messages and comparisons only (the core never calculates with it).
func (s Share) Percent() float64 {
	if !s.IsSet() {
		return 0
	}
	return float64(s.Num) * 100 / float64(s.Den)
}

// WholePercent is the share as a whole number of percent, and whether it is one (1/2 is 50, 33/50 is 66, 1/3 is not).
func (s Share) WholePercent() (int, bool) {
	if !s.IsSet() || (s.Num*100)%s.Den != 0 {
		return 0, false
	}
	return s.Num * 100 / s.Den, true
}

// Of is the share of n models in whole models, rounded up or down, with integers only: (n*num + den - 1)/den up, (n*num)/den down.
func (s Share) Of(n int, roundUp bool) int {
	if !s.IsSet() {
		return 0
	}
	if roundUp {
		return (n*s.Num + s.Den - 1) / s.Den
	}
	return n * s.Num / s.Den
}

// Text is the share as the pages say it: a whole percent as "50%", any other share as its fraction ("1/3").
func (s Share) Text() string {
	if p, ok := s.WholePercent(); ok {
		return strconv.Itoa(p) + "%"
	}
	return s.String()
}

// NewShare makes the share num/den in lowest terms. It fails for a denominator of 0 or over MaxShareDen, a negative numerator or a
// share above 1.
func NewShare(num, den int) (Share, error) {
	switch {
	case den <= 0:
		return Share{}, fmt.Errorf("the denominator of %d/%d must be above 0", num, den)
	case num < 0:
		return Share{}, fmt.Errorf("the numerator of %d/%d must not be negative", num, den)
	case den > MaxShareDen:
		return Share{}, fmt.Errorf("the denominator of %d/%d is above %d", num, den, MaxShareDen)
	case num > den:
		return Share{}, fmt.Errorf("%d/%d is more than the whole; a share is at most 1", num, den)
	}
	g := gcd(num, den)
	return Share{num / g, den / g}, nil
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 1
	}
	return a
}

// ParseShare reads the fraction "n/d" (spaces around the parts allowed).
func ParseShare(s string) (Share, error) {
	n, d, ok := strings.Cut(s, "/")
	if !ok {
		return Share{}, fmt.Errorf(`%q is not a fraction; write it like "1/3"`, s)
	}
	num, e1 := strconv.Atoi(strings.TrimSpace(n))
	den, e2 := strconv.Atoi(strings.TrimSpace(d))
	if e1 != nil || e2 != nil || strings.ContainsAny(n+d, "+-") {
		return Share{}, fmt.Errorf(`%q is not a fraction of whole numbers; write it like "1/3"`, s)
	}
	return NewShare(num, den)
}

// PercentShare converts a percent written as a decimal number (no float involved: "33.33" is 3333/10000) to its exact share.
func PercentShare(text string) (Share, error) {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(text))
	if !ok || r.Sign() < 0 {
		return Share{}, fmt.Errorf("%s is not a percent from 0 to 100", text)
	}
	r.Quo(r, big.NewRat(100, 1))
	if r.Cmp(big.NewRat(1, 1)) > 0 {
		return Share{}, fmt.Errorf("%s is more than 100 percent", text)
	}
	if !r.Denom().IsInt64() || r.Denom().Int64() > MaxShareDen {
		return Share{}, fmt.Errorf("%s has too many decimals to be a share", text)
	}
	return NewShare(int(r.Num().Int64()), int(r.Denom().Int64()))
}

// PercentFloatShare is PercentShare of a float read from a percent key.
func PercentFloatShare(p float64) (Share, error) {
	return PercentShare(strconv.FormatFloat(p, 'f', -1, 64))
}

// nearFractions are the simple fractions an uploaded percent may be a rounding of (denominators 2 to 12).
var nearFractions = func() []Share {
	var out []Share
	for den := 2; den <= 12; den++ {
		for num := 1; num < den; num++ {
			if gcd(num, den) == 1 {
				out = append(out, Share{num, den})
			}
		}
	}
	return out
}()

// NearMiss returns the simple fraction (denominator up to 12) that the share is close to without being equal (33.33 percent is 1/3
// rounded; within 0.1 percentage points), if any.
func (s Share) NearMiss() (Share, bool) {
	if !s.IsSet() {
		return Share{}, false
	}
	for _, f := range nearFractions {
		if f.Num*s.Den == s.Num*f.Den {
			return Share{}, false // it is exactly a simple fraction
		}
	}
	p := s.Percent()
	for _, f := range nearFractions {
		if d := p - f.Percent(); d < 0.1 && d > -0.1 {
			return f, true
		}
	}
	return Share{}, false
}

// BindJSON reads a fraction string ("1/3") or a number, which is a percent.
func (s *Share) BindJSON(v any, at string, rep *Reporter) {
	const want = `expected a fraction string like "1/3" (or a number, a percent)`
	var got Share
	var err error
	switch x := v.(type) {
	case string:
		got, err = ParseShare(x)
	case json.Number:
		got, err = PercentShare(x.String())
	default:
		rep.Errorf(at, "%s, got %s", want, describe(v))
		return
	}
	if err != nil {
		rep.Errorf(at, "%s", err.Error())
		return
	}
	*s = got
}

func (s Share) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }
