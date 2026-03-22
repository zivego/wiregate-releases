#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${WIREGATE_VERSION:-}"
if [ -z "${WIREGATE_BACKUP_PASSPHRASE:-}" ] && [ -n "${WIREGATE_BACKUP_PASSPHRASE_FILE:-}" ] && [ -r "${WIREGATE_BACKUP_PASSPHRASE_FILE}" ]; then
  mode="$(stat -c '%a' "${WIREGATE_BACKUP_PASSPHRASE_FILE}" 2>/dev/null || true)"
  if [ -z "$mode" ] || [ $((8#$mode & 077)) -ne 0 ]; then
    echo "[release-upgrade] insecure permissions for secret file: ${WIREGATE_BACKUP_PASSPHRASE_FILE} (mode: ${mode:-unknown})" >&2
    echo "[release-upgrade] expected owner-only permissions such as 600" >&2
    exit 1
  fi
  export WIREGATE_BACKUP_PASSPHRASE
  WIREGATE_BACKUP_PASSPHRASE="$(head -n 1 "${WIREGATE_BACKUP_PASSPHRASE_FILE}" | tr -d '\r\n')"
fi

if [ -z "$VERSION" ]; then
  echo "[release-upgrade] WIREGATE_VERSION is required" >&2
  exit 1
fi
if [ -z "${WIREGATE_BACKUP_PASSPHRASE:-}" ]; then
  echo "[release-upgrade] WIREGATE_BACKUP_PASSPHRASE is required (pre-upgrade backup is mandatory)" >&2
  exit 1
fi

echo "[release-upgrade] creating pre-upgrade encrypted backup"
"$ROOT/scripts/release_backup_create.sh"

echo "[release-upgrade] deploying target version $VERSION"
"$ROOT/scripts/release_deploy.sh"

echo "[release-upgrade] verifying upgraded release"
"$ROOT/scripts/release_verify.sh"

echo "[release-upgrade] upgrade completed"
