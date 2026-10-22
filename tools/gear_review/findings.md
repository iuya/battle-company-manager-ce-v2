# Gear annotations: reviewer findings

Review of bead `battle-company-manager-ce-v2-wgv` (curator's work: bead `a2g`). Reviewer: gear-classification-reviewer.
Nothing in `definitions/` or `tools/bc/` was edited; the amends below are proposals for the user to accept and the
curator to apply in `DECISIONS` (`tools/bc/gear.py`).

## Summary

- **Items checked: 109 of 109** (every row of `data.gearAnnotations`). I read each item's text in the vanilla file
  (`data.gear`, units' `wargear` and `options`) and in the BC file (`wargear`, `equipment`, company unique wargear), and
  in `sources/` where the text was silent, and decided classes and grants *before* opening the curator's column.
- **Agreement: about 90 items** (classes, main class, ranged values and grants all match my own derivation). All
  ranges and strengths that are present match the text (bows 24/18, crossbow 24/S4, throwing 6"/8", Troll Chain 3"/S5,
  Whip 2"/S1). All 23 `grants` entries resolve and, except where noted below, each has a sentence saying the bearer
  gets the rule ("has/gains the X special rule", `grantsSpecialRules`, "grants the bearer the ... keyword"). No false
  positive grant found. The decisions the curator listed (spear never with melee, Whip, Wood Elf Spear, Light Shield,
  Torching Brand, Envenom as modifier, the four Company items) are all consistent with the texts and are not
  questioned below.
- **Disagreements and doubts: 14 findings touching about 27 items.** By kind:
  - missed grant: 1 (Company Horn)
  - inconsistency between like items: 1 group of 7 (hand-and-a-half weapons)
  - items no company can carry: 5 (named-hero unit upgrades)
  - missing `ranged`: 2
  - class/grant doubts that need a rules decision: Blowpipe, Lance, Barding, Dunlending War Axe, Flaming vs Torching Brand
  - mount rules as "grants", Elven duplicates, Whip and company limits: lower-key notes.
- **Coverage:** my own walk of the 113 vanilla units a company can field (wargear + options, bundles and "Exchange X
  for Y" split into parts), the BC tables, unique wargear, creatures and wanderers found **no reachable item without an
  annotation** and no annotation that does not exist in the data. The over-collection is the opposite problem (finding 3).
- `python3 tools/validate_definitions.py`: **0 errors, 0 warnings** (confirmed). It misses several things, see "Validator".
- **Confidence:** high for coverage, ranges and grants; medium for the rules-interpretation items (marked as questions).
- **Seriousness:** nothing here breaks the file. Three amends are worth applying before the app consumes the data
  (findings 1, 2, 3); the rest are decisions for the user.

## Disagreements and doubts

| # | item | curator says | you say (reviewer) | evidence (the text) | proposed amend | severity |
|---|---|---|---|---|---|---|
| 1 | Company Horn | `instrument`; grants Command Support | same class; grants **Command Support and Dominant (2)** | Company Horn: "This is a War Horn that grants the bearer the Command Support keyword". War Horn: "a model with a war horn gains the Dominant (2) special rule" | add `Dominant (2)` to Company Horn's grants (a "This is a X" item inherits X's grants, see systematic issue S1) | medium |
| 2 | Hand-and-a-half weapon, Hand-and-a-half hammer, Elven hand-and-a-half weapon, Elven-made hand-and-a-half weapon, Elven-made Hand-and-a-half sword vs Berserker Blade | the five: `["melee"]`; Berserker Blade: `["melee","two_handed"]` | all six the same | Rulebook, Hand-and-a-half Weapon: the wielder "can choose to fight with it as either a hand weapon or a two-handed weapon". Berserker Blade: "This is a hand-and-a-half weapon". The shield rule also treats the two-handed use specially | give all six `["melee","two_handed"]` (recommended; also what the app's current guess does for "hand-and-a-half") or reduce Berserker Blade to `["melee"]` and keep its extra rule as text | medium |
| 3 | Hearthguard (grants Burly), Westfold Redshield, King's Guard, Guardian of the King, Venom-back | annotated, `other` | **not reachable from any company**; should not be annotated | These are unit options with `availableIn` a named hero's own list: Durin, Erkenbrand, Gil-galad, Thror, Ashrak. No company `unitAliases` selects them (the five that are reachable do: Helmingas, Noldorian Exile, Lorien Guard, Battlin' Brandybuck, Tookish Hunter). Venom-back's own evidence says "no rules text in the sources" | skip options whose `availableIn` lists only named heroes unless a company alias selects the option (S2). That removes 5 items (109 -> 104); Hearthguard's Burly grant disappears with it | medium |
| 4 | Bow with poisoned arrows | `bow`, **no `ranged`** | `bow` + `ranged` 24"/S2 | Same bow as "Bow" (Haradrim lists name it only "bow with poisoned arrows"); the Haradrim Warrior already has `Poisoned Attacks (Bow)` in its own rules, so the arrows add nothing to grant | add `ranged {24, 2}` (S3) | low |
| 5 | Smoke Bombs | `throwing`, no `ranged` | `throwing`, `ranged` 6"/S1 | "These are a throwing weapon with a Strength of 1" (no range printed, so the generic throwing weapon's 6") | add `ranged {6, 1}` once the user confirms the 6" (Q2) | low |
| 6 | Blowpipe | `other` + ranged 12"/S2, no grant | see Q1 (class) and Q2 (grant) | Vanilla: "12" Range, 2 Strength. Benefits from the Poisoned Weapons special rule (re-roll 1s to wound)". "Poisoned Weapons" is **not a keyword** in either file (the rule is `Poisoned Attacks`), so the validator cannot see the mention | decide Q1/Q2 | medium |
| 7 | Lance, Elven lance | `spear` | `spear` is defensible but not what the rulebook says | Rulebook: a lance gives +1 to wound on a Charge; only the spear and pike Support. War spear "follows the rules for a spear" for Infantry and "for a lance" for Cavalry. The app's own `EquipSpear` comment already reads "spears, pikes, lances" | keep (consistent with the app) or `["melee"]`; Q3 | low |
| 8 | Barding | `armour`, grants Expert Rider, Horse Lord | doubt: `armour` of the *steed*, no Defence modifier | Equipment: "The rider gains the Expert Rider & Horselord special rules." No `modifiesStats`. `Horse Lord` resolves to the vanilla keyword (the text spells it "Horselord"; fine) | keep `armour` (what you can point at) but make sure the app does not read it as the rider's armour stage; or `other` (Q4) | low |
| 9 | Dunlending War Axe | `spear, two_handed` | unverified | Only source: BC wargear table "Simple hybrid-weapons (e.g. Axe of Lossarnach or Dunlending War Axe)". No text in vanilla (Dunlending Huscarl carries it) or BC says *which two* weapons it combines. The evidence reads "like the Axe of Lossarnach", which is an inference | keep, but mark the evidence "inferred from 'hybrid-weapon'"; Q5 | low |
| 10 | Flaming Brand (`melee`) vs Torching Brand (`other`) | different classes for two items with the same Terror grants | defensible difference, record it | Flaming Brand: BC wargear table type "weapon"; Hill Tribesman keeps his hand weapon and picks the brand in the same choose-one group as spear/two-handed. Torching Brand: Large *Equipment* ("A model with a Torch gains..."), plus Dominant (2) | no change; add the reason to the two `why` strings so the next reviewer does not flag it | info |
| 11 | Pony (Timid), Fell Warg (Fell Sight), War Camel (Impaler), War goat (Mountain Dweller, Devastating Charge), Galadhrim Steed (Woodland Creature) | mount `specialRules` recorded as grants of the item | keep, but they are the **mount's** rules | Timid: "If a Mount has this special rule...". Impaler: "this model Charges". The text of the Steed ("This is an armoured horse that also has Woodland Creature") is the only one that says it directly | keep (brief allows "a mount's `specialRules`"); Q6 asks whether the card should show them as the unit's or the mount's | low |
| 12 | Elven Glaive / Elven-made Glaive; Elven hand-and-a-half weapon / Elven-made hand-and-a-half weapon | four separate items (the generator's own sheet flags them "possibly the same, NOT merged") | same items: BC says "Elven-made weaponry uses their generic equivalent values" | Same classes in each pair, so no wrong class today | optional: ignore "made" in `item_key` and merge as aliases (S4); harmless if left | low |
| 13 | Whip (`throwing`, `melee`) | decided | fine, but a limit consequence | BC, Sharkey's Rogues: "Whips do not count against the Throwing Weapon Wargear Limit." The app's `rank()`/limit logic would treat any `throwing` first class as counting | no data change; the app must exempt Whip by name or by a flag (S5) | info |
| 14 | Helmingas, Noldorian Exile, Lorien Guard, Battlin' Brandybuck, Tookish Hunter (and the 5 of finding 3) | `other`, no flag | correct class, but they are **unit upgrades, not items** | Every `why` already says "named unit upgrade (a package of stats/rules), not a physical item" | add a flag like `weaponModifier` (e.g. `unitUpgrade: true`) so the card does not show a gear icon (S6) | low |

## Systematic issues

- **S1: "This is a X" does not inherit X's grants.** `derive()` reads grants only from the item's own sentence. Company
  Horn ("This is a War Horn") loses Dominant (2). Fix: in `derive()`/`DECISIONS` resolve a leading "This is a(n) X" to the
  annotated item X and union its grants (classes stay curated). Affected today: Company Horn only (Company Drum, Company
  Instrument, Company Standard inherit nothing; Elven Glaive, Wood Elf Spear, Small Shield inherit nothing either, the
  last two being the user's no-grant decisions).
- **S2: the collector ignores `availableIn`.** `collect()` takes every option of every reachable unit, including
  options reserved to a named hero's army list. Fix: skip an option whose `availableIn` contains only hero names unless
  a company `unitAliases` entry selects it in `selectedOptions` (that is how the five legitimate ones arrive).
  Option entries whose `availableIn` names *armies* (Knight of Dol Amroth's banner, horn, lance) stay.
- **S3: ranged is never required.** Fix in the rule: every item whose main class is `bow`, `crossbow` or `throwing`
  must have `ranged`; derive it from the nearest sibling ("Bow with poisoned arrows" = "Bow").
- **S4: `item_key` keeps "made".** The sheet already lists the pairs; folding "made" would merge them.
- **S5: class list vs company limits.** The app counts `bow`/`crossbow` and `throwing` against the Bow and Throwing
  limits. Exceptions that the data cannot express: Whip (explicit in the BC text), Blowpipe and Smoke Bombs (the text
  is silent). A boolean (e.g. `countsAgainstLimit: false`) would carry the Whip rule as data instead of in Go.
- **S6: unit upgrades and weapon modifiers are mixed with items.** One flag already exists (`weaponModifier`); the ten
  named upgrades want one too.

### Validator (`tools/validate_definitions.py`), things it misses

1. **Coverage check is circular**: it calls the same `collect()` as the generator, so a gap or an over-collection in
   `collect()` passes. I repeated the check independently (units from company strings, all `wargear` and `options`
   names, bundles split): no gap; the over-collection of finding 3 is invisible to it.
2. `ranged` is accepted on any `other` item (the rule is "bow, crossbow, throwing or other"); restrict `other` to
   items named in a short allow-list (Blowpipe).
3. No check that a `bow`/`crossbow`/`throwing` main class has `ranged` (findings 4, 5).
4. No check that `other` is never combined with another class, nor that `spear` is not combined with `melee`
   (a user decision that currently holds only by hand). Both are one-liners.
5. A grant that names something not resolvable cannot be flagged when it is only a *mention*
   ("Poisoned Weapons" on the Blowpipe): the mention scan only knows current keyword names. The "Open questions" section
   of `tools/gear_review.md` is empty, which would be wrong after finding 6.

## Questions only the user can settle

- **Q1. What class is the Blowpipe?** (unconfirmed by the curator.)
  (a) `other` + `ranged`, as now: not counted against any limit, generic icon. (b) `throwing`: the BC text groups
  "Blowpipe or Throwing Weapon" for the move penalty, and `ranged` exists for it, but it would count against the
  Throwing limit, which the BC never says. (c) a new class `blowpipe`/`missile`: cleanest, but changes the enum, the
  schema and the app. **Recommendation: (a)**, with the validator allow-list of finding S3; revisit if the app wants a
  missile icon.
- **Q2. Does the Blowpipe grant a rule, and are Smoke Bombs 6"?**
  The Blowpipe benefits from "Poisoned Weapons" (an old name of Poisoned Attacks). Options: (a) no grant (current);
  (b) `grants: ["Poisoned Attacks (Blowpipe)"]`, the same representation as Envenom Weapon (X) and the Haradrim
  `Poisoned Attacks (Bow)`. **Recommendation: (b)**, medium confidence. For Smoke Bombs: **6"** (generic throwing
  weapon), since the text gives only S1.
- **Q3. Lance and Elven lance as `spear`?** (a) keep, matching the app ("spears, pikes, lances") and the BC wargear
  row "Pike, Spear, Lance, Warspear"; (b) `melee` because a lance does not Support. **Recommendation: (a)**, because the
  class drives icon and sort order, not rules; note it in the lance `why`.
- **Q4. Barding: `armour` or `other`?** **Recommendation: keep `armour`**, with the grants, and tell the app it is
  the steed's armour.
- **Q5. Dunlending War Axe: which hybrid?** The BC only calls it a "simple hybrid-weapon". **Recommendation: keep
  `spear, two_handed`** (the example it is given with) and check against the Dunlending list in the army book when
  convenient.
- **Q6. Mount rules on the card.** (a) show them as the unit's rules (current data); (b) keep them but mark them as the
  mount's (e.g. `grantedTo: "mount"`). **Recommendation: (a) for now**; the display-only policy of the project makes
  the difference cosmetic.
- **Q7. Hand-and-a-half weapons** (finding 2): confirm `["melee","two_handed"]` for all six.

## Next steps (in order)

1. User answers Q1–Q7 and accepts or rejects findings 1–14.
2. Curator applies the accepted amends in `DECISIONS`/`collect()` (S1, S2, S3 first), rebuilds, runs
   `python3 tools/validate_definitions.py`, and regenerates `tools/gear_review.md`.
3. Close bead `battle-company-manager-ce-v2-wgv` when the findings are resolved and the validator reports 0 errors.

## User decisions (first round)

- Finding 1 (Company Horn gets Dominant (2)): accepted.
- Finding 2 / Q7 (all six hand-and-a-half weapons `["melee","two_handed"]`): accepted.
- Finding 3 (five hero-only unit upgrades): **keep annotated**, in case a campaign organiser hands them out. Rule S2 is
  dropped; they stay reachable on purpose (record this in the generator so the next reviewer does not flag it).
- Finding 4 (`ranged`): accepted for Bow with poisoned arrows (24"/S2) and Smoke Bombs (6"/S1; throwing weapons default to 6").
- Finding 6 / Q2: the Blowpipe is a poisoned weapon like any other. Open: how "any other poisoned weapon" is recorded
  (see the follow-up question below).
- Open: new classes for items now `other` (asked of the user).

## User decisions (second round)

- **New class `consumable`**: Healing Herbs, Wondrous Cram, Dwarven Brew, Scroll of Courage, Scroll of Hidden Paths
  (used up once). Enum, schema and the app's `EquipmentClass` need it.
- **New class `missile`** (Blowpipe, with `ranged` 12"/S2): a ranged weapon like bows and crossbows that does **not**
  count against the Bow/Throwing limits. The validator's `ranged` rule becomes `bow`, `crossbow`, `throwing`, `missile`
  (and no longer `other`); the app's `isRangedLimit` must not include it.
- **Poison is recorded as a grant on both**: Bow with poisoned arrows grants `Poisoned Attacks (Bow)`, Blowpipe grants
  `Poisoned Attacks (Blowpipe)`.
- All other miscellaneous items stay `other` (cloaks, belt, boots, badge, ring, pack, ropes, map, traps, stone, talisman, pelt, Torching Brand).

## Limit exemptions (user, third round; proposal, supersedes S5)

The BC text has four exemptions, all written as **Company Special Rules**: Rohan: Riders of Rohan count towards neither
the Bow nor the Cavalry limit; Paths of the Druadan: Riders of Rohan not towards Bow; Grand Army of the South: Khandish
Horsemen not towards Bow; Sharkey's Rogues: Whips not towards Throwing. Two are about units, one about an item, one
both a unit and two limits, so the item annotations are the wrong place for it (a Whip counts everywhere else).

Proposal for the curator (needs the user's go-ahead):
1. Give each entry of the top-level `limits` array a stable `id` (`bow`, `throwing`, `cavalry`).
2. Per company, a `limitExemptions` list: `{"unit": "Rider of Rohan", "limits": ["bow", "cavalry"], "text": "<short quote>"}`
   or `{"item": "Whip", "limits": ["throwing"], "text": "..."}`. Exactly one of `unit` / `item`; `limits` are ids from
   step 1; the validator checks unit/item exist and that the ids exist. The generator fills the four cases by hand.
3. Semantics (decided by the user): an exempt model is left out of **both** the count and the base the maximum is
   worked out on. Example: 2 Warriors of Rohan + 10 Riders of Rohan, Rohan company: the base is 2, so the Bow and the
   Cavalry maximum are ceil(2 / 3) = 1 each, and the 10 Riders count towards neither.
4. `gearAnnotations` stays unchanged: `missile` (Blowpipe) simply belongs to no limit; no per-item flag is needed.
5. The app's limit check reads the class (bow/crossbow, throwing) or the Cavalry keyword to find what counts, then
   subtracts the company's exemptions.

## User decisions (fourth round: Q3 to Q6)

- **Q3: new class `lance`** (cavalry spear, own icon), distinct from the infantry `spear`.
  - Lance, Elven lance: `["lance"]`.
  - War spear (spear dismounted, lance mounted, per the rulebook) and Warspear: `["spear","lance"]`. "Warspear" (BC wargear
    row) and "War spear" (unit lists) are one item: merge them as aliases.
  - Needs: the enum in the schema and validator, `EquipmentClass` in the app (`lance`, sorted with the melee weapons),
    an icon (`tools/build_icons.py`), and the app's name guess `has(n, "spear", "lance", "pike")` split.
- **Q4: Barding stays `armour`** with its grants (Expert Rider, Horse Lord go to the rider). The app must not read it as the
  rider's armour stage.
- **Q5: Dunlending War Axe stays `["spear","two_handed"]`, now confirmed.** The vanilla file defines it as a keyword
  (`data.keywords`, not `data.gear`, which is why it was missed): "This can be used as either a spear or a two-handed
  weapon." The curator should cite that as evidence instead of "inferred from hybrid-weapon". (Same wording as the Axe of
  Lossarnach and the Easterling Halberd.)
- **Q6: mount rules are marked as the mount's.** Add an optional field `grantedTo` (`"mount"`; absent = the bearer) to
  the annotation, set on the five mounts that have grants: Pony (Timid), Fell Warg (Fell Sight), War Camel (Impaler),
  War goat (Mountain Dweller, Devastating Charge), Galadhrim Steed (Woodland Creature). Barding and every other item
  keep the default. Schema `enum: ["mount"]`; the validator checks it appears only on items whose main class is `mount`.

---

# Re-review of the current data (106 items, after rounds 1 to 4)

Reviewer: gear-classification-reviewer, bead `battle-company-manager-ce-v2-wgv`. Scope: `data.gearAnnotations` as it
stands now (106 items; new classes `lance`, `missile`, `consumable`; `grantedTo`; `definition`/`abilities`/`aliases`;
company `limitExemptions`). Nothing in `definitions/` or `tools/bc/` was edited. Validator: **0 errors, 0 warnings**.

## Summary

- **Checked: 106 of 106 rows**, plus a fresh, independent coverage walk (units named in the companies, their `wargear`
  and `options`, bundles split) against the annotations and aliases.
- **All round 1 to 4 decisions are present in the data and consistent**: Company Horn has Dominant (2); the six
  hand-and-a-half weapons are `melee, two_handed`; Bow with poisoned arrows has `ranged` and `Poisoned Attacks (Bow)`;
  Smoke Bombs 6"/S1; Blowpipe `missile` with `Poisoned Attacks (Blowpipe)`; lances `lance`, War spear `spear, lance`
  (Warspear merged); consumables; `grantedTo: mount` on the five mounts; the four `limitExemptions` match the BC text.
  Classes, main classes and ranged values show no new error. Every grant still has a sentence that gives the rule.
- **New findings: 6** (1 gap in coverage, 2 wrong rule data in `abilities`, 1 inconsistency, 2 minor).
- **Confidence:** high for classes, grants and coverage; medium for the `abilities` details (they are new and are not
  validated against the text).

## New disagreements and doubts

| # | item | curator says | you say | evidence | proposed amend | severity |
|---|---|---|---|---|---|---|
| N1 | Iron Shield, Foe Spear | **not annotated** | add: `Iron Shield` = `["shield"]`, `Foe Spear` = `["spear"]` | Company Erebor Reclaimed rolls "Vault Warden Team" (rare); vanilla unit `Vault Warden Team` has `includes` Iron Shield Bearer (wargear "Iron Shield") and Foe Spear Wielder ("Foe Spear"). Vanilla gear: Iron Shield "This is a shield that provides a bonus of +3 ... -1 penalty to their Duel Rolls, unless they are supported by a model with a Foe Spear"; Foe Spear "This is a spear" (+1 To Wound when Supporting an Iron Shield vs Monsters) | add both to `DECISIONS`; fix `collect()` (S7). The validator's coverage check cannot see it (circular) | **high** (a reachable unit shows two items with no class) |
| N2 | Pavise (`shield`) | `abilities` defenceBonus **1** (class default), text +3 | defenceBonus **3**, and "not while Engaged" | "a shield that provides the bearer +3 Defence rather than +1. ... cannot gain this bonus whilst Engaged in Combat" (the `definition` also keeps the generic "Adds 1 to Defence") | give Pavise its own ability (value 3, `notWhileEngaged`) and drop the generic shield text from its definition (same for Iron Shield, +3 and Duel -1 unless Foe Spear Support) | medium |
| N3 | Wood Elf Spear (`spear, shield`) | `abilities` include `defenceBonus 1` and `cancelledBy...` from the shield class | no Defence bonus; only `shielding` | "may use the Shielding rule even though they do not have a shield" (user decision: shield class, no grant). Light Shield and Small Shield correctly carry `shielding` only | for items listed under `shield` that are not real shields (Wood Elf Spear), keep only `shielding` (S8) | medium |
| N4 | Pike, Elven pike (`abilities duelPenaltyWith ["shield","missile"]`) | `with: missile` | `missile` collides with the new class `missile` (Blowpipe only). The rulebook's "Missile Weapon" is every ranged weapon | Pike: "-1 penalty ... if they are armed with a shield or a Missile Weapon" | use the class list `["shield","bow","crossbow","throwing","missile"]` or rename the key `rangedWeapon` | medium |
| N5 | Goat (`mount`, no grants) vs War goat (`mount`, Mountain Dweller, Devastating Charge) | Goat has no grants, though its profile is the vanilla War goat | same grants + `grantedTo: mount` | `MOUNT_PROFILE`: "the BC Goat is the vanilla War goat" | add the two grants to Goat (or merge it as an alias of War goat) | medium |
| N6 | Easterling Halberd (`spear, two_handed`) | abilities `support depth 1`, no pike penalty | as a pike: depth 2 and `duelPenaltyWith` shield/ranged | "can be used as either a pike or a two-handed weapon"; its own mode key is `pike` | take the abilities of Pike for the pike mode, or note that the mode decides | low |

Not changed, checked again: Axe of Lossarnach and Dunlending War Axe (`spear, two_handed`, user-confirmed), Berserker Blade,
Whip (limit exemption now data), Flaming vs Torching Brand, Barding `armour` (no stat change), Light Armour (`armour`,
Defence only against shooting, no `statChanges`: fine), the ten unit upgrades (class `other`, still without the flag of
old S6: acceptable, but the card must not draw a gear icon for them).

## Systematic issues (new)

- **S7: `collect()` ignores a unit's `includes`.** Bundled units (Vault Warden Team) bring in members whose wargear is
  not reached. Fix: when a reachable unit has `includes`, also collect the wargear/options of every included unit.
  Only Vault Warden Team is reachable today (other `includes` units are not in any company), but the rule is general.
  The validator's coverage check should be rewritten so it does not call `collect()` (it passes any gap in it).
- **S8: class rules are attached by class, not by what the text says.** `CLASS_RULES` adds the class's `abilities` and text
  to every item with that class. That is right for plain shields and wrong when an item merely has the class (Wood Elf
  Spear) or overrides the numbers (Pavise, Iron Shield). Fix: an item rule (`ITEM_RULES`) replaces the class rule, and
  a check that an item whose text says "+N" has a matching `defenceBonus`.
- **S9: ability vocabulary is not validated.** `abilities[].key` and the `with`/`cancelledBy*` lists are free text; N4 is
  the kind of mistake it hides. Add an enum for keys and check that class names in these lists are real classes.

## Questions for the user

- None new. N1 to N6 are corrections of data against its own text; I recommend accepting all of them (N1 first).
  If you want Foe Spear and Iron Shield to be shown as a pair, say so: the text links them (Foe Spear Supporting an Iron
  Shield), which is not a class matter.

## Next steps

1. User accepts or rejects N1 to N6. 2. Curator applies them in `DECISIONS`/`collect()`/`ITEM_RULES`, rebuilds,
runs `python3 tools/validate_definitions.py`. 3. Close the bead when resolved.
