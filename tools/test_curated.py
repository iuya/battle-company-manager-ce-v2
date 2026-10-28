"""Tests of tools/bc/curated.py:  python3 -m unittest discover -s tools -p 'test_*.py'

The shipped Battle Companies file plus tools/data is the audit view; cutting that view again must give back exactly the shipped
file and the same curator data, so nothing is lost between the two and neither goes stale on its own."""
import copy
import json
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from bc import curated  # noqa: E402

ROOT = Path(__file__).resolve().parent.parent
BC = json.load(open(ROOT / "definitions/battle_companies/battle_companies.json", encoding="utf-8"))
STORE = curated.load_store(ROOT / "tools/data")


class CuratedTest(unittest.TestCase):
    def test_cutting_the_audit_view_gives_back_the_file_and_the_data(self):
        view = curated.attach(BC, STORE)
        store, _ = curated.split(view)
        self.assertEqual(view, BC)
        self.assertEqual(store, STORE)

    def test_the_file_carries_none_of_the_curator_fields(self):
        self.assertEqual({"resolution", "validations"} & set(BC), set())
        self.assertEqual({"customCompany", "gold"} & set(BC["campaign"]), set())
        self.assertEqual({"pathNotes", "wargearReadAs", "profileWargearNotSold"} & set(BC["data"]), set())
        text = json.dumps(BC)
        for gone in ('"evidence"', '"outOfScope"', '"unreachable"', '"occurrences"'):
            self.assertNotIn(gone, text)

    def test_the_loader_still_gets_the_reviewed_duplicates(self):
        self.assertEqual({k for a in BC["data"]["vanillaAmbiguities"] for k in a}, {"kind", "name", "status", "occurrence"})
        self.assertEqual(len(BC["data"]["vanillaAmbiguities"]), len(STORE["duplicate_decisions"]))
        self.assertTrue(all(a["note"] for a in STORE["duplicate_decisions"]))

    def test_a_stale_store_is_refused(self):
        stale = copy.deepcopy(STORE)
        stale["gear_evidence"].pop(next(iter(stale["gear_evidence"])))
        with self.assertRaises(KeyError):
            curated.attach(BC, stale)

    def test_recruitment_keeps_what_the_engine_reads(self):
        r = BC["campaign"]["recruitment"]
        self.assertEqual(set(r), {"die", "adjust", "rare"})
        self.assertEqual(r["die"], "d6")
        self.assertEqual({"direction", "costPerPoint", "maxPerAttempt"} - set(r["adjust"]), set())
        self.assertEqual(set(r["rare"]), set(curated.RECRUITMENT_RARE_KEEP))
        self.assertEqual({s["key"] for s in BC["campaign"]["influence"]["spend"]}, {"adjustRoll"})


if __name__ == "__main__":
    unittest.main()
