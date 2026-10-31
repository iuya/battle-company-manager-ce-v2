"""The concept graph of the three shipped rules files (docs/concept-graph.md): nodes, derived edges, checks.

Used by build_concept_graph.py (writes definitions/concepts/concept_graph.json) and validate_definitions.py. The nodes are the
terms of the ruleset's glossary exactly as the app builds it; the app's own loader makes them (`go run ./tools/conceptnodes`, which
calls core.LoadRuleSet) so that this file never repeats that logic, and internal/core/concept_graph_file_test.go checks every node
of the generated file against core. The edges are derived here from the lean Battle Companies file (gear, paths) and from
tools/concepts_curated.json.
"""
import hashlib
import json
import os
import re
import subprocess
import tempfile
from pathlib import Path

GRAPH_VERSION = "1.0"
CURATED_TYPES = ("grants", "cancels", "lifts", "requires")
FILES = ("vanilla", "battleCompanies", "glossary")
SOURCE_FILE = {"Battle Companies": "battleCompanies", "Vanilla": "vanilla", "": "glossary"}
PARAMETER = re.compile(r"\s*\([^)]*\)\s*")
ROOT = Path(__file__).resolve().parent.parent

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



# ---- the nodes: the glossary as the app builds it ----

_LOADED = {}


def load_terms(raws):
    """(terms, gear_classes) of the three files (bytes) as core.LoadRuleSet makes them: runs `go run ./tools/conceptnodes`."""
    key = tuple(hashlib.sha256(r).hexdigest() for r in raws)
    if key not in _LOADED:
        with tempfile.TemporaryDirectory() as tmp:
            paths = []
            for name, raw in zip(FILES, raws):
                p = Path(tmp) / f"{name}.json"
                p.write_bytes(raw)
                paths.append(str(p))
            r = subprocess.run(["go", "run", "./tools/conceptnodes", *paths], cwd=ROOT, capture_output=True, text=True, encoding="utf-8", env={**os.environ, "GOTOOLCHAIN": "local"})
        if r.returncode != 0:
            raise RuntimeError("go run ./tools/conceptnodes failed (the nodes come from the app's loader; is go on the PATH?): " + r.stderr.strip()[:400])
        out = json.loads(r.stdout)
        _LOADED[key] = (out["terms"], out["gearClasses"])
    return _LOADED[key]


def _key(s):
    return s.strip().lower()

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



def make_nodes(raws):
    terms, _ = load_terms(raws)
    nodes = []
    for t in terms:
        kind = concept_kind(t["category"])
        n = {"id": f"{kind}:{t['id']}", "kind": kind, "name": t["name"], "file": SOURCE_FILE.get(t["source"], "glossary"), "textHash": text_hash(t["text"])}
        if t.get("aliases"):
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

    def texts(self, name):
        """The text of every node named `name` (parameter ignored), joined; None when no node has that name."""
        ids = self.by_phrase.get(_key(name)) or self.by_phrase.get(_key(PARAMETER.sub(" ", name))) or []
        return " ".join(self.nodes[i]["_text"] for i in ids) if ids else None

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


def derive_edges(bc: dict, idx: Index, gear_classes: dict):
    """grants, cancels, lifts and requires the lean Battle Companies file already states. Returns (edges, skipped) with skipped = [(edge, reason)]."""
    edges, skipped = [], []
    gear = bc["gear"]

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

    def classes(g):
        return g.get("classes") or gear_classes.get(g["name"].lower(), [])

    def plain_weapon(g):
        """A standard weapon (a variant with a modifier such as Elven or poisoned, or a kind of hand-and-a-half weapon) that has no term of
        its own: it gets no node and no edge, by decision, so it is not reported as a gap."""
        return node_of(g["name"]) is None and (g.get("base") or "melee" in classes(g))

    by_name = {g["name"]: g for g in gear}
    # grants: an item gives its bearer a rule
    for g in gear:
        frm = node_of(g["name"])
        if plain_weapon(g):
            continue
        for rule in g.get("grants", []):
            to = node_of(rule)
            via = None
            if frm and to:
                via = sentence_with(idx.nodes[frm]["_text"], PARAMETER.sub("", rule).strip()) or None
            add(frm, to, "grants", via, f"{g['name']} grants {rule}: {'the item is ' + idx.why_not(g['name']) if not frm else 'the rule is ' + idx.why_not(rule)}")
    # grants: a path gives its starting heroic action and the rules of its chart
    for p in bc["paths"]:
        frm = node_of(p["name"])
        act = p.get("heroicAction")
        if act:
            add(frm, node_of(act), "grants", f"Starting heroic action: {act}.", f"{p['name']} grants {act}")
        for row in p.get("rows", []):
            for o in row["options"]:
                rule = o.get("rule")
                rule = rule.get("name") if isinstance(rule, dict) else rule
                if rule:
                    add(frm, node_of(rule), "grants", f"Gained by heroes of the {p['name']}.", f"{p['name']} grants {rule}")
    # cancels and lifts: the defence bonus of a shield (cancelledByClasses/Items, liftedByRules)
    by_class = {}
    for g in gear:
        for c in classes(g):
            by_class.setdefault(c, []).append(g)
    for g in gear:
        shield = node_of(g["name"])
        ab = (g.get("abilities") or {}).get("defenceBonus")
        if not ab:
            continue
        for c in ab.get("cancelledByClasses", []):
            for other in by_class.get(c, []):
                if other["name"] != g["name"] and not plain_weapon(other):
                    add(node_of(other["name"]), shield, "cancels", f"defence bonus is lost with a {c.replace('_', '-')} weapon", f"{other['name']} cancels {g['name']}")
        for item in ab.get("cancelledByItems", []):
            add(node_of(item), shield, "cancels", f"defence bonus is lost with a {item}", f"{item} cancels {g['name']}")
        for lift in ab.get("liftedByRules", []):
            what = ", ".join(lift.get("cancels", []))
            via = f"lifts the loss of the defence bonus caused by: {what}" + (" (while mounted)" if lift.get("whileMounted") else "")
            add(node_of(lift["rule"]), shield, "lifts", via, f"{lift['rule']} lifts {g['name']}")
    # requires: gear only a mounted model can use, equipment restricted to a kind of model
    cavalry = idx.find("Cavalry")
    for g in gear:
        if g.get("requiresMount") and not plain_weapon(g):
            add(node_of(g["name"]), cavalry, "requires", "restriction: cavalry", f"{g['name']} requires Cavalry")
    hero = idx.find("Hero")
    for g in gear:
        who = g.get("who") or {}
        restrictions = (["hero"] if who.get("hero") is True else []) + (["cavalry"] if "Cavalry" in who.get("unitTypes", []) else [])
        for r in restrictions:
            target = {"cavalry": cavalry, "hero": hero}[r]
            if not target:
                skipped.append((f"{g['name']} requires {r}", "restriction with no term to point at"))
            else:
                add(node_of(g["name"]), target, "requires", f"restriction: {r}", f"{g['name']} requires {r}")
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
    nodes = make_nodes(raws)
    _, gear_classes = load_terms(raws)
    idx = Index(nodes)
    derived, skipped = derive_edges(json.loads(raws[1]), idx, gear_classes)
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
    nodes = make_nodes(raws)
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
