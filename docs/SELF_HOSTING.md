# Self-Hosted Relay

> The relay is optional. Standalone Smart works without any server.

## Requirements

- Linux VPS with systemd
- SSH access
- Open UDP port (default: 7443)

## Automated Setup

Use the [Quick Start guide](QUICKSTART.md) — it handles key exchange and systemd installation.

## Manual Setup

### 1. Build

```sh
go build -trimpath -o nr-relay ./cmd/nr-relay
```

Or use pre-built `nr-relay-linux-amd64` / `nr-relay-linux-arm64`.

### 2. Generate Identities

**Client (Windows):**
```powershell
.\bin\networkroute-windows-amd64.exe init --dir secrets/client
```

**Server (Linux):**
```sh
./nr-relay --init-identity ./identity
```

Each outputs a public pin. Keys are valid for 1 year.

### 3. Run

```sh
./nr-relay --listen :7443 \
  --cert ./identity/identity.pem \
  --key ./identity/identity.key \
  --clients CLIENT_PUBLIC_PIN
```

Multiple client pins: comma-separated. Max 32 clients, 128 flows each.

### 4. Client Config

```json
{
  "version": 1,
  "listen": "127.0.0.1:1080",
  "mode": "smart",
  "profile": "INTERACTIVE",
  "fail_mode": "open",
  "certificate": "secrets/client/identity.pem",
  "private_key": "secrets/client/identity.key",
  "relay": {
    "address": "YOUR_VPS:7443",
    "server_pin": "64_HEX_CHARACTERS"
  }
}
```

### 5. Verify

```powershell
.\bin\networkroute-windows-amd64.exe relay --config config.local.json
.\bin\networkroute-windows-amd64.exe test --config config.local.json --target example.com:443
.\bin\networkroute-windows-amd64.exe serve --config config.local.json
```

## systemd

Example unit: [docs/networkroute-relay.service](networkroute-relay.service)

## Security Notes

- Relay blocks private/loopback/metadata destinations (SSRF protection)
- Only authenticated clients with pinned keys are served
- Not an open proxy — do not expose without key authentication
- Add egress firewall rules on public deployments

## Uninstall

Stop the service, remove its files. No routes, adapters, or firewall rules to clean up.
