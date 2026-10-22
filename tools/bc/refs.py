"""Which vanilla names does the Battle Companies file pull in (directly and through the vanilla units it uses)?"""
import collections


def referenced(bc, van):
    """Returns {'units': set, 'keywords': set, 'gear': set} of vanilla names needed to display the BC file."""
    bd, vd = bc["data"], van["data"]
    units, keywords = set(), set()

    def walk(o):
        if isinstance(o, dict):
            if isinstance(o.get("unit"), str):
                units.add(o["unit"])
            if isinstance(o.get("basedOn"), str):
                units.add(o["basedOn"])
            if isinstance(o.get("specialRule"), str):
                keywords.add(o["specialRule"])
            keywords.update(o.get("addSpecialRules", []))
            for v in o.values():
                walk(v)
        elif isinstance(o, list):
            for v in o:
                walk(v)

    walk(bd["companies"])
    walk(bd["paths"])
    walk(bd["minorSpecialRules"])
    for w in bd["wanderers"]["items"] + bd["creatures"]["items"]:
        keywords.update(w.get("specialRules", []))
    for e in bd["equipment"]["items"]:
        keywords.update(e.get("grantsSpecialRules", []))

    by_name = collections.defaultdict(list)
    for kind in ("warriors", "heroes"):
        for e in vd[kind]:
            by_name[e["name"]].append(e)
    gear = set()
    for u in units:
        for e in by_name.get(u, []):
            keywords.update(e.get("specialRules", []))
            gear.update(e.get("wargear", []))
            gear.update(o["name"] for o in e.get("options", []))
    return {"units": units, "keywords": keywords, "gear": gear}


def vanilla_duplicates(van):
    """{'units'|'keywords'|'gear': {name: [entries]}} for names defined more than once, in the order and with the
    case-insensitive matching of the Go loader (units: heroes then warriors; the group keeps the first spelling)."""
    vd = van["data"]
    out = {}
    for key, cols in (("units", ("heroes", "warriors")), ("keywords", ("keywords",)), ("gear", ("gear",))):
        groups = {}
        names = {}
        for c in cols:
            for e in vd[c]:
                n = names.setdefault(e["name"].lower(), e["name"])
                groups.setdefault(n, []).append((c, e))
        out[key] = {n: es for n, es in groups.items() if len(es) > 1}
    return out
