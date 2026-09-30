# Local Controls

## Release Files

| Platform | File |
|---|---|
| Windows x64 | `NetworkRoute.exe` |
| Windows ARM64 | `NetworkRoute-arm64.exe` |
| Linux x64 | `networkroute-linux-amd64` |
| Linux ARM64 | `networkroute-linux-arm64` |

Only one file needed per platform. No Go, DLLs, or source code required.

## Connection Settings (Menu 2)

Mode, profile, SOCKS port, fail policy, VPN/relay, bypass rules, connection limit.

`direct_race_delay_ms`: 10–2000 ms (default 250). Controls stagger interval for Smart TCP racing without relay.

## Measurements (Menu 10)

- Target: `host:port`
- Probes: 5–60 TCP connects
- Timeout: 100–5000 ms
- **Before/After** comparison uses same numeric address

Reports saved in `data-dir/reports/`. This measures TCP connect time, not game ping or throughput.

CLI: `networkroute local settings|traffic|before|after --data-dir PATH`

## Windows QoS Upload Limits (Menu 10)

Set per-application outbound bandwidth limits (kbit/s). Example: `1000` = 1 Mbit/s.

- Uses Windows NetQoS ActiveStore
- **Requires administrator** to apply/remove
- Survives app close, cleared on reboot
- Only removes policies created by NetworkRoute

CLI: `networkroute local status|apply|remove --data-dir PATH`

## Autostart (Menu 11)

Copies binary to data directory, registers login startup:
- **Windows:** HKCU Run (no elevation)
- **Linux:** XDG Autostart (requires graphical desktop + terminal)

CLI: `networkroute autostart enable|disable|status --data-dir PATH`
