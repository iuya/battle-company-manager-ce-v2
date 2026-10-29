# Collaborating on this project

How to work on the app, especially with a coding agent at your side. Humans and agents follow the same rules; this page is the short version.
Rules for pull requests and issues are in [CONTRIBUTING.md](../CONTRIBUTING.md).

## 1. Where everything is

| Path | What lives there |
|---|---|
| `internal/core` | Business logic (campaigns, companies, matches, aftermath, rules engine). Declares the repository interfaces it needs |
| `internal/core/lean` | The lean rules format: typed model, strict decoder, validation ([definitions.md](definitions.md)) |
| `internal/core/testdata` | Golden files: `snapshot/` (all 37 companies), `auras_golden.txt`, `tts_golden.txt` |
| `internal/postgres` | PostgreSQL adapter (pgx), embedded goose migrations in `internal/postgres/migrations` |
| `internal/web` | HTTP adapter: handlers, Go templates, htmx/Alpine, Tailwind, static assets. Declares the core interfaces it needs |
| `cmd/server` | `main`, wiring, the `user`, `seed`, `migrate`, `audit` commands, the demo world script |
| `cmd/rulecheck` | Checks a Battle Companies file like the upload does (`make check-ruleset`) |
| `definitions/` | Rules data: `battle_companies/battle_companies.json` (lean, the one you edit) and its `.schema.json`, `vanilla/` (read-only), `glossary/`, `concepts/` |
| `tools/` | Python tooling for the definitions, glossary, concept graph, icons, theme contrast ([tools/README.md](../tools/README.md)) |
| `tests/e2e` | Browser tests (puppeteer-core, headless Chrome) against the demo world; `tests/ci` tests the pull request check |
| `docs/` | Documentation of the current state; specs of designed features (`*-spec.md`) |
| `.claude/agents` | Specialist agent roles (section 3) |
| `.beads` | Beads (bd) hooks and setup; the issue database itself is private and local to the maintainers |
| `sources/` | Gitignored, local only: copyrighted rulebooks and anything converted from them |

## 2. How we build

**Ports and adapters.** `core` knows nothing about HTTP or SQL. Each consumer declares the interface it needs, next to the code that uses it, and its
tests run against a generated mock. After you change a consumer interface run `make generate` (mockgen, driven by `//go:generate` lines). See [architecture.md](architecture.md).

**Beads (`bd`) instead of TODO lists.** Tasks, decisions and shared memory live in the tracker, so any agent or person can pick up where another stopped.
`bd ready` finds work, `bd show <id>` reads it, `bd update <id> --claim` takes it, `bd close <id>` finishes it, `bd create` files follow-ups.
Add progress with `bd update <id> --append-notes "..."` (not `--notes`, which replaces). Durable facts go to `bd remember`; read them with `bd memories`.
No markdown TODO lists, no memory files. `bd prime` prints the full workflow.

The maintainers' tracker is **private** (its database is backed up to a private repository, and neither the issue export nor the local `bd` config is committed). Outside contributors work from **GitHub issues** and pull requests. If you run `bd` in your own clone, `bd init` creates your own local database; it never mixes with the maintainers'.

**Specs first.** A feature starts as a spec with acceptance criteria and the interface contract (the domain expert writes `docs/<feature>-spec.md`, for
example [practice-match-spec.md](practice-match-spec.md)); planners agree the core and persistence contracts; then a coder implements one bead at a time,
test first, against mocks. Large features start from an agreed issue.

**Behaviour safety nets.**
- *Golden snapshot*: `go test ./internal/core -run Snapshot` compares everything derived from the Battle Companies file with `internal/core/testdata/snapshot/`.
  Update with `-update-snapshot` only when the change is intended (a rules-data fix, a deliberate engine change); review the diff, list the changes by category
  in the commit or PR, and commit the golden update on its own. A golden that changes in a refactor means the refactor changed behaviour.
- *Custom rulesets*: every engine parameter has a table-driven `TestCustomRuleset*` row that changes the value in a copy of the file and checks the effect, so nothing
  stays hard-coded ([testing.md](testing.md), "Custom rulesets").
- *Equivalence pattern*: when a representation changes, keep the old and new path side by side with a test proving they agree, then delete the old path and keep the goldens as the proof.
- *Browser tests*: `make e2e` runs flows in groups (`tests/e2e/groups.json`); every flow file is in exactly one group (`make e2e-check-groups`, part of `make test`).

**Gates before you ask for review**

```bash
gofmt -l .                # prints nothing
go vet ./...
make test                 # unit tests, python tool tests, e2e group check
make validate             # rules data, if you touched definitions/ or tools/
make test-integration     # if you touched internal/postgres or SQL
make e2e-enforce          # if you touched templates, handlers or JS (blocking CSP)
```

**Commits and releases.** Conventional Commits (`<type>[(scope)][!]: <description>`), one logical change per commit. Maintainers squash the local commits of a
release into one meaningful commit, tag it `vMAJOR.MINOR.PATCH` and push; nobody else pushes to `main` or tags.

**Documentation and comments.** `docs/` describes the app as it is, not how it got there; update the page in the same change. Comments explain why, not what
or what used to be.

## 3. Working with an agent

**First prompt.** Paste something like this and adapt the task:

```text
Read CLAUDE.md (or AGENTS.md), run `bd prime`, and read docs/README.md plus the doc or spec for the area I name.
Task: <what, and the issue number>. Work test first. Use `bd` for tracking, no TODO lists.
Commit only your own paths with explicit `git add <paths>`. Do not push, tag, or change repository settings.
Before you finish run: gofmt -l ., go vet ./..., make test (and make validate / make e2e-enforce when relevant) and report the results.
```

**Roles in `.claude/agents`** (Claude Code subagents; for other tools read the file as a role prompt):

| Role | Use it for |
|---|---|
| `frontend-domain-expert` | Turn a feature idea into a spec, acceptance criteria and a core interface; no code |
| `frontend-implementer` | Build one page or flow in `internal/web` from a spec |
| `backend-planner` | Agree entities, services, repository interfaces and error values before coding |
| `backend-coder` | Implement one backend bead in `internal/core`, test first |
| `database-master` | Schema, forward-only migrations, `internal/postgres`, integration tests |
| `definitions-curator` | Anything in the rules data, schemas, glossary, concept graph and their tooling |
| `gear-classification-reviewer` | Independent, read-only review of gear annotations in the rules data |
| `company-auditor` | Review generated audit reports (`make audit`) of Battle Companies and triage the differences |
| `qa-tester` | Play one area of the demo world in headless Chrome and file or fix what breaks |
| `perf-frontend`, `perf-backend`, `perf-database` | Behaviour-neutral performance passes, in that order |
| `infra-security` | Infrastructure and security review |

**Several agents at once** share one working tree, so:
- commit only your own paths with explicit `git add <paths>`; never `git add -A` or `git add .`;
- do not touch files another agent is editing, or files that are not yours (untracked notes, for example);
- do not kill processes you did not start (no `pkill`); no waiting shells, no `tail -f`: run long jobs in the background and check their result;
- do not run the same kind of work in parallel; one agent per area keeps context and cost down.

**Never without the maintainer:** push, tag or release; change repository settings, branch rules or secrets; read or write credentials; create cloud
resources; delete the tracker or force-reset history.

**Reviewing an agent's work.** Read the diff by category (production code, tests, rules data, goldens, docs) rather than file by file. Check that a bug fix has a
test that failed first; that a golden diff is explained and limited to what the change should affect; that rules data came from a rule you can name, not a guess;
that no new constant duplicates a rule the json holds. Then run the gates above yourself instead of trusting the report.

**Agents that are not Claude Code.** [AGENTS.md](../AGENTS.md) carries the same rules in a tool-neutral form: read `CLAUDE.md`, use `bd`, keep to non-interactive
shell flags (`cp -f`, `rm -f`), and stop at the commit: report changed files, validation and suggested next commands.

## 4. Recipes

| Task | Steps |
|---|---|
| Add or change a rule, keyword or unit | Edit `definitions/battle_companies/battle_companies.json` (never vanilla), `make validate`, `go test ./internal/core -run Snapshot`, review the golden diff, update with `-update-snapshot` if intended. Format and conventions: [definitions.md](definitions.md) |
| Add a company, path, item or unit | Same as above, following the neighbouring entries and the schema; the audit (`make audit`) and [rules-coverage.md](rules-coverage.md) show what the app does with it. The step-by-step guide is [customizing-rulesets.md](customizing-rulesets.md) |
| Fix a bug | Write the failing test first (unit in `internal/core`, handler test in `internal/web`, or an e2e step), fix, run the gates, reference the issue |
| Add a UI page | Read [frontend.md](frontend.md); semantic theme tokens only; server-render and use htmx for swaps; add it to the style guide when it is a new component (`/styleguide` in `make run`); check 360, 768 and 1280 px in light and dark; add or extend an e2e flow and register it in `tests/e2e/groups.json` |
| Change the database schema | New numbered file in `internal/postgres/migrations` (forward-only, safe on existing data, never edit an applied one); adapt the repository and its integration test; `make test-integration` |
| Change a consumer interface | Edit the interface where it is used, `make generate`, update the tests against the new mock |

## 5. Good first contributions

- A rules-data correction with the rule reference (use the "Rules data" issue form first).
- A missing or unclear sentence in `docs/`.
- A rule or gap listed in [rules-coverage.md](rules-coverage.md) that lacks a test.
- A small accessibility or responsive fix found with `/styleguide` at 360 px.
- A bug you can reproduce with `make demo`.

## 6. What reviewers expect

One logical change per pull request, linked to an issue, with a Conventional Commit title and the template filled in. Tests that would fail without the change.
Docs updated. No unrelated reformatting. Maintainers review, approve and merge; expect questions, and answer them in the pull request.
