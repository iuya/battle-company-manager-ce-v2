# Optimization report

What the optimization work of release 2.0.0 changed, in numbers. Every figure was taken on one machine with the tools of this repository
([Method](#method), [Reproducing the numbers](#reproducing-the-numbers)). Three states are compared:

- **v1.3.0**, the release before the lean data format (column "v1.3.0", only where a number could be taken from that release's files; `-` otherwise);
- **Baseline**, the local tag `1.5.0-pre-optimization` (release/v2.0.0 at `b3300b3`, after "practice matches", PR #12): the lean data format is in, nothing else is optimized;
- **Final**, the tip of release/v2.0.0 (`03103b9`, PRs #14 to #16: backend, frontend and database passes), measured with the same commands as the baseline.

Rules and screens are the same in all three states apart from what the releases added; the rendered pages of the baseline and the final tree are
byte identical apart from the static-file hashes and the attributes listed in [Frontend pass](#frontend-pass) and [Database pass](#database-pass), and
the golden snapshots and every test are as they were. How the speed pass of the earlier releases measured and what it changed (static files, gzip,
one script bundle) is in [performance.md](performance.md).

Contents: [Executive summary](#executive-summary), [What changed and why](#what-changed-and-why), [What did not improve](#what-did-not-improve),
[Method](#method), [A. Data format](#a-data-format), [B. Runtime](#b-runtime), [C. Database](#c-database), [D. Build and test](#d-build-and-test),
[What is heaviest at the baseline](#what-is-heaviest-at-the-baseline), [Backend pass](#backend-pass), [Frontend pass](#frontend-pass),
[Database pass](#database-pass), [Reproducing the numbers](#reproducing-the-numbers).

## Executive summary

Times are medians (see [Method](#method)); "Change" is final against baseline, negative is better. The percentage for the v1.3.0 column is in the text of the
sections where it matters.

**Pages: the server answers 3 to 20 times faster.** The campaign list went from 91.5 ms to 4.7 ms of server time, the matches tab from 88.9 to 12.8 ms,
My Company from 40.7 to 10.1 ms, the match page from 23.4 to 8.7 ms; the statements of a page fell by up to 82 %, and a whole
`BenchmarkPages` run now runs 2,517 statements that take 528 ms in PostgreSQL where it ran 4,159 that took 1,934 ms. In the browser, with 300 ms of latency,
the match page settles in 386 ms instead of 1,002 ms, a tab click in 370 ms instead of 678 ms, a live match in 407 ms instead of 701 ms. A page that is one round
trip away from the browser (campaign list, overview, matches) was already at its latency floor and stays there.

### Data format and code

| Metric | v1.3.0 | Baseline | Final | Change |
|---|---|---|---|---|
| Battle Companies file, pretty (as shipped), bytes | 920,796 | 465,538 | 465,538 | 0 % |
| Battle Companies file, compact, bytes | 609,349 | 313,311 | 313,311 | 0 % |
| Battle Companies file, gzip, bytes | 131,573 | 76,977 | 76,977 | 0 % |
| Battle Companies schema file, bytes | 93,953 | 123,196 | 123,196 | 0 % |
| Go code that parses the whole Battle Companies file when a rule set loads | 14 parsers | 1 decoder (`lean`) | 1 decoder (`lean`) | - |
| Non-test Go lines in `internal/core` (with subpackages) | 26,459 | 33,389 | 33,750 | +1.1 % |
| Python lines in `tools/` | 8,535 | 2,548 | 2,548 | 0 % |
| Rule-number literals reported by `TestMagicNumbersReport` | 171 | 153 | 155 | +1.3 % |

### Loading a rule set (once per content hash, at start and on upload)

| Metric | v1.3.0 | Baseline | Final | Change |
|---|---|---|---|---|
| `core.LoadRuleSet`, three shipped files: time | 56.5 ms | 65.2 ms | 47.9 ms | -26.5 % |
| `core.LoadRuleSet`: memory allocated | 19.5 MB | 34.2 MB | 26.1 MB | -23.7 % |
| `core.LoadRuleSet`: allocations | 203,903 | 483,794 | 313,663 | -35.2 % |
| `lean.Decode` alone, Battle Companies file: time | - | 7.3 ms | 5.3 ms | -28.2 % |
| `lean.Decode`: memory / allocations | - | 4.6 MB / 124,956 | 3.8 MB / 87,560 | -17.1 % / -29.9 % |

### Server time per page (statements in brackets; HTTP probe, `pagetimes.mjs`)

| Page | Baseline | Final | Change |
|---|---|---|---|
| Campaign list `/campaigns` | 91.5 ms (11) | 4.7 ms (2) | -94.9 % |
| Matches tab | 88.9 ms (7) | 12.8 ms (6) | -85.6 % |
| My Company | 40.7 ms (16) | 10.1 ms (9) | -75.2 % |
| Armory | 53 ms (10) | 13.9 ms (7) | -73.8 % |
| Match page | 23.4 ms (37) | 8.7 ms (12) | -62.8 % |
| Tabletop Simulator dialog, aura rings on | 34.2 ms (7) | 4.4 ms (4) | -87.1 % |
| My Company over the wire (HTML gzip) | 30.7 KB | 30.8 KB | +0.3 % |

### In the browser (`make perf REPS=5`, 300 ms of emulated latency)

| Flow | Baseline | Final | Change |
|---|---|---|---|
| My Company: loaded / settled | 709 / 732 ms | 691 / 714 ms | -2.5 % settled |
| Armoury, full page: settled | 724 ms | 707 ms | -2.3 % |
| Click from the overview to My Company: settled (requests) | 678 ms (2) | 369 ms (1) | -45.6 % |
| Match page: settled (requests) | 1,002 ms (6) | 386 ms (2) | -61.5 % |
| Live match: settled (requests) | 701 ms (3) | 407 ms (2) | -41.9 % |
| Login flow: settled | 1,655 ms | 1,680 ms | +1.5 % |
| Armoury DOM elements | 5,115 | 1,760 | -65.6 % |

### Process, database seed and tests

| Metric | v1.3.0 | Baseline | Final | Change |
|---|---|---|---|---|
| Statements of one `BenchmarkPages` run (14 pages, 20 requests each) / their executor time in PostgreSQL | - | 4,159 / 1,934 ms | 2,517 / 528 ms | -39.5 % / -72.7 % |
| Server memory (RSS) after 200 page requests | - | 180 MB | 177 MB | -1.8 % |
| Server start to first answer (warm binary) | - | 207 ms | 211 ms | +1.9 % |
| Demo world seeded into an empty database (`server seed --world`): wall time / statements | - | 37.7 s / 98,954 | 23.0 s / 45,440 | -39.0 % / -54.1 % |
| `go test ./...` without a database | - | 30.5 s | 31 s | +1.6 % |
| `make test` (includes the PostgreSQL tests) | - | 139.9 s | 86 s | -38.5 % |
| `make test-integration` (`go test -p 1`, all tests with the database) | - | 191 s | 136 s | -28.8 % |
| `make e2e`, the four groups one after the other (local) | - | 1,163 s | 1,167 s | +0.3 % |
| CI end-to-end workflow wall time, PR #12 / PR #16 | - | 594 s | 560 s | -5.7 % |
| `go build` of the server, cold build cache | - | 7.1 s | 5.9 s | -16.9 % |
| Binary (release flags, linux/arm64) / Docker image | - | 21.7 MB / 26.1 MB | 21.7 MB / 26.1 MB | 0 % |

## What changed and why

**Data format (v1.3.0 to baseline).** The Battle Companies file moved to the lean, validated, fully parameterised format (schema version 2) read by
one decoder, `internal/core/lean`, instead of 14 type-specific parsers. The file is half the size (920,796 to 465,538 bytes, gzip 131,573 to
76,977), the Python in `tools/` is 70 % smaller (8,535 to 2,548 lines) and fewer rule numbers sit in Go (171 to 153 literals; the 5 engine constants the lint wanted
in data are gone). The costs: the schema file is 31 % larger (93,953 to 123,196 bytes), `internal/core` gained about 6,900 lines (26,459 to 33,389) for the
decoder and the parameterised rules, and loading a rule set allocates more than at v1.3.0 (see [What did not improve](#what-did-not-improve)).

**Backend pass (`internal/core`, PR #14).** The server time of the pages that build companies was Go CPU and garbage collection, not the database
(the campaign list allocated 101 MB per request). The changes: the equipment of a unit is remembered per rule set (it was half of every page
that builds a company), campaign lists and match pages rate companies from four figures instead of building whole company details, name keys of
ASCII names are made without decoding runes, the vanilla file is decoded once per load and duplicate checks only run when a name occurs twice, and
a few slices and tooltip texts are sized or remembered once. Result: the campaign list, matches tab, overview and Tabletop Simulator dialog
are 3 to 4 times faster and allocate 11 to 14 times less memory; `LoadRuleSet` is 26 % faster, uses 24 % less memory and 35 % fewer allocations.
No page output changed.

**Frontend pass (`internal/web`, PR #15).** What the browser waits for after the bytes arrive: the heading font is preloaded (layout shift on a
first visit 0.38 to 0.02), the To Wound chart is fetched when first wanted instead of on every match page view (live page 3 requests to 2,
settled 699 to 410 ms at 300 ms), a boosted click on the My Company or Armory tab is answered without the alias redirect (settled 676 to 366 ms),
the armoury's per-item unit lists are built when they are opened (5,115 to 1,760 elements, script time 44 to 34 ms) and the armoury is drawn in its
default filter (layout shift 0.20 to 0.02).

**Database pass (`internal/postgres` and the read path, PR #16).** Pages asked for the same rows several times and rated more companies than
they showed. A read-only GET now opens a read scope that shares the campaign, company, match and roster reads of the request (My Company 16 to 9
statements, match page 37 to 12), the roster is read as typed rows (the largest statement of the baseline went from 9.2 to 3.8 ms on the biggest
campaign), a page that rates one company of each of the player's campaigns reads those companies by id (campaign list 11 to 2 statements, 19 to 5 ms),
a company save writes only the rows that changed (seeding the demo world 98,954 to 45,440 statements, 37.7 to 23.0 s, which is also what shortens
`make test`) and the match page's event stream starts from the state the page was built from, which removes the refetch round trips (match page at
300 ms: 1,002 to 386 ms). The pool holds at least ten connections.

## What did not improve

Everything below was measured; none of it is a regression of the application code unless said.

- **`LoadRuleSet` memory and allocations against v1.3.0.** 47.9 ms is below v1.3.0 (56.5 ms, -15 %), but 26.1 MB and 313,663 allocations are above it (19.5 MB, 203,903:
  +34 % and +54 %). What remains is decoding the 1.2 MB vanilla file into one map of raw values per entry, the generic tree the lean decoder builds from the
  Battle Companies file with its path strings, and building the glossary twice. It runs once per content hash (start, upload), so a request never pays it.
- **`internal/core` grew.** 33,750 non-test lines against 33,389 at the baseline (+361, +1.1 %): the remembered values, the figures-only rating path and the read-scope
  sharing. The whole repository went from 52,016 to 52,674 lines. Two more rule-number literals (155 against 153): the bound of the remembered values (20,000
  entries) and the stack buffer of the name key (96 bytes); they are engine tuning, not rules.
- **The Armoury, full page, under latency.** 707 ms settled at 300 ms against 724 ms at the baseline (-2.3 %, inside the 10 % noise of a run): the full-page
  address still redirects (two document requests, chain 2), so the page is two round trips plus the work. Only the boosted tab click lost its redirect
  (695 to 370 ms). At 0 ms it is 109 ms against 210 ms. The same holds for My Company (714 against 732 ms).
- **Pages that are one round trip.** Campaign list, overview, matches tab, aftermath and home settle at 300 ms within 3 % of the baseline (350 to 380 ms): they are at the
  latency floor, the server time they saved (80 ms and more) shows at 0 ms (home 167 to 57 ms) and not at 300 ms, where the response body waits for the
  emulated latency. The two click flows that only open a window (recruit, level-up) are also latency-bound.
- **The login flow** settles at 1,680 ms against 1,655 ms (+1.5 %, noise): 11 requests, 8 of them static files fetched on a first visit.
- **Local browser-test time.** `make e2e` takes 1,167 s for the four groups against 1,163 s; the core group 221 s against 214 s (harness 217 against 211 s). The suite
  waits on browser steps and animations, not on the server. The CI workflow is 560 s against 594 s (-5.7 %), within the spread of GitHub runners (the three PRs in
  between ran 505 to 521 s with the same suite).
- **`go test ./...` without a database** is 31 s against 30.5 s: `internal/web` (30 s of many small tests) and `internal/core` (18.5 s) set the time and the passes
  did not touch them. `internal/postgres` in `make test` takes 23.8 s against 21.1 s (3 more database tests were added).
- **Start time** is 211 ms against 207 ms and the first start of a freshly built binary 604 to 629 ms against 199 to 634 ms (same within noise: the
  cold page cache decides); memory after start is 117.5 MB against 115 MB (+2.2 %).
- **Bytes.** The script bundle is 244.1 KB against 243.0 KB (+0.5 %), the armoury HTML 451.6 KB against 449.0 KB raw (+0.6 %; 27.8 against 27.6 KB gzip),
  My Company 30.8 against 30.7 KB gzip. The savings of the frontend pass are in requests and elements, not in bytes.

## Method

**Machine.** Apple M2 Pro (6 performance and 4 efficiency cores), 16 GB, macOS 14.4.1. PostgreSQL 17.11 in Docker Desktop 24.0.5 (5 CPUs,
7.7 GB) on the same host, the compose database of `compose.dev.yaml` with scratch databases beside the development one. Go 1.27.3,
Node 25.9.0, Python 3.11.4, Google Chrome 156 driven by puppeteer-core 24.4.0 (headless). The measurements ran one after the other with the
machine otherwise idle (CPU about 85 % idle between runs); times show proportions on this machine, not absolutes.

**World.** The demo world ([demo-world.md](demo-world.md)) seeded once into a template database and copied for each run (the mechanism of
`make e2e`); the development database is never touched. The pages are those of the demo accounts `eowyn@example.com` and
`admin@example.com` (campaign "Matches: every scenario and every state" for most pages).

**Final tree.** The tip of release/v2.0.0 (`03103b9`), measured on 2026-10-29 in a checkout whose application code is exactly the tip's (the branch
`perf/report` adds only this document), with the baseline's measuring tools (`BenchmarkLoadRuleSet`, `pagetimes.mjs`, `perf.mjs`) and the same commands.
Every "Final" number below was taken in that session, one command after the other with the machine otherwise idle; the sections of the passes keep the
numbers that pass measured at its own tip, so they can differ from the final ones by the noise of a run.

**Baseline tree.** A git worktree of the tag, with these additions that are not part of the tag and are also on the branch `perf/baseline`:
`BenchmarkLoadRuleSet`, `tests/e2e/pagetimes.mjs`, a time-to-first-byte and DOM-size column in `tests/e2e/perf.mjs`, and two repairs of the
measuring tools that had gone stale at the tag (`perf.mjs` waited for the address `/my-company` that now redirects to
`/my-company/{company}`; `BenchmarkPages` asked for the same redirecting address and for the armoury, which redirects as well, and now
follows the redirect once before it measures). The application code is exactly the tag's.

| Number | How it was taken | Runs and statistic |
|---|---|---|
| File sizes | the shipped files; compact = the same JSON re-encoded without whitespace; gzip level 9 | exact |
| Parser and line counts | `grep` for `json.Unmarshal` and `json.NewDecoder` in non-test Go; `cat` of the non-test `*.go` of `internal/core` (subpackages included, `testdata` excluded) and of `tools/*.py` | exact |
| Rule-number literals | `go test ./internal/core -run TestMagicNumbersReport -v`; at v1.3.0 the same test file copied into the v1.3.0 tree | exact |
| `LoadRuleSet`, `lean.Decode` | `go test ./internal/core -run xxx -bench LoadRuleSet -benchmem -count 10`; `Decode` times the decoder alone on the prepared vanilla book and glossary | 10 runs, median |
| `make perf` | `node perf.mjs --reps 5`: headless Chrome, Chrome's network emulation for 0, 100 and 300 ms of latency; user flows on the demo world | 5 runs per flow and latency; the run with the median settle time stands for the flow (all its figures come from that run) |
| Server time, database time, statements per page | `node pagetimes.mjs`: plain HTTP requests with `PERF_TRACE=true` (`Server-Timing`: `app` is the time to the first byte, `db` the time in statements, summed over the request); 5 warm-up requests then 20 measured per page | 3 server starts of 20; median of the 3 medians (range of the 3 in brackets) |
| HTML, CSS and JS bytes | the same requests: HTML raw (no `Accept-Encoding`) and gzip (as sent); the stylesheets and scripts the page references, served raw and gzip | exact |
| Server start | `pagetimes.mjs`: time from `spawn` to the first 200 of `/healthz` (polling every 5 ms) and of `/login` | 5 warm starts of 3 runs (15), median; the first start of a freshly built binary is reported apart |
| Server memory | `ps -o rss` of the server process: just after start, after one visit to each measured page, after 200 more page requests | 3 runs, median |
| Statements per page (tests) | `BenchmarkPages` (`stmts/op`, the pgx tracer counts) and `TestQueryCountsOnAPreloadedWorld` | exact |
| Server time per page (benchmark) | `go test ./cmd/server -run xxx -bench Pages -benchtime 20x -benchmem -count 5` on a scratch copy of the world | 5 runs, median |
| Slowest statements | the PostgreSQL module `auto_explain` (`log_analyze`, `log_buffers`, `log_min_duration=0`, set on the scratch database only) during `BenchmarkPages -benchtime 20x`; statements ranked by mean execution time (at least 5 calls); the plan, planning and execution time of the slowest call then re-run with `EXPLAIN (ANALYZE, BUFFERS)` and its parameters | one workload of 20 requests per page |
| Build time | `go build -o server ./cmd/server` with an empty `GOCACHE` (modules cached) | 3 builds, median; warm numbers 3 builds |
| Test time | `make test`, `go test -count=1 ./...` (no database), `go test -p 1 -count=1 ./...` with `TEST_DATABASE_URL` (what `make test-integration` runs); per package from the `ok` lines | `make test` and the no-database run 3 times (median); cold run and integration run once |
| End-to-end time | `make e2e GROUP=<name>`, one run per group (seeded template cached) | 1 run |
| CI time | `gh run view` of the workflow runs of PR #12 (baseline) and PR #16 (final), and of PRs #13 to #15 for the spread (read only) | the one observed run of each |
| Image | `docker build --no-cache` of the repository's `Dockerfile` (base images already pulled) | 1 run |

Notes on the medians: the settled time under emulated latency varies by about 10 % between runs (the five values of each flow are in
`tests/e2e/out/perf-<label>.json`); the two Tabletop Simulator rows (rings on and off) differ by under 1 ms in the HTTP probe and by 1 ms in the benchmark; read them as equal within the noise.
Statements are counted by the same tool for both states, so their counts are exact; times of a few milliseconds vary by about 5 to 10 % between sessions.

`make test` runs the PostgreSQL tests too when the compose database is up, because the Makefile exports `TEST_DATABASE_URL` to every command;
`go test ./...` run by hand without that variable skips them. Both are reported.

## A. Data format

The Battle Companies file (baseline and final) is the schema-version-2 ("lean") format read by one decoder, `internal/core/lean`. The
vanilla file, the glossary and the concept graph did not change in format. The optimization passes did not touch `definitions/` or `tools/`.

### Files

| File | Pretty (as shipped) | Compact | Pretty gzip | Compact gzip |
|---|---|---|---|---|
| `battle_companies.json`, v1.3.0 | 920,796 | 609,349 | 131,573 | 119,372 |
| `battle_companies.json`, baseline | 465,538 | 313,311 | 76,977 | 70,590 |
| `battle_companies.json`, final | 465,538 | 313,311 | 76,977 | 70,590 |
| `battle_companies.schema.json`, v1.3.0 | 93,953 | 46,414 | 13,041 | 10,040 |
| `battle_companies.schema.json`, baseline | 123,196 | 81,884 | 20,065 | 18,184 |
| `battle_companies.schema.json`, final | 123,196 | 81,884 | 20,065 | 18,184 |
| `data2024.json` (vanilla), all three states | 1,178,030 | 882,090 | 157,596 | 148,097 |
| `mesbg_glossary.json`, all three states | 248,443 | 199,490 | 50,913 | 49,024 |
| `concept_graph.json`, v1.3.0 / baseline and final | 247,512 / 247,686 | 174,559 / 174,707 | 39,435 / 39,468 | 38,449 / 38,486 |

Bytes. The three files a rule set is built from (vanilla, Battle Companies, glossary) are 2,347,269 bytes at v1.3.0 and 1,892,011 at the baseline and the final tree (`definitions/` is the same in both).

### Code

| Measure | v1.3.0 | Baseline | Final |
|---|---|---|---|
| Functions that parse the whole Battle Companies file when a rule set loads (`json.Unmarshal` into a document type) | 14 (`ParseBattleCompanies`, `parsePathRules`, `ParseVanillaAmbiguities`, `ParseProgression`, `ParseCampaignOptions`, `ParseScenarios`, `ParseATO`, `ParseCreaturesAndWanderers`, `ParseAftermath`, `ParseInjuries`, `parseProfiles`, `ParseAuras`, `ParseTTSModels`, `GlossaryExtras`) | 1 (`lean.parseJSON`, behind `lean.Decode`) | 1 (`lean.parseJSON`, behind `lean.Decode`) |
| Passes that only look at the file (shape limits, header, `schemaVersion`) | 2 (`checkRulesetShape`, `RulesetMeta`) | 3 (`checkRulesetShape`, `RulesetMeta`, `schemaVersionOf`) | 3 (the same) |
| Parsers of the file in the demo-world seeder (`cmd/server`) | 3 | 1 | 1 |
| Non-test Go lines, `internal/core` (79 / 118 files) | 26,459 | 33,389 (package `core` 27,991, `lean` 4,536, `ttshelper` 862) | 33,750 (118 files; `core` 28,319, `lean` 4,569, `ttshelper` 862) |
| Non-test Go lines, whole repository | 43,751 | 52,016 | 52,674 |
| Python lines, `tools/` (all) | 8,535 | 2,548 | 2,548 |
| Python lines, `tools/`, without tests | 7,946 | 2,054 | 2,054 |
| Go lines, `tools/` | 0 | 78 | 78 |
| Rule-number literals (`TestMagicNumbersReport`: integer literals other than 0, 1, 2 outside the allow-list in non-test `internal/core/*.go`) | 171 (and 5 engine constants the lint wants in data) | 153 (0 engine constants) | 155 (0 engine constants; the two new ones are the bound of the remembered values, 20,000, and the 96-byte stack buffer of the name key) |
| Rule literals in templates (`TestTemplateRuleLiteralsReport`: "up to N", "ND6") | - | 8 | 8 |

The v1.3.0 literal count is the baseline's test file run on the v1.3.0 sources (the test did not exist then); the allow-list is the
baseline's.

### Loading the rule set

`go test ./internal/core -run xxx -bench LoadRuleSet -benchmem -count 10` (`BenchmarkLoadRuleSet` in `internal/core/bench_loadruleset_test.go`),
the three shipped files, median of 10:

| Benchmark | Wall time | Bytes allocated | Allocations |
|---|---|---|---|
| `core.LoadRuleSet`, v1.3.0 (same call, old parsers) | 56.46 ms | 19,472,288 | 203,903 |
| `core.LoadRuleSet`, baseline | 65.22 ms (64.77 to 65.54) | 34,211,698 | 483,794 |
| `core.LoadRuleSet`, final | 47.94 ms (47.66 to 49.15) | 26,108,000 (26.1 MB) | 313,663 |
| `lean.Decode` alone, baseline (Battle Companies file, prepared vanilla book and glossary) | 7.33 ms (7.32 to 7.37) | 4,565,394 | 124,956 |
| `lean.Decode` alone, final | 5.26 ms (5.25 to 5.31) | 3,785,723 (3.8 MB) | 87,560 |

Final against baseline: `LoadRuleSet` -26.5 % time, -23.7 % memory, -35.2 % allocations; `Decode` -28.2 %, -17.1 %, -29.9 %. Against v1.3.0: -15.1 % time, but +34.1 % memory and +53.8 % allocations.

A rule set is built once per content hash and kept (see [architecture.md](architecture.md)), so this cost is paid at start and when a
ruleset is uploaded, not per request.

## B. Runtime

### B1. Browser flows (`make perf`)

`node tests/e2e/perf.mjs --label baseline --reps 5` (the target `make perf REPS=5`; the final column is `make perf LABEL=final REPS=5`), headless Chrome, 5 runs per flow and latency,
the run with the median settle time kept. Reads only (no flow saves). "Requests" are those that went over the network; "+ cached" were
answered from the browser cache. Chain is the longest series of requests where each starts after the previous one ended. The flow "my
company" opens `/my-company`, which redirects to the company: two document requests. The login flow is the GET, POST, redirect and home page.

TTFB is the browser's `responseStart`. Under Chrome's network emulation the added latency delays the response body, not that timestamp, so
read the time to load and the settled time for the visitor's wait. Server ms and statements are the sums over the flow's document and htmx
requests.

Per flow:

| Flow | Baseline: requests (doc / xhr / sse / static) + cached | Chain | KB on the wire | KB HTML | DOM elements | Server ms | Statements | Final: requests | Chain | KB on the wire | KB HTML | DOM elements | Server ms | Statements |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| login | 11 (2 / 1 / 0 / 8) + 9 | 6 | 164.3 | 11.4 | 449 | 110 | 15 | 11 (2 / 1 / 0 / 8) + 9 | 5 | 164.9 | 11.6 | 450 | 42.8 | 6 |
| home | 1 (1 / 0 / 0 / 0) + 8 | 1 | 7.5 | 7.5 | 449 | 125.4 | 11 | 1 (1 / 0 / 0 / 0) + 8 | 1 | 7.6 | 7.6 | 450 | 9.9 | 2 |
| home, cold cache | 9 (1 / 0 / 0 / 8) + 0 | 5 | 156.8 | 7.5 | 449 | 115.9 | 11 | 9 (1 / 0 / 0 / 8) + 0 | 4 | 157.3 | 7.6 | 450 | 11.8 | 2 |
| campaign overview | 1 (1 / 0 / 0 / 0) + 8 | 1 | 7.5 | 7.5 | 434 | 81.1 | 5 | 1 (1 / 0 / 0 / 0) + 8 | 1 | 7.6 | 7.6 | 435 | 25.7 | 5 |
| my company | 2 (2 / 0 / 0 / 0) + 9 | 2 | 31.6 | 31.6 | 1943 | 58.9 | 16 | 2 (2 / 0 / 0 / 0) + 9 | 2 | 31.6 | 31.6 | 1944 | 17.8 | 9 |
| recruit window (click) | 1 (0 / 1 / 0 / 0) + 0 | 1 | 2.0 | 2.0 | 1999 | 33.8 | 12 | 1 (0 / 1 / 0 / 0) + 0 | 1 | 2.0 | 2.0 | 2000 | 23.9 | 12 |
| level-up window (click) | 1 (0 / 1 / 0 / 0) + 0 | 1 | 4.0 | 4.0 | 3094 | 18.9 | 8 | 1 (0 / 1 / 0 / 0) + 0 | 1 | 4.0 | 4.0 | 3095 | 17.5 | 8 |
| click: overview to my company (htmx) | 2 (0 / 2 / 0 / 0) + 2 | 2 | 27.2 | 27.2 | 1943 | 67.8 | 15 | 1 (0 / 1 / 0 / 0) + 2 | 1 | 27.3 | 27.3 | 1944 | 20.4 | 8 |
| click: overview to armoury (htmx) | - | - | - | - | - | - | - | 1 (0 / 1 / 0 / 0) + 2 | 1 | 24.1 | 24.1 | 1760 | 27.4 | 6 |
| click: home to campaign overview (htmx) | 1 (0 / 1 / 0 / 0) + 1 | 1 | 3.0 | 3.0 | 434 | 90.5 | 4 | 1 (0 / 1 / 0 / 0) + 1 | 1 | 3.0 | 3.0 | 435 | 27.6 | 4 |
| armoury | 2 (2 / 0 / 0 / 0) + 9 | 2 | 28.4 | 28.4 | 5115 | 64.4 | 10 | 2 (2 / 0 / 0 / 0) + 9 | 2 | 28.6 | 28.6 | 1760 | 27.8 | 7 |
| matches list | 1 (1 / 0 / 0 / 0) + 9 | 1 | 7.6 | 7.6 | 564 | 119.6 | 7 | 1 (1 / 0 / 0 / 0) + 9 | 1 | 7.6 | 7.6 | 565 | 32.8 | 6 |
| match page | 6 (1 / 4 / 1 / 0) + 8 | 3 | 20.5 | 20.5 | 1053 | 94 | 68 | 2 (1 / 0 / 1 / 0) + 8 | 1 | 11.9 | 11.9 | 844 | 29 | 12 |
| live match (+3 s observed) | 3 (1 / 1 / 1 / 0) + 9 | 2 | 34.5 | 34.5 | 2343 | 45.2 | 27 | 2 (1 / 0 / 1 / 0) + 9 | 1 | 29.6 | 29.6 | 2134 | 35.8 | 16 |
| aftermath | 1 (1 / 0 / 0 / 0) + 9 | 1 | 8.1 | 8.1 | 586 | 33.7 | 14 | 1 (1 / 0 / 0 / 0) + 9 | 1 | 8.2 | 8.2 | 587 | 22.4 | 13 |

The flow "click: overview to armoury" exists only in the final column (the baseline tool had no such flow; its click went through the same redirect as the My Company tab, 2 requests). A live match opens no polling request (0 extra requests in 3 s after it settled). A page that sits on its event stream shows the stream as
one request.

Per flow and latency (milliseconds; first paint is the first contentful paint):

| Flow | Latency | TTFB | First paint | Load | Settled | Final: TTFB | Final: first paint | Final: load | Final: settled | Settled, change |
|---|---|---|---|---|---|---|---|---|---|---|
| login | 0 ms | 84 | 368 | 111 | 266 | 16 | 308 | 42 | 194 | -27 % |
| login | 100 ms | 110 | 320 | 140 | 640 | 16 | 280 | 141 | 642 | +0 % |
| login | 300 ms | 115 | 520 | 347 | 1655 | 19 | 520 | 360 | 1680 | +2 % |
| home | 0 ms | 127 | 292 | 146 | 167 | 11 | 208 | 30 | 57 | -66 % |
| home | 100 ms | 107 | 280 | 130 | 153 | 13 | 280 | 136 | 161 | +5 % |
| home | 300 ms | 107 | 504 | 335 | 360 | 18 | 508 | 349 | 370 | +3 % |
| home, cold cache | 0 ms | 117 | 212 | 139 | 163 | 13 | 108 | 35 | 55 | -66 % |
| home, cold cache | 100 ms | 124 | 324 | 361 | 499 | 14 | 320 | 351 | 481 | -4 % |
| home, cold cache | 300 ms | 106 | 704 | 945 | 1263 | 16 | 716 | 962 | 1283 | +2 % |
| campaign overview | 0 ms | 84 | 244 | 105 | 132 | 28 | 196 | 52 | 76 | -42 % |
| campaign overview | 100 ms | 87 | 264 | 129 | 154 | 24 | 272 | 131 | 157 | +2 % |
| campaign overview | 300 ms | 78 | 496 | 341 | 365 | 21 | 488 | 352 | 376 | +3 % |
| my company | 0 ms | 96 | 316 | 134 | 159 | 28 | 248 | 65 | 88 | -45 % |
| my company | 100 ms | 186 | 452 | 270 | 292 | 135 | 448 | 263 | 288 | -1 % |
| my company | 300 ms | 411 | 868 | 709 | 732 | 338 | 872 | 691 | 714 | -2 % |
| recruit window (click) | 0 ms | 46 | 288 | 86 | 187 | 22 | 328 | 56 | 197 | +5 % |
| recruit window (click) | 100 ms | 38 | 304 | 79 | 217 | 25 | 268 | 64 | 197 | -9 % |
| recruit window (click) | 300 ms | 61 | 280 | 107 | 409 | 35 | 288 | 82 | 413 | +1 % |
| level-up window (click) | 0 ms | 61 | 272 | 110 | 199 | 29 | 244 | 79 | 192 | -4 % |
| level-up window (click) | 100 ms | 52 | 300 | 108 | 197 | 30 | 248 | 81 | 193 | -2 % |
| level-up window (click) | 300 ms | 71 | 252 | 124 | 408 | 44 | 264 | 101 | 414 | +1 % |
| click: overview to my company (htmx) | 0 ms | 83 | 240 | 102 | 153 | 20 | 64 | 38 | 66 | -57 % |
| click: overview to my company (htmx) | 100 ms | 88 | 240 | 132 | 266 | 27 | 188 | 75 | 157 | -41 % |
| click: overview to my company (htmx) | 300 ms | 87 | 240 | 129 | 678 | 25 | 184 | 78 | 369 | -46 % |
| click: overview to armoury (htmx) | 0 ms | - | - | - | - | 33 | 196 | 57 | 70 | - |
| click: overview to armoury (htmx) | 100 ms | - | - | - | - | 25 | 200 | 48 | 158 | - |
| click: overview to armoury (htmx) | 300 ms | - | - | - | - | 25 | 204 | 50 | 370 | - |
| click: home to campaign overview (htmx) | 0 ms | 101 | 272 | 120 | 108 | 33 | 216 | 60 | 53 | -51 % |
| click: home to campaign overview (htmx) | 100 ms | 131 | 308 | 151 | 133 | 31 | 188 | 83 | 137 | +3 % |
| click: home to campaign overview (htmx) | 300 ms | 130 | 288 | 150 | 349 | 36 | 200 | 64 | 346 | -1 % |
| armoury | 0 ms | 106 | 308 | 187 | 210 | 39 | 216 | 85 | 109 | -48 % |
| armoury | 100 ms | 181 | 412 | 291 | 314 | 142 | 420 | 274 | 298 | -5 % |
| armoury | 300 ms | 405 | 816 | 699 | 724 | 355 | 836 | 683 | 707 | -2 % |
| matches list | 0 ms | 121 | 280 | 143 | 167 | 35 | 196 | 60 | 88 | -47 % |
| matches list | 100 ms | 118 | 268 | 139 | 163 | 34 | 272 | 141 | 167 | +2 % |
| matches list | 300 ms | 109 | 492 | 348 | 372 | 23 | 496 | 347 | 374 | +1 % |
| match page | 0 ms | 46 | 208 | 81 | 132 | 30 | 196 | 60 | 83 | -37 % |
| match page | 100 ms | 51 | 276 | 147 | 383 | 30 | 292 | 144 | 165 | -57 % |
| match page | 300 ms | 51 | 540 | 361 | 1002 | 21 | 540 | 367 | 386 | -61 % |
| live match (+3 s observed) | 0 ms | 44 | 224 | 95 | 134 | 39 | 216 | 90 | 121 | -10 % |
| live match (+3 s observed) | 100 ms | 54 | 288 | 164 | 277 | 36 | 288 | 168 | 197 | -29 % |
| live match (+3 s observed) | 300 ms | 62 | 496 | 373 | 701 | 28 | 492 | 375 | 407 | -42 % |
| aftermath | 0 ms | 36 | 200 | 60 | 87 | 24 | 180 | 51 | 72 | -17 % |
| aftermath | 100 ms | 33 | 264 | 139 | 166 | 28 | 260 | 137 | 160 | -4 % |
| aftermath | 300 ms | 29 | 492 | 352 | 374 | 29 | 496 | 354 | 380 | +2 % |

### B2. Server time and bytes per page (`pagetimes.mjs`)

`node tests/e2e/pagetimes.mjs`: plain HTTP, no browser or network emulation, so these are the server's own cost and the exact bytes. Server
ms is `Server-Timing: app` (time to the first byte), DB ms the time spent in statements (summed, so it can exceed the server time when
statements run in parallel, as on the match page where the four cards are built concurrently). Statements are those of the request itself,
including the session lookup. Every row is for one warm request after the rule sets are loaded. The Tabletop Simulator dialog is the htmx
fragment `…/companies/{company}/tts` (aura rings off is `?auras=0`); the new-company page is the one of campaign 21, the first campaign of the demo world that opens it for `eowyn@example.com` or `admin@example.com`. The CSS and JS columns (the baseline's; the final stylesheet is 103.7 / 15.1 KB and the script bundle 244.1 / 79.4 KB, raw / gzip, the same on every page) are the page's referenced files (one stylesheet, one script bundle, the same on every page; served
gzip ahead of time and `immutable` for a year, so a warm visit does not fetch them).

| Page | Baseline: server ms (3 runs, min to max) | DB ms | Statements | HTML KB raw | HTML KB gzip | CSS KB raw / gzip | JS KB raw / gzip | Final: server ms (min to max) | DB ms | Statements | HTML KB raw | HTML KB gzip |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Campaign list | 91.5 (83.1 to 93.3) | 35.3 | 11 | 41.0 | 6.8 | 103.6 / 15.1 | 243.0 / 79.0 | 4.7 (4.2 to 4.8) | 2.8 | 2 | 41.1 | 6.9 |
| Campaign overview | 64.4 (61.7 to 65.7) | 26 | 5 | 37.1 | 6.8 | 103.6 / 15.1 | 243.0 / 79.0 | 8.8 (8.1 to 9.1) | 5.9 | 5 | 37.3 | 6.8 |
| My Company | 40.7 (40.3 to 43.5) | 14.8 | 16 | 251.6 | 30.7 | 103.6 / 15.1 | 243.0 / 79.0 | 10.1 (9.3 to 22.5) | 5.2 | 9 | 251.8 | 30.8 |
| Matches tab | 88.9 (83.8 to 95) | 34.6 | 7 | 49.2 | 6.8 | 103.6 / 15.1 | 243.0 / 79.0 | 12.8 (12.3 to 14.2) | 7.6 | 6 | 49.3 | 6.8 |
| Armory | 53 (48.9 to 53) | 20.6 | 10 | 449.0 | 27.6 | 103.6 / 15.1 | 243.0 / 79.0 | 13.9 (13.3 to 14.6) | 4.3 | 7 | 451.6 | 27.8 |
| New company | 14.2 (11 to 14.8) | 3.8 | 3 | 61.3 | 7.3 | 103.6 / 15.1 | 243.0 / 79.0 | 4.3 (4 to 4.5) | 2.3 | 3 | 61.4 | 7.4 |
| Match page | 23.4 (23.3 to 23.7) | 30.7 | 37 | 77.3 | 11.1 | 103.6 / 15.1 | 243.0 / 79.0 | 8.7 (8.6 to 8.9) | 7.1 | 12 | 77.4 | 11.1 |
| Live match (company page) | 24.9 (24.8 to 25.8) | 16.5 | 26 | 248.6 | 28.7 | 103.6 / 15.1 | 243.0 / 79.0 | 13 (12.4 to 14.3) | 8 | 16 | 248.8 | 28.8 |
| Aftermath | 13.5 (13.3 to 13.6) | 8 | 14 | 54.7 | 7.3 | 103.6 / 15.1 | 243.0 / 79.0 | 9.1 (8.4 to 9.9) | 7 | 13 | 54.8 | 7.4 |
| Tabletop Simulator dialog, aura rings on | 34.2 (34.1 to 35.4) | 11.2 | 7 | 45.2 | 12.1 | - | - | 4.4 (4.3 to 4.9) | 2.6 | 4 | 45.2 | 12.1 |
| Tabletop Simulator dialog, aura rings off | 39.4 (37 to 40.3) | 15.3 | 7 | 43.2 | 11.6 | - | - | 3.7 (3.6 to 4.4) | 2.5 | 4 | 43.2 | 11.6 |

### B3. Server process

| Measure | Baseline | Final |
|---|---|---|
| Start to first answer of `/healthz` and `/login` (warm binary, median of 15 starts) | 207 ms and 208 ms | 211 ms and 211 ms |
| First start of a freshly built binary (cold page cache), 3 observations | 199, 634, 621 ms | 628, 604, 629 ms |
| Memory (RSS) just after start | 115 MB | 117.5 MB |
| Memory after one visit to each measured page (rule sets, templates, caches warm) | 177 MB | 175.6 MB |
| Memory after 200 more page requests | 180 MB | 176.8 MB |
| Memory after the measurement (about 450 requests in all) | 181 MB | 177.2 MB |

## C. Database

### Statements per page

`BenchmarkPages` (`cmd/server/pagebench_test.go`) on a scratch copy of the demo world: time, memory and statements of one whole request
(handler, services, statements, template, gzip), without browser or network. `go test ./cmd/server -run xxx -bench Pages -benchtime 20x
-benchmem -count 5`, median of 5 (final: the same command at the tip). The statements here come from the pgx tracer and include the session lookup but not the
rule-set loads (warm).

| Page | Baseline: ms per request | MB allocated | Allocations (thousands) | Statements | KB | Final: ms | Change | MB | Allocations (thousands) | Statements | KB |
|---|---|---|---|---|---|---|---|---|---|---|---|
| home (`/`, campaign list) | 90.1 | 101.5 | 216.6 | 11 | 40.9 | 4.5 | -95 % | 1.7 | 21.9 | 2 | 41.0 |
| `/nav/campaigns` (sidebar fallback) | 1.0 | 0.2 | 3.8 | 1 | 19.3 | 0.7 | -30 % | 0.2 | 3.8 | 1 | 19.3 |
| campaign overview | 63.3 | 66.0 | 142.8 | 5 | 37.0 | 8.4 | -87 % | 2.8 | 33.9 | 5 | 37.2 |
| my company | 43.4 | 40.4 | 121.6 | 16 | 251.5 | 9.7 | -78 % | 3.3 | 50.8 | 9 | 251.7 |
| armoury | 53.8 | 46.5 | 194.1 | 10 | 448.9 | 16.8 | -69 % | 7.1 | 118.2 | 7 | 451.5 |
| matches list | 94.8 | 98.8 | 213.0 | 7 | 49.1 | 14.9 | -84 % | 4.6 | 57.1 | 6 | 49.2 |
| match page | 25.0 | 14.0 | 40.8 | 37 | 77.2 | 7.4 | -70 % | 1.9 | 24.5 | 12 | 77.4 |
| Against the Odds card | 17.8 | 6.7 | 14.6 | 14 | 0.5 | 4.4 | -75 % | 0.5 | 5.4 | 8 | 0.5 |
| live log card | 12.4 | 4.9 | 11.0 | 12 | 1.1 | 5.5 | -56 % | 0.5 | 5.3 | 10 | 1.1 |
| live company page | 26.6 | 11.3 | 61.6 | 26 | 248.6 | 13.1 | -51 % | 3.5 | 51.7 | 16 | 248.7 |
| aftermath | 14.1 | 9.2 | 30.3 | 14 | 54.6 | 8.3 | -41 % | 1.6 | 20.3 | 13 | 54.7 |
| new company | 5.3 | 3.8 | 35.9 | 2 | 80.5 | 3.2 | -40 % | 1.7 | 28.1 | 2 | 80.7 |
| Tabletop Simulator dialog, aura rings on | 41.1 | 40.5 | 88.1 | 8 | 45.2 | 5.2 | -87 % | 1.5 | 14.5 | 5 | 45.2 |
| Tabletop Simulator dialog, aura rings off | 37.6 | 40.2 | 83.4 | 8 | 43.2 | 4.1 | -89 % | 1.2 | 10.2 | 5 | 43.2 |

(The browser flow and the HTTP probe count a few more or fewer statements for the same page: the browser's match page adds its htmx cards,
and each tool picks its own user and campaign for the new-company page.)

The service-level counts of `TestQueryCountsOnAPreloadedWorld` (`cmd/server/querycount_test.go`; "queries for N rows" proves the count does
not grow with the rows; the budget is the test's limit; the test calls the services without a read scope and with the player of the demo world who has no company, so the page-level gains of the database pass do not show in it, the page table above has them):

| Call | Statements, small / large input | Budget | Final |
|---|---|---|---|
| `Campaigns.Get` (campaign page) | 1 for 21 rows / 1 for 21 rows | 3 | unchanged |
| `Campaigns.Companies` (companies, rankings, hand-over) | 1 for 0 rows / 3 for 37 rows | 6 | unchanged |
| `Matches.Matches` (matches page) | 5 for 0 rows / 2 for 25 rows | 8 | unchanged |
| `Matches.Match` (match page) | 3 for 1 row / 3 for 1 row | 6 | unchanged |
| `Campaigns.CompanyDetail` (company page) | 4 for 4 rows / 4 for 24 rows | 7 | unchanged |
| `Campaigns.List` (home page) | 1 for 0 rows / 1 for 0 rows | 3 | unchanged |

### The ten slowest statements

Demo world (the data volume is small: 1,324 units, 893 equipment rows, 130 companies, 44 matches, 21 campaigns), `auto_explain` with analyze
and buffers during `BenchmarkPages -benchtime 20x` (about 4,200 statements). Ranked by mean execution time, statements called at least 5
times; the load of a stored rule set by hash (6 calls, 0.05 ms of execution) is left out. "Mean", "max" and "calls" are over that run's executions as
`auto_explain` timed them (executor only, planning excluded); "Plan" and "Exec" are the planning and execution time of `EXPLAIN (ANALYZE,
BUFFERS)` for the slowest call with its parameters; "Buffers" are shared-buffer hits of that execution (all from cache). The "pages" are the
pages of `BenchmarkPages` that run the statement (`PERF_SQL=1` prints the statements per page).

| # | Statement (store method) | Calls | Mean ms | Max ms | Plan ms | Exec ms | Buffers | Plan shape |
|---|---|---|---|---|---|---|---|---|
| 1 | company roster of a campaign: every company with its units and equipment as JSON (`companyStore.ReadRoster`) | 615 | 2.78 | 9.62 | 3.17 | 5.04 | 1,478 | index scans; units and equipment sub-selects run once per company (28 companies, 200 loops) |
| 2 | all matches of a campaign with sides, companies, proposal and objectives as JSON (`matchStore.Matches`) | 48 | 2.29 | 4.96 | 3.21 | 2.05 | 556 | sequential scans of `match_sides`, `match_side_companies`, `companies` inside 25 sub-select loops |
| 3 | company figures of a campaign: unit count, wins, draws, losses (`campaignStore.Companies`) | 25 | 1.62 | 3.51 | 2.63 | 1.72 | 237 | sequential scans of `companies`, `matches`, `match_side_companies`; correlated sub-selects per company |
| 4 | the user's campaigns with their own companies as JSON (`campaignStore.List`) | 23 | 0.78 | 1.75 | 3.15 | 1.07 | 157 | sequential scans of five tables (all under 200 rows) |
| 5 | one company in full (`companyStore.Read`) | 844 | 0.29 | 2.53 | 3.54 | 0.71 | 71 | index scans and bitmap scans, 119-line plan |
| 6 | campaign detail (`campaignStore.Get`) | 498 | 0.29 | 3.11 | 4.49 | 0.81 | 48 | sequential scans of six small tables |
| 7 | one company with its figures (`campaignStore.Company`) | 186 | 0.12 | 0.85 | 2.61 | 0.31 | 13 | like 3, for one company |
| 8 | one match (`matchStore.Match`, `load`) | 542 | 0.11 | 0.75 | 2.80 | 0.49 | 37 | like 2, for one match |
| 9 | live match counter and units (`liveStore.Read`) | 143 | 0.10 | 0.22 | 1.77 | 0.41 | 9 | sequential scans of `match_live_log`, `match_live` |
| 10 | sidebar list: id, name, status of the user's campaigns (`campaignStore.Nav`) | 324 | 0.06 | 0.33 | 1.68 | 0.24 | 15 | hash join of two sequential scans |

Plan times are larger than execution times
for most: PostgreSQL plans these multi-sub-select statements in 1.7 to 4.5 ms. pgx prepares a statement once per connection and the server
may switch to a generic plan after five executions, so in the running server the planning is not paid on every call; the benchmark and
`EXPLAIN` figures show the first-call cost.

The same workload at the final tree (`BenchmarkPages -benchtime 20x` over the 14 pages, 2,517 statements, `auto_explain` as in [Reproducing the numbers](#reproducing-the-numbers) on a fresh scratch copy; the parameters for `EXPLAIN` come from the statement log of a separate run). The mean and max are the medians of three runs, "Plan" and "Exec" the medians of three `EXPLAIN (ANALYZE, BUFFERS)` of the slowest call's parameters, the baseline calls and mean are from the table above:

| # | Statement (store method) | Calls (baseline) | Mean ms (baseline) | Max ms | Plan ms | Exec ms | Buffers | Plan shape |
|---|---|---|---|---|---|---|---|---|
| 1 | company roster of a campaign: companies with their units and the equipment lists as typed rows (`companyStore.ReadRoster`) | 48 (615) | 1.80 (2.78) | 3.10 | 2.41 | 2.44 | 606 | hash joins of sequential scans of `company_units`, `companies`, `users`; one `unit_equipment` index probe per unit (200 loops) |
| 2 | all matches of a campaign with sides, companies, proposal and objectives as JSON (`matchStore.List`) | 48 (48) | 1.47 (2.29) | 2.64 | 3.17 | 2.33 | 556 | sequential scans of `match_sides`, `match_side_companies`, `companies` inside sub-select loops |
| 3 | company figures of a campaign: unit count, wins, draws, losses (`campaignStore.Companies`) | 25 (25) | 1.29 (1.62) | 1.56 | 2.48 | 1.89 | 237 | sequential scans of `companies`, `matches`, `match_side_companies`; correlated sub-selects per company |
| 4 | the user's campaigns with their own companies as JSON (`campaignStore.List`) | 23 (23) | 0.67 (0.78) | 0.73 | 3.11 | 1.42 | 157 | sequential scans of five tables (all under 200 rows) |
| 5 | one company in full (`companyStore.Read`) | 304 (844) | 0.29 (0.29) | 1.83 | 3.64 | 1.04 | 71 | index scans and bitmap scans |
| 6 | campaign detail (`campaignStore.Get`) | 324 (498) | 0.31 (0.29) | 3.11 | 4.58 | 1.37 | 48 | sequential scans of six small tables |
| 7 | rosters of some companies by id (`companyStore.ReadRosterOf`, used by the campaign list and the pages that rate a few companies) | 290 (-) | 0.16 (-) | 0.99 | 2.03 | 2.28 | 364 | the roster statement with `c.id = ANY($1)` |
| 8 | one match (`matchStore.Match`, `load`) | 118 (542) | 0.11 (0.11) | 0.50 | 2.90 | 0.76 | 36 | like 2, for one match |
| 9 | live match counter and units (`liveStore.Read`) | 95 (143) | 0.10 (0.10) | 0.47 | 1.98 | 0.54 | 9 | sequential scans of `match_live_log`, `match_live` |
| 10 | one company with its figures (`campaignStore.Company`) | 138 (186) | 0.10 (0.12) | 0.36 | 2.77 | 0.52 | 13 | like 3, for one company |

The sidebar list (`campaignStore.Nav`) runs 324 times at 0.04 ms (baseline 0.06). All statements of the run: 2,517 (baseline 4,159) with 492, 528 and 550 ms of executor time in three runs (baseline 1,934 ms). The statements that remain are the same shapes with the same plans; what changed is how often a page runs them (the read scope: one company read 844 to 304 times, one match 542 to 118) and the roster, which is read less often (615 whole-campaign reads to 48, the rest by id, 290 reads at 0.16 ms) and costs 35 % less when it is read in full. The multi-sub-select statements were not rewritten: pgx prepares them once per connection, and a server that has run for a while does not plan them again ([performance.md](performance.md), "Database pass at v2.0.0"). The planning times in the tables are first-call costs. (The roster's mean is 1.8 ms in this session; the database pass measured 1.14 ms on its own session, both with `auto_explain` timing every node: the instrumented figures move by up to 50 % between sessions, the statement and buffer counts do not.)

### Indexes

Indexes of the schema at the baseline (51, including primary keys; sizes on the demo world):

| Table | Index | Size | Definition |
|---|---|---|---|
| campaign_invites | campaign_invites_campaign_idx | 16 kB | campaign_invites USING btree (campaign_id) |
| campaign_invites | campaign_invites_pkey | 40 kB | UNIQUE campaign_invites USING btree (token_hash) |
| campaign_members | campaign_members_organiser_key | 16 kB | UNIQUE campaign_members USING btree (campaign_id) WHERE (role = 'organiser'::text) |
| campaign_members | campaign_members_pkey | 16 kB | UNIQUE campaign_members USING btree (campaign_id, user_id) |
| campaign_members | campaign_members_user_idx | 16 kB | campaign_members USING btree (user_id) |
| campaigns | campaigns_pkey | 16 kB | UNIQUE campaigns USING btree (id) |
| companies | companies_id_campaign_key | 16 kB | UNIQUE companies USING btree (id, campaign_id) |
| companies | companies_owner_idx | 16 kB | companies USING btree (campaign_id, owner_id, id) |
| companies | companies_pkey | 16 kB | UNIQUE companies USING btree (id) |
| company_perks | company_perks_company_idx | 16 kB | company_perks USING btree (company_id, ((used_on IS NOT NULL)), id) |
| company_perks | company_perks_pkey | 16 kB | UNIQUE company_perks USING btree (id) |
| company_units | company_units_company_id_position_key | 56 kB | UNIQUE company_units USING btree (company_id, "position") |
| company_units | company_units_pkey | 48 kB | UNIQUE company_units USING btree (id) |
| fallen_units | fallen_units_company_id_position_key | 16 kB | UNIQUE fallen_units USING btree (company_id, "position") |
| fallen_units | fallen_units_pkey | 16 kB | UNIQUE fallen_units USING btree (id) |
| fallen_units | fallen_units_unit_id_key | 16 kB | UNIQUE fallen_units USING btree (unit_id) |
| goose_db_version | goose_db_version_pkey | 16 kB | UNIQUE goose_db_version USING btree (id) |
| grants | grants_company_idx | 16 kB | grants USING btree (company_id, id DESC) |
| grants | grants_pkey | 16 kB | UNIQUE grants USING btree (id) |
| match_aftermaths | match_aftermaths_company_idx | 16 kB | match_aftermaths USING btree (company_id) |
| match_aftermaths | match_aftermaths_pkey | 16 kB | UNIQUE match_aftermaths USING btree (match_id, company_id) |
| match_ato | match_ato_pkey | 8192 bytes | UNIQUE match_ato USING btree (match_id) |
| match_end_confirmation | match_end_confirmation_pkey | 8192 bytes | UNIQUE match_end_confirmation USING btree (match_id, company_id) |
| match_end_proposal | match_end_proposal_pkey | 16 kB | UNIQUE match_end_proposal USING btree (match_id) |
| match_events | match_events_match_idx | 16 kB | match_events USING btree (match_id, id) |
| match_events | match_events_pkey | 16 kB | UNIQUE match_events USING btree (id) |
| match_live | match_live_pkey | 16 kB | UNIQUE match_live USING btree (match_id) |
| match_live_log | match_live_log_live_idx | 16 kB | match_live_log USING btree (match_id, seq DESC) WHERE (undone_at IS NULL) |
| match_live_log | match_live_log_match_id_seq_key | 16 kB | UNIQUE match_live_log USING btree (match_id, seq) |
| match_live_log | match_live_log_pkey | 16 kB | UNIQUE match_live_log USING btree (id) |
| match_live_unit | match_live_unit_company_idx | 16 kB | match_live_unit USING btree (match_id, company_id) |
| match_live_unit | match_live_unit_pkey | 40 kB | UNIQUE match_live_unit USING btree (match_id, unit_id) |
| match_objectives | match_objectives_pkey | 16 kB | UNIQUE match_objectives USING btree (match_id, company_id, key) |
| match_side_companies | match_side_companies_company_idx | 16 kB | match_side_companies USING btree (company_id) |
| match_side_companies | match_side_companies_match_id_position_slot_key | 16 kB | UNIQUE match_side_companies USING btree (match_id, "position", slot) |
| match_side_companies | match_side_companies_pkey | 16 kB | UNIQUE match_side_companies USING btree (match_id, company_id) |
| match_sides | match_sides_pkey | 16 kB | UNIQUE match_sides USING btree (match_id, "position") |
| match_sides | match_sides_role_key | 16 kB | UNIQUE match_sides USING btree (match_id, role) WHERE (role IS NOT NULL) |
| matches | matches_campaign_id_number_key | 16 kB | UNIQUE matches USING btree (campaign_id, number) |
| matches | matches_campaign_idx | 16 kB | matches USING btree (campaign_id, number DESC) |
| matches | matches_id_campaign_id_key | 16 kB | UNIQUE matches USING btree (id, campaign_id) |
| matches | matches_pkey | 16 kB | UNIQUE matches USING btree (id) |
| rulesets | rulesets_default_key | 16 kB | UNIQUE rulesets USING btree (kind) WHERE is_default |
| rulesets | rulesets_hash_kind_key | 16 kB | UNIQUE rulesets USING btree (hash, kind) |
| rulesets | rulesets_pkey | 16 kB | UNIQUE rulesets USING btree (hash) |
| sessions | sessions_expires_at_idx | 16 kB | sessions USING btree (expires_at) |
| sessions | sessions_pkey | 16 kB | UNIQUE sessions USING btree (token_hash) |
| sessions | sessions_user_id_idx | 16 kB | sessions USING btree (user_id) |
| unit_equipment | unit_equipment_pkey | 56 kB | UNIQUE unit_equipment USING btree (unit_id, origin, "position") |
| users | users_email_key | 16 kB | UNIQUE users USING btree (lower(email)) |
| users | users_pkey | 16 kB | UNIQUE users USING btree (id) |

The final schema has the same 51 indexes (counted in the final session, `pg_indexes`; no migration was added by the passes): `EXPLAIN` of the 32 distinct statements of a `BenchmarkPages` run with `enable_seqscan = off` shows an index path for every statement the application runs, the sequential scans in the plans above are the planner's choice on tables of 40 to 1,300 rows. No unused index is a candidate for removal: the primary keys and unique constraints that never show a scan (`fallen_units_unit_id_key`, `match_live_log_match_id_seq_key`, `match_sides_role_key`, `matches_campaign_id_number_key`) are integrity rules, and `sessions_user_id_idx` serves the foreign key. `matches_campaign_idx (campaign_id, number DESC)` is covered by the unique `(campaign_id, number)` (a b-tree is read backwards), so it is redundant; dropping it saves 16 kB and one index update per match, nothing a page or a test can measure, so there is no migration.

## D. Build and test

### Build

| Measure | Baseline | Final |
|---|---|---|
| `go build ./cmd/server`, empty build cache, modules cached (3 builds: baseline 8.4, 5.9, 7.1 s; final 6.5, 5.9, 5.9 s) | 7.1 s | 5.9 s |
| `go build`, nothing changed | 0.20 s | 0.21 s |
| `go build` after a change in `internal/core` | 1.3 s | 1.15 s |
| Binary, default flags (darwin/arm64) | 31.2 MB | 31.3 MB |
| Binary, `-s -w` (darwin/arm64) | 22.3 MB | 22.4 MB |
| Binary as the `Dockerfile` builds it (linux/arm64, `-trimpath -s -w`) | 21.7 MB | 21.7 MB |
| `docker build --no-cache` (distroless runtime, stylesheet stage included) | 26.6 s | 20.8 s |
| Image size (`docker images`) | 26.1 MB | 26.1 MB |

The cold build differs by less than the spread of the baseline's own three builds (5.9 to 8.4 s); read it as equal.

### Go tests

| Run | Baseline | Final |
|---|---|---|
| `go test -count=1 ./...` without a database (median of 3; packages run side by side) | 30.5 s | 31 s (31, 31, 30) |
| `make test` with the compose PostgreSQL up (PostgreSQL tests run; also docs links, group check and 61 Python tests in 5.8 s), median of 3 | 139.9 s | 86 s (83, 86, 90) |
| `make test` with an empty build cache, 1 run | 141.3 s | 96 s |
| `go test -p 1 -count=1 ./...` with a database (`make test-integration`), 1 run | 191 s | 136 s |

Per package (seconds; the test binary's own time):

| Package | Baseline: no database (median of 3) | `make test` (median of 3) | `-p 1` with a database (1 run) | Final: no database | `make test` | `-p 1` with a database |
|---|---|---|---|---|---|---|
| `cmd/rulecheck` | 1.7 | 1.9 | 1.6 | 1.3 | 1.4 | 1.4 |
| `cmd/server` | 1.1 | 132.3 | 118.5 | 0.9 | 78.4 | 68.1 |
| `internal/core` | 29.7 | 33.5 | 29.1 | 18.5 | 20.3 | 17.2 |
| `internal/core/lean` | 0.8 | 0.8 | 0.4 | 1.1 | 0.9 | 0.4 |
| `internal/core/ttshelper` | 0.6 | 0.9 | 0.4 | 0.9 | 1.0 | 0.4 |
| `internal/postgres` | 1.3 | 21.1 | 10.7 | 1.2 | 23.8 | 15.5 |
| `internal/web` | 28.9 | 31.9 | 28.0 | 30.0 | 31.2 | 28.1 |

`cmd/server` with a database is the package that moved (132 to 78 s in `make test`): it runs four long tests side by side that wait for the demo-world seed. `internal/postgres` is slower in `make test` than at the baseline (23.8 against 21.1 s, 3 more database tests); `internal/web` is as it was.

Slowest single tests: with a database `TestLiveActionsSurviveExtremeValues` 108.7 s, `TestMalformedInputNeverCrashesAHandler` 67.3 s,
`TestSeedWorld` 63.0 s, `TestPermissionMatrix` 51.2 s (they run side by side in `cmd/server`), `TestHostileUploadsAnswer4xx` 7.7 s;
with the final tree `TestLiveActionsSurviveExtremeValues` 62.8 s, `TestMalformedInputNeverCrashesAHandler` 36.9 s, `TestSeedWorld` 31.7 s, `TestPermissionMatrix` 28.7 s, `TestHostileUploadsAnswer4xx` 7.3 s (the demo-world tests wait for the seeding of the world, which is now 23 s instead of 38 s; see [Database pass](#database-pass)); without a database `TestSnapshot` 4.1 s, `TestCustomRulesetWho` 3.0 s, `TestStartingCompanyPointsPlusGoldAre105InEveryCompany` 2.6 s,
`TestCustomRulesetCompanyAndLimits` 2.0 s in `internal/core`, and `internal/web` 0.7 s for its slowest (about 29 s there comes from many small tests).

### Browser tests (`make e2e`)

| Group | Steps | Baseline: local wall time (`make e2e GROUP=…`) | Final: local | CI job, PR #12 (job / test step) | CI job, PR #16 (job / test step) |
|---|---|---|---|---|---|
| core | 57 | 214 s (harness 211 s) | 221 s (217 s) | 358 s / 332 s | 271 s / 253 s |
| pages | 12 | 245 s (243 s) | 242 s (239 s) | 412 s / 395 s | 392 s / 373 s |
| creation | 101 | 407 s (404 s) | 407 s (403 s) | 575 s / 553 s | 540 s / 508 s |
| rules | 78 | 297 s (294 s) | 297 s (293 s) | 456 s / 435 s | 410 s / 387 s |
| All four, one after the other | 248 | 1,163 s | 1,167 s | | |

All 248 steps passed in the final run (57 + 12 + 101 + 78, 0 failed). The local time is the browser's: the suite waits on steps and animations.

CI, as GitHub reported it (`gh run view`). PR #12 (head `ae502d6`): workflow `e2e` from 14:30:49 to 14:40:43 UTC, 594 s wall; the four group jobs run in
parallel, so the longest (creation, 575 s) sets the time; job `plan` 6 s, `pr-check` 9 s, the final gate job 2 s. PR #16 (head `a280fb9`): 19:47:57 to 19:57:17 UTC,
560 s wall (-5.7 %), creation again the longest (540 s); `plan` 7 s, `pr-check` 8 s, gate 3 s. The PRs between them: #13 (measuring tools only, no application change) 505 s, #14 (backend) 521 s,
#15 (frontend) 514 s. The 90 s spread between PRs with the same suite is the runners'. A CI job spends about 15 to 20 s
on checkout, Go, Node and the Tailwind binary before the tests. The pull-request workflows do not run the Go unit tests.

## What is heaviest at the baseline

Facts the optimization passes started from (the baseline numbers above; profile from `go test ./cmd/server -bench 'Pages/(home|matches|overview|tts$|armoury|my-company)$'
-cpuprofile`):

- **Server time is Go CPU and garbage collection, not the database.** On the campaign list, matches tab and overview the page costs
  64 to 95 ms with 5 to 11 statements and 100 MB, 99 MB and 66 MB allocated per request; the garbage collector's mark work is about 45 % of the
  CPU of those benchmarks. `companyDeps.figures` (`BuildDetail` for every company to rate it) is 27 % of the time, `buildUnit` 26 %,
  `equipmentFor` 17 %, `completeBundle` 9 %, `BCBook.Offer` 8 %.
- **Largest pages:** the armoury is 449 KB of HTML (28 KB gzip), my company and the live company page 250 KB (29 to 31 KB gzip) with
  1,943 and 2,343 DOM elements; the armoury has 5,115 elements.
- **Slowest in the browser at 300 ms:** the match page settles at 1.0 s (three links in its chain, 68 statements across its htmx cards);
  my company and the armoury load at 0.7 s with two document requests (the alias redirect); the login flow 1.7 s.
- **Rule-set load:** 65 ms and 34 MB for the three files per content hash, more than v1.3.0 (56 ms, 19 MB); the decoder is 7 ms of it, the
  rest is building the book, glossary and the second vanilla parse.
- **Statement shapes:** the multi-sub-select statements cost more to plan (1.7 to 4.5 ms) than to execute (0.2 to 5 ms); the largest
  execution is the campaign roster read (5 ms, 1,478 buffers).
- **Tests:** `make test` is 140 s, of which `cmd/server` with the database is 132 s (four long tests that run side by side);
  `internal/core` and `internal/web` take about 30 s each without a database. The end-to-end suite is 19 minutes locally.

## Backend pass

PR #14 (`internal/core`). Same machine, same demo world (a scratch copy of the
template, 130 companies), same commands. Rules, screens and tests are unchanged: the rendered body of all 77 benchmarked pages (the 14 pages of
`BenchmarkPages` plus the overview, matches tab and armoury of every campaign) is byte identical before and after, apart from the static `?v=`
hashes (`PAGES_DUMP=<dir>`, diffed against a worktree of the baseline commit); the golden snapshot, aura and Tabletop Simulator goldens did not
change. The sequence of changes was profile, change one thing, `BenchmarkPages -count 5` (or `BenchmarkLoadRuleSet -count 10`), keep it only if
the gain was clear (more than 5 % of time on its benchmark or more than 10 % of allocations). Times are medians; the commits are in this order.

### Kept

| # | Change | Measured (before to after) |
|---|---|---|
| 1 | **`equipmentFor` remembered per rule set** (`RuleSet.equipment`, key: company, armoury size, item, bought, profile; 20,000 entries at most). It was 17 % of the CPU on its own and, through `Offer` (a scan of the armoury per item), `itemAs` in `completeBundle` and `applyGearStats`, about half of every page that builds a company: per item and unit it scanned the armoury, normalised names and ran a regular expression over the description. | home 91 to 42 ms, 101.5 to 16.5 MB; matches 95 to 37 ms, 98.8 to 15.8 MB; overview 63 to 25 ms, 66 to 10.6 MB; my company 43 to 25 ms; armoury 55 to 27 ms; Tabletop Simulator 42 to 18 ms |
| 2 | **`companyFigures`**: the campaign list, the overview and the match pages rate every company of a campaign (`figures`, `rated`) and needed four numbers, but built a whole `CompanyDetail` (keywords, auras, limits, recruit charts, cemetery) for each. It builds the units (keywords and auras aside, which feed nothing in the rating) and sums the figures; `TestCompanyFiguresEqualBuildDetail` compares the two. | home 42 to 28 ms, 16.5 to 9.3 MB, 169k to 99k allocations; overview 25 to 18 ms; matches 37 to 28 ms |
| 3 | **`NameKey` on ASCII names** without rune decoding (stack buffer; the string itself when it already is a key). `TestASCIINameKeyEqualsTheRuneDefinition` compares it with the rune-by-rune definition on 20,000 random strings. | 148 to 46-56 ns and 3 to 0-1 allocations per call; home 99k to 77k allocations; `LoadRuleSet` 484k to 441k, 66.4 to 63.7 ms; `lean.Decode` 7.4 to 5.9 ms |
| 4 | **Vanilla duplicates compared only when a name occurs twice** (`duplicates.wins` encoded every vanilla entry to JSON to have a form to compare). | `LoadRuleSet` 63.7 to 56.8 ms, 441k to 407k allocations |
| 5 | **The vanilla file is decoded once per load** (`decodeVanilla` then `vanillaDoc.book` twice: the book the Battle Companies file is checked against, and the one with the duplicates it pins). | 56.8 to 51.6 ms, 407k to 380k allocations |
| 6 | **A missing vanilla field is not decoded** (`unmarshalField`): decoding a missing key builds and rewrites an error that every caller ignored (`transformSyntacticError` was 7 % of the load's allocations). | 51.6 to 50.3 ms, 380k to 349k allocations |
| 7 | **Glossary sort keys once per term** instead of lower-casing and trimming in every comparison. | 50.3 to 48.6 ms, 349k to 327k allocations |
| 8 | **The lean binder builds a field's JSON path only for fields present** (`bindStruct`). | `lean.Decode` 5.8 to 5.3 ms, 327k to 312k allocations |
| 9 | **Equipment list and stat block sized once** (`buildUnit`, `VanillaBook.Profile`): doubling a slice of several-hundred-byte structs for every unit of every page. | home 9.1 to 7.4 MB, matches 8.8 to 7.2 MB, my company 7.5 to 6.9 MB per request |
| 10 | **A glossary term remembers its `Summary(n)`** per length (every unit asked for the same tooltip texts: join, split into fields, count runes, cut). | my company 21.2 to 19.9 ms and 6.9 to 5.7 MB; live company page 5.2 to 4.3 MB |

Result, `BenchmarkPages` (median of 5, `-benchtime 20x`), against the baseline of section C:

| Page | Baseline ms, MB, allocations (thousands) | Now |
|---|---|---|
| home (`/`) | 90.1, 101.5, 216.6 | 26.8, 7.4, 73.6 |
| campaign overview | 63.3, 66.0, 142.8 | 19.1, 4.9, 51.0 |
| my company | 43.4, 40.4, 121.6 | 20.4, 5.7, 71.7 |
| armoury | 53.8, 46.5, 194.1 | 23.4, 9.5, 139.0 |
| matches list | 94.8, 98.8, 213.0 | 24.7, 7.2, 75.4 |
| match page | 25.0, 14.0, 40.8 | 21.7, 3.3, 28.2 |
| live company page | 26.6, 11.3, 61.6 | 22.5, 4.3, 53.0 |
| aftermath | 14.1, 9.2, 30.3 | 9.8, 1.7, 20.4 |
| new company | 5.3, 3.8, 35.9 | 3.2, 1.7, 28.1 |
| Tabletop Simulator dialog, rings on | 41.1, 40.5, 88.1 | 12.7, 3.7, 35.3 |

The pages that were GC-bound (campaign list, matches tab, overview, Tabletop Simulator) are 3 to 4 times faster; the rest of what a request
costs now is PostgreSQL round trips, the roster decode in `internal/postgres` and the template. The two click flows that only open a window (recruit, level-up) are within the noise of the baseline (`make perf` runs 5 flows of 5 reps; 20 and 24 ms of server time against 34 and 19: those requests build one company and were never GC-bound). Memory of the server process did not move
(176 MB against 180 MB after 200 requests): it is the rule sets, templates and caches, and the caches added here are small. The statement counts
did not change. `make test` went from 139.9 to 110 s because `internal/core` and `internal/web` build many companies (`internal/core` 29.7 to 18.4 s).

Rule-set load, `BenchmarkLoadRuleSet` (median of 10): 66.1 ms, 34.2 MB, 483,790 allocations to 48.7 ms, 26.1 MB, 313,680. That is below the v1.3.0
figure in time (56.5 ms) but still above it in memory (19.5 MB) and allocations (204k): what remains is decoding the 1.2 MB vanilla file into one
map of raw values per entry (the typed document `readUnit` reads field by field), the generic tree the lean decoder builds from the Battle Companies
file, the path strings of that tree, and building the glossary twice (once without the Battle Companies terms, to check the file against, once
with them). The upload check (`/tools/validate`) loads the rule set the same way, so it gets the same 25 % cut.

### Tried and not kept

- **`encoding/json/v2` for decoding and encoding** (Go 1.27, `BenchmarkJSON`, `internal/core/bench_json_test.go`, same inputs for both). In Go 1.27
  `encoding/json` is itself implemented on v2, so the v1 API and v2 with `json.DefaultOptionsV1()` are the same engine: vanilla decode 4.97 and 4.89 ms,
  glossary 0.72 and 0.72 ms, Tabletop Simulator payload (6 KB) 8 µs and 8 µs, event and settings round trip 2 µs and 2 µs. Only v2 with its own
  defaults is faster (vanilla 3.91 ms, glossary 0.54 ms, the Battle Companies file into a tree 1.71 against 3.05 ms; the payload is byte identical but
  8 µs either way). Those defaults are different behaviour (case-sensitive field names, an error for a repeated key, strict UTF-8, another
  `omitempty`), which would change what an uploaded rules file is accepted for, and the lean decoder's error positions and texts depend on the v1
  token API. The whole gain would be 1 to 2 ms once per rule set (2 to 4 % of a load) and nothing per request, so no call site was switched.
- **A separate memo for `BCBook.Offer`** (a map by company and name): not needed once `equipmentFor` is remembered, which is its only hot caller (`Offer` is gone from the profile after change 1).
- **`sync.Pool` for the template buffers** (`bytes.Buffer` growth is 10 % of the allocated bytes in `internal/web/render.go`): not tried, it is the
  web layer's and the pages are now dominated by round trips.
- **Skipping the first glossary build** (the one without the Battle Companies terms): `LeanBook` and the extras read its terms' paragraphs and
  links, so it cannot be reduced to names without changing what they see; not attempted.
- **Hub, session cache, goroutines**: profiled, not in any profile above 1 %; unchanged.

### What the backend pass left in the profile

Not in `core` (the database pass took it, change 3 and 5 below): `postgres.decodeUnits`/`ReadRoster` allocate about 10 % of a request's bytes (the roster JSON decoded per
company, with reflection-based case-insensitive field lookup `reflect.(*structType).FieldByNameFunc`, 3 %), and the statements with high planning
time listed in section C.

## Frontend pass

PR #15 (`internal/web`). Same machine and demo world; every change was measured alone and kept only for a measured gain.
Tools: `make perf REPS=3` (network, 0/100/300 ms; labels `fe-before` / `fe-after`), the new `tests/e2e/rendercost.mjs` (what Chrome spends after
the bytes arrive: CDP style, layout and script time, long tasks, first and largest contentful paint, layout shift; `--latency`, `--cold`, `--cpu 4`),
and a byte comparison of the rendered pages (`PAGES_DUMP`, before and after: the only differences are the intended ones, listed below).
What was already in place before this pass (versioned immutable static files, gzip, one script bundle, sidebar inside every page, lazy cards inline) is
described in [performance.md](performance.md); the pass looked for what is left.

### Kept

| # | Change | Measured effect |
|---|---|---|
| 1 | **Heading font preloaded** (`<link rel=preload as=font crossorigin>` under the stylesheet's own versioned address) | Layout shift on a first visit (cache disabled, 300 ms): campaign pages 0.378 to 0.019, live company page 0.522 to 0.029 (the same at 100 ms). The headings were laid out in the fallback serif and changed height when the font arrived after the stylesheet. First contentful paint unchanged (804 to 832 ms, noise) |
| 2 | **To Wound chart fetched when first wanted** (pointer, touch or focus on a button that opens it, or the click) instead of on every match and live page load | Live company page at 300 ms: 3 requests to 2, chain 2 to 1, 34.5 to 29.6 KB, settled 699 to 410 ms. The match page does not move (its chain is the event stream's initial sync). 43 KB of HTML and a template run per page view gone |
| 3 | **Boosted click on the My Company and Armory tabs answered without the redirect** (`HX-Push-Url` names the final address; full-page requests and history restores still redirect) | Click from the overview at 300 ms: My Company 676 to 366 ms, Armory 695 to 369 ms (requests 2 to 1, chain 2 to 1); at 100 ms 263 to 157 ms. Two more statements for the second campaign read |
| 4 | **Armoury unit lists built on opening** (`<template x-if="open">`, htmx processes the forms then) | Demo armoury, warm cache, median of 9: elements 5,116 to 1,760, script time 44 to 34 ms, task 99 to 91 ms; with the CPU slowed 4 times: script 177 to 138 ms, task 354 to 305 ms. Raw HTML unchanged (the repeated lists compress to 4 KB of 26) |
| 5 | **Armoury drawn with its default filter** (unavailable items hidden by CSS until Alpine starts; the "N hidden" line carries its count) | Layout shift on a first visit at 300 ms: 0.199 to 0.019 (63 items were painted for one round trip and 30 stayed) |

Page by page after the pass (`make perf REPS=3`, 300 ms): live match settled 699 to 410 ms, 34.5 to 29.6 KB, DOM 2,343 to 2,134 elements; match page 6 to 5 requests, 20.5 to 15.6 KB, DOM 1,053 to 844, settled 1,001 to 1,021 ms (noise; the initial-sync refetch decides it); armoury DOM 5,115 to 1,760, settled 722 to 710 ms. The other flows did not move (home 369 to 374, overview 370 to 373, my company 731 to 707, matches 377 to 365, aftermath 366 to 378 ms).

### Rejected, with the measurement

- **Minifying our own scripts**: they are 94 KB raw and 28.9 KB gzip of the 243 KB / 79 KB bundle (the rest is htmx, SortableJS and Alpine, already minified); minifying would save about 4 KB gzip (5 % of the bundle), once per release, and needs a build tool or a Go dependency nothing else uses.
- **Splitting SortableJS (45 KB, 14.9 KB gzip) off the bundle**: only the company and list pages use it, but a boosted navigation would have to load it lazily; the bundle is fetched once per release and then cached for a year.
- **Tailwind output**: the content scan reaches the vendor scripts and `dice3d.js`; excluding them (`@source not`) removes 308 bytes of 106,022 (0.3 %). The stylesheet is already minified (15.5 KB gzip) and has no unused rule families.
- **Alpine on the sidebar** (about 200 bindings on every page): `Alpine.initTree` of a clone of the sidebar takes 1.8 ms.
- **Whitespace in the HTML**: 11.6 of 257 KB raw (4.5 %) on my company, nothing after gzip.
- **gzip level 5 to 9**: my company 30.9 to 29.4 KB, live page 28.9 to 27.5 KB (about 5 %) for more CPU on every page; not taken.
- **`content-visibility: auto` on the armoury cards**: style and layout together are 22 ms of the 99; nothing to gain.
- **Early hints / preload of more than the heading font**: the stylesheet, script bundle and favicon are in the head already; the image used as a mask (12 KB) is decorative.

### Left open

- **Tooltip texts repeated in the HTML** (`data-tip-text`): 26 % of my company's raw HTML (67 of 257 KB, 48 distinct texts of 236). Removing them from the HTML altogether would take its gzip from 30.9 to 17.4 KB; a per-page table of the distinct texts (about 13 KB raw) would give about 22 KB (minus 29 %), for 12 template places and 3 scripts. The live page gains less (28.9 to about 27.7 KB). Brotli would find the repeats without any template change, but needs a new dependency.

## Database pass

PR #16 (`internal/postgres` and the read path). Same machine and demo world, scratch databases only (a seeded template `bcm_dbp_tpl`, copies made with `CREATE DATABASE … TEMPLATE`; the development database was never touched). Method: `BenchmarkPages` (`-benchtime 20x`, medians of 3 to 5, the numbers in section C), `auto_explain` and `EXPLAIN (ANALYZE, BUFFERS)` on the statements the pages run, `PERF_SQL=1` and the new `PERF_STACK` for who runs which statement, `pagetimes.mjs` (3 runs), `make perf REPS=3`, and for every change the rendered bodies of all 77 benchmarked pages (`PAGES_DUMP`) diffed against a worktree of the tip before the pass: byte identical, apart from the `?v=` hashes of the static files and one attribute (`data-live-since` on the match page's log slot). One change at a time, one commit each; the golden snapshot and every test are as they were (tests and test tools were added; no assertion was changed or removed; the web fixture got one more stub line for the new interface method).

### Kept

| # | Change | Measured (before to after) |
|---|---|---|
| 1 | **A read-only GET opens the read scope** (`readOnly` in `internal/web/session.go` around the GET pages and cards; `core.WithReadScope` was only on the aftermath page). The company page read the same company five times and the campaign four times, the match page reread the match eleven times. | statements: my company 16 to 9, match page 37 to 16, live company page 26 to 17, armoury 10 to 7, Tabletop Simulator dialog 8 to 5, a boosted click on the My Company tab 17 to 8 (the alias route is wrapped too, 34.7 to 19.5 ms of server time); ms: my company 20.3 to 16.7, match page 23.4 to 19.0, live page 22.0 to 16.8, Tabletop Simulator dialog 13.3 to 10.8 |
| 2 | **The scope shares loads in flight and remembers the roster and the match history** (the four cards of the match page are built side by side and each asked for the same rows; `ReadRoster` and `Events` are remembered like `Get` and `Read`). | overview 5 to 4 statements and 16.9 to 12.3 ms, matches tab 7 to 5 and 24.6 to 16.4 ms, match page 16 to 14 and 19.0 to 12.7 ms, Against the Odds card 12 to 10 and 16.1 to 9.6 ms |
| 3 | **`ReadRoster` returns typed rows** instead of one `jsonb_build_object` per unit with three sub-selects on `unit_equipment`: the unit columns and the three equipment lists from one lateral probe, the four jsonb columns of a unit decoded only when they hold more than their default. The statement was the largest of the baseline (5 ms, 1,478 buffers), and it was the statement, not the Go decode, that cost: 9.2 of the 12.7 ms of `ReadRoster` on the biggest campaign (37 companies, 536 units). | the statement 9.2 to 3.8 ms and 1,478 to 350 buffers (`EXPLAIN (ANALYZE, BUFFERS, TIMING OFF)`); `BenchmarkReadRoster` 12.7 to 4.3 ms; pages: campaign list 26 to 19 ms, matches tab 16.4 to 14.4, overview 12.3 to 10.3, match page 12.7 to 9.5 |
| 4 | **`atoStore.Get` and `GameStarted` check the campaign in the statement that reads their data** (they loaded the whole match, with its sides, companies, proposal and objectives as JSON, only to learn that it belongs to the campaign). | Against the Odds card 10 to 8 statements (6.6 to 5.8 ms), match page 14 to 12, aftermath 14 to 13, live page 17 to 16 |
| 5 | **Rate only what a page shows** (`ReadRosterOf(ids)`, an optional method of the company repository found by type assertion; the campaign list reads the player's companies of all campaigns in one statement). The list read the complete roster of each of the player's campaigns (ten statements, 12 ms of database time) to rate one or two companies of each. | campaign list 19.3 to 5.0 ms and 11 to 2 statements, my company 13.2 to 9.5 ms, Tabletop Simulator dialog 8.8 to 4.9, overview 10.3 to 8.2 (4 to 5 statements), matches tab 14.4 to 12.9 (5 to 6 statements) |
| 6 | **A company save writes the rows that changed** (`rowsOf` fingerprints the state after the locking read, `save` skips rows that did not move). A save wrote every unit: an UPDATE, a DELETE of its equipment and an INSERT per item, for fourteen units. | seeding the demo world 98,954 to 45,434 statements and 37.7 to 22.4 s; the stored units, equipment and graves of a seeded world are identical to the old code's; a sheet save of one unit writes 1 UPDATE, 1 DELETE and its items instead of the whole roster |
| 7 | **The match page's event stream starts with the state the cards were built from** (the generation counter of `…-ru1n`; details in [performance.md](performance.md#the-match-pages-stream)): the page reads the match revision and the live log sequence before it builds its cards, sends them as `?since=`, and `SubscribeSince` skips the initial batch that made the page fetch the end card, the end checks and the log again. | match page at 300 ms of latency: settled 1,021 to 384 ms (360 in the full `make perf`), requests 5 to 2, chain 2 to 1, 15.6 to 11.9 KB, statements 67 to 12, server time 76 to 24 ms; at 100 ms 383 to 165, at 0 ms 125 to 81 |
| 8 | **At least ten pooled connections** unless `DATABASE_URL` sets `pool_max_conns` (pgxpool's default is the number of CPUs, at least 4). | `BenchmarkPages/match-parallel` (160 match pages at once): 2.8 ms a page with 4 connections, 2.0 with 10, 1.8 with 20 |

Result, `BenchmarkPages` (median of 5), against the baseline of section C and the state after the frontend pass (the backend column of the table in [Backend pass](#backend-pass)):

| Page | Baseline ms (statements) | After the backend pass | After the database pass (the final tree, section C) |
|---|---|---|---|
| home (`/`) | 90.1 (11) | 26.8 (11) | 5.0 (2) |
| campaign overview | 63.3 (5) | 19.1 (5) | 8.3 (5) |
| my company | 43.4 (16) | 20.4 (16) | 9.5 (9) |
| armoury | 53.8 (10) | 23.4 (10) | 16.5 (7) |
| matches list | 94.8 (7) | 24.7 (7) | 13.0 (6) |
| match page | 25.0 (37) | 21.7 (37) | 7.6 (12) |
| Against the Odds card | 17.8 (14) | 16.3 (14) | 4.5 (8) |
| live company page | 26.6 (26) | 22.5 (26) | 12.6 (16) |
| aftermath | 14.1 (14) | 9.8 (14) | 8.3 (13) |
| Tabletop Simulator dialog, rings on | 41.1 (8) | 12.7 (8) | 4.9 (5) |

Test time: the demo-world tests of `cmd/server` wait for a seed of the world, which is 99,000 round trips at 0.38 ms. Change 6 halves the statements; `TestLiveActionsSurviveExtremeValues` 109 to 63 s, `TestMalformedInputNeverCrashesAHandler` 67 to 37 s, `TestSeedWorld` 63 to 32 s, `TestPermissionMatrix` 51 to 29 s, `cmd/server` 118.5 to 70.8 s, `make test-integration` 191 to 132 s (161 s after the backend pass), `make test` 83 s (110 s). No assertion was touched.

### Rejected or not done, with the measurement

- **Prepared or cached statements, `plan_cache_mode=force_generic_plan`.** pgx already prepares each statement once per connection, so the planning times of section C are first-call costs. `BenchmarkPages` at 300 requests per page gives the same times with and without `force_generic_plan` in the URL (campaign list 18.3 against 17.8 ms, my company 13.3 against 13.4, match page 10.5 against 10.9). Not set: a generic plan can pick the wrong index on a skewed table and there is nothing to win.
- **Rewriting the multi-sub-select statements** (matches of a campaign 1.3 ms and 553 buffers, company figures 1.3 ms, campaign list 0.6 ms): a page runs each once, after the changes above they are 5 to 15 % of a page's server time, and the plans are index scans or sequential scans of 40-row tables. The one statement that mattered was the roster, and it was reshaped (change 3).
- **A hand-written or exact-case decoder for the roster JSON.** The struct tags are already exact-case, `FieldByNameFunc` is not in the profile of `decodeUnits` in `internal/postgres` (the profile of the web server had it under the decoding of `core` types that is not in this package), and the whole decode of 130 companies is 3.6 ms (`BenchmarkDecodeUnits`). What cost was the statement (9.2 of the 12.7 ms), so the JSON is gone from the roster instead of being decoded faster; `Read` of one company still builds its units as JSON, 0.24 ms.
- **Pool: a larger default than ten, `pool_min_conns`.** Sequential requests do not change with the pool size (2, 4, 10 or 25 connections: my company 13.1 to 13.3 ms); only simultaneous ones do, and 20 or 40 connections gain 10 to 15 % more over 10 at the cost of server memory.
- **Indexes.** No migration: see section C, "Indexes". `matches_campaign_idx` is redundant and its removal is not measurable.
- **`synchronous_commit = off` for the seed.** 44.5 s against 37.9 s: the seed is bound by round trips, not by the commit.
- **`pgx.Batch` for the unit writes of a save.** Not needed after change 6 (a save of one unit is a handful of statements); it would also change how errors surface.
- **Merging the remaining statements of a page** (my company: the sidebar list, the company, its aftermath history, the membership check, the grants, the campaign, the rating figures, the perks): each is a different aggregate, a merge would cost the shape of the repositories for about 0.15 ms a statement.
- **A scope that serves `ReadRosterOf` from a whole roster it already holds.** The order of the calls on a page (the player's own company first) makes it help only by luck; the overview and the matches tab pay one small extra statement for it.
- **The live write path** (`liveStore.Update`: lock, snapshot, sequence, insert, notify, commit) is already lean; the sweep tests are bound by request handling and rules code.

### Left open

- **Matches list** (`matchStore.List`: 25 matches with eight sub-selects each, 1.3 ms) and the figures statement of `campaignStore.Companies` (1.3 ms) are the largest statements left; each page runs one of them once.

## Reproducing the numbers

Commands, from the repository root (the compose PostgreSQL up; `make e2e`, `make perf` and the scripts copy the cached seeded template
into a scratch database and never touch the development one):

```bash
# A. data format
python3 - <<< 'import json,gzip,sys; b=open("definitions/battle_companies/battle_companies.json","rb").read(); print(len(b), len(json.dumps(json.loads(b),separators=(",",":"),ensure_ascii=False).encode()), len(gzip.compress(b,9)))'
go test ./internal/core -run 'Magic|TemplateRule' -v
go test ./internal/core -run xxx -bench LoadRuleSet -benchmem -count 10
go test ./internal/core -run xxx -bench JSON -benchmem -count 6        # encoding/json against encoding/json/v2

# B. browser flows and server time per page
make perf LABEL=final REPS=5                         # tests/e2e/out/perf-final.json
node tests/e2e/rendercost.mjs --label final --reps 7 [--latency 300 --cold] [--cpu 4]   # browser cost of the big pages (tests/e2e/out/rendercost-final.json)
node tests/e2e/pagetimes.mjs --label final1          # tests/e2e/out/pagetimes-final1.json (run it three times: final1, final2, final3)

# C. database (WORLD_DATABASE_URL: a scratch copy of the template database that make e2e or make perf created, bcm_e2e_tpl_<hash>)
WORLD_DATABASE_URL=postgres://bcm:bcm@localhost:5432/<scratch>?sslmode=disable \
  go test ./cmd/server -run xxx -bench Pages -benchtime 20x -benchmem -count 5
WORLD_DATABASE_URL=… go test ./cmd/server -run TestQueryCountsOnAPreloadedWorld -v -count=1
WORLD_DATABASE_URL=… PERF_SQL=1 go test ./cmd/server -run xxx -bench Pages -benchtime 1x     # statements per page
WORLD_DATABASE_URL=… PAGES_DUMP=/tmp/pages go test ./cmd/server -run xxx -bench Pages/home -benchtime 1x   # the pages' bodies, to diff two builds
WORLD_DATABASE_URL=…'&pool_max_conns=4' go test ./cmd/server -run xxx -bench Pages/match-parallel -benchtime 400x   # 160 match pages at once; the pool size is a URL setting
WORLD_DATABASE_URL=… PERF_STACK='WHERE c.id = ANY' go test ./cmd/server -run xxx -bench Pages/home -benchtime 1x   # who runs a statement that contains the text
WORLD_DATABASE_URL=… go test ./internal/postgres -run xxx -bench 'Decode|ReadRoster' -benchmem   # the roster read and the decode of its units

# D. build and test
go build -o /tmp/server ./cmd/server && ls -l /tmp/server
go test -count=1 ./...            # no database
make test                         # with the compose database
make e2e GROUP=core               # and pages, creation, rules
make test-integration             # go test -p 1 with the database
DATABASE_URL=… go run ./cmd/server seed --world   # seeding time and statements (log_statement = 'all' on the scratch database counts them)
docker build --no-cache -t bcm-measure . && docker images bcm-measure
gh run view <run id> --json jobs,startedAt,updatedAt  # CI times
```

The slowest-statement list is made by setting `session_preload_libraries = 'auto_explain'` and `auto_explain.log_min_duration = 0` (with
`log_analyze`, `log_buffers` and `log_format = 'json'`) on the scratch database with `ALTER DATABASE`, running the benchmark, and reading the
container's log; `EXPLAIN (ANALYZE, BUFFERS)` of the slowest call of each statement uses the parameters the log records.
