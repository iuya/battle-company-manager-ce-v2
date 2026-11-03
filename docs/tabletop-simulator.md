# Tabletop Simulator export

This page explains how a Battle Company is taken from this app into Tabletop Simulator (TTS), where many players use the Workshop mod *MESBG FTC - Version 0.4*.
It is for developers; players use the dialog described in [frontend.md](frontend.md#copy-as-a-tabletop-simulator-army-list).

The export has two levels:

- **Army list.**
  A copy button next to the company name copies the whole company as a text list.
  The button is on My Company, another player's company and the live match.
  Each Battle Companies (BC) unit becomes the closest vanilla unit, because the mod can only spawn vanilla models.
  Riders are one line.
  A mount that can fight on its own (Attacks above 0, such as wargs) also gets a figure line.
- **Names and descriptions.**
  A small helper object in TTS gives each spawned figure this app's name and description: BC name, promoted hero name, path, wargear, rules, stats and injuries.
  Optionally it also draws aura rings ([aura-ranges.md](aura-ranges.md)).

What is not verified is how the current Workshop version of the mod reads the list; see [What remains to verify](#what-remains-to-verify).

Contents:

- [How the mod works](#how-the-mod-works)
- [The text format](#the-text-format)
- [Mapping a BC unit to a vanilla unit](#mapping-a-bc-unit-to-a-vanilla-unit)
- [Core](#core)
- [Web](#web)
- [Names and descriptions](#names-and-descriptions)
- [The helper](#the-helper)
- [Why not other approaches](#why-not-other-approaches)
- [What remains to verify](#what-remains-to-verify)

## How the mod works

This section describes what the mod's scripts do.
It was read from saved games that embed them, and it is the ground for the list format.
The mod's own code and model list are not copied into the repository.

1. **Input.** The *Spawn Army* button sits on one table object. It reads the **Notebook tab whose title is exactly `ArmyList`** (`getNotebookTabs()`), else it
   broadcasts `Notebook page 'ArmyList' not found.` The player creates the tab once and pastes into it.
2. **Parsing, line by line.** For each line the script takes the text between the **last `(` and the last `)`**, trims spaces and drops empty lines. A line without brackets is used
   whole. So there is **at most one pair of brackets per line, none inside a name**.
3. **Hero or follower.** A line that matches `^%d+x` is a follower (`count, model = line:match("^(%d+)x%s+(.+)")`) of the last hero line; a follower before any hero line is silently
   dropped. **Any other line is a hero line**, whatever the model is. Warbands spawn in an unspecified order, each as a block of 6 per row, 1.1 inch apart; there is no cap per warband.
4. **Name lookup: exact string, options included.** The remaining text, with any further `(...)` group removed, is compared case-insensitively and as a whole with the `Nickname` of each
   object in one bag (GUID `24f6d0`). The nicknames have the form `Unit name: Option A, Option B`, and `Unit name:` when there are none: exactly the text of a line without its brackets and count.
   So the list does not "choose options": every model in the bag is one name-and-option combination, and the combination must exist. Not found: the orange broadcast `Could not find: <name>`, and the script goes on.
5. **Spawning.** The matching bag object is cloned and **the clone's Nickname becomes the line text** (`Warrior of Rohan: Banner`). Description, GMNotes and tags are left as the bag object has them.
   Afterwards the player sees `N Models | B Break | R Remaining` (N = every figure spawned, B = N/2 + 1, R = N/4).
6. **No hook for text.** There is no suffix, comment or note syntax: extra text becomes part of the lookup key and the model is not found. Naming the figures therefore needs a script of our own, not a richer list.

About the bag:

- It holds about 1760 objects (about 1480 distinct keys).
- Most keys are exactly a vanilla unit name with options that exist in the vanilla file; options are mostly alphabetical.
- A unit whose options are all "choose one" has **no bare model**.
  `Mirkwood Elf Warrior:` does not exist; only `...: Shield`, `...: Elf Bow` and `...: Elven Glaive` do.
- About 490 bag models carry TTS *States* (the mounted rider can be changed to the rider on foot).

Players often name and describe models by hand, with a Nickname such as `[b]Name[/b] (56)` and a Description in TTS rich text (`[FFCC00][b]M[-][/b] 6 | ...`).
The markup is `[RRGGBB]...[-]`, `[b]` and `[i]`.

## The text format

```text
(Hero Name: Option A, Option B)
    (3x Warrior Name: Option)
    (1x Other Warrior: )
```

- A line is `(` + `[Nx ]name: option list` + `)`. Heroes and units without options: `(Name: )`. Leading spaces are optional.
- No bracket inside the text: names and options with brackets (`Troll Brute (Troll)`) cannot be spawned, so brackets are stripped; a few such models will not be found.
- Options are written **sorted, case-insensitively**.
- `includes` of a vanilla unit (siege crew, Beorn's bear) are separate follower lines; `hideInTts` gear is not an option.

## Mapping a BC unit to a vanilla unit

A BC unit is a `CompanyUnit` (`internal/core/company.go`):

- `Type` is a BC name that resolves to a vanilla key or to a company alias;
- `Equipment` holds what it carries;
- `Path` is the hero's path;
- `Parts` are the models of a composite unit.

Over the 37 companies the vocabulary is about 128 unit names:

- most are vanilla names as they are;
- 8 are legacy renames resolved through the company alias;
- 5 are a preselected option (Tookish Hunter, Battlin' Brandybuck, Helminga, Lorien Guard, Noldorin Exile);
- 2 are a vanilla unit plus an invisible rule;
- 19 cavalry units have a vanilla mount;
- wanderers (4) and creatures (3) have no model in the bag and use approved stand-ins.

The implementation computes all of this; nothing is hard-coded from these counts.

### Mapping rules

1. **Name**: `CompanyUnit.Type`, then the alias `unit`, then the vanilla name byte for byte as in the vanilla file, then through the `tts` data below when the mod's model has another name or no 1:1 model exists.
2. **Options**: from `Equipment` and the alias's `selectedOptions`, find the vanilla options of the unit by normalised name, expanding `includes` ("Shield and Spear" = Shield + Spear), at most one per
   `chooseOneKey` / `requireChooseOneKey`, preferring one option that covers several items. Items with no option are dropped and listed as notes. Armour upgrades and BC-only equipment are never options. Options are sorted.
3. **Order and warbands.** Units are written in the order of the My Company page (`core.OrderUnits`: leader, sergeants, other heroes, regular units, or the player's saved order). The export is **one warband**:
   the first unit (the Leader) is the hero line and every other unit follows as a follower line in page order, `1x` each, with identical adjacent units merged (`3x Warrior of Rohan: Shield`). This keeps the
   page order exactly (several warbands would spawn in a random order) and the company spawns as one block the player lifts out.
4. **Riders.** The rider is the mounted model.
   - *One figure with states* is preferred: when the unit's models have these states (`states: all`), the export adds nothing and the note tells the player to change the figure's state to dismount.
   - *Otherwise* two or three versions (a switch in the dialog, default on): the mounted line, a follower line of the on-foot model (`foot` model of the unit, same options when the foot unit has them) and, when
     the mount has **Attacks above 0** in the vanilla `gear` entry (Warg, Fell Warg; Fell beast, Great Eagle), a line for the mount alone. The data decides the Attacks, never a list. Mounts with 0 Attacks
     (horses, camels, goats, ponies) get no figure.
   - State support in the bag varies:
     - states on every key (Rider of Rohan, Son of Eorl, Mirkwood Elf Knight, Hunter Orc Warg Rider; for the last, the states are rider on foot and Fell Warg alone);
     - states on some keys, treated as none, so the on-foot line is added;
     - no states: the on-foot line comes from the table, and the three warg riders also get their Fell Warg line.
   - A foot unit that bought a Warg or Fell Warg gets a mount-alone line.
   - The mod counts every spawned figure, so `N Models | Break | Remaining` is too high by the number of extra figures; the dialog states the real numbers ("14 models + 3 extra figures. Break Point: 7 (50% of 14)").
5. **Notes, not guesses**: every approximation is a note to the player and every stand-in is a "shown as" note.

### The `tts` data in the Battle Companies file

The mod's model-key catalogue is never committed. Everything that has no 1:1 name in the mod is a `tts` property of the unit or item it describes in the lean format (`schemaVersion` 2; see
[definitions.md](definitions.md)), which core reads into the tables the export uses (`LeanTTSModels` in `internal/core/tts_lean.go`, `BCBook.TTSModels()`):

- `gear[].tts {model, alternatives, name}` on a wanderer or creature the mod has no figure of (the stand-in) or on a mount that fights (`Warg` becomes `Fell Warg`). Approved stand-ins: Wandering Swordsman is
  `Captain of Dale`, Wandering Marksman `Ruffian: Bow`, Wandering Scavenger `Goblin Mercenary Captain`, Wandering Sage `Malbeth the Seer`, Baggage Pony `Bill the Pony`, Falcon `Crebain`, Hunting Dog `Fang`.
- `units[].tts {foot, states, options, defaultOptions, alternatives, marker, model}` on a shared unit that only annotates a vanilla one: the on-foot model and whether the mod has dismount states (`all`, `some`, `none`)
  for the cavalry units, spelling differences between a vanilla option and the mod's (`Elven-made glaive` is `Elven Glaive` for the Mirkwood Elf Warrior), and the defaults below.
- The validator (`tools/validate_definitions.py`, `make validate`) checks that every unit, option and model name exists in the vanilla file and, **only if** the gitignored `sources/tts/models.json` exists, that every
  model is a key of it. `tools/extract_tts_models.py` writes that file from a local TTS save (`python3 -I tools/extract_tts_models.py [save.json]`; it refuses to write unless the file is under `sources/tts/` and git ignores it, and keeps only the stripped nicknames).

**Keys the bag does not have.** Three gaps are fixed mappings (`defaultOptions`, `markerOptions`), applied always and each with an `approximate` note: a Lorien Guard with no other option requests the bare
`Galadhrim Warrior:`; a `Mahûd Warrior` with no option requests `Mahûd Warrior: Spear`; a `Rohirrim Traitor` with no option requests `Rohirrim Traitor: Shield`. As a second safety net, when `TTSOptions.Catalogue` is
set, any other line whose key is not in the bag falls back to the closest key (first without the marker options, else the key that keeps most of the carried options, then without ranged weapon or banner, then with
a melee weapon, then the cheapest), with an `approximate` note. `cmd/server` loads the catalogue only when `TTS_CATALOGUE` names a file; the published image has none.

## Core

`internal/core/ttslist.go`: pure functions over a built company (`CompanyDetail`, its roster already in page order) and the rules.

```go
func (r *RuleSet) TTSList(d CompanyDetail, opts TTSOptions) TTSExport
func (r *RuleSet) TTSLiveList(d CompanyDetail, live TTSLive, opts TTSOptions) TTSExport // live match
```

Types:

- `TTSOptions{FootVersions, MountFigures bool; Catalogue TTSCatalogue}`.
- `TTSExport{Text, Warbands []TTSWarband, Notes []TTSNote, Models, ExtraFigures, BreakPercent, BreakPoint string}`.
  `Lines()` flattens the warbands.
  - `Models` counts the figures of units and creatures in the list: a stand-in counts, a unit without a model does not.
  - `ExtraFigures` counts the on-foot and mount figures the switches add.
  - `BreakPoint` is the app's rule (`percent x Models / 100`, one decimal, "6.5") with the company's `BreakPointPercent` or the file's default.
- `TTSLine{UnitIDs, Name, Count, Options, Key, Extra}`.
  `Line.Text(hero)` writes `(Name: A, B)` for the first line and `(3x Name: A, B)` otherwise.
- `TTSNote` kinds: `noModel`, `optionDropped`, `shownAs`, `approximate`, `hasStates`, `extraFigure`, `fallen`.

Mapping follows the rules above:

- Whole options come first; an exchange option is chosen by the item it gives.
- A leftover item goes to the option that holds it (`approximate`) or is dropped (`optionDropped`).
  Armour and equipment slots are dropped silently.
- A required choose-one group that nobody took gets its cheapest option (`approximate`), because the mod has no bare figure.
- Brackets are removed, accents kept, options sorted.

Live lists use `TTSLive{Units, Absent}`.
Units that retreated, fell or are absent form a second warband (`Fallen`) after the main one, with one `fallen` note.
The plain `TTSList` ignores all of it.

Service and web wiring:

- `core.TTSService` (`ttsservice.go`) loads the company and the campaign's `*RuleSet` and calls these functions.
- The web layer declares `ttsExporter` with `TTSExport`, `TTSNames` and `TTSAuras`.
- The `CustomRules` flag belongs to the web layer.
  It marks a campaign whose vanilla or Battle Companies file is not the shipped one, and shows "some unit names may not exist in the mod".

Tests:

- `ttslist_test.go`.
- `TestTTSListAllCompanies` checks every start, reinforcement, special-chart, special and advancement entry of the 37 companies against `testdata/tts_golden.txt`.
  Rewrite the golden with `go test ./internal/core -run TTSListAll -update-tts` and review the diff.
- The local-only `TestTTSListCatalogue` reads `sources/tts/models.json` when it exists.

## Web

`GET /campaigns/{id}/companies/{company}/tts[?live=1&foot=0|1&mounts=0|1&rules=none|short|full&auras=...]` returns an htmx fragment.
It lives in `internal/web/tts.go`, `partials/tts.gohtml` and `static/js/tts.js`.
It is member-only like the company page and sends `Cache-Control: private, no-cache`.

The button's tooltip reads:

> Copy this company as an army list for Tabletop Simulator. Each unit becomes the closest standard MESBG model, because the Tabletop Simulator mod can only spawn standard models. Paste it into a Notebook tab named
> ArmyList and press Spawn Army. Names, paths, injuries and special rules are not carried over.

The dialog is described in [frontend.md](frontend.md#copy-as-a-tabletop-simulator-army-list).
Its title is "Army list for Tabletop Simulator", and it has the switches "Riders on foot" and "Mounts that fight as figures of their own", the copy buttons, the notes and the helper block.

Special cases:

- An empty company has no button.
- A company with no unit that has a model says "None of these units has a standard model".
- In a live match, the list uses the units as they are now, with fallen units in the second warband and a note each.

`GET /tts/helper.zip` (`handleTTSHelper`, behind `protected`) serves the helper download.

Tests: `internal/web/tts_test.go`, `internal/core/ttsservice_test.go`, and flows `91-tts`, `92-tts-names` and `96-tts-auras` ([testing.md](testing.md)).

## Names and descriptions

The spawner gives each figure the exact line text as Nickname and leaves Description and GMNotes alone.
No other script of the mod reads names, so renaming figures is safe.

The flow for a player:

1. In the app's dialog, the second block "Name and describe the figures" has a text area with JSON, a **Copy names** button and a download link for the helper.
   Both texts come from the same switches, so they agree.
2. In TTS, paste the list into the tab `ArmyList` and press *Spawn Army*.
3. Paste the JSON into a second tab named **`ArmyNames`**.
   A second tab is needed because the spawner would treat JSON in `ArmyList` as hero lines.
4. Drag-select the army (or not) and press **Apply names** on the helper object, which is installed once.
5. A summary appears: `Named 14 of 14 figures`.

### The `ArmyNames` payload

`(*RuleSet).TTSNames(d, exp, TTSNamesOptions{Rules, Auras}) TTSNamesPayload` lives in `internal/core/ttsnames.go`.
It is pure, and `TTSNamesPayload.JSON()` is compact and deterministic.
`Rules` is `none` (names only, the default), `short` or `full`; empty or unknown reads as `none`.

```json
{"v":1,"company":"The Last Alliance","figures":[
 {"k":"warrior of rohan: banner","l":"Warrior of Rohan: Banner","u":123,"x":"","o":0,"n":"[b]Eadric[/b] (9)","d":"[i]Leader · Path of the Tactician[/i]\n[b]Mv[/b] | [b]FV[/b] | ...\n6\"   | [44BA26]5[-]   | ..."},
 {"k":"fell warg:","l":"Fell Warg: ","u":124,"x":"mount","o":0,"n":"[b]Grub[/b]'s Fell Warg","d":"..."} ]}
```

`figures` has **one entry per figure, in spawn order**.
That is the lines of the export in warband order, each line expanded `Count` times, so a merged `3x` line gives three entries.

| Key | Meaning |
|---|---|
| `k` | the folded key (lower case, single spaces) |
| `l` | the nickname the spawner gives (the list line without brackets and count) |
| `u` | the company unit id |
| `x` | the extra kind: `""`, `foot` or `mount` |
| `o` | the 0-based occurrence among entries with the same `k` |
| `n` | the Nickname |
| `d` | the Description |

The payload also carries `Rules`, `Size` and `Notes`, which are not in the JSON.
The note kinds are `rulesLowered`, `descriptionCut`, `tooLarge` and `aurasLeftOut`.

#### Nicknames

`n` is `[b]` + unit title + `[/b]` + ` (points)`, with these variations:

- a promoted hero keeps its own name;
- a stand-in adds ` ~ as Captain of Dale`;
- an on-foot figure adds ` (on foot)`;
- a mount is `<owner>'s <mount>`;
- a creature is `[b]Owner[/b]'s Creature (points)`.

**Named unit upgrades** (Hearthguard, Helmingas, Lorien Guard...) get no line of their own and name the unit.
They are the reserved options of the vanilla file and the options a unit alias selects.
A Khazad Guard with Hearthguard is `[b]Hearthguard[/b] (12)`.
Their stat changes and granted rules are in the profile (`TestTTSNamesUnitUpgrades`).

#### Descriptions

`d` is TTS rich text, the same for every rule-text level (the level only changes the Rules texts).
It has these parts:

1. A header in italic, with parts joined by ` · `:
   - role and path (`Leader · Path of the Tactician`, `Warrior`, `Warrior · On Their Own Path`);
   - `on foot` for a rider on foot;
   - `Creature of <owner>` for a creature, `Mount of <owner>` for a mount.

   Level and XP are not shown.
   A fallen unit of a live list starts with the red `Fallen` line.
2. A stat table of two lines: bold names Mv FV SV S D A W C I, then the values.
   Cells are joined with ` | ` and padded with spaces so the columns line up in the proportional tooltip font.
   Better values are green `44BA26` and worse red `E03C31` (inverted for `+` stats).
   Creatures and mounts use the same table.
3. For heroes, `[00CCFF]Might 1  Will 1  Fate 1[-]`.
4. Sections of one line each, only when not empty: `Mount:`, `Wargear:` (items without a slot), `Equipment:` (items that take a slot), `Heroic actions:` (only the path's), `Rules:`, and `Injuries:` (red).

Text from users and rules is made safe: `[` and `]` become `(` and `)`, and control characters go.

**Width-aware padding** (`internal/core/ttsfont.go`):

- Each column is as wide as its widest cell.
- Each cell gets `round((width - cellWidth) / 5)` spaces (a space is 5 px) before the ` | `.
- The widths are measurements in the game: bold M 16, v 10, F 10, V 12, S 10, D 13, A 12, W 17, C 10, I 5; regular digits 11, `+` 10, `"` 7, space 5, `|` 6; unknown characters 8 regular and 12 bold.
- A test checks that for all stat tables of the 37 companies the bars of the two lines are at most 3 px apart by the model.

#### Rule texts and size limits

- Rule texts are the card's definitions cut to the first sentence, at most 160 characters (`short`), or `none`, or `full`.
- The payload limit is **24 KB**.
  That is about 230-300 characters of description per figure with names only, and 380-800 with short texts.
  JSON escaping adds about 5%.
- Above the limit, the generator lowers `full`, then `short`, to `none`, and says so.
- A description is capped at 900 characters (cut with `...`).
- The Notebook's own limit is undocumented; the helper accepts up to 262144 characters.

#### Payload version

`v` is 2 only when at least one aura ring is written.
Otherwise the payload is the version 1 shape byte for byte (`TestTTSNamesV1StaysByteIdentical`).

### Aura rings in the payload

The specification and classification are in [aura-ranges.md](aura-ranges.md).

- `TTSNamesOptions.Auras` is a `core.AuraSelection`.
  The zero value writes none, and `DefaultAuraSelection()` reads the defaults of `data.auras`.
- Per figure: `a:[{"r":3,"k":"banner","w":"a","n":"Banner"}]`.
  The keys are radius in inches, kind id, when (`a` always, `d` declared, `x` conditional, `c` cast, `t` target) and labels joined.
- There is no base size; the helper measures the figure's bounds.
- Rings go to the rider figure of a unit and its on-foot version, and to a creature's own figure.
  They never go to a mount or a unit of the fallen warband, and a bundle gets them only on the first figure.
- Limits, in this order:
  1. **6 rings per figure** (the kinds in data order, then the smaller radius);
  2. **150 in the payload** (the helper's own hard limits are 6 and 200);
  3. the 24 KB rule: rule texts are lowered first, and if the payload with rings is still too big the rings are given up (the payload falls back to version 1) with the note `aurasLeftOut`.

  A ring costs about 45 bytes.
- `RuleSet.CompanyAuraSummary(d, sel)` and `TTSService.TTSAuras(ctx, campaignID, companyID, live, opts, sel)` give the dialog the kinds the company has, the rings and radii, the totals and what each switch adds.
  `AuraSelection` has an `Optional` switch for the Torch.

Tests: `ttsauras_test.go` and `ttsglue_test.go`.
The demo campaign "Aura parade" carries every aura a company can reach ([demo-world.md](demo-world.md#aura-parade)).

## The helper

`internal/core/ttshelper` is pure: no web, no repository.

- `helper.lua` is the script, embedded with `go:embed`.
  It uses the common subset of Lua 5.1 and 5.2 (MoonSharp), so no `goto`, integer division, bit operators or `string.pack`.
- `ttshelper.Script()` returns the script, and `ShowLua()` is the text of the "Show the Lua" fallback.
- `SavedObjectJSON()` is the Saved Object.
  Its envelope has the keys of real Saved Objects.
  The one object is a locked built-in `BlockRectangle` named "BC names helper", with the how-to as Description, the tag `BCHelper` and the script.
- `PNG()` is the 256x256 icon.
- `Zip()` holds `BC names helper.json` and `BC names helper.png` with fixed timestamps.
- `ETag()` serves the download.

### Installing the helper

1. Unzip the download into the Saved Objects folder:
   - macOS: `~/Library/Tabletop Simulator/Saves/Saved Objects`
   - Windows: `Documents\My Games\Tabletop Simulator\Saves\Saved Objects`
   - Linux: `~/.local/share/Tabletop Simulator/Saves/Saved Objects` (not verified)
2. In TTS, open **Objects > Saved Objects** and click "BC names helper".
3. The helper appears at the pointer. It can stay in the menu for every game.

Without the file (unverified): spawn any block, right-click it, choose Scripting, paste the Lua and Save & Play, **before** spawning an army.

### Reading the names

- The tab `ArmyNames` of the pressing player's colour wins; otherwise the first tab is used.
- Errors go to the presser in red:
  - the tab is missing;
  - invalid JSON ("paste it again; the tab may have been cut at N characters");
  - an unknown `v` ("This helper is older than the app (names format N). Download the helper again from the app.").
- The helper reads `v` 1 and 2.

### Finding the figures

Candidates are the figures the player selected or, if none, every object on the table.
The helper itself and objects already tagged `BCNamed` are left out.
A candidate matches when its folded Nickname equals the `k` of some entry.
Folding means lower case, single spaces, and accented Latin-1 capitals folded too.

Identical nicknames are grouped.
Within a group the figures are sorted into **spawn-grid order** (rows by `floor(z / 0.55 + 0.5)`, then x) and paired with the entries in list order.
Identical figures are interchangeable.
So a player who moved some figures only gets the names in another order of identical figures, never a wrong name for a different model.

Extra on-foot and mount figures have their own keys.
Version 1 names the current state only and says so ("N figures have states").

### Setting names

The helper calls `setName`, `setDescription`, `setGMNotes("bc-unit:<u>")` and `addTag("BCNamed")`.
The GM notes get `:foot` or `:mount` for extra figures, plus `/<n>` when figures still share the id.
The script never moves, clones or destroys an object.

### Buttons

- **Apply names** names only untagged figures ("0 new, 14 already named" the second time).
- **Re-apply** ignores the tag and finds figures by GM notes first, then by Nickname, so changed data (a new injury) can be sent again.

The summary goes to everybody:

- green when every figure was named;
- orange with `Not found: key (n)`, `n more figures than names ... were left alone`, or `N figures named like the list were left alone (not selected)`;
- with no match at all, the first three Nicknames on the table.

### Rings

Each ring is one closed line of 48 segments (49 points) set with `Object.setVectorLines` in the figure's local space, so it moves with the figure.

- Radius: `r + base - 0.0225`, where base is `max(size.x, size.z) / 2` of `getBoundsNormalized()` (the mod's own circle measures the same).
- Scale: 1 inch = 1 world unit; x, y, z and the thickness are divided by `getScale().x`.
- Thickness 0.045 (enemy 0.06); local height `0.1 + 0.015 * kind index`; a second ring of the same radius on a figure is 0.03 larger.
- Colours (Okabe-Ito): banner `F0E442`, command `E69F00`, support `56B4E9`, enemy `D55E00`, utility `CC79A7`.

`setVectorLines` replaces all lines.
So the helper reads `getVectorLines()`, drops its own lines (a kind's colour and thickness times the scale), appends the new ones and sets the result.
The mod's own green circle stays.
If the lines cannot be read, the helper overwrites and says so once.

The helper keeps `guid -> rings` and saves it with the game (`onSave`), with the on/off state, the hidden kinds and the payload palette.

- `onLoad` redraws 3 seconds later, because the mod's figure scripts clear their lines at 2 seconds.
- A state change moves the entry to the new GUID.
- A destroyed figure drops its entry.

Buttons are **Rings: on/off**, **Redraw**, and one coloured button per kind (B C S E U).
The message reads `Rings drawn for 14 figures (31 rings).`

### Helper tests

The tests need no game and no `lua` binary.
`internal/core/ttshelper/*_test.go` run the script in gopher-lua (`github.com/yuin/gopher-lua`, test-only) against a fake TTS API (`ttshelper/ttsfake`) and a simulation of the spawner's grid.
The fixtures are `tests/tts/rohan_list.txt`, `tests/tts/rohan_names.json` and `tests/tts/rohan_names_v2.json`.
A lint test bans the unsupported Lua forms and the functions that move, clone, rescale or destroy objects.

`internal/core/ttsglue_test.go` feeds the real `TTSList` and `TTSNames` output of all 37 companies through the helper.
It also covers every unit doubled so lines merge, mount figures, and a live list with a fallen second warband.
It checks that:

- every figure is named exactly once and in the right figure;
- nothing is unmatched;
- a second Apply names nothing;
- Re-apply renames all.

What the fake cannot show (MoonSharp differences, the real API) is checked in the game.

## Why not other approaches

| Alternative | Why not |
|---|---|
| Talk to TTS from the browser | Impossible: TTS listens on a local TCP port (39999, External Editor API); only a local program can connect |
| A local helper using the External Editor API | Requires installing and trusting a program |
| An own spawner that takes models from the bag by GUID | Hard-codes the bag GUID and duplicates the mod |
| An own `gameUrl` JSON for the modular tool | The bag is keyed by vanilla names, so BC names are not found |
| A `WebRequest.get` from the helper to the app | Needs a public URL carrying company data; the Notebook tab keeps the data on the player's machine |
| Info cards as separate objects | More objects to manage than naming the figures |

## What remains to verify

These depend on the game itself, so they are checked by playing:

1. **Same spawner?** In a fresh game with the current Workshop version, right-click the *Spawn Army* object, Scripting; check it reads the tab `ArmyList` and compares names as described.
2. **Does the output spawn?** Paste the dialog's list of three companies, press Spawn Army, and look for `Could not find:` lines and the model count message.
3. **States and extra figures:** change the state of a Rider of Rohan (is it the rider on foot?); check the extra `Fell Warg` of a Mordor Warg Rider.
4. **The helper:** the file loads from Saved Objects without the `.png` and *Apply names* appears; the Notebook tab accepts 30 KB; `getSelectedObjects()` returns the drag selection; the description tooltips show
   their colours; the name shown after a state change; which tab is read when two players use the same title.
5. **Rings:** the scale against the mod's own circle, the wipe at load, the merge with the mod's lines, a state change, pickup/rotate/scale, and the layout of the new buttons on the block face
   ([aura-ranges.md](aura-ranges.md)).

Known risks:

- The mod changes its naming or GUIDs.
  The helper then reports "not found" and names nothing; it never breaks the army.
- The Notebook has an unknown size limit.
- MoonSharp differs from gopher-lua in details (strings, `#`, error text).
- States lose names.
- Two armies have the same figures (the selection guard).
