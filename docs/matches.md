# Matches: scenario, results and ending

How a match is recorded: the scenario, the reported result, who may edit what, Against the Odds before the game, and how a match ends.
It describes the match page `/campaigns/{id}/matches/{match}` and the Go types and methods behind it (`core.Matches`). What a match does to
the companies afterwards (injury rolls, experience, influence) is the War's Aftermath, in [aftermath.md](aftermath.md); while a match is
open its companies are in live mode ([live-match.md](live-match.md)).

Code: `internal/core/match.go`, `matches.go`, `match_end.go`, `ato*.go`; `internal/web/match.go`, `ato.go`; `internal/postgres/match.go`, `ato.go`
(tables in [architecture.md](architecture.md)). A write answers with the whole `#match` block, which htmx swaps in, so the header badge,
scenario and result stay in step. A stale revision answers 200 with a notice; a viewer who cannot edit gets 403; an editor with invalid input gets 422 with the form kept.

## Concepts

- A **match** has sides (each one or more companies), a **scenario**, **results** and a state: *open* until its participants end it, *ended* afterwards.
- A **scenario** is one of the Battle Companies scenarios of the campaign's rules file (chosen by name; 18 in the default file, in three
  tables of six) or a **one-off scenario** invented by the players (name, description, victory conditions: free text, not enforced, players are
  trusted). A one-off scenario lives only in that match's record.
- The scenario is optional while the match is open (players often fill it in at the table) but **required to end** the match.
- **Results** are what the players report: who won (or a draw), optional Victory Points per side, which companies fled (All is Lost), and which
  scenario objectives each company completed.

## Practice matches

A match is a **practice match** when its creator ticks **Practice match** on the new-match form (`practice=1`; off by default; "No experience, influence, rewards or
injuries. The result is only recorded."). It is stored in `matches.practice` (migration 00025), set by `core.NewMatch{Sides, Practice}` in `CreateMatch` and **never changed afterwards**:
there is no setter, route or button; to change course, end the match and start another. A practice match is otherwise a real match: its companies are in an open match (one at a time,
`CompanyBusyError`), the company pages show the same live state, the live log, end checks and secret objectives work, ending needs the same confirmations, and an administrator can
reopen it (`ReopenMatch` ignores the "aftermath started" rule for a practice match, which has none). What it never does: award or spend experience, influence, gold or rerolls, apply
scenario or Against the Odds rewards, injure, kill, change the rating, consume an item or count toward anything. Its aftermath is derived as complete and empty ([aftermath.md](aftermath.md#practice-matches)).

- **Counts**: wins, draws, losses and the Matches figure exclude practice matches; the Overview adds a muted "n practice matches" line (`CampaignSummary.PracticeMatches`) and the home list "· n practice".
- **Marks**: the `practice_badge` partial (a neutral pill, icon plus "Practice", never a result colour) after the match number on the Matches list, the Overview's latest matches, the match page header, the aftermath page
  and the live bar of the company page; a banner "Practice match: no experience, influence, rewards or injuries." on the match page (open and ended) and in the live bar ("No aftermath will follow."). The Matches tab has a filter, `?practice=1` (chips All and Practice with the count).
- **Pages**: the Aftermath card of an ended practice match says "Aftermath: nothing to roll (practice match)" for each company (no badge, no Start, no Waive); the result form says "Objectives: recorded only; no reward in a practice match"; the end checks say a practice match awards no experience (`EndChecks.Practice`) and the End match reminder adds "No aftermath will follow.".
- **Against the Odds**: the card stays for its table effects (kits, Wanderer, Ambush!). The reward bonuses (influence, experience, reroll) are disabled with "No reward in a practice match" (`ATOPracticeReason`) and cannot be newly taken (`SaveAgainstTheOdds` refuses); one saved before stays as entered and does nothing. Nothing is consumed.
- **Multi-company**: a fight between two companies of one player is the natural practice match; the "Same player" note of the form offers "Make it a practice match", which ticks the box (`/matches/new?practice=1` also ticks it).

## Who may edit

| Viewer | Match open | Match ended |
|---|---|---|
| Owner of a company in the match | edit scenario and results, propose and confirm the end | read only |
| Campaign organiser | same as a participant | edit scenario and results, reopen |
| Administrator or superuser | same as a participant | edit scenario and results, reopen |
| Other player of the campaign | read only | read only |

A **finished campaign** is read only for everybody, administrators included ([admin.md](admin.md)): creating a match and every change to one
(scenario, result, end, reopen) and to its aftermath is refused with "The campaign is finished."; the page hides "New match" and the edit
controls, and the write routes answer 403 with the same reason.

`core.Match.Access(user, organiserID)` holds the rule so the page and the service apply the same one (`MatchNoAccess`, `MatchEdit`,
`MatchEditEnded`). The page only hides controls; the service enforces the rule and returns `ErrRefused` with the reason ("Only the players in this match can
change it." / "This match has ended; ask the organiser or an administrator."). Reopening an ended match is a manager action (`ReopenMatch`),
recorded in the history, and is refused once a company has started its aftermath.

Edits are guarded by `Match.Revision`: every write takes the revision the form was rendered with and returns `ErrMatchChanged` when it is stale
(the page re-renders the match with a notice "<name> changed this while you were editing"). Two players editing at the table is the normal case.

## Go contract

```go
type ScenarioKind string // ScenarioRulebook | ScenarioCustom

type MatchScenario struct {
    Kind        ScenarioKind
    Name        string // 1-60 characters: the rules file's name, or the players' title
    Description string // custom only, 0-1000 characters, plain text
    Victory     string // custom only: how it is won, 0-1000 characters, plain text
}

type SideResult struct{ VictoryPoints *int }  // nil: not reported; 0-99
type CompanyResult struct {
    CompanyID  int64
    Fled       bool           // All is Lost: left the battlefield once Broken; the opponent claims the win
    Objectives map[string]int // objective key (rules file) -> times completed
}
type MatchResult struct {
    Decided   bool   // an outcome was reported; false means nothing reported yet (Winner 0 is then not a draw)
    Winner    int    // 1-based side, 0 a draw
    Sides     []SideResult
    Companies []CompanyResult
    Note      string // 0-500 characters
}

func (m Match) Access(u User, organiserID int64) MatchAccess
func (m Match) CanEnd() error     // no scenario, outcome not decided, roles not tagged, a Fled company on the winning side
func (m Match) ValidateResult(r MatchResult, info *ScenarioInfo) (MatchResult, error)
var ErrMatchChanged = errors.New("the match changed")
```

`Match` also carries `Scenario *MatchScenario`, `Result`, `Revision`, `EndedOn`, `History` and `EndProposal`. The methods of the web `campaigns`
interface all take the acting `core.User` and return `ErrNotFound` for an unknown campaign or match, `ErrRefused` when `Access` says no,
`ErrMatchChanged` for a stale revision and `ErrInvalidMatch` (joined sentences fit to show) for bad input:

| Method | Does |
|---|---|
| `SetScenario(ctx, user, id, matchID, revision, ScenarioInput)` | Chooses or replaces the scenario; an unknown rulebook name is `ErrInvalidMatch`; clears the reported objectives (they belonged to the old scenario) |
| `ClearScenario(...)` | Removes it |
| `SaveResult(..., MatchResult)` | Replaces the reported result as one document. Winner within `0..len(Sides)`, every company in the match, objective keys known to the scenario, counts within the maximum, VP 0-99 |
| `ProposeEnd`, `ConfirmEnd`, `WithdrawEnd`, `CancelEnd`, `OverrideEnd` | The end of the match, see "Ending a match" |
| `ReopenMatch(...)` | Managers only; refused once an aftermath started |
| `Scenarios(ctx, id)` | What the picker offers: the campaign's rules file grouped by table |
| `AgainstTheOdds`, `SaveAgainstTheOdds` | See "Against the Odds" |
| `GrantReward`, `Grants` | See "Organiser grants and scenario rewards" |

`core.ScenarioInfo` (from `core.ParseScenarios`) describes a scenario as the picker and the page show it: name, table (1-3) and roll (1-6),
`Objectives` (the ticks of the result form), `Scoring` (the Victory Point text, composed by `core.ComposeObjectiveText` from the typed objective's
`instruction` and `options[].text`), `ScoringObjectives` (with `ObjectivesFor(role)` and `HasRoles()`), `About` (outline, description, armies, layout,
starting positions, how the game ends, special rules, roles) and `Prizes` (rewards that follow from the result: Rendezvous, Vantage Point, Survival of the Fittest; shown, not ticked).

A `ScenarioObjective` is one tick per company on the result form: a reward condition of the rules file that the players tick
(`data.scenarioDetails[].rewards[]` with a `companyFlag` condition). Today those are Scavenge the Battlefield (free equipment), Baggage Train
(1 Influence) and Relic of Ages Past. Rewards that follow from the result itself (`victory`, `leaderNotRemoved`) are not ticks; they belong to the aftermath. Objective fields are posted as
`obj-<company id>-<key>`.

## The match page

Order of the cards (mobile first, 360 px up):

1. **Header**: `Match N`, result badge (Open / Draw / "<side> won"), date, "Ended <date>". An open match shows, to an editor, **Propose to end** (disabled with the reason until `CanEnd` passes)
   and **To wound**.
2. **Sides**: side cards with the companies linking to their pages; a side left at its default name "Side N" is shown by its companies (`MatchSide.Label`). When result data exists each side shows its VP and a "Fled" badge per company.
3. **Scenario card**: empty state "No scenario yet" with **Choose scenario** and **One-off scenario** (editors; others read "The players have not chosen a scenario yet"). A
   one-off shows a badge and its description and victory conditions as escaped plain paragraphs. A rulebook scenario shows **What the result earns**, a line about Attacker and
   Defender when it has roles, and three collapsed sections: **About this scenario**, **Special rules** and **How it is scored** (rule tables of the file are drawn as tables). Editors get **Change** and **Remove** (with a confirmation).
4. **Against the Odds card** (open matches; read only on an ended match that has one).
5. **Result card** (editors: a form; others: read only): the winner as a radio group (one per side plus "Draw"; nothing preselected, so an unreported match is not read as a draw), a VP field per side, per company a
   "Fled the battlefield" checkbox, one tick (or stepper) per objective of a rulebook scenario, and the note. `static/js/match_result.js` shows hints that block nothing: a winner with fewer VP than another
   side, a draw with unequal VP, two companies ticking a one-holder objective (the Relic). One **Save result** button.
6. **History** (`Match.History`, a `MatchEvent{Kind "ended"|"reopened", By, On, Detail}` per transition): once the match was ended, "Ended by X · date". Managers get **Reopen match** while no company has started or waived its aftermath; after that the
   "Edit as administrator" form warns that changes do not redo the aftermath.

States of every card: empty, editing, read only, stale (`ErrMatchChanged`: notice on top, form reloaded), error (`ErrInvalidMatch` / `ErrRefused` reasons beside the field or on top). Every campaign player can read the page; a non-member gets a 404.
Controls are touch sized (44 px) and the winner choices stack at 360 px.

### Scenario picker

A dialog loaded by `GET .../matches/{match}/scenario`: the scenarios grouped under "Table 1/2/3 (roll 1-2, 3-4, 5-6)", each a radio row with its name and a one-line summary, with a search box. "Roll for it" shows each side's rating
(`MatchSide.Rating`, the sum of its companies' points; injured units are counted) with the Top Dog / Underdog (`Match.TopDog`; equal ratings or other than two sides: no verdict), and rolls the two D6 of `scenarioSelection` (`POST .../scenario/roll`): the first picks the table, the second the scenario
(`core.ScenarioByRolls`). The server rolls and the dice animate; a typed roll needs `ManualRolls` and a thrown one `ShakeTheTable` (`CampaignSettings.RollRefusal`, 422 otherwise). The roll writes nothing: it preselects the scenario in the list and the player confirms with "Use this scenario".

### One-off scenario form

Fields: **Name** (required, 60), **Description** (1000), **How it is won** (1000), with the helper text "Players are trusted: the app does not check the victory conditions. It is kept with this match only." The same form edits it later. Text is rendered escaped with line breaks kept. A one-off scenario has no rewards; the organiser grants those.

### To wound chart

An open match has a **To wound** button beside the heading; the live company page has the same one in its banner. It opens the rulebook's To Wound Chart (Strength rows by Defence columns) in a window, so a player can check a roll without leaving the match.
The chart is `reference.toWound` of the campaign's glossary file ([definitions.md](definitions.md)); `core.Glossary.ToWound` carries it, so a campaign with another glossary file has that file's chart, and a file without one answers "This campaign's rules file has no To Wound chart.". Code: `partials/to_wound.gohtml`, `internal/web/towound.go`, `static/js/towound.js`.

- **A window, not a tab**: the live company page is long and its zones are refetched by the event stream; a window opened by an always reachable button covers neither, and it sits outside `#match` and `#live-zones`, so a write or a live refresh never closes it. Its body is fetched once when the page loads (`GET /campaigns/{id}/to-wound`, any member) and is static HTML after that, so it works while the event stream is down.
- **Find control**: two steppers, the attacker's Strength and the target's Defence; the result is shown large ("4+", "6+ then 4+", "Cannot wound") with the chart's own notation, the row, column and cell are marked in the table, and tapping a cell sets both. No unit is preselected.
- **The table**: sticky header row and column inside the window's own scroll box, 27 px cells at 360 px without sideways scrolling. Cells are colour bands from semantic tokens and never colour alone: the roll is printed in every cell ("6/4" is a 6+ then a further 4+, "-" cannot wound) and a key under the table words each band. The chart's notes (Rend, Trapped) are listed under it.

## Team tags and secret objectives

### Attacker / Defender tags

Scenarios with roles (`data.scenarioDetails[].roles`: Stand Your Ground, Defend the Gate, Hold the Line, Baggage Train) have an Attacker and a Defender and scoring objectives limited by `side`. The tag is a property of a **side**: `MatchSide.Role` is `""`, `"attacker"` or `"defender"` (`core.RoleAttacker`, `RoleDefender`), and every company of the side has it.

- **Where**: the create-match form has an optional **Role** select per side (not in all vs all); an editor of an open match changes the tags in the **Roles** card (`POST .../matches/{match}/roles`, `.../roles/swap`; **Swap roles** is one action); a badge shows on each side and per company on the result form. The controls are offered while the scenario is unchosen or has roles, and hidden when the chosen scenario has none.
- **Validation**: at most one side per role. Ending a two-team match with a roles scenario requires exactly one Attacker and one Defender ("Choose which team attacks and which defends"); a match of more sides ends without tags.
- The result form lists, per company, the scoring objectives whose `side` is `both` or equals the company's tag (`ObjectivesFor(role)`); without a tag all are shown with the note "Set the roles to hide the other side's objectives".
- Storage: `match_sides.role`, unique per match where set (migration 00017). Code: `Matches.SetSideRoles`, `SwapSideRoles`, `Match.RoleOf`.

### Secret objectives (Fog of War)

A scoring objective with `secret: true` is noted down by the player during the game and revealed at the end. The rules file marks it with `secret` and `instruction`; per objective it can carry
`nomination` (`from: ownHeroes | enemyHeroes`, `allowLeader: onlyHero`, or a terrain-piece note), `reveal: endOfGame` and the tiers the player picks from.

- The match must be open. A player **nominates** the unit for each objective that needs one (`LiveSecretNominate`: objective key and unit; the third Fog of War objective nominates a terrain piece, so `Note` carries its description) and later **logs a completion** (`LiveSecretDone`: objective key and 1-based tier, 0 clears). Both are live actions on the author's own company, editable until the end; the service checks the objective is a secret one of the scenario, who may be nominated (Heroes of the allowed side, the Leader only when the company has no other Hero) and the tier. The fold keeps the last entry per company and objective (`LiveState.SecretOf`).
- **Who sees what**: while the match is open only the author sees an entry. Others see "Noted already: <companies>" and never keys, tiers or units; administrators and the organiser see only their own. `LiveService.Log` drops other users' secret entries and `SecretObjectives(user, id, match)` returns the viewer's own lines plus the names of the companies that have some (`SecretView.Others`). A secret entry sends no live notification. Only the author undoes an entry.
- After End match everybody's entries are **revealed** in the log and on the result form. The completed tiers prefill the result form's Victory Points through `SecretScore(user, id, match, company)` with the badge "From the live match" (editable; saved only with Save result). Nothing is scored automatically.
- Page: the `secret_card` on the match page (`internal/web/secret.go`, `GET .../matches/{match}/secret`, `POST .../secret/{company}` with `kind=nominate|done`).

`SecretLine.Label` (and `ScoringObjective.Label`, `core.ObjectiveLabel`) is the short name of a secret objective for lists and the log ("Own hero survives"); `Text` is the printed rule.

## Against the Odds

The lower rated company may buy bonuses **after the Scenario Table roll and before setting up**. The card records the choice where it is made, so the loaned items, the temporary Wanderer, the chosen scenario and the Break Point can use it. Rules data: `campaign.againstTheOdds` (`eligible`, `budget`, bonuses `reroll`, `influence`, `experience`, `toolKit`, `wanderer`, `ambush`, each with `rating` and `appliesIn`; see [definitions.md](definitions.md)).

### Rules as the app applies them

- **Who**: the strictly lower rated company, in matches with **two sides of one company each**. Ratings are `CampaignCompany.Points` (injured units are not counted), compared before any bonus. Equal ratings: nobody. The higher rated company has no bonuses.
- **Budget**: the sum of the Ratings of the chosen bonuses must not exceed the gap (`opponent rating - own rating`; equal is allowed while `budget.inclusive`). Ambush! alone needs a gap of 60, a Wanderer 45, each Tool Kit 30.
- **Each bonus once**, except the Tool Kit: up to five **different** kits (Healer's, Explorer's, Scholar's, Hunter's, Raider's), each a 30 Rating purchase.
- **Tool Kit items** are assigned to roster models that take part (Warriors or Heroes, not the temporary Wanderer) before setup; capacity and alignment rules are ignored, Hero-only still applies, no extra Rating for the models, and the items are gone after the aftermath. An item that goes on a weapon (Envenom Weapon) names the weapon (`Detail`, 0-40 characters). Unassigned items are allowed while editing and are simply not in play.
- **Temporary Wanderer**: one of `data.wanderers.items`, priced at the flat 45, never together with a permanent Wanderer on the roster; not on the roster, not against the 15 model cap, counts as a model for the Break Point, no injury roll or experience, and the Top Dog / Underdog comparison ignores it.
- **Ambush!**: the lower company **chooses the scenario**, any of the 18, replacing the rolled one. Attacker and Defender are decided by the scenario's own rule, not by Ambush!. Removing the bonus restores the scenario that was set before, as long as the match still shows the scenario Ambush! chose.
- **Locked** once the first live entry exists (the game began): the card turns read only with "The game has started"; undoing every entry unlocks it. Administrators and the organiser edit like the lower company's owner and are not exempt from the lock.

### The card

It sits after the Scenario card and is visible to every campaign player (the opponent must see the Wanderer, the kits and the Ambush!); only `ATOAccessFor` (the owner of the lower rated company, the organiser and administrators, while the match is open, the campaign is not finished and the game has not started) edits. An ended match shows it only when something was chosen.

1. **Not eligible**: a read only line ("Against the Odds: nobody. Both companies are rated 240." / "Only the lower rated company can choose Against the Odds bonuses (Hador, 85 below)." / "Against the Odds is only available in two-company matches"). A match with only one side filled hides it.
2. **Gap header and budget bar**: "Hador is 85 below Gorgoroth (240 vs 325). Spend up to 85 Rating." and a bar "Spent 75 / 85, 10 left" (the text is always present; colour is only an addition).
3. **Bonus rows** (44 px targets): switch, name, Rating, one-line effect, help popup. A row that does not fit the remaining budget is disabled with the reason ("Needs 45, 10 left"). Tool Kit opens the five kit checkboxes and, per chosen kit, one select per item copy listing the models that take part (Hero-only items list Heroes only, with a weapon field for Envenom Weapon); the Wanderer opens a radio list (name, profile summary, own price crossed out, "45 here"; disabled with "The company already has a Wanderer"); Ambush! opens a select of the campaign's scenarios grouped by table, and the Scenario card then shows the badge "Ambush!".
4. **Summary line** for viewers and the locked card ("Hador took: Reroll, Wanderer (...) and the Healer's Kit.") and, for an editor, **Save** (the whole selection) and **Clear**.

Code: `LeanATO` reads `campaign.againstTheOdds` and `data.wanderers.items` (a kit key is made from its name, "Healer's Kit" is `healer`; an item is Hero-only when `data.equipment.items[].restrictions` says `hero`).

```go
type ATOSelection struct {
    Bonuses  []string        // reroll, influence, experience, toolKit, wanderer, ambush
    Kits     []string        // toolKit only: distinct kit keys, 0-5
    Items    []KitAssignment // where each kit item went (Kit, Item, Index, UnitID, Detail); may be shorter than the items
    Wanderer string          // wanderer only
    Ambush   string          // ambush only: a rulebook scenario name
}
func (s ATOSelection) Validate(rules ATORules, roster []CompanyUnit, gap int, scenarioNames []string) (ATOSelection, error)

AgainstTheOdds(ctx, user, id, matchID) (core.ATO, error)  // never an error for "not eligible": ATO.Eligible=false with Why
SaveAgainstTheOdds(ctx, user, id, matchID, revision, sel) (core.ATO, error)
```

`Validate` refuses an unknown or repeated bonus, a kit twice, more kits than the file allows, kits without the Tool Kit, an assignment outside the chosen kits or beyond an item's count, a Hero-only item on a non-Hero, a model that misses the game or is not on the roster, a weapon on anything but an `(X)` item, an unknown Wanderer or a Wanderer when the roster has a permanent one, Ambush! without a known scenario, and a spend above the gap. `SaveAgainstTheOdds` takes the match revision and runs `atoRepository.Update` in one transaction (the match, the record and "has a live entry"): Ambush! sets a rulebook scenario, clears the objectives and remembers the scenario it replaced once; an empty selection deletes the record; the match revision goes up by one. The live service and the aftermath read the record with `atoRepository.Get`.

Routes: `GET /campaigns/{id}/matches/{match}/ato` (the card, loaded into `#ato-slot`) and `POST .../ato` (the whole selection: `bonus`, `kit`, `assign-<kit>-<item>-<copy>`, `detail-...`, `wanderer`, `ambush`, `revision`). A successful save answers the whole match (`HX-Retarget: #match`) with "Against the Odds saved."; a refusal (422), a stale revision or a lock answers the card alone; a viewer's write is 403.

## Organiser grants and scenario rewards

Gold and influence also come from a scenario's special victory condition and from the organiser's narrative scenarios, in one history:

```go
type GrantKind string // "organiser" | "scenario"
type Grant struct {
    ID, CompanyID   int64
    Kind            GrantKind
    Gold, Influence int    // may be negative to correct a mistake
    Experience      int    // to UnitID when set
    UnitID, MatchID int64  // MatchID 0 for a free grant
    Note, By        string // note required for an organiser grant, 1-200 characters
    On              time.Time
}
GrantReward(ctx, user, id, companyID, core.GrantInput) (core.Grant, error) // organiser or administrator, campaign not finished
Grants(ctx, id, companyID) ([]core.Grant, error)                           // newest first, visible to every player
```

Scenario rewards are not granted by the match page: they are shown as a preview ("Completing this earns 1 Gold") and become `Grant`s of kind `scenario` when the aftermath is applied ([aftermath.md](aftermath.md)). The company page has a **Grants** card (also on the owner's My Company tab) and, on another company's page, a **Grant reward** form for the organiser and administrators (Gold, Influence, Experience and a unit, a note) on every company page, one's own included (the card stays at the bottom of the My Company tab); a player never sees the form, and neither does anyone in a finished campaign. `core.GrantInput.Validate` checks the note, that there is something to give, amounts within +-1000 Gold / +-100 Influence and Experience, and a unit for experience; a result below zero Gold or Influence is refused. `core.CampaignDetail.CanGrant` is shared by every caller. `POST .../companies/{company}/grants` answers the card again.

## Ending a match

A match ends only when **every participant confirms**: *propose, confirm, end*. The result card, the end checks and the aftermath are separate.

### Rules

- **Participants** are the owners of the companies in the match, one confirmation per company; an owner of several companies (a campaign may allow up to 4 per player, [multi-company-spec.md](multi-company-spec.md)) confirms all of them with one tap, whether they share a side or face each other. `ValidateSides` does not look at owners: a player's own companies may be on one side or on opposite sides, and `core.SamePlayers(sides)` / `Match.SamePlayers()` list the owners with several companies (companies, sides holding two or more, across sides) for the "Same player" note of the setup and the match page; each company has its own live card and its own aftermath, locked independently. A company without an owner (closed account) can only be covered by the override.
- **Proposing**: anyone who may edit the match. It runs `CanEnd` at once (scenario set, outcome decided, roles tagged, no Fled conflict) and is refused with those reasons. The proposer's own companies are confirmed automatically; an organiser or administrator who owns no company in the match confirms nothing. If nobody is left waiting (one owner, or all companies are the proposer's) the match ends at once.
- **Pending state** (`Match.EndProposal`): visible to every campaign player on the match page and, through the live banner, on every company page of the match. It lists every company as Confirmed (by name, time) or Waiting.
- **Confirm**: the owner of a waiting company; the last confirmation ends the match (`Ended`, `EndedOn`, history line "Ended: confirmed by Hador, Gorgoroth").
- **Withdraw**: the owner takes back their own confirmation while the proposal is pending; a withdrawal that leaves nobody confirmed cancels the proposal.
- **Cancel / "Keep playing"**: any participant owner, the organiser or an administrator cancels the whole proposal (all confirmations are dropped, with a line in the log). There is no timeout.
- **Override for an absent participant**: the organiser or an administrator can **End without X's confirmation** while at least one company waits. It asks for a note (1-200) and shows the warning "Gorgoroth has not confirmed that the game is over. The result and the aftermath will be fixed for everyone." The history reads "Ended by <name> overriding Gorgoroth: <note>". An organiser who owns a waiting company is confirmed, not overridden.
- **What resets confirmations**: any change to the scenario, the result, the side roles or the Against the Odds selection resets every confirmation except the proposer's and tells the others ("Hador changed the result; confirm again", `EndProposal.Notice`).
- **Reopen** clears the proposal. A finished campaign and an ended match are read only for proposals. Viewers see the pending state and cannot write (403).

### The page

While a match is open the header button reads **Propose to end**; once a proposal exists it is replaced by the **End card** directly below the header:

1. "Ending the match" with who proposed and when; one row per company: name, owner, a chip **Confirmed** (check icon and text) or **Waiting** (clock icon and text), and the time.
2. For the owner of a waiting company, **Confirm the end** and **Keep playing**; for a confirmed owner, **Withdraw my confirmation**; for the proposer, **Cancel**.
3. For the organiser or an administrator while someone waits, a quiet **End without confirmation...** that opens the dialog with the warning and the note field.
4. Viewers see the rows without buttons. A stale revision shows the usual notice; a refusal shows its reason (422). An ended match shows the history line only.

### Go contract

```go
type EndConfirmation struct {
    CompanyID, OwnerID int64
    CompanyName        string
    Confirmed          bool
    By                 string    // who confirmed (the owner, or the override author)
    On                 time.Time // zero while waiting
    Override           bool
}
type EndProposal struct {
    ProposedBy, Notice string
    ProposedByID       int64
    ProposedOn         time.Time
    Companies          []EndConfirmation // one per company, the match's order
}
// Helpers: Waiting(), WaitingOn(userID), ConfirmedBy(userID)

type EndAccess struct {
    CanPropose, CanConfirm, CanWithdraw, CanCancel, CanOverride bool
    Reason string
}
func EndAccessFor(u User, m Match, companies []CampaignCompany, organiserID int64) EndAccess

// All take the revision the form was rendered with and answer the whole match.
ProposeEnd / ConfirmEnd / WithdrawEnd / CancelEnd (ctx, user, id, matchID, revision) (core.Match, error)
OverrideEnd(ctx, user, id, matchID, revision, note string) (core.Match, error) // organiser or administrator; note 1-200
```

All five run through the match `edit` path, so the revision check, `MatchEdit` access and the finished-campaign refusal apply. `finishEnd` is the step the last confirm, a single-owner propose and the override share: it sets `Ended`, records the history and starts the aftermath lock. Every proposal change bumps `Match.Revision`, calls `Match.resetEnd` where a result changes, and notifies the live hub through `Matches.EndNotify` (event `match`). The live mode cancels a pending end with `CancelPendingEnd` (no revision).

History: `MatchEvent.Detail` is "confirmed by Rangers, Raiders" or "overriding Raiders: note". Routes (all with `revision`; the page swaps `#match`): `POST /campaigns/{id}/matches/{match}/end/propose`, `/end/confirm`, `/end/withdraw`, `/end/cancel`, `/end/override`. Storage: `match_end_proposal` and `match_end_confirmation` (migration 00023). The page templates and the live banner are described in [frontend.md](frontend.md). The demo data and the seed end matches with `ProposeEnd` plus `OverrideEnd`.
