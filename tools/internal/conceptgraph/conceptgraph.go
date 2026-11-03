// Package conceptgraph makes and checks the concept graph of the three shipped rules files (docs/concept-graph.md): nodes, derived
// edges, checks.
//
// Used by tools/conceptgraph (writes definitions/concepts/concept_graph.json) and tools/validate. The nodes are the terms of the
// ruleset's glossary exactly as the app builds it: the app's own loader makes them (core.LoadRuleSet) so that this package never
// repeats that logic, and internal/core/concept_graph_file_test.go checks every node of the generated file against core. The edges
// are derived here from the lean Battle Companies file (gear, paths) and from tools/concepts_curated.json.
package conceptgraph

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/jsonx"
)

// GraphVersion is the version written to the graph.
const GraphVersion = "1.0"

// Files are the three rules files, in the order of the hashes and of the arguments.
var Files = []string{"vanilla", "battleCompanies", "glossary"}

var curatedTypes = []string{"grants", "cancels", "lifts", "requires"}

var parameter = regexp.MustCompile(`\s*\([^)]*\)\s*`)

// ---- hashes (core.RulesetHash, core.ConceptTextHash) ----

// Compact is json.Compact: it drops the white space outside strings and touches nothing else.
func Compact(raw []byte) []byte {
	out := make([]byte, 0, len(raw))
	inStr, esc := false, false
	for _, b := range raw {
		switch {
		case inStr:
			out = append(out, b)
			if esc {
				esc = false
			} else if b == '\\' {
				esc = true
			} else if b == '"' {
				inStr = false
			}
		case b == '"':
			inStr = true
			out = append(out, b)
		case b != ' ' && b != '\t' && b != '\n' && b != '\r':
			out = append(out, b)
		}
	}
	return out
}

// RulesetHash is the hash of a rules file the graph records.
func RulesetHash(raw []byte) string {
	h := sha256.Sum256(Compact(raw))
	return hex.EncodeToString(h[:])
}

// TextHash is the hash of a node text: its words, single-spaced.
func TextHash(text string) string {
	h := sha256.Sum256([]byte(strings.Join(strings.Fields(text), " ")))
	return hex.EncodeToString(h[:])[:16]
}

// ---- the nodes: the glossary as the app builds it ----

// Node is one concept.
type Node struct {
	ID, Kind, Name, File, TextHash string
	Aliases                        []string
	Text                           string
}

// Terms is what the app's loader makes of the three files: the glossary terms and the classes of every gear item.
type Terms struct {
	Nodes       []Node
	GearClasses map[string][]string
}

var sourceFile = map[string]string{"Battle Companies": "battleCompanies", "Vanilla": "vanilla", "": "glossary"}

func conceptKind(category string) string {
	switch category {
	case "special rules", "path rules":
		return "rule"
	case "wargear", "equipment", "creatures", "wanderers":
		return "item"
	case "paths":
		return "path"
	case "magical powers":
		return "power"
	case "scenario", "scenarios":
		return "scenario"
	}
	return "term"
}

// LoadTerms makes the nodes and the gear classes of the three files (as bytes), as core.LoadRuleSet does.
func LoadTerms(raws [3][]byte) (*Terms, error) {
	rs, err := core.LoadRuleSet(raws[0], raws[1], raws[2], nil)
	if err != nil {
		return nil, fmt.Errorf("the nodes come from the app's loader (core.LoadRuleSet): %w", err)
	}
	if rs.Doc == nil {
		return nil, fmt.Errorf("the nodes come from the app's loader (core.LoadRuleSet): the Battle Companies file is not a lean (schemaVersion 2) file")
	}
	t := &Terms{GearClasses: map[string][]string{}}
	for _, g := range rs.Glossary.Terms {
		kind := conceptKind(g.Category)
		file, ok := sourceFile[g.Source]
		if !ok {
			file = "glossary"
		}
		text := g.Text()
		t.Nodes = append(t.Nodes, Node{ID: kind + ":" + g.ID, Kind: kind, Name: g.Name, File: file, TextHash: TextHash(text), Aliases: g.Aliases, Text: text})
	}
	// Keyed by the lower-cased name of each gear item as the file spells it; the annotations are keyed by core.NameKey, so a name with
	// a hyphen or an accent ("Uruk-Hai Bow") is looked up through it.
	ann := core.LeanGear(rs.Doc, rs.Vanilla).Annotations
	for _, g := range rs.Doc.Gear {
		for _, c := range ann[core.NameKey(g.Name)].Classes {
			k := strings.ToLower(g.Name)
			t.GearClasses[k] = append(t.GearClasses[k], string(c))
		}
	}
	return t, nil
}

// MakeNodes is the nodes of the three files.
func MakeNodes(raws [3][]byte) ([]Node, error) {
	t, err := LoadTerms(raws)
	if err != nil {
		return nil, err
	}
	return t.Nodes, nil
}

func key(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// Index is the name lookup over the nodes, like core's byPhrase.
type Index struct {
	Nodes    map[string]*Node
	byPhrase map[string][]string
}

// NewIndex indexes nodes by name and alias.
func NewIndex(nodes []Node) *Index {
	idx := &Index{Nodes: map[string]*Node{}, byPhrase: map[string][]string{}}
	for i := range nodes {
		idx.Nodes[nodes[i].ID] = &nodes[i]
	}
	for _, n := range nodes {
		seen := map[string]bool{}
		for _, p := range append([]string{n.Name}, n.Aliases...) {
			k := key(p)
			if k != "" && !seen[k] {
				seen[k] = true
				idx.byPhrase[k] = append(idx.byPhrase[k], n.ID)
			}
		}
	}
	return idx
}

// Ambiguous lists the aliases claimed by more than one node, as the graph file stores them.
func (x *Index) Ambiguous() []jsonx.Ordered {
	keys := make([]string, 0, len(x.byPhrase))
	for k, v := range x.byPhrase {
		if len(v) > 1 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	out := []jsonx.Ordered{}
	for _, k := range keys {
		c := append([]string{}, x.byPhrase[k]...)
		sort.Strings(c)
		out = append(out, jsonx.Ordered{{Key: "alias", Val: k}, {Key: "candidates", Val: c}})
	}
	return out
}

func (x *Index) ids(name string) []string {
	if ids := x.byPhrase[key(name)]; len(ids) > 0 {
		return ids
	}
	return x.byPhrase[key(parameter.ReplaceAllString(name, " "))]
}

// Find is the one node a displayed name stands for, a rule with its parameter ("Terror (Cavalry)") included; "" when unknown or
// ambiguous.
func (x *Index) Find(name string) string {
	for _, k := range []string{key(name), key(parameter.ReplaceAllString(name, " "))} {
		if ids := x.byPhrase[k]; len(ids) == 1 {
			return ids[0]
		}
	}
	return ""
}

// Texts is the text of every node named name (parameter ignored), joined; ok is false when no node has that name.
func (x *Index) Texts(name string) (string, bool) {
	ids := x.ids(name)
	if len(ids) == 0 {
		return "", false
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = x.Nodes[id].Text
	}
	return strings.Join(parts, " "), true
}

// WhyNot says why a name stands for no one node.
func (x *Index) WhyNot(name string) string {
	ids := x.ids(name)
	if len(ids) > 1 {
		s := append([]string{}, ids...)
		sort.Strings(s)
		return "ambiguous (" + strings.Join(s, ", ") + ")"
	}
	return "no such term"
}

// ---- derived edges ----

// Edge is a link between two concepts.
type Edge struct{ From, To, Type, Via string }

func (e Edge) ordered() jsonx.Ordered {
	o := jsonx.Ordered{{Key: "from", Val: e.From}, {Key: "to", Val: e.To}, {Key: "type", Val: e.Type}}
	if e.Via != "" {
		o = append(o, jsonx.KV{Key: "via", Val: e.Via})
	}
	return o
}

// Skipped is an edge that is not in the graph, and why.
type Skipped struct{ What, Why string }

// sentenceWith is the sentence of text that holds needle (whole, collapsed), or "".
func sentenceWith(text, needle string) string {
	text = strings.Join(strings.Fields(text), " ")
	low := strings.ToLower(needle)
	start := 0
	for i := 0; i <= len(text); i++ {
		if i == len(text) || (text[i] == ' ' && i > 0 && strings.IndexByte(".!?", text[i-1]) >= 0) {
			s := text[start:i]
			if strings.Contains(strings.ToLower(s), low) {
				if jsonx.Len(s) <= 220 {
					return s
				}
				return ""
			}
			start = i + 1
		}
	}
	return ""
}

// DeriveEdges is the grants, cancels, lifts and requires the lean Battle Companies file already states.
func DeriveEdges(bc jsonx.Obj, idx *Index, gearClasses map[string][]string) ([]Edge, []Skipped) {
	var edges []Edge
	var skipped []Skipped
	gear := jsonx.A(bc["gear"])
	nodeOf := idx.Find
	add := func(from, to, typ, via, what string) {
		if from == "" || to == "" {
			skipped = append(skipped, Skipped{what, "unresolved"})
			return
		}
		edges = append(edges, Edge{from, to, typ, via})
	}
	classes := func(g jsonx.Obj) []string {
		if c := jsonx.Strs(g["classes"]); len(c) > 0 {
			return c
		}
		return gearClasses[strings.ToLower(jsonx.S(g["name"]))]
	}
	// A standard weapon (a variant with a modifier such as Elven or poisoned, or a kind of hand-and-a-half weapon) that has no term of
	// its own: it gets no node and no edge, by decision, so it is not reported as a gap.
	plainWeapon := func(g jsonx.Obj) bool {
		if nodeOf(jsonx.S(g["name"])) != "" {
			return false
		}
		if jsonx.Truthy(g["base"]) {
			return true
		}
		for _, c := range classes(g) {
			if c == "melee" {
				return true
			}
		}
		return false
	}
	// grants: an item gives its bearer a rule
	for _, gi := range gear {
		g := jsonx.O(gi)
		name := jsonx.S(g["name"])
		frm := nodeOf(name)
		if plainWeapon(g) {
			continue
		}
		for _, rule := range jsonx.Strs(g["grants"]) {
			to := nodeOf(rule)
			via := ""
			if frm != "" && to != "" {
				via = sentenceWith(idx.Nodes[frm].Text, strings.TrimSpace(parameter.ReplaceAllString(rule, "")))
			}
			why := "the rule is " + idx.WhyNot(rule)
			if frm == "" {
				why = "the item is " + idx.WhyNot(name)
			}
			add(frm, to, "grants", via, fmt.Sprintf("%s grants %s: %s", name, rule, why))
		}
	}
	// grants: a path gives its starting heroic action and the rules of its chart
	for _, pi := range jsonx.A(bc["paths"]) {
		p := jsonx.O(pi)
		name := jsonx.S(p["name"])
		frm := nodeOf(name)
		if act := jsonx.S(p["heroicAction"]); act != "" {
			add(frm, nodeOf(act), "grants", "Starting heroic action: "+act+".", fmt.Sprintf("%s grants %s", name, act))
		}
		for _, row := range jsonx.A(p["rows"]) {
			for _, oi := range jsonx.A(jsonx.O(row)["options"]) {
				o := jsonx.O(oi)
				rule := ""
				if ro := jsonx.O(o["rule"]); ro != nil {
					rule = jsonx.S(ro["name"])
				} else {
					rule = jsonx.S(o["rule"])
				}
				if rule != "" {
					add(frm, nodeOf(rule), "grants", "Gained by heroes of the "+name+".", fmt.Sprintf("%s grants %s", name, rule))
				}
			}
		}
	}
	// cancels and lifts: the defence bonus of a shield (cancelledByClasses/Items, liftedByRules)
	byClass := map[string][]jsonx.Obj{}
	for _, gi := range gear {
		g := jsonx.O(gi)
		for _, c := range classes(g) {
			byClass[c] = append(byClass[c], g)
		}
	}
	for _, gi := range gear {
		g := jsonx.O(gi)
		name := jsonx.S(g["name"])
		shield := nodeOf(name)
		ab := jsonx.O(jsonx.O(g["abilities"])["defenceBonus"])
		if !jsonx.Truthy(ab) {
			continue
		}
		for _, c := range jsonx.Strs(ab["cancelledByClasses"]) {
			for _, other := range byClass[c] {
				on := jsonx.S(other["name"])
				if on != name && !plainWeapon(other) {
					add(nodeOf(on), shield, "cancels", fmt.Sprintf("defence bonus is lost with a %s weapon", strings.ReplaceAll(c, "_", "-")), fmt.Sprintf("%s cancels %s", on, name))
				}
			}
		}
		for _, item := range jsonx.Strs(ab["cancelledByItems"]) {
			add(nodeOf(item), shield, "cancels", "defence bonus is lost with a "+item, fmt.Sprintf("%s cancels %s", item, name))
		}
		for _, li := range jsonx.A(ab["liftedByRules"]) {
			lift := jsonx.O(li)
			via := "lifts the loss of the defence bonus caused by: " + strings.Join(jsonx.Strs(lift["cancels"]), ", ")
			if jsonx.Truthy(lift["whileMounted"]) {
				via += " (while mounted)"
			}
			rule := jsonx.S(lift["rule"])
			add(nodeOf(rule), shield, "lifts", via, fmt.Sprintf("%s lifts %s", rule, name))
		}
	}
	// requires: gear only a mounted model can use, equipment restricted to a kind of model
	cavalry := idx.Find("Cavalry")
	for _, gi := range gear {
		g := jsonx.O(gi)
		if jsonx.Truthy(g["requiresMount"]) && !plainWeapon(g) {
			add(nodeOf(jsonx.S(g["name"])), cavalry, "requires", "restriction: cavalry", jsonx.S(g["name"])+" requires Cavalry")
		}
	}
	hero := idx.Find("Hero")
	for _, gi := range gear {
		g := jsonx.O(gi)
		name := jsonx.S(g["name"])
		who := jsonx.O(g["who"])
		var restrictions []string
		if b, ok := who["hero"].(bool); ok && b {
			restrictions = append(restrictions, "hero")
		}
		for _, u := range jsonx.Strs(who["unitTypes"]) {
			if u == "Cavalry" {
				restrictions = append(restrictions, "cavalry")
				break
			}
		}
		for _, r := range restrictions {
			target := map[string]string{"cavalry": cavalry, "hero": hero}[r]
			if target == "" {
				skipped = append(skipped, Skipped{fmt.Sprintf("%s requires %s", name, r), "restriction with no term to point at"})
			} else {
				add(nodeOf(name), target, "requires", "restriction: "+r, fmt.Sprintf("%s requires %s", name, r))
			}
		}
	}
	return edges, skipped
}

// CuratedEdges resolves tools/concepts_curated.json (names) to edges; every via must be a quote of the text of one of the two nodes.
func CuratedEdges(curated jsonx.Obj, idx *Index) ([]Edge, []string) {
	var out []Edge
	var errors []string
	for _, ci := range jsonx.A(curated["edges"]) {
		c := jsonx.O(ci)
		from, to, typ, via := jsonx.S(c["from"]), jsonx.S(c["to"]), jsonx.S(c["type"]), jsonx.S(c["via"])
		frm, tn := idx.Find(from), idx.Find(to)
		label := fmt.Sprintf("%s %s %s", from, typ, to)
		if frm == "" || tn == "" {
			why := idx.WhyNot(to)
			if frm == "" {
				why = idx.WhyNot(from)
			}
			errors = append(errors, fmt.Sprintf("curated edge %s: %s", label, why))
			continue
		}
		hay := strings.Join(strings.Fields(idx.Nodes[frm].Text+" "+idx.Nodes[tn].Text), " ")
		if !strings.Contains(hay, strings.Join(strings.Fields(via), " ")) {
			errors = append(errors, fmt.Sprintf("curated edge %s: via is not a quote of either text: %s", label, jsonx.ReprString(via)))
			continue
		}
		out = append(out, Edge{frm, tn, typ, via})
	}
	return out, errors
}

// OrderEdges sorts edges, keeps the first of each (from, to, type) and drops self links.
func OrderEdges(edges []Edge) []Edge {
	s := append([]Edge{}, edges...)
	sort.SliceStable(s, func(i, j int) bool {
		a, b := s[i], s[j]
		if a.From != b.From {
			return a.From < b.From
		}
		if a.To != b.To {
			return a.To < b.To
		}
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		if (a.Via == "") != (b.Via == "") {
			return a.Via != "" // "via" not in e is False for an edge with a via, which sorts first
		}
		return a.Via < b.Via
	})
	seen := map[[3]string]bool{}
	var out []Edge
	for _, e := range s {
		k := [3]string{e.From, e.To, e.Type}
		if !seen[k] && e.From != e.To {
			seen[k] = true
			out = append(out, e)
		}
	}
	return out
}

// Graph is a built concept graph.
type Graph struct {
	Nodes     []Node
	Edges     []Edge
	Ambiguous []jsonx.Ordered
	BuiltFrom [3]string
	Name      string
}

// Report is what a build says besides the graph.
type Report struct {
	Skipped []Skipped
	Errors  []string
}

// Build makes the graph of raws = (vanilla, bc, glossary) and the curated edges.
func Build(raws [3][]byte, curated jsonx.Obj, name string) (*Graph, Report, error) {
	terms, err := LoadTerms(raws)
	if err != nil {
		return nil, Report{}, err
	}
	bcv, err := jsonx.Decode(raws[1])
	if err != nil {
		return nil, Report{}, err
	}
	idx := NewIndex(terms.Nodes)
	derived, skipped := DeriveEdges(jsonx.O(bcv), idx, terms.GearClasses)
	hand, errors := CuratedEdges(curated, idx)
	dk := map[[3]string]bool{}
	for _, e := range derived {
		dk[[3]string{e.From, e.To, e.Type}] = true
	}
	for _, e := range hand {
		if dk[[3]string{e.From, e.To, e.Type}] {
			errors = append(errors, fmt.Sprintf("curated edge %s %s %s is already derived from the data: remove it from the curated list", e.From, e.Type, e.To))
		}
	}
	var kept []Edge
	for _, e := range append(derived, hand...) { // a direction violation is a skipped edge, listed, never silently dropped
		ff, tf := idx.Nodes[e.From].File, idx.Nodes[e.To].File
		if (ff == "vanilla" || ff == "glossary") && tf == "battleCompanies" {
			skipped = append(skipped, Skipped{fmt.Sprintf("%s %s %s", e.From, e.Type, e.To), "direction: a vanilla or glossary node cannot point into Battle Companies"})
		} else {
			kept = append(kept, e)
		}
	}
	g := &Graph{Name: name, Edges: OrderEdges(kept), Ambiguous: idx.Ambiguous(), Nodes: append([]Node{}, terms.Nodes...)}
	for i, r := range raws {
		g.BuiltFrom[i] = RulesetHash(r)
	}
	sort.SliceStable(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID < g.Nodes[j].ID })
	return g, Report{skipped, errors}, nil
}

// JSON is the graph file's text: json.dumps(doc, indent=2, ensure_ascii=False) and a newline.
func (g *Graph) JSON() []byte {
	built := jsonx.Ordered{}
	for i, f := range Files {
		built = append(built, jsonx.KV{Key: f, Val: g.BuiltFrom[i]})
	}
	nodes := make(jsonx.Arr, len(g.Nodes))
	for i, n := range g.Nodes {
		o := jsonx.Ordered{{Key: "id", Val: n.ID}, {Key: "kind", Val: n.Kind}, {Key: "name", Val: n.Name}, {Key: "file", Val: n.File}, {Key: "textHash", Val: n.TextHash}}
		if len(n.Aliases) > 0 {
			o = append(o, jsonx.KV{Key: "aliases", Val: n.Aliases})
		}
		nodes[i] = o
	}
	edges := make(jsonx.Arr, len(g.Edges))
	for i, e := range g.Edges {
		edges[i] = e.ordered()
	}
	doc := jsonx.Ordered{{Key: "name", Val: g.Name}, {Key: "version", Val: GraphVersion}, {Key: "builtFrom", Val: built},
		{Key: "nodes", Val: nodes}, {Key: "edges", Val: edges}, {Key: "ambiguous", Val: g.Ambiguous}}
	return append(jsonx.Marshal(doc), '\n')
}

// ---- checks (tools/validate) ----

// CheckGraph makes the checks of docs/concept-graph.md on a graph document against the three files it should describe (as bytes).
func CheckGraph(doc jsonx.Obj, raws [3][]byte) ([]string, error) {
	var errors []string
	nodes, err := MakeNodes(raws)
	if err != nil {
		return nil, err
	}
	truth := map[string]Node{}
	for _, n := range nodes {
		truth[n.ID] = n
	}
	built := jsonx.O(doc["builtFrom"])
	for i, f := range Files {
		if jsonx.Str(built[f]) != RulesetHash(raws[i]) || built[f] == nil {
			errors = append(errors, fmt.Sprintf("builtFrom.%s is stale: the file changed since the graph was built (run go run ./tools/conceptgraph)", f))
		}
	}
	ids := map[string]bool{}
	fileOf := map[string]any{}
	for _, ni := range jsonx.A(doc["nodes"]) {
		n := jsonx.O(ni)
		id := jsonx.S(n["id"])
		if ids[id] {
			errors = append(errors, "duplicate node "+id)
		}
		ids[id] = true
		fileOf[id] = n["file"]
		t, ok := truth[id]
		if !ok {
			errors = append(errors, fmt.Sprintf("node %s does not resolve to an entry of the files", id))
			continue
		}
		if !jsonx.Equal(n["file"], t.File) {
			errors = append(errors, fmt.Sprintf("node %s: file %s but the entry lives in %s", id, jsonx.Repr(n["file"]), jsonx.ReprString(t.File)))
		}
		if !jsonx.Equal(n["name"], t.Name) || !jsonx.Equal(n["kind"], t.Kind) {
			errors = append(errors, fmt.Sprintf("node %s: name or kind differs from the entry", id))
		}
		if !jsonx.Equal(n["textHash"], t.TextHash) {
			errors = append(errors, fmt.Sprintf("node %s: textHash is stale", id))
		}
	}
	var missing []string
	for id := range truth {
		if !ids[id] {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)
	for _, id := range missing {
		errors = append(errors, fmt.Sprintf("node %s is missing from the graph", id))
	}
	want := jsonx.Arr{}
	for _, a := range NewIndex(nodes).Ambiguous() {
		c := jsonx.Arr{}
		for _, s := range a[1].Val.([]string) {
			c = append(c, s)
		}
		want = append(want, jsonx.Obj{"alias": a[0].Val, "candidates": c})
	}
	if !reflect.DeepEqual(doc["ambiguous"], any(want)) {
		errors = append(errors, "ambiguous[] is not the list of aliases claimed by more than one node")
	}
	seen := map[[3]string]bool{}
	for _, ei := range jsonx.A(doc["edges"]) {
		e := jsonx.O(ei)
		label := fmt.Sprintf("edge %s %s %s", jsonx.Str(e["from"]), jsonx.Str(e["type"]), jsonx.Str(e["to"]))
		typ, isStr := e["type"].(string)
		known := false
		for _, t := range curatedTypes {
			known = known || (isStr && t == typ)
		}
		if !known {
			errors = append(errors, fmt.Sprintf("%s: only %s are stored (mentions and see are computed by the app)", label, strings.Join(curatedTypes, ", ")))
		}
		for _, end := range []string{"from", "to"} {
			if s, ok := e[end].(string); !ok || !ids[s] {
				errors = append(errors, fmt.Sprintf("%s: %s is not a node", label, end))
			}
		}
		k := [3]string{jsonx.Str(e["from"]), jsonx.Str(e["to"]), jsonx.Str(e["type"])}
		if seen[k] {
			errors = append(errors, label+": duplicate")
		}
		seen[k] = true
		if jsonx.Equal(e["from"], e["to"]) {
			errors = append(errors, label+": self link")
		}
		ff, _ := fileOf[jsonx.S(e["from"])].(string)
		tf, _ := fileOf[jsonx.S(e["to"])].(string)
		if (ff == "vanilla" || ff == "glossary") && tf == "battleCompanies" {
			errors = append(errors, fmt.Sprintf("%s: points from %s into Battle Companies", label, ff))
		}
		via := ""
		if v, ok := e["via"]; ok {
			via = jsonx.Str(v) // a null via counts as written, as the text "None"
		}
		if (typ == "cancels" || typ == "lifts" || typ == "requires") && strings.TrimSpace(via) == "" {
			errors = append(errors, label+": needs a via")
		}
	}
	return errors, nil
}
