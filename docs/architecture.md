# Architecture

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
| `cmd/server` | `main`: configuration, wiring (`backend.go`), graceful shutdown; the `user` and `seed` commands (see [auth.md](auth.md)); `world*.go` is the demo world script ([demo-world.md](demo-world.md)) |
| `internal/core` | Business logic: accounts, sessions, campaign rules, password hashing. Declares the repository interfaces it needs |
| `internal/postgres` | Persistence adapter (pgx). Implements the core's repository interfaces; embedded goose migrations |
| `internal/web` | HTTP adapter: routes, handlers, middleware, templates, static assets. Declares the core interfaces it needs |
| `definitions/` | Rules data as JSON, see [definitions.md](definitions.md) |
| `tools/` | Python tooling for the definitions, icons and the theme contrast check |

## Interfaces live where they are used

Go interfaces are satisfied implicitly, so each package defines the small interface **it** consumes:

- `core` declares `authRepository`; `postgres.Store` satisfies it.
- `web` declares `auth`, `campaigns` and `rulesets`; `core.Auth` satisfies the first, `cmd/server`'s facade over the core services the second, `core.Rulesets` the third (and `core.Glossaries` the glossary one).

Consequences:

- A layer never imports the layer below it for its types except through the core's plain data types (`core.User`,
  `core.CampaignSummary`).
- Every layer is tested alone against a generated mock of what it consumes (`go generate ./...`, mockgen with the
  `-typed` flag; the mocks are `mock_*_test.go` files next to the interface).
- A page can be designed before its real backend exists: add the method to the web interface, regenerate the mock
  (`make generate`) and test the handler against it; the core service comes next and nothing in `web` changes.

## Campaign settings are part of the contract

`core.CampaignSettings` travels through the interfaces the web layer declares: the organiser sets it when creating a campaign
(`CampaignInput.Settings`), every player reads it (`CampaignDetail.Settings`), nobody changes it afterwards, and the zero
value is every option off. A backend stores it with the campaign (a JSON column or one column per flag) and enforces what
it allows; the web layer only hides controls the settings do not offer.

| Flag | Default | Effect | Enforced by |
| --- | --- | --- | --- |
| `HeroesDropProfileGear` | off | heroes may discard any wargear, even what is built into their profile | `DropEquipment` |
| `ShakeTheTable` | off | the browser throws the dice and decides the roll (client authoritative) | a backend refuses a thrown roll (`thrown` in `RollRecruit`, `Treatment.Thrown`) when off |
| `ManualRolls` | off | players may type in the result of physical dice | a backend refuses a typed roll (`RollRecruit`, `TreatInjury`, `RollLevelUp`) when off |

`CampaignSettings.RollRefusal(entered, thrown)` holds the rule for the last two so every backend applies the same one. A new
option is added to the struct, to this table, to the campaign form and to the Overview's settings card.

## Backend design

Everything runs on the real backend; `cmd/server/backend.go` (`newBackend`) builds it over the Postgres store and is the only place
that knows all the services. The core services are `Rulesets`, the loader `Rules` / `RuleSet`, `Campaigns`, `CompanyService`,
`ArmouryService`, `RecruitService`, `LevelService`, `TreatmentService`, `CemeteryService`, `Matches`, `AftermathService`, `Invites` and
`Glossaries`; `campaignsFacade` embeds the first ten (under aliases, so `Matches.Matches` is not hidden by the field name) and so
satisfies `web.campaigns`, because their method names are disjoint. Wiring details: `CompanyService.WithAftermaths` gives the company
page its perks and history, `Matches.AftermathStarted` is the aftermath store's `Started` (a match cannot be reopened once a company
started), and a finished campaign's invite cannot be redeemed (`Redeem` returns `ErrNotFound`). The rules *data* parsers
(`ParseVanilla`, `ParseBattleCompanies`, `RulesetHash`, `RulesetMeta`, `OrderUnits`, progression and injury rules) are in `core`.

### Layers

- **Loader** (`core.Rules`): given the three hashes of a campaign (`RulesetHashes`) returns an immutable `RuleSet`
  `{Vanilla *VanillaBook, BC *BCBook, Glossary}` built from the stored files (`Rules.For`; `RuleSet.Keyword(embedded, name)` applies entity > BC > vanilla). Resolution order entity > BC > vanilla; a name
  vanilla defines twice resolves to the first occurrence unless `data.vanillaAmbiguities` pins another; a non-identical
  duplicate that is not listed logs a warning. Cache keyed by the hash triple, built once (singleflight), shared by every
  campaign with the same triple. Never hits the database per request.
- **`core.Rulesets`** implements `web.rulesets` over `rulesetRepository`: recomputes the hash, checks the kind with
  `RulesetMeta`, validates against the JSON schema (`ErrInvalidRuleset`), applies the vanilla edition rule
  (`ErrEditionExists`), registers the shipped files as defaults at startup (`RegisterDefaults`). The JSON schemas are passed in as bytes
  (`NewRulesets(repo, schemas)`), read from disk by `cmd/server` like the definitions; validation uses
  `santhosh-tekuri/jsonschema/v6`.
- **`core.Campaigns`** implements the whole `web.campaigns` interface as a facade over focused services that share one
  `RuleSet` per campaign: campaigns and membership, `Companies` (create, detail, rename, formation), `Recruiting`,
  `Levelling`, `Armoury`, `Treatment`, `Cemetery`. The facade only routes; each service owns its use cases.
- **Repositories** (declared in `internal/core/repositories.go`, implemented in `internal/postgres`): `rulesetRepository`
  and `campaignRepository` today; `companyRepository` below.

### Company aggregate and transactions

A company is one aggregate. All company mutations (spend influence, recruit, buy, level up, treat, reorder, bury) are
"load, apply rules in memory, save" and must be atomic and serialised per company. The contract is small:

```go
// Update runs fn on the company's state inside one transaction holding a row lock on the company, and stores the state fn
// leaves behind when fn returns nil. Read is the same without the lock and without saving.
type companyRepository interface {
    Read(ctx, campaignID, companyID int64) (CompanyState, error)
    ReadRoster(ctx, campaignID int64) ([]CompanyState, error) // every company's units and resources in a fixed number of queries, for rating lists (no aftermaths, open match, perks, cemetery)
    Update(ctx, campaignID, companyID int64, fn func(*CompanyState) error) error
    Create(ctx, campaignID int64, owner User, c NewCompany) (CampaignCompany, error) // ErrHasCompany on unique (campaign, owner)
}
```

`CompanyState` is plain data (ids, names, bought items, progress, injuries, resources, pending processes, orders) and
contains no rules. Rules code in `core` is pure: `BuildDetail(RuleSet, CampaignSettings, CompanyState) CompanyDetail` turns
state plus rules into what the pages show (profiles, limits, rating, order), and each action is a function
`(RuleSet, settings, *CompanyState, input) (result, error)`. Refusals are `Refuse(...)` (wraps `ErrRefused`); the service checks
that the user owns the company before `Update`. Rolls come from an injected `Dice` (`D6() int`) so tests are deterministic;
`CampaignSettings.RollRefusal` gates typed and thrown rolls.

The services (`internal/core`, one file each) share `companyDeps` (campaign and company repositories, the rule source, the dice):

| Service | Methods (of `web.campaigns`) | Notes |
|---|---|---|
| `Campaigns` (`campaigns.go`) | `List`, `Create`, `Get`, `Companies`, `Company`, `Settings` | Only users who may create campaigns (`User.MayCreateCampaigns`) create; empty hashes are the defaults, an unknown or wrong-kind hash is `ErrInvalidCampaign`. `Points`, `FullPoints` and `Units` of every returned company come from `BuildDetail` (the repositories return 0) |
| `CompanyService` (`companies.go`) | `CompanyProfiles`, `CreateCompany`, `CompanyDetail`, `RenameCompany`, `RenameUnit`, `SaveFormation` | New companies start with `StartingInfluence` (0) and the profile's Gold; "the user's company" is the one `campaignRepository.Get` reports, so a stranger gets `ErrNotFound` |
| `ArmouryService` (`armoury.go`) | `Armoury`, `ArmouryEntries`, `BuyEquipment`, `DropEquipment`, `ArmouryCreatures`, `BuyCreature`, `ArmouryWanderers`, `HireWanderer`, `DismissWanderer`, `Carry` | Every purchase re-checks inside `Update` on the locked state |
| `RecruitService` (`recruiting.go`) | `RecruitState`, `RollRecruit`, `ConfirmRecruit`, `ChooseLower`, `RecruitSpecial`, `PendingChoice`, `ChooseOptions` | The recruit in progress is `CompanyState.Recruit`. A result of several models adds all of them; a "choice" result (take any/above result of the chart, "any 2") reuses the lower-result state (`LowerChoice.Picks`); `pick` is `alt`, `opt:N` or `member` |
| `LevelService` (`levelling.go`) | `LevelUpState`, `RollLevelUp`, `ChooseLevelUp`, `RerollLevelUp`, `PathOptions`, `ChoosePath` | The roll in progress is `UnitState.Pending.Level`; re-rolls spend `CompanyState.Rerolls` |
| `TreatmentService` (`treatment.go`) | `TreatInjury`, `TreatmentState` | A roll waiting for the extra-influence decision is `UnitState.Pending.Treat`; injury counts are `UnitState.Injuries` |
| `CemeteryService` (`cemetery.go`) | `SaveCemeteryOrder`; for the casualty flow (no user): `Bury`, `Injure` | The fallen are a frozen `CompanyUnit` snapshot and count towards no limit |

`CompanyState` holds what waits between two requests: `Recruit` (`RecruitProgress`: roll, lower-result choice, options choice)
and each unit's `Pending`. The repository stores them as versioned jsonb (`companies.pending`, `company_units.pending`).

### Persistence shape (proposal for the database master)

Ids are `BIGINT GENERATED ALWAYS AS IDENTITY`. Unit ids are never reused or shared between roster and cemetery.

| Table | Content |
|---|---|
| `rulesets` | `hash` PK, `kind`, `name`, `version`, `size`, `content` (compacted JSON, `bytea`), `uploaded_by`, `is_default`, `created_at`; unique `(kind) WHERE is_default`; index `(kind, name, version)` |
| `campaigns` | `id`, `name`, `description`, `status`, `round`, `rounds`, `vanilla_hash`, `bc_hash`, `glossary_hash` (FKs to `rulesets`), `settings jsonb`, `created_by`, `created_at` |
| `campaign_members` | `(campaign_id, user_id)` PK, `role` (organiser, player), `joined_at` |
| `campaign_invites` | `token_hash` PK (SHA-256, like sessions), `campaign_id`, `created_by`, `expires_at`, `max_uses`, `uses` |
| `companies` | `id`, `campaign_id`, `owner_id`, `name`, `faction`, `influence`, `gold`, `max_models_override`, `pack jsonb`, `pending jsonb` (recruit roll, lower choice, option choice), `created_at`; unique `(campaign_id, owner_id)` |
| `company_units` | `id`, `company_id`, `position` (unique per company, deferrable), `base` (vanilla unit name), `custom_name`, `role`, `level`, `xp`, `progress jsonb` (`core.Progress`), `injuries jsonb`, `injured bool`, `sent_to_healers bool`, `pending jsonb` (level roll, treatment), `wanderer`, `creature` |
| `unit_equipment` | `unit_id`, `name`, `origin` (`bought` or `dropped_profile`), `for_weapon` (Envenom Weapon target); profile wargear is derived from rules, only deviations are stored |
| `fallen_units` | `id`, `company_id`, `position`, `snapshot jsonb` (profile, wargear, path, level, XP), `fell_in`, `fate`, `fell_at` |
| `matches`, `match_sides`, `match_side_companies`, `match_objectives`, `match_events`, `grants` | the match layer, see below |

Implemented: `rulesets` (00005, `Store.Rulesets()`), `campaigns` and `campaign_members` (00006, `Store.Campaigns()` in
`campaign.go`) and the company tables (00007, `Store.Companies()` in `company.go`). Differences from the proposal above:
`campaign_invites` (00012, `Store.Invites()` in `invite.go`: `Redeem` locks the invite row, and a member re-joining does not use it up; `Lookup` reads the same row for the confirmation page, which names the campaign, says so when the visitor is already in it and shows an invalid link before confirming) has a 64-hex `token_hash` check and `max_uses` 0 for unlimited; the three hash columns have composite foreign keys that also check the file's kind;
`settings` is jsonb with a `v`; a company's owner must be a member of the campaign (foreign key); `pack` is `text[]` and the
saved roster and cemetery orders are `bigint[]` (`unit_order`, `grave_order`); the processes that wait for the player are jsonb `pending` columns on `companies` (the recruit in progress) and `company_units` (a level-up roll, a treatment roll), added by 00011; units keep what is queried in columns and put
progress, injuries and the rest (bonus, actions, spells) in versioned jsonb; `unit_equipment` has origins `gear`, `bought`,
`dropped` (no `for_weapon`); a unit's `position` is its index in `CompanyState.Units` (unique per company, deferrable) and
`Update` locks the company row (`FOR UPDATE`), then UPDATEs by id, INSERTs new units (id 0) and DELETEs the missing ones.
A fallen unit keeps its roster id in `fallen_units.unit_id` (unique, no foreign key) with a frozen `core.CompanyUnit` snapshot.
`CampaignCompany.Points` and `FullPoints` are zero from the repositories (prices live in the rules files, not in SQL): the
service fills them from `BuildDetail`; `Wins/Draws/Losses` and `Matches` stay zero until the matches tables exist.
Matches (00008, `Store.Matches()` in `match.go`, interface `matchRepository` in `core/match_repository.go`): `matches`
holds the number (unique per campaign, allocated under a lock on the campaign row), the scenario (`scenario_kind` NULL = none,
a CHECK keeps one-off text out of rulebook scenarios), the reported result (`decided`, 1-based `winner`, `note`), `ended` /
`ended_on` and `revision`. `match_sides` has the side name and its optional `victory_points`; `match_side_companies` the company
(once per match, composite foreign keys make it a company of the match's campaign), its `slot` in the side and `fled`;
`match_objectives` the completed objectives; `match_events` who ended or reopened it (`detail`: who confirmed the end, or the override and its note); `match_end_proposal` (one per open match: `proposed_by`, `proposed_on`, `notice`, `revision`) and `match_end_confirmation` (one row per company that confirmed: `confirmed_by`, `confirmed_on`, `override`, `note`) hold a pending end (migration 00023; read and written inside `matchStore.Update` with the match, deleted when it ends or is cancelled; a change notifies the live channel with company 0 so every page refetches). `grants` is a company's history of
awards (cascade with the company). The repository only stores: `Update(campaignID, matchID, by, fn)` locks the match row and
runs `fn`, where the service applies `Access`, the revision check, `CanEnd` and `ValidateResult`; it bumps the revision,
stamps `ended_on` and writes the events itself. `AddGrant` adds Gold/Influence to the company and Experience to the unit's XP
in the same transaction. A company's `Wins/Draws/Losses` and the campaign's `Matches` are counted from the tables
(only ended matches count for the record); a result entry exists on read only for companies that fled or completed an objective.
Ruleset notes: Each repository is a small
type returned by a `Store` accessor, because the interfaces share method names (`Get`, `Insert`). `Insert` ignores
`RulesetInfo.Default` (the default is only changed by `SetDefault`, which also checks the kind); `uploaded_by` is `NULL` for
shipped files; `size` is checked against the content length.

Pending processes and progress are `jsonb` because they are opaque to SQL and versioned with the rules code; everything
queried or constrained (ownership, order, resources, membership, hashes) is a column. Company rating figures (`Points`,
`Units`, wins) are derived on read, not stored.

### Order of work

Rules infrastructure first (rulesets store, loader), then campaigns, then the company aggregate and the services in the order
the company page needs them; the last bead (7v5.26) swapped the wiring in `cmd/server` and removed the in-memory `internal/fake`. Dependencies and
acceptance criteria are in the beads of epic `7v5`.

## Live match (backend built; epic `pib`, spec in [live-match.md](live-match.md))

Contract declarations are in `internal/core/live.go`; the rules of the mode are in the spec, this section is the technical design. Built so far: the fold (`live_fold.go`: `FoldLive`, `LiveState.Check`, `ValidateLog`, `CanUndoEntry`), `LiveService` (`live_service.go`: Init, Live, Apply, Undo, Log, AftermathDefaults, Subscribe), `LiveHub` (`live_hub.go`), the company lock (`CompanyState.Locked` checks `OpenMatch` first; level-ups, paths, carry and special recruits call it), the creation guards (`Matches.CompanyAftermaths`, `Matches.LiveInit`, `*CompanyBusyError` from the repository), the aftermath prefill (`AftermathService.WithLive`, `Aftermath.FromLive`) and the wiring in `cmd/server/backend.go` (the hub starts in `main` and is closed by `RegisterOnShutdown`). Live state survives a restart: only the hub's pending batches are in memory, and every new stream starts with a batch for all companies (`internal/postgres/live_restart_test.go`). Not built: the web layer (SSE handler, routes, pages: beads pib.16-21, pib.30).

### State is a fold of the log

- Truth is `match_live_unit` (starting values, written once) plus the not-undone rows of `match_live_log`. `FoldLive(inits, entries) LiveState` is pure
  and in `core`; nothing else mutates state, there is no state cache (a match has hundreds of entries; a fold is microseconds).
- An entry stores the **action as the player meant it, resolved to deltas**: a "set to" is stored as the delta it meant when applied, so entries commute and an
  undo in the middle never changes what the others mean. A wound declaration is one entry with all its targets; its effects (target Wounds and Fate, the target's
  `fatePrevented`, the attacker's `woundedEnemy`, a kill counted when the target `CountsAsKill`, the fall into Field casualties, a mount at 0) are derived by the fold, never stored,
  so undoing the entry undoes all of them. `CountsAsKill` (Hero or Monster) is a column of `match_live_unit`, decided from `CompanyUnit.Role`/`Kind` when the
  match opens, so the fold needs no rules data.
- **Return to the battle** is its own action kind (`return`), not an undo: it clears Retreated/Died and lifts Wounds 0 to 1. This replaces the spec's "undo of
  the entry that moved it, or a manual +1 when not unique": one rule, always logged, and undoing the return puts the unit back. Experience flags already earned stay.
- **Apply** (`LiveService.Apply`): in `LiveRepository.Update`, `fn` folds the snapshot, validates the action against the folded state (unit in the Field, target in the
  Field, Fate available, trackers within `0..start`, `Why`/`Note` limits; refusals are `ErrRefused`, bad input `ErrInvalidLive`), builds the text and `Touches`, and returns
  `LiveChange{Append}`. **Undo** (`LiveService.Undo`): `fn` removes the entry from the list, **refolds the whole remaining log in order, checking every entry's
  preconditions again** (the same validation as Apply); the first that fails makes the undo `ErrLiveDepends`. This one mechanism replaces special cases.
- Both live in `core.LiveService` (`live_service.go`), not in `Matches`: it owns `liveRepository`, `companyRepository`/rules (to compute the starting values) and the
  hub. `web` declares `liveMatches` (the methods in the spec, with the amendments below).

### Transactions and locking

- `LiveRepository.Update` takes `SELECT ... FROM matches WHERE id=$1 FOR NO KEY UPDATE`: the same row lock `matchRepository.Update` takes, so a live write cannot
  interleave with End match (a write after the end is refused with the `Ended` flag read under the lock; the aftermath default never misses an entry), and all writes of a
  match are serialised (a table has a handful of writers). It does **not** bump `matches.revision`, so a tracker never fails a form with a stale revision.
- A cross-company wound is one transaction on the match; no company row is locked, because live state is separate from `CompanyState`. Two players on the same unit
  both succeed (serialised, deltas against the fresh fold). `seq` is a per-match counter in `match_live.seq`, incremented under that lock (no sequence objects, no gaps for appends; an undo also takes a seq, `undone_seq`).
- Permissions: Apply requires `LiveEdit` on the **acting** company (`LiveAccessFor`: owner, organiser, administrator; none when the match or the campaign has ended); the
  targets of a wound may be units of any company in the match. Undo is allowed for the owner of the acting company, the owner of any company in `Touches`, the
  organiser and administrators (`LiveEntry.CanUndo` is computed per viewer).
- `CreateMatch` initialises the live units (`LiveService.Init`, idempotent, after `matchRepository.Create`); the first read of a match opened before the feature calls the same Init
  (`ON CONFLICT DO NOTHING`, rosters are locked so both computations agree). If the eager Init fails the match exists and the next read initialises it.

### Company lock and creation guards

- **One lock rule.** `CompanyState` gets `OpenMatch *OpenMatchRef`, filled by the company repository when it loads a state (a join of `match_side_companies` and `matches` where
  `NOT ended`, like `pendingRefs` for `Aftermaths`) and ignored on write. `CompanyState.Locked()` checks it first ("Match N is running; finish it first.", `LockMessageOpenMatch`), then the aftermaths.
  Every roster action that calls `st.Locked()` today (armoury x5, recruiting, treatment) is locked for free; the missing ones (level-up rolls and choices, path, promote, carry,
  hire, gold changes: audit in pib.12) get the call. `RenameUnit`, `SaveFormation`, `SaveCemeteryOrder` and organiser grants do not call it. `CompanyDetail.OpenMatch` carries it to the page (banner, hidden controls); there is no separate `OpenMatchOf`.
- **Creating and reopening** a match: the check "a company is in another open match" is made **inside** `matchRepository.Create` (it already holds the campaign row lock to number
  the match, so two creates cannot both pass) and inside `Update` when `Ended` turns false (reopen); both return `*CompanyBusyError` (an `ErrRefused`, names company and match).
  The pending-aftermath check is in the service (`Matches.PendingAftermaths func(ctx, campaignID, companyID) ([]AftermathRef, error)`, wired like `AftermathStarted`, message `LockMessage`); question `pib.24`.
- An open match with a pending aftermath of an older one cannot happen: the guard above refuses the second match.

### Aftermath prefill

`AftermathService.view` already calls `DefaultSheet(d.Roster)` while the sheet has no saved input. It becomes: if a `liveDefaults` is wired and the match has live data, build the
sheet from `LiveService.AftermathDefaults(ctx, campaignID, matchID, companyID) ([]ModelSheet, bool, error)` (survived/fled/casualty and the `Gains` flags and counts; `participated` 1;
absent units skipped; a roster unit missing from the live state gets the old default) and set `Aftermath.FromLive = true`. Once the player saves, `rec.Input` is
stored and the log is never read again; the badge is shown only while the sheet is the unsaved default (REVIEW: the spec said a badge on every prefilled value; per value
provenance needs the diff against the defaults and is a later option). Reopening a match recomputes the unsaved default; a started aftermath blocks reopening already.

### Persistence shape (for the database master)

Built in migration `00014_live_match.sql` (`internal/postgres/live.go`: `Store.Live()`, `Store.LiveFeed()`; the open-match guards are in `match.go`, `OpenMatch` is loaded in `company.go`):

| Table | Content |
|---|---|
| `match_live` | `match_id` PK (FK `matches` ON DELETE CASCADE), `seq bigint NOT NULL DEFAULT 0`, `started_at`. Existence = initialised; `seq` is the counter |
| `match_live_unit` | PK `(match_id, unit_id)`, `company_id` (composite FK `(match_id, company_id)` to `match_side_companies`), `start_wounds`, `start_might`, `start_will`, `start_fate` (CHECK >= 0), `has_mount`, `mount_wounds`, `counts_as_kill`, `absent`. No FK on `unit_id` (a unit that dies in the aftermath leaves `company_units`, like `fallen_units`) |
| `match_live_log` | `id` identity PK, `match_id` (FK cascade), `seq`, `company_id`, `unit_id`, `kind` (CHECK in the `LiveKind` values), `payload jsonb` (`{"v":1,...}`, the `LiveAction`), `text`, `touches bigint[]`, `by_user_id` (FK users, SET NULL), `created_at`, `undone_at`, `undone_by` (FK users), `undone_seq` |

| `match_ato` | `match_id` PK, `company_id` (composite FK `(match_id, company_id)` to `match_side_companies`, cascade), `selection jsonb` (`{"v":1,...}`: bonuses, kits, where each kit item went, the temporary Wanderer, the Ambush! scenario), `previous_scenario jsonb` and `had_previous` (the scenario Ambush! replaced), `revision` (saves of the row), `created_at`, `updated_at` |

Against the Odds (00022, `Store.ATO()` in `ato.go`, interface `atoRepository` in `core/ato_repository.go`): one row per match for the lower rated company. `Update` locks the match row, hands the service the match, the record and "a live entry exists", and stores the scenario Ambush! set (clearing the reported objectives when it changed), the record (an empty selection deletes the row) and bumps the match revision, all in one transaction; `Get` and `GameStarted` are the reads of the live service and the aftermath. Migration 00021 allows the Break Point and `itemUsed` log kinds.

`match_live_unit` and `match_live_log` reference `match_live` (cascade, which in turn cascades from `matches`), so a log row cannot exist before `Init`. Reopening a match (`matchRepository.Update`, `Ended` true to false) locks the campaign row like `Create` before the busy-company check. `LiveFeed` names its connection `application_name = bcm-live-feed`.

Indexes: `UNIQUE (match_id, seq)`; partial `(match_id, seq DESC) WHERE undone_at IS NULL` (the log sheet, the fold); `match_live_unit (match_id, company_id)`.
Retention: rows live and die with the match (cascade); no pruning (a few hundred small rows per match; the log stays readable on the ended match page and undone rows are the audit).
`matchRepository.Update` also emits `pg_notify('live_match', ...)` (`c` and `s` 0) with kind `ended` when `Ended` changes either way, in its transaction.
NOTIFY payload `{"m": matchID, "c": companyID, "s": seq, "k": "changed"|"ended"}` (well below the 8000 byte limit); the same transaction emits one per touched company.

### Live sync (SSE)

`GET /campaigns/{id}/matches/{match}/events` (`protected`, GET so no CSRF issue). Feasibility against the current stack, with what must change:

- **Driver**: pgx v5 (pool). Streams hold **no database connection**: all fan-out is in memory. One extra connection outside the pool (`pgx.Connect(ctx, url)`, so a large
  number of streams never starves `pgxpool`) runs `LISTEN live_match` in `liveFeed.Run`: `WaitForNotification` with a 30 s timeout then a ping, reconnect with backoff 1 s to 30 s, and
  `onResync()` after every (re)connect because notifications sent meanwhile are lost. The `Store` keeps the URL for it (`Store.LiveFeed()`).
- **One path, even with one process**: writers only `pg_notify` inside their transaction; the hub is fed by the listener only. No second in-process path, so a single-process
  deployment tests the multi-process one, and no event is published before the commit.
- **Hub** (`core.LiveHub`, pure Go, fed by a `liveFeed`; tested with a fake feed): subscribers per match; each subscriber has a mutex-guarded pending `LiveBatch` and a wake channel of
  capacity 1 (`Next(ctx)` blocks and returns the merged batch), so a slow client costs one batch, never a backlog. A resync marks every subscriber's batch with all of the match's
  companies. Caps checked in `Subscribe`: 10 per user, 50 per match (`ErrRefused`; the handler answers 429 and the client polls). `Close()` on the hub ends every stream.
- **Handler** (`internal/web`): checks the user can read the campaign and the match is open (else 404 / an `ended` event and close); sets `Content-Type: text/event-stream`, `Cache-Control: no-cache, no-transform`,
  `X-Accel-Buffering: no`; writes `retry: 3000` and a comment first and flushes at once so the proxy and `EventSource` open; then loops on `Next`, a 25 s ticker for `: ping`, `ctx.Done()`,
  and a maximum age of 2 h (the client reconnects, which re-checks the session). Each write sets `http.NewResponseController(w).SetWriteDeadline(now+10s)` and flushes (a dead peer does not pin the goroutine).
- **Events**: per batch one `event: company-<id>` per company to refetch and one `event: match` (log card), `id: <seq>`, `data: {"seq":41}`; `event: ended` when the match ended or reopened (the page reloads).
  Event names instead of a company field in the data let htmx triggers (`sse:company-12`) match without JavaScript filter expressions. On connect with `Last-Event-ID` (or without) the server
  sends one batch for all companies at the current seq: the fetches are idempotent, nothing is replayed.
- **Authorisation**: at connect (campaign member, match open). No per-event re-check: events hold ids only and every fetch goes through the normal authorised GETs (REVIEW; the spec asked for a re-check per event).
- **Changes needed in the existing server**: (1) `statusRecorder` in `internal/web/middleware.go` hides `http.Flusher`; add `Unwrap() http.ResponseWriter` so `ResponseController.Flush` works (pib.14).
  (2) `http.Server` has no `WriteTimeout` (only `ReadHeaderTimeout`): keep it that way. (3) `Shutdown` waits for active requests and SSE never ends: wire `httpSrv.RegisterOnShutdown(live.Close)` so streams
  return before the 10 s deadline. (4) No compression middleware may wrap the route. (5) `cmd/server` starts `liveFeed.Run` in a goroutine tied to the same signal context.
- **Reverse proxy**: `X-Accel-Buffering: no` for nginx, the 25 s ping stays below the common 60 s read timeout, Caddy/Traefik flush `text/event-stream` immediately. Over HTTP/1.1 a browser allows 6 connections per origin
  shared by all tabs: the client closes the stream when the tab is hidden and reopens on `visibilitychange` (pib.21), and the 10 per user cap is a backstop.
- **htmx**: the `sse` extension (`hx-ext="sse" sse-connect=".../events"`) on the live banner element; the company fragment has `hx-get=".../companies/{c}/live" hx-trigger="sse:company-{c}"`, the log card `sse:match`, the banner `sse:ended`.
  Writes answer their own tab by htmx swap and put the new `seq` in the fragment, so the tab ignores an event whose seq it already has. Polling fallback `hx-trigger="every 10s"` when `EventSource` errors.

## Request flow

1. `net/http` `ServeMux` (Go 1.22 method + path patterns, no router dependency).
2. Middleware chain: panic recovery, security headers, request logging, cross-origin (CSRF) protection.
3. Per route: `session` (resolves the session cookie, never rejects) or `protected` (also redirects visitors who are not
   signed in to `/login`).
4. A handler calls the core through its interface and renders a template.

Routes:

| Route | Access | Purpose |
|---|---|---|
| `GET /login`, `POST /login` | public (signed-in users are redirected to `/`) | Sign in |
| `POST /logout` | session | Sign out |
| `GET /` | signed in | Main page: the user's campaigns |
| `GET /campaigns/new`, `POST /campaigns` | may create campaigns | New-campaign form and creation |
| `GET /campaigns/{id}` and `/companies`, `/rankings`, `/matches` | signed in | The campaign's tabs |
| `GET /rulesets/{hash}` | signed in | Is this rules file already stored? (JSON, lets the browser skip the upload) |
| `GET /nav/campaigns` | signed in | Items of the sidebar's campaigns sub-menu |
| `GET/POST /campaigns/{id}/my-company/recruit`, `GET /campaigns/{id}/my-company/units/{unit}/armoury`, `POST …/units/{unit}/equipment` | signed in | Recruit and buy equipment windows and their actions |
| `GET /campaigns/{id}/company/new`, `POST /campaigns/{id}/company` | signed in | Create your company |
| `GET /campaigns/{id}/companies/{company}` | signed in | A company's My Company page, read only (own company redirects to My Company) |
| `GET /campaigns/{id}/matches/{match}/events` | campaign member | Live match stream (SSE, `internal/web/live_events.go`): `company-<id>`, `match`, `ended` events with `{"seq":N}`, `: ping` every 25 s, `429` at the caps, 2 h limit |
| `GET /campaigns/{id}/matches/new`, `POST /campaigns/{id}/matches`, `GET /campaigns/{id}/matches/{match}` | signed in | New match, match page |
| `GET /campaigns/{id}/matches/{match}/events` (live design) | signed in, campaign member | Server-Sent Events of a running match, see "Live match" |
| `GET /glossary`, `GET /campaigns/{id}/glossary`, `GET /account` | signed in | Glossary and account pages, see [frontend.md](frontend.md) and [auth.md](auth.md) |
| `GET /admin` | superuser | Redirects to `/admin/users` (every `/admin/*` route, POSTs included, is a 404 for everybody else) |
| `GET /admin/users` | superuser | Accounts list; `?q=&offset=` swapped into the list by htmx (`HX-Target` `user-rows` / `user-more`) |
| `POST /admin/users/{id}/permissions` | superuser | Role and creation flag; swaps the row (422 with the reason) |
| `GET /admin/rulesets` | superuser | Stored rules files with usage |
| `POST /admin/rulesets` | superuser | Upload (multipart `kind`, `file`); swaps the panel |
| `POST /admin/rulesets/{hash}/default`, `/delete`; `POST /admin/rulesets/prune` | superuser | Make default, delete, delete all unused; swap the panel |
| `GET /admin/rulesets/{hash}/download` | superuser | The stored file as an attachment |
| `GET /styleguide` | `STYLEGUIDE=true` (public, off by default) | Design tokens and components, see [frontend.md](frontend.md) |
| `GET /static/...` | public | Embedded assets |
| `GET /healthz` | public | Liveness: `200 ok` while the process serves; touches nothing else. `no-store` |
| `GET /readyz` | public | Readiness: pings the database (2 s); `503 unavailable` when it does not answer (the reason is only logged) |
| `POST /csp-report` | public (exempt from the cross-origin check) | CSP violation reports: 4 KB body cap, at most 60 logged a minute for the whole server, answers 204 |

## Persistence

`postgres.Open` connects with a pgx pool and applies pending migrations (`internal/postgres/migrations/*.sql`, embedded,
run by goose) before the server starts. Tables: `users`, `sessions` (the backend design above adds the rest).

## Configuration

Environment variables (loaded by `go-simpler.org/env`; `.env.example` lists them, the Makefile also reads a `.env`):

| Variable | Default | Meaning |
|---|---|---|
| `DATABASE_URL` | required (the Makefile defaults to the compose database) | PostgreSQL connection string |
| `PORT` | `8080` | HTTP port |
| `STATIC_MAX_AGE` | `0s` | Cache lifetime of static files (`no-cache` when 0) |
| `STYLEGUIDE` | `false` | Serve `/styleguide` (public when on); `make run`, `make dev` and `make demo` set it to `true` |
| `APP_ENV` | `production` | `production` is secure by default; `development` (set by `make run`, `dev`, `demo`) is for plain http on localhost. Anything else is refused at start |
| `SECURE_COOKIES` | unset | `Secure` session, theme and glossary cookies, and the `Strict-Transport-Security` header (one year, no `includeSubDomains`). Unset: on in production, off in development. `false` in production is **refused at start** unless `ALLOW_INSECURE_COOKIES=true` (then it starts with a warning). Behind a TLS proxy `true` is the setting |
| `ALLOW_INSECURE_COOKIES` | `false` | Explicit opt-out that lets production start with `SECURE_COOKIES=false` (plain http on a private network, a try-out) |
| `MAX_CACHED_RULESETS` | `16` | Parsed rule sets (hash triples) kept in memory, least recently used dropped first; at least 1, anything else is refused at start |
| `APP_TIMEZONE` | unset | IANA zone for visitors with no account zone and no browser zone (UTC when empty) |
| `TRUST_PROXY` | `false` | Count login attempts per `X-Forwarded-For` (last entry) instead of the peer address; only behind a reverse proxy that sets it. Production without it logs a warning |
| `CSP_MODE` | `report` | `off`, `report` (`Content-Security-Policy-Report-Only` and the `/csp-report` logger) or `enforce`; see "Content-Security-Policy" below |
| `MIGRATE_ON_START` | `true` | Apply pending migrations when the server starts. `false`: the server only checks and refuses to start on a database that is behind; run `server migrate` (`make migrate`) as a deploy step |

## Deploying

**Image.** `docker build -t bcm .` builds in three stages (Tailwind CSS, the static Go binary, a distroless `nonroot` runtime with the binary and
`definitions/`). The container runs as uid 65532, `APP_ENV=production`, and has a `HEALTHCHECK` that runs `server healthcheck` (GET `/readyz` on its own port, no curl needed).
Plain `docker compose up -d --build` (`make up`) is the production setup: `compose.yaml` (project `bcm`) runs the image next to PostgreSQL on a named volume, read-only, capabilities dropped, no database port; settings in `.env` (`.env.example`), first account with `make prod-user`; see [operations.md](operations.md) ("First start").
`compose.dev.yaml` is the development database only (published on 127.0.0.1, password `bcm`), which `make run`, `dev`, `demo`, `seed` and the integration tests use; they keep running the server on the host.
The runtime image is distroless static (no shell), non-root uid 65532, with the base images pinned by digest and Tailwind by version.

**Behind a reverse proxy (TLS).** Terminate TLS at the proxy (Caddy, nginx, a cloud load balancer) and forward to `:8080`. Set `TRUST_PROXY=true` only if the proxy sets
`X-Forwarded-For` itself and replaces any value the client sent (the app reads the last entry); leave `SECURE_COOKIES` unset (on in production). Probe `/healthz` for liveness and `/readyz` for readiness.
Without TLS (a private network) set `SECURE_COOKIES=false` **and** `ALLOW_INSECURE_COOKIES=true`; without the second the server refuses to start, with it it warns.

**Migrations.** `postgres.Open` runs goose under a Postgres advisory lock, so two instances starting together run each migration once (the second waits, then finds nothing to do;
`TestConcurrentMigrations`). For a controlled deploy set `MIGRATE_ON_START=false` on the servers and run `server migrate` (`make migrate`, or `docker compose run --rm app migrate`) first:
the servers then refuse to start while migrations are pending (`TestSkipMigrationsChecksPending`). Every migration has a `Down` section (`TestMigrationsHaveDown`), but a rollback of data is a
restore: take a backup before upgrading ([operations.md](operations.md)).

## Content-Security-Policy

Sent on every response according to `CSP_MODE` (`internal/web/hardening.go`; the analysis and the staged plan are in [security-and-risks.md](security-and-risks.md) section 1):
`default-src 'self'; script-src 'self' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; form-action 'self'; base-uri 'self'; object-src 'none'; report-uri /csp-report`.
`'unsafe-eval'` is there for Alpine's standard build, style `'unsafe-inline'` for the `style=` attributes.

- **Report-only (default; `make run`, `dev`, `demo` set it).** The browser loads everything and posts violations to `/csp-report`, which logs one `csp violation` line (directive, blocked URI, document, source file, line).
  Use the app for a week (the demo world exercises most pages, including the dice and the live stream) and read the log: `grep "csp violation"`. A `csp reports dropped` line means the rate limit (60 a minute) cut a flood.
- **What to watch.** `script-src` reports with an empty or `inline` blocked URI (an inline script or handler to move into a file or give a nonce), `worker-src` / `blob:` (the dice), `img-src` / `connect-src` (the live stream is `EventSource` on the same origin),
  and reports from browser extensions (their `source-file` is `chrome-extension://...`: ignore them).
- **Switching to enforce.** When a week of use leaves no report that belongs to the app, set `CSP_MODE=enforce` and restart; the same policy then blocks. If something breaks, set it back to `report` (or `off`) and restart: nothing else depends on it.
  The reports keep flowing in enforce mode so a regression is visible.

## Commands

| Command | Does |
|---|---|
| `make run` | Builds the CSS, starts PostgreSQL (docker compose) and runs the server |
| `make dev` | Same with live reload (air) and Tailwind in watch mode |
| `make user EMAIL=… NAME=…` | Creates an account (asks for the password) |
| `make seed [PASSWORD=…]` | Demo accounts, a campaign with companies and matches for local development; see "Demo data and signing in" in [frontend.md](frontend.md) |
| `make demo [PASSWORD=…]` | **Destructive: empties the development database** (`db-reset`; every account, campaign and match in it is gone, take `make backup COMPOSE_FILE=compose.dev.yaml` first if it matters), loads the rich demo world (`server seed --world`, [demo-world.md](demo-world.md)) and runs the server on :8080; the output is kept in `tmp/demo-world.txt` |
| `make db-reset` | **Destructive:** drops the development database (all data) and starts an empty one |
| `make test` | Unit tests (the PostgreSQL tests are skipped) |
| `make test-integration` | All tests, including the PostgreSQL ones, against the separate `bcm_test` database of the compose PostgreSQL (made by `scripts/dev-initdb.sh` on a new volume, or by `make test-db`); the tests empty it and never touch the development database `bcm` |
| `make e2e` | Browser smoke tests on a scratch copy of the demo world ([testing.md](testing.md)) |
| `make test-db` | Makes sure `bcm_test` exists (an older volume has none) |
| `make generate` | Regenerates the mocks |
| `make css` | Builds `internal/web/static/app.css` (generated, not committed) |
| `make db-up` / `make db-down` | Starts / stops the development PostgreSQL (`compose.dev.yaml`) |
| `make migrate` | Applies pending migrations and exits (`server migrate`) |
| `make backup` / `make restore FILE=… CONFIRM=yes` / `make restore-drill` | `pg_dump` into `backups/`, restore over the database, restore into a scratch database and smoke-check it; see [operations.md](operations.md) |
| `make vendor-check [ONLINE=1]` | The vendored scripts match `internal/web/vendor.json`; see [vendored-js.md](vendored-js.md) |
| `make up` (`make up-app`) / `make prod-down` | Production compose (`compose.yaml`): builds the image and starts it with PostgreSQL / stops it, see "Deploying" |
| `make prod-user EMAIL=… NAME=… [ROLE=superuser]` | Creates the first account in the production container |
| `make check-env-docs` | Fails when the server reads an environment variable that `compose.yaml`, `compose.dev.yaml` or `.env.example` does not document |

## Conventions

- Dependencies are kept to what is needed: pgx, goose, `go-simpler.org/env`, `golang.org/x/crypto` (argon2),
  `golang.org/x/term`, mockgen. Everything else is the standard library.
- Errors that callers must tell apart are sentinel values in `core` (`ErrInvalidName`, `ErrInvalidCredentials`,
  `ErrEmailTaken`, `ErrNotFound`…); adapters translate their own errors to them.
- Handlers are small: parse input, call the core, render. Rules live in the core.
- Templates and assets are embedded in the binary (`go:embed`), so one binary is the whole deployment.
