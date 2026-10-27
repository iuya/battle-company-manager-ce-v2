"""Tests of tools/extract_tts_models.py with a hand-written fake save (no mod content):  python3 -m unittest discover -s tools -p 'test_*.py'"""
import sys
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).parent))
import extract_tts_models as ex  # noqa: E402

FAKE = {"ObjectStates": [
    {"GUID": "aaaaaa", "Nickname": "Table"},
    {"GUID": "bbbbbb", "ContainedObjects": [{"GUID": "24f6d0", "ContainedObjects": [
        {"Nickname": "Test Warrior: Shield"}, {"Nickname": "test  warrior: shield"}, {"Nickname": "Test Hero:"}, {"Nickname": ""}, {"Nickname": "Test Hero: Horse, Bow"}]}]},
]}


class ExtractTest(unittest.TestCase):
    def test_keys_are_stripped_deduplicated_and_ordered(self):
        self.assertEqual(ex.model_keys(FAKE), ["Test Warrior: Shield", "Test Hero:", "Test Hero: Horse, Bow"])

    def test_bag_is_found_when_nested(self):
        self.assertEqual(ex.find_bag(FAKE["ObjectStates"])["GUID"], "24f6d0")

    def test_missing_bag_is_an_error(self):
        with self.assertRaises(ValueError):
            ex.model_keys({"ObjectStates": [{"GUID": "x"}]})
        with self.assertRaises(ValueError):
            ex.model_keys(FAKE, "ffffff")

    def test_refuses_to_write_outside_sources_tts(self):
        with self.assertRaises(SystemExit):
            ex.check_output(ex.ROOT / "definitions/models.json")
        with self.assertRaises(SystemExit):
            ex.check_output(ex.ROOT / "sources/models.json")

    def test_refuses_when_git_does_not_ignore(self):
        with mock.patch.object(ex.subprocess, "run", return_value=mock.Mock(returncode=1)):
            with self.assertRaises(SystemExit):
                ex.check_output(ex.OUT)

    def test_output_is_ignored_by_git(self):
        ex.check_output(ex.OUT)  # does not exit

    def test_output_format_is_what_the_validator_reads(self):
        import json, tempfile
        import validate_definitions as vd
        with tempfile.TemporaryDirectory() as d:
            p = Path(d) / "models.json"
            p.write_text(json.dumps(ex.model_keys(FAKE)), encoding="utf-8")
            cat = vd.tts_catalogue(p)
        self.assertIn(("test hero", "horse bow"), cat)


if __name__ == "__main__":
    unittest.main()
