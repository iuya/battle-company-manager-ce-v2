# Frontend: the company roster

What the My Company page shows and lets a player do with a built company: the unit cards, the Armory, equipment, healing, formation, Gold, creatures and the aftermath cards.
The design language and the page patterns are in [frontend.md](frontend.md).

## Contents

[My Company: units](#my-company-units) ·
[Company limits](#company-limits) ·
[The unit card](#the-unit-card) ·
[Item icons](#item-icons) ·
[What the cards read from the data](#what-the-cards-read-from-the-data) ·
[The Armory tab](#the-armory-tab) ·
[What is possible in the Armory](#what-is-possible-in-the-armory) ·
[Equipment slots](#equipment-slots) ·
[Armour upgrades](#armour-upgrades) ·
[Dropping equipment](#dropping-equipment) ·
[Rare units](#rare-units) ·
[Campaign settings](#campaign-settings) ·
[The cemetery](#the-cemetery) ·
[Units made of several models](#units-made-of-several-models) ·
[Formation](#formation) ·
[Rating and injuries](#rating-and-injuries) ·
[Optional rules and the expanded card](#optional-rules-and-the-expanded-card) ·
[Healing and injuries](#healing-and-injuries) ·
[Renaming units](#renaming-units) ·
[Gold](#gold) ·
[Creatures and wanderers](#creatures-and-wanderers) ·
[Company pages and the aftermath](#company-pages-and-the-aftermath)

## My Company: units

The My Company tab is the page a player uses most.
Company pages are built by `core.BuildDetail` from the stored company state and the campaign's rules files.

From top to bottom the tab shows:

1. A row of **Points**, **Units** (current / the company's maximum), **Matches** and **W / D / L**.
2. A row of **Influence**, **Gold** and the **limits**.
3. One **card per unit**, in the company's order (`core.OrderUnits`: leader, sergeants, other heroes, regular units, unless the player saved another order).

## Company limits

The limits cover bows and crossbows, throwing weapons and cavalry.
By default each is a third of the company; the numbers are the `limits[]` of the rules file, read through `BCBook.Limits`.

- `CompanyLimit.Next` is the maximum once one more model has joined.
  A model that joins counts in the numerator and the denominator, so 6 models with 2 bows may take a 7th bow model (`RoomForNew`).
- Each limit is a segmented bar like the experience one (`.xp-bar.limit-bar`, with `--seg` = `CompanyLimit.Segments`).
  There is one segment per allowed model, and the counted ones are filled from the left.
- At the right end there is one **hatched** segment (`.limit-exempt`) per model or item that the company's exemptions leave out (`CompanyLimit.Exempt`).
  The numbers then read "0 / 2 · total 1" (a Rider of Rohan in a company where they do not count), with the explanation in the tooltip.
- A limit with no `RoomForNew` says a new recruit would not fit.

## The unit card

A card, top to bottom:

- **Frame and insignia** for the unit's place, with no text tag, each in its own colour in every palette (the game colours):
  leader amber and doubled with three rank chevrons, sergeant green and doubled with two, other heroes solid blue with
  one, regular units thin. The shape of the insignia tells them apart without colour.
- **Title plate** (`.unit-head`) across the top: kind icon, the name (a click renames, see "Renaming units") over a second line
  that is always there (the type when the unit has its own name, the points against its right edge, so every plate is the icon's 48 px), and alone on the
  right the insignia, large (`.unit-rank`, 40 px; a warrior has none); tinted by the same place (`leader-soft`, `sergeant-soft`, `hero-soft`, a warrior's plain) and by `danger-soft`
  while the unit is injured (its frame turns danger too). Injuries and the live badges follow under it.
- **Kind icon** (left of the plate), a filled game-icons.net icon (CC BY 3.0, credited in `static/img/CREDITS.md`): a swordwoman for
  infantry, a cavalry charge for cavalry, a wolf head for beasts, an orc head for monsters and a dragon head for monstrous
  cavalry (`core.KindOf` from the vanilla unit types). On the plate it is a fixed 3 rem tile with the glyph almost to its edges,
  centred: a name that wraps makes the plate taller, never the icon (`.kind-icon`, 2.25 rem elsewhere, and
  `.unit-head > .kind-icon` in `tailwind.css`). The equipment and item icons are
  game-icons.net icons too (the lance is a mounted knight), except the horn of instruments (SVG Repo, CC0) and our own
  stroke icons for an unknown class or item. Barding is Lorc's armoured horse head, apart from the mount's plainer one. Some items get their own icon by name (`core.IconFor`)
  instead of their class's: wargs, camels and goats, the armoured horse (the barding head), hand-and-a-half weapons,
  hammers and mattocks, twin blades, natural weapons (claws, teeth, fangs), the glaive, throwing spears, smoke bombs, the
  troll chain, the whip, light and heavy armour, small shields and the pavise. Some share a drawing and differ by colour: the
  Galadhrim Steed is a green horse, the pony a grey one. An item without its own icon shows one per class, the later ones
  smaller: the halberd is spears and a two-handed axe. The main icon of an Elven weapon (`core.IsElvenWeapon`: Elven and
  Elven-made weapons, the Wood Elf Spear), which wins a drawn combat's roll-off more often, carries a small green leaf on its
  corner (our own `elven-leaf`, the `M` mark of `gi_icon`) and keeps its class's colour, since nearly every weapon of an elven
  company is one. Unused icon files are kept in `static/img/archive/`.
  In a gear chip of the card the icon is 1.375 rem. **Gear style**, a per-browser choice in the "Unit cards" section of `theme_controls` (the
  Appearance menu and the Account page; `$store.theme.gear` / `setGear`, buttons `[data-gear-option]`): "Icon and name" (default) or "Icons only",
  which writes the cookie `bcm-gear=tiles`; `theme-init.js` then sets `data-gear="tiles"` on `<html>` before the first paint and `tailwind.css`
  draws every gear chip with an icon on a unit card as a 2.25 rem tile with a 1.75 rem icon. The name and numbers stay in the chip for screen
  readers (visually hidden) and show in its tooltip, or in its `title` when the item has no tooltip, and on the expanded card; a struck item fades. Below
  the buttons a sample (`[data-gear-preview]`, a `.unit` list of four chips, hidden from screen readers) shows the chosen style, since it
  follows the same `<html>` attribute.
- **Name** (units are nameable), **type** and **points** (plain text, no box).
- **Keyword row** (`keyword_row` in `partials/units.gohtml`, spec [`unit-keywords.md`](unit-keywords.md)), in the bottom right corner of the card,
  below the experience line, with small chips (`.unit-kw` inside `.unit-foot`: seldom needed at the table, so out of the way; the full list, when open, is left aligned):
  chips for the unit's race, factions and the unit types no other part of the card says, at most 5, race first. The web layer (`notShownKeywords`
  in `internal/web/keywords.go`, one set for row and full list) draws **no chip** for Hero and Warrior (frame and insignia), Mount (the gear),
  or Infantry, Cavalry, Beast, War Beast and Monster (the kind icon); Unique, Elite, Swarm, Chariot and the siege types stay. core still computes
  and flags all of them. Chips are `kw-chip` list items in three shapes so the group does not depend on colour: race solid and bold (`kw-race`),
  faction muted (`kw-faction`), unit type outlined (`kw-type`); the group is also in each chip's `aria-label` ("Race keyword: Elf"). Each chip is a
  `has-tip` element (`tabindex="0"`, `data-tip-title/meta/text` from `UnitKeyword.Text`) and opens the helper popup with `data-concept` =
  `UnitKeyword.Concept` (the glossary term of the same name, else `Keywords`). A `+N` button (the chips past five) expands the row in place (Alpine
  `x-show`, no request, `aria-expanded`) to the full list in labelled groups Race, Faction, Unit type; "Fewer keywords" closes it. The expanded
  (maximised) card passes `Open = "expanded === ID"`, so there the full list is always shown and the row and the button are hidden. A lost mount
  changes nothing here (Cavalry and the mount are not chips; the kind icon does not change on dismount either). A unit with no chip draws no row. Not on
  the aftermath/level-up windows; the live compact cards (Retreated, Field casualties, Missed) have the row too, so the opponent's units are readable
  there. Fallen cards show the same row. The company browser's unit hover card (`unit_preview`)
  shows the unit's stats with the changes of its gear (the same as its roster card, `core.recruitUnit`) and its special rules with tooltips; it shows the full list in plain, always open, non-interactive chips (`Static`; that card cannot be pointed at). A bundle (the Vault Warden Team) holds the card of each member (`UnitPreview.Members`, made in `finishPreviews` with the points its company gives the member), like the glossary's article. The armoury's wanderer cards use the
  row; a creature (types only, all of them icon types) has none. Tap targets: the chips get the invisible 44 px coarse-pointer area of the corner
  tab. `/styleguide` shows the states.
- **Who a rule works against** (spec [`unit-keywords.md`](unit-keywords.md) "Rule targets"): the rule chips of the card (the unit's own, the path's) carry
  `UnitRule.Works`, filled by core from `data.ruleTargets`. The tooltip text is `UnitRule.TipText()`: the description and a second paragraph
  "Works against: enemy models with the Orc keyword." ("Works for: ..." for friendly models, Fury (X)); the same line is in `data-tip-works`, which
  `concepts.js` adds under the text of the helper popup (`.concept-works`, an accent-soft box; the popup itself is the rule's glossary term and does not
  know the parameter). The expanded card prints the line (`data-works`) under each rule. An own rule and a rule the table does not list have no line; a
  ruleset without `data.ruleTargets` has plain tooltips. **Nothing is highlighted on the unit's own card** for an enemy rule.
- **Attributes** from the vanilla file's `stats` list (`VanillaBook.Stats/Profile`): Mv FV SV S D A W C I, then M W F for
  heroes that have any, as a datasheet profile (`.statline`, `.statline-hero`, framed in `muted`): the names on a strip above the values. Values the company raised are green, lowered red; hovering shows the full name. The backend
  fills them dynamically; the page just draws what it is given.
- **Buy buttons**: two small `+` chips (`eq-add eq-add-sm`, text size) open the armoury window filtered with `?kind=wargear`
  (right after the "Wargear" label, which has its own line, items below) or `?kind=equipment` (right after the "Equipment"
  label, before the slot pips). The window lists only that half, hides what a rule forbids (`ArmouryItem.Offered`), shows armour
  upgrades (`GearOffer.ArmourStage`) as rows with an "armour upgrade" badge and gives the reason for the others. A search field ("Find an item", name and aliases, creatures too) narrows the list in the browser. Read-only cards have no chips.
- **Item descriptions are linked text**: wherever an item's description is drawn (the unit card's expanded item, the buy window, the Armory tab and its
  "Gives:" line, the free equipment the aftermath offers) it goes through `itemText` (`campaign.go`) and the `segments` template: every glossary term
  and base unit it names is a `term-link` with the usual tooltip. `Glossary.LinkExcept` never links the item's own names (its name, its `AlsoCalled`
  / `Spellings`, with plural and possessive forms: the Backpack's text keeps "Backpack" plain) and, for text of items, links the single-word names of
  weapon and wargear terms in any case (a "lance" in the War spear's text). Tooltips (chips, `data-tip-text`) stay plain text. The live match card's
  "Item used" window lists names only.
- **Effective stats**: `Attribute.Value` already includes the gear; a characteristic with `Attribute.Gear != 0` has an accent
  ring and its tooltip (`statTip`) lists the `CompanyUnit.StatMods` (`+1 from Shield`, `Shield: cancelled by a bow`). Rule
  chips and tooltips print `UnitRule.Shown()` (the label), never the key. A limit with no `RoomForNew` says a new recruit would not fit.
- **Equipment**, always sorted mount, melee, shield, armour, ranged, rest (`core.SortEquipment`); each class has its own
  icon and colour, and an item can be several things (a blade-spear shows a spear and a blade icon and counts for each). Starting items are plain, bought ones carry a `+` and an accent fill, items with stats show them
  (`24" · S2`) and items with rules have a dashed outline and a `?`, with a hover preview.
- **Path** (every hero, the leader and sergeants included): a box between the equipment and the special rules, headed by the path's icon (one game-icons.net icon per path in a round badge, `path_icon`; also in the
  path window and the fallen card) and its name, the "Path of the" lead small above the rest (`pathLook` in `paths.go`; on the unit card and the fallen card the name alone, beside a smaller badge, `path_name` with Short). Badge
  and box take the colour of the kind of hero the path makes: melee red (`path-melee`), ranged green (`path-ranged`), command
  amber (`leader`), magic blue (`path-magic`), the rest muted; a path the app does not know gets the medal. The name's tooltip is the glossary's text
  of the path with its characteristic limits and level-up chart, as in the level-up window. Folded, it lists the heroic actions unlocked so far (the unit's own, the path's and those bought later; their text comes
  from the glossary term of the same name) and the special rules the path gave, and the magical powers it taught (the Path of the Sorcerer; a spell icon with
  casting value and range); unfolded, the same heading opens a section that has them in full. Those rules are not repeated below (`CompanyUnit.Path`). Magical powers a unit knows from some other source stay among the special rules, after its own rules.
- **Special rules**, sorted unit, spells, equipment (`core.SortRules`); spells are highlighted with a star,
  their casting value and range; every rule previews its description on hover.
- **Level and experience**: the level on a gold medal (`.level-medal`) and a bar of `XPInLevel / 5` cut into one segment per
  point (`.xp-bar`, `--seg` is `LevelEvery`; filled in `muted`, `.xp-fill`, or `warn` while a level-up waits) at the bottom. A unit with promotions waiting gets a gold
  **"Level Up ready!" banner across the top of its card** (a slow shine, a Roll chip; it is the level-up button). A unit gains a
  level only when its roll is made, so it can have several rolls pending (15 XP at level 0 is "×3",
  `PendingLevelUps`). The banner opens the level-up panel (roll, re-roll, choose; `levelup.go`). In a running match the experience earned so
  far (`LiveUnit.MatchXP`, `core.MatchExperience`: taking part, the gains and the objective at the file's amounts; the MVP, Against the Odds
  and scenario rewards only come at the end) follows the fill in the magic path's colour (`--hero`, `.xp-match`), up to the end of the level,
  and the count reads "+2 0 / 5 XP" with the "+2" in the same colour (`.xp-match-text`; the bar's `aria-valuetext` says it too).

## Item icons

Every item has an icon: specific ones by name (backpack, badge, rope, drum, horn, standard, cloak, brew, herbs,
talisman, map, boots, ring, scroll, seeing stone, torch, snares, pelt, cram, belt, vial, blowpipe, axe, barding;
`core.IconFor`), the class icons for weapons, armour, shields, bows and mounts (a blade-spear shows two), and a generic
**pouch (small)** or **chest (large)** for equipment the rules data adds that has no icon yet. The icons are in
sprite `static/img/gi.svg` (one `<symbol>` per icon, drawn with `<use href>` by `gi_icon`, so a page carries a reference per icon and
the browser caches the drawings); `internal/web/icons.go` says which icon draws what (`kindIcons`, `roleLooks`, and `gearLooks` by item
class and item key, each with a tone: the `.eq-tone-*` colour of a class, so items drawn more precisely than by their class still group
by colour), and `partials/units.gohtml` draws them (`item_icon`, `eq_icon`, `gear_icons`). To give a new item its own icon, add a name
to `iconRules`, its key to `gearLooks` and the symbol to the sprite (with its credit in `static/img/CREDITS.md`); `icons_test.go` fails
on a key with no look, a name missing from the sprite and a symbol nothing draws.

## What the cards read from the data

An item's classes, granted rules and missile profile come from the Battle Companies
file's `data.gearAnnotations` (`BCBook.Annotation`), not from guessing its name. The classes are mount, melee, two_handed,
spear, lance (a cavalry spear, sorted with the melee weapons), shield, armour, bow, crossbow, throwing, missile (the
blowpipe: a ranged weapon that counts against no limit), banner, instrument, consumable (scrolls, herbs, brew, cram) and
other, each with its icon and colour; an item that is several things leads with its first class and shows the others as small
secondary markers. Granted rules are the item's rules, or the mount's when the annotation says `grantedTo: mount` (Pony:
Timid); Barding is the steed's armour, sorted right after the mount, and its rules go to the rider. Envenom Weapon is not an
item but a modifier: the unit gains Poisoned Attacks (<the weapon the player chose>) and the weapon stays plain. Envenom Weapon asks for the weapon at purchase, like every special rule of the "X" kind (the level-up's minor rule too), with the one `weapon_chooser` select: in the buy window and in each unit row of the Armory tab (`armoryView.Weapons`, the unit's `PoisonableWeapons`; both forms post `weapon`, `handleBuyEquipment` and `handleArmoryBuy` share `buyEquipment`, and a refusal names the choices), and the item's `Choices` on the expanded card, only for a legacy `Envenom Weapon (X)` that has no weapon yet ("Choose weapon"; `handleChooseModifierWeapon`). The choice is final: a chosen item shows "<weapon> · envenomed" with no chooser and no "Change weapon", the rule stays when the weapon is dropped (the unit card keeps showing it once) and core refuses a second choice or a purchase without a weapon with a message naming the choices. A unit with no weapon left that Poisoned Attacks could go to gets the reason (`no_weapon_reason`) instead of the chooser and the button; the level-up's "A minor special rule" lists the same weapons (`HeroSheet.Weapons` is the unit's `PoisonableWeapons`). Named unit upgrades
(Helmingas, Hearthguard...) are badges, not gear. Limits are counted by class (bow and crossbow, throwing, the Cavalry
keyword) with a company's `limitExemptions` (an exempt unit is left out of the count and of the base the maximum is worked
out on) and `limitOverrides` (`BCBook.Limits`).

## The Armory tab

The Armory tab lists every purchase the company could ever make, whether or not a unit can take it now, with its cost, slot
size, text and what it gives; it leaves out only what no unit could ever buy. **Buy for…** opens the company's units: those that can
take it have a Buy button, the others are shadowed with the reason (heroes only, cavalry only, no free slot, a limit full, not enough
influence...). An item that is possible but that no unit can take now is **greyed**: its name is muted, the button reads "No unit can
take it now" and is `aria-disabled="true"` (still focusable, it still opens the per-unit reasons: not a dead end) and `aria-describedby` the
sentence under the item's text (`ArmouryEntry.Why`: what stops each kind of unit, then who could take it later, e.g. "No unit can take it
now: already wears this armour or better (Warrior of Arnor); not an option of its profile (Ranger of Arnor). A Ranger of the North may take
it after Armour."). A search box filters by name and alias.

## What is possible in the Armory

What is possible is decided in core in one place (`shop.possible` through `shop.entries`, `core/armoury_possible.go`).
An item is possible in a company when some unit that can exist in it could buy it under the rules, whatever the units carry or the influence and slots now.

**Which units can exist** (`shop.candidates`):

- The units the company has.
- From the same sources as a hero's options (`heroOptions`: reinforcement and special charts, advancements, promotions, starting company, walked once by `shop.reachable`): a warrior of every profile it recruits and a hero of every profile a hero can be. A profile a promotion replaces is no hero: the Ranger of Arnor becomes a Ranger of the North.
- Each is taken with nothing carried and an empty roster, so "already carried", a unique item's count and the roster of the day never make an item impossible.

**What is possible, by kind.**

- **Wargear and equipment:** a candidate no rule rules out. `whyNot` is the one source: hero or warrior, who may take it, the Beast rule, a warrior's profile options and a hero's company-wide ones.
- **An armour upgrade:** a candidate wearing less than the stage whose profile (a warrior) or company (a hero, `armourPool`) has every stage from the one above its own up to it, as armour goes one stage at a time.
- **Creatures:** possible unless the rules give no officer one.
- **Wanderers:** possible unless the company could not hire it even with an empty roster (the models it may have, a keyword's share of them; the balance between two races is met by hiring the other).

**Examples.**

- In a fresh Arnor company Armour and Heavy Armour are possible (a Ranger of the North may buy them) while no unit can take them now, and no Warg or halberd is ever possible.
- In the Shire Armour is possible and Heavy Armour is not.
- In Rohan and Moria every profile starts in Armour or better, so Armour is not possible and Heavy Armour is.

### Armory filters

The filters are the Alpine component `armory`, so they work in the browser.
A switch **Hide what <company> could never take** (ON by default, remembered in `localStorage`
`bcm-armory-mine`, styled like the main page's "Show finished campaigns" switch) hides the items, creatures and wanderers that are not
possible (`data-avail="0"`, from `ArmouryEntry.Possible`; another company's goat or Fell Warg) and says how many; with the switch off they
show, and every purchase that is possible shows either way: lacking influence, a slot, a unit that can take it now, or a stage to climb does
not hide anything. Main category chips (All, Wargear, Equipment, Creatures, Wanderers) and, under
them, the item category chips (Melee, Ranged, Armour and shields, Mounts, Consumables, Banners and horns, Other: the item's first class,
`subOf`); picking a main category disables the item categories it has no items in (`data-applies`), so Creatures and Wanderers have none.
The names on creature and wanderer cards (special rules, wargear, heroic actions) carry a tooltip when a file explains them: the armory's
own item text (aliases too) or the glossary term; a name nothing explains has none.

## Equipment slots

Battle Companies equipment takes a slot: a model carries one large and one small item, and a Backpack (large) holds three more small ones (`CompanyUnit.Slots`, `GearOffer.Size`).
Wargear (weapons, armour, mounts) takes none.

A card shows its slots as pips (`Large ■ 1/1`, `Small □ 0/1`), and a **+** after its equipment opens a window with what the company's armoury sells (`Armoury`: the common wargear and equipment plus the company's unique wargear), each item with its influence cost and slot size.

**Only what the unit could take is listed.** An item a rule forbids it is left out:

- already carried, heroes only, cavalry only, evil only;
- for wargear, an option its profile does not have: a warrior may only buy wargear matching one of its profile's options, a hero any wargear of the profiles in its company's charts, promotions and starting company (`heroOptions`);
- a Beast takes no wargear and only equipment that grants exactly one special rule (Trophy Pelt, Badge of Courage), `beastMayCarry`;
- a warrior promoted to any profile loses the wargear that profile does not list, by item identity, see [aftermath.md](aftermath.md#wargear-on-a-promotion) (the Mirkwood Elf Warrior's Elven-made glaive on becoming a Mirkwood Elf Knight: gone for good, `dropUnlisted`; a hero Knight buys it back);
- a profile without options, such as the Guard of the Fountain Court, buys no wargear, and a Mattock is not the Two-handed weapon option.

An item that only lacks influence, a free slot or room under a company limit (bows, throwing weapons) is listed with that reason (`ArmouryItem.Blocker`): a full limit is not a rule against the unit.

## Armour upgrades

Armour upgrades (`core/armour.go`, `data.wargear.armourUpgrade`): the shop adds one offer per armour stage above the first
(`GearOffer.ArmourStage` > 0: "Armour", "Heavy Armour"; 1 influence or Gold, rating per stage: 1 until a hero's Attacks + Wounds reach 3, then 5, and no extra rating for a stage that its profile or its Options already have, BC "Calculate Rating"; Gold price: the armoury's first value, the same for every unit). Buying
one through `BuyEquipment` with the stage's name replaces the armour the unit wears (the old one is dropped) with the next stage;
stages are never skipped. A warrior (a Beast that is still a warrior included) needs the stage as an option of its profile; a hero only the stages that units
of its company can have (`armourPool`), so a company without heavy-armoured units never sells Heavy Armour to a hero.
`ArmourUpgrade.UnitStage` is the stage a unit wears. Frontend: the offers appear in `Armoury` and `ArmouryEntries` like any item
(their `Reason`/`Blocker` say why not) and, like any item, in the Armory tab whenever they are possible in the company even if every unit
wears the stage or cannot reach it now (see above); the Defence change shows in the effective stats.

## Dropping equipment

The expanded card has a **Drop** button on equipment the rules let the unit drop (`core.DropReason`);
otherwise it says why. Battle Companies: equipment (slot items) can always be dropped; a hero freely drops any wargear it
chose or bought, but wargear of its profile only with the optional rule "heroes may discard any wargear"
(the campaign setting `HeroesDropProfileGear`, off by default); a warrior may drop wargear only if what it then
carries is another option of its profile (a shield alone, yes; a spear alone, no) and never its profile's wargear.
Nothing is refunded, but the item's rating leaves the unit's points (a unit's points are its vanilla points plus the rating
of what it chose or bought: warrior or hero column), and the slot and company limit it used are freed, so another unit can
buy the limited item. Buying spends the company's
influence and may grant special rules (`grantsSpecialRules` in the Battle Companies file). Every item the company can buy is in the Armory tab.

## Rare units

A chart row or special unit marked rare (`Rare N` badge in the recruit tables) may be had N times in the company at
once (the units of that type in the roster). A rare result the company already has the most of cannot join: the player takes a
lower result of the same chart (or cannot buy the special unit), like any other result that cannot join. A result that offers
several units (`unitChoice`, the Wanderers' Hobbits: Hobbit Militia, Hobbit Shirriff, Tookish Hunter, Battlin' Brandybuck) lets the
player pick one the company does not have yet (each is rare 1 as a profile); with all four it cannot join.

## Campaign settings

The options an organiser can switch on when creating a campaign (`heroesDropProfileGear`, `shakeTheTable`,
`manualRolls`) are listed in the Battle Companies file (`campaign.options[]`: key, scope, label, description, default,
implemented); the form is drawn from that list (`core.ParseCampaignOptions`, grouped by scope: rules, app). An option whose key the
app does not know, or that the file marks as not implemented (the homebrew path), is listed struck through and disabled with an "unknown setting" tag (its tooltip says the app may need updating or the rule in the file may be wrong), and logged as a warning.

## The cemetery

A unit that dies (a casualty roll of Dead, a hero retiring on its second arm or leg wound) leaves the roster for
good and rests in the **Cemetery** section under the Battle company, as it was when it fell (`core.FallenUnit`: its profile, wargear,
path and level, how it ended and where). The cards only look. The player can reorder them (**Change order**, with drag and drop or
arrows, saved with `SaveCemeteryOrder`), but a fallen unit never goes back to the roster and a unit of the company cannot be put
in the cemetery by hand (the two lists are separate Sortable instances, and each save refuses ids of the other list). The cemetery is filled from the casualty rolls of the aftermath ([aftermath.md](aftermath.md)).

## Units made of several models

The Vault Warden Team joins as two units of their own (vanilla `includes`, points from the
company's `includedUnitPoints`: Iron Shield Bearer 13, Foe Spear Wielder 12), each with its own card, experience and injuries. A
`compositeModel` (the Warg Marauder: three Goblin riders and a Warg on one Cavalry card) is one unit that counts as `Slots` models
against the company's maximum (`CompanyUnit.Models`; `CompanyDetail.ModelCount`) and shows its parts on the card; it counts once for
the Bow and Cavalry limits. The company's `unitTypes` change what a unit is (Infantry to Cavalry). Units **On Their Own Path** (the
company's `onTheirOwnPath`, matched by exact name) start on the path "On Their Own Path": no experience, no level-ups, the rule At Path's End; the path is not selectable (`selectable: false`), so a hero's path choice never offers it. With the campaign option **customOnTheirOwnPath** (off by default, `CampaignSettings.CustomOnTheirOwnPath`) these units
are instead ordinary warriors that earn experience (`CompanyUnit.OwnPath` is false) and roll on the warrior level-up table; on a 6 they become heroes on the homebrew "Path of Their Own Making" at once (0 Might, 0 Will, 1 Fate, a 2D6 chart in the file): `RollLevelUp` returns `Done` with that summary (`Result` heroInTheMaking, no `Offers`), there is no path window and no promotion to forgo, so the roll panel treats it like a final result: the celebration of a Hero in the Making with the summary, and Done. The campaign overview describes the setting that way; until the 6 the unit's card is a plain warrior's (XP bar, Level Up ready button). The path is hidden from the player's path choices like the empty one (`ProgressionRules.Hidden`). Officers made at company creation, or taking a lost officer's place, already carry the company's hero promotion (an Arnor officer is a Ranger of the North on the roster); only the path is chosen later. Unit names in the file are exact vanilla names (or a
company's label for a unit, like the Goblin Hulk), so they are matched exactly. A `heroPromotion` (Arnor) makes the unit change profile
and become a hero in one step, with no extra Might, Will and Fate.

## Formation

The order of the units comes from the backend: the default (leader, sergeants, other heroes, regular units; within each group the most points first, then by name) until
the player orders their company, and their saved order after that (`core.OrderUnits`; units they have not placed,
such as new recruits, follow at the end in the default order). A match uses the same order inside each of its zones, and the aftermath too (its lists after the sheet put the casualties first). **Change formation**, next to the unit count, puts a handle above
every card: drag it (SortableJS, `static/js/sortable.min.js`, MIT, with its pointer-based drag so it feels the same with a
mouse and on touch screens) or use the arrows on the handle (keyboard friendly). **Save formation** sends the ids in their
new order (`SaveFormation`, which refuses an order that is empty, repeats a unit, misses one or names a unit of another company) and the page comes back in that order; **Cancel** (or Escape, when nothing else is open) puts the
cards back. Meanwhile the cards themselves do not react. A company with a saved order also has **Restore default order** there (`ResetFormation`), which forgets it.

## Rating and injuries

A unit's points are its Battle Companies rating: a warrior is its MESBG points plus the option its
wargear amounts to plus the rating of its equipment; a hero built from a warrior profile is that profile's points plus
5 per point of Might, Will, Fate, Movement, Fight, Strength and Defence and 10 per point of Attack and Wounds above the
profile and above the average Battle Company profile, plus 5 per special rule its path gave and 5 per minor special rule it took at level-up, those at most 10 together (`BCBook.HeroRating`; Envenom Weapon's rule is the item's and does not count), plus
the options of its profile it carries at their MESBG points (BC: wargear of the profile or its Options "makes no adjustment, it
is as it is in their book", the same price a warrior pays), plus the rating of gear that is neither part of its profile nor of
its options (at the hero rating when its Attacks and Wounds add up to 3 or more); other heroes (named characters, monsters)
are their vanilla points plus their options plus gear. A starting company's points plus its gold are 105 once the officers are chosen. **Injured** units
(marked on their card) miss the next game and are left out of the company's rating (the Points card says how many).

## Optional rules and the expanded card

Optional rules are chosen when the campaign is created and shown in its Rules card (see "Campaign settings" above and [architecture.md](architecture.md#campaign-settings)).

The **expand** button (a small tab flush in the card's corner frame, so it takes no room from the content) maximises one card (only one at a time; click outside, press Esc or the close button to return): it
shows every item and rule in full, with the glossary terms inside the descriptions linked and previewed on hover
(`core.Glossary.Link`, the campaign's glossary). Static behaviour is in `static/js/company.js`.

Company pages are built by `core.BuildDetail` from the stored company state and the campaign's rules files.

## Healing and injuries

Costs, the roll and what each injury does come from the Battle Companies file (`campaign.injuries`, read by
`core.ParseInjuries`; `BCBook.Injuries()`), so a modified file changes them.

- An injured **warrior** card has "Heal · N influence" (no roll): the Injured condition goes and the unit counts for the
  rating again.
- A **hero** carries counts of its persistent injuries (arm wound, leg wound, broken honour; `CompanyUnit.Injuries`),
  shown as badges. They are applied by `InjuryRules.StatBonus` / `ApplyToUnit`: Movement −1 per leg wound (a red boost on
  the stat), gear the injury disables is struck through (`Equipment.Disabled`), abilities lost are listed under the
  badges (`LostAbilities`), and the second broken honour grants Fearful.
- "Heal" opens a window (`treat_panel`) with two ways: an **attempt** (cost, D6 against the target; if the roll misses and extra influence could still make it, the player is
  asked after seeing the roll whether to spend it, up to the ceiling; the influence is lost when it fails and the player may retry; it can also cancel the missed
  game of T'is Just a Scratch) and the **healers** (no roll, the hero misses the next game; persistent injuries only).
  The roll uses the dice (below): "Roll for me" is animated to the server's value, "I rolled a N" is the player's word.

A unit that misses the next match ("Out next match", the Injured condition) is shown at the end of the list
(`.unit-away`, a CSS `order`, so the saved formation is not touched and it returns by itself when it recovers; the
formation editor shows the real order). The Heal button is a round red cross, like the "+" that buys equipment.
The card lists wargear (no slot) first, then the slots line, then the equipment that takes the slots with the "+"
button; an item with a tooltip has a dashed border.

## Renaming units

A unit is renamed by clicking its name (`.unit-name`, a button that reads as the title; a dotted underline on hover and focus says it
can be changed). There is no separate pencil. The field then takes the plate beside the kind icon, with Cancel and Save under it: Enter
saves, Escape gives up, and a click elsewhere saves a change (or just closes when nothing changed). It always opens on the saved name.
The field's placeholder is the unit's type: the name is at most 40 characters and an empty one takes the unit back to its type. Rules
that aim at a unit use its id, so they follow the new name (`campaigns.RenameUnit`). The form is shown with a `:class`, not
`x-show`: Alpine shows an element with a `@click.outside` one frame late, which would leave nothing to focus. The browser
flow `82-company-ui` covers it.

## Gold

Gold is given to some companies in their starting company, as a special victory reward of some scenarios, and by the organiser
in a narrative scenario (a custom one); what is left is kept (`campaign.gold`). Equipment, creatures and wanderers cost the first value of
their rating (`gold` in the file: the warrior value) in Gold and no influence, and wargear can be bought with Gold as well.
One price function, `shop.goldPrice`, serves the armory lists, the buy and the refusal reasons: every unit, warrior or hero,
pays the armoury's first value; the second value only changes a hero's rating. The exception is wargear that is on its own one
of the unit's own profile's options, which costs that option's points, lower or higher (a Dwarf Ranger pays 1 for a Dwarf
Longbow, a Dwarf Warrior or a hero of another profile pays the armoury's 2; Arnor's starting leader pays 1 for a Torching
Brand). The armory and the company creation page show one Gold number. Slots, limits and
restrictions still apply. Every buy button of the armory (the tab, a unit's equipment window, creatures, wanderers) offers the
payments the company can afford: "Buy · N influence" and/or "Buy · G gold" (`pay` field). The Gold, Influence and Re-rolls
cards of My Company have tooltips saying what they are for and where they come from. The organiser can grant Gold from the grants card of a company page, and scenario results award it in the aftermath ([matches.md](matches.md)).

## Creatures and wanderers

The Armory tab lists **creatures** (for the leader and the sergeants, one each, at most 3 per company; `data.creatures`) and
**wanderers** (mercenary heroes, one at a time; `data.wanderers`) below the gear. A creature is not a unit: it takes no
roster slot, equipment slot or wargear limit and rolls no injuries, but it counts for its points in the rating. The owner's
card shows a creature chip (tooltip with its stat block) and a small card appears under the units. The **Baggage Pony**
gives the company a pack (`companyFeature.carryGear`): "Open the pack" lets any member store, take out or swap gear for
free (`campaigns.Carry`; what a member gets must be something it could buy, mounts and upgrades cannot be carried, gear in the pack still counts towards the bow and throwing limits, and the
app does not count the "once per occasion"). A wanderer is a normal hero card with a Wanderer badge: it counts towards
the unit limit, has no experience, cannot buy equipment, rolls the hero injuries and can be dismissed (no refund). A creature nobody can take now and a wanderer
that cannot be hired now (the company is full, not enough influence) stay listed, greyed like an item, with the reason (`aria-disabled`
button described by it); only the ones that are not possible in the company (the rules give no officer a creature; the company could not
hire the wanderer even with an empty roster) are hidden by the Armory's switch.

Item tooltips read the Battle Companies file's `gearAnnotations[].definition`; a mount's tooltip also has its stat block
(`gearAnnotations[].profile`, the vanilla mount entry).

## Company pages and the aftermath

Roster, promotions (level-up, advancements) and injuries (Heal) are the cards of My Company on this page; level-up is in [frontend-level-up.md](frontend-level-up.md). Around them ([aftermath.md](aftermath.md)):

- **Aftermath banner** (`#aftermath-banner`) at the top of My Company when `CompanyDetail.Aftermaths` is not empty: a button per
  match: Start sends `POST /campaigns/{id}/matches/{match}/aftermath/{company}/begin` ([aftermath.md](aftermath.md#the-aftermath-page)), Continue opens the page. The company is locked: the controls marked
  `data-lock` (Recruit, Armory buy, Drop, Heal, Dismiss) look disabled, are exposed as `aria-disabled="true"` (set by `syncLock` in `company.js` on the buttons and fields inside them, described by the banner, and re-set after any htmx swap inside the page by a `MutationObserver`; this also exempts the dimmed look from the contrast rule) and, on click or Enter or Space, `lockClick` in `company.js` says "Finish the
  aftermath of Match N first." instead of acting (the root's `data-lock` holds the reason, empty when unlocked). The core refuses
  the action too, so a stale page shows the refusal as a notice.
- **Perks card** (`perks_card`): the company's unused scenario perks (name, the relic bearer, text, when it applies) with
  **Mark used** for the owner (`POST /campaigns/{id}/my-company/{company}/perks/{perk}/use`, `UsePerk`); used ones sit under a collapsed
  "Used (n)". Empty state when there are none.
- **History card** (`history_card`): the latest 20 aftermath lines (`CompanyDetail.History`), newest first, on both company
  pages; the grants have their own card. Every `core.AftermathLine` carries `Match` (number), `MatchID` and `On` (when the aftermath was finished, else started, else when the match ended; zero when unknown), on `CompanyDetail.History` and on `Aftermath.Lines` of the match page. The card shows "Match N" as a link to `/campaigns/{id}/matches/{MatchID}` (not on the match page itself) and the date through the viewer's time zone formatter (`fmtDateShort`); the card takes `(dict "Lines" … "CID" campaign id)`.
- The company page of somebody else (`/campaigns/{id}/companies/{company}`, from the Rankings) is the **My Company page, read
  only** (`myCompanyView.ReadOnly`, same template `campaign_my_company`): a "Viewing X, the company of Y. Read only" banner with a
  link back to all companies (the Rankings), the same stats, limits, full unit cards (expandable, tooltips) and cemetery, creatures, perks
  (no "use" button) and history; no rename, recruit, formation, buy, drop, heal, level-up, path, pack or dismiss controls and
  no buy window. The organiser's award form of the grants card stays. Your own company's URL redirects to `/my-company/{company}`.


## Casualty cards of the aftermath

On the Injuries step each casualty is a unit card in the same grid as the rest of the company (`unit-choices`: one column, two from 1024 px, where a card has the width for its rows). A strip replaces the status badge: before the roll, two empty dice, the Casualty badge, the Roll button and, with `ManualRolls` on, a pencil that opens the typed dice; after it, the dice as they fell (a modifier beside them, or the item that replaced the roll), the result and the Re-roll button. The strip is coloured by the result: danger for a unit lost, warning for an injury or a Scratch, success for a recovery. The result's rule is its tooltip (a tap opens the helper popup), and the injuries chart, with the row of the roll marked, is the tooltip of the dice (hover, keyboard focus or a tap: `aftermathTips`). A card that waits for the viewer, to roll or to answer a question of its result, has a highlighted border (`unit-pending`); a Re-roll that is only on offer does not count. Where the cards sit side by side, a unit with no place or path keeps an empty row of the same height, so the strips of a hero and a warrior line up while the hero's row fits on one line. The milestones sit at the bottom of the card, right above the experience bar, whatever the height of the card beside it.

See also: [frontend-company-building.md](frontend-company-building.md) (recruiting and creating a company), [frontend-level-up.md](frontend-level-up.md) (paths and level-up), [frontend-live-match.md](frontend-live-match.md) (the same page in an open match), [aftermath.md](aftermath.md), [frontend.md](frontend.md).
