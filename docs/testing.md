# Testing

| Command | What it runs |
|---|---|
| `make test` | Go unit tests (the PostgreSQL ones are skipped) |
| `make test-integration` | All Go tests against the `bcm_test` database of the compose PostgreSQL |
| `python3 tools/validate_definitions.py` | Rules JSON schemas and cross references |
| `make check-ruleset FILE=…` | Checks a lean rules file like the upload does ([definitions.md](definitions.md#upload-validation)) |
| `make fuzz [FUZZTIME=1m]` | Explores `FuzzParseDice` and `FuzzDecode` (their seed corpus already runs in `make test`) |
| `make e2e` | Browser smoke tests (this page); `make e2e-groups` runs them group by group, `make e2e-check-groups` (part of `make test`) checks the grouping |
| `make perf` | Page-load measurements at 0/100/300 ms of latency, not a test: see [performance.md](performance.md) |

## Tabletop Simulator helper (Lua in `go test`)

`internal/core/ttshelper` runs the helper script (`helper.lua`) inside `go test` with gopher-lua, a test-only dependency, against a fake Tabletop Simulator
API and a simulation of the army spawner's grid; the fixtures are in `tests/tts/`. It needs no game and no Lua binary and runs with `make test`. The real
game is only exercised by the owner's smoke test ([tabletop-simulator.md](tabletop-simulator.md#built-the-helper-and-the-saved-object)).

## Golden snapshot of the Battle Companies rules (`go test ./internal/core -run Snapshot`)

`internal/core/snapshot_test.go` freezes everything the app derives from the Battle Companies file, as text in `internal/core/testdata/snapshot/`
(about 2 MB): one `<company>.txt` per company plus `_index`, `_armoury`, `_progression`, `_campaign`, `_ruletargets`, `_tooltips` and
`_offersets`. It fails on any difference (the first differing lines are shown); it reads only what core exposes (BCBook, CompanyProfile,
`AuditRules`, `BuildDetail`, the progression and campaign tables), never the json, so a change of the file format must leave the goldens
untouched. The few unexported helpers it needs are in its "seams" block. After an intended change, `go test ./internal/core -run Snapshot
-update-snapshot` rewrites the files and the diff is reviewed in its own commit.

| File | Content |
|---|---|
| `<company>.txt` | anomalies; the whole profile (start, charts, rules, effects, aliases, hero upgrades, limits, oath, notes); starting gold, points and the totals of every allowed officer choice; the company's unique wargear (limits, `Only`, ratings); every unit (stats, keywords, options with reserved/unsold/read-as, built warrior and hero, what each can buy with gold, `via` and refusals); who may take each hero upgrade; every promotion (match as warrior and hero for each gear combination, choices, gear kept or dropped); tooltips (digest) and glossary terms; the name-resolution map (each unit, gear and rule the company mentions and what it resolves to) |
| `_armoury.txt` | the common armoury with its ratings, gold, restrictions, grants and gear annotations (asserted equal for every company) |
| `_progression.txt` | level-up tables, warrior table, per path the offers for each 2D6 result of a fresh hero and the stat limits (`improvedStat`) |
| `_campaign.txt` | injuries, aftermath (experience, influence, Against the Odds bonuses, charts, effects, scenario rewards), scenarios, campaign options, Against the Odds, break point, hero rating, creatures, wanderers, armour upgrade |
| `_ruletargets.txt` | the rule-target table and, for every rule in use, who it works against expanded over the keywords of every unit |
| `_tooltips.txt`, `_offersets.txt` | texts the company files cite by digest (tooltips, the equipment and rule refusals shared by many units) |

## The lean decoder (`internal/core/lean`)

Pure package, so most tests need no mock; `Lower` (what vanilla and the glossary know) has a hand-written fake for the worked examples
(`fakelower_test.go`) and a generated mock (`make generate`) where a test must see the calls. Tests are table-driven.

- **Fixtures.** `testdata/valid/*.json` are complete minimal files built from the worked examples of the design (section 4); each decodes with no
  issue, in the lean package against the fake world and in `internal/core` against the shipped vanilla file and glossary. `testdata/invalid/<case>.json`
  come with `<case>.want`, the exact report text, one per row of the section 7 catalogue. `testdata/gen_fixtures.py` writes the inputs
  (`python3 -I internal/core/lean/testdata/gen_fixtures.py`); `go test ./internal/core/lean -run TestInvalidFixtures -update` rewrites the `.want`
  files, and the diff is read: the wording is reviewed there.
- **Upload error catalogue.** `internal/core/rulesets_catalogue_test.go` drives every invalid fixture through `Rulesets.Check` against the real
  vanilla file and glossary and compares the full rendered report with `internal/core/testdata/catalogue/<case>.want`
  (`go test ./internal/core -run TestUploadErrorCatalogue -update-catalogue`); a file with twelve mistakes returns twelve issues, a file with
  thousands is capped at 1,000 and counted.
- **Fuzz.** `FuzzDecode` (no panic, no hang beyond 2 s, the same bytes give the same report, a clean document re-encoded and decoded is equal) and
  `FuzzParseDice`. The seeds are every fixture and the shipped file; they run in `make test`, `make fuzz` explores.
- **Equivalence.** `internal/core/lean_convert_test.go` has the test-only converter `oldToLean` (never shipped) and `TestLeanEquivalence`, a registry with
  one entry per section of the old file. Slice 1 converts the header and `campaign` and checks that the typed parameters equal what the engine
  uses today; slice 3 converts `scenarios[]` (scoring objectives as typed `scoring[]` with one option per tier sentence, stored once) and checks that
  the line composed from the options equals the three old prose forms byte for byte, that `maxPoints` is derivable, and (`TestLeanScenariosGaps`)
  which old keys the lean model still has no field for. A later slice adds its section: extend `oldToLean`, replace the `skip` of its entry with a `check`, build the consumer from
  `RuleSet.Doc`, and the golden snapshot proves the pages did not change.
- **Benchmark.** `go test ./internal/core/lean -run xxx -bench DecodeLarge`: a 650 KB file decodes in about 25 ms (budget 100 ms); a decode happens
  once per content hash.

## Custom rulesets and the magic-number lint (`internal/core`)

`customRuleset(t, mutate)` (`customruleset_test.go`) loads the shipped vanilla, Battle Companies and glossary files, lets `mutate` change the
decoded Battle Companies document (a `map[string]any`) and builds a `RuleSet` with `LoadRuleSet`, the path production uses. `TestCustomRuleset`
is table-driven, one row per engine parameter ([definitions-lean-design.md](definitions-lean-design.md) section 12.8): `mutate` sets the value,
`check` asserts the behaviour through the built rule set. The parameter slices P1 to P6 add rows; today it holds a default row, the treatment
`perPoint` of the "attempt" method, the recruiting parameters (P1) and the dice (P4: injury dice d12, Courage dice, bonus dice, plus
`TestCustomRulesetRefusesDiceTheTablesDoNotCover` for dice a table does not cover). `dice_test.go` tests `Dice.Roll`, `FixedDice`,
`CheckCovers`, the typed-roll folding, `ParseScenarioSelection` and the hero roll with another number of results.
`customruleset_company_test.go` holds the rows of slices P2 and P3 (`TestCustomRulesetCompanyAndLimits`: officers, hero start, `maxModels`, XP threshold, the
hero rating threshold, `limits[]`, creature, wanderer and path limits, the Courage and Intelligence cap; `TestCustomRulesetRefusesBadCompanyAndLimitParameters`
for the path-naming load errors); the treatment dice are in `treatment_test.go`. The web templates that print these numbers are tested in
`internal/web/rules_numbers_test.go` (the officer places, the experience bar, the treatment window).

`magicnumbers_test.go` holds two report-only checks (they log, never fail; run `go test ./internal/core -run 'Magic|TemplateRule' -v`):

- `TestMagicNumbersReport` parses the non-test `internal/core/*.go` with `go/ast`, counts integer literals other than 0, 1 and 2 (set
  `magicAllowedValues`) and not in `magicAllowList`, logs the total, the top files and values, and lists declarations of the engine constants
  slices P1 to P3 delete (`magicForbiddenNames`).
- `TestTemplateRuleLiteralsReport` greps `internal/web/templates` for `up to N` and `ND6` and logs the count per file.

Allow-list format: `magicAllowList["file.go:value"] = "reason"` (that literal anywhere in the file) or `"file.go:Func:value"` (inside one function);
the reason is mandatory in review, and the list should shrink. To make the lint enforcing (slice P6), set `magicNumbersEnforce = true`, clear or
justify every remaining literal, and delete the now-empty rows of the report.

## Browser smoke tests (`make e2e`)

Risk R1 of [security-and-risks.md](security-and-risks.md): Alpine, htmx swaps, the live event stream, dice and dialogs are
only exercised by a real browser. The suite in `tests/e2e` drives headless Chrome with
[puppeteer-core](https://pptr.dev/) through the demo world. It is a separate Node project (its own `package.json` and
`package-lock.json`, `puppeteer-core` pinned); nothing of it is in the Go module.

**Setup (once).** Node 20+ and a Chrome or Chromium (found in the usual places; set `CHROME_PATH` otherwise). `make e2e-install`
runs `npm ci` in `tests/e2e` (`make e2e` does it when `node_modules` is missing). The compose PostgreSQL must run (`make db-up`,
done by the target).

**Run.** `make e2e` (about 6 minutes; `make e2e-enforce` is the same with the server in `CSP_MODE=enforce`, which is what CI runs: a Content-Security-Policy violation of any page, browser event or `/csp-report` log line, fails the step or the run; flow `90-csp`), `make e2e ONLY=match` for the flow files whose name contains `match`. Output goes
to the terminal: one line per step (`ok` / `FAIL` with the reason). On a failure `tests/e2e/out/` gets, for every signed-in
browser session, a full-page screenshot (`<step>.<n>-<who>.png`) and the page text (`.txt`), plus `server.log`; `out/` is
gitignored and emptied at each run.

**Isolation.** The run never touches the development database or `:8080`:

- It builds the server to `tests/e2e/out/bcm-e2e-server-<pid>` (one binary per run, removed at the end).
- The demo world takes minutes to seed, so it is seeded once into a template database `bcm_e2e_tpl_<hash>` (the hash covers
  `cmd/server`, the migrations and `definitions/`, so a change to any of them reseeds; `E2E_RESEED=1` forces it) and every run
  creates `bcm_e2e_run_<pid>` from it with `CREATE DATABASE ... TEMPLATE`, so each run starts from the same fresh world. The hash
  uses paths relative to the repository, so a clone or worktree shares the template. Seeding and copying the template are
  serialised across runs by a lock directory (`$TMPDIR/bcm-e2e-prepare.lock`; a run that finds it waits, a lock older than
  20 minutes is taken over), because PostgreSQL refuses `TEMPLATE` while another session uses the template.
- Several runs at once (two agents, a clone) are safe: own database, own port, own binary. They still share `tests/e2e/out/`
  (each run empties the screenshots and `server.log` of the others when it starts) and the compose PostgreSQL; from another
  checkout set `COMPOSE_PROJECT_NAME` to the project of the running PostgreSQL (`docker ps` label `com.docker.compose.project`).
- `TestSeedWorld` makes `bcm_world_test_<pid>_<n>`, unique per run, so two `make test-integration` runs do not drop each other's
  database (it used to be one fixed name).
- The server listens on a free port chosen by the OS (`APP_ENV=development`, `CSP_MODE=report` or `E2E_CSP_MODE`, `STYLEGUIDE=true`).
- At the end (also on Ctrl-C) it stops only the processes it started (by pid, no `pkill`) and drops its own database.
  `make e2e-clean` drops the cached templates (`bcm_e2e_*`) and `out/`.
- `E2E_BASE_URL=http://127.0.0.1:PORT node run.mjs` uses a server you started yourself (scratch data only: the flows write).
  `E2E_ADMIN_URL` (a PostgreSQL URL with rights to create databases) replaces the `docker compose exec psql` calls;
  `E2E_DB_URL_BASE` is the base URL the server gets (default `postgres://bcm:bcm@localhost:5432`).

**Failures that pass on a rerun.** (Root cause of the lost sheet click found and fixed: htmx wires a swapped-in fragment only when it settles, 20 ms after the swap, and a click in that gap sends nothing; `static/js/htmx_ready.js` now processes the fragment at once, and the step "a menu entry clicked the instant it is swapped in" in `40-match` holds it. Other first-run failures are still load until shown otherwise.) Three full runs of the optimisation pass (6 live and aftermath steps failing together, then green)
were investigated in [mqn.169]: the six are one step failing and the steps after it depending on it (a match flow is a chain). Not
reproduced on a quiet machine (about 40 match-flow runs, serial and four at once, also with 30 busy processes) and no race found by `go test -race`.
The one failure seen later was a click on a sheet entry ("Wounded a unit") that sent no request, on a machine running a race
test loop. So the harness now (a) waits for the clicked box to hold still, (b) clicks again once when a button with `hx-get` or
`hx-post` sent no request and prints `(click ... sent no request, clicking again)` and a count in the summary, (c) writes into
every failure dump the last 60 requests of the page with times, the live dot, `data-seq` and the open sheets. If that count shows
up in a green run, or a failure dump shows the request missing, that is the evidence for a real app bug in the sheet; until then
treat a first-run failure that is green on rerun as load, not as a regression.

**What it covers** (`tests/e2e/flows`, run in file order; every step also fails on any browser console error, uncaught
exception, failed request or 4xx/5xx answer it did not expect):

| File | Steps |
|---|---|
| `10-accounts` | login refused and accepted, logout and the redirect; account time zone saved; glossary cross reference scrolls to its term; admin pages load and a plain user is refused |
| `20-campaign` | organiser creates a campaign; invitation link; a second player joins by it; both take command (confirmation dialog); an officer chooses a path; Manage campaign: pause, resume, end with the typed name (Continue stays disabled until it matches) |
| `30-company` | recruit (a typed roll, confirm, influence and units change); armoury purchase with gold; a hero's level-up; Change formation by dragging a card with the mouse, saved and kept after a reload |
| `40-match` | create a match, pick a scenario; live mode with two players and the organiser watching: tracker change seen by the viewer through the event stream and Undo; the stream cut by request interception ("Reconnecting", the page catches up when it is back) and refused with a 429 (the page polls every 10 s); mount removed (seen by the viewer); wound an enemy unit; retreat and return; end checks; log; result; propose to end, the other player confirms; aftermath start, injury roll and Finish for both companies |
| `50-ranger` | the Ranger of the North swap on a recruit result in the "Cemetery" campaign |
| `60-pages` | 21 page types (login, home, campaign pages, a match, a live company, an aftermath, glossary, account, admin, style guide) at 360, 768 and 1280 px, light and dark: no horizontal scroll, the colour scheme really switches, no console errors or failed requests |

| `60-pages` (end) | axe-core 4.10 (pinned) on the same pages at 1280 and 360 px, light and dark: no serious or critical violation of WCAG 2.x A/AA; `AXE_OFF` in the file lists the rules left out, each with its reason |
| `70-keyboard` | the keyboard walk: Tab reaches the controls of the sign-in form and a company page with a visible focus ring; a sheet opens with Enter, keeps the focus inside, closes with Escape and gives the focus back |
| `80-creation` | company creation: the Shire's Ranger of the North starts as the Leader, locked (real mouse drags out of the place and onto it, the Leader button, no Remove) and a forged POST without him gets a 422 with "must start as the Leader"; Rohan's officers moved with real drags between the places and the pool (the form's hidden fields follow, the suggestion banner goes, the vow dialog names the officers); the officer panel at 360/768/1280 px, light and dark |
| `81-aftermath-ui` | the aftermath sheet's XP chips (height, padding, inside their card, no horizontal scroll at 3 widths x 2 themes) and, once the sheet is locked, the Survived / Casualty / Fled badges: three different computed colour pairs, contrast of at least 4.5 (read from the computed styles, `color-mix()` normalised through a canvas) |
| `82-company-ui` | the Elven Cloak badge's tooltip on My Company (Stalk Unseen, To Hit) at 3 widths x 2 themes; the buy window's "Find an item" by name, by alias and "No item matches your search."; the armoury's hide switch (on by default, remembered after a reload), main category chips disabling the item chips that do not apply, creature and wanderer tooltips |
| `83-fallback` | the Warrior-to-Hero fallback of "Aftermath ward 3" of the demo world: one Warrior ahead in Experience (offered alone, chosen), two tied (both listed, none chosen, answer required), a Leader fallen with both Sergeants; no "nobody" option; the chosen Warrior's path is refused until the aftermath is finished, then taken |
| `84-match-extras` | a three-company match (Éowyn's cavalry and Gimli's Dwarves against Sauron) on Fog of War: the secret objectives by label with told-apart heroes, kept private and revealed with their score at the end; the Rohan live cards (mounts, the granted Arise rule, losing one mount strikes only that horse); an MVP tie with two opposing votes ("Tied: ... picks in the aftermath"), broken by a third vote and restored; the aftermath's tie hint |
| `85-creation-submit` | a throwaway campaign of its own (the organiser creates it, two players join by the invitation link, so the demo world is untouched): a Shire company is sworn (the Ranger of the North leads, the vow dialog names the officers, redirect to My company, one Leader and two Sergeants as suggested, every unit with its points) and a Rohan company with officers arranged by real mouse drags is created exactly as dragged; both are on Companies and Rankings with the sum of their units' points; a second submit is refused with a 422 ("already have a company"), nothing is added, and the creation page sends the member to My company |
| `86-dice3d` | the dice animation, with real WebGL (headless Chrome through SwiftShader, flags in `lib/harness.mjs`): a server roll (the campaign where the server rolls every die) loads `dice3d.js` only then, rolls on the 3D tray, ends, reports the number the server sent (`data-roll`) and shows it; the result, raise buttons and Confirm are hidden while the canvas rolls and appear when it stops (sampled every frame), the recruit can then be finished; with `getContext('webgl')` answering null the flat tray takes over and the result still shows; with `dice3d.js` blocked the flat tray is used; the style guide tray stops on given values (1, 2, 4 dice, two lanes) and can be rolled again, a browser roll is shaken once with the space bar, `prefers-reduced-motion` gives the result at once; the tray fits 360, 768 and 1280 px, light and dark |
| `87-concepts` | the helper popup: a chip on My Company opens it (hover keeps the tooltip, which steps aside), its groups and "Open in glossary", a chip swaps the content and Back returns, Escape closes and the focus returns to the term, backdrop and Close; the shield's explicit cancels fold into closed lines with a count; a vanished concept says "no longer in these rules", an unknown name falls back to the tooltip text; keyboard (Enter opens, Tab and Shift+Tab stay inside, the heading takes the focus after a step); a glossary link in an expanded card opens it over the card and Escape closes only the popup; the live company page's terms open it; the sheet at 360/768/1280 px in both themes (bottom sheet at 360, card from 768, inside the window, no horizontal scroll, axe on the popup) with screenshots `out/concept-popup-*.png` |
| `88-own-path` | units On Their Own Path and promoted officers: the overview text of the "everything on" campaign, a Sentinel as a warrior with XP and Level Up, a typed 6 that makes it a hero on the Path of Their Own Making with no path window, re-roll or forgo offer (360/768/1280, both themes, no horizontal scroll), the Take command badges following the campaign setting, and Arnor's officers previewed, confirmed and rostered as Rangers of the North (a throwaway campaign) |
| `91-tts` | the copy button and its dialog (Tabletop Simulator army list): button next to the name with an accessible name and a tooltip on hover and keyboard focus, the list of a Rohan cavalry company (line format, hero line, counts, Break Point, notes), Escape and the focus return, the Tab trap, Copy into the clipboard (and the select-all fallback), the two switches on a Warg company (htmx, no page load, remembered in localStorage), another player's company (wanderers and creatures as stand-ins), a stranger's 404, the live variant (a retreated unit goes to a second warband with a note), 360/768/1280 px in both themes with axe and screenshots (`out/tts-*.png`) |
| `92-tts-names` | level B block of the Tabletop Simulator dialog: valid ArmyNames JSON with one entry per figure matching the list lines in order, the rule-text switch (none < short <= full, htmx, remembered, survives a list switch), Copy names (clipboard and select fallback, own live region), `/tts/helper.zip` fetched in the page (200, `application/zip`, ETag, `PK` magic, a `.json` and a `.png` entry by their central directory sizes), the collapsed Lua disclosure with its script and copy button, 360/768/1280 px in both themes with axe and screenshots (`out/tts-names-*.png`); the live fallen marker is checked in `91-tts` |
| `93-to-wound` | the To Wound chart window: the match page's button opens a 10 x 10 chart with seven well-known cells of the printed chart (S3 vs D3 4+, S4 vs D6 5+, S5 vs D3 3+, S1 vs D1 4+, S2 vs D7 6+/4+, S1 vs D9 -, S9 vs D10 5+), Escape and the focus return, the window outside `#match`; the find control (typed values, steppers, clamping, a tap on a cell, row/column/cell marked); on the live company page the window outside `#live-zones` stays open with its pick after a live refresh and finding a roll sends no request; the Tab trap, Close and the backdrop; 360/768/1280 px in both themes on both pages: no page scroll, sticky headers, six distinct band colours with contrast of at least 4.5, axe and screenshots (`out/to-wound-*.png`) |
| `94-envenom` | Envenom Weapon (X) on the gallery's Mordor company: the Armory tab's unit rows each carry a weapon chooser built from that unit's weapons (nothing preselected, required); bought from the buy window (Orc Bow) and from the Armory tab (Spear), the card shows `Poisoned Attacks (<weapon>)` once and the weapon item stays plain; the choice is final (no chooser, no Change weapon), dropping the Spear keeps the rule and buying it again shows it once; Harad's profile-poisoned Bow is not offered; a unit that already carries it has the reason and no button; 3 widths x 2 themes: no horizontal scroll, axe clean on the Armory tab and the buy window (the "no weapon left" reason is a handler test: no demo unit reaches it) |
| `95-keyword-targets` | the demo campaign "Keyword targets" (Gimli's Dwarves with Ancient Enemies (Orc) against Radagast's Mirkwood company): a rule chip's tooltip and helper popup say "Works against: enemy models with the Orc keyword." and the own card is not marked; on the opponent's page every card shows its keywords and exactly the Orcs carry "Ancient Enemies (Orc) applies" (tooltip, Enter opens the popup, Escape returns the focus); the reverse side, the opponent's own page and an onlooker see no marker; "Wounded a unit" lists the enemy units with keywords and the same markers; a retreating Orc loses its marker through the live refresh, its compact card keeps the chips, and the marker returns with the unit; 360/768/1280 px x light/dark: no horizontal scroll, markers inside the card, axe, screenshots |
| `96-tts-auras` | aura rings on the demo campaign "Aura parade" (Fiefdoms of Gondor as `you`): the Auras section of the Tabletop Simulator dialog (switch on by default, the five kind rows with counts and radii equal to the rings of the v2 payload, groups, totals line, legend with letters, five colours and the thicker enemy line), known figures (Company Standard 3" banner, Company Drum 12" declared, Voice of Command 12" and no 6" command ring on the Leader, Stand Fast 6" on every Hero, at most 6 rings per figure, the Leader keeps all rings with Furious Tirade, the Large rings switch is there), the "download the helper again" notice and the Rings button steps, the group and kind switches (declared, targeting ranges, Torch, a kind off) change the payload by htmx with no page load and are remembered in `bcm-tts`, switch off or no kind gives version 1 byte for byte, keyboard (Space on the switches, the focus returns after the swap), the chips hidden by default and the "Show auras on unit cards" toggle (cookie `bcm-auras`, kept after a reload, the expanded Auras section follows), the aura chips on the cards (radius, kind letter, tooltip, aria-label, muted war horn ring, `+N` in place, glossary popup on Enter and focus back on Escape, the expanded card), another player's read-only company and own company of another account, the live variant (a retreated unit has no ring, its compact card no chip row), 360/768/1280 px x light/dark with axe and screenshots (`out/tts-auras-*.png`, `out/tts-auras-chips-*.png`) |

**Groups.** `tests/e2e/groups.json` lists the flows of each group (`core`, `pages`, `creation`, `rules`); `make e2e GROUP=core` (or `node run.mjs --group core`) runs one, `make e2e-groups` all in turn. Every file in `flows/` must be in exactly one group: `make e2e-check-groups` (run by `make test`, by the runner before it starts and by the CI `plan` job) fails on a flow in no group or in two, so a new flow cannot be forgotten: add it to a group when you add the file. The groups are independent: each passes alone from a fresh copy of the seeded world (checked by running every flow file alone and then every group alone; flows hand each other only the `shared` object, which no later flow reads, and the "pool companies left busy" are per flow). Keep it so: a flow may rely on the demo world, never on what another flow did. Balance by the step time of the flows (measured locally, seconds): `60-pages` 230 alone, the other 23 flows about 640 together, so four groups of about 190 to 230 s.

**CI.** `.github/workflows/e2e.yml` runs on every push to `main` and every pull request: a `plan` job checks the grouping and reads the group names from `groups.json`, then one `browser (<group>)` job per group runs in parallel (`make e2e-enforce GROUP=<group>`, `fail-fast: false` so every group reports; each job has its own compose PostgreSQL and seeds its own template, about 2.5 min, in parallel with the others), and the final `e2e` job (`needs` all of them, `if: always()`) fails unless every group passed. Require the check named `e2e` (branch protection); `release-image.yml` calls the workflow and its image jobs wait for it. The seeded world is not cached or shared as an artifact: the groups seed concurrently, so a shared dump would only add a serial seed, dump, upload and restore in front of the groups, and a cache keyed by the hash of `cmd/server`, the migrations and `definitions/` would miss on most changes. A failed group uploads its screenshots as `e2e-out-<group>`. (The runner's Chrome, the Linux
Tailwind CLI, compose PostgreSQL); the screenshots and page texts of a failed run are uploaded as the `e2e-out-<group>` artifact.
The workflows use the Node 24 majors of the actions (`checkout`, `setup-go`, `setup-node` and `upload-artifact` v7; the `docker/*` actions of `release-image.yml` are on Node 24 too), pinned to major tags. `runs-on: ubuntu-latest` is kept: the runner image (Ubuntu 24.04, moving to 26.04 in a gradual migration) ships Google Chrome 155 at `/usr/bin/google-chrome` on both, so `CHROME_PATH` holds; pin `ubuntu-24.04` only if a run on the new image breaks the suite, and look first at the hover emulation and the step timings below.
Headless Chrome on Linux has no pointing device (`(hover: hover)` is false, true on macOS), so the harness reports one (`matchMedia` override in `lib/harness.mjs`); without it the hover tooltips never open on the runner. To reproduce the runner locally: start the server on a scratch copy of the seeded template and run the suite in `node:22-bookworm` with `apt-get install chromium`, `CHROME_PATH=/usr/bin/chromium` and `E2E_BASE_URL=http://host.docker.internal:<port>`.

**Not covered:** which face of a 3D die ends up on top (the canvas is not read: the tray's reported values and the page are compared instead), and an aftermath roll of 12 with its bonus-die form: the server's dice are random at run time and a typed roll skips the animation, so a 12 cannot be forced without a hook in production code; the same `revealed` gate (`x-show="revealed"`) that hides that form is asserted on the recruit window. Other flows type their rolls so they do not wait for the animation. The cavalry check reads what the app strikes (the mount and the rules a mount gave); the demo's Rohan company has no rule that only a mounted model has beyond those, so a "cavalry-only rule applies" assertion beyond them is not made.

**Writing steps.** `lib/harness.mjs` has the `Session` (click by selector and text, `fill`, `select`, `waitText`, `settle`
for htmx) and `Harness.step`; `lib/util.mjs` the helpers. Things learnt:

- Never wait for `networkidle`: live pages keep an event stream open. `goto` waits for `load` and then for htmx to finish.
- Sheets are `role=dialog` elements, "Take command" uses a native `<dialog open>`; `hx-confirm` opens a native `confirm()`,
  which the harness accepts (the text is in `session.dialogs`); an unanswered one freezes the page.
- `innerText` follows CSS `text-transform`: match visible text, not the template's.
- Drags (Sortable with `forceFallback`) need the whole target on screen and re-aim at the target before the button is released, because the layout shifts while the ghost passes over other places (`80-creation`'s `drag`); use a tall viewport for them.
- Colours: read `getComputedStyle`, normalise `color-mix()` results through a canvas, then compute the WCAG ratio (`contrast` in `lib/util.mjs`); `layout(session, width, scheme)` sets viewport and colour scheme.
- State is shared by the flows in order (the demo world is mutated), so a new step goes after the ones it must not disturb,
  and picks a campaign by name with `campaignId`.
