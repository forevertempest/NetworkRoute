package quality

import (
	"context"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestInterleaveFamilies(t *testing.T) {
	got := Interleave([]string{"[::1]:1", "[::2]:1", "127.0.0.1:1", "127.0.0.2:1"})
	if len(got) != 4 || got[1] != "127.0.0.1:1" || got[2] != "[::2]:1" {
		t.Fatalf("bad family order: %v", got)
	}
}
func TestRaceFallsBackWithoutWaitingForBrokenIPv6(t *testing.T) {
	firstStopped := make(chan struct{})
	var attempts atomic.Int32
	dial := func(ctx context.Context, target string) (net.Conn, error) {
		attempts.Add(1)
		if target == "[::1]:1" {
			defer close(firstStopped)
			<-ctx.Done()
			return nil, ctx.Err()
		}
		a, b := net.Pipe()
		b.Close()
		return a, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := DialStaggered(ctx, []string{"[::1]:1", "127.0.0.1:1"}, 5*time.Millisecond, dial)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	select {
	case <-firstStopped:
	case <-time.After(time.Second):
		t.Fatal("losing dial not canceled")
	}
	if attempts.Load() != 2 {
		t.Fatal("did not try both families")
	}
}
func TestRaceClosesLateSuccessfulLoser(t *testing.T) {
	losingPeer := make(chan net.Conn, 1)
	lateDone := make(chan struct{})
	dial := func(ctx context.Context, target string) (net.Conn, error) {
		a, b := net.Pipe()
		if target == "[::1]:1" {
			losingPeer <- b
			<-ctx.Done()
			defer close(lateDone)
			return a, nil
		}
		b.Close()
		return a, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := DialStaggered(ctx, []string{"[::1]:1", "127.0.0.1:1"}, 5*time.Millisecond, dial)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	<-lateDone
	peer := <-losingPeer
	defer peer.Close()
	peer.SetReadDeadline(time.Now().Add(time.Second))
	var one [1]byte
	if _, err = peer.Read(one[:]); !errors.Is(err, io.EOF) {
		t.Fatalf("loser socket not closed: %v", err)
	}
}
func TestRaceCancellationAndBound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := DialStaggered(ctx, []string{"127.0.0.1:1"}, 0, func(ctx context.Context, _ string) (net.Conn, error) { return nil, ctx.Err() }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	var count atomic.Int32
	addresses := make([]string, 12)
	for i := range addresses {
		addresses[i] = "127.0.0.1:1"
	}
	_, err := DialStaggered(context.Background(), addresses, 0, func(context.Context, string) (net.Conn, error) { count.Add(1); return nil, errors.New("unreachable") })
	if err == nil || count.Load() != 8 {
		t.Fatalf("unbounded attempts: %d %v", count.Load(), err)
	}
}
func TestDirectMeasurementsRetainFailures(t *testing.T) {
	report := MeasureDirect(context.Background(), []string{"[::1]:1", "127.0.0.1:1"}, 2, time.Second, func(_ context.Context, target string) (net.Conn, error) {
		if target == "[::1]:1" {
			return nil, errors.New("IPv6 down")
		}
		a, b := net.Pipe()
		b.Close()
		return a, nil
	})
	if len(report.Results) != 2 || report.Results[0].Quality.FailureRate != 1 || report.Results[1].Quality.Successes != 2 {
		t.Fatalf("bad direct report: %+v", report)
	}
}
