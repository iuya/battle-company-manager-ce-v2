# Live match mode

What a company's pages do while its match is running.
It is for players, organisers and developers who want to know how the live tools behave and what they record.

While a match is open, a company's page gains:

- trackers for Wounds, Might, Will and Fate;
- actions that also earn experience;
- the Retreated and Field casualties zones;
- a match log with undo;
- the objective and MVP ("most valuable player") entries that the End match sheet reviews;
- the Break Point;
- Against the Odds items;
- live updates for the other players watching.

Related pages:

- The match record (scenario, result, ending) is in [matches.md](matches.md).
- What the finished match does to the companies is in [aftermath.md](aftermath.md).
- The technical design (fold, locking, event stream) is in [architecture.md](architecture.md#live-match-design).
- The pages themselves are described in [frontend-live-match.md](frontend-live-match.md).

Code: `internal/core/live.go`, `live_fold.go`, `live_service.go`, `live_hub.go`, `live_break.go`; `internal/web/live*.go`.
Tables: `match_live`, `match_live_unit`, `match_live_log` (and `match_ato` for Against the Odds).

## Contents

- [Principles](#principles)
- [Experience: which actions earn it](#experience-which-actions-earn-it)
- [Who may do what](#who-may-do-what)
- [Match start rules](#match-start-rules)
- [Screens](#screens-mobile-first-360-px-up): the [company page](#company-page-in-a-live-match), the [unit card](#unit-card-in-the-field-zone), [Wounded a unit](#wounded-a-unit-the-only-cross-company-action), [Mount removed / re-added](#mount-removed--re-added) and the [match log](#match-log)
- [Zones](#zones)
- [Practice matches in live mode](#practice-matches-in-live-mode)
- [Aftermath feed](#aftermath-feed)
- [Live sync for viewers](#live-sync-for-viewers)
- [Go contracts](#go-contracts)
- [Objective and MVP](#objective-and-mvp)
- [Company Break Point in play](#company-break-point-in-play)
- [Against the Odds items in a live match](#against-the-odds-items-in-a-live-match)
- [Ending a match while live](#ending-a-match-while-live)

## Principles

- **A running match is an open match** (`Match.Ended == false`). There is no separate "start" step: creating the match starts it, and ending it
  stops it. A match opened before its live rows exist becomes live the first time a page asks.
- **The log is the truth.** The state of every unit (wounds, might, will, fate, zone, mount) is a pure function of the match's initial values and
  its not-undone log entries. Undo marks an entry undone and the state is recomputed; nothing else mutates state. Experience flags come out of the same log.
- **Players are trusted, as everywhere.** The app records what the players say happened and does the arithmetic; it does not check the dice. Trackers
  are permissive and revertible; the only refusals are impossible states (a stat outside `0..start`, spending Fate at 0, acting with or targeting a
  unit that is not in the Field).
- **Not live, not changed.** A company that is in no open match has its normal page, no live connection and no trackers.
- **The live view adds to the page.** Cards keep every existing element.

## Experience: which actions earn it

`campaign.experience.gains` in the Battle Companies file decides, and the shape of each gain in `campaign.matchInput.perModel` is what matters:

| Gain key | Rulebook | Shape | So the live mode records |
|---|---|---|---|
| `woundedEnemy` | "Wounding at least one enemy, even if it was subsequently prevented" | flag (once per game) | the first wound declared by the unit; later wounds change nothing. Counts even when every wound was then prevented (Fate) |
| `slainHeroOrMonster` | "Each enemy Hero or Monster they slay" | count, repeatable | +1 per enemy **Hero or Monster** the unit slays, every time. Slaying a Warrior earns nothing |
| `fatePrevented` | "Preventing a Wound by spending a Fate point" | flag | the first Fate point the unit spends to prevent a wound |
| `brokenCourageTestPassed` | "Rolling for and then passing a Courage test for being Broken" | flag | the first Broken courage test it passes (only a rolled pass; automatic passes and exemptions earn nothing) |
| `participated`, `objective`, `mvp` | | flags | `participated` defaults on in the aftermath sheet; `objective` and `mvp` come from the live log (see "Objective and MVP") |

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
   Renaming units, the formation order (saving it or restoring the default), the cemetery order and organiser grants stay allowed.
3. **Who is in the match**: every unit of the company that is not `Injured` or at the healers at start; those are listed under "Missed this game" and take
   no part (no xp).
4. **Initial values** are taken when the match opens: Wounds, Might, Will, Fate from the unit's current profile (`CompanyUnit.Stats`, so promotions and path
   boosts count) and Mounted when the unit has a mount. The roster is locked, so the maxima are fixed for the match.

## Screens (mobile first, 360 px up)

### Company page in a live match

Applies to the owner's My Company tab and to every other player's view of the company. Top to bottom:

1. **Live banner**, sticky below the app bar: "Match N" with the scenario name, a link to the match, a live-connection dot (green "Live",
   grey "Reconnecting..."), **Go to match** (the match log is on that page), **Wound chart** ([matches.md](matches.md#wound-chart); its window is a sibling of the
   banner and outside `#live-zones`, so a live refresh never closes it) and, for who may edit, **End match** (the End match sheet,
   [matches.md](matches.md#the-sheets)). Below, one chip per company of the match with its owner: each opens that company's page in match mode,
   the current one marked with a tick. Under it `#live-end` names every Broken company ("Minas Morgul is broken",
   "... is broken and down to 25%" once it has also reached the end of the scenario) and the pending end with **Confirm**. On 360 px the buttons share one row and the chips scroll sideways, without the owners, so the sticky banner stays short.
2. **Locked notice** instead of the buy, recruit and heal controls (hidden, not disabled): "The company is locked until the match ends." The company stats (points, units, matches, wins), Influence, Gold and the limits are not shown during the match; Re-rolls stay when the company has any, because they are spent during the game.
3. **Battle company**: the units in the Field zone in the company's order (`core.OrderUnits`: the saved formation, or role, points and name), as unit cards plus trackers. The other zones use the same order, and the units that do not take part come last.
4. **Retreated** (a one-line header with a count, "None" when empty): compact cards (name, type, a "Retreated" badge with the reason "Failed courage" or "Left
   the board") with **Return to the battle** for editors.
5. **Field casualties**: units at 0 Wounds or marked Died. Compact card with a "Casualty" badge (reason "Wounded by <unit> (Company)", "Died: <note>", "Wounds
   lowered to 0"), "Rolls for injuries after the match" and **Return to the battle**.
6. **Missed this game** (only when there are such units): muted compact cards.
7. **Cemetery** as usual, read only.

Sections 4 and 5 sit between the Battle company and the Cemetery. States: empty live, viewer (no tracker controls or action menu), ended (the live banner is
replaced by the normal aftermath banner), connection lost (the page keeps working and refetches on reconnect).

### Unit card in the Field zone

Everything the card has outside a match (except the Equipment line of a unit with no equipment: nothing can be bought in a match), plus:

- **Trackers** replace the static W, M, W, F values in the attribute row: one segmented bar per stat, a segment per point, tapped to set the value
  ([frontend-live-match.md](frontend-live-match.md#trackers)). Wounds, Might, Will and Fate stay within `0..start`. Heroes show M W F and Wounds, warriors only Wounds; Wounds
  is always shown, also when it starts at 1, so every model can fall from its card.
- **Wounds -1** from the tracker is a manual change (no xp, no target). At 0 Wounds the unit moves to Field casualties, as Died does, with the
  toast "Aldor fell. Undo"; Return to the battle (or the toast's Undo) is the way back.
- **Milestones** at the foot of the card, with game-icons.net icons: **Wounded** (`machete`, with a `death-skull` count
  of the Heroes and Monsters slain), **Fate** (`shield-reflect`, heroes with Fate), **Courage** (`mighty-force`, a passed Broken courage
  test) and **Objective** (`flying-flag`). Off is a dashed, muted outline; on (the unit's `Gains` flag, or `LiveUnit.Objective`) is filled
  with the accent and carries a tick, so a lit one reads across the table. **Wounded** opens the Wounded a unit window (it can happen
  again; the window starts with "Recorded this match", the unit's wound entries with **Undo**). **Fate**, **Courage** and **Objective**
  switch on and off with one press and no window: Fate records Passed a fate roll with no point spent (`count=0`: only the mark, so it can be pressed at 0 Fate; the Fate tracker is set by hand), Courage Passed a
  Broken courage roll, Objective an `objective` action; pressed when lit, Fate and Courage undo the unit's last entry of that kind
  (`LiveUnit.FateEntry`, `BrokenEntry`, kept by the fold) and stay lit while an earlier one still earns the experience. A Fate point saved
  in a wound declaration has no entry of its own, so that Fate stays lit and disabled (undo the wound in the log). Viewers see the same row
  as marks they cannot press.
- **Other actions**, a second row for editors, icon above the label like the milestones, shown only while the unit has a loaned item to
  spend: **Item used** opens its window. The card has no button for Retreated, Died or the mount.
- The actions and their windows:

| Action | Prompt | Effects | Log text |
|---|---|---|---|
| Wounded a unit | see below | target wounds, attacker xp flags | "Aldor wounded Grashnak (2 wounds, 1 saved by Fate)" |
| Passed a fate roll | none from the milestone (`count=0`: no point spent); its window asks how many (at most the current Fate) | Fate -n (none for 0), `fatePrevented` flag; a count above the current Fate is refused | "Aldor passed a Fate roll" / "Aldor spent Fate (1 left)" |
| Passed a Broken courage roll | none from the milestone; the Courage test window records it too | `brokenCourageTestPassed` flag; always offered, also before the app says Broken | "Aldor passed a Broken courage test" |
| Retreated | "Failed a courage roll" / "Left the board", optional note | zone Retreated, outcome `fled` in the aftermath; no injury roll; cannot act or be targeted | "Aldor retreated (failed courage)" |
| Died | optional note, for deaths not caused by an enemy wound | zone Field casualties with reason "Died"; injury roll later | "Aldor died: ..." |
| Mount removed / re-added | none: one press | Mounted flips | "Aldor lost his mount" |
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

On the match page, in the **Log** card below History (read only for viewers); the live banner has **Go to match**. Newest first, grouped by minute; each row: time, text, the author's company
colour dot and **Undo** for who may. Undone entries disappear from the log and stay in the database for audit (`undone_by`, `undone_at`). Empty state: "Nothing has happened yet." Undoing an entry recomputes the state; later entries stay as
long as they remain valid. If undoing would make a later entry impossible, the undo is refused with "A later change depends on it; undo that first". Manual tracker changes appear in the log like actions. After the match the log stays on the
match page, read only and permanent, also after the campaign is finished.

## Zones

A unit's zone is derived from state, in this priority: **Retreated** if a not-undone `retreat` entry exists; **Field casualties** if a not-undone `died` entry exists or Wounds are 0; **Field** otherwise. A unit outside the Field cannot act
or be targeted. **Return to the battle** is its own action kind (`return`): it clears Retreated and Died and lifts Wounds 0 to 1; it is always logged, and undoing it puts the unit back.

## Practice matches in live mode

A [practice match](matches.md#practice-matches) plays exactly like a real one: its companies are locked while it is open, their pages show the same "match active" state with the trackers and counters, and the log works. The live bar adds the Practice badge and the line "Practice match: no experience, influence, rewards or injuries. No aftermath will follow." (`liveView.Practice`). The objective entries stay, no MVP vote is asked, and
say they are recorded only. When the match ends there is no aftermath feed: the sheet is never built.

## Aftermath feed

At End match, and for the aftermath sheet built at its first read, the default `AftermathInput.Models` comes from the live state (`LiveService.AftermathDefaults(ctx, id, matchID, companyID) ([]ModelSheet, bool, error)`; the bool is false without live data):

- `Outcome`: Field gives `survived`; Retreated gives `fled` (no injury roll); Field casualties give `casualty`.
- `Gains`: `woundedEnemy`, `fatePrevented`, `brokenCourageTestPassed` from the flags, `slainHeroOrMonster` from the count, `objective` and `mvp` from the live log (the votes' tally); `participated` stays on.
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
    // EndChecks, RollCourage, MatchDefaults ... see below
}
```

Concurrency: every write is a single transaction that appends the entry and bumps `Seq`; two players hitting the same unit at once both succeed (deltas), which is the normal case at a table. No per-write revision is required; the page re-renders the
card from the returned state.

Routes (htmx, CSRF as elsewhere): `POST /campaigns/{id}/matches/{match}/live/{company}/actions` (form fields by kind; answers the zones and a toast), `POST .../live/log/{entry}/undo`, `GET .../live/log` (the sheet; `?card=1` the Log card of the match page; `?before=<seq>` older rows), `GET /campaigns/{id}/companies/{company}/live` (the whole live section; what the stream triggers), `GET .../events` (SSE), `GET .../live/{company}/units/{unit}/sheet/{kind}` (an action sheet).

## Objective and MVP

Two things the players settle when the game ends, both worth experience (`objective` and `mvp`, 1 xp each) and both recorded in the live log, so they undo, show in the log and feed the aftermath sheet. The objective is set from the unit
cards' Objective milestone during the game; both are reviewed in the End match and Confirm sheets ([matches.md](matches.md#the-sheets)): the MVP vote is **mandatory** to end a competitive match, and the sheet shows the units with the objective as chips to switch.

### Objective (per unit)

- A flag per unit, so several units of a company can have it; each earns 1 xp. Set by the owner of the company, an administrator or the organiser (`LiveAccessFor == LiveEdit` on that company); a viewer sees the units that have it, read only.
- Listed: the units of the company being edited that are in the Field or Retreated zone (a unit that left by the board edge may count; casualties and "Missed this game" cannot). The owner sees their own company; an administrator or organiser sees one list per company.
- Control: the Objective milestone of the unit card, and the chips of the sheet. Setting it again is refused (the page treats it as a no-op); switching off is its own entry. Log text: "Aldor got the objective" / "Aldor no longer has the objective".

### MVP

- "Being declared MVP by the opposing player after the game." With several sides, "opposing" is every company of a different side.
- **Who votes**: a user who owns at least one company in the match, once per opposing company: one unit of that company that took part (Field, Retreated or Field casualty), chosen in a searchable combobox of the sheet. There is no "No MVP": a competitive match does not end until every owner voted (`LiveService.MissingVotes`). Administrators and the organiser vote only through a company they own; they can undo any vote.
- Voting again replaces the earlier vote (the fold keeps the last not-undone vote per `(voter, target company)`); undoing the new vote restores the earlier one. After End match the votes are frozen.
- **Tally** (per target company, computed by the fold, never stored): the unit with the most votes is the company's MVP. A tie gives no automatic MVP: the aftermath sheet shows "Tied: Aldor, Brand" and the owner picks one. No votes: no MVP. Votes are shown in the log and on the card, like every other live fact.
- Log text: "Frodo voted Aldor as MVP of Hador" / "Frodo withdrew the MVP vote for Hador". `CanUndoEntry` for a vote: the voter, administrators and the organiser. The aftermath sheet's `mvp` is prefilled with the winning unit and the badge "Voted MVP (2 votes)".

The objective and MVP entries use two `LiveAction` kinds: `objective` (`UnitID`, `On`) and `mvpVote` (`TargetCompanyID`, `UnitID`, 0 withdraws).
The fold exposes `HasObjective`, `Vote` and `MVPTally` (`LiveState.CheckAs` with `LiveVoter`).
Both kinds are refused once the match has ended.

`MissingVotes(ctx, id, matchID, ownerID)` lists, per owner (0: every owner), the opposing companies they have not voted yet (`MissingVote{OwnerID, Owner, CompanyID, Company}`); a practice match needs none.

```go
EndChecks(ctx, user, id, matchID int64) (core.EndChecks, error) // what the sheets read; ErrNotFound when the match is not open
type EndChecks struct {
    Objective []CompanyObjectives // companies the viewer may edit
    MVP       []MVPBallot         // one per company of an opposing side of any company the viewer owns
    Tally     []MVPTally          // every company of the match: unit -> votes, Winner, Tied
}
```

Errors: `ErrRefused` ("Only the owner of this company can change it"; "You can only vote for companies of the other teams"; "You have no company in this match"), `ErrInvalidLive` (unit not in the company or not eligible). The sheets post the votes and objective switches as live actions together with the end (`POST .../end/propose`, `/end/confirm`).

## Company Break Point in play

The Rules Manual's Break Point is half the models of the starting force; a company whose casualties **exceed** it is Broken and tests Courage at every activation. Muster
of Isengard uses 66%. Data: `campaign.breakPoint` (`default.percent`, `threshold`, `startingModels`, `casualties`, `whenBroken`, `experience`, `scenarioExceptions`) and `companies[].effects[kind=breakPoint].percent` ([definitions.md](definitions.md)).
The app is a **companion, not a simulator**: it counts from the log, shows the state and prompts the tests; it never decides an activation and tracks no turns.

### What is counted

- **Starting models** of a company = its models in the match + its creatures + the temporary Wanderer + models yet to arrive (Maelstrom of Battle); never neutral models. Each roster unit counts `BreakModels` (default 1; a Cavalry model, a Chariot and a War Beast with its Commander are the
  units' own counts; the Mirkwood Warg Marauder card counts as 3, one per rider). Units absent at the start are not in the match.
- **Casualties** = units in Field casualties + units Retreated with the reason "Failed courage" (weighted by their model count) + the manual adjustment. A unit that **left the board** and a mount removed alone are not casualties. Undo and Return to the battle subtract again.
- **Break Point** = `percent x StartingModels / 100`, never rounded and never shown: players see whole models (`LiveBreak.BreakAt`, `ToBreak`). With 15 models it is 7.5 and the 8th casualty breaks the company. **Broken** iff `casualties * 100 > percent * StartingModels`, at every step of the fold: a company is Broken as soon as its casualties exceed the Break Point and whole again if an undo or Return to the battle takes them back under.
- The manual **adjustment** is a logged action `breakAdjust` (`Field` "start" or "casualties", `Delta` -9..+9, never below one starting model or below no casualties, `Note` 0-200) for creatures killed, Ponies not counted, models yet to arrive and composite cards.

- **The scenario's end** (`scenarios[].endsAtLosses`, `{"of": "startingModels", "share": "3/4"}` on the scenarios that print it; none on the others): the game ends once the company has lost that share of its starting models or more. The comparison is fixed and never rounded: `casualties * 100 >= percent * StartingModels`, an "at least" where the Break Point is a strict "more than", so with 16 models the 9th casualty breaks the company and the 12th ends the game. `LiveBreak.EndAt` is that casualty, `ToEnd` how many more are needed, and `EndLeftPercent` the share that remains, which is what the players read (25%). The losses are the casualties above, so a unit that failed a Courage test counts and one that left the board does not, including the units of an older log's All is Lost. Rescue prints "less than 25%", an error of the rules, and is treated like the others.

### Broken, for every viewer

There is no turn tracking (no Start turn, no turn number). When a write makes a company Broken the hub event refreshes every open page of the match, and the live banner of every viewer names it ("Minas Morgul is broken", `MatchBreaks`); it goes when the company is whole again. Older logs that hold `turn` entries still fold (the entries are ignored).

### Screens

1. **Break Point panel** on the company page: always visible, "5 more to Break" and, on a scenario with an end share, "9 more to 25%" ("Broken" and "25% reached" once there; amber at 2 or fewer), above a bar with one cell per starting model. Casualties fill it from the left in the red of the Wounds tracker, models that left the table are striped at the far end, and a mark in the colour of each text's key shows the model that crosses it. Opened, it shows how the counts come from the percent (labelled when it is the company's own rule), the starting models with the **+/- adjustment** (editors, with a note),
   the casualties by name, the scenario exceptions (Defend the Hill: models on the hill do not test; Let nothing through!: the Defender within 6" of the Gate passes automatically; Only a Pony After All: Baggage Ponies do not count for the Defender; Neutral Third-Party: no tests; Maelstrom of Battle: models yet
   to arrive count as on the board) and the break point log lines. The company header shows "Break Point: N% of the starting models" (`CompanyDetail.BreakPointPercent`, the company's own rule or the file's default).
2. **Broken notice** in the live banner (for everybody): "Hador is broken"; the Break Point panel says "Every model tests Courage before it activates, each turn. Models in combat, or that cannot activate, do not test."
3. **Unit cards of a Broken company** in the Field offer a **Courage test**; once the unit has a recorded test they show "Tested: passed", "Tested: failed", "Auto-pass" or "Exempt" (the last result; players test again at the table as often as the rules say). The sheet:
   - **Roll** 2D6 against the unit's Courage (`RollCourage`, which writes nothing; a typed roll is "I rolled" with two dice fields as `RollRefusal` permits): "7 against Courage 4: passed", then **Record: passed** or **Record: failed**. Players may also just tap the outcome.
   - **Passed** (`broken` action) earns `brokenCourageTestPassed` once. A Hero offers **Call Stand Fast** (not for a Hero with Broken Honour or a Wanderer); it is logged, and Warriors pass with the `standFast` reason.
   - **Failed**: the `retreat` action with reason "Failed courage": the unit flees, counts as a casualty, outcome `fled`, no injury roll, no experience.
   - **Passes without a roll** (`courageSkip`, reason `standFast`, `heroicResolve`, `chariot`, `scenario`, `engaged`, `cannotActivate`; the last two are exemptions): no experience. Heroic Resolve does not touch the Might tracker.

There is no All is Lost action: a company that leaves the field is reported by the players as the other side's win ([matches.md](matches.md#concepts)); its units can still retreat one by one ("Left the board").

### Log and fold

```go
LiveBreakAdjust LiveKind = "breakAdjust" // Field "start"|"casualties", Delta, Note
LiveCourageSkip LiveKind = "courageSkip" // UnitID, Reason
```

`LiveAction` carries `Field`, `StandFast` (on a `broken` action: this Hero called Stand Fast), `Reason`, `Item` and `Index`. Old `turn` entries are ignored by the fold; old `allIsLost` flee entries still take the units off the board, and new ones are refused. Old secret objective notes of Fog of War (`secretNominate`, `secretDone`) are ignored by the fold and new ones are refused; while their match is open only their author sees and undoes them, they send no live notification, and after the end they are lines of the log. They still count as the start of the game. `FoldLiveWith(inits, entries, LiveFoldConfig)` and `ValidateLogWith` take what the log does not hold: per company `BreakConfig{Percent, Extra, Exceptions}` (the company's own
`breakPointPercentage` or `campaign.breakPoint.default.percent`; `Extra` is the company's creatures; the exceptions are the match scenario's `scenarioExceptions` as sentences), `BreakModels` per unit and `NoStandFast` (non-Heroes, Wanderers, a Hero whose injuries took Stand Fast away). The service builds the config at every read
(`liveContext.breakConfig`); nothing is stored. `FoldLive` is the same with no config, and then nothing ever breaks. `LiveState.Break(company)` returns `LiveBreak{Percent, StartingModels, Casualties, Threshold100, Broken, AtLeast, LeftTable, EndLossPercent, Exceptions}` (`BreakAt()`, `ToBreak()`, `EndAt()`, `ToEnd()` in whole models, `EndLeftPercent()`);
`LiveCompany` carries `Break`; `LiveUnit` carries `BreakModels`, `CourageHow` and `Temporary`. Migration `00021_live_break_point_kinds.sql` allows the kinds. `broken` and `courageSkip` are allowed even when the company is not Broken (the table may count differently); both are refused for a unit outside the Field.

```go
RollCourage(ctx, user, id, matchID, unitID int64, entered []int, thrown bool) (core.CourageRoll, error) // writes nothing; honours RollRefusal; the unit must be in the Field and have a Courage
type CourageRoll struct{ Dice []int; Total, Courage int; Passed bool }
MatchDefaults(ctx, id, matchID int64) (core.LiveMatchDefaults, bool, error) // EnemyBroken per company; false without live data
MatchBreaks(ctx, id, matchID int64) (map[int64]core.LiveBreak, error)        // the Break Point of every company, for the banner
```

`RollCourage` requires `campaign.breakPoint.whenBroken.courageTest.dice` to be `2d6` and `passIf` `totalAtLeastCourage`. `EnemyBroken` (every company of every other side is Broken at the end) prefills the `enemyBroken` scenario objective of the End match sheet. Routes: the `actions` route
carries `breakAdjust`, `courageSkip` and the rest; `POST .../live/{company}/units/{unit}/courage/roll` rolls.

## Against the Odds items in a live match

Nothing can be noted until the Against the Odds decision is made ([matches.md](matches.md#against-the-odds)): `Apply` asks `Matches.ATOBlocks` (`WithATOGate`) while `Match.ATODecided` is false, because only the match service has the rated companies.

The match's Against the Odds record ([matches.md](matches.md#against-the-odds)) is **overlaid at read time** on the live state (`LiveService.WithATO`, `liveContext.overlay`), so a selection saved after the live state was first read shows at once and one cleared before the game starts disappears.

- **Loaned items** (Tool Kit): `LiveUnit.Loaned []LoanedItem{UnitID, Item, Kit, Detail, Index, AfterGame, Used}`, listed under the Wargear of the card grouped by item ("Climbing ropes ×5", tag "Loaned (Raider's Kit)") with no Rating. Consumables get **Item used** in the action menu (`LiveItemUsed`: `UnitID`, `Item`, `Index`),
  which strikes the item through and logs "Brand used Dwarven Brew"; undo restores it. Items that act in the aftermath (Healing Herbs, Wondrous Cram) read "use after the game" and have no Item used button. Owned items marked used are in `LiveUnit.UsedItems` (any owned item may be marked used, except the aftermath ones). The kit never changes stats.
  The fold checks that the unit is in the Field and carries the item, that it does not act in the aftermath ("Healing Herbs acts after the game: use it in the aftermath.") and was not used ("Brand has already used its Dwarven Brew").
- **Temporary Wanderer**: `LiveCompany.Wanderer *TemporaryWanderer{UnitID, Name, Summary, Rating (45), Courage}` for the company that took it. It is a normal unit of the fold with the synthetic id `core.TemporaryUnitID(company)` (the negative company id), badges "Wanderer" and "Temporary", trackers from `data.wanderers.items`, no Stand Fast, no Dismiss. It is
  counted in the Break Point starting models, can wound, be wounded and roll Courage, cannot be voted MVP and is skipped by `AftermathDefaults` (no roll, no experience, no progression). It appears in the zone its live unit is in and as a wound target.
- **Ambush!** has no live effect beyond the scenario already set.

## Ending a match while live

The match ends only when every participant confirms ([matches.md](matches.md#ending-a-match)). While live:

- **Banner.** When a proposal is pending, every company page of the match (owners and viewers, live via the `match` event; `GET .../live/end/banner?company=N`) shows `#live-end` in the sticky live banner: "Hador proposes to end: Rohan won · Waiting for Gorgoroth". The waiting owner gets **Confirm** (the Confirm sheet) and **Keep playing**, a confirmed owner **Withdraw**,
  the organiser and administrators the override, viewers no buttons. End match is hidden while an end is pending.
- **What is allowed while pending.** `objective` and `mvpVote` entries are accepted and do **not** cancel the proposal. **Any other action or undo** cancels the proposal first (confirmations dropped, a log line "Ending cancelled: Aldor wounded Grashnak"), so the game is never played under
  a confirmed end. The cancellation happens after the game entry is stored, in a second transaction.
- **Log.** Informational entries `endProposed`, `endConfirmed`, `endWithdrawn`, `endCancelled`, `endOverride` appear in the log with the company colour and the author (`LiveKind.Informational()`): written by `LiveService.RecordEnd` from `Matches.EndNotify`, skipped by the fold, not undoable and refused when a player posts them. `LiveService.WithEnds` makes
  `Apply` and `Undo` cancel a pending end after any entry that is not an end check (`LiveKind.EndCheckKind()`). Migration `00023` allows the kinds. Every proposal change notifies the live channel with company 0, so open pages refetch; the last confirmation emits `ended`.
- Whether a company is Broken and `EnemyBroken` are read from the fold at the moment the match ends. Changing the result in the Confirm sheet resets the confirmations except the proposer's.
- The aftermath starts when the match ends and `AftermathDefaults` reads the final fold.
