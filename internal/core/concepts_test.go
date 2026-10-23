package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// conceptTerm is one term of a fixture glossary.
type conceptTerm struct{ name, category, def, aliases string }

// conceptRuleset builds a rule set whose glossary holds the terms; only the glossary matters to the graph.
func conceptRuleset(t *testing.T, hashes RulesetHashes, terms ...conceptTerm) *RuleSet {
	t.Helper()
	var parts []string
	for _, x := range terms {
		aliases := "[]"
		if x.aliases != "" {
			aliases = `["` + strings.ReplaceAll(x.aliases, ",", `","`) + `"]`
		}
		parts = append(parts, fmt.Sprintf(`{"name":%q,"category":%q,"aliases":%s,"definition":%q,"sources":[{"section":"s","rulebookPage":1}]}`, x.name, x.category, aliases, x.def))
	}
	g, err := BuildGlossary(RulesetInfo{Name: "G"}, []byte(`{"name":"G","version":"1","terms":[`+strings.Join(parts, ",")+`]}`))
	if err != nil {
		t.Fatal(err)
	}
	return &RuleSet{Hashes: hashes, Glossary: g}
}

var (
	baseHashes  = RulesetHashes{Vanilla: "v1", BattleCompanies: "b1", Glossary: "g1"}
	otherHashes = RulesetHashes{Vanilla: "v1", BattleCompanies: "b2", Glossary: "g1"}

	cloak   = conceptTerm{"Elven Cloak", "wargear", "A cloak that grants Stealthy to its bearer. It is not a Shield.", ""}
	stealth = conceptTerm{"Stealthy", "special rules", "Hard to see from afar.", ""}
	shield  = conceptTerm{"Shield", "wargear", "A shield adds +1 Defence. Worth a Heroic Action.", ""}
	fury    = conceptTerm{"Fury", "special rules", "Fury (X): the model is Fearless. It wields a Shield and cares about Fear. Fury does not stack, Fury again.", ""}
	hatred  = conceptTerm{"Hatred", "special rules", "Hatred (X): a model that is Stealthy hates its foe.", ""}
	fearRU  = conceptTerm{"Fear", "special rules", "A model that causes Fear makes enemies test.", ""}
	fearBA  = conceptTerm{"Fear", "basics", "Fear is also an ordinary term.", ""}
	heroic  = conceptTerm{"Heroic Action", "heroic actions", "Heroes use Heroic Actions.", "Heroic Actions"}
)

func baseTerms() []conceptTerm {
	return []conceptTerm{cloak, stealth, shield, fury, hatred, fearRU, fearBA, heroic}
}

// curatedFor makes the curated file of a rule set, with the three reviewed edges and the text hashes of its nodes.
func curatedFor(rs *RuleSet) *ConceptFile {
	g := buildConceptGraph(rs, nil, nil)
	f := &ConceptFile{Name: "graph", Version: "1"}
	f.BuiltFrom.Vanilla, f.BuiltFrom.BattleCompanies, f.BuiltFrom.Glossary = rs.Hashes.Vanilla, rs.Hashes.BattleCompanies, rs.Hashes.Glossary
	for _, n := range g.nodes {
		f.Nodes = append(f.Nodes, ConceptFileNode{ID: n.id, Kind: n.kind, Name: n.name, Aliases: n.aliases, TextHash: n.hash})
	}
	f.Edges = []ConceptFileEdge{
		{From: "item:elven-cloak", To: "rule:stealthy", Type: EdgeGrants, Via: "grants Stealthy"},
		{From: "item:shield", To: "rule:fury", Type: EdgeCancels, Via: "a bow cancels"},
		{From: "rule:hatred", To: "rule:stealthy", Type: EdgeRequires, Via: "is Stealthy"},
		{From: "rule:hatred", To: "rule:stealthy", Type: EdgeMentions, Via: "never read from the file"},
	}
	return f
}

func counts(m map[string]int) string {
	n := 0
	for _, v := range m {
		n += v
	}
	return fmt.Sprint(n)
}

func hasEdge(g *conceptGraph, from, to, typ string) bool {
	for _, e := range g.out[from] {
		if e.to == to && e.typ == typ {
			return true
		}
	}
	return false
}

func TestMentionsAndSeeFromTheText(t *testing.T) {
	rs := conceptRuleset(t, baseHashes, baseTerms()...)
	g := buildConceptGraph(rs, nil, nil)
	for _, c := range []struct{ from, to string }{
		{"item:elven-cloak", "rule:stealthy"}, {"item:elven-cloak", "item:shield"}, {"rule:hatred", "rule:stealthy"},
		{"item:shield", "term:heroic-action"}, {"rule:fury", "item:shield"},
	} {
		if !hasEdge(g, c.from, c.to, EdgeMentions) {
			t.Errorf("missing mention %s -> %s", c.from, c.to)
		}
	}
	// first occurrence only: one edge although Fury names itself twice and a Shield once; a term never mentions itself
	if n := len(g.out["rule:fury"]); n != 1 { // Shield; "Fear" is ambiguous, "Fearless" is another word, itself is never linked
		t.Errorf("Fury has %d edges: %+v", n, g.out["rule:fury"])
	}
	for _, e := range g.out["rule:fury"] {
		if strings.Contains(e.to, "fear") {
			t.Errorf("ambiguous Fear was linked: %+v", e)
		}
	}
	if len(g.stats.Kept) != 0 || g.stats.Exact {
		t.Errorf("no file, yet stats %+v", g.stats)
	}
}

func TestSeeLinks(t *testing.T) {
	g, err := BuildGlossary(RulesetInfo{}, []byte(glossaryDoc))
	if err != nil {
		t.Fatal(err)
	}
	cg := buildConceptGraph(&RuleSet{Glossary: g}, nil, nil)
	if !hasEdge(cg, "term:stand-fast", "term:heroic-action", EdgeSee) {
		t.Errorf("see edge missing: %+v", cg.out["term:stand-fast"])
	}
}

func fakeRules(sets map[RulesetHashes]*RuleSet, calls *atomic.Int32) ruleSource {
	return rulesFunc(func(_ context.Context, h RulesetHashes) (*RuleSet, error) {
		if calls != nil {
			calls.Add(1)
		}
		if rs, ok := sets[h]; ok {
			return rs, nil
		}
		return nil, errors.New("no such rule set")
	})
}

type rulesFunc func(context.Context, RulesetHashes) (*RuleSet, error)

func (f rulesFunc) For(ctx context.Context, h RulesetHashes) (*RuleSet, error) { return f(ctx, h) }

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestConceptLookup(t *testing.T) {
	rs := conceptRuleset(t, baseHashes, baseTerms()...)
	c := NewConcepts(fakeRules(map[RulesetHashes]*RuleSet{baseHashes: rs}, nil), curatedFor(rs), quiet())
	ctx := context.Background()

	t.Run("view with both directions", func(t *testing.T) {
		v, err := c.Concept(ctx, baseHashes, "rule:stealthy")
		if err != nil {
			t.Fatal(err)
		}
		if v.Kind != "rule" || v.Name != "Stealthy" || v.GlossaryID != "stealthy" {
			t.Errorf("view: %+v", v)
		}
		in := v.In[EdgeMentions]
		if len(in) != 2 || in[0].Name != "Elven Cloak" || in[1].Name != "Hatred" || in[0].Via != "Stealthy" {
			t.Errorf("mentioned in: %+v", in)
		}
		if len(v.In[EdgeGrants]) != 1 || v.In[EdgeGrants][0].Via != "grants Stealthy" || len(v.In[EdgeRequires]) != 1 {
			t.Errorf("curated in: %+v", v.In)
		}
		cl, _ := c.Concept(ctx, baseHashes, "item:elven-cloak")
		var linked []string
		var text strings.Builder
		for _, s := range cl.Text {
			text.WriteString(s.Text)
			if s.ConceptID != "" {
				linked = append(linked, s.ConceptID)
			}
		}
		if text.String() != cloak.def || strings.Join(linked, ",") != "rule:stealthy,item:shield" {
			t.Errorf("text %q links %v", text.String(), linked)
		}
	})
	t.Run("unknown id", func(t *testing.T) {
		if _, err := c.Concept(ctx, baseHashes, "rule:nope"); !errors.Is(err, ErrNotFound) {
			t.Errorf("err %v", err)
		}
	})
	t.Run("resolve", func(t *testing.T) {
		for _, tc := range []struct {
			name, id string
			ok       bool
		}{
			{"Stealthy", "rule:stealthy", true}, {"  stealthy ", "rule:stealthy", true}, {"Heroic Actions", "term:heroic-action", true},
			{"Hatred (Spirit)", "rule:hatred", true}, {"Fury (2)", "rule:fury", true}, {"Nothing", "", false}, {"", "", false},
		} {
			ref, ok := c.Resolve(ctx, baseHashes, tc.name)
			if ok != tc.ok || ref.ID != tc.id {
				t.Errorf("Resolve(%q) = %+v, %v", tc.name, ref, ok)
			}
		}
	})
	t.Run("ambiguous name", func(t *testing.T) {
		ref, ok := c.Resolve(ctx, baseHashes, "FEAR")
		if !ok || ref.ID != "" || len(ref.Candidates) != 2 || ref.Candidates[0] == ref.Candidates[1] {
			t.Errorf("Resolve(Fear) = %+v, %v", ref, ok)
		}
		for _, id := range ref.Candidates {
			if _, err := c.Concept(ctx, baseHashes, id); err != nil {
				t.Errorf("candidate %s: %v", id, err)
			}
			if v, _ := c.Concept(ctx, baseHashes, id); len(v.In[EdgeMentions]) != 0 {
				t.Errorf("%s is mentioned although the name is ambiguous: %+v", id, v.In)
			}
		}
	})
}

func TestNoGraph(t *testing.T) {
	var calls atomic.Int32
	rs := conceptRuleset(t, baseHashes)
	c := NewConcepts(fakeRules(map[RulesetHashes]*RuleSet{baseHashes: rs}, &calls), nil, quiet())
	ctx := context.Background()
	missing := RulesetHashes{Vanilla: "x"}
	if _, err := c.Concept(ctx, missing, "rule:a"); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("an unloadable rule set must be an error other than not found: %v", err)
	}
	if _, ok := c.Resolve(ctx, missing, "A"); ok {
		t.Error("resolved in a rule set that cannot be loaded")
	}
	if c.Cached() != 0 {
		t.Errorf("a failed build was cached: %d", c.Cached())
	}
	// an empty glossary is a graph without nodes
	if _, err := c.Concept(ctx, baseHashes, "rule:a"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err %v", err)
	}
	if _, ok := c.Resolve(ctx, baseHashes, "A"); ok {
		t.Error("resolved in an empty graph")
	}
	// no glossary at all (link nil): still a graph
	bare := &RuleSet{Hashes: RulesetHashes{Vanilla: "bare"}}
	c = NewConcepts(fakeRules(map[RulesetHashes]*RuleSet{bare.Hashes: bare}, nil), nil, quiet())
	if _, err := c.Concept(ctx, bare.Hashes, "term:a"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err %v", err)
	}
}

func TestConceptBuildPanicIsAnError(t *testing.T) {
	boom := rulesFunc(func(context.Context, RulesetHashes) (*RuleSet, error) { panic("boom") })
	c := NewConcepts(boom, nil, quiet())
	if _, err := c.Concept(context.Background(), baseHashes, "x"); err == nil || !strings.Contains(err.Error(), "panicked") {
		t.Errorf("err %v", err)
	}
	if c.Cached() != 0 {
		t.Error("a panicked build was cached")
	}
}

func TestConceptGraphBuiltOncePerTripleConcurrently(t *testing.T) {
	var calls atomic.Int32
	rs := conceptRuleset(t, baseHashes, baseTerms()...)
	c := NewConcepts(fakeRules(map[RulesetHashes]*RuleSet{baseHashes: rs}, &calls), nil, quiet())
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Concept(context.Background(), baseHashes, "rule:stealthy"); err != nil {
				t.Error(err)
			}
			c.Resolve(context.Background(), baseHashes, "Fury (3)")
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Errorf("built %d times", calls.Load())
	}
}

func TestConceptCacheIsBounded(t *testing.T) {
	sets := map[RulesetHashes]*RuleSet{}
	c := NewConcepts(rulesFunc(func(_ context.Context, h RulesetHashes) (*RuleSet, error) {
		return conceptRuleset(t, h, stealth), nil
	}), nil, quiet())
	c.max = 3
	for i := 0; i < 10; i++ {
		h := RulesetHashes{Vanilla: fmt.Sprint(i)}
		sets[h] = nil
		if _, err := c.Concept(context.Background(), h, "rule:stealthy"); err != nil {
			t.Fatal(err)
		}
	}
	if c.Cached() != 3 {
		t.Errorf("cached %d", c.Cached())
	}
}

// The curated file is reused edge by edge for a ruleset it was not built from.
func TestCuratedEdgesPerRuleset(t *testing.T) {
	base := conceptRuleset(t, baseHashes, baseTerms()...)
	file := curatedFor(base)
	with := func(mod func([]conceptTerm) []conceptTerm) *RuleSet {
		return conceptRuleset(t, otherHashes, mod(baseTerms())...)
	}
	replace := func(name, def string) func([]conceptTerm) []conceptTerm {
		return func(ts []conceptTerm) []conceptTerm {
			for i := range ts {
				if ts[i].name == name && ts[i].category != "basics" {
					ts[i].def = def
				}
			}
			return ts
		}
	}
	drop := func(name string) func([]conceptTerm) []conceptTerm {
		return func(ts []conceptTerm) []conceptTerm {
			var out []conceptTerm
			for _, x := range ts {
				if x.name != name {
					out = append(out, x)
				}
			}
			return out
		}
	}
	tests := []struct {
		name          string
		rs            *RuleSet
		exact         bool
		kept, dropped int
		check         func(t *testing.T, g *conceptGraph)
	}{
		{"exact builtFrom", base, true, 3, 0, func(t *testing.T, g *conceptGraph) {
			if !hasEdge(g, "item:elven-cloak", "rule:stealthy", EdgeGrants) || !hasEdge(g, "item:elven-cloak", "rule:stealthy", EdgeMentions) {
				t.Error("curated and mentions edge missing")
			}
		}},
		{"same files, other triple", with(func(ts []conceptTerm) []conceptTerm { return ts }), false, 3, 0, nil},
		{"modified text drops the edges from it, keeps the others", with(replace("Elven Cloak", "A cloak. It is not a Shield, no more.")), false, 2, 1, func(t *testing.T, g *conceptGraph) {
			if hasEdge(g, "item:elven-cloak", "rule:stealthy", EdgeGrants) {
				t.Error("grants edge of a modified text was kept")
			}
			if !hasEdge(g, "item:shield", "rule:fury", EdgeCancels) || !hasEdge(g, "rule:hatred", "rule:stealthy", EdgeRequires) {
				t.Error("edges of unchanged texts were dropped")
			}
		}},
		{"modified target text keeps the edge", with(replace("Stealthy", "Harder to see.")), false, 3, 0, nil},
		{"deleted rule drops the edges to and from it", with(drop("Stealthy")), false, 1, 2, func(t *testing.T, g *conceptGraph) {
			if !hasEdge(g, "item:shield", "rule:fury", EdgeCancels) {
				t.Error("unrelated edge dropped")
			}
		}},
		{"deleted source drops its edges", with(drop("Shield")), false, 2, 1, nil},
		{"renamed alias", with(func(ts []conceptTerm) []conceptTerm {
			for i := range ts {
				switch ts[i].name {
				case "Shield":
					ts[i].name, ts[i].aliases = "Round Shield", "Shield,Buckler"
				case "Fury":
					ts[i].def = "Fury (X): carries a Buckler."
				}
			}
			return ts
		}), false, 3, 0, func(t *testing.T, g *conceptGraph) {
			// the shield's id follows its name, the file's node is found through the alias; its cancel edge needs the unchanged text of the
			// *source*, which is the shield (unchanged). The Fury text changed, but Fury is only a target.
			if !hasEdge(g, "item:round-shield", "rule:fury", EdgeCancels) {
				t.Errorf("cancel edge not kept through the node: %+v", g.out["item:round-shield"])
			}
			if !hasEdge(g, "rule:fury", "item:round-shield", EdgeMentions) {
				t.Error("the mention did not follow the new alias")
			}
		}},
		{"added rule gets mentions only", with(func(ts []conceptTerm) []conceptTerm {
			return append(ts, conceptTerm{"Brand New", "special rules", "Like Stealthy but new.", ""})
		}), false, 3, 0, func(t *testing.T, g *conceptGraph) {
			if !hasEdge(g, "rule:brand-new", "rule:stealthy", EdgeMentions) || len(g.out["rule:brand-new"]) != 1 {
				t.Errorf("added rule: %+v", g.out["rule:brand-new"])
			}
		}},
		{"fully different ruleset", conceptRuleset(t, RulesetHashes{Vanilla: "z", BattleCompanies: "z", Glossary: "z"},
			conceptTerm{"Orc Banner", "wargear", "Wave the Rally Flag.", ""}, conceptTerm{"Rally Flag", "special rules", "Rally.", ""}), false, 0, 3, func(t *testing.T, g *conceptGraph) {
			if !hasEdge(g, "item:orc-banner", "rule:rally-flag", EdgeMentions) {
				t.Error("mentions do not work")
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := buildConceptGraph(tc.rs, file, quiet())
			if g.stats.Exact != tc.exact || counts(g.stats.Kept) != fmt.Sprint(tc.kept) || counts(g.stats.Dropped) != fmt.Sprint(tc.dropped) {
				t.Errorf("exact %v kept %v dropped %v, want %v %d %d", g.stats.Exact, g.stats.Kept, g.stats.Dropped, tc.exact, tc.kept, tc.dropped)
			}
			if tc.check != nil {
				tc.check(t, g)
			}
		})
	}
}

func TestCuratedFileEdgeCases(t *testing.T) {
	base := conceptRuleset(t, baseHashes, baseTerms()...)
	file := curatedFor(base)
	other := conceptRuleset(t, otherHashes, baseTerms()...)

	t.Run("a node without a text hash cannot vouch for its edges", func(t *testing.T) {
		f := *file
		f.Nodes = append([]ConceptFileNode(nil), file.Nodes...)
		for i := range f.Nodes {
			f.Nodes[i].TextHash = ""
		}
		if g := buildConceptGraph(other, &f, nil); counts(g.stats.Kept) != "0" || counts(g.stats.Dropped) != "3" {
			t.Errorf("%+v", g.stats)
		}
	})
	t.Run("edge to a node the file does not know", func(t *testing.T) {
		f := *file
		f.Edges = []ConceptFileEdge{{From: "item:elven-cloak", To: "rule:ghost", Type: EdgeGrants}}
		if g := buildConceptGraph(base, &f, nil); counts(g.stats.Kept) != "0" || g.stats.Dropped[EdgeGrants] != 1 {
			t.Errorf("%+v", g.stats)
		}
	})
	t.Run("self link, duplicate and unknown type", func(t *testing.T) {
		f := *file
		f.Edges = []ConceptFileEdge{
			{From: "rule:fury", To: "rule:fury", Type: EdgeLifts}, {From: "rule:fury", To: "rule:hatred", Type: EdgeLifts},
			{From: "rule:fury", To: "rule:hatred", Type: EdgeLifts}, {From: "rule:fury", To: "rule:hatred", Type: "bogus"},
		}
		g := buildConceptGraph(base, &f, nil)
		if g.stats.Kept[EdgeLifts] != 1 || g.stats.Dropped[EdgeLifts] != 2 || len(g.stats.Kept)+len(g.stats.Dropped) != 2 {
			t.Errorf("%+v", g.stats)
		}
	})
	t.Run("parse", func(t *testing.T) {
		if _, err := ParseConceptFile([]byte(`{`)); err == nil {
			t.Error("garbage accepted")
		}
		if f, err := ParseConceptFile([]byte(`{"name":"n","builtFrom":{"vanilla":"a"},"nodes":[{"id":"rule:x","textHash":"h"}],"edges":[{"from":"a","to":"b","type":"grants"}]}`)); err != nil || f.BuiltFrom.Vanilla != "a" || len(f.Edges) != 1 {
			t.Errorf("%+v %v", f, err)
		}
	})
	t.Run("text hash ignores white space", func(t *testing.T) {
		if ConceptTextHash("a  b\n\nc") != ConceptTextHash(" a b c ") || ConceptTextHash("a") == ConceptTextHash("b") {
			t.Error("hash")
		}
	})
}

func TestStatsThroughService(t *testing.T) {
	base := conceptRuleset(t, baseHashes, baseTerms()...)
	c := NewConcepts(fakeRules(map[RulesetHashes]*RuleSet{baseHashes: base}, nil), curatedFor(base), quiet())
	st, err := c.Stats(context.Background(), baseHashes)
	if err != nil || !st.Exact || counts(st.Kept) != "3" {
		t.Errorf("%+v %v", st, err)
	}
}
