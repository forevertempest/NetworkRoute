package onboarding

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWireGuardProfileExchange(t *testing.T) {
	for _, full := range []bool{false, true} {
		client, server := t.TempDir(), t.TempDir()
		request, err := PrepareVPN(client)
		if err != nil {
			t.Fatal(err)
		}
		ticket, err := SetupVPNServer(server, "vpn.example.com:51820", request, "eth0", "1.1.1.1", full)
		if err != nil {
			t.Fatal(err)
		}
		if full {
			if _, err = PairVPN(client, ticket, false); err == nil {
				t.Fatal("full tunnel silently accepted")
			}
		}
		path, err := PairVPN(client, ticket, full)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(path)
		if !strings.Contains(string(data), "PersistentKeepalive = 25") {
			t.Fatal("missing NAT keepalive")
		}
		if !full && strings.Contains(string(data), "0.0.0.0/0") {
			t.Fatal("split setup stole default route")
		}
		public, _ := os.ReadFile(ticket)
		if strings.Contains(string(public), "PrivateKey") {
			t.Fatal("private key exported")
		}
		if _, err = PairVPN(client, ticket, full); err != nil {
			t.Fatalf("same profile cannot be reused: %v", err)
		}
		if _, err = SetupVPNServer(server, "other.example.com:51820", request, "eth0", "1.1.1.1", full); err == nil {
			t.Fatal("changed endpoint silently overwrote installed profile")
		}
	}
}
func TestWireGuardKeyMatchesOfficialToolWhenAvailable(t *testing.T) {
	path, err := exec.LookPath("wg")
	if err != nil {
		t.Skip("official wg tool not installed; CI validates it")
	}
	private, public, err := wgIdentity(filepath.Join(t.TempDir(), "test.key"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(path, "pubkey")
	cmd.Stdin = strings.NewReader(private + "\n")
	b, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(b)) != public {
		t.Fatal("WireGuard public key mismatch")
	}
}
func TestWireGuardRejectsInjectionAndWrongClient(t *testing.T) {
	client, server := t.TempDir(), t.TempDir()
	request, err := PrepareVPN(client)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = SetupVPNServer(server, "vpn.example:51820", request, "eth0;evil", "1.1.1.1", true); err == nil {
		t.Fatal("shell injection accepted")
	}
	ticket, err := SetupVPNServer(server, "vpn.example:51820", request, "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	PrepareVPN(other)
	if _, err = PairVPN(other, ticket, false); err == nil {
		t.Fatal("wrong client accepted")
	}
	if validateWGPublic(base64.StdEncoding.EncodeToString(make([]byte, 32))) == nil {
		t.Fatal("zero key accepted")
	}
	var exported WGRequest
	if err = ReadJSON(request, &exported); err != nil {
		t.Fatal(err)
	}
	if validateWGPublic(exported.ClientPublicKey+"\n") == nil {
		t.Fatal("non-canonical multiline key accepted")
	}
}
