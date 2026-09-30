# Validation Record

## v0.5.0 — Console + Autostart + Local Controls

- Persistent interactive console UI with embedded offline manuals
- Settings in LOCALAPPDATA or custom data directory
- Tests: menu navigation, config save/restore, SOCKS start/stop with connected client
- `tools/test-release.ps1`: isolated EXE test with fresh LOCALAPPDATA
- Full `go test`, `go vet`, cross-builds (4 targets) passed

## v0.3.0 — VPN/VPS Setup

- Onboarding: key exchange, idempotent reuse, QUIC handshake via `doctor`
- WireGuard: split/full profiles, client identity matching
- `tools/test-setup.ps1`: standalone setup, relay pairing, VPN generation
- Bash syntax checks for Linux installers

## v0.2.0 — Standalone Smart

- Standalone config without relay/keys
- IPv4/IPv6 interleaving, staggered racing, cancellation
- Eight-attempt bound, direct-only measurement

## Test Environment

- Windows x64, Go 1.27.1 (project minimum: Go 1.26)
- `govulncheck v1.8.0`: zero reachable vulnerabilities

## Not Validated

- Linux runtime (no WSL available)
- Local race detector (no CGO compiler)
- Windows ARM64 / Windows 10 runtime
- Real WAN improvement
- WFP/Service/GUI (planned, not implemented)
