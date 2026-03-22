# Windows Agent MVP Foundation

Current slice provides:
- a native Go client library for Windows enrollment and check-in
- shared local state persistence with restrictive file permissions
- local private key generation/loading
- WireGuard config rendering and staged config write
- a CLI entrypoint at `cmd/wiregate-agent-windows`
- periodic daemon mode for repeated authenticated check-ins

Current commands:
- `go run ./cmd/wiregate-agent-windows enroll --server http://localhost:8080 --token <token> --apply=true`
- `go run ./cmd/wiregate-agent-windows check-in --apply=true`
- `go run ./cmd/wiregate-agent-windows daemon --interval 30s --failure-backoff 5s --max-failure-backoff 1m --apply=true`
- `go run ./cmd/wiregate-agent-windows install-service --binary-path "C:\Program Files\Wiregate\wiregate-agent-windows.exe"`
- `go run ./cmd/wiregate-agent-windows uninstall-service`

Current limitations:
- local apply uses `wireguard.exe /installtunnelservice` through a bounded adapter
- runtime inspection currently validates tunnel service presence and single-peer WireGuard state

Operator notes:
- install flow, daemon commands, and basic recovery steps are documented in `docs/implementation/WINDOWS_AGENT_RUNBOOK.md`
