"""Text helpers shared by the Battle Companies document parsers."""
import re
import unicodedata


def norm(s: str) -> str:
    """Normalises typography the way the vanilla json does (ASCII quotes, inches as "), collapses whitespace."""
    s = s.replace("﻿", "").replace(" ", " ")
    s = re.sub(r"(\d)\s*[”“]", r'\1"', s)
    s = s.replace("’", "'").replace("‘", "'").replace("“", '"').replace("”", '"')
    s = s.replace("…", "...")
    return re.sub(r"[ \t]+", " ", s).strip()


def fold(s: str) -> str:
    """Lowercase, accent-free, punctuation-free key used to match names written slightly differently."""
    s = unicodedata.normalize("NFKD", norm(s))
    s = "".join(c for c in s if not unicodedata.combining(c)).lower()
    return re.sub(r"[^a-z0-9]+", " ", s).strip()


def blocks(lines):
    """Groups consecutive non-empty lines; blank lines separate blocks. Lines are stripped."""
    out, cur = [], []
    for raw in lines:
        l = raw.strip()
        if l:
            cur.append(l)
        elif cur:
            out.append(cur)
            cur = []
    if cur:
        out.append(cur)
    return out


def join(lines) -> str:
    """Joins wrapped lines into one paragraph."""
    return norm(" ".join(lines))


def slug(s: str) -> str:
    return fold(s).replace(" ", "-")


def blocks_tab(lines):
    """Like blocks(), but a line starting with a tab also starts a new block (cell boundary in the source)."""
    out, cur = [], []
    for raw in lines:
        l = raw.strip()
        if not l:
            if cur:
                out.append(cur)
                cur = []
            continue
        if raw.startswith("\t") and cur:
            out.append(cur)
            cur = []
        cur.append(l)
    if cur:
        out.append(cur)
    return out


_SMALL = {"of", "the", "to", "in", "and", "a", "on", "for"}


def smart_title(s: str) -> str:
    words = norm(s).lower().split(" ")
    out = []
    for i, w in enumerate(words):
        if i and w in _SMALL:
            out.append(w)
        else:
            out.append("-".join(p[:1].upper() + p[1:] for p in w.split("-")))
    return " ".join(out).replace("'S", "'s")
