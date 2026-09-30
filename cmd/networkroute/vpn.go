package main

import (
	"flag"
	"networkroute/internal/onboarding"
)

func vpnCommand(command string, args []string) error {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	dir := fs.String("directory", "state/vpn", "client WireGuard directory")
	ticket := fs.String("ticket", "", "public wireguard-connection.json from VPS")
	full := fs.Bool("allow-full-tunnel", false, "explicitly accept a default-route/DNS-changing IPv4 VPN profile (IPv6 blocked)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var path string
	var err error
	if command == "vpn-prepare" {
		path, err = onboarding.PrepareVPN(*dir)
	} else {
		path, err = onboarding.PairVPN(*dir, *ticket, *full)
	}
	if err != nil {
		return err
	}
	return output(map[string]string{"created": path, "note": "Private key stays local. Import nro-client.conf into the official WireGuard Windows client only when ready; generation does not activate a VPN."})
}
