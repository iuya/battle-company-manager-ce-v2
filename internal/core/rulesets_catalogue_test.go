package core

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The upload error catalogue (docs/definitions.md, "Upload validation"): every invalid fixture of the lean package goes through
// Rulesets.Check against the shipped vanilla file and glossary, and the whole rendered report (path, message, closest match, hint,
// order, truncation) is compared with testdata/catalogue/<case>.want. A change of wording shows up here as a diff to review:
//
//	go test ./internal/core -run TestUploadErrorCatalogue -update-catalogue
var updateCatalogue = flag.Bool("update-catalogue", false, "rewrite testdata/catalogue/*.want")

func renderReport(rep RulesetReport) string {
	var b strings.Builder
	for _, i := range rep.Issues() {
		b.WriteString(i.String())
		b.WriteByte('\n')
	}
	if rep.Truncated() > 0 {
		fmt.Fprintf(&b, "and %d more\n", rep.Truncated())
	}
	return b.String()
}

// v1Routed are fixtures with no schemaVersion or schemaVersion 1: an upload like that is an old-format file and goes to the JSON schema, not to the
// lean decoder (they are pinned against the decoder in the lean package).
var v1Routed = map[string]bool{"old_format": true, "version_missing": true, "version": true}

func TestUploadErrorCatalogue(t *testing.T) {
	ctx := context.Background()
	rs, _, _ := v2Rulesets(t, ctx)
	files, _ := filepath.Glob("lean/testdata/invalid/*.json")
	if len(files) < 40 || len(files)-len(v1Routed) != countWants(t) {
		t.Fatalf("only %d invalid fixtures", len(files))
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".json")
		if v1Routed[name] {
			continue
		}
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			rep, cerr := rs.Check(ctx, KindBattleCompanies, raw)
			got := renderReport(rep)
			if (rep.Errors() > 0) != (cerr != nil) || (cerr != nil && !errors.Is(cerr, ErrInvalidRuleset)) {
				t.Errorf("Check error %v does not match the report (%d errors)", cerr, rep.Errors())
			}
			want := filepath.Join("testdata", "catalogue", name+".want")
			if *updateCatalogue {
				if err := os.WriteFile(want, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			w, err := os.ReadFile(want)
			if err != nil {
				t.Fatalf("no %s (run with -update-catalogue): %v", want, err)
			}
			if got != string(w) {
				t.Errorf("report differs from %s\n--- got\n%s--- want\n%s", want, got, w)
			}
		})
	}
}

// TestUploadReportsEveryMistakeOfOneFile: a file with twelve independent mistakes returns twelve issues in one report, not the first.
func TestUploadReportsEveryMistakeOfOneFile(t *testing.T) {
	ctx := context.Background()
	rs, _, _ := v2Rulesets(t, ctx)
	raw, err := os.ReadFile("lean/testdata/invalid/many_mistakes.json")
	if err != nil {
		t.Fatal(err)
	}
	rep, cerr := rs.Check(ctx, KindBattleCompanies, raw)
	if cerr == nil || rep.Errors() != 12 || len(rep.Issues()) != 12 || rep.Truncated() != 0 {
		t.Fatalf("%d errors, %d issues, err %v", rep.Errors(), len(rep.Issues()), cerr)
	}
	paths := map[string]bool{}
	for _, i := range rep.Issues() {
		if i.File != "battle_companies.json" || i.Path == "" || i.Message == "" || paths[i.Path] {
			t.Errorf("issue %+v (paths so far %v)", i, paths)
		}
		paths[i.Path] = true
	}
}

// TestUploadReportIsDeterministic: the same bytes give the same text, run after run.
func TestUploadReportIsDeterministic(t *testing.T) {
	ctx := context.Background()
	rs, _, _ := v2Rulesets(t, ctx)
	raw, _ := os.ReadFile("lean/testdata/invalid/many_mistakes.json")
	first, _ := rs.Check(ctx, KindBattleCompanies, raw)
	for i := 0; i < 10; i++ {
		again, _ := rs.Check(ctx, KindBattleCompanies, raw)
		if renderReport(again) != renderReport(first) {
			t.Fatal("the report changed between runs")
		}
	}
}

// TestUploadReportIsCappedAt1000: a file with thousands of mistakes lists 1000 and counts the rest.
func TestUploadReportIsCappedAt1000(t *testing.T) {
	ctx := context.Background()
	rs, _, _ := v2Rulesets(t, ctx)
	var b strings.Builder
	b.WriteString(`{"schemaVersion":2,"name":"x","version":"1","extends":"y","campaign":{},"rules":[`)
	for i := 0; i < 1500; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"name":"Rule ` + strings.Repeat("x", i%5) + string(rune('a'+i%26)) + `","bogus":1}`)
	}
	b.WriteString(`]}`)
	rep, err := rs.Check(ctx, KindBattleCompanies, []byte(b.String()))
	if err == nil || len(rep.Issues()) != 1000 || rep.Truncated() == 0 {
		t.Fatalf("%d issues, %d truncated, err %v", len(rep.Issues()), rep.Truncated(), err)
	}
	if !strings.Contains(err.Error(), "more not listed") {
		t.Errorf("the summary counts the rest: %v", err)
	}
}

func countWants(t *testing.T) int {
	t.Helper()
	w, _ := filepath.Glob("testdata/catalogue/*.want")
	return len(w)
}
