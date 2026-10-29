#!/usr/bin/env python3
"""Checks every palette in tailwind.css against WCAG AA (4.5:1 text, 3:1 accent on page).

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
         ("fg", "hero-soft", 4.5), ("muted", "hero-soft", 4.5), ("gold", "surface", 3.0), ("sergeant", "surface", 3.0), ("hero", "surface", 3.0)]


def lum(h):
    r, g, b = [int(h[i:i + 2], 16) / 255 for i in (1, 3, 5)]
    f = lambda c: c / 12.92 if c <= 0.03928 else ((c + 0.055) / 1.055) ** 2.4
    return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b)


def ratio(a, b):
    la, lb = sorted((lum(a), lum(b)), reverse=True)
    return (la + 0.05) / (lb + 0.05)


def main(path):
    css = Path(path).read_text()
    blocks = re.findall(r'\[data-palette="(\w+)"\]\[data-mode="(\w+)"\]\s*\{(.*?)\}', css, re.S)
    bad = 0
    for pal, mode, body in blocks:
        c = dict(re.findall(r"--([\w-]+):\s*(#[0-9a-fA-F]{6});", body))
        for a, b, need in PAIRS:
            r = ratio(c[a], c[b])
            if r < need:
                print(f"FAIL {pal}/{mode}: {a} on {b} = {r:.2f}:1 (needs {need})")
                bad += 1
    print(f"{len(blocks)} palette/mode combinations, {bad} failure(s)")
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1] if len(sys.argv) > 1 else "tailwind.css"))
