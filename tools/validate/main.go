// Command validate checks the definitions files; exit status 1 when there is an error (`make validate`).
//
//		go run ./tools/validate [-vanilla F] [-battle-companies F] [-glossary F] [-concept-graph F] [-schema F] [-tts-models F]
//
//	  - JSON Schema (Draft 2020-12) of the vanilla file, the glossary, the concept graph and the lean Battle Companies file
//	    (battle_companies.schema.json, generated from the Go model; it must also accept the Go valid fixtures).
//	  - The lean Battle Companies file is checked by the app's own validator, core.CheckBattleCompaniesV2: the code the upload runs and
//	    `go run ./cmd/rulecheck` prints. It owns the cross references (every unit, rule, power, item and path name resolves against the
//	    file, vanilla and the glossary), the closed vocabularies, the charts and the engine parameter ranges (docs/definitions.md). Its
//	    warnings stay warnings.
//	  - What that validator does not check (tools/internal/leancheck): the company texts (repeated rules, parse artefacts), the aura
//	    radii against the texts that name them, the keywords of the rule targets, the Tabletop Simulator models against the local
//	    catalogue.
//	  - The glossary (terms, see links, the To Wound Chart) and the concept graph (nodes and hashes against the three files).
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/conceptgraph"
	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/glossaryref"
	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/jsonx"
	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/leancheck"
	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/repo"
	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/schemacheck"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// withSuffix is Path.with_suffix: the last extension replaced.
func withSuffix(p, suffix string) string { return strings.TrimSuffix(p, filepath.Ext(p)) + suffix }

func schemaErrors(doc any, schemaPath string) []string {
	s, err := schemacheck.Compile(schemaPath)
	if err != nil {
		return []string{fmt.Sprintf("%s: not a valid schema: %s", filepath.Base(schemaPath), jsonx.Slice(err.Error(), 200))}
	}
	return schemacheck.Errors(doc, s)
}

// leanSchemaErrors: the schema of the lean format is generated from the Go model (go test ./internal/core/lean -run TestSchemaUpToDate
// -update-schema); here it must be a valid Draft 2020-12 schema and accept every valid fixture of the Go decoder.
func leanSchemaErrors(schemaPath, fixtures string) []string {
	s, err := schemacheck.Compile(schemaPath)
	if err != nil {
		return []string{fmt.Sprintf("%s: not a valid schema: %s", filepath.Base(schemaPath), jsonx.Slice(err.Error(), 200))}
	}
	files, _ := filepath.Glob(filepath.Join(fixtures, "*.json"))
	sort.Strings(files)
	var errors []string
	for _, f := range files {
		doc, err := jsonx.ReadFile(f)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s vs fixture %s: %v", filepath.Base(schemaPath), filepath.Base(f), err))
			continue
		}
		for _, e := range schemacheck.Errors(doc, s) {
			errors = append(errors, fmt.Sprintf("%s vs fixture %s: %s", filepath.Base(schemaPath), filepath.Base(f), e))
		}
	}
	return errors
}

// rulecheck runs the app's validator on the Battle Companies file and returns its errors and warnings as printed blocks.
func rulecheck(bcPath string, bc, vanilla, glossary []byte) (errors, warnings []string) {
	rep, err := core.CheckBattleCompaniesV2(filepath.Base(bcPath), bc, vanilla, glossary)
	if len(rep.Issues()) == 0 && err != nil { // the lower layers could not be read
		return []string{"the validator could not run: " + jsonx.Slice(err.Error(), 400)}, nil
	}
	for _, i := range rep.Issues() {
		s := i.String()
		if strings.HasPrefix(s, "warn: ") {
			warnings = append(warnings, strings.TrimPrefix(s, "warn: "))
		} else {
			errors = append(errors, s)
		}
	}
	if rep.Truncated() > 0 {
		errors = append(errors, fmt.Sprintf("and %d more", rep.Truncated()))
	}
	if rep.Errors() > 0 && len(errors) == 0 {
		errors = append(errors, "the validator reports errors but none could be read from its report")
	}
	return errors, warnings
}

// floats turns the json.Number of a decoded value into float64, which glossaryref reads.
func floats(v any) any {
	switch x := v.(type) {
	case jsonx.Obj:
		o := make(jsonx.Obj, len(x))
		for k, e := range x {
			o[k] = floats(e)
		}
		return o
	case jsonx.Arr:
		a := make(jsonx.Arr, len(x))
		for i, e := range x {
			a[i] = floats(e)
		}
		return a
	}
	if jsonx.IsNum(v) {
		return jsonx.Float(v)
	}
	return v
}

func run(args []string, stdout, stderr io.Writer) int {
	root := repo.Root()
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	vanilla := fs.String("vanilla", filepath.Join(root, "definitions/vanilla/data2024.json"), "the vanilla rules file")
	bcFlag := fs.String("battle-companies", filepath.Join(root, "definitions/battle_companies/battle_companies.json"), "the Battle Companies file")
	glossaryFlag := fs.String("glossary", filepath.Join(root, "definitions/glossary/mesbg_glossary.json"), "the glossary")
	tts := fs.String("tts-models", filepath.Join(root, "sources/tts/models.json"), "local catalogue of the mod's model keys (optional)")
	schema := fs.String("schema", filepath.Join(root, "definitions/battle_companies/battle_companies.schema.json"), "the schema of the Battle Companies file")
	graphFlag := fs.String("concept-graph", filepath.Join(root, "definitions/concepts/concept_graph.json"), "the concept graph")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	fail := func(err error) int {
		fmt.Fprintln(stderr, "validate:", err)
		return 1
	}
	vp, bp, gp := *vanilla, *bcFlag, *glossaryFlag
	vv, err := jsonx.ReadFile(vp)
	if err != nil {
		return fail(err)
	}
	bv, err := jsonx.ReadFile(bp)
	if err != nil {
		return fail(err)
	}
	van, bc := jsonx.O(vv), jsonx.O(bv)
	var glossary jsonx.Obj
	if exists(gp) {
		gv, err := jsonx.ReadFile(gp)
		if err != nil {
			return fail(err)
		}
		glossary = jsonx.O(gv)
	}
	var errors, warnings []string

	// schemas
	type check struct {
		doc          any
		path, schema string
	}
	checks := []check{{vv, vp, withSuffix(vp, ".schema.json")}, {bv, bp, *schema}}
	if jsonx.Truthy(glossary) {
		checks = append(checks, check{glossary, gp, withSuffix(gp, ".schema.json")})
	}
	for _, c := range checks {
		if exists(c.schema) {
			for _, e := range schemaErrors(c.doc, c.schema) {
				errors = append(errors, fmt.Sprintf("%s: %s", filepath.Base(c.path), e))
			}
		} else {
			errors = append(errors, c.schema+" is missing")
		}
	}
	if exists(*schema) {
		errors = append(errors, leanSchemaErrors(*schema, filepath.Join(root, "internal/core/lean/testdata/valid"))...)
	}
	if !jsonx.Equal(bc["schemaVersion"], jsonx.Number("2")) {
		errors = append(errors, fmt.Sprintf("%s: schemaVersion is %s, the lean format is 2", filepath.Base(bp), jsonx.Repr(bc["schemaVersion"])))
	}
	if !jsonx.Equal(bc["extends"], van["game"]) {
		errors = append(errors, fmt.Sprintf("extends=%s does not match vanilla game %s", jsonx.Repr(bc["extends"]), jsonx.Repr(van["game"])))
	}

	// the app's validator: cross references, vocabularies, charts, engine parameters
	rawV, rawB := mustRead(vp), mustRead(bp)
	var rawG []byte
	if exists(gp) {
		rawG = mustRead(gp)
	}
	e, w := rulecheck(bp, rawB, rawV, rawG)
	errors = append(errors, e...)
	warnings = append(warnings, w...)
	loads := len(e) == 0 // the nodes of the graph come from the app's loader, which refuses a file with errors

	// what it does not check
	errors = append(errors, leancheck.CompanyTextErrors(bc)...)
	errors = append(errors, leancheck.RuleTargetErrors(bc, van)...)
	cat, err := leancheck.TTSCatalogue(*tts)
	if err != nil {
		return fail(err)
	}
	errors = append(errors, leancheck.TTSErrors(bc, cat)...)
	var raws *[3][]byte
	if jsonx.Truthy(glossary) && loads {
		raws = &[3][]byte{rawV, rawB, rawG}
		nodes, err := conceptgraph.MakeNodes(*raws)
		if err != nil {
			errors = append(errors, err.Error())
		} else {
			errors = append(errors, leancheck.AuraErrors(bc, conceptgraph.NewIndex(nodes).Texts, van)...)
		}
	}

	// the glossary
	known := map[string]bool{}
	if jsonx.Truthy(glossary) {
		terms := jsonx.A(glossary["terms"])
		count := map[string]int{}
		for _, t := range terms {
			count[jsonx.S(jsonx.O(t)["name"])]++
		}
		var dups []string
		for n, c := range count {
			if c > 1 {
				dups = append(dups, n)
			}
		}
		sort.Strings(dups)
		for _, n := range dups {
			errors = append(errors, fmt.Sprintf("duplicate glossary term %s", jsonx.ReprString(n)))
		}
		for n := range count {
			known[n] = true
		}
		for _, ti := range terms {
			t := jsonx.O(ti)
			name := jsonx.S(t["name"])
			if strings.TrimSpace(jsonx.S(t["definition"])) == "" {
				errors = append(errors, fmt.Sprintf("glossary term %s is empty", jsonx.ReprString(name)))
			}
			for _, target := range jsonx.Strs(t["see"]) {
				if !known[target] {
					errors = append(errors, fmt.Sprintf("glossary term %s: see -> %s is not a term", jsonx.ReprString(name), jsonx.ReprString(target)))
				}
			}
		}
		tw := jsonx.O(jsonx.O(glossary["reference"])["toWound"])
		if tw == nil {
			errors = append(errors, "glossary: reference.toWound is missing")
		} else {
			errors = append(errors, glossaryref.ToWoundErrorsJSON(floats(tw).(jsonx.Obj))...)
			if term, ok := tw["term"].(string); !ok || !known[term] {
				errors = append(errors, fmt.Sprintf("glossary reference.toWound: term %s is not a term", jsonx.Repr(tw["term"])))
			}
		}
	}

	// the concept graph
	cp := *graphFlag
	switch {
	case jsonx.Truthy(glossary) && exists(cp) && raws != nil:
		gv, err := jsonx.ReadFile(cp)
		if err != nil {
			return fail(err)
		}
		gs := withSuffix(cp, ".schema.json")
		if !exists(gs) {
			errors = append(errors, fmt.Sprintf("%s: %s is missing", filepath.Base(cp), gs))
		} else {
			for _, x := range schemaErrors(gv, gs) {
				errors = append(errors, fmt.Sprintf("%s: %s", filepath.Base(cp), x))
			}
		}
		errs, err := conceptgraph.CheckGraph(jsonx.O(gv), *raws)
		if err != nil {
			errors = append(errors, err.Error())
		}
		for _, x := range errs {
			errors = append(errors, fmt.Sprintf("%s: %s", filepath.Base(cp), x))
		}
	case jsonx.Truthy(glossary) && !exists(cp):
		errors = append(errors, cp+" is missing: run go run ./tools/conceptgraph")
	}

	uniq := map[string]bool{}
	var ws []string
	for _, x := range warnings {
		if !uniq[x] {
			uniq[x] = true
			ws = append(ws, x)
		}
	}
	sort.Strings(ws)
	for _, x := range ws {
		fmt.Fprintln(stdout, "warning:", x)
	}
	for _, x := range errors {
		fmt.Fprintln(stdout, "ERROR:", x)
	}
	fmt.Fprintf(stdout, "%d error(s), %d warning(s)\n", len(errors), len(ws))
	if len(errors) > 0 {
		return 1
	}
	return 0
}

func mustRead(p string) []byte {
	b, err := os.ReadFile(p)
	if err != nil {
		fmt.Fprintln(os.Stderr, "validate:", err)
		os.Exit(1)
	}
	return b
}
