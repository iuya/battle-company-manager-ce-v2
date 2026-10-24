# Battle Companies 2nd CE Manager: documentation

A manager for the *Middle-earth Strategy Battle Game* **Battle Companies 2nd Community Edition** game mode:
campaigns, companies and matches. These documents describe the app as it is now.

| Document | What it covers |
|---|---|
| [architecture.md](architecture.md) | Layers, packages, how they are wired and tested, configuration, commands |
| [frontend.md](frontend.md) | Design language (theme, components, icons), page and htmx patterns, responsive and performance rules |
| [matches.md](matches.md) | The match layer: scenario (rulebook or one-off), results, who may edit, grants (contract, designed) |
| [aftermath.md](aftermath.md) | The War's Aftermath after a match: sheet, injury rolls, experience, level-ups, influence, scenario rewards, locks (contract, designed) |
| [live-match.md](live-match.md) | Live match mode: trackers, action trackers and experience, Retreated and Field casualties, match log with undo, live sync (spec, designed) |
| [unit-keywords.md](unit-keywords.md) | Race, faction and unit-type keywords on the unit cards: data sources, the computation (`core.UnitKeywords`), chips, tooltips, mounted/dismounted, optional rule-target highlight (spec, designed) |
| [security-and-risks.md](security-and-risks.md) | CSP analysis and plan, ranked risk register (security, data, performance, operations, quality, rules fidelity) |
| [vendored-js.md](vendored-js.md) | The committed third-party scripts: versions, sources, checksums (`internal/web/vendor.json`), `make vendor-check`, update procedure |
| [testing.md](testing.md) | Test commands; the browser smoke tests (`make e2e`): what they cover, isolation, how to write steps |
| [performance.md](performance.md) | Measured page cost: the baseline, what was changed and what it gained, `make perf`, `BenchmarkPages`, what the backend and the database should look at |
| [operations.md](operations.md) | Backups with `pg_dump`, restore and the restore drill, retention, upgrading safely |
| [auth.md](auth.md) | Accounts, sessions, passwords, CSRF, login throttling |
| [admin.md](admin.md) | The Administration section: users and permissions, rules files (default, upload, prune), organiser handover (spec, designed) |
| [demo-world.md](demo-world.md) | `make demo`: the rich demo world, campaign by campaign: what each shows, where to click, and the states the services cannot reach |
| [definitions.md](definitions.md) | The rules data (vanilla, Battle Companies, glossary): format, conventions, tooling |
| [review-register.md](review-register.md) | Every open REVIEW default and assumption the agents recorded, by area, with how to change it; settled decisions at the end |

`tools/README.md` has the step-by-step commands for the definitions tooling.
