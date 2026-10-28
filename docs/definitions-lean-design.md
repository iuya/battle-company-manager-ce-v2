# Lean definitions format: target design

Status: **ready for implementation.** Every question is decided by the owner (2026-10-27, decisions 1 to 14 below); the slice 0
reader verdicts are in section 9, the engine parameters in section 12, the slices in section 11. Inputs: [`bcjson-audit-curator.md`](bcjson-audit-curator.md) (the
data side), [`bcjson-audit-backend.md`](bcjson-audit-backend.md) (the Go side), [`engine-parameters-audit.md`](engine-parameters-audit.md)
(the rule values hard-coded in Go and templates), [`definitions.md`](definitions.md) (the current format). Bead `battle-company-manager-ce-v2-msl`.

Owner decisions that bound the design:

1. **Clean break.** No old-format loader survives; the format version is bumped; development data is wiped and re-seeded.
2. **No loss of feature or content.** Content that leaves the loaded ruleset goes to a documented place (`reference/`,
   `tools/data/`, `docs/`), it is not deleted. The one exception is decision 14: rulebook prose that no code reads or shows
   (`recruitment`, `customCompany`) is dropped.
3. The Battle Companies prose is free to keep in the committed JSON; copies are removed anyway.
4. `vanilla/data2024.json` is a read-only standard.
5. **Display names stay stable.** Postgres stores names (unit base names, gear, paths, company names, `Progress` keys);
   the loader resolves by name, so no database migration.
6. Goal: **lean, and easy for people to add their own companies, units, equipment and paths.**
7. **Three uploaded files.** A user uploads the vanilla file, the Battle Companies file and the glossary. The Battle Companies
   ruleset therefore stays **one file**: no folder, zip or manifest exists for users. Their own companies are the shipped file
   plus additions, uploaded as their Battle Companies file.
8. **Rules are referenced by name**, as the vanilla file does. An inline definition where a rule is granted is allowed but is
   not the norm.
9. **Unknown keys are errors.**
10. **An upload is all or nothing.** It is validated and every error is returned to the user (file, path, hint, closest
    match); nothing is saved and no campaign is created.
11. **Demo data and tests are regenerated, not dropped.** Development data is wiped and re-seeded, but every demo campaign
    and test case (`cmd/server/world*.go`, e2e fixtures, `tests/tts`, inline JSON in Go and Python tests) is rewritten in the new
    syntax with no loss of coverage (section 11, slice 0 and slice 9).
12. **Every rule parameter of the engine takes its value from the Battle Companies json** (section 12): a number, a dice
    specification, a threshold, a limit, a direction. A custom ruleset can change any of them (example: Influence may also lower
    a roll, at 2 per point, and move up to 5 points per attempt instead of 3). The shipped file carries today's values, so the
    golden snapshot stays byte-identical. The **13 fixed-choice mechanics** (section 12.3) stay in code only and are documented
    as "the engine implements these rules"; no switch for them exists in the json. Engine safety limits (input sizes, timeouts,
    retries; section 9 of the audit) also stay in code.
13. **The starting totals (105, 106, 60) are not rules.** No json field, no per-company override and no validator warning
    carries them; they remain a test-only invariant of the shipped companies (`internal/core/startingtotal_test.go`, with the
    Defenders of the North exception). Leaders, sergeants and the hero start are parameters.
14. **Prose.** Parameters stay in the json and are read. The explanatory prose of `campaign.recruitment` and `campaign.customCompany`
    is dropped (players read it in the real rulebook; the generator source still has it); `campaign.matchInput` prose is kept and
    must be shown to players (section 4.8). `recruitment.adjust`, `recruitment.rare` and `influence.spend[adjustRoll]` stay in
    `campaign` and are read. **Annotations** (an entry that adds properties to a vanilla or glossary thing), **`extends`** and
    **brand-new units or items with no base** (a full definition) are all allowed.

## 1. Principles

- **One file, several sections.** The Battle Companies ruleset is one document
  `{ "schemaVersion": 2, "name", "version", "credit", "extends", <the sections below> }`. The loader decodes it once into a
  typed in-memory model (shared by content hash, as today). What the database stores for a campaign is that file, as today.
  There is no merging of documents.
- **One home per fact.** A fact is written where the thing it describes is written (a hero upgrade's text in the
  company that has it, a rule's aura on the rule, a unit's TTS figure on the unit). Indexes, tables and summaries are
  derived at load.
- **Names are the keys.** Within a kind (company, unit, gear, rule, path, scenario) the `name` is unique, compared case
  and punctuation insensitively; `aka` lists other spellings that resolve to the same entity (a 2018 name, `Lake-Town`,
  `Armoured Horse`/`Goat`). `id` is optional and defaults to the slug of the name; it is internal and never stored, so
  renaming is "change `name`, keep the old one in `aka`". Fuzzy matching (`ItemKey`: plural s, "made", `&`) lives
  only in the vanilla bridge, never for names the ruleset writes itself.
- **Layers, not redefinitions.** Resolution order is unchanged (entity-embedded > Battle Companies > vanilla). An entry
  whose name exists in a lower layer and that has **no** `definition` / base fields is an *annotation*: it adds
  properties (aura, target, tts, classes, fill) to the lower thing and inherits the rest. An entry with a definition that
  shadows a lower one must say `"overrides": true` (today's `overrides`), otherwise the loader reports it.
- **Strict.** Unknown keys are errors (typos stop silently disappearing), and so is an unknown value of a closed vocabulary
  (gear class, aura kind, path kind, stat).
- **Rules by name.** A rule is defined once, in `rules[]` (or in the vanilla file or the glossary), and units, hero
  upgrades, path rows and gear name it. A hero upgrade or path row may carry an inline `definition` for a rule used only
  there; the loader hoists it into the rule index and the same name defined twice with different text is an error. The
  shipped file uses names.
- **Prose stays in the data where it is displayed**; prose that nothing displays goes to `reference/`.

## 2. File layout

```text
definitions/
  vanilla/data2024.json, .schema.json          read-only standard (uploaded)
  glossary/mesbg_glossary.json, .schema.json   unchanged (uploaded)
  concepts/concept_graph.json, .schema.json    unchanged (generated)
  battle_companies/
    battle_companies.json           THE ruleset, one file (uploaded), generated by tools/build_battle_companies.py
    battle_companies.schema.json    one schema, $defs per section
    reference/*.json                rule prose nothing reads (influence prose, pathNotes, gold prose); committed, never uploaded or hashed
tools/data/                         curator-only decisions (section 8), read by the generator and the validator, never shipped
```

Sections of the one file: the header (`schemaVersion`, `name`, `version`, `credit`, `extends`), `campaign` (engine tables and
parameters: injuries, progression, influence, recruitment, rating, breakPoint, againstTheOdds, ...; section 12), `rules`, `magic`, `gear`, `paths`, `scenarios`,
`companies`, `units` (shared units), `vanilla` (what BC says about vanilla things, 4.9) and `auras` (`kinds`, `groups`).

Rules of the one file (the whole story for users): within a kind (company, unit, gear, rule, path, scenario) a name is unique;
two entries with the same name are an error. An entry that shadows a vanilla or glossary thing says `"overrides": true`; one that
only adds properties to it is an annotation (section 4.10). A user changes a shipped company by editing it in their copy of the
file; there is no `replaces`, `patch` or `extensions` folder.

**Is a split source layout in the repository worth it?** A split (`campaign`, `catalogue`, 37 company files, merged into the one
file by `tools/`) would only pay if the company files became the hand-curated source. Today the generator is the only author:
the companies come from the rulebook text and the tables of `tools/bc/core.py`, so a JSON source layout would be a second
format to keep in step with the generator, plus a merge step in `tools/` repeating part of the loader's semantics, for a
diff benefit that the committed single file already gives if the generator writes it pretty-printed with a stable order
(companies by name, one section per block). Recommendation: **no split now**; revisit only if the shipped data stops being
generated. Nothing in the design depends on the answer.

The `schemaVersion` (integer, starts at 2; 1 was the implicit old format) is separate from `version` (the rulebook,
"1.1"). The loader rejects a file with a version it does not know, naming the file and the supported versions.

## 3. Defaults and inheritance (applied once, by the loader)

| Where | Default / inheritance |
|---|---|
| any entity | `id` = slug(name); `aka` = []; `note` optional, shown as a footnote |
| company | `faction` = `name` (the vanilla faction; `vanillaFaction` is written in 18 companies and equals the name in all 18, so it is only written when different); `maxModels` = `campaign.company.maxModels`; `oath` by alignment (`{vow, text}` fallback texts in `campaign`), `{name}` placeholder; `extends: "<company>"` inherits `reinforcements.influence`, `maxModels`, `limits`, `heroUpgrades` shape, `oath`; lists (`units`, `rows`) replace, scalars override |
| company start / chart rows | `count` = 1, `rare` = 0, `roll` is `3` or `"2-3"` (no `[lo,hi]` pair); a chart with `rare` rows yields the old `rareUnits`; the first `nothing` row with `blocks` keeps `blocksFurtherRolls` |
| unit | `extends` = vanilla unit (or another BC unit); `name` defaults to `extends` (an annotation); `with` = options pre-selected (was `selectedOptions`); `addRules`, `addKeywords`; the unit's `types`, `composite`, `includes`, `models` replace the old effects |
| gear | `rating: 5` means `{warrior:5, hero:5}`; `gold` = `rating` (41 of 41 equal today); `classes` default from the vanilla `type` (35 of 106 annotations are exactly that); `extends: "<gear>"` inherits everything but name/definition (Elven-made Glaive extends Glaive); `size` for equipment only; `names[]` groups become `name` + `aka` |
| rule | `tag` absent = a plain rule; `minor: true` replaces `minorSpecialRules`; a rule without `definition` resolves its text in the lower layer (vanilla, glossary) |
| rule `target` | `appliesTo` default `own` (a rule not listed is own); `match` default `any` |
| path | `kind` default `heroic`; the kind sets `canLevelUp`, `givesExperience`, `countsAsHero`, `staysWarrior`, `selectable`, `startsOnPathWhenRecruited` (table in section 5.4); a path may `extends` another and replace only `maximums` or `rows` |
| scenario | `side` default `both`; `maxPoints` derived from the options and `count` |
| rows of a path | `text` derived (section 6); `offers` and `summary` derived |
| campaign | none: every engine parameter is written in the file (section 12 lists the value the shipped file carries); heroes count = `company.leaders` + `company.sergeants` |

## 4. Schema per concept, with a minimal worked example

Abbreviated texts are marked `...`; real files hold the full text.

### 4.1 Company (`companies[]`)

Fields: `name`, `aka?`, `alignment`, `description`, `epigraph?`, `faction?`, `oath?`, `extends?`, `maxModels?`, `breakPoint?`
(percent), `start{gold, units[]}`, `reinforcements{influence, rows[]}`,
`special{rows[]}?`, `advancements[]?`, `heroUpgrades[]`, `units[]?` (company-local units), `rules[]?` (company-local rule
definitions, hoisted), `gear[]?` (the old `uniqueWargear`), `promotions[]?`, `heroes?`, `recruit?`, `limits?`, `notes[]?`.
The 25 old `effects[]` kinds become typed fields (table in 4.1.1); there is no `effects[]` and no `layer`.

```json
{
  "companies": [{
    "name": "Arnor",
    "alignment": "good",
    "description": "Arnor was once a great realm, formed by Elendil in the Second Age, ...",
    "epigraph": { "quote": "Leave all that can be spared behind. We travel light.", "text": "As Arnor fell, ..." },
    "oath": { "vow": "I solemnly vow", "text": "Heirs of Arnor, I take command of this company ... called {name}. ..." },
    "start": { "gold": 4, "units": [ { "unit": "Warrior of Arnor", "count": 4 }, { "unit": "Ranger of Arnor", "count": 3 } ] },
    "reinforcements": { "influence": 3, "rows": [
      { "roll": 1,      "nothing": true, "blocks": true },
      { "roll": "2-3",  "unit": "Warrior of Arnor" },
      { "roll": 4,      "unit": "Ranger of Arnor" },
      { "roll": 5,      "unit": "Ranger of Arnor", "with": ["Spear"] },
      { "roll": 6,      "unit": "Knight of Arnor" } ] },
    "advancements": [ { "from": "Warrior of Arnor", "to": ["Knight of Arnor"] } ],
    "heroUpgrades": [ { "name": "Foresight of the Seer", "flavour": "It is the hero's sworn duty ..." } ],
    "promotions": [ { "from": "Ranger of Arnor", "on": "heroInTheMaking", "becomes": "Ranger of the North",
                      "keep": ["Spear"], "drop": ["Armour"], "statGain": false, "pathChoice": "any",
                      "text": "Should a Ranger of Arnor Warrior achieve a Hero in the Making result ..." } ]
  }]
}
```

What disappeared against today (Arnor is 4.3 KB of JSON today with `effects`, `vanillaFaction`, `specialRules` (same sentence as
the promotion), `layer` x2, the `heroUpgrade` effect and the hero upgrade's keyword row): a hero upgrade **names** its rule
(`Foresight of the Seer`, defined once in `rules[]` with source "Arnor Hero Upgrade", as the old `data.keywords` row was), so the
`effects` twin and the separate rule text collapse to one place. A hero upgrade may instead carry an inline `definition` (the
loader hoists it into the rule index); the shipped file does not.

A **shared start for companies of one family** uses `extends`; an **Elite company** is `"maxModels": 12`.

#### 4.1.1 The old `effects[]` kinds, one home each

| Old effect kind (count) | New home |
|---|---|
| heroUpgrade (44), maxModels (15), unitAlias (9), ruleSubstitution (7), uniqueWargear (5), limitExemption (4), limitOverride (3), grantedRule (3), specialUnit (3), companyLimit (1), ruleExchange (1), aura (1), breakPoint (1), onTheirOwnPath (17) | the typed field they already duplicate (`heroUpgrades`, `maxModels`, `units`, `substitutions`, `gear`, `limits.exempt`, `limits.override`, `granted`, `special.rows`, `limits.company`, `exchanges`, rule `aura`, `breakPoint`, `ownPath`); **the effect is deleted** (97 of 129 entries) |
| unitTypes (1) | `unit.types: {add, remove}` |
| compositeModel (1) | `unit.composite` |
| includedUnitPoints (1) | `unit.includes` |
| modelCount (1) | `unit.models` |
| heroPromotion (1) | `company.promotions[]` |
| recruitFallback (2), rangerReplacement (1), reinforcementReplacement (1) | `company.recruit.fallback` / `company.recruit.replace[]` |
| startingHeroRestriction (2), startingHeroNoStatGain (2), recruitAsHero (1) | `company.heroes: {allowed, excluded, noStartGain[], recruitAsHero}` |
| raceBalance (1) | `company.recruit.balance` |

Every one of these is a typed property the engine reads without a `kind` switch, so a user company can use them without
a Go change. The text that explains a typed rule lives **in** the typed object (`text`); `specialRules[]` and `notes[]`
keep only the sentences that have no typed form, so `CompanyProfile.OwnRules()` (which hides copies) goes away.

### 4.2 Unit: vanilla base plus a delta

Units are not redefined (a name that exists in a lower layer is an annotation, 4.10). A BC unit is a name, a vanilla (or BC) base, and deltas. It lives in `company.units[]`
(company-local) or the top-level `units[]` (shared). Resolution of a unit name: company-local, shared, vanilla.

```json
{ "name": "Ranger of Ithilien", "extends": "Ranger of Gondor", "addRules": ["Loyal to the Captain"],
  "note": "A Ranger of Ithilien is a Ranger of Gondor with the Loyal to the Captain special rule at no additional cost." }

{ "name": "Tookish Hunter", "extends": "Hobbit Archer", "with": ["<the vanilla option>"] }

{ "name": "Angmar Warg Rider", "tts": { "foot": "Angmar Orc Warrior", "states": false } }
```

A **brand-new unit with no base** is also allowed: an entry with a full definition (`profile`, options, price) and no `extends`; the name must not
exist in vanilla. The same holds for items (4.3, a full `definition` and prices) and rules. The shipped file uses it for no unit; the generator does not need it, a user may.

The third form (a name that is already a vanilla unit, no `extends`, only properties) is an *annotation* of that unit; the
two forms are compared side by side, with what the loader does in each, in section 4.10. Per-unit typed properties: `types{add,remove}`, `composite`,
`includes`, `models`, `ownPath` (On Their Own Path, with `path?`), `tts`. A legacy printed name is another `aka` of the same unit, which removes the
`ChangesUnit` distinction.

### 4.3 Item (gear)

One list `gear[]` replaces `wargear.items`, `equipment.items`, `creatures.items`, `wanderers.items`, `uniqueWargear` and
`gearAnnotations`. `kind` is `wargear | equipment | creature | wanderer`; a company-local `gear[]` entry is unique wargear.

```json
{ "name": "Badge of Courage", "kind": "equipment", "size": "small", "influence": 2, "rating": 5,
  "definition": "A model with a Badge of Courage gains the Fearless special rule.", "grants": ["Fearless"] }

{ "name": "Bow", "aka": ["Dwarf Bow", "Uruk-hai Bow", "Orc Bow", "Short Bow"], "kind": "wargear", "type": "bow",
  "influence": 1, "rating": { "warrior": 1, "hero": 5 } }

{ "name": "Dunharrow Spear", "kind": "wargear", "extends": "Spear", "influence": 1, "rating": { "warrior": 1, "hero": 5 },
  "who": { "units": ["Rider of Rohan"], "hero": false }, "limit": 2,
  "definition": "A model armed with a Dunharrow Spear ...", "classes": ["melee", "spear"] }
```

The last is a hypothetical user item; it is the full set of fields a person writes. Fields: `name`, `aka`, `kind`, `type` (vanilla
type word), `classes[]`, `influence`, `rating`, `gold?`, `size?`, `definition?` (absent = the vanilla text, the loader
appends the class rule text as `gearAnnotations` does today), `grants[]` (rules the bearer gets),
`grantsForChosenWeapon {rule, weaponClasses}` (Envenom Weapon), `statChanges[]`, `abilities {defenceBonus, attacksBonus}`
(the only two keys Go reads; others are dropped from the shipped data), `ranged`, `pricedAs`/`base`, `requiresMount`, `modifiers[]`
tags (`elven`), `aura?`, `tts?`, `who?`, `limit?`, `unit?` stats for creatures/wanderers (`profile`).
Access restriction is **one** shape for every item: `who {hero?, race?, units?, unitTypes?, alignment?, text?}`; the
`Only`/`OnlyUnits`/`OnlyKind` and the " heroes" string parse (backend 2.6) disappear. Limits count classes through
`limits[].classes`. Vanilla gear needs no entry unless BC annotates it (`name` only, plus `classes`/`grants`...).

### 4.4 Path

```json
{ "name": "Path of the Tactician", "kind": "heroic", "dice": "2d6",
  "description": "...",
  "maximums": { "fight": { "maxIncrease": 3 }, "shoot": { "best": 3 }, "attack": { "max": 2 }, "wounds": { "max": 2 } },
  "mastery": "Heroic Move Mastery",
  "heroicAction": null,
  "signature": ["will"],
  "rows": [
    { "roll": 2, "options": [ { "rule": "Spy" } ] },
    { "roll": 3, "options": [ { "rule": "Gatekeeper" } ] },
    { "roll": 4, "options": [ { "stat": "fight" } ] }
  ] }
```

`rows[].options[]` stay a closed alternative list: `{stat}`, `{rule}`, `{minorRule, maxTotal}`, `{heroicAction, maxTotal}`,
`{power}`, `{castingValue}`, `{oneOf:[...]}`; a `rule` is a name (defined in `rules[]`, `Spy` above has its entry there with
`"tag": "Active"`); an inline definition object is accepted and hoisted into the rule index, but the shipped file names.
The row `text` is derived. `spells` (the old `spellList`) is the list of power names; `heroicAction` is a name (its text comes
from the rule index or the glossary, "Heroic Channeling"). `kind: heroic | custom | endOfPath`
(a person adding a path writes the object above and nothing else; the picker tooltip comes from the derived `offers`).

### 4.5 Rule / keyword (aura, target, tts on the rule or item itself)

One list `rules[]` replaces `keywords`, `minorSpecialRules`, `parameterisedRules`, `ruleTargets` and the rule side of `auras`.

```json
{ "name": "At 'em, Lads!", "definition": "...", "source": "Hobbit Heroes",
  "target": { "appliesTo": "enemy-keyword", "keywords": ["Hobbit"], "match": "all", "text": "enemy {X} models" } }

{ "name": "Hatred", "overrides": true, "parameter": { "kind": "keyword", "picks": "company" },
  "target": { "appliesTo": "enemy-keyword", "text": "enemy models with the {X} keyword" } }

{ "name": "Stand Fast", "aura": { "radius": 6, "kind": "command", "affects": "friendly", "when": "always", "los": false,
                                  "tip": "Friendly models within 6\" ..." } }
```

- `tag` (`Active`/`Passive`), `source` (a path or company), `minor: true`, `overrides: true`, `aka` (`keywordSpellings`
  merges: Lothlorien/Iron Hills/Misty Mountains move here from `keywords.go`), `parameter {kind: keyword|weapon|rollTarget|number|companyHero, picks}`.
- `target`: `appliesTo` `enemy-keyword | enemy-any | friendly-keyword | own`, `keywords[]`, `match` `any|all`,
  `bearerKeywords[]`, `text` with `{X}`. A parameterised rule is matched by its base (`Hatred (Orc)` is `Hatred`), so the 21 spelled `variants` go away.
- `aura`: `radius`, `kind` (`banner|command|support|enemy|utility`), `affects`, `when` (a group id), `los`, `keywords[]`, `tip`, `concept`.
  The same object may sit on a **gear** entry (Banner, Company Standard). Rings with identical (radius, kind, affects, when,
  los, keywords) merge at load (Banner + Company Standard), as `group` does today. The `kinds` and `groups` tables (colours, labels) stay once, in the top-level
  `auras: {kinds, groups}`: they are engine vocabulary, not per-rule data.
- `tts` on a rule is not used; see units/gear.
- An annotation (no `definition`) adds `aura`/`target` to a vanilla rule with the vanilla text (`Stand Fast` above is a glossary
  concept, so its text resolves there).

### 4.6 TTS

`tts` is a property of the thing: `unit.tts {model?, foot?, states?, options?, defaultOptions?, alternatives?, marker?}` and
`gear.tts {name?, model?}`. Units and gear BC defines carry it inline; a vanilla unit, mount or item that BC cannot edit gets it
through an annotation (4.10): an entry of `units[]` / `gear[]` with that name and only `tts`. The optional `vanilla.units` /
`vanilla.mounts` (4.9) stay for fill-ins of missing vanilla data, not for properties.
`standIns`, `mounts`, `riders`, `names`, `defaultOptions`, `markerOptions` stop being six name-keyed tables; `reason` strings go to `tools/data/`.

### 4.7 Scenario (`scenarios[]`)

```json
{ "name": "Seize Ground", "description": "As two companies become locked in a battle of wits ...",
  "armies": "All available members of each Battle Company.", "layout": "...", "startingPositions": "...",
  "objectives": "The game lasts until ...", "specialRules": [], "roles": [], "rewards": [],
  "scoring": [
    { "key": "objectiveMarkers", "side": "both", "kind": "eachTiers", "item": "Objective Marker", "count": 5,
      "options": [ { "text": "You score 1 Victory Point if you have more models within 3\" than your opponent.", "points": 1 },
                   { "text": "If you have at least twice as many models as your opponent within 3\", you instead score 2 Victory Points.", "points": 2 },
                   { "text": "If you are the only player to have models within 3\", you instead score 3 Victory Points.", "points": 3 } ] }
  ] }
```

### 4.8 Campaign tables and parameters (`campaign`)

`campaign` keeps its shape (cost properties, injuries, progression, ...). Every rule value the engine uses is a field of it and
is **read**; section 12 lists them with name, default, type and range. What changes:

- `campaign.company` becomes

  ```json
  "company": { "maxModels": 15, "leaders": 1, "sergeants": 2, "heroStart": { "might": 1, "will": 1, "fate": 1 },
               "startingInfluence": 0, "equipmentSlots": { "small": 1, "large": 1 } }
  ```

  The Elite model limit is the company's own `maxModels: 12`; `eliteMaxModels`, `startingHeroes` (= leaders + sergeants),
  `startingPoints`, `heroicTier` and `text` leave. There is **no `startingTotal`**: the 105 points of a starting company (106 for
  Defenders of the North) and the 60 of the custom-company guideline are not rules (decision 13). The contradictory
  `campaign.gold.startingTotal: 60` is deleted; `campaign.gold` keeps what code reads or shows.
- `campaign.recruitment` keeps only parameters: `die`, `adjust` (`direction`, `costPerPoint`, `maxPerAttempt`) and `rare`
  (`injuredCount`, `whenExhausted` is a fixed choice and leaves). Its explanatory prose (`text` and the meaning keys) is dropped.
  `campaign.customCompany` is dropped altogether (prose only). `influence.spend[adjustRoll]` stays and is read: its `directions`,
  `costPerPoint` and `maxPerRoll` are the same three values as `recruitment.adjust`, written **once**: `spend[adjustRoll]` holds
  the player-facing name and text and refers to `recruitment.adjust` for the numbers (the template prints them from the view).
- `campaign.validations` is dropped (its content is `company.*`, `limits[]`, `creatures.rules`, `wanderers.rules`, `path.limit`).
- **`campaign.matchInput` stays, prose included, and is shown to players.** The app does not show it today: a search of
  `internal/`, `cmd/`, templates and `static/` finds no reader (only `tools/validate_definitions.py` reads it). The typed decode must
  expose `matchInput.text`, `result` and the `text` of every `perCompany[]` and `perModel[]` entry, and the match page
  (`match.gohtml` and the live panels) must show them as the field help: gap filed as a frontend bead
  (`battle-company-manager-ce-v2-ok8`, section 11, slice 10).
- Unread keys that stay unread are cut: `resolution`, `credit` stays in the header, `courageIntelligenceTotalCap` and
  `peak.willMaxOverrides` are either read or deleted (slice P6).

### 4.9 The `vanilla` section: everything BC says about vanilla things

```json
"vanilla": {
  "pins":   [ { "kind": "hero", "name": "Peregrin Took", "occurrence": 2 }, { "kind": "hero", "name": "Bain, Son of Bard", "occurrence": 2 } ],
  "units":  { "Hunter Orc Warrior": { "race": ["Orc"], "faction": ["Gundabad"], "unitType": ["Warrior","Infantry"], "baseSize": ["25mm"] } },
  "mounts": { "<vanilla mount>": { "race": ["..."], "faction": ["..."] } },
  "options": { "notSold": ["banner", "war horn", "war drum"],
               "exceptions": [ { "unit": "Cave Troll", "option": "Hand-and-a-half hammer", "readAs": "Hand-and-a-half Weapon" },
                               { "unit": "Cave Troll", "option": "Troll Spear", "ownItem": "Troll Spear" },
                               { "unit": "Ranger of the North", "option": "Armour", "granted": true } ] }
}
```

`units`/`mounts` fill values apply **only where the vanilla field is empty** (as `unitFillIns`/`mountFillIns`). `pins` is the only shipped record of a duplicate decision (first occurrence wins otherwise).

### 4.10 `extends` and annotation, side by side (real data)

Both attach to something that already exists; they differ in whether a **new name** is created.

**A. `extends`: a new unit based on another (Ranger of Ithilien, company Osgiliath & Ithilien).** Today this is a `unitAliases`
entry, an `effects` twin (`kind: unitAlias`, `reason: delta`), a `companyNotes` footnote and a generated sentence. New:

```json
{ "name": "Ranger of Ithilien", "extends": "Ranger of Gondor", "addRules": ["Loyal to the Captain"],
  "note": "A Ranger of Ithilien is a Ranger of Gondor with the Loyal to the Captain special rule at no additional cost." }
```

The company then writes the new name wherever it recruits: `{ "roll": 4, "unit": "Ranger of Ithilien" }`.

**B. Annotation: properties on an existing vanilla thing, no new name.**

```json
{ "name": "Angmar Warg Rider", "tts": { "foot": "Angmar Orc Warrior", "states": false } }
```
```json
{ "name": "Lead by Example",
  "aura": { "radius": 6, "kind": "banner", "affects": "friendly", "when": "conditional", "los": true, "keywords": ["Orc", "Troll"],
            "tip": "After slaying 3 enemy Heroes or Monsters, other friendly Orcs and Trolls treat this model as a banner with range 6\"." } }
```

The first entry sits in `units[]` (today a row of `data.ttsModels.riders`: unit, footModel, states, reason). The second sits in
`rules[]` (today an entry of `data.auras.entries` with `source: {type: rule, names: [Lead by Example]}`, a `reason` and a `group`);
"Lead by Example" is Buhrdur's vanilla rule and has no Battle Companies text.

| | A. `extends` | B. annotation |
|---|---|---|
| Trigger | the entry has `extends` (a base) | the entry has the name of an existing lower-layer thing, no `extends`, no `definition` |
| Name | new: a roster, a chart row and a database row say "Ranger of Ithilien"; the name must not exist in vanilla | the same name; nothing new appears in any list |
| Stats, options, price, text | taken from the base (vanilla Ranger of Gondor), plus `with` options and the deltas | taken from the lower layer unchanged; the entry adds only its own properties (`tts`, `aura`, `target`, `classes`, `fill`) |
| Loader | resolves the base (company-local, shared, vanilla), builds the unit as base plus delta, registers the name and its `aka`; error if the base is missing or the name shadows another unit | resolves the name in the lower layers (exact or `aka`, no fuzzy match), merges the properties onto that thing; error if nothing of that name exists, a property is set twice, or a `definition` is present without `overrides` |
| Typical use | a company-specific unit, a legacy 2018 name (`aka`), a unit with preselected options | engine metadata about things the book does not rename: TTS figures of vanilla units and mounts, aura rings of vanilla rules, classes and grants of vanilla gear, targets of vanilla rules |
| Size today | 9 `unitAlias` entries (2 deltas, 7 aliases that preselect options) | 18 rider entries of `ttsModels`, at least 5 vanilla-only rules with an aura (Lead by Example, Keen Sight, Master of Battle, A Fell Light is in Them, Enchanting Song), 106 `gearAnnotations`, 53 named rule targets |

**Why both are needed.** The player sees "Ranger of Ithilien" as a distinct recruit with its own note, which needs a new name; a profile of the
vanilla file still carries "Ranger of Gondor" with the vanilla rules, so a delta cannot rename it. The Angmar Warg Rider and Buhrdur's rule are the other way round: the
vanilla name is exactly what appears on the model's profile, in the vanilla options and in the TTS figure lookup, so a property must attach to that name. Making
it an `extends` would invent a second name that no profile, option or chart carries and would create a phantom unit in the pickers. Without annotations the
properties would stay in the name-keyed side tables (`ttsModels`, `auras.entries`, `gearAnnotations`, `ruleTargets`) that this design removes.

## 5. What the loader derives (nothing below is stored)

| Today's section | Derivation at load |
|---|---|
| `profileWargearNotSold` (5 KB) | vanilla profile wargear of every unit a company uses whose item no gear row, equipment or unique gear names; a golden test pins the 23 results |
| `reservedOptions` (1.6 KB) | vanilla options whose `availableIn` lists only hero names; same 10 |
| `optionSales` notSold (63) | rule `vanilla.options.notSold`: option name contains (case-insensitive) one of the words |
| `paths[].offers`, `summary` (6.8 KB) | pure function of `rows`, `heroicAction`, `spells`, `mastery`; the English summary sentence moves into Go with a golden test |
| `rating` / `gold` | `gold` = `rating` unless written |
| `magicalPowers.items[].definition` (13 KB) | vanilla text; BC adds `magic.powers {Name: {range, casting}}` (31 entries) and `magic.castingRule`; `learnable` = the power is in a path's `spells`; the 12 others need nothing |
| keyword text inside progression rows (10.7 KB) | row text = `"{name} ({tag}) - {definition}"` for rule options; an explicit `text` is kept only where a row differs (the validator lists them) |
| `startingHeroicAction.definition` (3.5 KB) | by name from the rule index / glossary |
| `rareUnits` | rows with `rare` on `special.rows` / `reinforcements.rows` |
| `vanillaFaction` | `faction` defaults to the company name |
| `onTheirOwnPathRulebook`, `layer`, every `reason` | not stored (the rulebook spelling is an `aka` or a tools record) |
| `ruleTargets.patterns[].variants` | pattern matching by base name |
| `scenarios` (name list), `scenarioScoring`, `scoring` prose, `scoringObjectives[].text` | the list of scenario names; the displayed sentence joins the option texts |
| class of a gear item | vanilla `type` when not annotated |
| company `oath`, `maxModels`, the three `limits` defaults | `campaign` defaults |
| Hero-upgrade, path and company rules | hoisted from inline `definition` into the rule index |

## 6. Scoring: three forms to one

Keep the typed `scenarios[].scoring[]` form (it carries `side`, `secret`, `kind`, `item`, `count`, `instruction`, and `options[{text, points}]`), because
`options.go` needs the structure and the tier sentence exists exactly once there. Delete `scenarioScoring[]` (21 KB), the
prose `scoring[]` (17 KB), the objective-level `text` (the joined options, about 20 KB) and the `scenarios` name list. Display
text is composed from `options[].text`. The validator asserts, before the old form is dropped, that every old prose bullet equals
a typed option and the counts match (the generator zips by order today).

## 7. Validation messages for hand-written files

An upload (vanilla, Battle Companies or glossary) is validated as a whole before anything is stored; **every** problem is
collected and returned to the user, none stops the report early, and when any `error` exists nothing is saved and no campaign
is created. Format: `<file>: <json path>: <message>` followed by a one-line hint and, for a name that does not resolve, the
closest match; `<file>` is one of the three uploaded files. A `warn` is shown with the report but does not block.

| Case | Message |
|---|---|
| unknown key | `companies/x.json: companies[2].strat: unknown field; did you mean "start"?` |
| unknown unit | `... start.units[1].unit: no unit "Ranger of Arnr" in this company, the shared units or vanilla; closest: "Ranger of Arnor"` |
| unknown gear / option | `... reinforcements.rows[4].with[0]: "Speer" is not an option of "Ranger of Arnor" nor a gear item; closest: "Spear"` |
| unknown rule | `... heroUpgrades[0].name: no rule "Foresight of the Seer" in rules[], vanilla or the glossary; closest: "Foresight"; define it in rules[]` |
| duplicate name | `... gear[3].name: "Bow" is already defined at gear[1]; names are unique, use another name or `aka`` |
| silent shadowing | `... rules[0]: "Hatred" exists in vanilla; add "overrides": true or drop "definition" to annotate it` |
| missing definition | `... rules[7]: "Foo" has no definition here, in Battle Companies, vanilla or the glossary` |
| bad chart | `... reinforcements.rows: rolls cover 1-5, 6 is missing (a D6 chart must cover 1-6 exactly once)` |
| unreachable row | `... reinforcements.rows[3].roll: "3-2" is empty` |
| officers | `... start: the company has no unit that may lead (heroes.allowed excludes all)` |
| limits | `... limits.override[0]: no limit "bows"; known: bow, throwing, cavalry` |
| aliases | `... units[1].aka: "Ranger of Gondor" is a vanilla name; an alias must not shadow a different unit` |
| path rows | `... rows: rolls 2..12 need an option each; 7 is missing` ; `... options[0].stat: "figth" is not a characteristic` |
| path text | `warn: ... rows[3].text differs from the composed "Spy (Active) - ..."; remove "text" unless intended` |
| aura | `... aura.kind: "banners"; known: banner, command, support, enemy, utility` |
| scenario | `... scoring[1].options[0].points: expected a number`; `... scoring: "objectiveMarkers" is listed twice` |
| version | `battle_companies.json: schemaVersion 1 is not supported (this app reads 2); the file must be regenerated or converted` |
| vanilla drift | `warn: vanilla.pins[0]: occurrence 2 does not exist for "Peregrin Took" in this vanilla file` |
| vocabulary | `... gear[2].kind: "wargaer" is not a kind; known: wargear, equipment, creature, wanderer; closest: "wargear"` |

All messages are produced by the single Go decoder (strict decode plus a reference-resolution pass) and mirrored by `tools/validate_definitions.py` through the same JSON schema. The upload handler (app session) calls the decoder, shows the whole list and stores nothing on any `error`.

## 8. Curator-only data that moves to `tools/data/`

| File | Content | Today |
|---|---|---|
| `duplicate_decisions.json` | status + note of the 91 vanilla duplicates (the "never ask twice" record, from `DUPLICATE_DECISIONS`) | `data.vanillaAmbiguities` (22 KB) |
| `aura_audit.json` | `excluded`, `unreachable`, `outOfScope`, per-entry `reason` | `data.auras` (16 KB of 34) |
| `gear_evidence.json` | `evidence`, `status` (curated/derived), `isA`, notes | `gearAnnotations` (7.8 KB) |
| `option_sales_reasons.json` | `reason` + `source` of the 68 sales; `wargearReadAs` | `optionSales`, `wargearReadAs` (about 16 KB) |
| `fillin_reviews.json` | `reason`, `REVIEW:` status of fill-ins | `unitFillIns`, `mountFillIns` |
| `tts_provenance.json` | `reason` strings | `ttsModels` (1.4 KB) |
| (stays in `tools/bc/core.py`) | core tables, `UNIT_DELTAS`, `LEGACY_NAMES`, `DUPLICATE_DECISIONS` source | |

The generator reads these to keep provenance and the validator asserts every decision still has a record. One convention: `provenance: confirmed | inferred | review`.

## 9. Reader verdicts (slice 0, read-only grep of `internal/`, `cmd/`, `tools/`, templates, e2e, `tests/`)

Verdicts for the 15 questions of the curator audit, section 6, and for the points the two audits disagreed on. "Read" means a Go
struct, a template or a tool accesses the key; the validator is named when it is the only reader.

1. **Independent decodes of the same bytes?** Yes, about 14: `ParseBattleCompanies`, `parseProfiles`, `ParseProgression`,
   `parsePathRules`, `ParseInjuries`, `ParseCreaturesAndWanderers`, `ParseATO`, `ParseAuras`, `ParseTTSModels`,
   `ParseScenarios`/`ParseCampaignOptions` (`options.go`), `ParseVanillaAmbiguities`, `glossary_extras.go`, aftermath (`aftermath.go`), plus the
   raw readers of `cmd/server/world_*.go`. One typed decode shared by hash replaces them (section 1).
2. **`campaign.influence`: read or not?** Settled. Go reads exactly one part: `campaign.influence.gains` (participating, draw,
   win; `internal/core/aftermath.go`, documented in `docs/aftermath.md`). Everything else under `influence` (`text`, `allIsLost`,
   `reinforcementRollOfOne`, `rareUnits`, `specialUnits`, `wargearLimits`, and the whole `spend[]`) has **no reader** in `internal/`, `cmd/`,
   templates, e2e or fixtures. The validator reads `spend[treatInjuries].cost` only to assert it exists; the number itself lives in
   `campaign.injuries.treatment`, which Go reads, and `spend[treatInjuries].details` points there. So the backend audit
   ("never read") was wrong about `gains`, and the curator's "readers" were the documents and the validator.
   **Verdict (as decided, decisions 12 and 14):** `campaign.influence.gains` stays as data and is read. `spend[adjustRoll]` stays and
   is **read** (its numbers are `recruitment.adjust`, section 12.2); the other `spend[]` entries and the rest of the influence prose go to
   `reference/`; the validator's `treatInjuries` assertion moves to the typed treatment table. `campaign.recruitment` keeps its
   parameters (read) and loses its prose; `campaign.customCompany` is dropped; `campaign.matchInput` stays, prose included, and is
   shown to players (4.8; its validator cross-checks `perModel` with `experience.gains` and `perCompany.fledBroken` stay).
   `data.pathNotes` goes to `reference/`; `resolution` and `validations` are dropped; `credit` stays in the header (not shown
   anywhere in the app yet).
3. **Effects or typed twins?** Both, as the backend audit says. **`onTheirOwnPath` settled:** the effect carries `units` and `path`
   and *is* what `OnOwnPath`/`OwnPathUnits`/`OwnPath` are built from (`companyprofile.go` around 425); the top-level
   `onTheirOwnPath` list is read only by the company-creation template (`company_new.gohtml`, a "has units on their own path"
   test) and by `snapshot_test.go`. In all 17 companies the two `units` lists are identical and the effect's `path` is always
   "On Their Own Path". **Verdict:** one typed field `ownPath: {units[], path?}` on the company (default path "On Their Own Path");
   the loader derives `OwnPathUnits`, `OwnPath` and the template flag from it; the effect and `onTheirOwnPathRulebook` go.
   The other 12 kinds the code reads from `effects` get the typed homes of table 4.1.1.
4. **Option-sales code paths?** Only `battlecompanies.go`, `audit.go`, `rules.go`, `ttsnames.go`, `world_aura.go` use the lists. The rule form
   (`vanilla.options`) is easier. `profileWargearNotSold` and `wargearReadAs` have no Go reader (the validator reads them): derive and move to `tools/`.
   `reservedOptions` derives from `availableIn`.
5. **`gearAnnotations` fields?** Read: `classes`, `aliases`, `grants`, `grantedTo`, `weaponModifier`, `grantsRuleForChosenWeapon`, `requiresMount`, `definition`,
   `pricedAs`, `base`, `modifiers`, `statChanges`, `abilities.defenceBonus/attacksBonus`, `relatedTo`, `profile`, `ranged`. Unread: `evidence`, `status`, `isA`, `note`.
   The concept graph (`tools/concept_graph.py`, `build_concept_graph.py`) additionally reads `gearAnnotations.items`, `equipment.items[].grantsSpecialRules`
   and `paths[].startingHeroicAction`/`progression[].options`; its derivation must be ported to `gear[]`/`paths[]` (a task of slice 5 and 6).
6. **`vanillaAmbiguities` / `duplicates.go` / `audit.go`?** Settled. `VanillaAmbiguity` has four fields: `Kind`, `Name`, `Status`, `Occurrence`.
   The Go code (`duplicates.go`, `audit.go`, `rules.go`, `cmd/server/audit.go`) never reads a note or reason. The `vanilla.pins` list plus
   "first wins" is enough for the loader; the audit report needs the `status` strings (`equivalent`, `overridden`, `firstWins`) of the 91
   reviewed duplicates: they go to `tools/data/duplicate_decisions.json`, which `cmd/server/audit.go` reads (a dev command), or `make audit`
   reports only the pins and the unreviewed ones. No notes are needed at run time.
7. **`auras` extras?** `excluded`, `unreachable`, `outOfScope` have no Go reader (24 KB audit-only data): move to `tools/data/aura_audit.json`. `tip` is a shortened rule text and is **not derivable**: kept as `aura.tip`. Per-entry `reason` is not in the structs read; it moves with the audit data.
8. **`offers`/`summary`, heroic action definition?** Yes, both computed at load (section 5); the loader already walks the rows in `parsePathRules`/`ParseProgression`.
9. **`ttsModels` separate file?** It already has its own parser. It becomes `tts` properties on the thing; annotations of vanilla things in `units[]`/`gear[]` (4.6, 4.10). `world_aura.go` and `tools/test_tts_models.py` read the old shape and are regenerated (section 11).
10. **`magicalPowers` from vanilla?** Yes; the Go loader already reads vanilla; BC gives `range`/`casting` only (section 5).
11. **`tag`, `overrides`, `minorSpecialRules`?** `definition`, `label` and the minor kinds are read; `tag` and `overrides` are resolution/popup metadata kept as one word each.
    `minorSpecialRules` becomes `minor: true`.
12. **`scenarioScoring` vs `scoringObjectives`? Settled: the UI uses both.** `scenarioScoring[].points[][].text` feeds `ScenarioInfo.Scoring` (`options.go`), the
    prose lines of the match page and the scenario card (`match.gohtml`). `data.scenarioDetails[].scoringObjectives` feeds the typed
    objectives (`ScoringObjective`: ticks, Victory Point entry, secret objectives, nominations in `match.go`, `live_service.go`, `internal/web/secret.go`), and also
    copies the objective-level `text`. **Verdict:** keep the typed form (4.7, section 6). `Scoring` is composed from `options[].text` (one line per objective, options joined by a space,
    which is what the reader does with the groups of `points` today); the validator proves each old bullet equals the composed line before the old form is dropped.
13. **`gold` separately?** `rating`/`gold` are both read, but 41 of 41 are equal; `gold` defaults to `rating` (section 3).
14. **Fixtures that depend on field names?** Settled; the full inventory is in section 11, slice 0. Goldens (`auras_golden.txt`, `tts_golden.txt`, `make audit` output) are keyed by names and survive. The e2e flows
    (`tests/e2e`) and `tests/tts` read no rules JSON (they drive the app and compare names), so they need no change other than names. What reads the file's *shape* and must be regenerated: three raw readers in
    `cmd/server` (`world_settings.go` `forkedEdition`, `world_gallery.go` `bcDefs`, `world_aura.go` `pathChart`), 23 Go tests that load the real file, 11 Go test files with inline JSON,
    and the Python tools listed in slice 0.
15. **Version policy?** `schemaVersion` (integer, 2) is separate from `version` ("1.1"); an unknown value is rejected with the file and the supported versions (section 2, 7).

**Keys that look unused at the time of the audit** (no Go, template, fixture, validator or concept-graph reader): `credit`, `resolution`, `validations`, `data.pathNotes`, `campaign.customCompany`,
`campaign.recruitment`, `campaign.influence` except `gains`; their fate is in decisions 12 and 14 (parameters are now read, prose is cut). Read by the validator only: `campaign.matchInput` (now shown, slice 10), `data.profileWargearNotSold`, `data.wargearReadAs`, `spend[treatInjuries]`.
`credit` should stay in the header for attribution even though the app does not show it (a decision for the owner; shown on the rules card would be a one-line app change).
Concept graph: it reads `keywords`, `parameterisedRules`, `paths`, `magicalPowers`, `campaign.injuries`, `gearAnnotations`, `equipment`; nothing else of the BC file.

## 10. Migration table, section by section

Saved is compact bytes removed from the loaded ruleset (curator numbers). "Moves" means kept elsewhere.

| Old section | New home | Saved | Risk |
|---|---|---|---|
| `name`, `version`, `extends`, `credit`, `resolution`, `limits`, `validations` | header of the file + `campaign` | 0.6 KB (`resolution`: code, not data) | low |
| `campaign.*` read tables (company, experience, injuries, progression, limits, influence, rating, ATO, scenarioSelection, gold numbers, aftermath, breakPoint, options) | `campaign`, same shape, **plus the parameters of section 12** (leaders, sergeants, hero start, slots, dice, adjust, thresholds); `campaign.gold.startingTotal` / `startingPoints` deleted | about 0 (new parameters offset the cuts) | low |
| `campaign.recruitment` (adjust, rare, die), `influence.spend[adjustRoll]` | stay in `campaign`, **read** (section 12.2) | 0 | low |
| `campaign.recruitment` prose, `campaign.customCompany`, `validations` | dropped (decision 14); rulebook text is in the real rulebook | about 8 KB | low (no reader) |
| `campaign.matchInput` | stays with its prose; shown to players (4.8, slice 10) | 0 | low |
| `gold` prose, `influence` prose except `gains` and `spend[adjustRoll]`, `pathNotes` | `reference/` (not uploaded) | about 4 KB moves | low (verified: no reader) |
| `scenarios`, `scenarioScoring`, `data.scenarioDetails` | `scenarios[]` one list (4.7) | 62 KB | low; validator proves equivalence |
| `data.companies` (37) | `companies[]`; effects twins gone, `rareUnits`, `vanillaFaction`, `onTheirOwnPathRulebook`, duplicate sentences | 22 KB | medium (Go reads both forms; one typed form) |
| `data.keywords`, `minorSpecialRules`, `parameterisedRules`, `ruleTargets` | `rules[]` (4.5) | 6.6 KB | medium (three Go readers) |
| `data.auras` | `aura` on rules/gear + `auras{kinds,groups}`; working data to `tools/data/` | 16 KB | low |
| `data.magicalPowers` | `magic.powers` + vanilla text | 13 KB | low |
| `data.paths` | `paths[]` (4.4); `offers`, keyword text in rows, heroic action text derived | 23 KB (10.7 + 6.8 + 3.5 + 1.8 spells) | low |
| `data.wargear`, `equipment`, `creatures`, `wanderers`, `gearAnnotations`, `uniqueWargear` | `gear[]` (4.3) | 24 KB | medium (a2g open; tooltip text rule must be preserved) |
| `optionSales`, `reservedOptions`, `profileWargearNotSold`, `wargearReadAs`, fill-ins | `vanilla.options`, derived, `vanilla.units/mounts` (4.9, section 5) | 28 KB | low to medium (Go switches from lookup to rule) |
| `data.vanillaAmbiguities` | `vanilla.pins` + `tools/data/duplicate_decisions.json` | 20 KB | low |
| `data.ttsModels` | `tts` on units/gear; annotations (4.10) for vanilla things | 1.4 KB (reasons); 3.5 KB reshaped | low |
| **Total** | | **about 225 KB of 609 KB compact (37 percent); 920 KB pretty to about 575 KB** | |

Python: with the derived sections gone, about 800 to 1,000 of the 4,600 lines in `tools/bc/` go (`armoury_gaps.py`, most of `auras.py`, offers in
`paths.py`, `rule_targets.py`, parts of `vanilla.py` and `gear.py`). Go: about 1,500 lines of `internal/core` (backend audit).

## 11. Order of slices

Each slice leaves `make test` and the golden snapshot (`internal/core/snapshot_test.go`, byte-identical) green. Old sections shrink one
by one; a transitional old-section reader may exist inside a slice, but **none survives slice 9**. Slices 1 to 9 are the format
change; **P1 to P6 are the parameter slices** (section 12) and slice 10 shows `matchInput`. Each parameter slice is guarded by (a) the
golden snapshot, unchanged, because the shipped file carries today's values, and (b) the `TestCustomRuleset` rows named in the slice
(12.8): the same build with the value changed must change the behaviour and the page text, and the default row must still equal the
shipped behaviour. A parameter slice has a curator part (generator `tools/bc/core.py`, schema with the range, validator, the
field in the shipped file) and an app part (typed decode, engine, view, template); the curator part goes first.

| Id | Slice | Owner agent | Depends on | Size |
|---|---|---|---|---|
| 0 | Freeze: golden snapshot is **done** (commit 02be79c); remaining: the `customRuleset(t, mutate)` helper, the empty `TestCustomRuleset` table, the go/ast magic-number lint in report-only mode, the fixture inventory below | backend-coder | none | S |
| 1 | Foundation: `schemaVersion`, header, one typed strict decoder, reference-resolution pass collecting every error (section 7), upload handler returning the list, `defaults/extends` engine, the dice-spec type of 12.1 (parser only) | backend-planner (plan) then backend-coder; definitions-curator (schema) | 0 | L |
| 2 | Cuts without behaviour change: unread prose to `reference/`, recruitment/customCompany prose and `validations`, `gold.startingTotal`/`startingPoints`, `eliteMaxModels` deleted, curator data to `tools/data/`, `vanillaAmbiguities` to pins, aura working data, evidence/status, `resolution` | definitions-curator | 1 | M |
| 3 | Scenarios: one list | definitions-curator + backend-coder | 1 | M |
| 4 | Rules: `rules[]`, hero upgrades and path rows by name, aura/target on the rule, magic from vanilla, `minor` flag, annotations | definitions-curator + backend-coder | 1, 2 | L |
| 5 | Gear: `gear[]`, `who`, option sale rule, derived `profileWargearNotSold`/`reservedOptions`, fill-ins in `vanilla`; port the concept graph's gear edges (after bead `a2g` and the reviewer pass) | definitions-curator + backend-coder | 1, 2, a2g | L |
| 6 | Paths: rows with rule names, derived `offers`/text; port the concept graph's path edges | definitions-curator + backend-coder | 4 | M |
| 7 | Companies: typed fields instead of `effects`, units with `extends`/`with`/deltas/no base, `ownPath`; delete the effects switch and `unitAliases` special cases | definitions-curator + backend-coder | 4, 5 | L |
| 8 | Name resolution and TTS: delete `canonicaliseWargear`, `promotion_gear` heuristics, `keywordSpellings`, spelling fallbacks; `tts` properties and annotations | backend-coder + definitions-curator | 5, 7 | M |
| P1 | Influence and recruiting parameters: A1 to A4, A7, A8, Rare counting flags (12.2 A9), H2; `RecruitRules`, `RecruitRoll.Budget`, `recruitment.die` | definitions-curator then backend-coder; frontend-implementer for H2 text | 1 | M |
| P2 | XP threshold, officers, hero start: C1, C6, B1, B3, B4, H1, H13; delete `XPPerLevel`, `maxSergeants`, `HeroBonus` constants; carry `LevelEvery` into `CompanyUnit` | definitions-curator then backend-coder; frontend-implementer for text | 1 | M |
| P3 | Rating and limits: F1, F2, B8, B9, B10; delete `defaultLimitRules` and the `max(1, ...)` fallbacks, honour `roundUp`, add `classes` | definitions-curator then backend-coder | 1 (uses today's item classes; moves to `gear[].classes` with slice 5) | M |
| P4 | Dice (`DiceSpec`): C2 to C4, D1, D2, E4, F5 die, A4 die, H3, H5, H6, H9, H10, H12; `Dice.Roll(spec)`, entry validation, chart coverage checks, views carry the spec | backend-coder; frontend-implementer for templates and `static/js` | 1 | L |
| P5 | Slots, break point, scenario tables, progression amounts, injuries, race: B6, B7, E1 (`comparison`), E6, F5 ranges, C5, D3, G3, H4, H7, H8, H11 | definitions-curator then backend-coder; frontend-implementer for text | 1, P4 (scenario ranges) | M |
| P6 | Data clean-up: read `courageIntelligenceTotalCap` and `willMaxOverrides` (C7): the Will maximum is 3 by default and 6 on Path of the Tactician and Path of the Sorcerer, with a snapshot row proving the 6, strict vocabularies (12.4), turn the magic-number lint from report-only to enforcing | backend-coder + definitions-curator | P1 to P5 | S |
| 9 | Cut over: delete the old sections and every old parser, bump the version, wipe and re-seed dev data, regenerate demo campaigns and tests (slice 0 inventory), rewrite `docs/definitions.md` as "how to add a company, unit, item, path, parameter" tested by a sample custom file the upload accepts and a file with typical mistakes it rejects with the whole list | definitions-curator + backend-coder | 1 to 8, P1 to P6 | L |
| 10 | Show `campaign.matchInput` prose on the match pages (bead `battle-company-manager-ce-v2-ok8`) | frontend-domain-expert (spec) then frontend-implementer | 1 | S |

Slice 0 inventory of what slice 9 must regenerate, with no loss of coverage (field-name grep done, section 9, question 14):

- `cmd/server/world*.go`: `forkedEdition` (changes `name`/`version` of the raw file), `bcDefs` (`data.companies[].specialUnits`), `pathChart` (`data.paths[].progression[].options`); the demo
  campaigns themselves use names only. After the change `make demo` must produce the same campaigns, companies, matches and aftermaths (compare the `show` log of the script before and after).
- Go tests: the 23 that load `definitions/battle_companies/battle_companies.json` (`core`, `web`, `postgres`, `cmd/server`) keep their assertions; the up to 11 with inline JSON (found by a rough grep, `aftermath_items_test`, `aftermath_test`,
  `auras_test`, `concepts_test`, `glossary_test`, `rules_test`, `ruleset_test`, `rulesets_test`, `vanilla_test`, `web/admin_test`, `fuzz_forms_test`) are rewritten in the new syntax.
- Python: `validate_definitions.py` (the largest reader), `test_auras.py`, `test_rule_targets.py`, `test_tts_models.py`, `test_concept_graph.py`, `test_company_text_checks.py`, `concept_graph.py`, `build_concept_graph.py`, `company_text_checks.py`, `tools/gear_review`.
- e2e (`tests/e2e`) and `tests/tts` read no rules JSON; they need only the names to stay stable (decision 5).
- `internal/core/startingtotal_test.go` keeps the 105 / 106 invariant of the shipped companies as test constants (decision 13); it is not moved into the data.

A shorter path to the stated goal (people adding companies): 1, 4, 7 first, then the rest. The parameter slices are independent of
the format slices and of each other except where the table says so; P1 and P2 (the owner's influence example, officers, XP)
come first.

## 12. Engine parameters

Decision 12: every rule value the engine uses is a field of the Battle Companies file, read at load into the typed ruleset and
carried into the views. There is **no code default**: a missing required parameter is an error (section 7 format), a value outside
its range is an error. The shipped file carries the **default** column (today's value), which is why the golden snapshot does not
change. Names are paths under `campaign` unless they start with `limits[]`, `gear[]`, `paths[]`, `creatures` or `wanderers`
(top-level). Ids (A1, B3, ...) are those of [`engine-parameters-audit.md`](engine-parameters-audit.md).

### 12.1 Dice specification grammar

```text
spec  = [ count ] ( "d" | "D" ) sides        count: integer 1..10 (default 1)    sides: integer 2..100
```

`"d6"`, `"2d6"`, `"3d6"`, `"d3"`, `"d66"` is **not** a spec (no concatenated dice), nor are modifiers (`"2d6+1"`), spaces or other
letters. The loader parses every dice string once into `DiceSpec{Count, Sides}` with `Min = Count`, `Max = Count*Sides`; a bad
string is an error naming the path (`campaign.injuries.roll: "2d6x" is not a dice spec; expected [N]dM, N 1..10, M 2..100`).
`Dice.Roll(spec)` returns the sum of `Count` uniform dice of `Sides`; individual dice (the 3D dice animation, the typed entry of
each die) are `1..Sides`; a total is `Min..Max`. Every table keyed by a roll (injuries, warrior and hero progression rows,
reinforcement and special charts, scenario tables, `maelstromOfBattle`) must cover `Min..Max` of its spec exactly once; the
validator and the decoder check it. The templates and `static/js` print and size the dice from the spec in the view (H3, H5,
H6, H9, H10, H12).

### 12.2 Parameters

Type `int` is a whole number, `spec` a dice string (12.1), `bool` a boolean. The range is validated by the schema and the Go
decoder.

**Influence, recruiting and rolls adjusted** (slice P1)

| Id | json name | Default | Type | Range / validation |
|---|---|---|---|---|
| A1 | `recruitment.adjust.maxPerAttempt` | 3 | int | 0..20; most points of adjustment per recruiting attempt, over both charts together |
| A2 | `recruitment.adjust.direction` | `"up"` | `"up"`, `"down"`, `"both"` | `down` and `both` allow lowering the die; lowering is paid at `costPerPoint`, counts towards `maxPerAttempt` and cannot leave the die range. Replaces the keys `direction` (prose value), `lowering` |
| A3 | `recruitment.adjust.costPerPoint` | 1 | int | 0..10; influence per point, either direction |
| A4 | `recruitment.die` | `"d6"` | spec, `Count` = 1 | every reinforcement and special chart covers `1..Sides` exactly once |
| A7 | `aftermathEffects[key=rendezvous\|vantagePoint].range` | `[-1, 1]` | `[int, int]` | each -6..6, low <= high; how far the perk moves a die |
| A8 | `company.startingInfluence` | 0 | int | 0..100 |
| A9 | `recruitment.rare.injuredCount`, `.deadCount`, `.promotedCount` | true, false, false | bool | which models of the profile count towards Rare N (injured count; dead and promoted do not) |
| - | `influence.spend[key=adjustRoll]` | name, text | strings | its `directions`, `costPerPoint`, `maxPerRoll` are removed: the numbers live once in `recruitment.adjust`; the entry keeps the player-facing name and text and the template prints the numbers from the view |
| - | `influence.gains.participating`, `.draw`, `.win` | 2, 1, 2 | int | 0..20 (already data) |
| - | `company` reinforcement cost, `treatment.*` | per company; 1 influence, target 5 | int | already data and read |

**Company shape, officers, limits, slots** (slices P2, P3, P5)

| Id | json name | Default | Type | Range / validation |
|---|---|---|---|---|
| B1 | `company.leaders`, `company.sergeants` | 1, 2 | int | leaders 1..3, sergeants 0..6; leaders + sergeants >= 1 and <= `maxModels`; the number of heroes of a starting company is their sum; "choose one leader and two sergeants" and the succession check read them |
| B3 | `company.heroStart` `{might, will, fate}` | 1, 1, 1 | int each | 0..5; what a new hero (and a Hero in the Making result, C6) starts with |
| B4 | `company.maxModels`; company `maxModels` | 15; Elite 12 | int | 1..50 |
| B6 | `company.equipmentSlots` `{small, large}` | 1, 1 | int each | 0..10 |
| B7 | `gear[].grantsSlots` `{small, large}` (Backpack: small 3) | none; Backpack 3 | int each | 0..10; replaces the Backpack name check |
| B8, B9 | `limits[].maximumPercentage`, `.roundUp`, `.classes` (new) | 33.33, true, the item classes of bow / throwing / Cavalry | number, bool, class list | percentage in (0, 100]; each class known to the gear vocabulary; the Go default `defaultLimitRules` is deleted |
| B10 | `creatures.rules.maxPerHero`, `wanderers.rules.maxAtOnce`, `paths[].limit` | 1, 1, 1 (Sorcerer) | int | required, 0..99 (the `max(1, ...)` fallback goes). There is no company-wide creature limit: "3 in the company" is the consequence of one creature per officer, so `creatures.rules.maxPerCompany` is deleted |

**Experience and progression** (slices P2, P4, P5)

| Id | json name | Default | Type | Range / validation |
|---|---|---|---|---|
| C1 | `progression.threshold` | 5 | int | 1..50; the single XP-per-level field (`experience.progressionThreshold` is removed); carried into `CompanyUnit` as `LevelEvery` |
| C2 | `progression.warrior.roll` | `"d6"` | spec | `progression.warrior.table` rows cover it |
| C3 | `progression.hero.rollsPerProgression`, `.choose` | 2, 1 | int | rolls 1..6, choose 1..rolls; the copies in `hero.resolution.{dice,pairs,choose}` are removed |
| C4 | `progression.hero.roll`; `paths[].dice` | `"2d6"`; the hero roll | spec | each path's rows cover its spec; a path without `dice` uses `hero.roll` |
| C5 | `progression.warrior.noPromotion.courage`, `.intelligence`, `.maximum` | 1, 1, 3 | int | 0..5, 0..5, 1..10; read as amounts |
| D1 | `injuries.roll` | `"2d6"` | spec | injury rows cover `Min..Max` |
| D2 | `experience.gains[].dice` and injury rows `grantsExperience.dice` | `"d3"`, `"d6"` where present | spec | any spec (the `d3`/`d6`-only code goes) |
| D3 | injury row `statChanges[].stat` | `fate` | stat | any characteristic, not only Fate |

**Break point and match** (slice P5, P4 for the dice)

| Id | json name | Default | Type | Range / validation |
|---|---|---|---|---|
| E1 | `breakPoint.threshold.comparison` | `"gt"` (casualties strictly greater than the Break Point) | `"gt"`, `"gte"` | replaces the prose value `strictlyGreater`; `rounding` is a fixed choice (none) and the key leaves |
| E4 | `breakPoint.whenBroken.courageTest.dice` | `"2d6"` | spec | the test passes when the total is at least the model's Courage (fixed) |
| E6 | `breakPoint.default.percent`; company `breakPoint` | 50; Muster of Isengard 66 | int | 1..100 |

**Rating, Against the Odds, scenarios** (slices P3, P5)

| Id | json name | Default | Type | Range / validation |
|---|---|---|---|---|
| F1 | `rating.hero.wargear.threshold` `{stats, sumAtLeast}` | `["attack", "wounds"]`, 3 | stat list, int | stats are characteristics; 0..20; a hero whose summed stats reach it pays the hero rating for gear |
| F2 | (no new field) | a stat with no `characteristicLimits.average` entry (Might, Will, Fate) pays for every point; the others pay only above their average | | |
| F5 | `scenarioSelection.die` (new), `scenarioSelection.tables[].roll` | `"d6"`; `[1,2]`, `[3,4]`, `[5,6]` | spec `Count` = 1, `[lo, hi]` | the tables' ranges partition `1..Sides` exactly once; each table lists exactly `Sides` scenarios (the second die picks one); the first die picks the table |
| - | `againstTheOdds.bonuses[]` rating and count, `kits`, `maxPurchases`, `budget.inclusive`, minor rule rating and cap, `rating.hero.perPoint[].rating`, armour stage prices | per bonus | int | already data and read; the web text prints them (H4) |
| - | `experience.gains[].amount` | 1 (per gain) | int | already data; the match page prints them (H7) |

**Armoury** (slice P5)

| Id | json name | Default | Type | Range / validation |
|---|---|---|---|---|
| G3 | `gear[].who.race` | Elven-made items: Elf | race word | replaces the code list of Elven races (armoury_race.go); a race known to the vanilla data |

### 12.3 The 13 fixed-choice mechanics that stay in code

They are behaviour with several code paths, not numbers. The json carries **no key** for them (the shipped keys that spelled them
are removed from the file, since unknown keys are errors); `docs/definitions.md` lists them under "what the engine implements".

1. A5: the adjustment budget is shared by the reinforcement and the special chart; unspent points do not carry over.
2. A6: the player pays every point needed to reach the wanted result; picking a lower result on the chart is free.
3. A9 (exhausted): when a Rare roll cannot be taken, the player takes a lower result (`lowerResult`). (The counting flags are parameters.)
4. C8: succession when officers fall (Sergeant becomes Leader, Hero becomes Sergeant, otherwise the warrior with most experience, tie the higher rating).
5. D4: a fled company gets Influence for participating (and Against the Odds), no Win or Draw and no scenario rewards.
6. E1 (rounding): the Break Point is not rounded (13 models: 6.5).
7. E2: what counts as a casualty (fallen, or fled after a failed Courage test; not a lost mount).
8. E3: which models count at the start (creatures and wanderers count, a composite model is one).
9. F3: injured models do not count towards the Rating.
10. F4: the higher Rating is Top Dog; a tie gives none.
11. F7: only the strictly lower-rated company chooses Against the Odds.
12. G1: a warrior who takes an option of his own profile pays that option's points, not the armoury price.
13. G2: a Beast takes no wargear and carries only the items listed for it.

### 12.4 Closed vocabularies (values are data, keys are the engine)

The validator rejects an unknown key: warrior table keys `noChange`, `promoted`, `heroInTheMaking` (C9); injury row property
`cancelsMissedGame` replaces the engine's `"justAScratch"` key (D5); the reasons a model skips the Courage test, from `whenBroken`
(E5); Against the Odds bonus keys `toolKit`, `wanderer`, `ambush` (F6); gear classes, aura kinds, path kinds, stats.

### 12.5 Safety limits that stay in code

Input sizes, timeouts, retries, page sizes, session lifetimes, the 60 hero-roll retries, the Tabletop Simulator mod limits and the
organiser grant caps (audit section 9; 13 entries). They are not rules and are on the allow-list of the magic-number lint, each with a
reason.

### 12.6 What the views and templates print

A number that is a parameter is printed from the view, never typed in a template: `LevelEvery` ("/ 5 XP"), `Roll.Budget`, `Adjust.CostPerPoint`
and `Adjust.Direction` ("up to 3, 1 influence per point"), the dice string of the injury, hero, warrior, Courage and scenario rolls,
the Against the Odds rating and re-roll count, `experience.gains[].amount`, the table ranges of the scenario selection, the officer
counts, the aura group radius (audit H1 to H13). A template test per slice renders a custom ruleset and asserts the number.

### 12.7 Validation messages for parameters

In the section 7 format: `campaign.recruitment.adjust.maxPerAttempt: 40 is out of range 0..20`;
`campaign.company.leaders: 0 with sergeants 0; a company needs at least one officer`;
`campaign.scenarioSelection.tables[1].roll: [3,5] overlaps [5,6]; ranges must partition 1..6`;
`campaign.injuries.roll: rows cover 2..11, 12 is missing`; `campaign.progression.hero.choose: 3 is more than rollsPerProgression 2`;
`campaign.breakPoint.threshold.comparison: "ge"; known: gt, gte`; a required parameter missing: `campaign.company.heroStart: missing`.

### 12.8 Test rows

`internal/core` `TestCustomRuleset` is table-driven, one row per parameter id, built with `customRuleset(t, mutate)` (the shipped
file, one mutation, a built `RuleSet`). Every row asserts the new behaviour **and** the view/page value, and the default row is
the golden snapshot itself:

| Slice | Rows |
|---|---|
| P1 | A1 `maxPerAttempt = 5`: adjust by 5 accepted, by 6 refused with "5" in the message, `Budget` 5. A2/A3 `direction = both`, `costPerPoint = 2`: a lowered die is accepted and costs 2 per point. A4 `die = "d8"` with a covering chart. A7 `range = [-2, 2]`. A8 `startingInfluence = 2`. A9 `injuredCount = false`: an injured Rare unit no longer blocks a recruit |
| P2 | C1 `threshold = 3`; B1 `leaders = 1, sergeants = 3`; B3 `heroStart = {2, 1, 0}` (new hero and Hero in the Making); B4 company `maxModels` |
| P3 | F1 `sumAtLeast = 4`; B8/B9 `maximumPercentage = 50`, `roundUp = false`, `classes`; B10 a missing value is an error |
| P4 | C2, C3, C4 (`hero.roll = "3d6"`, path `dice`), D1 (`injuries.roll = "d6"`), D2, E4, A4, F5 die: rolls, accepted typed ranges, chart coverage error, view dice |
| P5 | B6 `equipmentSlots {small: 2}`; B7 `grantsSlots`; E1 `comparison = "gte"` (exactly 50 % lost is Broken); E6; F5 ranges; C5 amounts; D3 a non-Fate stat; G3 race |
| P6 | C7 either read (a changed value changes the cap) or the key is rejected; each vocabulary rejects an unknown key |

A test that parses `internal/core/*.go` with `go/ast` fails on integer literals other than 0, 1, 2 and the allow-list of 12.5, and on
the names `XPPerLevel`, `MaxAdjust`, `maxSergeants`, `defaultLimitRules`, `HeroBonus` constants; report-only after slice 0, enforcing
after P6.

## 13. Decided questions

Kept for the record. One uploaded Battle Companies file (decision 7); rules by name with allowed inline definitions (8); unknown keys are errors
(9); validation returns every error and saves nothing (10); no converter for existing campaigns, development data is wiped and
re-seeded (a real campaign would need a one-off converter, say so if one exists). Former open questions 2 and 3, answered 2026-10-27:
unread prose is cut or goes to `reference/` as in decision 14 and `matchInput` stays and is shown; annotations, `extends` and brand-new
entities with no base are all accepted (4.2, 4.10).
