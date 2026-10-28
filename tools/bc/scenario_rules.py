"""Scenario objectives, rewards and roles as data (the part the rule engine would otherwise have to interpret).

The Victory Point bullets of every scenario are parsed by `scenarios.py` (text + tiers with their points). This module adds, per
bullet, what the player is asked and how it scores (`scoringObjectives`), the rewards that follow the match (`rewards`) and the
Attacker / Defender roles. The table below is hand-transcribed from the rulebook: `enrich` asserts that it lines up with the parsed
bullets, so a change of the source surfaces as an error instead of a silent shift.

Objective kinds (`kind`):
  tiers      pick the best tier reached, or none (the "instead score" sentences are alternatives, not additions)
  each       a count, `pointsEach` Victory Points per `item`, capped by `maxCount` / `maxPoints`
  eachTiers  `count` items (Objective Markers, Cargo, turns), each one scores the best tier it reached
"""
import re

# Standard bullets that recur in many scenarios keep the same key.
LEADER, BROKEN = "enemyLeader", "enemyBroken"
NO_COMMAND, COMMAND = "noCommandSupport", "commandSupport"
NO_COMPANY = "noCompanySupport"

ATT_DEF = [{"key": "attacker", "name": "Attacker"}, {"key": "defender", "name": "Defender"}]


def T(key, **kw):
    return {"key": key, "kind": "tiers", **kw}


def E(key, item, **kw):
    return {"key": key, "kind": "each", "item": item, **kw}


def ET(key, item, count, **kw):
    return {"key": key, "kind": "eachTiers", "item": item, "count": count, **kw}


# name -> roles, objectives (one per parsed Victory Point bullet, in order), cut (reward / special rule text glued to a bullet by the
# source layout: {bullet index: marker}; the text from the marker on is removed from the bullet and published as reward / rule).
SCENARIOS = {
    "Seize Ground": {"objectives": [ET("objectiveMarkers", "Objective Marker", 5), T(LEADER), T(BROKEN)]},
    "Chance Encounter": {
        "objectives": [T(LEADER), T(BROKEN), T(NO_COMPANY), T(COMMAND), T("reducedTo25"), E("enemyHeroesSlain", "enemy Hero removed as a casualty", maxCount=3, maxPoints=3)],
        "cut": {5: "Rendezvous - "},
        "rewards": [{"effect": "rendezvous", "name": "Rendezvous", "condition": {"kind": "victory"}}],
    },
    "Secure the Area": {
        "objectives": [T("objective"), T(LEADER), T(BROKEN), T(NO_COMMAND)],
        "cut": {3: "Vantage Point - "},
        "rewards": [{"effect": "vantagePoint", "name": "Vantage Point", "condition": {"kind": "victory"}}],
    },
    "Plunder the Supplies": {
        "objectives": [E("suppliesDestroyed", "enemy Supply Marker destroyed", maxCount=3), E("suppliesContested", "enemy Supply Marker not destroyed where you have more models within 3\"", maxCount=3),
                       T("moreSuppliesDestroyed"), T(LEADER), T(BROKEN), T(NO_COMMAND), T(COMMAND)],
    },
    "Fog of War": {
        "objectives": [T("ownHeroSurvives", secret=True), T("enemyHeroHarmed", secret=True), T("terrainPiece", secret=True), T(LEADER), T(BROKEN)],
    },
    "Show of Strength": {"objectives": [T("leaderKills"), T("leaderDuel"), T(BROKEN)]},
    "Scavenge the Battlefield": {
        "objectives": [ET("objectiveMarkers", "Objective Marker", 6), T(LEADER), T(BROKEN), T(COMMAND)],
        "rewards": [{"effect": "toTheVictorTheSpoils", "name": "To the Victor Goes the Spoils", "source": {"rule": "Objectives", "marker": "To the Victor Goes the Spoils", "dropMarker": True},
                     "condition": {"kind": "companyFlag", "key": "allObjectiveMarkers", "label": "Scored all 6 Objective Markers"}}],
    },
    "Relic of Ages Past": {
        "objectives": [T("relic"), E("decoysRevealed", "Objective Marker revealed to be a decoy", maxCount=5), T(BROKEN), T(LEADER), T(COMMAND)],
        "rewards": [{"effect": "aFinalBlessing", "name": "A Final Blessing", "source": {"rule": "A Final Blessing"},
                     "condition": {"kind": "modelFlag", "key": "possessesRelic", "label": "In possession of the Relic at the end of the game"}}],
    },
    "Hoard of Treasures": {"objectives": [E("artefacts", "artefact in the possession of a friendly model", maxCount=6), T(LEADER), T(BROKEN), T("modelsInEnemyZone")]},
    "Sites of Power": {
        "objectives": [ET("objectiveMarkers", "Objective Marker", 4), T(LEADER), T(BROKEN), T("highestRatedModel")],
        "cut": {3: "Strange Aura:"},
    },
    "Rescue": {
        "objectives": [T(LEADER), T(BROKEN), T("noCompanyStandard"), T(COMMAND), T("secondaryForceHero"), E("heroesAtCentre", "of your Hero models within 3\" of the centre of the board", maxPoints=3)],
        "cut": {5: "Survival of the Fittest - "},
        "rewards": [{"effect": "survivalOfTheFittest", "name": "Survival of the Fittest", "condition": {"kind": "leaderNotRemoved"}}],
    },
    "Stand Your Ground": {
        "roles": ATT_DEF,
        "objectives": [ET("holdTheHill", "turn after the first", 7), T(LEADER), T(BROKEN), T(NO_COMMAND)],
    },
    "Defend the Gate": {"roles": ATT_DEF, "objectives": [T("gate"), T(LEADER), T(BROKEN), T(NO_COMMAND)]},
    "Hold the Line": {
        "roles": ATT_DEF,
        "objectives": [T("escape", side="attacker"), E("heroesSlain", "Attacking Hero model removed as a casualty", side="defender", maxPoints=6), T(LEADER), T(BROKEN, side="defender")],
    },
    "Baggage Train": {
        "roles": ATT_DEF,
        "objectives": [ET("cargoHeld", "Cargo", 5, side="defender"), ET("cargoTaken", "Cargo", 5, side="attacker"), T(LEADER), T(BROKEN)],
        "rewards": [{"effect": "cargo", "name": "Cargo", "source": {"rule": "Cargo", "marker": "If either side scores all Cargo"}, "condition": {"kind": "companyFlag", "key": "allCargo", "label": "Scored all Cargo (possessing it or escaping with it)"}}],
    },
    "The Wolves Attack": {"objectives": [T("leaderWargs"), T("companyWargs"), T("leaderSurvives"), T(BROKEN)]},
    "Seize the Outpost": {"objectives": [T("outpost"), T(LEADER), T(BROKEN), T(NO_COMMAND)]},
    "Tame the Beast": {"objectives": [T("woundMonster"), T("trophy"), T(LEADER), T(BROKEN), T(NO_COMMAND)]},
}

# Effects of the scenario rewards that are not Against the Odds bonuses (consumed through `campaign.aftermathEffects`).
EFFECTS = [
    {"key": "vantagePoint", "scenario": "Secure the Area", "who": "victor", "kind": "scenarioTableModifier", "die": "second", "range": [-1, 1],
     "when": "nextScenarioTableRoll", "text": "The winner of the scenario may apply +1 or -1 to the second die result the next time they roll on the Scenario Table, after both dice have been rolled."},
    {"key": "aFinalBlessing", "scenario": "Relic of Ages Past", "who": "modelInPossessionOfRelic", "kind": "magicalPower", "power": "Fortify Spirit",
     "when": "nextGame", "ignoresWill": True, "text": "The model in possession of the Relic at the end of the game gains the benefit of the Fortify Spirit Magical Power in their next game, even if they have no Will remaining."},
]

# The Rescue bullet about the Secondary Force and the Hold the Line "Defender" scoring are plain tiers; the sides below tell
# which company a bullet is for when it is not both (the keys are the role keys of the scenario).


def _cut(text, marker):
    return text.split(marker)[0].rstrip() if marker else text


# What a player secretly notes down at the start for each secret objective (Fog of War); read from the instruction of the bullet.
#   from: ownHeroes | enemyHeroes (a Hero model) | terrain (a terrain piece wholly in the opponent's half);
#   allowLeader: onlyHero = the Leader may be chosen only when the company has a single Hero.
SECRET_NOMINATIONS = {
    "ownHeroSurvives": {"from": "ownHeroes", "allowLeader": "onlyHero"},
    "enemyHeroHarmed": {"from": "enemyHeroes", "allowLeader": "onlyHero"},
    "terrainPiece": {"from": "terrain", "where": "opponentHalf"},
}


def enrich(scenarios, effects):
    """Adds roles / scoringObjectives / rewards to the parsed scenarios, moves glued reward and rule texts out of the bullets."""
    by_key = {e["key"]: e for e in effects}
    for s in scenarios:
        spec = SCENARIOS[s["name"]]
        bullets = s.get("scoring", [])
        assert len(bullets) == len(spec["objectives"]), (s["name"], len(bullets), len(spec["objectives"]))
        glued = {}
        for i, marker in spec.get("cut", {}).items():
            full = bullets[i]["text"]
            assert marker in full, (s["name"], marker)
            glued[marker] = full[full.index(marker):]
            bullets[i]["text"] = _cut(full, marker)
            for t in bullets[i]["tiers"]:
                t["text"] = _cut(t["text"], marker)
        if "roles" in spec:
            s["roles"] = [dict(r) for r in spec["roles"]]
        objectives = []
        for b, o in zip(bullets, spec["objectives"]):
            tiers = b["tiers"]
            lead = [t for t in tiers if t["points"] == 0]
            options = [{"text": t["text"], "points": t["points"]} for t in tiers if t["points"] > 0]  # in the printed order
            assert options, (s["name"], b["text"])
            ob = {"key": o["key"], "text": b["text"], "side": o.get("side", "both"), "kind": o["kind"]}
            if lead:
                ob["instruction"] = " ".join(t["text"] for t in lead)
            if o.get("secret"):
                ob["secret"] = True
                ob["reveal"] = "endOfGame"  # kept hidden from the opponent until the end of the game (the scenario's Objectives)
                ob["completion"] = o["kind"]  # the player reports the best tier reached at the end
                if o["key"] in SECRET_NOMINATIONS:
                    ob["nomination"] = dict(SECRET_NOMINATIONS[o["key"]])
            if o["kind"] == "tiers":
                ob["options"] = options
                ob["maxPoints"] = max(x["points"] for x in options)
            elif o["kind"] == "each":
                assert len(options) == 1, (s["name"], o["key"])
                ob["item"], ob["pointsEach"] = o["item"], options[0]["points"]
                ob["options"] = [{"text": b["text"], "points": options[0]["points"]}]  # the one printed sentence, worth pointsEach
                if "maxCount" in o:
                    ob["maxCount"] = o["maxCount"]
                ob["maxPoints"] = min([x for x in (o.get("maxPoints"), o["maxCount"] * ob["pointsEach"] if "maxCount" in o else None) if x is not None], default=None)
                if ob["maxPoints"] is None:
                    del ob["maxPoints"]
                elif "maxCount" not in ob:
                    ob["maxCount"] = ob["maxPoints"] // ob["pointsEach"]
                ob["count"] = ob["maxCount"]
            else:
                ob["item"], ob["count"], ob["options"] = o["item"], o["count"], options
                ob["maxPoints"] = o["count"] * max(x["points"] for x in options)
            objectives.append(ob)
        s["scoringObjectives"] = objectives
        keys = [o["key"] for o in objectives]
        assert len(keys) == len(set(keys)), (s["name"], keys)
        rewards = []
        for r in spec.get("rewards", []):
            eff = by_key[r["effect"]]
            assert eff["scenario"] == s["name"], (r["effect"], s["name"])
            src = r.get("source")
            if src is None:  # glued to a Victory Point bullet: "<Name> - <text>"
                text = next(v for m, v in glued.items() if m.startswith(r["name"])).split(" - ", 1)[1]
            else:  # inside a special rule: the whole rule, or the text from a marker on
                rule = next(x for x in s["specialRules"] if x.get("name") == src["rule"])
                if "marker" in src:
                    d = rule["definition"]
                    assert src["marker"] in d, (s["name"], src)
                    text = d[d.index(src["marker"]):]
                    if src.get("dropMarker"):
                        text = text[len(src["marker"]):]
                    text = text.strip()
                    rule["definition"] = d[:d.index(src["marker"])].rstrip()
                else:
                    text = rule["definition"]
                    s["specialRules"].remove(rule)
            rewards.append({**r, "text": text, "who": eff["who"]})
            rewards[-1].pop("source", None)
        if rewards:
            s["rewards"] = rewards
        if "Strange Aura:" in glued:  # a special rule that the layout glued to the last Victory Point bullet
            rule = {"name": "Strange Aura", "definition": glued["Strange Aura:"].split(":", 1)[1].strip()}
            head, _, flat = rule["definition"].partition(" D6 Result ")  # the layout flattened the cells of its D6 table
            assert flat, "Strange Aura: the D6 table is gone"
            parts = re.split(r"(?:(?<=^)|\s)(\d - \d)\s", " " + flat.strip() + " ")
            cells = [x.strip() for x in parts[1:]]
            rows = []
            for i in range(0, len(cells), 2):
                lo, hi = (int(n) for n in cells[i].split(" - "))
                rows.append({"min": lo, "max": hi, "cells": [cells[i], cells[i + 1]]})
            assert [(r["min"], r["max"]) for r in rows] == [(1, 2), (3, 4), (5, 6)], rows
            rule["definition"], rule["table"] = head.strip(), {"lookup": "d6", "columns": ["D6", "Result"], "rows": rows}
            s.setdefault("specialRules", []).append(rule)
    return scenarios
