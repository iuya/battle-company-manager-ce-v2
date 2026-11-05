package glossaryref

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ToWoundRows has one row per Strength 1..10, one cell per Defence 1..10. "N+" is one dice
// scoring N or more; "6+/N+" is a 6+ on a dice, then a further dice that must score N+ (two
// individual rolls); "-" cannot wound. Hand-transcribed from the rulebook's chart
// (sources/mesbg_rules.md, "TO WOUND CHART", printed page 44).
var ToWoundRows = []string{
	"4+  5+  5+  6+  6+  6+/4+ 6+/5+ 6+/6+ -     -",
	"4+  4+  5+  5+  6+  6+    6+/4+ 6+/5+ 6+/6+ -",
	"3+  4+  4+  5+  5+  6+    6+    6+/4+ 6+/5+ 6+/6+",
	"3+  3+  4+  4+  5+  5+    6+    6+    6+/4+ 6+/5+",
	"3+  3+  3+  4+  4+  5+    5+    6+    6+    6+/4+",
	"3+  3+  3+  3+  4+  4+    5+    5+    6+    6+",
	"3+  3+  3+  3+  3+  4+    4+    5+    5+    6+",
	"3+  3+  3+  3+  3+  3+    4+    4+    5+    5+",
	"3+  3+  3+  3+  3+  3+    3+    4+    4+    5+",
	"3+  3+  3+  3+  3+  3+    3+    3+    4+    4+",
}

var allowedCells = map[string]bool{"3+": true, "4+": true, "5+": true, "6+": true, "6+/4+": true, "6+/5+": true, "6+/6+": true, "-": true}

// Notation is a JSON object that keeps its key order.
type Notation []NotationEntry

// NotationEntry is one key and value of a Notation.
type NotationEntry struct{ Key, Value string }

// MarshalJSON writes the entries as an object, in order.
func (n Notation) MarshalJSON() ([]byte, error) {
	var b strings.Builder
	b.WriteByte('{')
	for i, e := range n {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := json.Marshal(e.Key)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(e.Value)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}

// ToWoundTable is the "toWound" reference table of the glossary file.
type ToWoundTable struct {
	Title        string     `json:"title"`
	Term         string     `json:"term"`
	RulebookPage int        `json:"rulebookPage"`
	Strengths    []int      `json:"strengths"`
	Defences     []int      `json:"defences"`
	Cells        [][]string `json:"cells"`
	Notation     Notation   `json:"notation"`
	Notes        []string   `json:"notes"`
}

func oneToTen() []int {
	r := make([]int, 10)
	for i := range r {
		r[i] = i + 1
	}
	return r
}

// ToWound returns the To Wound chart as the glossary file carries it.
func ToWound() ToWoundTable {
	cells := make([][]string, len(ToWoundRows))
	for i, r := range ToWoundRows {
		cells[i] = strings.Fields(r)
	}
	return ToWoundTable{
		Title:        "To Wound Chart",
		Term:         "To Wound Chart",
		RulebookPage: 44,
		Strengths:    oneToTen(),
		Defences:     oneToTen(),
		Cells:        cells,
		Notation: Notation{
			{"N+", "roll one dice; N or more wounds"},
			{"6+/N+", "roll one dice and score a 6+, then a further dice that must score N+; both rolls are individual (either can be re-rolled by a rule)"},
			{"-", "cannot wound"},
		},
		Notes: []string{
			"Rows are the attacker's Strength, columns the target's Defence; the cell is the minimum roll to wound.",
			"A Rend uses the nominated model's Strength in place of its Defence.",
			"Strikes against a Trapped model make two To Wound Rolls and apply both.",
		},
	}
}

// CellRank gives the difficulty order: 3+ < 4+ < 5+ < 6+ < 6+/4+ < 6+/5+ < 6+/6+ < -
func CellRank(c string) int {
	if c == "-" {
		return 99
	}
	r := int(c[0]-'0') * 10
	if len(c) > 2 {
		r += int(c[3] - '0')
	}
	return r
}

func cellErrors(axesOK bool, cells [][]string) []string {
	var errs []string
	if !axesOK {
		errs = append(errs, "toWound: strengths and defences must be 1..10")
	}
	bad := len(cells) != 10
	for _, r := range cells {
		if len(r) != 10 {
			bad = true
		}
	}
	if bad {
		return append(errs, "toWound: cells must be a 10x10 grid (rows Strength, columns Defence)")
	}
	seen := map[string]bool{}
	for _, r := range cells {
		for _, c := range r {
			if !allowedCells[c] {
				seen[c] = true
			}
		}
	}
	if len(seen) > 0 {
		var list []string
		for c := range seen {
			list = append(list, c)
		}
		sort.Strings(list)
		return append(errs, fmt.Sprintf("toWound: values outside the allowed set: %v", list))
	}
	for s, row := range cells {
		for d := 1; d < 10; d++ {
			if CellRank(row[d]) < CellRank(row[d-1]) {
				errs = append(errs, fmt.Sprintf("toWound: S%d: D%d (%s) is easier than D%d (%s)", s+1, d+1, row[d], d, row[d-1]))
			}
		}
	}
	for s := 1; s < 10; s++ {
		for d := 0; d < 10; d++ {
			if CellRank(cells[s][d]) > CellRank(cells[s-1][d]) {
				errs = append(errs, fmt.Sprintf("toWound: D%d: S%d (%s) is harder than S%d (%s)", d+1, s+1, cells[s][d], s, cells[s-1][d]))
			}
		}
	}
	return errs
}

// ToWoundErrorsJSON checks a decoded JSON "toWound" object (numbers as float64, lists as
// []any), as found in the glossary file: complete 10x10 grid, allowed values, never easier as
// Defence rises, never harder as Strength rises. It returns nil for a valid table.
func ToWoundErrorsJSON(tw map[string]any) []string {
	axis := func(key string) bool {
		l, ok := tw[key].([]any)
		if !ok || len(l) != 10 {
			return false
		}
		for i, x := range l {
			if f, ok := x.(float64); !ok || f != float64(i+1) {
				return false
			}
		}
		return true
	}
	axesOK := axis("strengths") && axis("defences")
	var cells [][]string
	rows, _ := tw["cells"].([]any)
	for _, r := range rows {
		var row []string
		l, _ := r.([]any)
		for _, c := range l {
			s, ok := c.(string)
			if !ok {
				s = fmt.Sprint(c) // not an allowed value: reported below
			}
			row = append(row, s)
		}
		cells = append(cells, row)
	}
	return cellErrors(axesOK, cells)
}
