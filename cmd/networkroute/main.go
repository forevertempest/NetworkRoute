package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"networkroute/internal/config"
	"networkroute/internal/console"
	"networkroute/internal/engine"
	"networkroute/internal/identity"
	"networkroute/internal/monitor"
	"networkroute/internal/policy"
	"networkroute/internal/quality"
	"networkroute/internal/tunnel"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const version = "0.5.0"

func output(v any) error { e := json.NewEncoder(os.Stdout); e.SetIndent("", "  "); return e.Encode(v) }
func relayClient(c config.Config) (*tunnel.Client, error) {
	if c.Mode == "direct" || c.Relay.Address == "" {
		return nil, nil
	}
	cfg, err := identity.Config(c.Certificate, c.PrivateKey, []string{c.Relay.ServerPin}, false)
	if err != nil {
		return nil, err
	}
	return &tunnel.Client{Address: c.Relay.Address, TLS: cfg}, nil
}
func run() error {
	if len(os.Args) < 2 {
		return menu(nil)
	}
	command := os.Args[1]
	switch command {
	case "autostart", "local":
		return localCommand(command, os.Args[2:])
	case "menu":
		return menu(os.Args[2:])
	case "vpn-prepare", "vpn-pair":
		return vpnCommand(command, os.Args[2:])
	case "setup", "pair", "doctor":
		return setupCommand(command, os.Args[2:], os.Stdin)
	case "help", "--help", "-h":
		return help()
	case "version":
		fmt.Println(version)
		return nil
	case "apps":
		rows, err := monitor.Connections()
		if err != nil {
			return err
		}
		return output(rows)
	case "status":
		interfaces, err := monitor.Interfaces()
		if err != nil {
			return err
		}
		hints, hintErr := monitor.VPNHints()
		if hintErr != nil {
			return hintErr
		}
		return output(map[string]any{"version": version, "capture": "explicit SOCKS5 only", "system_routes_modified": false, "interfaces": interfaces, "vpn_hints": hints, "service": "not installed by this MVP"})
	case "diagnostics", "routes":
		return diagnostics(command)
	case "init":
		fs := flag.NewFlagSet("init", flag.ContinueOnError)
		dir := fs.String("dir", "secrets/client", "new identity directory")
		if err := fs.Parse(os.Args[2:]); err != nil {
			return err
		}
		pin, err := identity.Generate(*dir)
		if err != nil {
			return err
		}
		return output(map[string]string{"public_key_pin": pin, "directory": *dir})
	case "serve", "test", "relay":
		return configured(command)
	case "optimize", "optimize-all":
		return fmt.Errorf("transparent executable/system routing requires the planned signed WFP backend; this MVP supports explicitly configured SOCKS5 applications only (serve command)")
	default:
		return fmt.Errorf("unknown command %q; use networkroute help", command)
	}
}
func configured(command string) error {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	path := fs.String("config", "config.local.json", "config file")
	target := fs.String("target", "", "host:port to compare (test)")
	samples := fs.Int("samples", 8, "probes per route, 5..30")
	debug := fs.Bool("debug", false, "debug errors; may include destination metadata")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}
	if *debug {
		slog.SetLogLoggerLevel(slog.LevelDebug)
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
	}
	c, err := config.Load(*path)
	if err != nil {
		return err
	}
	relay, err := relayClient(c)
	if err != nil {
		return err
	}
	if relay != nil {
		defer relay.Close()
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if command == "relay" {
		if relay == nil {
			return fmt.Errorf("no optional relay configured; serve and test work without one")
		}
		start := time.Now()
		testCtx, stop := context.WithTimeout(ctx, 4*time.Second)
		defer stop()
		err := relay.Connect(testCtx)
		if err != nil {
			return err
		}
		return output(map[string]any{"address": c.Relay.Address, "authenticated": true, "cold_handshake_ms": time.Since(start).Milliseconds(), "note": "handshake duration is not destination RTT"})
	}
	if command == "test" {
		if *samples < 5 || *samples > 30 {
			return fmt.Errorf("samples must be 5..30")
		}
		resolveCtx, stop := context.WithTimeout(ctx, 3*time.Second)
		addrs, err := policy.Resolve(resolveCtx, net.DefaultResolver, *target)
		stop()
		if err != nil {
			return err
		}
		if relay == nil {
			report := quality.MeasureDirect(ctx, addrs, *samples, 2*time.Second, quality.NativeDial)
			return output(map[string]any{"benchmark": report, "relay_required": false, "verdict": "Existing OS routes only. Smart races available TCP endpoints; no ISP route improvement is implied."})
		}
		if err = relay.Connect(ctx); err != nil {
			return err
		}
		report := quality.Measure(ctx, addrs[0], *samples, 2*time.Second, quality.NativeDial, relay.DialTCP)
		verdict := "Direct retained: no demonstrated advantage"
		if report.Relay.Successes >= 5 && quality.Score(report.Relay, c.Profile)+5 < quality.Score(report.Direct, c.Profile) && quality.Score(report.Relay, c.Profile) < .85*quality.Score(report.Direct, c.Profile) {
			verdict = "Relay is a candidate; stability windows still required before automatic selection"
		}
		return output(map[string]any{"benchmark": report, "verdict": verdict, "profile": c.Profile, "note": "No packet RTT, UDP loss, throughput or TLS/TTFB claim. Same numeric destination on both paths."})
	}
	session, err := engine.Start(ctx, c)
	if err != nil {
		return err
	}
	defer func() {
		stopCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		session.Stop(stopCtx)
	}()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				slog.Info("metrics", "counters", session.Router.Metrics.Snapshot())
			}
		}
	}()
	slog.Info("explicit SOCKS5 proxy listening", "address", session.Address, "mode", c.Mode, "profile", c.Profile, "fail_mode", c.FailMode)
	<-session.Done
	return session.Err()
}
func menu(args []string) error {
	fs := flag.NewFlagSet("menu", flag.ContinueOnError)
	dir := fs.String("data-dir", "", "optional directory for portable settings (default LOCALAPPDATA/NetworkRoute)")
	start := fs.Bool("start", false, "start the proxy immediately, keeping the menu available")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return console.Launch(ctx, *dir, version, diagnostics, *start)
}
func help() error {
	fmt.Print(`NetworkRoute Optimizer 0.5.0 — standalone Windows / Linux release

NetworkRoute.exe                            Persistent interactive console menu
NetworkRoute.exe menu --data-dir PATH        Optional portable settings directory
networkroute menu --start                    Start proxy immediately and open menu
networkroute autostart enable|disable|status [--data-dir PATH]
  Login startup; installs a standalone copy under the data directory/app.
networkroute local settings|traffic|before|after [--data-dir PATH]
networkroute local status|apply|remove [--data-dir PATH]
  Windows outbound per-application limits; apply/remove require administrator.
  Configure rules and measurements in menu 10; startup in menu 11.
The menu includes offline manuals, settings, start/stop, diagnostics and VPS export.

networkroute setup                          Interactive setup (standalone / VPN / VPS)
networkroute setup --mode relay --directory state/connection
networkroute pair --directory state/connection --ticket relay-connection.json
networkroute doctor --config state/connection/config.local.json --check-relay
networkroute vpn-prepare --directory state/vpn
networkroute vpn-pair --directory state/vpn --ticket wireguard-connection.json
  WireGuard profiles only; see tools/setup-wireguard.md for activation/full tunnel.

networkroute status                         Interfaces and implementation status
networkroute apps                           Windows/Linux connection snapshot
networkroute diagnostics                    Read-only route/DNS/interface report
networkroute routes                         Existing OS routing table
networkroute init --dir secrets/client       Optional relay identity only
networkroute serve --config config.local.json
networkroute test --config config.local.json --target example.com:443 --samples 8
networkroute relay --config config.local.json   Optional relay handshake check

Selected apps must explicitly use the loopback SOCKS5 proxy. UDP requires client
SOCKS5 UDP ASSOCIATE support. Without a relay, Smart staggers TCP IPv4/IPv6
endpoints and retains the first connected socket. No VPS, account or key needed.
Smart UDP retains Direct. Existing OS/VPN routes and DNS are preserved.
See ARCHITECTURE.md and README.md for production roadmap and current limitations.
`)
	return nil
}
func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	if err := run(); err != nil {
		slog.Error("networkroute failed", "error", err)
		os.Exit(1)
	}
}
