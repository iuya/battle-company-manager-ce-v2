package web

import (
	"slices"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func TestPathLook(t *testing.T) {
	for _, c := range []struct{ name, icon, kind, lead, main string }{
		{"Path of the Duelist", "fencer", "melee", "Path of the", "Duelist"},
		{"Path of the Marksman", "arrow-cluster", "ranged", "Path of the", "Marksman"},
		{"Path of the Ranger", "boot-prints", "ranged", "Path of the", "Ranger"},
		{"Path of the Warlord", "crown", "command", "Path of the", "Warlord"},
		{"Path of the Sorcerer", "wizard-staff", "magic", "Path of the", "Sorcerer"},
		{"Path of Their Own Making", "hiking", "other", "Path of", "Their Own Making"},
		{"On Their Own Path", "padlock", "other", "", "On Their Own Path"},
		{"Path of the Smith", "medal", "other", "Path of the", "Smith"}, // another rules file
	} {
		if got := pathLook(c.name); got.Icon != c.icon || got.Kind != c.kind || got.Lead != c.lead || got.Main != c.main {
			t.Errorf("pathLook(%q) = %+v, want %s %s %q %q", c.name, got, c.icon, c.kind, c.lead, c.main)
		}
	}
}

// Only the Ranger and the Duelist are hybrid: an unknown path, or any other of the file, is not.
func TestPathLookHybrid(t *testing.T) {
	var got []string
	for name, l := range pathLooks {
		if l.Hybrid {
			got = append(got, name)
		}
	}
	slices.Sort(got)
	if want := []string{"Path of the Duelist", "Path of the Ranger"}; !slices.Equal(got, want) {
		t.Errorf("hybrid paths = %v, want %v", got, want)
	}
	if !pathLook("Path of the Ranger").Hybrid || pathLook("Path of the Smith").Hybrid {
		t.Error("pathLook carries the flag of the known path only")
	}
}

func cardsNamed(names ...string) []pathCard {
	out := make([]pathCard, len(names))
	for i, n := range names {
		out[i] = pathCard{PathOption: core.PathOption{Name: n}}
	}
	return out
}

func cardNames(cards []pathCard) []string {
	out := make([]string, len(cards))
	for i, c := range cards {
		out[i] = c.Name
	}
	return out
}

// The rows come in the declared order whatever the rules file's order is; the flat list is the rows one after the other.
func TestPathRowsOrder(t *testing.T) {
	var file []string
	for name := range pathLooks {
		if pathLooks[name].Kind != "other" {
			file = append(file, name)
		}
	}
	slices.Sort(file)
	rows, flat := pathRows(cardsNamed(file...))
	want := []struct {
		icon  string
		kind  string
		label string
		paths []string
	}{
		{"horn", "command", "Command paths", []string{"Path of the Warlord", "Path of the Tactician", "Path of the Shieldbearer", "Path of the Sorcerer"}},
		{"sword-brandish", "melee", "Melee paths", []string{"Path of the Soldier", "Path of the Claimant", "Path of the Berserker", "Path of the Duelist"}},
		{"high-shot", "ranged", "Ranged paths", []string{"Path of the Sharpshooter", "Path of the Marksman", "Path of the Ranger"}},
	}
	if len(rows) != len(want) {
		t.Fatalf("%d rows, want %d", len(rows), len(want))
	}
	var all []string
	for i, w := range want {
		if rows[i].Icon != w.icon || rows[i].Kind != w.kind || rows[i].Label != w.label || !slices.Equal(cardNames(rows[i].Cards), w.paths) {
			t.Errorf("row %d = %+v, want %+v", i, rows[i], w)
		}
		all = append(all, w.paths...)
	}
	if got := cardNames(flat); !slices.Equal(got, all) {
		t.Errorf("flat = %v, want %v", got, all)
	}
}

// A path no row names goes in an extra last row without an icon, in the file's order; a row with no paths is left out; a blocked path keeps its place.
func TestPathRowsUnknownAndEmpty(t *testing.T) {
	cards := cardsNamed("Path of the Smith", "Path of the Ranger", "Path of the Cook", "Path of the Sharpshooter")
	cards[3].Blocked = "full"
	rows, flat := pathRows(cards)
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want the ranged row and the extra one", rows)
	}
	if rows[0].Icon != "high-shot" || !slices.Equal(cardNames(rows[0].Cards), []string{"Path of the Sharpshooter", "Path of the Ranger"}) {
		t.Errorf("first row = %+v", rows[0])
	}
	if rows[0].Cards[0].Blocked != "full" {
		t.Error("the blocked path keeps its place and its reason")
	}
	if rows[1].Icon != "" || !slices.Equal(cardNames(rows[1].Cards), []string{"Path of the Smith", "Path of the Cook"}) {
		t.Errorf("extra row = %+v", rows[1])
	}
	if len(flat) != len(cards) {
		t.Errorf("flat has %d paths, want every one of %d", len(flat), len(cards))
	}
}
