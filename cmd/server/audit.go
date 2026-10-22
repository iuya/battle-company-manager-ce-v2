package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// runAudit writes one markdown file per Battle Company, plus an index with the anomaly counts, from the shipped rules
// (docs/definitions.md, "Audit report"). It needs no database.
func runAudit(args []string) error {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	out := fs.String("out", "tmp/audit", "directory the reports are written to")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var files [3][]byte
	for i, name := range []string{"vanilla/data2024.json", "battle_companies/battle_companies.json", "glossary/mesbg_glossary.json"} {
		b, err := os.ReadFile(filepath.Join("definitions", name))
		if err != nil {
			return err
		}
		files[i] = b
	}
	return writeAudit(*out, files[0], files[1], files[2])
}

func writeAudit(dir string, vanilla, bc, glossary []byte) error {
	capture := &core.CaptureDuplicates{}
	rs, err := core.LoadRuleSet(vanilla, bc, glossary, slog.New(capture))
	if err != nil {
		return err
	}
	reviewed, err := core.ParseVanillaAmbiguities(bc)
	if err != nil {
		return err
	}
	report := core.AuditRules(rs, core.AuditInput{Ambiguities: reviewed, Unreviewed: capture.Found})
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, content := range core.RenderAudit(report) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			return err
		}
	}
	counts := map[string]int{}
	for _, f := range report.Flags() {
		counts[f.Kind]++
	}
	fmt.Printf("audited %d companies into %s\n", len(report.Companies), dir)
	for _, k := range core.AuditKinds {
		fmt.Printf("  %-22s %d\n", k, counts[k])
	}
	return nil
}
