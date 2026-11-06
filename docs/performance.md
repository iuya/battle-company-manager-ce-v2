# Performance

How the app is measured and tuned, and the design rules that keep it fast.
Read it before changing rendering, queries or caching.

The numbers come from one machine (Apple M-series laptop, PostgreSQL 17 in Docker on the same host, headless Chrome).
They show proportions, not absolutes.
The results of the optimisation passes (headline table, what each pass changed, what did not improve) are in [issue #68](https://github.com/iuya/battle-company-manager-ce-v2/issues/68).

- [How to measure](#how-to-measure): the tools and the method
- [What to keep when changing the code](#what-to-keep-when-changing-the-code): the rules behind the backend, frontend and database
- [Reading a report](#reading-a-report)

## How to measure

| Tool | What it gives | Run |
|---|---|---|
| `make perf` | The main flows in headless Chrome at 0, 100 and 300 ms of emulated latency (CDP `Network.emulateNetworkConditions`): requests over the network and from the browser cache, serial chain, bytes, server time and database statements (from `Server-Timing`), time until the network is quiet, first contentful paint. Table on stdout, details per request in `tests/e2e/out/perf-<LABEL>.json` | `make perf LABEL=after [REPS=3] [LATENCIES=0,100,300] [ONLY=match]`; `node tests/e2e/perf.mjs --kbps 200` adds a download limit (200 KB/s, about a 1.6 Mbit/s link) |
| `PERF_TRACE=true` | The server adds `Server-Timing: app;dur=<ms to first byte>, db;dur=<ms>;desc="<statements>"` to every answer (`internal/perftrace`). Off by default; `make perf` turns it on for its own server. Do not set it in production | env var |
| `node tests/e2e/pagetimes.mjs [--label name] [--encoding br]` | Server time (`Server-Timing`), database time, statements and exact HTML, CSS and JS bytes per page over plain HTTP (20 requests after warm-up, per page; the main pages, the two glossary pages and the glossary unit cards fragment), the server's start time and memory; the tables of [issue #68](https://github.com/iuya/battle-company-manager-ce-v2/issues/68) | scratch database like `make perf`; run it three times and take the median of the medians; `--encoding br` asks for brotli like a browser on HTTPS (the "gzip" columns then hold the brotli size) |
| `node tests/e2e/rendercost.mjs [--label name] [--reps 7] [--latency 300] [--cold] [--cpu 4]` | What Chrome spends on the big pages once the bytes are there (CDP style, layout, script and task time, elements, long tasks), first and largest contentful paint and the layout shift score; `--cold` disables the cache (a first visit), `--cpu 4` slows the CPU (a phone), `SHOW_SHIFTS=1` prints what moved. The armoury and the glossary also get their search, clear, switch and open interactions | scratch database like `make perf` (or `PERF_BASE_URL` of a running app); `tests/e2e/out/rendercost-<LABEL>.json` |
| `BenchmarkLoadRuleSet` (`internal/core`) | Cost of building a rule set from the three shipped files and of the Battle Companies decoder alone | `go test ./internal/core -run xxx -bench LoadRuleSet -benchmem -count 10` |
| `BenchmarkPages` (`cmd/server/pagebench_test.go`) | The whole server-side cost of the main pages, without browser or network, with statements per request; take a CPU profile with it | `WORLD_DATABASE_URL=<scratch copy of the loaded world> go test ./cmd/server -run xxx -bench Pages -benchtime 20x -cpuprofile /tmp/cpu.out`, then `go tool pprof -top -cum` |

`make perf` uses the same scratch-database machinery as `make e2e`: a copy of the cached, seeded demo world per run, on its own port, never the development database.
The flows only read: they open pages and panels, they do not save anything.

A scratch copy for `BenchmarkPages` is `CREATE DATABASE x TEMPLATE bcm_e2e_tpl_<hash>`, using the template that `make e2e` or `make perf` created.

### Method notes

- **User and data.** The user is `eowyn@example.com` (admin for the match pages) in the campaigns "Matches: every scenario", "Companies being built" and "Level-ups waiting".
- **Runs.** The numbers are the run with the median time of three.
- **Cache.** Pages are loaded in a browsing session (the static files cached after the first page), except the rows marked "cold cache".
- **Settled.** "Settled" is the time from the navigation (or click) until no request is in flight for 300 ms, not counting those 300 ms.
  The live event stream is not counted as in flight.
- **Serial chain.** The longest run of requests where each starts after the previous one ended (cache hits excluded).
  At 300 ms each link of the chain is a visible 300 ms.
- **Latency emulation.** Chrome's emulation adds the latency per request, so it also shows what HTTP/1.1's six connections per host cost (the server speaks HTTP/1.1 only).

## What to keep when changing the code

These rules come out of the optimisation passes (kept in [issue #68](https://github.com/iuya/battle-company-manager-ce-v2/issues/68)).
The tests are the safety net: rules and screens are the same as before the passes.

### Backend: what to keep (`internal/core`)

The backend pass cut the server time and the memory allocated per request of the main pages.
`BenchmarkPages` on the demo world (130 companies), median of 5, ms per request and MB allocated per request, before and after.
The full tables with the baseline are in [the report linked from issue #68](https://github.com/iuya/battle-company-manager-ce-v2/issues/68), "Backend pass".

| Page | Before | After |
|---|---|---|
| campaign list | 91 ms, 101 MB | 27 ms, 7.4 MB |
| matches tab | 95 ms, 99 MB | 25 ms, 7.2 MB |
| campaign overview | 63 ms, 66 MB | 19 ms, 4.9 MB |
| my company | 43 ms, 40 MB | 20 ms, 5.7 MB |
| armoury | 57 ms, 46 MB | 23 ms, 9.5 MB |
| Tabletop Simulator dialog | 42 ms, 40 MB | 13 ms, 3.7 MB |
| `core.LoadRuleSet` (three shipped files) | 66 ms, 34 MB, 484k allocations | 49 ms, 26 MB, 314k allocations |

What to keep when changing the code:

- **An item is built once per rule set.** `equipmentFor` returns the classes, stats, description, grants and icon of a gear name for a company.
  It is the hottest function of every page that builds a company, because it is called for every item of every unit.
  Without a cache each call scans the armoury, normalises names and rewrites the description with a regular expression.
  Its answer depends only on the rule set, the company's armoury, the name and two flags, so `RuleSet.equipment` remembers it (bounded at 20,000 entries).
  The result's slices are shared: do not write to them.
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
- **Compressing the static files is not part of the start.** `buildStaticFiles` reads and hashes the 52 embedded files (7 ms) and sets up the
  gzip and brotli copies as once-values; `web.WarmStatic` makes the gzip copies in the background once the server listens (55 ms of CPU, 54 MB
  allocated, `BenchmarkStaticGzip`) and a request that comes first makes the one it needs. The brotli copies stay on demand: level 11 for the
  whole set is 2 s of CPU and 1.7 GB allocated, for browsers on HTTPS only.
- **`encoding/json` against `encoding/json/v2`** (`BenchmarkJSON`, `internal/core/bench_json_test.go`, Go 1.27, median of 6). In Go 1.27
  `encoding/json` is a layer over v2, so the v1 API and v2 with `DefaultOptionsV1` run the same engine and measure the same (vanilla file
  decode 4.97 and 4.89 ms, glossary 0.72 and 0.72 ms). v2 with its own defaults decodes 20 to 25 % faster (vanilla 3.9 ms, glossary 0.54 ms,
  the Battle Companies file into a tree 1.7 against 3.0 ms) but is a different language: case-sensitive field names, an error on a repeated
  key, strict UTF-8, other `omitempty`, and the lean decoder's error positions and texts come from the v1 token API. The whole gain is
  1 to 2 ms once per rule set (about 2 to 4 % of a load) and nothing on a request: the Tabletop Simulator payload (6 KB) encodes in 8 µs either
  way and the event, settings and session JSON in 2 µs. No call site was switched.
- **A start parses only what it has to check.** `Rulesets.RegisterDefaults` runs at every start; the vanilla book and the glossary the shipped
  Battle Companies file is checked against are read only when that file is not stored yet (`prepareWith` takes them as a function), so a
  restart on a known database does not parse them (`BenchmarkRegisterDefaults`, `go test ./internal/core -run xxx -bench RegisterDefaults`).
  The rule sets themselves are built on the first request that needs them, not at the start.
- **Not in the profile:** the live hub, the session cache and goroutine use never showed. What is left in a request is PostgreSQL
  round trips (`kevent`, system calls and thread wake-ups, about half of the CPU samples, waiting included), garbage collection (about 30 %, mostly the roster decode in
  `internal/postgres` and the template buffers) and template execution.

- **The start builds one template set per page, not two.** `newRenderer` parses the layouts and partials once (`base`, never executed) and
  clones it for every page. The set of a page for a viewer in another time zone (`pageIn`, `location.go`) is cloned and parsed on the first
  request from that zone and kept, instead of a second, never executed copy of every page being built at the start (`BenchmarkNewRenderer`,
  `go test ./internal/web -run xxx -bench NewRenderer -benchmem`). Parsing the pages side by side did not help: `Clone` holds the base's lock.
  Nine tenths of the allocation left is `Template.Clone` copying the partials' parse trees for each page. The first request from another
  zone costs about 0.5 ms more for the biggest page (`BenchmarkZoneSet`).

Two tools for comparing builds: `PAGES_DUMP=<dir>` makes `BenchmarkPages` write the body of every benchmarked page, and of the overview, matches
and armoury page of every campaign, to `<dir>` (77 files); two builds are equal when `diff -r` of the two directories (static `?v=` hashes aside)
is empty. `go test ./internal/core -run xxx -bench JSON -benchmem -count 6` repeats the JSON comparison.


### Frontend: what to keep (`internal/web`)

Numbers and the rejected ideas are in [the report linked from issue #68](https://github.com/iuya/battle-company-manager-ce-v2/issues/68), "Frontend pass". What to keep when changing the code:

- **A first visit must not move.** The heading font is preloaded (`base.gohtml`) under the same versioned address the stylesheet uses, so it is there before the first paint and the headings keep their height (layout shift 0.38 to 0.02). A new font face used above the fold needs the same line; a page that draws a state before Alpine starts must draw the state Alpine will leave (the armoury hides what no unit can take by CSS, `data-armory-pending`, until `lists.js` starts it; the "N hidden" line is drawn with its count). Check with `rendercost.mjs --cold --latency 300` and `SHOW_SHIFTS=1`.
- **A redirect costs a round trip inside a click.** htmx follows a 303 invisibly but the browser still pays it (300 ms at 300 ms of latency). The My Company and Armory tabs answer a boosted click (`HX-Boosted`, not a history restore) with the page and `HX-Push-Url`; the whole-page request keeps the redirect (`boostedLink`). Do the same for any address that only redirects.
- **A closed window does not fetch on load.** The To Wound chart is fetched when the pointer, a touch or the focus reaches a button that opens it, or at the click (`towound.js`, `hx-trigger="tw-load once"`). Content in a dialog that nobody may open should not be a request of every page view.
- **Hidden panels are templates.** The armoury's per-item unit lists are `<template x-if="open">`: Alpine builds them when "Buy for" opens, and `x-init="htmx.process($el)"` wires their forms (htmx does not watch the DOM, it processes what it swaps). Elements 5,116 to 1,760. Use the same for any list of forms that is hidden until a click and repeated per row.
- **Off-screen glossary terms are not laid out.** `.term:not([data-unit])` has `content-visibility: auto` (`tailwind.css`), so the 1,245 articles of the glossary cost style and layout only near the screen (load 123 to 24 ms of style and layout, 4x CPU 517 to 92 ms; search, clear and chip clicks about halved). An article that is not laid out has no `innerText`: code that reads one (the hover preview in `lists.js`) sets `style.contentVisibility = 'visible'` around the read, and the unit cards (read by tests and tools) are left out. Keep that pairing for any new reader of a term's text.
- **What is not worth it here**, measured: minifying our scripts (about 4 KB gzip once per release), `@source not` for the vendor scripts (0.3 % of the CSS), the sidebar's Alpine bindings (1.8 ms), whitespace stripping (nothing after gzip), gzip level 9 (5 % of a page for more CPU), `content-visibility` on the armoury (22 ms of layout and style in all).
- **Open items.** The repeated tooltip texts (`data-tip-text`) are 26 % of my company's HTML and 44 % of its gzip: a per-page table of the distinct texts or brotli would take 30.9 KB to about 22 KB.


### Database: what to keep (`internal/postgres`, `internal/core`)

Numbers, the rejected ideas and the method are in [the report linked from issue #68](https://github.com/iuya/battle-company-manager-ce-v2/issues/68), "Database pass". What to keep when changing the code:

- **Statements are prepared once per connection.** pgx caches the prepared statement of each connection, so the 1.7 to 4.5 ms of planning that `EXPLAIN` shows for the multi-sub-select statements is paid once per connection and statement, not per request. `BenchmarkPages` at 300 requests per page gives the same time with `plan_cache_mode=force_generic_plan` in the URL as without it; do not set it (a generic plan can pick the wrong index on a skewed table, and there is nothing to win).
- **A read-only GET opens a read scope.** `readOnly` (`internal/web/session.go`) wraps the GET pages and cards in `server.go`; the services behind them share the campaign, company, match, Against the Odds record, live snapshot, match history, rosters, whether the game has started (`ScopedATO.GameStarted`), and a company's perks and aftermath history (`ScopedAftermaths`) they read (`core.WithReadScope`). Cards built side by side share a load in flight. A new GET handler that only reads gets the wrapper; a handler that writes never does (a write does not invalidate the scope). The scope is why my company went from 16 to 9 statements and the match page from 37 to 12.
- **A repository method that checks that a match belongs to the campaign does it in the statement that reads what it needs** (`atoStore.Get`, `GameStarted`): never `Matches().Get` for a yes or no.
- **Rosters are read as typed rows.** `ReadRoster` selects the unit columns and the three equipment lists (one lateral probe of `unit_equipment`) instead of building a JSON document per unit; the four jsonb columns of a unit are decoded only when they hold more than their default (`{"v": 1}`). `Read` (one company) still builds its units as JSON.
- **Rate only what a page shows.** `companyRepository` may also have `ReadRosterOf(ids)` (found by type assertion, like `Nav`). `Campaigns.List` reads all the player's companies of all campaigns in one statement; `Get`, the match page and the Tabletop Simulator dialog read the ids they rate. A page that lists the whole campaign (`Companies`, the matches tab) still calls `ReadRoster`, which the other readers of the request share.
- **A company save writes the rows that changed.** `Update` fingerprints the loaded state (`rowsOf`: the company row, every unit with its equipment, every grave, built by the same functions that write them) and `save` skips the rows whose fingerprint is unchanged; without this, a sheet save would cost an UPDATE, a DELETE and an INSERT per item for every unit. A new column of `company_units` goes into `unitRowOf` and is then in the fingerprint without further work. Rows that are not rewritten keep their stored form (a company never saved keeps `pending = {"v": 1}`, the same state as `{"v":1,"recruit":{...null}}` when decoded).
- **Pool.** Without `pool_max_conns` in `DATABASE_URL` the pool holds at least 10 connections (`minPoolConns`); pgxpool's default is the number of CPUs, at least 4. With 160 match pages requested at once, 4 connections took 2.8 ms a page, 10 took 2.0 and 20 took 1.8 (`BenchmarkPages/match-parallel`, `&pool_max_conns=N` on `WORLD_DATABASE_URL`). The other pool settings are the URL's (see [architecture.md](architecture.md#configuration)).
- **Indexes.** Every statement the pages and the live mode run has an index path (`SET enable_seqscan = off` and `EXPLAIN` of the 32 distinct statements of a `BenchmarkPages` run shows no sequential scan outside the harness's own lookup of a campaign by name). The demo world is small, so the planner scans the 40-row tables anyway; nothing was added. `matches_campaign_idx (campaign_id, number DESC)` is covered by the unique `(campaign_id, number)`, but dropping it saves nothing measurable and was left.

#### The match page's stream

A full initial batch on the event stream would make the match page fetch the end card and the log a second time.
That costs three requests and 30 statements, and a third round trip (1.0 s at 300 ms of latency).
The page therefore proves it is current instead.

How it works:

1. `handleMatch` reads a generation before it builds the cards.
   `LiveService.LiveGeneration` returns the match revision and the live log sequence as `"<revision>.<seq>"`.
2. The generation goes on the log slot (`data-live-since`), and `live.js` opens `/events?since=<generation>`.
3. `LiveService.SubscribeSince` registers the stream first and reads the state after.
   A write in between reaches the stream as an event or shows in the state read.
4. The stream sends the full initial batch only when the state moved.
   A reconnect with an old value, or a change between render and subscribe, still refetches everything.

Every notification of the live feed comes from a write that raises the revision or the sequence:
`matchStore.Update` and `atoStore.Update` raise the revision; `liveStore.Update` raises the sequence.

A page without a generation (the company page, or a page whose generation could not be read) starts with the full batch.

Anything that changes what the cards show must raise the revision or the sequence, or notify.
A new kind of change that does neither would not reach a page that has the generation.

#### Test run time

The demo-world tests of `cmd/server` wait for a seed of the world: 99,000 statements, 38 s, almost all of it round trips (`synchronous_commit=off` did not help). Writing only the changed rows halves the statements (45,000) and the seed (22 s). `TestLiveActionsSurviveExtremeValues` 109 to 63 s, `TestMalformedInputNeverCrashesAHandler` 67 to 37 s, `TestSeedWorld` 63 to 32 s, `TestPermissionMatrix` 51 to 29 s, the package 118 to 71 s.

Tools: `PERF_SQL=1` shows the end of each statement, `PERF_STACK=<text>` prints who runs a statement that contains the text, `BenchmarkPages/match-parallel`, `internal/postgres` `BenchmarkDecodeUnits` and `BenchmarkReadRoster` (`WORLD_DATABASE_URL`).

## Reading a report

- `Chain` counts links, not requests: parallel requests are one link.
- `+N cached` are answers from the browser cache (no round trip).
- A flow row for a click starts at the click.
- FCP (first contentful paint) is from the page's navigation, so for a click it is the old page's value.
- Statements and server time are those of requests the flow made, so a page that made no request for the sidebar shows fewer than its document would at a first load.
