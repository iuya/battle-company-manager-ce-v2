# Frontend: glossary, item definitions and the helper popup

How rule texts are explained in the app: the campaign Glossary tab and its unit cards, item definitions, links between terms and the helper popup.
The design language and the page patterns are in [frontend.md](frontend.md).

## Contents

[Glossary and item definitions](#glossary-and-item-definitions) ·
[Glossary](#glossary) ·
[Helper popup](#helper-popup)

## Glossary and item definitions

The Armory lists the full definition of every item (the rules file's annotation text, the stat line and the price). A campaign's
Glossary tab is its glossary file plus everything the campaign's vanilla and Battle Companies files define (`core.GlossaryExtras`):
special rules, magical powers (with the Sorcerer's casting value and range), wargear, equipment and mounts, paths, path rules
and heroic actions, creatures, wanderers and injuries, sorted by category and then by name, with a heading per category (hidden when a search leaves it empty), a category badge on every term and a category filter; the default glossary of the sidebar is only the
glossary file. When a name is defined in several places Battle Companies wins (it is a game mode built on the base rules): its term replaces
the glossary file's term of the same name; the glossary file wins over the vanilla file.

**Unit cards.** A campaign's Glossary tab also has a card for every base unit its ruleset's companies name (`core.RuleSet.BaseUnits`, `core.Glossary.Units`: the units of the
starting companies, charts, special units and promotions, one card per distinct name; 120 in the shipped ruleset), under a "Units" heading, with a "Units" chip in the category
filter (`?c=units`) and the search finding them by name, by the base profile of a company's own version (Helminga: Rohan Royal Guard) and by company. A card is the redesigned unit card as a static reference (`glossary_units` in `partials/glossary.gohtml`, with the card's own classes: the title plate with the kind icon, name, type and points, tinted for a hero; the datasheet `statline`; the body with wargear, heroic actions and special rules; the keyword foot), without trackers, experience or rename. Besides what the new-company hover card shows it has the
options the profile may take (with their points), the units a bundle brings (Vault Warden Team), the company's own text and "Named by" the companies that use it. The sidebar
glossary (a bare file, no campaign) has none.

- **Stable anchors.** Each card is `<article id="u-<name>" tabindex="-1">`; `core.UnitAnchor(name)` makes the id: `u-` and the `NameKey` of the name with dashes
  (`u-warrior-of-minas-tirith`). A link to a unit is `/campaigns/{id}/glossary#<UnitAnchor>` with `hx-boost="false"` like a term link. Two companies that give one name to different
  profiles get one card each, the later anchored with the company's name. In the page, `#u-` links are handled like `#t-` ones: the search and category clear, the card is brought
  into view, highlighted and takes the focus.
- **Fetched on demand.** The 120 cards are 0.37 MB of HTML (19 KB gzip, 3.1 KB a card), on top of a tab that is already 1.5 MB, so the tab leaves them out: it counts them and
  shows a placeholder with a "Show the N unit cards" button. `GET /campaigns/{id}/glossary/units` (a campaign member; 404 for a ruleset without units) returns the cards as the
  `glossary_units` fragment, which replaces the placeholder (`htmx.ajax` from `loadUnits` in `static/js/lists.js`, one request however many things ask). The button, choosing
  the Units chip, typing in the search, opening `?c=units` and an address with `#u-...` all fetch them. The tab and the fragment are two of the pages `node tests/e2e/pagetimes.mjs` measures ([performance.md](performance.md)): the tab stays at about 27 ms of server time and 1.5 MB (198 KB gzip), the fragment is 7.5 ms and 365 KB (19 KB gzip).

### Unit names in texts

A unit named in a rule or glossary text is a link to its card in the campaign glossary, with a tooltip, like a glossary term. The campaign's glossary (`core.RuleSet.GlossaryWithUnits`, built once per
rule set, `glossary_units.go`) has the base units in its linker, so `Glossary.Link` and the definitions of its terms return `Segment`s with a `UnitID` (the card's anchor) next to the `TermID` ones. Where a text is
shown through `linkText` / `companyInfoView.Linked` (`segViews`) the unit's tooltip comes with the link, so there is no request on hover:

- **Matching.** Exact names only, the ones in `BaseUnits()` (so only the units of the ruleset in use): whole words (`isWordRune` boundaries, a hyphen is part of a name, never inside another word), case insensitive for names of
  several words and exactly as spelled for a name of one word (`Ruffian`, `Spectre`, `Werewolf`, `Crebain`, `Helminga`), at least three characters, the longest name first (`Erebor Dwarf Warrior` is one unit, not a `Dwarf Warrior` in it; a
  glossary term inside a unit name, `Dwarf`, `Warrior`, is part of the name), each unit linked once per text. A name that is also a glossary term (`Fell Warg`, `Lorien Guard`, `Tookish Hunter`, `Warg Marauder`: the wargear and the rule) stays the term.
  There are no aliases or plurals: the data has none for units (a company's "base profile" is not another name for it). A name two companies give to different profiles links to the first card (`UnitAnchor(name)`).
- **The link.** `<a class="term-link unit-link" href="/campaigns/{id}/glossary#u-…" hx-boost="false" data-tip-title data-tip-meta data-tip-text>`, from the `segments` template: a full load, so the browser jumps to the card and the
  glossary component fetches the cards if needed, scrolls, highlights and focuses it. It has no `data-concept` (a unit is not a concept in the graph), so a click follows the link where a term opens the helper popup.
  On the glossary page itself the link is the in-page `#u-…` one (`glossary_unit_link`) with its own `data-tip-*`, since the card may not be loaded: its preview reads them first (`showTip` in `lists.js`).
- **The tooltip** is the unit's profile in short (`BaseUnit.TipMeta`: types and points; `TipText`: the characteristics on one line, then "Made of", "Wargear", "Heroic actions" and "Special rules" a line each), about 300 bytes
  per link on the page. The card in the glossary has the rest (options, the companies that name it). Hover and keyboard focus show it in the page's own tooltip (the company browser, the unit cards, the glossary), Escape hides it,
  a touch screen follows the link to the card. A unit with no card is plain text: a glossary without units (the sidebar glossary, a ruleset whose companies name none) links no unit.
- **Where.** Every text cut by the campaign glossary: the company pages' rule texts and notes (the Arnor promotion), rule, item and path descriptions on the unit cards, the level-up choices and the definitions on the glossary tab.
  After an in-page jump the glossary scrolls again once the web fonts are in (the page is 280,000 px long and the lines above a far card change height when the fonts arrive).
- **Tested:** `internal/core/glossary_units_test.go` (the matcher: names inside other names and other words, case, punctuation and hyphens, the same name twice, terms, the shipped ruleset), `internal/web/unitlinks_test.go`
  (generated mock of `glossaries`: the Arnor page, a ruleset without cards, the glossary page) and the e2e flow `102-unit-links` ([testing.md](testing.md)).

## Glossary

`/campaigns/{id}/glossary` shows the glossary the campaign is played with, and the sidebar's `/glossary` the one the app
ships with (`partials/glossary.gohtml`). Terms are cards with their category, aliases, definition, related terms
("See also") and rulebook pages; a search box filters them in the browser by name and alias.

The links between terms are made by the backend, not the JSON: `core.BuildGlossary` indexes the names and aliases, and
turns each whole-word, case-insensitive mention of another term into a link (the first time per definition, never to the
term itself); the page just renders the segments (`core.Segment`), so no HTML comes from the data. Links are `#t-<id>`
fragments handled by the `glossary` component (`static/js/lists.js`): following one clears the search first (so a link
in a filtered list still finds its target), jumps to the term and highlights it, and remembers where you came from; a
floating **Back to <term>** button returns there (a trail, so it works through several jumps) and **Top** goes to the top.
Hovering (or focusing) a link previews its term in a small card (name, category, definition as plain text, so previews
never nest); touch screens just follow the link. A single-word name or alias is linked only when written exactly as in the file ("will" is not the Will term, "Will" is);
multi-word names stay case insensitive (the case rule applies to every single-word term). A campaign's glossary also links the unit names its texts mention to the unit cards ([Unit names in texts](#unit-names-in-texts)). `web.glossaries` (`core.Glossaries`, which builds once per file) gives a campaign's glossary, the default one, or one glossary file alone by
hash (`ByHash`, `core.ErrNotFound` for an unknown hash or a file of another kind).

A row of **category chips** (All plus one per category, `aria-pressed`) sits under the search and combines with it; the
choice is kept in the URL query `?c=<category>` (an unknown category is ignored), and the glossary's name, version and short
hash show under the title. When nothing matches, a status line names the search and category and offers "Clear the filters".

The sidebar page can show **another glossary**: a collapsed "Use another glossary" form with the rules file picker (kind glossary: a
file the server already knows by hash, or an upload; `POST /glossary`, `rulesetChoice`) keeps the file's hash in the `bcm-glossary`
cookie (a year, this browser only). With a cookie the page shows that file through `glossaries.ByHash`, says it is not the default
and offers **Back to the default** (the cookie is cleared); an unknown or malformed hash falls back to the default with a notice and
drops the cookie. The campaign tab always shows the campaign's own glossary.

## Helper popup

Tapping or clicking a term opens the **helper popup**: what the rules say about it, and the things it is linked to
([`concept-graph.md`](concept-graph.md) is the contract; the graph is `core.Concepts`, wired in `cmd/server` and given to the server with
`SetConcepts`). The page keeps its hover tooltip (a desktop hover shows it as before); a click, a tap or Enter opens the popup instead,
and the tooltip steps aside.

- **Terms that open it** carry `data-concept` next to their `data-tip-title`: the glossary links inside descriptions (`segments`), the rule,
  heroic action, spell, path rule, unit upgrade, injury, creature and equipment chips of the unit cards (My Company, the live company page,
  other companies), the armoury's chips and the company browser's terms. The name looked up is `data-concept`'s value or, when it is empty,
  `data-tip-title`. A glossary link keeps working with Ctrl/Cmd/Shift-click, in a new tab and on a page without a campaign in its address.
  The sidebar's own glossary page (`/glossary`) and the glossary page itself are left as they are (their links jump within the page).
- **Endpoints** (campaign members only, behind the recover and timeout middleware, answers are HTML fragments of
  `partials/concept.gohtml`): `GET /campaigns/{id}/concepts/{concept}` (one concept by id: 200, or 404 with the "no longer in these rules"
  fragment) and `GET /campaigns/{id}/concepts/resolve?name=...` (a displayed name such as `Fury (2)`, an alias, any case: 200 with the
  concept; an ambiguous name lists its meanings and picks none; an unknown name is a 404 and the page falls back to the tooltip). Both
  send `Cache-Control: private, max-age=3600` and an `ETag` made from the campaign's ruleset triple (and the partial's own hash, so a deploy
  does not serve old markup); a matching `If-None-Match` is answered 304 before the graph is asked. Without a graph (`SetConcepts` not
  called) the routes do not exist.
- **Content:** a kind badge, the name, the text (plain, escaped; words that are other concepts are buttons), then groups: Grants / Granted by,
  Cancels / Cancelled by, Lifts / Lifted by, Requires / Required by, Mentions, Mentioned in, See also (both ends, once each); a group with
  no entries is not drawn. In the groups of the typed edges the chips that share a reason (`via`) fold into one closed line with a count once
  there are three of them ("Defence bonus is lost with a bow weapon (9)": the shield's 54 explicit cancels read as a few lines); chips with a
  reason of their own show it beside them. Mentions and See also cut after ten chips behind "Show N more". "Open in glossary" (the
  campaign's glossary at `#t-<id>`, `hx-boost="false"`) is hidden for a concept with no glossary term.
- **Behaviour** (`static/js/concepts.js`, in the bundle after `dialog.js`): the sheet is built when the first term is opened and removed on
  close. A chip or linked word fetches that concept and swaps the content; **Back** returns through the steps (kept in memory, as is every fetched
  fragment). While loading it shows a skeleton; a network error or 5xx shows "Could not load this" with **Retry**; a name the graph does not
  know (or an old server) shows the tooltip's own text (`data-tip-text`, with the glossary link of a link). It closes with **Escape**, the
  backdrop, **Close**, or a page change (htmx navigation, back and forward). The sheet is a `role="dialog" aria-modal="true"` with the concept's name as its label;
  focus moves to the heading, Tab and Shift+Tab stay inside (the script keeps this itself: `data-focus-managed` keeps `dialog.js` out of it),
  Escape closes only the popup (a unit card or window under it stays), and the focus returns to the term. Under `md` it is a bottom sheet (full width, at most 70% of the
  height, scrolling inside, dimmed backdrop, the page behind does not scroll); from `md` up a 24 rem card placed beside the term (below it, above
  when there is no room) over a transparent backdrop. Everything uses the semantic tokens (`.concept-*` in `tailwind.css`).
- **Safe by construction:** no inline script or handler; the fragment is made by the server from text of the rules files, escaped by
  `html/template`; the script inserts markup in one place only (the fragment) and positions the card through the `style` property
  (allowed under the policy). `TestHelperPopupScriptFitsThePolicy` guards it and the e2e suite runs under `CSP_MODE=enforce`.
- **Tested:** `internal/web/concepts_test.go` (generated mock of `concepts`: body, groups, folds, escaping, glossary link, caching and 304,
  404, ambiguous and unknown names, non-members and anonymous visitors) and the e2e flow `87-concepts` ([testing.md](testing.md)).


See also: [concept-graph.md](concept-graph.md) (the popup's data), [unit-keywords.md](unit-keywords.md), [definitions.md](definitions.md), [frontend.md](frontend.md).
