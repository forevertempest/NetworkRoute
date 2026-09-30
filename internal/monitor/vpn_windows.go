//go:build windows

package monitor

import (
	"fmt"
	"golang.org/x/sys/windows"
	"strings"
	"unsafe"
)

// VPNHints is deliberately conservative and advisory, not a proof of VPN absence.
func VPNHints() ([]string, error) {
	size := uint32(16384)
	for tries := 0; tries < 4; tries++ {
		if size > 16<<20 {
			return nil, fmt.Errorf("adapter table too large")
		}
		b := make([]byte, size)
		head := (*windows.IpAdapterAddresses)(unsafe.Pointer(&b[0]))
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, 0, 0, head, &size)
		if err == windows.ERROR_BUFFER_OVERFLOW {
			continue
		}
		if err != nil {
			return nil, err
		}
		var hints []string
		for a := head; a != nil; a = a.Next {
			if a.OperStatus != 1 {
				continue
			}
			description := windows.UTF16PtrToString(a.Description)
			name := windows.UTF16PtrToString(a.FriendlyName)
			text := strings.ToLower(description + " " + name)
			suspect := a.IfType == 23 || a.IfType == 131
			for _, word := range []string{"wireguard", "wintun", "openvpn", "tailscale", "warp", "tap-windows", "radmin vpn", "zerotier", "hamachi"} {
				if strings.Contains(text, word) {
					suspect = true
				}
			}
			if suspect {
				hints = append(hints, name+": "+description)
			}
		}
		return hints, nil
	}
	return nil, fmt.Errorf("adapter table kept changing")
}
