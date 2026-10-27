"""Tests of data.ruleTargets and its validator checks:  python3 -m unittest discover -s tools -p 'test_*.py'"""
import copy
import json
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
import validate_definitions as vd  # noqa: E402

ROOT = Path(__file__).resolve().parent.parent
VAN = json.load(open(ROOT / "definitions/vanilla/data2024.json", encoding="utf-8"))
BC = json.load(open(ROOT / "definitions/battle_companies/battle_companies.json", encoding="utf-8"))


class RuleTargetsTest(unittest.TestCase):
    def setUp(self):
        self.bc = copy.deepcopy(BC)
        self.t = self.bc["data"]["ruleTargets"]

    def errors(self):
        return vd.rule_target_errors(self.bc, VAN)

    def test_shipped_table_is_clean(self):
        self.assertEqual(self.errors(), [])

    def test_patterns(self):
        by = {p["base"]: p for p in self.t["patterns"]}
        self.assertEqual({b: p["appliesTo"] for b, p in by.items()},
                         {"Hatred": "enemy-keyword", "Ancient Enemies": "enemy-keyword", "Terror": "enemy-keyword", "Fury": "friendly-keyword"})
        self.assertIn("Hatred (Goblin)(Orc)(Uruk-Hai)", by["Hatred"]["variants"])
        self.assertIn("Ancient Enemies (Dwarf)(Elf)", by["Ancient Enemies"]["variants"])

    def test_terror_is_enemy_any(self):
        self.assertEqual({e["rule"]: e["appliesTo"] for e in self.t["names"]}["Terror"], "enemy-any")

    def test_unclassified_keyword_parameter_is_an_error(self):
        self.t["patterns"] = [p for p in self.t["patterns"] if p["base"] != "Hatred"]
        self.assertTrue(any("Hatred" in e for e in self.errors()))

    def test_unknown_rule_is_an_error(self):
        self.t["names"].append({"rule": "Nonexistent Rule", "appliesTo": "own"})
        self.assertTrue(any("Nonexistent Rule" in e for e in self.errors()))

    def test_enemy_without_target_is_an_error(self):
        self.t["names"][0].pop("target")
        self.assertTrue(self.errors())


if __name__ == "__main__":
    unittest.main()
