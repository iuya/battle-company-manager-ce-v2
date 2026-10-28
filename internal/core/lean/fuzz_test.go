package lean

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// FuzzDecode feeds arbitrary bytes to the decoder. Invariants: no panic, no hang (2 s per input), the same bytes give the same
// report, and a document that decoded without errors, written back with Encode and decoded again, comes out equal. The seeds are
// every fixture and the shipped (old format) file, plus a depth bomb and a wide file; they run as ordinary tests in `make test`,
// `make fuzz` explores for a minute.
func FuzzDecode(f *testing.F) {
	for _, pattern := range []string{"testdata/valid/*.json", "testdata/invalid/*.json"} {
		files, _ := filepath.Glob(pattern)
		for _, file := range files {
			raw, err := os.ReadFile(file)
			if err != nil {
				f.Fatal(err)
			}
			f.Add(raw)
		}
	}
	if shipped, err := os.ReadFile("../../../definitions/battle_companies/battle_companies.json"); err == nil {
		f.Add(shipped)
	}
	f.Add([]byte(strings.Repeat("[", 5000)))
	f.Add([]byte(`{"schemaVersion":2,"x":` + strings.Repeat(`{"a":`, 200) + `1` + strings.Repeat("}", 200) + `}`))
	f.Add([]byte(`{"schemaVersion":2,"rules":[` + strings.Repeat(`{"name":"r"},`, 3000) + `{"name":"z"}]}`))
	f.Add([]byte(`{"schemaVersion":2,"name":"\u0000\ud800","version":1e999}`))
	f.Add([]byte{})
	f.Add([]byte("\xef\xbb\xbf{}"))

	lower := newFakeLower()
	f.Fuzz(func(t *testing.T, raw []byte) {
		start := time.Now()
		doc, ix, rep := decode("f.json", raw, lower)
		if d := time.Since(start); d > 2*time.Second {
			t.Fatalf("decoding took %v", d)
		}
		if (doc == nil) != (ix == nil) {
			t.Fatal("document and index come together")
		}
		_, _, again := decode("f.json", raw, lower)
		if !reflect.DeepEqual(rep, again) || rep.String() != again.String() {
			t.Fatal("the same bytes gave two reports")
		}
		for _, i := range rep.Issues {
			if i.File != "f.json" || i.Message == "" {
				t.Fatalf("malformed issue %+v", i)
			}
		}
		if len(rep.Issues) > MaxIssues {
			t.Fatalf("%d issues kept", len(rep.Issues))
		}
		if doc == nil || len(rep.Errors()) > 0 {
			return
		}
		out, err := Encode(doc)
		if err != nil {
			t.Fatalf("a clean document does not encode: %v", err)
		}
		doc2, _, rep2 := decode("f.json", out, lower)
		if len(rep2.Errors()) > 0 {
			t.Fatalf("the re-encoded document has errors:\n%s\nfrom %s", rep2, bytes.TrimSpace(out))
		}
		if !reflect.DeepEqual(doc, doc2) {
			t.Fatalf("encode and decode changed the document:\n%s", bytes.TrimSpace(out))
		}
	})
}
