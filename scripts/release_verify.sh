#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PROJECT_NAME="${WIREGATE_RELEASE_PROJECT_NAME:-wiregate-release}"
RELEASE_NETWORK="${WIREGATE_RELEASE_NETWORK:-${PROJECT_NAME}_default}"
BACKEND_PORT="${WIREGATE_BACKEND_PORT:-8080}"
FRONTEND_PORT="${WIREGATE_FRONTEND_PORT:-5173}"
FRONTEND_SCHEME="${WIREGATE_FRONTEND_SCHEME:-https}"
TLS_VERIFY="${WIREGATE_TLS_VERIFY:-false}"
API_BASE_URL="${WIREGATE_API_BASE_URL:-http://localhost:${BACKEND_PORT}/api/v1}"
FRONTEND_URL="${WIREGATE_FRONTEND_URL:-${FRONTEND_SCHEME}://localhost:${FRONTEND_PORT}}"
API_INTERNAL_BASE_URL="${WIREGATE_API_INTERNAL_BASE_URL:-http://backend:8080/api/v1}"
FRONTEND_INTERNAL_URL="${WIREGATE_FRONTEND_INTERNAL_URL:-${FRONTEND_SCHEME}://frontend}"
CURL_FALLBACK_IMAGE="${WIREGATE_CURL_FALLBACK_IMAGE:-curlimages/curl:8.12.1}"
VERIFY_EMAIL="${WIREGATE_VERIFY_USER_EMAIL:-${WIREGATE_BOOTSTRAP_ADMIN_EMAIL:-admin@wiregate.local}}"
VERIFY_PASSWORD="${WIREGATE_VERIFY_USER_PASSWORD:-${WIREGATE_BOOTSTRAP_ADMIN_PASSWORD:-}}"
BOOTSTRAP_EMAIL="${WIREGATE_BOOTSTRAP_ADMIN_EMAIL:-admin@wiregate.local}"
BOOTSTRAP_PASSWORD="${WIREGATE_BOOTSTRAP_ADMIN_PASSWORD:-}"
USE_DOCKER_CURL=false

COOKIE_JAR="$(mktemp)"
LOGIN_BODY="$(mktemp)"
cleanup() {
  rm -f "$COOKIE_JAR" "$LOGIN_BODY"
}
trap cleanup EXIT

is_true() {
  case "${1:-}" in
    1|true|TRUE|yes|YES|on|ON) return 0 ;;
    *) return 1 ;;
  esac
}

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

run_curl() {
  if [ "$USE_DOCKER_CURL" = "true" ]; then
    docker run --rm \
      --user "$(id -u):$(id -g)" \
      --network "$RELEASE_NETWORK" \
      -v /tmp:/tmp \
      "$CURL_FALLBACK_IMAGE" \
      "$@"
    return
  fi
  curl "$@"
}

wait_http_200() {
  local url="$1"
  local label="$2"
  shift 2
  for _ in $(seq 1 30); do
    if run_curl "$@" -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "[release-verify] $label is not ready: $url" >&2
  return 1
}

FRONTEND_CURL_OPTS=()
if [ "$FRONTEND_SCHEME" = "https" ] && ! is_true "$TLS_VERIFY"; then
  FRONTEND_CURL_OPTS+=(-k)
fi

configure_verify_transport() {
  if curl -fsS "${API_BASE_URL}/health/live" >/dev/null 2>&1; then
    return 0
  fi
  if docker run --rm --network "$RELEASE_NETWORK" "$CURL_FALLBACK_IMAGE" -fsS "${API_INTERNAL_BASE_URL}/health/live" >/dev/null 2>&1; then
    API_BASE_URL="$API_INTERNAL_BASE_URL"
    FRONTEND_URL="$FRONTEND_INTERNAL_URL"
    USE_DOCKER_CURL=true
    return 0
  fi
}

load_verify_user_secret() {
  if [ -n "${VERIFY_PASSWORD:-}" ]; then
    return 0
  fi
  local secret_file="${WIREGATE_VERIFY_USER_PASSWORD_FILE:-${WIREGATE_BOOTSTRAP_ADMIN_PASSWORD_FILE:-}}"
  case "$secret_file" in
    /*) ;;
    "") ;;
    *) secret_file="$ROOT/$secret_file" ;;
  esac
  if [ -n "$secret_file" ] && [ -r "$secret_file" ]; then
    local mode
    mode="$(stat -c '%a' "$secret_file" 2>/dev/null || true)"
    if [ -z "$mode" ] || [ $((8#$mode & 077)) -ne 0 ]; then
      echo "[release-verify] insecure permissions for verify secret file: $secret_file (mode: ${mode:-unknown})" >&2
      echo "[release-verify] expected owner-only permissions such as 600" >&2
      exit 1
    fi
    VERIFY_PASSWORD="$(head -n 1 "$secret_file" | tr -d '\r\n')"
  fi
}

load_bootstrap_admin_secret() {
  if [ -n "${BOOTSTRAP_PASSWORD:-}" ]; then
    return 0
  fi
  local secret_file="${WIREGATE_BOOTSTRAP_ADMIN_PASSWORD_FILE:-}"
  case "$secret_file" in
    /*) ;;
    "") ;;
    *) secret_file="$ROOT/$secret_file" ;;
  esac
  if [ -n "$secret_file" ] && [ -r "$secret_file" ]; then
    local mode
    mode="$(stat -c '%a' "$secret_file" 2>/dev/null || true)"
    if [ -z "$mode" ] || [ $((8#$mode & 077)) -ne 0 ]; then
      echo "[release-verify] insecure permissions for bootstrap admin password file: $secret_file (mode: ${mode:-unknown})" >&2
      echo "[release-verify] expected owner-only permissions such as 600" >&2
      exit 1
    fi
    BOOTSTRAP_PASSWORD="$(head -n 1 "$secret_file" | tr -d '\r\n')"
  fi
}

configure_verify_transport

echo "[release-verify] checking backend health endpoint"
wait_http_200 "$API_BASE_URL/health/live" "backend"

echo "[release-verify] checking frontend root endpoint"
wait_http_200 "$FRONTEND_URL" "frontend" "${FRONTEND_CURL_OPTS[@]}"

load_verify_user_secret
load_bootstrap_admin_secret
if [ -z "$VERIFY_PASSWORD" ] || [ -z "$VERIFY_EMAIL" ]; then
  echo "[release-verify] verify user credentials are not configured" >&2
  exit 1
fi

LOGIN_PAYLOAD="$(printf '{"email":"%s","password":"%s"}' "$(json_escape "$VERIFY_EMAIL")" "$(json_escape "$VERIFY_PASSWORD")")"
LOGIN_STATUS="$(run_curl -sS -o "$LOGIN_BODY" -w "%{http_code}" -c "$COOKIE_JAR" \
  -H "Content-Type: application/json" \
  -d "$LOGIN_PAYLOAD" \
  "$API_BASE_URL/sessions")"
if [ "$LOGIN_STATUS" = "401" ] && [ -n "$BOOTSTRAP_PASSWORD" ] && [ -n "$BOOTSTRAP_EMAIL" ]; then
  rm -f "$COOKIE_JAR"
  LOGIN_PAYLOAD="$(printf '{"email":"%s","password":"%s"}' "$(json_escape "$BOOTSTRAP_EMAIL")" "$(json_escape "$BOOTSTRAP_PASSWORD")")"
  LOGIN_STATUS="$(run_curl -sS -o "$LOGIN_BODY" -w "%{http_code}" -c "$COOKIE_JAR" \
    -H "Content-Type: application/json" \
    -d "$LOGIN_PAYLOAD" \
    "$API_BASE_URL/sessions")"
  if [ "$LOGIN_STATUS" = "200" ]; then
    VERIFY_EMAIL="$BOOTSTRAP_EMAIL"
  fi
fi
if [ "$LOGIN_STATUS" != "200" ]; then
  echo "[release-verify] login failed with status $LOGIN_STATUS" >&2
  cat "$LOGIN_BODY" >&2
  exit 1
fi

CURRENT_STATUS="$(run_curl -sS -o /dev/null -w "%{http_code}" -b "$COOKIE_JAR" \
  "$API_BASE_URL/sessions/current")"
if [ "$CURRENT_STATUS" != "200" ]; then
  echo "[release-verify] current session check failed with status $CURRENT_STATUS" >&2
  exit 1
fi

CURRENT_BODY="$(run_curl -sS -b "$COOKIE_JAR" "$API_BASE_URL/sessions/current")"
MUST_CHANGE=false
if printf "%s" "$CURRENT_BODY" | grep -q '"must_change_password":[[:space:]]*true'; then
  MUST_CHANGE=true
fi

USERS_STATUS="$(run_curl -sS -o "$LOGIN_BODY" -w "%{http_code}" -b "$COOKIE_JAR" \
  "$API_BASE_URL/users")"

if [ "$MUST_CHANGE" = "true" ]; then
  if [ "$USERS_STATUS" != "403" ]; then
    echo "[release-verify] expected 403 for /users while password change is required, got $USERS_STATUS" >&2
    exit 1
  fi
  if ! grep -q '"code":"password_change_required"' "$LOGIN_BODY"; then
    echo "[release-verify] expected password_change_required response while must_change_password=true" >&2
    cat "$LOGIN_BODY" >&2
    exit 1
  fi
  echo "[release-verify] password change guard is active (must_change_password=true)"
else
  if [ "$USERS_STATUS" != "200" ]; then
    echo "[release-verify] authenticated users list check failed with status $USERS_STATUS" >&2
    exit 1
  fi
fi

echo "[release-verify] release smoke checks passed"
