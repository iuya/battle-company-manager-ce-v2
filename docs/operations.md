# Operations

For whoever runs the app: the first start of the production setup, TLS and cookies, configuration, and a map of the pages for deploying, releasing, backing up and hardening.

- [First start (production)](#first-start-production)
- [Account links: logs, audit, retention, several instances](#account-links-logs-audit-retention-several-instances)
- [Where to look next](#where-to-look-next)

## First start (production)

Plain `docker compose up -d` (the default `compose.yaml`, project `bcm`) is the production setup.
`compose.dev.yaml` is the throwaway development database.

```bash
cp .env.example .env                      # set POSTGRES_PASSWORD (openssl rand -hex 24); everything else has a default
docker compose up -d --build --wait       # make up: builds the image, starts PostgreSQL on the named volume, then the app
curl -s localhost:8080/readyz             # ok
make prod-user EMAIL=you@example.org NAME="You" ROLE=superuser     # the first account; asks for the password
```

Later accounts join through an administrator's invitation link.

What these commands do:

- `make prod-user` runs `docker compose run --rm app user create --email ... --name ...`.
  The password is read from the terminal, or from one line of stdin with `-T`.
  With `ROLE=` it also runs `app user grant --role superuser --create-campaigns`.
- The app applies the migrations when it starts (under an advisory lock).
- `restart: unless-stopped` brings the app back after a reboot.
- Without `POSTGRES_PASSWORD` compose stops with a message; there is no default password.
- The database has no published port; only the app reaches it.
- The app is published on `APP_BIND:APP_PORT` (default `127.0.0.1:8080`, for a reverse proxy on the same host).
  Use `APP_BIND=0.0.0.0` to listen on every interface.
- The app container runs with a read-only root filesystem, all capabilities dropped and `no-new-privileges`.

### TLS and cookies

Production expects HTTPS in front: a reverse proxy terminates TLS and forwards to the app port.

- Session, theme and glossary cookies are `Secure` and `Strict-Transport-Security` is sent.
  This is `SECURE_COOKIES=true`, the compose default and the setting behind a TLS proxy.
- The server **refuses to start** in production with `SECURE_COOKIES=false` unless `ALLOW_INSECURE_COOKIES=true` is also set.
  That is for a plain-http try-out or a private network; uncomment that line in `compose.yaml`.
- Set `TRUST_PROXY=true` only when the proxy sets `X-Forwarded-For` itself.
- `CSP_MODE` stays `report` until you switch it (see [Content-Security-Policy](csp.md)).

The variables beyond `POSTGRES_PASSWORD`, `APP_BIND`, `APP_PORT` and `SECURE_COOKIES` are commented in the app's `environment:` in `compose.yaml`.
Uncomment the ones you change; they then read `.env`.

### Configuration reference

Every variable the server reads is listed, with its default, in `compose.yaml`, `compose.dev.yaml` and `.env.example`.
`make check-env-docs` fails when one is missing.

`MAX_CACHED_RULESETS` (default 16, at least 1) caps how many parsed rule sets stay in memory.
A rule set is a few MB and there is one per distinct hash triple of rules files; the least recently used is dropped first.
Raise it only if a superuser uploads many rules files that campaigns pin; lower it on a very small host.

## Account links: logs, audit, retention, several instances

- **Logs never hold a link secret.** The request and panic logs write the route (`/register/{token}`, `/recover/{token}`, `/join/{token}`) instead of the path.
- **Audit and retention.** `audit_events` is kept for ever and holds no secret.
  The housekeeping of `Auth.Login` deletes unused revoked or expired account invitations, and ended recovery links, 90 days after they ended.
  Used invitations stay.
  A named invitation keeps its email after the account is closed.
- **One instance.** The throttles (login, registration, recovery) and the session cache are per process.
  With more than one app instance the limits multiply, and a recovery link can survive on another instance for up to 60 seconds (the session cache lifetime).
  The current deployment runs one instance.


## Where to look next

| Page | What it covers |
|---|---|
| [backup-restore.md](backup-restore.md) | What to back up, backup, restore, the restore drill, the recovery point |
| [releases.md](releases.md) | The published image, how a release is built, tags, signed images, rolling back an image |
| [deploy-server.md](deploy-server.md) | The server kit: updater, memory, rollback, Oracle Cloud Always Free, DuckDNS |
| [csp.md](csp.md) | Content-Security-Policy: report mode, then enforce |
| [robustness.md](robustness.md) | Guards, limits and timeouts that keep one failure from taking the app down |
| [architecture.md](architecture.md) | Configuration, health checks, the migration command |
