//go:build windows

package monitor

import (
	"fmt"
	"golang.org/x/sys/windows"
	"sort"
	"unsafe"
)

func routeFingerprint() (string, error) {
	var table *windows.MibIpForwardTable2
	if err := windows.GetIpForwardTable2(windows.AF_UNSPEC, &table); err != nil {
		return "", err
	}
	defer windows.FreeMibTable(unsafe.Pointer(table))
	values := []string{}
	for _, r := range table.Rows() {
		values = append(values, fmt.Sprint(r.InterfaceLuid, r.InterfaceIndex, r.DestinationPrefix, r.NextHop, r.Metric, r.Protocol))
	}
	sort.Strings(values)
	return fmt.Sprint(values), nil
}
