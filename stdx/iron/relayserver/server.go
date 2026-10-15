// Package relayserver implements a minimal relay server: it
// authenticates clients by challenge/signature and forwards opaque QUIC
// datagrams between them.
package relayserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/zeebo/blake3"

	"markdown.ninja/stdx/iron/base"
	"markdown.ninja/stdx/iron/proto"
	"markdown.ninja/stdx/iron/stun"
)

const relayPath = "/relay"

// relayBroadcastTimeout caps how long a client lookup waits for the HTTP
// broadcast to the configured peer relays before the relay answers with what
// it has.
const relayBroadcastTimeout = 5 * time.Second

// Backbone link reconnection backoff bounds.
const (
	relayBackoffMin = 100 * time.Millisecond
	relayBackoffMax = 10 * time.Second
)

// Backbone keepalive: a relay-to-relay link is considered dead when nothing has
// been received for backbonePingTimeout; the peer is pinged every
// backbonePingInterval. Package vars so tests can shrink them.
var (
	backbonePingInterval = 3 * time.Second
	backbonePingTimeout  = 9 * time.Second
)

// backboneDialTimeout bounds one attempt to open a backbone link to a peer
// relay, so an unreachable (black-holed) peer cannot stall a forward path or a
// backbone manager indefinitely.
var backboneDialTimeout = 5 * time.Second

// peerReconcileInterval is how often the dynamic peer list is reconciled
// (starting/stopping backbone managers for added/removed peers). A var so
// tests can shrink it.
var peerReconcileInterval = 2 * time.Second

// helloInterval is how often the larger-address relay invites a peer it is not
// connected to, so the smaller side dials and a pair keeps a single connection.
// A var so tests can shrink it.
var helloInterval = 5 * time.Second

// canonicalRelayAddr normalizes a relay URL to a comparable "host:port" string
// (scheme stripped), used to elect which relay dials in a pair.
func canonicalRelayAddr(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return strings.ToLower(u.Hostname()) + ":" + port
}

// Keepalive tuning, mirroring the client: clients are pinged every
// pingInterval and disconnected when silent for pingTimeout. Package vars so
// tests can shrink them.
var (
	pingInterval = 15 * time.Second
	pingTimeout  = 45 * time.Second
)

// parseRemoteAddr converts a "host:port" string (e.g. r.RemoteAddr) into a
// UDP-style address holding the observed source IP.
func parseRemoteAddr(s string) *net.UDPAddr {
	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		return nil
	}
	port, _ := strconv.Atoi(portStr)
	return &net.UDPAddr{IP: net.ParseIP(host), Port: port}
}

// Server is a relay. Clients connect to /relay, authenticate, and exchange
// datagrams keyed by NodeID.
type Server struct {
	logger *slog.Logger
	// Secret, when non-empty, is the shared token a peer relay must present
	// (Authorization: Bearer <secret>, backbone subprotocol) to establish a
	// relay-to-relay backbone link. Empty disables backbone auth (tests).
	Secret string
	// Peers lists the other relay URLs this relay federates with. Lookup
	// requests from clients are broadcast to them over HTTP (see handleAPI) and
	// persistent backbone links are maintained to each. It is dynamic: use
	// SetPeers to update it at runtime (peers may be added without restarting).
	Peers []string
	// Self is this relay's own http(s):// URL. It is used to elect who dials
	// whom (the smaller address dials, the larger invites) so each relay pair
	// keeps a single backbone connection, and is advertised in the backbone
	// handshake.
	Self string

	// peersMu guards Peers, which is read by the reconciler and broadcast
	// paths while SetPeers mutates it.
	peersMu sync.RWMutex
	// peerMgrsMu guards peerMgrs: peer URL -> stop channel for its backbone
	// manager goroutine.
	peerMgrsMu sync.Mutex
	peerMgrs   map[string]chan struct{}

	// Backbone tuning, defaulted from the package vars in NewServer. Snapshot
	// on the Server so tests can adjust them per instance without racing the
	// running goroutines.
	reconcileInterval    time.Duration
	helloInterval        time.Duration
	backbonePingInterval time.Duration
	backbonePingTimeout  time.Duration
	backboneDialTimeout  time.Duration

	// Mutex for clients and endpoints
	mutex   sync.Mutex
	clients map[base.NodeID]*client

	// backboneMu guards backbone (peer URL -> link) and dialing (peer URLs
	// with a backbone dial in progress, so concurrent forwarders and the
	// manager never open duplicate links).
	backboneMu   sync.Mutex
	backbone     map[string]*backboneLink
	dialing      map[string]bool
	backboneOnce sync.Once // starts the reconciler once

	// relayRouteMu guards relayRoutes: peer NodeID -> relay URL it is
	// reachable through, learned from inbound backbone traffic so replies to a
	// dialing peer can be routed even without a client-supplied hint.
	relayRouteMu sync.Mutex
	relayRoutes  map[base.NodeID]string

	closeCh   chan struct{}
	closeOnce sync.Once

	// HolePunchRequests counts received HolePunchRequest frames, for tests
	// and observability.
	HolePunchRequests atomic.Int64
}

// client is an authenticated relay connection.
type client struct {
	id       base.NodeID
	ws       *websocket.Conn
	ctx      context.Context
	observed net.Addr // source address the relay sees on this connection
	addrs    []*net.UDPAddr

	mu sync.Mutex // serializes outbound writes

	lastRecvNano atomic.Int64 // unixnano of the last received frame (0 = none)
}

// backboneLink is a persistent relay-to-relay connection used to forward
// packets between the two relays' clients.
type backboneLink struct {
	ws  *websocket.Conn
	ctx context.Context

	mu           sync.Mutex    // serializes outbound writes
	closed       chan struct{} // closed when the link dies
	lastRecvNano atomic.Int64  // unixnano of the last received frame (0 = none)
}

// write serializes an outbound frame on the backbone link.
func (link *backboneLink) write(frame []byte) error {
	link.mu.Lock()
	defer link.mu.Unlock()
	return link.ws.Write(link.ctx, websocket.MessageBinary, frame)
}

// NewServer creates an empty relay server.
func NewServer(logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Server{
		clients:              make(map[base.NodeID]*client),
		backbone:             make(map[string]*backboneLink),
		dialing:              make(map[string]bool),
		relayRoutes:          make(map[base.NodeID]string),
		peerMgrs:             make(map[string]chan struct{}),
		closeCh:              make(chan struct{}),
		logger:               logger,
		reconcileInterval:    peerReconcileInterval,
		helloInterval:        helloInterval,
		backbonePingInterval: backbonePingInterval,
		backbonePingTimeout:  backbonePingTimeout,
		backboneDialTimeout:  backboneDialTimeout,
	}
}

// Handler returns the HTTP handler serving the relay. It exposes:
//
//   - POST /relay/api: the signed CBOR directory. The body is a lookup of a
//     peer's announced addresses and its observed public IP (MsgAPILookup); the
//     peer must currently be connected to this relay. Any other message is
//     rejected.
//   - /relay: the WebSocket relay for authenticated datagram forwarding, and,
//     when connected with the backbone subprotocol and shared secret, the
//     relay-to-relay backbone link.
func (server *Server) Handler() http.Handler {
	// Ensure eager backbone links are maintained on any serving path.
	server.Start()
	mux := http.NewServeMux()
	mux.HandleFunc(relayPath, server.handleRelay)
	mux.HandleFunc("POST /relay/api", server.handleAPI)
	return mux
}

// handleAPI answers the HTTP directory API at POST /relay/api. A signed client
// request (Iron Authorization header) is answered from this relay's clients and
// the lookup is broadcast to the configured peer relays; a relay-to-relay
// request (Bearer Authorization header matching Secret) is answered locally
// only, so broadcasts never recurse.
func (server *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 256_000)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Relay-to-relay request: authenticate with the shared secret. A lookup is
	// answered from local clients only (no recursion); a Hello invites this
	// relay to dial the sender back, adding it to the dynamic peer list.
	if server.isRelayRequest(r) {
		var v any
		if err := proto.Unmarshal(body, &v); err != nil {
			http.Error(w, "bad api message", http.StatusBadRequest)
			return
		}
		switch m := v.(type) {
		case proto.APILookupRequest:
			server.writeLookupResp(w, server.lookupRespLocal(m.ID, ""))
		case proto.Hello:
			server.learnPeer(m.Self)
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "bad api message", http.StatusBadRequest)
		}
		return
	}

	// Client request: signed, then answered locally and via broadcast.
	if _, err := server.authRequest(r, body); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var apiReq proto.APILookupRequest
	if err := proto.Unmarshal(body, &apiReq); err != nil {
		http.Error(w, "bad api message", http.StatusBadRequest)
		return
	}
	self := relayURLFromRequest(r)
	resp := server.lookupRespLocal(apiReq.ID, self)
	for _, pr := range server.broadcastLookup(server.broadcastTargets(self), apiReq.ID) {
		if !pr.found {
			continue
		}
		resp.FoundRelays = append(resp.FoundRelays, pr.url)
		resp.Addrs = append(resp.Addrs, pr.addrs...)
		if resp.Observed == "" && pr.observed != "" {
			resp.Observed = pr.observed
		}
	}
	// A node announces the same direct addresses to every relay it connects
	// to, so the aggregated answer may repeat them (and the found relays);
	// collapse the duplicates, keeping first occurrences.
	resp.Addrs = dedupeStrings(resp.Addrs)
	resp.FoundRelays = dedupeStrings(resp.FoundRelays)
	server.writeLookupResp(w, resp)
}

// lookupRespLocal builds the directory answer from this relay's own clients.
// self, when non-empty, is reported as a "found" relay when the peer is local.
func (server *Server) lookupRespLocal(peer base.NodeID, self string) proto.APILookupResponse {
	addrs, observed, found := server.lookupLocal(peer)
	resp := proto.APILookupResponse{Addrs: addrs, Observed: observed}
	if self != "" && found {
		resp.FoundRelays = append(resp.FoundRelays, self)
	}
	return resp
}

// writeLookupResp encodes and writes a directory lookup response.
func (server *Server) writeLookupResp(w http.ResponseWriter, resp proto.APILookupResponse) {
	out, err := proto.Encode(resp)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/cbor")
	_, _ = w.Write(out)
}

// isRelayRequest reports whether r is an authenticated relay-to-relay request
// (Bearer token matching the configured Secret). Without a Secret, no request
// is a relay request.
func (server *Server) isRelayRequest(r *http.Request) bool {
	return server.Secret != "" && constantTimeEqual(server.Secret, bearerToken(r))
}

// broadcastTargets returns the peer relays to broadcast a lookup to (the
// dynamic peer list), deduplicated and excluding this relay's own address.
func (server *Server) broadcastTargets(self string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, u := range server.peersSnapshot() {
		if u == "" || seen[u] || u == self {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}

// dedupeStrings collapses repeated strings, keeping each value's first
// occurrence so the relative order is preserved.
func dedupeStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// SetPeers replaces the dynamic peer list (federation partners). Peers may be
// added or removed at runtime without restarting; the change is applied
// immediately and kept in sync by the reconciler.
func (server *Server) SetPeers(peers []string) {
	server.peersMu.Lock()
	server.Peers = append([]string(nil), peers...)
	server.peersMu.Unlock()
	server.reconcile()
}

// peersSnapshot returns a copy of the current dynamic peer list.
func (server *Server) peersSnapshot() []string {
	server.peersMu.RLock()
	defer server.peersMu.RUnlock()
	return append([]string(nil), server.Peers...)
}

// isWantedPeer reports whether peerURL is currently in the dynamic peer list.
func (server *Server) isWantedPeer(peerURL string) bool {
	for _, u := range server.peersSnapshot() {
		if u == peerURL {
			return true
		}
	}
	return false
}

// learnPeer adds a peer discovered via a Hello invite to the dynamic peer list,
// so the reconciler starts (or keeps) its backbone manager.
func (server *Server) learnPeer(peerURL string) {
	if peerURL == "" || peerURL == server.Self {
		return
	}
	server.peersMu.Lock()
	for _, u := range server.Peers {
		if u == peerURL {
			server.peersMu.Unlock()
			return
		}
	}
	server.Peers = append(server.Peers, peerURL)
	server.peersMu.Unlock()
	server.ensurePeerManager(peerURL)
}

// peerLookupResult is one HTTP broadcast answer from a peer relay.
type peerLookupResult struct {
	url      string
	addrs    []string
	observed string
	found    bool
}

// broadcastLookup asks every target relay whether it knows peer over HTTP
// (Bearer-authenticated with the shared secret), returning each answer. A peer
// that does not reply within relayBroadcastTimeout contributes no result.
func (server *Server) broadcastLookup(targets []string, peer base.NodeID) []peerLookupResult {
	if len(targets) == 0 {
		return nil
	}

	var outMutex sync.Mutex
	out := make([]peerLookupResult, 0, len(targets))
	var waitgroup sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), relayBroadcastTimeout)
	defer cancel()

	for _, url := range targets {
		waitgroup.Go(func() {
			result := server.peerLookupOne(ctx, url, peer)
			outMutex.Lock()
			out = append(out, result)
			outMutex.Unlock()
		})
	}

	waitgroup.Wait()
	return out
}

// peerLookupOne performs one signed... rather, one Bearer-authenticated HTTP
// lookup against a single peer relay's /relay/api directory.
func (server *Server) peerLookupOne(ctx context.Context, relayURL string, peer base.NodeID) peerLookupResult {
	u, err := relayHTTPURL(relayURL, "/relay/api")
	if err != nil {
		return peerLookupResult{url: relayURL}
	}
	body, err := proto.Encode(proto.APILookupRequest{ID: peer})
	if err != nil {
		return peerLookupResult{url: relayURL}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return peerLookupResult{url: relayURL}
	}
	if server.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+server.Secret)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return peerLookupResult{url: relayURL}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return peerLookupResult{url: relayURL}
	}
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 256_000))
	if err != nil {
		return peerLookupResult{url: relayURL}
	}
	var lr proto.APILookupResponse
	if err := proto.Unmarshal(respBody, &lr); err != nil {
		return peerLookupResult{url: relayURL}
	}
	return peerLookupResult{
		url:      relayURL,
		addrs:    lr.Addrs,
		observed: lr.Observed,
		found:    len(lr.Addrs) > 0 || lr.Observed != "",
	}
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

// relayURLFromRequest derives this relay's own http(s):// address from an
// incoming HTTP request: the Host header plus the scheme implied by TLS.
func relayURLFromRequest(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// authRequest verifies the signed Authorization header of an HTTP request,
// returning the authenticated node id. Body must already be read.
func (server *Server) authRequest(r *http.Request, body []byte) (base.NodeID, error) {
	id, ts, sig, err := base.ParseAuthHeader(r.Header.Get("Authorization"))
	if err != nil {
		return base.NodeID{}, err
	}
	now := time.Now().Unix()
	if ts < now-int64(base.HTTPRequestSignatureTTL) || ts > now+int64(base.HTTPRequestSignatureTTL) {
		return base.NodeID{}, errors.New("stale authorization timestamp")
	}
	if !base.VerifyHTTPRequest(id, r.Method, r.URL.RequestURI(), body, ts, sig) {
		return base.NodeID{}, errors.New("bad authorization signature")
	}
	return id, nil
}

// ListenAndServe starts the relay on addr (e.g. ":3333"). In addition to the
// WebSocket relay, a UDP socket on the same address answers STUN binding
// requests, letting clients discover their public UDP address for hole
// punching. The TCP listener is bound before Start(), so peer relays can reach
// us (e.g. a Hello invite) as soon as the backbone reconciler runs.
func (server *Server) ListenAndServe(addr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	server.ServeUDP(udpConn)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	server.Start()
	return http.Serve(l, server.Handler())
}

// ServeUDP answers STUN binding requests on conn, reporting each client's
// observed (source) address.
func (server *Server) ServeUDP(conn *net.UDPConn) {
	go server.stunLoop(conn)
}

// stunLoop answers STUN binding requests with the client's observed address.
func (server *Server) stunLoop(conn *net.UDPConn) {
	buf := make([]byte, proto.MaxPacketSize)
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if !stun.IsBindingRequest(buf[:n]) {
			continue
		}
		_, id, err := stun.ParseHeader(buf[:n])
		if err != nil {
			continue
		}
		_, _ = conn.WriteToUDP(stun.EncodeXORMappedAddress(id, addr), addr)
	}
}

// Serve starts the relay on the given listener.
func (server *Server) Serve(l net.Listener) error {
	server.Start()
	return http.Serve(l, server.Handler())
}

// Close force-closes all connected clients and stops the backbone managers.
// Active websocket connections are hijacked (not tracked by the http.Server),
// so they must be torn down explicitly for the clients to observe the outage.
func (server *Server) Close() error {
	server.closeOnce.Do(func() { close(server.closeCh) })
	server.mutex.Lock()
	clients := make([]*client, 0, len(server.clients))
	for _, cl := range server.clients {
		clients = append(clients, cl)
	}
	server.mutex.Unlock()
	for _, client := range clients {
		client.mu.Lock()
		client.ws.CloseNow()
		client.mu.Unlock()
	}
	return nil
}

// Restarting advises all connected clients that the relay is restarting and
// force-closes their connections. reconnectIn is the advised delay before
// reconnecting (to smear out the reconnect storm), tryFor how long they should
// keep trying.
func (server *Server) Restarting(reconnectIn, tryFor time.Duration) {
	server.mutex.Lock()
	clients := make([]*client, 0, len(server.clients))
	for _, client := range server.clients {
		clients = append(clients, client)
	}
	server.mutex.Unlock()
	frame, err := proto.Encode(proto.Restarting{ReconnectAfter: reconnectIn, TryFor: tryFor})
	if err != nil {
		return
	}
	for _, client := range clients {
		client.mu.Lock()
		_ = client.ws.Write(client.ctx, websocket.MessageBinary, frame)
		client.ws.CloseNow()
		client.mu.Unlock()
	}
}

// ClientCount returns the number of currently connected clients.
func (server *Server) ClientCount() int {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	return len(server.clients)
}

func (server *Server) handleRelay(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols:       []string{proto.RelayProtocol, proto.RelayBackboneProtocol},
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	if ws.Subprotocol() == proto.RelayBackboneProtocol {
		server.handleBackbone(w, r, ws)
		return
	}
	if ws.Subprotocol() != proto.RelayProtocol {
		ws.Close(websocket.StatusPolicyViolation, "unsupported relay protocol")
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	id, addrs, err := server.serverHandshake(ctx, ws, parseRemoteAddr(r.RemoteAddr))
	if err != nil {
		server.logger.Warn("relay handshake failed", "err", err)
		ws.Close(websocket.StatusPolicyViolation, "authentication failed")
		return
	}

	client := &client{id: id, ws: ws, ctx: ctx, observed: parseRemoteAddr(r.RemoteAddr), addrs: addrs}
	client.lastRecvNano.Store(time.Now().UnixNano()) // base for the keepalive watchdog
	if old, replaced := server.register(client); replaced {
		// Kick the previous connection that used the same identity.
		old.mu.Lock()
		old.ws.CloseNow()
		old.mu.Unlock()
	}

	server.logger.Info("client connected", slog.String("client", id.String()))
	defer func() {
		server.unregister(id, client)
		server.logger.Info("client disconnected", slog.String("client", id.String()))
	}()

	go client.watchdog()
	server.readLoop(client)
}

// handleBackbone accepts a relay-to-relay backbone link. It authenticates the
// peer against the shared secret (when configured), records which peer it is
// (from the X-Iron-Relay header) so the link can be reused as the write path,
// and then reads forwarded packet batches (RelayToRelayBatch) from it. A pair
// of relays keeps exactly one connection: the side whose canonical URL is
// smaller dials (see backboneManager); this side accepts and reuses the
// inbound connection for both directions.
func (server *Server) handleBackbone(w http.ResponseWriter, r *http.Request, ws *websocket.Conn) {
	if server.Secret != "" && !constantTimeEqual(server.Secret, bearerToken(r)) {
		ws.Close(websocket.StatusPolicyViolation, "invalid backbone secret")
		return
	}
	peerURL := r.Header.Get(backbonePeerHeader)
	if peerURL == "" {
		// Cannot attribute the link to a peer: without a write path it is
		// useless; drop it.
		ws.Close(websocket.StatusPolicyViolation, "missing backbone peer header")
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	link := &backboneLink{ws: ws, ctx: ctx, closed: make(chan struct{})}
	link.lastRecvNano.Store(time.Now().UnixNano())
	server.installBackbone(peerURL, link)
	server.logger.Info("relay connected", slog.String("relay", peerURL))
	server.runBackbone(link, peerURL)
	server.removeBackboneIf(peerURL, link)
	server.logger.Info("relay disconnected", slog.String("relay", peerURL))
	close(link.closed)
}

// runBackbone runs a backbone link until it dies: a watchdog pings the peer
// every backbonePingInterval and force-closes the link when nothing has been
// received for backbonePingTimeout, while the read loop forwards frames and
// answers pings. srcRelay is the URL of the relay on the other end (used to
// learn the route back to senders).
func (server *Server) runBackbone(link *backboneLink, srcRelay string) {
	go server.watchdogLink(link)
	server.backboneReadLoop(link, srcRelay)
}

// watchdogLink keeps the link alive: it sends pings and tears the connection
// down if the peer goes silent for the keepalive timeout.
func (server *Server) watchdogLink(link *backboneLink) {
	tick := time.NewTicker(server.backbonePingInterval)
	defer tick.Stop()
	for {
		select {
		case <-link.ctx.Done():
			return
		case <-tick.C:
			var p [8]byte
			binary.LittleEndian.PutUint64(p[:], uint64(time.Now().UnixNano()))
			if frame, err := proto.Encode(proto.Ping{Nonce: p}); err == nil {
				_ = link.write(frame)
			}
			if time.Since(time.Unix(0, link.lastRecvNano.Load())) > server.backbonePingTimeout {
				_ = link.ws.CloseNow()
				return
			}
		}
	}
}

// backboneReadLoop reads frames from one end of a backbone link, delivering
// forwarded packet batches to local clients, learning the route back to the
// sender, answering pings and tracking liveness. It returns when the
// connection dies.
func (server *Server) backboneReadLoop(link *backboneLink, srcRelay string) {
	for {
		typ, msg, err := link.ws.Read(link.ctx)
		if err != nil {
			return
		}
		link.lastRecvNano.Store(time.Now().UnixNano())
		if typ != websocket.MessageBinary {
			continue
		}
		var v any
		if err := proto.Unmarshal(msg, &v); err != nil {
			continue
		}
		switch m := v.(type) {
		case proto.RelayToRelayBatch:
			server.forwardToLocal(m.Remote, m.Sender, srcRelay, m.Ecn, m.Packets)
		case proto.Ping:
			if frame, err := proto.Encode(proto.Pong{Nonce: m.Nonce}); err == nil {
				_ = link.write(frame)
			}
		}
	}
}

// backbonePeerHeader is the HTTP header a relay sets when dialing a backbone
// link, carrying its own URL so the acceptor can attribute the connection.
const backbonePeerHeader = "X-Iron-Relay"

// Start launches the reconciler that maintains backbone managers for the
// dynamic peer list. It is idempotent and is called automatically by Handler,
// Serve and ListenAndServe.
func (server *Server) Start() {
	server.backboneOnce.Do(func() {
		go server.reconcileLoop()
	})
}

// reconcileLoop reconciles the dynamic peer list, starting and stopping the
// per-peer backbone managers as peers are added or removed, and invites peers
// this relay is ordered to not dial but is not connected to.
func (server *Server) reconcileLoop() {
	tick := time.NewTicker(server.reconcileInterval)
	defer tick.Stop()
	server.reconcile()
	for {
		select {
		case <-server.closeCh:
			return
		case <-tick.C:
			server.reconcile()
		}
	}
}

// reconcile diffs the dynamic peer list against the running managers.
func (server *Server) reconcile() {
	peers := server.peersSnapshot()
	seen := make(map[string]bool, len(peers))
	for _, u := range peers {
		if u == "" {
			continue
		}
		seen[u] = true
		server.ensurePeerManager(u)
	}
	server.peerMgrsMu.Lock()
	var stopped []string
	for u := range server.peerMgrs {
		if !seen[u] {
			stopped = append(stopped, u)
		}
	}
	for _, u := range stopped {
		stop := server.peerMgrs[u]
		delete(server.peerMgrs, u)
		close(stop)
	}
	server.peerMgrsMu.Unlock()
	// Drop the cached backbone link to each removed peer, so a federation
	// partner that is unlisted no longer holds a live connection (and its
	// goroutines) here.
	for _, u := range stopped {
		server.closeBackbone(u)
	}
}

// closeBackbone force-closes the cached backbone link to relayURL, if any.
func (server *Server) closeBackbone(relayURL string) {
	server.backboneMu.Lock()
	link, ok := server.backbone[relayURL]
	if ok {
		delete(server.backbone, relayURL)
	}
	server.backboneMu.Unlock()
	if ok && link != nil {
		link.mu.Lock()
		_ = link.ws.CloseNow()
		link.mu.Unlock()
	}
}

// ensurePeerManager starts the backbone manager for peerURL unless one is
// already running.
func (server *Server) ensurePeerManager(peerURL string) {
	server.peerMgrsMu.Lock()
	if _, ok := server.peerMgrs[peerURL]; ok {
		server.peerMgrsMu.Unlock()
		return
	}
	stop := make(chan struct{})
	server.peerMgrs[peerURL] = stop
	server.peerMgrsMu.Unlock()
	go server.maintainBackbone(peerURL, stop)
}

// maintainBackbone keeps the backbone link to one peer alive. The relay with
// the smaller canonical address dials the peer; the larger relay instead
// periodically invites itself (a Hello) so the smaller side dials, keeping a
// single connection per pair.
func (server *Server) maintainBackbone(peerURL string, stop <-chan struct{}) {
	backoff := relayBackoffMin
	for {
		// Bail if the peer was removed from the dynamic list while we were
		// waiting on the previous link: a manager woken by link.closed must
		// not re-dial a peer that is no longer wanted.
		if !server.isWantedPeer(peerURL) {
			return
		}
		if !server.selfLessThan(peerURL) {
			// Larger side: invite the smaller relay to dial us. Retry quickly
			// while the peer is unreachable, then settle into helloInterval.
			if server.backboneLinkFor(peerURL) == nil {
				if err := server.helloPeer(peerURL); err != nil {
					select {
					case <-stop:
						return
					case <-server.closeCh:
						return
					case <-time.After(backoff):
					}
					backoff *= 2
					if backoff > relayBackoffMax {
						backoff = relayBackoffMax
					}
					continue
				}
			}
			backoff = relayBackoffMin
			select {
			case <-stop:
				return
			case <-server.closeCh:
				return
			case <-time.After(server.helloInterval):
			}
			continue
		}
		link := server.backboneFor(peerURL)
		if link == nil {
			select {
			case <-stop:
				return
			case <-server.closeCh:
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > relayBackoffMax {
				backoff = relayBackoffMax
			}
			server.logger.Debug("relay dial failed; retrying", slog.String("relay", peerURL))
			continue
		}
		backoff = relayBackoffMin
		select {
		case <-stop:
			return
		case <-server.closeCh:
			return
		case <-link.closed:
			server.removeBackboneIf(peerURL, link)
			server.logger.Info("relay connection lost; reconnecting", slog.String("relay", peerURL))
		}
	}
}

// selfLessThan reports whether this relay is ordered to dial peerURL (its own
// canonical address is smaller, or it has no Self so it must dial to connect).
func (server *Server) selfLessThan(peerURL string) bool {
	if server.Self == "" {
		return true
	}
	return canonicalRelayAddr(server.Self) < canonicalRelayAddr(peerURL)
}

// backboneLinkFor returns the cached backbone link to peerURL, if any.
func (server *Server) backboneLinkFor(peerURL string) *backboneLink {
	server.backboneMu.Lock()
	defer server.backboneMu.Unlock()
	return server.backbone[peerURL]
}

// helloPeer invites peerURL to dial us, sending a Bearer-authenticated Hello
// over the HTTP API. It is sent by the larger-address relay so the smaller one
// dials. It returns nil when the invite reached the peer.
func (server *Server) helloPeer(peerURL string) error {
	if server.Self == "" {
		return errors.New("relay: no Self configured")
	}
	u, err := relayHTTPURL(peerURL, "/relay/api")
	if err != nil {
		return err
	}
	body, err := proto.Encode(proto.Hello{Self: server.Self})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), relayBroadcastTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	if server.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+server.Secret)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		server.logger.Debug("relay hello failed", slog.String("relay", peerURL), slog.String("err", err.Error()))
		return err
	}
	io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("relay: hello rejected: " + resp.Status)
	}
	return nil
}

// watchdog pings the client periodically and force-closes the connection if
// the client stays silent for pingTimeout, freeing the relay's resources.
func (client *client) watchdog() {
	tick := time.NewTicker(pingInterval)
	defer tick.Stop()
	for {
		select {
		case <-client.ctx.Done():
			return
		case <-tick.C:
			var p [8]byte
			binary.LittleEndian.PutUint64(p[:], uint64(time.Now().UnixNano()))
			if frame, err := proto.Encode(proto.Ping{Nonce: p}); err == nil {
				_ = client.write(frame)
			}
			if time.Since(time.Unix(0, client.lastRecvNano.Load())) > pingTimeout {
				client.mu.Lock()
				client.ws.CloseNow()
				client.mu.Unlock()
				return
			}
		}
	}
}

// serverHandshake runs the 3-message auth handshake and returns the
// authenticated client's NodeID and the direct addresses it published.
func (server *Server) serverHandshake(ctx context.Context, ws *websocket.Conn, remote net.Addr) (base.NodeID, []*net.UDPAddr, error) {
	var challenge [16]byte
	if _, err := rand.Read(challenge[:]); err != nil {
		return base.NodeID{}, nil, err
	}
	frame, err := proto.Encode(proto.ServerHello{Challenge: challenge})
	if err != nil {
		return base.NodeID{}, nil, err
	}
	if err := ws.Write(ctx, websocket.MessageBinary, frame); err != nil {
		return base.NodeID{}, nil, err
	}

	wsMessageType, wsMessage, err := ws.Read(ctx)
	if err != nil {
		return base.NodeID{}, nil, err
	}
	if wsMessageType != websocket.MessageBinary {
		return base.NodeID{}, nil, errors.New("expected binary client hello")
	}
	var hello proto.ClientHello
	if err := proto.Unmarshal(wsMessage, &hello); err != nil {
		_ = server.denyAuth(ctx, ws, "invalid client hello frame")
		return base.NodeID{}, nil, err
	}
	id := hello.ID
	var key [32]byte
	blake3.DeriveKey(proto.HandshakeDomainSep, challenge[:], key[:])
	if !base.Verify(id, key[:], hello.Sig[:]) {
		_ = server.denyAuth(ctx, ws, "invalid client signature")
		return base.NodeID{}, nil, errors.New("invalid client signature")
	}

	addrs := make([]*net.UDPAddr, 0, len(hello.Addrs))
	for _, sAddr := range hello.Addrs {
		if a, err := net.ResolveUDPAddr("udp", sAddr); err == nil {
			addrs = append(addrs, a)
		}
	}

	var observed net.IP
	if ua, ok := remote.(*net.UDPAddr); ok && ua.IP != nil {
		observed = ua.IP
	}
	finish, err := proto.Encode(proto.Finished{Result: true, Observed: observed})
	if err != nil {
		return base.NodeID{}, nil, err
	}
	if err := ws.Write(ctx, websocket.MessageBinary, finish); err != nil {
		return base.NodeID{}, nil, err
	}
	return id, addrs, nil
}

// denyAuth sends a Finished(false) message and returns its error (nil when the
// write succeeded, so callers can ignore it).
func (server *Server) denyAuth(ctx context.Context, ws *websocket.Conn, reason string) error {
	deny, err := proto.Encode(proto.Finished{Result: false})
	if err != nil {
		return err
	}
	return ws.Write(ctx, websocket.MessageBinary, deny)
}

// readLoop reads datagrams from a client and forwards them to their
// destinations.
func (server *Server) readLoop(client *client) {
	for {
		typ, msg, err := client.ws.Read(client.ctx)
		if err != nil {
			return
		}
		client.lastRecvNano.Store(time.Now().UnixNano())
		if typ != websocket.MessageBinary {
			continue
		}
		var v any
		if err := proto.Unmarshal(msg, &v); err != nil {
			continue
		}
		switch m := v.(type) {
		case proto.ClientToRelayBatch:
			server.forwardBatch(client, m.Remote, m.Relay, m.Ecn, m.Packets)
		case proto.Ping:
			if frame, err := proto.Encode(proto.Pong{Nonce: m.Nonce}); err == nil {
				_ = client.write(frame)
			}
		case proto.HolePunchRequest:
			server.HolePunchRequests.Add(1)
			server.punch(client, m.Target)
		case proto.LocalAddrs:
			addrs := make([]*net.UDPAddr, 0, len(m.Addrs))
			for _, s := range m.Addrs {
				if a, err := net.ResolveUDPAddr("udp", s); err == nil {
					addrs = append(addrs, a)
				}
			}
			server.mutex.Lock()
			client.addrs = addrs
			server.mutex.Unlock()
		}
	}
}

// punch tells both the requester and the target about each other's direct
// addresses so they can punch through their NATs simultaneously. Each side
// receives a HolePunch frame carrying the other side's announced addresses.
func (server *Server) punch(from *client, target base.NodeID) {
	server.mutex.Lock()
	tc, ok := server.clients[target]
	server.mutex.Unlock()
	if !ok {
		return // target not connected: nothing to coordinate
	}
	fromAddrs := server.answerFor(from.id)
	targetAddrs := server.answerFor(target)
	if len(fromAddrs) == 0 || len(targetAddrs) == 0 {
		return
	}
	toTarget, err := proto.Encode(proto.HolePunch{Target: from.id, Addrs: addrStrings(fromAddrs)})
	if err == nil {
		_ = tc.write(toTarget)
	}
	toFrom, err := proto.Encode(proto.HolePunch{Target: target, Addrs: addrStrings(targetAddrs)})
	if err == nil {
		_ = from.write(toFrom)
	}
}

// answerFor returns the announced direct addresses for a peer, taken from the
// connected client (purged on disconnect).
func (server *Server) answerFor(target base.NodeID) []*net.UDPAddr {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	cl, ok := server.clients[target]
	if !ok || len(cl.addrs) == 0 {
		return nil
	}
	return append([]*net.UDPAddr(nil), cl.addrs...)
}

// lookupLocal returns a peer's directory entry on this relay.
func (server *Server) lookupLocal(peer base.NodeID) (addrs []string, observed string, found bool) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	cl, ok := server.clients[peer]
	if !ok {
		return nil, "", false
	}
	for _, a := range cl.addrs {
		if a != nil {
			addrs = append(addrs, a.String())
		}
	}
	if ua, ok2 := cl.observed.(*net.UDPAddr); ok2 && ua.IP != nil {
		observed = ua.IP.String()
	}
	return addrs, observed, true
}

// forwardBatch ships a batch of datagrams received from `from` to the
// destination `dstID`. When the destination is not connected locally but the
// frame carries a non-empty `relay` hint (the URL of another relay), the batch
// is forwarded over the backbone to that relay. The relay does no buffering or
// re-segmentation: each client frame is forwarded as-is.
func (server *Server) forwardBatch(from *client, dstID base.NodeID, relay string, ecn byte, packets [][]byte) {
	server.mutex.Lock()
	dst, ok := server.clients[dstID]
	server.mutex.Unlock()
	if ok {
		out, err := proto.Encode(proto.RelayToClientBatch{
			Remote: from.id, Ecn: ecn, Packets: packets,
		})
		if err == nil {
			_ = dst.write(out)
		}
		return
	}
	// Not local. If the sender told us where to relay it, forward over the
	// backbone; otherwise use a route learned from inbound backbone traffic,
	// and drop if we have neither.
	if relay == "" {
		server.relayRouteMu.Lock()
		relay = server.relayRoutes[dstID]
		server.relayRouteMu.Unlock()
	}
	if relay == "" {
		return
	}
	server.forwardOverBackbone(relay, dstID, from.id, ecn, packets)
}

// forwardToLocal delivers a batch that arrived over the backbone to a local
// client, tagging the packets with the originating sender's NodeID. It also
// learns that the sender is reachable through srcRelay, so replies to it can be
// routed back without a client-supplied hint.
func (server *Server) forwardToLocal(dstID, sender base.NodeID, srcRelay string, ecn byte, packets [][]byte) {
	if srcRelay != "" {
		server.relayRouteMu.Lock()
		server.relayRoutes[sender] = srcRelay
		server.relayRouteMu.Unlock()
	}
	server.mutex.Lock()
	dst, ok := server.clients[dstID]
	server.mutex.Unlock()
	if !ok {
		return
	}
	out, err := proto.Encode(proto.RelayToClientBatch{
		Remote: sender, Ecn: ecn, Packets: packets,
	})
	if err == nil {
		_ = dst.write(out)
	}
}

// forwardOverBackbone sends a batch to another relay over the backbone,
// opening a link on demand.
func (server *Server) forwardOverBackbone(relayURL string, dstID, sender base.NodeID, ecn byte, packets [][]byte) {
	link := server.backboneFor(relayURL)
	if link == nil {
		return
	}
	frame, err := proto.Encode(proto.RelayToRelayBatch{
		Remote: dstID, Sender: sender, Ecn: ecn, Packets: packets,
	})
	if err != nil {
		return
	}
	if err := link.write(frame); err != nil {
		server.removeBackboneIf(relayURL, link)
	}
}

// backboneFor returns the cached backbone link to relayURL, opening one on
// demand. It returns nil if the peer cannot be reached or authenticated, or if
// a dial to it is already in progress (the caller retries and picks up the
// established link).
func (server *Server) backboneFor(relayURL string) *backboneLink {
	server.backboneMu.Lock()
	if link, ok := server.backbone[relayURL]; ok {
		server.backboneMu.Unlock()
		return link
	}
	if server.dialing[relayURL] {
		server.backboneMu.Unlock()
		return nil
	}
	server.dialing[relayURL] = true
	server.backboneMu.Unlock()
	defer func() {
		server.backboneMu.Lock()
		delete(server.dialing, relayURL)
		server.backboneMu.Unlock()
	}()

	headers := http.Header{}
	if server.Secret != "" {
		headers.Set("Authorization", "Bearer "+server.Secret)
	}
	if server.Self != "" {
		headers.Set(backbonePeerHeader, server.Self)
	}
	dctx, cancel := context.WithTimeout(context.Background(), server.backboneDialTimeout)
	defer cancel()
	ws, _, err := websocket.Dial(dctx, relayURL+relayPath, &websocket.DialOptions{
		Subprotocols: []string{proto.RelayBackboneProtocol},
		HTTPHeader:   headers,
	})
	if err != nil {
		return nil
	}
	if ws.Subprotocol() != proto.RelayBackboneProtocol {
		ws.Close(websocket.StatusPolicyViolation, "unsupported backbone protocol")
		return nil
	}
	// The peer may have been removed while the dial was in flight; a stale
	// dial must not install a link for a peer that is no longer wanted.
	if !server.isWantedPeer(relayURL) {
		_ = ws.Close(websocket.StatusPolicyViolation, "peer removed")
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	link := &backboneLink{ws: ws, ctx: ctx, closed: make(chan struct{})}
	link.lastRecvNano.Store(time.Now().UnixNano())
	server.installBackbone(relayURL, link)
	server.logger.Info("relay connected", slog.String("relay", relayURL))
	// A single bidirectional connection: read what the peer sends us on it.
	go func() {
		defer cancel()
		server.runBackbone(link, relayURL)
		close(link.closed)
	}()
	return link
}

// installBackbone records a backbone link, returning any previous link for the
// same relay (which is not closed here; callers do that).
func (server *Server) installBackbone(relayURL string, link *backboneLink) {
	server.backboneMu.Lock()
	defer server.backboneMu.Unlock()
	if old, ok := server.backbone[relayURL]; ok {
		old.mu.Lock()
		_ = old.ws.CloseNow()
		old.mu.Unlock()
	}
	server.backbone[relayURL] = link
}

// removeBackboneIf drops the cached backbone link to relayURL only when it is
// the given link, so a stale cleanup never deletes a newer replacement.
func (server *Server) removeBackboneIf(relayURL string, link *backboneLink) {
	server.backboneMu.Lock()
	if cur, ok := server.backbone[relayURL]; ok && cur == link {
		delete(server.backbone, relayURL)
	}
	server.backboneMu.Unlock()
}

// removeBackbone drops the cached backbone link to relayURL, if present.
func (server *Server) removeBackbone(relayURL string) {
	server.backboneMu.Lock()
	delete(server.backbone, relayURL)
	server.backboneMu.Unlock()
}

// bearerToken extracts the token from an "Authorization: Bearer <token>"
// header, returning "" when absent or malformed.
func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(prefix) || h[:len(prefix)] != prefix {
		return ""
	}
	return h[len(prefix):]
}

// constantTimeEqual compares two strings in constant time.
func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

// addrStrings renders UDP addresses as "ip:port" strings for the wire.
func addrStrings(addresses []*net.UDPAddr) []string {
	out := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if address != nil {
			out = append(out, address.String())
		}
	}
	return out
}

func (client *client) write(frame []byte) error {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.ws.Write(client.ctx, websocket.MessageBinary, frame)
}

func (server *Server) register(cl *client) (*client, bool) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	old, ok := server.clients[cl.id]
	server.clients[cl.id] = cl
	return old, ok
}

// unregister removes a client from the directory the moment it disconnects, so
// lookups never return stale addresses.
func (server *Server) unregister(id base.NodeID, cl *client) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	if server.clients[id] == cl {
		delete(server.clients, id)
	}
}
