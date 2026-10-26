"""Structured reference tables of the MESBG rulebook that the glossary file carries next to its terms
(`reference` in definitions/glossary/mesbg_glossary.json). Numbers hand-transcribed from the rulebook's chart
(sources/mesbg_rules.md, "TO WOUND CHART", printed page 44); the notes are short paraphrases.
"""
import re

# One row per Strength 1..10, one cell per Defence 1..10. "N+" = one dice scoring N or more; "6+/N+" = a 6+ on a dice,
# then a further dice that must score N+ (two individual rolls); "-" = cannot wound.
TO_WOUND_ROWS = [
    "4+  5+  5+  6+  6+  6+/4+ 6+/5+ 6+/6+ -     -",
    "4+  4+  5+  5+  6+  6+    6+/4+ 6+/5+ 6+/6+ -",
    "3+  4+  4+  5+  5+  6+    6+    6+/4+ 6+/5+ 6+/6+",
    "3+  3+  4+  4+  5+  5+    6+    6+    6+/4+ 6+/5+",
    "3+  3+  3+  4+  4+  5+    5+    6+    6+    6+/4+",
    "3+  3+  3+  3+  4+  4+    5+    5+    6+    6+",
    "3+  3+  3+  3+  3+  4+    4+    5+    5+    6+",
    "3+  3+  3+  3+  3+  3+    4+    4+    5+    5+",
    "3+  3+  3+  3+  3+  3+    3+    4+    4+    5+",
    "3+  3+  3+  3+  3+  3+    3+    3+    4+    4+",
]

CELL = re.compile(r"^(?:[2-6]\+|6\+/[2-6]\+|-)$")
ALLOWED = {"3+", "4+", "5+", "6+", "6+/4+", "6+/5+", "6+/6+", "-"}


def to_wound():
    return {
        "title": "To Wound Chart",
        "term": "To Wound Chart",
        "rulebookPage": 44,
        "strengths": list(range(1, 11)),
        "defences": list(range(1, 11)),
        "cells": [r.split() for r in TO_WOUND_ROWS],
        "notation": {
            "N+": "roll one dice; N or more wounds",
            "6+/N+": "roll one dice and score a 6+, then a further dice that must score N+; both rolls are individual (either can be re-rolled by a rule)",
            "-": "cannot wound",
        },
        "notes": [
            "Rows are the attacker's Strength, columns the target's Defence; the cell is the minimum roll to wound.",
            "A Rend uses the nominated model's Strength in place of its Defence.",
            "Strikes against a Trapped model make two To Wound Rolls and apply both.",
        ],
    }


def cell_rank(c):
    """Difficulty order: 3+ < 4+ < 5+ < 6+ < 6+/4+ < 6+/5+ < 6+/6+ < -"""
    if c == "-":
        return 99
    return int(c[0]) * 10 + (int(c[3]) if len(c) > 2 else 0)


def to_wound_errors(tw):
    """Checks of the table: complete 10x10 grid, allowed values, never easier as Defence rises, never harder as Strength rises."""
    errs = []
    if tw.get("strengths") != list(range(1, 11)) or tw.get("defences") != list(range(1, 11)):
        errs.append("toWound: strengths and defences must be 1..10")
    cells = tw.get("cells", [])
    if len(cells) != 10 or any(len(r) != 10 for r in cells):
        return errs + ["toWound: cells must be a 10x10 grid (rows Strength, columns Defence)"]
    bad = sorted({c for r in cells for c in r} - ALLOWED)
    if bad:
        return errs + [f"toWound: values outside the allowed set: {bad}"]
    rank = cell_rank
    for s, row in enumerate(cells, 1):
        for d in range(1, 10):
            if rank(row[d]) < rank(row[d - 1]):
                errs.append(f"toWound: S{s}: D{d + 1} ({row[d]}) is easier than D{d} ({row[d - 1]})")
    for s in range(1, 10):
        for d in range(10):
            if rank(cells[s][d]) > rank(cells[s - 1][d]):
                errs.append(f"toWound: D{d + 1}: S{s + 1} ({cells[s][d]}) is harder than S{s} ({cells[s - 1][d]})")
    return errs
