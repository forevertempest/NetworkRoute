//go:build !windows

package main

import (
	"fmt"
	"networkroute/internal/monitor"
	"os"
	"runtime"
)

func diagnostics(command string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("%s is unsupported on this OS", command)
	}
	interfaces, err := monitor.Interfaces()
	if err != nil {
		return err
	}
	files := map[string]string{}
	for _, path := range []string{"/proc/net/route", "/proc/net/ipv6_route", "/etc/resolv.conf"} {
		b, err := os.ReadFile(path)
		if err != nil {
			files[path] = err.Error()
		} else {
			files[path] = string(b)
		}
	}
	return output(map[string]any{"interfaces": interfaces, "network_files": files, "read_only": true})
}
