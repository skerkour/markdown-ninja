package relayserver

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/zeebo/blake3"

	"markdown.ninja/stdx/iron/base"
	"markdown.ninja/stdx/iron/proto"
	"markdown.ninja/stdx/iron/stun"
)

// addConnectedClient connects a client over websocket and runs the 3-message
// auth handshake, publishing `addrs` for the node identified by `secret`. It
// returns a closure that, when called, disconnects the client so the relay
// purges it from the directory.
func addConnectedClient(t *testing.T, srv *Server, secret *base.NodeSecret, addrs []string) func() {
	t.Helper()
	id := secret.Public()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	httpSrv := &http.Server{Handler: srv.Handler()}
	go func() { _ = httpSrv.Serve(ln) }()
	t.Cleanup(func() { httpSrv.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	ws, _, err := websocket.Dial(ctx, "ws://"+ln.Addr().String()+relayPath, &websocket.DialOptions{
		Subprotocols: []string{proto.RelayProtocol},
	})
	if err != nil {
		t.Fatal(err)
	}
	// ServerHello -> reply ClientHello (signed) with the announced addresses.
	typ, msg, err := ws.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if typ != websocket.MessageBinary {
		t.Fatalf("expected binary server hello, got %v", typ)
	}
	var hello proto.ServerHello
	if err := proto.Unmarshal(msg, &hello); err != nil {
		t.Fatal(err)
	}
	var key [32]byte
	blake3.DeriveKey(proto.HandshakeDomainSep, hello.Challenge[:], key[:])
	sig := secret.Sign(key[:])
	clientHello, err := proto.Encode(proto.ClientHello{ID: id, Sig: sig, Addrs: addrs})
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.Write(ctx, websocket.MessageBinary, clientHello); err != nil {
		t.Fatal(err)
	}
	// Finished.
	typ, msg, err = ws.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if typ != websocket.MessageBinary {
		t.Fatalf("expected binary finished, got %v", typ)
	}
	var finished proto.Finished
	if err := proto.Unmarshal(msg, &finished); err != nil {
		t.Fatal(err)
	}
	if !finished.Result {
		t.Fatalf("handshake denied")
	}

	return func() { ws.CloseNow(); cancel() }
}

// waitFor polls cond until it returns true or a timeout elapses.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	waitForTimeout(t, cond, 3*time.Second)
}

func waitForTimeout(t *testing.T, cond func() bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}

// TestServeUDPSTUN verifies the relay answers STUN binding requests with the
// client's observed address.
func TestServeUDPSTUN(t *testing.T) {
	srv := NewServer(nil)
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	srv.ServeUDP(conn)

	client, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	id, err := stun.NewTransactionID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.WriteToUDP(stun.EncodeBindingRequest(id), conn.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatal(err)
	}

	if err := client.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 2048)
	n, _, err := client.ReadFromUDP(buf)
	if err != nil {
		t.Fatal(err)
	}
	resp := buf[:n]
	if !stun.IsBindingResponse(resp) {
		t.Fatal("received a non-STUN response")
	}
	if _, gotID, err := stun.ParseHeader(resp); err != nil || gotID != id {
		t.Fatalf("transaction id mismatch: %v %v", gotID, err)
	}
	addr, err := stun.ParseXORMappedAddress(resp, id)
	if err != nil {
		t.Fatal(err)
	}
	want := client.LocalAddr().(*net.UDPAddr)
	if !addr.IP.Equal(want.IP) || addr.Port != want.Port {
		t.Fatalf("observed %v, want %v", addr, want)
	}
}

// signedRequest builds an HTTP request with a valid iron Authorization header.
func signedRequest(ts *httptest.Server, secret *base.NodeSecret, method, path string, body []byte) *http.Request {
	req, err := http.NewRequest(method, ts.URL+path, bytes.NewReader(body))
	if err != nil {
		panic(err)
	}
	now := time.Now().Unix()
	req.Header.Set("Authorization", base.BuildAuthHeader(secret.Public(), now,
		base.SignHTTPRequest(secret, method, path, body, now)))
	return req
}

func doRequest(t *testing.T, req *http.Request) *http.Response {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// TestHTTPEndpoints verifies the signed CBOR directory lookup at POST
// /relay/api: it returns the announced addresses and observed IP of a client
// that is connected (whose addresses were published in the handshake), and
// returns empty once that client disconnects.
func TestHTTPEndpoints(t *testing.T) {
	srv := NewServer(nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	secret, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	id := secret.Public()

	// A connected WS client that announced its addresses in the handshake.
	announced := []string{"192.168.1.5:7000"}
	done := addConnectedClient(t, srv, secret, announced)

	// POST lookup (signed) -> returns the announced addrs + observed IP.
	lookup, _ := proto.Encode(proto.APILookupRequest{ID: id})
	resp := doRequest(t, signedRequest(ts, secret, http.MethodPost, "/relay/api", lookup))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("lookup status = %d, want 200", resp.StatusCode)
	}
	lookupBody, _ := readAll(resp)
	var got proto.APILookupResponse
	if err := proto.Unmarshal(lookupBody, &got); err != nil {
		t.Fatalf("lookup decode: %v", err)
	}
	if len(got.Addrs) != 1 || got.Addrs[0] != announced[0] {
		t.Fatalf("addrs = %v, want [%s]", got.Addrs, announced[0])
	}
	if got.Observed == "" {
		t.Fatalf("expected a non-empty observed IP")
	}

	// Unknown peer: empty result, still 200 (signed).
	unknown := make([]byte, base.NodeIDLen)
	unknown[0] = 1
	uid, _ := base.NodeIDFromBytes(unknown)
	lookup2, _ := proto.Encode(proto.APILookupRequest{ID: uid})
	resp = doRequest(t, signedRequest(ts, secret, http.MethodPost, "/relay/api", lookup2))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unknown lookup status = %d, want 200", resp.StatusCode)
	}
	got2Body, _ := readAll(resp)
	var got2 proto.APILookupResponse
	_ = proto.Unmarshal(got2Body, &got2)
	if len(got2.Addrs) != 0 || got2.Observed != "" {
		t.Fatalf("unknown peer returned %+v", got2)
	}

	// Once the client disconnects, it is purged from the directory.
	done()
	waitFor(t, func() bool {
		resp = doRequest(t, signedRequest(ts, secret, http.MethodPost, "/relay/api", lookup))
		b, _ := readAll(resp)
		var g proto.APILookupResponse
		_ = proto.Unmarshal(b, &g)
		return len(g.Addrs) == 0 && g.Observed == ""
	})

	// A tagged message that is not a lookup -> 400.
	ping, _ := proto.Encode(proto.Ping{})
	resp = doRequest(t, signedRequest(ts, secret, http.MethodPost, "/relay/api", ping))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("non-api message status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// An untagged body -> 400.
	resp = doRequest(t, signedRequest(ts, secret, http.MethodPost, "/relay/api", []byte{0xa1}))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("untagged body status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// Unsigned lookup -> 401.
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/relay/api", bytes.NewReader(lookup))
	resp = doRequest(t, req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unsigned lookup status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()

	// Tampered body: signature is for a different body -> 401.
	otherBody, _ := proto.Encode(proto.APILookupRequest{ID: uid})
	tampered := signedRequest(ts, secret, http.MethodPost, "/relay/api", otherBody)
	tampered.Body = http.NoBody
	resp = doRequest(t, tampered)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("tampered lookup status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()

	// Stale timestamp -> 401.
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/relay/api", bytes.NewReader(lookup))
	old := time.Now().Add(-time.Hour).Unix()
	req.Header.Set("Authorization", base.BuildAuthHeader(id, old,
		base.SignHTTPRequest(secret, http.MethodPost, "/relay/api", lookup, old)))
	resp = doRequest(t, req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("stale lookup status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func readAll(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(resp.Body)
	return buf.Bytes(), err
}

// TestBackboneForwarding verifies that a batch from a client on one relay is
// forwarded (via the relay-to-relay backbone) to a client on a different relay
// when the sender tags the destination relay, and that the reply flows back.
func TestBackboneForwarding(t *testing.T) {
	srvA := NewServer(nil)
	srvB := NewServer(nil)
	tsA := httptest.NewServer(srvA.Handler())
	defer tsA.Close()
	tsB := httptest.NewServer(srvB.Handler())
	defer tsB.Close()

	// Both relays know their own URLs and each other, so a single backbone
	// link is elected (the smaller address dials) and kept.
	selfA := "http://" + tsA.Listener.Addr().String()
	selfB := "http://" + tsB.Listener.Addr().String()
	srvA.Self = selfA
	srvB.Self = selfB
	srvA.SetPeers([]string{selfB})
	srvB.SetPeers([]string{selfA})
	srvA.Start()
	srvB.Start()

	secA, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	secB, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	_, wsA := connectedClientFor(t, tsA, secA)
	_, wsB := connectedClientFor(t, tsB, secB)
	defer wsA.CloseNow()
	defer wsB.CloseNow()

	// A -> B, tagging B's relay.
	if err := wsA.Write(context.Background(), websocket.MessageBinary,
		mustEncode(t, proto.ClientToRelayBatch{Remote: secB.Public(), Relay: selfB, Packets: [][]byte{{0xaa}}})); err != nil {
		t.Fatal(err)
	}
	got := readBatch(t, wsB)
	if got.Remote != secA.Public() || len(got.Packets) != 1 || got.Packets[0][0] != 0xaa {
		t.Fatalf("B received %+v, want packets from %s", got, secA.Public())
	}

	// B -> A, tagging A's relay.
	if err := wsB.Write(context.Background(), websocket.MessageBinary,
		mustEncode(t, proto.ClientToRelayBatch{Remote: secA.Public(), Relay: selfA, Packets: [][]byte{{0xbb}}})); err != nil {
		t.Fatal(err)
	}
	got = readBatch(t, wsA)
	if got.Remote != secB.Public() || len(got.Packets) != 1 || got.Packets[0][0] != 0xbb {
		t.Fatalf("A received %+v, want packets from %s", got, secB.Public())
	}
}

// connectedClientFor runs the 3-message handshake against a relay server and
// returns the client's NodeID and the client websocket.
func connectedClientFor(t *testing.T, ts *httptest.Server, secret *base.NodeSecret) (base.NodeID, *websocket.Conn) {
	t.Helper()
	ctx := context.Background()
	ws, _, err := websocket.Dial(ctx, "ws://"+ts.Listener.Addr().String()+relayPath, &websocket.DialOptions{
		Subprotocols: []string{proto.RelayProtocol},
	})
	if err != nil {
		t.Fatal(err)
	}
	typ, msg, err := ws.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if typ != websocket.MessageBinary {
		t.Fatalf("expected server hello, got %v", typ)
	}
	var hello proto.ServerHello
	if err := proto.Unmarshal(msg, &hello); err != nil {
		t.Fatal(err)
	}
	var key [32]byte
	blake3.DeriveKey(proto.HandshakeDomainSep, hello.Challenge[:], key[:])
	id := secret.Public()
	ch, err := proto.Encode(proto.ClientHello{
		ID: id, Sig: secret.Sign(key[:]),
		Addrs: []string{"203.0.113.9:1234"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.Write(ctx, websocket.MessageBinary, ch); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ws.Read(ctx); err != nil {
		t.Fatal(err)
	}
	return id, ws
}

// readBatch reads the next RelayToClientBatch frame on a client websocket.
func readBatch(t *testing.T, ws *websocket.Conn) proto.RelayToClientBatch {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rctx, rcancel := context.WithTimeout(context.Background(), time.Second)
		typ, msg, err := ws.Read(rctx)
		rcancel()
		if err != nil {
			continue
		}
		if typ != websocket.MessageBinary {
			continue
		}
		var v any
		if err := proto.Unmarshal(msg, &v); err != nil {
			continue
		}
		if b, ok := v.(proto.RelayToClientBatch); ok {
			return b
		}
	}
	t.Fatal("no batch received within timeout")
	return proto.RelayToClientBatch{}
}

func mustEncode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := proto.Encode(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// lookupViaHTTP issues a signed HTTP lookup for peer against ts and returns the
// decoded response.
func lookupViaHTTP(t *testing.T, ts *httptest.Server, secret *base.NodeSecret, peer base.NodeID) proto.APILookupResponse {
	t.Helper()
	lookup, _ := proto.Encode(proto.APILookupRequest{ID: peer})
	resp := doRequest(t, signedRequest(ts, secret, http.MethodPost, "/relay/api", lookup))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("lookup status = %d, want 200", resp.StatusCode)
	}
	body, _ := readAll(resp)
	var got proto.APILookupResponse
	if err := proto.Unmarshal(body, &got); err != nil {
		t.Fatalf("lookup decode: %v", err)
	}
	return got
}

// TestHTTPLookupLocal verifies a client's HTTP directory lookup is answered
// locally by its own relay when the peer is connected there, and that the relay
// reports its own address as the found relay.
func TestHTTPLookupLocal(t *testing.T) {
	srv := NewServer(nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	sec, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	id, ws := connectedClientFor(t, ts, sec)
	defer ws.CloseNow()

	// Lookup ourself: found locally, with the relay's own address reported.
	resp := lookupViaHTTP(t, ts, sec, id)
	self := "http://" + ts.Listener.Addr().String()
	if len(resp.FoundRelays) != 1 || resp.FoundRelays[0] != self {
		t.Fatalf("found relays = %v, want [%s]", resp.FoundRelays, self)
	}
	if len(resp.Addrs) != 1 || resp.Addrs[0] != "203.0.113.9:1234" {
		t.Fatalf("addrs = %v, want [203.0.113.9:1234]", resp.Addrs)
	}
	if resp.Observed == "" {
		t.Fatal("expected a non-empty observed IP")
	}

	// Unknown peer: not found anywhere, empty response.
	resp = lookupViaHTTP(t, ts, sec, unknownID(t))
	if len(resp.FoundRelays) != 0 || len(resp.Addrs) != 0 || resp.Observed != "" {
		t.Fatalf("unknown peer lookup returned %+v", resp)
	}
}

// TestHTTPLookupBroadcast verifies a client's HTTP lookup is broadcast by its
// relay to the configured peer relays, and the aggregated answer includes the
// peer's entry from the other relay.
func TestHTTPLookupBroadcast(t *testing.T) {
	const secret = "shared-secret"
	srvA := NewServer(nil)
	srvA.Secret = secret
	srvB := NewServer(nil)
	srvB.Secret = secret
	tsA := httptest.NewServer(srvA.Handler())
	defer tsA.Close()
	tsB := httptest.NewServer(srvB.Handler())
	defer tsB.Close()
	relayBURL := "http://" + tsB.Listener.Addr().String()
	srvA.SetPeers([]string{relayBURL})

	secA, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	secB, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	_, wsA := connectedClientFor(t, tsA, secA)
	_, wsB := connectedClientFor(t, tsB, secB)
	defer wsA.CloseNow()
	defer wsB.CloseNow()
	_ = wsB

	// A asks its relay about B (on relayB); relayA broadcasts to relayB.
	resp := lookupViaHTTP(t, tsA, secA, secB.Public())
	if len(resp.FoundRelays) != 1 || resp.FoundRelays[0] != relayBURL {
		t.Fatalf("found relays = %v, want [%s]", resp.FoundRelays, relayBURL)
	}
	if len(resp.Addrs) != 1 || resp.Addrs[0] != "203.0.113.9:1234" {
		t.Fatalf("addrs = %v, want [203.0.113.9:1234]", resp.Addrs)
	}
	if resp.Observed == "" {
		t.Fatal("expected a non-empty observed IP")
	}
}

// TestHTTPLookupBroadcastDedup verifies that when several peer relays report
// the same peer (and the same announced addresses), the aggregated answer
// deduplicates both the addresses and the found relays.
func TestHTTPLookupBroadcastDedup(t *testing.T) {
	const secret = "shared-secret"
	srvA := NewServer(nil)
	srvA.Secret = secret
	srvB := NewServer(nil)
	srvB.Secret = secret
	srvC := NewServer(nil)
	srvC.Secret = secret
	tsA := httptest.NewServer(srvA.Handler())
	defer tsA.Close()
	tsB := httptest.NewServer(srvB.Handler())
	defer tsB.Close()
	tsC := httptest.NewServer(srvC.Handler())
	defer tsC.Close()
	relayBURL := "http://" + tsB.Listener.Addr().String()
	relayCURL := "http://" + tsC.Listener.Addr().String()
	srvA.SetPeers([]string{relayBURL, relayCURL})

	secA, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	peerSec, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	// The same peer announces the same addresses to both relays B and C.
	announced := []string{"203.0.113.9:1234"}
	doneB := addConnectedClient(t, srvB, peerSec, announced)
	doneC := addConnectedClient(t, srvC, peerSec, announced)
	defer doneB()
	defer doneC()

	// A asks its relay about the peer; the broadcast to B and C both find it,
	// but the aggregated answer must not repeat addresses or relays.
	resp := lookupViaHTTP(t, tsA, secA, peerSec.Public())
	if len(resp.Addrs) != 1 || resp.Addrs[0] != announced[0] {
		t.Fatalf("addrs = %v, want [%s]", resp.Addrs, announced[0])
	}
	want := map[string]bool{relayBURL: true, relayCURL: true}
	if len(resp.FoundRelays) != len(want) {
		t.Fatalf("found relays = %v, want %v", resp.FoundRelays, want)
	}
	for _, fr := range resp.FoundRelays {
		if !want[fr] {
			t.Fatalf("found relays = %v, want %v", resp.FoundRelays, want)
		}
	}
}

// TestHTTPLookupBroadcastAuth verifies the relay-to-relay broadcast rejects a
// wrong shared secret.
func TestHTTPLookupBroadcastAuth(t *testing.T) {
	srvA := NewServer(nil)
	srvA.Secret = "right"
	srvB := NewServer(nil)
	srvB.Secret = "right"
	tsA := httptest.NewServer(srvA.Handler())
	defer tsA.Close()
	tsB := httptest.NewServer(srvB.Handler())
	defer tsB.Close()
	relayBURL := "http://" + tsB.Listener.Addr().String()

	secA, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	_, wsA := connectedClientFor(t, tsA, secA)
	defer wsA.CloseNow()

	// A relays with the WRONG secret for B: B rejects, so the lookup is empty.
	srvA.Secret = "wrong"
	srvA.SetPeers([]string{relayBURL})
	resp := lookupViaHTTP(t, tsA, secA, unknownID(t))
	if len(resp.FoundRelays) != 0 || len(resp.Addrs) != 0 {
		t.Fatalf("broadcast with wrong secret returned %+v", resp)
	}
}

// TestBackboneLearnedRoute verifies a reply from B to A is routed over the
// backbone even when B supplies no relay hint, using the route learned when
// A's data first crossed the backbone.
func TestBackboneLearnedRoute(t *testing.T) {
	srvA, srvB, tsA, tsB := federatedServers(t)
	waitFor(t, func() bool { return backboneLen(srvA) == 1 && backboneLen(srvB) == 1 })

	secA, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	secB, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	_, wsA := connectedClientFor(t, tsA, secA)
	_, wsB := connectedClientFor(t, tsB, secB)
	defer wsA.CloseNow()
	defer wsB.CloseNow()

	relayB := "http://" + tsB.Listener.Addr().String()

	// A -> B with the hint: relay2 learns A is reachable via relay1.
	if err := wsA.Write(context.Background(), websocket.MessageBinary,
		mustEncode(t, proto.ClientToRelayBatch{Remote: secB.Public(), Relay: relayB, Packets: [][]byte{{0xaa}}})); err != nil {
		t.Fatal(err)
	}
	if got := readBatch(t, wsB); got.Remote != secA.Public() {
		t.Fatalf("B received from %s, want %s", got.Remote, secA.Public())
	}

	// B -> A WITHOUT a hint: relay2 must route via the learned route.
	if err := wsB.Write(context.Background(), websocket.MessageBinary,
		mustEncode(t, proto.ClientToRelayBatch{Remote: secA.Public(), Packets: [][]byte{{0xbb}}})); err != nil {
		t.Fatal(err)
	}
	got := readBatch(t, wsA)
	if got.Remote != secB.Public() || len(got.Packets) != 1 || got.Packets[0][0] != 0xbb {
		t.Fatalf("A received %+v, want packet 0xbb from %s", got, secB.Public())
	}
}

func unknownID(t *testing.T) base.NodeID {
	t.Helper()
	var b [base.NodeIDLen]byte
	b[0] = 0xfe
	id, err := base.NodeIDFromBytes(b[:])
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// backboneLen returns the number of cached backbone links on a server.
func backboneLen(srv *Server) int {
	srv.backboneMu.Lock()
	defer srv.backboneMu.Unlock()
	return len(srv.backbone)
}

// federatedServers sets up two servers that know each other and returns them
// plus their httptest servers.
func federatedServers(t *testing.T) (*Server, *Server, *httptest.Server, *httptest.Server) {
	t.Helper()
	srvA := NewServer(nil)
	srvB := NewServer(nil)
	tsA := httptest.NewServer(srvA.Handler())
	t.Cleanup(tsA.Close)
	tsB := httptest.NewServer(srvB.Handler())
	t.Cleanup(tsB.Close)
	selfA := "http://" + tsA.Listener.Addr().String()
	selfB := "http://" + tsB.Listener.Addr().String()
	srvA.Self = selfA
	srvB.Self = selfB
	srvA.SetPeers([]string{selfB})
	srvB.SetPeers([]string{selfA})
	srvA.Start()
	srvB.Start()
	return srvA, srvB, tsA, tsB
}

// TestBackboneEagerSingleConnection verifies the two relays establish exactly
// one backbone link at startup (before any client or data), elected
// deterministically.
func TestBackboneEagerSingleConnection(t *testing.T) {
	srvA, srvB, _, _ := federatedServers(t)

	waitFor(t, func() bool { return backboneLen(srvA) == 1 && backboneLen(srvB) == 1 })
	if got := backboneLen(srvA); got != 1 {
		t.Fatalf("relay A has %d backbone links, want 1", got)
	}
	if got := backboneLen(srvB); got != 1 {
		t.Fatalf("relay B has %d backbone links, want 1", got)
	}
}

// TestBackboneReconnects verifies a dropped backbone link is re-established by
// the maintenance manager.
func TestBackboneReconnects(t *testing.T) {
	srvA, srvB, tsA, tsB := federatedServers(t)
	waitFor(t, func() bool { return backboneLen(srvA) == 1 && backboneLen(srvB) == 1 })

	// Kill A's link to B: the write path fails and the manager re-dials.
	srvA.backboneMu.Lock()
	link := srvA.backbone["http://"+tsB.Listener.Addr().String()]
	srvA.backboneMu.Unlock()
	if link == nil {
		t.Fatal("no backbone link on A")
	}
	link.ws.CloseNow()

	// The manager reconnects: both sides end up with a live link again.
	waitFor(t, func() bool {
		return backboneLen(srvA) == 1 && backboneLen(srvB) == 1
	})

	// And data flows again over the fresh link.
	secA, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	secB, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	_, wsA := connectedClientFor(t, tsA, secA)
	_, wsB := connectedClientFor(t, tsB, secB)
	defer wsA.CloseNow()
	defer wsB.CloseNow()
	if err := wsA.Write(context.Background(), websocket.MessageBinary,
		mustEncode(t, proto.ClientToRelayBatch{Remote: secB.Public(), Relay: "http://" + tsB.Listener.Addr().String(), Packets: [][]byte{{0x42}}})); err != nil {
		t.Fatal(err)
	}
	got := readBatch(t, wsB)
	if len(got.Packets) != 1 || got.Packets[0][0] != 0x42 {
		t.Fatalf("B received %+v after reconnect, want packet 0x42", got)
	}
}

// TestBackboneAsymmetricConfig verifies a single backbone link is established
// even when only one relay has the other configured: the configured relay dials
// or, if it is the larger-address side, invites itself (Hello) so the other
// dials. This is the "added the second relay only on one side" scenario.
func TestBackboneAsymmetricConfig(t *testing.T) {
	srvA := NewServer(nil)
	srvB := NewServer(nil)
	// Fast hello + reconcile so the invite path is quick.
	srvA.helloInterval = 100 * time.Millisecond
	srvA.reconcileInterval = 50 * time.Millisecond
	srvB.helloInterval = 100 * time.Millisecond
	srvB.reconcileInterval = 50 * time.Millisecond
	tsA := httptest.NewServer(srvA.Handler())
	t.Cleanup(tsA.Close)
	tsB := httptest.NewServer(srvB.Handler())
	t.Cleanup(tsB.Close)
	selfA := "http://" + tsA.Listener.Addr().String()
	selfB := "http://" + tsB.Listener.Addr().String()
	srvA.Self = selfA
	srvB.Self = selfB
	srvA.Secret = "s"
	srvB.Secret = "s"
	// Only A knows about B.
	srvA.SetPeers([]string{selfB})
	srvA.Start()
	srvB.Start()

	waitForTimeout(t, func() bool { return backboneLen(srvA) == 1 && backboneLen(srvB) == 1 }, 5*time.Second)
	if got := backboneLen(srvA); got != 1 {
		t.Fatalf("relay A has %d backbone links, want 1", got)
	}
	if got := backboneLen(srvB); got != 1 {
		t.Fatalf("relay B has %d backbone links, want 1", got)
	}
}

// TestBackboneHelloInvite verifies the invite path specifically: the
// larger-address relay has the smaller one configured (so it must Hello to be
// dialed), and the smaller relay has no peers at all.
func TestBackboneHelloInvite(t *testing.T) {
	srvA := NewServer(nil)
	srvB := NewServer(nil)
	srvA.helloInterval = 100 * time.Millisecond
	srvA.reconcileInterval = 50 * time.Millisecond
	srvB.helloInterval = 100 * time.Millisecond
	srvB.reconcileInterval = 50 * time.Millisecond
	tsA := httptest.NewServer(srvA.Handler())
	t.Cleanup(tsA.Close)
	tsB := httptest.NewServer(srvB.Handler())
	t.Cleanup(tsB.Close)
	selfA := "http://" + tsA.Listener.Addr().String()
	selfB := "http://" + tsB.Listener.Addr().String()
	srvA.Self = selfA
	srvB.Self = selfB
	srvA.Secret = "s"
	srvB.Secret = "s"

	// Configure the LARGER relay with the smaller one; the smaller has nothing.
	if canonicalRelayAddr(selfA) < canonicalRelayAddr(selfB) {
		srvB.SetPeers([]string{selfA}) // B larger, has A; A must be invited
	} else {
		srvA.SetPeers([]string{selfB}) // A larger, has B; B must be invited
	}
	srvA.Start()
	srvB.Start()

	waitForTimeout(t, func() bool { return backboneLen(srvA) == 1 && backboneLen(srvB) == 1 }, 5*time.Second)
}

// TestBackboneDynamicPeers verifies a peer can be added at runtime (without
// restarting) and a single backbone link forms.
func TestBackboneDynamicPeers(t *testing.T) {
	srvA := NewServer(nil)
	srvB := NewServer(nil)
	srvA.reconcileInterval = 50 * time.Millisecond
	srvB.reconcileInterval = 50 * time.Millisecond
	tsA := httptest.NewServer(srvA.Handler())
	t.Cleanup(tsA.Close)
	tsB := httptest.NewServer(srvB.Handler())
	t.Cleanup(tsB.Close)
	selfA := "http://" + tsA.Listener.Addr().String()
	selfB := "http://" + tsB.Listener.Addr().String()
	srvA.Self = selfA
	srvB.Self = selfB
	srvA.Secret = "s"
	srvB.Secret = "s"
	srvA.Start()
	srvB.Start()

	// No peers yet: no backbone links.
	time.Sleep(100 * time.Millisecond)
	if backboneLen(srvA) != 0 || backboneLen(srvB) != 0 {
		t.Fatalf("expected no links before peers are set, got A=%d B=%d", backboneLen(srvA), backboneLen(srvB))
	}

	// Add the peer at runtime: a link forms without restarting.
	srvA.SetPeers([]string{selfB})
	waitForTimeout(t, func() bool { return backboneLen(srvA) == 1 && backboneLen(srvB) == 1 }, 5*time.Second)
}

// TestBackboneKeepalive verifies a silently-dead peer (no frames, no RST) is
// detected by the keepalive watchdog and the link is re-established.
func TestBackboneKeepalive(t *testing.T) {
	srvA := NewServer(nil)
	srvB := NewServer(nil)
	srvA.backbonePingInterval = 30 * time.Millisecond
	srvA.backbonePingTimeout = 90 * time.Millisecond
	srvB.backbonePingInterval = 30 * time.Millisecond
	srvB.backbonePingTimeout = 90 * time.Millisecond
	tsA := httptest.NewServer(srvA.Handler())
	t.Cleanup(tsA.Close)
	tsB := httptest.NewServer(srvB.Handler())
	t.Cleanup(tsB.Close)
	selfA := "http://" + tsA.Listener.Addr().String()
	selfB := "http://" + tsB.Listener.Addr().String()
	srvA.Self = selfA
	srvB.Self = selfB
	srvA.SetPeers([]string{selfB})
	srvB.SetPeers([]string{selfA})
	srvA.Start()
	srvB.Start()

	waitFor(t, func() bool { return backboneLen(srvA) == 1 && backboneLen(srvB) == 1 })

	// With only ping/pong traffic (no data), a healthy link must survive well
	// past the keepalive timeout. Capture the established link: if the watchdog
	// ever closes it, the managers tear it down and re-establish a new one,
	// which closes the captured link's channel. This catches a watchdog that
	// kills healthy links because it never observes the pings the peer answers.
	srvA.backboneMu.Lock()
	var survive *backboneLink
	for _, l := range srvA.backbone {
		survive = l
		break
	}
	srvA.backboneMu.Unlock()
	if survive == nil {
		t.Fatal("no backbone link on A")
	}
	time.Sleep(5 * srvA.backbonePingTimeout)
	select {
	case <-survive.closed:
		t.Fatal("healthy backbone link was closed under keepalive")
	default:
	}
	if backboneLen(srvA) != 1 || backboneLen(srvB) != 1 {
		t.Fatalf("healthy backbone link died under keepalive: A=%d B=%d", backboneLen(srvA), backboneLen(srvB))
	}

	// Sever the link on one end; the peer's read loop (or its keepalive
	// watchdog) notices and the manager re-establishes the link.
	srvA.backboneMu.Lock()
	var link *backboneLink
	for _, l := range srvA.backbone {
		link = l
		break
	}
	srvA.backboneMu.Unlock()
	if link == nil {
		t.Fatal("no backbone link on A")
	}
	link.ws.CloseNow()

	waitFor(t, func() bool { return backboneLen(srvA) == 1 && backboneLen(srvB) == 1 })
}

// TestBackboneLogging verifies relays log peer connect/disconnect.
func TestBackboneLogging(t *testing.T) {
	var logs syncLogBuffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	srvA := NewServer(logger)
	srvB := NewServer(nil)
	tsA := httptest.NewServer(srvA.Handler())
	t.Cleanup(tsA.Close)
	tsB := httptest.NewServer(srvB.Handler())
	t.Cleanup(tsB.Close)
	selfA := "http://" + tsA.Listener.Addr().String()
	selfB := "http://" + tsB.Listener.Addr().String()
	srvA.Self = selfA
	srvB.Self = selfB
	srvA.SetPeers([]string{selfB})
	srvB.SetPeers([]string{selfA})
	srvA.Start()
	srvB.Start()

	waitFor(t, func() bool { return backboneLen(srvA) == 1 && backboneLen(srvB) == 1 })

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(logs.String(), "relay connected") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected 'relay connected' in logs, got:\n%s", logs.String())
}

// syncLogBuffer is a mutex-guarded bytes.Buffer usable as an io.Writer.
type syncLogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestBackboneDialTimeoutAndDedup verifies that a backbone dial to an
// unreachable peer is bounded by the dial timeout (instead of blocking the
// forward path forever), and that a concurrent backboneFor never starts a
// second dial to the same peer.
func TestBackboneDialTimeoutAndDedup(t *testing.T) {
	srvA := NewServer(nil)
	srvA.backboneDialTimeout = 200 * time.Millisecond

	// A listener that accepts TCP but never completes the WebSocket upgrade,
	// so a dial to it can only fail via the dial timeout.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				time.Sleep(5 * time.Second)
			}(c)
		}
	}()

	peerURL := "http://" + l.Addr().String()

	start := time.Now()
	done := make(chan *backboneLink, 1)
	go func() { done <- srvA.backboneFor(peerURL) }()

	// A concurrent call while the dial is in flight must not start a second
	// dial: it returns nil immediately.
	time.Sleep(50 * time.Millisecond)
	secondStart := time.Now()
	if second := srvA.backboneFor(peerURL); second != nil {
		t.Fatal("concurrent backboneFor returned a link for an unestablished dial")
	}
	if elapsed := time.Since(secondStart); elapsed > 500*time.Millisecond {
		t.Fatalf("concurrent backboneFor blocked behind the in-flight dial: %v", elapsed)
	}

	first := <-done
	if first != nil {
		t.Fatal("expected a dial to an unresponsive peer to fail")
	}
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("dial returned before the dial timeout could fire: %v", elapsed)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("dial to a stalled peer took %v; the dial timeout was not applied", elapsed)
	}

	// The dialing guard must be cleared (backboneFor's defers ran before it
	// returned), so a later dial is allowed again.
	srvA.backboneMu.Lock()
	inFlight := srvA.dialing[peerURL]
	srvA.backboneMu.Unlock()
	if inFlight {
		t.Fatal("dialing guard not cleared after the dial failed")
	}
}

// TestBackboneRemovedPeerClosesLink verifies that removing a peer via SetPeers
// tears down the cached backbone link, so an unlisted federation partner no
// longer holds a live connection here.
func TestBackboneRemovedPeerClosesLink(t *testing.T) {
	srvA := NewServer(nil)
	srvB := NewServer(nil)
	tsA := httptest.NewServer(srvA.Handler())
	t.Cleanup(tsA.Close)
	tsB := httptest.NewServer(srvB.Handler())
	t.Cleanup(tsB.Close)
	selfA := "http://" + tsA.Listener.Addr().String()
	selfB := "http://" + tsB.Listener.Addr().String()
	srvA.Self = selfA
	srvB.Self = selfB
	srvA.SetPeers([]string{selfB})
	srvB.SetPeers([]string{selfA})
	srvA.Start()
	srvB.Start()

	waitFor(t, func() bool { return backboneLen(srvA) == 1 && backboneLen(srvB) == 1 })

	srvA.SetPeers(nil)
	srvB.SetPeers(nil)
	waitFor(t, func() bool { return backboneLen(srvA) == 0 && backboneLen(srvB) == 0 })

	// The links must stay gone: neither relay maintains the peer anymore.
	time.Sleep(200 * time.Millisecond)
	if backboneLen(srvA) != 0 || backboneLen(srvB) != 0 {
		t.Fatalf("backbone links reappeared after peer removal: A=%d B=%d", backboneLen(srvA), backboneLen(srvB))
	}
}
