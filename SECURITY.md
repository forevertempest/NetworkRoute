# Security Policy

## Reporting Vulnerabilities

If you discover a security vulnerability, please report it privately via GitHub's
[Security Advisories](../../security/advisories/new) feature. Do not open a public issue.

## Security Model

### Cryptography

- **TLS 1.3** via Go standard library (no custom crypto)
- **Ed25519** identity keys
- **SHA-256 SPKI pinning** with constant-time comparison
- No 0-RTT forwarding
- Certificate validity and expiry checked on every connection

### Key Management

| Platform | Protection |
|---|---|
| Windows | DACL: current user + SYSTEM + Administrators (set before writing secrets) |
| Linux | File mode `0600`, exclusive creation |

Keys are never overwritten silently. Private keys never appear in logs or configs.

### Relay Security

- Mutual TLS authentication required — no anonymous clients
- **SSRF protection:** blocks private, loopback, link-local, multicast, cloud metadata, documentation/benchmark ranges
- DNS rebinding defense: validates resolved IP before dialing
- Resource limits: 32 clients (max 256), 128 flows/client, bounded control frames
- Not an open proxy — only authenticated peers with pinned keys

### Client Security

- SOCKS5 listens on **loopback only** — config rejects non-loopback addresses
- No TLS interception of destination traffic
- No payload logging, no telemetry
- VPN detection suspends relay by default (conservative heuristic)
- `fail-closed` blocks eligible flows without relay (not a system firewall)

### What This Does NOT Cover

- Local admin/root access (can always read process memory)
- System-wide firewall or kill switch
- Per-user SOCKS authentication
- DPAPI/CNG key storage (planned)
- Signed WFP driver (planned)
- Production hostile-load audit

## Supported Versions

| Version | Status |
|---|---|
| 0.5.x | ✅ Current development release |

## Scope

This is a development release. No production security audit has been performed.
The relay is designed for a small number of trusted identities, not public hosting.
