package onboarding

import (
	"fmt"
	"net"
	"networkroute/internal/config"
	"networkroute/internal/identity"
	"networkroute/internal/policy"
	"os"
	"path/filepath"
)

type ClientRequest struct {
	Version   int    `json:"version"`
	Type      string `json:"type"`
	ClientPin string `json:"client_pin"`
}
type RelayTicket struct {
	Version   int    `json:"version"`
	Type      string `json:"type"`
	Address   string `json:"address"`
	ServerPin string `json:"server_pin"`
}
type RelayConfig struct {
	Version     int      `json:"version"`
	Listen      string   `json:"listen"`
	Certificate string   `json:"certificate"`
	PrivateKey  string   `json:"private_key"`
	ClientPins  []string `json:"client_pins"`
	MaxClients  int      `json:"max_clients"`
}

func baseConfig() config.Config {
	return config.Config{Version: 1, Listen: "127.0.0.1:1080", Mode: "smart", Profile: policy.Auto, FailMode: "open", MaxConnections: 128, BypassDomains: []string{"localhost", "local"}}
}
func SetupClient(dir, mode string) (string, error) {
	if mode != "standalone" && mode != "vpn" && mode != "relay" {
		return "", fmt.Errorf("setup mode must be standalone, vpn or relay")
	}
	if mode != "relay" {
		path := filepath.Join(dir, "config.local.json")
		return path, writeJSON(path, baseConfig())
	}
	pin, err := identity.Existing(filepath.Join(dir, "secrets", "client"))
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "client-request.json")
	return path, writeJSON(path, ClientRequest{Version: 1, Type: "networkroute-client", ClientPin: pin})
}
func SetupRelay(dir, address, requestPath string) (string, error) {
	if err := Endpoint(address); err != nil {
		return "", err
	}
	var request ClientRequest
	if err := ReadJSON(requestPath, &request); err != nil {
		return "", err
	}
	if request.Version != 1 || request.Type != "networkroute-client" {
		return "", fmt.Errorf("invalid client request type/version")
	}
	if err := validatePin(request.ClientPin); err != nil {
		return "", err
	}
	// Refuse changing an installed config before creating/reusing any key.
	_, port, _ := net.SplitHostPort(address)
	configuration := RelayConfig{Version: 1, Listen: net.JoinHostPort("", port), Certificate: "identity/identity.pem", PrivateKey: "identity/identity.key", ClientPins: []string{request.ClientPin}, MaxClients: 32}
	configPath := filepath.Join(dir, "relay.json")
	if _, err := os.Stat(configPath); err == nil {
		var old RelayConfig
		if err = ReadJSON(configPath, &old); err != nil {
			return "", err
		}
		if old.Listen != configuration.Listen || len(old.ClientPins) != 1 || old.ClientPins[0] != request.ClientPin {
			return "", fmt.Errorf("existing relay configuration differs; choose another directory")
		}
	}
	pin, err := identity.Existing(filepath.Join(dir, "identity"))
	if err != nil {
		return "", err
	}
	if err = writeJSON(configPath, configuration); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "relay-connection.json")
	return path, writeJSON(path, RelayTicket{Version: 1, Type: "networkroute-relay", Address: address, ServerPin: pin})
}
func Pair(dir, ticketPath, vpnPolicy string) (string, error) {
	if vpnPolicy != "respect" && vpnPolicy != "nested" {
		return "", fmt.Errorf("vpn-policy must be respect or nested")
	}
	var ticket RelayTicket
	if err := ReadJSON(ticketPath, &ticket); err != nil {
		return "", err
	}
	if ticket.Version != 1 || ticket.Type != "networkroute-relay" {
		return "", fmt.Errorf("invalid relay ticket type/version")
	}
	if err := Endpoint(ticket.Address); err != nil {
		return "", err
	}
	if err := validatePin(ticket.ServerPin); err != nil {
		return "", err
	}
	// Pairing never creates a different client identity from the one sent to VPS.
	var request ClientRequest
	if err := ReadJSON(filepath.Join(dir, "client-request.json"), &request); err != nil {
		return "", fmt.Errorf("run setup --mode relay first: %w", err)
	}
	identityDir := filepath.Join(dir, "secrets", "client")
	if _, err := os.Stat(filepath.Join(identityDir, "identity.key")); err != nil {
		return "", err
	}
	pin, err := identity.Existing(identityDir)
	if err != nil {
		return "", err
	}
	if request.Version != 1 || request.Type != "networkroute-client" || pin != request.ClientPin {
		return "", fmt.Errorf("client request does not match the local identity")
	}
	c := baseConfig()
	c.Certificate = "secrets/client/identity.pem"
	c.PrivateKey = "secrets/client/identity.key"
	c.Relay = config.Relay{Address: ticket.Address, ServerPin: ticket.ServerPin}
	c.AllowNestedTunnel = vpnPolicy == "nested"
	path := filepath.Join(dir, "config.local.json")
	return path, writeJSON(path, c)
}
func LoadRelay(path string) (RelayConfig, error) {
	var c RelayConfig
	if err := ReadJSON(path, &c); err != nil {
		return c, err
	}
	if c.Version != 1 || len(c.ClientPins) == 0 || len(c.ClientPins) > 256 {
		return c, fmt.Errorf("invalid relay version or client pin count")
	}
	host, port, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return c, err
	}
	if host == "" {
		host = "127.0.0.1"
	}
	if err = Endpoint(net.JoinHostPort(host, port)); err != nil {
		return c, err
	}
	for _, p := range c.ClientPins {
		if err = validatePin(p); err != nil {
			return c, err
		}
	}
	if c.MaxClients < 1 || c.MaxClients > 256 || c.Certificate == "" || c.PrivateKey == "" {
		return c, fmt.Errorf("invalid relay limits or identity paths")
	}
	for _, p := range []*string{&c.Certificate, &c.PrivateKey} {
		if !filepath.IsAbs(*p) {
			*p = filepath.Join(filepath.Dir(path), *p)
		}
	}
	return c, nil
}
