package glossaryref

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func isOneToTen(v []int) bool {
	if len(v) != 10 {
		return false
	}
	for i, x := range v {
		if x != i+1 {
			return false
		}
	}
	return true
}

// ToWoundErrors runs the checks of ToWoundErrorsJSON on a typed table, so a test can break it field by field.
func ToWoundErrors(tw ToWoundTable) []string {
	return cellErrors(isOneToTen(tw.Strengths) && isOneToTen(tw.Defences), tw.Cells)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	d, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for ; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			t.Fatal("go.mod not found")
		}
	}
}

func clone(tw ToWoundTable) ToWoundTable {
	c := tw
	c.Cells = make([][]string, len(tw.Cells))
	for i, r := range tw.Cells {
		c.Cells[i] = append([]string(nil), r...)
	}
	return c
}

func hasError(errs []string, sub string) bool {
	for _, e := range errs {
		if strings.Contains(e, sub) {
			return true
		}
	}
	return false
}

func TestToWoundTableIsValid(t *testing.T) {
	if errs := ToWoundErrors(ToWound()); len(errs) != 0 {
		t.Fatal(errs)
	}
}

func TestCommittedFileMatchesGenerator(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "definitions/glossary/mesbg_glossary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Reference struct {
			ToWound json.RawMessage `json:"toWound"`
		} `json:"reference"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(ToWound())
	var got, exp any
	if err := json.Unmarshal(doc.Reference.ToWound, &got); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(want, &exp)
	if !reflect.DeepEqual(got, exp) {
		t.Fatal("the committed toWound table differs from the generator")
	}
	// The same checks on the decoded JSON.
	var m map[string]any
	_ = json.Unmarshal(doc.Reference.ToWound, &m)
	if errs := ToWoundErrorsJSON(m); len(errs) != 0 {
		t.Fatal(errs)
	}
}

func TestKnownCells(t *testing.T) { // corners and the examples of the rulebook (S2 vs D3 = 5+)
	c := ToWound().Cells
	got := []string{c[0][0], c[0][9], c[1][9], c[9][0], c[9][9], c[1][2], c[4][9]}
	want := []string{"4+", "-", "-", "3+", "4+", "5+", "6+/4+"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestIncompleteGrid(t *testing.T) {
	tw := clone(ToWound())
	tw.Cells[3] = tw.Cells[3][:9]
	if len(ToWoundErrors(tw)) == 0 {
		t.Fatal("no error for an incomplete grid")
	}
}

func TestBadAxes(t *testing.T) {
	tw := clone(ToWound())
	tw.Strengths = tw.Strengths[:9]
	if !hasError(ToWoundErrors(tw), "1..10") {
		t.Fatal("no error for short strengths")
	}
}

func TestValueOutsideSet(t *testing.T) {
	tw := clone(ToWound())
	tw.Cells[5][5] = "2+"
	if !hasError(ToWoundErrors(tw), "allowed") {
		t.Fatal("no error for 2+")
	}
}

func TestEasierWithDefence(t *testing.T) {
	tw := clone(ToWound())
	tw.Cells[0][4] = "3+"
	if !hasError(ToWoundErrors(tw), "easier") {
		t.Fatal("no error for a cell that eases with Defence")
	}
}

func TestHarderWithStrength(t *testing.T) {
	tw := clone(ToWound())
	tw.Cells[9][9] = "-"
	if !hasError(ToWoundErrors(tw), "harder") {
		t.Fatal("no error for a cell that hardens with Strength")
	}
}

func TestErrorsJSONNegative(t *testing.T) {
	b, _ := json.Marshal(ToWound())
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	m["cells"].([]any)[0].([]any)[4] = "2+"
	if !hasError(ToWoundErrorsJSON(m), "allowed") {
		t.Fatal("no error for 2+ in decoded JSON")
	}
	m["strengths"] = []any{1.0}
	if !hasError(ToWoundErrorsJSON(m), "1..10") {
		t.Fatal("no error for short strengths in decoded JSON")
	}
	delete(m, "cells")
	if !hasError(ToWoundErrorsJSON(m), "10x10") {
		t.Fatal("no error for missing cells")
	}
}

func TestCellRankOrder(t *testing.T) {
	order := []string{"3+", "4+", "5+", "6+", "6+/4+", "6+/5+", "6+/6+", "-"}
	for i := 1; i < len(order); i++ {
		if CellRank(order[i-1]) >= CellRank(order[i]) {
			t.Fatalf("%s should rank below %s", order[i-1], order[i])
		}
	}
}

func TestTermsWellFormed(t *testing.T) {
	names := map[string]bool{}
	for _, tm := range Terms() {
		if names[tm.Name] {
			t.Errorf("duplicate term %q", tm.Name)
		}
		names[tm.Name] = true
		if len(tm.Headings) == 0 && len(tm.Paragraphs) == 0 {
			t.Errorf("%q has neither headings nor paragraphs", tm.Name)
		}
	}
	if len(names) != 252 {
		t.Errorf("got %d terms, want 252", len(names))
	}
}

func TestKeywordTerms(t *testing.T) {
	k := KeywordTerms()
	if len(k) != len(RaceKeywords)+len(FactionKeywords) {
		t.Fatalf("got %d keyword terms", len(k))
	}
	if k[0].Name != "Bat keyword" || k[0].Kind != "race" ||
		k[0].Definition != "Bat is a race keyword. Special rules that name a keyword, such as Hatred (Bat), apply to models that have it." {
		t.Fatalf("unexpected first keyword term: %+v", k[0])
	}
	if last := k[len(k)-1]; last.Kind != "faction" || last.Name != "Umbar keyword" {
		t.Fatalf("unexpected last keyword term: %+v", last)
	}
	if KeywordSource != (Source{Section: "Keywords", RulebookPage: 8}) {
		t.Fatal("keyword source changed")
	}
}

func TestNorm(t *testing.T) {
	got := norm("\ufeffa “b” 3 ” ’x’  …\t")
	if want := `a "b" 3" 'x' ...`; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestTitleCase(t *testing.T) {
	if got := strings.ReplaceAll(titleCase("HERO'S WAY 2ND"), "'S", "'s"); got != "Hero's Way 2Nd" {
		t.Fatalf("got %q", got)
	}
}

const fixture = `<!-- page 3 -->
## HEROIC ACTIONS

A Heroic Action is something a hero can do once per turn, at the right moment.

## STAND FAST!

Stand Fast! lets a model take a Courage test again and again.
`

func build(t *testing.T, md, summaries string) (BuildResult, string) {
	t.Helper()
	dir := t.TempDir()
	src, sum, out := filepath.Join(dir, "r.md"), filepath.Join(dir, "s.json"), filepath.Join(dir, "o", "g.json")
	if err := os.WriteFile(src, []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sum, []byte(summaries), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := BuildGlossary(BuildOptions{Src: src, Out: out, Summaries: sum})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return res, string(b)
}

func TestBuildReportsProblems(t *testing.T) {
	res, out := build(t, fixture, `{"Nothing":"x"}`)
	if !strings.HasSuffix(out, "\n") || !strings.Contains(out, `"Heroic Actions"`) {
		t.Fatal("the file lacks the extracted term or the final newline")
	}
	joined := strings.Join(res.Problems, "\n")
	for _, want := range []string{"heading 'PRIORITY PHASE' not found", "summary for 'Nothing' has no synth term"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in %v", want, res.Problems[:3])
		}
	}
	if res.Terms < len(KeywordTerms()) {
		t.Fatalf("the keyword terms are missing: %d", res.Terms)
	}
}
