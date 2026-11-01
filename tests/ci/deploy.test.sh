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

# DuckDNS updater (deploy/duckdns/duckdns-update.sh) against stub curl and getent: the token never reaches an argument, the output or the log.
secret=11111111-2222-3333-4444-555555555555
dstubs="$tmp/dstubs"; mkdir -p "$dstubs"
cat > "$dstubs/curl" <<'EOF2'
#!/bin/sh
echo "curl $*" >> "$STUB_LOG"
case " $* " in
  *" -K - "*) cat >> "$STUB_STDIN"; echo "${STUB_ANSWER:-OK}"; [ "${STUB_ANSWER:-OK}" = OK ] || exit 22 ;;
  *) [ -n "${STUB_IP:-}" ] && echo "$STUB_IP" || exit 6 ;;
esac
EOF2
cat > "$dstubs/getent" <<'EOF2'
#!/bin/sh
[ -n "${STUB_DNS:-}" ] && echo "$STUB_DNS STREAM bc-manager-ce.duckdns.org"
exit 0
EOF2
chmod +x "$dstubs"/*
drun() { # env assignments...; state in $tmp/dd/state (set STATE_IP to pre-fill, STATE_AGE_MIN to age it); output in $tmp/dout, curl arguments in $tmp/log, stdin in $tmp/stdin
  local d="$tmp/dd" a STATE_IP="" STATE_AGE_MIN="" NO_TOKEN=""
  for a in "$@"; do case "$a" in STATE_IP=* | STATE_AGE_MIN=* | NO_TOKEN=*) eval "$a" ;; esac; done
  rm -rf "$d"; mkdir -p "$d/state"
  [ -z "${STATE_IP:-}" ] || { echo "$STATE_IP" > "$d/state/duckdns-ip"; touch -t "$(date -d "-${STATE_AGE_MIN:-1} minutes" +%Y%m%d%H%M 2> /dev/null || date -v-"${STATE_AGE_MIN:-1}"M +%Y%m%d%H%M)" "$d/state/duckdns-ip"; }
  [ "${NO_TOKEN:-}" = 1 ] || printf '%s\n' "$secret" > "$d/token"
  : > "$tmp/log"; : > "$tmp/stdin"
  env PATH="$dstubs:$PATH" STUB_LOG="$tmp/log" STUB_STDIN="$tmp/stdin" BCM_DIR="$d" TOKEN_FILE="$d/token" DUCKDNS_DOMAIN=bc-manager-ce "$@" bash "$root/deploy/duckdns/duckdns-update.sh" > "$tmp/dout" 2>&1
  echo $?
}
calls() { grep -c 'token\|duckdns.org/update' "$tmp/log" || true; }
leaks() { grep -c "$secret" "$tmp/dout" "$tmp/log" "$tmp/dd/state/duckdns-ip" 2> /dev/null | awk -F: '{s+=$2} END{print s+0}'; }

check "duckdns first run: exit 0" 0 "$(drun STUB_IP=203.0.113.7 STUB_DNS=198.51.100.1)"
grep -q 'OK name=bc-manager-ce ip=203.0.113.7' "$tmp/dout"; check "duckdns first run: OK line" 0 $?
grep -qF "token=$secret" "$tmp/stdin"; check "duckdns first run: token went in on stdin" 0 $?
check "duckdns first run: token in no output, argument or state" 0 "$(leaks)"
grep -q -- '-K - --fail --silent --show-error --max-time 20' "$tmp/log"; check "duckdns: curl flags" 0 $?
grep -qE -- ' -v|--verbose' "$tmp/log"; check "duckdns: curl not verbose" 1 $?
check "duckdns first run: state written" "203.0.113.7" "$(cat "$tmp/dd/state/duckdns-ip")"

check "duckdns unchanged: exit 0" 0 "$(drun STUB_IP=203.0.113.7 STUB_DNS=203.0.113.7 STATE_IP=203.0.113.7 STATE_AGE_MIN=30)"
grep -q 'unchanged' "$tmp/dout"; check "duckdns unchanged: says so" 0 $?
check "duckdns unchanged: no DuckDNS call" 0 "$(grep -c -- '-K -' "$tmp/log" || true)"

check "duckdns IP changed: exit 0" 0 "$(drun STUB_IP=203.0.113.9 STUB_DNS=203.0.113.7 STATE_IP=203.0.113.7 STATE_AGE_MIN=30)"
check "duckdns IP changed: one DuckDNS call" 1 "$(grep -c -- '-K -' "$tmp/log")"
check "duckdns IP changed: state follows" "203.0.113.9" "$(cat "$tmp/dd/state/duckdns-ip")"

check "duckdns stale DNS, state says current: exit 0" 0 "$(drun STUB_IP=203.0.113.7 STUB_DNS=198.51.100.1 STATE_IP=203.0.113.7 STATE_AGE_MIN=30)"
check "duckdns stale DNS: DuckDNS called" 1 "$(grep -c -- '-K -' "$tmp/log")"
drun STUB_IP=203.0.113.7 STUB_DNS=198.51.100.1 STATE_IP=203.0.113.7 STATE_AGE_MIN=2 > /dev/null
check "duckdns just updated, DNS not settled: no call" 0 "$(grep -c -- '-K -' "$tmp/log" || true)"
drun STUB_IP=203.0.113.7 STUB_DNS=203.0.113.7 STATE_IP=203.0.113.7 STATE_AGE_MIN=1500 > /dev/null
check "duckdns daily refresh: DuckDNS called" 1 "$(grep -c -- '-K -' "$tmp/log")"

check "duckdns KO: exit 1" 1 "$(drun STUB_IP=203.0.113.7 STUB_DNS=198.51.100.1 STUB_ANSWER=KO)"
grep -q 'KO name=bc-manager-ce' "$tmp/dout"; check "duckdns KO: log line" 0 $?
check "duckdns KO: no secret in output" 0 "$(leaks)"
[ -e "$tmp/dd/state/duckdns-ip" ]; check "duckdns KO: no state, so the next run retries" 1 $?
check "duckdns no IP lookup answers: exit 1" 1 "$(drun STUB_DNS=198.51.100.1)"
check "duckdns no IP lookup answers: DuckDNS not called" 0 "$(grep -c -- '-K -' "$tmp/log" || true)"

check "duckdns no token file: exit 0" 0 "$(drun NO_TOKEN=1 STUB_IP=203.0.113.7)"
grep -q 'no token file yet' "$tmp/dout"; check "duckdns no token file: log line" 0 $?
check "duckdns no token file: curl not called" 0 "$(wc -l < "$tmp/log" | tr -d ' ')"
check "duckdns bad name: exit 1" 1 "$(drun DUCKDNS_DOMAIN='a&b' STUB_IP=203.0.113.7)"

# install.sh: REQUIRE_SIGNATURE is validated before anything else runs in a real install; here only the syntax of the kit scripts.
for f in deploy/bcm-update.sh deploy/install.sh deploy/duckdns/duckdns-update.sh deploy/oracle/host-setup.sh deploy/oracle/offsite-oci.sh tests/ci/deploy.test.sh; do
  bash -n "$root/$f"; check "bash -n $f" 0 $?
done
if command -v shellcheck > /dev/null; then
  shellcheck -S warning "$root"/deploy/bcm-update.sh "$root"/deploy/install.sh "$root"/deploy/duckdns/duckdns-update.sh; check "shellcheck" 0 $?
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
