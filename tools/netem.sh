#!/usr/bin/env bash
# Isolated development lab only. Never touches the host's routes or qdisc.
set -euo pipefail
if [[ $# -lt 5 ]]; then
  echo "usage: $0 delay_ms jitter_ms loss_percent rate command [args...]" >&2
  exit 2
fi
delay=$1 jitter=$2 loss=$3 rate=$4
shift 4
[[ $delay =~ ^[0-9]+$ && $jitter =~ ^[0-9]+$ && $loss =~ ^[0-9]+([.][0-9]+)?$ && $rate =~ ^[0-9]+(kbit|mbit|gbit)$ ]] || { echo "Invalid netem parameters" >&2; exit 2; }
[[ $EUID -eq 0 ]] || { echo "Network namespace creation requires root in the development lab" >&2; exit 2; }
namespace="networkroute-lab-$$"
created=0
cleanup() { if [[ $created == 1 ]]; then ip netns delete "$namespace"; fi; }
trap cleanup EXIT
ip netns add "$namespace"
created=1
ip -n "$namespace" link set lo up
ip netns exec "$namespace" tc qdisc add dev lo root netem delay "${delay}ms" "${jitter}ms" loss "${loss}%" rate "$rate"
ip netns exec "$namespace" "$@"
