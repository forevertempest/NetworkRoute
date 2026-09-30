package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/netip"
	"networkroute/internal/policy"
	"os"
	"path/filepath"
)

type Relay struct {
	Address   string `json:"address"`
	ServerPin string `json:"server_pin"`
}
type Config struct {
	Version           int            `json:"version"`
	Listen            string         `json:"listen"`
	Mode              string         `json:"mode"`
	Profile           policy.Profile `json:"profile"`
	FailMode          string         `json:"fail_mode"`
	Certificate       string         `json:"certificate"`
	PrivateKey        string         `json:"private_key"`
	Relay             Relay          `json:"relay"`
	BypassDomains     []string       `json:"bypass_domains"`
	BypassCIDRs       []string       `json:"bypass_cidrs"`
	MaxConnections    int            `json:"max_connections"`
	AllowNestedTunnel bool           `json:"allow_nested_tunnel"`
	DirectRaceDelayMS int            `json:"direct_race_delay_ms,omitempty"`
}

func Load(path string) (Config, error) {
	var c Config
	f, err := os.Open(path)
	if err != nil {
		return c, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return c, err
	}
	if info.Size() > 65536 {
		return c, fmt.Errorf("config exceeds 64 KiB")
	}
	d := json.NewDecoder(io.LimitReader(f, 65537))
	d.DisallowUnknownFields()
	if err = d.Decode(&c); err != nil {
		return c, err
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return c, fmt.Errorf("unexpected trailing config content")
	}
	if c.Version != 1 {
		return c, fmt.Errorf("unsupported config version")
	}
	if c.Listen == "" {
		c.Listen = "127.0.0.1:1080"
	}
	host, _, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return c, err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !ip.IsLoopback() {
		return c, fmt.Errorf("MVP SOCKS listener must be a numeric loopback address")
	}
	if c.Mode != "smart" && c.Mode != "direct" && c.Mode != "relay" {
		return c, fmt.Errorf("mode must be smart, direct or relay; transparent system mode is not implemented")
	}
	if c.FailMode != "open" && c.FailMode != "closed" {
		return c, fmt.Errorf("fail_mode must be open or closed")
	}
	if !c.Profile.Valid() {
		return c, fmt.Errorf("unknown traffic profile")
	}
	if c.MaxConnections == 0 {
		c.MaxConnections = 128
	}
	if c.MaxConnections < 1 || c.MaxConnections > 1024 {
		return c, fmt.Errorf("max_connections must be 1..1024")
	}
	if c.DirectRaceDelayMS == 0 {
		c.DirectRaceDelayMS = 250
	}
	if c.DirectRaceDelayMS < 10 || c.DirectRaceDelayMS > 2000 {
		return c, fmt.Errorf("direct_race_delay_ms must be 10..2000")
	}
	for _, p := range c.BypassCIDRs {
		if _, err := netip.ParsePrefix(p); err != nil {
			return c, err
		}
	}
	if c.Mode == "relay" && c.Relay.Address == "" {
		return c, fmt.Errorf("manual relay mode requires an explicitly configured relay; smart mode works without one")
	}
	if c.Mode == "smart" && c.FailMode == "closed" && c.Relay.Address == "" {
		return c, fmt.Errorf("fail-closed requires a relay; use fail_mode open for standalone Smart")
	}
	if c.Mode != "direct" && c.Relay.Address != "" {
		if _, _, err = net.SplitHostPort(c.Relay.Address); err != nil {
			return c, fmt.Errorf("relay.address: %w", err)
		}
		if c.Certificate == "" || c.PrivateKey == "" || len(c.Relay.ServerPin) != 64 {
			return c, fmt.Errorf("relay modes require identity files and a 64-character server pin")
		}
	}
	for _, p := range []*string{&c.Certificate, &c.PrivateKey} {
		if *p != "" && !filepath.IsAbs(*p) {
			*p = filepath.Join(filepath.Dir(path), *p)
		}
	}
	return c, nil
}
