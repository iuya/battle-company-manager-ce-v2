# REVIEW register

Every default the agents built while a question was still open, in one place for the owner. Compiled 2026-10-21 from the
`REVIEW:` notes in beads (`bd search REVIEW`, label `review`, notes of closed beads under the epics pib, mtu, amy, mqn, rzt,
csg, 7v5 and the earlier ones), the `REVIEW` sections of `docs/`, and the saved user decisions
(`bd memories user-decisions`). Bead ids are written without the `battle-company-manager-ce-v2-` prefix; read one with
`bd show battle-company-manager-ce-v2-<id>`.

**Status**: *open* = nobody has confirmed it, the owner decides; *confirmed* = the owner confirmed the default; *changed* = the owner decided differently and a bead holds the work; Rows the owner skipped because they follow the rulebook are removed and listed in Confirmed; *superseded* = a later decision or build replaced it;
*unverified* = could not be checked against code or data. Items the owner already settled are only in
[Confirmed](#confirmed) at the end. To change a default, update the value or rule in the place named in the "Change"
column, then add a bead note and adjust this row.

Spot-checked against the code or data before writing (marked **V**): starting influence, rules cache cap, invite lifetime,
grant limits, scenario text limits, HSTS, CSP mode, APP_ENV default, campaign end/reactivate rules, reopen permission,
Rendezvous timing in the JSON, invite refusal on finished campaigns, timezone setting. Everything else is copied from the bead
or doc notes and is not re-verified.

## Start here (top 10)

Answered 2026-10-22 (`docs/review-register-reviewed.md`, memory `user-decisions-2026-10-22-register`): 1 R1 rulebook (Elven roll-off only), confirmed and removed; 2 C1 confirmed; 3 R2 still open, the four readings are being listed (`5p8`); 4 M1 changed, epic `6uh`; 5 A1 confirmed; 6 R4 confirmed and removed; 7 R7 changed, `0mu`; 8 S1 changed, `bnm`; 9 L1 confirmed; 10 R5 confirmed. Remaining rows are open.

## Rules data

| ID | Entry | Default built | Why | In the app | Change | Status |
|---|---|---|---|---|---|---|
| R2 | Four curator readings | Curator guesses applied; Crossbow is S4 (24in) | No clear source text | Affects the gear shown for those units | `rzt.2` (the four readings are listed there; not itemised here, unverified); `5p8` | open (Crossbow S4 24in confirmed 2026-10-22; the four readings are being listed for you, `5p8`) |
| R3 | Hunter Orc Warrior fill-ins | Race Orc, Warrior + Infantry, base 25mm, copied from the Warg Rider | Vanilla entry incomplete | Shows these values on the unit card | `amy.34`; `data.unitFillIns` (applied only where vanilla is empty) | confirmed 2026-10-22 |
| R5 | Vanilla duplicate names | First entry wins for ~11 ambiguous names (Alfrid, Radagast, Thrain, Rosie, One of Nine ...) | Loader needs one value | Possibly wrong stats on those profiles | `2il`; `DUPLICATE_DECISIONS` in `tools/bc/core.py`, `data.vanillaAmbiguities`; `7bg` | confirmed 2026-10-22 (first wins in general; old-edition remains dropped by heuristic, `7bg`) |
| R6 | Cave Troll options | Troll Spear read as Spear, Hand-and-a-half hammer as Hand-and-a-half Weapon | Option name differs from armoury name | Trolls may buy the armoury item | `docs/definitions.md` (exchange options); `7v5.11` area; `tfo` | changed 2026-10-22: Troll Spear is its own item, `tfo` (hammer reading confirmed) |
| R7 | Units with no options | Black Numenorean, Corsair and similar units buy nothing | New strict identity rule | Fewer purchases than before; list in `amy.6` notes (a local file, unverified) | `amy.6`; `internal/core/armoury.go`; `0mu` | changed 2026-10-22: units with no options buy equipment only, heroes use the hero pool, `0mu` |
| R8 | Iron Shield and Pavise | Both Defence +3 (Pavise not while engaged; Iron Shield loses duel dice unless supported by a Foe Spear) | Gear annotation review | Shown in the gear tooltip and the stats | `wgv`; `v4v` | changed 2026-10-22: conditional bonuses are item text only, not in the stats, `v4v` |
| R9 | Non-learnable powers | The 12 non-learnable powers have no base value | Vanilla units disagree per unit | Sorcerer path cannot offer them | `6tx` | confirmed 2026-10-22 (stay out of the Sorcerer path; custom campaigns may add them) |
| R10 | Canonical wargear spelling | Armoury name first, else the sorted-first vanilla spelling | Several spellings per item | Wording of item names | `amy` (epic notes) | confirmed 2026-10-22 |
| R11 | Path summaries | Composed from the path table, no invented prose; signature stats = stats improved on the Mastery row | No prose in source | Path page wording | `rzt.7` | confirmed 2026-10-22 |
| R12 | Shire Spear for heroes | Closed as review; outcome not recorded in my sample | Ranger of the North option is a start unit | Shire heroes may not buy a Spear | `amy.30`; `333` | changed 2026-10-22: Shire heroes may buy a Spear, `333` |
| R13 | Unbuilt rules | Shire rangerReplacement, WitW recruitAsHero, lostStartingUnit are informational only; epic 36y holds Healing Herbs, Wondrous Cram, Tool Kit, Temporary Wanderer, Ambush, Break Point in play | Not in scope yet | Players apply them by hand | `7v5.22`, `36y`; `wcu` | changed 2026-10-22: implement the informational effects (check which exist), `wcu`; 36y specs written |
| R14 | Rendezvous timing | JSON says "right after this match" | Your decision | Bonus in the aftermath of the same match | `gm1` | confirmed 2026-10-22 (was superseded) |
| R15 | Elf limit at Helm's Deep | Worked out on the maximum size, rounded down | Early reading | Replaced by "round up, recruit counted" (5 Elves at 15) | `amy.44`, `pib.7` | confirmed 2026-10-22 (was superseded) |
| R16 | Fiefdoms knight price 26 vs 20 | Option A (data change) not needed | Price is now base + extras | Iron Hills Warrior Spear alone now 12 | `amy.43`, `pib.6` | confirmed 2026-10-22 (was superseded) |

## Armoury, pricing and limits

| ID | Entry | Default built | Why | In the app | Change | Status |
|---|---|---|---|---|---|---|
| G2 | Starting influence and re-rolls | Both 0 (**V** `core.StartingInfluence = 0`) | Rules file gives a starting company and Gold only | New companies begin without influence or re-rolls; re-rolls come from Against the Odds bonuses | `7v5.19`, `7v5.23`; `internal/core/companies.go` | confirmed 2026-10-22 |
| G3 | Max models | Default 15; every company's own rules override it (12 elite, 20 horde, 25 Shire); recruit blocked at the company's cap | Rules file; the company definition is the only thing that overrides the default | Recruit blocked at the cap | `7v5.20`, `7v5.22`; `internal/core/battlecompanies.go` | confirmed 2026-10-22 (the earlier "no per-company override" was a wrong description) |
| G4 | Hero upgrade swap | A hero upgrade can be swapped once per hero for any result | Rule reading | One swap per hero | `7v5.23` | confirmed 2026-10-22 |
| G5 | Free equipment price | Free equipment ignores price, uses armoury Cost <= maxInfluenceValue; recorded as Bought | Spec had no rule | Grants of gear are limited by cost ceiling | `csg.12.5` | confirmed 2026-10-22 |
| G6 | Item a rule never offers | Buying it is ErrNotFound (not a friendly refusal) | Same as the fake | Rare, forged requests only | `7v5.21` | confirmed 2026-10-22 |
| G7 | "Choice of any" result | Offers only unit results of the chart rolled, special results are not offered | Simplest safe reading | Fewer choices on some charts | `7v5.22`; `dji` | changed 2026-10-22: any result of any chart (normal or special), downgrade allowed, `dji` |
| G8 | Pending roll cost | Spent influence of a pending roll is the attempt cost; extra influence taken on Resolve | Keeps balance honest | Balance shows after Resolve | `7v5.24`; `6qt` | changed 2026-10-22: attempt cost spent at the attempt and not undoable, extra influence after the result, `6qt` |
| G9 | Missing team member | Fewer roster units than the other team member (cemetery not consulted, as the rule says: a team exists only while a member is on the roster) | Avoids a query | Edge case in team rules | `7v5.22`; `nfe` | decided: roster only; whole team gone = no substitution, a normal roll recruits a fresh team (`nfe`) |

## Company management

| ID | Entry | Default built | Why | In the app | Change | Status |
|---|---|---|---|---|---|---|
| CM1 | Other company's page | Compact roster list, not full read-only unit cards | Cheaper first version | Opponents see less detail than your decision (6) asks | `03z` | open |
| CM2 | History lines | No match link or date on aftermath lines | Not in the read model | Hard to trace a line to a match | `ro1`, `csg.12.9` | open |
| CM3 | Using a perk | Only the owner marks a perk used (not the organiser) | Perk is the player's | Organiser cannot mark it for the player | `csg.12.5` (`UsePerk`) | open |
| CM4 | Vantage Point holder | Any company in the match with the unused perk | Spec silent | Any holder may use it | `mtu.10` | open |
| CM5 | New wanderer | Appended as a hero unit with name, id assigned by the repository | Simplest | None visible | `7v5.21` | open |
| CM6 | Injured vs healers | `Injured` and `SentToHealers` both mean "misses next game" | Same effect | None visible | `7v5.18` | open |
| CM7 | Unknown campaign options | Ignored with a log warning | Settings are a fixed struct | A typo in options does nothing | `7v5.20` | open |
| CM8 | Hero in the Making fallback | Warrior becomes Leader with 1 Might/Will/Fate and picks a path | No advancement available | Happens only with no other advancement | `csg.12.4`; `mqn.17` confirms the base rule | open |
| CM9 | Officer panel component | The Ranger reshuffle uses its own Alpine component `officers` (`lists.js`) with partials `officers_panel`/`officer_card`, not the company-creation `commission` component (tied to profile data); same CSS | Creation component is bound to creation data | Two near-identical panels to keep in step | `d1z`; docs/frontend.md | open |
| CM10 | Officer panel drops and pool | A card dropped on an occupied place swaps with it; the Ranger card has a drag handle, no remove button and the pool refuses it; when exactly one officer is dropped (`Drop=1`) the last old officer starts in the pool | Never lose the Ranger; least surprise | Pre-placed arrangement the player can change before Appoint | `d1z`, `kmn`, `eye` | open |
| CM11 | Ranger reshuffle contract | Ranger id 0 (`RangerID`) appears exactly once, as Leader or Sergeant; a Leader is required, at most 2 Sergeants; officers not named are dropped to Hero keeping their progress; the Ranger joins with the role of its place (you decided on 2026-10-22 that he may sit anywhere) | Spec update of 2026-10-22 | Recruit, match creation and the aftermath lock refuse while the arrangement is pending (`PendingOfficers`); My Company shows a banner | `kmn`, `eye`, `d1z` | open |
| CM12 | Succession fallback | A lost Sergeant is replaced by a non-officer Hero (optional) or, with no Hero left, by the Warrior with most XP (ties: Rating, then the player picks) as Hero in the Making who chooses a path; the fallback is mandatory (no "nobody"), a single Warrior is preselected, the page lists XP and points | Rulebook "Death of a Hero" | Aftermath succession step; "nobody left to lead" now shows a message | `6n1.1`, `6n1.2` | open |

## Matches and the live match

| ID | Entry | Default built | Why | In the app | Change | Status |
|---|---|---|---|---|---|---|
| M1 | Ending a match | Any participant can end; no confirmation by other sides; needs a scenario and an explicit outcome | Keeps tables moving | One player can end a game | `spe`, `1ey`; `6uh` | changed 2026-10-22: ending needs confirmation from all participants, epic `6uh` (docs/matches.md) |
| M2 | One-off scenario limits | Name 60, texts 1000 characters (**V** `core/match.go`); changing the scenario clears ticked objectives | Sensible sizes | Longer text is refused | `spe`; `maxScenarioName`, `maxScenarioText` | open |
| M3 | Organiser edits | Organiser edits like a participant while open; admin edit of an ended match does not redo the aftermath | Your organiser decision | See L-entries | `1ey` | open |
| M4 | Draw with a fled company | `FleeConflict` flags only a winning side, not a draw | As the doc says | No warning on such a draw | `csg.1` | open |
| M5 | Scenario objectives | Only tickable reward conditions; relic bearer is a company tick; max 1 | Aftermath derives the rest | Result form shows company ticks only | `csg.7` | open |
| M6 | Attacker/Defender tags | Free early, required only for scenarios with roles; three-sided matches end without tags; a single tag gives the other side the opposite role; roles change only while open | Your tag decision, edge cases chosen | Tag form on setup | `mtu.11`, `mtu.18` | open |
| M7 | Secret objectives | A secret entry notifies nobody; only the author sees it until End; revealed lines print the long text; Hero options show unit titles only | Privacy | Opponents learn nothing until end | `mtu.20`, `mtu.29` | open |
| M8 | Match page layout | Scenario sections collapsed on every width; roles one-line note; draw/VP info tone | No warning token | Extra tap to read the scenario | `mqn.21`, `mqn.22`, `mqn.23` | open |
| M9 | Objective and MVP votes | No secret ballot; teammates not voted; tie = no default, owner picks on the sheet; votes changeable until End; retreated units may get the objective; an already-set flag is refused; the viewer sees the tally only | Table is trusted | Votes visible in the log | `mtu.12`, `mtu.15`, `mtu.16`; `docs/live-match.md` | open |
| L1 | Wound on an enemy mount | Earns the wounding XP; slain monstrous mount counts as a Monster only if the rider is also slain | Rule text plus cavalry note | XP in live matches | `docs/live-match.md` REVIEW section | confirmed 2026-10-22 ("yes for now") |
| L2 | Trackers | Capped at starting value; dismounted units keep the mounted profile; "Broken" is not tracked; mount wound counter only above 1 Wound | Simplest | Players watch Broken themselves | `docs/live-match.md` REVIEW; `36y` | changed: "Broken" is now tracked by the turn button, epic `36y` (docs/live-match.md); the rest open |
| L3 | Return to the battle | Own action kind, not an undo; stat change allowed on Field and wounds-casualty units, refused for retreated/died | Avoids ambiguity | Undo list stays clear | `pib.11` | open |
| L4 | Live window | A full-width window, not a bottom sheet or popover; formation change hidden while live; non-empty zones always open; mount item struck through | Popovers get clipped | Layout on 360 px | `pib.16`, `pib.18`, `pib.19` | open |
| L5 | Live sync | Auth checked at connect only (no per-event re-check); stream max age 2h instead of idle 2h; caps 10/user, 50/match; 25 s keepalive; 10 s poll | Events hold ids only, fetches authorise | A removed member's open stream lasts up to 2h | `pib.14`; `docs/architecture.md`, `docs/live-match.md` | open |
| L6 | Live log storage | Undo of an unknown entry is ErrNotFound; log row needs Init first | Foreign key shape | None visible | `pib.10` | open |
| L7 | Aftermath badge | "From the live match" badge on the whole unsaved default, not per value | Simpler | Sheet badge | `pib.11`, `pib.22` | open |
| L8 | Live script | Loaded on every page, connects only where a live part exists | Tiny | None | `pib.21` | open |
| L9 | Log time display | Server zone, minute headings | Early | Superseded by the user timezone setting | `pib.20`, `mtu.13` | superseded |

## Aftermath

| ID | Entry | Default built | Why | In the app | Change | Status |
|---|---|---|---|---|---|---|
| A1 | XP of absent, dead and retired units | Absent units earn no participation XP and recover at Start; dead or retired units get none | Reading of the rules | Fewer XP after a lost unit | `csg.12`, `csg.12.3`; `docs/aftermath.md` | confirmed 2026-10-22 (units not active in the match and dead units earn no XP) |
| A2 | Aftermath per company | One per company; owner, organiser or admin may do it; waive = skip all with a note | Locks the company until done | Company locked for recruit, buy, drop, heal | `csg.12` | open |
| A3 | Reopening a match | Refused once any company started; waiving counts as started | Keeps results stable | Cannot undo a result after the aftermath | `csg.12.4`, `7v5.27` | open |
| A4 | Re-rolls on the injuries step | The step waits for "continue" while Against the Odds re-rolls remain; no explicit confirm elsewhere | A re-roll could not be used after "Roll all" | One extra click | `csg.12.3`; `docs/aftermath.md` | open |
| A5 | Progression not forced | Player may continue with rolls waiting | Rules put progression first, app allows both | Rolls can stay pending | `docs/aftermath.md` | open |
| A6 | "Per model" XP | Read as per participating model | Wording | Experience totals | `docs/aftermath.md` | open |
| A7 | Against the Odds "bonuses" | Read as the Against the Odds bonuses | Spec wording | See Confirmed (fled company) | `mtu.5` | open |
| A8 | Casualty flow actors | `Bury` and `Injure` take no user (system calls); casualty flow must authorise | Layering | None | `7v5.25` | open |
| A9 | Ratings hint | Two-sided ratings shown as a hint, otherwise trusted | No dispute tool | Players type ratings | `docs/aftermath.md` | open |

## Campaign, admin and accounts

| ID | Entry | Default built | Why | In the app | Change | Status |
|---|---|---|---|---|---|---|
| C1 | Ending a campaign | Organiser or admin may end; one typed campaign name plus warnings; invites revoked; matches frozen (**V** `SetCampaignStatus`); only admins reactivate | You asked for "a series of confirmations" | One confirm step, not several | `mtu.7`, `mtu.21`, `mtu.26`; `internal/core/campaigns.go` | confirmed 2026-10-22 (as built) |
| C2 | Invite defaults | 7 days, unlimited uses (**V** `InviteLifetime`); finished campaigns refuse the link even for members (**V**); paused refuse new members | Common defaults | Link expires after a week | `7v5.15`, `ht1`, `7v5.26`; `internal/core/invites.go` | open |
| C3 | Grants | Gold +-1000, Influence and Experience +-100 (**V** `grant.go`); a note is required; result below 0 refused; XP goes to a named unit | Typo guard | Organiser awards are capped | `csg.10`, `e3e`; `internal/core/grant.go` | open |
| C4 | Handover | Organiser handover from the campaign overview only; candidates are owners of the campaign's companies; same person refused; finished campaigns allowed | Simplest | Admin Users row lacks it | `49v`, `3je` | open |
| C5 | Closing an account | Anonymise (keeps companies and matches); refused for organisers of unfinished campaigns and the last superuser; password change ends other sessions; email not editable | No email delivery | Players cannot change email | `0p9`, `0p9.1`, `0p9.2` | open |
| C6 | No account forms | No create, reset or delete in the UI, CLI only | No email delivery | Admin runs `make user` | `j62`; `docs/admin.md` | open |
| C7 | Rules files admin | Prune removes shipped files too (re-added at start); no "replace edition", delete then upload; upload hashed on the server | Harmless and simple | Two-step replace | `j62`, `r1b`, `9c4`, `zi4` | open |
| C8 | Glossary links | Case rule for every single-word term (name or alias) | Avoids linking "will" | Some words not linked | `2p6`, `2p6.1` | open |
| C9 | Appearance | Signed-in preference wins over cookie; a new account adopts the cookie | Predictable | None | `ubv` | open |
| C10 | Time zone | Account setting, else cookie, else `APP_TIMEZONE`, else UTC (**V** env exists) | Your timezone decision | Log times | `mtu.13` | open |
| C11 | "Two companies" threshold | A warning that needs two companies in the campaign (not two owners) | Count what is cheap | Edge case | `mqn.127` | open |

## Security and operations

| ID | Entry | Default built | Why | In the app | Change | Status |
|---|---|---|---|---|---|---|
| S1 | Insecure cookies in production | Warn, do not refuse (`SECURE_COOKIES=false` with https) | Avoid bricking a deploy | A misconfiguration starts silently | `mqn.161`; `internal/web/hardening.go`; `bnm` | changed 2026-10-22: refuse insecure cookies in production, `bnm` |
| S2 | APP_ENV default | Production (**V**): Secure cookies and HSTS; `make run/dev/demo` set development; bare `go run` on http://localhost gets Secure cookies (not Safari) | Safe by default | Set APP_ENV=development locally | `mqn.161`; `internal/web/server.go` | open |
| S3 | HSTS | max-age 1 year, no includeSubDomains or preload (**V**) | Reversible | Browsers remember HTTPS | `mqn.161`; `hardening.go` | open |
| S4 | Migrations on start | `MIGRATE_ON_START` true with a goose advisory lock | Single-binary deploy | App migrates on boot | `mqn.161` | open |
| S6 | Rules cache cap | 16 rule sets (**V** `MaxCachedRuleSets`) | Memory bound | Cold loads when over 16 | `mqn.167`; `internal/core/rules.go` | open |
| S7 | Hostile-string audit | Stdlib regex tag scanner, aftermath option names not reached by the stubs | No new dependency | Test gap | `mqn.165` | open |
| S8 | Storage | Rules stored as `bytea` (exact hashed bytes); schema lib santhosh-tekuri/jsonschema/v6; embed package in `definitions/` | Exactness, single binary | None | `7v5.9`, `7v5.10` | open |
| S9 | Balances | Negative balances refused in SQL (`ErrNegativeBalance`) | Safety | Message on grant | `7v5.27` | open |

## Demo world

| ID | Entry | Default built | Why | In the app | Change | Status |
|---|---|---|---|---|---|---|
| D1 | Demo accounts | Password `demo-password` (override `PASSWORD=`), 37 gallery accounts `<company>@example.com`, `admin@example.com` superuser | Quick tour | Demo only | `mqn.1`, `7v5.26` | open |
| D2 | Second edition | A fork of the Battle Companies file with another name/version, stored via `Rulesets.StoreAs` | Shows several editions | Demo only | `mqn.1` | open |
| D3 | Seed | Not transactional; half-seeded needs `make db-reset` | Simple script | Demo only | `7v5.26` | open |

## Counts

| Area | Open | Confirmed | Changed | Superseded |
|---|---|---|---|---|
| Rules data | 1 | 8 | 5 | 0 |
| Armoury, pricing, limits | 1 | 5 | 2 | 0 |
| Company management | 12 | 0 | 0 | 0 |
| Matches and live match | 14 | 1 | 2 | 1 |
| Aftermath | 8 | 1 | 0 | 0 |
| Campaign, admin, accounts | 10 | 1 | 0 | 0 |
| Security and operations | 7 | 0 | 1 | 0 |
| Demo world | 3 | 0 | 0 | 0 |

Total 83 entries in the tables: 56 open, 16 confirmed, 10 changed (work in beads), 1 superseded. R1 and R4 were removed on 2026-10-22 (they follow the rulebook). The Confirmed table below lists the settled decisions.

## Confirmed

Settled by you; recorded so nobody reopens them. Source: `bd memories user-decisions-*`.

| Date | Decision | Where it lives |
|---|---|---|
| 2026-10-20 | Rivendell Warrior spear is an Elven spear; Dwarf two-handed axe is a Two-handed weapon; Mordor/Hunter Orc bow is an Orc bow; Uruk-hai never had a Mattock option; Elven Glaive and Wood Elf Spear have their own rules | rules data, `rzt.3`, `rzt.13` |
| 2026-10-20 | Easterling Halberd at 1 influence in the hybrid-weapons row; Two-handed axe aliases Two-handed weapon | rules data |
| 2026-10-21 | No vanilla chooseOne exclusivity; unit price = base + extras; limits round up and count the recruit; armour tiers replace the current armour; heroes buy only tiers their company has; oath texts are flavour; other companies are read-only views; live match epic | `pib.6`, `pib.7`, `pib.8` |
| 2026-10-21 | Smoke Bombs: Corsair units; War Camel: the unit Mahûd Warrior only; rules have key and label (app shows the label); player chooses among promotion rows; Cave Trolls and Crebain are On Their Own Path; Beast equipment list; Feral Charge for Warg heroes only (supersedes the mount reading) | `amy.41`, `amy.58`-`amy.61` |
| 2026-10-21 | A wound on another company's unit applies at once; a company must finish its aftermath before a new match; creating a match starts live mode | `pib.23`-`pib.25` |
| 2026-10-21 | A fled company gets both the XP and the influence bonus | `mqn.114` |
| 2026-10-21 | Paused campaign: open matches may finish, no new ones, no invite joins | `mqn.137` |
| 2026-10-21 | Rules files identified by hash; same name and version can coexist | `mqn.92` |
| 2026-10-21 | Campaign buttons for admin and organiser: pause, end, invalidate and regenerate invites; ended campaign reopened by admins only, matches frozen | `mqn.3`, `mtu.26` |
| 2026-10-21 | Attacker/Defender tags, secret objectives logged by users, no join requests | `mqn.4` |
| 2026-10-21 | Rendezvous applies in the aftermath right after the match; perk +1/-1 built into the rolls | `mqn.120`, `gm1` |
| 2026-10-21 | Choice of Options limited to armoury items; profile wargear outside the armoury is added on recruit but never bought; Small Shield is equipment (+1 Defence against Strikes) | `mtu.2`, `mqn.111` |
| 2026-10-21 | Hero rating per the BC "Calculate Rating" rule (gear outside profile and options costs the Armoury values once Attacks + Wounds >= 3) | rating code |
| 2026-10-21 | Organiser manages the campaign like an admin (edit and reopen ended matches); grants by admins and organiser only; waive = skip the whole aftermath | `mtu.6`, `csg.12` |
| 2026-10-21 | Live match: objective check and MVP vote at match end, both give XP; state survives restarts (Postgres is the truth); full log kept and reviewable forever | `mtu.12`, `pib.9`, `docs/live-match.md` |
| 2026-10-21 | Log times stored in UTC, shown in the user's timezone | `mtu.13` |
| 2026-10-21 | Lake-town spelling; no "take the promotion instead" for units without advancement | `amy.56`, `mqn.17` |
| 2026-10-21 | CSP: report-only first, enforce about a week after real use; very low priority otherwise | `mqn.164`, `mqn.94` |
| 2026-10-22 | Follow the rulebook always (R1 Elven spear: roll-off only, no Fight Value bonus; R4 Phalanx lifts the pike penalty with a shield, but buying a bow brings the shield penalty back because of the bow, check `0yo`); rows that follow the rulebook are reviewed | rules data, `0yo` |
| 2026-10-22 | Crossbow is S4 with 24in range (data2024.json); R3 Hunter Orc fill-ins, R10 spelling, R11 path summaries, R14 Rendezvous, R15 Elf limit, R16 Fiefdoms price, G2 starting influence, G4 hero upgrade swap, G5 free equipment price, G6 item never offered: confirmed | rules data |
| 2026-10-22 | Each company's rules override the 15 model default (12 elite, 20 horde, 25 Shire); the company definition is the only override | G3 |
| 2026-10-22 | Ending a campaign as built is good; units not active in a match (missed through injury) earn no XP, dead units take part in nothing; a wound on an enemy mount earns XP (for now); duplicates are remains of old editions, keep the first in general | C1, A1, L1, R5 |
| 2026-10-22 | **Ending a match requires confirmation from all participants** (override by organiser/administrator for an absent one) | epic `6uh`, docs/matches.md |
| 2026-10-22 | Units with no options buy equipment only, heroes use the hero pool; conditional stat bonuses stay item text; Troll Spear is its own item; Shire heroes may buy a Spear; informational effects are implemented; refuse insecure cookies in production; non-learnable powers stay out of the Sorcerer path | `0mu`, `v4v`, `tfo`, `333`, `wcu`, `bnm` |
| 2026-10-22 | "Choice of any" is from any chart and may be downgraded; the attempt cost of a pending roll is spent at the attempt and cannot be undone, extra influence comes after the result | `dji`, `6qt` |
| 2026-10-22 | Mirkwood Warg Marauder counts as 3 models for the Break Point; Tame the Beast budget uses ratings before Against the Odds | `36y.18`, `36y.19`, `36y.12` |
