# Frontend: account, invitations and demo sign-in

The Account page, the copy component used for invitation and recovery links, invite links, and the accounts of the minimal demo data.
The design language and the page patterns are in [frontend.md](frontend.md).

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

## Account invitations, recovery links and the copy component

Three screens for the single-use links of [auth.md](auth.md). All are one column at 360 px, tokens only, 44 px targets, htmx swaps of their own card.

- **Invitations** (`templates/pages/admin_invitations.gohtml`, `admin_invitations.go`): the creation card first (kind radios Named/Anonymous, email or
  "Maximum accounts", "Expires in" select, "Can create campaigns" checkbox; a 422 keeps what was typed), then filter chips (a link each, swapping
  `#invitation-list`) and one card per invitation with the state badge (`data-state`), creator, dates, "used N of M" and an inline-confirmed
  **Revoke** (a refusal goes to the row's message slot through `HX-Retarget`). A successful create answers the card with the message and sends
  `HX-Trigger: invitations-changed`, which reloads the list.
- **Users page**: the permission form has `role="switch"` checkboxes shown per actor and target (never a disabled-only control, no Superuser
  option), a muted note where the viewer has no control, and, for superusers, the **Recovery link** card (status line with `data-recovery-state`,
  expiry select, Generate/Revoke with inline `role="alertdialog"` confirmations, the one-time message).
- **Public pages** `register.gohtml`, `recover.gohtml`, `link_unusable.gohtml`: a card like the login's, plain forms (`hx-boost="false"`, so a 422 is a
  full page), password and confirmation with Show/Hide and a client-side match hint, and for a named invitation the stored email read-only. Every answer
  from these routes is `Cache-Control: no-store` plus `Referrer-Policy: no-referrer` and `X-Robots-Tag: noindex` (`secretPage`).
- **Copy component** (`static/js/copylink.js`, in `bundleScripts`, `x-data="copyLink"`; partial `copy_message`): a read-only `<textarea>` with the
  message (selectable where the Clipboard API is missing, e.g. plain http on a LAN) and a **Copy** button that calls `navigator.clipboard.writeText`
  and falls back to selecting the text; "Copied" feedback in a live region. No inline script, and the attributes hold only fixed text: the message
  is element content, escaped.

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


See also: [auth.md](auth.md), [admin.md](admin.md), [demo-world.md](demo-world.md), [frontend.md](frontend.md).
