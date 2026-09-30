package console

import (
	"bytes"
	"context"
	"io"
	"net"
	"networkroute/internal/config"
	"networkroute/internal/localnet"
	"strings"
	"testing"
	"time"
)

func TestLocalMenuSavesRulesWithoutApplyingThem(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	var out bytes.Buffer
	a := New(ctx, dir, "test", strings.NewReader("10\n2\nbackup.exe\n500\n\n11\n3\n\n0\n"), &out)
	if err := a.Run(); err != nil {
		t.Fatal(err)
	}
	s, err := localnet.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !s.StartOnLaunch || len(s.Rules) != 1 || s.Rules[0].UploadKbps != 500 {
		t.Fatalf("not saved: %+v", s)
	}
}

func TestAutomaticStartUsesSavedProfile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	out := &observedOutput{ready: make(chan struct{})}
	a := New(ctx, dir, "test", input, out)
	if err := a.loadState(); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(a.configPath)
	if err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	c.Listen = l.Addr().String()
	l.Close()
	if err := saveConfig(a.configPath, c); err != nil {
		t.Fatal(err)
	}
	s := localnet.Defaults()
	s.StartOnLaunch = true
	if err := localnet.Save(dir, s); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- a.Run() }()
	select {
	case <-out.ready:
	case <-time.After(3 * time.Second):
		t.Fatal("startup stalled")
	}
	conn, err := net.DialTimeout("tcp", c.Listen, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	conn.SetDeadline(time.Now().Add(time.Second))
	conn.Write([]byte{5, 1, 0})
	var reply [2]byte
	if _, err := io.ReadFull(conn, reply[:]); err != nil || reply != [2]byte{5, 0} {
		t.Fatalf("SOCKS not running: %v", err)
	}
	conn.Close()
	if _, err := lockDirectory(dir); err == nil {
		t.Fatal("second instance acquired lock")
	}
	io.WriteString(writer, "0\n")
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("automatic start did not stop")
	}
	unlock, err := lockDirectory(dir)
	if err != nil {
		t.Fatal("lock not released", err)
	}
	unlock()
}
