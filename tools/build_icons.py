#!/usr/bin/env python3
"""Builds the favicon and app icons from the One Ring inscription image (internal/web/static/img/ring_text.png).

Needs ImageMagick (`magick`) and potrace:  brew install imagemagick potrace
The inscription is traced to vector paths and set in gold on a transparent background (it must read on light and dark pages and tabs). Also writes img/ring-text.svg, the page background. Two icon sizes are produced: a
bold, low-detail one for tabs (favicon.svg) and a finer one for large uses (icon.svg, app icons).

    python3 tools/build_icons.py
"""
import re
import subprocess
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
STATIC = ROOT / "internal/web/static"
SRC = STATIC / "img/ring_text.png"
GOLD = "#c9a032"  # dark enough for light backgrounds, bright enough for dark ones


def trace(scale: int, turd: int) -> tuple[str, float]:
    """Returns (path data, image size in potrace units) for the inscription."""
    with tempfile.TemporaryDirectory() as tmp:
        pbm, svg = Path(tmp, "r.pbm"), Path(tmp, "r.svg")
        subprocess.run(["magick", str(SRC), "-background", "white", "-alpha", "remove", "-colorspace", "gray",
                        "-filter", "Lanczos", "-resize", f"{scale}%", "-threshold", "55%", str(pbm)], check=True)
        subprocess.run(["potrace", "-s", "-t", str(turd), "-O", "2", "-a", "1.2", str(pbm), "-o", str(svg)], check=True)
        text = svg.read_text()
    size = float(re.search(r'viewBox="0 0 ([\d.]+) ', text)[1])
    d = " ".join(" ".join(p.split()) for p in re.findall(r' d="([^"]*)"', text))
    return d, size


def icon(d: str, size: float, stroke: float, pad: float) -> str:
    s = (64 - 2 * pad) / size
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" role="img" aria-label="Battle Companies 2nd CE Manager">\n'
            f'<g transform="translate({pad} {pad}) scale({s:.5f}) translate(0 {size}) scale(.1 -.1)" fill="{GOLD}" '
            f'stroke="{GOLD}" stroke-width="{stroke}" stroke-linejoin="round"><path d="{d}"/></g>\n</svg>\n')


def ring_mask(d: str, size: float) -> str:
    """The bare inscription, used as a CSS mask: the page decides the color (see body::before in tailwind.css)."""
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {size:g} {size:g}">'
            f'<g transform="translate(0 {size:g}) scale(.1 -.1)"><path d="{d}"/></g></svg>\n')


def png(svg: Path, out: Path, px: int, bg: str = "none") -> None:
    """bg is opaque for the iOS home screen icon, which would otherwise get a black background."""
    cmd = ["magick", "-background", bg, "-density", "1536", str(svg), "-resize", f"{px}x{px}"]
    cmd += ["-flatten"] if bg != "none" else []
    subprocess.run(cmd + ["-colors", "32", "-define", "png:compression-level=9", str(out)], check=True)


def main() -> None:
    d, size = trace(100, 6)
    (STATIC / "favicon.svg").write_text(icon(d, size, stroke=48, pad=3))  # bold: it is read at 16-32 px
    d, size = trace(200, 8)
    (STATIC / "icon.svg").write_text(icon(d, size, stroke=14, pad=4))
    (STATIC / "img/ring-text.svg").write_text(ring_mask(d, size))
    png(STATIC / "icon.svg", STATIC / "icon-512.png", 512)
    png(STATIC / "icon.svg", STATIC / "apple-touch-icon.png", 180, bg="#101a14")
    png(STATIC / "favicon.svg", STATIC / "favicon-32.png", 32)
    for f in ("img/ring-text.svg", "favicon.svg", "icon.svg", "favicon-32.png", "apple-touch-icon.png", "icon-512.png"):
        print(f"{f}: {(STATIC / f).stat().st_size} bytes")


if __name__ == "__main__":
    main()
