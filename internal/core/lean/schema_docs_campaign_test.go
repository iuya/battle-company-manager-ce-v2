package lean

// The descriptions of the campaign tables (see schema_docs_test.go for how they reach the schema and the reference). One file per group
// of tables keeps the diff of a table with its type; init merges them into typeDocs and fieldDocs.
func init() {
	for k, v := range campaignTypeDocs {
		typeDocs[k] = v
	}
	for k, v := range campaignFieldDocs {
		fieldDocs[k] = v
	}
}

var campaignTypeDocs = map[string]string{
	"Injuries":           "The injury charts, the die they are rolled on and the treatments. The chart rows and the treatments are read by the engine; the prose keys (applies, deathOfAHero, healing, retirement, rollModifier) are rule text for the players.",
	"InjuryHealingText":  "The rulebook text of how injuries are healed. Prose; the app does not read it.",
	"InjuryRetirement":   "What happens to a hero that suffers a persistent injury for the retireAtCount-th time. Prose; the count is the row's retireAtCount.",
	"InjuryRollModifier": "The rulebook text about modifiers of the injury roll (Healing Herbs). Prose; the modifier itself is a campaign.aftermathEffects entry.",
	"InjuryRow":          "A row of an injury chart. The engine reads effects (remove, missNextGame, fullRecovery) to decide what the result does, and persistent, retireAtCount, statChanges, disables and grantsAtCount for a hero's lasting injuries.",
	"InjuryChange":       "A characteristic an injury changes: a gain on a row that is not a persistent injury, a loss (per count) on one.",
	"DiceGrant":          "Experience or Influence rolled on a die.",
	"InjuryReroll":       "The optional second roll of T'is Just a Scratch.",
	"InjuryDisable":      "What a lasting injury takes from a hero: the benefit of gear of some classes or by name, or an ability.",
	"CountGrant":         "A special rule a hero gets once it has an injury a given number of times.",
	"InjuryHeals":        "What a healing result removes: injuries of the listed keys, chosen by the player.",
	"InjuryTreatment":    "How Influence treats injuries (Battle Companies, Step 3): the hero methods, the warrior's and what removing an injury does.",
	"HeroTreatment":      "One way to treat a hero: an attempt on a die (attempt) or sending the hero to the healers (healers).",
	"TreatRoll":          "The die of a treatment attempt: the total must reach the target, and extra Influence raises it.",
	"TreatModifier":      "How extra Influence raises a treatment roll, like a Might point.",
	"TreatFailure":       "What a failed attempt costs.",
	"WarriorTreatment":   "The treatment of a warrior: the cost in Influence of removing the Injured condition.",
	"TreatmentRemoval":   "What removing an injury does to the hero's counts.",
}

var campaignFieldDocs = map[string]string{
	// Injuries
	"Injuries.roll":         "The injury die, such as 2d6. Both charts must cover its totals exactly once.",
	"Injuries.applies":      "Who rolls on the charts. Prose shown to the players.",
	"Injuries.deathOfAHero": "What happens when a Leader is slain. Prose shown to the players.",
	"Injuries.healing":      "How injuries are healed. Prose shown to the players.",
	"Injuries.warrior":      "The warrior injury chart.",
	"Injuries.hero":         "The hero injury chart.",
	"Injuries.retirement":   "Forced retirement. Prose; the count is the row's retireAtCount.",
	"Injuries.rollModifier": "Modifiers of the injury roll. Prose; the modifier is a campaign.aftermathEffects entry.",
	"Injuries.treatment":    "How Influence treats injuries.",

	"InjuryHealingText.hero":    "How a hero is healed.",
	"InjuryHealingText.warrior": "How a warrior is healed.",
	"InjuryRetirement.text":     "The rule in words.",
	"InjuryRetirement.result":   "The key of the chart row a forced retirement counts as (dead).",

	"InjuryRollModifier.text":            "The rule in words.",
	"InjuryRollModifier.appliesTo":       "What the modifier is added to: total2d6, the total of the injury dice.",
	"InjuryRollModifier.clamp":           "[low, high]: the modified total is kept inside these bounds.",
	"InjuryRollModifier.includesRerolls": "true: the modifier also applies to a re-roll.",
	"InjuryRollModifier.replacedBy":      "Where the modifier is defined, for the reader of the file.",

	// InjuryRow
	"InjuryRow.roll":              "The roll or range the row covers.",
	"InjuryRow.key":               "The key the engine switches on (dead, injured, fullRecovery...). A row with no effects is read by its key: dead, injured, else a full recovery.",
	"InjuryRow.name":              "The name of the result.",
	"InjuryRow.text":              "The rule text of the result.",
	"InjuryRow.effects":           "What the result does. remove (the model dies), missNextGame and fullRecovery decide the result; the others (healOneInjury, reduceMovement, noShield...) label the effect for the reader and are done by the other keys of the row.",
	"InjuryRow.grantsExperience":  "Experience rolled on a die when the result comes up.",
	"InjuryRow.grantsInfluence":   "Influence rolled on a die for the company when the result comes up.",
	"InjuryRow.experience":        "The die of grantsExperience in the rulebook's words (D3). The app reads grantsExperience.",
	"InjuryRow.influence":         "The die of grantsInfluence in the rulebook's words (D6). The app reads grantsInfluence.",
	"InjuryRow.statChanges":       "The characteristics the result changes (a gain on a row that is not a persistent injury, a loss on one).",
	"InjuryRow.stat":              "The characteristics changed, as {characteristic: change}, for the reader of the file. The app reads statChanges.",
	"InjuryRow.stat_note":         "A note on stat, for the reader of the file.",
	"InjuryRow.cancelsMissedGame": "true: the result cancels the missed next game.",
	"InjuryRow.persistent":        "true: the hero keeps the injury and counts how often it suffered it (arm wound, leg wound, broken honour).",
	"InjuryRow.retireAtCount":     "A persistent injury forces the hero to retire at this count, as the Dead result.",
	"InjuryRow.healable":          "true: a treatment can remove the injury.",
	"InjuryRow.healableBy":        "The treatment methods that can remove the injury (a key of campaign.injuries.treatment.hero), for a status such as T'is Just a Scratch.",
	"InjuryRow.clearsAfterGames":  "The injury ends after this many games.",
	"InjuryRow.optionalReroll":    "true: the player may roll again on the chart instead of taking the result (T'is Just a Scratch).",
	"InjuryRow.reroll":            "How the re-roll of an optionalReroll result works.",
	"InjuryRow.disables":          "What a lasting injury takes from the hero.",
	"InjuryRow.grantsAtCount":     "Special rules the hero gets from a count of the injury on: {\"2\": {specialRule}}.",
	"InjuryRow.heals":             "The injuries a healing result removes.",

	"InjuryChange.stat":              "The characteristic changed.",
	"InjuryChange.amount":            "The change, a positive gain or a negative loss.",
	"InjuryChange.per":               "count: the amount is applied once per time the hero has the injury.",
	"InjuryChange.minimum":           "The lowest value the characteristic may reach; null for no floor. Informational; the app does not read it.",
	"InjuryChange.permanent":         "true: the change stays for good. Informational; a row that is not persistent always keeps its gain.",
	"InjuryChange.capAtPathMaximum":  "true: the gain stops at the maximum of the hero's path. Informational; the app does not read it.",
	"InjuryChange.raisesPathMaximum": "true: the gain also raises the path's maximum. Informational; the app does not read it.",
	"DiceGrant.dice":                 "The die rolled.",

	"InjuryReroll.dice":                       "The dice rolled again.",
	"InjuryReroll.optional":                   "true: the player chooses whether to roll again.",
	"InjuryReroll.secondResultStands":         "true: the second result applies even if it is worse than the first.",
	"InjuryReroll.ifScratchAgainMissNextGame": "true: a second result of the same row means the hero misses the next game.",

	"InjuryDisable.kind":     "gearClass (items of the listed classes give no benefit), gearItem (the named items give none) or ability (the hero loses the ability).",
	"InjuryDisable.classes":  "The gear classes, for kind gearClass.",
	"InjuryDisable.names":    "The item names, for kind gearItem.",
	"InjuryDisable.key":      "The ability, for kind ability: standFast or heroicActionsOnCompany.",
	"CountGrant.specialRule": "The special rule granted.",

	"InjuryHeals.choice": "Who chooses the injury healed: player.",
	"InjuryHeals.count":  "How many injuries are healed.",
	"InjuryHeals.keys":   "The keys of the hero injuries that can be healed.",

	// Treatment
	"InjuryTreatment.currency":      "What a treatment costs: influence. Informational; the app spends Influence.",
	"InjuryTreatment.phase":         "When a treatment is made: companyManagement. Informational.",
	"InjuryTreatment.hero":          "The hero treatments. The engine knows attempt and healers.",
	"InjuryTreatment.warrior":       "The warrior treatment.",
	"InjuryTreatment.removalEffect": "What removing an injury does to the hero's counts.",

	"HeroTreatment.key":             "attempt (a roll to remove an injury) or healers (no roll: the hero misses the next game).",
	"HeroTreatment.text":            "The rule in words.",
	"HeroTreatment.cost":            "The Influence the method costs.",
	"HeroTreatment.roll":            "The roll of an attempt; null for a method with no roll.",
	"HeroTreatment.healable":        "The injuries (keys of the hero chart) the method can remove.",
	"HeroTreatment.maxAttempts":     "The most attempts allowed; null for no limit. Informational; the app puts no limit.",
	"HeroTreatment.missNextGame":    "true: the hero misses the next game.",
	"HeroTreatment.removesInjuries": "How many injuries the method removes.",
	"HeroTreatment.onFailure":       "What a failed attempt costs.",

	"TreatRoll.dice":                     "The dice rolled.",
	"TreatRoll.compare":                  "gte: the total must be at least the target.",
	"TreatRoll.target":                   "The total the roll must reach; a total the dice can show.",
	"TreatRoll.modifier":                 "How extra Influence raises the roll.",
	"TreatRoll.naturalMaxIsAlwaysEnough": "true: the highest roll always succeeds. Informational; the app does not read it.",

	"TreatModifier.currency":  "influence.",
	"TreatModifier.direction": "up: Influence raises the roll.",
	"TreatModifier.perPoint":  "The roll gained per extra Influence point.",
	"TreatModifier.ceiling":   "The raised roll never goes above this; 0 for no ceiling. At least the target.",
	"TreatModifier.maxPoints": "The most extra points allowed; null for no limit. Informational; the ceiling limits the roll.",
	"TreatModifier.like":      "What the modifier resembles in the rulebook (a Might point).",

	"TreatFailure.costKept": "true: the Influence is lost on a failure.",
	"TreatFailure.effect":   "none: a failure has no other effect.",

	"WarriorTreatment.key":      "A name for the treatment.",
	"WarriorTreatment.text":     "The rule in words.",
	"WarriorTreatment.cost":     "The Influence the treatment costs.",
	"WarriorTreatment.roll":     "null: a warrior is treated without a roll.",
	"WarriorTreatment.removes":  "injured: what the treatment removes.",
	"WarriorTreatment.requires": "injured: what the warrior must have.",

	"TreatmentRemoval.choice":         "Who chooses the injury removed: player.",
	"TreatmentRemoval.countDelta":     "The change of the injury's count (-1).",
	"TreatmentRemoval.derivedEffects": "recomputeFromCounts: the effects of the injuries follow from the counts.",
}
