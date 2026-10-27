# The War's Aftermath

Contract for what a finished match does to the companies: injury rolls for casualties, experience, level-ups, influence,
scenario rewards and the Against the Odds bonuses. It is the spec of the aftermath page and of the Go types and methods
behind it. The match layer that comes before it (scenario, results, End match) is in [matches.md](matches.md). Status:
**core types, rules, the PostgreSQL-backed service, the company lock and perks are built** (`internal/core/aftermath.go`, `aftermath_service.go`, `aftermath_perks.go`, `aftermath_repository.go`, `internal/postgres/aftermath.go`; beads `csg.12.3` to `csg.12.5`); the pages are being built (beads `csg.12.6` and following).

Rules come from the Battle Companies file (`campaign.matchInput`, `campaign.injuries`, `campaign.experience`,
`campaign.progression`, `campaign.influence`, `campaign.againstTheOdds`, `campaign.aftermathEffects`, scenario `rewards[]`;
see [definitions.md](definitions.md)), never from constants in the code. The order is `campaign.matchInput.aftermath.order`:
**injuries, experience, progression, influence**; scenario rewards follow their `condition`.

The default sheet of a match played in live mode comes from its log ([live-match.md](live-match.md), Aftermath feed).

## Principles

- **One aftermath per company per match**, entered and applied by that company's owner (an administrator or the campaign
  organiser may do it for an absent player, like the match page). Companies are independent: a slow player never blocks
  another company, and a match with several sides has one aftermath per company.
- **Only after the match is ended.** An open match refuses it (`ErrRefused`, "End the match first"). The result (winner,
  Victory Points, Fled) is the match's; the aftermath reads it and never changes it.
- **Guided, with rolls.** Injury rolls are the app's dice (`core.Dice`, `CampaignSettings.RollRefusal` for typed or thrown
  rolls, exactly as the recruit and level-up windows). The player types in what happened at the table (the sheet), the app
  does the arithmetic and asks only for decisions the rules leave to the player.
- **Everything is a transaction on the company** (`companyRepository.Update`, see [architecture.md](architecture.md)), one
  per step, with an audit trail of lines the pages show ("Aldor: Injured", "+2 Influence for a Win").
- **Locked while pending.** From the moment a match ends until the company's aftermath is *done* or *waived*, the company
  cannot recruit, buy, drop, heal or change its roster (`ErrRefused`, "Finish the aftermath of Match N first"). Renaming,
  formation order and level-ups (rolls earned earlier) stay allowed. This keeps the roster the sheet was written for
  (who was absent, who is a hero) and the rule that a model absent from this game recovers *before* the new injuries land.
  An organiser or administrator can **waive** an aftermath (with a note) to unlock a company whose player is gone.
- Applying is **not** redone by editing an ended match (matches.md); `ReopenMatch` is refused with `ErrRefused` once any
  company of the match has *started* its aftermath (REVIEW: simplest safe rule).

## Flow

States of a company's aftermath in a match (`AftermathStatus`): `pending` (nothing saved), `sheet` (draft saved), `injuries`
(started: casualties being rolled), `levelups` (injuries and experience applied; progression rolls may wait), `done`,
`waived`.

1. **Sheet** (`pending`/`sheet`). The player enters, for the company:
   - the **Against the Odds** bonuses it took (only the bonuses `influence`, `experience` and `reroll` have an effect here;
     the others are shown as "no effect on the aftermath"). The page does not check that the company really was the lower
     rated one (REVIEW: shown as a hint with both ratings when two sides, otherwise trusted);
   - for each unit of the roster that took part: `Outcome` (`survived`, `casualty`, `fled` = left the board, no injury
     roll), and the experience gains it earned, one entry per `campaign.experience.gains` key (a flag, or a count for a
     repeatable gain such as `slainHeroOrMonster`); `participated` defaults to true; MVP at most one per company;
     `possessesRelic` (a `modelFlag` of the scenario): the result form already ticks that the company holds it, the sheet says which unit does; exactly one when ticked, none otherwise, never a casualty;
   - units that missed this game (`Injured` or `SentToHealers` when the match ended) are not on the sheet: they are listed
     as "Missed this game, recovers" and get nothing (REVIEW: an absent unit earns no participation XP, as the
     `participated` note says).
   The company's own `Fled` and `Objectives` come from the match result and are shown read only with a link back to the
   match. The sheet shows a **preview** after every save: experience per unit, the influence breakdown, how many injury
   rolls are coming, the scenario rewards that will apply (and which still need a decision).
2. **Start** (`injuries`). Locks the sheet (the player can still *unlock* it until the first roll: `ReopenSheet`). The
   units that missed this game recover (`Injured` and `SentToHealers` cleared). The Against the Odds `reroll` bonus credits
   `CompanyState.Rerolls` (+2). One **casualty** is created per unit with outcome `casualty`.
3. **Injuries.** Per casualty, 2D6 on the chart of its type (`campaign.injuries.warrior[]` or `.hero[]`, `Roll` ranges):
   - Warrior: Dead (leaves the roster for the cemetery, fate "Dead"), Injured (misses the next game, `Injured`), Full
     Recovery (nothing), Lesson Learned (Full Recovery and +D3 Experience, rolled by the server and shown).
   - Hero: Dead (cemetery), Arm Wound / Leg Wound / Broken Honour (the count of that injury rises; at `retireAtCount` the
     hero is forced to retire: cemetery, fate "Retired"; derived effects follow from the counts as in the Heal window),
     Just a Scratch (misses the next game; **decision**: roll again once, the second result stands even if worse, and a
     second Scratch means it misses the next game), Full Recovery (**decision** if the hero has persistent injuries: heal
     one, player's choice, or none), Protection by the Valar (+1 Fate up to the path maximum, `statChanges`), Wounds of a
     Hero (+D6 influence for the company, rolled by the server and shown).
   - **Re-rolls.** While Against the Odds re-rolls remain (`Rerolls` > 0), a rolled but not yet final result offers
     "Re-roll · Against the Odds", like the level-up window; the new result stands. A result is *final* when the player
     moves on to the next decision (REVIEW: no explicit confirm; the re-roll button disappears when every casualty has been
     rolled and all decisions are made, i.e. when the step closes).
   - **Shire's Ranger of the North.** Nothing special happens here when he falls: he goes to the cemetery like any Hero (a fallen
     Leader opens the succession below). The summary shows an informational line that he may return with a reinforcement; the
     return, in an officer place (Leader to start with) with an officer reshuffle, is in [frontend.md](frontend.md) (recruiting).
   - **Death of a Hero.** If the Leader fell (Dead or retired), the page asks which Sergeant becomes Leader (decision
     `succession`). If the company then has fewer than two Sergeants, it asks which other Hero becomes Sergeant (optional,
     "Nobody for now"). A **Sergeant who died or retired** (the Leader living) opens the same Sergeant question
     (`Work.SergeantsToFill` counts the lost Sergeants not yet replaced): a Hero first, optional; with no Hero left, the Warrior
     with the most Experience (ties: highest points, then the player decides; `Succession.Fallback`) must take the place and is
     treated as having rolled Hero in the Making (might, will, fate +1, no hero promotion: stepping up as an officer is not a roll of 6, so a unit keeps its profile, and the company's `heroPromotion` stays a level-up result; it chooses its path in the path window). The same fallback
     applies to a lost Leader. A Warrior is never made a Sergeant just because the Leader was replaced from the Sergeants. Several officers lost in one match are handled one question at a time (Leader first, then one Sergeant question per lost Sergeant; after a Hero fills the place, the Sergeant place stays offered, optional, while there are fewer than two Sergeants and a Hero is free). Retirement counts as death; only units on the active roster are candidates; a company with nobody left to lead has no question. **Frontend contract (6n1.1):** no new state: `Aftermath.Succession` with `NeedSergeant` and `Fallback` set is the mandatory Warrior question (radios of `Fallback`, no "nobody" answer; the `aftermath_succession` / `aftermath_fallback` templates list each Warrior with its Experience and points, preselect a single one, say when several are tied, and tell that the Warrior chooses its path in the company window as a Hero in the Making), `NeedSergeant` with `SergeantOptions` is the optional Hero question.
   When every casualty is final and every decision is made, the server applies **experience** and moves to `levelups`.
4. **Experience** (automatic, at the end of step 3). Each surviving unit that took part gets the sum of its gains
   (`campaign.experience.gains[].amount` times the count) plus the Against the Odds `experience` bonus (+2 for a Win, +1
   otherwise, per participating unit; REVIEW: "per model" read as per participating model) plus scenario experience
   rewards (`survivalOfTheFittest`: +1 to the Leader when `leaderNotRemoved`). The lines list every unit's gain.
5. **Progression** (`levelups`). XP is cumulative and never spent (`campaign.progression.xp`): units that crossed a
   multiple of 5 show the existing "Level Up ready!" banner and are rolled in the existing level-up window (`LevelUpState`,
   `RollLevelUp`...), where Against the Odds re-rolls also work. The step lists the units with rolls waiting and a button for
   each. **Progression is not forced**: the player may continue with rolls waiting (REVIEW: the rules put progression before
   influence but nothing in the app depends on it; rolls stay pending on the cards as today).
6. **Finish** (`done`). Applies influence and rewards, clears `Rerolls` (the bonus `expires` at the end of the aftermath,
   `defaultNotConfirmed`) and unlocks the company:
   - **Influence** (`campaign.influence.gains`): participating 2 always; a Draw +1; a Win +2; the Against the Odds
     `influence` bonus +2 for a Win, +1 otherwise; plus Wounds of a Hero. **All is Lost** (`Fled`): the company gets
     participating and its Against the Odds bonuses (influence and experience), no win and no scenario rewards; the other side is the winner and claims everything.
   - **Scenario rewards** (`scenarioDetails[].rewards[]`, effect = `campaign.aftermathEffects[key]`, only the winner side
     for `victory`, never a fled company): `cargo` (+1 Influence when the `allCargo` tick was reported), `toTheVictorTheSpoils`
     (decision: choose a free equipment item worth at most `maxInfluenceValue` that a unit can take and carry; `allObjectiveMarkers`
     tick), `survivalOfTheFittest` (applied at Experience), and the **perks** `rendezvous` (+1/-1 on the first reinforcement
     roll of the next aftermath), `vantagePoint` (+1/-1 on the next scenario table roll), `aFinalBlessing` (the relic
     bearer gets Fortify Spirit next game, ignoring Will). Perks are stored on the company (`Perk`) and shown until used.
     Gold is never rewarded by a scenario (`campaign.matchInput.gold`).
   - Each scenario reward in Influence or Experience is also recorded as a `Grant` of kind `scenario` with the match id
     (matches.md), so the company's history has one list; participation and result influence are lines of the aftermath only.

A one-off scenario has no rewards: nothing in step 6 but the standard influence.

## Go contract

Types in `internal/core/aftermath.go`; the rule parsing (`ParseAftermath(bcFile)`: experience gains, influence gains, Against
the Odds bonuses, scenario rewards) beside it. Methods go to the `campaigns` interface in `internal/web/server.go`; all take
the acting `core.User`, return `ErrNotFound` (campaign, match or company unknown or the company is not in the match),
`ErrRefused` (reason fit to show: match open, not your company, locked step, finished campaign), `ErrAftermathChanged` (stale
`Revision`, like `ErrMatchChanged`) and `ErrInvalidAftermath` (joined, fit to show) for bad input.

```go
type AftermathStatus string // "pending" | "sheet" | "injuries" | "levelups" | "done" | "waived"

type ModelOutcome string // "survived" | "casualty" | "fled"

// ModelSheet is what happened to one unit at the table.
type ModelSheet struct {
    UnitID  int64
    Outcome ModelOutcome
    Gains   map[string]int // campaign.experience.gains key -> times earned; flags 0 or 1, repeatable gains a count (0-9)
    Relic   bool           // the bearer of the scenario's modelFlag objective (the result form's tick says the company holds it)
}

// AftermathInput is the whole sheet; saved as one document.
type AftermathInput struct {
    AgainstTheOdds []string     // campaign.againstTheOdds.bonuses[].key taken by the company
    Models         []ModelSheet // one per unit of the roster that did not miss this game
}

// Validate checks the sheet against the roster, the rules file and the company's reported result (known units, gain keys and maxima, at most one MVP, one
// Relic exactly when the result ticks `possessesRelic`, not on a casualty, bonuses known). Returns it cleaned or ErrInvalidAftermath.
func (in AftermathInput) Validate(roster []CompanyUnit, rules AftermathRules, result CompanyResult) (AftermathInput, error)

// Casualty is one injury roll and what came of it.
type Casualty struct {
    UnitID    int64
    Rolled    bool
    Dice      []int  // the two dice (last roll)
    Total     int
    ResultKey string // injuries chart key; "" until rolled
    Text      string // the chart's text for the result
    Bonus     int    // the D3 of Lesson Learned or the D6 of Wounds of a Hero, rolled by the server
    Rerolled  bool   // an Against the Odds re-roll was used (the first result is kept in Previous)
    Previous  []string
    Decision  *CasualtyDecision // what the player still has to decide; nil when nothing is open
    Final     bool
}

// CasualtyDecision is an open question. Kind: "scratch" (roll again or accept), "heal" (Options are the hero's injury keys,
// empty answer = none), "succession" (Options are unit ids to promote), "sergeant" (the same, optional).
type CasualtyDecision struct {
    Kind    string
    Options []DecisionOption // Key and Label (a unit name or an injury name)
}

// Aftermath is a company's aftermath in a match, as the page shows it.
type Aftermath struct {
    MatchID, CompanyID int64
    Status             AftermathStatus
    Revision           int
    Outcome            string // "win" | "draw" | "loss" for this company; Fled says All is Lost
    Fled               bool
    Absent             []int64 // units that missed this game
    Input              AftermathInput
    Preview            AftermathPreview // recomputed on read from Input while status is pending/sheet; the applied numbers afterwards
    Casualties         []Casualty
    Pending            []Decision // rewards that need a choice at Finish (free equipment)
    Lines              []AftermathLine
    Access             AftermathAccess
}

type AftermathPreview struct {
    Experience []UnitGain // per unit: Total and the Lines that make it
    Influence  []AmountLine // "Participating +2", "Win +2", "Against the Odds +2", "Cargo +1"...
    Rolls      int          // casualty rolls to come
    Rewards    []AmountLine // scenario rewards and perks that will apply, with the decision they need
}

type AftermathLine struct { Kind, Text string; UnitID int64; Amount int } // Kind: injury, experience, influence, reward, perk, succession, note

type AftermathAccess int // AftermathNone (read only) | AftermathEdit
func AftermathAccessFor(u User, m Match, c CampaignCompany, organiserID int64) AftermathAccess
```

```go
// Aftermath loads the state (a computed default sheet when none is saved: everyone took part and survived).
Aftermath(ctx, user core.User, id, matchID, companyID int64) (core.Aftermath, error)
// AftermathSummary lists, for the match page and the company page, every company's status without the details.
AftermathStatuses(ctx, id, matchID int64) ([]core.AftermathRef, error)      // {CompanyID, CompanyName, Status}
PendingAftermaths(ctx, id, companyID int64) ([]core.AftermathRef, error)    // for the My Company banner and the lock (Match number too)

SaveAftermath(ctx, user, id, matchID, companyID int64, revision int, in core.AftermathInput) (core.Aftermath, error)
StartAftermath(ctx, user, id, matchID, companyID int64, revision int, in core.AftermathInput) (core.Aftermath, error) // saves then starts
ReopenSheet(ctx, user, id, matchID, companyID int64, revision int) (core.Aftermath, error) // only before the first roll
// RollCasualty rolls one casualty (unitID) or all unrolled ones (unitID 0, then entered must be nil). entered holds the two
// dice of a typed roll, thrown says the browser threw them; core.CampaignSettings.RollRefusal applies.
RollCasualty(ctx, user, id, matchID, companyID, unitID int64, entered []int, thrown bool) (core.Aftermath, error)
// RerollCasualty spends one Against the Odds re-roll on a rolled casualty that is not final.
RerollCasualty(ctx, user, id, matchID, companyID, unitID int64, entered []int, thrown bool) (core.Aftermath, error)
// Decide answers the open decision of a casualty (Answer: "reroll"/"keep" for scratch, an injury key or "" for heal, a unit id
// for succession and sergeant) or a pending reward (unitID 0, Key the reward, Answer "<item>|<unit id>" for free equipment).
Decide(ctx, user, id, matchID, companyID int64, d core.AftermathAnswer) (core.Aftermath, error)
// FinishAftermath applies step 6; refused while a casualty is not final or a reward decision is open.
FinishAftermath(ctx, user, id, matchID, companyID int64, revision int) (core.Aftermath, error)
// WaiveAftermath is for the organiser and administrators: no effects, unlocks the company, records the note (1-200).
WaiveAftermath(ctx, user, id, matchID, companyID int64, note string) (core.Aftermath, error)
```

Also, for the company pages (bead `csg.10` brings `Grants`):

```go
type Perk struct {
    ID, MatchID int64
    UnitID      int64     // the relic bearer; 0 for the company
    Effect      string    // campaign.aftermathEffects key
    Text        string
    When        string    // "nextReinforcementRoll" | "nextScenarioRoll" | "nextGame"
    UsedOn      *time.Time
}
// CompanyDetail gets: Perks []Perk (unused first), Aftermaths []AftermathRef (pending ones), History []AftermathLine (latest 20).
UsePerk(ctx, user, id, companyID, perkID int64) error // the owner marks it used (REVIEW: manual; the recruit window may apply Rendezvous later)
```

The lock is a core rule, not a page rule: the company actions (`RollRecruit`, `BuyEquipment`, `TreatInjury`, `DropEquipment`,
`BuyCreature`, `HireWanderer`...) return `ErrRefused` while `PendingAftermaths` is not empty.

## The aftermath page (mobile first, 360 px up)

`/campaigns/{id}/matches/{match}/aftermath/{company}`; readable by every player of the campaign, editable per
`AftermathAccess`. A header (match N, scenario, the company, outcome badge: Win / Draw / Defeat / "Fled (All is Lost)"),
then a **stepper** of four steps (Sheet, Injuries, Level-ups, Finish) where the current one is open, earlier ones collapsed to
their summary line ("3 casualties, 1 Dead") and later ones dimmed. All writes answer with the whole step block (htmx), like
the match page. States for each step: *empty*, *editing*, *read only* (not your company, or step already done), *stale*
(notice "<name> changed this while you were editing", form reloaded), *error* (reason on top or beside the field), *locked*
(match open: "End the match first").

- **Sheet.** One card per unit (the company's order, leader first), mobile first as a single column: name and type; a
  three-way segmented control Survived / Casualty / Fled; a "Took part" switch (default on); the gains as toggle chips in
  the file's wording (short label, full text on the help popup) with a stepper for the repeatable one; MVP and Relic
  chips; touch size 44 px. Above: the Against the Odds block (checkboxes with rating and effect) and the read-only match
  result for the company. A **sticky bottom bar** on phones (inline card from `md`) shows the preview: "XP +N across M units,
  Influence +N, K injury rolls" with **Save** and **Start aftermath** (disabled with the reason when invalid). Absent units
  appear below as "Missed this game, back next match".
- **Injuries.** One card per casualty: unit, the chart type, a Roll button using the dice component (animated, "I rolled"
  when allowed), a **Roll all** button, the chart with the row highlighted, the result text, the decision controls when
  open (radio rows: Roll again / Keep; which injury to heal; which Sergeant to promote), a re-roll button with the
  remaining count. Dead heroes and warriors show a small tombstone note and move to the cemetery when the step closes.
- **Level-ups.** The units with rolls waiting as rows with "Roll" (opens the level-up window as on My Company); the
  step ends with **Continue** (and a note when rolls are left).
- **Finish.** The influence breakdown, scenario rewards, decisions (free equipment: a list of what the company's armoury
  sells up to the allowed value, each with the units that can take it), **Finish aftermath**. Afterwards the page is the
  **summary**: all lines grouped by unit, plus links to My Company and the match.

Match page: after the result card an **Aftermath** card (only when ended) lists each company with its status badge
(Pending / In progress / Done / Waived) and a button (Start or Continue for an editor, View otherwise); the organiser sees
"Waive". The company page and My Company get the banner and the history, see [frontend.md](frontend.md).

## Beads

Epic `csg`, task `csg.12` (this spec). Backend: `csg.12.3` core types, rules parser, pure rules; `csg.12.4`
PostgreSQL service and the company lock; `csg.12.5` perks, free equipment offers and company detail data. Frontend:
`csg.12.6` match page card, `csg.12.7` page shell and Sheet step, `csg.12.1` Injuries step, `csg.12.8` Level-ups and
Finish steps, `csg.12.2` My Company banner and lock messages, `csg.12.9` perks and history on the company pages.

## Built so far (csg.12.3)

- `core.ParseAftermath(bcFile)` -> `AftermathRules` (gains, influence, Against the Odds bonuses, both injuries charts as
  `InjuryRow` with a `Kind`, the `aftermathEffects` and the scenario `rewards[]`). A chart that does not cover 2-12 exactly once
  is `ErrInvalidRuleset`.
- Pure functions, all table-tested: `CompanyOutcome`, `DefaultSheet`, `AftermathInput.Validate`, `Experience`, `Influence`,
  `Rewards`, `Preview`, `RerollsFor`, `ResolveCasualty` / `ResolveScratchReroll` / `CasualtyEffect.Bonus` (D3, D6),
  `NextSuccession` / `NextSergeant`, `AftermathAccessFor`, `CanWaive`.
- Differences from the sketch above: participation is the `participated` gain of the sheet (default 1); `Validate` takes the
  company's `CompanyResult` instead of a `ScenarioInfo`; `AftermathAnswer` has no revision; `Decide` also takes the kind
  `continue` (see below); `Aftermath` carries `Succession *Succession` while the Leader's place or a Sergeant's is open (`NextSergeant(roster, replace)`; `Fallback` is now also set on a Sergeant question, which is then not optional and has no "nobody" answer: frontend contract, the `aftermath_succession` template shows it),
  `Rerolls` and `WaivedNote`. A unit is absent when `CompanyUnit.Injured` is set.
- Rules of the flow (`core.AftermathService`): the injuries step applies its effects when every
  casualty is rolled and no question is open; if Against the Odds re-rolls are left it waits for `Decide(kind "continue")`
  (REVIEW: the spec has no explicit confirm, but without one a re-roll could never be used after "Roll all"); using the last re-roll closes the step on its own (nothing is left to wait for); after the effects
  the Leader's succession (kinds `succession`, `sergeant`; a skipped Sergeant is the answer "") must be settled before the
  experience is given and the status becomes `levelups`. Dead and retired units get no experience; Lesson Learned's D3 is
  applied with the injuries, Wounds of a Hero's D6 Influence at Finish. A casualty on its Scratch re-roll keeps the second
  result, and a second Scratch is plain Injured. Re-rolls are refused once the injuries are applied.
- A match ended in the app starts pending. `ReopenMatch` is refused once a company has started (`Matches.AftermathStarted` is
  wired to the aftermath store in `cmd/server`). Tests inject dice with `core.FixedDice`.

## Built so far (csg.12.6)

The match page shows the **Aftermath card** (`aftermath_card` in `templates/pages/match.gohtml`, `internal/web/aftermath.go`) once
the match has ended: one row per company with the status badge (`aftermath_badge`, `partials/aftermath.gohtml`), **Start** (pending) or
**Continue** for an editor (`core.AftermathAccessFor`), **View** for everybody else, the one-line summary of a finished or waived one
and, for the organiser and administrators on an unfinished one, **Waive** (an inline form with the note, `POST
.../aftermath/{company}/waive`, answered with the whole match). A failure to load the statuses shows an error in the card, not a
failed page. The links point at `/campaigns/{id}/matches/{match}/aftermath/{company}`.

## Built so far (csg.12.7)

`/campaigns/{id}/matches/{match}/aftermath/{company}` (`handleAftermath`, `templates/pages/aftermath.gohtml`): header with the outcome
and status badges, the four-step stepper, and the **Sheet** step (the Injuries, Level-ups and Finish steps are described below). Every write answers with `#aftermath-page`
(`aftermath_body`): `POST .../sheet` (SaveAftermath), `.../start` (StartAftermath, `hx-confirm`), `.../reopen-sheet`. The form
lists `unit` ids, per unit `outcome-<id>`, `took-<id>`, `gain-<id>-<key>` (a count for repeatable gains) and, when the result
ticks the Relic, a single `relic`; Against the Odds bonuses are `odds`. Alpine keeps one MVP and one Relic holder and disables the
chips of a unit that did not take part; Start is disabled with the reason while the Relic is ticked but unassigned. The preview in
the bar (sticky on phones) is the one of the *saved* sheet. Validation messages that start with a unit's title are shown on that
unit's card, the others on top (422, the typed sheet is kept); a stale revision reloads the current state with a notice; a viewer
who may not edit gets the sheet read only and a plain 403 on writes. The web interface gained `AftermathRules(ctx, campaignID)`
(the gains and bonuses the form renders), provided by the PostgreSQL-backed service (`csg.12.4`).

## Built so far (csg.12.4, csg.12.5)

- `core.AftermathService` (`NewAftermathService(campaigns, companies, matches, rulesets, aftermaths, rules, dice)`) implements the
  methods above over `aftermathRepository`. Its `Update` is one transaction that locks the match row (shared, so the match cannot be
  reopened meanwhile), the company row and the aftermath row; the company state, the record (`match_aftermaths`: status, revision,
  `input`, `state`, `lines`, timestamps), the scenario `Grant`s and the new `Perk`s are stored together. Every write bumps the
  revision once. `Bury` and `Injure` take no user, so the service authorizes the caller itself (`AftermathAccessFor`, `CanWaive`) and
  uses the package-private `bury` inside its own transaction.
- **The lock** is `CompanyState.Locked()`: the repository fills `CompanyState.Aftermaths` on load (ended matches whose aftermath is
  not done or waived, no row meaning pending) and `BuyEquipment`, `DropEquipment`, `BuyCreature`, `HireWanderer`, `DismissWanderer`,
  `TreatInjury` and the start of `RollRecruit` return `ErrRefused` "Finish the aftermath of Match N first." inside their own
  transaction. Renaming, formation, level-ups, the Baggage Pony's pack and the rest of a recruit already in progress stay allowed.
  The migration marks the companies of matches that had ended before as done.
- `AftermathService.Started` is what `Matches.AftermathStarted` is set to by the wiring; a waived aftermath counts as started too.
- Effects: absent units recover at Start (undone by `ReopenSheet`), injuries count up (`Injuries`), a second arm wound or leg wound
  buries the hero as "Retired", Dead buries as "Dead", Fate rises through the path's limits (`Progress.Bonus["fate"]`), Lesson Learned
  adds XP, Wounds of a Hero adds Influence at Finish. A fallen Leader's place is chosen in `Decide`; with no hero left, the chosen
  Warrior becomes Leader with the officer's start (1 Might, Will and Fate, and its company's hero promotion) and chooses its path in the usual path window.
- **Perks and free equipment** (csg.12.5): `Perk` rows (`company_perks`) are created at Finish for the winner only (the relic perk
  bound to its bearer); `UsePerk` is for the owner; `CompanyDetail` carries `Aftermaths`, `Perks` and the latest 20 `History` lines
  (`CompanyService.WithAftermaths`). `FreeEquipmentOffers(id, companyID, maxInfluenceValue)` lists armoury items worth at most the
  value that a unit can take and carry (the armoury's legality without the price); `Decide` checks the choice with it and Finish
  checks it again. `ScenarioObjective.Model` tells the result form that the objective is a unit's (ticked company-wide there, the
  sheet picks the unit).

## Built so far (csg.12.1)

The **Injuries** step (`aftermath_injuries`, `aftermath_casualty`, `aftermath_decision`, `aftermath_succession`): one card per casualty
with the dice component (`rollPanel`, `throwCasualty` in `static/js/company.js`), **Roll 2D6** (typed dice need ManualRolls, thrown
ones ShakeTheTable; `POST .../roll` with `unit`, `roll`, `roll2`, `thrown`), **Roll all** when two or more wait (the server rolls; the
form reports `unrolled` so only those dice animate), the result with its bonus die, the chart (collapsed, the row of the roll
highlighted), a tombstone note for the fallen, the open decision (`POST .../decide` with `kind`, `unit`, `answer`: scratch roll again or
keep, which injury to heal or none), **Re-roll · Against the Odds** (`POST .../reroll`) with the count left, and the Leader's succession
and new Sergeant questions. **Continue** (`kind=continue`) appears when everything is rolled and decided and re-rolls are left. Once
the step closes it collapses to the results and the experience and succession lines. State lives in the core, so a reload restores it.
Only editors get the buttons; others see the cards read only.

## Built so far (csg.12.8)

**Level-ups** (`aftermath_levelups`): the units with rolls waiting, each with a **Level up** button that opens the My Company
level-up window in a modal on this page (same endpoints and `level_panel`; only the owner of the company has the buttons, others are
told who rolls). Closing the window reloads the page. **Continue to Finish** is always allowed (rolls stay waiting on the unit
cards); the core has no status between level-ups and done, so this is an Alpine flag (`finish`), open from the start when no roll
waits and kept open by the server after a reward decision or a refused Finish. **Finish** (`aftermath_finish`): the influence
breakdown, the scenario rewards, the free-equipment decision (items from `FreeEquipmentOffers` with the units that can take each;
`POST .../decide kind=reward key=<reward> answer="<item>|<unit id>"`) and **Finish aftermath** (`POST .../finish`, disabled while a
decision is open; a refusal is shown on the step). Afterwards the page is the **summary**: the audit lines grouped by unit (the
company first) with links to My Company and the match, readable by everybody. The web interface gained
`FreeEquipmentOffers(ctx, id, companyID, maxInfluenceValue)` (`core.AftermathService` implements it).

## Wargear on a promotion

One rule (`core/promotion_gear.go`): a warrior who is promoted keeps only the wargear his new profile lists, under any name the armoury
recognises as the same item (`ItemKey`, the armoury's other names and the annotations' aliases such as Two-handed axe = Two-handed weapon,
`wargearReadAs`); the rest is lost for good. The new profile lists its options, its own wargear, what its company label selects and what
the promotion gives. Equipment (slot items) stays, and a unit that becomes a hero keeps everything. The points follow from what is left.
Different weapons are different items (a Wood Elf Spear, an Elven glaive and an Elven spear are three).

Every carried item is judged on its own. A stored option label stands for the items it carries: "Shield and Spear" is two items, and an
exchange ("Exchange hand-and-a-half weapon for bow") carries only the item it gives, since the other was exchanged away. A carried item
the new profile lists stays as itself; when the new profile has an exchange option that gives it, that exchange is already chosen and
the entry becomes that option; when the new profile carries it as its own wargear the profile supplies it (a Khandish Warrior with the
bow becomes a Khandish Horseman, who lists the hand-and-a-half weapon and the bow, and has both).

Poison is no property of wargear (docs/definitions.md **Poison**): the plain Bow stays when the new profile lists it, and the unit is
poisonous only if the new profile has Poisoned Attacks (a Haradrim Raider has it, a Khandish Horseman does not). An Envenom Weapon keeps its chosen
weapon whether or not the new profile lists it: the choice is final and the rule `Poisoned Attacks (<weapon>)` belongs to the weapon's
name, so the unit keeps it; the player may buy the weapon again if the new profile offers it.
`TestEveryPromotionAppliesTheOneRule` applies the rule to every promotion of every company, with no per-item list.

## Unit titles on the aftermath

Cards, casualties, succession options and every audit line use `core.Distinguish(roster)`: a Leader or Sergeant carries its role
("Mordor Orc Warrior (Leader)"), units that still share a title are numbered in the company's order ("Warrior of Rohan 2"),
and a name the player gave stays as it is. The titles are for display only (never stored on the units).

Level-ups step: **Continue to Finish** is shown to editors only (readers see the Finish block open); once the aftermath is done the
step reads "N units still have a level-up roll waiting" when rolls remain, not "Done.".

The collapsed Injuries step says what each settled result did beside it: "goes to the cemetery" (Dead), "forced to retire by its
injury number N, goes to the cemetery", "misses the next game", "healed one Arm Wound" / "nothing healed" (Full Recovery).

The bar under the sheet also lists the scenario rewards that will apply (from `Preview.Rewards`), e.g. "Survival of the Fittest:
+1 Experience to <Leader>", so the player sees them before Finish.

The History card on My Company and the company page groups the lines by match ("Match 4", newest first); the repository takes whole
aftermaths until it has at least 20 lines, so one is never cut in the middle. Every `AftermathLine` is filled with `Match` (number), `MatchID` and `On` (finished, else started, else the match's end) when it is read, both in `History` and in `Aftermath.Lines`; they are not stored in the lines JSON.

The lock message names every pending aftermath ("Finish the aftermath of Match 3 first." or "Finish the aftermaths of Matches 3 and 4
first."): `core.LockMessage`, used by the refusals and by the My Company page.

The Armory tab of a locked company shows "The company is locked. <reason>" with a link to My Company, and its Buy / Hire controls are
disabled with the reason as their title (My Company's controls already carry `data-lock`). A free item from "To the Victor Goes the
Spoils" is stored like a bought one (origin "bought"); a distinct origin would need a new equipment origin in the unit state.

**Typed dice.** With `ManualRolls` on, the Against the Odds re-roll takes the two dice the player rolled (the re-roll form has the same
two selects as the roll; "Roll for me" lets the server roll; `RollRefusal` applies), and the bonus die of Lesson Learned (a D3) and
Wounds of a Hero (a D6) is asked for after the 2D6 as a decision of kind `bonus` (`Decide`, Answer "0" for the server or "1" to "6"
for the die the player rolled; a D3 reads 1-2 as 1, 3-4 as 2, 5-6 as 3). With `ManualRolls` off the server rolls the bonus die at once
as before. A re-roll that lands on one of those results asks for its own bonus die. Thrown dice (ShakeTheTable) are not offered for the
re-roll or the bonus die.


## Live end checks and perks in the rolls (spec, designed)

- **Objective and MVP** come from the live match ([live-match.md](live-match.md) "End checks"): `AftermathDefaults` prefills `objective` on each unit marked and `mvp` on the voted unit (a tie prefills none and lists the tied units). The sheet still has one MVP at most per company, chosen by the owner; saved sheets never re-read the log.
- **Perks +1/-1 in the rolls** (replaces the manual "Mark used" for these two; bead `mtu.10`): *Rendezvous* is an aftermath bonus right after the match (bead `mtu.1` moves it from "next match"): in the recruit roll panel of that aftermath, when the company holds an unused `rendezvous` perk (including one granted earlier in the same aftermath), show a one-time **Rendezvous: -1 / 0 / +1** selector next to "raise the die with Influence". It is free, applied to the die before the chart lookup, written in the roll line ("5 + Rendezvous +1 = 6"), and `UsePerk` runs inside the same transaction as the roll. Only the first reinforcement roll of the aftermath offers it. *Vantage Point*: in the scenario-table roll on the match page, after both dice are known (typed or thrown) the second die can be adjusted **+1 / -1** by a company in the match that holds an unused Vantage Point; marked used on save, written in the roll line. A Final Blessing stays a card note. Core: `RecruitInfo.Perk`, a `PerkAdjust int` parameter (-1, 0, +1; others `ErrInvalidRoll`) on `RollRecruit` and the scenario roll, double use refused (`ErrRefused`). Tests per perk and the double-use refusal.

Built (mtu.25, backend): perks in the rolls. `Perk.When` carries the data's `when` (`firstReinforcementRollThisAftermath` for Rendezvous, `nextScenarioTableRoll` for Vantage Point). `CompanyState.Perks` holds the company's unused perks (filled on load) and `CompanyState.PerksUsed` the ones a change uses up, marked in the same transaction (refused when used already). `RecruitService.RollRecruitWithPerk(..., perkAdjust)` (-1, 0, +1; others `ErrInvalidRoll`; needs an unused `rendezvous`; only the roll that pays for a new recruit takes it; the summary reads "You rolled a 5, Rendezvous +1 = 6"; `RollRecruit` is the same with 0) and `AftermathService.UseVantagePoint(user, id, matchID, companyID, second, adjust)` (+1 or -1, the second die stays on 1-6, the company must play in the open match, returns the moved die and the line "3, Vantage Point +1 = 4"). `core.UnusedPerk(perks, effect)` finds the perk; `CompanyDetail.Perks` (from `WithAftermaths`) tells the page whether to show the selector. Web: the Rendezvous selector in the recruit panel of the aftermath and the Vantage Point +1/-1 on the scenario roll (`handleScenarioRoll` calls `UseVantagePoint` when the player confirms the adjustment).

Contract additions (frontend): `Aftermath.MVP *MVPHint{Winner, Tied, Lines}` is the live MVP vote tally of the company (nil without votes): the winner is prefilled in the sheet, on a tie show "Tied: ..." from `Tied` and let the owner choose; `RecruitRoll.PerkAdjust` / `PerkRaw` let the recruit window badge a die moved by Rendezvous ("5, Rendezvous +1 = 6").

## Healing Herbs, Wondrous Cram and the Against the Odds items (core built: 36y.4; screens and Against the Odds designed; epic `36y`)

Two consumables are used in the aftermath, before any injury roll: **Healing Herbs** (a Hero that took part and is not a casualty;
every Injury roll of the company gets +1) and **Wondrous Cram** (a casualty that carries it skips its roll and counts as
Full Recovery). Rules data: `campaign.aftermathEffects[key=healingHerbs|wondrousCram]`, `campaign.injuries.rollModifier`,
`matchInput.perModel[key=usesItem]` ([definitions.md](definitions.md)). Every reading below is the curator's recommended one;
each is a `REVIEW:` for the owner (register: `docs/review-register.md`).

### Readings used as defaults (REVIEW)

- **Election is part of the sheet**, so it happens before the first roll: `ModelSheet.UsesItem` is `""`, `"healingHerbs"` or
  `"wondrousCram"`. `ReopenSheet` (before the first roll) can still change it.
- **Healing Herbs.** One Hero carrier, `participated`, outcome `survived` or `fled` (a model that fled the board counts as not a
  casualty; REVIEW R1). Applies to the 2d6 total of **every** injury roll of the company that aftermath (Warriors and Heroes),
  also the Hero's "Just a Scratch" roll again and an Against the Odds re-roll (R3). The total is clamped to 12 (R2); a
  modified 12 is a real 12. **Not cumulative**: two Herbs (Healer's Kit has two) never give +2, and only the elected one is consumed.
- **Wondrous Cram.** The casualty itself (`outcome casualty`, `participated`), any model, any company. No roll: the casualty is
  created already rolled with the chart's `fullRecovery` key, `Final`, `Dice` empty, text "Wondrous Cram: Full Recovery". A
  Hero's Full Recovery still offers the "heal one persistent injury" decision (R4); Dead, retirement, Valar and Wounds of a Hero
  cannot happen. A fled model has no roll to cancel, so Cram on `fled` is refused.
- **Carriers.** The item is in the model's Equipment, or loaned to it by a Healer's Kit ([matches.md](matches.md), Against the
  Odds). Herbs from the kit still go to Heroes only (R5, T3). The temporary Wanderer is never a carrier.
- **Consumed at Finish**, not before: the used item leaves the model's equipment (its Small slot is free again) as one aftermath
  line "Aldor used Healing Herbs". An item that was loaned is simply spent. Unused items stay. Waiving an aftermath consumes nothing.

### Go contract

```go
// ModelSheet gets one field.
//   UsesItem string // "" | "healingHerbs" | "wondrousCram" (aftermathEffects key)

// ItemUse is the validated election, derived from the sheet.
type ItemUse struct {
    Effect string // aftermathEffects key
    UnitID int64
    Loaned bool   // came from a Healer's Kit
}

// ItemChoices lists what the sheet may offer: per unit the effects it can elect right now (carries the item, outcome and role allow it).
// Pure; used by the page to show the chips and by Validate.
func (in AftermathInput) ItemChoices(roster []CompanyUnit, loaned []LoanedItem, rules AftermathRules) map[int64][]string

// Validate (existing) additionally rejects, with ErrInvalidAftermath and a sentence fit to show:
//   more than one healingHerbs in the company ("Healing Herbs do not stack"); Herbs on a non-Hero, a casualty or a unit that did not
//   take part; Cram on a unit that is not a casualty; an item the unit does not carry.

// Casualty gets
//   Modifier int    // +1 when Herbs applied to this roll (shown "9 +1 = 10")
//   Item     string // "wondrousCram" when the roll was skipped
// and the dice total used for the chart is min(12, total+Modifier); the stored Total is the modified one, Dice the raw dice.

// AftermathPreview gets Items []AmountLine ("Healing Herbs: +1 to every injury roll", "Wondrous Cram on Aldor: no roll"); Rolls excludes Cram casualties.
```

No new service methods: `SaveAftermath` / `StartAftermath` carry the election in `AftermathInput`, `RollCasualty` /
`RerollCasualty` apply the modifier, `FinishAftermath` consumes the items. Errors as the existing ones. Loaned items
(Healer's Kit) come from the match's Against the Odds record, see the next section.

Built (36y.4, backend): `ParseAftermath` reads the consumables from the file: `AftermathRules.ItemEffects` (equipment name, lower case, to the `aftermathEffect` key of `data.equipment.items[]`) and, per effect, `RewardEffect.Modifier`, `ClampMin/ClampMax`, `Stackable`, `Carrier`, `Requires`, `Result`, `NoRoll` (an item naming an unknown effect makes the ruleset invalid). Nothing names Healing Herbs or Wondrous Cram in code. `ModelSheet.UsesItem` is stored in the sheet JSON (no migration). `AftermathInput.ValidateWith(roster, loaned, rules, result)` (`Validate` is the same with no loaned items) refuses with `ErrInvalidAftermath`: an unknown election, an item the unit does not carry, a Hero-only effect on a non-Hero, a missing `participated`, `casualty` or `notCasualty`, and a second non-stackable item ("Healing Herbs do not stack"). `LoanedItem{UnitID, Item}` is what 36y.6 will feed from a Healer's Kit. `AftermathInput.ItemChoices(roster, loaned, rules) map[unitID][]effectKey` is the list for the chips (pure; stacking is left to the page). `RollModifierOf(rules, in).Apply(total)` gives the modified total and the shown modifier; `AftermathService.roll` uses it for every roll, the Scratch roll again and an Against the Odds re-roll. `Casualty.Modifier` (shown `9 +1 = 10`; `Dice` stay the raw dice, `Total` is the modified one) and `Casualty.Item` (the effect key when the unit did not roll). A Cram casualty is made at Start: `Rolled`, no dice, `Total` the first value of the chart's `Result` row, `Kind` the row's (a Hero's Full Recovery keeps its `heal` decision), `RerollCasualty` refuses it, and `AftermathPreview.Rolls` excludes it. `AftermathPreview.Items` carries the lines ("Healing Herbs: +1 to every injury roll", "Wondrous Cram on Fen: no roll"). At Finish `consumeItems` removes exactly the elected item from the carrier's bought or starting gear (a loaned one is simply spent) and writes "Aldor used Healing Herbs" (Cram appends ": Full Recovery"); a waived aftermath consumes nothing. REVIEW: the line says "used" for both items, not "ate".

### Screens (aftermath page)

- **Sheet.** On a unit card that can elect an item, a switch row below the gains: "Use Healing Herbs (+1 to every injury roll
  of the company)" or "Use Wondrous Cram (skips the injury roll: counts as Full Recovery)" with the effect text in the help
  popup. Only offered when `ItemChoices` allows it; once one unit has the Herbs on, the other Herbs switches show disabled
  "Already used by Brand (they do not stack)". A carrier that does not qualify shows the reason in muted text ("Casualties
  cannot use Healing Herbs"). The preview line gains "Items: Herbs +1, 1 Cram". Read-only for viewers (chips shown as
  set), locked after Start. States: no item anywhere (nothing shown), election saved, election invalid (inline error).
- **Injuries.** Herbs: a banner at the top of the step "Healing Herbs: +1 to every roll"; every roll card shows `9 +1 = 10`
  in the result and the chart row of the modified total is highlighted. A Cram casualty card has no Roll button, a badge
  "Wondrous Cram" and the Full Recovery row highlighted (the hero heal decision, when open, is as usual); it is already `Final`.
- **Finish / summary.** Lines "Aldor used Healing Herbs" / "Brand ate Wondrous Cram: Full Recovery"; items listed as removed.

### Acceptance criteria

- [ ] Herbs: only a participating non-casualty Hero carrier can elect; two carriers allow only one; every roll (Warrior and
  Hero, including the Scratch roll again and re-rolls) gets +1, clamped to 12; the item is gone from the carrier only after Finish.
- [ ] Cram: only on a casualty that carries it; no roll, Full Recovery, never Dead or retired, hero heal decision still offered;
  Cram on a fled or surviving unit is refused.
- [ ] Items loaned by a Healer's Kit are electable and vanish at Finish; kit Herbs on a Warrior are refused.
- [ ] `ReopenSheet` before the first roll can change the election; after the first roll it cannot.
- [ ] Waiving consumes nothing. The preview and the lines say what happened. 360 px, light and dark, tokens only.
- [ ] Tests: validation table (every refusal), modifier table (2-12 clamp, scratch re-roll, AtO re-roll), Cram casualty never rolls,
  two Herbs consume one, Finish removes exactly the used item and nothing else, template states.

### Against the Odds in the aftermath (what changes)

The sheet's Against the Odds block is **prefilled from the match's selection record** ([matches.md](matches.md)) with the badge
"From the match"; the player can still change it (players are trusted). Aftermath effects of the three new bonuses:

- **Tool Kit**: every loaned item still unused is dropped at Finish; Healer's Kit items are electable in the sheet (above).
  No Rating, no equipment ever enters the roster. REVIEW T2: unused kit items are not kept.
- **Temporary Wanderer**: not on the sheet, no injury roll, no experience, no progression, no relic/MVP; it leaves at Finish
  (REVIEW W3). Never counts for the 15-model cap.
- **Ambush!**: nothing in the aftermath (it only chose the scenario).
- **All is Lost** (`Fled`): unchanged and already built (user decision 2026-10-21, mqn.114): the company earns the participation Influence **and** its Against the Odds Influence and experience bonuses, no Win/Draw Influence, no scenario reward; injuries and experience resolve. This overrides the curator's "participation only" reading (B3). The other side is the winner with full rewards. The epic only feeds the `Fled` field from the live match.

Built (36y.11, backend): `AftermathService.WithATO(repo)` (wired in `cmd/server`) reads the match's Against the Odds record for the company that took it. **Prefill**: while no sheet is saved, `Aftermath.Input.AgainstTheOdds` is the record's bonuses that do something in the aftermath (influence, experience, reroll: `OddsBonus.Effective()`), `Aftermath.FromMatch` is true ("From the match" badge) and the preview counts them; once the player saves or starts, the saved sheet wins and the badge goes. The Tool Kit, Wanderer and Ambush! bonuses have no aftermath effect and are not ticks. **Kit items**: the assigned items (`ATORules.LoanedFor`) are the `loaned` argument of `ValidateWith` and `ItemChoices`, so a Hero can elect the kit's Healing Herbs and a casualty the kit's Wondrous Cram; `Aftermath.ItemChoices map[unitID][]effectKey` is what the sheet can offer right now (it follows the sheet's outcomes, so Cram shows once the unit is a casualty; empty after Start). A loaned item was never on the roster, so Finish writes the line ("Hador used Healing Herbs") and removes nothing; unused loaned items simply stay unused and disappear with the match (REVIEW T2). **Temporary Wanderer**: never on the sheet (it is not on the roster and `AftermathDefaults` skips it), so no roll, no experience, no progression; nothing to drop at Finish. **All is Lost** is untouched: `TestAllIsLostKeepsAgainstTheOddsBonusesWithTheSelection` pins participation plus the Against the Odds Influence and experience, no Win/Draw, no scenario rewards. Frontend (36y.5): show the badge from `FromMatch`, the chips from `ItemChoices`.

Built (36y.5, web): the sheet form gives each unit that carries (or is loaned) a consumable a checkbox row below the gains (`aftermathView.itemRows`: carrier and `requires` decide when it is enabled; the reason shows in muted text, a second Herbs shows "Already used by X (they do not stack)"); the election is a hidden `item-<unitID>` field read by `sheetFromForm` into `ModelSheet.UsesItem`. Unsaved outcome changes drop an election the new outcome forbids. The "From the match" badge sits on the Against the Odds legend, the preview line gains "Items: ...", the read-only sheet shows "Uses Healing Herbs". Injuries: a banner "Healing Herbs: +1 to every injury roll (the total stops at 12)", result "Rolled 9 +1 = 10 (4 + 5)", a Cram card ("Wondrous Cram No roll: Full Recovery", no Roll button). Loaned kit items appear only after the sheet is saved with the outcome that allows them (`Aftermath.ItemChoices` follows the saved sheet).

### Beads (epic `36y`)

Backend `36y.4` (Herbs/Cram core), `36y.11` (Against the Odds in the aftermath); frontend `36y.5` (sheet and injuries UI). See the
epic for the whole list and its dependencies.
