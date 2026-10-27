"""Tests of data.ttsModels and its validator checks:  python3 -m unittest discover -s tools -p 'test_*.py'"""
import copy
import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
import validate_definitions as vd  # noqa: E402

ROOT = Path(__file__).resolve().parent.parent
VAN = json.load(open(ROOT / "definitions/vanilla/data2024.json", encoding="utf-8"))
BC = json.load(open(ROOT / "definitions/battle_companies/battle_companies.json", encoding="utf-8"))


class TTSModelsTest(unittest.TestCase):
    def setUp(self):
        self.bc = copy.deepcopy(BC)
        self.t = self.bc["data"]["ttsModels"]

    def errors(self, catalogue=None):
        return vd.tts_errors(self.bc, VAN, catalogue)

    def test_shipped_table_is_clean(self):
        self.assertEqual(self.errors(), [])

    def test_content(self):
        t = self.t
        self.assertEqual({s["name"]: s["model"] for s in t["standIns"]}, {
            "Wandering Swordsman": "Captain of Dale", "Wandering Marksman": "Ruffian: Bow", "Wandering Scavenger": "Goblin Mercenary Captain",
            "Wandering Sage": "Malbeth the Seer", "Baggage Pony": "Bill the Pony", "Falcon": "Crebain", "Hunting Dog": "Fang"})
        self.assertEqual({m["gear"]: m["model"] for m in t["mounts"]}, {"Warg": "Fell Warg", "Fell Warg": "Fell Warg"})
        riders = {r["unit"]: r for r in t["riders"]}
        self.assertEqual(len(riders), 18)  # plus Mirkwood Cavalry, the alias of Mirkwood Elf Knight
        self.assertEqual({u for u, r in riders.items() if r["states"]}, {"Rider of Rohan", "Son of Eorl", "Mirkwood Elf Knight", "Hunter Orc Warg Rider"})
        self.assertEqual(riders["Serpent Rider"]["footModel"], "Serpent Guard")

    def test_default_and_marker_options(self):
        self.assertEqual({d["unit"]: d["options"] for d in self.t["defaultOptions"]}, {"Mahûd Warrior": ["Spear"], "Rohirrim Traitor": ["Shield"]})
        self.assertEqual([(m["unit"], m["option"]) for m in self.t["markerOptions"]], [("Galadhrim Warrior", "Lorien Guard")])
        self.t["defaultOptions"][0]["options"] = ["Lance"]
        self.t["markerOptions"][0]["option"] = "Nope"
        e = "\n".join(self.errors())
        self.assertIn("'Lance'", e)
        self.assertIn("'Nope'", e)

    def test_unknown_names_are_errors(self):
        self.t["standIns"][0]["model"] = "Captain of Nowhere"
        self.t["riders"][0]["footModel"] = "Nobody"
        self.t["names"][0]["from"] = "No such option"
        e = "\n".join(self.errors())
        for needle in ("Captain of Nowhere", "Nobody", "No such option"):
            self.assertIn(needle, e)

    def test_missing_mapping_is_an_error(self):
        del self.t["standIns"][-1]
        del self.t["riders"][0]
        del self.t["mounts"][0]
        e = "\n".join(self.errors())
        self.assertIn("Hunting Dog", e)
        self.assertIn("cavalry unit", e)
        self.assertIn("fighting mount", e)

    def test_a_catalogue_is_not_welcome(self):
        self.t["names"] += [{"unit": "Rider of Rohan", "from": "Banner", "to": "Banner", "reason": "x"}] * 101
        self.assertTrue(any("never the mod's catalogue" in e for e in self.errors()))

    def test_local_catalogue(self):
        keys = {("captain of dale", ""), ("ruffian", "bow")}
        e = "\n".join(self.errors(keys))
        self.assertIn("Malbeth the Seer", e)
        self.assertNotIn("Captain of Dale", e)
        self.assertNotIn("Ruffian: Bow", e)

    def test_catalogue_file_is_optional(self):
        with tempfile.TemporaryDirectory() as tmp:
            self.assertIsNone(vd.tts_catalogue(Path(tmp) / "models.json"))
            p = Path(tmp) / "models.json"
            p.write_text(json.dumps(["Ruffian: Bow", "Fang:"]))
            self.assertEqual(vd.tts_catalogue(p), {("ruffian", "bow"), ("fang", "")})


if __name__ == "__main__":
    unittest.main()
