"""Parses the Armoury: Equipment, Creatures and Wanderers."""
import re

from .text import blocks, join, norm

EQUIP_RE = re.compile(
    r"^(?P<name>.+?)\s*(?:[….]{2,}|\.{2,})\s*(?P<size>Small|Large)\s*\((?P<r1>\d+)(?:\s*/\s*(?P<r2>\d+))?\)\s*$")
INFL_RE = re.compile(r"^(?P<n>\d+)\s+Influence Points?\s*(?:\((?P<who>[^)]*)\))?\s*$", re.I)
STAT_ORDER = ["movement", "fight", "shoot", "strength", "defence", "attack", "wounds", "courage", "intelligence",
              "might", "will", "fate"]


def rating(r1, r2):
    return {"warrior": int(r1), "hero": int(r2 if r2 else r1)}


def parse_equipment(lines, vanilla):
    items, cur = [], None
    for raw in lines:
        s = raw.strip()
        m = EQUIP_RE.match(norm(s))
        if m:
            cur = {"name": m["name"], "size": m["size"].lower(), "rating": rating(m["r1"], m["r2"]), "_text": []}
            tags = re.search(r"\s*\(([^)]*(?:Only|Unique)[^)]*)\)\s*$", cur["name"])
            if tags:
                cur["name"] = cur["name"][:tags.start()].strip()
                cur["restrictions"] = [t.strip().lower() for t in tags[1].split(",")]
            items.append(cur)
            continue
        if cur is None or not s or s.startswith("_") or s.startswith("Conversion Corner"):
            if s.startswith("Conversion Corner"):
                cur = None
            continue
        m = INFL_RE.match(norm(s))
        if m and "influence" not in cur:
            cur["influence"] = int(m["n"])
            if m["who"]:
                cur.setdefault("restrictions", []).append(m["who"].strip().lower())
            continue
        if re.match(r"^\*\s+|^\s*\*", raw):
            b = norm(re.sub(r"^\s*\*\s*", "", raw))
            if re.fullmatch(r"(Cavalry|Hero|Good|Evil|Unique)( Only)?", b, re.I):
                cur.setdefault("restrictions", []).append(b.lower())
            else:
                cur["_text"].append(b)
    out = []
    for it in items:
        text = it.pop("_text")
        if "restrictions" in it:
            it["restrictions"] = [re.sub(r" only$", "", r) for r in it["restrictions"]]
            it["restrictions"] = sorted(set(it["restrictions"]))
        it["definition"] = " ".join(text)
        grants = []
        for g in re.finditer(r"(?:has|gains?|gain)\s+the\s+(.+?)\s+special rules?", it["definition"], re.I):
            for part in re.split(r"\s*&\s*|\s+and\s+", g[1]):
                kw = vanilla.keyword(part.strip())
                if kw and kw not in grants:
                    grants.append(kw)
        if grants:
            it["grantsSpecialRules"] = grants
        out.append(it)
    return out


def read_stats(lines, i, count):
    """Header tokens then the same number of value lines; returns (values, next_index)."""
    i0 = i
    heads = []
    while len(heads) < count and i < len(lines):
        if lines[i].strip():
            heads.append(lines[i].strip())
        i += 1
    vals = []
    while len(vals) < count and i < len(lines):
        if lines[i].strip():
            vals.append(norm(lines[i]))
        i += 1
    assert heads[0] in ("Mv",), (i0, heads)
    return vals, i


def stat_value(v):
    v = v.strip()
    if v in ("-", "—"):
        return 0
    m = re.fullmatch(r'(\d+)"?', v)
    if m:
        return int(m[1])
    m = re.fullmatch(r"(\d+)\+", v)
    if m:
        return int(m[1])
    return v  # e.g. "*" (same as master)


def parse_creatures(lines):
    out, i = [], 0
    head_re = re.compile(r"^(?P<name>[A-Z][A-Za-z' -]+?) \((?P<types>[^)]*)\)\s*\.{2,}\s*(?P<pts>\d+) points$")
    while i < len(lines):
        m = head_re.match(norm(lines[i]))
        if not m:
            i += 1
            continue
        c = {"name": m["name"], "unitType": [t.strip() for t in m["types"].split(",")], "points": int(m["pts"])}
        i += 1
        while not lines[i].strip():
            i += 1
        im = INFL_RE.match(norm(lines[i]))
        c["influence"] = int(im["n"])
        i += 1
        vals, i = read_stats(lines, i, 9)
        for k, v in zip(STAT_ORDER[:9], vals):
            c[k] = stat_value(v)
        # bullets until next creature header or end
        bullets, para = [], []
        while i < len(lines) and not head_re.match(norm(lines[i])):
            s = lines[i]
            if re.match(r"^\s*\*\s+", s):
                bullets.append(norm(re.sub(r"^\s*\*\s+", "", s)))
            elif s.strip() and not s.strip().startswith("_") and bullets:
                bullets[-1] += " " + norm(s)
            i += 1
        for b in bullets:
            mb = re.match(r"^Base Size:\s*(.+)$", b)
            if mb:
                c["baseSize"] = [mb[1]]
                continue
            mb = re.match(r"^Special Rules:\s*(.+)$", b)
            if mb:
                c["specialRules"] = [x.strip() for x in mb[1].split(",")]
                continue
            c.setdefault("rules", []).append(b)
        out.append(c)
    return out


def parse_wanderers(lines, vanilla=None):
    out, i = [], 0
    types_re = re.compile(r"^\((?P<types>[^)]*)\)\s*[….]{2,}\s*(?P<pts>\d+) points$")
    while i < len(lines):
        s = norm(lines[i])
        nxt = norm(lines[i + 1]) if i + 1 < len(lines) else ""
        if s.startswith("Wandering ") and types_re.match(nxt):
            m = types_re.match(nxt)
            w = {"name": s, "unitType": [t.strip() for t in m["types"].split(",")], "points": int(m["pts"])}
            i += 2
            while not lines[i].strip():
                i += 1
            im = INFL_RE.match(norm(lines[i]))
            w["influence"] = int(im["n"])
            i += 1
            vals, i = read_stats(lines, i, 12)
            for k, v in zip(STAT_ORDER, vals):
                w[k] = stat_value(v)
            powers = []
            while i < len(lines):
                t = lines[i]
                tn = norm(t)
                if tn.startswith("Wandering ") and types_re.match(norm(lines[i + 1]) if i + 1 < len(lines) else ""):
                    break
                mb = re.match(r"^\s*\*\s+(.*)$", t)
                if mb:
                    b = norm(mb[1])
                    key, _, val = b.partition(":")
                    key = key.strip()
                    val = val.strip()
                    if key == "Wargear":
                        w["wargear"] = [x.strip() for x in val.split(",")]
                    elif key == "Heroic Action":
                        w["heroicActions"] = [x.strip() for x in val.split(",")]
                    elif key == "Special Rules":
                        rules = []
                        for x in re.split(r",\s*(?![^()]*\))", val):
                            x = x.strip()
                            ref = re.search(r"\s*\((see [^)]*)\)$", x)
                            if ref:
                                x = x[:ref.start()]
                                w.setdefault("ruleReferences", {})[x] = ref[1]
                            if vanilla and x.lower() == "backstabber":
                                x = "Backstabbers"
                            rules.append((vanilla.keyword(x) if vanilla else None) or x)
                        w["specialRules"] = rules
                    elif key == "Keyword":
                        w["keywordNote"] = val
                    elif key == "Magical Powers":
                        pass
                    else:
                        pm = re.match(r"^(.+?)\s*\((Self|\d+\"),\s*(\d+)\+\)$", b)
                        if pm:
                            powers.append({"magicalPower": (vanilla.power(pm[1]) if vanilla else None) or pm[1], "range": pm[2] if pm[2] == "Self" else int(pm[2].rstrip('"')),
                                           "casting": int(pm[3])})
                i += 1
            if powers:
                w["magicalPowers"] = powers
            out.append(w)
            continue
        i += 1
    return out
