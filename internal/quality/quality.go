// Package quality scores comparable observations. A failed TCP connect is NOT packet loss.
package quality

import (
	"encoding/json"
	"math"
	"networkroute/internal/policy"
	"sort"
	"sync"
	"time"
)

type Sample struct {
	Millis float64   `json:"millis"`
	OK     bool      `json:"ok"`
	At     time.Time `json:"at"`
}
type Stats struct {
	Samples       int      `json:"samples"`
	Successes     int      `json:"successes"`
	Median        float64  `json:"median_ms"`
	Min           float64  `json:"min_ms"`
	P95           float64  `json:"p95_ms"`
	P99           float64  `json:"p99_ms"`
	Jitter        float64  `json:"successive_variation_ms"`
	JitterSamples int      `json:"adjacent_success_pairs"`
	FailureRate   float64  `json:"failed_connect_fraction"`
	PacketLoss    *float64 `json:"packet_loss,omitempty"`
	Mbps          *float64 `json:"throughput_mbps,omitempty"`
}

func (s Stats) MarshalJSON() ([]byte, error) {
	v := map[string]any{"samples": s.Samples, "successes": s.Successes, "failed_connect_fraction": s.FailureRate, "packet_loss": s.PacketLoss, "throughput_mbps": s.Mbps, "median_ms": nil, "min_ms": nil, "p95_ms": nil, "p99_ms": nil, "successive_variation_ms": nil}
	if s.Successes > 0 {
		v["median_ms"] = s.Median
		v["min_ms"] = s.Min
		v["p95_ms"] = s.P95
		v["p99_ms"] = s.P99
	}
	if s.JitterSamples > 0 {
		v["successive_variation_ms"] = s.Jitter
	}
	if s.Samples == 0 {
		v["failed_connect_fraction"] = nil
	}
	return json.Marshal(v)
}

func Summarize(samples []Sample) Stats {
	s := Stats{Samples: len(samples)}
	values := []float64{}
	delta := 0.0
	pairs := 0
	for i, v := range samples {
		if !v.OK {
			continue
		}
		values = append(values, v.Millis)
		if i > 0 && samples[i-1].OK {
			delta += math.Abs(v.Millis - samples[i-1].Millis)
			pairs++
		}
	}
	s.Successes = len(values)
	if s.Samples > 0 {
		s.FailureRate = 1 - float64(s.Successes)/float64(s.Samples)
	}
	if len(values) == 0 {
		return s
	}
	sort.Float64s(values)
	s.Min = values[0]
	percentile := func(p float64) float64 { return values[int(math.Ceil(p*float64(len(values))))-1] }
	s.Median = percentile(.5)
	if len(values)%2 == 0 {
		s.Median = (values[len(values)/2-1] + values[len(values)/2]) / 2
	}
	s.P95 = percentile(.95)
	s.P99 = percentile(.99)
	if pairs > 0 {
		s.Jitter = delta / float64(pairs)
	}
	s.JitterSamples = pairs
	return s
}
func Score(s Stats, profile policy.Profile) float64 {
	if s.Successes == 0 {
		return math.Inf(1)
	}
	jitter, tail, failures := 1.0, .5, 1500.0
	switch profile {
	case policy.Realtime:
		jitter, tail, failures = 2, 1, 3000
	case policy.Bulk:
		jitter, tail, failures = .2, .3, 2500
	case policy.Streaming:
		jitter, tail, failures = .5, .5, 2500
	}
	score := s.Median + jitter*s.Jitter + tail*(s.P95-s.Median) + failures*s.FailureRate
	if s.PacketLoss != nil {
		score += failures * *s.PacketLoss
	}
	if s.Mbps != nil && (profile == policy.Bulk || profile == policy.Streaming) {
		score += 1000 / math.Max(*s.Mbps, .01)
	}
	return score
}

type Decision struct {
	Route   string    `json:"route"`
	Reason  string    `json:"reason"`
	Direct  Stats     `json:"direct"`
	Relay   Stats     `json:"relay"`
	Updated time.Time `json:"updated"`
}
type entry struct {
	Decision
	candidateSince   time.Time
	switched         time.Time
	candidateWindows int
}
type Engine struct {
	mu                   sync.Mutex
	entries              map[string]*entry
	TTL, Hold, Cooldown  time.Duration
	MinSamples, Capacity int
}

func NewEngine() *Engine {
	return &Engine{entries: make(map[string]*entry), TTL: 2 * time.Minute, Hold: 10 * time.Second, Cooldown: 30 * time.Second, MinSamples: 5, Capacity: 1024}
}
func (e *Engine) Reset() { e.mu.Lock(); defer e.mu.Unlock(); e.entries = make(map[string]*entry) }
func (e *Engine) Get(key string, now time.Time) (Decision, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	v, ok := e.entries[key]
	if !ok || now.Sub(v.Updated) > e.TTL {
		delete(e.entries, key)
		return Decision{}, false
	}
	return v.Decision, true
}
func (e *Engine) Fail(key string, now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if v := e.entries[key]; v != nil {
		v.Route = "direct"
		v.Reason = "relay unavailable; new flows use existing OS route"
		v.switched = now
		v.candidateSince = time.Time{}
		v.candidateWindows = 0
	}
}
func (e *Engine) Observe(key string, direct, relay Stats, profile policy.Profile, now time.Time) Decision {
	e.mu.Lock()
	defer e.mu.Unlock()
	v := e.entries[key]
	if v == nil || now.Sub(v.Updated) > e.TTL {
		if len(e.entries) >= e.Capacity {
			var oldest string
			var at time.Time
			for k, x := range e.entries {
				if at.IsZero() || x.Updated.Before(at) {
					oldest = k
					at = x.Updated
				}
			}
			delete(e.entries, oldest)
		}
		v = &entry{Decision: Decision{Route: "direct", Reason: "collecting comparable samples"}}
		e.entries[key] = v
	}
	v.Direct = direct
	v.Relay = relay
	v.Updated = now
	if relay.Successes == 0 {
		v.Route = "direct"
		v.Reason = "relay unavailable or probes failed"
		v.candidateSince = time.Time{}
		v.candidateWindows = 0
		return v.Decision
	}
	if direct.Samples < e.MinSamples || relay.Samples < e.MinSamples || relay.Successes < e.MinSamples {
		v.candidateSince = time.Time{}
		v.candidateWindows = 0
		v.Reason = "insufficient sample confidence; retained current route"
		return v.Decision
	}
	ds, rs := Score(direct, profile), Score(relay, profile)
	desired := "direct"
	if rs+5 < ds && rs < ds*.85 {
		desired = "relay"
	}
	if desired == v.Route {
		v.candidateSince = time.Time{}
		v.candidateWindows = 0
		v.Reason = "current route retained; no material stable improvement"
		return v.Decision
	}
	// Changing from relay to direct also needs a material improvement, except failures.
	if v.Route == "relay" && !(ds+5 < rs && ds < rs*.85) {
		v.candidateSince = time.Time{}
		v.candidateWindows = 0
		v.Reason = "within hysteresis; retained relay"
		return v.Decision
	}
	if v.candidateSince.IsZero() {
		v.candidateSince = now
		v.candidateWindows = 0
	}
	v.candidateWindows++
	if now.Sub(v.candidateSince) < e.Hold || v.candidateWindows < 2 || (!v.switched.IsZero() && now.Sub(v.switched) < e.Cooldown) {
		v.Reason = "candidate needs more stable observations / cooldown"
		return v.Decision
	}
	v.Route = desired
	v.switched = now
	v.candidateSince = time.Time{}
	v.candidateWindows = 0
	v.Reason = "material improvement sustained across measurement windows"
	return v.Decision
}
