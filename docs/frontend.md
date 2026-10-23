# Frontend

Server-rendered Go templates, **htmx** for navigation and partial updates, **Alpine.js** for client-only UI state and
**Tailwind CSS v4** for styling. No JavaScript build step: htmx and Alpine are vendored in `internal/web/static/js`,
Tailwind runs as a standalone binary.

## Principles

1. **Fast to navigate.** The server renders HTML. Navigation and form submissions swap only the page content
   (see [Pages and htmx](#pages-and-htmx)). Anything that needs no data from the server is done in the browser with
   Alpine (disabling a button, toggling a menu) instead of a request.
2. **Responsive everywhere.** Every page works from a 360 px phone to a wide desktop; see
   [Responsive design](#responsive-design).
3. **Cheap to load.** Body text uses the system font. One self-hosted web font (headings), no framework, small CSS,
   embedded assets with cache headers.
4. **Accessible.** Colors pass WCAG AA, controls are keyboard reachable with a visible focus ring, form fields have
   labels, errors use `role="alert"`.

## Responsive design

Responsive behaviour is a requirement of every page, not a finishing touch.

- **Design mobile first**, starting at 360 px wide, then add `sm:` / `md:` / `lg:` variants for larger screens.
- **No horizontal page scroll at any width.** Use fluid containers (`max-w-*` with side gutters `px-4`), `flex-wrap`
  and `grid` with responsive columns. Avoid fixed widths in `px`. Wide content (tables) scrolls inside its own
  `overflow-x-auto` wrapper or becomes stacked cards on small screens.
- **Touch friendly.** Targets are at least 40 px tall on phones, with room between them.
- **The header collapses.** Below `sm:` the navigation links and Sign out move into a menu, leaving room for the brand.
- **Text wraps, never truncates** important content (names, rules). `main` has `overflow-wrap: anywhere`, so a 60-character name without spaces breaks instead of widening the page. The campaign tab bar scrolls sideways on a phone, with an edge fade where there is more and the current tab brought into view (`static/js/tabs.js`). Filter chips, steppers and `summary` rows are 44 px tall on phones. Small controls (the unit card corner tab, the item add chip) get an invisible larger touch area on coarse pointers. Headings are set for the narrow case and may break
  deliberately (the login heading breaks after "Battle Companies").
- **Background and images scale with the viewport** (`vmin`/`vw` units, `max-*` limits), never with fixed pixel sizes.
- **Check a page at 360, 768 and 1280 px** (and in both modes) before it is done.

## Theme

Colors are **semantic tokens**: CSS variables, exposed to Tailwind as utilities (`bg-page`, `text-fg`, `border-line`,
`bg-accent`…). Components use only these names, never raw colors, so a palette is one block of variables in
`tailwind.css`.

| Token | Use |
|---|---|
| `page` / `surface` / `raised` | Page background / cards, header, inputs / hover and secondary areas |
| `fg` / `muted` / `line` | Text / secondary text / borders |
| `accent` / `accent-fg` | Primary action background / text on it |
| `accent-text` | Accent-colored text and links on `page` or `surface` |
| `accent-soft` | Tinted backgrounds (selected state, badges, info alerts) |
| `danger`, `success` (+ `-soft`) | Errors and confirmations |
| `warn` | Draws (yellow); with `success` and `danger` it gives the `tone-win` / `tone-draw` / `tone-loss` classes for results |
| `gold` | Decoration; the ring background |

**Palettes** (each with a light and a dark variant): **Forest & Gold** (default), **Parchment & Ember** and **Mithril &
Night**. A palette is selected by `data-palette` and `data-mode` (`light` | `dark`) on the `<html>` element; the same
attributes on any element preview that palette there. `tools/check_theme_contrast.py` checks every palette and mode
against WCAG AA (4.5:1 for text, 3:1 for the accent).

**Mode** is System by default (follows the operating system), or Light / Dark. **Palette** and mode are chosen in the
settings menu (gear icon in the header) and kept in the `bcm-theme` cookie as `<palette>.<mode>`
(for example `forest.dark`). An inline script in the layout's `<head>` applies it before the first paint, so there is
no flash. `static/js/theme.js` holds the shared Alpine store (`$store.theme`). Signed-in users also have it stored in
their account ([auth.md](auth.md), "Appearance per account"): the layout puts `data-auth` and `data-pref` on `<html>`, the head
script and the store prefer `data-pref` to the cookie, `$store.theme.set` also calls `PUT /account/appearance` (silent on
failure), and sign-in saves the browser's cookie to an account that never chose, or writes the saved look to the cookie. The
Account page's Appearance card reuses the menu's `theme_controls` partial.

**Typography.** Headings use self-hosted *Cormorant Garamond* (600/700, `font-display: swap`) with lining numerals so
digits match the capitals. Body text uses the system UI font.

**Ring background.** `body::before` paints `static/img/ring-text.svg` (the One Ring inscription) centered on the
viewport in the palette's `gold` at low opacity, through a CSS mask. In landscape the whole ring fits the screen; in
portrait it is sized by the width so it shows around the cards. Cards and the header are opaque and cover it.

**Icons.** The favicon and app icons are traced from the inscription (`tools/build_icons.py`): `favicon.svg` (bold, for
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

`/styleguide` (served when `STYLEGUIDE=true`, which `make run`, `make dev` and `make demo` set; off by default) renders every token, a live contrast table, the type scale and all components in
the active theme. Use it to review any change to the design language.

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
- **Links to a glossary term** (`/campaigns/N/glossary#t-...`, from a tooltip chip or a "Glossary" link) carry `hx-boost="false"`: a boosted swap runs the glossary component before the address has its `#fragment` and scrolls to the top, so the term was never reached. A full load lets the browser jump and the component highlight the term.
- **Wargear names** are lower case in the rules file; the `upfirst` template function (`sentence`) gives them a capital wherever the company chooser shows them (the same helper as for messages).
- **Swapped content is wired at once.** htmx reads the `hx-*` attributes of a swapped-in fragment only when it settles (20 ms after the swap), and a click in that gap sends nothing (seen: a sheet menu entry that did nothing under load). `static/js/htmx_ready.js` calls `htmx.process` on the swap target in `htmx:afterSwap`, so the new buttons work from the moment they are on screen. The e2e step "a menu entry clicked the instant it is swapped in" holds it.
- **Double submit.** `static/js/submit.js` disables a form's buttons while its request is in flight and ignores a second submit. On the server, making a match or a campaign is deduplicated for 10 s (the same person, the same content opens the first one), and the actions that spend or roll (recruit, level-up, purchases, hires, treatment, perks, grants, invites, aftermath rolls and decisions) run behind `Server.once` (`internal/web/dedupe.go`): an identical request (same address and body) from the same person within 3 s gets the first answer instead of running again.
- **Refusals and failures** go through `Server.refuse` / `notFound` (`internal/web/errors.go`), keeping the status code: a full page request gets the `error` page in the layout (title, heading, the reason, a way back: the campaigns, or Sign in for a visitor), an htmx request gets the short plain text that the layout's error banner shows. A 5xx logs its cause and shows a fixed sentence. A catch-all route makes unknown addresses a rendered 404.
- **Validation errors** are returned with status `422` and swapped in place (htmx is configured to swap 422). Any other
  failure of an htmx request (403, 404, 500, no connection) is not swapped; the app shell shows it in a dismissible red
  alert under the header (`err` in the shell's Alpine state, filled from the plain-text body of a 403/404 or a generic message), so a click
  never does nothing silently.
- **Sign-in and sign-out** answer htmx requests with `HX-Redirect` (a full page load) so the header, which depends on
  who is signed in, is drawn again. A protected page requested without a session does the same toward `/login`.

**Windows** (`role="dialog" aria-modal="true"`, shown by `x-show`) are handled by `static/js/dialog.js`: focus moves into the window when it opens, Tab stays inside it and focus returns to the opener when it closes; the page closes it on Escape and the backdrop and locks the body scroll while it is open. Its close button is 44 px square. In running text (`p`, `li`, `td`, `dd`) links are underlined, so colour is not the only cue.

**Static files** (`internal/web/static.go`, [performance.md](performance.md)). Templates name them with `{{static "js/x.js"}}`, which adds `?v=<content hash>`;
a request with the current hash is `Cache-Control: public, max-age=31536000, immutable`, so a page costs no request for them. `app.css` is
served with the same `?v=` on the fonts and images it loads. Text files are gzipped at start and served with an ETag (304 on a match);
`STATIC_MAX_AGE` only governs addresses without `?v=`. Pages load three scripts: `theme-init.js` and `tz.js` (head, synchronous) and
`js/app.js`, a bundle made at start of htmx, the theme store, the page scripts, SortableJS and Alpine in the order of `bundleScripts` (a new
script goes into that list, not into the layout). Pages and fragments over 1 KB are gzipped for browsers that accept it (`compress.go`; the
live stream is not).

**Alpine** is used for state that never needs the server: menus, show/hide password, enabling a button as you type,
the theme store. Keep it to that.

Text that comes from a rules file or from users must never be written inside quotes in an Alpine expression (`x-data`, `:class`,
`x-show`): html/template escapes for HTML only and the browser decodes the entities before Alpine evaluates the text. Use the
`jsstr` template function (a JSON string literal) or a `data-*` attribute.

## App shell and navigation

Signed-in pages sit in a shell: a sticky top bar (menu button below `md`, name, appearance settings) and the sidebar
(`partials/sidebar.gohtml`) beside the content. From `md` up the sidebar is a fixed column; below that it is a drawer
that slides over the page (Alpine state `open`, closed by the backdrop, Esc or choosing a link). The sidebar lives
outside `#main`, so Alpine keeps `path` in step with htmx navigation to mark the current link (`aria-current="page"`).

Sidebar order: **Administration** (superusers only, gold-outlined so it stands apart, a 404 for everybody else),
Campaigns (`/`, the main page) with a sub-menu of the user's active campaigns, Glossary; at the bottom the user's name, Account and Sign out. The sub-menu shows on the main page and inside any campaign; its items are drawn into every full page (`pageBase.base` asks `NavCampaigns` for a signed-in user, `NavInline`; the main page uses its own list, `NavOwn`), so there is no extra request. An htmx content swap has no sidebar and skips the query; if the list cannot be read the sidebar falls back to fetching `GET /nav/campaigns` on load. The list is refetched there only when it changes (`campaigns-changed`). Only the campaign you are in is expanded, with its pages (Overview, Companies, Rankings,
Matches; the current one marked) under it, and is marked with an accent bar, soft fill and a faint gold glow;
switching campaign collapses the previous one while the new one expands, both animating their height together (a
grid-rows 0fr/1fr transition, collapsed lists `inert`; an `x-transition` made the leaving list linger and the sidebar
jump) (Alpine `inCampaign` / `onTab` on the shell, from the current path). The shell also sets `path` when you click a link
(`navigate`), not only when the page has loaded, so the sidebar answers at once on a slow connection. Administration (superusers
only, gold-marked) opens `/admin`, whose tabs (Users, Rules files; the sidebar entry stays marked on both) are the
`admin_head` partial; both pages are described in [admin.md](admin.md). Signed-out pages (login) get only the top bar.

Every page ends with the fan-project disclaimer (`partials/footer.gohtml`).

## Main page: campaigns

`/` lists the user's unfinished campaigns as cards (name, status badge, description, progress, the user's company in
it). "New campaign" (`/campaigns/new`) is shown to users who may create campaigns: admins, superusers, and users with
the `can_create_campaigns` flag (`User.MayCreateCampaigns`); the server answers 403 to anyone else. Roles are
`user | admin | superuser`, set with `make grant EMAIL=… ROLE=… [CREATE=1]`.

A "Show finished campaigns" switch adds the finished ones after the active ones (cards dimmed, sidebar entries greyed).
It is remembered in the browser cookie `bcm-finished` (a one-year preference, nothing is stored server side); the
handlers read it for both the page and the sidebar sub-menu, and the switch reloads both.

The campaign list has a search box that filters the cards by name in the browser (`listSearch` in
`static/js/lists.js`; accent and case insensitive, no request). It only sees what is on the page, so finished
campaigns are searched only while "Show finished campaigns" is on.

`web.campaigns` is implemented by the facade in `cmd/server/backend.go` over the core services and the PostgreSQL store (see
[architecture.md](architecture.md)). `make seed` fills a development database with demo accounts and a campaign.

## Campaign pages

`/campaigns/{id}` has seven tabs, each its own URL and page (the header and tabs are the `campaign_header` partial;
`campaignBase` carries the campaign and the current tab): **Overview** (your company or "create my company", campaign
facts, rules files, latest matches), **My Company** (your company: the page "Manage company" opens; your company's own
URL under `/companies/{id}` redirects there), **Armory**, **Companies** (everybody's; another company opens read only, yours redirects to My Company), **Rankings**, **Matches** and **Glossary** (the glossary this campaign is played with). The sidebar lists the
same tabs under the campaign you are in.

- **Manage campaign** card on the Overview (`CanManage`: the organiser and administrators; `campaign_manage.go`, template `manage_card`): the
  status badge; **Pause** / **Resume** with one inline confirmation; **End campaign** in three steps (consequences from
  `CampaignDetail.OpenMatches/PendingAftermaths/ActiveInvites`, the exact campaign name typed, a final "End the campaign for good"), posted to
  `POST /campaigns/{id}/status` (`to`, `confirm`; success redirects to the overview, a refusal re-renders the card with the reason, 422).
  A finished campaign shows "ended for good" to the organiser; administrators get **Reactivate**. The invite block shows "N links work now"
  with **Regenerate** and **Invalidate links** (inline confirm; `POST .../invites/regenerate`, `.../invites/invalidate`); while paused it hides
  "Invite players" and Regenerate. Paused: the Matches tab hides New match (`CampaignDetail.CanStartMatch`) and says why, `/matches/new` is 403, the
  join page says the campaign is paused (`InvitePreview.Paused`; a refused join is 422). Finished: the Matches tab says matches are frozen.
- A player has one company per campaign: `/campaigns/{id}/company/new` creates it (name, faction) and redirects to the
  company page when one already exists. A company page shows a management section only to its owner.
- **Rankings** sort by the total point cost of a company's units (`core.Rank`; equal points share a rank) and show
  wins/draws/losses, matches played and unit count.
- **Matches** are listed newest first with their sides (`A + B vs C`) and a result badge (Open, Draw, "<side> won").
  Names are coloured by outcome: the winning side green, losing sides red, every side yellow after a draw
  (`Match.Tone`). Wins/draws/losses are shown as green/yellow/red numbers wherever they appear (`wdl` partial).
- **New match** (`/campaigns/{id}/matches/new`, `static/js/match_form.js`): choose the number of sides (2 to 8), name
  each one (blank means "Side N") and search the campaign's companies to add to it; added companies are chips that can
  be removed, and a company already on a side no longer shows up in the searches (Enter in a search box adds the first match instead of submitting the form). **All vs all** turns the sides into one
  pool of companies (with an "Add every company" button) where each company fights alone: the handler turns each into a
  side named after it (`mode=ffa`). The form is posted as `sides`,
  `side-name-N` and one `side-N` per company; `core.ValidateSides` checks that every side has a company and none repeats.
  Any player of the campaign can create a match. A campaign with fewer than two companies offers no "New match" (the matches page says what is needed) and the form page says the same instead of showing the form; a finished campaign offers and accepts none.
- **Match page** (`/campaigns/{id}/matches/{match}`): header (result badge, date, "Ended <date>"), side
  cards (reported Victory Points, a "Fled" badge per company), **Scenario** card (empty state, a rulebook scenario with
  "How it is scored" collapsed, or a one-off with its description and victory text escaped, `whitespace-pre-line`; the folded Special rules print the rule
  tables of the file (`ScenarioText.Table`, template `rule_table`: Strange Aura by D6, the Tame the Beast Monster Budget with the row of the
  companies' combined rating marked) and the rules a scenario `usesRules` by name) and
  **Result** card (outcome as reported, note). The view carries the viewer's `core.MatchAccess` (`Match.Access`), which decides
  the edit controls (scenario picker and one-off form, result form with objectives, Propose to end and the End card, administrator reopen and history,
  organiser grants, the Aftermath card); see [matches.md](matches.md).

All of it runs on the real backend; `make seed` creates the demo campaign (see "Demo data" below).

## Rules files (vanilla and Battle Companies)

A campaign is played with a vanilla, a Battle Companies and a glossary rules file (`CampaignDetail.Rulesets`, shown in the
overview's Rules card with name, version and a short hash). The new-campaign form lets the organiser use the app's
default file or their own, per kind (`partials/ruleset_picker.gohtml`, `static/js/ruleset_picker.js`):

1. The browser hashes the chosen file: SHA-256 of the JSON with whitespace outside strings removed, the same value as
   `core.RulesetHash` on the server (checked byte for byte with the real definitions files).
2. It asks `GET /rulesets/{hash}?kind=&name=&version=`. If the server already has the file, the file input is cleared
   and only the hash goes with the form; otherwise the file is uploaded with its hash. The user sees the same "name · version" line either
   way (the difference is only logged with `console.debug`).
3. The handler (`rulesetChoice`) looks the hash up, or stores the upload, which the store must verify (hash and kind)
   before keeping it. Name and version come from `core.RulesetMeta` and are shown in both cases.

The form is `multipart/form-data` and not boosted. Rules files of every kind are identified by their **hash** alone (`GET /rulesets/{hash}` ignores `name` and `version`; `core.Rulesets.Find` keeps them in its signature only): two files with the same name and version coexist, and `name` and `version` are display metadata (the vanilla one reads `game` and `appVersion`, `core.RulesetMeta`). The old "edition exists" conflict (409) is gone from the core; the handlers and the picker's 409 branch are dead code for the frontend implementer to remove.

Wherever a file is shown (picker, overview's Rules card) it reads "name · version" followed by a short-hash label (the
first 8 characters, `RulesetInfo.ShortHash`), so two files with the same name and version can be told apart.

The store is `core.Rulesets` over PostgreSQL; at startup it registers the real files under `definitions/` as the defaults (so
the server must be started from the repository root).

## Adding a page

1. A template in `pages/` defining `title` and `content`, written mobile first with the tokens and components above.
2. A view type embedding `pageBase`, a handler that fills it, and a route wrapped in `protected` (or `session` when
   public).
3. The handler consumes a small interface; see [architecture.md](architecture.md) for developing it against a generated mock.
4. Check it at 360, 768 and 1280 px in light and dark, then compare with `/styleguide`.

## My Company: units

The My Company tab shows, in this order, a row of Points, Units (current / the company's maximum), Matches and W / D / L, a row
of Influence, Gold and the limits, (bows and crossbows, throwing weapons and
cavalry: a third of the company, always rounded up, `core.CompanyLimits`; `CompanyLimit.Next` is the maximum once one more model has joined, because a model that
joins counts in the numerator and the denominator: 6 models with 2 bows may take a 7th bow model, `RoomForNew`) and one **card per unit**, in the company's order
(`core.OrderUnits`: leader, sergeants, other heroes, regular units, unless the player saved another order).

A card, top to bottom:
- **Frame and insignia** for the unit's place, with no text tag: leader gold and doubled with a crown, sergeant doubled
  accent with a star, other heroes solid accent with three chevrons, regular units thin.
- **Kind icon** (top left), a filled silhouette: a knight in a great helm for infantry, a horse for cavalry (the same horse is the
  mount icon in the equipment list), a wolf for beasts, a horned dragon for monsters, an oliphaunt for monstrous cavalry
  (`core.KindOf` from the vanilla unit types). The earlier outline icons are kept in `static/img/archive/`.
- **Name** (units are nameable), **type** and **points** (plain text, no box).
- **Attributes** from the vanilla file's `stats` list (`VanillaBook.Stats/Profile`): Mv FV SV S D A W C I, then M W F for
  heroes that have any. Values the company raised are green, lowered red; hovering shows the full name. The backend
  fills them dynamically; the page just draws what it is given.
- **Buy buttons**: two small `+` chips (`eq-add eq-add-sm`, text size) open the armoury window filtered with `?kind=wargear`
  (right after the "Wargear" label, which has its own line, items below) or `?kind=equipment` (right after the "Equipment"
  label, before the slot pips). The window lists only that half, hides what a rule forbids (`ArmouryItem.Offered`), shows armour
  upgrades (`GearOffer.ArmourStage`) as rows with an "armour upgrade" badge and gives the reason for the others. A search field ("Find an item", name and aliases, creatures too) narrows the list in the browser. Read-only cards have no chips.
- **Effective stats**: `Attribute.Value` already includes the gear; a characteristic with `Attribute.Gear != 0` has an accent
  ring and its tooltip (`statTip`) lists the `CompanyUnit.StatMods` (`+1 from Shield`, `Shield: cancelled by a bow`). Rule
  chips and tooltips print `UnitRule.Shown()` (the label), never the key. A limit with no `RoomForNew` says a new recruit would not fit.
- **Equipment**, always sorted mount, melee, shield, armour, ranged, rest (`core.SortEquipment`); each class has its own
  icon and colour, and an item can be several things (a blade-spear shows a spear and a blade icon and counts for each). Starting items are plain, bought ones carry a `+` and an accent fill, items with stats show them
  (`24" · S2`) and items with rules have a dashed outline and a `?`, with a hover preview.
- **Path** (every hero, the leader and sergeants included): a box between the equipment and the special rules with the
  path's name, the heroic actions unlocked so far (the unit's own, the path's and those bought later; their text comes
  from the glossary term of the same name) and the special rules the path gave, and the magical powers it taught (the Path of the Sorcerer; a spell icon with
  casting value and range). Those rules are not repeated below
  (`CompanyUnit.Path`). Magical powers a unit knows from some other source stay among the special rules, after its own rules.
- **Special rules**, sorted unit, spells, equipment (`core.SortRules`); spells are highlighted with a star,
  their casting value and range; every rule previews its description on hover.
- **Level and experience**: Lvl and a bar of `XPInLevel / 5` at the bottom. A unit with promotions waiting gets a gold
  **"Level Up ready!" banner across the top of its card** (a slow shine, a Roll chip; it is the level-up button). A unit gains a
  level only when its roll is made, so it can have several rolls pending (15 XP at level 0 is "×3",
  `PendingLevelUps`). The banner opens the level-up panel (roll, re-roll, choose; `levelup.go`).

**Item icons.** Every item has an icon: specific ones by name (backpack, badge, rope, drum, horn, standard, cloak, brew, herbs,
talisman, map, boots, ring, scroll, seeing stone, torch, snares, pelt, cram, belt, vial, blowpipe, axe, barding;
`core.IconFor`), the class icons for weapons, armour, shields, bows and mounts (a blade-spear shows two), and a generic
**pouch (small)** or **chest (large)** for equipment the rules data adds that has no icon yet. The icons are in
`partials/units.gohtml` (`item_icon`, `eq_icon`, `gear_icons`); add a name to `iconRules` and a drawing to give a new item its own.

**What the cards read from the data.** An item's classes, granted rules and missile profile come from the Battle Companies
file's `data.gearAnnotations` (`BCBook.Annotation`), not from guessing its name. The classes are mount, melee, two_handed,
spear, lance (a cavalry spear, sorted with the melee weapons), shield, armour, bow, crossbow, throwing, missile (the
blowpipe: a ranged weapon that counts against no limit), banner, instrument, consumable (scrolls, herbs, brew, cram) and
other, each with its icon and colour; an item that is several things leads with its first class and shows the others as small
secondary markers. Granted rules are the item's rules, or the mount's when the annotation says `grantedTo: mount` (Pony:
Timid); Barding is the steed's armour, sorted right after the mount, and its rules go to the rider. Envenom Weapon is not an
item but a modifier: the card shows a copy of the weapon it was bought for with Poisoned Attacks (X). Named unit upgrades
(Helmingas, Hearthguard...) are badges, not gear. Limits are counted by class (bow and crossbow, throwing, the Cavalry
keyword) with a company's `limitExemptions` (an exempt unit is left out of the count and of the base the maximum is worked
out on) and `limitOverrides` (`BCBook.Limits`).

**The Armory tab** lists every item the company's armoury sells, whether or not a unit can take it now, with its cost, slot
size, text and what it gives. **Buy for…** opens the company's units: those that can take it have a Buy button, the others
are shadowed with the reason (heroes only, cavalry only, no free slot, a limit full, not enough influence...). An item
nobody can take says so and still shows who is refused and why. A search box filters by name and alias.

Filters (Alpine `armory`, in the browser): a switch **Hide what <company> cannot take** (ON by default, remembered in `localStorage`
`bcm-armory-mine`, styled like the main page's "Show finished campaigns" switch) hides the items every unit of the company is ruled
out of (`UnitOption.Blocker == BlockedByRule` for all: another company's goat or Fell Warg) and the wanderers the company cannot take;
lacking influence or a slot does not hide anything. Main category chips (All, Wargear, Equipment, Creatures, Wanderers) and, under
them, the item category chips (Melee, Ranged, Armour and shields, Mounts, Consumables, Banners and horns, Other: the item's first class,
`subOf`); picking a main category disables the item categories it has no items in (`data-applies`), so Creatures and Wanderers have none.
The names on creature and wanderer cards (special rules, wargear, heroic actions) carry a tooltip when a file explains them: the armory's
own item text (aliases too) or the glossary term; a name nothing explains has none.

**Equipment slots.** Battle Companies equipment takes a slot: a model carries one large and one small item, and a Backpack
(large) holds three more small ones (`CompanyUnit.Slots`, `GearOffer.Size`). Wargear (weapons, armour, mounts) takes none. A
card shows its slots as pips (`Large ■ 1/1`, `Small □ 0/1`), and a **+** after its equipment opens a window with what the
company's armoury sells (`Armoury`: the common wargear and equipment plus the company's unique wargear), each item with
its influence cost and slot size. **Only what the unit could take is listed**: an item a rule forbids it is left out
(already carried, heroes only, cavalry only, evil only, and, for wargear, an option its
profile does not have: a warrior may only buy wargear matching one of its profile's options, a hero any wargear of the
profiles in its company's charts, promotions and starting company (`heroOptions`); a Beast takes no wargear and only equipment that grants exactly one special rule (Trophy Pelt, Badge of Courage), `beastMayCarry`; a profile without options, such as the Guard of the Fountain Court, buys no wargear, a Mattock is not the Two-handed weapon option). An item that only lacks influence, a free slot or room under a company limit (bows, throwing weapons) is listed with that
reason (`ArmouryItem.Blocker`): a full limit is not a rule against the unit.

**Armour upgrades** (`core/armour.go`, `data.wargear.armourUpgrade`): the shop adds one offer per armour stage above the first
(`GearOffer.ArmourStage` > 0: "Armour", "Heavy Armour"; 1 influence or Gold, rating per stage: 1 until a hero's Attacks + Wounds reach 3, then 5, and none for a stage that its profile or its Options already have, BC "Calculate Rating"; a warrior pays its option's points). Buying
one through `BuyEquipment` with the stage's name replaces the armour the unit wears (the old one is dropped) with the next stage;
stages are never skipped. A warrior (a Beast that is still a warrior included) needs the stage as an option of its profile; a hero only the stages that units
of its company can have (`armourPool`), so a company without heavy-armoured units never sells Heavy Armour to a hero.
`ArmourUpgrade.UnitStage` is the stage a unit wears. Frontend: the offers appear in `Armoury` and `ArmouryEntries` like any item
(their `Reason`/`Blocker` say why not); the Defence change comes with the effective stats (pib.2).

**Dropping.** The expanded card has a **Drop** button on equipment the rules let the unit drop (`core.DropReason`);
otherwise it says why. Battle Companies: equipment (slot items) can always be dropped; a hero freely drops any wargear it
chose or bought, but wargear of its profile only with the optional rule "heroes may discard any wargear"
(`heroesMayDropProfileGear`, off in the demo; a campaign setting later); a warrior may drop wargear only if what it then
carries is another option of its profile (a shield alone, yes; a spear alone, no) and never its profile's wargear.
Nothing is refunded, but the item's rating leaves the unit's points (a unit's points are its vanilla points plus the rating
of what it chose or bought: warrior or hero column), and the slot and company limit it used are freed, so another unit can
buy the limited item. Buying spends the company's
influence and may grant special rules (`grantsSpecialRules` in the Battle Companies file). Every item the company can buy is in the Armory tab.

**Recruiting.** The **Recruit** button above the units opens a window that stays open through the whole recruit:
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

**Ranger of the North replacement (Shire; epic bead `6n1`; first design built by `7rf`, `kbk`, `04t`, `3b2`, now being reworked into the officer reshuffle below).** The Shire starts
with a Ranger of the North as Leader (fixed: `CompanyProfile.ValidateStart` refuses any other choice with "The Ranger of the North must start as the Leader of the The Shire company."; the form should lock him), a vanilla Hero (Might, Will, Fate 1 each, so `startingHeroNoStatGain`). The company's rule:
"if the company's Ranger of the North has been slain, you may swap any successful result on the Reinforcements chart for a Ranger
of the North, who may immediately take the role of Leader or Sergeant. The company may only ever have a single Ranger of the
North at any time." The data is the effect `companies[Shire].effects[kind=rangerReplacement]`: `unit`, `max` (1, counted in the
roster), `role` (`leader`, `sergeant`), `when` (text). Rules answered from the file and the user's decisions:
- *Condition* (`RangerOffer`): the company has the rule, the roster holds fewer than `max` of `unit`, and the cemetery holds one
  (`FallenUnit.Unit.Type == unit`, however it fell: Dead or a hero forced to retire). REVIEW: "slain" includes a retirement.
- *Which results*: any successful row of the chart, so not row 1 "Nobody steps forward" and not a special chart (the Shire has none).
  The whole result is swapped for one Ranger, also a two-model result ("2 Hobbit Militia") or a choice ("Choose any 2 Hobbits").
  REVIEW: one Ranger replaces the whole result. The swap is free; the roll's price and any raise are paid as for any roll. The swap
  is chosen at the confirm of the rolled result, before the company checks whether the result can join, so a result that could not
  join (over the model maximum, the Shirriff's Rare 12) no longer sends the player to the lower-result panel when the Ranger can
  join. A lower result chosen afterwards in that panel is not swapped. REVIEW.
- *The recruit*: a Hero (no Might, Will, Fate gain), vanilla wargear only (the starting armour is a starting company item, not
  buyable). REVIEW. It joins with no path and chooses it in the usual path window (`PathPending`; the company cannot be put in a
  match until it has). A direct Hero, not a Hero in the Making: no promotion roll.
- *Role: Leader to start with, any officer place after* (user decision; the rules text says "Leader or Sergeant"). The panel
  pre-places the Ranger as Leader, and the player may move him to a Sergeant place (a Sergeant, or the old Leader, then leads). He
  never leaves the officer places: his card has no remove button and he cannot be the one dropped. Undoing an officer's promotion is not modelled, so instead the officers are
  **reshuffled**: the old Leader and the two Sergeants (the *other officers*, all active Heroes) are arranged in the two Sergeant
  places, and the player decides which of the three stops being an officer. That unit becomes an ordinary Hero (`RoleHero`) and
  keeps everything else: path, level, XP, gear, injuries. The player gets the same officer panel as "Take command" when founding
  the company (drag and drop between slots, or the Sergeant/Remove buttons of each card), with the Ranger's card pre-placed in the
  Leader slot, draggable between the three officer places but with no remove button and not accepted by the pool, the old officers
  in the other places, and the dropped one in the pool of "Heroes who are not officers".
- *Where the panel appears: a pending step the roll leaves behind, finished in the recruit window.* Picking the swap at confirm pays
  the roll and the raise, validates that the Ranger can join (`block`), and parks the recruit as `Recruit.Officers` (like
  `Recruit.Choice`): the window turns into the officer panel and stays open on a reload; it blocks other recruits and the actions
  the aftermath lock blocks, and `CreateMatch` refuses ("Arrange the officers first"). My Company also shows a banner "Your
  company is waiting for its new Leader: arrange the officers" that opens the panel. Nothing changes in the roster until the player
  presses the panel's confirm button (**Appoint**); there is no cancel (the influence is spent and the swap chosen), only
  **Appoint**, because a company with a pending arrangement is otherwise unusable. REVIEW: pending state rather than a field in
  the confirm form, to keep the confirm form light on a phone and the arrangement one atomic step.
- *Fewer than 3 other officers*: the officers of a company are never empty (an officer's death is replaced from the active units),
  so normally there are exactly three and one is dropped. If there are fewer (older data, k = 1 or 2), nobody is dropped: the
  Sergeant places the old officers do not fill stay empty (the pool offers the other active Heroes as an optional choice for an
  empty place, `Candidates`) and the usual officer succession of the aftermath fills them. If k = 0 the Ranger is the lone
  Leader (or stays where the player put him). REVIEW.
- *Aftermath and live match*: nothing changes there except that a fallen Ranger shows the hint below. A Ranger dying is an ordinary
  casualty (a Leader falling still opens the succession of [aftermath.md](aftermath.md)). A recruit is not possible while the
  aftermath is open (company lock).

*Flow (360 px first).* Recruit window, rolled and successful result, `RecruitRoll.Ranger != nil`: next to the confirm button a
radio pair "Take X as rolled" (default) / "Swap for the Ranger of the North (free; he becomes your Leader)". Choosing the swap
changes the button to "Confirm and arrange officers". After the confirm the window shows the **officer panel**: heading "The Ranger
of the North takes command", the Leader slot (the Ranger pre-placed, movable to a Sergeant slot, never to the pool), two Sergeant slots, the pool of the one who steps down ("Not an
officer"), a live line "Name stops being an officer and stays a Hero", **Appoint** (disabled until the arrangement is valid, with
the reason under it). The panel is a sibling of the "Take command" one: the `officers_panel` / `officer_card` partials of `company_panels.gohtml` and the Alpine/Sortable `officers` component of `static/js/lists.js` (same `.officer-slot` / `.start-card` styles). The Ranger's card is marked `data-keep`: draggable between the three officer places, no remove button, refused by the pool; a card dropped on an occupied place swaps with the one there. The buttons of every card (Leader, Sergeant, Not an officer) are the keyboard path. Cards of old officers carry `data-officer`, and Appoint stays disabled until exactly `Drop` of them are in the pool and a Leader is set. Empty/not offered state: nothing is shown. Errors: `ErrRefused`
text in the notice. Permissions: the owner only, like every recruit.

*Contract* (consumer interface `internal/web`, implemented by `core.RecruitService`):
- `RecruitRoll.Ranger *RangerOffer` becomes `RangerOffer{Unit string}` (nil when not offered; filled only for `Stage == "chart"` and a
  successful rolled row). The role fields (`Roles`, `Holders`, `Full`, `Offers`) and `roleSlots` go away.
- `ConfirmRecruit(ctx, user, campaignID, adjust, pick)` takes `pick == "ranger"` (no role, no unit id). It swaps the result, pays,
  checks `block`, stores `Recruit.Officers = &OfficerStep{Unit, Paid, Roll}` and returns `RecruitResult{Summary, Officers *OfficerPanel}`.
  Refusals (`ErrRefused`): no offer now, not a chart result, the Ranger cannot join (limit text), `rangerPick` of the old
  `ranger:<role>[:<id>]` form is no longer accepted.
- `OfficerPanel{Unit string; Leader OfficerCard; Officers []OfficerCard; Candidates []OfficerCard; Drop int}`: `OfficerCard{ID int64; Title string;
  Role UnitRole; Type string}` (`Distinguish` titles; the Ranger is `Leader` with ID 0). `Officers` are the other officers (Leader and
  Sergeants, active Heroes), `Candidates` the non-officer active Heroes (only used when `len(Officers) < 3`), `Drop` how many
  of `Officers` must stop being officers (`max(0, len(Officers)-2)`).
- New `RecruitService.PendingOfficers(ctx, user, campaignID) (*OfficerPanel, error)`: nil when nothing waits.
- New `RecruitService.ArrangeOfficers(ctx, user, campaignID, a OfficerArrangement) (RecruitResult, error)`,
  `OfficerArrangement{Leader int64; Sergeants []int64}`: the unit ids for the Leader place and the Sergeant places (at most 2); the
  new Ranger is id `0` (`RangerID`), and the panel pre-places him as `Leader`. Validation, all `ErrRefused` and with nothing stored
  on error: an arrangement waits; ids distinct; each an id of `Officers` or `Candidates`, or `0`; `0` appears exactly once, in
  `Leader` or in `Sergeants` (the Ranger is in exactly one officer place and is never dropped); `Leader` is set (so exactly one
  Leader); `len(Sergeants) <= 2`; the units of `Officers` not named number exactly `Drop` (one of the three non-Ranger officers
  when there are three); with fewer than 3 other officers nobody is dropped and the free places are left empty or filled from
  `Candidates`. Effect: the Ranger joins (`UnitState{Base: Unit, Role: <the place he was put in>}`, no Bonus, vanilla wargear, path
  pending); each named unit takes its place's role (so a Sergeant may become Leader, and the Ranger a Sergeant); the units left out
  become `RoleHero` keeping path and progress; `Recruit.Officers = nil`; summary "The Ranger of the North takes command as Leader.
  X leads; Y and Z are Sergeants; W is no longer an officer." Formation order stays valid.
- `CreateMatch` and the other recruit actions refuse while `Recruit.Officers != nil`. `CompanyDetail.OfficersPending bool` drives the My Company banner.
  Persistence: `Recruit` is part of the company state; if it is stored as JSON no migration is needed (database-master to confirm).

*Backend built (kmn, eye; `internal/core/recruiting_ranger.go`).* As specified above, with these details for `d1z`: `RangerOffer{Unit}` only;
`ConfirmRecruit(..., "ranger")` returns `RecruitResult.Officers` (the `OfficerPanel`) and stores `RecruitProgress.Officers`
(`OfficerStep{Unit, Paid, Roll}`, part of the company's pending JSON, so **no migration**; the Postgres round trip is tested);
`PendingOfficers` / `ArrangeOfficers` are on `core.RecruitService` (add them to the web `campaigns` interface and regenerate its
mock); `Locked()` refuses with "Arrange the officers first: the Ranger of the North is waiting to take command." for every action the aftermath lock
blocks, and `CreateMatch` with "<company> cannot play yet: arrange the officers first."; `CompanyDetail.OfficersPending`. Panel:
`Officers` are sorted Leader first; `Drop` = `max(0, len(Officers)-2)`; the Ranger's card is `ID 0`, `Role Leader`. The arrangement posts
`Leader` (0 = the Ranger) and `Sergeants`. Refusal texts (`ErrRefused`): "No Ranger of the North is waiting to take command.", "A company has
at most 2 Sergeants.", "That unit cannot be appointed.", "<name> is named twice.", "The Ranger of the North takes command: he cannot be left out.",
"Exactly N of the other officers keep their place; the rest stop being officers." (three officers) / "The other officers keep their places."
(fewer). The summary is "Officers: A as Leader, B as Sergeant, ...; X is no longer an officer." **As built (`d1z`):** the `#ranger-offer` fieldset of the
recruit window is one pair of radios (as rolled / `pick=ranger`; choosing the swap adds "and arrange the officers" to the Confirm button);
the confirm answers with the officer panel in the window; `GET .../my-company/recruit` shows it whenever `PendingOfficers` is non-nil (reload resumes);
`POST .../my-company/recruit/officers` (`leader`, repeated `sergeant`; 0 is the Ranger) calls `ArrangeOfficers` and answers with the whole
page (a refusal re-renders the panel with the reason, status 422); My Company shows the banner `#officers-banner` while `CompanyDetail.OfficersPending`
and opens the window by itself on the panel.
The old pick `ranger:<role>[:<id>]` is gone.

*What had to change in what was first built (commit 117ed16, `04t`, `3b2`).* Backend (`internal/core/recruiting_ranger.go`,
`recruiting.go`, `company.go`): drop the role/holder/full machinery and `rangerPick`'s role and displaced id, rewrite `joinRanger`
into the pending step plus `ArrangeOfficers`, `RangerReplacement.Roles` stays parsed but the offer exists only when it contains
`leader`; the tests of the displaced-holder case are replaced. Web: the `#ranger-offer` fieldset loses the role radios and the
"who steps down" select (one pair of radios and a hidden `pick=ranger`), gains the officer panel step and the My Company banner. The
cemetery tag (`RangerMayReturn`, "May return with a reinforcement") and the aftermath alert (`RangerFell`) stay as built; their
text says the Ranger "returns as your Leader".

*Acceptance.* (1) Offer absent without the rule, without a fallen Ranger, with a Ranger in the roster, on row 1. (2) Swap is free
(influence only drops by the roll and raise). (3) A result that cannot join is swappable and then the lower panel does not open.
(4) After Appoint: exactly one Leader and two Sergeants among active Heroes, the Ranger in exactly one of the three places, one old officer a plain Hero with path,
level, XP, gear and injuries intact; invalid arrangements (Ranger absent, named twice or dropped, a non-officer as Sergeant while three officers exist, a
duplicate id, the wrong number dropped, no pending step) are refused and store nothing. (5) The Ranger has no Might/Will/Fate bonus,
a pending path, and blocks `CreateMatch` until chosen; `CreateMatch` also refuses while the arrangement is pending. (6) A pending
arrangement survives a reload, blocks other recruits, and the banner on My Company opens it. (7) Tests: the Ranger kept as Leader; the Ranger moved to a Sergeant place with the old Leader or a Sergeant as Leader; a Sergeant as Leader with the old Leader dropped. (8) With fewer than 3 other officers
nobody is dropped and the empty Sergeant places remain. (9) The panel reuses the Take command component and its keyboard
alternatives; the Ranger card can move between officer places but cannot be removed; 360/768/1280 px, light and dark.

**Rare units.** A chart row or special unit marked rare (`Rare N` badge in the recruit tables) may be had N times in the company at
once (the units of that type in the roster). A rare result the company already has the most of cannot join: the player takes a
lower result of the same chart (or cannot buy the special unit), like any other result that cannot join. A result that offers
several units (`unitChoice`, the Wanderers' Hobbits: Hobbit Militia, Hobbit Shirriff, Tookish Hunter, Battlin' Brandybuck) lets the
player pick one the company does not have yet (each is rare 1 as a profile); with all four it cannot join.

**Campaign settings.** The options an organiser can switch on when creating a campaign (`heroesDropProfileGear`, `shakeTheTable`,
`manualRolls`) are listed in the Battle Companies file (`campaign.options[]`: key, scope, label, description, default,
implemented); the form is drawn from that list (`core.ParseCampaignOptions`, grouped by scope: rules, app). An option whose key the
app does not know, or that the file marks as not implemented (the homebrew path), is listed struck through and disabled with an "unknown setting" tag (its tooltip says the app may need updating or the rule in the file may be wrong), and logged as a warning.

**The cemetery.** A unit that dies (a casualty roll of Dead, a hero retiring on its second arm or leg wound) leaves the roster for
good and rests in the **Cemetery** section under the Battle company, as it was when it fell (`core.FallenUnit`: its profile, wargear,
path and level, how it ended and where). The cards only look. The player can reorder them (**Change order**, with drag and drop or
arrows, saved with `SaveCemeteryOrder`), but a fallen unit never goes back to the roster and a unit of the company cannot be put
in the cemetery by hand (the two lists are separate Sortable instances, and each save refuses ids of the other list). The demo
company has two fallen units; the real backend fills the cemetery from the casualty rolls of a match (see its bead).

**Units made of several models.** The Vault Warden Team joins as two units of their own (vanilla `includes`, points from the
company's `includedUnitPoints`: Iron Shield Bearer 13, Foe Spear Wielder 12), each with its own card, experience and injuries. A
`compositeModel` (the Warg Marauder: three Goblin riders and a Warg on one Cavalry card) is one unit that counts as `Slots` models
against the company's maximum (`CompanyUnit.Models`; `CompanyDetail.ModelCount`) and shows its parts on the card; it counts once for
the Bow and Cavalry limits. The company's `unitTypes` change what a unit is (Infantry to Cavalry). Units **On Their Own Path** (the
company's `onTheirOwnPath`, matched by exact name) start on the path "On Their Own Path": no experience, no level-ups, the rule At Path's End; the path is not selectable (`selectable: false`), so a hero's path choice never offers it. With the campaign option **customOnTheirOwnPath** (off by default, `CampaignSettings.CustomOnTheirOwnPath`) these units
instead start as heroes on the homebrew "Path of Their Own Making" (0 Might, 0 Will, 1 Fate, a 2D6 chart in the file, earn experience and
level up); the path is hidden from the player's path choices like the empty one (`ProgressionRules.Hidden`). Unit names in the file are exact vanilla names (or a
company's label for a unit, like the Goblin Hulk), so they are matched exactly. A `heroPromotion` (Arnor) makes the unit change profile
and become a hero in one step, with no extra Might, Will and Fate.

**Formation.** The order of the units comes from the backend: the default (leader, sergeants, other heroes, regular units) until
the player orders their company, and their saved order for good after that (`core.OrderUnits`; units they have not placed,
such as new recruits, follow at the end in the default order). **Change formation**, next to the unit count, puts a handle above
every card: drag it (SortableJS, `static/js/sortable.min.js`, MIT, with its pointer-based drag so it feels the same with a
mouse and on touch screens) or use the arrows on the handle (keyboard friendly). **Save formation** sends the ids in their
new order (`SaveFormation`, which refuses an order that is empty, repeats a unit, misses one or names a unit of another company) and the page comes back in that order; **Cancel** (or Escape, when nothing else is open) puts the
cards back. Meanwhile the cards themselves do not react.

**Rating and injuries.** A unit's points are its Battle Companies rating: a warrior is its MESBG points plus the option its
wargear amounts to plus the rating of its equipment; a hero built from a warrior profile is that profile's points plus
5 per point of Might, Will, Fate, Movement, Fight, Strength and Defence and 10 per point of Attack and Wounds above the
profile and above the average Battle Company profile, plus 5 per special rule its path gave (`BCBook.HeroRating`), plus
the rating of gear that is neither part of its profile nor of its options (at the hero rating when its Attacks and Wounds
add up to 3 or more); other heroes (named characters, monsters) are their vanilla points plus gear. **Injured** units
(marked on their card) miss the next game and are left out of the company's rating (the Points card says how many).

**Optional rules** are chosen when the campaign is created and shown in its Rules card. For now there is one, "heroes may
discard any wargear" (`CampaignSettings.HeroesDropProfileGear`).

The **expand** button (a small tab flush in the card's corner frame, so it takes no room from the content) maximises one card (only one at a time; click outside, press Esc or the close button to return): it
shows every item and rule in full, with the glossary terms inside the descriptions linked and previewed on hover
(`core.Glossary.Link`, the campaign's glossary). Static behaviour is in `static/js/company.js`.

Company pages are built by `core.BuildDetail` from the stored company state and the campaign's rules files.

### Healing and injuries

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

### Level-up

The "Level Up ready!" banner of a card opens the level-up window (`level_panel`). The rules are read from the Battle
Companies file (`core.ParseProgression`: `campaign.progression`, `data.paths`, `campaign.characteristicLimits`, each
company's `advancements`); a unit earns a roll every 5 XP and several can wait.

- A **warrior** rolls a D6: 1-2 nothing, 3-5 a promotion (the player picks one of the company's advancements that match
  the unit; with none, Courage and Intelligence +1 up to 3+), 6 a hero in the making (pick a path, +1 Might, Will and Fate,
  or forgo it and take the promotion).
- A **hero** rolls two pairs of 2D6 on its path chart. A result it cannot take (stat at the path's or the overall
  maximum, rule or action already owned, minor-rule or heroic-action total reached) is re-rolled by the app, with a note.
  The player picks one of the results and, where a row has alternatives or a list (stat of a pair, a minor rule, a heroic
  action), the concrete choice. A result marked "take the other roll too" asks for the other result in the same window: choosing it turns the other roll's choices into a second, required pick, and one Choose applies both. When rolls remain, "Next level up" (the card's banner effect) opens the next.
- The choices that give a rule, a minor rule, a heroic action, a power or a path show the glossary's description under their name, with links to other terms that preview on hover; the whole choice also has a tooltip with the full text.
- "Roll for me" is animated (one die for a warrior; a hero's four dice run in two lanes, one per roll, named and totalled once they stop); "I rolled" takes the player's own dice. A roll
  waits for its choice if the window is closed (`campaigns.LevelUpState`).
- **Magical powers** (Path of the Sorcerer only): the rows offer "a new power or improve a casting value". The list is the
  Sorcerer's spell list (`data.magicalPowers.items[learnable]`, each with range and casting value) minus the powers the hero
  knows; an improvement lowers a power's casting value by 1 (5+ becomes 4+), at most twice per power and never below 2+
  (`castingRule`). A new Sorcerer (Hero in the Making on that path) also picks its first power. Fury is learned for a species (one choice per "Fury (Species)"), and a path with a company limit (the Sorcerer: 1) is not offered once the company has that many heroes on it. The backend must provide the
  eligible list and the improvements; the app only renders the choice (a collapsible list with each power's casting and
  range, its definition on hover).
- **Minor rules with a target.** Poisoned Attacks asks which weapon (one choice per weapon that is not poisoned yet, by a rule, a
  level-up or Envenom Weapon) and Combat Synergy which other hero of the company (the Battle Companies wording: "this Hero and X").
  The choice is stored with the target (`Progress.Minor` entries such as `Poisoned Attacks|weapon:Bow` and `Combat Synergy|unit:7`);
  the card shows the rule as "Poisoned Attacks (Bow)" with the weapon marked "· poisoned", and "Combat Synergy - Hador". A unit target
  is kept by id, so renaming the unit renames the rule. The parameter kinds come from `minorSpecialRules[].parameterKind` and the
  wording from `parameterisedRules` in the rules file.
- A hero that takes a path gets its starting heroic action at once (`paths[].startingHeroicAction`).
- If only one of the two results can be taken, the player must take it; the unavailable ones are re-rolled only when neither
  can be taken.
- **Against the Odds** (mock: the company has one re-roll, shown on My Company): while a roll waits for its choice, each result
  has a "Re-roll · Against the Odds" button that rolls that result again (a warrior's D6, or a hero's pair).
- Company Hero Upgrades are not modelled. A new hero's remaining waiting rolls roll on its path at once (the rules say "from
  the next game on"; the app has no game step yet).

Companies' points are shown as `current (full)`: what counts now, with injured units left out, and in brackets what they
would be with nobody injured (left out when equal; `CampaignCompany.FullPoints`, template `points`).

### Choosing a company

The page to create a company (`/campaigns/{id}/company/new`) is a browser of every company of the campaign's Battle Companies
file (`CompanyProfiles`, `core.CompanyProfile`): a searchable list with an alignment filter on the left, and the chosen company's page
on the right (loaded as a fragment, `company/info?c=NAME`): history, facts (models, reinforcement cost, starting gold, break point),
starting company, reinforcement and special charts, special units, the promotion table, what only the company has (exclusive wargear,
hero upgrades, special rules, free rules, own-name units, exchanges) and its limits and notes. Every unit named (in the starting
company, charts, special units and promotions) shows its card on hover or focus (`core.UnitPreview`, drawn from the vanilla profiles;
a company's own label for a unit is resolved to the profile it is); items and rules show their text. The creation form is part of the
page. The page ends with **Take command**: three places (a leader with a crown, two sergeants with a star) and, below them, the
starting models. The player drags three models into the places (SortableJS; a model dropped on a taken place sends the one that was
there back to the pool) or uses the Leader, Sergeant and Remove buttons of a card, which also work with a keyboard. Models the
company does not let lead (`startingHeroRestriction`: Lake-town only the Lake-town Guard, Cirith Ungol not the Spider) cannot be
placed. The company's own suggestion (the roles its file gives, else the first models) starts in the places, marked "Suggested"
until the player changes anything. Below, the company's **oath** (`data.companies[].oath`: text with `{name}` where the player types
the company's name, and the label of the vow button; companies of a custom file fall back to `campaign.company.oath` for their
alignment) and the vow button, which opens a "These decisions are final" confirmation with a recap. The officers and the company are
final; the name can be changed later from My Company (the pencil by its name, `RenameCompany`). `CompanyProfile.ValidateStart`
checks one leader, two sergeants, distinct models that may lead. A hero starts with a point of Might, Will and Fate, except the
units the company lists in `startingHeroNoStatGain` (Ranger of the North in Helm's Deep and the Shire).

**Choosing a path later.** The leader and the sergeants start as heroes without a path (`CompanyUnit.PathPending`): their cards
carry a "Choose a path" banner that opens the same kind of window as a hero in the making (`PathOptions`, `ChoosePath`: paths that
are at their limit are listed with the reason, and a Sorcerer chooses its first magical power). The path's heroic action comes with
it. A pending officer cannot level up and the company cannot be put in a match until every hero has a path (`CreateMatch` refuses). A hero starts with a point of Might, Will and Fate and its
path's heroic action, except the units the company lists in `startingHeroNoStatGain` (Ranger of the North in Helm's Deep and the
Shire).

### Company rules and hero upgrades

- A **hero upgrade** of the company is offered with every hero level-up as a third option, in a gold frame ("Company hero upgrade"): a
  hero may take it instead of either result, even when neither result can be taken, if it does not have it yet (the Battle Companies
  rule). It appears on the hero as a company-exclusive rule (gold frame, tooltip "Company-exclusive hero upgrade").
- **Applied now** (`applyCompanyRules`, `buildUnit`): the maximum number of models, limit overrides and exemptions, free special rules
  some models get (Rohan's heroes: shown as company-exclusive rules), units with their own name (resolved to the vanilla profile, with
  the rules the company adds), exclusive wargear (marked company-exclusive in the armory, with its text), and the share of the company a
  keyword may have (a limit bar, and enforced: a reinforcement, special unit or wanderer of that keyword is refused over the limit; a
  reinforcement roll that lands on it spends the influence and nothing joins).
- **Race balance** (Defenders of the North: Dwarves not more than Dale): a reinforcement that would outnumber is replaced by the highest
  lower result that does not (the rule lets the player choose any lower result; the demo picks for them and says so).
- **Hero upgrade restrictions** (Fiefdoms' four, Hobbit, Spiders, Orc/Warg) are matched against the hero's vanilla `race`, `faction`,
  unit types and name (`HeroUpgrade.Allows`, plurals count), so a Hobbit of the Wanderers in the Wild is offered both upgrades and the
  others only the open one. A level-up offers one upgrade, and a hero takes each once. What a skill does to units of one kind in a match
  is the player's to apply at the table.
- **Rule exchanges are applied** to the models (`applyCompanyRules`): Sharkey's Rogues' Hatred (Rohan) is Hatred (Hobbit) on every card.
- **Displayed only for now:** substitutions of rules, auras and break points (match layer, played at the table, never
  applied by the app), and, in the management layer, "On Their Own Path" units, Arnor's promotion, Fornost's reinforcement replacement,
  the Vault Warden Team and the Warg Marauder's three roster slots. The data has them as `companies[].effects[]` with a `layer`
  (management, match or both).

### Renaming units

A unit is renamed by double-clicking its name, or with the pencil next to the name on the expanded card (the collapsed card shows no
extra button, so nothing moves). The name is at most 40 characters; an empty name takes the unit back to its type. Rules that aim at a
unit use its id, so they follow the new name (`campaigns.RenameUnit`).

### Gold

Gold is given to some companies in their starting company, as a special victory reward of some scenarios, and by the organizer
in a narrative scenario (a custom one); what is left is kept (`campaign.gold`). Equipment, creatures and wanderers cost their
rating (`gold` in the file) in Gold and no influence, and wargear can be bought with Gold as well. Slots, limits and
restrictions still apply. Every buy button of the armory (the tab, a unit's equipment window, creatures, wanderers) offers the
payments the company can afford: "Buy · N influence" and/or "Buy · G gold" (`pay` field). The Gold, Influence and Re-rolls
cards of My Company have tooltips saying what they are for and where they come from. There is no screen yet for the organizer
to grant Gold or for scenario results to award it.

### Glossary and item definitions

The Armory lists the full definition of every item (the rules file's annotation text, the stat line and the price). A campaign's
Glossary tab is its glossary file plus everything the campaign's vanilla and Battle Companies files define (`core.GlossaryExtras`):
special rules, magical powers (with the Sorcerer's casting value and range), wargear, equipment and mounts, paths, path rules
and heroic actions, creatures, wanderers and injuries, sorted by category and then by name, with a heading per category (hidden when a search leaves it empty), a category badge on every term and a category filter; the default glossary of the sidebar is only the
glossary file. When a name is defined in several places Battle Companies wins (it is a game mode built on the base rules): its term replaces
the glossary file's term of the same name; the glossary file wins over the vanilla file.

### Creatures and wanderers

The Armory tab lists **creatures** (for the leader and the sergeants, one each, at most 3 per company; `data.creatures`) and
**wanderers** (mercenary heroes, one at a time; `data.wanderers`) below the gear. A creature is not a unit: it takes no
roster slot, equipment slot or wargear limit and rolls no injuries, but it counts for its points in the rating. The owner's
card shows a creature chip (tooltip with its stat block) and a small card appears under the units. The **Baggage Pony**
gives the company a pack (`companyFeature.carryGear`): "Open the pack" lets any member store, take out or swap gear for
free (`campaigns.Carry`; what a member gets must be something it could buy, mounts and upgrades cannot be carried, gear in the pack still counts towards the bow and throwing limits, and the
app does not count the "once per occasion" yet). A wanderer is a normal hero card with a Wanderer badge: it counts towards
the unit limit, has no experience, cannot buy equipment, rolls the hero injuries and can be dismissed (no refund).

Item tooltips read the Battle Companies file's `gearAnnotations[].definition`; a mount's tooltip also has its stat block
(`gearAnnotations[].profile`, the vanilla mount entry).

### Dice

The dice are 3D: `static/js/dice3d.js` (three.js draws them, cannon-es rolls them) loads the first time a roll needs it, so other
pages never pay for it; where WebGL or the file is missing, `static/js/dice.js` falls back to its flat 2D tray. `window.diceMixin()`
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
- **Paths.** A path's glossary term is shown as in the rulebook: its characteristic limits as a centred row of cells with the abbreviated attributes linked to their terms (a number of level-up increases is written `(+2)`, a total `9`, a best value `3+`), then the 2D6 chart as a table (each result linked to the rules it names). `core.PathTable` carries the data (`GlossaryTerm.Path`); the level-up lists paths by name, and the tooltip draws the same two tables (`data-tip-path`).
- **Lanes.** A hero's two rolls run in two lanes of the same table (taller than a single lane, so thrown dice do not keep hitting the divider), one pair each, named and totalled at the right edge.
- Typed rolls ("I rolled a N") are the campaign setting **Players may enter their own rolls** (off by default, set at creation,
  shown in the Overview): off, every roll panel only has a Roll button and the backend refuses a typed roll.
- Reduced motion: the dice lie in a row showing their numbers. The style guide has a demo.

### Company management: what is built and what is missing

Roster, promotions (level-up, advancements) and injuries (Heal) are the cards of My Company above and run on the real
backend. The company pages and the aftermath ([aftermath.md](aftermath.md)):

- **Aftermath banner** (built, `#aftermath-banner`) at the top of My Company when `CompanyDetail.Aftermaths` is not empty: a link per
  match to `/campaigns/{id}/matches/{match}/aftermath/{company}` (Start or Continue). The company is locked: the controls marked
  `data-lock` (Recruit, Armory buy, Drop, Heal, Dismiss) look disabled and, on click, `lockClick` in `company.js` says "Finish the
  aftermath of Match N first." instead of acting (the root's `data-lock` holds the reason, empty when unlocked). The core refuses
  the action too, so a stale page shows the refusal as a notice.
- **Perks card** (built, `perks_card`): the company's unused scenario perks (name, the relic bearer, text, when it applies) with
  **Mark used** for the owner (`POST /campaigns/{id}/my-company/perks/{perk}/use`, `UsePerk`); used ones sit under a collapsed
  "Used (n)". Empty state when there are none.
- **History card** (built, `history_card`): the latest 20 aftermath lines (`CompanyDetail.History`), newest first, on both company
  pages; the grants have their own card. **Built (34m):** the match heading is a link to the match with its date (`fmtDateShort`); the card takes `(dict "Lines" … "CID" campaign id)`. **Contract (ro1):** every `core.AftermathLine` now carries `Match` (number), `MatchID` and `On` (time.Time: when the aftermath was finished, else started, else when the match ended; zero when unknown), on `CompanyDetail.History` and on `Aftermath.Lines` of the match page. Show "Match N" as a link to `/campaigns/{id}/matches/{MatchID}` (not needed on the match page itself) and the date through the user's timezone formatter; the lines of one aftermath share the same values.
- The company page of somebody else (`/campaigns/{id}/companies/{company}`, from the Companies tab) is the **My Company page, read
  only** (`myCompanyView.ReadOnly`, same template `campaign_my_company`): a "Viewing X, the company of Y. Read only" banner with a
  link back to all companies, the same stats, limits, full unit cards (expandable, tooltips) and cemetery, creatures, perks
  (no "use" button) and history; no rename, recruit, formation, buy, drop, heal, level-up, path, pack or dismiss controls and
  no buy window. The organiser's award form of the grants card stays. Your own company's URL redirects to My Company.

## Live match (built)

The design is in [live-match.md](live-match.md) and [architecture.md](architecture.md) "Live match"; this is what the pages do now
(`internal/web/live*.go`, `static/js/live.js`, the `live_*` templates in `campaign_my_company.gohtml`, `company_panels.gohtml`,
`units.gohtml` and `match.gohtml`). `web` declares `liveMatches` (embedded in `campaigns`; `core.LiveService` implements it).

- **Company page of a company in an open match** (`CompanyDetail.OpenMatch` says so; `liveFor` loads `Live`): the same page with
  - `#live-banner` (sticky under the app bar): "Match N is running" with the scenario, the sync dot, **End checks** (everybody) and, for who may edit, **Match log**;
  - the roster locked like read only (no recruit, buy chips, heal, level-up, drop, dismiss; rename stays) with the line "The company is
    locked until the match ends"; "Change formation" is hidden while live (the grid is re-rendered by the sync);
  - `#live-zones` (`live_zones`): the Field as unit cards, then **Retreated**, **Field casualties** and **Missed this game** as
    one-line headers with a count ("None" when empty) and compact cards (`live_compact`: badge, reason, **Return to the battle** for
    who may edit), then the Cemetery. `GET /campaigns/{id}/companies/{company}/live` answers `#live-zones` alone (its `data-seq` is the
    match's last sequence).
  - Another player's company is the same page read only (`ReadOnly`); an editor who is not the owner (organiser, administrator) has the
    trackers and actions on it. Viewers see `current / start` without buttons.
- **Trackers** (`live_trackers`, replace W, Might, Will, Fate in the attribute row): `-` and `+` (44 px high), a tap on the number opens a
  "set to" field; the buttons are disabled at 0 and at the start value; the number is red at 0 (and at 1 for Wounds when it started higher).
  Each press is one `POST .../matches/{match}/live/{company}/actions` (`kind=stat`, `unit`, `stat`, `delta` or `set`), answered by
  `live_response`: the zones plus an out-of-band **toast** (`#live-toast`) with the log text and **Undo**, or, on a refusal, 422 with
  the state as it is and the reason as the toast. A viewer's write is 403. Live POSTs are not wrapped in `once()`.
- **Actions** button on each field card (editors): opens the company window with the list (Wounded a unit, Passed a fate roll, Passed a
  Broken courage roll, Retreated, Died, Mount removed/re-added; Fate and Mount only when the unit has them), each opening its sheet
  (`GET .../live/{company}/units/{unit}/sheet/{kind}`, `live_sheet`). **Wounded a unit** lists the enemies in the Field (then the same
  side after a "Friendly fire" divider), with search, Rider/Mount for a mounted target, wounds caused and saved by Fate steppers
  (`woundSheet` in `live.js`) and a preview sentence in the words of the log. REVIEW: a window, not a bottom sheet or popover (a popover
  inside the card is clipped by its frame).
- **Break Point, turns and courage** (36y.15-17, `live_break` in `campaign_my_company.gohtml`, inside `#live-zones` so the SSE refetch refreshes viewers too): a turn row ("Turn 3" or "Turn not started", **Start turn N** for editors: `kind=turn`, `turn=N`; a stale press is the service's 422 toast); a Break Point panel (`<details>`, summary "Break Point 6.5 of 13 · Casualties 4 · 50%" with a "Will break next turn" or "Broken since turn N" badge and a progressbar; open: the casualties by name, the scenario exceptions and, for editors, +/- adjusters for starting models and casualties with a shared note, `kind=breakAdjust`); the **Company broken** banner (`#live-broken`, danger alert, not sticky: it would cover the live banner) with the All is Lost choice as an inline `<details>` (**Flee the battlefield** with a confirm / **Fight on**, editors, while `Break.Offer`) and "Who has tested this turn". Cards of a Broken company show "Test due" / a resolved chip (`CourageDue`: `CourageTurn != Turn`; the chip is `liveView.CourageChip` from `LiveUnit.CourageHow`: "Tested: passed", "Tested: failed", "Auto-pass" for standFast, heroicResolve, chariot, scenario, "Exempt" for engaged, cannotActivate, else "Courage resolved"). The company header (own page and other companies' read-only page, `campaign_my_company.gohtml`) shows "Break Point: N% of the starting models" from `CompanyDetail.BreakPointPercent` (the company's own rule, e.g. 66 for Muster of Isengard, else the file's default; never 0 with real rules) and a primary **Courage test** button that opens the `courage` sheet (`live_courage`): Roll 2D6 (`POST .../units/{unit}/courage/roll`, `RollCourage`, nothing written; typed dice only with ManualRolls; a plain server roll, not the animated dice component), "Record: passed" (`broken`, with Call Stand Fast for Heroes that are not Wanderers) or "Record: failed" (`retreat` why `courage`, confirm), and the six pass or exempt buttons (`courageSkip`). All writes go through the one `actions` route (`liveActionFrom` reads `turn`, `field`+`delta`, `reason`, `flee`, `standFast`, `item`+`index`). REVIEW: the spec's routes `turn`, `break`, `courage`, `allislost` are not separate; the All is Lost choice and "Who has tested" are inline rather than sheets.
- **Against the Odds items** (36y.10): `LiveUnit.Loaned` is listed under the Wargear of the card, grouped by item ("Climbing ropes ×5", tag "Loaned (Raider's Kit)", "use after the game" for Healing Herbs and Wondrous Cram, struck through and "used" once spent); owned items in `UsedItems` are struck through; the action menu has **Item used** (`item` sheet: the loaned items still to spend, `kind=itemUsed`) when something is left. `LiveCompany.Wanderer` makes a card for the temporary Wanderer (`temporaryUnit`: a roster unit with the synthetic negative id, badges "Wanderer" and "Temporary", the profile summary, Rating as its points, no Dismiss), in the zone its live unit is in, and a wound target in the wound sheet.
- **Result form** (36y.17): for a company whose result was not reported yet, `prefillLiveDefaults` ticks **Fled** (All is Lost) and the `enemyBroken` objective from `MatchDefaults`, each with the badge "From the live match", and the Fled switch explains "The other side claims victory (All is Lost). Saving this does not end the match." Not built: the company page and My Company do not show the Break Point rule (50% or 66%), because `CompanyDetail` has no field for it yet (a backend bead).
- **Mount lost**: "On foot" badge; the mount item, the gear with `Equipment.RequiresMount`, the rules the mount gave and the Cavalry rule
  are struck through (`itemView.Struck`, `ruleView.Struck`, aria-label "needs a mount"). The data flag `requiresMount` is the curator's
  (pib.15); nothing sets `RequiresMount` yet.
- **Match log**: `GET .../matches/{match}/live/log` is the sheet (company page; `?company=` so Undo refreshes the zones out of band), `?card=1` the card of
  the match page (below History, drawn inline in the full page, refreshed on `live:match`), `?before=<seq>` the next 30 older rows. Newest first,
  grouped by minute, Undo only where the service says `CanUndo` and the match is open; a dependency refusal shows in the log. After the
  match the card stays (read only, permanent); an ended match recorded after the fact says it has no log.
- **End checks** (`live_end.go`, `end_*` templates in `match.gohtml`): the sheet (banner button, `GET .../matches/{match}/live/end?company=`) and the card on
  the match page (`?card=1`, `#end-checks-slot`, refreshed on `live:match`, empty once the match ended) show `core.EndChecks`: per own company a
  switch per unit "Got the objective" (`POST .../live/{company}/objective`, `unit`, `on`), per opposing company a radio list "Select MVP" with "No
  MVP" (`POST .../live/{company}/mvp`, `unit`; 0 withdraws) and the vote tally (a tie says the owner picks in the aftermath). Every write answers
  the body again with a message and Undo (`from=end|endcard` of the shared undo route); a refusal is 422, a user with nothing to set gets 403.
  **Propose to end** is a button that fetches `.../live/end/reminder?revision=` (what is still open, **Go back** / **Propose anyway**; never blocks;
  `POST .../end/propose`).
- **Ending a match by confirmation** (`match.go` `handleEnd`, `live_end_banner.go`; templates `end_card`, `end_propose` in `match.gohtml`, `end_override` in
  `partials/end.gohtml`, `end_banner` in `campaign_my_company.gohtml`). Routes (all `revision`, 422 refusal, stale revision answers the current state):
  `POST .../matches/{match}/end/propose|confirm|withdraw|cancel|override` (override also `note`, trimmed, 1-200). `core.EndAccessFor` (via `endAccess`) hides the
  controls; the service enforces. Match page: the header button is hidden while an end is pending and the **End card** (`#end-card-slot`, refreshed on
  `live:match` through `GET .../end/card`, which also sends the header button out of band) lists one row per company (**Confirmed** with who and when / **Waiting**),
  the reset notice, **Confirm the end** (owner of a waiting company), **Withdraw my confirmation**, **Keep playing** (any participant, organiser, admin) and, for
  the organiser or an administrator while somebody waits, the quiet **End without confirmation...** that opens the warning and the note field (the dialog stays
  open with the error on a missing note). Viewers see rows without buttons. The history line of an ended match carries `MatchEvent.Detail`.
  Company page: `#live-end` inside the sticky `#live-banner` ("Ending: waiting for Gorgoroth", same actions, override included) is fetched on load and on
  `live:match` (`GET .../live/end/banner?company=`); its forms post with `view=banner&company=` and get the banner back, or `HX-Refresh` when the match ended.
  **Start turn** is disabled with "Cancel the ending first: the match is about to end." while `liveView.EndPending`. The log lines ("Ending cancelled: ...") come
  from the backend and show in the normal log.
  On the aftermath sheet the prefilled objective shows "From the live match" and the MVP "Voted MVP". REVIEW: a viewer sees the tally only
  (`EndChecks.Objective` is empty for them); no per-unit vote counts or tie hint on the aftermath sheet (the view has no field for them).
  The two log/end slots set `hx-target="this"` (the body's inherited `hx-target="#main"` would swap them over the page).
- **Sync** (`live.js`): one `EventSource` on `/campaigns/{id}/matches/{match}/events` per page that has a live part (`#live-zones` or
  `[data-live-match]`), none elsewhere; an event of the page's company with a `seq` above the zones' `data-seq` refetches the zones (the author's
  own change is ignored); `ended` reloads the page; the stream is closed while the tab is hidden and reopened (with a refetch) when shown;
  the dot is green "Live", pulsing grey "Reconnecting" (the browser retries every 3 s; when the stream is back the page refetches what it
  missed, the state is in Postgres); when the stream is refused (caps) or unsupported the page polls every 10 s.
- **Secret objectives** (`secret.go`, `secret_card` in `match.gohtml`; only for a scenario with `ScoringObjectives[].Secret`, i.e. Fog of War): the card
  (`GET .../matches/{match}/secret`, drawn inline in the full page; the endpoint serves the refresh and the fallback) lists, for each company the viewer owns, each secret objective with a nomination
  form (a select of own or enemy Heroes without the Leader unless he is the only Hero, or a text box for the terrain piece) and a tier radio
  list ("Not reached yet" clears); `POST .../secret/{company}` (`kind=nominate|done`, `key`, `unit`/`note`/`tier`) applies the live action, with an
  Undo (`from=secret`). While the match is open nothing of other players is shown but "Noted already: <companies>" (privacy is the service's);
  after End match the card lists everybody's. On the result form of an ended match the Victory Points of a side with none reported are prefilled
  from `SecretScore` with the badge "From the live match" (editable, saved only with Save result).
- **Perks in the rolls**: the recruit roll form shows **Rendezvous** (Not now / +1 / -1, field `perk`, `RollRecruitWithPerk`) while the company has an unused
  Rendezvous (`core.UnusedPerk(detail.Perks, ...)`; the free roll on the special chart does not offer it); after a scenario roll ("Roll for it") the
  panel offers **Vantage Point** +1 / -1 on the second die to an owner of a company in the match with an unused one (`POST .../scenario/roll` with
  `vantage`=company, `adjust`; the dice posted are the ones on show, so the typed-roll setting does not apply; `UseVantagePoint` returns the moved die and the line shown).
  The recruit window shows the moved die as "4, Rendezvous +1 = 5" (`RecruitRoll.PerkRaw/PerkAdjust`).
- **Gear notes on the card**: a `StatMod` with `Delta` 0 and a `Note` (the Small Shield's "+1 Defence against strikes only") prints as a muted line under the
  attributes (`data-stat-note`), besides the characteristic's tooltip, so it shows on a phone too.
- **Finished campaigns are frozen in the UI**: My company is read only with the notice "frozen as it was" (`myCompanyView.Frozen`, badge "Frozen"; no recruit window),
  an open match says it stays open but cannot change, and shows no End checks, Roles or secret objectives cards. The organiser (not only administrators) sees
  "Edit the result" and "Reopen match" on an ended match (`CanManageCampaign`).
- **Aftermath MVP hint** (`Aftermath.MVP`): above the units of the sheet, the vote lines, who is ticked, or "Tied: ..." with the owner choosing. Revealed secret lines print `SecretLine.Label`.
- **Admin Users** rows with `AdminUser.Organises` have an "Organises N campaigns" expander listing each campaign (status, players) with a select of the campaign's members except the organiser (`Campaigns.Members`; "Name (Company)" or "Name (no company yet)") and **Hand over** (`POST /admin/users/{id}/handover`: `campaign`, `organiser`, `email`; `Campaigns.HandOver`, administrators need not be members).
- **Side roles** (Attacker/Defender tags, docs/matches.md): a **Role** select per side on the new-match form (`side-role-N`, not in all vs all) and, on
  the match page, a **Roles** card for editors of an open match (`POST .../matches/{match}/roles` with `role-N`, `.../roles/swap`; 422 with the
  reason when invalid) plus a badge on each side and per company on the result form. Offered only while the scenario is not chosen or has roles
  (`matchView.ShowRoles`); End match with a roles scenario and untagged teams shows "Choose which team attacks and which defends".
- **Match creation** shows "Company X is already in Match N." with a link (`*core.CompanyBusyError`) or "Finish the aftermath of Match N
  first."; the aftermath sheet shows **From the live match** (heading badge and a line) while it is the unsaved default
  (`Aftermath.FromLive`; REVIEW: per-value badges are a later option).

## Account

`/account` (`internal/web/account.go`, `templates/pages/account.gohtml`) is one column of cards, each an htmx form that
swaps only its own `<section>` (`hx-target` + `outerHTML`) and answers 422 with what was typed kept (never a password):
**Profile** (name; email read only), **Password** (current, new, confirm with show/hide; the browser checks the match and
the length, the handler repeats it; wrong current passwords are throttled per user with the same `loginThrottle` as
login), **Sessions** (Sign out everywhere else; the notice says how many ended) and **Close account** (a collapsed
`<details>` danger card asking the email typed again and the password, both required; a refusal from the core shows its
reason; success clears the cookie and `HX-Redirect`s to `/login?notice=closed`, which shows a fixed message). The
`auth` interface of the web layer carries `UpdateProfile`, `ChangePassword`, `SignOutOthers`, `CloseAccount` and
`SetAppearance`, `SetTimezone`. The show/hide password field and the danger card are in `/styleguide` ("Account patterns").

**Time zone** card (`account_timezone`, `POST /account/timezone`, field `zone`): a text input with a `<datalist>` of the IANA names (`timezones.go`), empty = automatic, buttons
"Use this device's zone" and "Automatic"; an unknown name is 422. **Times** are stored in UTC and shown in the viewer's zone (`location.go`): the session
middleware works out `core.UserLocation(user, tz cookie, APP_TIMEZONE)` once per request (context for Go code such as the live log headings, and on the
response writer for the renderer, which keeps one template set per zone cloned from a never-executed one, at most 64). Templates print times with
`fmtDate`, `fmtDateShort` and `fmtDateTime` (a `time.Time` or `*time.Time`), never `.Format`. `static/js/tz.js` (every page) sets the cookie `tz` to the browser's zone.


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
multi-word names stay case insensitive (REVIEW: the case rule applies to every single-word term). `web.glossaries` (`core.Glossaries`, which builds once per file) gives a campaign's glossary, the default one, or one glossary file alone by
hash (`ByHash`, `core.ErrNotFound` for an unknown hash or a file of another kind).

A row of **category chips** (All plus one per category, `aria-pressed`) sits under the search and combines with it; the
choice is kept in the URL query `?c=<category>` (an unknown category is ignored), and the glossary's name, version and short
hash show under the title. When nothing matches, a status line names the search and category and offers "Clear the filters".

The sidebar page can show **another glossary**: a collapsed "Use another glossary" form with the rules file picker (kind glossary: a
file the server already knows by hash, or an upload; `POST /glossary`, `rulesetChoice`) keeps the file's hash in the `bcm-glossary`
cookie (a year, this browser only). With a cookie the page shows that file through `glossaries.ByHash`, says it is not the default
and offers **Back to the default** (the cookie is cleared); an unknown or malformed hash falls back to the default with a notice and
drops the cookie. The campaign tab always shows the campaign's own glossary. (An explicit link list in the file would be the alternative to the case rule.)

## Invite links

The organiser (and administrators, until the campaign is finished) sees **Invite players** in the campaign card of the overview
(`invite_card`, `internal/web/invite.go`): `POST /campaigns/{id}/invite` makes a link (`campaigns.Invite`, 7 days, unlimited uses) and
the card shows it once in a read-only field with **Copy**; **Make another link** replaces it. Opening `/join/{token}` never changes
anything: signed out visitors are sent to `/login?next=/join/{token}` (`next` is kept only when it is a path of this site) and
come back; signed in ones get a confirmation, and **Join the campaign** (`POST /join/{token}`, `campaigns.Join`) redirects to the
campaign, where they can create their company. An unknown, expired or used-up token shows "not valid any more" (404).

## Demo data and signing in

`make seed` (idempotent: it does nothing when the campaign exists; `make db-reset` empties the database first and is **destructive**) creates four accounts that share one password (default
`demo-password`, `make seed PASSWORD=...` to change it) and "The Siege of Minas Tirith", where each has a company with its heroes' paths
chosen and 10 Gold / 6 Influence granted: `admin@example.com` (Gandalf, **superuser**: use it for the administrator views such as
`/admin`, reopening matches and waiving aftermaths), `you@example.com` (You, the organiser), `eowyn@example.com` and
`sauron@example.com`. Match 1 (You against Sauron) has ended and the organiser has waived both aftermaths (the note says so), which frees the companies for match 2, an open three-sided match. The seed needs an empty database for this: `make db-reset` first, or `make demo` for the rich world.
Further accounts: `make user` and `make grant`. For the rich world (every state of every page) use `make demo`; see [demo-world.md](demo-world.md).

## Create company and aftermath sheet details (review3)

- **Locked Leader.** A company whose start list names a Leader that is also its Ranger replacement unit (the Shire's Ranger of the North,
  `startRow.Locked`) shows him in the Leader place with `data-keep`: he cannot be dragged, removed or replaced, the Leader button does
  nothing and the card says "Always leads". The core validates the same rule.
- **Aftermath sheet.** The experience a unit earned are small toggle chips (`chip`, a tick when on) under "Earned experience"; the
  outcome labels of a saved sheet are `badge-success` (survived), `badge-warn` (fled) and `badge-danger` (casualty), tints of the semantic
  tokens ringed in the colour (in `tailwind.css` the `.badge` base rule comes before every `badge-*` tone: it has the same specificity and
  would otherwise win; `TestBadgeBaseComesBeforeTones`, and the e2e flow `81-aftermath-ui` reads the computed colours). The injuries step spells out the bonus die of Lesson Learned and Wounds of a Hero (`BonusLine`).
