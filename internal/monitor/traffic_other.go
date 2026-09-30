//go:build !windows && !linux

package monitor

import "fmt"

func TrafficSnapshot() ([]Traffic, error) {
	return nil, fmt.Errorf("traffic counters support Windows and Linux")
}
