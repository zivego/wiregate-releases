# Agent Contract Notes

1. Enrollment command includes API endpoint and enrollment token.
2. Agent sends enrollment request and platform identity metadata.
3. Server validates token scope/expiry/revocation and returns:
   - enrolled agent inventory
   - peer reference
   - one-time `agent_auth_token` for subsequent check-ins
4. Model B pre-authorized single-use short-TTL tokens are default MVP path.
5. Agent check-in uses the one-time `agent_auth_token`, includes the last local apply/drift report, and receives current desired peer state plus `reconfigure_required`.
6. When server bootstrap metadata is configured, enrollment/check-in can also return `wireguard_config`:
   - interface address
   - server endpoint
   - server public key
   - rendered `AllowedIPs`
7. Linux MVP apply path uses a bounded local adapter that runs `wg-quick` directly without shell interpolation.
8. Agent runtime report currently includes:
   - reported config fingerprint derived from server-authoritative public config fields
   - last apply status (`applied`, `staged`, `apply_failed`, `drifted`)
   - last apply error
   - last applied timestamp
9. Linux agent drift detection now combines:
   - local rendered config hash verification
   - bounded runtime inspection of the target WireGuard interface
10. Windows MVP foundation reuses the shared enrollment/check-in contract and can apply config through a bounded `wireguard.exe` adapter or remain in staged-only mode.
11. Windows runtime inspection now validates the WireGuard tunnel service plus single-peer runtime data before reporting `applied` during check-in.
