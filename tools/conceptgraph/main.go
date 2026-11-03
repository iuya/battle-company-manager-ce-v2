// Command conceptgraph builds definitions/concepts/concept_graph.json: the reviewed edges (grants, cancels, lifts, requires) between
// the concepts of the three shipped rules files, with the hashes that tell the app whether they still apply (docs/concept-graph.md).
//
//	go run ./tools/conceptgraph [-check] [-out FILE] [-curated FILE] [-vanilla FILE] [-battle-companies FILE] [-glossary FILE]
//
// Run after any change to one of the three files: the graph records the hashes of the files it was built from, and tools/validate fails
// when they changed. Derived from the lean Battle Companies file: item grants, path grants, shield cancellations and lifts, gear
// restrictions. The nodes come from the app's loader (core.LoadRuleSet). By hand: tools/concepts_curated.json. Mentions and see links are
// not stored: the app computes them.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/conceptgraph"
	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/jsonx"
	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/repo"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	root := repo.Root()
	fs := flag.NewFlagSet("conceptgraph", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", filepath.Join(root, "definitions/concepts/concept_graph.json"), "the graph file")
	curatedPath := fs.String("curated", filepath.Join(root, "tools/concepts_curated.json"), "the hand-curated edges")
	vanilla := fs.String("vanilla", filepath.Join(root, "definitions/vanilla/data2024.json"), "the vanilla rules file")
	bcPath := fs.String("battle-companies", filepath.Join(root, "definitions/battle_companies/battle_companies.json"), "the Battle Companies file")
	glossaryPath := fs.String("glossary", filepath.Join(root, "definitions/glossary/mesbg_glossary.json"), "the glossary")
	check := fs.Bool("check", false, "only report whether the file on disk is what the build would write")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	fail := func(err error) int {
		fmt.Fprintln(stderr, "conceptgraph:", err)
		return 1
	}
	var raws [3][]byte
	for i, p := range []string{*vanilla, *bcPath, *glossaryPath} {
		b, err := os.ReadFile(p)
		if err != nil {
			return fail(err)
		}
		raws[i] = b
	}
	curated, err := jsonx.ReadFile(*curatedPath)
	if err != nil {
		return fail(err)
	}
	g, report, err := conceptgraph.Build(raws, jsonx.O(curated), "Concept graph")
	if err != nil {
		return fail(err)
	}
	for _, e := range report.Errors {
		fmt.Fprintln(stdout, "ERROR:", e)
	}
	if len(report.Errors) > 0 {
		return 1
	}
	text := g.JSON()
	var order []string
	byType := map[string]int{}
	for _, e := range g.Edges {
		if byType[e.Type] == 0 {
			order = append(order, e.Type)
		}
		byType[e.Type]++
	}
	counts := make([]string, len(order))
	for i, t := range order {
		counts[i] = fmt.Sprintf("'%s': %d", t, byType[t])
	}
	fmt.Fprintf(stdout, "%d nodes, %d edges {%s}, %d ambiguous names\n", len(g.Nodes), len(g.Edges), strings.Join(counts, ", "), len(g.Ambiguous))
	seen := map[conceptgraph.Skipped]bool{}
	var skipped []conceptgraph.Skipped
	for _, s := range report.Skipped {
		if !seen[s] {
			seen[s] = true
			skipped = append(skipped, s)
		}
	}
	sort.Slice(skipped, func(i, j int) bool {
		if skipped[i].What != skipped[j].What {
			return skipped[i].What < skipped[j].What
		}
		return skipped[i].Why < skipped[j].Why
	})
	for _, s := range skipped {
		fmt.Fprintf(stdout, "not in the graph: %s: %s\n", s.What, s.Why)
	}
	if *check {
		disk, err := os.ReadFile(*out)
		same := err == nil && string(disk) == string(text)
		if same {
			fmt.Fprintln(stdout, "up to date")
			return 0
		}
		fmt.Fprintln(stdout, "OUT OF DATE: run go run ./tools/conceptgraph")
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(*out, text, 0o644); err != nil {
		return fail(err)
	}
	return 0
}
