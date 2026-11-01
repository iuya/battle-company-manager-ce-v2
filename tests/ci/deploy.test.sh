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

# install.sh: REQUIRE_SIGNATURE is validated before anything else runs in a real install; here only the syntax of the kit scripts.
for f in deploy/bcm-update.sh deploy/install.sh deploy/oracle/host-setup.sh deploy/oracle/offsite-oci.sh tests/ci/deploy.test.sh; do
  bash -n "$root/$f"; check "bash -n $f" 0 $?
done
if command -v shellcheck > /dev/null; then
  shellcheck -S warning "$root"/deploy/bcm-update.sh "$root"/deploy/install.sh; check "shellcheck" 0 $?
fi

# The workflow: valid YAML, the signing permission on the merge job only.
if command -v python3 > /dev/null && python3 -c 'import yaml' 2> /dev/null; then
  python3 - "$root/.github/workflows/release-image.yml" <<'EOF'
import sys, yaml
wf = yaml.safe_load(open(sys.argv[1]))
with_oidc = [n for n, j in wf["jobs"].items() if "id-token" in (j.get("permissions") or {})]
assert with_oidc == ["merge"], with_oidc
assert "id-token" not in (wf.get("permissions") or {})
steps = [s.get("name") for s in wf["jobs"]["merge"]["steps"]]
assert "Sign the image digest" in steps and steps.index("Sign the image digest") > steps.index("Create the manifest list")
EOF
  check "release-image.yml: id-token only on merge, signing after the manifest" 0 $?
fi

echo "deploy tests: $pass passed, $fail failed"
[ "$fail" = 0 ]
