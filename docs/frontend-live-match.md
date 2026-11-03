# Frontend: live match screens

What the pages of an open match do: the company page with its banner, trackers and milestones, the End checks and the sync.
The rules of the mode are in [live-match.md](live-match.md) and the technical design in [architecture.md](architecture.md#live-match-design).
Code: `internal/web/live*.go`, `static/js/live.js`, the `live_*` templates in `campaign_my_company.gohtml`, `company_panels.gohtml`, `units.gohtml` and `match.gohtml`; `web` declares `liveMatches` (embedded in `campaigns`; `core.LiveService` implements it).
The design language and the page patterns are in [frontend.md](frontend.md).

## Contents

[Company page in an open match](#company-page-in-an-open-match) ·
[The opponent's keywords and "applies" markers](#the-opponents-keywords-and-applies-markers) ·
[Trackers](#trackers) ·
[Milestones](#milestones) ·
[Break Point, turns and courage](#break-point-turns-and-courage) ·
[Against the Odds items](#against-the-odds-items) ·
[Result form](#result-form) ·
[Mount lost](#mount-lost) ·
[Match log](#match-log) ·
[End checks](#end-checks) ·
[Ending a match by confirmation](#ending-a-match-by-confirmation) ·
[Sync](#sync) ·
[Secret objectives](#secret-objectives) ·
[Perks in the rolls](#perks-in-the-rolls) ·
[Gear notes on the card](#gear-notes-on-the-card) ·
[Finished campaigns are frozen](#finished-campaigns-are-frozen) ·
[Match creation refusals](#match-creation-refusals) ·
[To Wound chart (window)](#to-wound-chart-window)

## Company page in an open match

A company in an open match (`CompanyDetail.OpenMatch`; `liveFor` loads its `Live` state) gets the usual company page with:
- `#live-banner` (sticky under the app bar): "Match N is running" with the scenario, the sync dot, **To wound** (opens the To Wound chart window, below), **End checks** (everybody) and, for who may edit, **Match log**;
- the roster locked like read only (no recruit, buy chips, heal, level-up, drop, dismiss; rename stays) with the line "The company is
  locked until the match ends"; "Change formation" is hidden while live (the grid is re-rendered by the sync);
- `#live-zones` (`live_zones`): the Field as unit cards, then **Retreated**, **Field casualties** and **Missed this game** as
  one-line headers with a count ("None" when empty) and compact cards (`live_compact`: badge, reason, **Return to the battle** for
  who may edit), then the Cemetery. `GET /campaigns/{id}/companies/{company}/live` answers `#live-zones` alone (its `data-seq` is the
  match's last sequence). Every control that answers the zones (trackers, milestones, the turn, the Break Point, the courage and item
  windows) takes the `hx_zones` attributes: it replaces `#live-zones`, and `hx-sync` queues the presses one at a time.
- Another player's company is the same page read only (`ReadOnly`); an editor who is not the owner (organiser, administrator) has the
  trackers and actions on it. Viewers see `current / start` without buttons.

## The opponent's keywords and "applies" markers

Code: `kw_applies`, `keyword_flat` in `partials/units.gohtml`; [unit-keywords.md](unit-keywords.md) "Rule targets".

The Field cards, the compact cards and the rows of "Wounded a unit" of an enemy company show the unit's keyword chips:

- Compact cards use the normal `keyword_row`.
- The wound sheet's rows sit inside a label, so they get a flat list without tips: `keyword_flat`, `data-keywords-flat`.

**The "applies" marker.**

- An enemy Field unit that has a keyword named by an enemy-keyword rule of **your side's** units in the Field gets a `.kw-applies` marker, one per rule: "Hatred (Orc) applies".
- The rules that count are Hatred (X), Ancient Enemies (X) and the named rules the table marks `enemy-keyword`; not `enemy-any` rules such as Terror.
- The marker is an accent outline plus the text. It is `has-tip` (`tabindex="0"`, tooltip "Your Aldor and Hama have Hatred (Orc). This unit has the Orc keyword, so the rule works against it."), and Enter opens the helper popup for the rule.
- Markers sit in a `ul[data-rule-matches]` under the keyword row. The sheet's rows show the same text in a plain span.
- The reverse (the enemy's rules against you) is not shown.

**How it is computed.**

- `liveFor` computes it (`enemyMatches`: `core.RuleMatches` over the Field units of the viewer's side and of the company on the page, each read through `Live` and `CompanyDetail`).
- The refresh of `#live-zones` redraws the markers, so a unit that retreats or falls loses its marker.
- A viewer with no company in the match, and the viewer's own side, get none.
- Without `data.ruleTargets` no rule has targets and nothing is marked.
- Read errors of the extra companies drop the markers, never the page.

## Trackers

Code: `live_trackers`, `.tracker*` and `.seg-hit` in `tailwind.css`.
Trackers replace the static W, Might, Will and Fate values in the attribute row.

Each tracker is a segmented bar like the experience one:

- It is led by the stat's full name (Might, Will, Fate, Wounds).
- It has one segment per point of the start value, full up to the current one, then `current/start`.
- Wounds uses hearts instead of segments, inside the same 16 px row.
- Above 10 segments a tracker falls back to the number between 28 px `-` and `+` buttons.

Layout:

- One stat per row, left-aligned so the bars line up, with the counts in a column after the longest bar (the rows are subgrids of one grid).
- The order is Might, Will and Fate (heroes only), then Wounds.
- Wounds is always shown, also when it starts at 1 (a plain warrior).
  Taking it to 0 makes the unit a Field casualty, as Died does.

Colour: each stat has its own colour on its bold name and its bar, from the theme tokens of the path kinds (`.path-k-*`):

- gold Might (command);
- hero-blue Will (magic);
- success-green Fate (ranged);
- danger-red Wounds (melee, as the Soldier).

An empty segment or heart is a faint tint of that colour with a thin outline that does not change its size.

Pressing a segment (22 x 16 px) is the owner's way to change a value:

- the last full segment is `delta -1` ("... down by one");
- the first empty segment is `delta +1` ("... up by one");
- any other segment is `set` to that segment ("Set ... to N").

A viewer gets the same bars as marks, without buttons.

Each press is one `POST .../matches/{match}/live/{company}/actions` (`kind=stat`, `unit`, `stat`, `delta` or `set`), answered by `live_response`:

- the zones plus an out-of-band **toast** (`#live-toast`) with the log text and **Undo**;
- or, on a refusal, 422 with the state as it is and the reason as the toast.

A viewer's write is 403. Live POSTs are not wrapped in `once()`.

## Milestones

Code: `liveView.Milestones` in `live_milestones.go` builds them, `live_actions_menu` and `milestone` in `company_panels.gohtml` draw them, `.milestone*` in `tailwind.css`.

**The row on each field card.**

- It has Wounded, Fate, Courage and Objective (`aria-pressed`): off is dashed and muted, on is filled with the accent and a tick.
- Editors get buttons; viewers get marks (`role="img"`).
- **Wounded** opens its sheet, whose `live_recorded` block lists the unit's wound entries with Undo (`UnitLog`: the unit's not-undone entries of one kind, filtered in the query).
- **Fate**, **Courage** and **Objective** post to the actions route at once: `fate` with `count=0` (only the mark), `broken`, `objective`.
- A lit Fate or Courage posts the undo of `LiveUnit.FateEntry` / `BrokenEntry`.

**Actions below the row.**

- `.live-actions` (editors only, icon above the label) appears only while the unit has a loaned item to spend; **Item used** opens its sheet.
- The route also serves the list of every action (`sheet/menu`, which no card opens): Wounded a unit, Passed a fate roll, Passed a Broken courage roll, Retreated, Died, Mount removed/re-added. Fate and Mount appear only when the unit has them.
- Each action opens its sheet (`GET .../live/{company}/units/{unit}/sheet/{kind}`, `live_sheet`).

**Wounded a unit.**

- It lists the enemies in the Field, then the same side after a "Friendly fire" divider.
- It has a search, Rider/Mount for a mounted target, and wounds caused and saved by Fate steppers (`woundSheet` in `live.js`).
- A preview sentence uses the words of the log.

## Break Point, turns and courage

Code: `live_break` in `campaign_my_company.gohtml`, inside `#live-zones` so the SSE (server-sent events) refetch refreshes viewers too.
The rules are in [live-match.md](live-match.md#company-break-point-in-play).

**Turn row.**
It reads "Turn 3" or "Turn not started".
Editors get **Start turn N** (`kind=turn`, `turn=N`).
A stale press is the service's 422 toast.

**Break Point panel.**
It is a `<details>` element.

- The summary reads "Break Point 6.5 of 13 · Casualties 4 · 50%", with a "Will break next turn" or "Broken since turn N" badge and a progressbar.
- Opened, it lists the casualties by name and the scenario exceptions.
- Editors also get +/- adjusters for starting models and casualties, with a shared note (`kind=breakAdjust`).

**Company broken banner** (`#live-broken`).
It is a danger alert and is not sticky, because a sticky banner would cover the live banner.

- It holds the All is Lost choice as an inline `<details>`: **Flee the battlefield** (with a confirmation) or **Fight on**, for editors, while `Break.Offer` is set.
- It also shows "Who has tested this turn".

**Cards of a Broken company** show "Test due" until the unit's courage is resolved this turn (`CourageDue`: `CourageTurn != Turn`).
Then they show a chip (`liveView.CourageChip`, from `LiveUnit.CourageHow`):

- "Tested: passed" or "Tested: failed";
- "Auto-pass" for `standFast`, `heroicResolve`, `chariot` and `scenario`;
- "Exempt" for `engaged` and `cannotActivate`;
- otherwise "Courage resolved".

**Company header.**
On the company's own page and on another company's read-only page it shows "Break Point: N% of the starting models".
The percentage is `CompanyDetail.BreakPointPercent`: the company's own rule (for example 66 for Muster of Isengard), else the file's default, and never 0 with real rules.

**Courage test button.**
A primary **Courage test** button in the header opens the `courage` sheet (`live_courage`):

1. **Roll 2D6** (`POST .../units/{unit}/courage/roll`, `RollCourage`).
   Nothing is written. Typed dice are accepted only with ManualRolls. It is a plain server roll, not the animated dice component.
2. **Record: passed** (`broken`, with Call Stand Fast for Heroes that are not Wanderers) or **Record: failed** (`retreat` with reason `courage`, after a confirmation).
3. The six pass-or-exempt buttons (`courageSkip`).

All writes go through the one `actions` route (`liveActionFrom` reads `turn`, `field` + `delta`, `reason`, `flee`, `standFast`, `item` + `index`).

## Against the Odds items

- `LiveUnit.Loaned` is listed under the Wargear of the card, grouped by item ("Climbing ropes ×5", tag "Loaned (Raider's Kit)").
  "use after the game" marks Healing Herbs and Wondrous Cram. A spent item is struck through and marked "used".
- Owned items in `UsedItems` are struck through.
- The action menu has **Item used** (the `item` sheet lists the loaned items still to spend, `kind=itemUsed`) when something is left.
- `LiveCompany.Wanderer` makes a card for the temporary Wanderer (`temporaryUnit`).
  It is a roster unit with a synthetic negative id, badges "Wanderer" and "Temporary", the profile summary, Rating as its points and no Dismiss.
  It sits in the zone its live unit is in, and is a wound target in the wound sheet.

## Result form

For a company whose result was not reported yet, `prefillLiveDefaults` ticks **Fled** (All is Lost) and the `enemyBroken` objective from `MatchDefaults`, each with the badge "From the live match", and the Fled switch explains "The other side claims victory (All is Lost). Saving this does not end the match."

## Mount lost

A unit that lost its mount gets an "On foot" badge.
These are struck through (`itemView.Struck`, `ruleView.Struck`, `aria-label` "needs a mount"):

- the mount item;
- the gear with `Equipment.RequiresMount`;
- the rules the mount gave;
- the Cavalry rule.

The data flag `requiresMount` comes from the Battle Companies file.

## Match log

`GET .../matches/{match}/live/log` is the sheet (company page; `?company=` so Undo refreshes the zones out of band), `?card=1` the card of
the match page (below History, drawn inline in the full page, refreshed on `live:match`), `?before=<seq>` the next 30 older rows. Newest first,
grouped by minute, Undo only where the service says `CanUndo` and the match is open; a dependency refusal shows in the log. After the
match the card stays (read only, permanent); an ended match recorded after the fact says it has no log.

## End checks

Code: `live_end.go`, `end_*` templates in `match.gohtml`.

**Where it shows.**

- The sheet opens from the banner button (`GET .../matches/{match}/live/end?company=`).
- The card on the match page (`?card=1`, `#end-checks-slot`) is refreshed on `live:match` and empty once the match ended.
- Both show `core.EndChecks`.

**What it holds.**

- Per own company, a switch per unit "Got the objective" (`POST .../live/{company}/objective`, `unit`, `on`).
- Per opposing company, a radio list "Select MVP" with "No MVP" (`POST .../live/{company}/mvp`, `unit`; 0 withdraws) and the vote tally. A tie says the owner picks in the aftermath ([aftermath.md](aftermath.md)).

**Answers.**

- Every write answers the body again with a message and Undo (`from=end|endcard` of the shared undo route).
- A refusal is 422; a user with nothing to set gets 403.

**Propose to end** is a button that fetches `.../live/end/reminder?revision=`: what is still open, with **Go back** and **Propose anyway**. It never blocks (`POST .../end/propose`).

## Ending a match by confirmation

Code: `match.go` `handleEnd`, `live_end_banner.go`; templates `end_card`, `end_propose` in `match.gohtml`, `end_override` in `partials/end.gohtml`, `end_banner` in `campaign_my_company.gohtml`. The rules are in [matches.md](matches.md).

**Routes.**

- `POST .../matches/{match}/end/propose|confirm|withdraw|cancel|override`; override also takes `note`, trimmed, 1-200.
- All take `revision`; a refusal is 422 and a stale revision answers the current state.
- `core.EndAccessFor` (via `endAccess`) hides the controls; the service enforces.

**Match page.**

- The header button is hidden while an end is pending.
- The **End card** (`#end-card-slot`, refreshed on `live:match` through `GET .../end/card`, which also sends the header button out of band) lists one row per company: **Confirmed** with who and when, or **Waiting**.
- It shows the reset notice and the buttons: **Confirm the end** (owner of a waiting company), **Withdraw my confirmation**, **Keep playing** (any participant, organiser, admin).
- For the organiser or an administrator, while somebody waits, a quiet **End without confirmation...** opens the warning and the note field; the dialog stays open with the error on a missing note.
- Viewers see rows without buttons.
- The history line of an ended match carries `MatchEvent.Detail`.

**Company page.**

- `#live-end` inside the sticky `#live-banner` ("Ending: waiting for Gorgoroth", same actions, override included) is fetched on load and on `live:match` (`GET .../live/end/banner?company=`).
- Its forms post with `view=banner&company=` and get the banner back, or `HX-Refresh` when the match ended.
- **Start turn** is disabled with "Cancel the ending first: the match is about to end." while `liveView.EndPending`.
- The log lines ("Ending cancelled: ...") come from the backend and show in the normal log.

**Elsewhere.**

- On the aftermath sheet the prefilled objective shows "From the live match" and the MVP "Voted MVP".
- The two log/end slots set `hx-target="this"`: the body's inherited `hx-target="#main"` would swap them over the page.

## Sync

Code: `live.js`.

- One `EventSource` on `/campaigns/{id}/matches/{match}/events` per page that has a live part (`#live-zones` or `[data-live-match]`), none elsewhere.
- An event of the page's company with a `seq` above the zones' `data-seq` refetches the zones; the author's own change is ignored.
- `ended` reloads the page.
- The stream is closed while the tab is hidden and reopened, with a refetch, when shown.
- The dot is green "Live", or pulsing grey "Reconnecting": the browser retries every 3 s, and when the stream is back the page refetches what it missed (the state is in Postgres).
- When the stream is refused (caps) or unsupported, the page polls every 10 s.

## Secret objectives

Code: `secret.go`, `secret_card` in `match.gohtml`; only for a scenario with `ScoringObjectives[].Secret`, i.e. Fog of War.

The card
(`GET .../matches/{match}/secret`, drawn inline in the full page; the endpoint serves the refresh and the fallback) lists, for each company the viewer owns, each secret objective with a nomination
form (a select of own or enemy Heroes without the Leader unless he is the only Hero, or a text box for the terrain piece) and a tier radio
list ("Not reached yet" clears); `POST .../secret/{company}` (`kind=nominate|done`, `key`, `unit`/`note`/`tier`) applies the live action, with an
Undo (`from=secret`). While the match is open nothing of other players is shown but "Noted already: <companies>" (privacy is the service's);
after End match the card lists everybody's. On the result form of an ended match the Victory Points of a side with none reported are prefilled
from `SecretScore` with the badge "From the live match" (editable, saved only with Save result).

## Perks in the rolls

The recruit roll form shows **Rendezvous** (Not now / +1 / -1, field `perk`, `RollRecruitWithPerk`) while the company has an unused
Rendezvous (`core.UnusedPerk(detail.Perks, ...)`; the free roll on the special chart does not offer it); after a scenario roll ("Roll for it") the
panel offers **Vantage Point** +1 / -1 on the second die to an owner of a company in the match with an unused one (`POST .../scenario/roll` with
`vantage`=company, `adjust`; the dice posted are the ones on show, so the typed-roll setting does not apply; `UseVantagePoint` returns the moved die and the line shown).
The recruit window shows the moved die as "4, Rendezvous +1 = 5" (`RecruitRoll.PerkRaw/PerkAdjust`).

## Gear notes on the card

A `StatMod` with `Delta` 0 and a `Note` (the Small Shield's "+1 Defence against strikes only") prints as a muted line under the
attributes (`data-stat-note`), besides the characteristic's tooltip, so it shows on a phone too.

## Finished campaigns are frozen

My company is read only with the notice "frozen as it was" (`myCompanyView.Frozen`, badge "Frozen"; no recruit window),
an open match says it stays open but cannot change, and shows no End checks, Roles or secret objectives cards. The organiser (not only administrators) sees
"Edit the result" and "Reopen match" on an ended match (`CanManageCampaign`).

## Match creation refusals

Shows "Company X is already in Match N." with a link (`*core.CompanyBusyError`) or "Finish the aftermath of Match N
first."; the aftermath sheet shows **From the live match** (heading badge and a line) while it is the unsaved default
(`Aftermath.FromLive`).

## To Wound chart (window)

`partials/to_wound.gohtml` (`to_wound_button`, `to_wound_dialog`, `to_wound_body`), `web/towound.go`, `static/js/towound.js`, the `tw-*` classes in `tailwind.css`; the
behaviour and the reasons are in [matches.md](matches.md#to-wound-chart).

- The button (`data-to-wound-open`) is on the match page (next to the heading, open matches) and in the live banner. `towound.js` turns its click into a window
  event (`open-to-wound`), as `dialog.js` does for `data-open-modal`: no inline handler. The window (`#to-wound-dialog`, `role="dialog" aria-modal="true"`, labelled
  by its title, the pattern of [Windows](frontend.md#windows): `dialog.js` moves the focus in and back and traps Tab, Escape, the backdrop and Close shut it, the body scroll
  is locked) is rendered once per page next to, never inside, `#match`, `#live-zones` and the sticky banner (a sticky element is a stacking context that would put the
  window under the app bar).
- Its body is fetched by htmx when the page loads (`hx-trigger="load"`) and kept; the Alpine component `toWound` keeps Strength and Defence in the browser and reads
  the cells' `data-s`, `data-d`, `data-big`, `data-tier` and `data-meaning`.
- Cells are colour bands from the semantic tokens (`success-soft`, a tint of `success`, `warn`, `danger`, `raised`); the contrast of every band's text is checked in
  `93-to-wound` (4.5:1, light and dark), and the roll is always printed, so colour is not the only cue.


See also: [live-match.md](live-match.md) (the rules of the mode), [matches.md](matches.md) (side roles, ending a match, the To Wound chart), [aftermath.md](aftermath.md) (the aftermath sheet and its MVP hint), [unit-keywords.md](unit-keywords.md), [architecture.md](architecture.md#live-match-design), [frontend-company.md](frontend-company.md), [frontend.md](frontend.md).
