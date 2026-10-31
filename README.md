# Battle Companies 2nd CE Manager

[![e2e](https://github.com/iuya/battle-company-manager-ce-v2/actions/workflows/e2e.yml/badge.svg)](https://github.com/iuya/battle-company-manager-ce-v2/actions/workflows/e2e.yml)
[![release-image](https://github.com/iuya/battle-company-manager-ce-v2/actions/workflows/release-image.yml/badge.svg)](https://github.com/iuya/battle-company-manager-ce-v2/actions/workflows/release-image.yml)

A web app to run campaigns of the *Middle-earth Strategy Battle Game* **Battle Companies 2nd Community Edition** game mode. Organisers create campaigns, players
build and grow their companies, and everyone records matches and their aftermath (injuries, experience, level-ups, influence) from a phone at the table.
It is a Go service with PostgreSQL and a server-rendered frontend (Go templates, htmx, Alpine.js, Tailwind).

- Campaigns, companies and matches for all the Battle Companies, with the rules applied by the app.
- Live match mode: trackers, casualties and a match log with undo, synced between players.
- The War's Aftermath after each match: injury rolls, experience, promotions and rewards.
- Rules data is JSON you can customise and upload; the shipped Battle Companies file is the default.
- Helper popups for rules terms, and a company export for Tabletop Simulator.
- Responsive from 360 px, light and dark themes.

## Quick start

Prerequisites: the Go version in [go.mod](go.mod), Docker (PostgreSQL), Python 3 (rules validation), and Node 20+ with Chrome or Chromium only for the browser tests.

```bash
make get-tailwind                                  # once: downloads the Tailwind standalone binary for this machine (macOS and Linux, x64 and arm64)
make user EMAIL=you@example.com NAME="Your Name"   # creates an account (asks for a password); other people join through an administrator's invitation link
make run                                           # CSS + PostgreSQL + server on http://localhost:8080
```

- `make dev` is the same with live reload.
- `make seed` loads a small demo (accounts and one campaign).
- `make demo` **empties the development database**, loads a rich demo world (many campaigns in every state) and starts the server. Sign in as
  `admin@example.com` with `demo-password`; see [docs/demo-world.md](docs/demo-world.md).

## Run the published image

Every release publishes `ghcr.io/iuya/battle-company-manager-ce-v2` (linux/amd64 and arm64, tags such as `1.2.0`, `1.2`, `1` and `latest`).

```bash
docker pull ghcr.io/iuya/battle-company-manager-ce-v2:latest
```

For a real deployment use `compose.yaml` with the image (or build from the checkout): `cp .env.example .env`, set `POSTGRES_PASSWORD`, `docker compose up -d --wait`,
then `make prod-user EMAIL=... NAME=... ROLE=superuser` for the first account. Production mode has secure cookies, HSTS, a Content-Security-Policy and `/healthz`
and `/readyz` probes. Details, backups and upgrades: [docs/operations.md](docs/operations.md).

## Tests

```bash
make test               # unit tests (Go and the Python tools)
make test-integration   # everything, including the PostgreSQL tests (starts the compose database)
make e2e                # browser tests in headless Chrome (make e2e-install first; make e2e-enforce runs under the blocking CSP)
make validate           # rules data: JSON Schemas, validators, glossary and concept graph
```

More in [docs/testing.md](docs/testing.md).

## Project layout

| Path | Contents |
|---|---|
| `cmd/server` | The `main` program, wiring, and the `user`, `seed`, `migrate`, `audit` commands |
| `cmd/rulecheck` | Checks a Battle Companies rules file |
| `internal/core` | Business logic and the repository interfaces it needs |
| `internal/postgres` | PostgreSQL persistence and migrations |
| `internal/web` | HTTP handlers, templates, htmx/Alpine and static assets |
| `definitions/` | Rules data in JSON, with schemas |
| `tools/` | Python tooling for the rules data, icons and theme checks |
| `tests/` | Browser tests (`e2e`), pull request check tests (`ci`), Tabletop Simulator tests (`tts`) |
| `docs/` | Documentation |
| `scripts/` | Backup and restore scripts |

## Documentation

Start at [docs/README.md](docs/README.md). Everything in `docs/`, by audience:

**Players and organisers**

| Document | What it covers |
|---|---|
| [demo-world.md](docs/demo-world.md) | The demo world: what each campaign shows and where to click |
| [matches.md](docs/matches.md) | Matches: scenario, results, Against the Odds, ending by confirmation |
| [aftermath.md](docs/aftermath.md) | The War's Aftermath: injuries, experience, level-ups, rewards |
| [live-match.md](docs/live-match.md) | Live match mode: trackers, casualties, Break Point, log with undo |
| [unit-keywords.md](docs/unit-keywords.md) | Race, faction and unit-type keywords on the unit cards |
| [aura-ranges.md](docs/aura-ranges.md) | Aura ranges and how to draw them in Tabletop Simulator |
| [tabletop-simulator.md](docs/tabletop-simulator.md) | Taking a company into Tabletop Simulator |
| [admin.md](docs/admin.md) | The Administration section, campaign status, invitations, handover |
| [rules-coverage.md](docs/rules-coverage.md) | Which Battle Companies mechanics the app implements |
| [multi-company-spec.md](docs/multi-company-spec.md) | Several companies per player (up to a campaign setting), the company in the URL, and opening the last campaign directly (implemented) |
| [practice-match-spec.md](docs/practice-match-spec.md) | Practice matches: no rewards, no injuries, an empty aftermath (implemented) |

**Rules authors (customising the Battle Companies JSON)**

| Document | What it covers |
|---|---|
| [customizing-rulesets.md](docs/customizing-rulesets.md) | A guide to customising the rules file, step by step |
| [customizing-rulesets-reference.md](docs/customizing-rulesets-reference.md) | The reference of every setting of the rules file |
| [definitions.md](docs/definitions.md) | The rules data: format, conventions, tooling |
| [concept-graph.md](docs/concept-graph.md) | The concept graph behind the helper popups |
| [tools/README.md](tools/README.md) | Step-by-step commands of the definitions tooling |

**Developers**

| Document | What it covers |
|---|---|
| [collaborating.md](docs/collaborating.md) | How to work on the project, with or without a coding agent |
| [architecture.md](docs/architecture.md) | Layers, packages, persistence, configuration, commands |
| [frontend.md](docs/frontend.md) | Design language, page and htmx patterns, responsive rules |
| [testing.md](docs/testing.md) | Test commands, golden snapshots, browser tests |
| [auth.md](docs/auth.md) | Accounts, sessions, passwords, CSRF, login throttling |
| [vendored-js.md](docs/vendored-js.md) | The committed third-party scripts and how to update them |
| [performance.md](docs/performance.md) | Measured page cost and the performance tooling |
| [design/match-redesign](docs/design/match-redesign/README.md) | Pictures of the three proposals for the match and live-match pages (a design discussion, not the current app) |
| [optimization-report.md](docs/optimization-report.md) | Executive summary of the optimization work (v1.3.0, baseline and final numbers, what changed and why, what did not improve), then the detailed data format, runtime, database, build and test sections |

**Operators**

| Document | What it covers |
|---|---|
| [operations.md](docs/operations.md) | First start, the published image, backups, restore, robustness |
| [SECURITY.md](SECURITY.md) | How to report a vulnerability |

## Contributing

Issues and pull requests are welcome: see [CONTRIBUTING.md](CONTRIBUTING.md), [docs/collaborating.md](docs/collaborating.md) (a guide for working with coding agents)
and the [Code of Conduct](CODE_OF_CONDUCT.md). Only maintainers review and merge.

## Licence and credits

Released under [CC BY-NC-SA 4.0](LICENSE).

The rules are the *Battle Companies Community 2nd Edition* by the "Middle-earth: SBG Community". `definitions/vanilla/data2024.json` is read-only reference data. This
repository contains no copyrighted rulebook text beyond that; rulebooks and anything converted from them stay in the gitignored `sources/`.

The unit, rank and equipment icons are from [game-icons.net](https://game-icons.net) under
[CC BY 3.0](https://creativecommons.org/licenses/by/3.0/) (the horn from SVG Repo, CC0); each icon and its author is credited in
[internal/web/static/img/CREDITS.md](internal/web/static/img/CREDITS.md).

Not affiliated with Games Workshop.
