#!/usr/bin/env bash
# Local test of the backup, restore and restore-drill scripts as they run on a server: needs Docker and Go, nothing else, and touches no real data.
#
#   scripts/test-backup-restore.sh          (make test-backup-restore)
#
# It lays out a scratch directory like /opt/bcm (compose.yaml from deploy/compose.prod.yaml, scripts/), starts only PostgreSQL there under
# its own project name with throwaway credentials, applies the migrations, writes a marker row, takes a backup, changes the data, restores,
# checks that the data is back as it was at the backup, and runs the restore drill. The scratch project and volume are removed at the end.
set -euo pipefail
unset GOROOT # an inherited GOROOT can name another Go version than go.mod asks for (see the Makefile)
root="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
export COMPOSE_PROJECT_NAME="bcm-restoretest-$$"
export POSTGRES_PASSWORD="restoretest$$"
export APP_IMAGE="unused:test"
cleanup() { (cd "$work" && docker compose down -v --remove-orphans > /dev/null 2>&1) || true; rm -rf "$work"; }
trap cleanup EXIT

mkdir -p "$work/scripts"
cp "$root/deploy/compose.prod.yaml" "$work/compose.yaml"
cp "$root/deploy/Caddyfile" "$work/Caddyfile"
cp "$root/scripts/backup.sh" "$root/scripts/restore.sh" "$root/scripts/restore-drill.sh" "$work/scripts/"
cat > "$work/publish.yaml" <<'EOF'
services:
  postgres:
    ports: ["127.0.0.1::5432"]
    networks: [edge]   # the production network of the database is internal (no published ports); the test publishes one on loopback
EOF
export COMPOSE_FILE="$work/compose.yaml:$work/publish.yaml"
cd "$work"

step() { echo "== $*"; }
psql_bcm() { docker compose exec -T postgres psql -U bcm -d bcm -v ON_ERROR_STOP=1 -qtA "$@"; }

step "start PostgreSQL (project $COMPOSE_PROJECT_NAME)"
docker compose up -d --wait postgres
port="$(docker compose port postgres 5432 | sed 's/.*://')"

step "apply the migrations of this checkout"
(cd "$root" && DATABASE_URL="postgres://bcm:$POSTGRES_PASSWORD@127.0.0.1:$port/bcm?sslmode=disable" go run ./cmd/server migrate)

step "marker row, backup"
psql_bcm -c "CREATE TABLE restore_marker (v text); INSERT INTO restore_marker VALUES ('before')"
KEEP=3 scripts/backup.sh backups
dump="$(ls -1t backups/bcm-*.dump | head -n 1)"

step "change the data after the backup"
psql_bcm -c "INSERT INTO restore_marker VALUES ('after'); DROP TABLE goose_db_version CASCADE" > /dev/null

step "restore"
CONFIRM=yes scripts/restore.sh "$dump"
got="$(psql_bcm -c "SELECT string_agg(v, ',') FROM restore_marker")"
[ "$got" = "before" ] || { echo "FAIL: marker is '$got', expected 'before'" >&2; exit 1; }
[ "$(psql_bcm -c "SELECT count(*) FROM goose_db_version")" -ge 1 ] || { echo "FAIL: schema version table not restored" >&2; exit 1; }

step "restore drill"
scripts/restore-drill.sh "$dump"
echo "backup, restore and restore drill: ok"
