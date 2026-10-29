package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
)

// The concept graph (docs/concept-graph.md): the things of a ruleset (rules, terms, items, paths...) and how they relate. The nodes are the
// terms of the ruleset's glossary, which already holds the special rules, wargear, paths and powers of the vanilla and Battle Companies files.
// Mentions and "see also" are computed from the ruleset's own text with the glossary linker; the reviewed relations (grants, cancels, lifts,
// requires) come from the curated file, reused edge by edge.

// Edge types.
const (
	EdgeMentions = "mentions"
	EdgeSee      = "see"
	EdgeGrants   = "grants"
	EdgeCancels  = "cancels"
	EdgeLifts    = "lifts"
	EdgeRequires = "requires"
)

// curatedTypes are the edge types read from the curated file; mentions and see are never read from it.
var curatedTypes = []string{EdgeGrants, EdgeCancels, EdgeLifts, EdgeRequires}

// RulesetRef names the ruleset a concept is looked up in: its three file hashes.
type RulesetRef = RulesetHashes

// ConceptRef is a link to a concept. For an ambiguous name ID is empty and Candidates lists the ids it may mean.
type ConceptRef struct {
	ID, Kind, Name string
	Via            string
	Candidates     []string
}

// ConceptSegment is a piece of a concept's text: plain, or (ConceptID set) a mention of another concept.
type ConceptSegment struct {
	Text      string
	ConceptID string
}

// ConceptView is a concept ready for the popup.
type ConceptView struct {
	ID, Kind, Name string
	Text           []ConceptSegment
	GlossaryID     string                  // "" when there is no glossary term
	Out, In        map[string][]ConceptRef // keyed by edge type
}

// ConceptFile is the curated graph file (definitions/concepts/concept_graph.json). Node text is not stored, only its hash.
type ConceptFile struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	BuiltFrom struct {
		Vanilla         string `json:"vanilla"`
		BattleCompanies string `json:"battleCompanies"`
		Glossary        string `json:"glossary"`
	} `json:"builtFrom"`
	Nodes []ConceptFileNode `json:"nodes"`
	Edges []ConceptFileEdge `json:"edges"`
}

type ConceptFileNode struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Name     string   `json:"name"`
	Aliases  []string `json:"aliases,omitempty"`
	TextHash string   `json:"textHash"`
}

type ConceptFileEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
	Via  string `json:"via,omitempty"`
}

// ParseConceptFile reads the curated file. The result is only a candidate: whether it applies is decided per ruleset.
func ParseConceptFile(content []byte) (*ConceptFile, error) {
	var f ConceptFile
	if err := json.Unmarshal(content, &f); err != nil {
		return nil, fmt.Errorf("the concept graph is not valid JSON: %w", err)
	}
	return &f, nil
}

// ConceptTextHash is the hash the curated file stores per node: the first 16 hex digits of the SHA-256 of the node's text with its
// white space collapsed to single spaces.
func ConceptTextHash(text string) string {
	sum := sha256.Sum256([]byte(strings.Join(strings.Fields(text), " ")))
	return hex.EncodeToString(sum[:])[:16]
}

// ConceptStats is what building a graph did with the curated edges.
type ConceptStats struct {
	Exact   bool           // the file was built from this very ruleset: its edges are used without checks
	Kept    map[string]int // curated edges taken, by type
	Dropped map[string]int // curated edges left out, by type
	Edges   int            // all edges of the graph
}

type conceptNode struct {
	id, kind, name, glossaryID string
	aliases                    []string
	text                       []ConceptSegment
	hash                       string
}

type conceptEdge struct{ from, to, typ, via string }

type conceptGraph struct {
	nodes    map[string]*conceptNode
	byPhrase map[string][]string // lower-cased name or alias -> node ids (more than one: ambiguous)
	out, in  map[string][]conceptEdge
	stats    ConceptStats
}

// conceptKind is the kind of node a glossary term makes, from its category.
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

// buildConceptGraph makes the graph of a ruleset's glossary: mentions and see always from its text, plus the curated edges that still hold
// (see keepCurated). A nil file means mentions and see only.
func buildConceptGraph(rs *RuleSet, file *ConceptFile, log *slog.Logger) *conceptGraph {
	g := &conceptGraph{
		nodes: map[string]*conceptNode{}, byPhrase: map[string][]string{}, out: map[string][]conceptEdge{}, in: map[string][]conceptEdge{},
		stats: ConceptStats{Kept: map[string]int{}, Dropped: map[string]int{}},
	}
	terms := rs.Glossary.Terms
	nodeOf := make(map[string]string, len(terms)) // glossary term id -> node id
	for _, t := range terms {
		n := &conceptNode{id: conceptID(conceptKind(t.Category), t.ID), kind: conceptKind(t.Category), name: t.Name, glossaryID: t.ID, aliases: t.Aliases}
		n.hash = ConceptTextHash(t.Text())
		g.nodes[n.id] = n
		nodeOf[t.ID] = n.id
		seen := map[string]bool{}
		for _, p := range slices.Concat([]string{t.Name}, t.Aliases) {
			if k := NameKey(p); k != "" && !seen[k] {
				seen[k] = true
				g.byPhrase[k] = append(g.byPhrase[k], n.id)
			}
		}
	}
	for _, t := range terms { // after all names are known: a mention of an ambiguous name is left as plain text
		g.nodes[nodeOf[t.ID]].text = g.termText(t, nodeOf)
	}
	add := func(e conceptEdge) bool {
		if e.from == e.to || g.nodes[e.from] == nil || g.nodes[e.to] == nil {
			return false
		}
		for _, o := range g.out[e.from] {
			if o.to == e.to && o.typ == e.typ {
				return false
			}
		}
		g.out[e.from] = append(g.out[e.from], e)
		g.in[e.to] = append(g.in[e.to], e)
		g.stats.Edges++
		return true
	}
	for _, t := range terms {
		from := nodeOf[t.ID]
		mentioned := map[string]bool{}
		for _, seg := range g.nodes[from].text {
			if seg.ConceptID == "" || mentioned[seg.ConceptID] {
				continue
			}
			mentioned[seg.ConceptID] = true
			add(conceptEdge{from: from, to: seg.ConceptID, typ: EdgeMentions, via: seg.Text})
		}
		for _, s := range t.See {
			if to, ok := nodeOf[s.ID]; ok && !g.ambiguous(s.Name) {
				add(conceptEdge{from: from, to: to, typ: EdgeSee, via: s.Name})
			}
		}
	}
	if file != nil {
		g.keepCurated(rs.Hashes, file, add)
	}
	if log != nil {
		log.Info("concept graph built", "vanilla", short(rs.Hashes.Vanilla), "battleCompanies", short(rs.Hashes.BattleCompanies), "glossary", short(rs.Hashes.Glossary),
			"nodes", len(g.nodes), "edges", g.stats.Edges, "exact", g.stats.Exact, "curatedKept", g.stats.Kept, "curatedDropped", g.stats.Dropped)
	}
	return g
}

func short(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
}

func conceptID(kind, glossaryID string) string { return kind + ":" + glossaryID }

// termText is the term's definition as segments: paragraphs (and the rows of a path's chart) joined by a blank line, mentions of
// ambiguous names left as plain text.
func (g *conceptGraph) termText(t GlossaryTerm, nodeOf map[string]string) []ConceptSegment {
	var out []ConceptSegment
	push := func(segs []Segment) {
		if len(out) > 0 {
			out = append(out, ConceptSegment{Text: "\n\n"})
		}
		for _, s := range segs {
			cs := ConceptSegment{Text: s.Text}
			if to, ok := nodeOf[s.TermID]; ok && s.TermID != "" && !g.ambiguous(s.Text) {
				cs.ConceptID = to
			}
			out = append(out, cs)
		}
	}
	for _, para := range t.Paragraphs {
		push(para)
	}
	if t.Path != nil {
		for _, row := range t.Path.Rows {
			push(row.Segments)
		}
	}
	return out
}

func (g *conceptGraph) ambiguous(name string) bool { return len(g.byPhrase[NameKey(name)]) > 1 }

// keepCurated adds the curated edges that hold for this ruleset. With an exact builtFrom match the file is taken as it is (an edge to an
// unknown node is still refused: the graph must stay consistent). Otherwise an edge is kept only when both endpoints still resolve (by id,
// then by name or alias) and the text of its from node is unchanged since the file was built; nothing is guessed or repaired.
func (g *conceptGraph) keepCurated(h RulesetHashes, f *ConceptFile, add func(conceptEdge) bool) {
	exact := f.BuiltFrom.Vanilla == h.Vanilla && f.BuiltFrom.BattleCompanies == h.BattleCompanies && f.BuiltFrom.Glossary == h.Glossary && h.Vanilla != ""
	g.stats.Exact = exact
	fileNode := make(map[string]ConceptFileNode, len(f.Nodes))
	for _, n := range f.Nodes {
		fileNode[n.ID] = n
	}
	// resolve finds the ruleset node a file node stands for: the same id, else its name or an alias when that names exactly one node.
	resolve := func(id string) (string, bool) {
		if _, ok := g.nodes[id]; ok {
			return id, true
		}
		fn, ok := fileNode[id]
		if !ok {
			return "", false
		}
		for _, p := range slices.Concat([]string{fn.Name}, fn.Aliases) {
			if ids := g.byPhrase[NameKey(p)]; len(ids) == 1 && g.nodes[ids[0]].kind == fn.Kind {
				return ids[0], true
			}
		}
		return "", false
	}
	for _, e := range f.Edges {
		if !slices.Contains(curatedTypes, e.Type) {
			continue
		}
		from, ok1 := resolve(e.From)
		to, ok2 := resolve(e.To)
		keep := ok1 && ok2
		if keep && !exact {
			fn, known := fileNode[e.From]
			keep = known && fn.TextHash != "" && fn.TextHash == g.nodes[from].hash
		}
		if keep {
			keep = add(conceptEdge{from: from, to: to, typ: e.Type, via: e.Via})
		}
		if keep {
			g.stats.Kept[e.Type]++
		} else {
			g.stats.Dropped[e.Type]++
		}
	}
}
