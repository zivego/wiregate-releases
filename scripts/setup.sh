#!/usr/bin/env bash
set -euo pipefail

echo "[setup] verifying basic tool availability"
for cmd in bash find rg; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "[setup] missing required tool: $cmd" >&2
    exit 1
  fi
done

echo "[setup] optional tools (not required in this scaffold): go, node, npm, docker"
for optional in go node npm docker; do
  if command -v "$optional" >/dev/null 2>&1; then
    echo "[setup] found optional tool: $optional"
  else
    echo "[setup] optional tool not found: $optional"
  fi
done

echo "[setup] done"
