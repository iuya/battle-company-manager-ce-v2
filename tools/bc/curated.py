"""What the shipped Battle Companies file does not carry: curator-only data, unread sections and reference prose.

`split(doc)` takes the complete document the generator assembles and cuts it in place into the file the app loads. What it cuts
goes to three places:

- `store` (written to tools/data/*.json by the generator): provenance and working data of the curator and of the audit tooling:
  the notes of the reviewed vanilla duplicates, the aura audit, the evidence of the gear classes, the sources of the option
  sales, the reviews of the fill-ins, the reasons of the Tabletop Simulator mappings and the `source` / `note` fields of single
  entries. The validator and the tests read them back with `attach`.
- `reference` (written to definitions/battle_companies/reference/*.json, never loaded by the app): rulebook prose that no code
  reads and that is worth keeping next to the data (influence, gold, designer notes).
- nothing: sections that nobody reads and that the generator source still has (`resolution`, `validations`,
  `campaign.customCompany`, the prose of `campaign.recruitment`).

`attach(shipped, store)` puts the curator data back, giving the "audit view" the validator and the tools check (the complete
document minus the dropped sections and the reference prose). `tools/test_curated.py` proves `attach(split(x)) == x` for them.
"""
import copy
import json
from pathlib import Path

DROPPED_TOP = ("resolution", "validations")

# campaign.recruitment keeps the parameters the rule engine reads; every other key is prose or a description of fixed mechanics.
RECRUITMENT_ADJUST_DROP = ("text",)
RECRUITMENT_RARE_KEEP = ("injuredUnitsCount", "deadUnitsCount", "promotedUnitsCount")
# campaign.influence keeps what is read: the gains, and the "adjustRoll" entry of spend[] (its numbers are recruitment.adjust).
INFLUENCE_KEEP = ("gains",)
SPEND_KEEP = ("adjustRoll",)


def _pop(d, key, default=None):
    return d.pop(key) if key in d else default


def split(doc):
    """Cuts `doc` in place; returns (store, reference)."""
    store, ref = {}, {}
    for k in DROPPED_TOP:
        doc.pop(k, None)
    camp, data = doc["campaign"], doc["data"]
    camp.pop("customCompany", None)
    _recruitment(camp)
    _influence(camp, ref)
    ref["gold"] = _pop(camp, "gold")
    ref["pathNotes"] = _pop(data, "pathNotes")
    _provenance(doc, store)
    _duplicates(data, store)
    _auras(data, store)
    _gear(data, store)
    _option_sales(data, store)
    _fill_ins(data, store)
    _tts(data, store)
    return store, {k: v for k, v in ref.items() if v is not None}


def _recruitment(camp):
    r = camp["recruitment"]
    for k in RECRUITMENT_ADJUST_DROP:
        r["adjust"].pop(k, None)
    r["rare"] = {k: r["rare"][k] for k in RECRUITMENT_RARE_KEEP}
    camp["recruitment"] = {"die": r["die"], "adjust": r["adjust"], "rare": r["rare"]}


def _influence(camp, ref):
    inf = camp["influence"]
    ref["influence"] = {k: v for k, v in inf.items() if k not in INFLUENCE_KEEP and k != "spend"}
    ref["influence"]["spend"] = [s for s in inf["spend"] if s["key"] not in SPEND_KEEP]
    camp["influence"] = {"gains": inf["gains"], "spend": [s for s in inf["spend"] if s["key"] in SPEND_KEEP]}


# ---- single fields: (address, field) -> value, keyed by the name of the entry -------------------------------------------------

def _sites(doc):
    """Every place that may carry a provenance field: (entry, field, address of the entry's field)."""
    camp, data = doc["campaign"], doc["data"]
    for k in ("breakPoint", "progression.limits", "progression.warrior.effectsByKey.promoted.promotion", "injuries.rollModifier",
              "matchInput.aftermath", "matchInput.result"):
        o = camp
        for part in k.split("."):
            o = o[part]
        yield o, "source", f"campaign.{k}.source"
    for kind in ("perCompany", "perModel"):
        for i, f in enumerate(camp["matchInput"][kind]):
            yield f, "source", f"campaign.matchInput.{kind}[{i}].source"
    for o in camp["options"]:
        yield o, "source", f"campaign.options[{o['key']}].source"
    for e in camp["aftermathEffects"]:
        yield e, "note", f"campaign.aftermathEffects[{e['key']}].note"
    for kw in data["keywords"]:  # the source of a rule with a label is shown by the rules popup: it stays
        if "label" not in kw:
            yield kw, "source", f"data.keywords[{kw['name']}].source"
    for p in data["paths"]:
        yield p, "source", f"data.paths[{p['name']}].source"
        for r, row in enumerate(p["progression"]):
            for j, o in enumerate(row["options"]):
                yield o, "note", f"data.paths[{p['name']}].progression[{r}].options[{j}].note"
    for m in data["gearAnnotations"]["modifiers"]:
        yield m, "source", f"data.gearAnnotations.modifiers[{m['key']}].source"


def _provenance(doc, store):
    """`source` and `note` strings that say where a value comes from, stored under the address of their field."""
    store["provenance"] = {addr: obj.pop(field) for obj, field, addr in _sites(doc) if field in obj}


# ---- the reviewed duplicates of the vanilla file -----------------------------------------------------------------------------

def _duplicates(data, store):
    """The loader reads kind, name, status and occurrence; the note and the number of occurrences are the curator's record."""
    full = data["vanillaAmbiguities"]
    store["duplicate_decisions"] = copy.deepcopy(full)
    data["vanillaAmbiguities"] = [{k: v for k, v in a.items() if k in ("kind", "name", "status", "occurrence")} for a in full]


# ---- aura working data -------------------------------------------------------------------------------------------------------

def _auras(data, store):
    a = data["auras"]
    store["aura_audit"] = {
        "text": a.pop("text"),
        "entryReasons": {e["id"]: e.pop("reason") for e in a["entries"] if "reason" in e},
        "excluded": a.pop("excluded"),
        "outOfScope": a.pop("outOfScope"),
        "unreachable": a.pop("unreachable"),
    }


# ---- gear classification evidence --------------------------------------------------------------------------------------------

def _gear(data, store):
    ev = {}
    for g in data["gearAnnotations"]["items"]:
        e = {k: g.pop(k) for k in ("evidence", "status", "isA") if k in g}
        for i, r in enumerate(g.get("relatedTo", [])):
            if "note" in r:
                e.setdefault("relatedNotes", {})[r["name"]] = r.pop("note")
        ev[g["name"]] = e
    store["gear_evidence"] = ev


# ---- option sales, read-as readings, gaps ------------------------------------------------------------------------------------

def _option_sales(data, store):
    sales = data["optionSales"]
    store["option_sales_reasons"] = {
        "text": sales.pop("text", None),
        "sources": {f"{i['unit']} / {i['option']}": i.pop("source") for i in sales["items"] if "source" in i},
        "wargearReadAs": data.pop("wargearReadAs"),
        "profileWargearNotSold": data.pop("profileWargearNotSold"),
    }


def _fill_ins(data, store):
    out = {}
    for kind, key in (("unitFillIns", "unit"), ("mountFillIns", "gear")):
        out[kind] = {i[key]: {k: i.pop(k) for k in ("reason", "source") if k in i} for i in data[kind]["items"]}
    store["fillin_reviews"] = out


def _tts(data, store):
    t = data["ttsModels"]
    out = {}
    for kind, items in t.items():
        if isinstance(items, list):
            out[kind] = [i.pop("reason") for i in items]
    store["tts_provenance"] = out


# ---- the way back ------------------------------------------------------------------------------------------------------------

def attach(shipped, store):
    """The shipped document with the curator data put back (a copy). Raises KeyError when a store file is stale."""
    doc = copy.deepcopy(shipped)
    data = doc["data"]
    data["vanillaAmbiguities"] = copy.deepcopy(store["duplicate_decisions"])
    aud = store["aura_audit"]
    a = data["auras"]
    for e in a["entries"]:
        if e["id"] in aud["entryReasons"]:
            e["reason"] = aud["entryReasons"][e["id"]]
    a["text"] = aud["text"]
    a["excluded"], a["outOfScope"], a["unreachable"] = (copy.deepcopy(aud[k]) for k in ("excluded", "outOfScope", "unreachable"))
    for g in data["gearAnnotations"]["items"]:
        e = store["gear_evidence"][g["name"]]
        g.update({k: v for k, v in e.items() if k != "relatedNotes"})
        for r in g.get("relatedTo", []):
            if r["name"] in e.get("relatedNotes", {}):
                r["note"] = e["relatedNotes"][r["name"]]
    s = store["option_sales_reasons"]
    if s["text"] is not None:
        data["optionSales"]["text"] = s["text"]
    for i in data["optionSales"]["items"]:
        k = f"{i['unit']} / {i['option']}"
        if k in s["sources"]:
            i["source"] = s["sources"][k]
    data["wargearReadAs"], data["profileWargearNotSold"] = copy.deepcopy(s["wargearReadAs"]), copy.deepcopy(s["profileWargearNotSold"])
    for kind, key in (("unitFillIns", "unit"), ("mountFillIns", "gear")):
        for i in data[kind]["items"]:
            i.update(store["fillin_reviews"][kind][i[key]])
    for kind, reasons in store["tts_provenance"].items():
        for i, r in zip(data["ttsModels"][kind], reasons, strict=True):
            i["reason"] = r
    _attach_provenance(doc, store["provenance"])
    return doc


def _attach_provenance(doc, prov):
    for obj, field, addr in _sites(doc):
        if addr in prov:
            obj[field] = prov[addr]


STORE_FILES = ("duplicate_decisions", "aura_audit", "gear_evidence", "option_sales_reasons", "fillin_reviews", "tts_provenance", "provenance")


def load_store(data_dir):
    """The curator data the generator wrote (tools/data); FileNotFoundError when a file is missing."""
    return {name: json.load(open(Path(data_dir) / f"{name}.json", encoding="utf-8")) for name in STORE_FILES}


def load_full(bc_path, data_dir):
    """The audit view of a shipped file: the file with its curator data attached."""
    return attach(json.load(open(bc_path, encoding="utf-8")), load_store(data_dir))
