//go:build windows

package startup

import (
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"strings"
	"testing"
	"time"
)

func TestStartupRegistryIsolationAndQuoting(t *testing.T) {
	// This never touches the real Run key or schedules a process.
	path := `Software\NetworkRoute\Tests\` + time.Now().Format("20060102150405.000000000")
	dir := `C:\Users\Test User\Данные\`
	exe := `C:\Users\Test User\app\NetworkRoute.exe`
	defer registry.DeleteKey(registry.CURRENT_USER, path)
	if err := enableAt(dir, exe, path); err != nil {
		t.Fatal(err)
	}
	s, err := statusAt(dir, path)
	if err != nil || !s.Enabled {
		t.Fatalf("missing registration: %v", err)
	}
	args, err := windows.DecomposeCommandLine(s.Command)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 5 || args[0] != exe || args[1] != "menu" || args[2] != "--start" || args[4] != dir {
		t.Fatalf("bad quoting: %#v", args)
	}
	if err := disableAt(dir, path); err != nil {
		t.Fatal(err)
	}
	s, err = statusAt(dir, path)
	if err != nil || s.Enabled {
		t.Fatalf("not disabled: %v", err)
	}
	if _, err := command(exe, strings.Repeat("x", 300)); err == nil {
		t.Fatal("accepted too-long Run command")
	}
}
