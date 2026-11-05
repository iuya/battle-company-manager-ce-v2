# Frontend: paths and level-up

The window that picks a hero's path (a mosaic and a carousel, with the Sorcerer's magical power window) and the level-up window that applies the advancement rolls.
The design language and the page patterns are in [frontend.md](frontend.md).

## Path carousel

A leader or sergeant without a path chooses it in the path window.
The window is a carousel with a mosaic of every path above it.

Code: `path_panel`, `handleChoosePath` and `pathCards` (`company_actions.go`); flow `109-path-carousel`.

### The mosaic

On top, every path is a small button (a small icon and the name on one line).

- The buttons form a wrapping grid (`.path-strip`, columns of at least 7.75 rem), so there are 2 columns at 360 px and more as the window widens.
- Every path is visible at once, with no scrolling.
- The grid is a `role="radiogroup"` of radios named `path`, so Tab, the arrow keys, Enter and screen readers work natively.
  A button's accessible name is the full path name.
- The selected button is marked (`.is-on`).
  Exactly one is always selected: the first available path at the start.

### The cards

Below the mosaic, the paths are cards stacked in one grid cell.
The Alpine component `pathCarousel` (`static/js/company.js`) gives each card two values:

- `data-rel`, its place against the selection, clamped to -2..2 for the look at rest;
- `--r`, the same place unclamped, from which the transform is computed.

At rest:

- The selected card is in front and whole.
- The previous and next cards sit behind it: smaller (scale .86), half transparent, blurred and partly covered.
- The other cards are away and invisible.
- The cards behind are `aria-hidden` with an `inert` body, so they cannot be focused or read.
  A click on one only selects it.

### Moving between paths

A path is chosen with a tab, the floating arrows, the arrow keys, a swipe on the stack, or a click on a card behind.
The cards then move with a transform and opacity transition.

- The length of the transition (`--dur`) grows with the distance: 0.2 s plus 0.08 s per step, at most 0.6 s.
- A jump of five sweeps the cards past each other.
  The cards in between are seen going by (blurred and faint, `.is-moving`) and are gone again at rest.
- The card in front has an 85% background, not opacity, so its text keeps its contrast.
  The cards behind are blurred 2 px, also under reduced motion.
- Under `prefers-reduced-motion` the cards just switch (`--dur` is 0).

Two **floating previous/next buttons** (`.path-float`) sit in a sticky bar at the middle of the window over the stack.
They are 44 px and overlap only the card's margin, so they stay in reach when the card is scrolled.
They share `step()` and the disabled state with the buttons at the sides of the mosaic.

### What a card shows

A card shows everything the app knows of the path:

- its description;
- **Path Limits**, the most each characteristic can reach, from the glossary term's limits.
  The profile's limits (F, S, D, A, W, C...) are in a `statline`.
  Might, Will and Fate are stacked under it as a second row, as on a hero's card (`statline-hero`; `.path-limits` only stacks them);
- the **starting heroic action** and, for a Sorcerer, **Starting power** (neither is a row of the chart);
- the whole **level-up chart** ("Level-up: roll 2D6") as a list of bordered rows (`border-line`), one per roll, with the usual glossary links.
  The chart already names every Mastery and special rule the path unlocks, so they are not listed again.

A row that offers alternatives shows each on its own line with a small "or" divider.
`splitAlternatives` cuts the chart's text at the file's upper-case " OR ", at ", or improve " and before "Pick an additional" in a "Choice of" row.
An ordinary "or" inside a sentence is left alone.

A blocked path shows the reason, and "Take this path" is off.

### Select magical power (Sorcerer)

A Sorcerer's first magical power is a second step under the stack.

1. A **"Select magical power"** button opens a second window inside the path window.
   Once a power is chosen, the button reads "Change" and the chosen name is shown.
2. The window is a `role="dialog"`.
   `dialog.js` keeps a stack, so focus is trapped in the inner window, Escape closes only it, and focus goes back to the button.
3. The window lists every power the Sorcerer can begin with as real radio inputs named `power` in a `radiogroup`.
   Each shows its **whole description** (`powerViews`: the rules file's definition, never cut, with glossary links), so the player knows what each power does.
4. A click on a power chooses it and closes the list.
   The arrow keys only move through the list; Enter or "Use this power" closes it.
5. "Take this path" stays off for a Sorcerer until a power is chosen.
   The radios are disabled (and not submitted) for any other path.

The form posts `POST .../units/{unit}/path` with `path` (and `power`). The choice is final.

Styles: `.path-carousel`, `.path-strip`, `.path-tab`, `.path-stage` and `.path-card` in `tailwind.css`.

## Level-up

The "Level Up ready!" banner of a card opens the level-up window (`level_panel`). The rules are read from the Battle
Companies file (`core.ParseProgression`: `campaign.progression`, `data.paths`, `campaign.characteristicLimits`, each
company's `advancements`); a unit earns a roll every 5 XP and several can wait.

- A **warrior** rolls its dice (`campaign.progression.warrior.roll`, d6 in the shipped file):
  - 1-2: nothing. A failure in danger colours with the rolled number. With no roll left the window closes by itself 2 seconds after the reveal (`levelPanel.CloseAfter`, counted when the dice stop in `rollPanel`) and refreshes the page it was opened from; with rolls left it stays open with "Next level up".
  - 3-5: a promotion. The player presses one of the company's advancements that match the unit; with none, Courage and Intelligence +1 up to 3+, with its text and Done.
  - 6: a **Hero in the Making**, see below.
- A **hero** rolls `progression.hero.rollsPerProgression` results (two) of its path's dice (`paths[].dice`, else `progression.hero.roll`, 2D6) on its path chart, and keeps one (`choose`, 1).
  - A result it cannot take is re-rolled by the app, with a note: a stat at the path's or the overall maximum, a rule or action already owned, or a minor-rule or heroic-action total reached.
  - The player presses the option it wants, see [Options](#options).
  - When rolls remain, "Next level up" (the card's banner effect) opens the next.
- The window goes back to the page it was opened from when it is done (`rollPanel.finish`): My Company, or the aftermath, whose window reloads its own page when it closes (`data-reloads-on-close`). A path chosen from the aftermath also goes back to it (`handleChoosePath`).
- "Roll for me" is animated (a warrior's dice; a hero's dice run in one lane per result, named and totalled once they stop); "I rolled" takes the player's own dice. A roll
  waits for its choice if the window is closed (`campaigns.LevelUpState`).
- **Magical powers** (Path of the Sorcerer only): the rows offer "a new power or improve a casting value".
  - The list is the Sorcerer's spell list (`data.magicalPowers.items[learnable]`, each with range and casting value) minus the powers the hero knows.
  - An improvement lowers a power's casting value by 1 (5+ becomes 4+), at most twice per power and never below 2+ (`castingRule`).
  - A new Sorcerer picks its first power in the path window (a unit whose hidden path starts with one, in the level-up window: `Stage` startingPower).
  - Fury is learned for a species (one choice per "Fury (Species)").
  - A path with a company limit (the Sorcerer: 1) is not offered once the company has that many heroes on it.
  - The backend must provide the eligible list and the improvements; the app only renders the choice (a collapsible list with each power's casting and range and its whole definition, never cut, with glossary links; also on hover).
- **Minor rules with a target.**
  - Poisoned Attacks asks which weapon (one choice per weapon that is not poisoned yet, by a rule, a level-up or Envenom Weapon).
  - Combat Synergy asks which other hero of the company (the Battle Companies wording: "this Hero and X").
  - The choice is stored with the target (`Progress.Minor` entries such as `Poisoned Attacks|weapon:Bow` and `Combat Synergy|unit:7`).
  - The card shows the rule as "Poisoned Attacks (Bow)" with the weapon marked "· poisoned", and "Combat Synergy - Hador". A unit target is kept by id, so renaming the unit renames the rule.
  - The parameter kinds come from `minorSpecialRules[].parameterKind` and the wording from `parameterisedRules` in the rules file.
- A hero that takes a path gets its starting heroic action at once (`paths[].startingHeroicAction`), whenever it chooses the path.
- If only one of the two results can be taken, the player must take it; the unavailable ones are re-rolled only when neither
  can be taken.
- **Re-rolls** (Against the Odds, `CompanyState.Rerolls`, shown on My Company): while a roll waits for its choice, each result
  has a "Re-roll · Against the Odds" button that rolls that result again (a warrior's D6, or a hero's pair).
- A new hero's remaining waiting rolls roll on its path at once (the rules say "from the next game on").

### Options

The options are buttons that apply at once (`level_option`, `level_choices`); there are no radios and no Choose button, except "Apply both" below. This holds for hero results, warrior promotions, the company hero upgrade and a first power chosen in this window.

- A button shows the option's short name (`choiceView.Name`); the full text is its tooltip (`data-tip-*`).
- Beside it, an **(i)** button (`.level-info`) opens that text in the helper popup without choosing, for touch screens, where a tap applies at once and cannot be undone. A stat is not a glossary term: the popup falls back to the option's own text.
- An option that gives a special rule, a minor rule, a heroic action or a magical power has a badge with the kind (`kw-chip`).
- A **stat option** shows its amount (Fight +1) and progress dots against its limit: taken, the gain in gold, free (`core.StatGain`, `statDots`). The steps count up for stats where lower is better, the limit is what `improvedStat` allows (the path's limit, a path's maximum and the overall peak), and the tooltip has the exact values ("Fight 4 → 5, path limit +3, peak 9").
- Kept: the re-roll buttons, the shadowed results that give nothing, the collapsible groups of long lists.

### Options that also take the other result

A `RollAgain` option also takes the other result: Courage +1 or Intelligence +1 on a 5, and on the Warlord's 7 beside a normal option. `core.LevelPlan` (`levelling_plan.go`, worked out on every read, never stored) says what goes together, trying each pair on a copy of the company; the Alpine component `levelChoices` (`static/js/company.js`) keeps the marks.

| Case | What the window does |
|---|---|
| One result takes the other, and the other has something to take | One option is marked: Courage, or Intelligence when Courage cannot go up. Pressing it does nothing and it cannot be unmarked, pressing the other stat moves the mark and applies nothing, pressing an option of the other result applies both in one request. |
| The other result has nothing to take (or nothing after any of the options) | Nothing is marked: plain buttons that apply at once. A single option with nothing left after it is a plain button too (`LevelPlan.Alone`). |
| A normal option in the same result (the Warlord's 7) | The stat is still marked, and pressing an option of the other result applies both. Pressing the normal option applies only it and the other result is lost: only the options that take it say "+ other roll". |
| Both results take the other (5 and 5, 5 and 7) | One mark in each, Courage in the first and Intelligence in the second by default, each movable within its result. **Apply both** applies them. The normal option of a 7 can take that result's mark: the player gets it plus the 5's stat, since the 5 is what grants the other result. |

An option that does not fit with the mark is off, with the reason on the button and in its tooltip (`LevelPlan.Conflicts`: the same stat past its limit, two normal options). The company hero upgrade replaces either result: pressing it applies only the upgrade, whatever is marked. An Against the Odds re-roll comes back with a new plan, so the marks are worked out again. The request carries the option that takes the other result first (`choice`, then `also`).

### Hero in the Making

A warrior's 6 is one screen: a gold celebration (`hero_fanfare`) with "Hero in the Making!" and the unit's name, announced with `role="status"`. Confetti (`.confetti`, CSS and theme colours only, none under `prefers-reduced-motion`) plays only when the roll was just made: reopening the window shows the screen without it.

- **Accept and close** makes it a hero with no path and closes the window; **Choose path now** does the same and opens the path window. Choosing a path is not part of the roll: the card's "Choose a path" button does it later, and a company with a pathless hero cannot start a match (`matches.go`). A pathless hero with more rolls waiting is sent to the path window first.
- Closing with X or Escape decides nothing: the roll stays pending.
- Forgoing it for a promotion is possible, as buttons on this screen, only until the hero is accepted.
- A unit on its own path (`roll.Auto`) has no path to choose and nothing to forgo: the same celebration and Done.

In core, becoming a hero and taking a path are two steps (`levelEnv.becomeHero`, the choice `hero`; `takePath`, through `ChoosePath`). Becoming a hero gives the usual Might, Will and Fate and any change of profile (`PromotionOnHero`); the path gives its heroic action and a Sorcerer's first power, and a homebrew path with its own start replaces the usual points (`Progress.StartBonus`), never both. `LimitPerCompany` is checked when the path is chosen, and the path window says why a path is blocked. A roll stored by an older version with its paths in the offer is brought up to date when read (`modernHeroOffer`).

Companies' points are shown as `current (full)`: what counts now, with injured units left out, and in brackets what they
would be with nobody injured (left out when equal; `CampaignCompany.FullPoints`, template `points`).


See also: [frontend-company.md](frontend-company.md) (the unit cards), [aftermath.md](aftermath.md) (where experience is earned), [definitions.md](definitions.md) (paths and progression data), [frontend.md](frontend.md).
