# Several companies per player, and opening the last campaign directly (spec)

Status: **steps 1 to 6 and 8 of section 11 are built (backend, database, the company in the URL, the switcher, the setting, the match setup and notes, the demo world, the e2e flow `98-several-companies`); step 7 (the `bcm-last` cookie, `GET /campaigns`, the link changes, e2e flow `99-last-campaign`) is built too, as section 8 says, with one refinement: a foreign cookie is cleared (the task of issue #2), and a finished campaign is not remembered.** The decisions taken are in section 12 and applied throughout. The feature has two parts:
(1) a user can have more than one battle company in the same campaign; (2) a cookie remembers the last campaign viewed so the user lands on
it instead of the campaign list.

## 1. Where "one company per player per campaign" is assumed today

The assumption is enforced by the database and then built into the shape of almost every API, URL and template:

| Layer | Where | What it assumes |
|---|---|---|
| Schema | `00007_companies.sql`: `CONSTRAINT companies_campaign_owner_key UNIQUE (campaign_id, owner_id)` | The only hard guarantee. `companies_member_fk (campaign_id, owner_id) -> campaign_members` stays valid with several rows. |
| Postgres | `postgres/company.go` (`Create` maps the unique violation to `core.ErrHasCompany`); `postgres/campaign.go` `List`, `Get`, `Members` do `LEFT JOIN companies c ON c.campaign_id = ... AND c.owner_id = m.user_id` | One row per member or campaign. With several companies the joins multiply rows (home list, Get, Members) and `Scan` into a single `optionalCompany` is wrong. |
| Core types | `CampaignSummary.Company *CampaignCompany`, `CampaignMember.CompanyID int64`, comment "A user has at most one per campaign", `ErrHasCompany` | A single "the user's company". |
| Core services | `companyDeps.myCompany` / `myCompanyToChange` (`campaigns.go`) return `camp.Company.ID`. Used by `CompanyService` (create, rename, formation, cemetery), recruiting (`RecruitState`, `RollRecruit*`, `ConfirmRecruit`, `ChooseLower`, `RecruitSpecial`, `ChooseOptions`, `PendingChoice`, `PendingOfficers`, `ArrangeOfficers`), levelling (`LevelUpState`, `RollLevelUp`, `ChooseLevelUp`, `RerollLevelUp`), treatment (`TreatmentState`, `TreatInjury`), armoury (`BuyEquipment*`, `BuyCreature`, `HireWanderer`, `DismissWanderer`, `Carry`, `ChooseModifierWeapon`, `DropEquipment`), paths (`PathOptions`, `ChoosePath`), `RenameUnit`, `UsePerk` (aftermath_perks.go). Their signatures are `(ctx, user, campaignID, ...)`: no company id. `CreateCompany` refuses with `ErrHasCompany` when `camp.Company != nil`. | The company is implied by (user, campaign). About 30 methods of the web `campaigns` interface (`internal/web/server.go`) change. |
| URLs | `/campaigns/{id}/my-company` and everything under it (about 35 routes in `server.go`: recruit, formation, cemetery, pack, units/{unit}/levelup, treat, path, equipment, dismiss, rename, perks/use), `/campaigns/{id}/armory*` (`POST armory/buy`, `creature`, `wanderer`), `/campaigns/{id}/company/new` | The owner's company has no id in the URL. The aftermath page reuses `/my-company/units/{unit}/levelup` (`aftermath.gohtml`), so it inherits the problem. |
| Web handlers | `loadCampaign` then `base.Campaign.Company` in `campaign.go` (`myCompanyPage`, `handleCompany` redirect of an own company to `/my-company`, `handleNewCompanyPage` redirects to `/my-company` when one exists), `company_actions.go`, `levelup.go`, `creatures.go`, `armory.go`, `aftermath.go` (`UsePerk`, `IsOwn` check at line 319), `live.go` (`viewerSide`, `liveFragmentView` read-only test), `live_log.go`, `live_sheet.go`, `live_actions.go` | "Mine" is one pointer. |
| Templates | `partials/campaign.gohtml` tab bar ("My Company"), `campaign_overview.gohtml` ("Your company", "Manage company", "Each player has one company per campaign"), `campaign_companies.gohtml` and `campaign_rankings.gohtml` ("Yours" badge and row highlight by `Campaign.Company.ID`), `campaign_armory.gohtml`, `campaign_my_company.gohtml` ("Create my company"), `match_new.gohtml` and `campaign_matches.gohtml` ("Each player creates a company from My company", "A match needs at least two companies"), `partials/sidebar.gohtml` ("My Company"), `home.gohtml` card | Wording and links assume one. |
| Already company-id keyed (no change) | Matches (`match_side_companies PK (match_id, company_id)`), `ValidateSides`, `Match.Access`, `LiveAccessFor`, `AftermathAccessFor`, `ATOAccessFor` (owner compared per company), live service (`Live(user, campaign, company)`), aftermath service and its routes `/aftermath/{company}`, grants `/companies/{company}/grants`, TTS export `/companies/{company}/tts`, `TTSExport(campaign, company, ...)`, ratings and rankings (`Rank` sorts companies, not players), end-of-match confirmation (docs/matches.md already says "an owner of several companies confirms all of them with one tap"), `match_aftermaths`, `grants`, `company_perks`, `fallen_units` | These were designed company-first. They only need a test that an owner with two companies behaves. |
| Demo world | `cmd/server/world*.go`: every scripted player has exactly one `CreateCompany` per campaign; `docs/demo-world.md` table says "owner of the company" | Needs one campaign that shows the feature (section 9). |
| E2E | `tests/e2e/flows/*.mjs` visit `/campaigns/${c}/my-company` | Keep working through the alias (section 5.3). |

Not assumed anywhere: companies cannot be deleted (no `DELETE FROM companies` anywhere) and campaigns cannot be deleted by users either.
That matters for the cap (a mistaken second company stays for good) and for the cookie (a remembered campaign only disappears by losing
access, which today cannot happen except by closing the account).

## 2. Feature 1: several companies per player

### 2.1 User stories

- As a player I can create another company in a campaign I already play in, up to the number the organiser allows, so I can field an
  Easterling band and a Rohan band in the same campaign.
- As a player I can see all my companies of a campaign at a glance and switch between them without losing my place.
- As a player I choose which of my companies fights when I set up a match, and I can tell my companies apart everywhere.
- As the campaign creator I decide how many companies each player may have (default 1, as today), and whoever may edit the campaign settings can change it later.
- As an opponent or spectator I see the owner's name next to each company so I know two companies belong to the same person.

### 2.2 Acceptance criteria

- [ ] A campaign has a setting "Companies per player" (1 to 4, default 1), set by the creator on the creation form and editable at any time by
  those who may change the campaign (organiser, administrators). Lowering it only blocks new creations: companies above the new limit stay
  and keep playing. With 1, and for any player who owns a single company, nothing changes: no switcher, no new wording (the legacy URLs
  still work, section 5.3).
- [ ] `CreateCompany` refuses with `core.ErrCompanyLimit` (message "You already have the 2 companies this campaign allows.") when the user
  owns `Settings.CompanyCap()` or more companies there (a lowered cap counts the same way: a player above it keeps theirs and cannot add). The check and the insert are atomic (two simultaneous creates never exceed the cap).
- [ ] A company name must be unique among the same owner's companies of the campaign, ignoring case (`ErrInvalidCompany`, "You already have a
  company with that name."). Different owners may still share a name (today's behaviour).
- [ ] My Company shows one company at a time, chosen by the URL. When the player has two or more, a company switcher of chips sits under the tab bar
  (section 4.1). When they have one, the page is as today (no chip, no change), whatever the limit.
- [ ] Every owner-only action (recruit, level-up, treat, equipment, armoury, formation, cemetery, rename, pack, path, perks, wanderers,
  creatures) acts on the company in the URL, and answers 404 when that company is not the user's own in that campaign (never "someone
  else's company": a stranger learns nothing).
- [ ] The organiser and administrators still cannot change a player's company roster (only grants, aftermath waive and the match powers they
  have today).
- [ ] A match can have several companies of one player on the same side (a "2 vs 1" with your own two) and on different sides (section 7.1
  says whether that is allowed); the match page, live match, end confirmation, aftermath and ATO treat each company separately.
- [ ] Overview lists "Your companies" (one row each with Manage), or "Choose your company" + "Create my company" when none.
- [ ] Companies and Rankings show every company; "Yours" marks all of the viewer's companies; the owner name is shown on every row.
- [ ] A player below the cap sees "Create another company" on the Overview (and a "+ New company" chip once the switcher is shown); at the cap the control is replaced by the
  plain text "You have all N companies this campaign allows."
- [ ] A finished campaign is read only for every company as today (`CanPlay`); creating is refused.
- [ ] Mobile 360 px: no horizontal scroll; the switcher is a horizontally scrolling row of chips or a select (section 4.1); 44 px targets.
- [ ] Light and dark, `/styleguide` updated with the switcher.
- [ ] Existing data and campaigns behave exactly as before after the migration; `make test`, `make test-integration`, `make e2e` pass.

### 2.3 Rules questions and answers

Answers come from `definitions/` and `docs/`; the rulebook says nothing about two companies of one player in one campaign, because the
game mode assumes one company per player. Everything below is therefore a house rule that the organiser opts into (setting default 1).

1. **May two companies of the same player fight each other?** Decided: yes, through the normal match flow, with a non-blocking
   "Same player" note on the setup (and on the match page). It is the player testing a build, or a two-person group using several bands.
   Nothing in the data or in `ValidateSides` forbids it, and the live match, aftermath, grants and rating all use company ids. Such a match
   counts like any other (rating farming by one player is visible to the organiser in the rankings; the practice match, see
   [practice-match-spec.md](practice-match-spec.md), is the way to play without consequences).
2. **Influence, gold, XP.** Per company, as today. They live on the `companies` row (`influence`, `gold`, `rerolls`, `pack`), on
   `company_units` and in `grants`; no resource is shared between a player's companies and there is no transfer (a transfer would let a
   player pool resources; not requested, not in the rulebook). Recruit, treatment, level-up, perks and `Pending` state are per company.
3. **Rating.** The Rating (points) is per company: `Rank` sorts companies, and Against the Odds compares two companies' ratings
   (`ATOAccessFor` is per company). Nothing aggregates by player. The rankings stay a company ranking; an optional "Player total" is out of
   scope (decision 7).
4. **Caps set by the organiser.** One new campaign setting `CompaniesPerPlayer` (int, 1 to 4, zero value read as 1). Settings are an
   opaque JSON document with `"v": 1` (`campaigns.settings`), so a new key needs **no migration**; old campaigns read it as absent = 1.
   The other settings are immutable after creation; this one is editable at any time (up or down) by those who may change the campaign.
   Lowering it never removes or locks anything: it only refuses new creations while the player owns that many or more.
5. **Aftermath.** Already one aftermath per company per match (`match_aftermaths PK (match_id, company_id)`); a user with two companies in
   a match has two aftermaths, each locked independently (`CompanyAftermaths` locks only that company). The match page lists both. The
   company that has not finished its aftermath cannot start another match, but the player's other company can.
6. **Handover.** Organiser handover moves the `organiser` role between members (`campaign_members`); companies stay with their owners. The
   handover list (`Members`) must list each member once (the join currently yields one row per company: aggregate).
7. **Campaign finish and ranking.** Ending the campaign freezes everything; the Rankings tab shows companies. With several companies of one
   owner the page shows each with the owner name; there is no per-player winner (decision 7).
8. **Account closure.** `CloseUser` keeps companies and shows "Former player" for each; unchanged. A closed owner's several companies can
   only be covered in an end-of-match by the organiser override, as today.
9. **A hero cannot move between companies**, no company merge, no company deletion, no retiring: out of scope (decision 6). The company form says that
   creating a company is permanent.

## 3. Persistence impact

Migration `00024_several_companies.sql`:

```
-- +goose Up
ALTER TABLE companies DROP CONSTRAINT companies_campaign_owner_key;
CREATE INDEX companies_owner_idx ON companies (campaign_id, owner_id, id);
-- +goose Down
-- refuses when a player has two companies: re-adding the unique constraint would fail, and that is the right answer.
ALTER TABLE companies ADD CONSTRAINT companies_campaign_owner_key UNIQUE (campaign_id, owner_id);
DROP INDEX companies_owner_idx;
```

- `companies_member_fk` (composite FK to `campaign_members`) stays: it is what keeps a company's owner a member.
- The cap cannot be a plain constraint (it is per campaign and variable). `Create` enforces it in one transaction: `SELECT ... FROM
  campaign_members WHERE campaign_id=$1 AND user_id=$2 FOR UPDATE`, then `count(*)` of the owner's companies, compare with the cap passed by
  core, then insert. The member row lock serialises concurrent creates by one player. No trigger.
- Name uniqueness per owner is checked in the same transaction (`lower(name)`), no unique index: adding one over existing data could fail
  on historic duplicates and a case-insensitive per owner unique index adds little (two owners may share names).
- No new column and no data migration: ids, URLs of companies (`/companies/{company}`), matches and aftermaths are unchanged.
- `ErrHasCompany` is replaced by `ErrCompanyLimit` (keep `ErrHasCompany` only if the limit is 1? No: one error, the message states the number).
- Query changes (database master): `List`/`Nav` aggregate the viewer's companies (`array_agg` or `jsonb_agg` ordered by company id) so the
  campaign list keeps one row per campaign; `Get` returns the viewer's companies as a slice; `Members` aggregates names; `Companies` and
  `Company` are unchanged. Keep `ReadRoster` as is.
- Performance: `figures`/`ratedAll` already read the campaign's rosters once; the home list rates the viewer's companies per campaign,
  keep it a constant number of queries (`querycount_test` in `cmd/server` guards this).

## 4. UX

### 4.1 The company switcher (new, campaign pages)

Appears under the campaign tab bar on the pages that act on one of the viewer's companies (My Company, Armory) only when the viewer owns
two or more companies. A player with a single company sees no chip and no change, whatever the limit.

- 360 px: a row of chips (one per company, name truncated with title, current one `aria-current="true"`, faction as the second line only
  from 640 px), horizontally scrollable, plus a final chip "+ New company" when allowed. Switching is a normal link (full navigation to
  `/campaigns/{id}/my-company/{company}`), not an htmx swap, so the URL, the back button and the cookie stay right.
- 768 px and up: same chips, wrapping.
- States: no company or one company: not shown (a player with one company discovers the feature through "Create another company" on the
  Overview). Two or more: shown, with a final "+ New company" chip while below the limit; at or above it: no "+" chip, tooltip text
  "All N companies created" (above it after a lowered limit).
- A chip shows a small dot when that company has something waiting (an aftermath to finish, a level-up, a recruit in progress, officers to
  arrange): the data exists (`CompanyDetail.Aftermaths`, `PendingChoice`, `OfficersPending`); for the switcher it needs a cheap
  `CompanyAttention` summary (section 6, optional, a second step).

### 4.2 Page by page

| Page | Today | Change |
|---|---|---|
| Home (campaign list) | card shows "Your company: X" | "Your companies: X, Y" (names, wrap, max 2 lines then "+n"); route moves to `/campaigns` (section 8.3) |
| Sidebar | "My Company" under the open campaign, "Campaigns" | "My Company" goes to the active company (alias URL); with several, a small submenu is not added (the switcher does that) |
| Campaign header tabs | "My Company" | label stays "My Company" with one company, becomes "My Companies" only on the Overview card; the tab itself stays "My Company" (decision 5: keep the tab label, the switcher carries the plural) |
| Overview | "Your company" card with Manage company, or "Choose your company" and "Each player has one company per campaign" | "Your companies" card: one row per company (name, faction, Points, Units, W/D/L, Manage), then "Create another company" when allowed; copy "Each player can have up to N companies in this campaign." (N from settings); the "one company" sentence only when N = 1 |
| New company page | redirects to My Company when one exists | open to create while below the cap; title "Create another company" for the 2nd+; same form; no change otherwise. At the cap it redirects to My Company with a notice |
| My Company | the single company | the company in the URL; heading = company name; the switcher; the rename form unchanged |
| Armory | acts on the viewer's company | acts on `?company=ID` (default the active one); the switcher is shown; the Buy forms carry a hidden `company` |
| Companies tab | list with "Yours" badge | "Yours" on each of the viewer's companies; owner name always shown |
| Rankings | highlight own row | highlight all own rows |
| Matches tab / match setup | "Each player creates a company from My company" | wording "Each player creates at least one company..."; the setup form (`match_form.js`) lists campaign companies searchable by name, owner or faction; add a "Mine" filter chip and group "Your companies" first; a company already in an open match is disabled with the link as today (`CompanyBusyError`) |
| Match page, live match, aftermath | company per row; owner name shown | owner name shown next to every company name (already available as `CampaignCompany.Owner`); in the aftermath a "My Company" link goes to that company, not to the alias |
| TTS export | button on My Company and company page | unchanged (company keyed); the copy button sits on the company in view |
| Join page (invite) | joins the campaign, then "Create my company" | unchanged |

### 4.3 Match setup, opponent selection

The new-match form already picks companies from the whole campaign into sides; nothing limits a player to "their" company. Changes:
a "Mine" quick filter, the owner shown on each result, and a non-blocking "Same player" note: under a side that holds two companies of one
player ("Both are Ana's"), and between the sides when one player owns companies on both ("Same player: Ana plays both sides"). Nothing is
refused; `ValidateSides` is unchanged.

### 4.4 Naming

- A company is always called by its name (never "your company") in titles, toasts and logs when the viewer has several; with one company
  the old wording remains. `CompanyDetail.Name` is used in page titles: `<Company> · <Campaign> · ...` already when read only; make it so
  for the owner too when they have several.
- The tab and sidebar keep "My Company". Overview card heading: "Your company" (1) or "Your companies" (2+).

## 5. URL design

### 5.1 Company-scoped owner routes

Insert the company id after `my-company`:

`/campaigns/{id}/my-company/{company}` and every action under it:
`/campaigns/{id}/my-company/{company}/recruit/...`, `/units/{unit}/...`, `/formation`, `/cemetery`, `/pack`, `/rename`, `/perks/{perk}/use`.
Armory: `/campaigns/{id}/armory?company=ID` for the page; the POSTs carry a hidden form field `company`
(`/armory/buy`, `/armory/creature`, `/armory/wanderer`). The aftermath level-up button uses `/my-company/{company}/units/{unit}/levelup`.

Reasons: the company is a real resource in the path (like `/companies/{company}`), one `PathValue("company")` read, and the handler
verifies ownership once in a shared helper. Alternative: put the id in a hidden field or the cookie for actions. Rejected: htmx fragments
and a second tab would then act on a different company than the one on screen, a dangerous mismatch with real resources.

### 5.2 Ownership check helper (web)

One helper, used by all owner routes, instead of the 30 `base.Campaign.Company == nil` checks:

```
func (s *Server) ownCompany(w http.ResponseWriter, r *http.Request, base campaignBase) (core.CampaignCompany, bool)
// reads {company}; 404 unless it is one of base.Campaign.Companies (the viewer's own).
```

### 5.3 Legacy and alias URLs

- `GET /campaigns/{id}/my-company` (no company): 303 to `/campaigns/{id}/my-company/{company}` where company is the cookie's company when
  it is one of the viewer's, else the viewer's oldest company; with no company it renders the empty "Choose your company" page as today.
  Bookmarks, e2e flows and sidebar keep working.
- `GET /campaigns/{id}/companies/{company}` with an own company: 303 to `/my-company/{company}` (replaces the redirect to `/my-company`).
- Old `POST /my-company/...` URLs without a company are not kept (POSTs are never bookmarked); a stale page open during the deploy gets
  404 and the page reload fixes it. Mention in the release notes.

## 6. Go interfaces (contract)

The web layer's `campaigns` interface (`internal/web/server.go`) changes as follows. `core` implements the services; the web layer declares what it consumes.

Types (core):

```go
// CampaignSummary.Company becomes:
Companies []CampaignCompany // the viewer's own companies, oldest first; empty until they create one

// CampaignMember.CompanyID/CompanyName become:
Companies []MemberCompany // {ID int64; Name string}

// CampaignSettings gains (JSON key "companiesPerPlayer", zero value = 1):
CompaniesPerPlayer int
func (s CampaignSettings) CompanyCap() int // 1..4, 0 and out of range read as 1; Validate rejects >4 on create

var ErrCompanyLimit = errors.New("you already have all the companies this campaign allows") // replaces ErrHasCompany

// CampaignDetail gains the convenience:
func (d CampaignDetail) Own(companyID int64) (CampaignCompany, bool)
func (d CampaignDetail) CanCreateCompany() bool // below the cap and the campaign is not finished
```

Interface methods that gain `companyID int64` right after `id` (campaign id), semantics unchanged, plus the new error: `ErrNotFound`
when the company is not the user's own in that campaign (so a stranger or an organiser probing another player's id learns nothing),
`ErrRefused` for a finished campaign (`CanPlay`) as today:

`LevelUpState, RollLevelUp, ChooseLevelUp, RerollLevelUp, BuyCreature, HireWanderer, DismissWanderer, Carry, SaveFormation,
SaveCemeteryOrder, BuyEquipment, BuyEquipmentFor, ChooseModifierWeapon, TreatmentState, TreatInjury, RenameUnit, RenameCompany,
PathOptions, ChoosePath, DropEquipment, RecruitState, RollRecruit, RollRecruitWithPerk, ConfirmRecruit, ChooseLower, RecruitSpecial,
ChooseOptions, PendingChoice, PendingOfficers, ArrangeOfficers`. `UsePerk(ctx, user, id, companyID, perkID)` already has it.

Example: `RecruitState(ctx context.Context, user core.User, id, companyID int64) (*core.RecruitRoll, error)`.

`CreateCompany(ctx, user, id, in)` keeps its signature. New errors: `ErrCompanyLimit`; `ErrInvalidCompany` joined with "You already have a
company with that name." Core implementation hint: `companyDeps.ownCompany(ctx, user, campaignID, companyID)` replaces `myCompany` /
`myCompanyToChange`: reads `campaigns.Get`, finds the company in `camp.Companies`, then `CanPlay` for the changing variant. No repository
interface change for these; `companyRepository.Create` gains `cap int` (or a `NewCompany.Cap`): "ErrCompanyLimit when the owner already has
`cap` companies there; ErrInvalidCompany when the name is taken by another of the owner's companies".

Optional (a second step, the switcher dots):

```go
// CompanyAttention says what waits for the player in each of their companies of the campaign, in one call.
CompanyAttention(ctx context.Context, user core.User, id int64) (map[int64]core.Attention, error)
type Attention struct{ Aftermath, LevelUps, Recruit, Officers, Treatment bool }
```

Settings creation: `CampaignInput.Settings.CompaniesPerPlayer` (checked by `CampaignInput.Validate`; >4 or <0 is `ErrInvalidCampaign`).
Editing later (decision 3): `SetCompaniesPerPlayer(ctx, user core.User, id int64, n int) error` (organiser or administrator, any `n` from 1 to 4,
up or down; `ErrRefused` for anyone else or a finished campaign, `ErrInvalidCampaign` out of range). Lowering touches no company. The Overview
of the organiser (and the campaign settings area) shows a small form with the number input and Save.

The last-campaign cookie adds one more consumer (section 8.4):
```go
// web side only: campaigns.Get(ctx, userID, id) is the validity check; no new core API is needed.
```

## 7. Questions on matches and fairness

7.1 Same owner on opposite sides: allowed with the "Same player" note (rule 1, decision 2). 7.2 Same owner on the same side: allowed (the only way a solo player can use
a small campaign); `EndAccessFor` and confirmation already count one confirmation per company and let one owner confirm all. 7.3 Visibility
of secret objectives: `secret.go` writes per company for `co.OwnerID == user.ID`: with two companies of one player on the same match the
card lists both; verify the vote rule "a user who owns at least one company in the match votes only for companies of the other teams"
(live-match.md) still reads right when the voter owns companies on both sides (now possible: the voter votes only for companies of the other teams than any of their own, and abstains when none remains). 7.4 Against the Odds compares
only matches of two sides with one company each: unchanged.

## 8. Feature 2: open the last campaign directly

### 8.1 User stories and acceptance criteria

- As a player with a campaign I use every week, I open the app (or sign in) and land on that campaign, not on the list.
- As a player I can still reach the campaign list with one tap.

- [ ] `GET /` for a signed-in user with a valid last-campaign cookie answers 303 to `/campaigns/{id}` (the Overview). Without a cookie, or
  with an invalid one, it renders the campaign list as today.
- [ ] The list is reachable at `GET /campaigns` and by the "Campaigns" links: the brand link in the header, "← Campaigns" in the campaign
  header, the sidebar's "Campaigns". None of them redirects. No redirect loop is possible: `/campaigns` never redirects.
- [ ] After sign-in the user lands on the last campaign (the post-login target `/` redirects) unless `?next=` names another page.
- [ ] A visit to any `/campaigns/{id}/...` page that renders in full (not htmx fragments, not SSE, not POST) refreshes the cookie.
- [ ] Viewing a company page (`/my-company/{company}` or `/companies/{company}` of the viewer's own company) records the company too; the
  alias `/my-company` and the switcher use it.
- [ ] If the remembered campaign is finished, no redirect (the user sees the list; finished campaigns are hidden there unless toggled) and
  the cookie is cleared. A paused campaign still redirects.
- [ ] If the user can no longer open the campaign (`Get` returns `ErrNotFound`), the cookie is cleared and the list renders, no error page.
- [x] A cookie of another user (shared browser) is ignored and cleared (decision of issue #2: the user id inside must be the session user's).
- [ ] Sign out keeps the cookie (it holds only ids); signing in as someone else ignores it.
- [ ] 360/768/1280, no visual change except the redirect; the home page shows nothing new. Optionally the list highlights the remembered
  campaign with a "Last opened" chip (cheap, helps a user who came by the brand link).
- [ ] A player with several companies in the remembered campaign lands on the last company they opened there: the alias
  `/campaigns/{id}/my-company` reads the company id of the cookie (valid and still the viewer's), else their oldest company. With one company
  nothing changes.
- [ ] A user without JavaScript behaves the same (server redirect only).

### 8.2 Cookie design

| Item | Decision |
|---|---|
| Name | `bcm-last` (same `bcm-` prefix as `bcm-theme`, `bcm-finished`, `bcm-glossary`) |
| Value | `<userID>.<campaignID>` or `<userID>.<campaignID>.<companyID>`, base-10 integers. Parsed strictly (`^\d{1,18}(\.\d{1,18}){1,2}$`), anything else is dropped and cleared |
| Path / flags | `Path=/`, `SameSite=Lax`, `HttpOnly` (only the server reads it: the redirect and the alias; no script needs it), `Secure` from `cfg.SecureCookies` like the other cookies |
| Expiry | `MaxAge` 90 days, renewed on each write (sliding). The theme cookie lives a year; a campaign pointer going stale is cheaper to forget sooner |
| Signed? | No. The value is three ids, authorises nothing, and is validated against the database on every read (`campaigns.Get(user, id)` is membership-checked, company must be in `Campaign.Companies`). Tampering can only make the user land on a campaign they already belong to. A signature would add key management for no protection |
| Privacy | Contains numeric ids only, no names, no email. Does not identify the person without the database. Cleared on account close (the close handler clears it with the session). Not a tracking cookie: first party, same site, not sent to third parties. Mention it in the privacy note if one exists (none today: grep found no cookie notice) |
| Written when | a full-page GET of any `/campaigns/{id}/...` page for a campaign the user is in, via `loadCampaign` (one place); only if the value changed, to avoid a Set-Cookie on every view. Never from htmx/SSE/POST. Never from `/join/{token}` before joining, but yes right after a successful join (lands on the campaign anyway) |
| Read when | `GET /` (redirect), `GET /campaigns/{id}/my-company` (alias company choice) |
| Cleared when | invalid or foreign campaign for this user (`ErrNotFound`), finished campaign, malformed value, account closed. Not cleared on logout, not cleared when a different user signs in (ignored instead, via the user id in the value) |
| Interplay with `bcm_session` | independent. The session cookie authenticates; this one is only a hint read after authentication (`protected`). When the session expired the user goes to `/login?next=...` as today; after login `/` applies the hint |
| Interplay with `bcm-theme` | none. Same attributes and prefix; the theme cookie is read before auth for the first paint, this one only after auth |
| Interplay with `bcm-finished` | the list toggle is independent; the redirect ignores it |
| Campaign closed/deleted/user removed | no campaign or company deletion exists; if added, `Get` fails with `ErrNotFound` and the rule above clears it |

### 8.3 Routes

- `GET /{$}`: if signed in and the cookie is valid -> 303 to `/campaigns/{id}`; otherwise the list. Done in `handleHome` (the existing
  `showFinished` cookie logic stays).
- `GET /campaigns` (new, protected): the list, exactly today's `handleHome` body without the redirect. Add it before
  `GET /campaigns/new` in the mux (the more specific `/campaigns/new` still wins).
- Header brand, sidebar "Campaigns" and "← Campaigns" link to `/campaigns`. The brand link goes to `/campaigns` for signed-in users and
  to `/` (login) for visitors; or keep brand at `/` and rely on a one-time `?all=1`? Rejected: a query escape hatch is easy to forget.
- `loginURL`/post-login: `next` of `/` or empty -> `/`, which redirects. No change.

### 8.4 Go interface

```go
// internal/web (no core change): the helpers
type lastVisit struct{ UserID, CampaignID, CompanyID int64 }
func parseLastVisit(r *http.Request) (lastVisit, bool)          // strict parse of bcm-last
func (s *Server) rememberVisit(w http.ResponseWriter, r *http.Request, userID, campaignID, companyID int64) // Set-Cookie only on change
func (s *Server) forgetVisit(w http.ResponseWriter)             // MaxAge -1
// handleHome uses the existing campaigns.Get(ctx, userID, campaignID): ErrNotFound -> forgetVisit and render the list.
// CampaignDetail.Status == finished -> forgetVisit and render the list.
```

One extra read per `GET /` visit that redirects (a `Get`); the redirect target then loads normally. No new query for the list.

## 9. Demo world impact (`cmd/server/world*.go`, `docs/demo-world.md`)

- New campaign "Two bands" (or extend an existing quiet one): setting `CompaniesPerPlayer = 3`; Éowyn owns two companies (Rohan and
  Riders of the Mark? any two factions), Galadriel owns two, Gimli owns one. A match with two of Éowyn's companies on the same side vs
  Sauron; one ended match with both aftermaths (one done, one pending) to show independent locks; one open match with a company in it and
  the other free. A player at the cap and a player below it.
- Existing campaigns stay at cap 1 (guards backward compatibility). `docs/demo-world.md` gets a row and the "Where to click".
- The cookie needs no demo data; the e2e tests set it by visiting.
- `querycount_test.go` in `cmd/server` guards the number of queries of the home list and the campaign pages: update the expectation only
  if the aggregation adds a query (it should not).

## 10. Test and e2e plan

Core (mocks regenerated with `make generate`): table tests for `ownCompany` (own, another player's, other campaign, finished, organiser
probing); `CreateCompany` cap (below, at, concurrent via the repository test), name clash per owner, cap 1 default for absent settings;
settings `CompanyCap` edge values; every service whose signature changed has one test with two companies of one owner proving they do not
share influence, gold, pending recruit, level-up roll or perks; `ValidateSides` with two companies of one owner (same side, opposite sides
now allowed); end confirmation: one owner, two companies, one tap; `AftermathAccessFor` / `LiveAccessFor` per company.
Postgres (integration): migration up/down (down refuses with two companies), `Create` atomicity under concurrency (two goroutines, cap 2,
three creates: exactly one fails with `ErrCompanyLimit`), `List`/`Get`/`Members` return one row per campaign/member with ordered
companies, rankings unchanged.
Web: handler tests for each moved route (404 for a company that is not the caller's, 303 alias, 303 own company redirect), template state
tests for the switcher (0, 1 with cap 1, 1 with cap 3, 3 at cap, finished), Overview card, "Yours" marks, wording at cap 1, 360 px
no-scroll assertions in the existing e2e helpers; cookie tests: parse table (valid, malformed, other user), `GET /` redirect, invalid
campaign clears, finished clears, `/campaigns` never redirects, htmx request does not set the cookie, `Secure` flag from config.
E2E (`tests/e2e/flows`, new `97-several-companies.mjs`, `98-last-campaign.mjs`): create two companies, cap message on the third, switch,
recruit in one while the other is unchanged, play a two-company match through aftermath, owner confirms both ends with one tap; visit
campaign, go to `/` -> lands on it, brand link -> list, sign out/in as another user -> list. Existing flows that use `/my-company` keep
passing through the alias. `make test`, `make test-integration`, `make e2e`, `python3 tools/validate_definitions.py` is not needed (no
rules data read).

## 11. Implementation order

1. Backend and database: migration 00024 and `Create` with cap/name check, queries aggregated. **Done.**
2. Backend: `CampaignSettings.CompaniesPerPlayer`, `ErrCompanyLimit`, `ownCompany`, signature change of the 30 service methods, mocks. **Done**
   (also `Campaigns.SetCompaniesPerPlayer`, `CampaignDetail.Own/Owns/CanCreateCompany`, `core.SamePlayers`). The web layer compiles against
   the new interface through one documented seam, `myCompanyOf` / `seamCompany` / `campaignBase.Mine` in `internal/web/campaign.go`
   (the viewer's first company): step 3 replaces it with the company of the URL; nothing changes for a player with one company. The
   `CompanyAttention` summary of section 6 is not built.
3. Frontend: web interface and routes `my-company/{company}`, `ownCompany` helper, alias redirects. **Done** (the alias opens the first company until the cookie exists).
4. Frontend: Overview/Companies/Rankings/Home wording and multi rows; campaign creation form field and the settings edit form (`SetCompaniesPerPlayer`). **Done.**
5. Frontend: company switcher partial + `/styleguide`, New company page rules. **Done** (the attention dots of 4.1 are not built).
6. Frontend: match setup "Mine" filter and same-player note; "Same player" note. **Done.**
7. Frontend: `bcm-last` cookie, `GET /campaigns`, link changes. **Done** (`internal/web/lastvisit.go`; e2e `99-last-campaign`; the alias opens the last company opened).
8. Qa: demo world campaign ("Two bands", `docs/demo-world.md`), e2e flow `98-several-companies`, docs (`frontend.md`, `architecture.md`, `demo-world.md`, `testing.md`). **Done** (the `auth.md` cookie row came with step 7; `matches.md` never mentions the list URL).

## 12. Decisions

All eight are applied in the sections above.

1. **Number of companies per player.** A campaign setting, 1 to 4, default 1, set by the campaign creator on the creation form. Existing
   campaigns and anyone who does not touch it see no change.
2. **Two companies of one player fighting each other.** Allowed and counted like any other match, with a "Same player" note on the setup and
   the match page. No `ValidateSides` rule. The note offers "Make it a practice match" ([practice-match-spec.md](practice-match-spec.md)), which ticks the
   box of the form so that the fight awards nothing and leaves no aftermath.
3. **Changing the limit later.** Editable at any time by those with permission (organiser, administrators), up or down. Lowering only
   affects new creations: existing companies stay, keep playing, and nothing is deleted.
4. **Company ids in the URL of every owner action** (`/my-company/{company}/...`). The GET alias keeps bookmarks; old POST URLs stop working at
   upgrade.
5. **Switcher and wording.** Chips under the tab bar, tab stays "My Company", only the Overview heading turns plural. The switcher does not
   appear for a player with a single company: no visible change for them today.
6. **Retiring a company.** Out of scope; creating a company is permanent and the form says so.
7. **Rankings by player.** A company ranking only, owner name on each row. No player total.
8. **Last-campaign memory.** Cookie `bcm-last` only (no column on `users`), unsigned ids validated on read, landing on the campaign
   Overview; with several companies in that campaign the "My Company" alias opens the last company the player opened.
