# Architecture

How the service is built: the layers, the packages, the data model, the live match design, the request flow, the configuration and the commands.
Read it first if you are going to change the code. The feature documents ([matches.md](matches.md), [aftermath.md](aftermath.md),
[live-match.md](live-match.md), ...) describe behaviour; this one describes structure.

A small Go service with a server-rendered frontend and one external dependency: PostgreSQL. It is organised as
**ports and adapters**: the business logic knows nothing about HTTP or SQL, and talks to both through interfaces.

```
 browser ──HTTP──▶  internal/web  ──interface──▶  internal/core  ──interface──▶  internal/postgres ──▶ PostgreSQL
                    (HTTP adapter)                (business logic)               (persistence adapter)
 cmd/server: reads the config, builds the adapters and plugs them into each other (the only place that knows all three)
```

## Packages

| Package | Role |
|---|---|
| `cmd/server` | `main`: configuration, wiring (`backend.go`), graceful shutdown; the `user`, `audit`, `seed`, `migrate` and `healthcheck` commands (see [auth.md](auth.md)); `world*.go` is the demo world script ([demo-world.md](demo-world.md)) |
| `cmd/rulecheck` | Checks a Battle Companies rules file the way an upload does ([definitions.md](definitions.md)) |
| `internal/core` | Business logic: accounts, sessions, campaigns, companies, matches, rules computation. Declares the repository interfaces it needs |
| `internal/core/lean` | The Battle Companies file format (`schemaVersion` 2): typed model, strict decoder, name resolution, validation. Pure: it imports only the standard library, never `core`; `core` adapts it (`ruleset_v2.go`) |
| `internal/postgres` | Persistence adapter (pgx). Implements the core's repository interfaces; embedded goose migrations |
| `internal/web` | HTTP adapter: routes, handlers, middleware, templates, static assets. Declares the core interfaces it needs |
| `definitions/` | Rules data as JSON with schemas, see [definitions.md](definitions.md) |
| `tools/` | Python tooling for the definitions, icons and the theme contrast check; `tools/dice3d` builds the 3D dice script |
| `tests/` | Browser tests (`e2e`), pull request check tests (`ci`), Tabletop Simulator tests (`tts`); see [testing.md](testing.md) |

## Interfaces live where they are used

Go interfaces are satisfied implicitly, so each package defines the small interface **it** consumes:

- `core` declares `authRepository`, `rulesetRepository`, `campaignRepository`, `companyRepository`, `matchRepository`, ...; `postgres.Store` satisfies them.
- `web` declares `auth`, `campaigns`, `rulesets` and `concepts`. `core.Auth`, `core.Rulesets` and `core.Concepts` satisfy three of them;
  `cmd/server` has a facade (`campaignsFacade`) over the core services for the fourth. `core.Glossaries` serves the glossary pages.

Consequences:

- A layer never imports the layer below it for its types except through the core's plain data types (`core.User`, `core.CampaignSummary`).
- Every layer is tested alone against a generated mock of what it consumes (`make generate`, mockgen with `-typed`; the mocks are
  `mock_*_test.go` files next to the interface).
- A page can be designed before its backend exists: add the method to the web interface, regenerate the mock and test the handler
  against it; the core service comes next and nothing in `web` changes.

## Campaign settings

`core.CampaignSettings` travels through the interfaces the web layer declares: the organiser sets it when creating a campaign, every
player reads it, nobody changes it afterwards, and the zero value is every option off. The web layer only hides controls the
settings do not offer; the core enforces them.

| Flag | Default | Effect | Enforced by |
|---|---|---|---|
| `HeroesDropProfileGear` | off | heroes may discard any wargear, even what is built into their profile | `DropEquipment` |
| `ShakeTheTable` | off | the browser throws the dice and decides the roll (client authoritative) | a thrown roll is refused when off |
| `ManualRolls` | off | players may type in the result of physical dice | a typed roll is refused when off |

`CampaignSettings.RollRefusal(entered, thrown)` holds the rule for the last two so every call site applies the same one. A new
option is added to the struct, to this table, to the campaign form and to the Overview's settings card.

## Backend design

`cmd/server/backend.go` (`newBackend`) builds the services over the Postgres store; it is the only place that knows all of them.

### Layers

- **Loader** (`core.Rules`): given the three hashes of a campaign (`RulesetHashes`: Vanilla, Battle Companies, glossary) returns an
  immutable `RuleSet` built from the stored files (`Rules.For`). Name resolution order is entity, then Battle Companies, then
  vanilla. A name vanilla defines twice resolves to the first occurrence unless the Battle Companies file pins another one
  (`vanillaAmbiguities`). The cache is keyed by the hash triple, built once (singleflight) and shared by every campaign with the same
  triple (`MAX_CACHED_RULESETS`); a request never builds a rule set.
- **`core.Rulesets`** stores uploaded rules files: it recomputes the hash, checks the kind, validates against the JSON schema
  (`ErrInvalidRuleset`; schemas are passed in as bytes and validated with `santhosh-tekuri/jsonschema/v6`), applies the one-edition
  rule for vanilla files (`ErrEditionExists`) and registers the shipped files as defaults at start (`RegisterDefaults`).
- **`core.Campaigns`** implements the campaign side of `web.campaigns` as a facade over focused services that share one `RuleSet` per
  campaign. The facade only routes; each service owns its use cases (table below).
- **Repositories** are declared in `internal/core` (`repositories.go`, `match_repository.go`, `aftermath_repository.go`, `ato_repository.go`, the live repository in `live.go`)
  and implemented in `internal/postgres`. Each is a small type returned by a `Store` accessor because the interfaces share method names.

### Company aggregate and transactions

A company is one aggregate. Every company mutation (spend influence, recruit, buy, level up, treat, reorder, bury) is "load,
apply the rules in memory, save" and is atomic and serialised per company:

```go
// Update runs fn on the company's state inside one transaction holding a row lock on the company, and stores the state fn
// leaves behind when fn returns nil. Read is the same without the lock and without saving.
type companyRepository interface {
    Read(ctx, campaignID, companyID int64) (CompanyState, error)
    ReadRoster(ctx, campaignID int64) ([]CompanyState, error) // every company in one statement, for rating lists
    Update(ctx, campaignID, companyID int64, fn func(*CompanyState) error) error
    Create(ctx, campaignID int64, owner User, c NewCompany) (CampaignCompany, error) // ErrCompanyLimit at c.Cap, ErrCompanyNameTaken
}
```

`CompanyState` is plain data (ids, names, bought items, progress, injuries, resources, pending processes, orders) and contains no
rules. Rules code in `core` is pure: `BuildDetail(RuleSet, CampaignSettings, CompanyState) CompanyDetail` turns state plus rules into
what the pages show (profiles, limits, rating, order), and each action is a function `(RuleSet, settings, *CompanyState, input)
(result, error)`. Refusals are `Refuse(...)` (they wrap `ErrRefused`); the service checks that the user owns the company before
`Update`. Rolls come from an injected `Dice` (`D6()`, `Die(sides)`, `Roll(spec)` for an `NdM` spec of the rules file) so tests are
deterministic (`core.FixedDice`).

A player may own several companies in a campaign (`CampaignSettings.CompaniesPerPlayer`, 1 to 4, default 1, in the settings JSON; the
organiser or an administrator changes it at any time with `Campaigns.SetCompaniesPerPlayer`, and lowering it only refuses new
companies). Every owner action takes the company id after the campaign id, `(ctx, user, campaignID, companyID, ...)`, and resolves and
authorises it through the one helper `companyDeps.ownCompany` / `ownCompanyToChange` (`campaigns.go`): `ErrNotFound` unless the company
is among `CampaignDetail.Companies`, the viewer's own, so another player's company, an organiser probing and a stranger all learn
nothing; the changing variant also refuses a finished campaign. `CompanyService.CreateCompany` checks the cap and the owner's company
names in core and passes `NewCompany.Cap` to `companyRepository.Create`, which repeats both checks in the transaction that holds the
owner's `campaign_members` row (`SELECT ... FOR UPDATE`), so simultaneous creates never exceed the cap (`ErrCompanyLimit`; a name used
by another company of the same owner, case ignored, is `ErrCompanyNameTaken`, an `ErrInvalidCompany`). The listing queries (`List`,
`Get`, `Members`) keep one row per campaign or member and aggregate the companies, oldest first (`CampaignSummary.Companies`,
`CampaignMember.Companies`). Migration `00024_several_companies.sql` drops the `(campaign_id, owner_id)` unique constraint and adds
`companies_owner_idx`; its down refuses while a player owns two companies. Matches, live, aftermath, grants and ATO were already keyed
by company: two companies of one owner may share a side or fight each other, and `core.SamePlayers` / `Match.SamePlayers` tell the web
layer which owners have several companies in a match (a note, never a refusal). See [multi-company-spec.md](multi-company-spec.md).

The services (`internal/core`, one file each) share `companyDeps` (campaign and company repositories, the rule source, the dice):

| Service | Methods | Notes |
|---|---|---|
| `Campaigns` (`campaigns.go`) | `List`, `Create`, `Get`, `Companies`, `Company`, `Settings`, `SetCompaniesPerPlayer` | Only users who may create campaigns (`User.MayCreateCampaigns`) create; empty hashes mean the defaults, an unknown or wrong-kind hash is `ErrInvalidCampaign`. `Points`, `FullPoints` and `Units` of every returned company come from `BuildDetail` |
| `CompanyService` (`companies.go`) | `CompanyProfiles`, `CreateCompany`, `CompanyDetail`, `RenameCompany`, `RenameUnit`, `SaveFormation` | A new company starts with the rules' `campaign.company.startingInfluence` and the profile's Gold; a user who is not in the campaign gets `ErrNotFound` |
| `ArmouryService` (`armoury.go`) | `Armoury`, `BuyEquipment`, `DropEquipment`, `BuyCreature`, `HireWanderer`, `DismissWanderer`, `Carry`, ... | Every purchase re-checks inside `Update` on the locked state |
| `RecruitService` (`recruiting.go`) | `RecruitState`, `RollRecruit`, `ConfirmRecruit`, `ChooseLower`, `RecruitSpecial`, `ChooseOptions`, ... | The recruit in progress is `CompanyState.Recruit` |
| `LevelService` (`levelling.go`) | `LevelUpState`, `RollLevelUp`, `ChooseLevelUp`, `RerollLevelUp`, `PathOptions`, `ChoosePath` | The roll in progress is `UnitState.Pending.Level`; re-rolls spend `CompanyState.Rerolls` |
| `TreatmentService` (`treatment.go`) | `TreatInjury`, `TreatmentState` | A roll waiting for the extra-influence decision is `UnitState.Pending.Treat` |
| `CemeteryService` (`cemetery.go`) | `SaveCemeteryOrder`, `Bury`, `Injure` | The fallen are a frozen `CompanyUnit` snapshot and count towards no limit |
| `Matches`, `AftermathService`, `LiveService`, `ATOService` | see [matches.md](matches.md), [aftermath.md](aftermath.md), [live-match.md](live-match.md) | |
| `Invites`, `Glossaries`, `Concepts` | invite links; glossary pages; the concept graph of the helper popups ([concept-graph.md](concept-graph.md)) | |

`CompanyState` also holds what waits between two requests: `Recruit` (roll, lower-result choice, options choice) and each unit's
`Pending`. They are stored as versioned jsonb.

### Persistence shape

Ids are `BIGINT GENERATED ALWAYS AS IDENTITY`. Unit ids are never reused or shared between roster and cemetery. What is queried or
constrained (ownership, order, resources, membership, hashes) is a column; progress, injuries and pending processes are versioned
jsonb because SQL never looks inside them. Prices and rating figures are derived on read, never stored. The schema is in
`internal/postgres/migrations/` (goose, each with a `Down` section).

| Table | Content |
|---|---|
| `users`, `sessions` | Accounts and sessions ([auth.md](auth.md)) |
| `rulesets` | `hash` primary key, `kind`, `name`, `version`, `size`, `content` (compacted JSON), `uploaded_by` (NULL for shipped files), `is_default` (one default per kind), `created_at` |
| `campaigns`, `campaign_members` | The campaign (status, round, three ruleset hashes with composite foreign keys that also check the file kind, `settings` jsonb) and its members (organiser or player) |
| `campaign_invites` | Invite links: `token_hash` (SHA-256), expiry, `max_uses` (0 is unlimited), `uses`, revoked flag. `Redeem` locks the invite row and a member re-joining does not use it up |
| `companies` | Name, faction, influence, gold, `pack text[]`, `pending jsonb`, saved roster and cemetery orders (`unit_order`, `grave_order` as `bigint[]`); unique `(campaign, owner)`; the owner is a member of the campaign |
| `company_units` | One row per model: `position` (its index in `CompanyState.Units`, unique per company, deferrable), base unit, name, role, level, XP, `progress`, `injuries` and `pending` jsonb |
| `unit_equipment` | Bought and dropped gear only (origins `gear`, `bought`, `dropped`); profile wargear is derived from the rules |
| `fallen_units` | A frozen snapshot of each fallen model; keeps its roster id in `unit_id` (unique, no foreign key) |
| `matches`, `match_sides`, `match_side_companies`, `match_objectives`, `match_events`, `match_end_proposal`, `match_end_confirmation` | The match layer. `matches` holds the number (unique per campaign, allocated under a lock on the campaign row), the scenario, the reported result and a `revision`; the end proposal and its confirmations hold a pending end |
| `grants` | A company's history of awards (cascade with the company) |
| `match_aftermaths`, `company_perks` | The War's Aftermath record per company and match, and the perks a company holds ([aftermath.md](aftermath.md)) |
| `match_live`, `match_live_unit`, `match_live_log` | Live match state: see below |
| `match_ato` | Against the Odds per match: the selection, the scenario Ambush! replaced, a revision |

`Update` on the company repository locks the company row (`FOR UPDATE`), UPDATEs units by id, INSERTs new units (id 0) and DELETEs the
missing ones. `CampaignCompany.Points` and `FullPoints` are zero from the repositories (prices live in the rules files): the service
fills them from `BuildDetail`. A company's Wins/Draws/Losses and a campaign's match count are counted from the tables, and only
ended matches count.

`matchRepository.Update(campaignID, matchID, by, fn)` locks the match row and runs `fn`, where the service applies the access check,
the revision check, `CanEnd` and `ValidateResult`; it bumps the revision, stamps `ended_on` and writes the events itself. `AddGrant`
adds Gold or Influence to the company and Experience to the unit in the same transaction. Against the Odds (`Store.ATO()`): `Update`
locks the match row, stores the scenario Ambush! set (clearing the reported objectives when it changed), the record, and bumps the
match revision in one transaction.

## Live match design

The rules of the mode are in [live-match.md](live-match.md); this section is the technical design. The code: the fold (`live_fold.go`:
`FoldLive`, `LiveState.Check`, `ValidateLog`, `CanUndoEntry`), `LiveService` (`live_service.go`: Init, Live, Apply, Undo, Log,
AftermathDefaults, Subscribe), `LiveHub` (`live_hub.go`), the web handlers in `internal/web/live*.go`, the tables in `internal/postgres/live.go`.

### State is a fold of the log

- Truth is `match_live_unit` (starting values, written once) plus the not-undone rows of `match_live_log`. `FoldLive(inits, entries)
  LiveState` is pure and in `core`; nothing else mutates state, and there is no state cache (a match has hundreds of entries; a fold
  is microseconds). Live state survives a restart: only the hub's pending batches are in memory.
- An entry stores the **action as the player meant it, resolved to deltas**: a "set to" is stored as the delta it meant when applied,
  so entries commute and an undo in the middle never changes what the others mean. A wound declaration is one entry with all its
  targets; its effects (target Wounds and Fate, the attacker's `woundedEnemy`, a kill counted when the target `CountsAsKill`, the fall
  into Field casualties, a mount at 0) are derived by the fold, never stored, so undoing the entry undoes all of them. `CountsAsKill`
  (Hero or Monster) is a column of `match_live_unit`, decided when the match opens, so the fold needs no rules data.
- **Return to the battle** is its own action kind (`return`), not an undo: it clears Retreated and Died and lifts Wounds 0 to 1. It is
  always logged, and undoing the return puts the unit back. Experience flags already earned stay.
- **Apply** (`LiveService.Apply`): inside `LiveRepository.Update`, `fn` folds the snapshot, validates the action against the folded state
  (unit in the Field, target in the Field, Fate available, trackers within `0..start`, text limits; refusals are `ErrRefused`, bad
  input `ErrInvalidLive`), builds the text and `Touches`, and appends. **Undo** removes the entry, **refolds the whole remaining log
  in order checking every entry's preconditions again** (the same validation as Apply); the first that fails makes the undo
  `ErrLiveDepends`. One mechanism, no special cases.
- Permissions: Apply requires live edit access on the **acting** company (owner, organiser, administrator; none when the match or the
  campaign has ended); the targets of a wound may be units of any company in the match. Undo is allowed for the owner of the acting
  company, the owner of any company in `Touches`, the organiser and administrators (`LiveEntry.CanUndo` is computed per viewer).

### Transactions and locking

- `LiveRepository.Update` takes `SELECT ... FROM matches WHERE id=$1 FOR NO KEY UPDATE`, the row lock `matchRepository.Update` also takes,
  so a live write cannot interleave with End match and all writes of a match are serialised. It does not bump `matches.revision`, so a
  tracker never fails a form with a stale revision.
- A cross-company wound is one transaction on the match; no company row is locked, because live state is separate from `CompanyState`.
  `seq` is a per-match counter in `match_live.seq`, incremented under that lock; an undo also takes a seq (`undone_seq`).
- `CreateMatch` initialises the live units (`LiveService.Init`, idempotent); a match that has none yet is initialised by its first read.

### Company lock and creation guards

- `CompanyState.OpenMatch` is filled by the company repository when it loads a state (a join of `match_side_companies` and `matches`
  where the match is not ended) and ignored on write. `CompanyState.Locked()` checks it first ("Match N is running; finish it
  first."), then the aftermaths. Every roster action that changes the company calls it; `RenameUnit`, `SaveFormation`,
  `SaveCemeteryOrder` and organiser grants do not.
- Creating or reopening a match checks "a company is in another open match" inside `matchRepository.Create` and `Update` (which hold the
  campaign row lock); both return `*CompanyBusyError`. The check for a pending aftermath is in the service (`Matches.PendingAftermaths`).

### Aftermath prefill

While the aftermath sheet has no saved input, `AftermathService` builds it from `LiveService.AftermathDefaults` (survived, fled or
casualty, the experience flags and counts) and sets `Aftermath.FromLive`. Once the player saves, the saved input is used and the log is
never read again; the "from the live match" badge shows only while the sheet is the unsaved default.

### Persistence (migrations 00014, 00016, 00021)

| Table | Content |
|---|---|
| `match_live` | `match_id` primary key (cascades from `matches`), `seq` counter, `started_at`. Its existence means the match is initialised |
| `match_live_unit` | Key `(match_id, unit_id)`, `company_id`, `start_wounds`, `start_might`, `start_will`, `start_fate`, `has_mount`, `mount_wounds`, `counts_as_kill`, `absent`. No foreign key on `unit_id` (a unit that dies in the aftermath leaves `company_units`) |
| `match_live_log` | `id`, `match_id`, `seq`, `company_id`, `unit_id`, `kind` (checked against the `LiveKind` values), `payload jsonb` (the `LiveAction`, versioned), `text`, `touches bigint[]`, `by_user_id`, `created_at`, `undone_at`, `undone_by`, `undone_seq` |

Indexes: `UNIQUE (match_id, seq)` and a partial `(match_id, seq DESC) WHERE undone_at IS NULL` for the fold. Rows live and die with the
match (cascade); undone rows stay as the audit.

### Live sync (server-sent events)

`GET /campaigns/{id}/matches/{match}/events` (`internal/web/live_events.go`):

- **Fan-out is in memory.** Streams hold no database connection. One connection outside the pool (`Store.LiveFeed()`, named
  `bcm-live-feed`) runs `LISTEN live_match`, waits with a 30 s timeout then pings, reconnects with backoff from 1 s to 30 s, and tells
  the hub to resync after every reconnect because notifications sent meanwhile are lost.
- **One path.** Writers only `pg_notify('live_match', {"m": match, "c": company, "s": seq, "k": "changed"|"ended"})` inside their
  transaction; the hub is fed by the listener only, so no event is published before the commit and a one-process setup exercises the
  same path as several.
- **Hub** (`core.LiveHub`): subscribers per match; each has a mutex-guarded pending batch and a wake channel of capacity 1, so a slow
  client costs one batch, never a backlog. Caps: 10 streams per user, 50 per match (`ErrRefused`; the handler answers 429 and the client
  polls). `Close()` ends every stream.
- **Handler**: checks that the user can read the campaign and the match is open; sets `Content-Type: text/event-stream`,
  `Cache-Control: no-cache, no-transform`, `X-Accel-Buffering: no`; writes `retry: 3000` and flushes at once; loops on the hub with a
  25 s `: ping`; each write has a 10 s write deadline; a stream ends after 2 h (the client reconnects, which re-checks the session). The
  server has no `WriteTimeout`, no compression wraps the route and `Shutdown` closes the hub.
- **Events**: one `event: company-<id>` per company to refetch, `event: match` for the log card, `event: ended` when the match ended or
  was reopened; `id: <seq>`, `data: {"seq":41}`. On connect the server sends one batch for all companies at the current seq; fetches
  are idempotent, nothing is replayed. Events carry ids only, and every fetch goes through the normal authorised GET.
- **Browser**: htmx `sse` extension on the live banner; each company fragment refetches on `sse:company-<id>`, the log card on `sse:match`. A
  write answers its own tab by swap and carries the new `seq`, so the tab ignores the matching event. The client closes the stream when the
  tab is hidden and reopens it on `visibilitychange` (a browser allows 6 connections per origin over HTTP/1.1), and polls every 10 s when
  `EventSource` fails.
- **Reverse proxy**: `X-Accel-Buffering: no` covers nginx; the ping interval stays below the usual 60 s read timeout.

## The lean decoder and the upload report

`internal/core/lean` reads the Battle Companies file (`schemaVersion` 2). Files: `model.go` and `campaign.go` (the contract: one struct per
concept, tags `json`, `required`, `enum`, `min`/`max`, `rest`), `types.go` and `dice.go` (`Roll`, `Rating`, `RuleRef`, `Scalar`,
`DiceSpec`, the polymorphic values), `jsontree.go` and `bind.go` (the strict binder: JSON to a tree with duplicate-key and depth checks,
then reflection over the model; every error is collected with its path), `defaults.go` and `extends.go` (`Normalise`: extends of company,
unit, gear and path, then the defaults, idempotent), `lower.go` and `resolve.go` (`Lower`, the interface the lower layers implement, and
`Resolve`: every name looked up in the entity's scope, the shared lists and the lower layers), `validate.go` and `validate_charts.go`
(structure, chart coverage by dice spec, parameter ranges), `issue.go` and `closest.go` (`Issue`, `Report`, `Closest` for "did you mean"),
`decode.go` (`Decode`, the one entry point), `encode.go` (the inverse of the binder, for the round-trip test).

`internal/core` adapts it: `ruleset_v2.go` has `DecodeBattleCompanies` and the `lean.Lower` over the vanilla book and the glossary;
`ruleset_report.go` has `RulesetReport` / `RulesetIssue`, the error of a rejected file (plain data, so `web` never imports `lean`);
`LoadRuleSet` and `Rules.build` decode the file (a file of an older format gets one clear issue) and `loadV2` builds the whole rule set
from the typed document with the `*_lean.go` readers; `RuleSet.Doc` carries the document. The aftermath, Against the Odds and scenario
services read their rules from the `RuleSet`, never from the stored bytes. The file format is in [definitions.md](definitions.md) and the
upload flow in [definitions.md](definitions.md#upload-validation).

## Request flow

1. `net/http` `ServeMux` (Go 1.22 method and path patterns, no router dependency).
2. Middleware chain: panic recovery, security headers, request logging, cross-origin (CSRF) protection, request deadline.
3. Per route: `session` (resolves the session cookie, never rejects) or `protected` (also redirects visitors who are not signed in to `/login`).
4. A handler calls the core through its interface and renders a template.

Routes by area (the feature documents list the actions):

| Route | Access | Purpose |
|---|---|---|
| `GET/POST /login`, `POST /logout` | public / session | Sign in and out |
| `GET /`, `GET /nav/campaigns` | signed in | The user's campaigns; the sidebar sub-menu |
| `GET /campaigns/new`, `POST /campaigns` | may create campaigns | New campaign |
| `GET /campaigns/{id}` and `/companies`, `/rankings`, `/matches` | campaign member | The campaign's tabs |
| `/campaigns/{id}/my-company/{company}/...` | the company's owner (404 for anyone else) | One of your companies: recruit, armoury, level up, path, treatment, formation, cemetery, pack, perks; `/my-company` alone is an alias that opens one |
| `/campaigns/{id}/companies/{company}` | campaign member | Another company, read only (your own redirects to `/my-company/{company}`) |
| `/campaigns/{id}/matches/...` | campaign member | Matches, scenario, result, end confirmation, Against the Odds, aftermath ([matches.md](matches.md), [aftermath.md](aftermath.md)) |
| `/campaigns/{id}/matches/{match}/live/...`, `.../events` | campaign member | Live match actions, log, undo and the event stream |
| `/join/{token}` | signed in | Redeem an invite link |
| `GET /glossary`, `/campaigns/{id}/glossary`, `/campaigns/{id}/concepts/...` | signed in | Glossary and helper popups |
| `GET /campaigns/{id}/companies/{company}/tts`, `GET /tts/helper.zip` | campaign member | Tabletop Simulator army list and helper ([tabletop-simulator.md](tabletop-simulator.md)) |
| `GET/POST /account...` | signed in | Account page ([auth.md](auth.md)) |
| `/admin/...` | superuser (a 404 for everybody else) | Users, permissions, rules files ([admin.md](admin.md)) |
| `GET /styleguide` | `STYLEGUIDE=true` | Design tokens and components ([frontend.md](frontend.md)) |
| `GET /static/...` | public | Embedded assets |
| `GET /healthz` | public | Liveness: `200 ok` while the process serves; touches nothing else |
| `GET /readyz` | public | Readiness: pings the database (2 s); `503` when it does not answer |
| `POST /csp-report` | public | Content-Security-Policy violation reports: 4 KB body cap, at most 60 logged a minute |

## Persistence

`postgres.Open` connects with a pgx pool and applies pending migrations (`internal/postgres/migrations/*.sql`, embedded, run by goose
under a Postgres advisory lock, so two instances starting together run each migration once) before the server starts.

## Configuration

Environment variables (loaded by `go-simpler.org/env`; `.env.example` lists them, the Makefile also reads a `.env`):

| Variable | Default | Meaning |
|---|---|---|
| `DATABASE_URL` | required (the Makefile defaults to the compose database) | PostgreSQL connection string |
| `PORT` | `8080` | HTTP port |
| `STATIC_MAX_AGE` | `0s` | Cache lifetime of static files requested without their `?v=` hash (`no-cache` when 0; the templates always add the hash, and those requests are cached for a year) |
| `PERF_TRACE` | `false` | Adds a `Server-Timing` header (time, database statements) to every answer, for `make perf` ([performance.md](performance.md)); development only |
| `STYLEGUIDE` | `false` | Serve `/styleguide`; `make run`, `make dev` and `make demo` set it to `true` |
| `APP_ENV` | `production` | `production` is the default; `development` (set by `make run`, `dev`, `demo`) is for plain http on localhost. Anything else is refused at start |
| `SECURE_COOKIES` | unset | `Secure` session, theme and glossary cookies, and the `Strict-Transport-Security` header. Unset: on in production, off in development. `false` in production is refused at start unless `ALLOW_INSECURE_COOKIES=true` (then it starts with a warning). Behind a TLS proxy `true` is the setting |
| `ALLOW_INSECURE_COOKIES` | `false` | Lets production start with `SECURE_COOKIES=false` (plain http on a private network, a try-out) |
| `MAX_CACHED_RULESETS` | `16` | Parsed rule sets (hash triples) kept in memory, least recently used dropped first; at least 1 |
| `REQUEST_TIMEOUT` | `30s` | Context deadline of every request but the live stream: queries and rule computation stop when it is up or the client leaves, the answer is 503. `0` turns it off; negative is refused at start |
| `RULES_BUILD_TIMEOUT` | `60s` | Deadline of building a parsed rule set from its three files; at least `1s` |
| `APP_TIMEZONE` | unset | IANA zone for visitors with no account zone and no browser zone (UTC when empty) |
| `TRUST_PROXY` | `false` | Count login attempts per `X-Forwarded-For` (last entry) instead of the peer address; only behind a reverse proxy that sets it |
| `CSP_MODE` | `report` | `off`, `report` (`Content-Security-Policy-Report-Only` and the `/csp-report` logger) or `enforce`; see [operations.md](operations.md#content-security-policy-report-mode-then-enforce) |
| `MIGRATE_ON_START` | `true` | Apply pending migrations when the server starts. `false`: the server only checks and refuses to start on a database that is behind; run `server migrate` (`make migrate`) as a deploy step |

## Deploying

**Image.** `docker build -t bcm .` builds in three stages (Tailwind CSS, the static Go binary, a distroless `nonroot` runtime with the
binary and `definitions/`). The container runs as uid 65532 with `APP_ENV=production` and has a `HEALTHCHECK` that runs `server
healthcheck` (GET `/readyz` on its own port). `docker compose up -d --build` (`make up`) is the production setup: `compose.yaml`
(project `bcm`) runs the image next to PostgreSQL on a named volume, read-only, capabilities dropped, no database port; settings in
`.env` (`.env.example`), first account with `make prod-user`; see [operations.md](operations.md). `compose.dev.yaml` is the development
database only (published on 127.0.0.1), which `make run`, `dev`, `demo`, `seed` and the integration tests use while the server runs on the host.

**Behind a reverse proxy (TLS).** Terminate TLS at the proxy and forward to `:8080`. Set `TRUST_PROXY=true` only if the proxy sets
`X-Forwarded-For` itself and replaces any value the client sent; leave `SECURE_COOKIES` unset. Probe `/healthz` for liveness and
`/readyz` for readiness. Without TLS (a private network) set `SECURE_COOKIES=false` and `ALLOW_INSECURE_COOKIES=true`.

**Migrations.** For a controlled deploy set `MIGRATE_ON_START=false` on the servers and run `server migrate` (`make migrate`, or
`docker compose run --rm app migrate`) first: the servers then refuse to start while migrations are pending. A rollback of data is a
restore: take a backup before upgrading ([operations.md](operations.md)).

## Commands

| Command | Does |
|---|---|
| `make run` | Builds the CSS, starts PostgreSQL (docker compose) and runs the server |
| `make dev` | Same with live reload (air) and Tailwind in watch mode |
| `make user EMAIL=… NAME=…` | Creates an account (asks for the password) |
| `make grant EMAIL=… ROLE=…` | Changes an account's role (`user`, `admin`, `superuser`; `CREATE=1` lets a user create campaigns) |
| `make seed [PASSWORD=…]` | Demo accounts, a campaign with companies and matches for local development |
| `make demo [PASSWORD=…]` | **Destructive: empties the development database**, loads the rich demo world ([demo-world.md](demo-world.md)) and runs the server on :8080; the output is kept in `tmp/demo-world.txt` |
| `make db-reset` | **Destructive:** drops the development database (all data) and starts an empty one |
| `make test` | Unit tests (the PostgreSQL tests are skipped) |
| `make test-integration` | All tests, including the PostgreSQL ones, against the separate `bcm_test` database of the compose PostgreSQL; the tests empty it and never touch the development database `bcm` |
| `make e2e` | Browser smoke tests on a scratch copy of the demo world ([testing.md](testing.md)) |
| `make test-db` | Makes sure `bcm_test` exists |
| `make generate` | Regenerates the mocks (and the concept graph) |
| `make validate` | Validates the four rules files: JSON schemas, cross references, the Go validator and the curator checks ([definitions.md](definitions.md)) |
| `make check-ruleset FILE=…` | Checks a Battle Companies file like the upload does and prints every problem (`cmd/rulecheck`) |
| `make fuzz` | Explores the decoder and the dice parser with go's fuzzer ([testing.md](testing.md)) |
| `make css` | Builds `internal/web/static/app.css` (generated, not committed) |
| `make db-up` / `make db-down` | Starts / stops the development PostgreSQL (`compose.dev.yaml`) |
| `make migrate` | Applies pending migrations and exits (`server migrate`) |
| `make backup` / `make restore FILE=… CONFIRM=yes` / `make restore-drill` | `pg_dump` into `backups/`, restore over the database, restore into a scratch database and smoke-check it ([operations.md](operations.md)) |
| `make vendor-check [ONLINE=1]` | The vendored scripts match `internal/web/vendor.json` ([vendored-js.md](vendored-js.md)) |
| `make up` (`make up-app`) / `make prod-down` | Production compose (`compose.yaml`): builds the image and starts it with PostgreSQL / stops it |
| `make prod-user EMAIL=… NAME=… [ROLE=superuser]` | Creates the first account in the production container |
| `make check-env-docs` | Fails when the server reads an environment variable that `compose.yaml`, `compose.dev.yaml` or `.env.example` does not document |
| `make docs-check` | Checks that every relative link in the Markdown files resolves |

## Conventions

- Dependencies are kept to what is needed: pgx, goose, `go-simpler.org/env`, `golang.org/x/crypto` (argon2), `golang.org/x/term`, a JSON
  schema validator, mockgen. Everything else is the standard library.
- Errors that callers must tell apart are sentinel values in `core` (`ErrInvalidName`, `ErrInvalidCredentials`, `ErrEmailTaken`,
  `ErrNotFound`, ...); adapters translate their own errors to them.
- Handlers are small: parse input, call the core, render. Rules live in the core.
- Templates and assets are embedded in the binary (`go:embed`), so one binary is the whole deployment.
