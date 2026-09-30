//go:build !windows && !linux

package monitor

func routeFingerprint() (string, error) { return "", nil }
