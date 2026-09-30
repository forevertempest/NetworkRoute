package tunnel

import (
	"context"
	"crypto/tls"
	"net"
	"testing"
	"time"
)

func TestHandshakeWaitRespectsIndependentDeadline(t *testing.T) {
	blackhole, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer blackhole.Close()
	client := &Client{Address: blackhole.LocalAddr().String(), TLS: &tls.Config{MinVersion: tls.VersionTLS13, NextProtos: []string{"networkroute/1"}}}
	defer client.Close()
	first, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- client.Connect(first) }()
	time.Sleep(10 * time.Millisecond)
	second, stop := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer stop()
	start := time.Now()
	if err = client.Connect(second); err == nil {
		t.Fatal("blackhole connected")
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Fatal("second caller blocked on another caller's handshake deadline")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handshake cancellation leaked")
	}
}
