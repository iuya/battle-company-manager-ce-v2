# Matches: scenario, results and editing rules

Contract for the match layer between the web layer and any backend (`core.Matches`). It is
the spec of the match page `/campaigns/{id}/matches/{match}` and of the Go types and methods behind it. What a match
*does* to the companies afterwards (the War's Aftermath: injury rolls, experience, influence) is a separate layer; this one
only records what happened at the table. The aftermath is specified in [aftermath.md](aftermath.md). Status: **types, access rule, the service, the PostgreSQL repository and the page are built** (`internal/core/match.go`, `internal/core/matches.go`, `internal/web/match.go`: picker, one-off form, result form with objectives, End match); the PostgreSQL repository is `internal/postgres/match.go` (see [architecture.md](architecture.md)). A write answers with the whole `#match` block, which htmx swaps in, so header badge, scenario and result stay in step (stale revision: 200 with a notice; refused for a viewer who cannot edit: 403; refused for an editor or invalid: 422 with the form kept).

While a match is open its companies are in live mode (trackers, zones, locks): see [live-match.md](live-match.md).

Creating a match (sides and companies) already exists, see [frontend.md](frontend.md) (Campaign pages).

## Concepts

- A **match** has sides (each one or more companies), a **scenario**, **results** and a state: *open* until a participant
  ends it, *ended* afterwards.
- A **scenario** is one of the Battle Companies scenarios of the campaign's rules file (chosen by name; 18 in the default
  file, in three tables of six, see `scenarioSelection`) or a **one-off scenario** invented by the players (name,
  description, victory conditions; free text, not enforced, players are trusted). A one-off scenario lives only in that
  match's record; it never becomes a scenario type, is not offered to other matches, and a later change of the rules file
  does not touch it.
- The scenario is optional while the match is open (players often fill it in at the table) but **required to end** the match.
- **Results** are what the players report: who won (or a draw), optional Victory Points per side, which companies fled
  (All is Lost), and which scenario objectives each company completed (from the rules file, bead `gm1`).

## Who may edit (bead 1ey)

| Viewer | Match open | Match ended |
|---|---|---|
| Owner of a company in the match | edit scenario and results, end the match | read only |
| Admin or superuser | same as a participant | edit scenario and results (the page offers "Edit as administrator"; the backend also accepts it from the admin panel) |
| Campaign organiser | same as a participant (REVIEW: a default, see bead notes) | read only |
| Other player of the campaign | read only | read only |

A **finished campaign** is read only for everybody, administrators included: `CampaignDetail.CanPlay` refuses creating a match and
every change to one (scenario, result, end, reopen) and to its aftermath with "The campaign is finished."; the page hides "New
match" and the edit controls, and the write routes answer 403 with the same reason. When an editor is refused because the
match has ended, the reason is `Match.NoAccessReason` ("This match has ended; ask the organiser or an administrator."), otherwise "Only the players
in this match can change it."

`core.Match.Access(user, organiserID)` holds the rule so every backend and the page apply the same one (returns `MatchAccess`:
`None`, `Edit`, `EditEnded`). The page uses it only to hide controls; the backend enforces it and returns `core.ErrRefused`
with the reason ("Only the players in this match can change it", "This match has ended; ask the organiser or an administrator").
Ending a match is one-way for players. Reopening is an administrator action (`ReopenMatch`), recorded in the history.

All edits are last-writer-wins guarded by `Match.Revision`: every write method takes the revision the form was rendered with
and returns `ErrMatchChanged` when it is stale (the page re-renders the match with a notice "<name> changed this while you
were editing"). Two players editing at the table is the normal case.

## Go contract

Types go to `internal/core/match.go` (extending `Match` in `campaign.go`); the methods are added to the `campaigns`
interface in `internal/web/server.go`.

```go
// ScenarioKind says where a match's scenario comes from.
type ScenarioKind string

const (
    ScenarioRulebook ScenarioKind = "rulebook" // a scenario of the campaign's Battle Companies file
    ScenarioCustom   ScenarioKind = "custom"   // a one-off, free text
)

// MatchScenario is the scenario of one match. Name is the rules file's name for a rulebook scenario (matched exactly)
// and the players' title for a custom one.
type MatchScenario struct {
    Kind        ScenarioKind
    Name        string // 1-60 characters
    Description string // custom only, 0-1000 characters, plain text
    Victory     string // custom only: how it is won, 0-1000 characters, plain text
}

// ScenarioInput is what the picker or the one-off form submits. Kind decides which fields count.
type ScenarioInput = MatchScenario

func (in ScenarioInput) Validate() (ScenarioInput, error) // trims; ErrInvalidMatch (joined, fit to show) when rejected

// SideResult is what is reported for one side.
type SideResult struct {
    VictoryPoints *int // nil: not reported; 0-99
}

// CompanyResult is what is reported for one company of the match.
type CompanyResult struct {
    CompanyID  int64
    Fled       bool            // All is Lost: left the battlefield once Broken; the opponent claims the win
    Objectives map[string]int  // objective key (rules file) -> times completed (0 or absent: not completed); bead gm1 fixes keys
}

// MatchResult is the whole report. Winner uses Match.Winner's convention (1-based side, 0 a draw) once Decided; SaveResult also sets Match.Winner.
type MatchResult struct {
    Decided   bool            // the players reported an outcome; false means nothing reported yet (Winner 0 is then not a draw)
    Winner    int
    Sides     []SideResult    // same order as Match.Sides
    Companies []CompanyResult // zero or one per company of the match
    Note      string          // 0-500 characters: "a word about the game" shown on the match page
}

// Match gets these fields besides the existing ones.
//   Scenario *MatchScenario // nil until chosen
//   Result   MatchResult    // zero value while nothing is reported
//   Revision int            // bumped by every write
//   EndedOn  *time.Time

type MatchAccess int

const (
    MatchNoAccess  MatchAccess = iota // read only
    MatchEdit                         // open match, participant/organiser/admin
    MatchEditEnded                    // ended match, administrator
)

func (m Match) Access(u User, organiserID int64) MatchAccess
func (m Match) CanEnd() error // nil, or ErrRefused with the reason: no scenario, outcome not decided (a draw is chosen explicitly: Decided with Winner 0), Fled company on the winning side
func (m Match) ValidateResult(r MatchResult, info *ScenarioInfo) (MatchResult, error) // shared by every backend's SaveResult; info is the rulebook scenario (nil: unknown or custom)
func (m Match) FleeConflict() string // "" or why the reported winner contradicts a Fled company (informative; the page warns, the backend refuses on End)

var ErrMatchChanged = errors.New("the match changed")
```

`CampaignDetail.OrganiserID` gives the organiser's user id for `Access`; `core.ParseScenarios(bcFile)` builds `ScenarioInfo` (names, table and roll, scoring text).

Methods of `campaigns` (all take the acting `core.User`; they return `core.ErrNotFound` for an unknown campaign or match,
`ErrRefused` when `Access` says no, `ErrMatchChanged` for a stale revision, `ErrInvalidMatch` for bad input):

```go
// SetScenario chooses or replaces the scenario. A rulebook name unknown to the campaign's Battle Companies file is
// ErrInvalidMatch. Changing the scenario clears Result.Companies[].Objectives (they belonged to the old one).
SetScenario(ctx, user core.User, id, matchID int64, revision int, in core.ScenarioInput) (core.Match, error)
// ClearScenario removes it (open or admin-edited match only).
ClearScenario(ctx, user core.User, id, matchID int64, revision int) (core.Match, error)
// SaveResult replaces the reported result (whole document, so the form is one POST). Validation: Winner in 0..len(Sides),
// Sides length equal to the match's, every CompanyID in the match, objective keys known to the scenario (rulebook) and
// counts within the objective's maximum, VP 0-99.
SaveResult(ctx, user core.User, id, matchID int64, revision int, r core.MatchResult) (core.Match, error)
// ProposeEnd starts the end (Match.CanEnd must pass); when every participant has confirmed the match ends: Ended and EndedOn are set. The aftermath starts from here ([aftermath.md](aftermath.md)); `ReopenMatch` is refused once a company has started it.
ProposeEnd(ctx, user core.User, id, matchID int64, revision int) (core.Match, error)
// ReopenMatch is for administrators only; it records who reopened it.
ReopenMatch(ctx, user core.User, id, matchID int64, revision int) (core.Match, error)
// Scenarios lists what the picker offers: the campaign's rules file, grouped by table, with what the page shows of each.
Scenarios(ctx, id int64) ([]core.ScenarioInfo, error)
```

```go
// ScenarioInfo is a scenario of the rules file as the picker and the match page show it.
type ScenarioInfo struct {
    Name       string
    Table      int  // 1-3 (the scenario selection table it is on); 0 when the file does not place it
    Roll       int  // 1-6 on that table's second D6; 0 when unknown
    Objectives []ScenarioObjective // bead gm1; empty until the file formalises them
    Scoring    []string // text of the Victory Point conditions (scenarioScoring), shown as a read-only reminder
    About      ScenarioAbout   // rulebook text: outline, description, sections (armies, layout, starting positions, how the game ends), special rules, roles
    Prizes     []ScenarioPrize // rewards that follow from the result (Rendezvous, Vantage Point, Survival of the Fittest): shown, not ticked
}

// ScenarioObjective is one tick per company on the result form: a reward condition of the rules file that the players
// tick (data.scenarioDetails[].rewards[] with condition.kind companyFlag, and modelFlag shown as a company tick). Key and
// Label are the condition's, Max is 1 (a stepper appears when a future objective has Max > 1).
type ScenarioObjective struct {
    Key, Label string
    Max        int
    Rewards    []ScenarioReward // from campaign.aftermathEffects[key=reward.effect]
}
// Kind: influence and experience (with Amount), equipment (freeEquipment), other (the effect's text).
type ScenarioReward struct{ Kind string; Amount int; Text string }
// Preview: "1 Influence" for an amount, otherwise the effect's text; the form says "Completing this earns: ...".
func (r ScenarioReward) Preview() string
```

`core.ParseScenarios` fills `Objectives` from the file (bead `gm1`): today Scavenge the Battlefield (`allObjectiveMarkers`,
free equipment), Baggage Train (`allCargo`, 1 Influence) and Relic of Ages Past (`possessesRelic`, other). Rewards that
follow from the result (`victory`, `leaderNotRemoved`: Chance Encounter, Secure the Area, Rescue) are not ticks and not
shown on the form; they belong to the aftermath layer. Scenarios without ticks show none. Changing the scenario clears the
ticks. Objective fields are posted as `obj-<company id>-<key>`.

## The match page (mobile first, 360 px up)

Order of the cards on `/campaigns/{id}/matches/{match}`; each state below applies to every card.

1. **Header**: `Match N`, result badge (`match_result`: Open / Draw / "<side> won"), date, and when ended "Ended <date>".
   An open match shows the "Open" badge and, to an editor, **End match** (disabled with the reason underneath until
   `CanEnd` passes).
2. **Sides** (a side left at its default name "Side N" is shown by its companies, `MatchSide.Label`, everywhere on the page: cards, winner choices, Roll for it, "<side> won"): as today (side cards, companies link to the company page). When result data exists each side shows its VP and
   a "Fled" badge per company. Winner tone as `Match.Tone`.
3. **Scenario card**: empty state "No scenario yet" with **Choose scenario** and **One-off scenario** buttons (editors
   only; others read "The players have not chosen a scenario yet"). With a scenario: name; for a one-off a badge
   "One-off" and its description and victory conditions as plain paragraphs (text, never HTML); for a rulebook scenario the
   VP conditions in a collapsed section ("How it is scored"). Editors get **Change** and **Remove** (Remove asks for confirmation: a one-off's text and a rulebook scenario's ticks are lost).
   For a rulebook scenario the card also shows, from `data.scenarioDetails` (`core.ParseScenarios` fills `ScenarioInfo.About` and
   `.Prizes`): **What the result earns** (one card per reward that follows from the result, with who earns it; the ticked
   rewards stay on the result form), a line about Attacker and Defender when the scenario has roles (the app does not track
   who plays which), and three collapsed sections: **About this scenario** (outline, description, armies, layout, starting positions,
   how the game ends), **Special rules** and **How it is scored**. All text is escaped with line breaks kept.
4. **Result card** (editors: a form; others: read only): winner as a radio group (one per side, plus "Draw"; nothing
   preselected so an unreported match is not read as a draw), VP number field per side (optional, `inputmode=numeric`),
   per company a "Fled the battlefield" checkbox (with the All is Lost rule in a help popup), one tick (or stepper if
   `Max`>1) per objective of a rulebook scenario, and the note. As the players type, `static/js/match_result.js` shows hints that block nothing (the server never refuses these): a winner with fewer Victory Points than another side, a draw with unequal Victory Points, and two companies ticking a one-holder objective (the Relic). One **Save result** button; htmx swaps the card, the
   header badge updates out of band.
5. **History** (`Match.History`, a `core.MatchEvent{Kind "ended"|"reopened", By, On}` per transition, oldest first, newest last): shown once the match was ended; "Ended by X · date, time". Administrators also get **Reopen match** in the header of an ended match while no company has started or waived its aftermath (`matchView.AftermathApplied`; after that the button is gone, the backend refuses anyway, and the "Edit as administrator" form says changes do not redo the aftermath, otherwise that the new result will be used) (`POST …/reopen`; anyone else gets 403, a stale revision the usual notice); the backend records the event when the match ends and on `ReopenMatch` and fills `Match.History` when it loads a match.

States for every card: *empty* (nothing reported), *editing* (open, editor), *read only* (open, not an editor; ended, not an
admin), *admin on ended* (the form behind an "Edit as administrator" button, with a warning "The aftermath was already
applied; changes here do not redo it"), *stale* (`ErrMatchChanged`: notice on top, form reloaded with current values), *error*
(`ErrInvalidMatch` / `ErrRefused` reasons beside the field or on top, as the other forms do), *forbidden* (the page itself is
readable by every campaign player; a non-player gets the existing 404).

Controls are touch sized (44 px), the winner choices stack on 360 px, and the VP fields sit beside the side name from 768 px.

### Scenario picker (bead spe, part 1)

A dialog or inline card (`htmx` GET `…/matches/{match}/scenario`, POST to set): the scenarios grouped under
"Table 1/2/3 (roll 1-2, 3-4, 5-6)" each as a radio row with its name and a one-line summary; a search box above (accent and
case insensitive, as `lists.js`). Choosing posts the name and revision. "Roll for it" sits above the list: it shows each side's rating (`MatchSide.Rating`, the sum of its companies' points, injured units counted) with the Top Dog / Underdog (`Match.TopDog`; equal ratings or other than two sides: no verdict), and rolls the two D6 of `scenarioSelection` (`POST …/scenario/roll`, answers the `#scenario-roll` panel): the first picks the table, the second the scenario (`core.ScenarioByRolls`). The server rolls and the dice animate; a typed roll needs `ManualRolls` and a thrown one `ShakeTheTable` (`CampaignSettings.RollRefusal`, 422 otherwise). It writes nothing: it only preselects the scenario in the list (and clears the search), the player confirms with "Use this scenario".

### One-off scenario form (bead spe, part 2)

Fields: **Name** (required, 60), **Description** (textarea, 1000), **How it is won** (textarea, 1000), helper text
"Players are trusted: the app does not check the victory conditions. It is kept with this match only." Counters on the textareas;
errors from `ScenarioInput.Validate`. The same form edits it later (prefilled). Plain text is rendered escaped with line
breaks kept (`white-space: pre-line`). No rewards: gold or influence for a one-off scenario is granted by the organiser (below).

## Organiser and scenario rewards (bead e3e, lower priority)

Gold and influence also come from a scenario's special victory condition (objectives with `Rewards`, `gm1`) and from the
organiser's narrative scenarios. Two sources, one history:

```go
type GrantKind string // "organiser" | "scenario"

// Grant is one entry of a company's history of awards.
type Grant struct {
    ID         int64
    CompanyID  int64
    Kind       GrantKind
    Gold, Influence int  // may be negative to correct a mistake
    Experience int       // to UnitID when set; ignored (rejected) when UnitID is 0
    UnitID     int64
    MatchID    int64     // the match it came from, 0 for a free grant
    Note       string    // required for an organiser grant, 1-200 characters
    By         string    // display name of who granted it
    On         time.Time
}

// GrantReward lets the organiser (or an administrator) award a company; it changes the company's Gold/Influence at once.
// Refused (ErrRefused) for anyone else and for a finished campaign. Input errors are ErrInvalidCompany.
GrantReward(ctx, user core.User, id, companyID int64, g core.GrantInput) (core.Grant, error)
// Grants is a company's history, newest first (visible to every player of the campaign, like the rest of the company page).
Grants(ctx context.Context, id, companyID int64) ([]core.Grant, error)
```

Scenario rewards are *not* granted by the match page: they are shown as a preview ("Completing this earns 1 Gold") and become
`Grant`s of kind `scenario` when the aftermath is applied ([aftermath.md](aftermath.md), step 6). Until then `GrantReward` is
the way to hand them out by hand. Page: the company page gets a **Grants** card (history, newest first, visible to every player; also on the owner's My Company tab) and,
on the page of another company, for the organiser and administrators, a **Grant reward** form (Gold, Influence, Experience + unit,
note; the company's current Gold and Influence beside it). The form is not offered on one's own company. `core.GrantInput.Validate`
(note 1-200 required, something to give, amounts within +-1000 Gold / +-100 Influence and Experience, a unit for experience) and
`core.CampaignDetail.CanGrant` (organiser, administrator, campaign not finished) are shared by every backend; a result below zero
Gold or Influence is refused. `POST …/companies/{company}/grants` answers the card again.

## Rules data

- Scenario names, tables and Victory Point text: `campaign.scenarioSelection`, `scenarios`, `scenarioScoring` in the
  Battle Companies file, already there.
- Rewards that depend on a scenario: `campaign.aftermathEffects[]` (e.g. Rendezvous, To the Victor the Spoils, Cargo) and
  the Against the Odds bonuses; to be folded into per-scenario objectives with stable keys by bead `gm1` (curator, in
  progress at the time of writing). The contract above assumes: a list of objectives per scenario, each with `key`, `label`,
  a maximum, and its rewards (Gold, Influence, Experience, equipment). If `gm1` lands different names, only
  `ScenarioInfo`/`ScenarioObjective` and the parser (`core.ParseScenarios`) change; the match methods and the page do not.
- Experience a model earns (`campaign.experience.gains`), the per-casualty rolls and which model holds the Relic belong to the aftermath layer ([aftermath.md](aftermath.md)), not here; the result form only ticks that the company holds the Relic.

## Team tags and secret objectives (tags built in core and db, mtu.18; web: mtu.19; secret objectives: mtu.20)

### Attacker / Defender tags on teams

- Scenarios with roles (`data.scenarioDetails[].roles`: Stand Your Ground, Defend the Gate, Hold the Line, Baggage Train) have an Attacker and a Defender and scoring objectives limited by `side`. The tag is a property of a **side** (team): `MatchSide.Role` `""`, `"attacker"` or `"defender"`; every company of the side has it. Rule data `campaign.matchInput.perCompany.role` ("when scenario.roles") is derived from it, not asked again.
- **Where**: the create-match form has an optional **Role** selector per side (None / Attacker / Defender) because the scenario is often chosen later; the match page header shows a badge per side and an editor (anyone who may edit the open match) can change the tags while the match is open. Offered always while the scenario is unchosen or has roles; **hidden when the chosen scenario has no roles** (the stored tag is kept but ignored and not shown). So: free to set early, meaningful and validated only for scenarios that have roles.
- **Validation** (`Match` validate): at most one side per role; with roles scenario chosen, End match requires exactly one Attacker and one Defender side, otherwise `ErrInvalidMatch` "Choose which team attacks and which defends" (REVIEW: only for matches of exactly two sides; a three side match with a roles scenario ends without tags). Swapping is one action (**Swap roles**).
- The result form shows, per company, only the scoring objectives whose `side` is `both` or equals the company's tag (without a tag all are shown with the note "Set the roles to hide the other side's objectives"). The aftermath reads the role for Against the Odds as today.
- Persistence: `match_sides.role TEXT NULL CHECK (role IN ('attacker','defender'))`, unique per match where not null.

### Secret objectives (Fog of War)

Today `scoringObjectives[].secret` and `instruction` already mark them in the rules file; what is missing is the model for what the player notes down. **Proposal to the definitions-curator** (bead `mtu.11.1`), per scoring objective with `secret: true`:
- `nomination`: `{from: "ownHeroes" | "enemyHeroes", allowLeader: "onlyHero"}` when the instruction says to note down a Hero model (ownHeroSurvives, enemyHeroHarmed...); absent when nothing is noted (objectives that score on events).
- `reveal`: `"endOfGame"` (only Fog of War; the default for a secret objective).
- `completion`: how the player reports it: `tiers` as today (the player picks the best reached option at the end) or `done` (a yes/no for a one-point objective). The existing `kind`/`options` already hold the tiers; the curator confirms every Fog of War bullet is flagged.
- Schema, validator and `docs/definitions.md` updated by the curator; the app reads only these keys.

App behaviour (log entries of the live layer, see [live-match.md](live-match.md); the match must be open):
- At the start (any time during the game) a player **nominates** the unit for each objective that needs one (`secretNominate`: objective key, unit; enemy unit picked among the opposing companies' Heroes) and later **logs a completion** (`secretDone`: objective key, tier index; "Frodo completed a secret objective" with no detail in what others see). Both are editable until the end, undo as everywhere.
- **Who sees what**: the author only (nomination, objective, tier) until End match; others see at most "Frodo has secret objectives" counts, never keys, tiers or nominated units; administrators and the organiser see only their own (they are players too; REVIEW). The live event stream notifies only the author's tab. After End match the entries are **revealed** to everybody in the log and on the result form ("Secret objectives: Frodo scored 3 for Hero survives untouched"). Privacy is enforced in the service when listing (`LiveEntry.Hidden` is filtered on read, not in the template).
- **Feed**: at End match the completed tiers prefill the company's `Objectives`/`victoryPoints` on the result form (`From the live match` badge), editable; nothing is scored automatically (players are trusted).
- **No join requests** anywhere: a campaign is entered only by invite link (decision 2026-10-21).

Acceptance criteria:
- [ ] Side tags on create and on the match page; hidden for scenarios without roles; Swap roles; End refuses a roles scenario without both tags (two sides).
- [ ] The result form hides objectives of the other role.
- [ ] A secret objective can be nominated and logged as done by its author; no other user's response (page, log, SSE, API) contains the key, tier or unit before End match (test with a second user); after End match everyone sees them and the result form is prefilled.
- [ ] Tests: validation of tags, filtered log per viewer, reveal at end, prefill.

Built for the tags (mtu.18): `MatchSide.Role` / `NewSide.Role` (`core.RoleAttacker`, `RoleDefender`, `ValidRole`), `Match.RoleOf(companyID)`, `Matches.SetSideRoles` (one tag per side, open matches only) and `SwapSideRoles`, `EndMatch` requiring both tags of a two-team match with a roles scenario ("Choose which team attacks and which defends"), migration `00017_match_side_role.sql`, and `ScenarioInfo.ScoringObjectives` / `ObjectivesFor(role)` / `HasRoles()` (the scoring objectives as data, secret ones with their nomination). Web contract: the create form passes `NewSide.Role`; the match page calls `SetSideRoles` / `SwapSideRoles` with the revision; the result form lists `ScenarioInfo.ObjectivesFor(match.RoleOf(company))`.

Built for the secret objectives (mtu.20, backend): `LiveSecretNominate` / `LiveSecretDone` actions (`LiveAction.ObjectiveKey`, `Tier` 1-based with 0 clearing, `UnitID` or `Note` for the nominee: the third Fog of War objective nominates a terrain piece, so `Note` carries its description), entered with `LiveService.Apply` on the author's own company; the service checks the objective is a secret one of the match's scenario (`BCBook.Scenario`), who may be nominated (`ownHeroes` / `enemyHeroes` Heroes, the Leader only when the company has no other Hero) and the tier. The fold keeps the last entry per company and objective (`LiveState.SecretOf`). Privacy on read: `LiveService.Log` drops other users' secret entries while the match is open, `SecretObjectives(user, id, match)` returns the viewer's own lines plus the names of the companies that have some (`SecretView.Others`), everybody's after the end (`Revealed`); only the author undoes an entry (`CanUndoEntry`, even the organiser cannot); a secret entry sends no NOTIFY (its seq still advances). `SecretScore(user, id, match, company)` gives the Victory Points of the reported tiers for the result form to prefill after the end. Migration 00016 allows the kinds.

`SecretLine.Label` (and `ScoringObjective.Label`, `core.ObjectiveLabel`) is the short name of a secret objective for lists and the log ("Own hero survives"); `Text` is the printed rule.

## Against the Odds before the game (spec, designed; epic `36y`)

The lower rated company may buy bonuses **after the Scenario Table roll and before setting up** ("selectedAt:
afterScenarioRollBeforeSetup"). Today only the aftermath sheet records them; this card records the choice where it is made, so the
loaned items, the temporary Wanderer, the chosen scenario and the break point can use it. Rules data: `campaign.againstTheOdds`
(`eligible`, `budget`, bonuses `reroll`, `influence`, `experience`, `toolKit`, `wanderer`, `ambush`, each with `rating`,
`appliesIn`; see [definitions.md](definitions.md)). Readings below are the curator's recommended ones; each is a `REVIEW:`.

### Rules as the app applies them (REVIEW)

- **Who**: the strictly lower rated company (ratings are `MatchSide.Rating`, injured units counted, compared **before** any bonus).
  Equal ratings: nobody. Only matches with **two sides of one company each** (REVIEW: other shapes get no card; the aftermath
  sheet's manual ticks remain). The higher rated company has no bonuses.
- **Budget**: the sum of the Ratings of the chosen bonuses must not exceed the gap (`opponent rating - own rating`, inclusive); this
  is the same as "rating after bonuses may equal but not exceed the opponent's". Ambush! alone needs a gap of 60, a Wanderer 45, each Kit 30.
- **Each bonus once** (REVIEW: the source never says if Influence/Experience/Reroll may repeat; the data is a set), except the Tool
  Kit: up to five **different** kits, each a 30 Rating purchase = one kit (T1).
- **Tool Kit**: the items of a chosen kit are assigned to roster models that take part (Warriors or Heroes, not the temporary
  Wanderer) before setup; capacity and alignment rules are ignored, Hero-only still applies, no extra Rating for the models, gone after
  the match's aftermath (T2-T5). Envenom Weapon items name the weapon they are on (`Detail`, free text 0-40).
- **Temporary Wanderer**: one of `data.wanderers.items`, priced at the flat 45 (W4), never together with a permanent Wanderer on the roster (W1),
  not on the roster, not against the 15 model cap (W2), counts as a model for the Break Point, no injury roll or experience (W3), the Top Dog/Underdog comparison ignores it (W5).
- **Ambush!**: the lower company **chooses the scenario**, any of the 18 (A1), replacing the rolled one. Roles (Attacker/Defender)
  are decided by the scenario's own rule, not by Ambush (A2; the existing Attacker/Defender tags stay as they are). Nothing special
  for Fog of War (A3, A4).
- **Locked** once the first live entry exists (the game began): the card turns read-only with "The game has started"; undoing every
  entry unlocks it. Administrators and the organiser edit like the lower company's owner and are not exempt from the lock.

### Match page: the card

Position: after the **Scenario** card and before the **Result** card (an open match only; an ended match shows it read only when
something was chosen, otherwise it is hidden). Visible to every campaign player (table information: the opponent must see the
Wanderer, the kits and the Ambush). Editable only by `ATOAccess` (below).

States, top to bottom:

1. **Not eligible** (read only line, no controls): "Against the Odds: nobody. Both companies are rated 240." or "Only the lower rated
   company can choose Against the Odds bonuses (Hador, 85 below)." for the higher company; "Against the Odds is only available in
   two-company matches" otherwise. A match with only one side filled: hidden.
2. **Gap header** (eligible): "Hador is 85 below Gorgoroth (240 vs 325). Spend up to 85 Rating." and a **budget bar**: segments per chosen
   bonus, "Spent 75 / 85, 10 left" (text always present, colour only an addition). Ratings in the header link to the sides.
3. **Bonus rows** (touch targets 44 px, one row per bonus of `campaign.againstTheOdds.bonuses`): switch, name, Rating, one-line effect,
   help popup with the full text. A row that does not fit the remaining budget is disabled with the reason underneath ("Needs 45, 10 left").
   Rows with a sub-choice open it in place when switched on:
   - **Tool Kit**: five kit checkboxes (Healer's, Explorer's, Scholar's, Hunter's, Raider's) each 30, with the list of items; each chosen
     kit opens an **item assignment** list: one row per item instance ("Wondrous Cram", "Healing Herbs" x2...) with a select of
     the company's models that take part (name and type; Hero-only items list Heroes only), a text field for Envenom Weapon. Unassigned items are
     allowed while editing (shown "Not given to anybody yet" with a warning dot) and are simply not in play.
   - **Temporary Wanderer**: a radio list of the Wanderers (name, profile summary, own price crossed out, "45 here"). Disabled with the reason
     "The company already has a Wanderer" when it has one.
   - **Ambush!**: a **Choose the scenario** button that opens the scenario picker (matches.md, picker) in Ambush mode; the card then
     shows "Scenario: X, chosen by Hador with Ambush!" and the Scenario card above gets the badge "Ambush!". Removing the bonus restores the scenario that was set before (REVIEW).
4. **Summary line** for viewers and when locked: the same content as plain text: "Hador took: Reroll, Wanderer (Rangers of ...) and the
   Healer's Kit."
5. Buttons for an editor: **Save** (one save for the whole selection; stale revision gives the usual notice and reloads the card), **Clear** (asks to confirm).

Order of play: the Scenario card can still roll and set a scenario first; Ambush simply replaces it. The card is usable before
any scenario is set (Ambush then sets it), but the Rating gap is always shown.

Empty state: eligible but nothing chosen: the rows with their switches off and "No bonus chosen."; error state: reasons from
`ErrInvalidMatch` beside the row (over budget, kit assigned to the wrong model); forbidden: viewers see the summary only (403 on writes).
360 px: bonus rows stack, the budget bar is full width under the header, the assignment list is one row per item with the select below the name.

### Go contract

Declared by the web layer (`internal/web/server.go`, the `campaigns` interface), implemented by `core.Matches`.

```go
// ATOSelection is what the company took; saved as one document with the match.
type ATOSelection struct {
    Bonuses   []string        // campaign.againstTheOdds.bonuses[].key: reroll, influence, experience, toolKit, wanderer, ambush
    Kits      []string        // toolKit only: distinct kit keys (healer, explorer, scholar, hunter, raider), 0-5
    Items     []KitAssignment // where each kit item went; len may be less than the items (unassigned)
    Wanderer  string          // wanderer only: data.wanderers.items name
    Ambush    string          // ambush only: rulebook scenario name (one of the campaign's scenarios)
}

type KitAssignment struct {
    Kit    string // kit key
    Item   string // data.equipment.items name
    Index  int    // 0-based copy of that item inside the kit (Healing Herbs x2)
    UnitID int64  // 0: unassigned
    Detail string // Envenom Weapon: the weapon, 0-40
}

// Validate checks keys, distinct kits, the Hero-only restriction on assignments, Ambush against the scenarios, the Wanderer against the data and the
// rule "no permanent Wanderer" (given roster), and the budget (sum of ratings <= gap). ErrInvalidMatch joined, fit to show.
func (s ATOSelection) Validate(rules ATORules, roster []CompanyUnit, gap int) (ATOSelection, error)

// ATOOption is one row of the card.
type ATOOption struct {
    Key, Name, Text string
    Rating          int
    Taken           bool
    Disabled        bool
    Reason          string // why disabled ("Needs 45, 10 left"; "The company already has a Wanderer")
    AppliesIn       string // setup | game | aftermath | scenarioChoice
}

// ATO is the card's whole state.
type ATO struct {
    MatchID, CompanyID int64
    Eligible           bool
    Why                string // when not eligible: the sentence shown
    OwnRating, OppRating, Gap, Spent int
    Options            []ATOOption
    Kits               []ATOKit      // {Key, Name, Items []{Item, Count, HeroOnly}}
    Wanderers          []ATOWanderer // {Name, Summary, OwnPrice}
    Models             []ATOModel    // {UnitID, Name, Type, Hero bool}: who may carry, those that take part
    Selection          ATOSelection
    Locked             bool          // the game began
    LockedWhy          string
    Revision           int
    Access             ATOAccess
}

type ATOAccess int // ATONone (read only) | ATOEdit
// ATOAccessFor: the owner of the lower rated company, the campaign organiser and administrators, while the match is open, the campaign is not
// finished and the game has not started.
func ATOAccessFor(u User, m Match, c CampaignCompany, organiserID int64) ATOAccess
```

```go
// AgainstTheOdds loads the card for the viewer. ErrNotFound: campaign or match unknown, or the viewer is not in the campaign. Never an error for "not eligible": ATO.Eligible=false with Why.
AgainstTheOdds(ctx, user core.User, id, matchID int64) (core.ATO, error)
// SaveAgainstTheOdds replaces the selection (an empty one clears it). ErrRefused (not the lower company's owner / locked: reason fit to show),
// ErrMatchChanged (stale revision), ErrInvalidMatch (Validate). Choosing Ambush sets Match.Scenario (rulebook kind) and
// records the previous scenario; dropping Ambush restores it. Bumps Match.Revision.
SaveAgainstTheOdds(ctx, user core.User, id, matchID int64, revision int, sel core.ATOSelection) (core.ATO, error)
```

Persistence: one row per match and company (`match_ato`: selection as jsonb, previous scenario, revision). The record is read by the
live service (loaned items, the temporary Wanderer, Break Point models) and by the aftermath (prefilled bonuses, kit items). Routes:
`GET /campaigns/{id}/matches/{match}/ato` (the card), `POST .../ato` (save; answers the card and, for Ambush, the Scenario card out of band).

Built (36y.6, backend, `internal/core/ato.go`, `ato_service.go`, `ato_repository.go`): `ParseATO` reads `campaign.againstTheOdds` (bonuses with their Ratings, `budget.inclusive`, the Tool Kit's kits and `maxPurchases`) and `data.wanderers.items` (name, own price, a one-line profile); a kit key is made from its name ("Healer's Kit" is `healer`; the file has no key, bead for the curator if it should) and an item is Hero-only when `data.equipment.items[].restrictions` says `hero`. `ATOSelection.Validate(rules, roster, gap, scenarioNames)` (one more argument than the sketch above: the scenarios Ambush! may choose from) refuses, with `ErrInvalidMatch` sentences, an unknown or repeated bonus, a kit twice, more kits than the file allows, kits without the Tool Kit, an assignment outside the chosen kits or beyond an item's count, a Hero-only item on a non-Hero, a model that misses the game or is not on the roster, a weapon on anything but an `(X)` item (0-40 characters), a Wanderer that is unknown or when the roster has a permanent one, Ambush! without a known scenario, and a spend above the gap (equal is allowed while `budget.inclusive`); it returns the selection cleaned (the file's order). `Matches.WithATO(repo)` gives the service the store; `Matches.AgainstTheOdds(user, id, match)` builds the card (`ATO.Eligible=false` with `Why` for a shape without one; ratings are `CampaignCompany.Points`, so injured models are not counted: REVIEW, the spec above said they were; `Locked` once `GameStarted`, `Access` from `ATOAccessFor` minus lock and finished campaign; every viewer sees the lower company's selection); `Matches.SaveAgainstTheOdds(user, id, match, revision, sel)` takes the **match** revision, validates, then runs `atoRepository.Update` (one transaction: the match, the record and "has a live entry"): Ambush! sets a rulebook `Match.Scenario`, clears the objectives and remembers the scenario it replaced once (`ATORecord.Previous`, `HadPrevious`); dropping Ambush! restores it only while the match still shows the scenario Ambush! chose (a scenario set by hand since is left alone); an empty selection deletes the record; the match revision goes up by one. `atoRepository` has `Get`, `GameStarted` and `Update` (declared in `ato_repository.go`, mock generated); the live service and the aftermath read the record with `Get`. Frontend (36y.8): render `ATO` as in the card above; post the whole selection to `SaveAgainstTheOdds` with the card's `Revision`.

Built (36y.8, web, `internal/web/ato.go`, `templates/partials/ato.gohtml`): the match page has an `#ato-slot` after the Scenario card that loads `GET .../ato` (`ato_card`). Editors get one form: bonus switches with the budget worked out live in the browser (a row that does not fit is disabled with "Needs N, M left"; the Wanderer's server reason shows as is), a budget bar with the text "Spent X / G, Y left", the kit checkboxes (30 each), one select per kit item copy (Hero-only items list Heroes only; a Detail item has a weapon field), the Wanderer radios (price crossed out, "45 here") and, for Ambush!, a select of the campaign's scenarios grouped by table (a select, not the picker window). One Save posts the whole selection (`bonus`, `kit`, `assign-<kit>-<item>-<copy>`, `detail-...`, `wanderer`, `ambush`, `revision`). A successful save answers the whole match (`HX-Retarget: #match`: the revision moved and the Scenario card may have changed) with "Against the Odds saved."; a refusal (422), a stale revision or a lock answers the card alone with the message; a viewer's write is a 403. Viewers and the locked card show the summary line and where each item went. An ended match shows the card only when something was chosen. Clear unticks everything and saves.

### Acceptance criteria

- [ ] The card shows the gap and the budget from the sides' ratings; equal ratings and the higher company see "not eligible" and no controls; non two-company matches hide the controls.
- [ ] A bonus that does not fit the remaining budget is disabled with the reason; the server refuses an over budget selection the page let through (a tampered form).
- [ ] Tool Kit: up to five different kits at 30 each, items assignable to roster models taking part only, Hero-only items to Heroes only, unassigned allowed.
- [ ] Wanderer: refused with a permanent Wanderer on the roster; Ambush: any scenario of the campaign, replaces the match scenario, removing it restores the previous one.
- [ ] Viewers (including the opposing owner) see the summary and cannot write (403); after the first live entry the card is read only with the reason.
- [ ] Stale revision gives the usual notice; 360 px, light and dark, tokens only, no horizontal scroll, `/styleguide` components.
- [ ] Tests: `Validate` tables (budget edge equal, one over, duplicate kit, wrong carrier), access table, service tests for Ambush set/restore and lock, handler tests for the card states, a postgres integration test.

Beads: backend `36y.6`, database `36y.7`, frontend `36y.8`; live and aftermath consumers `36y.9`, `36y.10`, `36y.11`.

## Ending a match: confirmation by all participants (spec, designed; epic `6uh`)

User decision 2026-10-22: **a match ends only when every participant confirms**. Before this, any editor ended it alone (`EndMatch`, now internal). The new flow is
*propose, confirm, end*; the result card, the end checks and the aftermath are unchanged. Defaults below are `REVIEW:` items.

### Rules

- **Participants** are the owners of the companies in the match, **one confirmation per company**; an owner of several companies confirms all of
  them with one tap. A company without an owner (account closed) can only be covered by the override.
- **Who starts it**: anyone who may edit the match (`MatchEdit`: a participant, the organiser, an administrator). It runs `CanEnd` at once (scenario
  set, outcome decided, roles tagged, no Fled conflict) and is refused with the same reasons as today. The proposer's own companies are confirmed
  automatically; an organiser or administrator who owns no company in the match confirms nothing (they are not a participant). If nobody is left waiting
  (a single owner, or all the companies are the proposer's) the match ends **at once**, as today.
- **Pending state** (`Match.EndProposal != nil`): visible to every campaign player on the match page and, through the live banner, on every
  company page of the match (see [live-match.md](live-match.md)). It lists every company as *Confirmed (by name, time)* or *Waiting*.
- **Confirm**: the owner of a waiting company. The last confirmation ends the match (`Ended`, `EndedOn`, history line "Ended: confirmed by Hador, Gorgoroth").
- **Withdraw**: the owner takes back their own confirmation while the match is still pending (a proposer who withdraws cancels the proposal if nobody else confirmed; otherwise the proposal stays with the remaining confirmations).
- **Cancel / "Keep playing"**: any participant owner, the organiser or an administrator cancels the whole proposal (all confirmations are dropped; a line in the log). There is no timeout: a
  pending end waits for as long as the players need.
- **Absent participant: override.** The organiser or an administrator can **End without X's confirmation** while at least one company is waiting. It asks for a
  note (1-200) and shows the warning "Gorgoroth has not confirmed that the game is over. The result and the aftermath will be fixed for everyone." The
  history reads "Ended by <name> overriding Gorgoroth: <note>". A participant who is also the organiser overrides only the other companies, never their own (they confirm). REVIEW: the override is the only way out for an absent player.
- **What invalidates confirmations**: any change to the scenario, the result (winner, VP, Fled, objectives, note), the side roles or the Against the Odds
  selection resets every confirmation except the proposer's and shows "Hador changed the result; confirm again" to the others. Live play after a proposal is described in [live-match.md](live-match.md).
- **Reopen** is unchanged (administrator, before any aftermath started); reopening clears the proposal. A finished campaign and an ended match are read only for proposals.
- Ratings, aftermath and permissions otherwise as above: viewers (not in the match) see the pending state and cannot write (403).

### Page (match header and a card)

The header button of an open match reads **Propose to end** (it was **End match**; disabled with the reason underneath until `CanEnd` passes, plus the end-check reminder with *Go back* / *Propose anyway* from live-match.md). Once a proposal exists the button is replaced by the
**End card**, directly below the header:

1. Title "Ending the match" with who proposed and when; one row per company: name, owner, chip **Confirmed** (check icon, text) or **Waiting** (clock icon, text), the time for confirmed ones.
2. For a waiting company's owner: primary **Confirm the end**, secondary **Keep playing** (cancels). For a confirmed owner: **Withdraw my confirmation**. For the proposer: **Cancel**. Touch targets 44 px.
3. For the organiser/administrator when someone waits: a quiet **End without confirmation...** that opens the dialog with the warning and the note field.
4. Viewers: the same rows, no buttons. Stale revision: the usual notice and the card reloads. Refused: reason on top (422). An ended match shows the history line only.

States: *no proposal*, *pending (you waiting)*, *pending (you confirmed)*, *pending (viewer)*, *override dialog*, *stale*, *ended*. 360 px: rows stack, the chips sit below the company name.

### Go contract

```go
// EndConfirmation is one company's part of a pending end.
type EndConfirmation struct {
    CompanyID, OwnerID int64
    CompanyName        string
    Confirmed          bool
    By                 string    // display name of who confirmed (the owner, or the override author)
    On                 time.Time // zero while waiting
    Override           bool      // confirmed by the organiser/administrator override
}

// EndProposal is the pending end; Match.EndProposal is nil when there is none.
type EndProposal struct {
    ProposedBy string
    ProposedOn time.Time
    Companies  []EndConfirmation // one per company of the match, the match's order
}
func (p EndProposal) Waiting() []EndConfirmation

// EndAccess is what the viewer may do now; the page only hides controls with it, the service enforces it.
type EndAccess struct {
    CanPropose, CanConfirm, CanWithdraw, CanCancel, CanOverride bool
    Reason string // why a control is missing, when it helps ("Only the players in this match can change it")
}
func EndAccessFor(u User, m Match, companies []CampaignCompany, organiserID int64) EndAccess

// Match gets EndProposal *EndProposal. MatchEvent.Kind "ended" gets Detail (who confirmed, or the override and its note).
```

```go
// Declared by the web layer (replaces EndMatch on the page); implemented by core.Matches. All take the revision the form was rendered with
// and answer the whole match: ErrNotFound, ErrRefused (reason fit to show), ErrMatchChanged (stale), ErrInvalidMatch (CanEnd's reasons).
ProposeEnd(ctx, user core.User, id, matchID int64, revision int) (core.Match, error)   // CanEnd, auto-confirms the proposer's companies, ends at once when nobody waits
ConfirmEnd(ctx, user core.User, id, matchID int64, revision int) (core.Match, error)   // the viewer's waiting companies; the last one ends the match
WithdrawEnd(ctx, user core.User, id, matchID int64, revision int) (core.Match, error)  // the viewer's own confirmations
CancelEnd(ctx, user core.User, id, matchID int64, revision int) (core.Match, error)    // participant owner, organiser, administrator
OverrideEnd(ctx, user core.User, id, matchID int64, revision int, note string) (core.Match, error) // organiser/administrator; note 1-200 (ErrInvalidMatch)
```

`finishEnd` (on `Match`) is the step the last `ConfirmEnd`, `ProposeEnd` and `OverrideEnd` share (it runs after `CanEnd`, sets `Ended`, records the history, and starts the aftermath lock). The exported `Matches.EndMatch` is gone (7k7): the unexported `endMatch` remains for core tests; the demo data and the seed end matches with `ProposeEnd` plus `OverrideEnd`. Every proposal change bumps `Match.Revision` and notifies the live hub (event `match`, see live-match.md).
Routes: `POST /campaigns/{id}/matches/{match}/end/propose`, `/end/confirm`, `/end/withdraw`, `/end/cancel`, `/end/override` (htmx, CSRF, answer the `#match` block).
Persistence: `match_end_proposal` and `match_end_confirmation` (beads `6uh.2`).

### Acceptance criteria

- [ ] A proposal runs `CanEnd` (same refusals as today); a single-owner match ends at once; otherwise the proposer's companies are confirmed and the rest wait.
- [ ] Every owner of a waiting company can confirm; the last confirmation ends the match with a history line naming the confirmers; a user with several companies confirms them in one tap.
- [ ] Withdraw, Cancel and "Keep playing" work and show in the log; no timeout; a viewer cannot do any of it (403 on the routes, controls hidden).
- [ ] Override by the organiser or an administrator needs a note, shows the warning, ends the match, and the history names the overridden companies and the note; a plain participant cannot override.
- [ ] Changing the scenario, result, roles or Against the Odds selection while pending drops the other confirmations with a visible notice; a stale revision gives the usual notice.
- [ ] Reopen clears a proposal; an ended match and a finished campaign refuse every end action.
- [ ] 360/768/1280, light and dark, tokens only; the chips carry text and icon, not colour alone.
- [ ] Tests: table tests for `EndAccessFor` and the proposal state machine (propose, confirm, withdraw, cancel, override, invalidation, single owner), service permission tests, handler and template state tests, a postgres integration test.

Beads: `6uh.1` (core, backend), `6uh.2` (database), `6uh.3` (live integration, backend), `6uh.4` (match page, frontend), `6uh.5` (live banner, frontend).

Built (6uh.1-3, backend; `internal/core/match_end.go`, `matchStore.Update`, migration `00023`): the contract above as written, with these details.
`Matches.ProposeEnd/ConfirmEnd/WithdrawEnd/CancelEnd/OverrideEnd` (all `edit`, so the revision check, `MatchEdit` access and the finished-campaign refusal apply; a stranger is ErrRefused). `EndProposal` also has `ProposedByID` and `Notice` (why the confirmations were reset: "Hador changed the result; confirm again"; shown to the others, cleared by their next confirmation) and the helpers `Waiting()`, `WaitingOn(userID)`, `ConfirmedBy(userID)`. `EndAccessFor(u, m, companies, organiserID)` takes the match's companies (`matchCompanies(m)`; the page can pass `m.Sides[*].Companies` flattened). History: `MatchEvent.Detail` is "confirmed by Rangers, Raiders" (company names) or "overriding Raiders: note" (the event's `By` is who pressed last). A withdrawal that leaves nobody confirmed cancels the proposal. Companies with no owner (`OwnerID` 0) can only be overridden. An organiser who owns a waiting company is confirmed, not overridden, by `OverrideEnd`. Scenario, result, roles and Against the Odds changes call `Match.resetEnd` (the proposer's own companies stay confirmed). `Matches.EndNotify` is told about each change (`EndChange{Kind endProposed|endConfirmed|endWithdrawn|endCancelled|endOverride|ended, By, CompanyIDs, Text}`); `cmd/server` points it at the live log. `CancelPendingEnd(ctx, user, id, match, why)` is the live mode's cancel (no revision). Built (6uh.4, page): the match page calls `ProposeEnd`, `ConfirmEnd`, `WithdrawEnd`, `CancelEnd` and `OverrideEnd` (routes and templates in [frontend.md](frontend.md)); `EndMatch` is no longer exported (7k7). Frontend contract for `6uh.4`: add `ProposeEnd`, `ConfirmEnd`, `WithdrawEnd`, `CancelEnd`, `OverrideEnd` to the web `campaigns` interface (already promoted on the facade through `*core.Matches`), render `Match.EndProposal` and `core.EndAccessFor`; a stale revision is `ErrMatchChanged`, `CanEnd`'s reasons are ErrRefused / ErrInvalidMatch.
