//go:build !windows

package console

import "os"

func terminal() (bool, func()) {
	stat, err := os.Stdout.Stat()
	return err == nil && stat.Mode()&os.ModeCharDevice != 0 && os.Getenv("TERM") != "dumb", func() {}
}
