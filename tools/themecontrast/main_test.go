package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRatio(t *testing.T) {
	if r := ratio("#000000", "#ffffff"); r < 20.99 || r > 21.01 {
		t.Fatal(r)
	}
	if r := ratio("#777777", "#777777"); r != 1 {
		t.Fatal(r)
	}
}

func TestShippedPalettesPass(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run("../../tailwind.css", &out, &errOut); code != 0 || !strings.Contains(out.String(), "12 palette/mode combinations, 0 failure(s)") {
		t.Fatalf("%d\n%s%s", code, out.String(), errOut.String())
	}
}

func TestALowContrastPaletteFails(t *testing.T) {
	css := `[data-mode="light"] { --leader: #000000; }
[data-palette="p"][data-mode="light"] { --fg: #777777; --page: #808080; --surface: #808080; --raised: #808080; }`
	p := filepath.Join(t.TempDir(), "t.css")
	if err := os.WriteFile(p, []byte(css), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := run(p, &out, &errOut)
	// the palette lacks most tokens: the tool refuses rather than guessing
	if code != 1 || !strings.Contains(errOut.String(), "missing") {
		t.Fatalf("%d\n%s%s", code, out.String(), errOut.String())
	}
}
