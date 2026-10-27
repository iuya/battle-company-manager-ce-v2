# Tabletop Simulator export (research and proposals)

Status: **built: the core list (`internal/core/ttslist.go`, see [Built: the core list](#built-the-core-list)) and the web side (button and dialog on My Company, another player's company and the live match, see [Built: the web side](#built-the-web-side)).** The Level B helper script (with aura rings, see [Built: aura rings in the helper](#built-aura-rings-in-the-helper-bead-4fe5)) and Saved Object builder are built too (see [Built: the helper and the Saved Object](#built-the-helper-and-the-saved-object)); the names payload, the web block, the Auras section and the download route are built as well. What is not verified is how the mod itself reads the list (see [What remains to verify](#what-remains-to-verify-owner-in-tts)). First research 2026-10-25, revised the same day after reading the mod's real Lua from the
owner's saved games. This page answers: can a Battle Company be taken from this app into Tabletop Simulator (TTS), where many players use
the Workshop mod *MESBG FTC - Version 0.4*, and how simply. The recommended first slice is in [Recommendation](#recommendation); its beads
carry the label `tts`.

Wanted by the owner, two levels:

- **A.** A copy icon next to the company name on My Company (with a tooltip saying what it does and what it is for) that copies the whole
  company as a list of the closest vanilla unit for each Battle Companies (BC) unit, because the mod can only spawn vanilla models.
  Riders are one line; a mount that can fight on its own (Attacks above 0 in the data: wargs) also gets a figure line.
- **B.** If possible, the spawned models also carry this app's in-game name and description (BC name, promoted hero name, path, wargear,
  rules, stats, injuries).

## Evidence and its limits

**Where the facts come from.** The first research found no Workshop mods on the development Mac. Since 25 October 2026 they are there: `~/Library/Tabletop Simulator/Mods/Workshop/`
(103 json files, among them the FTC mod 3133220714 and the Age of Sigmar table 3272401079; read for [aura-ranges.md](aura-ranges.md)). The findings below were made from the saved games, which hold the same scripts. What exists is
`~/Library/Tabletop Simulator/Saves`: 14 saved games (April to June 2026, their `GameMode` is "MESBG FTC - Version 0.4", the paths in
`SaveFileInfos.json` are from a Windows machine) and `Saved Objects/` (about 20 `.json` + `.png` pairs). A TTS save embeds the **Global
script, every object's script, the Notebook tabs and every object**, so the mod's spawner and its model bag are readable there. Files were read
only, extracted to `/tmp` (outside the repository); nothing of the mod's code or model list is copied into the repository, only short
quotations and counts.

**What this does not prove.** The saves are older than the mod's last update (27 September). The 14 saves contain one single version of the
spawner script (identical in all of them), but the current Workshop version may differ. The owner should confirm with the smoke test below.

**Other sources:** the Siege Ladder guide
(<https://siegeladder.com/articles/@joepavitt/how-to-play-mesbg-on-tabletop-simulator>); the Steam page of the mod (id 3133220714, fetched with
curl): author Bazuso, "base for additive loading of maps", description with no army list instructions; the code of TTA's modular tool (client
bundle) and of <https://github.com/avcordaro/mesbg-list-builder-v2024> (`TtsView.tsx`, `tta2mlb.json`); the TTS API docs
(<https://api.tabletopsimulator.com>). The Steam page showed a "removed from the community ... incompatible with Tabletop Simulator" banner when
fetched without a login; the owner reports the mod is subscribable, so we treat the banner as a fetch artefact.

## How the mod really works (from the saved games)

1. **Input.** The *Spawn Army* button sits on one table object. Its script reads the **Notebook tab whose title is exactly `ArmyList`**
   (not "the first tab"; the guide's wording is loose): `getNotebookTabs()`, `page.title == "ArmyList"`, else it broadcasts
   `Notebook page 'ArmyList' not found.` The player must create the tab once and paste into it.
2. **Parsing, line by line.** For each line the script takes the text between the **last `(` and the last `)`** of the line
   (`line:sub(firstChar + 1, secondChar - 1)`, where both indices are overwritten on each bracket), trims spaces, and drops empty lines. A
   line without brackets is used whole. Consequences: **at most one pair of brackets per line, none inside a name**. This is why the builders
   strip brackets.
3. **Hero or follower.** A line that matches `^%d+x` is a follower (`count, model = line:match("^(%d+)x%s+(.+)")`); it belongs to the last
   hero line; a follower before any hero line is **silently dropped**. **Any other line is a hero line**, whatever the model is: hero and
   follower names are looked up in the same place, so a warrior-list unit can head a warband. Repeated hero names are kept apart
   (`name (2)`), warbands spawn in an unspecified order (`pairs`), each as a block of 6 per row, 1.1 inch apart.
4. **Name lookup: exact string, options included.** The remaining text, with any further `(...)` group removed, is compared
   **case-insensitively and as a whole** with the `Nickname` of each object in one bag (GUID `24f6d0`, hard-coded). The bag's nicknames are of the form
   `Unit name: Option A, Option B` and `Unit name:` when there are none, i.e. **exactly the TTA text of a line without its brackets and
   count**. So the list does not "choose options": every model in the bag is one *name + option combination*, and the combination must exist.
   Not found: the orange broadcast `Could not find: <name>` and the script goes on with the next model.
5. **Spawning.** The matching bag object is taken out, cloned (`original.clone`), the original is put back; **both get `setName(task.name)`**,
   that is, the spawned model's Nickname becomes the line text (`Warrior of Rohan: Banner`, with the colon and without count). Description,
   GMNotes and tags are left as the bag object has them. After the queue, the player sees `N Models | B Break | R Remaining` (N = every model
   spawned, B = N/2 + 1, R = N/4).
6. **No hook for text.** There is no suffix, comment or note syntax: any extra text becomes part of the lookup key and the model is not found.
   **Level B therefore needs our own script**, not a richer list.

**The model bag** (read from save 11): 1764 objects (1477 distinct keys after case/space folding). 1471 have no Description, 293 carry an
English stat card in TTS rich text; most scripts are the 1" to 30" range-circle helper. 1282 of the 1477 keys are exactly a vanilla unit
name with options that exist in the vanilla file (the rest: old spellings such as `Aragorn Strider:` and `King of Men Rohan:`, 13 keys with brackets that
the parser cannot reach). Options are **mostly alphabetical** (445 of 486 multi-option keys). A unit whose options are all
"choose one" has **no bare model**: `Mirkwood Elf Warrior:` does not exist, only `...: Shield`, `...: Elf Bow`, `...: Elven Glaive`.

**Descriptions in the owner's own games.** The owner already renames and describes models by hand in the saves: Nickname
`[b]Rutabi, General of the Dragon Legion[/b] (56)` (name and points) and Description in TTS rich text (`[FFCC00][b]M[-][/b] 6 | ...`, one block for
stats, one for Might/Will/Fate, then equipment, rules, heroic actions). This is exactly what level B would generate; the markup is
`[RRGGBB]...[-]`, `[b]`, `[i]`.

**Other mods.** The saves hold `Snare Trap` and `Tripwire Trap` objects (Traps & Snares) as ordinary objects; no list syntax spawns them. The "Models
[Cleaned]" and 3D model mods are not on disk; the bag in the save is self-contained (mesh/texture URLs on Steam UGC), so the list only needs the
base mod. Traps for Master of Traps are outside this feature.

## The text format that works

```
(Hero Name: Option A, Option B)
    (3x Warrior Name: Option)
    (1x Other Warrior: )

(Next Hero: )
    (6x Warrior Name: Option)
```

- A line = `(` + `[Nx ]name: option list` + `)`. Heroes without options: `(Name: )`. The leading spaces are optional (the parser trims them).
- No bracket inside the text: names and options with brackets (`Troll Brute (Troll)`, 13 bag keys) cannot be spawned; strip the brackets
  as the builders do, knowing a few will not be found.
- Option order: write them **sorted, case-insensitively**, (41 of 486 multi-option keys in the bag are not alphabetical; the local
  developer test can try the permutations against the local catalogue and report such units for `data.ttsModels`).
- `includes` of a vanilla unit (siege crew, Beorn's bear) are separate follower lines in the builders' output; `hideInTts` gear is not an option.
- A blank line between warbands is cosmetic.

## How a BC unit maps to a vanilla unit (data, 37 companies)

A BC unit in this app is `CompanyUnit` (`internal/core/company.go`): `Type` is a BC name that resolves to a vanilla key (or to a company
alias), `Equipment` holds what it carries (wargear, equipment, bought mounts), `Path` the hero's path, `Parts` the models of a composite
unit. Counted over every unit name the 37 companies use (the walk of `internal/core/keywords_oracle_test.go`):

| Finding | Count | Consequence |
|---|---|---|
| Distinct unit names used by the 37 companies | 128 | the vocabulary |
| ... a vanilla unit name as is | 113 | emit the name |
| ... legacy renames (High Elf Warrior, Grimhammer, Warrior of Erebor, Iron Hills Dwarf, Mirkwood Cavalry, Giant Mirkwood Spider, Abrakhan Guard, Rhunish War Drake) | 8 | emit the alias's `unit` |
| ... preselected option (Tookish Hunter, Battlin' Brandybuck, Helminga, Lorien Guard, Noldorin Exile) | 5 | base unit + that option (`Noldorian Exile` is vanilla's spelling) |
| ... vanilla unit plus a rule (Ranger of Ithilien, Goblin Hulk) | 2 | base unit; the rule is invisible (level B) |
| Distinct vanilla units after resolving aliases | 113 | 41 have no options, 72 have 1 to 7 |
| Names vanilla defines twice, used by companies | 3 (Galadhrim Knight, Guard of the Galadhrim Court, Ranger of the North) | the bag has one key per name+options, so nothing to decide |
| Cavalry units whose mount is vanilla wargear | 19 | one line (rider and mount are one model in the bag); 4 of them have a fighting mount (see below) |
| Mounts a company can buy (class `mount`) | 9 | 2 of them fight (Warg, Fell Warg) |
| Composite unit (Warg Marauder) | 1 | the bag has `Warg Marauder:` (and a misspelt `Warg Maruader:`) |
| Wanderers (4), creatures (3) | 7 | not in the bag: approved stand-ins in `data.ttsModels` (below) |
| Table entries with a wargear list (start, reinforcements, special chart) | 155 | 143 map to a model key that exists in the bag with a plain selection (prototype of the rules below); 12 name a unit whose bag models need another option (`Mirkwood Elf Warrior` asks for `Elven-made glaive`, the bag has `Elven Glaive`...); the exporter must fall back to the nearest existing key |

The numbers are from the shipped files and the bag read from save 11; the implementation computes, it does not hard-code them.

### Mapping rules

1. **Name**: `CompanyUnit.Type` -> alias `unit` -> vanilla name, byte for byte as in `data2024.json`, then through `data.ttsModels` (below) when the
   mod's model has another name or no 1:1 model exists.
2. **Options**: from `Equipment` and the alias's `selectedOptions`, find the vanilla options of the unit by normalised name, expanding `includes`
   ("Shield and Spear" = Shield + Spear), at most one per `chooseOneKey`/`requireChooseOneKey`, preferring one option that covers several items. Items
   with no option are dropped and listed as notes. Armour upgrades and BC-only equipment are never options. Options are written sorted.
3. **Order and warbands** (decided with the owner: the units are written in the order of the My Company page, `core.OrderUnits`: leader,
   sergeants, other heroes, regular units, or the player's saved order). The spawner makes a *follower line belong to the previous hero line* and
   treats every non-`Nx` line as a hero line, whatever the model. The code has **no cap per warband**: `modelIndex % 6` only lays six figures
   per row, and `warbandSpacing`/`rowSpacing` only place the blocks (the 6-per-warband idea was the tabletop rule and is dropped). Design: **one
   warband**. The first unit of the page (the Leader) is the hero line; every other unit follows as a follower line, in page order, `1x` each, with
   identical *adjacent* units merged (`3x Warrior of Rohan: Shield`). Why: it keeps the page order exactly (several warbands would be spawned in
   the random `pairs()` order and heroes could not be told from warriors by the mod anyway), it needs no warband-size rule, and the whole company
   spawns as one block the player lifts out. If the first unit is not a hero (no hero in the company) the same holds: it heads the list.
   Alternative (not built): one warband per hero, the warriors after the last hero all under it. `REVIEW`.
4. **Riders** (decided with the owner; data below): the rider is the mounted model. Dismounting, when the data supports it:
   - **Preferred: one figure with states.** 488 of the 1764 bag models carry TTS *States* (`States` in the save, 471 with one more state, 13 with
     two); the player changes state (right-click > States, or the hot key) and the mounted rider becomes the rider on foot, and for the Hunter Orc
     Warg Rider also the Fell Warg alone. When the unit's models have these states (`dismountStates: all`), the export adds nothing.
   - **Otherwise two or three versions** (a switch in the dialog, default on): the mounted line, a follower line of the on-foot model
     (`footModel` from the table, same options when the foot unit has them), and, when the mount has **Attacks above 0** in the vanilla `gear`
     entry (Warg, Fell Warg; `Fell beast`, `Armoured Fell beast`, `Great Eagle` too, none recruited today), a line for the mount alone. The
     data decides the Attacks (`gear[].attack > 0`, `unitType` contains `Mount`), never a list. Mounts with 0 Attacks (horses, camels, goats, ponies) get no
     figure.
   - Data support for the 19 cavalry units, by the states in the bag (save 11; "keys" are the bag's model keys of that unit):

     | Support | Units | What the export does |
     |---|---|---|
     | states on every key (4) | Rider of Rohan (7 keys), Son of Eorl (2), Mirkwood Elf Knight (1), Hunter Orc Warg Rider (3; the states are rider on foot **and** Fell Warg alone) | one line; note "change the figure's state to dismount" |
     | states on some keys (6) | Black Numenorean Knight (3 of 4), Galadhrim Knight (3 of 7; the states are the same name, unclear), Iron Hills Goat Rider (1 of 2), Knight of Minas Tirith (1 of 2), Mordor Warg Rider (2 of 5), Rivendell Knight (2 of 3) | treated as "no states": the on-foot line is added (the player may get a spare model); the Iron Hills Goat Rider's foot model is a plain `Dwarf Warrior`, **provisional** (owner choice: the bag has no Iron Hills foot model) |
     | no states (8) | Angmar Warg Rider, Dunlending Horseman, Easterling Kataphrakt, Haradrim Raider, Isengard Warg Rider, Khandish Horseman, Knight of Arnor, Serpent Rider | on-foot line from the table (a foot model exists as separate keys for the people: Angmar Orc Warrior, Dunlending Warrior, Easterling Warrior, Haradrim Warrior, Isengard Orc Warrior, Khandish Warrior, Warrior of Arnor, Serpent Guard); the three warg riders also get their Fell Warg line |
     | model not in the bag (1) | Mirkwood Cavalry (BC alias of Mirkwood Elf Knight: the alias resolves to a model that has states) | as Mirkwood Elf Knight |

     The bought mounts: a foot unit that bought a Warg or Fell Warg gets a mount-alone line (stand-in Fell Warg); one with a Horse etc. gets nothing.
     **Side effect to show the player**: the mod counts every spawned figure, so `N Models | Break | Remaining` is too high by the number of extra
     figures; the dialog states the real numbers ("14 models + 3 extra figures; the Break Point for 14 is 8").
5. **Notes, not guesses**: every approximation is a note to the player; every stand-in is a "shown as" note.

### The fixed mappings: `data.ttsModels` in the Battle Companies json

Decision of the owner: **the model-key catalogue is not committed**. Everything that has no 1:1 name in the mod becomes a small table in
`battle_companies.json`, generated by `tools/` (curator-owned), which core reads (`RuleSet.TTSModels`); so the exporter needs no catalogue.

```json
"ttsModels": {
  "text": "How a Battle Companies unit is written for the Tabletop Simulator MESBG FTC mod ...",
  "standIns": [ { "kind": "wanderer", "name": "Wandering Swordsman", "model": "Captain of Dale", "alternatives": ["Ruffian", "Knight of Dale"] }, ... ],
  "mounts":   [ { "gear": "Warg", "model": "Fell Warg", "alternatives": ["Wolf"] } ],
  "riders":   [ { "unit": "Angmar Warg Rider", "footModel": "Angmar Orc Warrior", "dismountStates": "none" }, ... ],
  "names":    [ { "from": "Elven-made glaive", "to": "Elven Glaive", "scope": "Mirkwood Elf Warrior" } ]
}
```

- `standIns` (the approved choices): Wandering Swordsman = `Captain of Dale`, Wandering Marksman = `Ruffian: Bow`, Wandering Scavenger = `Goblin Mercenary
  Captain`, Wandering Sage = `Malbeth the Seer`, Baggage Pony = `Bill the Pony`, Falcon = `Crebain`, Hunting Dog = `Fang`; the unit and mount `model` is
  a key `Name: options` or a bare name. The mount `Warg` -> `Fell Warg`. Alternatives are kept for a later change of mind.
- `riders`: the 19 cavalry units: foot model and `dismountStates` (`all|some|none`, from the table above; regenerated by hand from the local bag when
  the mod changes).
- `names`: spelling differences between a vanilla option and the mod's (found by comparing with the local catalogue), scoped to a unit.
- **Validator** (`tools/validate_definitions.py`): every unit, option and model name in the table must exist in the vanilla file (all the stand-ins do);
  **only if** `sources/tts/models.json` exists (gitignored; `tools/extract_tts_models.py` generates it from a local TTS save, see below) every model must also be a key of it. No catalogue
  is ever in the repository. `core` consumes the table. `TTSCatalogue` (an interface in core) is optional: see "Keys the bag does not have".

### Keys the bag does not have

Three gaps are fixed mappings in `data.ttsModels` (`defaultOptions`, `markerOptions`), applied always, with no catalogue (the published image has none),
each with an `approximate` note: a Lorien Guard with no other option requests the bare `Galadhrim Warrior:` (`TTSLine.Dropped` keeps the marker; the
descriptions header says `Lorien Guard`, the figure name already uses the alias label, the nickname is the bare key); a `Mahûd Warrior` with no option
requests `Mahûd Warrior: Spear`; a `Rohirrim Traitor` with no option requests `Rohirrim Traitor: Shield` (REVIEW: the owner may prefer `Shield and throwing spears`).

As a second safety net, when `TTSOptions.Catalogue` is set, any other line whose exact key (name and options) is not in the bag falls back to the closest key,
with an `approximate` note that names what changed. Order: (1) drop the marker options (those a company alias selects) if the rest is a key; (2) else the key
of that unit that keeps most of the carried options, then has no ranged weapon/banner/war horn (bow, crossbow, blowpipe, throwing, banner, horn, sling), then
has a melee weapon (spear, sword, axe...), then the cheapest by the unit's option points. The catalogue is the mod's content and never committed: `cmd/server`
loads it only when `TTS_CATALOGUE` names a file (same format as `sources/tts/models.json`); a production image has none.

### The local model catalogue (`tools/extract_tts_models.py`)

`python3 -I tools/extract_tts_models.py [save.json]` reads a saved game of the owner's Tabletop Simulator (default: the newest
`~/Library/Tabletop Simulator/Saves/TS_Save_*.json`), finds the model bag (GUID `24f6d0`, also when nested) and writes the stripped, de-duplicated
`Nickname` of its objects as a JSON list to `sources/tts/models.json`. That is all it keeps: no scripts, descriptions or meshes. The path is fixed;
the tool refuses to write unless the file is under `sources/tts/` and git ignores it (the catalogue is the mod's content and is never committed).
It is read by `tools/validate_definitions.py` (names of `data.ttsModels` must be keys of it) and by the local-only `TestTTSListCatalogue`
(skipped when the file is absent): for every unit of every company with no gear, with each option and with each pair of options, every emitted key
(trimmed, case-insensitive: a bare unit is `crebain: ` with a trailing space, the bag has `crebain:`) must be in the catalogue. Tests with a hand-written fake save: `tools/test_extract_tts_models.py`.

## What TTS offers, and what a web page can do

| Mechanism | Needs | Notes |
|---|---|---|
| Copy text to the clipboard from our page | nothing (`navigator.clipboard.writeText` over HTTPS or localhost; fallback: select a text area) | the user pastes into the `ArmyList` Notebook tab |
| Download a file from our page | nothing | lands in Downloads; Saved Objects live in `Documents/My Games/Tabletop Simulator/Saves/Saved Objects` (macOS: `~/Library/Tabletop Simulator/Saves/Saved Objects`, `.json` + `.png` pairs, seen on disk); a browser cannot write there |
| Talk to TTS from the browser | **impossible** | TTS listens on a local TCP port (39999, External Editor API); only a local program can connect |
| Lua on a Saved Object | the mod allows scripts (it uses them itself) | `getObjects`, `setName`, `setDescription` (rich text), `setGMNotes`, `addTag`, `getNotebookTabs`, `JSON.decode`, `WebRequest.get` |
| Custom Model / Figurine | model + texture URLs | not needed: the bag already has every model |

## Proposals, ranked by simplicity

| # | Proposal | Level | Effort | Verified how |
|---|---|---|---|---|
| 1 | **P1 Copy as text list** in the `ArmyList` format, with mount figures and a notes dialog | A | S to M (3 beads) | golden tests over all 37 companies against the vanilla file; an optional local test against the bag catalogue; owner smoke test |
| 2 | **P4 "Apply BC names" helper**: a Saved Object `.json` (downloaded once from the app) whose script reads a second Notebook tab `BCArmy` (JSON from the app) and, on a button, finds each spawned model by its Nickname (the spawner sets it to the exact list line) and sets Name, Description (rich text), GMNotes and a tag | B | M | the JSON/Lua text is generated and linted; behaviour only in TTS (owner) |
| 3 | **P2 Info cards**: the same data as one card/notecard object per unit in a Saved Object | B (cards) | M | owner in TTS |
| 4 | **P4b Own spawner** (a script that takes models from the bag by GUID itself and names them while spawning) | B | M to L | rejected: hard-codes the bag GUID and duplicates the mod |
| 5 | **P5 Local helper using the External Editor API** | B | L | rejected: install and trust |
| 6 | **P3 Own `gameUrl` JSON** for the modular tool | none | M | rejected: the bag is keyed by vanilla names, BC names are not found |

Hybrid: **P1, then P4**. Level B needs no change to the list: the spawner already names each model with the exact line text, which makes
P4 a lookup by Nickname. Matching is by name+options key; identical models (six `Warrior of Rohan: Shield`) are interchangeable, so the helper assigns BC
units to them in arbitrary order and the players move them as they like (unit names make them distinct afterwards). P4 data per unit:
Nickname `[b]Name[/b] (points)` (the owner's own convention), Description with stats (with the green/red change colours), path, equipment, rules,
injuries; the app already has every piece on the unit card.

## Recommendation

Build **P1** as the first slice (the epic `battle-company-manager-ce-v2-zrs`): the copy button on My Company, on other players' companies and in live
matches; the dialog with the list, the switches, the warnings and the real model count and Break Point; the mapping in core with golden tests; the fixed mappings in the
Battle Companies json. **P4** (own Lua helper, Saved Object download; the owner accepts the one-time install) is a **later epic**, after the owner
confirmed the spawner and the list. Level B needs no change to the list: the spawner already names each model with the exact line text.

Exact tooltip of the button (a `has-tip`, also the `aria-label`):

> Copy this company as an army list for Tabletop Simulator. Each unit becomes the closest standard MESBG model, because the Tabletop Simulator mod
> can only spawn standard models. Paste it into a Notebook tab named ArmyList and press Spawn Army. Names, paths, injuries and special rules are not carried over.

### Spec of the slice

**Screens and states** (mobile first, 360 px up). Next to the company name (and the rename pencil) on My Company, on another player's company page
(read only) and in the live match (next to the company's name in the live company view): an icon button (copy icon, 44 px touch area) opening a
dialog "Army list for Tabletop Simulator" in the app's dialog pattern ([frontend.md](frontend.md)):

- A short numbered how-to (open the Notebook, add a tab named `ArmyList`, paste, press Spawn Army at the north end of the table).
- The list in a read-only monospace text area (and the same text in a `<pre>` for no-script use), a **Copy** button (success: "Copied" for 2
  seconds and a polite live region "Copied N models"; clipboard refused: the text is selected and "Press Ctrl+C" appears).
- Two switches that re-render the list (htmx): "Riders on foot" (on-foot lines for riders whose model has no dismount state; default on) and
  "Mounts that can fight as their own figure" (default on).
- The numbers: "14 models + 3 extra figures. The mod will report 17 models; the Break Point for 14 is 8."
- A hint when a rider's model has states: "Change the figure's state to dismount" (a `has-tip`).
- "Not carried over" and "Shown as" sections (only when there are notes), one line per unit, each a `has-tip`.
- **Empty company**: no button. **No unit has a model**: "None of these units has a standard model" and no copy. **Frozen campaign**, **another player's
  company**, **live match**: the button is drawn, it only reads (in a live match it uses the units as they are now, fallen or retreated units
  included, with a note per fallen unit; REVIEW). **Changed rules file**: "This campaign uses a changed rules file; some names may not exist in the
  mod". Server error: the app's standard error panel.

**Acceptance checklist**

- [ ] Button next to the company name on My Company, another company's page and the live match; keyboard focusable, visible focus, tooltip on hover and focus with the text above; no horizontal scroll at 360 px; light and dark; in `/styleguide`.
- [ ] The dialog opens by htmx, closes with Escape, focus returns to the button; the switches re-render the list without a full page.
- [ ] Text format exactly as in [The text format that works](#the-text-format-that-works): one bracket pair per line, no brackets inside, `Nx`, `: )` for no options, options sorted.
- [ ] Order: the units in the order of My Company (`core.OrderUnits` or the saved order); the first unit is the hero line, all others follow it as follower lines; identical adjacent units are merged; no warband cap.
- [ ] Test over all 37 companies (starting units, reinforcement and special-chart units, advancement targets): every emitted name and option exists in the campaign's vanilla file; a unit that cannot be mapped fails the test unless a note is produced.
- [ ] Aliases resolve (Tookish Hunter -> `Hobbit Archer: Tookish Hunter`, High Elf Warrior -> `Rivendell Warrior`, Ranger of Ithilien -> `Ranger of Gondor`).
- [ ] The 19 cavalry units follow the table of rule 4: states on all keys -> one line; otherwise the on-foot line (switch on); Warg/Fell Warg riders and bought Warg/Fell Warg add a `Fell Warg` figure; a Horse adds none.
- [ ] The seven wanderers/creatures appear as their stand-in with a "Shown as" note, read from `data.ttsModels`, not from Go constants.
- [ ] The Battle Companies json has `data.ttsModels`, generated by `tools/`; `tools/validate_definitions.py` checks it against vanilla and, when `sources/tts/models.json` exists, against it.
- [ ] No other player can read a company they cannot see (the check of the company page); the Copy button works under the CSP (no inline script).
- [ ] `docs/frontend.md` (My Company, company page, live) and this page say "built".

**Interface the web layer needs from core** (declared in `internal/web`, implemented by `core`; names are proposals for the backend planner):

```go
// TTSExporter is what the web layer needs for the copy button (My Company, another company, the live match).
type TTSExporter interface {
    // TTSList returns the company as an ArmyList text for the Tabletop Simulator MESBG FTC mod.
    // viewerID must be allowed to see the company (same rule as the company page).
    TTSList(ctx context.Context, viewerID, companyID int64, opts core.TTSOptions) (core.TTSExport, error)
}

type TTSOptions struct {
    FootVersions bool // add the on-foot line of riders whose model has no dismount state
    MountFigures bool // add a figure line for every mount with Attacks above 0
}

type TTSExport struct {
    Text         string       // paste-ready, format of docs/tabletop-simulator.md
    Models       int          // company models in the list, without the extra figures
    ExtraFigures int          // on-foot and mount lines added by the options
    BreakPoint   int          // half of Models, rounded up, as the rules count it (the mod's message counts every figure)
    Lines        []TTSLine    // the same content, structured (tests, the later helper); Lines[0] is the hero line
    Notes        []TTSNote
    CustomRules  bool         // the campaign's vanilla file is not the shipped one
}
type TTSLine struct {
    UnitIDs []int64  // company units this line stands for (several when identical adjacent units are merged); an extra figure carries its rider's ID
    Name    string   // exactly as the mod's bag names it, no brackets
    Count   int
    Options []string // sorted
    Key     string   // lower-cased "name: options", the bag key (the later helper's lookup)
    Extra   string   // "", "foot" or "mount": why the line exists
}
type TTSNote struct {
    UnitID int64
    Kind   string // "noModel", "optionDropped", "shownAs", "approximate", "hasStates", "extraFigure"
    Text   string // one sentence for the player
}

// TTSCatalogue is an optional local developer aid (keys from sources/tts/models.json); nil in production.
type TTSCatalogue interface{ Has(key string) bool }
```

Errors: `core.ErrNotFound` (company missing or not visible) and the same forbidden error as the company page (the backend planner confirms the
sentinel names). The function is pure over the loaded company and ruleset (+ `RuleSet.TTSModels`): no repository method, no migration. The mapping sits
next to `RuleSet.UnitKeywords` and uses `resolveUnit`. In a live match the caller passes the live company's units.

## Built: the core list

`internal/core/ttslist.go` (bead zrs.2). Pure functions over a built company (`CompanyDetail`, its `Roster` already in page order) and the rules; the
contract above is the web layer's view, this is what core offers it (the permission check stays in the web or company service):

```go
func (r *RuleSet) TTSList(d CompanyDetail, opts TTSOptions) TTSExport
func (r *RuleSet) TTSLiveList(d CompanyDetail, live TTSLive, opts TTSOptions) TTSExport // live match
```

- `TTSExport{Text, Warbands []TTSWarband, Notes []TTSNote, Models, ExtraFigures, BreakPercent, BreakPoint string}`; `Lines()` flattens the warbands.
  `Models` are the figures of units and creatures in the list (a stand-in counts, a unit without a model does not), `ExtraFigures` the on-foot and
  mount figures the switches add (the mod reports the sum). `BreakPoint` is the app's rule (`percent x Models / 100`, one decimal, "6.5"), with the
  company's `BreakPointPercent` or the file's default.
- `TTSLine{UnitIDs, Name, Count, Options, Key, Extra}`; `Line.Text(hero)` writes `(Name: A, B)` for the first line of a warband and `(3x Name: A, B)`
  otherwise, `(Name: )` without options. Note kinds: `noModel`, `optionDropped`, `shownAs`, `approximate`, `hasStates`, `extraFigure`, `fallen`.
- Mapping: the unit through `resolveUnit` (aliases), its non-profile wargear (and the options an alias selected) against the vanilla options (whole
  options first, at most one per choose-one group, an exchange option by the item it gives); a leftover item goes to the option that holds it
  (`approximate`) or is dropped (`optionDropped`; armour and equipment slots are silent); a required choose-one group nobody took gets its cheapest
  option (`approximate`: the mod has no bare figure). `data.ttsModels` supplies the stand-ins (wanderers, creatures), the riders (a rider with `states`
  adds nothing; otherwise `FootVersions` adds its on-foot figure, or a `noModel` note when `footModel` is empty), the mounts that fight
  (`MountFigures`, Attacks above 0) and the spelling differences. Brackets are removed from names and options, accents kept, options sorted.
- Live: `TTSLive{Units: LiveCompany.Units, Absent: LiveCompany.Absent}`. Units in the zone retreated or casualty, and absent ones, are the second
  warband (`Fallen`), after the main one, with one `fallen` note; the plain `TTSList` ignores all of it.
- Not in core: the `CustomRules` flag (the web layer compares the campaign's ruleset hashes with the shipped ones) and `TTSCatalogue` (a local
  test, `TestTTSListCatalogue`, reads `sources/tts/models.json` when it exists).
- Tests: `ttslist_test.go` (format, merging, the 18 riders, mounts, stand-ins, live, break point) and `TestTTSListAllCompanies`, which writes every
  start, reinforcement, special-chart, special and advancement entry of the 37 companies and compares with `testdata/tts_golden.txt`
  (`go test ./internal/core -run TTSListAll -update-tts` rewrites it; review the diff).

## Built: the web side

Bead zrs.3. Details of the window and its behaviour are in [frontend.md](frontend.md#copy-as-a-tabletop-simulator-army-list-built).

- `GET /campaigns/{id}/companies/{company}/tts[?live=1&foot=0|1&mounts=0|1]` (`internal/web/tts.go`, template `partials/tts.gohtml`, script
  `static/js/tts.js`): an htmx fragment, member-only like the company page, `Cache-Control: private, no-cache`, behind the recoverer and request timeout.
- Web interface `ttsExporter.TTSExport(ctx, campaignID, companyID, live *core.TTSLive, opts)`, implemented by the new `core.TTSService`
  (`internal/core/ttsservice.go`: loads the company and the campaign's `*RuleSet`, calls `TTSList` or `TTSLiveList`); the proposed
  `TTSExporter` with a viewer id and `CustomRules` in the export was not needed: the handler checks access and compares the rules files itself.
- `CustomRules` is the web layer's `customRules(CampaignDetail)`: any of the campaign's vanilla or Battle Companies files that is not the one the app ships
  with (`RulesetInfo.Default`) shows the warning "This campaign uses a changed rules file, so some unit names may not exist in the mod".
- Tests: `internal/web/tts_test.go` (mock), `internal/core/ttsservice_test.go`, and e2e flow `91-tts` ([testing.md](testing.md)).

### Built: the Auras section in the dialog (bead 4fe.6)

`ttsExporter.TTSAuras(ctx, campaignID, companyID, live, TTSOptions, AuraSelection)` (`core.TTSService.TTSAuras`) gives the kinds the company has with their rings and radii, the totals and what
each group switch adds; the handler builds the selection from `auras`, `akinds`, `adeclared`, `aconditional`, `acast`, `atarget`, `alarge`, `aoptional` and passes it on to `TTSNames`, which writes payload
version 2 only when a ring is selected. The dialog tells to download the helper again when the payload is v2. Details in [frontend.md](frontend.md#copy-as-a-tabletop-simulator-army-list-built) and
[aura-ranges.md](aura-ranges.md); tests: `internal/web/tts_test.go`, flow `96-tts-auras`.

### Built: the names block in the dialog (bead wu3.4)

`ttsExporter.TTSNames(ctx, campaignID, companyID, exp TTSExport, TTSNamesOptions)` (`core.TTSService.TTSNames`) returns the payload for the export the handler built;
the dialog (`?rules=none|short|full`, default none: names only) shows the JSON, **Copy names**, the size and the notes, the four steps, the helper download and the Lua
fallback. Details in [frontend.md](frontend.md#copy-as-a-tabletop-simulator-army-list-built); tests: `internal/web/tts_test.go`, flows `91-tts` (live: fallen marker) and `92-tts-names`.

## Built: the helper and the Saved Object

Beads wu3.1 and wu3.2. Package `internal/core/ttshelper` (pure, no web, no repository):

- `helper.lua`, the Lua script (embedded with `go:embed`). Common subset of Lua 5.1 and 5.2 (MoonSharp): no `goto`, integer division, bit operators,
  `string.pack`. `ttshelper.Script()` returns it, `ttshelper.ShowLua()` is the text for the "Show the Lua" fallback.
- `SavedObjectJSON()`: the Saved Object. The envelope has the keys of the owner's real Saved Objects (checked against `El dado.json` when it exists on the
  machine); the one object is a locked built-in `BlockRectangle` named "BC names helper" with the how-to as Description, the tag `BCHelper` and the script.
  `PNG()`: the 256x256 icon drawn with `image/png`. `Zip()`: `BC names helper.json` and `BC names helper.png` with fixed timestamps; `ETag()` for the
  download route (`GET /tts/helper.zip`, built in wu3.4).
- Behaviour of the script, as designed above, with these details: the buttons are **Apply names** and **Re-apply**; the tab `ArmyNames` of the pressing
  player's colour wins, else the first; the text must be at most 262144 characters. Errors go to the presser in red, the summary to everybody (green when
  every figure was named, orange with `Not found: key (n)`, `n more figures than names ... were left alone`, `N figures named like the list were left alone (not selected)`,
  `N figures have states`). The helper matches on the folded `l` (the nickname the spawner gives; `k` when `l` is missing) and pairs identical nicknames by grid position, which agrees with the payload's `o` for an unmoved army (checked by `internal/core/ttsglue_test.go`). The GM note is `bc-unit:<u>`, `bc-unit:<u>:foot` or `bc-unit:<u>:mount` for the extra figures that share their rider's id, plus `/<n>` when figures still share that (the parts of a composite unit such as Iron Shield Bearer + Foe Spear Wielder). A tagged figure whose note matches an entry counts as "already named" (so the second *Apply* reports
  "0 new, 9 already named" and misses nothing); a tagged figure of another company is left alone. Keys are compared after the helper's own folding
  (lower case, single spaces, and the accented capitals of Latin-1, because the app lower-cases with Unicode and Lua's `lower` knows ASCII).
  Grid order: rows by `floor(z / 0.55 + 0.5)`, then x. The script has no function that moves, clones or destroys an object.
- Tests (no game, no `lua` binary): `internal/core/ttshelper/*_test.go` run the script in gopher-lua (`github.com/yuin/gopher-lua`, **test-only**: pure Go, MIT,
  not linked into `cmd/server`) against a fake TTS API (`fakets_test.go`: objects without move/clone/destruct, `Player[...]`, notebook tabs, `JSON.decode`) and a
  simulation of the spawner's grid. Fixtures: `tests/tts/rohan_list.txt` (an `ArmyList`) and `tests/tts/rohan_names.json` (a hand-written JSON v1 payload for it).
  Cases: identical nicknames in grid order, merged `Nx`, foot and mount extras, an accented key, selection against whole table, an opponent's identical army,
  double Apply, Re-apply (also after a hand rename), missing and extra figures, tagged figures, every error message, the tab colour, no match, states, and a lint test
  for the banned Lua forms. The fake TTS lives in the test-support package `internal/core/ttshelper/ttsfake` (not imported by the server). `internal/core/ttsglue_test.go` feeds the real `TTSList` and `TTSNames` output of all 37 companies (also with every unit doubled so lines merge `Nx`), Rohan, Angmar with mount figures and a live list with a fallen second warband through the helper: every figure named exactly once and in the right figure, nothing unmatched, a second Apply names nothing, Re-apply renames all. The optional `tests/tts/run_real_lua.sh` is not written. What the fake cannot show (MoonSharp differences, the real API) is the owner's smoke test.

### Built: aura rings in the helper (bead 4fe.5)

Spec: [`aura-ranges.md`](aura-ranges.md). The producer of the v2 payload is `core.TTSNames` (see [Built: the names payload v2](#built-the-names-payload-v2-bead-4fe4) below); `tests/tts/rohan_names_v2.json` stays the hand-written fixture of the helper tests.

- **Payload versions.** The helper reads `v` 1 and 2; any other value prints "This helper is older than the app (names format N). Download the helper again from the app." (so an
  old helper that only knows v1 asks for a new download when it meets a v2 payload). Rings come only from a v2 payload: a v1 payload draws nothing and says nothing about rings (an `a`
  field in it is ignored). Per figure `a` = `[{r, k, w, n}]` (radius in inches 0 < r <= 48, kind, when, labels; invalid rings are skipped), optional `b` = base radius in inches; top level
  `auras` = `{kinds:[{id, rgb}], cap}` optional (recolour, lower the cap below 4).
- **Drawing.** One closed line of 48 segments (49 points) per ring with `Object.setVectorLines`, in the figure's local space, so it moves with it. Radius `r + base - 0.0225` where
  base is `b` or `max(size.x, size.z) / 2` of `getBoundsNormalized()` (the mod's own circle measures the same), 1 inch = 1 world unit; x, y, z and the thickness are divided by
  `getScale().x`. Thickness 0.045 (enemy 0.06), local height `0.1 + 0.015 * kind index`, a second ring of the same radius on a figure is 0.03 larger. Colours (Okabe-Ito): banner `F0E442`,
  command `E69F00`, support `56B4E9`, enemy `D55E00`, utility `CC79A7`.
- **Merge.** `setVectorLines` replaces all lines, so the helper reads `getVectorLines()`, drops the lines that are its own (a kind's colour and that kind's thickness times the scale),
  appends the new ones and sets the result: the mod's own green 0.05 circle stays. If the lines cannot be read it overwrites and says so once ("the mod's own circle may disappear").
- **Memory.** The helper keeps `guid -> rings` and saves it with the game (`onSave`, JSON, with the on/off state, the hidden kinds and the payload palette). `onLoad` redraws 3 seconds
  later (the mod's figure scripts clear their lines at 2 seconds). `onObjectStateChange(new, oldGuid)` moves the entry to the new GUID and draws 3 seconds later; `onObjectDestroy`
  drops the entry (kept 5 seconds in case the state change event comes second). Figures that are not on the table (in a bag) are skipped.
- **Buttons.** Apply names, Re-apply, **Rings: on/off**, **Redraw**, and one coloured button per kind (B C S E U; the letter is in brackets while the kind is hidden). Each press redraws the
  remembered figures, 10 per frame. Apply and Re-apply draw the rings of the figures they name; Re-apply first removes the helper's lines from every remembered figure.
  Message: `Rings drawn for 14 figures (31 rings).`; `N rings left out (limit 6 per figure, 200 in all).`
- **Tests.** `ttshelper/helper_rings_test.go` with the fake TTS (`ttsfake`: `setVectorLines`/`getVectorLines` recorder with the calls copied in order, `getScale`, `getBoundsNormalized`,
  `getObjectFromGUID`, a `Wait` shim on a virtual clock advanced by hand, `Load`/`Save`, `Destroy`/`ChangeState` events, the mod's wipe simulated with `After`): geometry, scale, base from bounds
  or `b`, colours/heights, merge, overwrite fallback, switches, redraw after load/wipe/state change, destroyed figures, Re-apply cleanup, idempotency, caps, chunks, versions, payload palette.
  The lint test also bans setScale, setRotation, setState, setLock, spawnObject and destroyObject. `internal/core/ttsglue_test.go` feeds real v2 payloads through it (below).
- **Owner's smoke tests** (aura-ranges.md, section 8, tests 1 to 5: scale against the mod's circle, wipe at load, merge, state change, pickup/rotate/scale) and the layout of the new buttons on the block face are not verified.

### Built: the names payload v2 (bead 4fe.4)

`core.TTSNames` writes the rings of [`aura-ranges.md`](aura-ranges.md) (`internal/core/ttsauras.go`, `auras.go`). `TTSNamesOptions{Rules, Auras AuraSelection}`; the zero `AuraSelection` writes none.

- **Version.** `v` is 2 only when at least one ring is written; with no rings (the selection draws nothing, the company has none, everything is filtered) the payload is the v1 of before, byte for byte
  (`TestTTSNamesV1StaysByteIdentical` compares it with the written-out v1 shape). `TTSNamesVersionAuras = 2`.
- **Per figure** `a:[{"r":3,"k":"banner","w":"a","n":"Banner"}]` (radius in inches, kind id, when: `a` always, `d` declared, `x` conditional, `c` cast, `t` target, labels joined) and, only when
  `AuraSelection.Base` is on, `b` = the base radius in inches of the vanilla unit (`VanillaUnit.BaseSize` parsed by `core.BaseRadius`: `25mm` is 0.49, an oval takes its longer side, the typo `170mmm oval`
  reads as 170 mm; a unit with no base size in the file, 51 of them, sends none and the helper measures the figure; no `b` for the on-foot figure). Off by default (spec question Q7: the
  ring agrees with the mod's own circle when the helper measures). The top-level `auras` object (palette, cap) is not written: the helper has the same palette built in.
- **Which figures.** The rider figure of a unit and its on-foot version (`x` `""` and `foot`), a creature's own figure (Keen Sight of the Falcon, shown as Crebain); never a mount (`x` `mount`),
  never a unit of the fallen warband, and of a unit made of several figures (a bundle such as Iron Shield Bearer + Foe Spear Wielder) only the first.
- **Limits** (core, in this order): at most **6 rings per figure** (the kinds in the order of the data, then the smaller radius) and **150 in the payload**, then the 24 KB rule: the rule texts are
  lowered as before, and when the payload with rings is still too big the rings are what is given up (the payload falls back to v1) with the note `aurasLeftOut` ("All aura rings were left out to fit the size limit").
  The helper's own hard limits are 6 per figure and **200** in all: core stops at 150, so the helper never has to cut and prints nothing about it. A ring costs about 45 bytes.
- **Notes.** `TTSNote{Kind: "aurasLeftOut"}` ("N aura rings were left out (limit 6 per figure, 150 in all).") next to the existing `rulesLowered`, `descriptionCut`, `tooLarge`.
- **Summary for the dialog.** `RuleSet.CompanyAuraSummary(d, sel)` / `AuraSummaryFor(d, exp, sel)` and `TTSService.TTSAuras(ctx, campaignID, companyID, live, opts, sel)` (the web interface method `ttsExporter.TTSAuras`):
  the kinds the company has with the rings and radii of the selection's groups, the totals (rings, figures, left out), `Counts` = how many rings each switch (declared, conditional, cast, target, large, optional) adds
  (or removes when it is on) and the notes. The `AuraSelection` also has a switch `Optional` (the Torch's light) that the spec's struct did not have. `DefaultAuraSelection()` reads the defaults of `data.auras`.
- **Tests.** `ttsauras_test.go` (v1 identity, version, kinds, mounts/foot/fallen/creature, base radius, caps, size fallback), `ttsglue_test.go` (`ttsGlueRings`: real `TTSList` + `TTSNames` v2 output of
  all 37 companies, and a Rohan company with banners, a Warlord, a Sorcerer, a Falcon, a rider and a fallen warband, through the Lua helper: one ring line per payload ring on the right figure at `r + base - 0.0225`,
  none on mounts or the fallen, nothing doubled by a second Apply or a Re-apply).
- **Demo.** The campaign "Aura parade" of the demo world has ten companies that carry every aura a company can reach, for the owner's smoke test ([`demo-world.md`](demo-world.md#aura-parade)).

## Level B: the figures get the name and description of their card (web side built, bead wu3.4; the owner's smoke test remains)

Level A works (owner-tested). Level B is the epic `battle-company-manager-ce-v2-wu3`. Facts it rests on (read from the spawner in the saves, see
[How the mod really works](#how-the-mod-really-works-from-the-saved-games)): the spawner gives each figure the **exact line text as Nickname**
(`Warrior of Rohan: Banner`), leaves Description and GMNotes alone, has no text hook, and no other script of the mod reads names (only the dice
injection detector prints `getName()`), so renaming figures is safe. The owner already names and describes figures by hand in the saves
(`[b]Name[/b] (points)`, rich-text stats), which is the target look.

### Parts and flow

1. **The app** (dialog of level A) gets a second block "Names and descriptions": a text area with JSON, a **Copy names** button, and a download link
   "Helper for Tabletop Simulator (install once)". The two texts are generated together from the same switches, so they always agree.
2. **The player in TTS**: pastes the list in the tab `ArmyList` and presses *Spawn Army* (as today); pastes the JSON in a second tab **`ArmyNames`**; drag-selects
   the army (or not) and presses **Apply names** on the helper object. A summary appears: `Named 14 of 14 figures`.
3. **The helper** is a Saved Object with a Lua script, installed once (below).

A second tab, not the same text: the spawner treats every bracket-free line as a hero line, so JSON in `ArmyList` would be spawned as models.

### (1) The data the app sends: `ArmyNames` JSON v1

Generated by core next to the list: `core.TTSNames(d CompanyDetail, exp TTSExport, opts TTSNamesOptions) TTSNamesPayload` (pure, same inputs as `TTSList`;
the web layer passes the export it already built). Compact JSON, no pretty printing:

```json
{"v":1,"company":"The Last Alliance","figures":[
 {"k":"warrior of rohan: banner","l":"Warrior of Rohan: Banner","u":123,"x":"","o":0,"n":"[b]Eadric[/b] (9)","d":"[i]Leader · Path of the Tactician[/i]\n[b]Mv[/b] | [b]FV[/b] | ...\n6\"   | [44BA26]5[-]   | ..."},
 {"k":"fell warg:","l":"Fell Warg: ","u":124,"x":"mount","o":0,"n":"[b]Grub[/b]'s Fell Warg","d":"..."} ]}
```

- **Built** (`internal/core/ttsnames.go`): `(*RuleSet).TTSNames(d, exp, TTSNamesOptions{Rules: "none"|"short"|"full"} (empty or unknown is "none")) TTSNamesPayload` (a method because the mount figures and the glossary come from the rules),
  `TTSNamesPayload.JSON()` (compact, deterministic, no HTML escaping). Fields beyond the sketch below: `l` is the nickname the spawner gives, byte for byte the list line without its
  brackets and count (`Name: Option A, Option B`; the helper matches on it folded, `k` is that fold), `o` the 0-based occurrence among the entries with the same `k` in the whole list
  (the merged `3x` line counts as three; the helper still pairs by grid order within the group). The payload also carries `Rules` (the level used), `Size` and `Notes` (not in the JSON):
  `rulesLowered` (shortened to fit 24 KB), `descriptionCut` (some description lost lines or a rule text to fit 900 characters), `tooLarge` (over 24 KB even without rule texts).
- A creature's figure is `[b]Owner[/b]'s Creature (points)` with the creature's stats and rules; a mount `[b]Owner[/b]'s Fell Warg` with the mount's stats and rules; a rider on foot has
  the unit's description with ` · on foot` in the header and no Mount line (the foot profile is not looked up). A fallen unit of a live list starts its description with `[E03C31][b]Fallen[/b]: wounded or out of the fight[-]`.
- `figures` has **one entry per figure, in spawn order** (the lines of the export in warband order, each line expanded `Count` times; the merged line
  `3x Warrior of Rohan: Shield` gives three entries, one per `UnitIDs` element). `k` is `TTSLine.Key` (lower case, single spaces), `u` the company
  unit id, `x` the extra kind of the line (`""`, `foot`, `mount`), `n` the Nickname, `d` the Description. The occurrence index is implicit: the
  i-th entry with key `k`.
- **Named unit upgrades** (Hearthguard, Helmingas, Lorien Guard, King's Guard...: the options of `data.reservedOptions` and the options a unit alias selects) get no line of
  their own. The upgrade names the unit: a Khazad Guard with Hearthguard is `[b]Hearthguard[/b] (12)` (a company's alias, e.g. Helminga, already is the title; a unit with its own name keeps it),
  in `n` and in the headers of its mount and creatures. What the upgrade adds is in the profile: stat changes in the table in colour (Helmingas: green S 4), granted rules (Hearthguard: Burly)
  in Rules, and a rule with no name of its own (Lorien Guard's re-roll, Tookish Hunter) as a rule under the upgrade's name. The nickname `l` stays the list line (`Khazad Guard: Hearthguard`).
  Gear that is flagged as an upgrade only because it takes no slot (Elven Cloak) is shown under Wargear. `TestTTSNamesUnitUpgrades` checks all of these combinations.
- `n`: `[b]` + unit title + `[/b]` + ` (points)`, as the owner writes it; a promoted hero keeps its own name; a stand-in adds ` ~ as Captain of Dale`;
  an on-foot figure adds ` (on foot)`; a mount `<owner>'s <mount>`. Officer role and path go to the first line of `d`.
- `d` (TTS rich text, the owner's own markup `[RRGGBB]..[-]`, `[b]`, `[i]`), layout "B" tested by the owner in the game, the same for every rule-text level (the level only
  changes the Rules texts):
  1. Header, italic, parts joined with ` · `: the role and the path only (`Leader · Path of the Tactician`, `Warrior`, `Warrior · On Their Own Path`), plus `on foot` for a
     rider on foot; a creature is `Creature of <owner>`, a mount `Mount of <owner>`. **Level and XP are not shown** (no effect during play).
  2. Stat table, two lines: the bold stat names (Mv FV SV S D A W C I), then the values; cells joined with ` | `, each padded with ordinary spaces so the columns line up
     in the proportional tooltip font. Changed values: green `44BA26` better, red `E03C31` worse (inverted for `+` stats). The creature and mount descriptions use the same table.
  3. For heroes the line `[00CCFF]Might 1  Will 1  Fate 1[-]` (unchanged).
  4. Sections, one line each and only when not empty: `Mount:` (class mount), `Wargear:` (weapons, armour, shields: items without a slot), `Equipment:` (items that take a
     slot: banners, herbs...; the same split as the unit card; named unit upgrades are in neither), `Heroic actions:` (only those the path gave, never the ones every hero has),
     `Rules:` (see below), `Injuries:` (red).  A fallen unit of a live list starts with the red `Fallen` line.
  Text from users and rules is made safe: `[` and `]` become `(` and `)` (they would start a colour code), control characters go.
- **Width-aware padding** (`internal/core/ttsfont.go`). The tooltip font is proportional, so each column is as wide as its widest cell and each cell gets
  `round((width - cellWidth) / 5)` spaces (space = 5 px) before the ` | `. The widths are the owner's measurements in the game (pixels): bold M 16, v 10, F 10, V 12, S 10,
  D 13, A 12, W 17, C 10, I 5; regular digits 11, `+` 10, `"` 7, space 5, `|` 6; unknown characters 8 (regular) and 12 (bold); colour codes have no width. To retune, edit the
  tables at the top of `ttsfont.go`. The padding is measured against where the bar should be, not cell by cell, so rounding errors do not add up; a test checks that for all
  stat tables of all 37 companies (units, mounts) the bars of the two lines are at most 3 px apart by the model (the precision measured in the game).  Example tooltip of a
  Rider of Rohan leader (short rule texts):
  ```
  Leader
  Mv | FV | SV | S | D | A | W | C   | I          (names bold)
  6"   | 3   | 4+ | 3 | 5 | 1 | 1   | 7+ | 6+
  Mount: Horse
  Wargear: Hand weapon, Shield, Armour, Bow
  Expert Rider: A Cavalry model with this special rule may re-roll ...
  ```
- **Rule texts** are the card's definitions cut to the first sentence, at most 160 characters (`short`), `none` (names only, **the default** everywhere: the option, the web handler, the dialog's first radio and the `localStorage` default) or `full`; when the
  payload would pass the limit the generator lowers `full`, then `short`, to `none` and says so.
- **Size.** Measured: about 230 to 300 characters of description per figure with the names of the rules only, 380 to 800 with short rule texts (the longest of all units: 899, under the 900 cap), so about 9 KB for 14 figures and 15 KB for 24 (JSON escaping adds about 5%).
  Limit: **24 KB**; above it the generator lowers the rule texts and says so ("Rule texts left out to fit"). The Notebook's own limit is **not
  documented and not tested**: the smoke test pastes 30 KB. Hard cap per description: 900 characters (cut with `...`).
- **Why a Notebook tab, not a button inside the dialog or a web request.** The helper cannot read our page, and a `WebRequest.get` to the app would need a
  public URL carrying company data. The tab keeps the data on the player's machine and the helper small. The text the player pastes is the same text the
  dialog shows, so "Copy names" is one button like "Copy". Alternative rejected: one tab with the list and the names (the spawner would read the JSON).

### (2) How the helper finds the figures

The Saved Object is one locked block on the table with a script (the same way the mod's *Spawn Army* object is built: `createButton` in `onLoad`).

- **Source.** `getNotebookTabs()`; the tab titled `ArmyNames`, preferring the tab whose `color` is the pressing player's (the tab's player colour; the
  spawner itself takes the first `ArmyList`), else the first. Errors by `broadcastToColor` to the presser: tab missing; text not valid JSON ("paste it again; the
  tab may have been cut at N characters"); `v` unknown ("this helper is older than the app; download it again").
- **Candidates.** The figures the player has selected (`Player[color].getSelectedObjects()`, to be verified in TTS) or, if none are selected, every object on the table
  (`getObjects()`), minus the helper itself, minus objects already tagged **`BCNamed`** (idempotency), whose folded Nickname (lower case, single spaces, no
  `\r\n`) equals the `k` of some entry. Selection is the guard against a second army with the same nicknames: select your army, or only yours is named.
- **Identical nicknames.** Entries and figures are grouped by key. Within a group the figures are sorted into **spawn-grid order** and paired with the
  group's entries in list order. The spawner puts figure `i` of a warband at `x = x0 + (i mod 6) * 1.1`, `z = z0 + floor(i / 6) * 1.1 + 1.5` (the hero at `x0, z0`), so
  "ascending z (rows quantised to 0.55), then ascending x" is the spawn order for a figure that has not been moved. Identical figures are interchangeable,
  so a player who moved some before pressing the button only gets the names in another order of identical figures, never a wrong name for a different
  model. This is robust enough that **the app need not stop merging `Nx` lines** (the spawner expands `3x`, and `1x` works too, as level A shows); the
  occurrence index is the position in the group, not a property of the line.
- **Extra figures.** On-foot and mount figures have their own keys (`rider of rohan: banner` foot model, `fell warg:`), so they are matched like any figure; their
  entries come right after the rider's.
- **States.** The spawner names the object in its first state; after a state change (rider to foot) the new state object has the mod's own Nickname
  (`Warrior of Rohan w/Banner`) and loses our name. Version 1 names the **current state only** and says so in the summary ("3 figures have states; changing the
  state shows the mod's name"). TTS has `getStates()` and `setState()`; whether the helper can name every state by cycling them is a smoke test (then a follow-up).
- **Setting.** `setName(n)`, `setDescription(d)`, `setGMNotes("bc-unit:<u>")` (so a later tool can find the figure), `addTag("BCNamed")`. The Nickname is no
  longer the key afterwards, hence the tag.
- **Idempotency.** Pressing *Apply names* again names only untagged figures: "0 new, 14 already named". A second button **Re-apply** ignores the tag and finds
  figures by `GMNotes` (`bc-unit:<u>`, see Built) first, then by Nickname, so changed data (a new injury) can be sent again.
- **Failures** (`broadcastToAll`, orange like the mod's): `Named 13 of 14 figures. Not found: warrior of rohan: banner (1)`; `3 figures named like the list were left alone (not
  selected)`; with no match at all it prints the first three Nicknames on the table to help (the mod may have renamed its figures: "the mod may have changed; tell the app's owner").
- Never `destruct`, never move or clone; the helper only reads objects and sets name, description, notes and a tag.

### (3) Installing the helper

- The app serves `GET /tts/helper.zip` (session-protected, constant content, `Content-Disposition: attachment`; allowed like any download) with two files: `BC names helper.json`
  (a Saved Object) and a 256x256 `BC names helper.png` (generated by Go's `image/png`; every Saved Object on disk has one, whether TTS needs it is a smoke test).
- The player unzips it into the **Saved Objects folder**: macOS `~/Library/Tabletop Simulator/Saves/Saved Objects` (seen on disk), Windows `Documents\My Games\Tabletop Simulator\Saves\Saved Objects`
  (the paths in `SaveFileInfos.json`), Linux `~/.local/share/Tabletop Simulator/Saves/Saved Objects` (from memory, not verified). In TTS: top menu **Objects > Saved Objects**,
  click "BC names helper": it appears at the pointer; put it on the table (it is a small block, its button reads *Apply names*). It can stay in the Saved Objects menu for every game.
- **Format** (from the owner's real Saved Objects): a JSON envelope with `SaveName, Date, VersionNumber, GameMode, GameType, GameComplexity, Tags, Gravity,
  PlayArea, Table, Sky, Note, TabStates, LuaScript, LuaScriptState, XmlUI, ObjectStates[]`; the one object is a built-in `BlockRectangle` (no mesh, no
  assets to host) with `Transform, Nickname, Description (the how-to), ColorDiffuse, Locked, Tooltip` and `LuaScript`. A Go builder writes it from an embedded Lua file
  (`internal/core/ttshelper/helper.lua`, our own code, not derived from the mod's); a test compares the envelope's key set with a fixture of key names.
- **Fallback without the file**: spawn any block (Objects > Components > Blocks), right-click it > Scripting > paste the Lua (a "Show the Lua" link in the dialog)
  > Save & Play. Do it **before** spawning an army (Save & Play reloads the scripts of the game). Unverified.

### (4) Testing without the game

- TTS runs MoonSharp (a Lua 5.2-like interpreter). No Lua is installed on the dev machine; `brew install lua` or a docker image would add an interpreter, but a test that needs it
  would be skipped on most machines. Proposal: run the helper in **gopher-lua** (pure Go, Lua 5.1) inside `go test`, with a **fake TTS API** written in Go: `getNotebookTabs`,
  `getObjects`, `Player[...]`, `broadcastTo*`, `JSON.decode` (backed by `encoding/json`), `self.createButton`, and figure objects with `getName/setName/getDescription/
  setDescription/getPosition/getGMNotes/setGMNotes/addTag/hasTag/getStates`. gopher-lua is a new test-only dependency (`REVIEW`). The helper is written in the common
  subset of 5.1 and 5.2 (no `goto`, no integer division, no bit operators; `table.unpack or unpack`), and a lint test greps for the banned forms.
- Fixtures under `tests/tts/` (payloads and expected outcomes); the optional `tests/tts/run_real_lua.sh` runs the same fixtures with a real `lua` and is skipped when absent.
- Cases: a company spawned by simulating the spawner (the grid formula above) from a real `TTSExport` of three companies, every figure gets the entry of its occurrence;
  identical nicknames; merged lines; extra on-foot and mount figures; the live list's second warband; selection only; an opponent's identical figures left alone; pressing twice; Re-apply;
  a missing figure; an extra figure; bad JSON; unknown version; a tab of another colour.
- **Golden / round trip on the Go side**: `TTSNames` for all 37 companies (the entries of the golden list) must (a) have exactly one entry per figure of the list in order,
  (b) JSON round-trip through `encoding/json` unchanged, (c) stay under the size limit with `short` rule texts, (d) contain no `[` or `]` outside rich-text codes, (e) have descriptions that do not exceed 900 characters.

### (5) What remains for the owner's smoke test

1. Does the file load from Saved Objects without the `.png`, and does *Apply names* appear (a hand-written minimal helper is the first deliverable so this can be tried early)?
2. Does the Notebook tab accept 30 KB? Does `getSelectedObjects()` return the drag selection?
3. After naming, do hover tooltips show the description with its colours? Is the Nickname with `[b]` readable in the object list?
4. State changes: what name shows after switching a Rider of Rohan to its foot state; can the helper cycle states (`setState`) to name each?
5. Two players pasting into tabs with the same title: which tab does the helper read?
6. The current Workshop spawner still names figures with the line text (the same check as level A, point 1).

### (6) Effort, risks, order

- **Effort**: Lua helper with fake-API tests M; core payload M; Saved Object builder and zip S; web (second block, download, switches, size message) S to M; QA S; owner smoke test.
- **Risks**: the mod changes its naming or GUIDs (then the helper reports "not found" and names nothing; it never breaks the army); the Workshop version differs from the saves; the Notebook has an unknown size limit; MoonSharp
  differs from gopher-lua in details (strings, `#`, error text) so the owner's test is the real test; states lose names; two armies with the same figures (selection guard); players unwilling to install a file (level A stays complete without it).
- **Slice order**: (1) the helper script with a hand-made payload and the Saved Object builder, so the owner can try the install and the button early (their smoke tests 1 to 5 need nothing else); (2) the core payload;
  (3) web block and download; (4) QA; (5) states (follow-up if the cycling works).

## What remains to verify (owner, in TTS)

1. **Same spawner?** In a fresh game with the current Workshop version, right-click the *Spawn Army* object, Scripting; check it reads the tab
   `ArmyList` and compares names as described (or save the game and tell us; we diff the script). Also that the single-warband list spawns as one block.
2. **Does the P1 output spawn?** Paste the dialog's list of 3 companies, press Spawn Army, report every `Could not find:` line and the model count message.
3. **States and extra figures:** with a Rider of Rohan, change the figure's state (is it the rider on foot?); with a Mordor Warg Rider, does the extra `Fell Warg` look right?
4. Level B (helper, names and descriptions): see [Level B](#level-b-the-figures-get-the-name-and-description-of-their-card-design-not-built), point 5.

## Decisions of the owner (2026-10-25)

- No model-key catalogue in the repository; fixed mappings in `data.ttsModels` (curator), validator against a local gitignored catalogue only if present.
- Seven stand-ins approved (table above); Warg mount shown as Fell Warg.
- Order of the page; one warband (design above); no cap per warband.
- The button also on other players' companies and in live matches.
- A one-time helper Saved Object install is acceptable (level B, own Lua; woken 2026-10-25 after the owner tested level A, designed in the Level B section).
- A dismounted rider gets a foot model: one figure with states when the bag has them, else extra versions (switch).

## Open questions (remaining)

1. **Q6** One warband (the design) or one per hero? (REVIEW default: one.)
2. **Q7** Live match: list all units, or only those still in the field? (Default: all, with a note per fallen unit.)
3. **Q8** The 6 "some states" units: keep the on-foot line (default), or trust the states and add nothing?
