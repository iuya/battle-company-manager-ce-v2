"""The concept graph of the three shipped rules files (docs/concept-graph.md): nodes, derived edges, checks.

Used by build_concept_graph.py (writes definitions/concepts/concept_graph.json) and validate_definitions.py. The nodes are the
terms of the ruleset's glossary exactly as the app builds it (internal/core: GlossaryExtras + BuildGlossaryWith); the port below
follows those two Go functions line by line, and internal/core/concept_graph_file_test.go checks it against the real thing
(ids, kinds and text hashes of every node).
"""
import hashlib
import json
import re
import unicodedata

GRAPH_VERSION = "1.0"
CURATED_TYPES = ("grants", "cancels", "lifts", "requires")
FILES = ("vanilla", "battleCompanies", "glossary")
SOURCE_FILE = {"Battle Companies": "battleCompanies", "Vanilla": "vanilla", "": "glossary"}
STAT_ORDER = [("movement", "Move"), ("fight", "Fight"), ("shoot", "Shoot"), ("strength", "Strength"), ("defence", "Defence"),
              ("attack", "Attacks"), ("wounds", "Wounds"), ("courage", "Courage"), ("intelligence", "Intelligence"),
              ("might", "Might"), ("will", "Will"), ("fate", "Fate")]
PARAMETER = re.compile(r"\s*\([^)]*\)\s*")


# ---- hashes (core.RulesetHash, core.ConceptTextHash) ----

def compact(raw: bytes) -> bytes:
    """json.Compact: drops the white space outside strings and touches nothing else."""
    out, in_str, esc = bytearray(), False, False
    for b in raw:
        if in_str:
            out.append(b)
            if esc:
                esc = False
            elif b == 0x5C:
                esc = True
            elif b == 0x22:
                in_str = False
        elif b == 0x22:
            in_str = True
            out.append(b)
        elif b not in (0x20, 0x09, 0x0A, 0x0D):
            out.append(b)
    return bytes(out)


def ruleset_hash(raw: bytes) -> str:
    return hashlib.sha256(compact(raw)).hexdigest()


def text_hash(text: str) -> str:
    return hashlib.sha256(" ".join(text.split()).encode("utf-8")).hexdigest()[:16]


# ---- the glossary as the app builds it ----

def _str(d, key):
    v = d.get(key) if isinstance(d, dict) else None
    return v if isinstance(v, str) else ""


def _int(d, key):
    v = d.get(key) if isinstance(d, dict) else None
    return v if isinstance(v, int) and not isinstance(v, bool) else 0


def _strs(d, key):
    v = d.get(key) if isinstance(d, dict) else None
    if not isinstance(v, list):
        return []
    out = []
    for x in v:
        if not isinstance(x, str):
            break
        out.append(x)
    return out


def _items(d, key):
    v = d.get(key) if isinstance(d, dict) else None
    items = v.get("items") if isinstance(v, dict) else None
    return [x for x in items if isinstance(x, dict)] if isinstance(items, list) else []


def _dicts(v):
    return [x for x in v if isinstance(x, dict)] if isinstance(v, list) else []


def tidy(s):
    return " ".join(s.split())


def stat_line(raw):
    parts = []
    for key, label in STAT_ORDER:
        v = raw.get(key)
        if isinstance(v, int) and not isinstance(v, bool) and (v != 0 or key == "attack"):
            parts.append(f"{label} {v}")
    return ", ".join(parts)


def glossary_extras(vanilla: dict, bc: dict):
    """core.GlossaryExtras: the terms the vanilla and Battle Companies files define."""
    out, variants = [], []

    def add(name, category, definition, source, aliases=()):
        name = name.strip()
        if name and tidy(definition):
            out.append({"name": name, "category": category, "aliases": list(aliases), "definition": definition.strip(), "source": source, "weak": False})

    if bc:
        src = "Battle Companies"
        data = bc.get("data") if isinstance(bc.get("data"), dict) else {}
        camp = bc.get("campaign") if isinstance(bc.get("campaign"), dict) else {}
        for k in _dicts(data.get("keywords")):
            label = _str(k, "label")
            if label and label != _str(k, "name"):
                variants.append((label, _str(k, "definition"), _str(k, "source")))
                continue
            add(_str(k, "name"), "special rules", _str(k, "definition"), src)
        for pr in _dicts(data.get("parameterisedRules")):
            pattern = _str(pr, "pattern")
            if pattern and _str(pr, "definition"):
                add(_str(pr, "name"), "special rules", "Written as " + pattern + ".\n\n" + _str(pr, "definition"), src)
        for p in _dicts(data.get("paths")):
            definition = _str(p, "description").strip() or _str(p, "startingNote").strip()
            if not definition:
                definition = "A heroic path of Battle Companies: the chart below is what a hero of this path rolls on when it levels up."
            act = p.get("startingHeroicAction")
            if isinstance(act, dict):
                definition += "\n\nStarting heroic action: " + _str(act, "name") + "."
                add(_str(act, "name"), "heroic actions", _str(act, "definition"), src)
            add(_str(p, "name"), "paths", definition, src)
            for row in _dicts(p.get("progression")):
                for o in _dicts(row.get("options")):
                    if _str(o, "type") == "specialRule" and _str(o, "specialRule"):
                        text = _str(row, "text")
                        if " - " in text:
                            text = text.split(" - ", 1)[1]
                        add(_str(o, "specialRule"), "path rules", text + "\n\nGained by heroes of the " + _str(p, "name") + ".", src)
        magic = data.get("magicalPowers")
        for pw in _items({"m": magic}, "m") if isinstance(magic, dict) else []:
            definition = _str(pw, "definition")
            if _int(pw, "casting") > 0:
                rng = pw.get("range")
                rng = f'{rng}"' if isinstance(rng, int) and not isinstance(rng, bool) and rng > 0 else "self"
                definition += f"\n\nBattle Companies Sorcerer: casting value {_int(pw, 'casting')}+, range {rng}. Duration: {_str(pw, 'duration')}."
            add(_str(pw, "name"), "magic", definition, src)
        notes = {}
        for a in _dicts(_items(data, "gearAnnotations")):
            n = {"name": _str(a, "name"), "aliases": _strs(a, "aliases"), "def": tidy(_str(a, "definition"))}
            for k in [n["name"]] + n["aliases"]:
                notes.setdefault(k.lower(), n)

        def gear_term(it, name, category, weak):
            if not name:
                return
            definition, aliases = "", []
            if name.lower() in notes:
                n = notes[name.lower()]
                definition, aliases, name = n["def"], n["aliases"], n["name"]
            if not definition:
                definition = tidy(_str(it, "definition"))
            price = []
            if _int(it, "influence") > 0:
                price.append(f"{_int(it, 'influence')} influence")
            gold = it.get("gold")
            if isinstance(gold, dict) and _int(gold, "warrior") + _int(gold, "hero") > 0:
                price.append(f"{_int(gold, 'warrior')} Gold for a warrior, {_int(gold, 'hero')} for a hero")
            extra = ""
            if _str(it, "size"):
                extra = f" It takes a {_str(it, 'size')} equipment slot."
            if price:
                extra = " Battle Companies armory: " + " or ".join(price) + "." + extra
            if not definition and not extra:
                return
            add(name, category, (definition + "\n\n" + extra.strip()).strip(), src, aliases)
            if weak and out and out[-1]["name"] == name:
                out[-1]["weak"] = True

        for it in _items(data, "wargear"):
            for n in _strs(it, "names"):
                gear_term(it, n, "wargear", True)
        for it in _items(data, "equipment"):
            gear_term(it, _str(it, "name"), "equipment", False)
        for c in _items(data, "creatures"):
            definition = stat_line(c)
            if _strs(c, "specialRules"):
                definition += "\n\nSpecial rules: " + ", ".join(_strs(c, "specialRules")) + "."
            definition += f"\n\nA creature for a leader or a sergeant. It costs {_int(c, 'influence')} influence and is worth {_int(c, 'points')} points."
            for n in _strs(c, "notes"):
                definition += "\n\n" + n
            add(_str(c, "name"), "creatures", definition, src)
        for w in _items(data, "wanderers"):
            definition = stat_line(w)
            if _strs(w, "wargear"):
                definition += "\n\nWargear: " + ", ".join(_strs(w, "wargear")) + "."
            if _strs(w, "heroicActions"):
                definition += "\n\nHeroic actions: " + ", ".join(_strs(w, "heroicActions")) + "."
            if _strs(w, "specialRules"):
                definition += "\n\nSpecial rules: " + ", ".join(_strs(w, "specialRules")) + "."
            definition += f"\n\nA mercenary hero for a company. It costs {_int(w, 'influence')} influence and is worth {_int(w, 'points')} points."
            add(_str(w, "name"), "wanderers", definition, src)
        inj = camp.get("injuries") if isinstance(camp.get("injuries"), dict) else {}
        for chart in (_dicts(inj.get("hero")), _dicts(inj.get("warrior"))):
            for i in chart:
                add(_str(i, "name"), "injuries", _str(i, "text"), src)
    if vanilla:
        src = "Vanilla"
        data = vanilla.get("data") if isinstance(vanilla.get("data"), dict) else {}
        for k in _dicts(data.get("keywords")):
            add(_str(k, "name"), "special rules", _str(k, "definition"), src)
        for p in _dicts(data.get("magicalPowers")):
            definition = _str(p, "definition")
            if _str(p, "duration"):
                definition += "\n\nDuration: " + _str(p, "duration") + "."
            add(_str(p, "name"), "magic", definition, src)
        for g in _dicts(data.get("gear")):
            name = _str(g, "name")
            if name.startswith("Exchange") or name.startswith("Replace"):
                continue
            if _str(g, "type") == "mount":
                definition = stat_line(g)
                if _strs(g, "specialRules"):
                    definition += "\n\nSpecial rules: " + ", ".join(_strs(g, "specialRules")) + "."
                add(name, "mounts", definition, src)
                continue
            add(name, "wargear", _str(g, "definition"), src)
    for label, definition, source in variants:
        i = next((n for n, t in enumerate(out) if t["name"] == label), -1)
        who = source or "Battle Companies"
        text = "Variant (" + who + "): " + definition.strip()
        if i >= 0:
            out[i]["definition"] += "\n\n" + text
        else:
            add(label, "special rules", text, "Battle Companies")
    return out


def slugify(s):
    out, dash = [], False
    for r in s.lower():
        if r < "\u0080" and (("a" <= r <= "z") or ("0" <= r <= "9")):
            out.append(r)
            dash = False
        elif not dash and out:
            out.append("-")
            dash = True
    return "".join(out).strip("-")


def _key(s):
    return s.strip().lower()


def glossary_terms(glossary: dict, extras):
    """core.BuildGlossaryWith, down to what a node needs: id, name, category, aliases, source and the definition text."""
    terms = [{"name": _str(t, "name"), "category": _str(t, "category"), "aliases": _strs(t, "aliases"), "definition": _str(t, "definition"), "from": ""}
             for t in _dicts(glossary.get("terms"))]
    owner = {}
    from_file = len(terms)
    for i, t in enumerate(terms):
        for n in [t["name"]] + t["aliases"]:
            owner[_key(n)] = i
    for x in extras:
        names = [x["name"]] + x["aliases"]
        at = next((owner[_key(n)] for n in names if _key(n) in owner), -1)
        if 0 <= at < from_file and x["weak"]:
            pass
        elif 0 <= at < from_file and x["source"] == "Battle Companies":
            t = terms[at]
            t["category"], t["definition"], t["from"] = x["category"], x["definition"], x["source"]
            for n in x["aliases"]:
                if n not in t["aliases"]:
                    t["aliases"].append(n)
                owner[_key(n)] = at
        elif at >= 0:
            pass
        else:
            terms.append({"name": x["name"], "category": x["category"], "aliases": list(x["aliases"]), "definition": x["definition"], "from": x["source"]})
            for n in names:
                owner[_key(n)] = len(terms) - 1
    terms.sort(key=lambda t: (_key(t["category"]).encode(), _key(t["name"]).encode()))  # stable, like slices.SortStableFunc
    used = set()
    for t in terms:
        base = slugify(t["name"])
        tid, n = base, 2
        while tid in used or tid == "":
            tid = f"{base}-{n}"
            n += 1
        used.add(tid)
        t["id"] = tid
        paras = [p.strip() for p in t["definition"].split("\n\n") if p.strip()]
        t["text"] = "\n\n".join(paras)
    return terms


def concept_kind(category):
    if category in ("special rules", "path rules"):
        return "rule"
    if category in ("wargear", "equipment", "creatures", "wanderers"):
        return "item"
    if category == "paths":
        return "path"
    if category == "magical powers":
        return "power"
    if category in ("scenario", "scenarios"):
        return "scenario"
    return "term"


def make_nodes(vanilla: dict, bc: dict, glossary: dict):
    nodes = []
    for t in glossary_terms(glossary, glossary_extras(vanilla, bc)):
        kind = concept_kind(t["category"])
        n = {"id": f"{kind}:{t['id']}", "kind": kind, "name": t["name"], "file": SOURCE_FILE.get(t["from"], "glossary"), "textHash": text_hash(t["text"])}
        if t["aliases"]:
            n["aliases"] = t["aliases"]
        n["_text"] = t["text"]
        nodes.append(n)
    return nodes


class Index:
    """Name lookup over the nodes, like core's byPhrase."""

    def __init__(self, nodes):
        self.nodes = {n["id"]: n for n in nodes}
        self.by_phrase = {}
        for n in nodes:
            seen = set()
            for p in [n["name"]] + n.get("aliases", []):
                k = _key(p)
                if k and k not in seen:
                    seen.add(k)
                    self.by_phrase.setdefault(k, []).append(n["id"])

    def ambiguous(self):
        return [{"alias": k, "candidates": sorted(v)} for k, v in sorted(self.by_phrase.items()) if len(v) > 1]

    def find(self, name, kind=None):
        """The one node a displayed name stands for, a rule with its parameter ("Terror (Cavalry)") included; None when unknown or ambiguous."""
        for k in (_key(name), _key(PARAMETER.sub(" ", name))):
            ids = self.by_phrase.get(k, [])
            if kind:
                ids = [i for i in ids if self.nodes[i]["kind"] == kind]
            if len(ids) == 1:
                return ids[0]
        return None

    def why_not(self, name):
        ids = self.by_phrase.get(_key(name)) or self.by_phrase.get(_key(PARAMETER.sub(" ", name))) or []
        return f"ambiguous ({', '.join(sorted(ids))})" if len(ids) > 1 else "no such term"


# ---- derived edges ----

def sentence_with(text, needle):
    """The sentence of text that holds needle (whole, collapsed), or None."""
    text = " ".join(text.split())
    for s in re.split(r"(?<=[.!?])\s+", text):
        if needle.lower() in s.lower():
            return s if len(s) <= 220 else None
    return None


def derive_edges(bc: dict, idx: Index):
    """grants, cancels, lifts and requires the Battle Companies data already states. Returns (edges, skipped) with skipped = [(edge, reason)]."""
    edges, skipped = [], []
    D = bc["data"]
    ann = D["gearAnnotations"]["items"]

    def node_of(name):
        return idx.find(name)

    def add(frm, to, typ, via=None, what=""):
        if frm is None or to is None:
            skipped.append((what, "unresolved"))
            return
        e = {"from": frm, "to": to, "type": typ}
        if via:
            e["via"] = via
        edges.append(e)

    by_name = {a["name"]: a for a in ann}

    def plain_weapon(name):
        """A standard weapon (a variant with a modifier such as Elven or poisoned, or a kind of hand-and-a-half weapon) that has no term of
        its own: it gets no node and no edge, by decision, so it is not reported as a gap."""
        a = by_name.get(name, {})
        return node_of(name) is None and (a.get("base") or "melee" in a.get("classes", []))

    # grants: an item gives its bearer a rule
    for a in ann:
        frm = node_of(a["name"])
        if plain_weapon(a["name"]):
            continue
        for rule in a.get("grants", []):
            to = node_of(rule)
            via = None
            if frm and to:
                via = sentence_with(idx.nodes[frm]["_text"], PARAMETER.sub("", rule).strip()) or None
            add(frm, to, "grants", via, f"{a['name']} grants {rule}: {'the item is ' + idx.why_not(a['name']) if not frm else 'the rule is ' + idx.why_not(rule)}")
    for it in D["equipment"]["items"]:  # same fact in the equipment table; duplicates are merged below
        for rule in it.get("grantsSpecialRules", []):
            add(node_of(it["name"]), node_of(rule), "grants", None, f"{it['name']} grants {rule}")
    # grants: a path gives its starting heroic action and the rules of its chart
    for p in D["paths"]:
        frm = node_of(p["name"])
        act = p.get("startingHeroicAction")
        if isinstance(act, dict) and act.get("name"):
            add(frm, node_of(act["name"]), "grants", f"Starting heroic action: {act['name']}.", f"{p['name']} grants {act['name']}")
        for row in p["progression"]:
            for o in row["options"]:
                if o["type"] == "specialRule" and o.get("specialRule"):
                    add(frm, node_of(o["specialRule"]), "grants", f"Gained by heroes of the {p['name']}.", f"{p['name']} grants {o['specialRule']}")
    # cancels and lifts: the defence bonus of a shield (cancelledByClasses/Items, liftedByRules)
    by_class = {}
    for a in ann:
        for c in a.get("classes", []):
            by_class.setdefault(c, []).append(a["name"])
    for a in ann:
        shield = node_of(a["name"])
        for ab in a.get("abilities", []):
            if ab.get("key") != "defenceBonus":
                continue
            for c in ab.get("cancelledByClasses", []):
                for item in by_class.get(c, []):
                    if item != a["name"] and not plain_weapon(item):
                        add(node_of(item), shield, "cancels", f"defence bonus is lost with a {c.replace('_', '-')} weapon", f"{item} cancels {a['name']}")
            for item in ab.get("cancelledByItems", []):
                add(node_of(item), shield, "cancels", f"defence bonus is lost with a {item}", f"{item} cancels {a['name']}")
            for lift in ab.get("liftedByRules", []):
                what = ", ".join(lift.get("cancels", []))
                via = f"lifts the loss of the defence bonus caused by: {what}" + (" (while mounted)" if lift.get("whileMounted") else "")
                add(node_of(lift["rule"]), shield, "lifts", via, f"{lift['rule']} lifts {a['name']}")
    # requires: gear only a mounted model can use, equipment restricted to a kind of model
    cavalry = idx.find("Cavalry")
    for a in ann:
        if a.get("requiresMount") and not plain_weapon(a["name"]):
            add(node_of(a["name"]), cavalry, "requires", "restriction: cavalry", f"{a['name']} requires Cavalry")
    hero = idx.find("Hero")
    for it in D["equipment"]["items"]:
        for r in it.get("restrictions", []):
            target = {"cavalry": cavalry, "hero": hero}.get(r)
            if r in ("evil", "unique"):
                continue  # armoury restrictions only: no term, no edge
            if not target:
                skipped.append((f"{it['name']} requires {r}", "restriction with no term to point at"))
            else:
                add(node_of(it["name"]), target, "requires", f"restriction: {r}", f"{it['name']} requires {r}")
    return edges, skipped


def curated_edges(curated: dict, idx: Index):
    """Resolves tools/concepts_curated.json (names) to edges; every `via` must be a quote of the text of one of the two nodes."""
    out, errors = [], []
    for c in curated["edges"]:
        frm, to = idx.find(c["from"]), idx.find(c["to"])
        label = f"{c['from']} {c['type']} {c['to']}"
        if not frm or not to:
            errors.append(f"curated edge {label}: {idx.why_not(c['from']) if not frm else idx.why_not(c['to'])}")
            continue
        haystack = " ".join(" ".join((idx.nodes[frm]["_text"] + " " + idx.nodes[to]["_text"]).split()).split())
        if " ".join(c["via"].split()) not in haystack:
            errors.append(f"curated edge {label}: via is not a quote of either text: {c['via']!r}")
            continue
        out.append({"from": frm, "to": to, "type": c["type"], "via": c["via"]})
    return out, errors


def order_edges(edges):
    seen, out = set(), []
    for e in sorted(edges, key=lambda e: (e["from"], e["to"], e["type"], "via" not in e, e.get("via", ""))):
        k = (e["from"], e["to"], e["type"])
        if k not in seen and e["from"] != e["to"]:
            seen.add(k)
            out.append(e)
    return out


def build_graph(raws, curated, name="Concept graph"):
    """raws = (vanilla, bc, glossary) as bytes. Returns (document, report) with report = {'skipped': [...], 'errors': [...]}."""
    docs = [json.loads(r) for r in raws]
    nodes = make_nodes(*docs)
    idx = Index(nodes)
    derived, skipped = derive_edges(docs[1], idx)
    hand, errors = curated_edges(curated, idx)
    derived_keys = {(e["from"], e["to"], e["type"]) for e in derived}
    for e in hand:
        if (e["from"], e["to"], e["type"]) in derived_keys:
            errors.append(f"curated edge {e['from']} {e['type']} {e['to']} is already derived from the data: remove it from the curated list")
    edges, kept = [], []
    for e in derived + hand:  # a direction violation is a skipped edge, listed, never silently dropped
        if idx.nodes[e["from"]]["file"] in ("vanilla", "glossary") and idx.nodes[e["to"]]["file"] == "battleCompanies":
            skipped.append((f"{e['from']} {e['type']} {e['to']}", "direction: a vanilla or glossary node cannot point into Battle Companies"))
        else:
            kept.append(e)
    edges = order_edges(kept)
    doc = {
        "name": name, "version": GRAPH_VERSION,
        "builtFrom": dict(zip(FILES, (ruleset_hash(r) for r in raws))),
        "nodes": [{k: v for k, v in n.items() if not k.startswith("_")} for n in nodes],
        "edges": edges,
        "ambiguous": idx.ambiguous(),
    }
    doc["nodes"].sort(key=lambda n: n["id"])
    return doc, {"skipped": skipped, "errors": errors}


# ---- checks (validate_definitions.py) ----

def check_graph(doc, raws):
    """The checks of docs/concept-graph.md on a graph document against the three files it should describe (as bytes)."""
    errors = []
    docs = [json.loads(r) for r in raws]
    nodes = make_nodes(*docs)
    truth = {n["id"]: n for n in nodes}
    now = dict(zip(FILES, (ruleset_hash(r) for r in raws)))
    for f in FILES:
        if doc.get("builtFrom", {}).get(f) != now[f]:
            errors.append(f"builtFrom.{f} is stale: the file changed since the graph was built (run tools/build_concept_graph.py)")
    ids = set()
    for n in doc.get("nodes", []):
        if n["id"] in ids:
            errors.append(f"duplicate node {n['id']}")
        ids.add(n["id"])
        t = truth.get(n["id"])
        if t is None:
            errors.append(f"node {n['id']} does not resolve to an entry of the files")
            continue
        if n.get("file") != t["file"]:
            errors.append(f"node {n['id']}: file {n.get('file')!r} but the entry lives in {t['file']!r}")
        if n.get("name") != t["name"] or n.get("kind") != t["kind"]:
            errors.append(f"node {n['id']}: name or kind differs from the entry")
        if n.get("textHash") != t["textHash"]:
            errors.append(f"node {n['id']}: textHash is stale")
    for i in sorted(set(truth) - ids):
        errors.append(f"node {i} is missing from the graph")
    if doc.get("ambiguous") != Index(nodes).ambiguous():
        errors.append("ambiguous[] is not the list of aliases claimed by more than one node")
    file_of = {n["id"]: n.get("file") for n in doc.get("nodes", [])}
    seen = set()
    for e in doc.get("edges", []):
        label = f"edge {e.get('from')} {e.get('type')} {e.get('to')}"
        if e.get("type") not in CURATED_TYPES:
            errors.append(f"{label}: only {', '.join(CURATED_TYPES)} are stored (mentions and see are computed by the app)")
        for end in ("from", "to"):
            if e.get(end) not in ids:
                errors.append(f"{label}: {end} is not a node")
        k = (e.get("from"), e.get("to"), e.get("type"))
        if k in seen:
            errors.append(f"{label}: duplicate")
        seen.add(k)
        if e.get("from") == e.get("to"):
            errors.append(f"{label}: self link")
        if file_of.get(e.get("from")) in ("vanilla", "glossary") and file_of.get(e.get("to")) == "battleCompanies":
            errors.append(f"{label}: points from {file_of[e['from']]} into Battle Companies")
        if e.get("type") in ("cancels", "lifts", "requires") and not str(e.get("via", "")).strip():
            errors.append(f"{label}: needs a via")
    return errors
