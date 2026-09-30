package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"networkroute/internal/identity"
	"networkroute/internal/onboarding"
	"networkroute/internal/tunnel"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func run() error {
	if len(os.Args) > 1 && os.Args[1] == "vpn-setup" {
		fs := flag.NewFlagSet("vpn-setup", flag.ContinueOnError)
		dir := fs.String("directory", "vpn-bundle", "WireGuard server bundle")
		endpoint := fs.String("endpoint", "", "client-reachable host:WireGuard-UDP-port")
		request := fs.String("client-request", "client-wireguard.json", "public Windows request")
		egress := fs.String("egress", "", "VPS uplink interface (full tunnel only)")
		dns := fs.String("dns", "", "IPv4 DNS resolver (full tunnel only)")
		full := fs.Bool("full-tunnel", false, "IPv4 Internet VPN with IPv6 blocked on Windows; default is VPS-only split tunnel")
		if err := fs.Parse(os.Args[2:]); err != nil {
			return err
		}
		path, err := onboarding.SetupVPNServer(*dir, *endpoint, *request, *egress, *dns, *full)
		if err != nil {
			return err
		}
		fmt.Println("Public WireGuard connection file:", path)
		fmt.Println("Generated profiles only. Review tools/setup-wireguard.md before installing/activating.")
		return nil
	}
	if len(os.Args) > 1 && os.Args[1] == "setup" {
		fs := flag.NewFlagSet("setup", flag.ContinueOnError)
		dir := fs.String("directory", "relay-state", "server setup directory")
		address := fs.String("public-address", "", "client-reachable host:UDP-port")
		request := fs.String("client-request", "client-request.json", "public request from Windows")
		if err := fs.Parse(os.Args[2:]); err != nil {
			return err
		}
		path, err := onboarding.SetupRelay(*dir, *address, *request)
		if err != nil {
			return err
		}
		fmt.Println("Public connection file:", path)
		fmt.Println("Start relay with --config", *dir+"/relay.json")
		return nil
	}
	configPath := flag.String("config", "", "generated relay.json; cannot be combined with individual server flags")
	listen := flag.String("listen", ":7443", "QUIC UDP listen address")
	cert := flag.String("cert", "", "relay certificate PEM")
	key := flag.String("key", "", "relay private key PEM")
	clients := flag.String("clients", "", "comma-separated authorized client SPKI SHA256 pins")
	initDir := flag.String("init-identity", "", "generate a new identity and print its public pin, then exit")
	maxClients := flag.Int("max-clients", 32, "maximum simultaneous authenticated clients")
	flag.Parse()
	if *configPath != "" {
		conflict := false
		flag.Visit(func(f *flag.Flag) {
			if f.Name != "config" {
				conflict = true
			}
		})
		if conflict {
			return fmt.Errorf("use --config alone; edit a reviewed relay config for custom settings")
		}
		c, err := onboarding.LoadRelay(*configPath)
		if err != nil {
			return err
		}
		*listen = c.Listen
		*cert = c.Certificate
		*key = c.PrivateKey
		*clients = strings.Join(c.ClientPins, ",")
		*maxClients = c.MaxClients
	}
	if *initDir != "" {
		pin, err := identity.Generate(*initDir)
		if err != nil {
			return err
		}
		fmt.Println(pin)
		return nil
	}
	if *maxClients < 1 || *maxClients > 256 {
		return fmt.Errorf("max-clients must be 1..256")
	}
	cfg, err := identity.Config(*cert, *key, strings.Split(*clients, ","), true)
	if err != nil {
		return err
	}
	server := &tunnel.Server{TLS: cfg, MaxClients: *maxClients, MaxFlows: 128}
	l, err := server.Listen(*listen)
	if err != nil {
		return err
	}
	defer l.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	slog.Info("authenticated QUIC relay listening", "address", l.Addr().String())
	return server.Serve(ctx, l)
}
func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	if err := run(); err != nil {
		slog.Error("relay stopped", "error", err)
		os.Exit(1)
	}
}
