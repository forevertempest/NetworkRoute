//go:build windows

package startup

import (
	"fmt"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

func command(exe, dir string) (string, error) {
	if strings.ContainsAny(exe+dir, "\x00\r\n") {
		return "", fmt.Errorf("invalid startup path")
	}
	cmd := windows.EscapeArg(exe) + " menu --start --data-dir " + windows.EscapeArg(dir)
	if len(utf16.Encode([]rune(cmd))) > 260 {
		return "", fmt.Errorf("startup command exceeds Windows Run limit; choose a shorter data directory")
	}
	return cmd, nil
}

func Status(dir string) (State, error) { return statusAt(dir, runKey) }
func statusAt(dir, path string) (State, error) {
	s := State{When: "при входе текущего пользователя в Windows"}
	name, err := key(dir)
	if err != nil {
		return s, err
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err == registry.ErrNotExist {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	defer k.Close()
	s.Command, _, err = k.GetStringValue(name)
	if err == registry.ErrNotExist {
		return s, nil
	}
	s.Enabled = err == nil && s.Command != ""
	return s, err
}

func Enable(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	// Validate before copying or editing the registry.
	if _, err := command(filepath.Join(abs, "app", "NetworkRoute.exe"), abs); err != nil {
		return err
	}
	exe, err := installCopy(abs)
	if err != nil {
		return err
	}
	return enableAt(abs, exe, runKey)
}
func enableAt(dir, exe, path string) error {
	name, err := key(dir)
	if err != nil {
		return err
	}
	cmd, err := command(exe, dir)
	if err != nil {
		return err
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(name, cmd)
}
func Disable(dir string) error { return disableAt(dir, runKey) }
func disableAt(dir, path string) error {
	name, err := key(dir)
	if err != nil {
		return err
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err == registry.ErrNotExist {
		return nil
	}
	if err != nil {
		return err
	}
	defer k.Close()
	err = k.DeleteValue(name)
	if err == registry.ErrNotExist {
		return nil
	}
	return err
}
