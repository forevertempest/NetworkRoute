package tunnel

import (
	"context"
	"crypto/tls"
	"fmt"
	quic "github.com/quic-go/quic-go"
	"io"
	"net"
	"networkroute/internal/protocol"
	"sync"
	"sync/atomic"
	"time"
)

type Client struct {
	Address    string
	TLS        *tls.Config
	mu         sync.Mutex
	conn       *quic.Conn
	flows      map[uint64]*UDPFlow
	closed     bool
	connecting chan struct{}
}

func (c *Client) Connect(ctx context.Context) error { _, err := c.connection(ctx); return err }
func (c *Client) connection(ctx context.Context) (*quic.Conn, error) {
	for {
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return nil, net.ErrClosed
		}
		if c.conn != nil && c.conn.Context().Err() == nil {
			q := c.conn
			c.mu.Unlock()
			return q, nil
		}
		if pending := c.connecting; pending != nil {
			c.mu.Unlock()
			select {
			case <-pending:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		c.connecting = make(chan struct{})
		c.mu.Unlock()
		dialCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		q, err := quic.DialAddr(dialCtx, c.Address, c.TLS, quicConfig())
		cancel()
		c.mu.Lock()
		close(c.connecting)
		c.connecting = nil
		if c.closed {
			if q != nil {
				q.CloseWithError(0, "shutdown")
			}
			c.mu.Unlock()
			return nil, net.ErrClosed
		}
		if err != nil {
			c.mu.Unlock()
			return nil, fmt.Errorf("relay QUIC handshake failed (3s limit): %w", err)
		}
		c.conn = q
		c.flows = make(map[uint64]*UDPFlow)
		c.mu.Unlock()
		go c.receive(q)
		return q, nil
	}
}
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.conn != nil {
		return c.conn.CloseWithError(0, "shutdown")
	}
	return nil
}
func (c *Client) open(ctx context.Context, network, target string) (*StreamConn, protocol.Response, error) {
	var response protocol.Response
	q, err := c.connection(ctx)
	if err != nil {
		return nil, response, err
	}
	s, err := q.OpenStreamSync(ctx)
	if err != nil {
		return nil, response, err
	}
	conn := &StreamConn{Stream: s, conn: q}
	stop := context.AfterFunc(ctx, func() { s.CancelRead(1); s.CancelWrite(1) })
	defer stop()
	deadline := time.Now().Add(5 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	s.SetDeadline(deadline)
	if err = protocol.Write(s, protocol.Request{Version: protocol.Version, Network: network, Target: target}); err == nil {
		err = protocol.Read(s, &response)
	}
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil || !response.OK {
		conn.Close()
		if err == nil {
			err = fmt.Errorf("relay refused flow: %s", response.Error)
		}
		return nil, response, err
	}
	s.SetDeadline(time.Time{})
	return conn, response, nil
}
func (c *Client) DialTCP(ctx context.Context, target string) (net.Conn, error) {
	s, _, err := c.open(ctx, "tcp", target)
	if err != nil {
		return nil, err
	}
	return s, nil
}

type UDPFlow struct {
	ID       uint64
	control  *StreamConn
	client   *Client
	incoming chan []byte
	once     sync.Once
	sequence atomic.Uint32
	done     chan struct{}
}

func (c *Client) OpenUDP(ctx context.Context, target string) (*UDPFlow, error) {
	s, r, err := c.open(ctx, "udp", target)
	if err != nil {
		return nil, err
	}
	if !s.conn.ConnectionState().SupportsDatagrams.Remote {
		s.Close()
		return nil, fmt.Errorf("relay did not negotiate QUIC DATAGRAM")
	}
	f := &UDPFlow{ID: r.FlowID, control: s, client: c, incoming: make(chan []byte, 64), done: make(chan struct{})}
	c.mu.Lock()
	if c.conn != s.conn {
		c.mu.Unlock()
		s.Close()
		return nil, fmt.Errorf("relay connection changed")
	}
	c.flows[f.ID] = f
	c.mu.Unlock()
	go func() { var b [1]byte; io.ReadFull(s, b[:]); f.Close(); close(f.done) }()
	return f, nil
}
func (f *UDPFlow) Send(p []byte) error {
	frames, err := protocol.Fragments(f.ID, f.sequence.Add(1), p)
	if err != nil {
		return err
	}
	for _, b := range frames {
		if err = f.control.conn.SendDatagram(b); err != nil {
			return err
		}
	}
	return nil
}
func (f *UDPFlow) Receive(ctx context.Context) ([]byte, error) {
	select {
	case p := <-f.incoming:
		return p, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-f.control.conn.Context().Done():
		return nil, net.ErrClosed
	case <-f.done:
		return nil, net.ErrClosed
	}
}
func (f *UDPFlow) Close() error {
	f.once.Do(func() {
		f.client.mu.Lock()
		if f.client.flows[f.ID] == f {
			delete(f.client.flows, f.ID)
		}
		f.client.mu.Unlock()
		f.control.Close()
	})
	return nil
}
func (c *Client) receive(q *quic.Conn) {
	var reassembly protocol.Reassembler
	for {
		b, err := q.ReceiveDatagram(q.Context())
		if err != nil {
			return
		}
		id, p, complete, err := reassembly.Push(b, time.Now())
		if err != nil || !complete {
			continue
		}
		c.mu.Lock()
		if c.conn == q {
			if f := c.flows[id]; f != nil {
				select {
				case f.incoming <- p:
				default:
				}
			}
		}
		c.mu.Unlock()
	}
}
