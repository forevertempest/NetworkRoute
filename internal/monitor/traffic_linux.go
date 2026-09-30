//go:build linux

package monitor

import (
	"os"
	"strconv"
	"strings"
)

func TrafficSnapshot() ([]Traffic, error) {
	b, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return nil, err
	}
	var result []Traffic
	for _, line := range strings.Split(string(b), "\n") {
		name, values, ok := strings.Cut(line, ":")
		fields := strings.Fields(values)
		if !ok || len(fields) < 16 {
			continue
		}
		rx, e1 := strconv.ParseUint(fields[0], 10, 64)
		tx, e2 := strconv.ParseUint(fields[8], 10, 64)
		if e1 != nil || e2 != nil {
			continue
		}
		result = append(result, Traffic{Name: strings.TrimSpace(name), Received: rx, Sent: tx})
	}
	return result, nil
}
