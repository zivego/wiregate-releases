#!/bin/sh
# ---------------------------------------------------------------------------
# wg-entrypoint.sh — Integrated WireGuard + WireGate backend entrypoint.
#
# On first run: generates WireGuard server keys automatically.
# Then: creates wg0 interface, enables IP forwarding, starts backend.
#
# If WireGuard setup fails (missing NET_ADMIN, /dev/net/tun, etc.),
# the backend starts with the fake adapter instead of crashing.
#
# The backend uses the kernel WG adapter to manage peers directly.
# ---------------------------------------------------------------------------
set -e

WG_STATE_DIR="/etc/wireguard"
WG_INTERFACE="${WIREGATE_WG_INTERFACE:-wg0}"
WG_ADDRESS="${WIREGATE_WG_SERVER_ADDRESS:-10.77.0.254/24}"
WG_PORT="${WIREGATE_WG_LISTEN_PORT:-55182}"
WG_OK=true

# --- Generate keys on first run -------------------------------------------
if [ ! -f "$WG_STATE_DIR/private.key" ]; then
  mkdir -p "$WG_STATE_DIR"
  umask 077
  wg genkey > "$WG_STATE_DIR/private.key"
  wg pubkey < "$WG_STATE_DIR/private.key" > "$WG_STATE_DIR/public.key"
  umask 022
  echo "[wiregate] generated WireGuard server keys"
fi

# --- Create WireGuard interface -------------------------------------------
if ! ip link add "$WG_INTERFACE" type wireguard 2>/dev/null; then
  if ! ip link show "$WG_INTERFACE" >/dev/null 2>&1; then
    echo "[wiregate] WARNING: cannot create WireGuard interface"
    echo "[wiregate] ensure --cap-add NET_ADMIN and --device /dev/net/tun"
    echo "[wiregate] starting backend with fake WG adapter"
    WG_OK=false
  fi
fi

if [ "$WG_OK" = true ]; then
  wg set "$WG_INTERFACE" private-key "$WG_STATE_DIR/private.key" listen-port "$WG_PORT" || WG_OK=false
  ip addr add "$WG_ADDRESS" dev "$WG_INTERFACE" 2>/dev/null || true
  ip link set "$WG_INTERFACE" up 2>/dev/null || WG_OK=false
fi

if [ "$WG_OK" = true ]; then
  # Enable IP forwarding + NAT (non-fatal if read-only /proc).
  sysctl -w net.ipv4.ip_forward=1 >/dev/null 2>&1 || \
    echo "[wiregate] WARNING: could not enable ip_forward (read-only /proc?)"
  iptables -t nat -C POSTROUTING -o eth0 -j MASQUERADE 2>/dev/null \
    || iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE 2>/dev/null \
    || echo "[wiregate] WARNING: could not set up NAT"
fi

# --- Auto-detect endpoint if not set --------------------------------------
if [ -z "$WIREGATE_WG_SERVER_ENDPOINT" ]; then
  DETECTED_IP=""
  # Try public IP first (works when host has internet access).
  # Use /ip path to get plain-text response (wget doesn't send curl User-Agent).
  DETECTED_IP="$(wget -qO- https://ifconfig.me/ip 2>/dev/null || true)"
  # Fallback: container's default gateway host IP.
  if [ -z "$DETECTED_IP" ]; then
    DETECTED_IP="$(ip route | awk '/default/{print $3}' 2>/dev/null || true)"
  fi
  # Fallback: first non-loopback IP.
  if [ -z "$DETECTED_IP" ]; then
    DETECTED_IP="$(hostname -i 2>/dev/null | awk '{print $1}' || true)"
  fi
  if [ -n "$DETECTED_IP" ]; then
    export WIREGATE_WG_SERVER_ENDPOINT="${DETECTED_IP}:${WG_PORT}"
    echo "[wiregate] auto-detected endpoint: $WIREGATE_WG_SERVER_ENDPOINT"
    echo "[wiregate] override with WIREGATE_WG_SERVER_ENDPOINT if this is wrong"
  else
    echo "[wiregate] WARNING: could not detect public IP"
    echo "[wiregate] set WIREGATE_WG_SERVER_ENDPOINT=<your-ip>:${WG_PORT}"
  fi
fi

# --- Export config for the backend ----------------------------------------
export WIREGATE_WG_SERVER_PUBLIC_KEY="$(cat "$WG_STATE_DIR/public.key")"

if [ "$WG_OK" = true ]; then
  export WIREGATE_WG_ADAPTER=kernel
  export WIREGATE_WG_INTERFACE="$WG_INTERFACE"
  PUB_KEY="$(cat "$WG_STATE_DIR/public.key")"
  echo "[wiregate] WireGuard $WG_INTERFACE is up (${WG_ADDRESS}, port ${WG_PORT})"
  echo "[wiregate] server public key: $PUB_KEY"
  echo "[wiregate] endpoint: $WIREGATE_WG_SERVER_ENDPOINT"
else
  echo "[wiregate] running without WireGuard (fake adapter)"
fi
echo ""

# --- Start backend --------------------------------------------------------
exec wiregate-server
