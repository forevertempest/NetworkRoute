# Console Menu Reference

## Starting

Double-click `NetworkRoute.exe` or run without arguments. No Go/source code needed.

Settings stored in `%LOCALAPPDATA%\NetworkRoute` (Windows) or `~/.config/NetworkRoute` (Linux).
Portable mode: `NetworkRoute.exe menu --data-dir PATH`

## Menu Options

| # | Action | Notes |
|---|---|---|
| **1** | Start / Stop proxy | Starts SOCKS5 on 127.0.0.1:1080 |
| **2** | Connection settings | Mode, profile, port, fail policy, bypass rules |
| **3** | VPN & VPS | Relay setup, WireGuard config generation |
| **4** | Network & apps | Interfaces, connections with PID/process |
| **5** | Route comparison | TCP connect-time benchmark (not packet RTT) |
| **6** | Config check | Validates config, port, VPN state, relay handshake |
| **7** | Guide | Built-in offline documentation |
| **8** | Counters & log | Current session metrics (no payload logged) |
| **9** | Diagnostics | Routes, interfaces, DNS, MTU (read-only) |
| **10** | Local controls | Before/after measurements, adapter load, QoS limits |
| **11** | Autostart | Login startup, auto-start proxy on launch |
| **0** | Exit | |

## Modes

| Mode | Behavior |
|---|---|
| `smart` | Starts Direct, compares with relay if configured. Without relay: races IPv4/IPv6 endpoints |
| `direct` | Uses current OS routing (including active VPN) |
| `relay` | Forces relay for eligible traffic (no speed guarantee) |

## Profiles

`REALTIME` · `INTERACTIVE` · `STREAMING` · `BULK` · `AUTO`

AUTO = INTERACTIVE. STREAMING/BULK stay Direct in Smart (no throughput measurement yet).

## Fail Modes

- **open** — falls back to Direct if relay fails (before payload)
- **closed** — blocks eligible connections without relay (not a system firewall)

## Linux

Copy `networkroute-linux-amd64`, `chmod +x`, run from terminal. Same menu and features.
