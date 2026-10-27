# Aura ranges in Tabletop Simulator and on the unit cards (spec)

Status: **built, waiting for the owner's smoke tests in Tabletop Simulator;** built: the helper (4.3, 4.4, 4.7 helper cases, bead 4fe.5, see [`tabletop-simulator.md`](tabletop-simulator.md#built-aura-rings-in-the-helper-bead-4fe5)), the data (`data.auras`, bead 4fe.2, [`definitions.md`](definitions.md#aura-ranges-dataauras)) and core (`UnitAuras`, `AuraSelection`, `CompanyAuraSummary`, payload v2 with caps and notes, `TTSAuras`, glue test through the helper; beads 4fe.3, 4fe.4, 4fe.9, see [`tabletop-simulator.md`](tabletop-simulator.md#built-the-names-payload-v2-bead-4fe4)); the demo campaign "Aura parade" carries every reachable aura for the owner smoke test ([`demo-world.md`](demo-world.md#aura-parade)); the web layer: the Auras section of the dialog with its query parameters (bead 4fe.6) and the chips on the unit cards (bead 4fe.7), see [`frontend.md`](frontend.md#copy-as-a-tabletop-simulator-army-list-built), tested by e2e flow `96-tts-auras` (bead 4fe.8, [`testing.md`](testing.md)). Not verified: the owner smoke tests in the game (section 8).** Owner request (2026-10-27): draw the ranges of auras (a banner, Stand Fast, "within 3" of this model" rules) as
rings around the miniatures in Tabletop Simulator (TTS), as the owner has seen in the Age of Sigmar (AoS) workshop mod, and show the same
ranges on the unit cards of the app. Epic `battle-company-manager-ce-v2-4fe` (label `auras`; children listed at the end). This page is the contract
between the definitions (`data.auras`), core (`UnitAuras`, `TTSNames` v2), the Lua helper and the web layer. Related:
[`tabletop-simulator.md`](tabletop-simulator.md) (list, ArmyNames payload, helper), [`unit-keywords.md`](unit-keywords.md) (chips, rule targets),
[`definitions.md`](definitions.md) (rules data).

## 1. Summary

- An **aura** is a fixed radius, in inches from the edge of the bearer's base, inside which models are affected or eligible by a rule or an item the
  bearer has (a banner, a Hero's Stand Fast, "friendly models within 3" of this Hero ..."). Not an aura: a weapon's range, "choose a model in the same
  combat", a radius around somebody else (the rule's receiver side), around an objective or a target.
- **Scale: 1 inch = 1.0 TTS world unit** (high confidence, section 3). A ring is a TTS vector line (`setVectorLines`) in the figure's own space, so it
  moves, turns and is picked up with the figure with no tracking code. This is exactly what the AoS mod does, and what the FTC mod's own range circle does.
- The data is a new curated table `data.auras` in the Battle Companies json (curator-owned); core turns it into `[]Aura` per unit
  (`RuleSet.UnitAuras`); the same list feeds three consumers: the unit cards (a chip "Aura 6"" with tooltip), the **Auras** section of the TTS dialog
  (types, legend, switches) and the names payload (**ArmyNames v2**, field `a` per figure), which the helper draws.
- The one hazard found: the FTC mod's own script on every figure **wipes all vector lines of the figure** (it calls `setVectorLines({})` two seconds after
  load, and on its hot keys). The helper must (re)draw after that, merge with the lines that exist, and offer a Redraw button (section 4.3).

## 2. What is an aura in this app's data

### 2.1 Method

Read-only audit (2026-10-27) of: vanilla `data.keywords` (all special rules, 182 of them mention a range), `data.gear` (items, banners, horns, drums),
the 117 vanilla units that the 37 companies use (their special rules), the glossary (Banner, War Horn, War Drum, Stand Fast, Heroic Actions...), the Battle Companies
json (`keywords` 29 range hits, `equipment.items`, the 13 `paths` with their progression and starting heroic actions, `magicalPowers`, `parameterisedRules`,
`gearAnnotations`, `scenarioDetails`). A regular expression for `within N"`, `N" of`, `range of N"`, `N" range` found the candidates; every candidate was read
and classified. Facts that shaped the design:

- Most of the 182 vanilla hits are rules of **named characters** (Gandalf, Thranduil...) that no Battle Company unit has; they are out of scope until a
  company can field the character. The validator of the data bead lists them as `unreachable`, not as errors.
- 13 reachable vanilla rules are **receiver-side**: the unit gets a bonus "whilst within X" of *a named hero / a General / a Hero of its faction*
  (King's Guard, Riders of Eorl, Arise, Riders of Rohan!, Pride of Dol Amroth, Loyal to the Captain, Warrior of the Dragon Cult, Favour of the War Chief,
  Protectors of the White Tree, Everyone has a Price, Onwards Men of Esgaroth, The Honour of Lamedon, King's Knights). The ring would have to be on the *source*,
  and the source is not the bearer (and may not exist in a BC company). **No ring in v1** (default); an entry with the source resolved to the company's Leader is a
  possible follow-up (`Arise, Riders of Rohan!`: 12" around a Rohan General).
- Hero abilities of the **paths** (the main source of BC auras) are copied as text in `paths[].progression[]` and again as flat entries in `keywords[]`; the
  card shows them from the path, the data bead keys on the rule name.
- There is no "aura" field in the data and no machine-readable radius anywhere. `data.auras` is therefore curated (like `data.ruleTargets`), with a
  validator that keeps it honest (section 5.1).

### 2.2 The criterion

A rule or item gets a **ring** when all of these hold:

1. the radius is a number of inches written in the rule text, measured from the bearer (rulebook "Measuring": between the closest parts of the bases, so
   the ring is drawn at the radius **plus the bearer's base radius**);
2. the radius is centred on the bearer (not on a target, an objective, a marker, the table edge, or another model);
3. it decides *which models are affected or eligible* (everything in the area, or the candidates for a trigger).

Not rings: line of sight and engagement ("in the same combat", "Supporting"), base contact, weapon ranges, "up to its Move Value", Control Zone (1", every model),
targeting of a single model *if* the owner does not want it (ambiguous, default off, see `target`), and anything with no number.
**Line of sight** does not stop a ring: the ring shows the radius, the tooltip says "needs line of sight". **Bearer state** (alive, standing) does not change
the radius: a figure removed from the table takes its ring with it; the live list's fallen units get no ring (section 4.5).

### 2.3 Fields of an aura

| Field | Meaning |
|---|---|
| `radius` | inches, from the base edge |
| `kind` | the colour group: `banner`, `command`, `support`, `enemy`, `utility` (section 2.6) |
| `affects` | `friendly`, `enemy`, `any`, or `self` (the bearer benefits when others are near, A Good Sort) |
| `keywords` | a keyword filter of the affected models (Lossarnach, Lamedon, Goblin, bow models, Warrior only...), shown in the tooltip, not drawn |
| `when` | `always` (passive, or a rule of every Hero), `declared` (an Active rule, a Heroic Action, a once-per-game rule: needs a Will/Might Point or a declaration), `conditional` (only after a game event: Claim the Throne, a horn used), `cast` (a Magical Power in effect), `target` (the radius only limits the choice of one model) |
| `los` | the rule needs line of sight (tooltip) |
| `bearer` | state or keyword conditions of the bearer: `Hero`, a unit type, `Cavalry`... (only decides who gets the entry) |

### 2.4 Classified list: rings (v1)

"Source" is the card text the player knows; all are in the data already. Radii are the rule text. `D` = default shown in the dialog (section 4.4).

**Items (the unit carries the item; class from `gearAnnotations`)**

| Source | Radius | Kind | Affects | When | Notes | D |
|---|---|---|---|---|---|---|
| Banner (any vanilla option that gives a banner, class `banner`: 31 plain `Banner` options and 12 exchanges) | 3" | banner | friendly | always | Engaged or Supporting friendly model with line of sight; bearer must be standing and has -1 Duel Roll; a banner limited to keywords works for those models only | on |
| Company Standard (a Pennant, class `banner`) | 3" | banner | friendly | always | also re-roll of 1s in Duel Rolls within 3"; same ring as the banner, one ring when both | on |
| Company Instrument | 3" | support | friendly | always | models that suffer a wound roll a D6, ignore on a 6 | on |
| Company Drum, War drum (vanilla options and hero items) | 12" | command | friendly | declared | free Heroic March, "At the Double" range 12" and finish within 12" (keyword filter in brackets) | on |
| Company Horn, War horn (class `instrument`) | 24" | command | friendly | always | +1 to Courage Tests of models within 24"; also +3" to a Hero's Stand Fast once per game | **off** (large) |
| War horn: Stand Fast extension | 9" (6"+3") | command | friendly | declared | only for a Hero or a model within the Hero's Stand Fast; once per game | on |
| Torching Brand (Torch) | 6" | utility | any | always | lit area, daylight; terror (Beast, Cavalry) for the bearer | off |

**Every Hero (rulebook rules, not in the unit's rule list)**

| Source | Radius | Kind | Affects | When | Notes | D |
|---|---|---|---|---|---|---|
| Stand Fast (glossary) | 6" | command | friendly Warriors | always | broken army only, line of sight, the Hero must have passed its own test | on |
| Heroic Move "With Me!", Heroic March "At the Double", "Loose", Heroic Accuracy "Take Aim", Heroic Resolve | 6" | command | friendly | declared | the shouts are 6" around the Hero's starting position; one merged ring with Stand Fast (same radius and kind: a ring per radius and kind, the labels joined) | on |

**Paths and rules (BC `keywords`, path progression)**

| Source | Radius | Kind | Affects | When | Notes | D |
|---|---|---|---|---|---|---|
| Voice of Command (Warlord) | 12" | command | friendly (Heroes too) | always | replaces the 6" Stand Fast ring of that Hero: the 6" command ring is not drawn for the bearer | on |
| Furious Tirade (Warlord) | 6" | enemy | enemy Warriors | always | they cannot use the Hero's Stand Fast | on |
| Heroic Resolve (Warlord, start) | 6" | command | friendly | declared | extra Resist die; merged into the Hero's 6" command ring | on |
| Claim the Throne (Claimant) | 6" | banner | friendly | conditional | the Hero counts as a banner after the enemy Leader falls within 6" | on |
| Heroic Move Mastery (Tactician) | 6" | support | friendly Heroes | always | roll-off bonus for Heroic Moves | on |
| Reading the Portents (Tactician) | 6" | support | friendly | declared | spend Will, re-roll one Duel die | on |
| Knife Mastery (Sharpshooter) | 3" | support | friendly models with a bow or crossbow | always | line of sight; re-roll natural 1 To Wound; Shielding without shield | on |
| "Hold positions, loose the arrows!" (Marksman) | 3" | support | friendly | always | line of sight; re-roll failed To Hit if they did not Move | on |
| Take Aim (Marksman heroic action) | 6" | support | friendly | declared | merged into the Hero's 6" command ring | on |
| A Good Sort (Soldier) | 3" | support | self | always | the bearer gets +1 Attack near another friendly Hero | on |
| Relentless Commander (Soldier) | 6" | support | friendly | declared | only when the opponent was given Priority | on |
| The King's Protector (Shieldbearer) | 6" | support | friendly Hero | declared | a free Heroic Combat when the chosen Hero is engaged | on |
| Chief Advisor (Ranger) | 6" | support | friendly Heroes | always | a Might Point is free on a 6 | on |
| To Arms! (Berserker) | 3" | support | friendly | declared | once per game, +1 Strength | on |
| For Glory (Duelist) | 6" | support | friendly | declared | once per game, +1 Fight, re-roll 1s To Wound | on |
| Remember Today | 3" | support | friendly Heroes | always | may use another friendly Hero's Might, Will and Fate | on |
| Lord of Lossarnach / Chief of Lamedon / Lord of the Vale | 3" | support | friendly Lossarnach / Lamedon / any (shooters) | always | re-roll 1s To Wound / auto-pass Courage / re-roll 1s To Hit | on |
| Moneybags | 6" | support | friendly Lake-town | declared | +1 Fight Value, counts the Hero as a banner | on |
| Callous Leadership | 3" | support | friendly Goblin | declared | +1 To Wound, a risk of a wound | on |
| Seize Command | 3" | support | the Leader | declared | line of sight; the Leader may be removed to give leadership | on |
| You Have My Sword | 6" | support | friendly Hobbit | declared | a free Heroic Combat to join a Hobbit's fight | on |
| Keen Sight (Falcon, a creature figure; Crebain stand-in) | 12" | enemy | enemy | always | enemy models within 12" of a Falcon apart from its Hero lose Stalk Unseen | on |
| Master of Battle (X) (vanilla units that have it) | 6" | enemy | enemy Heroes | declared | the bearer may copy an enemy Hero's Heroic Action | on |

**Magical Powers in effect (BC `magicalPowers`; the unit's known powers)**

| Source | Radius | Kind | Affects | When | D |
|---|---|---|---|---|---|
| Aura of Command | 6" | command | friendly (caster included) | cast | off |
| Fury (X) variants | 6" | support | friendly with the keywords in brackets | cast | off |
| Aura of Dismay, Fog of Disarray, Instill Fear | 6" | enemy | enemy | cast | off |
| Blinding Light | 6" | utility | any (daylight, shooting at models inside hit only on 6) | cast | off |
| Writhing Vines | 3" around the marker | utility | enemy | cast | off (the marker is placed within 3" of the caster; the ring shows where) |

### 2.5 Classified list: not rings (decided) and ambiguous (default shown)

| Source | Radius | Why not, and the default |
|---|---|---|
| Weapon ranges (bows 12/18/24", throwing 6", Whip 2", Spider Webs 8", blowpipe 12") | many | a weapon range, not a rule radius. Possible later feature (shooting range ring); out of scope here |
| Control Zone 1" | 1" | every model, the mod has its own 1" circle |
| Terror, Bodyguard, Command Support, Fearless, Dominant | none | no number (Command Support has no effect in play) |
| Pillager | 3" of an Objective | centred on the objective |
| Rapid Fire | 3" of the target | centred on the target |
| Receiver-side rules (13, section 2.1) | 3" to 12" | the source is another model; default none, follow-up for "Leader as General" |
| Writhing Vines marker, Rallying Point (scenario), Detonation tables, Siege deployment | n/a | centred on markers or terrain |
| Control, Reinforcement (3" of the entry point), Sentries, Scatter | n/a | scenario or table mechanics |
| **Ambiguous: targeting ranges (`when: target`)** | | the radius only limits the choice of one model; a ring is informative but noisy. **Default off**, group "Targeting ranges": Gatekeeper 6" (enemy Infantry, line of sight), Spy 12" (chosen Hero, line of sight), Heroic Challenge 6" (enemy Hero of the same tier or higher), Swap with Me! 3" (friendly Goblin on a 25 mm base), A fell light is in them 12" (line of sight), Handler's Glove 3" (release a creature) |
| **Ambiguous: Stand Fast on every Hero** | 6" | owner named Stand Fast; a ring on every Hero is clutter in a company with six Heroes. **Default on**, switchable as the type `command` |
| **Ambiguous: war horn 24"** | 24" | correct but fills the table. **Default off** under "Large rings (more than 12")" |
| **Ambiguous: bearer is mounted / on foot** | same | no auras depend on the mount; both versions of a rider (mounted and the on-foot spare) get the same rings |

### 2.6 Kinds (colour groups)

Five kinds, each with an id, a label, a colour (palette in 4.2), a legend sentence and a default:

| Kind | Meaning | Examples |
|---|---|---|
| `banner` | banner-like re-roll in Fights | Banner, Company Standard, Claim the Throne |
| `command` | courage, Stand Fast and the Heroic shouts | Stand Fast 6", Voice of Command, war horn, drum, Aura of Command |
| `support` | bonuses to friendly models | the Hero abilities of the paths, Instrument |
| `enemy` | penalties, denial or reaction against the enemy | Furious Tirade, Keen Sight, Aura of Dismay |
| `utility` | light, markers, other | Torch, Blinding Light, Writhing Vines |

Rings of the same kind and radius on one figure merge into one ring with the labels joined ("Stand Fast, With Me!, Take Aim"), so the table stays short.

## 3. How to draw it in Tabletop Simulator

### 3.1 The API (TTS docs, fetched 2026-10-27; the owner's installed mods)

- `Object.setVectorLines(parameters)`: a list of lines; each line has `points` (a list of vectors in the **object's local space**), `color` (rgb or rgba,
  default white), `thickness` (Unity units, default 0.1), `rotation` (default 0). `Global.setVectorLines` takes world positions. `getVectorLines()` returns the current lines.
  The call **replaces every line of the object**; there is no add.
- A line is a child of the object: it moves, turns and lifts with it with no script running. Points scale with the object, so a figure scaled 1.5 or 1.6
  (62 of the 1764 bag figures; most are 1.0) needs `points / scale` and `thickness / scale`. The FTC mod's own firing-arc script (Windlance) divides by
  `getScale().x` for the same reason.
- Events a script can use (Global and object scripts): `onObjectPickUp`, `onObjectDrop`, `onObjectDestroy`, `onObjectSpawn`, `onObjectStateChange(object, old_state_guid)`.
- Alternatives considered and rejected: **decals** (flat textures, a fixed image per radius, no thickness or colour variation, heavier to ship); **custom tiles** or
  child objects (an object per ring, must be moved with the figure: collisions and physics); **XML UI** (screen or world panels, not for circles on the table);
  `Global.setVectorLines` with a tracking loop (needs `onUpdate` or a timer per figure, drifts, lags).
- Limits and cost: a ring of 48 to 64 segments is 49 to 65 points; the owner's AoS tool draws up to 40 models with rings every 0.1 s without trouble; the FTC mod puts
  one such circle on each of 1605 bag figures. Drawing is a one-off cost per press of a button. The helper caps rings (section 4.2) and draws in chunks of 10 figures per frame.

### 3.2 What the owner's machine shows (all read-only)

Contrary to the first research of [`tabletop-simulator.md`](tabletop-simulator.md), the Workshop mods **are on disk** now: `~/Library/Tabletop Simulator/Mods/Workshop/`
(103 json files, `WorkshopFileInfos.json`; 25 October 2026). Relevant: 3133220714 "MESBG FTC - Version 0.4", 3272401079 "Age of Sigmar 4th Edition Table", 3272457903 "Malkavichs Model Updates (Age of Sigmar 4.0)",
3477721044 "Scott's MESBG Items". Saved games (`Saves/TS_Save_*.json`, AutoSave) contain no saved vector lines at all (the circles are off by default).

1. **FTC mod, the circle on each figure** (script on 1605 of the bag's 1764 models, also objectives, traps, trolls and the dragon with variants): numpad 1 and 2
   step a ring through `{0,1,2,3,4,5,6,8,10,12,18,24}` inches, numpad 3 shows or hides it. The script comment says "inches measured **from the edge of the base**" and the code is
   `radius = distance + baseOffset` with `baseOffset = max(bounds.size.x, bounds.size.z) / 2` from `getBounds()`, no conversion factor. Colour `{0,0.8,0.4}`, 64 steps, thickness 0.05, local height
   0.1, `show = true` at start with distance 0 (nothing drawn). **It calls `self.setVectorLines({})` two seconds after load (`Wait.time(..., 2)`) and whenever the ring is hidden or stepped**:
   every aura we draw on the same figure is wiped by it. A second script (Windlance) draws firing arcs and its comment says "range in TTS units (inch)"; a "+1 to Wound bubble" there is 7 units.
2. **FTC spawner:** figures are laid out 1.1 apart (`x = (i mod 6) * 1.1`, rows 1.1 apart), which for 25 mm bases (0.98") is base contact plus a hair.
3. **AoS coherency tool** (3272401079, a locked block; its Aura panel gives buttons 3, 6, 9, 12 and 18, a custom radius, twelve colours): `MM_TO_INCH = 0.0393701`, base sizes
   in mm converted to world units, rings drawn **on the model itself** with `setVectorLines`, 64 steps, thickness 0.05, `radius + base semi-axis - half the line width` so the outer edge of the line lands on the range, points divided by the model's scale,
   height from `getBoundsNormalized()` (just above the base bottom), ring list kept on the model with `setTable("aos_auras", ...)` so it survives save and load, redrawn on demand, one ring per press. It watches pickups
   only for its coherency lines. **So the approach the owner has seen is discoverable on this machine, and it is the one chosen here.** (We write our own code; only the technique is shared.)

### 3.3 Scale conversion: tabletop inch to TTS unit

**1 inch = 1.0 world unit.** Evidence: (a) the FTC range circle adds `distance` to a half-size taken from `getBounds()` in world units with no factor, and is labelled inches; (b) the Windlance script says
"TTS units (inch)"; (c) the AoS tool converts millimetres to world units with `0.0393701` (mm to inch) and compares it to `getBoundsNormalized()`; (d) the FTC spawner spaces 25 mm bases 1.1 apart (25 mm = 0.98");
(e) the TTS ruler and grid are commonly described as inch based (not checked in the docs fetched here). **Confidence: high (about 95%).** Not yet seen in the game: the owner's smoke test 1 (section 8) turns the FTC 6" circle on a figure and our 6" ring on the same figure
and they must coincide. The mesh bounds are the base plus any overhang (weapons, wings): the FTC circle takes the same value, so our rings agree with the mod's own circle (a deliberate choice, section 9 Q7); an optional `b` per figure from the vanilla
`baseSize` removes the overhang.

## 4. Design

### 4.1 Data: `data.auras` in the Battle Companies json (curator)

Generated by a new `tools/bc/auras.py` from a curated table with a `reason` per entry; the validator checks it (section 5.1). Shape:

```json
"auras": {
  "text": "Fixed radii around the bearer ... (how to read it)",
  "kinds": [{"id": "banner", "label": "Banner", "text": "Re-rolls in a Fight near a banner", "default": true}, ...],
  "entries": [
    {"id": "banner-3", "source": {"type": "item", "class": "banner"}, "label": "Banner", "radius": 3, "kind": "banner", "affects": "friendly",
     "when": "always", "los": true, "keywords": [], "tip": "Friendly models in a Fight within 3\" may re-roll a Duel die.", "concept": "Banner"},
    {"id": "standfast-6", "source": {"type": "hero"}, "label": "Stand Fast", "radius": 6, "kind": "command", "affects": "friendly", "when": "always",
     "los": true, "tip": "...", "concept": "Stand Fast", "mergeLabels": ["With Me!", "At the Double", "Loose", "Take Aim"]},
    {"id": "voice-12", "source": {"type": "rule", "name": "Voice of Command"}, "label": "Voice of Command", "radius": 12, "kind": "command",
     "affects": "friendly", "when": "always", "replaces": "standfast-6", "tip": "...", "concept": "Voice of Command"}
  ],
  "excluded": [{"source": "Pillager", "radius": 3, "reason": "centred on an objective"}, {"source": "Gatekeeper", "radius": 6, "reason": "targeting", "ring": "target"}],
  "unreachable": ["Thranduil's rules ..."]
}
```

`source.type`: `item` (by `name` or `class`), `rule` (a special rule or path rule by name, case-insensitive, label match), `hero` (every Hero), `power` (a Magical Power), `creature`
(a creature figure), `heroicAction`. `replaces` removes another entry's ring for the same bearer. `merge` rules: same kind and radius per figure.

### 4.2 Payload: ArmyNames v2

`TTSNamesPayload.Version` becomes 2 **when the Auras switch is on and at least one ring is selected**; otherwise the payload stays v1, so an old helper keeps working. Added per figure:

```json
{"k":"warrior of rohan: banner","l":"Warrior of Rohan: Banner","u":12,"x":"","o":0,"n":"...","d":"...",
 "a":[{"r":3,"k":"banner","w":"a","n":"Banner"}],
 "b":0.49}
```

- `a`: the rings of that figure (omitted when none): `r` radius in inches (number, one decimal at most), `k` kind id, `w` when (`a` always, `d` declared, `x` conditional, `c` cast, `t` target), `n` labels joined (for the log and tooltip).
- `b`: optional base radius in inches of the figure (from the vanilla `baseSize`: 25mm gives 0.49); the helper otherwise measures `getBoundsNormalized().size` and uses `max(x, z) / 2`.
- Top level `"auras":{"kinds":[{"id":"banner","rgb":"F0E442"}, ...],"cap":4}` is optional; the helper has the same palette built in and uses the payload's only when present (so the app can retune colours without a new helper).
- Rings attach to the **rider figure** of a unit and to its on-foot version (`x` `""` and `foot`); a mount (`x` `mount`) gets none; a creature gets its own (Keen Sight on the Falcon figure); a fallen unit of a live list gets none.
- Caps, applied by core in this order and reported as `TTSNote` kind `aurasLeftOut`: at most **6 rings per figure** (kept in the order of kinds banner, command, support, enemy, utility, then smaller radius), at most **150 rings** in the payload; the payload size rule of the names block still holds (24 KB; the aura data is about 50 bytes per ring).
- The helper accepts `v` 1 and 2; v3 or higher says "older than the app". An old helper receiving v2 prints the existing "older than the app" message; the dialog says so next to the download link.

### 4.3 The helper: drawing, toggles, clean-up

All in `internal/core/ttshelper/helper.lua` (common subset of Lua 5.1/5.2; no `goto`, no integer division). Public behaviour:

- **Draw.** *Apply names* and *Re-apply* draw the rings of every figure they name (when rings are on). A ring is one line: 48 points (`steps = 48`, closed), colour of its kind, thickness `0.045` (so the helper can recognise its own lines: the FTC circle is 0.05 and
  `{0,0.8,0.4}`), local height `0.1 + 0.015 * kindIndex` (the FTC circle's own height plus a step per kind so rings do not flicker against each other), radius `r + baseRadius - 0.0225` (the outer edge of the line lands on the range), x and z divided by
  `getScale().x`, thickness divided by it too. Two kinds with the same radius on one figure are drawn 0.03 apart.
- **Merge, not replace.** `setVectorLines` replaces everything, so the helper first reads `getVectorLines()` (inside `pcall`), drops its own lines (thickness `0.045` and a colour of the palette), appends the new ones and sets the result. If `getVectorLines`
  fails or is missing, the helper sets only its own lines (and says once that the mod's circle may disappear).
- **Remember.** The helper keeps a table `guid -> {id, rings}` (saved in `onSave` as JSON with the hidden kinds and the on/off state). On `onLoad` it waits **3 seconds** (the figures' own scripts clear their lines after 2) and redraws.
- **Buttons** (block face, below the existing two): **Rings** (show/hide all, state kept), and one small coloured button per kind (show/hide that kind); each press redraws all remembered figures in chunks of 10 per frame. **Redraw rings** after a player used the mod's hot keys (numpad 1-3 on a figure wipes
  its lines: a press of any ring button restores ours). A message in the summary: `Rings drawn for 14 figures (31 rings).`
- **Re-apply** first removes the helper's lines from every remembered figure, then draws the new payload; **unit removal**: a figure that is destroyed or goes into a container loses its lines with it; `onObjectDestroy` drops it from the table.
  A figure that is not on the table any more (in a bag) is skipped on redraw.
- **State changes** (mounted and foot states of the mod's figures): `onObjectStateChange(newObj, oldGuid)` looks `oldGuid` up in the table, moves the entry to the new GUID and redraws after 3 s (the new state's own script wipes its lines at 2 s). The names are not carried over by
  state change (existing limitation, [`tabletop-simulator.md`](tabletop-simulator.md)); the rings are.
- **Cap and cost.** Hard limits in the helper: 6 rings per figure, 200 in total; more are ignored with a message.
- **Never** moves, clones or destroys objects; it only sets vector lines (and name, description, notes, tag as today).

### 4.4 Palette and switches

Colour-blind friendly: the Okabe-Ito set, not green (the mod's own circle is green) and not too close in lightness.

| Kind | RGB | Why |
|---|---|---|
| banner | `F0E442` yellow | high lightness |
| command | `E69F00` orange | distinct from yellow by hue and saturation |
| support | `56B4E9` sky blue | |
| enemy | `D55E00` vermillion | with a thicker line (0.06) so it reads without colour |
| utility | `CC79A7` reddish purple | |

A second cue that does not rely on colour: the enemy kind is thicker; every ring has a tooltip text in the app legend with the radius; the kind buttons on the helper carry a one-letter label (B, C, S, E, U). Dashed rings need one line per dash (24 per ring); a possible follow-up, not in v1.

Dialog switches (section 4.6) and defaults (`REVIEW`, see questions): kinds all on; when groups: always on, declared on, conditional on, cast **off**, targeting **off**; "Large rings (more than 12")" **off**.

### 4.5 Live match and the unit state

Rings follow `TTSList`'s lines. Fallen units (live second warband) get none. A unit's rings depend on its **current** equipment and path: a banner dropped in a Retreated unit is not drawn; a hero who loses a path rule through an injury (none today) loses the ring.
The names payload is generated at the press of **Copy names**, like today; a changed company needs a new copy and *Re-apply*.

### 4.6 App side

**TTS dialog, section "Auras"** (new, in the names block after the rule-text switch; mobile first, one column at 360 px):

- A switch **Draw aura rings** (default on when the company has at least one ring; off and the section collapsed otherwise, with the sentence "This company has no aura ranges.").
- Under it one row per **kind that exists in the company**: a colour swatch drawn as a ring icon (shape plus colour: the kind's ring is also a letter), the label, "N rings, radii 3" 6"" and a checkbox. Kinds with no ring are not shown.
- Three more checkboxes: **Rules that need an action** (declared and conditional; on), **Spells in effect** (off), **Large rings, over 12"** (off), **Targeting ranges** (off). Each says how many rings it adds or removes ("adds 3 rings").
- **Legend** (always visible under the rows, `<dl>`): kind, colour name, one sentence; plus "Rings are measured from the edge of the base".
- A line "N rings on M figures; 2 left out (limit 6 per figure)" from `TTSNote`s; the helper download link carries "Helper v2: download again to draw rings".
- All switches re-render the names block by htmx like the rule-text switch; the choices are remembered in `localStorage` (as the rule level is). No script is needed for the list itself.

**Unit cards** (My Company, other companies, live, previews, read only; not the compact company overview): a row **Auras** under the keyword chips with the merged auras of the unit as chips `aura-chip` styled like the rule chips (`has-tip`, `tabindex="0"`): text `Aura 6"`
(`Stand Fast +3` merged labels do not show; the tooltip lists them), a ring icon as the shape cue, `aria-label` "Aura, 6 inches: Stand Fast, With Me!, Take Aim; works for friendly models". Tooltip title = the first label, meta = `friendly · always · needs line of sight`, text = the entries' `tip`,
`data-concept` = the first entry's `concept` (opens the glossary popup like a rule chip). At most 3 chips and a `+N` that expands in place (the pattern of the keyword row). Kinds are not coloured on the card (the TTS legend carries the colours) except a 2 px accent in the kind colour on the chip's border. No row when the unit has no aura.
Fallen or Retreated units in the live view keep their chips (they show what the unit has).

**As built (differences from this text):** the query parameters are `auras`, `akinds` (the kinds that are on, absent = all), `adeclared`, `aconditional`, `acast`, `atarget`, `alarge`, `aoptional` (the group "optional" of the data, the Torch, was added by core: off by default); `TTSAuras` takes the export's `TTSOptions` too, so its counts agree with the figures. Declared and conditional are two switches. A group with no ring in the company has no switch, and the counts (kind rows, group switches such as Large rings) come before the limits (6 per figure, 150): a ring the limits cut is still counted and the switch is still there; "N left out" and the totals line speak for the payload. The Okabe-Ito colours are fixed CSS classes. `CompanyUnit.Auras` carries `UnitAuras` into the cards. The chips are hidden by default and shown by a per-browser setting, "Show auras on unit cards" (cookie `bcm-auras`, in the appearance settings; the Auras section of an expanded card follows it; the style guide always shows them). The chips show every aura of the unit and mark the groups that are off by default (muted, dashed, said in the tooltip). Details: [`frontend.md`](frontend.md).

**Interfaces** (names for the backend planner; declared in `internal/web` where used, implemented by core):

```go
// package core
type AuraKind string    // "banner", "command", "support", "enemy", "utility"
type AuraWhen string    // "always", "declared", "conditional", "cast", "target"
type Aura struct {
    ID        string    // data.auras entry id; merged auras carry the first entry's
    Kind      AuraKind
    Radius    float64   // inches, from the base edge; 0 < r <= 48
    Affects   string    // "friendly", "enemy", "any", "self"
    When      AuraWhen
    LOS       bool
    Label     string    // labels joined with ", "
    Rules     []string  // rule/item names that give it (for the tooltip)
    Tip       string
    Concept   string    // glossary term for the chip popup
}
type AuraSelection struct {                 // the dialog's switches; the zero value draws nothing
    Draw       bool
    Kinds      map[AuraKind]bool            // nil: all kinds
    Declared, Conditional, Cast, Target, Large bool
}
// The auras of one unit (rider, or creature), unfiltered, merged by kind and radius, sorted by kind then radius.
func (r *RuleSet) UnitAuras(u CompanyUnit) []Aura
// What the dialog shows for a company: per kind its rings and radii, the totals and the notes, for a selection.
func (r *RuleSet) CompanyAuraSummary(d CompanyDetail, sel AuraSelection) AuraSummary
type AuraSummary struct {
    Kinds  []AuraKindSummary                // only kinds that exist in the company
    Rings, Figures, LeftOut int
    Counts map[string]int                   // "declared", "conditional", "cast", "target", "large": rings each switch adds
    Notes  []TTSNote
}
type AuraKindSummary struct { Kind AuraKind; Label, Text, RGB string; Rings int; Radii []float64 }
// Extension of the existing TTSNamesOptions: the names payload carries the rings of the selection.
type TTSNamesOptions struct { Rules string; Auras AuraSelection }
```

Web side (extends the existing `ttsExporter`):

```go
TTSAuras(ctx context.Context, campaignID, companyID int64, live *core.TTSLive, sel core.AuraSelection) (core.AuraSummary, error)
```

Errors: the same as `TTSNames` (`core.ErrNotFound` for a company the viewer cannot see, the forbidden error of the company page). Pure over the loaded company and ruleset, no repository method, no migration. Query parameters of the existing route
`GET /campaigns/{id}/companies/{company}/tts`: `auras=1|0`, `akinds=banner,command,...` (omitted: all), `adeclared`, `aconditional`, `acast`, `atarget`, `alarge` (`1|0`).

### 4.7 Offline tests

- **Helper under gopher-lua** with the fake TTS (`internal/core/ttshelper/ttsfake`): extend `Obj` with `setVectorLines`/`getVectorLines` (a **recorder**: every call stored, in order, with lines copied), `getBoundsNormalized`, `getScale`, `getGUID`, and a fake `Wait.time`/`Wait.frames` that a test advances by hand, plus `onObjectStateChange`/`onObjectDestroy` dispatch.
  Cases: a ring has 49 points on radius `r + base - 0.0225` (computed from the figure's bounds); scale 1.5 divides points and thickness; two kinds on one figure get distinct colours and heights; a figure with the mod's own circle keeps it (merge) and a second *Re-apply* leaves exactly one set of ours;
  `getVectorLines` missing gives overwrite mode and one message; hide all, hide one kind, show again; state change moves the entry to the new GUID and redraws after the delay; the mod's own wipe at load (`setVectorLines({})` fired by the fake at 2 s) is followed by our redraw at 3 s; a destroyed figure leaves the table and no call touches it;
  caps (5th ring and 201st ring ignored with a message); an old payload (v1) draws nothing and prints nothing; v3 prints the "older" message; no function moves, clones or destroys an object (the fake still has none). The lint test of banned Lua forms covers the new code.
- **Glue**: `internal/core/ttsglue_test.go` already feeds real `TTSList`/`TTSNames` output of 37 companies through the helper: add the v2 payloads (all kinds on) and assert every ring lands on the right figure, none on a mount, none on a fallen unit, none above the caps.
- **Data**: `tools/test_auras.py` and the validator (5.1); a Go test `TestAuraRadiiInRuleText` that for every entry finds the radius as `N"` in the source rule or item text (so a curator's typo or a rules change fails); `TestUnitAurasAllCompanies` writes every start, reinforcement, special and advancement unit of the 37 companies and compares with a golden
  `testdata/auras_golden.txt` (`-update-auras`), like `tts_golden.txt`; unit tests for merge (same kind and radius), `replaces` (Voice of Command removes the 6" ring), mounts and creatures, the caps and the selection filters.
- **Web**: `internal/web/tts_test.go` (mock `ttsExporter`): the Auras section renders kinds, counts, legend, the switches change the query; the cards test for the chip row (`has-tip`, aria-label, `+N`); e2e flow `93-tts-auras` ([`testing.md`](testing.md)).

### 4.8 Effort and risks

Effort: definitions M (the table and its validator: the audit above is the input), core M (`UnitAuras`, payload v2, caps, golden), helper M to L (the drawing is small; merge, redraw, state change and tests are the work), web S + S (dialog section, card chips), qa S. About six small beads (below).

Risks and what contains them:

| Risk | Containment |
|---|---|
| The mod's per-figure script wipes vector lines (found) | merge on draw, redraw 3 s after load/state change, Redraw button, smoke test 2 |
| `getVectorLines` returns something we cannot filter (thickness precision) | overwrite fallback with a one-time message; smoke test 3 decides |
| Events `onObject*` not delivered to an object script | the AoS tool relies on them in a block script; if not, the Redraw button covers state changes (smoke test 4) |
| Mesh bounds larger than the base (overhang) | optional payload `b` from the vanilla base size; ring and mod's own circle agree anyway |
| Oval bases | circle on the larger half-side (rings clear the base); three ovals in vanilla, rare in BC |
| Clutter (six Heroes with 6" rings) | merge by kind and radius, caps, per-kind buttons, large rings off |
| The curated radii drift from the rules | validator and the rule-text radius test |
| Old helper with a v2 payload | version rule above; dialog note |
| Data wrongly says a rule is an aura (receiver side, targeting) | explicit `excluded` list with reasons; ambiguous entries default off |

## 5. Definitions and core contract (for the curator and the backend planner)

### 5.1 Validator (`tools/validate_definitions.py`, `tools/bc/auras.py`)

- Every entry: `kind` one of `kinds`, `radius` is a number in 0.5 to 48, `when` and `affects` in their sets, `source` resolves (the item, rule, power or path rule exists in the vanilla or the BC file), `concept` is a glossary or rule name, `replaces` points to an entry.
- **The radius appears as `N"` in the text of the source** (or in the glossary text of its `concept` for `hero`). A source with no number is an error.
- **Coverage**: every rule, item, path rule or power reachable by a Battle Companies unit whose text matches the range patterns is in `entries` or in `excluded` (with a reason); rules of characters no company can field are in `unreachable` (a list, so a new company that can field one fails the check). This is the same discipline as `ruleTargets`.
- No application text of the rulebooks is copied beyond the short `tip` (one sentence, own words).

### 5.2 Core semantics

- `UnitAuras(u)` is pure over the built `CompanyUnit` and `RuleSet`. Item sources match by item name or by class (`gearAnnotations`), on the unit's current equipment; rule and path sources match by name on `Rules` and `Path.Rules` (only the path rules unlocked so far: `PathRules`); the `hero` source matches units whose current profile has the Hero type; `power` on the unit's known powers; a creature figure by the creature's rules. Results are merged (same kind and radius, labels joined with `, `, `Rules` listing each) and sorted (kind order, radius).
- `replaces` is applied before merging. The result does not depend on mounted state or on the live state; the filters by `AuraSelection` are applied by `CompanyAuraSummary` and `TTSNames`, not by `UnitAuras`.
- `TTSNames` writes `a` only for the figures that carry a rider or creature of a unit with rings (section 4.2) and sets `Version = 2` when any ring is written.

## 6. Acceptance checklist (for the implementers; each bead copies its part)

- [ ] `data.auras` exists, generated by `tools/`, passes the validator (5.1), has every reachable range rule classified; the audit table of section 2.4 and 2.5 is reproduced by the data.
- [ ] `UnitAuras` of a Warrior of Rohan with a Banner is one ring 3" `banner`; of a Warlord Hero with Voice of Command: 12" `command` and no 6" `command`, plus 6" `enemy` (Furious Tirade); of a plain Hero: 6" `command` labelled "Stand Fast, With Me!, ..."; of a mount or a unit without items or rules: none.
- [ ] The golden over 37 companies is committed and reviewed; no unit has more than 4 merged rings in the default selection.
- [ ] The names payload is v2 only with rings; v1 payloads are byte-identical to today's; all payload tests and the size rule pass.
- [ ] The helper draws rings with the scale and base rules of 4.3, merges with other lines, redraws after load and state change, hides/shows by kind, cleans up on Re-apply, respects the caps; all the fake-TTS cases of 4.7 pass; the Lua lint test passes.
- [x] Dialog: Auras section with switches, counts, legend, no horizontal scroll at 360 px, keyboard operable, light and dark, in `/styleguide`; a company with no ring says so.
- [x] Cards: the Auras chip row on My Company, other companies, live and previews; tooltip text, aria-label, `+N`; not on the compact overview.
- [ ] `docs/tabletop-simulator.md` (payload v2, helper rings), `docs/frontend.md` (dialog, chips), `docs/definitions.md` (`data.auras`), `docs/testing.md` (flow) say "built" and describe the current state.
- [ ] The helper zip (`/tts/helper.zip`) has the new script and its `ETag` changed.

## 7. Beads

Epic `battle-company-manager-ce-v2-4fe` (label `auras`). Order: the owner smoke test first (it needs a hand-made file only), then data, core, helper in parallel with web.

| Bead | Label | Content |
|---|---|---|
| `.1` | `auras`, `tts`, `decision` | Owner smoke test 0: a small Saved Object / script to check scale, merge and wipe (section 8, tests 1 to 5) before the helper work |
| `.2` | `auras`, `definitions` | `data.auras` + `tools/bc/auras.py` + validator + tests (section 5.1) |
| `.3` | `auras`, `backend` | `core.UnitAuras`, `AuraSelection`, `CompanyAuraSummary`, golden over 37 companies (4.6, 5.2) |
| `.4` | `auras`, `backend` | `TTSNames` v2 (`a`, `b`, caps, notes, version rule) + glue test |
| `.5` | `auras`, `tts`, `frontend` | helper rings (4.3, 4.4) + fake TTS recorder + tests (4.7) + Saved Object rebuild |
| `.6` | `auras`, `frontend` | TTS dialog Auras section, query parameters, `ttsExporter.TTSAuras` |
| `.7` | `auras`, `frontend` | Aura chip row on the unit cards (all surfaces) |
| `.8` | `auras`, `qa` | e2e flow, docs updates, helper zip check, `make test`, `make test-integration`, validator |
| `.9` | `auras`, `backend`, `definitions` | optional: base radius `b` from vanilla `baseSize` (parse `25mm`, ovals, the typos `170mmm`); fall back to bounds |
| `.10` | `auras`, `tts`, `decision` | Owner smoke test of the finished feature (section 8, tests 6 to 10) |

## 8. Smoke tests for the owner (TTS)

1. **Scale.** On any spawned figure press numpad 3 (the mod's circle), numpad 1 until 6", and with the new helper draw the 6" command ring (or the test file of bead `.1`): the two rings must coincide to the line width. If ours is smaller or larger by a constant factor, tell us the factor (the conversion constant is one line in the helper).
2. **Wipe at load.** Draw rings, save the game, reload it: the rings must be back after about 3 seconds (the helper redraws). Without the helper redraw they would be gone.
3. **Merge.** With a figure that shows the mod's circle (numpad 3 on, 3") press Apply names: both the mod's circle and the aura rings show. Press numpad 3 again: the mod's circle goes (our rings may go too: press Redraw rings and they return). Report which.
4. **State change.** Switch a Rider of Rohan to its foot state (right-click, States): the rings return within 3 seconds on the new figure.
5. **Pickup, rotate, scale.** Lift a figure, turn it, drop it: the ring stays centred under it. Do the same with a big (scale 1.5) figure: the ring still measures 6" from its base edge.
6. **Kind buttons.** Hide and show `banner`, `command`: only those rings go. Hide all and show all.
7. **Many figures.** Spawn 24 figures with banners and Heroes: pressing Apply names does not freeze the game (draw in chunks).
8. **Colours.** Check the five colours on your table cloth in the mod's lighting; tell us if two are hard to tell apart (the palette is one table in the app).
9. **Re-apply** after changing the company (buy a banner): old rings vanish, new ones appear; a figure you removed from the table has no ring left behind.
10. **Old helper** with a new payload: the message asks for a new download; with the new helper and an old (v1) payload nothing is drawn and nothing breaks.

## 9. Questions for the owner (with the default taken)

| # | Question | Default (`REVIEW`) |
|---|---|---|
| Q1 | A 6" Stand Fast ring on every Hero: wanted? | Yes, kind `command`, one switch to hide |
| Q2 | Which groups are on in the dialog? | always, declared and conditional on; spells and targeting ranges off |
| Q3 | War horn 24" and other rings over 12" | off ("Large rings") |
| Q4 | Receiver-side rules (a unit near its named hero) | no ring; a ring on the company's Leader for "General" rules as a later bead |
| Q5 | Targeting ranges (Gatekeeper 6", Swap with Me! 3"...) | off, switchable |
| Q6 | Dashed rings for the enemy kind? | no: a thicker solid line; dashes only if the colours prove hard to read |
| Q7 | Ring measured from the mesh bounds (as the FTC circle) or from the vanilla base size? | bounds (agrees with the mod's circle), `b` from the data as a later bead |
| Q8 | Auras switch on by default (it makes the old helper ask for a new download)? | on |
| Q9 | Palette (section 4.4) | Okabe-Ito |
| Q10 | The on-foot spare of a rider gets the same rings | yes |
| Q11 | A ring-per-weapon-range feature (bow 24") later? | not now |
