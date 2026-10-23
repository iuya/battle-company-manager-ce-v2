# Concept graph (helper popups)

Status: **built: backend, generated file and popup** (bead `6lu.9.1`: `internal/core/concept_graph.go`, `concepts.go`, interface `concepts` in
`internal/web/concepts.go`; bead `6lu.9.3`: `tools/build_concept_graph.py`, `definitions/concepts/concept_graph.json`, validation); the HTTP endpoints and the popup (`6lu.9.2`: `internal/web/concepts.go`, `partials/concept.gohtml`, `static/js/concepts.js`, e2e flow `87-concepts`) are **built**, described in [`frontend.md`](frontend.md) ("Helper popup"). See "As built" below. This page is the contract between the curator (data), the backend
(loader) and the frontend (popup). Current glossary linking, tooltips and the glossary page are described in
[`frontend.md`](frontend.md); the rules files in [`definitions.md`](definitions.md).

## As built (first slice)

- **Nodes** are the terms of the ruleset's glossary (which already holds the vanilla and Battle Companies special rules, wargear, equipment,
  paths, powers... as extra terms). Node id = `<kind>:<glossary term id>`; the kind comes from the term's category (`special rules`/`path rules`
  rule, `wargear`/`equipment`/`creatures`/`wanderers` item, `paths` path, `magical powers` power, `scenario` scenario, anything else term).
  Units are not nodes yet.
- **Edges computed at load** from the ruleset's own text with the glossary linker (`mentions`: whole words, first occurrence per target,
  never to itself; `see`). A name or alias claimed by more than one node is *ambiguous*: it is never linked (neither as a mention segment nor
  as an edge) and `Resolve` answers its `Candidates`.
- **`core.Concepts`** (`NewConcepts(rules, curatedFile, log)`) implements `Concept(ctx, rs, id)` and `Resolve(ctx, rs, name)` and `Stats`. Deviation
  from the sketch below: `rs` is `core.RulesetRef` (= `core.RulesetHashes`) and both methods take a `context.Context`, because the graph is
  built on first use from `Rules.For`. `ConceptView.Text` is `[]core.ConceptSegment` (`Text`, `ConceptID`). Graphs are cached per triple,
  built once under concurrency, a failed or panicked build (`safely`) is an error and not cached, at most `MaxCachedRuleSets` graphs stay in
  memory (least recently used out).
- **Curated file hook:** `ParseConceptFile` reads the shape below; `NewConcepts` takes it (nil today: mentions and see only). The per-edge rule
  is implemented (`keepCurated`): exact `builtFrom` match takes the edges without checks (an edge to an unknown node is still refused);
  otherwise an edge is kept when both endpoints resolve (same id, else the file node's name or an alias naming exactly one node of the same
  kind) and the file's `textHash` of the `from` node equals the hash of the ruleset's text for it; only `grants/cancels/lifts/requires` are read.
  One log line per build with node/edge counts and kept/dropped counts by type. `Concepts.Stats` returns them.
- **`textHash`** (for the generator): the first 16 hex digits of the SHA-256 of the node's glossary text (`GlossaryTerm.Text()`: paragraphs
  joined by a blank line) with white space collapsed to single spaces (`core.ConceptTextHash`).
- Tests: `internal/core/concepts_test.go` (ambiguous alias, parameterised rule, unknown id, no graph, modified text, deleted rule, renamed
  alias, added rule, fully different ruleset, concurrency, bounded cache, panic).

- **Generated file (`6lu.9.3`):** built as described in [`definitions.md`](definitions.md) ("The concept graph file"). Deviations from the sketch below:
  nodes carry `file` (vanilla, battleCompanies, glossary: where the term's text comes from, for the direction rule); `via` of a derived
  `cancels`/`lifts`/`requires` may be a short description of the data field instead of a quote (hand-curated `via` are verbatim quotes, checked);
  the direction rule is exactly "no edge from a vanilla or glossary node into Battle Companies" (decided by the owner);
  the 54 shield `cancels` edges are kept explicit (one per cancelling item and shield): the popup groups them by display; units are not nodes (as in core); `ambiguous[]` is empty for the shipped files.

- **Popup and endpoints (`6lu.9.2`):** `GET /campaigns/{id}/concepts/{conceptID}` and `GET /campaigns/{id}/concepts/resolve?name=` (a displayed
  name, which the page's terms carry, instead of an id; see [`frontend.md`](frontend.md)) serve the popup fragment; `cmd/server` builds
  `core.Concepts` over the loader and the curated file and calls `Server.SetConcepts`. Deviations from the sketch: terms are resolved by name on
  the server (`Resolve`) rather than by an id in the markup, so the page needs no lookup per chip; the ETag also carries the fragment's own hash;
  "Mentioned in" and "Mentions" cut after ten chips (not a request to the server); chips sharing a `via` fold into a closed line with a count
  from three up. An unknown id (404) says "no longer in these rules" without Retry (it would not change); Retry is for load errors. The
  bottom sheet is chosen by width (under 768 px), not by the kind of pointer.

## What it is

A directed, labelled graph over the *concepts* of the three rules files. A node is one named thing; an edge says how
one thing relates to another.

**Nodes** (`id` = `<kind>:<slug of name>`, unique per ruleset; `file` says where the text lives):

| kind | source | examples |
|---|---|---|
| `rule` | vanilla special rules, BC `keywords`, `minorSpecialRules`, parameterised rules | Spy, Fury (X), Cave Dweller |
| `term` | glossary terms (all categories, incl. heroic actions, phases, characteristics) | Heroic March, Stand Fast, Move phase |
| `item` | BC `wargear`, `equipment`, `creatures`, `wanderers`, vanilla wargear | Elven Cloak, Shield |
| `unit` | vanilla and BC units (profile), `unitAliases` | Ranger of Ithilien |
| `path` | BC `paths` | Path of the Tactician |
| `power` | magical powers | Immobilise |
| `scenario` | BC `scenarioDetails` terms | Seize Ground |

A node is a *reference*, not a copy: the text stays in its file and is read through the existing resolution order
(BC, then vanilla, then glossary). Ambiguous names (Fear, Wound, ...) get one node per meaning and are never
auto-linked (see Edges).

**Edges** (`from`, `to`, `type`, `via`):

| type | meaning | produced by |
|---|---|---|
| `mentions` | the text of `from` names `to` (by name or alias, whole word) | generator, by matching |
| `grants` | `from` gives its bearer the rule `to` (item or path gives a rule, a company gives a unit) | generator from `gearAnnotations.grants`, `grantedSpecialRules`, `paths`, plus curated additions |
| `cancels` | `from` stops `to` applying (a bow cancels a Shield bonus; an exchange replaces a rule) | curated, seeded from `ruleExchanges`, `ruleSubstitutions`, `overrides`, StatMods cancellations |
| `lifts` | `from` removes a restriction `to` (a rule lets a Hero take what limits forbid) | curated only |
| `requires` | `from` only works with `to` (Heroic action needs Might; spell needs a Wizard) | curated only |
| `see` | glossary "see also" | copied from the glossary `see` field |

Direction follows the file dependency rule: no edge goes from a vanilla or glossary node into a Battle Companies node (BC may point to
vanilla and glossary; edges between vanilla and glossary nodes, such as Heroic Actions requires Might, are allowed). `via` (optional) is a short piece of the source text so the
popup can say why ("Elven Cloak grants Stealthy"). Self links and duplicates are dropped; at most one `mentions` edge
per target per node (like glossary linking: first occurrence).

## What it is used for

1. **Helper popup on any term.** Tapping a term chip (special rule, item, heroic action, power, path, injury, scenario
   term; wherever a name is shown today with a tooltip) opens a popup: name, kind badge, text (plain), then groups
   "Grants", "Cancelled by", "Mentions", "Mentioned in", "See also" listing linked concepts. Tapping one swaps the popup
   content to that concept (one-click navigation, with a Back step), without leaving the page.
2. **Glossary jump.** Every popup has "Open in glossary" (existing `#t-...` fragment links, `hx-boost="false"`); for a
   concept with no glossary term the link is hidden.
3. **Armoury / company tooltips.** The tooltips that exist today (`data-tip`, `gearAnnotations.definition`, glossary
   preview) keep working on hover; the graph replaces their *source of the text and links* so one lookup serves all.
   On touch screens the tooltip becomes the popup.
4. Later: "Mentioned in" answers "what gives me X" (reverse edges) for the company page.

Out of scope: rules computation. The engine does not read the graph; it reads the typed data.

## Where it is generated, stored, loaded

- **Generator** (`tools/`, owned by the definitions-curator): `tools/build_concept_graph.py` reads the three files and
  writes `definitions/concepts/concept_graph.json` plus `concept_graph.schema.json`. Steps: collect nodes; match
  names/aliases with the same rules as `core.BuildGlossary` (whole word, single words case-sensitive, ambiguous aliases
  skipped and listed in `ambiguous[]` with their candidates); derive `grants`/`cancels`/`see` from typed fields; merge
  the curated list `tools/concepts_curated.json` (`cancels`, `lifts`, `requires`, extra `grants`; each with a `via`
  quote). Output is deterministic (sorted) so diffs are reviewable. No rulebook text beyond short `via` quotes.
- **File shape:** `{name, version, builtFrom: {vanilla, battleCompanies, glossary} (sha256 of the compacted files, the
  same hashes as the ruleset triple), nodes: [{id, kind, name, aliases?, textHash}], edges: [{from, to, type, via?}] (curated and derived `grants`/`cancels`/`lifts`/`requires`; mentions and `see` are not stored), ambiguous:
  [{alias, candidates[]}]}`. Node text is not stored (looked up from its file).
- **Validation** (`tools/validate_definitions.py`): schema; every edge endpoint is a node; every node resolves to an
  existing entry in its file (cross reference by name); direction rule (no edge from vanilla/glossary into BC); no
  duplicate edges; `builtFrom` equals the current hashes and every node's `textHash` equals the hash of its text in the shipped files (stale graph fails the check); `cancels/lifts/requires` have a
  `via`; ambiguous aliases never appear as `mentions`.
- **Loader:** most custom rulesets modify or extend the shipped ones, so the curated file is reused for them where it
  still holds. `core.Rules` builds the graph per ruleset triple (cached with the other maps, built once):
  1. **Exact `builtFrom` match:** fast path, take the file as is (no per-edge checks); `mentions`/`see` are still
     recomputed from the ruleset's own text, so file and load agree.
  2. **Different triple:** start from the curated nodes and edges (`grants`, `cancels`, `lifts`, `requires`) and keep an
     edge only if (a) both endpoints still resolve by id, name or alias in the ruleset's files, and (b) the text that
     states the relation is unchanged: the file stores a short `textHash` per node (hash of the node's rule or
     definition text in its source file) and the hash of the edge's `from` node must equal the hash of the ruleset's
     current text for it. Changed or missing text drops the edge; the loader never guesses or repairs.
  3. **Always:** `mentions` and `see` are computed from the ruleset's own text (never read from the file) and unioned
     with the surviving curated edges, so new rules and renamed terms still link. Nodes that exist only in the ruleset
     (additions) get mentions only. A fully different ruleset keeps no curated edge but its mentions work.
  4. One log line per build: ruleset hash, curated edges kept and dropped (by type). REVIEW: if cheap, show the counts
     where the ruleset info is shown to admins.
- **Consumer interface** (declared in `internal/web`, implemented by core; mock via `make generate`):

```go
// Concepts is the concept graph of one ruleset.
type Concepts interface {
    // Concept returns the node, its text as segments (plain text with glossary links) and its edges grouped by type
    // and direction. core.ErrNotFound for an unknown id.
    Concept(rs core.RulesetRef, id string) (core.ConceptView, error)
    // Resolve maps a displayed name (with a parameter, an alias, any case) to a concept id; ok is false for an
    // unknown name; ambiguous names return Candidates and no ID.
    Resolve(rs core.RulesetRef, name string) (core.ConceptRef, bool)
}

type ConceptView struct {
    ID, Kind, Name string
    Text           []core.Segment   // definition, plain; mentions are segments carrying ConceptID
    GlossaryID     string           // "" when no glossary term
    Out, In        map[string][]ConceptRef // keyed by edge type
}
type ConceptRef struct {
    ID, Kind, Name string
    Via            string
    Candidates     []string         // ids, set only for an ambiguous name
}
```

- **HTTP:** `GET /campaigns/{id}/concepts/{conceptID}` returns the popup partial (htmx, `hx-boost="false"` unaffected);
  `Cache-Control: private, max-age=3600` with the ruleset triple hash as ETag. Unknown id: 404 partial "no longer in
  these rules". A ruleset with no graph (load failure): popups fall back to today's tooltip.

## Size and caching

Estimate for the shipped files (about 2.3 MB of JSON, roughly 252 glossary terms, 105 BC keywords, hundreds of vanilla
rules and items): about 1.5-2k nodes and 6-10k edges, a graph file of about 0.5-1 MB uncompressed, 100 KB gzipped. Held
once per ruleset triple in memory as adjacency maps (a few MB). Nothing is sent whole to the browser: the popup loads
one concept (a few KB) on demand, and the page keeps a small in-memory cache of fetched concepts (Alpine store) for
Back/forward.

## Acceptance criteria

Curator
- [x] `make concept-graph`/`python3 tools/build_concept_graph.py` regenerates the graph deterministically; running twice gives no diff.
- [x] `validate_definitions.py` passes with the checks above and fails on each (tested: dangling edge, stale `builtFrom`,
      BC target from vanilla, missing `via`, ambiguous alias auto-linked).
- [x] Every glossary term, BC keyword and BC item is a node (the ruleset's glossary holds them all); `ambiguous[]` is empty.
- [ ] Spot check of 30 edges against the rules is recorded; wrong ones are fixed by the curated list, not by hand edits.

Backend
- [ ] Exact `builtFrom` match: file edges used without per-edge checks; mentions/see recomputed from text and equal to what the shipped files give.
- [ ] Different triple: curated edges kept only when both endpoints resolve and the `from` node's `textHash` matches; the rest dropped; mentions/see recomputed and unioned; additions get mentions only; a log line reports kept/dropped counts.
- [ ] `Concept` and `Resolve` as above, concurrent safe, built once per triple; unit tests against a fixture ruleset
      (ambiguous alias, parameterised rule, unknown id, no graph) and these fixture rulesets, each asserting the kept/dropped
      counts: a modified text (its edges dropped, others kept), a deleted rule (edges to and from it dropped), a renamed
      alias (curated edge kept via the node, the mention follows the new alias), an added rule (mentions only), and a
      fully different ruleset (no curated edge survives, mentions work).
- [ ] `make generate` produces the `Concepts` mock; `docs/definitions.md` and `docs/frontend.md` updated.

Frontend
- [x] At 360 px the popup is a bottom sheet (max 70% height, scrolls inside, closes with Esc, the backdrop, or a close
      button, focus trapped and restored); at 768 px and up a floating card next to the term.
- [x] Hover/focus keeps today's tooltip on desktop; tap/Enter opens the popup. Works in light and dark, semantic tokens only.
- [x] States: loading skeleton; empty (a concept with no links shows only its text); error/404 ("no longer in these
      rules", Retry); ambiguous name lists candidates, none chosen automatically.
- [x] One click on a linked concept swaps the content, Back returns; "Open in glossary" hidden when absent; no
      horizontal scroll; text is plain (no HTML from data).
- [x] Permission: popup content is public rules text; no per-campaign data beyond the ruleset the viewer already sees.

## First slice (smallest useful)

Glossary-aware popup for **special rules and heroic actions on unit/hero cards**, mentions only, computed at load time by
the existing linker, no JSON file yet: a `GET /campaigns/{id}/concepts/{id}` popup showing text and the outgoing
`mentions` and `see` chips with one-click navigation and "Open in glossary". That is the backend loader bead (B) plus
the frontend popup (C) over the current `core.Glossary`; it delivers the user-visible value. The curator bead (A)
(generated file, `grants`/`cancels`/`lifts`/`requires`, validation) follows and enriches the same popup with groups.

## REVIEW items (defaults chosen)

- REVIEW: the original bead default was "edges computed at load, not stored". Default now: both. Mentions are computed at
  load for any ruleset; a curator-generated, validated file adds the reviewed edges (`cancels`, `lifts`, `requires`),
  reused per edge by custom rulesets (next item). Easy to drop the file and keep only load-time edges.
- REVIEW: ambiguous aliases (Fear, Wound, ...) are not auto-linked; the popup lists candidates.
- REVIEW: `mentions` links per term, first occurrence only (as the glossary does), not every occurrence in the text.
- REVIEW: custom rulesets reuse the curated file edge by edge (endpoint resolves and the `from` text hash is unchanged);
  a changed text drops its edges rather than guessing, so some curated links may be missing in modified rulesets.
- REVIEW: graph file location `definitions/concepts/` (a fourth kind of file, never uploaded by users).
- REVIEW: tap opens the popup on touch; hover keeps the old tooltip on desktop.
- REVIEW: unit nodes: only named units that appear in companies; vanilla units that no company uses are skipped.
