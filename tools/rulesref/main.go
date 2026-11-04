// Command rulesref writes docs/customizing-rulesets-reference.md from definitions/battle_companies/battle_companies.schema.json.
//
//	go run ./tools/rulesref            # rewrite the reference
//	go run ./tools/rulesref -check     # exit 1 when the committed reference is stale
//
// Flags -schema and -out override the two paths (relative paths are read from the repository root).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/repo"
	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/rulesref"
)

func main() {
	root := repo.Root()
	check := flag.Bool("check", false, "exit 1 when the committed reference differs from what the schema gives")
	schemaPath := flag.String("schema", "definitions/battle_companies/battle_companies.schema.json", "the schema")
	outPath := flag.String("out", "docs/customizing-rulesets-reference.md", "the reference")
	flag.Parse()
	schemaFile, out := abs(root, *schemaPath), abs(root, *outPath)

	raw, err := os.ReadFile(schemaFile)
	if err != nil {
		fail(err)
	}
	text, err := rulesref.Render(raw)
	if err != nil {
		fail(fmt.Errorf("%s: %w", schemaFile, err))
	}
	if *check {
		have, _ := os.ReadFile(out)
		if string(have) != text {
			fmt.Fprintf(os.Stderr, "%s is stale: run `make ruleset-reference`\n", out)
			os.Exit(1)
		}
		return
	}
	if err := os.WriteFile(out, []byte(text), 0o644); err != nil {
		fail(err)
	}
	fmt.Println("wrote " + out)
}

func abs(root, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(root, p)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "rulesref:", err)
	os.Exit(1)
}
