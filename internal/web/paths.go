package web

import (
	"cmp"
	"slices"
	"strings"
)

// pathLookView is how a hero's path is drawn: its game-icons.net icon (see static/img/CREDITS.md), the colours of its categories and
// its name cut in two, the "Path of the" lead in small and the rest in large.
type pathLookView struct {
	Icon   string
	Kind   string // the main category, which colours the icon, the box and the name: support, melee, ranged, will, or other without one
	Second string // the secondary category, the other half of the icon; empty when the path has one category or none
	Lead   string // "Path of the", "Path of" or empty
	Main   string
}

// pathIcons are the icons of the paths of the Battle Companies file. Only the icon goes by the name: the colours come from the categories.
var pathIcons = map[string]string{
	"Path of the Soldier":      "sword-clash",
	"Path of the Claimant":     "crowned-skull",
	"Path of the Berserker":    "enrage",
	"Path of the Duelist":      "fencer",
	"Path of the Sharpshooter": "bullseye",
	"Path of the Marksman":     "arrow-cluster",
	"Path of the Ranger":       "boot-prints",
	"Path of the Tactician":    "compass",
	"Path of the Warlord":      "crown",
	"Path of the Shieldbearer": "checked-shield",
	"Path of the Sorcerer":     "wizard-staff",
	"Path of Their Own Making": "hiking",
	"On Their Own Path":        "padlock",
}

// pathLook is the look of a path from its name and the categories the rules file gives it, the main one first. A path the app does not
// know (another rules file) gets the medal; a path without categories the muted colour.
func pathLook(name string, categories []string) pathLookView {
	l := pathLookView{Icon: "medal", Kind: "other", Main: name}
	if icon, ok := pathIcons[name]; ok {
		l.Icon = icon
	}
	if len(categories) > 0 {
		l.Kind = categories[0]
	}
	if len(categories) > 1 {
		l.Second = categories[1]
	}
	for _, lead := range []string{"Path of the ", "Path of "} {
		if rest, ok := strings.CutPrefix(name, lead); ok && rest != "" {
			l.Lead, l.Main = strings.TrimSpace(lead), rest
			break
		}
	}
	return l
}

// pathStripRow is one row of the path window: the heading icon (none for the extra row), the category that colours it and its paths.
type pathStripRow struct {
	Label string
	Icon  string
	Kind  string
	Cards []pathCard
}

// pathRowList is the path window, row by row. A path goes in the row of its main category, which the rules file gives; Paths only sorts a
// row, with any other path after the named ones. A will path goes in the first row, so the Sorcerer ends it.
var pathRowList = []struct {
	Label, Icon, Kind string
	Categories        []string
	Paths             []string
}{
	{"Command paths", "horn", "support", []string{"support", "will"}, []string{"Path of the Warlord", "Path of the Shieldbearer", "Path of the Tactician", "Path of the Sorcerer"}},
	{"Melee paths", "sword-brandish", "melee", []string{"melee"}, []string{"Path of the Soldier", "Path of the Claimant", "Path of the Berserker", "Path of the Duelist"}},
	{"Ranged paths", "high-shot", "ranged", []string{"ranged"}, []string{"Path of the Sharpshooter", "Path of the Marksman", "Path of the Ranger"}},
}

// pathRowOf is the index in pathRowList of the row of a path; len(pathRowList), the extra row, for one with no category or a main category
// no row takes.
func pathRowOf(c pathCard) int {
	if len(c.Categories) > 0 {
		for i, def := range pathRowList {
			if slices.Contains(def.Categories, c.Categories[0]) {
				return i
			}
		}
	}
	return len(pathRowList)
}

// pathRows puts the paths offered in the rows of the path window and returns them with the same cards as one flat list, the order of
// the radios and of the stack (pathCarousel pairs them by index). A row with no paths is left out; a path no row takes goes in an extra
// last row without a heading icon, in the file's order, so none is lost.
func pathRows(cards []pathCard) ([]pathStripRow, []pathCard) {
	grouped := make([][]pathCard, len(pathRowList)+1)
	for _, c := range cards {
		i := pathRowOf(c)
		grouped[i] = append(grouped[i], c)
	}
	rows := make([]pathStripRow, 0, len(grouped))
	flat := make([]pathCard, 0, len(cards))
	for i, def := range pathRowList {
		rank := func(c pathCard) int {
			if n := slices.Index(def.Paths, c.Name); n >= 0 {
				return n
			}
			return len(def.Paths)
		}
		slices.SortStableFunc(grouped[i], func(a, b pathCard) int { return cmp.Compare(rank(a), rank(b)) })
		if len(grouped[i]) > 0 {
			rows = append(rows, pathStripRow{Label: def.Label, Icon: def.Icon, Kind: def.Kind, Cards: grouped[i]})
			flat = append(flat, grouped[i]...)
		}
	}
	if extra := grouped[len(pathRowList)]; len(extra) > 0 {
		rows = append(rows, pathStripRow{Label: "Other paths", Kind: "other", Cards: extra})
		flat = append(flat, extra...)
	}
	return rows, flat
}
