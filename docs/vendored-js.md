# Vendored JavaScript

The third-party browser scripts the app ships, where they come from, how the repository checks them and how to update them.

The browser libraries are committed under `internal/web/static/js` (no CDN, no build step for them). Their versions, sources and
checksums are in [`internal/web/vendor.json`](../internal/web/vendor.json); `make vendor-check` (a Go test, also part of `make test`) fails when a file
and the manifest disagree or when a vendored script is not listed. `make vendor-check ONLINE=1` also downloads every upstream source and compares the bytes.

| File | What | Source |
|---|---|---|
| `alpine.min.js` | Alpine.js (standard build; needs `'unsafe-eval'` in the Content-Security-Policy, see [operations.md](operations.md#content-security-policy-report-mode-then-enforce)) | jsDelivr, npm `alpinejs` `dist/cdn.min.js` |
| `htmx.min.js` | htmx | jsDelivr, npm `htmx.org` `dist/htmx.min.js` |
| `sortable.min.js` | SortableJS | jsDelivr, npm `sortablejs` `Sortable.min.js` |
| `dice3d.js` | three.js and cannon-es cut down for the 3D dice, built by `tools/dice3d` (`make dice3d`, needs node) | built here |

## Update procedure

Review the scripts regularly and whenever an advisory appears.

1. Read the `advisories` link of each entry in `vendor.json` and the changelogs since the committed version.
2. Download the new file from the pattern in `source` with the new version, e.g.
   `curl -sL https://cdn.jsdelivr.net/npm/htmx.org@<new>/dist/htmx.min.js -o internal/web/static/js/htmx.min.js`.
   For `dice3d.js` change the versions in `tools/dice3d/package.json` and run `make dice3d`.
3. `shasum -a 256 internal/web/static/js/<file>` and update `version`, `source` and `sha256` in `vendor.json`.
4. `make vendor-check ONLINE=1`, then `make test` and click through the pages the library drives (Alpine: every dialog and tab; htmx: swaps and the live match;
   Sortable: formation drag and drop; dice: the 3D roll) on `make demo`.
5. Commit the file and the manifest together; mention the old and new versions in the message.
