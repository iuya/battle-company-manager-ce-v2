package conceptgraph

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/jsonx"
	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/repo"
)

var (
	raws    [3][]byte
	curated jsonx.Obj
	graph   []byte
)

func TestMain(m *testing.M) {
	root := repo.Root()
	for i, p := range []string{"definitions/vanilla/data2024.json", "definitions/battle_companies/battle_companies.json", "definitions/glossary/mesbg_glossary.json"} {
		b, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			panic(err)
		}
		raws[i] = b
	}
	c, err := jsonx.ReadFile(filepath.Join(root, "tools/concepts_curated.json"))
	if err != nil {
		panic(err)
	}
	curated = jsonx.O(c)
	if graph, err = os.ReadFile(filepath.Join(root, "definitions/concepts/concept_graph.json")); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func doc(t *testing.T) jsonx.Obj {
	t.Helper()
	v, err := jsonx.Decode(graph)
	if err != nil {
		t.Fatal(err)
	}
	return jsonx.O(v)
}

func first(d jsonx.Obj, typ string) jsonx.Obj {
	for _, e := range jsonx.A(d["edges"]) {
		if jsonx.S(jsonx.O(e)["type"]) == typ {
			return jsonx.O(e)
		}
	}
	return nil
}

func errors(t *testing.T, d jsonx.Obj) []string {
	t.Helper()
	e, err := CheckGraph(d, raws)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func assertFails(t *testing.T, d jsonx.Obj, text string) []string {
	t.Helper()
	errs := errors(t, d)
	for _, e := range errs {
		if strings.Contains(e, text) {
			return errs
		}
	}
	t.Fatalf("%q not in %v", text, errs)
	return nil
}

func TestShippedGraphIsCurrent(t *testing.T) {
	if e := errors(t, doc(t)); len(e) != 0 {
		t.Fatal(e)
	}
}

func TestBuildIsDeterministicAndEqualsTheFile(t *testing.T) {
	a, rep, err := Build(raws, curated, "Concept graph")
	if err != nil {
		t.Fatal(err)
	}
	b, _, _ := Build(raws, curated, "Concept graph")
	if len(rep.Errors) != 0 {
		t.Fatal(rep.Errors)
	}
	if !bytes.Equal(a.JSON(), b.JSON()) {
		t.Fatal("two builds differ")
	}
	if !bytes.Equal(a.JSON(), graph) {
		t.Fatal("the build is not byte-identical to definitions/concepts/concept_graph.json")
	}
}

func TestDanglingEdge(t *testing.T) {
	d := doc(t)
	d["edges"] = append(jsonx.A(d["edges"]), jsonx.Obj{"from": "rule:nope", "to": first(d, "grants")["to"], "type": "grants"})
	assertFails(t, d, "from is not a node")
}

func TestStaleBuiltFrom(t *testing.T) {
	d := doc(t)
	jsonx.O(d["builtFrom"])["battleCompanies"] = strings.Repeat("0", 64)
	if errs := assertFails(t, d, "builtFrom.battleCompanies is stale"); len(errs) != 1 {
		t.Fatal(errs)
	}
}

func TestStaleTextHash(t *testing.T) {
	d := doc(t)
	jsonx.O(jsonx.A(d["nodes"])[0])["textHash"] = strings.Repeat("0", 16)
	assertFails(t, d, "textHash is stale")
}

func TestVanillaOrGlossaryCannotPointIntoBattleCompanies(t *testing.T) {
	d := doc(t)
	var old, bc string
	for _, n := range jsonx.A(d["nodes"]) {
		o := jsonx.O(n)
		if f := o["file"]; old == "" && (f == "vanilla" || f == "glossary") {
			old = jsonx.S(o["id"])
		} else if bc == "" && f == "battleCompanies" {
			bc = jsonx.S(o["id"])
		}
	}
	d["edges"] = append(jsonx.A(d["edges"]), jsonx.Obj{"from": old, "to": bc, "type": "grants"})
	assertFails(t, d, "into Battle Companies")
}

func TestCancelsLiftsRequiresNeedAVia(t *testing.T) {
	for _, typ := range []string{"cancels", "lifts", "requires"} {
		d := doc(t)
		delete(first(d, typ), "via")
		assertFails(t, d, "needs a via")
	}
}

func TestDuplicateEdgeAndSelfLink(t *testing.T) {
	d := doc(t)
	dup := jsonx.Obj{}
	for k, v := range first(d, "grants") {
		dup[k] = v
	}
	d["edges"] = append(jsonx.A(d["edges"]), dup)
	assertFails(t, d, "duplicate")
	d = doc(t)
	id := jsonx.O(jsonx.A(d["nodes"])[0])["id"]
	d["edges"] = append(jsonx.A(d["edges"]), jsonx.Obj{"from": id, "to": id, "type": "grants"})
	assertFails(t, d, "self link")
}

func TestMentionsAndSeeAreNotStored(t *testing.T) {
	for _, typ := range []string{"mentions", "see"} {
		d := doc(t)
		e := jsonx.Obj{}
		for k, v := range first(d, "grants") {
			e[k] = v
		}
		e["type"] = typ
		d["edges"] = jsonx.Arr{e}
		assertFails(t, d, "only grants")
	}
}

func TestAmbiguousListMustMatchTheNames(t *testing.T) {
	d := doc(t)
	d["ambiguous"] = jsonx.Arr{jsonx.Obj{"alias": "fear", "candidates": jsonx.Arr{"rule:fear", "term:fear"}}}
	assertFails(t, d, "ambiguous[]")
	d = doc(t)
	delete(d, "ambiguous")
	assertFails(t, d, "ambiguous[]")
}

func TestMissingAndUnknownNodes(t *testing.T) {
	d := doc(t)
	gone := jsonx.O(jsonx.A(d["nodes"])[0])
	d["nodes"] = jsonx.A(d["nodes"])[1:]
	assertFails(t, d, "node "+jsonx.S(gone["id"])+" is missing")
	d = doc(t)
	d["nodes"] = append(jsonx.A(d["nodes"]), jsonx.Obj{"id": "rule:invented", "kind": "rule", "name": "Invented", "file": "glossary", "textHash": strings.Repeat("0", 16)})
	assertFails(t, d, "does not resolve")
}

func TestWrongFileAndDuplicateNode(t *testing.T) {
	d := doc(t)
	n := jsonx.O(jsonx.A(d["nodes"])[0])
	if n["file"] == "vanilla" {
		n["file"] = "glossary"
	} else {
		n["file"] = "vanilla"
	}
	assertFails(t, d, "but the entry lives in")
	d = doc(t)
	d["nodes"] = append(jsonx.A(d["nodes"]), jsonx.A(d["nodes"])[0])
	assertFails(t, d, "duplicate node")
}

func TestCuratedViaMustBeAQuote(t *testing.T) {
	bad := jsonx.Obj{"edges": jsonx.Arr{jsonx.Obj{"from": "Might", "to": "Fate", "type": "requires", "via": "this sentence is in neither text"}}}
	_, rep, err := Build(raws, bad, "x")
	if err != nil || !strings.Contains(strings.Join(rep.Errors, "\n"), "not a quote") {
		t.Fatal(rep.Errors, err)
	}
	unknown := jsonx.Obj{"edges": jsonx.Arr{jsonx.Obj{"from": "No Such Term", "to": "Fate", "type": "requires", "via": "x"}}}
	_, rep, _ = Build(raws, unknown, "x")
	if !strings.Contains(strings.Join(rep.Errors, "\n"), "no such term") {
		t.Fatal(rep.Errors)
	}
}

func TestCuratedEdgeRepeatingTheDataIsRefused(t *testing.T) {
	d := doc(t)
	names := map[string]string{}
	for _, n := range jsonx.A(d["nodes"]) {
		names[jsonx.S(jsonx.O(n)["id"])] = jsonx.S(jsonx.O(n)["name"])
	}
	var derived jsonx.Obj
	for _, e := range jsonx.A(d["edges"]) {
		if o := jsonx.O(e); o["type"] == "requires" && strings.HasPrefix(jsonx.S(o["via"]), "restriction") {
			derived = o
			break
		}
	}
	from, to := names[jsonx.S(derived["from"])], names[jsonx.S(derived["to"])]
	dup := jsonx.Obj{"edges": jsonx.Arr{jsonx.Obj{"from": from, "to": to, "type": "requires", "via": from}}}
	_, rep, err := Build(raws, dup, "x")
	if err != nil || len(rep.Errors) == 0 {
		t.Fatal(rep.Errors, err)
	}
}

func TestIndexFindsParametersAndReportsAmbiguity(t *testing.T) {
	idx := NewIndex([]Node{{ID: "rule:terror", Name: "Terror", Text: "a"}, {ID: "term:terror", Name: "Terror", Text: "b"}, {ID: "rule:fury", Name: "Fury", Aliases: []string{"Fierce"}, Text: "c"}})
	if id := idx.Find("Fury (Cavalry)"); id != "rule:fury" {
		t.Fatal(id)
	}
	if id := idx.Find("fierce"); id != "rule:fury" {
		t.Fatal(id)
	}
	if id := idx.Find("Terror"); id != "" {
		t.Fatal(id)
	}
	if w := idx.WhyNot("Terror"); w != "ambiguous (rule:terror, term:terror)" {
		t.Fatal(w)
	}
	if w := idx.WhyNot("Nothing"); w != "no such term" {
		t.Fatal(w)
	}
	if s, ok := idx.Texts("Terror (x)"); !ok || s != "a b" {
		t.Fatal(s, ok)
	}
	if len(idx.Ambiguous()) != 1 {
		t.Fatal(idx.Ambiguous())
	}
}

func TestHashesAndSentences(t *testing.T) {
	if got := RulesetHash([]byte("{ \"a\" : \"b c\" ,\n \"d\": [1, 2] }")); got != RulesetHash([]byte(`{"a":"b c","d":[1,2]}`)) {
		t.Fatal("white space outside strings must not change the hash")
	}
	if RulesetHash([]byte(`{"a":"b  c"}`)) == RulesetHash([]byte(`{"a":"b c"}`)) {
		t.Fatal("white space inside strings must change the hash")
	}
	if TextHash("a  b\n c") != TextHash("a b c") || len(TextHash("x")) != 16 {
		t.Fatal("text hash")
	}
	if s := sentenceWith("First one. The Shield rule applies! Last?", "shield"); s != "The Shield rule applies!" {
		t.Fatal(s)
	}
	if s := sentenceWith("Nothing here.", "shield"); s != "" {
		t.Fatal(s)
	}
	if s := sentenceWith(strings.Repeat("shield ", 40)+".", "shield"); s != "" {
		t.Fatal("a sentence longer than 220 characters is not a via")
	}
}

func TestOrderEdgesDropsSelfLinksAndRepeats(t *testing.T) {
	out := OrderEdges([]Edge{{"b", "a", "grants", ""}, {"a", "b", "grants", ""}, {"a", "b", "grants", "via"}, {"c", "c", "grants", "x"}})
	if len(out) != 2 || out[0] != (Edge{"a", "b", "grants", "via"}) || out[1].From != "b" {
		t.Fatal(out)
	}
}
