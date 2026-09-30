package quality

import (
	"networkroute/internal/policy"
	"testing"
	"time"
)

func stats(values ...float64) Stats {
	var samples []Sample
	for _, v := range values {
		samples = append(samples, Sample{OK: v >= 0, Millis: v})
	}
	return Summarize(samples)
}
func TestStatistics(t *testing.T) {
	s := stats(10, 20, -1, 30, 40)
	if s.Median != 25 || s.P95 != 40 || s.Successes != 4 || s.Jitter != 10 || s.FailureRate < .19 || s.FailureRate > .21 {
		t.Fatalf("bad statistics %+v", s)
	}
	if s.PacketLoss != nil || s.Mbps != nil {
		t.Fatal("unknown metrics must stay unknown")
	}
}
func TestHysteresisAndFailure(t *testing.T) {
	e := NewEngine()
	now := time.Now()
	d, r := stats(58, 58, 58, 58, 58), stats(39, 39, 39, 39, 39)
	if e.Observe("a", d, r, policy.Realtime, now).Route != "direct" {
		t.Fatal("premature switch")
	}
	if e.Observe("a", d, r, policy.Realtime, now.Add(11*time.Second)).Route != "relay" {
		t.Fatal("stable improvement not selected")
	}
	e.Fail("a", now.Add(12*time.Second))
	v, _ := e.Get("a", now.Add(12*time.Second))
	if v.Route != "direct" {
		t.Fatal("failure did not fail open")
	}
}
func TestDirectOptimalAndInsufficientEvidence(t *testing.T) {
	e := NewEngine()
	now := time.Now()
	for i := 0; i < 10; i++ {
		v := e.Observe("a", stats(17, 17, 17, 17, 17), stats(28, 28, 28, 28, 28), policy.Interactive, now.Add(time.Duration(i)*time.Second))
		if v.Route != "direct" {
			t.Fatal("worse relay selected")
		}
	}
	v := e.Observe("b", stats(90), stats(10), policy.Auto, now)
	if v.Route != "direct" {
		t.Fatal("one sample selected relay")
	}
}
func TestLossJitterCacheAndEpoch(t *testing.T) {
	stable := stats(40, 40, 40, 40, 40)
	bad := stats(10, 10, -1, 10, 200)
	if Score(bad, policy.Realtime) <= Score(stable, policy.Realtime) {
		t.Fatal("loss/spikes not penalized")
	}
	e := NewEngine()
	e.Capacity = 2
	now := time.Now()
	for _, key := range []string{"a", "b", "c"} {
		e.Observe(key, stable, bad, policy.Auto, now)
		now = now.Add(time.Second)
	}
	if len(e.entries) != 2 {
		t.Fatal("unbounded cache")
	}
	e.Reset()
	if _, ok := e.Get("c", now); ok {
		t.Fatal("network epoch retained stale result")
	}
}
func TestSimulation(t *testing.T) {
	e := NewEngine()
	now := time.Now()
	for i := 0; i < 60; i++ {
		direct := stats(47, 47, 47, 47, 47)
		relay := stats(46, 46, 46, 46, 46)
		if i%2 == 0 {
			relay = stats(48, 48, 48, 48, 48)
		}
		if e.Observe("flow", direct, relay, policy.Realtime, now.Add(time.Duration(i)*time.Second)).Route != "direct" {
			t.Fatal("route flapping")
		}
	}
}
