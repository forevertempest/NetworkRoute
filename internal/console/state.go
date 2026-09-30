package console

import (
	"encoding/json"
	"fmt"
	"networkroute/internal/config"
	"networkroute/internal/onboarding"
	"networkroute/internal/storage"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

func DefaultDirectory() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		var err error
		base, err = os.UserConfigDir()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(base, "NetworkRoute"), nil
}

func (a *App) loadState() error {
	if err := os.MkdirAll(a.dir, 0700); err != nil {
		return err
	}
	a.configPath = filepath.Join(a.dir, "profiles", "default", "config.local.json")
	state, err := os.ReadFile(filepath.Join(a.dir, "active.json"))
	if err == nil {
		var relative string
		if err = json.Unmarshal(state, &relative); err != nil {
			return fmt.Errorf("active.json: %w", err)
		}
		if !filepath.IsLocal(relative) {
			return fmt.Errorf("invalid active profile path")
		}
		a.configPath = filepath.Join(a.dir, relative)
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err = os.Stat(a.configPath); os.IsNotExist(err) {
		if state != nil {
			return fmt.Errorf("active profile missing: %s", a.configPath)
		}
		_, err = onboarding.SetupClient(filepath.Dir(a.configPath), "standalone")
	}
	if err != nil {
		return err
	}
	_, err = config.Load(a.configPath)
	return err
}

func (a *App) activate(path string) error {
	if _, err := config.Load(path); err != nil {
		return err
	}
	relative, err := filepath.Rel(a.dir, path)
	if err != nil || !filepath.IsLocal(relative) {
		return fmt.Errorf("profile must be inside the application data directory")
	}
	b, err := json.Marshal(relative)
	if err != nil {
		return err
	}
	if err = storage.Write(filepath.Join(a.dir, "active.json"), b, nil); err != nil {
		return err
	}
	a.configPath = path
	return nil
}

func saveConfig(path string, c config.Config) error {
	for _, p := range []*string{&c.Certificate, &c.PrivateKey} {
		if *p != "" {
			if rel, err := filepath.Rel(filepath.Dir(path), *p); err == nil && filepath.IsLocal(rel) {
				*p = rel
			}
		}
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return storage.Write(path, append(b, '\n'), func(temp string) error { _, err := config.Load(temp); return err })
}

type logBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	if len(b.data) > 32768 {
		b.data = b.data[len(b.data)-32768:]
	}
	return len(p), nil
}
func (b *logBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.data) }

// Never allow metadata from processes/config/network to inject terminal escapes.
func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || (r >= 0x80 && r <= 0x9f) {
			return -1
		}
		return r
	}, s)
}
