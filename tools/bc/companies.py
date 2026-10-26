"""Parses the Battle Company lists (starting company, reinforcement/special charts, hero upgrade, advancements...)."""
import re

from .text import blocks, blocks_tab, fold, join, norm

ROLE_RE = re.compile(r"\((?:must be your\s+)?(leader|sergeant|hero in the making)\)", re.I)
RARE_RE = re.compile(r"\(\s*Rare\s+(\d+)\s*\)", re.I)
CHOICE_RE = re.compile(r"\bwith\s+(?:a\s+)?choice of\s+(?:options?|wargear|weapon)s?\b", re.I)
HEADINGS = [
    ("starting", re.compile(r"^Starting Battle Company\b")),
    ("reinforcements", re.compile(r"^Reinforcements\s*-\s*(\d+)\s*Influence")),
    ("hero_upgrade", re.compile(r"^Company Hero Upgrades?\b[:]?")),
    ("advancements", re.compile(r"^Advancements\b")),
    ("special", re.compile(r"^Special(?: Chart)?$")),
    ("special_rules", re.compile(r"^Company Special Rules?\b[:]?")),
    ("elite", re.compile(r"^Elite Company\b[.:]?")),
    ("unique_wargear", re.compile(r"^Unique (?:Wargear|Equipment)\b[:]?")),
]


def parse_unit(text: str, vanilla, with_count=True):
    """'2 Mordor Orc with two-handed weapon' -> {count, unit, wargear, choice, role, rare}."""
    t = norm(text)
    out = {}
    rm = ROLE_RE.search(t)
    if rm:
        out["role"] = {"leader": "leader", "sergeant": "sergeant", "hero in the making": "heroInTheMaking"}[rm[1].lower()]
        t = (t[:rm.start()] + t[rm.end():]).strip()
    ref = re.search(r"\s*\((see [^)]*)\)", t)
    if ref:
        out["reference"] = ref[1]
        t = (t[:ref.start()] + t[ref.end():]).strip()
    rare = RARE_RE.search(t)
    if rare:
        out["rare"] = int(rare[1])
        t = (t[:rare.start()] + t[rare.end():]).strip()
    count = 1
    if with_count:
        m = re.match(r"^(\d+)\s*x?\s+(.*)$", t)
        if m:
            count, t = int(m[1]), m[2]
    if with_count:
        out["count"] = count
    choice = CHOICE_RE.search(t)
    if choice:
        out["choice"] = True
        t = (t[:choice.start()] + t[choice.end():]).strip()
    name, _, gear = re.split(r"\s+(with)\s+", t, maxsplit=1, flags=re.I) if re.search(r"\s+with\s+", t, re.I) else (t, "", "")
    name = name.strip().rstrip("*").strip()
    mount = re.match(r"^(?P<n>.+?)\s+on\s+(?P<m>armou?red horse|horse)$", name, re.I)
    if mount:
        name = mount["n"]
        out.setdefault("wargear", []).append(mount["m"].lower())
    res = vanilla.resolve(name)
    out["unit"] = res["unit"] if res else name
    if res and (res["via"] != "name" or res["option"]) and fold(name) != fold(res["unit"]):
        out["label"] = name  # the name printed in the document, shown to the user
        if res["option"]:
            out["selectedOptions"] = [res["option"]]
        if res.get("delta"):
            for k in ("addSpecialRules", "addKeywords"):
                if res["delta"].get(k):
                    out[k] = list(res["delta"][k])
    if not res:
        out["missingInVanilla"] = True
    if gear and gear.strip().lower() in ("equivalent wargear", "equivalent wargear."):
        out["keepsWargear"] = True
    elif gear:
        parts = [p.strip() for p in re.split(r"\s*(?:&|,|\band\b)\s*", gear) if p.strip()]
        out["wargear"] = parts + out.get("wargear", [])
    return out


def split_roll(s):
    m = re.fullmatch(r"(\d)\s*(?:-\s*(\d))?", s.strip())
    if not m:
        return None
    lo = int(m[1])
    return [lo, int(m[2]) if m[2] else lo]


def parse_result(text, vanilla):
    t = norm(text)
    if t.lower().startswith("nobody steps forward"):
        return {"type": "nothing", "blocksFurtherRolls": True}
    if t.lower() == "nothing":
        return {"type": "nothing"}
    if re.match(r"^(roll on( the)? special chart|special chart)", t, re.I):
        return {"type": "specialChart"}
    if re.match(r"^(choice of (any|the above|above)|choose any)", t, re.I):
        return {"type": "choice", "text": t}
    if re.match(r"^Hobbit\*", t):
        rare = RARE_RE.search(t)
        return {"type": "lostStartingUnit", "race": "Hobbit", **({"rare": int(rare[1])} if rare else {})}
    # "2 Hobbit Militia", "Dwarf Ranger with choice of Options (Rare 1)"
    u = parse_unit(t, vanilla)
    return {"type": "unit", **u}


def parse_table(lines, vanilla):
    """lines: stripped non-empty lines, starting at/after the D6 / Result header."""
    rows, cur_roll, cur_text = [], None, []
    notes = []
    done = False
    for l in lines:
        if l in ("D6", "Result"):
            continue
        r = split_roll(l)
        if r and not done and (cur_roll is None or r[0] == cur_roll[1] + 1) and (cur_roll is not None or r[0] == 1):
            if cur_roll is not None:
                rows.append((cur_roll, cur_text))
            cur_roll, cur_text = r, []
            continue
        if cur_roll is not None and not done:
            cur_text.append(l)
            continue
        notes.append(l)
    if cur_roll is not None:
        rows.append((cur_roll, cur_text))
    table = [{"roll": r, "result": parse_result(" ".join(t), vanilla)} for r, t in rows]
    return table


def parse_advancement(text, vanilla):
    t = norm(text)
    if "→" not in t:
        return None
    left, right = t.split("→", 1)
    adv = {"from": parse_unit(left, vanilla, with_count=False), "to": []}
    for part in re.split(r"\s+or\s+", right.strip()):
        part = part.strip().rstrip("*").strip()
        if part:
            adv["to"].append(parse_unit(part, vanilla, with_count=False))
    if t.endswith("*") or right.strip().endswith("*"):
        adv["footnote"] = True
    return adv


def split_company_blocks(lines):
    """Returns (description_paragraphs, sections); a section is [kind, match, blocks] and starts at its heading line."""
    desc_lines, sections, cur = [], [], None
    for idx, raw in enumerate(lines):
        first = norm(raw)
        if first.startswith("____"):
            continue
        kind = None
        for k, rx in HEADINGS:
            m = rx.match(first)
            if m:
                # "Special Chart" is also a reinforcement result: it is a heading only when a D6 table follows
                if k == "special" and next((l.strip() for l in lines[idx + 1:] if l.strip()), "") != "D6":
                    continue
                kind = (k, m)
                break
        if kind:
            cur = [kind[0], kind[1], [raw]]
            sections.append(cur)
        elif cur is None:
            desc_lines.append(raw)
        else:
            cur[2].append(raw)
    desc = [join(b) for b in blocks(desc_lines)]
    for sec in sections:
        sec[2] = (blocks if sec[0] in ("starting", "reinforcements", "special") else blocks_tab)(sec[2])
    return desc, sections


EPIGRAPH_RE = re.compile(r"^[“\"](?P<quote>[^“”\"]+)[”\"]\s*[–—-]\s*(?P<text>[^“”\"]+)$")
TITLE_RE = re.compile(r"^(?P<name>[^\n]{3,110}?)(?:\s+[–-]\s*|–\s*|(?<=[a-z])-\s+(?=[A-Z]))(?!\d)(?P<rest>.*)$", re.S)


def parse_hero_upgrades(sec_blocks, first_match_len, keywords, company):
    """sec_blocks[0] starts with the heading. Titles look like 'Name - flavour' followed by a rule paragraph."""
    paras = []
    for i, b in enumerate(sec_blocks):
        lines = list(b)
        if i == 0:
            lines[0] = re.sub(r"^Company Hero Upgrades?\b[:]?\s*", "", norm(lines[0]))
            if not lines[0]:
                lines = lines[1:]
        if lines:
            paras.append(join(lines))
    items, i = [], 0

    def is_title(p):
        m = TITLE_RE.match(p)
        return bool(m) and len(m["name"].split()) <= 12 and not re.search(r"[.!?]\s", m["name"])

    while i < len(paras):
        p = paras[i]
        m = TITLE_RE.match(p)
        if not m or not is_title(p):
            if items:
                items[-1]["definition"] = (items[-1].get("definition", "") + " " + p).strip()
            i += 1
            continue
        name = m["name"].strip().strip("'\"")
        tg = re.search(r"\s*\[(Active|Passive)\]$", name)
        tag = tg[1] if tg else None
        if tg:
            name = name[:tg.start()]
        rest = m["rest"].strip()
        has_rule_next = i + 1 < len(paras) and not is_title(paras[i + 1])
        item = {}
        rr = re.match(r"^(?P<n>.*?)\s*\((?P<who>[^)]*?)\s*only\)\s*$", name, re.I)
        if rr:
            name, item["restriction"] = rr["n"].strip(), rr["who"].strip()
        item["name"] = name
        if tag:
            item["tag"] = tag
        if has_rule_next and re.match(r"(However|Additionally)\b", paras[i + 1]) and ". " in rest:
            # the rule's first paragraph shares the title paragraph with the flavour (Callous Leadership)
            fl, _, first = rest.partition(". ")
            item["flavour"] = fl + "."
            item["definition"] = first + " " + paras[i + 1]
            i += 2
        elif has_rule_next:
            item["flavour"] = rest
            item["definition"] = paras[i + 1]
            i += 2
        else:
            item["definition"] = rest
            i += 1
        items.append(item)
    return items


def parse_company(name, alignment, lines, vanilla, keywords):
    desc, sections, = split_company_blocks(lines)
    c = {"name": name, "alignment": alignment}
    c["description"] = "\n\n".join(desc)
    notes = []
    for kind, m, bl in sections:
        if kind == "starting":
            units, gold = [], 0
            for b in bl:
                p = join(b)
                p = re.sub(r"^Starting Battle Company\s*", "", p)
                if not p:
                    continue
                gm = re.fullmatch(r"(\d+)\s*Gold", p, re.I)
                if gm:
                    gold = int(gm[1])
                elif re.match(r"^\d+\s*x?\s", p) and "Gold" not in p:
                    units.append(parse_unit(p, vanilla))
                else:
                    notes.append({"section": "startingCompany", "text": p})
            c["startingCompany"] = {"units": units, "gold": gold}
        elif kind == "reinforcements":
            flat = [norm(l) for b in bl for l in b]
            flat = flat[1:]  # heading
            # table ends at the first block boundary: the first block holds the table, the rest are notes
            table = parse_table([norm(l) for l in bl[0][1:]], vanilla)
            c["reinforcements"] = {"influence": int(m[1]), "table": table}
            for b in bl[1:]:
                notes.append({"section": "reinforcements", "text": join(b)})
        elif kind == "special":
            table = parse_table([norm(l) for l in bl[0][1:]], vanilla)
            c["specialChart"] = {"table": table}
            for b in bl[1:]:
                notes.append({"section": "specialChart", "text": join(b)})
        elif kind == "hero_upgrade":
            ups = parse_hero_upgrades(bl, m.end(), keywords, c["name"])
            for u in ups:
                kw = {"name": u["name"], "definition": u["definition"], "source": c["name"]}
                if "tag" in u:
                    kw["tag"] = u["tag"]
                keywords.append(kw)
            c["heroUpgrades"] = [{k: v for k, v in u.items() if k not in ("definition", "tag")} | {"specialRule": u["name"]}
                                 for u in ups]
        elif kind == "advancements":
            advs = []
            for i, b in enumerate(bl):
                lines_b = [norm(l) for l in b]
                if i == 0:
                    lines_b = lines_b[1:]
                if not lines_b:
                    continue
                p = join(lines_b)
                a = parse_advancement(p, vanilla)
                if a:
                    advs.append(a)
                elif p.startswith("*") or True:
                    # an advancement continues on the next block when the arrow got separated
                    notes.append({"section": "advancements", "text": p})
            c["advancements"] = advs
        elif kind == "special_rules":
            for i, b in enumerate(bl):
                lines_b = [norm(l) for l in b]
                if i == 0:
                    lines_b[0] = re.sub(r"^Company Special Rules?\b[:]?\s*", "", lines_b[0])
                    lines_b = [l for l in lines_b if l]
                if not lines_b:
                    continue
                text = join(lines_b)
                # a quoted title and a flavour sentence ("Leave all that ..." - As Arnor fell ...) head the rule like a hero
                # upgrade's name and flavour: they are the company's epigraph, not a rule (the quote may sit apart from its
                # flavour line)
                ep = EPIGRAPH_RE.match(text)
                if ep and "epigraph" not in c:
                    c["epigraph"] = {"quote": ep["quote"].strip(), "text": ep["text"].strip()}
                    continue
                c.setdefault("specialRules", []).append(text)
        elif kind == "elite":
            c["maxModels"] = 12
            for i, b in enumerate(bl):
                p = join(b)
                p = re.sub(r"^Elite Company\.?\s*", "", p)
                if p and not p.startswith("This Battle Company may only have up to 12"):
                    notes.append({"section": "eliteCompany", "text": p})
        elif kind == "unique_wargear":
            items, curr = [], None
            for i, b in enumerate(bl):
                ls = [norm(l) for l in b]
                if i == 0:
                    ls[0] = re.sub(r"^Unique (?:Wargear|Equipment):?\s*", "", ls[0])
                    ls = [l for l in ls if l]
                for l in ls:
                    mm = re.match(r"^(?:\((?P<who>[^)]*)\):\s*)?(?P<name>.+?)\s*\((?P<r1>\d+)(?:/(?P<r2>\d+))?\)\s*[–-]?\s*$", l)
                    im = re.match(r"^(\d+)\s+Influence Points?", l, re.I)
                    if mm:
                        curr = {"name": mm["name"], "rating": {"warrior": int(mm["r1"]), "hero": int(mm["r2"] or mm["r1"])}}
                        if mm["who"]:
                            curr["restriction"] = mm["who"]
                        items.append(curr)
                    elif im and curr is not None:
                        curr["influence"] = int(im[1])
                    elif curr is not None:
                        curr["definition"] = (curr.get("definition", "") + " " + l).strip()
            c["uniqueWargear"] = items
    if notes:
        c["notes"] = notes
    # structured bits derived from the free text
    texts = [t for t in c.get("specialRules", [])] + [n["text"] for n in notes]
    lim = {}
    for t in texts:
        m = re.search(r"up to (\d+)% of (?:its|their) (?:models|Battle Company)[^.]*?(bow|Cavalry)", t, re.I)
        if m:
            lim[m[2].lower() if m[2].lower() != "bow" else "bow"] = int(m[1])
        m = re.search(r"up to (\d+) (?:models|members),? (?:rather|instead)", t, re.I)
        if m:
            c["maxModels"] = int(m[1])
        m = re.search(r"Break Point[^.]*? is (\d+)%", t)
        if m:
            c["breakPointPercentage"] = int(m[1])
    if lim:
        c["limitOverrides"] = lim
    company_limits = []
    for t in texts:
        m = re.search(r"No more than (\d+)% of your company may include models with the (\w+) keyword", t)
        if m:
            company_limits.append({"keyword": m[2], "maximumPercentage": int(m[1]), "text": t})
    if company_limits:
        c["companyLimits"] = company_limits
    opp = []
    for t in texts:
        m = re.search(r"^(.*?)\s+(?:are|is)\s+On Their Own Path", t)
        if m:
            for part in re.split(r"\s*(?:&|,|\band\b)\s*", m[1]):
                part = part.strip()
                if part:
                    opp.append(vanilla.unit(part) or part)
    if opp:
        c["onTheirOwnPath"] = opp
    c.update(rule_changes(texts, vanilla))
    hires = []
    for t in texts:
        for m in re.finditer(r"may hire an? (?P<unit>.+?)(?: with Choice of Option)? \(Rare (?P<rare>\d+)\)"
                             r"(?:,? using the profile of the (?P<profile>.+?) with the (?P<kw>\w+) keyword,?)?"
                             r" for (?P<n>\d+) Influence", t):
            if m["profile"]:  # "a Goblin Hulk, using the profile of the Gundabad Ogre with the Goblin keyword"
                base = vanilla.unit(m["profile"]) or m["profile"]
                h = {"unit": base, "label": m["unit"], "addKeywords": [m["kw"]]}
                if not vanilla.unit(m["profile"]):
                    h["missingInVanilla"] = True
            else:
                h = parse_unit(m["unit"], vanilla, with_count=False)
            h.update({"rare": int(m["rare"]), "influence": int(m["n"])})
            if "with Choice of Option" in m[0]:
                h["choice"] = True
            hires.append(h)
    if hires:
        c["specialUnits"] = hires
    link_unit_notes(c, vanilla)
    return c


SUBST_RE = re.compile(
    r"(?P<who>[^.]*?)\btreat (?P<target>.+?) (?:exactly )?as(?: if they were)? (?P<as>(?:an? |the )?[A-Z][^.]*?) for the purposes? of "
    r"(?:their|the) (?:(?P<rule>[^.]*?) )?special rules?", re.S)
EXCH_RE = re.compile(r"exchange (?P<a>[A-Z][\w ]+ \([^)]+\)) for (?P<b>[A-Z][\w ]+ \([^)]+\))")
GRANT_RE = re.compile(r"(?P<who>All [^.]*?) gain the (?P<rule>[^.]+?) special rule(?P<tail> (?:for free|for no additional cost))?\.?$")


def rule_changes(texts, vanilla):
    """Company special rules that rewire vanilla rules: who counts as the named character, rule swaps, free rules."""
    out = {}
    for t in texts:
        t = norm(t)
        m = SUBST_RE.search(t)
        if m:
            target = m["target"].strip()
            if re.fullmatch(r"(the )?Leader", target, re.I):
                tgt = {"role": "leader"}
            elif re.fullmatch(r"(the )?Sergeants?", target, re.I):
                tgt = {"role": "sergeant"}
            else:
                has = re.search(r"with the (.+?) special rule", target)
                tgt = {"hero": True, **({"withSpecialRule": has[1]} if has else {})}
            rule = (m["rule"] or "").strip()
            sub = {"appliesTo": re.sub(r"\s+in this Battle Company$", "", re.sub(r"^(Models in this Battle Company|Friendly)\s*", "", m["who"].strip())) or "Battle Company models",
                   "target": tgt, "treatedAs": re.sub(r"^(an? |the )", "", m["as"].strip()), "text": t}
            if rule:
                kw = vanilla.keyword(rule)
                sub["specialRule"] = kw or rule
            out.setdefault("ruleSubstitutions", []).append(sub)
        m = EXCH_RE.search(t)
        if m:
            out.setdefault("ruleExchanges", []).append({"remove": m["a"], "add": m["b"], "text": t})
        m = GRANT_RE.search(t)
        if m and not SUBST_RE.search(t):
            who = m["who"].strip()
            rule = vanilla.keyword(m["rule"]) or m["rule"]
            out.setdefault("grantedSpecialRules", []).append(
                {"specialRule": rule, "appliesTo": who, "cost": 0, "text": t})
    return out


def link_unit_notes(c, vanilla):
    """Units written under another name get an explanation on the company page (`unitAliases`).

    A unit is either a vanilla unit with an option pre-selected (Tookish Hunter = Hobbit Archer + Tookish Hunter) or a
    name the document prints for a vanilla unit (2018 name, abbreviation) (Grimhammer = Grim Hammer Warrior). Notes of the document
    that talk about them get `about`, and their text is reused as the explanation.
    """
    found = {}

    def walk(o):
        if isinstance(o, dict):
            if o.get("label"):
                found.setdefault(o["label"], o)
            for v in o.values():
                walk(v)
        elif isinstance(o, list):
            for v in o:
                walk(v)

    walk({k: v for k, v in c.items() if k != "notes"})
    for n in c.get("notes", []):
        text = fold(n["text"])
        about = [l for l in sorted(found)
                 if fold(l) in text or fold(l).rstrip("s") in text
                 or (len(l.split()) > 1 and len(l.split()[0]) >= 6 and fold(l.split()[0]) in text)]
        if about:
            n["about"] = about
    aliases = []
    for label, o in sorted(found.items()):
        entry = {"label": label, "unit": o["unit"]}
        if o.get("selectedOptions"):
            entry["selectedOptions"] = o["selectedOptions"]
            entry["reason"] = "option"
            opt = vanilla.option(o["unit"], o["selectedOptions"][0]) or {}
            entry["text"] = f"{label} is a {o['unit']} with the {o['selectedOptions'][0]} option selected."
            if opt.get("points") is not None:
                entry["text"] += f" The option costs {opt['points']} point{'s' if opt['points'] != 1 else ''}."
            if opt.get("availableIn") and all(isinstance(a, str) for a in opt["availableIn"]):
                entry["text"] += (" In the base game only " + " and ".join(opt["availableIn"])
                                  + " can take it; this company's rules allow it.")
        elif o.get("addSpecialRules") or o.get("addKeywords"):
            entry["reason"] = "delta"
            for k in ("addSpecialRules", "addKeywords"):
                if o.get(k):
                    entry[k] = o[k]
            added = [f"the {r} special rule" for r in o.get("addSpecialRules", [])] + [f"the {k} keyword" for k in o.get("addKeywords", [])]
            entry["text"] = f"{label} uses the profile of the {o['unit']} with {' and '.join(added)} added (a Battle Companies addition)."
            if o.get("influence"):
                entry["text"] += f" It is hired for {o['influence']} Influence Points" + (f" (Rare {o['rare']})." if o.get("rare") else ".")
        else:
            entry["reason"] = "legacyName"
            entry["text"] = (f"The Battle Companies document calls this unit {label}; in the base game it is the "
                             f"{o['unit']}, with the same profile and points.")
        said = [n["text"] for n in c.get("notes", []) if label in n.get("about", [])]
        if said:
            entry["companyNotes"] = said
        aliases.append(entry)
    if aliases:
        c["unitAliases"] = aliases
