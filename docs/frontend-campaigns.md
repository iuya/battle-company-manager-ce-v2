# Frontend: campaigns, rules files and the admin pages

What the campaign list, the campaign pages and the rules-file screens do.
The design language and the page patterns are in [frontend.md](frontend.md).

## Contents

[Main page: campaigns](#main-page-campaigns) ·
[Campaign pages](#campaign-pages) ·
[Rules files (vanilla and Battle Companies)](#rules-files-vanilla-and-battle-companies) ·
[Handing over a campaign (admin)](#handing-over-a-campaign-admin)

## Main page: campaigns

`/campaigns` (and `/` for a user with no remembered campaign) lists the user's unfinished campaigns as cards (name, status badge, description, progress, the user's company in
it). "New campaign" (`/campaigns/new`) is shown to users who may create campaigns: admins, superusers, and users with
the `can_create_campaigns` flag (`User.MayCreateCampaigns`); the server answers 403 to anyone else. Roles are
`user | admin | superuser`, set in the Administration section (organiser and administrator switches) or with `make grant EMAIL=… ROLE=… [CREATE=1]` (the only way to a superuser).

The brand link in the header (for a signed-in user), the sidebar's "Campaigns" and every "← Campaigns" / "Cancel" / "Back to your campaigns" link point at `/campaigns`, the one address that never redirects, so a user whose `/` jumps to their last campaign can always reach the list. Creating a campaign returns to it.

A "Show finished campaigns" switch adds the finished ones after the active ones (cards dimmed, sidebar entries greyed).
It is remembered in the browser cookie `bcm-finished` (a one-year preference, nothing is stored server side); the
handlers read it for both the page and the sidebar sub-menu, and the switch reloads both.

The campaign list has a search box that filters the cards by name in the browser (`listSearch` in
`static/js/lists.js`; accent and case insensitive, no request). It only sees what is on the page, so finished
campaigns are searched only while "Show finished campaigns" is on.

`web.campaigns` is implemented by the facade in `cmd/server/backend.go` over the core services and the PostgreSQL store (see
[architecture.md](architecture.md)). `make seed` fills a development database with demo accounts and a campaign.

## Campaign pages

`/campaigns/{id}` has six tabs.
Each tab is its own URL and page.
The header and tabs are the `campaign_header` partial (the campaign title on one line on every inner page, the description only on the Overview), and `campaignBase` carries the campaign and the current tab.
The sidebar lists the same tabs under the campaign you are in.

| Tab | What it shows |
|---|---|
| **Overview** (`/campaigns/{id}`) | Your company (or "create my company"), campaign facts, rules files, latest matches |
| **My Company** | Your company: the page "Manage company" opens. Your company's own URL under `/companies/{id}` redirects to `/campaigns/{id}/my-company/{company}` |
| **Armory** | What the company can buy |
| **Rankings** | Every company of the campaign, by points. Another company opens read only; yours redirects to My Company. `/campaigns/{id}/companies` redirects here |
| **Matches** | The campaign's matches |
| **Glossary** | The glossary this campaign is played with |

Entering a campaign opens **My Company** (`/campaigns/{id}/my-company`), the page a player uses most.
This covers its name in the sidebar, its card on the campaign list, the last-campaign redirect of `/` and an invite link.
The Overview stays the first tab.

- **Manage campaign** card on the Overview (`CanManage`: the organiser and administrators; `campaign_manage.go`, template `manage_card`): the
  status badge; **Pause** / **Resume** with one inline confirmation; **End campaign** in three steps (consequences from
  `CampaignDetail.OpenMatches/PendingAftermaths/ActiveInvites`, the exact campaign name typed, a final "End the campaign for good"), posted to
  `POST /campaigns/{id}/status` (`to`, `confirm`; success redirects to the overview, a refusal re-renders the card with the reason, 422).
  A finished campaign shows "ended for good" to the organiser; administrators get **Reactivate**. The invite block shows "N links work now"
  with **Regenerate** and **Invalidate links** (inline confirm; `POST .../invites/regenerate`, `.../invites/invalidate`); while paused it hides
  "Invite players" and Regenerate. Paused: the Matches tab hides New match (`CampaignDetail.CanStartMatch`) and says why, `/matches/new` is 403, the
  join page says the campaign is paused (`InvitePreview.Paused`; a refused join is 422). Finished: the Matches tab says matches are frozen.
- **Practice matches** ([matches.md](matches.md#practice-matches)): the new-match form has a **Practice match** checkbox (`name=practice`, off by default, `data-practice-box`; a line says what is skipped and that it cannot be changed; the "Same player" note
  offers "Make it a practice match"); the Matches tab shows the badge on the row (no Practice filter); the Overview line "n practice matches" (`data-practice-count`); the match page banner
  (`data-practice-banner`) and the aftermath block text; the aftermath page short state; the live bar badge. 360 px: the badge sits on the title line and wraps below; no horizontal scroll; semantic tokens only.
- **Several companies per player** ([multi-company.md](multi-company.md)). The campaign setting
  **Companies per player** (1 to 4, default 1; `CampaignSettings.CompanyCap`) is a number field on the creation form (`companies-per-player`, with
  its help text; empty keeps 1, a value outside 1 to 4 is a 422) and is editable later by the organiser and administrators (`companies_cap_card`
  in the Campaign settings card of the Overview, `POST /campaigns/{id}/settings/companies`, `SetCompaniesPerPlayer`; success reloads the Overview,
  a refusal or a non-number shows in an `alert-danger` on the control, 422). The help text states the rule: lowering it only stops new companies,
  the ones that exist stay and keep playing. Everyone sees the value in the settings list.
  - **The company is in every owner address**: `/campaigns/{id}/my-company/{company}` and everything under it (recruit, units, formation, cemetery,
    pack, rename, perks). One helper, `ownCompany` (`campaign.go`), reads `{company}` and answers 404 unless it is one of the viewer's own
    companies of the campaign (`CampaignDetail.Own`); `loadOwn` is `loadCampaign` plus that check and sets `campaignBase.Current`, `ownCompanyID`
    is the same for the POSTs. No handler guesses a company, and a stranger or the organiser learns nothing. The bare `GET /campaigns/{id}/my-company`
    is an **alias**: 303 to the company the player opened last (their oldest until the last-opened cookie exists), or the "Choose your company" page
    when they have none. The armory takes the company in `?company=ID` (the bare tab 303s to the first) and its buy forms carry a hidden `company`. The
    templates build every owner address from `campaignBase.OwnPath` (or the `Co` of a window's data).
  - **Company switcher** (`company_switcher` in `partials/campaign.gohtml`, on `/styleguide`): chips under the tab bar of My Company and the
    Armory, **only when the viewer owns two or more companies** (`campaignBase.Switcher`); a player with one company sees no chip, no new wording
    and the same pages as before, whatever the limit. One `chip` link per company (name, the faction from 640 px up; the current one
    `aria-current="true"` and `chip-on`; plain navigation, `hx-boost="false"`), then a "New company" chip while `CanCreateCompany`, else the note
    "All N companies created". At 360 px the row scrolls sideways, from 640 px it wraps. With several companies the page title names the company.
  - **New company** (`/campaigns/{id}/company/new`) opens for a player below the cap (a lowered cap counts the same way); at the cap it redirects to
    My Company, and a stale POST gets the refusal "You already have the N companies this campaign allows." (422; "a company in this campaign" when
    the cap is 1). A created company opens at its own address. Creating a company is permanent.
  - **Overview**: "Your company" (one) or "Your companies" (several), one block per company with Manage company, then "Create another company"
    below the cap, "You have all N companies this campaign allows." at it, or "This campaign now allows N ... per player; you keep your M." above it
    after a lowered cap. With a limit of 1 nothing is added. Companies and Rankings mark every company of the viewer "Yours" and show the owner.
  - **Same player** (`core.SamePlayers`): a non-blocking note on the match page (`matchView.SamePlayerNotes`: "Same player: both of these companies
    are X's." / "Same player: X plays both sides.") and, live, in the new match form (`match_form.js`: "Both are X's" under a side, "Same player:
    X plays both sides" across sides). Nothing is refused. The form lists each company with its owner and faction and marks the viewer's own
    (accent border, "Yours" badge): first, and last on the other sides once one of them is placed.
- A company page shows a management section only to its owner. `/campaigns/{id}/company/new` creates a company (name, faction).
- **Rankings** sort by the total point cost of a company's units (`core.Rank`; equal points share a rank) and show
  wins/draws/losses, matches played and unit count.
- **Matches** (`pages/campaign_matches.gohtml`): a **Your turn** list on top (`yourTurns`, partial `your_turn`; also on the Overview):
  the matches that wait on the viewer, as buttons ("Vote the MVP and confirm the end of Match 25", "Start the aftermath of Match 3",
  "Continue the aftermath of Match 4", with the company when the viewer has several); nothing when nothing waits. Then the filter chips
  **Mine** / **Open** / **Ended** (`?filter=mine|open|ended`; no chip on lists every match) and the list, newest first, one compact row per
  match (partial `match_row`): its sides (`A + B vs C`), "Match N · scenario", the Practice badge and the result badge (Open, Draw,
  "<side> won"). Names are coloured by outcome: the winning side green, losing sides red, every side yellow after a draw (`Match.Tone`).
  Wins/draws/losses are shown as green/yellow/red numbers wherever they appear (`wdl` partial).
- **New match** (`/campaigns/{id}/matches/new`, `internal/web/match_new.go`, `static/js/match_form.js`): two side blocks (accent border, thick top bar) with the company search, then the scenario card
  only (added companies are chips that can be removed, a company already on a side no longer shows up, Enter adds the first result),
  **Practice match**, **+ Add side** (up to 8), **All vs all** (one pool where each company fights alone, with "Add every company";
  `mode=ffa`); the scenario card is the one the server rolled (Roll again, Choose another, One-off scenario). Against the Odds is chosen on the match page.
  The form is posted as `sides`, one `side-N` per company and the scenario fields;
  `core.ValidateSides` checks that every side has a company and none repeats. Any player of the campaign can create a match. A campaign
  with fewer than two companies offers no "New match" (the matches page says what is needed) and the form page says the same instead of
  showing the form; a finished campaign offers and accepts none. See [matches.md](matches.md#new-match).
- **Match page** (`/campaigns/{id}/matches/{match}`): only the current phase is open (preparing, playing, ending, ended; `data-phase`):
  header (result badge, date, "Ended <date>"), the scoreboard (one row per side, no VP, no roles), **Scenario** card (a rulebook scenario with
  "How it is scored" collapsed, or a one-off with its description and victory text escaped, `whitespace-pre-line`; the folded Special rules print the rule
  tables of the file (`ScenarioText.Table`, template `rule_table`: Strange Aura by D6, the Tame the Beast Monster Budget with the row of the
  companies' combined rating marked) and the rules a scenario `usesRules` by name), **Go to my company** and **End match** while playing,
  the End card while an end is pending, and the result, Aftermath card and note once ended; History and the match log share one
  **Log** card; it and the scenario (with its sections) are open while playing or ending and folded otherwise (`matchView.InProgress`). The End match and Confirm sheets open in the page's window (`sheet_dialog`, `#modal-body`). The view carries the viewer's
  `core.MatchAccess` (`Match.Access`), which decides the edit controls; see [matches.md](matches.md).

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

The form is `multipart/form-data` and not boosted. Rules files of every kind are identified by their **hash** alone (`GET /rulesets/{hash}` ignores `name` and `version`; `core.Rulesets.Find` keeps them in its signature only): two files with the same name and version coexist, and `name` and `version` are display metadata (the vanilla one reads `game` and `appVersion`, `core.RulesetMeta`).

Wherever a file is shown (picker, overview's Rules card) it reads "name · version" followed by a short-hash label (the
first 8 characters, `RulesetInfo.ShortHash`), so two files with the same name and version can be told apart.

The store is `core.Rulesets` over PostgreSQL; at startup it registers the real files under `definitions/` as the defaults (so
the server must be started from the repository root).

### The rules file report

A refused rules file shows **every** problem, not the first. `rulesetIssues(err)` (`internal/web/ruleset_report.go`) reads the
`*core.RulesetReport` out of the error with `errors.As` (also the merged report of the campaign form) and builds a `reportView`:
issues grouped by file in report order, at most 200 rows (`maxIssuesShown`), the rest counted together with the core's own
`Truncated()` as "and N more not listed", plus the error and warning counts. `partials/ruleset_report.gohtml` draws it: a status
line ("12 problems found. Nothing was stored."), one section per file (monospace file name), one row per issue with an Error or
Warning badge (text, not colour only), the message, `At <path>` in `<code>` (breaks anywhere, no horizontal scroll), the closest
known name as a muted chip and the hint. It sits in an `alert-danger` on:

- the **admin upload card** (`POST /admin/rulesets`, 422, the panel is swapped with the report; a refusal that is not a report
  keeps its one-line reason);
- the **new-campaign form** (`POST /campaigns`, 422 with the form kept; the three files are all checked before any is stored,
  so the text is "Nothing was stored and no campaign was created."; the browser keeps the name but a file input cannot be
  refilled, so the file is chosen again);
- the **Check this file** button of the admin upload card: `POST /admin/rulesets/check` runs `rulesets.CheckSet` (a dry run, stores
  nothing) and swaps `#rs-check-result` under the buttons: 200 `alert-success` ("No problems found", warnings listed) or 422
  with the report. The web layer never imports `lean`; tests build reports with `core.NewRulesetReport`. Flow `89-ruleset-report`
  drives all three at 360/768/1280 in light and dark; `/styleguide` shows a sample.

### Online validator

`/tools/validate` (`validator.go`, `pages/validator.gohtml`) is for every signed-in player. The form has three file inputs, each with its
`<label>`: **Battle Companies file** (required), **Vanilla rules file** and **Glossary** (optional, the hint names the shipped file used
when it is left empty). `POST /tools/validate` (multipart `battle_companies`, `vanilla`, `glossary`) runs `rulesets.CheckSet`, so the
Battle Companies file is checked against the vanilla file and glossary uploaded with it, and answers with the same report partial
(`alert-success` or 422 `alert-danger`, "Nothing was stored and no campaign was created.") plus a line saying which files were the
person's own and which the shipped ones. Nothing is written: the page never calls a `Store*` method. The result sits in
`#validator-result` (`aria-live="polite"`, `tabindex="-1"`, outside the form); htmx swaps its inner HTML and the page script focuses it
(`@htmx:after-swap`). Without JavaScript the form posts normally and the whole page comes back with the result. Limits: sign-in
required, 16 MB per file (422 "Choose .json files of at most 16 MB each."), 6 checks a minute per account (`validatorGate.allow`,
in memory like the login throttle) and 2 checks running at once (`validatorGate.acquire`, no queue); both answer **429** with a
`Retry-After` and a plain sentence (the layout's alert for an htmx request). Linked from the new-campaign form ("Not sure your file is
valid? Check it first", opens in a new tab so the form is kept), the admin Rules files page and the guide. Flow `97-online-validator`.
The page adds no component: it reuses the report partial, `card`, `field` and `alert`, so `/styleguide` is unchanged.

## Handing over a campaign (admin)

Rows with `AdminUser.Organises` have an "Organises N campaigns" expander listing each campaign (status, players) with a select of the campaign's members except the organiser (`Campaigns.Members`; "Name (Company)" or "Name (no company yet)") and **Hand over** (`POST /admin/users/{id}/handover`: `campaign`, `organiser`, `email`; `Campaigns.HandOver`, administrators need not be members).


See also: [matches.md](matches.md), [admin.md](admin.md), [customizing-rulesets.md](customizing-rulesets.md), [frontend-live-match.md](frontend-live-match.md), [frontend.md](frontend.md).
