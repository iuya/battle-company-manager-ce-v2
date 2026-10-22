# Administration

Status: **built** (`Auth.ListUsers`, `Auth.SetUserPermissions`, `Rulesets.ListWithUsage/SetDefault/Delete/PruneUnused`, `Campaigns.HandOver`, with Postgres and tests; pages in `internal/web/admin.go` and `templates/pages/admin_*.gohtml`). `/admin` is superusers only, a 404
for everybody else (`requireSuperuser`). This document is the spec of the section and of the Go methods it needs.
Deviations: the upload card posts the file to `POST /admin/rulesets`, which hashes it on the server (no browser hashing; the
file is at most 16 MB and a superuser uploads rarely), and the download is `GET /admin/rulesets/{hash}/download`
(`Rulesets.Download`), because `GET /rulesets/{hash}` is only the picker's lookup. `Rulesets.StoreAs` records the uploader
(the campaign form uses it too). The Users row is redrawn by looking the account up again with the email the form carries. Bead `j62`; the implementer beads are
listed at the end.

The section has two pages under one sub-navigation (tabs on a phone, a row of links on desktop): **Users**
(`/admin/users`) and **Rules files** (`/admin/rulesets`). `/admin` redirects to `/admin/users`.
Everything is superuser only. Roles keep their meaning (`core.Role`): a plain `user` plays, an `admin` also creates campaigns and
edits any match or aftermath, a `superuser` additionally sees this section.

Layout rules are those of [frontend.md](frontend.md): one column at 360 px, tables become card lists, theme tokens only, every
action an htmx form that swaps its own row or card, 422 with the form kept on error, 404 for non-superusers (also for the POSTs).

## Users (`/admin/users`)

List of accounts, ordered by display name; a search box filters by name or email (a GET with `?q=`, swapped into the list, 25 per
page, "Show more"). Each row (a card at 360 px) shows: display name, email, role badge, "Can create campaigns" badge, the number of
campaigns the user takes part in, whether the account is closed (closed accounts are listed last, greyed, and have no controls),
and the date the account was created.

Controls on an open account (a collapsed "Edit permissions" form in the row):

- **Role**: radio `user | admin | superuser`.
- **Can create campaigns**: checkbox, only meaningful for `user` (disabled and checked for admins and superusers, as
  `User.MayCreateCampaigns` ignores the flag for them).
- **Save** sends `POST /admin/users/{id}/permissions`; the row is swapped with a notice ("Saved").

Refusals (`core.ErrRefused`, reason shown in the row): a superuser cannot demote **themselves** when they are the last open
superuser ("Make someone else a superuser first", the same rule as closing the account), and nobody edits a closed account.
There is deliberately no delete, no password reset and no email change: accounts are created and recovered with the CLI
(`make user`, `make grant`), because the service has no email delivery. A "New account" form would need to hand a password
over; not part of this spec. (REVIEW: no create/reset form in the first version.)

Empty and error states: no search match ("No accounts match"), a failed save keeps the form open with the reason.

### Contract (web consumes, core provides)

```go
// AdminUser is one row of the Users page.
type AdminUser struct {
    User                      // ID, Email, DisplayName, Role, CanCreateCampaigns
    Campaigns int             // campaigns the user is a member of (any status)
    CreatedAt time.Time
    Closed    bool            // closed_at is set; the other fields are the anonymised ones
}

// ListUsers returns accounts matching q (case-insensitive substring of name or email, empty: all), open ones first by
// display name, then closed ones. limit/offset page it; total is the number of matches.
ListUsers(ctx context.Context, actor User, q string, limit, offset int) (rows []AdminUser, total int, err error)

// SetUserPermissions changes the role and the creation flag of the user with that id. ErrRefused (reason fit to show) unless
// actor.IsSuperuser(); ErrRefused for a closed account and for demoting the last open superuser; ErrInvalidUser for a bad
// role; ErrNotFound for an unknown id. Drops the user's cached sessions (Auth cache) so the change applies at once.
SetUserPermissions(ctx context.Context, actor User, userID int64, role Role, canCreateCampaigns bool) error
```

Both live on `core.Auth`. `SetPermissions(email, ...)` (used by `make grant`) stays and is implemented on top of the same
checks without the actor. Repository additions on `authRepository`: `ListUsers(ctx, q, limit, offset)` (join count of
`campaign_members`; `users.created_at`, `closed_at`), `UserByID(ctx, id)` (exists as `CredentialsByID`, reuse) and
`SetUserPermissionsByID(ctx, id, role, can)`. Counting the superusers uses `OtherSuperusers` which exists.

## Rules files (`/admin/rulesets`)

The stored rules files ([definitions.md](definitions.md), table `rulesets`), grouped by kind (Vanilla, Battle Companies,
Glossary), newest first inside a group. Each file is a card/row: `name · version`, the 8-character hash label (full hash in a
`title`), size (KB), who uploaded it (display name, "Shipped with the app" when `uploaded_by` is null, "Former player" for closed
accounts), the upload date, a **Default** badge, and **the campaigns using it**: a count and, collapsed, the names linking to the
campaign overview (superusers may open any campaign; REVIEW: if campaign pages are member-only, show names without links).

Actions per file:

- **Make default** (not offered for the default file): `POST /admin/rulesets/{hash}/default`. New campaigns and the glossary
  picker start from the default; existing campaigns keep their files. Confirmation line "New campaigns will use this file".
  Calls `SetDefault(kind, hash)`, which already exists and is transactional.
- **Download**: `GET /rulesets/{hash}` already serves the content (login protected); the button links to it with
  `download="name-version.json"`.
- **Delete (prune)**: offered only when no campaign uses the file and it is not a default; `POST /admin/rulesets/{hash}/delete`
  with a confirm step ("Delete this file? It is used by no campaign."). Refused with `ErrRefused` when a campaign has
  appeared meanwhile or it is the default (the database foreign keys from `campaigns` also forbid it, so a race ends in the
  refusal, not in a broken campaign).
- **Prune all unused**: one button at the top, "Delete N unused files" (not counting defaults), with a confirm step.
  Files shipped with the app and not default are pruned too; the app registers its own files again at startup
  (`RegisterDefaults`), so nothing is lost (REVIEW: this makes prune of shipped files harmless).

**Upload** is one card at the top: choose a `.json` file, the browser hashes it (the existing `ruleset_picker.js`, `GET
/rulesets/{hash}` then upload when unknown) and the server validates it against the JSON schema for the kind it is asked for
(a kind selector: Vanilla, Battle Companies, Glossary). The outcome is shown in the card: stored (the new row appears), already
stored (the existing row is highlighted), or the rejection (`ErrInvalidRuleset` messages). The same size cap (16 MB) applies. Uploading is superuser only on
this page; organisers keep uploading from the campaign form.

A file with the same hash is stored once; every kind is identified by its hash alone, so two files with the same name and version but different content coexist (user decision 2026-10-21, migration 00015). The list shows name, version and short hash (`RulesetInfo.ShortHash`) to tell them apart.
(REVIEW: no "replace edition" action; delete then upload.)

### Contract

```go
// RulesetUsage is a stored file with who made it and what uses it.
type RulesetUsage struct {
    RulesetInfo                     // Kind, Hash, Name, Version, Default
    Size        int
    UploadedBy  *User               // nil: shipped with the app
    CreatedAt   time.Time
    Campaigns   []CampaignRef       // campaigns whose vanilla, bc or glossary hash is this file; ID and Name
}

// ListWithUsage returns every stored file (content not loaded) with its usage, grouped by the caller (kind, created_at desc).
ListWithUsage(ctx context.Context, actor User) ([]RulesetUsage, error)

// SetDefault makes the stored file the default of its kind (ErrNotFound for an unknown hash, ErrRefused unless superuser).
SetDefault(ctx context.Context, actor User, hash string) error

// Delete removes a stored file no campaign uses and that is not a default; ErrRefused with the reason otherwise.
Delete(ctx context.Context, actor User, hash string) error

// PruneUnused deletes every file no campaign uses and that is not a default, and returns how many.
PruneUnused(ctx context.Context, actor User) (int, error)
```

On `core.Rulesets` next to `Store`, `Default`, `Lookup`; the web `rulesets` interface grows by these four. Upload uses the
existing `Store` (with the uploading user recorded; today `Store` has no actor and records the uploader in `store`, check that
the HTTP handler passes the user). Repository additions on `rulesetRepository`:
`ListUsage(ctx)` (SELECT with the three `campaigns` joins and `users` for the uploader), `Delete(ctx, hash)` that deletes in one
statement `WHERE hash = $1 AND NOT is_default AND NOT EXISTS (campaigns referencing it)` and tells apart "unknown" from
"in use" (the foreign keys give the final say). `SetDefault` exists.

## Organiser handover

`Auth.CloseAccount` refuses the organiser of an unfinished campaign, so the organiser needs a way out: **Hand over the campaign**
on the campaign overview (a collapsed form in "The campaign" card, `POST /campaigns/{id}/handover`, for the organiser and
administrators until the campaign is finished) calls `Campaigns.HandOver` (one transaction in the repository: actor is the
organiser or an administrator, the new one must be a member, the old organiser becomes a player; `ErrRefused` otherwise, shown in
the form). The players offered are the campaign's members other than the organiser, with or without a company (`Campaigns.Members`, built).
**Frontend contract:** `handoverCandidates` should call `Campaigns.Members(ctx, id)` (add it to the web `campaigns` interface; `*core.Campaigns` has it)
and offer every `CampaignMember` with `Organiser == false` (`UserID`, `Name`; `CompanyName` is empty for a member without a company, it can be shown
next to the name); `HandOver` already accepts any member. Not built: the Users page does not list the campaigns a user organises, so a superuser who is not
a member of the campaign has to reach it another way. See [auth.md](auth.md).

## Implementer beads

Backend (labels `backend`) come first, each frontend bead depends on its backend one.

| Bead | Label | Depends on | What |
|---|---|---|---|
| users list backend | backend | | `Auth.ListUsers`, `authRepository.ListUsers`, Postgres query, tests (mock and integration) |
| users permissions backend | backend | | `Auth.SetUserPermissions` with the last-superuser and closed-account checks, session cache drop, Postgres by-id update, tests |
| rulesets usage backend | backend | | `Rulesets.ListWithUsage`, `SetDefault`, repository `ListUsage` and tests |
| rulesets delete backend | backend | rulesets usage backend | `Rulesets.Delete`, `PruneUnused`, repository `Delete`, tests incl. in-use and default |
| admin shell frontend | frontend | | `/admin` redirect, sub-navigation, `requireSuperuser` on `/admin/*`, replace `handleSoon`, web interfaces and mocks (`make generate`) |
| users page frontend | frontend | admin shell, users list backend, users permissions backend | `/admin/users` list, search, paging, permission form |
| rulesets page frontend | frontend | admin shell, rulesets usage backend, rulesets delete backend | `/admin/rulesets` list, default, download, delete, prune all, upload card |
| campaign handover backend | backend | | `Campaigns.HandOver` and repository transaction (REVIEW: P4, not needed for the section) |
| campaign handover frontend | frontend | campaign handover backend | handover control on the campaign page and on the Users row; then update the message in `CloseAccount` and auth.md |

Docs to update with each bead: `architecture.md` routes table (`GET /admin/...`), `frontend.md` navigation, `auth.md`.

## Campaign controls, organiser powers, timezone (backend: pause/end/invites built in mtu.21, powers mtu.23, timezone mtu.24; web beads follow)

### Who is a campaign manager

`core.CanManageCampaign(user, organiserID) bool` = administrator, superuser or the campaign's **organiser** (`CampaignDetail.OrganiserID`, the creator unless handed over). It replaces the seven copies of the check today (`AftermathAccessFor`, `CanWaive`, `LiveAccessFor`, `CanUndoEntry`, `Match.Access`, grants, invites, `web/campaign.go`). Organiser = campaign owner manages like an admin **inside that campaign only**; the Administration section, user permissions and rules files stay with administrators and superusers.

Admin-only actions in the app today and the proposal:

| Action (where) | Today | Proposal |
|---|---|---|
| Reopen an ended match (`Matches.ReopenMatch`, `web.handleReopenMatch`) | admin | **manager** (REVIEW) |
| Edit scenario/results of an ended match (`Match.Access` -> `MatchEditEnded`) | admin | **manager**; still refused once an aftermath started changes nothing, warning kept |
| Hand over the campaign (`Campaigns.HandOver`) | current organiser or admin | unchanged |
| Waive aftermath, grants, invites, live trackers, undo, match edit while open | admin or organiser | unchanged (via `CanManageCampaign`) |
| Pause, end, reactivate, invalidate/regenerate invites | do not exist | manager (pause, end, invites); **reactivate a finished campaign: admin only** (REVIEW) |
| Users list, roles, `CanCreateCampaigns` (`/admin`), rules files, prune | superuser | unchanged |

A finished campaign stays read only for managers too, except the admin-only reactivate.

### Campaign status: pause and end

States: `active`, `paused`, `finished` (exist). Controls on the campaign overview, a **Manage campaign** card visible to managers only:
- **Pause** (active -> paused) and **Resume** (paused -> active): one-step confirm dialog "Pause the campaign? Open matches can be finished; nobody can start a new match or join with an invitation until you resume."
- **End campaign** (active or paused -> finished): confirm dialog that lists consequences ("N matches are still open and will stay unfinished; N aftermaths pending", "Everything becomes read only, invitations stop working") and needs the campaign name typed (REVIEW). One-way for the organiser; administrators have **Reactivate** (finished -> active) on the same card.
- Status badge on the overview header, the campaign list and the home page: Paused (warning tone) and Finished (existing).

What each state means:

| | active | paused | finished |
|---|---|---|---|
| New match (`CanStartMatch`) | yes | **no** ("The campaign is paused.") | no |
| Finish open matches, results, End match, aftermath, live mode | yes | **yes** | no (read only) |
| Company actions (recruit, buy, heal, level up, grants, formation) | yes | yes (outside a locked company) | no |
| Create company for a member without one | yes | yes | no |
| Join by invitation, create invitation | yes | **no** (join: the link page says "This campaign is paused"; creating: refused) | no (as today) |
| Pages visible to members | yes | yes | yes, read only |

`CampaignDetail.CanPlay` (not finished) keeps meaning "may change anything"; a new `CampaignDetail.CanStartMatch` (active only) guards `CreateMatch` and the New match button. Ending revokes all invitations.

### Invitations: invalidate and regenerate

Next to **Create invitation** (managers; same hidden/refused rules): when the campaign has at least one active link, the card shows "N active invitation links" and two buttons: **Invalidate links** (all existing links stop working; confirm "Players who already joined stay") and **Regenerate** (invalidate all and show one new link once, the same copy panel as today). Joining with a revoked link behaves as an unknown link (`ErrNotFound`, "This invitation is not valid"). Tokens stay hashed, so existing links cannot be listed, only counted.

```go
SetCampaignStatus(ctx context.Context, actor core.User, campaignID int64, status core.CampaignStatus) error // active<->paused, ->finished; finished->active admin only; ErrRefused otherwise or for non-managers
InvalidateInvites(ctx context.Context, actor core.User, campaignID int64) (revoked int, err error)
RegenerateInvite(ctx context.Context, actor core.User, campaignID int64) (core.Invite, error)             // invalidate then Invite, one transaction
```
`CampaignDetail` gains `ActiveInvites int`, `OpenMatches int`, `PendingAftermaths int` (for the dialog text). Postgres: `campaign_invites.revoked_at TIMESTAMPTZ NULL`; `Redeem`/`Lookup`/counting ignore revoked rows; `Redeem` also refuses `status <> 'active'`. Routes: `POST /campaigns/{id}/status` (`to`), `POST /campaigns/{id}/invites/invalidate`, `POST /campaigns/{id}/invites/regenerate`, all with CSRF; non-managers get 403. The demo world replaces its raw SQL for the finished campaign with `SetCampaignStatus`.

Acceptance criteria: [ ] card visible to managers only, 403 otherwise; [ ] pause refuses CreateMatch and joins, keeps matches/aftermath/live working; [ ] end shows the counts and the typed name, makes everything read only and revokes links; [ ] admin reactivate; [ ] invalidate counts and revokes, regenerate yields one working link, old ones fail; [ ] 360 px, dialogs reachable by keyboard; [ ] service tests for each transition/permission, Postgres integration for revoked links.

### Timezone for displaying times

Times stay `TIMESTAMPTZ`/UTC in storage. `users.timezone TEXT NULL` (IANA name, validated with `time.LoadLocation`; NULL = automatic). Account page: **Time zone** selector (searchable list of IANA names, "Automatic (this device)" default, button "Use this device's zone"). Automatic means the browser zone: a tiny script sets a cookie `tz` (non-sensitive, validated as above) the server reads, falling back to `APP_TIMEZONE` and then UTC. Template funcs `fmtTime`/`fmtDate` take the request location from the context; used by the live log (`web/live_log.go`), match history and dates, grants, aftermath lines, sessions. `Auth.SetTimezone(ctx, user, name) error` (`ErrInvalid` for an unknown name). Acceptance: [ ] a user with Pacific/Auckland sees all times in that zone, others unaffected; [ ] invalid name refused; [ ] no stored timestamp changes; [ ] tests for formatting around a DST change.

Built (mtu.21): `core.CanManageCampaign(user, organiserID)` / `CampaignDetail.CanManage`, `Campaigns.SetCampaignStatus` (organiser or administrator pause, resume, end, also with open matches; only administrators reactivate: option A of mtu.26), `Invites.InvalidateInvites` / `RegenerateInvite` (managers; regenerate is refused for a paused or finished campaign), `campaignRepository.SetStatus` (under a row lock; ending revokes every link in the same transaction), `inviteRepository.Revoke` / `Replace`, `campaign_invites.revoked_at` (migration 00018; `Redeem` and `Lookup` ignore revoked links), `CampaignDetail.ActiveInvites` / `OpenMatches` / `PendingAftermaths` for the dialog texts. Like `Invite`, the invite methods need the actor to be a member of the campaign (an administrator who is not a member gets `ErrNotFound`; `SetCampaignStatus` has no such condition). The demo world uses `SetCampaignStatus` instead of raw SQL for the paused and the finished campaign.

User decision on ending (mtu.26, 2026-10-21): an ended campaign is ended for good for the organiser; only administrators and superusers reactivate it. `SetCampaignStatus(actor, id, to, confirm)` needs `confirm` = the campaign's exact name to end it (the page asks after its warnings). Matches stay exactly as they were, frozen: open matches stay open but nothing changes until an administrator reactivates the campaign. Every write path refuses with "The campaign is finished.": match edits and ending (`CanPlay`), the live service (trackers, objective, MVP votes, undo), side tags, grants, the aftermath, invitations, and now every company use case (`companyDeps.myCompanyToChange`: armoury, recruit, level-ups, treatment, renaming, formation, cemetery order, perks); tested in `TestFinishedCampaignRefusesEveryCompanyWrite`.

Built (mtu.24, backend): `users.timezone` (migration 00019, NULL = automatic), `User.Timezone`, `Auth.SetTimezone(ctx, userID, name)` ("" clears; an unknown IANA name or "Local" is `ErrInvalidUser`; the session cache follows at once), `core.ValidTimezone`, `core.UserLocation(user, browserZone, fallback)` (saved zone, then the `tz` cookie value, then APP_TIMEZONE, then UTC) and `time/tzdata` embedded in the server. Left for the web bead: the `tz` cookie script, the `fmtTime` / `fmtDate` template functions taking the request's location from the context, the Account page selector, and applying them to the live log, match history, grants, aftermath lines and sessions.

`AdminUser.Organises []OrganisedCampaign{ID, Name, Status, Players}` (filled by `ListUsers`) lists the campaigns a user organises for the Users page; the handover itself is `Campaigns.HandOver(actor, campaignID, newOrganiserID)`, which administrators may call without being a member (i3e web part: row expander in `admin_users.gohtml`, `POST /admin/users/{id}/handover`).
