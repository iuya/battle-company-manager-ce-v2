# Contributing

Thank you for helping. This project is a Go + PostgreSQL web app for the Battle Companies 2nd Community Edition game mode. By taking part you agree to the
[Code of Conduct](CODE_OF_CONDUCT.md). If you work with a coding agent, read [docs/collaborating.md](docs/collaborating.md) too: it is the practical guide.

## Reporting issues

Use the issue forms (blank issues are off):

- **Bug report** for something that does not work as documented.
- **Feature request** for a new behaviour; large features are discussed in the issue before any code is written.
- **Rules data** for a wrong unit, rule or number, or a question about a custom ruleset. Give the rule reference, what it says and what the app does.

Search existing issues first. Security vulnerabilities do not go in issues: follow [SECURITY.md](SECURITY.md).

## Proposing changes

1. Open (or pick) an issue and say you want to work on it.
2. Fork the repository, create a branch, and make the change from your fork. Only maintainers push to this repository.
3. Open a pull request against `main` from your fork, with the template filled in and the issue linked.

Keep to **one logical change per pull request**. Unrelated clean-ups go in their own pull request.

## Setting up

You need the Go version in `go.mod`, Docker (PostgreSQL for development), Python 3 (`make validate`, tool tests) and, for the browser tests, Node 20+ and Chrome or Chromium.

```bash
make get-tailwind                                    # once: the Tailwind standalone binary for this machine (macOS and Linux, x64 and arm64)
make user EMAIL=you@example.com NAME="You"           # an account (no sign-up page)
make run                                             # http://localhost:8080
make demo                                            # destructive: empties the dev database and loads the demo world
```

See the [README](README.md) for more and [docs/architecture.md](docs/architecture.md) for how the code is organised.

## Commits and pull request titles

[Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/): `<type>[(scope)][!]: <description>`.

- Types: `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `build`, `ci`, `chore`.
- Scope is the area: `core`, `web`, `postgres`, `definitions`, `e2e`, `demo`...
- A breaking change adds `!` or a `BREAKING CHANGE:` footer.
- Describe what and why in the body. Pull requests are squashed, so the **pull request title** is what lands in history: it must follow the rule.

### Git hooks

`make hooks` turns on the hooks in [.githooks](.githooks) for your clone (undo with `git config --unset core.hooksPath`). They check, before the work leaves your machine:

| Hook | What it does |
|---|---|
| `commit-msg` | The subject follows Conventional Commits (the same rule as the `pr-check` workflow). |
| `pre-commit` | Staged Go files are `gofmt`-formatted, and the added lines contain no private key, AWS key id or GitHub token (for a deliberate fake in a test fixture: `BCM_SKIP_SECRET_SCAN=1 git commit ...`). |
| `pre-push` | Refuses a push to `main`: changes reach `main` through a reviewed pull request. The repository rules enforce this on GitHub as well. |

Each hook runs the [beads](docs/collaborating.md) hook of `.beads/hooks` first, so task tracking keeps working. `make hooks-test` checks the hooks in a throwaway repository.

## Tests to run

```bash
gofmt -l .                # prints nothing
go vet ./...
make test                 # unit tests and python tool tests
make test-integration     # when you touch internal/postgres (starts the compose database)
make validate             # when you touch definitions/ or tools/
make e2e-enforce          # when you touch templates, handlers or JavaScript
node tests/ci/pr-check.test.mjs   # when you touch the pull request check
```

Write the test first for a bug fix. Interfaces are declared where they are used; after changing one run `make generate`.
Golden snapshot updates (`go test ./internal/core -run Snapshot -update-snapshot`) are reviewed and explained in the pull request ([docs/testing.md](docs/testing.md)).

## Rules-data changes

The rules the app uses live in `definitions/battle_companies/battle_companies.json` (checked against its `.schema.json`). Edit that file, never
`definitions/vanilla/data2024.json`, which is read-only. Run `make validate` and the snapshot test, and quote the rule you follow in the pull request.
Do not add text copied from copyrighted rulebooks; short names and numbers with a reference are fine. Details: [docs/definitions.md](docs/definitions.md).

## Documentation

`docs/` describes the current state of the app, not its history. Update the page that covers your change in the same pull request. UI changes follow
[docs/frontend.md](docs/frontend.md): semantic theme tokens, responsive from 360 px, checked at 360, 768 and 1280 px in light and dark.

## Review and merge

Contributors open issues and pull requests from forks. **Only maintainers review, approve and merge.** Expect a review with questions or requested changes; push
fixes to the same branch. Draft pull requests are welcome for early feedback.

### Automatic pull request check

The `pr-check` workflow ([.github/workflows/pr-check.yml](.github/workflows/pr-check.yml)) fails when the title is not a Conventional Commit, when the description lacks
a section of the [pull request template](.github/PULL_REQUEST_TEMPLATE.md) (Summary, Type, Linked issue, How it was tested, Checklist), or when a mandatory checklist box
(`make test`, no secrets, licence) is not ticked. The message in the failed run says what to fix; edit the title or description and it runs again. The check is meant to be a
**required status check** on `main`, which the repository owner enables in the repository settings. Its logic is tested by `node tests/ci/pr-check.test.mjs`
(no dependencies).

## Licence

Contributions are licensed under [CC BY-NC-SA 4.0](LICENSE), like the project. Only contribute what you have the right to contribute.
