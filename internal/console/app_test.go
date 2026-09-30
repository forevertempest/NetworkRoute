package console

import (
	"bytes"
	"context"
	"io"
	"networkroute/internal/config"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type observedOutput struct {
	logBuffer
	once  sync.Once
	ready chan struct{}
}

func (o *observedOutput) Write(p []byte) (int, error) {
	n, err := o.logBuffer.Write(p)
	if strings.Contains(string(p), "Выберите пункт") {
		o.once.Do(func() { close(o.ready) })
	}
	return n, err
}

func TestMenuRemainsOpenUntilUserExits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	output := &observedOutput{ready: make(chan struct{})}
	app := New(ctx, t.TempDir(), "test", input, output)
	done := make(chan error, 1)
	go func() { done <- app.Run() }()
	select {
	case <-output.ready:
	case <-time.After(3 * time.Second):
		t.Fatal("menu not shown")
	}
	select {
	case err := <-done:
		t.Fatalf("closed without input: %v", err)
	case <-time.After(80 * time.Millisecond):
	}
	if _, err := io.WriteString(writer, "0\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("menu did not exit")
	}
	if !strings.Contains(output.String(), "VPN и собственный VPS") {
		t.Fatal("missing console actions")
	}
}

func TestOfflineGuideAndSettingsPersist(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var output bytes.Buffer
	// Set port, return, open guide, leave pagination, return, exit.
	app := New(ctx, dir, "test", strings.NewReader("2\n3\n10999\n\n7\n1\n0\n\n0\n"), &output)
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(filepath.Join(dir, "profiles", "default", "config.local.json"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != "127.0.0.1:10999" {
		t.Fatal("port not saved")
	}
	if !strings.Contains(output.String(), "Первый запуск") {
		t.Fatal("embedded guide not displayed")
	}
	before, err := os.ReadFile(app.configPath)
	if err != nil {
		t.Fatal(err)
	}
	c.Listen = "0.0.0.0:10999"
	if err = saveConfig(app.configPath, c); err == nil {
		t.Fatal("unsafe listener accepted")
	}
	after, _ := os.ReadFile(app.configPath)
	if !bytes.Equal(before, after) {
		t.Fatal("invalid update replaced valid config")
	}
}

func TestMetadataCannotInjectTerminalControl(t *testing.T) {
	got := safe("name\x1b[2J\r\n\u009bX")
	if strings.ContainsAny(got, "\x1b\r\n\u009b") {
		t.Fatal("terminal control retained")
	}
}
