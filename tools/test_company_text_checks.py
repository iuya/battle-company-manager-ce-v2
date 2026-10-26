"""Tests of the company text checks (repeated rules, parse artefacts):  python3 -m unittest discover -s tools -p 'test_*.py'"""
import copy
import json
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
import company_text_checks as k  # noqa: E402

ROOT = Path(__file__).resolve().parent.parent
BC = json.load(open(ROOT / "definitions/battle_companies/battle_companies.json", encoding="utf-8"))


class CompanyTextChecksTest(unittest.TestCase):
    def setUp(self):
        self.bc = copy.deepcopy(BC)
        self.co = {c["name"]: c for c in self.bc["data"]["companies"]}

    def test_shipped_file_is_clean(self):
        self.assertEqual(k.company_text_errors(self.bc), [])

    def test_arnor_epigraph_is_flavour_and_the_rule_is_whole(self):
        a = self.co["Arnor"]
        self.assertEqual(a["epigraph"]["quote"], "Leave all that can be spared behind. We travel light.")
        self.assertEqual(len(a["specialRules"]), 1)
        self.assertTrue(a["specialRules"][0].startswith("Should a Ranger of Arnor Warrior"))
        self.assertTrue(a["specialRules"][0].endswith("Ranger of Arnor profile."))

    def test_last_alliance_lists_the_rivendell_warrior_with_no_alias(self):
        la = self.co["The Last Alliance"]
        self.assertNotIn("unitAliases", la)
        self.assertNotIn("label", json.dumps(la["startingCompany"]))
        self.assertIn({"count": 1, "unit": "Rivendell Warrior", "wargear": ["Elf bow"]}, la["startingCompany"]["units"])
        self.assertNotIn("High Elf", json.dumps(self.bc["data"]["companies"]))

    def test_giant_mirkwood_spider_is_the_vanilla_unit_with_no_alias(self):
        cu = self.co["Cirith Ungol"]
        self.assertNotIn("Giant Mirkwood", json.dumps(self.bc["data"]["companies"]))
        self.assertNotIn("Mirkwood Giant Spider", [a["label"] for a in cu.get("unitAliases", [])])

    def test_legacy_names_are_the_vanilla_units_with_no_card(self):
        t = json.dumps(self.bc["data"]["companies"])
        for w in ("Grimhammer", "Warrior of Erebor", "Iron Hills Dwarf", "Mirkwood Cavalry", "Abrakhân Guard", "Rhûnish"):
            self.assertNotIn(w, t)

    def test_option_alias_card_states_cost_and_origin(self):
        a = next(x for x in self.co["Helm's Deep"]["unitAliases"] if x["label"] == "Helminga")
        self.assertIn("costs 1 point", a["text"])
        self.assertIn("Grimbold of Grimslade", a["text"])

    def test_glued_epigraph_is_found(self):
        self.co["Arnor"]["specialRules"].insert(0, '"Leave all that can be spared behind. We travel light." - As Arnor fell, the Dunedain grew wilder.')
        self.assertTrue(any("epigraph" in e for e in k.company_text_errors(self.bc)))

    def test_repeated_rule_is_found(self):
        f = self.co["Fiefdoms"]
        f["specialRules"].append(f["specialRules"][0])
        self.assertTrue(any("repeat the same text" in e for e in k.company_text_errors(self.bc)))

    def test_near_duplicate_rule_is_found(self):
        f = self.co["Fiefdoms"]
        f["specialRules"].append("Friendly Dol Amroth models treat the Leader as Prince Imrahil for the purposes of their Pride of Dol Amroth special rule")
        self.assertTrue(any("repeat the same text" in e for e in k.company_text_errors(self.bc)))

    def test_a_typed_entry_may_repeat_its_rule(self):
        f = self.co["Fiefdoms"]
        self.assertEqual(f["ruleSubstitutions"][0]["text"], f["specialRules"][0])
        self.assertEqual(k.company_text_errors(self.bc), [])

    def test_fragments(self):
        for t, why in [('Models gain "Fearless', "unbalanced"), ("and then they fight.", "mid-sentence"), ("This Hero may take the", "mid-sentence"),
                       ("- As Arnor fell, they grew.", "mid-sentence"), ("Fine rule. Battle Companies: 2nd Community Edition", "page header")]:
            self.assertTrue(any(why in r for r in k.artefacts(t)), t)
        self.assertEqual(k.artefacts("A whole rule, with (brackets) and “quotes”."), [])


if __name__ == "__main__":
    unittest.main()
