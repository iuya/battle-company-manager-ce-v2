package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/jsonx"
	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/repo"
)

// Tests of the validator: it runs on a temporary copy of the shipped files, changed in one place.

var shipped = map[string]string{
	"vanilla":    "definitions/vanilla/data2024.json",
	"battle":     "definitions/battle_companies/battle_companies.json",
	"glossary":   "definitions/glossary/mesbg_glossary.json",
	"graph":      "definitions/concepts/concept_graph.json",
	"bcSchema":   "definitions/battle_companies/battle_companies.schema.json",
	"vanSchema":  "definitions/vanilla/data2024.schema.json",
	"glosSchema": "definitions/glossary/mesbg_glossary.schema.json",
	"graphSchem": "definitions/concepts/concept_graph.schema.json",
}

// validate copies the files, lets change edit the decoded document called which, runs the validator and returns the exit status and output.
func validate(t *testing.T, which string, change func(doc jsonx.Obj)) (int, string) {
	t.Helper()
	dir, root := t.TempDir(), repo.Root()
	paths := map[string]string{}
	for k, p := range shipped {
		raw, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			t.Fatal(err)
		}
		if k == which {
			v, err := jsonx.Decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			change(jsonx.O(v))
			raw = jsonx.Marshal(jsonx.O(v))
		}
		paths[k] = filepath.Join(dir, filepath.Base(p))
		if err := os.WriteFile(paths[k], raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"-vanilla", paths["vanilla"], "-battle-companies", paths["battle"], "-glossary", paths["glossary"], "-concept-graph", paths["graph"],
		"-schema", paths["bcSchema"], "-tts-models", filepath.Join(dir, "none.json")}, &out, &errOut)
	return code, out.String() + errOut.String()
}

func TestShippedFilesHaveNoErrors(t *testing.T) {
	code, out := validate(t, "", nil)
	if code != 0 || !strings.Contains(out, "0 error(s)") {
		t.Fatalf("%d\n%s", code, out)
	}
}

func TestRunWithoutArgumentsChecksTheShippedFiles(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run(nil, &out, &errOut); code != 0 || !strings.Contains(out.String(), "0 error(s)") {
		t.Fatalf("%d\n%s%s", code, out.String(), errOut.String())
	}
}

func TestRunRejectsAnUnknownFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"-nope"}, &out, &errOut); code != 2 {
		t.Fatal(code)
	}
}

func TestADanglingUnitNameIsAnError(t *testing.T) {
	code, out := validate(t, "battle", func(bc jsonx.Obj) {
		c := jsonx.O(jsonx.A(bc["companies"])[0])
		u := jsonx.O(jsonx.A(jsonx.O(c["start"])["units"])[0])
		u["unit"] = "Warrior of Nowhere"
	})
	if code != 1 || !strings.Contains(out, "Warrior of Nowhere") {
		t.Fatalf("%d\n%s", code, out)
	}
}

func TestAnUnknownKeyIsAnError(t *testing.T) {
	code, out := validate(t, "battle", func(bc jsonx.Obj) { jsonx.O(jsonx.A(bc["rules"])[0])["colour"] = "red" })
	if code != 1 || !strings.Contains(out, "colour") {
		t.Fatalf("%d\n%s", code, out)
	}
}

func TestAMissingRequiredKeyIsAnError(t *testing.T) {
	code, out := validate(t, "battle", func(bc jsonx.Obj) { delete(jsonx.O(jsonx.A(bc["companies"])[0]), "name") })
	if code != 1 || !strings.Contains(out, "name") {
		t.Fatalf("%d\n%s", code, out)
	}
}

func TestAWrongSchemaVersionOrExtendsIsAnError(t *testing.T) {
	code, out := validate(t, "battle", func(bc jsonx.Obj) { bc["schemaVersion"] = jsonx.Number("3") })
	if code != 1 || !strings.Contains(out, "schemaVersion is 3, the lean format is 2") {
		t.Fatalf("%d\n%s", code, out)
	}
	code, out = validate(t, "battle", func(bc jsonx.Obj) { bc["extends"] = "Other Game" })
	if code != 1 || !strings.Contains(out, "extends='Other Game' does not match vanilla game") {
		t.Fatalf("%d\n%s", code, out)
	}
}

func TestACompanyTextAndAnAuraRadiusAreErrors(t *testing.T) {
	code, out := validate(t, "battle", func(bc jsonx.Obj) {
		for _, c := range jsonx.A(bc["companies"]) {
			if co := jsonx.O(c); co["name"] == "Fiefdoms" {
				co["specialRules"] = append(jsonx.A(co["specialRules"]), jsonx.A(co["specialRules"])[0])
			}
		}
	})
	if code != 1 || !strings.Contains(out, "repeat the same text") {
		t.Fatalf("%d\n%s", code, out)
	}
	code, out = validate(t, "battle", func(bc jsonx.Obj) {
		for _, r := range jsonx.A(bc["rules"]) {
			if ro := jsonx.O(r); ro["name"] == "Furious Tirade" {
				jsonx.O(jsonx.A(ro["auras"])[0])["radius"] = jsonx.Number("8")
			}
		}
	})
	if code != 1 || !strings.Contains(out, "Furious Tirade") {
		t.Fatalf("%d\n%s", code, out)
	}
}

func TestAStaleGraphExitsWithOne(t *testing.T) {
	code, out := validate(t, "graph", func(g jsonx.Obj) { jsonx.O(g["builtFrom"])["glossary"] = strings.Repeat("0", 64) })
	if code != 1 || !strings.Contains(out, "builtFrom.glossary is stale") {
		t.Fatalf("%d\n%s", code, out)
	}
}

func TestGlossaryChecks(t *testing.T) {
	for _, c := range []struct {
		name, want string
		change     func(g jsonx.Obj)
	}{
		{"duplicate term", "duplicate glossary term", func(g jsonx.Obj) { g["terms"] = append(jsonx.A(g["terms"]), jsonx.A(g["terms"])[0]) }},
		{"empty definition", "is empty", func(g jsonx.Obj) { jsonx.O(jsonx.A(g["terms"])[0])["definition"] = "  " }},
		{"dangling see", "is not a term", func(g jsonx.Obj) { jsonx.O(jsonx.A(g["terms"])[0])["see"] = jsonx.Arr{"No Such"} }},
		{"to wound cell", "values outside the allowed set", func(g jsonx.Obj) {
			jsonx.A(jsonx.A(jsonx.O(jsonx.O(g["reference"])["toWound"])["cells"])[0])[0] = "9+"
		}},
		{"to wound order", "is easier than", func(g jsonx.Obj) {
			jsonx.A(jsonx.A(jsonx.O(jsonx.O(g["reference"])["toWound"])["cells"])[3])[0] = "6+"
		}},
		{"to wound missing", "reference.toWound is missing", func(g jsonx.Obj) { delete(jsonx.O(g["reference"]), "toWound") }},
		{"to wound term", "toWound: term 'Nope' is not a term", func(g jsonx.Obj) { jsonx.O(jsonx.O(g["reference"])["toWound"])["term"] = "Nope" }},
	} {
		t.Run(c.name, func(t *testing.T) {
			code, out := validate(t, "glossary", c.change)
			if code != 1 || !strings.Contains(out, c.want) {
				t.Fatalf("%d\n%s", code, out)
			}
		})
	}
}

func TestAMissingGraphIsAnError(t *testing.T) {
	dir, root := t.TempDir(), repo.Root()
	var out, errOut bytes.Buffer
	code := run([]string{"-concept-graph", filepath.Join(dir, "none.json"), "-glossary", filepath.Join(root, shipped["glossary"])}, &out, &errOut)
	if code != 1 || !strings.Contains(out.String(), "is missing: run go run ./tools/conceptgraph") {
		t.Fatalf("%d\n%s", code, out.String())
	}
}
