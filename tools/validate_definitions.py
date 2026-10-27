#!/usr/bin/env python3
"""Validates the definitions files: JSON Schema (if `jsonschema` is installed) plus cross references.

    python3 tools/validate_definitions.py [--vanilla ...] [--battle-companies ...]

Cross references checked: every unit/keyword/magical power/gear a Battle Companies entry points at must exist in the
vanilla file or be defined by the Battle Companies file itself. Names flagged `missingInVanilla` by the generator are
reported as warnings (they come from other books); anything else unresolved is an error. Exit code 1 on errors.
"""
import argparse
import json
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
import company_text_checks  # noqa: E402
import concept_graph  # noqa: E402
import glossary_reference  # noqa: E402
from bc.gear import ABILITY_KEYS, CLASSES, ITEM_ALIASES, RANGED_CLASSES, item_key, resolve_rule, _keyword_names  # noqa: E402
from bc.refs import referenced, vanilla_duplicates  # noqa: E402
from bc.text import fold  # noqa: E402

PROFILE_STATS = ["movement", "fight", "shoot", "strength", "defence", "attack", "wounds", "courage", "intelligence", "might", "will", "fate"]
ROOT = Path(__file__).resolve().parent.parent


def schema_errors(doc, schema_path):
    try:
        import jsonschema
    except ImportError:
        print(f"  (skipping JSON Schema for {schema_path.name}: pip install jsonschema)")
        return []
    validator = jsonschema.Draft202012Validator(json.load(open(schema_path, encoding="utf-8")))
    return [f"{'/'.join(map(str, e.absolute_path))}: {e.message[:200]}" for e in validator.iter_errors(doc)]


TTS_CATALOGUE = ROOT / "sources/tts/models.json"  # local, gitignored (bead zrs.4); never committed
TTS_MAX_ENTRIES = 100  # data.ttsModels holds fixed mappings only: a catalogue (1700 keys) must never end up in the repository


def tts_catalogue(path):
    """The local catalogue's model keys ('Name: options'), or None when the file does not exist. A list of keys or {'models': [...]}."""
    if not Path(path).exists():
        return None
    raw = json.load(open(path, encoding="utf-8"))
    keys = raw["models"] if isinstance(raw, dict) else raw
    return {(fold(k.partition(":")[0]), fold(k.partition(":")[2])) for k in keys}


def tts_errors(bc, van, catalogue=None):
    """Checks data.ttsModels: names exist in the vanilla file; in the local catalogue when there is one; every wanderer, creature,
    fighting mount and cavalry unit a company uses has its mapping; every used unit has a model (1:1 vanilla name or a mapping)."""
    t = bc["data"].get("ttsModels")
    if t is None:
        return ["data.ttsModels is missing"]
    errors = []
    d = van["data"]
    unit_names = {fold(e["name"]) for k in ("heroes", "warriors") for e in d[k]}
    all_names = unit_names | {fold(e["name"]) for e in d["gear"]}
    units = {}
    for k in ("heroes", "warriors"):
        for e in reversed(d[k]):
            units[fold(e["name"])] = e

    def base(m):
        return m.split(":")[0].strip()

    def model(m, where):
        if fold(base(m)) not in all_names:
            errors.append(f"ttsModels {where}: {m!r} is not a name in the vanilla file")
        if catalogue is not None and not any(b == fold(base(m)) and (":" not in m or o == fold(m.partition(":")[2])) for b, o in catalogue):
            errors.append(f"ttsModels {where}: {m!r} is not a model of the local catalogue {TTS_CATALOGUE.name}")

    entries = len(t["standIns"]) + len(t["mounts"]) + len(t["riders"]) + len(t["names"])
    if entries > TTS_MAX_ENTRIES:
        errors.append(f"ttsModels has {entries} entries: fixed mappings only, never the mod's catalogue")
    for kind, group in (("standIns", t["standIns"]), ("mounts", t["mounts"]), ("riders", t["riders"]), ("names", t["names"])):
        for x in group:
            for v in (x.get("model"), x.get("footModel"), *(x.get("alternatives") or [])):
                if v is not None and len(v) > 80:
                    errors.append(f"ttsModels {kind}: {v[:30]!r}... is not a short model name")
    for x in t["standIns"]:
        model(x["model"], f"standIn {x['name']}")
        for a in x.get("alternatives", []):
            model(a, f"standIn {x['name']} alternative")
    for x in t["mounts"]:
        model(x["model"], f"mount {x['gear']}")
        g = next((g for g in d["gear"] if fold(g["name"]) == fold(x["gear"])), None)
        if not g or not (g.get("attack") or 0) > 0:
            errors.append(f"ttsModels mount {x['gear']!r}: not vanilla gear with Attacks above 0")
    for x in t["riders"]:
        if fold(x["unit"]) not in unit_names:
            errors.append(f"ttsModels rider {x['unit']!r} is not a vanilla unit")
        elif "Cavalry" not in (units[fold(x["unit"])].get("unitType") or []):
            errors.append(f"ttsModels rider {x['unit']!r} is not a cavalry unit")
        if "footModel" in x:
            model(x["footModel"], f"rider {x['unit']} footModel")
    for x in t["names"]:
        e = units.get(fold(x["unit"]))
        if not e:
            errors.append(f"ttsModels name: {x['unit']!r} is not a vanilla unit")
        elif not any(o["name"] == x["from"] for o in e.get("options", [])):
            errors.append(f"ttsModels name: {x['unit']!r} has no vanilla option {x['from']!r}")
    for key in ("standIns", "mounts", "riders", "names"):
        seen = set()
        for x in t[key]:
            k = (x.get("name") or x.get("gear") or x.get("unit"), x.get("from"))
            if k in seen:
                errors.append(f"ttsModels {key}: duplicate entry {k[0]!r}")
            seen.add(k)

    # coverage: what the companies use
    stand = {x["name"] for x in t["standIns"]}
    for w in bc["data"]["wanderers"]["items"] + bc["data"]["creatures"]["items"]:
        if w["name"] not in stand:
            errors.append(f"ttsModels standIns: {w['name']!r} (wanderer or creature) has no stand-in")
    mounts = {fold(x["gear"]) for x in t["mounts"]}
    for g in bc["data"]["wargear"]["items"]:
        if g.get("type") != "mount":
            continue
        for m in g["names"]:
            vg = next((v for v in d["gear"] if fold(v["name"]) == fold(m)), None)
            if vg and (vg.get("attack") or 0) > 0 and fold(m) not in mounts:
                errors.append(f"ttsModels mounts: the fighting mount {m!r} has no entry")
    riders = {fold(x["unit"]) for x in t["riders"]}
    used = referenced(bc, van)["units"]
    for u in sorted(used):
        e = units.get(fold(u))
        if e and "Cavalry" in (e.get("unitType") or []) and fold(u) not in riders:
            errors.append(f"ttsModels riders: the cavalry unit {u!r} has no entry")
    if catalogue is not None:
        from_names = {b for b, _ in catalogue}
        mapped = riders  # a rider is a mounted model of its own name
        for u in sorted(used):
            if fold(u) not in from_names and fold(u) not in mapped:
                errors.append(f"ttsModels: the unit {u!r} used by a company has no model in the local catalogue and no mapping")
    return errors


def need_units(bc, van):
    return referenced(bc, van)["units"]


def iter_path_options(paths):
    for p in paths:
        for row in p["progression"]:
            stack = list(row["options"])
            while stack:
                o = stack.pop()
                if o["type"] == "oneOf":
                    stack.extend(o["options"])
                yield p["name"], o


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--vanilla", default=str(ROOT / "definitions/vanilla/data2024.json"))
    ap.add_argument("--battle-companies", default=str(ROOT / "definitions/battle_companies/battle_companies.json"))
    ap.add_argument("--glossary", default=str(ROOT / "definitions/glossary/mesbg_glossary.json"))
    ap.add_argument("--tts-models", default=str(TTS_CATALOGUE), help="local catalogue of the mod's model keys (optional)")
    ap.add_argument("--concept-graph", default=str(ROOT / "definitions/concepts/concept_graph.json"))
    args = ap.parse_args()
    vp, bp = Path(args.vanilla), Path(args.battle_companies)
    van, bc = json.load(open(vp, encoding="utf-8")), json.load(open(bp, encoding="utf-8"))
    errors, warnings = [], []

    gp = Path(args.glossary)
    glossary = json.load(open(gp, encoding="utf-8")) if gp.exists() else None
    for doc, path in [(van, vp), (bc, bp)] + ([(glossary, gp)] if glossary else []):
        schema = path.with_suffix(".schema.json")
        if schema.exists():
            errors += [f"{path.name}: {e}" for e in schema_errors(doc, schema)]

    V = {k: {fold(e["name"]) for e in v} for k, v in van["data"].items()}
    units = V["warriors"] | V["heroes"]
    D = bc["data"]
    own_kw = {fold(k["name"]) for k in D["keywords"]}

    if bc["extends"] != van["game"]:
        errors.append(f"extends={bc['extends']!r} does not match vanilla game {van['game']!r}")

    # Battle Companies always wins over vanilla: a rule may redefine a vanilla one, but must say so with `overrides`.
    names = [k["name"] for k in D["keywords"]]
    for n in {n for n in names if names.count(n) > 1}:
        errors.append(f"duplicate BC keyword {n!r}")
    for k in D["keywords"]:
        if "overrides" in k:
            if fold(k["overrides"]) not in V["keywords"]:
                errors.append(f"BC keyword {k['name']!r} overrides unknown vanilla keyword {k['overrides']!r}")
        elif fold(k["name"]) in V["keywords"]:
            errors.append(f"BC keyword {k['name']!r} redefines a vanilla keyword without `overrides`")
    param_names = {fold(p["name"]) for p in D["parameterisedRules"]}
    for p in D["parameterisedRules"]:
        if not p["vanillaVariants"] and "overrides" not in p and p["name"] not in ("Terror",):
            warnings.append(f"parameterised rule {p['pattern']!r} has no vanilla variants")
        if p.get("overrides") and fold(p["overrides"]) not in V["keywords"]:
            errors.append(f"parameterised rule {p['pattern']!r} overrides unknown {p['overrides']!r}")
    vopts = {}
    for kind in ("warriors", "heroes"):
        for e in van["data"][kind]:
            vopts.setdefault(fold(e["name"]), set()).update(fold(o["name"]) for o in e.get("options", []))

    def unit_ok(name):
        return fold(name) in units

    def walk(o, where):
        if isinstance(o, dict):
            if "unit" in o and isinstance(o["unit"], str) and not unit_ok(o["unit"]):
                (warnings if o.get("missingInVanilla") else errors).append(f"{where}: unit {o['unit']!r} not in vanilla")
            for rule in o.get("addSpecialRules", []):
                if fold(rule) not in V["keywords"] and fold(rule) not in own_kw:
                    errors.append(f"{where}: added special rule {rule!r} unknown")
            for opt in o.get("selectedOptions", []):
                if fold(opt) not in vopts.get(fold(o.get("unit", "")), set()):
                    errors.append(f"{where}: {o.get('unit')!r} has no vanilla option {opt!r}")
            for k, v in o.items():
                walk(v, where)
        elif isinstance(o, list):
            for v in o:
                walk(v, where)

    for c in D["companies"]:
        walk(c, f"company {c['name']}")
        for u in c["heroUpgrades"]:
            if fold(u["specialRule"]) not in own_kw:
                errors.append(f"company {c['name']}: hero upgrade rule {u['specialRule']!r} undefined")
        for key in ("ruleSubstitutions", "grantedSpecialRules"):
            for r in c.get(key, []):
                rule = r.get("specialRule")
                if rule and fold(rule) not in V["keywords"] and fold(rule) not in own_kw:
                    errors.append(f"company {c['name']}: {key} rule {rule!r} unknown")
        for tag in ("reinforcements", "specialChart"):
            if tag in c:
                rolls = sorted(r for row in c[tag]["table"] for r in range(row["roll"][0], row["roll"][1] + 1))
                if rolls != list(range(1, 7)):
                    errors.append(f"company {c['name']}: {tag} rolls do not cover 1-6 exactly once: {rolls}")

    for pname, o in iter_path_options(D["paths"]):
        if o["type"] == "specialRule" and fold(o["specialRule"]) not in own_kw and fold(o["specialRule"]) not in V["keywords"]:
            errors.append(f"{pname}: special rule {o['specialRule']!r} defined nowhere")
    for p in D["paths"]:
        if p.get("kind", "heroic") == "heroic" or (p.get("kind") == "custom" and p.get("implemented")):
            rolls = [r["roll"] for r in p["progression"]]
            if rolls != list(range(2, 13)):
                errors.append(f"path {p['name']}: progression rolls must be 2..12, got {rolls}")
            if p.get("dice") != "2d6":
                errors.append(f"path {p['name']}: missing dice")
            if not p.get("maximums"):
                errors.append(f"path {p['name']}: no maximums")
            if p.get("kind") == "custom" and not (p.get("startingStats") and p.get("givesExperience") and p.get("canLevelUp")):
                errors.append(f"path {p['name']}: custom path needs startingStats, givesExperience and canLevelUp")
            for r in p["progression"]:
                if not r["options"]:
                    errors.append(f"path {p['name']} roll {r['roll']}: no options")
                for o in r["options"]:
                    if o["type"] == "improveStat" and not set(o["stats"]) <= set(PROFILE_STATS):
                        errors.append(f"path {p['name']} roll {r['roll']}: unknown stat in {o['stats']}")
    for p in D["paths"]:
        for s in p.get("spellList", []):
            if fold(s["magicalPower"]) not in V["magicalPowers"]:
                errors.append(f"{p['name']}: magical power {s['magicalPower']!r} not in vanilla")
    for m in D["minorSpecialRules"]:
        r = fold(m["specialRule"])
        if r not in V["keywords"] and r not in own_kw and r not in param_names:
            errors.append(f"minor special rule {m['specialRule']!r} defined nowhere")
        if m.get("parameterKind") and r not in param_names:
            errors.append(f"minor special rule {m['specialRule']!r} has a parameter but no parameterised rule")
    for w in D["wanderers"]["items"]:
        for r in w.get("specialRules", []):
            if fold(r) not in V["keywords"] and fold(r) not in own_kw and "(" not in r:
                errors.append(f"wanderer {w['name']}: rule {r!r} unknown")
        for p in w.get("magicalPowers", []):
            if fold(p["magicalPower"]) not in V["magicalPowers"]:
                errors.append(f"wanderer {w['name']}: power {p['magicalPower']!r} unknown")
    for e in D["equipment"]["items"]:
        for g in e.get("grantsSpecialRules", []):
            if fold(g) not in V["keywords"]:
                errors.append(f"equipment {e['name']}: grants unknown rule {g!r}")
    # ---- gear annotations ---------------------------------------------------------------------------------------------
    ga = D["gearAnnotations"]["items"]
    kwn, params = _keyword_names(bc, van), [p["name"] for p in D["parameterisedRules"]]
    seen_keys = {}
    for it in ga:
        n, cl = it["name"], it["classes"]
        for spelling in [n] + it.get("aliases", []):
            k = item_key(spelling)
            if k in seen_keys and seen_keys[k] != n:
                errors.append(f"gear annotation {n!r}: {spelling!r} is also covered by {seen_keys[k]!r}")
            seen_keys[k] = n
        for g in it.get("grants", []):
            if not resolve_rule(g, kwn, params):
                errors.append(f"gear annotation {n!r}: grant {g!r} is neither a vanilla keyword nor a Battle Companies rule")
        ranged_class = bool(set(cl) & set(RANGED_CLASSES))
        if "ranged" in it and not ranged_class:
            errors.append(f"gear annotation {n!r}: ranged is only for {', '.join(RANGED_CLASSES)}")
        if ranged_class and "ranged" not in it:
            errors.append(f"gear annotation {n!r}: class {cl[0]} needs a ranged profile")
        if "other" in cl and len(cl) > 1:
            errors.append(f"gear annotation {n!r}: other is never combined with another class")
        if "spear" in cl and "melee" in cl:
            errors.append(f"gear annotation {n!r}: every spear is melee, do not list both")
        if "grantedTo" in it and cl[0] != "mount":
            errors.append(f"gear annotation {n!r}: grantedTo is only for items whose main class is mount")
        if "grantedTo" not in it and cl[0] == "mount" and it.get("grants"):
            errors.append(f"gear annotation {n!r}: a mount's rules belong to the mount (grantedTo: mount)")
    # R8: a stat-raising ability says whether it is conditional (and then why); armour stat changes are unconditional
    for a in D["gearAnnotations"]["items"]:
        for ab in a.get("abilities", []):
            if ab["key"] in ("defenceBonus", "attacksBonus"):
                if not isinstance(ab.get("conditional"), bool) or (ab["conditional"] and not ab.get("condition", "").strip()):
                    errors.append(f"gear annotation {a['name']!r}: {ab['key']} needs conditional (bool) and, when true, a condition text")
                situational = ab["key"] == "attacksBonus" or bool(ab.get("notWhileEngaged") or ab.get("against"))
                if bool(ab.get("conditional")) != situational:
                    errors.append(f"gear annotation {a['name']!r}: {ab['key']} is conditional exactly when it is situational (Engaged, Supported, against Strikes)")
        for sc in a.get("statChanges", []):
            if sc.get("conditional") is not False:
                errors.append(f"gear annotation {a['name']!r}: statChanges must carry conditional false (armour is unconditional)")
    ts = next((x for x in D["gearAnnotations"]["items"] if x["name"] == "Troll Spear"), None)
    if not ts or ts["classes"] != ["spear"] or "doubleWoundOnNatural6" not in {x["key"] for x in ts.get("abilities", [])}:
        errors.append("Troll Spear must be an own spear item with the doubleWoundOnNatural6 ability")
    for a in D["gearAnnotations"]["items"]:  # ability vocabulary: known keys, real class names in the class lists
        for ab in a.get("abilities", []):
            if ab["key"] not in ABILITY_KEYS:
                errors.append(f"gear annotation {a['name']!r}: unknown ability key {ab['key']!r}")
            for lst in ("with", "cancelledByClasses"):
                for c in ab.get(lst, []):
                    if c not in CLASSES:
                        errors.append(f"gear annotation {a['name']!r}: ability {ab['key']} {lst} names {c!r}, not a gear class")
    # Coverage, written independently of the generator's collect(): walk the units the companies name, the options and
    # the Battle Companies tables again and check that every item has an annotation.
    vgear = {}
    for g in van["data"]["gear"]:
        vgear.setdefault(item_key(g["name"]), g)
    cunits = set()

    def find_units(o):
        if isinstance(o, dict):
            if isinstance(o.get("unit"), str):
                cunits.add(fold(o["unit"]))
            for v in o.values():
                find_units(v)
        elif isinstance(o, list):
            for v in o:
                find_units(v)

    find_units(D["companies"])
    find_units(D["wanderers"])
    wanted = {}

    def want(text, where):
        low = text.lower()
        if "(hand weapon" in low:
            wanted[item_key(text)] = (text, where)
            return
        g = vgear.get(item_key(text))
        if g and g.get("includes"):
            for inc in g["includes"]:
                want(inc["name"], where)
            return
        m = re.match(r"^(?:exchange|swap|replace)\s+.+?\s+(?:for|with)\s+(.+)$", text.strip(), re.I)
        body = m[1] if m else text
        parts = [x for x in re.split(r"\s*(?:,\s*and\s+|,|\s+and\s+|&)\s*", body.strip()) if x]
        if len(parts) > 1 or m:
            for x in parts:
                want(x, where)
        else:
            wanted[item_key(text)] = (text, where)

    for kind in ("warriors", "heroes"):
        for e in van["data"][kind]:
            if fold(e["name"]) in cunits or any(fold(e["name"]) == fold(r) for r in need_units(bc, van)):
                for w in e.get("wargear", []):
                    want(w, f"unit {e['name']}")
                for o in e.get("options", []):
                    want(o["name"], f"unit {e['name']} option")
                for inc in e.get("includes") or []:  # a bundled unit: the gear of its members
                    for m in van["data"][inc["dataSource"]]:
                        if m["name"] == inc["name"]:
                            for w in m.get("wargear", []):
                                want(w, f"unit {e['name']} (includes {inc['name']})")
                            for o in m.get("options", []):
                                want(o["name"], f"unit {e['name']} (includes {inc['name']}) option")
    for e in D["wargear"]["items"]:
        for nm in e["names"]:
            wanted[item_key(nm)] = (nm, "BC wargear table")
    for e in D["equipment"]["items"]:
        wanted[item_key(e["name"])] = (e["name"], "BC equipment")
    for c in D["companies"]:
        for e in c.get("uniqueWargear", []):
            wanted[item_key(e["name"])] = (e["name"], f"unique wargear of {c['name']}")
    for stage in D["wargear"]["armourUpgrade"]["stages"]:
        for nm in stage:
            if nm != "None":
                want(nm, "BC armour stages")
    for k, (text, where) in wanted.items():
        key = item_key(ITEM_ALIASES.get(k, text))
        if key not in seen_keys:
            errors.append(f"gear item {text!r} ({where}) has no annotation")
    annotated = set(seen_keys)
    # gear identity: every item keeps its own identity (written independently of the generator; regression for rzt.2 / rzt.3)
    by_key = {}
    for it in ga:
        for nm in [it["name"]] + it.get("aliases", []):
            k = item_key(nm)
            if by_key.setdefault(k, it["name"]) != it["name"]:
                errors.append(f"gear identity: {nm!r} is a spelling of both {by_key[k]!r} and {it['name']!r}")
    seen_group = {}
    for row in D["wargear"]["items"]:  # a row is a price group: its names are distinct items
        owners = [by_key.get(item_key(nm)) or by_key.get(item_key(ITEM_ALIASES.get(item_key(nm), nm))) for nm in row["names"]]
        for nm, o in zip(row["names"], owners):
            if o is None:
                errors.append(f"wargear table: {nm!r} has no annotation of its own")
        if len({o for o in owners if o}) != len([o for o in owners if o]):
            errors.append(f"wargear table row {row['names']}: two names are the same item (a price group is not an item)")
        for o in owners:
            if o and seen_group.setdefault(o, tuple(row["names"])) != tuple(row["names"]):
                errors.append(f"wargear table: {o!r} is in two rows")
    ga_by_name = {it["name"]: it for it in ga}
    table_keys = {item_key(ITEM_ALIASES.get(item_key(nm), nm)) for row in D["wargear"]["items"] for nm in row["names"]}
    for it in ga:
        for r in it.get("relatedTo", []):
            other = ga_by_name.get(r["name"])
            if other is None:
                errors.append(f"gear {it['name']!r}: relatedTo {r['name']!r} is not an item")
            elif other is it or item_key(r["name"]) == item_key(it["name"]):
                errors.append(f"gear {it['name']!r}: related to itself")
            elif it["name"] not in [x["name"] for x in other.get("relatedTo", [])]:
                errors.append(f"gear {it['name']!r}: relatedTo {r['name']!r} is not symmetric")
        for field in ("variantOf", "pricedAs"):
            tgt = it.get(field)
            if tgt and item_key(tgt) not in by_key:
                errors.append(f"gear {it['name']!r}: {field} {tgt!r} is not an item")
            if tgt and item_key(tgt) == item_key(it["name"]):
                errors.append(f"gear {it['name']!r}: {field} itself")
        if it.get("pricedAs") and item_key(it["pricedAs"]) not in table_keys:
            errors.append(f"gear {it['name']!r}: pricedAs {it['pricedAs']!r} is not in the wargear table")
        ab = {a["key"]: a for a in it.get("abilities", [])}
        if "bow" in it["classes"] and ("moveAndShoot" not in ab or "half its Move Value" not in it.get("definition", "")):
            errors.append(f"gear {it['name']!r}: a bow may move up to half its Move and shoot (moveAndShoot, and the text says so)")
        if "crossbow" in it["classes"]:
            if "noFireAfterMove" not in ab or "moveAndShoot" in ab:
                errors.append(f"gear {it['name']!r}: a crossbow cannot fire after moving")
            if it.get("ranged") != {"range": 24, "strength": 4}:
                errors.append(f"gear {it['name']!r}: a crossbow is 24\" S4 (vanilla and the rulebook's Missile Weapon chart)")
    # item modifiers: an item is a base item plus modifiers; pricedAs / variantOf are derived from them
    mods = {m["key"]: m for m in D["gearAnnotations"]["modifiers"]}
    if len(mods) != len(D["gearAnnotations"]["modifiers"]):
        errors.append("gearAnnotations.modifiers: duplicate key")
    gterms = {t["name"] for t in glossary["terms"]} if glossary else None
    for m in mods.values():
        if not m["text"].startswith(m.get("tooltip", "")):
            errors.append(f"modifier {m['key']!r}: tooltip is a prefix of text")
        if gterms is not None and m.get("glossary") and m["glossary"] not in gterms:
            errors.append(f"modifier {m['key']!r}: glossary term {m['glossary']!r} does not exist")
    for it in ga:
        nm = it["name"]
        if ("base" in it) != ("modifiers" in it):
            errors.append(f"gear {nm!r}: base and modifiers go together")
        if "base" in it:
            if item_key(it["base"]) not in by_key or item_key(it["base"]) == item_key(nm):
                errors.append(f"gear {nm!r}: base {it['base']!r} is not another item")
            for k in it["modifiers"]:
                if k not in mods:
                    errors.append(f"gear {nm!r}: unknown modifier {k!r}")
            kinds = {mods[k]["identity"] for k in it["modifiers"] if k in mods}
            if ("variant" in kinds) != (it.get("variantOf") == it["base"]):
                errors.append(f"gear {nm!r}: variantOf must be the base exactly when a modifier has identity 'variant'")
            if it.get("pricedAs") and (it["pricedAs"] != it["base"] or "elven" not in it["modifiers"]):
                errors.append(f"gear {nm!r}: pricedAs is the base of an Elven item")
        else:
            for field in ("pricedAs", "variantOf"):
                if it.get(field):
                    errors.append(f"gear {nm!r}: {field} is derived from base + modifiers")
        g = it.get("grantsRuleForChosenWeapon")
        if g:
            if not it.get("weaponModifier") or f"{g['rule']} (X)" not in it.get("grants", []):
                errors.append(f"gear {nm!r}: grantsRuleForChosenWeapon goes with weaponModifier and the grant '{g['rule']} (X)'")
            if not g["weaponClasses"] or any(c not in ("melee", "two_handed", "spear", "lance", "bow", "crossbow", "throwing", "missile") for c in g["weaponClasses"]):
                errors.append(f"gear {nm!r}: grantsRuleForChosenWeapon.weaponClasses must be weapon classes")
        elif it.get("weaponModifier"):
            errors.append(f"gear {nm!r}: a weapon modifier says which rule it gives the chosen weapon (grantsRuleForChosenWeapon)")
        if "poison" in nm.lower() and not it.get("weaponModifier"):
            errors.append(f"gear {nm!r}: poison is a rule of the unit (Poisoned Attacks (<weapon>)), not a wargear variant; only the Blowpipe itself carries it")
        if "elven" in it.get("modifiers", []) and not (nm.lower().startswith("elven") or "elf" in nm.lower() or "This is an Elven" in it.get("definition", "")):
            errors.append(f"gear {nm!r}: an Elven item is called Elven or its text says so")
    for need, depth in (("Pike", 2), ("Spear", 1)):
        a = ga_by_name.get(need)
        sup = [x for x in (a or {}).get("abilities", []) if x["key"] == "support"]
        if not a or not sup or sup[0].get("depth") != depth or ("twoHanded" in [x["key"] for x in a["abilities"]]) != (need == "Pike"):
            errors.append(f"gear {need!r}: a Pike is two-handed and supports two deep (through a second pike); a Spear supports one deep")
    for rd in D["wargearReadAs"]["items"]:
        if item_key(rd["item"]) not in by_key:
            errors.append(f"wargearReadAs: {rd['item']!r} is not an item")
        if "poison" in rd["written"].lower():  # poison belongs to the unit: its vanilla profile must carry Poisoned Attacks (<the item>)
            vu = next((x for k in ("warriors", "heroes") for x in van["data"][k] if x["name"] == rd["unit"]), None)
            want = f"poisoned attacks ({rd['item'].lower()})"
            if not vu or want not in [r.lower() for r in vu.get("specialRules", [])]:
                errors.append(f"wargearReadAs: {rd['unit']!r} reads {rd['written']!r} as a plain {rd['item']}, but its profile has no {want}")
    # limits: stable ids, exemptions
    limit_ids = [l["id"] for l in bc["limits"]]
    if len(set(limit_ids)) != len(limit_ids):
        errors.append("duplicate limit ids")
    ann_names = {fold(it["name"]) for it in ga}
    for c in D["companies"]:
        for key in c.get("limitOverrides", {}):
            if key not in limit_ids:
                errors.append(f"company {c['name']}: limitOverrides key {key!r} is not a limit id")
        for ex in c.get("limitExemptions", []):
            if ("unit" in ex) == ("item" in ex):
                errors.append(f"company {c['name']}: a limit exemption needs exactly one of unit / item")
            if "unit" in ex and fold(ex["unit"]) not in units:
                errors.append(f"company {c['name']}: exempt unit {ex['unit']!r} not in vanilla")
            if "item" in ex and item_key(ex["item"]) not in annotated:
                errors.append(f"company {c['name']}: exempt item {ex['item']!r} has no gear annotation")
            for lid in ex["limits"]:
                if lid not in limit_ids:
                    errors.append(f"company {c['name']}: exemption names unknown limit {lid!r}")
    errors += tts_errors(bc, van, tts_catalogue(args.tts_models))
    # every vanilla name that is defined more than once and needed here must have a recorded decision
    decided = {(a["kind"], a["name"]): a for a in D["vanillaAmbiguities"]}
    need = referenced(bc, van)
    for kind, groups in vanilla_duplicates(van).items():
        singular = kind if kind == "gear" else kind[:-1]
        for name in groups:
            if name in need[kind] and (singular, name) not in decided:
                errors.append(f"vanilla duplicate {kind} {name!r} is used but has no decision in data.vanillaAmbiguities")
    vdups = vanilla_duplicates(van)
    for (kind, name), a in decided.items():
        group = vdups.get("gear" if kind == "gear" else kind + "s", {})
        if a["status"] == "overridden":
            if kind == "keyword" and name not in names:
                errors.append(f"vanilla duplicate keyword {name!r} is marked overridden but Battle Companies does not define it")
        elif a["status"] in ("firstWins", "pinned"):
            if name not in group:
                errors.append(f"{kind} {name!r}: marked {a['status']} but vanilla does not define it twice")
            elif a["occurrences"] != len(group[name]):
                errors.append(f"{kind} {name!r}: vanilla now has {len(group[name])} occurrences, decision was taken for {a['occurrences']}")
            if a["status"] == "firstWins" and a.get("occurrence") != 1:
                errors.append(f"{kind} {name!r}: firstWins must use occurrence 1")
            if a["status"] == "pinned" and not 1 <= a.get("occurrence", 0) <= a["occurrences"]:
                errors.append(f"{kind} {name!r}: pinned occurrence out of range")
    scen = set(bc["scenarios"])
    for t in bc["campaign"]["scenarioSelection"]["tables"]:
        for s in t["scenarios"]:
            if s not in scen:
                errors.append(f"scenario table references unknown scenario {s!r}")
    # scenario objectives, rewards and the per-match input
    effects = {e["key"]: e for e in bc["campaign"]["aftermathEffects"]}
    power_names = {fold(p["name"]) for p in D["magicalPowers"]["items"]}
    # magical powers: every vanilla power has an entry; learnable ones are exactly the Sorcerer's Spell List with its range/casting
    for n in V["magicalPowers"]:
        if n not in power_names:
            errors.append(f"magical power {n!r} of vanilla has no data.magicalPowers entry")
    sorc = next(p for p in D["paths"] if p["name"] == "Path of the Sorcerer")
    learn = {fold(p["name"]): p for p in D["magicalPowers"]["items"] if p["learnable"]}
    listed = {fold(s["magicalPower"]): s for s in sorc["spellList"]}
    if set(learn) != set(listed):
        errors.append(f"learnable powers differ from the Sorcerer's Spell List: {sorted(set(learn) ^ set(listed))}")
    for k, s in listed.items():
        l = learn.get(k)
        if l and (l["casting"] != s["casting"] or l["range"] != ("self" if s["range"] == "Self" else s["range"])):
            errors.append(f"magical power {k!r}: range/casting differ from the Spell List")
    scoring = {s["name"]: s for s in bc["scenarioScoring"]}
    exp_keys = {g["key"] for g in bc["campaign"]["experience"]["gains"]}
    for sc in D["scenarioDetails"]:
        n = sc["name"]
        objs = sc.get("scoringObjectives", [])
        if len(objs) != len(sc.get("scoring", [])):
            errors.append(f"scenario {n}: {len(objs)} scoringObjectives for {len(sc.get('scoring', []))} scoring bullets")
        if len({o["key"] for o in objs}) != len(objs):
            errors.append(f"scenario {n}: duplicate scoringObjectives key")
        roles = {r["key"] for r in sc.get("roles", [])}
        for i, o in enumerate(objs):
            if o["side"] != "both" and o["side"] not in roles:
                errors.append(f"scenario {n}: objective {o['key']!r} is for {o['side']!r} but the scenario has no such role")
            opts = [x["points"] for x in o.get("options", [])]
            if o["kind"] in ("tiers", "eachTiers") and (not opts or opts != sorted(opts) or len(set(opts)) != len(opts)):
                errors.append(f"scenario {n}: objective {o['key']!r} tiers must be ascending and distinct")
            if o["kind"] == "each" and not ("pointsEach" in o and o.get("maxPoints")):
                errors.append(f"scenario {n}: objective {o['key']!r} needs pointsEach and maxPoints")
            if o["kind"] == "eachTiers" and "count" not in o:
                errors.append(f"scenario {n}: objective {o['key']!r} needs count")
            sp = scoring.get(n)  # the same points must appear in the vanilla-style scenarioScoring
            if sp and i < len(sp["points"]):
                pts = {t["points"] for t in sp["points"][i] if t["points"] > 0}
                if pts != (set(opts) if opts else {o.get("pointsEach")}):
                    errors.append(f"scenario {n}: objective {o['key']!r} points differ from scenarioScoring")
        for r in sc.get("rewards", []):
            e = effects.get(r["effect"])
            if not e or e.get("scenario") != n:
                errors.append(f"scenario {n}: reward {r['effect']!r} has no aftermathEffects entry for this scenario")
            if e and e.get("kind") == "magicalPower" and fold(e["power"]) not in power_names:
                errors.append(f"aftermath effect {e['key']!r}: unknown magical power {e['power']!r}")
    for e in bc["campaign"]["aftermathEffects"]:
        if e.get("scenario") and e["scenario"] not in scen:
            errors.append(f"aftermath effect {e['key']!r}: unknown scenario {e['scenario']!r}")
    mi = bc["campaign"]["matchInput"]
    got = {f.get("experience") for f in mi["perModel"] if f.get("experience")}
    if got != exp_keys:
        errors.append(f"matchInput.perModel experience fields {sorted(got ^ exp_keys)} do not match campaign.experience.gains")
    eq_names = {fold(e["name"]) for e in D["equipment"]["items"]}
    for b in bc["campaign"]["againstTheOdds"]["bonuses"]:
        for kit in b.get("kits", []):
            for it in kit["items"]:
                if fold(it["equipment"]) not in eq_names:
                    errors.append(f"kit {kit['name']}: unknown equipment {it['equipment']!r}")
    # equipment effects on the injury rolls (Healing Herbs, Wondrous Cram)
    camp = bc["campaign"]
    charts = {"warrior": camp["injuries"]["warrior"], "hero": camp["injuries"]["hero"]}
    lo = min(r["roll"][0] for r in charts["hero"]); hi = max(r["roll"][1] for r in charts["hero"])
    eq_by_name = {e["name"]: e for e in D["equipment"]["items"]}
    for e in camp["aftermathEffects"]:
        if e["kind"] not in ("injuryRollModifier", "injuryRollReplacement"):
            continue
        m = re.match(r"data\.equipment\.items\[name=(.+)\]$", e.get("source", ""))
        item = eq_by_name.get(m[1]) if m else None
        if not item:
            errors.append(f"aftermath effect {e['key']!r}: source {e.get('source')!r} is not an equipment item")
        elif item.get("aftermathEffect") != e["key"]:
            errors.append(f"equipment {item['name']!r}: aftermathEffect must be {e['key']!r}")
        if e["kind"] == "injuryRollModifier" and (e.get("clamp") != [lo, hi] or not isinstance(e.get("modifier"), int)):
            errors.append(f"aftermath effect {e['key']!r}: clamp must be the injury chart range [{lo}, {hi}] and modifier an integer")
        if e["kind"] == "injuryRollReplacement":
            for t, rows in charts.items():
                if e["result"] not in {r["key"] for r in rows}:
                    errors.append(f"aftermath effect {e['key']!r}: result {e['result']!r} is not on the {t} chart")
        if "hero" in (e["activatedBy"]["carrier"],) and not (item and "hero" in item.get("restrictions", [])):
            errors.append(f"aftermath effect {e['key']!r}: carrier is a Hero but the item is not Hero only")
    for e in D["equipment"]["items"]:
        if e.get("aftermathEffect") and e["aftermathEffect"] not in {x["key"] for x in camp["aftermathEffects"]}:
            errors.append(f"equipment {e['name']!r}: aftermathEffect {e['aftermathEffect']!r} unknown")
    # Against the Odds extras
    ato = {b["key"]: b for b in camp["againstTheOdds"]["bonuses"]}
    tk = ato["toolKit"]
    if tk["purchase"]["maxPurchases"] != len(tk["kits"]) or len({k["name"] for k in tk["kits"]}) != len(tk["kits"]):
        errors.append("toolKit: maxPurchases must equal the number of distinct kits")
    for k, kind in (("toolKit", "loanedEquipment"), ("wanderer", "temporaryModel"), ("ambush", "chooseScenario")):
        if ato[k].get("grants", {}).get("kind") != kind:
            errors.append(f"againstTheOdds {k}: grants.kind must be {kind!r}")
    if not ato["wanderer"].get("temporary") or ato["wanderer"]["grants"]["from"] != "data.wanderers.items":
        errors.append("againstTheOdds wanderer: temporary and from data.wanderers.items expected")
    # Break Point
    bp = camp["breakPoint"]
    if bp["default"]["percent"] != 50:
        errors.append("breakPoint.default.percent must be 50 (Rules Manual: half the starting models)")
    for co in D["companies"]:
        eff = [x for x in co.get("effects", []) if x["kind"] == "breakPoint"]
        if len(eff) != (1 if "breakPointPercentage" in co else 0) or any(x["percent"] != co.get("breakPointPercentage") or not 1 <= x["percent"] <= 100 for x in eff):
            errors.append(f"company {co['name']}: breakPoint effect and breakPointPercentage disagree")
    if bp["experience"]["gain"] != "campaign.experience.gains[key=brokenCourageTestPassed]" or "brokenCourageTestPassed" not in exp_keys:
        errors.append("breakPoint.experience.gain must name the brokenCourageTestPassed experience gain")
    scen_rules = {sc["name"]: {r["name"] if isinstance(r, dict) else r for r in sc.get("specialRules", [])} for sc in D["scenarioDetails"]}
    for x in bp["scenarioExceptions"]:
        if not x.get("text", "").strip():
            errors.append(f"breakPoint.scenarioExceptions: {x['effect']} / {x['who']} has no text")
        if "scenario" in x and x["rule"] not in scen_rules.get(x["scenario"], set()):
            errors.append(f"breakPoint.scenarioExceptions: scenario {x['scenario']!r} has no special rule {x['rule']!r}")
    if "noStandFast" not in {fx for r in charts["hero"] for fx in r["effects"]} or D["wanderers"]["rules"].get("providesStandFast") is not False:
        errors.append("breakPoint.whenBroken.standFast: Broken Honour / Wanderer no-Stand-Fast data missing")
    if "fledBroken" not in {f["key"] for f in mi["perCompany"]}:
        errors.append("matchInput.perCompany needs fledBroken (All is Lost)")
    use = next((f for f in mi["perModel"] if f["key"] == "usesItem"), None)
    if not use or set(use["options"]) != {e["key"] for e in camp["aftermathEffects"] if e["kind"] in ("injuryRollModifier", "injuryRollReplacement")}:
        errors.append("matchInput.perModel usesItem must list the injury-roll item effects")
    # reserved options: written independently of the generator (vanilla options whose availableIn names only heroes)
    heroes_v = {e["name"] for e in van["data"]["heroes"]}
    want = {(e["name"], o["name"]) for e in van["data"]["heroes"] + van["data"]["warriors"] for o in e.get("options", [])
            if o.get("availableIn") and all(isinstance(a, str) and a in heroes_v for a in o["availableIn"])}
    have = {(i["unit"], i["option"]) for i in D["reservedOptions"]["items"]}
    for u, o in sorted(want - have):
        errors.append(f"reservedOptions: {u} / {o} is reserved to named heroes in vanilla but not listed")
    for u, o in sorted(have - want):
        errors.append(f"reservedOptions: {u} / {o} is not a hero-only option of vanilla")
    # option sales: banner / war horn / war drum options are all listed; every entry names a real vanilla option
    vopt = {(e["name"], o["name"]) for e in van["data"]["heroes"] + van["data"]["warriors"] for o in e.get("options", [])}
    sales = {(i["unit"], i["option"]) for i in D["optionSales"]["items"]}
    banner_re = re.compile(r"\b(banner|war ?horn|war drum)\b", re.I)
    for u, o in sorted({k for k in vopt if banner_re.search(k[1])} - sales):
        errors.append(f"optionSales: {u} / {o} is a banner, war horn or war drum option but not listed")
    for u, o in sorted(sales - vopt):
        errors.append(f"optionSales: {u} / {o} is not an option of that vanilla unit")
    if len(sales) != len(D["optionSales"]["items"]):
        errors.append("optionSales: duplicate (unit, option)")
    # readAs options name a real Armoury item; profile wargear outside the Armoury is really outside it
    sold = {item_key(n) for e in D["wargear"]["items"] for n in e["names"]} | {item_key(e["name"]) for e in D["equipment"]["items"]}
    sold |= {item_key(u["name"]) for co in D["companies"] for u in co.get("uniqueWargear", [])}
    for i in D["optionSales"]["items"]:
        if (i["status"] == "readAs") != ("armouryItem" in i):
            errors.append(f"optionSales: {i['unit']} / {i['option']}: armouryItem goes with status readAs only")
        if (i["status"] == "ownItem") != ("item" in i):
            errors.append(f"optionSales: {i['unit']} / {i['option']}: item goes with status ownItem only")
        if i.get("item") and item_key(i["item"]) not in {item_key(a["name"]) for a in D["gearAnnotations"]["items"]}:
            errors.append(f"optionSales: {i['unit']} / {i['option']}: ownItem {i['item']!r} has no gear annotation")
        if i.get("armouryItem") and item_key(i["armouryItem"]) not in sold:
            errors.append(f"optionSales: {i['unit']} / {i['option']}: {i['armouryItem']!r} is not an Armoury item")
    vwar = {}
    for e in van["data"]["heroes"] + van["data"]["warriors"]:
        vwar.setdefault(e["name"], e.get("wargear", []))
    for i in D["profileWargearNotSold"]["items"]:
        if item_key(i["item"]) in sold:
            errors.append(f"profileWargearNotSold: {i['item']!r} is sold by an Armoury")
        for u in i["units"]:
            if i["item"] not in vwar.get(u, []):
                errors.append(f"profileWargearNotSold: {u} has no profile wargear {i['item']!r}")
    ann_names = {item_key(g["name"]) for g in D["gearAnnotations"]["items"]}
    for g in D["gearAnnotations"]["items"]:
        if "isA" in g and (item_key(g["isA"]) not in ann_names or item_key(g["isA"]) == item_key(g["name"])):
            errors.append(f"gear {g['name']}: isA {g['isA']!r} is not another annotated item")
    # requiresMount: every pure lance needs a mount, a mount never does, the War spear (spear + lance) does not
    for g in D["gearAnnotations"]["items"]:
        cl = g["classes"]
        if g.get("requiresMount") and cl[0] == "mount":
            errors.append(f"gear {g['name']}: a mount cannot require a mount")
        if cl == ["lance"] and not g.get("requiresMount"):
            errors.append(f"gear {g['name']}: a lance needs requiresMount")
        if "spear" in cl and g.get("requiresMount"):
            errors.append(f"gear {g['name']}: usable on foot as a spear, must not require a mount")
    # stat-modifier abilities: the rules that lift a cancellation are vanilla rules
    for g in D["gearAnnotations"]["items"]:
        for ab in g.get("abilities", []):
            names = [l["rule"] for l in ab.get("liftedByRules", [])] + list(ab.get("unlessRules", []))
            for n in names:
                if fold(n) not in V["keywords"]:
                    errors.append(f"gear {g['name']}: ability {ab['key']} names unknown rule {n!r}")
    # rule key / label: `name` is the key; a label that differs is only used with related rules sharing it
    bc_keys = {k["name"] for k in D["keywords"]}
    for k in D["keywords"]:
        if "label" in k and k["label"] == k["name"]:
            errors.append(f"BC keyword {k['name']!r}: label equals the key, drop it")
        for r in k.get("relatedTo", []):
            if r not in bc_keys and fold(r) not in V["keywords"]:
                errors.append(f"BC keyword {k['name']!r}: relatedTo {r!r} is not a rule")
        if "relatedTo" in k and "label" not in k:
            errors.append(f"BC keyword {k['name']!r}: relatedTo without a label (related rules share a label)")
    # restrictions by unit identity: exact vanilla units that the company really uses
    vnames = {e["name"] for e in van["data"]["heroes"] + van["data"]["warriors"]}
    for co in D["companies"]:
        used = set()
        def _u(o):
            if isinstance(o, dict):
                if isinstance(o.get("unit"), str):
                    used.add(o["unit"])
                for v in o.values():
                    _u(v)
            elif isinstance(o, list):
                for v in o:
                    _u(v)
        _u({k: co.get(k) for k in ("startingCompany", "reinforcements", "specialChart", "specialUnits", "advancements")})
        for it in co.get("uniqueWargear", []) + co.get("heroUpgrades", []):
            for u in it.get("restrictedToUnits", []):
                if u not in vnames:
                    errors.append(f"{co['name']}: {it['name']}: restrictedToUnits names unknown unit {u!r}")
                elif u not in used:
                    errors.append(f"{co['name']}: {it['name']}: restrictedToUnits {u!r} is not a unit of the company")
        for it in co.get("uniqueWargear", []):
            if "restriction" in it and "restrictedToUnits" not in it and "restrictedToKind" not in it:
                errors.append(f"{co['name']}: unique wargear {it['name']} has a restriction word but no restrictedToUnits")
    # secret objectives: complete modelling, and the nomination matches the instruction
    for sc in D["scenarioDetails"]:
        for o in sc.get("scoringObjectives", []):
            if o.get("secret"):
                if o.get("reveal") != "endOfGame" or o.get("completion") != o["kind"] or "instruction" not in o:
                    errors.append(f"scenario {sc['name']}: secret objective {o['key']} lacks reveal/completion/instruction")
                text = o["instruction"].lower()
                want = ("enemyHeroes" if "opponent's hero" in text else "ownHeroes" if "your own hero" in text
                        else "terrain" if "terrain piece" in text else None)
                if (o.get("nomination") or {}).get("from") != want:
                    errors.append(f"scenario {sc['name']}: secret objective {o['key']}: nomination {o.get('nomination')} does not match its instruction")
            elif any(k in o for k in ("nomination", "reveal", "completion")):
                errors.append(f"scenario {sc['name']}: {o['key']} is not secret but has secret fields")
    # scenario tables: rows cover their range without gaps or overlaps, d6 tables cover 1-6
    for sc in D["scenarioDetails"]:
        for r in sc.get("specialRules", []):
            t = r.get("table")
            if not t:
                continue
            prev = None
            for row in t["rows"]:
                if len(row["cells"]) != len(t["columns"]):
                    errors.append(f"scenario {sc['name']}: table of {r.get('name')} has a row with the wrong number of cells")
                if prev is not None and (prev is None or row["min"] != prev + 1):
                    errors.append(f"scenario {sc['name']}: table of {r.get('name')} has a gap or overlap at {row['min']}")
                prev = row["max"] if row["max"] is not None else prev
            if t["lookup"] == "d6" and (t["rows"][0]["min"], t["rows"][-1]["max"]) != (1, 6):
                errors.append(f"scenario {sc['name']}: d6 table of {r.get('name')} does not cover 1-6")
            if t["lookup"] == "combinedRating" and t["rows"][-1]["max"] is not None:
                errors.append(f"scenario {sc['name']}: the last row of the rating table must be open ended")
    # unit fill-ins only fill fields that vanilla leaves empty
    vunits = {e["name"]: e for e in van["data"]["heroes"] + van["data"]["warriors"]}
    for i in D["unitFillIns"]["items"]:
        e = vunits.get(i["unit"])
        if e is None:
            errors.append(f"unitFillIns: unknown vanilla unit {i['unit']!r}")
            continue
        for f in ("race", "faction", "unitType", "baseSize"):
            if f in i and e.get(f):
                errors.append(f"unitFillIns: {i['unit']} already has {f} in vanilla")
    vgear = {g["name"]: g for g in van["data"]["gear"]}
    for i in D["mountFillIns"]["items"]:
        g = vgear.get(i["gear"])
        if g is None or g.get("type") != "mount":
            errors.append(f"mountFillIns: {i['gear']!r} is not a vanilla mount")
            continue
        for f in ("race", "faction", "unitType"):
            if f in i and g.get(f):
                errors.append(f"mountFillIns: {i['gear']} already has {f} in vanilla")
    # injuries: the treatment and the structured effects must point at real chart entries / gear classes
    inj = bc["campaign"]["injuries"]
    hero = {e["key"]: e for e in inj["hero"]}
    gear_names = {fold(g["name"]) for g in D["gearAnnotations"]["items"]}
    for e in inj["hero"] + inj["warrior"]:
        for d in e.get("disables", []):
            for c in d.get("classes", []):
                if c not in CLASSES:
                    errors.append(f"injury {e['key']}: unknown gear class {c!r}")
            for n in d.get("names", []):
                if fold(n) not in gear_names:
                    errors.append(f"injury {e['key']}: unknown gear item {n!r}")
        for k in e.get("heals", {}).get("keys", []):
            if not hero.get(k, {}).get("healable"):
                errors.append(f"injury {e['key']}: heals {k!r}, which is not a healable hero injury")
    for opt in inj["treatment"]["hero"]:
        for k in opt["healable"]:
            e = hero.get(k, {})
            persistent = e.get("persistent") and e.get("healable")
            status = "missNextGame" in e.get("effects", []) and e.get("healable") and opt["key"] in e.get("healableBy", [])
            if not (persistent or status):
                errors.append(f"treatment {opt['key']}: {k!r} is not a healable hero injury or a status healable by this method")
    vmounts = {fold(g["name"]) for g in van["data"]["gear"] if g.get("type") == "mount"}
    for a in D["gearAnnotations"]["items"]:
        if "profile" in a and fold(a["profile"]["name"]) not in vmounts:
            errors.append(f"gear annotation {a['name']}: mount profile {a['profile']['name']!r} is not a vanilla mount")
        if a["classes"][0] == "mount" and "profile" not in a:
            warnings.append(f"mount {a['name']!r} has no stat block (profile)")
    kinds = set(re.findall(r"^  (\w+)[{ ]", open(ROOT / "tools/bc/company_effects.py").read().split('Vocabulary')[1].split('"""')[0], re.M))
    bckw = {k["name"]: k for k in D["keywords"]}
    for co in D["companies"]:
        for e in co.get("effects", []):
            if e["kind"] not in kinds:
                errors.append(f"{co['name']}: unknown effect kind {e['kind']!r}")
            if e["kind"] == "heroUpgrade" and not bckw.get(e["specialRule"], {}).get("definition"):
                errors.append(f"{co['name']}: hero upgrade {e['name']!r} has no rule text")
    path_names = {x["name"]: x for x in D["paths"]}
    vinc = {fold(w["name"]): [fold(i["name"]) for i in w.get("includes", [])] for w in van["data"]["warriors"]}
    for co in D["companies"]:
        for e in co["effects"]:
            k = e["kind"]
            if k == "onTheirOwnPath" and e.get("path") not in path_names:
                errors.append(f"{co['name']}: onTheirOwnPath path {e.get('path')!r} is not in data.paths")
            if k == "includedUnitPoints":
                if sorted(fold(m["name"]) for m in e["members"]) != sorted(vinc.get(fold(e["unit"]), [])):
                    errors.append(f"{co['name']}: includedUnitPoints members differ from vanilla includes of {e['unit']!r}")
            if k in ("unitTypes", "heroPromotion", "modelCount", "recruitFallback") and fold(e.get("unit", e.get("from", ""))) not in units:
                errors.append(f"{co['name']}: {k} unit {e.get('unit', e.get('from'))!r} not in vanilla")
            if k == "heroPromotion" and fold(e["becomes"]) not in units:
                errors.append(f"{co['name']}: heroPromotion becomes unknown unit {e['becomes']!r}")
            if k == "compositeModel":
                for part in e["parts"]:
                    if "unit" in part and fold(part["unit"]) not in units:
                        errors.append(f"{co['name']}: compositeModel part {part['unit']!r} not in vanilla")
    for o in bc["campaign"]["options"]:
        if "swaps" in o and o["swaps"]["path"] not in path_names:
            errors.append(f"campaign option {o['key']}: swaps unknown path {o['swaps']['path']!r}")
    for x in D["paths"]:
        if x.get("specialRule") and x["specialRule"] not in bckw:
            errors.append(f"path {x['name']}: special rule {x['specialRule']!r} has no keyword")
        if x.get("kind") in ("endOfPath", "custom") and (x.get("selectable") or x["progression"] and x["kind"] == "endOfPath"):
            errors.append(f"path {x['name']}: end-of-path / custom paths are not selectable and (end of path) have no progression")
    # exact unit names: every unit a company names must be spelled like the vanilla unit or like a company unitAlias label
    exact = {w["name"] for k in ("warriors", "heroes") for w in van["data"][k]}
    for co in D["companies"]:
        labels = {a["label"] for a in co.get("unitAliases", [])}

        def exact_ok(n, where, co=co, labels=labels):
            if n not in exact and n not in labels:
                errors.append(f"{co['name']}: {where}: {n!r} is not an exact vanilla unit name or company unit alias")

        def walk_units(o, where):
            if isinstance(o, dict):
                if isinstance(o.get("unit"), str):
                    exact_ok(o["unit"], where)
                for v in o.values():
                    walk_units(v, where)
            elif isinstance(o, list):
                for x in o:
                    walk_units(x, where)
        for k in ("startingCompany", "reinforcements", "specialChart", "specialUnits", "advancements", "rareUnits"):
            walk_units(co.get(k, {}), k)
        for n in co.get("onTheirOwnPath", []):
            exact_ok(n, "onTheirOwnPath")
        for a in co.get("unitAliases", []):
            exact_ok(a["unit"], "unitAliases.unit")
        for e in co["effects"]:
            for key in ("unit", "from", "becomes"):
                if isinstance(e.get(key), str):
                    exact_ok(e[key], f"effect {e['kind']}.{key}")
            for key in ("units", "allowed", "excluded"):
                for n in e.get(key, []) if isinstance(e.get(key), list) else []:
                    exact_ok(n, f"effect {e['kind']}.{key}")
    for co in D["companies"]:
        aliases = {a["label"]: a for a in co.get("unitAliases", [])}
        for chart in ("reinforcements", "specialChart"):
            for row in co.get(chart, {}).get("table", []):
                r = row["result"]
                if r["type"] == "lostStartingUnit":
                    errors.append(f"{co['name']}: {chart} roll {row['roll']}: vague lostStartingUnit result, use unitChoice")
                if r["type"] == "unitChoice":
                    seen = set()
                    for o in r["options"]:
                        ident = (o["unit"], tuple(o.get("selectedOptions", [])))
                        if ident in seen:
                            errors.append(f"{co['name']}: unitChoice has {ident} twice")
                        seen.add(ident)
                        if not isinstance(o.get("rare"), int) or o["rare"] < 1:
                            errors.append(f"{co['name']}: unitChoice option {o['unit']!r} needs rare >= 1")
                        if o.get("label"):
                            a = aliases.get(o["label"])
                            if not a or a["unit"] != o["unit"] or a.get("selectedOptions", []) != o.get("selectedOptions", []):
                                errors.append(f"{co['name']}: unitChoice label {o['label']!r} does not match a company unit alias")
    fallback = bc["campaign"]["company"].get("oath", {})
    for al, o in fallback.items():
        if o["text"].count("{name}") != 1:
            errors.append(f"campaign.company.oath.{al}: {{name}} must appear exactly once")
    for co in D["companies"]:
        o = co.get("oath") or fallback.get(co["alignment"])
        if not o:
            errors.append(f"{co['name']}: no oath and no fallback for alignment {co['alignment']!r}")
            continue
        text = o["text"]
        if text.count("{name}") != 1 or text.startswith("{name}") or text.endswith("{name}") or text.rstrip(" .!").endswith("{name}"):
            errors.append(f"{co['name']}: oath must contain {{name}} exactly once, not at the start or end")
        if not 1 <= len(o["vow"]) <= 40:
            errors.append(f"{co['name']}: oath vow must be 1-40 characters")
    treat = [x for x in bc["campaign"]["influence"]["spend"] if x["key"] == "treatInjuries"]
    if not treat or "cost" not in treat[0]:
        errors.append("campaign.influence.spend[treatInjuries] needs a cost")
    if glossary:
        gnames = [t["name"] for t in glossary["terms"]]
        for n in {n for n in gnames if gnames.count(n) > 1}:
            errors.append(f"duplicate glossary term {n!r}")
        known = set(gnames)
        for t in glossary["terms"]:
            if not t["definition"].strip():
                errors.append(f"glossary term {t['name']!r} is empty")
            for target in t.get("see", []):
                if target not in known:
                    errors.append(f"glossary term {t['name']!r}: see -> {target!r} is not a term")

    if glossary:
        tw = glossary.get("reference", {}).get("toWound")
        if tw is None:
            errors.append("glossary: reference.toWound is missing")
        else:
            errors += glossary_reference.to_wound_errors(tw)
            if tw.get("term") not in known:
                errors.append(f"glossary reference.toWound: term {tw.get('term')!r} is not a term")

    errors += company_text_checks.company_text_errors(bc)

    cp = Path(args.concept_graph)
    if glossary and cp.exists():
        graph = json.load(open(cp, encoding="utf-8"))
        errors += [f"{cp.name}: {e}" for e in schema_errors(graph, cp.with_suffix(".schema.json"))]
        errors += [f"{cp.name}: {e}" for e in concept_graph.check_graph(graph, [Path(x).read_bytes() for x in (args.vanilla, args.battle_companies, args.glossary)])]
    elif glossary:
        errors.append(f"{cp} is missing: run tools/build_concept_graph.py")

    for w in sorted(set(warnings)):
        print("warning:", w)
    for e in errors:
        print("ERROR:", e)
    print(f"{len(errors)} error(s), {len(set(warnings))} warning(s)")
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
