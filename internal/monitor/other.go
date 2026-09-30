//go:build !windows && !linux

package monitor

import "fmt"

func Connections() ([]Connection, error) {
	return nil, fmt.Errorf("connection ownership uses Windows IP Helper and is available only on Windows")
}
