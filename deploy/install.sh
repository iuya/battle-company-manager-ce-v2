#!/usr/bin/env bash
# Installs the deployment kit into /opt/bcm on a server that already has Docker with the compose plugin, curl and flock (provider-neutral;
# a provider script such as deploy/oracle/host-setup.sh prepares the host first). Run as root from a checkout (or an unpacked release) of this repository.
#
#   sudo SITE_ADDRESS=bcm.example.org deploy/install.sh
#
# Idempotent: existing .env and data are kept. It generates POSTGRES_PASSWORD on the host when .env does not exist yet, starts nothing but
# the update timer's first run (bcm-update.service), and prints no secret.
set -euo pipefail
[ "$(id -u)" = 0 ] || { echo "run as root" >&2; exit 2; }
src="$(cd "$(dirname "$0")/.." && pwd)"
dest="${BCM_DIR:-/opt/bcm}"

install -d -m 0750 "$dest" "$dest/scripts" "$dest/backups" "$dest/state"
install -m 0644 "$src/deploy/compose.prod.yaml" "$dest/compose.yaml"
install -m 0644 "$src/deploy/Caddyfile" "$dest/Caddyfile"
for f in bcm-update.sh bcm-backup.sh bootstrap-admin.sh memcheck.sh; do install -m 0750 "$src/deploy/$f" "$dest/$f"; done
for f in backup.sh restore.sh restore-drill.sh; do install -m 0750 "$src/scripts/$f" "$dest/scripts/$f"; done
if [ ! -f "$dest/.env" ]; then
  : "${SITE_ADDRESS:?set SITE_ADDRESS to the public host name}"
  umask 077
  { echo "SITE_ADDRESS=$SITE_ADDRESS"; echo "POSTGRES_PASSWORD=$(openssl rand -hex 24)"; echo "APP_IMAGE="; echo "CSP_MODE=${CSP_MODE:-report}"; } > "$dest/.env"
  chmod 600 "$dest/.env"
fi
# REQUIRE_SIGNATURE=1 (or 0) given in the environment is written to .env, where bcm-update.sh reads it (docs/operations.md, "Signed images").
if [ -n "${REQUIRE_SIGNATURE:-}" ]; then
  [[ "$REQUIRE_SIGNATURE" =~ ^[01]$ ]] || { echo "REQUIRE_SIGNATURE must be 0 or 1" >&2; exit 2; }
  tmp="$(mktemp "$dest/.env.XXXXXX")"
  { grep -v '^REQUIRE_SIGNATURE=' "$dest/.env" || true; echo "REQUIRE_SIGNATURE=$REQUIRE_SIGNATURE"; } > "$tmp"
  chmod 600 "$tmp"; mv "$tmp" "$dest/.env"
fi
if [ -d /run/systemd/system ]; then
  for u in "$src"/deploy/systemd/*; do install -m 0644 "$u" /etc/systemd/system/; done
  sed -i "s#/opt/bcm#$dest#g" /etc/systemd/system/bcm-*.service
  systemctl daemon-reload
  systemctl enable --now bcm-update.timer bcm-backup.timer bcm-restore-drill.timer
  systemctl start bcm-update.service
fi
echo "installed in $dest; next: $dest/bootstrap-admin.sh you@example.org \"Your Name\""
