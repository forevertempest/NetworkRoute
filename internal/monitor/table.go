package monitor

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
)

func ParseTable(data []byte, ipv6, tcp bool) ([]Connection, error) {
	stride := 12
	if tcp {
		stride = 24
	}
	if ipv6 {
		stride = 28
		if tcp {
			stride = 56
		}
	}
	if len(data) < 4 {
		return nil, fmt.Errorf("short IP Helper table")
	}
	count := int(binary.LittleEndian.Uint32(data[:4]))
	if count > (len(data)-4)/stride {
		return nil, fmt.Errorf("IP Helper row count exceeds buffer")
	}
	result := make([]Connection, 0, count)
	u32 := func(b []byte) uint32 { return binary.LittleEndian.Uint32(b) }
	address := func(b []byte, scope uint32, p []byte) string {
		host := net.IP(b).String()
		if scope != 0 {
			host += "%" + strconv.FormatUint(uint64(scope), 10)
		}
		return net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(p[:2]))))
	}
	for i := 0; i < count; i++ {
		b := data[4+i*stride : 4+(i+1)*stride]
		c := Connection{Protocol: "UDP", Note: "UDP owner table has no remote destination"}
		if tcp {
			c.Protocol = "TCP"
			c.Note = ""
		}
		switch {
		case tcp && ipv6:
			c.Local = address(b[:16], u32(b[16:20]), b[20:24])
			c.Remote = address(b[24:40], u32(b[40:44]), b[44:48])
			c.State = u32(b[48:52])
			c.PID = u32(b[52:56])
		case tcp:
			c.State = u32(b[:4])
			c.Local = address(b[4:8], 0, b[8:12])
			c.Remote = address(b[12:16], 0, b[16:20])
			c.PID = u32(b[20:24])
		case ipv6:
			c.Local = address(b[:16], u32(b[16:20]), b[20:24])
			c.PID = u32(b[24:28])
		default:
			c.Local = address(b[:4], 0, b[4:8])
			c.PID = u32(b[8:12])
		}
		result = append(result, c)
	}
	return result, nil
}
