#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
RUN_DIR="$ROOT/.wiregate-run"
DEV_PID_FILE="$RUN_DIR/dev.pid"
BACKEND_PID_FILE="$RUN_DIR/backend.pid"
FRONTEND_PID_FILE="$RUN_DIR/frontend.pid"
STOP_REQUEST_FILE="$RUN_DIR/stop-requested"

read_pid() {
  local pid_file="$1"
  if [ -f "$pid_file" ]; then
    tr -d '[:space:]' < "$pid_file"
  fi
}

is_running() {
  local pid="$1"
  [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null
}

wait_for_exit() {
  local pid="$1"
  local label="$2"

  for _ in $(seq 1 20); do
    if ! is_running "$pid"; then
      echo "[stop] $label stopped"
      return 0
    fi
    sleep 0.2
  done

  if is_running "$pid"; then
    echo "[stop] $label did not exit after SIGTERM, sending SIGKILL"
    kill -KILL "$pid" 2>/dev/null || true
  fi
}

DEV_PID="$(read_pid "$DEV_PID_FILE")"
BACKEND_PID="$(read_pid "$BACKEND_PID_FILE")"
FRONTEND_PID="$(read_pid "$FRONTEND_PID_FILE")"

if ! is_running "$DEV_PID" && ! is_running "$BACKEND_PID" && ! is_running "$FRONTEND_PID"; then
  echo "[stop] nothing is running"
  rm -f "$DEV_PID_FILE" "$BACKEND_PID_FILE" "$FRONTEND_PID_FILE" "$STOP_REQUEST_FILE"
  exit 0
fi

mkdir -p "$RUN_DIR"
touch "$STOP_REQUEST_FILE"

if is_running "$BACKEND_PID"; then
  echo "[stop] stopping backend ($BACKEND_PID)"
  kill -TERM "$BACKEND_PID" 2>/dev/null || true
  wait_for_exit "$BACKEND_PID" "backend"
fi

if is_running "$FRONTEND_PID"; then
  echo "[stop] stopping frontend ($FRONTEND_PID)"
  kill -TERM "$FRONTEND_PID" 2>/dev/null || true
  wait_for_exit "$FRONTEND_PID" "frontend"
fi

if is_running "$DEV_PID"; then
  echo "[stop] waiting for dev supervisor ($DEV_PID)"
  for _ in $(seq 1 20); do
    if ! is_running "$DEV_PID"; then
      echo "[stop] dev supervisor stopped"
      break
    fi
    sleep 0.2
  done
fi

if is_running "$DEV_PID"; then
  echo "[stop] forcing dev supervisor stop ($DEV_PID)"
  kill -TERM "$DEV_PID" 2>/dev/null || true
  wait_for_exit "$DEV_PID" "dev supervisor"
fi

rm -f "$DEV_PID_FILE" "$BACKEND_PID_FILE" "$FRONTEND_PID_FILE" "$STOP_REQUEST_FILE"
rmdir "$RUN_DIR" 2>/dev/null || true
echo "[stop] done"
