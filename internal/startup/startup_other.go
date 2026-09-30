//go:build !windows && !linux

package startup

import "fmt"

func Status(string) (State, error) {
	return State{}, fmt.Errorf("autostart supports Windows and Linux")
}
func Enable(string) error  { return fmt.Errorf("autostart supports Windows and Linux") }
func Disable(string) error { return fmt.Errorf("autostart supports Windows and Linux") }
