package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// The examples of the customizing guide (docs/customizing-rulesets.md) are files of examples/customizing-rulesets. These tests run every
// one of them through the real checker and the real loader, so an example cannot rot: a change of the format that breaks one fails
// make test. They also check that the README lists them, that the guide quotes them verbatim, and that the fragments work in the
// shipped file, which is where most people start.

const (
	examplesDir = "../../examples/customizing-rulesets"
	guidePath   = "../../docs/customizing-rulesets.md"
	shippedBC   = "../../definitions/battle_companies/battle_companies.json"
)

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func exampleFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(examplesDir, "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no examples in %s (%v)", examplesDir, err)
	}
	return files
}

func load(t *testing.T, bc []byte) (*core.RuleSet, error) {
	t.Helper()
	return core.LoadRuleSet(readFile(t, "../../definitions/vanilla/data2024.json"), bc, readFile(t, "../../definitions/glossary/mesbg_glossary.json"), slog.New(slog.DiscardHandler))
}

func TestExamplesCheckAndLoad(t *testing.T) {
	for _, f := range exampleFiles(t) {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			code, out, _ := runCheck(t, f)
			if code != 0 || !strings.HasSuffix(out, name+": 0 errors, 0 warnings\n") {
				t.Errorf("make check-ruleset: exit %d\n%s", code, out)
			}
			if _, err := load(t, readFile(t, f)); err != nil {
				t.Errorf("the loader refuses it: %v", err)
			}
		})
	}
}

// TestBrokenExampleReport: the report the guide walks through is what the checker prints for broken/typos.json today.
func TestBrokenExampleReport(t *testing.T) {
	code, out, _ := runCheck(t, filepath.Join(examplesDir, "broken", "typos.json"))
	want := string(readFile(t, filepath.Join(examplesDir, "broken", "typos.report.txt")))
	if code != 1 || out != want {
		t.Errorf("exit %d; the report changed (rewrite broken/typos.report.txt if the new wording is right):\n%s", code, out)
	}
}

func TestExamplesAreListedInTheReadme(t *testing.T) {
	readme := string(readFile(t, filepath.Join(examplesDir, "README.md")))
	for _, f := range exampleFiles(t) {
		if !strings.Contains(readme, filepath.Base(f)) {
			t.Errorf("examples/customizing-rulesets/README.md does not list %s", filepath.Base(f))
		}
	}
}

// quote is `<!-- example: file.json -->` followed by a fenced json block in the guide: the block must be a piece of that file,
// whitespace aside, so the guide never retypes an example in a form that drifts.
var quote = regexp.MustCompile("(?s)<!-- example: ([a-z-]+\\.json) -->\\s*```json\\n(.*?)```")

// report is `<!-- report: broken/typos.report.txt -->` followed by a fenced text block that is the whole file.
var report = regexp.MustCompile("(?s)<!-- report: ([a-z/.-]+) -->\\s*```text\\n(.*?)```")

func squash(s string) string { return strings.Join(strings.Fields(s), "") }

func TestGuideQuotesTheExamples(t *testing.T) {
	guide, err := os.ReadFile(guidePath)
	if err != nil {
		t.Skip("the guide is not there: ", err)
	}
	quotes := quote.FindAllStringSubmatch(string(guide), -1)
	if len(quotes) == 0 {
		t.Fatal("the guide quotes no example (<!-- example: file.json --> before a json block)")
	}
	used := map[string]bool{}
	for _, q := range quotes {
		file, block := q[1], q[2]
		used[file] = true
		src, err := os.ReadFile(filepath.Join(examplesDir, file))
		if err != nil {
			t.Errorf("the guide quotes %s, which is not an example", file)
			continue
		}
		if !strings.Contains(squash(string(src)), squash(block)) {
			t.Errorf("a block of the guide is not a piece of %s any more:\n%s", file, block)
		}
	}
	for _, q := range report.FindAllStringSubmatch(string(guide), -1) {
		want, err := os.ReadFile(filepath.Join(examplesDir, q[1]))
		if err != nil {
			t.Errorf("the guide quotes the report %s, which is not there", q[1])
			continue
		}
		if strings.TrimSpace(q[2]) != strings.TrimSpace(string(want)) {
			t.Errorf("the report block of the guide differs from %s:\n%s", q[1], q[2])
		}
	}
	for _, f := range exampleFiles(t) {
		if !used[filepath.Base(f)] {
			t.Errorf("the guide quotes nothing of %s", filepath.Base(f))
		}
	}
}

// sections are the lists whose new entries the example adds to the shipped file.
var sections = []string{"rules", "gear", "paths", "units", "companies"}

// TestExamplesWorkInTheShippedFile: the entries an example adds (the ones whose name the shipped file does not use) are added to a
// copy of the shipped file, which must still check and load. That is how a real ruleset grows, not as a standalone file.
func TestExamplesWorkInTheShippedFile(t *testing.T) {
	for _, f := range exampleFiles(t) {
		name := filepath.Base(f)
		if name == "minimal.json" || name == "parameter-override.json" || name == "custom-scenario.json" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			var ex, doc map[string]any
			mustJSON(t, readFile(t, f), &ex)
			mustJSON(t, readFile(t, shippedBC), &doc)
			added := 0
			for _, sec := range sections {
				have := map[string]bool{}
				list, _ := doc[sec].([]any)
				for _, e := range list {
					have[lean.NameKey(e.(map[string]any)["name"].(string))] = true
				}
				for _, e := range asList(ex[sec]) {
					if n := e.(map[string]any)["name"].(string); !have[lean.NameKey(n)] {
						list = append(list, e)
						added++
					}
				}
				doc[sec] = list
			}
			if added == 0 {
				t.Fatalf("%s adds nothing the shipped file lacks: the example is no longer an example", name)
			}
			bc, _ := json.Marshal(doc)
			tmp := filepath.Join(t.TempDir(), "shipped-plus-"+name)
			if err := os.WriteFile(tmp, bc, 0o644); err != nil {
				t.Fatal(err)
			}
			if code, out, _ := runCheck(t, tmp); code != 0 || !strings.Contains(out, "0 errors, 0 warnings") {
				t.Errorf("the shipped file with %s added does not check:\n%s", name, out)
			}
			if _, err := load(t, bc); err != nil {
				t.Errorf("the loader refuses the shipped file with %s added: %v", name, err)
			}
		})
	}
}

// TestParameterOverrideWorksInTheShippedFile: the parameters the example changes are set in a copy of the shipped file, and the
// engine reads the new values.
func TestParameterOverrideWorksInTheShippedFile(t *testing.T) {
	var ex, base, doc map[string]any
	mustJSON(t, readFile(t, filepath.Join(examplesDir, "parameter-override.json")), &ex)
	mustJSON(t, readFile(t, filepath.Join(examplesDir, "minimal.json")), &base)
	mustJSON(t, readFile(t, shippedBC), &doc)
	changed := 0
	var walk func(path []string, a, b any)
	walk = func(path []string, a, b any) {
		am, aok := a.(map[string]any)
		bm, _ := b.(map[string]any)
		if aok {
			for k, v := range am {
				walk(append(path[:len(path):len(path)], k), v, bm[k])
			}
			return
		}
		if _, isList := a.([]any); isList || a == b {
			return
		}
		node := doc
		for _, k := range path[:len(path)-1] {
			node = node[k].(map[string]any)
		}
		node[path[len(path)-1]] = a
		changed++
	}
	walk([]string{"campaign"}, ex["campaign"], base["campaign"])
	if changed < 5 {
		t.Fatalf("the example changes %d parameters", changed)
	}
	bc, _ := json.Marshal(doc)
	rs, err := load(t, bc)
	if err != nil {
		t.Fatalf("the loader refuses the shipped file with the parameters changed: %v", err)
	}
	if got := rs.BC.Recruiting().Adjust.Max; got != 5 {
		t.Errorf("recruitment.adjust.maxPerAttempt: the engine reads %d, want 5", got)
	}
	if got := rs.BC.Recruiting().Adjust.CostPerPoint; got != 2 {
		t.Errorf("recruitment.adjust.costPerPoint: the engine reads %d, want 2", got)
	}
	if got := rs.BC.Recruiting().StartingInfluence; got != 2 {
		t.Errorf("company.startingInfluence: the engine reads %d, want 2", got)
	}
	if got := rs.BC.Shape().MaxModels; got != 18 {
		t.Errorf("company.maxModels: the engine reads %d, want 18", got)
	}
	if got := rs.BC.Aftermath().LevelEvery; got != 4 {
		t.Errorf("progression.threshold: the engine reads %d, want 4", got)
	}
	if got := rs.BC.BreakRules().DefaultPercent; got != 40 {
		t.Errorf("breakPoint.default.percent: the engine reads %d, want 40", got)
	}
}

func mustJSON(t *testing.T, b []byte, into *map[string]any) {
	t.Helper()
	if err := json.Unmarshal(b, into); err != nil {
		t.Fatal(err)
	}
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

// TestUnitFormsInTheApp pins what the app does with the three forms of a unit, which the guide states: a unit built on another (a company's
// own or a shared one) becomes the base profile with its options and rules, a brand-new unit is a profile of its own, and the company fields them
// by name in its starting force and charts.
func TestUnitFormsInTheApp(t *testing.T) {
	rs, err := load(t, readFile(t, filepath.Join(examplesDir, "custom-unit.json")))
	if err != nil {
		t.Fatal(err)
	}
	p, ok := rs.BC.Profile("Greenway Wardens")
	if !ok {
		t.Fatal("no profile for Greenway Wardens")
	}
	_, alias, found := rs.ResolveUnit(p, "Greenway Watcher")
	if !found || alias == nil || alias.Unit != "Ranger of Arnor" || len(alias.AddRules) != 1 || alias.AddRules[0] != "Greenway Lore" || len(alias.Selected) != 1 || alias.Selected[0] != "Spear" {
		t.Errorf("the company's own unit does not resolve to Ranger of Arnor + Spear + Greenway Lore: %v %+v", found, alias)
	}
	v, alias, found := rs.ResolveUnit(p, "Arnor Scout")
	if !found || alias == nil || v.Name != "Ranger of Arnor" || len(alias.Selected) != 1 || alias.Selected[0] != "Spear" {
		t.Errorf("the shared unit does not resolve to Ranger of Arnor + Spear: %v %+v %s", found, alias, v.Name)
	}
	v, alias, found = rs.ResolveUnit(p, "Hound of the Barrows")
	if !found || alias != nil || v.Name != "Barrow Hound" || v.Points != 6 || v.Stats["movement"] != 9 || len(v.Options) != 1 {
		t.Errorf("the brand-new unit does not resolve to its own profile: %v %+v", found, v)
	}
	var fielded []string
	for _, u := range p.Start {
		fielded = append(fielded, u.Unit)
	}
	if !slices.Contains(fielded, "Barrow Hound") {
		t.Errorf("the company does not start with the brand-new unit: %v", fielded)
	}
}
