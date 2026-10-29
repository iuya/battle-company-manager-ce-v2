# Performance

Measured findings of the optimisation pass (epic `6ej`), shared by the frontend, backend and database passes. Rules and screens did not
change; the tests are the safety net. Numbers come from one machine (Apple M-series laptop, PostgreSQL 17 in Docker on the same host, Chrome
headless) and show proportions, not absolutes.

## How to measure

| Tool | What it gives | Run |
|---|---|---|
| `make perf` | The main flows in headless Chrome at 0, 100 and 300 ms of emulated latency (CDP `Network.emulateNetworkConditions`): requests over the network and from the browser cache, serial chain, bytes, server time and database statements (from `Server-Timing`), time until the network is quiet, first contentful paint. Table on stdout, details per request in `tests/e2e/out/perf-<LABEL>.json` | `make perf LABEL=after [REPS=3] [LATENCIES=0,100,300] [ONLY=match]`; `node tests/e2e/perf.mjs --kbps 200` adds a download limit (200 KB/s, about a 1.6 Mbit/s link) |
| `PERF_TRACE=true` | The server adds `Server-Timing: app;dur=<ms to first byte>, db;dur=<ms>;desc="<statements>"` to every answer (`internal/perftrace`). Off by default; `make perf` turns it on for its own server. Do not set it in production | env var |
| `node tests/e2e/pagetimes.mjs [--label name]` | Server time (`Server-Timing`), database time, statements and exact HTML, CSS and JS bytes per page over plain HTTP (20 requests after warm-up, per page), the server's start time and memory; the tables of [optimization-report.md](optimization-report.md) | scratch database like `make perf`; run it three times and take the median of the medians |
| `BenchmarkLoadRuleSet` (`internal/core`) | Cost of building a rule set from the three shipped files and of the Battle Companies decoder alone | `go test ./internal/core -run xxx -bench LoadRuleSet -benchmem -count 10` |
| `BenchmarkPages` (`cmd/server/pagebench_test.go`) | The whole server-side cost of the main pages, without browser or network, with statements per request; take a CPU profile with it | `WORLD_DATABASE_URL=<scratch copy of the loaded world> go test ./cmd/server -run xxx -bench Pages -benchtime 20x -cpuprofile /tmp/cpu.out`, then `go tool pprof -top -cum` |

`make perf` uses the same scratch-database machinery as `make e2e` (a copy of the cached, seeded demo world per run, own port; never the
development database). The flows only read: they open pages and panels, they do not save anything. A scratch copy for
`BenchmarkPages` is `CREATE DATABASE x TEMPLATE bcm_e2e_tpl_<hash>` of the template `make e2e` or `make perf` created.

Method notes. The user is `eowyn@example.com` (admin for the match pages) in the campaigns "Matches: every scenario", "Companies being
built" and "Level-ups waiting"; the numbers are the run with the median time of three. Pages are loaded in a browsing session (the static
files cached after the first page) except the rows marked "cold cache". "Settled" is the time from the navigation (or click) until no request
is in flight for 300 ms, not counting those 300 ms; the live event stream is not counted as in flight. The "serial chain" is the longest run of
requests where each starts after the previous one ended (cache hits excluded); at 300 ms each link of the chain is a visible 300 ms. Chrome's
emulation adds the latency per request, so it also shows what HTTP/1.1's six connections per host cost (the server speaks HTTP/1.1 only).

## Baseline (before the pass)

Every page was 21 static requests, all uncompressed and all revalidated in full (the embedded files carry no ETag or date, so `no-cache` meant
"download again"), plus the sidebar's own request after the page:

| Flow | Requests | Chain | KB on the wire | Settled at 0 ms | 100 ms | 300 ms | 300 ms, 200 KB/s | Server ms | DB statements |
|---|---|---|---|---|---|---|---|---|---|
| login (GET, POST, redirect, home) | 47 | 8 | 881 | 471 | 1543 | 3813 | 6723 | 361 | 182 |
| home | 23 | 4 | 440 | 398 | 791 | 1884 | 3347 | 369 | 178 |
| campaign overview | 23 | 4 | 438 | 495 | 962 | 1886 | 3346 | 459 | 117 |
| my company | 24 | 4 | 653 | 339 | 792 | 1907 | 4190 | 274 | 174 |
| recruit window (click, 1 request) | 1 | 1 | 5 | 140 | 138 | 355 | 390 | 109 | 73 |
| level-up window (click, 1 request) | 1 | 1 | 12 | 113 | 139 | 350 | 419 | 82 | 43 |
| armoury | 23 | 4 | 828 | 1179 | 1618 | 2556 | 5533 | 1117 | 148 |
| matches list | 23 | 4 | 449 | 719 | 1186 | 2087 | 3570 | 692 | 124 |
| match page (7 htmx cards and the stream) | 30 | 5 | 490 | 1064 | 1497 | 2316 | 3853 | 2127 | 418 |
| live match (company page, stream) | 25 | 4 | 646 | 452 | 968 | 1968 | 4302 | 385 | 124 |
| aftermath | 24 | 4 | 475 | 260 | 805 | 1912 | 3562 | 228 | 145 |

Server ms and statements are the sums over the page's document and htmx requests. A live match opens no polling request (0 extra requests in
3 s), the page waits on the event stream.

## What was changed (frontend), one change at a time

| # | Change | Measured effect |
|---|---|---|
| 1 | **Static files**: a content hash in every address (`?v=`, `{{static "..."}}`), served `immutable` for a year; ETag and 304 for addresses without it; the stylesheet's fonts and images carry their hash too (`internal/web/static.go`) | home at 300 ms: 1884 to 647 ms; requests 23 to 2 on a warm page; 440 KB to 36 KB |
| 2 | **gzip** for static text (ahead of time) and for pages and fragments over 1 KB (`compress.go`, level 5) | the company page 226 KB to 25 KB, the armoury 424 KB to 28 KB; at 200 KB/s my company 3587 to 536 ms (with 1); about 0.2 to 1 ms of CPU per page |
| 3 | **One script bundle** (`js/app.js`: htmx, theme, page scripts, Sortable, Alpine) instead of 14 files | first visit (cache disabled), home at 300 ms: 1888 to 1272 ms; requests 23 to 10; login flow 2599 to 1966 ms |
| 4 | **The main page draws the sidebar's campaign list from its own data** instead of fetching `/nav/campaigns` (a second `List`, 89 statements, 150 ms) | home at 300 ms: 647 to 365 ms; 2 requests to 1; 178 to 89 statements |

Tried and dropped (no measurable gain): a `<link rel=preload>` for the heading font (the font arrives 300 ms earlier on a first visit, but the page
was not waiting for it: the settle time and first contentful paint did not move).

Looked at and left alone, with the evidence:

- **Template cost**: executing the templates is about 5 % of a page's server time (3 ms of 65 ms for my company, `go tool pprof` of `BenchmarkPages`); it is not worth changing.
- **HTML size**: the company page is 214 KB because every glossary link repeats its tooltip text (`data-tip-text`), the armoury 417 KB (every item lists every unit). After gzip the repetition costs almost nothing (25 and 28 KB), and the browser's parse and layout time did not show in the load times; a tooltip map per page would need changes in JavaScript and templates for no measured gain.
- **htmx navigation** already returns the page's content block only: one request, 2 to 20 KB, 150 to 370 ms at 300 ms of latency.
- **The initial sync of the live stream** makes the match page fetch the end card, the end checks and the log a second time (`LiveHub.subscribe` marks everything changed on purpose: nothing is lost between rendering the page and opening the stream). It is after the first paint and the second answers are the same cards; removing it would reopen that race, so it stays.

## After

Same flows, same method (three runs each, median):

| Flow | Requests over the network | Chain | KB on the wire | Settled at 0 ms | 100 ms | 300 ms | 300 ms, 200 KB/s |
|---|---|---|---|---|---|---|---|
| login | 47 to 11 | 8 to 5 | 881 to 153 | 471 to 327 | 1543 to 711 | 3813 to 1673 | 6723 to 2449 |
| home | 23 to 1 | 4 to 1 | 440 to 7 | 398 to 223 | 791 to 253 | 1884 to 354 | 3347 to 386 |
| home, first visit (cache disabled) | 23 to 9 | 4 to 4 | 440 to 146 | 406 to 263 | 853 to 541 | 1884 to 1281 | 3333 to 2001 |
| campaign overview | 23 to 2 | 4 to 2 | 438 to 8 | 495 to 485 | 962 to 473 | 1886 to 661 | 3346 to 688 |
| my company | 24 to 3 | 4 to 2 | 653 to 48 | 339 to 338 | 792 to 327 | 1907 to 691 | 4190 to 795 |
| recruit window (click) | 1 to 1 | 1 | 5 to 2 | 140 to 136 | 138 to 144 | 355 to 369 | 390 to 369 |
| level-up window (click) | 1 to 1 | 1 | 12 to 4 | 113 to 123 | 139 to 148 | 350 to 357 | 419 to 387 |
| armoury | 23 to 2 | 4 to 2 | 828 to 28 | 1179 to 1181 | 1618 to 1173 | 2556 to 1308 | 5533 to 1458 |
| matches list | 23 to 2 | 4 to 2 | 449 to 8 | 719 to 712 | 1186 to 720 | 2087 to 852 | 3570 to 891 |
| match page | 30 to 9 | 5 to 3 | 490 to 18 | 1064 to 1059 | 1497 to 1060 | 2316 to 1037 | 3853 to 1098 |
| live match | 25 to 3 | 4 to 2 | 646 to 27 | 452 to 451 | 968 to 554 | 1968 to 740 | 4302 to 888 |
| aftermath | 24 to 2 | 4 to 2 | 475 to 8 | 260 to 276 | 805 to 389 | 1912 to 664 | 3562 to 685 |

What is left at 300 ms is one document round trip, one request for the sidebar on pages other than the main page (second link of the chain),
and the server's own time: at 0 ms a page takes what the server needs, and that is now the whole cost of most pages.

## Where the time is now (for the backend and the database passes)

`BenchmarkPages` on the demo world (warm rule sets, one request, includes template and gzip; statements per request counted by the pgx tracer):

| Page | ms per request | Statements | Of it, in `core` rules code |
|---|---|---|---|
| home | 152 | 89 | `Campaigns.List` builds a full company detail per campaign (`BuildDetail`, 29 %) |
| `/nav/campaigns` (sidebar, on every page but the main one) | 145 | 89 | the same `List`; the sidebar uses only id, name and status |
| campaign overview | 254 | 28 | `Matches.Matches` and its `rater` build every company's detail (`BuildDetail` 35 %) |
| my company | 65 | 85 | `BuildDetail` 25 % |
| armoury | 909 | 59 | `ArmouryService.ArmouryEntries`, 85 % |
| matches list | 485 | 35 | `BuildDetail` 36 % |
| match page | 279 | 68 | `Matches.Match`, `complete`, `rater`, `BuildDetail` 33 % |
| live company page | 327 | 123 | the same |
| aftermath | 184 | 144 | the same |

The profile of nearly every page has one shape: `core.BuildDetail` / `buildUnit` / `equipmentFor` call `BCBook.Profile(s)` for every unit, which calls
`ProgressionRules.PowerChoices` and `PowerOffer.Label` (`fmt.Sprintf`), again and again with the same arguments; GC takes a third of the CPU.
The armoury does the same inside `shop.whyNot` / `legalWargear` / `heroOptions` for every item and every unit (items times units). The
statement counts (`List` 89, the aftermath page 144, the live page 123, the match page 68 plus 71 for the ATO card and 63 for the log) are
per-company and per-unit loops. The requests they come from, by page, are in beads `6ej.1` (perf-backend) and `6ej.2` (perf-database); the quick check
after a change is `BenchmarkPages` (time and statements per request) and `make perf` (what a user sees).

Asked of the next passes, in order of what a user waits for:

1. **perf-backend**: `Profile`/`PowerChoices`/`PowerOffer.Label` memoised per rule set (the result depends only on the rule set and the unit's type and path), and `BuildDetail` not called for every company when a page needs only the ranking figures (`Matches.rater`, `Campaigns.List`). Armoury: compute each item's reasons once per unit type, not once per item and unit.
2. **perf-backend / perf-database**: a one-statement `NavCampaigns(user, withFinished)` returning id, name and status. With it the sidebar can be drawn inside every page (it is now a second request, 89 statements and about 150 ms of server time, whose answer fills the sidebar one round trip after the page), as the main page already does.
3. **perf-database**: `Campaigns.List` (89 statements for a player with 20 campaigns), the aftermath page (144), the live page (123), the ATO card (71 statements, 629 ms on the first load), `live/log?card=1` (63 statements, 357 ms), `Match` (68). The cheap fix is one batched query per page instead of one per company or unit.

## Backend pass (`internal/core`)

`BenchmarkPages`, same machine and world, `-benchtime 20x`, ms per request (allocations per request in brackets). Each change was measured on its own; the profile before them was one third GC and, in `core`, three repeated computations:

| Page | Before | 1. profiles once | 2. regexps compiled once | 3. `ItemKey`/`IconFor` remembered | After (statements) |
|---|---|---|---|---|---|
| home | 149 (1.78 M) | 123 | 63 | 47 | 42 (89) |
| `/nav/campaigns` | 152 | 105 | 66 | 63 | **3.9** (9; 1 once the repository has `Nav`) |
| campaign overview | 254 (4.39 M) | 168 | 84 | 34 | 33 (28) |
| my company | 71 | 61 | 45 | 42 | 46 (85) |
| armoury | **943** (21.3 M) | 89 | 55 | 43 | 43 (59) |
| matches list | 502 (8.6 M) | 311 | 157 | 66 | 58 (35) |
| match page | 297 | 181 | 96 | 47 | 47 (68) |
| live company page | 363 | 224 | 119 | 71 | 75 (123) |
| aftermath | 192 | 121 | 94 | 58 | 57 (144) |

1. **`BCBook.Profile(s)` built once** (`companyprofile.go`). `Profiles()` cloned every profile and rebuilt the magical-power list (`PowerChoices`, a `Sprintf` per power) on each call, and `Profile(name)` called it twice, for every unit of every page and for every item x unit of the armoury. The book is immutable after loading, so the set is built on first use (`sync.Once`) and `Profile` is a map lookup (exact name, then lower-cased: the older spelling still resolves). The armoury went from 943 to 89 ms; no query changed.
2. **`hasWord` compiled a regexp per word per call** (`IconFor` tries 28 icon rules for every item name): the compiled patterns are cached per word (`sync.Map`). Home 123 to 63 ms.
3. **`ItemKey` and the name part of `IconFor` are pure functions of the item name**, called per item x unit x comparison: remembered in a bounded table (`memo.go`, 20 000 entries; past that it computes without storing, so text from players cannot grow it). Matches list 157 to 66 ms.
4. **`NavCampaigns(ctx, userID, withFinished) []core.CampaignNav`** (id, name, status) is in the web `campaigns` interface and drives `/nav/campaigns`: no company is rated, no rules are loaded. `core.Campaigns.NavCampaigns` asks the repository for a `Nav(ctx, userID, withFinished) ([]CampaignNav, error)` when it has one (one statement, to be added in `internal/postgres`, bead `6ej.2`) and otherwise maps `campaignRepository.List` (still its N+1 company reads, but no rules code): 152 to 3.9 ms.

What is left in `core` is small: in the profile of the matches page `BuildDetail` is 10 % of the CPU, GC and waiting for PostgreSQL the rest, so the remaining server time follows the statement counts (about 0.4 ms per round trip on this machine): `Campaigns.List` 89, aftermath 144, live page 123, ATO card 71, live log card 63, match 68 are for `6ej.2`. A ranking-figures-only path next to `BuildDetail` for `Campaigns.List`/`rater` was not built: the figures need the built units (gear, injuries, creatures), so it would duplicate rule code for about 10 ms.

Left alone: the live hub, session cache and goroutine use did not show in any profile; `ParameterisedKeyword`'s regexp replace is 3 % of a page.

## Backend pass at v2.0.0 (`internal/core`)

`BenchmarkPages` on the demo world (130 companies), median of 5, ms per request, MB allocated per request. The full tables with the baseline
are in [optimization-report.md](optimization-report.md), "Backend pass".

| Page | Before | After |
|---|---|---|
| campaign list | 91 ms, 101 MB | 27 ms, 7.4 MB |
| matches tab | 95 ms, 99 MB | 25 ms, 7.2 MB |
| campaign overview | 63 ms, 66 MB | 19 ms, 4.9 MB |
| my company | 43 ms, 40 MB | 20 ms, 5.7 MB |
| armoury | 57 ms, 46 MB | 23 ms, 9.5 MB |
| Tabletop Simulator dialog | 42 ms, 40 MB | 13 ms, 3.7 MB |
| `core.LoadRuleSet` (three shipped files) | 66 ms, 34 MB, 484k allocations | 49 ms, 26 MB, 314k allocations |

What the code does now, and what to keep when changing it:

- **An item is built once per rule set.** `equipmentFor` (the classes, stats, description, grants and icon of a gear name for a company) is the
  hottest function of every page that builds a company: it ran for every item of every unit, each time scanning the armoury, normalising names
  and rewriting the description with a regular expression. Its answer depends on the rule set, the company's armoury, the name and two flags only,
  so `RuleSet.equipment` remembers it (bounded at 20,000 entries). The result's slices are shared: do not write to them.
- **Rating a company is not building its page.** `companyFigures` returns what `BuildDetail(...).CampaignCompany` returns (points, full points,
  models, faction label) from the built units, without keywords, auras, limits, recruit charts and cemetery. The campaign list, the overview and
  the match pages rate every company of a campaign from one read of the rosters; `TestCompanyFiguresEqualBuildDetail` keeps the two equal. A
  change to what feeds a company's points must go in `buildUnit`, which both use.
- **Pure helpers allocate nothing in the common case.** `NameKey` has an ASCII path on a stack buffer and returns the string itself when it is
  a key already; the equipment list and the stat block are sized once; a glossary term remembers its `Summary(n)` per length.
- **Loading a rule set decodes each file once.** The vanilla file is decoded one time and builds the two books `loadV2` needs (to check the
  Battle Companies file against, and with the duplicates it pins); a vanilla name is encoded for comparison only when it occurs twice; a field
  the entry does not have is not decoded (a missing key makes the decoder build and rewrite an error); the glossary sorts by keys worked
  out once per term; the lean binder builds a field's path only for the fields present.
- **`encoding/json` against `encoding/json/v2`** (`BenchmarkJSON`, `internal/core/bench_json_test.go`, Go 1.27, median of 6). In Go 1.27
  `encoding/json` is a layer over v2, so the v1 API and v2 with `DefaultOptionsV1` run the same engine and measure the same (vanilla file
  decode 4.97 and 4.89 ms, glossary 0.72 and 0.72 ms). v2 with its own defaults decodes 20 to 25 % faster (vanilla 3.9 ms, glossary 0.54 ms,
  the Battle Companies file into a tree 1.7 against 3.0 ms) but is a different language: case-sensitive field names, an error on a repeated
  key, strict UTF-8, other `omitempty`, and the lean decoder's error positions and texts come from the v1 token API. The whole gain is
  1 to 2 ms once per rule set (about 2 to 4 % of a load) and nothing on a request: the Tabletop Simulator payload (6 KB) encodes in 8 µs either
  way and the event, settings and session JSON in 2 µs. No call site was switched.
- **Not in the profile any more:** the live hub, the session cache and goroutine use never showed. What is left in a request is PostgreSQL
  round trips (`kevent`, system calls and thread wake-ups, about half of the CPU samples, waiting included), garbage collection (about 30 %, mostly the roster decode in
  `internal/postgres` and the template buffers) and template execution.

Two tools for the next pass: `PAGES_DUMP=<dir>` makes `BenchmarkPages` write the body of every benchmarked page, and of the overview, matches
and armoury page of every campaign, to `<dir>` (77 files); two builds are equal when `diff -r` of the two directories (static `?v=` hashes aside)
is empty. `go test ./internal/core -run xxx -bench JSON -benchmem -count 6` repeats the JSON comparison.

## Sidebar list on every full page (follow-up)

`pageBase` now carries the sidebar's campaign list on every full page for a signed-in user (`Server.base` calls `NavCampaigns`; the main page keeps using its own list; htmx content swaps skip it; on error the sidebar falls back to the lazy `/nav/campaigns` fetch). The `/nav/campaigns` round trip is gone from every other page. `make perf REPS=3`, label `nav-before` / `nav-after` (`tests/e2e/out/`), requests and settled time (ms) at 0 / 100 / 300 ms. The server ms and statement columns also moved because the database pass ran at the same time; only requests and settled are this change's.

| Flow | Requests before to after | Chain | Settled @ 0 / 100 / 300 before | after |
|---|---|---|---|---|
| campaign overview | 2 to 1 | 2 to 1 | 114 / 247 / 658 | 56 / 134 / 349 |
| my company | 3 to 1 (doc, xhr, static) | 3 to 1 | 159 / 264 / 682 | 68 / 156 / 393 |
| armoury | 2 to 1 | 2 to 1 | 180 / 284 / 703 | 93 / 172 / 414 |
| matches list | 2 to 1 | 2 to 1 | 142 / 250 / 647 | 92 / 141 / 374 |
| aftermath | 2 to 1 | 2 to 1 | 126 / 267 / 631 | 69 / 137 / 371 |
| live match | 3 to 2 | 2 to 1 | 209 / 294 / 663 | 112 / 171 / 400 |

At 300 ms every full page except the match page is one round trip (about 300 ms saved). The match page keeps its chain of 3 (its own lazy cards); see "Match page cards" below.

## Match page cards (lazy or inline)

Why they were lazy: not because most users do not need them. All four (Against the Odds, secret objectives, end checks, log) are on screen on every visit to a match; they were built as separate `hx-get` slots because the card endpoints already existed for refreshing them (`live:match`, saves) and because at the time they were slow (the ATO card 71 statements and 629 ms, the log 63 statements), which would have held back the whole page. The database pass made them cheap (ATO 13 statements, log 12, end checks 7).

Now the full page (`handleMatch`, `loadMatchCards` in `internal/web/match_cards.go`) builds the four in parallel goroutines and draws them in its own answer. A card that fails to build stays a lazy slot (`hx-trigger="load"`, the old behaviour); the slots keep their `live:match` refresh and the log slot keeps `data-live-match`, so the stream and the answers to writes (`match_body`, which stays lazy) are unchanged. The SSE initial sync stays: `subscribe` still marks everything changed and the page refetches the end checks and the log once after the stream opens (match-level notices carry no sequence number, so the page cannot prove it is current without a `core` change; skipping it would reopen the render-to-subscribe race).

`make perf ONLY=match REPS=3`, labels `lazy-before` / `lazy-after`, demo world:

| Match page | before | after |
|---|---|---|
| requests (doc/xhr/sse) | 8 (1/6/1) | 5 (1/3/1) |
| serial chain | 3 | 2 for the cards (the initial-sync refetch remains) |
| statements | 74 | 70 (the log no longer reloads the match) |
| server ms, all requests | 380 | 255 |
| settled ms @ 0 / 100 / 300 | 245 / 364 / 986 | 276 / 396 / 978 |
| first contentful paint @ 300 | 384 | 352 |

The cards are in the first answer instead of one round trip later (at 300 ms about 340 ms earlier on screen); the "settled" figure does not move because the initial-sync refetch, gated by the stream's own round trip, finishes at the same time as before. Two requests fewer per card set, one fewer table of work per page. A further step needs `core`: a sequence number on match-level notices (end proposal, ATO save), then the page could skip the initial refetch when it is current.

## Database pass (`internal/postgres`)

`BenchmarkPages` (`-benchtime 20x`, scratch copy of the demo world, same machine), ms per request and statements per request, before this pass (after the backend pass) and after. The ms columns also contain the concurrent frontend change (the sidebar list is drawn into every page), so read the statements as this pass's effect and the time as the combined state; the pages are CPU-bound in `core` and the template at these sizes, and a statement costs about 0.15 to 0.4 ms of round trip, so the time gain is smaller than the statement gain (on a database that is not on the same host it grows with the latency).

| Page | Before ms (statements) | After ms (statements) |
|---|---|---|
| home | 42.3 (89) | 19.5 (9) |
| `/nav/campaigns` | 3.9 (9) | 0.7 (1) |
| campaign overview | 31.8 (28) | 28.8 (5) |
| my company | 37.4 (85) | 18.1 (16) |
| armoury | 41.4 (59) | 28.1 (10) |
| matches list | 57.1 (35) | 53.5 (7) |
| match page | 44.8 (68) | 33.0 (12) |
| ATO card `/matches/{m}/ato` | (71) | 54.1 (14) |
| live log card `/live/log?card=1` | (63) | 31.3 (12) |
| live company page | 67.9 (123) | 44.3 (26) |
| aftermath | 56.1 (144) | 25.5 (27) |

What was changed, each on its own, all behind the same core interfaces (no mock changed):

1. **`campaignStore.Nav`** (one statement; `core.Campaigns.NavCampaigns` picks it up by a type assertion): the sidebar no longer reads a company or loads rules. `List`, `Get`: the user's company with its figures (wins, draws, losses, unit count) is part of the same statement (`LEFT JOIN`), and `Get` joins the three rules files' info. `Get` 5 statements to 1, `List` 1 + one per company to 1.
2. **`companyStore.Read` is one statement** (company row, pending aftermaths, open match, unused perks, units with equipment, cemetery as JSON sub-selects) and runs without a transaction: a single statement is its own snapshot, so the `BEGIN`/`ROLLBACK` pair and the nine statements inside it are gone (11 round trips to 1). `ReadRoster` the same (6 to 1). `Update` (the locking read) keeps the multi-statement `load`; `TestReadEqualsLoad` compares `Read` and `ReadRoster` with `load` for every company in the database (the 107 companies of the demo world pass).
3. **`matchStore.load` is one statement** (sides, companies, end proposal, confirmations and objectives as JSON sub-selects; the locking variant first locks the rows in a statement of its own, then reads, so it sees what the lock holder left). `Match` was loaded four to eight times per page by the services and each load was 6 statements; `Events` no longer loads the match first (one statement that answers "no such match" too). The decoded matches of the demo world are identical to the old code's (diffed).
4. **`liveStore.Read` is one statement** (match counter, units and not-undone log as JSON; `TestLiveReadEqualsSnapshot` against the locking `snapshot`), **`aftermathStore.Get`** one statement instead of a transaction with two.
5. **`campaignStore.Hashes` is remembered per process**: a campaign's rules files and settings are written when it is created and never change, campaigns are never deleted, so the lookup (7 to 8 times a page on the aftermath and live pages) is read once per campaign (bounded at 10 000 entries).

Looked at and left alone:

- **Indexes**: `EXPLAIN (ANALYZE, BUFFERS)` of the figures, match and pending-aftermath queries shows index scans where the tables are big enough to need them (`company_units_company_id_position_key`, `match_aftermaths_company_idx`, `matches_campaign_idx`) and sequential scans on the 40-row tables of the demo world; the executions take 0.03 to 1 ms. No index was added and none is redundant. Planning, not execution, is the bigger part of the figures query (1.6 ms for 37 companies the first time); pgx prepares and caches statements per connection, so it is paid once per connection.
- **Pool**: pgxpool defaults (the larger of 4 and the number of CPUs); the limits can be set in `DATABASE_URL` (`?pool_max_conns=N`, `pool_min_conns`, `pool_max_conn_lifetime`) without code. No pool setting moved a measurement.
- **Transactions**: all remaining read transactions are in the Update paths, which need them.
- What was left in the statement count (the aftermath page loading the match five times and the company three times, each one statement) is request-level reuse, not a query shape: see the read scope below.

### Aftermath page: request-level reuse (backend, after the database pass)

The aftermath page asked for the same rows from several services (match 7 times, company 6, Against the Odds record 3, live snapshot 2, campaign 2). `core.WithReadScope(ctx)` (`internal/core/readscope.go`) opens a per-request memo; the repositories wired in `cmd/server/backend.go` are wrapped (`ScopeMatches`, `ScopeCompanies`, `ScopeATO`, `ScopeLive`, `ScopeCampaigns`: `Get`/`Read`/`Company` only) and answer a repeated read from it. It is opt-in: a context without a scope reads straight through, so no write path changes. `handleAftermath` (a read only request) opens the scope; nothing that writes does, because a write would not invalidate the memo. Results are shared, so callers must not change the slices and maps in them (the rating of a match already clones before it changes).

`BenchmarkPages/aftermath`, scratch copy of the demo world, `-benchtime 30x`, three runs each: 27 to 14 statements, 33 to 46 ms (median 41) to 26.5 ms. The rendered HTML of all eleven benchmarked pages is byte identical before and after. The other pages are unchanged (match 12, live 26 statements). The htmx actions (`/sheet`, `/start`, ...) were measured too (48 statements for a sheet save, before and after) and are left as they were: their reads happen before and after the write and are not repeated within a step, the bulk is the save itself.

Left for the database layer: `atoStore.Get` loads the whole match to check that it belongs to the campaign (a `SELECT true FROM matches WHERE id AND campaign_id` would do), and `companyStore.save` writes each unit with an UPDATE, DELETE and INSERT (a sheet save is about 35 write statements).

Tools: `PERF_SQL=1 go test ./cmd/server -run xxx -bench Pages -benchtime 1x` prints every statement of a page, most frequent first (`BenchmarkPages` also has the ATO and log cards now). The budgets of `TestQueryCountsOnAPreloadedWorld` (`cmd/server/querycount_test.go`) were tightened to the new counts: Get 3, Companies 6, Matches 8, Match 6, CompanyDetail 7, List 3 (all without growing per row).

## Reading a report

`Chain` counts links, not requests: parallel requests are one link. `+N cached` are answers from the browser cache (no round trip). A flow row
for a click starts at the click. FCP is from the page's navigation, so for a click it is the old page's value. Statements and server time are
those of requests the flow made, so a page that made no request for the sidebar shows fewer than its document would at a first load.
