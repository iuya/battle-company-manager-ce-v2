# Concept graph (helper popups)

The concept graph links the concepts of the rules files (special rules, wargear, equipment, paths, powers, scenario terms) to each other.
It powers the helper popup: tap a term and read what the rules say about it and what it is linked to.

This page is the contract between the curator (data), the backend (loader) and the frontend (popup).
It is for developers and rules curators.
The popup and its endpoints are described in [frontend-glossary.md](frontend-glossary.md#helper-popup).
The rules files and the generated graph file are described in [definitions.md](definitions.md).

Contents: [What it is](#what-it-is), [Generation, storage and loading](#generation-storage-and-loading), [Go contract](#go-contract), [Size](#size), [Tests](#tests), [Defaults](#defaults).

Where the code lives:

- `internal/core/concept_graph.go` and `concepts.go` (`core.Concepts`).
- The interface `concepts` in `internal/web/concepts.go`, the template `partials/concept.gohtml` and `static/js/concepts.js`.
- The generator `tools/conceptgraph` (shared code in `tools/internal/conceptgraph`), with the curated list `tools/concepts_curated.json`.
- The generated `definitions/concepts/concept_graph.json` and its schema.
- The e2e flow `87-concepts`.

## What it is

### Nodes

Nodes are the terms of the ruleset's glossary.
The glossary already holds the vanilla and Battle Companies special rules, wargear, equipment, paths and powers as extra terms.

A node id is `<kind>:<glossary term id>`.
The kind comes from the term's category:

| kind | category | examples |
|---|---|---|
| `rule` | special rules, path rules | Spy, Fury (X), Cave Dweller |
| `item` | wargear, equipment, creatures, wanderers | Elven Cloak, Shield |
| `path` | paths | Path of the Tactician |
| `power` | magical powers | Immobilise |
| `scenario` | scenario | Seize Ground |
| `term` | anything else (heroic actions, phases, characteristics) | Heroic March, Stand Fast, Move phase |

A node is a reference, not a copy: the text stays in the glossary.
Ambiguous names (Fear, Wound, ...) have one node per meaning and are never auto-linked.

Units are not nodes.
Their cards are in the campaign glossary under their own anchors (`core.UnitAnchor`, [frontend-glossary.md](frontend-glossary.md#glossary-and-item-definitions)).
The glossary linker links a unit named in a rule text, using a `UnitID` segment next to the `TermID` ones ([frontend-glossary.md](frontend-glossary.md#unit-names-in-texts)).
The graph does not do this: the popup's own text is made from the ruleset's glossary file and does not link units, and a unit is never a `mentions` edge.

### Edges

An edge has the fields `from`, `to`, `type` and `via`.

| type | meaning | produced by |
|---|---|---|
| `mentions` | the text of `from` names `to` (whole word, name or alias; first occurrence per target; never to itself) | computed at load by the glossary linker |
| `see` | glossary "see also" | computed at load |
| `grants` | `from` gives its bearer the rule `to` (an item or path gives a rule) | generator, from the item annotations, granted rules and paths, plus curated additions |
| `cancels` | `from` stops `to` applying (a bow cancels a Shield bonus; an exchange replaces a rule) | curated, seeded from rule exchanges, substitutions, overrides and stat-mod cancellations |
| `lifts` | `from` removes a restriction `to` | curated only |
| `requires` | `from` only works with `to` (a heroic action needs Might; a spell needs a Wizard) | curated only |

`via` is a short piece of the source text, or a short description of the data field.
The popup uses it to say why two nodes are linked ("Elven Cloak grants Stealthy").
`cancels`, `lifts` and `requires` always have one.

Direction follows the file dependency rule.
No edge goes from a vanilla or glossary node into a Battle Companies node.
Battle Companies nodes may point to vanilla and glossary nodes.

The 54 shield `cancels` edges are explicit, one per cancelling item and shield.
The popup groups them by display.

### What the graph is for

The engine does not read the graph; it reads the typed data.
The graph serves the popup, and later "what gives me X" questions through reverse edges.

## Generation, storage and loading

### Generator

`make concept-graph` runs `go run ./tools/conceptgraph` and then validates.
The generator reads the three rules files and writes `definitions/concepts/concept_graph.json`.

It does the following:

1. Collects the nodes.
2. Matches names and aliases with the same rules as `core.BuildGlossary`.
3. Derives `grants`, `cancels` and `see` from typed fields.
4. Merges the curated list `tools/concepts_curated.json` (each curated edge has a `via` quote).

The output is deterministic (sorted), so diffs are reviewable.
It holds no rulebook text beyond short `via` quotes.
Run the generator after rebuilding any rules file, because the graph records their hashes.

### File shape

```json
{
  "name": "...",
  "version": "...",
  "builtFrom": { "vanilla": "...", "battleCompanies": "...", "glossary": "..." },
  "nodes": [ { "id": "...", "kind": "...", "name": "...", "aliases": [], "file": "...", "textHash": "..." } ],
  "edges": [ { "from": "...", "to": "...", "type": "...", "via": "..." } ],
  "ambiguous": [ { "alias": "...", "candidates": [] } ]
}
```

- `builtFrom` holds the SHA-256 of each compacted file, the same hashes as the ruleset triple.
- `aliases` and `via` are optional.
- `file` (`vanilla`, `battleCompanies` or `glossary`) says where the term's text comes from, for the direction rule.
- `textHash` is the first 16 hex digits of the SHA-256 of the node's glossary text, with white space collapsed (`core.ConceptTextHash`).
  The glossary text is `GlossaryTerm.Text()`: paragraphs joined by a blank line.
- Only `grants`, `cancels`, `lifts` and `requires` edges are stored.
  `mentions` and `see` are not.
- Node text is not stored.
- `ambiguous[]` is empty for the shipped files.

### Validation

`tools/validate` (`make validate`) checks:

- The schema.
- Every edge endpoint is a node.
- Every node resolves to an existing entry in its file.
- The direction rule.
- No duplicate edges.
- `builtFrom` equals the current hashes, and every node's `textHash` equals the hash of its text in the shipped files (a stale graph fails).
- `cancels`, `lifts` and `requires` have a `via`.
- Ambiguous aliases never appear as `mentions`.

### Loader

`core.Concepts` (`NewConcepts(rules, curatedFile, log)`) builds the graph per ruleset triple on first use, from `Rules.For`.

- The graph is cached per triple and built once under concurrency.
- A failed or panicked build (`safely`) is an error and is not cached.
- At most `MAX_CACHED_RULESETS` graphs stay in memory (least recently used out).
- `cmd/server` reads the curated file at start (`loadConceptFile`).
  A missing file only drops the curated edges.
- `cmd/server` hands the graph to the server with `Server.SetConcepts`.
  Without it the popup routes are not served and terms keep their tooltips.

How the loader treats the edges of the generated file:

1. **Exact `builtFrom` match.**
   The file's edges are taken without per-edge checks (an edge to an unknown node is still refused).
2. **Different triple (a custom ruleset).**
   An edge is kept only when both endpoints still resolve and the `textHash` of the edge's `from` node equals the hash of the ruleset's current text for it.
   An endpoint resolves when it has the same id, or when the file node's name or an alias names exactly one node of the same kind.
   Changed or missing text drops the edge.
   The loader never guesses or repairs.
3. **Always.**
   `mentions` and `see` are computed from the ruleset's own text and unioned with the surviving curated edges, so new rules and renamed terms still link.
   Nodes that exist only in the ruleset get mentions only.
   A fully different ruleset keeps no curated edge, but its mentions work.
4. **Logging.**
   Each build writes one log line with node and edge counts and the kept and dropped counts by type.
   `Concepts.Stats` returns them.

A name or alias claimed by more than one node is ambiguous.
It is never linked (neither as a mention segment nor as an edge), and `Resolve` answers its `Candidates`.

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

`core.RulesetRef` is `core.RulesetHashes`.

Two HTTP routes return the popup fragment:

- `GET /campaigns/{id}/concepts/{conceptID}`
- `GET /campaigns/{id}/concepts/resolve?name=`

Both send `Cache-Control: private, max-age=3600` and an ETag from the ruleset triple.
The pages' terms carry a displayed name, so the server resolves it and the markup needs no id lookup.

The popup's content groups, the folding of chips that share a `via`, its behaviour and its accessibility are in [frontend-glossary.md](frontend-glossary.md#helper-popup).

## Size

The shipped files are about 2.3 MB of JSON.
They hold roughly 250 glossary terms, 105 Battle Companies keywords and hundreds of vanilla rules and items.

The graph is held once per ruleset triple in memory as adjacency maps (a few MB).
Nothing is sent whole to the browser.
The popup loads one concept (a few KB) on demand, and the page keeps the fetched concepts in memory for Back.

## Tests

- `internal/core/concepts_test.go`: ambiguous alias, parameterised rule, unknown id, no graph, modified text, deleted rule, renamed alias, added rule, fully different ruleset, concurrency, bounded cache, panic.
- `internal/web/concepts_test.go`: body, groups, folds, escaping, glossary link, caching and 304, 404, ambiguous and unknown names, non-members and anonymous visitors.
- `tools/internal/conceptgraph/conceptgraph_test.go`.
- The e2e flow `87-concepts`.

## Defaults

- Mentions are computed at load for any ruleset.
- The generated file adds the reviewed edges (`cancels`, `lifts`, `requires`). Custom rulesets reuse them edge by edge, so some curated links may be missing in modified rulesets.
- `mentions` link per term, first occurrence only (as the glossary does).
- The graph file lives in `definitions/concepts/` (a fourth kind of file, never uploaded by users).
- Hover keeps the tooltip on desktop; a click, tap or Enter opens the popup.
