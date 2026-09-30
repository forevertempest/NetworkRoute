// Package proxy implements the explicit, unprivileged SOCKS5 MVP data plane.
package proxy

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"networkroute/internal/tunnel"
	"strconv"
	"sync"
	"time"
)

type Server struct{ Router *Router }

func (s *Server) Serve(ctx context.Context, l net.Listener) error {
	slots := make(chan struct{}, s.Router.Config.MaxConnections)
	var wg sync.WaitGroup
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			l.Close()
		case <-done:
		}
	}()
	defer wg.Wait()
	for {
		c, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case slots <- struct{}{}:
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-slots }()
				defer c.Close()
				stop := context.AfterFunc(ctx, func() { c.Close() })
				defer stop()
				if err := s.handle(ctx, c); err != nil {
					slog.Debug("SOCKS flow ended", "error", err)
				}
			}()
		default:
			c.Close()
		}
	}
}
func readAddress(r io.Reader, kind byte) (string, error) {
	var host string
	switch kind {
	case 1:
		b := make([]byte, 4)
		if _, err := io.ReadFull(r, b); err != nil {
			return "", err
		}
		host = net.IP(b).String()
	case 4:
		b := make([]byte, 16)
		if _, err := io.ReadFull(r, b); err != nil {
			return "", err
		}
		host = net.IP(b).String()
	case 3:
		var n [1]byte
		if _, err := io.ReadFull(r, n[:]); err != nil {
			return "", err
		}
		if n[0] == 0 {
			return "", fmt.Errorf("empty SOCKS hostname")
		}
		b := make([]byte, int(n[0]))
		if _, err := io.ReadFull(r, b); err != nil {
			return "", err
		}
		host = string(b)
	default:
		return "", fmt.Errorf("unsupported SOCKS address type")
	}
	var port [2]byte
	if _, err := io.ReadFull(r, port[:]); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(port[:])))), nil
}
func encodeAddress(target string) ([]byte, error) {
	h, p, err := net.SplitHostPort(target)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(p)
	if err != nil || port < 0 || port > 65535 {
		return nil, fmt.Errorf("invalid port")
	}
	var b []byte
	if ip := net.ParseIP(h); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			b = append([]byte{1}, ip4...)
		} else {
			b = append([]byte{4}, ip.To16()...)
		}
	} else {
		if len(h) > 255 {
			return nil, fmt.Errorf("hostname too long")
		}
		b = append([]byte{3, byte(len(h))}, []byte(h)...)
	}
	return append(b, byte(port>>8), byte(port)), nil
}
func reply(c net.Conn, code byte, address string) error {
	b, err := encodeAddress(address)
	if err != nil {
		return err
	}
	_, err = c.Write(append([]byte{5, code, 0}, b...))
	return err
}
func (s *Server) handle(ctx context.Context, c net.Conn) error {
	c.SetDeadline(time.Now().Add(10 * time.Second))
	var head [2]byte
	if _, err := io.ReadFull(c, head[:]); err != nil {
		return err
	}
	if head[0] != 5 || head[1] == 0 {
		return fmt.Errorf("SOCKS5 greeting required")
	}
	methods := make([]byte, int(head[1]))
	if _, err := io.ReadFull(c, methods); err != nil {
		return err
	}
	if !bytes.Contains(methods, []byte{0}) {
		c.Write([]byte{5, 255})
		return fmt.Errorf("SOCKS authentication method unsupported")
	}
	if _, err := c.Write([]byte{5, 0}); err != nil {
		return err
	}
	var req [4]byte
	if _, err := io.ReadFull(c, req[:]); err != nil {
		return err
	}
	if req[0] != 5 || req[2] != 0 {
		return fmt.Errorf("invalid SOCKS request")
	}
	target, err := readAddress(c, req[3])
	if err != nil {
		reply(c, 8, "0.0.0.0:0")
		return err
	}
	switch req[1] {
	case 1:
		dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		remote, _, err := s.Router.DialTCP(dctx, target)
		cancel()
		if err != nil {
			reply(c, 1, "0.0.0.0:0")
			return err
		}
		defer remote.Close()
		if err = reply(c, 0, "0.0.0.0:0"); err != nil {
			return err
		}
		c.SetDeadline(time.Time{})
		s.Router.Metrics.Active.Add(1)
		defer s.Router.Metrics.Active.Add(-1)
		n, err := tunnel.Bridge(ctx, c, remote)
		s.Router.Metrics.Bytes.Add(uint64(n))
		return err
	case 3:
		return s.udp(ctx, c, target)
	default:
		reply(c, 7, "0.0.0.0:0")
		return fmt.Errorf("SOCKS BIND is not supported")
	}
}
func (s *Server) udp(parent context.Context, control net.Conn, requested string) error {
	// Pin replies to the association's local client, never create an open UDP proxy.
	peer := control.RemoteAddr().(*net.TCPAddr)
	local := control.LocalAddr().(*net.TCPAddr)
	addr, err := net.ResolveUDPAddr("udp", requested)
	if err != nil {
		return err
	}
	if !addr.IP.IsUnspecified() && !addr.IP.Equal(peer.IP) {
		reply(control, 2, "0.0.0.0:0")
		return fmt.Errorf("UDP association IP differs from TCP peer")
	}
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: local.IP})
	if err != nil {
		return err
	}
	defer conn.Close()
	if err = reply(control, 0, conn.LocalAddr().String()); err != nil {
		return err
	}
	control.SetDeadline(time.Time{})
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { conn.Close(); control.Close() })
	defer stop()
	go func() { io.Copy(io.Discard, control); cancel() }()
	type binding struct {
		flow   DatagramFlow
		cancel context.CancelFunc
	}
	var mu sync.Mutex
	flows := make(map[string]*binding)
	var wg sync.WaitGroup
	defer func() {
		cancel()
		mu.Lock()
		for _, f := range flows {
			f.cancel()
			f.flow.Close()
		}
		mu.Unlock()
		wg.Wait()
	}()
	endpoint := &net.UDPAddr{IP: peer.IP, Port: addr.Port}
	buf := make([]byte, 65536)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if !from.IP.Equal(peer.IP) || (endpoint.Port != 0 && from.Port != endpoint.Port) {
			s.Router.Metrics.Dropped.Add(1)
			continue
		}
		if n < 4 || buf[0] != 0 || buf[1] != 0 || buf[2] != 0 {
			s.Router.Metrics.Dropped.Add(1)
			continue
		} // SOCKS fragmentation is not supported.
		reader := bytes.NewReader(buf[4:n])
		target, err := readAddress(reader, buf[3])
		if err != nil {
			s.Router.Metrics.Dropped.Add(1)
			continue
		}
		payload := buf[n-reader.Len() : n]
		if endpoint.Port == 0 {
			endpoint.Port = from.Port
		}
		mu.Lock()
		f := flows[target]
		full := len(flows) >= 64
		mu.Unlock()
		if f == nil {
			if full {
				s.Router.Metrics.Dropped.Add(1)
				continue
			}
			dctx, dcancel := context.WithTimeout(ctx, 5*time.Second)
			flow, err := s.Router.OpenUDP(dctx, target)
			dcancel()
			if err != nil {
				s.Router.Metrics.Dropped.Add(1)
				continue
			}
			fctx, fcancel := context.WithCancel(ctx)
			f = &binding{flow: flow, cancel: fcancel}
			mu.Lock()
			flows[target] = f
			mu.Unlock()
			s.Router.Metrics.Active.Add(1)
			wg.Add(1)
			go func(target string, f *binding) {
				defer wg.Done()
				defer s.Router.Metrics.Active.Add(-1)
				defer f.flow.Close()
				defer f.cancel()
				// Keep a closed binding until this SOCKS association ends. Reopening
				// it on another route would silently change a live UDP session's NAT.
				header, _ := encodeAddress(target)
				for {
					payload, err := f.flow.Receive(fctx)
					if err != nil {
						return
					}
					b := append(append([]byte{0, 0, 0}, header...), payload...)
					if _, err = conn.WriteToUDP(b, endpoint); err != nil {
						s.Router.Metrics.Dropped.Add(1)
						if ctx.Err() != nil {
							return
						}
						continue
					}
					s.Router.Metrics.Datagrams.Add(1)
					s.Router.Metrics.Bytes.Add(uint64(len(payload)))
				}
			}(target, f)
		}
		if err = f.flow.Send(payload); err != nil {
			s.Router.Metrics.Dropped.Add(1)
		} else {
			s.Router.Metrics.Datagrams.Add(1)
			s.Router.Metrics.Bytes.Add(uint64(len(payload)))
		}
	}
}
