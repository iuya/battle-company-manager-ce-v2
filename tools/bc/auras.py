"""data.auras: the fixed radii around a unit (a banner, a Hero's Stand Fast, "friendly models within 3" of this Hero") that the app
draws as rings in Tabletop Simulator and shows as chips on the unit cards (bead 4fe.2, spec docs/aura-ranges.md, contract docs/definitions.md).

Curated, like data.ruleTargets: there is no machine-readable radius in the rules data. Every entry has a `reason`; every rule, item, power or
heroic action that is reachable by a Battle Company unit and whose text names a distance is either an entry, or in EXCLUDED (with a category
and a reason), or a weapon range (listed by the generator from the gear annotations). Vanilla rules of named characters no company can field
are listed as `unreachable` (computed, so a new company that can field one makes the coverage check fail).
"""
import re

from .refs import referenced
from .text import fold

TEXT = (
    "Fixed radii around the bearer, in inches from the edge of its base, inside which models are affected by (or eligible for) a rule or an item the bearer has: "
    "a banner, a Hero's Stand Fast, 'friendly models within 3\" of this Hero'. Not auras: weapon ranges, 'in the same combat', a radius around somebody else "
    "(receiver-side rules), around an objective, a marker or a target. An entry is found on a unit by `source` (item, rule or power names; the Hero rules of the "
    "rulebook; a heroic action); `kind` is the colour group, `when` and `group` say which dialog switch governs it. Rings of one figure with the same kind, radius "
    "and tier are merged (labels joined). `replaces` lists entries that the bearer does not get when it has this one. `radius` is written in the source text (or is "
    "the sum of `radiusParts`)."
)

# id, label, colour name, Okabe-Ito rgb (docs/aura-ranges.md section 4.4), legend sentence
KINDS = [
    ("banner", "Banner", "yellow", "F0E442", "Banners: friendly models in a Fight re-roll a Duel die"),
    ("command", "Command", "orange", "E69F00", "Courage and the Hero's shouts: Stand Fast, the Heroic Actions, horns and drums"),
    ("support", "Support", "sky blue", "56B4E9", "Bonuses to friendly models near the bearer"),
    ("enemy", "Enemy", "vermillion", "D55E00", "Penalties, denial or reactions against the enemy"),
    ("utility", "Utility", "reddish purple", "CC79A7", "Light, markers and other effects"),
]

# The dialog switches. `group` of an entry: target -> targeting, cast -> spells, radius over 12 -> large, else its `when` (or an explicit GROUP_OVERRIDE).
GROUPS = [
    ("always", "Rules always in force", "Passive rules and every Hero's Stand Fast", True),
    ("declared", "Rules that need an action", "Heroic Actions, Active rules and once-per-game rules (a Might or Will Point, or a declaration)", True),
    ("conditional", "Rules that need an event", "Rules that start working after something happened in the game (Claim the Throne)", True),
    ("cast", "Spells in effect", "Magical Powers of the bearer while they are in effect", False),
    ("target", "Targeting ranges", "The radius only limits the choice of one model (Gatekeeper, Spy...): informative, noisy", False),
    ("large", "Large rings, over 12\"", "Rings larger than 12 inches fill the table (the war horn's 24\")", False),
    ("optional", "Optional rings", "Light of a Torch: correct but not a rule that decides who is affected", False),
]
LARGE = 12  # inches: a radius over this is in the group `large`
GROUP_OVERRIDE = {"torch-6": "optional"}

WHEN = ["always", "declared", "conditional", "cast", "target"]
AFFECTS = ["friendly", "enemy", "any", "self"]
SOURCE_TYPES = ["item", "rule", "hero", "heroicAction", "power"]


def _e(id, source, label, radius, kind, affects, when, tip, concept, reason, los=False, keywords=(), replaces=(), merge=(), parts=(), text_from=(), bearer=None, pattern=None):
    s = dict(source)
    if pattern:
        s["pattern"] = pattern
    e = {"id": id, "source": s, "label": label, "radius": radius, "kind": kind, "affects": affects, "when": when, "los": los,
         "keywords": list(keywords), "tip": tip, "concept": concept, "reason": reason}
    if merge:
        e["mergeLabels"] = list(merge)
    if parts:
        e["radiusParts"] = list(parts)
    if text_from:
        e["textFrom"] = list(text_from)
    if replaces:
        e["replaces"] = list(replaces)
    if bearer:
        e["bearer"] = bearer
    return e


def item(*names):
    return {"type": "item", "names": list(names)}


def rule(*names):
    return {"type": "rule", "names": list(names)}


def power(*names):
    return {"type": "power", "names": list(names)}


def action(*names):
    return {"type": "heroicAction", "names": list(names)}


HERO = {"type": "hero"}
MASTER_OF_BATTLE = ["Master of Battle (3+)"]  # the only variant a company can have (Rutabi, the Warlord option); the others are in `unreachable`
FURY = ["Fury", "Fury (Easterling)", "Fury (Moria Goblin)", "Fury (Mordor Orc)", "Fury (Isengard Orc)", "Fury (Isengard Uruk-hai)", "Fury (Beast)", "Fury (Spider)"]

ENTRIES = [
    # ---- items -------------------------------------------------------------------------------------------------------------------------------
    _e("banner-3", item("Banner"), "Banner", 3, "banner", "friendly", "always",
       "A friendly model in a Fight within 3\" of a banner may re-roll one Duel die; the bearer needs line of sight and is -1 on Duel Rolls.", "Banner",
       "The Banner text: a 3\" area of effect. Vanilla banner options (plain, exchanges, bundles) all resolve to the item Banner.", los=True),
    _e("standard-3", item("Company Standard"), "Company Standard", 3, "banner", "friendly", "always",
       "Friendly models within 3\" may re-roll Duel dice of 1.", "Company Standard",
       "A Pennant (a banner) of the Battle Companies equipment table: the same 3\" as a banner; merges with it into one ring."),
    _e("instrument-3", item("Company Instrument"), "Company Instrument", 3, "support", "friendly", "always",
       "A friendly model within 3\" that suffers a wound rolls a D6 and ignores the wound on a 6.", "Company Instrument",
       "Battle Companies equipment table: 3\" around the bearer."),
    _e("drum-12", item("War drum", "Company Drum"), "War Drum", 12, "command", "friendly", "declared",
       "The war drum's free Heroic March shout reaches 12\" (models with the keywords after the drum).", "War Drum",
       "War drum text: the At the Double of the free Heroic March has range 12\" instead of 6\". The Company Drum is a War Drum (isA)."),
    _e("horn-24", item("War Horn", "Company Horn"), "War Horn", 24, "command", "friendly", "always",
       "Friendly models within 24\" of a war horn get +1 to Courage Tests.", "War Horn",
       "War horn text: 24\". Correct but it fills the table: group `large`, off by default (Q3). The Company Horn is a War Horn (isA)."),
    _e("horn-standfast-9", item("War Horn", "Company Horn"), "War Horn: Stand Fast +3\"", 9, "command", "friendly", "declared",
       "Once per game a war horn extends the Hero's own Stand Fast by 3\", to 9\".", "War Horn",
       "War horn text: once per game +3\" to a friendly Hero's Stand Fast, or to the bearer's own when it is a Hero (6\" + 3\"). Only a Hero bearer has the ring "
       "(for a Warrior bearer the 9\" is centred on the Hero, not on the horn).", parts=(6, 3), text_from=("Stand Fast",), bearer="Hero"),
    _e("torch-6", item("Torching Brand"), "Torching Brand", 6, "utility", "any", "always",
       "Torch light: models within 6\" are seen as if in daylight.", "Torching Brand",
       "Battle Companies equipment table: 'illuminate the area within 6\"'. Light, not a rule on a model: group `optional`, off by default (spec 2.4)."),
    # ---- every Hero (rulebook rules that are not in the unit's rule list) and the heroic actions ----------------------------------------------
    _e("standfast-6", HERO, "Stand Fast", 6, "command", "friendly", "always",
       "A Hero that passed its Broken Army Courage Test calls Stand Fast: friendly Warriors within 6\" in sight pass theirs.", "Stand Fast",
       "Glossary Stand Fast (every Hero). With Me! (Heroic Move) and Loose (Heroic Shoot) are universal Heroic Actions with the same 6\": their labels are merged.",
       los=True, keywords=("Warrior",), merge=("With Me!", "Loose"), text_from=("With Me!", "Loose")),
    _e("atthedouble-6", action("Heroic March"), "At the Double", 6, "command", "friendly", "declared",
       "The Heroic March shout: friendly models within 6\" of the Hero's start add to their Move and must end within 6\".", "At the Double",
       "Heroic March is not universal: the Ranger path (or a vanilla profile) gives it. The shout is in the glossary term At the Double."),
    _e("resolve-6", action("Heroic Resolve"), "Heroic Resolve", 6, "command", "friendly", "declared",
       "Friendly models within 6\" get an extra free die on Resist Tests until the End Phase.", "Heroic Resolve",
       "Warlord path starting Heroic Action; merged into the Hero's 6\" command ring."),
    _e("takeaim-6", action("Heroic Accuracy"), "Take Aim", 6, "command", "friendly", "declared",
       "The Take Aim shout of Heroic Accuracy: friendly models within 6\" re-roll failed In The Way Tests.", "Take Aim",
       "Marksman path starting Heroic Action; spec 2.4 lists it as support but says it merges into the 6\" command ring, so it is command (REVIEW)."),
    # ---- paths and rules -----------------------------------------------------------------------------------------------------------------------
    _e("voice-12", rule("Voice of Command"), "Voice of Command", 12, "command", "friendly", "always",
       "The Hero's Stand Fast reaches 12\" instead of 6\".", "Voice of Command",
       "Warlord rule: replaces the Hero's 6\" command rings (Stand Fast and the shouts) with one 12\" ring.",
       los=True, replaces=("standfast-6", "atthedouble-6", "resolve-6", "takeaim-6")),
    _e("tirade-6", rule("Furious Tirade"), "Furious Tirade", 6, "enemy", "enemy", "always",
       "Enemy Warriors within 6\" cannot use any Hero's Stand Fast.", "Furious Tirade",
       "Warlord rule (also the named rule of Lobelia, which the Battle Companies rule replaces).", keywords=("Warrior",)),
    _e("bellow-3", rule("Commanding Bellow"), "Commanding Bellow", 3, "banner", "friendly", "always",
       "Friendly models treat this Hero as a banner with range 3\".", "Commanding Bellow",
       "Warlord rule: 'treat this Hero as a banner with a range of 3\"'. Not in the spec table (omitted by mistake); added as a banner ring.", los=True),
    _e("claim-6", rule("Claim the Throne"), "Claim the Throne", 6, "banner", "friendly", "conditional",
       "After the enemy Leader falls within 6\" of this Hero, friendly models treat it as a banner with range 6\".", "Claim the Throne",
       "Claimant rule (and Freca's vanilla rule, replaced by it): the banner range 6\" starts after a game event.", los=True),
    _e("dolamroth-6", rule("Lord of Dol Amroth"), "Lord of Dol Amroth", 6, "banner", "friendly", "always",
       "Other friendly Dol Amroth models treat this Hero as a banner with range 6\".", "Lord of Dol Amroth",
       "Fiefdoms rule: 'a banner with a range of 6\"'. Not in the spec table (omitted by mistake); added as a banner ring.", keywords=("Dol Amroth",), los=True),
    _e("leadbyexample-6", rule("Lead by Example"), "Lead by Example", 6, "banner", "friendly", "conditional",
       "After slaying 3 enemy Heroes or Monsters, other friendly Orcs and Trolls treat this model as a banner with range 6\".", "Lead by Example",
       "Vanilla rule of Buhrdur, Hill Troll Chieftain (a unit companies can field). Not in the spec table; added as a banner ring.", keywords=("Orc", "Troll"), los=True),
    _e("hmm-6", rule("Heroic Move Mastery"), "Heroic Move Mastery", 6, "support", "friendly", "always",
       "Friendly Heroes within 6\" gain a bonus on the roll-off for Heroic Moves.", "Heroic Move Mastery", "Tactician mastery rule.", keywords=("Hero",)),
    _e("portents-6", rule("Reading the Portents"), "Reading the Portents", 6, "support", "friendly", "declared",
       "Spend a Will Point to let a friendly model within 6\" re-roll one Duel die.", "Reading the Portents",
       "Tactician rule (Oin's vanilla rule of the same name is replaced by it)."),
    _e("knife-3", rule("Knife Mastery"), "Knife Mastery", 3, "support", "friendly", "always",
       "Friendly models with a bow or crossbow within 3\" in sight re-roll To Wound rolls of 1.", "Knife Mastery", "Sharpshooter rule.", los=True),
    _e("hold-3", rule("\"Hold positions, loose the arrows!\""), "\"Hold positions, loose the arrows!\"", 3, "support", "friendly", "always",
       "Friendly models within 3\" re-roll failed To Hit rolls when they shoot, if they did not Move.", "\"Hold positions, loose the arrows!\"", "Marksman rule."),
    _e("goodsort-3", rule("A Good Sort"), "A Good Sort", 3, "support", "self", "always",
       "Within 3\" of another friendly Hero this Hero has +1 Attack.", "A Good Sort",
       "Soldier rule (also Dori's vanilla rule, replaced by it): the bearer benefits when another Hero is near."),
    _e("relentless-6", rule("Relentless Commander"), "Relentless Commander", 6, "support", "friendly", "declared",
       "If you give Priority to the opponent, friendly models within 6\" re-roll To Wound rolls of 1.", "Relentless Commander", "Soldier rule."),
    _e("kingprot-6", rule("The King's Protector"), "The King's Protector", 6, "support", "friendly", "declared",
       "The chosen Hero engaged within 6\" gets a free Heroic Combat from this Hero.", "The King's Protector", "Shieldbearer rule.", keywords=("Hero",)),
    _e("advisor-6", rule("Chief Advisor"), "Chief Advisor", 6, "support", "friendly", "always",
       "A friendly Hero within 6\" that spends a Might Point rolls a D6: on a 6 it is free.", "Chief Advisor", "Ranger rule.", keywords=("Hero",)),
    _e("toarms-3", rule("To Arms!"), "To Arms!", 3, "support", "friendly", "declared",
       "Once per game friendly models within 3\" get +1 Strength until the end of the turn.", "To Arms!",
       "Berserker rule (also Thorin's vanilla rule, replaced by it)."),
    _e("forglory-6", rule("For Glory"), "For Glory", 6, "support", "friendly", "declared",
       "Once per game friendly models within 6\" get +1 Fight and re-roll To Wound rolls of 1.", "For Glory", "Duelist rule."),
    _e("remember-3", rule("Remember Today"), "Remember Today", 3, "support", "friendly", "always",
       "May use the Might, Will and Fate of another friendly Hero within 3\".", "Remember Today", "Osgiliath & Ithilien rule.", keywords=("Hero",)),
    _e("lossarnach-3", rule("Lord of Lossarnach"), "Lord of Lossarnach", 3, "support", "friendly", "always",
       "Other friendly Lossarnach models within 3\" re-roll To Wound rolls of 1.", "Lord of Lossarnach",
       "Fiefdoms rule (also Forlong's vanilla rule, replaced by it).", keywords=("Lossarnach",)),
    _e("lamedon-3", rule("Chief of Lamedon"), "Chief of Lamedon", 3, "support", "friendly", "always",
       "Other friendly Lamedon models within 3\" pass their Courage Tests.", "Chief of Lamedon",
       "Fiefdoms rule (also Angbor's vanilla rule, replaced by it).", keywords=("Lamedon",)),
    _e("vale-3", rule("Lord of the Vale"), "Lord of the Vale", 3, "support", "friendly", "always",
       "Other friendly models within 3\" re-roll To Hit rolls of 1 when they shoot, if they did not Move.", "Lord of the Vale", "Fiefdoms rule."),
    _e("moneybags-6", rule("Moneybags"), "Moneybags", 6, "support", "friendly", "declared",
       "Friendly Lake-town models within 6\" get +1 Fight and treat this Hero as a banner.", "Moneybags",
       "Lake-town rule (also the vanilla rule of the Master of Lake-town, replaced by it).", keywords=("Lake-town",)),
    _e("callous-3", rule("Callous Leadership"), "Callous Leadership", 3, "support", "friendly", "declared",
       "Friendly Goblins within 3\" get +1 To Wound; one that loses a Duel risks a wound.", "Callous Leadership",
       "Moria rule (replaces Durburz's vanilla 6\" rule).", keywords=("Goblin",)),
    _e("seize-3", rule("Seize Command"), "Seize Command", 3, "support", "friendly", "declared",
       "When the Leader has one Wound left within 3\" in sight, it may be removed to give this Hero command.", "Seize Command", "City of Umbar rule.", los=True),
    _e("sword-6", rule("You Have My Sword"), "You Have My Sword", 6, "support", "friendly", "declared",
       "A Hobbit engaged within 6\" lets this Hero declare a free Heroic Combat.", "You Have My Sword", "Wanderers in the Wild rule.", keywords=("Hobbit",)),
    _e("keensight-12", rule("Keen Sight", "Keen Sight (Falcon)"), "Keen Sight", 12, "enemy", "enemy", "always",
       "Enemy models within 12\" gain no benefit from Stalk Unseen (a Falcon apart from its Hero).", "Keen Sight (Falcon)",
       "Falcon creature figure (its own `aura` agrees) and the Crebain of the vanilla file."),
    _e("battle-6", rule(*MASTER_OF_BATTLE), "Master of Battle", 6, "enemy", "enemy", "declared",
       "When an enemy Hero within 6\" declares a Heroic Action, roll a D6 to copy it.", "Master of Battle (3+)",
       "Warlord option and Rutabi, General of the Dragon Legion (the pattern covers the other variants if a company ever gets them).", keywords=("Hero",), pattern="Master of Battle (X+)"),
    # ---- targeting ranges: the radius only limits the choice of one model (default off) ----------------------------------------------------
    _e("gatekeeper-6", rule("Gatekeeper"), "Gatekeeper", 6, "enemy", "enemy", "target",
       "May select one enemy Infantry model within 6\" in sight.", "Gatekeeper", "Tactician rule: chooses one model.", los=True, keywords=("Infantry",)),
    _e("spy-12", rule("Spy"), "Spy", 12, "enemy", "enemy", "target",
       "Whilst within 12\" and in sight of the chosen enemy Hero, its Heroic Actions may be cancelled.", "Spy", "Tactician rule: one chosen Hero.", los=True, keywords=("Hero",)),
    _e("challenge-6", action("Heroic Challenge"), "Heroic Challenge", 6, "enemy", "enemy", "target",
       "Names an enemy Hero within 6\" of the same Heroic Tier or higher.", "Heroic Challenge", "Claimant starting Heroic Action: names one Hero.", keywords=("Hero",)),
    _e("swap-3", rule("Swap with Me!"), "Swap with Me!", 3, "support", "friendly", "target",
       "May swap places with a friendly Goblin on a 25mm base within 3\".", "Swap with Me!", "Goblin-Town rule: one chosen model.", keywords=("Goblin",)),
    _e("fell-12", rule("A fell light is in them", "A Fell Light is in Them"), "A fell light is in them", 12, "enemy", "enemy", "target",
       "At the end of its Move may choose one enemy model within 12\" in sight.", "A fell light is in them", "Chooses one model.", los=True),
    _e("glove-3", rule("Handler's Glove"), "Handler's Glove", 3, "utility", "any", "target",
       "The Hero may release its creature anywhere within 3\".", "Handler's Glove", "Places one creature; the radius only limits where."),
    _e("hymn-12", rule("Enchanting Song"), "Enchanting Song: Hymn of Elbereth", 12, "support", "friendly", "target",
       "The Hymn of Elbereth chooses one friendly model within 12\".", "Enchanting Song", "Wood Elf Sentinel: chooses one model."),
    # ---- other (declared) ------------------------------------------------------------------------------------------------------------------
    _e("lay-3", rule("Enchanting Song"), "Enchanting Song: Lay of Gondolin", 3, "support", "friendly", "declared",
       "The Lay of Gondolin gives the Sentinel and friendly models within 3\" the Terror rule.", "Enchanting Song", "Wood Elf Sentinel song."),
    # ---- Magical Powers in effect (cast, off by default) ---------------------------------------------------------------------------------
    _e("aoc-6", power("Aura of Command"), "Aura of Command", 6, "command", "friendly", "cast",
       "While in effect, the caster and friendly models within 6\" pass Courage Tests.", "Aura of Command", "Magical Power."),
    _e("fury-6", power(*FURY), "Fury", 6, "support", "friendly", "cast",
       "While in effect, friendly models within 6\" with the keywords in brackets pass Courage Tests.", "Fury", "Magical Power (and its keyword variants).", pattern="Fury (X)"),
    _e("dismay-6", power("Aura of Dismay"), "Aura of Dismay", 6, "enemy", "enemy", "cast",
       "While in effect, enemy models within 6\" of the caster take -1 on Courage Tests.", "Aura of Dismay", "Magical Power."),
    _e("fog-6", power("Fog of Disarray"), "Fog of Disarray", 6, "enemy", "enemy", "cast",
       "While in effect, enemy models within 6\" of the caster take -1 on Intelligence Tests.", "Fog of Disarray", "Magical Power."),
    _e("fear-6", power("Instill Fear"), "Instill Fear", 6, "enemy", "enemy", "cast",
       "While in effect, enemy models within 6\" of the caster count as Fearful.", "Instill Fear", "Magical Power."),
    _e("blinding-6", power("Blinding Light"), "Blinding Light", 6, "utility", "any", "cast",
       "While in effect, the area within 6\" of the caster is daylight and shots at models in it hit only on a 6.", "Blinding Light", "Magical Power."),
    _e("vines-3", power("Writhing Vines"), "Writhing Vines", 3, "utility", "enemy", "cast",
       "The Vine Marker is placed within 3\" of the caster and makes 3\" around it difficult terrain for the enemy.", "Writhing Vines",
       "Magical Power: the radius is also around the marker; the ring shows where the marker may go."),
]

# type, names, pattern, radius (the first or main one), category, reason. A reachable rule with a distance in its text that is not an entry must be here.
RECEIVER = ("receiver-side", "the radius is around another model (a named hero, a General, the Leader): the source is not the bearer; no ring in v1 (spec 2.1, Q4)")
EXCLUDED = [
    ("rule", ["The King's Guard"], None, 3, *RECEIVER),
    ("rule", ["Riders of Eorl"], None, 6, *RECEIVER),
    ("rule", ["Arise, Riders of Rohan!"], None, 12, *RECEIVER),
    ("rule", ["Pride of Dol Amroth"], None, 3, *RECEIVER),
    ("rule", ["Loyal to the Captain"], None, 6, *RECEIVER),
    ("rule", ["Warrior of the Dragon Cult"], None, 3, *RECEIVER),
    ("rule", ["Favour of the War Chief"], None, 3, *RECEIVER),
    ("rule", ["Protectors of the White Tree"], None, 6, *RECEIVER),
    ("rule", ["Everyone has a Price"], None, 6, *RECEIVER),
    ("rule", ["Onwards Men of Esgaroth"], None, 3, *RECEIVER),
    ("rule", ["The Honour of Lamedon"], None, 3, *RECEIVER),
    ("rule", ["King's Knights"], None, 6, *RECEIVER),
    ("rule", ["Mindless Killers"], None, 6, "receiver-side", "the 6\" is measured from a friendly Corsair bo'sun, not from the bearer (receiver-side, like the 13 of spec 2.1)"),
    ("rule", ["Pillager"], None, 3, "centred-elsewhere", "the radius is around an Objective Marker"),
    ("rule", ["Rapid Fire"], None, 3, "centred-elsewhere", "the radius is around the first target of the shot"),
    ("rule", ["Master of Traps"], None, 3, "centred-elsewhere", "radii around Trap Markers and Objectives (where a marker may not be placed, when it is triggered)"),
    ("rule", ["Combat Synergy"], "Combat Synergy (X)", 1, "centred-elsewhere", "two named Heroes within 1\" of each other swap places: a pair, not an area"),
    ("rule", ["Volley Fire"], None, 6, "weapon-range", "adds 6\" to the range of the bearer's shooting attacks"),
    ("rule", ["Throwing Stones"], None, 8, "weapon-range", "the range of a throwing weapon"),
    ("rule", ["Spider Webs"], None, 8, "weapon-range", "the range of a throwing weapon"),
    ("rule", ["Fly"], None, 4, "movement", "the Move Value of a model that chooses not to Fly"),
    ("rule", ["Stalk Unseen"], None, 6, "visibility", "a concealment distance (line of sight): models further than 6\" cannot see the bearer; not a radius of affected models. Keen Sight, which removes it, has a ring"),
    ("rule", ["Silent Hunters"], None, 6, "visibility", "a concealment distance (line of sight), like Stalk Unseen"),
    ("power", ["Call Winds"], None, 3, "effect-length", "the distance a target is blown (D3+3\"); the range of the power is its `range`"),
    ("power", ["Tremor"], None, 3, "effect-length", "a line of D3+3\" from the caster, not an area around it"),
]

# not a candidate list: whole parts of the data that hold distances but are no rule of a unit
OUT_OF_SCOPE = [
    ("data.scenarioDetails", "scenario layout, deployment and scoring (objectives, board edges, entry points): centred on objectives, markers or the table, never on a unit"),
    ("data.gearAnnotations ranged items", "missile profiles (bows, crossbows, throwing weapons, the blowpipe, the whip): weapon ranges, listed in `excluded` by the generator"),
    ("data.companies", "company rules print no distance of their own (the validator checks the keywords they use, which are covered above)"),
]

RANGE = re.compile(r'\d+\s*["”]')


def has(n, text):
    """True when the number `n` (inches) is written in the text as N" (not as part of a longer number)."""
    return re.search(rf'(?<![\d.]){re.escape(str(n))}\s*["”]', text) is not None


def group_of(e):
    if e["id"] in GROUP_OVERRIDE:
        return GROUP_OVERRIDE[e["id"]]
    if e["when"] == "target":
        return "target"
    if e["when"] == "cast":
        return "cast"
    if e["radius"] > LARGE:
        return "large"
    return e["when"]


def candidates(bc, van):
    """The coverage universe: {fold(name): (type, name, text)} of every reachable rule, power, item and heroic action whose text names a distance
    (the Battle Companies text wins over the vanilla one of the same name). Ranged items (weapon ranges) are returned apart."""
    d = bc["data"]
    need = referenced(bc, van)
    bck = {fold(k["name"]): k for k in d["keywords"]}
    vk = {}
    for k in van["data"]["keywords"]:
        vk.setdefault(fold(k["name"]), k)
    out = {}
    for f in {fold(x) for x in need["keywords"]} | set(bck):
        k = bck.get(f) or vk.get(f)
        if k and RANGE.search(k["definition"]):
            out[("rule", f)] = (k["name"], k["definition"])
    for p in d["parameterisedRules"]:
        if RANGE.search(p["definition"]):
            out[("rule", fold(p["pattern"]))] = (p["pattern"], p["definition"])
    for p in d["magicalPowers"]["items"]:
        if RANGE.search(p["definition"]):
            out[("power", fold(p["name"]))] = (p["name"], p["definition"])
    vg = {}
    for g in van["data"]["gear"]:
        vg.setdefault(fold(g["name"]), g)
    weapons = {}
    for a in d["gearAnnotations"]["items"]:
        text = " ".join([a.get("definition", ""), (vg.get(fold(a["name"])) or {}).get("definition", "")])
        banner_or_instrument = {"banner", "instrument"} & set(a.get("classes", []))
        if a.get("ranged"):
            if RANGE.search(text):
                weapons[fold(a["name"])] = (a["name"], text)
        elif RANGE.search(text) or banner_or_instrument:
            out[("item", fold(a["name"]))] = (a["name"], text)
    for it in d["equipment"]["items"]:
        if RANGE.search(it["definition"]):
            out.setdefault(("item", fold(it["name"])), (it["name"], it["definition"]))
    for p in d["paths"]:
        h = p.get("startingHeroicAction")
        if h and RANGE.search(h["definition"]):
            out[("heroicAction", fold(h["name"]))] = (h["name"], h["definition"])
    return out, weapons


def unreachable(bc, van):
    """Vanilla rules with a distance in their text that no Battle Companies unit can have (rules of named characters of other books)."""
    need = {fold(x) for x in referenced(bc, van)["keywords"]}
    own = {fold(k["name"]) for k in bc["data"]["keywords"]}
    names = {k["name"] for k in van["data"]["keywords"] if RANGE.search(k["definition"]) and fold(k["name"]) not in need | own}
    return sorted(names)


def build(doc, van):
    d = doc["data"]
    kw = {fold(k["name"]) for k in van["data"]["keywords"]} | {fold(k["name"]) for k in d["keywords"]}
    powers = {fold(p["name"]) for p in d["magicalPowers"]["items"]}
    annotated = {fold(a["name"]) for a in d["gearAnnotations"]["items"]}
    actions = {fold(p["startingHeroicAction"]["name"]) for p in d["paths"] if p.get("startingHeroicAction")}
    bad = []
    for e in ENTRIES:
        s = e["source"]
        known = {"rule": kw, "power": powers, "item": annotated, "heroicAction": actions | {fold("Heroic March")}, "hero": None}[s["type"]]
        for n in s.get("names", []):
            if known is not None and fold(n) not in known:
                bad.append(f"{e['id']}: {s['type']} {n!r}")
    assert not bad, f"auras.py names that do not exist in the data: {bad}"
    entries = []
    for e in ENTRIES:
        e = dict(e)
        e["group"] = group_of(e)
        entries.append(e)
    excluded = [{"type": t, "names": n, **({"pattern": p} if p else {}), "radius": r, "category": c, "reason": why} for t, n, p, r, c, why in EXCLUDED]
    _, weapons = candidates(doc, van)
    excluded.append({"type": "item", "names": sorted(w[0] for w in weapons.values()), "radius": 0, "category": "weapon-range",
                     "reason": "a weapon's range (missile profile), not a rule radius: a possible later feature (Q11)"})
    return {
        "text": TEXT,
        "kinds": [{"id": i, "label": l, "colour": c, "rgb": rgb, "text": t, "default": True} for i, l, c, rgb, t in KINDS],
        "groups": [{"id": i, "label": l, "text": t, "default": df} for i, l, t, df in GROUPS],
        "entries": entries,
        "excluded": excluded,
        "outOfScope": [{"scope": s, "reason": r} for s, r in OUT_OF_SCOPE],
        "unreachable": unreachable(doc, van),
    }
