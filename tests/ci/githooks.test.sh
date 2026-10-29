#!/usr/bin/env sh
# Tests of .githooks/: builds a throwaway repository, installs the hooks and checks what they accept and refuse.
# Run: sh tests/ci/githooks.test.sh   (needs only git and a POSIX shell)
set -u
root=$(cd "$(dirname "$0")/../.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fail=0
pass=0

check() { # name expected-exit actual-exit
  if [ "$2" = "$3" ]; then pass=$((pass + 1)); else fail=$((fail + 1)); echo "FAIL: $1 (expected exit $2, got $3)"; fi
}

cd "$tmp" && git init -q . && git config user.email t@example.com && git config user.name t
mkdir -p .githooks && cp "$root"/.githooks/* .githooks/ && git config core.hooksPath .githooks

msg() { # message -> exit code of the commit-msg hook
  printf '%s\n' "$1" >"$tmp/msg"
  .githooks/commit-msg "$tmp/msg" >/dev/null 2>&1
  echo $?
}
for ok in "feat: add a thing" "fix(core): a hero keeps gear" "feat(web)!: drop the old route" "docs: describe the rules file" \
  "chore(deps): bump actions" "Merge branch 'x'" 'Revert "feat: x"' "fixup! feat: x"; do
  check "accepts: $ok" 0 "$(msg "$ok")"
done
for bad in "added a thing" "Fix: wrong case" "feature: unknown type" "feat:no space" "feat(): empty scope" "feat" "WIP"; do
  check "refuses: $bad" 1 "$(msg "$bad")"
done

# pre-commit: gofmt and secrets
printf 'package x\nfunc  f( ) {}\n' >bad.go && git add bad.go
if command -v gofmt >/dev/null 2>&1; then
  .githooks/pre-commit >/dev/null 2>&1; check "refuses an unformatted Go file" 1 $?
  gofmt -w bad.go && git add bad.go
  .githooks/pre-commit >/dev/null 2>&1; check "accepts a formatted Go file" 0 $?
fi
printf 'key = "AKIAABCDEFGHIJKLMNOP"\n' >creds.txt && git add creds.txt
.githooks/pre-commit >/dev/null 2>&1; check "refuses an AWS access key id" 1 $?
BCM_SKIP_SECRET_SCAN=1 .githooks/pre-commit >/dev/null 2>&1; check "the scan can be skipped for a fixture" 0 $?
git rm -q --cached creds.txt
printf -- '-----BEGIN RSA PRIVATE KEY-----\n' >k.txt && git add k.txt
.githooks/pre-commit >/dev/null 2>&1; check "refuses a private key" 1 $?
git rm -q --cached k.txt

# pre-push: never to main
printf 'refs/heads/topic abc refs/heads/main def\n' | .githooks/pre-push origin url >/dev/null 2>&1; check "refuses a push to main" 1 $?
printf 'refs/heads/topic abc refs/heads/topic def\n' | .githooks/pre-push origin url >/dev/null 2>&1; check "allows a push to a branch" 0 $?
printf 'refs/tags/v1 abc refs/tags/v1 0000\n' | .githooks/pre-push origin url >/dev/null 2>&1; check "allows pushing a tag" 0 $?

echo "githooks: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
