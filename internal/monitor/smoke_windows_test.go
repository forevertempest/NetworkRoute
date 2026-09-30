//go:build windows

package monitor

import (
	"net"
	"os"
	"testing"
)

func TestLiveWindowsOwnership(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
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
			if r.Path == "" {
				t.Fatal("own process path unavailable")
			}
			return
		}
	}
	t.Fatal("IP Helper did not report own listening socket")
}
func TestLiveWindowsNetworkSnapshot(t *testing.T) {
	if _, err := Fingerprint(); err != nil {
		t.Fatal(err)
	}
	if _, err := VPNHints(); err != nil {
		t.Fatal(err)
	}
}
