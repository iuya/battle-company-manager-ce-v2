# Live match mode

Spec and status (built: core `live_*.go`, Postgres `match_live*` tables, the web pages described in [frontend.md](frontend.md) "Live match (built)"; epic `pib`) of what happens to a company's pages while a match is running: trackers for
Wounds, Might, Will and Fate, action trackers that also earn experience, the Retreated and Field casualties zones, a match
log with undo, and live updates for the other players watching. The match record (scenario, result, End match) is in
[matches.md](matches.md); what the finished match does to the companies is in [aftermath.md](aftermath.md); the visual
language is in [frontend.md](frontend.md).

## Principles

- **A running match is an open match** (`Match.Ended == false`). There is no separate "start" step: creating the match starts
  it, and ending it (matches.md) stops it. Matches already open when this ships become live the first time a page asks.
- **The log is the truth.** The state of every unit (wounds, might, will, fate, zone, mount) is a pure function of the
  match's initial values and its not-undone log entries. Undo marks an entry undone and the state is recomputed; nothing
  else mutates state. Experience flags come out of the same log.
- **Players are trusted, as everywhere.** The app records what the players say happened and does the arithmetic; it does not
  check the dice. Trackers are permissive and revertible; the only refusals are impossible states (a stat outside
  `0..start`, spending Fate at 0, acting with or targeting a unit that is not in the Field).
- **Not live, not changed.** A company that is in no open match has the page it has today, no live connection and no trackers.
- **The live view adds to the company page; it never replaces the roster logic.** Cards keep every existing element.

## Experience: which actions earn it (evidence)

`campaign.experience.gains` (Battle Companies file; text of the rulebook's *Experience Gains*, `sources/battle_companies_rules.txt`
"Experience Gains") decides, and the shape of each gain in `campaign.matchInput.perModel` is what matters:

| Gain key | Rulebook | Shape in `matchInput.perModel` | So the live mode records |
|---|---|---|---|
| `woundedEnemy` | "Wounding at least one enemy, even if it was subsequently prevented" | **flag** (once per game) | the first wound declared by the unit; later wounds change nothing. Counts even when every wound was then prevented (Fate). |
| `slainHeroOrMonster` | "Each enemy Hero or Monster they slay" | **count**, `repeatable: true` | +1 per enemy **Hero or Monster** the unit slays, every time. Slaying a Warrior earns nothing, first or not. |
| `fatePrevented` | "Preventing a Wound by spending a Fate point" | **flag** | the first Fate point the unit spends to prevent a wound. |
| `brokenCourageTestPassed` | "Rolling for and then passing a Courage test for being Broken" | **flag** | the first Broken courage test it passes. |
| `participated`, `objective`, `mvp` | | flags | not live: `participated` defaults true in the aftermath sheet; `objective` and `mvp` stay on the aftermath sheet. |

Answers to the user's belief: *only the first wound gives xp*: right (a flag). *Only the first kill gives xp*: **not**
right. There is no xp for killing a Warrior at all; killing Heroes **and Monsters** gives 1 xp each, every one, and a kill
is not needed to earn the wound xp. The note under the gains adds: if the rider of a Cavalry model is slain but its mount
stays on the board, wounds the mount deals still earn the xp for the rider (so a unit's xp is attributed by the unit, not by
the model on the table).

Corollaries the spec fixes (all REVIEW, see the end): a wound on an enemy **mount** counts as wounding an enemy; a
unit is a Hero when `CompanyUnit.Role` is a hero role, a Monster when `Kind` is monster (or the mount of a
monstrous cavalry unit is slain, REVIEW); the kill is credited to the unit named in the action, never split.

## Who may do what

| Viewer | Own company page in a live match | Another company's page in the match |
|---|---|---|
| Owner of a company in the match | trackers and all actions on own units; may declare a wound on an enemy unit (the only write that touches another company, see Wounded a unit) | read only, live updates |
| Administrator, organiser | same as the owner, on any company in the match (REVIEW: same rule as `Match.Access`) | same |
| Other player of the campaign | read only, live updates | read only, live updates |
| Not in the campaign | 404 (as today) | 404 |

`core.LiveAccess(user, match, company, organiserID)` returns `LiveNone` (read only) or `LiveEdit`; the page uses it only to hide controls, the
backend enforces it (`ErrRefused`, "Only the owner of this company can change it"). A finished campaign and an ended match
are read only for everybody, administrators included ("The match has ended"). Undo follows the entry: its author's company
owner, the owner of a company the entry touched, an administrator.

## Match start rules

1. **Creating a match is refused** (`ErrRefused`, shown on the create form) when any company of any side is in another open
   match: "Company X is already in Match N." (link to it). Same for a company in the same match twice (exists today).
2. **Also refused** while a company has a pending aftermath of an earlier match ("Finish the aftermath of Match N first",
   same text as the aftermath lock; REVIEW, question bead `pib.9.q2`).
3. **Companies are locked while the match is open**: every roster action (`RollRecruit`, `BuyEquipment`, `DropEquipment`,
   `TreatInjury`/heal, `BuyCreature`, `HireWanderer`, level-up rolls, recruit, promote, carry, gold changes, the choosing of a path) returns `ErrRefused`
   "Match N is running; finish it first" exactly as the aftermath lock does (`PendingAftermaths`). Renaming units and the formation order stay allowed. Grants by the organiser stay allowed (REVIEW).
   Implemented in the same guard as the aftermath lock (`companyDeps`), so it is one rule in one place (`core.LockedCompany`).
4. **Who is in the match** (the roster that appears in the zones): every unit of the company that is not `Injured`
   or `AtHealers` at start; those two are listed under "Missed this game" and take no part (no xp, as the aftermath
   sheet already says).
5. **Initial values** are taken when the match opens (or lazily at the first read for matches opened before this existed): Wounds, Might,
   Will, Fate from the unit's current profile (`CompanyUnit.Stats`, so promotions and path boosts count), Mounted when the
   unit has a mount (equipment class mount, or Kind cavalry). Later permanent changes cannot happen (rule 3), so the maxima
   are fixed for the match.

## Screens (mobile first, 360 px up)

### Company page in a live match

Applies to the owner's My company tab and to every other player's view of the company (`/campaigns/{id}/companies/{company}`).

Top to bottom:

1. **Live banner**, accent surface, sticky below the app bar: "Match N is running" with the scenario name when chosen, a link
   to the match, a live-connection dot (green "Live", grey "Reconnecting...", hidden on the owner's own tab when connection works)
   and, for the owner, a button **Match log** (opens the log sheet, below). The first button is **To wound** (the To Wound chart in a window, [matches.md](matches.md#to-wound-chart-a-window-built); the window is a sibling of the banner and outside `#live-zones`, so a live refresh never closes it). On 360 px the banner is two or three lines.
2. **Locked notice** instead of the buy/recruit/heal controls: the buttons are hidden, not disabled, and a single line
   "The company is locked until the match ends." explains why. Counters (Points, Units, W/D/L, Influence, Gold) stay.
3. **Battle company** section: the units in the Field zone, in the company's order. Unit cards as today plus trackers.
4. **Retreated** section (collapsed when empty, shows "None" in muted text): units that left the table. Cards are the compact form
   (name, type, a "Retreated" badge with the reason "Failed courage" or "Left the board", nothing else); the owner has **Return to the battle** (= undo of the retreat, logged as its own reversal; only from this card, so an error is easy to fix).
5. **Field casualties** section (collapsed when empty): units at 0 Wounds or marked Died. Compact card with a "Casualty" badge
   (reason "Wounded by <unit> (Company)", "Died: <note>", "Wounds lowered to 0"), the line "Rolls for injuries after the match" and
   **Return to the battle** for the owner (undo). A note that these units still show their fate in the aftermath.
6. **Missed this game** (only when there are such units): muted compact cards.
7. **Cemetery** as today (unchanged, read only here).

Sections 4 and 5 sit *between* the Battle company and the Cemetery, as the user asked. Empty sections do not take room on 360 px: a one line header with a count badge, tap to expand.

States: *empty live* (everyone in the field: sections 4 and 5 show "None"), *viewer* (same page, no tracker controls, no action menu), *finished campaign / ended
match* (the live banner is replaced by the normal aftermath banner), *connection lost* (banner dot grey, the page keeps working and refetches on reconnect), *stale write* (a 409 re-renders the card with the notice "X changed this; check and try again").

### Unit card in the Field zone

Everything the card has today (frame, kind icon, name, attributes, equipment, path, rules), plus:

- **Trackers replace the static W, M, W, F values** in the attribute row (the other attributes are unchanged). A tracker shows `current / start`, a minus and a plus button
  (44 px touch targets, the whole cell is the target on 360 px), long press or tap on the number opens a small "set to" stepper. Rules: Wounds, Might, Will and Fate stay within `0..start`
  (REVIEW: cap at the starting value; the rulebook has no routine way above it, an escape hatch is "set to").
  A colour tone shows low values (Wounds at 1 uses the existing danger token). Heroes show M W F, warriors only Wounds.
- **Wounds -1** from the tracker is a manual change (no xp, no target). At 0 Wounds the unit moves to Field casualties with a toast
  "Aldor fell. Undo". The plus button is the way back (it is the undo of the fall, logged as "Wounds +1 (manual)"; the unit returns to the Field).
- **Actions menu** (button "Actions" at the bottom of the card, a bottom sheet on 360 px, a popover from 768 px). Entries, each with its prompt:

| Action | Prompt | Effects | Log text |
|---|---|---|---|
| Wounded a unit | see Wounded a unit | target wounds, attacker xp flags | "Aldor wounded Grashnak (2 wounds, 1 saved by Fate)" |
| Passed a fate roll | confirm only (a choice of how many: stepper, default 1, max current Fate) | Fate -n on this unit, `fatePrevented` flag; refused at 0 Fate | "Aldor spent Fate (1 left)" |
| Passed a Broken courage roll | confirm (when the company is Broken the card prompts it, see Company break point) | `brokenCourageTestPassed` flag; no stat change. Always offered, also before the app says Broken (a table that does not use the turn button still works) | "Aldor passed a Broken courage test" |
| Retreated | choice "Failed a courage roll" / "Left the board", optional note | zone Retreated, outcome `fled` in the aftermath; no injury roll; removed from the active page; cannot act or be targeted | "Aldor retreated (failed courage)" |
| Died | optional note ("Fell off a cliff"), for deaths not caused by an enemy wound | zone Field casualties with reason "Died"; rolls for injuries later | "Aldor died: ..." |
| Mount removed / Mount re-added | see Mount | Mounted flips | "Aldor lost his mount" |

Hidden when the unit has no use for it (Fate for a unit with 0 Fate at start: "Passed a fate roll" is not offered; Mount actions for units without a mount).
Actions on a unit outside the Field are not offered (the compact cards only have *Return to the battle*).

### Wounded a unit (the only cross-company action)

A sheet in steps on 360 px (one screen on 768 up):

1. **Who**: list of enemy units that are in the Field zone, grouped by company (side name), each row shows name, type, current / start Wounds and a small mount icon when mounted. Searchable. Allied
   companies in the same match are shown after a "Friendly fire" divider (not hidden: the app does not know alliances). The unit itself is not offered.
2. **Which part**: only for a mounted target, "Rider" or "Mount".
3. **How many wounds caused** (stepper, 1 to 9, default 1) and, when the target is a Hero with Fate, **How many were saved by Fate** (stepper `0..min(caused, target Fate)`, default 0). Below, the preview line
   "Grashnak: Wounds 3 -> 1. Aldor earns the wounding XP." or "...Grashnak falls. Aldor earns a kill XP (Hero)." in plain words.
4. **Confirm**. One log entry for the declaration with linked effects (below).

Semantics of one declaration (`LiveWound`):

- `caused` wounds are dealt; `savedByFate` of them are prevented and cost the target `savedByFate` Fate points (target tracker -n, target `fatePrevented` flag set on the first one, as if "Passed a fate roll" had been logged on the target);
  the target's Wounds drop by `caused - savedByFate`. A part "Mount" drops the mount's wounds the same way (a mount with more than one Wound has its own counter; at 0 the mount is removed as in Mount removed) and never makes the rider a casualty.
- The attacker gets `woundedEnemy` when `caused >= 1`, **even if all were saved** (the rule). Idempotent: the flag is a set, the log shows each declaration.
- If the target reaches 0 Wounds (rider part) it is a casualty (Field casualties, reason "Wounded by Aldor (Company)") and, when it is a Hero or Monster, the attacker's `slainHeroOrMonster` count rises by one. Undoing the declaration undoes all of it.
- The declaration is applied at once; the target's owner sees the card change live and a toast "Aldor wounded Grashnak (2). Undo" (REVIEW question `pib.9.q1`: apply at once vs confirm by the target's owner; the default is apply at once, either owner may undo).

### Mount removed / re-added

- **Mount removed** ("fell, now infantry") sets `Mounted = false`. The card then:
  - strikes through equipment whose annotation says it needs the mount (`requiresMount`, lances; Barding is the steed's) and the rules the mount gave (`grantedTo: mount`, the Pony's Timid), and the **Cavalry** keyword; the items stay listed and are not lost, the
    strikethrough has `aria-label` "needs a mount" and the text stays readable. Stats are not recomputed (REVIEW: the dismounted Mv/base would come from the infantry profile; out of scope);
    a small "On foot" badge near the name.
  - the limits on the company page do not change (the roster is the same).
- **Mount re-added** reverses it (a button on the same menu, plus undo).
- Removing the mount is a plain state change: no xp, no injury; if the mount is a creature in the company's creatures (Baggage Pony) it is not affected.
- A wound declaration with part Mount that takes the mount to 0 does this automatically, logged as one entry with the wound.

### Match log

A bottom sheet on 360 px (a side panel from 1280), reached from the live banner and from the match page ("Live log" card, read only for viewers, below History). Newest first, grouped by minute; each row: time, text, author's company colour dot,
"Undo" for who may (author or involved owners), a row note "Undone" is **not** shown: undone entries disappear from the log (the user's rule) and are only kept in the database for audit
(`undone_by`, `undone_at`). Empty state: "Nothing has happened yet." Undoing entry N:
recomputes state; entries that were applied after it are kept as long as they remain valid (a wound entry is a delta: wounds +n restored, no conflict). If undoing would make a later entry impossible (undoing a retreat
after the unit was re-targeted cannot happen since a retreated unit cannot be targeted; undoing a "Fate spent" after later fate changes that no longer fit within `0..start`) the undo is refused with the reason
("A later change depends on it; undo that first"). Manual tracker changes appear in the log like actions.

## Zones: how a unit gets into each

Zone is derived from state, in this priority:

1. **Retreated** if a not-undone `retreat` entry exists for the unit.
2. **Field casualties** if a not-undone `died` entry exists, or Wounds are 0.
3. **Field** otherwise.

A unit in Retreated or Field casualties cannot be targeted by Wounded a unit and cannot act. "Return to the battle" is an undo of the entry that moved it (for a casualty by wounds the plus on the card is not available: the unit
is a compact card; the owner uses the Return button, which undoes the entry that took Wounds to zero, or if that is not unique, adds a manual "Wounds +1").

## Aftermath feed

At **End match** (and for the aftermath sheet that is built at first read) the default `AftermathInput.Models` comes from the live state instead of "everyone survived":

- `Outcome`: Field -> `survived`; Retreated -> `fled` (no injury roll, the aftermath already supports it); Field casualties -> `casualty`.
- `Gains`: `woundedEnemy`, `fatePrevented`, `brokenCourageTestPassed` from the flags, `slainHeroOrMonster` from the count; `participated` stays default true.
- `objective`, `mvp`, `possessesRelic` are not live: the sheet leaves them as today.
- The sheet shows a badge "From the live match" beside every prefilled value; the player can change anything (players are trusted). Saving the sheet no longer reads the log.
- A match with no live data (recorded after the fact, or opened before this feature) behaves as today.
- After the match ends the trackers disappear, the log stays on the match page (read only, permanent, reviewable at any time, also after the campaign is finished; user requirement 2026-10-21). Live state survives server restarts: all state is in Postgres and the SSE hub only notifies.

`core.LiveMatches.AftermathDefaults(ctx, matchID, companyID) ([]ModelSheet, error)` is the single call the aftermath service makes (it declares the interface it needs; the live service implements it).

## Live sync for viewers (proposal, to hand to the backend planner)

Requirement: a viewer of a company in a running match sees changes within about a second without reloading and cannot modify
that company's trackers. Companies not in a running match get none of this.

**Recommendation: Server-Sent Events**, one stream per match: `GET /campaigns/{id}/matches/{match}/events` (`text/event-stream`).
Reasons: one-way push is all that is needed (writes stay ordinary htmx POSTs), the session cookie authenticates it, it works
through the Go `http.ResponseWriter` with `http.Flusher`, reconnects and `Last-Event-ID` are built into `EventSource`, and htmx has the SSE extension
(`hx-ext="sse"`, `sse-connect`, `hx-trigger="sse:..."`). WebSockets add upgrade handling and a second protocol for no gain; true webhooks do not fit browsers.

Design constraints:

- **Events carry a pointer, not HTML.** The event is `event: changed`, `id: <seq>`, `data: {"company": 12, "seq": 41}`. The page that cares (a company page showing company 12, the match page's log) answers with an
  `hx-get` of its own view fragment. This keeps rendering per viewer (the owner sees controls, a viewer does not) and puts no HTML in the broker.
- `seq` is the match's log sequence (monotonic per match). On reconnect the client sends `Last-Event-ID`; the server replays nothing and sends one `changed` for every company so the page refetches (state is the log, the fetch is idempotent).
- Events (final names in architecture.md): `company-<id>` and `match` (a change or an undo), `ended` (match over: the page reloads into the normal view), `ping` comment every 25 s (keeps proxies open; the response sets `Cache-Control: no-cache`, `X-Accel-Buffering: no`).
- **Fan-out** inside one process with a per-match hub (subscribe/unsubscribe, buffered channel of 1 and coalescing: a slow client gets "something changed", never a backlog). Across processes (if the app is ever run twice) PostgreSQL `LISTEN/NOTIFY` on channel `live_match` with payload `{match, company, seq}`; the hub is the listener.
- **Authorisation**: same as reading the page (campaign member). The handler re-checks that the match is still open when it connects and when each event is sent.
- **Limits**: a viewer needs at most one stream per tab; cap open streams per user (REVIEW: 10) and per match (REVIEW: 50); streams end at `ended` and after a read deadline-free idle of 2 hours (client reconnects).
- **Fallback**: if `EventSource` fails or is unavailable, the live banner polls the company fragment every 10 s (`hx-trigger="every 10s"`) and shows the dot grey.
- **No live cost outside a match**: the SSE script is only included on pages of a company in an open match.
- A write answers its own page directly (htmx swap), the stream makes the *other* tabs follow; the author's tab ignores its own `changed` when the `seq` is the one it just received.

## Go contracts

The data types, errors and repository interfaces are declared in `internal/core/live.go`; the design (fold, transactions, hub, SSE) is in
[architecture.md](architecture.md) "Live match". `internal/web` declares what the pages need and `core.LiveService` implements it:

```go
type liveMatches interface {
    // Live: the live state of a company in an open match; ErrNotFound when the company is in no open match (the page renders as today).
    Live(ctx context.Context, user core.User, id, companyID int64) (core.LiveCompany, error)
    // Apply records an action; returns the acting company's new state and the entry. ErrRefused / ErrInvalidLive / ErrNotFound.
    Apply(ctx context.Context, user core.User, id, matchID, companyID int64, a core.LiveAction) (core.LiveCompany, core.LiveEntry, error)
    // Undo removes an entry and returns the state of the company the viewer is on; ErrLiveDepends when a later entry depends on it.
    Undo(ctx context.Context, user core.User, id, matchID, entryID, companyID int64) (core.LiveCompany, error)
    // Log lists not-undone entries newest first (CanUndo set for user); `before` is a seq for paging (0: newest).
    Log(ctx context.Context, user core.User, id, matchID, before int64, limit int) ([]core.LiveEntry, error)
    // Subscribe feeds the SSE handler; Next blocks for a coalesced core.LiveBatch. ErrRefused at the stream caps.
    Subscribe(ctx context.Context, user core.User, id, matchID int64) (core.LiveStream, error)
}

// Declared by the aftermath service; implemented by LiveService.
type liveDefaults interface {
    AftermathDefaults(ctx context.Context, id, matchID, companyID int64) ([]core.ModelSheet, bool, error) // false: no live data
}
```

Differences from the first proposal: a `return` action kind (Return to the battle is its own logged action, not an undo); `Set` is stored as the delta; the
event stream names events `company-<id>`, `match` and `ended` (no company in the data); `OpenMatchOf` is `CompanyDetail.OpenMatch` (loaded with the company state);
`Undo` takes the company the viewer is on; the creation guard returns `*core.CompanyBusyError`.

Concurrency: all writes are single transactions that append the entry and bump `Seq`; two players hitting the same unit at the same time both succeed
(deltas), which is the normal case at a table (the target's owner marks Fate while the attacker declares the wound).
No per-write revision is required; the page re-renders the whole card from the returned state.

Routes (all htmx, CSRF as elsewhere): `POST /campaigns/{id}/matches/{match}/live/{company}/actions` (form fields by kind; answers the card fragment `#unit-{id}`, the zones and a toast), `POST /campaigns/{id}/matches/{match}/live/log/{entry}/undo`,
`GET /campaigns/{id}/matches/{match}/live/log` (sheet), `GET /campaigns/{id}/companies/{company}/live` (fragment of the whole live section; what the SSE `changed` triggers), `GET /campaigns/{id}/matches/{match}/events` (SSE).

## Persistence

Tables `match_live`, `match_live_unit` and `match_live_log`, indexes, retention and NOTIFY: [architecture.md](architecture.md) "Live match".

## Acceptance criteria

- [ ] Creating a match with a company already in an open match is refused with the named company and a link; so is one with a pending aftermath (REVIEW).
- [ ] While a match is open the company page shows the live banner, hides every buy/recruit/heal/level-up control and the backend refuses those calls with "Match N is running"; after End match the page and the calls are as before.
- [ ] Sections Battle company, Retreated, Field casualties, (Missed this game) sit before the Cemetery; empty ones are collapsed one-liners; no horizontal scroll at 360 px; light and dark; against `/styleguide`.
- [ ] Wounds/Might/Will/Fate are trackers `current / start` with +/-, stay within `0..start`, Wounds 0 moves the unit to Field casualties, + brings it back.
- [ ] Wounded a unit: target picker (enemies, then friends), rider/mount for mounted targets, wounds caused and saved by Fate with live preview; target Wounds and Fate change; attacker earns the `woundedEnemy` flag once (also when all wounds are saved), `slainHeroOrMonster` +1 per Hero/Monster slain, nothing for slain Warriors; target gets `fatePrevented` once.
- [ ] Passed a fate roll lowers Fate and is refused at 0; Broken courage sets the flag; Retreated and Died move the unit and give the right aftermath outcomes (`fled` without injury roll; `casualty`).
- [ ] Mount removed strikes through lances (`requiresMount`), mount-granted rules and Cavalry, shows "On foot"; re-adding restores; the same through undo.
- [ ] Every change is in the log; Undo removes the entry and restores state, is refused with a reason when a later entry depends on it; undone entries are not shown.
- [ ] A viewer (other player, non-editor) sees no tracker/action controls, the write routes answer 403, and they see a change made by the owner within about 2 s without reloading; reconnect after a network loss refetches; the poll fallback works.
- [ ] No SSE connection is opened on a company page of a company not in an open match.
- [ ] End match prefills the aftermath sheet outcomes and gains from the live state, with the "From the live match" badge; the sheet stays editable.
- [ ] Tests: core table tests for the fold (every action, every undo, xp flags, zone priority); handler tests for permissions; integration test for log/seq and NOTIFY; template test for the viewer/owner/ended states.

## REVIEW

- Cap of every tracker at its starting value; "set to" is allowed within the same range.
- Admin and organiser may use the trackers of any company (the match rule).
- A wound on an enemy mount earns the wounding XP (the rule's text says "wounding at least one enemy"; the cavalry note supports mounts' wounds counting for the rider).
- Monster = `Kind` monster; a monstrous mount slain does not count as a slain Monster unless the rider is also slain.
- Broken is tracked only through the turn button (see Company break point); "Passed a Broken courage roll" stays always offered.
- Dismounted units keep their mounted profile (no infantry Mv).
- Grants by the organiser stay allowed during a match; renaming and order too.
- Mount's own wound counter only when the mount profile has more than 1 Wound.
- Stream caps (10 per user, 50 per match), 25 s keepalive, 10 s poll fallback.

## End checks: objective and MVP (backend built, mtu.15; web: mtu.16)

Two things the players settle when the game ends, both worth experience (`campaign.experience.gains`: `objective` and `mvp`, 1 xp each) and both
recorded in the live log, so they undo, show in the log and feed the aftermath sheet like every other live fact. They are available at any time while the match is open
(players usually do them at the end) from the live banner button **End checks** and as a card on the match page; **End match** first reminds the editor of what is still open
("No unit has the objective yet · you have not voted for Hador") with *Go back* and *End anyway*; it never blocks.

### Got objective (per unit)

- Rule text: "Carrying or being within range of an Objective at the end of the game (includes escaping off the board edge...)". A **flag per unit**, so several units of a company can have it; each earns 1 xp.
- **Who sets it**: the owner of the company, an administrator and the campaign organiser (`LiveAccessFor == LiveEdit` on that company). Nobody else; a viewer sees the card read only with the units that have it.
- **What is listed**: only the units of the company being edited that are in the Field or Retreated zone (a unit that left by the board edge may count; casualties and "Missed this game" cannot). The owner sees only their own company; an administrator or organiser sees one list per company of the match, each headed by its company name. No unit of another company is ever offered to its non-owner.
- Control: a checklist (switch per unit, 44 px rows), changes at once with a toast, "Hador: got the objective. Undo". Setting again is a no-op; switching off is its own log entry.
- Log text: "Aldor got the objective" / "Aldor no longer has the objective". Author: whoever clicked; company: the unit's.
- Anywhere the aftermath sheet is prefilled (`AftermathDefaults`), `objective` = 1 on those units, with the badge "From the live match". The player can still change it.

### Select MVP (votes between opposing companies)

- Rule text: "Being declared MVP by the opposing player after the game". With several sides, "opposing" is every company of a different side (not a teammate, REVIEW).
- **Who votes**: a user who owns at least one company in the match (the player at the table). Administrators and the organiser vote only through a company they own; they can undo any vote like any entry. A user with companies on one side votes once per opposing company.
- **One vote per voter per opposing company**: the voter picks **one unit of that company** that took part (Field, Retreated or Field casualty; not "Missed this game"), or "No MVP" (clears the vote). The control is a list per opposing company (`Select MVP`) with unit rows as radio buttons (title, type, current Wounds), the current choice marked.
- **Changeable until the match ends**: voting again replaces the earlier vote (a new log entry; the fold keeps the last not-undone vote per `(voter, target company)`), undo of the new vote restores the earlier one. After End match the votes are frozen (read only; reopening the match unfreezes them).
- **Tally** (per target company, computed by the fold, never stored): the unit with the most votes is the company's MVP. **Tie** (two or more units with the top count): there is no automatic MVP; the aftermath sheet shows "Tied: Aldor, Brand" and the target company's owner picks one of them there (or none). **No votes**: no MVP. A single vote decides in a two player game; in a multi player game several voters (e.g. 3 opponents) make a tie possible.
- **Privacy**: votes are shown in the log and on the card ("Frodo voted for Aldor (Hador)"), like every other live fact (REVIEW: no secret ballot; the table is trusted).
- Log text: "Frodo voted Aldor as MVP of Hador" / "Frodo withdrew the MVP vote for Hador". The entry belongs to the target company and is authored by the voter; `CanUndoEntry`: the voter, an administrator or the organiser.
- **Feed to the aftermath sheet**: the sheet has one MVP flag per company (`mvp`, at most one unit, chosen by the owner). `AftermathDefaults` sets `mvp` = 1 on the winning unit with the badge "Voted MVP (2 votes)"; on a tie it sets none and shows the tied units as a hint; with no votes the owner chooses freely as today. The owner can still change it (players are trusted, `Validate` still allows one). The validity rule "at most one MVP" is unchanged.

### Log, undo, access (shared)

New `LiveAction` kinds `objective` (`UnitID`, `On bool`) and `mvpVote` (`TargetCompanyID`, `UnitID`, 0 = withdraw). Both are ordinary entries of `match_live_log` with `payload` json; zone and stat state are untouched, the fold gains `Objective map[unitID]bool` and `Votes map[(voterUserID,targetCompanyID)]unitID`. Undo is refused only when it would leave an impossible state (a vote for a unit that cannot exist cannot happen). Both are refused once the match has ended ("The match has ended") and, for a finished campaign, like every live write.

```go
// Declared by the web layer; implemented by LiveService (the existing Apply/Undo take the new action kinds).
type liveEndChecks interface {
    // EndChecks is the card: per company the units that may get the objective (with the flag), per opposing company the candidates, the viewer's current vote and the tally.
    // ErrNotFound when the match is not open or the viewer is not in the campaign.
    EndChecks(ctx context.Context, user core.User, id, matchID int64) (core.EndChecks, error)
}
type EndChecks struct {
    Objective []CompanyObjectives // companies the viewer may edit (owner: one; administrator/organiser: all); read only list for the others is empty
    MVP       []MVPBallot         // one per company of an opposing side of any company the viewer owns; empty for a viewer who owns none
    Tally     []MVPTally          // every company of the match: unit -> votes, Winner (0 on a tie or no votes), Tied []int64; shown to every player
    Pending   []string            // what the End match reminder lists
}
```
Errors on `Apply`: `ErrRefused` ("Only the owner of this company can change it"; "You can only vote for companies of the other teams"; "You have no company in this match"), `ErrInvalidLive` (unit not in the company or not eligible).
Routes (htmx, CSRF): `GET /campaigns/{id}/matches/{match}/live/end` (the sheet/card), `POST .../live/{company}/objective` (`unit`, `on`), `POST .../live/{company}/mvp` (`unit`, 0 withdraws; the voter is the session user).

Acceptance criteria:
- [ ] The Got objective list shows only the editor's own company units in Field/Retreated; an administrator/organiser sees one list per company; a viewer cannot change anything (403).
- [ ] Several units of a company can have the objective; each is a log entry and undoes; after End match the sheet has `objective` = 1 on them with the live badge.
- [ ] Select MVP lists only companies of other sides; one vote per voter per company, changing replaces it, withdrawing works; refused after the match ends and for a user without a company in the match.
- [ ] Tally: most votes wins; tie gives no default and the sheet names the tied units; no votes leaves the field free; the sheet's MVP flag is prefilled with the "Voted MVP" badge and stays editable.
- [ ] Undo of a vote restores the previous vote; undone entries vanish from the log (as everywhere).
- [ ] End match shows the reminder of open checks and does not block; 360 px, light and dark, `/styleguide` tokens.
- [ ] Tests: fold tables (vote replace, undo restores, tie, no votes, teammate refused), service permissions, `AftermathDefaults` prefill, template states (owner, admin, viewer, ended).

Built (core): `LiveObjective` / `LiveMVPVote` actions (`LiveAction.On`, `TargetCompanyID`), the fold (`LiveState.HasObjective`, `Vote`, `MVPTally`, `CheckAs` with `LiveVoter`), `CanUndoEntry` (a vote: its voter, administrators, the organiser), `LiveService.EndChecks` (`core.EndChecks`), the `objective` / `mvp` gains in `AftermathDefaults` (a tie leaves `mvp` to the owner; the tally card names the tied units), migration `00016_live_end_checks.sql` (kinds, including the secret objective kinds of mtu.20). A vote is `Apply(user, id, match, targetCompany, LiveAction{Kind: LiveMVPVote, UnitID})`: the permission is "owns a company on another team", not the editor rule. Setting an objective flag that is already set is refused (the page treats it as a no-op).

## Company break point in play (spec, designed; epic `36y`)

The Rules Manual's Break Point is half the models of the starting force (not rounded, never changes); a company whose casualties
**exceed** it at the **start of a turn** is Broken for the rest of the game, tests Courage at every activation, and may flee
(All is Lost). Muster of Isengard uses 66% (REVIEW B1: Break Point = 66% of the starting models). Data: `campaign.breakPoint`
(`default.percent`, `threshold`, `startingModels`, `casualties`, `whenBroken`, `allIsLost`, `experience`, `scenarioExceptions`),
`companies[].effects[kind=breakPoint].percent` ([definitions.md](definitions.md)). The app is a **companion, not a simulator**: it
counts from the log, flips the state when the players say a new turn starts, and prompts the tests; it never rolls or decides
an activation.

### What is counted (from the log)

- **Starting models** of a company = its models in the match + creatures of the company in the match + the temporary Wanderer
  + models yet to arrive (Maelstrom of Battle), minus none; never neutral models. Each roster unit counts `BreakModels` (default 1;
  Cavalry, a Chariot and a War Beast with its Commander are the units' own counts; the Mirkwood Warg Marauder card counts as **3** models, one per rider (user decision 2026-10-22): `BreakModels` 3 and each rider removed is a casualty, so the card needs a rider counter, bead `36y.12`),
  computed when the live state is first read (units absent at start are not in the match, so not counted).
  Creatures are counted from the company's creatures (B4; the Baggage Pony of a Defender in Only a Pony After All is subtracted by the
  adjustment below, the live card says so). Anything the data cannot know is fixed by the manual **adjustment** (below).
- **Casualties** = units in Field casualties + units in Retreated with reason "Failed courage" (they fled as a casualty, B7) + manual
  adjustment. A unit that **left the board** (reason "Left the board", an escape through a scenario edge) is **not** a casualty (B6). A mount
  removed alone is not a casualty; the rider's fall is. Undo and Return to the battle subtract again, as everything else (it is a fold).
- **Break Point** = `percent x StartingModels / 100`, shown with one decimal ("6.5"); **Broken** iff `casualties * 100 > percent * StartingModels`
  tested **only** at a turn boundary (the integer test, B9).
- The adjustment is a logged action `breakAdjust` (`Field` "start" or "casualties", `Delta` -9..+9, `Note` 0-200): creatures killed,
  Ponies not counted, models yet to arrive, composite cards. It undoes like everything else and appears in the log ("Hador: 1 more starting model (late arrival)").

### Turn boundary and Broken

New action `turn` (match level): **Start turn N** on the live banner (any editor of a company in the match: owners, organiser, administrators).
It carries the turn number it starts (`Turn`, 1 for the first); a second press with a stale number is refused ("Turn 3 has already started")
so two players do not advance twice. The fold evaluates the test **for each company at that entry** with the counts as they are then: a company
that fails the test (casualties above the Break Point) and was not Broken becomes Broken with `BrokenAtTurn = N`, permanently. Undoing the
`turn` entry un-breaks the companies it broke. The app never infers a turn: a table that never presses the button never gets Broken, and
the card then shows only the counts (the "Passed a Broken courage roll" action keeps working, REVIEW).

Before the turn button is pressed the panel warns, from the current counts: "Hador will be Broken at the start of the next turn
(7 casualties, Break Point 6.5)" (informational, can change if a unit returns).

### Screens (mobile first, 360 px up)

1. **Break Point panel** on the company page (live banner area, collapsed to one line, tap to open; also on the match page "Live" card for
   viewers): "Break Point 6.5 of 13 · Casualties 4 · 50%" with a thin meter (text first, the bar an addition), the percent labelled as
   the company rule ("Muster of Isengard: 66%"). Opening shows: starting models with the **+/- adjustment** buttons (editors only, with a note field), the
   casualties listed by name (Field casualties and Failed courage), the scenario exceptions for the match's scenario
   (Defend the Hill: models on the hill do not test; Let nothing through!: Defender within 6" of the Gate passes automatically;
   Only a Pony After All: Baggage Ponies do not count for the Defender; Neutral Third-Party: no tests; Maelstrom of Battle: models yet to
   arrive count as on the board), and the log lines of the break point.
2. **Turn button** in the live banner: "Start turn 4" (44 px), a small "Turn 3" label otherwise. After the first press the banner always shows the turn.
3. **Company broken banner** (accent/danger surface, below the live banner, on the company page of the broken company for everybody, sticky on 360 px): "Hador is Broken
   (turn 4). Every model tests Courage before it does anything else. Models in combat, or that cannot activate, do not test." with the button
   **All is Lost** while the offer is open (below) and a link "Who has tested this turn".
4. **Unit cards of a Broken company** in the Field zone: a chip beside the name "Test due" (danger tone, text) until the unit's courage is
   resolved **this turn**, then "Tested" / "Auto-pass" / "Exempt" (muted). The card's primary button is **Courage test**, which opens a small sheet:
   - **Roll** (uses the dice component and `RollCourage`, 2D6 against the unit's Courage; the sheet shows "7 against Courage 4: passed" and then
     offers **Record: passed** or **Record: fled** prefilled with the roll; allowed only as the campaign's `RollRefusal` permits, a typed roll
     is "I rolled" with two dice fields). Players may also just tap the outcome (they rolled at the table).
   - **Passed** (rolled): `broken` action, earns the `brokenCourageTestPassed` flag once. A Hero offers the switch **Call Stand Fast**
     (hidden for a Hero with Broken Honour and for Wanderers, which cannot Stand Fast): Warriors of the company get the hint "A Hero called
     Stand Fast this turn: Warriors within 6" in line of sight pass automatically".
   - **Failed**: the existing `retreat` action with reason "Failed courage": the unit flees, is removed as a casualty (counts for the Break Point), outcome `fled`, **no injury roll**, no experience.
   - **Passes without a roll**: buttons "Auto-pass: Stand Fast", "Auto-pass: Heroic Resolve", "Auto-pass: Chariot", "Auto-pass: scenario rule", and
     "Exempt: in combat", "Exempt: cannot activate". They record `courageSkip` (below); **no experience**. Heroic Resolve does not
     touch the Might tracker (the player spends the point there; REVIEW: the rule says a Hero spends Might, Mastery waives it when Broken).
   Cards outside the Field zone, and every card of a company that is not Broken, do not show the chip.
5. **Who has tested this turn**: a list of the Field units with "Test due" first; it is the table's checklist and carries no rule.
6. **All is Lost offer** (sheet): shown on the Broken banner from the turn the company became Broken until the next `turn` entry ("This is decided at
   the beginning of the turn they are broken and before Priority", B2): **Flee the battlefield** / **Fight on**. Text: "Your opponent claims victory and full
   rewards. You earn only the Influence for taking part (the injury rolls and experience still happen)". Flee: all
   remaining Field units become Retreated ("Left the board"), they are not casualties but they were Broken's survivors: the company's outcome is Fled; the match page's
   result form prefills "Fled" for the company and a hint "The other side claims victory (All is Lost)" (it does not end the match). **Fight on** closes the offer. Only editors of the company; undoing the flee entry restores everything.

### Log and fold

New `LiveKind`s (declared in `internal/core/live.go`, kinds also in the log table's check constraint). Built (36y.13): the five kinds `turn`, `breakAdjust`, `courageSkip`, `allIsLost`, `itemUsed` (itemUsed belongs to the next section) are declared with the `LiveAction` fields `Turn`, `Field`, `StandFast`, `Reason`, `Flee`, `Item`, `Index`; migration `00021_live_break_point_kinds.sql` allows the kinds and the postgres payload (`liveActionJSON`) stores the fields; `TestLiveBreakPointAndItemKinds` covers the round trip, a match-level turn entry (company 0), undo, a restart and the refusal of an undeclared kind. The fold and `Apply` rules follow in 36y.12:

```go
LiveTurn        LiveKind = "turn"        // Turn (the number it starts); match level, company 0
LiveBreakAdjust LiveKind = "breakAdjust" // Field "start"|"casualties", Delta, Note
LiveCourageSkip LiveKind = "courageSkip" // UnitID, Reason: standFast|heroicResolve|chariot|scenario|engaged|cannotActivate
LiveAllIsLost   LiveKind = "allIsLost"   // Flee bool: true flees, false fights on
// LiveAction gets: Turn int; Field string; StandFast bool (on a broken action: this Hero called Stand Fast); Reason string (courageSkip). Why "courage" already exists on retreat.
```

```go
// LiveCompany gets
//   Break LiveBreak
//   Turn  int // match level: the last started turn, 0 before the first
type LiveBreak struct {
    Percent        int   // company breakPoint effect or the default 50
    StartingModels int   // computed + adjustment
    Casualties     int   // Field casualties + Failed courage + adjustment
    Threshold100   int   // Percent * StartingModels; the Break Point is Threshold100/100
    Broken         bool
    BrokenAtTurn   int   // 0 when not Broken
    WillBreak      bool  // casualties*100 > Threshold100 and not yet Broken: warning for the next turn
    Offer          bool  // the All is Lost offer is open (turn == BrokenAtTurn and no allIsLost entry since)
    Fled           bool  // chose to flee
    Exceptions     []string // scenario exceptions for the match's scenario (texts from the data)
}
// LiveUnit gets: BreakModels int (default 1); CourageTurn int (the turn its courage was resolved: broken, courageSkip or a failed retreat); StandFastTurn int (a Hero that called Stand Fast, for the hint); Temporary bool.
```

`Apply` rules: `turn` is refused for a stale `Turn`, on an ended match, for a non editor (`ErrRefused`); `courageSkip` and `broken` are refused
for a unit outside the Field and (REVIEW) are allowed when the company is not Broken, to keep tables without turn tracking working; `allIsLost`
is refused when no offer is open ("There is nothing to flee from yet"). A failed test is `retreat` with `Why: "courage"` (unchanged). Experience: only `broken` sets
`brokenCourageTestPassed`; `courageSkip` never does.

```go
// Added to core.LiveService and the web `liveMatches` interface.
// RollCourage rolls (or reads the typed dice of) a Courage test for a unit; it writes nothing, the player then records the result with Apply.
// ErrRefused: not an editor / unit not in the Field / RollRefusal; ErrNotFound.
RollCourage(ctx, user core.User, id, matchID, unitID int64, entered []int, thrown bool) (core.CourageRoll, error)
type CourageRoll struct { Dice []int; Total, Courage int; Passed bool }

// Also the match page's result form defaults (declared by the match page, implemented by LiveService):
MatchDefaults(ctx context.Context, id, matchID int64) (core.LiveMatchDefaults, bool, error) // false: no live data
type LiveMatchDefaults struct {
    Fled        map[int64]bool // company -> chose All is Lost (feeds CompanyResult.Fled)
    EnemyBroken map[int64]bool // company -> every opposing company is Broken at the end of the game (feeds the enemyBroken scenario objective)
}
```

Routes: `POST /campaigns/{id}/matches/{match}/live/turn`, `POST .../live/{company}/break` (adjust), `POST .../live/{company}/courage` (skip),
`POST .../live/{company}/allislost`, `POST .../live/{company}/units/{unit}/courage/roll` (the dice component), the rest through the existing `actions` route.

Built (36y.12, 36y.14, backend, `internal/core/live_break.go`): `FoldLiveWith(inits, entries, LiveFoldConfig)` and `ValidateLogWith` take what the log and the starting values do not hold: per company `BreakConfig{Percent, Extra, Exceptions}` (the percent is the company's own `breakPointPercentage` or `campaign.breakPoint.default.percent`; `Extra` is the company's creatures; the exceptions are the match scenario's `scenarioExceptions` as sentences: the entry's own `text`, falling back to wording from the effect codes when `text` is missing, bead `9gb`), `BreakModels` per unit (the unit's model count, 1 for a unit made of parts until `36y.18` is answered) and `NoStandFast` (non-Heroes, Wanderers, a Hero whose injuries took Stand Fast away). `FoldLive` is the same with no config and then nothing ever breaks. The service builds the config in `liveContext.breakConfig` from the rosters and the file at every read (nothing is stored; no migration beyond `00021`). `LiveState.Break(company) LiveBreak` (Percent, StartingModels, Casualties, `BreakPoint()` as "6.5", Broken, BrokenAtTurn, WillBreak, Offer, Fled, Exceptions) and `LiveState.Turn()`; `LiveCompany.Break` / `.Turn`; `LiveUnit.BreakModels`, `CourageTurn`, `CourageHow` (how that courage was resolved: `passed`, `failed` or the `courageSkip` reason), `StandFastTurn`; `CompanyDetail.BreakPointPercent` is the company's resolved percent (36y.20). Casualties are Field casualties plus units retreated with "Failed courage" (weighted by their model count) plus the manual adjustment; "Left the board" and a lost mount alone are not. The integer test `casualties*100 > percent*startingModels` runs only inside a `turn` entry; Broken is permanent; undoing the turn entry (`ValidateLogWith` + fold without it) un-breaks. `turn` is refused when stale ("Turn 3 has already started", `ErrRefused`) or not the next number (`ErrInvalidLive`); it is stored with company 0 and any owner of a company in the match may press and undo it (`CanUndoEntry`). `breakAdjust` (Field start|casualties, Delta -9..9, never below one starting model or below no casualties), `courageSkip` (Reason standFast|heroicResolve|chariot|scenario|engaged|cannotActivate; engaged and cannotActivate are exemptions), `broken` with `StandFast`, and `retreat` with Why "courage" all set the unit's `CourageTurn`; only `broken` earns the experience flag. `allIsLost` is refused unless the offer is open (the turn the company broke, no answer yet); Flee retreats every Field unit of the company as "Left the board" (not casualties). REVIEW: a `broken` or `courageSkip` is allowed when the company is not Broken, as the spec says. `LiveService.RollCourage(ctx, user, id, matchID, unitID, entered, thrown) (CourageRoll, error)` reads the dice (`campaign.breakPoint.whenBroken.courageTest.dice` must be `2d6` and `passIf` `totalAtLeastCourage`, otherwise an error), honours `CampaignSettings.RollRefusal` and writes nothing; the unit must be in the Field and have a Courage. `LiveService.MatchDefaults(ctx, id, matchID) (LiveMatchDefaults, bool, error)` (no user: the web layer gates) gives `Fled` and `EnemyBroken` (every company of every other side is Broken at the end), false without live data. Frontend contracts: 36y.15 reads `LiveCompany.Break/Turn` and posts `turn` (Turn = `Turn()+1`) and `breakAdjust`; 36y.16 shows "Test due" for a Broken company's Field units with `CourageTurn != Turn`, posts `broken` (with `StandFast` for a Hero that is not in `NoStandFast`: the page cannot see that set, so it offers the switch to Heroes and shows the refusal sentence), `courageSkip`, `retreat` (Why "courage"), `allIsLost`, and calls `RollCourage`; 36y.17 prefills `CompanyResult.Fled` and the `enemyBroken` objective from `MatchDefaults`.

### Aftermath and match feed

- `AftermathDefaults` already carries `brokenCourageTestPassed` from the flag; unchanged (one XP per model per match, only for rolled passes).
- `fledBroken` / All is Lost: the live `Fled` prefills `CompanyResult.Fled` in the result form with the badge "From the live match"; the aftermath
  then follows [aftermath.md](aftermath.md) (unchanged: participation plus the Against the Odds Influence and experience bonuses, no Win/Draw or scenario rewards; user decision mqn.114).
- `enemyBroken` Victory Point objectives default from `EnemyBroken` (state at the end of the game); the editor can change it.
- Company page and My Company show the company's Break Point rule ("Break Point 50% of the starting models", "66%" for Muster of Isengard) from the company effect.

### Acceptance criteria

- [ ] Starting models and casualties come from the log and the adjustment; fled-by-courage count, left-the-board and a lost mount alone do not; undo and Return to the battle restore both.
- [ ] Broken only flips on a `turn` entry with `casualties * 100 > percent * models` (13 models, 6 casualties: not Broken; 7: Broken; Muster of Isengard 66%: 9 of 13); permanent; undoing the turn un-breaks.
- [ ] A stale turn press is refused; a viewer cannot press (403); the banner, chips and All is Lost offer appear on every viewer's page live (SSE) and survive a server restart.
- [ ] A Broken company's cards show "Test due" until resolved this turn; Passed (rolled) earns the flag once; auto-pass and exempt earn nothing; Failed removes the unit as a `fled` casualty with no injury roll and raises the casualty count.
- [ ] Stand Fast hint appears for Warriors only after a Hero (not Broken Honour, not a Wanderer) called it; scenario exceptions are listed from the data.
- [ ] All is Lost: offered once in the turn the company broke, gone at the next turn; flee marks `Fled`, the result form and aftermath follow; Fight on closes it; undo reverses.
- [ ] 360 px, light and dark, tokens only, no horizontal scroll, `/styleguide` components; panel and chips readable without colour.
- [ ] Tests: fold tables (threshold edges 6.5/7, 66%, integer test, adjustments, undo, composite counts), `Apply` permission and stale-turn tests, `RollCourage` refusals, handler and template states (owner, viewer, Broken, offer, ended), integration test for the kinds and the restart.

### REVIEW

- B1 66% means 66% of the starting models; B2 All is Lost only in the turn the company breaks; B3 is overridden by the user decision mqn.114 (the fleeing company keeps its Against the Odds Influence and experience bonuses; nothing changes in the aftermath); B4 creatures count in the starting models; B6 escaped models are not casualties; B8 the temporary Wanderer counts; the turn button as the only
  evaluation point; Heroic Resolve does not touch the Might tracker; failed-courage and left-the-board are the existing Retreated reasons.
- Decided (user, 2026-10-22): the Mirkwood Warg Marauder counts as 3 models (bead `36y.18`, closed). Tame the Beast budget uses ratings before Against the Odds bonuses (`36y.19`).

## Against the Odds items in a live match (spec, designed; epic `36y`)

The match's Against the Odds record ([matches.md](matches.md)) is **overlaid at read time** on the live state (nothing is stored in the init rows, so a
selection made after the live state was first read still shows):

- **Loaned items** (Tool Kit): `LiveUnit.Loaned []LoanedItem{Item, Kit, Detail, Used bool}`. The unit card lists them under the equipment, tagged "Loaned (Healer's Kit)", with no
  Rating. The action menu gains **Item used** for consumables (`LiveItemUsed`, `UnitID`, `Item`, `Index`: Dwarven Brew, Torching Brands, Scroll of Hidden Paths...), which strikes the item through and
  logs "Brand used Dwarven Brew"; undo restores it. Healer's Kit items (Healing Herbs, Wondrous Cram) show "Use after the game" and no Item used button (they act in the aftermath); a Wondrous Cram or Healing Herbs on a
  roster card also gets the reminder (36y.1). The kit never changes stats (the data's `stillApplies`).
- **Temporary Wanderer**: a normal Hero card in the Battle company section, badge "Temporary" and Rating shown as 45; Stand Fast, experience flags and the Might/Will/Fate trackers behave as for a Hero, but the card has no
  Stand Fast option, never earns the experience flags in the aftermath feed, and is **not** in `AftermathDefaults`. It uses a synthetic unit id from the selection record (`LiveUnit.Temporary`, `UnitID` from `match_ato`); its owner is the lower company; it
  is a model for the Break Point (counted in the starting models, a casualty if it falls) and cannot carry loaned items.
- **Ambush!** has no live effect beyond the scenario already set.

```go
LiveItemUsed LiveKind = "itemUsed" // UnitID, Item, Index: a loaned or owned consumable used during the game
// LiveAction gets Item string, Index int. LiveUnit gets Loaned []LoanedItem, Temporary bool.
```

Built (36y.9, backend): `LiveService.WithATO(repo)` (wired in `cmd/server`) overlays the `match_ato` record at read time, in `liveContext.overlay`; nothing is stored in the init rows, so a selection saved after the live state was first read shows at once and one cleared before the game starts disappears. `LiveUnit.Loaned []LoanedItem` (UnitID, Item, Kit name, Detail, Index = the copy inside its kit, `AfterGame` for the items that activate an aftermath effect, `Used` filled by the fold), `LiveUnit.UsedItems` (owned items marked used), `LiveUnit.Temporary`, `LiveCompany.Wanderer *TemporaryWanderer{UnitID, Name, Summary, Rating (45), Courage}` for the company that took it. The temporary Wanderer is a normal unit of the fold with the synthetic id `core.TemporaryUnitID(company)` (the negative company id: it never meets a roster id), its trackers from `data.wanderers.items`, `CountsAsKill`, no Stand Fast (`LiveFoldConfig.NoStandFast`), counted in the Break Point starting models, named in the log, able to wound, to be wounded and to roll Courage (`RollCourage`); it cannot be voted MVP, is skipped by `AftermathDefaults` (no roll, no experience, no progression) and by nothing else. `LiveItemUsed{UnitID, Item, Index}` is checked in the fold: the unit must be in the Field and carry the item (loaned: by name and copy; owned: by name, from `LiveFoldConfig.Owned`), the item must not act in the aftermath ("Healing Herbs acts after the game: use it in the aftermath.") and must not have been used ("Brand has already used its Dwarven Brew"); undo is the fold without the entry. `AftermathDefaults`, `MVPHint`, `MatchDefaults` and `EndChecks` now fold with the overlay (a wound the Wanderer dealt counts). REVIEW: any owned item may be marked used (the data does not say which equipment is a consumable); an owned `Healing Herbs` or `Wondrous Cram` is refused like the loaned ones. Frontend (36y.10): show `Loaned` under the equipment with the kit tag, a button "Item used" for `Loaned` entries that are not `AfterGame` and not `Used`, "Use after the game" for `AfterGame` ones, the Wanderer card from `LiveCompany.Wanderer` plus the unit with `Temporary`.

Built (36y.10, 36y.15, 36y.16, 36y.17, web): see [frontend.md](frontend.md) "Live match" (Break Point, turns and courage; Against the Odds items; result form defaults).

Acceptance: the loaned items and the temporary Wanderer appear on the cards of all viewers; Item used is logged and undoable; the Wanderer is in the Break Point count and absent from the aftermath defaults; a selection saved before the first entry shows without reloading the match;
360 px, light and dark. Beads: backend `36y.9`, frontend `36y.10`.

## Ending a match while live (spec, designed; epic `6uh`)

The match ends only when every participant confirms ([matches.md](matches.md), "Ending a match: confirmation by all participants"). While live:

- **Banner.** When a proposal is pending, every company page of the match (owners and viewers, live via the `match` event) shows a sticky banner under the live banner:
  "Ending the match: Hador confirmed, waiting for Gorgoroth." The waiting owner gets **Confirm the end** and **Keep playing**; a confirmed owner **Withdraw**; viewers no buttons. On 360 px the banner is two lines, buttons below. The same card sits on the match page.
- **What is allowed while pending.** The end checks stay open: `objective`, `mvpVote` and the secret-objective entries are accepted and do **not** cancel the proposal (players do them at the end). **Any other
  action or undo** (stats, wounds, fate, retreat, died, mount, return, Item used, break adjustment, courage, All is Lost) **cancels the proposal** first (confirmations dropped, a log line "Ending cancelled: Aldor wounded Grashnak"), so the game is
  never played under confirmed end. The **Start turn** button is refused while pending ("Cancel the ending first"). REVIEW: auto-cancel instead of refusal, so nobody is locked out of a correction.
- **Log.** Informational entries `endProposed`, `endConfirmed`, `endWithdrawn`, `endCancelled`, `endOverride` appear in the log with the company colour and the author; they are not undoable and do not touch the fold. SSE: each change emits `match` (the pages refetch the banner), the final confirmation emits `ended`.
- **Break point and 36y.** Whether a company is Broken, `Fled` (All is Lost) and `EnemyBroken` are read from the fold at the moment the match ends; a Fled company still has to confirm the end like any other (its owner is a participant). The result form's prefilled values (Fled, enemyBroken) are a result change like any other: editing them resets the confirmations except the proposer's.
- **Aftermath.** Unchanged: it starts when the match ends; `AftermathDefaults` reads the final fold.
- **Absent owner**: the organiser or an administrator overrides (match page card); the live banner shows the override button to them too.

Built (6uh.3, backend): `LiveKind`s `endProposed`, `endConfirmed`, `endWithdrawn`, `endCancelled`, `endOverride` are informational (`LiveKind.Informational()`): written by `LiveService.RecordEnd` from `Matches.EndNotify`, in the colour of the company concerned (the author's company for a proposal), skipped by the fold and `ValidateLogWith`, refused when a player posts them, not undoable (`CanUndoEntry`); migration `00023` allows them in the log constraint. `LiveService.WithEnds(matches)` makes `Apply` and `Undo` cancel a pending end after any entry that is not an end check (`LiveKind.EndCheckKind()`: objective, mvpVote, secret entries); the line reads "Ending cancelled: <entry text>". `Apply` of `turn` while `Match.EndProposal != nil` is refused ("Cancel the ending first: the match is about to end."). Every proposal change is stored with the match and notifies the live channel with company 0, so the open pages refetch (`LiveBatch.Match`, all companies); the last confirmation emits `ended`. Frontend contract for `6uh.5`: read `Match.EndProposal` and `core.EndAccessFor` in the live view, show the banner for every viewer, post to the match routes of `6uh.4`, and disable Start turn with the reason while `EndProposal != nil`. Built (6uh.5, page): the banner is `#live-end` in the sticky live banner, Start turn is disabled while pending; details in [frontend.md](frontend.md). REVIEW: the cancellation happens after the game entry is stored, in a second transaction; if it fails the entry stays and the error is returned.

Acceptance: the banner appears for all viewers within about two seconds; objective and MVP entries during a pending end keep it; a wound declaration cancels it with a log line; Start turn is refused while pending; the last confirmation ends the match and replaces the trackers by the normal view; override works for the organiser and administrators only; 360 px, light and dark. Beads: `6uh.3` (backend), `6uh.5` (frontend).
