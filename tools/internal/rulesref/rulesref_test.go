package rulesref

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func root(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

func schemaBytes(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root(t), "definitions/battle_companies/battle_companies.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// The committed docs/customizing-rulesets-reference.md is what the generator makes from the schema.
func TestCommittedReferenceIsCurrent(t *testing.T) {
	have, err := os.ReadFile(filepath.Join(root(t), "docs/customizing-rulesets-reference.md"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Render(schemaBytes(t))
	if err != nil {
		t.Fatal(err)
	}
	if string(have) != got {
		t.Fatal("docs/customizing-rulesets-reference.md is stale: run `make ruleset-reference`")
	}
}

func TestEveryKeyOfTheSchemaIsListed(t *testing.T) {
	raw := schemaBytes(t)
	text, err := Render(raw)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := parse(raw)
	defs := v.(*obj).obj("$defs")
	for _, name := range defs.keys {
		props := defs.obj(name).obj("properties")
		if props == nil {
			continue
		}
		for _, key := range props.keys {
			if !strings.Contains(text, "| `"+key+"` |") {
				t.Errorf("%s.%s is missing from the reference", name, key)
			}
		}
	}
}

func TestOutputIsDeterministic(t *testing.T) {
	raw := schemaBytes(t)
	a, _ := Render(raw)
	b, _ := Render(raw)
	if a != b {
		t.Fatal("two renders of the same schema differ")
	}
}

// A small fake schema (no rules data) pins the layout and the formatting rules.
const fake = `{
 "description": "A fake file. Generated from the model. Never edit it.",
 "required": ["name"],
 "properties": {
  "name": {"type": "string", "minLength": 1, "description": "The name"},
  "size": {"type": "integer", "minimum": 1, "maximum": 5, "default": 2},
  "tags": {"type": "array", "items": {"type": "string", "enum": ["a", "b"]}, "minItems": 1},
  "box": {"anyOf": [{"$ref": "#/$defs/Box"}, {"type": "null"}]}
 },
 "$defs": {
  "Box": {"description": "A box.", "type": "object", "properties": {"w": {"type": "number", "description": "pipe | here"}}}
 }
}`

func TestFakeSchemaLayout(t *testing.T) {
	text, err := Render([]byte(fake))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"A fake file.\n\nThe file is one JSON object.",
		"| `name` | string | yes | - | not empty | The name. |",
		"| `size` | integer | no | `2` | 1..5 |  |",
		"| `tags` | list of string | no | - | `a`, `b`; at least 1 items |  |",
		"| `box` | [Box](#box) or null | no | - | - |  |",
		"| `w` | number | no | - | - | pipe \\| here. |",
		"Found at: `box`.",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}
	if strings.Contains(text, "Never edit") || strings.Contains(text, "Generated from the model") {
		t.Error("the maintainer note was not dropped")
	}
}

func TestMalformedSchemasAreErrorsNotPanics(t *testing.T) {
	for name, in := range map[string]string{
		"not json":       `{`,
		"trailing data":  `{"$defs": {}} 1`,
		"not an object":  `[]`,
		"no defs":        `{"properties": {}}`,
		"dangling ref":   `{"properties": {"a": {"$ref": "#/$defs/Missing"}}, "$defs": {}}`,
		"empty anyOf":    `{"properties": {"a": {"anyOf": [{"type": "null"}]}}, "$defs": {}}`,
		"bad properties": `{"properties": {"a": 5}, "$defs": {}}`,
	} {
		if _, err := Render([]byte(in)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestSentenceHelpers(t *testing.T) {
	if got := sentence("  one   two\n"); got != "one two." {
		t.Errorf("sentence: %q", got)
	}
	if got := sentence("Done?"); got != "Done?" {
		t.Errorf("sentence: %q", got)
	}
	if got := sentence(""); got != "" {
		t.Errorf("sentence: %q", got)
	}
	if got := splitSentences("One. Two  three. Four"); len(got) != 3 || got[1] != "Two  three." {
		t.Errorf("split: %q", got)
	}
	if got := anchor("Value Forms (x)"); got != "value-forms-x" {
		t.Errorf("anchor: %q", got)
	}
}
