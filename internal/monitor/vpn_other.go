//go:build !windows && !linux

package monitor

func VPNHints() ([]string, error) { return nil, nil }
