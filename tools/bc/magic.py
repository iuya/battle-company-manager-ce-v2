"""Magical Powers a Battle Companies Hero can learn (Path of the Sorcerer) and the starting gear of each Path.

`build_magical_powers(paths, van)` -> data.magicalPowers: every vanilla power with its text and duration (vanilla file) joined
with the range and Casting Value the Battle Companies rules give the Sorcerer's Spell List. `learnable` powers are the ones on
that list; the others only appear on unit profiles (data.gear / units[].magicalPowers carry their own range and casting).
`add_starting_gear(paths, glossary)` adds `startingHeroicAction` to every path.
"""

CASTING_RULE = {
    "notation": "X+",
    "lowerIsBetter": True,
    "improvement": {"delta": -1, "maxPerPower": 2, "scope": "perHeroPerPower", "source": "paths[Path of the Sorcerer].progression[].options[type=improveCastingValue]"},
    "floor": 2, "defaultNotConfirmed": ["floor", "capAppliesToEveryRow", "cannotLearnKnownPower"],
    "confirmedByUser": ["onePathOfTheSorcererHeroPerCompany", "furyPerSpecies"],
    "contributesToRating": False,
    "text": "A Casting Value is the dice score needed on a D6 for a Casting Test (a 4+ Power needs a 4, 5 or 6). Improving it by 1 lowers it "
            "by 1 (5+ becomes 4+); a Power can be improved at most twice. Magical Powers and Casting Value improvements add no rating.",
}


def build_magical_powers(paths, van):
    sorcerer = next(p for p in paths if p["name"] == "Path of the Sorcerer")
    listed = {m["magicalPower"]: m for m in sorcerer["spellList"]}
    out = []
    for m in van["data"]["magicalPowers"]:
        item = {"name": m["name"], "duration": m["duration"], "definition": m["definition"]}
        if m["name"] in listed:
            s = listed[m["name"]]
            item.update(learnable=True, range=("self" if s["range"] == "Self" else s["range"]), casting=s["casting"])
            if "parameter" in s:
                item["parameter"] = s["parameter"]
                item["variants"] = [v["name"] for v in van["data"]["magicalPowers"] if v["name"].startswith(m["name"] + " (")]
                item["learnedPerVariant"] = True
        else:
            item["learnable"] = False
        out.append(item)
    missing = set(listed) - {m["name"] for m in van["data"]["magicalPowers"]}
    assert not missing, missing
    return {
        "text": "Every Magical Power of the vanilla file. `learnable` ones are on the Path of the Sorcerer's Spell List (the Battle "
                "Companies rules give them the `range` and the Casting Value `casting` a Hero has when it learns them); the others "
                "appear only on unit profiles, which carry their own range and casting. `range` is inches or \"self\".",
        "castingRule": CASTING_RULE,
        "items": out,
    }


UNIVERSAL = {"Path of the Tactician": "Heroic Move", "Path of the Sharpshooter": "Heroic Shoot", "Path of the Soldier": "Heroic Combat"}


def add_starting_gear(paths, glossary):
    gl = {t["name"].replace("Channelling", "Channeling"): t["definition"] for t in glossary["terms"]}
    for p in paths:
        ha = p.get("heroicAction")
        if ha:
            p["startingHeroicAction"] = {"name": ha, "universal": False, "definition": gl.get(ha, "")}
        else:
            p["startingHeroicAction"] = None
            p["startingNote"] = ("The path adds no Heroic Action; its Mastery rule improves the universal %s that every Hero has."
                                 % UNIVERSAL.get(p["name"], "Heroic Action"))
        if p.get("startingMagicalPower"):
            p["startingMagicalPower"] = {"choose": 1, "from": "data.magicalPowers.items[learnable]"}
    for p in paths:
        stack = [o for r in p["progression"] for o in r["options"]]
        while stack:
            o = stack.pop()
            if o["type"] == "oneOf":
                stack.extend(o["options"])
            elif o["type"] == "magicalPower":
                o["from"] = "data.magicalPowers.items[learnable]"
                o["excludeKnown"] = True
            elif o["type"] == "improveCastingValue":
                o["rule"] = "data.magicalPowers.castingRule"
