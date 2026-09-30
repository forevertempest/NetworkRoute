package localnet

import (
	"context"
	"net"
	"networkroute/internal/quality"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsRejectInvalidUpdate(t *testing.T) {
	dir := t.TempDir()
	s := Defaults()
	s.Rules = []Rule{{Application: "backup.exe", UploadKbps: 500}}
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}
	s.Rules[0].Application = "*.exe"
	if err := Save(dir, s); err == nil {
		t.Fatal("wildcard accepted")
	}
	s, err := Load(dir)
	if err != nil || s.Rules[0].Application != "backup.exe" {
		t.Fatalf("valid settings lost: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "local.json"))
	if err != nil {
		t.Fatal(err)
	}
	b = []byte(strings.Replace(string(b), `"version": 1`, `"version": 1, "unknown": true`, 1))
	if err := os.WriteFile(filepath.Join(dir, "local.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("unknown settings accepted")
	}
}

func TestBaselineRoundtripAndComparableConditions(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	s := Defaults()
	s.Target = l.Addr().String()
	s.Samples = 5
	s.IntervalMS = 50
	r, err := Measure(context.Background(), s, "")
	if err != nil {
		t.Fatal(err)
	}
	if r.Stats.Successes != 5 {
		t.Fatalf("real TCP probes failed: %+v", r)
	}
	dir := t.TempDir()
	if err := SaveReport(dir, "before", r); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadBaseline(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Endpoint != r.Endpoint || loaded.Stats.Successes != 5 {
		t.Fatal("report did not round-trip")
	}
	if strings.Contains(Compare(loaded, loaded), "быстрее") {
		t.Fatal("claimed improvement without evidence")
	}
	changed := loaded
	changed.Network = "other"
	if !strings.Contains(Compare(loaded, changed), "Несопоставимые") {
		t.Fatal("compared different network paths")
	}
	changed = loaded
	changed.Stats = quality.Stats{Samples: 5, Successes: 5, Median: 900, P95: 999}
	if !strings.Contains(Compare(loaded, changed), "ухудшение") {
		t.Fatal("missed regression")
	}
}
