package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStandaloneSmartNeedsNoServerOrIdentity(t *testing.T) {
	for _, mode := range []string{"smart", "relay"} {
		path := filepath.Join(t.TempDir(), "config.json")
		data := `{"version":1,"mode":"` + mode + `","profile":"AUTO","fail_mode":"open"}`
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := Load(path)
		if (err == nil) != (mode == "smart") {
			t.Fatalf("mode %s: %v", mode, err)
		}
	}
}
func TestStandaloneFailClosedIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"version":1,"mode":"smart","profile":"AUTO","fail_mode":"closed"}`), 0600)
	if _, err := Load(path); err == nil {
		t.Fatal("standalone fail-closed silently accepted")
	}
}
