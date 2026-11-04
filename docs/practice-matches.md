# Practice matches

A practice match is a game that awards nothing and injures nobody.
This page describes how it works, for developers and organisers.

Related: [matches.md](matches.md), [live-match.md](live-match.md), [aftermath.md](aftermath.md), [multi-company.md](multi-company.md), [demo-world.md](demo-world.md#two-bands).

Contents:

- [What a player sees](#what-a-player-sees)
- [The flag](#the-flag)
- [The aftermath is complete and empty](#the-aftermath-is-complete-and-empty)
- [What a practice match records, and what it never does](#what-a-practice-match-records-and-what-it-never-does)
- [Screens](#screens)
- [Against the Odds](#against-the-odds)
- [Persistence](#persistence)
- [Core contract](#core-contract)
- [Interaction with several companies](#interaction-with-several-companies)
- [Demo world and tests](#demo-world-and-tests)

## What a player sees

Typical uses are a friendly game, testing a new build, teaching a newcomer, or two companies of one player fighting each other.

A practice match is not a separate flow.
You create it, play it live, confirm it and end it exactly like any match.
The only difference is that its aftermath is already complete and empty, so nothing is rolled, awarded or locked.

On the new-match form, tick **Practice match** (off by default).
The helper text reads "No experience, influence, rewards or injuries. The result is only recorded."
Any member who may create a match may tick it.

The rulebook has no practice mode, so this is a house rule of the app.
Nothing in `definitions/` changes: the aftermath rules are simply not applied.

## The flag

The flag is set once, on the creation form, which posts `practice=1`.
Nobody can change it afterwards.
To change course, end the match and create a new one.
There is no route, event or menu action for it.

A practice match is a real match while it is open:

- its companies are in the open match, so they cannot join another (`CompanyBusyError`);
- the company page shows the same "match active" state, trackers and counters;
- the live log and secret objectives work;
- ending it needs the same confirmations.

The only additions on those pages are the Practice badge and banner.

## The aftermath is complete and empty

The aftermath of a practice match is derived, never stored.
From the moment the match ends, each company's aftermath reads as `done`, with these values:

- revision 1;
- an empty input and state;
- one line, "Practice match: nothing to roll.";
- `FinishedAt` equal to the match's `EndedOn`;
- this holds whether or not a `match_aftermaths` row exists, and none is ever written.

The existing aftermath state machine already supports "done with an empty record", and `AftermathStatus.Locks()` is false for `done`.
So the lock, the "cannot start a new match" check and the organiser's pending count behave correctly with no special case.
The queries (`pendingRefs`, `CompanyAftermaths`, `PendingAftermaths`, the status listing and `readRecord`) use this expression:

```sql
CASE WHEN m.practice THEN 'done' ELSE COALESCE(a.status, 'pending') END
```

Writing a `done` row at end time would need a write in every end path and a failure window.
It would also make `ReopenMatch` refuse, because a done row counts as started.
The derived read avoids all three.

`AftermathService.Aftermath` returns the derived record, so the page shows a short read-only state.
Every write answers `ErrRefused` "Practice match: nothing to roll.":
`SaveAftermath`, `StartAftermath`, `RollCasualty`, `RerollCasualty`, `Decide`, `FinishAftermath`, `ReopenSheet` and `WaiveAftermath`.
The refusal comes before any rules are read or any company is touched.

`ReopenMatch` keeps working on a practice match, so an administrator can correct a result.
Ending it again yields the same empty aftermath.
The rule "reopen is refused once an aftermath started" ignores practice matches.

## What a practice match records, and what it never does

It records:

- the scenario (rulebook or one-off) and the sides;
- the result: winner or draw, objectives completed, a note;
- who played, the live log (trackers, wounds, kills, zones, MVP votes);
- the end confirmation and history, the Against the Odds selection, and secret objectives.

The match page, live pages and log stay available forever, also after the campaign finishes.

It never does any of the following to any company of the match:

- run an aftermath: no sheet, injury rolls, casualties, cemetery entry, promotion or succession;
- award or spend experience, influence, gold or rerolls, or apply Against the Odds reward bonuses or scenario `rewards[]` (including `possessesRelic` and objective bonuses);
- change the rating, because the rating is based on the roster and the roster does not change;
- heal a model that was Injured before the game, because recovery is part of the aftermath;
- consume items (Healing Herbs, Wondrous Cram, kit items), which stay usable at the table but are never spent;
- count in Wins, Draws, Losses or "matches played";
- count toward a campaign finish, which is manual (`SetStatus`).

Wounds, Might, Will and Fate spent in live mode are match state only, derived from the log.
They never reach the company, so nothing needs undoing.

A practice match still blocks a company from being in two open matches, refuses creation while an officer or path is pending, and appears in "Open matches".
The organiser's `GrantReward` is independent of matches.

## Screens

A practice match carries a **Practice** badge wherever a match appears.
The badge is a pill in the neutral tone (`bg-surface-muted` / `text-muted`, never the win, loss or accent tones, so it is never read as a result).
It has an icon and text, text of at least 12 px, and it is on `/styleguide`.
Pages follow the usual rules: 360 px up, no horizontal scroll, light and dark, semantic tokens only.

| Page | What it shows |
|---|---|
| New match | The checkbox; when ticked, a line repeats what is skipped. The "Same player" note offers "Make it a practice match", which ticks the box |
| Campaign Matches list | The badge on the row (no Practice filter); ended practice rows show the result and no aftermath chip |
| Campaign Overview | "Matches played" excludes practice; a muted line "n practice matches" when n > 0; open practice matches count in "Open matches" |
| Match page | The badge beside "Match N" and a banner, open or ended: "Practice match: no experience, influence, rewards or injuries." Each company shows "Aftermath: nothing to roll (practice match)" with no call to action |
| Aftermath page of a company | A short read-only state: badge, "Practice match: nothing to roll.", a link back to the match and one line saying nothing was awarded and nobody was injured. No sheet, rolls or waive button |
| End match and Confirm sheets | The same sheets, with no MVP vote required; MVP and objectives say "recorded only", and the sheet says "No aftermath will follow" |
| Live match | Identical to a real match plus the badge in the match bar |
| Company page, match history | The badge and no aftermath chip |
| Rankings, company W/D/L | Exclude practice matches |
| Viewers, finished campaigns | The same badge and banner, no actions |

## Against the Odds

The Against the Odds card stays, because kits, the Temporary Wanderer and Ambush! are table effects.
Bonuses that only pay after the match (`influence`, `experience`, `reroll`) are shown disabled with "No reward in a practice match", and the aftermath ignores them.
The saved selection is kept as entered, and kit items are never consumed.

## Persistence

Migration `00025_practice_matches.sql` adds `matches.practice BOOLEAN NOT NULL DEFAULT false`.
Existing matches read `false`, and the flag is never updated after the insert.

- `matchColumns` and `matchJSON` in `postgres/match.go` select and scan `m.practice`; `Create` inserts it.
- The aftermath queries use the derived status expression above.
- The W/D/L subqueries add `AND NOT m.practice`.
  The matches-played figure counts ended, non-practice matches, and a separate count feeds the Overview line.
- The query count stays constant (`cmd/server/querycount_test.go`), because the new figure is one more column of an existing aggregate.

## Core contract

```go
// Match.Practice is set at creation and never changed.
Practice bool

type NewMatch struct {
    Sides    []NewSide
    Practice bool
}
func (s *Matches) CreateMatch(ctx context.Context, user User, id int64, in NewMatch) (Match, error)
```

- `matchRepository.Create` takes a `NewMatch`; `ValidateSides` keeps its `[]NewSide` argument.
- `AftermathView` and `Aftermath` carry `Practice bool`, so the page renders the short state.
- The ATO service marks reward bonuses `Disabled` with the reason `ATOPracticeReason` ("No reward in a practice match").
- `LiveMatches.AftermathDefaults` is never called for a practice match.
- `CampaignSummary` and `CampaignDetail` gain `PracticeMatches int`.
  `Matches` counts played non-practice matches, and `CampaignCompany.Wins`, `Draws` and `Losses` exclude practice matches.

## Interaction with several companies

- A practice match is the natural way for one player's two companies to fight, or to test a build ([multi-company.md](multi-company.md)).
  A competitive same-player match is still allowed.
- An ended practice match never locks a company.
  While it is open, its companies are locked like in any open match, one open match per company.
- Rankings stay per company, and practice matches never move them.

## Demo world and tests

The demo campaign "Two bands" has two practice matches ([demo-world.md](demo-world.md#two-bands)).
One is ended: Éowyn's two companies against each other, result entered, aftermath "nothing to roll".
The other is open with a live log.
An ended competitive match of the same companies shows that the Overview counts both and that W/D/L count only the real one.

Tests:

- Core: `CreateMatch` with `Practice`; the aftermath service returning the done record and refusing each write without calling `companyRepository.Update`; reward bonuses disabled and ignored; counts excluding practice; `ReopenMatch` allowed.
- PostgreSQL (integration): the migration; the derived status in every aftermath query for a practice and a real match side by side; no `match_aftermaths` row ever written; W/D/L and played queries; query count.
- Web: the form value, the aftermath page and its refused POSTs, the badge, banner, Overview line and disabled ATO options.
- End to end: flow `100-practice-match` in `tests/e2e/flows`.
