package tunnel

import (
	"context"
	quic "github.com/quic-go/quic-go"
	"io"
	"net"
	"sync"
	"time"
)

type StreamConn struct {
	*quic.Stream
	conn *quic.Conn
}

func (s *StreamConn) LocalAddr() net.Addr  { return s.conn.LocalAddr() }
func (s *StreamConn) RemoteAddr() net.Addr { return s.conn.RemoteAddr() }
func (s *StreamConn) Close() error         { s.CancelRead(0); return s.Stream.Close() }
func (s *StreamConn) CloseWrite() error    { return s.Stream.Close() }

// Bridge preserves half-close and cancels both directions on errors/shutdown.
func Bridge(ctx context.Context, a, b net.Conn) (int64, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var total int64
	var first error
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			a.Close()
			b.Close()
		case <-done:
		}
	}()
	copyOne := func(dst, src net.Conn) {
		defer wg.Done()
		n, err := io.Copy(dst, src)
		mu.Lock()
		total += n
		if first == nil {
			first = err
		}
		mu.Unlock()
		if err != nil {
			a.Close()
			b.Close()
			return
		}
		if half, ok := dst.(interface{ CloseWrite() error }); ok {
			half.CloseWrite()
		} else {
			dst.Close()
		}
	}
	wg.Add(2)
	go copyOne(a, b)
	go copyOne(b, a)
	wg.Wait()
	return total, first
}
func quicConfig() *quic.Config {
	return &quic.Config{EnableDatagrams: true, HandshakeIdleTimeout: 3 * time.Second, MaxIdleTimeout: 45 * time.Second, KeepAlivePeriod: 15 * time.Second, MaxIncomingStreams: 128, MaxIncomingUniStreams: -1, InitialStreamReceiveWindow: 128 << 10, MaxStreamReceiveWindow: 2 << 20, InitialConnectionReceiveWindow: 512 << 10, MaxConnectionReceiveWindow: 8 << 20}
}
