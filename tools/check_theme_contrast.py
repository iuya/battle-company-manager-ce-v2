#!/usr/bin/env python3
"""Checks every palette in tailwind.css against WCAG AA (4.5:1 text, 3:1 accent on page), each together with the game
colours of its mode and contrast (the shared leader, sergeant, hero and path tokens).

    python3 tools/check_theme_contrast.py [tailwind.css]
"""
import re
import sys
from pathlib import Path

PAIRS = [("fg", "page", 4.5), ("fg", "surface", 4.5), ("fg", "raised", 4.5), ("muted", "page", 4.5),
         ("muted", "surface", 4.5), ("muted", "raised", 4.5), ("accent-fg", "accent", 4.5), ("accent-text", "page", 4.5),
         ("accent-text", "surface", 4.5), ("accent-text", "accent-soft", 4.5), ("danger", "surface", 4.5),
         ("danger", "danger-soft", 4.5), ("success", "surface", 4.5), ("success", "success-soft", 4.5),
         ("fg", "accent-soft", 4.5), ("fg", "danger-soft", 4.5), ("fg", "success-soft", 4.5), ("accent", "page", 3.0),
         ("page", "danger", 4.5), ("warn", "surface", 4.5), ("warn", "page", 4.5), ("warn", "raised", 4.5), ("page", "warn", 4.5),
         ("fg", "leader-soft", 4.5), ("muted", "leader-soft", 4.5), ("fg", "sergeant-soft", 4.5), ("muted", "sergeant-soft", 4.5),
         ("fg", "hero-soft", 4.5), ("muted", "hero-soft", 4.5), ("gold", "surface", 3.0), ("sergeant", "surface", 3.0), ("hero", "surface", 3.0),
         ("leader", "surface", 4.5), ("leader", "leader-soft", 3.0), ("path-melee", "surface", 4.5), ("path-ranged", "surface", 4.5),
         ("path-magic", "surface", 4.5)]
# The high-contrast variants ([data-contrast="high"]) raise every bar: text to AAA (7:1), the accent, the gold and the
# place colours to text level (4.5:1), and the borders to the non-text minimum (3:1 against surface).
HIGH = {4.5: 7.0, 3.0: 4.5}
HIGH_EXTRA = [("line", "surface", 3.0), ("line", "page", 3.0)]


def lum(h):
    r, g, b = [int(h[i:i + 2], 16) / 255 for i in (1, 3, 5)]
    f = lambda c: c / 12.92 if c <= 0.03928 else ((c + 0.055) / 1.055) ** 2.4
    return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b)


def ratio(a, b):
    la, lb = sorted((lum(a), lum(b)), reverse=True)
    return (la + 0.05) / (lb + 0.05)


def main(path):
    css = Path(path).read_text()
    # a high-contrast block is selected as `[data-contrast="high"] [data-palette=...][data-mode=...]` (among others)
    blocks = re.findall(r'(\[data-contrast="high"\]\s+)?\[data-palette="(\w+)"\]\[data-mode="(\w+)"\]\s*\{(.*?)\}', css, re.S)
    tokens = lambda body: dict(re.findall(r"--([\w-]+):\s*(#[0-9a-fA-F]{6});", body))
    # the game colours are in blocks selected by the mode alone (`[data-mode=...]`, not after another attribute selector)
    game = {}
    for high, mode, body in re.findall(r'(?<![\]\w-])(\[data-contrast="high"\]\s+)?\[data-mode="(\w+)"\]\s*\{(.*?)\}', css, re.S):
        game.setdefault((bool(high), mode), {}).update(tokens(body))
    bad = 0
    for high, pal, mode, body in blocks:
        c = game[(bool(high), mode)] | tokens(body)
        name = f"{pal}/{mode}" + ("/high" if high else "")
        pairs = [(a, b, HIGH[n]) for a, b, n in PAIRS] + HIGH_EXTRA if high else PAIRS
        for a, b, need in pairs:
            r = ratio(c[a], c[b])
            if r < need:
                print(f"FAIL {name}: {a} on {b} = {r:.2f}:1 (needs {need})")
                bad += 1
    print(f"{len(blocks)} palette/mode combinations, {bad} failure(s)")
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1] if len(sys.argv) > 1 else "tailwind.css"))
