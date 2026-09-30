//go:build linux

package startup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopAutostartIsIndependentAndReversible(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := filepath.Join(t.TempDir(), "data with spaces")
	if err := Enable(dir); err != nil {
		t.Fatal(err)
	}
	s, err := Status(dir)
	if err != nil || !s.Enabled {
		t.Fatalf("no entry: %v", err)
	}
	b, err := os.ReadFile(s.Command)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), filepath.Join(dir, "app", "networkroute")) || !strings.Contains(string(b), "--start") {
		t.Fatal("entry does not use installed copy")
	}
	if info, err := os.Stat(filepath.Join(dir, "app", "networkroute")); err != nil || info.Mode().Perm()&0100 == 0 {
		t.Fatal("copy not executable")
	}
	if err := Disable(dir); err != nil {
		t.Fatal(err)
	}
	s, err = Status(dir)
	if err != nil || s.Enabled {
		t.Fatal("entry still enabled")
	}
}
