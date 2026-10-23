package core

import (
	"os"
	"slices"
	"testing"
)

// The generated graph (tools/build_concept_graph.py) must describe the shipped triple the way core sees it: the Python port of the
// glossary build is checked here against the real one, node by node.
func TestShippedConceptGraphMatchesCore(t *testing.T) {
	read := func(path string) []byte {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	vanilla := read("../../definitions/vanilla/data2024.json")
	bc := read("../../definitions/battle_companies/battle_companies.json")
	glossary := read("../../definitions/glossary/mesbg_glossary.json")
	graphJSON := read("../../definitions/concepts/concept_graph.json")

	var h RulesetHashes
	for _, x := range []struct {
		to      *string
		content []byte
	}{{&h.Vanilla, vanilla}, {&h.BattleCompanies, bc}, {&h.Glossary, glossary}} {
		hash, err := RulesetHash(x.content)
		if err != nil {
			t.Fatal(err)
		}
		*x.to = hash
	}
	gl, err := BuildGlossaryWith(RulesetInfo{Kind: KindGlossary}, glossary, GlossaryExtras(vanilla, bc))
	if err != nil {
		t.Fatal(err)
	}
	rs := &RuleSet{Hashes: h, Glossary: gl}

	file, err := ParseConceptFile(graphJSON)
	if err != nil {
		t.Fatalf("ParseConceptFile refuses the generated file: %v", err)
	}
	if file.BuiltFrom.Vanilla != h.Vanilla || file.BuiltFrom.BattleCompanies != h.BattleCompanies || file.BuiltFrom.Glossary != h.Glossary {
		t.Errorf("builtFrom %+v is not the hashes of the shipped files %+v (run tools/build_concept_graph.py)", file.BuiltFrom, h)
	}

	want := map[string]*conceptNode{}
	for _, term := range gl.Terms {
		id := conceptID(conceptKind(term.Category), term.ID)
		want[id] = &conceptNode{id: id, kind: conceptKind(term.Category), name: term.Name, aliases: term.Aliases, hash: ConceptTextHash(term.Text())}
	}
	if len(file.Nodes) != len(want) {
		t.Errorf("the file has %d nodes, core makes %d", len(file.Nodes), len(want))
	}
	for _, n := range file.Nodes {
		w, ok := want[n.ID]
		switch {
		case !ok:
			t.Errorf("node %s is not a node of core", n.ID)
		case n.TextHash != w.hash:
			t.Errorf("node %s: textHash %s, core %s", n.ID, n.TextHash, w.hash)
		case n.Kind != w.kind || n.Name != w.name || !slices.Equal(n.Aliases, w.aliases):
			t.Errorf("node %s: kind, name or aliases differ from core's", n.ID)
		}
		delete(want, n.ID)
	}
	for id := range want {
		t.Errorf("core has node %s, the file does not", id)
	}

	// With the exact triple every edge of the file is taken, none refused.
	g := buildConceptGraph(rs, file, nil)
	if !g.stats.Exact {
		t.Error("the file is not an exact match for the shipped triple")
	}
	kept := 0
	for _, n := range g.stats.Kept {
		kept += n
	}
	dropped := 0
	for _, n := range g.stats.Dropped {
		dropped += n
	}
	if kept != len(file.Edges) || dropped != 0 {
		t.Errorf("kept %d and dropped %d of %d edges", kept, dropped, len(file.Edges))
	}
}
