# Releases and the published image

For the maintainer who cuts a release and for anyone who runs the published image: how an image is built, tagged, signed, verified and rolled back.

- [Published image](#published-image)
- [Use the image with compose](#use-the-image-with-compose)
- [How a release is built](#how-a-release-is-built)
- [Make the package public (once)](#make-the-package-public-once)
- [Verify a pushed image](#verify-a-pushed-image)
- [Roll back to an earlier image](#roll-back-to-an-earlier-image)
- [Signed images](#signed-images)

## Published image

Every release tag builds the image in CI and publishes it to the GitHub Container Registry: `ghcr.io/iuya/battle-company-manager-ce-v2`.
It is built from the same `Dockerfile` as the local build (distroless, non-root uid 65532, no secrets baked in), for `linux/amd64` and `linux/arm64`.
The binary knows its release: the workflow passes the tag as the `VERSION` build argument, `server version` prints it and the footer of every page shows it, linked to the release page.
The image carries the OCI labels `source`, `version`, `revision` (the commit), `licenses`, `title` and `description`.

When the image is published, the workflow also creates the **GitHub Release** of the tag.
The title is the tag; the notes are the body of the tagged commit and the image reference.
A tag with a hyphen is a pre-release and never the latest.
Every named tag `vX.Y.Z` has a release page.

| Tag | Moves | Example |
|---|---|---|
| `1.1.0` | never (one release) | pin this in production |
| `1.1` | to the newest patch of that minor | |
| `1` | to the newest release of that major (not published below 1.0.0) | |
| `latest` | to the newest stable release (pre-releases such as `v1.2.0-rc.1` get only their full tag) | |

### Use the image with compose

In `compose.yaml` the app service has two alternative blocks:

- `build: .` / `image: bcm:latest` (active): builds from the checkout.
- A commented `image: ghcr.io/...:latest` with `pull_policy: always`.

To use the published image:

1. Comment the first pair and uncomment the second. Pin `:1.1.0` instead of `:latest` if you want.
2. Run `docker compose pull app && docker compose up -d --wait`.

The rest (database, `.env`, `make prod-user`) is unchanged; `make up` passes `--build`, which is harmless for an image-only service.
Without compose: `docker pull ghcr.io/iuya/battle-company-manager-ce-v2:1.1.0`.
To go back to the local build, swap the lines and run `docker compose up -d --build`.

### How a release is built

`.github/workflows/release-image.yml` runs when a tag `v*` is pushed (`git tag v1.1.0 && git push origin v1.1.0`).
The jobs, in order:

1. `e2e`: the browser tests of `e2e.yml`, called as a reusable workflow (four parallel groups and a final `e2e` job). Any failing group stops the release.
2. `build`: a matrix of two jobs on native runners (no QEMU), `linux/amd64` on `ubuntu-26.04` and `linux/arm64` on `ubuntu-26.04-arm` (free for public repositories).
   Each job builds its image into its runner and smoke-tests it: it starts the image read-only against a PostgreSQL service container and checks `/readyz`, `/healthz`, the login page, the built-in healthcheck and the architecture.
   Only then does it push the image by digest (no tag) with the `GITHUB_TOKEN` (permissions: contents read, packages write) and upload the digest as an artifact.
3. `merge` (tags only): downloads both digests and creates the multi-platform manifest with the tags of the table above (`docker buildx imagetools create`).
   It then inspects the manifest and **signs the manifest digest** keylessly with cosign (see [Signed images](#signed-images); `id-token: write` is granted to this job only).

The build cache (gha) is kept per platform.
Running the workflow by hand from the Actions tab is a dry run: the browser tests and both builds with their smoke tests, no login, no push, and `merge` is skipped.

### Make the package public (once)

The first push creates the package private, and visibility cannot be changed through the API or CI.
On GitHub, open the repository's *Packages*, then the package, then *Package settings*, then *Change visibility*, and choose Public.
*Manage Actions access* should also list this repository with Write.
After that anyone can pull without logging in.

### Verify a pushed image

```bash
docker buildx imagetools inspect ghcr.io/iuya/battle-company-manager-ce-v2:1.1.0     # two platforms: linux/amd64 and linux/arm64
docker pull ghcr.io/iuya/battle-company-manager-ce-v2:1.1.0 && docker inspect --format '{{json .Config.Labels}}' ghcr.io/iuya/battle-company-manager-ce-v2:1.1.0
```

### Roll back to an earlier image

Pin the previous full tag in `compose.yaml` (`image: ...:1.0.3`), then run `docker compose pull app && docker compose up -d --wait`.

Migrations are forward-only.
If the newer version migrated the database, restore the backup taken before the upgrade (see [Restore](backup-restore.md#restore)) rather than only swapping the image.

A bad tag can be removed on the package page (*Versions*, then delete).
Fix forward with a new patch tag rather than re-pushing the same tag.


## Signed images

The `merge` job of the release workflow signs the digest of the multi-platform manifest keylessly with cosign.
There is no key.
The certificate is logged in the public Sigstore transparency log.
It names the workflow `release-image.yml` of this repository on a `vMAJOR.MINOR.PATCH` tag and was issued to GitHub's OIDC provider.
The signature is stored in the registry next to the image.

`bcm-update.sh` verifies the signature before it backs up, changes `.env`, pulls or starts anything.
A failure stops the update with the previous release still running.
Failures are: no `cosign` on the host, no signature, or any other signer or ref.

The first thing the updater logs (`journalctl -u bcm-update`) is which mode ran:

- `signature verification: ON, ... is signed by the release workflow of <owner/repo>`
- `signature verification: off (REQUIRE_SIGNATURE is not 1)`

**Enable the check.** Set `REQUIRE_SIGNATURE=1` in `/opt/bcm/.env` (or in the environment of one run) and put `cosign` on the host `PATH` (`COSIGN_BIN` names another location).

- A provider-specific host setup can do both.
  It downloads the pinned cosign release (`COSIGN_VERSION`, v3.1.4; another version needs its `COSIGN_SHA256`) and checks the SHA-256 before installing it to `/usr/local/bin`.
  `install.sh` writes `REQUIRE_SIGNATURE=1`.
  `REQUIRE_SIGNATURE=0` in the environment of such a setup skips both.
- On another host, install cosign from its release page with the checksum, then run `echo REQUIRE_SIGNATURE=1 >> /opt/bcm/.env`.
- An existing server without the line keeps updating without the check.

**Only signed releases verify.** Images published before signing existed have no signature.
With the check on, `UPDATE_DIGEST` of such an image (a rollback to an old release) is refused too.
Turn the check off (`REQUIRE_SIGNATURE=0` in `.env`) for that deploy and on again after.
A fork that signs with its own workflow sets `SIGNER_REPO=<owner/repo>` next to its `IMAGE_REPO`.

**Verify an image by hand** (cosign installed):

```bash
digest=$(docker buildx imagetools inspect ghcr.io/iuya/battle-company-manager-ce-v2:1.1.0 --format '{{json .Manifest}}' | jq -r .digest)
cosign verify \
  --certificate-identity-regexp '^https://github\.com/iuya/battle-company-manager-ce-v2/\.github/workflows/release-image\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/iuya/battle-company-manager-ce-v2@$digest
```

It prints the verified claims on success and exits non-zero otherwise.

`make deploy-test` (`tests/ci/deploy.test.sh`) runs the updater against stub `docker`, `curl` and `cosign` for the on, off, bad-signature and missing-cosign paths, and checks the workflow's permissions.


See also: [deploy-server.md](deploy-server.md) (the updater that pulls and verifies releases), [backup-restore.md](backup-restore.md), [operations.md](operations.md).
