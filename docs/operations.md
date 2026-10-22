# Operations: backups, restore, upgrades

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
is also set (a plain-http try-out or private network; uncomment that line in `compose.yaml`). Set `TRUST_PROXY=true` only when the proxy sets `X-Forwarded-For` itself. `CSP_MODE` stays `report` for now.
The variables beyond `POSTGRES_PASSWORD`, `APP_BIND`, `APP_PORT` and `SECURE_COOKIES` are commented in the app's `environment:` in `compose.yaml`; uncomment the ones you change (they then read `.env`).

**Configuration reference.** Every variable the server reads is listed (with default) in `compose.yaml`, `compose.dev.yaml` and `.env.example`; `make check-env-docs` fails when one is missing.
`MAX_CACHED_RULESETS` (default 16, at least 1) caps how many parsed rule sets (a few MB each, one per distinct hash triple of rules files) stay in memory, least recently used dropped first;
raise it only if a superuser uploads many rules files that campaigns pin, lower it on a very small host.

The backup, restore and drill scripts use the default compose file, i.e. production, and need `POSTGRES_PASSWORD` visible to compose (it is, from `.env`). For the development database:
`make backup COMPOSE_FILE=compose.dev.yaml`.

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

## Not covered

Point-in-time recovery (WAL archiving) is not set up: the recovery point is the last dump. If a day of loss becomes unacceptable, move to a managed PostgreSQL with PITR or add `pgBackRest`/WAL-G; the app needs no change.
