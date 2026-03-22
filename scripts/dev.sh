#!/usr/bin/env bash
set -euo pipefail

export PATH="$PATH:/usr/local/go/bin"
NODE_BIN="$(ls -d /home/zivego/.nvm/versions/node/*/bin 2>/dev/null | tail -1)"
[ -n "$NODE_BIN" ] && export PATH="$PATH:$NODE_BIN"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
RUN_DIR="$ROOT/.wiregate-run"
BACKEND_PID_FILE="$RUN_DIR/backend.pid"
FRONTEND_PID_FILE="$RUN_DIR/frontend.pid"
DEV_PID_FILE="$RUN_DIR/dev.pid"
STOP_REQUEST_FILE="$RUN_DIR/stop-requested"
CLEANED_UP=0

# Default env
export WIREGATE_BOOTSTRAP_ADMIN_EMAIL="${WIREGATE_BOOTSTRAP_ADMIN_EMAIL:-admin@example.com}"
export WIREGATE_BOOTSTRAP_ADMIN_PASSWORD="${WIREGATE_BOOTSTRAP_ADMIN_PASSWORD:-secret}"
export WIREGATE_COOKIE_INSECURE="${WIREGATE_COOKIE_INSECURE:-true}"
export WIREGATE_WG_SERVER_ENDPOINT="${WIREGATE_WG_SERVER_ENDPOINT:-vpn.example.com:55182}"
export WIREGATE_WG_SERVER_PUBLIC_KEY="${WIREGATE_WG_SERVER_PUBLIC_KEY:-server-public-key-000000000000000000000000000001}"
export WIREGATE_WG_CLIENT_CIDR="${WIREGATE_WG_CLIENT_CIDR:-10.77.0.0/24}"

mkdir -p "$RUN_DIR"
printf '%s\n' "$$" > "$DEV_PID_FILE"

cleanup() {
  if [ "$CLEANED_UP" -eq 1 ]; then
    return
  fi
  CLEANED_UP=1

  echo ""
  echo "Shutting down..."
  if [ -n "${BACKEND_PID:-}" ]; then
    kill "$BACKEND_PID" 2>/dev/null || true
  fi
  if [ -n "${FRONTEND_PID:-}" ]; then
    kill "$FRONTEND_PID" 2>/dev/null || true
  fi
  if [ -n "${BACKEND_PID:-}" ]; then
    wait "$BACKEND_PID" 2>/dev/null || true
  fi
  if [ -n "${FRONTEND_PID:-}" ]; then
    wait "$FRONTEND_PID" 2>/dev/null || true
  fi
  rm -f "$BACKEND_PID_FILE" "$FRONTEND_PID_FILE" "$DEV_PID_FILE" "$STOP_REQUEST_FILE"
  rmdir "$RUN_DIR" 2>/dev/null || true
  echo "Done."
}
trap cleanup EXIT INT TERM

echo "==> Building backend..."
go build ./... 2>&1

echo "==> Starting backend on :8080..."
go run "$ROOT/cmd/wiregate-server" &
BACKEND_PID=$!
printf '%s\n' "$BACKEND_PID" > "$BACKEND_PID_FILE"

# Wait for backend to be ready
for i in $(seq 1 20); do
  if curl -sf http://localhost:8080/api/v1/health/live > /dev/null 2>&1; then
    echo "    Backend ready."
    break
  fi
  sleep 0.3
done

echo "==> Starting frontend on :5173..."
cd "$ROOT/web/app" && npm run dev &
FRONTEND_PID=$!
printf '%s\n' "$FRONTEND_PID" > "$FRONTEND_PID_FILE"

echo ""
echo "  Backend:  http://localhost:8080"
echo "  Frontend: http://localhost:5173"
echo "  Login:    $WIREGATE_BOOTSTRAP_ADMIN_EMAIL / $WIREGATE_BOOTSTRAP_ADMIN_PASSWORD"
echo ""
echo "  Press Ctrl+C to stop."
echo "  Or run:   make stop"
echo ""

wait_status=0
wait "$BACKEND_PID" || wait_status=$?
wait "$FRONTEND_PID" || wait_status=$?

if [ -f "$STOP_REQUEST_FILE" ]; then
  exit 0
fi

exit "$wait_status"
