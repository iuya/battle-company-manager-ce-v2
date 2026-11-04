// Command glossary builds definitions/glossary/mesbg_glossary.json from the rulebook converted
// to Markdown (the text of the rules PDF made with any PDF-to-text tool):
//
//	go run ./tools/glossary [--src sources/mesbg_rules.md] [--out F] [--summaries F] [--uncapped F]
//
// The rulebook is copyrighted: keep it under sources/ (gitignored). The resulting json may be
// committed.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/glossaryref"
)

func repoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			return dir
		}
	}
}

func main() {
	root := repoRoot()
	var o glossaryref.BuildOptions
	flag.StringVar(&o.Src, "src", filepath.Join(root, "sources/mesbg_rules.md"), "the rulebook converted to Markdown")
	flag.StringVar(&o.Out, "out", filepath.Join(root, "definitions/glossary/mesbg_glossary.json"), "the glossary file to write")
	flag.StringVar(&o.Summaries, "summaries", filepath.Join(root, "tools/glossary_summaries.json"), "the curated summaries of the long chapters")
	flag.StringVar(&o.Uncapped, "uncapped", "", "write the full verbatim text of every term to this json file (for the curating agent)")
	flag.Parse()
	res, err := glossaryref.BuildGlossary(o)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if o.Uncapped == "" {
		fmt.Printf("%d terms, %d KB of text\n", res.Terms, res.KB)
		for _, p := range res.Problems {
			fmt.Println("problem:", p)
		}
	}
}
