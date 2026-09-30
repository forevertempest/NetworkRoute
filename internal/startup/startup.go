// Package startup installs a self-contained per-user copy for login startup.
package startup

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type State struct {
	Enabled bool   `json:"enabled"`
	Command string `json:"command,omitempty"`
	When    string `json:"when"`
}

func key(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		abs = strings.ToLower(abs)
	}
	h := sha256.Sum256([]byte(filepath.Clean(abs)))
	return fmt.Sprintf("NetworkRoute-%x", h[:8]), nil
}

// Copy the running executable out of the source/download folder. No sidecar DLLs
// or scripts are needed; settings intentionally remain separate writable data.
func installCopy(dir string) (string, error) {
	source, err := os.Executable()
	if err != nil {
		return "", err
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	name := "networkroute"
	if runtime.GOOS == "windows" {
		name = "NetworkRoute.exe"
	}
	dest := filepath.Join(abs, "app", name)
	if source == dest || (runtime.GOOS == "windows" && strings.EqualFold(source, dest)) {
		return dest, nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return "", err
	}
	in, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.CreateTemp(filepath.Dir(dest), ".install-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(out.Name())
	if err = out.Chmod(0700); err == nil {
		_, err = io.Copy(out, in)
	}
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err = os.Rename(out.Name(), dest); err != nil {
		return "", fmt.Errorf("install standalone copy (close the installed instance before updating): %w", err)
	}
	return dest, nil
}
