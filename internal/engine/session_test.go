package engine

import (
	"context"
	"io"
	"net"
	"networkroute/internal/config"
	"networkroute/internal/policy"
	"testing"
	"time"
)

func TestStopClosesConnectionsAndAllowsRestart(t *testing.T) {
	c := config.Config{Version: 1, Listen: "127.0.0.1:0", Mode: "smart", Profile: policy.Auto, FailMode: "open", MaxConnections: 8}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session, err := Start(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialTimeout("tcp", session.Address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(time.Second))
	if _, err = conn.Write([]byte{5, 1, 0}); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 2)
	if _, err = io.ReadFull(conn, reply); err != nil {
		t.Fatal(err)
	}
	if reply[0] != 5 || reply[1] != 0 {
		t.Fatal("SOCKS server not running")
	}
	shutdown, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	if err = session.Stop(shutdown); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Read(reply); err == nil {
		t.Fatal("client connection survived stop")
	}
	c.Listen = session.Address
	again, err := Start(ctx, c)
	if err != nil {
		t.Fatalf("port not released: %v", err)
	}
	if err = again.Stop(shutdown); err != nil {
		t.Fatal(err)
	}
}
