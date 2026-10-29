# Authentication

How accounts, passwords, sessions and request protection work. Accounts sign in with **email and password**. There is no sign-up page: accounts are created by an administrator with
the server's command line. A visitor who is not signed in sees the login page; a signed-in user sees the main page.

## Accounts

```
make user EMAIL=you@example.com NAME="Your Name"      # or: server user create --email … --name …
```

`make grant EMAIL=… ROLE=user|admin|superuser [CREATE=1]` sets the global role and whether a plain user may create
campaigns (columns `role`, `can_create_campaigns`). Like the superuser-only `Auth.SetUserPermissions` of the administration
section ([admin.md](admin.md)) it refuses closed accounts and demoting the last open superuser.

The password is read from the terminal without echo (twice), or from stdin when piped. It must be 10–128 characters (no
composition rules; the upper bound keeps argon2 from being fed unbounded input). Emails are trimmed and lower-cased and
unique case-insensitively. Forgot-password is not implemented (it needs email delivery, which the service does not
have).

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
- Sessions last 30 days and are renewed when more than half of that is gone, so regular users stay signed in.
- **In-memory cache:** every request of a page, htmx partials included, goes through `Auth.Authenticate`. A session
  confirmed within the last minute is answered from memory, so most requests do not touch the database. Logging out
  clears the entry immediately; a session deleted directly in the database stops working within a minute. The cache is
  per process and bounded in size.
- Expired sessions are purged opportunistically on login.

## Request protection

- **CSRF:** Go's `http.CrossOriginProtection` rejects cross-site state-changing requests (`Sec-Fetch-Site` / `Origin`),
  together with the `SameSite=Lax` cookie.
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
