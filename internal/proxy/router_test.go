package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/netip"
	"networkroute/internal/config"
	"networkroute/internal/policy"
	"networkroute/internal/tunnel"
	"testing"
	"time"
)

type fixedResolver struct {
	ips []netip.Addr
	err error
}

func (r fixedResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return r.ips, r.err
}
func TestBrokenIPv6FallsBackToIPv4(t *testing.T) {
	r := NewRouter(context.Background(), config.Config{Mode: "direct", FailMode: "open", Profile: policy.Auto}, nil)
	defer r.Close()
	r.resolver = fixedResolver{ips: []netip.Addr{netip.MustParseAddr("2606:4700:4700::1111"), netip.MustParseAddr("1.1.1.1")}}
	calls := 0
	r.nativeDial = func(ctx context.Context, target string) (net.Conn, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("simulated broken IPv6")
		}
		a, b := net.Pipe()
		b.Close()
		return a, nil
	}
	c, route, err := r.DialTCP(context.Background(), "test.example:443")
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if calls != 2 || route != "direct" {
		t.Fatal("IPv4 fallback not attempted")
	}
}
func TestRelayFailureFallbackBeforePayloadAndFailClosed(t *testing.T) {
	for _, mode := range []string{"open", "closed"} {
		t.Run(mode, func(t *testing.T) {
			relay := &tunnel.Client{Address: "127.0.0.1:0", TLS: &tls.Config{MinVersion: tls.VersionTLS13}}
			r := NewRouter(context.Background(), config.Config{Mode: "relay", FailMode: mode, Profile: policy.Auto, AllowNestedTunnel: true}, relay)
			defer r.Close()
			calls := 0
			r.nativeDial = func(ctx context.Context, target string) (net.Conn, error) {
				calls++
				a, b := net.Pipe()
				b.Close()
				return a, nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			c, route, err := r.DialTCP(ctx, "1.1.1.1:443")
			if mode == "closed" {
				if err == nil || calls != 0 {
					t.Fatal("fail-closed leaked to Direct")
				}
			} else {
				if err != nil || route != "direct" || calls != 1 {
					t.Fatalf("fallback failed: route=%s calls=%d error=%v", route, calls, err)
				}
				c.Close()
			}
		})
	}
}
func TestDNSFailureDoesNotDial(t *testing.T) {
	r := NewRouter(context.Background(), config.Config{Mode: "direct", FailMode: "open"}, nil)
	defer r.Close()
	r.resolver = fixedResolver{err: errors.New("DNS down")}
	r.nativeDial = func(context.Context, string) (net.Conn, error) { t.Fatal("dial after DNS failure"); return nil, nil }
	if _, _, err := r.DialTCP(context.Background(), "test.example:443"); err == nil {
		t.Fatal("DNS error hidden")
	}
}
func TestNetworkChangeClearsMeasurements(t *testing.T) {
	r := NewRouter(context.Background(), config.Config{Mode: "direct", FailMode: "open", AllowNestedTunnel: true}, nil)
	defer r.Close()
	before := r.epoch.Load()
	r.NetworkChanged()
	if r.epoch.Load() != before+1 {
		t.Fatal("network epoch did not advance")
	}
}
