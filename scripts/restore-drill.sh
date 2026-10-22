#!/usr/bin/env bash
# The restore drill: proves a backup can be restored. Restores a dump into a scratch database next to the real one, runs a smoke
# check on it and drops it. The real database is never touched.
#
#   scripts/restore-drill.sh [dump file]      (make restore-drill; default: the newest in backups/)
#
# Smoke check: the schema is there (tables), the goose version is the latest embedded migration or older (a dump from before an
# upgrade is fine, the app migrates it on start), and the main tables can be counted. Exit status 0 means the drill passed.
set -euo pipefail
cd "$(dirname "$0")/.."

file="${1:-$(ls -1t backups/bcm-*.dump 2>/dev/null | head -n 1 || true)}"
[ -n "$file" ] && [ -f "$file" ] || { echo "no dump file: run scripts/backup.sh first" >&2; exit 2; }
svc="${COMPOSE_SERVICE:-postgres}"
user="${PGUSER:-bcm}"
scratch="bcm_drill_$(date -u +%Y%m%d%H%M%S)"
psql_admin() { docker compose exec -T "$svc" psql -U "$user" -d postgres -v ON_ERROR_STOP=1 -qtA "$@"; }
psql_scratch() { docker compose exec -T "$svc" psql -U "$user" -d "$scratch" -v ON_ERROR_STOP=1 -qtA "$@"; }
cleanup() { psql_admin -c "DROP DATABASE IF EXISTS $scratch WITH (FORCE)" > /dev/null 2>&1 || true; }
trap cleanup EXIT

echo "restoring $file into scratch database $scratch"
psql_admin -c "CREATE DATABASE $scratch"
docker compose exec -T "$svc" pg_restore -U "$user" --no-owner --exit-on-error -d "$scratch" < "$file"

fail=0
check() { # name, expectation (shell test), actual
  if eval "$2"; then echo "ok    $1: $3"; else echo "FAIL  $1: $3"; fail=1; fi
}

tables="$(psql_scratch -c "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE'")"
check "tables restored" "[ $tables -ge 10 ]" "$tables"

latest="$(ls internal/postgres/migrations/*.sql | sed 's#.*/\([0-9]*\)_.*#\1#' | sort -n | tail -n 1 | sed 's/^0*//')"
version="$(psql_scratch -c "SELECT coalesce(max(version_id),0) FROM goose_db_version WHERE is_applied")"
check "schema version" "[ $version -ge 1 ] && [ $version -le $latest ]" "$version (latest migration in this checkout: $latest)"

for t in users campaigns companies matches; do
  n="$(psql_scratch -c "SELECT count(*) FROM $t")"
  check "can count $t" "[ $n -ge 0 ]" "$n rows"
done

orphans="$(psql_scratch -c "SELECT count(*) FROM companies c LEFT JOIN campaigns k ON k.id=c.campaign_id WHERE k.id IS NULL")"
check "no company without a campaign" "[ $orphans -eq 0 ]" "$orphans"

if [ $fail -eq 0 ]; then echo "restore drill passed"; else echo "restore drill FAILED" >&2; fi
exit $fail
