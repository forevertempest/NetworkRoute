//go:build windows

package monitor

import (
	"fmt"
	"golang.org/x/sys/windows"
	"path/filepath"
	"unsafe"
)

var iphlpapi = windows.NewLazySystemDLL("iphlpapi.dll")

func table(ipv6, tcp bool) ([]Connection, error) {
	name := "GetExtendedUdpTable"
	class := uintptr(1)
	if tcp {
		name = "GetExtendedTcpTable"
		class = 5
	}
	af := uintptr(2)
	if ipv6 {
		af = 23
	}
	proc := iphlpapi.NewProc(name)
	var size uint32
	ret, _, _ := proc.Call(0, uintptr(unsafe.Pointer(&size)), 0, af, class, 0)
	if ret != 122 && ret != 0 {
		return nil, fmt.Errorf("%s sizing failed: Windows error %d", name, ret)
	}
	for tries := 0; tries < 4; tries++ {
		if size < 4 || size > 64<<20 {
			return nil, fmt.Errorf("invalid IP Helper table size %d", size)
		}
		buf := make([]byte, size)
		ret, _, _ = proc.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, af, class, 0)
		if ret == 122 {
			continue
		}
		if ret != 0 {
			return nil, fmt.Errorf("%s failed: Windows error %d", name, ret)
		}
		return ParseTable(buf, ipv6, tcp)
	}
	return nil, fmt.Errorf("IP Helper table kept changing; retry snapshot")
}
func Connections() ([]Connection, error) {
	all := []Connection{}
	for _, v6 := range []bool{false, true} {
		for _, tcp := range []bool{true, false} {
			rows, err := table(v6, tcp)
			if err != nil {
				return nil, err
			}
			all = append(all, rows...)
		}
	}
	paths := make(map[uint32]string)
	for i := range all {
		pid := all[i].PID
		path, ok := paths[pid]
		if !ok {
			h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
			if err == nil {
				buf := make([]uint16, 32768)
				n := uint32(len(buf))
				if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) == nil {
					path = windows.UTF16ToString(buf[:n])
				}
				windows.CloseHandle(h)
			}
			paths[pid] = path
		}
		all[i].Path = path
		if path != "" {
			all[i].Process = filepath.Base(path)
		}
	}
	return all, nil
}
