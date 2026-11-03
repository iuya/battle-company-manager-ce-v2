# Unit keywords on the cards

Every unit card shows the unit's race, faction and unit-type keywords as small chips, and rule tooltips say which models a keyword rule works against.
This page is for developers and rules-data curators: it is the contract between the frontend (chips, tooltips), the backend (`core.UnitKeywords`) and the data.

Why show keywords at all?
A card already has a kind icon (infantry, cavalry, beast, monster, monstrous cavalry).
But rules such as Hatred (X), Ancient Enemies (X), Fury (X), Terror (X) and "whilst this model has the Cavalry keyword" act on, or for, models with a given keyword.

Related: [frontend.md](frontend.md) (cards, tooltips, helper popup), [concept-graph.md](concept-graph.md), [definitions.md](definitions.md), [live-match.md](live-match.md).

Contents:

- [What the rules say](#what-the-rules-say-glossary-term-keywords)
- [Where the data lives](#where-the-data-lives)
- [What a unit's keyword set is](#what-a-units-keyword-set-is)
- [What the card shows](#what-the-card-shows)
- [Contract: the Go interface](#contract-the-go-interface-the-web-layer-gets-from-core)
- [Rule targets](#rule-targets-who-a-keyword-rule-works-against)
- [Defaults](#defaults)

## What the rules say (glossary term `Keywords`)

Each profile has three kinds of keyword:

- **Race** (Man, Elf, Orc).
- **Faction** (Gondor, Rohan, Mordor). A model may have several.
- **Unit Type** (Infantry, Cavalry, Monster, Hero...).

A rule that names keywords affects models that have **all** of them.
A model made of several parts (rider and Mount) has separate keywords for each part.
So a rule for Man models affects the rider but not the Mount.

## Where the data lives

| Source | What it holds |
|---|---|
| vanilla `data.heroes[]` and `data.warriors[]` | `race[]` (27 values), `faction[]` (52 spellings, some of them variants of the same word: `Khazad-dum`/`Khazad-Dum`, `Lothlorien`/`Lothlórien`, `The Iron Hills`/`Iron Hills`, `Misty Mountains`/`The Misty Mountains`; `Fellowship, Dunedain` is one string for two factions; `*` and `None` are placeholders), `unitType[]` (14 values: Infantry, Hero, Unique, Warrior, Monster, Cavalry, Beast, Elite, War Beast, Swarm, Chariot, siege types). `core.VanillaUnit` has `Race`, `Faction`, `UnitTypes` |
| vanilla `data.gear[type=mount]` | the mount's own `race[]` (Horse, Warg, Camel, Fell Beast...), `faction[]` (Rohan, Gundabad..., or `See Rider`) and `unitType[]` (Mount, Chariot, Hero+Mount, Monster); five mounts have none |
| Battle Companies `unitFillIns` | race, faction and unit type for a vanilla unit that has none (Hunter Orc Warrior), applied by the loader |
| Battle Companies unit alias / special unit `addKeywords` | a keyword a company adds to a vanilla unit: the Goblin Hulk (Gundabad Ogre + Goblin, Goblin-town) |
| Battle Companies `unitTypes` company effect | the company's change of unit types (Mirkwood's Warg Marauder: Cavalry added, Infantry removed) |
| creatures and wanderers | creatures have `unitType` only (Beast, Infantry, Warrior); wanderers have `unitType` only and it contains the race (`Man, Infantry, Hero`) |
| glossary terms | `Keywords` (the section above) and the unit types `Hero`, `Warrior`, `Infantry`, `Cavalry`, `Beast`, `Elite`, `Unique`, `Swarm`, `Mount`. No term for any race or faction, nor for `Monster` or `Chariot` |

Over the 37 companies' 113 distinct unit names there are 15 races and 36 factions.

Two used profiles have no race, faction or type in vanilla:

- Hunter Orc Warrior: fixed by the fill-in.
- Vault Warden Team: a bundle that includes Iron Shield Bearer and Foe Spear Wielder; the computation takes the first member's keywords.

Some rules need the bearer's own keyword ("Whilst this model has the Cavalry keyword": Slay their Horses!, The Gleaming Horde, Arise, Riders of Rohan!).
So Cavalry on a dismounted rider matters to the rules.

"Evil" and "Good" are not keywords in the data (alignment is a company property), so they are not shown.

## What a unit's keyword set is

A rider's keywords are the race, faction and unit types of the unit's **current** vanilla profile.
Then, in order:

1. **Fill-ins** for a profile with none; an included bundle takes the first member's.
2. **Company alias deltas**: `addKeywords` of the alias the unit was recruited under (the Goblin Hulk gets Goblin, in the race group because it is a race word). A race word goes to the race group, anything else to the faction group; the company's keyword limits count the added keyword too.
3. **Company unit-type changes** (the `unitTypes` effect).
4. **Promotion to a hero** (level-up advancement or officer): the unit takes the hero profile (`CompanyUnit.Type`), so its race, factions and types are those of that profile; nothing is merged with the old profile.
5. **Options and wargear** do not add keywords. A **bought mount** adds the **Cavalry** unit-type keyword to the rider and the mount gets its own keyword list.
6. **Wanderers** and temporary wanderers: the race word is taken out of `unitType` into the race group (Man); creatures have unit types only.

Mount keywords are not merged into the rider's.
A mount has the gear's own race, faction and types.
A faction of `See Rider` takes the rider's factions, and a mount with no keywords shows its name only.

A **composite** unit (Warg Marauder: three riders and a Warg) shows the rider keywords and the part list already on the card.

When the mount is lost (live `Mounted == false` with `HasMount`), the rider loses **Cavalry** (`OnFoot()`) and the mount block goes.
A unit whose own profile is Cavalry (Rider of Rohan, Warg Rider) has no mount to lose.

## What the card shows

### Compact row

The row sits small in the bottom right corner of the card, below the experience line.
Chips come in this order: **race**, **factions**, then **types with no other display**.

The web layer draws **no chip** for types that the card already shows another way:

- Hero and Warrior: the frame and insignia.
- Mount: the gear.
- Infantry, Cavalry, Beast, War Beast and Monster: the kind icon.

Unique, Elite, Swarm, Chariot and the siege types stay.
Core still computes and flags all of them (`Hidden`, `Off`).

The row wraps (no truncation, 360 px up) and shows at most **5** chips, then a `+N` button.
A unit with no chip draws no row.

Markup and accessibility:

- The row is a `ul` with `aria-label="Keywords"`; chips are `kw-chip` list items.
- Three shapes keep the groups apart without colour: race solid and bold (`kw-race`), faction muted (`kw-faction`), unit type outlined (`kw-type`).
- Each chip's `aria-label` names its group ("Race keyword: Elf").

### Full list

`+N` expands the row in place (Alpine `x-show`, no request, `aria-expanded`).
It shows every keyword in three labelled groups: Race, Faction and Unit type.
"Fewer keywords" closes it.
On the expanded (maximised) card the full list is always shown.

A lost mount changes nothing here: the kind icon is static, and the "On foot" badge and struck-through gear say it.
Creatures, whose types are all icon types, have no row.

### Tooltips and the helper popup

Every chip is a `has-tip` element like the rule chips (`tabindex="0"`, `data-concept`, `data-tip-title`, `data-tip-meta`, `data-tip-text`).
Hover shows the tooltip; a click, tap or Enter opens the helper popup.
There is no inline script.

- `data-tip-title` is the keyword (`Elf`); `data-tip-meta` is `Race keyword`, `Faction keyword` or `Unit type`;
- `data-tip-text` is `UnitKeyword.Text`: the glossary definition of the term of the same name when there is one (Cavalry, Beast, Elite, Unique, Swarm, Mount...), else a generated sentence: "Elf is a race keyword.
  Rules that name it (Hatred (Elf), Ancient Enemies (Elf)) apply to models that have it.";
- `data-concept` is the keyword when the glossary has a term of that name, else **`Keywords`**, so a race or faction chip always opens an explanation and "Open in glossary" goes to `Keywords`.

### Where it appears

| Surface | Source of the data | Behaviour |
|---|---|---|
| My Company card (collapsed and expanded) | `CompanyUnit.Keywords` | row as above; the full list on the expanded card |
| Live company page card | `CompanyUnit.Keywords` + `LiveUnit.Mounted` | the state is applied in the web layer with `Keywords.OnFoot()` |
| Live compact cards (Retreated, Field casualties, Missed) | `CompanyUnit.Keywords` | the row: the opponent's units must be readable |
| Live "Wounded a unit" rows | `CompanyUnit.Keywords` | a flat chip list without tips, plus the "applies" markers |
| Another company's read-only page, fallen cards | same | same row, tooltips and popup |
| Company browser unit previews and chart or promotion previews | `UnitPreview.Keywords` | the full list at the top of the hover card, plain, always open, non-interactive (`Static`) |
| Armoury: wanderers, creatures, special units, reinforcement offers | `Keywords` on `ArmouryWanderer`, `ArmouryCreature` and the unit offers | row on the offer card; creatures show unit types only |
| Aftermath and level-up windows | none | not shown (they are about the choice) |

Chips get the invisible 44 px coarse-pointer area of the card's corner tab. `/styleguide` shows the states.

## Contract: the Go interface the web layer gets from core

`internal/core/keywords.go` has no I/O.
Everything is derived from the loaded `RuleSet` with pure functions, so it is cheap to call per card.
Tests: `keywords_test.go` and the oracle `keywords_oracle_test.go`.

```go
type KeywordKind string // "race" | "faction" | "type"

type UnitKeyword struct {
    Name    string // display spelling, variants merged
    Kind    KeywordKind
    Hidden  bool   // left out of the compact row (Hero, Warrior, Infantry, Unique, Mount)
    Source  string // "profile", "company" (alias addKeywords, unitTypes effect), "fillIn", "mount"
    Text    string // tooltip text, already resolved
    Concept string // data-concept value
    Off     bool   // set by OnFoot on Cavalry
}

type UnitKeywords struct {
    Model     []UnitKeyword // race, faction, then unit types, each group in the vanilla order, no duplicates
    Mount     string        // "Horse", "" when there is none
    MountKeys []UnitKeyword // the mount's own keywords (a faction of "See Rider" replaced by the rider's)
}
func (k UnitKeywords) Row(max int) (shown []UnitKeyword, more int)
func (k UnitKeywords) OnFoot() UnitKeywords
func (k UnitKeywords) Empty() bool
func (k UnitKeywords) Group(kind KeywordKind) []UnitKeyword

func (r *RuleSet) UnitKeywords(p CompanyProfile, in UnitKeywordsInput) UnitKeywords
type UnitKeywordsInput struct{ Type string; Role UnitRole; Mount, Wanderer, Creature string }
```

Behaviour:

- **`Type`** resolves as `resolveUnit` does: the company's label alias or own brand-new unit, the shared units, then the vanilla name.
  When nothing is found the result is the empty value (no panic, no error).
- **Order** is race group, faction group, type group.
  Inside a group the vanilla order is kept and duplicates are removed (case-insensitive).
- **Spelling variants** are merged by the table `keywordSpellings` in core:
  - `Lothlórien`, `Khazad-dum`, `Iron Hills` and `Misty Mountains` take one spelling each;
  - `Fellowship, Dunedain` is split on the comma;
  - `*` and `None` are dropped;
  - lowercase `hero` becomes `Hero`.

  The data stays untouched.
- **`Hidden`** is true for `Hero`, `Warrior`, `Infantry`, `Unique` and `Mount` (the package constant `hiddenTypes`).
  This is core's default for the compact row; the web layer applies its own list, described under [Compact row](#compact-row).
- **A mount** adds `Cavalry` (Source `mount`) to the model if absent and fills `Mount` and `MountKeys` from the vanilla gear.
  The unit's mount is the first equipment item of class mount (bought or from the profile), or the `mount` part of a composite.
- **`Role`** does not change the keywords; the profile does.

Core fills the keywords; the web layer never computes them:

- `CompanyUnit.Keywords`, including the units in `FallenUnit.Unit`.
  A snapshot stored earlier gets its keywords, without a mount, when the detail is built.
- `UnitPreview.Keywords` (`CompanyService.CompanyProfiles`).
  Chart rows, special units and promotion targets are all previews.
- `ArmouryWanderer.Keywords` and `ArmouryCreature.Keywords`.

The web layer:

- templates `keyword_row` and `keyword_chip` in `partials/units.gohtml`;
- helpers in `internal/web/keywords.go` (`keywordRow`, `notShownKeywords`, `keywordMeta`, `keywordLabel`);
- styles `.kw-*` in `tailwind.css`;
- tests `keywords_test.go` and `live_test.go`, and the e2e flows `79-keywords` and `84-match-extras`.

The card layout is in [frontend.md](frontend.md).

### Oracle test

For every one of the 37 companies, the test builds every unit name the company uses through the real loader.
That covers the starting company, reinforcement and special charts, special units, promotion targets, hero-upgrade units and aliases.

Each result is compared with a keyword set computed independently from the raw JSON files:

- the vanilla `race`, `faction` and `unitType` of the resolved profile;
- plus `addKeywords`, `unitTypes` effects and fill-ins;
- after the same normalisation table.

The test also asserts that the races, factions and types over all companies equal the measured set, so a data change that adds a keyword is noticed.

## Rule targets: who a keyword rule works against

Rules whose parameter or text names a keyword act on other models, not on the bearer:

- Hatred (X) and Ancient Enemies (X) work against **enemy** models with X.
- Terror works against every enemy.
- Fury (X) works for friendly models.

So nothing is highlighted on the unit's own card.
Instead, there are three mechanisms.

### 1. Tooltip wording

`data.ruleTargets` of the Battle Companies file classifies each rule by direction:

- `enemy-keyword`, `enemy-any` or `friendly-keyword`;
- a rule that is not listed is `own`.

Each entry has a text template (`target`; `{X}` stands for the keywords).
The table has two parts (see [definitions.md](definitions.md)):

- `patterns`: families written with brackets, matched on the base name (`Hatred (X)`, `Ancient Enemies (X)`, `Terror (X)`, `Fury (X)`).
- `names`: single rules, with the `keywords` their text names, `keywordMatch` (any or all) and the `bearerKeywords` the bearer must have.

Core reads the table (`BCBook.RuleTargets()`, `core.RuleTargets.Describe`).
`buildDetail` then sets these fields:

- `UnitRule.Works`: for example "enemy models with the Orc keyword".
  Several bracket alternatives are joined with "or": `Hatred (Goblin)(Orc)(Uruk-Hai)` reads "Goblin, Orc or Uruk-Hai".
- `UnitRule.Against`: the keyword alternatives of an enemy-keyword rule.
  One bracket with several words needs all of them.
- `Needs` and `Friends`.

The rule chip's tooltip and the helper popup then say "Works against: ..." (`UnitRule.TipText`, `WorksLine`).
A bracket that is a number or a distance (`Terror (2)`, `Harbinger of Evil (12")`) is no keyword and falls back to the rule by name.
A ruleset without the table has plain tooltips.

### 2. The opponent's keyword chips in the live match

The Field cards, the compact live cards and the "Wounded a unit" rows show them.

### 3. "Your Hatred applies" markers in the live match

`core.RuleMatches(mine, theirs []TargetUnit) map[unitID][]RuleMatch` is a pure function.
`TargetUnit` comes from `core.NewTargetUnit(unit, liveUnit)`.
It takes only a unit **still in the Field**, applies `OnFoot()` when the mount is lost, and leaves out the rules granted to the mount.

An enemy unit gets a `RuleMatch` for each enemy-keyword rule of your side's units that has an alternative whose words it all has.
Matching is case-insensitive, with spellings as in `keywordSpellings`.
The match lists the holders.
A rule with `Needs` only counts for a bearer that has one of them.

Only `enemy-keyword` rules give markers, never `enemy-any` (Terror) or `friendly-keyword`.
Every `enemy-keyword` rule of the table marks, so "Glory Seekers" marks every enemy Hero.
The marker reads "<rule> applies"; "Works for" is used for friendly rules.
There is no switch: the markers belong to the live match.

### Tests and limits

Tests: `ruletargets_test.go`, `internal/web/ruletargets_test.go` and e2e flow `95-keyword-targets`.
The core test covers parsing, descriptions, matching and fields.
One test runs over all companies' real previews and checks that every named keyword exists in the data and that an enemy with it is matched.

Not covered:

- The mount's keywords: a rule naming `Mount`, such as "Slay their Horses!", cannot match a rider.
- A rule that names a unit's keyword together with a distance or a visibility: the marker says the keyword matches, not that the models are in range.

## Defaults

1. Not shown anywhere: Hero, Warrior, Mount, Infantry, Cavalry, Beast, War Beast, Monster (insignia, gear and the kind icon say them). Shown: race, factions, Unique, Elite, Swarm, Chariot, siege types.
2. Row limit 5 chips then `+N`; `+N` expands in place (not the popup). Order race, faction, type.
3. A bought mount makes the rider Cavalry; the mount's keywords stay apart from the rider's (the glossary says separate keywords per part).
4. Race and faction spelling variants are merged by a table in core; the JSON is not changed.
5. Races and factions open the glossary term `Keywords` in the popup.
6. Aftermath and level-up windows show no keyword row; the live compact cards do.
