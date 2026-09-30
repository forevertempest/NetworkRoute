// Package protocol defines bounded, versioned flow control framing. Payload bytes remain opaque.
package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

const Version = 1
const ALPN = "networkroute/1"
const MaxControl = 2048

// Maximum original UDP payload. Tunnel fragments are unreliable DATAGRAMs, never streams.
const MaxUDP = 65507

type Request struct {
	Version int    `json:"version"`
	Network string `json:"network"`
	Target  string `json:"target"`
}
type Response struct {
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
	FlowID uint64 `json:"flow_id,omitempty"`
}

func Write(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(b) > MaxControl {
		return fmt.Errorf("control frame too large")
	}
	var h [4]byte
	binary.BigEndian.PutUint32(h[:], uint32(len(b)))
	if err = writeAll(w, h[:]); err != nil {
		return err
	}
	return writeAll(w, b)
}
func writeAll(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}
func Read(r io.Reader, v any) error {
	var h [4]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(h[:])
	if n == 0 || n > MaxControl {
		return fmt.Errorf("invalid control frame length: %d", n)
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
