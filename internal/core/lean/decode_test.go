package lean

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

type panickingLower struct{ *fakeLower }

func (panickingLower) Has(Kind, string) bool { panic("boom") }

func TestDecodeUnusableAndVersion(t *testing.T) {
	for _, c := range []struct {
		name, src string
		nilDoc    bool
		want      string
	}{
		{"syntax", `{"a":`, true, "not valid JSON at line 1"},
		{"not an object", `[1,2]`, true, "expected an object, got a list"},
		{"version 1", `{"schemaVersion":1}`, true, "schemaVersion 1 is not supported (this app reads 2)"},
		{"version 3", `{"schemaVersion":3}`, true, "schemaVersion 3 is not supported"},
		{"version string", `{"schemaVersion":"2"}`, true, "expected the whole number 2, got a string"},
		{"version fraction", `{"schemaVersion":2.5}`, true, "expected the whole number 2"},
		{"version missing", `{}`, true, "schemaVersion: missing"},
		{"version 2 binds", `{"schemaVersion":2}`, false, "name: missing"},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc, ix, rep := Decode("bc.json", []byte(c.src), newFakeLower())
			if (doc == nil) != c.nilDoc || (ix == nil) != c.nilDoc {
				t.Errorf("doc %v index %v, nil expected %v", doc != nil, ix != nil, c.nilDoc)
			}
			if rep.OK() || !strings.Contains(rep.String(), c.want) {
				t.Errorf("report\n%s\nwant %q", rep, c.want)
			}
			for _, i := range rep.Issues {
				if i.File != "bc.json" {
					t.Errorf("issue without the file: %+v", i)
				}
			}
		})
	}
}

func TestDecodeValidFixtures(t *testing.T) {
	raw, err := os.ReadFile("testdata/valid/company.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, ix, rep := Decode("bc.json", raw, newFakeLower())
	if !rep.OK() || len(rep.Issues) != 0 || doc == nil || ix == nil {
		t.Fatal(rep)
	}
	c := doc.Companies[0]
	if c.ID != "arnor" || c.Faction != "Arnor" || c.MaxModels != 15 || c.Start.Units[0].Count != 4 {
		t.Errorf("the decoded document is normalised: %+v", c)
	}
	if e, ok := ix.Lookup(KindRule, "foresight of the seer", ""); !ok || e.Class != ClassNew {
		t.Errorf("index: %+v %v", e, ok)
	}
}

func TestDecodeIsDeterministic(t *testing.T) {
	raw, _ := os.ReadFile("testdata/invalid/param_dice.json")
	_, _, first := Decode("bc.json", raw, newFakeLower())
	for i := 0; i < 20; i++ {
		_, _, again := Decode("bc.json", raw, newFakeLower())
		if !reflect.DeepEqual(first, again) {
			t.Fatal("the same bytes gave a different report")
		}
	}
}

func TestDecodeRecoversFromPanics(t *testing.T) {
	raw, _ := os.ReadFile("testdata/valid/company.json")
	_, _, rep := Decode("bc.json", raw, panickingLower{newFakeLower()})
	if rep.OK() || !strings.Contains(rep.String(), "internal error: boom") {
		t.Fatalf("report\n%s", rep)
	}
}

func TestDecodeAllValidFixturesRoundTrip(t *testing.T) {
	// A clean document written back with Encode and decoded again is equal (the fuzz test relies on it).
	for _, name := range []string{"minimal", "company", "company_extends", "unit_forms", "gear", "path", "rules", "scenario", "vanilla_section", "limits_followers"} {
		raw, _ := os.ReadFile("testdata/valid/" + name + ".json")
		doc, _, rep := Decode("bc.json", raw, newFakeLower())
		if len(rep.Issues) != 0 {
			t.Fatalf("%s: %s", name, rep)
		}
		out, err := Encode(doc)
		if err != nil {
			t.Fatal(err)
		}
		again, _, rep2 := Decode("bc.json", out, newFakeLower())
		if len(rep2.Issues) != 0 {
			t.Fatalf("%s re-encoded: %s", name, rep2)
		}
		if !reflect.DeepEqual(doc, again) {
			t.Errorf("%s: encode and decode changed the document", name)
		}
	}
}

// bigDocument is the company fixture with n copies of its company under other names: a file of the size of the shipped one.
func bigDocument(tb testing.TB, n int) []byte {
	tb.Helper()
	raw, err := os.ReadFile("testdata/valid/company.json")
	if err != nil {
		tb.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		tb.Fatal(err)
	}
	companies := doc["companies"].([]any)
	base := companies[0].(map[string]any)
	for i := 1; i < n; i++ {
		c := map[string]any{}
		for k, v := range base {
			c[k] = v
		}
		c["name"] = fmt.Sprintf("Arnor %d", i)
		companies = append(companies, c)
	}
	doc["companies"] = companies
	out, _ := json.Marshal(doc)
	return out
}

func BenchmarkDecodeLarge(b *testing.B) {
	raw := bigDocument(b, 600)
	b.SetBytes(int64(len(raw)))
	b.ReportAllocs()
	lower := newFakeLower()
	for i := 0; i < b.N; i++ {
		if _, _, rep := Decode("bc.json", raw, lower); !rep.OK() {
			b.Fatal(rep)
		}
	}
}

func TestDecodeLargeDocumentIsClean(t *testing.T) {
	raw := bigDocument(t, 600)
	if len(raw) < 500_000 {
		t.Fatalf("only %d bytes", len(raw))
	}
	if _, _, rep := Decode("bc.json", raw, newFakeLower()); !rep.OK() {
		t.Fatal(rep)
	}
}
