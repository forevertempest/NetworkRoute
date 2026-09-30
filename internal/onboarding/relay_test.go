package onboarding

import (
	"context"
	"encoding/json"
	"net"
	"networkroute/internal/config"
	"networkroute/internal/identity"
	"networkroute/internal/tunnel"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPairingWorkflowAuthenticates(t *testing.T) {
	clientDir, serverDir := t.TempDir(), t.TempDir()
	request, err := SetupClient(clientDir, "relay")
	if err != nil {
		t.Fatal(err)
	}
	// Reserve a port only to create the ticket; relay later binds it for a real handshake.
	port, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := port.LocalAddr().String()
	port.Close()
	ticket, err := SetupRelay(serverDir, address, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = SetupRelay(serverDir, address, request); err != nil {
		t.Fatalf("idempotent setup: %v", err)
	}
	path, err := Pair(clientDir, ticket, "respect")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Pair(clientDir, ticket, "respect"); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil || cfg.AllowNestedTunnel {
		t.Fatalf("config: %v", err)
	}
	sc, err := LoadRelay(filepath.Join(serverDir, "relay.json"))
	if err != nil {
		t.Fatal(err)
	}
	tlsConfig, err := identity.Config(sc.Certificate, sc.PrivateKey, sc.ClientPins, true)
	if err != nil {
		t.Fatal(err)
	}
	server := &tunnel.Server{TLS: tlsConfig}
	listener, err := server.Listen(address)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, listener) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(time.Second):
			t.Error("relay did not stop")
		}
	}()
	report := Doctor(ctx, path, true)
	verified := false
	for _, check := range report.Checks {
		if check.Name == "relay" && check.Status == "ok" {
			verified = true
		}
	}
	if !verified {
		t.Fatalf("no authenticated relay check: %+v", report)
	}
	// A reachable server with the wrong pinned key must not pass the doctor.
	cfg.Relay.ServerPin = strings.Repeat("0", 64)
	badConfig := filepath.Join(t.TempDir(), "wrong-pin.json")
	if err = writeJSON(badConfig, cfg); err != nil {
		t.Fatal(err)
	}
	deadline, stop := context.WithTimeout(ctx, time.Second)
	defer stop()
	if report = Doctor(deadline, badConfig, true); report.Status != "failed" {
		t.Fatalf("wrong pin reported ready: %+v", report)
	}
}
func TestSetupPreservesExistingAndRejectsTampering(t *testing.T) {
	dir := t.TempDir()
	path, err := SetupClient(dir, "standalone")
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	if _, err = SetupClient(dir, "vpn"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("existing config changed")
	}
	bad := filepath.Join(t.TempDir(), "ticket.json")
	os.WriteFile(bad, []byte(`{"version":1,"type":"networkroute-relay","address":"example.com:7443","server_pin":"`+strings.Repeat("a", 64)+`","private_key":"injected"}`), 0600)
	if _, err = Pair(dir, bad, "nested"); err == nil {
		t.Fatal("unknown ticket fields accepted")
	}
	for _, endpoint := range []string{"host:0", "host:+7443", "host:99999", "example.com;cmd:7443", "https://host:7443", "[::]:7443"} {
		if Endpoint(endpoint) == nil {
			t.Fatal(endpoint)
		}
	}
}

func TestRejectOversizedAndTrailingTicket(t *testing.T) {
	for _, content := range []string{strings.Repeat(" ", 65537), `{"version":1} {"version":2}`} {
		path := filepath.Join(t.TempDir(), "request.json")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		var request ClientRequest
		if ReadJSON(path, &request) == nil {
			t.Fatal("malformed public request accepted")
		}
	}
}
func TestExportHasNoPrivateIdentity(t *testing.T) {
	dir := t.TempDir()
	path, err := SetupClient(dir, "relay")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	var v map[string]any
	if err = json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	if len(v) != 3 || strings.Contains(string(b), "PRIVATE") {
		t.Fatal("unexpected exported fields")
	}
}
