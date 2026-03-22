#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
API_BASE_URL="${WIREGATE_API_BASE_URL:-http://localhost:8080/api/v1}"
VERIFY_EMAIL="${WIREGATE_VERIFY_USER_EMAIL:-${WIREGATE_BOOTSTRAP_ADMIN_EMAIL:-admin@example.com}}"
VERIFY_PASSWORD="${WIREGATE_VERIFY_USER_PASSWORD:-${WIREGATE_BOOTSTRAP_ADMIN_PASSWORD:-}}"
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
  echo "[release-backup-verify] PostgreSQL mode uses external database backup/restore workflows." >&2
  echo "[release-backup-verify] automated SQLite backup verification is not supported in postgres mode yet." >&2
  exit 1
fi

resolve_path_from_root() {
  local path_value="$1"
  case "$path_value" in
    /*) printf "%s" "$path_value" ;;
    "") printf "" ;;
    *) printf "%s/%s" "$ROOT" "$path_value" ;;
  esac
}

if [ -z "${WIREGATE_BACKUP_PASSPHRASE:-}" ] && [ -n "${WIREGATE_BACKUP_PASSPHRASE_FILE:-}" ] && [ -r "${WIREGATE_BACKUP_PASSPHRASE_FILE}" ]; then
  mode="$(stat -c '%a' "${WIREGATE_BACKUP_PASSPHRASE_FILE}" 2>/dev/null || true)"
  if [ -z "$mode" ] || [ $((8#$mode & 077)) -ne 0 ]; then
    echo "[release-backup-verify] insecure permissions for secret file: ${WIREGATE_BACKUP_PASSPHRASE_FILE} (mode: ${mode:-unknown})" >&2
    echo "[release-backup-verify] expected owner-only permissions such as 600" >&2
    exit 1
  fi
  export WIREGATE_BACKUP_PASSPHRASE
  WIREGATE_BACKUP_PASSPHRASE="$(head -n 1 "${WIREGATE_BACKUP_PASSPHRASE_FILE}" | tr -d '\r\n')"
fi

if [ -z "${WIREGATE_BACKUP_PASSPHRASE:-}" ]; then
  echo "[release-backup-verify] WIREGATE_BACKUP_PASSPHRASE is required" >&2
  exit 1
fi

VERIFY_SECRET_FILE="$(resolve_path_from_root "${WIREGATE_VERIFY_USER_PASSWORD_FILE:-}")"
if [ -z "$VERIFY_PASSWORD" ] && [ -n "$VERIFY_SECRET_FILE" ] && [ -r "$VERIFY_SECRET_FILE" ]; then
  mode="$(stat -c '%a' "$VERIFY_SECRET_FILE" 2>/dev/null || true)"
  if [ -z "$mode" ] || [ $((8#$mode & 077)) -ne 0 ]; then
    echo "[release-backup-verify] insecure permissions for verify secret file: $VERIFY_SECRET_FILE (mode: ${mode:-unknown})" >&2
    echo "[release-backup-verify] expected owner-only permissions such as 600" >&2
    exit 1
  fi
  VERIFY_PASSWORD="$(head -n 1 "$VERIFY_SECRET_FILE" | tr -d '\r\n')"
fi
if [ -z "$VERIFY_PASSWORD" ] || [ -z "$VERIFY_EMAIL" ]; then
  echo "[release-backup-verify] verify user credentials are required" >&2
  exit 1
fi

TMP_DIR="$(mktemp -d)"
COOKIE_JAR="$TMP_DIR/cookies.txt"
trap 'rm -rf "$TMP_DIR"' EXIT

login() {
  local payload
  payload="$(printf '{"email":"%s","password":"%s"}' "$VERIFY_EMAIL" "$VERIFY_PASSWORD")"
  local status
  status="$(curl -sS -o "$TMP_DIR/login-body.json" -w "%{http_code}" -c "$COOKIE_JAR" \
    -H "Content-Type: application/json" \
    -d "$payload" \
    "$API_BASE_URL/sessions")"
  [ "$status" = "200" ]
}

wait_health() {
  for _ in $(seq 1 30); do
    if curl -fsS "$API_BASE_URL/health/live" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

if ! wait_health; then
  echo "[release-backup-verify] backend is not healthy before verification" >&2
  exit 1
fi
if ! login; then
  echo "[release-backup-verify] verify-user login failed before backup verification" >&2
  cat "$TMP_DIR/login-body.json" >&2
  exit 1
fi

VERIFY_EMAIL="backup-verify-$(date +%s)@example.com"
BACKUP_FILE="$TMP_DIR/backup.enc"

echo "[release-backup-verify] creating encrypted backup"
WIREGATE_BACKUP_OUTPUT="$BACKUP_FILE" "$ROOT/scripts/release_backup_create.sh"

CREATE_PAYLOAD="$(printf '{"email":"%s","password":"%s","role":"operator"}' "$VERIFY_EMAIL" "temp-secret")"
CREATE_STATUS="$(curl -sS -o "$TMP_DIR/create-user-body.json" -w "%{http_code}" -b "$COOKIE_JAR" \
  -H "Content-Type: application/json" \
  -d "$CREATE_PAYLOAD" \
  "$API_BASE_URL/users")"
if [ "$CREATE_STATUS" != "201" ]; then
  echo "[release-backup-verify] failed to create verification user ($CREATE_STATUS)" >&2
  cat "$TMP_DIR/create-user-body.json" >&2
  exit 1
fi

USERS_BEFORE="$(curl -fsS -b "$COOKIE_JAR" "$API_BASE_URL/users")"
if ! printf "%s" "$USERS_BEFORE" | rg -q "$VERIFY_EMAIL"; then
  echo "[release-backup-verify] verification user was not found before restore" >&2
  exit 1
fi

echo "[release-backup-verify] restoring from encrypted backup"
"$ROOT/scripts/release_backup_restore.sh" "$BACKUP_FILE"

if ! wait_health; then
  echo "[release-backup-verify] backend is not healthy after restore" >&2
  exit 1
fi
if ! login; then
  echo "[release-backup-verify] verify-user login failed after restore" >&2
  cat "$TMP_DIR/login-body.json" >&2
  exit 1
fi

USERS_AFTER="$(curl -fsS -b "$COOKIE_JAR" "$API_BASE_URL/users")"
if printf "%s" "$USERS_AFTER" | rg -q "$VERIFY_EMAIL"; then
  echo "[release-backup-verify] verification user still exists after restore; restore failed" >&2
  exit 1
fi

echo "[release-backup-verify] backup/restore verification passed"
