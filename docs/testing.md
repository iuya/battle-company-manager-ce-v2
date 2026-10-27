# Testing

| Command | What it runs |
|---|---|
| `make test` | Go unit tests (the PostgreSQL ones are skipped) |
| `make test-integration` | All Go tests against the `bcm_test` database of the compose PostgreSQL |
| `python3 tools/validate_definitions.py` | Rules JSON schemas and cross references |
| `make e2e` | Browser smoke tests (this page) |
| `make perf` | Page-load measurements at 0/100/300 ms of latency, not a test: see [performance.md](performance.md) |

## Tabletop Simulator helper (Lua in `go test`)

`internal/core/ttshelper` runs the helper script (`helper.lua`) inside `go test` with gopher-lua, a test-only dependency, against a fake Tabletop Simulator
API and a simulation of the army spawner's grid; the fixtures are in `tests/tts/`. It needs no game and no Lua binary and runs with `make test`. The real
game is only exercised by the owner's smoke test ([tabletop-simulator.md](tabletop-simulator.md#built-the-helper-and-the-saved-object)).

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

**CI.** `.github/workflows/e2e.yml` runs `make e2e` on every push to `main` and every pull request (the runner's Chrome, the Linux
Tailwind CLI, compose PostgreSQL); the screenshots and page texts of a failed run are uploaded as the `e2e-out` artifact.
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
