"""Hand-authored data: the Battle Companies core rules that live in prose and small tables in the document.

Everything here was transcribed from "Battle Companies: 2nd Community Edition - Core Rules" (v1.1). The json says
WHAT the rules are; the rule engine decides HOW to apply them. Each rule has a stable `key` the engine can switch on.
"""

PROFILE_STATS = ["movement", "fight", "shoot", "strength", "defence", "attack", "wounds", "courage", "intelligence"]

LIMITS = [
    {"id": "bow", "type": "bow", "property": "all", "maximumPercentage": 33.33, "roundUp": True,
     "label": "bows (heroes included)"},
    {"id": "throwing", "type": "throwingWeapon", "property": "all", "maximumPercentage": 33.33, "roundUp": True,
     "label": "throwing weapons (heroes included)"},
    {"id": "cavalry", "type": "keyword", "value": "Cavalry", "property": "all", "maximumPercentage": 33.33, "roundUp": True,
     "label": "Cavalry"},
]

# Company Special Rules that take models out of a limit. An exempt model is left out of BOTH the count and the base the
# maximum is worked out on (2 Warriors + 10 exempt Riders: base 2). `text` must be a sentence of the company's page
# (the build asserts it). Exactly one of `unit` / `item` per entry; `limits` are ids of LIMITS.
LIMIT_EXEMPTIONS = {
    "Rohan": [{"unit": "Rider of Rohan", "limits": ["bow", "cavalry"],
               "text": "Riders of Rohan in this Battle Company do not count towards your Bow nor Cavalry Limit."}],
    "Paths of the Druadan": [{"unit": "Rider of Rohan", "limits": ["bow"],
                              "text": "Riders of Rohan in this Battle Company do not count towards your Bow Limit."}],
    "Grand Army of the South": [{"unit": "Khandish Horseman", "limits": ["bow"],
                                 "text": "Khandish Horsemen in this Battle Company do not count towards your Bow Limit."}],
    "Sharkey's Rogues": [{"item": "Whip", "limits": ["throwing"],
                          "text": "Whips do not count against the Throwing Weapon Wargear Limit."}],
}

VALIDATIONS = [
    {"key": "maxModels", "type": "maxModels", "value": 15,
     "message": "A Battle Company may have up to 15 members (12 for Elite companies; some companies allow more)."},
    {"key": "singleLeader", "type": "maxRole", "role": "leader", "value": 1,
     "message": "A Battle Company has a maximum of one Leader."},
    {"key": "maxSergeants", "type": "maxRole", "role": "sergeant", "value": 2,
     "message": "A Battle Company has a maximum of two Sergeants. Other Heroes are Heroes in the Making."},
    {"key": "oneCreaturePerHero", "type": "maxPerHero", "property": "creatures", "value": 1,
     "message": "Each Hero may only ever have a single Creature, attached to the Leader or a Sergeant (max. 3 per company)."},
    {"key": "oneWanderer", "type": "maxPerCompany", "property": "wanderers", "value": 1,
     "message": "A Battle Company may only ever have one Wanderer at a time."},
    {"key": "oneSorcerer", "type": "maxPerCompany", "property": "path", "name": "Path of the Sorcerer", "value": 1,
     "message": "Path of the Sorcerer (0 - 1): rare limit of 1 per company."},
    {"key": "equipmentSlots", "type": "equipmentSlots", "small": 1, "large": 1,
     "message": "A model may only carry one Large and one Small piece of Equipment (a Backpack adds three more Small pieces)."},
]

COMPANY = {
    "maxModels": 15,
    "eliteMaxModels": 12,
    "leaders": 1,
    "sergeants": 2,
    "startingHeroes": 3,
    "startingHeroStats": {"might": 1, "will": 1, "fate": 1},
    "startingPoints": 60,
    "heroicTier": "Minor Hero",
    "text": {
        "heroes": "When you start a Battle Company you select three of its members to become Heroes; they each get a single point of Might, Will and Fate and are treated as Hero models (all are Minor Heroes). One is the Leader (and may use Last Stand!), the other two are Sergeants. The rest are Heroes in the Making.",
        "heroicActions": "New Hero models can only use the universal Heroic Actions (Move, Shoot, Combat) and the special Heroic Action of their Path (if applicable) when they start.",
        "gold": "Some factions indicate a number of Gold in the Starting Company. Gold may be spent in the Equipment & Creatures section of the Armoury: treat the initial Rating of the Equipment, Creature or Wanderer as its Gold value and ignore the Influence cost. Alternatively, Gold can buy additional Wargear for starting models as long as Warriors stay legal to one of their profile's Options.",
        "customSelection": "At the discretion of the Campaign Organiser, a starting roster may be built from models anywhere in the company's charts and tables. It may not exceed 60 points, including wargear, equipment and creatures, and must adhere to any restrictions.",
        "choiceOfOptions": "A 'Choice of Options' result lets the model take any option on its profile, except any Option that features a Banner, Warhorn or War Drum.",
        "companyHeroUpgrade": "Any Hero may replace a result from their Path with their company's Company Hero Upgrade, if they do not already have it.",
        "companySpecialRules": "A Company Special Rule is a modifier that automatically applies to the models detailed on the company page, at no additional rating.",
        "onTheirOwnPath": "Models that are On Their Own Path may not gain Experience or receive any kind of Advancement or Promotion.",
        "wargears": "Models in a Battle Company cannot swap wargear between each other. A Warrior may drop wargear if it translates to another Option of its standard profile (marking down the difference in rating). Heroes can freely drop any wargear from their Options.",
        "youDontHaveMySword": "Optional: Heroes may discard any Wargear, even if it is built into the base profile. Any armour is discarded, not downgraded. This does not impact the model's rating unless the group decides otherwise.",
    },
}

CUSTOM_COMPANY = {
    "text": "Optional: to customise a company beyond Custom Selection, talk with the Campaign Organiser about modifying an existing company (starting company, reinforcement/special charts, advancements and the Company Hero Upgrade).",
    "rulesOfThumb": [
        "Keep it Simple, Keep it Safe.",
        "Aim to make design choices that favour a theme over an advantage.",
        "Be wary of multi-attack or multi-wound profiles and special rules that grant +1 to Wound.",
        "Consider adding Equipment to profiles before inventing new profiles from scratch (e.g. a Haradrim Warrior with a Small Shield can create a generic light spearman).",
        "Alternatively, allocate Wargear or Characteristic Increases to create new profiles (e.g. an Orc Tracker with 3+ Shoot and an Uruk Bow might make an Orc Sniper).",
        "Your Starting Battle Company should always add up to a total of 60 points of combined Rating and Gold, before Might / Will / Fate.",
        "As cool as it sounds, a Nazgul does not a company make.",
    ],
}

EXPERIENCE = {
    "progressionThreshold": 5,
    "sameForWarriorsAndHeroes": True,
    "gains": [
        {"key": "participated", "label": "Participating in the scenario", "amount": 1},
        {"key": "woundedEnemy", "label": "Wounding at least one enemy, even if it was subsequently prevented", "amount": 1},
        {"key": "slainHeroOrMonster", "label": "Each enemy Hero or Monster they slay", "amount": 1, "repeatable": True},
        {"key": "fatePrevented", "label": "Preventing a Wound by spending a Fate point", "amount": 1},
        {"key": "brokenCourageTestPassed", "label": "Rolling for and then passing a Courage test for being Broken", "amount": 1},
        {"key": "objective", "label": "Carrying or being within range of an Objective at the end of the game (includes escaping off the board edge, controlling sections of the board, etc.)", "amount": 1},
        {"key": "mvp", "label": "Being declared MVP by the opposing player after the game", "amount": 1},
    ],
    "notes": [
        "If the rider of a Cavalry model is slain but their mount stays on the board, any wounds dealt by the mount still grant Experience Points to the model.",
        "Wargs: some companies feature Wild or Fell Warg Warriors that may promote to a warg ridden by an Orc. The rider assumes the identity of the model - the Warg has been successfully tamed!",
        "Bonus Progression Rolls: some Scenarios offer 'free' advancements for a great feat or mission objective. They are not counted against the amount a model should have for its Experience Points.",
    ],
}

INJURIES = {
    "roll": "2d6",
    "applies": "Every model removed as a casualty (except models that fled the board) rolls on the chart for its type. Models that are Injured do not count towards the company's Rating.",
    "rollModifier": {"source": "campaign.aftermathEffects[kind=injuryRollModifier]", "appliesTo": "total2d6", "clamp": [2, 12], "includesRerolls": True,
                     "replacedBy": "campaign.aftermathEffects[kind=injuryRollReplacement]", "defaultNotConfirmed": ["clamp", "includesRerolls"],
                     "text": "A modifier (Healing Herbs +1) is added to the 2d6 total before the chart is read; a total above 12 reads as 12."},
    "warrior": [
        {"roll": [2, 3], "key": "dead", "name": "Dead", "effects": ["remove"],
         "text": "Remove the Warrior, and all their wargear, from the Company Roster."},
        {"roll": [4, 5], "key": "injured", "name": "Injured", "effects": ["missNextGame"],
         "healable": True, "clearsAfterGames": 1,
         "text": "The Warrior must miss the next game that your Battle Company participates in, but suffers no further effects."},
        {"roll": [6, 11], "key": "fullRecovery", "name": "Full Recovery", "effects": ["fullRecovery"],
         "text": "The Warrior makes a full recovery, and may fight in the next battle as normal."},
        {"roll": [12, 12], "key": "lessonLearned", "name": "Lesson Learned", "effects": ["fullRecovery"],
         "experience": "D3", "grantsExperience": {"dice": "d3"},
         "text": "The Warrior is considered to have made a Full Recovery. Additionally, the Warrior gains +D3 Experience."},
    ],
    "hero": [
        {"roll": [2, 2], "key": "dead", "name": "Dead", "effects": ["remove"],
         "text": "Your Hero's adventure ends here; no doubt their followers will mourn them. Remove the Hero, and all of their wargear, from your Battle Company Roster."},
        {"roll": [3, 3], "key": "armWound", "name": "Arm Wound", "effects": ["noShield", "noTwoHandedWeapon", "noPike", "noBow"],
         "persistent": True, "healable": True, "retireAtCount": 2,
         "disables": [{"kind": "gearClass", "classes": ["shield"]},
                      {"kind": "gearClass", "classes": ["two_handed"]},
                      {"kind": "gearItem", "names": ["Pike", "Elven pike"]},
                      {"kind": "gearClass", "classes": ["bow", "crossbow"]}],
         "text": "From this point on, the Hero cannot benefit from a shield in any way, use a two-handed weapon, pike, or fire a bow or crossbow of any sort. They may still carry a Company Standard with the normal penalties and fight with a Hand Weapon. If the Hero would suffer a second Arm Wound, they will be forced to retire, as per the Dead result."},
        {"roll": [4, 4], "key": "legWound", "name": "Leg Wound", "effects": ["reduceMovement"],
         "stat": {"movement": -1}, "persistent": True, "healable": True, "retireAtCount": 2,
         "statChanges": [{"stat": "movement", "delta": -1, "per": "count", "minimum": None}],
         "text": "The Hero has their Move value permanently reduced by 1\". If the Hero would suffer a second Leg Wound, they will be forced to retire, as per the Dead result."},
        {"roll": [5, 5], "key": "brokenHonour", "name": "Broken Honour", "effects": ["noStandFast", "noHeroicActionsOnCompany"],
         "persistent": True, "healable": True, "retireAtCount": 3, "grantsAtCount": {"2": {"specialRule": "Fearful"}},
         "disables": [{"kind": "ability", "key": "standFast"}, {"kind": "ability", "key": "heroicActionsOnCompany"}],
         "text": "The Hero has been shamed in battle, they may no longer provide a \"Stand Fast!\". Nor may they affect members of the Battle Company with their Heroic Actions. Should the Hero suffer this result a second time, they will gain the Fearful special rule. If the Hero would suffer a third Broken Honour, they will be forced to retire, as per the Dead result."},
        {"roll": [6, 6], "key": "justAScratch", "name": "T'is Just a Scratch", "effects": ["missNextGame"],
         "optionalReroll": True, "healable": True, "healableBy": ["attempt"],
         "reroll": {"dice": "2d6", "optional": True, "secondResultStands": True, "ifScratchAgainMissNextGame": True},
         "text": "They must miss the next game. Alternatively, the Hero may roll again on this chart - however, the second result will apply even if it is worse than the first. If this result should occur after any reroll, the Hero must miss the next game."},
        {"roll": [7, 10], "key": "fullRecovery", "name": "Full Recovery", "effects": ["fullRecovery", "healOneInjury"],
         "heals": {"count": 1, "choice": "player", "keys": ["armWound", "legWound", "brokenHonour"]},
         "text": "The Hero may play in the next game as normal. Additionally, the Hero may heal one Arm Wound, Leg Wound, or Broken Honour they previously obtained. This resets the \"count\" that the specific Injury is giving them toward a forced retirement."},
        {"roll": [11, 11], "key": "protectionByTheValar", "name": "Protection by the Valar", "effects": ["fullRecovery"],
         "stat": {"fate": 1}, "stat_note": "to the Path's Maximum",
         "statChanges": [{"stat": "fate", "delta": 1, "permanent": True, "capAtPathMaximum": True, "raisesPathMaximum": False}],
         "text": "The Hero is considered to have made a Full Recovery. Additionally, the Hero permanently gains +1 Fate point (to their Path's Maximum)."},
        {"roll": [12, 12], "key": "woundsOfAHero", "name": "Wounds of a Hero", "effects": ["fullRecovery"],
         "influence": "D6", "grantsInfluence": {"dice": "d6"},
         "text": "The Hero model's Battle Company immediately gains an extra D6 Influence Points; additionally, the Hero is considered to have made a Full Recovery."},
    ],
    "retirement": {"result": "dead", "text": "A Hero that would suffer a retireAtCount-th occurrence of an injury is forced to retire, as per the Dead result: removed from the roster with all wargear."},
    "deathOfAHero": "If your Leader is slain, choose one of your Sergeants to be promoted to Leader after the battle. If you ever have less than two Sergeants, another Hero who is not already a Leader or Sergeant may become a Sergeant. If there is no Hero available, the Warrior with the most Experience Points is instantly treated as having rolled Hero in the Making on the Warrior Progression Chart (gaining Might, Will and Fate). Ties go to the highest Rating, then the controlling player.",
    "healing": {
        "warrior": "Spend 1 Influence to remove the Injured condition from a Warrior.",
        "hero": "Spend 1 Influence to attempt to remove an Injury from a Hero on a 5+ (the roll may be modified with additional Influence the same way as a Might point), or send the Hero to the healers: they miss the next game but remove one Injury from their profile.",
    },
    # Machine-readable treatment (campaign.influence.spend[treatInjuries] points here). `cost` is what the player pays in
    # the currency named by `currency`; every number may be changed by a modified file.
    "treatment": {
        "currency": "influence",
        "phase": "companyManagement",
        "warrior": {"key": "treatWarrior", "cost": 1, "requires": "injured", "removes": "injured", "roll": None,
                    "text": "Remove the Injured condition from a Warrior."},
        "hero": [
            {"key": "attempt", "cost": 1, "removesInjuries": 1, "maxAttempts": None,
             "healable": ["armWound", "legWound", "brokenHonour", "justAScratch"],
             "roll": {"dice": "d6", "target": 5, "compare": "gte", "naturalMaxIsAlwaysEnough": False,
                      "modifier": {"currency": "influence", "perPoint": 1, "direction": "up", "maxPoints": None,
                                   "ceiling": 6, "like": "Might point"}},
             "onFailure": {"effect": "none", "costKept": True},
             "text": "Spend 1 Influence and roll a D6: on a 5+ remove one Injury, or the miss-next-game status of T'is Just a Scratch (the Hero then plays the next game). The roll may be raised with additional Influence, as with a Might point. A failed attempt loses the Influence; the player may try again, with no limit."},
            {"key": "healers", "cost": 1, "removesInjuries": 1, "healable": ["armWound", "legWound", "brokenHonour"],
             "roll": None, "missNextGame": True, "maxAttempts": None,
             "text": "Send the Hero to the healers: no roll, the Hero misses the next game but removes one Injury."},
        ],
        "removalEffect": {"countDelta": -1, "derivedEffects": "recomputeFromCounts", "choice": "player"},
        "confirmedByUser": ["hero[attempt].healable", "hero[healers].healable", "hero[attempt].maxAttempts", "removalEffect.choice", "healers.missNextGame"],
        "defaultNotConfirmed": ["healersHeroIsInjuredNextGame", "legWoundMinimumMove", "fearfulGoneBelowTwoBrokenHonour"],
    },
}

CHARACTERISTIC_LIMITS = {
    "lowerIsBetter": ["shoot", "courage", "intelligence"],
    "increases": {"maxIncreaseCountsOnly": "levelUps", "excludes": ["gear", "base", "promotions"], "trackPerStat": True,
                  "courageIntelligenceTotalCap": {"value": 3, "includesGear": True, "duringBattleToo": True}, "confirmedByUser": True},
    "limitKinds": {"max": "highest value the stat may reach", "maxIncrease": "most the stat may be raised above the profile it started with",
                   "best": "best (lowest) value allowed, for the stats where lower is better"},
    "average": {"movement": 6, "fight": 4, "shoot": 4, "strength": 3, "defence": 3, "attack": 1, "wounds": 1,
                "courage": 7, "intelligence": 7,
                "text": "The profile of an Average Battle Company character. Advancements that bring a Hero's characteristic to equal or under these values do not add to the Hero's rating."},
    "peak": {"movement": {"maxIncrease": 2}, "fight": {"max": 9}, "shoot": {"best": 2}, "strength": {"maxIncrease": 1},
             "defence": {"maxIncrease": 1}, "attack": {"max": 3}, "wounds": {"max": 3}, "courage": {"best": 3},
             "intelligence": {"best": 3}, "might": {"max": 3}, "will": {"max": 3}, "fate": {"max": 3},
             "willMaxOverrides": {"Path of the Tactician": 6, "Path of the Sorcerer": 6},
             "text": "The peak of a Battle Company character's growth: the overall ceiling for each characteristic. Each Heroic Path has its own, lower limits. Once at a maximum a Hero may not increase it again, but may still gain temporary bonuses from special rules or wargear."},
}

PROGRESSION = {
    "threshold": 5,
    # How a level-up works, as data (rulebook "3. Progression", "Characteristic Limits", "Path Progression").
    "xp": {"model": "cumulative", "spentOnRoll": False, "rollsEarned": "floor(experience / threshold)",
           "rollsAvailable": "rollsEarned + bonusRolls - rollsTaken", "bonusRolls": "scenario description, not counted against experience",
           "text": "Experience is a running total and is never spent or lost: a model has earned floor(XP / 5) rolls, takes them one by one "
                   "(several can be waiting) and keeps its Experience Points when it is promoted or becomes a Hero."},
    "timing": {"phase": "warsAftermath", "step": 3, "after": ["injuries", "experience"], "before": ["influence"],
               "text": "Progression is step 3 of the War's Aftermath, after injuries and experience and before influence. There is no rule "
                       "for rolling at another time."},
    "influenceAdjust": False, "mightAdjust": False,
    "rerollSource": "campaign.againstTheOdds.bonuses[key=reroll] (a re-roll of the War's Aftermath for a single result)",
    "limits": {"source": "campaign.characteristicLimits", "directionKey": "lowerIsBetter"},
    "warrior": {
        "roll": "d6",
        "table": [
            {"roll": [1, 2], "key": "noChange", "name": "No Change", "text": "The Warrior does not gain a promotion."},
            {"roll": [3, 5], "key": "promoted", "name": "Promoted",
             "text": "The Warrior has been granted a promotion by the leader of the company. Check the Advancements table on the company's page: the Warrior exchanges all characteristics and wargear for the new profile, while retaining any wargear that the new profile allows. A Warrior eligible for several promotions picks one. If there is no promotion, the Warrior increases Courage & Intelligence by +1 each, to a maximum of 3+."},
            {"roll": [6, 6], "key": "heroInTheMaking", "name": "A Hero in the Making",
             "text": "The Warrior is treated as a Hero and immediately gains 1 Might, Will, and Fate point. They retain their Experience Points and advance like a Hero from the next game on. Alternatively the model may forgo the Hero's Journey and take their Promotion instead."},
        ],
        "noPromotion": {"courage": 1, "intelligence": 1, "maximum": 3},
        "effectsByKey": {
            "noChange": {},
            "promoted": {"promotion": {"source": "data.companies[].advancements", "match": "from.unit + from.wargear",
                                       "exchange": ["characteristics", "wargear"], "keepAllowedWargear": True,
                                       "choice": "player", "retainsExperience": True},
                         "fallbackWhenNoAdvancement": {"improveStat": {"courage": 1, "intelligence": 1}, "limit": 3, "direction": "lowerIsBetter"}},
            "heroInTheMaking": {"becomesHero": True, "grants": {"might": 1, "will": 1, "fate": 1}, "selectsPath": True,
                                "retainsExperience": True, "advancesAsHeroFrom": "nextGame",
                                "alternative": "promoted",
                                "firstHeroRoll": "nextTimeItEarns5Xp", "pendingRollsStay": True, "confirmedByUser": ["firstHeroRoll", "pendingRollsStay"]},
        },
    },
    "hero": {
        "roll": "2d6",
        "rollsPerProgression": 2,
        "choose": 1,
        "pathSelection": "All Heroes select their Path the moment they become a Hero, either via Company Creation or Warrior Progression, and gain any starting rules and abilities related to their Path.",
        "text": "Each time a Hero gains 5 Experience Points they roll two pairs of six-sided dice (2d6), note the sum of each pair, and choose one of the two results. The Hero may only roll on their chosen Path, unless an Advancement, special rule or Scenario bonus says otherwise.",
        "resolution": {
            "dice": "2d6", "pairs": 2, "choose": 1, "onlyOwnPath": True,
            "window": "oneWindowTwoRolls",
            "unavailable": {"when": ["statAtMaximum", "ruleAlreadyOwned", "heroicActionAlreadyOwned", "minorRuleCountReached"],
                            "oneAvailable": "playerMustTakeIt", "bothUnavailable": "rerollBothUntilAvailable", "ifNothingPossible": "learnedAll"},
            "confirmedByUser": ["window", "unavailable.oneAvailable", "unavailable.bothUnavailable"],
            "companyHeroUpgrade": {"mayReplaceAnyResult": True, "unlessAlreadyOwned": True},
            "rollAgain": {"optionFlag": "improveStat.rollAgain", "meaning": "also take the other pair's result"},
            "optionsAre": "alternatives: when a row has several options (or a oneOf) the player chooses one",
            "statLimits": ["data.paths[].maximums", "campaign.characteristicLimits.peak"],
            "ratingEffect": "an increase adds rating only above campaign.characteristicLimits.average (campaign.rating.hero.perPoint)",
        },
        "unavailableResults": "If neither result can be gained due to Characteristic Maximums or duplicate Special Rules, re-roll any unavailable results until an applicable result is found (if possible).",
        "companyHeroUpgradeSwap": "Regardless of availability, a Hero may swap any result for their Company Specific Hero Upgrade if they don't already have it.",
        "exhausted": "If a Hero has already exhausted every possible result on their path, they have learned all that they need to know.",
        "pathResultTemplate": "Rolls 2, 3, 7, 11 and 12 are Path-specific; 4: Attacks or Wounds +1; 5: Courage or Intelligence +1 (and take the other roll too); 6: Strength or Defence +1; 8: Fight or Shoot +1; 9: Might, Will or Fate +1; 10: Minor Special Rule or a new Heroic Action.",
    },
}

BREAK_POINT = {
    "text": "A company's force is Broken when its casualties exceed its Break Point, checked at the start of every turn; from then on every model must pass a Courage test before it acts. "
            "Battle Companies uses the Broken Army rule of the Rules Manual (page 46) with the Break Point set per company (50%, Muster of Isengard 66%).",
    "source": ["MESBG Rules Manual: Broken Army (46), Stand Fast (47), Cavalry and Break Points", "Battle Companies: All is Lost!, Muster of Isengard company rule, scenario special rules"],
    "default": {"percent": 50, "of": "startingModels"},
    "companyOverride": "data.companies[].effects[kind=breakPoint].percent",
    "threshold": {"comparison": "strictlyGreater", "rounding": "none", "integerTest": "casualties * 100 > percent * startingModels",
                  "text": "Break Point = percent of the models that started the game, not rounded (13 models: 6.5; 12 models: 6). Broken when the casualties are greater than the Break Point: exactly 50% lost is not Broken."},
    "startingModels": {
        "count": "modelsOnTheBoardOrYetToArriveAtTheStart",
        "includes": ["warriors", "heroes", "creatures", "temporaryWanderer", "modelsYetToArrive"],
        "excludes": ["neutralModels", "scenarioModelsOfTheOpponent"],
        "perScenario": [{"scenario": "Baggage Train", "rule": "Only a Pony After All", "excludes": "Baggage Ponies", "side": "defender"}],
        "composite": "oneCountPerPhysicalModel",
        "defaultNotConfirmed": ["includes.creatures", "composite"],
        "text": "Every model of the company in the starting force, Heroes and Warriors alike; creatures are models on the board and count unless a scenario says otherwise (the Baggage Ponies). "
                "Models that arrive later (Maelstrom of Battle) count as being on the battlefield. A Cavalry model is one model; a Chariot is one model; a War Beast and its Commander are two."},
    "casualties": {
        "counts": ["removedAsCasualty", "fledAfterFailedCourageTest"],
        "doesNotCount": ["mountOfASlainRider", "escapedOffTheBoardByScenarioRule", "modelsNotYetArrived"],
        "cavalry": "The rider's death is the casualty, whether the mount stays or flees; the mount's death alone is not a casualty.",
        "defaultNotConfirmed": ["doesNotCount.escapedOffTheBoardByScenarioRule", "composite models"],
        "text": "Casualties are the models removed from the board as casualties, including models that flee after failing a Courage test for being Broken."},
    "evaluatedAt": "startOfEachTurn", "permanent": True,
    "whenBroken": {
        "courageTest": {"when": "everyActivation", "before": "anythingElse", "dice": "2d6", "passIf": "totalAtLeastCourage",
                        "pass": "stays", "fail": "fleesAndIsRemovedAsCasualty", "exempt": ["engagedInCombat", "unableToActivate"],
                        "autoPass": ["standFastInRange", "heroicResolve", "chariot", "defenderWithin6OfTheGate", "modelsOnTheHillOfDefendTheHill"],
                        "text": "Each model of a Broken company takes a Courage test every time it activates, before doing anything else. Models that cannot activate (Engaged in Combat, held by a rule or power) do not test."},
        "standFast": {"given": "byHeroThatPassedTheTest", "range": 6, "lineOfSight": True, "affects": "friendlyWarriorsActivatingInRange",
                      "when": "endOfTheHeroActivation", "cannotGive": ["brokenHonourHero", "wanderer", "heroThatCouldNotActivate"],
                      "sources": ["glossary: Stand Fast", "campaign.injuries.hero[key=brokenHonour]", "data.wanderers.rules.providesStandFast"],
                      "text": "A Hero that passes the test calls Stand Fast at the end of its activation: friendly Warriors that activate within 6\" and see it pass automatically. Other Heroes test for themselves."},
    },
    "allIsLost": {
        "decidedAt": "startOfTheTurnTheCompanyBecomesBroken", "before": "priority", "decidedBy": "brokenCompany", "oncePerMatch": True,
        "outcome": {"opponent": {"result": "victory", "scenarioRewards": "full"},
                    "fleeingCompany": {"influence": "participatingPlusAgainstTheOdds", "experience": "participatingPlusAgainstTheOdds", "scenarioRewards": False, "victoryOrDrawInfluence": False}},
        "feeds": "campaign.matchInput.perCompany[key=fledBroken]",
        "confirmedByUser": ["fleeingCompany (participation plus the Against the Odds Influence and Experience bonuses, no Win/Draw Influence, no scenario rewards; user decision mqn.114)"],
        "defaultNotConfirmed": ["decidedAt (only the turn it becomes Broken)"],
        "text": "At the start of the turn it becomes Broken, before the Priority phase, a company may flee the battlefield: the opponent wins and claims the full scenario rewards; the fleeing company keeps the participation Influence and Experience and its Against the Odds Influence and Experience bonuses, but no Win/Draw Influence and no scenario rewards."},
    "experience": {
        "gain": "campaign.experience.gains[key=brokenCourageTestPassed]", "perModelPerMatch": 1, "requires": "rolledAndPassed",
        "automaticPassesDoNotCount": ["standFast", "heroicResolve", "chariot", "scenarioAutoPass"],
        "text": "+1 Experience to each model that rolled for the Courage test for being Broken and passed it, once per match however many times it passed. An automatic pass (Stand Fast, Heroic Resolve, a scenario rule) is not a roll. A model that fails flees and is removed."},
    "scenarioExceptions": [
        {"scenario": "Stand Your Ground", "rule": "Defend the Hill", "effect": "noCourageTest", "who": "modelsOnTheHill",
         "text": "Models standing on the hill never take Courage tests for being Broken."},
        {"scenario": "Defend the Gate", "rule": "Let nothing through!", "effect": "autoPassCourage", "who": "defenderWithin6OfTheGate",
         "text": "Defender models within 6\" of the Gate pass every Courage test automatically."},
        {"scenario": "Baggage Train", "rule": "Only a Pony After All", "effect": "notCountedForBreakPoint", "who": "defenderBaggagePonies",
         "text": "The Defender's Baggage Ponies are not counted when working out its Break Point."},
        {"scenarioSetting": "Neutral Third-Party", "effect": "noCourageTest", "who": "neutralModels",
         "text": "Neutral models never take Courage tests for being Broken."},
        {"scenarioSetting": "Maelstrom of Battle", "effect": "yetToArriveCountAsOnTheBattlefield", "who": "ownModelsNotArrived",
         "text": "Models that have not arrived yet count as being on the battlefield when checking whether the company is Broken."},
    ],
    "liveMatch": {"track": ["startingModels", "casualties", "breakPoint", "broken", "brokenAtTurn"],
                  "text": "The live match shows the starting models, the casualties and the Break Point per company, flags Broken at the start of the turn the test above turns true, and offers All is Lost in that turn."},
}

INFLUENCE = {
    "gains": {"participating": 2, "draw": 1, "win": 2},
    "text": "A Battle Company always gains 2 Influence Points for participating in a Scenario, +1 for a Draw and +2 for a Win.",
    "allIsLostSee": "campaign.breakPoint.allIsLost",
    "allIsLost": "Any Battle Company may voluntarily flee the battlefield once their force is Broken, decided at the beginning of the turn they are broken and before priority. The opponent claims victory and full scenario rewards; the fleeing company earns the standard Influence for participating, but nothing further.",
    "spend": [
        {"key": "recruit", "name": "Recruit Reinforcements",
         "defaultCost": 3, "companyCostPath": "data.companies[].reinforcements.influence",
         "text": "Roll on the company's Reinforcement chart. It costs 3 Influence for most companies (see each company page for its Reinforcement cost). A Special Chart result lets you continue on the company's Special Chart. If a roll would give a Rare model the company cannot have, choose a lower result on the same chart. If a roll would break the Wargear Limits you may either increase your roll with Influence or take the next option down."},
        {"key": "adjustRoll", "name": "Adjust a Reinforcement Roll",
         "perPoint": 1, "costPerPoint": 1, "directions": ["up"], "maxPerRoll": 3, "details": "campaign.recruitment.adjust", "carriesToSpecialChart": False,
         "text": "For each Influence Point spent you may increase the die of a Reinforcement roll by 1 (house rule: the rulebook also allows decreasing; lowering is never paid, see campaign.recruitment). Spent Influence does not carry over to adjust rolls on the Special Chart. A maximum of 3 Influence can be spent adjusting across both the regular and Special Charts per Reinforcement Roll."},
        {"key": "treatInjuries", "name": "Treat Injuries",
         "cost": 1, "details": "campaign.injuries.treatment",
         "text": "Spend 1 Influence to: remove the Injured condition from a Warrior; attempt to remove an Injury from a Hero on a 5+ (modifiable with additional Influence like a Might point); or send a Hero to the healers (they miss the next game but remove one Injury)."},
        {"key": "armoury", "name": "Visit the Armoury",
         "costSource": ["data.wargear.items[].influence", "data.wargear.armourUpgrade.influencePerUpgrade", "data.equipment.items[].influence", "data.creatures.items[].influence", "data.wanderers.items[].influence", "data.companies[].uniqueWargear[].influence", "data.companies[].specialUnits[].influence"],
         "text": "Buy additional equipment and wargear for the cost in Influence Points. Warriors may only buy wargear matching one of their profile's Options. Heroes may take any wargear (including Armour upgrades) available to any profile included in their company's Reinforcement and Special Charts and Promotion table."},
    ],
    "reinforcementRollOfOne": "Nobody steps forward: no Influence is spent but you may not roll again this Post-Battle Sequence.",
    "rareUnits": "Models shown with (Rare X) can only ever be taken up to X times by a company.",
    "specialUnits": "Some companies have profiles that are not on their charts (e.g. Moria's Cave Troll). They have their own costs and limitations on the company page.",
    "wargearLimits": "Battle Companies use the MESBG Bow & Throwing limits, but Heroes count towards them. Additionally, only up to 33% of a Battle Company may have the Cavalry keyword. Promotions are affected by these limits. Some companies list exceptions.",
}

RATING = {
    "text": "The Battle Company Rating is the sum of the rating of its models. Models that are missing the next game because they are Injured do not count.",
    "warrior": "A Warrior is worth the same points as in MESBG plus the rating of any Equipment it bears.",
    "hero": {
        "base": "pointsOfWarriorProfile",
        "perPoint": [
            {"stat": "might", "rating": 5}, {"stat": "will", "rating": 5}, {"stat": "fate", "rating": 5},
            {"stat": "movement", "rating": 5}, {"stat": "fight", "rating": 5}, {"stat": "strength", "rating": 5},
            {"stat": "defence", "rating": 5}, {"stat": "attack", "rating": 10}, {"stat": "wounds", "rating": 10},
        ],
        "perSpecialRule": 5,
        "perMinorSpecialRule": 5,
        "maxMinorSpecialRuleRating": 10,
        "noRating": ["shoot", "courage", "intelligence", "heroicActions", "magicalPowers", "castingValueImprovements"],
        "notes": [
            "Add the point value of the Warrior profile the Hero is built from, then the ratings above.",
            "Advancements that bring a characteristic to equal or under the Average Battle Company profile (see characteristicLimits.average) do not add to rating.",
            "Special Rules that improve with additional results do not increase in rating with each additional improvement.",
        ],
        "wargear": {
            "threshold": {"stats": ["attack", "wounds"], "sumAtLeast": 3},
            "text": "Wargear built into the base profile or in the profile's Options list adds no rating. If a Hero's Attacks and Wounds add up to at least 3, wargear and equipment that were neither part of the base profile nor of its Options list use the (hero) rating from the Armoury and Equipment lists. E.g. a Ranger of Gondor pays nothing for armour, bow or spear, but pays for a horse or heavy armour.",
        },
    },
}

AGAINST_THE_ODDS = {
    "text": "The Battle Company with the lower rating may select Against the Odds bonuses after the roll on the Scenario Table was made, but before setting up. Each bonus adds its Rating to the company's rating, which must not exceed the opponent's as a result of these bonuses. The rating difference is checked before Against the Odds bonuses.",
    # How the bonuses are chosen (shared by all of them). Source: the intro of the table and changelog 1.1 item 8.
    "selectedAt": "afterScenarioRollBeforeSetup",
    "eligible": {"company": "lowerRating", "compare": "strictlyLower", "ratingBeforeBonuses": True,
                 "text": "Only the company with the lower Rating chooses; at equal Ratings nobody does. The comparison (and the 25 rating difference of the scenarios with an Attacker and a Defender) is made before Against the Odds bonuses."},
    "budget": {"rule": "ratingAfterBonusesNotAboveOpponent", "inclusive": True,
               "text": "The sum of the Ratings of the chosen bonuses may not take the lower company above its opponent's Rating (equal is allowed: 'must not exceed'). So a bonus needs a Rating gap at least as large as its price: Ambush! needs a gap of 60 on its own."},
    "defaultNotConfirmed": ["budget.inclusive", "repeatability of the bonuses other than the Tool Kit"],
    "bonuses": [
        {"key": "influence", "name": "Influence", "rating": 15, "gains": {"influence": {"win": 2, "other": 1}}, "appliesIn": ["aftermath"],
         "text": "+2 Influence for Victory, +1 for Draw or Defeat."},
        {"key": "experience", "name": "Experience", "rating": 30, "gains": {"experiencePerModel": {"win": 2, "other": 1}}, "appliesIn": ["aftermath"],
         "text": "+2 Experience per model for Victory, +1 per model for Draw or Defeat."},
        {"key": "reroll", "name": "Reroll", "rating": 15, "count": 2, "usableIn": ["game", "warsAftermath"], "appliesIn": ["game", "aftermath"],
         "affects": "singleDieOrSingleResult", "expires": "endOfMatchAftermath", "carriesOver": False, "defaultNotConfirmed": ["expires", "carriesOver"], "text": "Two rerolls that each can be used at any point during the game to re-roll a single die or the War's Aftermath for a single result."},
        {"key": "toolKit", "name": "Tool Kit", "rating": 30, "appliesIn": ["setup", "game", "aftermath"],
         "text": "You may only have one of each Kit, but their items may be distributed to any members and ignore all capacity / alignment rules.",
         # One purchase (30 Rating) is one Kit chosen from `kits`; the same Kit cannot be taken twice. Interpretation, see REVIEW in bead 36y.1.
         "purchase": {"chooses": "oneKit", "distinctKits": True, "maxPurchases": 5, "ratingPerKit": True},
         "grants": {"kind": "loanedEquipment", "assignTo": "anyModelOfTheMatch", "assignAt": "beforeSetup", "oncePerKit": True,
                    "ignores": ["capacity", "alignment"], "stillApplies": ["heroOnly"], "addsRating": False,
                    "expires": "endOfMatchAftermath", "carriesOver": False, "usableByTemporaryWanderer": False},
         "defaultNotConfirmed": ["purchase", "grants.stillApplies", "grants.expires", "grants.carriesOver", "grants.usableByTemporaryWanderer"],
         "kits": [
             {"name": "Healer's Kit", "items": [{"equipment": "Wondrous Cram", "count": 5}, {"equipment": "Healing Herbs", "count": 2}]},
             {"name": "Explorer's Kit", "items": [{"equipment": "Scroll of Hidden Paths", "count": 1}, {"equipment": "Mountain Boots", "count": 3},
                                                  {"equipment": "Woodland Belt", "count": 3}, {"equipment": "Map", "count": 1}]},
             {"name": "Scholar's Kit", "items": [{"equipment": "Ring of Warding", "count": 1}, {"equipment": "Badge of Courage", "count": 1},
                                                 {"equipment": "Lucky Talisman", "count": 1}, {"equipment": "Seeing Stone", "count": 1}]},
             {"name": "Hunter's Kit", "items": [{"equipment": "Envenom Weapon (X)", "count": 5}, {"equipment": "Trophy Pelt", "count": 1},
                                                {"equipment": "Concealing Cloak", "count": 1}]},
             {"name": "Raider's Kit", "items": [{"equipment": "Torching Brand", "count": 5}, {"equipment": "Climbing ropes", "count": 5},
                                                {"equipment": "Dwarven Brew", "count": 1}]},
         ]},
        {"key": "wanderer", "name": "Wanderer", "rating": 45, "temporary": True, "appliesIn": ["setup", "game"],
         "text": "You may temporarily recruit a Wanderer for the match.",
         # The flat 45 replaces the Wanderer's own points (changelog 1.1 item 3); no Influence or Gold is paid.
         "grants": {"kind": "temporaryModel", "from": "data.wanderers.items", "count": 1, "ratingCost": "bonusRating", "influenceCost": 0, "goldCost": 0,
                    "takesRosterSlot": False, "countsTowardsMaxModels": False, "countsTowardsBreakPoint": True, "exclusiveWithPermanentWanderer": True,
                    "rollsInjuries": False, "gainsExperience": False, "persistsAfterMatch": False, "expires": "endOfMatch",
                    "rules": "data.wanderers.rules (no Stand Fast, no Heroic Actions on the company, no Experience or Advancements)"},
         "defaultNotConfirmed": ["grants.countsTowardsMaxModels", "grants.exclusiveWithPermanentWanderer", "grants.rollsInjuries", "grants.expires"]},
        {"key": "ambush", "name": "Ambush!", "rating": 60, "appliesIn": ["scenarioChoice"], "text": "You choose the scenario.",
         "grants": {"kind": "chooseScenario", "from": "scenarios", "replaces": "scenarioTableRoll", "chosenBy": "lowerRatedCompany",
                    "rolesBy": "scenarioRules", "duration": "thisMatch"},
         "defaultNotConfirmed": ["grants.replaces", "grants.rolesBy"]},
    ],
}

ARMOURY_WARGEAR = {
    "text": "The most common types of wargear available to models in a Battle Company. Each entry has an Influence cost and a rating (warrior / hero). If the Hero's faction uses a lower/higher point value for a Warrior, use that for the first (warrior) value instead. Elven-made weaponry uses the generic equivalent values but is restricted to Elf models unless otherwise noted. Companies may list additional wargear on their page.",
    "items": [
        {"names": ["Armoured Horse", "Goat", "Galadhrim Steed"], "type": "mount", "influence": 4, "rating": {"warrior": 15, "hero": 25}},
        {"names": ["Horse", "Fell Warg", "Warg"], "type": "mount", "influence": 3, "rating": {"warrior": 10, "hero": 20}},
        {"names": ["Bow", "Dwarf Bow", "Uruk-hai Bow", "Orc Bow", "Short Bow"], "type": "bow", "influence": 1, "rating": {"warrior": 1, "hero": 5}},
        {"names": ["Elf Bow", "Esgaroth Bow", "Longbow", "Dwarf Longbow", "Crossbow"], "type": "bow", "influence": 2, "rating": {"warrior": 2, "hero": 5}},
        {"names": ["Blowpipe"], "type": "blowpipe", "influence": 1, "rating": {"warrior": 1, "hero": 5}},
        {"names": ["Throwing Weapon", "Throwing Spear"], "type": "throwingWeapon", "influence": 1, "rating": {"warrior": 2, "hero": 5}},
        {"names": ["Pike", "Spear", "Lance", "Warspear"], "type": "weapon", "influence": 1, "rating": {"warrior": 1, "hero": 5}},
        {"names": ["Hand-and-a-half Weapon", "Two-handed Weapon", "Mattock"], "type": "weapon", "influence": 1, "rating": {"warrior": 1, "hero": 5}},
        # "Simple hybrid-weapons (e.g. Axe of Lossarnach or Dunlending War Axe)": the Easterling Halberd (a spear or a two-handed weapon,
        # chosen per fight) is the third hybrid weapon of the profiles; confirmed by the user (2026-10-20).
        {"names": ["Axe of Lossarnach", "Dunlending War Axe", "Easterling Halberd"], "type": "weapon", "influence": 1, "rating": {"warrior": 1, "hero": 5},
         "note": "Simple hybrid-weapons"},
        {"names": ["Flaming Brand"], "type": "weapon", "influence": 1, "rating": {"warrior": 1, "hero": 5}},
        {"names": ["Light Shield", "Shield"], "type": "shield", "influence": 1, "rating": {"warrior": 1, "hero": 5}},
    ],
    "armourUpgrade": {
        "text": "Upgrade Armour by one stage. Each upgrade costs 1 Influence and adds (1 / 5) rating.",
        "stages": [["None", "Light Armour"], ["Armour"], ["Heavy Armour", "Dwarf Armour"]],
        "influencePerUpgrade": 1, "rating": {"warrior": 1, "hero": 5},
    },
    "restrictions": [
        "Warrior models follow their usual Options rules and may only purchase items that fit that list. A Warrior may drop Wargear provided it keeps a legal Option, or buys a new legal Option.",
        "Hero models are not bound to Warrior Options. They can purchase any wargear available to any member of their Battle Company, including those that could be promoted or join as Reinforcements; but not those that joined by other means. Some options are explicitly unique to certain profiles and are not in this table. A piece of wargear gained through other means does not let Heroes buy the same type.",
        "Heroes need to take note of the rules for Shields being cumbersome (MESBG rules manual, p. 110).",
    ],
    "beasts": "Models with the Beast keyword may not take additional wargear other than upgrading their Armour and may only carry Equipment that exclusively gives a Special Rule (e.g. Trophy Pelt).",
}

EQUIPMENT_TEXT = {
    "text": "Equipment is not as restricted as wargear as to who can take it, but some pieces can only be purchased by certain companies, only by Heroes, or only by Good or Evil models. The number in brackets is the rating the item adds (warrior / hero, like the Armoury). A model may only carry one Large and one Small piece of Equipment. Equipment is lost if its owner is slain.",
}

# Per-creature behaviour, merged into data.creatures.items[] by name.
CREATURE_ITEM_RULES = {
    "Baggage Pony": {
        "companyFeature": {
            "key": "carryGear",
            "capacity": 15,
            "unit": "pieces of equipment, weapons or additional wargear",
            "excludes": ["mount"],
            "carriedItemsInactive": True,
            "swap": {
                "who": "anyCompanyMember",
                "perMemberPerOccasion": 1,
                "actions": ["retrieve", "store", "swap"],
                "when": [
                    {"phase": "warsAftermath", "after": "injuries", "before": "scenarioSelection"},
                    {"phase": "scenario", "condition": "beginsTurnInBaseContactWithCreature"},
                ],
                "cost": {"influence": 0},
                "mustObeyWargearLimits": True,
                "mustObeyUnitRestrictions": True,
            },
            "storedItemsCountTowardsLimits": True, "loadLostWithOwner": True,
            "confirmedByUser": ["storedItemsCountTowardsLimits", "loadLostWithOwner"],
            "defaultNotConfirmed": ["swap.perMemberPerOccasion", "swap.cost"],
        },
    },
    "Falcon": {
        "handlersGlove": {"startsOnOwnersArm": True, "releaseWithinInches": 3, "landWithinInches": 3, "landPhase": "end",
                          "untargetableOnArm": True},
        "aura": {"key": "keenSight", "radiusInches": 12, "condition": "separateFromOwner", "effect": "enemiesLoseStalkUnseen"},
    },
    "Hunting Dog": {"courage": {"sameAsOwner": True}},
}

CREATURES_TEXT = {
    "rules": {
        # Everything below is the Creatures section of sources/battle_companies_rules.txt as data. `confirmedByUser` lists keys the user
        # decided; `defaultNotConfirmed` lists defaults chosen by the app that the user has not confirmed.
        "attachTo": ["leader", "sergeant"],
        "maxPerHero": 1,
        "maxPerCompany": 3,
        "takesRosterSlot": False,
        "countsTowardsMaxModels": False,
        "usesEquipmentSlots": False,
        "countsTowardsWargearLimits": False,
        "onBoard": True,
        "deployWithinOwnerInches": 6,
        "rollsInjuries": False,
        "diesWithOwner": True,
        "lostIfOwnerSlain": True,
        "onTheirOwnPath": True,
        "gainsExperience": False,
        "neverUnarmed": True,
        "ratingCounts": "points",
        "costCurrency": "influence",
        "perOfficer": 1,
        "confirmedByUser": ["attachTo", "maxPerHero", "maxPerCompany", "takesRosterSlot", "usesEquipmentSlots", "rollsInjuries",
                            "countsTowardsWargearLimits"],
        "defaultNotConfirmed": ["gainsExperience", "ratingCounts"],
    },
    "text": "A purchased Creature must be attached to the Leader or one of the Sergeants - it does not take a space on the roster. Each Hero may only ever have a single Creature (max. 3 in the company). Creatures are On Their Own Path and do not roll on any Injury chart if slain; a Creature is only Dead if the Hero it is attached to would be. Creatures are never considered to be unarmed.",
}

WANDERERS_TEXT = {
    "rules": {
        "maxAtOnce": 1,
        "role": "hero",
        "isHero": True,
        "takesRosterSlot": True,
        "countsTowardsMaxModels": True,
        "canBuyWargear": False,
        "canBuyEquipment": False,
        "gainsExperience": False,
        "gainsAdvancements": False,
        "providesStandFast": False,
        "heroicActionsAffectCompany": False,
        "rollsInjuries": "hero",
        "countsTowardsWargearLimits": True,
        "canBeDismissed": True,
        "dismissRefund": 0,
        "injuriesTreatable": True,
        "alignment": None,
        "ratingCounts": "points",
        "costCurrency": "influence",
        "temporaryVariant": "campaign.againstTheOdds.bonuses[key=wanderer]",
        "confirmedByUser": ["takesRosterSlot", "countsTowardsMaxModels", "countsTowardsWargearLimits", "maxAtOnce", "canBeDismissed",
                            "dismissRefund", "rollsInjuries", "injuriesTreatable", "gainsExperience", "gainsAdvancements"],
        "defaultNotConfirmed": ["ratingCounts", "alignment", "cannotBecomeLeaderOrSergeant"],
    },
    "text": "A Battle Company may only ever have one Wanderer at a time. Wanderers cannot buy additional wargear or Equipment. They are Heroes in all ways with these exceptions: they cannot gain Experience Points or receive Advancements; they do not provide a Stand Fast!; they may not affect members of the Battle Company with their Heroic Actions (e.g. calling \"With Me!\").",
}

SCENARIO_SELECTION = {
    "text": "Compare the Battle Company Ratings: the higher rating is the Top Dog, the other the Underdog. The Underdog is referenced in scenarios for special rules and automatically wins the roll-off to determine Layout and Starting Positions (on a tie follow the normal scenario rules). Then roll a D6 to pick one of the three tables, and a second D6 to pick the Scenario (ideally rolled by different players).",
    "layout": "All scenarios are written with a 48\"x48\" board in mind (36\"x36\" is popular). Cover the board with a dense amount of terrain; some scenarios have specific terrain requirements.",
    "tables": [
        {"roll": [1, 2], "scenarios": ["Chance Encounter", "Show of Strength", "The Wolves Attack", "Fog of War", "Tame the Beast", "Rescue"]},
        {"roll": [3, 4], "scenarios": ["Relic of Ages Past", "Seize Ground", "Sites of Power", "Plunder the Supplies", "Hoard of Treasures", "Scavenge the Battlefield"]},
        {"roll": [5, 6], "scenarios": ["Secure the Area", "Stand Your Ground", "Hold the Line", "Defend the Gate", "Seize the Outpost", "Baggage Train"]},
    ],
    "maelstromOfBattle": {
        "text": "At the end of each of your Move phases, roll a D6 for one of your models not already on the battlefield and consult the chart (Hero models can use Might to increase this roll), then roll for the next. Models can't Charge in the turn that they arrive. Models yet to arrive count as being on the battlefield for the purposes of determining if your force is Broken.",
        "table": [
            {"roll": [1, 1], "text": "The model has not arrived yet."},
            {"roll": [2, 2], "edge": "west", "text": "The model enters the battlefield from the western board edge."},
            {"roll": [3, 3], "edge": "east", "text": "The model enters the battlefield from the eastern board edge."},
            {"roll": [4, 4], "edge": "south", "text": "The model enters the battlefield from the southern board edge."},
            {"roll": [5, 5], "edge": "north", "text": "The model enters the battlefield from the northern board edge."},
            {"roll": [6, 6], "edge": "any", "text": "The model enters the battlefield from any board edge chosen by the controlling player."},
        ],
    },
    "specialRules": [
        {"name": "Command Support", "definition": "Units with the Command keyword are referenced periodically in scenarios. This keyword is granted by certain Equipment to represent the command retinue that accompanies the Leader."},
        {"name": "Neutral Third-Party", "definition": "If the NPC Scenario is using the rules for a Neutral Third-Party, there are three sides of the battle: each battle company and the non-player forces. The non-player models activate first in each Move phase, even before Heroic Moves, in the order that is most advantageous to them (e.g. so as not to block each other's paths). What the models do when they activate depends on their behaviour. Regardless of behaviour, neutral models will not take Courage tests for being Broken."},
        {"name": "Aggressive", "definition": "When activated, each model will follow the most efficient path in an effort to Engage models from either Battle Company."},
        {"name": "Passive", "definition": "These models will always Activate first in each Move phase, even before Heroic Move. Starting with the player without Priority, players alternate moving these models until all have been moved. Passive Neutral models do not make any actions that would result in risking failed climb, leap or swim tests. They cannot willingly go prone and will stand up as soon as it is possible. Any neutral model armed with a ranged weapon activates in such a way that they can still shoot. During the Shoot Phase, they must target the nearest enemy model to them that also provides the greatest odds of hitting their target."},
        {"name": "Resolving Combat", "definition": "In a combat involving a neutral model and members of both companies that are not in base contact with one another, strikes will alternate starting with the Company that rolled the highest to win the Duel. Ties in the highest roll are then determined by the highest Fight Value on each side. Further ties are determined by a roll-off. If only one side is engaged with a neutral model, then the opposing player makes all decisions for that model, including using any Brutal Power Attacks! In the event all sides are engaged with each other, then for each combat of this nature the decisions of the NPC in each similar combat will alternate between players; starting with the player without Priority."},
        {"name": "Allies of Convenience", "definition": "Some scenarios pit the Battle Companies against a common enemy. Members of both count as friendly models to each other. Players roll for Priority as normal, with the player that won Priority acting with their models first in each phase if able. When resolving Ranged Attacks or Magical Powers, Good models only follow their Good targeting restrictions if they would risk hitting another Good model."},
    ],
}


# Special rules the scenarios use that no path or upgrade defines: they get an entry in data.keywords so that gear can grant them and the
# glossary can show them. Paraphrased from the "Scenario Special Rules" section and the scoring bullets of the scenarios that use it.
SCENARIO_KEYWORDS = [
    {"name": "Command Support", "source": "Scenario Special Rules",
     "definition": "A keyword some equipment (a War Drum, a War Horn, a Pennant, an Instrument) gives its bearer to stand for the leader's command retinue. "
                   "It has no effect in play: a few scenarios score Victory Points for Command Support models still alive at the end of the game."},
]


# Rules that take a parameter "X". Vanilla already defines most of them as families of concrete rules (Hatred (Man),
# Hatred (Rohan)...); here the family is formalised once: what X means and who chooses it. `definitionFrom` names the
# vanilla rule whose text is the template. Battle Companies generalisations replace a named character with the rule's
# owner / a parameter: those carry `overrides` (Battle Companies always wins over vanilla).
PARAMETERISED_RULES = [
    {"name": "Hatred", "pattern": "Hatred (X)", "type": "specialRule", "definitionFrom": "Hatred (Man)",
     "parameter": {"name": "X", "kind": "keyword",
                   "description": "A race, faction or nationality keyword. The rule works against enemy models with that keyword; any race may be chosen when the rule is taken."}},
    {"name": "Poisoned Attacks", "pattern": "Poisoned Attacks (X)", "type": "specialRule", "definitionFrom": "Poisoned Attacks (Bow)",
     "parameter": {"name": "X", "kind": "weapon",
                   "description": "One of the weapons on the model's profile, chosen when the rule is taken (e.g. with Envenom Weapon (X)). Only that weapon benefits."}},
    {"name": "Master of Battle", "pattern": "Master of Battle (X+)", "type": "specialRule", "definitionFrom": "Master of Battle (5+)",
     "parameter": {"name": "X", "kind": "rollTarget", "min": 2, "max": 6,
                   "description": "The D6 score needed, from 2+ (best) to 6+ (worst)."}},
    {"name": "Dominant", "pattern": "Dominant (X)", "type": "specialRule", "definitionFrom": "Dominant (2)",
     "parameter": {"name": "X", "kind": "number", "min": 2,
                   "description": "How many models the model counts as."}},
    {"name": "Fury", "pattern": "Fury (X)", "type": "magicalPower", "definitionFrom": "Fury",
     "parameter": {"name": "X", "kind": "keyword",
                   "description": "The species (keywords) of the friendly models that benefit; chosen when the power is taken, normally the company's race."}},
    {"name": "Combat Synergy", "pattern": "Combat Synergy (X)", "type": "specialRule", "definitionFrom": "Combat Synergy",
     "overrides": "Combat Synergy",
     "replace": [("Kili and Fili are", "this Hero and X are")],
     "parameter": {"name": "X", "kind": "companyHero",
                   "description": "A friendly Hero of the same Battle Company, selected when the rule is taken (vanilla names Kili and Fili)."}},
    {"name": "Terror", "pattern": "Terror (X)", "type": "specialRule", "definitionFrom": "Terror",
     "append": " When the rule is shown with a keyword in brackets (e.g. the Torching Brand's Terror (Beast) and Terror (Cavalry)), it only affects models that have that keyword.",
     "parameter": {"name": "X", "kind": "keyword", "optional": True,
                   "description": "Optional restriction: only models with this keyword are affected."}},
]

# Vanilla rules written around a named character that Battle Companies uses for any Hero ("with only enough wording
# tweaks to make it function without that specific named character").
GENERALIZED_VANILLA_RULES = [
    {"name": "Well-aimed Shot", "replace": [("Damrod", "this model")]},
]

# Vanilla units list a special rule under a spelling its own rule table does not use (the vanilla file is never modified): the
# Battle Companies file carries the spelling the units write, with the text copied from the vanilla rule, so every lookup resolves.
SPELLING_VARIANT_RULES = [
    {"name": "A fell light is in them", "vanilla": "A Fell Light is in Them"},  # Dead Marsh Spectre (Minas Morgul, Angmar)
]


# Restrictions are by unit identity (user, 2026-10-21): the "X Only" line of a unique wargear item is read as the exact units it names, not as
# a keyword the unit may share with others (the Half Troll has the Mahud keyword and still cannot buy the War Camel). Key: (company, item).
UNIQUE_WARGEAR_UNITS = {
    ("Grand Army of the South", "War Camel"): ["Mahûd Warrior"],  # printed "Limit 1; Mahûd Only"
    # printed "Limit 1; Corsair Only": the Corsair units of the company that owns the item (City of Umbar's chart units named Corsair)
    ("City of Umbar", "Smoke Bombs"): ["Corsair Crossbowman", "Corsair Reaver", "Corsair of Umbar"],
}
# A restriction that is a kind of model, not a list of units (printed "Hobbit Heroes"): the race and the Hero role.
UNIQUE_WARGEAR_KIND = {("The Shire", "Pony"): {"race": "Hobbit", "heroesOnly": True}}
# Hero upgrades whose "(X Only)" is narrower than the word suggests: `restrictedToUnits` are exact units.
HERO_UPGRADE_UNITS = {
    ("Wolves of Isengard", "Feral Charge"): {"restrictedToUnits": ["Isengard Warg"],
                                             "restrictionNote": "REVIEW (user, 2026-10-21): Warg only means Isengard Warg heroes; the company recruits the "
                                             "Isengard Warg without it, and the Isengard Warg Rider (an Orc) cannot take it."},
}


# Company names the title-casing of the document's capitals heading gets wrong (official spelling, user decision 2026-10-21).
COMPANY_NAMES = {"lake town": "Lake-town"}  # key: fold of the heading


# Vanilla names that are defined more than once with different content, or needed by this file (directly, or through the
# vanilla units it uses): every one is reviewed here and recorded in data.vanillaAmbiguities (the generator stops on an
# unreviewed one). Unit occurrences count the heroes list first, then the warriors list (the loader's order). The loader's rule of thumb is "the first occurrence wins" (confirmed against the official rules).
#   equivalent : the entries only differ in wording, any of them works
#   overridden : Battle Companies defines the name itself, so vanilla is not consulted
#   firstWins  : the entries really differ; the first one is the right one (rule of thumb holds, nothing is copied)
#   pinned     : the rule of thumb fails here; `use` (1-based occurrence) says which vanilla entry wins
DUPLICATE_DECISIONS = {
    ("keywords", "Caras Galadhon Fighting Style"): ("equivalent", None, "Same rule (the Guard may use Shielding with the pike), different wording."),
    ("keywords", "Dead-eye Shot"): ("equivalent", None, "Same rule, only spelling differs."),
    ("keywords", "Spear Mastery"): ("equivalent", None, "Same rule, only capitalisation differs."),
    ("keywords", "Pride of Dol Amroth"): ("equivalent", None, "Identical."),
    ("keywords", "Longbeard"): ("overridden", None, "Replaced by the Path of the Tactician's generalised Longbeard (Battle Companies override)."),
    ("keywords", "One Final Blow"): ("overridden", None, "Replaced by the Helm's Deep Company Hero Upgrade of the same name (Battle Companies override)."),
    ("keywords", "Enchanting Song"): ("firstWins", 1, "Two versions of the Wood Elf Sentinel's songs; the second uses the older wording (Courage test, Eldamar Madrigal takes control of the target)."),
    ("keywords", "Mercenary Ambush"): ("firstWins", 1, "Two versions (Intelligence Test vs older Courage test to arrive); the first is the current one."),
    ("keywords", "Relentless Advance"): ("firstWins", 1, "Two versions of the Gundabad Ogre rule; the second is the older, Ogre-specific text that still mentions Command."),
    ("keywords", "The Honour of Lamedon"): ("firstWins", 1, "Two different rules: the Clansman's (first) and Angbor's banner effect (second, used by no unit)."),
        ('units', 'Aldor, Rohan Archer'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Aragorn (Strider)'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Arwen Undomiel'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Balin the Dwarf'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Barliman Butterbur'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Bifur the Dwarf'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Bilbo Baggins'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Bilbo Baggins, Ageing Hobbit'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Bilbo Baggins, Master Burglar'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Bill the Pony'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Black Guard Drummer'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Bofur the Dwarf'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Bombur the Dwarf'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Boromir of Gondor'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Castellan of Dol Guldur'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Dori the Dwarf'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Dwalin the Dwarf'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Eowyn, Shieldmaiden of Rohan'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Fili the Dwarf'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Folco Boffin'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Fredegar `Fatty` Bolger'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Frodo Baggins'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Gildor Inglorion'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Gimli, Son of Gloin'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Gloin the Dwarf'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Grima Wormtongue'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Haleth, Son of Hama'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Kili the Dwarf'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Legolas Greenleaf'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Lobelia Sackville-Baggins'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Meriadoc Brandybuck'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Meriadoc Brandybuck, Esquire of Rohan'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Mordor Orc Drummer'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Nazgul of Dol Guldur (Dark Headsman)'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Nazgul of Dol Guldur (Khamul)'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Nazgul of Dol Guldur (The Abyssal Knight)'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Nazgul of Dol Guldur (The Forsaken)'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Nazgul of Dol Guldur (The Lingering Shadow)'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Nazgul of Dol Guldur (The Slayer of Men)'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Nazgul of Dol Guldur (Witch King)'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Nori the Dwarf'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Oin the Dwarf'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Ori the Dwarf'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Peregrin Took, Guard of the Citadel'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Samwise Gamgee'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'The Gatekeeper of Bree'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'The Keeper of the Dungeons'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Tom Bombadil'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Troll Brute'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Uruk-Hai Drummer'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Goldberry'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Gollum'): ('equivalent', None, 'Same profile, rules and wargear; the entries only differ in army-building data (factions, unique, countAsProperty, option availability). The heroes entry is the first.'),
    ('units', 'Alfrid the Councillor'): ('firstWins', 1, "Re-reviewed 2026-10-22 with the user's heuristic (an older entry lacks Intelligence, carries a 'legacy' value or misses fields; otherwise the first wins): the first (heroes) entry has Intelligence 7, Might 1 and Fearful; the second has no Intelligence, so it is the older entry. First kept."),
    ('units', 'Bain, Son of Bard'): ('pinned', 2, 'The first (heroes) entry misspells the unit type key (uniType) so the type list is lost; the second (warriors) entry is the complete one. Profile otherwise identical.'),
    ('units', 'Mordor Orc Siege Veteran'): ('pinned', 2, "Re-reviewed 2026-10-22 with the user's heuristic (an older entry lacks Intelligence, carries a 'legacy' value or misses fields; otherwise the first wins): both entries are complete and in the heroes list; they differ only in unitType. 'Warrior, Hero' is the only entry of the file that mixes Warrior and Hero (the types exclude each other); 'Infantry, Hero' is how the other automatically added heroes are typed. The second is pinned."),
    ('units', 'Peregrin Took'): ('pinned', 2, 'The first (heroes) entry has no race, unit type or base size; the second (warriors) entry is the complete one. Same stats otherwise.'),
    ('units', 'Radagast the Brown'): ('firstWins', 1, "Re-reviewed 2026-10-22 with the user's heuristic (an older entry lacks Intelligence, carries a 'legacy' value or misses fields; otherwise the first wins): Nature's Wrath casts on 5+ in the first entry, 4+ in the second. Neither has a legacy value or misses fields; the first belongs to the current armies (Radagast's Alliance, White Council, Five Armies) and the second only to Thorin's Company, so the first is kept."),
    ('units', 'Rosie Cotton'): ('firstWins', 1, "Re-reviewed 2026-10-22 with the user's heuristic (an older entry lacks Intelligence, carries a 'legacy' value or misses fields; otherwise the first wins): the first entry has Fearful, the second does not; both are complete and not legacy. First kept (no evidence against it)."),
    ('units', 'Sauron, the Necromancer'): ('firstWins', 1, 'The second entry has no unit type or base size.'),
    ('units', 'Thrain the Broken'): ('firstWins', 1, "Re-reviewed 2026-10-22 with the user's heuristic (an older entry lacks Intelligence, carries a 'legacy' value or misses fields; otherwise the first wins): the first entry carries a Hand weapon, the second a Dagger and lacks race, unit type and base size (incomplete). First kept."),
    ('gear', 'Anduril, Flame of the West'): ('equivalent', None, 'Same item, only whitespace differs.'),
    ('gear', 'The Ring of Durin'): ('equivalent', None, 'Same effect; the first entry only adds that it is Unique wargear.'),
    ('gear', 'The Oakenshield'): ('firstWins', 1, "Re-reviewed 2026-10-22 with the user's heuristic (an older entry lacks Intelligence, carries a 'legacy' value or misses fields; otherwise the first wins): different wording of the shielding bonus; only the first carries the +1 Defence (modifiesStats) and the second has no stat data. First kept."),
    ('gear', 'Fate'): ('firstWins', 1, "The first entry is the stat point the Heroic Tier / army rules add; the second is the Witch-king's text and is not an item."),
    ('gear', 'Might'): ('firstWins', 1, "The first entry is the stat point the Heroic Tier / army rules add; the second is the Witch-king's text and is not an item."),
    ('gear', 'Will'): ('firstWins', 1, "The first entry is the stat point the Heroic Tier / army rules add; the second is the Witch-king's text and is not an item."),
    ('keywords', 'Area Effect'): ('equivalent', None, 'Same rule, only typos and capitalisation differ (the second has the Active tag).'),
    ('keywords', 'It has not yet Awoken'): ('equivalent', None, 'Same rule, different wording.'),
    ('keywords', 'Whips of the Master'): ('equivalent', None, 'Same rule; the first entry only adds flavour text.'),
    ('keywords', 'Crushing Strength'): ('firstWins', 1, "Re-reviewed 2026-10-22 with the user's heuristic (an older entry lacks Intelligence, carries a 'legacy' value or misses fields; otherwise the first wins): Grimbeorn's rule in the first, Beorn's in the second (same effect); the Battle Companies Beorning Hero is Grimbeorn. First kept."),
    ('keywords', 'Drums in the Deep'): ('firstWins', 1, 'The first is the current Drum rule; the second adds older army-building text about the Drum taking up two spaces.'),
    ('keywords', 'King Under the Mountain'): ('firstWins', 1, "Two wordings of Dain's rule (the first says Erebor Dwarf Hero models, the second Erebor Dwarf models including Heroes); the first is kept."),
    ('keywords', 'Leader of the Beornings'): ('firstWins', 1, "Re-reviewed 2026-10-22 with the user's heuristic (an older entry lacks Intelligence, carries a 'legacy' value or misses fields; otherwise the first wins): the second still writes 'Stand Fast!' with the old exclamation mark and adds Bears; the first uses the current wording. First kept."),
    ('keywords', 'One of Nine'): ('firstWins', 1, "Re-reviewed 2026-10-22 with the user's heuristic (an older entry lacks Intelligence, carries a 'legacy' value or misses fields; otherwise the first wins): the first names the Nazgul of Dol Guldur (the newer unit list); the second only the Ringwraiths and named Wraiths and adds the Ring invisibility sentence. First kept."),
    ('keywords', 'Take up the Drum'): ('firstWins', 1, 'Re-reviewed 2026-10-22 with the user\'s heuristic (an older entry lacks Intelligence, carries a \'legacy\' value or misses fields; otherwise the first wins): the first is Active, applies to a Warrior within 1" and carries the slain Drummer\'s wargear; the second is Passive and says any unengaged Warrior. First kept.'),
    ('keywords', 'Brawler'): ('firstWins', 1, "Re-reviewed 2026-10-22 with the user's heuristic (an older entry lacks Intelligence, carries a 'legacy' value or misses fields; otherwise the first wins): Sid's rule in two versions; the first (re-roll natural 1s when outnumbered plus never Unarmed) is the fuller one, the second only the Unarmed part. First kept."),
    ("gear", "Axe of Lossarnach"): ("equivalent", None, "Same weapon (spear or two-handed weapon)."),
    ("gear", "Easterling Halberd"): ("equivalent", None, "Identical."),
    ("units", "Galadhrim Knight"): ("firstWins", 1, "The second profile is incomplete (no factions, race, unit type or base size)."),
    ("units", "Guard of the Galadhrim Court"): ("firstWins", 1, "The second profile is incomplete (no factions, race, unit type or base size)."),
    ("units", "Ranger of the North"): ("equivalent", None, "The warriors entry is the hero profile used as a warrior (countAsProperty: heroes): same stats."),
}


# ---------------------------------------------------------------------------------------------------------------------
# Gold (rulebook "Gold", Company Creation) and the other sources of re-rolls / aftermath bonuses.
# ---------------------------------------------------------------------------------------------------------------------
GOLD = {
    "startingGoldPath": "data.companies[].startingCompany.gold",
    "ignoresInfluenceCost": True,
    "sources": [
        {"key": "startingCompany", "path": "data.companies[].startingCompany.gold"},
        {"key": "scenarioVictoryReward", "path": None, "note": "some scenarios give Gold to the victor; no scenario in the data lists an amount"},
        {"key": "organizerGrant", "path": None, "note": "the Campaign Organiser may grant Gold in a narrative scenario that is not in the rulebook"},
    ],
    "when": ["companyCreation", "campaign"],
    "carryOver": True,
    "spendableOn": [
        {"kind": "equipment", "price": "gold", "priceIs": "rating.warrior | rating.hero (the model's kind)", "path": "data.equipment.items[].gold"},
        {"kind": "creature", "price": "gold", "priceIs": "points", "path": "data.creatures.items[].gold"},
        {"kind": "wanderer", "price": "gold", "priceIs": "points", "path": "data.wanderers.items[].gold"},
        {"kind": "wargear", "price": "gold", "priceIs": "rating.warrior | rating.hero", "path": "data.wargear.items[].gold",
         "condition": "warriorsStayLegalToAProfileOption"},
    ],
    "cannotBuy": ["units", "influence"],
    "stillApplies": ["equipmentSlots", "wargearLimits", "rare", "restrictions"],
    "startingTotal": {"target": 60, "counts": ["rating", "gold"], "excludes": ["might", "will", "fate"],
                      "source": "campaign.company.startingPoints", "kind": "guideline for custom companies"},
    "confirmedByUser": ["sources", "when", "carryOver", "spendableOn[kind=wargear].priceIs", "heroesPayHeroRating",
                        "stillApplies", "heroesMayTakeWargearOutsideProfile", "startingTotalIsGuideline"],
    "defaultNotConfirmed": ["cannotBuy"],
    "heroesPayHeroRating": True,
    "heroesMayTakeWargearOutsideProfile": True,
    "text": "Gold comes from some companies' starting company, from victory rewards of some scenarios and from the organiser's narrative scenarios; it carries over and can be spent in the Armoury later. Spending at company creation:  an Equipment, Creature or Wanderer costs its initial Rating (point value) in Gold "
            "and no Influence; Gold can alternatively buy extra Wargear for starting models as long as Warriors stay legal to one of their "
            "profile's Options. The starting company is meant to add up to 60 points of Rating plus Gold (before Might / Will / Fate).",
}

AFTERMATH_EFFECTS = [
    {"key": "rerollBonus", "source": "campaign.againstTheOdds.bonuses[key=reroll]", "who": "lowerRatedCompany", "kind": "rerolls", "count": 2,
     "usableIn": ["game", "warsAftermath"], "text": "Two re-rolls, each for a single die or a single result of the War's Aftermath."},
    {"key": "influenceBonus", "source": "campaign.againstTheOdds.bonuses[key=influence]", "who": "lowerRatedCompany", "kind": "influence",
     "gains": {"win": 2, "other": 1}, "text": "+2 Influence for a Victory, +1 for a Draw or Defeat."},
    {"key": "experienceBonus", "source": "campaign.againstTheOdds.bonuses[key=experience]", "who": "lowerRatedCompany", "kind": "experiencePerModel",
     "gains": {"win": 2, "other": 1}, "text": "+2 Experience per model for a Victory, +1 for a Draw or Defeat."},
    # Equipment that acts on the injury rolls of the War's Aftermath (step 1, before the rolls). `source` is the item;
    # the item points back with `aftermathEffect`. The player elects to use the item (`choice`), which removes it from the Equipment.
    {"key": "healingHerbs", "source": "data.equipment.items[name=Healing Herbs]", "kind": "injuryRollModifier", "who": "allCompanyModels",
     "activatedBy": {"carrier": "hero", "requires": ["participated", "notCasualty"], "fledModelIs": "notCasualty"},
     "when": "beforeInjuryRolls", "choice": "player", "modifier": 1, "applies": "eachInjuryRoll", "stackable": False, "usesPerAftermath": 1,
     "consumes": "oneItem", "clamp": [2, 12], "includesHeroReroll": True,
     "defaultNotConfirmed": ["activatedBy.fledModelIs", "clamp", "includesHeroReroll", "usesPerAftermath"],
     "text": "One Hero that took part and was not removed as a casualty may use Healing Herbs (the item is removed): every Injury roll the company makes at the end of the game gets +1. Not cumulative."},
    {"key": "wondrousCram", "source": "data.equipment.items[name=Wondrous Cram]", "kind": "injuryRollReplacement", "who": "carrier",
     "activatedBy": {"carrier": "anyModel", "requires": ["participated", "casualty"], "fledModelIs": "notCasualty"},
     "when": "beforeInjuryRolls", "choice": "player", "noRoll": True, "result": "fullRecovery", "includesHealOneInjury": True,
     "consumes": "thisItem",
     "defaultNotConfirmed": ["activatedBy.fledModelIs", "includesHealOneInjury"],
     "text": "A model removed as a casualty that took part may use its Wondrous Cram (the item is removed): it does not roll on the Injury chart and counts as having rolled the Full Recovery result."},
    {"key": "rendezvous", "scenario": "Chance Encounter", "who": "victor", "kind": "reinforcementRollModifier", "range": [-1, 1], "rolls": 1,
     "when": "firstReinforcementRollThisAftermath",
     "text": "The victorious company may apply +1 / -1 to its first Reinforcement roll in the War's Aftermath right after this match.",
     "note": "The scenario prints 'in the subsequent War's Aftermath'; the user confirmed (2026-10-21) that it is the aftermath right after the match, not the next match's."},
    {"key": "toTheVictorTheSpoils", "scenario": "Scavenge the Battlefield", "who": "companyThatScoresAllObjectives", "kind": "freeEquipment",
     "maxInfluenceValue": 2, "text": "A company that scores all 6 Objective Markers may acquire a free piece of Equipment worth up to 2 Influence."},
    {"key": "survivalOfTheFittest", "scenario": "Rescue", "who": "eachCompanyWhoseLeaderSurvives", "kind": "experience", "amount": 1, "to": "leader",
     "text": "Each Leader that is not removed from the board earns one additional Experience Point."},
    {"key": "cargo", "scenario": "Baggage Train", "who": "companyThatScoresAllCargo", "kind": "influence", "amount": 1,
     "text": "A company that scores all Cargo earns 1 bonus Influence."},
]


# ---------------------------------------------------------------------------------------------------------------------
# Recruitment core rules (decided by the user, explicit so the engine never guesses) and the paths that are not heroic paths.
# ---------------------------------------------------------------------------------------------------------------------
RECRUITMENT = {
    "adjust": {"direction": "up", "currency": "influence", "costPerPoint": 1, "maxPerAttempt": 3, "appliesTo": "die",
               "sharedAcross": ["reinforcements", "specialChart"], "payForEveryPointToReachChoice": True, "lowering": "never",
               "text": "Influence raises the DIE, one point per Influence, at most 3 per recruiting attempt across the reinforcement chart and the "
                       "special chart together. A 5 raised by 1 becomes a 6 and leads on to the special chart. If the rolled result and the next ones "
                       "cannot be recruited, the player still pays for every point needed to reach the result they want. Lowering is never paid."},
    "rare": {"meaning": "atMostNInRoster", "countsUnitsOfThatType": "inRoster", "injuredUnitsCount": True, "deadUnitsCount": False,
             "promotedUnitsCount": False, "unitIdentity": "unit + selectedOptions (a profile)", "confirmedByUser": True,
             "whenExhausted": "lowerResult",
             "appliesTo": ["reinforcements.table[].result.rare", "specialChart.table[].result.rare", "specialUnits[].rare",
                           "table[].result.options[].rare (type unitChoice)"],
             "listedIn": "data.companies[].rareUnits",
             "text": "A unit marked Rare N can be in the company at most N times at the same time: the units of that profile currently in the roster. "
                     "Injured units count; units that died, left, or were promoted into another profile no longer count. When a roll gives a rare unit "
                     "the company cannot have, it cannot be recruited and the player takes a lower result (lowerResult, reason rareExhausted)."},
    "unitChoice": {"resultType": "unitChoice", "shape": {"type": "unitChoice", "options": [{"unit": "exact unit name", "label": "alias label (optional)",
                                                                                         "selectedOptions": ["optional"], "rare": 1}]},
                   "semantics": "The player picks ONE option whose own rare limit is not exhausted (the company has fewer than `rare` of that profile). "
                                "If every option is exhausted the result cannot be recruited and the normal lowerResult rule applies on the same chart.",
                   "text": "A chart row that offers a choice among named units, each with its own Rare limit."},
    "lowerResult": {"choice": "free", "range": "anyLowerResult", "chartScope": "sameChart",
                    "allowedOnlyWhen": ["rareExhausted", "overMaxModels", "overKeywordLimit", "overWargearLimit", "raceBalance", "companyRule"],
                    "otherwise": "notAllowed", "cost": 0,
                    "text": "When the rolled result cannot be recruited (rare units used up, over the model maximum, over a keyword or wargear limit, a race "
                            "balance) or a company rule says so, the player may take ANY lower result for free. A lower result may not be picked just because "
                            "the player prefers it. A company effect (recruitFallback) can widen the scope, e.g. to both charts."},
}

KEYWORDS_OWN_MAKING = [
    {"name": "Whirl of Attacks", "tag": "Active", "source": "Path of Their Own Making",
     "definition": "When rolling a natural 6 for the Duel Roll, this model rolls an additional attack when rolling to Wound. You can not use this rule if the enemy model is trapped."},
    {"name": "Master of Fighting Styles", "source": "Path of Their Own Making",
     "definition": "The Hero has mastered different fighting styles. Before the Duel Roll, the model may use the rules for a two-handed weapon or shielding, even without the equipment."},
    {"name": "I shall pass!", "tag": "Active", "source": "Path of Their Own Making",
     "definition": "If this Hero wins his Duel Roll, after all models have backed away but before making Strikes, the model may make no Strikes and instead move again up to half its Move Value. If it charges an enemy, it fights as normal. This rule can only be used once per turn. (Inspired by the Barge Brutal Power attack.)"},
]

KEYWORD_END_OF_PATH = {"name": "At Path's End", "source": "On Their Own Path",
                       "definition": "This unit earns no Experience Points and cannot level up (no progression, no promotion, no Hero in the Making)."}

EXTRA_PATHS = [
    {"name": "On Their Own Path", "kind": "endOfPath", "selectable": False, "assignedAutomatically": True, "implemented": True,
     "assignedWhen": "onTheirOwnPathUnitRecruited", "progression": [], "maximums": {}, "startingHeroicAction": None,
     "startingStats": None, "givesExperience": False, "canLevelUp": False, "staysWarrior": True, "specialRule": "At Path's End",
     "description": "The end of the road: a unit that is On Their Own Path stays a warrior, earns no XP and never levels up."},
    {"name": "Path of Their Own Making", "kind": "custom", "selectable": False, "assignedAutomatically": True, "implemented": True, "homebrew": True,
     "assignedWhen": "onTheirOwnPathUnitRecruited", "enabledBy": "campaign.options[key=customOnTheirOwnPath]",
     "startsOnPathWhenRecruited": True, "startingStats": {"might": 0, "will": 0, "fate": 1}, "startingHeroicAction": None,
     "givesExperience": True, "canLevelUp": True, "countsAsHero": True, "limitPerCompany": None,
     "dice": "2d6", "diceNote": "the document names no dice; its results run 2 to 12, so the standard two pairs of 2d6 are used",
     "maximums": {"fight": {"maxIncrease": 2}, "shoot": {"maxIncrease": 2}, "strength": {"maxIncrease": 1}, "defence": {"maxIncrease": 1},
                  "attack": {"max": 2}, "wounds": {"max": 2}, "courage": {"maxIncrease": 2}, "intelligence": {"maxIncrease": 2},
                  "might": {"max": 2}, "will": {"max": 2}, "fate": {"max": 2}},
     "source": "community homebrew document (docs.google.com, id 1__3ZeEy7-0eXsTrSWLAmLJ25WSzcGYBE6Yb6uOc0kv0, untested; its author invites changes). "
               "Texts were read through a document reader: check them against the document.",
     "progression": [
         {"roll": 2, "text": "Whirl of Attacks (Active) - When rolling a natural 6 for the Duel Roll, this model rolls an additional attack when rolling to Wound. "
                             "You can not use this rule if the enemy model is trapped.",
          "options": [{"type": "specialRule", "specialRule": "Whirl of Attacks"}]},
         {"roll": 3, "text": "Lead by Example - If this Hero kills an enemy model, he is treated as a banner for the duration of the next turn.",
          "options": [{"type": "specialRule", "specialRule": "Lead by Example",
                       "definition": "If this Hero kills an enemy model, he is treated as a banner for the duration of the next turn.",
                       "note": "defined here because the vanilla rule of the same name belongs to Buhrdur"}]},
         {"roll": 4, "text": "Improve Attacks or Wounds by 1. OR Improve Might, Will, or Fate by 1.",
          "options": [{"type": "improveStat", "stats": ["attack", "wounds"], "amount": 1, "mode": "oneOf"},
                      {"type": "improveStat", "stats": ["might", "will", "fate"], "amount": 1, "mode": "oneOf"}]},
         {"roll": 5, "text": "Improve Courage or Intelligence by 1 (do not take the second result!)",
          "options": [{"type": "improveStat", "stats": ["courage", "intelligence"], "amount": 1, "mode": "oneOf", "rollAgain": False}]},
         {"roll": 6, "text": "Improve Strength or Defence by 1.",
          "options": [{"type": "improveStat", "stats": ["strength", "defence"], "amount": 1, "mode": "oneOf"}]},
         {"roll": 7, "text": "Master of Fighting Styles - The Hero has mastered different fighting styles. Before the Duel Roll, the model may use the rules for a "
                             "two-handed weapon or shielding, even without the equipment. OR Pick an additional Action to add to the Hero's list of Heroic Actions.",
          "options": [{"type": "specialRule", "specialRule": "Master of Fighting Styles"}, {"type": "heroicAction"}]},
         {"roll": 8, "text": "Improve Fight Value or Shoot Value by 1.",
          "options": [{"type": "improveStat", "stats": ["fight", "shoot"], "amount": 1, "mode": "oneOf"}]},
         {"roll": 9, "text": "Improve Might, Will, or Fate by 1.",
          "options": [{"type": "improveStat", "stats": ["might", "will", "fate"], "amount": 1, "mode": "oneOf"}]},
         {"roll": 10, "text": "Choice of Minor Special Rule", "options": [{"type": "minorSpecialRule"}]},
         {"roll": 11, "text": "Master Duelist (Active) - At the start of a Combat involving this Hero, before any Heroic Strikes are resolved, this Hero may increase their "
                              "Fight Value to match that of a single enemy model they are Engaged in Combat with.",
          "options": [{"type": "specialRule", "specialRule": "Master Duelist"}]},
         {"roll": 12, "text": "I shall pass! (Active) - If this Hero wins his Duel Roll, after all models have backed away but before making Strikes, the model may "
                              "make no Strikes and instead move again up to half its Move Value. If it charges an enemy, it fights as normal. This rule can only be "
                              "used once per turn.",
          "options": [{"type": "specialRule", "specialRule": "I shall pass!"}]},
     ],
     "description": "Homebrew replacement for On Their Own Path: the unit starts on the path with 0 Might, 0 Will and 1 Fate when recruited, earns XP and levels up on this table; it counts as a Hero for levelling and stays a hero in name only for rules that need Might, Will or Fate."},
]

CAMPAIGN_OPTIONS = [
    {"key": "heroesDropProfileGear", "scope": "rules", "label": "Heroes may discard any wargear",
     "description": "Heroes may drop wargear that is part of their profile.", "default": False, "implemented": True,
     "source": "campaign.company.text.youDontHaveMySword"},
    {"key": "customOnTheirOwnPath", "scope": "rules", "label": "Path of Their Own Making (homebrew)",
     "description": "Units that are On Their Own Path level up on a homebrew path instead of staying warriors.",
     "default": False, "implemented": True, "swaps": {"path": "Path of Their Own Making", "replaces": "On Their Own Path"}},
    {"key": "shakeTheTable", "scope": "app", "label": "Shake the table",
     "description": "Throw the 3D dice with physics: one shake per roll.", "default": False, "implemented": True},
    {"key": "manualRolls", "scope": "app", "label": "Players may enter their own rolls",
     "description": "Let players type in the result of a roll instead of rolling in the app.", "default": False, "implemented": True},
]


def match_input():
    """What the players enter after a match (matches are played offline); everything the War's Aftermath needs.

    The per-model experience fields are generated from EXPERIENCE so the two never drift apart."""
    per_model = [
        {"key": "outcome", "type": "enum", "options": ["survived", "casualty", "fled"], "default": "survived", "feeds": ["injuries"],
         "text": "A model removed as a casualty rolls on its Injury chart; a model that fled the board does not (campaign.injuries.applies). 'survived' covers every model still on the board at the end."},
        {"key": "participated", "type": "flag", "default": True, "feeds": ["experience"], "experience": "participated",
         "text": "Every model that took part in the scenario; a model missing the game (Injured) is not in the match roster."},
    ]
    per_model.append({"key": "usesItem", "type": "enum", "options": ["healingHerbs", "wondrousCram"], "optional": True, "feeds": ["injuries"],
                      "source": "campaign.aftermathEffects[kind=injuryRollModifier|injuryRollReplacement]",
                      "text": "The item the model elects to use before the injury rolls (Healing Herbs: a Hero that was not a casualty; Wondrous Cram: a casualty); it is removed from the Equipment."})
    for g in EXPERIENCE["gains"]:
        if g["key"] == "participated":
            continue
        f = {"key": g["key"], "type": "count" if g.get("repeatable") else "flag", "default": 0 if g.get("repeatable") else False,
             "feeds": ["experience"], "experience": g["key"], "text": g["label"]}
        if g["key"] == "mvp":
            f.update({"maxPerCompany": 1, "declaredBy": "opponent", "inferred": ["maxPerCompany"]})
        per_model.append(f)
    return {
        "text": "Per match the players enter, for each company: the roles, the Against the Odds bonuses, the Victory Points (from the scenario's scoringObjectives), "
                "whether the Battle Company fled, the scenario reward flags and, for every model, what happened to it and which experience gains it earned. "
                "The winner is the company with the most Victory Points; equal Victory Points are a draw (stated in the Objectives of most scenarios).",
        "result": {"decidedBy": "victoryPoints", "outcomes": ["win", "draw", "loss"], "tie": "draw", "source": "data.scenarioDetails[].scoringObjectives"},
        "perCompany": [
            {"key": "role", "type": "enum", "options": ["attacker", "defender"], "when": "scenario.roles", "text": "Attacker or Defender, for the scenarios that have roles."},
            {"key": "againstTheOdds", "type": "multiSelect", "options": "campaign.againstTheOdds.bonuses[].key", "who": "lowerRatedCompany", "feeds": ["influence", "experience", "rerolls"],
             "text": "The Against the Odds bonuses taken before setting up; they only apply to the lower rated company (campaign.aftermathEffects)."},
            {"key": "victoryPoints", "type": "integer", "min": 0, "computedFrom": "scenario.scoringObjectives", "feeds": ["influence"],
             "text": "The total of the Victory Points scored; entered as a total or built up objective by objective."},
            {"key": "scenarioObjectives", "type": "scoringObjectives", "from": "scenario.scoringObjectives",
             "text": "One entry per scoring objective (tier, count or per-item tiers); 'side' limits an objective to the Attacker or the Defender."},
            {"key": "rewardFlags", "type": "flags", "from": "scenario.rewards[].condition", "feeds": ["rewards"],
             "text": "The tickable conditions of the scenario rewards (companyFlag, modelFlag); the conditions victory and leaderNotRemoved are derived from the result and the model outcomes."},
            {"key": "toolKits", "type": "multiSelect", "options": "campaign.againstTheOdds.bonuses[key=toolKit].kits[].name", "when": "againstTheOdds contains toolKit", "feeds": ["itemUse"],
             "text": "The Kits taken (one Tool Kit bonus each, no Kit twice); their items are then assigned to models."},
            {"key": "temporaryWanderer", "type": "select", "options": "data.wanderers.items[].name", "when": "againstTheOdds contains wanderer", "feeds": ["breakPoint"],
             "text": "The Wanderer that joins the company for this match only."},
            {"key": "ambushScenario", "type": "select", "options": "scenarios", "when": "againstTheOdds contains ambush",
             "text": "The scenario chosen with Ambush!, replacing the rolled one."},
            {"key": "startingModels", "type": "integer", "min": 1, "computed": True, "feeds": ["breakPoint"], "source": "campaign.breakPoint.startingModels",
             "text": "Models in the starting force (roster of the match, creatures, temporary Wanderer); the Break Point follows from it."},
            {"key": "brokenAtTurn", "type": "integer", "min": 1, "optional": True, "feeds": ["breakPoint"], "source": "campaign.breakPoint",
             "text": "The turn at whose start the company became Broken, if it did."},
            {"key": "fledBroken", "type": "flag", "default": False, "feeds": ["influence"], "source": "campaign.influence.allIsLost",
             "text": "All is Lost: the Broken company fled the battlefield; the opponent claims victory and full scenario rewards, the fleeing company keeps the participation Influence and Experience and its Against the Odds Influence and Experience bonuses, but no Win/Draw Influence and no scenario rewards."},
        ],
        "perModel": per_model,
        "aftermath": {"order": ["injuries", "experience", "progression", "influence"], "source": "campaign.progression.timing",
                      "rewards": "campaign.aftermathEffects[] whose key is listed in the scenario's rewards[].effect, applied when its condition holds"},
        "gold": {"scenarioRewards": False, "text": "No scenario of the rulebook rewards Gold; Gold comes from the starting company and from the organiser (campaign.gold.sources)."},
    }


# Printing errors of the Battle Companies document, corrected when it is read (the vanilla option is "Battlin' Brandybuck").
# "High Elf Warrior" is the 2018 name of the Rivendell Warrior (same profile, the only Elf of the Last Alliance list): the company
# lists the Rivendell Warrior, with no alias card, as the document intends.
# "Giant Mirkwood Spider" (Cirith Ungol) is the same document's "Mirkwood Giant Spider" printed in the other lists (Dol Guldur,
# Spider Queen's brood): word order slip, the vanilla key is Mirkwood Giant Spider.
# Applied longest first. The 2018 names and abbreviations below are replaced by the vanilla unit the document means (owner
# decision 2026-10-26, see docs/definitions.md "Unit names that differ from vanilla"): no alias card.
SOURCE_TYPOS = {"Brandbybuck": "Brandybuck", "High Elf Warrior": "Rivendell Warrior",
                "Giant Mirkwood Spider": "Mirkwood Giant Spider",
                "Rhûnish War Drakes": "War Drakes of Rhun", "Rhûnish War Drake": "War Drake of Rhun",
                "Grimhammer": "Grim Hammer Warrior", "Warrior of Erebor": "Erebor Dwarf Warrior",
                "Iron Hills Dwarf": "Iron Hills Warrior", "Mirkwood Cavalry": "Mirkwood Elf Knight",
                "Abrakhân Guard": "Abrakhan Merchant Guard"}
