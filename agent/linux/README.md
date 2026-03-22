# Linux Agent MVP Foundation

Current slice provides:
- a minimal Go client library for enrollment and check-in
- local state persistence with restrictive file permissions
- a CLI entrypoint at `cmd/wiregate-agent-linux`
- local private key generation/loading
- WireGuard config rendering and secure file write
- bounded local config apply via `wg-quick up/down`
- periodic daemon mode for repeated authenticated check-ins
- local apply/drift status tracking persisted in agent state and reported during check-in

Current commands:
- `go run ./cmd/wiregate-agent-linux enroll --server http://localhost:8080 --token <token> --apply=true`
- `go run ./cmd/wiregate-agent-linux check-in --apply=true`
- `go run ./cmd/wiregate-agent-linux daemon --interval 30s --failure-backoff 5s --max-failure-backoff 1m --apply=true`
- `go run ./cmd/wiregate-agent-linux install-service --binary-path /usr/local/bin/wiregate-agent-linux --failure-backoff 5s --max-failure-backoff 1m`
- `go run ./cmd/wiregate-agent-linux uninstall-service`

Current limitations:
- local apply uses `wg-quick` directly rather than a native WireGuard API path
- systemd install flow assumes root and `systemctl` availability on the host
- runtime inspection currently checks one WireGuard interface and single-peer MVP state
- failed daemon syncs use deterministic exponential backoff without jitter
