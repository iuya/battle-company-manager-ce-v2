"""Converts the MESBG rules PDF into page-annotated Markdown for local reading and for tools/build_glossary.py.

Usage: python tools/pdf2md.py ~/Downloads/MESBG_RULES.pdf sources/mesbg_rules.md
Headings are detected from the heading typefaces (upper case; BriosoPro / AlbertusMT in the 2018-2022 manuals, P22MackinacPro-Bold in the 2024 one) and written as `##`..`####` by
font size, so sections can be extracted by title. Italic paragraphs (worked examples) are written as `> ` blockquotes. The output is copyrighted material: it must stay under the
gitignored sources/ folder. Requires: pip install pymupdf
"""
import re
import sys

import pymupdf

HEADING_FONTS = ("BriosoPro", "AlbertusMT", "P22MackinacPro-Bold")
NOT_HEADING_FONTS = ("P22MackinacPro-BoldItali",)  # italic body text shares the prefix of the 2024 heading face


def level(size: float) -> str:
    return "##" if size >= 12 else "###" if size >= 8.5 else "####"


def block_text(block):
    """Returns ('heading', level, text) or ('text', text) for one text block."""
    lines, heading_lines = [], []
    for line in block.get("lines", []):
        spans = [s for s in line["spans"] if s["text"].strip()]
        if not spans:
            continue
        text = spans[0]["text"]
        for prev, cur in zip(spans, spans[1:]):  # some heading fonts draw words as spans without a space glyph
            gap = cur["bbox"][0] - prev["bbox"][2]
            text += (" " if gap > 0.8 and not prev["text"].endswith(" ") and not cur["text"].startswith(" ") else "") + cur["text"]
        text = text.strip()
        is_head = all(s["font"].startswith(HEADING_FONTS) and not s["font"].startswith(NOT_HEADING_FONTS) for s in spans) and text.upper() == text and len(text) > 2
        italic = all("Ita" in s["font"] for s in spans)
        lines.append((is_head, text, max(s["size"] for s in spans), italic))
    if not lines:
        return []
    out, cur = [], None
    for is_head, text, size, italic in lines:
        if is_head:
            if cur and cur[0] == "heading" and cur[1] == level(size):
                cur[2] += " " + text
            else:
                cur = ["heading", level(size), text]
                out.append(cur)
        else:
            kind = "example" if italic else "text"
            if cur and cur[0] == kind:
                cur[1] += " " + text
            else:
                cur = [kind, text]
                out.append(cur)
    return out


def main(src: str, dst: str) -> None:
    doc = pymupdf.open(src)
    with open(dst, "w", encoding="utf-8") as out:
        for i, page in enumerate(doc, start=1):
            out.write(f"\n\n<!-- page {i} -->\n\n")
            blocks = [b for b in page.get_text("dict")["blocks"] if b.get("type") == 0]
            # top-to-bottom, left-to-right per column so multi-column pages read in order
            for b in sorted(blocks, key=lambda b: (round(b["bbox"][0] / 250), b["bbox"][1])):
                for item in block_text(b):
                    if item[0] == "heading":
                        out.write(f"{item[1]} {item[2]}\n\n")
                    elif item[1].strip():
                        text = re.sub(r"\s+", " ", item[1]).strip()
                        out.write(("> " if item[0] == "example" else "") + text + "\n\n")


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
