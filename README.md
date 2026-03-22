# wiregate

Self-hosted WireGuard peer management UI.

## MVP Status

MVP core scope is implemented for:
- backend control plane (Go + REST + OpenAPI)
- admin UI (React + TypeScript)
- SQLite persistence and migrations
- enrollment token lifecycle (Model A + Model B)
- policy rendering and peer reconciliation
- Linux and Windows native agents (enroll, check-in, daemon, apply, service flow)
- lifecycle actions (disable/enable/revoke/reissue)
- agent-driven key rotation
- audit events and operational UI pages

Phase 5 (release hardening) assets are implemented for:
- registry-tagged Docker Compose release deployment
- Ubuntu non-container deployment (`systemd + nginx`) runbook
- encrypted backup/restore procedure (OpenSSL AES-256)
- upgrade/rollback procedures and validation scripts

## Commands

- `make setup`
- `make dev`
- `make stop`
- `make test`
- `make lint`
- `make smoke`
- `make lab-up`
- `make lab-down`
- `make lab-status`
- `make release-init`
- `make release`
- `make release-gate`
- `make release-publish`
- `make release-deploy`
- `make release-verify`
- `make release-backup-create`
- `make release-backup-verify`
- `make release-upgrade`
- `make release-rollback`

## Local Run

Start the project:
- `make dev`

Stop the project cleanly from any terminal:
- `make stop`

`make dev` starts:
- backend on `http://localhost:8080`
- frontend on `http://localhost:5173`

You can still stop an interactive `make dev` session with `Ctrl+C`, but `make stop` is the preferred automated shutdown path.

## Release-Ready Gate

Use this minimum gate before a release candidate:
1. `make test`
2. `make lint`
3. `make smoke`
4. `make release-verify`
5. `make release-backup-verify`
6. `make release-upgrade-test`
7. `make release-rollback-test`

Operational runbooks:
- `docs/implementation/COMPOSE_RELEASE_RUNBOOK.md`
- `docs/implementation/UBUNTU_SYSTEMD_NGINX_RUNBOOK.md`
- `docs/implementation/BACKUP_RESTORE_RUNBOOK.md`
- `docs/implementation/UPGRADE_ROLLBACK_RUNBOOK.md`

## Simplified Release UX

Canonical release config file:
- `deploy/compose/release.stack.env`

Quick start (minimal operator actions):
1. `make release-init`
2. `make release` (deploy + verify)

`release-init` now:
- creates `deploy/compose/release.stack.env` if missing
- generates bootstrap admin password file at `deploy/compose/.secrets/bootstrap_admin_password` (`chmod 600`)
- generates default self-signed TLS cert/key at `deploy/compose/.secrets/tls/` (`chmod 600`)
- keeps compatibility with explicit `WIREGATE_BOOTSTRAP_ADMIN_PASSWORD` when set

Before production, edit `deploy/compose/release.stack.env` with real values:
- `WIREGATE_VERSION`, `WIREGATE_PREVIOUS_VERSION`, registry repos
- `WIREGATE_WG_SERVER_ENDPOINT`, `WIREGATE_WG_SERVER_PUBLIC_KEY`
- `WIREGATE_API_MAX_BODY_BYTES`, `WIREGATE_API_MAX_JSON_BYTES` (request size limits)
- `WIREGATE_VERIFY_USER_EMAIL`, `WIREGATE_VERIFY_USER_PASSWORD_FILE` (dedicated admin for release verify/gate)
- `WIREGATE_BACKUP_PASSPHRASE_FILE` (must point to existing `chmod 600` file)
- `WIREGATE_TLS_CERT_FILE`, `WIREGATE_TLS_KEY_FILE` (optional custom cert instead of default self-signed)
- optional Phase 2+ SSO values (`WIREGATE_OIDC_*`) if you want OIDC login in addition to local fallback admin

Frontend is HTTPS by default in release compose:
- open UI at `https://<host>:5173`
- for default self-signed cert, browser warning is expected until trusted/imported

Security behavior:
- Bootstrap admin is created with `must_change_password=true`.
- First login is restricted to `Account` page until password is changed.
- Web UI uses cookie-based auth (`HttpOnly`, `SameSite=Strict`) and does not persist bearer tokens in browser storage.
- OIDC SSO is optional and additive (local password login remains available as break-glass path).

## Security Baseline

Release-ready baseline (MVP):
1. HTTPS enabled for frontend ingress (compose and systemd+nginx paths).
2. Nginx security headers enabled: CSP, HSTS (HTTPS), frame-deny, nosniff, referrer-policy, permissions-policy.
3. API request limits enabled:
   - `WIREGATE_API_MAX_BODY_BYTES` default `1048576`
   - `WIREGATE_API_MAX_JSON_BYTES` default `65536`
4. Secret files are owner-only (`chmod 600`) for bootstrap and backup passphrases.
5. Backups are encrypted with OpenSSL AES-256; production hosts must use encrypted disk/volume for persisted data.

Primary one-command interface:
- `./scripts/release.sh deploy`
- `./scripts/release.sh upgrade`
- `./scripts/release.sh rollback`
- `./scripts/release.sh verify`

## Lab Shortcuts

For day-to-day lab resume/pause with preserved data:
- `make lab-up` starts release containers and optional lab containers if present (`wg-real-server`, `wg-test-agent-1`, `wg-test-agent-2`)
- `make lab-down` stops all of the above safely (no volume/data deletion)
- `make lab-status` prints running/saved container state
