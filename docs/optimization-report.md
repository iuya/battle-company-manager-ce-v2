# Optimization report

Numbers of the optimization work, taken on one machine with the tools of this repository, so that every pass can be compared with the same
yardstick. The baseline is the local tag `1.5.0-pre-optimization` (release/v2.0.0 at `b3300b3`, after "practice matches", PR #12). The
"After" columns are empty: each optimization pass fills them with the same commands ([Reproducing the numbers](#reproducing-the-numbers)).
The v1.3.0 column exists where a number could be taken from that release's files. Rules and screens do not change between baseline and
after; the tests are the safety net. How the earlier speed pass measured and what it changed is in [performance.md](performance.md).

Contents: [Summary](#summary), [Method](#method), [A. Data format](#a-data-format), [B. Runtime](#b-runtime),
[C. Database](#c-database), [D. Build and test](#d-build-and-test), [What is heaviest at the baseline](#what-is-heaviest-at-the-baseline),
[Reproducing the numbers](#reproducing-the-numbers).

## Summary

`-` in the v1.3.0 column: not measured at that release. Times are medians (see [Method](#method)); "After" is filled by the passes.

| Metric | v1.3.0 | Baseline | After |
|---|---|---|---|
| Battle Companies file, pretty (as shipped) | 920,796 B | 465,538 B | |
| Battle Companies file, compact | 609,349 B | 313,311 B | |
| Battle Companies file, gzip | 131,573 B | 76,977 B | |
| Battle Companies schema file | 93,953 B | 123,196 B | |
| Go code that parses the whole Battle Companies file when a rule set loads | 14 parsers | 1 decoder (`lean`) | |
| Non-test Go lines in `internal/core` (with subpackages) | 26,459 | 33,389 | |
| Python lines in `tools/` | 8,535 | 2,548 | |
| Rule-number literals reported by `TestMagicNumbersReport` | 171 | 153 | |
| `core.LoadRuleSet`, three shipped files | 56.5 ms, 19.5 MB, 203,903 allocs | 65.2 ms, 34.2 MB, 483,794 allocs | |
| `lean.Decode` alone, Battle Companies file | - | 7.3 ms, 4.6 MB, 124,956 allocs | |
| Campaign list `/campaigns`, server time (statements) | - | 91.5 ms (11) | |
| Matches tab, server time (statements) | - | 88.9 ms (7) | |
| My Company, server time (statements) | - | 40.7 ms (16) | |
| Armory, server time (statements) | - | 53 ms (10) | |
| Tabletop Simulator dialog, aura rings on, server time (statements) | - | 34.2 ms (7) | |
| My Company over the wire (HTML gzip) | - | 30.7 KB | |
| `make perf` my company, 300 ms latency: loaded / settled | - | 709 / 732 ms | |
| `make perf` match page, 300 ms latency: settled | - | 1002 ms | |
| Server memory (RSS) after 200 page requests | - | 180 MB | |
| Server start to first answer | - | 207 ms | |
| `go test ./...` without a database | - | 30.5 s | |
| `make test` (includes the PostgreSQL tests) | - | 139.9 s | |
| `make e2e`, the four groups one after the other | - | 1,163 s | |
| CI end-to-end workflow, PR #12 | - | 594 s | |
| `go build` of the server, cold build cache | - | 7.1 s | |
| Binary (release flags, linux/arm64) / Docker image | - | 21.7 MB / 26.1 MB | |

## Method

**Machine.** Apple M2 Pro (6 performance and 4 efficiency cores), 16 GB, macOS 14.4.1. PostgreSQL 17.11 in Docker Desktop 24.0.5 (5 CPUs,
7.7 GB) on the same host, the compose database of `compose.dev.yaml` with scratch databases beside the development one. Go 1.27.3,
Node 25.9.0, Python 3.11.4, Google Chrome 156 driven by puppeteer-core 24.4.0 (headless). The measurements ran one after the other with the
machine otherwise idle (CPU about 85 % idle between runs); times show proportions on this machine, not absolutes.

**World.** The demo world ([demo-world.md](demo-world.md)) seeded once into a template database and copied for each run (the mechanism of
`make e2e`); the development database is never touched. The pages are those of the demo accounts `eowyn@example.com` and
`admin@example.com` (campaign "Matches: every scenario and every state" for most pages).

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
| CI time | `gh run view` of the workflow runs of PR #12 (read only) | the one observed run |
| Image | `docker build --no-cache` of the repository's `Dockerfile` (base images already pulled) | 1 run |

Notes on the medians: the settled time under emulated latency varies by about 10 % between runs (the five values of each flow are in
`tests/e2e/out/perf-<label>.json`); the two Tabletop Simulator rows (rings on and off) differ by 5 ms in the HTTP probe and by 3.5 ms the other way in the benchmark; read them as equal within the noise.

`make test` runs the PostgreSQL tests too when the compose database is up, because the Makefile exports `TEST_DATABASE_URL` to every command;
`go test ./...` run by hand without that variable skips them. Both are reported.

## A. Data format

The Battle Companies file at the baseline is the schema-version-2 ("lean") format read by one decoder, `internal/core/lean`. The
vanilla file, the glossary and the concept graph did not change in format.

### Files

| File | Pretty (as shipped) | Compact | Pretty gzip | Compact gzip |
|---|---|---|---|---|
| `battle_companies.json`, v1.3.0 | 920,796 | 609,349 | 131,573 | 119,372 |
| `battle_companies.json`, baseline | 465,538 | 313,311 | 76,977 | 70,590 |
| `battle_companies.json`, after | | | | |
| `battle_companies.schema.json`, v1.3.0 | 93,953 | 46,414 | 13,041 | 10,040 |
| `battle_companies.schema.json`, baseline | 123,196 | 81,884 | 20,065 | 18,184 |
| `battle_companies.schema.json`, after | | | | |
| `data2024.json` (vanilla), both | 1,178,030 | 882,090 | 157,596 | 148,097 |
| `mesbg_glossary.json`, both | 248,443 | 199,490 | 50,913 | 49,024 |
| `concept_graph.json`, v1.3.0 / baseline | 247,512 / 247,686 | 174,559 / 174,707 | 39,435 / 39,468 | 38,449 / 38,486 |

Bytes. The three files a rule set is built from (vanilla, Battle Companies, glossary) are 2,347,269 bytes at v1.3.0 and 1,892,011 at the baseline.

### Code

| Measure | v1.3.0 | Baseline | After |
|---|---|---|---|
| Functions that parse the whole Battle Companies file when a rule set loads (`json.Unmarshal` into a document type) | 14 (`ParseBattleCompanies`, `parsePathRules`, `ParseVanillaAmbiguities`, `ParseProgression`, `ParseCampaignOptions`, `ParseScenarios`, `ParseATO`, `ParseCreaturesAndWanderers`, `ParseAftermath`, `ParseInjuries`, `parseProfiles`, `ParseAuras`, `ParseTTSModels`, `GlossaryExtras`) | 1 (`lean.parseJSON`, behind `lean.Decode`) | |
| Passes that only look at the file (shape limits, header, `schemaVersion`) | 2 (`checkRulesetShape`, `RulesetMeta`) | 3 (`checkRulesetShape`, `RulesetMeta`, `schemaVersionOf`) | |
| Parsers of the file in the demo-world seeder (`cmd/server`) | 3 | 1 | |
| Non-test Go lines, `internal/core` (79 / 118 files) | 26,459 | 33,389 (package `core` 27,991, `lean` 4,536, `ttshelper` 862) | |
| Non-test Go lines, whole repository | 43,751 | 52,016 | |
| Python lines, `tools/` (all) | 8,535 | 2,548 | |
| Python lines, `tools/`, without tests | 7,946 | 2,054 | |
| Go lines, `tools/` | 0 | 78 | |
| Rule-number literals (`TestMagicNumbersReport`: integer literals other than 0, 1, 2 outside the allow-list in non-test `internal/core/*.go`) | 171 (and 5 engine constants the lint wants in data) | 153 (0 engine constants) | |
| Rule literals in templates (`TestTemplateRuleLiteralsReport`: "up to N", "ND6") | - | 8 | |

The v1.3.0 literal count is the baseline's test file run on the v1.3.0 sources (the test did not exist then); the allow-list is the
baseline's.

### Loading the rule set

`go test ./internal/core -run xxx -bench LoadRuleSet -benchmem -count 10` (`BenchmarkLoadRuleSet` in `internal/core/bench_loadruleset_test.go`),
the three shipped files, median of 10:

| Benchmark | Wall time | Bytes allocated | Allocations | After |
|---|---|---|---|---|
| `core.LoadRuleSet`, v1.3.0 (same call, old parsers) | 56.46 ms | 19,472,288 | 203,903 | |
| `core.LoadRuleSet`, baseline | 65.22 ms (64.77 to 65.54) | 34,211,698 | 483,794 | |
| `lean.Decode` alone, baseline (Battle Companies file, prepared vanilla book and glossary) | 7.33 ms (7.32 to 7.37) | 4,565,394 | 124,956 | |

A rule set is built once per content hash and kept (see [architecture.md](architecture.md)), so this cost is paid at start and when a
ruleset is uploaded, not per request.

## B. Runtime

### B1. Browser flows (`make perf`)

`node tests/e2e/perf.mjs --label baseline --reps 5` (the target `make perf REPS=5`), headless Chrome, 5 runs per flow and latency,
the run with the median settle time kept. Reads only (no flow saves). "Requests" are those that went over the network; "+ cached" were
answered from the browser cache. Chain is the longest series of requests where each starts after the previous one ended. The flow "my
company" opens `/my-company`, which redirects to the company: two document requests. The login flow is the GET, POST, redirect and home page.

TTFB is the browser's `responseStart`. Under Chrome's network emulation the added latency delays the response body, not that timestamp, so
read the time to load and the settled time for the visitor's wait. Server ms and statements are the sums over the flow's document and htmx
requests.

Per flow:

| Flow | Requests (doc / xhr / sse / static) + cached | Chain | KB on the wire | KB HTML | DOM elements | Server ms | Statements | After: requests | After: KB | After: server ms | After: statements |
|---|---|---|---|---|---|---|---|---|---|---|---|
| login | 11 (2 / 1 / 0 / 8) + 9 | 6 | 164.3 | 11.4 | 449 | 110 | 15 | | | | |
| home | 1 (1 / 0 / 0 / 0) + 8 | 1 | 7.5 | 7.5 | 449 | 125.4 | 11 | | | | |
| home, cold cache | 9 (1 / 0 / 0 / 8) + 0 | 5 | 156.8 | 7.5 | 449 | 115.9 | 11 | | | | |
| campaign overview | 1 (1 / 0 / 0 / 0) + 8 | 1 | 7.5 | 7.5 | 434 | 81.1 | 5 | | | | |
| my company | 2 (2 / 0 / 0 / 0) + 9 | 2 | 31.6 | 31.6 | 1943 | 58.9 | 16 | | | | |
| recruit window (click) | 1 (0 / 1 / 0 / 0) + 0 | 1 | 2.0 | 2.0 | 1999 | 33.8 | 12 | | | | |
| level-up window (click) | 1 (0 / 1 / 0 / 0) + 0 | 1 | 4.0 | 4.0 | 3094 | 18.9 | 8 | | | | |
| click: overview to my company (htmx) | 2 (0 / 2 / 0 / 0) + 2 | 2 | 27.2 | 27.2 | 1943 | 67.8 | 15 | | | | |
| click: home to campaign overview (htmx) | 1 (0 / 1 / 0 / 0) + 1 | 1 | 3.0 | 3.0 | 434 | 90.5 | 4 | | | | |
| armoury | 2 (2 / 0 / 0 / 0) + 9 | 2 | 28.4 | 28.4 | 5115 | 64.4 | 10 | | | | |
| matches list | 1 (1 / 0 / 0 / 0) + 9 | 1 | 7.6 | 7.6 | 564 | 119.6 | 7 | | | | |
| match page | 6 (1 / 4 / 1 / 0) + 8 | 3 | 20.5 | 20.5 | 1053 | 94 | 68 | | | | |
| live match (+3 s observed) | 3 (1 / 1 / 1 / 0) + 9 | 2 | 34.5 | 34.5 | 2343 | 45.2 | 27 | | | | |
| aftermath | 1 (1 / 0 / 0 / 0) + 9 | 1 | 8.1 | 8.1 | 586 | 33.7 | 14 | | | | |

A live match opens no polling request (0 extra requests in 3 s after it settled). A page that sits on its event stream shows the stream as
one request.

Per flow and latency (milliseconds; first paint is the first contentful paint):

| Flow | Latency | TTFB | First paint | Load | Settled | After: TTFB | After: first paint | After: load | After: settled |
|---|---|---|---|---|---|---|---|---|---|
| login | 0 ms | 84 | 368 | 111 | 266 | | | | |
| login | 100 ms | 110 | 320 | 140 | 640 | | | | |
| login | 300 ms | 115 | 520 | 347 | 1655 | | | | |
| home | 0 ms | 127 | 292 | 146 | 167 | | | | |
| home | 100 ms | 107 | 280 | 130 | 153 | | | | |
| home | 300 ms | 107 | 504 | 335 | 360 | | | | |
| home, cold cache | 0 ms | 117 | 212 | 139 | 163 | | | | |
| home, cold cache | 100 ms | 124 | 324 | 361 | 499 | | | | |
| home, cold cache | 300 ms | 106 | 704 | 945 | 1263 | | | | |
| campaign overview | 0 ms | 84 | 244 | 105 | 132 | | | | |
| campaign overview | 100 ms | 87 | 264 | 129 | 154 | | | | |
| campaign overview | 300 ms | 78 | 496 | 341 | 365 | | | | |
| my company | 0 ms | 96 | 316 | 134 | 159 | | | | |
| my company | 100 ms | 186 | 452 | 270 | 292 | | | | |
| my company | 300 ms | 411 | 868 | 709 | 732 | | | | |
| recruit window (click) | 0 ms | 46 | 288 | 86 | 187 | | | | |
| recruit window (click) | 100 ms | 38 | 304 | 79 | 217 | | | | |
| recruit window (click) | 300 ms | 61 | 280 | 107 | 409 | | | | |
| level-up window (click) | 0 ms | 61 | 272 | 110 | 199 | | | | |
| level-up window (click) | 100 ms | 52 | 300 | 108 | 197 | | | | |
| level-up window (click) | 300 ms | 71 | 252 | 124 | 408 | | | | |
| click: overview to my company (htmx) | 0 ms | 83 | 240 | 102 | 153 | | | | |
| click: overview to my company (htmx) | 100 ms | 88 | 240 | 132 | 266 | | | | |
| click: overview to my company (htmx) | 300 ms | 87 | 240 | 129 | 678 | | | | |
| click: home to campaign overview (htmx) | 0 ms | 101 | 272 | 120 | 108 | | | | |
| click: home to campaign overview (htmx) | 100 ms | 131 | 308 | 151 | 133 | | | | |
| click: home to campaign overview (htmx) | 300 ms | 130 | 288 | 150 | 349 | | | | |
| armoury | 0 ms | 106 | 308 | 187 | 210 | | | | |
| armoury | 100 ms | 181 | 412 | 291 | 314 | | | | |
| armoury | 300 ms | 405 | 816 | 699 | 724 | | | | |
| matches list | 0 ms | 121 | 280 | 143 | 167 | | | | |
| matches list | 100 ms | 118 | 268 | 139 | 163 | | | | |
| matches list | 300 ms | 109 | 492 | 348 | 372 | | | | |
| match page | 0 ms | 46 | 208 | 81 | 132 | | | | |
| match page | 100 ms | 51 | 276 | 147 | 383 | | | | |
| match page | 300 ms | 51 | 540 | 361 | 1002 | | | | |
| live match (+3 s observed) | 0 ms | 44 | 224 | 95 | 134 | | | | |
| live match (+3 s observed) | 100 ms | 54 | 288 | 164 | 277 | | | | |
| live match (+3 s observed) | 300 ms | 62 | 496 | 373 | 701 | | | | |
| aftermath | 0 ms | 36 | 200 | 60 | 87 | | | | |
| aftermath | 100 ms | 33 | 264 | 139 | 166 | | | | |
| aftermath | 300 ms | 29 | 492 | 352 | 374 | | | | |

### B2. Server time and bytes per page (`pagetimes.mjs`)

`node tests/e2e/pagetimes.mjs`: plain HTTP, no browser or network emulation, so these are the server's own cost and the exact bytes. Server
ms is `Server-Timing: app` (time to the first byte), DB ms the time spent in statements (summed, so it can exceed the server time when
statements run in parallel, as on the match page where the four cards are built concurrently). Statements are those of the request itself,
including the session lookup. Every row is for one warm request after the rule sets are loaded. The Tabletop Simulator dialog is the htmx
fragment `…/companies/{company}/tts` (aura rings off is `?auras=0`); the new-company page is the one of campaign 21, the first campaign of the demo world that opens it for `eowyn@example.com` or `admin@example.com`. The CSS and JS columns are the page's referenced files (one stylesheet, one script bundle, the same on every page; served
gzip ahead of time and `immutable` for a year, so a warm visit does not fetch them).

| Page | Server ms (3 runs, min to max) | DB ms | Statements | HTML KB raw | HTML KB gzip | CSS KB raw / gzip | JS KB raw / gzip | After: server ms | After: DB ms | After: statements | After: HTML KB gzip |
|---|---|---|---|---|---|---|---|---|---|---|---|
| Campaign list | 91.5 (83.1 to 93.3) | 35.3 | 11 | 41.0 | 6.8 | 103.6 / 15.1 | 243.0 / 79.0 | | | | |
| Campaign overview | 64.4 (61.7 to 65.7) | 26 | 5 | 37.1 | 6.8 | 103.6 / 15.1 | 243.0 / 79.0 | | | | |
| My Company | 40.7 (40.3 to 43.5) | 14.8 | 16 | 251.6 | 30.7 | 103.6 / 15.1 | 243.0 / 79.0 | | | | |
| Matches tab | 88.9 (83.8 to 95) | 34.6 | 7 | 49.2 | 6.8 | 103.6 / 15.1 | 243.0 / 79.0 | | | | |
| Armory | 53 (48.9 to 53) | 20.6 | 10 | 449.0 | 27.6 | 103.6 / 15.1 | 243.0 / 79.0 | | | | |
| New company | 14.2 (11 to 14.8) | 3.8 | 3 | 61.3 | 7.3 | 103.6 / 15.1 | 243.0 / 79.0 | | | | |
| Match page | 23.4 (23.3 to 23.7) | 30.7 | 37 | 77.3 | 11.1 | 103.6 / 15.1 | 243.0 / 79.0 | | | | |
| Live match (company page) | 24.9 (24.8 to 25.8) | 16.5 | 26 | 248.6 | 28.7 | 103.6 / 15.1 | 243.0 / 79.0 | | | | |
| Aftermath | 13.5 (13.3 to 13.6) | 8 | 14 | 54.7 | 7.3 | 103.6 / 15.1 | 243.0 / 79.0 | | | | |
| Tabletop Simulator dialog, aura rings on | 34.2 (34.1 to 35.4) | 11.2 | 7 | 45.2 | 12.1 | - | - | | | | |
| Tabletop Simulator dialog, aura rings off | 39.4 (37 to 40.3) | 15.3 | 7 | 43.2 | 11.6 | - | - | | | | |

### B3. Server process

| Measure | Baseline | After |
|---|---|---|
| Start to first answer of `/healthz` and `/login` (warm binary, median of 15 starts) | 207 ms and 208 ms | |
| First start of a freshly built binary (cold page cache), 3 observations | 199, 634, 621 ms | |
| Memory (RSS) just after start | 115 MB | |
| Memory after one visit to each measured page (rule sets, templates, caches warm) | 177 MB | |
| Memory after 200 more page requests | 180 MB | |
| Memory after the measurement (about 450 requests in all) | 181 MB | |

## C. Database

### Statements per page

`BenchmarkPages` (`cmd/server/pagebench_test.go`) on a scratch copy of the demo world: time, memory and statements of one whole request
(handler, services, statements, template, gzip), without browser or network. `go test ./cmd/server -run xxx -bench Pages -benchtime 20x
-benchmem -count 5`, median of 5. The statements here come from the pgx tracer and include the session lookup but not the
rule-set loads (warm).

| Page | ms per request | MB allocated | Allocations (thousands) | Statements | KB | After: ms | After: MB | After: statements |
|---|---|---|---|---|---|---|---|---|
| home (`/`, campaign list) | 90.1 | 101.5 | 216.6 | 11 | 40.9 | | | |
| `/nav/campaigns` (sidebar fallback) | 1.0 | 0.2 | 3.8 | 1 | 19.3 | | | |
| campaign overview | 63.3 | 66.0 | 142.8 | 5 | 37.0 | | | |
| my company | 43.4 | 40.4 | 121.6 | 16 | 251.5 | | | |
| armoury | 53.8 | 46.5 | 194.1 | 10 | 448.9 | | | |
| matches list | 94.8 | 98.8 | 213.0 | 7 | 49.1 | | | |
| match page | 25.0 | 14.0 | 40.8 | 37 | 77.2 | | | |
| Against the Odds card | 17.8 | 6.7 | 14.6 | 14 | 0.5 | | | |
| live log card | 12.4 | 4.9 | 11.0 | 12 | 1.1 | | | |
| live company page | 26.6 | 11.3 | 61.6 | 26 | 248.6 | | | |
| aftermath | 14.1 | 9.2 | 30.3 | 14 | 54.6 | | | |
| new company | 5.3 | 3.8 | 35.9 | 2 | 80.5 | | | |
| Tabletop Simulator dialog, aura rings on | 41.1 | 40.5 | 88.1 | 8 | 45.2 | | | |
| Tabletop Simulator dialog, aura rings off | 37.6 | 40.2 | 83.4 | 8 | 43.2 | | | |

(The browser flow and the HTTP probe count a few more or fewer statements for the same page: the browser's match page adds its htmx cards,
and each tool picks its own user and campaign for the new-company page.)

The service-level counts of `TestQueryCountsOnAPreloadedWorld` (`cmd/server/querycount_test.go`; "queries for N rows" proves the count does
not grow with the rows; the budget is the test's limit):

| Call | Statements, small / large input | Budget | After |
|---|---|---|---|
| `Campaigns.Get` (campaign page) | 1 for 21 rows / 1 for 21 rows | 3 | |
| `Campaigns.Companies` (companies, rankings, hand-over) | 1 for 0 rows / 3 for 37 rows | 6 | |
| `Matches.Matches` (matches page) | 5 for 0 rows / 2 for 25 rows | 8 | |
| `Matches.Match` (match page) | 3 for 1 row / 3 for 1 row | 6 | |
| `Campaigns.CompanyDetail` (company page) | 4 for 4 rows / 4 for 24 rows | 7 | |
| `Campaigns.List` (home page) | 1 for 0 rows / 1 for 0 rows | 3 | |

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

| After | Mean ms of the same statements | Calls per page |
|---|---|---|
| | | |

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

## D. Build and test

### Build

| Measure | Baseline | After |
|---|---|---|
| `go build ./cmd/server`, empty build cache, modules cached (3 builds: 8.4, 5.9, 7.1 s) | 7.1 s | |
| `go build`, nothing changed | 0.20 s | |
| `go build` after a change in `internal/core` | 1.3 s | |
| Binary, default flags (darwin/arm64) | 31.2 MB | |
| Binary, `-s -w` (darwin/arm64) | 22.3 MB | |
| Binary as the `Dockerfile` builds it (linux/arm64, `-trimpath -s -w`) | 21.7 MB | |
| `docker build --no-cache` (distroless runtime, stylesheet stage included) | 26.6 s | |
| Image size (`docker images`) | 26.1 MB | |

### Go tests

| Run | Total | After |
|---|---|---|
| `go test -count=1 ./...` without a database (median of 3; packages run side by side) | 30.5 s | |
| `make test` with the compose PostgreSQL up (PostgreSQL tests run; also docs links, group check and 61 Python tests in 6.6 s), median of 3 | 139.9 s | |
| `make test` with an empty build cache, 1 run | 141.3 s | |
| `go test -p 1 -count=1 ./...` with a database (`make test-integration`), 1 run | 191 s | |

Per package (seconds; the test binary's own time):

| Package | No database (median of 3) | `make test` (median of 3) | `-p 1` with a database (1 run) | After (no database) | After (`make test`) |
|---|---|---|---|---|---|
| `cmd/rulecheck` | 1.7 | 1.9 | 1.6 | | |
| `cmd/server` | 1.1 | 132.3 | 118.5 | | |
| `internal/core` | 29.7 | 33.5 | 29.1 | | |
| `internal/core/lean` | 0.8 | 0.8 | 0.4 | | |
| `internal/core/ttshelper` | 0.6 | 0.9 | 0.4 | | |
| `internal/postgres` | 1.3 | 21.1 | 10.7 | | |
| `internal/web` | 28.9 | 31.9 | 28.0 | | |

Slowest single tests: with a database `TestLiveActionsSurviveExtremeValues` 108.7 s, `TestMalformedInputNeverCrashesAHandler` 67.3 s,
`TestSeedWorld` 63.0 s, `TestPermissionMatrix` 51.2 s (they run side by side in `cmd/server`), `TestHostileUploadsAnswer4xx` 7.7 s;
without a database `TestSnapshot` 4.1 s, `TestCustomRulesetWho` 3.0 s, `TestStartingCompanyPointsPlusGoldAre105InEveryCompany` 2.6 s,
`TestCustomRulesetCompanyAndLimits` 2.0 s in `internal/core`, and `internal/web` 0.7 s for its slowest (about 29 s there comes from many small tests).

### Browser tests (`make e2e`)

| Group | Steps | Local wall time (`make e2e GROUP=…`) | CI job, PR #12 (job / test step) | After (local) |
|---|---|---|---|---|
| core | 57 | 214 s (harness 211 s) | 358 s / 332 s | |
| pages | 12 | 245 s (243 s) | 412 s / 395 s | |
| creation | 101 | 407 s (404 s) | 575 s / 553 s | |
| rules | 78 | 297 s (294 s) | 456 s / 435 s | |
| All four, one after the other | 248 | 1,163 s | | |

CI, PR #12 (head `ae502d6`), as GitHub reported it: workflow `e2e` from 14:30:49 to 14:40:43 UTC, 594 s wall; the four group jobs run in
parallel, so the longest (creation, 575 s) sets the time; job `plan` 6 s, `pr-check` 9 s, the final gate job 2 s. A CI job spends about 15 to 20 s
on checkout, Go, Node and the Tailwind binary before the tests. The pull-request workflows do not run the Go unit tests.

## What is heaviest at the baseline

Facts the optimization passes start from (all numbers above; profile from `go test ./cmd/server -bench 'Pages/(home|matches|overview|tts$|armoury|my-company)$'
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

## Reproducing the numbers

Commands, from the repository root (the compose PostgreSQL up; `make e2e`, `make perf` and the scripts copy the cached seeded template
into a scratch database and never touch the development one):

```bash
# A. data format
python3 - <<< 'import json,gzip,sys; b=open("definitions/battle_companies/battle_companies.json","rb").read(); print(len(b), len(json.dumps(json.loads(b),separators=(",",":"),ensure_ascii=False).encode()), len(gzip.compress(b,9)))'
go test ./internal/core -run 'Magic|TemplateRule' -v
go test ./internal/core -run xxx -bench LoadRuleSet -benchmem -count 10

# B. browser flows and server time per page
make perf LABEL=after REPS=5                         # tests/e2e/out/perf-after.json
node tests/e2e/pagetimes.mjs --label after           # tests/e2e/out/pagetimes-after.json (run it three times)

# C. database (WORLD_DATABASE_URL: a scratch copy of the template database that make e2e or make perf created, bcm_e2e_tpl_<hash>)
WORLD_DATABASE_URL=postgres://bcm:bcm@localhost:5432/<scratch>?sslmode=disable \
  go test ./cmd/server -run xxx -bench Pages -benchtime 20x -benchmem -count 5
WORLD_DATABASE_URL=… go test ./cmd/server -run TestQueryCountsOnAPreloadedWorld -v -count=1
WORLD_DATABASE_URL=… PERF_SQL=1 go test ./cmd/server -run xxx -bench Pages -benchtime 1x     # statements per page

# D. build and test
go build -o /tmp/server ./cmd/server && ls -l /tmp/server
go test -count=1 ./...            # no database
make test                         # with the compose database
make e2e GROUP=core               # and pages, creation, rules
docker build --no-cache -t bcm-measure . && docker images bcm-measure
gh run view <run id> --json jobs  # CI times
```

The slowest-statement list is made by setting `session_preload_libraries = 'auto_explain'` and `auto_explain.log_min_duration = 0` (with
`log_analyze`, `log_buffers` and `log_format = 'json'`) on the scratch database with `ALTER DATABASE`, running the benchmark, and reading the
container's log; `EXPLAIN (ANALYZE, BUFFERS)` of the slowest call of each statement uses the parameters the log records.
