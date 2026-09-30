//go:build linux

package monitor

import (
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func procAddress(s string) (string, error) {
	host, port, ok := strings.Cut(s, ":")
	if !ok {
		return "", fmt.Errorf("invalid proc address")
	}
	b, err := hex.DecodeString(host)
	if err != nil || (len(b) != 4 && len(b) != 16) {
		return "", fmt.Errorf("invalid proc IP")
	}
	// Linux amd64/arm64 proc encodes each 32-bit address word in host order.
	for i := 0; i < len(b); i += 4 {
		b[i], b[i+1], b[i+2], b[i+3] = b[i+3], b[i+2], b[i+1], b[i]
	}
	p, err := strconv.ParseUint(port, 16, 16)
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(net.IP(b).String(), strconv.Itoa(int(p))), nil
}

func Connections() ([]Connection, error) {
	var rows []Connection
	byInode := map[string][]int{}
	readAny := false
	for _, proto := range []string{"tcp", "tcp6", "udp", "udp6"} {
		b, err := os.ReadFile("/proc/net/" + proto)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		readAny = true
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) < 10 {
				continue
			}
			local, e := procAddress(f[1])
			if e != nil {
				continue
			}
			remote, e := procAddress(f[2])
			if e != nil {
				continue
			}
			state, _ := strconv.ParseUint(f[3], 16, 32)
			byInode[f[9]] = append(byInode[f[9]], len(rows))
			rows = append(rows, Connection{Protocol: strings.ToUpper(proto), Local: local, Remote: remote, State: uint32(state), Note: "Linux proc snapshot; PID may be unavailable due to permissions or process exit"})
		}
	}
	if !readAny {
		return nil, fmt.Errorf("Linux /proc/net is unavailable")
	}
	processes, err := os.ReadDir("/proc")
	if err != nil {
		return rows, nil
	}
	budget := 100000
	for _, entry := range processes {
		pid, e := strconv.ParseUint(entry.Name(), 10, 32)
		if e != nil {
			continue
		}
		base := filepath.Join("/proc", entry.Name())
		fds, e := os.ReadDir(filepath.Join(base, "fd"))
		if e != nil {
			continue
		}
		for _, fd := range fds {
			budget--
			if budget < 0 {
				return rows, nil
			}
			link, e := os.Readlink(filepath.Join(base, "fd", fd.Name()))
			if e != nil || !strings.HasPrefix(link, "socket:[") {
				continue
			}
			inode := strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")
			indexes := byInode[inode]
			if len(indexes) == 0 {
				continue
			}
			exe, _ := os.Readlink(filepath.Join(base, "exe"))
			name, _ := os.ReadFile(filepath.Join(base, "comm"))
			for _, i := range indexes {
				if rows[i].PID == 0 {
					rows[i].PID = uint32(pid)
					rows[i].Process = strings.TrimSpace(string(name))
					rows[i].Path = exe
				}
			}
		}
	}
	return rows, nil
}

func VPNHints() ([]string, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var hints []string
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 {
			continue
		}
		name := strings.ToLower(i.Name)
		for _, prefix := range []string{"tun", "tap", "wg", "tailscale", "ppp", "ipsec"} {
			if strings.HasPrefix(name, prefix) {
				hints = append(hints, i.Name)
				break
			}
		}
	}
	return hints, nil
}

func routeFingerprint() (string, error) {
	var result strings.Builder
	for _, path := range []string{"/proc/net/route", "/proc/net/ipv6_route"} {
		b, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		// Exclude volatile RefCnt/Use counters from the network epoch.
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if path == "/proc/net/route" && len(f) >= 11 {
				f[4], f[5] = "", ""
			}
			if path == "/proc/net/ipv6_route" && len(f) >= 10 {
				f[6], f[7] = "", ""
			}
			result.WriteString(strings.Join(f, " "))
			result.WriteByte('\n')
		}
	}
	return result.String(), nil
}
