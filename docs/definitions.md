# Rules definitions

The game rules are data, in `definitions/`. The JSON says **what** the rules are; the (future) rule engine decides
**how** to apply them. Most special rules only matter during matches, which are played away from the app, so the app
mostly displays them next to the unit, item or company they affect; a few (injury rolls, progression, influence)
change the campaign and are applied by the engine.

| File | Role |
|---|---|
| `vanilla/data2024.json` | The base game: units, wargear, special rules (keywords), magical powers. A **reference store**, never edited |
| `battle_companies/battle_companies.json` | Everything specific to Battle Companies 2nd CE. **Generated.** The source of truth for the rule engine |
| `glossary/mesbg_glossary.json` | Concepts of the MESBG rules manual that the vanilla file leaves out (Heroic Actions, Stand Fast, the turn phases…); from the 2024 rules manual |
| `concepts/concept_graph.json` | The reviewed edges between concepts (`grants`, `cancels`, `lifts`, `requires`) of the three files above, for the helper popups. **Generated** by `tools/build_concept_graph.py`, never uploaded by users; see [`concept-graph.md`](concept-graph.md) |

Every file we author (Battle Companies, glossary) starts with a top-level `name` and `version`, both strings: the
Battle Companies ones come from the rulebook's title page ("Stable Released Version 1.1" is `1.1`), the glossary's
version is ours and is bumped when its content changes (`GLOSSARY_VERSION` in `tools/build_glossary.py`). The vanilla
file is a third-party store that is never edited, so it keeps `game` and `appVersion`; `core.RulesetMeta` and `meta()`
in `ruleset_picker.js` are the only places that know this. The hash covers the whole file, so any change to these
fields is a new ruleset.

Schemas next to each file (`*.schema.json`) are checked by `tools/validate_definitions.py`. How to regenerate the files
and what the tools do: [`tools/README.md`](../tools/README.md).

## Dependency direction

References only point one way: **Battle Companies → vanilla and glossary**, and glossary terms may point to other
glossary terms (`see`). Vanilla and the glossary never refer to the Battle Companies file. A consumer loads the Battle
Companies file and looks up any name it does not define in vanilla, then in the glossary. Concepts that the Battle
Companies rulebook itself defines belong in the Battle Companies file.

## How the Battle Companies file relates to vanilla

- **It complements vanilla.** It never redefines a unit. A name used by a company resolves to a vanilla key as a base
  unit with an option pre-selected (`selectedOptions`, for example Tookish Hunter = Hobbit Archer + that option), a unit
  a name the document prints for a vanilla unit (2018 name or abbreviation, printed name kept as `label`), or a vanilla unit plus a delta (`addSpecialRules`,
  `addKeywords`, for example Ranger of Ithilien). Each company lists these in `unitAliases` with an explanation.
- **Resolution order:** a definition embedded in the entity that uses it, then the Battle Companies file, then vanilla.
  Battle Companies wins: a rule with the same name as a vanilla rule is its generalisation (a named character becomes
  "this Hero") and carries `overrides`. Vanilla scenarios and army bonuses are not used.
- **Names defined twice in vanilla:** the loader takes the **first occurrence**. Every duplicate whose entries differ (and every one the file needs) is
  reviewed in `data.vanillaAmbiguities`: `equivalent` (only wording differs), `overridden` (Battle Companies defines the
  name), `firstWins` (they really differ, the first is right) or `pinned` (the rule of thumb fails; `occurrence` says
  which entry wins; occurrences count the heroes list before the warriors list, case-insensitively, as the loader does).
  Pinned today: Peregrin Took and Bain, Son of Bard (the first entry is incomplete). Notes starting `REVIEW:` mark
  value differences settled by the default (first wins) that a person may want to confirm.
- **Parameterised rules** (`data.parameterisedRules`): Hatred (X), Poisoned Attacks (X), Master of Battle (X+),
  Dominant (X), Fury (X), Combat Synergy (X), Terror (X) say what X is (`keyword`, `weapon`, `rollTarget`, `number`,
  `companyHero`) and who picks it.
- **Company rule rewiring:** `ruleSubstitutions` (the Leader counts as Faramir for Loyal to the Captain),
  `ruleExchanges` (Hatred (Rohan) becomes Hatred (Hobbit)) and `grantedSpecialRules` (free rules).

## Contents of the Battle Companies file

- `campaign`: the core tables with stable keys the engine can switch on: injuries, experience and progression,
  characteristic limits, influence, rating, Against the Odds, scenario selection.
  **Costs are properties**, so a modified file can change them: `campaign.influence.spend[]` (`recruit.defaultCost`, a
  company's own `data.companies[].reinforcements.influence`, `adjustRoll.costPerPoint/maxPerRoll`,
  `treatInjuries.cost`; `armoury.costSource` lists where item prices live: `influence` on wargear, equipment,
  creatures, wanderers, unique wargear and special units, `wargear.armourUpgrade.influencePerUpgrade`), the Against
  the Odds `rating`s and `gains`, and `campaign.injuries.treatment` (see below).
- `campaign.injuries.hero[]` / `warrior[]` entries carry, besides `roll`, `key`, `effects` and `text`: `persistent`
  (counted per Hero), `healable`, `retireAtCount`, `grantsAtCount`, `statChanges[{stat, delta, per:"count"}]`,
  `disables[{kind: gearClass|gearItem|ability, classes|names|key}]` (gear classes are those of
  `data.gearAnnotations`), `heals`, `reroll`, `grantsInfluence`, `grantsExperience`. `campaign.injuries.treatment`
  holds the healing options (cost, roll `{dice, target, modifier}`, `healable` keys, `missNextGame`,
  `removalEffect`). Each method has its own `healable` list: the persistent injuries, plus, for `attempt` only, the
  miss-next-game status `justAScratch` (an outcome with `healableBy`). `maxAttempts: null` means unlimited retries. The
  Valar's Fate is `capAtPathMaximum`; a Leg Wound's `minimum` Move is null (the rules state none). The prose in `healing` and `influence.spend` stays for display.
- Level-up: `campaign.progression` (`xp`: cumulative, rolls = floor(XP / `threshold`), nothing is spent; `timing`: War's
  Aftermath step 3; `influenceAdjust`/`mightAdjust`: false; `warrior.table[]` + `warrior.effectsByKey`; `hero.resolution`:
  2d6 twice, choose one, unavailable results re-rolled, Company Hero Upgrade swap), `campaign.characteristicLimits`
  (`average`, `peak`, `lowerIsBetter`, `limitKinds`), per path `data.paths[].maximums` and `progression[]` rows whose
  `options[]` are alternatives (`improveStat`, `specialRule`, `minorSpecialRule{maxTotal}`, `heroicAction{maxTotal}`,
  `magicalPower`, `improveCastingValue`, `oneOf`).
- Magic: `data.magicalPowers.items[]` (all 43 vanilla powers: `name`, `duration`, `definition`, `learnable`, and for the 31 on the
  Path of the Sorcerer's Spell List the `range` (inches or "self") and Casting Value `casting` a Hero learns them at) and
  `data.magicalPowers.castingRule` (X+, improving lowers the number by 1, at most 2 per power, floor 2 inferred). The 12 non-learnable powers (Chill Soul, Immobilise, Paralyse, Wrath of Bruinen, Your Staff is Broken, 7 Fury species) have no base range/casting: unit profiles differ per unit and the Spell List does not offer them. The validator checks that every vanilla power has an item and that the learnable ones equal the Spell List. A path's
  `startingHeroicAction` ({name, definition} or null when the path only has a Mastery rule for a universal action) and the
  Sorcerer's `startingMagicalPower` say what a Hero gets when it takes the path.
- Gold: `campaign.gold` (spendable on, when, what it cannot buy, the 60-point guideline, `inferred`/`undecided` keys) and a
  `gold` property on every purchasable thing, equal to its initial rating: `{warrior, hero}` on `data.equipment.items[]`,
  `data.wargear.items[]`, `data.wargear.armourUpgrade` and `data.companies[].uniqueWargear[]`; a number (its points) on
  `data.creatures.items[]` and `data.wanderers.items[]`. Starting Gold is `data.companies[].startingCompany.gold`.
- `campaign.aftermathEffects[]`: every rulebook source of re-rolls and aftermath bonuses (Against the Odds Reroll / Influence /
  Experience, and the scenario rewards Rendezvous (`when: firstReinforcementRollThisAftermath`: the +1/-1 applies in the aftermath right after the match although the scenario prints "subsequent"; user decision 2026-10-21), Vantage Point, To the Victor the Spoils, A Final Blessing, Survival of the
  Fittest, Cargo). Scenario rewards carry `scenario`, `who`, a typed `kind` (`reinforcementRollModifier`,
  `scenarioTableModifier{die, range}`, `freeEquipment{maxInfluenceValue}`, `magicalPower{power, ignoresWill}`, `experience{amount, to}`,
  `influence{amount}`) and `when`. The Reroll bonus has `count: 2`, `usableIn`, `carriesOver: false` (inferred).
- Injury-roll items (`campaign.aftermathEffects[]`, kinds `injuryRollModifier` / `injuryRollReplacement`; the item carries
  `aftermathEffect`, the effect `source: data.equipment.items[name=...]`). Both are elected by the player before the injury rolls
  of step 1 and removed from the Equipment (`consumes`). `healingHerbs`: carried by a Hero that took part and is not a
  casualty (`activatedBy`), +1 (`modifier`) to the 2d6 total of every Injury roll of the company, `stackable: false`, one use per
  aftermath, result clamped to the chart (`clamp` [2, 12], also on the Hero's re-roll). `wondrousCram`: carried by any model
  removed as a casualty that took part, no roll, result `fullRecovery` (the Hero's includes healing one Injury). A model that
  fled the board counts as not a casualty for both (`fledModelIs`); `defaultNotConfirmed` lists the readings not yet confirmed.
  `campaign.injuries.rollModifier` says how a modifier is applied; `matchInput.perModel[key=usesItem]` records the election.
- Against the Odds (`campaign.againstTheOdds`): `selectedAt` (after the Scenario Table roll, before setup), `eligible` (the
  strictly lower rated company, ratings compared before the bonuses), `budget` (rating after the bonuses may equal but not exceed
  the opponent's), and per bonus `appliesIn` (setup / game / aftermath / scenarioChoice). `toolKit.purchase`: one purchase (30
  Rating) is one Kit, no Kit twice (up to 5); `toolKit.grants` (`loanedEquipment`): items assigned to any model of the match
  before setup, ignoring capacity and alignment but not Hero only, no extra Rating, gone after the match's aftermath.
  `wanderer.grants` (`temporaryModel`): one Wanderer from `data.wanderers.items` for the match, priced at the flat 45 Rating
  (no Influence, no Gold, no roster slot), counts for the Break Point, no injury roll, gone after the match. `ambush.grants`
  (`chooseScenario`): the lower rated company picks the scenario instead of the rolled one. `matchInput.perCompany` has
  `toolKits`, `temporaryWanderer`, `ambushScenario`.
- Break Point in play (`campaign.breakPoint`): `default.percent` 50 of the starting models (companies override it with the
  `breakPoint` effect: Muster of Isengard 66), `threshold` (casualties strictly greater than the not-rounded Break Point, tested
  as `casualties * 100 > percent * startingModels`), `startingModels` and `casualties` (what counts: Heroes, creatures, the
  temporary Wanderer, models yet to arrive; fled models are casualties; a Cavalry rider's death is the casualty, not the mount's),
  `evaluatedAt` the start of each turn and permanent, `whenBroken.courageTest` (every activation, before anything else; fail =
  flees and is removed; `autoPass`), `whenBroken.standFast`, `allIsLost` (decided once, at the start of the turn the company
  becomes Broken, before Priority; opponent wins with full rewards; the fleeing company keeps participation plus its Against the Odds Influence and Experience bonuses, no Win/Draw Influence, no scenario rewards; user decision mqn.114), `experience` (the
  `brokenCourageTestPassed` gain: +1 per model that rolled and passed, automatic passes do not count) and `scenarioExceptions`
  (Defend the Hill, Let nothing through!, Only a Pony After All, Neutral Third-Party, Maelstrom of Battle), each with a short player-facing `text`.
  `matchInput.perCompany` has `startingModels` (computed), `brokenAtTurn` and `fledBroken`.
- Company page: `data.companies[]` carries description, alignment, `maxModels` (absent = 15), `startingCompany{units, gold}`,
  `reinforcements{influence, table}`, `specialChart`, `specialUnits`, `uniqueWargear` (cost, rating, `gold`, `definition`,
  `limit`, `restriction`, `profile`), `advancements`, `heroUpgrades[{name, flavour?, restriction?, specialRule}]` (the rule
  text is `data.keywords[name=specialRule].definition`), `specialRules` (prose), `notes[{section, text}]` and
  `effects[]`: every company rule as a typed effect with a `layer` (management / match / both); the vocabulary is
  documented in `tools/bc/company_effects.py`. Applied by the app today: `recruitAsHero` (every recruit joins as a Hero in the Making), `uniqueWargear[].restriction` (who may take the item: "Hobbit Heroes", "Corsair") and `limit` (most models carrying it); a unique item with a `restriction` is not tied to a profile option. Company Hero Upgrades: a Hero may swap ANY rolled result of a level-up for
  its company's upgrade if it does not already have it (rulebook, Path Progression).
- `data.companies` (37): starting company, reinforcement and special charts (D6 tables), Company Hero Upgrade,
  advancements, unique wargear, special rules and limits. `specialRules[]` holds the company's rule sentences exactly as printed;
  a sentence that also has a typed form (`ruleSubstitutions`, `ruleExchanges`, `grantedSpecialRules`, `limitExemptions`) is
  repeated there on purpose and the app prints it once (`CompanyProfile.OwnRules`). `epigraph {quote, text}` (Arnor) is the quoted
  title and flavour sentence that head the rule text in the rulebook: flavour, not a rule. `tools/company_text_checks.py`
  (run by the validator) rejects a rule repeated inside `specialRules`/`notes` and parse artefacts in company texts (unbalanced
  quotation marks or brackets, an epigraph glued to a rule, fragments at either end, page headers). A name from the 2018 books
  that the document still prints in a list and that is only a rename (High Elf Warrior = Rivendell Warrior) is corrected when the
  document is read (`SOURCE_TYPOS`), so the list names the 2024 unit with no alias card; `LEGACY_NAMES` is empty today. Same for "Giant Mirkwood Spider" (Cirith Ungol), which the document itself prints as "Mirkwood Giant
  Spider" in its other lists.

### Unit names that differ from vanilla (`unitAliases` cards, reviewed)

Every name below is resolved once; the card text is generated (`link_unit_notes`) and says only what the sources support. A generated entry whose printed name equals the base unit name (ignoring case and accents) gets no `label` and no card; the
card heading in the creation page is "Printed name".

| Printed name (company) | Decision | Evidence |
|---|---|---|
| Giant Mirkwood Spider (Cirith Ungol) | (a) typo, `SOURCE_TYPOS`, no card | the same document prints "Mirkwood Giant Spider" in the Dol Guldur lists; vanilla key is Mirkwood Giant Spider (20 pts) |
| Grimhammer, Warrior of Erebor (Erebor & Dale); Iron Hills Dwarf (Defenders of the North, Iron Hills); Mirkwood Cavalry (Halls of Thranduil); Abrakhân Guard (Harad); Rhûnish War Drake(s) (Easterlings) | (a) 2018 name or abbreviation, `SOURCE_TYPOS` (owner decision), no card | no vanilla unit has these names; mapped to Grim Hammer Warrior, Erebor Dwarf Warrior, Iron Hills Warrior, Mirkwood Elf Knight (mounted, 16 pts; the document's step is "Mirkwood Elf with Elven-made glaive"), Abrakhan Merchant Guard (`legacy: true` in vanilla), War Drake of Rhun. REVIEW: the mappings are best matches; Mirkwood Elf Knight is the least certain. The 105-point test over all companies is the check on prices |
| Helminga, Lorien Guard, Noldorin Exile, Tookish Hunter, Battlin' Brandybuck | (b) base unit + vanilla option (`selectedOptions`) | vanilla options Helmingas (Warrior of Rohan), Lorien Guard (Galadhrim Warrior), Noldorian Exile (Lothlorien Warrior), Tookish Hunter (Hobbit Archer), Battlin' Brandybuck (Hobbit Militia), each +1 point, reserved to a named hero's list in vanilla; the document names them directly (Noldorin Exile: "a Lothlorien Warrior with Gildor Inglorion's unique upgrade"). The card now states the +1 point and who can take the option in the base game |
| Ranger of Ithilien (Osgiliath & Ithilien) | (b) delta `addSpecialRules: Loyal to the Captain` | document: "a Ranger of Gondor with the Loyal to the Captains special rule at no additional cost" |
| Goblin Hulk (Goblin-Town) | (c) delta `addKeywords: Goblin` on the Gundabad Ogre, Rare 1, 15 Influence | document: "using the profile of the Gundabad Ogre with the Goblin keyword, for 15 Influence Points" (the vanilla Ogre costs 60 points). The card says so |

Check against the 105-point rule (points plus starting gold, see the app's `startingtotal_test.go`): every company totals 105
except Defenders of the North (106). The mapping Iron Hills Dwarf = Iron Hills Warrior is confirmed by the Iron Hills company
(3 with shield & spear 12, mattock 11, crossbow 12 = 59, plus 1 gold = 60) and Warrior of Dale by Erebor & Dale (2 Erebor
Dwarf Warriors with shield 10, spear 10, shield & spear 9, two Esgaroth bows 9 = 57, plus 3 gold = 60). Defenders of the North
prints no gold and its six warriors (12, 11, 11 and 9, 9, 9) cost 61 at the 2024 prices: no other vanilla unit fits the
Iron Hills Dwarf's options (shield & spear, mattock) while keeping Iron Hills at 60, so the 106 is a book inconsistency, kept
as the one documented exception.
- `data.paths` (11 heroic + End of Path + custom): hero paths with maximums and the 2d6 progression table; `data.minorSpecialRules`.
  Each path has `description` (the document's intro sentence, absent for the three paths that add no Heroic Action; their
  `startingNote` says so) and `offers`, derived from the table for the path picker's tooltip: `summary` (ready-made text: starting
  Heroic Action / Magical Power, Mastery rule, own special rules, signature characteristic, choices), `startingHeroicAction`,
  `startingMagicalPower`, `masteryRule`, `specialRules[]` (without the Mastery rule), `signatureStats[]` and `choices[]`
  (`minorSpecialRule`, `heroicAction`, `magicalPower`, `improveCastingValue`). Show `summary` as the tooltip; list the parts
  (rules linking to their definitions in `data.paths[].progression[].options[].specialRule`) in the path detail.
- `data.wargear`, `data.equipment`, `data.creatures`, `data.wanderers`, `data.keywords` (Battle Companies rules),
  `scenarios`, `scenarioScoring` and `data.scenarioDetails`. A scenario special rule that prints a table carries `table {lookup, columns, rows[{min, max, cells}]}` (`lookup` `d6`: Sites of Power's Strange Aura; `combinedRating`: Tame the Beast's Monster Budget by the two companies' combined rating, `max` null = open ended); the `definition` keeps only the text around the table. A scoring objective flagged `secret` (Fog of War) also carries `reveal: "endOfGame"`, `completion: "tiers"` (the player reports the best tier reached) and, when the player notes something down at the start, `nomination {from: ownHeroes | enemyHeroes | terrain, allowLeader: onlyHero | where: opponentHalf}` (own Hero, opponent's Hero, a terrain piece wholly in the opponent's half; the Leader only if the company has one Hero); `instruction` keeps the printed sentence. The validator checks the nomination against the instruction. A scenario lists rules it applies without text of their own in `usesRules` (Maelstrom of Battle, Dark of Night, Heirloom of Power). The Wolves Attack prints one Victory Point bullet twice (the Broken company); it is kept as printed.
- Scenario objectives, rewards and roles (`data.scenarioDetails[]`, built by `tools/bc/scenario_rules.py`; the prose fields
  `objectives`, `scoring`, `specialRules` stay for display, `scoring` no longer holds the reward paragraphs the layout glued
  to its last bullet). New per scenario: `roles[{key, name}]` (`attacker`/`defender`, only Stand Your Ground, Defend the Gate,
  Hold the Line, Baggage Train), `outline` (Seize the Outpost), `scoringObjectives[]` (one per `scoring` bullet, same order) and
  `rewards[]`.
  * `scoringObjectives[]`: `{key, text, side (both|attacker|defender), kind, maxPoints, instruction?, secret?}` plus by kind:
    `tiers` -> `options[{text, points}]` ascending, pick the best tier reached or none ("instead score" sentences are
    alternatives, not additions); `each` -> `item`, `pointsEach`, `maxCount?`, `maxPoints` (count x pointsEach, capped);
    `eachTiers` -> `item`, `count`, `options[]` (every item, e.g. each of 5 Objective Markers, scores its own best tier).
    `secret` marks the Fog of War objectives that are kept hidden until the end; `instruction` is the note-down sentence of
    those. A company's Victory Points are the sum of its objectives (`side` restricts an objective to that role); the most
    points wins, equal is a draw. Standard keys recur: `enemyLeader`, `enemyBroken`, `noCommandSupport`, `commandSupport`,
    `noCompanySupport`.
  * `rewards[]`: `{effect, name, text, who, condition}`; `effect` is the key of `campaign.aftermathEffects[]` (the typed
    grant), `text` the rulebook wording. `condition.kind`: `victory` (derived: the winner), `leaderNotRemoved` (derived: the
    Leader is neither casualty nor fled), `companyFlag` (players tick `key`/`label`: all 6 Objective Markers, all Cargo) or
    `modelFlag` (players pick the model: the Relic bearer). No scenario rewards Gold, free progression rolls or an XP bonus to
    all models; `campaign.matchInput.gold` says so.
  * `campaign.matchInput`: what the players enter per match. `perCompany[]` (`role`, `againstTheOdds`, `victoryPoints`,
    `scenarioObjectives`, `rewardFlags`, `fledBroken`), `perModel[]` (`outcome` survived|casualty|fled feeding the injury
    rolls, `participated`, then one field per `campaign.experience.gains` key: a `flag`, or a `count` when repeatable; `mvp` at
    most once per company, inferred), `result` (decided by Victory Points, tie = draw) and `aftermath.order`
    (injuries, experience, progression, influence; the rewards follow their `condition`). Influence follows
    `campaign.influence.gains`.
- Tooltips: `data.gearAnnotations.items[].definition` is the text to show for an item (the source text from vanilla gear or
  BC equipment, plus the rulebook text of its class when the source has none or only a profile: spear, pike, shield,
  light shield, lance, war spear, two-handed, throwing, bow, crossbow, armour); `abilities[]` are the same rules as data
  (`support{depth}`, `chargeWoundBonus`, `defenceBonus`, `shielding`, `duelPenalty`, `throwOnCharge`, `limit`...; the keys are a closed list, `ABILITY_KEYS` in `tools/bc/gear.py` and the schema enum). An item's own rule (`ITEM_RULES`) replaces its class rule: Pavise and Iron Shield `defenceBonus` 3 (Pavise `notWhileEngaged`), Wood Elf Spear only `shielding`, Small Shield `defenceBonus` 1 plus `shielding` (the BC text gives it the +1 the vanilla Light Shield lacks; Battle Companies wins), Easterling Halberd abilities tagged `mode` (`pike` / `two_handed`); `with` lists name gear classes (a pike's Duel penalty applies with a shield or any ranged class);
  Heroic action spellings: vanilla writes "Heroic Channeling"/"Channelling" and "Heroic Defence"/"Defense"; the glossary terms (category `heroic actions`) carry the other spelling as an alias, and the app (`RuleSet.HeroicActionName`) maps any spelling to the glossary name, so each action is offered once and a hero owning one spelling is not offered the other.
  `statChanges` for armour; a mount's `profile` names its stat block (vanilla `data.gear[]`, type mount).
- `data.creatures.rules` and `data.wanderers.rules`: the Creatures / Wanderers rules as flags (roster slot, limits,
  injuries, experience...; `inferred` names the keys that are a reading, not a sentence of the rulebook). Per creature:
  `companyFeature` (Baggage Pony `carryGear`, with `swap`), `handlersGlove`, `aura` (Falcon).
- `data.gearAnnotations`: one entry per piece of gear a Battle Company model can carry (wargear and options of the
  vanilla units the companies field, what bundles include, the BC wargear and equipment tables, unique wargear,
  creature/wanderer wargear, the company lists). Fields: `name`, `classes`, `grants`, `ranged`, `aliases`, `evidence`,
  `status`.
  - **Stat modifiers from gear.** The numbers come from two places that already exist: the vanilla `gear[].modifiesStats` (armour +1/+2/+3
    Defence, Helmingas, Battlin' Brandybuck... `{stat, modifier}`; the vanilla Shield also has `cancelledBy` bow / Two Hand Weapon with
    `uncancelledByKeyword` Expert Rider) and the annotation's `abilities[]` `defenceBonus {value, ...}`. The BC file adds, on every
    `defenceBonus` of a shield (Shield, Iron Shield, Pavise): `cancelledByClasses` (bow, crossbow, two_handed), `cancelledByItems` (Pike),
    `cancelledByTwoHandedUse` (a hand-and-a-half weapon used two-handed in that Combat; decided per fight offline, so the app only shows it)
    and `liftedByRules[]` `{rule, cancels, whileMounted?}`: Expert Rider lifts the bow cancellation while mounted (not the crossbow's),
    Phalanx (Easterling Warrior) lifts the pike's (REVIEW: the rulebook says a Phalanx model uses pike and shield "without penalty", read as
    also keeping the Defence bonus). The pike's `duelPenaltyWith` carries `unlessRules: ["Phalanx"]`. Algorithm for the app: start from the
    profile Defence (a profile that lists a shield already includes the +1 for it, rulebook), add each carried item's bonus, drop a shield bonus
    for each cancelling item the unit carries unless a rule it has lifts that cancellation. The validator checks the rule names.
  - **Conditional stat bonuses (user decision R8, 2026-10-22).** Two kinds of condition. SITUATIONAL (depends on what happens in the game:
    Engaged, Supported, the kind of attack): the increase is never added to a unit's characteristics, the item text carries it and the player
    applies it when it holds. LOADOUT (a bow, crossbow, two-handed weapon or pike carried, being mounted, Expert Rider / Phalanx): computed by
    the app as before (`cancelledByClasses`, `cancelledByItems`, `liftedByRules`). Stat-raising abilities (`defenceBonus`, `attacksBonus`) carry
    `conditional` (bool, true = situational) and, when true, `condition` (player-facing text). Today: Pavise (+3, not while Engaged), Small Shield
    (+1 only against Strikes) and the Elven Glaive (`attacksBonus`: Engaged, not Supported) are conditional; Shield and Iron Shield are not
    (loadout cancellations only; `cancelledByTwoHandedUse`, a hand-and-a-half weapon used two-handed, is decided per fight and only noted).
    `statChanges` (armour) carry `conditional: false`; unit upgrades with a vanilla `modifiesStats` (Helmingas, Battlin' Brandybuck) are
    unconditional. Roll modifiers (`woundBonus`, `duelPenalty`, `chargeWoundBonus`, `doubleWoundOnNatural6`...) are not stat changes and are
    shown as item text only. The validator enforces the flags.
  - `classes` say what the item IS, from the closed set `mount, melee, two_handed, spear, lance, shield, armour, bow,
    crossbow, throwing, missile, banner, instrument, consumable, other`; the first is the main class (icon lead,
    sorting). `lance` is the cavalry spear (own icon; a War spear counts as `["spear","lance"]`); `missile` is a ranged
    weapon that belongs to no limit (the Blowpipe); `consumable` is used up once (Healing Herbs, Scrolls, Brew, Cram).
    Hand-and-a-half weapons are `["melee","two_handed"]`. `other` is never combined with another class. Every spear is a
    melee weapon, so `melee` is never listed with `spear`; an item "used either as a spear or a two-handed weapon" is
    `["spear","two_handed"]`; a Wood Elf Spear (can use Shielding without a shield) is `["spear","shield"]`. Named unit upgrades (Helmingas, Lorien Guard...) and the utility items of the BC
    equipment table are `other`.
  - `grants` are special rules the bearer gets: a vanilla keyword, a BC rule or an instance of a parameterised rule
    (`Terror (Cavalry)`). Only recorded when the text says the bearer gets the rule ("has/gains the X special rule(s)")
    or the data says so (a mount's `specialRules`, `grantsSpecialRules` of a BC equipment item). A text that merely
    mentions a rule is not a grant; unclear cases are left out and listed in `tools/gear_review.md`.
  - `isA` (derived from the BC text "This is a X"): the item is X plus extras, like the Company Drum = War drum, Company Horn = War Horn, Elven
    Glaive = Elven spear + its attacks rule, **Small Shield = Light Shield + Defence against Strikes**. The Small Shield is Equipment (small slot,
    1 Influence), not wargear; its `defenceBonus` ability is `{value 1, against "strikes", notWithClasses ["shield"]}` (only against Strikes, so
    the Defence shown on the card stays unchanged; not with another kind of shield) next to the Light Shield's `shielding`. BACKEND: a Defence
    bonus with `against` is not added to the profile Defence; show it as a note on the card (pib.2).
  - `requiresMount: true` marks gear only a mounted model can use: the Lance, the Elven lance and Barding (`REQUIRES_MOUNT` in `tools/bc/gear.py`).
    A live match strikes them through when the mount is lost (`core.Equipment.RequiresMount`, copied by `equipmentFor`). The War spear is a spear
    on foot and does not require a mount; its lance mode is already `cavalryOnly` in its `chargeWoundBonus` ability. Rules a mount brings are the
    items' `grants` with `grantedTo: "mount"` (the Pony's Timid; `UnitRule.Mount`); the Cavalry keyword is matched by the rule name `Cavalry`.
    The validator: every pure `lance` item has the flag, a mount or a spear never does.
  - `weaponModifier: true` marks something that is not an item but a modifier bought for one weapon (Envenom Weapon
    (X)): the app shows a copy of the chosen weapon with the rule. It carries `grantsRuleForChosenWeapon` (see **Poison** below).
  - `ranged` (`range`, `strength`) is required on `bow`, `crossbow`, `throwing` and `missile`, and allowed only there.
    The Blowpipe records its rule as a grant (`Poisoned Attacks (Blowpipe)`): it is the only item that carries poison itself.
  - An item whose text says "This is a(n) X" inherits X's grants (the Company Horn gets the War horn's Dominant (2)).
  - `grantedTo: "mount"` marks the rules a mount brings (a Pony's Timid): they belong to the mount, not the rider.
    Only on items whose main class is `mount`.
  - The hero-only unit upgrades (Hearthguard, Westfold Redshield, King's Guard, Guardian of the King, Venom-back) stay
    annotated on purpose (`KEEP_ANNOTATED` in `tools/bc/gear.py`): a campaign organiser may hand them out.
  - Case, punctuation, plural `s` and the word "made" never make a different item (Elven-made Glaive = Elven Glaive). `aliases` are other spellings of the same
    item (case, punctuation and plural `s` never make a different item; the key is `item_key` in `tools/bc/gear.py`).
  - **Identity.** An item is identified by its name (`item_key`; the Go twin is `ItemKey`) and nothing else: the same stats, class or
    price never make two items one. A row of `data.wargear.items[]` (`names`) is a **price group**: every name in it is a distinct
    item with its own annotation, text and availability (Pike, Spear, Lance and Warspear share a price, not an identity; Elf Bow,
    Esgaroth Bow, Longbow, Dwarf Longbow and Crossbow likewise). `aliases` are spellings of one item only. Optional links between items:
    `relatedTo[{name, note}]` (related but distinct, symmetric: Elf Bow / Longbow have the same profile, a Pike is not a Spear),
    `base` + `modifiers` (below). `data.wargearReadAs` lists the company-list spellings read as the profile's specific item (curator's
    readings, e.g. Rivendell Warrior "spear" -> Elven spear); the lists carry the resolved name. "Two-handed axe" (Dwarf Warrior, Wild Man of
    Dunland) is an alias of the Two-handed weapon (confirmed by the user); the capitalised vanilla spelling "Two-handed Axe" is listed as an alias too (`VANILLA_SPELLING_VARIANTS` in `tools/bc/gear.py`) so every canonical spelling agrees. The Easterling Halberd is in the simple hybrid-weapons row at 1 Influence (confirmed). The Elven Glaive and Wood Elf Spear are Spear + `elven` that also carry their own rule (`ITEM_RULES`): the glaive's `attacksBonus` (+1 Attacks for the Duel Roll or for Strikes when Engaged and not Supported; the wording is the vanilla gear text, the BC document only prices it), the Wood Elf Spear's `shielding`.
  - **Item modifiers.** An item is a base item plus modifiers: "Elven spear" = Spear + `elven`. Poison is not a modifier (see **Poison**).
    `data.gearAnnotations.modifiers[]` defines each modifier once (`key`, `name`, `glossary` term, `identity`, `appliesTo`, the rulebook `text`
    and a shorter `tooltip`, the effect as `abilities`, the `price` rule, `source`); an item carries `base` (the generic item) and `modifiers`
    (keys). The tooltip (`definition`) of a modified item ends with each modifier's text, so the chips, the glossary extras and the armoury
    explain it with no extra code. The modifiers of the sources: `elven` (rulebook "Elven weapon": wins a Drawn Combat's roll-off on 3+ for a
    Good model, on 1-4 for an Evil one, cancelled when both sides have one, melee weapons only, any weapon with "Elf" in its name is Elven;
    BC wargear table: Elven-made weaponry uses the generic equivalent's values, restricted to Elf models), `masterForged` (rulebook: no -1
    Duel Roll for using it as a two-handed weapon; no BC item carries it today). "Masterwork" and "Dwarven-made" appear in neither document; "Fire-based attacks" has no inherent
    effect, so none of them is a modifier. `identity` says what the modifier does to identity: `distinct` (the Elven spear is its own item: a
    Spear option does not take it, so a Spear is not bought by an Elf profile and the other way round) or `variant` (fills the profile's base item; no item uses it today). `pricedAs` is derived from base + modifiers for the app
    (sold at the base's price and rating) for an Elven item whose base is in the wargear table and which is not unique wargear
    (the Elven-made Glaive and the Wood Elf Spear are Spear + `elven` too but keep the price of their company's page; the Galadhrim Knight's
    Elven-made hand-and-a-half sword is never sold, `NOT_SOLD` in `tools/bc/gear.py`). `relatedTo` stays for items that are related
    but have no base (Elf Bow / Longbow). The validator checks keys, bases, the glossary terms and that the derived fields agree.
    A warrior may buy an item only when it is exactly one of the items its profile options name (`OptionItems` / `GearMatchesOption`,
    `internal/core/gearidentity.go`), a hero any item of its company's profiles. Chips and tooltips look the item up by its own name:
    `GlossaryExtras` adds one term per item, weak against the rulebook's own glossary entry (Spear, Pike, Bow, Crossbow, Light Shield).
  - **Poison** (owner decision 2026-10-27). Poison is the special rule `Poisoned Attacks (<weapon>)` of a UNIT; wargear stays plain. It comes
    from three places, and no wargear item other than the Blowpipe carries it:
    1. *A unit rule.* Haradrim Warrior, Haradrim Raider, Haradrim Chieftain, Watcher of Kârna... have `Poisoned Attacks (Bow)` on their vanilla
       profile. A Harad list row written "bow with poisoned arrows" / "bow with poison arrows" is read as the plain `Bow`
       (`data.wargearReadAs`, `READ_AS` in `tools/bc/gear.py`) and needs no grant: the unit already has the rule. The validator checks that
       every such reading names a unit whose profile has `Poisoned Attacks (<item>)`. A promoted unit therefore keeps the plain Bow but is
       poisonous only if the NEW profile has the rule (the Haradrim Raider has, a unit without it does not).
    2. *The weapon itself.* The Blowpipe grants `Poisoned Attacks (Blowpipe)` (annotation `grants`, rulebook: "benefits from the Poisoned
       Weapons special rule"); most Blowpipe profiles list the rule too, and a unit that has both shows it once. The grant leaves with the
       Blowpipe: a promotion to a profile that does not list the Blowpipe drops the item and the rule.
    3. *Envenom Weapon (X)* (equipment, `restrictions: ["evil"]`, Small, 1 Influence). The annotation says what it gives:
       `weaponModifier: true`, `grants: ["Poisoned Attacks (X)"]` and
       `grantsRuleForChosenWeapon: {"rule": "Poisoned Attacks", "weaponClasses": ["melee","two_handed","spear","lance","bow","crossbow","throwing","missile"]}`.
       The player chooses ONE item of the unit whose class is in `weaponClasses` (a weapon the unit has, not already named by a
       `Poisoned Attacks (<weapon>)` rule it holds); the UNIT gains the rule `Poisoned Attacks (<that weapon's name>)` (the parameter of
       `data.parameterisedRules` "Poisoned Attacks"). The weapon stays the plain item. In a company the choice is stored in the name of the
       bought item: `Envenom Weapon (X)` has no weapon yet, `Envenom Weapon (Bow)` is chosen (`BCBook.ChosenWeapon` resolves it to the
       annotation and the armoury offer; the card's `Equipment.Modifies` is the weapon, `Equipment.Choices` the weapons it could be for,
       `CompanyUnit.PoisonableWeapons` those of the unit). The classes come from `weaponClasses` (Go: `BCBook.CanBePoisoned`, also used by the
       level-up's Poisoned Attacks); in a match the kit assignment `Detail` names the weapon in free text. The minor special rule "Poisoned Attacks (X)" of the paths asks the same
       question (`Progress.Minor`, `Poisoned Attacks|weapon:<name>`) and gives the same unit rule.
       The choice is FINAL (owner decision 2026-10-27) and belongs to the weapon's NAME: the unit keeps the rule when the weapon is dropped
       or lost in a promotion (the item keeps its chosen weapon, `Equipment.Choices` is empty), and buying the weapon again poisons it again
       (the rule shows once); dropping the Envenom Weapon item itself removes the rule. The rule is only ever created with a weapon chosen
       from the unit's own `PoisonableWeapons`: `BuyEquipmentFor` refuses a purchase without a weapon, with a weapon the unit lacks, one
       already poisoned or outside `weaponClasses` (the refusal names the choices), and `ArmouryService.ChooseModifierWeapon` only serves a
       legacy `Envenom Weapon (X)` that has no weapon yet (it gives no rule until one is chosen) and refuses a second choice. Promotion:
       Envenom Weapon is equipment and stays as it is. A unit with the same rule from its profile, the Blowpipe and Envenom Weapon shows it
       once; a weapon a rule already names is not offered.
  - Bow-type weapons (`bow` class: Bow, Longbow, Elf Bow...) carry `moveAndShoot` (may shoot after moving up to half the Move Value,
    Shoot Value worsened by 1); a `crossbow` (24" S4, vanilla and the rulebook's Missile Weapon chart) carries `noFireAfterMove`. A
    Pike carries `twoHanded`, Support `depth` 2 (it supports like a spear and through a second pike-bearer) and the shield/missile
    Duel penalty; a Spear has depth 1.
  - Bundles ("Shield and Spear") and "Exchange X for Y" options are not items: their parts are. `status` is `derived`
    (a pattern in the text or the vanilla type) or `curated` (a decision in `DECISIONS`, `tools/bc/gear.py`).
  - The validator checks the class enum and the combinations (`other` alone, no `spear`+`melee`), that every grant
    resolves, `ranged`/`grantedTo` placement, and that every reachable item has exactly one annotation. The coverage
    walk is written separately from the generator's `collect()`; both follow a unit's `includes` (the Vault Warden Team brings Iron Shield and Foe Spear). It also checks the ability keys and the class names inside ability lists.
- `data.unitFillIns` (`UNIT_FILL_INS` in `tools/bc/vanilla.py`): race, faction, unitType and baseSize for vanilla units that leave them empty
  (today the Hunter Orc Warrior, taken from its sibling Warg Rider, and the Vault Warden Team bundle, taken from its two members: Dwarf, Khazad-dum, Warrior + Infantry; both REVIEW). The app applies a value only where vanilla is empty; the
  validator checks the vanilla field really is empty. Other vanilla units with gaps (crews, siege engines, bundles) are not used by a company.
- **Rule key and label.** A Battle Companies rule's `name` is its key (identity: `specialRules`, grants, links); the optional `label` is the display
  name when it differs, `relatedTo` lists the rules it is a variant of (they share the label; the glossary links them). Vanilla rules and BC
  rules without `label`: the name is both. The app shows the label, never the key. Today: `Keen Sight (Falcon)` (label `Keen Sight`, related to
  vanilla Keen Sight, used by the Crebain). A BC rule defined twice in the file is keyed `Name (source)` with the printed name as label. The
  validator checks label/relatedTo.
- **Restrictions by unit identity.** A unique wargear item or hero upgrade whose "X Only" line names units carries `restrictedToUnits` (exact
  vanilla unit names the company uses): War Camel -> `Mahûd Warrior` (the Half Troll shares the Mahud keyword and may not buy it); Smoke
  Bombs -> the Corsair units of City of Umbar. `restrictedToKind {race, heroesOnly}` is for a kind of model (The Shire's Pony: Hobbit
  heroes). `restriction` keeps the printed word. Feral Charge: `restrictedToUnits ["Isengard Warg"]`, (Isengard Warg heroes only; the
  company recruits the Warg without it and the Orc Isengard Warg Rider cannot take it). The other "(X Only)" hero upgrades name a race or faction keyword (Dol Amroth,
  Lossarnach, Lamedon, Blackroot Vale, Hobbit, Spiders, Orc) and stay `restriction`; equipment "(Hero Only)" / "(Evil Only)" are roles, not units.
- **Choice of Options and the Armoury (user, 2026-10-21).** An option can be chosen (Choice of Options, recruit) only if it maps to an Armoury item.
  Not choosable: options in `data.optionSales` with status `notSold` or `granted` (banner / war horn / war drum options, Twin Blades, Whip,
  Armour, the Wood Elf spear) and `data.reservedOptions` (army-list upgrades: Lorien Guard, Hearthguard, Helmingas...). Status `readAs` keeps an
  option choosable as `armouryItem` (Cave Troll: Hand-and-a-half hammer -> Hand-and-a-half Weapon; the hammer is flavour). Status `ownItem` (`item`) keeps an option choosable under its own name when it is a weapon of its own that the Armoury does not sell: the Cave Troll's Troll Spear (a spear with the additional rule `doubleWoundOnNatural6`, annotation class `spear`). Exchange options
  ("Exchange lance for banner") are judged by the item they bring. `data.profileWargearNotSold` lists the wargear of the profiles no Armoury sells (profile
  armour and hand weapons, natural weapons, Elven cloak...): a model always starts with it, it is added on recruit, never an option and never
  buyable. The full tables are generated into `tools/armoury_gaps.md` (`bc/armoury_gaps.py`); the validator checks the list. Backend rule:
  choosable = not in `Unsold`, not `Reserved`, or `readAs` (then treated as `armouryItem`); recruit adds the profile wargear regardless.
- `data.optionSales` (`option_sales` in `tools/bc/vanilla.py`): the vanilla profile options no Armoury sells, so the audit does not flag
  them as `option-unbuyable`. Items `{unit, option, status, reason, source}`; status `notSold` (banner / war horn / war drum options,
  generated: the BC equipment sells the Company Standard / Horn / Drum instead and a Choice of Options excludes them; a Unique Wargear
  item of one company, the Wood Elf spear) or `granted` (comes with the charts or starting company: Twin Blades, Whip, a Cave Troll's
  weapons, the Ranger of the North's armour). A company whose armoury does sell the item is unaffected. The validator recomputes the
  banner list and checks every entry names a real vanilla option (`BCBook.Unsold`).
- `data.reservedOptions` (generated from vanilla by `reserved_options` in `tools/bc/vanilla.py`): the options of vanilla
  units whose `availableIn` names only heroes (King's Guard of Gil-galad's list, Lorien Guard, Hearthguard, Noldorian
  Exile, Venom-back, Westfold Redshield, Helmingas, Guardian of the King, Tookish Hunter, Battlin' Brandybuck). Decision:
  the Battle Companies document does not lift army-list restrictions ("Choice of Options" says "refer to the model's
  profile", and a Battle Company has no named hero's list), so a Choice of Options never offers them
  (`BCBook.Reserved`, used by `choiceFor`). A company still selects one explicitly through `unitAliases`
  (`selectedOptions`), which is unaffected, and the gear annotations stay (see `KEEP_ANNOTATED`). Options whose
  `availableIn` names armies or scenarios are not listed and stay offered. The validator rebuilds the list independently.
- **Effective stats (app).** `applyGearStats` (`internal/core/gearstats.go`) adds to a unit's characteristics what its gear changes
  relative to the gear its vanilla profile lists (the profile already counts that gear): armour by stage (best one only, `statChanges`),
  the best active shield (`defenceBonus`: cancelled by a bow/crossbow/two-handed weapon/pike unless a rule lifts it, Expert Rider while mounted,
  Phalanx; a hand-and-a-half weapon is only noted, since two-handed use is decided per fight), and the plain `modifiesStats` of unit
  upgrades (Helmingas, Battlin' Brandybuck Strength +1). Results: `Attribute.Gear` (already in `Value`) and `CompanyUnit.StatMods`
  `{stat, delta, from, note}`; hero rating counts the profile without the gear's part. Disabled (injured) gear gives nothing.
  **Conditional bonuses are item text, never added to the stats** (register R8): a bonus that holds only in a game situation the app cannot
  see is a `StatMod` with `Delta` 0 and a `Note`: the Pavise's +3 (`notWhileEngaged`), the Small Shield's +1 (`against` strikes), a
  `statChanges[].conditional` change and a vanilla `modifiesStats` entry with any key besides `stat` and `modifier`. A shield's loss through
  the unit's own gear (bow, two-handed, pike) is decided from the unit's items and is not a note: the bonus counts by default. The data's `conditional` flag on a `defenceBonus` or `attacksBonus` ability (with its `condition` text) means situational (Pavise, Small Shield,
  Elven Glaive: `GearAnnotation.Situational`); the loadout conditions (bow, two-handed, pike, Expert Rider, Phalanx) stay computed. Data `optionSales`
  status `ownItem` (Cave Troll's Troll Spear) is a choosable option under its own item name, `readAs` one read as another armoury item.
- `data.unitFillIns` is applied on the vanilla book at load (`BCBook.ApplyUnitFillIns`), only where the vanilla field is empty.
- `data.mountFillIns` (`MOUNT_FILL_INS` in `tools/bc/vanilla.py`; `gear`, `race`, `faction`, `unitType`) does the same for the five vanilla mounts without keywords (Brego, War boar, Elk, Cart, Moria Goblin Drum; factions are `See Rider`; all REVIEW); applied by `ApplyUnitFillIns` on the gear. The spelling variants of vanilla keywords (Khazad-Dum/Khazad-dum, ...) cannot be fixed in the vanilla file; core merges them with `keywordSpellings` (`internal/core/keywords.go`).
- Glossary terms `<Race> keyword` / `<Faction> keyword` (category `keywords`, `RACE_KEYWORDS`/`FACTION_KEYWORDS` in `tools/glossary_terms.py`) are brief authored entries (`summarized`) behind the keyword chips; the " keyword" suffix keeps the glossary linker from linking every "man" or "shire" in the rules texts. `Monster` and `Chariot` chips use the existing terms `Monsters` and `Chariots` (aliases).
- A rule with a `label` is shown by it: `BCBook.RuleLabel`, `UnitRule.Label` / `Shown()` (templates must print `Shown`, never `Name`);
  the glossary merges such variants into the term of their label. Unique wargear `restrictedToUnits` / `restrictedToKind` and hero
  upgrade `restrictedToUnits` decide who may take them by unit identity (`GearOffer.OnlyUnits/OnlyKind`, `HeroUpgrade.Units`);
  `restriction` is display text only for those. A unit On Its Own Path is never an officer (`CompanyProfile.CanLead`).
- **Whole bundle.** A start, chart or promotion row that names one piece of a multi-item profile option (vanilla "Shield and Spear") gives the unit the whole option and prices it at the option's points (`completeBundle` in `internal/core/bundles.go`, applied in `buildUnit`; an exact single-item option wins; unsold and exchange options are no bundles). Iron Hills Warrior (Spear) therefore carries Shield and Spear for 12. A piece dropped and bought back costs its own armoury price. The audit kind `partial-bundle` lists the rows (today Iron Hills Warrior [Spear] and Fiefdoms' Knight of Dol Amroth [Armoured horse], which also gets the lance).
- **Price rule.** A unit's points are the vanilla base profile cost plus the options it bought, nothing else: a warrior's wargear
  is priced by the options of its profile (`OptionsCover`: the exact option; or the options whole in the wargear added up, since a
  warrior may buy every option at once; or the cheapest option that has the leftover item as a part, so a Knight of Dol Amroth with an
  armoured horse is the "Armoured horse and lance" option, 11 + 9 = 20); only wargear no option has is charged its armoury rating. The
  audit flag `price-mismatch` checks it for every start, chart and promotion row.
- **Limits** (top-level `limits`): every limit (bows, throwing weapons, cavalry, a company's keyword limit such as Helm's Deep Elves)
  rounds up (`LimitMax`) and counts the model being recruited in the numerator and the denominator (`CompanyLimit.Next`,
  `recruitEnv.limitBlock`, `companyBlockFor`); the `roundUp` flag of the file is no longer read. each has a stable `id` (`bow`, `throwing`, `cavalry`), the keys of a company's
  `limitOverrides`. A company's `limitExemptions` (`unit` or `item`, `limits` ids, `text`) takes models out of a limit:
  a model counts as Cavalry for the `cavalry` limit when its profile says so or it carries a mount (a bought Horse); buying a mount is refused when the limit is full. An exempt model is left out of both the count and the base the maximum is worked out on (2 Warriors of Rohan + 10
  Riders of Rohan: base 2, so Bow and Cavalry maximum ceil(2 / 3) = 1). Today: Rohan (Rider of Rohan: bow, cavalry),
  Paths of the Druadan (Rider of Rohan: bow), Grand Army of the South (Khandish Horseman: bow), Sharkey's Rogues
  (Whip: throwing). The table is `LIMIT_EXEMPTIONS` in `tools/bc/core.py`; the build asserts each `text` is on the
  company's page.

### Tabletop Simulator model names (`data.ttsModels`)

Fixed mappings from Battle Companies to the model names of the Tabletop Simulator MESBG FTC mod, read by the TTS export
(`BCBook.TTSModels()`, `internal/core/ttsmodels.go`; spec in [`tabletop-simulator.md`](tabletop-simulator.md)). Generated by
`tools/bc/tts.py` from curated tables with a `reason` each; the mod's catalogue of model keys is never in the repository.

- `standIns[]` (`kind` wanderer or creature, `name`, `model`, `alternatives`): the seven approved stand-ins (Wandering Swordsman = Captain of Dale,
  Marksman = Ruffian: Bow, Scavenger = Goblin Mercenary Captain, Sage = Malbeth the Seer, Baggage Pony = Bill the Pony, Falcon = Crebain, Hunting Dog
  = Fang). A `model` is a vanilla name or `Name: options`.
- `mounts[]` (`gear`, `model`, `attacks`): the mounts a company can buy whose vanilla gear has Attacks above 0 (derived from the data: Warg, Fell Warg);
  `model` is the figure to spawn (the Warg is shown as the Fell Warg).
- `riders[]` (`unit`, `footModel`, `states`): the cavalry units the companies use (18 vanilla units; the alias Mirkwood Cavalry resolves to Mirkwood Elf
  Knight). `states` is true when every model key of the unit in the mod carries the state that dismounts the figure (Rider of Rohan, Son of Eorl,
  Mirkwood Elf Knight, Hunter Orc Warg Rider); six units with states on only some keys are false for now (owner question pending, one value to flip).
  `footModel` is the rider on foot; absent when the mod has none (Son of Eorl); the Iron Hills Goat Rider's plain Dwarf Warrior is provisional (owner choice, the bag has no Iron Hills foot model).
- `names[]` (`unit`, `from`, `to`): option spellings that differ between the vanilla file and the mod (Isengard Orc Warrior: Orc bow -> Bow).

The build stops when a stand-in no longer matches the wanderers and creatures, or when the cavalry units the companies use differ from `RIDERS` in
`tools/bc/tts.py`. The validator (`tts_errors`) checks every name against the vanilla file, that the entries are few (a catalogue is refused), that every
wanderer, creature, fighting mount and used cavalry unit is mapped and, only when the gitignored `sources/tts/models.json` exists (a list of model keys,
`--tts-models` to point elsewhere), that every model is a key of it and that every used unit has a model there.

## Sources and copyright

The Battle Companies document and the MESBG rules manual are copyrighted. They (and any text converted from them) live
only in the gitignored `sources/` folder. The JSON files built from them may be committed.

## Reference tables (`reference` in the glossary file)

Besides `terms`, `definitions/glossary/mesbg_glossary.json` carries structured rulebook tables the terms only describe
(`tools/glossary_reference.py`, hand-transcribed from the rulebook's chart; the validator and `tools/test_to_wound.py` check them).
The home is the glossary because the chart is a base-game rule (not Battle Companies); the file hash covers it like the terms.

`reference.toWound` (the To Wound Chart, rulebook page 44; the glossary term `To Wound Chart` is its prose):

```json
{"title": "To Wound Chart", "term": "To Wound Chart", "rulebookPage": 44,
 "strengths": [1,2,...,10], "defences": [1,2,...,10],
 "cells": [["4+","5+","5+","6+","6+","6+/4+","6+/5+","6+/6+","-","-"], ...],
 "notation": {"N+": "...", "6+/N+": "...", "-": "..."}, "notes": ["..."]}
```

`cells[s-1][d-1]` is the roll for Strength `s` against Defence `d` (10 rows by 10 columns; Strength and Defence stay within 1..10).
Values: `3+`, `4+`, `5+`, `6+` (one dice at that score or more), `6+/4+`, `6+/5+`, `6+/6+` (a 6+ on one dice, then a further dice
scoring 4+/5+/6+; two individual rolls, either can be re-rolled by a rule) and `-` (cannot wound). The validator requires the
complete grid, only those values, and monotonicity (never easier as Defence rises, never harder as Strength rises; order
3+ < 4+ < 5+ < 6+ < 6+/4+ < 6+/5+ < 6+/6+ < -). The rulebook states no rule for natural 1s or 6s on this chart, so none is encoded.

## The concept graph file

`definitions/concepts/concept_graph.json` (schema `concept_graph.schema.json`) is built by `tools/build_concept_graph.py` (library
`tools/concept_graph.py`) from the shipped triple and holds only what cannot be recomputed from the text: `builtFrom` (the three ruleset
hashes, as `core.RulesetHash` computes them), `nodes` (`id` = `<kind>:<glossary term id>`, `kind`, `name`, `aliases`, `file` = the file the text
comes from, `textHash` = `core.ConceptTextHash` of the term's text), `edges` (`grants`, `cancels`, `lifts`, `requires`, with `via`) and
`ambiguous` (names or aliases claimed by more than one node). `mentions` and `see` are not stored. The nodes are the terms of the ruleset's
glossary exactly as the app builds it (`GlossaryExtras` + `BuildGlossaryWith`): `tools/concept_graph.py` is a port of those two functions and
`internal/core/concept_graph_file_test.go` checks every node (id, kind, name, aliases, text hash), the `builtFrom` hashes and that the file
is taken whole by the loader. Output is sorted: regenerating gives no diff.

Edges derived from the data (`derive_edges`): `grants` from `gearAnnotations[].grants`, equipment `grantsSpecialRules`, a path's starting heroic
action and chart rules; `cancels` from the shield `defenceBonus` abilities (every item of the cancelling classes, and `cancelledByItems`,
points at the shield); `lifts` from `liftedByRules`; `requires` from `requiresMount` and equipment `restrictions` (cavalry, hero). Their
`via` is a sentence of the item's text or a short description of the field (`restriction: cavalry`). By hand: `tools/concepts_curated.json`
(names, a `via` that must be a verbatim quote of one of the two texts, checked by the build, and a `why`). Whatever has no node to point at
or would point from vanilla/glossary into Battle Companies is not in the graph and the build prints it (today Hearthguard -> Burly, because
Burly is a Battle Companies override of the vanilla rule, and Blowpipe -> Poisoned Attacks (Blowpipe), which vanilla does not define). Left out
by decision, without a report: standard weapons with a modifier or a kind that has no term of their own (Elven lance, Elven hand-and-a-half
weapon, Hand-and-a-half hammer...: no node is invented) and the `evil`/`unique` equipment restrictions (armoury only).
Command Support (a scenario special rule, `SCENARIO_KEYWORDS` in `tools/bc/core.py`) is a Battle Companies keyword, so the instruments' grants point at it.

`validate_definitions.py` checks the schema, that every edge endpoint is a node, every node resolves to a term of the files (same file,
name, kind and `textHash`) and no term lacks a node, the direction rule (no edge from a vanilla or glossary node into Battle Companies),
no duplicate or self edge, only the four stored types, a `via` on `cancels`/`lifts`/`requires`, `ambiguous` equal to the real list, and
that `builtFrom` is current (a stale graph fails: rerun the builder after rebuilding any of the three files). Tests:
`tools/test_concept_graph.py` (`python3 -m unittest discover -s tools -p 'test_*.py'`, part of `make test`). `make concept-graph` rebuilds and validates.

## The loader

The **loader** (`core.Rules`, `internal/core/rules.go`) merges the three files into in-memory maps the rule engine queries. The concept graph (bead 6lu.9) is specified in [`concept-graph.md`](concept-graph.md); its loader (`core.Concepts`, mentions and see computed per ruleset triple from the glossary, curated-file hook with per-edge reuse) and the generated file (`definitions/concepts/concept_graph.json`, read by `cmd/server` at start) are built, and so is the popup (`GET /campaigns/{id}/concepts/...`, [`frontend.md`](frontend.md) "Helper popup"). Design:

- **Maps, not database lookups.** The files are parsed once into immutable maps keyed by name (BC first, then vanilla,
  then glossary, following the rules above), read concurrently without locks. The concept graph is computed into
  adjacency maps on first use per triple (`core.Concepts`, its own bounded cache).
- **Shared by content.** A ruleset is identified by the content hash (SHA-256 of the compacted JSON, so whitespace does
  not matter) of its vanilla, Battle Companies and glossary documents. Campaigns with the same hashes share one set of
  maps: the cache is keyed by that triple and builds each set once, even under concurrent requests.
- **Uploaded rulesets** (a campaign may use its own files) are stored once per hash, content-addressed, in the database;
  a campaign only references the hashes. The default files shipped with the app go through the same path.

## Confirmed rules and defaults

Objects that describe a house-rule decision (`campaign.gold`, `campaign.injuries.treatment`, `data.creatures.rules`,
`data.wanderers.rules`, `data.magicalPowers.castingRule`, `campaign.progression.hero.resolution`,
`campaign.characteristicLimits.increases`) carry two lists: `confirmedByUser` (the user decided it) and
`defaultNotConfirmed` (a default chosen by the app, not confirmed by the user). Gold comes from a company's starting
company, scenario victory rewards and organiser grants, carries over and can be spent in the Armoury later
(`campaign.gold.sources`, `when`, `carryOver`). Dual weapons (Axe of Lossarnach, Dunlending War Axe, Easterling
Halberd) have `gearAnnotations.items[].modes` (chosen per fight; the two-handed mode is not usable with an Arm Wound).
Stat maximums written `(+1)` count increases from level-ups only (`characteristicLimits.increases`); the backend tracks
the number of increases per stat.

## Officer succession (Battle Companies rulebook, "Leaders & Sergeants" and "Death of a Hero")

The rules text is not data in the JSON; the rule engine applies it. What the rulebook says:

- **Shape.** "You may only ever have a maximum of one Leader and two Sergeants. The rest will be referred to as Heroes in the
  Making." The rulebook never demands exactly one and two at all times; it says "maximum".
- **Lost for good.** A Leader or Sergeant who dies, or is forced to retire ("as per the Dead result"), leaves the roster.
- **Leader lost:** "choose one of your Sergeants to be promoted to the Leader ... after the battle." (The rulebook names only
  Sergeants; the app also accepts another Hero when no Sergeant is left.)
- **Fewer than two Sergeants:** "another Hero in your company, who is not already a Leader or Sergeant, may become a
  Sergeant" ("may": optional).
- **Warrior as officer, only as a last resort:** "Should you not have another Hero model available to replace a Leader or
  Sergeant who has died, then the Warrior with the most Experience Points is instantly treated as having rolled Hero in the
  Making ... gaining a Might, Will, and Fate". Ties: highest Rating, then the controlling player. So a Warrior may become an
  officer only when no non-officer Hero is available; a Hero must be chosen first. This applies to a lost Leader and to a
  lost Sergeant alike.
- **Stepping down.** The rulebook says nothing about an officer ceasing to be one. Nothing returns a Hero to Warrior status,
  so a Hero promoted only to fill an officer place stays a Hero (a Hero in the Making) with its path. The Shire's Ranger of the
  North replacement ("may immediately take the role of Leader or Sergeant") is the one case that displaces an officer: the Ranger starts as Leader, may be moved to any officer place and is never dropped, and the old officers are reshuffled, one stopping being an officer and staying a Hero (see frontend.md).
- **Pool.** The text does not restrict the pool beyond "in your company"; the app uses the units still on the roster (the fallen
  and retired are gone). Whether an injured unit qualifies is not stated (the app counts it).

## Explicit company edge cases

- `campaign.recruitment.adjust` / `.lowerResult`: Influence raises the die only (1 per point, at most 3 per attempt across both
  charts, every point paid, lowering never paid); a lower result is a free choice of any lower result, allowed only when the
  rolled result cannot be recruited or a company rule says so. `companies[].effects[]` kinds `reinforcementReplacement`
  (alternative option on chart rows, chosen by the player, free), `rangerReplacement` (Shire: `unit`, `max`, `role[]`, `when`; swaps a successful reinforcement result for the unit once the company's one was slain, see [frontend.md](frontend.md); the backend reads it into `CompanyProfile.RangerReplacement`), `includedUnitPoints` + `modelCount` + `recruitFallback`
  (Durin's Folk Vault Warden Team: the members come from the vanilla `includes`, the file adds their points and rules),
  `unitTypes` + `compositeModel` (Warg Marauder: one Cavalry card of three riders and a mount), `heroPromotion` (Arnor:
  promotion AND hero, no promotion-only outcome). An officer is not a warrior that rolled a 6: the company's promotion applies only to the level-up roll. An officer chosen at company creation (`StartingUnits`), a Warrior taking a lost officer's place and a recruit that joins as a Hero (`recruitAsHero`) keep their profile (Arnor's Rangers of Arnor chosen as officers stay Rangers of Arnor, as heroes with their Might, Will and Fate) and gain the point of each (`startingHeroNoStatGain` lists units that already have them). Officers made by an earlier version that were promoted at creation keep their profile as it is. A start unit the file marks `heroInTheMaking` (Wanderers in the Wild) starts as a hero with that point and its path to choose, and is not one of the officers the player picks (`CompanyProfile.CanLeadStart`). The starting company adds up: after the officers are chosen, the company's points plus its starting gold are 105 (60 points of rating and gold plus 5 each for Might, Will and Fate of 3 heroes; BC "Creating Custom Companies"); `TestStartingCompanyPointsPlusGoldAre105InEveryCompany` checks every company and every allowed choice of officers, with the exceptions listed in `startingTotalExceptions`. A starting unit that is a hero by its profile (`startingHeroNoStatGain`: the Ranger of the North of The Shire and Helm's Deep) must be one of the officers (`ValidateStart`), or the company would add up to 120. Defenders of the North adds up to 106: the book's starting list is 61 points of warriors at the current MESBG points (bead for the curator). Wanderers in the Wild: all five starting models are heroes (3 officers and 2 Heroes in the Making, shown on the creation page as "Hero in the Making") and every recruit joins as a Hero in the Making.
- `data.paths[].kind`: `heroic` (the 11 selectable paths), `endOfPath` ("On Their Own Path": not selectable, assigned
  automatically when an On Their Own Path unit is recruited, special rule "At Path's End": no XP, no level-up) and
  `custom` ("Path of Their Own Making", homebrew from a community document, complete: `dice`, `maximums`, an 11-row `progression`, `startingStats` 0/0/1, `givesExperience`, `canLevelUp`, `implemented: true`, enabled by `campaign.options[key=customOnTheirOwnPath]`; with it the units On Their Own Path start as Warriors, earn experience on the warrior table and take this path automatically, with its 0/0/1 start, on a roll of 6: no path window, no forgone promotion). Every path has `dice` ("2d6": two pairs, choose one, as `campaign.progression.hero.resolution`). A `specialRule` option may carry its own `definition` (Lead by Example: the vanilla rule of that name belongs to Buhrdur).
  `onTheirOwnPath` effects name their `path`.
- `campaign.options[]` is the single list of settings. Each option is self-describing (`key`, `scope`: `rules` for rule variants and
  `app` for how the app behaves, `label`, `description`, `default`, `implemented`, `swaps`). Keys today: `heroesDropProfileGear`,
  `customOnTheirOwnPath` (rules), `shakeTheTable`, `manualRolls` (app). A reader that does not know an option key must ignore it
  and log a warning.
- Unit names are exact: every unit named by a chart, special unit, starting company, advancement, alias or effect (including
  `onTheirOwnPath.units`) is spelled like the vanilla unit or like a company `unitAliases[].label`; the rulebook wording is kept
  in `onTheirOwnPathRulebook` and the effect `text`. The validator fails otherwise.
- Rare units: `campaign.recruitment.rare` (at most N of that unit in the roster at the same time; dead or promoted units no longer
  count; a rare-exhausted result is recruited as a lower result) and `data.companies[].rareUnits[]` (unit, rare, where, roll).

## Chart rows that choose among named units

A reinforcement or special chart row with `result.type: "unitChoice"` lists `options[]`, each `{unit (exact name), label?,
selectedOptions?, rare}`; `label` + `selectedOptions` name a company `unitAliases` entry (a profile, e.g. Tookish Hunter =
Hobbit Archer with that option). The player picks ONE option whose own Rare limit is not used up (fewer than `rare` units of that
profile, unit + selectedOptions, are in the roster; injured units count, dead or promoted ones do not). When every option is used
up the result cannot be recruited and `campaign.recruitment.lowerResult` applies on the same chart. Wanderers in the Wild row 2 is
the only such row (its four Hobbit profiles). Each option is listed in `companies[].rareUnits[]` (`type: "unitChoice"`). Harad's
On Their Own Path covers the whole Watcher of Kârna unit (decision recorded in the effect text).

## Oaths

`data.companies[].oath = {text, vow}`: the oath the player swears when taking command of the company. `text` contains the placeholder
`{name}` exactly once, never at the start or end (the app shows it as an inline field for the company name); `vow` is the button
label (1-40 characters). `campaign.company.oath.{good,evil}` have the same shape and are used by alignment for companies of a custom
file that have no oath. All texts are original; the validator checks both rules.

## Audit report

`make audit` (`go run ./cmd/server audit [--out DIR]`, default `tmp/audit/`, gitignored, no database) loads the shipped rules and
writes one markdown file per Battle Company plus `index.md`. The data comes from the real code, not from the json: `BuildDetail`
(starting company, limits), the recruit charts, `BCBook.Armoury`, the armoury's `whyNot` for a warrior and for a hero of every
distinct unit profile (start list, chart rows, special units, promotion targets), `GearMatchesOption`, and the tooltip text a chip
shows (`equipmentFor`) with the glossary term its name leads to. The code is `internal/core/audit.go` and `audit_render.go`; the
output is stable (no timestamps, sorted), so two runs can be diffed. The index counts anomalies per kind and per company; kinds:
`offer-not-derivable` (wargear a unit can buy that no option of its resolved profile explains), `option-unbuyable`,
`price-missing`, `unit-unresolved`, `unit-no-wargear`, `duplicate-name` (first-wins or unreviewed vanilla duplicates the company
uses; reviewed ones are listed per company), `tooltip-name` (text never names its item), `tooltip-missing`, `option-unknown-item`,
`name-format` (differs from the item table only by case or spacing), `rule-unresolved`, `price-mismatch` (a unit's points are not the vanilla base plus its options) and `partial-bundle` (a row names one piece of a bundled option). A vanilla unit that writes a rule under another spelling than the rule table ("A fell light is in them") gets a BC rule of that spelling with the vanilla text and `overrides` (`SPELLING_VARIANT_RULES` in `tools/bc/core.py`). A bundle unit (`includes`, the Vault Warden Team) carries its members' wargear and is not `unit-no-wargear`. Flags are leads for a reviewer, not
verdicts: a tooltip may legitimately describe an item without naming it.
