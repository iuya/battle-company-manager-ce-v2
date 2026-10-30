#!/usr/bin/env bash
# Memory readings of a small server: memory and swap, one docker stats snapshot, and whether the kernel killed anything.
#
#   sudo /opt/bcm/memcheck.sh
#
# Prints no secret (container names, sizes, kernel messages about OOM only). Run it after the first start, after a busy evening and
# after an update; swap in use that keeps growing, or any "Out of memory" line, means the host needs more memory.
set -uo pipefail
cd "$(dirname "$0")"
echo "== free -m";           free -m
echo "== swap";              swapon --show || true
echo "== vm.swappiness";     sysctl -n vm.swappiness
echo "== docker stats";      docker stats --no-stream --format 'table {{.Name}}\t{{.MemUsage}}\t{{.MemPerc}}\t{{.CPUPerc}}'
echo "== restarts";          docker compose ps --format 'table {{.Service}}\t{{.Status}}'
echo "== OOM kills (dmesg)"; (dmesg 2>/dev/null || journalctl -k --no-pager 2>/dev/null) | grep -iE 'out of memory|oom-kill|killed process' | tail -n 5 || true
echo "== end (no lines under OOM kills means none)"
