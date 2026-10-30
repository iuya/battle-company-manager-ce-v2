#!/usr/bin/env bash
# AWS specific: prepares a fresh Amazon Linux 2023 EC2 instance (arm64 or x86_64), then runs the provider-neutral deploy/install.sh.
# Run as root from a checkout or an unpacked release of this repository, over Session Manager (there is no SSH).
#
#   sudo SITE_ADDRESS=203-0-113-7.sslip.io [BACKUP_BUCKET=my-bucket] deploy/aws/host-setup.sh
#
# What it does: Docker and the compose plugin, 1 GiB swap, Docker log rotation, automatic security updates, the optional S3 off-site hook.
# The SSM agent is part of the AMI. It creates no user, key or secret; the instance role (not keys) is what reaches S3.
set -euo pipefail
[ "$(id -u)" = 0 ] || { echo "run as root" >&2; exit 2; }
src="$(cd "$(dirname "$0")/../.." && pwd)"

dnf install -y docker dnf-automatic
arch="$(uname -m)"
mkdir -p /usr/local/lib/docker/cli-plugins
if [ ! -x /usr/local/lib/docker/cli-plugins/docker-compose ]; then
  v="${COMPOSE_VERSION:-v2.29.7}"
  base="https://github.com/docker/compose/releases/download/$v/docker-compose-linux-$arch"
  curl -fsSL -o /tmp/docker-compose "$base"
  want="$(curl -fsSL "$base.sha256" | cut -d' ' -f1)"
  echo "$want  /tmp/docker-compose" | sha256sum -c - # the release publishes the checksum next to the binary
  install -m 0755 /tmp/docker-compose /usr/local/lib/docker/cli-plugins/docker-compose
  rm -f /tmp/docker-compose
fi
mkdir -p /etc/docker
cat > /etc/docker/daemon.json <<'JSON'
{ "log-driver": "local", "log-opts": { "max-size": "10m", "max-file": "5" }, "live-restore": true, "no-new-privileges": true }
JSON
systemctl enable --now docker

if ! swapon --show | grep -q /swapfile; then
  fallocate -l 1G /swapfile && chmod 600 /swapfile && mkswap /swapfile > /dev/null && swapon /swapfile
  echo '/swapfile none swap sw 0 0' >> /etc/fstab
fi

sed -i 's/^apply_updates = .*/apply_updates = yes/; s/^upgrade_type = .*/upgrade_type = security/' /etc/dnf/automatic.conf
systemctl enable --now dnf-automatic.timer

"$src/deploy/install.sh"

if [ -n "${BACKUP_BUCKET:-}" ]; then
  install -m 0750 "$src/deploy/aws/offsite-s3.sh" /opt/bcm/offsite-hook.sh
  echo "BACKUP_BUCKET=$BACKUP_BUCKET" > /opt/bcm/aws.env
  chmod 600 /opt/bcm/aws.env
fi
