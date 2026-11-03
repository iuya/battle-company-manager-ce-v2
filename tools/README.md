# Definitions tooling

Scripts that check and build the JSON rules data in `definitions/`, for people who edit that data or maintain the tooling.

The scripts use the Python 3 standard library.
`jsonschema` is optional, for the schema checks.
For the Battle Companies file they also call the Go code of the app, so `go` must be on the `PATH`.

## The Battle Companies file is edited by hand

`definitions/battle_companies/battle_companies.json` (lean format, `schemaVersion` 2) is the **source of truth**, and nothing generates it.
Open it in an editor (the schema `battle_companies.schema.json` gives completion and the closed vocabularies), change it, then check it:

```bash
make validate                                                                # all four files (see below)
make check-ruleset FILE=definitions/battle_companies/battle_companies.json   # only the Go validator, with the report the upload shows
make concept-graph                                                           # after a change to one of the three files: rebuild the concept graph and validate
make test                                                                    # Go tests (including the golden snapshots) and the Python tests
```

A change to the rules text is a change to the file.
A change that moves the generated views (armoury, auras, Tabletop Simulator lists) shows up in the golden snapshots under `internal/core/testdata`.
Review that diff and regenerate the snapshots as [`docs/testing.md`](../docs/testing.md) says.

Where to read more:

- What the file may say, key by key: the generated [reference](../docs/customizing-rulesets-reference.md).
- How to write new rules, items, units, paths, companies and scenarios: the [guide](../docs/customizing-rulesets.md).
- How the app reads the file: [`docs/definitions.md`](../docs/definitions.md).

The rulebook ("Battle Companies: 2nd Community Edition - Core Rules") is copyrighted and must never be committed.
Keep any local copy under `sources/` (gitignored).

## The schema, the reference and the examples (`make ruleset-reference`)

The schema `battle_companies.schema.json` is generated from the Go model and the descriptions in `internal/core/lean/schema_docs_test.go`.
The reference is generated from the schema.

```bash
go test ./internal/core/lean -run TestSchemaUpToDate -update-schema   # after changing the model or a description
make ruleset-reference                                                # runs tools/gen_ruleset_reference.py: rewrites docs/customizing-rulesets-reference.md
python3 tools/gen_ruleset_reference.py --check                        # exit 1 when the committed reference is stale
```

`tools/test_ruleset_reference.py` (part of `make test`) fails when the reference is stale or leaves a key out.

The complete example files the guide quotes are in `examples/customizing-rulesets/`.
`cmd/rulecheck/examples_test.go` (part of `go test ./...`) runs every example through the checker and the loader, adds its entries to a copy of the shipped file, and fails when the guide quotes a block that is not in its example.

## validate_definitions.py (`make validate`)

```bash
python3 tools/validate_definitions.py [--vanilla F] [--battle-companies F] [--glossary F] [--concept-graph F] [--schema F]
```

The script exits 1 on errors. It runs these checks:

| Check | By |
|---|---|
| JSON Schema of the vanilla file, the glossary, the concept graph and the Battle Companies file (`battle_companies.schema.json`, which must also accept the Go valid fixtures in `internal/core/lean/testdata/valid`) | `jsonschema` (skipped with a note when it is not installed) |
| Battle Companies file: cross references (every unit, rule, power, item, path and scenario name resolves against the file, vanilla and the glossary), closed vocabularies, charts, engine parameter ranges, `extends` | `go run ./cmd/rulecheck`, the Go validator the upload runs |
| Company texts (a rule repeated inside a company, parse artefacts in texts copied from the rulebook), aura radii against the texts that name them, the keywords of the rule targets against the vanilla keywords, the Tabletop Simulator models against the local catalogue `sources/tts/models.json` when that gitignored file exists | `tools/lean_checks.py` |
| Glossary: unique terms, `see` links, the To Wound Chart (`tools/glossary_reference.py`) | `validate_definitions.py` |
| Concept graph: nodes, text hashes and `builtFrom` against the three files, edge rules | `tools/concept_graph.py` |

**Why the script calls the Go validator.**
The Go code in `internal/core/lean` decides whether an upload is accepted.
It reports the path, the closest name and a hint, and it is tested by `internal/core/lean/testdata/invalid`.
A Python copy of the cross references and parameter ranges could only drift from it.
The script therefore calls the Go validator and keeps in Python only what Go does not check (`tools/lean_checks.py`, tested by `tools/test_lean_checks.py`).

## Rules glossary

`definitions/glossary/mesbg_glossary.json` complements the vanilla file with concepts of the 2024 rules manual that it leaves out (Heroic Actions, Stand Fast, Shielding, the turn phases...).
Each term carries the printed page of the rulebook (`rulebookPage`).
Short sections are verbatim; long chapters have a brief summary.

The glossary is independent of the Battle Companies file, and `see` links connect related terms.
The manual is copyrighted and stays local; the JSON may be committed.

The glossary is generated from the rulebook:

```bash
python3 tools/pdf2md.py ~/Downloads/MESBG_RULES.pdf sources/mesbg_rules.md   # needs pymupdf; use the 2024 manual
python3 tools/build_glossary.py
```

The terms are listed in `tools/glossary_terms.py` (manual heading titles, or inline paragraphs). The conventions:

- One rulebook heading is one term.
- `aliases` only for real synonyms. A related concept is its own term, linked with `see`.
- Every term has `see` links (the phases link to what happens in them).
- Entries are brief reminders.
- `first=N` keeps only the first N paragraphs of the heading's text. It is an extraction boundary that stops the next chapter or nested sub-sections from leaking in, not a summary.
- Long rule chapters are marked `synth=True`. Their brief entry is written by the curating agent in `tools/glossary_summaries.json` (`summarized: true` in the output).
  The build reports marked terms that have no summary yet and falls back to the verbatim text.

`python3 tools/build_glossary.py --uncapped out.json` dumps the full verbatim text of every term, to read when writing or reviewing a summary.
Rebuilding with the same sources gives the same file.

## Reference tables

`tools/glossary_reference.py` holds the hand-transcribed To Wound Chart (`reference.toWound` in the glossary file, written by `build_glossary.py`).
It also holds the chart's checks (complete 10x10 grid, allowed values, monotonic), which `validate_definitions.py` uses.
Tests: `tools/test_to_wound.py`.

After changing it, rebuild the glossary, then run `make concept-graph`.

## Concept graph

`python3 tools/build_concept_graph.py` writes `definitions/concepts/concept_graph.json` from the three shipped files and `tools/concepts_curated.json`.
With `--check` it only says whether the file is up to date.

Run it (or `make concept-graph`) after changing any of the inputs.
The graph records their hashes, so `validate_definitions.py` and `TestShippedConceptGraphMatchesCore` fail on a stale one.
It prints what it could not put in the graph: names that resolve to no term, and edges that would point from vanilla or the glossary into Battle Companies.

- The **nodes** are the terms of the glossary that the app builds from the three files.
  The app's own loader makes them: `go run ./tools/conceptnodes` (`tools/conceptnodes/main.go`) calls `core.LoadRuleSet` and prints the terms and the classes of every item as JSON.
  Python never repeats the glossary logic.
  `internal/core/concept_graph_file_test.go` checks every node of the file against core.
- The **edges** are derived by `tools/concept_graph.py` from the lean file:
  item `grants`, path heroic actions and rule rows, shield `abilities` (`defenceBonus` cancelled by classes and items and lifted by rules), `requiresMount`, and `who` hero/Cavalry.
  The reviewed edges of `tools/concepts_curated.json` are added; each `via` must quote one of the two texts.
  `mentions` and `see` are not stored: the app computes them.

Tests: `tools/test_concept_graph.py`. Details: [`docs/definitions.md`](../docs/definitions.md) ("The concept graph file").

## Curator records (`tools/data/`)

Three records of decisions, kept as documentation. No code reads them (neither the loader nor the tests).

- `duplicate_decisions.json`: the 91 reviewed duplicates of the vanilla file (kind, name, status, occurrence and a note).
  A note starting `REVIEW:` took the default, first wins, where the sources disagree on a value.
  The decisions that matter to the engine are in the vanilla file as `vanilla.pins`.
- `aura_audit.json`: the rules, items and powers that name a distance but draw no ring (`excluded` with a category and a reason, `outOfScope`, `unreachable`), and the reason of each ring.
  Read it before adding a ring: it says why, for example, receiver-side rules and weapon ranges are not rings.
- `tts_provenance.json`: the reason of each Tabletop Simulator mapping (stand-ins, mounts, riders, option names).

Edit them by hand when a decision changes.

## Tabletop Simulator catalogue

`python3 -I tools/extract_tts_models.py [save.json]` writes the local catalogue `sources/tts/models.json` from a Tabletop Simulator saved game.

- The default save is the newest `~/Library/Tabletop Simulator/Saves/TS_Save_*.json`.
- It refuses to write unless `sources/tts/` is git-ignored.
- It does not read the Battle Companies file.

Tests: `tools/test_extract_tts_models.py`.
The validator checks the models the Battle Companies file names against the catalogue ([`docs/tabletop-simulator.md`](../docs/tabletop-simulator.md)).

## Rulebook to Markdown (local reading only)

`python3 tools/pdf2md.py ~/Downloads/MESBG_RULES.pdf sources/mesbg_rules.md` converts the MESBG rules PDF (`pip install pymupdf`) into page-annotated Markdown.
The output is copyrighted: it stays in `sources/`.

## Other tools

- `tools/dice3d` builds `internal/web/static/js/dice3d.js`: three.js and cannon-es cut down by esbuild to what the 3D dice use.
  Run `make dice3d` (needs node and npm); the built file is committed.
- `build_icons.py` makes the armoury icons (ImageMagick and potrace).
- `check_theme_contrast.py` checks the theme tokens.
- `tools/gear_review/findings.md` is the independent reviewer's report on the gear classes, kept as the record of that review.

## Python tests

`make test` runs the Python tests after the Go tests:

```bash
cd tools && python3 -m unittest test_extract_tts_models test_to_wound test_lean_checks test_concept_graph test_ruleset_reference
```

`test_lean_checks` and `test_concept_graph` call `go run` for the glossary nodes, so `go` must be on the `PATH`.
