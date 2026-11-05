# Simpler matches (spec)

Status: **implemented, awaiting the owner's manual test: all five phases written, Go tests and docs done, e2e flows updated** (see section 8, "Progress"). Branch: `feat/simplify-matches` (all phases are developed on this branch, one commit or more per
phase, Conventional Commits). The owner agreed every decision below in a design session on 2026-11-03; phases 1 to 5 are the
order of work. When a phase lands, update this status line and the docs it names, and keep [matches.md](matches.md),
[live-match.md](live-match.md) and [aftermath.md](aftermath.md) describing the app as it then is. Related:
[practice-matches.md](practice-matches.md), [multi-company.md](multi-company.md), [demo-world.md](demo-world.md).

Why: a match takes too many clicks, and every page shows more than the player needs at that moment. Measured at 375 px on the demo
world before the change: the open match page is ~4,500 px tall (nine cards at once: two side cards, scenario, roles, Against the Odds,
End checks with one "No" row per model of every company, result, history, log); the aftermath sheet is ~7,900 px (six large gain chips per
unit even when it earned nothing); the new match form asks for the number of sides, side names, roles and an all-vs-all flag for a plain
1 v 1. The goal of a normal game is three presses after the table: **End match → Confirm → Start aftermath**, stopping only for real
decisions.

## 1. Decisions (all agreed)

| # | Decision |
|---|---|
| D1 | **No Attacker / Defender roles** anywhere: not at creation, not on the match page, not required to end. Players settle roles at the table. |
| D2 | **No side names**: a side is shown by its companies ("Rohan + Gondor"). |
| D3 | **No Victory Points and no "Fled the battlefield" (All is Lost)** in the result. The result is the winner (or a draw) and an optional note. All is Lost is handled by the players: they report the other side as the winner, which already takes the Win / Draw influence and the scenario rewards from the fled company (it keeps the participation influence). The app applies no flee penalty of its own and the live All is Lost offer goes (D14). |
| D4 | **Scenario at creation**: the new match form shows a scenario the server already rolled (both dice, never typed or thrown, whatever `ManualRolls` / `ShakeTheTable` say), with **Roll again** (both dice, no question), **Choose another** (today's list grouped by table; one press sets it) and **One-off scenario** (today's form). The match is created with its scenario. The match page keeps **Change scenario** (same three actions) until the game starts (first live entry), as Against the Odds does. |
| D5 | **Got the objective card removed**. The objective is set from the unit cards' Objective milestone only (already a live `objective` action). |
| D6 | **MVP is chosen with a searchable dropdown** per opposing company (the same combobox as the company search of the new match form). |
| D7 | **MVP is mandatory to end a competitive match**: every company owner votes one unit of every opposing company before proposing or confirming. "No MVP" is no longer offered. Practice matches need no MVP (they award nothing). The organiser's / administrator's override end (`OverrideEnd`) still ends without votes. |
| D8 | **Both sides choose their MVP**: the proposer votes in the End match sheet, the other owners vote in the Confirm sheet. |
| D9 | **Against the Odds on the match page only**: the new match form has no block, whoever creates the match. The lower rated player, the organiser or an administrator decides on the match page during Preparing, with bonuses or none; nothing can be noted live and the match cannot end until then, unless the gap pays for no bonus. A change of the scenario reopens the decision, and Ambush! fixes the scenario to the card ([matches.md](matches.md#against-the-odds)). |
| D10 | **No turn tracking**: no **Start turn N**, no turn number, no `turn` entry. A company is Broken as soon as its casualties exceed its Break Point; the banner of every viewer shows it at once ("Minas Morgul is broken") and it stops being Broken if an undo takes it back under. |
| D11 | **The live company page keeps what the game needs.** Later changes took out the stats, Influence, Gold, limits and the match log, and turned the selector into one chip per company with **Go to match**. Its live banner: a **View ▾** selector to the other companies of the match (each opens that company's page in match mode; a "← Back to mine" link there), To wound, Match log and **End match**; the End checks button and the turn row go. |
| D12 | **Match list**: a **Your turn** list on top of the Matches page and on the campaign Overview ("Confirm the end of Match 25", "Vote the MVP of Match 25", "Start the aftermath of Match 3"); filters **Mine / Open / Ended** instead of All / Practice (practice stays a badge on the row); lower rows: "Rohan vs Mordor · Hold the Line · Open" in one or two lines. |
| D13 | **Compact campaign header** on every inner campaign page: the title on one line, the description only on the Overview. |
| D14 | The live **All is Lost offer** (`LiveBreak.Offer`, the flee action that retreats a whole company) is removed with the turns; units still retreat one by one ("Left the board"). |
| D15 | **Aftermath**: empty steps are skipped (no casualty: no Injuries step; no level-up waiting: no Level-ups step); the sheet of a match played live is a **review** of what the live log produced (each unit shows only what it earned, "+ add" for the rest); Against the Odds shows only the bonuses taken; the sheet **saves itself** and has one button, **Start aftermath**; the aftermath **finishes on its own** when nothing is left to decide (no Finish press), and ends on a **summary of changes** ("+7 XP, +5 Influence, 1 dead, 2 level-ups ready"). Injuries are still rolled one by one by the player (the excitement is wanted); the level-up window stays as it is. |
| D16 | **Company search in the new match form**: the "Mine" chip goes; the user's own companies are highlighted (accent border and a "Yours" badge) and listed first; once one of them is on a side, on the other sides they are listed last instead (playing yourself is rare; the existing "Same player" note stays). |
| D17 | **End match sheet reviews the objective**: before proposing, the proposer sees for each of their companies which units have the objective experience, as chips they can switch on and off (each switch is today's live `objective` action, so it is logged and undoable); the confirmer does the same for their own companies in the Confirm sheet. |
| D18 | **Scenario reward ticks stay, compact**: the ticks of the result that feed the aftermath rewards (`CompanyResult.Objectives`: the Relic, all the cargo, all the objective markers) are kept, only for scenarios that have such rewards, as one compact line per company in the End match sheet (and the Confirm sheet for the confirmer's own companies). Scenarios without rewards show nothing. |

## 2. Phase 1: new match

Behaviour:

- Form (`pages/match_new.gohtml`, `static/js/match_form.js`, `handleNewMatchPage` / `handleCreateMatch` in `internal/web/campaign.go`):
  two side blocks, each with the company search only (no name, no role, no number-of-sides select).
- One options block under the sides: **Practice match** checkbox, **+ Add side** button (adds a side block, up to `maxSides`; an added side
  has a remove control), **All vs all** checkbox (every company on its own side; at least two companies).
- Company search (D16): no "Mine" chip (`lists.js` / `match_form.js`), own companies highlighted and first, and last on the other sides once
  one is placed.
- Scenario card (D4) inside the form, after the sides: shows the rolled scenario (name, table, the two dice) with Roll again / Choose another /
  One-off scenario. The roll needs no match: a campaign-level endpoint (for example `POST /campaigns/{id}/matches/new/scenario-roll`) rolls with
  `ScenarioSelection` and `core.ScenarioByRolls` and returns the card; nothing is written until **Create match**. The chosen scenario is posted
  with the form (`scenario-kind`, `scenario-name`, and the one-off fields).
- No Against the Odds on the form (D9).
- `core.NewMatch` gains `Scenario *ScenarioInput`; `CreateMatch` validates and stores it in the same transaction as
  the match. `NewSide.Name` and `NewSide.Role` are
  no longer read from the form (the core may keep the fields for old data; `ValidateSides` still names sides "Side N" internally, and
  `MatchSide.Label` is what the pages show).

Acceptance:

- A 1 v 1 is created with: pick own company (highlighted, first), pick rival, Create match; the match page shows the rolled scenario.
- Roll again changes the scenario without writing anything; Choose another and One-off replace it; the created match carries what the form showed.
- The form has no Against the Odds block, whoever creates the match.
- With a gap of 40 the block shows Reroll, Influence, Experience (not Wanderer 45, not Ambush! 60) and the saved match has the selection.
- 360 / 768 / 1280 px, light and dark, no horizontal scroll.

Demo world and tests: the "Matches: every scenario and every state" campaign keeps matches created through the new input (scenario at
creation), two with a gap of 60 that wait for or have Against the Odds confirmed (`cmd/server/world_matches.go`); e2e `40-match`, `84-match-extras`, `98-several-companies`
and `100-practice-match` updated, a new flow for the form (roll again, choose another, own companies order). Docs: [matches.md](matches.md)
(match page header, scenario picker, new match), [multi-company.md](multi-company.md) (the Mine chip).

## 3. Phase 2: match page

Behaviour (`pages/match.gohtml`, `internal/web/match.go`, `match_cards.go`):

- **Only the current phase is open**:
  - *Preparing* (no live entry yet): scoreboard, scenario card (Change scenario), Against the Odds card when eligible.
  - *Playing* (a live entry exists, not ended): scoreboard, a large **Go to my company** button (to the live company page), To wound,
    **End match**; the scenario collapsed to its name with its sections inside.
  - *Ending* (an end is pending): the pending banner with Confirm for who must confirm (phase 3).
  - *Ended*: scoreboard with the result, the aftermath card, the note.
  The other cards are collapsed under their heading or absent.
- **Scoreboard** instead of the side cards: one compact row per side, its companies (links), the result badge; no VP, no role badges.
- **No roles** (D1): the Roles card, `handleSetRoles`, `handleSwapRoles`, `ShowRoles` / `AnyRole` and the role requirement of `checkEnd`
  (`internal/core/match_end.go`) go; `SetSideRoles` / `SwapSideRoles` and their routes are removed. Scenario objectives limited by `side` are
  no longer split by role in the page.
- **No Got the objective / End checks card** (D5) on the match page.
- **History and Match log together** in one collapsed **Log** card at the bottom.
- Secret objectives (Fog of War) stay in the scenario card.

Acceptance: the open match page of the demo world is at most about two screens at 375 px in the Playing phase; nothing about roles is in the
HTML; ending a scenario that had roles works without them. Demo world: the role states of `world_matches.go` go; docs-check of
[demo-world.md](demo-world.md). Docs: [matches.md](matches.md) ("The match page", "Team tags" section removed).

## 4. Phase 3: ending a match

Behaviour (`internal/core/match_end.go`, `live_end.go`, `live_end_banner.go`, `match.go` `handleEnd*`, `static/js/match_result.js`):

- **End match** (match page and live banner) opens one sheet: the winner as buttons (each side by its companies, plus Draw; for more than two
  sides one per side), the **MVP** of every opposing company (D6, searchable dropdown of its units that took part: Field, Retreated or Field
  casualty), the **objective review** of the proposer's own companies (D17), the scenario reward ticks when the scenario has rewards (D18),
  and the optional note. **End match** in the sheet saves the result, the votes and the objective changes and
  proposes the end in one request (today `SaveResult` + `mvpVote` actions + `ProposeEnd`; one handler, one revision check).
- The other owners get the banner "Hador proposes to end: Rohan won" with **Confirm**, which opens the Confirm sheet: their MVP votes for every
  opposing company, the objective review and the reward ticks (D18) of their own companies, and **Confirm** (votes + `ConfirmEnd`). **Change result** in that sheet edits the
  winner and resets the confirmations as today (`resetEnd`).
- `CanEnd` / `checkEnd` refuse a competitive match while a company owner has not voted for every opposing company (D7), with the reason
  naming who is missing; practice matches skip the check; `OverrideEnd` is unchanged.
- The result form loses Victory Points (`SideResult`, `SideVP`, `maxVictoryPts`), Fled (`CompanyResult.Fled`, `FleeConflict`, the "What is All
  is Lost?" help) and "Not decided yet". Stored old values are ignored (no migration needed; a cleanup migration is optional). The aftermath no
  longer reads `Fled` (`aftermath_service.go` influence and rewards): the winner alone decides Win / Draw influence and rewards.
- The **End checks** sheet and card (`handleEndChecks`, `handleEndReminder`, the `?card=1` card) are removed; the reminder of "Propose to end"
  is replaced by the mandatory MVP of the sheet.

Acceptance: a 1 v 1 played live ends with End match (winner, MVP, review) → the rival's Confirm (MVP, review) → ended, aftermath open; a
proposer without an MVP vote cannot submit; a practice match ends without MVP; an old match with VP or Fled still renders. Demo world: a match
pending confirmation with the proposer's votes in place, and one ended through the new sheets. e2e: the end flow in `40-match` /
`84-match-extras`. Docs: [matches.md](matches.md) ("Ending a match", result card), [live-match.md](live-match.md) ("End checks", "Ending a match while live").

## 5. Phase 4: live banner and Break Point without turns

Behaviour (`internal/core/live_break.go`, `live_fold.go`, `live_service.go`, `internal/web/live.go`, `live_actions.go`,
`templates/pages/campaign_my_company.gohtml`, `static/js/live.js`):

- Banner (D11): Match N and scenario, the live dot, **View ▾** (the companies of the match; each links to
  `/campaigns/{id}/companies/{company}` in match mode, the current one marked), To wound, Match log, End match. On another company's page:
  "← Back to mine". No End checks button, no turn row.
- Break Point (D10): `LiveBreak.Broken` is derived from the casualties at every fold step (`cfg.exceeded`), no `BrokenAtTurn`, no `WillBreak`,
  no `Offer`; the `turn` kind (`LiveTurn`, `startTurn`, `liveMeta.turn`, `LiveState.Turn()`, the stale-turn refusal and "Cancel the ending
  first" for turns) goes. Old logs that contain `turn` entries still fold (the entry is ignored). The hub event of a company that becomes
  Broken shows a banner notice to every viewer; the Break Point card keeps the counts.
- All is Lost offer removed (D14): the flee action of a whole company and its UI go; per-unit Retreated stays.

Acceptance: in the Veterans demo match, wounding enough units turns the company Broken immediately with the notice on both players' pages,
undoing one returns it; View ▾ moves between the two companies and back. Demo world: `world_live.go` / `world_veterans.go` / `world_bands.go`
stop pressing turns; a Broken company is in the Fog of War match. Docs: [live-match.md](live-match.md) ("Company Break Point in play",
"Company page in a live match").

## 6. Phase 5: lists, header and aftermath

Behaviour:

- Compact campaign header (D13) in the shared campaign layout partial.
- Matches page (`pages/campaign_matches.gohtml`, the handler `handleCampaign("campaign_matches")`): Your turn list, filters Mine / Open / Ended
  (`?filter=`), compact rows; the Overview shows the same Your turn list (D12). Your turn lists, for the viewer: matches where they must confirm
  an end, must vote an MVP, or own a company whose aftermath is pending or in progress.
- Aftermath (`pages/aftermath.gohtml`, `internal/web/aftermath.go`, `internal/core/aftermath_service.go`) per D15: the stepper shows only the
  steps that apply; the live-fed sheet is a review (earned gains as chips, "+ add" opens the rest); Against the Odds lists only the bonuses
  taken; the sheet autosaves on change (debounced POST of the sheet; the sticky bar keeps the preview and **Start aftermath** only); when every
  casualty is final, no decision is open and no reward needs a choice, the server finishes the aftermath without the Finish press; the done page
  starts with the summary of changes.

Acceptance: an aftermath with no casualty and no level-up goes from Start aftermath to the summary in one press; with casualties the player
rolls each one and the page finishes itself after the last decision. Demo world: ward 1 keeps a case for each skipped step; docs-check.
Docs: [aftermath.md](aftermath.md) ("Flow", "The aftermath page"), [frontend.md](frontend.md) for the campaign header.

## 7. How to work

- One phase at a time, on `feat/simplify-matches`; each phase ends with `make test`, `make test-integration`, `python3 tools/validate_definitions.py`
  when rules data is touched, the e2e flows it changes, a check at 360 / 768 / 1280 px light and dark, and the docs above updated.
- Run the app for checks with the launch configuration `server-8082-exe` (`.claude/launch.json`): build `go build -o tmp/server.exe ./cmd/server`
  first; on this machine Windows Application Control blocks the binaries that `go run` leaves in `tmp/gotmp`. Demo accounts and the password
  are in `tmp/demo-world.txt`; `make demo` reloads the world (destructive).
- `bd` (beads) is not installed on this machine, so this file is the tracker of the work: tick the phases in the status line.

## 8. Progress (2026-11-03, nothing committed yet: the owner tests by hand first)

All five phases were written in one pass on `feat/simplify-matches`; nothing is committed. `go build ./...` passes.

### Done

- **Core** (`internal/core`):
  - `match.go`: `SideResult`, `SideVP`, `maxVictoryPts`, `CompanyResult.Fled`, `FleeConflict`, `HasRoles`, `ObjectivesFor` removed; `ReportedWinner` uses `Label`.
  - `campaign.go`: `MatchSide.Label` always shows the companies; `NewSide` has no `Role`; `NewMatch` gains `Scenario`; `Match` gains `ATODecided`.
  - `matches.go`: `CreateMatch` validates and stores the scenario; `SetSideRoles` / `SwapSideRoles` removed; `SetScenario` / `ClearScenario` refused once the game started (`GameStarted` hook); `MatchStarted`; `validScenario`.
  - `match_end.go`: `checkEnd` requires the MVP votes (`MissingVotes` hook, type `MissingVote`): the proposer's in `ProposeEnd`, the confirmer's in `ConfirmEnd`, everybody's when the match ends; practice skips it; `OverrideEnd` needs none.
  - `ato_service.go`: `atoCardWith`, `ReopenAgainstTheOdds`, `ATOStatus`, `ATOBlocks`; `ATO.Inclusive`.
  - Live (`live.go`, `live_break.go`, `live_fold.go`, `live_service.go`): no turns (`LiveTurn`, `startTurn`, `liveMeta`, `Turn()`, `LiveCompany.Turn`, `CourageTurn`, `StandFastTurn` gone); `Broken` derived from the casualties; `WillBreak`, `BrokenAtTurn`, `Offer`, `Fled` gone; older `turn` entries are ignored, older `allIsLost` flee entries still take the units off the board, new ones are refused (`liveTurnLegacy`, `liveAllIsLostLegacy`); `MatchDefaults` only `EnemyBroken`; new `MatchBreaks` and `MissingVotes`.
  - Aftermath: `CompanyOutcome` ignores Fled; `advance` finishes the aftermath by itself when no reward choice is open (`finish`, `rewardChoiceOpen`); `FinishAftermath` is a no-op on a finished one.
- **Postgres**: `Create` stores the scenario and the `match_ato` row in the same transaction; VP, fled and roles are no longer read or written (columns kept, no migration).
- **Wiring** (`cmd/server/backend.go`): `matchSvc.MissingVotes = live.MissingVotes`, `matchSvc.GameStarted = atoRepo.GameStarted`. Note: the facade has both the `Matches.MissingVotes` field and the `LiveService.MissingVotes` method; call `c.liveSvc.MissingVotes` explicitly.
- **Web**:
  - Phase 1: `match_new.go` (new file; the old code left `campaign.go`), `match_new.gohtml`, `match_form.js` (no Mine chip, own companies first/last, + Add side, All vs all, scenario card with Roll again / Choose another / One-off via `GET .../matches/new/scenario` and `POST .../new/scenario-roll`, `unitPick`).
  - Phase 2: `match.go` and `match.gohtml` rewritten (phases `preparing` / `playing` / `ending` / `ended` in `data-phase`, scoreboard, Go to my company, roles gone, Log card with History + match log, result form only for organiser/admin on an ended match, Roll again sets the scenario directly with Vantage Point). `match_result.js` and `live_end.go` deleted; `/roles`, `/live/end`, `/live/end/reminder`, `/live/{company}/objective`, `/live/{company}/mvp` routes removed.
  - Phase 3: `end_sheet.go` (new): `GET .../end/sheet` (End match or Confirm sheet), `POST .../end/propose` and `/end/confirm` do votes + objective switches + result + propose/confirm in one request and answer `HX-Redirect` to the match page. Sheet window on the match page (`sheet_dialog`, `#modal-body`, `open-modal` / `close-modal`).
  - Phase 4: live banner (View ▾, ← Back to mine, To wound, Match log, End match), `end_banner` lists Broken companies and Confirm opens the sheet, Break panel without turns or All is Lost; Courage test offered to every Field unit of a Broken company (`CourageTest`), `CourageDue` = not tested yet.
  - Phase 5: compact campaign header; Your turn list (`yourTurns`, `your_turn`, `match_row` partials) on Matches and Overview; filters `?filter=mine|open|ended`; aftermath stepper shows only the applicable steps (`Steps`, `HasInjuries`, `HasLevelUps`), Done block with `ChangeSummary`, sheet review (`Review`, `Earned`, "+ add"), Against the Odds only taken bonuses (+ add), autosave (`/sheet?auto=1` answers `aftermath_bar`), no Finish button.
- **Demo world** (`cmd/server/world_*.go`, `seed.go`): no VP, Fled, roles or turns; `endNow` casts the missing MVP votes (`voteMissing`); `played` creates the match with its scenario (`newMatchOn`); matches campaign adds a match waiting for Against the Odds and one confirmed (`atoWaiting`), scripts decide before they play (`decideATO`), an end pending confirmation and one ended through the sheets (`sheetEnd`); the Fog of War Rangers become Broken.
- **Tests**: `make test` and `make test-integration` pass (core, web, postgres, cmd/server). New: `internal/web/match_new_test.go` (form, scenario card, Your turn), `internal/core/ato_gate_test.go` (the decision and the gate), `internal/core/simplify_matches_test.go` (`CreateMatch` with scenario, the scenario fixed once the game started, `LiveService.MissingVotes`), the free equipment choice finishing the aftermath, the vote tests in `match_end_test.go`.
- **Docs**: `matches.md`, `live-match.md`, `aftermath.md`, `frontend.md`, `demo-world.md`, `multi-company.md`, `docs/README.md`.
- **e2e**: `40-match`, `81-aftermath-ui`, `83-fallback`, `84-match-extras`, `98-several-companies`, `100-practice-match` updated; new `110-new-match-form`; shared helpers `endMatch`, `confirmEnd`, `chooseScenario` in `tests/e2e/lib/util.mjs`; the runner names the server binary `.exe` on Windows.

### Left to do

- The owner's manual test; then the commits (one or more per phase) and the pull request.
- The browser check at 360 / 768 / 1280 px, light and dark, of the pages the e2e flows do not screenshot.
