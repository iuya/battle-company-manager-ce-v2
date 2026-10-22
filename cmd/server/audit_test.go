package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAudit(t *testing.T) {
	var files [3][]byte
	for i, name := range []string{"vanilla/data2024.json", "battle_companies/battle_companies.json", "glossary/mesbg_glossary.json"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "definitions", name))
		if err != nil {
			t.Fatal(err)
		}
		files[i] = b
	}
	dir := t.TempDir()
	if err := writeAudit(dir, files[0], files[1], files[2]); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.md", "minas-tirith.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s not written: %v", name, err)
		}
	}
}
