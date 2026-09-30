package tunnel

import (
	"context"
	"crypto/tls"
	"fmt"
	quic "github.com/quic-go/quic-go"
	"io"
	"net"
	"net/netip"
	"networkroute/internal/metrics"
	"networkroute/internal/policy"
	"networkroute/internal/protocol"
	"sync"
	"sync/atomic"
	"time"
)

type Server struct {
	TLS            *tls.Config
	Metrics        *metrics.Counters
	MaxClients     int
	MaxFlows       int
	AllowedPorts   []uint16
	Resolver       policy.Resolver
	testAllowLocal bool
	// Dialer is injectable for integration tests/embedding; validation still runs first.
	Dialer interface {
		DialContext(context.Context, string, string) (net.Conn, error)
	}
}

func (s *Server) Listen(address string) (*quic.Listener, error) {
	return quic.ListenAddr(address, s.TLS, quicConfig())
}
func (s *Server) Serve(ctx context.Context, l *quic.Listener) error {
	if s.Metrics == nil {
		s.Metrics = &metrics.Counters{}
	}
	if s.MaxClients <= 0 {
		s.MaxClients = 32
	}
	if s.MaxFlows <= 0 {
		s.MaxFlows = 128
	}
	if s.Resolver == nil {
		s.Resolver = net.DefaultResolver
	}
	if s.Dialer == nil {
		s.Dialer = &net.Dialer{}
	}
	go func() { <-ctx.Done(); l.Close() }()
	slots := make(chan struct{}, s.MaxClients)
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		q, err := l.Accept(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case slots <- struct{}{}:
			wg.Add(1)
			go func() { defer wg.Done(); defer func() { <-slots }(); s.serveConn(ctx, q) }()
		default:
			q.CloseWithError(1, "client capacity reached")
		}
	}
}

type serverUDP struct {
	conn   net.Conn
	stream *quic.Stream
}

func (s *Server) serveConn(ctx context.Context, q *quic.Conn) {
	defer q.CloseWithError(0, "closed")
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			q.CloseWithError(0, "shutdown")
		case <-done:
		}
	}()
	var mu sync.RWMutex
	flows := make(map[uint64]*serverUDP)
	var ids atomic.Uint64
	slots := make(chan struct{}, s.MaxFlows)
	var wg sync.WaitGroup
	defer func() { q.CloseWithError(0, "closed"); wg.Wait() }()
	go func() {
		var reassembly protocol.Reassembler
		for {
			b, err := q.ReceiveDatagram(q.Context())
			if err != nil {
				return
			}
			id, p, complete, err := reassembly.Push(b, time.Now())
			if err != nil {
				s.Metrics.Dropped.Add(1)
				continue
			}
			if !complete {
				continue
			}
			mu.RLock()
			f := flows[id]
			mu.RUnlock()
			if f != nil {
				f.conn.SetWriteDeadline(time.Now().Add(time.Second))
				if _, err = f.conn.Write(p); err != nil {
					s.Metrics.Dropped.Add(1)
				} else {
					s.Metrics.Datagrams.Add(1)
					s.Metrics.Bytes.Add(uint64(len(p)))
				}
			}
		}
	}()
	for {
		stream, err := q.AcceptStream(ctx)
		if err != nil {
			return
		}
		select {
		case slots <- struct{}{}:
		default:
			stream.CancelRead(1)
			stream.CancelWrite(1)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			defer stream.Close()
			defer stream.CancelRead(0)
			stream.SetDeadline(time.Now().Add(5 * time.Second))
			var req protocol.Request
			if err := protocol.Read(stream, &req); err != nil {
				return
			}
			deny := func(msg string) { protocol.Write(stream, protocol.Response{Error: msg}) }
			if req.Version != protocol.Version || (req.Network != "tcp" && req.Network != "udp") {
				deny("unsupported protocol version or network")
				return
			}
			dctx, cancel := context.WithTimeout(q.Context(), 3*time.Second)
			defer cancel()
			target, err := s.destination(dctx, req.Target)
			if err != nil {
				deny("destination denied or resolution failed")
				return
			}
			s.Metrics.Active.Add(1)
			defer s.Metrics.Active.Add(-1)
			if req.Network == "tcp" {
				remote, err := s.Dialer.DialContext(dctx, "tcp", target)
				if err != nil {
					deny("destination TCP connect failed within 3000 ms")
					return
				}
				defer remote.Close()
				if err = protocol.Write(stream, protocol.Response{OK: true}); err != nil {
					return
				}
				stream.SetDeadline(time.Time{})
				s.Metrics.Relayed.Add(1)
				n, _ := Bridge(q.Context(), &StreamConn{Stream: stream, conn: q}, remote)
				s.Metrics.Bytes.Add(uint64(n))
				return
			}
			remote, err := s.Dialer.DialContext(dctx, "udp", target)
			if err != nil {
				deny("UDP socket unavailable")
				return
			}
			defer remote.Close()
			id := ids.Add(1)
			mu.Lock()
			flows[id] = &serverUDP{conn: remote, stream: stream}
			mu.Unlock()
			defer func() { mu.Lock(); delete(flows, id); mu.Unlock() }()
			if err = protocol.Write(stream, protocol.Response{OK: true, FlowID: id}); err != nil {
				return
			}
			stream.SetDeadline(time.Time{})
			s.Metrics.Relayed.Add(1)
			udpDone := make(chan struct{})
			go func() { io.Copy(io.Discard, stream); remote.Close(); close(udpDone) }()
			defer func() { stream.CancelRead(0); remote.Close(); <-udpDone }()
			buf := make([]byte, 65536)
			var sequence uint32
			for {
				remote.SetReadDeadline(time.Now().Add(2 * time.Minute))
				n, err := remote.Read(buf)
				if err != nil {
					return
				}
				sequence++
				frames, err := protocol.Fragments(id, sequence, buf[:n])
				if err != nil {
					s.Metrics.Dropped.Add(1)
					continue
				}
				for _, b := range frames {
					if err = q.SendDatagram(b); err != nil {
						break
					}
				}
				if err != nil {
					s.Metrics.Dropped.Add(1)
				} else {
					s.Metrics.Datagrams.Add(1)
					s.Metrics.Bytes.Add(uint64(n))
				}
			}
		}()
	}
}
func (s *Server) destination(ctx context.Context, target string) (string, error) {
	addrs, err := policy.Resolve(ctx, s.Resolver, target)
	if err != nil {
		return "", err
	}
	for _, a := range addrs {
		ap, err := netip.ParseAddrPort(a)
		if err != nil {
			continue
		}
		allowed := len(s.AllowedPorts) == 0
		for _, p := range s.AllowedPorts {
			if p == ap.Port() {
				allowed = true
			}
		}
		if !allowed {
			continue
		}
		if s.testAllowLocal || (policy.Public(ap.Addr()) && !policy.LocalAddress(ap.Addr()) && !policy.OnLink(ap.Addr())) {
			return a, nil
		}
	}
	return "", fmt.Errorf("destination is not permitted")
}
