"""Checks of the company texts of the Battle Companies file: repeated company rules and parse artefacts.

Used by validate_definitions.py; tests in test_company_text_checks.py.

* A company's own lists (`specialRules`, `notes`) must not say the same thing twice (same sentence, or one inside the other).
  A rule that a typed entry (`ruleSubstitutions`, `ruleExchanges`, `grantedSpecialRules`, `limitExemptions`) repeats is fine:
  the typed entry is the machine-readable form and the app prints the sentence once (`CompanyProfile.OwnRules`).
* Texts copied from the rulebook must be whole: balanced quotation marks and brackets, no quote-and-dash epigraph glued to a
  rule ("Leave all that ..." - As Arnor fell ...), no fragment at either end, no page header or footer.
"""
import re

QUOTES = '"“”'
FRAGMENT_END = re.compile(r"(?:[,;:–—-]|\b(?:and|or|the|a|an|of|to|in|on|for|with|than|that|which|as|if|but|while|rather))$", re.I)
EPIGRAPH = re.compile(r"^\s*[" + QUOTES + r"].{3,200}?[" + QUOTES + r"]\s*[–—-]")
PAGE_FURNITURE = re.compile(r"Battle Companies:?\s*2nd|Community Edition\s*-|^\s*Page\s+\d+\s*$|^\s*\d+\s*/\s*\d+\s*$", re.I)


def norm(t):
    return re.sub(r"\W+", " ", t.lower()).strip()


def balanced(t):
    """Quotation marks and brackets pair up (typographic quotes pair in order, straight quotes in twos)."""
    if t.count("“") != t.count("”") or t.count("(") != t.count(")") or t.count("[") != t.count("]"):
        return False
    return t.count('"') % 2 == 0


def artefacts(t):
    """What looks wrong with a rule or note text, as a list of short reasons."""
    out = []
    s = t.strip()
    if not s:
        return ["empty"]
    if not balanced(s):
        out.append("unbalanced quotation marks or brackets")
    if EPIGRAPH.match(s):
        out.append("starts with a quoted epigraph and a dash")
    if re.match(r"^[–—-]\s", s) or re.match(r"^[a-z]", s) or re.match(r"^[,;:.)]", s):
        out.append("starts mid-sentence")
    if FRAGMENT_END.search(s):
        out.append("ends mid-sentence")
    if PAGE_FURNITURE.search(s):
        out.append("contains a page header or footer")
    return out


def texts(c):
    """(where, text) of every sentence of a company that is copied from the rulebook as running text."""
    for i, r in enumerate(c.get("specialRules", [])):
        yield f"specialRules[{i}]", r
    for i, n in enumerate(c.get("notes", [])):
        yield f"notes[{i}]", n["text"]
    if c.get("oath", {}).get("text"):
        yield "oath.text", c["oath"]["text"]
    for i, p in enumerate(c.get("description", "").split("\n\n")):
        if p.strip():
            yield f"description[{i}]", p
    ep = c.get("epigraph")
    if ep:
        yield "epigraph.quote", ep["quote"]
        yield "epigraph.text", ep["text"]
    for i, h in enumerate(c.get("heroUpgrades", [])):
        for k in ("flavour", "text"):
            if h.get(k):
                yield f"heroUpgrades[{i}].{k}", h[k]


def company_text_errors(bc):
    errors = []
    for c in bc["data"]["companies"]:
        name = c["name"]
        for where, t in texts(c):
            if where == "epigraph.quote":
                continue
            for why in artefacts(t):
                errors.append(f"company {name!r} {where}: {why}: {t[:80]!r}")
        for kind in ("specialRules", "notes"):
            items = [(f"{kind}[{i}]", e["text"] if isinstance(e, dict) else e) for i, e in enumerate(c.get(kind, []))]
            if kind == "specialRules":
                items += [(f"notes[{i}]", n["text"]) for i, n in enumerate(c.get("notes", []))]
            for a in range(len(items)):
                for b in range(a + 1, len(items)):
                    ka, kb = norm(items[a][1]), norm(items[b][1])
                    if kb and ka and (ka == kb or ka in kb or kb in ka):
                        errors.append(f"company {name!r}: {items[a][0]} and {items[b][0]} repeat the same text: {items[a][1][:80]!r}")
        if c.get("epigraph") and any(norm(c["epigraph"]["text"]) in norm(r) for r in c.get("specialRules", [])):
            errors.append(f"company {name!r}: the epigraph is repeated in specialRules")
    return errors
