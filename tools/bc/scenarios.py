"""Parses the Scenarios chapter."""
import re

from .text import blocks, join, norm, smart_title

SECTION_KEYS = {
    "SCENARIO OUTLINE": "outline",  # a one-line summary some scenarios (Seize the Outpost) carry under the flavour text
    "THE ARMIES": "armies",
    "LAYOUT": "layout",
    "STARTING POSITIONS": "startingPositions",
    "OBJECTIVES": "objectives",
    "SCORING VICTORY POINTS": "scoring",
    "SPECIAL RULES": "specialRules",
}
RULE_SEP = re.compile(r"^(?P<name>[A-Z][^.\n]{2,60}?)\s+[–-]\s+(?P<text>.+)$", re.S)
VP_RE = re.compile(r"(\d+)\s+Victory Points?", re.I)


def tiers(bullet: str):
    """'You score 4 VP if A. If B, you instead score 8 VP.' -> [{text, points}, ...] like vanilla scenarioScoring."""
    sentences = re.split(r"(?<=[.!?])\s+(?=[A-Z])", bullet)
    out = []
    for s in sentences:
        m = VP_RE.search(s)
        if m:
            out.append({"text": s.strip(), "points": int(m[1])})
        elif out:
            out[-1]["text"] += " " + s.strip()
        else:
            out.append({"text": s.strip(), "points": 0})
    return out


# Tables of the scenario special rules that the document lays out in cells (one cell per line): header columns -> kind of lookup.
RULE_TABLES = (
    (("D6", "Result"), "d6"),
    (("Combined Rating", "Monster Budget"), "combinedRating"),
)


def _range(cell):
    nums = [int(n) for n in re.findall(r"\d+", cell)]
    return nums[0], (None if cell.rstrip().endswith("+") else nums[-1])


def split_table(block):
    """Returns (lines before, table or None) for a block of cell lines that holds one of RULE_TABLES."""
    for cols, lookup in RULE_TABLES:
        for k in range(len(block) - len(cols) + 1):
            if tuple(block[k:k + len(cols)]) == cols:
                cells = [norm(c) for c in block[k + len(cols):]]
                assert cells and len(cells) % len(cols) == 0, (cols, cells)
                rows = []
                for i in range(0, len(cells), len(cols)):
                    lo, hi = _range(cells[i])
                    rows.append({"min": lo, "max": hi, "cells": cells[i:i + len(cols)]})
                return block[:k], {"lookup": lookup, "columns": list(cols), "rows": rows}
    return block, None


def parse_scenario(name, lines):
    sc = {"name": smart_title(name)}
    sections, cur, key = {}, [], "description"
    for raw in lines:
        s = raw.strip()
        if s in SECTION_KEYS:
            sections[key] = cur
            key, cur = SECTION_KEYS[s], []
            continue
        if s.startswith("____"):
            continue
        cur.append(raw)
    sections[key] = cur
    sc["description"] = "\n\n".join(join(b) for b in blocks(sections.get("description", [])))
    for k in ("outline", "armies", "layout", "startingPositions", "objectives"):
        if k in sections:
            sc[k] = "\n\n".join(join(b) for b in blocks(sections[k]))
    if "scoring" in sections:
        bullets = []
        for raw in sections["scoring"]:
            if re.match(r"^\s*\*\s+", raw):
                bullets.append(norm(re.sub(r"^\s*\*\s+", "", raw)))
            elif raw.strip() and bullets:
                bullets[-1] += " " + norm(raw)
            elif raw.strip():
                sc.setdefault("scoringNotes", []).append(norm(raw))
        sc["scoring"] = [{"text": b, "tiers": tiers(b)} for b in bullets]
    if "specialRules" in sections:
        rules = []
        for b in blocks(sections["specialRules"]):
            b, table = split_table(b)
            p = join(b)
            if table:
                if p:  # the introduction of a rule written in the same block as its table
                    m = RULE_SEP.match(p) or re.match(r"^(?P<name>[A-Z][^.:\n]{2,60}?):\s+(?P<text>.+)$", p, re.S)
                    rules.append({"name": m["name"].strip(), "definition": m["text"].strip()} if m else {"definition": p})
                rules[-1]["table"] = table
                continue
            m = RULE_SEP.match(p)
            if m:
                rules.append({"name": m["name"].strip(), "definition": m["text"].strip()})
            elif rules:
                rules[-1]["definition"] += "\n\n" + p
            else:
                rules.append({"definition": p})
        # A line that only lists rules defined elsewhere ("Maelstrom of Battle, Heirloom of Power") is a reference list, not a rule.
        used = []
        for r in list(rules):
            if "name" not in r and not re.search(r"[.!?]", r["definition"]):
                used += [x.strip() for x in r["definition"].split(",") if x.strip()]
                rules.remove(r)
        if used:
            sc["usesRules"] = used
        sc["specialRules"] = rules
    return sc
