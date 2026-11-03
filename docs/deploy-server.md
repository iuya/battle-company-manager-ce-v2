# Deploying on a server

For the owner of one Linux server (Oracle Cloud Always Free or any Docker host) that always runs the newest stable release from the published image, without a checkout of the source.

- [Running the published image on a server](#running-the-published-image-on-a-server)
- [First start on a server](#first-start-on-a-server)
- [The updater](#the-updater)
- [Memory](#memory)
- [Roll back on a server](#roll-back-on-a-server)
- [On Oracle Cloud Always Free](#on-oracle-cloud-always-free)
- [A DuckDNS name that follows the IP](#a-duckdns-name-that-follows-the-ip)

## Running the published image on a server

`deploy/` is a kit for one Linux server that always runs the newest stable release, without a checkout of the source.
It needs Docker with the compose plugin, `curl`, `flock` and systemd; nothing else.

| File | What it is |
|---|---|
| `deploy/compose.prod.yaml` | Caddy, the app and PostgreSQL. Installed as `/opt/bcm/compose.yaml`. Only Caddy publishes ports (80, 443 and 443/udp); the app and the database sit on internal networks. The app runs read-only, with every capability dropped, memory, CPU and process limits, `TRUST_PROXY=true` and `SECURE_COOKIES=true`; container logs rotate (10 MB x 5) |
| `deploy/Caddyfile` | Automatic HTTPS for `SITE_ADDRESS`, HTTP redirected to HTTPS, a 17 MB request cap, the client address handed to the app as `X-Forwarded-For` (replacing whatever the client sent), live streams unbuffered, no `Server` header, admin API off |
| `deploy/env.example` | The `/opt/bcm/.env` the installer writes: `SITE_ADDRESS`, a `POSTGRES_PASSWORD` generated on the host, `APP_IMAGE` (kept by the updater), `CSP_MODE`, `REQUIRE_SIGNATURE`. Mode 0600, never committed |
| `deploy/install.sh` | Copies the kit and the backup scripts into `/opt/bcm` (`BCM_DIR` changes it), creates `.env` once, enables the timers. `sudo SITE_ADDRESS=bcm.example.org deploy/install.sh` |
| `deploy/bcm-update.sh` + `systemd/bcm-update.{service,timer}` | The pull-based updater, every 5 minutes (see [The updater](#the-updater)) |
| `deploy/bcm-backup.sh` + `systemd/bcm-backup.{service,timer}` | Nightly 03:15 dump (14 kept on disk), then the optional off-site hook `/opt/bcm/offsite-hook.sh <dump>`; a failing hook turns the unit red |
| `systemd/bcm-restore-drill.{service,timer}` | The monthly restore drill of `scripts/restore-drill.sh` (see [Restore drill](backup-restore.md#restore-drill)) |
| `deploy/memcheck.sh` | Memory, swap, per-container usage and out-of-memory readings (see [Memory](#memory)) |
| `deploy/bootstrap-admin.sh` | The first account: `sudo /opt/bcm/bootstrap-admin.sh you@example.org "Your Name"`. The password is typed at the prompt; it is in no argument, file or log |
| `deploy/oracle/` | Oracle Cloud specifics (see [On Oracle Cloud Always Free](#on-oracle-cloud-always-free)): `host-setup.sh` and `offsite-oci.sh` |

`deploy/oracle/host-setup.sh` prepares an Ubuntu 24.04 or 26.04 aarch64 instance:

- Docker and compose from the Ubuntu packages
- the host firewall rules for 80/443
- a block on the metadata service for containers
- swap and security updates
- cosign for the signature check
- then `install.sh`

`deploy/oracle/offsite-oci.sh` is an off-site hook that copies dumps to a private Object Storage bucket with the instance principal.

### First start on a server

1. Point a DNS name at the server (Let's Encrypt validates it).
2. Open ports 80 and 443 in the firewall and nothing else.
3. Run `deploy/install.sh` as above. On Oracle Cloud, run `deploy/oracle/host-setup.sh`, which calls it.
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
6. Probes `/readyz` through Caddy.

A release that does not become ready is replaced by the previous digest.
The server is therefore always on an immutable digest, never on a moving tag.

- `UPDATE_DIGEST=sha256:...` deploys an exact digest.
- `CHECK_ONLY=1` only prints what it would do.
- State is in `/opt/bcm/state` (`current-image`, `previous-image`).

### Memory

The compose file is sized for a 3 GB server (the Oracle shape of 1 OCPU and 3 GB).
The container limits add up to 1,088 MB:

| Container | Limit | Settings |
|---|---|---|
| caddy | 256 MB | |
| app | 512 MB | `GOMEMLIMIT=400MiB`, `MAX_CACHED_RULESETS=4` |
| PostgreSQL | 320 MB | `shared_buffers=64MB`, `work_mem=2MB`, `max_connections=20` |

A provider script such as `host-setup.sh` adds a 1 GB swap file with `vm.swappiness=10` as a safety net.
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

### On Oracle Cloud Always Free

`deploy/oracle/` targets one `VM.Standard.A1.Flex` instance (Arm) running Ubuntu 24.04 or 26.04 in the tenancy's home region, inside the Always Free allowance.
The account is never upgraded to a paid type.
The allowance covers, in total:

- 2 OCPUs and 12 GB of memory for all A1 instances
- 200 GB of boot and block volume
- 20 GB of Object Storage
- 10 TB of outbound traffic a month

Check the limits page of the console and [Always Free Resources](https://docs.oracle.com/en-us/iaas/Content/FreeTier/freetier_topic-Always_Free_Resources.htm) before relying on a number, because Oracle changes them.

- **Sizing.** One OCPU and 3 GB is enough with the shipped container limits.
  Oracle may reclaim an Always Free instance that stays idle for seven days (CPU 95th percentile, network and, for A1, memory all under 20%).
  The app is quiet, so size the memory so that real use stays above that share rather than adding load.
  Watch it with `/opt/bcm/memcheck.sh` and the console's memory metric.
- **Network.** The subnet's security list or network security group allows 80/tcp, 443/tcp and 443/udp from anywhere and nothing else from the internet.
  Port 22 is reachable only from inside the VCN (virtual cloud network), which is where the Bastion service connects from.
  Oracle's Ubuntu images also carry their own iptables rules that reject everything but SSH; `host-setup.sh` inserts 80/443 before that reject rule.
  Use an ephemeral public IPv4 and the name `<a-b-c-d>.sslip.io` for the address a.b.c.d as `SITE_ADDRESS`.
  A new instance gets a new address and so a new name.
- **Access.** Interactive access is a Bastion port-forwarding session (or the instance's console connection) with an SSH key kept on the owner's machine.
  Nothing in this repository holds a key or a credential.
- **Install.** On the instance, from an unpacked release:

  ```bash
  sudo SITE_ADDRESS=<a-b-c-d>.sslip.io BACKUP_BUCKET=<bucket> OCI_NAMESPACE=<namespace> deploy/oracle/host-setup.sh
  ```

  The last two variables are optional and enable the off-site hook.
  The OCI command-line tool it installs for that hook (`OCI_CLI_VERSION`, default 3.95.1) takes a few minutes and prints nothing while pip works; the script says so first.
  The script also installs cosign and turns the signature check on.
  While the newest release in the registry predates signing, pass `REQUIRE_SIGNATURE=0` (a server that is already running is not affected).
- **Off-site backup.** A dynamic group containing only this instance, and a policy limited to one bucket, let the instance principal write dumps to a private Object Storage bucket.
  The Always Free quota is 20 GB and 50,000 requests a month; a dump is a few megabytes.
  The tenancy and everything in it can disappear, so keep a copy of a dump somewhere outside Oracle as well, and run the restore drill.

### A DuckDNS name that follows the IP

An ephemeral public IP changes when the instance is stopped and started or recreated, and a name such as `<a-b-c-d>.sslip.io` changes with it.
A free [DuckDNS](https://www.duckdns.org) name, here `bc-manager-ce.duckdns.org`, stays the same.
A small updater on the host points it at the new address.

#### How the updater behaves

`deploy/duckdns/duckdns-update.sh` (installed as `/opt/bcm/duckdns-update.sh`) runs from `bcm-duckdns.timer`, two minutes after boot and every five minutes (up to 30 seconds of random delay).

Each run:

1. Looks up the host's public IPv4 address with a plain HTTPS request: `api.ipify.org`, then `checkip.amazonaws.com`, then `icanhazip.com`.
   Several providers mean one being down does not stop it; no cloud metadata service is used, so it works on any host.
2. Asks the host's resolver what the name resolves to (`getent ahostsv4`).
3. Calls DuckDNS only in these cases:
   - the address differs from the one it set last time (`/opt/bcm/state/duckdns-ip`);
   - DNS does not answer with the current address (a stale record is fixed even if the state file says otherwise; not within ten minutes of an update, which is DNS propagation);
   - once a day, as a refresh;
   - the previous call failed (no state is written on a failure, so the next run retries).

The call is `https://www.duckdns.org/update?domains=<name>&token=<token>&ip=` with an empty `ip`, for which DuckDNS uses the address the request comes from.
It answers `OK` or `KO`.
DuckDNS's auto-detection is IPv4 only, and its [spec](https://www.duckdns.org/spec.jsp) (read on 2026-11-01) states no rate limit, so the updater stays at one call per change, per day and per retry.

The log has one line per run (`journalctl -u bcm-duckdns`): `OK`, `KO`, `unchanged` or `no token file yet`, with the name and the IP the host sees.
It never contains the token or the URL.

#### The token is a secret

The token lives only in `/etc/bcm/duckdns.token` (root, mode 600), which the owner creates by typing the token once on the host.

- The script hands it to `curl -K -` on the standard input through the shell builtin `printf`.
  It is on no command line (`ps`), in no environment variable, and in no log or journal entry.
- curl's error text is discarded because it can echo a URL.
- The unit runs as a one-shot with `NoNewPrivileges`, `ProtectSystem=strict` (only `/opt/bcm/state` writable, `/etc/bcm` read-only), `PrivateTmp`, `PrivateDevices`, an empty capability set, a system-call filter and only IP and local sockets.
- Nobody pastes the token into chat, an issue, a pull request or CI.

#### One-time steps at DuckDNS (the owner)

1. Sign in at [duckdns.org](https://www.duckdns.org) with one of its sign-in providers.
2. Add the subdomain `bc-manager-ce`. Any free name works; use it without `.duckdns.org` below.
3. Copy the token shown at the top of the page.

Keep the token secret: anyone holding it can repoint every name of the account.
If it leaks, generate a new one on the page and recreate the file.

#### Enable the updater

Give the subdomain to the installer.
It installs the script and the timer and enables the timer, but does not create the token file:

```bash
sudo SITE_ADDRESS=<current name> DUCKDNS_DOMAIN=bc-manager-ce deploy/oracle/host-setup.sh     # or deploy/install.sh on another host
```

It prints the one line the owner then runs on the host (the prompt hides the typing):

```bash
sudo install -d -m 700 /etc/bcm && read -rsp 'DuckDNS token: ' t && printf %s "$t" | sudo install -m 600 /dev/stdin /etc/bcm/duckdns.token; unset t
sudo systemctl start bcm-duckdns.service
```

Until the file exists the timer only logs `no token file yet` and does nothing else.
On a host that is already installed, run the same command again with `DUCKDNS_DOMAIN`; it keeps `.env` and the data.

#### Check the updater

- `systemctl list-timers 'bcm-duckdns*'` shows the next run.
- `journalctl -u bcm-duckdns -n 20` shows the last ones.
- `getent hosts bc-manager-ce.duckdns.org` must print the public IP of the instance.

A failed run shows in `systemctl --failed` and heals on the next one.

#### Serve the app under the name

`SITE_ADDRESS` stays a single name (the updater's readiness probe uses it).
Once `getent hosts` resolves the DuckDNS name to the instance, set `SITE_ADDRESS=bc-manager-ce.duckdns.org` in `/opt/bcm/.env` and run `docker compose up -d` in `/opt/bcm`.

Consequences:

- The old name stops working.
- The session cookies of the old host stop working, so everyone logs in again.
- Invitation and recovery links already handed out name the old host in their text.
  The token in them is not tied to a host, so replace the old host by the new one in the address (or create a new link).
- Caddy asks Let's Encrypt for the certificate on the first request, so the DNS name must resolve before that.
  Do not hammer it: Let's Encrypt limits failed validations and certificates per name.
- DuckDNS's name servers occasionally time out, and then issuance fails.
  Wait and retry later (`docker compose logs caddy`).

#### When the IP changes

The address is replaced at the next timer run.
The app is unreachable by name for at most about five minutes (plus the DNS cache time of the clients) and then comes back by itself.
Caddy keeps its certificate because it belongs to the name, not to the address; there is no new issuance.

#### Alternative

An Oracle reserved public IP keeps one address through restarts and recreation, so a name would not have to follow anything.
Check Oracle's current documentation and the Always Free limits page for what it costs and how many are included before relying on it; this repository does not verify that.
The DuckDNS updater needs no cloud account action at all and works on any host.

#### Tests

`make deploy-test` runs the script against stub `curl` and `getent`.
It checks that:

- the token appears in no output, argument or state;
- there is no call when the address is unchanged and DNS matches;
- there is a call when it differs or DNS is stale;
- `KO` is handled and retried;
- a missing token file is handled.

It does not call DuckDNS, and the unit's sandboxing options are not exercised there.


See also: [releases.md](releases.md) ("Signed images", rollback of the image), [backup-restore.md](backup-restore.md), [operations.md](operations.md).
