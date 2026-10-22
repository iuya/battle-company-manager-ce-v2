"""Company effects: every company-specific rule as a small list of typed effects (`companies[].effects[]`).

Vocabulary (kind: layer = management / match / both):
  maxModels{value}                      management  roster size (15 normal, 12 elite, 20, 25)
  limitOverride{limit, percent}         management  Bow / Cavalry percentage replaced
  limitExemption{unit|item, limits[]}   management  units/items left out of a limit (count and base)
  companyLimit{keyword, maxPercentage}  management  at most N% of the company with a keyword
  grantedRule{specialRule, appliesTo}   both        free special rule on models/heroes
  ruleExchange{remove, add}             match       one special rule replaced by another
  ruleSubstitution{...}                 match       "treat X as Y" for a special rule
  unitAlias{label, unit, ...}           management  names a unit option / legacy name
  onTheirOwnPath{units[]}               management  no XP, advancement or promotion
  uniqueWargear{name, ...}              management  company-only wargear (cost, rating)
  specialUnit{unit, rare, influence}    management  hire outside the charts
  breakPoint{percent}                   match       Broken at N% (default 50)
  heroUpgrade{name, restriction?}       management  Company Hero Upgrade offered on level-up
  heroPromotion{from, onResult, becomes, keep, drop, heroStatGain, pathChoice, forgoAllowed}  management  Hero in the Making result promotes AND makes a hero
  startingHeroRestriction{allowed|excluded}      management  who may be one of the three starting Heroes
  startingHeroNoStatGain{unit}          management  already a Hero: no extra Might/Will/Fate at the start
  raceBalance{keyword, notMoreThanKeyword, onExceed}  management  recruiting limit between two groups
  includedUnitPoints{unit, members[{name, points}]}  management  rating of the units a vanilla pack includes (vanilla gives 0)
  modelCount{unit, models, limits[]}    management  one purchase counts as N models against the limits
  (chart rows of type unitChoice are described in campaign.recruitment.unitChoice)
  unitTypes{unit, add[], remove[]}      both        keyword override of a unit
  compositeModel{unit, parts[], counts, casualty, rules[]}  both  one card made of several riders and a mount
  reinforcementReplacement{chart, results, alternative, playerChoice, cost}  management  an alternative option on chart rows
  recruitFallback{unit, when, choose, charts?}  management  what to do when a recruit result is not possible
  recruitAsHero{statGain}               management  every recruit is a Hero in the Making
  rangerReplacement{unit, max, role}    management  replace a slain special hero with any success
  aura{...}                             match       conditional rule on the battlefield
"""
import re


def _norm(t):
    return re.sub(r"\s+", " ", t.replace("’", "'").replace("“", '"').replace("”", '"').replace("”", '"')).strip().lower()


# Hand-written effects: company -> [(effect, sentence that must appear in the company's rules/notes)]
HAND = {
    "Fornost": [({"kind": "reinforcementReplacement", "chart": "reinforcements", "results": [2, 6], "alternative": {"unit": "Hobbit Archer"},
                  "playerChoice": True, "cost": 0, "layer": "management"},
                 "replace result 2 - 6 on this chart with a Hobbit Archer")],
    "Arnor": [({"kind": "heroPromotion", "from": "Ranger of Arnor", "onResult": "heroInTheMaking", "becomes": "Ranger of the North",
                "keep": ["Spear"], "drop": ["Armour"], "heroStatGain": False, "pathChoice": "any", "forgoAllowed": False,
                "layer": "management"},
               "trade out their profile in its entirety for a Ranger of the North")],
    "Helm's Deep": [({"kind": "startingHeroNoStatGain", "unit": "Ranger of the North", "layer": "management"},
                     "already being a Hero, will not gain additional Might, Will and Fate")],
    "The Shire": [({"kind": "startingHeroNoStatGain", "unit": "Ranger of the North", "layer": "management"},
                   "already being a Hero, will not gain additional Might, Will and Fate"),
                  ({"kind": "rangerReplacement", "unit": "Ranger of the North", "max": 1, "role": ["leader", "sergeant"],
                    "when": "slain; replaces any successful reinforcement result", "layer": "management"},
                   "may swap any successful result on the Reinforcements chart for a Ranger of the North")],
    "Defenders of the North": [({"kind": "raceBalance", "keyword": "Dwarf", "notMoreThanKeyword": "Dale",
                                 "onExceed": "chooseLowerResult", "layer": "management"},
                                "number of Dwarf models may not exceed the number of Dale models")],
    "Durin's Folk": [({"kind": "includedUnitPoints", "unit": "Vault Warden Team",
                       "members": [{"name": "Iron Shield Bearer", "points": 13}, {"name": "Foe Spear Wielder", "points": 12}],
                       "source": "vanilla includes (points 0 there)", "layer": "management"},
                      "counts as two different models"),
                     ({"kind": "modelCount", "unit": "Vault Warden Team", "models": 2, "limits": ["maxModels"], "layer": "management"},
                      "counts as two different models"),
                     ({"kind": "recruitFallback", "unit": "Vault Warden Team", "when": "wouldExceedMaxModels", "choose": "anyResult",
                       "charts": ["reinforcements", "specialChart"], "cost": 0, "layer": "management"},
                      "you may choose any other option from the Special chart"),
                     ({"kind": "recruitFallback", "unit": "Vault Warden Team", "when": "memberMissing", "choose": "replacementOfMissingMember",
                       "swapRollOn": "specialChart", "anyRoll": True, "cost": 0, "layer": "management"},
                      "you may substitute any roll on the Special Chart")],
    "Lake-town": [({"kind": "startingHeroRestriction", "allowed": ["Lake-town Guard"], "layer": "management"},
                   "Only Lake-town Guard may be selected as Hero models")],
    "Wanderers in the Wild": [({"kind": "recruitAsHero", "statGain": {"might": 1, "will": 1, "fate": 1}, "layer": "management"},
                               "automatically promote to Heroes in the Making"),
                              ({"kind": "aura", "condition": "a friendly Hobbit is alive and on the battlefield",
                                "effect": "Men, Elf and Dwarf models are Fearless", "layer": "match"},
                               "all friendly Men, Elf, and Dwarf models are considered Fearless")],
    "Cirith Ungol": [({"kind": "startingHeroRestriction", "excluded": ["Mirkwood Giant Spider"], "layer": "management"},
                      "The Spider may not be one of the three starting Heroes")],
    "Mirkwood": [({"kind": "unitTypes", "unit": "Warg Marauder", "add": ["Cavalry"], "remove": ["Infantry"], "layer": "both"},
                  "takes up 3 slots of the Battle Company Roster"),
                 ({"kind": "compositeModel", "unit": "Warg Marauder",
                   "parts": [{"role": "shieldRider", "unit": "Moria Goblin Warrior", "count": 1, "wargear": ["Shield"]},
                             {"role": "bowRider", "unit": "Moria Goblin Warrior", "count": 2, "wargear": ["Orc bow"]},
                             {"role": "mount", "mount": "Warg", "count": 1}],
                   "counts": {"rosterSlots": 3, "maxModels": 3, "bowLimit": 1, "cavalryLimit": 1},
                   "onePhysicalCard": True,
                   "casualty": {"injuredWhen": "allRidersRemoved", "table": "warrior", "appliesToWholeModel": True},
                   "rules": ["Warg Marauder", "On the Hunt"], "layer": "both"},
                  "The result applies to the entire model")],
}


# On Their Own Path units named exactly as the units are called in the vanilla file (or a company unitAlias label). The key is
# what the rulebook says; the build asserts the parser still finds it. One entry per unit.
OTOP_EXACT = {
    ("Angmar & Carn Dum", "Werewolves"): ["Werewolf"],
    ("Dol Guldur", "Mirkwood Hunting"): ["Mirkwood Hunting Spider"],
    ("Dol Guldur", "Giant Spiders"): ["Mirkwood Giant Spider"],
    ("Easterlings", "Rhûnish War Drakes"): ["War Drake of Rhun"],
    ("Goblin-Town", "The Goblin Hulk"): ["Goblin Hulk"],
    ("Gundabad", "Berserkers"): ["Gundabad Berserker"],
    ("Harad", "Watchers of Kârna with Twin Blades"): ["Watcher of Kârna"],
    ("Mirkwood", "Spiders"): ["Mirkwood Hunting Spider", "Mirkwood Giant Spider"],
}


# Chart rows that offer a choice among named units, each with its own Rare limit: (company, chart, roll) -> options.
UNIT_CHOICES = {
    ("Wanderers in the Wild", "reinforcements", (2, 2)): {
        "was": "lostStartingUnit",
        "options": [
            {"unit": "Hobbit Militia", "rare": 1},
            {"unit": "Hobbit Shirriff", "rare": 1},
            {"unit": "Hobbit Archer", "label": "Tookish Hunter", "selectedOptions": ["Tookish Hunter"], "rare": 1},
            {"unit": "Hobbit Militia", "label": "Battlin' Brandybuck", "selectedOptions": ["Battlin' Brandybuck"], "rare": 1},
        ],
        "text": "Hobbit (rulebook: any Hobbit profile from the Starting Battle Company that was lost): the four Hobbit profiles of the starting "
                "company, one of each at most; the player may recruit any that the company does not have.",
    },
}


def apply_unit_choices(c):
    for (name, chart, roll), spec in UNIT_CHOICES.items():
        if name != c["name"]:
            continue
        for row in c[chart]["table"]:
            if tuple(row["roll"]) == roll and row["result"]["type"] != "unitChoice":
                assert row["result"]["type"] == spec["was"], row
                row["result"] = {"type": "unitChoice", "options": [dict(o) for o in spec["options"]], "whenAllExhausted": "lowerResult",
                                 "text": spec["text"]}


def exact_on_their_own_path(c):
    out = []
    for n in c.get("onTheirOwnPath", []):
        out.extend(OTOP_EXACT.get((c["name"], n), [n]))
    return out


def build_effects(c):
    otop_text = " ".join(n["text"] for n in c.get("notes", []) if "On Their Own Path" in n["text"])
    if c["name"] == "Harad":
        otop_text += (" Decision: the whole Watcher of Kârna unit is On Their Own Path; the rulebook's 'with Twin Blades' is treated as "
                      "a rulebook oversight.")
    apply_unit_choices(c)
    if "onTheirOwnPath" in c:
        c["onTheirOwnPathRulebook"] = list(c["onTheirOwnPath"])
        c["onTheirOwnPath"] = exact_on_their_own_path(c)
    page = _norm(" ".join(list(c.get("specialRules", [])) + [n["text"] for n in c.get("notes", [])]))
    out = []
    if "maxModels" in c:
        out.append({"kind": "maxModels", "value": c["maxModels"], "layer": "management"})
    for k, v in sorted(c.get("limitOverrides", {}).items()):
        out.append({"kind": "limitOverride", "limit": k, "percent": v, "layer": "management"})
    for x in c.get("limitExemptions", []):
        out.append({"kind": "limitExemption", **{k: v for k, v in x.items()}, "layer": "management"})
    for x in c.get("companyLimits", []):
        out.append({"kind": "companyLimit", **x, "layer": "management"})
    for x in c.get("grantedSpecialRules", []):
        out.append({"kind": "grantedRule", "specialRule": x["specialRule"], "appliesTo": x["appliesTo"], "cost": x.get("cost", 0),
                    "text": x["text"], "layer": "both"})
    for x in c.get("ruleExchanges", []):
        out.append({"kind": "ruleExchange", **x, "layer": "match"})
    for x in c.get("ruleSubstitutions", []):
        out.append({"kind": "ruleSubstitution", **x, "layer": "match"})
    for x in c.get("unitAliases", []):
        out.append({"kind": "unitAlias", **{k: v for k, v in x.items() if k != "companyNotes"}, "layer": "management"})
    if c.get("onTheirOwnPath"):
        out.append({"kind": "onTheirOwnPath", "units": c["onTheirOwnPath"], "path": "On Their Own Path", "text": otop_text, "layer": "management"})
    for x in c.get("uniqueWargear", []):
        out.append({"kind": "uniqueWargear", "name": x["name"], "layer": "management"})
    for x in c.get("specialUnits", []):
        out.append({"kind": "specialUnit", **x, "layer": "management"})
    if "breakPointPercentage" in c:
        out.append({"kind": "breakPoint", "percent": c["breakPointPercentage"], "layer": "match"})
    for u in c.get("heroUpgrades", []):
        e = {"kind": "heroUpgrade", "name": u["name"], "specialRule": u["specialRule"], "layer": "management"}
        if "restriction" in u:
            e["restriction"] = u["restriction"]
        out.append(e)
    for eff, sentence in HAND.get(c["name"], []):
        assert _norm(sentence) in page, (c["name"], sentence)
        out.append(dict(eff))
    return out
