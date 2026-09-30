//go:build windows

package monitor

import (
	"fmt"
	"golang.org/x/sys/windows"
	"unsafe"
)

func TrafficSnapshot() ([]Traffic, error) {
	var table *windows.MibIfTable2
	if err := windows.GetIfTable2Ex(windows.MibIfTableNormal, &table); err != nil {
		return nil, err
	}
	defer windows.FreeMibTable(unsafe.Pointer(table))
	if table.NumEntries > 65536 {
		return nil, fmt.Errorf("invalid interface table length")
	}
	var result []Traffic
	for _, r := range unsafe.Slice(&table.Table[0], int(table.NumEntries)) {
		if r.OperStatus != 1 {
			continue
		}
		result = append(result, Traffic{Name: windows.UTF16ToString(r.Alias[:]), Received: r.InOctets, Sent: r.OutOctets})
	}
	return result, nil
}
