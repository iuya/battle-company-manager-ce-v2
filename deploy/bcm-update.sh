#!/usr/bin/env bash
# Pull-based updater: moves the server to the newest stable release, pinned by image digest.
#
#   /opt/bcm/bcm-update.sh                 resolve the digest of the tag (default "latest") in the registry and deploy it if it changed
#   UPDATE_DIGEST=sha256:<64 hex> ...      deploy exactly this digest (manual deploy, or a rollback to state/previous-image)
#   UPDATE_TAG=1.2.0 ...                   follow another tag than "latest"
#   CHECK_ONLY=1 ...                       print what would change and stop
#
# Order: lock, resolve, dump the database, write APP_IMAGE into .env, pull, `up --wait`, probe /readyz through Caddy. A failed start puts
# the previous digest back. Migrations are forward-only: if the new release migrated the database and the old image refuses it, restore the
# dump printed in the message (docs/operations.md, "Rollback"). Registry access is anonymous (the package is public); nothing here reads a secret.
set -euo pipefail
cd "$(dirname "$0")"

repo="${IMAGE_REPO:-ghcr.io/iuya/battle-company-manager-ce-v2}"
tag="${UPDATE_TAG:-latest}"
state="${STATE_DIR:-./state}"
mkdir -p "$state" backups
exec 9>"$state/update.lock"
if command -v flock > /dev/null; then # util-linux, present on servers; a laptop without it can still use CHECK_ONLY
  flock -n 9 || { echo "another update is running" >&2; exit 0; }
else
  echo "warning: flock not found, running without the lock" >&2
fi

[[ "$tag" =~ ^[A-Za-z0-9._-]+$ ]] || { echo "bad UPDATE_TAG" >&2; exit 2; }
path="${repo#ghcr.io/}"
[ "$path" != "$repo" ] || { echo "IMAGE_REPO must be on ghcr.io" >&2; exit 2; }

digest="${UPDATE_DIGEST:-}"
if [ -z "$digest" ]; then
  token="$(curl -fsS "https://ghcr.io/token?service=ghcr.io&scope=repository:$path:pull" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')"
  [ -n "$token" ] || { echo "no registry token" >&2; exit 1; }
  digest="$(curl -fsSI -H "Authorization: Bearer $token" \
    -H "Accept: application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json" \
    "https://ghcr.io/v2/$path/manifests/$tag" | tr -d '\r' | sed -n 's/^[Dd]ocker-[Cc]ontent-[Dd]igest: //p')"
fi
[[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]] || { echo "not a digest: $digest" >&2; exit 1; }

target="$repo@$digest"
current="$(sed -n 's/^APP_IMAGE=//p' .env | tail -n 1)"
if [ "$current" = "$target" ]; then echo "up to date: $target"; exit 0; fi
echo "update: ${current:-none} -> $target"
[ "${CHECK_ONLY:-}" != "1" ] || exit 0

set_image() { # rewrite the APP_IMAGE line of .env atomically
  local tmp; tmp="$(mktemp .env.XXXXXX)"
  { grep -v '^APP_IMAGE=' .env || true; echo "APP_IMAGE=$1"; } > "$tmp"
  chmod 600 "$tmp"; mv "$tmp" .env
}
probe() { # readiness through Caddy, certificate checks skipped: the point is that the app answers behind the proxy
  local host; host="$(sed -n 's/^SITE_ADDRESS=//p' .env | tail -n 1)"
  for _ in $(seq 1 30); do
    if curl -fsS -k --resolve "$host:443:127.0.0.1" "https://$host/readyz" > /dev/null 2>&1; then return 0; fi
    sleep 2
  done
  return 1
}

if docker compose ps --status running --services 2>/dev/null | grep -qx postgres; then
  KEEP="${KEEP:-14}" scripts/backup.sh backups
  dump="$(ls -1t backups/bcm-*.dump | head -n 1)"
else
  dump=""
fi

[ -z "$current" ] || echo "$current" > "$state/previous-image"
set_image "$target"
if docker compose pull app && docker compose up -d --wait && probe; then
  echo "$target" > "$state/current-image"
  echo "deployed $target"
  exit 0
fi

echo "the new release did not become ready" >&2
if [ -n "$current" ]; then
  set_image "$current"
  if docker compose up -d --wait && probe; then
    echo "rolled back to $current" >&2
  else
    echo "the previous image does not start either; if the new release migrated the database, restore the dump:" >&2
    echo "  docker compose stop app && CONFIRM=yes scripts/restore.sh ${dump:-backups/<dump>} && docker compose up -d --wait" >&2
  fi
fi
exit 1
