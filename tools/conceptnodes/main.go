// Command conceptnodes prints, as JSON, what the app's loader makes of the three shipped rules files (vanilla, Battle Companies,
// glossary) and the concept graph needs: "terms", one object per glossary term (id, name, category, aliases, source and the text the
// graph hashes), and "gearClasses", the classes of every item of the Battle Companies file with the defaults the loader derives from
// the vanilla type (a bow, a shield...), which the file itself leaves out.
//
//	go run ./tools/conceptnodes [vanilla.json battle_companies.json glossary.json]
//
// tools/concept_graph.py reads this to make the nodes of the concept graph: the text of a term is built by the app's own loader
// (core.LoadRuleSet), so the curator's tools never repeat that logic. Without arguments it reads the files under definitions/.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

type node struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Category string   `json:"category"`
	Aliases  []string `json:"aliases,omitempty"`
	Source   string   `json:"source"`
	Text     string   `json:"text"`
}

func main() {
	paths := []string{"definitions/vanilla/data2024.json", "definitions/battle_companies/battle_companies.json", "definitions/glossary/mesbg_glossary.json"}
	if len(os.Args) == 4 {
		paths = os.Args[1:]
	} else if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: conceptnodes [vanilla.json battle_companies.json glossary.json]")
		os.Exit(2)
	}
	var files [3][]byte
	for i, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, "conceptnodes:", err)
			os.Exit(2)
		}
		files[i] = b
	}
	rs, err := core.LoadRuleSet(files[0], files[1], files[2], nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "conceptnodes:", err)
		os.Exit(1)
	}
	if rs.Doc == nil {
		fmt.Fprintln(os.Stderr, "conceptnodes: the Battle Companies file is not a lean (schemaVersion 2) file")
		os.Exit(1)
	}
	out := struct {
		Terms       []node              `json:"terms"`
		GearClasses map[string][]string `json:"gearClasses"`
	}{Terms: make([]node, 0, len(rs.Glossary.Terms)), GearClasses: map[string][]string{}}
	for _, t := range rs.Glossary.Terms {
		out.Terms = append(out.Terms, node{ID: t.ID, Name: t.Name, Category: t.Category, Aliases: t.Aliases, Source: t.Source, Text: t.Text()})
	}
	// Keyed by the lower-cased name of each gear item as the file spells it (what the Python side looks up); the annotations are
	// keyed by core.NameKey, so a name with a hyphen or an accent ("Uruk-Hai Bow") is looked up through it.
	ann := core.LeanGear(rs.Doc, rs.Vanilla).Annotations
	for _, g := range rs.Doc.Gear {
		for _, c := range ann[core.NameKey(g.Name)].Classes {
			k := strings.ToLower(g.Name)
			out.GearClasses[k] = append(out.GearClasses[k], string(c))
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, "conceptnodes:", err)
		os.Exit(1)
	}
}
