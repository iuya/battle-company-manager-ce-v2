#!/usr/bin/env python3
"""Builds definitions/concepts/concept_graph.json: the reviewed edges (grants, cancels, lifts, requires) between the concepts
of the three shipped rules files, with the hashes that tell the app whether they still apply (docs/concept-graph.md).

    python3 tools/build_concept_graph.py [--check]

Run after any change to one of the three files: the graph records the hashes of the files it was built from, and
`validate_definitions.py` fails when they changed. Derived from the lean Battle Companies file: item grants, path grants, shield cancellations and
lifts, gear restrictions. The nodes come from the app's loader (`go run ./tools/conceptnodes`). By hand: tools/concepts_curated.json. mentions and see are not stored: the app computes them.
"""
import argparse
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
import concept_graph as cg  # noqa: E402

ROOT = Path(__file__).resolve().parent.parent
PATHS = [ROOT / "definitions/vanilla/data2024.json", ROOT / "definitions/battle_companies/battle_companies.json", ROOT / "definitions/glossary/mesbg_glossary.json"]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default=str(ROOT / "definitions/concepts/concept_graph.json"))
    ap.add_argument("--curated", default=str(ROOT / "tools/concepts_curated.json"))
    ap.add_argument("--check", action="store_true", help="only report whether the file on disk is what the build would write")
    args = ap.parse_args()
    raws = [p.read_bytes() for p in PATHS]
    curated = json.load(open(args.curated, encoding="utf-8"))
    doc, report = cg.build_graph(raws, curated)
    for e in report["errors"]:
        print("ERROR:", e)
    if report["errors"]:
        return 1
    text = json.dumps(doc, indent=2, ensure_ascii=False) + "\n"
    by_type = {}
    for e in doc["edges"]:
        by_type[e["type"]] = by_type.get(e["type"], 0) + 1
    print(f"{len(doc['nodes'])} nodes, {len(doc['edges'])} edges {by_type}, {len(doc['ambiguous'])} ambiguous names")
    for what, why in sorted(set(report["skipped"])):
        print(f"not in the graph: {what}: {why}")
    out = Path(args.out)
    if args.check:
        same = out.exists() and out.read_text(encoding="utf-8") == text
        print("up to date" if same else "OUT OF DATE: run tools/build_concept_graph.py")
        return 0 if same else 1
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(text, encoding="utf-8")
    return 0


if __name__ == "__main__":
    sys.exit(main())
