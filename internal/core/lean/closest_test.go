package lean

import (
	"encoding/json"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"unicode"
)

func TestNameKey(t *testing.T) {
	for in, want := range map[string]string{
		"Lake-Town":           "lake town",
		"lake town":           "lake town",
		"  Lake   Town  ":     "lake town",
		"At 'em, Lads!":       "at em lads",
		"Wizard's Staff":      "wizards staff",
		"Wizard’s Staff":      "wizards staff",
		"Bow":                 "bow",
		"Bows":                "bows", // plurals are not folded
		"Elven-made Glaive":   "elven made glaive",
		"Elven made Glaive":   "elven made glaive",
		"Elven Glaive":        "elven glaive", // "made" is not dropped
		"Dúnedain":            "dunedain",     // accents are folded
		"Lothlórien":          "lothlorien",
		"Mahûd Warrior":       "mahud warrior",
		"!!!":                 "",
		"":                    "",
		"Armoured Horse/Goat": "armoured horse goat",
		"2d6":                 "2d6",
	} {
		if got := NameKey(in); got != want {
			t.Errorf("NameKey(%q) = %q, want %q", in, got, want)
		}
	}
	if NameKey("Bow") == NameKey("Bows") {
		t.Error("plural must not fold")
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{"Ranger of Arnor": "ranger-of-arnor", "Lake-Town": "lake-town", "Wizard's Staff": "wizards-staff"} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClosest(t *testing.T) {
	gear := []string{"Bow", "Spear", "Shield", "Sword", "Banner", "Armour"}
	for _, c := range []struct {
		name  string
		cands []string
		in    string
		want  string
	}{
		{"typo", gear, "Speer", "Spear"},
		{"swap", gear, "Sheild", "Shield"},
		{"case and punctuation", gear, "spear!", "Spear"},
		{"exact", gear, "Bow", "Bow"},
		{"nothing close", gear, "Zeppelin", ""},
		{"short words stay strict", []string{"Bow", "Axe"}, "Cow", "Bow"},
		{"short word too far", []string{"Bow", "Axe"}, "Lance", ""},
		{"containment", []string{"Foresight", "Hatred"}, "Foresight of the Seer", "Foresight"},
		{"tie goes alphabetical", []string{"Bat", "Cat"}, "Hat", "Bat"},
		{"tie is order independent", []string{"Cat", "Bat"}, "Hat", "Bat"},
		{"nearer beats alphabetical", []string{"Aaaaa", "Speer"}, "Spear", "Speer"},
		{"typo beats containment", []string{"Ranger of Arnor Elite", "Ranger of Arnor"}, "Ranger of Arnr", "Ranger of Arnor"},
		{"no candidates", nil, "x", ""},
		{"empty input", gear, "", ""},
		{"punctuation only", gear, "!!", ""},
		{"candidate with no letters ignored", []string{"--"}, "x", ""},
		{"unicode", []string{"Dúnedain"}, "Dunedain", "Dúnedain"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := Closest(c.cands, c.in); got != c.want {
				t.Errorf("Closest(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestDistance(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{{"", "", 0}, {"a", "", 1}, {"", "ab", 2}, {"speer", "spear", 1}, {"ab", "ba", 1}, {"kitten", "sitting", 3}, {"abc", "abc", 0}} {
		if got := distance(c.a, c.b); got != c.want {
			t.Errorf("distance(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestSortedKeys(t *testing.T) {
	got := SortedKeys(map[string]int{"b": 1, "a": 2, "c": 3})
	if !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatal(got)
	}
}

// Folding accents makes names equal that were different keys. The shipped files must not hold two names that only the folding makes one
// (a company called Lothlorien and a unit called Lothlórien would silently be the same thing). The exceptions are the spellings the
// vanilla file itself mixes for one thing, listed here with the reason.
func TestFoldingAccentsMergesNoTwoShippedNames(t *testing.T) {
	allowed := map[string]string{
		"lothlorien": "the faction keyword is spelled both ways by the vanilla file (keywordSpellings merged it by hand); the company is Lothlorien with the label Lothlórien",
	}
	for _, file := range []string{"vanilla/data2024.json", "battle_companies/battle_companies.json", "glossary/mesbg_glossary.json"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "definitions", file))
		if err != nil {
			t.Fatal(err)
		}
		var doc any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		names := map[string]bool{}
		var walk func(v any, key string)
		walk = func(v any, key string) {
			switch x := v.(type) {
			case map[string]any:
				for k, e := range x {
					walk(e, k)
				}
			case []any:
				for _, e := range x {
					walk(e, key)
				}
			case string:
				if slices.Contains([]string{"name", "aka", "label", "faction", "race", "keywords", "unitType"}, key) {
					names[x] = true
				}
			}
		}
		walk(doc, "")
		groups := map[string][]string{}
		for n := range names {
			if nameKey(n, false) == nameKey(n, true) { // no accent in it: folding changes nothing
				continue
			}
			groups[nameKey(n, true)] = append(groups[nameKey(n, true)], n)
		}
		for n := range names {
			if k := nameKey(n, true); len(groups[k]) > 0 && !slices.Contains(groups[k], n) {
				groups[k] = append(groups[k], n) // a plain spelling of an accented name
			}
		}
		for key, list := range groups {
			plain := 0
			for _, n := range list {
				if nameKey(n, false) == nameKey(n, true) {
					plain++
				}
			}
			distinctAccented := map[string]bool{}
			for _, n := range list {
				if nameKey(n, false) != nameKey(n, true) {
					distinctAccented[nameKey(n, false)] = true
				}
			}
			if _, ok := allowed[key]; ok {
				continue
			}
			if plain > 0 || len(distinctAccented) > 1 {
				sort.Strings(list)
				t.Errorf("%s: %q fold to the one key %q", file, list, key)
			}
		}
	}
}

// referenceNameKey is the rune-by-rune definition of nameKey that asciiNameKey must equal on ASCII text.
func referenceNameKey(s string) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case r == '\'' || r == '’' || r == '`':
			continue
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(unicode.ToLower(r))
		default:
			space = true
		}
	}
	return b.String()
}

func TestASCIINameKeyEqualsTheRuneDefinition(t *testing.T) {
	alphabet := []byte("abcXYZ019 -_'`.,()/\t\x00~")
	rng := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 20000; i++ {
		b := make([]byte, rng.IntN(130)) // past the stack buffer too
		for j := range b {
			b[j] = alphabet[rng.IntN(len(alphabet))]
		}
		if got, want := NameKey(string(b)), referenceNameKey(string(b)); got != want {
			t.Fatalf("NameKey(%q) = %q, want %q", b, got, want)
		}
	}
	for c := 0; c < 128; c++ {
		s := "a" + string(rune(c)) + "B"
		if got, want := NameKey(s), referenceNameKey(s); got != want {
			t.Fatalf("NameKey(%q) = %q, want %q", s, got, want)
		}
	}
}

func BenchmarkNameKey(b *testing.B) {
	for _, s := range []string{"Warrior of Minas Tirith", "warrior of minas tirith", "Lake-Town", "Lothlórien"} {
		b.Run(s, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				NameKey(s)
			}
		})
	}
}
