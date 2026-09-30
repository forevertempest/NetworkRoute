// Package onboarding builds reviewable local configuration without changing OS networking.
package onboarding

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/netip"
	"networkroute/internal/identity"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func ReadJSON(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil {
		return err
	}
	if len(data) > 65536 {
		return fmt.Errorf("configuration exceeds 64 KiB")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(v); err != nil {
		return err
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("unexpected trailing JSON")
	}
	return nil
}
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(path, append(b, '\n'))
}
func writeFile(path string, b []byte) error {
	if old, err := os.ReadFile(path); err == nil {
		if bytes.Equal(old, b) {
			return nil
		}
		return fmt.Errorf("%s already exists with different content; choose a new setup directory", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return identity.WritePrivateFile(path, b)
}
func validatePin(pin string) error {
	b, err := hex.DecodeString(pin)
	if err != nil || len(b) != 32 {
		return fmt.Errorf("public-key pin must contain 64 hex characters")
	}
	return nil
}
func Endpoint(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("endpoint must be host:port (IPv6 uses [address]:port): %w", err)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("endpoint port must be 1..65535")
	}
	for _, c := range port {
		if c < '0' || c > '9' {
			return fmt.Errorf("endpoint port must contain decimal digits only")
		}
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.Zone() != "" || ip.IsUnspecified() || ip.IsMulticast() {
			return fmt.Errorf("invalid endpoint IP")
		}
		return nil
	}
	if len(host) == 0 || len(host) > 253 {
		return fmt.Errorf("invalid endpoint hostname")
	}
	for _, label := range strings.Split(strings.TrimSuffix(host, "."), ".") {
		if len(label) == 0 || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return fmt.Errorf("invalid endpoint hostname")
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return fmt.Errorf("endpoint hostname must be ASCII DNS or IP")
			}
		}
	}
	return nil
}
