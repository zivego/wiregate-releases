# Agent Shared Contracts

This directory holds shared agent contracts and cross-platform client/state helpers.

Current contents:
- `contracts.md` documents the enrollment and check-in protocol boundaries
- `client/` contains shared HTTP client, state persistence, key generation, and WireGuard config render helpers

MVP constraints:
- keep OS-specific runtime mutation logic outside this directory
- keep server-authoritative policy enforcement boundaries explicit
