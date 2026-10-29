# Administration

Who may do what beyond playing: the Administration section (users and rules files), the powers of a campaign's organiser, the campaign
status (active, paused, finished) and the organiser handover. Read it if you run a server or organise campaigns.

## Roles

| Role | Can |
|---|---|
| `user` | Play. Creates campaigns only when the account has the "can create campaigns" flag |
| `admin` | Also creates campaigns and edits any match or aftermath |
| `superuser` | Everything an admin does, and sees the Administration section |

Roles are set in the Administration section or with `make grant EMAIL=... ROLE=user|admin|superuser [CREATE=1]` ([auth.md](auth.md)).

## The Administration section

`/admin` is for superusers; it answers 404 to everybody else, POSTs included (`requireSuperuser`). It has two pages under one
sub-navigation (tabs on a phone, a row of links on a desktop): **Users** (`/admin/users`) and **Rules files** (`/admin/rulesets`);
`/admin` redirects to Users. The pages follow [frontend.md](frontend.md): one column at 360 px, theme tokens only, every action an htmx
form that swaps its own row or card, 422 with the form kept on error. The code is in `internal/web/admin.go` and
`templates/pages/admin_*.gohtml`.

### Users

The accounts, open ones first by display name, then closed ones (greyed, with no controls). A search box filters by name or email
(`GET /admin/users?q=&offset=`, swapped into the list, 25 per page, "Show more"). A row shows the display name, email, role badge,
"Can create campaigns" badge, the number of campaigns the user takes part in and the creation date.

A collapsed **Edit permissions** form on an open account sets the role (`user`, `admin`, `superuser`) and the "Can create campaigns"
checkbox (disabled and checked for admins and superusers, as `User.MayCreateCampaigns` ignores the flag for them).
`POST /admin/users/{id}/permissions` swaps the row with a notice. The service (`Auth.SetUserPermissions`) refuses a closed account and
demoting the last open superuser ("Make someone else a superuser first"), and drops the user's cached sessions so the change applies at once.

There is no delete, password reset or email change: accounts are created and recovered with the command line (`make user`, `make grant`),
because the service sends no email.

A user who organises campaigns has an "Organises N campaigns" expander listing each campaign (status, players) with a select of the
campaign's members and **Hand over** (`POST /admin/users/{id}/handover`).

### Rules files

The stored rules files ([definitions.md](definitions.md), table `rulesets`), grouped by kind (Vanilla, Battle Companies, Glossary), newest
first. A row shows `name · version`, the 8-character hash label, the size, who uploaded it ("Shipped with the app" when there is no
uploader, "Former player" for closed accounts), the upload date, a **Default** badge and the campaigns using it (a count and, collapsed,
their names linking to the campaign). A file is identified by its hash alone: two files with the same name and version but different
content coexist, and the short hash tells them apart.

| Action | Route | Rule |
|---|---|---|
| Upload | `POST /admin/rulesets` (multipart `kind`, `file`) | The server hashes the file, validates it against the schema of its kind and stores it (at most 16 MB). Outcome in the card: stored, already stored, or the rejection |
| Check this file | `POST /admin/rulesets/check` | The same validation as a dry run; stores nothing |
| Make default | `POST /admin/rulesets/{hash}/default` | New campaigns and the glossary picker start from the default; existing campaigns keep their files |
| Download | `GET /admin/rulesets/{hash}/download` | The stored file as an attachment |
| Delete | `POST /admin/rulesets/{hash}/delete` | Only when no campaign uses the file and it is not a default (confirm step); the database foreign keys back the check |
| Prune | `POST /admin/rulesets/prune` | "Delete N unused files" with a confirm step. Shipped files are registered again at start, so pruning them is harmless |

A rejected upload lists every problem (file, path, message, hint, closest known name; 200 shown, the rest counted) and stores nothing; the
rendering is described in [frontend.md](frontend.md) ("The rules file report"). Organisers upload from the new-campaign form as well.

The methods are on `core.Rulesets` (`ListWithUsage`, `SetDefault`, `Delete`, `PruneUnused`, `Download`, `Check`, `Store`/`StoreAs`, which record the uploader); each takes the acting user and refuses
anyone who is not a superuser.

## Campaign management

### Who manages a campaign

`core.CanManageCampaign(user, organiserID)` is true for an administrator, a superuser and the campaign's **organiser** (the creator, unless
the campaign was handed over). The organiser manages like an admin inside that campaign only; the Administration section, user
permissions and rules files stay with administrators and superusers.

| Action | Who |
|---|---|
| Pause, resume, end the campaign; invalidate or regenerate invitation links | Managers |
| Reactivate a finished campaign | Administrators and superusers only |
| Reopen an ended match; edit the scenario and result of an ended match | Managers |
| Waive an aftermath, grants, invites, live trackers and undo, edit a match while it is open | Managers |
| Hand over the campaign | The current organiser or an administrator |

### Status: pause and end

The **Manage campaign** card on the campaign Overview is visible to managers only (`campaign_manage.go`, template `manage_card`).

- **Pause** and **Resume**: one inline confirmation. A paused campaign lets open matches be finished but nobody can start a new match or join with an invitation.
- **End campaign** takes three steps: the consequences ("N matches are still open and will stay unfinished", pending aftermaths and active invitations), the exact campaign name typed, a final "End the campaign for good". `POST /campaigns/{id}/status` carries `to` and `confirm`.
  Ending revokes every invitation link. An ended campaign is ended for good for the organiser; administrators have **Reactivate**.

| | active | paused | finished |
|---|---|---|---|
| New match (`CampaignDetail.CanStartMatch`) | yes | no ("The campaign is paused.") | no |
| Finish open matches, results, End match, aftermath, live mode | yes | yes | no (read only) |
| Company actions (recruit, buy, heal, level up, grants, formation) | yes | yes (outside a locked company) | no |
| Create a company for a member without one | yes | yes | no |
| Join by invitation, create an invitation | yes | no | no |
| Pages visible to members | yes | yes | yes, read only |

A finished campaign is frozen: matches stay exactly as they were (an open match stays open but nothing changes), and every write path refuses
with "The campaign is finished." until an administrator reactivates it. That covers match edits and ending, the live service, side tags,
grants, the aftermath, invitations and every company use case (`TestFinishedCampaignRefusesEveryCompanyWrite`).

The service is `Campaigns.SetCampaignStatus(actor, id, to, confirm)`; the repository changes the status under a row lock. `CampaignDetail`
carries `ActiveInvites`, `OpenMatches` and `PendingAftermaths` for the dialog texts.

### Invitations

The **Invite players** block of the Overview makes a link (7 days, unlimited uses) shown once with **Copy**; **Make another link** replaces it.
When links exist the block shows "N links work now" with **Invalidate links** (all stop working; players who already joined stay) and
**Regenerate** (invalidate all and show one new link). A revoked link behaves like an unknown one ("This invitation is not valid"). Tokens
are stored hashed, so links can be counted but not listed. Opening `/join/{token}` never changes anything by itself: a signed-out visitor
is sent to `/login?next=/join/{token}`, a signed-in one sees a confirmation and **Join the campaign** (`POST /join/{token}`).
A paused or finished campaign refuses the join and says why. Routes: `POST /campaigns/{id}/invite`, `.../invites/invalidate`, `.../invites/regenerate`.

### Organiser handover

`Auth.CloseAccount` refuses the organiser of an unfinished campaign, so the organiser needs a way out: **Hand over the campaign**, a
collapsed form in the Overview's campaign card (`POST /campaigns/{id}/handover`, organiser and administrators until the campaign is
finished), calls `Campaigns.HandOver`. In one transaction the new organiser must be a member and the old one becomes a player; the
players offered are the campaign's other members (`Campaigns.Members`), with or without a company. Administrators may hand over a campaign
without being a member, from the Users page.

## Time zones

Times are stored in UTC and shown in the viewer's zone: the account's saved zone, else the browser's, else `APP_TIMEZONE`, else UTC. The
Account page has the selector ([frontend.md](frontend.md#account)); `Auth.SetTimezone` validates the IANA name.
