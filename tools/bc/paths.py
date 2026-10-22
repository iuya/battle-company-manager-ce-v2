"""Parses the Hero Paths section (path maximums, the 2d6 progression table and the sorcerer's spell list)."""
import re

from .text import blocks, join, norm

STAT_ORDER = ["movement", "fight", "shoot", "strength", "defence", "attack", "wounds", "courage", "intelligence",
              "might", "will", "fate"]
HEADER_TOKENS = {"Mv", "Fv", "Sv", "S", "D", "A", "W", "C", "I", "M", "F"}
STAT_WORDS = {
    "attacks": "attack", "wounds": "wounds", "courage": "courage", "intelligence": "intelligence",
    "strength": "strength", "defence": "defence", "fight value": "fight", "fight": "fight",
    "shoot value": "shoot", "shoot": "shoot", "move value": "movement", "move": "movement",
    "might": "might", "will": "will", "fate": "fate",
}
RULE_RE = re.compile(r"^(?P<name>.+?)\s*\((?P<tag>Active|Passive)\)\s*[-–]\s*(?P<def>.+)$", re.S)
MASTERY_RE = re.compile(r"^(?P<name>Heroic [A-Za-z]+ Mastery)\s*[-–]\s*(?P<def>.+)$", re.S)


def parse_limit(v: str):
    v = v.strip()
    m = re.fullmatch(r"\(\+(\d+)\)", v)
    if m:
        return {"maxIncrease": int(m[1])}
    m = re.fullmatch(r"(\d+)\+", v)
    if m:
        return {"best": int(m[1])}
    if re.fullmatch(r"\d+", v):
        return {"max": int(v)}
    return {"text": v}


def stat_list(s: str):
    s = re.sub(r"\s*\(.*?\)", "", s)
    parts = re.split(r",\s*or\s+|,\s*|\s+or\s+|\s*&\s*|\s+and\s+", s.strip().lower())
    return [STAT_WORDS[p.strip()] for p in parts if p.strip() in STAT_WORDS]


def parse_improve(text: str):
    """'Improve Might or Fate by 1, or improve Will by 2 (to a Maximum of 6 Will)!' -> option dicts."""
    out = []
    for seg in re.split(r",\s*or improve\s+", re.sub(r"^\s*improve\s+", "", text, flags=re.I)):
        m = re.match(r"(?P<stats>[A-Za-z ,&]+?)\s+by\s+(?P<n>\d+)(?P<rest>.*)$", seg.strip(), re.S)
        if not m:
            return None
        stats = stat_list(m["stats"])
        if not stats:
            return None
        opt = {"type": "improveStat", "stats": stats, "amount": int(m["n"]), "mode": "all" if "&" in m["stats"] else "oneOf"}
        if len(stats) == 1:
            opt.pop("mode")
        mx = re.search(r"Maximum of (\d+)", m["rest"])
        if mx:
            opt["maximum"] = int(mx[1])
        if re.search(r"take your other roll too", m["rest"], re.I):
            opt["rollAgain"] = True
        out.append(opt)
    return out


def parse_option(seg: str, path: str, keywords: list):
    seg = norm(seg).rstrip(".")
    seg_clean = re.sub(r"^OR\s+", "", seg)
    m = RULE_RE.match(seg_clean) or MASTERY_RE.match(seg_clean)
    if m:
        name = m["name"].strip()
        tag = m.groupdict().get("tag")
        definition = norm(m["def"])
        opt = {"type": "specialRule", "specialRule": name}
        if definition.lower().startswith("see page"):
            opt["vanilla"] = True
            opt["reference"] = definition
        else:
            kw = {"name": name, "definition": definition if definition.endswith((".", "!", "?")) else definition + ".", "source": path}
            if tag:
                kw["tag"] = tag
            keywords.append(kw)
        return opt
    if seg_clean.lower().startswith("improve") or re.match(r"improve ", seg_clean, re.I):
        parsed = parse_improve(seg_clean)
        if parsed:
            return parsed[0] if len(parsed) == 1 else {"type": "oneOf", "options": parsed}
    if re.match(r"Choice of Minor Special Rule", seg_clean, re.I):
        opt = {"type": "minorSpecialRule"}
        mx = re.search(r"maximum of (\d+)", seg_clean)
        if mx:
            opt["maxTotal"] = int(mx[1])
        if "Magical Power" in seg_clean:
            return {"type": "oneOf", "options": [opt, {"type": "magicalPower"},
                                                  {"type": "improveCastingValue", "amount": 1}]}
        return opt
    if re.match(r"Pick an additional Action", seg_clean, re.I):
        opt = {"type": "heroicAction"}
        mx = re.search(r"maximum of (\d+)", seg_clean)
        if mx:
            opt["maxTotal"] = int(mx[1])
        return opt
    if "Magical Power" in seg_clean:
        opts = [{"type": "magicalPower"}]
        if "Casting Value" in seg_clean:
            o = {"type": "improveCastingValue", "amount": 1}
            mx = re.search(r"maximum of (\d+) improvements per Magical Power", seg_clean)
            if mx:
                o["maxPerPower"] = int(mx[1])
            opts.append(o)
        return opts[0] if len(opts) == 1 else {"type": "oneOf", "options": opts}
    return {"type": "text", "text": seg_clean}


def parse_row(roll: int, lines, path: str, keywords: list):
    segs, cur = [], []
    for b in blocks(lines):
        t = join(b)
        if t.upper().startswith("OR ") and cur:
            segs.append(" ".join(cur))
            cur = [t]
        else:
            cur.append(t)
    if cur:
        segs.append(" ".join(cur))
    segs = [p for sg in segs for p in re.split(r"\s+OR\s+(?=Pick an additional Action)", sg)]
    options = [parse_option(s, path, keywords) for s in segs]
    row = {"roll": roll, "text": norm(" ".join(segs)).replace("..", ".")}
    # "(& take your other roll too!)" etc. live on the option; the row text keeps the source wording.
    row["options"] = options
    return row


def parse_paths(lines, start, end, vanilla=None):
    """lines[start:end] is the Hero Paths section. Returns (paths, keywords, minor_special_rules, notes)."""
    paths, keywords, notes = [], [], []
    minor = []
    # minor special rules list sits before the first path
    head_re = re.compile(r"^Path of the ([A-Za-z]+)\s*(\(0 - 1\))?$")
    heads = [(j, head_re.match(lines[j].strip())) for j in range(start, end) if head_re.match(lines[j].strip())]

    first = heads[0][0]
    for raw in lines[start:first]:
        if re.match(r"^\*\s+", raw):
            minor.append({"name": norm(re.sub(r"^\*\s+", "", raw)), "notes": []})
        elif re.match(r"^\s+\*\s+", raw) and minor:
            minor[-1]["notes"].append(norm(re.sub(r"^\s+\*\s+", "", raw)))
    for idx, (j, m) in enumerate(heads):
        stop = heads[idx + 1][0] if idx + 1 < len(heads) else end
        seg = lines[j + 1:stop]
        name = "Path of the " + m[1]
        path = {"name": name}
        if m[2]:
            path["limitPerCompany"] = 1
        # intro paragraph(s) until first stat header token
        h = next(n for n, l in enumerate(seg) if l.strip() in ("Fv", "Mv"))
        intro = join([l for l in seg[:h] if l.strip()])
        ha = re.search(r"adds (Heroic [A-Za-z]+)", intro)
        if ha:
            path["heroicAction"] = ha[1]
        if "Magical Power to their Spell List" in intro:
            path["startingMagicalPower"] = True
        # drop the sentence about the maximums (the table below says it) and the dangling "and the" it leaves in the intro
        intro = re.sub(r",?\s*(?:and\s+)?(?:the\s+)?Hero must adhere to the following Path Maximums as they advance down this Path\.?", ".", intro, flags=re.I)
        intro = re.sub(r"\s+\.", ".", intro).replace("..", ".").strip()
        if not intro.strip(". "):
            intro = ""
        if intro:
            path["description"] = intro
        toks = []
        n = h
        while n < len(seg) and (seg[n].strip() in HEADER_TOKENS or not seg[n].strip()):
            if seg[n].strip():
                toks.append(seg[n].strip())
            n += 1
        names = STAT_ORDER if toks[0] == "Mv" else STAT_ORDER[1:]
        vals = []
        while len(vals) < len(toks):
            if seg[n].strip():
                vals.append(seg[n].strip())
            n += 1
        assert len(vals) == len(names), (name, toks, vals)
        path["maximums"] = {s: parse_limit(v) for s, v in zip(names, vals)}
        # progression rows
        while seg[n].strip() not in ("Path Result",):
            n += 1
        n += 1
        rows, cur_roll, cur_lines, expect = [], None, [], 2
        spells = []
        notes_here = []
        for line in seg[n:]:
            s = line.strip()
            if re.fullmatch(r"\d{1,2}", s) and int(s) == expect:
                if cur_roll is not None:
                    rows.append((cur_roll, cur_lines))
                cur_roll, cur_lines, expect = int(s), [], expect + 1
                continue
            if cur_roll is None:
                continue
            if expect == 13 and re.match(r"^\*\s+", s):
                spells.append(line if line[:1] in " \t" else s)
                continue
            if expect == 13 and s.startswith("Designer Note") and cur_lines is not None:
                rows.append((cur_roll, cur_lines))
                cur_lines = None
            if cur_lines is not None and not s.startswith("____"):
                cur_lines.append(line)
            elif cur_lines is None and s and not s.startswith("____"):
                notes_here.append(s)
        if cur_roll is not None and cur_lines is not None:
            rows.append((cur_roll, cur_lines))
        path["progression"] = [parse_row(r, ls, name, keywords) for r, ls in rows]
        assert [r["roll"] for r in path["progression"]] == list(range(2, 13)), name
        if spells:
            sl = []
            for s in spells:
                if s[:1] in " \t":  # sub-bullet: note for the previous spell
                    if sl:
                        sl[-1].setdefault("notes", []).append(norm(re.sub(r"^\s*\*\s*", "", s)))
                    continue
                mm = re.match(r"^\*\s+(.+?)\s*\(\s*(Self|\d+\")\s*,\s*(\d+)\+\s*\)\s*$", norm(s))
                if mm:
                    nm = mm[1].strip()
                    entry = {"magicalPower": (vanilla.power(nm) if vanilla else None) or re.sub(r"\s*\(X\)$", "", nm),
                             "range": mm[2] if mm[2] == "Self" else int(mm[2].rstrip('"')), "casting": int(mm[3])}
                    if nm.endswith("(X)"):
                        entry["parameter"] = "species"
                    sl.append(entry)
                elif sl and not s.startswith("*"):
                    pass
                else:
                    sl.append({"text": norm(s)})
            path["spellList"] = sl
        if notes_here:
            notes.extend(notes_here)
        paths.append(path)
    return paths, keywords, minor, notes


STAT_LABEL = {"movement": "Move", "fight": "Fight", "shoot": "Shoot", "strength": "Strength", "defence": "Defence", "attack": "Attacks",
              "wounds": "Wounds", "courage": "Courage", "intelligence": "Intelligence", "might": "Might", "will": "Will", "fate": "Fate"}
STAT_ORDER_KEYS = list(STAT_LABEL)


def _flat(options):
    for o in options:
        yield o
        if o["type"] == "oneOf":
            yield from _flat(o["options"])


def _list(names):
    return names[0] if len(names) == 1 else ", ".join(names[:-1]) + " and " + names[-1]


def add_offers(paths):
    """`offers` of every path: what taking the path gives a Hero, derived from the path's own data (nothing is invented), for the
    path picker's tooltip: the starting Heroic Action / Magical Power, the path's special rules, its Mastery rule (which improves a
    Heroic Action and comes with the path's signature characteristic) and the other choices (a Minor Special Rule, a new Heroic
    Action, Magical Powers)."""
    for p in paths:
        if not p["progression"]:  # On Their Own Path: no progression
            p["offers"] = {"summary": p.get("description", ""), "specialRules": [], "signatureStats": [], "choices": []}
            continue
        rules, mastery, signature = [], None, []
        for r in p["progression"]:
            row = list(_flat(r["options"]))
            names = [o["specialRule"] for o in row if o["type"] == "specialRule"]
            for n in names:
                if n not in rules:
                    rules.append(n)
            masteries = [n for n in names if n.startswith("Heroic ") and n.endswith(" Mastery")]
            if masteries:
                mastery = masteries[0]
                signature = [k for o in row if o["type"] == "improveStat" for k in o["stats"]]
        own = [r for r in rules if r != mastery]
        opts = [o for r in p["progression"] for o in _flat(r["options"])]
        choices = [c for c in ("minorSpecialRule", "heroicAction", "magicalPower", "improveCastingValue") if any(o["type"] == c for o in opts)]
        ha = (p.get("startingHeroicAction") or {}).get("name")
        offers = {"startingHeroicAction": ha, "startingMagicalPower": bool(p.get("startingMagicalPower")),
                  "masteryRule": mastery, "specialRules": own, "signatureStats": signature, "choices": choices}
        parts = []
        if ha:
            parts.append(f"Adds the Heroic Action {ha}" + (" and a Magical Power of your choice to the Spell List." if p.get("startingMagicalPower") else "."))
        elif mastery:
            parts.append("Adds no Heroic Action.")
        if mastery:
            action = mastery[: -len(" Mastery")]
            parts.append(f"{mastery} improves {action}" + ("." if ha else " for every Hero.") if ha else f"{mastery} improves the {action} every Hero has.")
        if own:
            parts.append("Path special rules: " + _list(own) + ".")
        if signature:
            parts.append("Signature characteristic: " + _list([STAT_LABEL[k] for k in signature]) + ".")
        extra = {"minorSpecialRule": "a Minor Special Rule", "heroicAction": "a new Heroic Action", "magicalPower": "a Magical Power",
                 "improveCastingValue": "a better Casting Value"}
        if choices:
            parts.append("Level-ups can also give " + _list([extra[c] for c in choices]) + ".")
        offers["summary"] = " ".join(parts)
        p["offers"] = offers
