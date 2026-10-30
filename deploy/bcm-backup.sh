#!/usr/bin/env bash
# Nightly job: dump the database, keep KEEP dumps on this disk, then hand the new dump to an optional off-site hook.
#
#   /opt/bcm/bcm-backup.sh
#
# OFFSITE_HOOK (a path in .env or the environment, default /opt/bcm/offsite-hook.sh when it exists and is executable) is called with the
# dump path as its only argument; a failing hook fails the job so the unit shows red. deploy/oracle/offsite-oci.sh is one such hook.
# A backup on the same disk is not a backup (docs/operations.md, "Backup").
set -euo pipefail
cd "$(dirname "$0")"
KEEP="${KEEP:-14}" scripts/backup.sh backups
dump="$(ls -1t backups/bcm-*.dump | head -n 1)"
hook="${OFFSITE_HOOK:-./offsite-hook.sh}"
if [ -x "$hook" ]; then "$hook" "$dump"; else echo "no off-site hook: the dump stays on this disk only" >&2; fi
mkdir -p state && date -u +%s > state/last-backup
