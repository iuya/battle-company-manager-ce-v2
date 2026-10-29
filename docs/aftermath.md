# The War's Aftermath

What a finished match does to the companies: injury rolls for casualties, experience, level-ups, influence, scenario rewards and the
Against the Odds bonuses. It describes the aftermath page and the Go types and methods behind it. The match layer that comes before it
(scenario, results, ending) is in [matches.md](matches.md); the default sheet of a match played in live mode comes from its log
([live-match.md](live-match.md)).

Code: `internal/core/aftermath.go`, `aftermath_service.go`, `aftermath_perks.go`, `aftermath_items.go`, `aftermath_repository.go`,
`promotion_gear.go`; `internal/postgres/aftermath.go`; `internal/web/aftermath.go` with `templates/pages/aftermath.gohtml` and
`partials/aftermath.gohtml`.

Rules come from the Battle Companies file (`campaign.matchInput`, `campaign.injuries`, `campaign.experience`, `campaign.progression`,
`campaign.influence`, `campaign.againstTheOdds`, `campaign.aftermathEffects`, scenario `rewards[]`; see [definitions.md](definitions.md)),
never from constants in the code. The order is `campaign.matchInput.aftermath.order`: **injuries, experience, progression, influence**;
scenario rewards follow their `condition`. `core.ParseAftermath` reads them into `AftermathRules`; a chart that does not cover 2-12 exactly once is `ErrInvalidRuleset`.

## Practice matches

The aftermath of a **practice match** ([matches.md](matches.md#practice-matches)) is complete and empty from the moment the match ends, and nothing is stored for it. It is **derived**: the repository reads
the status of a company in a practice match as `done` (`CASE WHEN m.practice THEN 'done' ELSE COALESCE(a.status, 'pending') END` in `pendingRefs`, the company read, `Statuses` and the campaign's pending count), so
`AftermathStatus.Locks()` is false and the lock, "cannot start a match" and the organiser's pending figure need no special case. `aftermathStore.Get` returns the record `done`, revision 1, no input, one line
"Practice match: nothing to roll." and `FinishedAt` = the match's end; no `match_aftermaths` row is ever written (`aftermathStore.Update` refuses a practice match too).

`AftermathService.Aftermath` returns that record (`Aftermath.Practice`, no rules or company read), and every write (`SaveAftermath`, `StartAftermath`, `RollCasualty`, `RerollCasualty`, `Decide`, `FinishAftermath`,
`ReopenSheet`, `WaiveAftermath`) answers `ErrPracticeNothingToRoll` ("Practice match: nothing to roll.") before any rule is read and without touching the company: roster, resources, `Injured` flags, cemetery,
experience and influence are the same before and after. The page `GET .../aftermath/{company}` shows the short state (badge, the line, "Nothing was awarded and nobody was injured.", a link back to the match; no sheet,
stepper or waive); a write route answers 422 with the line for an editor and 403 for a reader. `LiveMatches.AftermathDefaults` is never read for it.

## Principles

- **One aftermath per company per match**, entered and applied by that company's owner (an administrator or the campaign organiser may do
  it for an absent player). Companies are independent: a slow player never blocks another company.
- **Only after the match is ended.** An open match refuses it ("End the match first"). The result (winner, Victory Points, Fled) is the
  match's; the aftermath reads it and never changes it.
- **Guided, with rolls.** Injury rolls are the app's dice (`core.Dice`, `CampaignSettings.RollRefusal` for typed or thrown rolls, as in the
  recruit and level-up windows). The player types in what happened at the table (the sheet), the app does the arithmetic and asks only for
  decisions the rules leave to the player.
- **Every step is a transaction on the company** ([architecture.md](architecture.md)), with an audit trail of lines the pages show ("Aldor:
  Injured", "+2 Influence for a Win").
- **Locked while pending.** From the moment a match ends until the company's aftermath is *done* or *waived*, the company cannot recruit,
  buy, drop, heal, hire or dismiss (`ErrRefused`, "Finish the aftermath of Match N first."; several: "Finish the aftermaths of Matches 3
  and 4 first.", `core.LockMessage`). Renaming, formation, level-ups, the Baggage Pony's pack and the rest of a recruit already in
  progress stay allowed. This keeps the roster the sheet was written for. The organiser or an administrator can **waive** an aftermath (with a note of 1-200 characters) to unlock a
  company whose player is gone; a waived aftermath consumes nothing.
- Editing an ended match does not redo the aftermath, and `ReopenMatch` is refused once any company of the match has *started* it (a
  waived one counts as started: `Matches.AftermathStarted`).

The lock is `CompanyState.Locked()`: the repository fills `CompanyState.Aftermaths` on load (ended matches whose aftermath is not done or
waived; no row means pending) and the company actions return `ErrRefused` inside their own transaction. The Armory tab of a locked company
shows "The company is locked." with a link to My Company, and its Buy and Hire controls are disabled with the reason as their title.

## Flow

States (`AftermathStatus`): `pending` (nothing saved), `sheet` (draft saved), `injuries` (started: casualties being rolled), `levelups`
(injuries and experience applied; progression rolls may wait), `done`, `waived`.

1. **Sheet** (`pending`, `sheet`). The player enters for the company:
   - the **Against the Odds** bonuses it took, prefilled from the match's record (see below); only `influence`, `experience` and `reroll`
     have an effect here, the others read "no effect on the aftermath";
   - for each unit of the roster that took part: its `Outcome` (`survived`, `casualty`, `fled` = left the board, no injury roll) and the
     experience gains it earned, one entry per `campaign.experience.gains` key (a flag, or a count for a repeatable gain such as
     `slainHeroOrMonster`). `participated` defaults to on; at most one MVP per company; `possessesRelic` (a `modelFlag` of the scenario):
     the result form ticks that the company holds it and the sheet says which unit does, exactly one when ticked, never a casualty;
   - units that missed this game (`Injured` or `SentToHealers` when the match ended) are not on the sheet: they are listed as "Missed this
     game, recovers" and earn nothing.
   The company's own `Fled` and `Objectives` come from the match result and are shown read only with a link back. The sheet shows a **preview**
   after every save: experience per unit, the influence breakdown, how many injury rolls are coming, the scenario rewards that will apply.
2. **Start** (`injuries`). Locks the sheet (the player can still unlock it until the first roll: `ReopenSheet`). The units that missed this
   game recover. The Against the Odds `reroll` bonus credits `CompanyState.Rerolls` (+2). One **casualty** is created per unit with outcome `casualty`.
3. **Injuries.** Per casualty, the injury dice (`campaign.injuries.roll`, 2D6 in the shipped file) on the chart of its type
   (`campaign.injuries.warrior[]` or `.hero[]`):
   - Warrior: Dead (leaves the roster for the cemetery, fate "Dead"), Injured (misses the next game), Full Recovery (nothing), Lesson Learned (Full Recovery and +D3 Experience).
   - Hero: Dead (cemetery); Arm Wound, Leg Wound and Broken Honour (the count of that injury rises; at `retireAtCount` the hero is forced to
     retire: cemetery, fate "Retired"; derived effects follow from the counts as in the Heal window); Just a Scratch (misses the next game;
     **decision**: roll again once, the second result stands even if worse, and a second Scratch means it misses the next game); Full
     Recovery (**decision** if the hero has persistent injuries: heal one, the player's choice, or none); Protection by the Valar (+1 Fate up to the path maximum); Wounds of a Hero (+D6 influence for the company).
   - **Re-rolls.** While Against the Odds re-rolls remain (`Rerolls` > 0), a rolled result that is not final offers "Re-roll · Against the
     Odds"; the new result stands. A result is final when the player moves on to the next decision. Re-rolls are refused once the injuries are applied.
   - **Death of a Hero.** If the Leader fell (Dead or retired), the page asks which Sergeant becomes Leader (decision `succession`). If the
     company then has fewer than two Sergeants it asks which other Hero becomes Sergeant (optional, "Nobody for now"). A Sergeant who died
     or retired (the Leader living) opens the same question (`Work.SergeantsToFill`). With no Hero left, the Warrior with the most
     Experience (ties: highest points, then the player decides; `Succession.Fallback`) must take the place and is treated as a Hero in the
     Making (Might, Will, Fate +1, no hero promotion; it keeps its profile and chooses its path in the path window). Several officers lost in one
     match are handled one question at a time (Leader first); retirement counts as death; only units on the active roster are candidates; a
     company with nobody left to lead has no question. The Warrior question has no "nobody" answer: `Aftermath.Succession` with `NeedSergeant`
     and `Fallback` set is the mandatory question (radios of `Fallback`; the templates `aftermath_succession` and `aftermath_fallback` list each
     Warrior with its Experience and points, preselect a single one and say when several are tied), `NeedSergeant` with `SergeantOptions` the optional Hero question.
   - **The Shire's Ranger of the North.** Nothing special happens when he falls: he goes to the cemetery like any Hero (a fallen Leader opens the succession). The summary says he may return with a reinforcement; see "Ranger of the North replacement" in [frontend.md](frontend.md).
   - When every casualty is final and every decision is made, the server applies **experience** and moves to `levelups`. If re-rolls are left,
     the step waits for `Decide` with kind `continue`; using the last re-roll closes the step on its own.
4. **Experience** (automatic). Each surviving unit that took part gets the sum of its gains (`campaign.experience.gains[].amount` times the
   count) plus the Against the Odds `experience` bonus (+2 for a Win, +1 otherwise, per participating unit) plus scenario experience rewards
   (`survivalOfTheFittest`: +1 to the Leader when `leaderNotRemoved`). Dead and retired units get none.
5. **Progression** (`levelups`). XP is cumulative and never spent (`campaign.progression.xp`): units that crossed a multiple of 5 show the
   "Level Up ready!" banner and are rolled in the level-up window (`LevelUpState`, `RollLevelUp`...), where Against the Odds re-rolls also
   work. Progression is not forced: the player may continue with rolls waiting; they stay pending on the cards.
6. **Finish** (`done`). Applies influence and rewards, clears `Rerolls` (the bonus expires at the end of the aftermath), consumes the elected items and unlocks the company:
   - **Influence** (`campaign.influence.gains`): participating 2 always; a Draw +1; a Win +2; the Against the Odds `influence` bonus +2 for a
     Win, +1 otherwise; plus Wounds of a Hero. **All is Lost** (`Fled`): the company gets participating and its Against the Odds bonuses
     (influence and experience), no Win or Draw influence and no scenario rewards; the other side is the winner and claims everything.
   - **Scenario rewards** (`scenarioDetails[].rewards[]`, effect = `campaign.aftermathEffects[key]`, only the winner side for `victory`, never a
     fled company): `cargo` (+1 Influence when the `allCargo` tick was reported), `toTheVictorTheSpoils` (decision: choose a free equipment item
     worth at most `maxInfluenceValue` that a unit can take and carry, listed by `FreeEquipmentOffers`; the `allObjectiveMarkers` tick; stored like a bought item),
     `survivalOfTheFittest` (applied at Experience), and the **perks** `rendezvous`, `vantagePoint` and `aFinalBlessing` (the relic bearer
     gets Fortify Spirit next game, ignoring Will). Perks are stored on the company (`Perk`, table `company_perks`) for the winner only and shown until used. Gold is never rewarded by a scenario.
   - Each scenario reward in Influence or Experience is also recorded as a `Grant` of kind `scenario` with the match id ([matches.md](matches.md)), so the company's history has one list; participation and result influence are lines of the aftermath only.

A one-off scenario has no rewards: only the standard influence applies.

## Healing Herbs, Wondrous Cram and the Against the Odds items

Two consumables are used in the aftermath, before any injury roll. Rules data: `campaign.aftermathEffects[key=healingHerbs|wondrousCram]`,
`campaign.injuries.rollModifier`, `matchInput.perModel[key=usesItem]`; `ParseAftermath` reads `AftermathRules.ItemEffects` (equipment name to effect
key) and per effect `Modifier`, `ClampMin/ClampMax`, `Stackable`, `Carrier`, `Requires`, `Result` and `NoRoll`, so nothing in the code names the items.

- **Election is part of the sheet**, so it happens before the first roll: `ModelSheet.UsesItem` is `""`, `"healingHerbs"` or `"wondrousCram"`; `ReopenSheet` can still change it.
- **Healing Herbs.** One Hero carrier that `participated` with outcome `survived` or `fled`. It adds +1 to the 2D6 total of **every** injury roll of the company in that aftermath
  (Warriors and Heroes), also the Hero's "Just a Scratch" roll again and an Against the Odds re-roll, and the total is clamped to 12. Not cumulative: two Herbs
  never give +2, and only the elected one is consumed. `Casualty.Modifier` shows it ("Rolled 9 +1 = 10 (4 + 5)"); `Dice` stay the raw dice and `Total` is the modified one.
- **Wondrous Cram.** The casualty itself (outcome `casualty`, `participated`), any model. No roll: the casualty is made at Start already rolled with the chart's Full Recovery
  row, final, with the text "Wondrous Cram: Full Recovery" (`Casualty.Item`). A Hero's Full Recovery still offers the "heal one persistent injury" decision; Dead, retirement, Valar and Wounds
  of a Hero cannot happen. Cram on a fled or surviving unit is refused, `RerollCasualty` refuses a Cram casualty and `Preview.Rolls` excludes it.
- **Carriers.** The item is in the model's equipment, or loaned to it by a Healer's Kit (the Kit's Herbs go to Heroes only). The temporary Wanderer is never a carrier.
- **Consumed at Finish**, not before: the used item leaves the model's equipment (the small slot is free again) and one line records it ("Aldor used Healing Herbs"; Cram appends ": Full Recovery"). A loaned item is simply spent; unused items stay.

`AftermathInput.ValidateWith(roster, loaned, rules, result)` (`Validate` is the same with no loaned items) refuses with `ErrInvalidAftermath` an unknown election, an item the unit does not
carry, a Hero-only effect on a non-Hero, a missing `participated`, `casualty` or `notCasualty`, and a second non-stackable item ("Healing Herbs do not stack").
`AftermathInput.ItemChoices(roster, loaned, rules)` is the pure list of what each unit can elect right now, used by the page and by `Validate`. `LoanedItem{UnitID, Item}` comes from the Healer's Kit.
`RollModifierOf(rules, in).Apply(total)` gives the modified total and the shown modifier.

### Against the Odds in the aftermath

`AftermathService.WithATO` reads the match's Against the Odds record for the company that took it ([matches.md](matches.md)).

- **Prefill**: while no sheet is saved, `Aftermath.Input.AgainstTheOdds` is the record's bonuses that do something here (influence, experience, reroll: `OddsBonus.Effective()`), `Aftermath.FromMatch` is true (badge "From the match") and the preview counts them. Once the player saves or starts, the saved sheet wins and the badge goes.
- **Tool Kit**: the assigned items (`ATORules.LoanedFor`) are the `loaned` argument of `ValidateWith` and `ItemChoices`, so a Hero can elect the Kit's Healing Herbs and a casualty the Kit's Wondrous Cram; unused kit items disappear with the match and no equipment ever enters the roster.
- **Temporary Wanderer**: never on the sheet (it is not on the roster and `AftermathDefaults` skips it): no roll, no experience, no progression.
- **Ambush!**: nothing in the aftermath (it only chose the scenario).
- **All is Lost** keeps the participation influence and the Against the Odds bonuses (`TestAllIsLostKeepsAgainstTheOddsBonusesWithTheSelection`).

### Typed dice

With `ManualRolls` on, the Against the Odds re-roll takes the dice the player rolled (the same selects as the roll, one per die of `injuries.roll`; "Roll for me" lets the server roll) and the bonus die of
Lesson Learned (a D3) and Wounds of a Hero (a D6) is asked for after the injury roll as a decision of kind `bonus` (`Decide`, answer "0" for the server or the number rolled, up to `core.TypedMax` of the row's
`grantsExperience.dice` / `grantsInfluence.dice`; a D3 is rolled on a D6 and read 1-2 as 1, 3-4 as 2, 5-6 as 3, `core.FromTyped`). The bonus dice are any `NdM` spec (`InjuryRow.BonusSpec`). With `ManualRolls` off the server rolls the bonus die at once. A re-roll that lands on one of those results asks for its own bonus die. Thrown dice (ShakeTheTable) are not offered for the re-roll or the bonus die.

## Perks in the rolls

`Perk.When` carries the data's `when` (`firstReinforcementRollThisAftermath` for Rendezvous, `nextScenarioTableRoll` for Vantage Point). `CompanyState.Perks` holds the company's unused perks (filled on load) and
`CompanyState.PerksUsed` the ones a change uses up, marked in the same transaction (refused when used already). `core.UnusedPerk(perks, effect)` finds one; `CompanyDetail.Perks` tells the page whether to show the selector.

- **Rendezvous**: in the recruit roll panel, a company with an unused `rendezvous` perk sees a one-time selector (Not now / +1 / -1) next to "raise the die with Influence". It is free, applied to the die before the chart lookup, and written in the roll line ("You rolled a 5, Rendezvous +1 = 6"). `RecruitService.RollRecruitWithPerk(..., perkAdjust)` takes -1, 0 or +1 (others `ErrInvalidRoll`); only the roll that pays for a new recruit takes it; `RollRecruit` is the same with 0. `RecruitRoll.PerkRaw` and `PerkAdjust` let the window show "4, Rendezvous +1 = 5".
- **Vantage Point**: after a scenario roll on the match page, the second die can be adjusted +1 or -1 by a company in the match that holds an unused one (`AftermathService.UseVantagePoint(user, id, matchID, companyID, second, adjust)`: the die stays on 1-6; it returns the moved die and the line "3, Vantage Point +1 = 4"). `POST .../scenario/roll` carries `vantage` (the company) and `adjust`; the dice posted are the ones on show.
- A Final Blessing stays a note on the perks card; **Mark used** (`UsePerk`, for the owner) covers the others.

## Go contract

All methods take the acting `core.User` and return `ErrNotFound` (campaign, match or company unknown, or the company is not in the match), `ErrRefused` (reason fit to show: match open, not your company, locked step, finished campaign),
`ErrAftermathChanged` (stale `Revision`) and `ErrInvalidAftermath` (joined, fit to show). `core.AftermathService` (`NewAftermathService(campaigns, companies, matches, rulesets, aftermaths, rules, dice)`) implements them
over `aftermathRepository`; its `Update` is one transaction that locks the match row (shared, so the match cannot be reopened meanwhile), the company row and the aftermath row, and stores the company state, the record
(`match_aftermaths`: status, revision, `input`, `state`, `lines`, timestamps), the scenario `Grant`s and the new `Perk`s together. Every write bumps the revision once.
`Bury` and `Injure` take no user, so the service authorises the caller itself (`AftermathAccessFor`, `CanWaive`).

```go
type AftermathStatus string // pending | sheet | injuries | levelups | done | waived
type ModelOutcome string    // survived | casualty | fled

type ModelSheet struct {
    UnitID   int64
    Outcome  ModelOutcome
    Gains    map[string]int // campaign.experience.gains key -> times earned; flags 0 or 1, repeatable gains 0-9
    Relic    bool
    UsesItem string         // "" | healingHerbs | wondrousCram
}
type AftermathInput struct {
    AgainstTheOdds []string // campaign.againstTheOdds.bonuses[].key
    Models         []ModelSheet
}
func (in AftermathInput) Validate(roster []CompanyUnit, rules AftermathRules, result CompanyResult) (AftermathInput, error)

type Casualty struct {
    UnitID          int64
    Rolled, Final   bool
    Dice            []int
    Total, Modifier int
    ResultKey, Text string
    Item            string   // the effect key when the unit did not roll
    Bonus           int      // the D3 of Lesson Learned or the D6 of Wounds of a Hero
    Rerolled        bool
    Previous        []string
    Decision        *CasualtyDecision // Kind: scratch | heal | succession | sergeant | bonus
}

type Aftermath struct {
    MatchID, CompanyID int64
    Status             AftermathStatus
    Revision           int
    Outcome            string // win | draw | loss; Fled says All is Lost
    Fled, FromMatch, FromLive bool
    Absent             []int64
    Input              AftermathInput
    Preview            AftermathPreview // experience, influence, rolls, rewards, items
    Casualties         []Casualty
    Succession         *Succession
    ItemChoices        map[int64][]string
    Pending            []Decision // rewards that need a choice at Finish
    Lines              []AftermathLine
    MVP                *MVPHint
    Access             AftermathAccess // AftermathNone | AftermathEdit
}
```

| Method | Does |
|---|---|
| `Aftermath(ctx, user, id, matchID, companyID)` | Loads the state (a computed default sheet when none is saved: everyone took part and survived, or the live match's) |
| `AftermathStatuses(ctx, id, matchID)`, `PendingAftermaths(ctx, id, companyID)` | Each company's status for the match page; the pending ones for the My Company banner and the lock |
| `SaveAftermath`, `StartAftermath` (saves then starts), `ReopenSheet` (before the first roll) | The sheet |
| `RollCasualty(..., unitID, entered, thrown)` | Rolls one casualty, or all unrolled ones (`unitID` 0, then `entered` nil); `RollRefusal` applies |
| `RerollCasualty(...)` | Spends one Against the Odds re-roll on a rolled casualty that is not final |
| `Decide(ctx, user, ..., core.AftermathAnswer)` | Answers an open decision (scratch: `reroll`/`keep`; heal: an injury key or `""`; succession and sergeant: a unit id; `bonus`; `continue`) or a pending reward (`"<item>\|<unit id>"` for free equipment) |
| `FinishAftermath(...)` | Applies step 6; refused while a casualty is not final or a reward decision is open |
| `WaiveAftermath(..., note)` | Organiser and administrators: no effects, unlocks the company, records the note |
| `UsePerk(ctx, user, id, companyID, perkID)` | The owner marks a perk used |
| `FreeEquipmentOffers(ctx, id, companyID, maxInfluenceValue)` | Armoury items worth at most the value that a unit can take and carry; `Decide` and Finish check the choice with it |
| `AftermathRules(ctx, campaignID)` | The gains and bonuses the form renders |

The pure rules are table-tested: `CompanyOutcome`, `DefaultSheet`, `AftermathInput.Validate`, `Experience`, `Influence`, `Rewards`, `Preview`, `RerollsFor`, `ResolveCasualty`,
`ResolveScratchReroll`, `NextSuccession`, `NextSergeant`, `AftermathAccessFor`, `CanWaive`. Tests inject dice with `core.FixedDice`.

Effects: absent units recover at Start (undone by `ReopenSheet`); injuries count up (`Injuries`); a second arm or leg wound buries the hero as "Retired", Dead buries as "Dead"; Fate rises through the path's limits;
Lesson Learned adds XP and Wounds of a Hero adds Influence at Finish. A fallen Leader's place is chosen in `Decide`; with no Hero left, the chosen Warrior becomes Leader with the officer's start and chooses its path in the
usual path window. `CompanyDetail` carries `Aftermaths` (pending ones), `Perks` and the latest 20 `History` lines (`CompanyService.WithAftermaths`).

## The aftermath page

`/campaigns/{id}/matches/{match}/aftermath/{company}` is readable by every player of the campaign and editable per `AftermathAccess`. A header (match N, scenario, the company, an outcome badge: Win / Draw / Defeat /
"Fled (All is Lost)"), then a **stepper** of four steps (Sheet, Injuries, Level-ups, Finish) where the current one is open, earlier ones collapsed to their summary line ("3 casualties, 1 Dead") and later ones dimmed. Every write
answers with the whole `#aftermath-page` (`aftermath_body`), like the match page. States of each step: empty, editing, read only (not your company, or the step is done), stale (notice "<name> changed this while you were editing"),
error, locked (match open: "End the match first"). State lives in the core, so a reload restores it. Cards use `core.Distinguish(roster)` for titles: a Leader or Sergeant carries its role ("Mordor Orc Warrior (Leader)"),
units that still share a title are numbered ("Warrior of Rohan 2"), a name the player gave stays; titles are for display only.

- **Sheet** (`POST .../sheet`, `.../start` with a confirmation, `.../reopen-sheet`). One card per unit in the company's order, single column: name and type; a segmented control Survived / Casualty / Fled; a "Took part" switch; the gains as
  toggle chips in the file's wording (short label, full text in the help popup) with a stepper for the repeatable one; MVP and Relic chips (Alpine keeps one holder of each and disables the chips of a unit that did not take part);
  44 px touch size. Above: the Against the Odds block (checkboxes with rating and effect, badge "From the match") and the read-only match result. A unit that can elect an item has a switch row below the gains
  ("Use Healing Herbs (+1 to every injury roll of the company)", "Use Wondrous Cram (skips the injury roll: counts as Full Recovery)"); once one unit has the Herbs on, the other Herbs switches show "Already used by Brand
  (they do not stack)", and a carrier that does not qualify shows the reason ("Casualties cannot use Healing Herbs"). The form posts `unit` ids, per unit `outcome-<id>`, `took-<id>`, `gain-<id>-<key>`, `item-<id>`, a single `relic` and `odds`.
  A sticky bottom bar on phones (an inline card from `md`) shows the preview of the *saved* sheet ("XP +N across M units, Influence +N, K injury rolls", the scenario rewards, "Items: Herbs +1, 1 Cram") with **Save** and **Start aftermath**
  (disabled with the reason while invalid, or while the Relic is ticked but unassigned). Messages that start with a unit's title appear on that unit's card, the others on top (422, the typed sheet kept). Absent units appear below
  as "Missed this game, back next match". When the sheet is the unsaved default of a live match it shows "From the live match" and the MVP vote lines (`Aftermath.MVP`: the winner is prefilled, a tie shows "Tied: ..." and the owner chooses).
- **Injuries** (`aftermath_injuries`, `aftermath_casualty`, `aftermath_decision`, `aftermath_succession`). One card per casualty with the dice component, **Roll {injury dice}** (`POST .../roll` with `unit`, `roll`, `roll2`, `thrown`), **Roll all**
  when two or more wait (the server rolls; only the unrolled dice animate), the result with its bonus die, the chart (collapsed, the row of the roll highlighted), a tombstone note for the fallen, the open decision (`POST .../decide`
  with `kind`, `unit`, `answer`), **Re-roll · Against the Odds** (`POST .../reroll`) with the count left, and the Leader's succession and new Sergeant questions. A Herbs banner reads "Healing Herbs: +1 to every injury roll (the total stops
  at 12)"; a Cram card has no Roll button and the badge "Wondrous Cram". The collapsed step says what each result did ("goes to the cemetery", "forced to retire by its injury number N", "misses the next game", "healed one Arm Wound" / "nothing healed").
- **Level-ups** (`aftermath_levelups`). The units with rolls waiting, each with a **Level up** button that opens the My Company level-up window in a modal on this page (same endpoints and `level_panel`; only the owner has the buttons, others
  are told who rolls); closing the window reloads the page. **Continue to Finish** (editors only; readers see Finish open) is always allowed. Once the aftermath is done the step reads "N units still have a level-up roll waiting" when rolls remain.
- **Finish** (`aftermath_finish`). The influence breakdown, the scenario rewards, the free-equipment decision (`POST .../decide kind=reward key=<reward> answer="<item>|<unit id>"`) and **Finish aftermath** (`POST .../finish`,
  disabled while a decision is open). Afterwards the page is the **summary**: the audit lines grouped by unit (the company first), with links to My Company and the match, readable by everybody.

The match page has an **Aftermath card** once the match has ended (`aftermath_card`): one row per company with its status badge (Pending / In progress / Done / Waived), **Start** or **Continue** for an editor, **View** for everybody
else, the one-line summary of a finished or waived one and, for the organiser and administrators on an unfinished one, **Waive** (an inline form with the note, `POST .../aftermath/{company}/waive`). My Company and the company page get the
banner, the perks card and the history ([frontend.md](frontend.md)). The History card groups the lines by match ("Match 4", newest first); the repository takes whole aftermaths until it has at least 20 lines. Every `AftermathLine` is filled with
`Match`, `MatchID` and `On` when it is read; they are not stored in the lines JSON.

## Wargear on a promotion

One rule (`internal/core/promotion_gear.go`): a warrior who is promoted keeps only the wargear his new profile lists, under any name the armoury recognises as the same item (`ItemKey`, the armoury's other names and the annotations'
aliases such as Two-handed axe = Two-handed weapon, `wargearReadAs`); the rest is lost for good. The new profile lists its options, its own wargear, what its company label selects and what the promotion gives. Equipment (slot items) stays, and a
unit that becomes a hero keeps everything. The points follow from what is left. A hero promotion that also changes the profile (Ranger of Arnor to Ranger of the North) has one rule too (`replacedWargear`): the wargear the old profile carried
as its own and the new one does not carry as its own is lost (the Armour); what the player chose from the options stays (the Spear). Different weapons are different items (a Wood Elf Spear, an Elven glaive and an Elven spear are three).

Every carried item is judged on its own. A stored option label stands for the items it carries: "Shield and Spear" is two items, and an exchange ("Exchange hand-and-a-half weapon for bow") carries only the item it gives. A carried item the new
profile lists stays as itself; when the new profile has an exchange option that gives it, that exchange is already chosen and the entry becomes that option; when the new profile carries it as its own wargear the profile supplies it (a Khandish
Warrior with the bow becomes a Khandish Horseman, who lists the hand-and-a-half weapon and the bow, and has both).

Poison is no property of wargear ([definitions.md](definitions.md)): the plain Bow stays when the new profile lists it, and the unit is poisonous only if the new profile has Poisoned Attacks (a Haradrim Raider has it, a Khandish Horseman does
not). An Envenom Weapon keeps its chosen weapon whether or not the new profile lists it: the choice is final and the rule `Poisoned Attacks (<weapon>)` belongs to the weapon's name; the player may buy the weapon again if the new profile
offers it. `TestEveryPromotionAppliesTheOneRule` applies the rule to every promotion of every company, with no per-item list.
