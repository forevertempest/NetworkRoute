package proxy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"networkroute/internal/config"
	"networkroute/internal/metrics"
	"networkroute/internal/monitor"
	"networkroute/internal/policy"
	"networkroute/internal/quality"
	"networkroute/internal/tunnel"
	"sync"
	"sync/atomic"
	"time"
)

type Router struct {
	Config         config.Config
	Relay          *tunnel.Client
	Engine         *quality.Engine
	Metrics        *metrics.Counters
	ctx            context.Context
	cancel         context.CancelFunc
	probes         chan string
	mu             sync.Mutex
	pending        map[string]bool
	exclusions     []netip.Prefix
	epoch          atomic.Uint64
	relaySuspended atomic.Bool
	resolver       policy.Resolver
	nativeDial     quality.Dial
}

func NewRouter(parent context.Context, c config.Config, relay *tunnel.Client) *Router {
	ctx, cancel := context.WithCancel(parent)
	r := &Router{Config: c, Relay: relay, Engine: quality.NewEngine(), Metrics: &metrics.Counters{}, ctx: ctx, cancel: cancel, probes: make(chan string, 32), pending: make(map[string]bool)}
	r.resolver = net.DefaultResolver
	r.nativeDial = quality.NativeDial
	for _, p := range c.BypassCIDRs {
		r.exclusions = append(r.exclusions, netip.MustParsePrefix(p))
	}
	r.checkVPN()
	go r.worker()
	return r
}
func (r *Router) Close() {
	r.cancel()
	if r.Relay != nil {
		r.Relay.Close()
	}
}
func (r *Router) NetworkChanged() {
	r.epoch.Add(1)
	r.Engine.Reset()
	r.checkVPN()
	slog.Info("network changed; measurements invalidated; existing sessions remain pinned")
}
func (r *Router) checkVPN() {
	if r.Config.AllowNestedTunnel {
		r.relaySuspended.Store(false)
		return
	}
	hints, err := monitor.VPNHints()
	suspend := err != nil || len(hints) > 0
	r.relaySuspended.Store(suspend)
	if suspend && r.Config.Mode != "direct" && r.Relay != nil {
		slog.Warn("possible VPN or unknown adapter state; relay suspended, existing OS route retained (fail-closed blocks eligible flows)", "vpn_hints", hints)
	}
}
func (r *Router) key(target string) string { return r.epochKey(target, r.epoch.Load()) }
func (r *Router) epochKey(target string, epoch uint64) string {
	return fmt.Sprintf("%d|tcp|%s|%s", epoch, target, r.Config.Profile)
}
func (r *Router) enqueue(target string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pending[target] {
		return
	}
	select {
	case r.probes <- target:
		r.pending[target] = true
	default:
	}
}
func (r *Router) worker() {
	for {
		select {
		case <-r.ctx.Done():
			return
		case target := <-r.probes:
			if r.Relay != nil && r.Config.Mode == "smart" && !r.relaySuspended.Load() {
				epoch := r.epoch.Load()
				// Two windows separated by the stability hold. One bounded worker prevents probe storms.
				for i := 0; i < 2 && r.ctx.Err() == nil; i++ {
					if i > 0 {
						select {
						case <-r.ctx.Done():
							return
						case <-time.After(10 * time.Second):
						}
					}
					ctx, cancel := context.WithTimeout(r.ctx, 20*time.Second)
					err := r.Relay.Connect(ctx)
					if err == nil {
						b := quality.Measure(ctx, target, 5, time.Second, quality.NativeDial, r.Relay.DialTCP)
						if epoch != r.epoch.Load() || r.relaySuspended.Load() {
							cancel()
							break
						}
						d := r.Engine.Observe(r.epochKey(target, epoch), b.Direct, b.Relay, r.Config.Profile, time.Now())
						slog.Info("route comparison completed", "route", d.Route, "reason", d.Reason, "direct_setup_ms", d.Direct.Median, "relay_setup_ms", d.Relay.Median)
					}
					cancel()
				}
			}
			r.mu.Lock()
			delete(r.pending, target)
			r.mu.Unlock()
		}
	}
}
func (r *Router) destination(ctx context.Context, target string) (string, bool, error) {
	bypass := policy.Bypass(target, r.Config.BypassDomains, r.exclusions)
	addrs, err := policy.Resolve(ctx, r.resolver, target)
	if err != nil {
		return "", false, err
	}
	// MVP uses the system resolver's first address. Full Happy Eyeballs racing is a later gate.
	numeric := addrs[0]
	bypass = bypass || policy.Bypass(numeric, nil, r.exclusions)
	if ap, err := netip.ParseAddrPort(numeric); err == nil && policy.OnLink(ap.Addr()) {
		bypass = true
	}
	return numeric, bypass, nil
}
func (r *Router) DialTCP(ctx context.Context, target string) (net.Conn, string, error) {
	addrs, err := policy.Resolve(ctx, r.resolver, target)
	if err != nil {
		return nil, "", err
	}
	if r.Config.Mode == "smart" && r.Relay == nil && r.Config.FailMode == "open" {
		delay := r.Config.DirectRaceDelayMS
		if delay == 0 {
			delay = 250
		}
		conn, err := quality.DialStaggered(ctx, addrs, time.Duration(delay)*time.Millisecond, r.nativeDial)
		if err == nil {
			r.Metrics.Direct.Add(1)
		}
		return conn, "direct", err
	}
	bypassHost := policy.Bypass(target, r.Config.BypassDomains, r.exclusions)
	var failures []error
	for i, numeric := range addrs {
		if i >= 8 || ctx.Err() != nil {
			break
		}
		bypass := bypassHost || policy.Bypass(numeric, nil, r.exclusions)
		if ap, e := netip.ParseAddrPort(numeric); e == nil && policy.OnLink(ap.Addr()) {
			bypass = true
		}
		attempt := ctx
		cancel := func() {}
		if len(addrs) > 1 {
			attempt, cancel = context.WithTimeout(ctx, 1500*time.Millisecond)
		}
		conn, route, err := r.dialOne(attempt, numeric, bypass)
		cancel()
		if err == nil {
			return conn, route, nil
		}
		failures = append(failures, err)
	}
	if len(failures) == 0 {
		return nil, "", ctx.Err()
	}
	return nil, "", errors.Join(failures...)
}
func (r *Router) dialOne(ctx context.Context, numeric string, bypass bool) (net.Conn, string, error) {
	route := "direct"
	key := r.key(numeric)
	if !bypass && r.Relay != nil && !r.relaySuspended.Load() {
		switch r.Config.Mode {
		case "relay":
			route = "relay"
		case "smart":
			if r.Config.Profile == policy.Bulk || r.Config.Profile == policy.Streaming {
				break
			} // No throughput evidence in this MVP.
			if d, ok := r.Engine.Get(key, time.Now()); ok {
				route = d.Route
				if time.Since(d.Updated) > 30*time.Second {
					r.enqueue(numeric)
				}
			} else {
				r.enqueue(numeric)
			}
		}
	}
	if route == "relay" {
		budget := 2 * time.Second
		if d, ok := ctx.Deadline(); ok && time.Until(d) < 4*time.Second {
			budget = time.Until(d) / 2
		}
		relayCtx, cancel := context.WithTimeout(ctx, budget)
		c, err := r.Relay.DialTCP(relayCtx, numeric)
		cancel()
		if err == nil {
			r.Metrics.Relayed.Add(1)
			return c, route, nil
		}
		r.Engine.Fail(key, time.Now())
		if r.Config.FailMode == "closed" {
			return nil, route, fmt.Errorf("relay unavailable; fail-closed: %w", err)
		}
		r.Metrics.Fallbacks.Add(1)
		slog.Warn("relay unavailable; new connection falling back to existing OS route")
	}
	if !bypass && r.Config.FailMode == "closed" && r.Config.Mode != "direct" {
		return nil, "blocked", fmt.Errorf("no validated relay route; fail-closed blocks this new connection")
	}
	c, err := r.nativeDial(ctx, numeric)
	if err == nil {
		r.Metrics.Direct.Add(1)
	}
	return c, "direct", err
}

type DatagramFlow interface {
	Send([]byte) error
	Receive(context.Context) ([]byte, error)
	Close() error
}
type nativeUDP struct{ conn *net.UDPConn }

func (n *nativeUDP) Send(p []byte) error {
	n.conn.SetWriteDeadline(time.Now().Add(time.Second))
	_, err := n.conn.Write(p)
	return err
}
func (n *nativeUDP) Receive(ctx context.Context) ([]byte, error) {
	deadline := time.Now().Add(2 * time.Minute)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	n.conn.SetReadDeadline(deadline)
	b := make([]byte, 65536)
	size, err := n.conn.Read(b)
	return b[:size], err
}
func (n *nativeUDP) Close() error { return n.conn.Close() }
func (r *Router) OpenUDP(ctx context.Context, target string) (DatagramFlow, error) {
	numeric, bypass, err := r.destination(ctx, target)
	if err != nil {
		return nil, err
	}
	// Never infer UDP path quality from TCP probes. Manual relay is explicit.
	if !bypass && r.Config.Mode == "relay" && r.Relay != nil && !r.relaySuspended.Load() {
		relayCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		f, err := r.Relay.OpenUDP(relayCtx, numeric)
		cancel()
		if err == nil {
			r.Metrics.Relayed.Add(1)
			return f, nil
		}
		if r.Config.FailMode == "closed" {
			return nil, err
		}
		r.Metrics.Fallbacks.Add(1)
	}
	if !bypass && r.Config.FailMode == "closed" && r.Config.Mode != "direct" {
		return nil, fmt.Errorf("UDP quality unknown; fail-closed blocks Direct")
	}
	addr, err := net.ResolveUDPAddr("udp", numeric)
	if err != nil {
		return nil, err
	}
	c, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, err
	}
	r.Metrics.Direct.Add(1)
	return &nativeUDP{conn: c}, nil
}
