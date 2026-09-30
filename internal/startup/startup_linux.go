//go:build linux

package startup

import (
	"fmt"
	"networkroute/internal/storage"
	"os"
	"path/filepath"
	"strings"
)

func desktopQuote(value string) (string, error) {
	if strings.ContainsAny(value, "\x00\r\n") {
		return "", fmt.Errorf("invalid startup path")
	}
	// Desktop Entry Exec quoting has two escaping layers, not shell quoting.
	value = strings.NewReplacer("\\", "\\\\\\\\", "\"", "\\\\\"", "`", "\\\\`", "$", "\\\\$", "%", "%%").Replace(value)
	return "\"" + value + "\"", nil
}
func desktopPath(dir string) (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	name, err := key(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "autostart", name+".desktop"), nil
}
func Status(dir string) (State, error) {
	s := State{When: "при входе в графическую сессию Linux (XDG Autostart)"}
	path, err := desktopPath(dir)
	if err != nil {
		return s, err
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	s.Enabled = strings.Contains(string(b), "X-NetworkRoute-Owned=true\n") && !strings.Contains(string(b), "Hidden=true")
	s.Command = path
	return s, nil
}
func Enable(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if strings.Contains(abs, "=") {
		return fmt.Errorf("XDG Exec executable paths cannot contain '='; choose another data directory")
	}
	qdir, err := desktopQuote(abs)
	if err != nil {
		return err
	}
	exe, err := installCopy(abs)
	if err != nil {
		return err
	}
	qexe, err := desktopQuote(exe)
	if err != nil {
		return err
	}
	path, err := desktopPath(dir)
	if err != nil {
		return err
	}
	if b, e := os.ReadFile(path); e == nil && !strings.Contains(string(b), "X-NetworkRoute-Owned=true\n") {
		return fmt.Errorf("refusing to replace unrelated desktop entry")
	} else if e != nil && !os.IsNotExist(e) {
		return e
	}
	b := "[Desktop Entry]\nType=Application\nName=NetworkRoute\nTerminal=true\nX-NetworkRoute-Owned=true\nExec=" + qexe + " menu --start --data-dir " + qdir + "\n"
	return storage.Write(path, []byte(b), nil)
}
func Disable(dir string) error {
	path, err := desktopPath(dir)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !strings.Contains(string(b), "X-NetworkRoute-Owned=true\n") {
		return fmt.Errorf("refusing to delete unrelated desktop entry")
	}
	return os.Remove(path)
}
