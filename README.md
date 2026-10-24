# Battle Companies 2nd CE Manager

A manager for the *Middle-earth Strategy Battle Game* Battle Companies 2nd Community Edition game mode: campaigns,
companies and matches. Go + PostgreSQL, with a server-rendered frontend (Go templates, htmx, Alpine.js, Tailwind).

## Quick start

```
make user EMAIL=you@example.com NAME="Your Name"   # creates an account (asks for a password)
make seed                                          # optional: demo accounts and campaign (docs/frontend.md, "Demo data")
make run                                           # http://localhost:8080
```

To explore every state of the app at once, `make demo` **empties the development database (destructive)**, loads a rich demo world (about fifty
accounts and seventeen campaigns: every company, injury, level-up, scenario and aftermath state) and starts the server on
http://localhost:8080. Sign in as `admin@example.com` with `demo-password`; [docs/demo-world.md](docs/demo-world.md) says what
each campaign shows and where to click. `make seed` stays the minimal demo.

Needs Go, Docker (for PostgreSQL) and the Tailwind standalone binary (`make get-tailwind`).

## Deploying

A `Dockerfile` (multi-stage, distroless, non-root) and the default `compose.yaml` run the server in production mode from scratch:
`cp .env.example .env` (set `POSTGRES_PASSWORD`), `docker compose up -d --build --wait`, then `make prod-user EMAIL=... NAME=... ROLE=superuser` for the first account
([docs/operations.md](docs/operations.md), "First start"). Each release tag also publishes the image to `ghcr.io/iuya/battle-company-manager-ce-v2` (amd64, arm64; "Published image" there). It starts with secure cookies and HSTS on, Content-Security-Policy in report-only mode, `/healthz` and `/readyz` for probes.
`make run`, `dev` and `demo` run in development mode on localhost against the development database in `compose.dev.yaml`. Settings, migrations, proxy recipe and the CSP switch to enforce:
[docs/architecture.md](docs/architecture.md) ("Configuration", "Deploying").

## Documentation

See [docs/](docs/README.md): architecture, frontend design language, authentication and the rules definitions.
`tools/README.md` explains the definitions tooling.
