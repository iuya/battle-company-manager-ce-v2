# Testing

| Command | What it runs |
|---|---|
| `make test` | Go unit tests (the PostgreSQL ones are skipped) |
| `make test-integration` | All Go tests against the `bcm_test` database of the compose PostgreSQL |
| `python3 tools/validate_definitions.py` | Rules JSON schemas and cross references |
| `make e2e` | Browser smoke tests (this page) |

## Browser smoke tests (`make e2e`)

Risk R1 of [security-and-risks.md](security-and-risks.md): Alpine, htmx swaps, the live event stream, dice and dialogs are
only exercised by a real browser. The suite in `tests/e2e` drives headless Chrome with
[puppeteer-core](https://pptr.dev/) through the demo world. It is a separate Node project (its own `package.json` and
`package-lock.json`, `puppeteer-core` pinned); nothing of it is in the Go module.

**Setup (once).** Node 20+ and a Chrome or Chromium (found in the usual places; set `CHROME_PATH` otherwise). `make e2e-install`
runs `npm ci` in `tests/e2e` (`make e2e` does it when `node_modules` is missing). The compose PostgreSQL must run (`make db-up`,
done by the target).

**Run.** `make e2e` (about 4 minutes), `make e2e ONLY=match` for the flow files whose name contains `match`. Output goes
to the terminal: one line per step (`ok` / `FAIL` with the reason). On a failure `tests/e2e/out/` gets, for every signed-in
browser session, a full-page screenshot (`<step>.<n>-<who>.png`) and the page text (`.txt`), plus `server.log`; `out/` is
gitignored and emptied at each run.

**Isolation.** The run never touches the development database or `:8080`:

- It builds the server to `tests/e2e/out/bcm-e2e-server`.
- The demo world takes minutes to seed, so it is seeded once into a template database `bcm_e2e_tpl_<hash>` (the hash covers
  `cmd/server`, the migrations and `definitions/`, so a change to any of them reseeds; `E2E_RESEED=1` forces it) and every run
  creates `bcm_e2e_run_<pid>` from it with `CREATE DATABASE ... TEMPLATE`, so each run starts from the same fresh world.
- The server listens on a free port chosen by the OS (`APP_ENV=development`, `CSP_MODE=report`, `STYLEGUIDE=true`).
- At the end (also on Ctrl-C) it stops only the processes it started (by pid, no `pkill`) and drops its own database.
  `make e2e-clean` drops the cached templates (`bcm_e2e_*`) and `out/`.
- `E2E_BASE_URL=http://127.0.0.1:PORT node run.mjs` uses a server you started yourself (scratch data only: the flows write).
  `E2E_ADMIN_URL` (a PostgreSQL URL with rights to create databases) replaces the `docker compose exec psql` calls;
  `E2E_DB_URL_BASE` is the base URL the server gets (default `postgres://bcm:bcm@localhost:5432`).

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

**CI.** `.github/workflows/e2e.yml` runs `make e2e` on every push to `main` and every pull request (the runner's Chrome, the Linux
Tailwind CLI, compose PostgreSQL); the screenshots and page texts of a failed run are uploaded as the `e2e-out` artifact.

**Not covered yet:** the 3D dice animation (typed rolls are used so the steps do not wait for it).

**Writing steps.** `lib/harness.mjs` has the `Session` (click by selector and text, `fill`, `select`, `waitText`, `settle`
for htmx) and `Harness.step`; `lib/util.mjs` the helpers. Things learnt:

- Never wait for `networkidle`: live pages keep an event stream open. `goto` waits for `load` and then for htmx to finish.
- Sheets are `role=dialog` elements, "Take command" uses a native `<dialog open>`; `hx-confirm` opens a native `confirm()`,
  which the harness accepts (the text is in `session.dialogs`); an unanswered one freezes the page.
- `innerText` follows CSS `text-transform`: match visible text, not the template's.
- State is shared by the flows in order (the demo world is mutated), so a new step goes after the ones it must not disturb,
  and picks a campaign by name with `campaignId`.
