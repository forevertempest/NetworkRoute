// Package monitor contains read-only Windows API snapshots, not enforcement decisions.
package monitor

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"sort"
)

type Connection struct {
	PID      uint32 `json:"pid"`
	Process  string `json:"process,omitempty"`
	Path     string `json:"executable_path,omitempty"`
	Protocol string `json:"protocol"`
	Local    string `json:"local"`
	Remote   string `json:"remote,omitempty"`
	State    uint32 `json:"state,omitempty"`
	Note     string `json:"note,omitempty"`
}
type Interface struct {
	Name      string   `json:"name"`
	Index     int      `json:"index"`
	MTU       int      `json:"mtu"`
	Flags     string   `json:"flags"`
	Addresses []string `json:"addresses"`
}

func Interfaces() ([]Interface, error) {
	list, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	result := []Interface{}
	for _, i := range list {
		v := Interface{Name: i.Name, Index: i.Index, MTU: i.MTU, Flags: i.Flags.String()}
		a, err := i.Addrs()
		if err != nil {
			return nil, err
		}
		for _, x := range a {
			v.Addresses = append(v.Addresses, x.String())
		}
		result = append(result, v)
	}
	return result, nil
}
func Fingerprint() (string, error) {
	list, err := Interfaces()
	if err != nil {
		return "", err
	}
	rows := []string{}
	for _, i := range list {
		sort.Strings(i.Addresses)
		rows = append(rows, fmt.Sprint(i.Index, i.Flags, i.Addresses))
	}
	sort.Strings(rows)
	platform, err := routeFingerprint()
	if err != nil {
		return "", err
	}
	rows = append(rows, platform)
	sum := sha256.Sum256([]byte(fmt.Sprint(rows)))
	return hex.EncodeToString(sum[:]), nil
}
