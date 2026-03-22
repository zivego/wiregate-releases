#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

PREVIOUS_VERSION="${WIREGATE_PREVIOUS_VERSION:-}"
if [ -z "$PREVIOUS_VERSION" ]; then
  echo "[release-rollback] WIREGATE_PREVIOUS_VERSION is required" >&2
  exit 1
fi

echo "[release-rollback] rolling back to version: $PREVIOUS_VERSION"
export WIREGATE_VERSION="$PREVIOUS_VERSION"
"$ROOT/scripts/release_deploy.sh"
