package quality

import (
	"context"
	"net"
	"time"
)

type Dial func(context.Context, string) (net.Conn, error)
type Benchmark struct {
	Target         string    `json:"target"`
	Metric         string    `json:"metric"`
	Direct         Stats     `json:"direct"`
	Relay          Stats     `json:"relay"`
	Started        time.Time `json:"started"`
	DurationMillis int64     `json:"duration_ms"`
}

// Measure alternates order, measures the same numeric endpoint, and sends no application data.
func Measure(ctx context.Context, target string, count int, timeout time.Duration, direct, relay Dial) Benchmark {
	started := time.Now()
	a, b := []Sample{}, []Sample{}
	probe := func(d Dial) Sample {
		at := time.Now()
		pctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		c, err := d(pctx, target)
		elapsed := time.Since(at)
		if c != nil {
			c.Close()
		}
		return Sample{Millis: float64(elapsed) / float64(time.Millisecond), OK: err == nil, At: at}
	}
	for i := 0; i < count && ctx.Err() == nil; i++ {
		if i%2 == 0 {
			a = append(a, probe(direct))
			if ctx.Err() == nil {
				b = append(b, probe(relay))
			}
		} else {
			b = append(b, probe(relay))
			if ctx.Err() == nil {
				a = append(a, probe(direct))
			}
		}
		if i+1 < count {
			select {
			case <-ctx.Done():
			case <-time.After(150 * time.Millisecond):
			}
		}
	}
	return Benchmark{Target: target, Metric: "TCP destination connect confirmation (not packet RTT)", Direct: Summarize(a), Relay: Summarize(b), Started: started, DurationMillis: time.Since(started).Milliseconds()}
}
func NativeDial(ctx context.Context, target string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "tcp", target)
}
