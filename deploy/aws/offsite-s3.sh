#!/usr/bin/env bash
# AWS off-site hook for bcm-backup.sh: copies a dump to a private S3 bucket with the instance role (no access keys on the host).
# Installed as /opt/bcm/offsite-hook.sh by deploy/aws/host-setup.sh; the bucket name is in /opt/bcm/aws.env (BACKUP_BUCKET=...).
set -euo pipefail
dump="${1:?usage: $0 <dump file>}"
cd "$(dirname "$0")"
# shellcheck disable=SC1091
. ./aws.env
: "${BACKUP_BUCKET:?BACKUP_BUCKET is not set in aws.env}"
aws s3 cp --only-show-errors --sse AES256 "$dump" "s3://$BACKUP_BUCKET/dumps/$(basename "$dump")"
echo "off-site copy: s3://$BACKUP_BUCKET/dumps/$(basename "$dump")"
