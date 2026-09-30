package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"networkroute/internal/onboarding"
	"os"
	"strings"
)

func setupCommand(command string, args []string, input io.Reader) error {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	dir := fs.String("directory", "state/connection", "local setup directory (existing different files are never overwritten)")
	mode := fs.String("mode", "", "standalone, vpn (existing VPN), or relay")
	ticket := fs.String("ticket", "", "public relay-connection.json from trusted VPS")
	vpnPolicy := fs.String("vpn-policy", "respect", "respect existing VPN or explicitly permit nested relay")
	path := fs.String("config", "config.local.json", "config file to check")
	check := fs.Bool("check-relay", false, "actively authenticate the configured relay")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if command == "doctor" {
		report := onboarding.Doctor(context.Background(), *path, *check)
		if err := output(report); err != nil {
			return err
		}
		if report.Status == "failed" {
			return fmt.Errorf("setup checks failed; see the named checks above")
		}
		return nil
	}
	if command == "pair" {
		p, err := onboarding.Pair(*dir, *ticket, *vpnPolicy)
		if err != nil {
			return err
		}
		return output(map[string]string{"config": p, "next": "Run doctor --config <config> --check-relay, then serve --config <config>"})
	}
	if *mode == "" {
		fmt.Fprintln(os.Stderr, "NetworkRoute setup\n1. Standalone (free, no server)\n2. Use my existing VPN (keep its OS routes)\n3. Connect my VPS relay\nFor a new WireGuard VPN see tools/setup-wireguard.md.\nChoose 1, 2 or 3:")
		line, err := bufio.NewReader(input).ReadString('\n')
		if err != nil && !(err == io.EOF && len(line) > 0) {
			return fmt.Errorf("read selection: %w", err)
		}
		switch strings.TrimSpace(line) {
		case "1":
			*mode = "standalone"
		case "2":
			*mode = "vpn"
		case "3":
			*mode = "relay"
		default:
			return fmt.Errorf("invalid choice; no files changed")
		}
	}
	p, err := onboarding.SetupClient(*dir, *mode)
	if err != nil {
		return err
	}
	next := "Run doctor --config <config>, then serve --config <config>"
	if *mode == "relay" {
		next = "Copy only client-request.json to VPS. Run nr-relay setup there, then import relay-connection.json using networkroute pair. Private keys stay on each machine."
	}
	return output(map[string]string{"mode": *mode, "created": p, "next": next})
}
