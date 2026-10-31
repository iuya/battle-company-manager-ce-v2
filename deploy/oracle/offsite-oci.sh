#!/usr/bin/env bash
# Oracle off-site hook for bcm-backup.sh: copies a dump to a private Object Storage bucket with the instance principal (no API key on the host).
# Installed as /opt/bcm/offsite-hook.sh by deploy/oracle/host-setup.sh; the bucket and namespace names are in /opt/bcm/oci.env
# (BACKUP_BUCKET=..., OCI_NAMESPACE=...). Neither is a secret.
set -euo pipefail
dump="${1:?usage: $0 <dump file>}"
cd "$(dirname "$0")"
# shellcheck disable=SC1091
. ./oci.env
: "${BACKUP_BUCKET:?BACKUP_BUCKET is not set in oci.env}"
: "${OCI_NAMESPACE:?OCI_NAMESPACE is not set in oci.env}"
name="dumps/$(basename "$dump")"
/opt/oci-cli/bin/oci os object put --auth instance_principal --namespace-name "$OCI_NAMESPACE" --bucket-name "$BACKUP_BUCKET" \
  --name "$name" --file "$dump" --force > /dev/null
echo "off-site copy: oci://$OCI_NAMESPACE/$BACKUP_BUCKET/$name"
