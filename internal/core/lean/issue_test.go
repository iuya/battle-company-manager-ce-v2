package lean

import (
	"fmt"
	"strings"
	"testing"
)

func TestIssueString(t *testing.T) {
	for _, c := range []struct {
		name string
		in   Issue
		want string
	}{
		{"unknown key", Issue{File: "companies/x.json", Path: "companies[2].strat", Message: `unknown field; did you mean "start"?`, Closest: "start"},
			`companies/x.json: companies[2].strat: unknown field; did you mean "start"?`},
		{"closest appended", Issue{File: "bc.json", Path: "start.units[1].unit", Message: `no unit "Ranger of Arnr" in this company, the shared units or vanilla`, Closest: "Ranger of Arnor"},
			`bc.json: start.units[1].unit: no unit "Ranger of Arnr" in this company, the shared units or vanilla; closest: "Ranger of Arnor"`},
		{"warn with hint", Issue{File: "bc.json", Path: "rows[3].text", Message: "differs", Hint: "remove text", Severity: Warn},
			"warn: bc.json: rows[3].text: differs\n  hint: remove text"},
		{"no path", Issue{File: "battle_companies.json", Message: "schemaVersion 1 is not supported (this app reads 2)"},
			"battle_companies.json: schemaVersion 1 is not supported (this app reads 2)"},
		{"no file", Issue{Path: "a", Message: "m"}, "a: m"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := c.in.String(); got != c.want {
				t.Errorf("got\n%s\nwant\n%s", got, c.want)
			}
		})
	}
}

func TestReportCapAndOK(t *testing.T) {
	var r Report
	if !r.OK() {
		t.Fatal("empty report must be OK")
	}
	r.Warnf("f", "p", "w")
	if !r.OK() || len(r.Errors()) != 0 || len(r.Warnings()) != 1 {
		t.Fatalf("a warning does not block: %+v", r)
	}
	r.Errorf("f", "p", "e")
	if r.OK() || len(r.Errors()) != 1 {
		t.Fatal("an error blocks")
	}
	var big Report
	for i := 0; i < MaxIssues+7; i++ {
		big.Errorf("f", fmt.Sprintf("p[%d]", i), "m")
	}
	if len(big.Issues) != MaxIssues || big.Truncated != 7 {
		t.Fatalf("cap: %d kept, %d truncated", len(big.Issues), big.Truncated)
	}
	if !strings.HasSuffix(big.String(), "and 7 more\n") {
		t.Fatalf("report text does not end with the count: %q", big.String()[len(big.String())-30:])
	}
	var onlyWarn Report
	for i := 0; i < MaxIssues+1; i++ {
		onlyWarn.Warnf("f", "p", "w")
	}
	if onlyWarn.OK() {
		t.Fatal("a truncated report is never OK")
	}
	var m Report
	m.Merge(big)
	m.Merge(big)
	if len(m.Issues) != MaxIssues || m.Truncated != 7+MaxIssues+7 {
		t.Fatalf("merge: %d kept, %d truncated", len(m.Issues), m.Truncated)
	}
}

func TestReportDeterministicText(t *testing.T) {
	build := func() string {
		var r Report
		r.Errorf("a.json", "x", "one")
		r.Warnf("a.json", "y", "two")
		return r.String()
	}
	if build() != build() || build() != "a.json: x: one\nwarn: a.json: y: two\n" {
		t.Fatalf("unexpected report text %q", build())
	}
}
