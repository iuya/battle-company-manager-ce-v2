# Aura ranges

How the ranges of auras (a banner, Stand Fast, "within 3" of this model" rules) are classified, drawn as rings around the miniatures in Tabletop Simulator (TTS) and shown as chips on the unit
cards. This page is the contract between the definitions (the `auras` of rules, items and powers), core (`UnitAuras`, `TTSNames` v2), the Lua helper and the web layer. Related:
[tabletop-simulator.md](tabletop-simulator.md) (list, names payload, helper), [unit-keywords.md](unit-keywords.md) (chips, rule targets), [definitions.md](definitions.md) (rules data),
[frontend.md](frontend.md#copy-as-a-tabletop-simulator-army-list) (dialog and chips).

Status: built end to end (data, core, payload, helper, dialog, card chips, tests). What is not verified is the drawing in the game itself; see [Smoke tests](#smoke-tests-in-tabletop-simulator).

## Summary

- An **aura** is a fixed radius, in inches from the edge of the bearer's base, inside which models are affected or eligible by a rule or an item the bearer has (a banner, a Hero's Stand Fast, "friendly
  models within 3" of this Hero ..."). Not an aura: a weapon's range, "choose a model in the same combat", a radius around somebody else (the rule's receiver side), around an objective or a target.
- **Scale: 1 inch = 1.0 TTS world unit** (high confidence). A ring is a TTS vector line (`setVectorLines`) in the figure's own space, so it moves, turns and is picked up with the figure with no tracking code.
  This is how the Age of Sigmar workshop table draws its aura rings and how the FTC mod's own range circle works.
- The data is a curated table of `auras` on the rules, items and powers that draw them (curator-owned); core turns it into `[]Aura` per unit (`RuleSet.UnitAuras`); the same list feeds three consumers:
  the unit cards (a chip "Aura 6"" with tooltip), the **Auras** section of the TTS dialog (kinds, legend, switches) and the names payload (**version 2**, field `a` per figure), which the helper draws.
- One hazard: the FTC mod's own script on every figure **wipes all vector lines of the figure** (it calls `setVectorLines({})` two seconds after load, and on its hot keys). The helper therefore
  (re)draws after that, merges with the lines that exist and offers a Redraw button.

## What is an aura in this app's data

Most special rules that mention a range belong to **named characters** (Gandalf, Thranduil...) that no Battle Company unit has; they are out of scope until a company can field the character, and the
validator lists them as `unreachable`, not as errors. Thirteen reachable vanilla rules are **receiver-side**: the unit gets a bonus "whilst within X" of a named hero, a General or a Hero of its faction
(King's Guard, Riders of Eorl, Arise, Riders of Rohan!, Pride of Dol Amroth, Loyal to the Captain, Warrior of the Dragon Cult, Favour of the War Chief, Protectors of the White Tree, Everyone has a Price,
Onwards Men of Esgaroth, The Honour of Lamedon, King's Knights). The ring would have to be on the source, which is not the bearer (and may not exist in a company), so they draw no ring. The Hero
abilities of the **paths** (the main source of BC auras) are copied as text in the path progression and as keywords. There is no machine-readable radius anywhere in the rulebooks, so the table is curated
(like the rule targets) with a validator that keeps it honest.

### The criterion

A rule or item gets a **ring** when all of these hold:

1. the radius is a number of inches written in the rule text, measured from the bearer (rulebook "Measuring": between the closest parts of the bases, so the ring is drawn at the radius **plus the bearer's base radius**);
2. the radius is centred on the bearer (not on a target, an objective, a marker, the table edge, or another model);
3. it decides which models are affected or eligible (everything in the area, or the candidates for a trigger).

Not rings: line of sight and engagement ("in the same combat", "Supporting"), base contact, weapon ranges, "up to its Move Value", Control Zone (1", every model), and anything with no number. **Line of sight**
does not stop a ring: the ring shows the radius and the tooltip says "needs line of sight". **Bearer state** (alive, standing) does not change the radius: a figure removed from the table takes its ring with it, and the live
list's fallen units get no ring.

### Fields of an aura

| Field | Meaning |
|---|---|
| `radius` | inches, from the base edge |
| `kind` | the colour group: `banner`, `command`, `support`, `enemy`, `utility` |
| `affects` | `friendly`, `enemy`, `any`, or `self` (the bearer benefits when others are near, A Good Sort) |
| `keywords` | a keyword filter of the affected models (Lossarnach, Lamedon, Goblin, bow models, Warrior only...), shown in the tooltip, not drawn |
| `when` | `always` (passive, or a rule of every Hero), `declared` (an Active rule, a Heroic Action, a once-per-game rule: needs a Will/Might Point or a declaration), `conditional` (only after a game event: Claim the Throne, a horn used), `cast` (a Magical Power in effect), `target` (the radius only limits the choice of one model) |
| `los` | the rule needs line of sight (tooltip) |
| `bearer` | state or keyword conditions of the bearer (`Hero`...): only decides who gets the entry |
| `replaces` | removes another entry's ring for the same bearer (Voice of Command replaces the 6" Stand Fast ring) |

### Rings

"Source" is the card text the player knows. `D` is the default in the dialog.

**Items (the unit carries the item; class from the item annotations)**

| Source | Radius | Kind | Affects | When | Notes | D |
|---|---|---|---|---|---|---|
| Banner (any vanilla option that gives a banner, class `banner`) | 3" | banner | friendly | always | Engaged or Supporting friendly model with line of sight; the bearer must be standing and has -1 Duel Roll; a banner limited to keywords works for those models only | on |
| Company Standard (a Pennant, class `banner`) | 3" | banner | friendly | always | also re-roll of 1s in Duel Rolls within 3"; one ring when both | on |
| Company Instrument | 3" | support | friendly | always | models that suffer a wound roll a D6, ignore on a 6 | on |
| Company Drum, War drum | 12" | command | friendly | declared | free Heroic March, "At the Double" range 12" | on |
| Company Horn, War horn (class `instrument`) | 24" | command | friendly | always | +1 to Courage Tests of models within 24" | **off** (large) |
| War horn: Stand Fast extension | 9" (6"+3") | command | friendly | declared | only for a Hero or a model within the Hero's Stand Fast; once per game | on |
| Torching Brand (Torch) | 6" | utility | any | always | lit area, daylight; terror for the bearer | off |

**Every Hero (rulebook rules, not in the unit's rule list)**

| Source | Radius | Kind | Affects | When | Notes | D |
|---|---|---|---|---|---|---|
| Stand Fast | 6" | command | friendly Warriors | always | broken army only, line of sight, the Hero must have passed its own test | on |
| Heroic Move "With Me!", Heroic March "At the Double", "Loose", Heroic Accuracy "Take Aim", Heroic Resolve | 6" | command | friendly | declared | merged into one ring with Stand Fast (same radius and kind: a ring per radius and kind, the labels joined) | on |

**Paths and rules**

| Source | Radius | Kind | Affects | When | Notes | D |
|---|---|---|---|---|---|---|
| Voice of Command (Warlord) | 12" | command | friendly (Heroes too) | always | replaces the 6" Stand Fast ring of that Hero | on |
| Furious Tirade (Warlord) | 6" | enemy | enemy Warriors | always | they cannot use the Hero's Stand Fast | on |
| Heroic Resolve (Warlord, start) | 6" | command | friendly | declared | merged into the Hero's 6" command ring | on |
| Claim the Throne (Claimant) | 6" | banner | friendly | conditional | the Hero counts as a banner after the enemy Leader falls within 6" | on |
| Heroic Move Mastery (Tactician) | 6" | support | friendly Heroes | always | roll-off bonus for Heroic Moves | on |
| Reading the Portents (Tactician) | 6" | support | friendly | declared | spend Will, re-roll one Duel die | on |
| Knife Mastery (Sharpshooter) | 3" | support | friendly models with a bow or crossbow | always | line of sight; re-roll natural 1 To Wound | on |
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
| Master of Battle (X) | 6" | enemy | enemy Heroes | declared | the bearer may copy an enemy Hero's Heroic Action | on |

**Magical Powers in effect (the unit's known powers)**

| Source | Radius | Kind | Affects | When | D |
|---|---|---|---|---|---|
| Aura of Command | 6" | command | friendly (caster included) | cast | off |
| Fury (X) variants | 6" | support | friendly with the keywords in brackets | cast | off |
| Aura of Dismay, Fog of Disarray, Instill Fear | 6" | enemy | enemy | cast | off |
| Blinding Light | 6" | utility | any (daylight, shooting at models inside hit only on 6) | cast | off |
| Writhing Vines | 3" around the marker | utility | enemy | cast | off (the marker is placed within 3" of the caster; the ring shows where) |

### Not rings, and ambiguous

| Source | Radius | Why not, and the default |
|---|---|---|
| Weapon ranges (bows 12/18/24", throwing 6", Whip 2", Spider Webs 8", blowpipe 12") | many | a weapon range, not a rule radius |
| Control Zone 1" | 1" | every model; the mod has its own 1" circle |
| Terror, Bodyguard, Command Support, Fearless, Dominant | none | no number |
| Pillager | 3" of an Objective | centred on the objective |
| Rapid Fire | 3" of the target | centred on the target |
| Receiver-side rules | 3" to 12" | the source is another model |
| Writhing Vines marker, Rallying Point (scenario), Detonation tables, Siege deployment, Control, Reinforcement, Sentries, Scatter | n/a | centred on markers or terrain, or scenario and table mechanics |
| **Targeting ranges (`when: target`)** | | the radius only limits the choice of one model; a ring is informative but noisy. **Default off**, group "Targeting ranges": Gatekeeper 6", Spy 12", Heroic Challenge 6", Swap with Me! 3", A fell light is in them 12", Handler's Glove 3" |
| **Stand Fast on every Hero** | 6" | a ring on every Hero is clutter in a company with six Heroes. Default on, switchable as the kind `command` |
| **War horn 24"** | 24" | correct but fills the table. Default off under "Large rings (more than 12")" |
| **Mounted or on foot** | same | no aura depends on the mount; both versions of a rider get the same rings |

### Kinds (colour groups)

| Kind | Meaning | Examples |
|---|---|---|
| `banner` | banner-like re-roll in Fights | Banner, Company Standard, Claim the Throne |
| `command` | courage, Stand Fast and the Heroic shouts | Stand Fast 6", Voice of Command, war horn, drum, Aura of Command |
| `support` | bonuses to friendly models | the Hero abilities of the paths, Instrument |
| `enemy` | penalties, denial or reaction against the enemy | Furious Tirade, Keen Sight, Aura of Dismay |
| `utility` | light, markers, other | Torch, Blinding Light, Writhing Vines |

Rings of the same kind and radius on one figure merge into one ring with the labels joined ("Stand Fast, With Me!, Take Aim"), so the table stays short.

## Drawing it in Tabletop Simulator

### The API

- `Object.setVectorLines(parameters)`: a list of lines, each with `points` (vectors in the **object's local space**), `color` (default white), `thickness` (Unity units, default 0.1) and `rotation`.
  `Global.setVectorLines` takes world positions. `getVectorLines()` returns the current lines. The call **replaces every line of the object**; there is no add.
- A line is a child of the object: it moves, turns and lifts with it with no script running. Points scale with the object, so a figure scaled 1.5 needs `points / scale` and `thickness / scale` (the FTC mod's own firing-arc script divides by `getScale().x` for the same reason).
- Events a script can use: `onObjectPickUp`, `onObjectDrop`, `onObjectDestroy`, `onObjectSpawn`, `onObjectStateChange(object, old_state_guid)`.
- Rejected alternatives: **decals** (flat textures, no thickness or colour variation); **custom tiles** or child objects (must be moved with the figure: collisions and physics); **XML UI** (not for circles on the table); `Global.setVectorLines` with a tracking loop (needs a timer per figure, drifts, lags).
- Cost: a ring of 48 segments is 49 points; the FTC mod puts one such circle on each of its 1605 bag figures. Drawing is a one-off cost per press of a button; the helper draws in chunks of 10 figures per frame.

### What the installed mods show

1. **FTC mod, the circle on each figure** (script on most bag models): numpad 1 and 2 step a ring through `{0,1,2,3,4,5,6,8,10,12,18,24}` inches, numpad 3 shows or hides it. Its comment says "inches measured
   **from the edge of the base**" and the code is `radius = distance + baseOffset` with `baseOffset = max(bounds.size.x, bounds.size.z) / 2` from `getBounds()`, no conversion factor. Colour `{0,0.8,0.4}`, 64 steps, thickness
   0.05, local height 0.1. **It calls `self.setVectorLines({})` two seconds after load and whenever the ring is hidden or stepped**: every aura drawn on the same figure is wiped by it.
2. **FTC spawner:** figures are laid out 1.1 apart, which for 25 mm bases (0.98") is base contact plus a hair.
3. **AoS coherency tool** (a locked block with an Aura panel): converts base sizes in mm to world units with `0.0393701`, draws rings on the model itself with `setVectorLines` at `radius + base semi-axis - half the
   line width`, points divided by the model's scale, ring list kept on the model so it survives save and load. Only the technique is shared; the code here is our own.

### Scale

**1 inch = 1.0 world unit.** Evidence: the FTC range circle adds `distance` to a half-size taken from `getBounds()` with no factor and is labelled inches; its firing-arc script says "TTS units (inch)"; the AoS tool
converts millimetres to world units with the mm-to-inch constant; the FTC spawner spaces 25 mm bases 1.1 apart. Confidence is high (about 95%) but not yet seen in the game (smoke test 1). The mesh bounds are the base plus
any overhang (weapons, wings): the FTC circle takes the same value, so our rings agree with the mod's own circle. The app sends no base size; the helper measures the bounds itself.

## Design

### Data

Curated `auras` on the rules, items and magic powers that draw them (several where a thing draws several rings: Enchanting Song, War Horn) in the lean Battle Companies file, plus the top-level `auras {kinds, groups}`
vocabulary tables; the schema and the curator checks are described in [definitions.md](definitions.md). A ring has `radius`, `kind`, `affects`, `when`, `los`, `keywords`, `bearer`, `replaces`, a one-sentence `tip` and the
glossary `concept` its chip opens. The working list of what was considered and left out (and why) is `tools/data/aura_audit.json`. Checks: every named source exists, every radius appears as `N"` in the text of its source,
kinds and group defaults are exactly the vocabulary, ids are unique, and every rule, item, path rule or power reachable by a Battle Companies unit whose text matches a range pattern is either a ring, listed as
excluded with a reason, or listed as `unreachable` (so a new company that can field a character fails the check). Tests: `TestAuraRadiiInRuleText` and the golden `testdata/auras_golden.txt` (every unit the 37 companies
can have, a hero on each path with every rule unlocked, and each company hero upgrade; `go test ./internal/core -run UnitAurasAll -update-auras` rewrites it).

### Core

```go
type AuraKind string // banner | command | support | enemy | utility
type AuraWhen string // always | declared | conditional | cast | target
type Aura struct {
    ID, Label, Tip, Concept string
    Kind    AuraKind
    Radius  float64  // inches from the base edge, 0 < r <= 48
    Affects string   // friendly | enemy | any | self
    When    AuraWhen
    LOS     bool
    Rules   []string // rule or item names that give it, for the tooltip
}
type AuraSelection struct { // the dialog's switches; the zero value draws nothing
    Draw       bool
    Kinds      map[AuraKind]bool // nil: all kinds
    Declared, Conditional, Cast, Target, Large, Optional bool
}
func (r *RuleSet) UnitAuras(u CompanyUnit) []Aura          // unfiltered, merged by kind and radius, sorted by kind then radius
func (r *RuleSet) CreatureAuras(c CompanyCreature) []Aura
func (r *RuleSet) CompanyAuraSummary(d CompanyDetail, sel AuraSelection) AuraSummary
```

`UnitAuras` is pure over the built `CompanyUnit` and `RuleSet`: item entries match by item name, alias or class on the unit's current equipment; rule and path entries match by name on `Rules` and the unlocked `Path.Rules`
(never a mount's rule); the `hero` source matches units whose current profile has the Hero type; powers match the unit's known powers; a creature figure matches by the creature's rules. `replaces` is applied before
merging and the result does not depend on mounted state or live state. `AuraSelection` filters by kind and group **before** the merge (`DefaultAuraSelection()` reads the data defaults). `CompanyUnit.Auras` carries
the unfiltered list to the cards.

`AuraSummary{Kinds, Rings, Figures, LeftOut, Counts, Notes}` is what the dialog shows: per kind that exists in the company its rings and radii (`AuraKindSummary{Kind, Label, Text, RGB, Rings, Radii}`), the totals, and in `Counts`
how many rings each switch (declared, conditional, cast, target, large, optional) adds (or removes when it is on). Counts come before the limits: a ring the limits cut is still counted and its switch stays.

### The names payload, version 2

`TTSNamesPayload.Version` is 2 only when at least one ring is selected and written; otherwise the payload is the v1 shape byte for byte, so an old helper keeps working. Per figure:

```json
{"k":"warrior of rohan: banner","l":"Warrior of Rohan: Banner","u":12,"x":"","o":0,"n":"...","d":"...",
 "a":[{"r":3,"k":"banner","w":"a","n":"Banner"}]}
```

`a` lists the rings (omitted when none): `r` radius in inches, `k` kind id, `w` when (`a` always, `d` declared, `x` conditional, `c` cast, `t` target), `n` labels joined. The top-level `auras` object (palette, cap) is
optional and not written by the app; the helper has the same palette built in. Rings attach to the **rider figure** of a unit and its on-foot version, to a creature's own figure, never to a mount or a fallen unit, and
for a bundle (Iron Shield Bearer + Foe Spear Wielder) only the first figure. Core applies these limits in order: at most **6 rings per figure** (kinds in the data's order, then the smaller radius) and **150** in the payload
(the helper's own hard limits are 6 and 200), then the 24 KB rule (rule texts lowered first; if still too big the rings are given up and the payload falls back to v1). The note kind is `aurasLeftOut`. The helper accepts `v`
1 and 2; any other value prints "This helper is older than the app (names format N). Download the helper again from the app."

### The helper

`internal/core/ttshelper/helper.lua` (Lua 5.1/5.2 common subset). Behaviour:

- **Draw.** *Apply names* and *Re-apply* draw the rings of every figure they name (when rings are on). A ring is one closed line of 48 segments, colour of its kind, thickness `0.045` (enemy `0.06`; the FTC circle is 0.05, so the helper
  can recognise its own lines), local height `0.1 + 0.015 * kindIndex`, radius `r + baseRadius - 0.0225` (the outer edge of the line lands on the range), x and z divided by `getScale().x`, thickness divided too. Two rings of the same
  radius on one figure are drawn 0.03 apart.
- **Merge, not replace.** The helper reads `getVectorLines()` (inside `pcall`), drops its own lines (a kind's colour and thickness), appends the new ones and sets the result. If the lines cannot be read it sets only its own and says
  once that the mod's circle may disappear.
- **Remember.** A table `guid -> rings` is saved in `onSave` with the on/off state, the hidden kinds and the payload palette. `onLoad` redraws after **3 seconds** (the figures' own scripts clear their lines after 2).
  `onObjectStateChange(newObj, oldGuid)` moves the entry to the new GUID and redraws after 3 s; `onObjectDestroy` drops it; figures that are not on the table (in a bag) are skipped.
- **Buttons** (block face): **Rings: on/off**, **Redraw** (a press restores our lines after the mod's hot keys wiped them) and one small coloured button per kind (B C S E U; the letter is in brackets while the kind is hidden). Each
  press redraws the remembered figures, 10 per frame. Summary: `Rings drawn for 14 figures (31 rings).` and `N rings left out (limit 6 per figure, 200 in all).`
- **Never** moves, clones, rescales or destroys objects; it only sets vector lines (and name, description, notes and tag as in [tabletop-simulator.md](tabletop-simulator.md)).

**Palette** (Okabe-Ito, colour-blind friendly, not green because the mod's circle is green): banner `F0E442` yellow, command `E69F00` orange, support `56B4E9` sky blue, enemy `D55E00` vermillion (with the thicker line), utility
`CC79A7` reddish purple. Colour is never the only cue: the enemy kind is thicker, the app legend names every kind and radius, and the helper's kind buttons carry a one-letter label. Dashed rings would need one line per
dash and are not drawn.

### App side

**TTS dialog, "Auras" section** (in the names block, mobile first): a switch **Draw aura rings** (default on when the company has a ring; with none the section says "This company has no aura ranges."); one row per **kind that
exists in the company** (a ring icon in the kind's colour with its letter, the label, "N rings, radii 3", 6"" and a checkbox); switches for the **groups** that have at least one ring in the company: Rules that need an action
(declared, on), Rules that need an event (conditional, on), Spells in effect (cast, off), Targeting ranges (target, off), Large rings over 12" (off) and Optional rings (the Torch, off), each saying how many rings it adds;
a **legend** (kind, colour name, one sentence, "Rings are measured from the edge of the base"); and a line "16 rings on 7 figures, 1 left out (limit 6 per figure, 150 in all)". The switches re-render the names block by htmx and are
remembered in `localStorage`. Query parameters of `GET /campaigns/{id}/companies/{company}/tts`: `auras=1|0`, `akinds=banner,command,...` (the kinds that are on; absent: all), `adeclared`, `aconditional`, `acast`, `atarget`,
`alarge`, `aoptional` (`1|0`). The web interface method is `ttsExporter.TTSAuras(ctx, campaignID, companyID, live, TTSOptions, AuraSelection)`.

**Unit cards** (My Company, another company, the live match; not the compact company overview, the fallen cards, the armoury or the company browser): an **Auras** row under the special rules with the merged auras of the unit
as chips (`aura-chip`, `has-tip`, `tabindex="0"`): text `Aura 6"`, a ring icon as the shape cue and `aria-label` "Aura, 6 inches: Stand Fast, With Me!, Take Aim; works for friendly models, always, needs line of sight". Tooltip title is the
merged labels, meta `friendly · always · needs line of sight`, text the entries' `tip`; `data-concept` opens the glossary popup. At most 3 chips and a `+N` that expands in place. The card shows every aura the unit has,
and marks the groups that are off by default in the dialog (muted and dashed, with "off by default in Tabletop Simulator" in the tooltip). The chips are hidden by default and shown by a per-browser setting, **Show auras on unit
cards** (cookie `bcm-auras`, in the appearance settings; the Auras section of an expanded card follows it). The ring colours are fixed CSS classes (`.aura-k-banner` ...), not theme tokens. `/styleguide` shows them
("Aura rings and chips").

### Tests

- **Helper** under gopher-lua with the fake TTS (`internal/core/ttshelper/ttsfake`, with a recorder for `setVectorLines`/`getVectorLines`, `getBoundsNormalized`, `getScale`, `getGUID`, a virtual clock for `Wait.time`/`Wait.frames`, and `onObjectStateChange`/`onObjectDestroy`
  dispatch): geometry (49 points on `r + base - 0.0225`), scale 1.5, distinct colours and heights, merge with the mod's circle, one set of ours after Re-apply, overwrite mode, hide and show by kind, state change, the mod's wipe at load followed by our redraw at 3 s, destroyed
  figures, caps, an old v1 payload drawing nothing, an unknown version, and the lint test of banned Lua forms.
- **Glue**: `internal/core/ttsglue_test.go` feeds real v2 payloads of all 37 companies through the helper: every ring on the right figure, none on a mount, none on a fallen unit, none above the caps.
- **Core**: `ttsauras_test.go` (v1 identity, version, kinds, mounts, foot, fallen, creature, caps, size fallback), the merge, `replaces` and selection filters of `auras_test.go`.
- **Web**: `internal/web/tts_test.go` (the Auras section, the switches, the chip row) and the e2e flow `96-tts-auras` ([testing.md](testing.md)). The demo world's campaign "Aura parade" carries every aura a company can reach ([demo-world.md](demo-world.md#aura-parade)).

### Risks and what contains them

| Risk | Containment |
|---|---|
| The mod's per-figure script wipes vector lines | merge on draw, redraw 3 s after load or state change, a Redraw button |
| `getVectorLines` returns something we cannot filter | overwrite fallback with a one-time message |
| `onObject*` events not delivered to an object script | the Redraw button covers state changes |
| Mesh bounds larger than the base (overhang) | accepted: the helper measures the bounds, so the ring and the mod's own circle agree |
| Oval bases | the circle uses the larger half-side, so rings clear the base |
| Clutter (six Heroes with 6" rings) | merge by kind and radius, caps, per-kind buttons, large rings off |
| The curated radii drift from the rules | the validator and the rule-text radius test |
| An old helper with a v2 payload | the version rule; the dialog tells to download the helper again |
| The data wrongly says a rule is an aura | the explicit excluded list with reasons; ambiguous entries default off |

## Smoke tests in Tabletop Simulator

These are checked by playing; they are the part that no test can show.

1. **Scale.** On any spawned figure press numpad 3 (the mod's circle), numpad 1 until 6", and draw the 6" command ring: the two rings must coincide to the line width. If ours is off by a constant factor, the conversion constant is one line in the helper.
2. **Wipe at load.** Draw rings, save, reload: the rings must be back after about 3 seconds.
3. **Merge.** On a figure that shows the mod's circle (numpad 3 on) press Apply names: both the mod's circle and the aura rings show. Press numpad 3 again: the mod's circle goes (ours may go too: press Redraw and they return).
4. **State change.** Switch a Rider of Rohan to its foot state: the rings return within 3 seconds on the new figure.
5. **Pickup, rotate, scale.** Lift a figure, turn it, drop it: the ring stays centred under it. A big (scale 1.5) figure's ring still measures 6" from its base edge.
6. **Kind buttons.** Hide and show `banner`, `command`: only those rings go; hide all and show all.
7. **Many figures.** Spawn 24 figures with banners and Heroes: Apply names does not freeze the game.
8. **Colours.** The five colours in the mod's lighting: any two hard to tell apart (the palette is one table in the app).
9. **Re-apply** after changing the company (buy a banner): old rings vanish, new ones appear; a figure removed from the table leaves no ring behind.
10. **Old helper** with a v2 payload asks for a new download; the new helper with a v1 payload draws nothing and breaks nothing.

## Defaults taken

| Question | Default |
|---|---|
| A 6" Stand Fast ring on every Hero | yes, kind `command`, one switch to hide |
| Groups on in the dialog | always, declared and conditional on; spells, targeting ranges, large rings and optional rings off |
| Receiver-side rules (a unit near its named hero) | no ring |
| Dashed rings for the enemy kind | no: a thicker solid line |
| Ring measured from the mesh bounds or the vanilla base size | bounds (agrees with the mod's circle) |
| Aura switch on by default | on (it makes an old helper ask for a new download) |
| The on-foot spare of a rider | gets the same rings |
| A ring-per-weapon-range feature | not offered |
