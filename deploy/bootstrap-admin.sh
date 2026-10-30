#!/usr/bin/env bash
# Creates the first account of a server running the published image and makes it a superuser. There is no sign-up page.
#
#   sudo /opt/bcm/bootstrap-admin.sh you@example.org "Your Name"
#
# The password is typed at the prompt (twice, no echo); it never goes into an argument, a file or a log.
set -euo pipefail
cd "$(dirname "$0")"
email="${1:?usage: $0 <email> <name>}"
name="${2:?usage: $0 <email> <name>}"
docker compose run --rm app user create --email "$email" --name "$name"
docker compose run --rm app user grant --email "$email" --role superuser --create-campaigns
echo "account ready: $email (superuser)"
