# Several companies per player, and opening the last campaign

A player can run more than one Battle Company in the same campaign, and the app reopens the last campaign a player looked at.
This page describes both features for developers and organisers.

Related: [practice-matches.md](practice-matches.md) (games that award nothing), [matches.md](matches.md), [aftermath.md](aftermath.md), [demo-world.md](demo-world.md#two-bands), [frontend.md](frontend.md), [auth.md](auth.md).

Contents:

- [Several companies per player](#several-companies-per-player)
- [Rules and house rules](#rules-and-house-rules)
- [Persistence](#persistence)
- [Screens](#screens)
- [URLs](#urls)
- [Go interfaces](#go-interfaces)
- [Matches and fairness](#matches-and-fairness)
- [Opening the last campaign](#opening-the-last-campaign)
- [Demo world and tests](#demo-world-and-tests)

## Several companies per player

### How it works for a player

- The campaign has a setting, **Companies per player**, from 1 to 4. The default is 1.
- The creator sets it on the campaign creation form.
  The organiser and administrators can change it at any time, up or down.
- A player below the limit sees **Create another company** on the Overview.
- The rulebook assumes one company per player, so anything above 1 is a house rule that the organiser opts into.

A player with one company sees no change at all: no switcher, no new wording, whatever the limit is.

A player with two or more companies sees a **company switcher** under the campaign tab bar.
It appears on the pages that act on one of the viewer's own companies (My Company and the Armoury).
Each chip is a company; the current one is marked `aria-current="true"`.
A final chip, **New company**, shows while the limit allows another company.
Switching is a normal link, not an htmx swap, so the URL and the back button stay right.

At the limit, the Overview replaces the create control with "You have all N companies this campaign allows."

### Limits and names

- `CreateCompany` refuses with `core.ErrCompanyLimit` when the user already owns `Settings.CompanyCap()` companies in the campaign.
  The message is "you already have all the companies this campaign allows".
- Lowering the limit never removes or locks anything.
  A player above the new limit keeps their companies and cannot add more.
- A company name must be unique among the same owner's companies in the campaign, ignoring case.
  Otherwise the error is `ErrInvalidCompany`: "You already have a company with that name."
  Different owners may share a name.
- A finished campaign is read only for every company, and creating one is refused.
- Creating a company is permanent.
  Companies cannot be deleted, merged or retired, and a hero cannot move between companies.
  The company form says so.

### Ownership

Every owner-only action acts on the company in the URL.
These actions are recruiting, level-ups, treatment, equipment, the Armoury, formation, cemetery, renaming, the pack, paths, perks, wanderers and creatures.
They answer 404 when that company is not the user's own in that campaign.
A stranger, or an organiser probing another player's company id, learns nothing.

The organiser and administrators still cannot change a player's roster.
They keep only grants, waiving an aftermath and the match powers they had before.

## Rules and house rules

The rulebook does not cover two companies of one player.
The app resolves the open questions like this:

1. **Resources are per company.**
   Influence, gold, rerolls, the pack, experience, recruiting, treatment, level-ups, perks and pending choices live on the company, its units and its grants.
   There is no transfer between a player's companies.
2. **Rating is per company.**
   `Rank` sorts companies and Against the Odds compares two companies' ratings.
   Nothing aggregates by player, and there is no player total.
3. **Aftermath is per company and per match.**
   A user with two companies in a match has two aftermaths, each locked on its own.
   A company with an unfinished aftermath cannot start another match, but the player's other company can.
4. **Handover** moves the organiser role between members.
   Companies stay with their owners, and the handover list shows each member once.
5. **Ending a campaign** freezes everything.
   Rankings show companies, each with its owner's name, and there is no per-player winner.
6. **Closing an account** keeps its companies, shown as "Former player".
   Only the organiser's override can cover such an owner in an end-of-match confirmation.
7. **Two companies of one player may fight each other**, on the same side or on opposite sides.
   The match setup shows a non-blocking "Same player" note, and `ValidateSides` does not forbid it.
   The match counts like any other.
   For a game with no consequences, the note offers "Make it a practice match" ([practice-matches.md](practice-matches.md)).

## Persistence

Migration `00024_several_companies.sql` drops the constraint `companies_campaign_owner_key` (unique on campaign and owner) and adds the index `companies_owner_idx` on `(campaign_id, owner_id, id)`.
The down migration re-adds the constraint, so it fails when a player has two companies.
That refusal is intended.

- `companies_member_fk` stays, so a company's owner is always a member of the campaign.
- The limit is variable per campaign, so it is not a constraint.
  `Create` enforces it in one transaction:
  1. lock the owner's `campaign_members` row (`FOR UPDATE`);
  2. count the owner's companies;
  3. compare with the cap that core passes in;
  4. check the name (`lower(name)`);
  5. insert.
  The row lock serialises two simultaneous creates by one player.
- The setting is the key `companiesPerPlayer` in the campaign's `settings` JSON document.
  An absent key reads as 1, so old campaigns need no data migration.
- `List`, `Get` and `Members` aggregate the viewer's companies (ordered by id), so the campaign list keeps one row per campaign and the member list one row per member.
  The query count stays constant (`cmd/server/querycount_test.go` guards it).
- Matches, aftermaths, grants, ratings and the TTS export were already keyed by company id and needed no change.

## Screens

| Page | What it shows |
|---|---|
| Home (campaign list) | "Your companies" with the names, wrapping |
| Overview | "Your companies": one row per company (name, faction, points, units, W/D/L, Manage), then **Create another company** when allowed; "Choose your company" when none |
| My Company | The company in the URL, with the switcher; the heading is the company name |
| Armoury | The company named by `?company=ID` (default: the active one); the Buy forms carry a hidden `company` field |
| Rankings | Every company, the owner's name on each row, all of the viewer's rows highlighted |
| New company | Open while below the limit, titled "Create another company" from the second one; at the limit it redirects to My Company with a notice |
| New match | The search lists companies by name, owner or faction; the viewer's own are marked "Yours" and listed first (last on the other sides once one is placed); a company already in an open match is disabled |
| Match, live match, aftermath | The owner's name next to every company name |

The tab and sidebar keep the label "My Company".
When a player has several companies, titles, toasts and logs call a company by its name, not "your company".

The "Same player" note appears under a side holding two companies of one player ("Both are Ana's") and between sides when one player owns companies on both ("Same player: Ana plays both sides").

Layout rules are the usual ones: no horizontal scroll at 360 px, 44 px tap targets, semantic theme tokens only.
`/styleguide` shows the switcher.

## URLs

Owner routes carry the company id after `my-company`:

- `/campaigns/{id}/my-company/{company}` and everything below it: `recruit`, `units/{unit}/...`, `formation`, `cemetery`, `pack`, `rename`, `perks/{perk}/use`.
- The aftermath level-up button uses `/my-company/{company}/units/{unit}/levelup`.
- The Armoury page is `/campaigns/{id}/armory?company=ID`.
  Its POSTs (`/armory/buy`, `/armory/creature`, `/armory/wanderer`) carry the company in a hidden form field.

The id sits in the path so that htmx fragments and a second browser tab always act on the company shown on screen.
One helper, `ownCompany`, reads `{company}` and answers 404 unless it is one of the viewer's own companies.

Two GET redirects keep bookmarks working:

- `GET /campaigns/{id}/my-company` answers 303 to `my-company/{company}`.
  The company is the one in the `bcm-last` cookie when it is still the viewer's own, otherwise the viewer's oldest company.
  A player with no company sees the "Choose your company" page.
- `GET /campaigns/{id}/companies/{company}` for one's own company answers 303 to `my-company/{company}`.

POST URLs without a company id do not exist.

## Go interfaces

The web layer declares the `campaigns` interface it consumes (`internal/web/server.go`); core implements it.

Types and errors in `internal/core`:

```go
CampaignSummary.Companies []CampaignCompany // the viewer's own companies, oldest first
CampaignMember.Companies  []MemberCompany   // {ID int64; Name string}

CampaignSettings.CompaniesPerPlayer int      // JSON "companiesPerPlayer"
func (s CampaignSettings) CompanyCap() int   // 1..4; zero and out of range read as 1
const MaxCompaniesPerPlayer = 4

var ErrCompanyLimit = errors.New("you already have all the companies this campaign allows")

func (d CampaignDetail) Own(companyID int64) (CampaignCompany, bool)
func (d CampaignDetail) CanCreateCompany() bool // below the cap and the campaign is not finished
```

Every owner-only service method takes `companyID int64` right after the campaign id.
Example: `RecruitState(ctx, user, id, companyID)`.
They return `ErrNotFound` when the company is not the user's own, and `ErrRefused` for a finished campaign.
Internally, `companyDeps.ownCompany(ctx, user, campaignID, companyID)` finds the company in the campaign detail and checks that the campaign can be played.

Other entry points:

- `CreateCompany(ctx, user, id, in)` returns `ErrCompanyLimit` or `ErrInvalidCompany`.
  The repository's `Create` receives the cap in its input.
- `CampaignInput.Validate` rejects a `CompaniesPerPlayer` below 0 or above 4.
- `SetCompaniesPerPlayer(ctx, user, id, n)` is for the organiser or an administrator (`ErrRefused` for anyone else or a finished campaign, `ErrInvalidCampaign` out of range).
  It touches no company.
  The Overview shows the organiser a small form for it (`POST /campaigns/{id}/settings/companies`).
- `core.SamePlayers` finds the companies of one player in a match, for the "Same player" note.

## Matches and fairness

- Companies of one player may share a side or face each other.
  Ending a match counts one confirmation per company, and one owner confirms all of theirs with one tap.
- Secret objectives are written per company.
  A company is opposing for a voter who owns a company on another side, so a player with companies on two sides votes every company of the match, their own included.
- Against the Odds compares only matches of two sides with one company each.

## Opening the last campaign

### How it works for a player

- Opening the app, or signing in, lands on the last campaign the player viewed instead of the campaign list.
- The list is always one tap away at `GET /campaigns`: the brand link, "← Campaigns" in the campaign header and the sidebar's "Campaigns" all go there.
  `/campaigns` never redirects, so no loop is possible.
- A player with several companies in that campaign lands on the last company they opened there.
  The `my-company` alias reads the company from the cookie.
- Without JavaScript the behaviour is the same, because it is a server redirect.

### Rules

- `GET /` for a signed-in user with a valid cookie answers 303 to `/campaigns/{id}` (the Overview).
  Without a valid cookie it renders the campaign list.
- After sign-in, `/` applies the same rule unless `?next=` names another page.
- A full-page GET of any `/campaigns/{id}/...` page for a campaign the user belongs to refreshes the cookie.
  Fragments (htmx), event streams and POSTs never write it.
  Viewing one of the viewer's own company pages records the company too.
- A finished campaign is not remembered: no redirect, the list shows, and the cookie is cleared.
  A paused campaign still redirects.
- If the user can no longer open the campaign (`Get` returns `ErrNotFound`), the cookie is cleared and the list renders without an error page.
- A cookie that belongs to another user (a shared browser) is ignored and cleared.
- Signing out keeps the cookie, because it holds only ids.
  Signing in as someone else ignores it.

### The `bcm-last` cookie

| Item | Value |
|---|---|
| Name | `bcm-last`, with the same `bcm-` prefix as `bcm-theme` and `bcm-finished` |
| Value | `<userID>.<campaignID>` or `<userID>.<campaignID>.<companyID>`: positive base-10 integers, at most 18 digits each, at most 64 characters; anything else is dropped and cleared |
| Flags | `Path=/`, `SameSite=Lax`, `HttpOnly`, `Secure` from `cfg.SecureCookies` |
| Lifetime | 90 days, renewed on each write |
| Signed | No. The value is three ids and authorises nothing; every read validates it against the database (`campaigns.Get` checks membership and the company must be one of the viewer's own), so tampering can only land the user on a campaign they already belong to |
| Privacy | Numeric ids only: no name and no email. First party, never sent elsewhere |
| Written | By `loadCampaign` on a full-page GET, and only when the value changed |
| Read | `GET /` and the `my-company` alias |
| Cleared | Malformed, another user's, no longer the user's campaign or company, finished campaign, account closed |

The cookie is independent of the session cookie.
The session authenticates; this cookie is only a hint read afterwards.

The code is in `internal/web/lastvisit.go`: `parseLastVisit`, `rememberVisit`, `forgetVisit` and `lastCompany`.
It needs no change in core.
A redirecting `GET /` costs one extra `campaigns.Get`.

## Demo world and tests

The demo world covers the feature in the campaign "Two bands: several companies per player" (`cmd/server/world_bands.go`, [demo-world.md](demo-world.md#two-bands)).
It sets `CompaniesPerPlayer` to 3, gives several players two or three companies, and includes an ended match with two aftermaths and open matches.
Every other campaign keeps the default of 1, which guards backward compatibility.

Tests:

- Core: ownership of each service, the cap, name clashes, settings edge values, independence of two companies' resources, and `ValidateSides` with two companies of one owner.
- PostgreSQL (integration): the migration up and down, the atomic cap under concurrent creates, and the aggregated list queries.
- Web: the moved routes (404 for a foreign company, the 303 aliases), the switcher states, the Overview wording, and the cookie parsing and redirects.
- End to end: flows `98-several-companies` and `99-last-campaign` in `tests/e2e/flows`.
