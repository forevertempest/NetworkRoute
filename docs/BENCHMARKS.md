# Measurement Methodology

## What Is Measured

**TCP connection establishment time** — client-observed time to successful TCP connect.

This is **NOT**: packet RTT, TLS handshake time, TTFB, throughput, or UDP loss.
Unknown values are `null`, never zero.

### Standalone (no relay)

Samples up to 8 resolved endpoints through existing OS routes. Different IPs may be different servers — this is an endpoint comparison, not a routing comparison.

### With Relay

Alternates Direct and Relay probes to the **same numeric IP:port**. No application payload sent. Relay measurement includes full QUIC stream + relay's destination connect.

## Smart Decision Criteria

| Parameter | Value |
|---|---|
| Measurement windows | 2 |
| Min relay successes | 5 |
| Stability hold | 10 seconds |
| Absolute threshold | 5 ms improvement |
| Relative threshold | 15% improvement |
| Switch cooldown | 30 seconds |

STREAMING/BULK stay Direct (no throughput evidence). UDP stays Direct in Smart.

## Running Tests

```sh
go test ./... -count=1 -timeout=60s
go vet ./...
go test -race ./... -timeout=90s
go test ./internal/protocol -fuzz=FuzzFraming -fuzztime=10s
go test ./internal/proxy -fuzz=FuzzSOCKSAddress -fuzztime=10s
```

## Test Coverage

- Score/hysteresis/no-flapping scenarios
- Mutual TLS auth and rejection
- TCP half-close, binary payload integrity
- SOCKS5 TCP + UDP full path through QUIC relay
- IPv6, DNS failure, fail-open/closed
- Protocol fuzzing, fragment reassembly/loss
- Windows IP Helper table parsing

## Linux Impairment Lab

```sh
sudo bash tools/netem.sh 20 5 0 20mbit ./test-binary -test.run TestName
```

Uses isolated network namespace — never modifies host qdiscs.

## Not Yet Validated

- Real WAN latency improvement
- Windows ARM64 runtime
- High-load CPU/RAM benchmarks
- HTTP/3 client compatibility
- Sleep/resume behavior
- Third-party VPN coexistence matrix
