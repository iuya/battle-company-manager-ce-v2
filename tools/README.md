# Definitions tooling

Scripts that build and check the JSON rule definitions in `definitions/`. They use only the Python 3 standard
library (`jsonschema` is optional, for the schema checks).

## Battle Companies json

`definitions/battle_companies/battle_companies.json` is **generated** from the community document
"Battle Companies: 2nd Community Edition - Core Rules". The document is copyrighted material and must never be
committed: keep it under `sources/` (gitignored).

```
# 1. export the Google Doc as plain text (File > Download > Plain text, or the /export?format=txt URL)
#    into sources/battle_companies_rules.txt
# 2. regenerate
python3 tools/build_battle_companies.py
# 3. check schema + cross references against the vanilla file
python3 tools/validate_definitions.py
```

The generator writes three things: the file the app loads, `tools/data/*.json` (the curator's provenance and audit records: duplicate
decisions, aura audit, gear evidence, option-sale sources, fill-in reviews, Tabletop Simulator reasons, single `source` / `note` strings) and
`definitions/battle_companies/reference/*.json` (prose no code reads: influence, gold, designer notes). `tools/bc/curated.py` does the cut
(`split`) and the way back (`attach`, the "audit view" the validator and the Python tests read); `tools/test_curated.py` proves that both agree.
`validate_definitions.py` takes `--data-dir` and reports a missing or stale record as an error. To change a record, change its table in
`tools/bc/` (`DUPLICATE_DECISIONS` in `core.py`, `EXCLUDED` in `auras.py`, ...) and rebuild; never edit `tools/data/` by hand.

`validate_definitions.py` also range-checks the engine parameters of the file (`engine_parameter_errors`: officers, hero start, starting influence, recruitment die, XP threshold, limits, follower limits, wargear rating threshold; table in docs/definitions.md).

`validate_definitions.py` also checks `definitions/battle_companies/battle_companies.v2.schema.json` (the lean, schemaVersion 2 schema): a valid
schema that accepts the Go valid fixtures. That file is **generated from the Go model**, not by `tools/`: regenerate it with
`go test ./internal/core/lean -run TestSchemaUpToDate -update-schema` (docs/definitions.md, "The schema of the lean format").

What is parsed from the text (`tools/bc/`): hero paths and their 2d6 progression tables, equipment, creatures,
wanderers, the 37 company lists (starting company, reinforcement/special charts, hero upgrades, advancements, unique
wargear, company rules) and the 18 scenarios. The Victory Point objectives, rewards and Attacker/Defender roles of the scenarios, as input fields
for the match page, are a hand-written table in `tools/bc/scenario_rules.py` (checked against the parsed bullets: a
source change makes the build fail); the per-match input (`campaign.matchInput`) is `match_input()` in `tools/bc/core.py`. The core rule tables (injuries, progression, influence, rating,
Against the Odds, Break Point in play, injury-roll items, scenario selection) are transcribed in `tools/bc/core.py` (`AGAINST_THE_ODDS`, `BREAK_POINT`, `AFTERMATH_EFFECTS`): edit them there when a new version of the
document changes them.

When a new version of the document comes out, re-export it and run the build. The parsers assert on the structure they
rely on (11 rows per path, a 1-6 table per chart...) and the validator reports every unit, rule or power that no
longer resolves against the vanilla file, so layout changes surface as errors instead of silent data loss. If the
document is restructured drastically, adjust the matching parser in `tools/bc/`.

Names in the document are resolved to the vanilla keys (`tools/bc/vanilla.py`, which also holds the alias table for
spellings that differ). Anything that still cannot be resolved is flagged `"missingInVanilla": true` in the output.

### Gear annotations

`data.gearAnnotations` (classes, granted rules and missile profile of every item a Battle Company model can carry) is
produced by `tools/bc/gear.py` during the build: it collects the reachable items, derives candidates from the item
texts and the vanilla type, then applies the hand-written `DECISIONS` table (`curated`). The build also writes
`tools/gear_review.md` (one row per item, coverage by class, items classified `other`, items with grants, the open
questions about grants and the spellings merged or aliased) for an independent review. To change a class or a grant,
edit `DECISIONS`/`PROPOSALS`/`ITEM_ALIASES`/`KEEP_ANNOTATED` in `tools/bc/gear.py` and rebuild. `validate_definitions.py` checks that
every reachable item has exactly one annotation (its coverage walk is independent of the generator), that every grant resolves to a rule, the class combinations, `ranged` and `grantedTo`, and the company `limitExemptions`. Limit exemptions live in `LIMIT_EXEMPTIONS` in `tools/bc/core.py`.

Item identity tables in `tools/bc/gear.py`: `RELATED` (related but distinct items: Elf Bow / Longbow; emitted as `relatedTo`), `MODIFIERS` and `MODIFIED` (item modifiers: an item is a `base` plus `modifiers` such as Elven (poison is not a modifier: it is a rule of the unit, `Poisoned Attacks (<weapon>)`; see docs/definitions.md **Poison**; `GRANTS_RULE_FOR_CHOSEN_WEAPON` describes Envenom Weapon); the modifier's rulebook text is added to the tooltip, `pricedAs` is derived; `NOT_SOLD` lists Elven starting wargear that is never sold) and `READ_AS` (company lists that write a generic item where the unit's profile has a specific one; emitted as `data.wargearReadAs` and applied to the lists). The validator checks that every name of a wargear price row has its own annotation, that no spelling belongs to two items, that `relatedTo` is symmetric, that bows have `moveAndShoot` and crossbows (24" S4) `noFireAfterMove`, that bases, modifiers and the derived `pricedAs` agree, and that a Pike is two-handed with Support depth 2 while a Spear has depth 1.

Path tooltips: `add_offers` in `tools/bc/paths.py` derives `data.paths[].offers` (starting Heroic Action, Mastery rule, own special rules, signature characteristics, choices and a ready-made `summary`) from each path's progression table.

`data.ttsModels` (stand-ins, fighting mounts, cavalry riders, option spellings, default options and marker options for the Tabletop Simulator mod) comes from the curated tables in `tools/bc/tts.py` (a reason each); the validator checks it against the vanilla file and, if present, the local gitignored `sources/tts/models.json`; tests in `tools/test_tts_models.py`. `python3 -I tools/extract_tts_models.py [save.json]` writes that local catalogue from a Tabletop Simulator saved game (default: the newest `~/Library/Tabletop Simulator/Saves/TS_Save_*.json`; it refuses to write unless `sources/tts/` is git-ignored; tests: `tools/test_extract_tts_models.py`). `data.ruleTargets` (who a keyword rule works against or for) comes from `tools/bc/rule_targets.py`; the validator checks it; tests: `tools/test_rule_targets.py`. `data.auras` (the aura ranges drawn as rings in Tabletop Simulator and shown as chips on the unit cards) comes from the curated table in `tools/bc/auras.py` (a reason each; `excluded` with categories; `unreachable` computed); the validator (`aura_errors`) checks that every radius is written in the source text and that every reachable rule, power, item or heroic action with a distance is classified; tests: `tools/test_auras.py`; contract in docs/definitions.md (Aura ranges).

`data.reservedOptions` is generated from vanilla (`reserved_options` in `tools/bc/vanilla.py`; the validator recomputes it). Printing errors of the Battle Companies document are corrected when it is read, listed in `SOURCE_TYPOS` in `tools/bc/core.py` (today "Brandbybuck" -> "Brandybuck", the Last Alliance's 2018 "High Elf Warrior" -> "Rivendell Warrior" "Giant Mirkwood Spider" -> "Mirkwood Giant Spider" and the 2018 names Grimhammer, Warrior of Erebor, Iron Hills Dwarf, Mirkwood Cavalry, Abrakhân Guard, Rhûnish War Drake -> their 2024 units); `sources/` is never edited.

`tools/company_text_checks.py` (called by `validate_definitions.py`, tests in `tools/test_company_text_checks.py`) checks the company texts: no rule repeated inside a company's `specialRules`/`notes`, and no parse artefact (unbalanced quotes, an epigraph glued to a rule, a text starting or ending mid-sentence, a page header). A quoted title with a flavour sentence at the head of the company rules becomes `epigraph`.

### How the file relates to vanilla

* **Battle Companies always wins.** A BC keyword with the same name as a vanilla one carries `overrides`: it is a
  generalisation of the vanilla rule (a named character becomes "this Hero") and replaces it. The validator rejects an
  undeclared clash. The vanilla `scenarios`/`scenarioScoring` are ignored; the BC ones are used.
* **Resolution order** (`resolution` in the json): a definition embedded in the entity that uses it wins over the same
  term in `data.keywords`, Battle Companies wins over vanilla, and vanilla is only a reference store that is never
  modified. The vanilla `scenarios`/`scenarioScoring` are not used.
* **Vanilla names defined twice**: the loader takes the **first occurrence** (`resolution.firstOccurrenceWins`), a rule
  of thumb confirmed against the official rules (the later copies are stale or incomplete). Every duplicate whose
  entries differ, and every one this file needs (directly, or through the vanilla units it uses), is listed in `data.vanillaAmbiguities` as `equivalent`
  (only wording differs), `overridden` (BC defines the name itself), `firstWins` (they really differ, nothing is
  copied) or `pinned` (the rule of thumb fails; `occurrence` says which vanilla entry wins). The decisions live in
  `DUPLICATE_DECISIONS` in `tools/bc/core.py`; the build stops on a new, unreviewed duplicate (matching and occurrence order
  are the Go loader's: case-insensitive, units heroes list first) and the validator checks the list against the vanilla
  file. Decisions marked `REVIEW:` in their note took the default (first wins) where the sources disagree on a value.
* **Parameterised rules** (`data.parameterisedRules`): `Hatred (X)`, `Poisoned Attacks (X)`, `Master of Battle (X+)`,
  `Dominant (X)`, `Fury (X)`, `Combat Synergy (X)`, `Terror (X)` say what X means (`keyword`, `weapon`, `rollTarget`,
  `number`, `companyHero`) and who picks it. Minor special rules point to them with `parameterKind`.
* **Company rule rewiring**: `ruleSubstitutions` (the Leader counts as Faramir for Loyal to the Captain),
  `ruleExchanges` (Hatred (Rohan) becomes Hatred (Hobbit)) and `grantedSpecialRules` (free rules).
* **Units written under another name** resolve to a vanilla unit: either the base unit with an option pre-selected
  (`selectedOptions`, e.g. Tookish Hunter = Hobbit Archer + Tookish Hunter) a name the document prints for a vanilla unit (`LEGACY_NAMES` in
  `tools/bc/vanilla.py`), or a vanilla unit plus a **delta** (`UNIT_DELTAS`; `addSpecialRules` / `addKeywords`, e.g.
  Ranger of Ithilien = Ranger of Gondor + Loyal to the Captain). No new units are created. The printed name stays as `label` and every company lists them in `unitAliases` with an
  explanation.

## Rules glossary

`definitions/glossary/mesbg_glossary.json` complements the vanilla json with concepts of the rules manual that it leaves
out (Heroic Actions, Stand Fast, Shielding, the turn phases...), taken from the 2024 rules manual, with the printed page of the rulebook (`rulebookPage`): verbatim for short sections, a brief summary for long chapters (see below). It is
independent of the Battle Companies file; `see` links connect related terms. The manual is copyrighted and stays local; the json may be committed.

```
python3 tools/pdf2md.py ~/Downloads/MESBG_RULES.pdf sources/mesbg_rules.md   # needs pymupdf; use the 2024 manual
python3 tools/build_glossary.py
```

The terms are listed in `tools/glossary_terms.py` (manual heading titles, or inline paragraphs). Conventions: one
rulebook heading = one term; `aliases` only for real synonyms (a related concept is its own term, linked with `see`);
every term has `see` links (the phases link to what happens in them); entries are brief reminders. `first=N` keeps only
the first N paragraphs of the headings' text (an extraction boundary: it stops the next chapter or the nested sub-sections
from leaking in, it is not a summary). Long rule chapters are marked `synth=True`: their brief entry is written by the curating
agent in `tools/glossary_summaries.json` (`summarized: true` in the output; the build reports marked terms that have no
summary yet and falls back to the verbatim text). `python3 tools/build_glossary.py --uncapped out.json` dumps the full
verbatim text of every term to read when writing or reviewing a summary.

## Reference tables

`tools/glossary_reference.py` holds the hand-transcribed To Wound Chart (`reference.toWound` in the glossary file, written by
`build_glossary.py`) and its checks (complete 10x10 grid, allowed values, monotonic), used by `validate_definitions.py`; tests in
`tools/test_to_wound.py`. After changing it: rebuild the glossary, then `python3 tools/build_concept_graph.py`.

## Concept graph

`python3 tools/build_concept_graph.py` writes `definitions/concepts/concept_graph.json` (`--check`: only say whether it is up to date) from the
three shipped files and `tools/concepts_curated.json`; run it after rebuilding any of them, because the graph records their hashes and
`validate_definitions.py` fails on a stale one (`make concept-graph` does both). It prints what it could not put in the graph (names that
resolve to no term, edges that would point from vanilla/glossary into Battle Companies). The library is `tools/concept_graph.py`
(a port of the app's glossary build, checked by `internal/core/concept_graph_file_test.go`); tests: `python3 -m unittest discover -s tools -p 'test_*.py'`.
Details: [`docs/definitions.md`](../docs/definitions.md) ("The concept graph file").

## Rulebook to Markdown (local reading only)

`python3 tools/pdf2md.py ~/Downloads/MESBG_RULES.pdf sources/mesbg_rules.md` converts the MESBG rules PDF
(`pip install pymupdf`) into page-annotated Markdown. The output is copyrighted: it stays in `sources/`.

## dice3d

`tools/dice3d` builds `internal/web/static/js/dice3d.js` (three.js and cannon-es cut down by esbuild to what the 3D dice use): `make dice3d`.
It needs node and npm; the built file is committed.
