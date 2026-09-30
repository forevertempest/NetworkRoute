package proxy

import (
	"context"
	"io"
	"net"
	"net/netip"
	"networkroute/internal/config"
	"networkroute/internal/identity"
	"networkroute/internal/policy"
	"networkroute/internal/tunnel"
	"path/filepath"
	"testing"
	"time"
)

type endpointDialer struct{ target string }

func (d endpointDialer) DialContext(ctx context.Context, network, target string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, network, d.target)
}

// Full data plane. Only the destination dialer is redirected to a local echo;
// SOCKS framing, route selection, mTLS, QUIC, relay framing and payload I/O are real.
func TestSOCKSThroughAuthenticatedRelay(t *testing.T) {
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		c, err := echo.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		io.Copy(c, c)
	}()
	sd, cd := t.TempDir(), t.TempDir()
	sp, err := identity.Generate(sd)
	if err != nil {
		t.Fatal(err)
	}
	cp, err := identity.Generate(cd)
	if err != nil {
		t.Fatal(err)
	}
	stls, err := identity.Config(filepath.Join(sd, "identity.pem"), filepath.Join(sd, "identity.key"), []string{cp}, true)
	if err != nil {
		t.Fatal(err)
	}
	ctls, err := identity.Config(filepath.Join(cd, "identity.pem"), filepath.Join(cd, "identity.key"), []string{sp}, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	relay := &tunnel.Server{TLS: stls, Dialer: endpointDialer{target: echo.Addr().String()}}
	ql, err := relay.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- relay.Serve(ctx, ql) }()
	client := &tunnel.Client{Address: ql.Addr().String(), TLS: ctls}
	r := NewRouter(ctx, config.Config{Mode: "relay", FailMode: "closed", Profile: policy.Interactive, MaxConnections: 8, AllowNestedTunnel: true}, client)
	r.resolver = fixedResolver{ips: []netip.Addr{netip.MustParseAddr("1.1.1.1")}}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	proxyDone := make(chan error, 1)
	go func() { proxyDone <- (&Server{Router: r}).Serve(ctx, l) }()
	defer func() {
		cancel()
		r.Close()
		for _, d := range []chan error{done, proxyDone} {
			select {
			case err := <-d:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(3 * time.Second):
				t.Error("full data plane did not shut down")
			}
		}
	}()
	conn, _ := socksRequest(t, l.Addr().String(), 1, "test.example:443")
	defer conn.Close()
	conn.Write([]byte("opaque application bytes"))
	buf := make([]byte, len("opaque application bytes"))
	if _, err = io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "opaque application bytes" || r.Metrics.Relayed.Load() != 1 || r.Metrics.Direct.Load() != 0 {
		t.Fatal("payload did not use relay exclusively")
	}
}
