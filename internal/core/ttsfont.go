package core

import (
	"regexp"
	"strings"
)

// The width-aware padding of the stat table in the figure descriptions (docs/tabletop-simulator.md, Built: description format).
//
// The tooltip of Tabletop Simulator uses a proportional font, so a column of a text table only lines up when each cell is padded to the
// same pixel width. The widths below were MEASURED in the game by the owner (pixels); to retune, change the numbers and nothing else:
// the padding, the descriptions and the test that checks the alignment follow.

// ttsBoldWidth is the width of the bold characters used by the stat names.
var ttsBoldWidth = map[rune]int{'M': 16, 'v': 10, 'F': 10, 'V': 12, 'S': 10, 'D': 13, 'A': 12, 'W': 17, 'C': 10, 'I': 5}

// ttsRegularWidth is the width of the regular characters used by the values: digits, plus, inch mark, space and the bar.
var ttsRegularWidth = map[rune]int{'+': 10, '"': 7, ' ': 5, '|': 6,
	'0': 11, '1': 11, '2': 11, '3': 11, '4': 11, '5': 11, '6': 11, '7': 11, '8': 11, '9': 11}

// Widths of a character that is not measured, and of the padding space.
const (
	ttsFallbackRegular = 8
	ttsFallbackBold    = 12
	ttsPadSpace        = 5 // the ordinary space, the unit of the padding
)

// ttsTags are the colour codes of the rich text; they have no width.
var ttsTags = regexp.MustCompile(`\[(?:[0-9A-Fa-f]{6}|-)\]`)

// ttsTextWidth is the width in pixels of s in the tooltip font. Colour codes count zero.
func ttsTextWidth(s string, bold bool) int {
	table, fallback := ttsRegularWidth, ttsFallbackRegular
	if bold {
		table, fallback = ttsBoldWidth, ttsFallbackBold
	}
	w := 0
	for _, r := range ttsTags.ReplaceAllString(s, "") {
		if cw, ok := table[r]; ok {
			w += cw
		} else {
			w += fallback
		}
	}
	return w
}

// ttsPad is the number of ordinary spaces that brings a cell of cellWidth to width (never negative).
func ttsPad(width, cellWidth int) int {
	d := width - cellWidth
	if d <= 0 {
		return 0
	}
	return (d + ttsPadSpace/2) / ttsPadSpace // round(d / 5)
}

// ttsTableSep separates the cells of the stat table.
const ttsTableSep = " | "

// ttsTable lays out two rows: the bold names and the values (which may carry colour codes). Each column is as wide as its wider cell, and
// each cell is padded with spaces so the bars line up. The padding of a cell is round(missing width / 5) as ttsPad says, but the missing
// width is measured against where the bar of the column SHOULD be (the sum of the column widths) for the names, and against the bar of
// the names for the values, not against the cell alone, so the rounding errors of the earlier cells do not add up along the row and the
// two rows never differ by more than half a space (2.5 px). The last cell is not padded (nothing follows it).
func ttsTable(names, values []string) (string, string) {
	sepW := ttsTextWidth(ttsTableSep, false)
	var r1, r2 []string
	target := 0    // where the bar of this column should be, in pixels
	x1, x2 := 0, 0 // where each row actually is
	for i := range names {
		nw, vw := ttsTextWidth(names[i], true), ttsTextWidth(values[i], false)
		target += max(nw, vw)
		c1, c2 := "[b]"+names[i]+"[/b]", values[i]
		x1, x2 = x1+nw, x2+vw
		if i < len(names)-1 {
			p1 := ttsPad(target, x1)
			p2 := ttsPad(x1+p1*ttsPadSpace, x2) // the values follow the names, so the bars of the two rows differ by 2.5 px at most
			c1, c2 = c1+strings.Repeat(" ", p1), c2+strings.Repeat(" ", p2)
			x1, x2 = x1+p1*ttsPadSpace, x2+p2*ttsPadSpace
		}
		target += sepW
		x1, x2 = x1+sepW, x2+sepW
		r1, r2 = append(r1, c1), append(r2, c2)
	}
	return strings.Join(r1, ttsTableSep), strings.Join(r2, ttsTableSep)
}

// ttsBarPositions are the pixel offsets of the bars of a table row as the model measures them (for the alignment test).
func ttsBarPositions(row string) []int {
	var out []int
	x := 0
	for _, part := range strings.SplitAfter(row, "|") {
		if !strings.HasSuffix(part, "|") {
			break
		}
		x += ttsRichWidth(part[:len(part)-1])
		out = append(out, x)
		x += ttsRegularWidth['|']
	}
	return out
}

// ttsRichWidth is the width of a text with [b]..[/b] spans and colour codes.
func ttsRichWidth(s string) int {
	w := 0
	for s != "" {
		i := strings.Index(s, "[b]")
		if i < 0 {
			return w + ttsTextWidth(s, false)
		}
		w += ttsTextWidth(s[:i], false)
		s = s[i+3:]
		j := strings.Index(s, "[/b]")
		if j < 0 {
			return w + ttsTextWidth(s, true)
		}
		w += ttsTextWidth(s[:j], true)
		s = s[j+4:]
	}
	return w
}
