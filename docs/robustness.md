# Robustness: what keeps one failure from taking the app down

For developers and operators who want to know which guards, limits and timeouts protect the running server, and which tests prove them.

A panic in an HTTP handler is caught by the outermost middleware (`recoverer`): it logs the stack, answers 500 and keeps serving.
Everything that runs outside a request needs its own guard, because an unrecovered panic in any goroutine ends the process.

### Goroutines

There are four goroutines.

- **Rule-set build** (`core.Rules.For`) and **live hub** (`LiveHub.Start`) run under `core.safely`.
  A panic is logged with its stack.
  The build then answers an error (and is not cached).
  The hub restarts the Postgres feed after a second, and the restart marks every stream changed.
- **Match page cards** (`internal/web/match_cards.go`) recover and fall back to the lazy fetch.
- **`ListenAndServe`** is handled by net/http.

There are no cleanup loops.
The session cache, the login throttle, the memos and the rule-set cache are bounded by size and prune on access.

The rule-set build has a deadline (`RULES_BUILD_TIMEOUT`, default 60 s, at least 1 s):

- Its database reads and the checks between the parsing stages stop with it.
- The parsers take no context, so a build still inside one at the deadline is given up on.
  The waiters get an error wrapping `context.DeadlineExceeded` (503).
  The failure is not cached, so the next request builds again.
  The parser goroutine is left to end by itself.
- At most 4 such stuck builds are tolerated (`Rules.Stuck`).
  Further ones are refused at once (`ErrBuildsStuck`) instead of piling up goroutines.

### LISTEN/NOTIFY

The live feed uses one connection outside the pool.

- Connect and `LISTEN` have a 15 s deadline.
- An idle connection is pinged every 30 s.
- Any failure reconnects with 1 s to 30 s backoff, logs a warning and re-syncs all streams.
- The hub never blocks on a subscriber.
  Each stream holds one coalesced batch and a one-slot wake channel, so a slow browser costs one batch, never a backlog.
- Publish holds the hub lock only for non-blocking work.
- A stream frees its slot when its request ends (`context.AfterFunc`).
- Caps: 10 streams per user, 50 per match.

### Server limits

Set in `cmd/server/main.go`:

| Limit | Value |
|---|---|
| ReadHeaderTimeout | 5 s |
| ReadTimeout | 60 s |
| WriteTimeout | 120 s |
| IdleTimeout | 120 s |
| MaxHeaderBytes | 64 KB |

- **Live stream.** The live stream (SSE) sets a write deadline per write and lifts the read deadline, so it is not cut (`TestLiveStreamOutlivesTheServerTimeouts`).
- **Request deadline.** Every request but the live stream has a context deadline (`REQUEST_TIMEOUT`, default 30 s, `0` = off; `requestTimeout` in `internal/web/middleware.go`).
  `WriteTimeout` only cuts the connection and never cancels `r.Context()`; this deadline does.
  Queries and rule computation stop when the time is up or the client leaves, and the handler answers 503 (`serverError` maps context errors).
- **Request bodies.** 1 MiB for everything that is not a multipart upload (`limitBody`).
- **Uploads.** Uploads set their own limit (`maxUpload`, 16 MiB) before parsing.
  `core.Rulesets` streams the JSON once before validating it.
  More than 1,000,000 values or more than 64 levels is refused with a 422.
  The shipped files have about 40,000 values and 11 levels; validating costs about 40 times the file size in memory.
- **Hostile uploads.** `TestHostileUploadsAnswer4xx` sends the three upload handlers malformed, truncated, huge, deeply nested, oddly typed and badly encoded files and multipart bodies.
  The answer is always a 4xx with a message.
- **Shutdown.** On SIGINT/SIGTERM the hub closes its streams and in-flight requests get 10 s.

### Database

- Every pooled connection gets `statement_timeout=60s` and `idle_in_transaction_session_timeout=60s`.
  A setting in `DATABASE_URL` wins; migrations run before the limits apply.
  A stuck request therefore cannot hold the match row lock or a pool connection for ever.
- Every transaction begins with `defer tx.Rollback`.
- Handlers pass the request context, so a client that leaves cancels its queries.

### Boot versus run

- Configuration errors, a failed migration, a broken embedded asset table (`staticFiles`) and an unparsable template fail at start (`NewServer`, `run`), never at the first request.
- At run time nothing calls `log.Fatal`.
  The `panic` calls in `cmd/server/world*.go` belong to the `seed` command.

### Robustness tests

- **`TestLiveActionsSurviveExtremeValues`** (cmd/server; needs `TEST_DATABASE_URL`; about a minute with the seeding, `SWEEP_FULL=1` several times longer).
  It puts ONE extreme but plausible value at a time into an otherwise valid form of every live POST.
  - Values: wounds 2^31, negatives, 0, empty, overflowing numbers, the unit of another company, match or campaign, wrong parts, reasons and fields, oversized or NUL notes.
  - Forms: each action kind, undo, courage roll, objective and MVP votes, secret objectives, the end-of-match steps.
  - Each is also repeated against the wrong match, an ended match and a finished campaign.
  - No answer is a 5xx or an empty 4xx.
  - After every kind the folded state is checked: every tracker within 0..start, no negative or overflowing Break Point counter, the log readable.
- **Service bounds** (`LiveState.Check`): trackers 0..start, wounds caused 1 to 9, Fate within what is left, Break correction -9..9 and never below one starting model, turns in sequence.
  Typed courage dice outside 1 to 6 are an `ErrInvalidLive` (422).
- **`TestMalformedInputNeverCrashesAHandler`** (cmd/server; needs `TEST_DATABASE_URL`; about 40 s with the seeding, `SWEEP_FULL=1` tries 23 values and takes longer).
  It sends every route, read from `server.go`, hostile values for every form field the handlers and templates know, and hostile path ids, against the demo world.
  Nothing returns a 5xx or logs a panic.
- Run the packages you touch with `go test -race`.


See also: [operations.md](operations.md), [architecture.md](architecture.md), [testing.md](testing.md).
