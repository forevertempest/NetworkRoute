package monitor

import (
	"encoding/binary"
	"testing"
)

func TestIPv4TCPTable(t *testing.T) {
	b := make([]byte, 28)
	binary.LittleEndian.PutUint32(b, 1)
	row := b[4:]
	binary.LittleEndian.PutUint32(row, 5)
	copy(row[4:], []byte{127, 0, 0, 1})
	binary.BigEndian.PutUint16(row[8:], 1234)
	copy(row[12:], []byte{1, 1, 1, 1})
	binary.BigEndian.PutUint16(row[16:], 443)
	binary.LittleEndian.PutUint32(row[20:], 42)
	rows, err := ParseTable(b, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].PID != 42 || rows[0].Remote != "1.1.1.1:443" || rows[0].Local != "127.0.0.1:1234" {
		t.Fatalf("bad parse %+v", rows)
	}
}
func TestIPv6UDPNoInventedDestination(t *testing.T) {
	b := make([]byte, 32)
	binary.LittleEndian.PutUint32(b, 1)
	b[19] = 1
	binary.BigEndian.PutUint16(b[24:], 53)
	binary.LittleEndian.PutUint32(b[28:], 55)
	rows, err := ParseTable(b, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Remote != "" || rows[0].PID != 55 || rows[0].Local != "[::1]:53" {
		t.Fatalf("bad row %+v", rows)
	}
}
func TestShortTable(t *testing.T) {
	if _, err := ParseTable([]byte{255, 255, 255, 255}, true, true); err == nil {
		t.Fatal("unbounded rows")
	}
}
