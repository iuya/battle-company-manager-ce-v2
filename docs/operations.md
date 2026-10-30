# Operations: backups, restore, upgrades

For whoever runs a server: the first start, the published image, the Content-Security-Policy modes, backups and restore, and what keeps one failure from taking the app down.

All state of the app (accounts, sessions, campaigns, companies, matches, the permanent live log, the uploaded rules files) is in one
PostgreSQL database; the shipped rules files and the code are in the image or the repository. A backup is therefore a dump of that database.
Configuration, health checks and the migration command are in [architecture.md](architecture.md) ("Configuration", "Deploying").

## First start (production)

Plain `docker compose up -d` (the default `compose.yaml`, project `bcm`) is the production setup; `compose.dev.yaml` is the throwaway development database.

```
cp .env.example .env                      # set POSTGRES_PASSWORD (openssl rand -hex 24); everything else has a default
docker compose up -d --build --wait       # make up: builds the image, starts PostgreSQL on the named volume, then the app
curl -s localhost:8080/readyz             # ok
make prod-user EMAIL=you@example.org NAME="You" ROLE=superuser     # the first account (there is no sign-up page); asks for the password
```

`make prod-user` runs `docker compose run --rm app user create --email ... --name ...` (the password is read from the terminal, or from one line of stdin with `-T`) and, with `ROLE=`,
`app user grant --role superuser --create-campaigns`. The app applies the migrations when it starts (under an advisory lock) and `restart: unless-stopped` brings it back after a reboot.
Without `POSTGRES_PASSWORD` compose stops with a message; there is no default password. The database has no published port (only the app reaches it); the app is published on
`APP_BIND:APP_PORT` (default `127.0.0.1:8080`, for the reverse proxy on the same host; `APP_BIND=0.0.0.0` to listen on every interface). The app container runs with a read-only root
filesystem, all capabilities dropped and `no-new-privileges`.

**TLS and cookies.** Production expects HTTPS in front: a reverse proxy terminates TLS and forwards to the app port. Session, theme and glossary cookies are `Secure` and `Strict-Transport-Security`
is sent (`SECURE_COOKIES=true`, the compose default and the setting behind a TLS proxy). The server **refuses to start** in production with `SECURE_COOKIES=false` unless `ALLOW_INSECURE_COOKIES=true`
is also set (a plain-http try-out or private network; uncomment that line in `compose.yaml`). Set `TRUST_PROXY=true` only when the proxy sets `X-Forwarded-For` itself. `CSP_MODE` stays `report` until you switch it (see "Content-Security-Policy" below).
The variables beyond `POSTGRES_PASSWORD`, `APP_BIND`, `APP_PORT` and `SECURE_COOKIES` are commented in the app's `environment:` in `compose.yaml`; uncomment the ones you change (they then read `.env`).

**Configuration reference.** Every variable the server reads is listed (with default) in `compose.yaml`, `compose.dev.yaml` and `.env.example`; `make check-env-docs` fails when one is missing.
`MAX_CACHED_RULESETS` (default 16, at least 1) caps how many parsed rule sets (a few MB each, one per distinct hash triple of rules files) stay in memory, least recently used dropped first;
raise it only if a superuser uploads many rules files that campaigns pin, lower it on a very small host.

The backup, restore and drill scripts use the default compose file, i.e. production, and need `POSTGRES_PASSWORD` visible to compose (it is, from `.env`). For the development database:
`make backup COMPOSE_FILE=compose.dev.yaml`.

## Published image

Every release tag builds the image in CI and publishes it to the GitHub Container Registry: `ghcr.io/iuya/battle-company-manager-ce-v2`. It is the same `Dockerfile` as the local build
(distroless, non-root uid 65532, no secrets baked in), for `linux/amd64` and `linux/arm64`.

When the image is published, the workflow also creates the **GitHub Release** of the tag (title = the tag, notes = the body of the tagged commit and the image reference; a tag with a hyphen is a pre-release and never the latest). Every named tag `vX.Y.Z` has a release page.

| Tag | Moves | Example |
|---|---|---|
| `1.1.0` | never (one release) | pin this in production |
| `1.1` | to the newest patch of that minor | |
| `1` | to the newest release of that major (not published below 1.0.0) | |
| `latest` | to the newest stable release (pre-releases such as `v1.2.0-rc.1` get only their full tag) | |

The image carries the OCI labels `source`, `version`, `revision` (the commit), `licenses`, `title` and `description`.

**Use it.** In `compose.yaml` the app service has two alternative blocks: `build: .` / `image: bcm:latest` (active, builds from the checkout) and a commented
`image: ghcr.io/...:latest` with `pull_policy: always`. Comment the first pair, uncomment the second (pin `:1.1.0` instead of `:latest` if you want), then
`docker compose pull app && docker compose up -d --wait`. The rest (database, `.env`, `make prod-user`) is unchanged; `make up` passes `--build`, which is harmless for an image-only service.
Without compose: `docker pull ghcr.io/iuya/battle-company-manager-ce-v2:1.1.0`. To go back to the local build, swap the lines and run `docker compose up -d --build`.

**Release.** `.github/workflows/release-image.yml` runs when a tag `v*` is pushed (`git tag v1.1.0 && git push origin v1.1.0`). Jobs, in order:
`e2e` (the browser tests of `e2e.yml`, called as a reusable workflow: four parallel groups and a final `e2e` job; any failing group stops the release), then `build`, a matrix of two jobs on native runners
(no QEMU): `linux/amd64` on `ubuntu-26.04` and `linux/arm64` on `ubuntu-26.04-arm` (free for public repositories). Each builds its image into its runner, smoke-tests it (starts it read-only against a PostgreSQL service
container, `/readyz`, `/healthz`, the login page, the built-in healthcheck, the architecture), and only then pushes it by digest (no tag) with the `GITHUB_TOKEN` (permissions: contents read, packages write) and uploads the
digest as an artifact. The `merge` job (tags only) downloads both digests and creates the multi-platform manifest with the tags of the table above (`docker buildx imagetools create`), then inspects it. The build cache (gha) is kept per platform. Running the workflow by hand from the Actions tab is a dry run: the browser tests and both builds with their smoke tests, no login, no push, and `merge` is skipped.

**Make the package public, once.** The first push creates the package private, and visibility cannot be changed through the API or CI. On GitHub: the repository's *Packages* >
the package > *Package settings* > *Change visibility* > Public (also *Manage Actions access* should list this repository with Write). After that anyone can pull without logging in.

**Verify a pushed image.**
```
docker buildx imagetools inspect ghcr.io/iuya/battle-company-manager-ce-v2:1.1.0     # two platforms: linux/amd64 and linux/arm64
docker pull ghcr.io/iuya/battle-company-manager-ce-v2:1.1.0 && docker inspect --format '{{json .Config.Labels}}' ghcr.io/iuya/battle-company-manager-ce-v2:1.1.0
```

**Rollback.** Pin the previous full tag in `compose.yaml` (`image: ...:1.0.3`), `docker compose pull app && docker compose up -d --wait`. Migrations are forward-only: if the newer version
migrated the database, restore the backup taken before the upgrade (Restore, below) rather than only swapping the image. A bad tag can be removed on the package page (*Versions* > delete);
fix forward with a new patch tag rather than re-pushing the same tag.

## Running the published image on a server

`deploy/` is a kit for one Linux server that always runs the newest stable release without a checkout of the source. It needs Docker with the compose plugin, `curl`, `flock` and systemd; nothing else.

| File | What it is |
|---|---|
| `deploy/compose.prod.yaml` | Caddy, the app and PostgreSQL. Installed as `/opt/bcm/compose.yaml`. Only Caddy publishes ports (80, 443 and 443/udp); the app and the database sit on internal networks. The app runs read-only, with every capability dropped, memory, CPU and process limits, `TRUST_PROXY=true` and `SECURE_COOKIES=true`; container logs rotate (10 MB x 5). |
| `deploy/Caddyfile` | Automatic HTTPS for `SITE_ADDRESS`, HTTP redirected to HTTPS, a 17 MB request cap, the client address handed to the app as `X-Forwarded-For` (replacing whatever the client sent), live streams unbuffered, no `Server` header, admin API off. |
| `deploy/env.example` | The `/opt/bcm/.env` the installer writes: `SITE_ADDRESS`, a `POSTGRES_PASSWORD` generated on the host, `APP_IMAGE` (kept by the updater), `CSP_MODE`. Mode 0600, never committed. |
| `deploy/install.sh` | Copies the kit and the backup scripts into `/opt/bcm` (`BCM_DIR` changes it), creates `.env` once, enables the timers. `sudo SITE_ADDRESS=bcm.example.org deploy/install.sh` |
| `deploy/bcm-update.sh` + `systemd/bcm-update.{service,timer}` | The pull-based updater, every 15 minutes: asks ghcr.io for the digest of the `latest` tag (`UPDATE_TAG` follows another), and when it differs takes a dump, writes `APP_IMAGE=...@sha256:<digest>` into `.env`, pulls, starts, waits for the health check and probes `/readyz` through Caddy. A release that does not become ready is replaced by the previous digest. `UPDATE_DIGEST=sha256:...` deploys an exact digest, `CHECK_ONLY=1` only prints. State is in `/opt/bcm/state` (`current-image`, `previous-image`). The server is therefore always on an immutable digest, never on a moving tag. |
| `deploy/bcm-backup.sh` + `systemd/bcm-backup.{service,timer}` | Nightly 03:15 dump (14 kept on disk), then the optional off-site hook `/opt/bcm/offsite-hook.sh <dump>`; a failing hook turns the unit red. |
| `systemd/bcm-restore-drill.{service,timer}` | The monthly restore drill of `scripts/restore-drill.sh` (below). |
| `deploy/memcheck.sh` | Memory, swap, per-container usage and out-of-memory readings (see Memory below). |
| `deploy/bootstrap-admin.sh` | The first account: `sudo /opt/bcm/bootstrap-admin.sh you@example.org "Your Name"`. The password is typed at the prompt; it is in no argument, file or log. |
| `deploy/aws/` | Amazon Web Services specifics: `host-setup.sh` prepares an Amazon Linux 2023 instance (Docker, swap, security updates, then `install.sh`) and `offsite-s3.sh` is an off-site hook that copies dumps to a private S3 bucket with the instance role. |

**First start.** Point a DNS name at the server (Let's Encrypt validates it), open 80 and 443 in the firewall and nothing else, then run `deploy/install.sh` as above (on AWS, `deploy/aws/host-setup.sh`, which calls it), create the first account with `bootstrap-admin.sh`, and open `https://<name>/`. The first update run starts everything; follow it with `journalctl -u bcm-update`. Check the state with `cat /opt/bcm/state/current-image`, `docker compose -f /opt/bcm/compose.yaml ps` and `systemctl list-timers 'bcm-*'`.

**Memory.** The compose file is sized for a 1 GB server: container limits caddy 64 MB, app 256 MB (`GOMEMLIMIT=200MiB`, `MAX_CACHED_RULESETS=4`) and PostgreSQL 320 MB (`shared_buffers=64MB`, `work_mem=2MB`, `max_connections=20`), 640 MB in all, and `host-setup.sh` adds a 1 GB swap file with `vm.swappiness=10` as a safety net. `/opt/bcm/memcheck.sh` prints memory and swap, a `docker stats` snapshot, container restarts and any out-of-memory kills from the kernel log. On a bigger server raise the three limits and PostgreSQL's settings together.

**Rollback.** `UPDATE_DIGEST=$(cat /opt/bcm/state/previous-image | sed 's/.*@//') /opt/bcm/bcm-update.sh` goes back to the previous release; stop the update timer first (`systemctl stop bcm-update.timer`) or the next run moves forward again. If the newer release migrated the database and the old one refuses it, stop the app and restore the dump taken just before the update (`/opt/bcm/backups`, newest first): see Restore below.

**Tested locally.** `make test-backup-restore` (`scripts/test-backup-restore.sh`) lays out a scratch directory like `/opt/bcm`, starts PostgreSQL there under its own compose project, migrates it, backs up, changes the data, restores, and runs the drill; it removes everything afterwards and needs Docker and Go.

## Content-Security-Policy: report mode, then enforce

Production starts in `CSP_MODE=report` (the default): the browser loads everything, sends each violation to `POST /csp-report` and the app logs one line. Before the switch the browser suite already ran
the whole app under the blocking policy with no violation (`make e2e-enforce`); the week of report mode covers what a suite cannot: real browsers and real data.

**What to read after a week** (the app logs to stdout):

```
docker compose logs --since 168h app | grep -E "csp violation|csp reports dropped"
docker compose logs --since 168h app | grep "csp violation" | grep -v chrome-extension | sed 's/.*directive=//' | sort | uniq -c | sort -rn
```

- **No line, or only lines whose `source=` starts with `chrome-extension://`, `moz-extension://` or `safari-extension://` (a user's extension, not the app):** ready.
- **A line with `source=` a `/static/js/...` file or `blocked=inline` or `blocked=eval` on `directive=script-src...`:** something the policy would break (`eval` is allowed by `'unsafe-eval'`, so `eval` should not appear). Fix it (move the code to a file under `static/js`) and run another week.
- **`directive=img-src`, `font-src`, `connect-src` or `style-src-elem` with a `blocked=` URL of the app's own origin or `data:`/`blob:`:** a source the policy omits; add it to `cspPolicy` in `internal/web/hardening.go`, with a test, before enforcing.
- **`csp reports dropped count=N`:** the 60 per minute limit cut a flood; one browser, one broken page. Look at the first lines around it.
- Every line shows `disposition=report`; `document=` is the page, `line=` the position in `source=`. Reports from a reverse proxy that injects scripts (analytics, a banner) are also visible here: the proxy's script would be blocked after the switch.

**The switch** is one line in the `environment:` block of `compose.yaml`, among the active settings:

```
      CSP_MODE: enforce
```

then `docker compose up -d app` (a restart; no migration). The same policy now blocks and `Content-Security-Policy` replaces `Content-Security-Policy-Report-Only`; reports keep arriving (`disposition=enforce`), so read the same log for a day.
Rollback is the same line with `report` (or `off`) and the same command: nothing else depends on it. The default in the code and in `.env.example` is `report`.

## Backup

```
make backup               # backups/bcm-<UTC timestamp>.dump, keeps the newest 14 (KEEP=30 make backup)
scripts/backup.sh DIR     # the same into another directory
```

`scripts/backup.sh` runs `pg_dump --format=custom --no-owner` inside the compose `postgres` service (nothing to install on the host), writes to a
`.partial` file, checks that `pg_restore --list` can read it, renames it, and prunes the oldest beyond `KEEP`. For a database that is not the compose one
(managed, remote) use the host's client tools: `PGDUMP_LOCAL=1 DATABASE_URL=postgres://... scripts/backup.sh`. The dump is taken from a consistent snapshot
while the app keeps running; no downtime is needed. `backups/` is gitignored.

**Schedule and retention.** Daily is enough for a table-top app (a day of play lost at worst).
Example crontab on the host, 03:15 every day, 14 daily dumps kept:

```
15 3 * * *  cd /srv/battle-companies && KEEP=14 scripts/backup.sh >> backups/backup.log 2>&1
```

Keep, as a minimum, 14 dailies plus one dump a month for a year (copy the first of each month to a `monthly/` directory with a longer `KEEP`). **A backup on the same disk
is not a backup**: copy `backups/` off the machine (`rsync`, `rclone`, object storage with versioning) as part of the cron line, and give the copy its own retention.
Dumps contain password hashes and session tokens' hashes: treat them like the database (restrict permissions, encrypt the off-site copy).

**Before every upgrade** run `make backup` (migrations run on start, and a bad migration is undone only by a restore). With `MIGRATE_ON_START=false` do it between stopping the old
version and running `server migrate`.

## Restore

```
make restore FILE=backups/bcm-20261021T031500Z.dump CONFIRM=yes
```

Stop the app first (`docker compose stop app`, or the server process): it holds connections and writes. The script replaces the database's contents
(`pg_restore --clean --if-exists`); start the app afterwards and it applies any migration the dump predates. To go back from a failed upgrade: stop the new version, restore the
pre-upgrade dump, start the old version. Take a backup of the current state first if there is anything in it worth keeping.

## Restore drill

An untested backup is a hope. `make restore-drill` (`scripts/restore-drill.sh [dump]`) restores the newest dump (or the given one) into a scratch database `bcm_drill_<time>` next to the real one,
runs a smoke check and drops the scratch database, whatever the outcome; the real database is not touched. The check: the tables are there, the schema version is between 1 and the latest migration of the checkout,
users, campaigns, companies and matches can be counted, and no company is without its campaign. Output ends with `restore drill passed` and a zero exit status, or `restore drill FAILED`.

Run it after the first backup is set up, after every change of the backup procedure or of PostgreSQL's major version, and monthly from cron (`15 4 1 * * ... make restore-drill`) so a silent breakage shows up.
For a deeper check, point a staging copy of the app at the scratch database: `DATABASE_URL=...bcm_drill_x go run ./cmd/server` (comment out the `DROP` in the script's trap, or restore by hand with `make restore` into a staging database).

## Robustness: what keeps one failure from taking the app down

A panic in an HTTP handler is caught by the outermost middleware (`recoverer`: logs the stack, answers 500, keeps serving). Everything that runs outside a request
needs its own guard, because an unrecovered panic in any goroutine ends the process.

- **Goroutines.** There are four. The rule-set build (`core.Rules.For`) and the live hub (`LiveHub.Start`) run under `core.safely`: a panic is logged with its stack, the
  build answers an error (and is not cached), the hub restarts the Postgres feed after a second and the restart marks every stream changed. The rule-set build has a deadline (`RULES_BUILD_TIMEOUT`, default 60 s, at least 1 s): its database reads and the checks between the parsing stages stop with it; the parsers take no context, so a build that is still inside one at the deadline is given up on: the waiters get an error wrapping `context.DeadlineExceeded` (503), the failure is not cached (the next request builds again) and the parser goroutine is left to end by itself. At most 4 such stuck builds are tolerated (`Rules.Stuck`); further ones are refused at once (`ErrBuildsStuck`) instead of piling up goroutines. The match page cards
  (`internal/web/match_cards.go`) recover and fall back to the lazy fetch. `ListenAndServe` is handled by net/http. There are no cleanup loops: the session cache, the login
  throttle, the memos and the rule-set cache are bounded by size and prune on access.
- **LISTEN/NOTIFY.** One connection outside the pool. Connect and `LISTEN` have a 15 s deadline; an idle connection is pinged every 30 s; any failure reconnects with
  1 s to 30 s backoff, logs a warning and re-syncs all streams. The hub never blocks on a subscriber: each stream holds one coalesced batch and a one-slot wake channel
  (so a slow browser costs one batch, never a backlog), publish holds the hub lock only for non-blocking work, and a stream frees its slot when its request ends
  (`context.AfterFunc`). Caps: 10 streams per user, 50 per match.
- **Server limits** (`cmd/server/main.go`): ReadHeaderTimeout 5 s, ReadTimeout 60 s, WriteTimeout 120 s, IdleTimeout 120 s, MaxHeaderBytes 64 KB. The live stream (SSE) sets
  a write deadline per write and lifts the read deadline, so it is not cut (`TestLiveStreamOutlivesTheServerTimeouts`). Every request but the live stream has a context deadline (`REQUEST_TIMEOUT`, default 30 s, `0` = off; `requestTimeout` in `internal/web/middleware.go`): WriteTimeout only cuts the connection and never cancels `r.Context()`, this does, so queries and rule computation stop when the time is up or the client leaves; the handler answers 503 (`serverError` maps context errors). Request bodies: 1 MiB for everything that is not a
  multipart upload (`limitBody`); uploads set their own limit (`maxUpload`, 16 MiB) before parsing and `core.Rulesets` streams the JSON once before validating it: more than 1 000 000 values or more than 64 levels is refused with a 422 (the shipped files have about 40 000 values and 11 levels; validating costs about 40 times the file size in memory). `TestHostileUploadsAnswer4xx` sends the three upload handlers malformed, truncated, huge, deeply nested, oddly typed and badly encoded files and multipart bodies: always a 4xx with a message. Shutdown on SIGINT/SIGTERM: the hub closes its streams, in-flight requests get 10 s.
- **Database.** Every pooled connection gets `statement_timeout=60s` and `idle_in_transaction_session_timeout=60s` (a setting in `DATABASE_URL` wins; migrations run before
  the limits apply), so a stuck request cannot hold the match row lock or a pool connection for ever. Every transaction begins with `defer tx.Rollback`. Handlers pass
  the request context, so a client that leaves cancels its queries.
- **Boot versus run.** Configuration errors, a failed migration, a broken embedded asset table (`staticFiles`) and an unparsable template fail at start (`NewServer`,
  `run`), never at the first request. At run time nothing calls `log.Fatal`; the `panic` calls in `cmd/server/world*.go` belong to the `seed` command.
- **Live match actions.** `TestLiveActionsSurviveExtremeValues` (cmd/server, needs `TEST_DATABASE_URL`; about a minute with the seeding, `SWEEP_FULL=1` several times longer) is the sweep that puts ONE extreme but plausible value at a time (wounds 2^31, negatives, 0, empty, overflowing numbers, the unit of another company, match or campaign, wrong parts, reasons and fields, oversized or NUL notes) into an otherwise valid form of every live POST (each action kind, undo, courage roll, objective and MVP votes, secret objectives, the end-of-match steps), also repeated and against the wrong match, an ended match and a finished campaign. No answer is a 5xx or an empty 4xx, and after every kind the folded state is checked: every tracker within 0..start, no negative or overflowing Break Point counter, the log readable. The service validates all bounds (`LiveState.Check`: trackers 0..start, wounds caused 1 to 9, Fate within what is left, Break correction -9..9 and never below one starting model, turns in sequence); typed courage dice outside 1 to 6 are an `ErrInvalidLive` (422).
- **Tests.** `TestMalformedInputNeverCrashesAHandler` (cmd/server, needs `TEST_DATABASE_URL`; about 40 s with the seeding, `SWEEP_FULL=1` tries 23 values and takes longer) sends every route, read from `server.go`, hostile values for every
  form field the handlers and templates know and hostile path ids against the demo world: no 5xx, no logged panic. Run the packages you touch with `go test -race`.

## Recovery point

Point-in-time recovery (WAL archiving) is not set up: the recovery point is the last dump. If a day of loss becomes unacceptable, move to a managed PostgreSQL with PITR or add `pgBackRest`/WAL-G; the app needs no change.
