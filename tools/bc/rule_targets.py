"""data.ruleTargets: the direction of every special rule that names or implies a keyword or a target (bead vj0.6, docs/unit-keywords.md).

A rule NOT listed here is `own` (about the bearer only). Listed: rules whose effect depends on who the OTHER model is (enemy or friendly,
by keyword) and rules that only work whilst the bearer has a keyword. Names are vanilla or Battle Companies rule names (checked by the
validator); `PATTERNS` are the parameterised families whose X is a keyword (every parameterised rule of kind keyword must be there).
"""
from .text import fold

TEXT = (
    "Who a special rule works against or for. appliesTo: enemy-keyword (enemy models that carry the keyword), enemy-any (enemy models in general, no "
    "keyword), friendly-keyword (friendly models that carry the keyword benefit), own (the bearer only; a rule that is not listed is own). target: "
    "short text for the tooltip, {X} is the keyword(s) of the rule's brackets (kind pattern) or of `keywords` (kind name), several joined with "
    "' or ' when keywordMatch is any and ' and ' when all. bearerKeywords: the rule only works whilst the bearer has one of these keywords. "
    "The label of a pattern rule is 'Rule (A)(B)': every bracket is one alternative (keywordMatch any); Fury (X) names the keywords of one "
    "group of models in a single bracket ('Mordor Orc': a model needs all of them)."
)

# (rule, appliesTo, target, keywordMatch) -- families with a keyword parameter, matched on the label's base name and a bracket.
PATTERNS = [
    ("Hatred (X)", "enemy-keyword", "enemy models with the {X} keyword", "any"),
    ("Ancient Enemies (X)", "enemy-keyword", "enemy models with the {X} keyword", "any"),
    ("Terror (X)", "enemy-keyword", "enemy models with the {X} keyword (plain Terror affects all enemy models)", "any"),
    ("Fury (X)", "friendly-keyword", "friendly models with the {X} keywords within 6\"", "all"),
]

# (rule, appliesTo, keywords, target, bearerKeywords). Read from the rule texts; the keyword list is what the text names.
NAMES = [
    # enemy models by keyword / type
    ("End the Race of Men", "enemy-keyword", ["Man"], "enemy {X} models", []),
    ("At 'em, Lads!", "enemy-keyword", ["Hobbit"], "enemy {X} models", []),
    ("Put 'em in chains!", "enemy-keyword", ["Hobbit"], "enemy {X} models", []),
    ("Against all odds", "enemy-keyword", ["Hero", "Monster"], "enemy {X} models", []),
    ("Glory Seekers", "enemy-keyword", ["Hero"], "enemy {X} models", []),
    ("Angmar Arise (Slayer of Men)", "enemy-keyword", ["Hero"], "enemy {X} models", []),
    ("Furious Tirade", "enemy-keyword", ["Warrior"], "enemy {X} models", []),
    ("Feral Charge", "enemy-keyword", ["Infantry"], "enemy {X} models it Charges", []),
    ("Gatekeeper", "enemy-keyword", ["Infantry"], "enemy {X} models", []),
    ("Sharpshooter", "enemy-keyword", ["Cavalry"], "enemy {X} models", []),
    ("\"Slay their Horses!\"", "enemy-keyword", ["Mount"], "enemy {X}s", ["Cavalry"]),
    # enemy models in general
    ("Terror", "enemy-any", [], "enemy models in general", []),
    ("Visage of Terror", "enemy-any", [], "enemy models that try to Charge it", []),
    ('Harbinger of Evil (12")', "enemy-any", [], "enemy models within 12\"", []),
    ('Harbinger of Evil (18")', "enemy-any", [], "enemy models within 18\"", []),
    ("Keen Sight", "enemy-any", [], "enemy models within 12\"", []),
    ("Wormtongue", "enemy-keyword", ["Hero"], "enemy {X} models within 6\"", []),
    # friendly models by keyword
    ("\"Dol Amroth for Gondor!\"", "friendly-keyword", ["Dol Amroth"], "friendly {X} models", []),
    ("Battle Wisdom", "friendly-keyword", ["Lothlorien", "Warrior"], "friendly {X} models within 3\"", []),
    ("Chronicler (Thorin's Company)", "friendly-keyword", ["Thorin's Company"], "friendly {X} models within 6\"", []),
    ("Gift of Foresight", "friendly-keyword", ["Arnor"], "friendly {X} models within 6\"", []),
    ("High King of the Elves", "friendly-keyword", ["Elf", "Hero"], "friendly {X} models", []),
    ("High Spirits", "friendly-keyword", ["Hobbit"], "friendly {X} models within 6\"", []),
    ("Official Meals", "friendly-keyword", ["Fellowship"], "friendly {X} models", []),
    ("Ghostly Resolve", "friendly-keyword", ["Angmar", "Orc"], "friendly {X} models within 12\"", []),
    ("Master of Birds", "friendly-keyword", ["Eagle"], "friendly {X} models within 12\"", []),
    ("Protect the little ones!", "friendly-keyword", ["Fellowship", "Hobbit"], "friendly {X} models (the bearer must be within 3\")", []),
    ("To me, Shire-folk!", "friendly-keyword", ["Hobbit"], "friendly {X} models", []),
    ("Sweet Maiden of the Golden Ale", "friendly-keyword", ["Hobbit"], "friendly {X} models", []),
    ("Relentless Advance", "friendly-keyword", ["Orc", "Goblin", "Bat"], "friendly {X} models it moves through", []),
    ("Favour of the War Chief", "friendly-keyword", ["Dunland", "Hero"], "a friendly {X} model within 3\"", []),
    ("The Honour of Lamedon", "friendly-keyword", ["Lamedon", "Hero"], "a friendly {X} model within 3\"", []),
    ("Onwards Men of Esgaroth", "friendly-keyword", ["Lake-town", "Hero"], "a friendly {X} model within 3\"", []),
    ("Raising Spirits (Thorin's Company)", "friendly-keyword", ["Thorin's Company", "Hero"], "friendly {X} models within 3\"", []),
    ("Reading the Portents (Thorin's Company)", "friendly-keyword", ["Thorin's Company", "Hero"], "friendly {X} models within 6\"", []),
    ("Spider-kin", "friendly-keyword", ["Spider"], "friendly {X} models", []),
    ("Packlord", "friendly-keyword", ["Warg"], "friendly {X} models", []),
    ("Pack Master", "friendly-keyword", ["Warg"], "friendly {X} models", []),
    ("Goblin Mastery", "friendly-keyword", ["Moria", "Goblin"], "friendly {X} models within 6\"", []),
    ("Lordly Quality", "friendly-keyword", [], "friendly models that share the bearer's faction keyword", []),
    ("Defender", "friendly-keyword", [], "a friendly Hero with the opposite species keyword (Man/Dwarf)", []),
    # rules that only work whilst the bearer has a keyword (the bearer is the subject: own)
    ("Arise, Riders of Rohan!", "own", [], "", ["Cavalry"]),
    ("The Gleaming Horde", "own", [], "", ["Cavalry"]),
    ("King's Knights", "own", [], "", ["Cavalry"]),
    ("Hunt Master", "own", [], "", ["Cavalry"]),
    ("Fury of the Pack", "own", [], "", ["Cavalry"]),
    ("Master Hunter", "own", [], "", ["Cavalry"]),
    ("Bladelord", "own", [], "", ["Infantry"]),
    ("Ancestral Fury", "own", [], "", ["Infantry"]),
    # rules about the bearer that mention keywords (listed so the audit sees a decision)
    ("Bodyguard", "own", [], "", []),
    ("Woodland Creature", "own", [], "", []),
    ("Mountain Dweller", "own", [], "", []),
    ("Command Support", "own", [], "", []),
]


def build(doc, van):
    """The data.ruleTargets table; stops on a name that is neither a vanilla nor a Battle Companies rule."""
    d = doc["data"]
    known = {fold(k["name"]) for k in van["data"]["keywords"]} | {fold(k["name"]) for k in d["keywords"]}
    bad = [n[0] for n in NAMES if fold(n[0]) not in known]
    params = {fold(p["name"]) for p in d["parameterisedRules"]}
    bad += [p[0] for p in PATTERNS if fold(p[0].split(" (")[0]) not in known | params]
    assert not bad, f"rule_targets.py names rules that exist neither in vanilla nor in the Battle Companies file: {bad}"
    patterns = []
    for rule, applies, target, match in PATTERNS:
        base = rule.split(" (")[0]
        variants = sorted({k["name"] for k in van["data"]["keywords"] if k["name"].startswith(base + " (")}
                          | {k["name"] for k in d["keywords"] if k["name"].startswith(base + " (")})
        patterns.append({"rule": rule, "base": base, "appliesTo": applies, "target": target, "keywordMatch": match, "variants": variants})
    names = []
    for rule, applies, keywords, target, bearer in NAMES:
        e = {"rule": rule, "appliesTo": applies}
        if keywords:
            e["keywords"] = keywords
            e["keywordMatch"] = "any" if rule in ("Against all odds", "Relentless Advance") else "all"
        if target:
            e["target"] = target
        if bearer:
            e["bearerKeywords"] = bearer
        names.append(e)
    return {"text": TEXT, "patterns": patterns, "names": names}
