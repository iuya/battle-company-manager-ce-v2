package web

import (
	"encoding/json"
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

// shippedPathCards are the named paths with the categories the Battle Companies file gives them.
func shippedPathCards(t *testing.T, names ...string) []pathCard {
	t.Helper()
	data, err := os.ReadFile("../../definitions/battle_companies/battle_companies.json")
	if err != nil {
		t.Fatal(err)
	}
	var file any
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	categories := map[string][]string{}
	var walk func(any)
	walk = func(v any) {
		switch v := v.(type) {
		case []any:
			for _, e := range v {
				walk(e)
			}
		case map[string]any:
			if name, ok := v["name"].(string); ok {
				if list, ok := v["categories"].([]any); ok {
					for _, c := range list {
						categories[name] = append(categories[name], c.(string))
					}
				}
			}
			for _, e := range v {
				walk(e)
			}
		}
	}
	walk(file)
	cards := cardsNamed(names...)
	for i := range cards {
		cards[i].Categories = categories[cards[i].Name]
		if len(cards[i].Categories) == 0 {
			t.Errorf("the rules file gives %s no category", cards[i].Name)
		}
	}
	return cards
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
	rows, flat := pathRows(shippedPathCards(t, file...))
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

// A path of another rules file goes in the row of its main category, whatever its name and its secondary category, after the named paths.
// The order of the file holds among the paths the list does not name.
func TestPathRowsGoByTheMainCategory(t *testing.T) {
	cards := cardsNamed("Path of the Smith", "Path of the Duelist", "Path of the Cook", "Path of the Seer", "Path of the Warlord", "Path of the Scout")
	for i, categories := range [][]string{{"melee", "ranged"}, {"melee"}, {"melee"}, {"will"}, {"support"}, {"ranged", "melee"}} {
		cards[i].Categories = categories
	}
	rows, flat := pathRows(cards)
	want := [][]string{
		{"Path of the Warlord", "Path of the Seer"},
		{"Path of the Duelist", "Path of the Smith", "Path of the Cook"},
		{"Path of the Scout"},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %+v, want %d", rows, len(want))
	}
	for i, w := range want {
		if got := cardNames(rows[i].Cards); !slices.Equal(got, w) {
			t.Errorf("row %d = %v, want %v", i, got, w)
		}
	}
	if rows[1].Icon != "sword-brandish" {
		t.Errorf("the melee row keeps its heading: %+v", rows[1])
	}
	if len(flat) != len(cards) {
		t.Errorf("flat has %d paths, want every one of %d", len(flat), len(cards))
	}
}

// The category decides, not the name: a known name with another main category moves, and with none it goes in the extra row.
func TestPathRowsIgnoreTheNameWithoutItsCategory(t *testing.T) {
	cards := cardsNamed("Path of the Duelist", "Path of the Ranger")
	cards[0].Categories = []string{"ranged"}
	rows, _ := pathRows(cards)
	if len(rows) != 2 || rows[0].Kind != "ranged" || !slices.Equal(cardNames(rows[0].Cards), []string{"Path of the Duelist"}) {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[1].Icon != "" || !slices.Equal(cardNames(rows[1].Cards), []string{"Path of the Ranger"}) {
		t.Errorf("extra row = %+v", rows[1])
	}
}

// A path with no category, or a main category no row takes, goes in an extra last row without an icon, in the file's order; a row with no
// paths is left out; a blocked path keeps its place.
func TestPathRowsUnknownAndEmpty(t *testing.T) {
	cards := cardsNamed("Path of the Smith", "Path of the Ranger", "Path of the Cook", "Path of the Sharpshooter")
	cards[1].Categories, cards[2].Categories, cards[3].Categories = []string{"ranged"}, []string{"siege"}, []string{"ranged"}
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
