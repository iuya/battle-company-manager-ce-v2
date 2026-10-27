#!/usr/bin/env python3
"""Builds definitions/battle_companies/battle_companies.json from the Battle Companies CE rules document.

The document is copyrighted community material that is NOT stored in git: export the Google Doc as plain text into
sources/battle_companies_rules.txt (see tools/README.md), then run:

    python3 tools/build_battle_companies.py

The generated file only complements definitions/vanilla/data2024.json: units, gear, keywords and magical powers that
already exist there are referenced by name instead of being redefined. A report of every name that could not be
resolved against the vanilla file is printed, so nothing is silently dropped.
"""
import argparse
import json
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from bc import core
from bc import auras, company_effects, magic, oaths, rule_targets, tts
from bc.gear import apply_read_as, build_gear_annotations, item_key, write_review
from bc.armoury import parse_creatures, parse_equipment, parse_wanderers
from bc.companies import parse_company
from bc.paths import add_offers, parse_paths
from bc.refs import referenced, vanilla_duplicates
from bc.scenarios import parse_scenario
from bc import scenario_rules
from bc.text import fold, norm, smart_title
from bc.armoury_gaps import profile_wargear_not_sold, write_report as write_gap_report
from bc.vanilla import Vanilla, reserved_options, option_sales, unit_fill_ins, mount_fill_ins

ROOT = Path(__file__).resolve().parent.parent


def find(lines, text, after=0):
    return next(i for i, l in enumerate(lines) if i > after and l.strip() == text)


def toc_names(lines, first):
    names = []
    for l in lines[first:first + 60]:
        m = re.match(r"^(.*?)\s{3,}\d+$", l.strip())
        if not m or m[1] == "SCENARIOS":
            break
        names.append(m[1])
    return names


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--src", default=str(ROOT / "sources/battle_companies_rules.txt"))
    ap.add_argument("--vanilla", default=str(ROOT / "definitions/vanilla/data2024.json"))
    ap.add_argument("--out", default=str(ROOT / "definitions/battle_companies/battle_companies.json"))
    args = ap.parse_args()

    src_text = open(args.src, encoding="utf-8").read().replace("\r", "")
    for wrong, right in core.SOURCE_TYPOS.items():  # printing errors of the document, listed in core.SOURCE_TYPOS
        src_text = src_text.replace(wrong, right)
    lines = src_text.split("\n")
    vanilla = Vanilla(args.vanilla)
    keywords = []

    # ---- locate sections ------------------------------------------------------------------------------------------
    paths_s = find(lines, "Hero Paths", 150)
    paths_e = find(lines, "4. Influence", paths_s)
    equip_s = find(lines, "Equipment", 1400)
    creat_s = find(lines, "Creatures", equip_s)
    wand_s = find(lines, "Wanderers", creat_s)
    lists_s = find(lines, "Battle Company Lists", wand_s)
    good_s = find(lines, "GOOD BATTLE COMPANIES", lists_s)
    evil_s = find(lines, "EVIL BATTLE COMPANIES", good_s)
    scen_s = find(lines, "SCENARIOS", evil_s)

    # ---- paths ----------------------------------------------------------------------------------------------------
    paths, path_keywords, minor_raw, path_notes = parse_paths(lines, paths_s, paths_e, vanilla)
    keywords.extend(path_keywords)

    # ---- armoury ----------------------------------------------------------------------------------------------------
    equipment = parse_equipment(lines[equip_s:creat_s], vanilla)
    creatures = parse_creatures(lines[creat_s:wand_s])
    for c in creatures:
        if c.get("courage") == "*":
            c["courage"] = None
            c["courageSameAsMaster"] = True
    wanderers = parse_wanderers(lines[wand_s:lists_s], vanilla)

    # ---- companies --------------------------------------------------------------------------------------------------
    names = toc_names(lines, next(i for i, l in enumerate(lines) if l.strip().startswith("MINAS TIRITH")))
    heads = sorted((find(lines, n, good_s), n) for n in names)
    companies = []
    for k, (j, n) in enumerate(heads):
        end = heads[k + 1][0] if k + 1 < len(heads) else scen_s
        tpl = next((i for i in range(j, end) if lines[i].startswith("This is a template for you to print")), end)
        body = [l for l in lines[j + 1:min(end, tpl)] if l.strip() not in ("EVIL BATTLE COMPANIES", "GOOD BATTLE COMPANIES")]
        alignment = "good" if j < evil_s else "evil"
        pretty = core.COMPANY_NAMES.get(fold(n), smart_title(n))
        vf = vanilla.find("factions", pretty)
        c = parse_company(vf or pretty, alignment, body, vanilla, keywords)
        if vf:
            c["vanillaFaction"] = vf
        if c["name"] in core.LIMIT_EXEMPTIONS:
            page = [t for t in c.get("specialRules", [])] + [n["text"] for n in c.get("notes", [])]
            for ex in core.LIMIT_EXEMPTIONS[c["name"]]:
                assert ex["text"] in page, (c["name"], ex["text"])
            c["limitExemptions"] = core.LIMIT_EXEMPTIONS[c["name"]]
        companies.append(c)

    # ---- scenarios ------------------------------------------------------------------------------------------------
    toc_s = next(i for i, l in enumerate(lines) if re.match(r"^SEIZE GROUND\s+\d+$", l.strip()))
    scen_names = []
    for l in lines[toc_s:toc_s + 25]:
        m = re.match(r"^([A-Z][A-Z ’']+?)\s{3,}\d+$", l.strip())
        if not m:
            break
        scen_names.append(m[1])
    sheads = sorted((find(lines, n, scen_s), n) for n in scen_names)
    thanks = find(lines, "Special Thanks", sheads[-1][0])
    scenarios = []
    for k, (j, n) in enumerate(sheads):
        end = sheads[k + 1][0] if k + 1 < len(sheads) else thanks
        scenarios.append(parse_scenario(n, lines[j + 1:end]))

    effects = core.AFTERMATH_EFFECTS + scenario_rules.EFFECTS
    scenario_rules.enrich(scenarios, effects)

    # ---- keywords: path rules + hero upgrades; a rule that shares its name with a vanilla rule is a Battle Companies -----
    # ---- generalisation of it (named character -> any Hero) and overrides it ------------------------------------------
    kw_final, seen = [], {}
    rename = {}
    for kw in keywords:
        name = kw["name"]
        label = None
        if name in seen:  # same name defined twice inside the Battle Companies file
            label = kw["name"]  # the key is "Name (source)", the label stays the printed name
            name = f"{kw['name']} ({kw['source']})"
            rename[(kw["source"], kw["name"])] = name
        seen[name] = kw
        kw = {**kw, "name": name}
        if label:
            kw["label"] = label
        if name == kw["name"] and vanilla.keyword(name):
            kw["overrides"] = vanilla.keyword(name)
        kw_final.append(kw)
    for kw in core.SCENARIO_KEYWORDS:
        assert not vanilla.keyword(kw["name"]) and kw["name"] not in seen, f"{kw['name']} is already defined"
        kw_final.append(dict(kw))
    for c in companies:
        for u in c.get("heroUpgrades", []):
            u["specialRule"] = rename.get((c["name"], u["name"]), u["specialRule"])
    for p in paths:
        for row in p["progression"]:
            stack = list(row["options"])
            while stack:
                o = stack.pop()
                if o["type"] == "oneOf":
                    stack.extend(o["options"])
                if o["type"] == "specialRule":
                    o["specialRule"] = rename.get((p["name"], o["specialRule"]), o["specialRule"])
    for cr in creatures:
        for r in cr.pop("rules", []):
            name, sep, text = r.partition(":")
            if sep and len(name.split()) <= 4:
                kname = name.strip()
                # A creature's rule that shares a vanilla name is a narrower, creature-bound variant (the Falcon's Keen Sight is not the
                # vanilla Keen Sight of the Crebain): it gets its own name instead of overriding the vanilla rule (amy.58).
                label = None
                if vanilla.keyword(kname):
                    label, kname = kname, f"{kname} ({cr['name']})"
                entry = {"name": kname, "definition": text.strip(), "source": cr["name"]}
                if label:  # key "Keen Sight (Falcon)", label "Keen Sight", related to the vanilla rule of that label
                    entry["label"], entry["relatedTo"] = label, [vanilla.keyword(label)]
                seen[kname] = True
                kw_final.append(entry)
                cr.setdefault("specialRules", []).append(kname)
            else:
                cr.setdefault("notes", []).append(r)
    # Creature rule defined in prose before the profiles
    kw_final.append({"name": "Handler's Glove", "tag": "Active", "source": "Creatures",
                     "definition": "This Creature starts the game upon the arm of its master rather than on the board. At the start of any turn in which the Creature is on their Hero model's arm, the Hero may release the creature - place it anywhere within 3\" of the Hero. Whilst separate from its Hero, the Creature may move independently. If, during the End phase of any turn, a Creature is within 3\" of their Hero, it may choose to land back on their Hero model's arm. Remove the Creature from the board. Whilst upon the Hero model's arm, the Creature cannot be targeted, attacked or hit with shooting attacks, and will not provide an In The Way roll for their Hero."})

    vanilla_kw_text = {}
    vdata = json.load(open(args.vanilla, encoding="utf-8"))["data"]
    for k in vdata["keywords"]:
        vanilla_kw_text.setdefault(k["name"], k)
    for g in core.GENERALIZED_VANILLA_RULES:
        src = vanilla_kw_text[g["name"]]
        text = src["definition"]
        for old, new in g["replace"]:
            assert old in text, (g["name"], old)
            text = text.replace(old, new)
        entry = {"name": g["name"], "definition": norm(text), "source": "Generalised from the vanilla rule", "overrides": g["name"]}
        if src.get("tag"):
            entry["tag"] = src["tag"]
        kw_final.append(entry)

    for sv in core.SPELLING_VARIANT_RULES:
        src = vanilla_kw_text[sv["vanilla"]]
        entry = {"name": sv["name"], "definition": norm(src["definition"]),
                 "source": f"Spelling used by the vanilla units; the vanilla rule is {sv['vanilla']}", "overrides": sv["vanilla"]}
        if src.get("tag"):
            entry["tag"] = src["tag"]
        kw_final.append(entry)

    param_rules = []
    vpowers = {p["name"]: p for p in vdata["magicalPowers"]}
    for pr in core.PARAMETERISED_RULES:
        pr = dict(pr)
        src = vpowers[pr["definitionFrom"]] if pr["type"] == "magicalPower" else vanilla_kw_text[pr["definitionFrom"]]
        text = norm(src["definition"])
        for old, new in pr.pop("replace", []):
            assert old in text, (pr["name"], old)
            text = text.replace(old, new)
        text += pr.pop("append", "")
        pr["definition"] = text
        if src.get("tag"):
            pr["tag"] = src["tag"]
        if pr["type"] == "magicalPower":
            pr["duration"] = src["duration"]
        base = pr["name"]
        pr["vanillaVariants"] = sorted({k for k in (list(vanilla_kw_text) + list(vpowers)) if k.startswith(base + " (")})
        param_rules.append(pr)
    param_kind = {pr["name"]: pr["parameter"]["kind"] for pr in param_rules}

    # ---- vanilla names defined more than once: record the decision, copy the chosen definition when they differ -----------
    van_doc = json.load(open(args.vanilla, encoding="utf-8"))
    pre = {"data": {"companies": companies, "paths": paths, "minorSpecialRules": [],
                    "wanderers": {"items": wanderers}, "creatures": {"items": creatures},
                    "equipment": {"items": equipment}, "parameterisedRules": param_rules}}
    needed = referenced(pre, van_doc)
    own_keywords = {k["name"] for k in kw_final}
    dups = vanilla_duplicates(van_doc)
    ambiguities = []
    for kind, groups in dups.items():
        for name, entries in groups.items():
            differs = len({json.dumps(e, sort_keys=True) for _, e in entries}) > 1
            reviewed = (kind, name) in core.DUPLICATE_DECISIONS
            if name not in needed[kind] and not (kind == "keywords" and name in own_keywords) and not reviewed and not differs:
                continue
            if kind == "keywords" and name in own_keywords and (kind, name) not in core.DUPLICATE_DECISIONS:
                decision = ("overridden", None, "Replaced by a Battle Companies rule of the same name.")
            else:
                decision = core.DUPLICATE_DECISIONS.get((kind, name))
            if decision is None:
                sys.exit(f"UNREVIEWED vanilla duplicate needed by Battle Companies: {kind} {name!r}: add it to core.DUPLICATE_DECISIONS")
            status, use, note = decision
            entry = {"kind": kind[:-1] if kind != "gear" else "gear", "name": name, "status": status, "note": note}
            if status in ("firstWins", "pinned"):
                entry["occurrence"] = use
                entry["occurrences"] = len(entries)
            ambiguities.append(entry)

    # ---- minor special rules ----------------------------------------------------------------------------------------------
    minor = []
    for m in minor_raw:
        raw = m["name"]
        ref = re.search(r"\((pg\.[^)]*)\)", raw)
        base = re.sub(r"\s*\(pg\.[^)]*\)", "", raw).strip()
        has_x = base.endswith("(X)")
        base = re.sub(r"\s*\(X\)$", "", base)
        entry = {"specialRule": vanilla.keyword(base) or base}
        if has_x:
            # parameterised rule: see data.parameterisedRules for what X means
            entry["specialRule"] = base
            entry["parameterKind"] = param_kind[base]
            entry["parameter"] = m["notes"][0] if m["notes"] else "X"
        elif not vanilla.keyword(base):
            entry["missingInVanilla"] = True
        if ref:
            entry["reference"] = ref[1]
        minor.append(entry)


    # ---- Gold price of everything that can be bought with it (= its initial rating / points) -----------------------------
    for e in equipment:
        e["gold"] = dict(e["rating"])
    for fx in effects:  # equipment that acts on the injury rolls points to its aftermath effect
        m = re.match(r"data\.equipment\.items\[name=(.+)\]$", fx.get("source", ""))
        if m:
            item = next(e for e in equipment if e["name"] == m[1])
            item["aftermathEffect"] = fx["key"]
    for g in core.ARMOURY_WARGEAR["items"]:
        g["gold"] = dict(g["rating"])
    core.ARMOURY_WARGEAR["armourUpgrade"]["gold"] = dict(core.ARMOURY_WARGEAR["armourUpgrade"]["rating"])
    for co in companies:
        for u in co.get("uniqueWargear", []):
            if u.get("rating"):
                u["gold"] = dict(u["rating"])
    for c in creatures:
        c["gold"] = c["points"]
    for w in wanderers:
        w["gold"] = w["points"]

    for co in companies:
        co["oath"] = oaths.oath(oaths.OATHS[co["name"]])
        company_effects.apply_unit_choices(co)
        rare = []
        for chart in ("reinforcements", "specialChart"):
            for row in co.get(chart, {}).get("table", []):
                r = row["result"]
                if r["type"] == "unitChoice":
                    for o in r["options"]:
                        rare.append({"unit": o["unit"], **({"label": o["label"]} if o.get("label") else {}),
                                     **({"selectedOptions": o["selectedOptions"]} if o.get("selectedOptions") else {}),
                                     "rare": o["rare"], "where": chart, "roll": row["roll"], "type": "unitChoice"})
                elif r.get("rare"):
                    rare.append({**({"unit": r["unit"]} if r.get("unit") else {"race": r.get("race")}), "rare": r["rare"], "where": chart,
                                 "roll": row["roll"], **({"type": r["type"]} if r.get("type") != "unit" else {})})
        for su in co.get("specialUnits", []):
            rare.append({"unit": su["unit"], **({"label": su["label"]} if su.get("label") else {}), "rare": su["rare"], "where": "specialUnits"})
        if rare:
            co["rareUnits"] = rare
    for co in companies:
        co["effects"] = company_effects.build_effects(co)
        for u in co.get("uniqueWargear", []):
            m = re.match(r"^Limit (\d+);\s*(.+?) Only\b", u.get("definition", ""))
            if m:
                u["limit"], u["restriction"] = int(m[1]), m[2]
                rest = u["definition"][m.end():].strip()  # the restriction line is data (limit, restriction), not rules text
                if rest:
                    u["definition"] = rest
                else:
                    del u["definition"]

    for co in companies:
        for u in co.get("uniqueWargear", []):
            if (co["name"], u["name"]) in core.UNIQUE_WARGEAR_UNITS:
                u["restrictedToUnits"] = core.UNIQUE_WARGEAR_UNITS[(co["name"], u["name"])]
            if (co["name"], u["name"]) in core.UNIQUE_WARGEAR_KIND:
                u["restrictedToKind"] = core.UNIQUE_WARGEAR_KIND[(co["name"], u["name"])]
        for u in co.get("heroUpgrades", []):
            if (co["name"], u["name"]) in core.HERO_UPGRADE_UNITS:
                u.update(core.HERO_UPGRADE_UNITS[(co["name"], u["name"])])

    # ---- magical powers and the starting gear of the paths ---------------------------------------------------------------
    glossary = json.load(open(ROOT / "definitions/glossary/mesbg_glossary.json", encoding="utf-8"))
    magic.add_starting_gear(paths, glossary)
    for pth in paths:
        pth.setdefault("kind", "heroic")
    paths.extend(json.loads(json.dumps(core.EXTRA_PATHS)))
    kw_final.append(dict(core.KEYWORD_END_OF_PATH))
    kw_final.extend(dict(k) for k in core.KEYWORDS_OWN_MAKING)
    for pth in paths:
        pth.setdefault("dice", "2d6")
    add_offers(paths)

    # ---- assemble -----------------------------------------------------------------------------------------------------
    scoring = [{"name": s["name"], "points": [[{"text": t["text"], "points": t["points"]} for t in b["tiers"]] for b in s["scoring"]]}
               for s in scenarios if "scoring" in s]
    campaign = {
        "company": {**core.COMPANY, "oath": {k: oaths.oath(v) for k, v in oaths.FALLBACK.items()}}, "experience": core.EXPERIENCE, "injuries": core.INJURIES,
        "progression": core.PROGRESSION, "characteristicLimits": core.CHARACTERISTIC_LIMITS,
        "influence": core.INFLUENCE, "rating": core.RATING, "againstTheOdds": core.AGAINST_THE_ODDS,
        "scenarioSelection": core.SCENARIO_SELECTION, "customCompany": core.CUSTOM_COMPANY,
        "gold": core.GOLD, "aftermathEffects": effects, "breakPoint": core.BREAK_POINT, "matchInput": core.match_input(),
        "recruitment": core.RECRUITMENT, "options": core.CAMPAIGN_OPTIONS,
    }
    doc = {
        "name": "Battle Companies: 2nd Community Edition - Core Rules",
        "version": "1.1",
        "extends": vanilla.game,
        "credit": [{"name": "Battle Companies: 2nd Community Edition - Core Rules", "author": "Middle-Earth: SBG Community",
                    "version": "1.1 (Stable)"}],
        "resolution": {
            "order": ["entity", "battleCompanies", "vanilla"],
            "firstOccurrenceWins": True,
            "text": "A definition embedded in the entity that uses it (a unit's own rule, a hero upgrade) wins over the same term in data.keywords; Battle Companies definitions win over the vanilla file, which is only a reference store and is never modified. A vanilla name defined more than once is resolved by the first occurrence unless data.vanillaAmbiguities pins another one.",
        },
        "limits": core.LIMITS,
        "validations": core.VALIDATIONS,
        "campaign": campaign,
        "scenarios": [s["name"] for s in scenarios],
        "scenarioScoring": scoring,
        "data": {
            "companies": companies,
            "paths": paths,
            "magicalPowers": magic.build_magical_powers(paths, van_doc),
            "minorSpecialRules": minor,
            "pathNotes": path_notes,
            "reservedOptions": reserved_options(van_doc),
            "optionSales": option_sales(van_doc),
            "unitFillIns": unit_fill_ins(),
            "mountFillIns": mount_fill_ins(),
            "vanillaAmbiguities": sorted(ambiguities, key=lambda a: (a["kind"], a["name"])),
            "wargear": core.ARMOURY_WARGEAR,
            "equipment": {**core.EQUIPMENT_TEXT, "items": equipment},
            "creatures": {**core.CREATURES_TEXT, "items": [{**c, **core.CREATURE_ITEM_RULES.get(c["name"], {})} for c in creatures]},
            "wanderers": {**core.WANDERERS_TEXT, "items": wanderers},
            "keywords": kw_final,
            "parameterisedRules": param_rules,
            "scenarioDetails": [{k: v for k, v in s.items() if k != "scoring"} | ({"scoring": [b["text"] for b in s["scoring"]]} if "scoring" in s else {}) for s in scenarios],
        },
    }
    # ---- gear annotations (need the finished file: keywords, wargear tables, companies) -------------------------------------
    doc["data"]["wargearReadAs"] = {
        "text": "Company lists that write a generic item where the unit's profile has a specific one are read as the profile's item, so every item keeps its own identity (curator's readings, to be confirmed).",
        "items": apply_read_as(doc)}
    gear, gear_report = build_gear_annotations(doc, van_doc, needed["units"])
    doc["data"]["gearAnnotations"] = gear
    ann = {item_key(a["name"]): a for a in gear["items"]}
    ann.update({item_key(x): a for a in gear["items"] for x in a.get("aliases", [])})
    for co in doc["data"]["companies"]:
        for u in co.get("uniqueWargear", []):
            a = ann.get(item_key(u["name"]))
            if a and not u.get("definition") and a.get("definition"):
                u["definition"] = a["definition"]
                u["definitionFrom"] = "data.gearAnnotations"
            if a and a.get("profile"):
                u["profile"] = a["profile"]
    doc["data"]["profileWargearNotSold"] = profile_wargear_not_sold(doc, van_doc, needed["units"])
    doc["data"]["ttsModels"] = tts.build(doc, van_doc)
    doc["data"]["ruleTargets"] = rule_targets.build(doc, van_doc)
    doc["data"]["auras"] = auras.build(doc, van_doc)
    write_gap_report(doc, ROOT / "tools/armoury_gaps.md")
    write_review(gear, gear_report, ROOT / "tools/gear_review.md")
    print(f"gear annotations: {len(gear['items'])} items, {len(gear_report.unclear_grants)} open grant questions, "
          f"{len(gear_report.unresolved)} unresolved")

    Path(args.out).parent.mkdir(parents=True, exist_ok=True)
    with open(args.out, "w", encoding="utf-8") as f:
        json.dump(doc, f, ensure_ascii=False, indent=2)
        f.write("\n")
    report(doc, vanilla)


def report(doc, vanilla):
    missing, bad_rules = set(), set()

    def walk(o):
        if isinstance(o, dict):
            if o.get("missingInVanilla"):
                missing.add(o.get("unit") or o.get("specialRule"))
            for v in o.values():
                walk(v)
        elif isinstance(o, list):
            for v in o:
                walk(v)

    walk(doc["data"])
    own = {k["name"] for k in doc["data"]["keywords"]}
    for p in doc["data"]["paths"]:
        for row in p["progression"]:
            stack = list(row["options"])
            while stack:
                o = stack.pop()
                if o["type"] == "oneOf":
                    stack.extend(o["options"])
                if o["type"] == "specialRule" and o["specialRule"] not in own and not vanilla.keyword(o["specialRule"]):
                    bad_rules.add(o["specialRule"])
    clash = [s for s in doc["scenarios"] if vanilla.find("keywords", s) or s in vanilla.collections.get("scenarios", set())]
    print(f"companies={len(doc['data']['companies'])} paths={len(doc['data']['paths'])} keywords={len(own)} "
          f"equipment={len(doc['data']['equipment']['items'])} scenarios={len(doc['scenarios'])}")
    print("units/rules not found in vanilla:", sorted(m for m in missing if m))
    print("path special rules defined nowhere:", sorted(bad_rules))
    if clash:
        print("scenario names that clash with vanilla names:", clash)


if __name__ == "__main__":
    main()
