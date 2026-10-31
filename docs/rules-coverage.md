# Rules coverage: the rulebook against the app

Every mechanic of the Battle Companies 2nd CE rules and what the app does with it: where it lives in the code or the docs, and the test that covers it. Read it to find
where a rule is implemented or to see what the app deliberately leaves to the table.

- **Source** is a line of `sources/battle_companies_rules.txt` (gitignored; read it there, nothing is copied here). `S202` is
  line 202.
- **Layer**: **CM** company management (roster, resources, between games); **PH** match and the phases around it (the game, the
  War's Aftermath); **APP** app design, not a rule.
- **Status**: **DONE** built and tested; **PARTIAL** built in part (the gap is named); **TODO** not built; **N-A** not in the
  rulebook, played on the table, or decided not to build.
- **Where** is the core file (`internal/core/`) or the docs page that holds the behaviour; **Test** is the test that covers it
  (core tests unless a path says otherwise; `e2e` is the browser suite in `tests/e2e/flows`, see [testing.md](testing.md)).
  Rules numbers and texts come from `definitions/` ([definitions.md](definitions.md)), never from constants in the code.

The rulebook has no campaign calendar, end, ranking or alliances: its loop is Muster, then repeat Step 1 Play a Game, Step 2
War's Aftermath (1 Injuries, 2 Experience, 3 Progression, 4 Influence), Step 3 Company Management, Step 4 Calculate Ratings
(S298-302, S1357, S1400). Kits exist only as Against the Odds items; "retirement" is an outcome of the injury chart.

## 1. Muster and company creation

| Mechanic | Source | Layer | Status | Where | Test |
|---|---|---|---|---|---|
| Choose one of the Battle Companies and its starting company (units, wargear, gold) | S190-203 | CM | DONE | `startingcompany.go`, `companies.go` `CreateCompany`, [frontend.md](frontend.md) "Choosing a company" | `TestTheCreationFormGivesTheLeaderAndSergeants` (web) |
| Starting 60 points of rating and gold, the 105 total with the three officers' Might, Will, Fate | S2062 (Creating Custom Companies) | CM | DONE | `startingcompany.go`; the data adds up for every company, a company that does not is in `startingTotalExceptions` | `TestStartingCompanyPointsPlusGoldAre105InEveryCompany` |
| Starting gold spent in the Armoury | S214 | CM | DONE | `armoury.go` (gold as a currency), [frontend.md](frontend.md) "Gold" | `TestEquipmentCanBeBoughtWithGold`, `TestWarriorGoldPriceIsTheOptionsPoints` |
| Select the Heroes: one Leader and two Sergeants, new Heroes with 1 Might, Will, Fate; the rest are Heroes in the Making | S204-213 | CM | DONE | `StartingUnits`, `ValidateStart`, `OfficerLock` in `startingcompany.go`, `companybuild.go` | `TestStartingOfficersKeepTheirProfileAndGainMightWillAndFate`, `TestOfficerLocksAgreeWithValidateStartInEveryCompany`, `TestShireRangerStartsAsLeader` |
| Heroes' path chosen on becoming a Hero, with its starting heroic action (Sorcerer: a first power, only one Sorcerer) | S505-515, S963 | CM | DONE | the officer picks it from the company page right after creation (`WithPath`, `LevelService.ChoosePath`); a unit with no path yet is `PathPending` | `TestAnOfficerChoosesItsPathFromTheCompanyPage`, `TestANewSorcererChoosesAPowerAndGetsItsHeroicAction`, `TestOnlyOneHeroOfTheCompanyTakesThePathOfTheSorcerer` |
| Model maximum (15, 12 for some, per company) | company pages, S1996 | CM | DONE | `companybuild.go` `companyBlock`, effect `maxModels` | `TestEffectMaxModels`, `TestCompanyBlockKeepsToKeywordLimitsAndBalance` |
| Name each model; roster sheet (profile, equipment, rules, injuries, XP) | S196 | CM | DONE | [frontend.md](frontend.md) "My Company", "Renaming units" | `TestAPromotionKeepsTheCompanysNameForTheUnit`, e2e `30-company`, `82-company-ui` |
| Optional: You (don't) have my sword (Heroes discard any wargear) | S1398 | CM | DONE | `CampaignSettings.HeroesDropProfileGear`, `DropReason` | `TestTheOptionalRuleLetsHeroesDropProfileGear`, `TestBuildDetailTheOptionalRuleLetsHeroesDropProfileGear` |
| Optional: Custom Selection (build the starting roster from any model of the charts, 60 points) | S202 | CM | N-A (decided not to build) | custom lists are edited in the rules JSON; the 60-point check is not built | none |
| Optional: Custom Company (organiser edits a company's lists) | S1996 | CM | N-A (decided) | an organiser uploads a changed Battle Companies file ([admin.md](admin.md)); there is no in-app editor | `rulesets_test.go`, `ruleset_test.go` (store and validate an uploaded file) |
| Company special rules: limit overrides and exemptions, unique wargear, granted rules, units with their own name, rule exchange | S1992 | CM | DONE | `applyCompanyRules` in `companybuild.go`, [frontend.md](frontend.md) "Company rules and hero upgrades" | `TestEffectLimitOverrideRaisesTheShareOfTheCompany`, `TestEffectLimitExemptionLeavesModelsOutOfTheCount`, `TestEffectCompanyLimitKeepsAKeywordToItsShare`, `TestUniqueWargearForAKindOfModelIsBuyable` |
| Company special rules that act in the game (auras, rule substitutions such as "treat the Leader as Faramir") | S1992 | PH | N-A (played at the table; shown on the cards) | `companies[].effects[]` with `layer` match, shown only | none |
| On Their Own Path (no experience, no promotion; creatures too) | S1994, S1717 | CM | DONE | `OwnPath`, `ownPathLevels`; optional homebrew path when `CustomOnTheirOwnPath` | `TestEffectOnTheirOwnPath`, `TestUnitsOnTheirOwnPathLevelUpAsWarriorsWhenTheCampaignSaysSo`, `TestOwnPathUnitsAreNeverOfficersOrHeroesInTheMaking`, e2e `88-own-path` |

## 2. Recruiting, rarity and limits (Step 3, S1357-1397)

| Mechanic | Source | Layer | Status | Where | Test |
|---|---|---|---|---|---|
| Roll on the Reinforcements chart for the company's price (2, 3, 4 Influence), paid whatever the result | S1359-1363 | CM | DONE | `recruiting.go` `RollRecruit` | `TestRecruitingRollsOnTheChartAndCostsInfluence`, `TestRollCostIsSpentAtOnceAndRaisingAtConfirm` |
| Special Chart result leads to a second roll | S1364 | CM | DONE | `RollRecruit`, `RecruitSpecial` | `TestTheSpecialChartIsASecondRollInTheSameRecruit` |
| Choice of Options (no banner, war horn or drum) | S1986 | CM | DONE | `ChooseOptions` | `TestAChoiceOfOptionsWaitsForTheChoice`, `TestAHobbitRollOffersTheHobbitProfilesTheCompanyLacks` |
| Rare (X) units: at most X in the company, a full chart means a lower result | S1366-1370 | CM | DONE | `rareBlock` in `companybuild.go`, `ChooseLower` | `TestARareUnitCannotJoinOnceTheCompanyHasAsManyAsItMayHave`, `TestALowerResultIsOfferedOnlyWhenTheRollCannotJoin` |
| Special Units bought at their own cost (Cave Troll...) | S1371 | CM | DONE | `RecruitSpecial` | `TestSpecialUnitIsRecruitedUnderTheCompanysName` |
| Wargear limits: bow and throwing limits with Heroes counted, Cavalry at most 33 percent; a roll over a limit means raise the roll or take the next option down; company exemptions | S1373-1377 | CM | DONE | `company.go` `CompanyLimits`, `resultBlock`, `limitBlock` | `TestCompanyLimits`, `TestLimitExemptionsLeaveModelsOutOfTheCountAndTheBase`, `TestARiderWithABoughtMountCountsAsCavalry`, `TestBuildDetailGearInThePackStillCountsTowardsTheBowLimit` |
| Adjust a Reinforcement roll with Influence (1 per point, at most 3 across both charts, not past 6) | S1378 | CM | DONE (house reading: the die is only raised; lowering is a free pick of a lower result, `recruitment.adjust.lowering: never`) | `ConfirmRecruit`, `MaxAdjust` | `TestAdjustingARollAfterSeeingItCostsInfluence`, `TestAdjustingNeverPaysPastASix`, `TestNothingStillCosts` |
| Company-specific recruiting (Vault Warden Team, Warg Marauder, Fornost alternative, Ranger of the North return, model-count units) | company pages | CM | DONE | `recruiting.go`, `recruiting_ranger.go`, `companyprofile.go` effects | `TestAVaultWardenTeamJoinsAsTwoUnitsOfTheirOwn`, `TestAWargMarauderTakesThreePlaces`, `TestFornostMayTakeAHobbitArcherInsteadOnA2To6`, `TestArrangeOfficers`, e2e `50-ranger` |
| Rendezvous perk adjusts the first reinforcement roll (plus or minus 1) | scenario text | CM | DONE | `RollRecruitWithPerk`, `aftermath_perks.go` | `TestRollsNeedTheCampaignSettings`, `TestScenarioRewardsAtFinish` |
| Treat Injuries: 1 Influence removes a Warrior's Injured; a Hero attempt on 5+ (extra Influence modifies); send to the healers (misses the next game, removes one injury) | S1380-1384 | CM | DONE | `treatment.go` | `TestTreatingAWarriorsInjuryCostsInfluence`, `TestTreatingAHeroByAttemptOrByTheHealers`, `TestAnInjuryCountGoesDownOneAtATimeAndTheEffectsFollow` |
| Visit the Armoury: wargear (Warriors only their Options, Heroes any wargear of the charts), armour upgrade stages, equipment, no swapping between models, dropping wargear | S1385-1397, S1446-1710 | CM | DONE | `armoury.go`, `armour.go`, `DropReason`, [frontend.md](frontend.md) "My Company: units" | `TestBuyingEquipmentSpendsInfluenceAndTakesASlot`, `TestArmourUpgradeReplacesTheArmourWithTheNextStage`, `TestDroppingFollowsTheBattleCompaniesRules`, `TestAWarriorMayOnlyDropGearWhenWhatIsLeftIsAnOption` |
| Creatures: only Leader or Sergeants, a company maximum, On Their Own Path, die with their Hero, Baggage Pony pack and swap | S1710-1803 | CM | DONE (the Fly creatures' Handler's Glove rule is shown as text, played at the table) | `creatures.go`, `armoury.go`, `cemetery.go` | `TestCreaturesGoToOfficersAndTakeNoSlots`, `TestTheBaggagePonyStoresAndSwapsGear`, `TestBuildDetailCreaturesGoToOfficersAndTakeNoSlots` |
| Wanderers: hired for Influence, one at a time (data `maxAtOnce`), Heroes without Stand Fast or heroic actions on the company, no experience or advancement | S1804-1830 | CM | DONE: hire and dismiss work, levelling refuses a Wanderer, the aftermath credits it no experience and its card never shows Level up ready | `armoury.go` `HireWanderer`, `levelling.go`, `aftermath.go` `Experience`, `CompanyUnit.PendingLevelUps` | `TestWanderersJoinAsUnitsAndCanBeDismissed`, `TestExperienceSkipsWanderers` |
| Unique Wargear of a company | S1990 | CM | DONE | `battlecompanies.go`, armoury | `TestUniqueWargearSpelledAsTheCompanyPageDoes` |

## 3. Injuries and the injury charts (Step 2.1, S302-358)

| Mechanic | Source | Layer | Status | Where | Test |
|---|---|---|---|---|---|
| Casualties roll 2d6; models that fled the board do not; Creatures and Wanderers rules; Warriors on the Warrior chart, Heroes on the Hero chart | S302-307 | PH | DONE | `aftermath.go` `ResolveCasualty`, [aftermath.md](aftermath.md) | `TestResolveCasualty_EveryRow`, `TestAftermathHeroInjuries`, `TestAftermathWarriorResultsAndRerolls` |
| Warrior chart: Dead (to the cemetery), Injured (misses the next game), Full Recovery, Lesson Learned (+D3 XP) | S342-358 | PH | DONE | `aftermath.go`, `cemetery.go` | `TestResolveCasualty_EveryRow`, `TestCasualtyBonusDice`, `TestBuryMovesTheUnitToTheCemeteryAsItWas` |
| Hero chart: Dead, Arm Wound, Leg Wound, Broken Honour (second time Fearful), Just a Scratch (roll again, second result stands), Full Recovery (heal one injury), Protection by the Valar (+1 Fate to the path maximum), Wounds of a Hero (+D6 Influence) | S308-334 | PH | DONE | `aftermath.go`, `injuries.go` (effects from `campaign.injuries`) | `TestScratchReroll`, `TestWoundsOfAHeroWithAnInjuryKeepsItsBonus`, `TestBuildDetailHeroInjuriesChangeItsProfileAndGear` |
| Forced retirement: second Arm or Leg Wound, third Broken Honour, as Dead | S313-324 | PH | DONE | `retireAtCount` in `injuries.go`, `aftermath.go`; fate "Retired" in the cemetery | `TestResolveCasualty_Retirement`, `TestAHeroRetiresOnItsSecondArmOrLegWound` |
| Injury effects on the profile and gear (Move minus 1, no shield or two-handed weapon or bow, no Stand Fast, no heroic actions on the company) | S311-322 | CM | DONE | `injuries.go`, `companybuild.go` | `TestBuildDetailHeroInjuriesChangeItsProfileAndGear` |
| Injured models miss the next game and are left out of the Rating | S1403, S308 | CM | DONE | `BuildDetail` (`Points` against `FullPoints`), aftermath start recovers absent units | `TestBuildDetailInjuredUnitsLeaveTheRatingButNotFullPoints`, `TestAftermathStartRecoversAbsentUnitsAndCreditsRerolls` |
| Death of a Hero: Leader slain means a Sergeant becomes Leader; fewer than two Sergeants means another Hero may become Sergeant; no Hero left means the Warrior with the most XP becomes a Hero in the Making | S335-340 | PH | DONE | `aftermath.go` `NextSuccession`, `NextSergeant`, [aftermath.md](aftermath.md), [definitions.md](definitions.md) "Officer succession" | `TestSuccession`, `TestSergeantSuccession`, `TestAftermathLeaderFallsAndSuccessionIsAskedFor`, `TestAftermathDeadSergeantNeedsAWarriorWhenNoHeroIsLeft`, `TestAftermathLeaderAndSergeantLostTogether` |
| Ranger of the North may return as a reinforcement (Shire, Helm's Deep) | company page | CM | DONE | `recruiting_ranger.go` | `TestRangerMayReturnOnTheCompanyDetail`, `TestArrangeOfficersWithFewerThanThreeOfficers` |
| Healing Herbs (+1 to every injury roll) and Wondrous Cram (skip the roll) | S242-262 (Healer's Kit), equipment lists | PH | DONE | `aftermath_items.go` | `TestHealingHerbsAddOneToEveryRoll`, `TestHealingHerbsClampAtTwelve`, `TestWondrousCramSkipsTheRoll`, `TestWondrousCramOnAHeroStillHealsAnInjury` |
| One aftermath per company, guided sheet, locked company while pending, waiver by the organiser | S296-302 | PH/APP | DONE | `aftermath_service.go`, [aftermath.md](aftermath.md) | `TestAftermathStatusLocks`, `TestCompanyActionsAreLockedWhileAnAftermathIsPending`, `TestWaiveAftermath`, e2e `81-aftermath-ui` |

## 4. Experience, progression, promotions (Step 2.2 and 2.3, S450-614)

| Mechanic | Source | Layer | Status | Where | Test |
|---|---|---|---|---|---|
| Experience gains (+1 each: participating, wounding an enemy, slaying a Hero or Monster, Fate point used, Courage test passed when Broken, objective, MVP); Warriors and Heroes alike | S457-466 | PH | DONE | `campaign.experience.gains`; sheet in `aftermath.go` `Experience`; filled from the live log by `AftermathDefaults` | `TestExperience`, `TestLiveAftermathDefaults`, `TestObjectiveFlags`, `TestMVPVotesTallyAndUndo` |
| Mount wounds and a slain rider's mount still earn the experience | S468-470 | PH | DONE | `live_fold.go` (mount part of a wound) | `TestMountWounds` |
| XP is cumulative, 5 XP per roll, rolls wait on the card | S474, S505-513 | CM | DONE | `PendingLevelUps`, `CompanyUnit.LevelEvery`, `levelling.go` | `TestLevelUps`, `TestAWarriorLevelUpFollowsTheD6Table` |
| Warrior progression: 1-2 No Change, 3-5 Promoted (choose when several fit), no promotion means Courage and Intelligence plus 1 up to 3+, 6 Hero in the Making (or forgo it for the promotion) | S478-503 | CM | DONE | `levelling.go` (`noPromotion`, the "Forgo the Hero's Journey" choice), `promotion_gear.go` | `TestAWarriorLevelUpFollowsTheD6Table`, `TestEveryMatchingPromotionIsOffered`, `TestEveryPromotionAppliesTheOneRule`, `TestPromotionWargearThroughTheLevelService` |
| Hero progression: two rolls on the path chart, choose one; a hero takes each result once; path exhausted means nothing left | S505-515 | CM | DONE | `progression.go`, `levelling.go` | `TestAHeroLevelUpOffersTwoResultsAndAppliesTheChoice`, `TestARollAgainResultIsChosenTogetherWithTheOtherOne` |
| Paths (12), minor special rules, heroic actions, magical powers and casting values, Fury species | S614-1356 | CM | DONE | `progression.go`, data `paths`, `minorSpecialRules`, `magicalPowers` | `TestMinorRulesWithATargetAskForOne`, `TestSorcererRowsOfferPowersAndCastingImprovements`, `TestProgressionRulesAreReadFromTheFile` |
| Characteristic limits (average, peak, increases counted from level-ups) | S536-560 | CM | DONE | `progression.go` `improvedStat` | `TestAHeroMustTakeTheOnlyResultItCanTake` |
| Company Hero Upgrade as a replacement for a path result | S1988 | CM | DONE | `levelling.go` | `TestAHeroMaySwapAResultForItsCompanysUpgrade` |
| Reroll bonus of Against the Odds usable on a progression or injury result | S250-252 | PH | DONE | `Rerolls` on the company state | `TestAnAgainstTheOddsRerollReplacesOneResult`, `TestAftermathLastRerollClosesTheStep` |
| Bonus Progression Rolls ("free" advancements a scenario text grants) | S516-518 | PH | N-A (no scenario in the data grants one; the data has the `bonusRolls` field but nothing feeds it) | `campaign.progression.xp.bonusRolls` | none |

## 5. Influence, gold, hero rating and points (Step 2.4 and Step 4)

| Mechanic | Source | Layer | Status | Where | Test |
|---|---|---|---|---|---|
| Influence: 2 for taking part, +1 for a Draw, +2 for a Win; Against the Odds Influence bonus | S1348-1356 | PH | DONE | `aftermath.go` `Influence`; written as the aftermath finishes | `TestInfluence`, `TestScenarioRewardsAtFinish` |
| Gold: only from the starting company and organiser grants; never from scenarios | S214 | CM | DONE | `grant.go`, `campaign.gold` | `TestGrantInputValidate`, `TestCanGrant`, `TestMatchesGrants` |
| Scenario rewards (cargo Influence, spoils, survival XP, Rendezvous, Vantage Point, Final Blessing) | scenario texts | PH | DONE | `aftermath_perks.go`, [aftermath.md](aftermath.md) | `TestRewards`, `TestScenarioRewardsAtFinish`, `TestVantagePointMovesTheSecondScenarioDie` |
| Warrior rating: its MESBG points plus the rating of equipment | S1405-1406 | CM | DONE | `companybuild.go` `pointsOf` | `TestBuildDetailPointsFollowTheBattleCompaniesRating`, `TestRatingFor` |
| Hero rating: Warrior profile points, 5 per Might, Will, Fate and per Move, Fight, Strength, Defence, 10 per Attack and Wound, 5 per special rule and 5 per minor special rule (with or without a target, at most 10 in all; a rule an item grants, Envenom Weapon's Poisoned Attacks, is the item's and does not count), nothing for Shoot, Courage, Intelligence, heroic actions, magical powers; advancements up to the average profile add nothing | S1407-1420, S560 | CM | DONE | `BCBook.HeroRating`, `pointsOf`, `UnitPath.MinorRules` | `TestHeroRatingOfGearOutsideTheProfileAndItsOptions`, `TestHeroRatingOfMinorSpecialRules` |
| Hero wargear and equipment: free when in the base profile or its Options; paid at the hero column once Attacks plus Wounds reach 3; armour stages | S1421-1445 | CM | DONE | `pointsOf`, `armourRating` | `TestHeroArmourInsideTheOptionsIsFree`, `TestArmourUpgradesAreOfferedOneStageAtATimeInEveryCompany` |
| Company rating is the sum, Injured models excluded (shown as "Points (full points)") | S1403 | CM | DONE | `BuildDetail`, `MatchSide.Rating` (injured counted for the comparison) | `TestBuildDetailInjuredUnitsLeaveTheRatingButNotFullPoints`, `TestCampaignsRateCompaniesFromTheRules` |
| Company standings by points | none (app) | APP | DONE | `Rank` in `campaign.go`, Rankings tab | `TestRank` |

## 6. The game: scenarios, Against the Odds, the match (Step 1, S218-300, S5247-)

| Mechanic | Source | Layer | Status | Where | Test |
|---|---|---|---|---|---|
| Find an opponent, create the match (any number of sides and companies) | S218-224 | PH | DONE | `matches.go`, [matches.md](matches.md) | `TestMatchesCreate`, `TestSideRolesValidationSwapAndEnd`, e2e `40-match` |
| Compare ratings, Top Dog and Underdog | S5249-5251 | PH | DONE | `Match.TopDog` in `scenarioroll.go` | `TestTopDog` |
| Scenario table: D6 for the column, D6 for the scenario (18 scenarios); or choose | S5252-5282 | PH | DONE | `ScenarioByRolls`, `ScenarioTable`; the match page picker | `TestScenarioByRollsUsesTheRealFile`, `TestMatchesSetScenarioAndAccess` |
| One-off scenario for a game outside the rulebook | none (app) | APP | DONE | [matches.md](matches.md) "One-off scenario form" | `TestScenarioInputValidate` |
| Scenario text: armies, layout, starting positions, how the game ends, special rules, tables, roles | S5340- | PH | DONE | `ScenarioAbout`, `ParseScenarios` | `TestParseScenarios`, `TestParseScenariosReadsTheObjectivesOfTheRealFile`, `TestScenarioRuleTableRowFor` |
| Objectives and Victory Points per scenario, winner, draw (scoring is a tally, the players decide the result) | scenario texts | PH | DONE | `ScoringTier`, `ValidateResult`, match result form | `TestValidateResult`, `TestScenariosKnowTheirScoringObjectivesBySide`, `TestMatchesSaveResultKnowsRulebookObjectives` |
| Board 48 by 48 inches, terrain | S5283-5286 | PH | N-A (physical) | none | none |
| Scenario special rules: Command Support, NPC scenarios (Neutral Third-Party, Aggressive, Passive, Resolving Combat), Allies of Convenience | S5308-5339 | PH | N-A (played at the table; the text is shown on the scenario card) | `ScenarioAbout.Rules` | `TestParseScenarios` |
| Maelstrom of Battle (models arriving late count for the Break Point) | S5287-5307 | PH | PARTIAL: the text is shown and the Break Point has a manual adjustment for models yet to arrive; no arrival roll or tracker (by design, a table rule) | `ScenarioAbout.Uses`, `breakAdjust` in `live_break.go` | `TestBreakPointModelsAndExtras` |
| Against the Odds: the strictly lower rated company buys bonuses up to the gap, after the scenario roll and before setup, ratings compared before bonuses | S241-300 | PH | DONE | `ato.go`, `ato_service.go`, [matches.md](matches.md) | `TestAgainstTheOddsCard`, `TestAgainstTheOddsEqualRatings`, `TestSaveAgainstTheOdds` |
| Bonuses Influence, Experience, Reroll (the aftermath applies them) | S244-253 | PH | DONE | `aftermath.go`, `ato.go` | `TestAftermathStartRecoversAbsentUnitsAndCreditsRerolls`, `TestInfluence`, `TestExperience` |
| Tool Kit (five kits, items on models ignoring capacity and alignment, gone after the aftermath) and its consumables | S254-290 | PH | DONE | `ato.go`, `live_service.go` overlay, [live-match.md](live-match.md) | `TestLoanedHealerKitItemsAreElectableAndSpent`, `TestAftermathSheetIsPrefilledFromTheMatchSelection`, `live_ato_test.go` |
| Temporary Wanderer for the match (45, no experience or injury roll, counts for the Break Point) | S291-293 | PH | DONE | `ato.go`, `live_service.go` | `live_ato_test.go`, `TestAllIsLostKeepsAgainstTheOddsBonusesWithTheSelection` |
| Ambush! (the lower company chooses the scenario) | S294-295 | PH | DONE | `ato.go` | `TestAmbushSetsAndRestoresTheScenario` |
| Keep track of wounds inflicted, casualties and by whom, fled models | S229-231 | PH | DONE | the live match log, `live_fold.go`, [live-match.md](live-match.md) | `TestWoundDeclarations`, `TestFoldIsDeterministicAndPure`, `TestLiveCrossCompanyWoundAndUndo` |
| End of the match: all participants confirm, organiser override, reopen | none (app) | APP | DONE | `match_end.go`, [matches.md](matches.md) | `TestProposingToEndWaitsForTheOtherOwners`, `TestOverrideEnd`, `TestMatchesEndAndReopen` |

## 7. The live match tools (companion at the table)

The rulebook has no live-play record; these tools are app design that serve rules (experience, Break Point, All is Lost).
Details in [live-match.md](live-match.md).

| Mechanic | Source | Layer | Status | Where | Test |
|---|---|---|---|---|---|
| Trackers for Wounds, Might, Will, Fate, mount, zones (Field, Retreated, casualties), action trackers that record experience flags | S457-466 | PH | DONE | `live.go`, `live_fold.go`, `live_service.go` | `TestFoldStatsSetAndZones`, `TestCheckRefusesImpossibleStates`, `TestFateBrokenRetreatDiedMountReturn` |
| Match log with undo; live sync for viewers (SSE) | none (app) | APP | DONE | `live_hub.go`, `live_fold.go` | `TestLiveUndoThatBreaksALaterEntryIsRefused`, `TestLiveSubscribeRules`, `TestHubCoalescesEventsForASlowClient` |
| Break Point (half the starting models, 66 percent for Muster of Isengard), Broken at the turn boundary, Courage tests, Heroic Resolve | break point rules, S4131 | PH | DONE (the app counts and prompts; players roll) | `live_break.go`, `breakPointPercent` | `TestBreakPointEdges`, `TestBrokenOnlyAtTheTurnBoundaryAndForGood`, `TestCourageTests`, `TestBreakRulesFromTheFile` |
| All is Lost! (a Broken company flees: the opponent wins and claims the rewards; the fleeing company gets the Influence for taking part) | S225-228 | PH | DONE (the fleeing company keeps its Against the Odds bonuses) | `live_break.go`, `aftermath.go` | `TestAllIsLost`, `TestAllIsLostFightOnAndTheOfferExpires`, `TestAllIsLostKeepsTheExperienceBonus` |
| MVP voted by the opposing player; objective flags | S466 | PH | DONE | `live_fold.go`, `MVPHint` | `TestMVPVotesTallyAndUndo`, `TestMVPHintForTheAftermathSheet`, `TestLiveObjectiveAndMVPEndChecks` |
| To Wound chart reference | MESBG | PH | DONE | `towound.go`, [matches.md](matches.md) | `TestToWoundChartOfTheShippedGlossary`, e2e `93-to-wound` |
| Unit keywords on cards (race, faction, type; mounted state) | none (app) | APP | DONE | `keywords.go`, [unit-keywords.md](unit-keywords.md) | `TestUnitKeywordsCases` (`keywords_test.go`), e2e `79-keywords` |

## 8. Company management and campaign structure that are app design

| Mechanic | Source | Layer | Status | Where | Test |
|---|---|---|---|---|---|
| Cemetery of the dead and retired | none (app) | CM | DONE | `cemetery.go` | `TestSaveCemeteryOrder`, `TestTheFallenCountTowardsNoLimit` |
| Formation order of the roster | none (app) | CM | DONE | `OrderUnits` | `TestOrderUnitsHonoursTheSavedOrder` |
| Organiser grants of Gold, Influence, XP with a note | none (app) | APP | DONE | `grant.go` | `TestGrantInputValidate`, `TestMatchesGrants` |
| Campaign: invite links, members, pause, end, hand over, settings (shake the table, manual rolls, optional rules) | none (app) | APP | DONE | `campaigns.go`, `invites.go`, [admin.md](admin.md), [auth.md](auth.md) | `TestSetCampaignStatusRules`, `TestCampaignsHandOver`, `TestCampaignsSettings` |
| Calendar, rounds, campaign victory, alliances between companies | none (not in the rulebook) | APP | N-A (not built) | none | none |
| Copy a company as a Tabletop Simulator army list | none (app) | CM | DONE | `ttslist.go`, [tabletop-simulator.md](tabletop-simulator.md) | `ttslist_test.go`, `TestTTSListAllCompanies`, e2e `91-tts` |

## Differences from the printed rules

No mechanic of the rulebook's loop is missing. Everything marked N-A is physical or played at the table (auras, rule substitutions, scenario special rules, the arrival rolls of Maelstrom of Battle). The deliberate readings are:

- a reinforcement roll can only be raised with Influence (lowering is the free pick of a lower result);
- the company that flees with All is Lost keeps its Against the Odds bonuses;
- the Mirkwood Warg Marauder counts as three models for the Break Point.

Not built by design: Custom Selection and Custom Company (the rules file is edited instead), Bonus Progression Rolls (no scenario grants one in the data; if a future scenario does, feed `campaign.progression.xp.bonusRolls`), a calendar, campaign victory, standings beyond the points rank, and alliances.
