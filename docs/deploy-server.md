# Deploying on a server

For the owner of one Linux server (any Docker host) that always runs the newest stable release from the published image, without a checkout of the source.

- [Running the published image on a server](#running-the-published-image-on-a-server)
- [First start on a server](#first-start-on-a-server)
- [The updater](#the-updater)
- [Memory](#memory)
- [Roll back on a server](#roll-back-on-a-server)
- [Other hosts](#other-hosts)

## Running the published image on a server

`deploy/` is a kit for one Linux server that always runs the newest stable release, without a checkout of the source.
It needs Docker with the compose plugin, `curl`, `flock` and systemd; nothing else.

| File | What it is |
|---|---|
| `deploy/compose.prod.yaml` | Caddy, the app and PostgreSQL. Installed as `/opt/bcm/compose.yaml`. Only Caddy publishes ports (80, 443 and 443/udp); the app and the database sit on internal networks. The app runs read-only, with every capability dropped, memory, CPU and process limits, `TRUST_PROXY=true` and `SECURE_COOKIES=true`; container logs rotate (10 MB x 5) |
| `deploy/Caddyfile` | Automatic HTTPS for `SITE_ADDRESS`, HTTP redirected to HTTPS, a 17 MB request cap, the client address handed to the app as `X-Forwarded-For` (replacing whatever the client sent), live streams unbuffered, `404` for `/healthz`, `/readyz` and `/metrics` (the probes are for the container network and the host), no `Server` header, admin API off |
| `deploy/env.example` | The `/opt/bcm/.env` the installer writes: `SITE_ADDRESS`, a `POSTGRES_PASSWORD` generated on the host, `APP_IMAGE` (kept by the updater), `CSP_MODE`, `REQUIRE_SIGNATURE`. Mode 0600, never committed |
| `deploy/install.sh` | Copies the kit and the backup scripts into `/opt/bcm` (`BCM_DIR` changes it), creates `.env` once, enables the timers. `sudo SITE_ADDRESS=bcm.example.org deploy/install.sh` |
| `deploy/bcm-update.sh` + `systemd/bcm-update.{service,timer}` | The pull-based updater, every 5 minutes (see [The updater](#the-updater)) |
| `deploy/bcm-backup.sh` + `systemd/bcm-backup.{service,timer}` | Nightly 03:15 dump (14 kept on disk), then the optional off-site hook `/opt/bcm/offsite-hook.sh <dump>`; a failing hook turns the unit red |
| `systemd/bcm-restore-drill.{service,timer}` | The monthly restore drill of `scripts/restore-drill.sh` (see [Restore drill](backup-restore.md#restore-drill)) |
| `deploy/memcheck.sh` | Memory, swap, per-container usage and out-of-memory readings (see [Memory](#memory)) |
| `deploy/bootstrap-admin.sh` | The first account: `sudo /opt/bcm/bootstrap-admin.sh you@example.org "Your Name"`. The password is typed at the prompt; it is in no argument, file or log |

### First start on a server

1. Point a DNS name at the server (Let's Encrypt validates it).
2. Open ports 80 and 443 in the firewall and nothing else.
3. Run `deploy/install.sh` as above.
4. Create the first account with `bootstrap-admin.sh`.
5. Open `https://<name>/`.

The first update run starts everything; follow it with `journalctl -u bcm-update`.
Check the state with:

- `cat /opt/bcm/state/current-image`
- `docker compose -f /opt/bcm/compose.yaml ps`
- `systemctl list-timers 'bcm-*'`

### The updater

`deploy/bcm-update.sh` runs every 5 minutes.
Only tagged releases reach `latest`; a pre-release tag such as `v2.2.0-rc.1` and plain commits on `main` never do.

Each run:

1. Asks ghcr.io for the digest of the `latest` tag (`UPDATE_TAG` follows another tag).
2. When the digest differs, verifies its signature if `REQUIRE_SIGNATURE=1` (see [Signed images](releases.md#signed-images)).
3. Takes a dump.
4. Writes `APP_IMAGE=...@sha256:<digest>` into `.env`.
5. Pulls and starts the image, and waits for the health check.
6. Asks the app for readiness with `docker compose exec app /app/server healthcheck` (Caddy does not forward `/healthz`, `/readyz` or `/metrics` to the internet).

A release that does not become ready is replaced by the previous digest.
The server is therefore always on an immutable digest, never on a moving tag.

- `UPDATE_DIGEST=sha256:...` deploys an exact digest.
- `CHECK_ONLY=1` only prints what it would do.
- State is in `/opt/bcm/state` (`current-image`, `previous-image`).

### Memory

The compose file is sized for a 3 GB server (1 CPU and 3 GB).
The container limits add up to 1,088 MB:

| Container | Limit | Settings |
|---|---|---|
| caddy | 256 MB | |
| app | 512 MB | `GOMEMLIMIT=400MiB`, `MAX_CACHED_RULESETS=4` |
| PostgreSQL | 320 MB | `shared_buffers=64MB`, `work_mem=2MB`, `max_connections=20` |

A provider-specific setup can add a 1 GB swap file with `vm.swappiness=10` as a safety net.
`/opt/bcm/memcheck.sh` prints memory and swap, a `docker stats` snapshot, container restarts and any out-of-memory kills from the kernel log.

On a smaller server (1 GB) lower the three limits and PostgreSQL's settings together, for example caddy 64 MB, app 256 MB with `GOMEMLIMIT=200MiB`, PostgreSQL 320 MB.
On a bigger one, raise them.

### Roll back on a server

```bash
systemctl stop bcm-update.timer     # otherwise the next run moves forward again
UPDATE_DIGEST=$(cat /opt/bcm/state/previous-image | sed 's/.*@//') /opt/bcm/bcm-update.sh
```

This goes back to the previous release.
If the newer release migrated the database and the old one refuses it, stop the app and restore the dump taken just before the update (`/opt/bcm/backups`, newest first); see [Restore](backup-restore.md#restore).

### Other hosts

Provider-specific kits (a cloud VM setup, an off-site backup hook for one provider, a dynamic DNS name) are not part of this repository.
They sit on top of the generic kit: run `deploy/install.sh` after preparing the host, put an executable at `/opt/bcm/offsite-hook.sh` (or name one in `OFFSITE_HOOK`) to copy each dump off the machine, and install `cosign` for the signature check.

See also: [releases.md](releases.md) ("Signed images", rollback of the image), [backup-restore.md](backup-restore.md), [operations.md](operations.md).
