"""data.ttsModels: the fixed mappings from Battle Companies to the model names of the Tabletop Simulator MESBG FTC mod.

Only short names, never the mod's catalogue (docs/tabletop-simulator.md). Every name is a vanilla name (the mod keys its models by
vanilla names); the validator also checks them against the local, gitignored sources/tts/models.json when that file exists.
"""
from .refs import referenced
from .text import fold

TEXT = (
    "How a Battle Companies unit is written for the Tabletop Simulator MESBG FTC mod (docs/tabletop-simulator.md). standIns: models the mod has no "
    "1:1 figure for (wanderers, creatures) and the mod model shown instead; mounts: a bought mount that fights (vanilla Attacks above 0) and the "
    "model of its figure; riders: the cavalry units, the model of the rider on foot and whether the mod's figure carries the dismounted state "
    "(`states`); names: option spellings that differ between the vanilla file and the mod; defaultOptions: the options a unit is shown with when it carries none "
    "(the bag has no bare figure); markerOptions: an alias marker option (Lorien Guard) left out of the key when it is the unit's only option. A name here is a vanilla name; a stand-in or mount "
    "model may also be 'Name: options'."
)

# kind, BC name -> (model, alternatives, reason). Approved by the owner (zrs epic notes).
STAND_INS = [
    ("wanderer", "Wandering Swordsman", "Captain of Dale", ["Knight of Dale"], "a human swordsman; no wanderer figure in the mod"),
    ("wanderer", "Wandering Marksman", "Ruffian: Bow", ["Ruffian"], "a human archer; the Ruffian with a bow"),
    ("wanderer", "Wandering Scavenger", "Goblin Mercenary Captain", ["Goblin Mercenary"], "a scavenger, a lone mercenary figure"),
    ("wanderer", "Wandering Sage", "Malbeth the Seer", [], "an old sage, a seer"),
    ("creature", "Baggage Pony", "Bill the Pony", [], "the pony of the Fellowship"),
    ("creature", "Falcon", "Crebain", [], "a bird; the Crebain figure"),
    ("creature", "Hunting Dog", "Fang", [], "a hound; Fang"),
]

# A bought mount (BC wargear class mount) whose figure is not the mount itself: gear name -> (model, alternatives, reason).
MOUNT_MODELS = {
    "Warg": ("Fell Warg", ["Wild Warg"], "the mod has no plain Warg figure; the Fell Warg is the closest (owner decision, REVIEW)"),
}

# Cavalry units: unit -> (foot model or None, states, reason). `states`: every model key of the unit in the mod carries the
# Tabletop Simulator state that turns the mounted figure into the rider on foot (docs/tabletop-simulator.md, "Riders"). The six units
# whose keys carry it only in part are False (owner question Q8: flip to True to treat them as stated).
RIDERS = {
    "Rider of Rohan": ("Warrior of Rohan", True, "states on every model key"),
    "Son of Eorl": (None, True, "states on every model key; no on-foot model of its own"),
    "Mirkwood Elf Knight": ("Mirkwood Elf Warrior", True, "states on every model key (the alias Mirkwood Cavalry resolves to it)"),
    "Hunter Orc Warg Rider": ("Hunter Orc Warrior", True, "states on every model key (rider on foot and the Fell Warg alone)"),
    "Black Numenorean Knight": ("Black Numenorean Warrior", False, "states on 3 of 4 keys"),
    "Galadhrim Knight": ("Galadhrim Warrior", False, "states on 3 of 7 keys"),
    "Iron Hills Goat Rider": ("Dwarf Warrior", False, "states on 1 of 2 keys; no Iron Hills foot model in the bag; owner chose a plain Dwarf Warrior for now"),
    "Knight of Minas Tirith": ("Warrior of Minas Tirith", False, "states on 1 of 2 keys"),
    "Mordor Warg Rider": ("Mordor Orc Warrior", False, "states on 2 of 5 keys"),
    "Rivendell Knight": ("Rivendell Warrior", False, "states on 2 of 3 keys"),
    "Angmar Warg Rider": ("Angmar Orc Warrior", False, "no states"),
    "Dunlending Horseman": ("Dunlending Warrior", False, "no states"),
    "Easterling Kataphrakt": ("Easterling Warrior", False, "no states"),
    "Haradrim Raider": ("Haradrim Warrior", False, "no states"),
    "Isengard Warg Rider": ("Isengard Orc Warrior", False, "no states"),
    "Khandish Horseman": ("Khandish Warrior", False, "no states"),
    "Knight of Arnor": ("Warrior of Arnor", False, "no states"),
    "Serpent Rider": ("Serpent Guard", False, "no states"),
}

# Spelling differences between a vanilla option and the mod's key: (unit, vanilla option, mod option, reason).
NAMES = [
    ("Isengard Orc Warrior", "Orc bow", "Bow", "the mod writes the plain Bow for the Isengard Orc Warrior"),
]

# Units whose figure needs an option when the company carries none: unit -> (options, reason). Without it the key would not be one of the
# mod's bag (no figure of the unit is bare), and the published image has no catalogue to look the closest key up in.
DEFAULT_OPTIONS = [
    ("Mahûd Warrior", ["Spear"], "the bag has no bare Mahûd Warrior; owner decision: melee over ranged, the cheapest (Spear, 1 point)"),
    ("Rohirrim Traitor", ["Shield"], "the bag has no bare Rohirrim Traitor; owner decision: melee over ranged, the cheapest (Shield, 1 point). "
     "REVIEW: the owner may prefer 'Shield and throwing spears'"),
]

# Options that only name the unit (a company alias selects them) and that the figure lacks when they are the only option: unit, option, reason.
MARKER_OPTIONS = [
    ("Galadhrim Warrior", "Lorien Guard", "the bag has no figure with the Lorien Guard marker alone: the bare Galadhrim Warrior is shown and the "
     "name and description keep the Lorien Guard identity (owner decision)"),
]


def _vanilla_names(van):
    d = van["data"]
    return {fold(e["name"]) for k in ("heroes", "warriors", "gear") for e in d[k]}


def _model_base(model):
    return model.split(":")[0].strip()


def build(doc, van):
    """The data.ttsModels table; stops when a name is not vanilla or a cavalry unit used by a company has no rider entry."""
    d = van["data"]
    names = _vanilla_names(van)
    units = {fold(e["name"]): e for k in ("heroes", "warriors") for e in reversed(d[k])}  # first occurrence wins
    bad = []

    def check(name, where):
        if fold(_model_base(name)) not in names:
            bad.append(f"{where}: {name!r}")

    mount_gear = {fold(g["name"]): g for g in reversed(d["gear"])}
    stand_ins = []
    for kind, name, model, alts, reason in STAND_INS:
        check(model, f"standIn {name}")
        for a in alts:
            check(a, f"standIn {name} alternative")
        stand_ins.append({"kind": kind, "name": name, "model": model, **({"alternatives": alts} if alts else {}), "reason": reason})
    sources = {w["name"] for w in doc["data"]["wanderers"]["items"]} | {c["name"] for c in doc["data"]["creatures"]["items"]}
    if sources != {s[1] for s in STAND_INS}:
        bad.append(f"stand-ins {sorted(s[1] for s in STAND_INS)} do not match the wanderers and creatures {sorted(sources)}")

    mounts = []
    bought = sorted({m for g in doc["data"]["wargear"]["items"] if g.get("type") == "mount" for m in g["names"]})
    for m in bought:
        g = mount_gear.get(fold(m))
        if not g or not (g.get("attack") or 0) > 0:
            continue
        model, alts, reason = MOUNT_MODELS.get(m, (m, [], "the mount fights (Attacks above 0): its figure is the mount itself"))
        check(model, f"mount {m}")
        mounts.append({"gear": m, "model": model, **({"alternatives": alts} if alts else {}), "attacks": g["attack"], "reason": reason})
    for m in MOUNT_MODELS:
        if m not in bought:
            bad.append(f"MOUNT_MODELS names {m!r}, which no company can buy")

    cavalry = set()
    for u in referenced(doc, van)["units"]:
        e = units.get(fold(u))
        if e and "Cavalry" in (e.get("unitType") or []):
            cavalry.add(e["name"])
    if cavalry != set(RIDERS):
        bad.append(f"RIDERS differs from the cavalry units the companies use: missing {sorted(cavalry - set(RIDERS))}, extra {sorted(set(RIDERS) - cavalry)}")
    riders = []
    for u, (foot, states, reason) in RIDERS.items():
        check(u, "rider")
        if foot:
            check(foot, f"rider {u} footModel")
        riders.append({"unit": u, **({"footModel": foot} if foot else {}), "states": states, "reason": reason})

    opts = []
    for unit, frm, to, reason in NAMES:
        e = units.get(fold(unit))
        if not e or not any(o["name"] == frm for o in e.get("options", [])):
            bad.append(f"names: {unit!r} has no vanilla option {frm!r}")
        opts.append({"unit": unit, "from": frm, "to": to, "reason": reason})
    defaults = []
    for unit, options, reason in DEFAULT_OPTIONS:
        e = units.get(fold(unit))
        have = {o["name"] for o in (e or {}).get("options", [])}
        if not e or not set(options) <= have:
            bad.append(f"defaultOptions: {unit!r} has no vanilla option(s) {options}")
        defaults.append({"unit": unit, "options": options, "reason": reason})
    markers = []
    for unit, option, reason in MARKER_OPTIONS:
        e = units.get(fold(unit))
        if not e or not any(o["name"] == option for o in e.get("options", [])):
            bad.append(f"markerOptions: {unit!r} has no vanilla option {option!r}")
        markers.append({"unit": unit, "option": option, "reason": reason})
    if bad:
        raise SystemExit("data.ttsModels: " + "; ".join(bad))
    return {"text": TEXT, "standIns": stand_ins, "mounts": mounts,
            "riders": sorted(riders, key=lambda r: r["unit"]), "names": opts,
            "defaultOptions": defaults, "markerOptions": markers}
