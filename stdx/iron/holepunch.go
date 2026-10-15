package iron

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strconv"
	"sync"
	"time"

	"markdown.ninja/stdx/iron/base"
	"markdown.ninja/stdx/iron/stun"
)

// punchTimeout bounds how long a single Connect attempt keeps punching before
// giving up. The relay dial races this, so a failed punch never delays a
// connection.
const punchTimeout = 3 * time.Second

// punchAttempt is the per-dial budget during hole punching. Keeping it short
// makes the dialer re-send QUIC Initials frequently, so a connection goes
// through as soon as the peer's NAT mapping opens instead of waiting for
// quic-go's internal retransmission timer (~1s).
const punchAttempt = 300 * time.Millisecond

// stunRetry is how long Discover waits before re-sending a binding request.
const stunRetry = 400 * time.Millisecond

// stunInterval is how often the endpoint re-discovers its public address.
const stunInterval = 30 * time.Second

// stunTimeout bounds a single discovery round.
const stunTimeout = 3 * time.Second

// stunConn wraps a direct PacketConn so that STUN Binding responses are
// intercepted before they reach the quic-go reader, which would otherwise drop
// them as invalid QUIC packets.
type stunConn struct {
	conn net.PacketConn

	mu      sync.Mutex
	pending map[stun.TransactionID]chan *net.UDPAddr
}

func newStunConn(conn net.PacketConn) *stunConn {
	return &stunConn{conn: conn, pending: make(map[stun.TransactionID]chan *net.UDPAddr)}
}

// Discover runs a STUN Binding request to the relay's UDP endpoint from the
// underlying socket and returns the observed (post-NAT) address.
func (c *stunConn) Discover(ctx context.Context, relay *net.UDPAddr) (*net.UDPAddr, error) {
	id, err := stun.NewTransactionID()
	if err != nil {
		return nil, err
	}
	ch := make(chan *net.UDPAddr, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	req := stun.EncodeBindingRequest(id)
	if _, err := c.conn.WriteTo(req, relay); err != nil {
		return nil, err
	}
	ticker := time.NewTicker(stunRetry)
	defer ticker.Stop()
	for {
		select {
		case addr := <-ch:
			return addr, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			if _, err := c.conn.WriteTo(req, relay); err != nil {
				return nil, err
			}
		}
	}
}

// ReadFrom forwards packets to the caller, but consumes STUN Binding responses
// that match an outstanding discovery request, delivering them to that caller
// instead.
func (c *stunConn) ReadFrom(p []byte) (int, net.Addr, error) {
	for {
		n, addr, err := c.conn.ReadFrom(p)
		if err != nil {
			return n, addr, err
		}
		if n > 0 && stun.IsBindingResponse(p[:n]) {
			var id stun.TransactionID
			copy(id[:], p[8:20])
			c.mu.Lock()
			ch, ok := c.pending[id]
			c.mu.Unlock()
			if ok {
				ua, perr := stun.ParseXORMappedAddress(p[:n], id)
				if perr == nil {
					select {
					case ch <- ua:
					default: // no listener: drop
					}
				}
				continue // consumed by STUN; keep reading for quic-go
			}
		}
		return n, addr, err
	}
}

func (c *stunConn) WriteTo(p []byte, addr net.Addr) (int, error) { return c.conn.WriteTo(p, addr) }
func (c *stunConn) Close() error                                 { return c.conn.Close() }
func (c *stunConn) LocalAddr() net.Addr                          { return c.conn.LocalAddr() }
func (c *stunConn) SetDeadline(t time.Time) error                { return c.conn.SetDeadline(t) }
func (c *stunConn) SetReadDeadline(t time.Time) error            { return c.conn.SetReadDeadline(t) }
func (c *stunConn) SetWriteDeadline(t time.Time) error           { return c.conn.SetWriteDeadline(t) }

// SetReadBuffer and SetWriteBuffer forward to the underlying connection when
// possible, so quic-go can still size the socket buffers.
func (c *stunConn) SetReadBuffer(n int) error {
	if bc, ok := c.conn.(interface{ SetReadBuffer(int) error }); ok {
		return bc.SetReadBuffer(n)
	}
	return nil
}

func (c *stunConn) SetWriteBuffer(n int) error {
	if bc, ok := c.conn.(interface{ SetWriteBuffer(int) error }); ok {
		return bc.SetWriteBuffer(n)
	}
	return nil
}

var _ net.PacketConn = (*stunConn)(nil)

// relayUDPAddr resolves the UDP address of the relay's STUN endpoint, which
// by convention shares host and port with the WebSocket relay.
func relayUDPAddr(relayURL string) (*net.UDPAddr, error) {
	u, err := url.Parse(relayURL)
	if err != nil {
		return nil, err
	}
	port := u.Port()
	switch u.Scheme {
	case "http":
		if port == "" {
			port = "80"
		}
	case "https":
		if port == "" {
			port = "443"
		}
	default:
		return nil, errors.New("invalid relay url scheme: want http or https")
	}
	return net.ResolveUDPAddr("udp", net.JoinHostPort(u.Hostname(), port))
}

// discoverPublicAddr periodically asks the relay's STUN endpoint what public
// UDP address it observes for us, announcing it once known and re-announcing
// on change. STUN must go out the direct socket so the answer reflects the
// NAT mapping of the socket peers punch to.
func (endpoint *Endpoint) discoverPublicAddr() {
	if endpoint.relayOnly || endpoint.stun == nil || endpoint.currentRelayConn() == nil {
		return
	}
	target, err := relayUDPAddr(endpoint.currentRelayURL())
	if err != nil {
		return
	}
	ticker := time.NewTicker(stunInterval)
	defer ticker.Stop()
	for {
		ctx, cancel := context.WithTimeout(context.Background(), stunTimeout)
		addr, err := endpoint.stun.Discover(ctx, target)
		cancel()
		if err == nil && addr != nil && addr.IP != nil {
			changed := endpoint.setPublicAddr(addr)
			if changed {
				endpoint.logger.Info("discovered public address", "addr", addr.String())
				endpoint.publishEndpoints()
			}
		}
		select {
		case <-endpoint.closeCh:
			return
		case <-ticker.C:
		}
	}
}

func (endpoint *Endpoint) setPublicAddr(addr *net.UDPAddr) bool {
	endpoint.publicMu.Lock()
	defer endpoint.publicMu.Unlock()
	if endpoint.public != nil && endpoint.public.IP.Equal(addr.IP) && endpoint.public.Port == addr.Port {
		return false
	}
	endpoint.public = addr
	return true
}

// PublicAddr returns this endpoint's public UDP address as observed by the
// relay, or nil until the first successful discovery.
func (endpoint *Endpoint) PublicAddr() *net.UDPAddr {
	endpoint.publicMu.RLock()
	defer endpoint.publicMu.RUnlock()
	return endpoint.public
}

// handleHolePunch is invoked when the relay tells us a peer wants a direct
// connection with us. We answer by probing the peer's addresses from our
// direct socket, opening our NAT mapping so the peer's connection attempts can
// reach us (simultaneous open).
func (endpoint *Endpoint) handleHolePunch(peer base.NodeID, addrs []*net.UDPAddr) {
	if endpoint.relayOnly || endpoint.directConn == nil {
		return
	}
	for _, a := range addrs {
		endpoint.probe(a)
	}
}

// probe sends a few small UDP datagrams to addr from our direct socket. A NAT
// creates a mapping towards addr when we send, which is what lets the peer's
// packets reach us. The datagrams are not QUIC packets; quic-go drops them.
func (endpoint *Endpoint) probe(addr *net.UDPAddr) {
	pc := endpoint.directConn
	if pc == nil {
		return
	}
	buf := []byte{0x00}
	for i := 0; i < 3; i++ {
		if endpoint.isClosing() {
			return
		}
		if _, err := pc.WriteTo(buf, addr); err != nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// punchCandidates orders the addresses to try for a direct connection: the
// peer's relay-observed public address first (that is the hole-punching
// target), then the announced addresses that look reachable from here.
func (endpoint *Endpoint) punchCandidates(addrs []*net.UDPAddr, observed *net.UDPAddr) []*net.UDPAddr {
	var out []*net.UDPAddr
	seen := make(map[string]bool)
	add := func(a *net.UDPAddr) {
		if a == nil {
			return
		}
		key := addrKey(a)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, a)
	}
	add(observed)
	for _, a := range endpoint.filterCandidates(addrs) {
		add(a)
	}
	return out
}

func addrKey(a *net.UDPAddr) string {
	return a.IP.String() + ":" + strconv.Itoa(a.Port)
}

var errNoDirectCandidates = errors.New("no direct candidates")
