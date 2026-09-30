#!/usr/bin/env bash
# Install a reviewed generated profile; default is a plan, not a mutation.
set -euo pipefail
bundle='' apply=0
while [[ $# -gt 0 ]]; do
  case "$1" in --bundle) bundle=${2:?}; shift 2;; --apply) apply=1; shift;; *) echo "usage: $0 --bundle ./vpn-bundle [--apply]" >&2; exit 2;; esac
done
[[ -f $bundle/nro-wg.conf ]] || { echo 'Generated nro-wg.conf is missing.' >&2; exit 2; }
if [[ $apply == 0 ]]; then
  echo 'PLAN: install /etc/wireguard/nro-wg.conf and start wg-quick@nro-wg.'
  if [[ -f $bundle/wireguard.nft ]]; then echo 'FULL IPv4 VPN: enables IPv4 forwarding and adds the owned nftables NAT table. Existing firewall rules remain effective; IPv6 Internet is not provided.'; fi
  echo 'Review files first. --apply requires root, WireGuard, systemd, iproute2, python3; full mode also needs nftables.'
  exit 0
fi
[[ $EUID == 0 && -d /run/systemd/system ]] || { echo 'Needs root on a Linux systemd VPS.' >&2; exit 2; }
for program in wg wg-quick ip python3 systemctl install; do command -v "$program" >/dev/null || { echo "Missing $program; install packages described in setup-wireguard.md." >&2; exit 2; }; done
[[ ! -e /etc/wireguard/nro-wg.conf && ! -e /etc/networkroute/wireguard.nft ]] || { echo 'Owned paths already exist; refusing to replace any profile.' >&2; exit 2; }
if ip link show nro-wg >/dev/null 2>&1; then echo 'Interface nro-wg already exists.' >&2; exit 2; fi
ip -j -4 route show table all | python3 -c 'import json,sys,ipaddress; subnet=ipaddress.ip_network("10.203.0.0/24"); conflicts=[r["dst"] for r in json.load(sys.stdin) if r.get("dst","default")!="default" and ipaddress.ip_network(r["dst"],strict=False).overlaps(subnet)]; sys.exit("Subnet conflict: "+str(conflicts) if conflicts else 0)'
if [[ -f $bundle/wireguard.nft ]]; then
  command -v nft >/dev/null || { echo 'Install nftables first.' >&2; exit 2; }
  if nft list table ip networkroute_wg >/dev/null 2>&1; then echo 'Owned NAT table already exists; refusing replacement.' >&2; exit 2; fi
  nft --check --file "$bundle/wireguard.nft"
fi
# wg-quick strip parses without executing hooks; never print its private-key output.
wg-quick strip "$bundle/nro-wg.conf" >/dev/null
install -d -m 700 /etc/wireguard
install -m 600 "$bundle/nro-wg.conf" /etc/wireguard/nro-wg.conf
if [[ -f $bundle/wireguard.nft ]]; then install -d -m 755 /etc/networkroute; install -m 600 "$bundle/wireguard.nft" /etc/networkroute/wireguard.nft; fi
if ! systemctl enable --now wg-quick@nro-wg; then
  echo 'VPN did not start. Profile preserved; inspect journalctl -u wg-quick@nro-wg. Existing VPN profiles/firewall rules were not replaced.' >&2
  exit 1
fi
echo 'VPN started. Open the configured UDP listen port in VPS/cloud firewall, then import the client profile on Windows.'
echo 'Check handshake using: sudo wg show nro-wg'
echo 'Stop with: sudo systemctl disable --now wg-quick@nro-wg'
echo 'Full mode leaves global IPv4 forwarding enabled when stopped; do not disable it if another VPN/router needs it.'
