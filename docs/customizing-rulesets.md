# Customizing rulesets

How to write your own Battle Companies rules: new rules, items, units, paths, companies and scenarios, or just different numbers. The guide is for people
and for coding agents that help them; it starts with a short section for agents.

- The tutorial (this page) explains the concepts, one worked example each.
- [customizing-rulesets-reference.md](customizing-rulesets-reference.md) lists **every key** with its type, default, allowed values and meaning. It is generated from the schema.
- [examples/customizing-rulesets](../examples/customizing-rulesets/README.md) holds the example files this page quotes. They are complete files, tested on every `make test`.
- [definitions.md](definitions.md) is the engine side: how the app reads and applies the file.

## For agents

**What you are editing.** A ruleset is three JSON files (vanilla, Battle Companies, glossary). Users customise the **Battle Companies file**: one complete
JSON object, `schemaVersion` 2, that the app loads next to the shipped vanilla file and glossary. It is not a patch: a custom file is the whole file.

**The loop.**

1. Read the schema (`definitions/battle_companies/battle_companies.schema.json`) or the [reference](customizing-rulesets-reference.md) for the section you touch.
2. Copy the nearest example from `examples/customizing-rulesets/` (or the nearest entry of `definitions/battle_companies/battle_companies.json`) and edit the copy. Do not write an entry from memory.
3. Check: `make check-ruleset FILE=path/to/file.json`. It prints every problem and exits 1 when there is an error.
4. Fix by error path: `companies[0].start.units[1].unit` is `companies` list, item 0, `start`, `units` item 1, key `unit` (indexes start at 0). Fix that place, not another one.
5. Repeat until `0 errors, 0 warnings`. Then, if the file is the shipped one or a copy of it that is going to ship, `make validate` and `make test`.

**Rules of thumb.**

- Start from a copy of the shipped file for a real campaign (about 460 KB, complete: every company, item, path and scenario of the rulebook). Use `examples/customizing-rulesets/minimal.json` to learn or to build something small.
- A name is the key of its entry. Unit, item, rule, path, scenario and company names must be unique in their list and are compared without case, accents or punctuation.
- Use the same spelling as vanilla for anything vanilla already has (a vanilla unit, option, keyword, item). The error report offers a `closest` name; it is a suggestion, not a fix: check it against the user's intent before taking it.
- Reference a rule **by name** (`"addRules": ["Greenway Lore"]`) and define it once in `rules[]`. Write a rule inline only where the format has a place for it (a path result, a hero upgrade).
- Unknown keys are errors, never ignored, and so are numbers out of range and words outside a closed vocabulary. Do not leave stray keys, comments or placeholder fields in the file. JSON has no comments.
- Charts must cover their dice exactly once: a D6 chart has rows for 1 to 6, no gap, no overlap.
- A change that adds data is one edit to one list. Keep each change small and check after each one.

**Never.**

- Never edit `definitions/vanilla/data2024.json`: it is a third-party store. A change to something vanilla defines is an entry in the Battle Companies file (an annotation, or `"overrides": true`).
- Never invent rules text. Rule, item and scenario texts are copied from the rulebook the user has. If the user has no text, leave the `definition` out and say so, or ask. The texts in the examples are placeholders, marked as such.
- The rulebook is the authority. When it and a file disagree, or the rulebook is ambiguous, show the user both versions and let them choose.
- Never hand-edit the generated files: `battle_companies.schema.json` and `customizing-rulesets-reference.md`.
- Never commit the rulebook, the Battle Companies document or text converted from them. They stay in `sources/`, which git ignores.
- Never guess a name that does not resolve. Report it, offer the `closest` match, and ask.

**A prompt to paste** into an agent session in this repository:

```text
Read docs/customizing-rulesets.md (section "For agents") and examples/customizing-rulesets/README.md.
I want to <add a unit / change the Influence cap / write a company ...>. Work in a copy of
definitions/battle_companies/battle_companies.json called my-ruleset.json (or start from examples/customizing-rulesets/minimal.json).
Copy the nearest example, edit it, run `make check-ruleset FILE=my-ruleset.json`, fix by error path, repeat until 0 errors and 0 warnings.
Do not edit data2024.json, do not invent rule text (ask me for it), do not leave unknown keys. Show me the diff when it is clean.
```

## 1. What a ruleset is

A **ruleset** is three JSON files the app plays a campaign with:

| File | What it holds | You edit it? |
|---|---|---|
| Vanilla (`definitions/vanilla/data2024.json`) | The base game: units, wargear, special rules, magical powers | **Never.** A third-party store |
| Battle Companies (`definitions/battle_companies/battle_companies.json`) | Everything specific to Battle Companies 2nd CE: companies, charts, items, paths, scenarios, the engine parameters | **Yes**: this is the file you customise |
| Glossary (`definitions/glossary/mesbg_glossary.json`) | Short reminders of rulebook concepts (the popups) | Rarely |

The Battle Companies file **extends vanilla**: a unit is a vanilla unit plus a delta, or a brand-new unit that vanilla does not have. A name the file does not define is
looked up in vanilla, then in the glossary. The order when a name is defined more than once is: the entity that embeds the definition, then the Battle Companies
file, then vanilla. Nothing in vanilla or the glossary refers back to the Battle Companies file.

A campaign picks one file of each kind when it is created. Files are told apart by their content (a short hash), so a changed file is a new ruleset and campaigns that
already use the old one are not affected.

**A custom Battle Companies file is a complete file, not a patch.** There is no merging with the shipped file: the app loads exactly the file you upload. That is why
the smallest valid file ([minimal.json](../examples/customizing-rulesets/minimal.json)) still has the whole `campaign` section, and why a real ruleset starts from a copy of the shipped file.

## 2. The file: layout and sections

Top-level keys (the [reference](customizing-rulesets-reference.md#document) has the full table). `schemaVersion`, `name`, `version` and `campaign` are required, and `extends` is needed by the upload; every section after them is optional.

| Key | What it is |
|---|---|
| `schemaVersion` | Must be `2`. A file of another version is refused with one message |
| `name`, `version` | The ruleset's name and version (a string such as `"1.1"`). They label the file on the admin page |
| `extends` | The name of the vanilla file, e.g. `"Middle-Earth Strategy Battle Game - 2024"`. The upload needs it to recognise a Battle Companies file |
| `credit` | Who wrote the rules the file transcribes. Informational |
| `campaign` | Required. The engine parameters and tables: company shape, recruitment, Influence, progression, injuries, experience, the Break Point, rating, scenario selection (section 11) |
| `rules` | Special rules the file defines or annotates (section 5) |
| `units` | Shared units: annotations of vanilla units (section 6) |
| `gear` | Items: wargear, equipment, creatures, wanderers (section 7) |
| `paths` | Heroic paths (section 8) |
| `companies` | The Battle Companies (section 9) |
| `scenarios` | The scenarios (section 10) |
| `limits`, `armourUpgrade`, `creatures`, `wanderers`, `magic`, `auras`, `vanilla` | Company-wide limits, the armour upgrade rule, creature and wanderer counts, magical powers, the vocabulary of aura rings, and what the file says about vanilla things |

The smallest valid file is the header, the `campaign` section and the scenarios the scenario tables name. Every other example is that file plus one concept.

<!-- example: minimal.json -->
```json
{
  "schemaVersion": 2,
  "name": "Minimal example ruleset",
  "version": "1.0",
  "extends": "Middle-Earth Strategy Battle Game - 2024",
  "campaign": {
```

Copy [minimal.json](../examples/customizing-rulesets/minimal.json) to start from nothing, or the shipped file to start from the whole game, then check it:

```
cp definitions/battle_companies/battle_companies.json my-ruleset.json
make check-ruleset FILE=my-ruleset.json
```

## 3. Names, `aka`, `id` and `overrides`

- **A name is the key of its entry.** Everywhere the file refers to something (a chart row to a unit, a hero upgrade to a rule, a scenario table to a scenario), it writes the name.
- **Names compare folded**: case, accents, apostrophes and punctuation do not count (`Lothlorien` is `Lothlórien`, `Lake-Town` is `Lake Town`). Plurals are not folded: `Spear` and `Spears` are different names.
- **Names are unique** within a list (all `gear`, all `rules`, ...). A company's own `units`, `rules` and `gear` form their own scope on top of the shared list.
- **`aka`** lists true synonyms of the same entry. An `aka` that is already the name of something else (a vanilla unit, another item) is an error: it would shadow it.
- **`label`** is how the pages spell a name when the key is plainer. It must be the same name once accents, case and punctuation are folded.
- **`id`** is generated from the name. Leave it out.
- **Annotating or replacing something below.** A name that vanilla or the glossary already defines may be used in two ways. *Annotation*: the entry has **no definition**, it only adds properties (a price, a class, a target, a Tabletop Simulator figure) to the lower thing. *Replacement*: the entry has a definition and `"overrides": true`. A definition without `overrides` for a name that exists below is an error ("exists in vanilla or the glossary; add overrides: true or drop the definition to annotate it"), so nothing shadows another rule silently.

## 4. Three ways to define a thing

The same three forms apply to units, items and rules:

1. **Annotate an existing vanilla thing.** Use its exact name and add properties only. This is how the file gives a vanilla item a price and makes it sold, or a vanilla rule a target.
2. **Extend a thing with a delta.** `"extends": "Ranger of Arnor"` names the base; the entry says what is different. Units use `with`, `addRules`, `addKeywords`; items and companies inherit every field they do not write.
3. **A brand-new thing with no base.** A full definition and a name nothing below uses. Items, rules and units are fully supported; for units see section 6.

## 5. Rules

A special rule is defined **once** in `rules[]` and **named** everywhere else, like vanilla: `addRules`, `grants`, `granted`, hero upgrades and path results all take the rule's name. Naming keeps one text for one rule, and the pages (cards, tooltips, the glossary popups) find it by name.

<!-- example: custom-rule.json -->
```json
"rules": [
  {
    "name": "Greenway Lore",
    "tag": "Passive",
    "source": "Example",
    "definition": "Example text, not a Battle Companies rule: a model with this rule may re-roll a failed Courage test once per game."
  },
```

- `tag` is `Active` or `Passive`; `source` says where the rule is printed; `minor: true` puts it on the minor special rules list a path may offer.
- A rule that **takes an argument** (`Hatred (X)`) declares `parameter` (`kind`: `keyword`, `weapon`, `rollTarget`, `number` or `companyHero`). Using it as `Hatred (Orc)` resolves to the rule `Hatred (X)`.
- `target` says who a rule works against or for, so the unit cards can highlight it: `appliesTo` is `enemy-keyword`, `enemy-any`, `friendly-keyword` or `own` (the default), with `keywords` and `match` (`any`/`all`). A rule whose argument is a keyword also needs `bareTarget` when its bare form means something else (Terror).
- `auras` draw a ring of a fixed radius around the bearer in Tabletop Simulator and on the card (banners, Stand Fast, "within 6" rules):

<!-- example: custom-rule.json -->
```json
{
  "name": "Lantern Bearer",
  "tag": "Passive",
  "source": "Example",
  "definition": "Example text, not a Battle Companies rule: friendly models within 6\" of the bearer ignore the penalties of darkness.",
  "target": {
    "appliesTo": "friendly-keyword",
    "keywords": [
      "Warrior"
    ]
  },
  "auras": [
    {
      "radius": 6,
      "kind": "support",
      "affects": "friendly",
      "when": "always",
      "los": true,
      "tip": "Friendly Warriors within 6\" of the bearer ignore the penalties of darkness."
    }
  ]
},
```

- **Replacing a vanilla rule** takes `overrides`, an **annotation** takes no definition:

<!-- example: custom-rule.json -->
```json
{
  "name": "Burly",
  "overrides": true,
  "source": "Example",
  "definition": "Example text, not a Battle Companies rule: this replaces the vanilla rule of the same name for this ruleset."
},
{
  "name": "Fearless",
  "note": "No definition: an annotation of the vanilla rule. It adds only a note."
},
```

- **By name or inline.** Prefer the name. A few places let you write the definition where you use it (it is moved into `rules[]` when the file is loaded): a hero upgrade's `definition`, a path result `{"rule": {"name": ..., "definition": ...}}`, and a rule defined in one company's own `rules`. An inline rule is a rule like any other: its name must be unique and it is shown the same way.
- A rule name that resolves nowhere (not in `rules[]`, vanilla or the glossary) is an error with the closest name.

## 6. Units

A unit is a vanilla unit plus what the company changes, or a unit vanilla does not have. The three forms all work everywhere in the app (company build, armoury, rating, promotion, level-up, live match, aftermath, Tabletop Simulator list). A name is looked up in this order: **the company's own `units`, then the shared top-level `units[]`, then vanilla**. The three forms:

**a. Annotate a vanilla unit** (top-level `units[]`): the vanilla unit's exact name plus properties. The shipped file uses it for Tabletop Simulator figures:

<!-- example: custom-unit.json -->
```json
{
  "name": "Mahûd Warrior",
  "tts": {
    "defaultOptions": [
      "Spear"
    ]
  }
},
```

**b. Build a unit on another** (`extends`): a unit that a company calls by its own name. Write it in the **company's own `units`** (one company), or in the shared top-level `units[]` (every company of the file may field it). A shared unit nobody fields is valid; it can be used later:

<!-- example: custom-unit.json -->
```json
"units": [
  {
    "name": "Greenway Watcher",
    "extends": "Ranger of Arnor",
    "with": [
      "Spear"
    ],
    "addRules": [
      "Greenway Lore"
    ],
    "addKeywords": [
      "Dunedain"
    ],
    "note": "A Greenway Watcher is a Ranger of Arnor with a spear and the Greenway Lore special rule at no additional cost."
  }
],
```

- `extends` names a vanilla unit (or another unit of the file). `with` pre-selects options of the base unit by their vanilla names (an option the base does not have is an error with the closest option). `addRules` names rules (section 5). `addKeywords` adds keywords. `types` adds or removes unit types, `models` says how many roster slots a recruit counts as, `composite` makes one model that counts as several (the Warg Marauder), `includes` bundles units (the Vault Warden Team), `ownPath` marks On Their Own Path units. `note` is the sentence the card prints.
- The company then **uses the unit by its own name** in the starting force, charts and advancements, never "base unit plus a label":

<!-- example: custom-unit.json -->
```json
"start": {
  "gold": 4,
  "units": [
    {
      "unit": "Greenway Watcher",
      "count": 3
    },
```

- The app turns the unit into its base unit's profile with the options and rules applied. A company's own unit with the same name as a shared one wins; a shared or own unit with the name of a vanilla unit needs `"overrides": true`. The base may itself be a unit of the file (own or shared), a brand-new one included.

**c. A brand-new unit** (no `extends`, a name vanilla does not use): `profile`, `points`, `options`, `definition`. The `profile` is written the way the vanilla file writes a profile: the characteristics by name (`movement`, `fight`, `shoot` (`"-"` for none), `strength`, `defence`, `attack`, `wounds`, `courage`, `intelligence`; heroes also `might`, `will`, `fate`) and the words `race`, `faction`, `unitType` (`Warrior` or `Hero`, `Infantry`, `Cavalry`...), `specialRules`, `wargear`, `heroicActions`, `magicalPowers` and `baseSize`. `race` and `unitType` are required; `specialRules` and `wargear` must name rules and items that exist. `points` is the unit's cost and `options` lists what it may take (`name`, `points`, and `chooseOneKey` / `requireChooseOneKey` for a choose-one group), as in vanilla. The app then treats the unit like a vanilla profile, in the starting force, charts, advancements and promotions. It can be company-local or shared; a shared one with a vanilla name needs `"overrides": true` and replaces the vanilla profile.

<!-- example: custom-unit.json -->
```json
{
  "name": "Barrow Hound",
  "aka": [
    "Hound of the Barrows"
  ],
  "points": 6,
  "profile": {
    "movement": 9,
    "fight": 3,
    "shoot": "-",
    "strength": 3,
    "defence": 4,
    "attack": 1,
    "wounds": 1,
    "courage": 5,
    "intelligence": 3,
    "race": [
      "Hound"
    ],
    "unitType": [
      "Warrior",
      "Infantry"
    ],
    "specialRules": [
      "Greenway Lore"
    ]
  },
  "definition": "Example text, not a Battle Companies rule: a brand-new unit with no base unit; the app treats its profile like a vanilla one.",
  "options": [
    {
      "name": "Spiked collar",
      "points": 2
    }
  ]
}
```

A shared unit that builds on a vanilla one is written the same way as form b, in the top-level `units[]`, and fielded by name:

<!-- example: custom-unit.json -->
```json
{
  "name": "Arnor Scout",
  "aka": [
    "Scout of Arnor"
  ],
  "extends": "Ranger of Arnor",
  "with": [
    "Spear"
  ],
  "addKeywords": [
    "Dunedain"
  ],
  "note": "A shared unit that builds on a vanilla one: any company of the file may field it by this name."
}
```

A unit name that resolves nowhere is an error: `no unit "Greenway Watchr" in this company, the shared units or vanilla; closest: "Greenway Watcher"`.

## 7. Items

Items live in the one `gear[]` list. `kind` is `wargear`, `equipment`, `creature` or `wanderer`; an entry with a `kind` is **sold** in the armoury (unless `unsold: true`); an entry without a `kind` annotates a vanilla item. A company's own `gear[]` is that company's **unique wargear**.

<!-- example: custom-item.json -->
```json
{
  "name": "Traveller's Lantern",
  "kind": "equipment",
  "size": "small",
  "influence": 1,
  "rating": 3,
  "aka": [
    "Lantern"
  ],
  "definition": "Example text, not a Battle Companies rule: the bearer ignores the penalties of darkness."
},
```

- **Cost**: `influence` (what it costs in Influence), `rating` (its points: one number, or `{"warrior": 1, "hero": 5}`), `gold` (default: the rating). `size` is `small` or `large` for equipment (the slot it takes).
- **What it is**: `classes` come from the closed set `melee, spear, two_handed, mount, bow, armour, throwing, shield, instrument, consumable, lance, banner, missile, crossbow, other` (the first is the main class); they are derived from `type` when the vanilla type says them. `ranged {range, strength}` is required for bows, crossbows, throwing weapons and missiles.
- **What it gives**: `grants` names rules the bearer gets; `statChanges` changes characteristics (with a `condition` it is only a note on the card); `grantsSlots {small, large}` adds equipment slots (the Backpack); `auras` draw rings.
- **Who may buy it**: `who` (below) and `limit` (the most models of a company that may carry it).
- **An item built on another**: `extends` inherits every field the item does not write (not `name`, `definition`, `note`, `tts`):

<!-- example: custom-item.json -->
```json
{
  "name": "Greenway Spear",
  "kind": "wargear",
  "extends": "Spear",
  "influence": 1,
  "rating": {
    "warrior": 1,
    "hero": 5
  },
  "definition": "Example text, not a Battle Companies rule: a spear that only Rangers of Arnor may carry, and no more than two to a company.",
  "who": {
    "units": [
      "Ranger of Arnor"
    ],
    "hero": false
  },
  "limit": 2
},
```

- **Equipment slots**: every model has the slots of `campaign.company.equipmentSlots`; an item with `grantsSlots` adds to them:

<!-- example: custom-item.json -->
```json
{
  "name": "Wayfarer's Pack",
  "kind": "equipment",
  "size": "large",
  "influence": 1,
  "rating": 2,
  "definition": "Example text, not a Battle Companies rule: a model with a Wayfarer's Pack may carry two more small pieces of equipment.",
  "grantsSlots": {
    "small": 2
  }
}
```

- **Annotating a vanilla item** gives it a price and sells it. The entry has a `kind` and no `definition`; the item's text stays vanilla's:

<!-- example: custom-item.json -->
```json
{
  "name": "Pitchfork",
  "kind": "wargear",
  "influence": 1,
  "rating": 1,
  "classes": [
    "melee"
  ],
  "note": "No definition: an annotation of the vanilla item. It is now sold for 1 Influence."
}
```

- An item's identity is its **name** and nothing else: two items with the same stats are two items. Related but distinct items go in `related`, true synonyms in `aka`.
- Shields, bows and similar items carry typed `abilities` (`defenceBonus`, `attacksBonus`) that the engine reads; copy them from the shipped items rather than writing new ones. Elven-made weaponry is an item with `base`/`modifiers`/`pricedAs`; see the shipped Elven items.
- `creature` and `wanderer` items carry a free `profile` and, for the shipped ones, `companyFeature` blocks the engine reads: copy from the shipped Baggage Pony or Wanderer.

### Which models: `who`

One selector says "which models" everywhere the file needs it: the items a model may buy (`gear[].who`, `companies[].gear[].who`), the models that may
take a hero upgrade (`heroUpgrades[].who`), the models that get a granted rule (`granted[].who`), the starting units that may be made heroes
(`heroes.who`) and the models of a company that may wear Elven-made weaponry (`elvenWeaponry`). A missing `who` admits every model. Every field narrows
it, all that are set must hold:

| field | admits |
|---|---|
| `hero` | `true`: heroes only; `false`: warriors only |
| `units` | models with one of these exact unit names (the name the company gives the model or the name of its profile); never part of a name |
| `except` | every model but these exact unit names |
| `race` | models of one of these races (`Elf`, `Hobbit`); a model whose profile is unknown is not refused |
| `keywords` | models with one of these keywords: a race, a faction or a unit type of the profile (`Dol Amroth`, `Spider`), or `Cavalry` for a model that rides |
| `alignment` | `good` or `evil`: the alignment of the company |
| `text` | display only: how the page names the models ("All Rohan Hero models"); made from the fields when absent |
| `refusal` | display only: what the armoury says to a model that is not admitted ("Elven-made weaponry is for Elves"); made from the fields when absent |

Unit names, `except` names and races are checked when the file is loaded. `heroes.who` counts only `units` and `except` (the starting units are
chosen by name). `Who.Admits` (`internal/core/lean/who.go`) is the only reader; the refusal it returns is the text the armoury shows.

## 8. Paths

A heroic path is how a hero levels up: a table keyed by a roll, whose results are improvements. A hero rolls the dice of the path (`dice`, default `campaign.progression.hero.roll`) and the table must cover every total exactly once.

<!-- example: custom-path.json -->
```json
"paths": [
  {
    "name": "Path of the Wayfarer",
    "description": "Example path for heroes who live on the road.",
    "dice": "2d6",
```

- `maximums` are the ceilings per characteristic, as in the shipped paths: `max` is the highest value the stat may reach, `maxIncrease` the most points that level-ups may add to the profile, and `best` the best (lowest) value of a stat where lower is better, such as Courage 3+.
- `mastery` names the Mastery rule, `heroicAction` the Heroic Action the path starts with (a name that resolves in `rules[]` or the glossary; `null` when the path adds none), `signature` the characteristics the path is known for, `spells` the magical powers of a path with magic, `limit` the most heroes of a company that may take it.
- `kind` is `heroic` (default, offered to heroes), `custom` (the homebrew path an option hands out) or `endOfPath` (no table, only a `rule`).
- Each row is `{"roll": 7, "options": [...]}`. **An option is exactly one of**: `stat` (with `amount`, `max`, `rollAgain`), `rule` (a name, or an inline definition), `minorRule`, `heroicAction`, `power`, `castingValue`, `oneOf` (the hero picks one of the listed options), or an open choice `anyMinorRule`, `anyHeroicAction`, `anyPower` (with `maxTotal`). Several options in a row are alternatives the player picks from:

<!-- example: custom-path.json -->
```json
{
  "roll": "5-6",
  "options": [
    {
      "stat": "attack"
    },
    {
      "stat": "wounds"
    }
  ]
},
{
  "roll": 7,
  "options": [
    {
      "stat": "strength"
    },
    {
      "stat": "defence"
    }
  ]
},
```

- A row that gives a rule by name needs no `text`: the app writes `Name (Tag) - definition` from the rule. A row that improves only characteristics needs none either. Write `text` only where it cannot be derived; a written text that differs from the derived one is a warning.
- The path picker's summary (the "offers") is derived from the table; do not write it.

An inline rule is moved into `rules[]` when the file is loaded, so its name must be new:

<!-- example: custom-path.json -->
```json
{
  "roll": 10,
  "options": [
    {
      "rule": {
        "name": "Hidden Paths",
        "tag": "Passive",
        "definition": "Example text, not a Battle Companies rule: the hero may re-deploy once per game."
      }
    }
  ]
},
```

The checker's message for a table with a gap: `paths[0].rows: rolls 2..12 need an option each; 7 is missing`.

## 9. Companies

A company is a name, an alignment, a starting force and the two charts a player rolls on, plus whatever makes it different.

<!-- example: custom-company.json -->
```json
{
  "name": "Greenway Wardens",
  "alignment": "good",
  "description": "Example company: rangers who keep the old road open.",
  "oath": {
    "vow": "I swear",
    "text": "Wardens of the Greenway, I take command of this company, called {name}. We keep the road open."
  },
  "start": {
    "gold": 2,
    "units": [
      {
        "unit": "Ranger of Arnor",
        "count": 2,
        "with": [
          "Spear"
        ]
      },
      {
        "unit": "Greenway Watcher",
        "count": 2
      },
      {
        "unit": "Warrior of Arnor",
        "count": 3
      }
    ]
  },
```

- **Header**: `name`, `alignment` (`good` or `evil`, required), `description`, `epigraph {quote, text}`, `oath {vow, text}` (`{name}` once, not at the start or end of the text), `maxModels` and `breakPoint` when the company has its own, `faction` (default: the name), `label`, `aka`.
- **`start`**: `gold` and `units[{unit, count, with, role}]`. A unit is written by the name the company calls it (a vanilla unit or one of the company's `units`). `with` lists options or wargear. The checker does not verify that the force adds up to the 105 points the rulebook asks for of a Battle Company (the shipped file's tests do); it does check `maxModels` and the officers.
- **Charts.** `reinforcements {influence, rows}` and `special {rows, buy}`. Every row has a `roll` (a number, `"2-3"` or `[2, 3]`) and **exactly one result**: `unit` (with `count`, `with`, `rare` for "at most N in the roster", `pick: true` when the player picks the unit's options), `nothing` (`blocks: true` also blocks further rolls), `special` (the roll goes on to the special chart; reinforcement chart only), `choose` or `options`. The rows of a chart must cover the recruiting die (`campaign.recruitment.die`, D6) exactly once.

<!-- example: custom-company.json -->
```json
"reinforcements": {
  "influence": 3,
  "rows": [
    {
      "roll": 1,
      "nothing": true,
      "blocks": true
    },
    {
      "roll": "2-3",
      "unit": "Warrior of Arnor"
    },
    {
      "roll": 4,
      "unit": "Greenway Watcher"
    },
    {
      "roll": 5,
      "choose": "Choice of above",
      "scope": "above"
    },
    {
      "roll": 6,
      "special": true
    }
  ]
},
```

### Chart rows that choose among results: `choose`, `scope`, `picks`

A reinforcement or special chart row that lets the player take a result of the chart has `choose` (the words the page prints), `scope` (required) and
`picks` (how many results the player takes, 1 to 10, default 1). `scope: "above"` offers the units of the rows above the roll on the chart being
rolled; `scope: "any"` offers the units of any row of both charts of the company. The words decide nothing. The row above is the `roll: 5` row: `{"roll": 5, "choose": "Choice of above", "scope": "above"}`.

A row with several units for the player to pick from is an `options` row with `text`:

<!-- example: custom-company.json -->
```json
{
  "roll": 6,
  "text": "Choose one",
  "options": [
    {
      "unit": "Knight of Arnor",
      "rare": 1
    },
    {
      "unit": "Greenway Watcher"
    }
  ]
}
```

### Advancements, hero upgrades, promotions

- **`advancements`**: `{from, with?, to[]}`. A target is a name, or `{unit, with}` when the promotion also gives wargear.
- **`heroUpgrades`**: `{name, flavour?, definition?, who?}`. The `name` is the rule the upgrade gives; either it resolves (vanilla, the glossary, `rules[]`) or the upgrade carries its own `definition`. A hero may swap **any** rolled result of a level-up for its company's upgrade if it does not have it already. `who` restricts which models may take it.
- **`promotions`**: what a unit of the company that rolls *Hero in the Making* (or another progression result) becomes: `{from, on, becomes, statGain?, pathChoice?, forgo?, text}`.

<!-- example: custom-company.json -->
```json
"advancements": [
  {
    "from": "Warrior of Arnor",
    "to": [
      "Knight of Arnor"
    ]
  },
  {
    "from": "Ranger of Arnor",
    "with": [
      "Bow"
    ],
    "to": [
      {
        "unit": "Greenway Watcher",
        "with": [
          "Spear"
        ]
      }
    ]
  }
],
"heroUpgrades": [
  {
    "name": "Road Warden",
    "flavour": "The road is their charge.",
    "definition": "Example text, not a Battle Companies rule: the hero and friendly models within 3\" ignore difficult terrain on the road."
  },
```

### Local units, rules and unique wargear

A company has its own `units` (section 6b), `rules` (rules only it uses) and `gear` (its **unique wargear**, optionally restricted with `who` and `limit`):

<!-- example: custom-company.json -->
```json
"gear": [
  {
    "name": "Warden's Horn",
    "kind": "equipment",
    "size": "small",
    "influence": 2,
    "rating": 8,
    "definition": "Example text, not a Battle Companies rule: a horn that only Greenway Watchers may carry, one to the company.",
    "who": {
      "units": [
        "Greenway Watcher"
      ],
      "hero": false
    },
    "limit": 1
  }
],
```

### Limits, heroes, granted rules and the other typed sentences

Rules that the rulebook prints on a company page are typed fields, so the engine can apply them:

| Key | What it does |
|---|---|
| `limits` | `exempt[{unit \| item, limits[]}]` (a model that does not count towards a limit), `override {limit id: percent}` (a limit's own share for this company), `company[{keyword, percent}]` (no more than that share may have a keyword). The ids are those of the top-level `limits[]` |
| `heroes` | `who` (only `units`/`except` count): the starting units that may be made heroes; `noStartGain`; `recruitAsHero` (every recruit joins as a Hero in the Making) |
| `recruit` | `fallback`, `replace` (a unit that takes an officer's place, or an alternative on a roll), `balance` (a keyword may not outnumber another) |
| `granted` | `{rule, who?, cost?, text}`: a rule the company's models get for free |
| `substitutions`, `exchanges` | A model treated as another for a rule; a rule swapped for another |
| `ownPath` | The units that are On Their Own Path |
| `notes`, `specialRules` | Text of the page: notes placed by section, and the sentences that have no typed form |

<!-- example: custom-company.json -->
```json
"heroes": {
  "who": {
    "units": [
      "Ranger of Arnor",
      "Greenway Watcher"
    ]
  }
},
"limits": {
  "override": {
    "bow": 50
  }
},
```

### Elven-made weaponry for other models: `elvenWeaponry`

The Battle Companies armoury restricts Elven-made weaponry to Elf models unless otherwise noted: the nine Elven items carry `who: {race: [Elf]}` with
the refusal "Elven-made weaponry is for Elves". A company may let other models of its own buy and wear them with `elvenWeaponry`, a `who` that names those
models (an empty object `{}` means every model of the company). No shipped company uses it. The company's rules section then prints a sentence made from the
data, for example "Warrior of Minas Tirith models may buy and wear Elven-made weaponry, which is otherwise restricted to Elf models." (`ElvenWeaponrySentence`,
shown with `CompanyProfile.OwnRules`). The option opens only the race restriction of the Elven items: a company's own unique wargear keeps its own `who`. The Elven items are in the shipped file; the minimal file has none, so in the example the option reads but has nothing to open.

<!-- example: custom-company.json -->
```json
"elvenWeaponry": {
  "units": [
    "Warrior of Arnor"
  ]
},
```

### A company built on another

`extends` copies every field the company does not write from another company of the file (lists and objects are replaced as a whole, not merged); `name`, `faction`, `description`, `epigraph`, `notes` and `specialRules` are never inherited. This is how the Elite companies are made.

<!-- example: custom-company.json -->
```json
{
  "name": "Elite Greenway Wardens",
  "extends": "Greenway Wardens",
  "alignment": "good",
  "faction": "Greenway Wardens",
  "maxModels": 12,
  "description": "Example of a company built on another: it inherits everything it does not write."
}
```

## 10. Scenarios and scoring

A scenario is text sections plus **scoring objectives**, one per printed Victory Point bullet, in printed order. It is played from the scenario tables of `campaign.scenarioSelection`: a first die picks a table, a second picks the scenario in it; the ranges of the tables partition `1..N` of the first die and each table lists exactly N scenario names.

<!-- example: custom-scenario.json -->
```json
"scenarioSelection": {
  "die": "d2",
  "tables": [
    {
      "roll": [
        1,
        1
      ],
      "scenarios": [
        "Seize Ground",
        "Chance Encounter"
      ]
    },
    {
      "roll": [
        2,
        2
      ],
      "scenarios": [
        "Hold the Line",
        "Night on the Greenway"
      ]
    }
  ]
}
```

(The shipped file uses a `d6` first die and a table of six scenarios for each of three ranges. The minimal file uses a `d2` and two scenarios per table to stay small.)

- **Text**: `description`, `outline`, `armies`, `layout`, `startingPositions`, `objectives` (how the game ends), `specialRules` (a list of `{name, definition}`; a rule that prints a table also has `table`), `usesRules` (rules applied by name only), `roles` (`attacker`/`defender` scenarios), `side`.
- **Scoring** objectives have a `key` (unique in the scenario), a `kind` and `options`, the printed sentences in printed order with their Victory Points:
  - `tiers`: the player scores the best tier reached ("instead score" sentences are alternatives, not additions).
  - `each`: a count of items worth a number of points each (`item`, `count`).
  - `eachTiers`: every item (each of 5 Objective Markers) scores its own best tier.
  - `side` (`both`, `attacker`, `defender`) restricts an objective to a role; `secret: true` keeps it hidden until the end, with an `instruction` (what the player notes down) and a `nomination` (`from`: `ownHeroes`, `enemyHeroes` or `terrain`; `allowLeader: onlyHero` only for heroes; `where: opponentHalf` only for terrain).

<!-- example: custom-scenario.json -->
```json
"scoring": [
  {
    "key": "enemyLeader",
    "kind": "tiers",
    "options": [
      {
        "text": "You score 1 Victory Point if the enemy Leader was wounded during the game.",
        "points": 1
      },
      {
        "text": "If the enemy Leader was removed as a casualty, you instead score 3 Victory Points.",
        "points": 3
      }
    ]
  },
  {
    "key": "enemyHeroesSlain",
    "kind": "each",
    "count": 3,
    "item": "enemy Hero removed as a casualty",
    "options": [
      {
        "text": "You score 1 Victory Point for each enemy Hero removed as a casualty, up to 3.",
        "points": 1
      }
    ]
  },
  {
    "key": "markedHero",
    "secret": true,
    "side": "attacker",
    "instruction": "Note down one of your own Heroes before the game.",
    "nomination": {
      "from": "ownHeroes",
      "allowLeader": "onlyHero"
    },
    "options": [
      {
        "text": "You score 2 Victory Points if that Hero is alive at the end of the game.",
        "points": 2
      }
    ]
  }
]
```

A company's Victory Points are the sum of its objectives; the most points wins, equal is a draw. The scenario **rewards** (`rewards[]`, each pointing at a `campaign.aftermathEffects` entry by `effect`) are the typed grants of the War's Aftermath; copy them from a shipped scenario that has the same reward rather than writing new effects, because the engine decides what each effect does.

## 11. Engine parameters

Everything the engine uses as a number, a die or a switch is a key of `campaign` (or of a few other sections), required and range-checked: there is no hidden default in the code. A custom file changes a value by writing a different one. The values below are those of the shipped file.

**Dice.** A dice spec is `[N]dM`: `d6`, `2d6`, `d3`. N is 1 to 10, M is 2 to 100; `d66` is refused. Where a single die is meant (`recruitment.die`, `scenarioSelection.die`) N must be 1. Every table keyed by a roll must cover the totals of its dice exactly once: the checker names the gaps.

| Key | Shipped | Allowed | What it does |
|---|---|---|---|
| `campaign.company.maxModels` | 15 | 1..50 | The most models in a company (a company's own `maxModels` replaces it) |
| `campaign.company.leaders`, `.sergeants` | 1, 2 | 1..3, 0..6 | The officers of a company; their sum may not exceed `maxModels` |
| `campaign.company.heroStart` | might 1, will 1, fate 1 | 0..5 each | What a new hero starts with |
| `campaign.company.startingInfluence` | 0 | 0..100 | The Influence a new company starts with |
| `campaign.company.equipmentSlots` | small 1, large 1 | 0..10 each | The slots of every model before items add any |
| `campaign.recruitment.die` | `d6` | one die | The recruiting die; every company's two charts cover it |
| `campaign.recruitment.adjust.maxPerAttempt` | 3 | 0..20 | **The Influence adjustment cap**: the most points the die may be moved in one recruiting attempt |
| `campaign.recruitment.adjust.direction` | `up` | `up`, `down`, `both` | Which way Influence may move the die |
| `campaign.recruitment.adjust.costPerPoint` | 1 | 0..10 | Influence per point of movement |
| `campaign.recruitment.rare` | injured counts, dead and promoted do not | booleans | Which models count towards a Rare N limit |
| `campaign.influence.gains` | participating 2, draw 1, win 2 | 0..20 each | Influence after a match |
| `campaign.progression.threshold` | 5 | 1..50 | **The XP threshold**: Experience per level-up roll (floor of XP / threshold; nothing is spent) |
| `campaign.progression.warrior.roll` and `.table` | `d6`; no change 1-2, promoted 3-5, Hero in the Making 6 | rows cover the die | The warrior level-up table |
| `campaign.progression.warrior.noPromotion` | courage 1, intelligence 1, maximum 3 | 0..5, 0..5, 1..10 | What a warrior with no promotion gains |
| `campaign.progression.hero.roll`, `.rollsPerProgression`, `.choose` | `2d6`, 2, 1 | 1..6; choose at most rolls | Dice of a path table, results rolled, results kept |
| `campaign.injuries.roll` and `.warrior`, `.hero` | `2d6`; the two charts | rows cover the die | The injury charts |
| `campaign.experience.gains` | seven gains of 1 | key unique, `amount` 0..20 | The ways to earn Experience |
| `campaign.breakPoint.default.percent` | 50 | 1..100 | The Break Point (a company may set its own `breakPoint`) |
| `campaign.breakPoint.threshold.comparison` | `gt` | `gt`, `gte` | `gte` makes exactly the Break Point Broken |
| `campaign.breakPoint.whenBroken.courageTest.dice` | `2d6` | a dice spec | The Courage test of a Broken company |
| `campaign.rating.hero.wargear.threshold` | stats attack, wounds; sum 3 | characteristics; 0..20 | A hero whose listed stats add up to this pays the hero rating for gear |
| `campaign.scenarioSelection.die` and `.tables` | `d6`, three tables | ranges partition the die | How a scenario is rolled |
| `campaign.aftermathEffects[rendezvous \| vantagePoint].range` | [-1, 1] | -6..6 | How far the perk moves a die |
| `magic.floor`, `magic.maxPerPower` | 2, 2 | 1..6 each | A casting value never improves below the floor; improvements per power |
| `creatures.rules.maxPerHero`, `wanderers.rules.maxAtOnce` | 1, 1 | 0..99 | Counts of creatures and wanderers |
| `limits[].maximumPercentage`, `.roundUp` | 33.33, true | 0.01..100 | The bow, throwing-weapon and Cavalry limits |
| `armourUpgrade` | influence 1, rating hero 5 / warrior 1 | 0..100 | Cost of one stage of armour |
| `paths[].limit` | 1 on the Sorcerer | 0..99 | Heroes of a company that may take the path |

The costs of things are properties of the things: `influence` and `rating` on every item, `influence` on creatures and wanderers, `reinforcements.influence` on a company.

A **parameter override** is a change of one of these values. Influence cap 5 instead of 3, an XP threshold of 4, bigger companies and a lower Break Point:

<!-- example: parameter-override.json -->
```json
"recruitment": {
  "die": "d6",
  "adjust": {
    "maxPerAttempt": 5,
    "direction": "up",
    "costPerPoint": 2
  },
```

<!-- example: parameter-override.json -->
```json
"company": {
  "maxModels": 18,
  "leaders": 1,
  "sergeants": 2,
  "heroStart": {
    "might": 1,
    "will": 1,
    "fate": 1
  },
  "startingInfluence": 2,
  "equipmentSlots": {
    "small": 1,
    "large": 1
  }
},
```

In your copy of the shipped file the change is a single value at a path, e.g. `campaign.recruitment.adjust.maxPerAttempt` from `3` to `5`; the test `TestParameterOverrideWorksInTheShippedFile` does exactly that to the shipped file and checks the engine reads 5. Out of range gives `campaign.recruitment.adjust.maxPerAttempt: 40 is out of range 0..20`.

**What the file cannot change.** Some rules are in the engine, not in the file: the injury result keys the app switches on (`dead`, `injured`, `fullRecovery`, ...), how the lower-result rule of the charts works, that the Break Point is never rounded, how the live match applies a Courage test. The tables that carry those keys (`campaign.injuries`, `aftermathEffects`, `characteristicLimits`, `againstTheOdds`, `matchInput`) are read by the app in the shape of the shipped file: change their numbers and texts, copy their structure, and do not invent keys the engine does not know.

## 12. The validation loop and the error report

```
make check-ruleset FILE=my-ruleset.json
make check-ruleset FILE=my-ruleset.json VANILLA=my-vanilla.json GLOSSARY=my-glossary.json   # when you also customise those
make validate     # the shipped files: schemas, cross references, curator checks (the repository's own gate)
make test         # the Go and Python tests, including these examples
```

`make check-ruleset` is the same check the upload runs: it decodes the file strictly (unknown keys, types, ranges, vocabularies), resolves every name against the file, vanilla and the glossary, checks the charts, tables and parameters, and prints **every** problem, not the first. Exit status 0: no errors (warnings are printed); 1: errors; 2: the command could not run.

**Without the command line:** every signed-in player can use the online validator at `/tools/validate` (linked from the new-campaign form: "Not sure your file is valid? Check it first"). Upload your Battle Companies file and, if you wrote your own, the vanilla file and the glossary; an empty vanilla or glossary input means the shipped one. Your Battle Companies file is checked against the vanilla file and glossary you upload there, not against the shipped ones, so a unit or rule that only exists in your own vanilla file or glossary is found. You get the same report as `make check-ruleset`; nothing is stored and no campaign is created. It allows 6 checks a minute per account and files of at most 16 MB; when the checker is busy it answers "try again in a few seconds". The new-campaign form does the same cross check when you upload the files together.

The admin page does the same: **Administration → Rules files**, pick the kind (Battle Companies rules data) and the file, then **Check this file**. It stores nothing and shows the same report. The schema (`battle_companies.schema.json`) gives completion in an editor and catches shape errors early, but the checker is the authority because it also checks names and charts.

**Reading a report.** One problem per line:

```text
<file>: <json path>: <message>[; closest: "<name>"]
  hint: <how to fix it>
```

- `file` is the file as you know it; `warn:` before it marks a warning, which never blocks.
- `json path` is where to look: dots go into objects, `[n]` is a list item counted from 0. `companies[0].start.units[0].unit` is the `unit` key of the first starting-force line of the first company.
- `message` says what is wrong, quoting the value.
- `closest` is the nearest known name when a name does not resolve. It is a suggestion: check it is the thing you meant.
- `hint` says how to fix it; for an unknown key it lists the keys that are allowed at that place.
- The order is the order the file is walked, the same bytes always give the same report, and the list stops at 1,000 problems (`and N more`).

The file [broken/typos.json](../examples/customizing-rulesets/broken/typos.json) has four typical mistakes. This is its report, and what each line tells you:

<!-- report: broken/typos.report.txt -->
```text
typos.json: companies[0].colour: unknown field
  hint: known fields: advancements, aka, alignment, breakPoint, description, elvenWeaponry, epigraph, exchanges, extends, faction, gear, granted, heroUpgrades, heroes, id, label, limits, maxModels, name, notes, oath, ownPath, promotions, recruit, reinforcements, rules, special, specialRules, start, substitutions, units
typos.json: companies[0].units[0].addRules[0]: no rule "Greenway Lor" in rules[], vanilla or the glossary; closest: "Greenway Lore"
  hint: define it in rules[]
typos.json: companies[0].start.units[0].unit: no unit "Greenway Watchr" in this company, the shared units or vanilla; closest: "Greenway Watcher"
typos.json: companies[0].reinforcements.rows: rolls cover 1-5, 6 is missing (a D6 chart must cover 1-6 exactly once)
typos.json: 4 errors, 0 warnings
```

1. `companies[0].colour: unknown field`: a key the format does not have. Remove it (or it is a typo of a real key; the hint lists them all).
2. `addRules[0]: no rule "Greenway Lor"`: a rule name that is defined nowhere. The closest match is right here; fix the spelling, or define the rule in `rules[]`.
3. `start.units[0].unit: no unit "Greenway Watchr"`: the same for a unit.
4. `reinforcements.rows: rolls cover 1-5, 6 is missing`: the chart has no row for a 6.

The messages are executable: `internal/core/lean/testdata/invalid/*.json` with the wording pinned in `*.want`. The common ones: `unknown field` (a key not in the schema), `is not a valid <vocabulary>; known: ...` (a closed word list), `is out of range a..b`, `is already defined at ... names are unique`, `exists in vanilla or the glossary; add "overrides": true or drop the definition to annotate it`, `has no definition here, in Battle Companies, vanilla or the glossary`, `rolls cover ... is missing` (a chart gap), `a row is one of unit, nothing, special, choose and options` (a chart row with two results), `schemaVersion ... is not supported`.

## 13. Uploading and using a ruleset

1. Check the file (section 12) until it reports `0 errors, 0 warnings`.
2. Upload it. On **Administration → Rules files** (superusers), pick the kind **Battle Companies rules data**, choose the file and press **Upload**; **Make default** makes
   it the file new campaigns start with. A file with the same content is stored once; files are told apart by their content (the short code next to the name), so two files
   may share a name and version. A file that fails is refused with the same report and nothing is stored.
3. Or choose it when a campaign is created: the **Rules** section of the new-campaign form offers, for each of the three kinds, the app's default file or **My own file**.
   The form checks and stores the file like the admin page does, and shows the report when it is refused. A campaign keeps the exact files it was created with.
4. A custom file plays with the vanilla file and the glossary chosen next to it. The checker compares names against the **shipped** vanilla file and glossary unless you pass
   `VANILLA=` and `GLOSSARY=`, so a file written for another vanilla file must be checked against that one.
