package monitor

import (
	"context"
	"time"
)

type Traffic struct {
	Name        string  `json:"name"`
	Received    uint64  `json:"received_bytes"`
	Sent        uint64  `json:"sent_bytes"`
	ReceiveMbps float64 `json:"receive_mbps"`
	SendMbps    float64 `json:"send_mbps"`
}

func TrafficRates(ctx context.Context) ([]Traffic, error) {
	before, err := TrafficSnapshot()
	if err != nil {
		return nil, err
	}
	at := time.Now()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(time.Second):
	}
	after, err := TrafficSnapshot()
	if err != nil {
		return nil, err
	}
	seconds := time.Since(at).Seconds()
	old := map[string]Traffic{}
	for _, r := range before {
		old[r.Name] = r
	}
	for i := range after {
		b, ok := old[after[i].Name]
		if !ok {
			continue
		}
		if after[i].Received >= b.Received {
			after[i].ReceiveMbps = float64(after[i].Received-b.Received) * 8 / seconds / 1e6
		}
		if after[i].Sent >= b.Sent {
			after[i].SendMbps = float64(after[i].Sent-b.Sent) * 8 / seconds / 1e6
		}
	}
	return after, nil
}
