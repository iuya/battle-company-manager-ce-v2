package web

import "strings"

// pathLookView is how a hero's path is drawn: its game-icons.net icon (see static/img/CREDITS.md), the kind of hero it makes
// (which colours the icon and the box) and its name cut in two, the "Path of the" lead in small and the rest in large.
type pathLookView struct {
	Icon string
	Kind string // melee, ranged, command, magic or other
	Lead string // "Path of the", "Path of" or empty
	Main string
	// Hybrid paths are the Ranger and the Duelist: their badge also carries the command colour. It is the owner's reading of what the
	// path is useful for, not a rules concept, so it lives here and not in the rules file. The Duelist specialises in winning duels,
	// dealing far less damage than the rest of its row so that its companions can hit.
	Hybrid bool
}

// pathLooks are the paths of the Battle Companies file. Fighters are melee; archers and rangers ranged; those who lead or
// guard the company are command; the Sorcerer is magic; the paths that are not chosen are other.
var pathLooks = map[string]pathLookView{
	"Path of the Soldier":      {Icon: "sword-clash", Kind: "melee"},
	"Path of the Claimant":     {Icon: "crowned-skull", Kind: "melee"},
	"Path of the Berserker":    {Icon: "enrage", Kind: "melee"},
	"Path of the Duelist":      {Icon: "fencer", Kind: "melee", Hybrid: true},
	"Path of the Sharpshooter": {Icon: "bullseye", Kind: "ranged"},
	"Path of the Marksman":     {Icon: "arrow-cluster", Kind: "ranged"},
	"Path of the Ranger":       {Icon: "boot-prints", Kind: "ranged", Hybrid: true},
	"Path of the Tactician":    {Icon: "compass", Kind: "command"},
	"Path of the Warlord":      {Icon: "crown", Kind: "command"},
	"Path of the Shieldbearer": {Icon: "checked-shield", Kind: "command"},
	"Path of the Sorcerer":     {Icon: "wizard-staff", Kind: "magic"},
	"Path of Their Own Making": {Icon: "hiking", Kind: "other"},
	"On Their Own Path":        {Icon: "padlock", Kind: "other"},
}

// pathLook is the look of a path by its name; a path the app does not know (another rules file) gets the medal.
func pathLook(name string) pathLookView {
	l, ok := pathLooks[name]
	if !ok {
		l = pathLookView{Icon: "medal", Kind: "other"}
	}
	l.Main = name
	for _, lead := range []string{"Path of the ", "Path of "} {
		if rest, ok := strings.CutPrefix(name, lead); ok && rest != "" {
			l.Lead, l.Main = strings.TrimSpace(lead), rest
			break
		}
	}
	return l
}

// pathStripRow is one row of the path window: the heading icon (none for the extra row), the kind that colours it and its paths.
type pathStripRow struct {
	Label string
	Icon  string
	Kind  string
	Cards []pathCard
}

// pathRowList is the order of the path window, row by row. The Sorcerer keeps its blue and goes last of the first row.
var pathRowList = []struct {
	Label, Icon, Kind string
	Paths             []string
}{
	{"Command paths", "horn", "command", []string{"Path of the Warlord", "Path of the Tactician", "Path of the Shieldbearer", "Path of the Sorcerer"}},
	{"Melee paths", "sword-brandish", "melee", []string{"Path of the Soldier", "Path of the Claimant", "Path of the Berserker", "Path of the Duelist"}},
	{"Ranged paths", "high-shot", "ranged", []string{"Path of the Sharpshooter", "Path of the Marksman", "Path of the Ranger"}},
}

// pathRows puts the paths offered in the rows of the path window and returns them with the same cards as one flat list, the order of
// the radios and of the stack (pathCarousel pairs them by index). A row with no paths is left out; a path no row names (another rules
// file) goes in an extra last row without a heading icon, so none is lost.
func pathRows(cards []pathCard) ([]pathStripRow, []pathCard) {
	byName := make(map[string]pathCard, len(cards))
	for _, c := range cards {
		byName[c.Name] = c
	}
	rows := make([]pathStripRow, 0, len(pathRowList)+1)
	flat := make([]pathCard, 0, len(cards))
	for _, def := range pathRowList {
		row := pathStripRow{Label: def.Label, Icon: def.Icon, Kind: def.Kind}
		for _, name := range def.Paths {
			if c, ok := byName[name]; ok {
				row.Cards = append(row.Cards, c)
				delete(byName, name)
			}
		}
		if len(row.Cards) > 0 {
			rows = append(rows, row)
			flat = append(flat, row.Cards...)
		}
	}
	extra := pathStripRow{Label: "Other paths", Kind: "other"}
	for _, c := range cards {
		if _, ok := byName[c.Name]; ok {
			extra.Cards = append(extra.Cards, c)
		}
	}
	if len(extra.Cards) > 0 {
		rows = append(rows, extra)
		flat = append(flat, extra.Cards...)
	}
	return rows, flat
}
