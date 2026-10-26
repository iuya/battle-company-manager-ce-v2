package core

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
)

// ToWoundChart is reference.toWound of the glossary file: the rulebook's To Wound Chart (docs/definitions.md, "Reference tables"). Cells[s][d] is the
// roll for Strength Strengths[s] against Defence Defences[d]: "3+".."6+", "6+/4+".."6+/6+" (a 6+ on one dice, then a further dice) or "-" (cannot wound).
type ToWoundChart struct {
	Title        string
	RulebookPage int
	Strengths    []int
	Defences     []int
	Cells        [][]string
	Notation     map[string]string // what a cell form means, by form ("N+", "6+/N+", "-")
	Notes        []string
}

// Roll is the cell for a Strength and a Defence ("" when the chart has no such row or column).
func (c ToWoundChart) Roll(strength, defence int) string {
	s, d := indexOf(c.Strengths, strength), indexOf(c.Defences, defence)
	if s < 0 || d < 0 {
		return ""
	}
	return c.Cells[s][d]
}

func indexOf(list []int, v int) int {
	for i, x := range list {
		if x == v {
			return i
		}
	}
	return -1
}

var toWoundCell = regexp.MustCompile(`^(?:[3-6]\+|6\+/[4-6]\+|-)$`)

// ParseToWound reads reference.toWound of a glossary file. ok is false when the file has none (an older or custom file); a chart that is present but
// malformed (not a complete grid, an unknown cell) is an error wrapping ErrInvalidRuleset.
func ParseToWound(glossary []byte) (chart ToWoundChart, ok bool, err error) {
	var doc struct {
		Reference struct {
			ToWound *struct {
				Title        string            `json:"title"`
				RulebookPage int               `json:"rulebookPage"`
				Strengths    []int             `json:"strengths"`
				Defences     []int             `json:"defences"`
				Cells        [][]string        `json:"cells"`
				Notation     map[string]string `json:"notation"`
				Notes        []string          `json:"notes"`
			} `json:"toWound"`
		} `json:"reference"`
	}
	if err := json.Unmarshal(glossary, &doc); err != nil {
		return ToWoundChart{}, false, invalidToWound("the glossary is not valid JSON")
	}
	tw := doc.Reference.ToWound
	if tw == nil {
		return ToWoundChart{}, false, nil
	}
	if len(tw.Strengths) == 0 || len(tw.Defences) == 0 || len(tw.Cells) != len(tw.Strengths) {
		return ToWoundChart{}, false, invalidToWound("the To Wound chart has no complete grid")
	}
	for i, row := range tw.Cells {
		if len(row) != len(tw.Defences) {
			return ToWoundChart{}, false, invalidToWound("row " + strconv.Itoa(i+1) + " of the To Wound chart is not as long as its Defences")
		}
		for _, c := range row {
			if !toWoundCell.MatchString(c) {
				return ToWoundChart{}, false, invalidToWound("unknown To Wound cell " + strconv.Quote(c))
			}
		}
	}
	return ToWoundChart{Title: tw.Title, RulebookPage: tw.RulebookPage, Strengths: tw.Strengths, Defences: tw.Defences, Cells: tw.Cells, Notation: tw.Notation, Notes: tw.Notes}, true, nil
}

func invalidToWound(msg string) error { return errors.Join(ErrInvalidRuleset, errors.New(msg)) }
