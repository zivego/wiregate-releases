#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
STACK_ENV_FILE="${WIREGATE_STACK_ENV_FILE:-$ROOT/deploy/compose/release.stack.env}"
STACK_ENV_EXAMPLE="$ROOT/deploy/compose/release.stack.env.example"
LEGACY_ENV_FILE="$ROOT/deploy/compose/release.env"

usage() {
  cat <<'EOF'
Usage: ./scripts/release.sh <command> [args]

Commands:
  init            Create canonical release env file from template
  publish         Build release-tagged images (and push when WIREGATE_PUSH=true)
  deploy          Deploy current version and run verify checks
  upgrade         Mandatory backup + deploy + verify
  rollback        Deploy previous version and run verify checks
  verify          Run runtime release smoke checks
  backup-create   Create encrypted backup artifact
  backup-restore  Restore from encrypted backup artifact
  backup-verify   Verify backup/restore flow
  upgrade-test    Validate previous->target upgrade flow
  rollback-test   Validate target->previous rollback flow
  systemd-check   Validate systemd+nginx release artifacts
  gate            Run full release-ready gate
  help            Show this help
EOF
}

is_true() {
  case "${1:-}" in
    1|true|TRUE|yes|YES|on|ON) return 0 ;;
    *) return 1 ;;
  esac
}

require_var() {
  local var_name="$1"
  if [ -z "${!var_name:-}" ]; then
    echo "[release] missing required variable: $var_name" >&2
    exit 1
  fi
}

assert_private_secret_file_mode() {
  local path="$1"
  local label="$2"
  local mode
  mode="$(stat -c '%a' "$path" 2>/dev/null || true)"
  if [ -z "$mode" ]; then
    echo "[release] could not determine permissions for $label file: $path" >&2
    exit 1
  fi
  if [ $((8#$mode & 077)) -ne 0 ]; then
    echo "[release] insecure permissions for $label file: $path (mode: $mode)" >&2
    echo "[release] expected owner-only permissions such as 600" >&2
    exit 1
  fi
}

resolve_path_from_root() {
  local path_value="$1"
  if [ -z "$path_value" ]; then
    printf ""
    return 0
  fi
  case "$path_value" in
    /*) printf "%s" "$path_value" ;;
    *) printf "%s/%s" "$ROOT" "$path_value" ;;
  esac
}

append_env_if_missing() {
  local key="$1"
  local value="$2"
  if ! grep -q "^${key}=" "$STACK_ENV_FILE"; then
    echo "${key}=${value}" >> "$STACK_ENV_FILE"
  fi
}

resolve_stack_env_file() {
  if [ -f "$STACK_ENV_FILE" ]; then
    return 0
  fi
  if [ -f "$LEGACY_ENV_FILE" ]; then
    STACK_ENV_FILE="$LEGACY_ENV_FILE"
    echo "[release] using legacy env file: $STACK_ENV_FILE"
    return 0
  fi
  return 1
}

load_stack_env() {
  if ! resolve_stack_env_file; then
    echo "[release] release env file not found: $STACK_ENV_FILE" >&2
    echo "[release] run: ./scripts/release.sh init" >&2
    exit 1
  fi

  set -a
  # shellcheck disable=SC1090
  . "$STACK_ENV_FILE"
  set +a

  export WIREGATE_RELEASE_ENV_FILE="$STACK_ENV_FILE"
}

load_backup_secret() {
  if [ -n "${WIREGATE_BACKUP_PASSPHRASE:-}" ]; then
    return 0
  fi
  local secret_file
  secret_file="$(resolve_path_from_root "${WIREGATE_BACKUP_PASSPHRASE_FILE:-}")"
  if [ -n "$secret_file" ] && [ -r "$secret_file" ]; then
    assert_private_secret_file_mode "$secret_file" "backup passphrase"
    WIREGATE_BACKUP_PASSPHRASE="$(head -n 1 "$secret_file" | tr -d '\r\n')"
    export WIREGATE_BACKUP_PASSPHRASE
  fi
}

load_bootstrap_admin_secret() {
  if [ -n "${WIREGATE_BOOTSTRAP_ADMIN_PASSWORD:-}" ]; then
    return 0
  fi
  local secret_file
  secret_file="$(resolve_path_from_root "${WIREGATE_BOOTSTRAP_ADMIN_PASSWORD_FILE:-}")"
  if [ -n "$secret_file" ] && [ -r "$secret_file" ]; then
    assert_private_secret_file_mode "$secret_file" "bootstrap admin password"
    WIREGATE_BOOTSTRAP_ADMIN_PASSWORD="$(head -n 1 "$secret_file" | tr -d '\r\n')"
    export WIREGATE_BOOTSTRAP_ADMIN_PASSWORD
  fi
}

load_verify_user_secret() {
  WIREGATE_VERIFY_USER_EMAIL="${WIREGATE_VERIFY_USER_EMAIL:-${WIREGATE_BOOTSTRAP_ADMIN_EMAIL:-}}"
  export WIREGATE_VERIFY_USER_EMAIL
  if [ -n "${WIREGATE_VERIFY_USER_PASSWORD:-}" ]; then
    return 0
  fi
  local secret_file
  secret_file="${WIREGATE_VERIFY_USER_PASSWORD_FILE:-${WIREGATE_BOOTSTRAP_ADMIN_PASSWORD_FILE:-}}"
  secret_file="$(resolve_path_from_root "$secret_file")"
  if [ -n "$secret_file" ] && [ -r "$secret_file" ]; then
    assert_private_secret_file_mode "$secret_file" "verify user password"
    WIREGATE_VERIFY_USER_PASSWORD="$(head -n 1 "$secret_file" | tr -d '\r\n')"
    export WIREGATE_VERIFY_USER_PASSWORD
    export WIREGATE_VERIFY_USER_PASSWORD_FILE="$secret_file"
  fi
}

require_bootstrap_admin_secret() {
  load_bootstrap_admin_secret
  if [ -z "${WIREGATE_BOOTSTRAP_ADMIN_PASSWORD:-}" ]; then
    local secret_file
    secret_file="$(resolve_path_from_root "${WIREGATE_BOOTSTRAP_ADMIN_PASSWORD_FILE:-$ROOT/deploy/compose/.secrets/bootstrap_admin_password}")"
    echo "[release] bootstrap admin password is missing" >&2
    echo "[release] set WIREGATE_BOOTSTRAP_ADMIN_PASSWORD or WIREGATE_BOOTSTRAP_ADMIN_PASSWORD_FILE in $STACK_ENV_FILE" >&2
    echo "[release] expected file: $secret_file" >&2
    exit 1
  fi
}

require_verify_user_secret() {
  load_verify_user_secret
  if [ -z "${WIREGATE_VERIFY_USER_EMAIL:-}" ]; then
    echo "[release] verify user email is missing" >&2
    echo "[release] set WIREGATE_VERIFY_USER_EMAIL in $STACK_ENV_FILE" >&2
    exit 1
  fi
  if [ -z "${WIREGATE_VERIFY_USER_PASSWORD:-}" ]; then
    local secret_file
    secret_file="$(resolve_path_from_root "${WIREGATE_VERIFY_USER_PASSWORD_FILE:-}")"
    echo "[release] verify user password is missing" >&2
    echo "[release] set WIREGATE_VERIFY_USER_PASSWORD or WIREGATE_VERIFY_USER_PASSWORD_FILE in $STACK_ENV_FILE" >&2
    if [ -n "$secret_file" ]; then
      echo "[release] expected file: $secret_file" >&2
    fi
    exit 1
  fi
}

require_backup_secret() {
  load_backup_secret
  if [ -z "${WIREGATE_BACKUP_PASSPHRASE:-}" ]; then
    local secret_file
    secret_file="$(resolve_path_from_root "${WIREGATE_BACKUP_PASSPHRASE_FILE:-/etc/wiregate/backup.pass}")"
    echo "[release] backup passphrase is missing" >&2
    echo "[release] set WIREGATE_BACKUP_PASSPHRASE_FILE in $STACK_ENV_FILE and create file with chmod 600" >&2
    echo "[release] expected file: $secret_file" >&2
    exit 1
  fi
}

bootstrap_password_secret_file() {
  if [ -n "${WIREGATE_BOOTSTRAP_ADMIN_PASSWORD_FILE:-}" ]; then
    resolve_path_from_root "$WIREGATE_BOOTSTRAP_ADMIN_PASSWORD_FILE"
    return 0
  fi
  printf "%s" "$ROOT/deploy/compose/.secrets/bootstrap_admin_password"
}

ensure_bootstrap_admin_secret() {
  local secret_file
  secret_file="$(bootstrap_password_secret_file)"
  local secret_dir
  secret_dir="$(dirname "$secret_file")"

  mkdir -p "$secret_dir"
  chmod 700 "$secret_dir" 2>/dev/null || true

  if [ -f "$secret_file" ] && ! is_true "${WIREGATE_INIT_FORCE:-false}"; then
    chmod 600 "$secret_file" 2>/dev/null || true
    echo "[release] bootstrap admin password file already exists: $secret_file"
    return 0
  fi

  if ! command -v openssl >/dev/null 2>&1; then
    echo "[release] missing required command for password generation: openssl" >&2
    exit 1
  fi

  umask 077
  openssl rand -base64 36 | tr -d '\r\n' > "$secret_file"
  chmod 600 "$secret_file"
  echo "[release] generated bootstrap admin password file: $secret_file"
}

tls_cert_secret_file() {
  if [ -n "${WIREGATE_TLS_CERT_FILE:-}" ]; then
    resolve_path_from_root "$WIREGATE_TLS_CERT_FILE"
    return 0
  fi
  printf "%s" "$ROOT/deploy/compose/.secrets/tls/tls.crt"
}

tls_key_secret_file() {
  if [ -n "${WIREGATE_TLS_KEY_FILE:-}" ]; then
    resolve_path_from_root "$WIREGATE_TLS_KEY_FILE"
    return 0
  fi
  printf "%s" "$ROOT/deploy/compose/.secrets/tls/tls.key"
}

ensure_tls_secret() {
  local default_cert default_key cert_file key_file cert_dir key_dir
  default_cert="$ROOT/deploy/compose/.secrets/tls/tls.crt"
  default_key="$ROOT/deploy/compose/.secrets/tls/tls.key"
  cert_file="$(tls_cert_secret_file)"
  key_file="$(tls_key_secret_file)"
  cert_dir="$(dirname "$cert_file")"
  key_dir="$(dirname "$key_file")"

  export WIREGATE_TLS_CERT_FILE="$cert_file"
  export WIREGATE_TLS_KEY_FILE="$key_file"

  mkdir -p "$cert_dir" "$key_dir"
  chmod 700 "$cert_dir" 2>/dev/null || true
  chmod 700 "$key_dir" 2>/dev/null || true

  if [ -f "$cert_file" ] && [ -f "$key_file" ]; then
    chmod 600 "$cert_file" 2>/dev/null || true
    chmod 600 "$key_file" 2>/dev/null || true
    echo "[release] TLS certificate/key found: $cert_file ; $key_file"
    return 0
  fi

  if [ "$cert_file" != "$default_cert" ] || [ "$key_file" != "$default_key" ]; then
    echo "[release] custom TLS cert/key files are missing" >&2
    echo "[release] expected cert: $cert_file" >&2
    echo "[release] expected key:  $key_file" >&2
    exit 1
  fi

  if ! command -v openssl >/dev/null 2>&1; then
    echo "[release] missing required command for TLS certificate generation: openssl" >&2
    exit 1
  fi

  local subject
  subject="${WIREGATE_TLS_SELF_SIGNED_SUBJECT:-/CN=wiregate.local}"
  local days
  days="${WIREGATE_TLS_SELF_SIGNED_DAYS:-825}"

  umask 077
  openssl req -x509 -nodes -newkey rsa:2048 -sha256 \
    -keyout "$key_file" \
    -out "$cert_file" \
    -days "$days" \
    -subj "$subject" >/dev/null 2>&1
  chmod 600 "$cert_file" "$key_file"
  echo "[release] generated self-signed TLS certificate: $cert_file"
}

init_stack_env() {
  if [ ! -f "$STACK_ENV_EXAMPLE" ]; then
    echo "[release] missing template: $STACK_ENV_EXAMPLE" >&2
    exit 1
  fi

  if [ -f "$STACK_ENV_FILE" ] && ! is_true "${WIREGATE_INIT_FORCE:-false}"; then
    echo "[release] env file already exists: $STACK_ENV_FILE"
  else
    cp "$STACK_ENV_EXAMPLE" "$STACK_ENV_FILE"
    echo "[release] created env file: $STACK_ENV_FILE"
  fi

  set -a
  # shellcheck disable=SC1090
  . "$STACK_ENV_FILE"
  set +a

  if [ -z "${WIREGATE_BOOTSTRAP_ADMIN_PASSWORD_FILE:-}" ]; then
    WIREGATE_BOOTSTRAP_ADMIN_PASSWORD_FILE="$ROOT/deploy/compose/.secrets/bootstrap_admin_password"
    if ! grep -q '^WIREGATE_BOOTSTRAP_ADMIN_PASSWORD_FILE=' "$STACK_ENV_FILE"; then
      echo "" >> "$STACK_ENV_FILE"
      echo "# Generated by release init (secret file path)" >> "$STACK_ENV_FILE"
      append_env_if_missing "WIREGATE_BOOTSTRAP_ADMIN_PASSWORD_FILE" "$WIREGATE_BOOTSTRAP_ADMIN_PASSWORD_FILE"
    fi
  fi

  if [ -z "${WIREGATE_TLS_CERT_FILE:-}" ]; then
    WIREGATE_TLS_CERT_FILE="$ROOT/deploy/compose/.secrets/tls/tls.crt"
    append_env_if_missing "WIREGATE_TLS_CERT_FILE" "$WIREGATE_TLS_CERT_FILE"
  fi
  if [ -z "${WIREGATE_TLS_KEY_FILE:-}" ]; then
    WIREGATE_TLS_KEY_FILE="$ROOT/deploy/compose/.secrets/tls/tls.key"
    append_env_if_missing "WIREGATE_TLS_KEY_FILE" "$WIREGATE_TLS_KEY_FILE"
  fi
  if [ -z "${WIREGATE_FRONTEND_SCHEME:-}" ]; then
    WIREGATE_FRONTEND_SCHEME="https"
    append_env_if_missing "WIREGATE_FRONTEND_SCHEME" "$WIREGATE_FRONTEND_SCHEME"
  fi
  if [ -z "${WIREGATE_VERIFY_USER_EMAIL:-}" ]; then
    WIREGATE_VERIFY_USER_EMAIL="${WIREGATE_BOOTSTRAP_ADMIN_EMAIL:-codex.api@wiregate.local}"
    append_env_if_missing "WIREGATE_VERIFY_USER_EMAIL" "$WIREGATE_VERIFY_USER_EMAIL"
  fi
  if [ -z "${WIREGATE_VERIFY_USER_PASSWORD_FILE:-}" ]; then
    WIREGATE_VERIFY_USER_PASSWORD_FILE="$ROOT/deploy/compose/.secrets/codex_api_password"
    append_env_if_missing "WIREGATE_VERIFY_USER_PASSWORD_FILE" "$WIREGATE_VERIFY_USER_PASSWORD_FILE"
  fi

  ensure_bootstrap_admin_secret
  load_bootstrap_admin_secret
  ensure_tls_secret

  local secret_file
  secret_file="$(resolve_path_from_root "${WIREGATE_BACKUP_PASSPHRASE_FILE:-/etc/wiregate/backup.pass}")"
  if [ ! -f "$secret_file" ]; then
    echo "[release] backup secret file not found: $secret_file"
    echo "[release] create it (example):"
    echo "  sudo install -m 600 /dev/null \"$secret_file\""
    echo "  sudo sh -c 'openssl rand -base64 32 > \"$secret_file\"'"
  fi

  if [ -n "${WIREGATE_BOOTSTRAP_ADMIN_EMAIL:-}" ]; then
    echo "[release] bootstrap admin email: $WIREGATE_BOOTSTRAP_ADMIN_EMAIL"
  fi
  echo "[release] bootstrap admin password source: ${WIREGATE_BOOTSTRAP_ADMIN_PASSWORD_FILE:-env var}"
  echo "[release] verify user email: ${WIREGATE_VERIFY_USER_EMAIL:-<unset>}"
  echo "[release] verify user password source: ${WIREGATE_VERIFY_USER_PASSWORD_FILE:-env var}"
  echo "[release] TLS certificate source: $WIREGATE_TLS_CERT_FILE"
  echo "[release] frontend URL scheme: ${WIREGATE_FRONTEND_SCHEME:-https}"
  echo "[release] next step: ./scripts/release.sh deploy"
}

run_gate() {
  load_stack_env
  require_backup_secret
  require_verify_user_secret
  require_bootstrap_admin_secret

  require_var WIREGATE_VERSION
  require_var WIREGATE_PREVIOUS_VERSION

  make test
  make lint
  make smoke

  "$ROOT/scripts/release_systemd_check.sh"
  "$ROOT/scripts/release_verify.sh"
  "$ROOT/scripts/release_backup_verify.sh"
  "$ROOT/scripts/release_upgrade_test.sh"
  "$ROOT/scripts/release_rollback_test.sh"
}

COMMAND="${1:-help}"
shift || true

case "$COMMAND" in
  init)
    init_stack_env
    ;;
  publish)
    load_stack_env
    load_bootstrap_admin_secret
    "$ROOT/scripts/release_publish.sh" "$@"
    ;;
  deploy)
    load_stack_env
    require_bootstrap_admin_secret
    require_verify_user_secret
    ensure_tls_secret
    "$ROOT/scripts/release_deploy.sh" "$@"
    "$ROOT/scripts/release_verify.sh"
    ;;
  upgrade)
    load_stack_env
    require_backup_secret
    require_bootstrap_admin_secret
    require_verify_user_secret
    "$ROOT/scripts/release_upgrade.sh" "$@"
    ;;
  rollback)
    load_stack_env
    require_bootstrap_admin_secret
    require_verify_user_secret
    "$ROOT/scripts/release_rollback.sh" "$@"
    "$ROOT/scripts/release_verify.sh"
    ;;
  verify)
    load_stack_env
    require_verify_user_secret
    "$ROOT/scripts/release_verify.sh"
    ;;
  backup-create)
    load_stack_env
    require_backup_secret
    "$ROOT/scripts/release_backup_create.sh" "$@"
    ;;
  backup-restore)
    load_stack_env
    require_backup_secret
    "$ROOT/scripts/release_backup_restore.sh" "$@"
    ;;
  backup-verify)
    load_stack_env
    require_backup_secret
    require_verify_user_secret
    "$ROOT/scripts/release_backup_verify.sh"
    ;;
  upgrade-test)
    load_stack_env
    require_backup_secret
    require_verify_user_secret
    "$ROOT/scripts/release_upgrade_test.sh"
    ;;
  rollback-test)
    load_stack_env
    require_verify_user_secret
    "$ROOT/scripts/release_rollback_test.sh"
    ;;
  systemd-check)
    "$ROOT/scripts/release_systemd_check.sh"
    ;;
  gate)
    run_gate
    ;;
  help|-h|--help)
    usage
    ;;
  *)
    echo "[release] unknown command: $COMMAND" >&2
    usage >&2
    exit 1
    ;;
esac
