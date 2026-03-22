#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "[release-publish] missing required command: $1" >&2
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

VERSION="${WIREGATE_VERSION:-}"
if [ -z "$VERSION" ]; then
  echo "[release-publish] WIREGATE_VERSION is required" >&2
  exit 1
fi

REGISTRY="${WIREGATE_REGISTRY-ghcr.io/zivego}"
BACKEND_REPO="${WIREGATE_BACKEND_REPO-wiregate-backend}"
FRONTEND_REPO="${WIREGATE_FRONTEND_REPO-wiregate-frontend}"
BACKEND_IMAGE="$(image_ref "$REGISTRY" "$BACKEND_REPO" "$VERSION")"
FRONTEND_IMAGE="$(image_ref "$REGISTRY" "$FRONTEND_REPO" "$VERSION")"

require_cmd docker

COMMIT_SHA="${WIREGATE_COMMIT:-$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)}"

echo "[release-publish] building backend image: $BACKEND_IMAGE"
docker build \
  --build-arg "WIREGATE_VERSION=$VERSION" \
  --build-arg "WIREGATE_COMMIT=$COMMIT_SHA" \
  -f "$ROOT/deploy/compose/backend.Dockerfile" -t "$BACKEND_IMAGE" "$ROOT"

echo "[release-publish] building frontend image: $FRONTEND_IMAGE"
docker build -f "$ROOT/deploy/compose/frontend.Dockerfile" -t "$FRONTEND_IMAGE" "$ROOT"

if is_true "${WIREGATE_PUSH:-false}"; then
  echo "[release-publish] pushing backend image"
  docker push "$BACKEND_IMAGE"
  echo "[release-publish] pushing frontend image"
  docker push "$FRONTEND_IMAGE"
else
  echo "[release-publish] WIREGATE_PUSH is false; skipping push"
fi

# Update manifest.json so deployed instances can detect the new version.
MANIFEST="$ROOT/manifest.json"
RELEASED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
CHANGELOG_URL="${WIREGATE_CHANGELOG_URL:-}"
cat > "$MANIFEST" <<EOJSON
{
  "latest_version": "$VERSION",
  "released_at": "$RELEASED_AT",
  "changelog_url": "$CHANGELOG_URL"
}
EOJSON
echo "[release-publish] updated manifest.json → $VERSION"

echo "[release-publish] backend_image=$BACKEND_IMAGE"
echo "[release-publish] frontend_image=$FRONTEND_IMAGE"
