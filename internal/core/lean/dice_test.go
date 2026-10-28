package lean

import (
	"strings"
	"testing"
)

func TestParseDice(t *testing.T) {
	valid := []struct {
		in       string
		want     DiceSpec
		min, max int
		canon    string
	}{
		{"d6", DiceSpec{1, 6}, 1, 6, "d6"},
		{"D6", DiceSpec{1, 6}, 1, 6, "d6"},
		{"1d6", DiceSpec{1, 6}, 1, 6, "d6"},
		{"2d6", DiceSpec{2, 6}, 2, 12, "2d6"},
		{"2D6", DiceSpec{2, 6}, 2, 12, "2d6"},
		{"10d100", DiceSpec{10, 100}, 10, 1000, "10d100"},
		{"d2", DiceSpec{1, 2}, 1, 2, "d2"},
		{"d3", DiceSpec{1, 3}, 1, 3, "d3"},
		{"010d06", DiceSpec{10, 6}, 10, 60, "10d6"},
	}
	for _, c := range valid {
		t.Run(c.in, func(t *testing.T) {
			got, err := ParseDice(c.in)
			if err != nil {
				t.Fatalf("ParseDice(%q): %v", c.in, err)
			}
			if got != c.want || got.Min() != c.min || got.Max() != c.max || got.String() != c.canon || !got.Valid() {
				t.Fatalf("ParseDice(%q) = %+v min %d max %d %q", c.in, got, got.Min(), got.Max(), got.String())
			}
			back, err := ParseDice(got.String())
			if err != nil || back != got {
				t.Fatalf("round trip of %q: %+v, %v", got.String(), back, err)
			}
		})
	}
	invalid := []struct{ in, reason string }{
		{"0d6", "count 0"},
		{"11d6", "count 11"},
		{"d1", "sides 1"},
		{"d101", "sides 101"},
		{"2d6+1", "not a dice spec"},
		{"d66", "d66 (concatenated dice)"},
		{"2D66", "d66 (concatenated dice)"},
		{" d6", "not a dice spec"},
		{"d6 ", "not a dice spec"},
		{"2", "not a dice spec"},
		{"d", "not a dice spec"},
		{"1.5d6", "not a dice spec"},
		{"", "not a dice spec"},
		{"2d6x", "not a dice spec"},
		{"d6d6", "not a dice spec"},
		{"-1d6", "not a dice spec"},
		{"2d-6", "not a dice spec"},
		{"99999999999d6", "count"},
		{"d99999999999", "sides"},
	}
	for _, c := range invalid {
		t.Run("invalid "+c.in, func(t *testing.T) {
			got, err := ParseDice(c.in)
			if err == nil {
				t.Fatalf("ParseDice(%q) = %+v, want an error", c.in, got)
			}
			if !contains(err.Error(), c.reason) {
				t.Fatalf("error %q lacks %q", err, c.reason)
			}
		})
	}
}

func TestDiceErrorMessages(t *testing.T) {
	for in, want := range map[string]string{
		"2d6x": `"2d6x" is not a dice spec; expected [N]dM, N 1..10, M 2..100`,
		"11d6": `"11d6": count 11 is outside 1..10`,
		"d101": `"d101": sides 101 is outside 2..100`,
	} {
		if _, err := ParseDice(in); err == nil || err.Error() != want {
			t.Errorf("ParseDice(%q) error %v, want %q", in, err, want)
		}
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func FuzzParseDice(f *testing.F) {
	for _, s := range []string{"d6", "2d6", "10d100", "", "d", "0d6", "11d6", "2d6+1", "d66", " d6", "9999999999999d6", "\x00", "dD"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		d, err := ParseDice(s)
		if err != nil {
			return
		}
		if !d.Valid() {
			t.Fatalf("%q parsed to an invalid spec %+v", s, d)
		}
		back, err := ParseDice(d.String())
		if err != nil || back != d {
			t.Fatalf("%q: %+v does not round trip: %+v, %v", s, d, back, err)
		}
	})
}
