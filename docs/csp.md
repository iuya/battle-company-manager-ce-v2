# Content-Security-Policy: report mode, then enforce

For whoever runs the app in production and decides when to switch the policy from reporting to blocking.

Production starts in `CSP_MODE=report` (the default).
The browser loads everything, sends each violation to `POST /csp-report`, and the app logs one line.

Before the switch, the browser suite runs the whole app under the blocking policy (`make e2e-enforce`) and must show no violation.
A week of report mode covers what a suite cannot: real browsers and real data.

### What to read after a week

The app logs to stdout:

```bash
docker compose logs --since 168h app | grep -E "csp violation|csp reports dropped"
docker compose logs --since 168h app | grep "csp violation" | grep -v chrome-extension | sed 's/.*directive=//' | sort | uniq -c | sort -rn
```

How to read the result:

- **No line, or only lines whose `source=` starts with `chrome-extension://`, `moz-extension://` or `safari-extension://`** (a user's extension, not the app): ready to enforce.
- **A line with `source=` a `/static/js/...` file, or `blocked=inline` or `blocked=eval` on `directive=script-src...`:** something the policy would break.
  `eval` is allowed by `'unsafe-eval'`, so `eval` should not appear.
  Fix it (move the code to a file under `static/js`) and run another week.
- **`directive=img-src`, `font-src`, `connect-src` or `style-src-elem` with a `blocked=` URL of the app's own origin or `data:`/`blob:`:** a source the policy omits.
  Add it to `cspPolicy` in `internal/web/hardening.go`, with a test, before enforcing.
- **`csp reports dropped count=N`:** the limit of 60 reports per minute cut a flood, usually one browser on one broken page.
  Look at the first lines around it.
- Every line shows `disposition=report`. `document=` is the page and `line=` the position in `source=`.
- Reports from a reverse proxy that injects scripts (analytics, a banner) also show here: the proxy's script would be blocked after the switch.

### Switch to enforce

Set one line in the `environment:` block of `compose.yaml`, among the active settings:

```yaml
      CSP_MODE: enforce
```

Then run `docker compose up -d app` (a restart; no migration).

- The same policy now blocks, and `Content-Security-Policy` replaces `Content-Security-Policy-Report-Only`.
- Reports keep arriving (`disposition=enforce`), so read the same log for a day.
- To roll back, set the line to `report` (or `off`) and run the same command. Nothing else depends on it.

The default in the code and in `.env.example` is `report`.


See also: [operations.md](operations.md) (TLS and cookies), [architecture.md](architecture.md) (configuration), [vendored-js.md](vendored-js.md).
