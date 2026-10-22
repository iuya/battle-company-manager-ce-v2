# Security and risks

Review of the app as it is now (refreshed 2026-10-22): the Content-Security-Policy analysis (bead `mqn.94`) and a ranked register of
risks. Evidence: the code (`internal/web/middleware.go`, `session.go`, `server.go`, `cmd/server/main.go`, templates, `static/js`),
the architecture docs and the QA/audit beads (`mqn`, `amy`). Score = likelihood (1-5) x impact (1-5). Effort: S under half a day, M a day or two, L more.
Items marked *bead* have one; items marked *REVIEW* need a decision from the owner.

## 1. Content-Security-Policy

**What it is.** A response header telling the browser which origins and mechanisms a page may use for scripts, styles,
images, fonts, connections, frames and forms. Anything outside the policy is refused by the browser, and violations can be reported.
It is a second line of defence: it does not fix injection bugs, it limits what an injection (or a compromised third-party script) can do.

**Why here.** The app renders user-chosen text (company and unit names, notes, scenario text, glossary and rules files uploaded by a superuser) in
Go templates (html/template escapes by context), and puts server values into **Alpine attributes** (`x-data`, `x-text`, `@click`: about 60 `x-data` and 135 `x-show` uses). An escaping slip in an attribute context
is the one XSS class this app has had (mqn.18). The headers today are `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: same-origin`,
a `Content-Security-Policy-Report-Only` header (`CSP_MODE=report`, the default; violations go to `POST /csp-report`, logged, rate limited) and HSTS when cookies are secure.
Sessions are `HttpOnly`, `SameSite=Lax`, `Secure` in production (the server refuses to start otherwise). That XSS (mqn.18, rules-file text inside Alpine expressions) is fixed: the text is emitted as JS string literals.

**What it would block, and what not**, given the current front end (Alpine standard build, htmx, Sortable, a bundled three.js dice, SSE, two self-hosted woff2 fonts, all from `/static`, no CDN):

| Directive | Value | Effect | Caveat |
|---|---|---|---|
| `default-src` | `'self'` | blocks any third-party script, style, image, font, XHR | none; no external resources are loaded |
| `script-src` | `'self' 'unsafe-eval'` | blocks injected `<script>` and `javascript:` URLs and event-handler attributes (`onclick=`); allows our files | **Alpine's standard build compiles attribute expressions with `new Function`, so `'unsafe-eval'` is required.** An injection that lands inside an Alpine attribute is still executable. So the policy stops the common XSS (script tags, `on*` attributes, external loaders) but not the Alpine-attribute class |
| `style-src` | `'self' 'unsafe-inline'` | blocks external CSS | 9 `style="..."` attributes and Alpine `:style` bindings need `'unsafe-inline'` (or moving them to classes); a CSS-only injection is low impact |
| `img-src` | `'self' data:` | blocks tracking images | `data:` for the inline SVG icons if any |
| `font-src` | `'self'` | ok | fonts are in `/static/fonts` |
| `connect-src` | `'self'` | covers htmx requests and the SSE `EventSource` (same origin) | none |
| `worker-src`, `child-src` | `'self' blob:` | only if the dice uses workers; `grep` finds none in the non-minified scripts | verify in a browser; three.js renders to a `<canvas>`, which needs no directive |
| `frame-ancestors` | `'none'` | clickjacking protection, replaces `X-Frame-Options` | none |
| `form-action` | `'self'` | an injected form cannot post elsewhere | none |
| `base-uri` | `'self'` | blocks `<base>` injection (htmx and relative links) | none |
| `object-src` | `'none'` | no plugins | none |

**Inline script.** `base.gohtml` already loads the theme script from `/static/js/theme.js` (a file, not inline), so no nonce is needed there; `grep` finds 16 `<script` tags, all with `src` (to confirm in the staged step 1).
If any inline block remains it gets a per-request nonce (`script-src 'nonce-...'`) from the existing middleware, never `'unsafe-inline'`.

**Removing `'unsafe-eval'`** needs Alpine's CSP build (`@alpinejs/csp`), which forbids expressions in attributes: every `x-data="{...}"`, `x-show="a && b"`, `@click="x = !x"` must become a named
`Alpine.data('component', ...)` with methods and getters. About 60 `x-data` and several hundred expressions in the templates plus the page scripts (`company.js`, `match_form.js`, `lists.js`...). Effort **L** (a week, with browser testing of every page),
risk of silent regressions because nothing in CI runs a browser (see risk R3). Benefit: the Alpine-attribute injection class is also closed. Not worth it until a browser test harness exists.

**Staged plan.**
1. (S) `Content-Security-Policy-Report-Only` with the table above (`'unsafe-eval'`, `'unsafe-inline'` for styles), `report-uri` to a tiny `POST /csp-report` handler that logs a line (rate limited, 4 KB body cap). Run a week on the demo world and real use; fix every report (inline scripts, `blob:`, dice).
2. (S) Switch to enforcing `Content-Security-Policy` with `default-src 'self'; script-src 'self' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; form-action 'self'; base-uri 'self'; object-src 'none'`. Keep `X-Frame-Options` for old browsers. Add HSTS when `SECURE_COOKIES` is on.
3. (S, optional) Move the `style=` attributes to classes and drop `style-src 'unsafe-inline'`.
4. (L, later) Alpine CSP build and drop `'unsafe-eval'`.
A `STYLEGUIDE=false` default and static listings (also in mqn.94): `STYLEGUIDE` defaults to false in code now (the Makefile turns it on for `run`/`dev`); verify `/static/` answers 404 for directories.

**Risks of doing it.** A policy too tight silently breaks a feature (dice canvas, SSE, htmx swaps with inline handlers); report-only first removes that. Maintenance: new inline code must be a file.
**Recommendation: do steps 1 and 2 (about a day, low risk); skip step 4 until browser tests exist.** Because of `'unsafe-eval'` the gain is moderate (blocks third-party loaders, `on*` handlers, script tags, form hijack, framing), not complete; the real protection stays context-correct escaping and the existing template tests. Bead below.

## 2. Risk register (ranked by likelihood x impact)

| # | Area | Risk | L | I | Score | Mitigation | Effort | Bead |
|---|---|---|---|---|---|---|---|---|
| R1 | Quality | **No browser tests.** Alpine, htmx swaps, `live.js` (SSE, reconnect, poll fallback), dice, drag-and-drop, dialogs and the 360 px layouts are covered only by Go template/handler tests and manual QA; regressions in JS ship silently (the QA epic found most bugs by hand). | 3 | 4 | 12 | built: `make e2e` (puppeteer-core suite on the demo world: flows, stream reconnect and poll fallback, formation drag, keyboard walk, axe-core), run in CI by `.github/workflows/e2e.yml` ([testing.md](testing.md)); left: the 3D dice animation, drag in the officer panel by real pointer events | M | mostly built |
| R2 | Rules | **Rules fidelity.** Data defects found by audits (`amy` epic), many assumptions recorded as `REVIEW` (live: wounds on mounts, Monster definition; aftermath defaults), rules interpretations from a gitignored source only the curator can check; the app trusts players so a wrong rule is not caught. | 4 | 4 | 16 | keep the audit loop, list every open `REVIEW` in one place for the owner, per-unit golden tests from the curator's rulebook checks | M ongoing | yes (review) |
| R3 | Ops | **Backups.** One Postgres holds all state (companies, logs); the live log is permanent by requirement. Built: `make backup`, `make restore`, `make restore-drill` ([operations.md](operations.md)). Left: no scheduled backup or offsite copy, that is the operator's job. | 2 | 5 | 10 | cron or sidecar, retention, run the drill regularly | S | built (scheduling left) |
| R4 | Ops | **Deploy artefact built; residual gaps.** Built: `/healthz` and `/readyz`, a distroless non-root Dockerfile with digest-pinned bases, a production `compose.yaml` (plain `docker compose up`: named volume, healthchecks, no database port, read-only app filesystem, capabilities dropped), migrations under an advisory lock, `make backup`/`restore`/`restore-drill`, and a refusal to start with insecure cookies in production. Left: no CI building or scanning the image, no image signing, no metrics or alerting, one host with no redundancy, the compose setup is only exercised by hand (`docs/operations.md`). | 1 | 3 | 3 | CI job that builds the image and boots compose against `/readyz`; image scan; metrics (see R15) | S | no |
| R5 | Security | **Transport defaults.** `SECURE_COOKIES=false` and `TRUST_PROXY=false` by default, no HSTS: behind TLS an operator who forgets either has non-`Secure` cookies; with `TRUST_PROXY` wrong, the throttle and request log use the proxy's address (everyone shares one bucket) or a spoofable header. | 3 | 4 | 12 | built: production refuses to start with `SECURE_COOKIES=false` unless `ALLOW_INSECURE_COOKIES=true`, HSTS when secure, proxy recipe documented; `TRUST_PROXY` misconfiguration remains the operator's | S | yes (review) |
| R6 | Maint. | **Authorisation logic duplicated** in ~7 places (admin / organiser / owner checks across `match.go`, `aftermath.go`, `live.go`, `grant.go`, `invites.go`, web handlers) while the powers are about to change (mtu.6/7); one place missed is a privilege bug or a button that lies. | 2 | 4 | 8 | built: the permission matrix over the real services (incl. `ArrangeOfficers` with a Shire fixture) and every web write route; the checks are still spread over several files, a new action must be added to the matrix | S ongoing | built (mqn.163, mqn.159.3; see "Permission matrix" below) |
| R7 | Security | **Alpine needs `unsafe-eval` and the templates put server text into attributes**: one escaping mistake is XSS (mqn.18 was exactly this). No CSP to limit it. | 2 | 5 | 10 | built: the XSS is fixed (mqn.18), `TestHostileStringsAreEscapedOnEveryPage`, CSP in report mode; left: read the reports for a week, then `CSP_MODE=enforce` in production (`mtu.14`, `mqn.94`); `'unsafe-eval'` stays until the Alpine CSP build | S | yes |
| R8 | Supply | **Vendored JS has no manifest.** `alpine.min.js`, `htmx.min.js`, `sortable.min.js` and the three.js bundle `dice3d.js` are committed minified; their versions and sources are not recorded, so security updates are never noticed. | 3 | 3 | 9 | `static/js/VERSIONS.md` (name, version, URL, sha256), a `make vendor-check`, quarterly review | S | yes |
| R9 | Security | **Rate limiting is in memory**: login throttle (5/10 min per address+email, 20 per address) resets on restart and does not span two instances; no limits on other POSTs (account creation is CLI only, so small), password change is throttled. SSE caps (10 per user, 50 per match) are per process. | 2 | 4 | 8 | fine for one instance; note in architecture.md; move to Postgres only if scaled | S | no (doc) |
| R10 | Quality | **Accessibility and UX debt** unchecked by tools: dialogs, bottom sheets, tracker +/-, live regions for toasts, focus return, contrast (a script checks theme tokens only). Several `[ux]` beads show the pattern (mqn.109, 117, 127). | 2 | 3 | 6 | built: axe-core (WCAG A/AA, serious and critical) on the page sweep at 1280 and 360 px in both themes and a keyboard walk of a sheet, in CI (R1); the contrast script checks theme tokens; left: screen reader pass by hand | S | with R1 |
| R11 | Data | **Concurrency and migrations.** Live writes are single transactions with deltas (good); roster actions rely on `companyRepository.Update` locking; destructive migrations have no down-tested path and run automatically at start (two instances could race, goose locks per run). A bad migration has no rollback besides restore (R3). | 2 | 4 | 8 | migration dry-run against a dump in CI, test of `Down` for each file, take a backup before upgrading | M | with R3 |
| R12 | Perf | **Rules loading and caching.** Parsed rules are cached by hash triple (singleflight) with no eviction; each uploaded file and parse costs memory; a superuser can upload many (size capped by `maxUpload`). Per-zone template sets and large pages (company page with live trackers, rankings, N+1 per company in lists) are fine at table scale (tens of users) but untested at hundreds. | 2 | 3 | 6 | cap the cache (LRU of the last N hashes), a load test on the demo world, query count assertions on list pages | M | yes |
| R13 | Maint. | **Size and drift.** `internal/core` is 32k lines and `internal/web` 17k in two flat packages; docs describe "built" and "designed" parts in the same file and have drifted before (docs say built what is not and the reverse). | 4 | 2 | 8 | split `core` by domain (company, match, live, aftermath) when a change is large anyway; "docs current state" check in the closing checklist; this file reviewed per epic | L / ongoing | no |
| R14 | Security | **Uploaded rules and glossary files and user names as XSS surfaces.** All rendered through html/template; rules text keeps line breaks with `white-space: pre-line`, not HTML. Upload is superuser only, size capped, schema validated (`tools/validate_definitions.py` is the offline twin). Low, but the validator in the server is the only gate. | 2 | 3 | 6 | hostile-string test (R7); keep rules uploads superuser only | S | with R7 |
| R15 | Ops | **Logging and diagnostics.** `slog` with request logging and a panic recovery; no request id, no metrics, no error counters, CSP and SSE problems not visible. | 3 | 2 | 6 | request id in logs and error pages, a few counters (`/metrics` or log lines) | S-M | no |
| R16 | Ops | **Graceful shutdown** exists (`Shutdown` with a context) but SSE streams are long lived: confirm they close on shutdown so deploys do not hang until the timeout. `ReadHeaderTimeout` 5 s is set; no `WriteTimeout` (deliberate for SSE) or `IdleTimeout`. | 2 | 2 | 4 | test shutdown with an open stream; add `IdleTimeout` | S | no |

Notes on items that are fine today: cross-origin POSTs with cookies are refused (Go `CrossOriginProtection`), sessions are stored hashed and rotated on password change, invites are stored hashed,
`X-Frame-Options`/`nosniff`/`Referrer-Policy` are set, uploads and bodies have size caps, writes to live matches are CSRF-protected htmx POSTs and permission-checked in the service, not the template.

Status (2026-10-22): R4 (health, image, migration lock), R5 (secure by default, HSTS) and CSP steps 1 and 2 (`CSP_MODE=report|enforce`, default report) are built; the inline theme script and the two `onclick` handlers moved to files, a test keeps templates free of inline scripts. R7/R14: `TestHostileStringsAreEscapedOnEveryPage` (`internal/web/escaping_test.go`) renders about 25 page types, full and htmx, with a hostile string (quotes, backslash, angle brackets, a script tag, an `<img onerror>`, an Alpine break-out) in every free-text field of the stubbed services (names, descriptions, notes, scenario text, rules-file labels, the viewer's own name), and audits the HTML for created elements, `on*` attributes and the text verbatim inside `x-*`/`@*`/`:*` attributes. A new page type goes into `hostileWorld`; a new free-text field name into `taintedFields`. See [architecture.md](architecture.md) ("Deploying", "Content-Security-Policy").

R12: the rules cache is an LRU of 16 hash triples (`core.MaxCachedRuleSets`, builds in progress are never dropped); `cmd/server/querycount_test.go` (run inside `TestSeedWorld`, or alone with `WORLD_DATABASE_URL` on a seeded database) counts the SQL statements of the campaign, companies, matches, match, company and home page reads on the demo world and pins their budgets. It found two N+1s (companies about 9 queries each, matches 10-20 each), fixed by `companyRepository.ReadRoster` (the units and equipment of every company of a campaign in a fixed number of queries, used by `Campaigns.Companies` and the match rater); both pages now cost a fixed number of statements whatever the data.

## 3. What to do first

Built: R1 browser tests (with CI, axe, reconnect), R3 backups, R4 health, image and production compose, R5 secure defaults, R6 permission matrix, the CSP report step, R7/R14 hostile-string test. Open: read the CSP reports and switch production to enforce (`mtu.14`), R8 vendor manifest, R4 image CI and scan, R15 request ids and counters. R2 is a standing process (curator and owner).


## Permission matrix (R6, built)

Two tests, one per layer:

- `cmd/server/permissions_test.go`, `TestPermissionMatrix` (needs `TEST_DATABASE_URL`, about 80 s, run by `make test-integration`): the real
  core services over PostgreSQL. One table (`permActions`) lists the state-changing actions (invitations, campaign status, handover,
  companies, recruiting, the armoury, level-ups, treatment, matches and their end, reopen, grants, aftermath, the officer reshuffle `ArrangeOfficers` on a Shire fixture with a fallen Ranger, the live mode and Against the Odds) and says who may do each while the campaign is active, paused and finished;
  the accounts are an outsider, an administrator who is not in the campaign, a member without a company, the owner, the other player of the
  match, the organiser, an administrator and a superuser. Everyone else must be refused with the expected reason (`ErrNotFound` for
  strangers). The comment at the top of the file says how to add an action; `prep` sets up game state (money, experience, injuries, a stronger
  opponent) before the match is made.
- `internal/web/permissions_test.go`: the routes are read from `server.go`; every POST/PUT/DELETE route must redirect an anonymous visitor to
  the login page (unless listed in `publicWrites` with a reason) and every `/admin/` write must answer 404 to a user or an administrator.
  A new route needs no edit here.

Facts the matrix records: an administrator who is not a member of a campaign gets `ErrNotFound` from every campaign action except the
campaign status changes and the handover (the demo makes administrators members of every campaign); the handover is not refused for a
finished campaign in the core (the page hides it).
