# Battle Companies 2nd CE Manager: documentation

The index of the documentation of the Battle Companies 2nd CE Manager, an app for running campaigns, companies and matches of the *Middle-earth Strategy Battle Game* **Battle Companies 2nd Community Edition** game mode.

These pages describe the app as it is.
A page that describes planned work says so in its first lines.
The tables group the pages by who they are for; the same map is in the [README](../README.md#documentation).

## Players and organisers

| Document | What it covers |
|---|---|
| [demo-world.md](demo-world.md) | The demo world: what each campaign shows, which account to use and where to click |
| [matches.md](matches.md) | Matches: scenario, results, Against the Odds, ending a match by confirmation, who may edit |
| [aftermath.md](aftermath.md) | The War's Aftermath: injuries, experience, level-ups, influence, rewards |
| [live-match.md](live-match.md) | Live match mode: trackers, casualties, Break Point, objective and MVP, log with undo |
| [unit-keywords.md](unit-keywords.md) | Race, faction and unit-type keywords on the unit cards, and who a rule works against |
| [aura-ranges.md](aura-ranges.md) | Aura ranges: which rules draw a ring, the chips on the cards, the rings in Tabletop Simulator |
| [tabletop-simulator.md](tabletop-simulator.md) | Taking a company into Tabletop Simulator: army list, figure names and descriptions, the helper |
| [admin.md](admin.md) | The Administration section, campaign status, invitations, handover |
| [rules-coverage.md](rules-coverage.md) | Which Battle Companies mechanics the app implements, with the tests |
| [multi-company.md](multi-company.md) | Several companies per player (up to a campaign setting), the company in the URL, and opening the last campaign directly |
| [practice-matches.md](practice-matches.md) | Practice matches: a flag on a match set on the creation form, no rewards, no injuries, an empty derived aftermath |

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
| [frontend.md](frontend.md) | Design language, tokens, themes, responsive rules, components, page and htmx/Alpine patterns, and the map of the screen pages |
| [frontend-company.md](frontend-company.md) | My Company: the roster, unit cards, Armory, equipment, healing, formation, Gold, creatures |
| [frontend-company-building.md](frontend-company-building.md) | Recruiting, the creation page, choosing a company, company rules |
| [frontend-level-up.md](frontend-level-up.md) | Choosing a path (mosaic and carousel), the magical power window, level-up |
| [frontend-live-match.md](frontend-live-match.md) | The live match screens |
| [frontend-glossary.md](frontend-glossary.md) | The Glossary tab, unit-name links, the helper popup |
| [frontend-campaigns.md](frontend-campaigns.md) | Campaign list and pages, rules files and the validator, hand-over |
| [frontend-account.md](frontend-account.md) | Account, invitation and recovery links, the demo sign-in |
| [testing.md](testing.md) | Test commands, golden snapshots, the browser tests |
| [auth.md](auth.md) | Accounts, sessions, passwords, CSRF, login throttling |
| [vendored-js.md](vendored-js.md) | The committed third-party scripts and how to update them |
| [performance.md](performance.md) | Measured page cost and the performance tooling |

## Operators

| Document | What it covers |
|---|---|
| [operations.md](operations.md) | First start, configuration, accounts and the map of the operations pages |
| [releases.md](releases.md) | The published image, tags, how a release is built, signed images, rolling back an image |
| [deploy-server.md](deploy-server.md) | The server kit: first start on a server, the updater, memory, rollback |
| [backup-restore.md](backup-restore.md) | What to back up, backups, restore and the restore drill |
| [csp.md](csp.md) | Content-Security-Policy: report mode, then enforce |
| [robustness.md](robustness.md) | Goroutines, LISTEN/NOTIFY, server limits, the database, boot versus run |
| [architecture.md](architecture.md) | Configuration variables, deploying, migrations |
| [admin.md](admin.md) | Users, roles and rules files in the Administration section |
| [auth.md](auth.md) | Creating accounts and the sign-in rules |
| [SECURITY.md](../SECURITY.md) | How to report a vulnerability |
