"""Index of the vanilla json, used to resolve names written in the Battle Companies document to vanilla keys."""
import json
import re

from .text import fold

# Names in the BC document that differ from the vanilla key for reasons other than case/accents/plurals.
ALIASES = {
    "hobbit shiriff": "Hobbit Shirriff",
    "black guard of barad dur": "Black Guard of Barad-dûr",
    "hunter orc on warg rider": "Hunter Orc Warg Rider",
    "black numenorean": "Black Numenorean Warrior",
    "warg rider": "Mordor Warg Rider",
    "wildman of dunland": "Wild Man of Dunland",
    "wildmen of dunland": "Wild Man of Dunland",
}


# Names the Battle Companies document prints for a vanilla unit under another name (2018 names, abbreviations); same stats and
# points. The printed name is kept as `label` and the company shows an explanation card. Each mapping is a reviewed guess
# (see docs/definitions.md, unit names).
LEGACY_NAMES = {}  # the former entries moved to SOURCE_TYPOS (tools/bc/core.py): the unit is simply named correctly

# Units that are a vanilla unit plus a rule/keyword that would be illegal in vanilla but is allowed in Battle Companies.
# No new unit is created: the reference points at the vanilla unit and carries the delta.
UNIT_DELTAS = {
    "ranger of ithilien": {"unit": "Ranger of Gondor", "addSpecialRules": ["Loyal to the Captain"]},
}

# Printed name -> (unit, vanilla option) for options whose vanilla spelling differs from the printed one.
OPTION_ALIASES = {
    "noldorin exile": ("Lothlorien Warrior", "Noldorian Exile"),
}


class Vanilla:
    def __init__(self, path):
        d = json.load(open(path, encoding="utf-8"))
        self.game = d["game"]
        data = d["data"]
        self._data = data
        self.collections = {k: {e["name"] for e in v} for k, v in data.items()}
        self.units = self.collections["warriors"] | self.collections["heroes"]
        self._unit_by_fold = {}
        for n in self.units:
            self._unit_by_fold.setdefault(fold(n), n)
        self._kw_by_fold = {}
        for n in self.collections["keywords"]:
            self._kw_by_fold.setdefault(fold(n), n)
        self._by_fold = {k: {fold(n): n for n in v} for k, v in self.collections.items()}
        self._options = {}  # folded option name -> [(unit, option)]
        for kind in ("warriors", "heroes"):
            for e in data[kind]:
                for o in e.get("options", []):
                    self._options.setdefault(fold(o["name"]), []).append((e["name"], o["name"]))

    def unit(self, name):
        """Returns the vanilla key of a warrior/hero, or None."""
        f = fold(name)
        singular = re.sub(r"ies$", "y", f) if f.endswith("ies") else f.rstrip("s")
        inner = re.sub(r"s( of\b)", r"\1", f)
        for cand in (f, singular, inner, f + " warrior", singular + " warrior", re.sub(r"\bwarriors\b", "warrior", f)):
            if cand in self._unit_by_fold:
                return self._unit_by_fold[cand]
            if cand in ALIASES:
                a = ALIASES[cand]
                if a in self.units:
                    return a
        return None

    def option(self, unit, name):
        """The vanilla option dict (name, points, availableIn) of a unit, or None."""
        for kind in ("warriors", "heroes"):
            for e in self._data[kind]:
                if e["name"] == unit:
                    return next((o for o in e.get("options", []) if o["name"] == name), None)
        return None

    def keyword(self, name):
        f = fold(name)
        if f in self._kw_by_fold:
            return self._kw_by_fold[f]
        compact = f.replace(" ", "")
        hit = next((v for k, v in self._kw_by_fold.items() if k.replace(" ", "") == compact), None)
        return hit or self._kw_by_fold.get(f.rstrip("s"))

    def find(self, collection, name):
        return self._by_fold[collection].get(fold(name))

    POWER_ALIASES = {"instil fear": "Instill Fear"}

    def power(self, name):
        """Resolves a magical power name; 'Fury (X)' -> ('Fury', True) because vanilla defines Fury per species."""
        base = re.sub(r"\s*\(X\)$", "", name.strip())
        f = fold(base)
        if f in self.POWER_ALIASES:
            return self.POWER_ALIASES[f]
        hit = self._by_fold["magicalPowers"].get(f)
        if hit:
            return hit
        compact = f.replace(" ", "")
        return next((v for k, v in self._by_fold["magicalPowers"].items() if k.replace(" ", "") == compact), None)

    def resolve(self, printed):
        """Resolves a name written in the Battle Companies document.

        Returns {"unit": vanilla key, "via": "name" | "legacyName" | "option", "option": vanilla option or None}
        or None. "option" means the printed name is an upgrade/option of a base unit (e.g. Tookish Hunter is a Hobbit
        Archer with the Tookish Hunter option selected).
        """
        f = fold(printed)
        if f in UNIT_DELTAS:
            return {"unit": UNIT_DELTAS[f]["unit"], "via": "delta", "option": None, "delta": UNIT_DELTAS[f]}
        if f in LEGACY_NAMES:
            return {"unit": LEGACY_NAMES[f], "via": "legacyName", "option": None}
        direct = self.unit(printed)
        if direct:
            return {"unit": direct, "via": "name", "option": None}
        if f in OPTION_ALIASES:
            unit, opt = OPTION_ALIASES[f]
            return {"unit": unit, "via": "option", "option": opt}
        for cand in (f, f + "s", f.rstrip("s")):
            hits = self._options.get(cand)
            if hits:
                hits = sorted(set(hits))
                warriors = [h for h in hits if h[0] in self.collections["warriors"]]
                unit, opt = (warriors or hits)[0]
                return {"unit": unit, "via": "option", "option": opt}
        return None


RESERVED_OPTIONS_TEXT = (
    "Some vanilla options belong to the army list of one named hero (availableIn lists only hero names: King's Guard of "
    "Gil-galad's list, Lorien Guard of Galadriel's and Celeborn's...). A Battle Company has no such army list and its "
    "document does not lift the restriction, so a 'Choice of Options' never offers them. A company can still name one "
    "explicitly: its unitAliases (Tookish Hunter, Battlin' Brandybuck, Helmingas, Noldorian Exile, Lorien Guard) select the "
    "option directly. Options whose availableIn names armies or scenarios are not listed here."
)


def reserved_options(van_doc):
    """data.reservedOptions: the options of vanilla units whose availableIn names only heroes (generated, not hand-written)."""
    data = van_doc["data"]
    heroes = {h["name"] for h in data["heroes"]}
    items, seen = [], set()
    for e in data["heroes"] + data["warriors"]:
        for o in e.get("options", []):
            av = o.get("availableIn")
            if not av or not all(isinstance(a, str) and a in heroes for a in av):
                continue
            key = (e["name"], o["name"])
            if key in seen:
                continue
            seen.add(key)
            items.append({"unit": e["name"], "option": o["name"], "availableIn": list(av)})
    return {"text": RESERVED_OPTIONS_TEXT, "items": sorted(items, key=lambda i: (i["unit"], i["option"]))}


OPTION_SALES_TEXT = (
    "Vanilla profile options that the Armoury of a Battle Company does not sell, so 'option has no matching armoury item' is "
    "expected and not a data defect. status `notSold`: no armoury item exists for it (the option is a banner, war horn or war "
    "drum, which the Battle Companies equipment table replaces with the Company Standard, Company Horn and Company Drum and "
    "which a 'Choice of Options' excludes; or it is Unique Wargear a company lists on its own page). status `readAs`: the option is read as an item the Armoury does sell (`armouryItem`), so it stays choosable and buyable as that item. status `ownItem`: the option is a weapon of its own that the Armoury does not sell but data.gearAnnotations describes (`item`, a Cave Troll's Troll Spear): choosable under its own name, no Armoury price. status `granted`: the model "
    "comes with it from the charts or the starting company and no Armoury entry sells it. A company whose armoury does sell "
    "the item (its Unique Wargear) is unaffected: this list only silences the check where no item matches."
)

# Hand-decided entries (unit, option) -> (status, reason, source). The banner / war horn / war drum options are generated.
OPTION_SALES_DECISIONS = {
    ("Watcher of K\u00e2rna", "Twin Blades"): ("granted", "Harad's starting company and charts give Watchers of K\u00e2rna 'with Twin Blades'; the Armoury sells no Twin Blades", "BC document, Harad"),
    ("Ruffian", "Whip"): ("granted", "Sharkey's Rogues start with 'Ruffian with whip'; the Armoury sells no whip, only the company page exempts it from the Throwing Weapon limit", "BC document, Sharkey's Rogues"),
    ("Cave Troll", "Troll Spear"): ("ownItem", "a Cave Troll is hired 'with Choice of Option' (15 Influence); the Troll Spear is a weapon of its own in the vanilla data (a spear with an additional rule), choosable under its own name (user decision 2026-10-22, R6)", "BC document, Moria; vanilla gear", "Troll Spear"),
    ("Cave Troll", "Hand-and-a-half hammer"): ("readAs", "a Cave Troll is hired 'with Choice of Option' (15 Influence); the hammer is only a Hand-and-a-half Weapon (the hammer is flavour), read as the Armoury item so it stays choosable (user decision 2026-10-22, R6)", "BC document, Moria", "Hand-and-a-half Weapon"),
    ("Ranger of the North", "Armour"): ("granted", "'1 Ranger of the North with armour': the Armoury wargear table has no armour row, it only comes from the starting company", "BC document, Helm's Deep"),
    ("Lothlorien Warrior", "Wood Elf spear"): ("notSold", "Unique Wargear of the one company that lists it (Wood Elf Spear, 1 Influence); no other armoury sells it", "BC document, Lothlorien (Unique Wargear)"),
}
_BANNER_WORDS = re.compile(r"\b(banner|war ?horn|war drum)\b", re.I)
OPTION_SALES_BANNER_REASON = ("a banner, war horn or war drum: the Battle Companies equipment sells the Company Standard, Company Horn and "
                              "Company Drum instead, and a 'Choice of Options' excludes these options")


def is_banner_option(name):
    return bool(_BANNER_WORDS.search(name))


def option_sales(van_doc):
    """data.optionSales: vanilla options the Armoury does not sell (generated for banners/horns/drums, hand-decided for the rest)."""
    data = van_doc["data"]
    items, seen = [], set()
    for e in data["heroes"] + data["warriors"]:
        for o in e.get("options", []):
            key = (e["name"], o["name"])
            if key in seen:
                continue
            if is_banner_option(o["name"]):
                seen.add(key)
                items.append({"unit": key[0], "option": key[1], "status": "notSold", "reason": OPTION_SALES_BANNER_REASON,
                              "source": "BC document, Equipment and Choice of Options"})
    for (u, o), dec in OPTION_SALES_DECISIONS.items():
        status, reason, source = dec[:3]
        assert any(e["name"] == u and any(x["name"] == o for x in e.get("options", [])) for e in data["heroes"] + data["warriors"]), (u, o)
        items.append({"unit": u, "option": o, "status": status, "reason": reason, "source": source, **({("item" if status == "ownItem" else "armouryItem"): dec[3]} if len(dec) > 3 else {})})
    return {"text": OPTION_SALES_TEXT, "items": sorted(items, key=lambda i: (i["unit"], i["option"]))}


UNIT_FILL_INS_TEXT = (
    "Fields a vanilla unit leaves empty that Battle Companies needs (race and unit type drive company limits and keyword rules). "
    "Each item gives values to use ONLY where the vanilla field is empty (the vanilla file is never modified); the values are "
    "taken from the unit's sibling profile and need a reviewer's confirmation."
)
UNIT_FILL_INS = [
    {"unit": "Hunter Orc Warrior", "race": ["Orc"], "faction": ["Gundabad"], "unitType": ["Warrior", "Infantry"], "baseSize": ["25mm"],
     "reason": "REVIEW: the vanilla Hunter Orc Warrior has no race, faction, unit type or base size; its sibling Hunter Orc Warg Rider has race Orc, faction Gundabad, "
               "types Warrior + Cavalry, 25mm. The Warrior is the same Orc on foot, so Infantry replaces Cavalry.",
     "source": "vanilla Hunter Orc Warg Rider (sibling profile)"},
    {"unit": "Vault Warden Team", "race": ["Dwarf"], "faction": ["Khazad-dum"], "unitType": ["Warrior", "Infantry"],
     "reason": "REVIEW: the vanilla Vault Warden Team is a bundle (it includes Iron Shield Bearer and Foe Spear Wielder) with no race, faction or unit type; "
               "both members are Dwarf, Khazad-dum, Warrior + Infantry, so the bundle takes those keywords.",
     "source": "vanilla Iron Shield Bearer and Foe Spear Wielder (included profiles)"},
]

MOUNT_FILL_INS_TEXT = (
    "Keywords (race, faction, unit type) a vanilla mount leaves empty. Each item gives values to use ONLY where the vanilla field is empty (the vanilla "
    "file is never modified). A faction of 'See Rider' means the rider's factions, as vanilla writes it for Shadowfax; the values are the "
    "curator's reading of the mount and need a reviewer's confirmation."
)
MOUNT_FILL_INS = [
    {"gear": "Brego", "race": ["Horse"], "faction": ["See Rider"], "unitType": ["Mount"],
     "reason": "REVIEW: Brego has no keywords in vanilla; he is a horse like Shadowfax (Horse, Mount, faction of the rider: Theodred of Rohan or Aragorn).",
     "source": "vanilla Shadowfax and Horse (sibling mounts)"},
    {"gear": "War boar", "race": ["Boar"], "faction": ["See Rider"], "unitType": ["Mount"],
     "reason": "REVIEW: the War boar (Dain Ironfoot's mount) has no keywords in vanilla; its race is the animal, the faction is the rider's, as for the War goat.",
     "source": "vanilla War goat (sibling mount)"},
    {"gear": "Elk", "race": ["Elk"], "faction": ["See Rider"], "unitType": ["Mount"],
     "reason": "REVIEW: Thranduil's Elk has no keywords in vanilla; the Great Elk is Elk, Mirkwood, Mount, but the faction is read as the rider's.",
     "source": "vanilla Great Elk (sibling mount)"},
    {"gear": "Cart", "race": ["Horse"], "faction": ["See Rider"], "unitType": ["Mount"],
     "reason": "REVIEW: Gandalf's Cart has no keywords in vanilla; the sibling Gandalf's Cart is Horse, Mount.",
     "source": "vanilla Gandalf's Cart (sibling mount)"},
    {"gear": "Moria Goblin Drum (Drum)", "unitType": ["Mount"],
     "reason": "REVIEW: the drum has no race, faction or type in vanilla (its keyword list is a copy of the White Warg's); it is a gear item of type mount, so only the Mount type is given.",
     "source": "vanilla gear type 'mount'"},
]


def mount_fill_ins():
    return {"text": MOUNT_FILL_INS_TEXT, "items": MOUNT_FILL_INS}


def unit_fill_ins():
    return {"text": UNIT_FILL_INS_TEXT, "items": UNIT_FILL_INS}
