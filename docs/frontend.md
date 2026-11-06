# Frontend

How the web pages are built, how they behave and what each screen does.
It is for developers who change templates, styles or scripts, and for anyone who wants to know how a screen works.

The stack is server-rendered Go templates, **htmx** for navigation and partial updates, **Alpine.js** for client-only UI state and **Tailwind CSS v4** for styling.
There is no JavaScript build step: htmx and Alpine are vendored in `internal/web/static/js`, and Tailwind runs as a standalone binary.

## Contents

**Design language**
[Principles](#principles) ·
[Responsive design](#responsive-design) ·
[Theme](#theme) ·
[Components](#components) ·
[Dice](#dice)

**Page patterns**
[Pages and htmx](#pages-and-htmx) ·
[App shell and navigation](#app-shell-and-navigation) ·
[Adding a page](#adding-a-page)

**The screens** are described on their own pages:

| Page | What it covers |
|---|---|
| [frontend-campaigns.md](frontend-campaigns.md) | The campaign list, the campaign pages, the rules-file report and online validator, the admin hand-over of a campaign |
| [frontend-company.md](frontend-company.md) | My Company: the unit card, the Armory, equipment, healing, formation, Gold, creatures, the aftermath cards |
| [frontend-company-building.md](frontend-company-building.md) | Recruiting, the creation page, choosing a Battle Company, company rules |
| [frontend-level-up.md](frontend-level-up.md) | The path mosaic and carousel, the Sorcerer's magical power window, the level-up window |
| [frontend-live-match.md](frontend-live-match.md) | The live match: banner, trackers, milestones, Break Point, the End match and Confirm sheets, sync, the Wound chart window |
| [frontend-glossary.md](frontend-glossary.md) | The campaign Glossary tab, item definitions, links to terms and units, the sidebar glossary, the helper popup |
| [frontend-account.md](frontend-account.md) | The Account page, invitation and recovery links, invite links, the minimal demo sign-in |
| [tabletop-simulator.md](tabletop-simulator.md#copy-as-a-tabletop-simulator-army-list) | The "Copy as a Tabletop Simulator army list" dialog |
| [aura-ranges.md](aura-ranges.md#aura-chips-on-the-unit-cards) | The aura chips of the unit cards |

## Principles

1. **Fast to navigate.** The server renders HTML. Navigation and form submissions swap only the page content
   (see [Pages and htmx](#pages-and-htmx)). Anything that needs no data from the server is done in the browser with
   Alpine (disabling a button, toggling a menu) instead of a request.
2. **Responsive everywhere.** Every page works from a 360 px phone to a wide desktop; see
   [Responsive design](#responsive-design).
3. **Cheap to load.** Body text uses the system font. One self-hosted web font (headings), no framework, small CSS,
   embedded assets with cache headers.
4. **Accessible.** Colours pass WCAG AA, controls are keyboard reachable with a visible focus ring, form fields have
   labels, errors use `role="alert"`.

## Responsive design

Responsive behaviour is a requirement of every page, not a finishing touch.

- **Design mobile first**, starting at 360 px wide, then add `sm:` / `md:` / `lg:` variants for larger screens.
- **No horizontal page scroll at any width.** Use fluid containers (`max-w-*` with side gutters `px-4`), `flex-wrap`
  and `grid` with responsive columns. Avoid fixed widths in `px`. Wide content (tables) scrolls inside its own
  `overflow-x-auto` wrapper or becomes stacked cards on small screens.
- **Touch friendly.** Targets are at least 40 px tall on phones, with room between them.
- **The header collapses.** Below `sm:` the navigation links and Sign out move into a menu, leaving room for the brand.
- **Text wraps, never truncates** important content (names, rules).
  `main` has `overflow-wrap: anywhere`, so a 60-character name without spaces breaks instead of widening the page.
  Headings are set for the narrow case and may break deliberately (the login heading breaks after "Battle Companies").
- **Phone details.**
  - The campaign tab bar scrolls sideways, with an edge fade where there is more and the current tab brought into view (`static/js/tabs.js`).
  - Filter chips, steppers and `summary` rows are 44 px tall.
  - Small controls (the unit card corner tab, the item add chip) get an invisible larger touch area on coarse pointers.
- **Background and images scale with the viewport** (`vmin`/`vw` units, `max-*` limits), never with fixed pixel sizes.
- **Check a page at 360, 768 and 1280 px** (and in both modes) before it is done.

## Theme

Colours are **semantic tokens**: CSS variables, exposed to Tailwind as utilities (`bg-page`, `text-fg`, `border-line`,
`bg-accent`…). Components use only these names, never raw colors, so a palette is one block of variables in
`tailwind.css`.

| Token | Use |
|---|---|
| `page` / `surface` / `raised` | Page background / cards, header, inputs / hover and secondary areas |
| `fg` / `muted` / `line` | Text / secondary text / borders |
| `accent` / `accent-fg` | Primary action background / text on it |
| `accent-text` | Accent-coloured text and links on `page` or `surface` |
| `accent-soft` | Tinted backgrounds (selected state, badges, info alerts) |
| `danger`, `success` (+ `-soft`) | Errors and confirmations |
| `warn` | Draws (yellow). With `success` and `danger` it gives the `tone-win` / `tone-draw` / `tone-loss` classes for results |
| `gold` | Decoration: the ring background, gold badges, the level medal |
| `leader`, `sergeant`, `hero`, `leader-soft`, `sergeant-soft`, `hero-soft` | Game colours. A unit's place on its card: frame and insignia (leader amber, sergeant green, hero blue) and the tint of its title plate. `leader` is also the command paths (Path of the Warlord) and the Might tracker |
| `path-melee`, `path-ranged`, `path-magic` | Game colours. The path kinds (red, green, blue) and the Wounds, Fate and Will trackers |

### Palettes

There are three palettes, each with a light and a dark variant:

- **Forest & Gold** (default)
- **Parchment & Ember**
- **Mithril & Night**

A palette is selected by `data-palette` and `data-mode` (`light` | `dark`) on the `<html>` element; the same
attributes on any element preview that palette there. `tools/themecontrast` checks every palette and mode
against WCAG AA (Web Content Accessibility Guidelines level AA: 4.5:1 for text, 3:1 for the accent).

In the dark variants the layers are kept apart on purpose:
`surface` sits clearly above `page` (about 1.2:1), `raised` above `surface` (about 1.25:1) and `line` reads against
`surface` (about 1.9:1 or more), so cards and borders do not melt into the page.

### Game colours

The leader, sergeant, hero and path tokens mean the same thing on the table whatever the palette, so they
are not part of a palette: one block per mode and contrast (`[data-mode]`, `[data-contrast="high"] [data-mode]`), tuned from
Mithril dark high contrast.

A unit card carries no palette accent: game colours and neutrals only (the experience bar,
the Might/Will/Fate frame, path and spell chips, a stat raised by gear).
The checker tests each palette together with the game colours of its mode and contrast.

### High contrast

Every palette and mode also has a high-contrast variant, selected by `data-contrast="high"` on `<html>`.
The blocks match `<html>` itself and the preview swatches inside it.

The checker holds the variant to WCAG AAA:

- text 7:1;
- the accent, `gold`, `sergeant` and `hero` 4.5:1;
- `line` 3:1 against `surface` and `page`.

Other rules of the variant:

- The `gold` token (ring, medals, gold badges) is the same amber in every palette.
- The focus ring takes the complement of the palette's accent so it stands apart: pink in Forest (green accent), cyan in Ember (red accent). Mithril already pairs blue with yellow.
- The game colours stay amber, green, blue and red.

The **High contrast** checkbox sits in `theme_controls`, below the palettes, and in the `/styleguide` theme lab.
It is wired to `$store.theme.highContrast` / `setContrast` and the input `[data-contrast-toggle]`.
It writes the cookie `bcm-contrast=high|normal`, per browser and not in the account.
Without that cookie `theme-init.js` follows the operating system's `prefers-contrast: more`.

### Choosing a palette and mode

**Mode** is System by default (it follows the operating system), or Light or Dark.
Palette and mode are chosen in the settings menu (gear icon in the header) and kept in the `bcm-theme` cookie as `<palette>.<mode>` (for example `forest.dark`).

- An inline script in the layout's `<head>` applies the choice before the first paint, so there is no flash.
- `static/js/theme.js` holds the shared Alpine store (`$store.theme`).
- Signed-in users also have the choice stored in their account ([auth.md](auth.md), "Appearance per account").
  - The layout puts `data-auth` and `data-pref` on `<html>`, and the head script and the store prefer `data-pref` to the cookie.
  - `$store.theme.set` also calls `PUT /account/appearance` (silent on failure).
  - Sign-in saves the browser's cookie to an account that never chose, or writes the saved look to the cookie.
- The Account page's Appearance card reuses the menu's `theme_controls` partial.

### Typography

Headings use self-hosted *Cormorant Garamond* (600/700, `font-display: swap`) with lining numerals so
digits match the capitals. Body text uses the system UI font.

### Ring background

`body::before` paints `static/img/ring-text.svg` (the One Ring inscription) centered on the
viewport in the palette's `gold` at low opacity, through a CSS mask. In landscape the whole ring fits the screen; in
portrait it is sized by the width so it shows around the cards. Cards and the header are opaque and cover it.

### Icons

The favicon and app icons are traced from the inscription (`tools/icons`): `favicon.svg` (bold, for
tabs), `icon.svg` (finer, large uses), PNG fallbacks and `ring-text.svg`. The ring is gold on a transparent
background so it reads on light and dark pages; only `apple-touch-icon.png` is opaque, because iOS would paint it black.

## Components

Defined in `tailwind.css` (`@layer components`) on top of the tokens:

| Class | Use |
|---|---|
| `btn` + `btn-primary` / `btn-secondary` / `btn-ghost` / `btn-danger` | Buttons (labels never wrap) |
| `field` (+ `.hint`, `.error`) and `input` | Labeled form fields; `aria-invalid="true"` styles the error state |
| `card` | Raised container |
| `alert` + `alert-info` / `alert-success` / `alert-danger` | Messages |
| `badge`, `divider` | Small labels, separators |
| `practice_badge` (template partial) | The "Practice" pill of a practice match: `badge badge-muted` with an icon and the word, never a win/loss colour; text 12 px; in `/styleguide`. Used on the Matches list, the Overview, the match page, the aftermath page and the live bar |

`/styleguide` renders every token, a live contrast table, the type scale and all components in the active theme.
Use it to review any change to the design language.
It is served only when `STYLEGUIDE=true`, which `make run`, `make dev` and `make demo` set.

## Pages and htmx

Templates live in `internal/web/templates`:

- `layouts/base.gohtml`: the page shell (`<head>`, top bar, sidebar, `<main id="main">`, footer).
- `pages/*.gohtml`: one file per page, defining `title` and `content`.
- `partials/*.gohtml`: reusable fragments, also returned alone by handlers for in-place updates.

Each page is rendered with its own copy of the layout plus all partials, so pages cannot clobber each other's
blocks. Page data embeds `pageBase` (the signed-in user and the dev flag) so the layout can draw the header.

`<body>` has `hx-boost`, `hx-target="#main"` and `hx-swap="innerHTML show:window:top"`. A link or form is therefore an
htmx request that swaps only `#main`:

- A **full** navigation gets the whole layout. An **htmx** request (`HX-Request`) gets only `<title>` and the page's
  `content`; htmx lifts the title into `document.title`. Responses carry `Vary: HX-Request`.
- A **partial update** (for example a form that refreshes one panel) returns just the partial and the form targets it with
  `hx-target`/`hx-swap`.
- **Links to a glossary term** (`/campaigns/N/glossary#t-...`, from a tooltip chip or a "Glossary" link) carry `hx-boost="false"`: a boosted swap would run the glossary component before the address has its `#fragment`, and scroll to the top instead of the term. A full load lets the browser jump and the component highlight the term.
- **Wargear names** are lower case in the rules file; the `upfirst` template function (`sentence`) gives them a capital wherever the company chooser shows them (the same helper as for messages).
- **Swapped content is wired at once.** htmx reads the `hx-*` attributes of a swapped-in fragment only when it settles (20 ms after the swap), and a click in that gap would send nothing (a sheet menu entry that did nothing under load). `static/js/htmx_ready.js` calls `htmx.process` on the swap target in `htmx:afterSwap`, so the new buttons work from the moment they are on screen. The e2e step "a menu entry clicked the instant it is swapped in" holds it.
- **A live refresh waits for the pointer.** Every refresh of `#live-zones` replaces the Actions buttons, so a click whose mousedown and mouseup straddle a swap would get no click event (the pressed element is gone). `static/js/live.js` holds the swap of a refresh (`GET .../live` into `#live-zones`, dropped in `htmx:beforeSwap`) while a pointer is down inside the zones or a request started from inside them (a menu or sheet being opened) is still running, and fetches the zones once more right after `pointerup`/`pointercancel`/`dragend`/the request's `loadend` (the latest state; nothing stale is kept). A hold ends by itself after 2.5 s, so a lost `pointerup` cannot freeze the live view. The e2e step "a click that straddles a live refresh still lands" (`40-match`) presses, forces a refresh, checks the pressed button is still there, releases and expects the click and then the new zones.
- **Double submit.** `static/js/submit.js` disables a form's buttons while its request is in flight and ignores a second submit. On the server, making a match or a campaign is deduplicated for 10 s (the same person, the same content opens the first one), and the actions that spend or roll (recruit, level-up, purchases, hires, treatment, perks, grants, invites, aftermath rolls and decisions) run behind `Server.once` (`internal/web/dedupe.go`): a request identical (same address and body) to the same person's previous guarded request, within 3 s, gets that answer instead of running again; any other request in between ends the match, so confirming a recruit, rolling on the special chart and confirming again runs both confirms.
- **Refusals and failures** go through `Server.refuse` / `notFound` (`internal/web/errors.go`), keeping the status code: a full page request gets the `error` page in the layout (title, heading, the reason, a way back: the campaigns, or Sign in for a visitor), an htmx request gets the short plain text that the layout's error banner shows. A 5xx logs its cause and shows a fixed sentence. A catch-all route makes unknown addresses a rendered 404.
- **Validation errors** are returned with status `422` and swapped in place (htmx is configured to swap 422). Any other
  failure of an htmx request (403, 404, 500, no connection) is not swapped; the app shell shows it in a dismissible red
  alert under the header (`err` in the shell's Alpine state, filled from the plain-text body of a 403/404 or a generic message), so a click
  never does nothing silently.
- **Sign-in and sign-out** answer htmx requests with `HX-Redirect` (a full page load) so the header, which depends on
  who is signed in, is drawn again. A protected page requested without a session does the same toward `/login`.

### Windows

Windows (`role="dialog" aria-modal="true"`, shown by `x-show`) are handled by `static/js/dialog.js`:

- Focus moves into the window when it opens, Tab stays inside it, and focus returns to the opener when it closes.
- Escape and a click on the backdrop close it, and the body scroll is locked while it is open.
- Windows can nest.
  `dialog.js` keeps a stack of open windows and treats the innermost shown one as the active window.
  Focus stays trapped in the inner window, Escape closes only that one, and focus goes back to the control that opened it (the Sorcerer's "Select magical power" list inside the path window, see [Path carousel](frontend-level-up.md#path-carousel)).
- The close button is 44 px square.
- In running text (`p`, `li`, `td`, `dd`) links are underlined, so colour is not the only cue.

### Static files

Static files are served by `internal/web/static.go` (measurements in [performance.md](performance.md)).

- Templates name a file with `{{static "js/x.js"}}`, which adds `?v=<content hash>`.
- A request with the current hash is `Cache-Control: public, max-age=31536000, immutable`, so a page costs no request for the file.
- `app.css` is served with the same `?v=` on the fonts and images it loads.
- Text files are gzipped at start and served with an ETag (304 on a match).
  `STATIC_MAX_AGE` only governs addresses without `?v=`.
- Pages load three scripts:
  - `theme-init.js` and `tz.js` (in the head, synchronous);
  - `js/app.js`, a bundle made at start of htmx, the theme store, the page scripts, SortableJS and Alpine, in the order of `bundleScripts`.
    A new script goes into that list, not into the layout.
- Pages and fragments over 1 KB are gzipped for browsers that accept it (`compress.go`; the live stream is not).

### Loading rules

These rules keep pages cheap to load (measured in [performance.md](performance.md), "Frontend pass at v2.0.0").

- The heading font is preloaded in the layout under the stylesheet's own `{{static}}` address.
- An address that only redirects (the My Company and Armory tabs without a company) answers a boosted click with the page and `HX-Push-Url` (`boostedLink`).
  Whole-page requests and history restores keep the redirect.
- A window that is closed on load fetches its body when first wanted, rather than with `hx-trigger="load"`.
  The To Wound chart does this with `tw-load`, triggered by `towound.js` from the pointer, touch, focus or click.
- A list of forms hidden until a click is a `<template x-if>` whose root calls `htmx.process($el)` in `x-init`.
- A page that Alpine filters at start is drawn in its default filtered state by CSS first, so nothing shifts when Alpine starts.

### Alpine

Alpine is used for state that never needs the server: menus, show/hide password, enabling a button as you type,
the theme store. Keep it to that.

Text that comes from a rules file or from users must never be written inside quotes in an Alpine expression (`x-data`, `:class`,
`x-show`): html/template escapes for HTML only and the browser decodes the entities before Alpine evaluates the text. Use the
`jsstr` template function (a JSON string literal) or a `data-*` attribute.

## App shell and navigation

Signed-in pages sit in a shell: a sticky top bar (menu button below `md`, name, appearance settings) and the sidebar
(`partials/sidebar.gohtml`) beside the content. From `md` up the sidebar is a fixed column; below that it is a drawer
that slides over the page (Alpine state `open`, closed by the backdrop, Esc or choosing a link). The sidebar lives
outside `#main`, so Alpine keeps `path` in step with htmx navigation to mark the current link (`aria-current="page"`).

Sidebar order: **Administration** (administrators and superusers, gold-outlined so it stands apart, a 404 for everybody else),
Campaigns (`/campaigns`, the list; `/` shows the same list when there is no valid last-campaign cookie, otherwise it redirects to the campaign last opened, see [auth.md](auth.md)) with a sub-menu of the user's active campaigns, Glossary; at the bottom the user's name, Account and Sign out. The sub-menu shows on the main page and inside any campaign; its items are drawn into every full page (`pageBase.base` asks `NavCampaigns` for a signed-in user, `NavInline`; the main page uses its own list, `NavOwn`), so there is no extra request. An htmx content swap has no sidebar and skips the query; if the list cannot be read the sidebar falls back to fetching `GET /nav/campaigns` on load. The list is refetched there only when it changes (`campaigns-changed`). Only the campaign you are in is expanded, with its pages (Overview, My Company, Armory, Rankings,
Matches; the current one marked) under it, and is marked with an accent bar, soft fill and a faint gold glow;
switching campaign collapses the previous one while the new one expands, both animating their height together (a
grid-rows 0fr/1fr transition with collapsed lists `inert`; an `x-transition` would let the leaving list linger and make the sidebar
jump) (Alpine `inCampaign` / `onTab` on the shell, from the current path). The shell also sets `path` when you click a link
(`navigate`), not only when the page has loaded, so the sidebar answers at once on a slow connection. Administration (administrators and
superusers, gold-marked) opens `/admin`, whose tabs (Users, Invitations, and Rules files for superusers only; the sidebar entry stays marked on all) are the
`admin_head` partial; the pages are described in [admin.md](admin.md). Signed-out pages (login) get only the top bar.

Every page ends with the fan-project disclaimer, the release the server was built from (`dev build` for a build from a checkout), the license and a link to the source (`partials/footer.gohtml`).

## Dice

The dice are 3D: `static/js/dice3d.js` (three.js draws them, cannon-es rolls them) loads the first time a roll needs it, so other
pages never pay for it; where WebGL or the file is missing, `static/js/dice.js` falls back to its flat 2D tray. **The number and the
faces of the dice come from the rules file** ([definitions.md](definitions.md)), never from a template or script: a roll panel carries
`data-sides` (faces of each die), `data-per-lane` (dice in one roll) and `data-lanes` (rolls side by side, a hero's results), and
`throwDice({count, sides, groups})` takes them. The 3D dice are cubes, so a roll of d6 uses them; a roll of any other size (d3, d8,
d12 ...) is drawn by the flat tray, a die with its number (a die of up to six faces shows pips, a bigger one its number). Typed
rolls are one select per die from `templates/partials/dice.gohtml` (`dice_selects`, fields `roll`, `roll2` ... read by
`enteredRolls`); a die with no physical form (a D3) is typed on a D6 (`core.TypedMax`, `core.FromTyped`). `window.diceMixin()`
(`throwDice`, `shake`, `onSpace`) is what the Alpine components use, with the same interface for both trays.

- **Built output.** dice3d.js is built from `tools/dice3d/src/index.js` (`make dice3d`, needs node and npm) with esbuild, which keeps
  only the parts of three.js and cannon-es the source imports (about 550 KB, 142 KB gzipped, against 240 KB gzipped for the
  libraries whole). The built file is committed; `node_modules` is not.
- **A roll the server made** ("Roll for me"): the throw is simulated first, without drawing; the faces of each die are relabelled
  so that the face that lands on top shows the server's number (opposite faces still add up to 7); the recording is then played
  back. The roll is animated to its value and nothing can change it, so it cannot be shaken. The panel keeps the result hidden until
  the dice stop.
- **A roll the browser decides** (the campaign setting **Shake the table**, "Throw the dice here"): the physics run live and the
  number is the face that ends up on top. The space bar shakes the table once while they roll (a second press does nothing, so nobody can shake until the dice show what they want): every die is woken, thrown up and spun
  again, and the tray jolts. The browser reports the result (`thrown`), so this is client authoritative and the backend refuses
  it when the setting is off.
- Where **Shake the table** is on, the **Roll** button of every roll panel (recruit, treatment, level-up) throws the dice in the browser (one die; a hero's level-up two pairs in two lanes) and sends the result (`thrown`); typing a roll instead (campaign setting) sends it as typed, and "Roll for me" in a campaign with both settings on still throws in the browser. Where it is off, Roll lets the server roll and the 3D dice only act it out. While dice roll, the space bar never scrolls the page.
- The new match form's scenario card is a `rollPanel` too (two dice, lanes Table and Scenario; `throwNewScenario` in the browser under Shake the table, the server's numbers acted out otherwise); it starts empty and rolls only when asked ([matches.md](matches.md#new-match)).
- **Paths.** A path's glossary term is shown as in the rulebook: its characteristic limits as a centred row of cells with the abbreviated attributes linked to their terms (a number of level-up increases is written `(+2)`, a total `9`, a best value `3+`), then the level-up chart as a table headed by the path's dice (each result linked to the rules it names). `core.PathTable` carries the data (`GlossaryTerm.Path`, `PathDice`); a path with neither limits nor chart (the end-of-path term On Their Own Path) is just its text, with no empty tables; the creation page's own-path note links the Path of Their Own Making when the campaign has that setting on and On Their Own Path otherwise (a test checks that every glossary link of the preview opens a non-empty article); the level-up lists paths by name, and the tooltip draws the same two tables (`data-tip-path`).
- **Lanes.** A hero's two rolls run in two lanes of the same table (taller than a single lane, so thrown dice do not keep hitting the divider), one pair each, named and totalled at the right edge.
- Typed rolls ("I rolled a N") are the campaign setting **Players may enter their own rolls** (off by default, set at creation,
  shown in the Overview): off, every roll panel only has a Roll button and the backend refuses a typed roll.
- Reduced motion: the dice lie in a row showing their numbers. The style guide has a demo.

## Adding a page

1. A template in `pages/` defining `title` and `content`, written mobile first with the tokens and components above.
2. A view type embedding `pageBase`, a handler that fills it, and a route wrapped in `protected` (or `session` when
   public).
3. The handler consumes a small interface; see [architecture.md](architecture.md) for developing it against a generated mock.
4. Check it at 360, 768 and 1280 px in light and dark, then compare with `/styleguide`.

See also: [architecture.md](architecture.md), [auth.md](auth.md), [vendored-js.md](vendored-js.md), [testing.md](testing.md).
