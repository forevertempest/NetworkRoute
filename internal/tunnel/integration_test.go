package tunnel

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"networkroute/internal/identity"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Client, context.CancelFunc) {
	t.Helper()
	serverDir, clientDir := t.TempDir(), t.TempDir()
	serverPin, err := identity.Generate(serverDir)
	if err != nil {
		t.Fatal(err)
	}
	clientPin, err := identity.Generate(clientDir)
	if err != nil {
		t.Fatal(err)
	}
	stls, err := identity.Config(filepath.Join(serverDir, "identity.pem"), filepath.Join(serverDir, "identity.key"), []string{clientPin}, true)
	if err != nil {
		t.Fatal(err)
	}
	ctls, err := identity.Config(filepath.Join(clientDir, "identity.pem"), filepath.Join(clientDir, "identity.key"), []string{serverPin}, false)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{TLS: stls, testAllowLocal: true}
	l, err := s.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, l) }()
	c := &Client{Address: l.Addr().String(), TLS: ctls}
	t.Cleanup(func() {
		c.Close()
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("relay shutdown leaked flows")
		}
	})
	return c, cancel
}
func TestEncryptedTCPHalfClose(t *testing.T) {
	c, _ := fixture(t)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	payload := bytes.Repeat([]byte{0, 1, 2, 255}, 65536)
	go func() {
		peer, err := l.Accept()
		if err != nil {
			return
		}
		defer peer.Close()
		b, err := io.ReadAll(peer)
		if err == nil {
			peer.Write(b)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := c.DialTCP(ctx, l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err = conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	conn.(*StreamConn).CloseWrite()
	got, err := io.ReadAll(conn)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("TCP payload modified/truncated")
	}
}
func TestEncryptedUDPAndQUICSize(t *testing.T) {
	c, _ := fixture(t)
	udp, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	go func() {
		b := make([]byte, 65536)
		for {
			n, a, err := udp.ReadFromUDP(b)
			if err != nil {
				return
			}
			udp.WriteToUDP(b[:n], a)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	f, err := c.OpenUDP(ctx, udp.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, size := range []int{0, 10, 1200, 4096} {
		p := bytes.Repeat([]byte{byte(size % 251)}, size)
		if err = f.Send(p); err != nil {
			t.Fatal(err)
		}
		got, err := f.Receive(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, p) {
			t.Fatalf("UDP %d bytes changed", size)
		}
	}
}
func TestAuthenticationRejectsUnknownServer(t *testing.T) {
	c, _ := fixture(t)
	dir := t.TempDir()
	if _, err := identity.Generate(dir); err != nil {
		t.Fatal(err)
	}
	cfg, err := identity.Config(filepath.Join(dir, "identity.pem"), filepath.Join(dir, "identity.key"), []string{strings.Repeat("0", 64)}, false)
	if err != nil {
		t.Fatal(err)
	}
	bad := &Client{Address: c.Address, TLS: cfg}
	defer bad.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err = bad.Connect(ctx); err == nil {
		t.Fatal("untrusted server accepted")
	}
}
func TestAuthenticationRejectsUnknownClient(t *testing.T) {
	c, _ := fixture(t)
	dir := t.TempDir()
	identity.Generate(dir)
	cert, err := tls.LoadX509KeyPair(filepath.Join(dir, "identity.pem"), filepath.Join(dir, "identity.key"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := c.TLS.Clone()
	cfg.Certificates = []tls.Certificate{cert}
	bad := &Client{Address: c.Address, TLS: cfg}
	defer bad.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := bad.DialTCP(ctx, "127.0.0.1:9")
	if conn != nil {
		conn.Close()
	}
	if err == nil {
		t.Fatal("unauthorized client opened flow")
	}
}
func TestRelayFailure(t *testing.T) {
	c, stop := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	stop()
	time.Sleep(30 * time.Millisecond)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel2()
	conn, err := c.DialTCP(ctx2, "127.0.0.1:9")
	if conn != nil {
		conn.Close()
	}
	if err == nil {
		t.Fatal("dead relay accepted flow")
	}
}
func TestRelayDeniesLocalTargets(t *testing.T) {
	s := &Server{Resolver: net.DefaultResolver}
	for _, target := range []string{"127.0.0.1:80", "[::1]:80", "169.254.169.254:80", "localhost:80"} {
		if _, err := s.destination(context.Background(), target); err == nil {
			t.Fatalf("SSRF target allowed: %s", target)
		}
	}
}

func TestDestinationTLSRemainsEndToEnd(t *testing.T) {
	c, _ := fixture(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("destination TLS intact")) }))
	defer server.Close()
	trusted := server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	transport := &http.Transport{TLSClientConfig: trusted, DialContext: func(ctx context.Context, network, target string) (net.Conn, error) { return c.DialTCP(ctx, target) }}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "destination TLS intact" {
		t.Fatalf("TLS payload: %q %v", body, err)
	}
}
func TestIPv6TCPRelay(t *testing.T) {
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	defer listener.Close()
	go func() {
		peer, err := listener.Accept()
		if err != nil {
			return
		}
		defer peer.Close()
		io.Copy(peer, peer)
	}()
	c, _ := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := c.DialTCP(ctx, listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(time.Second))
	conn.Write([]byte("v6"))
	b := make([]byte, 2)
	if _, err = io.ReadFull(conn, b); err != nil || string(b) != "v6" {
		t.Fatalf("IPv6 forwarding failed %v", err)
	}
}
