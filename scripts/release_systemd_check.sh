#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

required_files=(
  "$ROOT/deploy/systemd/wiregate-server.service"
  "$ROOT/deploy/systemd/wiregate.env.example"
  "$ROOT/deploy/nginx/wiregate.conf"
  "$ROOT/docs/implementation/UBUNTU_SYSTEMD_NGINX_RUNBOOK.md"
)

for file in "${required_files[@]}"; do
  if [ ! -s "$file" ]; then
    echo "[release-systemd-check] missing required file: $file" >&2
    exit 1
  fi
done

if command -v systemd-analyze >/dev/null 2>&1 && [ "${WIREGATE_SYSTEMD_ANALYZE:-false}" = "true" ]; then
  systemd-analyze verify "$ROOT/deploy/systemd/wiregate-server.service" >/dev/null
fi

echo "[release-systemd-check] systemd+nginx release artifacts look valid"
