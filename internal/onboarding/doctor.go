package onboarding

import (
	"context"
	"fmt"
	"net"
	"networkroute/internal/config"
	"networkroute/internal/identity"
	"networkroute/internal/monitor"
	"networkroute/internal/tunnel"
	"time"
)

type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}
type Report struct {
	Status string  `json:"status"`
	Checks []Check `json:"checks"`
}

func Doctor(ctx context.Context, path string, checkRelay bool) Report {
	report := Report{Status: "ready"}
	add := func(name, status, detail string) {
		report.Checks = append(report.Checks, Check{name, status, detail})
		if status == "failed" {
			report.Status = "failed"
		} else if status == "warning" && report.Status == "ready" {
			report.Status = "warnings"
		}
	}
	c, err := config.Load(path)
	if err != nil {
		add("configuration", "failed", err.Error())
		return report
	}
	add("configuration", "ok", "Valid config; loopback listener, existing OS routes preserved")
	listener, err := net.Listen("tcp", c.Listen)
	if err != nil {
		add("local_port", "warning", "Listener unavailable (proxy may already be running): "+err.Error())
	} else {
		listener.Close()
		add("local_port", "ok", c.Listen+" is available")
	}
	hints, err := monitor.VPNHints()
	if err != nil {
		add("vpn", "warning", "Cannot inspect VPN adapters: "+err.Error())
	} else if len(hints) > 0 {
		detail := fmt.Sprintf("%d possible VPN/tunnel adapters; OS routes remain authoritative", len(hints))
		if c.Relay.Address != "" && !c.AllowNestedTunnel {
			detail += ". Relay forwarding is suspended; use an explicitly paired nested profile if intended"
		}
		add("vpn", "warning", detail)
	} else {
		add("vpn", "ok", "No recognized VPN adapter; this is not proof of VPN absence")
	}
	if c.Mode == "direct" || c.Relay.Address == "" {
		add("relay", "ok", "Not required in standalone mode")
		return report
	}
	tlsConfig, err := identity.Config(c.Certificate, c.PrivateKey, []string{c.Relay.ServerPin}, false)
	if err != nil {
		add("identity", "failed", err.Error())
		return report
	}
	add("identity", "ok", "Local key/certificate match; server public-key pin configured")
	if !checkRelay {
		add("relay", "warning", "Not tested; run doctor --check-relay for an authenticated UDP/QUIC handshake")
		return report
	}
	client := &tunnel.Client{Address: c.Relay.Address, TLS: tlsConfig}
	defer client.Close()
	probe, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	start := time.Now()
	if err = client.Connect(probe); err != nil {
		add("relay", "failed", "Authentication/connect failed; check pin, UDP port, firewall and VPN: "+err.Error())
	} else {
		add("relay", "ok", fmt.Sprintf("Authenticated QUIC handshake in %d ms; destination path not benchmarked", time.Since(start).Milliseconds()))
	}
	return report
}
