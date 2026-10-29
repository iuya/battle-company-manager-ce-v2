# Battle Companies 2nd CE Manager: documentation

A manager for the *Middle-earth Strategy Battle Game* **Battle Companies 2nd Community Edition** game mode: campaigns, companies and matches. These pages describe the app as it is. Pages
that describe planned work say so in their first lines. The tables below group them by who they are for; the same map is in the [README](../README.md#documentation).

## Players and organisers

| Document | What it covers |
|---|---|
| [demo-world.md](demo-world.md) | The demo world: what each campaign shows, which account to use and where to click |
| [matches.md](matches.md) | Matches: scenario, results, Against the Odds, ending a match by confirmation, who may edit |
| [aftermath.md](aftermath.md) | The War's Aftermath: injuries, experience, level-ups, influence, rewards |
| [live-match.md](live-match.md) | Live match mode: trackers, casualties, Break Point, end checks, log with undo |
| [unit-keywords.md](unit-keywords.md) | Race, faction and unit-type keywords on the unit cards, and who a rule works against |
| [aura-ranges.md](aura-ranges.md) | Aura ranges: which rules draw a ring, the chips on the cards, the rings in Tabletop Simulator |
| [tabletop-simulator.md](tabletop-simulator.md) | Taking a company into Tabletop Simulator: army list, figure names and descriptions, the helper |
| [admin.md](admin.md) | The Administration section, campaign status, invitations, handover |
| [rules-coverage.md](rules-coverage.md) | Which Battle Companies mechanics the app implements, with the tests |
| [multi-company-spec.md](multi-company-spec.md) | Planned: several companies per player |
| [practice-match-spec.md](practice-match-spec.md) | Practice matches: a flag on a match set on the creation form, no rewards, no injuries, an empty derived aftermath (implemented) |

## Rules authors

| Document | What it covers |
|---|---|
| [customizing-rulesets.md](customizing-rulesets.md) | A guide to customising the Battle Companies rules file, step by step |
| [customizing-rulesets-reference.md](customizing-rulesets-reference.md) | The reference of every setting of the rules file |
| [definitions.md](definitions.md) | The rules data (vanilla, Battle Companies, glossary): format, conventions, tooling |
| [concept-graph.md](concept-graph.md) | The concept graph behind the helper popups |
| [aura-ranges.md](aura-ranges.md) | How aura rings are defined in the data |
| [tools/README.md](../tools/README.md) | Step-by-step commands of the definitions tooling |

## Developers

| Document | What it covers |
|---|---|
| [collaborating.md](collaborating.md) | How to work on the project, with or without a coding agent |
| [architecture.md](architecture.md) | Layers, packages, persistence, the live match design, configuration, commands |
| [frontend.md](frontend.md) | Design language, page and htmx patterns, responsive rules, the screens |
| [testing.md](testing.md) | Test commands, golden snapshots, the browser tests |
| [auth.md](auth.md) | Accounts, sessions, passwords, CSRF, login throttling |
| [vendored-js.md](vendored-js.md) | The committed third-party scripts and how to update them |
| [performance.md](performance.md) | Measured page cost and the performance tooling |

## Operators

| Document | What it covers |
|---|---|
| [operations.md](operations.md) | First start, the published image, Content-Security-Policy modes, backups, restore, robustness |
| [architecture.md](architecture.md) | Configuration variables, deploying, migrations |
| [admin.md](admin.md) | Users, roles and rules files in the Administration section |
| [auth.md](auth.md) | Creating accounts and the sign-in rules |
| [SECURITY.md](../SECURITY.md) | How to report a vulnerability |
