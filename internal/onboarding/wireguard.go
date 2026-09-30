package onboarding

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
)

// This only provisions standard WireGuard keys/profiles. Packet transport is
// performed by the official WireGuard implementation, not by this project.
type WGRequest struct {
	Version         int    `json:"version"`
	Type            string `json:"type"`
	ClientPublicKey string `json:"client_public_key"`
}
type WGTicket struct {
	Version         int      `json:"version"`
	Type            string   `json:"type"`
	Endpoint        string   `json:"endpoint"`
	ServerPublicKey string   `json:"server_public_key"`
	ClientPublicKey string   `json:"client_public_key"`
	Address         string   `json:"address"`
	AllowedIPs      []string `json:"allowed_ips"`
	DNS             string   `json:"dns,omitempty"`
	FullTunnel      bool     `json:"full_tunnel"`
}

func wgPublic(private string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(private))
	if err != nil || len(b) != 32 {
		return "", fmt.Errorf("invalid WireGuard private key")
	}
	k, err := ecdh.X25519().NewPrivateKey(b)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(k.PublicKey().Bytes()), nil
}
func validateWGPublic(public string) error {
	b, err := base64.StdEncoding.DecodeString(public)
	if err != nil || len(b) != 32 || base64.StdEncoding.EncodeToString(b) != public {
		return fmt.Errorf("WireGuard public key must be 32 bytes of base64")
	}
	allZero := true
	for _, v := range b {
		allZero = allZero && v == 0
	}
	if allZero {
		return fmt.Errorf("invalid zero public key")
	}
	return nil
}
func wgIdentity(path string) (string, string, error) {
	b, err := os.ReadFile(path)
	if err == nil {
		private := strings.TrimSpace(string(b))
		public, err := wgPublic(private)
		return private, public, err
	}
	if !os.IsNotExist(err) {
		return "", "", err
	}
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", "", err
	}
	raw[0] &= 248
	raw[31] &= 127
	raw[31] |= 64
	private := base64.StdEncoding.EncodeToString(raw)
	public, err := wgPublic(private)
	if err != nil {
		return "", "", err
	}
	if err = writeFile(path, []byte(private+"\n")); err != nil {
		return "", "", err
	}
	return private, public, nil
}
func PrepareVPN(dir string) (string, error) {
	_, public, err := wgIdentity(filepath.Join(dir, "secrets", "wireguard.key"))
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "client-wireguard.json")
	return path, writeJSON(path, WGRequest{Version: 1, Type: "networkroute-wireguard-client", ClientPublicKey: public})
}
func SetupVPNServer(dir, endpoint, requestPath, egress, dns string, full bool) (string, error) {
	if err := Endpoint(endpoint); err != nil {
		return "", err
	}
	var request WGRequest
	if err := ReadJSON(requestPath, &request); err != nil {
		return "", err
	}
	if request.Version != 1 || request.Type != "networkroute-wireguard-client" {
		return "", fmt.Errorf("invalid WireGuard client request")
	}
	if err := validateWGPublic(request.ClientPublicKey); err != nil {
		return "", err
	}
	if full {
		if len(egress) == 0 || len(egress) > 15 {
			return "", fmt.Errorf("full tunnel needs the VPS egress interface (--egress eth0)")
		}
		for _, c := range egress {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.') {
				return "", fmt.Errorf("invalid egress interface name")
			}
		}
		ip, err := netip.ParseAddr(dns)
		if err != nil || !ip.Is4() || !ip.IsGlobalUnicast() {
			return "", fmt.Errorf("IPv4 full tunnel needs an explicit IPv4 DNS address")
		}
	}
	ticket := WGTicket{Version: 1, Type: "networkroute-wireguard", Endpoint: endpoint, ClientPublicKey: request.ClientPublicKey, Address: "10.203.0.2/32", AllowedIPs: []string{"10.203.0.1/32"}, FullTunnel: full}
	if full {
		ticket.AllowedIPs = []string{"0.0.0.0/0", "::/0"}
		ticket.DNS = dns
	}
	// A changed endpoint/policy must use a new directory; never rotate keys on rerun.
	ticketPath := filepath.Join(dir, "wireguard-connection.json")
	if _, err := os.Stat(ticketPath); err == nil {
		var old WGTicket
		if err = ReadJSON(ticketPath, &old); err != nil {
			return "", err
		}
		if old.Endpoint != endpoint || old.ClientPublicKey != request.ClientPublicKey || old.FullTunnel != full || old.DNS != ticket.DNS {
			return "", fmt.Errorf("existing WireGuard bundle differs; choose another directory")
		}
	}
	private, public, err := wgIdentity(filepath.Join(dir, "secrets", "wireguard-server.key"))
	if err != nil {
		return "", err
	}
	ticket.ServerPublicKey = public
	_, port, _ := net.SplitHostPort(endpoint)
	server := fmt.Sprintf("[Interface]\nAddress = 10.203.0.1/24\nListenPort = %s\nPrivateKey = %s\n", port, private)
	if full {
		server += "PostUp = sysctl -w net.ipv4.ip_forward=1\nPostUp = nft -f /etc/networkroute/wireguard.nft\nPostDown = nft delete table ip networkroute_wg\n"
	}
	server += fmt.Sprintf("\n[Peer]\nPublicKey = %s\nAllowedIPs = 10.203.0.2/32\n", request.ClientPublicKey)
	if err = writeFile(filepath.Join(dir, "nro-wg.conf"), []byte(server)); err != nil {
		return "", err
	}
	if full {
		rules := fmt.Sprintf("table ip networkroute_wg {\n chain postrouting {\n  type nat hook postrouting priority srcnat; policy accept;\n  ip saddr 10.203.0.0/24 oifname \"%s\" masquerade\n }\n}\n", egress)
		if err = writeFile(filepath.Join(dir, "wireguard.nft"), []byte(rules)); err != nil {
			return "", err
		}
	}
	return ticketPath, writeJSON(ticketPath, ticket)
}
func PairVPN(dir, ticketPath string, allowFull bool) (string, error) {
	var ticket WGTicket
	if err := ReadJSON(ticketPath, &ticket); err != nil {
		return "", err
	}
	if ticket.Version != 1 || ticket.Type != "networkroute-wireguard" {
		return "", fmt.Errorf("invalid WireGuard ticket")
	}
	if err := Endpoint(ticket.Endpoint); err != nil {
		return "", err
	}
	if err := validateWGPublic(ticket.ServerPublicKey); err != nil {
		return "", err
	}
	// Deliberately restrict this first helper to the reviewed single-client topology.
	if ticket.Address != "10.203.0.2/32" {
		return "", fmt.Errorf("unsupported client subnet")
	}
	if ticket.FullTunnel {
		if !allowFull {
			return "", fmt.Errorf("full tunnel changes default routes/DNS and blocks IPv6 on this IPv4-only profile; rerun with --allow-full-tunnel only if intended")
		}
		if len(ticket.AllowedIPs) != 2 || ticket.AllowedIPs[0] != "0.0.0.0/0" || ticket.AllowedIPs[1] != "::/0" {
			return "", fmt.Errorf("invalid full-tunnel routes")
		}
		ip, err := netip.ParseAddr(ticket.DNS)
		if err != nil || !ip.Is4() || !ip.IsGlobalUnicast() {
			return "", fmt.Errorf("invalid full-tunnel DNS")
		}
	} else if len(ticket.AllowedIPs) != 1 || ticket.AllowedIPs[0] != "10.203.0.1/32" || ticket.DNS != "" {
		return "", fmt.Errorf("split profile must route only the VPS private address without DNS changes")
	}
	keyPath := filepath.Join(dir, "secrets", "wireguard.key")
	if _, err := os.Stat(keyPath); err != nil {
		return "", fmt.Errorf("run vpn-prepare first; private key missing")
	}
	private, public, err := wgIdentity(keyPath)
	if err != nil {
		return "", err
	}
	if public != ticket.ClientPublicKey {
		return "", fmt.Errorf("ticket belongs to a different client key")
	}
	profile := fmt.Sprintf("[Interface]\nPrivateKey = %s\nAddress = %s\n", private, ticket.Address)
	if ticket.DNS != "" {
		profile += "DNS = " + ticket.DNS + "\n"
	}
	profile += fmt.Sprintf("\n[Peer]\nPublicKey = %s\nEndpoint = %s\nAllowedIPs = %s\nPersistentKeepalive = 25\n", ticket.ServerPublicKey, ticket.Endpoint, strings.Join(ticket.AllowedIPs, ", "))
	path := filepath.Join(dir, "nro-client.conf")
	return path, writeFile(path, []byte(profile))
}
