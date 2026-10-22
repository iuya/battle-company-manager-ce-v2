#!/usr/bin/env python3
"""Builds definitions/glossary/mesbg_glossary.json: the base-rules terms that the vanilla and Battle Companies files
refer to without defining (Heroic Actions, Stand Fast!, Shielding, ...).

The definitions are extracted verbatim from the MESBG rules manual. The manual itself (and the Markdown converted from
it) is copyrighted: keep it under sources/ (gitignored). The resulting json may be committed.

    python3 tools/pdf2md.py ~/Downloads/MESBG_RULES.pdf sources/mesbg_rules.md
    python3 tools/build_glossary.py
"""
import argparse
import json
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from bc.text import norm  # noqa: E402
from glossary_terms import TERMS  # noqa: E402

ROOT = Path(__file__).resolve().parent.parent
GLOSSARY_VERSION = "1.0"  # ours, not the rulebook's: bump it when the content changes
HEAD = re.compile(r"^(#{2,4}) (.*)$")
INTRO_ONLY = {"Heroic Actions", "War Beasts", "Turn Sequence"}  # overviews whose sub-sections are separate terms
PAGE = re.compile(r"<!-- page (\d+) -->")
PAGE_OFFSET = 1  # the 2024 manual: printed page number = PDF page index - 1 (the PDF starts with a cover)


def title_key(t: str) -> str:
    return re.sub(r"\s+", " ", re.sub(r"\s*\(.*?\)\s*", " ", t)).strip().upper()


def load_sections(path):
    lines = open(path, encoding="utf-8").read().split("\n")
    heads, page = [], 0
    for i, l in enumerate(lines):
        m = PAGE.search(l)
        if m:
            page = int(m[1])
        m = HEAD.match(l)
        if m:
            heads.append({"line": i, "level": len(m[1]), "title": m[2].strip(), "key": title_key(m[2]), "page": page})
    for k, h in enumerate(heads):
        end = len(lines)
        for nxt in heads[k + 1:]:
            if nxt["level"] <= h["level"]:
                end = nxt["line"]
                break
        h["end"] = end
    return lines, heads


def paragraphs(lines, start, end):
    """Plain (non-example, non-heading) paragraphs of lines[start:end]."""
    out, dropcap = [], ""
    for l in lines[start:end]:
        l = l.strip()
        if not l or l.startswith(("#", ">", "<!--")) or l.isdigit():  # bare numbers are diagram step labels
            continue
        if re.fullmatch(r"[A-Z]", l):  # a drop cap is a paragraph of its own in the converted text
            dropcap = l
            continue
        if len(l) < 40 and not l.endswith((".", "!", ":", ")", "”")) and l[0] != "•":  # running heads
            continue
        text = re.sub(r"(?<=[.!?])\s+\d$", "", norm(dropcap + l))  # a step label can trail a paragraph
        if out and not dropcap and text[0] != "•" and len(out[-1]) > 25 and not out[-1].endswith((".", "!", "?", ":", "”", ")", '"', "’", "'")):
            out[-1] += " " + text  # a paragraph split by a page or column break
        else:
            out.append(text)
        dropcap = ""
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--src", default=str(ROOT / "sources/mesbg_rules.md"))
    ap.add_argument("--out", default=str(ROOT / "definitions/glossary/mesbg_glossary.json"))
    ap.add_argument("--summaries", default=str(ROOT / "tools/glossary_summaries.json"))
    ap.add_argument("--uncapped", help="write the full verbatim text of every term to this json file (for the curating agent)")
    args = ap.parse_args()
    summaries = json.load(open(args.summaries, encoding="utf-8")) if Path(args.summaries).exists() else {}
    uncapped = {}

    lines, heads = load_sections(args.src)
    page_of_line = {}
    page = 0
    for i, l in enumerate(lines):
        m = PAGE.search(l)
        if m:
            page = int(m[1])
        page_of_line[i] = page

    by_key = {}
    for h in heads:
        by_key.setdefault(h["key"], []).append(h)

    terms, problems = [], []
    for t in TERMS:
        picked = []
        for title in t["headings"]:
            hs = by_key.get(title_key(title), [])
            if not hs:
                problems.append(f"{t['name']}: heading {title!r} not found")
            picked.extend(hs)
        picked.sort(key=lambda h: h["line"])
        # drop sections nested in another picked section so text is not repeated
        picked = [h for h in picked if not any(o is not h and o["line"] < h["line"] < o["end"] for o in picked)]
        parts, sources = [], []
        for h in picked:
            end = h["end"]
            if t["name"] in INTRO_ONLY:  # overview sections whose sub-sections are separate terms
                end = next((n["line"] for n in heads if n["line"] > h["line"]), end)
            ps = paragraphs(lines, h["line"] + 1, end)
            if ps:
                parts.append(ps)
                sources.append({"section": re.sub(r"\s+", " ", h["title"]).title().replace("'S", "'s"), "rulebookPage": h["page"] - PAGE_OFFSET})
        if t["first"]:  # brief entry: keep the first paragraphs of the headings' text (not of each section)
            flat, seen = [], set()
            for ps in parts:
                for p in ps:
                    if p not in seen:
                        seen.add(p)
                        flat.append(p)
            parts = [flat[:t["first"]]]
        for rx, extra in t["paragraphs"]:
            found = False
            for i, l in enumerate(lines):
                if re.search(rx, l.strip()):
                    i0 = i
                    j0 = i - 1
                    while j0 > 0 and not lines[j0].strip():
                        j0 -= 1
                    if re.fullmatch(r"[A-Z]", lines[j0].strip()):
                        i = j0  # include the drop cap
                    end = i + 1
                    got = 0
                    j = i + 1
                    if i != i0:  # drop cap line: the paragraph itself follows
                        while j < len(lines) and not lines[j].strip():
                            j += 1
                        end = j + 1
                        j += 1
                    while got < extra and j < len(lines) and not HEAD.match(lines[j]):
                        if lines[j].strip() and not lines[j].startswith((">", "<!--")):
                            got += 1
                            end = j + 1
                        j += 1
                    parts.append(paragraphs(lines, i, end))
                    sources.append({"section": "(inline)", "rulebookPage": page_of_line[i] - PAGE_OFFSET})
                    found = True
                    break
            if not found:
                problems.append(f"{t['name']}: paragraph {rx!r} not found")
        if not parts:
            problems.append(f"{t['name']}: nothing extracted")
            continue
        seen, flat = set(), []
        for ps in parts:
            for p in ps:
                if p not in seen:  # the manual repeats some blocks (e.g. on spreads)
                    seen.add(p)
                    flat.append(p)
        entry = {"name": t["name"], "category": t["category"]}
        if t["aliases"]:
            entry["aliases"] = t["aliases"]
        entry["definition"] = "\n\n".join(flat)
        uncapped[t["name"]] = entry["definition"]
        if t["synth"]:  # a long chapter: the curating agent writes the brief reminder
            if t["name"] in summaries:
                entry["definition"] = summaries[t["name"]]
                entry["summarized"] = True
            else:
                problems.append(f"{t['name']}: needs a summary in tools/glossary_summaries.json (verbatim text kept for now)")
        entry["sources"] = sources
        terms.append(entry)

    # `see` links point to other terms of this file (a small concept graph); they must resolve
    names = {e["name"] for e in terms}
    by_name = {t["name"]: t for t in TERMS}
    for e in terms:
        see = by_name[e["name"]]["see"]
        for target in see:
            if target not in names:
                problems.append(f"{e['name']}: see -> {target!r} is not a term")
        if see:
            e["see"] = [t for t in see if t in names]

    doc = {
        "name": "MESBG Rules Glossary (2024 manual)",
        "version": GLOSSARY_VERSION,
        "source": "Middle-earth Strategy Battle Game Rules Manual (2024)",
        "description": "Concepts of the MESBG rules manual that the vanilla definitions leave out. Complements the vanilla file; text extracted verbatim from the rules manual (`rulebookPage` is the page number printed in the rulebook). This file refers to nothing else: other files refer to it.",
        "terms": terms,
    }
    for name in set(summaries) - {t["name"] for t in TERMS if t["synth"]}:
        problems.append(f"summary for {name!r} has no synth term")
    if args.uncapped:
        json.dump(uncapped, open(args.uncapped, "w", encoding="utf-8"), ensure_ascii=False, indent=1)
        return
    Path(args.out).parent.mkdir(parents=True, exist_ok=True)
    json.dump(doc, open(args.out, "w", encoding="utf-8"), ensure_ascii=False, indent=2)
    open(args.out, "a").write("\n")
    print(f"{len(terms)} terms, {sum(len(e['definition']) for e in terms) // 1000} KB of text")
    for p in problems:
        print("problem:", p)


if __name__ == "__main__":
    main()
