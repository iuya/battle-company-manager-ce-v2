# Live match mode

What happens to a company's pages while a match is running: trackers for Wounds, Might, Will and Fate, actions that also earn experience, the
Retreated and Field casualties zones, a match log with undo, end checks (objective and MVP), the Break Point, Against the Odds items, and live
updates for the other players watching. The match record (scenario, result, ending) is in [matches.md](matches.md); what the finished match does
to the companies is in [aftermath.md](aftermath.md); the technical design (fold, locking, event stream) is in
[architecture.md](architecture.md#live-match-design); the pages are described in [frontend.md](frontend.md#live-match).

Code: `internal/core/live.go`, `live_fold.go`, `live_service.go`, `live_hub.go`, `live_break.go`; `internal/web/live*.go`; tables `match_live`,
`match_live_unit`, `match_live_log` (and `match_ato` for Against the Odds).

## Principles

- **A running match is an open match** (`Match.Ended == false`). There is no separate "start" step: creating the match starts it, and ending it
  stops it. A match opened before its live rows exist becomes live the first time a page asks.
- **The log is the truth.** The state of every unit (wounds, might, will, fate, zone, mount) is a pure function of the match's initial values and
  its not-undone log entries. Undo marks an entry undone and the state is recomputed; nothing else mutates state. Experience flags come out of the same log.
- **Players are trusted, as everywhere.** The app records what the players say happened and does the arithmetic; it does not check the dice. Trackers
  are permissive and revertible; the only refusals are impossible states (a stat outside `0..start`, spending Fate at 0, acting with or targeting a
  unit that is not in the Field).
- **Not live, not changed.** A company that is in no open match has its normal page, no live connection and no trackers.
- The live view adds to the company page; cards keep every existing element.

## Experience: which actions earn it

`campaign.experience.gains` in the Battle Companies file decides, and the shape of each gain in `campaign.matchInput.perModel` is what matters:

| Gain key | Rulebook | Shape | So the live mode records |
|---|---|---|---|
| `woundedEnemy` | "Wounding at least one enemy, even if it was subsequently prevented" | flag (once per game) | the first wound declared by the unit; later wounds change nothing. Counts even when every wound was then prevented (Fate) |
| `slainHeroOrMonster` | "Each enemy Hero or Monster they slay" | count, repeatable | +1 per enemy **Hero or Monster** the unit slays, every time. Slaying a Warrior earns nothing |
| `fatePrevented` | "Preventing a Wound by spending a Fate point" | flag | the first Fate point the unit spends to prevent a wound |
| `brokenCourageTestPassed` | "Rolling for and then passing a Courage test for being Broken" | flag | the first Broken courage test it passes (only a rolled pass; automatic passes and exemptions earn nothing) |
| `participated`, `objective`, `mvp` | | flags | `participated` defaults on in the aftermath sheet; `objective` and `mvp` come from the end checks below |

A wound on an enemy **mount** counts as wounding an enemy (the rulebook note says a rider's xp is attributed by the unit, not by the model on the table). A unit
is a Hero when `CompanyUnit.Role` is a hero role and a Monster when `Kind` is monster; the kill is credited to the unit named in the action, never split.

## Who may do what

| Viewer | Own company page in a live match | Another company's page in the match |
|---|---|---|
| Owner of a company in the match | trackers and all actions on own units; may declare a wound on an enemy unit (the only write that touches another company) | read only, live updates |
| Administrator, organiser | same as the owner, on any company in the match | same |
| Other player of the campaign | read only, live updates | read only, live updates |
| Not in the campaign | 404 | 404 |

`core.LiveAccess(user, match, company, organiserID)` returns `LiveNone` (read only) or `LiveEdit`; the page uses it only to hide controls, the service
enforces it (`ErrRefused`, "Only the owner of this company can change it"). A finished campaign and an ended match are read only for everybody ("The match
has ended"). Undo follows the entry: the owner of its acting company, the owner of a company the entry touched, the organiser and administrators.

## Match start rules

1. **Creating a match is refused** (`*CompanyBusyError`, shown on the create form) when any company of any side is in another open match: "Company X is
   already in Match N." (with a link). Also while a company has a pending aftermath of an earlier match ("Finish the aftermath of Match N first").
2. **Companies are locked while the match is open**: every roster action (recruit, buy, drop, heal, creatures, wanderers, level-up rolls, promote, carry,
   gold changes, choosing a path) returns `ErrRefused` "Match N is running; finish it first", like the aftermath lock (one rule in `CompanyState.Locked`).
   Renaming units, the formation order, the cemetery order and organiser grants stay allowed.
3. **Who is in the match**: every unit of the company that is not `Injured` or at the healers at start; those are listed under "Missed this game" and take
   no part (no xp).
4. **Initial values** are taken when the match opens: Wounds, Might, Will, Fate from the unit's current profile (`CompanyUnit.Stats`, so promotions and path
   boosts count) and Mounted when the unit has a mount. The roster is locked, so the maxima are fixed for the match.

## Screens (mobile first, 360 px up)

### Company page in a live match

Applies to the owner's My Company tab and to every other player's view of the company. Top to bottom:

1. **Live banner**, sticky below the app bar: "Match N is running" with the scenario name when chosen, a link to the match, a live-connection dot (green "Live",
   grey "Reconnecting...") and the buttons **To wound** ([matches.md](matches.md#to-wound-chart); its window is a sibling of the banner and outside `#live-zones`,
   so a live refresh never closes it), **End checks** and, for who may edit, **Match log**. On 360 px the banner is two or three lines.
2. **Locked notice** instead of the buy, recruit and heal controls (hidden, not disabled): "The company is locked until the match ends." Counters stay.
3. **Battle company**: the units in the Field zone in the company's order, as unit cards plus trackers.
4. **Retreated** (a one-line header with a count, "None" when empty): compact cards (name, type, a "Retreated" badge with the reason "Failed courage" or "Left
   the board") with **Return to the battle** for editors.
5. **Field casualties**: units at 0 Wounds or marked Died. Compact card with a "Casualty" badge (reason "Wounded by <unit> (Company)", "Died: <note>", "Wounds
   lowered to 0"), "Rolls for injuries after the match" and **Return to the battle**.
6. **Missed this game** (only when there are such units): muted compact cards.
7. **Cemetery** as usual, read only.

Sections 4 and 5 sit between the Battle company and the Cemetery. States: empty live, viewer (no tracker controls or action menu), ended (the live banner is
replaced by the normal aftermath banner), connection lost (the page keeps working and refetches on reconnect).

### Unit card in the Field zone

Everything the card has outside a match, plus:

- **Trackers** replace the static W, M, W, F values in the attribute row: `current / start`, a minus and a plus (44 px targets); a tap on the number opens a
  "set to" field. Wounds, Might, Will and Fate stay within `0..start`; low values use the danger tone. Heroes show M W F, warriors only Wounds.
- **Wounds -1** from the tracker is a manual change (no xp, no target). At 0 Wounds the unit moves to Field casualties with the toast "Aldor fell. Undo"; the plus
  button is the way back (logged as "Wounds +1 (manual)").
- **Actions menu** (a bottom sheet on 360 px, a popover from 768 px):

| Action | Prompt | Effects | Log text |
|---|---|---|---|
| Wounded a unit | see below | target wounds, attacker xp flags | "Aldor wounded Grashnak (2 wounds, 1 saved by Fate)" |
| Passed a fate roll | how many (default 1, at most the current Fate) | Fate -n, `fatePrevented` flag; refused at 0 Fate | "Aldor spent Fate (1 left)" |
| Passed a Broken courage roll | confirm | `brokenCourageTestPassed` flag; always offered, also before the app says Broken | "Aldor passed a Broken courage test" |
| Retreated | "Failed a courage roll" / "Left the board", optional note | zone Retreated, outcome `fled` in the aftermath; no injury roll; cannot act or be targeted | "Aldor retreated (failed courage)" |
| Died | optional note, for deaths not caused by an enemy wound | zone Field casualties with reason "Died"; injury roll later | "Aldor died: ..." |
| Mount removed / re-added | | Mounted flips | "Aldor lost his mount" |
| Item used | see Against the Odds items | strikes a consumable through | "Brand used Dwarven Brew" |

Fate and mount actions are hidden when the unit has no use for them. Units outside the Field have only **Return to the battle**.

### Wounded a unit (the only cross-company action)

A sheet in steps on 360 px (one screen from 768):

1. **Who**: the enemy units in the Field, grouped by company, each row with name, type, current / start Wounds and a mount icon when mounted; searchable. Allied
   companies of the same match follow a "Friendly fire" divider (the app does not know alliances). The unit itself is not offered.
2. **Which part**: for a mounted target, "Rider" or "Mount".
3. **How many wounds caused** (1 to 9) and, for a Hero target with Fate, **how many were saved by Fate** (`0..min(caused, target Fate)`), with a preview line in plain words.
4. **Confirm**: one log entry for the declaration (`LiveWound`).

Semantics: `caused` wounds are dealt; `savedByFate` of them are prevented and cost the target that many Fate points (and set its `fatePrevented` flag on the first); the target's Wounds drop by `caused - savedByFate`. A
"Mount" part drops the mount's wounds the same way (a mount with more than one Wound has its own counter; at 0 the mount is removed, as in Mount removed) and never makes the rider a casualty. The attacker gets
`woundedEnemy` when `caused >= 1`, even if all were saved. If the target reaches 0 Wounds it is a casualty and, when it is a Hero or Monster, the attacker's `slainHeroOrMonster` count rises by one. The declaration is applied at once;
the target's owner sees the card change live and a toast "Aldor wounded Grashnak (2). Undo"; either owner may undo. Undoing the declaration undoes all of it.

### The opponent's keywords and which of your rules apply

Units of the other side show their keyword chips everywhere a unit is drawn in the match. A unit in the Field that has a keyword named by an enemy-keyword rule (Hatred (X), Ancient Enemies (X), named rules such as End the Race of
Men) of one of your side's units that is also in the Field carries a marker "Hatred (Orc) applies" naming the rule, and its tooltip says which of your units has it. Rules that work against every enemy (Terror) and friendly rules (Fury)
give no marker, and the enemy's rules against you are not shown. A unit that retreats, falls or is missed takes part in neither list, so the marker goes with it; a unit that lost its mount counts without Cavalry (`UnitKeywords.OnFoot`).
`core.RuleMatches` is pure and the web layer calls it with the Field units of both sides; the markers come back with every live refresh. Data: [unit-keywords.md](unit-keywords.md).

### Mount removed / re-added

**Mount removed** ("fell, now infantry") sets `Mounted = false`. The card strikes through the gear that needs a mount (`requiresMount`, lances; Barding is the steed's), the rules the mount gave (`grantedTo: mount`, the Pony's Timid) and the
Cavalry keyword; the items stay listed, the strikethrough has `aria-label` "needs a mount", and a small "On foot" badge shows near the name. Stats and company limits do not change. **Mount re-added** reverses it. It is a plain state change (no
xp, no injury; a mount that is a company creature, such as the Baggage Pony, is not affected). A wound declaration with part Mount that takes the mount to 0 does it automatically, in the same entry.

### Match log

A bottom sheet on 360 px (a side panel from 1280), reached from the live banner and from the match page (a "Live log" card below History, read only for viewers). Newest first, grouped by minute; each row: time, text, the author's company
colour dot and **Undo** for who may. Undone entries disappear from the log and stay in the database for audit (`undone_by`, `undone_at`). Empty state: "Nothing has happened yet." Undoing an entry recomputes the state; later entries stay as
long as they remain valid. If undoing would make a later entry impossible, the undo is refused with "A later change depends on it; undo that first". Manual tracker changes appear in the log like actions. After the match the log stays on the
match page, read only and permanent, also after the campaign is finished.

## Zones

A unit's zone is derived from state, in this priority: **Retreated** if a not-undone `retreat` entry exists; **Field casualties** if a not-undone `died` entry exists or Wounds are 0; **Field** otherwise. A unit outside the Field cannot act
or be targeted. **Return to the battle** is its own action kind (`return`): it clears Retreated and Died and lifts Wounds 0 to 1; it is always logged, and undoing it puts the unit back.

## Aftermath feed

At End match, and for the aftermath sheet built at its first read, the default `AftermathInput.Models` comes from the live state (`LiveService.AftermathDefaults(ctx, id, matchID, companyID) ([]ModelSheet, bool, error)`; the bool is false without live data):

- `Outcome`: Field gives `survived`; Retreated gives `fled` (no injury roll); Field casualties give `casualty`.
- `Gains`: `woundedEnemy`, `fatePrevented`, `brokenCourageTestPassed` from the flags, `slainHeroOrMonster` from the count, `objective` and `mvp` from the end checks; `participated` stays on.
- The sheet shows the badge "From the live match" while it is the unsaved default; the player can change anything and saving the sheet never reads the log again. A match with no live data behaves as if there were no live mode.
- After the match ends the trackers disappear. Live state survives server restarts: all state is in PostgreSQL and the hub only notifies.

## Live sync for viewers

A viewer of a company in a running match sees changes within about a second without reloading and cannot modify that company's trackers. One Server-Sent Events stream per match, `GET /campaigns/{id}/matches/{match}/events`, carries
pointers, not HTML: `event: company-<id>` and `event: match` (a change or an undo), `event: ended` (the page reloads), `id: <seq>`, `data: {"seq": 41}`, a `: ping` every 25 s. The page refetches its own authorised fragment, so rendering stays per
viewer. On reconnect the server sends one batch for every company and the fetch is idempotent. A write answers its own page directly; the author's tab ignores the event whose seq it already has. If `EventSource` fails or is refused (caps of
10 streams per user and 50 per match) the live banner polls the company fragment every 10 s and shows the dot grey. The script is only included on pages of a company in an open match. The design is in [architecture.md](architecture.md#live-sync-server-sent-events).

## Go contracts

The data types, errors and repository interfaces are in `internal/core/live.go`. `internal/web` declares what the pages need and `core.LiveService` implements it:

```go
type liveMatches interface {
    Live(ctx, user, id, companyID int64) (core.LiveCompany, error)                                  // ErrNotFound when the company is in no open match
    Apply(ctx, user, id, matchID, companyID int64, a core.LiveAction) (core.LiveCompany, core.LiveEntry, error) // ErrRefused / ErrInvalidLive / ErrNotFound
    Undo(ctx, user, id, matchID, entryID, companyID int64) (core.LiveCompany, error)                // ErrLiveDepends when a later entry depends on it
    Log(ctx, user, id, matchID, before int64, limit int) ([]core.LiveEntry, error)                  // not-undone entries newest first, CanUndo set for user
    Subscribe(ctx, user, id, matchID int64) (core.LiveStream, error)                                // Next blocks for a coalesced core.LiveBatch; ErrRefused at the caps
    // EndChecks, RollCourage, MatchDefaults, SecretObjectives ... see below
}
```

Concurrency: every write is a single transaction that appends the entry and bumps `Seq`; two players hitting the same unit at once both succeed (deltas), which is the normal case at a table. No per-write revision is required; the page re-renders the
card from the returned state.

Routes (htmx, CSRF as elsewhere): `POST /campaigns/{id}/matches/{match}/live/{company}/actions` (form fields by kind; answers the zones and a toast), `POST .../live/log/{entry}/undo`, `GET .../live/log` (the sheet; `?card=1` the match page
card; `?before=<seq>` older rows), `GET /campaigns/{id}/companies/{company}/live` (the whole live section; what the stream triggers), `GET .../events` (SSE), `GET .../live/{company}/units/{unit}/sheet/{kind}` (an action sheet).

## End checks: objective and MVP

Two things the players settle when the game ends, both worth experience (`objective` and `mvp`, 1 xp each) and both recorded in the live log, so they undo, show in the log and feed the aftermath sheet. They are available at any time while the
match is open, from the live banner button **End checks** and as a card on the match page; **Propose to end** first reminds the editor of what is still open ("No unit has the objective yet · you have not voted for Hador") with *Go back* and
*Propose anyway*; it never blocks.

### Got objective (per unit)

- A flag per unit, so several units of a company can have it; each earns 1 xp. Set by the owner of the company, an administrator or the organiser (`LiveAccessFor == LiveEdit` on that company); a viewer sees the units that have it, read only.
- Listed: the units of the company being edited that are in the Field or Retreated zone (a unit that left by the board edge may count; casualties and "Missed this game" cannot). The owner sees their own company; an administrator or organiser sees one list per company.
- Control: a checklist (a switch per unit, 44 px rows) that changes at once with a toast "Hador: got the objective. Undo". Setting it again is refused (the page treats it as a no-op); switching off is its own entry. Log text: "Aldor got the objective" / "Aldor no longer has the objective".

### Select MVP

- "Being declared MVP by the opposing player after the game." With several sides, "opposing" is every company of a different side.
- **Who votes**: a user who owns at least one company in the match, once per opposing company: one unit of that company that took part (Field, Retreated or Field casualty), or "No MVP" (clears the vote). Administrators and the organiser vote only through a company they own; they can undo any vote.
- Voting again replaces the earlier vote (the fold keeps the last not-undone vote per `(voter, target company)`); undoing the new vote restores the earlier one. After End match the votes are frozen.
- **Tally** (per target company, computed by the fold, never stored): the unit with the most votes is the company's MVP. A tie gives no automatic MVP: the aftermath sheet shows "Tied: Aldor, Brand" and the owner picks one. No votes: no MVP. Votes are shown in the log and on the card, like every other live fact.
- Log text: "Frodo voted Aldor as MVP of Hador" / "Frodo withdrew the MVP vote for Hador". `CanUndoEntry` for a vote: the voter, administrators and the organiser. The aftermath sheet's `mvp` is prefilled with the winning unit and the badge "Voted MVP (2 votes)".

New `LiveAction` kinds: `objective` (`UnitID`, `On`) and `mvpVote` (`TargetCompanyID`, `UnitID`, 0 withdraws). The fold gains `HasObjective`, `Vote` and `MVPTally` (`LiveState.CheckAs` with `LiveVoter`). Both are refused once the match has ended.

```go
EndChecks(ctx, user, id, matchID int64) (core.EndChecks, error) // ErrNotFound when the match is not open or the viewer is not in the campaign
type EndChecks struct {
    Objective []CompanyObjectives // companies the viewer may edit
    MVP       []MVPBallot         // one per company of an opposing side of any company the viewer owns
    Tally     []MVPTally          // every company of the match: unit -> votes, Winner, Tied
    Pending   []string            // what the reminder lists
}
```

Errors: `ErrRefused` ("Only the owner of this company can change it"; "You can only vote for companies of the other teams"; "You have no company in this match"), `ErrInvalidLive` (unit not in the company or not eligible). Routes: `GET .../live/end` (sheet or `?card=1`),
`POST .../live/{company}/objective` (`unit`, `on`), `POST .../live/{company}/mvp` (`unit`, 0 withdraws).

## Company Break Point in play

The Rules Manual's Break Point is half the models of the starting force; a company whose casualties **exceed** it at the **start of a turn** is Broken for the rest of the game, tests Courage at every activation, and may flee (All is Lost). Muster
of Isengard uses 66%. Data: `campaign.breakPoint` (`default.percent`, `threshold`, `startingModels`, `casualties`, `whenBroken`, `allIsLost`, `experience`, `scenarioExceptions`) and `companies[].effects[kind=breakPoint].percent` ([definitions.md](definitions.md)).
The app is a **companion, not a simulator**: it counts from the log, flips the state when the players say a new turn starts, and prompts the tests; it never decides an activation.

### What is counted

- **Starting models** of a company = its models in the match + its creatures + the temporary Wanderer + models yet to arrive (Maelstrom of Battle); never neutral models. Each roster unit counts `BreakModels` (default 1; a Cavalry model, a Chariot and a War Beast with its Commander are the
  units' own counts; the Mirkwood Warg Marauder card counts as 3, one per rider). Units absent at the start are not in the match.
- **Casualties** = units in Field casualties + units Retreated with the reason "Failed courage" (weighted by their model count) + the manual adjustment. A unit that **left the board** and a mount removed alone are not casualties. Undo and Return to the battle subtract again.
- **Break Point** = `percent x StartingModels / 100`, shown with one decimal ("6.5"). **Broken** iff `casualties * 100 > percent * StartingModels`, tested **only** inside a `turn` entry.
- The manual **adjustment** is a logged action `breakAdjust` (`Field` "start" or "casualties", `Delta` -9..+9, never below one starting model or below no casualties, `Note` 0-200) for creatures killed, Ponies not counted, models yet to arrive and composite cards.

### Turn boundary and Broken

`turn` is a match-level action: **Start turn N** on the live banner (any editor of a company in the match). It carries the number it starts; a stale press is refused ("Turn 3 has already started") and a number that is not the next is `ErrInvalidLive`. The fold evaluates the test for each company at that
entry; a company that fails and was not Broken becomes Broken with `BrokenAtTurn = N`, permanently. Undoing the `turn` entry un-breaks the companies it broke. The app never infers a turn: a table that never presses the button never gets Broken, and the card then shows only the counts. Before
the button is pressed the panel warns "Hador will be Broken at the start of the next turn (7 casualties, Break Point 6.5)".

### Screens

1. **Break Point panel** on the company page (collapsed to one line, also on the match page's live card): "Break Point 6.5 of 13 · Casualties 4 · 50%" with a thin meter (text first), the percent labelled as the company rule. Opened, it shows the starting models with the **+/- adjustment** (editors, with a note),
   the casualties by name, the scenario exceptions (Defend the Hill: models on the hill do not test; Let nothing through!: the Defender within 6" of the Gate passes automatically; Only a Pony After All: Baggage Ponies do not count for the Defender; Neutral Third-Party: no tests; Maelstrom of Battle: models yet
   to arrive count as on the board) and the break point log lines. The company header shows "Break Point: N% of the starting models" (`CompanyDetail.BreakPointPercent`, the company's own rule or the file's default).
2. **Turn row** in the live banner: "Start turn 4" (44 px), or a "Turn 3" label.
3. **Company broken banner** (below the live banner, for everybody): "Hador is Broken (turn 4). Every model tests Courage before it does anything else. Models in combat, or that cannot activate, do not test." with the All is Lost choice while the offer is open and "Who has tested this turn" (the table's checklist; no rule).
4. **Unit cards of a Broken company** in the Field show "Test due" (danger tone, text) until the unit's courage is resolved this turn, then "Tested: passed", "Tested: failed", "Auto-pass" or "Exempt". The company header has a **Courage test** button that opens a sheet:
   - **Roll** 2D6 against the unit's Courage (`RollCourage`, which writes nothing; a typed roll is "I rolled" with two dice fields as `RollRefusal` permits): "7 against Courage 4: passed", then **Record: passed** or **Record: failed**. Players may also just tap the outcome.
   - **Passed** (`broken` action) earns `brokenCourageTestPassed` once. A Hero offers **Call Stand Fast** (not for a Hero with Broken Honour or a Wanderer); Warriors then get the hint "A Hero called Stand Fast this turn: Warriors within 6" in line of sight pass automatically".
   - **Failed**: the `retreat` action with reason "Failed courage": the unit flees, counts as a casualty, outcome `fled`, no injury roll, no experience.
   - **Passes without a roll** (`courageSkip`, reason `standFast`, `heroicResolve`, `chariot`, `scenario`, `engaged`, `cannotActivate`; the last two are exemptions): no experience. Heroic Resolve does not touch the Might tracker.
5. **All is Lost offer**: from the turn the company became Broken until the next `turn` entry: **Flee the battlefield** / **Fight on**. Text: "Your opponent claims victory and full rewards. You earn only the Influence for taking part (the injury rolls and experience still happen)". Flee retreats every Field unit as "Left the
   board" (not casualties) and marks the company `Fled`; the match page's result form prefills "Fled" with the hint "The other side claims victory (All is Lost)" and does not end the match. Only editors of the company; undoing the flee restores everything. `allIsLost` is refused when no offer is open.

### Log and fold

```go
LiveTurn        LiveKind = "turn"        // Turn; match level, company 0
LiveBreakAdjust LiveKind = "breakAdjust" // Field "start"|"casualties", Delta, Note
LiveCourageSkip LiveKind = "courageSkip" // UnitID, Reason
LiveAllIsLost   LiveKind = "allIsLost"   // Flee bool
```

`LiveAction` carries `Turn`, `Field`, `StandFast` (on a `broken` action: this Hero called Stand Fast), `Reason`, `Flee`, `Item` and `Index`. `FoldLiveWith(inits, entries, LiveFoldConfig)` and `ValidateLogWith` take what the log does not hold: per company `BreakConfig{Percent, Extra, Exceptions}` (the company's own
`breakPointPercentage` or `campaign.breakPoint.default.percent`; `Extra` is the company's creatures; the exceptions are the match scenario's `scenarioExceptions` as sentences), `BreakModels` per unit and `NoStandFast` (non-Heroes, Wanderers, a Hero whose injuries took Stand Fast away). The service builds the config at every read
(`liveContext.breakConfig`); nothing is stored. `FoldLive` is the same with no config, and then nothing ever breaks. `LiveState.Break(company)` returns `LiveBreak{Percent, StartingModels, Casualties, Broken, BrokenAtTurn, WillBreak, Offer, Fled, Exceptions}` (`BreakPoint()` as "6.5") and `LiveState.Turn()`;
`LiveCompany` carries `Break` and `Turn`; `LiveUnit` carries `BreakModels`, `CourageTurn`, `CourageHow`, `StandFastTurn` and `Temporary`. Migration `00021_live_break_point_kinds.sql` allows the kinds. `broken` and `courageSkip` are allowed even when the company is not Broken, so tables without turn tracking keep working; both are refused for a unit outside the Field.

```go
RollCourage(ctx, user, id, matchID, unitID int64, entered []int, thrown bool) (core.CourageRoll, error) // writes nothing; honours RollRefusal; the unit must be in the Field and have a Courage
type CourageRoll struct{ Dice []int; Total, Courage int; Passed bool }
MatchDefaults(ctx, id, matchID int64) (core.LiveMatchDefaults, bool, error) // Fled and EnemyBroken per company; false without live data
```

`RollCourage` requires `campaign.breakPoint.whenBroken.courageTest.dice` to be `2d6` and `passIf` `totalAtLeastCourage`. `EnemyBroken` (every company of every other side is Broken at the end) prefills the `enemyBroken` scenario objective on the result form, with the badge "From the live match"; the editor can change it. Routes: the `actions` route
carries `turn`, `breakAdjust`, `courageSkip`, `allIsLost` and the rest; `POST .../live/{company}/units/{unit}/courage/roll` rolls.

## Against the Odds items in a live match

The match's Against the Odds record ([matches.md](matches.md#against-the-odds)) is **overlaid at read time** on the live state (`LiveService.WithATO`, `liveContext.overlay`), so a selection saved after the live state was first read shows at once and one cleared before the game starts disappears.

- **Loaned items** (Tool Kit): `LiveUnit.Loaned []LoanedItem{UnitID, Item, Kit, Detail, Index, AfterGame, Used}`, listed under the Wargear of the card grouped by item ("Climbing ropes ×5", tag "Loaned (Raider's Kit)") with no Rating. Consumables get **Item used** in the action menu (`LiveItemUsed`: `UnitID`, `Item`, `Index`),
  which strikes the item through and logs "Brand used Dwarven Brew"; undo restores it. Items that act in the aftermath (Healing Herbs, Wondrous Cram) read "use after the game" and have no Item used button. Owned items marked used are in `LiveUnit.UsedItems` (any owned item may be marked used, except the aftermath ones). The kit never changes stats.
  The fold checks that the unit is in the Field and carries the item, that it does not act in the aftermath ("Healing Herbs acts after the game: use it in the aftermath.") and was not used ("Brand has already used its Dwarven Brew").
- **Temporary Wanderer**: `LiveCompany.Wanderer *TemporaryWanderer{UnitID, Name, Summary, Rating (45), Courage}` for the company that took it. It is a normal unit of the fold with the synthetic id `core.TemporaryUnitID(company)` (the negative company id), badges "Wanderer" and "Temporary", trackers from `data.wanderers.items`, no Stand Fast, no Dismiss. It is
  counted in the Break Point starting models, can wound, be wounded and roll Courage, cannot be voted MVP and is skipped by `AftermathDefaults` (no roll, no experience, no progression). It appears in the zone its live unit is in and as a wound target.
- **Ambush!** has no live effect beyond the scenario already set.

## Ending a match while live

The match ends only when every participant confirms ([matches.md](matches.md#ending-a-match)). While live:

- **Banner.** When a proposal is pending, every company page of the match (owners and viewers, live via the `match` event) shows `#live-end` in the sticky live banner: "Ending the match: Hador confirmed, waiting for Gorgoroth." The waiting owner gets **Confirm the end** and **Keep playing**, a confirmed owner **Withdraw**,
  the organiser and administrators the override, viewers no buttons.
- **What is allowed while pending.** The end checks stay open: `objective`, `mvpVote` and the secret-objective entries are accepted and do **not** cancel the proposal. **Any other action or undo** cancels the proposal first (confirmations dropped, a log line "Ending cancelled: Aldor wounded Grashnak"), so the game is never played under
  a confirmed end. The cancellation happens after the game entry is stored, in a second transaction. **Start turn** is refused while pending ("Cancel the ending first: the match is about to end.").
- **Log.** Informational entries `endProposed`, `endConfirmed`, `endWithdrawn`, `endCancelled`, `endOverride` appear in the log with the company colour and the author (`LiveKind.Informational()`): written by `LiveService.RecordEnd` from `Matches.EndNotify`, skipped by the fold, not undoable and refused when a player posts them. `LiveService.WithEnds` makes
  `Apply` and `Undo` cancel a pending end after any entry that is not an end check (`LiveKind.EndCheckKind()`). Migration `00023` allows the kinds. Every proposal change notifies the live channel with company 0, so open pages refetch; the last confirmation emits `ended`.
- Whether a company is Broken, `Fled` and `EnemyBroken` are read from the fold at the moment the match ends; a Fled company still confirms the end like any other. The prefilled result values are a result change like any other: editing them resets the confirmations except the proposer's.
- The aftermath starts when the match ends and `AftermathDefaults` reads the final fold.
