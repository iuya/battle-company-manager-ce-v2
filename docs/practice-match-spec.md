# Practice matches (spec)

Status: **implemented** (steps 1 to 5 of section 13: migration 00025, `NewMatch`/`Match.Practice`, the derived aftermath and its refusals, the counts, the form, badge, banner, filter and pages, the demo world and the e2e flow `99-practice-match`). The feature: a match flagged as **practice** when it is created awards no rewards (experience, influence, any scenario bonus) and
skips injuries. The decisions taken are in section 14. Related: [matches.md](matches.md), [live-match.md](live-match.md),
[aftermath.md](aftermath.md), [multi-company-spec.md](multi-company-spec.md).

Why a player wants it: a friendly game, a test of a new build, teaching a newcomer, or two companies of one player (multi-company, "Same
player" fights) that must not feed the roster or the cemetery. A practice match is **not a separate flow**: it is created, played live,
confirmed and ended exactly like any match. The only difference is that its aftermath is already complete and empty, so nothing is rolled,
awarded or locked.

## 1. Not implemented today

Nothing in `internal/`, `cmd/` or the migrations knows about a practice match. The facts that make the feature a cross-cutting change:

| Place | Today | Consequence for a practice match |
|---|---|---|
| `core.Match` (`campaign.go`), `matches` table (migrations 00008, 00022, 00023) | no flag; `NewSide` list is the only creation input (`Matches.CreateMatch(ctx, user, id, sides)`, `matchRepository.Create`) | new field, column, creation input |
| `Matches.endMatch`, `docs/matches.md` | ending a match always opens one aftermath per company | none must open |
| `postgres/aftermath.go` `pendingRefs`, `CompanyAftermaths` (the lock), `readRecord`: a company of an ended match without a row counts as `pending` (`COALESCE(a.status,'pending')`); `AftermathStatus.Locks()` is true for everything but `done` and `waived` | an ended practice match would lock its companies | its aftermath must read as `done` and empty (section 3); the lock queries then need no special case |
| `postgres/campaign.go` lines 44-57 | W/D/L are counted from every ended match | practice excluded (decision 3) |
| `CampaignSummary.Matches`, `matches x` count in `postgres/campaign.go`, `OpenMatches` | every match counts | practice listed apart (see 5) |
| `AftermathService` (`sheet`, `FinishAftermath`, `applyInjuries`, `applyExperience`), routes `/matches/{match}/aftermath/{company}...` | reachable for any ended match | the page shows "Practice match: nothing to roll"; every write is refused |
| Against the Odds (`ato_service.go`, table `match_ato`) | bonuses `influence`, `experience`, `reroll` apply in the aftermath; kits, Wanderer, Ambush! act at the table | reward bonuses have no effect; table effects stay (decision 2) |
| `GrantReward` (organiser grants) | independent of matches | unchanged, still allowed |

## 2. Rules answers

- The *Battle Companies* rulebook has no practice mode; this is a house rule of the app. Nothing in `definitions/` changes: the aftermath
  rules data is simply not applied.
- Everything the aftermath would add comes from `campaign.matchInput.aftermath.order` (injuries, experience, progression, influence) and the
  scenario `rewards[]`; a practice match applies none of these. A model that took a casualty in the game does not roll on the injury chart;
  a model that was Injured before the game is not healed by it either (the "missed this game, recovers" step is part of the aftermath), so a
  practice match changes the roster in no way and does not advance any recovery.
- Wounds, Might, Will and Fate spent in live mode are match state only (derived from the log); they never reach the company, so nothing
  needs undoing.

## 3. The flag, and how the aftermath is already complete

**Set once, on the creation form** (`match_new.gohtml`, `match_form.js`): a single checkbox "Practice match" under the sides, off by default, helper
text "No experience, influence, rewards or injuries. The result is only recorded." The form posts `practice=1`. Any player who may create a match
today (a member passing `CanStartMatch`) may tick it; no extra permission. **Nobody can change the flag afterwards**: a mistaken choice is fixed by
ending the match and creating a new one. There is no `SetPractice`, no event, no menu action.

**The aftermath of a practice match is complete and empty from the moment the match ends.** Chosen design: *derived, nothing is written*. The
aftermath state machine (`AftermathStatus`: pending, sheet, injuries, levelups, done, waived; `Locks()`; the absence of a row means `pending`)
already supports "done with an empty record", so the smallest change is in the repository read, not in the flow:

- `postgres` reads a company's aftermath record for a practice match as `Status = done`, `Revision 1`, empty `Input`/`State`, one line "Practice match:
  nothing to roll." and `FinishedAt = EndedOn`, whether or not a `match_aftermaths` row exists (none is ever written). The list and lock queries
  (`pendingRefs`, `CompanyAftermaths`, `PendingAftermaths`, the statuses at `aftermath.go:193`) use the expression
  `CASE WHEN m.practice THEN 'done' ELSE COALESCE(a.status, 'pending') END`, a one-line change in each; **no `AND NOT m.practice` exclusion is
  needed** because nothing is pending, so the lock, the "cannot start a new match" check and the organiser's `PendingAftermaths` figure behave
  correctly by themselves.
- `AftermathService.Aftermath` returns that record (with `MatchEnded`), so the page renders the normal read-only "done" aftermath view with the
  short state "Practice match: nothing to roll" (section 5). Every write (`SaveAftermath`, `StartAftermath`, `RollCasualty`, `RerollCasualty`,
  `Decide`, `FinishAftermath`, `ReopenSheet`, `WaiveAftermath`) answers `ErrRefused` "Practice match: nothing to roll." (`WaiveAftermath` too: nothing to waive).
- Nothing reaches the company: no `companyRepository.Update` is called, so roster, resources, `Injured` flags, cemetery, experience and influence are
  untouched. `LiveMatches.AftermathDefaults` is never built.
- `ReopenMatch` keeps working on a practice match (no aftermath has "started"), so an administrator can still correct a result; ending again
  yields the same empty complete aftermath. The rule "reopen is refused once an aftermath started" ignores a practice match.

Considered and rejected: writing a `done` row for every company at end time. It needs a write in each of the end paths (confirmation, override,
administrator edit), a failure window between ending and writing, and it would make `ReopenMatch` refuse (a done row counts as started). The derived
read gives the same observable result (the same status, the same unlock, the same page) with one query expression.

**While the match is in progress** a practice match is a real match for locking and display: its companies are in the open match (`CompanyBusyError`
if they try to join another), the company page shows the same live "match active" state, trackers and counters as always, the live log and
secret objectives work, and ending needs the same confirmations. The only extra on those pages is the Practice badge and banner (section 5).

## 4. What a practice match records and what it must not do

Records (unchanged): scenario (rulebook or one-off), sides and roles, result (winner or draw, Victory Points, Fled, objectives completed, note),
who played, the live log (trackers, wounds, kills, zones, MVP votes), end confirmation and history, the Against the Odds card selection, the
secret objectives. The match page, the live pages and the log stay available forever, also after the campaign finishes.

Must NOT, for any company of the match:

- run an aftermath: no sheet, injury rolls, casualties, cemetery entry, promotion or succession; no row in `match_aftermaths`;
- award or spend experience, influence, gold, rerolls, nor apply Against the Odds `influence`/`experience`/`reroll` bonuses or scenario `rewards[]`
  (including `possessesRelic` and objective bonuses); no rating change (the rating is roster based, and the roster does not change);
- consume items (Healing Herbs, Wondrous Cram, kit items): usable at the table, never spent;
- count in Wins, Draws, Losses or in the "matches played" figure (decision 3); the Overview shows a separate "n practice matches" line;
- count toward a campaign finish: the finish is manual (`SetStatus`), nothing reads match counts.

It still: blocks a company from being in two open matches at once, refuses creation when an officer or path is pending, appears in "Open matches",
and the organiser's `GrantReward` stays independent of matches.

## 5. Screens (mobile first, 360 px up)

A practice match is marked **Practice** wherever a match appears, with one component: a pill badge "Practice" in the neutral tone (token family
`bg-surface-muted`/`text-muted`, never the win/loss/accent tones, so it is never read as a result), icon plus text, text min 12 px; 44 px tap targets
stay on the row, not the badge. It is added to `/styleguide`.

| Page | Change |
|---|---|
| New match | the checkbox above; when ticked, a line under it repeats what is skipped. With the multi-company "Same player" note, offer "Make it a practice match" as a link that ticks the box |
| Campaign Matches list (`campaign_matches.gohtml`) | the badge after the match number; a filter chip "Practice" beside the state filters (default list shows all); ended practice rows show the result and no aftermath chip |
| Campaign Overview | "Matches played" excludes practice; a muted line "n practice matches" when n > 0; open practice matches count in "Open matches" |
| Match page header | the badge beside "Match N" and a one-line banner "Practice match: no experience, influence, rewards or injuries." (open and ended). No header action to change it |
| Match page, aftermath block | each company shows "Aftermath: nothing to roll (practice match)" with no call to action and no lock notice |
| Aftermath page of a company (`GET .../aftermath/{company}`) | the normal read-only "done" view reduced to a short state: badge, "Practice match: nothing to roll.", a link back to the match and one line saying that nothing was awarded and nobody was injured; no sheet, no rolls, no waive button |
| Result form | unchanged; the helper under "Objectives" says "Recorded only; no reward in a practice match" |
| Live match (company page) | identical to a real match (same "match active" state and counters) plus the badge in the match bar; "End match" confirmation says "No aftermath will follow"; end checks (objective, MVP votes) stay; the aftermath feed step is skipped |
| Company page, match history | the badge and no aftermath chip |
| Company switcher dots (multi-company) | no dot, nothing is pending |
| Rankings, Companies | W/D/L exclude practice |
| Viewers, finished campaigns | same badge and banner, no actions |

States: empty (no practice match: no line, the filter chip shows no count), permission (viewers see everything read only). Mobile 360 px: the badge sits on
the title line and wraps below the number if it does not fit; the banner is a full-width wrapping block; no horizontal scroll; light and dark with
semantic tokens only.

## 6. Against the Odds in a practice match

The ATO card stays: kits, the Temporary Wanderer and Ambush! are table effects. Any effect that only applies after the match (`influence`,
`experience`, `reroll`) does not apply and is shown disabled with "No reward in a practice match." Kit items are never consumed (section 4). The
saved selection is kept as entered.

## 7. Persistence

Migration `00025_practice_matches.sql` (the number follows `00024_several_companies`; adjust to the next free one):

```
-- +goose Up
ALTER TABLE matches ADD COLUMN practice BOOLEAN NOT NULL DEFAULT false;
-- +goose Down
ALTER TABLE matches DROP COLUMN practice;
```

- Existing matches read `false`; no data migration; the flag is never updated after the insert, so there is no revision interplay and no event kind.
- `matchColumns`/`matchJSON` in `postgres/match.go` select and scan `m.practice`; `ms.Create` inserts it.
- The aftermath queries use the derived status expression of section 3 (`pendingRefs`, `CompanyAftermaths`, `PendingAftermaths` at `postgres/campaign.go:189`,
  the listing at `aftermath.go:193`, and `readRecord`/the read at `aftermath.go:120`). The W/D/L subqueries at `postgres/campaign.go:53-57` add
  `AND NOT m.practice`; the matches-played figure at `:94`/`:186` counts `ended AND NOT practice`, and a new `practice` count feeds the Overview line.
- The number of queries stays constant (`cmd/server/querycount_test.go` guards the home list and campaign pages): the new figure is one more column of an
  existing aggregate.

## 8. Core contract

```go
// Match gains (set at creation, never changed):
Practice bool // no rewards and no injuries: its aftermath is complete and empty from the moment the match ends

// NewMatch replaces the []NewSide argument of CreateMatch so the creation input can grow:
type NewMatch struct {
    Sides    []NewSide
    Practice bool
}
func (s *Matches) CreateMatch(ctx context.Context, user User, id int64, in NewMatch) (Match, error) // same errors as today
// matchRepository.Create(ctx, campaignID, by, in NewMatch) and the web interface follow; ValidateSides keeps its []NewSide signature.

// AftermathRecord for a practice match (built by the repository, never stored): Status AftermathDone, MatchEnded true, empty Input and State,
// one line "Practice match: nothing to roll.", FinishedAt = the match's EndedOn. AftermathStatus.Locks() is false for it, so CompanyAftermaths,
// PendingAftermaths, AftermathStatuses and the "cannot start a match" check need no special case.
//
// AftermathService: Aftermath returns that record; every write method (SaveAftermath, StartAftermath, RollCasualty, RerollCasualty, Decide,
// FinishAftermath, ReopenSheet, WaiveAftermath) returns ErrRefused "Practice match: nothing to roll." before reading rules or touching the company.
// AftermathView/Aftermath gains `Practice bool` so the page can render the short state.
// The ATO service marks the reward bonuses Disabled with Reason "No reward in a practice match" and the aftermath ignores them.
// LiveMatches.AftermathDefaults is never called for a practice match. ReopenMatch is unchanged and always allowed for an administrator.

// CampaignSummary and CampaignDetail: Matches counts played non-practice matches; both gain
PracticeMatches int
// CampaignCompany.Wins, Draws, Losses exclude practice matches.
```

## 9. Interaction with multi-company

- A practice match is the natural way to play two companies of one player against each other or to test a build; the "Same player" note on the setup
  offers the link that ticks the box. A competitive same-player match remains allowed ([multi-company-spec.md](multi-company-spec.md), decision 2).
- The lock, the switcher dot and "cannot start a new match until the aftermath is finished" are unaffected by an ended practice match; during it the
  companies are locked like in any open match, one open match per company.
- Rankings with several companies per owner stay per company; practice matches never move them.

## 10. Acceptance criteria

- [ ] The new-match form has the "Practice match" checkbox (off by default); `POST /campaigns/{id}/matches` accepts `practice=1`; the created match has
  `Practice == true`, lists with the badge, and there is no way to change the flag afterwards (no route, no button).
- [ ] While open, a practice match behaves like a real match: the companies cannot join another match, their pages show the same "match active"
  state with the counters, live tracking and log work, ending needs the same confirmations.
- [ ] After it ends, each company's aftermath reads as done and empty: no lock, the company can recruit, buy, heal and open a new match at once; the
  aftermath page shows "Practice match: nothing to roll." (no 404, no sheet, no rolls); every aftermath write is refused with that message.
- [ ] No experience, influence, gold, rerolls, scenario reward, Against the Odds reward, injury, casualty, cemetery entry or promotion results; the
  roster, `Injured` flags and resources of every company are identical before and after.
- [ ] Wins, Draws, Losses and "matches played" exclude practice matches; the Overview shows "n practice matches" and the Matches list has a "Practice" filter.
- [ ] The badge appears on the Matches list, Overview, match page, live pages and the company history; the banner on the match page and the live bar;
  360/768/1280 px, light and dark, no horizontal scroll, text plus icon.
- [ ] Against the Odds: reward bonuses show disabled with the reason; table effects still selectable; nothing is consumed.
- [ ] An administrator can reopen an ended practice match, fix the result and end it again with the same outcome.
- [ ] Existing matches (`practice = false`) behave exactly as before; `make test`, `make test-integration`, `make e2e` pass; `/styleguide` has the badge.

## 11. Test and e2e plan

Core (mocks regenerated with `make generate`): `CreateMatch` with `Practice`; the aftermath service for a practice match: `Aftermath` returns the done empty
record, each write refuses with the message and never calls `companyRepository.Update`; `Locks()` false so `CompanyAftermaths` is empty; ATO reward bonuses
disabled and ignored, table effects kept; `Wins/Draws/Losses` and `Matches` exclude practice; `ReopenMatch` allowed on a practice match.
Postgres (integration): migration up/down; `Create` stores the flag; the derived status in `pendingRefs`, `CompanyAftermaths`, `PendingAftermaths`, the
statuses listing and `readRecord` for a practice and a real match side by side (real unchanged, practice done); no `match_aftermaths` row is ever
written; the W/D/L and played queries exclude practice; query count unchanged.
Web: handler tests (form value; aftermath GET renders the short state; aftermath POST routes answer the refusal), template state tests for badge,
banner, filter, Overview line, disabled ATO options and the aftermath block text; no-horizontal-scroll checks at 360 px.
E2E (`tests/e2e/flows`, new flow `98-practice-match.mjs`; renumber at implementation): create a practice match between two companies, check both company pages
show the match-active state and the creation of another match is refused for them, set a scenario, enter a result, end with confirmations, see the banner
and "nothing to roll" on the aftermath link and page, check the company can recruit and open a new match at once, check the roster, resources and W/D/L are
unchanged and the Overview shows the practice count and the filter works; administrator reopens and ends again; viewer sees badge without actions; 360 px
screenshots of list, match page and the aftermath state.

## 12. Demo world (`cmd/server/world*.go`, `docs/demo-world.md`)

- A row "Practice matches" in `docs/demo-world.md` and "Where to click": in an existing quiet campaign (or the multi-company one), two practice matches:
  one ended (Éowyn's two companies against each other, the "Same player" case, result entered, aftermath "nothing to roll") and one open with a live log;
  plus one ended competitive match of the same companies, so that the Overview shows both counts and the W/D/L count only the real one.
- Existing matches stay `practice = false`; the match totals in the demo docs are updated. The e2e flow reads all of it through the UI.

## 13. Implementation order

1. Database: migration `practice`, repository columns, the derived aftermath status in the queries and `readRecord`, W/D/L and counts, query-count check.
2. Backend: `Match.Practice`, `NewMatch`, `CreateMatch`, aftermath service refusals and `Practice` on the view, ATO disabled options, counts, mocks.
3. Frontend: new-match checkbox, badge component and `/styleguide`, Matches list badge and filter, Overview counts.
4. Frontend: match page banner, aftermath block and page state, live bar and end screen, ATO card disabled options.
5. QA: demo world, e2e flow, docs (`matches.md`, `aftermath.md`, `live-match.md`, `frontend.md`, `demo-world.md`) after the code is built.

## 14. Decisions

1. **Changing the flag after creation.** Nobody can. It is set only on the creation form; to change course, end the match and start a real one.
2. **Against the Odds.** The card stays for its table effects; any after-the-match effects do not apply.
3. **Counts.** Practice matches are excluded from W/D/L and matches played, with a separate "n practice matches" line and a filter.
4. **Campaign setting to forbid practice matches.** None.
5. **Turning a real match into a practice one.** Moot: there is no change after creation.
Design choice: the aftermath of a practice match is complete and empty (derived, never stored), not a separate flow.
