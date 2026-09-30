package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigSafety(t *testing.T) {
	valid := `{"version":1,"listen":"127.0.0.1:1080","mode":"direct","profile":"AUTO","fail_mode":"open"}`
	cases := []struct {
		data string
		ok   bool
	}{{valid, true}, {strings.Replace(valid, "127.0.0.1", "0.0.0.0", 1), false}, {strings.Replace(valid, "direct", "system", 1), false}, {valid + `{}`, false}, {strings.Replace(valid, `"version":1`, `"version":1,"typo":true`, 1), false}}
	for _, tc := range cases {
		p := filepath.Join(t.TempDir(), "config.json")
		os.WriteFile(p, []byte(tc.data), 0600)
		_, err := Load(p)
		if (err == nil) != tc.ok {
			t.Fatalf("config accepted=%v expected=%v: %v", err == nil, tc.ok, err)
		}
	}
}
