//go:build linux

package monitor

import (
	"net"
	"os"
	"testing"
)

func TestProcAddressAndRealOwnership(t *testing.T) {
	for input, want := range map[string]string{"0100007F:1F90": "127.0.0.1:8080", "00000000000000000000000001000000:01BB": "[::1]:443"} {
		got, err := procAddress(input)
		if err != nil || got != want {
			t.Fatalf("%s -> %s (%v)", input, got, err)
		}
	}
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	rows, err := Connections()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Local == l.Addr().String() && r.PID == uint32(os.Getpid()) {
			return
		}
	}
	t.Fatal("own socket not attributed")
}
