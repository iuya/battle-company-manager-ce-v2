"""Tests of tools/lean_checks.py (company texts, aura rings, rule targets, Tabletop Simulator models) and of the validator:
python3 -m unittest discover -s tools -p 'test_*.py'  (make test). The glossary nodes come from the app's loader (go run ./tools/conceptnodes)."""
import copy
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
import concept_graph as cg  # noqa: E402
import lean_checks as k  # noqa: E402

ROOT = Path(__file__).resolve().parent.parent
PATHS = [ROOT / "definitions/vanilla/data2024.json", ROOT / "definitions/battle_companies/battle_companies.json", ROOT / "definitions/glossary/mesbg_glossary.json"]
RAWS = [p.read_bytes() for p in PATHS]
VAN, BC = json.loads(RAWS[0]), json.loads(RAWS[1])
INDEX = cg.Index(cg.make_nodes(RAWS))


def ring(bc, owner, label=None):
    for part in ("rules", "gear"):
        for x in bc[part]:
            if x["name"] == owner:
                for a in x.get("auras", []):
                    if a.get("label") == label:
                        return a
    raise KeyError(owner)


class CompanyTextTest(unittest.TestCase):
    def setUp(self):
        self.bc = copy.deepcopy(BC)
        self.co = {c["name"]: c for c in self.bc["companies"]}

    def test_shipped_file_is_clean(self):
        self.assertEqual(k.company_text_errors(self.bc), [])

    def test_arnor_epigraph_is_flavour_and_the_rule_is_whole(self):
        a = self.co["Arnor"]
        self.assertEqual(a["epigraph"]["quote"], "Leave all that can be spared behind. We travel light.")
        self.assertEqual(len(a["specialRules"]), 1)
        self.assertTrue(a["specialRules"][0].startswith("Should a Ranger of Arnor Warrior"))
        self.assertTrue(a["specialRules"][0].endswith("Ranger of Arnor profile."))

    def test_legacy_names_are_the_vanilla_units(self):
        text = json.dumps(self.bc["companies"])
        for w in ("High Elf", "Giant Mirkwood", "Grimhammer", "Warrior of Erebor", "Iron Hills Dwarf", "Mirkwood Cavalry", "Abrakhân Guard", "Rhûnish"):
            self.assertNotIn(w, text)

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
        self.assertEqual(f["substitutions"][0]["text"], f["specialRules"][0])
        self.assertEqual(k.company_text_errors(self.bc), [])

    def test_fragments(self):
        for t, why in [('Models gain "Fearless', "unbalanced"), ("and then they fight.", "mid-sentence"), ("This Hero may take the", "mid-sentence"),
                       ("- As Arnor fell, they grew.", "mid-sentence"), ("Fine rule. Battle Companies: 2nd Community Edition", "page header")]:
            self.assertTrue(any(why in r for r in k.artefacts(t)), t)
        self.assertEqual(k.artefacts("A whole rule, with (brackets) and “quotes”."), [])


class AuraTest(unittest.TestCase):
    def setUp(self):
        self.bc = copy.deepcopy(BC)

    def errors(self):
        return k.aura_errors(self.bc, INDEX.texts, VAN)

    def test_shipped_table_is_clean(self):
        self.assertEqual(self.errors(), [])

    def test_vocabulary_is_the_specified_one(self):
        a = self.bc["auras"]
        self.assertEqual({g["id"]: g["default"] for g in a["groups"]}, k.AURA_GROUP_DEFAULTS)
        self.assertEqual([x["id"] for x in a["kinds"]], k.AURA_KINDS)
        self.assertTrue(all(x["default"] for x in a["kinds"]))

    def test_items_of_the_spec(self):
        for owner, label, radius, kind, when in [("Banner", None, 3, "banner", "always"), ("Company Standard", None, 3, "banner", "always"),
                                                 ("War Horn", None, 24, "command", "always"), ("War Horn", 'War Horn: Stand Fast +3"', 9, "command", "declared")]:
            a = ring(self.bc, owner, label)
            self.assertEqual((a["radius"], a["kind"], a["when"]), (radius, kind, when), owner)
        self.assertEqual(ring(self.bc, "War Horn")["group"], "large")

    def test_voice_of_command_replaces_the_six_inch_command_rings(self):
        a = ring(self.bc, "Voice of Command")
        self.assertEqual((a["radius"], a["kind"]), (12, "command"))
        self.assertIn("Stand Fast", a["replaces"])
        self.assertEqual(ring(self.bc, "Stand Fast")["mergeLabels"], ["With Me!", "Loose"])

    def test_wrong_radius_is_an_error(self):
        ring(self.bc, "Furious Tirade")["radius"] = 8
        self.assertTrue(any("Furious Tirade" in e and "not written" in e for e in self.errors()))

    def test_the_sum_of_a_ring_is_written_in_parts(self):
        ring(self.bc, "War Horn", 'War Horn: Stand Fast +3"')["radius"] = 10
        self.assertTrue(any("War Horn" in e for e in self.errors()))

    def test_group_must_follow_when_and_radius(self):
        ring(self.bc, "Gatekeeper")["group"] = "always"
        self.assertTrue(any("Gatekeeper" in e and "group" in e for e in self.errors()))
        ring(self.bc, "Gatekeeper")["group"] = "target"
        ring(self.bc, "War Horn")["group"] = "always"
        self.assertTrue(any("War Horn" in e and "'large'" in e for e in self.errors()))

    def test_changed_default_is_an_error(self):
        next(g for g in self.bc["auras"]["groups"] if g["id"] == "cast")["default"] = True
        self.assertTrue(any("groups must be exactly" in e for e in self.errors()))

    def test_concept_must_resolve(self):
        ring(self.bc, "Banner")["concept"] = "No Such Term"
        self.assertTrue(any("No Such Term" in e for e in self.errors()))

    def test_keen_sight_of_the_falcon_agrees_with_keen_sight(self):
        self.assertEqual(ring(self.bc, "Keen Sight")["radius"], 12)
        self.assertEqual(ring(self.bc, "Keen Sight (Falcon)")["radius"], 12)

    def test_helpers(self):
        self.assertTrue(k.has(3, 'within 3" of'))
        self.assertFalse(k.has(3, 'within 13" of'))
        self.assertFalse(k.has(3, 'within 2.3" of'))
        self.assertTrue(k.has(12, "12” away"))
        self.assertEqual(k.group_of({"when": "always", "radius": 24}), "large")
        self.assertEqual(k.group_of({"when": "cast", "radius": 24}), "cast")
        self.assertEqual(k.group_of({"when": "declared", "radius": 12}), "declared")


class RuleTargetTest(unittest.TestCase):
    def setUp(self):
        self.bc = copy.deepcopy(BC)
        self.rule = {r["name"]: r for r in self.bc["rules"]}

    def test_shipped_table_is_clean(self):
        self.assertEqual(k.rule_target_errors(self.bc, VAN), [])

    def test_the_families_with_a_keyword_parameter(self):
        want = {"Hatred": "enemy-keyword", "Ancient Enemies": "enemy-keyword", "Terror": "enemy-keyword", "Fury": "friendly-keyword"}
        self.assertEqual({n: self.rule[n]["target"]["appliesTo"] for n in want}, want)
        self.assertTrue(all(self.rule[n]["parameter"]["kind"] == "keyword" for n in want))
        self.assertEqual(self.rule["Terror"]["bareTarget"]["appliesTo"], "enemy-any")

    def test_text_only_targets_have_no_keywords(self):
        # Lordly Quality (the bearer's own faction keyword) and Defender (the opposite species keyword) depend on the bearer
        for name in ("Lordly Quality", "Defender"):
            self.assertEqual(self.rule[name]["target"]["appliesTo"], "friendly-keyword")
            self.assertNotIn("keywords", self.rule[name]["target"])
            self.assertTrue(self.rule[name]["target"]["text"])

    def test_unknown_keyword_is_an_error(self):
        self.rule["Gatekeeper"]["target"]["keywords"] = ["Infantri"]
        self.assertTrue(any("Infantri" in e for e in k.rule_target_errors(self.bc, VAN)))


class TTSTest(unittest.TestCase):
    def setUp(self):
        self.bc = copy.deepcopy(BC)

    def test_stand_ins_of_the_spec(self):
        models = {n: m for n, m in ((g["name"], g["tts"]["model"]) for g in self.bc["gear"] if g.get("tts"))}
        self.assertEqual(models, {"Fell Warg": "Fell Warg", "Warg": "Fell Warg", "Baggage Pony": "Bill the Pony", "Falcon": "Crebain", "Hunting Dog": "Fang",
                                  "Wandering Swordsman": "Captain of Dale", "Wandering Marksman": "Ruffian: Bow", "Wandering Scavenger": "Goblin Mercenary Captain",
                                  "Wandering Sage": "Malbeth the Seer"})

    def test_no_catalogue_no_check(self):
        self.assertEqual(k.tts_errors(self.bc, None), [])

    def test_local_catalogue(self):
        keys = {("captain of dale", ""), ("ruffian", "bow")}
        e = "\n".join(k.tts_errors(self.bc, keys))
        self.assertIn("Malbeth the Seer", e)
        self.assertNotIn("Captain of Dale", e)
        self.assertNotIn("Ruffian: Bow", e)

    def test_catalogue_file_is_optional(self):
        with tempfile.TemporaryDirectory() as tmp:
            self.assertIsNone(k.tts_catalogue(Path(tmp) / "models.json"))
            p = Path(tmp) / "models.json"
            p.write_text(json.dumps(["Ruffian: Bow", "Fang:"]))
            self.assertEqual(k.tts_catalogue(p), {("ruffian", "bow"), ("fang", "")})


class ValidatorTest(unittest.TestCase):
    def run_validator(self, *args):
        return subprocess.run([sys.executable, str(ROOT / "tools/validate_definitions.py"), *args], capture_output=True, text=True)

    def test_shipped_files_have_no_errors(self):
        r = self.run_validator()
        self.assertEqual(r.returncode, 0, r.stdout)
        self.assertIn("0 error(s)", r.stdout)

    def test_a_dangling_unit_name_is_an_error(self):
        bc = copy.deepcopy(BC)
        bc["companies"][0]["start"]["units"][0]["unit"] = "Warrior of Nowhere"
        with tempfile.TemporaryDirectory() as tmp:
            p = Path(tmp) / "battle_companies.json"
            p.write_text(json.dumps(bc))
            r = self.run_validator("--battle-companies", str(p))
        self.assertEqual(r.returncode, 1)
        self.assertIn("Warrior of Nowhere", r.stdout)

    def test_an_unknown_key_is_an_error(self):
        bc = copy.deepcopy(BC)
        bc["rules"][0]["colour"] = "red"
        with tempfile.TemporaryDirectory() as tmp:
            p = Path(tmp) / "battle_companies.json"
            p.write_text(json.dumps(bc))
            r = self.run_validator("--battle-companies", str(p))
        self.assertEqual(r.returncode, 1)


if __name__ == "__main__":
    unittest.main()
