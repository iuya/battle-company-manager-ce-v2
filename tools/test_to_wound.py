"""Tests of the To Wound reference table:  python3 -m unittest discover -s tools -p 'test_*.py'"""
import copy
import json
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
import glossary_reference as gr  # noqa: E402

ROOT = Path(__file__).resolve().parent.parent
FILE = ROOT / "definitions/glossary/mesbg_glossary.json"


class ToWoundTest(unittest.TestCase):
    def setUp(self):
        self.tw = gr.to_wound()

    def test_table_is_valid(self):
        self.assertEqual(gr.to_wound_errors(self.tw), [])

    def test_committed_file_matches_generator(self):
        self.assertEqual(json.load(open(FILE, encoding="utf-8"))["reference"]["toWound"], self.tw)

    def test_known_cells(self):  # corners and the examples of the rulebook (S2 vs D3 = 5+)
        c = self.tw["cells"]
        self.assertEqual((c[0][0], c[0][9], c[1][9], c[9][0], c[9][9], c[1][2], c[4][9]), ("4+", "-", "-", "3+", "4+", "5+", "6+/4+"))

    def test_incomplete_grid(self):
        tw = copy.deepcopy(self.tw)
        tw["cells"][3].pop()
        self.assertTrue(gr.to_wound_errors(tw))

    def test_value_outside_set(self):
        tw = copy.deepcopy(self.tw)
        tw["cells"][5][5] = "2+"
        self.assertTrue(any("allowed" in e for e in gr.to_wound_errors(tw)))

    def test_easier_with_defence(self):
        tw = copy.deepcopy(self.tw)
        tw["cells"][0][4] = "3+"
        self.assertTrue(any("easier" in e for e in gr.to_wound_errors(tw)))

    def test_harder_with_strength(self):
        tw = copy.deepcopy(self.tw)
        tw["cells"][9][9] = "-"
        self.assertTrue(any("harder" in e for e in gr.to_wound_errors(tw)))


if __name__ == "__main__":
    unittest.main()
