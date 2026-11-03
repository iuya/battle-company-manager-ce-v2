# Frontend: recruiting and creating a company

How a company is created (choosing a Battle Company and its starting company), how units are recruited, and the company rules that change them.
The design language and the page patterns are in [frontend.md](frontend.md).

## Contents

[Recruiting](#recruiting) ·
[Officer locks on the creation page](#officer-locks-on-the-creation-page) ·
[Ranger of the North replacement (Shire)](#ranger-of-the-north-replacement-shire) ·
[Choosing a company](#choosing-a-company) ·
[Company rules and hero upgrades](#company-rules-and-hero-upgrades) ·
[Create company and aftermath sheet details](#create-company-and-aftermath-sheet-details)

## Recruiting

The **Recruit** button above the units opens a window that stays open through the whole recruit:
1. It shows the company's **reinforcement chart** and, if it has one, its **special chart** as a separate table, the
   company's own notes about reinforcements, and the price per roll (it differs per company: 2, 3 or 4). Companies with
   special units list them below, bought directly at their own cost (Moria's Cave Troll is 15).
2. **Roll** (the player's own roll or "roll for me") pays the price, whatever the result, and shows it in the window with
   the chart row it lands on highlighted.
3. The player may then **raise** the die by 1 per influence point, up to 3 in all for the recruit (both charts together), with the
   new row highlighted live, or just **confirm**. The die moves, whatever the results in between: to reach a result the player
   pays for every point. A roll is never lowered by paying, and never raised past 6 (no influence is taken for points the die cannot move). A result that leads to the special chart keeps the window open for a
   second, free roll (the points not yet used are still available). When a row has two options (Fornost's Hobbit Archer on 2 to 6,
   `Reinforcement.Alt`) the chart shows both ("X or Y") before anything is spent, and the form asks which one the roll takes.
   When the result **cannot join** the company (over the model maximum, a keyword limit, a race balance or a bow/throwing/cavalry limit its gear would break) the influence is spent
   and the window asks for a **lower result** (`ChooseLower`): any lower result of the same chart, free, each with the reason when it
   cannot join either. A player cannot pick a lower result they simply prefer. A unit with a `recruitFallback` over the model
   maximum (Vault Warden Team) may take any result of both charts.
   A chart result that is itself a **choice** ("Choice of any/above", "Choose any 2 Hobbits from above") uses the same panel
   (`RecruitLower.Picks` > 0): the heading is "Choose a result" (or "Choose N results"), the text says the influence is spent and
   how many results are taken. "Choice of any" offers the units of every row of both charts (normal and special), so the roll can be lowered as far as wanted; "Choice of above" / "from above" only the rows above the roll of the chart being rolled. With several picks the panel returns after each one ("Take this result (N to go)"). When no result
   of the chart can join (every rare unit is already in the company) the recruit ends at once with "no result on the chart can
   join the company", the influence spent, instead of waiting for a pick that cannot be made. When the
   company is missing a member of a team (`RecruitRoll.Replace`, Vault Warden Team) the confirm form of a special chart roll
   also offers the result as rolled or the missing member instead, at no cost (`pick` "member"); a team exists while at least one of its members is on the active roster (the cemetery is never consulted) and the missing member is the one the roster lacks; with the whole team gone, or never recruited, nothing is offered and a normal roll recruits a whole new team. The price of the roll leaves the balance at the roll (the Influence card of the page is swapped out of band, `#company-influence`); only the extra points to raise the roll are paid at the confirm.
4. Confirming ends the recruit: the page answers with a notice and the window closes. A "choice of options" result opens the
   choice window (a banner, war horn or war drum option is not allowed) and the unit joins once chosen.

A recruit in progress and a pending choice survive a reload (the window opens on them) and block other recruits. The company
cannot recruit beyond its maximum (15, or 12 for many) nor without enough influence. Both windows are fetched when they
open; their forms answer with the window again (`HX-Retarget` sends the final answer to the whole page) and
`core.ErrRefused` carries the reason when the rules say no.

## Officer locks on the creation page

Everything the server forces on the choice of officers is blocked in the page too, from one source:
`CompanyProfile.OfficerLocks()` (core, `startingcompany.go`) gives, per place of the starting company, an `OfficerLock{Kind, Reason}`:
`leader` (the Shire's Ranger of the North: must start as the Leader), `officer` (a unit that is a hero by its profile, `startingHeroNoStatGain`:
Helm's Deep's Ranger of the North: must be Leader or Sergeant), `barred` (a Hero in the Making of Wanderers in the Wild, a unit On Their Own
Path, or a unit the company excludes: cannot be an officer) or free. `ValidateStart` is built on the same locks (so the page and the
server cannot diverge; `TestOfficerLocksAgreeWithValidateStartInEveryCompany` checks all companies) and stays the safety net: a forged POST gets the
same 422 and message. The page (`start_card` in `company_new.gohtml`, `commission` in `lists.js`): the card carries `data-lock`, an
`aria-describedby` to its reason and a lock icon with "Locked: <reason>"; a `leader` card is pre-placed, immovable and says "Always leads"; an `officer`
card is pre-placed in an officer place and cannot go to the pool (drag, no Remove button), nor be replaced from the pool, but moves between the Leader
and Sergeant places by drag (the card it meets swaps places with it) or with the "Make Leader" / "Make Sergeant" buttons (the keyboard path); a `barred` card has no
assign buttons. The pool buttons that would do nothing (the Leader button while a locked card holds the place) are `aria-disabled` and describedby
the locking card's reason. Rows are `startRow.Lock` in `internal/web/companies.go`.

## Ranger of the North replacement (Shire)

The Shire starts with a Ranger of the North as Leader (fixed: `CompanyProfile.ValidateStart` refuses any other
choice with "The Ranger of the North must start as the Leader of the The Shire company."; the form locks him, see "Locked Leader" at the end of this page), a vanilla Hero (Might, Will, Fate 1 each, so
`startingHeroNoStatGain`). The company's rule: "if the company's Ranger of the North has been slain, you may swap any successful result on the Reinforcements chart
for a Ranger of the North, who may immediately take the role of Leader or Sergeant. The company may only ever have a single Ranger of the North at any time." The
data is the effect `companies[Shire].effects[kind=rangerReplacement]` (`unit`, `max`, `role`, `when`). What the app does (`internal/core/recruiting_ranger.go`):

- **The offer** (`RangerOffer`): the company has the rule, the roster holds fewer than `max` of the unit, and the cemetery holds one (however it fell: Dead or a hero forced
  to retire). It is made on any successful row of the reinforcement chart (not row 1 and not a special chart), also a two-model result or a choice; one Ranger replaces
  the whole result. The swap is free; the roll's price and any raise are paid as for any roll. It is chosen at the confirm of the rolled result, before the company checks
  whether the result can join, so a result that could not join does not send the player to the lower-result panel when the Ranger can join.
- **The recruit** is a Hero (no Might, Will, Fate gain) with vanilla wargear only, and no path yet: he chooses it in the usual path window (the company cannot be put in a
  match until he has). He is a direct Hero, not a Hero in the Making: no promotion roll.
- **Officer reshuffle.** Choosing the swap pays the roll, checks that the Ranger can join and parks the recruit as `Recruit.Officers` (an `OfficerStep`, part of the
  company's pending JSON): the recruit window turns into the officer panel and stays open on a reload, other recruits and the actions the aftermath lock blocks are
  refused, and so is `CreateMatch` ("Arrange the officers first"). My Company shows the banner `#officers-banner` ("Your company is waiting for its new Leader") that opens the panel.
  The panel is a sibling of "Take command" (`officers_panel`, `officer_card` in `company_panels.gohtml`; the Alpine/Sortable `officers` component of `static/js/lists.js`):
  the Ranger is pre-placed as Leader and can move between the three officer places (drag, or the Leader / Sergeant buttons for the keyboard) but never leaves them: his
  card has no remove button and the pool refuses him. The old Leader and the two Sergeants (the other officers) fill the other places and the player decides which of the
  three stops being an officer; that unit becomes an ordinary Hero (`RoleHero`) and keeps path, level, XP, gear and injuries. **Appoint** stays disabled until the
  arrangement is valid, with the reason under it; there is no cancel, because the influence is spent. With fewer than three other officers nobody is dropped: the Sergeant
  places left empty can be filled from the other active Heroes (`Candidates`) or by the usual officer succession of the aftermath.
- **Contract.** `RecruitService.PendingOfficers` returns the `OfficerPanel{Unit, Leader, Officers, Candidates, Drop}` (nil when nothing waits) and
  `ArrangeOfficers(ctx, user, campaignID, OfficerArrangement{Leader, Sergeants})` applies the arrangement (the Ranger is id `0`); `ConfirmRecruit(..., "ranger")` starts the step.
  Every invalid arrangement (Ranger left out or named twice, a unit named twice, a non-officer as Sergeant while three officers exist, the wrong number dropped, no pending
  step) is an `ErrRefused` with nothing stored. The web routes are `GET .../my-company/{company}/recruit` (shows the panel whenever one waits) and
  `POST .../my-company/{company}/recruit/officers` (`leader`, repeated `sergeant`), which answers with the whole page, or the panel again with the reason and status 422.
- A fallen Ranger is an ordinary casualty (a Leader falling still opens the succession of [aftermath.md](aftermath.md)); the cemetery tag `RangerMayReturn` says "May return
  with a reinforcement" and the aftermath alert `RangerFell` says he returns as the Leader.

## Choosing a company

The page to create a company (`/campaigns/{id}/company/new`) is a browser of every company of the campaign's Battle Companies
file (`CompanyProfiles`, `core.CompanyProfile`): a searchable list with an alignment filter on the left, and the chosen company's page
on the right (loaded as a fragment, `company/info?c=NAME`): history, facts (models, reinforcement cost, starting gold, break point),
starting company, reinforcement and special charts, special units, the promotion table, what only the company has (exclusive wargear,
hero upgrades, special rules, free rules, own-name units, exchanges) and its limits and notes. Every unit named (in the starting
company, charts, special units and promotions) shows its card on hover or focus (`core.UnitPreview`, drawn from the vanilla profiles;
a company's own label for a unit is resolved to the profile it is); items and rules show their text. The creation form is part of the
page. The page ends with **Take command**: three places (a leader with three chevrons, two sergeants with two) and, below them, the
starting models. The player drags three models into the places (SortableJS; a model dropped on a taken place sends the one that was
there back to the pool) or uses the Leader, Sergeant and Remove buttons of a card, which also work with a keyboard. Models the
company does not let lead (`startingHeroRestriction`: Lake-town only the Lake-town Guard, Cirith Ungol not the Spider) cannot be
placed. The company's own suggestion (the roles its file gives, else the first models) starts in the places, marked "Suggested"
until the player changes anything. An officer keeps its profile (the Arnor Rangers of Arnor stay Rangers of Arnor as officers: the company's hero promotion belongs to the level-up roll), so the cards and the confirmation dialog show the same unit in the pool and in the places; a start unit the company already makes a Hero in the Making (Wanderers) cannot be picked. The "On Their Own Path" badges on that page follow the campaign's `CustomOnTheirOwnPath`: without it they never become heroes and earn no experience, with it they are warriors that earn experience and become heroes on a 6. Below, the company's **oath** (`data.companies[].oath`: text with `{name}` where the player types
the company's name, and the label of the vow button; companies of a custom file fall back to `campaign.company.oath` for their
alignment) and the vow button, which opens a "These decisions are final" confirmation with a recap. The officers and the company are
final; the name can be changed later from My Company (the pencil by its name, `RenameCompany`). `CompanyProfile.ValidateStart`
checks one leader, two sergeants, distinct models that may lead. A hero starts with a point of Might, Will and Fate, except the
units the company lists in `startingHeroNoStatGain` (Ranger of the North in Helm's Deep and the Shire).

### Choosing a path later

The leader and the sergeants start as heroes without a path (`CompanyUnit.PathPending`): their cards
carry a "Choose a path" banner that opens the same kind of window as a hero in the making (`PathOptions`, `ChoosePath`: paths that
are at their limit are listed with the reason, and a Sorcerer chooses its first magical power). The path's heroic action comes with
it. A pending officer cannot level up and the company cannot be put in a match until every hero has a path (`CreateMatch` refuses). A hero starts with a point of Might, Will and Fate and its
path's heroic action, except the units the company lists in `startingHeroNoStatGain` (Ranger of the North in Helm's Deep and the
Shire).

## Company rules and hero upgrades

- A company's **epigraph** (`CompanyProfile.Epigraph`, optional `epigraph {quote, text}` in the Battle Companies file; only Arnor has
  one) is flavour, drawn on the Choose-your-company page at the head of "Only this company has", above the cards (`[data-epigraph]`):
  the quote in italic and medium weight, the sentence under it in the same italic muted style as a hero upgrade's flavour. Nothing is
  drawn for a company without one. The company overview shows no rule cards (only chips), so it draws no epigraph.

- A **hero upgrade** of the company is offered with every hero level-up as a third option, in a gold frame ("Company hero upgrade"): a
  hero may take it instead of either result, even when neither result can be taken, if it does not have it yet (the Battle Companies
  rule). It appears on the hero as a company-exclusive rule (gold frame, tooltip "Company-exclusive hero upgrade").
- **Applied** (`applyCompanyRules`, `buildUnit`): the maximum number of models, limit overrides and exemptions, free special rules
  some models get (Rohan's heroes: shown as company-exclusive rules), units with their own name (resolved to the vanilla profile, with
  the rules the company adds), exclusive wargear (marked company-exclusive in the armory, with its text), and the share of the company a
  keyword may have (a limit bar, and enforced: a reinforcement, special unit or wanderer of that keyword is refused over the limit; a
  reinforcement roll that lands on it spends the influence and nothing joins).
- **Race balance** (Defenders of the North: Dwarves not more than Dale): a reinforcement that would outnumber is replaced by the highest
  lower result that does not (the rule lets the player choose any lower result; the app takes the highest and says so).
- **Hero upgrade restrictions** (Fiefdoms' four, Hobbit, Spiders, Orc/Warg) are matched against the hero's vanilla `race`, `faction`,
  unit types and name (`HeroUpgrade.Allows`, plurals count), so a Hobbit of the Wanderers in the Wild is offered both upgrades and the
  others only the open one. A level-up offers one upgrade, and a hero takes each once. What a skill does to units of one kind in a match
  is the player's to apply at the table.
- **Rule exchanges are applied** to the models (`applyCompanyRules`): Sharkey's Rogues' Hatred (Rohan) is Hatred (Hobbit) on every card.
- **Played at the table:** substitutions of rules and auras are match-layer rules the app does not apply to the cards. The data has them as
  `companies[].effects[]` with a `layer` (management, match or both).

## Create company and aftermath sheet details

- **Locked Leader.** A company whose start list names a Leader that is also its Ranger replacement unit (the Shire's Ranger of the North,
  `startRow.Locked`) shows him in the Leader place with `data-keep`: he cannot be dragged, removed or replaced, the Leader button does
  nothing and the card says "Always leads". The core validates the same rule.
- **Aftermath sheet.** The experience a unit earned are small toggle chips (`chip`, a tick when on) under "Earned experience"; the
  outcome labels of a saved sheet are `badge-success` (survived), `badge-warn` (fled) and `badge-danger` (casualty), tints of the semantic
  tokens ringed in the colour (in `tailwind.css` the `.badge` base rule comes before every `badge-*` tone: it has the same specificity and
  would otherwise win; `TestBadgeBaseComesBeforeTones`, and the e2e flow `81-aftermath-ui` reads the computed colours). The injuries step spells out the bonus die of Lesson Learned and Wounds of a Hero (`BonusLine`).


See also: [frontend-company.md](frontend-company.md) (the roster), [frontend-level-up.md](frontend-level-up.md), [aftermath.md](aftermath.md), [definitions.md](definitions.md), [frontend.md](frontend.md).
