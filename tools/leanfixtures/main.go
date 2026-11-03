// Command leanfixtures writes the fixtures of internal/core/lean/testdata: the valid files (testdata/valid) and the
// inputs of the invalid catalogue (testdata/invalid). The .want files next to the invalid inputs come from
// `go test ./internal/core/lean -update`, not from this program.
//
//	go run ./tools/leanfixtures [-dir internal/core/lean/testdata]
//
// The output is deterministic: objects keep the order in which their keys were set, and the JSON is indented by two spaces with
// json.dumps-style escaping (non-ASCII escaped).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

var outDir, invDir string

func main() {
	dir := flag.String("dir", "internal/core/lean/testdata", "the testdata directory (valid/ and invalid/ are written inside it)")
	flag.Parse()
	outDir = filepath.Join(*dir, "valid")
	invDir = filepath.Join(*dir, "invalid")
	for _, d := range []string{outDir, invDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			fail(err)
		}
	}
	generate()
	entries, err := os.ReadDir(outDir)
	if err != nil {
		fail(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	fmt.Println(names)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "leanfixtures:", err)
	os.Exit(1)
}

// w writes a valid fixture; wi writes an invalid input, from the document or, when raw is given, verbatim.
func w(name string, d any) {
	write(filepath.Join(outDir, name+".json"), dumps(d, true)+"\n")
}

func wi(name string, d any, raw ...string) {
	text := ""
	if len(raw) > 0 {
		text = raw[0]
	} else {
		text = dumps(d, true) + "\n"
	}
	write(filepath.Join(invDir, name+".json"), text)
}

func write(path, text string) {
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		fail(err)
	}
}

// doc is the smallest valid document, with the keyword arguments set on top of it.
func doc(kw ...*Obj) *Obj {
	d := O("schemaVersion", 2, "name", "Fixture", "version", "1.0", "extends", "Middle-Earth Strategy Battle Game - 2024",
		"campaign", DeepCopy(CAMPAIGN), "scenarios", DeepCopy(SCEN))
	for _, k := range kw {
		for _, key := range k.keys {
			d.Set(key, k.vals[key])
		}
	}
	return d
}

// pathRows is a path chart of 2d6 rows, each one {fight}, with some rows replaced (keyed by roll).
func pathRows(over IM) any {
	rows := Rng(2, 13, func(r int) any { return O("roll", r, "options", L(O("stat", "fight"))) })
	for k, v := range over {
		rows.items[k-2] = v
	}
	return rows
}
