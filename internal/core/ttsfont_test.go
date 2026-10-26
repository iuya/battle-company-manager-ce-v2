package core

import (
	"strings"
	"testing"
)

func TestTTSTextWidth(t *testing.T) {
	for _, c := range []struct {
		s    string
		bold bool
		want int
	}{
		{"Mv", true, 26}, {"FV", true, 22}, {"I", true, 5},
		{`6"`, false, 18}, {"4+", false, 21}, {"10", false, 22},
		{"[44BA26]5+[-]", false, 21},     // colour codes have no width
		{"x", false, 8}, {"x", true, 12}, // not measured: the fallbacks
		{" | ", false, 16},
	} {
		if got := ttsTextWidth(c.s, c.bold); got != c.want {
			t.Errorf("width(%q, bold %v) = %d, want %d", c.s, c.bold, got, c.want)
		}
	}
}

func TestTTSPad(t *testing.T) {
	for _, c := range []struct{ width, cell, want int }{
		{26, 26, 0}, {26, 18, 2}, {26, 21, 1}, {26, 22, 1}, {26, 23, 1}, {26, 24, 0}, {20, 30, 0}, {12, 10, 0}, {13, 10, 1},
	} {
		if got := ttsPad(c.width, c.cell); got != c.want {
			t.Errorf("pad(%d, %d) = %d, want %d", c.width, c.cell, got, c.want)
		}
	}
}

func TestTTSTableLayout(t *testing.T) {
	r1, r2 := ttsTable([]string{"Mv", "FV", "I"}, []string{`6"`, "[44BA26]4[-]", "10+"})
	if r1 != "[b]Mv[/b] | [b]FV[/b] | [b]I[/b]" || r2 != `6"   | [44BA26]4[-]   | 10+` {
		t.Errorf("rows:\n%q\n%q", r1, r2)
	}
}

// TestTTSTableAlignsForAllCompanies: in every stat table of every unit, creature and mount of the 37 companies the bars of the two rows
// are within 3 px of each other by the measured model (the precision the owner measured in the game).
func TestTTSTableAlignsForAllCompanies(t *testing.T) {
	rs := shippedRules(t)
	const tolerance = 3
	tables, worst := 0, 0
	for _, bcCo := range ttsCompanies(rs) {
		for _, e := range ttsEntries(rs, bcCo) {
			d := ttsOne(rs, bcCo, e)
			ex := rs.TTSList(d, TTSOptions{FootVersions: true, MountFigures: true})
			for _, f := range rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesNone}).Figures {
				lines := strings.Split(f.Desc, "\n")
				for i, l := range lines {
					if !strings.HasPrefix(l, "[b]Mv[/b]") || i+1 >= len(lines) {
						continue
					}
					a, b := ttsBarPositions(l), ttsBarPositions(lines[i+1])
					if len(a) != len(b) || len(a) == 0 {
						t.Errorf("%s %s: bars %v / %v in\n%s\n%s", bcCo.Name, e.unit, a, b, l, lines[i+1])
						continue
					}
					tables++
					for k := range a {
						diff := a[k] - b[k]
						if diff < 0 {
							diff = -diff
						}
						worst = max(worst, diff)
						if diff > tolerance {
							t.Errorf("%s %s: bar %d is %d px off\n%s\n%s", bcCo.Name, e.unit, k, diff, l, lines[i+1])
						}
					}
				}
			}
		}
	}
	if tables < 100 {
		t.Errorf("only %d tables checked", tables)
	}
	t.Logf("%d tables checked, worst bar offset %d px", tables, worst)
}
