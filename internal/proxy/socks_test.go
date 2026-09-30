package proxy

import (
	"bytes"
	"context"
	"io"
	"net"
	"networkroute/internal/config"
	"networkroute/internal/policy"
	"testing"
	"time"
)

func proxyFixture(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := NewRouter(ctx, config.Config{Mode: "smart", FailMode: "open", Profile: policy.Auto, MaxConnections: 8}, nil)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- (&Server{Router: r}).Serve(ctx, l) }()
	t.Cleanup(func() {
		cancel()
		r.Close()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			t.Error("SOCKS shutdown leaked goroutine")
		}
	})
	return l.Addr().String()
}
func socksRequest(t *testing.T, proxy string, command byte, target string) (net.Conn, string) {
	t.Helper()
	c, err := net.DialTimeout("tcp", proxy, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	c.SetDeadline(time.Now().Add(3 * time.Second))
	c.Write([]byte{5, 1, 0})
	var greeting [2]byte
	if _, err = io.ReadFull(c, greeting[:]); err != nil || greeting != [2]byte{5, 0} {
		t.Fatalf("greeting %v %v", greeting, err)
	}
	addr, err := encodeAddress(target)
	if err != nil {
		t.Fatal(err)
	}
	c.Write(append([]byte{5, command, 0}, addr...))
	var reply [4]byte
	if _, err = io.ReadFull(c, reply[:]); err != nil || reply[1] != 0 {
		t.Fatalf("request reply %v %v", reply, err)
	}
	bound, err := readAddress(c, reply[3])
	if err != nil {
		t.Fatal(err)
	}
	return c, bound
}
func TestSOCKSTCPAndHalfClose(t *testing.T) {
	proxy := proxyFixture(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		c, err := listener.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		b, _ := io.ReadAll(c)
		c.Write(b)
	}()
	c, _ := socksRequest(t, proxy, 1, listener.Addr().String())
	payload := bytes.Repeat([]byte("binary\x00\xff"), 10000)
	c.Write(payload)
	c.(*net.TCPConn).CloseWrite()
	got, err := io.ReadAll(c)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("SOCKS payload mismatch: %v", err)
	}
}
func TestSOCKSUDPAssociation(t *testing.T) {
	proxy := proxyFixture(t)
	echo, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		b := make([]byte, 65536)
		for {
			n, a, err := echo.ReadFromUDP(b)
			if err != nil {
				return
			}
			echo.WriteToUDP(b[:n], a)
		}
	}()
	control, bound := socksRequest(t, proxy, 3, "0.0.0.0:0")
	defer control.Close()
	addr, err := net.ResolveUDPAddr("udp", bound)
	if err != nil {
		t.Fatal(err)
	}
	c, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(time.Second))
	dest, _ := encodeAddress(echo.LocalAddr().String())
	payload := bytes.Repeat([]byte{3}, 1200)
	packet := append(append([]byte{0, 0, 0}, dest...), payload...)
	if _, err = c.Write(packet); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 2048)
	n, err := c.Read(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b[:n], packet) {
		t.Fatal("SOCKS UDP datagram changed")
	}
}
func TestAddressFamiliesAndInvalidFrames(t *testing.T) {
	for _, target := range []string{"[::1]:443", "1.2.3.4:80", "example.com:9"} {
		b, err := encodeAddress(target)
		if err != nil {
			t.Fatal(err)
		}
		got, err := readAddress(bytes.NewReader(b[1:]), b[0])
		if err != nil || got != target {
			t.Fatalf("%s %s %v", target, got, err)
		}
	}
	if _, err := readAddress(bytes.NewReader(nil), 7); err == nil {
		t.Fatal("invalid type accepted")
	}
}
func FuzzSOCKSAddress(f *testing.F) {
	f.Add(byte(3), []byte{3, 'a', 'b', 'c', 0, 80})
	f.Fuzz(func(t *testing.T, kind byte, b []byte) { readAddress(bytes.NewReader(b), kind) })
}
