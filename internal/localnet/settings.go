// Package localnet manages optional, explicitly configured local network controls.
package localnet

import (
	"encoding/json"
	"fmt"
	"networkroute/internal/onboarding"
	"networkroute/internal/storage"
	"os"
	"path/filepath"
	"strings"
)

type Rule struct {
	Application string `json:"application"`
	UploadKbps  int    `json:"upload_kbps"`
}

type Settings struct {
	Version       int    `json:"version"`
	StartOnLaunch bool   `json:"start_on_launch"`
	Target        string `json:"target"`
	Samples       int    `json:"samples"`
	TimeoutMS     int    `json:"timeout_ms"`
	IntervalMS    int    `json:"interval_ms"`
	Rules         []Rule `json:"upload_rules"`
}

func Defaults() Settings {
	return Settings{Version: 1, Target: "example.com:443", Samples: 10, TimeoutMS: 1500, IntervalMS: 250, Rules: []Rule{}}
}

func (s Settings) Validate() error {
	if s.Version != 1 {
		return fmt.Errorf("unsupported local settings version")
	}
	if err := onboarding.Endpoint(s.Target); err != nil { return err }
	if s.Samples < 5 || s.Samples > 60 || s.TimeoutMS < 100 || s.TimeoutMS > 5000 || s.IntervalMS < 50 || s.IntervalMS > 5000 {
		return fmt.Errorf("samples: 5..60; timeout: 100..5000 ms; interval: 50..5000 ms")
	}
	if len(s.Rules) > 32 {
		return fmt.Errorf("at most 32 upload rules")
	}
	seen := map[string]bool{}
	for _, r := range s.Rules {
		app := strings.ToLower(r.Application)
		if strings.TrimSpace(app) != app || !strings.HasSuffix(app, ".exe") || len(app) > 240 || strings.ContainsAny(app, "\x00\r\n*?\"<>|") {
			return fmt.Errorf("application must be an exact .exe name or path, without wildcards")
		}
		if r.UploadKbps < 8 || r.UploadKbps > 10000000 {
			return fmt.Errorf("upload limit: 8..10000000 kbit/s")
		}
		if seen[app] {
			return fmt.Errorf("duplicate application rule: %s", r.Application)
		}
		seen[app] = true
	}
	return nil
}

func Load(dir string) (Settings, error) {
	s := Defaults()
	err := onboarding.ReadJSON(filepath.Join(dir, "local.json"), &s)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	return s, s.Validate()
}

func Save(dir string, s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return storage.Write(filepath.Join(dir, "local.json"), append(b, '\n'), nil)
}
