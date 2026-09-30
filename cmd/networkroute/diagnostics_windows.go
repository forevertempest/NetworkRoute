//go:build windows

package main

import (
	"context"
	"fmt"
	"networkroute/internal/monitor"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func diagnostics(command string) error {
	if command == "diagnostics" {
		interfaces, err := monitor.Interfaces()
		if err != nil {
			return err
		}
		if err = output(interfaces); err != nil {
			return err
		}
	}
	// Read-only fixed commands; no user text is interpolated into a shell.
	root := os.Getenv("SystemRoot")
	if root == "" {
		return fmt.Errorf("SystemRoot unavailable")
	}
	commands := [][]string{{"route.exe", "print"}}
	if command == "diagnostics" {
		commands = append(commands, []string{"ipconfig.exe", "/all"}, []string{"netsh.exe", "winhttp", "show", "proxy"})
	}
	for _, args := range commands {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		cmd := exec.CommandContext(ctx, filepath.Join(root, "System32", args[0]), args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		cancel()
		if err != nil {
			return fmt.Errorf("%s: %w", args[0], err)
		}
	}
	return nil
}
