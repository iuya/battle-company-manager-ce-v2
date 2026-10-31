# Authentication

How accounts, passwords, sessions and request protection work. Accounts sign in with **email and password**. There is no open sign-up page: an administrator invites people to create an account
([Account invitations](#account-invitations)), a superuser hands out recovery links ([Password recovery](#password-recovery)), and the
server's command line makes the first accounts. A visitor who is not signed in sees the login page; a signed-in user sees the main page.

## Accounts

```
make user EMAIL=you@example.com NAME="Your Name"      # or: server user create --email … --name …
```

`make grant EMAIL=… ROLE=user|admin|superuser [CREATE=1]` sets the global role and whether a plain user may create
campaigns (columns `role`, `can_create_campaigns`). It is **the only way to a superuser**: the web interface never creates, changes or
demotes one. `Auth.SetUserPermissions` of the Users page follows the permission matrix of [admin.md](admin.md#roles) (administrators make
organisers, superusers also make administrators); both paths refuse closed accounts and demoting the last open superuser, drop the user's
cached sessions and write an audit event.

The password is read from the terminal without echo (twice), or from stdin when piped. It must be 10–128 characters (no
composition rules; the upper bound keeps argon2 from being fed unbounded input). Emails are trimmed and lower-cased and
unique case-insensitively. The service sends no email, so invitation and recovery links are handed over by hand by an administrator or a
superuser. Registration and `make user` share one validator (`validateAccount`), which reports every problem at once.

## Account invitations

An administrator or superuser makes an **account invitation** on the Invitations page ([admin.md](admin.md#invitations)): *named* (one use, tied
to an email; the person chooses a display name and a password) or *anonymous* (up to 1-50 accounts, each person gives their own email).
Expiry 1 hour to 30 days (the page offers 1, 3, 7, 14, 30; default 7). The public page is `GET/POST /register/{token}`:

- the token (32 random bytes, 43 URL-safe characters) is stored **only as a SHA-256 hash** (`account_invitations.token_hash`, `BYTEA`); the
  raw token exists in the response that shows it once. It is never logged: the request log writes the route (`/register/{token}`), as it does for
  `/recover/{token}` and `/join/{token}`.
- a named link carries `?email=` **for people to read only**: the page and the service take the address from the stored invitation and ignore the query.
- in one transaction the service locks the invitation row, re-checks it, creates the account (role `user`, the invitation's creation flag), a
  session, the use row and the audit event, and increments `uses`; two concurrent uses of a one-use link cannot both win. Password hashing
  (argon2) runs before the transaction and only for a link that looked usable.
- the new person is **signed in at once** (the session cookie is set) and lands on the campaigns page.
- **Every failure of the link is the same**: a malformed, unknown, used, expired or revoked token, and an address that already has an account,
  show the generic page **"Talk with the administrator."** (`core.ErrLinkUnusable`; status 404 on GET, 422 on POST, 429 when throttled), so the form
  cannot be used to find out which addresses have an account. Field mistakes of a valid link (name 1-60, email format, password 10-128, the
  two passwords) are shown on the form. A signed-in visitor opening a link is sent to `/`; the token is not touched.
- the answers carry `Cache-Control: no-store`, `Referrer-Policy: no-referrer` and `X-Robots-Tag: noindex`.
- the registration form is **throttled** per address and link (5 failures in 10 minutes) and per address (20), in memory, like the login
  throttle; the key uses a fingerprint of the token, never the email.

Known limits: the holder of a valid anonymous link learns from the generic error versus success that an address exists (the throttles and the use
limit bound it), and can register an address that is not theirs (there is no email verification: the person it belongs to is then told by the administrator).

## Password recovery

A **superuser** makes a single-use "set a new password" link for any other open account from the Users page ([admin.md](admin.md#recovery-links));
administrators cannot make or revoke one, and nobody makes one for their own account (the Account page changes it). Expiry 1 hour to 7 days
(the page offers 1, 3, 7 days; default 1 day). An account has at most one pending link: a new one replaces the old (reason `replaced`).

- the token is stored as a hash like an invitation's and shown once, in a ready-to-copy message; the public page is `GET/POST /recover/{token}`
  (new password twice). On success the password is replaced, **every session of the user is dropped**, the link is spent and the person is sent
  to the login page with a notice: **no automatic sign-in**, the link alone never opens a session.
- the link is **invalidated if the person signs in correctly, or opens the app with a valid session, before using it** (reasons `login`, `access`):
  `Auth.Login` invalidates it after the password is verified (a failed password does nothing, and a failure of the update fails the login
  closed), and `Auth.Authenticate` does so for a session whose cache entry says a link is pending. That fact rides on the session cache entry,
  filled by the same query on a cache miss, so a cache hit with no pending link costs **zero extra queries**. `Recovery.Generate` sets the flag
  on the user's hot cache entries so a person browsing right now is caught on the next request. `/recover/{token}` is registered without the
  session middleware and never reads the cookie, so a request made with the link is not an app access. Closing an account invalidates its link (`closed`).
- states, derived: pending, used, revoked, invalidated (with the reason), expired. The Users page shows the latest link of each account.
- a wrong, used, expired, revoked or invalidated link shows the same generic page as invitations; the form has its own throttle (5 and 20 as above;
  a success resets the key).
- a superuser can make a link for another superuser or administrator (a compromised superuser can take over any account; accepted).

Limits: the throttles and the session cache are per process. With several app instances a recovery link can survive an app access on another
instance for at most the 60 seconds of the cache, and the throttle limits multiply (documented in [operations.md](operations.md)).

## Audit

`audit_events` (migration 00026) records, in the same transaction as the change: role changes (web, with who and from/to, or `source=console` for
`make grant`), invitations created, revoked and used (subject: the new account), recovery links created, revoked, used and invalidated (source
`public` or `system`). `detail` has a closed list of keys per action and never holds a token, a hash, a password or an email address. There is no
viewer page: the Invitations list reads who created and revoked each invitation and the accounts made through it, and the Users page shows the
recovery status. Unused revoked or expired invitations and ended recovery links are deleted 90 days after they ended by the housekeeping of
`Auth.Login` (`PurgeStaleTokens`); used invitations and the audit events are kept.

## Passwords

Hashed with **argon2id** (OWASP minimum: 19 MiB, 2 iterations, 1 lane) and stored as a PHC string
(`$argon2id$v=19$m=…,t=…,p=…$salt$hash`). Verification reads the parameters from the stored hash, so they can be raised
later without invalidating existing passwords. When the email is unknown the service still verifies against a dummy
hash, so timing does not reveal which emails exist, and the only failure message is "Invalid email or password."

## Sessions

- A successful login creates a random 256-bit token. **Only its SHA-256 is stored** (`sessions` table), so a leaked
  database cannot be used to impersonate anyone.
- The token travels in the `bcm_session` cookie: `HttpOnly`, `SameSite=Lax`, `Path=/`, `Secure` when
  `SECURE_COOKIES=true`.
- **Last campaign (`bcm-last`).** A second cookie remembers where the user was: `<userID>.<campaignID>[.<companyID>]`, base-10
  ids only (at most 64 characters, parsed strictly; anything else is dropped). `HttpOnly`, `SameSite=Lax`, `Path=/`, `Secure`
  like the session cookie, 90 days renewed on each write. It is unsigned because it authorises nothing: it is a hint read only
  after authentication and validated against the database on every read. It is written by full-page GETs of a campaign page
  the user belongs to (and of one of their own companies, which records the company); never by htmx fragments, event streams
  or POSTs, and never for a finished campaign. `GET /` for a signed-in user redirects (303) to My Company of the campaign from the
  cookie; the cookie is **cleared** when it is malformed, holds another user's id (the id inside must be the session user's),
  the campaign is finished, or the user can no longer open it (`ErrNotFound`), and the list is shown instead. A paused
  campaign still redirects. `GET /campaigns/{id}/my-company` (no company) opens the company recorded for that campaign if it is
  still the user's own, else their oldest. The campaign list lives at `GET /campaigns`, which never reads the cookie and never
  redirects, so the brand link, the sidebar's "Campaigns" and "← Campaigns" always reach it. Logging out keeps the cookie (ids
  only); closing the account clears it. After sign-in `/` applies it unless `?next=` names another page.
- Sessions last 30 days and are renewed when more than half of that is gone, so regular users stay signed in.
- **In-memory cache:** every request of a page, htmx partials included, goes through `Auth.Authenticate`. A session
  confirmed within the last minute is answered from memory, so most requests do not touch the database. Logging out
  clears the entry immediately; a session deleted directly in the database stops working within a minute. The cache is
  per process and bounded in size.
- Expired sessions are purged opportunistically on login.

## Request protection

- **CSRF:** Go's `http.CrossOriginProtection` rejects cross-site state-changing requests (`Sec-Fetch-Site` / `Origin`),
  together with the `SameSite=Lax` cookie.
- **Throttles:** each is in memory per process, window 10 minutes, per account key and per address.

  | Throttle | Account key | Per key | Per address | Per link |
  |---|---|---|---|---|
  | Login | address + email | 5 | 20 | - |
  | Account page password | user | 5 | 20 | - |
  | Registration (`/register/{token}`) | address + token fingerprint | 5 | 20 | 30 |
  | Recovery (`/recover/{token}`) | address + token fingerprint | 5 | 20 | 30 |

  On the link pages only a failed **attempt** (a POST that the link refuses: dead link, address taken) counts; looking at a dead link
  (GET) is cheap and never counts, so nobody can lock the page out by requesting it. The per-link cap counts the failures of one link
  whatever the address, so rotating addresses does not give a holder of a link more guesses at which emails exist. Behind a reverse
  proxy `TRUST_PROXY=true` is required: otherwise every visitor shares the proxy's address and 20 failed attempts from anyone block the
  link pages for everybody for 10 minutes.

- **Login throttling:** failed logins are counted in memory per address+email (5 per 10 minutes) and per address (20);
  beyond that the login page answers `429` without checking the password. A success resets the account counter. The address is
  the peer's; behind a reverse proxy set `TRUST_PROXY=true` and the last `X-Forwarded-For` entry is used instead (otherwise all
  users share the proxy's address).
- **Deep links:** a signed-out visitor who opens a protected GET page is sent to `/login?next=<that page>` (for an htmx request
  the page the browser shows); only paths of this site are kept (`safeNext`), POSTs are never remembered.
- **Headers:** `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: same-origin`; pages are
  sent with `Cache-Control: private, no-cache` because they depend on who is signed in.

## Where it lives

| Piece | File |
|---|---|
| Accounts, sessions, cache | `internal/core/auth.go`, `session_cache.go`, `password.go` |
| Tables and queries | `internal/postgres/migrations/00002_users_sessions.sql`, `internal/postgres/auth.go` |
| Cookie, middleware | `internal/web/session.go` |
| Login and logout handlers, throttle | `internal/web/login.go`, `throttle.go` |
| Roles matrix, single-use tokens, audit | `internal/core/roles.go`, `token.go`, `accountaudit.go` |
| Account invitations, recovery links | `internal/core/account_invitations.go`, `recovery.go`; `internal/postgres` (migrations 00026-00028) |
| Invitations page, registration and set-password pages | `internal/web/admin_invitations.go`, `register.go`, `static/js/copylink.js` |
| Account command | `cmd/server/user.go` |

## The Account page

`/account` (sidebar, bottom; see [frontend.md](frontend.md#account)) is for the signed-in user. Mobile first: one column of cards, each its
own htmx form that swaps only itself (422 with the form kept on error).

- **Profile**: display name (1-60 characters, as `CreateUser`); the email is shown read only ("ask an administrator to
  change it": there is no email delivery). `Auth.UpdateProfile(ctx, userID, displayName)` returns the updated `User`
  and refreshes the session cache.
- **Password**: current password, new password (10-128 characters, with show/hide), confirmation (checked in the browser and the
  handler). `Auth.ChangePassword(ctx, userID, current, next, keepToken)` verifies the current one (a wrong one is
  `ErrInvalidCredentials`, throttled in the web layer like login, per user), stores the new hash and **deletes every other
  session** of the user (`keepToken` stays signed in). A success notice says so.
- **Sessions**: "Sign out everywhere else" (same deletion, without a password change). The page does not list the other sessions; the notice reports how many ended.
- **Appearance**: the palette and mode of the settings menu, saved for the account (below).
- **Close account** (danger card, collapsed): asks the password and the email typed again.
  `Auth.CloseAccount(ctx, userID, password)` **anonymises** the user instead of deleting rows (companies, matches and
  rankings stay intact for the other players): display name "Former player", email replaced by `closed-<id>@invalid`,
  password hash emptied so it cannot sign in, `closed_at` set, every session deleted, companies kept and listed under the
  new name. The old address is free again: it can be registered as a brand-new account that has no link to the closed one
  (the closed account's history stays under "Former player"). Refused with `ErrRefused` (reason fit to show) for the organiser of an unfinished campaign ("Hand over your
  campaigns (on the campaign overview) or finish them first") and for the last superuser. The organiser (or an administrator)
  hands a campaign over with **Hand over the campaign** on its Overview (`POST /campaigns/{id}/handover`, `Campaigns.HandOver`; the
  players offered are the owners of the campaign's companies).

Errors: `ErrInvalidUser` (joined messages fit to show), `ErrInvalidCredentials`, `ErrRefused`. Repository additions:
`CredentialsByID`, `UpdateDisplayName`, `UpdatePasswordHash`, `DeleteSessionsExcept(userID, keepTokenHash)`, `CloseUser`,
`OrganisesUnfinishedCampaign`, `OtherSuperusers`, `SetAppearance`. `Auth.SignOutOthers(ctx, userID, keepToken)` returns how many
sessions it ended. Migrations `00009` (`users.closed_at`) and `00010` (`palette`, `mode`). Changes to the name or the
appearance update every cached session of the user; sign-out-others and closing drop them from the cache. A closed user cannot
log in (empty hash, replaced email) and `CredentialsByID` and the update methods ignore closed users.

## Appearance per account

`core.User` carries `Appearance{Palette, Mode string}` (empty = never chosen), loaded with the session so the layout needs no
extra query. Columns `users.palette` and `users.mode` (nullable, checked against the known keys; the known palettes and modes
live in `core` as `Palettes` and `Modes`, the web layer's picker reads them).

- **Signed in**: the layout renders `data-pref="<palette>.<mode>"` on `<html>`; the inline script in `<head>` uses it
  instead of the `bcm-theme` cookie, so the choice follows the user across devices. `$store.theme.set` still writes the cookie
  and also calls `PUT /account/appearance` (form body `palette`, `mode`; 204; 422 for an unknown key). A failed call is
  silent: the cookie keeps working. `SetAppearance` must also update the in-memory session cache entry.
- **Login**: a user with no saved appearance adopts the cookie's value (saved on first sign-in, so a choice made on the login
  page is not lost); otherwise the saved value is written to the cookie by the login response. Logging out keeps the cookie.
- **Anonymous visitors** (the login page): the cookie only.
