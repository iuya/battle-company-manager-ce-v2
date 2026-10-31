#!/usr/bin/env bash
# Oracle Cloud specific: prepares a fresh Ubuntu 24.04 (aarch64) VM.Standard.A1.Flex instance, then runs the provider-neutral deploy/install.sh.
# Run as root from a checkout or an unpacked release of this repository, over an OCI Bastion session or the instance console connection.
#
#   sudo SITE_ADDRESS=203-0-113-7.sslip.io [BACKUP_BUCKET=my-bucket OCI_NAMESPACE=mynamespace] deploy/oracle/host-setup.sh
#
# What it does: Docker (Ubuntu packages) with the compose plugin, the host firewall rules for 80/443 that Oracle's Ubuntu images need,
# a rule that keeps containers away from the instance metadata service, 1 GiB swap, Docker log rotation, automatic security updates and
# the optional Object Storage off-site hook. It creates no user, key or secret; the instance principal (not keys) reaches Object Storage.
set -euo pipefail
[ "$(id -u)" = 0 ] || { echo "run as root" >&2; exit 2; }
[ "$(uname -m)" = aarch64 ] || echo "warning: written for aarch64 (VM.Standard.A1.Flex); continuing on $(uname -m)" >&2
src="$(cd "$(dirname "$0")/../.." && pwd)"

export DEBIAN_FRONTEND=noninteractive
apt-get update -q
apt-get install -y -q docker.io docker-compose-v2 unattended-upgrades iptables-persistent curl util-linux openssl ca-certificates

mkdir -p /etc/docker
cat > /etc/docker/daemon.json <<'JSON'
{ "log-driver": "local", "log-opts": { "max-size": "10m", "max-file": "5" }, "live-restore": true, "no-new-privileges": true }
JSON
systemctl enable docker
systemctl restart docker

# Oracle's Ubuntu images ship /etc/iptables/rules.v4 that accepts SSH and rejects everything else. Insert 80/tcp, 443/tcp and 443/udp
# before the first REJECT/DROP rule, in the live rules and in the saved file (only the file is saved, so Docker's own chains are not persisted).
pos="$(iptables -L INPUT --line-numbers -n | awk '$2=="REJECT"||$2=="DROP"{print $1; exit}')"
for spec in "tcp 80" "tcp 443" "udp 443"; do
  set -- $spec
  iptables -C INPUT -p "$1" --dport "$2" -j ACCEPT 2>/dev/null || iptables -I INPUT "${pos:-1}" -p "$1" --dport "$2" -j ACCEPT
done
if [ -f /etc/iptables/rules.v4 ]; then
  for spec in "tcp 80" "tcp 443" "udp 443"; do
    set -- $spec
    line="-A INPUT -p $1 -m $1 --dport $2 -j ACCEPT"
    grep -qF -- "$line" /etc/iptables/rules.v4 || sed -i "0,/^-A INPUT .*-j \(REJECT\|DROP\)/s//$line\n&/" /etc/iptables/rules.v4
  done
fi

# Containers must not read the instance's credentials from the metadata service: drop their traffic to it, after every Docker start.
cat > /etc/systemd/system/bcm-metadata-block.service <<'UNIT'
[Unit]
Description=Keep containers away from the instance metadata service
After=docker.service
Requires=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/bin/sh -c 'iptables -C DOCKER-USER -d 169.254.169.254 -j DROP 2>/dev/null || iptables -I DOCKER-USER -d 169.254.169.254 -j DROP'

[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable --now bcm-metadata-block.service

if ! swapon --show | grep -q /swapfile; then
  fallocate -l 1G /swapfile && chmod 600 /swapfile && mkswap /swapfile > /dev/null && swapon /swapfile
  echo '/swapfile none swap sw 0 0' >> /etc/fstab
fi
# Swap is a safety net, not working memory: keep the kernel from using it eagerly (persisted).
echo 'vm.swappiness = 10' > /etc/sysctl.d/90-bcm-swap.conf
sysctl -q --system

# Security updates only, with a reboot in a quiet hour when a kernel update asks for it.
cat > /etc/apt/apt.conf.d/52bcm-unattended <<'CONF'
Unattended-Upgrade::Automatic-Reboot "true";
Unattended-Upgrade::Automatic-Reboot-Time "04:30";
CONF
systemctl enable --now unattended-upgrades

"$src/deploy/install.sh"

if [ -n "${BACKUP_BUCKET:-}" ]; then
  : "${OCI_NAMESPACE:?set OCI_NAMESPACE (the Object Storage namespace of the tenancy) together with BACKUP_BUCKET}"
  apt-get install -y -q python3-venv
  python3 -m venv /opt/oci-cli
  /opt/oci-cli/bin/pip install --quiet "oci-cli${OCI_CLI_VERSION:+==$OCI_CLI_VERSION}"
  install -m 0750 "$src/deploy/oracle/offsite-oci.sh" /opt/bcm/offsite-hook.sh
  printf 'BACKUP_BUCKET=%s\nOCI_NAMESPACE=%s\n' "$BACKUP_BUCKET" "$OCI_NAMESPACE" > /opt/bcm/oci.env
  chmod 600 /opt/bcm/oci.env
fi
