// Package policy contains payload-independent destination and profile rules.
package policy

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

type Profile string

const (
	Auto        Profile = "AUTO"
	Realtime    Profile = "REALTIME"
	Interactive Profile = "INTERACTIVE"
	Streaming   Profile = "STREAMING"
	Bulk        Profile = "BULK"
)

func (p Profile) Valid() bool {
	return p == Auto || p == Realtime || p == Interactive || p == Streaming || p == Bulk
}

// These ranges must never become relay-side SSRF or LAN bypass destinations.
var reserved = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/3"), netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("64:ff9b::/96"), netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"), netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("fc00::/7"), netip.MustParsePrefix("fe80::/10"), netip.MustParsePrefix("ff00::/8"),
}

func Public(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.Zone() != "" {
		return false
	}
	for _, p := range reserved {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}
func LocalAddress(ip netip.Addr) bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return true
	}
	for _, a := range addrs {
		p, err := netip.ParsePrefix(a.String())
		if err == nil && p.Addr().Unmap() == ip.Unmap() {
			return true
		}
	}
	return false
}

// OnLink also protects public-numbered LAN/VPN ranges such as overlay networks.
func OnLink(ip netip.Addr) bool {
	interfaces, err := net.Interfaces()
	if err != nil {
		return true
	}
	for _, i := range interfaces {
		if i.Flags&net.FlagUp == 0 {
			continue
		}
		addresses, err := i.Addrs()
		if err != nil {
			return true
		}
		for _, a := range addresses {
			p, err := netip.ParsePrefix(a.String())
			if err == nil && p.Contains(ip.Unmap()) {
				return true
			}
		}
	}
	return false
}

type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

// Resolve returns numeric addresses once. Callers dial these exact IPs, not the hostname again.
func Resolve(ctx context.Context, resolver Resolver, target string) ([]string, error) {
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return nil, fmt.Errorf("destination must be host:port: %w", err)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return nil, fmt.Errorf("invalid destination port")
	}
	if host == "" || len(host) > 253 {
		return nil, fmt.Errorf("invalid destination host")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return []string{net.JoinHostPort(ip.String(), port)}, nil
	}
	ips, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve destination: %w", err)
	}
	var out []string
	for _, ip := range ips {
		out = append(out, net.JoinHostPort(ip.Unmap().String(), port))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("destination has no addresses")
	}
	return out, nil
}

// DomainMatch uses label boundaries; example.com does not match notexample.com.
func DomainMatch(host, rule string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	rule = strings.ToLower(strings.TrimSuffix(rule, "."))
	return host == rule || strings.HasSuffix(host, "."+rule)
}
func Bypass(target string, domains []string, cidrs []netip.Prefix) bool {
	host, _, err := net.SplitHostPort(target)
	if err != nil {
		return true
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if !Public(ip) {
			return true
		}
		for _, p := range cidrs {
			if p.Contains(ip.Unmap()) {
				return true
			}
		}
	}
	for _, d := range domains {
		if DomainMatch(host, d) {
			return true
		}
	}
	return false
}
