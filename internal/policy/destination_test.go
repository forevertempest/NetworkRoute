package policy

import (
	"net/netip"
	"testing"
)

func TestPublic(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "::1", "::ffff:127.0.0.1", "10.1.2.3", "169.254.169.254", "100.64.0.1", "ff02::1", "2001:db8::1", "64:ff9b::7f00:1"} {
		if Public(netip.MustParseAddr(s)) {
			t.Fatalf("unsafe public classification: %s", s)
		}
	}
	for _, s := range []string{"1.1.1.1", "2606:4700:4700::1111"} {
		if !Public(netip.MustParseAddr(s)) {
			t.Fatal(s)
		}
	}
}
func TestDomainBoundary(t *testing.T) {
	if !DomainMatch("api.example.com.", "example.com") || DomainMatch("notexample.com", "example.com") {
		t.Fatal("domain boundary")
	}
}
