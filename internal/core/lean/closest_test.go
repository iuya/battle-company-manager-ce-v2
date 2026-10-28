package lean

import (
	"reflect"
	"testing"
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
		"Dúnedain":            "dúnedain",
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
