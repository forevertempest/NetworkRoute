package quality

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"
)

// Interleave preserves the resolver's preferred family, but gives the other
// family the next attempt instead of waiting behind every preferred-family IP.
func Interleave(addresses []string) []string {
	var first, other []string
	var family6 bool
	for i, a := range addresses {
		ap, err := netip.ParseAddrPort(a)
		if err != nil {
			continue
		}
		v6 := ap.Addr().Is6()
		if i == 0 {
			family6 = v6
		}
		if v6 == family6 {
			first = append(first, a)
		} else {
			other = append(other, a)
		}
	}
	result := make([]string, 0, len(addresses))
	for i := 0; i < len(first) || i < len(other); i++ {
		if i < len(first) {
			result = append(result, first[i])
		}
		if i < len(other) {
			result = append(result, other[i])
		}
	}
	return result
}

// DialStaggered selects the first successful TCP handshake; no application
// payload is sent on losing sockets. It changes no routes or VPN configuration.
func DialStaggered(ctx context.Context, addresses []string, delay time.Duration, dial Dial) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	addresses = Interleave(addresses)
	if len(addresses) > 8 {
		addresses = addresses[:8]
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("no numeric destinations to connect to")
	}
	raceCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		conn net.Conn
		err  error
	}
	results := make(chan result)
	for i, address := range addresses {
		go func(index int, target string) {
			if index > 0 {
				timer := time.NewTimer(time.Duration(index) * delay)
				defer timer.Stop()
				select {
				case <-timer.C:
				case <-raceCtx.Done():
					return
				}
			}
			conn, err := dial(raceCtx, target)
			if err == nil && conn == nil {
				err = fmt.Errorf("dial returned no connection")
			}
			if err != nil && conn != nil {
				conn.Close()
				conn = nil
			}
			select {
			case results <- result{conn: conn, err: err}:
			case <-raceCtx.Done():
				if conn != nil {
					conn.Close()
				}
			}
		}(i, address)
	}
	var failures []error
	for range addresses {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case r := <-results:
			if r.err == nil {
				if err := ctx.Err(); err != nil {
					r.conn.Close()
					return nil, err
				}
				return r.conn, nil
			}
			failures = append(failures, r.err)
		}
	}
	return nil, errors.Join(failures...)
}

type DirectEndpoint struct {
	Target  string `json:"target"`
	Family  string `json:"family"`
	Quality Stats  `json:"quality"`
}
type DirectReport struct {
	Metric  string           `json:"metric"`
	Results []DirectEndpoint `json:"results"`
	Note    string           `json:"note"`
}

// MeasureDirect rotates the order of a bounded set of resolved endpoints. It
// compares endpoint connection times, not equal-destination alternate routes.
func MeasureDirect(ctx context.Context, addresses []string, count int, timeout time.Duration, dial Dial) DirectReport {
	if len(addresses) > 8 {
		addresses = addresses[:8]
	}
	report := DirectReport{Metric: "TCP connection establishment through existing OS routes", Note: "Different DNS endpoints/families may have different servers. Not a same-destination routing comparison, packet RTT or throughput measurement."}
	samples := make([][]Sample, len(addresses))
	for round := 0; round < count && ctx.Err() == nil; round++ {
		for offset := range addresses {
			if ctx.Err() != nil {
				break
			}
			index := (round + offset) % len(addresses)
			at := time.Now()
			probe, cancel := context.WithTimeout(ctx, timeout)
			conn, err := dial(probe, addresses[index])
			elapsed := time.Since(at)
			cancel()
			if conn != nil {
				conn.Close()
			}
			samples[index] = append(samples[index], Sample{Millis: float64(elapsed) / float64(time.Millisecond), OK: err == nil, At: at})
		}
		if round+1 < count {
			select {
			case <-ctx.Done():
			case <-time.After(150 * time.Millisecond):
			}
		}
	}
	for i, a := range addresses {
		family := "IPv4"
		if ap, err := netip.ParseAddrPort(a); err == nil && ap.Addr().Is6() {
			family = "IPv6"
		}
		report.Results = append(report.Results, DirectEndpoint{Target: a, Family: family, Quality: Summarize(samples[i])})
	}
	return report
}
