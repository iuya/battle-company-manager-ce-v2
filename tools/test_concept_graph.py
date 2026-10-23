"""Tests of the concept graph build and its checks:  python3 -m unittest discover -s tools -p 'test_*.py'"""
import copy
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
import concept_graph as cg  # noqa: E402

ROOT = Path(__file__).resolve().parent.parent
PATHS = [ROOT / "definitions/vanilla/data2024.json", ROOT / "definitions/battle_companies/battle_companies.json", ROOT / "definitions/glossary/mesbg_glossary.json"]
GRAPH = ROOT / "definitions/concepts/concept_graph.json"
RAWS = [p.read_bytes() for p in PATHS]
CURATED = json.load(open(ROOT / "tools/concepts_curated.json", encoding="utf-8"))


def first(doc, typ="grants"):
    return next(e for e in doc["edges"] if e["type"] == typ)


class GraphTest(unittest.TestCase):
    def setUp(self):
        self.doc = json.load(open(GRAPH, encoding="utf-8"))

    def errors(self, doc=None):
        return cg.check_graph(doc or self.doc, RAWS)

    def assertFails(self, doc, text):
        errs = self.errors(doc)
        self.assertTrue(any(text in e for e in errs), f"{text!r} not in {errs[:5]}")

    def test_shipped_graph_is_current(self):
        self.assertEqual(self.errors(), [])

    def test_build_is_deterministic_and_equals_the_file(self):
        a, rep = cg.build_graph(RAWS, CURATED)
        b, _ = cg.build_graph(RAWS, CURATED)
        self.assertEqual(rep["errors"], [])
        self.assertEqual(json.dumps(a), json.dumps(b))
        self.assertEqual(a, self.doc)

    def test_dangling_edge(self):
        d = copy.deepcopy(self.doc)
        d["edges"].append({"from": "rule:nope", "to": first(d)["to"], "type": "grants"})
        self.assertFails(d, "from is not a node")

    def test_stale_built_from(self):
        d = copy.deepcopy(self.doc)
        d["builtFrom"]["battleCompanies"] = "0" * 64
        self.assertFails(d, "builtFrom.battleCompanies is stale")
        self.assertEqual(len(self.errors(d)), 1)

    def test_stale_text_hash(self):
        d = copy.deepcopy(self.doc)
        d["nodes"][0]["textHash"] = "0" * 16
        self.assertFails(d, "textHash is stale")

    def test_vanilla_or_glossary_cannot_point_into_battle_companies(self):
        d = copy.deepcopy(self.doc)
        files = {n["id"]: n["file"] for n in d["nodes"]}
        old = next(n["id"] for n in d["nodes"] if n["file"] in ("vanilla", "glossary"))
        bc = next(n["id"] for n in d["nodes"] if n["file"] == "battleCompanies")
        d["edges"].append({"from": old, "to": bc, "type": "grants"})
        self.assertIn(files[old], ("vanilla", "glossary"))
        self.assertFails(d, "into Battle Companies")

    def test_cancels_lifts_requires_need_a_via(self):
        for typ in ("cancels", "lifts", "requires"):
            d = copy.deepcopy(self.doc)
            del first(d, typ)["via"]
            self.assertFails(d, "needs a via")

    def test_duplicate_edge_and_self_link(self):
        d = copy.deepcopy(self.doc)
        d["edges"].append(dict(first(d)))
        self.assertFails(d, "duplicate")
        d = copy.deepcopy(self.doc)
        d["edges"].append({"from": d["nodes"][0]["id"], "to": d["nodes"][0]["id"], "type": "grants"})
        self.assertFails(d, "self link")

    def test_mentions_and_see_are_not_stored(self):
        for typ in ("mentions", "see"):
            d = copy.deepcopy(self.doc)
            e = dict(first(d))
            e["type"] = typ
            d["edges"] = [e]
            self.assertFails(d, "only grants")

    def test_ambiguous_list_must_match_the_names(self):
        d = copy.deepcopy(self.doc)
        d["ambiguous"] = [{"alias": "fear", "candidates": ["rule:fear", "term:fear"]}]
        self.assertFails(d, "ambiguous[]")

    def test_missing_and_unknown_nodes(self):
        d = copy.deepcopy(self.doc)
        gone = d["nodes"].pop(0)
        self.assertFails(d, f"node {gone['id']} is missing")
        d = copy.deepcopy(self.doc)
        d["nodes"].append({"id": "rule:invented", "kind": "rule", "name": "Invented", "file": "glossary", "textHash": "0" * 16})
        self.assertFails(d, "does not resolve")

    def test_curated_via_must_be_a_quote(self):
        bad = {"edges": [{"from": "Might", "to": "Fate", "type": "requires", "via": "this sentence is in neither text"}]}
        _, rep = cg.build_graph(RAWS, bad)
        self.assertTrue(any("not a quote" in e for e in rep["errors"]), rep["errors"])
        unknown = {"edges": [{"from": "No Such Term", "to": "Fate", "type": "requires", "via": "x"}]}
        _, rep = cg.build_graph(RAWS, unknown)
        self.assertTrue(any("no such term" in e for e in rep["errors"]), rep["errors"])

    def test_curated_edge_repeating_the_data_is_refused(self):
        derived = next(e for e in self.doc["edges"] if e["type"] == "requires" and e["via"].startswith("restriction"))
        names = {n["id"]: n["name"] for n in self.doc["nodes"]}
        dup = {"edges": [{"from": names[derived["from"]], "to": names[derived["to"]], "type": "requires", "via": names[derived["from"]]}]}
        _, rep = cg.build_graph(RAWS, dup)
        self.assertTrue(rep["errors"])

    def test_validator_exits_1_on_a_stale_graph(self):
        d = copy.deepcopy(self.doc)
        d["builtFrom"]["glossary"] = "0" * 64
        with tempfile.TemporaryDirectory() as tmp:
            p = Path(tmp) / "concept_graph.json"
            p.write_text(json.dumps(d))
            (Path(tmp) / "concept_graph.schema.json").write_text(GRAPH.with_suffix(".schema.json").read_text())
            r = subprocess.run([sys.executable, str(ROOT / "tools/validate_definitions.py"), "--concept-graph", str(p)], capture_output=True, text=True)
        self.assertEqual(r.returncode, 1)
        self.assertIn("builtFrom.glossary is stale", r.stdout)


if __name__ == "__main__":
    unittest.main()
