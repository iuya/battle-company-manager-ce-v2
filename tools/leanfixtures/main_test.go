package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const committed = "../../internal/core/lean/testdata"

func generateInto(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	outDir, invDir = filepath.Join(dir, "valid"), filepath.Join(dir, "invalid")
	for _, d := range []string{outDir, invDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	generate()
	return dir
}

func TestGeneratedFixturesMatchCommitted(t *testing.T) {
	dir := generateInto(t)
	for _, sub := range []string{"valid", "invalid"} {
		entries, err := os.ReadDir(filepath.Join(dir, sub))
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) < 19 {
			t.Fatalf("%s: only %d files generated", sub, len(entries))
		}
		for _, e := range entries {
			rel := sub + "/" + e.Name()
			got, _ := os.ReadFile(filepath.Join(dir, rel))
			want, err := os.ReadFile(filepath.Join(committed, rel))
			if err != nil {
				t.Errorf("%s is generated but not committed: %v", rel, err)
				continue
			}
			if string(got) != string(want) {
				t.Errorf("%s differs from the committed fixture", rel)
			}
		}
	}
}

func TestGenerationIsDeterministic(t *testing.T) {
	a, b := generateInto(t), generateInto(t)
	for _, rel := range []string{"valid/company.json", "invalid/many_mistakes.json", "invalid/syntax_error.json"} {
		x, _ := os.ReadFile(filepath.Join(a, rel))
		y, _ := os.ReadFile(filepath.Join(b, rel))
		if len(x) == 0 || string(x) != string(y) {
			t.Errorf("%s: not deterministic or empty", rel)
		}
	}
}

func TestOrderedJSONStyle(t *testing.T) {
	o := O("b", 1, "a", L(), "c", O(), "d", "tab\t\"q\" <é>\u007f\x01")
	o.Set("b", 2) // keeps its place
	want := "{\n  \"b\": 2,\n  \"a\": [],\n  \"c\": {},\n  \"d\": \"tab\\t\\\"q\\\" <é>\u007f\\u0001\"\n}"
	if got := dumps(o, true); got != want {
		t.Errorf("got %q want %q", got, want)
	}
	if got := dumpsASCII("é😀"); got != "\"\\u00e9\\ud83d\\ude00\"" {
		t.Errorf("ASCII escaping: %s", got)
	}
}

func TestPathHelpers(t *testing.T) {
	d := O("x", O("y", L(O("z", 1))))
	Put(d, 5, "x", "y", 0, "z")
	Append(d, 7, "x", "y")
	if got := dumps(d, true); !strings.Contains(got, "\"z\": 5") || !strings.Contains(got, "7") {
		t.Errorf("unexpected %s", got)
	}
	Del(d, "x", "y")
	if len(At(d, "x").(*Obj).keys) != 0 {
		t.Error("key not deleted")
	}
	c := DeepCopy(d).(*Obj)
	Put(c, 1, "x", "k")
	if len(At(d, "x").(*Obj).keys) != 0 {
		t.Error("deep copy shares structure")
	}
}

func TestMissingKeysPanic(t *testing.T) {
	for name, f := range map[string]func(){
		"At":  func() { At(O("a", 1), "b") },
		"Del": func() { Del(O("a", 1), "b") },
		"Idx": func() { At(L(1), 3) },
		"Enc": func() { dumps(1.5, true) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s did not panic", name)
				}
			}()
			f()
		}()
	}
}
