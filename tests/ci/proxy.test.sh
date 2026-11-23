#!/usr/bin/env bash
# The production stack as the internet sees it: the shipped deploy/compose.prod.yaml and deploy/Caddyfile (Caddy -> app -> PostgreSQL) with
# the app image built from this checkout, on local ports with Caddy's own local certificate. Checks what only the real proxy can show:
# HTTP is redirected to HTTPS, the security headers and HSTS arrive, the probes are 404 from outside, a client-supplied X-Forwarded-For
# is replaced (the login throttle counts the real peer), the session cookie is Secure, and no link secret reaches the logs.
# Run: make proxy-test   (needs Docker with compose v2, curl and openssl; APP_IMAGE=<image> skips the build; HTTP_PORT/HTTPS_PORT override 18080/18443)
set -u
root=$(cd "$(dirname "$0")/../.." && pwd)
tmp=$(mktemp -d)
project=bcmproxytest
http=${HTTP_PORT:-18080}
https=${HTTPS_PORT:-18443}
fail=0
pass=0
check() { # name expected actual
  if [ "$2" = "$3" ]; then pass=$((pass + 1)); else fail=$((fail + 1)); echo "FAIL: $1 (expected '$2', got '$3')"; fi
}
contains() { # name needle haystack
  case "$3" in *"$2"*) pass=$((pass + 1)) ;; *) fail=$((fail + 1)); echo "FAIL: $1 (no '$2' in: $(printf '%s' "$3" | head -c 300))" ;; esac
}
compose() { docker compose -p "$project" -f "$tmp/compose.yaml" --env-file "$tmp/.env" "$@"; }
cleanup() { compose down -v --remove-orphans >/dev/null 2>&1; rm -rf "$tmp"; }
trap cleanup EXIT

image=${APP_IMAGE:-}
if [ -z "$image" ]; then
  image=bcm:proxytest
  echo "building $image"
  docker build -q -t "$image" --build-arg VERSION=proxytest --build-arg COMMIT=0123456789abcdef0123456789abcdef01234567 "$root" >/dev/null || { echo "FAIL: the image does not build"; exit 1; }
fi

# The shipped files, with only the published ports moved to loopback ones (and no UDP: HTTP/3 is not under test).
cp "$root/deploy/Caddyfile" "$tmp/Caddyfile"
sed -e "s|\"80:80\"|\"127.0.0.1:$http:80\"|" -e "s|\"443:443\"|\"127.0.0.1:$https:443\"|" -e '/443:443\/udp/d' "$root/deploy/compose.prod.yaml" > "$tmp/compose.yaml"
printf 'APP_IMAGE=%s\nPOSTGRES_PASSWORD=proxytest\nSITE_ADDRESS=localhost\nCSP_MODE=enforce\n' "$image" > "$tmp/.env"

compose up -d >"$tmp/up.log" 2>&1 || { cat "$tmp/up.log"; echo "FAIL: the stack does not start"; exit 1; }
base=https://localhost:$https
ready=
for _ in $(seq 1 90); do
  if [ "$(curl -sk -o /dev/null -w '%{http_code}' "$base/login")" = 200 ]; then ready=1; break; fi
  sleep 2
done
[ -n "$ready" ] || { compose logs --tail 40; echo "FAIL: $base/login never answered 200"; exit 1; }

# 1. HTTP goes to HTTPS
out=$(curl -s -o /dev/null -w '%{http_code} %{redirect_url}' "http://localhost:$http/login")
check "http redirects to https" "308 https://localhost/login" "$out"

# 2. The headers of an answer through the proxy
hdr=$(curl -sk -D - -o /dev/null "$base/login" | tr -d '\r')
contains "HSTS" "strict-transport-security:" "$(printf '%s' "$hdr" | tr 'A-Z' 'a-z')"
contains "frame options" "x-frame-options: deny" "$(printf '%s' "$hdr" | tr 'A-Z' 'a-z')"
contains "nosniff" "x-content-type-options: nosniff" "$(printf '%s' "$hdr" | tr 'A-Z' 'a-z')"
contains "CSP enforced" "content-security-policy:" "$(printf '%s' "$hdr" | tr 'A-Z' 'a-z')"
check "no Server header" "0" "$(printf '%s' "$hdr" | grep -ci '^server:')"
check "no Server-Timing header" "0" "$(printf '%s' "$hdr" | grep -ci '^server-timing:')"

# 3a. /version is public through the proxy: the release tag and commit the image was built from, nothing else
ver=$(curl -sk "$base/version")
if [ -z "${APP_IMAGE:-}" ]; then
  check "/version through the proxy" '{"commit":"0123456789abcdef0123456789abcdef01234567","version":"proxytest"}' "$(printf '%s' "$ver" | jq -cS .)"
else # an image built elsewhere: its values are not known here, only the shape
  check "/version through the proxy has the version and the commit, nothing else" '["commit","version"]' "$(printf '%s' "$ver" | jq -cS 'keys')"
fi

# 3. The probes are for the host, not the internet
for p in /healthz /readyz /metrics; do check "$p is 404 from outside" 404 "$(curl -sk -o /dev/null -w '%{http_code}' "$base$p")"; done

# 4. A forged X-Forwarded-For does not give a client a fresh login budget: the throttle counts the peer Caddy saw
codes=
for i in 1 2 3 4 5 6 7; do
  codes="$codes $(curl -sk -o /dev/null -w '%{http_code}' -H "X-Forwarded-For: 10.9.8.$i" --data-urlencode 'email=nobody@example.com' --data-urlencode 'password=a wrong password' "$base/login")"
done
check "login failures counted per real client (5 refused attempts, then 429)" " 422 422 422 422 422 429 429" "$codes"

# 5. The demo accounts cannot be seeded into a production container unless forced; forced, they are there
out=$(compose exec -T app /app/server seed 2>&1); st=$?
check "seed refuses production" 1 "$st"
contains "seed says why" "refusing to seed" "$out"
compose exec -T app /app/server seed --force >"$tmp/seed.log" 2>&1 || { cat "$tmp/seed.log"; fail=$((fail + 1)); echo "FAIL: forced seed"; }

# 6. A real login: the session cookie is Secure and HttpOnly
hdr=$(curl -sk -D - -o /dev/null --data-urlencode 'email=admin@example.com' --data-urlencode 'password=demo-password' "$base/login" | tr -d '\r')
check "login redirects" 303 "$(printf '%s' "$hdr" | head -1 | cut -d' ' -f2)"
cookie=$(printf '%s' "$hdr" | grep -i '^set-cookie: bcm_session=')
contains "session cookie is Secure" "Secure" "$cookie"
contains "session cookie is HttpOnly" "HttpOnly" "$cookie"

# 7. Link secrets never reach a log (the app's or Caddy's)
secret=Zq9SecretLinkToken7Xw
for p in /register/$secret /recover/$secret /join/$secret "/register/$secret?next=1"; do curl -sk -o /dev/null "$base$p"; done
curl -skL -o /dev/null "$base/join/$secret" # signed out: sent on to /login?next=%2Fjoin%2F<secret>, which the browser then requests
curl -sk -o /dev/null -H "Referer: $base/register/$secret" "$base/login" # the assets of a link page carry its address as the Referer
curl -sk -o /dev/null -H "Cookie: bcm_session=CookieValueNeverLogged9" -H "Authorization: Bearer AuthValueNeverLogged9" "$base/login"
sleep 1
logs=$(compose logs caddy app 2>&1)
check "no link secret in the logs" 0 "$(printf '%s' "$logs" | grep -c "$secret")"
printf '%s' "$logs" | grep "$secret" | head -2 | cut -c1-600 | sed 's/^/  leaked: /'
check "no cookie or authorization value in the logs" 0 "$(printf '%s' "$logs" | grep -c "NeverLogged9")"
check "the requests were logged at all" 1 "$([ "$(printf '%s' "$logs" | grep -c '/register/')" -gt 0 ] && echo 1 || echo 0)"

echo "proxy tests: $pass passed, $fail failed"
[ "$fail" = 0 ]
