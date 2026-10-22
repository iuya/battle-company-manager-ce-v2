#!/usr/bin/env bash
# Restores a dump over the application database, replacing its contents.
#
#   CONFIRM=yes scripts/restore.sh backups/bcm-....dump        (make restore FILE=... CONFIRM=yes)
#
# Stop the app first (docker compose stop app, or the server process): it keeps connections and writes. Take a fresh backup
# before restoring over live data. With PGDUMP_LOCAL=1 the host's pg_restore and DATABASE_URL are used. See docs/operations.md.
set -euo pipefail
cd "$(dirname "$0")/.."

file="${1:-}"
[ -n "$file" ] && [ -f "$file" ] || { echo "usage: CONFIRM=yes $0 <dump file>" >&2; exit 2; }
[ "${CONFIRM:-}" = "yes" ] || { echo "this replaces the contents of the database; run again with CONFIRM=yes" >&2; exit 2; }
svc="${COMPOSE_SERVICE:-postgres}"
user="${PGUSER:-bcm}"
db="${PGDATABASE:-bcm}"

if [ "${PGDUMP_LOCAL:-}" = "1" ]; then
  : "${DATABASE_URL:?DATABASE_URL is needed with PGDUMP_LOCAL=1}"
  pg_restore --clean --if-exists --no-owner --exit-on-error -d "$DATABASE_URL" "$file"
else
  docker compose exec -T "$svc" pg_restore -U "$user" --clean --if-exists --no-owner --exit-on-error -d "$db" < "$file"
fi
echo "restored $file; start the app again (it applies any migration the dump is missing)"
