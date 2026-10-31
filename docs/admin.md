# Administration

Who may do what beyond playing: the Administration section (users, invitations, recovery links and rules files), the powers of a campaign's
organiser, the campaign status (active, paused, finished) and the organiser handover. Read it if you run a server or organise campaigns.

## Roles

There are four access levels, named in `core` (`roles.go`: `Level`, `Actor`, `RoleChangeAllowed`, one pure function the tests walk cell by cell):

| Level | Stored as | Can |
|---|---|---|
| **User** | `role=user`, `can_create_campaigns=false` | Play |
| **Organiser** | `role=user`, `can_create_campaigns=true` | Also create campaigns and manage them |
| **Administrator** | `role=admin` | Also edit any match or aftermath, see the Users and Invitations pages, make organisers |
| **Superuser** | `role=superuser` | Everything an administrator does, and make administrators, make rules-file changes and generate recovery links |

("Organiser" here is the global permission to create campaigns; the organiser of one campaign is a different thing, see [Who manages a campaign](#who-manages-a-campaign).)
A superuser exists only through the console (`make grant EMAIL=... ROLE=superuser`, [auth.md](auth.md)): **the web interface can never create, change
or demote one**, not even a superuser's own account. Who may change whom through the web (the service checks it again; hiding a control is not the protection):

| Actor \ target | User / Organiser | Administrator | Superuser |
|---|---|---|---|
| Administrator | to Organiser or back | refused (403) | refused (403) |
| Superuser | to Organiser or Administrator and back | to User, Organiser | refused (403) |
| Console (`make grant`) | any level | any level | any level |

State rules for every actor: a closed account is refused ("This account is closed"), demoting the last open superuser is refused (console only
in practice), promoting to administrator keeps the stored creation flag so a later demotion restores it, a change that changes nothing writes
nothing, and every change drops the user's cached sessions and writes an audit event (who, whom, from and to; `source=console` for `make grant`).

## The Administration section

`/admin` is for administrators and superusers; it answers 404 to everybody else, POSTs included. Under one sub-navigation (tabs on a phone, a row
of links on a desktop):

| Page | User, organiser | Administrator | Superuser |
|---|---|---|---|
| **Users** (`/admin/users`) | 404 | list, search, hand over a campaign, Organiser switch | also the Administrator switch and recovery links |
| **Invitations** (`/admin/invitations`) | 404 | create, list, revoke | the same |
| **Rules files** (`/admin/rulesets`) | 404 | 404 | yes |

`/admin` redirects to Users. The sidebar's Administration link is shown to administrators and superusers. Guards: `requireAdmin` (Users and
Invitations) and `requireSuperuser` (Rules files, recovery links). A signed-in administrator whose request the matrix refuses (a forged form) gets
**403**; a state refusal is a 422 with the form kept. The pages follow [frontend.md](frontend.md): one column at 360 px, theme tokens only, every
action an htmx form that swaps its own row or card. The code is in `internal/web/admin.go`, `admin_invitations.go`, `register.go` and
`templates/pages/admin_*.gohtml`.

### Users

The accounts, open ones first by display name, then closed ones (greyed, with no controls). A search box filters by name or email
(`GET /admin/users?q=&offset=`, swapped into the list, 25 per page, "Show more"). A row shows the display name, email, role badge,
"Can create campaigns" badge, the number of campaigns the user takes part in and the creation date.

A collapsed **Edit permissions** form shows only the switches the viewer may use: an administrator has the **Organiser** switch on users and
organisers; a superuser also has the **Administrator** switch on users, organisers and administrators (Organiser is shown checked and disabled
while Administrator is on, "Administrators always can"). There is no Superuser option anywhere. On accounts the viewer cannot change the row says
why ("Administrators are managed by superusers.", "Superusers are set from the console."). The form posts `role=admin` when the Administrator switch
is on (nothing otherwise) and `can_create`; `POST /admin/users/{id}/permissions` swaps the row with a notice, `Auth.SetUserPermissions` decides.

Superusers also get a collapsed **Recovery link** card on every other open account, see [Recovery links](#recovery-links).

A user who organises campaigns has an "Organises N campaigns" expander listing each campaign (status, players) with a select of the
campaign's members and **Hand over** (`POST /admin/users/{id}/handover`).

There is no delete or email change; the service sends no email, so links are handed over by hand.

### Invitations

Account invitations (not to be mistaken for the campaign invitation links below) let an administrator invite a person to **create an account**.
The page has the creation card, then a filter row (All, Pending, Used, Expired, Revoked) and the list (cards, 25 at a time):
kind, email or "Anyone with the link", state badge, "Created by NAME on DATE", expiry, "used N of M", the display names of the accounts made
through it, and **Revoke** (inline confirmation) on pending ones.

| | Named | Anonymous |
|---|---|---|
| Who | one person, one use, tied to an email (default kind) | anyone with the link, each with their own email |
| Limit | 1 use | 1 to 50 accounts (default 10) |
| Expiry | 1, 3, 7 (default), 14 or 30 days (service: 1 hour to 30 days) | the same |
| Option | "Can create campaigns" gives the new account the organiser flag | the same |

`POST /admin/invitations` shows the **message to send** once in a read-only box with a **Copy** button ("Open this link to create your account:
https://HOST/register/TOKEN?email=ADDRESS / It works once and expires on DATE."): the token is stored only as a hash, never listed, and not
shown again; a lost link is revoked and made again. A named invitation for an address that already has an account is refused with a clear message
here (administrators can already list the accounts). `POST /admin/invitations/{id}/revoke` works on pending invitations only. States are derived,
never stored: revoked, used, expired, pending (in that precedence). The public side is in [auth.md](auth.md).

### Recovery links

A superuser opens an account's **Recovery link** card on the Users page and presses **Generate link** (inline confirmation: "This replaces any
earlier link for NAME"), choosing the expiry (1 day by default, 3 or 7 days). The message ("Open this link to set a new password: ...") appears once
in the row, with **Copy**. The card's status line says: pending until DATE and who made it, used, expired, revoked by NAME, or invalidated
(the person signed in; the person opened the app; replaced by a newer link; the account was closed). **Revoke link** (confirmation) stops a
pending link. Rules: superusers only (an administrator gets 404 and no card), a closed account and the superuser's own account have none (use the
Account page), one pending link per account (a new one replaces the old). Routes: `POST /admin/users/{id}/recovery` and `.../recovery/revoke`.
What happens when the person uses the link is in [auth.md](auth.md).

### Rules files

The stored rules files ([definitions.md](definitions.md), table `rulesets`), grouped by kind (Vanilla, Battle Companies, Glossary), newest
first. A row shows `name · version`, the 8-character hash label, the size, who uploaded it ("Shipped with the app" when there is no
uploader, "Former player" for closed accounts), the upload date, a **Default** badge and the campaigns using it (a count and, collapsed,
their names linking to the campaign). A file is identified by its hash alone: two files with the same name and version but different
content coexist, and the short hash tells them apart.

| Action | Route | Rule |
|---|---|---|
| Upload | `POST /admin/rulesets` (multipart `kind`, `file`) | The server hashes the file, validates it against the schema of its kind and stores it (at most 16 MB). Outcome in the card: stored, already stored, or the rejection |
| Check this file | `POST /admin/rulesets/check` | The same validation as a dry run (`Rulesets.CheckSet` with the one file); stores nothing. To check a Battle Companies file against your own vanilla file and glossary, every player (not only superusers) can use the online validator at `/tools/validate` ([frontend.md](frontend.md#online-validator)); it stores nothing either |
| Make default | `POST /admin/rulesets/{hash}/default` | New campaigns and the glossary picker start from the default; existing campaigns keep their files |
| Download | `GET /admin/rulesets/{hash}/download` | The stored file as an attachment |
| Delete | `POST /admin/rulesets/{hash}/delete` | Only when no campaign uses the file and it is not a default (confirm step); the database foreign keys back the check |
| Prune | `POST /admin/rulesets/prune` | "Delete N unused files" with a confirm step. Shipped files are registered again at start, so pruning them is harmless |

A rejected upload lists every problem (file, path, message, hint, closest known name; 200 shown, the rest counted) and stores nothing; the
rendering is described in [frontend.md](frontend.md) ("The rules file report"). Organisers upload from the new-campaign form as well.

The methods are on `core.Rulesets` (`ListWithUsage`, `SetDefault`, `Delete`, `PruneUnused`, `Download`, `Check`, `Store`/`StoreAs`, which record the uploader); each takes the acting user and refuses
anyone who is not a superuser (administrators do not see this page).

## Campaign management

### Who manages a campaign

`core.CanManageCampaign(user, organiserID)` is true for an administrator, a superuser and the campaign's **organiser** (the creator, unless
the campaign was handed over). The organiser manages like an admin inside that campaign only; the Administration section, user
permissions, invitations and rules files stay with administrators and superusers.

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
