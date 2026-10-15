package iron

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/quic-go/quic-go"

	"markdown.ninja/stdx/iron/base"
)

// Path over which a Connection was established.
const (
	// PathDirect means the connection uses the peer's direct address.
	PathDirect = "direct"
	// PathRelay means the connection is tunnelled through the relay.
	PathRelay = "relay"
)

// Path-maintenance tuning. Package vars so tests can shrink them.
var (
	// upgradeInterval is how often a relay connection is checked for a chance
	// to upgrade to a direct one.
	upgradeInterval = 5 * time.Second
	// upgradeGrace is how long the old relay connection stays open after an
	// upgrade, so in-flight streams can drain.
	upgradeGrace = 2 * time.Second
	// upgradeTimeout bounds one direct-dial attempt during an upgrade.
	upgradeTimeout = 5 * time.Second
	// redialRetry is how long the path watcher waits after a failed redial.
	redialRetry = 1 * time.Second
	// redialTimeout bounds one relay redial.
	redialTimeout = 5 * time.Second
)

// Connection wraps a quic.Conn and remembers which path it came up on.
//
// For connections dialed by this endpoint (see Endpoint.Connect), it also
// provides transparent fallback: if the direct path dies, the endpoint
// automatically re-dials the peer over the relay and subsequent stream and
// datagram operations run on the replacement connection. Connections opened
// *by the peer* (from Endpoint.Accept) are not auto-redialed; that side sees
// the connection close and the peer re-dialing.
type Connection struct {
	endpoint *Endpoint
	peer     base.NodeID

	redialable bool
	relayOnly  bool           // never upgrade to direct (ConnectRelayOnly / WithRelayOnly)
	lookup     connectOptions // discovered addresses / channels for path upgrades

	mu   sync.RWMutex
	cur  *quic.Conn
	path string

	closed chan struct{}
	once   sync.Once

	redialMu  sync.Mutex
	redialing bool
}

func newConnection(ep *Endpoint, peer base.NodeID, conn *quic.Conn, path string, redialable bool, relayOnly bool, lookup connectOptions) *Connection {
	c := &Connection{
		endpoint:   ep,
		peer:       peer,
		cur:        conn,
		path:       path,
		redialable: redialable,
		relayOnly:  relayOnly,
		lookup:     lookup,
		closed:     make(chan struct{}),
	}
	go c.watch()
	return c
}

// Path reports whether the connection is direct or relayed.
func (conn *Connection) Path() string {
	conn.mu.RLock()
	defer conn.mu.RUnlock()
	return conn.path
}

// PeerID returns the node id of the remote endpoint, taken from the
// authenticated TLS certificate.
func (conn *Connection) PeerID() (base.NodeID, error) {
	return peerIDFromTLS(conn.State().TLS)
}

// State returns the QUIC connection state of the current underlying
// connection.
func (conn *Connection) State() quic.ConnectionState {
	return conn.current().ConnectionState()
}

// Context returns the context of the current underlying connection. It is
// canceled when that connection closes.
func (conn *Connection) Context() context.Context {
	return conn.current().Context()
}

// Close closes the underlying connection.
func (conn *Connection) Close() error {
	conn.once.Do(func() { close(conn.closed) })
	return conn.current().CloseWithError(0, "connection closed")
}

func (conn *Connection) current() *quic.Conn {
	conn.mu.RLock()
	defer conn.mu.RUnlock()
	return conn.cur
}

// isDead reports whether the given underlying connection has closed.
func (c *Connection) isDead(conn *quic.Conn) bool {
	return conn.Context().Err() != nil
}

func (conn *Connection) errIfClosed() error {
	select {
	case <-conn.closed:
		return errors.New("connection closed")
	default:
		return nil
	}
}

// watch maintains the connection's path: it re-establishes over the relay
// whenever the current connection dies, and transparently upgrades a relay
// connection to a direct one once a direct path becomes available (e.g. after
// a successful hole punch).
func (conn *Connection) watch() {
	if !conn.redialable {
		return
	}
	upgradeTick := time.NewTicker(upgradeInterval)
	defer upgradeTick.Stop()
	for {
		cur := conn.current()
		select {
		case <-conn.closed:
			return
		case <-cur.Context().Done():
			if conn.isClosed() {
				return
			}
			// The current connection died: re-establish over the relay (the
			// endpoint reconnects the relay transport itself, so a fresh dial
			// picks up whichever relay is available).
			conn.redial()
			if conn.current() == cur {
				// Redial failed; back off a little before retrying so we do
				// not spin while the relay is down.
				select {
				case <-time.After(redialRetry):
				case <-conn.closed:
					return
				}
			}
		case <-upgradeTick.C:
			if conn.Path() == PathRelay {
				conn.upgradeToDirect()
			}
		}
	}
}

func (conn *Connection) isClosed() bool {
	select {
	case <-conn.closed:
		return true
	default:
		return false
	}
}

// redial re-establishes the connection over the relay. It is guarded so that
// concurrent callers (the path watcher and stream operations that hit a dead
// connection) do not dial more than once at a time.
func (c *Connection) redial() {
	c.redialMu.Lock()
	if c.redialing {
		c.redialMu.Unlock()
		return
	}
	c.redialing = true
	c.redialMu.Unlock()
	defer func() {
		c.redialMu.Lock()
		c.redialing = false
		c.redialMu.Unlock()
	}()

	if c.isClosed() {
		return // explicitly closed: never resurrect
	}
	cur := c.current()
	if !c.isDead(cur) {
		return // the current connection is fine; nothing to do
	}
	ctx, cancel := context.WithTimeout(context.Background(), redialTimeout)
	var conn *quic.Conn
	var path string
	if c.endpoint.currentRelayConn() != nil {
		// Re-establish over the relay (the endpoint reconnects the relay
		// transport itself, so a fresh dial picks up whatever relay is
		// available).
		var err error
		conn, err = c.endpoint.dialRelay(ctx, c.peer)
		cancel()
		if err != nil {
			return
		}
		path = PathRelay
	} else {
		// Relay-free endpoint: re-establish directly, reusing the cached
		// candidate addresses (fresh lookups are for upgrades, where the
		// peer's announced addresses may have changed).
		var err error
		conn, err = c.endpoint.tryDirect(ctx, c.peer, c.lookup)
		cancel()
		if err != nil {
			return
		}
		path = PathDirect
	}

	// Only install the replacement if the connection is still open and the
	// current connection is still the dead one we dialed for. An explicit
	// Close while we were dialing must not be resurrected, and a concurrent
	// upgradeToDirect that already moved us onto a live direct connection
	// must not be clobbered (or the fresh dial leaked).
	c.mu.Lock()
	if c.isClosed() || c.cur != cur {
		c.mu.Unlock()
		_ = conn.CloseWithError(0, "")
		return
	}
	c.cur = conn
	c.path = path
	c.mu.Unlock()
}

// upgradeToDirect transparently moves the connection from the relay to a
// direct path when one becomes available, keeping new streams flowing on the
// direct connection while the old relay connection is closed after a grace
// period so in-flight streams can drain.
func (conn *Connection) upgradeToDirect() {
	if conn.relayOnly || conn.endpoint.relayOnly || conn.endpoint.directTr == nil {
		return
	}
	conn.mu.RLock()
	if conn.path != PathRelay {
		conn.mu.RUnlock()
		return
	}
	conn.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), upgradeTimeout)
	directConn, err := conn.endpoint.tryDirectFresh(ctx, conn.peer, conn.lookup)
	cancel()
	if err != nil {
		return
	}

	// Only swap if we are still on the relay (do not clobber a newer path).
	conn.mu.Lock()
	if conn.path != PathRelay {
		conn.mu.Unlock()
		_ = directConn.CloseWithError(0, "")
		return
	}
	old := conn.cur
	conn.cur = directConn
	conn.path = PathDirect
	conn.mu.Unlock()

	if !conn.isDead(old) {
		go func() {
			select {
			case <-time.After(upgradeGrace):
			case <-conn.closed:
			}
			_ = old.CloseWithError(0, "upgraded to direct")
		}()
	}
	conn.endpoint.logger.Info("connection upgraded to direct", "peer", conn.peer.String())
}

// ensureRedial attempts the direct->relay fallback, e.g. when a stream
// operation just observed a dead connection.
func (conn *Connection) ensureRedial() { conn.redial() }

// Forwards for stream operations. Each tries once on the underlying
// connection and, if that connection just died and was replaced, once more on
// the replacement.

// OpenStream opens a new bidirectional stream.
func (conn *Connection) OpenStream() (*quic.Stream, error) {
	if err := conn.errIfClosed(); err != nil {
		return nil, err
	}
	return conn.openStream(func(conn *quic.Conn) (*quic.Stream, error) {
		return conn.OpenStream()
	})
}

// OpenStreamSync opens a new bidirectional stream, blocking until a stream
// can be opened.
func (conn *Connection) OpenStreamSync(ctx context.Context) (*quic.Stream, error) {
	if err := conn.errIfClosed(); err != nil {
		return nil, err
	}
	return conn.openStream(func(conn *quic.Conn) (*quic.Stream, error) {
		return conn.OpenStreamSync(ctx)
	})
}

func (conn *Connection) openStream(f func(*quic.Conn) (*quic.Stream, error)) (*quic.Stream, error) {
	for i := 0; i < 2; i++ {
		cur := conn.current()
		st, err := f(cur)
		if err == nil {
			return st, nil
		}
		if i == 1 || !conn.isDead(cur) {
			return nil, err
		}
		conn.ensureRedial()
	}
	return nil, errors.New("open stream failed")
}

// OpenUniStream opens a new unidirectional stream.
func (conn *Connection) OpenUniStream() (*quic.SendStream, error) {
	if err := conn.errIfClosed(); err != nil {
		return nil, err
	}
	return conn.openUniStream(func(conn *quic.Conn) (*quic.SendStream, error) {
		return conn.OpenUniStream()
	})
}

// OpenUniStreamSync opens a new unidirectional stream, blocking until a
// stream can be opened.
func (conn *Connection) OpenUniStreamSync(ctx context.Context) (*quic.SendStream, error) {
	if err := conn.errIfClosed(); err != nil {
		return nil, err
	}
	return conn.openUniStream(func(conn *quic.Conn) (*quic.SendStream, error) {
		return conn.OpenUniStreamSync(ctx)
	})
}

func (conn *Connection) openUniStream(f func(*quic.Conn) (*quic.SendStream, error)) (*quic.SendStream, error) {
	for i := 0; i < 2; i++ {
		cur := conn.current()
		st, err := f(cur)
		if err == nil {
			return st, nil
		}
		if i == 1 || !conn.isDead(cur) {
			return nil, err
		}
		conn.ensureRedial()
	}
	return nil, errors.New("open stream failed")
}

// AcceptStream accepts the next bidirectional stream opened by the peer.
func (conn *Connection) AcceptStream(ctx context.Context) (*quic.Stream, error) {
	if err := conn.errIfClosed(); err != nil {
		return nil, err
	}
	return conn.acceptStream(func(conn *quic.Conn) (*quic.Stream, error) {
		return conn.AcceptStream(ctx)
	})
}

func (conn *Connection) acceptStream(f func(*quic.Conn) (*quic.Stream, error)) (*quic.Stream, error) {
	for i := 0; i < 2; i++ {
		cur := conn.current()
		st, err := f(cur)
		if err == nil {
			return st, nil
		}
		if i == 1 || !conn.isDead(cur) {
			return nil, err
		}
		conn.ensureRedial()
	}
	return nil, errors.New("accept stream failed")
}

// AcceptUniStream accepts the next unidirectional stream opened by the peer.
func (conn *Connection) AcceptUniStream(ctx context.Context) (*quic.ReceiveStream, error) {
	if err := conn.errIfClosed(); err != nil {
		return nil, err
	}
	for i := 0; i < 2; i++ {
		cur := conn.current()
		st, err := cur.AcceptUniStream(ctx)
		if err == nil {
			return st, nil
		}
		if i == 1 || !conn.isDead(cur) {
			return nil, err
		}
		conn.ensureRedial()
	}
	return nil, errors.New("accept stream failed")
}

// SendDatagram sends an unreliable datagram.
func (conn *Connection) SendDatagram(p []byte) error {
	if err := conn.errIfClosed(); err != nil {
		return err
	}
	for i := 0; i < 2; i++ {
		cur := conn.current()
		err := cur.SendDatagram(p)
		if err == nil {
			return nil
		}
		if i == 1 || !conn.isDead(cur) {
			return err
		}
		conn.ensureRedial()
	}
	return errors.New("send datagram failed")
}

// ReceiveDatagram receives the next datagram sent by the peer.
func (conn *Connection) ReceiveDatagram(ctx context.Context) ([]byte, error) {
	if err := conn.errIfClosed(); err != nil {
		return nil, err
	}
	for i := 0; i < 2; i++ {
		cur := conn.current()
		data, err := cur.ReceiveDatagram(ctx)
		if err == nil {
			return data, nil
		}
		if i == 1 || !conn.isDead(cur) {
			return nil, err
		}
		conn.ensureRedial()
	}
	return nil, errors.New("receive datagram failed")
}

// CloseWithError closes the connection with an application error.
func (conn *Connection) CloseWithError(code uint64, desc string) error {
	conn.once.Do(func() { close(conn.closed) })
	return conn.current().CloseWithError(quic.ApplicationErrorCode(code), desc)
}
