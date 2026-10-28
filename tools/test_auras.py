"""Tests of data.auras and its validator checks:  python3 -m unittest discover -s tools -p 'test_*.py'"""
import copy
import json
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
import validate_definitions as vd  # noqa: E402
from bc import auras as table  # noqa: E402
from bc import curated  # noqa: E402

ROOT = Path(__file__).resolve().parent.parent
VAN = json.load(open(ROOT / "definitions/vanilla/data2024.json", encoding="utf-8"))
BC = curated.load_full(ROOT / "definitions/battle_companies/battle_companies.json", ROOT / "tools/data")  # the audit view: the file plus the curator data
GL = json.load(open(ROOT / "definitions/glossary/mesbg_glossary.json", encoding="utf-8"))


class AurasTest(unittest.TestCase):
    def setUp(self):
        self.bc = copy.deepcopy(BC)
        self.a = self.bc["data"]["auras"]

    def errors(self):
        return vd.aura_errors(self.bc, VAN, GL)

    def entry(self, id):
        return next(e for e in self.a["entries"] if e["id"] == id)

    def test_shipped_table_is_clean(self):
        self.assertEqual(self.errors(), [])

    def test_group_defaults_are_the_specs(self):
        self.assertEqual({g["id"]: g["default"] for g in self.a["groups"]},
                         {"always": True, "declared": True, "conditional": True, "cast": False, "target": False, "large": False, "optional": False})
        self.assertTrue(all(k["default"] for k in self.a["kinds"]))
        self.assertEqual([k["id"] for k in self.a["kinds"]], ["banner", "command", "support", "enemy", "utility"])

    def test_items_of_the_spec(self):
        for id, radius, kind, group in [("banner-3", 3, "banner", "always"), ("standard-3", 3, "banner", "always"), ("instrument-3", 3, "support", "always"),
                                        ("drum-12", 12, "command", "declared"), ("horn-24", 24, "command", "large"),
                                        ("horn-standfast-9", 9, "command", "declared"), ("torch-6", 6, "utility", "optional")]:
            e = self.entry(id)
            self.assertEqual((e["radius"], e["kind"], e["group"], e["source"]["type"]), (radius, kind, group, "item"), id)

    def test_voice_of_command_replaces_the_six_inch_command_rings(self):
        e = self.entry("voice-12")
        self.assertEqual((e["radius"], e["kind"]), (12, "command"))
        for r in e["replaces"]:
            self.assertEqual((self.entry(r)["radius"], self.entry(r)["kind"]), (6, "command"))
        self.assertIn("standfast-6", e["replaces"])

    def test_default_off_groups(self):
        off = {e["id"] for e in self.a["entries"] if e["group"] in ("cast", "target", "large", "optional")}
        self.assertIn("aoc-6", off)
        self.assertIn("gatekeeper-6", off)
        self.assertIn("horn-24", off)
        self.assertNotIn("standfast-6", off)
        self.assertEqual(self.entry("standfast-6")["mergeLabels"], ["With Me!", "Loose"])

    def test_every_hero_has_exactly_one_hero_entry(self):
        self.assertEqual([e["id"] for e in self.a["entries"] if e["source"]["type"] == "hero"], ["standfast-6"])

    def test_wrong_radius_is_an_error(self):
        self.entry("tirade-6")["radius"] = 8
        self.assertTrue(any("tirade-6" in e and "not written" in e for e in self.errors()))

    def test_radius_of_an_item_must_be_in_its_text(self):
        self.entry("banner-3")["radius"] = 4
        self.assertTrue(any("banner-3" in e for e in self.errors()))

    def test_radius_parts_must_add_up(self):
        self.entry("horn-standfast-9")["radiusParts"] = [6, 4]
        self.assertTrue(any("add up" in e for e in self.errors()))

    def test_unknown_source_is_an_error(self):
        self.entry("tirade-6")["source"]["names"] = ["Furious Tirades"]
        self.assertTrue(any("does not exist" in e for e in self.errors()))

    def test_group_must_follow_when_and_radius(self):
        self.entry("aoc-6")["group"] = "always"
        self.assertTrue(any("aoc-6" in e and "group" in e for e in self.errors()))

    def test_changed_default_is_an_error(self):
        next(g for g in self.a["groups"] if g["id"] == "cast")["default"] = True
        self.assertTrue(any("groups must be exactly" in e for e in self.errors()))

    def test_unclassified_range_rule_is_an_error(self):
        self.a["entries"] = [e for e in self.a["entries"] if e["id"] != "tirade-6"]
        self.assertTrue(any("Furious Tirade" in e and "neither an entry nor excluded" in e for e in self.errors()))

    def test_unclassified_banner_item_is_an_error(self):
        self.a["entries"] = [e for e in self.a["entries"] if e["id"] != "standard-3"]
        self.assertTrue(any("Company Standard" in e for e in self.errors()))

    def test_weapon_range_exclusion_is_complete(self):
        x = next(x for x in self.a["excluded"] if x["category"] == "weapon-range" and x["type"] == "item")
        x["names"].remove("Longbow")
        self.assertTrue(any("Longbow" in e for e in self.errors()))

    def test_replaces_must_point_at_an_entry(self):
        self.entry("voice-12")["replaces"] = ["nothing-9"]
        self.assertTrue(any("nothing-9" in e for e in self.errors()))

    def test_unreachable_is_current_and_disjoint(self):
        self.assertEqual(self.a["unreachable"], table.unreachable(self.bc, VAN))
        self.assertIn("Wormtongue", self.a["unreachable"])
        self.a["unreachable"].append("Terror")
        self.assertTrue(any("unreachable" in e for e in self.errors()))

    def test_concept_must_resolve(self):
        self.entry("banner-3")["concept"] = "No Such Term"
        self.assertTrue(any("No Such Term" in e for e in self.errors()))

    def test_falcon_creature_agrees_with_keen_sight(self):
        self.assertEqual(self.entry("keensight-12")["radius"], 12)
        self.entry("keensight-12")["radius"] = 10
        self.assertTrue(self.errors())


class HelpersTest(unittest.TestCase):
    def test_has(self):
        self.assertTrue(table.has(3, 'within 3" of'))
        self.assertFalse(table.has(3, 'within 13" of'))
        self.assertFalse(table.has(3, 'within 2.3" of'))
        self.assertTrue(table.has(12, "12” away"))

    def test_group_of(self):
        self.assertEqual(table.group_of({"id": "x", "when": "always", "radius": 24}), "large")
        self.assertEqual(table.group_of({"id": "x", "when": "cast", "radius": 24}), "cast")
        self.assertEqual(table.group_of({"id": "torch-6", "when": "always", "radius": 6}), "optional")
        self.assertEqual(table.group_of({"id": "x", "when": "declared", "radius": 12}), "declared")


if __name__ == "__main__":
    unittest.main()
