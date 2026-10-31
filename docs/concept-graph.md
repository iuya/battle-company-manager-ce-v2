# Concept graph (helper popups)

A directed, labelled graph over the concepts of the rules files (special rules, wargear, equipment, paths, powers, scenario terms), and the helper popup that uses it: tap a term and read what the rules say
about it and what it is linked to. This page is the contract between the curator (data), the backend (loader) and the frontend (popup). The popup and its endpoints are described in
[frontend.md](frontend.md#helper-popup); the rules files and the generated graph file in [definitions.md](definitions.md).

Code: `internal/core/concept_graph.go` and `concepts.go` (`core.Concepts`), the interface `concepts` in `internal/web/concepts.go`, `partials/concept.gohtml`, `static/js/concepts.js`; the generator
`tools/build_concept_graph.py` with the curated list `tools/concepts_curated.json`; the generated `definitions/concepts/concept_graph.json` and its schema; the e2e flow `87-concepts`.

## What it is

**Nodes** are the terms of the ruleset's glossary, which already holds the vanilla and Battle Companies special rules, wargear, equipment, paths, powers and so on as extra terms. A node id is
`<kind>:<glossary term id>`; the kind comes from the term's category:

| kind | category | examples |
|---|---|---|
| `rule` | special rules, path rules | Spy, Fury (X), Cave Dweller |
| `item` | wargear, equipment, creatures, wanderers | Elven Cloak, Shield |
| `path` | paths | Path of the Tactician |
| `power` | magical powers | Immobilise |
| `scenario` | scenario | Seize Ground |
| `term` | anything else (heroic actions, phases, characteristics) | Heroic March, Stand Fast, Move phase |

Units are not nodes; their cards are in the campaign glossary under their own anchors (`core.UnitAnchor`, [frontend.md](frontend.md#glossary-and-item-definitions)). A unit named in a rule text is linked by the glossary linker (a `UnitID` segment next to the `TermID` ones, [frontend.md](frontend.md#unit-names-in-texts)), not by the graph: the popup's own text is made from the ruleset's glossary file and does not link units, and a unit is never a `mentions` edge. A node is a reference, not a copy: the text stays in the glossary. Ambiguous names (Fear, Wound, ...) have one node per meaning and are never auto-linked.

**Edges** (`from`, `to`, `type`, `via`):

| type | meaning | produced by |
|---|---|---|
| `mentions` | the text of `from` names `to` (whole word, name or alias; first occurrence per target; never to itself) | computed at load by the glossary linker |
| `see` | glossary "see also" | computed at load |
| `grants` | `from` gives its bearer the rule `to` (an item or path gives a rule) | generator, from the item annotations, granted rules and paths, plus curated additions |
| `cancels` | `from` stops `to` applying (a bow cancels a Shield bonus; an exchange replaces a rule) | curated, seeded from rule exchanges, substitutions, overrides and stat-mod cancellations |
| `lifts` | `from` removes a restriction `to` | curated only |
| `requires` | `from` only works with `to` (a heroic action needs Might; a spell needs a Wizard) | curated only |

Direction follows the file dependency rule: no edge goes from a vanilla or glossary node into a Battle Companies node (Battle Companies may point to vanilla and glossary). `via` is a short piece of the source
text (or a short description of the data field) so the popup can say why ("Elven Cloak grants Stealthy"); `cancels`, `lifts` and `requires` always have one. The 54 shield `cancels` edges are explicit (one per cancelling item and shield);
the popup groups them by display.

The engine does not read the graph; it reads the typed data. The graph serves the popup (and later, "what gives me X" through reverse edges).

## Generation, storage and loading

- **Generator** (`make concept-graph`, which also validates): `tools/build_concept_graph.py` reads the three rules files and writes `definitions/concepts/concept_graph.json`. It collects the nodes, matches names and aliases
  with the same rules as `core.BuildGlossary`, derives `grants`, `cancels` and `see` from typed fields, and merges the curated list `tools/concepts_curated.json` (each curated edge with a `via` quote). Output is
  deterministic (sorted), so diffs are reviewable. No rulebook text beyond short `via` quotes. Run it after rebuilding any rules file, because the graph records their hashes.
- **File shape:** `{name, version, builtFrom: {vanilla, battleCompanies, glossary} (sha256 of the compacted files, the same hashes as the ruleset triple), nodes: [{id, kind, name, aliases?, file, textHash}], edges: [{from, to, type, via?}], ambiguous: [{alias, candidates[]}]}`.
  `file` (vanilla, battleCompanies, glossary) says where the term's text comes from, for the direction rule. Only `grants`, `cancels`, `lifts` and `requires` are stored; `mentions` and `see` are not. Node text is not stored. `ambiguous[]` is empty for the shipped files.
  `textHash` is the first 16 hex digits of the SHA-256 of the node's glossary text (`GlossaryTerm.Text()`, paragraphs joined by a blank line) with white space collapsed (`core.ConceptTextHash`).
- **Validation** (`tools/validate_definitions.py`, `make validate`): schema; every edge endpoint is a node; every node resolves to an existing entry in its file; the direction rule; no duplicate edges; `builtFrom` equals
  the current hashes and every node's `textHash` equals the hash of its text in the shipped files (a stale graph fails); `cancels`, `lifts` and `requires` have a `via`; ambiguous aliases never appear as `mentions`.
- **Loader.** `core.Concepts` (`NewConcepts(rules, curatedFile, log)`) builds the graph per ruleset triple on first use from `Rules.For`: cached per triple, built once under concurrency, a failed or panicked build
  (`safely`) is an error and not cached, at most `MAX_CACHED_RULESETS` graphs stay in memory (least recently used out). `cmd/server` reads the curated file at start (`loadConceptFile`; a missing file only drops the
  curated edges) and hands the graph to the server with `Server.SetConcepts`; without it the popup routes are not served and terms keep their tooltips.
  1. **Exact `builtFrom` match:** the file's edges are taken without per-edge checks (an edge to an unknown node is still refused).
  2. **Different triple** (a custom ruleset): an edge is kept only when both endpoints still resolve (same id, else the file node's name or an alias naming exactly one node of the same kind) and the `textHash` of the
     edge's `from` node equals the hash of the ruleset's current text for it. Changed or missing text drops the edge; the loader never guesses or repairs.
  3. **Always:** `mentions` and `see` are computed from the ruleset's own text and unioned with the surviving curated edges, so new rules and renamed terms still link. Nodes that exist only in the ruleset get mentions only. A
     fully different ruleset keeps no curated edge but its mentions work.
  4. One log line per build with node and edge counts and kept and dropped counts by type; `Concepts.Stats` returns them.
- A name or alias claimed by more than one node is ambiguous: it is never linked (neither as a mention segment nor as an edge) and `Resolve` answers its `Candidates`.

## Go contract

Declared in `internal/web` where used, implemented by `core.Concepts`; the mock is generated (`make generate`):

```go
type concepts interface {
    // Concept returns the node, its text as segments (plain text with links) and its edges grouped by type and direction. core.ErrNotFound for an unknown id.
    Concept(ctx context.Context, rs core.RulesetRef, id string) (core.ConceptView, error)
    // Resolve maps a displayed name (with a parameter, an alias, any case) to a concept; ok is false for an unknown name; an ambiguous name returns Candidates and no ID.
    Resolve(ctx context.Context, rs core.RulesetRef, name string) (core.ConceptRef, bool, error)
}

type ConceptView struct {
    ID, Kind, Name string
    Text           []core.ConceptSegment // definition, plain; a mention is a segment carrying ConceptID
    GlossaryID     string                // "" when no glossary term
    Out, In        map[string][]ConceptRef
}
type ConceptRef struct {
    ID, Kind, Name, Via string
    Candidates          []string // ids, set only for an ambiguous name
}
```

`core.RulesetRef` is `core.RulesetHashes`. HTTP: `GET /campaigns/{id}/concepts/{conceptID}` and `GET /campaigns/{id}/concepts/resolve?name=` return the popup fragment, with `Cache-Control: private, max-age=3600` and an ETag from the ruleset triple;
the pages' terms carry a displayed name, so the server resolves it and the markup needs no id lookup. Details of the popup (content groups, folding of chips that share a `via`, behaviour, accessibility) are in
[frontend.md](frontend.md#helper-popup).

## Size

For the shipped files (about 2.3 MB of JSON; roughly 250 glossary terms, 105 Battle Companies keywords, hundreds of vanilla rules and items) the graph is held once per ruleset triple in memory as adjacency maps (a few MB).
Nothing is sent whole to the browser: the popup loads one concept (a few KB) on demand, and the page keeps the fetched concepts in memory for Back.

## Tests

`internal/core/concepts_test.go` (ambiguous alias, parameterised rule, unknown id, no graph, modified text, deleted rule, renamed alias, added rule, fully different ruleset, concurrency, bounded cache, panic),
`internal/web/concepts_test.go` (body, groups, folds, escaping, glossary link, caching and 304, 404, ambiguous and unknown names, non-members and anonymous visitors), `tools/test_concept_graph.py` and the e2e flow `87-concepts`.

## Defaults

- Mentions are computed at load for any ruleset; the generated file adds the reviewed edges (`cancels`, `lifts`, `requires`), reused edge by edge by custom rulesets, so some curated links may be missing in modified rulesets.
- `mentions` link per term, first occurrence only (as the glossary does).
- The graph file lives in `definitions/concepts/` (a fourth kind of file, never uploaded by users).
- Hover keeps the tooltip on desktop; a click, tap or Enter opens the popup.
