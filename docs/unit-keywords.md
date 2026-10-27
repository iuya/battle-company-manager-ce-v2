# Unit keywords on the cards (spec)

Status: **built: core (vj0.1-.3) and web (vj0.4/.5: unit cards, live, read-only, armoury, previews, fallen; e2e flow `79-keywords`, live dismount in `84-match-extras`).** The rule-target slices (vj0.6) are built as redefined below ("Rule targets"). Owner request: a unit's card shows only the kind icon (infantry, cavalry, beast, monster, monstrous
cavalry); the race and the other keywords are not shown, but rules such as Hatred (X), Ancient Enemies (X), Fury (X), Terror (X) and
"whilst this model has the Cavalry keyword" only act against, or for, models with a given keyword. Beads: epic `vj0` (children `vj0.1` core computation, `.2` core surfaces, `.3` oracle test, `.4` card row, `.5` other surfaces, `.6` optional rule targets).
This page is the contract between the frontend (chips, tooltips), the backend (`core.UnitKeywords`) and the data. Related:
[`frontend.md`](frontend.md) (cards, tooltips, helper popup), [`concept-graph.md`](concept-graph.md), [`definitions.md`](definitions.md),
[`live-match.md`](live-match.md) (mount removed).

## What the rules say (glossary term `Keywords`)

Each profile has **Race** keywords (Man, Elf, Orc), **Faction** keywords (Gondor, Rohan, Mordor; a model may have several) and **Unit Type**
keywords (Infantry, Cavalry, Monster, Hero...). A rule that names keywords affects models that have **all** of them. **A model made of several
parts (rider and Mount) has separate keywords for each part**, so a rule for Man models affects the rider but not the Mount.

## Where the data lives (investigated)

| Source | What it holds |
|---|---|
| `vanilla data.heroes[]` and `data.warriors[]` | `race[]` (27 values: Man, Dwarf, Orc, Spirit, Hobbit, Elf, Uruk-hai, Goblin, Troll, Warg, Ogre...), `faction[]` (52 spellings; Gondor, Rohan, Mordor...; some are spelling variants: `Khazad-dum`/`Khazad-Dum`, `Lothlorien`/`Lothlórien`, `The Iron Hills`/`Iron Hills`, `Misty Mountains`/`The Misty Mountains`, `Fellowship, Dunedain` is one string for two factions, `*` and `None` are placeholders), `unitType[]` (14 values: Infantry, Hero, Unique, Warrior, Monster, Cavalry, Beast, Elite, War Beast, Swarm, Chariot, siege types, and two stray lowercase `hero`). `core.VanillaUnit` already has `Race`, `Faction`, `UnitTypes`. |
| `vanilla data.gear[type=mount]` | the mount's own `race[]` (Horse, Warg, Camel, Fell Beast...), `faction[]` (Rohan, Gundabad..., or `See Rider`), `unitType[]` (Mount, Chariot, Hero+Mount, Monster). Five mounts have none (`core.VanillaGear` has no such fields yet). |
| BC `data.unitFillIns` | race/faction/unit type for a vanilla unit that has none (Hunter Orc Warrior). Already applied by the loader (`ApplyUnitFillIns`). |
| BC `unitAliases[]` / `specialUnits[]` `addKeywords` | a keyword a company adds to a vanilla unit: the **Goblin Hulk** (Gundabad Ogre + Goblin, Goblin-town). Today **core ignores it** (`UnitAlias` has no such field). 4 occurrences in the file, one unit. |
| BC `effects[kind=unitTypes]` `add`/`remove` | the company's change of unit types (Mirkwood's Warg Marauder: Cavalry added, Infantry removed). Applied (`CompanyProfile.UnitTypes`). |
| BC `data.creatures[]` | `unitType` only (Beast, Infantry, Warrior): no race, no faction. |
| BC `data.wanderers[]` | `unitType` only, and it **contains the race** (`Man, Infantry, Hero`). |
| BC `data.keywords[]` and vanilla `data.keywords[]` | definitions of **special rules**, not of race/faction/type words: `Elf`, `Man`, `Gondor`, `Hero`, `Cavalry` are in neither. |
| glossary terms | `Keywords` (the paragraph above), and unit types `Hero`, `Warrior`, `Infantry`, `Cavalry`, `Beast`, `Elite`, `Unique`, `Swarm`, `Mount`. No term for any race or faction, nor for `Monster` or `Chariot`. |

Facts measured over the 37 companies' 113 distinct unit names: 15 races, 36 factions, types Warrior 110, Infantry 93, Cavalry 18, Elite 10,
Beast 9, Monster 2, Hero 1, Swarm 1. Two used profiles have **no** race, faction or type in vanilla: Hunter Orc Warrior (fixed by the fill-in)
and **Vault Warden Team** (a bundle that `Includes` Iron Shield Bearer and Foe Spear Wielder; its keywords are those of its members, the same
Dwarf/Erebor... in both; the computation takes the first member). Rule parameters that name keywords in the used units: Hatred (Rohan, Angmar,
Goblin, Orc, Uruk-hai, Man, Dwarf, Spirit, Mordor, Dragon), Ancient Enemies (Dwarf, Elf, Orc, and the multi-parameter `(Dwarf)(Elf)`); vanilla
also has `Hatred (Goblin)(Orc)(Uruk-Hai)`. **Factions matter**: Hatred (Rohan), (Angmar) and (Mordor) name factions.
Rules that need the bearer's own keyword exist too: "Whilst this model has the Cavalry keyword" (Slay their Horses!, The Gleaming Horde,
Arise, Riders of Rohan!), so Cavalry on a dismounted rider is rules-relevant. "Evil" and "Good" are **not** keywords in the data (alignment is a
company property, `alignment`), so they are not shown.

## What a unit's keyword set is

Rider keywords = race + faction + unit types of the unit's **current** vanilla profile, then:
1. **Fill-ins** for a profile with none (Hunter Orc Warrior); an included bundle (Vault Warden Team) takes the first member's.
2. **Company alias deltas**: `addKeywords` of the alias the unit was recruited under (Goblin Hulk gets Goblin, in the race group because it is a race word).
3. **Company unit-type changes** (`unitTypes` effect: Warg Marauder is Cavalry, not Infantry).
4. **Promotion to a hero** (level-up advancement or officer): the unit takes the **hero profile** (`CompanyUnit.Type`), so its race, factions and
   types are those of that profile (Warrior becomes Hero in the type list). Nothing is merged with the old profile. A warrior made an officer
   (Heroes in the Making) is shown with the types of the profile it plays as; the role insignia already says leader or sergeant.
5. **Options and wargear** do not add keywords (a selected option such as Tookish Hunter is the same Hobbit). A **bought mount** adds the
   **Cavalry** unit-type keyword to the rider (as `isCavalry` and the cavalry limit already say) and the mount gets its own keyword list (below).
6. **Wanderers** and **temporary wanderers**: the race word is taken out of `unitType` into the race group (Man); creatures have unit types only.

Mount keywords (not merged into the rider's): the mount gear's own race, faction and types; a faction of `See Rider` takes the rider's factions;
an empty mount (five gear entries) shows its name only. A **composite** unit (Warg Marauder: three riders and a Warg) shows the rider keywords
(the Goblin riders) and the part list already on the card; the mount keywords go in the full list.

State: **dismounted** (live `Mounted == false` with `HasMount`): the rider loses **Cavalry** (inactive, shown struck through in the full list
and omitted in the compact row) and the mount block is omitted. A unit whose own profile is Cavalry (Rider of Rohan, Warg Rider) has no mount to lose:
the live state only applies to units with `HasMount`.

## What the card shows

**Compact row** (directly under the name/type line, above the badges; the kind icon stays where it is): chips in this order, race first:
**race**, **factions**, then **notable unit types**. Hidden from the row by default because the icon, the insignia or the type line already say
them: `Hero`, `Warrior`, `Infantry`, `Unique`, `Mount`. Shown: `Cavalry`, `Monster`, `Beast`, `War Beast`, `Elite`, `Swarm`, `Chariot`, siege types. The row
wraps (no truncation, 360 px up) and shows at most **5** chips, then a `+N` chip. Chip look: the existing small neutral badge (`badge badge-muted`,
semantic tokens only), race chip in the normal text weight, faction chips muted, type chips outlined, so the groups are distinguishable without
colour alone; the chip's group is also in its `aria-label` ("Race keyword: Elf"). The row has `aria-label="Keywords"`; it is a `ul`.

**Full list**: tapping the `+N` chip, or any keyword chip, is not a modal of its own: the chip opens the helper popup (below), and `+N` expands the
row in place (`<details>`-like, Alpine `x-show`, no request) to every keyword in three labelled groups: Race, Faction, Unit type (hidden types
included, dismounted Cavalry struck through with "needs a mount" as `aria-label`, as live-match.md does for gear), and, for a mounted unit, a
fourth group "Mount: Horse" with the mount's keywords. On the expanded (maximised) card the row is always open.

**Empty**: a unit with no race/faction/type data (an unknown profile, `ok == false`) draws no row. **Error**: none; the computation cannot fail
(missing parts are skipped), it returns the empty value.

### Tooltips and the helper popup

Every chip is a `has-tip` element like the rule chips (`tabindex="0"`, `data-concept`, `data-tip-title`, `data-tip-meta`, `data-tip-text`; hover
shows the tooltip, a click/tap/Enter opens the helper popup; no inline script, so it fits the CSP):
- `data-tip-title` = the keyword (`Elf`); `data-tip-meta` = `Race keyword`, `Faction keyword` or `Unit type`;
- `data-tip-text` = `UnitKeyword.Text` (core): the glossary definition of the term of the same name when there is one (Cavalry, Beast, Elite,
  Unique, Swarm, Mount...), else a generated sentence: "Elf is a race keyword. Rules that name it (Hatred (Elf), Ancient Enemies (Elf)) apply to
  models that have it." plus, when the **unit itself** has a rule naming one of its keywords (`Hatred (Rohan)` on a Rohan hater), that rule is not
  repeated here (see the optional slice).
- `data-concept` = the keyword when the glossary has a term of that name (the popup shows it and its links), else **`Keywords`** (the glossary
  term that explains race/faction/type keywords). Never an empty `data-concept` for a race or faction: `Resolve` would answer 404 and fall back to
  the tooltip, which is acceptable but loses the explanation. The popup's "Open in glossary" then goes to `Keywords`.
- The concept graph does not need new nodes. (Optional, curator: terms for the races and `Monster`/`Chariot`; filed as a follow-up, not a blocker.)

### Where it appears

| Surface | Source of the data | Behaviour |
|---|---|---|
| My Company card (collapsed and expanded) | `CompanyUnit.Keywords` | row as above; the full list on the expanded card |
| Live company page card (`unit_card` with `LiveUnit`) | `CompanyUnit.Keywords` + `LiveUnit.Mounted` | the state is applied in the web layer with `Keywords.OnFoot()` (pure, no request); a Mount-removed event updates the card through the existing SSE refresh |
| Live compact cards (Retreated, Field casualties, Missed) | `CompanyUnit.Keywords` | the row (REVIEW 9, rule targets): the opponent's units must be readable |
| Live "Wounded a unit" rows | `CompanyUnit.Keywords` | a flat chip list without tips, plus the "applies" markers |
| Other company's read-only page (`RO`) | same as My Company (read-only changes nothing about display) | same row, tooltips and popup |
| Fallen/cemetery cards | `CompanyUnit.Keywords` of the fallen unit | row without the mount block |
| Company browser unit previews (`UnitPreview`) and promotion/chart previews | `UnitPreview.Keywords` | row (no state, no mount) at the top of the hover card, full list always shown (it is a small card) |
| Armoury: wanderers, creatures, special units, reinforcement offers | `Keywords` on `ArmouryWanderer`, `CreatureOffer` and the unit offers | row on the offer card; creatures show unit types only |
| Aftermath / level-up windows that show a unit | none | not shown (they are about the choice) |

## Contract: the Go interface the web layer needs from core

Added to `internal/core` (no I/O, derived from the loaded `RuleSet`; pure functions, cheap to call per card; cached per `(ruleset, company,
unit type)` is allowed but not needed because the data is a few slices):

```go
type KeywordKind string

const (
    KeywordRace    KeywordKind = "race"
    KeywordFaction KeywordKind = "faction"
    KeywordType    KeywordKind = "type"
)

// UnitKeyword is one keyword of a model.
type UnitKeyword struct {
    Name   string      // display spelling: "Elf", "Khazad-dum" (variants merged, see below), "Cavalry"
    Kind   KeywordKind
    Hidden bool        // left out of the compact row (Hero, Warrior, Infantry, Unique, Mount)
    Source string      // "profile", "company" (alias addKeywords, unitTypes effect), "fillIn", "mount"
    Text   string      // tooltip text, already resolved (glossary definition or the generated sentence)
    Concept string     // data-concept value: the glossary term of the same name, else "Keywords"
    Off    bool        // set by OnFoot on Cavalry: shown struck through in the full list, omitted from the row
}

// UnitKeywords is a unit's keyword set: the rider's (or the model's own) in rule order, and the mount's apart.
type UnitKeywords struct {
    Model     []UnitKeyword // race, faction, then unit types, each group sorted as the vanilla file lists them, no duplicates
    Mount     string        // "Horse", "" when there is none
    MountKeys []UnitKeyword // the mount's own keywords (a faction of "See Rider" replaced by the rider's)
}

// Row is the compact row's chips: Model without Hidden and Off, at most max, and how many more there are.
func (k UnitKeywords) Row(max int) (shown []UnitKeyword, more int)
// OnFoot is the set of a rider whose mount is lost: Cavalry marked Off, no mount block.
func (k UnitKeywords) OnFoot() UnitKeywords

// Computation (methods of *RuleSet so the loader's books are used):
func (r *RuleSet) UnitKeywords(p CompanyProfile, in UnitKeywordsInput) UnitKeywords

type UnitKeywordsInput struct {
    Type      string   // CompanyUnit.Type: the vanilla unit name or the company's label
    Role      UnitRole
    Mount     string   // name of the bought mount gear ("" if none)
    Wanderer  string   // wanderer offer name, "" otherwise
    Creature  string   // creature offer name, "" otherwise
}
```

Semantics:
- Resolve `Type` as `resolveUnit` does (vanilla name, else company label alias). Not found: return the empty value (no panic, no error).
- Order: race group, faction group, type group; inside a group the vanilla order, duplicates removed (case-insensitive), spelling variants merged
  (`Khazad-Dum` and `Khazad-dum` once, `Lothlórien` and `Lothlorien`, `The Iron Hills`/`Iron Hills`, `Fellowship, Dunedain` split on the comma,
  `*` and `None` dropped, lowercase `hero` becomes `Hero`). The merge table lives in core next to the computation, not in the JSON (the data
  stays untouched; REVIEW: the curator may later normalise it at the source).
- `Hidden` is true for `Hero`, `Warrior`, `Infantry`, `Unique`, `Mount` (a package constant `hiddenTypes`, one place to change).
- A mount (`Mount != ""`) adds `Cavalry` (Source `mount`) to the model if absent and fills `Mount` / `MountKeys` from `rs.Vanilla.Gear(name)`.
- `Role` does not change the keywords (the profile does); it is in the input so a future rule can use it.
- `CompanyUnit.Keywords UnitKeywords` is filled by `buildUnit` (`companybuild.go`); `UnitPreview.Keywords`, `ArmouryWanderer.Keywords`,
  `CreatureOffer.Keywords` and `LiveUnit` (via `CompanyUnit`) carry it too. The web layer never calls `UnitKeywords` itself.
- `UnitAlias.AddKeywords []string` and the same on special units are read from `addKeywords` by the loader; a race word (one of the vanilla races)
  goes to the race group, anything else to the faction group. The company's keyword limits (`hasKeyword`) use the added keyword too (Goblin Hulk
  counts as a Goblin where a limit names Goblin). REVIEW: today no company limits Goblins, so this changes nothing yet.

## Implemented in core (what the web layer calls)

`internal/core/keywords.go`; tests `keywords_test.go` and the oracle `keywords_oracle_test.go`.
- Filled by core, never computed by the web layer: `CompanyUnit.Keywords` (also the units in `FallenUnit.Unit`; a snapshot stored before the
  feature gets its keywords, without a mount, when the detail is built), `UnitPreview.Keywords` (`CompanyService.CompanyProfiles`; also
  the chart rows, special units and promotion targets are all previews), `ArmouryWanderer.Keywords`, `ArmouryCreature.Keywords` (types only).
- `UnitKeywords.Empty()`, `Row(max)`, `OnFoot()`, and `Group(kind)` (the full list's three groups) are the helpers; `Mount` / `MountKeys` are the fourth group.
- The mount of a unit is the first equipment item of class mount (bought or profile) or the `mount` part of a composite (Warg Marauder: Warg).
- `UnitAlias.Keywords` (from `addKeywords`) is also honoured by the company keyword limits (`resolveUnitKeywords`); the special unit
  (`specialUnits[].addKeywords`) resolves through its alias, so it is not read twice.
- Source `fillIn` marks the keywords of a profile the Battle Companies file filled in (Hunter Orc Warrior).
- Spelling table: `keywordSpellings` in `keywords.go` (Lothlorien is shown as `Lothlórien`, Khazad-Dum as `Khazad-dum`, `Iron Hills`,
  `Misty Mountains`). Data gaps are bead `vo9` for the curator.

## Implemented in the web layer

`keyword_row` and `keyword_chip` in `internal/web/templates/partials/units.gohtml`, helpers in `internal/web/keywords.go` (`keywordRow`,
`notShownKeywords`, `keywordMeta`, `keywordLabel`), styles `.kw-*` in `tailwind.css`, tests `keywords_test.go`, `live_test.go` and the e2e flows
`79-keywords` and `84-match-extras`. Details in [`frontend.md`](frontend.md) (My Company: units).

**Owner decision (REVIEW 1, widened): the obvious keywords are drawn nowhere**, neither in the compact row nor in the full list: Hero and Warrior
(frame, insignia), Mount (the mount has its own display), and the movement types the kind icon says: Infantry, Cavalry, Beast, War Beast, Monster.
Chips are race, factions and the types without another display (Unique, Elite, Swarm, Chariot, siege types). Only the web layer filters; core keeps
computing `Hidden`, `Off`, `OnFoot()` and the mount's keywords (the optional rule-target slice needs them). Consequences: the dismount strike-through
of Cavalry and the "Mount: Horse" group are not drawn (the kind icon is static and does not change on dismount; a lost mount is shown by the existing
"On foot" badge and the struck-through gear); creatures, whose types are all icon types, have no row; the `+N` button only appears for more than five chips,
which the 37 companies' data hardly ever gives (the style guide shows it).

## Acceptance criteria

Backend (core):
- [ ] `UnitKeywords` exists as above; `Row(5)` returns race first, then factions, then notable types, never a hidden or `Off` keyword, and `more`
  counts the rest.
- [ ] The Goblin Hulk of Goblin-town has Goblin (race group) added to the Gundabad Ogre's Ogre/Gundabad/Monster.
- [ ] Warg Marauder of Mirkwood has Cavalry and not Infantry; Hunter Orc Warrior has Orc/Gundabad/Warrior/Infantry; Vault Warden Team takes its first member's.
- [ ] A hero with a bought Horse has Cavalry (Source `mount`) and `Mount == "Horse"` with Horse/Mount keywords (plus the gear's faction) from the gear; `OnFoot()` marks Cavalry `Off`
  and empties the mount; a profile that is itself Cavalry keeps it.
- [ ] A promoted unit shows the hero profile's keywords; wanderers get Man in the race group; creatures have types only.
- [ ] No panic or error for an unknown unit, an empty profile or an unknown mount.
- [ ] **Per-company oracle test**: for every one of the 37 companies, every unit name the company uses (starting company, reinforcement and special
  charts, special units, promotion targets, hero-upgrade `units`, aliases) is built through the real loader and compared with a keyword set
  computed **independently in the test from the raw JSON files** (decoded with `encoding/json` into generic maps, no `core` parsing): vanilla
  `race`/`faction`/`unitType` of the resolved profile, plus `addKeywords`, plus `unitTypes` effects, plus fill-ins, after the same normalisation
  table. Table-driven, one subtest per company, failing with the company and unit name. The test also asserts the **set of races, factions and
  types** over all companies equals the measured one (15 / 36 / the types above), so a data change that adds a keyword is noticed.
- [ ] Mocks regenerated if a consumer interface changes (`make generate`); `make test`.

Frontend:
- [ ] The row appears on the My Company, live company and other-company pages, 360/768/1280 px, light and dark, without horizontal scroll; chips
  wrap; at most 5 and `+N`; tap targets of the chips are at least 44 px through the invisible coarse-pointer area used by the corner tab.
- [ ] The kind icon is unchanged; the row sits under the name/type line and above the badges; a card with no keyword data draws no empty row.
- [ ] Every chip has the tooltip (hover/focus) and opens the helper popup on click, tap and Enter; a race chip's popup is the `Keywords` term; a
  Cavalry chip's popup is the Cavalry term; Escape closes it and focus returns to the chip.
- [ ] `+N` opens the full list in place (three groups, a Mount group for a mounted unit); the expanded card shows it open.
- [ ] Live: removing a mount strikes Cavalry through in the full list, drops it from the row and hides the mount group; re-adding restores it.
- [ ] Armoury offers and company-browser previews show the row; read-only cards show it too and no edit control.
- [ ] Only semantic theme tokens; no inline script or handler; `/styleguide` shows the chip row in its states (race, faction, type, `+N` open, Off).
- [ ] Web tests with the generated mock (row HTML, order, `+N`, escaping, `data-concept` values), and an e2e step in an existing flow (docs/testing.md).
- [ ] `docs/frontend.md` card section updated (this page stays the contract).

## Rule targets: who a keyword rule works against (built, vj0.6)

Rules whose parameter or text names a keyword act on other models, not on the bearer: Hatred (X) and Ancient Enemies (X) against **enemy** models with
X, Terror against every enemy, Fury (X) for friendly models. The first plan (ring the matching chip on the unit's own card) was wrong for them and was
**replaced by the owner's decision** with three parts, all built:

1. **Tooltip wording.** `data.ruleTargets` of the Battle Companies file classifies each rule by direction (`enemy-keyword`, `enemy-any`,
   `friendly-keyword`; a rule not listed is `own`) with a text template (`target`, `{X}` is the keywords). It has `patterns` (the families written with
   brackets, matched on the base name: `Hatred (X)`, `Ancient Enemies (X)`, `Terror (X)`, `Fury (X)`) and `names` (single rules, with the `keywords` their text
   names, `keywordMatch` any/all and `bearerKeywords` the bearer must have). Documented in [`definitions.md`](definitions.md). core reads it
   (`BCBook.RuleTargets()`, `core.RuleTargets.Describe`) and `buildDetail` sets `UnitRule.Works` ("enemy models with the Orc keyword"; several
   bracket alternatives are joined with "or": `Hatred (Goblin)(Orc)(Uruk-Hai)` is "Goblin, Orc or Uruk-Hai"), `UnitRule.Against` (the keyword alternatives
   of an enemy-keyword rule; one bracket with several words needs all of them), `Needs` and `Friends` on the unit's rules and its path's. The rule chip's
   tooltip and the helper popup then say "Works against: ..." (`UnitRule.TipText`, `WorksLine`; web in [`frontend.md`](frontend.md)). A bracket that
   is a number or a distance (`Terror (2)`, `Harbinger of Evil (12")`) is no keyword and falls back to the rule by name. **No highlight on the unit's own card.**
2. **The opponent's keyword chips in the live match**: the compact live cards and the "Wounded a unit" rows show them (the Field cards always did).
3. **"Your Hatred applies" markers** in the live match: `core.RuleMatches(mine, theirs []TargetUnit) map[unitID][]RuleMatch` is a pure function. `TargetUnit`
   comes from `core.NewTargetUnit(unit, liveUnit)`, which only takes a unit **still in the Field**, with `OnFoot()` applied when its mount is lost and
   without the rules granted to the mount. An enemy unit gets a `RuleMatch` for each enemy-keyword rule of your side's units that has an alternative whose
   words it all has (case-insensitive, spellings as in `keywordSpellings`), listing the holders; a rule with `Needs` only counts for a bearer that has one of
   them. Only `enemy-keyword` rules give markers, never `enemy-any` (Terror) or `friendly-keyword`; no table (a custom ruleset) means no `Against`, so no marker
   and plain tooltips. Tests: `ruletargets_test.go` (parsing, descriptions, matching, fields, and one over **all companies' real previews** that checks every
   named keyword exists in the data and that an enemy with it is matched), `internal/web/ruletargets_test.go`, e2e flow `95-keyword-targets`.

The cross-company slice B of the first plan ("a switch that marks...") is replaced by part 3 (no switch: the markers are the live match's, on by default).
Not done: the mount's keywords (a rule naming `Mount`, such as "Slay their Horses!", cannot match a rider), a rule that names a unit's keyword and a distance or a
visibility (the marker says the keyword matches, not that the models are in range).

## REVIEW defaults (owner to confirm)

1. **Decided by the owner:** not shown anywhere (row and full list): Hero, Warrior, Mount, Infantry, Cavalry, Beast, War Beast, Monster (insignia, gear and the kind icon say them). Shown: race, factions, Unique, Elite, Swarm, Chariot, siege types.
2. Row limit 5 chips then `+N`; `+N` expands in place (not the popup).
3. Order race, faction, type (the owner asked for race first).
4. A bought mount makes the rider Cavalry; the mount's keywords are only in the full list and the mount's tooltip, never mixed into the rider's
   (glossary: separate keywords per part).
5. Evil/Good not shown (not keywords in the data).
6. Race and faction spelling variants are merged by a table in core, the JSON is not changed.
7. Races and factions open the glossary term `Keywords` in the popup; terms per race are a later curator task.
8. Goblin Hulk's Goblin keyword is honoured by company keyword limits as well as shown.
9. Aftermath/level-up windows show no keyword row. The live compact cards do (the opponent's units must be readable).
10. Rule targets: every `enemy-keyword` rule of the table marks (not only Hatred and Ancient Enemies), so "Glory Seekers" marks every enemy Hero; the marker
    wording is "<rule> applies"; "Works for" is used for friendly rules.
