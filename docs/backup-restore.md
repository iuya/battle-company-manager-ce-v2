# Backup, restore and the restore drill

For whoever runs a server and has to keep the data safe: what to back up, how to dump and restore the PostgreSQL database, and how to prove that a dump restores.

- [What to back up](#what-to-back-up)
- [Backup scripts and the compose file](#backup-scripts-and-the-compose-file)
- [Backup](#backup)
- [Restore](#restore)
- [Restore drill](#restore-drill)
- [Tested locally](#tested-locally)
- [Recovery point](#recovery-point)

## What to back up

All state of the app is in one PostgreSQL database:

- accounts and sessions
- campaigns, companies and matches
- the permanent live log
- the uploaded rules files

The shipped rules files and the code are in the image or the repository.
A backup is therefore a dump of that database.

Configuration, health checks and the migration command are in [architecture.md](architecture.md) ("Configuration", "Deploying").

### Backup scripts and the compose file

The backup, restore and drill scripts use the default compose file, which is production.
They need `POSTGRES_PASSWORD` visible to compose (it is, from `.env`).
For the development database: `make backup COMPOSE_FILE=compose.dev.yaml`.

## Backup

```bash
make backup               # backups/bcm-<UTC timestamp>.dump, keeps the newest 14 (KEEP=30 make backup)
scripts/backup.sh DIR     # the same into another directory
```

`scripts/backup.sh`:

1. Runs `pg_dump --format=custom --no-owner` inside the compose `postgres` service (nothing to install on the host).
2. Writes to a `.partial` file and checks that `pg_restore --list` can read it.
3. Renames the file and prunes the oldest beyond `KEEP`.

For a database that is not the compose one (managed, remote), use the host's client tools:
`PGDUMP_LOCAL=1 DATABASE_URL=postgres://... scripts/backup.sh`.

The dump is taken from a consistent snapshot while the app keeps running; no downtime is needed.
`backups/` is gitignored.

**Schedule and retention.** Daily is enough for a table-top app (a day of play lost at worst).
Example crontab on the host, 03:15 every day, 14 daily dumps kept:

```cron
15 3 * * *  cd /srv/battle-companies && KEEP=14 scripts/backup.sh >> backups/backup.log 2>&1
```

Keep, as a minimum, 14 dailies plus one dump a month for a year.
Copy the first of each month to a `monthly/` directory with a longer `KEEP`.

**A backup on the same disk is not a backup.**
Copy `backups/` off the machine (`rsync`, `rclone`, object storage with versioning) as part of the cron line, and give the copy its own retention.

Dumps contain password hashes and session token hashes.
Treat them like the database: restrict permissions and encrypt the off-site copy.

**Before every upgrade** run `make backup`.
Migrations run on start, and a bad migration is undone only by a restore.
With `MIGRATE_ON_START=false`, take the backup between stopping the old version and running `server migrate`.

## Restore

```bash
make restore FILE=backups/bcm-20261021T031500Z.dump CONFIRM=yes
```

Stop the app first (`docker compose stop app`, or the server process): it holds connections and writes.
The script replaces the database's contents (`pg_restore --clean --if-exists`).
Start the app afterwards; it applies any migration the dump predates.

To go back from a failed upgrade:

1. Stop the new version.
2. Restore the pre-upgrade dump.
3. Start the old version.

Take a backup of the current state first if there is anything in it worth keeping.

## Restore drill

An untested backup is a hope.
`make restore-drill` (`scripts/restore-drill.sh [dump]`) restores the newest dump (or the given one) into a scratch database `bcm_drill_<time>` next to the real one.
It runs a smoke check and drops the scratch database whatever the outcome; the real database is not touched.

The smoke check verifies that:

- the tables are there;
- the schema version is between 1 and the latest migration of the checkout;
- users, campaigns, companies and matches can be counted;
- no company is without its campaign.

The output ends with `restore drill passed` and a zero exit status, or `restore drill FAILED`.

Run the drill:

- after the first backup is set up;
- after every change of the backup procedure or of PostgreSQL's major version;
- monthly from cron (`15 4 1 * * ... make restore-drill`), so a silent breakage shows up.

For a deeper check, point a staging copy of the app at the scratch database: `DATABASE_URL=...bcm_drill_x go run ./cmd/server`.
Comment out the `DROP` in the script's trap first, or restore by hand with `make restore` into a staging database.


## Tested locally

`make test-backup-restore` (`scripts/test-backup-restore.sh`) needs Docker and Go.
It lays out a scratch directory like `/opt/bcm`, starts PostgreSQL there under its own compose project and migrates it.
Then it backs up, changes the data, restores and runs the drill.
It removes everything afterwards.


## Recovery point

Point-in-time recovery (WAL archiving) is not set up: the recovery point is the last dump.
If a day of loss becomes unacceptable, move to a managed PostgreSQL with PITR or add `pgBackRest`/WAL-G; the app needs no change.

See also: [operations.md](operations.md) (first start), [deploy-server.md](deploy-server.md) (the nightly timer and off-site hook), [releases.md](releases.md) (rolling back an image).
