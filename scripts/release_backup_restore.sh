#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PROJECT_NAME="${WIREGATE_RELEASE_PROJECT_NAME:-wiregate-release}"
PASSPHRASE="${WIREGATE_BACKUP_PASSPHRASE:-}"
BACKUP_FILE="${1:-${WIREGATE_BACKUP_FILE:-}}"
RESTORE_ENV_FILE="${WIREGATE_RESTORE_ENV_FILE:-false}"
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
  echo "[release-backup-restore] PostgreSQL mode uses an external primary database." >&2
  echo "[release-backup-restore] restore the database with native PostgreSQL tooling before starting the stack; SQLite volume restore automation is not supported in postgres mode yet." >&2
  exit 1
fi

if [ -z "$PASSPHRASE" ] && [ -n "${WIREGATE_BACKUP_PASSPHRASE_FILE:-}" ] && [ -r "${WIREGATE_BACKUP_PASSPHRASE_FILE}" ]; then
  mode="$(stat -c '%a' "${WIREGATE_BACKUP_PASSPHRASE_FILE}" 2>/dev/null || true)"
  if [ -z "$mode" ] || [ $((8#$mode & 077)) -ne 0 ]; then
    echo "[release-backup-restore] insecure permissions for secret file: ${WIREGATE_BACKUP_PASSPHRASE_FILE} (mode: ${mode:-unknown})" >&2
    echo "[release-backup-restore] expected owner-only permissions such as 600" >&2
    exit 1
  fi
  PASSPHRASE="$(head -n 1 "${WIREGATE_BACKUP_PASSPHRASE_FILE}" | tr -d '\r\n')"
fi
export PASSPHRASE

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "[release-backup-restore] missing required command: $1" >&2
    exit 1
  fi
}

assert_private_secret_file_mode() {
  local path="$1"
  local mode
  mode="$(stat -c '%a' "$path" 2>/dev/null || true)"
  if [ -z "$mode" ]; then
    echo "[release-backup-restore] could not determine permissions for secret file: $path" >&2
    exit 1
  fi
  if [ $((8#$mode & 077)) -ne 0 ]; then
    echo "[release-backup-restore] insecure permissions for secret file: $path (mode: $mode)" >&2
    echo "[release-backup-restore] expected owner-only permissions such as 600" >&2
    exit 1
  fi
}

if [ -z "$PASSPHRASE" ]; then
  if [ -n "${WIREGATE_BACKUP_PASSPHRASE_FILE:-}" ] && [ -f "${WIREGATE_BACKUP_PASSPHRASE_FILE}" ]; then
    assert_private_secret_file_mode "${WIREGATE_BACKUP_PASSPHRASE_FILE}"
  fi
  echo "[release-backup-restore] WIREGATE_BACKUP_PASSPHRASE is required" >&2
  exit 1
fi
if [ -z "$BACKUP_FILE" ]; then
  echo "[release-backup-restore] backup file argument (or WIREGATE_BACKUP_FILE) is required" >&2
  exit 1
fi
if [ ! -f "$BACKUP_FILE" ]; then
  echo "[release-backup-restore] backup file not found: $BACKUP_FILE" >&2
  exit 1
fi

require_cmd docker
require_cmd openssl
require_cmd tar

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

DECRYPTED_ARCHIVE="$TMP_DIR/backup.tar.gz"
EXTRACT_DIR="$TMP_DIR/extracted"
mkdir -p "$EXTRACT_DIR"

openssl enc -d -aes-256-cbc -pbkdf2 -in "$BACKUP_FILE" -out "$DECRYPTED_ARCHIVE" -pass "env:PASSPHRASE"
tar -C "$EXTRACT_DIR" -xzf "$DECRYPTED_ARCHIVE"

if [ ! -f "$EXTRACT_DIR/wiregate.db" ]; then
  echo "[release-backup-restore] backup archive does not contain wiregate.db" >&2
  exit 1
fi

BACKEND_CONTAINER="$(docker ps -a \
  --filter "label=com.docker.compose.project=$PROJECT_NAME" \
  --filter "label=com.docker.compose.service=backend" \
  --format '{{.ID}}' \
  | head -n 1)"
FRONTEND_CONTAINER="$(docker ps -a \
  --filter "label=com.docker.compose.project=$PROJECT_NAME" \
  --filter "label=com.docker.compose.service=frontend" \
  --format '{{.ID}}' \
  | head -n 1)"
if [ -z "$BACKEND_CONTAINER" ]; then
  echo "[release-backup-restore] backend container not found for project $PROJECT_NAME" >&2
  exit 1
fi

VOLUME_NAME="${PROJECT_NAME}_wiregate-data"

echo "[release-backup-restore] stopping stack before restore"
if [ -n "$FRONTEND_CONTAINER" ]; then
  docker stop "$FRONTEND_CONTAINER" >/dev/null || true
fi
docker stop "$BACKEND_CONTAINER" >/dev/null || true
docker volume create "$VOLUME_NAME" >/dev/null

docker run --rm \
  -v "$VOLUME_NAME:/target" \
  -v "$EXTRACT_DIR:/backup" \
  alpine:3.20 \
  sh -c "cp /backup/wiregate.db /target/wiregate.db && chown 1000:1000 /target/wiregate.db"

if [ "$RESTORE_ENV_FILE" = "true" ] && [ -f "$EXTRACT_DIR/release.env" ]; then
  if [ -f "$ENV_FILE" ] || [ ! -e "$ENV_FILE" ]; then
    cp "$EXTRACT_DIR/release.env" "$ENV_FILE"
    echo "[release-backup-restore] restored env file to $ENV_FILE"
  fi
fi

echo "[release-backup-restore] starting stack after restore"
docker start "$BACKEND_CONTAINER" >/dev/null
if [ -n "$FRONTEND_CONTAINER" ]; then
  docker start "$FRONTEND_CONTAINER" >/dev/null
fi
echo "[release-backup-restore] restore completed from $BACKUP_FILE"
