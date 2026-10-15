// Package iron implements "dial by public key" networking on top of QUIC.
//
// Every node keeps a persistent connection to a relay and announces the
// direct addresses it is reachable at. To reach another node you dial its
// NodeID: the node tries the peer's direct addresses first and falls back to
// connecting through the relay. Connections are authenticated by the node's
// Ed25519 identity carried in self-signed X.509 certificates.
package iron

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/quic-go/quic-go"

	"markdown.ninja/stdx/iron/base"
	"markdown.ninja/stdx/iron/discovery"
	"markdown.ninja/stdx/iron/proto"
	"markdown.ninja/stdx/iron/relay"
)

// directAttemptTimeout caps how long one direct-dial attempt may take before
// trying the next candidate or falling back to the relay.
const directAttemptTimeout = 2 * time.Second

// relayHeadStart delays the relay dial so a reachable direct connection (or a
// successful hole punch) wins the race deterministically. A peer that is only
// reachable through the relay is still connected within this budget, which is
// far faster than the old sequential fallback that burned up to
// directAttemptTimeout per candidate.
const relayHeadStart = 400 * time.Millisecond

// Endpoint is a node. It maintains a relay connection, listens for direct UDP
// connections, and can accept inbound QUIC connections and dial outbound
// ones, both identified by NodeID.
type Endpoint struct {
	secret *base.NodeSecret
	logger *slog.Logger

	relayURL   string
	relays     []string     // all configured relay URLs, best (current) first
	relayMu    sync.RWMutex // guards relayConn/relayTr (replaced on reconnect)
	relayConn  *relay.RelayConn
	relayTr    *quic.Transport
	directTr   *quic.Transport
	directConn net.PacketConn // the direct UDP socket (wrapped for STUN)
	directAddr *net.UDPAddr   // our UDP socket, published as a candidate
	stun       *stunConn      // STUN interception on the direct socket
	relayOnly  bool           // no p2p: only ever connect through the relay

	announcers []discovery.Announcer // channels to announce to (besides the relay)

	publicMu sync.RWMutex
	public   *net.UDPAddr // our public UDP address, from relay STUN discovery

	relayWaitTimeout time.Duration // how long Connect waits for the relay to come back

	tlsCfg TLSConfig // KeyExchange group preferences for all peer connections

	relayBatchSize  int // batching config, preserved across reconnects
	relayBatchCount int
	relayDrainDelay time.Duration

	announceMu sync.Mutex
	announced  []*net.UDPAddr // override set via SetAnnouncedAddrs, else nil

	peerAddrs sync.Map // NodeID -> *candidateEntry (cached direct addresses)

	conns    chan *quic.Conn
	closeCh  chan struct{}
	closeOne sync.Once
}

// NewEndpoint binds a node to a UDP socket for direct connections, and, when
// relays are configured via WithRelayURLs, to a relay. With no relays the
// endpoint is relay-free: it only connects directly (via announced addresses,
// Discoverers or ConnectAddr) and never dials or accepts through a relay.
//
// The alpn parameter is kept for API compatibility but is no longer placed on
// the wire: peer connections advertise HTTP/3 ("h3") by default so traffic
// blends in with ordinary web HTTP/3. Configure a different ALPN, cipher
// suites, key exchange groups or SNI hostnames via WithTLSConfig.
func NewEndpoint(ctx context.Context, secret *base.NodeSecret, alpn string, opts ...EndpointOption) (*Endpoint, error) {
	var options endpointOptions
	for _, optionFn := range opts {
		optionFn(&options)
	}

	relays := append([]string(nil), options.relays...)
	if options.relayOnly && len(relays) == 0 {
		return nil, errors.New("WithRelayOnly requires at least one relay")
	}
	if options.logger == nil {
		options.logger = slog.New(slog.DiscardHandler)
	}

	// Bind the direct UDP socket (IPv6 with V6ONLY=0, so both IPv4 and IPv6
	// direct connections work from a single socket). In relay-only mode no
	// direct socket is opened at all. The socket is bound before the relay is
	// dialed so the endpoint value below is fully initialized before the relay
	// client's read-loop goroutine starts, which can dispatch an inbound hole
	// punch into endpoint.handleHolePunch as soon as the dial returns.
	var directConn net.PacketConn
	var directAddr *net.UDPAddr
	ownedDirect := false
	switch {
	case options.relayOnly:
		// no direct socket
	case options.directConn != nil:
		directConn = options.directConn
		directAddr = directConn.LocalAddr().(*net.UDPAddr)
	default:
		udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv6unspecified})
		if err != nil {
			return nil, err
		}
		directConn = udpConn
		directAddr = udpConn.LocalAddr().(*net.UDPAddr)
		ownedDirect = true
	}

	var stunC *stunConn
	if directConn != nil {
		stunC = newStunConn(directConn)
	}

	// Construct the endpoint before dialing the relay: the hole-punch handler
	// registered with the relay dial may run on the relay client's read-loop
	// goroutine the moment the dial completes, so it must observe a fully
	// initialized endpoint. Everything handleHolePunch reads (relayOnly,
	// directConn, closeCh) is set here, before any relay goroutine exists.
	endpoint := &Endpoint{
		secret:           secret,
		logger:           options.logger,
		relayOnly:        options.relayOnly,
		announcers:       append([]discovery.Announcer(nil), options.announcers...),
		directConn:       stunC,
		directAddr:       directAddr,
		stun:             stunC,
		conns:            make(chan *quic.Conn, 16),
		closeCh:          make(chan struct{}),
		relayWaitTimeout: options.relayWaitTimeout,
		tlsCfg:           options.tlsCfg,
		relayBatchSize:   options.batchSize,
		relayBatchCount:  options.batchCount,
		relayDrainDelay:  options.drainDelay,
	}
	if stunC != nil {
		endpoint.directTr = &quic.Transport{Conn: stunC}
	}
	if endpoint.relayWaitTimeout == 0 {
		endpoint.relayWaitTimeout = defaultRelayWaitTimeout
	}

	var rc *relay.RelayConn
	var url string
	if len(relays) > 0 {
		relayOpts := []relay.Option{
			relay.WithBatch(options.batchSize, options.batchCount, options.drainDelay),
			relay.WithHolePunchHandler(func(id base.NodeID, addrs []*net.UDPAddr) {
				endpoint.handleHolePunch(id, addrs)
			}),
		}
		var err error
		rc, url, err = dialRelayAny(ctx, relays, secret, relayOpts...)
		if err != nil {
			// Only the socket we bound ourselves: a caller-supplied
			// WithDirectConn is the caller's to close on a failed NewEndpoint.
			if ownedDirect {
				_ = directConn.Close()
			}
			return nil, err
		}
		relays = orderRelays(relays, url)
		endpoint.relayURL = url
		endpoint.relays = relays
		endpoint.relayConn = rc
		endpoint.relayTr = &quic.Transport{Conn: rc}
	}

	serverCfg, err := serverTLSConfig(secret, endpoint.tlsCfg)
	if err != nil {
		endpoint.Close()
		return nil, err
	}
	if endpoint.relayTr != nil {
		relayL, err := endpoint.relayTr.Listen(serverCfg, defaultQUICConfig())
		if err != nil {
			endpoint.Close()
			return nil, err
		}
		go endpoint.acceptLoop(relayL, false)
	}
	if endpoint.directTr != nil {
		directL, err := endpoint.directTr.Listen(serverCfg, defaultQUICConfig())
		if err != nil {
			endpoint.Close()
			return nil, err
		}
		go endpoint.acceptLoop(directL, true)
	}

	// Watch the relay connection and reconnect with backoff if it drops.
	if endpoint.relayConn != nil {
		go endpoint.watchRelay()
	}

	// Announce our direct addresses to the chosen channels (relay if opted in,
	// plus any registered announcers), unless the caller opted to publish a
	// specific set via SetAnnouncedAddrs (relay-only endpoints publish nothing).
	if !options.skipAnnounce && !options.relayOnly {
		endpoint.publishEndpoints()
	}
	// STUN discovery re-announces once our public address is known.
	go endpoint.discoverPublicAddr()
	return endpoint, nil
}

// EndpointOption configures Endpoint construction.
type EndpointOption func(*endpointOptions)

type endpointOptions struct {
	skipAnnounce     bool
	relayOnly        bool
	directConn       net.PacketConn
	relays           []string
	announcers       []discovery.Announcer
	batchSize        int
	batchCount       int
	drainDelay       time.Duration
	relayWaitTimeout time.Duration
	tlsCfg           TLSConfig
	logger           *slog.Logger
}

// defaultRelayWaitTimeout is how long Connect waits for the relay to come
// back before failing, when the relay is down.
const defaultRelayWaitTimeout = 5 * time.Second

// WithSkipAnnounce disables announcing this endpoint's direct addresses at
// startup. Use it when you want to publish a precise address set with
// SetAnnouncedAddrs instead of the auto-detected interface addresses.
func WithSkipAnnounce() EndpointOption {
	return func(o *endpointOptions) { o.skipAnnounce = true }
}

// WithRelayOnly disables p2p entirely: the endpoint opens no direct UDP socket
// and only ever connects through the relay. It will neither attempt direct
// connections nor respond to hole punching.
func WithRelayOnly() EndpointOption {
	return func(o *endpointOptions) { o.relayOnly = true }
}

// WithDirectConn overrides the socket used for direct connections. Mostly for
// tests that simulate NAT with an address-translating PacketConn; the conn's
// LocalAddr is what gets announced.
func WithDirectConn(conn net.PacketConn) EndpointOption {
	return func(o *endpointOptions) { o.directConn = conn }
}

// WithRelayURLs sets the full list of relay URLs the endpoint may use. The
// endpoint connects to the fastest reachable one and fails over to the others
// in order. If not given, only the NewEndpoint relayURL is used.
func WithRelayURLs(urls ...string) EndpointOption {
	return func(o *endpointOptions) {
		o.relays = append([]string(nil), urls...)
	}
}

// WithAnnouncers registers channels (other than the relay) to which this
// endpoint publishes its direct addresses. The endpoint owns them and closes
// them on Endpoint.Close. Announcement is independent of discovery: a channel
// may announce without discovering.
func WithAnnouncers(a ...discovery.Announcer) EndpointOption {
	return func(o *endpointOptions) {
		o.announcers = append(o.announcers, a...)
	}
}

// ConnectOption configures a single connection attempt (see Endpoint.Connect
// and Endpoint.ConnectAddr).
type ConnectOption func(*connectOptions)

type connectOptions struct {
	relayOnly   bool
	discoverers []discovery.Discoverer
}

// ConnectRelayOnly forces this connection through the relay, disabling direct
// dialing and hole punching for this connection only. Unlike the endpoint-wide
// WithRelayOnly, it does not change what the endpoint announces or whether it
// opens a direct socket.
func ConnectRelayOnly() ConnectOption {
	return func(o *connectOptions) { o.relayOnly = true }
}

// WithDiscoverers registers channels (other than the relay) from which this
// connection looks up a peer's direct addresses. These are caller-owned and
// are never closed by the endpoint. Discovery is independent of announcement.
func WithDiscoverers(d ...discovery.Discoverer) ConnectOption {
	return func(o *connectOptions) {
		o.discoverers = append(o.discoverers, d...)
	}
}

// WithRelayBatching configures outbound relay batching: max bytes per batch
// frame, max packets per batch frame, and how long a partial batch is held
// before flushing. A batchSize <= 0 disables batching. Defaults: 64 KiB,
// 16 packets, 500 microseconds.
func WithRelayBatching(batchSize, batchCount int, drainDelay time.Duration) EndpointOption {
	return func(o *endpointOptions) {
		o.batchSize = batchSize
		o.batchCount = batchCount
		o.drainDelay = drainDelay
	}
}

// WithRelayWaitTimeout sets how long Connect waits for the relay to come back
// after an outage before failing (default 5 seconds).
func WithRelayWaitTimeout(d time.Duration) EndpointOption {
	return func(o *endpointOptions) { o.relayWaitTimeout = d }
}

// WithTLSConfig customizes the TLS settings for all peer connections: the set
// of enabled KeyExchange groups (Go's CurvePreferences), ordered by
// preference. iron always negotiates TLS 1.3, whose cipher suites are fixed by
// the spec, so the curve preferences are the only tunable. A zero value keeps
// the default (X25519MLKEM768). Both endpoints in a connection must share at
// least one enabled group or the handshake fails.
func WithTLSConfig(c TLSConfig) EndpointOption {
	return func(o *endpointOptions) { o.tlsCfg = c }
}

func WithLogger(logger *slog.Logger) EndpointOption {
	return func(o *endpointOptions) {
		o.logger = logger
	}
}

// NodeID returns this endpoint's public identity.
func (endpoint *Endpoint) NodeID() base.NodeID { return endpoint.secret.Public() }

// NodeAddr returns this endpoint's complete address: its NodeID, its direct
// addresses (including the STUN-discovered public address) and its configured
// relays. Share it out of band so a peer can dial without a lookup.
func (endpoint *Endpoint) NodeAddr() base.NodeAddr {
	return base.NodeAddr{
		ID:     endpoint.secret.Public(),
		Direct: endpoint.localCandidates(),
		Relays: endpoint.relayList(),
	}
}

// ConnectAddr dials a node by its complete address (see NodeAddr). The direct
// addresses embedded in the address are tried first; the endpoint's relays are
// used as the fallback.
func (endpoint *Endpoint) ConnectAddr(ctx context.Context, addr base.NodeAddr, opts ...ConnectOption) (*Connection, error) {
	if len(addr.Direct) > 0 {
		endpoint.peerAddrs.Store(addr.ID, &candidateEntry{
			addrs: append([]*net.UDPAddr(nil), addr.Direct...),
			at:    time.Now(),
		})
	}
	return endpoint.Connect(ctx, addr.ID, opts...)
}

// SetAnnouncedAddrs overrides the direct addresses this endpoint announces to
// its announce channels (the relay if opted in, plus any announcers). Mostly
// useful for tests and unusual deployments.
func (endpoint *Endpoint) SetAnnouncedAddrs(addrs []*net.UDPAddr) error {
	endpoint.announceMu.Lock()
	endpoint.announced = append([]*net.UDPAddr(nil), addrs...)
	endpoint.announceMu.Unlock()
	endpoint.publishEndpoints()
	return nil
}

// publishEndpoints pushes the endpoint's current direct addresses to every
// announce channel: the connected relay (a LocalAddrs frame, so the directory
// entry lives only as long as the relay connection) and each registered
// announcer.
func (endpoint *Endpoint) publishEndpoints() {
	addrs := endpoint.localCandidates()
	if !endpoint.relayOnly {
		if rc := endpoint.currentRelayConn(); rc != nil {
			_ = rc.UpdateAnnounceAddrs(addrStrings(addrs))
		}
	}
	for _, a := range endpoint.announcers {
		a.Announce(addrs)
	}
}

func addrStrings(addrs []*net.UDPAddr) []string {
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if a != nil {
			out = append(out, a.String())
		}
	}
	return out
}

// Connect dials another node by its NodeID: it races a direct connection
// (using the peer's announced addresses and, if necessary, hole punching
// through its NAT) against a relay connection and returns whichever
// establishes first. The returned Connection redials over the relay
// transparently if a direct connection drops. ConnectOptions (e.g.
// ConnectRelayOnly) tailor this one connection attempt.
func (endpoint *Endpoint) Connect(ctx context.Context, peer base.NodeID, opts ...ConnectOption) (*Connection, error) {
	var options connectOptions
	for _, optFunc := range opts {
		optFunc(&options)
	}
	relayOnly := endpoint.relayOnly || options.relayOnly
	conn, path, err := endpoint.dial(ctx, peer, options, relayOnly)
	if err != nil {
		return nil, err
	}
	endpoint.logger.Info("connected", "peer", peer.String(), "path", path)
	return newConnection(endpoint, peer, conn, path, true, relayOnly, options), nil
}

// dialResult is the outcome of one dial path.
type dialResult struct {
	conn *quic.Conn
	path string
	err  error
}

// errPeerNotFound is returned when the peer could not be found on any
// configured relay and no discoverer or announced address gave us a candidate.
var errPeerNotFound = errors.New("peer not found on any relay or discovery channel")

// dial connects to peer. It first looks the peer up across all configured
// relays (and registered discoverers); only if the peer is found (on a relay,
// via its announced addresses, or by a discoverer) does it attempt to connect,
// racing a direct dial against a relay dial through our own relay. When the
// peer lives on a different relay, our relay forwards the packets over the
// backbone (see SetPeerRelay). A peer found nowhere fails with errPeerNotFound.
func (endpoint *Endpoint) dial(ctx context.Context, peer base.NodeID, co connectOptions, relayOnly bool) (*quic.Conn, string, error) {
	addrs, observed, foundRelays, err := endpoint.lookupCandidates(ctx, peer, co)
	if err != nil {
		return nil, "", err
	}
	hasDirect := len(addrs) > 0 || observed != nil
	foundOnRelay := len(foundRelays) > 0
	relayTr := endpoint.currentRelayTr()

	// Peer nowhere: nothing to dial.
	if !foundOnRelay && !hasDirect {
		return nil, "", errPeerNotFound
	}

	if relayOnly {
		if relayTr == nil {
			return nil, "", errors.New("no relay configured")
		}
		endpoint.setPeerRelay(peer, foundRelays)
		r := endpoint.dialRelayPath(ctx, peer)
		return r.conn, r.path, r.err
	}

	if endpoint.directTr == nil {
		return nil, "", errors.New("no direct socket available")
	}
	if relayTr == nil {
		// Relay-free endpoint: only the direct path.
		idle := make(chan struct{})
		r := endpoint.dialDirectPath(ctx, peer, co, idle)
		return r.conn, r.path, r.err
	}

	dctx, cancel := context.WithCancel(ctx)
	defer cancel()

	endpoint.setPeerRelay(peer, foundRelays)

	results := make(chan dialResult, 2)
	directIdle := make(chan struct{}) // closed when the direct path has nothing to try
	directDone := make(chan struct{}) // closed once the direct path has reported

	go func() {
		defer close(directDone)
		results <- endpoint.dialDirectPath(dctx, peer, co, directIdle)
	}()
	var relayDone chan struct{}
	if foundOnRelay {
		relayDone = make(chan struct{})
		go func() {
			defer close(relayDone)
			// If the direct path reports it has nothing to try, dial the relay
			// immediately; otherwise give the direct path a head start so a
			// reachable direct connection wins deterministically.
			select {
			case <-directIdle:
			case <-time.After(relayHeadStart):
			case <-dctx.Done():
				return
			}
			results <- endpoint.dialRelayPath(dctx, peer)
		}()
	}

	// Wait for the first successful path. A failing path is not a result: the
	// relay is the fallback, so we keep waiting until a path establishes or
	// all of them have failed.
	var pending dialResult
	done := 0
	for {
		select {
		case r := <-results:
			done++
			if r.err == nil {
				cancel()
				// The other path has lost the race: wait for it to report, so
				// no connection it established (e.g. a direct dial that
				// finished a moment before the cancel took effect) is leaked.
				other := relayDone
				if r.path == PathRelay {
					other = directDone
				}
				if other != nil {
					<-other
					select {
					case r2 := <-results:
						if r2.conn != nil && r2.conn != r.conn {
							_ = r2.conn.CloseWithError(0, "")
						}
					default:
					}
				}
				return r.conn, r.path, nil
			}
			if pending.conn == nil && pending.err == nil {
				pending = r
			}
			if done == 2 || (!foundOnRelay && done == 1) {
				return pending.conn, pending.path, pending.err
			}
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
	}
}

// setPeerRelay tells our relay where peer lives so it can forward packets over
// the backbone when the peer is on a different relay. When the peer is on our
// own relay (or we have no hint), the hint is cleared so packets route locally.
func (endpoint *Endpoint) setPeerRelay(peer base.NodeID, foundRelays []string) {
	rc := endpoint.currentRelayConn()
	if rc == nil {
		return
	}
	cur := endpoint.currentRelayURL()
	for _, u := range foundRelays {
		if u != cur {
			rc.SetPeerRelay(peer, u)
			return
		}
	}
	rc.SetPeerRelay(peer, "")
}

// dialRelayPath dials the peer solely through the relay, waiting for the relay
// to come back if it is currently down.
func (endpoint *Endpoint) dialRelayPath(ctx context.Context, peer base.NodeID) dialResult {
	if err := endpoint.waitForRelay(ctx); err != nil {
		return dialResult{err: err}
	}
	conn, err := endpoint.currentRelayTr().Dial(ctx, base.PeerAddr{ID: peer}, endpoint.clientTLSConfig(peer), defaultQUICConfig())
	if err != nil {
		return dialResult{err: err}
	}
	if ctx.Err() != nil { // we lost the race: close and report failure
		_ = conn.CloseWithError(0, "")
		return dialResult{err: errors.New("relay dial lost the race")}
	}
	return dialResult{conn: conn, path: PathRelay}
}

// dialDirectPath attempts a direct connection to the peer, in two phases. It
// first tries plain direct dials to candidates that need no NAT traversal (the
// same host, or a peer on one of our own subnets); only if those fail does it
// hole-punch to the peer's relay-observed public address, which is what lets
// packets through both peers' NATs. Publicly reachable servers are covered by
// the punch phase too: the coordination is a harmless no-op there and the dial
// succeeds directly. closeIdle is closed when the direct path has nothing left
// to try, so the relay dial can start immediately.
func (endpoint *Endpoint) dialDirectPath(ctx context.Context, peer base.NodeID, co connectOptions, closeIdle chan<- struct{}) dialResult {
	addrs, observed, _, err := endpoint.lookupCandidates(ctx, peer, co)
	if err != nil {
		close(closeIdle)
		return dialResult{err: err}
	}
	target := punchTarget(observed, addrs)
	if len(addrs) == 0 && target == nil {
		close(closeIdle)
		return dialResult{err: errNoDirectCandidates}
	}
	conn, err := endpoint.dialFromCandidates(ctx, peer, addrs, target)
	if err != nil {
		// Every candidate failed, so the peer's addresses are likely stale:
		// evict them so the next Connect re-discovers.
		endpoint.forgetCandidates(peer)
		return dialResult{err: err}
	}
	if ctx.Err() != nil { // we lost the race: close and report failure
		_ = conn.CloseWithError(0, "")
		return dialResult{err: errNoDirectCandidates}
	}
	return dialResult{conn: conn, path: PathDirect}
}

// punchTarget synthesizes the peer's relay-observed public address, which is
// the hole-punching target for a peer behind NAT. It is the peer's public IP as
// seen by the relay (observed) plus the peer's announced UDP port (port-
// preserving NAT assumption). observed may be nil for non-relay discovery
// channels, in which case there is no punch target.
func punchTarget(observed net.IP, addrs []*net.UDPAddr) *net.UDPAddr {
	if observed == nil {
		return nil
	}
	port := 0
	for _, a := range addrs {
		if a.IP.Equal(observed) {
			port = a.Port
			break
		}
	}
	if port == 0 && len(addrs) > 0 {
		port = addrs[0].Port
	}
	if port == 0 {
		return nil
	}
	return &net.UDPAddr{IP: observed, Port: port}
}

// dialFromCandidates runs the two-phase direct dial given a peer's candidates.
// target is the relay-observed punch address (may be nil for non-relay
// discovery channels).
func (endpoint *Endpoint) dialFromCandidates(ctx context.Context, peer base.NodeID, addrs []*net.UDPAddr, target *net.UDPAddr) (*quic.Conn, error) {
	if conn, ok := endpoint.dialLocal(ctx, peer, addrs); ok {
		return conn, nil
	}
	if conn, ok := endpoint.dialPunch(ctx, peer, addrs, target); ok {
		return conn, nil
	}
	return nil, errNoDirectCandidates
}

// tryDirect is dialFromCandidates with a fresh candidate lookup. Used by the
// path upgrade.
func (endpoint *Endpoint) tryDirect(ctx context.Context, peer base.NodeID, co connectOptions) (*quic.Conn, error) {
	addrs, observed, _, err := endpoint.lookupCandidates(ctx, peer, co)
	if err != nil {
		return nil, err
	}
	target := punchTarget(observed, addrs)
	if len(addrs) == 0 && target == nil {
		return nil, errNoDirectCandidates
	}
	return endpoint.dialFromCandidates(ctx, peer, addrs, target)
}

// tryDirectFresh is tryDirect with the candidate cache bypassed, so an upgrade
// sees the peer's latest addresses.
func (endpoint *Endpoint) tryDirectFresh(ctx context.Context, peer base.NodeID, co connectOptions) (*quic.Conn, error) {
	endpoint.forgetCandidates(peer)
	return endpoint.tryDirect(ctx, peer, co)
}

// dialLocal tries plain direct dials to candidates that are reachable without
// NAT traversal: the same host, or a peer on one of our own subnets. Each
// candidate gets a single dial attempt with a normal timeout; a reachable
// peer answers within a few round trips, so this is fast and needs no hole
// punching.
func (endpoint *Endpoint) dialLocal(ctx context.Context, peer base.NodeID, addrs []*net.UDPAddr) (*quic.Conn, bool) {
	ln := endpoint.localNetworks()
	for _, addr := range endpoint.filterCandidates(addrs) {
		if !equalAnyIP(addr.IP, ln.addrs) && !containsIP(ln.nets, addr.IP) {
			continue // requires NAT traversal: leave for the punch phase
		}
		dctx, cancel := context.WithTimeout(ctx, directAttemptTimeout)
		conn, err := endpoint.directTr.Dial(dctx, addr, endpoint.clientTLSConfig(peer), defaultQUICConfig())
		cancel()
		if err == nil {
			if ctx.Err() != nil { // we lost the race: close and report failure
				_ = conn.CloseWithError(0, "")
				return nil, false
			}
			endpoint.logger.Info("connected direct", "peer", peer.String(), "addr", addr.String())
			return conn, true
		}
		if ctx.Err() != nil {
			return nil, false
		}
	}
	return nil, false
}

// dialPunch asks the relay to tell the peer we are trying to reach it directly
// (so it opens its NAT mapping for us, the simultaneous open), then repeatedly
// dials the peer's candidates until one connects or the punch window closes.
// For a publicly reachable peer the coordination is a harmless no-op and the
// first dial simply succeeds.
func (endpoint *Endpoint) dialPunch(ctx context.Context, peer base.NodeID, addrs []*net.UDPAddr, observed *net.UDPAddr) (*quic.Conn, bool) {
	cands := endpoint.punchCandidates(addrs, observed)
	if len(cands) == 0 {
		return nil, false
	}
	if rc := endpoint.currentRelayConn(); rc != nil {
		_ = rc.RequestHolePunch(peer)
	}

	deadline := time.Now().Add(punchTimeout)
	for _, addr := range cands {
		for {
			attempt := time.Until(deadline)
			if attempt <= 0 {
				break
			}
			if attempt > punchAttempt {
				attempt = punchAttempt
			}
			dctx, cancel := context.WithTimeout(ctx, attempt)
			conn, derr := endpoint.directTr.Dial(dctx, addr, endpoint.clientTLSConfig(peer), defaultQUICConfig())
			cancel()
			if derr == nil {
				if ctx.Err() != nil { // we lost the race: close and report failure
					_ = conn.CloseWithError(0, "")
					return nil, false
				}
				endpoint.logger.Info("connected direct (punched)", "peer", peer.String(), "addr", addr.String())
				return conn, true
			}
			if ctx.Err() != nil || time.Now().After(deadline) {
				break
			}
		}
	}
	return nil, false
}

// Accept returns the next inbound QUIC connection (direct or through the
// relay).
func (endpoint *Endpoint) Accept(ctx context.Context) (*Connection, error) {
	select {
	case conn := <-endpoint.conns:
		path := PathRelay
		if _, ok := conn.RemoteAddr().(*net.UDPAddr); ok {
			path = PathDirect
		}
		peer, err := peerIDFromTLS(conn.ConnectionState().TLS)
		if err != nil {
			peer = base.NodeID{}
		}
		return newConnection(endpoint, peer, conn, path, false, false, connectOptions{}), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-endpoint.closeCh:
		return nil, errors.New("endpoint closed")
	}
}

// PeerID returns the NodeID of the peer on an established connection.
func (endpoint *Endpoint) PeerID(conn *Connection) (base.NodeID, error) {
	return conn.PeerID()
}

// Close shuts down the endpoint, its UDP socket, its relay connection and any
// announcers registered at construction.
func (endpoint *Endpoint) Close() error {
	endpoint.closeOne.Do(func() { close(endpoint.closeCh) })
	var err1 error
	if tr := endpoint.currentRelayTr(); tr != nil {
		err1 = tr.Close()
	}
	var err2 error
	if endpoint.directTr != nil {
		err2 = endpoint.directTr.Close()
	}
	if rc := endpoint.currentRelayConn(); rc != nil {
		_ = rc.Close()
	}
	for _, a := range endpoint.announcers {
		_ = a.Close()
	}
	if err1 != nil {
		return err1
	}
	return err2
}

// localCandidates returns this endpoint's direct addresses (or the override
// set via SetAnnouncedAddrs): every interface IP at our UDP port, plus the
// public address discovered via relay STUN. Loopback and IPv6 link-local
// interface addresses are skipped: loopback is present on every host (and we
// never dial it), while fe80::/10 addresses are only dialable with an
// interface zone a remote peer cannot know, so announcing them is pure noise.
func (endpoint *Endpoint) localCandidates() []*net.UDPAddr {
	if endpoint.relayOnly || endpoint.directAddr == nil {
		return nil
	}
	endpoint.announceMu.Lock()
	override := endpoint.announced
	endpoint.announceMu.Unlock()
	if override != nil {
		return override
	}

	port := endpoint.directAddr.Port
	var addrs []*net.UDPAddr
	ifaces, err := net.Interfaces()
	if err != nil {
		return addrs
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		ifAddrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range ifAddrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || isV6LinkLocal(ip) {
				continue
			}
			addrs = append(addrs, &net.UDPAddr{IP: ip, Port: port})
		}
	}
	if pub := endpoint.PublicAddr(); pub != nil {
		addrs = append(addrs, pub)
	}
	return sortCandidates(addrs)
}

// candidateTTL is how long a peer's direct addresses are cached before they
// are re-fetched.
var candidateTTL = 60 * time.Second

// discoverTimeout bounds one discovery-channel lookup during Connect, so a
// slow or hung discoverer cannot stall the dial.
const discoverTimeout = time.Second

// candidateEntry is a cached set of a peer's direct addresses plus the peer's
// observed public IP (from relay discovery), used for the punch target.
type candidateEntry struct {
	addrs       []*net.UDPAddr
	observed    net.IP
	foundRelays []string // configured relays the peer was found on
	at          time.Time
}

// lookupCandidates returns the peer's direct addresses from all enabled
// channels — the connected relay's directory (which fans out to the other
// configured relays over the backbone itself) and each registered discoverer —
// merged, deduplicated and cached for candidateTTL, together with the peer's
// observed public IP (the relay's view of the peer; may be nil) and the relays
// that reported the peer. The relay may not have processed the peer's
// connection yet, so an empty result is retried a couple of times and is
// never cached.
func (endpoint *Endpoint) lookupCandidates(ctx context.Context, peer base.NodeID, co connectOptions) ([]*net.UDPAddr, net.IP, []string, error) {
	if v, ok := endpoint.peerAddrs.Load(peer); ok {
		ent := v.(*candidateEntry)
		if len(ent.addrs) > 0 && time.Since(ent.at) < candidateTTL {
			return ent.addrs, ent.observed, ent.foundRelays, nil
		}
	}

	var combined []*net.UDPAddr
	var observed net.IP
	var foundRelays []string
	for attempt := 0; attempt < 3; attempt++ {
		combined = nil
		observed = nil
		foundRelays = nil

		// Relay discovery is automatic when the endpoint is connected to a
		// relay: look the peer up over HTTP on the connected relay (it answers
		// from its own clients and broadcasts to its peers, so the client only
		// talks to its one relay).
		if url := endpoint.currentRelayURL(); url != "" {
			addrs, obs, fr, err := endpoint.relayHTTPLookup(ctx, url, peer)
			if err != nil {
				return nil, nil, nil, err
			}
			combined = append(combined, addrs...)
			observed = obs
			foundRelays = fr
		}
		if len(co.discoverers) > 0 {
			combined = append(combined, endpoint.discover(ctx, co.discoverers, peer)...)
		}

		// A result is useful if we have direct addresses OR the peer was found
		// on a relay (e.g. a relay-only peer reports no addresses but is known).
		if len(combined) > 0 || len(foundRelays) > 0 {
			endpoint.peerAddrs.Store(peer, &candidateEntry{addrs: combined, observed: observed, foundRelays: foundRelays, at: time.Now()})
			return combined, observed, foundRelays, nil
		}
		select {
		case <-ctx.Done():
			return nil, nil, nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return nil, nil, nil, nil
}

// relayHTTPLookup performs one signed HTTP lookup against a single relay's
// /relay/api directory (the endpoint's connected relay), returning the peer's
// direct addresses, its observed public IP (may be nil) and the relays that
// reported the peer (the answering relay plus any of its peers that found it).
func (endpoint *Endpoint) relayHTTPLookup(ctx context.Context, relayURL string, peer base.NodeID) ([]*net.UDPAddr, net.IP, []string, error) {
	u, err := relayHTTPURL(relayURL, "/relay/api")
	if err != nil {
		return nil, nil, nil, err
	}
	body, err := proto.Encode(proto.APILookupRequest{ID: peer})
	if err != nil {
		return nil, nil, nil, err
	}

	dctx, cancel := context.WithTimeout(ctx, discoverTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(dctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, nil, nil, err
	}
	req.Header.Set("Authorization", endpoint.authHeader(http.MethodPost, u.RequestURI(), body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, nil, fmt.Errorf("relay lookup: status %d", resp.StatusCode)
	}
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, nil, nil, err
	}
	var lr proto.APILookupResponse
	if err := proto.Unmarshal(respBody, &lr); err != nil {
		return nil, nil, nil, err
	}

	var addrs []*net.UDPAddr
	for _, s := range lr.Addrs {
		if a, err := net.ResolveUDPAddr("udp", s); err == nil {
			addrs = append(addrs, a)
		}
	}
	var observed net.IP
	if lr.Observed != "" {
		observed = net.ParseIP(lr.Observed)
	}
	return addrs, observed, lr.FoundRelays, nil
}

// relayHTTPURL converts a ws(s) relay URL into the http(s) URL of a relay HTTP
// endpoint path.
func relayHTTPURL(relayURL, path string) (*url.URL, error) {
	u, err := url.Parse(relayURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("invalid relay url scheme: want http or https")
	}
	u.Path = path
	return u, nil
}

// authHeader signs the given request and renders the Authorization header.
func (endpoint *Endpoint) authHeader(method, requestURI string, body []byte) string {
	ts := time.Now().Unix()
	return base.BuildAuthHeader(endpoint.secret.Public(), ts, base.SignHTTPRequest(endpoint.secret, method, requestURI, body, ts))
}

// discover queries every discoverer concurrently, bounded by discoverTimeout,
// and returns the merged, de-duplicated addresses.
func (endpoint *Endpoint) discover(ctx context.Context, ds []discovery.Discoverer, peer base.NodeID) []*net.UDPAddr {
	type res struct {
		addrs []*net.UDPAddr
	}
	results := make(chan res, len(ds))
	for _, d := range ds {
		go func(dd discovery.Discoverer) {
			dctx, cancel := context.WithTimeout(ctx, discoverTimeout)
			results <- res{addrs: dd.Lookup(dctx, peer)}
			cancel()
		}(d)
	}
	var out []*net.UDPAddr
	seen := make(map[string]bool)
	for i := 0; i < len(ds); i++ {
		for _, a := range (<-results).addrs {
			if a == nil {
				continue
			}
			key := addrKey(a)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, a)
		}
	}
	return out
}

// forgetCandidates drops the cached direct addresses for a peer, forcing the
// next Connect to re-discover them.
func (endpoint *Endpoint) forgetCandidates(peer base.NodeID) {
	endpoint.peerAddrs.Delete(peer)
}

// dialRelay connects to the peer solely through the relay (used for the
// transparent fallback). It does not wait for the relay to come back. The
// peer's routing hint (which relay it lives on) is re-applied from the cache so
// the redial still reaches it over the backbone.
func (endpoint *Endpoint) dialRelay(ctx context.Context, peer base.NodeID) (*quic.Conn, error) {
	tr := endpoint.currentRelayTr()
	if tr == nil {
		return nil, errors.New("no relay configured")
	}
	var foundRelays []string
	if v, ok := endpoint.peerAddrs.Load(peer); ok {
		foundRelays = v.(*candidateEntry).foundRelays
	}
	endpoint.setPeerRelay(peer, foundRelays)
	return tr.Dial(ctx, base.PeerAddr{ID: peer}, endpoint.clientTLSConfig(peer), defaultQUICConfig())
}

func (endpoint *Endpoint) clientTLSConfig(peer base.NodeID) *tls.Config {
	cfg, err := clientTLSConfig(endpoint.secret, peer, endpoint.tlsCfg)
	if err != nil {
		panic(err) // identity is static; cannot fail
	}
	return cfg
}

// acceptLoop feeds inbound connections into e.conns. If fatal is true (the
// direct listener), a listener error shuts the whole endpoint down; the relay
// listener is not fatal because watchRelay owns reconnection.
func (endpoint *Endpoint) acceptLoop(l *quic.Listener, fatal bool) {
	for {
		conn, err := l.Accept(context.Background())
		if err != nil {
			if fatal {
				endpoint.closeOne.Do(func() { close(endpoint.closeCh) })
			}
			return
		}
		select {
		case endpoint.conns <- conn:
		case <-endpoint.closeCh:
			conn.CloseWithError(0, "")
			return
		}
	}
}

// sortCandidates orders addresses by likely reachability (same-host, then
// LAN, then public) and removes duplicates.
func sortCandidates(addrs []*net.UDPAddr) []*net.UDPAddr {
	seen := make(map[string]bool, len(addrs))
	out := make([]*net.UDPAddr, 0, len(addrs))
	for _, a := range addrs {
		key := ipKey(a.IP)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return rankIP(out[i].IP) < rankIP(out[j].IP)
	})
	return out
}

func rankIP(ip net.IP) int {
	switch {
	case ip.IsLoopback():
		return 0
	case ip.IsLinkLocalUnicast():
		return 2
	case ip.IsPrivate():
		return 1
	default:
		return 3
	}
}

func ipKey(ip net.IP) string {
	if ip4 := ip.To4(); ip4 != nil {
		return "4:" + ip4.String()
	}
	if ip6 := ip.To16(); ip6 != nil {
		return "6:" + ip6.String()
	}
	return ""
}

// localNet is a snapshot of this endpoint's network interfaces: the exact
// addresses (to detect same-host peers) and the prefixes (to detect LAN
// peers).
type localNet struct {
	addrs []net.IP
	nets  []net.IPNet
}

// localNetworks re-enumerates the up interfaces. It is called on every
// Connect so that interface changes (WiFi reconnects, etc.) are picked up.
func (endpoint *Endpoint) localNetworks() localNet {
	var ln localNet
	ifaces, err := net.Interfaces()
	if err != nil {
		return ln
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		ifAddrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range ifAddrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
				ln.nets = append(ln.nets, *v)
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil {
				ln.addrs = append(ln.addrs, ip)
			}
		}
	}
	return ln
}

// filterCandidates drops direct-dial candidates that cannot be reachable from
// this host, then dedups and orders the remainder.
func (endpoint *Endpoint) filterCandidates(addrs []*net.UDPAddr) []*net.UDPAddr {
	var observed net.IP
	if rc := endpoint.currentRelayConn(); rc != nil {
		observed = rc.ObservedIP()
	}
	return filterCandidatesFor(addrs, endpoint.localNetworks(), observed)
}

// filterCandidatesFor is the pure form of filterCandidates, parameterized by
// the interface snapshot and our observed address so it can be tested in
// isolation.
//
// When the peer is detected to be the same host (a candidate is strictly
// equal to one of our interface addresses), 127.0.0.1 is tried FIRST: both
// endpoints listen on 0.0.0.0, so loopback is the guaranteed-reachable,
// loopback-fast path (~microsecond RTT), while a same-host interface address
// might not actually be local to us (different network namespace, or a
// veth/docker peer) or could be dropped by strict reverse-path filtering —
// and each such failed attempt costs up to directAttemptTimeout. The real
// interface candidates are kept as follow-ups. Loopback is only ever added
// for genuinely same-host peers, so it does not reintroduce the cross-host
// loopback noise.
func filterCandidatesFor(addrs []*net.UDPAddr, ln localNet, observed net.IP) []*net.UDPAddr {
	sameHost := false
	kept := make([]*net.UDPAddr, 0, len(addrs))
	for _, a := range addrs {
		if reachableIP(a.IP, ln, observed) {
			kept = append(kept, a)
			if !sameHost && equalAnyIP(a.IP, ln.addrs) {
				sameHost = true
			}
		}
	}
	out := sortCandidates(kept)

	if sameHost {
		seen := make(map[string]bool, 1)
		var loopbacks []*net.UDPAddr
		for _, a := range kept {
			if !equalAnyIP(a.IP, ln.addrs) {
				continue
			}
			lb := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: a.Port}
			if key := "127.0.0.1:" + strconv.Itoa(a.Port); !seen[key] {
				seen[key] = true
				loopbacks = append(loopbacks, lb)
			}
		}
		out = append(loopbacks, out...)
	}
	return out
}

// reachableIP decides whether a candidate address is worth a direct-dial
// attempt.
//
// A candidate is tried only if it is plausibly reachable from this host:
//   - public and RFC 6598 CGNAT addresses are kept — they may be directly
//     reachable over the internet (CGNAT is not private per RFC 1918);
//   - RFC 1918 / ULA / link-local addresses are kept only when they fall on
//     one of our own subnets (the peer is on our LAN);
//   - a candidate strictly equal to one of our interface addresses is kept
//     (the peer is the same host);
//   - loopback candidates are always dropped: every endpoint announces
//     127.0.0.1, so keeping them would make us probe every peer's loopback.
//     Same-host peers are instead covered by the strict-equality rule on
//     their real interface addresses;
//   - a candidate equal to our own relay-observed address is dropped: that
//     address is our NAT's public IP. A peer behind the same home NAT shares
//     it, but dialing it would reach the router's WAN port, not the peer —
//     such a peer is reachable via its LAN candidates (same subnet) instead.
//     Two users behind carrier NAT (RFC 6598) may share a pool address; for
//     the same reason that shared address cannot route to a specific user, so
//     it is dropped there too, while users with *distinct* observed
//     addresses are unaffected and still get a direct attempt.
//
// Loopback never passes this predicate; when the peer is the same host,
// filterCandidatesFor appends 127.0.0.1 as a last-resort fallback instead.
func reachableIP(ip net.IP, ln localNet, observed net.IP) bool {
	switch {
	case ip == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLoopback():
		return false
	case isV6LinkLocal(ip):
		return false // not dialable without an interface zone
	case observed != nil && ip.Equal(observed):
		return false // our own NAT's public address (same-NAT / shared-CGNAT peer)
	case equalAnyIP(ip, ln.addrs):
		return true // same host
	case ip.IsLinkLocalUnicast() || isPrivateNet(ip):
		return containsIP(ln.nets, ip) // LAN peer
	default:
		return true // public / RFC 6598 CGNAT
	}
}

// isV6LinkLocal reports whether ip is an IPv6 link-local address (fe80::/10).
// Such addresses are only dialable with an interface zone, which a remote
// peer cannot supply, so they are never dialed.
func isV6LinkLocal(ip net.IP) bool {
	return ip.To4() == nil && len(ip) == net.IPv6len && ip[0] == 0xfe && ip[1]&0xc0 == 0x80
}

// isPrivateNet reports whether ip is in the RFC 1918 private ranges or the
// RFC 4193 ULA range. RFC 6598 CGNAT (100.64/10) is deliberately NOT
// included: CGNAT observed addresses can be directly reachable, so they are
// treated as public.
func isPrivateNet(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 10 ||
			(ip4[0] == 172 && ip4[1]&0xf0 == 16) ||
			(ip4[0] == 192 && ip4[1] == 168)
	}
	return len(ip) == net.IPv6len && ip[0]&0xfe == 0xfc
}

func equalAnyIP(ip net.IP, ours []net.IP) bool {
	for _, o := range ours {
		if o.Equal(ip) {
			return true
		}
	}
	return false
}

func containsIP(nets []net.IPNet, ip net.IP) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// currentRelayConn returns the active relay connection (the one being used for
// dialing, lookups and announcements).
func (endpoint *Endpoint) currentRelayConn() *relay.RelayConn {
	endpoint.relayMu.RLock()
	defer endpoint.relayMu.RUnlock()
	return endpoint.relayConn
}

// currentRelayTr returns the active relay QUIC transport.
func (endpoint *Endpoint) currentRelayTr() *quic.Transport {
	endpoint.relayMu.RLock()
	defer endpoint.relayMu.RUnlock()
	return endpoint.relayTr
}

// currentRelayURL returns the URL of the active relay connection.
func (endpoint *Endpoint) currentRelayURL() string {
	endpoint.relayMu.RLock()
	defer endpoint.relayMu.RUnlock()
	return endpoint.relayURL
}

// relayList returns a snapshot of the configured relay URLs, best (current)
// first.
func (endpoint *Endpoint) relayList() []string {
	endpoint.relayMu.RLock()
	defer endpoint.relayMu.RUnlock()
	return append([]string(nil), endpoint.relays...)
}

func (endpoint *Endpoint) isClosing() bool {
	select {
	case <-endpoint.closeCh:
		return true
	default:
		return false
	}
}

// Relay reconnection backoff bounds and per-relay dial timeout.
const (
	relayBackoffMin = 100 * time.Millisecond
	relayBackoffMax = 10 * time.Second
	relayDialTout   = 5 * time.Second
	// relayPreferWindow gives the first configured relay a head start, so
	// healthy peers converge on the same relay — two peers must share a relay
	// to connect through one.
	relayPreferWindow = 200 * time.Millisecond
)

// orderRelays moves first to the front of the list, keeping the rest in their
// original order.
func orderRelays(relays []string, first string) []string {
	out := []string{first}
	for _, u := range relays {
		if u != first {
			out = append(out, u)
		}
	}
	return out
}

// dialRelayAny dials a relay from the list, preferring the first configured
// one (so healthy peers converge on the same relay) and falling back to the
// others concurrently, first to connect wins. The chosen relay is returned
// together with its URL.
func dialRelayAny(ctx context.Context, relays []string, secret *base.NodeSecret, opts ...relay.Option) (*relay.RelayConn, string, error) {
	if len(relays) == 0 {
		return nil, "", errors.New("no relays configured")
	}
	if len(relays) == 1 {
		rc, err := relay.Dial(ctx, relays[0], secret, opts...)
		return rc, relays[0], err
	}

	// Give the first relay a head start so peers converge on it when healthy.
	fctx, fcancel := context.WithTimeout(ctx, relayPreferWindow)
	frc, ferr := relay.Dial(fctx, relays[0], secret, opts...)
	fcancel()
	if ferr == nil {
		return frc, relays[0], nil
	}
	if ctx.Err() != nil {
		return nil, "", ctx.Err()
	}

	// Preferred relay unavailable: race the rest, first to connect wins.
	rest := relays[1:]
	dctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type dialRes struct {
		rc  *relay.RelayConn
		url string
		err error
	}
	results := make(chan dialRes, len(rest))
	for _, url := range rest {
		go func(u string) {
			rdctx, rcancel := context.WithTimeout(dctx, relayDialTout)
			rc, err := relay.Dial(rdctx, u, secret, opts...)
			rcancel()
			results <- dialRes{rc: rc, url: u, err: err}
		}(url)
	}

	for i := 0; i < len(rest); i++ {
		r := <-results
		if r.rc != nil {
			cancel()
			// Close anything else that connected before the cancel took effect.
			for j := 0; j < len(rest)-1-i; j++ {
				if rr := <-results; rr.rc != nil && rr.rc != r.rc {
					_ = rr.rc.Close()
				}
			}
			return r.rc, r.url, nil
		}
	}
	return nil, "", errors.New("no relay reachable")
}

// reconnectRelay re-establishes a relay connection, trying the current relay
// first and then the others (in parallel, first to connect wins), so the
// endpoint does not flap between relays during an outage.
func (endpoint *Endpoint) reconnectRelay(ctx context.Context, opts ...relay.Option) (*relay.RelayConn, string, error) {
	dctx, cancel := context.WithTimeout(ctx, relayDialTout)
	rc, err := relay.Dial(dctx, endpoint.relayURL, endpoint.secret, opts...)
	cancel()
	if err == nil {
		return rc, endpoint.relayURL, nil
	}
	var others []string
	for _, u := range endpoint.relays {
		if u != endpoint.relayURL {
			others = append(others, u)
		}
	}
	return dialRelayAny(ctx, others, endpoint.secret, opts...)
}

// installRelay makes nrc the endpoint's active relay connection: it builds a
// fresh QUIC transport, listens on it for inbound relayed connections,
// re-announces our direct addresses, and swaps it in atomically.
func (endpoint *Endpoint) installRelay(nrc *relay.RelayConn, url string) error {
	// The endpoint may have been closed while the reconnection dial was in
	// flight (watchRelay checks closeCh before dialing, but not after): refuse
	// to install a fresh relay nobody would tear down.
	if endpoint.isClosing() {
		_ = nrc.Close()
		return errors.New("endpoint closed")
	}
	tr := &quic.Transport{Conn: nrc}
	serverCfg, err := serverTLSConfig(endpoint.secret, endpoint.tlsCfg)
	if err != nil {
		_ = nrc.Close()
		return err
	}
	l, err := tr.Listen(serverCfg, defaultQUICConfig())
	if err != nil {
		_ = nrc.Close()
		return err
	}
	if endpoint.isClosing() {
		_ = nrc.Close()
		return errors.New("endpoint closed")
	}
	go endpoint.acceptLoop(l, false)

	endpoint.relayMu.Lock()
	oldTr := endpoint.relayTr
	oldConn := endpoint.relayConn
	endpoint.relayConn = nrc
	endpoint.relayTr = tr
	endpoint.relayURL = url
	endpoint.relays = orderRelays(endpoint.relays, url)
	endpoint.relayMu.Unlock()

	// Re-announce on the new relay (and any announcers) once it becomes active.
	if !endpoint.relayOnly {
		endpoint.publishEndpoints()
	}

	// Close the old transport so connections still on the previous relay fail
	// fast and their path watchers redial over the new one, instead of
	// lingering until the idle timeout.
	if oldTr != nil {
		_ = oldTr.Close()
	}
	if oldConn != nil {
		_ = oldConn.Close()
	}
	return nil
}

// watchRelay reconnects to a relay with exponential backoff whenever the relay
// connection drops, failing over to the other configured relays, keeping the
// endpoint usable across relay outages. It does nothing on relay-free
// endpoints.
func (endpoint *Endpoint) watchRelay() {
	if endpoint.currentRelayConn() == nil {
		return
	}
	backoff := relayBackoffMin
	for {
		rc := endpoint.currentRelayConn()
		<-rc.Closed()
		if endpoint.isClosing() {
			return
		}
		relayOpts := []relay.Option{
			relay.WithBatch(endpoint.relayBatchSize, endpoint.relayBatchCount, endpoint.relayDrainDelay),
			relay.WithHolePunchHandler(func(id base.NodeID, addrs []*net.UDPAddr) {
				if endpoint != nil {
					endpoint.handleHolePunch(id, addrs)
				}
			}),
		}
		endpoint.logger.Warn("relay connection lost; reconnecting", "url", endpoint.relayURL)
		for {
			if endpoint.isClosing() {
				return
			}
			nrc, url, err := endpoint.reconnectRelay(context.Background(), relayOpts...)
			if err == nil {
				if ierr := endpoint.installRelay(nrc, url); ierr == nil {
					endpoint.logger.Info("relay reconnected", "url", url)
					backoff = relayBackoffMin
					break
				}
				nrc.Close()
			}
			select {
			case <-endpoint.closeCh:
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > relayBackoffMax {
				backoff = relayBackoffMax
			}
		}
	}
}

// waitForRelay blocks until the relay is usable or ctx/relayWaitTimeout
// expires. It is called before dialing over the relay so that a Connect
// during a relay outage briefly waits for reconnection instead of failing
// immediately.
func (endpoint *Endpoint) waitForRelay(ctx context.Context) error {
	if endpoint.currentRelayConn() == nil {
		return errors.New("no relay configured")
	}
	deadline := time.Now().Add(endpoint.relayWaitTimeout)
	for {
		rc := endpoint.currentRelayConn()
		select {
		case <-rc.Closed():
			if time.Now().After(deadline) {
				return errors.New("relay unavailable")
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-endpoint.closeCh:
				return errors.New("endpoint closed")
			case <-time.After(50 * time.Millisecond):
			}
		default:
			return nil
		}
	}
}

func defaultQUICConfig() *quic.Config {
	return &quic.Config{
		EnableDatagrams:       true,
		MaxIncomingStreams:    1024,
		MaxIncomingUniStreams: 1024,
		KeepAlivePeriod:       5 * time.Second,
		MaxIdleTimeout:        30 * time.Second,
		HandshakeIdleTimeout:  30 * time.Second,
	}
}
