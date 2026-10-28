package lean

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fixtures (testdata/valid, testdata/invalid) are the executable catalogue of design section 7. valid/*.json are complete
// minimal files built from the worked examples of section 4 and must produce no issue; invalid/<case>.json come with <case>.want,
// the exact report text. `go test ./internal/core/lean -run TestInvalidFixtures -update` rewrites the .want files: read the diff,
// the wording is reviewed there.
var update = flag.Bool("update", false, "rewrite the .want files of testdata/invalid")

// fixtureFile is the file name the reports quote.
const fixtureFile = "battle_companies.json"

// fixtureReport runs the whole decoder on a fixture.
func fixtureReport(t *testing.T, raw []byte) Report {
	t.Helper()
	return decodeFixture(t, raw)
}

func TestValidFixtures(t *testing.T) {
	files, _ := filepath.Glob("testdata/valid/*.json")
	if len(files) < 10 {
		t.Fatalf("only %d valid fixtures", len(files))
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if rep := fixtureReport(t, raw); len(rep.Issues) != 0 {
				t.Errorf("a valid fixture reports issues:\n%s", rep)
			}
		})
	}
}

func TestInvalidFixtures(t *testing.T) {
	files, _ := filepath.Glob("testdata/invalid/*.json")
	if len(files) == 0 {
		t.Fatal("no invalid fixtures")
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".json")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			got := fixtureReport(t, raw).String()
			want := strings.TrimSuffix(f, ".json") + ".want"
			if *update {
				if err := os.WriteFile(want, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			w, err := os.ReadFile(want)
			if err != nil {
				t.Fatalf("no %s (run with -update): %v", want, err)
			}
			if got == "" {
				t.Fatal("an invalid fixture must report something")
			}
			if got != string(w) {
				t.Errorf("report differs from %s\n--- got\n%s--- want\n%s", want, got, w)
			}
		})
	}
}

// TestFixtureCoverage: every .want has its .json and the reverse, so a renamed case cannot hide.
func TestFixtureCoverage(t *testing.T) {
	js, _ := filepath.Glob("testdata/invalid/*.json")
	ws, _ := filepath.Glob("testdata/invalid/*.want")
	if *update {
		return
	}
	if len(js) != len(ws) {
		t.Errorf("%d invalid fixtures but %d .want files", len(js), len(ws))
	}
}
