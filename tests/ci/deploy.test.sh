#!/usr/bin/env bash
# Tests of deploy/bcm-update.sh signature verification and of the deploy kit scripts, with stub docker, curl and cosign (no network, no Docker).
# Run: bash tests/ci/deploy.test.sh   (needs bash and coreutils)
set -u
root=$(cd "$(dirname "$0")/../.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fail=0
pass=0
check() { # name expected actual
  if [ "$2" = "$3" ]; then pass=$((pass + 1)); else fail=$((fail + 1)); echo "FAIL: $1 (expected '$2', got '$3')"; fi
}

digest=sha256:$(printf 'a%.0s' $(seq 1 64))
stubs="$tmp/stubs"
mkdir -p "$stubs"
cat > "$stubs/docker" <<'EOF'
#!/bin/sh
echo "docker $*" >> "$STUB_LOG"
exit 0
EOF
cat > "$stubs/curl" <<'EOF'
#!/bin/sh
exit 0
EOF
cat > "$stubs/cosign" <<'EOF'
#!/bin/sh
echo "cosign $*" >> "$STUB_LOG"
[ "${COSIGN_STUB_FAIL:-}" != 1 ]
EOF
chmod +x "$stubs"/*

run() { # name-of-scenario [env assignments...]; runs the updater in a fresh /opt/bcm lookalike, output in $tmp/out, log in $tmp/log
  local d="$tmp/bcm"
  rm -rf "$d"; mkdir -p "$d"
  cp "$root/deploy/bcm-update.sh" "$d/"
  printf 'SITE_ADDRESS=bcm.example.org\nAPP_IMAGE=\n%s' "${ENV_EXTRA:-}" > "$d/.env"
  : > "$tmp/log"
  (cd "$d" && env -u REQUIRE_SIGNATURE PATH="$stubs:$PATH" STUB_LOG="$tmp/log" UPDATE_DIGEST="$digest" "$@" bash ./bcm-update.sh > "$tmp/out" 2>&1)
  echo $?
}

# Off (nothing set): deploys, says so, never calls cosign.
check "off: exit 0" 0 "$(run)"
grep -q "signature verification: off" "$tmp/out"; check "off: log line" 0 $?
grep -q '^cosign' "$tmp/log"; check "off: cosign not called" 1 $?
grep -q "^APP_IMAGE=ghcr.io/iuya/battle-company-manager-ce-v2@$digest" "$tmp/bcm/.env"; check "off: APP_IMAGE written" 0 $?

# On via the environment, signature good: deploys, cosign called with the issuer, the digest reference and the tag-bound identity.
check "on: exit 0" 0 "$(run REQUIRE_SIGNATURE=1)"
grep -q "signature verification: ON, " "$tmp/out"; check "on: log line" 0 $?
grep -q -- "--certificate-oidc-issuer https://token.actions.githubusercontent.com ghcr.io/iuya/battle-company-manager-ce-v2@$digest" "$tmp/log"; check "on: issuer and digest reference" 0 $?
grep -qF -- 'iuya/battle-company-manager-ce-v2/\.github/workflows/release-image\.yml@refs/tags/v' "$tmp/log"; check "on: identity is the release workflow on a tag" 0 $?
grep -q 'docker compose up' "$tmp/log"; check "on: started" 0 $?

# On via .env.
check "on via .env: exit 0" 0 "$(ENV_EXTRA='REQUIRE_SIGNATURE=1
' run)"
grep -q "signature verification: ON" "$tmp/out"; check "on via .env: log line" 0 $?
grep -q '^cosign verify' "$tmp/log"; check "on via .env: cosign called" 0 $?

# On, signature bad: fails closed, nothing pulled or started, APP_IMAGE untouched.
check "bad signature: exit 1" 1 "$(run REQUIRE_SIGNATURE=1 COSIGN_STUB_FAIL=1)"
grep -q "FAILED" "$tmp/out"; check "bad signature: log line" 0 $?
grep -q 'docker' "$tmp/log"; check "bad signature: no docker call" 1 $?
grep -q '^APP_IMAGE=$' "$tmp/bcm/.env"; check "bad signature: APP_IMAGE untouched" 0 $?

# On, cosign missing: fails closed as well.
check "no cosign: exit 1" 1 "$(run REQUIRE_SIGNATURE=1 COSIGN_BIN=/nonexistent/cosign)"
grep -q "REQUIRED" "$tmp/out"; check "no cosign: log line" 0 $?
grep -q 'docker' "$tmp/log"; check "no cosign: no docker call" 1 $?

# A fork verifies against its own workflow (SIGNER_REPO), not this repository's.
run REQUIRE_SIGNATURE=1 SIGNER_REPO=someone/fork > /dev/null
grep -qF -- 'someone/fork/\.github/workflows/release-image\.yml' "$tmp/log"; check "SIGNER_REPO: identity follows" 0 $?

# bcm-backup.sh: the generic off-site hook. A hook (OFFSITE_HOOK, or ./offsite-hook.sh next to the script) gets the new dump path; a failing hook
# fails the job and leaves no last-backup stamp; no hook is fine and the dump stays local.
bk="$tmp/bk"; mkdir -p "$bk/scripts"
cp "$root/deploy/bcm-backup.sh" "$bk/"
cat > "$bk/scripts/backup.sh" <<'EOF2'
#!/bin/sh
mkdir -p "$1" && : > "$1/bcm-20260101.dump"
EOF2
printf '#!/bin/sh\necho "$1" >> "%s/hooklog"\nexit ${HOOK_EXIT:-0}\n' "$tmp" > "$bk/offsite-hook.sh"
chmod +x "$bk"/*.sh "$bk"/scripts/*.sh
bash "$bk/bcm-backup.sh" > /dev/null 2>&1; check "hook: exit 0" 0 $?
check "hook: called with the dump path" "backups/bcm-20260101.dump" "$(cat "$tmp/hooklog")"
[ -f "$bk/state/last-backup" ]; check "hook: stamp written" 0 $?
rm -rf "$bk/state" "$tmp/hooklog"
HOOK_EXIT=3 bash "$bk/bcm-backup.sh" > /dev/null 2>&1; check "failing hook: job fails" 3 $?
[ -f "$bk/state/last-backup" ]; check "failing hook: no stamp" 1 $?
cp "$bk/offsite-hook.sh" "$bk/custom-hook"; rm "$bk/offsite-hook.sh"; rm -f "$tmp/hooklog"
OFFSITE_HOOK="$bk/custom-hook" bash "$bk/bcm-backup.sh" > /dev/null 2>&1; check "OFFSITE_HOOK: used" 0 $?
check "OFFSITE_HOOK: called" "backups/bcm-20260101.dump" "$(cat "$tmp/hooklog")"
rm -f "$tmp/hooklog" "$bk/custom-hook"
bash "$bk/bcm-backup.sh" > /dev/null 2>&1; check "no hook: exit 0" 0 $?
[ -e "$tmp/hooklog" ]; check "no hook: nothing called" 1 $?

# install.sh: REQUIRE_SIGNATURE is validated before anything else runs in a real install; here only the syntax of the kit scripts.
for f in deploy/bcm-update.sh deploy/install.sh deploy/bcm-backup.sh tests/ci/deploy.test.sh; do
  bash -n "$root/$f"; check "bash -n $f" 0 $?
done
if command -v shellcheck > /dev/null; then
  shellcheck -S warning "$root"/deploy/bcm-update.sh "$root"/deploy/install.sh; check "shellcheck" 0 $?
fi

# The workflow: the signing permission (id-token) is written once, in the merge job, and the signing step comes after the manifest list.
wf="$root/.github/workflows/release-image.yml"
[ "$(grep -c "^ *id-token: write" "$wf")" = 1 ]; check "release-image.yml: id-token written once" 0 $?
awk '/^  [a-z-]+:$/ { job = $1 } /^ *id-token: write/ { print job }' "$wf" | grep -qx "merge:"; check "release-image.yml: id-token only on merge" 0 $?
m=$(grep -n "name: Create the manifest list" "$wf" | cut -d: -f1); s=$(grep -n "name: Sign the image digest" "$wf" | cut -d: -f1)
[ -n "$m" ] && [ -n "$s" ] && [ "$s" -gt "$m" ]; check "release-image.yml: signing after the manifest" 0 $?

echo "deploy tests: $pass passed, $fail failed"
[ "$fail" = 0 ]
