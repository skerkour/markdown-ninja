package iron_test

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"markdown.ninja/stdx/iron"
	"markdown.ninja/stdx/iron/base"
)

// natConn simulates a NAT in front of a UDP socket: a packet is only delivered
// inbound if the endpoint has previously sent to that source address (a
// filtering NAT with endpoint-dependent mapping). LocalAddr is the address
// peers dial, i.e. the "public" address.
type natConn struct {
	conn *net.UDPConn

	mu      sync.Mutex
	allowed map[string]bool
}

func newNatConn(t *testing.T) *natConn {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return &natConn{conn: conn, allowed: make(map[string]bool)}
}

func (n *natConn) ReadFrom(p []byte) (int, net.Addr, error) {
	for {
		nn, addr, err := n.conn.ReadFrom(p)
		if err != nil {
			return nn, addr, err
		}
		n.mu.Lock()
		ok := n.allowed[addrKey(addr)]
		n.mu.Unlock()
		if ok {
			return nn, addr, nil
		}
		// Unsolicited inbound: dropped, like a NAT would.
	}
}

func (n *natConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	n.mu.Lock()
	n.allowed[addrKey(addr)] = true
	n.mu.Unlock()
	return n.conn.WriteTo(p, addr)
}

func addrKey(a net.Addr) string { return a.String() }

func (n *natConn) Close() error                      { return n.conn.Close() }
func (n *natConn) LocalAddr() net.Addr               { return n.conn.LocalAddr() }
func (n *natConn) SetDeadline(t time.Time) error     { return n.conn.SetDeadline(t) }
func (n *natConn) SetReadDeadline(t time.Time) error { return n.conn.SetReadDeadline(t) }
func (n *natConn) SetWriteDeadline(t time.Time) error {
	return n.conn.SetWriteDeadline(t)
}

var _ net.PacketConn = (*natConn)(nil)

// waitPublicAddr blocks until the endpoint has discovered and announced its
// public address.
func waitPublicAddr(t *testing.T, ep *iron.Endpoint) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if pa := ep.PublicAddr(); pa != nil && pa.IP != nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("endpoint did not discover its public address")
}

// TestSTUNDiscovery verifies that an endpoint learns its public UDP address
// from the relay's STUN endpoint. With the relay on localhost the observed
// address is the endpoint's own loopback socket.
func TestSTUNDiscovery(t *testing.T) {
	relayAddr, _, stopRelay := startRelay(t)
	defer stopRelay()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	secret, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	ep, err := iron.NewEndpoint(ctx, secret, "", iron.WithRelayURLs("http://"+relayAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Close()

	waitPublicAddr(t, ep)
	if pa := ep.PublicAddr(); !pa.IP.IsLoopback() {
		t.Fatalf("expected a loopback observed address, got %v", pa)
	}
}

// TestPunchDirectNAT verifies that two endpoints behind NATs (simulated by
// natConn, which drops unsolicited inbound packets) establish a direct QUIC
// connection through relay-assisted hole punching, even though neither can be
// reached directly via its announced addresses.
func TestPunchDirectNAT(t *testing.T) {
	relayAddr, srv, stopRelay := startRelay(t)
	defer stopRelay()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	secA, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	secB, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}

	natA := newNatConn(t)
	natB := newNatConn(t)

	epA, err := iron.NewEndpoint(ctx, secA, "", iron.WithRelayURLs("http://"+relayAddr), iron.WithDirectConn(natA))
	if err != nil {
		t.Fatal(err)
	}
	defer epA.Close()
	epB, err := iron.NewEndpoint(ctx, secB, "", iron.WithRelayURLs("http://"+relayAddr), iron.WithDirectConn(natB))
	if err != nil {
		t.Fatal(err)
	}
	defer epB.Close()

	// Both endpoints must discover and announce their public addresses before
	// any punch can succeed. Behind the NAT only the public address is usable
	// by peers: pin the announced set to exactly that, so the endpoints' real
	// (NAT-hidden) interface addresses are not dialed.
	waitPublicAddr(t, epA)
	waitPublicAddr(t, epB)
	epA.SetAnnouncedAddrs([]*net.UDPAddr{epA.PublicAddr()})
	epB.SetAnnouncedAddrs([]*net.UDPAddr{epB.PublicAddr()})
	time.Sleep(200 * time.Millisecond) // let the announcements reach the relay

	go serveEcho(ctx, epB, make(chan error, 1))

	// Punching may need a retry or two while the NAT mappings are established.
	deadline := time.Now().Add(20 * time.Second)
	var conn *iron.Connection
	for time.Now().Before(deadline) {
		c, err := epA.Connect(ctx, secB.Public())
		if err == nil && c.Path() == iron.PathDirect {
			conn = c
			break
		}
		if c != nil {
			c.CloseWithError(0, "")
		}
		time.Sleep(250 * time.Millisecond)
	}
	if conn == nil {
		t.Fatal("did not establish a direct (punched) connection")
	}
	defer conn.CloseWithError(0, "")

	if id, err := epA.PeerID(conn); err != nil || id != secB.Public() {
		t.Fatalf("bad peer identity: %v %v", id, err)
	}
	echoRoundTrip(t, ctx, conn, "punched hello")
	if got := srv.HolePunchRequests.Load(); got == 0 {
		t.Fatal("punched connection never asked the relay to coordinate a hole punch")
	}
}

// TestConnectRelayOnly verifies that a single connection can be forced through
// the relay with ConnectRelayOnly, even though a direct connection is possible
// on the same host.
func TestConnectRelayOnly(t *testing.T) {
	relayAddr, _, stopRelay := startRelay(t)
	defer stopRelay()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	epA, epB, _, secB := newEndpointPair(t, ctx, "http://"+relayAddr)
	go serveEcho(ctx, epB, make(chan error, 1))

	conn, err := epA.Connect(ctx, secB.Public(), iron.ConnectRelayOnly())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.CloseWithError(0, "")
	if conn.Path() != iron.PathRelay {
		t.Fatalf("ConnectRelayOnly connection used %q path, want relay", conn.Path())
	}
	echoRoundTrip(t, ctx, conn, "relay-only connection")
}

// TestConnectAddrRelayOnly verifies ConnectRelayOnly works with ConnectAddr,
// forcing the relay even when the NodeAddr embeds a reachable direct address.
func TestConnectAddrRelayOnly(t *testing.T) {
	relayAddr, _, stopRelay := startRelay(t)
	defer stopRelay()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	epA, epB, _, _ := newEndpointPair(t, ctx, "http://"+relayAddr)
	go serveEcho(ctx, epB, make(chan error, 1))

	addr := epB.NodeAddr()
	if len(addr.Direct) == 0 {
		t.Fatal("listener endpoint announced no direct addresses")
	}

	conn, err := epA.ConnectAddr(ctx, addr, iron.ConnectRelayOnly())
	if err != nil {
		t.Fatalf("connect addr: %v", err)
	}
	defer conn.CloseWithError(0, "")
	if conn.Path() != iron.PathRelay {
		t.Fatalf("ConnectAddr relay-only connection used %q path, want relay", conn.Path())
	}
	echoRoundTrip(t, ctx, conn, "relay-only via addr")
}

// TestDirectLocalNoPunch verifies that a direct connection to a peer on the
// local network does NOT engage hole punching at all: the plain direct dial
// succeeds on its own and no hole punch is requested from the relay.
func TestDirectLocalNoPunch(t *testing.T) {
	relayAddr, srv, stopRelay := startRelay(t)
	defer stopRelay()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	epA, epB, _, secB := newEndpointPair(t, ctx, "http://"+relayAddr)
	go serveEcho(ctx, epB, make(chan error, 1))

	conn, err := epA.Connect(ctx, secB.Public())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.CloseWithError(0, "")
	if conn.Path() != iron.PathDirect {
		t.Fatalf("expected direct path, got %q", conn.Path())
	}
	echoRoundTrip(t, ctx, conn, "local direct hello")
	if got := srv.HolePunchRequests.Load(); got != 0 {
		t.Fatalf("local direct connection triggered %d hole punches, want 0", got)
	}
}

// TestRelayOnly verifies that an endpoint configured with WithRelayOnly never
// uses a direct connection: it only dials and accepts through the relay, even
// when a direct connection would be possible.
func TestRelayOnly(t *testing.T) {
	relayAddr, _, stopRelay := startRelay(t)
	defer stopRelay()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	secA, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	secB, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}

	epA, err := iron.NewEndpoint(ctx, secA, "", iron.WithRelayURLs("http://"+relayAddr), iron.WithRelayOnly())
	if err != nil {
		t.Fatal(err)
	}
	defer epA.Close()
	epB, err := iron.NewEndpoint(ctx, secB, "", iron.WithRelayURLs("http://"+relayAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer epB.Close()

	go serveEcho(ctx, epB, make(chan error, 1))

	// A (relay-only) dials B: even though B's direct addresses are reachable
	// on the same host, A must go through the relay.
	conn, err := epA.Connect(ctx, secB.Public())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.CloseWithError(0, "")
	if conn.Path() != iron.PathRelay {
		t.Fatalf("relay-only endpoint used %q path, want relay", conn.Path())
	}
	echoRoundTrip(t, ctx, conn, "relay-only hello")

	// B dials A: A announces no direct addresses, so B must go through the
	// relay too.
	conn2, err := epB.Connect(ctx, secA.Public())
	if err != nil {
		t.Fatalf("connect back: %v", err)
	}
	defer conn2.CloseWithError(0, "")
	if conn2.Path() != iron.PathRelay {
		t.Fatalf("dialing a relay-only endpoint used %q path, want relay", conn2.Path())
	}
}
