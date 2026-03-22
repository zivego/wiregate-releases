#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE_FILE="$ROOT/deploy/compose/docker-compose.release.yml"
ENV_FILE="${WIREGATE_RELEASE_ENV_FILE:-$ROOT/deploy/compose/release.stack.env}"
if [ ! -f "$ENV_FILE" ] && [ -f "$ROOT/deploy/compose/release.env" ]; then
  ENV_FILE="$ROOT/deploy/compose/release.env"
fi
PROJECT_NAME="${WIREGATE_RELEASE_PROJECT_NAME:-wiregate-release}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "[release-deploy] missing required command: $1" >&2
    exit 1
  fi
}

image_ref() {
  local registry="$1"
  local repo="$2"
  local tag="$3"
  if [ -n "$registry" ]; then
    printf "%s/%s:%s" "${registry%/}" "$repo" "$tag"
  else
    printf "%s:%s" "$repo" "$tag"
  fi
}

is_true() {
  case "${1:-}" in
    1|true|TRUE|yes|YES|on|ON) return 0 ;;
    *) return 1 ;;
  esac
}

resolve_path_from_root() {
  local path_value="$1"
  case "$path_value" in
    /*) printf "%s" "$path_value" ;;
    *) printf "%s/%s" "$ROOT" "$path_value" ;;
  esac
}

load_bootstrap_admin_secret() {
  if [ -n "${WIREGATE_BOOTSTRAP_ADMIN_PASSWORD:-}" ]; then
    return 0
  fi
  local secret_file="${WIREGATE_BOOTSTRAP_ADMIN_PASSWORD_FILE:-}"
  case "$secret_file" in
    /*) ;;
    "") ;;
    *) secret_file="$ROOT/$secret_file" ;;
  esac
  if [ -n "$secret_file" ] && [ -r "$secret_file" ]; then
    WIREGATE_BOOTSTRAP_ADMIN_PASSWORD="$(head -n 1 "$secret_file" | tr -d '\r\n')"
    export WIREGATE_BOOTSTRAP_ADMIN_PASSWORD
  fi
}

load_tls_paths() {
  WIREGATE_TLS_CERT_FILE="${WIREGATE_TLS_CERT_FILE:-$ROOT/deploy/compose/.secrets/tls/tls.crt}"
  WIREGATE_TLS_KEY_FILE="${WIREGATE_TLS_KEY_FILE:-$ROOT/deploy/compose/.secrets/tls/tls.key}"
  WIREGATE_TLS_CERT_FILE="$(resolve_path_from_root "$WIREGATE_TLS_CERT_FILE")"
  WIREGATE_TLS_KEY_FILE="$(resolve_path_from_root "$WIREGATE_TLS_KEY_FILE")"
  export WIREGATE_TLS_CERT_FILE
  export WIREGATE_TLS_KEY_FILE
}

VERSION="${WIREGATE_VERSION:-}"
if [ -z "$VERSION" ]; then
 echo "[release-deploy] WIREGATE_VERSION is required" >&2
  exit 1
fi

REGISTRY="${WIREGATE_REGISTRY-ghcr.io/zivego}"
BACKEND_REPO="${WIREGATE_BACKEND_REPO-wiregate-backend}"
FRONTEND_REPO="${WIREGATE_FRONTEND_REPO-wiregate-frontend}"
export WIREGATE_BACKEND_IMAGE
WIREGATE_BACKEND_IMAGE="$(image_ref "$REGISTRY" "$BACKEND_REPO" "$VERSION")"
export WIREGATE_FRONTEND_IMAGE
WIREGATE_FRONTEND_IMAGE="$(image_ref "$REGISTRY" "$FRONTEND_REPO" "$VERSION")"

require_cmd docker
load_bootstrap_admin_secret
load_tls_paths
if [ -z "${WIREGATE_BOOTSTRAP_ADMIN_PASSWORD:-}" ]; then
  echo "[release-deploy] bootstrap admin password is required (WIREGATE_BOOTSTRAP_ADMIN_PASSWORD or WIREGATE_BOOTSTRAP_ADMIN_PASSWORD_FILE)" >&2
  exit 1
fi
if [ ! -r "$WIREGATE_TLS_CERT_FILE" ] || [ ! -r "$WIREGATE_TLS_KEY_FILE" ]; then
  echo "[release-deploy] TLS files are required and must be readable" >&2
  echo "[release-deploy] cert: $WIREGATE_TLS_CERT_FILE" >&2
  echo "[release-deploy] key:  $WIREGATE_TLS_KEY_FILE" >&2
  echo "[release-deploy] run: ./scripts/release.sh init" >&2
  exit 1
fi

COMPOSE_ARGS=()
if [ -f "$ENV_FILE" ]; then
  COMPOSE_ARGS+=(--env-file "$ENV_FILE")
else
  echo "[release-deploy] env file not found ($ENV_FILE); using current shell env only"
fi

echo "[release-deploy] deploying version $VERSION"
echo "[release-deploy] backend image: $WIREGATE_BACKEND_IMAGE"
echo "[release-deploy] frontend image: $WIREGATE_FRONTEND_IMAGE"

if ! is_true "${WIREGATE_SKIP_PULL:-false}"; then
  docker compose -p "$PROJECT_NAME" "${COMPOSE_ARGS[@]}" -f "$COMPOSE_FILE" pull || true
fi
docker compose -p "$PROJECT_NAME" "${COMPOSE_ARGS[@]}" -f "$COMPOSE_FILE" up -d
docker compose -p "$PROJECT_NAME" "${COMPOSE_ARGS[@]}" -f "$COMPOSE_FILE" ps
