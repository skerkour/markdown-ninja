// Package relay implements the client side of the relay protocol.
//
// The relay data plane carries opaque QUIC packets: each packet is wrapped in
// a relay datagram framed with the destination (or source) NodeID. RelayConn
// presents that tunnel as a net.PacketConn, so a quic.Transport can treat the
// relay exactly like a UDP socket while quic-go keeps routing packets to the
// right connection via the QUIC connection IDs.
package relay

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/zeebo/blake3"

	"markdown.ninja/stdx/iron/base"
	"markdown.ninja/stdx/iron/proto"
)

const relayPath = "/relay"

// validateRelayURL checks that url is a well-formed relay URL with an http or
// https scheme. ws/wss URLs are rejected: the canonical iron relay address is
// http(s):// (the WebSocket connection is derived from it internally).
func validateRelayURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("relay: invalid relay url: " + err.Error())
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("relay: invalid relay url scheme " + strconv.Quote(u.Scheme) + ": want http or https")
	}
	if u.Host == "" {
		return errors.New("relay: invalid relay url: missing host")
	}
	return nil
}

// Default batching parameters.
const (
	defaultBatchSize  = 16 * 1024
	defaultBatchCount = 16
	defaultDrainDelay = 500 * time.Microsecond
)

// Keepalive tuning. A connection is considered dead when no frame at all has
// been received within pingTimeout; the peer is pinged every pingInterval to
// detect that. Package vars so tests can shrink them.
var (
	pingInterval = 15 * time.Second
	pingTimeout  = 45 * time.Second
)

// RelayConn is the relay transport: a net.PacketConn whose "remote address"
// is a base.PeerAddr (a NodeID). It is safe for concurrent use.
type RelayConn struct {
	ws        *websocket.Conn
	ctx       context.Context
	cancel    context.CancelFunc
	localAddr net.Addr

	writeMu sync.Mutex // serializes outbound websocket writes

	closed chan struct{} // closed when the read loop exits

	// relayMu guards relayURLs: the backbone relay URL to route a given peer's
	// packets through, when the peer is on a different relay.
	relayMu   sync.RWMutex
	relayURLs map[base.NodeID]string

	q              chan *batchEntry // incoming batches for ReadFrom
	readDeadlineMu sync.Mutex
	readDeadline   time.Time
	deadlineCh     chan struct{} // closed+replaced whenever the deadline changes

	readMu sync.Mutex
	cur    *batchEntry // batch currently being served across ReadFrom calls

	observed net.IP // our address as seen by the relay (from ObservedAddr)

	holePunchHandler HolePunchHandler // inbound relay-assisted punch requests

	batchSize  int // max bytes per batch frame (<=0 disables batching)
	batchCount int // max packets per batch frame
	drainDelay time.Duration

	batchMu sync.Mutex
	pending map[batchKey]*batch // buffered outbound packets

	pingInterval time.Duration // keepalive: ping period
	pingTimeout  time.Duration // keepalive: silence threshold

	lastRecvNano atomic.Int64 // unixnano of the last received frame (0 = none)
	lastRTTNano  atomic.Int64 // most recent ping/pong round-trip (0 = none)
}

// batchEntry is one receive-side batch: QUIC packets of possibly varying sizes
// from a single peer, served to the consumer one at a time.
type batchEntry struct {
	remote  base.NodeID
	ecn     byte
	packets [][]byte
	idx     int // consumer cursor: next packet index
}

// serve copies the next packet into p and advances the cursor. It returns the
// bytes copied, or 0 when the batch is exhausted. quic-go passes a
// MaxPacketBufferSize buffer, so packets never exceed it.
func (e *batchEntry) serve(p []byte) int {
	if e.idx >= len(e.packets) {
		return 0
	}
	pkt := e.packets[e.idx]
	e.idx++
	return copy(p, pkt)
}

// Option configures a relay connection.
type Option func(*options)

type options struct {
	batchSize         int
	batchCount        int
	drainDelay        time.Duration
	keepaliveInterval time.Duration
	keepaliveTimeout  time.Duration
	holePunchHandler  HolePunchHandler
	announceAddrs     []string
}

// HolePunchHandler is called when the relay asks this client to punch to a
// target node: it receives the target's announced direct addresses.
type HolePunchHandler func(target base.NodeID, addrs []*net.UDPAddr)

// WithHolePunchHandler sets the handler for inbound relay-assisted hole punch
// requests.
func WithHolePunchHandler(h HolePunchHandler) Option {
	return func(o *options) { o.holePunchHandler = h }
}

// WithAnnounceAddrs sets the direct addresses this client publishes to the
// relay during the handshake, so peers can find it. Empty means none.
func WithAnnounceAddrs(addrs []string) Option {
	return func(o *options) { o.announceAddrs = append([]string(nil), addrs...) }
}

// WithBatch configures outbound relay batching. QUIC packets destined for the
// same peer are coalesced into a single batch frame, regardless of packet
// size. batchSize is the max payload bytes per frame, batchCount the max
// packets per frame, and drainDelay bounds how long a partial batch is kept
// before being flushed. A batchSize <= 0 disables batching entirely.
func WithBatch(batchSize, batchCount int, drainDelay time.Duration) Option {
	return func(o *options) {
		o.batchSize = batchSize
		o.batchCount = batchCount
		o.drainDelay = drainDelay
	}
}

// WithKeepalive overrides the keepalive tuning: the relay is pinged every
// interval and the connection is declared dead when nothing has been received
// for timeout. Defaults: 15s / 45s.
func WithKeepalive(interval, timeout time.Duration) Option {
	return func(o *options) {
		o.keepaliveInterval = interval
		o.keepaliveTimeout = timeout
	}
}

// batchKey groups buffered packets by destination and backbone relay (batches
// may mix packet sizes, so the size is no longer part of the key).
type batchKey struct {
	dst   base.NodeID
	relay string
}

type batch struct {
	packets [][]byte
	count   int
	bytes   int // total payload bytes, for the batchSize limit
}

// Dial connects to the relay at ws(s)://host/relay, authenticates with the
// challenge/signature handshake and returns a ready-to-use transport.
func Dial(ctx context.Context, url string, secret *base.NodeSecret, opts ...Option) (*RelayConn, error) {
	var option options
	for _, optionFn := range opts {
		optionFn(&option)
	}
	if option.batchSize == 0 {
		option.batchSize = defaultBatchSize
	}
	if option.batchCount == 0 {
		option.batchCount = defaultBatchCount
	}
	if option.drainDelay == 0 {
		option.drainDelay = defaultDrainDelay
	}
	if option.keepaliveInterval == 0 {
		option.keepaliveInterval = pingInterval
	}
	if option.keepaliveTimeout == 0 {
		option.keepaliveTimeout = pingTimeout
	}
	if err := validateRelayURL(url); err != nil {
		return nil, err
	}

	ws, _, err := websocket.Dial(ctx, url+relayPath, &websocket.DialOptions{
		Subprotocols: []string{proto.RelayProtocol},
	})
	if err != nil {
		return nil, err
	}
	if ws.Subprotocol() != proto.RelayProtocol {
		ws.Close(websocket.StatusPolicyViolation, "unsupported relay protocol")
		return nil, errors.New("relay: unsupported subprotocol")
	}
	observedIP, err := clientHandshake(ctx, ws, secret, option.announceAddrs)
	if err != nil {
		ws.Close(websocket.StatusPolicyViolation, err.Error())
		return nil, err
	}

	cctx, cancel := context.WithCancel(context.Background())
	c := &RelayConn{
		ws:         ws,
		ctx:        cctx,
		cancel:     cancel,
		localAddr:  &net.UDPAddr{IP: net.IPv6loopback},
		closed:     make(chan struct{}),
		q:          make(chan *batchEntry, 1024),
		deadlineCh: make(chan struct{}),
		observed:   observedIP,
		batchSize:  option.batchSize,
		batchCount: option.batchCount,
		drainDelay: option.drainDelay,
		pending:    make(map[batchKey]*batch),
		relayURLs:  make(map[base.NodeID]string),

		holePunchHandler: option.holePunchHandler,

		pingInterval: option.keepaliveInterval,
		pingTimeout:  option.keepaliveTimeout,
	}
	c.lastRecvNano.Store(time.Now().UnixNano()) // base for the keepalive watchdog
	go c.readLoop()
	go c.flusher()
	go c.watchdog()
	return c, nil
}

// ObservedIP returns this client's address as seen by the relay (its public
// IP, or its LAN IP when the relay is local). It is nil until the relay has
// reported it, and is used to recognize candidates that point at our own NAT.
func (conn *RelayConn) ObservedIP() net.IP {
	return conn.observed
}

// PingRTT returns the most recent relay round-trip time, or 0 if no pong has
// been received yet. Useful for relay selection.
func (conn *RelayConn) PingRTT() time.Duration {
	return time.Duration(conn.lastRTTNano.Load())
}

// scheduleRestart honors the relay's restart advisory: after reconnectIn the
// connection is torn down (surfacing it to the watcher via Closed) so the
// endpoint reconnects, possibly to another relay. The delay is capped so a
// misbehaving relay cannot stall us indefinitely.
func (conn *RelayConn) scheduleRestart(d time.Duration) {
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	time.AfterFunc(d, func() {
		conn.ws.CloseNow()
		close(conn.closed)
	})
}

// clientHandshake runs the relay auth protocol with exactly three messages:
// it receives the server's ServerHello challenge, sends a ClientHello carrying
// its identity, signature and announced addresses, and awaits Finished. It
// returns the relay-reported observed IP (nil if the relay did not report one).
func clientHandshake(ctx context.Context, ws *websocket.Conn, secret *base.NodeSecret, announce []string) (net.IP, error) {
	wsMessageType, wsMessage, err := ws.Read(ctx)
	if err != nil {
		return nil, err
	}
	if wsMessageType != websocket.MessageBinary {
		return nil, errors.New("relay: expected binary server hello")
	}
	var hello proto.ServerHello
	if err := proto.Unmarshal(wsMessage, &hello); err != nil {
		return nil, err
	}

	var key [32]byte
	blake3.DeriveKey(proto.HandshakeDomainSep, hello.Challenge[:], key[:])
	sig := secret.Sign(key[:])
	auth, err := proto.Encode(proto.ClientHello{ID: secret.Public(), Sig: sig, Addrs: announce})
	if err != nil {
		return nil, err
	}
	if err := ws.Write(ctx, websocket.MessageBinary, auth); err != nil {
		return nil, err
	}

	wsMessageType, wsMessage, err = ws.Read(ctx)
	if err != nil {
		return nil, err
	}
	if wsMessageType != websocket.MessageBinary {
		return nil, errors.New("relay: expected finished")
	}
	var finished proto.Finished
	if err := proto.Unmarshal(wsMessage, &finished); err != nil {
		return nil, err
	}
	if !finished.Result {
		return nil, errors.New("relay: authentication denied")
	}
	return finished.Observed, nil
}

// readLoop decodes incoming frames and dispatches datagrams to ReadFrom. It
// also answers relay keep-alive pings, as the protocol requires.
func (conn *RelayConn) readLoop() {
	restarting := false // a restart advisory owns closing c.closed
	defer func() {
		if !restarting {
			close(conn.closed)
		}
	}()
	for {
		wsMessageType, wsMessage, err := conn.ws.Read(conn.ctx)
		if err != nil {
			return
		}
		conn.lastRecvNano.Store(time.Now().UnixNano())
		if wsMessageType != websocket.MessageBinary {
			continue
		}
		var v any
		if err := proto.Unmarshal(wsMessage, &v); err != nil {
			continue
		}
		switch m := v.(type) {
		case proto.RelayToClientBatch:
			select {
			case conn.q <- &batchEntry{remote: m.Remote, ecn: m.Ecn, packets: m.Packets}:
			default: // queue full: drop, like a UDP socket would
			}
		case proto.Ping:
			frame, err := proto.Encode(proto.Pong{Nonce: m.Nonce})
			if err == nil {
				_ = conn.writeFrame(frame)
			}
		case proto.Pong:
			sent := int64(binary.LittleEndian.Uint64(m.Nonce[:]))
			conn.lastRTTNano.Store(time.Now().UnixNano() - sent)
		case proto.Restarting:
			// The relay is restarting: surface the outage only after the
			// advised delay, so clients reconnect in a smear rather than all
			// at once.
			restarting = true
			conn.scheduleRestart(m.ReconnectAfter)
			return
		case proto.HolePunch:
			if conn.holePunchHandler != nil {
				addrs := make([]*net.UDPAddr, 0, len(m.Addrs))
				for _, s := range m.Addrs {
					if a, err := net.ResolveUDPAddr("udp", s); err == nil {
						addrs = append(addrs, a)
					}
				}
				conn.holePunchHandler(m.Target, addrs)
			}
		}
	}
}

func (conn *RelayConn) writeFrame(frame []byte) error {
	conn.writeMu.Lock()
	defer conn.writeMu.Unlock()
	return conn.ws.Write(conn.ctx, websocket.MessageBinary, frame)
}

// RequestHolePunch asks the relay to coordinate a direct connection to peer:
// the relay notifies both the peer and us with each other's direct addresses,
// so both sides can punch through their NATs simultaneously.
func (conn *RelayConn) RequestHolePunch(peer base.NodeID) error {
	frame, err := proto.Encode(proto.HolePunchRequest{Target: peer})
	if err != nil {
		return err
	}
	return conn.writeFrame(frame)
}

// SetPeerRelay tells the relay where the given peer lives. When set, outbound
// packets for peer are tagged with relayURL so the relaying server can forward
// them over the backbone to that relay. An empty URL clears the hint (the peer
// is assumed to be on this relay).
func (conn *RelayConn) SetPeerRelay(peer base.NodeID, relayURL string) {
	conn.relayMu.Lock()
	if relayURL == "" {
		delete(conn.relayURLs, peer)
	} else {
		conn.relayURLs[peer] = relayURL
	}
	conn.relayMu.Unlock()
}

// UpdateAnnounceAddrs publishes a refreshed set of direct addresses to the
// relay, updating the directory entry tied to this live connection. An empty
// slice clears the announced addresses.
func (conn *RelayConn) UpdateAnnounceAddrs(addrs []string) error {
	frame, err := proto.Encode(proto.LocalAddrs{Addrs: addrs})
	if err != nil {
		return err
	}
	return conn.writeFrame(frame)
}

// WriteTo tunnels a QUIC packet destined for the peer in addr. Unless
// batching is disabled, packets are buffered briefly and sent as a batch
// frame to cut per-packet overhead.
func (conn *RelayConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	pa, ok := addr.(base.PeerAddr)
	if !ok {
		return 0, errors.New("relay: write to a non-peer address")
	}
	conn.relayMu.RLock()
	relay := conn.relayURLs[pa.ID]
	conn.relayMu.RUnlock()
	if conn.batchSize <= 0 {
		frame, err := proto.Encode(proto.ClientToRelayBatch{
			Remote: pa.ID, Relay: relay, Packets: [][]byte{p},
		})
		if err != nil {
			return 0, err
		}
		if err := conn.writeFrame(frame); err != nil {
			return 0, err
		}
		return len(p), nil
	}
	conn.enqueue(pa.ID, relay, p)
	return len(p), nil
}

// enqueue buffers a packet into its destination's bucket, flushing immediately
// once a bucket reaches the configured size or count limits. The packet bytes
// are copied: quic-go may reuse the buffer it passed to WriteTo as soon as the
// call returns, and we only marshal the batch later.
func (conn *RelayConn) enqueue(dst base.NodeID, relay string, p []byte) {
	key := batchKey{dst: dst, relay: relay}
	pkt := append([]byte(nil), p...)
	conn.batchMu.Lock()
	b := conn.pending[key]
	if b == nil {
		b = &batch{}
		conn.pending[key] = b
	}
	b.packets = append(b.packets, pkt)
	b.count++
	b.bytes += len(pkt)
	full := b.count >= conn.batchCount || b.bytes >= conn.batchSize
	conn.batchMu.Unlock()
	if full {
		conn.flushPending()
	}
}

// flusher periodically drains buffered packets into batch frames.
func (conn *RelayConn) flusher() {
	tick := time.NewTicker(conn.drainDelay)
	defer tick.Stop()
	for {
		select {
		case <-conn.ctx.Done():
			return // Close() flushes synchronously before cancelling
		case <-tick.C:
			conn.flushPending()
		}
	}
}

// watchdog pings the relay periodically and force-closes the connection if no
// frame at all has been received for pingTimeout, so a silently-dead relay is
// detected and surfaced via Closed().
func (conn *RelayConn) watchdog() {
	tick := time.NewTicker(conn.pingInterval)
	defer tick.Stop()
	for {
		select {
		case <-conn.ctx.Done():
			return
		case <-tick.C:
			var p [8]byte
			binary.LittleEndian.PutUint64(p[:], uint64(time.Now().UnixNano()))
			frame, err := proto.Encode(proto.Ping{Nonce: p})
			if err == nil {
				_ = conn.writeFrame(frame)
			}
			if time.Since(time.Unix(0, conn.lastRecvNano.Load())) > conn.pingTimeout {
				conn.ws.CloseNow()
				return
			}
		}
	}
}

// flushPending sends all buffered packets as batch frames.
func (conn *RelayConn) flushPending() {
	conn.batchMu.Lock()
	if len(conn.pending) == 0 {
		conn.batchMu.Unlock()
		return
	}
	pending := conn.pending
	conn.pending = make(map[batchKey]*batch)
	conn.batchMu.Unlock()

	for k, b := range pending {
		frame, err := proto.Encode(proto.ClientToRelayBatch{
			Remote:  k.dst,
			Relay:   k.relay,
			Packets: b.packets,
		})
		if err == nil {
			_ = conn.writeFrame(frame)
		}
	}
}

// ReadFrom returns the next QUIC packet received from a peer. The returned
// addr is a base.PeerAddr carrying the sender's NodeID. Packets are served one
// at a time from the current batch (see batchEntry), so a batch frame is
// coalesced into a single queue entry while still satisfying quic-go's
// one-packet-per-call ReadFrom contract.
func (conn *RelayConn) ReadFrom(p []byte) (int, net.Addr, error) {
	for {
		conn.readMu.Lock()
		cur := conn.cur
		conn.readMu.Unlock()

		if cur != nil {
			if n := cur.serve(p); n > 0 {
				return n, base.PeerAddr{ID: cur.remote}, nil
			}
			// Batch exhausted: fetch the next one.
			conn.readMu.Lock()
			conn.cur = nil
			conn.readMu.Unlock()
		}

		conn.readDeadlineMu.Lock()
		deadlineCh := conn.deadlineCh
		var fired <-chan time.Time
		if !conn.readDeadline.IsZero() {
			// A deadline in the past fires immediately; a future one fires
			// after waiting. time.After's timer is GC-recovered when abandoned
			// (Go 1.23+), so a per-evaluation timer is cheap.
			wait := time.Until(conn.readDeadline)
			if wait < 0 {
				wait = 0
			}
			fired = time.After(wait)
		}
		conn.readDeadlineMu.Unlock()

		select {
		case <-conn.closed:
			return 0, nil, io.EOF
		case d := <-conn.q:
			conn.readMu.Lock()
			conn.cur = d
			conn.readMu.Unlock()
		case <-deadlineCh: // deadline changed: re-evaluate
		case <-fired:
			// A computed timer can fire after the deadline was extended
			// concurrently; only report it if it is still in the past.
			conn.readDeadlineMu.Lock()
			past := !conn.readDeadline.IsZero() && time.Now().After(conn.readDeadline)
			conn.readDeadlineMu.Unlock()
			if past {
				return 0, nil, os.ErrDeadlineExceeded
			}
			// Deadline extended: re-evaluate with the new deadline.
		}
	}
}

// SetDeadline sets the read deadline (write deadlines are a no-op).
func (conn *RelayConn) SetDeadline(t time.Time) error { return conn.SetReadDeadline(t) }

// SetReadDeadline sets the read deadline. A change immediately wakes any
// blocked ReadFrom so quic-go can shut the listener down.
func (conn *RelayConn) SetReadDeadline(t time.Time) error {
	conn.readDeadlineMu.Lock()
	conn.readDeadline = t
	close(conn.deadlineCh)
	conn.deadlineCh = make(chan struct{})
	conn.readDeadlineMu.Unlock()
	return nil
}

// SetWriteDeadline is a no-op.
func (conn *RelayConn) SetWriteDeadline(time.Time) error { return nil }

// SetReadBuffer and SetWriteBuffer satisfy quic-go's optional buffer-resizing
// hook; the relay's backpressure is bounded by its own queue instead.
func (conn *RelayConn) SetReadBuffer(int) error  { return nil }
func (conn *RelayConn) SetWriteBuffer(int) error { return nil }

// Close flushes any buffered packets, then tears down the relay connection.
func (conn *RelayConn) Close() error {
	conn.flushPending()
	conn.cancel()
	return conn.ws.CloseNow()
}

// Closed returns a channel that is closed when the relay connection dies
// (read loop exits), e.g. on a network error or an explicit Close.
func (conn *RelayConn) Closed() <-chan struct{} {
	return conn.closed
}

// LocalAddr returns the local address of the relay connection.
func (conn *RelayConn) LocalAddr() net.Addr { return conn.localAddr }

var _ net.PacketConn = (*RelayConn)(nil)
