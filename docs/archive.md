# Archives of ended campaigns

Ending a campaign is final, and it turns the campaign into a **read-only archive**: one document that holds what the pages of the campaign show, plus the three rules files cut down to what the campaign used.
Nobody, not an administrator either, can reopen or change an archived campaign.
The rows that held the campaign while it was played are deleted, and the campaign stops pinning its rules files.

[Why](#why) ·
[What ending does](#what-ending-does) ·
[The document](#the-document) ·
[Pruning the rules](#pruning-the-rules) ·
[Reading an archive](#reading-an-archive) ·
[What an archive has and has not](#what-an-archive-has-and-has-not) ·
[Writes are refused](#writes-are-refused) ·
[Campaigns ended before archives](#campaigns-ended-before-archives) ·
[Size](#size) ·
[Tests](#tests)

## Why

- A finished campaign keeps its companies, units, matches, live logs and grants as rows for ever, and its three rules files for as long as one campaign uses them.
- The archive keeps the same view of the campaign in one gzipped document, so the campaign no longer depends on the shape of the rules files or of the tables.
- Ended campaigns do not count towards the number of campaigns an account may organise ([admin.md](admin.md), "Campaign limit").

## What ending does

`Campaigns.SetCampaignStatus(.., finished, name)` checks the organiser or an administrator and the typed name, sets the status and revokes the invitation links.
Then `Archiver.Archive` (`internal/core/archiver.go`) does the rest:

1. Reads the campaign: settings, the three stored rules files, every company's stored state, the matches with their history, the grants and each company's aftermath log.
2. Prunes the rules files to what the campaign used and loads the pruned triple with the app's own loader.
3. Builds every company with the full rules and with the pruned rules and compares units, stats, gear, rules, keywords and auras.
   If anything differs the archive is not made: the campaign stays ended and unarchived (it keeps everything), the error is logged, and `server archive` tries again later.
4. `archiveStore.Save` (`internal/postgres/archive.go`), in **one transaction**: stores the document, keeps the figures the lists show on the campaign and company rows, deletes the matches (with their live data, history, aftermaths and Against the Odds records), the grants, perks, units, cemetery and invitation links, sets `archived_at`, and sets the three rules file columns to NULL.

What stays as rows: the campaign row, its members, and the identity of each company (id, owner, name, faction), because the home list, the member list and the Users page read them.
The rules files the campaign pinned are then unused by it; a superuser's "Prune unused files" removes the ones nothing else uses.

## The document

`core.Archive` is stored as gzipped JSON in `campaign_archives.document` (`bytea`), with a few real columns beside it (`format`, `created_at`, `raw_size`, `packed_size`, `companies`, `matches`, `files`).
It holds:

| Part | What |
|---|---|
| Header | name, description, round, rounds, settings, the three files the campaign was played with (kind, name, version, hash: the Rules box of the Overview) |
| Companies | the stored `CompanyState` of each: resources, roster, equipment, levels, paths, injuries, cemetery |
| Matches | every match with sides, scenario, result, objectives, history |
| Grants, aftermath logs | by company |
| Rules | the three pruned files |

`ArchiveFormat` is its version; a reader refuses a newer one.

## Pruning the rules

`pruneVanilla`, `pruneBC` and `pruneGlossary` (`internal/core/archive.go`) keep the units, gear, rules, paths, powers, scenarios and glossary terms the campaign used, work on generic JSON, and follow what the loader insists on.
There is no list to maintain by hand, and the loader is the judge:

- A **company stays whole** (its charts name units and gear); everything those name stays.
- **Gear** is kept under every spelling it is known by (`ItemKey`), with the gear its own entry names, the armour stages and the weapon modifier with the rule it gives.
- **Rules** the engine reads without a unit carrying them stay: the rings every hero draws (Stand Fast), the rings another replaces, the minor abilities a level-up names (Combat Synergy).
- **Scenarios**: only the ones played, and the ones the Rendezvous and Vantage Point ranges come from. The `archive: true` mark on the pruned Battle Companies file lets its scenario selection tables stay empty (no match is rolled again).
- **Glossary**: the terms the campaign named, and for each the terms it links to or lists under "see also". One level, never the terms of those, so a link inside a text of an archive can be plain text.
- After pruning the triple is loaded; every name the loader misses is added and the files pruned again, until it loads.

## Reading an archive

The pages and services are unchanged; the repositories answer from the document.

- `Header` and `Hashes` of an archived campaign give `archive:<id>:<kind>` hashes. The ruleset repository serves those three "files" from the document, so `core.Rules` builds the *mini rule set* of the archive with the unchanged loader.
- A company read, a roster read, the matches list and a match, a match's history and a company's grants and aftermath log are served from the document when the rows are gone (`companies.archived_at` says so in the query that was made anyway; an empty answer asks `campaigns.archived_at`). A campaign that is not archived pays nothing for it.
- The lists read the figures kept on the rows (`archived_units`, `archived_wins`, `archived_draws`, `archived_losses`, `archived_matches`, `archived_practice`).
- Decoded archives stay in a small LRU (8).

## What an archive has and has not

**Supported, read only, and tested to render exactly as the ended campaign did** (`TestArchivedPagesRenderTheSameAsTheFinishedCampaign`): Overview, Rankings, Matches, each match (sides, scenario, result, history), each company page (roster, cemetery, grants, aftermath log, ratings, tooltips), My company (frozen), the Glossary (only the terms in use) and the helper popups.

**Not part of an archive** (the page answers "this page is not part of it", and the tab or button is not drawn):

- the Armory, recruiting, level-ups, treatment, the pack, paths and every other change to a company
- creating a match or a company, grants, invitations, hand-over, settings
- live matches (tracker, log, end checks), aftermath sheets, Against the Odds
- the Tabletop Simulator export
- perks not used yet, a recruit in progress, the aftermath a company still owed (an archived company is shown as it stood, unlocked)
- the open matches of a campaign ended with matches open stay as unfinished matches; their live view is not kept
- glossary terms that no company used and no used term links to; the helper popups for them

Differences in a page that is supported, each deliberate: the Armory tab and the Manage card are not drawn, the card of a match shows no Against the Odds or aftermath or live log (and an open match with no history no Log card), a term link inside a text is plain text when the pruned glossary lacks the term, and the Rules box shows the original hashes (identifying what was played) although the archive carries its own pruned copy.

## Writes are refused

- The services return `core.ErrArchived` (a refusal: "This campaign is an archive: it has ended and can no longer be changed."): status changes, hand-over, settings, match create and update, grants, company create and update, the aftermath and live services.
- A finished campaign cannot change status again (`SetCampaignStatus`): **there is no reactivation**.
- The pages: `gateArchive` (`internal/web/archive.go`) answers a request for an archived campaign only when it is a GET of a page above; the others get 403 or 404 with the reason.
- The database: triggers on `companies`, `company_units`, `fallen_units`, `company_perks`, `grants`, `matches`, `campaign_invites` and `campaign_members` refuse (SQLSTATE `P0480`) any insert or update for an archived campaign.
- `TestPermissionMatrix` walks every action against the archive.

## Campaigns ended before archives

A campaign ended before this existed, or whose archiving failed, is finished and not archived: it keeps its rows and its files and its pages work as they did.
Archive them with the console command on the host (it asks first, and each archive is final):

```
server archive --campaign 12            # one ended campaign
server archive --campaign all-finished  # every ended campaign without an archive
server archive --campaign all-finished --yes
```
(`go run ./cmd/server archive ...` in development.)

In production: `docker compose run --rm app archive --campaign all-finished` (see [operations.md](operations.md)).

## Size

Measured on the real implementation by `TestArchiveKeepsTheReadOnlyPages` (it logs a `SIZE` line per campaign; `go test ./cmd/server -run TestArchiveKeepsTheReadOnlyPages -v`), on the demo world's campaigns:

| Campaign (companies) | Rows it held | Archive stored (gzip) | Document (JSON) |
|---|---:|---:|---:|
| Fog of War (2) | 12 KB | 52 KB | 224 KB |
| Veterans (2) | 32 KB | 59 KB | 275 KB |
| Two bands (7) | 43 KB | 65 KB | 304 KB |
| Cemetery (2, 7 fallen) | 84 KB | 57 KB | 274 KB |
| Aftermath ward 3 (4) | 101 KB | 58 KB | 276 KB |
| Aftermath ward 2 (6) | 213 KB | 67 KB | 319 KB |
| Aftermath ward 1 (8) | 129 KB | 73 KB | 348 KB |
| Matches: every scenario (28) | 131 KB | 95 KB | 542 KB |

- The rules files the campaign pinned are 1363 KB as JSON, 417 KB stored, once per hash: a campaign on the shipped files leaves them to the others, and the files go away when the last campaign using them is archived (a superuser's "Prune unused files").
  A campaign played with files of its own gives up all of its own copy.
- Each archive carries its own pruned rules (about 45 KB gzipped of the 50 to 95 KB): the price of a document that needs nothing else. For a small campaign the rows were smaller than the archive; for campaigns with many companies, matches, grants and aftermaths they are 2 to 3 times larger.
- The size grows with the companies and matches (the pruned rules grow slowly: 17% of the files for 4 companies, 33% for 32).

## Tests

| Test | What |
|---|---|
| `TestArchive*` (core) | every shipped company alone prunes to a loadable triple; the `archive` mark; wanderers and gear spellings |
| `TestArchiveKeepsTheReadOnlyPages` | the demo campaigns with matches: the data the pages are built from, before and after, the rows deleted, the files released, sizes |
| `TestArchivedPagesRenderTheSameAsTheFinishedCampaign` | 53 rendered pages of 8 campaigns, before and after |
| `TestPermissionMatrix` | every action against an archive |
| `TestSaveArchiveReleasesTheFilesAndFreezesTheCampaign` (postgres) | the transaction: files released, usage list, refusals |
| e2e `112-archive` | ending a campaign in the browser, the archive's pages, the pages it does not have |
