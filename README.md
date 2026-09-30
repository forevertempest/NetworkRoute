# NetworkRoute

> Measure · Compare · Route · Monitor

Network route optimizer for Windows and Linux. Free, no subscriptions, no accounts, no external servers required.

## What It Does

| Mode | Description |
|---|---|
| **Standalone** | Races available TCP endpoints (IPv4/IPv6) simultaneously, picks the fastest connection |
| **Smart + Relay** | Compares direct path vs your QUIC relay, switches only when measurably better |
| **WireGuard VPN** | Generates configs for your own WireGuard server |

## What It Does NOT Do

- ❌ Magically speed up internet — if there's only one path, that's it
- ❌ Intercept all traffic — only apps with SOCKS5 configured
- ❌ Provide a free VPN — relay = your own server
- ❌ Guarantee lower ping without an alternative route

## Quick Start

**Option A: Download release** — grab `NetworkRoute.exe` from [Releases](../../releases), double-click.

**Option B: Build from source:**

```powershell
# Requires Go 1.26+
powershell -NoProfile -ExecutionPolicy Bypass -File tools/build.ps1 -Test
.\bin\networkroute-windows-amd64.exe menu
```

**Verify it works:**

```powershell
curl.exe --proxy socks5h://127.0.0.1:1080 https://example.com/
```

## Project Structure

```
cmd/
├── networkroute/     CLI + interactive menu (Windows/Linux)
└── nr-relay/         Linux relay server

internal/
├── config/           Strict JSON config with validation
├── identity/         Ed25519 keys, TLS 1.3, SPKI pinning
├── protocol/         Versioned wire protocol + UDP fragmentation
├── tunnel/           QUIC client/server (TCP streams + UDP DATAGRAM)
├── proxy/            SOCKS5 data plane (TCP CONNECT + UDP ASSOCIATE)
├── quality/          Route measurement, scoring, hysteresis engine
├── policy/           Bypass rules, SSRF protection, DNS label matching
├── monitor/          IP Helper tables, VPN detection, network changes
├── engine/           Proxy session lifecycle
├── console/          Interactive console UI (Russian)
├── onboarding/       Setup wizards (relay + WireGuard)
├── localnet/         QoS upload limits, before/after measurements
├── startup/          Autostart (Windows HKCU Run / Linux XDG)
├── storage/          Atomic file replacement
└── metrics/          Connection counters (no payload)
```

## Platforms

| Platform | Binary |
|---|---|
| Windows x64 | `NetworkRoute.exe` |
| Windows ARM64 | `NetworkRoute-arm64.exe` |
| Linux x64 | `networkroute-linux-amd64` |
| Linux ARM64 | `networkroute-linux-arm64` |
| Relay (Linux x64) | `nr-relay-linux-amd64` |
| Relay (Linux ARM64) | `nr-relay-linux-arm64` |

## Dependencies

| Module | Purpose |
|---|---|
| [quic-go](https://github.com/quic-go/quic-go) | QUIC streams + DATAGRAM |
| [x/sys](https://pkg.go.dev/golang.org/x/sys) | Windows IP Helper API |

## Docs

| Document | Description |
|---|---|
| [ARCHITECTURE.md](ARCHITECTURE.md) | Design, decisions, roadmap |
| [SECURITY.md](SECURITY.md) | Security policy and threat model |
| [docs/QUICKSTART.md](docs/QUICKSTART.md) | Step-by-step setup guide |
| [docs/CONSOLE.md](docs/CONSOLE.md) | Interactive menu reference |
| [docs/SELF_HOSTING.md](docs/SELF_HOSTING.md) | Deploy your own relay |
| [docs/BENCHMARKS.md](docs/BENCHMARKS.md) | Measurement methodology |
| [docs/LOCAL_CONTROL.md](docs/LOCAL_CONTROL.md) | QoS, autostart, local settings |

## Build & Test

```powershell
tools/build.ps1 -Test       # unit/integration tests + cross-compile
tools/package.ps1            # ZIP archives + SHA256SUMS
tools/test-release.ps1       # end-to-end release validation
```

```sh
go test ./... -count=1 -timeout=60s
go test -race ./... -timeout=90s
go test ./internal/protocol -fuzz=FuzzFraming -fuzztime=10s
```

## Current Limitations

- No signed WFP driver (transparent per-process routing planned)
- No GUI — console menu + CLI only
- No Windows Service
- No DNS DoH/DoT
- No throughput/bandwidth measurement
- No multi-relay

See [ARCHITECTURE.md](ARCHITECTURE.md) § 12 for the full implementation plan.
