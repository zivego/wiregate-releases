#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PROJECT_NAME="${WIREGATE_RELEASE_PROJECT_NAME:-wiregate-release}"
BACKUP_DIR="${WIREGATE_BACKUP_DIR:-$ROOT/.wiregate-backups}"
PASSPHRASE="${WIREGATE_BACKUP_PASSPHRASE:-}"
OUTPUT_FILE="${WIREGATE_BACKUP_OUTPUT:-}"
ENV_FILE="${WIREGATE_RELEASE_ENV_FILE:-$ROOT/deploy/compose/release.stack.env}"
if [ ! -f "$ENV_FILE" ] && [ -f "$ROOT/deploy/compose/release.env" ]; then
  ENV_FILE="$ROOT/deploy/compose/release.env"
fi

DB_DRIVER="${WIREGATE_DB_DRIVER:-}"
if [ -z "$DB_DRIVER" ] && [ -f "$ENV_FILE" ]; then
  DB_DRIVER="$(sed -n 's/^WIREGATE_DB_DRIVER=//p' "$ENV_FILE" | tail -n 1)"
fi
if [ -z "$DB_DRIVER" ]; then
  DB_DRIVER="sqlite"
fi
if [ "$DB_DRIVER" = "postgres" ]; then
  echo "[release-backup-create] PostgreSQL mode uses an external primary database." >&2
  echo "[release-backup-create] use native PostgreSQL backup tooling; SQLite volume backup automation is not supported in postgres mode yet." >&2
  exit 1
fi

if [ -z "$PASSPHRASE" ] && [ -n "${WIREGATE_BACKUP_PASSPHRASE_FILE:-}" ] && [ -r "${WIREGATE_BACKUP_PASSPHRASE_FILE}" ]; then
  mode="$(stat -c '%a' "${WIREGATE_BACKUP_PASSPHRASE_FILE}" 2>/dev/null || true)"
  if [ -z "$mode" ] || [ $((8#$mode & 077)) -ne 0 ]; then
    echo "[release-backup-create] insecure permissions for secret file: ${WIREGATE_BACKUP_PASSPHRASE_FILE} (mode: ${mode:-unknown})" >&2
    echo "[release-backup-create] expected owner-only permissions such as 600" >&2
    exit 1
  fi
  PASSPHRASE="$(head -n 1 "${WIREGATE_BACKUP_PASSPHRASE_FILE}" | tr -d '\r\n')"
fi
export PASSPHRASE

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "[release-backup-create] missing required command: $1" >&2
    exit 1
  fi
}

assert_private_secret_file_mode() {
  local path="$1"
  local mode
  mode="$(stat -c '%a' "$path" 2>/dev/null || true)"
  if [ -z "$mode" ]; then
    echo "[release-backup-create] could not determine permissions for secret file: $path" >&2
    exit 1
  fi
  if [ $((8#$mode & 077)) -ne 0 ]; then
    echo "[release-backup-create] insecure permissions for secret file: $path (mode: $mode)" >&2
    echo "[release-backup-create] expected owner-only permissions such as 600" >&2
    exit 1
  fi
}

if [ -z "$PASSPHRASE" ]; then
  if [ -n "${WIREGATE_BACKUP_PASSPHRASE_FILE:-}" ] && [ -f "${WIREGATE_BACKUP_PASSPHRASE_FILE}" ]; then
    assert_private_secret_file_mode "${WIREGATE_BACKUP_PASSPHRASE_FILE}"
  fi
  echo "[release-backup-create] WIREGATE_BACKUP_PASSPHRASE is required" >&2
  exit 1
fi

require_cmd docker
require_cmd openssl
require_cmd tar

BACKEND_ID="$(docker ps \
  --filter "label=com.docker.compose.project=$PROJECT_NAME" \
  --filter "label=com.docker.compose.service=backend" \
  --format '{{.ID}}' \
  | head -n 1)"
if [ -z "$BACKEND_ID" ]; then
  echo "[release-backup-create] backend container not found; deploy release stack first" >&2
  exit 1
fi

mkdir -p "$BACKUP_DIR"
TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

TS="$(date -u +"%Y%m%dT%H%M%SZ")"
if [ -z "$OUTPUT_FILE" ]; then
  OUTPUT_FILE="$BACKUP_DIR/wiregate-backup-$TS.tar.gz.enc"
fi
PLAIN_ARCHIVE="$TMP_DIR/wiregate-backup-$TS.tar.gz"
STAGING="$TMP_DIR/staging"
mkdir -p "$STAGING"

docker cp "$BACKEND_ID:/var/lib/wiregate/wiregate.db" "$STAGING/wiregate.db"
cp "$ROOT/deploy/compose/docker-compose.release.yml" "$STAGING/docker-compose.release.yml"
if [ -f "$ENV_FILE" ]; then
  cp "$ENV_FILE" "$STAGING/release.env"
fi

{
  echo "created_at=$TS"
  echo "project_name=$PROJECT_NAME"
  echo "backend_container=$BACKEND_ID"
  docker ps --filter "label=com.docker.compose.project=$PROJECT_NAME"
} > "$STAGING/metadata.txt"

tar -C "$STAGING" -czf "$PLAIN_ARCHIVE" .
openssl enc -aes-256-cbc -pbkdf2 -salt -in "$PLAIN_ARCHIVE" -out "$OUTPUT_FILE" -pass "env:PASSPHRASE"

echo "[release-backup-create] backup file: $OUTPUT_FILE"
