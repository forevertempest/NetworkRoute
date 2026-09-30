// Package engine owns a running proxy independently of its CLI or console UI.
package engine

import (
	"context"
	"net"
	"networkroute/internal/config"
	"networkroute/internal/identity"
	"networkroute/internal/monitor"
	"networkroute/internal/proxy"
	"networkroute/internal/tunnel"
	"sync"
	"time"
)

type Session struct {
	Router  *proxy.Router
	Address string
	Done    chan struct{}
	cancel  context.CancelFunc
	err     error // written before Done is closed
}

func Relay(c config.Config) (*tunnel.Client, error) {
	if c.Mode == "direct" || c.Relay.Address == "" {
		return nil, nil
	}
	tls, err := identity.Config(c.Certificate, c.PrivateKey, []string{c.Relay.ServerPin}, false)
	if err != nil {
		return nil, err
	}
	return &tunnel.Client{Address: c.Relay.Address, TLS: tls}, nil
}

func Start(parent context.Context, c config.Config) (*Session, error) {
	relay, err := Relay(c)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", c.Listen)
	if err != nil {
		if relay != nil {
			relay.Close()
		}
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	router := proxy.NewRouter(ctx, c, relay)
	s := &Session{Router: router, Address: listener.Addr().String(), Done: make(chan struct{}), cancel: cancel}
	var monitorWG sync.WaitGroup
	monitorWG.Add(1)
	go func() {
		defer monitorWG.Done()
		last, _ := monitor.Fingerprint()
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				current, e := monitor.Fingerprint()
				if e == nil && current != last {
					last = current
					router.NetworkChanged()
				}
			}
		}
	}()
	go func() {
		s.err = (&proxy.Server{Router: router}).Serve(ctx, listener)
		cancel()
		listener.Close()
		router.Close()
		monitorWG.Wait()
		close(s.Done)
	}()
	return s, nil
}

func (s *Session) Stop(ctx context.Context) error {
	s.cancel()
	select {
	case <-s.Done:
		return s.err
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *Session) Err() error {
	select {
	case <-s.Done:
		return s.err
	default:
		return nil
	}
}
