"""Gear annotations: the class(es) of every piece of gear a Battle Company model can carry, and the special rules it grants.

`build_gear_annotations(bc, van)` returns the `data.gearAnnotations` section plus a report (`GearReport`). The pipeline:

1. `collect()` gathers every item a Battle Company can reach (wargear and options of the vanilla units it fields, what the
   vanilla gear entries include, the Battle Companies wargear/equipment tables, company unique wargear, creatures,
   wanderers, the wargear written in the company lists). Spellings that only differ in case, punctuation or a plural
   `s` are one item (`item_key`); the other spellings are kept as `aliases`.
2. `derive()` guesses classes and grants from the item's text with a few patterns. Grants are only derived from the
   explicit forms "has/gains the X special rule(s)" or from structured data (`specialRules` of a mount,
   `grantsSpecialRules` of a BC equipment item); anything that merely mentions a rule is NOT a grant.
3. `DECISIONS` (hand-written, below) overrides the derivation; those items are `curated`.
4. The report lists everything that needs the user's eyes: unresolved names, mentions of rules that were not recorded
   as grants (`unclear_grants`), items classified `other`.
"""
import collections
import re

from .text import fold

ABILITY_KEYS = ["attacksBonus", "chargeWoundBonus", "defenceBonus", "duelPenalty", "duelPenaltyUnlessSupportedBy", "duelPenaltyWith", "limit",
                "moveAndShoot", "noFireAfterMove", "noMovePenalty", "shielding", "support", "throwOnCharge", "twoHanded", "woundBonus",
                "woundBonusVsMountWhenCharged", "doubleWoundOnNatural6"]
# Abilities that raise a characteristic (user decision R8, 2026-10-22: conditional stat increases are never added to the unit's stats; the
# item text carries them and the player applies them when the condition holds). Each carries `conditional` and, when true, `condition`.
STAT_ABILITIES = ("defenceBonus", "attacksBonus")
CLASSES = ["mount", "melee", "two_handed", "spear", "lance", "shield", "armour", "bow", "crossbow", "throwing",
           "missile", "banner", "instrument", "consumable", "other"]
RANGED_CLASSES = ("bow", "crossbow", "throwing", "missile")  # carry a `ranged` profile (missile: no limit applies)


def item_key(name):
    """Key of an item: case, punctuation, plural `s` and the word "made" (Elven-made) do not make a different item."""
    words = []
    for w in fold(name.replace("&", " and ")).split():
        if w == "made":  # Elven-made weaponry is the generic Elven item
            continue
        if len(w) > 3 and w.endswith("s") and not w.endswith(("ss", "us")):
            w = w[:-1]
        words.append(w)
    return " ".join(words)


# ---------------------------------------------------------------------------------------------------------------------
# Hand-written decisions. Key = item_key(name). Fields: classes (ordered, first = main), grants, ranged, why.
# `why` is the justification written to `evidence` when the item text does not carry it.
# ---------------------------------------------------------------------------------------------------------------------
def _d(classes, why, grants=None, ranged=None, modifier=False):
    out = {"classes": classes, "why": why}
    if grants is not None:
        out["grants"] = grants
    if ranged is not None:
        out["ranged"] = ranged
    if modifier:
        out["modifier"] = True
    return out


# Spellings of the company lists that are NOT in the vanilla file and are read as another item (a guess: reported).
ITEM_ALIASES = {
    "warspear": "War spear",
    "uruk bow": "Uruk-hai Bow",
    "bow with poison arrows": "Bow with poisoned arrows",
    # The Wild Man of Dunland's option is "Two-handed axe", the Dwarf Warrior's too (the user confirmed 2026-10-20: it is the Two-handed
    # weapon; an axe is the miniature).
    "two-handed axe": "Two-handed weapon",
}
ITEM_ALIASES = {item_key(k): v for k, v in ITEM_ALIASES.items()}

# Gear only a mounted model can use: the live match strikes it through when the mount is lost. Lances are cavalry spears ("A Cavalry model
# using a lance..."); Barding is the steed's armour (BC Wargear table: Cavalry only). The War spear is NOT listed: it is a spear while dismounted.
REQUIRES_MOUNT = {item_key(n) for n in ("Lance", "Elven lance", "Barding")}

# The vanilla file spells some names two ways (the file is never modified): the capitalised spelling is listed as an alias too, so
# the app's canonical spelling (capitalised first) is the same everywhere. Key: item_key of the item.
VANILLA_SPELLING_VARIANTS = {
    "two-handed weapon": ["Two-handed Axe"],  # vanilla: Dwarf Warrior / Wild Man option "Two-handed axe", a Skin-changer's wargear "Two-handed Axe"
}
VANILLA_SPELLING_VARIANTS = {item_key(k): v for k, v in VANILLA_SPELLING_VARIANTS.items()}

UPGRADE = "named unit upgrade (a package of stats/rules), not a physical item"
DECISIONS = {
    # plain weapons that have no gear entry in the vanilla file
    "hand weapon": _d(["melee"], "a plain hand weapon"),
    "sword": _d(["melee"], "a sword is a hand weapon"),
    "axe": _d(["melee"], "an axe is a hand weapon"),
    "spear": _d(["spear"], "a spear (BC wargear table, type weapon)"),
    "two handed weapon": _d(["two_handed"], "a two-handed weapon"),
    "two handed axe": _d(["two_handed"], "a two-handed axe"),
    "hand and a half weapon": _d(["melee", "two_handed"], "hand-and-a-half weapon: wielded in one hand or two; classed as a melee weapon; wielded as a hand weapon or a two-handed weapon (decided by the user)"),
    "hand and a half hammer": _d(["melee", "two_handed"], "a hand-and-a-half weapon (hammer); wielded as a hand weapon or a two-handed weapon (decided by the user)"),
    "elven hand weapon": _d(["melee"], "an Elven-made hand weapon"),
    "elven hand and a half weapon": _d(["melee", "two_handed"], "an Elven-made hand-and-a-half weapon; wielded as a hand weapon or a two-handed weapon (decided by the user)"),
    "elven made hand and a half weapon": _d(["melee", "two_handed"], "an Elven-made hand-and-a-half weapon; wielded as a hand weapon or a two-handed weapon (decided by the user)"),
    "elven made hand and a half sword": _d(["melee", "two_handed"], "an Elven-made hand-and-a-half sword; wielded as a hand weapon or a two-handed weapon (decided by the user)"),
    "elven spear": _d(["spear"], "an Elven-made spear"),
    "elven made glaive": _d(["spear"], "BC unique wargear (Halls of Thranduil); the vanilla Elven Glaive is an Elven spear"),
    "elven lance": _d(["lance"], "an Elven-made lance (class lance, decided by the user)"),
    "elven pike": _d(["spear"], "an Elven-made pike"),
    "lance": _d(["lance"], "a lance: a cavalry spear, own class (decided by the user)"),
    "pike": _d(["spear"], "a pike: 'function in much the same way as spears'"),
    "wood elf spear": _d(["spear", "shield"], "'This is an Elven spear'; 'may use the Shielding rule even though they do not have a shield', so it also has the shield class (decided by the user: hover text + shield class, no grant). Every spear is a melee weapon, so `melee` is never listed with `spear`"),
    "twin blade": _d(["melee"], "two blades: a melee weapon"),
    "claws and teeth hand weapon": _d(["melee"], "natural weapons that count as hand weapons"),
    "beak and claw hand weapon": _d(["melee"], "natural weapons that count as hand weapons"),
    "fang hand weapon": _d(["melee"], "natural weapons that count as hand weapons"),
    "razor sharp teeth and talon hand weapon": _d(["melee"], "natural weapons that count as hand weapons"),
    "teeth and claw hand weapon": _d(["melee"], "natural weapons that count as hand weapons"),
    "berserker blade": _d(["melee", "two_handed"], "'This is a hand-and-a-half weapon'; used 'as a two-handed weapon' (decided by the user: all hand-and-a-half weapons are the same)"),
    "axe of lossarnach": _d(["spear", "two_handed"], "'used either as a spear or a two-handed weapon'"),
    "dunlending war axe": _d(["spear", "two_handed"], "vanilla keyword 'Dunlending War Axe' (data.keywords): 'This can be used as either a spear or a two-handed weapon.'"),
    "easterling halberd": _d(["spear", "two_handed"], "'used as either a pike or a two-handed weapon'"),
    "war spear": _d(["spear", "lance"], "'Counts as a lance while mounted, and a spear while dismounted'; the BC wargear row 'Warspear' is the same item"),
    "troll spear": _d(["spear"], "'This is a spear' with an additional rule of its own (vanilla gear entry; decided by the user 2026-10-22: its own weapon, not read as the Armoury Spear)"),
    "mattock": _d(["two_handed"], "'This is a two-handed weapon'"),
    "elven glaive": _d(["spear"], "'This is an Elven spear'"),
    "flaming brand": _d(["melee"], "BC wargear table lists it with the weapons (type weapon); unlike the Torching Brand, which is Equipment"),
    "whip": _d(["throwing", "melee"], "'counts as a throwing weapon ... can also be used as a hand weapon'", ranged={"range": 2, "strength": 1}),
    "troll chain": _d(["throwing"], "vanilla type throwingWeapon"),
    "blowpipe": _d(["missile"], "vanilla: '12\" Range, 2 Strength. Benefits from the Poisoned Weapons special rule'; a ranged weapon that counts against no limit (decided by the user)",
                   grants=["Poisoned Attacks (Blowpipe)"], ranged={"range": 12, "strength": 2}),
    "smoke bomb": _d(["throwing"], "'These are a throwing weapon with a Strength of 1'; no range is printed, so the 6\" of a generic throwing weapon (decided by the user)", ranged={"range": 6, "strength": 1}),
    "throwing weapon": _d(["throwing"], "BC wargear table: throwingWeapon", ranged={"range": 6, "strength": 3}),
    "throwing spear": _d(["throwing"], "BC wargear table: throwingWeapon", ranged={"range": 8, "strength": 3}),
    # armour and shields
    "light armour": _d(["armour"], "vanilla type armour (light armour: Defence bonus against Shooting only)"),
    "heavy dwarf armour": _d(["armour"], "vanilla type armour"),
    "dwarf armour": _d(["armour"], "vanilla type armour"),
    "light shield": _d(["shield"], "'a light shield' (may use Shielding, no Defence bonus); decided by the user: hover text + shield class, no grant"),
    "torching brand": _d(["other"], "decided by the user (Equipment, unlike the Flaming Brand weapon): 'They gain the Terror (Beast) and Terror (Cavalry) special rules. They gain Dominant (2).'",
                         grants=["Terror (Beast)", "Terror (Cavalry)", "Dominant (2)"]),
    "pavise": _d(["shield"], "'This is a shield that provides the bearer +3 Defence rather than +1' (not while Engaged)"),
    "iron shield": _d(["shield"], "vanilla type Shield: 'a shield that provides a bonus of +3 ... -1 penalty to their Duel Rolls, unless they are supported by a model with a Foe Spear'; reached through the Vault Warden Team (includes Iron Shield Bearer)"),
    "foe spear": _d(["spear"], "vanilla: 'This is a spear'; reached through the Vault Warden Team (includes Foe Spear Wielder)"),
    "easterling halberd and shield": _d(["shield"], "vanilla type shield (a bundle: Easterling Halberd + shield)"),
    "small shield": _d(["shield"], "'This is a Light Shield that gives the bearer +1 to their Defence Characteristic against Strikes' (BC overrides the vanilla Light Shield, which gives no bonus); a Light Shield may use Shielding"),
    "barding": _d(["armour"], "armour of the steed; the rider gains rules (see grants)"),
    # horns, drums, banners
    "war horn": _d(["instrument"], "a horn"),
    "company drum": _d(["instrument"], "'This is a War Drum'"),
    "company horn": _d(["instrument"], "'This is a War Horn'"),
    "company standard": _d(["banner"], "'This is a Pennant' (a banner)"),
    "company instrument": _d(["instrument"], "'This is an Instrument'"),
    "war drum": _d(["instrument"], "a drum (defined in the 'Exchange shield for war drum' entry)"),
    "drum": _d(["instrument"], "a drum"),
    "the drum": _d(["instrument"], "a drum"),
    "banner": _d(["banner"], "vanilla type banner"),
    # unit upgrades that sit in the vanilla gear list
    "helminga": _d(["other"], UPGRADE),
    "hearthguard": _d(["other"], UPGRADE),
    "king's guard": _d(["other"], UPGRADE),
    "lorien guard": _d(["other"], UPGRADE),
    "westfold redshield": _d(["other"], UPGRADE),
    "battlin brandybuck": _d(["other"], UPGRADE),
    "tookish hunter": _d(["other"], UPGRADE),
    "noldorian exile": _d(["other"], UPGRADE),
    "guardian of the king": _d(["other"], UPGRADE),
    "bow with poisoned arrows": _d(["bow"], "a Bow (24\" S2) in the Haradrim lists; the arrows are poisoned, so it grants Poisoned Attacks (Bow) (decided by the user)",
                                 grants=["Poisoned Attacks (Bow)"], ranged={"range": 24, "strength": 2}),
    "elven cloak": _d(["other"], "a cloak: neither weapon nor armour"),
    "envenom weapon (x)": _d(["other"], "decided by the user: not an item but a modifier; 'they will grant the Poisoned Attacks special rule' to the weapon bought it for, which the app shows as a poisoned copy of that weapon",
                         grants=["Poisoned Attacks (X)"], modifier=True),
    "healing herbs": _d(["consumable"], "used up once ('remove the Healing Herbs from their Equipment') (decided by the user)"),
    "wondrous cram": _d(["consumable"], "used up once ('remove the Wondrous Cram') (decided by the user)"),
    "dwarven brew": _d(["consumable"], "used once ('Once used...') (decided by the user)"),
    "scroll of courage": _d(["consumable"], "read once ('Once read...') (decided by the user)"),
    "scroll of hidden paths": _d(["consumable"], "read once at the start of a game (decided by the user)"),
    "venom back": _d(["other"], "unit upgrade of a creature (no rules text in the sources)"),
}

DECISIONS = {item_key(k): v for k, v in DECISIONS.items()}

# Spelling of a gained item inside an "Exchange A for B" option, so its definition (kept under the option's name in
# vanilla) can be found.
_EXCHANGE = re.compile(r"^(?:exchange|swap|replace)\s+(.+?)\s+(?:for|with)\s+(.+)$", re.I)
_SPLIT = re.compile(r"\s*(?:,\s*and\s+|,\s*|\s+and\s+|\s*&\s*)", re.I)
_RANGE = re.compile(r'(\d+)"\s*Range,\s*(\d+)\s*Strength', re.I)
_GRANT = re.compile(r"(?:has|have|gains?|gain)\s+the\s+(.+?)\s+special rules?", re.I)
_GRANT_KEYWORD = re.compile(r"grants (?:the bearer )?the ([A-Z][A-Za-z ]+?) keyword")


# What the curator would record for the unclear cases (shown to the user, NOT applied).
PROPOSALS = {
}
PROPOSALS = {item_key(k): v for k, v in PROPOSALS.items()}


# Mentions the user has ruled on (no grant): the hover text and the class carry the information.
SETTLED = {item_key("Light Shield"), item_key("Wood Elf Spear"), item_key("Envenom Weapon (X)")}


# ---------------------------------------------------------------------------------------------------------------------
# Rule text for tooltips. `CLASS_RULES[class]` is the rule every item of that class follows, paraphrased from the MESBG
# rulebook (sources/mesbg_rules.md, "Weapons" and "Shields"); `abilities` are the same rules as data. `ITEM_RULES[key]`
# replaces the class rule for a specific item (Pike, War spear, Light shield). The text is used as the item's
# `definition` when no source (vanilla gear, BC equipment) has a real one, and is appended when the source only says
# "This is a spear" / gives a profile.
# ---------------------------------------------------------------------------------------------------------------------
_SUPPORT = ("Support: an Infantry model with it that is not Engaged in Combat can assist a friendly model in base contact whose "
            "base is the same size or smaller. It adds one dice (its own Fight Value) to that Combat's Duel Roll and, if its side "
            "wins, makes one Strike with its own Strength. A Supporting model is not involved in the Combat (cannot be struck). "
            "It Supports one Combat per Fight Phase and cannot Support if Prone, unable to Activate, or if it shot that turn.")
# The shield's +1 Defence is lost with a bow, crossbow, two-handed weapon or pike, and when a hand-and-a-half weapon is used two-handed
# in that Combat (rulebook "Shields"; vanilla Shield: cancelledBy bow / Two Hand Weapon). Special rules lift some of the cancellations:
# Expert Rider keeps the bonus with a bow (not crossbow) while the rider stays mounted (rulebook Expert Rider; vanilla uncancelledByKeyword);
# Phalanx (Easterling Warrior...) uses a pike and a shield together without penalty (rulebook/vanilla Phalanx; REVIEW: read as also keeping the
# Defence bonus with a pike).
_SHIELD_LIFTS = [{"rule": "Expert Rider", "cancels": ["bow"], "whileMounted": True}, {"rule": "Phalanx", "cancels": ["Pike"]}]
_SHIELD_CANCEL = {"cancelledByClasses": ["bow", "crossbow", "two_handed"], "cancelledByItems": ["Pike"], "cancelledByTwoHandedUse": True,
                  "liftedByRules": _SHIELD_LIFTS}
CLASS_RULES = {
    "spear": {"text": _SUPPORT + " It is a hand weapon when the model is Engaged.",
              "abilities": [{"key": "support", "depth": 1}]},
    "lance": {"text": "A Cavalry model using a lance adds 1 to its To Wound rolls when making Strikes in a Combat in which it Charged "
                      "(not a Duel bonus), except when it Charged from within Difficult Terrain. A rider that dismounts or loses its "
                      "Mount discards the lance.",
              "abilities": [{"key": "chargeWoundBonus", "value": 1, "cavalryOnly": True, "notFromDifficultTerrain": True,
                             "discardOnDismount": True}]},
    "shield": {"text": "Adds 1 to Defence, but not if the model also has a bow, crossbow, two-handed weapon or pike (or uses a "
                       "hand-and-a-half weapon two-handed in that Combat). Shielding (Active): at the start of a Combat, before the "
                       "Duel Roll, the model may double its Attacks for that Combat; if it wins it cannot Strike and gets no benefit "
                       "from its weapons' special rules. In a Multiple Combat all friendly models must Shield or none can; a Shielding "
                       "model cannot be Supported; Cavalry cannot Shield; a Prone model can.",
               "abilities": [{"key": "defenceBonus", "value": 1, **{k: v for k, v in _SHIELD_CANCEL.items()}},
                             {"key": "shielding", "attacksMultiplier": 2, "cavalry": False}]},
    "two_handed": {"text": "Two-handed weapon: -1 to the Duel Roll (not on a natural 6) and +1 to To Wound rolls when making Strikes.",
                   "abilities": [{"key": "duelPenalty", "value": -1, "ignoredOnNatural": 6}, {"key": "woundBonus", "value": 1}]},
    "throwing": {"text": "Throwing weapon: can be used in the Shoot Phase even after a full Move (no Move-and-shoot penalty), or once per "
                         "turn in the Move Phase when Charging (stop 1\" short, throw, then continue the Charge against that target). "
                         "Counts towards the Throwing Weapon limit, not the Bow limit.",
                 "abilities": [{"key": "throwOnCharge", "perTurn": 1}, {"key": "noMovePenalty"}, {"key": "limit", "limit": "throwing"}]},
    "bow": {"text": "A model with a bow may shoot after moving, as long as it did not move more than half its Move Value; its Shoot "
                    "Value is then worsened by 1 (a natural 6 always hits). Missile weapon with the range and Strength shown. "
                    "Counts towards the Bow limit (Battle Companies count Heroes too); a model with a bow gets no Defence bonus from a shield.",
            "abilities": [{"key": "moveAndShoot", "maxMoveFraction": 0.5, "shootValuePenalty": 1}, {"key": "limit", "limit": "bow"}]},
    "crossbow": {"text": "A model with a crossbow may not fire in the same turn that it has moved. It may not fire even after a short move, unlike "
                         "a bow (which may fire after moving up to half its Move Value). Missile weapon with the range and Strength shown. Counts towards the "
                         "Bow limit; a model with a crossbow gets no Defence bonus from a shield.",
                 "abilities": [{"key": "noFireAfterMove"}, {"key": "limit", "limit": "bow"}]},
}
_RANGED_CLASSES = ["bow", "crossbow", "throwing", "missile"]  # the rulebook's "Missile Weapon" is every ranged weapon
_PIKE_ABILITIES = [{"key": "support", "depth": 2}, {"key": "twoHanded"},
                   {"key": "duelPenaltyWith", "value": -1, "with": ["shield"] + _RANGED_CLASSES, "unlessRules": ["Phalanx"]},
                   {"key": "woundBonusVsMountWhenCharged", "value": 1}]
ITEM_RULES = {
    # An item whose text gives its own numbers replaces the class rule (text "" = the source text says it all).
    "pavise": {"text": "", "abilities": [{"key": "defenceBonus", "value": 3, "notWhileEngaged": True, **_SHIELD_CANCEL},
                                         CLASS_RULES["shield"]["abilities"][1]]},
    "iron shield": {"text": "", "abilities": [{"key": "defenceBonus", "value": 3, **_SHIELD_CANCEL},
                                              {"key": "duelPenaltyUnlessSupportedBy", "value": -1, "item": "Foe Spear"},
                                              CLASS_RULES["shield"]["abilities"][1]]},
    # Not a shield: the Wood Elf Spear only borrows Shielding.
    "wood elf spear": {"text": CLASS_RULES["spear"]["text"],
                       "abilities": CLASS_RULES["spear"]["abilities"] + [CLASS_RULES["shield"]["abilities"][1]]},
    # The Elven Glaive is an Elven spear with a rule of its own (vanilla gear text; the BC document only prices it for the Halls of Thranduil).
    "elven glaive": {"text": CLASS_RULES["spear"]["text"],
                     "abilities": CLASS_RULES["spear"]["abilities"] + [
                         {"key": "attacksBonus", "value": 1, "applies": ["duelRoll", "strikes"], "choose": "one, before the Duel Roll",
                          "whenEngaged": True, "notSupported": True}]},
    # Vanilla: Merry's additionalRules "Battlin' Brandybucks have a Strength of 3" (the vanilla gear entry only has modifiesStats strength +1).
    "battlin brandybuck": {"text": "Battlin' Brandybucks have a Strength of 3.", "abilities": []},
    # Used as a pike or as a two-handed weapon, chosen per fight: each mode follows its own rule.
    "easterling halberd": {"text": "Used as a pike (Support two models deep; -1 to Duel Rolls with a shield or a Missile Weapon) or as a "
                                   "two-handed weapon (-1 to the Duel Roll, not on a natural 6; +1 To Wound); chosen for each fight.",
                           "abilities": [{**a, "mode": "pike"} for a in _PIKE_ABILITIES] +
                                        [{**a, "mode": "two_handed"} for a in CLASS_RULES["two_handed"]["abilities"]]},
    "pike": {"text": _SUPPORT + " A pike can also Support another pike-armed model that is itself Supporting (two Supports deep; beware "
                              "Trapping, only one model may Back Away). A pike needs two hands: -1 to Duel Rolls if the model also has "
                              "a shield or a Missile Weapon. If a Cavalry model Charges a pike-armed model, the pike-bearer gets +1 "
                              "To Wound when Striking the Mount.",
             "abilities": _PIKE_ABILITIES},
    "war spear": {"text": "A spear (Support, see Spear) when wielded by an Infantry model; a lance (+1 To Wound on a Charge) when wielded "
                          "by a Cavalry model, which does not have to discard it when it dismounts.",
                  "abilities": [{"key": "support", "depth": 1},
                                {"key": "chargeWoundBonus", "value": 1, "cavalryOnly": True, "notFromDifficultTerrain": True,
                                 "discardOnDismount": False}]},
    "light shield": {"text": "No Defence bonus, but the model may use Shielding (double Attacks for the Combat, then cannot Strike if it "
                             "wins; see Shield).",
                     "abilities": [{"key": "shielding", "attacksMultiplier": 2, "cavalry": False}]},
}
# The BC Small Shield is a Light Shield that, unlike the vanilla one, gives +1 Defence (against Strikes): the BC text wins, so the
# item carries the bonus (nothing in the BC text cancels it) and still has Shielding.
ITEM_RULES["small shield"] = {"text": "The bearer may also use Shielding (double Attacks for the Combat, then cannot Strike if it wins; see Shield).",
                              "abilities": [{"key": "defenceBonus", "value": 1, "against": "strikes", "notWithClasses": ["shield"]},
                                            ITEM_RULES["light shield"]["abilities"][0]]}
# The Troll Spear is a spear with an additional rule (vanilla gear text); the BC text and the app show it as its own item.
ITEM_RULES["troll spear"] = {"text": CLASS_RULES["spear"]["text"],
                             "abilities": CLASS_RULES["spear"]["abilities"] + [
                                 {"key": "doubleWoundOnNatural6", "wounds": 2, "notSupporting": True, "appliesTo": "strikes"}]}
ITEM_RULES["elven pike"] = ITEM_RULES["pike"]
ITEM_RULES["elven made glaive"] = ITEM_RULES["elven glaive"]
ITEM_RULES["elven lance"] = {"text": CLASS_RULES["lance"]["text"], "abilities": CLASS_RULES["lance"]["abilities"]}
ITEM_RULES = {item_key(k): v for k, v in ITEM_RULES.items()}
# A company list writes a generic item where the unit's profile has a specific one ("spear" for the Rivendell Warrior, whose option is
# "Shield and Elven spear"). The generator reads it as the profile's item so identity stays exact everywhere; every reading is a guess
# of the curator, listed in `data.wargearReadAs` for the user to confirm. Key: (unit, written item as in the lists, lower case).
READ_AS = {
    ("Rivendell Warrior", "spear"): ("Elven spear", "the Rivendell Warrior's option is 'Shield and Elven spear'"),
    ("Mordor Orc Warrior", "bow"): ("Orc bow", "the Mordor Orc Warrior's option is 'Orc bow'"),
    ("Hunter Orc Warrior", "bow"): ("Orc Bow", "the Hunter Orc Warrior's option is 'Orc Bow'"),
}
# ---------------------------------------------------------------------------------------------------------------------
# Item modifiers. An item is a BASE item plus MODIFIERS: "Elven spear" is a Spear that is an Elven weapon, "Bow with poisoned arrows"
# is a Bow that is poisoned. A modifier is defined once (`data.gearAnnotations.modifiers`: its rulebook text, its effect as data, what
# it does to identity and price) and the items carry `base` and `modifiers`. Sources: the MESBG rulebook "Weapons" chapter (Elven weapon,
# Master-forged; the other weapon sections are base weapons, not modifiers), the Poisoned Attacks rule and the Battle Companies
# wargear table / Envenom Weapon. Nothing in either document is called "Masterwork" or "Dwarven-made"; "Fire-based attacks" has no
# inherent effect (the Flaming Brand keeps its own text), so neither is a modifier.
#   identity "distinct": the modified item is its OWN item: a profile option that names a Spear does not take an Elven spear (the Elven
#                        weapon is restricted to Elves by being an option of the unit), and an Elven spear does not fill a Spear.
#   identity "variant":  the modified item fills the place of its base in a profile option (a Bow with poisoned arrows is the profile's Bow).
# ---------------------------------------------------------------------------------------------------------------------
MODIFIERS = [
    {"key": "elven", "name": "Elven weapon", "glossary": "Elven Weapon", "identity": "distinct", "appliesTo": "melee",
     "text": "If a model fights with an Elven weapon in a Combat, they will be more likely to win the Duel Roll in the result of a Drawn "
             "Combat. In these instances, a Good model armed with an Elven weapon will win the roll-off on a 3+, whilst an Evil model armed "
             "with an Elven weapon will win the roll-off on a 1-4. If both sides have an Elven weapon, neither side gains the benefit. "
             "Some Missile Weapons may also be classed as Elven weapons; however, the above bonus only applies to Melee Weapons. "
             "Any weapon with the word 'Elf' in its name is automatically considered to be an Elven weapon.",
     "tooltip": "If a model fights with an Elven weapon in a Combat, they will be more likely to win the Duel Roll in the result of a Drawn "
                "Combat. In these instances, a Good model armed with an Elven weapon will win the roll-off on a 3+, whilst an Evil model armed "
                "with an Elven weapon will win the roll-off on a 1-4. If both sides have an Elven weapon, neither side gains the benefit. "
                "Some Missile Weapons may also be classed as Elven weapons; however, the above bonus only applies to Melee Weapons.",
     "abilities": [{"key": "drawnCombatRollOff", "good": "3+", "evil": "1-4", "meleeOnly": True, "cancelledBy": "elven"}],
     "price": "Elven-made weaponry uses their generic equivalent values, but are restricted to Elf models unless otherwise noted.",
     "source": "MESBG rulebook, Elven weapon (p. 106); BC wargear table"},
    {"key": "masterForged", "name": "Master-forged", "glossary": "Master-forged", "identity": "distinct", "appliesTo": "two_handed",
     "text": "Some weapons may be classed as Master-forged. A model using a Master-forged weapon doesn't suffer the -1 penalty to the Duel "
             "Roll for using it as a two-handed weapon.",
     "abilities": [{"key": "noTwoHandedDuelPenalty"}],
     "source": "MESBG rulebook, Master-forged (p. 106); no Battle Companies item carries it today"},
    {"key": "poisoned", "name": "Poisoned", "identity": "variant", "appliesTo": "any",
     "text": "This weapon has the Poisoned Attacks special rule.",
     "abilities": [{"key": "grantsRule", "rule": "Poisoned Attacks"}],
     "source": "BC Envenom Weapon; Haradrim lists (bow with poisoned arrows); MESBG rulebook, blowpipe"},
]
# Item (key) -> (base item, [modifier keys]). The base is the generic item the modified one is a version of.
MODIFIED = {item_key(k): v for k, v in {
    "Elven pike": ("Pike", ["elven"]),
    "Elven spear": ("Spear", ["elven"]),
    "Elven lance": ("Lance", ["elven"]),
    "Elven throwing weapon": ("Throwing Weapon", ["elven"]),
    "Elven hand-and-a-half weapon": ("Hand-and-a-half Weapon", ["elven"]),
    "Elven-made Hand-and-a-half sword": ("Hand-and-a-half Weapon", ["elven"]),
    "Elven hand weapon": ("Hand weapon", ["elven"]),
    "Elven Glaive": ("Spear", ["elven"]),          # "This is an Elven spear" (vanilla); unique to the Halls of Thranduil, own price
    "Wood Elf Spear": ("Spear", ["elven"]),        # "This is an Elven spear" (vanilla); unique to Lothlorien, own price
    "Bow with poisoned arrows": ("Bow", ["poisoned"]),
}.items()}
# Elven items that are only a profile's starting wargear and have no armoury entry, though their base is in the price table. The Galadhrim
# Knight's "Elven-made hand-and-a-half sword" is not the Rivendell "Elven hand-and-a-half weapon" (kept apart as before; curator's reading).
NOT_SOLD = {item_key("Elven-made Hand-and-a-half sword")}
MODIFIER_BY_KEY = {m["key"]: m for m in MODIFIERS}
# Items that apply a modifier to a weapon of the bearer instead of being a weapon (Envenom Weapon).
GRANTS_MODIFIER = {item_key("Envenom Weapon (X)"): "poisoned"}


def apply_read_as(doc):
    """Replaces the written item by the one the unit's profile has (READ_AS) in every company list; returns the readings used."""
    used = {}

    def walk(o):
        if isinstance(o, dict):
            if isinstance(o.get("unit"), str) and isinstance(o.get("wargear"), list):
                for i, w in enumerate(o["wargear"]):
                    hit = READ_AS.get((o["unit"], w.strip().lower()))
                    if hit:
                        o["wargear"][i] = hit[0]
                        used[(o["unit"], w)] = hit
            for v in o.values():
                walk(v)
        elif isinstance(o, list):
            for v in o:
                walk(v)

    walk(doc["data"]["companies"])
    return [{"unit": u, "written": w, "item": h[0], "why": h[1]} for (u, w), h in sorted(used.items())]


# Items that are related but DISTINCT (same profile or same family, different identity: own availability, own text). Never merged:
# a company that may buy one may not buy the other. Each group is symmetric (every member lists the others) and carries the reason.
RELATED = [
    (["Elf Bow", "Longbow", "Esgaroth Bow", "Dwarf Longbow"], "Same missile profile (24\" Range, Strength 3), different make: each is its own item and a company's armoury may offer one without the others."),
    (["Shield", "Light Shield", "Small Shield"], "Shields of different kinds: the Light Shield gives no Defence bonus, the Small Shield gives +1 and Shielding."),
    (["Bow", "Short Bow", "Orc Bow", "Dwarf Bow"], "Bows of different makes and profiles; none is another's alias."),
]
RELATED_BY = collections.defaultdict(list)
for _names, _why in RELATED:
    for _n in _names:
        RELATED_BY[item_key(_n)].append(([m for m in _names if item_key(m) != item_key(_n)], _why))

# Weapons used as one of two things, chosen per fight (decided by the user); both modes are shown, and the two-handed mode
# cannot be used by a Hero with an Arm Wound (campaign.injuries.hero[armWound].disables).
DUAL_MODES = {item_key(n): m for n, m in {
    "Axe of Lossarnach": ("spear", "two_handed"),
    "Dunlending War Axe": ("spear", "two_handed"),
    "Easterling Halberd": ("pike", "two_handed"),
}.items()}
# A mount's profile is the vanilla gear entry of the same name unless it is listed here (the BC "Goat" is the vanilla War goat).
MOUNT_PROFILE = {item_key("Goat"): "War goat"}
_THIN = re.compile(r'^\s*(\d+"?\s*Range|This is an? )', re.I)


def _stat_condition(a):
    """Player-facing text of the SITUATIONAL condition of a stat-raising ability, or None when it only depends on the loadout.

    Situational (depends on what happens in the game: Engaged, Supported, the kind of attack...) -> conditional, text only, never added to
    the stats. Loadout (a bow, two-handed weapon or pike carried, being mounted, Expert Rider / Phalanx) -> computed by the app."""
    if a["key"] == "attacksBonus":
        return "Only while Engaged in Combat and not Supported: +1 Attacks for the Duel Roll or +1 Attacks for Strikes, chosen before the Duel Roll."
    parts = []
    if a.get("against") == "strikes":
        parts.append("Only against Strikes.")
    if a.get("notWhileEngaged"):
        parts.append("Not while Engaged in Combat.")
    return " ".join(parts) or None


def tooltip(item_k, classes, source_text):
    """(definition, abilities) for an item: the source text when it is real, plus the class rule when it is missing or thin."""
    rule = ITEM_RULES.get(item_k)
    if rule is None:
        parts = [CLASS_RULES[c] for c in classes if c in CLASS_RULES]
        rule = {"text": " ".join(p["text"] for p in parts), "abilities": [a for p in parts for a in p["abilities"]]} if parts else None
    source_text = (source_text or "").strip()
    thin = not source_text or bool(_THIN.match(source_text))
    if rule is None:
        return source_text, []
    abil = []
    for a in rule["abilities"]:
        if a["key"] in STAT_ABILITIES:  # situational -> conditional + text; loadout-only -> conditional false (computed by the app)
            cond = _stat_condition(a)
            a = {**a, "conditional": bool(cond), **({"condition": cond} if cond else {})}
        if a not in abil:
            abil.append(a)
    if thin and source_text:  # a sentence the source already says is not repeated
        source_text = source_text if source_text[-1] in ".!?" else source_text + "."
        known = {x.strip() for x in re.split(r"(?<=\.)\s+", source_text)}
        text = (source_text + " " + " ".join(x for x in re.split(r"(?<=\.)\s+", rule["text"]) if x.strip() not in known)).strip()
    elif thin:
        text = rule["text"]
    else:
        text = source_text
    return text, abil


class Item:
    def __init__(self, name):
        self.name = name
        self.spellings = {name}
        self.origins = set()
        self.vanilla = None   # first-occurrence vanilla gear entry
        self.bc = None        # BC equipment / wargear / unique wargear entry
        self.via_exchange = None  # the vanilla entry defining it under an "Exchange ..." name
        self.bc_type = None

    @property
    def definition(self):
        # A company's Unique Wargear is defined by the Battle Companies page (BC wins over vanilla, which may be a named hero's wording:
        # Smoke Bombs); the "Limit 1; Corsair Only" prefix is the restriction, kept in the company's own fields.
        if "company unique wargear" in self.origins and self.bc and self.bc.get("definition"):
            return re.sub(r"^Limit \d+(;[^.]*?\bOnly)?\s+", "", self.bc["definition"])
        for e in (self.vanilla, self.bc, self.via_exchange):
            if e and e.get("definition"):
                return e["definition"]
        return ""

    @property
    def vanilla_type(self):
        e = self.vanilla or self.via_exchange
        return (e or {}).get("type") or self.bc_type


class GearReport:
    def __init__(self):
        self.unresolved = []       # (text, where): names that are not items
        self.merged = []           # (key, spellings)
        self.unclear_grants = []   # (item, text, proposal)
        self.skipped_options = []  # prose option names that are not items
        self.by_class = collections.defaultdict(list)
        self.other = []            # (item, reason)
        self.grants = []           # (item, rules)
        self.notes = []


def _split_option(name):
    """Items named by an option string: `Shield and Spear` -> Shield, Spear; `Exchange lance for banner` -> banner."""
    m = _EXCHANGE.match(name.strip())
    if m:
        return [p for p in _SPLIT.split(m[2].strip()) if p], True
    return [p for p in _SPLIT.split(name.strip()) if p], False


def collect(bc, van, vanilla_units):
    bd, vd = bc["data"], van["data"]
    vgear = {}
    for g in vd["gear"]:
        vgear.setdefault(item_key(g["name"]), g)
    exch = {}  # key of the gained item -> vanilla entry named "Exchange ... for X"
    for g in vd["gear"]:
        m = _EXCHANGE.match(g["name"])
        if m:
            for p in _SPLIT.split(m[2]):
                exch.setdefault(item_key(p), g)
    items, report = {}, GearReport()

    def add(name, origin, bcentry=None, bc_type=None):
        name = re.sub(r"\s+", " ", name).strip()
        printed = name
        name = ITEM_ALIASES.get(item_key(name), name)
        name = name[:1].upper() + name[1:]
        k = item_key(name)
        if not k:
            return None
        it = items.get(k)
        if it is None:
            it = items[k] = Item(name)
            it.vanilla = vgear.get(k)
            it.via_exchange = exch.get(k) if not it.vanilla else None
        it.spellings.update({name, printed[:1].upper() + printed[1:]})
        it.origins.add(origin)
        if bcentry is not None and it.bc is None:
            it.bc = bcentry
        if bc_type and not it.bc_type:
            it.bc_type = bc_type
        return it

    def expand(name, origin):
        """Adds the items a wargear / option string stands for (a bundle or an exchange is not an item itself)."""
        k = item_key(name)
        g = vgear.get(k)
        if "(hand weapon" in name.lower():  # natural weapons: "Claws and teeth (hand weapons)" is one item
            add(name, origin)
            return
        if g is not None and g.get("includes"):
            for inc in g["includes"]:
                expand(inc["name"], origin)
            return
        parts, exchange = _split_option(name)
        if exchange:
            report.skipped_options.append(name)
        if len(parts) == 1 and not exchange:
            add(name, origin)
            return
        for p in parts:
            expand(p, origin)

    by_name = collections.defaultdict(list)
    for kind in ("warriors", "heroes"):
        for e in vd[kind]:
            by_name[e["name"]].append(e)
    def unit_gear(e, seen=()):
        for w in e.get("wargear", []):
            expand(w, "vanilla unit wargear")
        for o in e.get("options", []):
            expand(o["name"], "vanilla unit option")
        for inc in e.get("includes") or []:  # a bundled unit (Vault Warden Team) brings the gear of its members
            if inc["name"] not in seen:
                for m in by_name.get(inc["name"], []):
                    unit_gear(m, seen + (inc["name"],))

    for u in sorted(vanilla_units):
        for e in by_name.get(u, []):
            unit_gear(e, (u,))
    for e in bd["wargear"]["items"]:
        for n in e["names"]:
            add(n, "BC wargear table", e, e["type"])
    for e in bd["equipment"]["items"]:
        add(e["name"], "BC equipment", e)
    for c in bd["companies"]:
        for e in c.get("uniqueWargear", []):
            add(e["name"], "company unique wargear", e)
    for e in bd["wanderers"]["items"] + bd["creatures"]["items"]:
        for w in e.get("wargear", []):
            expand(w, "wanderer / creature wargear")

    def company_units(o):
        if isinstance(o, dict):
            if isinstance(o.get("unit"), str):
                for w in o.get("wargear", []):
                    yield w
            for v in o.values():
                yield from company_units(v)
        elif isinstance(o, list):
            for v in o:
                yield from company_units(v)

    for w in company_units(bd["companies"]):
        expand(w, "company lists")
    for stage in bd["wargear"]["armourUpgrade"]["stages"]:
        for n in stage:
            if n != "None":
                expand(n, "BC armour stages")
    for it in items.values():  # display name: the vanilla gear entry, else the Battle Companies table
        if it.vanilla:
            it.name = it.vanilla["name"]
        elif it.bc and it.bc.get("name"):
            it.name = it.bc["name"]
        it.spellings.add(it.name)
    return items, report


def _keyword_names(bc, van):
    names = {k["name"] for k in van["data"]["keywords"]} | {k["name"] for k in bc["data"]["keywords"]}
    names |= {p["name"] for p in bc["data"]["parameterisedRules"]}
    # rules defined in the Scenario Special Rules section (Command Support...) are Battle Companies rules too
    names |= {r["name"] for r in bc.get("campaign", {}).get("scenarioSelection", {}).get("specialRules", [])}
    return names


def resolve_rule(name, kwnames, params):
    """The rule a grant names: a vanilla / BC rule (spaces and case do not matter: Horselord = Horse Lord) or an
    instance of a parameterised rule (`Terror (Cavalry)`, `Poisoned Attacks (Bow)`). None if it resolves to nothing."""
    f = fold(name).replace(" ", "")
    for k in kwnames:
        if fold(k).replace(" ", "") == f:
            return k
    m = re.match(r"^(.+?)\s*\((.+)\)$", name.strip())
    if m and any(fold(p) == fold(m[1]) for p in params):
        return name.strip()
    return None


def derive(item, kwnames, params):
    """Guess classes and grants from the item's data; returns (classes, grants, ranged, evidence, unclear)."""
    name, d, t = item.name, item.definition, (item.vanilla_type or "")
    low, tl = fold(name), t.lower()
    classes, evidence = [], None
    if tl == "mount":
        classes, evidence = ["mount"], "vanilla type: mount"
    elif tl == "armour":
        classes, evidence = ["armour"], "vanilla type: armour"
    elif tl == "shield":
        classes, evidence = ["shield"], "vanilla type: shield"
    elif tl == "bow":
        classes = ["crossbow" if "crossbow" in low else "bow"]
        evidence = "vanilla type: bow" + (" ('may not fire in the same turn that it has moved')" if "crossbow" in low else "")
    elif tl == "throwingweapon":
        classes, evidence = ["throwing"], "vanilla type: throwingWeapon"
    elif tl == "banner":
        classes, evidence = ["banner"], "vanilla type: banner"
    elif tl == "horn":
        classes, evidence = ["instrument"], "vanilla type: horn"
    elif tl == "spear":
        classes, evidence = ["spear"], "vanilla type: spear"
    elif tl == "two-handed weapon":
        classes, evidence = ["two_handed"], "vanilla type: two-handed weapon"
    ranged = None
    m = _RANGE.search(d)
    if m and classes and classes[0] in RANGED_CLASSES:
        ranged = {"range": int(m[1]), "strength": int(m[2])}
    grants, unclear = [], []
    e = item.vanilla or item.via_exchange or {}
    for r in e.get("specialRules", []):
        grants.append(r)
    for r in (item.bc or {}).get("grantsSpecialRules", []):
        if r not in grants:
            grants.append(r)
    for g in _GRANT_KEYWORD.finditer(d):  # "grants the bearer the Command Support keyword" (a Scenario Special Rule)
        match = resolve_rule(g[1], kwnames, params)
        if match and match not in grants:
            grants.append(match)
    for g in _GRANT.finditer(d):
        if len(g[1]) > 80 or ":" in g[1]:  # not the plain "has the X special rule" form: left for the user
            unclear.append((g[1][:60] + " ...", "grant sentence in an unusual form"))
            continue
        for part in _SPLIT.split(g[1]):
            part = part.strip()
            match = resolve_rule(part, kwnames, params)
            if match:
                if match not in grants:
                    grants.append(match)
            elif len(part) < 40 and ":" not in part:
                unclear.append((part, "named in 'has/gains the ... special rule' but is not a known rule"))
    return classes, grants, ranged, evidence, unclear


def mentions(item, kwnames, grants):
    """Rule names that appear in the text without being recorded as a grant (for the user to rule on)."""
    d = item.definition
    out = []
    for k in sorted(kwnames, key=len, reverse=True):
        if len(k) < 6 or k in grants or fold(k) == fold(item.name) or any(k in o for o in out) \
                or any(fold(g).startswith(fold(k)) for g in grants):
            continue
        if re.search(r"(?<![A-Za-z])" + re.escape(k) + r"(?![A-Za-z])", d):
            out.append(k)
    if re.search(r"Shielding", d) and "Shielding" not in out:
        out.append("Shielding")
    for m in re.finditer(r"grants (?:the bearer )?the ([A-Z][A-Za-z ]+?) keyword", d):
        if m[1] not in grants:
            out.append(m[1] + " (a keyword)")
    return out


_THIS_IS = re.compile(r"^This is (?:a|an) (.+?)(?: that\b| which\b|[.,;])", re.I)

# Hero-only unit upgrades (options reserved to one named hero's list) that no company selects. They stay annotated ON
# PURPOSE: a campaign organiser may hand them out (decided by the user). Do not flag them as unreachable.
KEEP_ANNOTATED = ["Hearthguard", "Westfold Redshield", "King's Guard", "Guardian of the King", "Venom-back"]


def inherited_grants(item, items, kwnames, params, seen=()):
    """S1: an item whose text says "This is a(n) X" gets X's grants (the War horn's Dominant (2) for the Company Horn)."""
    m = _THIS_IS.match(item.definition.strip())
    if not m:
        return []
    target = items.get(item_key(m[1]))
    if target is None or target is item or target in seen:
        return []
    dec = DECISIONS.get(item_key(target.name))
    if dec and "grants" in dec:
        own = list(dec["grants"])
    else:
        own = list(derive(target, kwnames, params)[1])
    return own + [g for g in inherited_grants(target, items, kwnames, params, seen + (item,)) if g not in own]


def build_gear_annotations(bc, van, vanilla_units):
    kwnames = _keyword_names(bc, van)
    params = [p["name"] for p in bc["data"]["parameterisedRules"]]
    items, report = collect(bc, van, vanilla_units)
    priced_keys = {item_key(ITEM_ALIASES.get(item_key(n), n)) for row in bc["data"]["wargear"]["items"] for n in row["names"]}
    unique_keys = {item_key(u["name"]) for co in bc["data"]["companies"] for u in co.get("uniqueWargear", [])}
    out = []
    for k in sorted(items, key=lambda x: fold(items[x].name)):
        it = items[k]
        classes, grants, ranged, evidence, unclear = derive(it, kwnames, params)
        dec = DECISIONS.get(k)
        status = "derived"
        if dec:
            status = "curated"
            classes = dec["classes"]
            grants = dec.get("grants", grants)
            ranged = dec.get("ranged", ranged)
            evidence = dec["why"]
        if classes == ["mount"] and k in MOUNT_PROFILE and not grants:  # the BC Goat has the rules of the vanilla War goat
            pg = next((g for g in van["data"]["gear"] if g["name"] == MOUNT_PROFILE[k]), {})
            grants = list(pg.get("specialRules", []))
        if not (dec and "grants" in dec):
            for g in inherited_grants(it, items, kwnames, params):
                if g not in grants:
                    grants = grants + [g]
        if not classes and "BC equipment" in it.origins:
            classes, evidence = ["other"], "BC equipment: " + (_short(it.definition, 100) or "a utility item")
        if not classes:
            report.unresolved.append((it.name, "no class could be derived and no decision exists: " + ", ".join(sorted(it.origins))))
            classes, status, evidence = ["other"], "derived", evidence or "no class could be derived"
        if evidence is None:
            evidence = _short(it.definition) or it.name
        entry = {"name": it.name, "classes": classes}
        if grants:
            entry["grants"] = grants
        if ranged:
            entry["ranged"] = ranged
        if dec and dec.get("modifier"):
            entry["weaponModifier"] = True
        if k in REQUIRES_MOUNT:
            entry["requiresMount"] = True
        if grants and classes[0] == "mount":
            entry["grantedTo"] = "mount"  # a mount's own special rules (Timid, Impaler...), not the rider's
        others = sorted({s for s in it.spellings if fold(s) != fold(it.name)} | set(VANILLA_SPELLING_VARIANTS.get(k, [])))
        if others:
            entry["aliases"] = others
            report.merged.append((it.name, others))
        defn, abil = tooltip(k, classes, it.definition)
        if not defn and classes[0] == "armour":
            vg = next((g for g in van["data"]["gear"] if item_key(g["name"]) == k and g.get("modifiesStats")), None)
            if vg:
                defn = "Adds %d to Defence." % sum(m["modifier"] for m in vg["modifiesStats"] if m["stat"] == "defence")
                entry["statChanges"] = [{"stat": m["stat"], "delta": m["modifier"], "conditional": False} for m in vg["modifiesStats"]]
        if k in MODIFIED:  # the tooltip explains each modifier (the rule is defined once, in `modifiers`)
            for mk in MODIFIED[k][1]:
                m = MODIFIER_BY_KEY[mk]
                tip = m.get("tooltip", m["text"])
                if tip not in (defn or ""):
                    defn = ((defn + " ") if defn else "") + f"{m['name']}: {tip}"
        if defn:
            entry["definition"] = defn
        if abil:
            entry["abilities"] = abil
        m_is = _THIS_IS.match((it.definition or "").strip())
        if m_is and items.get(item_key(m_is[1])) not in (None, it):  # "This is a X": the item is X plus extras (Small Shield, Company Horn...)
            entry["isA"] = items[item_key(m_is[1])].name
        if k in DUAL_MODES:
            entry["modes"] = {"choice": "perFight", "options": [
                {"key": m, "class": ("spear" if m == "pike" else m), "usableWithArmWound": m != "two_handed"} for m in DUAL_MODES[k]]}
        if classes[0] == "mount":
            vm = {item_key(g["name"]): g["name"] for g in van["data"]["gear"] if g.get("type") == "mount"}
            pn = MOUNT_PROFILE.get(k) or vm.get(k)
            if pn:
                entry["profile"] = {"source": "vanilla", "collection": "gear", "name": pn}
        if k in MODIFIED:
            base_name, mods = MODIFIED[k]
            base_item = items.get(item_key(base_name))
            assert base_item is not None, f"{it.name}: base item {base_name!r} is not an item"
            entry["base"], entry["modifiers"] = base_item.name, list(mods)
            # derived for the rule engine: an Elven-made item without a price of its own (unique wargear has one) is sold at its base's
            # price (unless it is starting wargear that is never sold, NOT_SOLD); a "variant" modifier makes the item fill its base's place in a profile option.
            if "elven" in mods and k not in unique_keys and item_key(base_item.name) in priced_keys and k not in NOT_SOLD:
                entry["pricedAs"] = base_item.name
            if any(MODIFIER_BY_KEY[m]["identity"] == "variant" for m in mods):
                entry["variantOf"] = base_item.name
        if k in GRANTS_MODIFIER:
            entry["grantsModifier"] = GRANTS_MODIFIER[k]
        if k in RELATED_BY:
            rel = []
            for names, why in RELATED_BY[k]:
                for n in names:
                    rel.append({"name": items[item_key(n)].name if item_key(n) in items else n, "note": why})
            entry["relatedTo"] = rel
        entry["evidence"] = evidence
        entry["status"] = status
        out.append(entry)
        for c in classes:
            report.by_class[c].append(it.name)
        if it.name in KEEP_ANNOTATED:
            report.notes.append(f"{it.name}: hero-only unit upgrade kept annotated on purpose (decided by the user)")
        if classes == ["other"]:
            report.other.append((it.name, evidence))
        if grants:
            report.grants.append((it.name, grants))
        for part, why in ([] if dec and "grants" in dec else unclear):
            report.unclear_grants.append((it.name, part, why, it.definition, PROPOSALS.get(k, "")))
        ment = [] if dec and "grants" in dec else mentions(it, kwnames, grants)
        if dec and k in SETTLED:
            ment = []
        if ment:
            report.unclear_grants.append((it.name, ", ".join(ment), "mentioned in the text, not recorded as a grant", it.definition, PROPOSALS.get(k, "")))
    section = {
        "text": "Class(es) and granted special rules of every piece of gear a Battle Company model can carry. Generated by "
                "tools/bc/gear.py from the item texts plus a table of decisions. `classes` say what the item IS (the first is "
                "the main one: icon lead and sorting); `grants` are special rules the bearer gets (only recorded when the text "
                "or the Battle Companies document says the bearer gets the rule; `grantedTo: mount` = the rules belong to the mount); "
                "`ranged` is the missile profile (bow, crossbow, throwing, missile); `weaponModifier` marks a modifier bought for one weapon (`grantsModifier` says which); `base` and `modifiers` say that the item is a version of a generic item (Elven spear = Spear + Elven, see `modifiers` above; `pricedAs` and `variantOf` are derived from them); `aliases` "
                "are other spellings of the same item; `definition` is the tooltip text (the source text, plus the rulebook text of its class when the source has none or only a profile), `abilities` the same rules as data, `profile` points a mount at its stat block; `status` is `derived` (pattern) or `curated` (decided by hand).",
        "modifiers": MODIFIERS,
        "items": out,
    }
    return section, report


def _short(text, n=140):
    text = re.sub(r"\s+", " ", text).strip()
    return text if len(text) <= n else text[:n].rsplit(" ", 1)[0] + " ..."


def _cell(s):
    return str(s).replace("|", "\\|").replace("\n", " ")


def write_review(section, report, path):
    """tools/gear_review.md: one row per item for the independent reviewer, then the coverage report and the open questions."""
    items = section["items"]
    L = ["# Gear annotations: review sheet", "",
         "Generated by `tools/build_battle_companies.py` (do not edit). One row per item of `data.gearAnnotations` in the "
         "Battle Companies json. `derived` = classes/grants found by a pattern; `curated` = decided by hand in "
         "`tools/bc/gear.py` (`DECISIONS`).", "",
         f"{len(items)} items.", "", "## Items", "",
         "| Item | Classes | Grants | Ranged | Evidence | Status | Aliases |", "|---|---|---|---|---|---|---|"]
    for i in items:
        r = i.get("ranged")
        L.append("| " + " | ".join(_cell(x) for x in (
            i["name"], ", ".join(i["classes"]), ", ".join(i.get("grants", [])) or "-",
            f'{r["range"]}" S{r["strength"]}' if r else "-", i["evidence"], i["status"],
            ", ".join(i.get("aliases", [])) or "-")) + " |")
    L += ["", "## Coverage by main class", ""]
    for c in CLASSES:
        names = [i["name"] for i in items if i["classes"][0] == c]
        if names:
            L.append(f"* **{c}** ({len(names)}): " + ", ".join(names))
    L += ["", "## Items that also have other classes", ""]
    for i in items:
        if len(i["classes"]) > 1:
            L.append(f"* {i['name']}: {', '.join(i['classes'])}")
    L += ["", "## Classified `other` (with the reason)", ""]
    for n, why in report.other:
        L.append(f"* {n}: {why}")
    L += ["", "## Items with grants", ""]
    for n, g in report.grants:
        L.append(f"* {n}: {', '.join(g)}")
    L += ["", "## Open questions: rules mentioned but not recorded as grants", ""]
    for n, m, why, d, pr in report.unclear_grants:
        L.append(f"* **{n}** ({m}): {why}. Text: \"{_short(d, 300)}\"" + (f" Proposal: {pr}" if pr else ""))
    L += ["", "## Kept on purpose", ""] + [f"* {n}" for n in report.notes]
    L += ["", "## Spellings read as one item (case, punctuation, plural `s`)", ""]
    for n, o in report.merged:
        L.append(f"* {n} = {', '.join(o)}")
    L += ["", "## Possibly the same item, NOT merged (different words)", ""]
    byk = collections.defaultdict(list)
    for i in items:
        byk[" ".join(w for w in item_key(i["name"]).split() if w != "made")].append(i["name"])
    for k, v in byk.items():
        if len(v) > 1:
            L.append(f"* {', '.join(v)}")
    L += ["", "## Option texts that are not items (their items are annotated on their own)", ""]
    for n in sorted(set(report.skipped_options)):
        L.append(f"* {n}")
    L += ["", "## Alias guesses (names not in the vanilla file, read as another item)", ""]
    for k, v in ITEM_ALIASES.items():
        L.append(f"* {k.replace(' arrow', ' arrows')} -> {v}")
    L.append("")
    path.write_text("\n".join(L), encoding="utf-8")
