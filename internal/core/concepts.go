package core

import (
	"container/list"
	"context"
	"log/slog"
	"slices"
	"strings"
	"sync"
)

// Concepts looks concepts up in the graph of a ruleset (docs/concept-graph.md). It implements web.Concepts. A graph is built once per
// ruleset triple, from the glossary of the ruleset (mentions, see) and the curated file when there is one, and kept in a cache bounded
// like the rule sets (least recently used out).
type Concepts struct {
	rules   ruleSource
	curated *ConceptFile
	log     *slog.Logger

	mu    sync.Mutex
	calls map[RulesetHashes]*conceptCall
	lru   *list.List
	max   int
}

type conceptCall struct {
	done  chan struct{}
	graph *conceptGraph
	err   error
	elem  *list.Element
}

// NewConcepts builds the service. rules is the loader (*Rules); curated is the generated graph file or nil (mentions only).
func NewConcepts(rules ruleSource, curated *ConceptFile, log *slog.Logger) *Concepts {
	if log == nil {
		log = slog.Default()
	}
	return &Concepts{rules: rules, curated: curated, log: log, calls: map[RulesetHashes]*conceptCall{}, lru: list.New(), max: MaxCachedRuleSets}
}

// graph returns the graph of the ruleset, building it once however many ask at the same time. A failed build (or a panic in it) is an
// error and is not cached.
func (c *Concepts) graph(ctx context.Context, h RulesetHashes) (*conceptGraph, error) {
	c.mu.Lock()
	call, ok := c.calls[h]
	if ok {
		c.lru.MoveToFront(call.elem)
		c.mu.Unlock()
	} else {
		call = &conceptCall{done: make(chan struct{})}
		call.elem = c.lru.PushFront(h)
		c.calls[h] = call
		c.evict()
		c.mu.Unlock()
		call.err = safely("concept graph build", func() error {
			rs, err := c.rules.For(ctx, h)
			if err != nil {
				return err
			}
			call.graph = buildConceptGraph(rs, c.curated, c.log)
			return nil
		})
		if call.err != nil {
			c.mu.Lock()
			if c.calls[h] == call {
				delete(c.calls, h)
				c.lru.Remove(call.elem)
			}
			c.mu.Unlock()
		}
		close(call.done)
	}
	select {
	case <-call.done:
		return call.graph, call.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// evict drops the least recently used finished graphs beyond the cap. Callers hold the lock.
func (c *Concepts) evict() {
	for e := c.lru.Back(); e != nil && c.lru.Len() > c.max; {
		prev := e.Prev()
		h := e.Value.(RulesetHashes)
		select {
		case <-c.calls[h].done:
			delete(c.calls, h)
			c.lru.Remove(e)
		default:
		}
		e = prev
	}
}

// Cached is how many graphs are in memory (finished or being built).
func (c *Concepts) Cached() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

// Stats says what building the ruleset's graph did with the curated edges.
func (c *Concepts) Stats(ctx context.Context, rs RulesetRef) (ConceptStats, error) {
	g, err := c.graph(ctx, rs)
	if err != nil {
		return ConceptStats{}, err
	}
	return g.stats, nil
}

// Concept returns the concept with its text and its edges grouped by type and direction; ErrNotFound for an unknown id.
func (c *Concepts) Concept(ctx context.Context, rs RulesetRef, id string) (ConceptView, error) {
	g, err := c.graph(ctx, rs)
	if err != nil {
		return ConceptView{}, err
	}
	n, ok := g.nodes[id]
	if !ok {
		return ConceptView{}, ErrNotFound
	}
	return ConceptView{
		ID: n.id, Kind: n.kind, Name: n.name, Text: slices.Clone(n.text), GlossaryID: n.glossaryID,
		Out: g.group(g.out[id], func(e conceptEdge) string { return e.to }),
		In:  g.group(g.in[id], func(e conceptEdge) string { return e.from }),
	}, nil
}

// group turns edges into references keyed by type, each list sorted by name.
func (g *conceptGraph) group(edges []conceptEdge, other func(conceptEdge) string) map[string][]ConceptRef {
	out := map[string][]ConceptRef{}
	for _, e := range edges {
		n := g.nodes[other(e)]
		out[e.typ] = append(out[e.typ], ConceptRef{ID: n.id, Kind: n.kind, Name: n.name, Via: e.via})
	}
	for _, refs := range out {
		slices.SortFunc(refs, func(a, b ConceptRef) int {
			if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
				return c
			}
			return strings.Compare(a.ID, b.ID)
		})
	}
	return out
}

// Resolve maps a displayed name (a rule with its parameter, an alias, any case) to a concept. ok is false for an unknown name; an
// ambiguous name returns its Candidates and no ID. A ruleset whose graph cannot be built answers ok false.
func (c *Concepts) Resolve(ctx context.Context, rs RulesetRef, name string) (ConceptRef, bool) {
	g, err := c.graph(ctx, rs)
	if err != nil {
		return ConceptRef{}, false
	}
	ids := g.byPhrase[NameKey(name)]
	if len(ids) == 0 { // "Hatred (Spirit)" is the rule Hatred
		ids = g.byPhrase[NameKey(parameter.ReplaceAllString(name, ""))]
	}
	switch len(ids) {
	case 0:
		return ConceptRef{}, false
	case 1:
		n := g.nodes[ids[0]]
		return ConceptRef{ID: n.id, Kind: n.kind, Name: n.name}, true
	}
	cands := slices.Clone(ids)
	slices.Sort(cands)
	return ConceptRef{Name: strings.TrimSpace(name), Candidates: cands}, true
}
