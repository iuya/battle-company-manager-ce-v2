# Testing

| Command | What it runs |
|---|---|
| `make test` | Go unit tests (the PostgreSQL ones are skipped) |
| `make test-integration` | All Go tests against the `bcm_test` database of the compose PostgreSQL |
| `python3 tools/validate_definitions.py` | Rules JSON schemas and cross references |
| `make e2e` | Browser smoke tests (this page) |
| `make perf` | Page-load measurements at 0/100/300 ms of latency, not a test: see [performance.md](performance.md) |

## Browser smoke tests (`make e2e`)

Risk R1 of [security-and-risks.md](security-and-risks.md): Alpine, htmx swaps, the live event stream, dice and dialogs are
only exercised by a real browser. The suite in `tests/e2e` drives headless Chrome with
[puppeteer-core](https://pptr.dev/) through the demo world. It is a separate Node project (its own `package.json` and
`package-lock.json`, `puppeteer-core` pinned); nothing of it is in the Go module.

**Setup (once).** Node 20+ and a Chrome or Chromium (found in the usual places; set `CHROME_PATH` otherwise). `make e2e-install`
runs `npm ci` in `tests/e2e` (`make e2e` does it when `node_modules` is missing). The compose PostgreSQL must run (`make db-up`,
done by the target).

**Run.** `make e2e` (about 6 minutes), `make e2e ONLY=match` for the flow files whose name contains `match`. Output goes
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
- The server listens on a free port chosen by the OS (`APP_ENV=development`, `CSP_MODE=report`, `STYLEGUIDE=true`).
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

**CI.** `.github/workflows/e2e.yml` runs `make e2e` on every push to `main` and every pull request (the runner's Chrome, the Linux
Tailwind CLI, compose PostgreSQL); the screenshots and page texts of a failed run are uploaded as the `e2e-out` artifact.

**Not covered yet:** the 3D dice animation (typed rolls are used so the steps do not wait for it). The cavalry check reads what the app strikes (the mount and the rules a mount gave); the demo's Rohan company has no rule that only a mounted model has beyond those, so a "cavalry-only rule applies" assertion beyond them is not made.

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
