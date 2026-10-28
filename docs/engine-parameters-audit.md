# Engine parameters audit: rule values hard-coded in Go and templates

Read-only audit for the lean definitions format ([`definitions-lean-design.md`](definitions-lean-design.md), section 4.8, and
[`bcjson-audit-backend.md`](bcjson-audit-backend.md)). Bead `battle-company-manager-ce-v2-vm4`.

**Owner decision:** every core-engine rule that carries a magic number or a fixed choice takes its value from the Battle
Companies json, so that a custom ruleset can change it (example: Influence lowers a roll, or moves it up to 5 points per
attempt instead of 3).

Scope: `internal/core`, `internal/web` (Go and templates, `static/js`), `cmd/server`. Not rule values and left out: colours,
layout, CSS, test fixtures, `cmd/server/world*.go` demo data.

Status codes used in the tables:

- **IGN**: the json already holds the value (usually under `campaign.*`) and the code ignores it or only checks that it
  equals what the code assumes.
- **PART**: the code reads part of it.
- **LACK**: the json has no field for it (or only prose).
- **VOC**: a closed vocabulary the engine has to know (effect keys, kinds). The *value* stays data, the *keys* are engine.
- **param** = rule parameter, moves to data. **choice** = fixed rule behaviour; becomes a flag or enumerated value in the
  data when the owner wants it switchable, otherwise it is documented as "the engine implements these rules". **eng** =
  engine safety limit, stays in code.

Where a name is given as `campaign.x`, it is the existing section of the Battle Companies json; in the lean design the
section keeps its shape (4.8), so the proposed names are additions to it.

## 0. Findings in short

- 52 items in `internal/core` (plus 14 numbers printed by the web layer, section 8, and 13 engine limits that stay, section 9):
  31 rule parameters, 13 fixed choices, 5 vocabularies, 3 engine limits. Status: 28 IGN (the json has the value and the code
  ignores it), 8 PART, 8 LACK (no json field), 8 already data or not applicable.
- Most of what the owner asks for **is already in the json** and the code ignores it. The json is the better-prepared side.
- The code **reads** the tables for injuries, treatment, rating, experience gains, Against the Odds, stat limits, creatures,
  wanderers and path limits. It **does not read** `campaign.recruitment` (influence adjustment), `campaign.company.leaders` /
  `sergeants` / `startingHeroStats`, `campaign.validations` (equipment slots, max roles), the dice strings (`"2d6"`, `"d6"`,
  `rollsPerProgression`), `breakPoint.threshold`, `rating.hero.wargear.threshold` and the aftermath perk `range`.
- Dice are the largest hidden parameter: `Dice` has only `D6()`; every roll is built from it with the sizes spelled in the
  code (2D6 injuries, 2D6 x 2 hero level-up, D6 warrior level-up, recruit, treatment, scenario selection, courage test).
  The json says `"2d6"`, `"d6"`, `"dice": "2d6"` everywhere and nothing parses it (live_service.go:1012 compares the string to
  `"2d6"` and refuses anything else, which is the opposite of a parameter).
- A pattern repeats: the **same number is in three places** (json prose, Go constant, template text). Changing the json then
  changes nothing visible. The templates must print the value they are given (section 7).
- Two parameters are held by types that cannot see the ruleset: `XPPerLevel` is used by methods of `CompanyUnit`
  (company.go:441,449) and `MaxAdjust` by `RecruitRoll` code and templates. The ruleset value has to be carried into the
  view (`CompanyDetail`/`CompanyUnit.LevelEvery`), not looked up at call time.

## 1. Influence, recruiting and rolls adjusted (owner's example)

The json has all of it in `campaign.recruitment.adjust` and `campaign.influence.spend[adjustRoll]`, as prose that no code
reads (lean design 4.8 moves it to `reference/`; **it must stay in `campaign` and be read instead**).

| # | file:line | value | rule meaning | status | proposed json | class |
|---|---|---|---|---|---|---|
| A1 | company.go:599 `MaxAdjust = 3`; used recruiting.go:78,198 | 3 | most influence spent adjusting one recruit, over both charts | IGN (`recruitment.adjust.maxPerAttempt`: 3, `influence.spend[adjustRoll].maxPerRoll`: 3) | `campaign.recruitment.adjust.maxPerAttempt` | param |
| A2 | recruiting.go:198 (`max(0, ...)`), 125-126, 166 | up only | the die is raised, never lowered | IGN (`adjust.direction`: "up", `lowering`: "never") | `adjust.directions: ["up"]`, or `["up","down"]` with `lowering: "free"` / `"paid"` | param |
| A3 | recruiting.go:199,212 `st.Influence -= adjust` | 1 per point | influence price of one point of adjustment | IGN (`costPerPoint`: 1) | `adjust.costPerPoint` | param |
| A4 | recruiting.go:78,148,166,198,202 (`6`, `1..6`) | D6 | the reinforcement die and its 1 to 6 results | LACK (implicit in the chart rows) | `campaign.recruitment.die: "d6"` (validator checks the chart rows cover it) | param |
| A5 | recruiting.go:198 (`Adjusted` accumulates over charts) | shared | the adjustment budget is shared by the reinforcement and the special chart; no carry of unspent | IGN (`adjust.sharedAcross`) | `adjust.sharedAcross: ["reinforcements","specialChart"]` | choice |
| A6 | recruiting.go:198-203, 287-312 | pay to reach | the player pays every point needed to reach the wanted result; picking a lower result is free | IGN (`payForEveryPointToReachChoice`, `lowering`) | `adjust.payForEveryPoint`, `adjust.lowerResult: "free"` | choice |
| A7 | recruiting.go:125-126,166-169 (Rendezvous); aftermath_perks.go:94-97 (Vantage Point) | -1..+1 | the perk moves a die by one | IGN (`aftermathEffects[rendezvous|vantagePoint].range`: [-1,1]) | read `range`, `rolls` | param |
| A8 | companies.go:12 `StartingInfluence = 0` | 0 | influence of a new company | LACK | `campaign.company.startingInfluence` | param |
| A9 | companybuild.go:759-776 `rareBlock` | at most N of a type in the roster | Rare N: injured count, dead and promoted do not, exhausted gives a lower result | IGN (`recruitment.rare.*` flags, prose) | `campaign.recruitment.rare {injuredCount, whenExhausted}` | choice |

Reinforcement price (3), special chart price, Against the Odds ratings and the roll table already come from the data
(`BCCompany.ReinforcementCost`, companybuild.go:546). Already data: treatment cost, target, per-point rise and ceiling
(treatment.go:84,121-123; injuries.go:151,183), which is the pattern A1-A3 should follow.

Section 1: 9 items (6 param, 3 choice).

## 2. Company shape, officers, limits, slots

| # | file:line | value | rule meaning | status | proposed json | class |
|---|---|---|---|---|---|---|
| B1 | startingcompany.go:66,195; aftermath.go:952 `n >= 2`; recruiting_ranger.go:57,100,130 `maxSergeants = 2` | 1 leader, 2 sergeants | officers of a company; "choose one leader and two sergeants" | IGN (`campaign.company.leaders`, `sergeants`, `validations[singleLeader|maxSergeants]`) | `campaign.company.leaders`, `.sergeants` (lean 4.8) | param |
| B2 | startingcompany.go (3 heroes = 1 + 2) | 3 | heroes of a starting company | IGN (`startingHeroes`) | derived: leaders + sergeants | param |
| B3 | companybuild.go:915 `HeroBonus`; callers aftermath_service.go:763,779, recruiting.go:627, companybuild.go:462,468, audit.go:335; levelling.go:230 | Might 1, Will 1, Fate 1 | what a new hero starts with (also a Hero in the Making result) | IGN (`startingHeroStats`) | `campaign.company.heroStart` | param |
| B4 | battlecompanies.go:18,206,428 | 15 / 12 | default and Elite model limit | PART: `maxModels` read, `eliteMaxModels` loaded and never used | `campaign.company.maxModels`; Elite = company `maxModels: 12` (lean design) | param |
| B5 | startingtotal_test.go only (no production code) | 105 (106 Defenders) | points of the starting company plus gold | LACK (`campaign.gold.startingTotal` 60 and `startingPoints` 60 contradict it) | `campaign.company.startingTotal`, company `startTotal` | param (validator rule for uploads) |
| B6 | company.go:480 `Slots{LargeMax: 1, SmallMax: 1}` | 1 large, 1 small | equipment a model can carry | IGN (`validations[equipmentSlots]` small 1, large 1) | `campaign.company.equipmentSlots {small, large}` | param |
| B7 | company.go:488 `Backpack` -> `SmallMax += 3` | name + 3 | an item that adds Small slots | LACK (text only) | gear `grantsSlots {small: 3}` | param |
| B8 | gear.go:100-103 `defaultLimitRules`; company.go:525 uses it directly; gear.go:178 always rounds up | 33.33 %, round up | bow, throwing and cavalry limits | PART (`limits[]` read at battlecompanies.go:524, but `RoundUp` is ignored and company.go:525 bypasses the file) | existing top-level `limits[]`; remove the Go default | param |
| B9 | gear.go:132-165; armoury.go:260,284-286 | bow / throwing / cavalry ids -> item classes | which items and models a limit counts | LACK | `limits[].classes` (backend audit 2.6) | param |
| B10 | armoury.go:630 `max(1, wr.MaxAtOnce)`; creatures per hero, per company; path limit per company | 1 | wanderer, creature, Sorcerer limits | OK, data (`wanderers.rules.maxAtOnce`, `creatures.rules.maxPerHero|maxPerCompany`, `path.limit`); the `max(1, ...)` fallback only hides a missing value | keep; make the field required in the schema | param |

Not code, but listed because nothing enforces them: `validations[oneCreaturePerHero|oneWanderer|oneSorcerer|maxModels]`
duplicate data that is read elsewhere; the whole `validations` section is unread (bcjson-audit-backend 1). Drop it or make
it the single source (recommendation: drop; `campaign.company` and the typed fields are the source).

Section 2: 10 items (all parameters; B10 is already data).

## 3. Experience and progression

| # | file:line | value | rule meaning | status | proposed json | class |
|---|---|---|---|---|---|---|
| C1 | company.go:385 `XPPerLevel = 5`; used company.go:441,449; web units.gohtml:389 | 5 | experience per level-up roll | IGN (`progression.threshold`, `experience.progressionThreshold`; `ProgressionRules.Threshold` is read at progression.go:288 and `AftermathRules.Threshold` at aftermath.go:300, but the unit methods use the constant) | same fields; carry the value to `CompanyUnit` at build time | param |
| C2 | levelling.go:127-132,302; progression.go:714 | D6 | warrior level-up die; table `progression.warrior.table[].roll` | IGN (`progression.warrior.roll`: "d6") | same | param (dice) |
| C3 | progression.go:660 `make([]pair, 2)`; levelling.go:314; web levelup.go:22 `"2,2"` | 2 results, choose 1 | a hero rolls two pairs and keeps one | IGN (`progression.hero.rollsPerProgression`: 2, `choose`: 1) | same | param |
| C4 | progression.go:662,671; levelling.go:314; web templates "2D6" (campaign_my_company.gohtml:179) | 2D6, sums 2 to 12 | hero level-up dice; each path also says `"dice": "2d6"` | IGN (`progression.hero.roll`, `paths[].dice`) | same; per-path dice honoured | param (dice) |
| C5 | levelling.go:497-507 (`p.Bonus[stat]--`) | Courage and Intelligence by 1 to 3+ | a warrior without promotion improves them | PART (`noPromotion.courage|intelligence` tested `> 0` and `maximum` read; the amount 1 is ignored) | `warrior.noPromotion.{courage: n, intelligence: n, maximum}` read as amounts | param |
| C6 | levelling.go:230 | +1 Might, Will, Fate | a Hero in the Making result | IGN (same as B3) | `campaign.company.heroStart` | param |
| C7 | progression.go:380-470 | Peak / Average per stat | overall caps and averages | OK, data. Unread: `characteristicLimits.increases.courageIntelligenceTotalCap` (3, incl. gear) and `peak.willMaxOverrides` | read or delete | param (low) |
| C8 | aftermath.go:925-960; aftermath_service.go:728-794 | Sergeant -> Leader; Hero -> Sergeant; else warrior with most XP, tie highest rating | succession when officers fall | IGN (`injuries.deathOfAHero`, prose) | `campaign.company.succession: ["sergeant","hero","mostExperienced"]` | choice |
| C9 | levelling.go:370,377; progression.go:18; web | `noChange`, `promoted`, `heroInTheMaking` | keys of the warrior table | n/a | keep as a closed vocabulary; validator rejects other keys | VOC |

Section 3: 9 items (7 param, 1 choice, 1 VOC).

## 4. Injuries, treatment and the War's Aftermath

Mostly data-driven already (rows, kinds, retire count, healing numbers, experience gains, influence gains, Against the Odds
amounts, item effects, clamp). The exceptions:

| # | file:line | value | rule meaning | status | proposed json | class |
|---|---|---|---|---|---|---|
| D1 | aftermath_service.go:439-443 (`castDice`, "each from 1 to 6"); aftermath.go:353 (`2..12` coverage); web aftermath.gohtml:114,137; static/js/company.js:30 | 2D6 | the injury roll | IGN (`injuries.roll`: "2d6") | same | param (dice) |
| D2 | aftermath.go:884-891; aftermath_service.go:547,559 (`n > 6`, D3 = (n-1)/2+1) | D3, D6 | bonus dice of Lesson Learned, Wounds of a Hero | PART (names read; only `d3` and `d6` exist in code) | generic `NdM` parser, rows keep `grantsExperience.dice` | VOC |
| D3 | aftermath.go:346 (`sc.Stat == "fate"`) | Fate only | stat change of an injury row that is kept | PART | read any stat in `statChanges` | param (low) |
| D4 | aftermath.go:739 | participating only | a fled company gets Influence for participating (and Against the Odds), no Win or Draw, no rewards | IGN (`breakPoint.allIsLost.outcome`) | read the flags | choice |
| D5 | injuries.go:249-251; treatment.go:103 `"justAScratch"` | key | an injury cancelled by treatment | n/a | the key is a vocabulary; make it a row property `cancelsMissedGame: true` | VOC |
| D6 | aftermath.go:53 `maxGainCount = 9` | 9 | typed count of a repeatable XP gain | eng | stays | eng |

Section 4: 6 items (2 param, 1 choice, 2 VOC, 1 eng).

## 5. Break point and the live match

| # | file:line | value | rule meaning | status | proposed json | class |
|---|---|---|---|---|---|---|
| E1 | live_break.go:189 `Casualties*100 > Threshold100` | strictly greater, unrounded | the company is Broken when casualties exceed percent of the starting models | IGN (`breakPoint.threshold.comparison`: "strictlyGreater", `rounding`: "none") | read `comparison: gt|gte`, `rounding` | choice |
| E2 | live_break.go:156-170 | fallen or fled after failed Courage; not a lost mount | what counts as a casualty | IGN (`breakPoint.casualties.counts|doesNotCount`) | closed list read from data | choice |
| E3 | live_service.go:138 (`Extra: len(d.Creatures)`); live_break.go:170-176 | creatures and wanderers count; composite = one | models counted at the start | IGN (`breakPoint.startingModels.includes|composite`) | closed list read from data | choice |
| E4 | live_service.go:1012-1037; web company_panels.gohtml:536 | 2 dice, total >= Courage | Courage test of a Broken company | PART: the file is read, then compared to the one value the code implements and refused otherwise | generic dice expression and `passIf` vocabulary | param (dice) |
| E5 | live_break.go:125-148 | stand fast, heroic resolve, chariot, scenario; engaged and cannot activate are exempt | reasons a model skips the test | IGN (`whenBroken.exempt|autoPass`) | keep as vocabulary, texts from data | VOC |
| E6 | breakPoint default 50 %, company override | 50 | Break Point | OK, data (`BreakRules().DefaultPercent`, companybuild.go:1012) | keep | param |

Section 5: 6 items (2 param incl. E6, 3 choice, 1 VOC).

## 6. Rating, Against the Odds, scenarios

| # | file:line | value | rule meaning | status | proposed json | class |
|---|---|---|---|---|---|---|
| F1 | companybuild.go:377 `stats["attack"]+stats["wounds"] >= 3` | 3 | from this Attacks + Wounds a hero pays the hero rating for gear | IGN (`rating.hero.wargear.threshold {stats, sumAtLeast: 3}`) | same | param |
| F2 | battlecompanies.go:862 | Might/Will/Fate floor 0 | no Average for Might, Will, Fate | LACK | `rating.hero.perPoint[].floor` or `average` entries for them | param |
| F3 | scenarioroll.go:30-38; companybuild FullPoints | injured models excluded from Rating | models missing the next game do not count | IGN (`rating.text`, prose) | `rating.excludesInjured: true` | choice |
| F4 | scenarioroll.go:44-51 | higher = Top Dog, tie = none | Top Dog / Underdog | IGN (`scenarioSelection.text`) | document as engine rule | choice |
| F5 | scenarioroll.go:5-9 `(d+1)/2`, 17 (`> 6`); web match.go:306,698,718 | two D6; first die 1-2, 3-4, 5-6 pick the table | scenario selection | IGN (`scenarioSelection.tables[].roll`) | derive table from the ranges; die size `scenarioSelection.die` | param |
| F6 | ato.go:347 (`Inclusive`); 26-29 keys | inclusive budget; keys toolKit, wanderer, ambush | Against the Odds spending and structure | PART: inclusive is data; the three keys are vocabulary | keep | VOC |
| F7 | ato.go: eligibility "strictly lower" | strict | only the lower-rated company chooses | IGN (`eligible.compare`) | read `compare` | choice |
| F8 | match.go:20 `maxVictoryPts = 99` | 99 | input sanity | eng | stays | eng |

Already data: bonus ratings (15, 30, 45, 60), reroll count 2, kit contents, Tool Kit `maxPurchases` 5, hero rating
per-point values (5/10), minor rule rating and its cap (10), average profile, armour stage prices.

Section 6: 8 items (3 param, 3 choice, 1 VOC, 1 eng).

## 7. Economy, armoury

| # | file:line | value | rule meaning | status | proposed json | class |
|---|---|---|---|---|---|---|
| G1 | armoury.go:153 | option points | a warrior bought an option of its profile pays that option's points, not the armoury price | LACK | engine rule, document; flag `armoury.optionPricedAsProfile` | choice |
| G2 | armoury.go:238-273 | Beast keyword | a Beast takes no wargear, carries only listed items | LACK (`wargear.beasts` text) | `campaign.armoury.noWargearKeywords: ["Beast"]` | choice |
| G3 | armoury_race.go:5-17 | "for Elves" | Elven-made items are for Elf models | PART (tag `modifiers: ["elven"]` is data; the race is code) | gear `who {race}` (lean 4.3) | param |
| G4 | grant.go:11-14 `maxGrantGold 1000`, `Influ 100`, `Experi 100`, `Note 200` | caps | organiser grant sanity | eng | stays (campaign setting later if wanted) | eng |

Section 7: 4 items (1 param, 2 choice, 1 eng). Gold and influence prices of items, creatures, wanderers, the armour upgrade
and starting gold are already data.

## 8. Web layer: numbers printed from prose or checked in handlers

The web layer enforces no rule on its own (it calls core), but it **prints rule numbers as literals** and re-validates dice
ranges. When core takes the values from the ruleset, these must print the value from the view.

| # | file:line | literal | repeats | fix |
|---|---|---|---|---|
| H1 | units.gohtml:389 | "/ 5 XP" | C1 | `{{$u.XPPerLevel}}` from the view |
| H2 | company_panels.gohtml:30,72,119 | "up to 3", "1 influence per point" | A1, A3 | print `Roll.Budget`, `Adjust.CostPerPoint`, `Adjust.Max` |
| H3 | company_panels.gohtml:536 | "Courage (2D6, ...)" | E4 | print the rule's dice string |
| H4 | campaign_my_company.gohtml:54 | "costs 15 rating", "two re-rolls" | ATO reroll rating and count | print from the bonus rule (`Rating`, `Count`) |
| H5 | campaign_my_company.gohtml:179 | "Level-up chart (2D6)" | C4 | print the path's dice |
| H6 | aftermath.gohtml:114,137; glossary.gohtml:40,43 | "Roll 2D6", "2D6" | D1 | print the injury dice |
| H7 | match.gohtml:474,483 | "Each is worth 1 experience" | `experience.gains[].amount` | print the amounts |
| H8 | match.go:306 | "Table 1 (first die 1-2)" | F5 | build from the table ranges |
| H9 | match.go:698,718 | dice `1..6` | F5, A4 | take the die size from the rules |
| H10 | levelup.go:22 and levelup template | lanes `"2,2"` | C3, C4 | derive from pairs and dice |
| H11 | tts.go:77 | "over 12 inches" | aura group "large" | print from `data.auras.groups` |
| H12 | static/js/dice.js, company.js | 3D dice are six-sided, `max = min(budget, influence)` | A4 | dice sizes arrive as data attributes; budget already does |
| H13 | company_panels.gohtml:30 and `Succession` texts (aftermath.go) | "two Sergeants" in prose | B1 | print counts |
| H14 | pages/glossary.go, styleguide.go sample text | numbers in sample tips | none | not rules; ignore |

Section 8: 14 rows, 13 literals to fix (H14 is not a rule).

## 9. Engine safety limits that stay in code

Not rules: they bound input, memory or time. Listed so a later "no magic numbers" check can allow them.

| file:line | value |
|---|---|
| auth.go:31-33,158,345; campaign.go:74,154; match.go:17,56; companies.go:15,121-141 | password 10-128, session 30 days, names 1-60 and 40 characters |
| campaign.go:190-191 | `maxSides = 8`, `maxSideNameLen = 40` |
| aftermath.go:53 | `maxGainCount = 9` |
| match.go:20 | `maxVictoryPts = 99` |
| grant.go:11-14 | grant caps (G4) |
| ato.go:208; companies.go:119; auth.go:241-242; invites.go:17-18 | kit detail 40, history 20, page sizes 25/200, invite lifetime |
| levelling.go:313; progression.go:667 | 60 retries of a hero roll with no usable result |
| auras.go:40-42 | `AurasPerFigure = 6`, `AurasPerPayload = 150` (Tabletop Simulator mod limits) |
| rules.go:23,27,34; live_hub.go:13-14; live_service.go:548 | build timeout, stuck builds, ruleset cache, live streams, log page |

Section 9: 13 engine limits.

## 10. Counts

| Area | items | param | choice | VOC | eng | IGN | PART | LACK | OK / n/a |
|---|---|---|---|---|---|---|---|---|---|
| 1 Influence and recruiting | 9 | 6 | 3 | 0 | 0 | 7 | 0 | 2 | 0 |
| 2 Company shape | 10 | 10 | 0 | 0 | 0 | 4 | 2 | 3 | 1 |
| 3 Progression | 9 | 7 | 1 | 1 | 0 | 6 | 1 | 0 | 2 |
| 4 Injuries, aftermath | 6 | 2 | 1 | 2 | 1 | 2 | 2 | 0 | 2 |
| 5 Break point, live | 6 | 2 | 3 | 1 | 0 | 4 | 1 | 0 | 1 |
| 6 Rating, ATO, scenarios | 8 | 3 | 3 | 1 | 1 | 5 | 1 | 1 | 1 |
| 7 Economy, armoury | 4 | 1 | 2 | 0 | 1 | 0 | 1 | 2 | 1 |
| **Core total** | **52** | **31** | **13** | **5** | **3** | **28** | **8** | **8** | **8** |
| 8 Web literals | 14 (13 real) | | | | | | | | |
| 9 Engine limits that stay | 13 entries | | | | | | | | |

## 11. Prioritised list

**P1: the json already has the value, the change is local, and the owner's example is in this group.**

1. Influence adjustment (A1-A3, A7) and the web text (H2): `campaign.recruitment.adjust` read into `RecruitRules`;
   `RecruitRoll.Budget` and the template take it from there.
2. XP threshold (C1, H1): carry `Threshold` to `CompanyUnit` (or to `CompanyDetail`) and delete `XPPerLevel`.
3. Officers and hero start (B1, B3, C6): `campaign.company.leaders|sergeants|heroStart`; `HeroBonus` takes the rules.
4. Wargear rating threshold (F1); limits (B8, B9): delete `defaultLimitRules`, honour `roundUp`, add `classes`.
5. Starting influence (A8) and starting total (B5, validator side first).

**P2: dice become data.** One `DiceSpec` (`NdM`, parsed once at load) used for C2-C4, D1, D2, E4, F5, A4, with the
`Dice` interface gaining `Roll(spec)` (or `D(n)`), the entry validation (`1..n`, `N..N*n`) and the templates (H3, H5, H6,
H9, H10, H12) taking the spec from the view. This is the one structural change; do it as its own bead.

**P3: slots, break point, scenario tables.** B6, B7 (`equipmentSlots`, `grantsSlots`), E1 (`comparison`, `rounding`), F5
tables, C5 amounts, D3, G3.

**P4: fixed choices as flags (only if the owner wants them switchable).** A5, A6, A9, C8, D4, E2, E3, F3, F4, F7, G1, G2.
The default is to keep them as engine rules and say so in `docs/definitions.md` ("what the engine implements"), because
each is a behaviour with several code paths, not a number.

**P5: clean up the data.** Drop or implement the unread `validations` and `courageIntelligenceTotalCap`, `willMaxOverrides`,
`eliteMaxModels`; keep the `campaign.recruitment` prose that code now reads out of `reference/` (design 4.8 and section 9 of
the lean design list `recruitment` as unread; this audit changes that verdict for `recruitment.adjust`, `recruitment.rare`
and `influence.spend[adjustRoll]`).

## 12. Test strategy

1. **Golden stays the contract.** `internal/core/snapshot_test.go` (with `testdata/` goldens, `auras_golden.txt`,
   `tts_golden.txt`) must stay **byte-identical** after each of these changes: the shipped file carries today's values, so
   moving a constant to data may not change any rendered line. Run it after every bead.
2. **One custom-ruleset test per parameter.** A helper `customRuleset(t, mutate func(doc map[string]any))` loads the shipped
   json, applies the mutation, builds a `RuleSet`. A table-driven `TestCustomRuleset` has one row per item of sections 1 to 7
   (id, mutation, assertion), for example:
   - A1: `recruitment.adjust.maxPerAttempt = 5`, a recruit adjusted by 5 succeeds; by 6 is refused with the new number in
     the message (and the view's `Budget` is 5).
   - A2/A3: `directions: ["up","down"]`, `costPerPoint: 2`: a lowered roll is accepted and costs 2 per point.
   - C1: `progression.threshold = 3`: a unit with 3 XP has one pending level-up; `XPInLevel` and the page text say 3.
   - B1: `leaders: 1, sergeants: 3`: the starting company needs one leader and three sergeants, the 4th sergeant is refused.
   - B3: `heroStart: {might: 2, will: 1, fate: 0}`: a new hero and a Hero in the Making start so.
   - C2-C4, D1: `injuries.roll = "d6"`, `hero.roll = "3d6"`: rolls, accepted typed ranges and the view dice change.
   - E1: `comparison: "gte"`: exactly 50 % lost is Broken.
   - F1: `sumAtLeast: 4`: a hero with Attacks + Wounds 3 still pays the warrior rating for gear.
   - B6/B7: `equipmentSlots {small: 2}`: two Small items carry; Backpack `grantsSlots`.
   Each test also asserts the *default* row still equals the shipped behaviour (guards the golden).
3. **Template tests** (`internal/web`): render the page with a view built from the custom ruleset and assert the number
   appears (H1-H10); a grep-style test fails if `"/ 5 XP"`, `"up to 3"` or `2D6` literals are back in templates.
4. **Magic-number lint.** A test that parses `internal/core/*.go` (non-test) with `go/ast` and fails on integer literals
   other than 0, 1, 2 and the allow-list of section 9, plus a names list (`XPPerLevel`, `MaxAdjust`, ...). Every new rule
   constant then needs a data field or an explicit allow-list entry with a reason.
5. **Validator.** `tools/validate_definitions.py` and the Go loader reject a custom file whose dice string is unknown, whose
   chart rows do not cover the die (A4), whose injury rows do not cover the dice range (aftermath.go:353 already does for
   2-12; generalise), whose `leaders + sergeants` is 0, or whose vocabulary keys (C9, E5, F6) are unknown.
6. **Order of work for beads:** DiceSpec first (P2), then the P1 items in separate beads (each: json read, code, template,
   golden unchanged, one custom-ruleset test); fixed choices last and only on request.
