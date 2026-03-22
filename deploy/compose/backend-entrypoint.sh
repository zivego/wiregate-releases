#!/bin/sh
# Auto-detect docker socket GID and add wiregate user to that group.
SOCK=/var/run/docker.sock
if [ -S "$SOCK" ]; then
  SOCK_GID=$(stat -c '%g' "$SOCK")
  if [ "$SOCK_GID" != "0" ]; then
    addgroup -g "$SOCK_GID" dockersock 2>/dev/null || true
    addgroup wiregate dockersock 2>/dev/null || true
  else
    addgroup wiregate root 2>/dev/null || true
  fi
fi
exec su-exec wiregate /app/wiregate-server "$@"
