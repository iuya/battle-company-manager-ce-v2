"""Checks of the lean Battle Companies file that the Go validator (`go run ./cmd/rulecheck`) does not make. Used by validate_definitions.py;
tests in test_lean_checks.py.

* company texts: no rule repeated inside a company's `specialRules`/`notes`, no parse artefact in the texts copied from the rulebook
  (unbalanced quotes, an epigraph glued to a rule, a text starting or ending mid-sentence, a page header);
* aura rings (docs/aura-ranges.md): the vocabulary tables are the specified ones, a ring that names its group follows its `when` and
  radius, and the radius is written in the text of the thing that draws it (or of its concept, merged labels or items);
* rule targets: the keywords a target names are keywords of the vanilla units (race, faction, unit type) or of Battle Companies' own mounts;
* Tabletop Simulator: every model a piece of gear names (the stand-ins of the wanderers, creatures and mounts) is in the local catalogue `sources/tts/models.json` when that gitignored file exists.

Every function takes the parsed lean document (`bc`) and returns a list of error strings.
"""
import json
import re
import unicodedata
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
TTS_CATALOGUE = ROOT / "sources/tts/models.json"  # local, gitignored; never committed


def norm(t):
    return re.sub(r"\W+", " ", t.lower()).strip()


def fold(s):
    s = unicodedata.normalize("NFKD", s.replace("’", "'"))
    s = "".join(c for c in s if not unicodedata.combining(c)).lower()
    return re.sub(r"[^a-z0-9]+", " ", s).strip()


# ---- company texts ----

QUOTES = '"“”'
FRAGMENT_END = re.compile(r"(?:[,;:–—-]|\b(?:and|or|the|a|an|of|to|in|on|for|with|than|that|which|as|if|but|while|rather))$", re.I)
EPIGRAPH = re.compile(r"^\s*[" + QUOTES + r"].{3,200}?[" + QUOTES + r"]\s*[–—-]")
PAGE_FURNITURE = re.compile(r"Battle Companies:?\s*2nd|Community Edition\s*-|^\s*Page\s+\d+\s*$|^\s*\d+\s*/\s*\d+\s*$", re.I)


def balanced(t):
    """Quotation marks and brackets pair up (typographic quotes pair in order, straight quotes in twos)."""
    if t.count("“") != t.count("”") or t.count("(") != t.count(")") or t.count("[") != t.count("]"):
        return False
    return t.count('"') % 2 == 0


def artefacts(t):
    """What looks wrong with a rule or note text, as a list of short reasons."""
    out = []
    s = t.strip()
    if not s:
        return ["empty"]
    if not balanced(s):
        out.append("unbalanced quotation marks or brackets")
    if EPIGRAPH.match(s):
        out.append("starts with a quoted epigraph and a dash")
    if re.match(r"^[–—-]\s", s) or re.match(r"^[a-z]", s) or re.match(r"^[,;:.)]", s):
        out.append("starts mid-sentence")
    if FRAGMENT_END.search(s):
        out.append("ends mid-sentence")
    if PAGE_FURNITURE.search(s):
        out.append("contains a page header or footer")
    return out


def _note_text(n):
    return n["text"] if isinstance(n, dict) else n


def company_texts(c):
    """(where, text) of every sentence of a company that is copied from the rulebook as running text."""
    for i, r in enumerate(c.get("specialRules") or []):
        yield f"specialRules[{i}]", r
    for i, n in enumerate(c.get("notes") or []):
        yield f"notes[{i}]", _note_text(n)
    if (c.get("oath") or {}).get("text"):
        yield "oath.text", c["oath"]["text"]
    for i, p in enumerate((c.get("description") or "").split("\n\n")):
        if p.strip():
            yield f"description[{i}]", p
    ep = c.get("epigraph")
    if ep:
        yield "epigraph.quote", ep["quote"]
        yield "epigraph.text", ep["text"]
    for i, h in enumerate(c.get("heroUpgrades") or []):
        for k in ("flavour", "text"):
            if h.get(k):
                yield f"heroUpgrades[{i}].{k}", h[k]


def company_text_errors(bc):
    errors = []
    for c in bc["companies"]:
        name = c["name"]
        for where, t in company_texts(c):
            if where == "epigraph.quote":
                continue
            for why in artefacts(t):
                errors.append(f"company {name!r} {where}: {why}: {t[:80]!r}")
        rules = [(f"specialRules[{i}]", r) for i, r in enumerate(c.get("specialRules") or [])]
        notes = [(f"notes[{i}]", _note_text(n)) for i, n in enumerate(c.get("notes") or [])]
        for items in (rules + notes, notes):
            for a in range(len(items)):
                for b in range(a + 1, len(items)):
                    ka, kb = norm(items[a][1]), norm(items[b][1])
                    if ka and kb and (ka == kb or ka in kb or kb in ka):
                        errors.append(f"company {name!r}: {items[a][0]} and {items[b][0]} repeat the same text: {items[a][1][:80]!r}")
            if items is notes:
                break
        if c.get("epigraph") and any(norm(c["epigraph"]["text"]) in norm(r) for _, r in rules):
            errors.append(f"company {name!r}: the epigraph is repeated in specialRules")
    return errors


# ---- aura rings ----

AURA_KINDS = ["banner", "command", "support", "enemy", "utility"]
AURA_GROUP_DEFAULTS = {"always": True, "declared": True, "conditional": True, "cast": False, "target": False, "large": False, "optional": False}
LARGE = 12  # a ring larger than this many inches is in the "large" group
# A ring whose radius is a sum: (owner, radius) -> the parts each written in the text, and the other terms whose text is read too
# (the war horn adds 3" to a Hero's Stand Fast of 6").
RADIUS_SUMS = {("War Horn", 9): {"parts": [6, 3], "also": ["Stand Fast"]}}


def has(n, text):
    """True when the number `n` (inches) is written in the text as N" (not as part of a longer number)."""
    return re.search(rf'(?<![\d.]){re.escape(str(n))}\s*["”]', text) is not None


def group_of(ring):
    """The group a ring belongs to when it names none: its `when`; a spell in effect or a targeting range keeps its own group, a ring
    larger than LARGE inches is a large ring."""
    if ring["when"] in ("target", "cast"):
        return ring["when"]
    if ring["radius"] > LARGE:
        return "large"
    return ring["when"]


def rings(bc):
    """(owner kind, owner name, ring) of every ring of the document."""
    for r in bc.get("rules", []):
        for a in r.get("auras", []):
            yield "rule", r["name"], a
    for g in bc.get("gear", []):
        for a in g.get("auras", []):
            yield "item", g["name"], a
    for n, p in (bc.get("magic", {}).get("powers") or {}).items():
        for a in p.get("auras", []):
            yield "power", n, a


def aura_errors(bc, text_of, van=None):
    """text_of(name): the text of the glossary terms of that name, or None when there is none (the app's loader makes the glossary;
    concept_graph.Index.texts); van: the vanilla file, whose rule texts are read too (a glossary term of the same name may say something else,
    like the path rule Lead by Example)."""
    errors = []
    a = bc.get("auras")
    if a is None:
        return ["auras is missing"]
    if [k["id"] for k in a.get("kinds", [])] != AURA_KINDS:
        errors.append(f"auras.kinds must be {AURA_KINDS} in this order")
    for k in a.get("kinds", []):
        if not k.get("default", True):
            errors.append(f"auras kind {k['id']!r}: every kind is on by default (docs/aura-ranges.md 4.4)")
    if {g["id"]: g.get("default") for g in a.get("groups", [])} != AURA_GROUP_DEFAULTS:
        errors.append(f"auras.groups must be exactly {AURA_GROUP_DEFAULTS} (docs/aura-ranges.md section 9)")
    vanilla_rules = {}
    for k in ((van or {}).get("data") or {}).get("keywords", []):
        vanilla_rules.setdefault(fold(k["name"]), k.get("definition", ""))
    lean_rules = {fold(r["name"]): r.get("definition", "") for r in bc.get("rules", [])}

    def owner_text(n):
        extra = [lean_rules.get(fold(n)) or vanilla_rules.get(fold(n)) or ""]
        t = text_of(n)
        return None if t is None and not extra[0] else " ".join([t or ""] + extra)

    for kind, owner, ring in rings(bc):
        w = f"aura ring of {kind} {owner!r}" + (f" ({ring['label']})" if ring.get("label") else "")
        want = group_of(ring)
        group = ring.get("group")
        if group and group != want and not (group == "optional" and ring["when"] == "always" and ring["kind"] == "utility"):
            errors.append(f"{w}: group {group!r} but when {ring['when']!r} and radius {ring['radius']} make it {want!r}")
        if len(ring.get("tip", "")) > 200:
            errors.append(f"{w}: tip is longer than a sentence (200 characters)")
        sums = RADIUS_SUMS.get((owner, ring["radius"]), {})
        named = [x for x in [ring.get("concept")] + ring.get("items", []) + ring.get("mergeLabels", []) if x]
        pool = [owner_text(owner)] + [owner_text(x) for x in named + sums.get("also", [])]
        for x in named:
            if owner_text(x) is None:
                errors.append(f"{w}: {x!r} is not a term of the glossary")
        union = " ".join(t for t in pool if t)
        parts = sums.get("parts", [ring["radius"]])
        for part in parts:
            if not has(part, union):
                errors.append(f"{w}: the radius {part}\" is not written in the text of {owner!r} or of its concept")
    return errors


# ---- rule targets ----

def keyword_vocabulary(van):
    """Every keyword a vanilla model can have: race, faction, unit type, and the profile's own kind (Hero, Warrior)."""
    out = {"hero", "warrior"}
    for k in ("heroes", "warriors"):
        for u in van["data"][k]:
            for f in ("race", "faction", "unitType"):
                out.update(fold(x) for x in u.get(f) or [] if isinstance(x, str))
    return out


def rule_target_errors(bc, van):
    errors = []
    vocab = keyword_vocabulary(van) | {fold(x) for m in (bc.get("vanilla", {}).get("mounts") or {}).values() for f in ("race", "unitType") for x in m.get(f, [])}  # BC's own mounts
    for r in bc.get("rules", []):
        for field in ("target", "bareTarget"):
            t = r.get(field)
            if not t:
                continue
            for kw in t.get("keywords", []):
                if fold(kw) not in vocab:
                    errors.append(f"rule {r['name']!r}: {field} keyword {kw!r} is not a keyword of any vanilla model")
    return errors


# ---- Tabletop Simulator ----

def tts_catalogue(path):
    """The local catalogue's model keys ('Name: options') as (name, options) folded pairs, or None when the file does not exist."""
    if not Path(path).exists():
        return None
    with open(path, encoding="utf-8") as f:
        raw = json.load(f)
    keys = raw["models"] if isinstance(raw, dict) else raw
    return {(fold(k.partition(":")[0]), fold(k.partition(":")[2])) for k in keys}


def tts_models(bc):
    """(where, model key) of every Tabletop Simulator model the file names."""
    for g in bc.get("gear", []):
        t = g.get("tts")
        if t:
            for m in [t.get("model")] + t.get("alternatives", []):
                if m:
                    yield f"gear {g['name']!r}", m


def tts_errors(bc, catalogue=None):
    """Every model the file names is in the local catalogue (a no-op without one)."""
    if catalogue is None:
        return []
    errors = []
    for where, m in tts_models(bc):
        k = (fold(m.partition(":")[0]), fold(m.partition(":")[2]))
        if k not in catalogue and (k[0], "") not in catalogue:
            errors.append(f"{where}: model {m!r} is not in the local Tabletop Simulator catalogue")
    return errors
