# Quick Start Guide

## Requirements

- Windows 10/11 or Linux (x64/ARM64)
- Go 1.26+ (only for building from source)

## Option 1: Standalone (No Server)

```powershell
# Download and run
NetworkRoute.exe
# Select menu option 1 to start
# Configure SOCKS5 127.0.0.1:1080 in your app
```

Or via CLI:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File tools/setup.ps1
.\bin\networkroute-windows-amd64.exe serve --config state/connection/config.local.json
```

Verify:
```powershell
curl.exe --proxy socks5h://127.0.0.1:1080 https://example.com/
```

## Option 2: With Your Linux VPS (Relay)

### On Windows (client):

```powershell
.\bin\networkroute-windows-amd64.exe setup --mode relay --directory state/connection
```

### On VPS (via SSH):

```bash
chmod +x bin/nr-relay-linux-amd64
# Dry run (shows plan):
bash tools/install-relay.sh --binary ./bin/nr-relay-linux-amd64 \
  --client-request ~/client-request.json --public-address your-vps:7443
# Apply:
sudo bash tools/install-relay.sh --binary ./bin/nr-relay-linux-amd64 \
  --client-request ~/client-request.json --public-address your-vps:7443 --apply
```

Open **UDP 7443** in your VPS firewall.

### Back on Windows:

```powershell
.\bin\networkroute-windows-amd64.exe pair --directory state/connection --ticket relay-connection.json
.\bin\networkroute-windows-amd64.exe doctor --config state/connection/config.local.json --check-relay
.\bin\networkroute-windows-amd64.exe test --config state/connection/config.local.json --target example.com:443
.\bin\networkroute-windows-amd64.exe serve --config state/connection/config.local.json
```

## Option 3: WireGuard VPN

See [tools/setup-wireguard.md](../tools/setup-wireguard.md) for full instructions.

## Troubleshooting

| Problem | Solution |
|---|---|
| QUIC timeout | Check VPS address, UDP 7443 in cloud firewall, `systemctl status networkroute-relay` |
| Pin mismatch | Verify request/ticket files match; never disable key verification |
| Port 1080 busy | Stop other instance or change port in config |
| VPN warning | Heuristic detection; relay suspended by default when VPN detected |
| Direct is faster | Keep Direct — having a VPS doesn't guarantee improvement |

## Stopping

`Ctrl+C` stops the proxy. Remove SOCKS5 settings from your apps after stopping.
No system routes, DNS, or firewall rules are modified — ever.
