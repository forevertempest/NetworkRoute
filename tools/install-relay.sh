#!/usr/bin/env bash
# Fresh systemd installation only; existing keys/services are never replaced.
set -euo pipefail
binary='' request='' address='' apply=0 output='./relay-connection.json'
while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary) binary=${2:?}; shift 2;;
    --client-request) request=${2:?}; shift 2;;
    --public-address) address=${2:?}; shift 2;;
    --output) output=${2:?}; shift 2;;
    --apply) apply=1; shift;;
    *) echo "Usage: $0 --binary ./nr-relay-linux-amd64 --client-request client-request.json --public-address host:7443 [--output file] [--apply]" >&2; exit 2;;
  esac
done
[[ -f $binary && -f $request && -n $address ]] || { echo 'Binary, public client request and endpoint are required.' >&2; exit 2; }
if [[ $apply == 0 ]]; then
  echo 'PLAN: validate public request, create separate relay identity, install dedicated unprivileged systemd service.'
  echo 'Owned paths: /opt/networkroute/nr-relay, /var/lib/networkroute, /etc/systemd/system/networkroute-relay.service'
  echo 'No firewall, routing, DNS or existing VPN changes. Re-run with sudo and --apply to install.'
  exit 0
fi
[[ $EUID == 0 ]] || { echo '--apply requires root on the VPS.' >&2; exit 2; }
for program in systemctl useradd getent install mktemp; do command -v "$program" >/dev/null || { echo "Missing $program" >&2; exit 2; }; done
[[ -d /run/systemd/system && -x $binary ]] || { echo 'Needs systemd and an executable binary for this VPS architecture (chmod +x).' >&2; exit 2; }
for target in /opt/networkroute /var/lib/networkroute /etc/systemd/system/networkroute-relay.service; do
  [[ ! -e $target ]] || { echo "Refusing to overwrite $target; existing deployment is preserved." >&2; exit 2; }
done
[[ ! -e $output ]] || { echo 'Public output file already exists; choose another --output.' >&2; exit 2; }
if getent passwd networkroute >/dev/null; then echo 'Dedicated networkroute account already exists; use the manual deployment guide.' >&2; exit 2; fi
umask 077
staging=$(mktemp -d /tmp/networkroute-setup.XXXXXXXX)
cleanup() { case "$staging" in /tmp/networkroute-setup.*) rm -rf -- "$staging";; esac; }
trap cleanup EXIT
# Complete validation and key generation before installing anything system-wide.
"$binary" setup --directory "$staging" --public-address "$address" --client-request "$request"
useradd --system --user-group --no-create-home --home-dir /var/lib/networkroute --shell /usr/sbin/nologin networkroute
install -d -m 755 /opt/networkroute
install -m 755 "$binary" /opt/networkroute/nr-relay
install -d -m 700 -o networkroute -g networkroute /var/lib/networkroute /var/lib/networkroute/identity
install -m 600 -o networkroute -g networkroute "$staging/relay.json" /var/lib/networkroute/relay.json
install -m 600 -o networkroute -g networkroute "$staging/identity/identity.key" /var/lib/networkroute/identity/identity.key
install -m 600 -o networkroute -g networkroute "$staging/identity/identity.pem" /var/lib/networkroute/identity/identity.pem
cat > /etc/systemd/system/networkroute-relay.service <<'UNIT'
[Unit]
Description=NetworkRoute authenticated relay
After=network-online.target
Wants=network-online.target
[Service]
User=networkroute
Group=networkroute
WorkingDirectory=/var/lib/networkroute
ExecStart=/opt/networkroute/nr-relay --config /var/lib/networkroute/relay.json
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
CapabilityBoundingSet=
UMask=0077
MemoryMax=512M
TasksMax=256
[Install]
WantedBy=multi-user.target
UNIT
chmod 644 /etc/systemd/system/networkroute-relay.service
# This export contains public keys/address only, never a private key.
install -m 644 "$staging/relay-connection.json" "$output"
systemctl daemon-reload
if ! systemctl enable --now networkroute-relay; then
  echo 'Service did not start. Files/keys are preserved; inspect journalctl -u networkroute-relay. No firewall/routes were changed.' >&2
  exit 1
fi
echo "Installed. Copy public file $output back to Windows and use networkroute pair."
echo "Allow UDP ${address##*:} in VPS/cloud firewall if necessary; no firewall rule was changed."
echo 'Check: systemctl status networkroute-relay; journalctl -u networkroute-relay'
