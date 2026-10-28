package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// v2World loads the shipped vanilla file and glossary, the lower layers every v2 test decodes against.
func v2World(t *testing.T) (*VanillaBook, Glossary) {
	t.Helper()
	vb, err := os.ReadFile("../../definitions/vanilla/data2024.json")
	if err != nil {
		t.Fatal(err)
	}
	gb, err := os.ReadFile("../../definitions/glossary/mesbg_glossary.json")
	if err != nil {
		t.Fatal(err)
	}
	book, err := ParseVanilla(vb)
	if err != nil {
		t.Fatal(err)
	}
	g, err := BuildGlossary(RulesetInfo{Kind: KindGlossary}, gb)
	if err != nil {
		t.Fatal(err)
	}
	return book, g
}

func TestVanillaLower(t *testing.T) {
	book, gl := v2World(t)
	l := newVanillaLower(book, gl)
	for _, c := range []struct {
		kind string
		name string
		want bool
	}{
		{"unit", "Ranger of Arnor", true}, {"unit", "ranger of arnor", true}, {"unit", "Ranger of Arnr", false}, {"unit", "Rangers of Arnor", false},
		{"rule", "Stand Fast", true}, {"rule", "Heroic Strike", true}, {"rule", "Not A Rule Anywhere", false},
		{"gear", "Boar spear", true}, {"gear", "Spear of Doom", false},
		{"race", "Orc", true}, {"keyword", "Orc", true}, {"company", "Arnor", false},
	} {
		if got := l.Has(lean.Kind(c.kind), c.name); got != c.want {
			t.Errorf("Has(%s, %q) = %v, want %v", c.kind, c.name, got, c.want)
		}
	}
	names := l.Names(lean.KindUnit)
	if len(names) < 100 {
		t.Fatalf("only %d unit names", len(names))
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Fatalf("names are not sorted and unique at %d: %q %q", i, names[i-1], names[i])
		}
	}
	// options come from the profile
	var unit VanillaUnit
	for _, u := range book.units {
		if len(u.Options) > 0 && (unit.Name == "" || u.Name < unit.Name) {
			unit = u
		}
	}
	got := l.Options(strings.ToUpper(unit.Name))
	if len(got) != len(unit.Options) || got[0] != unit.Options[0].Name {
		t.Errorf("Options(%q) = %v, want the profile's %v", unit.Name, got, unit.Options)
	}
	if l.Options("Nobody") != nil {
		t.Error("an unknown unit has no options")
	}
}

func TestVanillaLowerDuplicates(t *testing.T) {
	book, gl := v2World(t)
	l := newVanillaLower(book, gl)
	raw, _ := os.ReadFile("../../definitions/vanilla/data2024.json")
	var doc struct {
		Data struct {
			Heroes   []struct{ Name string } `json:"heroes"`
			Warriors []struct{ Name string } `json:"warriors"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, u := range append(doc.Data.Heroes, doc.Data.Warriors...) {
		counts[strings.ToLower(u.Name)]++
	}
	checked := 0
	for name, n := range counts {
		if n > 1 {
			if got := l.Duplicates(lean.KindUnit, name); got != n {
				t.Errorf("Duplicates(%q) = %d, want %d", name, got, n)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("the vanilla file has no duplicated unit name to check")
	}
	if l.Duplicates(lean.KindUnit, "Nobody At All") != 0 {
		t.Error("an unknown name has no occurrence")
	}
	if l.Duplicates(lean.KindCompany, "Arnor") != 0 {
		t.Error("kinds without vanilla duplicates report 0")
	}
}

// TestDecodeValidFixturesAgainstShippedFiles: every valid fixture of the lean package decodes with no issue against the real
// vanilla file and glossary, so the worked examples of the design are true for the shipped lower layers.
func TestDecodeValidFixturesAgainstShippedFiles(t *testing.T) {
	book, gl := v2World(t)
	files, _ := filepath.Glob("lean/testdata/valid/*.json")
	if len(files) < 10 {
		t.Fatalf("only %d fixtures", len(files))
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			raw, _ := os.ReadFile(f)
			doc, rep := DecodeBattleCompanies("battle_companies.json", raw, book, gl)
			if doc == nil || len(rep.Issues) != 0 {
				t.Errorf("issues:\n%s", rep)
			}
		})
	}
}

// TestDecodeInvalidFixturesAgainstShippedFiles: every invalid fixture is still refused against the real lower layers (the exact
// wording is pinned in the lean package against a small fake world).
func TestDecodeInvalidFixturesAgainstShippedFiles(t *testing.T) {
	book, gl := v2World(t)
	files, _ := filepath.Glob("lean/testdata/invalid/*.json")
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			raw, _ := os.ReadFile(f)
			_, rep := DecodeBattleCompanies("battle_companies.json", raw, book, gl)
			if strings.HasSuffix(f, "silent_shadowing.json") {
				return // shadows "Hatred", which the shipped vanilla file does not define as a plain rule
			}
			if len(rep.Issues) == 0 {
				t.Errorf("an invalid fixture reported nothing")
			}
		})
	}
}

func TestSchemaVersionOf(t *testing.T) {
	for in, want := range map[string]int{
		`{"name":"x"}`: 1, `{"schemaVersion":1}`: 1, `{"schemaVersion":2}`: 2, `{"schemaVersion":3}`: 3,
		`{"schemaVersion":"2"}`: -1, `{"schemaVersion":2.5}`: -1, `[1]`: 0, `nope`: 0, ``: 0,
	} {
		if got := schemaVersionOf([]byte(in)); got != want {
			t.Errorf("schemaVersionOf(%s) = %d, want %d", in, got, want)
		}
	}
}

func TestOldShippedFileIsVersionOne(t *testing.T) {
	raw, err := os.ReadFile("../../definitions/battle_companies/battle_companies.json")
	if err != nil {
		t.Fatal(err)
	}
	if schemaVersionOf(raw) != 1 {
		t.Error("the shipped file is the old format")
	}
	book, gl := v2World(t)
	doc, rep := DecodeBattleCompanies("battle_companies.json", raw, book, gl)
	if doc != nil || rep.OK() || !strings.Contains(rep.String(), "schemaVersion: missing") {
		t.Errorf("the old file is refused with one clear message:\n%s", rep)
	}
}
