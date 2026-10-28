// Command rulecheck checks a Battle Companies rules file of the lean format (schemaVersion 2) the way the upload does, and prints
// every problem: file, path, message, closest match and hint. Exit status 0: no errors (warnings are printed); 1: the file has
// errors; 2: the command could not run (unreadable file, wrong arguments).
//
//	go run ./cmd/rulecheck [-vanilla FILE] [-glossary FILE] battle_companies.json
//	make check-ruleset FILE=battle_companies.json
//
// The vanilla file and the glossary default to the ones the app ships with (definitions/).
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("rulecheck", flag.ContinueOnError)
	fs.SetOutput(stderr)
	vanillaPath := fs.String("vanilla", "definitions/vanilla/data2024.json", "the vanilla rules file the file is checked against")
	glossaryPath := fs.String("glossary", "definitions/glossary/mesbg_glossary.json", "the glossary the file is checked against")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: rulecheck [-vanilla FILE] [-glossary FILE] battle_companies.json")
		return 2
	}
	read := func(path string) ([]byte, bool) {
		b, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(stderr, "rulecheck:", err)
			return nil, false
		}
		return b, true
	}
	bc, ok := read(fs.Arg(0))
	if !ok {
		return 2
	}
	vanilla, ok := read(*vanillaPath)
	if !ok {
		return 2
	}
	glossary, ok := read(*glossaryPath)
	if !ok {
		return 2
	}
	rep, err := core.CheckBattleCompaniesV2(filepath.Base(fs.Arg(0)), bc, vanilla, glossary)
	if len(rep.Issues()) == 0 && err != nil { // the lower layers could not be read
		fmt.Fprintln(stderr, "rulecheck:", err)
		return 2
	}
	for _, i := range rep.Issues() {
		fmt.Fprintln(stdout, i.String())
	}
	if rep.Truncated() > 0 {
		fmt.Fprintf(stdout, "and %d more\n", rep.Truncated())
	}
	errs, warns := rep.Errors(), len(rep.Warnings())
	fmt.Fprintf(stdout, "%s: %d %s, %d %s\n", filepath.Base(fs.Arg(0)), errs, noun(errs, "error"), warns, noun(warns, "warning"))
	if errs > 0 {
		return 1
	}
	return 0
}

func noun(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
