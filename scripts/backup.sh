#!/usr/bin/env bash
# Dumps the database to backups/bcm-<UTC timestamp>.dump (pg_dump custom format, compressed) and keeps the newest KEEP dumps.
#
#   scripts/backup.sh [DIR]           (make backup)
#
# Default: runs pg_dump inside the compose "postgres" service (no client tools needed on the host). With PGDUMP_LOCAL=1 it runs the
# host's pg_dump against DATABASE_URL instead (a managed or remote database). Environment: KEEP (default 14), COMPOSE_SERVICE
# (default postgres), PGUSER/PGDATABASE for the compose mode (default bcm/bcm). See docs/operations.md.
set -euo pipefail
cd "$(dirname "$0")/.."

dir="${1:-backups}"
keep="${KEEP:-14}"
svc="${COMPOSE_SERVICE:-postgres}"
user="${PGUSER:-bcm}"
db="${PGDATABASE:-bcm}"
mkdir -p "$dir"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
out="$dir/bcm-$stamp.dump"
tmp="$out.partial"
trap 'rm -f "$tmp"' EXIT

if [ "${PGDUMP_LOCAL:-}" = "1" ]; then
  : "${DATABASE_URL:?DATABASE_URL is needed with PGDUMP_LOCAL=1}"
  pg_dump --format=custom --no-owner "$DATABASE_URL" > "$tmp"
else
  docker compose exec -T "$svc" pg_dump -U "$user" --format=custom --no-owner "$db" > "$tmp"
fi

# A dump that cannot be listed is not a backup.
if [ "${PGDUMP_LOCAL:-}" = "1" ]; then
  pg_restore --list "$tmp" > /dev/null
else
  docker compose exec -T "$svc" pg_restore --list < "$tmp" > /dev/null
fi
mv "$tmp" "$out"
echo "backup written: $out ($(wc -c < "$out" | tr -d ' ') bytes)"

# Retention: the newest $keep dumps stay (a copy off the machine is on you, see docs/operations.md).
ls -1t "$dir"/bcm-*.dump 2>/dev/null | tail -n +"$((keep + 1))" | while read -r old; do
  echo "removing old backup: $old"
  rm -f "$old"
done
