# Battle Companies json: backend audit

Audit of how `internal/core` reads `definitions/battle_companies/battle_companies.json` (920 KB), for the lean-format
redesign. Read-only; the json side is in `bcjson-audit-curator.md`. Bead `battle-company-manager-ce-v2-ejx`.

## 1. Who parses what

There is no single loader: the same 920 KB is `json.Unmarshal`led about 14 times, each time into its own anonymous
struct, and the results are glued in `BCBook` (`battlecompanies.go`). Parsers: `ParseBattleCompanies` (companies, wargear,
equipment, gearAnnotations, fill-ins, option sales, rule targets, keywords, limits, rating), `parseProfiles`
(`companyprofile.go`: start, effects, aliases, upgrades, oath), `ParseProgression` (progression, paths, powers,
advancements), `parsePathRules` (`companybuild.go`), `ParseInjuries`, `ParseCreaturesAndWanderers`, `ParseATO`,
`ParseAuras`, `ParseTTSModels`, `ParseScenarios` and the aftermath/options parser (`options.go`, `aftermath.go`),
`CaptureDuplicates` (`duplicates.go`, vanillaAmbiguities), `glossary_extras.go`. Names are compared with `ItemKey`
(lower case, punctuation, plural s, "made") or `strings.EqualFold` in about 20 places.

| json section | Go reader | Used by code | Unused or redundant |
|---|---|---|---|
| `campaign.company` | battlecompanies, options | maxModels, eliteMaxModels, oath (by alignment) | leaders, sergeants, startingHeroes, startingHeroStats, startingPoints, heroicTier, text: prose, nothing reads them (leader/sergeant counts and the 3 heroes are hard-coded in `startingcompany.go`) |
| `campaign.gold` | none | nothing | the whole section; `startingTotal` (target 60, "guideline") is not what the test enforces (105, a test constant) |
| `campaign.rating`, `characteristicLimits` | battlecompanies, progression | rating perPoint/perSpecialRule/minor/average, peak, lowerIsBetter | `limitKinds`, `increases`, texts |
| `campaign.progression/experience` | progression, aftermath | threshold, warrior table, noPromotion | xp, timing, influenceAdjust, mightAdjust, rerollSource, limits, hero: mostly prose; `experience.gains` read in aftermath |
| `campaign.injuries` | injuries.go | roll, tables, treatment, retirement | deathOfAHero, healing text |
| `campaign.influence`, `customCompany`, `matchInput`, `recruitment`, `breakPoint` (except default percent, courage, exceptions), `scenarioSelection` (tables only), `againstTheOdds` (ato.go reads it) | partly | see ato.go, live_break.go | `customCompany`, `matchInput`, `recruitment`, `influence` are never read (rule prose for humans) |
| `data.companies[]` | battlecompanies, companyprofile, progression | see below | `vanillaFaction` (18), `rareUnits` (18), `onTheirOwnPathRulebook` (17), every effect `layer` (129), `reason`/`evidence`/`source` provenance |
| `data.paths` | progression, parsePathRules | rows, maximums, startingStats, kind, selectable, offers | `pathNotes` never read |
| `data.gearAnnotations` (106 items, 64 KB) | battlecompanies, gearstats, poison | classes, aliases, grants, statChanges, abilities.defenceBonus/attacksBonus, chosen weapon, pricedAs, base, modifiers, profile, ranged, relatedTo | `evidence`, `status`; `abilities` other than defenceBonus/attacksBonus are dropped silently |
| `data.wargear/equipment` | battlecompanies | names, type, size, influence, rating, gold, restrictions | `wargear.restrictions`, `beasts`, text |
| `data.optionSales` (68), `reservedOptions` (10) | battlecompanies | status readAs/ownItem/notSold/granted, reason | `wargearReadAs` and `profileWargearNotSold`: not read at all (the first is the same information as optionSales readAs) |
| `data.unitFillIns`, `mountFillIns` | battlecompanies | race/faction/unitType patches onto vanilla | |
| `data.keywords`, `parameterisedRules`, `minorSpecialRules` | battlecompanies, progression | definition, label, minor kinds | |
| `data.ruleTargets` (4 patterns, 53 names) | ruletargets.go | all | `variants` of patterns (unused: the pattern is matched by base) |
| `data.auras` (52 entries, 26 excluded, 181 unreachable, 3 outOfScope) | auras.go | kinds, groups, entries | `excluded`, `outOfScope`, `unreachable` (documentation of what is not modelled; 24 KB) |
| `data.ttsModels` | ttsmodels.go | all 6 tables | `reason` fields |
| `data.creatures/wanderers`, `scenarios`, `scenarioDetails`, `scenarioScoring`, `aftermathEffects`, `vanillaAmbiguities` | creatures, options, aftermath, duplicates | yes | `validations`, `credit`, `limits[].text` |

Company fields: read by Go are name, alignment, maxModels, reinforcements (influence + table), specialChart,
startingCompany (units, label, role, gold), advancements, unitAliases, specialUnits, uniqueWargear, limitExemptions,
limitOverrides, heroUpgrades, specialRules, grantedSpecialRules, ruleExchanges, ruleSubstitutions, companyLimits,
onTheirOwnPath, oath, epigraph, notes, breakPointPercentage, description and `effects` of 13 kinds. Not read: see table.

**Duplication inside a company.** `effects[]` (21 KB, 129 entries, `layer` management/match/both) repeats information that
sits in top-level company fields: kinds `heroUpgrade` (44), `maxModels` (15), `unitAlias` (9), `ruleSubstitution`,
`uniqueWargear`, `limitExemption`, `limitOverride`, `grantedRule`, `specialUnit`, `companyLimit`, `ruleExchange`, `aura`,
`breakPoint`, `onTheirOwnPath` (17) are never read from `effects` (97 of 129 entries); only `startingHeroRestriction`,
`startingHeroNoStatGain`, `recruitAsHero`, `rangerReplacement`, `unitTypes`, `includedUnitPoints`, `modelCount`,
`recruitFallback`, `compositeModel`, `heroPromotion`, `reinforcementReplacement`, `raceBalance` and `onTheirOwnPath`
(path/units) are. So the code reads some facts from the top level and some from the effects list of the same company.

## 2. Code that only copes with the shape, and what replaces it

Each item: what the code does today, then the data/loader change.

1. **Fourteen parsers over one file.** Replace by one typed model (`Ruleset` structs, one `Unmarshal`, or one
   `Decode` that also validates) with the other readers consuming it. Deletes ~600 lines of struct tags and the
   re-parse per concern; lets unknown-field detection (`DisallowUnknownFields` on the lean format) catch typos that today
   silently drop (`abilities` other than two keys, `keywordMatch` strings).
2. **Name resolution at read time.** `canonicaliseWargear` (battlecompanies.go, ~90 lines), `ItemKey` fuzzy match
   (plural s, "made", `&`), `spell` map, `Company()` and `Profile()` case-insensitive fallbacks ("Lake-Town"),
   `resolveUnit`, `ownNames`/`rows(... ownNames)` swapping a chart row's `unit` for its `label`, `SpellWargearLikeVanilla`.
   Cause: names are free text with several spellings (`shield` vs `Shield`, `Orc bow` vs `Orc Bow`, `label` vs `unit`).
   Instead: ids (stable slugs: `unit:warrior-of-minas-tirith`, `gear:orc-bow`, `company:lake-town`) with a display name,
   resolved **once** at load into index maps; chart rows, advancements, start units, aliases reference ids; the loader
   rejects an unknown id. Deletes canonicalise/spell/ItemKey-in-hot-paths and the case-insensitive fallbacks; `ItemKey`
   stays only for the vanilla-name bridge. Risk: persisted state stores names (see section 4).
3. **Unit aliases as a delta (`unitAliases`, 7 companies, plus 9 `effects`).** `UnitAlias{Label, Unit, AddRules, Selected,
   Keywords}`; code: `buildUnit` alias loop, `ownNames`, `ChangesUnit`, `preview`, `ApplyUnitFillIns`, `Advancement` label
   stripping, `listedNames`, `resolveUnit`. Instead `units[]` of the company: `{id, name, extends: <vanilla unit>,
   addRules, selectOptions, addKeywords, types}`; resolved once into a `Unit` value used by every consumer, so the
   alias special case in buildUnit/preview/armoury/promotion/audit/tts (6 places) becomes one lookup. A pure legacy
   spelling is just a second name on the same unit (`names: [...]`), which removes the `ChangesUnit` distinction.
4. **`effects` kinds parsed by a 120-line switch.** `typeOverride`, `compositeModel`, `includedUnitPoints`, `modelCount`,
   `recruitFallback`, `heroPromotion`, `rangerReplacement`, `startingHero*`, `raceBalance`, `recruitAsHero` each feed one
   special field on `CompanyProfile` (30+ fields, many used once; 22 of them are single-company: Warg Marauder, Vault
   Warden Team, Shire Ranger). Instead typed per-unit properties: `unit.types`, `unit.composite`, `unit.includes`,
   `unit.models`, `unit.promotesTo`, company `startRules` (`noStartGain`, `heroOnly`, `recruitAsHero`) and
   `recruit.fallback`. Same behaviour, no `kind` string switch, and a new company can use them without a Go change.
5. **Name-keyed Go tables.** `keywordSpellings` (`keywords.go`: Lothlorien/Iron Hills/Misty Mountains merges),
   `breakEffectText/breakWhoText/courageSkipReasons` (live_break.go), `statOrder`, `ttsRoleLabel`, `iconRules`
   (`company.go`, item class to icon), `settingFields` (`options.go`). Keyword spellings belong in the vanilla-bridge
   data (`aliases` on the keyword), break texts in `breakPoint.exceptions[].text` (already there), icons in
   `equipmentClasses[].icon`. `defaultLimitRules` and `limitLabel` hard-code the limit labels over the json `limits`.
6. **Armoury (`armoury.go`, `armoury_race.go`, `gearidentity.go`).** Special cases: `elvesOnly` (checks a `modifiers`
   tag `elven`), `beastMayCarry`, `uniqueReason` (three ways to say "who may take it": `Only` text, `OnlyUnits`, `OnlyKind`;
   plus a string parse of " heroes"/" hero" suffixes and `HeroUpgrade.Allows`' plural stripping), `legalWargear` (a
   warrior may only buy wargear matching its profile options, heroes any option of any unit in the charts, built by
   `heroOptions` walking six sources), `goldPrice` (a warrior pays the option's points for a lone option, not the
   armoury price), the limit loop in `whyNot` hard-coding `bow/throwing/cavalry` class to limit id. Instead:
   one restriction type `who: {hero?: bool, race?, units?: [id], unitTypes?, alignment?}` for every item (unique or
   common); `limits[].classes` naming the classes a limit counts (deletes the three-way `ranged` expression);
   `gear.perRole: {warrior, hero}` for influence/rating/gold; unit options as structured `options[] {id, gear:[ids],
   exchange?, cost}` so `OptionItems` (regex splitting "Shield and Spear", "Exchange X for Y") is only needed in the
   vanilla bridge. Then `legalWargear` is a set lookup, `heroOptions` a precomputed `company.offerableOptions`.
7. **Promotion gear (`promotion_gear.go`, 146 lines).** A heuristic reconstructing what a promoted warrior keeps:
   `listedNames`, `knownAs`, `carriedItems` (guesses whether a stored string is an item, an option label or an
   exchange), `keepsOnPromotion`. It exists because inventory is stored as free strings that may be option labels. With
   structured options and item ids stored per unit, the rule is "keep equipment; keep gear whose id is in the new
   profile's listed ids"; the label parsing goes.
8. **Poison (`poison.go`).** `ChosenWeapon`/`ChosenWeaponName` parse `"Envenom Weapon (Bow)"` by cutting a suffix `X)`;
   `PoisonRule` is a Go constant; the weapon classes live in the annotation. Instead the annotation says
   `grantsRuleForChosenWeapon{rule, weaponClasses}` (already) and the unit state stores `{item: id, weapon: id}`, no
   string surgery. `Progress.Minor` entries `Name|weapon:x` are the same pattern ("|" micro-syntax in `buildUnit`).
9. **Path rules from prose (`parsePathRules`).** The rule's description is the table `text` cut at the first " - ";
   the rule name is in `options[].specialRule`. Instead rows hold `{roll, rule: {name, kind: active|passive, text}}`.
10. **Rule targets (`ruletargets.go`) and auras (`auras.go`).** Two tables keyed by rule/item name, each with its
    own matching (`foldRule`, `family`, `keys`), 53 + 52 hand-written rows, many of them for rules that are a vanilla or
    BC keyword. Instead `rule.target` / `rule.aura` as properties on the keyword/gear definition itself (one place to add
    a unit or item), the tables derived at load. The `outOfScope/excluded/unreachable` lists (24 KB) become a test
    fixture, not shipped data.
11. **TTS (`ttsmodels.go`, `ttsnames.go`, `ttslist.go`).** `standIns`, `mounts`, `riders`, `names`, `defaultOptions`,
    `markerOptions`: six name-keyed tables of exceptions between the vanilla spelling and the mod's. Instead a per-unit
    `tts: {model, foot, options, alternatives}` on the unit and per-gear `tts` name, merged by id; `markerOptions` and
    `defaultOptions` disappear once alias options are structured (item 3).
12. **Company effects used by profile pages.** `CompanyProfile` carries `Rules []string`, `Substitutions`, `Exchanges`,
    `Granted`, `Notes[section]`, `Epigraph`, `Oath`: prose for display. Keep as `text[]`, drop the sections only the audit
    or the page branch on, and default the oath by alignment in the loader (already half done in parseProfiles).
13. **Defaults/inheritance that already half exist and should be uniform.** `MaxModels` default, `LimitOverrides`,
    `Oath` default by alignment, `defaultLimitRules`, `Count = max(1, …)`, `Group = When`. Make them one loader step:
    company `extends` a base company (shared reinforcement cost, hero-upgrade shape, limits), unit `extends` vanilla,
    gear `extends` another gear (Elven-made Glaive `extends` Glaive, replaces `pricedAs`/`base`/aliases), path `extends`
    a path (the 11 heroic paths share most `maximums`).

What the loader could do (summary): defaults (count 1, rare 0, maxModels, oath, rating 0), `extends` (company, unit,
gear, path), derive-from-vanilla (race/faction/unitType patches: `unitFillIns`/`mountFillIns` become properties of the
BC unit/mount, gear class from the vanilla `type` when not annotated, 35 of 106 annotations are `status: derived`, i.e. computed from vanilla),
resolve-once indexes (id to entity, name and alias to id, per-company offerable options, advancement by `(unit)`).
Estimated deletion: ~1,500 lines of `internal/core` (the 14 parsers' struct tags, canonicalise, promotion_gear, alias
and effect switches, name-keyed tables) and 40 to 50% of the json (effects duplicates, provenance, unused sections).

## 3. Golden-snapshot plan

Goal: one test that renders everything derived from the data, committed before any format change, and that must stay
byte-identical after the migration (ids and format may change; the *rendered names and values* may not).

**Exists**
- `internal/core/testdata/auras_golden.txt` + `TestUnitAurasAllCompanies` (`-update-auras`): auras of every start,
  reinforcement, special, advancement unit of the 37 companies as warrior and hero, and per path.
- `internal/core/testdata/tts_golden.txt` + `ttslist_golden_test.go` (`-update-tts`): TTS figures and notes per start,
  reinforcement, special chart, special unit, advancement.
- `TestStartingCompanyPointsPlusGoldAre105InEveryCompany` (`startingtotal_test.go`): points + gold = 105 for every
  officer choice (the 105 and the Defenders of the North 106 exception live in the test, not in the data).
- `startingcompany_test.go`: officer locks per company.
- `make audit` (`audit.go`, `audit_render.go`, stable output): per company the start, recruiting charts, company rules,
  per unit stats/wargear/options/points/armoury offers (`whyNot` for warrior and hero), tooltips, limits, duplicates.
  It is a ready-made base, but not a test: nothing compares it to a committed copy.

**Missing (to add as `snapshot_test.go` + `testdata/bc_snapshot/<company>.txt`, one file per company so a diff names the
company; `-update-snapshot` flag like the others)**
1. Commit the `make audit` output as the golden (37 files + index) and diff it in a test. Covers rosters, costs, options,
   armoury offers, limits, tooltips. Check what it omits and add: rating/gold of every offer for warrior and hero,
   `Restrictions`, unique-item `Only/OnlyUnits/OnlyKind/Limit`, `Reserved/Unsold/ReadAs` per option, gear classes and
   `Stats`, `Grants/GrantedTo`.
2. Promotion: for each advancement of each company, the result of `Advancement.Matches` and `dropUnlisted` for a unit
   carrying each combination of its options (kept/dropped gear) and as a hero; the promotion choices list.
3. Hero levelling: path tables (rows, options, maximums, starting stats/actions/powers), `OfferFor` for fixed 2D6 rolls
   per path, warrior table, hero upgrades with their `Allows` over every unit of the company, `improvedStat` limits
   (peak, lowerIsBetter), power choices and minor rules.
4. Rule targets: for every keyword rule with a target (names + patterns expanded over the keywords of all units), the
   direction/target text/keyword list.
5. Profile data not in the audit: effects as used (`Types`, `Composites`, `IncludedPoints`, `ModelCounts`, `Fallbacks`,
   `Promotions`, `StartHeroes`, `NoStartGain`, `RecruitAsHero`, `RangerReplacement`, `Balance`, `OwnPath`), hero
   upgrades, granted rules, exchanges, substitutions, key-word limits, oath, break-point percentage, aliases resolved.
6. Starting gold and points per company: `StartGold`, points of default heroes; make the 105 and its exception data, and
   assert it from the snapshot (`startingTotal` today also contradicts `campaign.gold.startingTotal` 60).
7. Campaign tables as used: injuries (warrior and hero chart, modifiers, treatment, retirement), experience gains and
   threshold, rating per point and caps, break point default/exceptions, Against the Odds tables and bonuses, aftermath
   effects, scenarios selection, creatures and wanderers (stats, cost, limits), reservations and option sales.
8. Name resolution map: for every name a company mentions (unit, label, wargear, option), the resolved entity and its
   canonical spelling (this is what the migration most risks changing).
9. Rendered pages: optionally the HTML of the company profile and roster for the 37 companies via the existing web
   tests, to catch template-level drift (not required if 1 to 8 hold).

Rule for migration: the snapshot serialiser reads only public `core` API (`BCBook`, `CompanyProfile`, `BuildDetail`,
services), never json, so it survives the format change unchanged. Any intended difference is edited in the golden in
a separate, reviewed commit.

## 4. Proposed order of work and risks

1. **Freeze behaviour**: items 1 to 8 of the golden plan; commit; `make test` green. Also move `105` and its exception
   into the data (or at least out of the test constant) so the snapshot owns it.
2. **Agree the schema with the curator**: ids, `extends`, unit/option/gear/restriction shapes (section 2 items 3, 4, 6),
   which sections are runtime data and which are documentation (to `docs/` or `sources/`: `customCompany`, `matchInput`,
   `recruitment`, `influence`, `unreachable`...). Write the Go model (structs) first as the contract in `internal/core`.
3. **One loader**: a single `Decode` into the typed model with `DisallowUnknownFields` and referential validation
   (every id exists), plus resolve-once indexes. Build it beside the old parsers with a converter test producing the
   same `BCBook` from the new file (the curator generates the new file from the old one in `tools/`); no old-format
   loader remains after the switch.
4. **Switch `BCBook` internals** section by section (companies/charts, then armoury/gear, then paths/progression, then
   rule targets/auras/tts), deleting each old parser and name table as its section moves; run the snapshot after each.
5. **Delete** `canonicaliseWargear`, `promotion_gear` heuristics, the effects switch, `keywordSpellings`, the legacy
   spelling fallbacks, and the ATO/auras/TTS parsers' private structs.
6. **Extension docs**: `docs/definitions.md` becomes "how to add a company, unit, item, path" with the loader's
   validation errors as the test of that claim; add a sample custom ruleset test.

**Risks**
- *Persisted names.* Postgres stores unit base names, gear names, path names, `Progress` keys (`power:`, `minor:Name|weapon:x`),
  company names and campaign settings; a clean break of the json renames or re-ids them. Either keep display names
  identical and resolve by name (recommended: id is internal, names unique per kind), or ship a migration with the
  database-master. The `Lake-Town` fallback shows names already drifted once. The demo world and e2e seeds also carry names.
- *Rulesets are content-hashed and per-campaign.* Existing campaigns point at a stored old-format file; the "no
  old-format loader" decision means those campaigns need re-pointing or a one-off conversion. Decide with the owner.
- *Loss of prose.* Many sections look unused to Go but are shown to humans (`campaign.*` prose, `description`, notes)
  or consumed by `tools/validate_definitions.py` and the concept graph (`build_concept_graph.py` reads the json);
  grep `tools/` and `web` templates before dropping a key (this audit only searched Go).
- *Vanilla bridge.* Vanilla duplicates (91 `vanillaAmbiguities`), first-occurrence-wins and spelling variants must stay
  explicit in the bridge, or the units silently change.
- *Hot-path assumptions.* `Profile()` and `Offer()` are linear scans today; indexes change iteration order, which
  goldens will expose (sort in the serialiser).
- *Behaviour hidden in heuristics* (`CanLead` substring match on unit names, `HeroUpgrade.Allows` plural stripping and
  substring match, `OptionItems` regex): replacing them by exact ids will change edge cases; the snapshot will show
  each one and the owner decides case by case.
