# Testing

How the project is tested, for contributors who write or run tests: the commands, the Go unit and integration tests, the golden snapshots of the rules data, the Go checks of the definitions tooling, and the headless-browser smoke tests (`make e2e`).

Contents:

- [Commands](#commands)
- [Tabletop Simulator helper](#tabletop-simulator-helper-lua-tests)
- [Definitions tooling](#definitions-tooling)
- [Golden snapshot of the Battle Companies rules](#golden-snapshot-of-the-battle-companies-rules)
- [The lean decoder](#the-lean-decoder)
- [Custom rulesets and the magic-number lint](#custom-rulesets-and-the-magic-number-lint)
- [Browser smoke tests](#browser-smoke-tests)

## Commands

| Command | What it runs |
|---|---|
| `make lint` | Static analysis, failing on any finding: `go vet`, `staticcheck` (default checks) and `deadcode -test` (unreachable functions; tests count as users). The tools are pinned in the `tool` block of `go.mod` and run with `go tool`, no global install. A deliberate exception is a `//lint:ignore <check> <reason>` comment. Runs in `make test`, in the `pre-push` hook and in the `pr-check` workflow; the Docker image build does not run it |
| `make hooks-test` | The hooks of `.githooks/` in a throwaway repository, including the pre-push refusal when `make lint` fails |
| `make test` | `make lint`, then Go unit tests (the PostgreSQL ones are skipped), including the tests of the programs under `tools/` (below) |
| `make test-integration` | All Go tests against the `bcm_test` database of the compose PostgreSQL. In `cmd/server` the demo world is seeded once per run into a template database (`testmain_test.go`) and the two fuzz tests run side by side on copies of it; `go test ./cmd/server` takes about 2 minutes |
| `make docs-check` | Every relative link of the Markdown files (README, CONTRIBUTING, `docs/`, `tools/`, `examples/`) resolves, headings in `#fragments` included (`tests/ci/links.mjs`; part of `make test`) |
| `make validate` | The four rules files: JSON Schemas, the Go validator on the Battle Companies file, the curator's checks, the glossary and the concept graph (below) |
| `make check-ruleset FILE=…` | Checks a lean rules file like the upload does ([definitions.md](definitions.md#upload-validation)) |
| `make fuzz [FUZZTIME=1m]` | Explores `FuzzParseDice` and `FuzzDecode` (their seed corpus already runs in `make test`) |
| `make e2e` | Browser smoke tests (this page); `make e2e-groups` runs them group by group, `make e2e-check-groups` (part of `make test`) checks the grouping and the dice loops of the flows |
| `make perf` | Page-load measurements at 0/100/300 ms of latency, not a test: see [performance.md](performance.md) |

## Tabletop Simulator helper (Lua tests)

`internal/core/ttshelper` runs the helper script (`helper.lua`) inside `go test` with gopher-lua, a test-only dependency.
The script runs against a fake Tabletop Simulator API and a simulation of the army spawner's grid; the fixtures are in `tests/tts/`.

It needs no game and no Lua binary, and runs with `make test`.
The real game is only exercised by playing ([tabletop-simulator.md](tabletop-simulator.md#what-remains-to-verify)).

## Definitions tooling

The programs under `tools/` ([tools/README.md](../tools/README.md)) are Go, and their tests run with the rest of `go test ./...` (`make test`); `go test ./tools/...` runs only these.
The tests read the shipped files, so they also prove the shipped files.
`tools/validate` calls the app's own validator in-process, so they need only `go`.

| Package | What it proves |
|---|---|
| `tools/validate` | `make validate` itself: 0 errors on the shipped files, exit 1 on a dangling unit name, an unknown key, a missing key, a wrong schema version, a stale graph, a glossary defect |
| `tools/internal/leancheck` | Company texts (repeated rules, parse artefacts), aura rings (vocabulary tables, group follows `when`/radius, radius written in the text, the war horn's sum), rule-target keywords, the Tabletop Simulator models against the local catalogue |
| `tools/internal/conceptgraph` | The shipped concept graph is current and what the build writes (deterministic), and each graph check fails on its own corruption (dangling edge, stale hash, wrong direction, missing `via`...) |
| `tools/internal/glossaryref` | The To Wound Chart of the glossary (complete grid, allowed values, monotonic), the term list, and that the committed glossary is what the build writes |
| `tools/internal/ttsmodels` | The Tabletop Simulator saved-game extractor and the catalogue format the validator reads |
| `tools/internal/rulesref` | The generated rules-file reference is current and leaves no key out |
| `tools/leanfixtures` | The committed lean fixtures are what the generator writes, deterministically |
| `tools/themecontrast`, `tools/icons` | The WCAG ratio and the shipped palettes; the trace parser and the icon markup |

`make validate` is the same checks on the shipped files without the corruption tests; `TestShippedConceptGraphMatchesCore` (Go) checks the generated graph file against core.
Where Application Control blocks the executables `go run` and `go test` build, see [collaborating.md](collaborating.md#7-working-on-windows).

## Golden snapshot of the Battle Companies rules

`go test ./internal/core -run Snapshot` runs `internal/core/snapshot_test.go`, which freezes everything the app derives from the Battle Companies file, as text in `internal/core/testdata/snapshot/` (about 2 MB).
There is one `<company>.txt` per company, plus `_index`, `_armoury`, `_progression`, `_campaign`, `_ruletargets`, `_tooltips` and `_offersets`.

The test fails on any difference and shows the first differing lines.
It reads only what core exposes (BCBook, CompanyProfile, `AuditRules`, `BuildDetail`, the progression and campaign tables), never the JSON, so a change of the file format must leave the goldens untouched.
The few unexported helpers it needs are in its "seams" block.

After an intended change, `go test ./internal/core -run Snapshot -update-snapshot` rewrites the files.
Review the diff in its own commit.

| File | Content |
|---|---|
| `<company>.txt` | anomalies; the whole profile (start, charts, rules, effects, aliases, hero upgrades, limits, oath, notes); starting gold, points and the totals of every allowed officer choice; the company's unique wargear (limits, `Only`, ratings); every unit (stats, keywords, options with reserved/unsold/read-as, built warrior and hero, what each can buy with gold, `via` and refusals); who may take each hero upgrade; every promotion (match as warrior and hero for each gear combination, choices, gear kept or dropped); tooltips (digest) and glossary terms; the name-resolution map (each unit, gear and rule the company mentions and what it resolves to) |
| `_armoury.txt` | the common armoury with its ratings, gold, restrictions, grants and gear annotations (asserted equal for every company) |
| `_progression.txt` | level-up tables, warrior table, per path the offers for each 2D6 result of a fresh hero and the stat limits (`improvedStat`) |
| `_campaign.txt` | injuries, aftermath (experience, influence, Against the Odds bonuses, charts, effects, scenario rewards), scenarios, campaign options, Against the Odds, break point, hero rating, creatures, wanderers, armour upgrade |
| `_ruletargets.txt` | the rule-target table and, for every rule in use, who it works against expanded over the keywords of every unit |
| `_tooltips.txt`, `_offersets.txt` | texts the company files cite by digest (tooltips, the equipment and rule refusals shared by many units) |

## The lean decoder

The package `internal/core/lean` is pure, so most tests need no mock.
`Lower` (what vanilla and the glossary know) has a hand-written fake for the worked examples (`fakelower_test.go`) and a generated mock (`make generate`) where a test must see the calls.
Tests are table-driven.

- **Fixtures.** `testdata/valid/*.json` are complete minimal files built from the worked examples of the format; each decodes with no
  issue, in the lean package against the fake world and in `internal/core` against the shipped vanilla file and glossary. `testdata/invalid/<case>.json`
  come with `<case>.want`, the exact report text, one per row of the catalogue of validation messages. `go run ./tools/leanfixtures` writes the inputs; `go test ./internal/core/lean -run TestInvalidFixtures -update` rewrites the `.want`
  files, and the diff is read: the wording is reviewed there.
- **Cross-file checks.** `internal/core/rulesets_checkset_test.go`: three different custom files (a vanilla file renaming a unit, a glossary with an
  extra rule, a Battle Companies file using both) pass together through `Rulesets.CheckSet` and `StoreAllAs`, and fail against the shipped
  vanilla file or glossary; the report is one list grouped by file; garbage (binary, truncated, 100,000 levels of nesting, 5 MB strings) is an
  issue, never a panic (`TestCheckSetNeverPanicsOnGarbage`, `go test ./internal/core -fuzz FuzzCheckSet`). `internal/web/validator_test.go` tests the
  online validator against the generated mock: no `Store*` call, the 16 MB limit, 6 checks a minute per account and 2 at once (429).
- **Upload error catalogue.** `internal/core/rulesets_catalogue_test.go` drives every invalid fixture through `Rulesets.Check` against the real
  vanilla file and glossary and compares the full rendered report with `internal/core/testdata/catalogue/<case>.want`
  (`go test ./internal/core -run TestUploadErrorCatalogue -update-catalogue`); a file with twelve mistakes returns twelve issues, a file with
  thousands is capped at 1,000 and counted.
- **Fuzz.** `FuzzDecode` (no panic, no hang beyond 2 s, the same bytes give the same report, a clean document re-encoded and decoded is equal) and
  `FuzzParseDice`. The seeds are every fixture and the shipped file; they run in `make test`, `make fuzz` explores.
- **Goldens as the proof of what the file means.** The file-based goldens freeze what the app derives from the shipped file: `internal/core/testdata/snapshot` (the audit and the pages' data for all 37 companies; `go test ./internal/core -run Snapshot -update-snapshot`), `auras_golden.txt` and `tts_golden.txt`, plus file-level checks in `ruleset_v2_test.go`
  (`TestShippedFileIsVersionTwo`: the shipped file decodes with no issue but the tolerated `I shall pass!` warning) and `TestCustomRuleset*` (one changed parameter of the
  shipped file changes the behaviour, or is refused with the decoder's message).
- **Benchmark.** `go test ./internal/core/lean -run xxx -bench DecodeLarge`: a 650 KB file decodes in about 25 ms (budget 100 ms); a decode happens
  once per content hash.

## Custom rulesets and the magic-number lint

`customRuleset(t, mutate)` (`customruleset_test.go`) loads the shipped vanilla, Battle Companies and glossary files.
It lets `mutate` change the decoded Battle Companies document (a `map[string]any`) and builds a `RuleSet` with `LoadRuleSet`, the path production uses.

`TestCustomRuleset` is table-driven, one row per engine parameter ([customizing-rulesets-reference.md](customizing-rulesets-reference.md)).
`mutate` sets the value and `check` asserts the behaviour through the built rule set.
The table holds:

- a default row;
- the treatment `perPoint` of the "attempt" method;
- the recruiting parameters;
- the dice: injury dice d12, Courage dice and bonus dice, plus `TestCustomRulesetRefusesDiceTheTablesDoNotCover` for dice a table does not cover.

Related files:

- `dice_test.go` tests `Dice.Roll`, `FixedDice`, `CheckCovers`, the typed-roll folding, `ParseScenarioSelection` and the hero roll with another number of results.
- `customruleset_company_test.go` holds the company rows.
  `TestCustomRulesetCompanyAndLimits` covers officers, hero start, `maxModels`, XP threshold, the hero rating threshold, `limits[]`, creature, wanderer and path limits, and the Courage and Intelligence cap.
  `TestCustomRulesetRefusesBadCompanyAndLimitParameters` covers the path-naming load errors.
- `treatment_test.go` holds the treatment dice.
- `internal/web/rules_numbers_test.go` tests the web templates that print these numbers (the officer places, the experience bar, the treatment window).

`magicnumbers_test.go` holds two report-only checks (they log, never fail; run `go test ./internal/core -run 'Magic|TemplateRule' -v`):

- `TestMagicNumbersReport` parses the non-test `internal/core/*.go` with `go/ast`, counts integer literals other than 0, 1 and 2 (set
  `magicAllowedValues`) and not in `magicAllowList`, logs the total, the top files and values, and lists declarations of the engine constants
  the engine takes from the data (`magicForbiddenNames`).
- `TestTemplateRuleLiteralsReport` greps `internal/web/templates` for `up to N` and `ND6` and logs the count per file.

Allow-list format:

- `magicAllowList["file.go:value"] = "reason"` allows that literal anywhere in the file.
- `magicAllowList["file.go:Func:value"] = "reason"` allows it inside one function.

The reason is mandatory in review, and the list should shrink.
To make the lint enforcing, set `magicNumbersEnforce = true`, clear or justify every remaining literal, and delete the empty rows of the report.

## Browser smoke tests

`make e2e` runs the browser smoke tests. Alpine, htmx swaps, the live event stream, dice and dialogs are only exercised by a real browser.
The suite in `tests/e2e` drives headless Chrome with [puppeteer-core](https://pptr.dev/) through the demo world.
It is a separate Node project (its own `package.json` and `package-lock.json`, `puppeteer-core` pinned); nothing of it is in the Go module.

### Setup (once)

- macOS, Linux or Windows ([collaborating.md](collaborating.md#7-working-on-windows)). CI runs the suite on Linux.
- Node 20+ and a Chrome or Chromium, found in the usual places, on Windows the Google Chrome install folders (set `CHROME_PATH` otherwise).
- `make e2e-install` runs `npm ci` in `tests/e2e`; `make e2e` does it when `node_modules` is missing.
- The compose PostgreSQL must run; the target starts it (`make db-up`).

### Run

- `make e2e` takes about 6 minutes.
- `make e2e-enforce` is the same with the server in `CSP_MODE=enforce`, which is what CI runs.
  A Content-Security-Policy violation of any page, browser event or `/csp-report` log line fails the step or the run (flow `90-csp`).
- `make e2e ONLY=match` runs the flow files whose name contains `match`.

Output goes to the terminal, one line per step (`ok` or `FAIL` with the reason).
On a failure, `tests/e2e/out/` gets a full-page screenshot (`<step>.<n>-<who>.png`) and the page text (`.txt`) for every signed-in browser session, plus `server.log`.
`out/` is gitignored and emptied at each run.

### Isolation

The run never touches the development database or `:8080`:

- It builds the server to `tests/e2e/out/bcm-e2e-server-<pid>` (one binary per run, removed at the end).
- The demo world takes minutes to seed, so it is seeded once into a template database `bcm_e2e_tpl_<hash>` (the hash, `templateKey()`
  in `lib/env.mjs`, covers `cmd/server`, `internal/` without `internal/web` and the test files, `go.mod`, `go.sum` and `definitions/`
  (the migrations are in `internal/`), because the world is written through the services of `internal/core` and the repositories of
  `internal/postgres`; so a change to a rule, a default or the schema reseeds, and a change to a page does not; `E2E_RESEED=1` forces it) and every run
  creates `bcm_e2e_run_<pid>` from it with `CREATE DATABASE ... TEMPLATE`, so each run starts from the same fresh world. The hash
  uses paths relative to the repository, so a clone or worktree shares the template. Seeding and copying the template are
  serialised across runs by a lock directory (`$TMPDIR/bcm-e2e-prepare.lock`; a run that finds it waits, a lock older than
  20 minutes is taken over), because PostgreSQL refuses `TEMPLATE` while another session uses the template.
- Several runs at once (two agents, a clone) are safe: own database, own port, own binary. They still share `tests/e2e/out/`
  (each run empties the screenshots and `server.log` of the others when it starts) and the compose PostgreSQL; from another
  checkout set `COMPOSE_PROJECT_NAME` to the project of the running PostgreSQL (`docker ps` label `com.docker.compose.project`).
- `TestSeedWorld` makes `bcm_world_test_<pid>_<n>`, unique per run, so two `make test-integration` runs do not drop each other's database.
- The server listens on a free port chosen by the OS (`APP_ENV=development`, `CSP_MODE=report` or `E2E_CSP_MODE`, `STYLEGUIDE=true`).
- At the end (also on Ctrl-C) it stops only the processes it started (by pid, no `pkill`) and drops its own database.
  `make e2e-clean` drops the cached templates (`bcm_e2e_*`) and `out/`.
- `E2E_BASE_URL=http://127.0.0.1:PORT node run.mjs` uses a server you started yourself (scratch data only: the flows write).
  `E2E_ADMIN_URL` (a PostgreSQL URL with rights to create databases) replaces the `docker compose exec psql` calls;
  `E2E_DB_URL_BASE` is the base URL the server gets (default `postgres://bcm:bcm@localhost:5432`).

### Failures that pass on a rerun

A match flow is a chain, so one failing step makes the steps after it fail too. Read the first failure.

The harness helps in four ways:

1. It waits for the clicked box to hold still.
2. It clicks again once when a button with `hx-get` or `hx-post` sent no request, and prints `(click ... sent no request, clicking again)` and a count in the summary.
3. It writes into every failure dump the last 60 requests of the page with times, the live dot, `data-seq` and the open sheets, and
   the page's events: a renderer crash, the page closing, the main frame's navigations and a lost WebGL context (`webglcontextlost`).
   A dump is written even when the page is gone (no screenshot then).
4. It warms the server once before the first step (`Harness.warm`: `/login` and every static file, including the lazily loaded
   `dice3d.js` and the fonts), so the first-use caches are not inside a timed step; sessions wait up to 30 s for a navigation,
   and a Chrome that fails to start is started once more.

htmx wires a swapped-in fragment only when it settles, 20 ms after the swap.
`static/js/htmx_ready.js` processes the fragment at once, and the step "a menu entry clicked the instant it is swapped in" in `40-match` holds that.

How to read a failure:

- A re-click count in a green run, or a failure dump that shows the request missing, is evidence of a real bug in the sheet.
- A first-run failure that is green on rerun on a loaded machine is load, not a regression.

### What the flows cover

The flows are in `tests/e2e/flows` and run in file order.
Every step also fails on any browser console error, uncaught exception, failed request or 4xx/5xx answer it did not expect.

| File | Steps |
|---|---|
| `10-accounts` | login refused and accepted, logout and the redirect; account time zone saved; glossary cross reference scrolls to its term; admin pages load and a plain user is refused |
| `20-campaign` | organiser creates a campaign; invitation link; a second player joins by it; both take command (confirmation dialog); an officer chooses a path; Manage campaign: pause, resume, end with the typed name (Continue stays disabled until it matches) |
| `30-company` | recruit (a typed roll, confirm, influence and units change); armoury purchase with gold; a hero's level-up; Change formation by dragging a card with the mouse, saved and kept after a reload, then Restore default order |
| `40-match` | create a match, pick a scenario; live mode with two players and the organiser watching: tracker change seen by the viewer through the event stream and Undo; the stream cut by request interception ("Reconnecting", the page catches up when it is back) and refused with a 429 (the page polls every 10 s); mount removed (seen by the viewer); wound an enemy unit; retreat and return; log; the End match sheet, the other player confirms; aftermath start, injury roll and Finish for both companies |
| `50-ranger` | the Ranger of the North swap on a recruit result in the "Cemetery" campaign |
| `60-pages` | 21 page types (login, home, campaign pages, a match, a live company, an aftermath, glossary, account, admin, style guide) at 360, 768 and 1280 px, light and dark: no horizontal scroll, the colour scheme really switches, no console errors or failed requests |
| `60-pages` (end) | axe-core 4.10 (pinned) on the same pages at 1280 and 360 px, light and dark: no serious or critical violation of WCAG 2.x A/AA; `AXE_OFF` in the file lists the rules left out, each with its reason |
| `61-card-variants` | the redesigned unit cards in the looks `60-pages` does not run: four looks (light, high contrast; dark, high contrast; light, gear as icon tiles; dark, high contrast and icon tiles; set by the cookies `bcm-contrast` and `bcm-gear`, checked on `<html>` after each load) on seven pages: the Veterans demo (heroes at twenty levels: its match page, both live company pages with the Might/Will/Fate bars, hearts and milestones, its rankings), an Elven company (Aftermath ward 1, Galadriel's My Company) and a company with injured units (Injuries and treatment, Éowyn's My Company); at 360, 768 and 1280 px, no horizontal scroll and no console error; axe-core (serious and critical) at 360 and 1280 px in each look and at 768 px in the first; and the pages together held leader/sergeant/hero insignia, live trackers, an injured unit, experience bars and Elven gear. A first step checks the company locked by a waiting aftermath: every control inside a `[data-lock]` element is `aria-disabled="true"`, a click, Enter and Space do nothing, and the attribute survives an htmx swap; axe runs on the whole page with no exclusion |
| `70-keyboard` | the keyboard walk: Tab reaches the controls of the sign-in form and a company page with a visible focus ring; a sheet opens with Enter, keeps the focus inside, closes with Escape and gives the focus back |
| `79-keywords` | the unit keyword chips ([unit-keywords.md](unit-keywords.md)): the chip row of My Company (in the bottom corner of the card, below the experience) without the keywords the icon, insignia and gear already say, the `+N` that opens the full list in place, the expanded card, a chip's tooltip and helper popup, the armoury's wanderers and creatures, a fallen unit, another player's read-only page, 360/768/1280 px in both themes, axe |
| `80-creation` | company creation: the Shire's Ranger of the North starts as the Leader, locked (real mouse drags out of the place and onto it, the Leader button, no Remove) and a forged POST without him gets a 422 with "must start as the Leader"; Rohan's officers moved with real drags between the places and the pool (the form's hidden fields follow, the suggestion banner goes, the vow dialog names the officers); the officer panel at 360/768/1280 px, light and dark |
| `81-aftermath-ui` | the aftermath sheet's milestones and outcome toggles (inside their card, hero stats only, no horizontal scroll at 3 widths x 2 themes), one press saves a milestone, every unit's results below the casualties and, once the sheet is locked, the Survived / Casualty / Fled badges: three different computed colour pairs, contrast of at least 4.5 (read from the computed styles, `color-mix()` normalised through a canvas) |
| `82-company-ui` | the Elven Cloak badge's tooltip on My Company (Stalk Unseen, To Hit) at 3 widths x 2 themes; renaming a unit (the 3 rem kind icon, the pencil focusing a field across the title plate with no horizontal scroll at 3 widths x 2 themes, Enter saves, Escape gives up and reopens on the saved name, an emptied name saved by a click elsewhere is the unit type); the gear style of Appearance ("Icon and name" by default with a 22 px icon, "Icons only" draws 36 px tiles with a 28 px icon and keeps the name for screen readers, cookie `bcm-gear` kept after a reload, no horizontal scroll at 3 widths x 2 themes, `out/gear-tiles-*.png`); the buy window's "Find an item" by name, by alias and "No item matches your search."; the armoury's hide switch (on by default, remembered after a reload), main category chips disabling the item chips that do not apply, creature and wanderer tooltips |
| `83-fallback` | the Warrior-to-Hero fallback of "Aftermath ward 3" of the demo world: one Warrior ahead in Experience (offered alone, chosen), two tied (both listed, none chosen, answer required), a Leader fallen with both Sergeants; no "nobody" option; the chosen Warrior's path is refused until the aftermath is finished, then taken |
| `84-match-extras` | a three-company match (Éowyn's cavalry and Gimli's Dwarves against Sauron) on Fog of War, which has no secret objectives card and ends with the winner and the MVP alone; the Rohan live cards (mounts, the granted Arise rule, losing one mount strikes only that horse); an MVP tie with two opposing votes ("Tied: ... picks in the aftermath"), broken by a third vote and restored; the aftermath's tie hint |
| `85-creation-submit` | a throwaway campaign of its own (the organiser creates it, two players join by the invitation link, so the demo world is untouched): a Shire company is sworn (the Ranger of the North leads, the vow dialog names the officers, redirect to My company, one Leader and two Sergeants as suggested, every unit with its points) and a Rohan company with officers arranged by real mouse drags is created exactly as dragged; both are on Companies and Rankings with the sum of their units' points; a second submit is refused with a 422 ("already have a company"), nothing is added, and the creation page sends the member to My company |
| `86-dice3d` | the dice animation, with real WebGL (headless Chrome through SwiftShader, flags in `lib/harness.mjs`): a server roll (the campaign where the server rolls every die) loads `dice3d.js` only then, rolls on the 3D tray, ends, reports the number the server sent (`data-roll`) and shows it; the result, raise buttons and Confirm are hidden while the canvas rolls and appear when it stops (sampled every frame), the recruit can then be finished; with `getContext('webgl')` answering null the flat tray takes over and the result still shows; with `dice3d.js` blocked the flat tray is used; the style guide tray stops on given values (1, 2, 4 dice, two lanes) and can be rolled again, a browser roll is shaken once with the space bar, `prefers-reduced-motion` gives the result at once; a D3 is a 3D cube marked 1, 1, 2, 2, 3, 3 (style guide tray: given values and a live shake); the Lesson Learned D3 of an aftermath is thrown in the browser where the table shakes (ward 4) and rolled by the server, acted out, with its bonus hidden until it stops, where it does not (ward 5, which also refuses a thrown die); the tray fits 360, 768 and 1280 px, light and dark |
| `115-server-dice` | the dice script: the endpoint refuses a request without its token and queues, reads and clears; the scenario roll on the new match form and a recruit roll show exactly the numbers queued |
| `87-concepts` | the helper popup: a chip on My Company opens it (hover keeps the tooltip, which steps aside), its groups and "Open in glossary", a chip swaps the content and Back returns, Escape closes and the focus returns to the term, backdrop and Close; the shield's explicit cancels fold into closed lines with a count; a vanished concept says "no longer in these rules", an unknown name falls back to the tooltip text; keyboard (Enter opens, Tab and Shift+Tab stay inside, the heading takes the focus after a step); a glossary link in an expanded card opens it over the card and Escape closes only the popup; the live company page's terms open it; the sheet at 360/768/1280 px in both themes (bottom sheet at 360, card from 768, inside the window, no horizontal scroll, axe on the popup) with screenshots `out/concept-popup-*.png` |
| `88-own-path` | units On Their Own Path and promoted officers: the overview text of the "everything on" campaign, a Sentinel as a warrior with XP and Level Up, a typed 6 that makes it a hero on the Path of Their Own Making with no path window, re-roll or forgo offer (360/768/1280, both themes, no horizontal scroll), the Take command badges following the campaign setting, and Arnor's officers previewed, confirmed and rostered as Rangers of the North (a throwaway campaign) |
| `89-ruleset-report` | a rules file with twelve mistakes is refused with every problem listed (file, path, hint, closest match) by "Check this file" (a dry run), by the admin upload and by the new-campaign form; nothing is stored and no campaign is created; 360/768/1280 px, light and dark |
| `90-csp` | the Content-Security-Policy header of the mode the server runs in, and a control that proves the harness sees a violation (otherwise "no violation" in the other flows would prove nothing); `make e2e-enforce` runs the whole suite under the enforcing policy |
| `91-tts` | the copy button and its dialog (Tabletop Simulator army list): button next to the name with an accessible name and a tooltip on hover and keyboard focus, the list of a Rohan cavalry company (line format, hero line, counts, Break Point, notes), Escape and the focus return, the Tab trap, Copy into the clipboard (and the select-all fallback), the two switches on a Warg company (htmx, no page load, remembered in localStorage), another player's company (wanderers and creatures as stand-ins), a stranger's 404, the live variant (a retreated unit goes to a second warband with a note), 360/768/1280 px in both themes with axe and screenshots (`out/tts-*.png`) |
| `92-tts-names` | the names block of the Tabletop Simulator dialog: valid ArmyNames JSON with one entry per figure matching the list lines in order, the rule-text switch (none < short <= full, htmx, remembered, survives a list switch), Copy names (clipboard and select fallback, own live region), `/tts/helper.zip` fetched in the page (200, `application/zip`, ETag, `PK` magic, a `.json` and a `.png` entry by their central directory sizes), the collapsed Lua disclosure with its script and copy button, 360/768/1280 px in both themes with axe and screenshots (`out/tts-names-*.png`); the live fallen marker is checked in `91-tts` |
| `93-to-wound` | the To Wound chart window: the match page's button opens a 10 x 10 chart with seven well-known cells of the printed chart (S3 vs D3 4+, S4 vs D6 5+, S5 vs D3 3+, S1 vs D1 4+, S2 vs D7 6+/4+, S1 vs D9 -, S9 vs D10 5+), Escape and the focus return, the window outside `#match`; the find control (typed values, steppers, clamping, a tap on a cell, row/column/cell marked); on the live company page the window outside `#live-zones` stays open with its pick after a live refresh and finding a roll sends no request; the Tab trap, Close and the backdrop; 360/768/1280 px in both themes on both pages: no page scroll, sticky headers, six distinct band colours with contrast of at least 4.5, axe and screenshots (`out/to-wound-*.png`) |
| `106-armoury-possible` | the armoury lists every purchase a company could make ([frontend-company.md](frontend-company.md), "The Armory tab", GitHub issue 57), on Aragorn's fresh Arnor company of "Companies being built": Armour and Heavy Armour are listed greyed ("No unit can take it now", `aria-disabled`, described by the reason that names the Ranger of the North and "after Armour"), no Warg, Fell Warg or halberd; Tab reaches the greyed button and Enter opens every unit's reason with no buy form; the switch brings the impossible ones back and is remembered; 360/768/1280 px x light, dark and high contrast: no horizontal scroll, the button is greyed with tokens (opacity 1), axe on the armoury list (`#armory-items`), a screenshot of the Heavy Armour card in `out/armoury-possible-*.png` |
| `107-organiser-awards-own` | the organiser "you", who also plays the Fiefdoms in the demo campaign "Aura parade", sees the Grant reward form on the Grants card at the bottom of their own My Company tab (GitHub issue 60), awards 4 Gold with a note and sees it in the history and the purse; 360/768/1280 px x light, dark and high contrast: no horizontal scroll, axe on `#grants` |
| `108-item-glossary-links` | item descriptions link the glossary ([frontend-company.md](frontend-company.md), "Item descriptions are linked text", GitHub issue 62), on Aragorn's Arnor company of "Companies being built": in the Armory tab Mountain Boots link Mountain Dweller with a tooltip, the War spear links lance and spear, the Company Drum links war drum, the Backpack does not link its own name, hovering a link shows its tooltip, and the "+" equipment window of a unit links its descriptions too |
| `110-glossary-unit-links` | the unit cards of the campaign Glossary link what they name like the company page's cards ([frontend-glossary.md](frontend-glossary.md#glossary-and-item-definitions)): searching the Goblin Mercenary shows its special rules, wargear and keywords as chips with a tooltip, hovering Chittering Hordes shows its text, a click opens the helper popup, and the card holds at 360 px in light and dark |
| `94-envenom` | Envenom Weapon (X) on the gallery's Mordor company: the Armory tab's unit rows each carry a weapon chooser built from that unit's weapons (nothing preselected, required); bought from the buy window (Orc Bow) and from the Armory tab (Spear), the card shows `Poisoned Attacks (<weapon>)` once and the weapon item stays plain; the choice is final (no chooser, no Change weapon), dropping the Spear keeps the rule and buying it again shows it once; Harad's profile-poisoned Bow is not offered; a unit that already carries it has the reason and no button; 3 widths x 2 themes: no horizontal scroll, axe clean on the Armory tab and the buy window (the "no weapon left" reason is a handler test: no demo unit reaches it) |
| `95-keyword-targets` | the demo campaign "Keyword targets" (Gimli's Dwarves with Ancient Enemies (Orc) against Radagast's Mirkwood company): a rule chip's tooltip and helper popup say "Works against: enemy models with the Orc keyword." and the own card is not marked; on the opponent's page every card shows its keywords and exactly the Orcs carry "Ancient Enemies (Orc) applies" (tooltip, Enter opens the popup, Escape returns the focus); the reverse side, the opponent's own page and an onlooker see no marker; "Wounded a unit" lists the enemy units with keywords and the same markers; a retreating Orc loses its marker through the live refresh, its compact card keeps the chips, and the marker returns with the unit; 360/768/1280 px x light/dark: no horizontal scroll, markers inside the card, axe, screenshots |
| `96-tts-auras` | aura rings on the demo campaign "Aura parade" (Fiefdoms of Gondor as `you`): the Auras section of the Tabletop Simulator dialog (switch on by default, the five kind rows with counts and radii equal to the rings of the v2 payload, groups, totals line, legend with letters, five colours and the thicker enemy line), known figures (Company Standard 3" banner, Company Drum 12" declared, Voice of Command 12" and no 6" command ring on the Leader, Stand Fast 6" on every Hero, at most 6 rings per figure, the Leader keeps all rings with Furious Tirade, the Large rings switch is there), the "download the helper again" notice and the Rings button steps, the group and kind switches (declared, targeting ranges, Torch, a kind off) change the payload by htmx with no page load and are remembered in `bcm-tts`, switch off or no kind gives version 1 byte for byte, keyboard (Space on the switches, the focus returns after the swap), the chips hidden by default and the "Show auras on unit cards" toggle (cookie `bcm-auras`, kept after a reload, the expanded Auras section follows), the aura chips on the cards (radius, kind letter, tooltip, aria-label, muted war horn ring, `+N` in place, glossary popup on Enter and focus back on Escape, the expanded card), another player's read-only company and own company of another account, the live variant (a retreated unit has no ring, its compact card no chip row), 360/768/1280 px x light/dark with axe and screenshots (`out/tts-auras-*.png`, `out/tts-auras-chips-*.png`) |
| `97-online-validator` | the online validator at `/tools/validate`: the examples of `examples/customizing-rulesets` pass ("No problems found", nothing stored), `broken/typos.json` lists its four problems with the closest names, a missing Battle Companies file and a vanilla file that is not JSON are refused, the rules page and the new-campaign form link to it, the focus moves to the result, no campaign or rules file appears (checked on the admin page); 360/768/1280 px, light and dark |
| `99-last-campaign` | the `bcm-last` cookie ([auth.md](auth.md)): a new browser sees the list at `/`; opening a campaign makes `/` land on its My Company (cookie HttpOnly, Lax, about 90 days, unreadable by scripts); the brand link, the sidebar and "← Campaigns" reach `/campaigns`, which does not redirect; the My Company alias opens the last company opened, else the first; another user's, malformed, over-long, unknown-campaign and finished values are ignored and cleared; a finished campaign is not remembered; signing in as someone else ignores it; the pages at 360/768/1280 px in both themes without horizontal scroll |
| `98-several-companies` | several companies per player ([multi-company.md](multi-company.md)): a throwaway campaign whose Companies per player starts at 1 (a player with one company sees no chip and no new wording, the bare `/my-company` is an alias, another player's company is 404 on every owner address, a second company is refused), the organiser raises the limit, a second company is created through the Overview and the chip switcher appears and switches in My Company and in the Armory, lowering the limit keeps both companies and refuses a third; in "Two bands" the chips for Éowyn (with "New company") and Galadriel (at the limit), none for Gimli, a company of another campaign is 404, the "Same player" note on the match page and as the new match form is built, the viewer's own companies marked and listed first, no Mine chip; 360/768/1280 px in both themes: no horizontal scroll, axe |
| `100-practice-match` | practice matches ([practice-matches.md](practice-matches.md)) in "Two bands": the Matches list badges (no Practice filter) and the Overview counts, an ended practice match (banner, "nothing to roll" on the match and aftermath pages, every aftermath write refused), an open one (same locked live state with the badge, another match refused), playing it through (scenario, the live match, the end sheets), no aftermath pending and the company figures unchanged, a new practice match from the form (box off by default, "Make it a practice match" link), an administrator reopening and ending it again, a viewer without actions; 360/768/1280 px in both themes: no horizontal scroll, axe |
| `101-glossary-units` | the campaign Glossary tab's unit cards ([frontend-glossary.md](frontend-glossary.md#glossary-and-item-definitions)): the page counts the units and has none of the cards, the button fetches them in one request and there is a card for every counted unit with a unique plain anchor, an address with `#u-...` fetches, scrolls to, highlights and focuses the card (and shows it after a search hid it), a search or the Units chip fetches them and filters (by name and by company; no "no term matches" notice while they load), `?c=units`, keyboard (Enter on the button and on an in-page link to a card), 360/768/1280 px x light/dark x normal/high contrast with no horizontal scroll, no overflowing card, and axe over the cards and the filter row at 360 and 1280 (light, dark, dark high contrast), and the placeholder at 360 px, with screenshots `out/glossary-units-*.png` |
| `102-unit-links` | unit names in rule texts ([frontend-glossary.md](frontend-glossary.md#unit-names-in-texts)): on the Arnor company page of "Brand new campaign" the special rule links "Ranger of Arnor" and "Ranger of the North" to `#u-…` of the campaign glossary (full load, no helper popup) while "Warrior" and "Hero" still open it; hover and keyboard focus show the profile, leaving and Escape hide it, Enter follows the link and lands on the focused card; a definition on the glossary tab links its units in the page with a tooltip before the cards are fetched and one request fetches them; the sidebar glossary has no unit link; 360/768/1280 px x light/dark x normal/high contrast: no horizontal scroll, the tooltip inside the window, axe (the light-theme `.exclusive-gold > .exclusive-label` contrast of the page is left out), screenshots `out/unit-links-*.png` |
| `103-admin-roles` | roles in the administration ([admin.md](admin.md#roles)): the administrator (`steward`) sees Users and Invitations, no Rules files, only the Organiser switch on plain users and organisers, notes on administrators and superusers; makes `pippin` an organiser and back with the keyboard (Space, Tab, Enter) and the change applies at once; forged `role=admin` or `role=superuser` posts get 403, the recovery routes 404; the superuser (`admin`) has the Administrator and Organiser switches (Organiser checked and disabled while Administrator is on), no Superuser option, makes `pippin` an administrator who then sees the section, and back; 360/768/1280 px x light/dark x normal/high contrast with no horizontal scroll and axe over the row and the sub-navigation |
| `104-invitations` | account invitations ([auth.md](auth.md#account-invitations)): a named invitation is created with the keyboard and the message is a read-only box (`?email=` in the link, no inline script, the list never shows the token); the person registers in a fresh browser (a forged `?email=` is ignored, the stored address shown read-only), lands on `/campaigns` signed in and the new password works; the link then shows "Talk with the administrator."; an anonymous invitation for two people, the third and an address that already has an account get the very same page (422); weak and mismatching passwords keep the form; revoke with the inline confirmation; the filter chips; the throttle (429); the creation card, the registration page and the generic page at 360/768/1280 px x light/dark x normal/high contrast with axe |
| `105-recovery` | recovery links ([auth.md](auth.md#password-recovery)): the demo states are worded on the Users page; the superuser makes a link for `recover-expired@` (3 days), the person sets a new password in a fresh browser (mismatch and weak password keep the form), is not signed in, the old password is refused, the new one works, the old session is gone and the link is dead; signing in first (`recover-login@`) or opening the app with a session first (`recover-access@`) invalidates the link and the row says why; revoke with confirmation; a new link replaces the old; an administrator has no card and a forged post is 404; the Users card and the set-password and generic pages at 360/768/1280 px x light/dark x normal/high contrast with axe. Never signs in as `recover-pending@` |
| `113-level-up-window` | the level-up window: a click only selects an option and Confirm applies it (a reload before Confirm leaves the level-up waiting), the marks of results that take the other too (moving a mark, both in one request, a normal option alone, Confirm with a mark in each, the Warlord 5 and 7), Courage at its limit, the dots and the (i) of a stat, no change closing by itself or staying with Next level up, a Hero in the Making with and without confetti, accepted or going on to the path window; 360, 768 and 1280 px, light and dark |
| `114-aftermath-sheet` | the Sheet step of "Aftermath ward 6" of the demo world, as a player and as the organiser: the Against the Odds block (bonuses taken read only, also those with no effect; the control for the organiser; "No bonus taken."; no block for the higher company or equal ratings), the match result line, the description and why the sheet opened (nothing recorded, the Relic with no holder, a tied MVP vote) |

### Scripting the server's dice

The rolls the server makes (a recruit, a level-up, the aftermath injury and bonus dice, the scenario roll) are random at run time. A flow that
must see a known number fixes it first. The e2e server is started with `E2E_DICE=1` and `E2E_DICE_TOKEN` (`lib/env.mjs`, which exports
`E2E_DICE_TOKEN`), and serves a queue at `/_e2e/dice`:

- `POST /_e2e/dice` with `{"next":[2,5]}` appends results; each is used by the next roll of the server whatever its sides, and must fit
  that die (1 to sides) when the roll takes it: a result that does not fit fails the roll with a 500 and is dropped. An empty queue rolls at random.
- `GET` returns `{"queued":n}`; `DELETE` empties the queue. Every answer carries the count.
- Every request needs the `X-E2E-Dice-Token` header (403 without it) and a loopback peer.

```js
await fetch(h.base + '/_e2e/dice', { method: 'POST', headers: { 'X-E2E-Dice-Token': E2E_DICE_TOKEN }, body: '{"next":[2,5]}' });
```

A scripted number is the die as the server rolls it: the scenario roll takes two (1 to the scenario die), a recruit one, a level-up the dice of its chart.
Queue only what the next action takes and check `queued` is 0 afterwards, so a number never leaks into another step. Only campaigns where the
server rolls (Shake the table off, "Settings: the server rolls every die") roll on the server; where the table is shaken the browser decides.
Flow `115-server-dice` is the example.

The guard: one `Dice` serves every service and the web server's scenario roll (`RandomDice`, at the same cost as ever). The queue exists only
when `E2E_DICE` is on and the route is registered only then: `APP_ENV=production` with `E2E_DICE` is refused at start (like insecure cookies),
as is `E2E_DICE` without a token, and a production configuration answers 404 for `/_e2e/dice` (`internal/web/e2edice_test.go`). The public
Content-Security-Policy and the reverse proxy configuration are unchanged: the route belongs to the development server and is never published.

### Groups

`tests/e2e/groups.json` lists the flows of each group (`core`, `pages`, `creation`, `rules`).

- `make e2e GROUP=core` (or `node run.mjs --group core`) runs one group.
- `make e2e-groups` runs all groups in turn.

Every file in `flows/` must be in exactly one group.
`make e2e-check-groups` fails on a flow in no group or in two.
It runs in `make test`, in the runner before it starts and in the CI `plan` job, so a new flow cannot be forgotten: add it to a group when you add the file.

The groups are independent: each passes alone from a fresh copy of the seeded world.
This was checked by running every flow file alone and then every group alone.
Flows hand each other only the `shared` object, which no later flow reads, and the "pool companies left busy" are per flow.
Keep it so: a flow may rely on the demo world, never on what another flow did.

The groups are balanced by the step time of the flows (measured locally, in seconds): `60-pages` takes 230 alone and the other 24 flows about 700 together, so there are four groups of about 190 to 260 s.

### CI

`.github/workflows/e2e.yml` runs on every push to `main` and every pull request:

1. A `plan` job checks the grouping and reads the group names from `groups.json`.
2. One `browser (<group>)` job per group runs in parallel (`make e2e-enforce GROUP=<group>`, `fail-fast: false` so every group reports).
   Each job has its own compose PostgreSQL and seeds its own template, which takes about 2.5 minutes.
3. The final `e2e` job (`needs` all of them, `if: always()`) fails unless every group passed.

Require the check named `e2e` in branch protection. `release-image.yml` calls the workflow and its image jobs wait for it.

The seeded world is not cached or shared as an artifact.
The groups seed concurrently, so a shared dump would only add a serial seed, dump, upload and restore in front of the groups.
A cache keyed by the hash of `cmd/server`, the migrations and `definitions/` would miss on most changes.

A failed group uploads the screenshots and page texts of the run as the `e2e-out-<group>` artifact.

The workflows use the Node 24 majors of the actions (`checkout`, `setup-go`, `setup-node` and `upload-artifact` v7; the `docker/*` actions of `release-image.yml` are on Node 24 too), pinned to major tags.

Runners are pinned to the newest GitHub-hosted Ubuntu image (`ubuntu-26.04`, `ubuntu-26.04-arm`) instead of `ubuntu-latest`, so a change of the label never alters a run by surprise.
The image ships Google Chrome at `/usr/bin/google-chrome`, so `CHROME_PATH` holds.
A pull request that touches a workflow checks the [runner image list](https://github.com/actions/runner-images#available-images) and moves the pins to the newest Ubuntu image.
If a run on a new image breaks the suite, look first at the hover emulation and the step timings.

Headless Chrome on Linux has no pointing device (`(hover: hover)` is false, true on macOS), so the harness reports one (`matchMedia` override in `lib/harness.mjs`).
Without it the hover tooltips never open on the runner.

To reproduce the runner locally:

1. Start the server on a scratch copy of the seeded template.
2. Run the suite in `node:22-bookworm` with `apt-get install chromium`, `CHROME_PATH=/usr/bin/chromium` and `E2E_BASE_URL=http://host.docker.internal:<port>`.

### Not covered

- Which face of a 3D die ends up on top. The canvas is not read; the tray's reported values and the page are compared instead.
- An aftermath roll of 12 with its bonus-die form. The server's dice are random at run time and a typed roll skips the animation, so a 12 cannot be forced without a hook in production code.
  The same `revealed` gate (`x-show="revealed"`) that hides that form is asserted on the recruit window.
  Other flows type their rolls so they do not wait for the animation.
- A "cavalry-only rule applies" assertion. The cavalry check reads what the app strikes (the mount and the rules a mount gave), and the demo's Rohan company has no rule that only a mounted model has beyond those.

### Dice in the flows

Physical dice (the 3D tray) take 1.5 to 4 s on a quiet machine and were seen to take 12 s or more on a starved runner, so a flow never
depends on how long or how a throw ends.

- Every wait for dice uses `DICE_WAIT` (40 s, `lib/util.mjs`): `waitStopped(session, selector)` for a roll panel, the style guide demo,
  a casualty card or the scenario card, and `rollNewScenario` / `waitRolledNewScenario` for the new-match card. `waitStopped` puts the
  tray's state (rolling, stopped, dice, 3D or flat) and the time waited into its error.
- There is no retry. The shipped rules place a scenario on all 36 pairs of dice (`TestScenarioByRollsUsesTheRealFile`), so the
  "found no scenario" note cannot show and a flow asserts it does not; a second throw would only hide the first one's failure.
- Prefer dice the flow decides: the style guide's "values decided beforehand" switch (`serverValues`) and the campaign "Settings: the
  server rolls every die" read the number the server sent and compare it with what the tray reports. A dev-only server endpoint that
  fixes the next roll is planned; its marker in a flow will be `diceSeed`.
- The lint in `lib/groups.mjs` (`lintDiceLoops`, run by `make e2e-check-groups`, part of `make test`) fails on a `for`/`while` whose body
  clicks a dice button (Roll again, Roll the scenario, Roll D3, the special chart, `rollNewScenario`, `throwDice`) without `serverValues`
  or `diceSeed` in the loop. The few loops that are fine as they are are in `DICE_LOOP_ALLOWED`, each with its reason; an entry that
  nothing needs any more is reported too. `lib/groups.test.mjs` tests the lint and the template key.
- Keep playbacks few: one per distinct behaviour. `86-dice3d` plays 3 given-value throws in the style guide step (a lone die, a pair, four
  dice in two lanes), 3 in the D3 step (two of one die, one of three dice; the cube's marks are asserted once per size of cube, not per
  throw), then one live throw, instead of repeating a behaviour in each; a step stays far below the 90 s step limit.

### Writing steps

`lib/harness.mjs` has the `Session` (click by selector and text, `fill`, `select`, `waitText`, `settle` for htmx) and `Harness.step`; `lib/util.mjs` has the helpers.
Tips:

- Never wait for `networkidle`: live pages keep an event stream open. `goto` waits for `load` and then for htmx to finish.
- Sheets are `role=dialog` elements, "Take command" uses a native `<dialog open>`; `hx-confirm` opens a native `confirm()`,
  which the harness accepts (the text is in `session.dialogs`); an unanswered one freezes the page.
- `innerText` follows CSS `text-transform`: match visible text, not the template's.
- Drags (Sortable with `forceFallback`) need the whole target on screen and re-aim at the target before the button is released, because the layout shifts while the ghost passes over other places (`80-creation`'s `drag`); use a tall viewport for them.
- Colours: read `getComputedStyle`, normalise `color-mix()` results through a canvas, then compute the WCAG ratio (`contrast` in `lib/util.mjs`); `layout(session, width, scheme)` sets viewport and colour scheme.
- State is shared by the flows in order (the demo world is mutated), so a new step goes after the ones it must not disturb,
  and picks a campaign by name with `campaignId`.
