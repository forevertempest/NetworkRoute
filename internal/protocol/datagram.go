package protocol

import (
	"encoding/binary"
	"fmt"
	"time"
)

// Fragmenting application datagrams preserves 1200-byte QUIC Initial packets and DNS/UDP.
// A lost fragment drops the whole original datagram. There is no retransmission layer.
const Chunk = 1000
const maxFragments = (MaxUDP + Chunk - 1) / Chunk

func Fragments(id uint64, message uint32, p []byte) ([][]byte, error) {
	if len(p) > MaxUDP {
		return nil, fmt.Errorf("UDP payload %d exceeds %d", len(p), MaxUDP)
	}
	count := (len(p) + Chunk - 1) / Chunk
	if count == 0 {
		count = 1
	}
	out := make([][]byte, 0, count)
	for i := 0; i < count; i++ {
		start := i * Chunk
		end := start + Chunk
		if end > len(p) {
			end = len(p)
		}
		b := make([]byte, 16+end-start)
		binary.BigEndian.PutUint64(b, id)
		binary.BigEndian.PutUint32(b[8:], message)
		binary.BigEndian.PutUint16(b[12:], uint16(i))
		binary.BigEndian.PutUint16(b[14:], uint16(count))
		copy(b[16:], p[start:end])
		out = append(out, b)
	}
	return out, nil
}

type assembly struct {
	created        time.Time
	parts          [][]byte
	received, size int
}
type Reassembler struct{ items map[[2]uint64]*assembly }

func (r *Reassembler) Push(b []byte, now time.Time) (uint64, []byte, bool, error) {
	if len(b) < 16 || len(b) > 16+Chunk {
		return 0, nil, false, fmt.Errorf("invalid fragment size")
	}
	id := binary.BigEndian.Uint64(b)
	msg := binary.BigEndian.Uint32(b[8:])
	index, count := int(binary.BigEndian.Uint16(b[12:])), int(binary.BigEndian.Uint16(b[14:]))
	if count < 1 || count > maxFragments || index >= count || (index < count-1 && len(b) != 16+Chunk) {
		return 0, nil, false, fmt.Errorf("invalid fragment header")
	}
	if count == 1 {
		return id, b[16:], true, nil
	}
	if r.items == nil {
		r.items = make(map[[2]uint64]*assembly)
	}
	for k, v := range r.items {
		if now.Sub(v.created) > 2*time.Second {
			delete(r.items, k)
		}
	}
	key := [2]uint64{id, uint64(msg)}
	v := r.items[key]
	if v == nil {
		if len(r.items) >= 32 {
			return id, nil, false, fmt.Errorf("reassembly capacity reached")
		}
		v = &assembly{created: now, parts: make([][]byte, count)}
		r.items[key] = v
	}
	if len(v.parts) != count {
		return id, nil, false, fmt.Errorf("fragment count changed")
	}
	if v.parts[index] != nil {
		return id, nil, false, nil
	}
	v.parts[index] = append([]byte{}, b[16:]...)
	v.received++
	v.size += len(b) - 16
	if v.size > MaxUDP {
		delete(r.items, key)
		return id, nil, false, fmt.Errorf("reassembled UDP payload too large")
	}
	if v.received != count {
		return id, nil, false, nil
	}
	out := make([]byte, 0, v.size)
	for _, p := range v.parts {
		out = append(out, p...)
	}
	delete(r.items, key)
	return id, out, true, nil
}
