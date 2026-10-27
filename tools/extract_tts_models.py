#!/usr/bin/env python3
"""Extracts the model keys ('Name: options') of the MESBG FTC mod's model bag from a LOCAL Tabletop Simulator saved game.

    python3 -I tools/extract_tts_models.py [save.json]

Default: the newest ~/Library/Tabletop Simulator/Saves/TS_Save_*.json. Writes only sources/tts/models.json (gitignored: it is the mod's content, so it is
never committed) as {"models": [...]}: the Nickname of every object in the bag with GUID 24f6d0, stripped, duplicates (case and spaces folded) removed, in
the save's order. Nothing else is read out of the save and nothing of the mod's scripts or descriptions is kept. The output path is fixed: the tool
refuses to write if sources/ is not ignored by git. validate_definitions.py and the local-only core test read the file (docs/tabletop-simulator.md).
"""
import argparse
import json
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / "sources/tts/models.json"
BAG_GUID = "24f6d0"
SAVES = Path.home() / "Library/Tabletop Simulator/Saves"


def newest_save(saves=SAVES):
    found = sorted(saves.glob("TS_Save_*.json"), key=lambda p: p.stat().st_mtime)
    if not found:
        sys.exit(f"no TS_Save_*.json in {saves}: pass the path of a saved game")
    return found[-1]


def find_bag(objects, guid=BAG_GUID):
    """The object with the bag's GUID, also when it sits inside another container."""
    for o in objects or []:
        if not isinstance(o, dict):
            continue
        if o.get("GUID") == guid:
            return o
        inner = find_bag(o.get("ContainedObjects"), guid)
        if inner:
            return inner
    return None


def model_keys(save, guid=BAG_GUID):
    """The folded-unique nicknames of the bag's objects, in order. Raises ValueError when the save has no such bag."""
    bag = find_bag(save.get("ObjectStates"), guid)
    if bag is None:
        raise ValueError(f"no object with GUID {guid} in the save: is it a MESBG FTC game?")
    seen, keys = set(), []
    for o in bag.get("ContainedObjects") or []:
        key = " ".join(str(o.get("Nickname") or "").split())
        folded = key.casefold()
        if key and folded not in seen:
            seen.add(folded)
            keys.append(key)
    if not keys:
        raise ValueError(f"the bag {guid} holds no named objects")
    return keys


def check_output(out):
    """Refuses any path that is not sources/tts/ of this repository, or that git does not ignore."""
    if out.resolve() != OUT.resolve() and ROOT / "sources/tts" not in out.resolve().parents:
        sys.exit(f"refusing to write {out}: only sources/tts/ is allowed")
    try:
        ignored = subprocess.run(["git", "-C", str(ROOT), "check-ignore", "-q", str(out)], check=False).returncode == 0
    except OSError:
        ignored = False
    if not ignored:
        sys.exit(f"refusing to write {out}: git does not ignore it (the mod's content must never be committed)")


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("save", nargs="?", help="saved game json (default: the newest TS_Save_*.json)")
    ap.add_argument("--guid", default=BAG_GUID, help="GUID of the model bag (default %(default)s)")
    args = ap.parse_args(argv)
    path = Path(args.save) if args.save else newest_save()
    check_output(OUT)
    with open(path, encoding="utf-8") as f:
        keys = model_keys(json.load(f), args.guid)
    OUT.parent.mkdir(parents=True, exist_ok=True)
    with open(OUT, "w", encoding="utf-8") as f:
        json.dump(keys, f, ensure_ascii=False, indent=0)
    print(f"{len(keys)} model keys from {path.name} -> {OUT.relative_to(ROOT)} (gitignored, never commit it)")


if __name__ == "__main__":
    main()
