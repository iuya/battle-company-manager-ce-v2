#!/usr/bin/env bash
# Keeps a DuckDNS name pointing at this host's public IPv4 address (docs/operations.md, "A DuckDNS name that follows the IP").
# Run by bcm-duckdns.timer as root; DUCKDNS_DOMAIN (the subdomain, e.g. bc-manager-ce) comes from /opt/bcm/duckdns.env.
#
# The token is a secret. It is read from a root-only file (TOKEN_FILE, default /etc/bcm/duckdns.token) and handed to curl on its standard
# input as a config file written by the shell builtin printf, so it is on no command line, in no environment variable and in no file other
# than the token file, and it is never logged. curl's own error text is discarded because it can echo a URL. The log says only OK/KO, the name
# and the IP.
#
# It calls DuckDNS only when needed: the public IP differs from the one set last time (state file), DNS does not resolve the name to the
# public IP (a stale record is fixed even if the state file says otherwise), the last update is older than REFRESH_MINUTES (default 1440),
# or the previous call failed (no state is written on failure, so the next run retries).
set -u
base="${BCM_DIR:-/opt/bcm}"
name="${DUCKDNS_DOMAIN:-}"
name="${name%.duckdns.org}"
token_file="${TOKEN_FILE:-/etc/bcm/duckdns.token}"
state="${STATE_FILE:-$base/state/duckdns-ip}"
refresh="${REFRESH_MINUTES:-1440}"
settle="${SETTLE_MINUTES:-10}"
api="${DUCKDNS_API:-https://www.duckdns.org/update}"
log() { echo "duckdns: $*"; }

[[ "$name" =~ ^[a-z0-9]([a-z0-9-]*[a-z0-9])?$ ]] || { log "KO DUCKDNS_DOMAIN is not a valid subdomain"; exit 1; }
if [ ! -r "$token_file" ]; then
  log "no token file yet ($token_file); see docs/operations.md, \"A DuckDNS name that follows the IP\""
  exit 0
fi
token="$(tr -d '[:space:]' < "$token_file")"
[[ "$token" =~ ^[0-9A-Za-z-]{8,64}$ ]] || { log "KO the token file does not hold a token"; exit 1; }

# Public IPv4 as this host sees it: plain HTTPS lookups, several providers in turn, so that one being down does not stop the updater.
# DuckDNS could detect the address itself, but then this script could not tell whether anything changed. The cloud metadata service is not
# used: it is provider specific and this kit stays provider-neutral.
ip=""
for url in https://api.ipify.org https://checkip.amazonaws.com https://icanhazip.com; do
  cand="$(curl -4 --fail --silent --show-error --max-time 10 "$url" 2> /dev/null | tr -d '[:space:]')"
  if [[ "$cand" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; then ip="$cand"; break; fi
done
[ -n "$ip" ] || { log "KO name=$name no public IP lookup answered"; exit 1; }

dns="$(getent ahostsv4 "$name.duckdns.org" 2> /dev/null | awk 'NR==1{print $1}')"
last="$(cat "$state" 2> /dev/null || true)"
reason=""
if [ "$last" != "$ip" ]; then
  reason="ip changed, was ${last:-unknown}"
elif [ -n "$(find "$state" -mmin +"$refresh" 2> /dev/null)" ]; then
  reason="refresh"
elif [ "$dns" != "$ip" ] && [ -n "$(find "$state" -mmin +"$settle" 2> /dev/null)" ]; then
  reason="DNS answers ${dns:-nothing}"
fi
if [ -z "$reason" ]; then log "unchanged name=$name ip=$ip"; exit 0; fi

# ip= is left empty on purpose: DuckDNS then uses the address the request comes from, which is the one looked up above.
answer="$(printf 'url = "%s?domains=%s&token=%s&ip="\n' "$api" "$name" "$token" \
  | curl -K - --fail --silent --show-error --max-time 20 2> /dev/null | tr -d '[:space:]')"
if [ "$answer" = OK ]; then
  mkdir -p "$(dirname "$state")" && printf '%s\n' "$ip" > "$state.tmp" && mv "$state.tmp" "$state"
  log "OK name=$name ip=$ip ($reason)"
else
  log "KO name=$name ip=$ip ($reason) DuckDNS did not answer OK; retrying on the next run"
  exit 1
fi
