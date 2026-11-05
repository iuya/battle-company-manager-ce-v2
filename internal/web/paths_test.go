package web

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func TestPathLook(t *testing.T) {
	for _, c := range []struct {
		name                           string
		categories                     []string
		icon, kind, second, lead, main string
	}{
		{"Path of the Duelist", []string{"melee", "support"}, "fencer", "melee", "support", "Path of the", "Duelist"},
		{"Path of the Marksman", []string{"ranged"}, "arrow-cluster", "ranged", "", "Path of the", "Marksman"},
		{"Path of the Tactician", []string{"support", "will"}, "compass", "support", "will", "Path of the", "Tactician"},
		{"Path of the Sorcerer", []string{"will"}, "wizard-staff", "will", "", "Path of the", "Sorcerer"},
		{"Path of Their Own Making", nil, "hiking", "other", "", "Path of", "Their Own Making"},
		{"On Their Own Path", nil, "padlock", "other", "", "", "On Their Own Path"},
		{"Path of the Smith", nil, "medal", "other", "", "Path of the", "Smith"},                           // another rules file
		{"Path of the Smith", []string{"will", "melee"}, "medal", "will", "melee", "Path of the", "Smith"}, // any pair, on any path
	} {
		got := pathLook(c.name, c.categories)
		if got.Icon != c.icon || got.Kind != c.kind || got.Second != c.second || got.Lead != c.lead || got.Main != c.main {
			t.Errorf("pathLook(%q, %v) = %+v, want %s %s %q %q %q", c.name, c.categories, got, c.icon, c.kind, c.second, c.lead, c.main)
		}
	}
}

// The colours come from the categories alone: the same name with other categories draws those, and any pair splits the badge with no
// code or CSS per path. Every category has both of its classes in the stylesheet source.
func TestPathIconAnyPair(t *testing.T) {
	r, err := newRenderer()
	if err != nil {
		t.Fatal(err)
	}
	css, err := os.ReadFile("../../tailwind.css")
	if err != nil {
		t.Fatal(err)
	}
	icon := func(categories ...string) string {
		var b strings.Builder
		if err := r.pages["styleguide"].ExecuteTemplate(&b, "path_icon", pathLook("Path of the Warlord", categories)); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	categories := []string{"support", "melee", "ranged", "will"}
	for _, main := range categories {
		for _, second := range categories {
			if main == second {
				continue
			}
			if got, want := icon(main, second), fmt.Sprintf(`class="path-icon path-k-%s path-icon-hybrid path-k2-%s"`, main, second); !strings.Contains(got, want) {
				t.Errorf("%s, %s: %s, want %s", main, second, got, want)
			}
		}
		if got, want := icon(main), fmt.Sprintf(`class="path-icon path-k-%s"`, main); !strings.Contains(got, want) {
			t.Errorf("%s: %s, want %s", main, got, want)
		}
		for _, class := range []string{".path-k-" + main + " {", ".path-k2-" + main + " {"} {
			if !strings.Contains(string(css), class) {
				t.Errorf("tailwind.css has no %s", class)
			}
		}
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
	for name := range pathIcons {
		if name != "Path of Their Own Making" && name != "On Their Own Path" {
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
		{"horn", "support", "Command paths", []string{"Path of the Warlord", "Path of the Shieldbearer", "Path of the Tactician", "Path of the Sorcerer"}},
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
