#!/usr/bin/env python3
"""Validates the definitions files; exit code 1 when there is an error (`make validate`).

    python3 tools/validate_definitions.py [--vanilla ...] [--battle-companies ...] [--glossary ...] [--concept-graph ...]

* JSON Schema (needs `jsonschema`) of the vanilla file, the glossary, the concept graph and the lean Battle Companies file
  (`battle_companies.schema.json`, generated from the Go model; it must also accept the Go valid fixtures).
* The lean Battle Companies file is checked by the Go validator, `go run ./cmd/rulecheck`: the same code the upload runs. It owns
  the cross references (every unit, rule, power, item and path name resolves against the file, vanilla and the glossary), the
  closed vocabularies, the charts and the engine parameter ranges (docs/definitions.md). Repeating that logic in Python would only
  let two validators drift apart, so this script calls it and prints its report (its warnings stay warnings).
* What the Go validator does not check (tools/lean_checks.py): the company texts (repeated rules, parse artefacts), the aura radii
  against the texts that name them, the keywords of the rule targets, the Tabletop Simulator models against the local catalogue.
* The glossary (terms, `see` links, the To Wound Chart) and the concept graph (nodes and hashes against the three files).
"""
import argparse
import json
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
import concept_graph  # noqa: E402
import glossary_reference  # noqa: E402
import lean_checks  # noqa: E402

ROOT = Path(__file__).resolve().parent.parent
LEAN_FIXTURES = ROOT / "internal/core/lean/testdata/valid"


def schema_errors(doc, schema_path):
    try:
        import jsonschema
    except ImportError:
        print(f"  (skipping JSON Schema for {schema_path.name}: pip install jsonschema)")
        return []
    validator = jsonschema.Draft202012Validator(json.load(open(schema_path, encoding="utf-8")))
    return [f"{'/'.join(map(str, e.absolute_path))}: {e.message[:200]}" for e in validator.iter_errors(doc)]


def lean_schema_errors(schema_path):
    """The schema of the lean format is generated from the Go model (go test ./internal/core/lean -run TestSchemaUpToDate -update-schema):
    here it must be a valid Draft 2020-12 schema and accept every valid fixture of the Go decoder."""
    try:
        import jsonschema
    except ImportError:
        return []
    schema = json.load(open(schema_path, encoding="utf-8"))
    try:
        jsonschema.Draft202012Validator.check_schema(schema)
    except jsonschema.SchemaError as e:
        return [f"{schema_path.name}: not a valid schema: {e.message[:200]}"]
    errors = []
    for f in sorted(LEAN_FIXTURES.glob("*.json")):
        errors += [f"{schema_path.name} vs fixture {f.name}: {e}" for e in schema_errors(json.load(open(f, encoding="utf-8")), schema_path)]
    return errors


def rulecheck(bc_path, vanilla_path, glossary_path):
    """Runs the Go validator on the Battle Companies file. Returns (errors, warnings) as printed lines."""
    cmd = ["go", "run", "./cmd/rulecheck", "-vanilla", str(vanilla_path), "-glossary", str(glossary_path), str(bc_path)]
    try:
        r = subprocess.run(cmd, cwd=ROOT, capture_output=True, text=True)
    except FileNotFoundError:
        return ["go is not on the PATH: the Battle Companies file is checked by `go run ./cmd/rulecheck`"], []
    errors, warnings, blocks = [], [], []
    for line in r.stdout.splitlines():
        if line.startswith("  hint:") or line.startswith("  closest:") or line.startswith(" "):
            if blocks:
                blocks[-1] += "\n" + line
            continue
        blocks.append(line)
    for b in blocks[:-1] if r.returncode in (0, 1) and blocks else blocks:
        (warnings if b.startswith("warn: ") else errors).append(b[len("warn: "):] if b.startswith("warn: ") else b)
    if r.returncode not in (0, 1):
        errors.append(f"rulecheck could not run (exit {r.returncode}): {(r.stderr or r.stdout).strip()[:400]}")
    elif r.returncode == 1 and not errors:
        errors.append("rulecheck reports errors but none could be read from its output")
    return errors, warnings


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--vanilla", default=str(ROOT / "definitions/vanilla/data2024.json"))
    ap.add_argument("--battle-companies", default=str(ROOT / "definitions/battle_companies/battle_companies.json"))
    ap.add_argument("--glossary", default=str(ROOT / "definitions/glossary/mesbg_glossary.json"))
    ap.add_argument("--tts-models", default=str(lean_checks.TTS_CATALOGUE), help="local catalogue of the mod's model keys (optional)")
    ap.add_argument("--schema", default=str(ROOT / "definitions/battle_companies/battle_companies.schema.json"), help="the schema of the Battle Companies file")
    ap.add_argument("--concept-graph", default=str(ROOT / "definitions/concepts/concept_graph.json"))
    args = ap.parse_args()
    vp, bp, gp = Path(args.vanilla), Path(args.battle_companies), Path(args.glossary)
    van, bc = json.load(open(vp, encoding="utf-8")), json.load(open(bp, encoding="utf-8"))
    glossary = json.load(open(gp, encoding="utf-8")) if gp.exists() else None
    errors, warnings = [], []

    # schemas
    for doc, path, schema in [(van, vp, vp.with_suffix(".schema.json")), (bc, bp, Path(args.schema))] + ([(glossary, gp, gp.with_suffix(".schema.json"))] if glossary else []):
        if schema.exists():
            errors += [f"{path.name}: {e}" for e in schema_errors(doc, schema)]
        else:
            errors.append(f"{schema} is missing")
    if Path(args.schema).exists():
        errors += lean_schema_errors(Path(args.schema))
    if bc.get("schemaVersion") != 2:
        errors.append(f"{bp.name}: schemaVersion is {bc.get('schemaVersion')!r}, the lean format is 2")
    if bc.get("extends") != van.get("game"):
        errors.append(f"extends={bc.get('extends')!r} does not match vanilla game {van.get('game')!r}")

    # the Go validator: cross references, vocabularies, charts, engine parameters
    e, w = rulecheck(bp, vp, gp)
    errors += e
    warnings += w
    loads = not e  # the nodes of the graph come from the app's loader, which refuses a file with errors

    # what it does not check
    errors += lean_checks.company_text_errors(bc)
    errors += lean_checks.rule_target_errors(bc, van)
    errors += lean_checks.tts_errors(bc, lean_checks.tts_catalogue(args.tts_models))
    raws = [Path(x).read_bytes() for x in (args.vanilla, args.battle_companies, args.glossary)] if glossary and loads else None
    if raws:
        try:
            errors += lean_checks.aura_errors(bc, concept_graph.Index(concept_graph.make_nodes(raws)).texts, van)
        except RuntimeError as ex:
            errors.append(str(ex))

    # the glossary
    known = set()
    if glossary:
        gnames = [t["name"] for t in glossary["terms"]]
        for n in {n for n in gnames if gnames.count(n) > 1}:
            errors.append(f"duplicate glossary term {n!r}")
        known = set(gnames)
        for t in glossary["terms"]:
            if not t["definition"].strip():
                errors.append(f"glossary term {t['name']!r} is empty")
            for target in t.get("see", []):
                if target not in known:
                    errors.append(f"glossary term {t['name']!r}: see -> {target!r} is not a term")
        tw = glossary.get("reference", {}).get("toWound")
        if tw is None:
            errors.append("glossary: reference.toWound is missing")
        else:
            errors += glossary_reference.to_wound_errors(tw)
            if tw.get("term") not in known:
                errors.append(f"glossary reference.toWound: term {tw.get('term')!r} is not a term")

    # the concept graph
    cp = Path(args.concept_graph)
    if glossary and cp.exists() and raws:
        graph = json.load(open(cp, encoding="utf-8"))
        errors += [f"{cp.name}: {x}" for x in schema_errors(graph, cp.with_suffix(".schema.json"))]
        try:
            errors += [f"{cp.name}: {x}" for x in concept_graph.check_graph(graph, raws)]
        except RuntimeError as ex:
            errors.append(str(ex))
    elif glossary and not cp.exists():
        errors.append(f"{cp} is missing: run tools/build_concept_graph.py")

    for x in sorted(set(warnings)):
        print("warning:", x)
    for x in errors:
        print("ERROR:", x)
    print(f"{len(errors)} error(s), {len(set(warnings))} warning(s)")
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
