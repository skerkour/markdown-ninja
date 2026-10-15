package iron_test

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"markdown.ninja/stdx/iron"
	"markdown.ninja/stdx/iron/base"
	"markdown.ninja/stdx/iron/discovery"
)

// fakeDiscoverer is a Discoverer that returns a fixed set of addresses for a
// peer (or nothing if it does not know the peer).
type fakeDiscoverer struct {
	mu      sync.Mutex
	known   map[base.NodeID][]*net.UDPAddr
	lookups int
	closed  bool
}

func newFakeDiscoverer() *fakeDiscoverer {
	return &fakeDiscoverer{known: make(map[base.NodeID][]*net.UDPAddr)}
}

func (d *fakeDiscoverer) add(peer base.NodeID, addrs ...*net.UDPAddr) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.known[peer] = append([]*net.UDPAddr(nil), addrs...)
}

func (d *fakeDiscoverer) Lookup(ctx context.Context, peer base.NodeID) []*net.UDPAddr {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lookups++
	return d.known[peer]
}

func (d *fakeDiscoverer) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	return nil
}

var _ discovery.Discoverer = (*fakeDiscoverer)(nil)

// fakeAnnouncer is an Announcer that records what it was told to announce.
type fakeAnnouncer struct {
	mu        sync.Mutex
	announced []*net.UDPAddr
	closed    bool
}

func newFakeAnnouncer() *fakeAnnouncer { return &fakeAnnouncer{} }

func (a *fakeAnnouncer) Announce(addrs []*net.UDPAddr) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.announced = append([]*net.UDPAddr(nil), addrs...)
}

func (a *fakeAnnouncer) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closed = true
	return nil
}

func (a *fakeAnnouncer) lastAnnounced() []*net.UDPAddr {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.announced
}

func (a *fakeAnnouncer) isClosed() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.closed
}

var _ discovery.Announcer = (*fakeAnnouncer)(nil)

// TestRelayDiscoverAuto verifies that, with relays configured, the relay
// announcer and discoverer are automatic: the dialer finds the peer's direct
// addresses (over HTTP) without any opt-in options.
func TestRelayDiscoverAuto(t *testing.T) {
	relayAddr, _, stopRelay := startRelay(t)
	defer stopRelay()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	epA, epB, _, secB := newEndpointPair(t, ctx, "http://"+relayAddr)
	go serveEcho(ctx, epB, make(chan error, 1))

	// No options needed: B announced to the relay automatically, and A's
	// HTTP lookup across its relays finds B's direct (same-host) addresses.
	conn, err := epA.Connect(ctx, secB.Public())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.CloseWithError(0, "")
	if conn.Path() != iron.PathDirect {
		t.Fatalf("expected direct path, got %q", conn.Path())
	}
	echoRoundTrip(t, ctx, conn, "direct via auto relay discovery")
}

// TestRelayDiscoverMultiRelay verifies that HTTP discovery broadcasts across
// federated relays: the acceptor connects to relay1 only, while the dialer is
// on relay2 — relay2 broadcasts the lookup to its configured peer relay1, the
// dialer finds the acceptor, and connects directly.
func TestRelayDiscoverMultiRelay(t *testing.T) {
	const secret = "shared-secret"
	relay1Addr, srv1, stopRelay1 := startRelay(t)
	defer stopRelay1()
	relay2Addr, srv2, stopRelay2 := startRelay(t)
	defer stopRelay2()
	srv1.Secret = secret
	srv2.Secret = secret
	srv1.Self = "http://" + relay1Addr
	srv2.Self = "http://" + relay2Addr
	srv1.SetPeers([]string{"http://" + relay2Addr})
	srv2.SetPeers([]string{"http://" + relay1Addr})
	srv1.Start()
	srv2.Start()

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

	// B announces to relay1 only.
	epB, err := iron.NewEndpoint(ctx, secB, "", iron.WithRelayURLs("http://"+relay1Addr))
	if err != nil {
		t.Fatal(err)
	}
	defer epB.Close()
	go serveEcho(ctx, epB, make(chan error, 1))

	// A connects to relay2 (preferred); relay2 broadcasts the lookup to relay1
	// and finds B there.
	epA, err := iron.NewEndpoint(ctx, secA, "",
		iron.WithRelayURLs("http://"+relay2Addr, "http://"+relay1Addr))
	if err != nil {
		t.Fatal(err)
	}
	defer epA.Close()

	conn, err := epA.Connect(ctx, secB.Public())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.CloseWithError(0, "")
	if conn.Path() != iron.PathDirect {
		t.Fatalf("expected direct path, got %q", conn.Path())
	}
	echoRoundTrip(t, ctx, conn, "direct via multi-relay http")
}

// TestRelayAnnounceToConnected verifies that an endpoint announces its direct
// addresses only to the relay it is connected to: a dialer sharing that relay
// discovers the listener through its directory and connects directly.
func TestRelayAnnounceToConnected(t *testing.T) {
	relayAddr, _, stopRelay := startRelay(t)
	defer stopRelay()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	secB, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	epB, err := iron.NewEndpoint(ctx, secB, "", iron.WithRelayURLs("http://"+relayAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer epB.Close()
	go serveEcho(ctx, epB, make(chan error, 1))

	secA, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	epA, err := iron.NewEndpoint(ctx, secA, "", iron.WithRelayURLs("http://"+relayAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer epA.Close()

	// A mirrors B's relay connection, so it discovers B's announced addresses
	// via the shared directory and connects directly.
	conn, err := epA.Connect(ctx, secB.Public())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.CloseWithError(0, "")
	if conn.Path() != iron.PathDirect {
		t.Fatalf("expected direct path, got %q", conn.Path())
	}
	echoRoundTrip(t, ctx, conn, "direct via shared relay")
}

// TestAnnouncerReceivesPublishes verifies a registered fake Announcer receives
// the endpoint's direct addresses on startup and on SetAnnouncedAddrs, and is
// closed by Endpoint.Close.
func TestAnnouncerReceivesPublishes(t *testing.T) {
	relayAddr, _, stopRelay := startRelay(t)
	defer stopRelay()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sec, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	ann := newFakeAnnouncer()
	ep, err := iron.NewEndpoint(ctx, sec, "", iron.WithRelayURLs("http://"+relayAddr), iron.WithAnnouncers(ann))
	if err != nil {
		t.Fatal(err)
	}

	// Startup publishes.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(ann.lastAnnounced()) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(ann.lastAnnounced()) == 0 {
		t.Fatal("announcer did not receive the startup announcement")
	}

	// SetAnnouncedAddrs republishes the override.
	override := []*net.UDPAddr{{IP: net.IPv4(127, 0, 0, 1), Port: 9999}}
	if err := ep.SetAnnouncedAddrs(override); err != nil {
		t.Fatal(err)
	}
	if got := ann.lastAnnounced(); len(got) != 1 || !got[0].IP.Equal(override[0].IP) || got[0].Port != 9999 {
		t.Fatalf("announcer did not receive the override: %v", got)
	}

	// Close closes the announcer.
	if err := ep.Close(); err != nil {
		t.Fatal(err)
	}
	if !ann.isClosed() {
		t.Fatal("announcer was not closed by Endpoint.Close")
	}
}

// TestDiscoveryRolesAreSeparate verifies that a type implementing only one of
// the interfaces can be used as that role (the interfaces do not embed each
// other).
func TestDiscoveryRolesAreSeparate(t *testing.T) {
	relayAddr, _, stopRelay := startRelay(t)
	defer stopRelay()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	sec, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}

	// A fakeAnnouncer (Announcer only) is accepted as an announcer...
	epA, err := iron.NewEndpoint(ctx, sec, "",
		iron.WithRelayURLs("http://"+relayAddr), iron.WithAnnouncers(newFakeAnnouncer()))
	if err != nil {
		t.Fatal(err)
	}
	_ = epA.Close()

	// ...and a fakeDiscoverer (Discoverer only) as a discoverer on Connect.
	// A compile-time check: both types satisfy their respective interfaces.
	var a discovery.Announcer = newFakeAnnouncer()
	var d discovery.Discoverer = newFakeDiscoverer()
	_ = a
	_ = d
}
