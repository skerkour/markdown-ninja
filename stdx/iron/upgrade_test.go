package iron_test

import (
	"context"
	"net"
	"testing"
	"time"

	"markdown.ninja/stdx/iron"
	"markdown.ninja/stdx/iron/base"
)

// TestRelayToDirectUpgrade verifies that a connection established over the
// relay transparently upgrades to a direct one once the peer's direct
// addresses become reachable, without dropping the application connection.
func TestRelayToDirectUpgrade(t *testing.T) {
	relayAddr, _, stopRelay := startRelay(t)
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

	// B announces only an unreachable address at first, so the connection is
	// relayed; later it publishes its real addresses and the path upgrades.
	epA, err := iron.NewEndpoint(ctx, secA, "", iron.WithRelayURLs("http://"+relayAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer epA.Close()
	epB, err := iron.NewEndpoint(ctx, secB, "", iron.WithRelayURLs("http://"+relayAddr), iron.WithSkipAnnounce())
	if err != nil {
		t.Fatal(err)
	}
	defer epB.Close()
	if err := epB.SetAnnouncedAddrs([]*net.UDPAddr{{IP: net.IPv4(127, 0, 0, 1), Port: 1}}); err != nil {
		t.Fatal(err)
	}

	go serveEcho(ctx, epB, make(chan error, 1))

	conn, err := epA.Connect(ctx, secB.Public())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.CloseWithError(0, "")
	if conn.Path() != iron.PathRelay {
		t.Fatalf("expected relay path initially, got %q", conn.Path())
	}
	echoRoundTrip(t, ctx, conn, "over relay")

	// Now the peer becomes directly reachable: publish its real address.
	real := epB.PublicAddr()
	if real == nil {
		t.Fatal("B did not discover a public address")
	}
	if err := epB.SetAnnouncedAddrs([]*net.UDPAddr{real}); err != nil {
		t.Fatal(err)
	}

	// The watcher should upgrade the connection to direct. Poll with a fresh
	// echo to make sure the connection stays usable across the upgrade.
	waitForPath(t, conn, iron.PathDirect)
	echoRoundTrip(t, ctx, conn, "upgraded to direct")
}

// TestMultiRelayFailover verifies that an endpoint fails over to a secondary
// relay when the primary goes away.
func TestMultiRelayFailover(t *testing.T) {
	relay1Addr, srv1, stopRelay1 := startRelay(t)
	defer stopRelay1()
	relay2Addr, srv2, stopRelay2 := startRelay(t)
	defer stopRelay2()

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

	// Both endpoints know both relays and only reach each other via relay.
	epA, err := iron.NewEndpoint(ctx, secA, "",
		iron.WithRelayURLs("http://"+relay1Addr, "http://"+relay2Addr))
	if err != nil {
		t.Fatal(err)
	}
	defer epA.Close()
	epB, err := iron.NewEndpoint(ctx, secB, "",
		iron.WithRelayURLs("http://"+relay1Addr, "http://"+relay2Addr), iron.WithSkipAnnounce())
	if err != nil {
		t.Fatal(err)
	}
	defer epB.Close()
	if err := epB.SetAnnouncedAddrs([]*net.UDPAddr{{IP: net.IPv4(127, 0, 0, 1), Port: 1}}); err != nil {
		t.Fatal(err)
	}
	go serveEcho(ctx, epB, make(chan error, 1))

	conn, err := epA.Connect(ctx, secB.Public())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.CloseWithError(0, "")
	echoRoundTrip(t, ctx, conn, "before failover")

	// Kill relay1. Both endpoints should fail over to relay2.
	stopRelay1()

	// The existing connection re-establishes over relay2: keep echoing until
	// it recovers.
	deadline := time.Now().Add(30 * time.Second)
	recovered := false
	for time.Now().Before(deadline) {
		if err := echoRoundTripErr(ctx, conn, "after failover"); err == nil {
			recovered = true
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if !recovered {
		t.Fatal("connection did not recover after relay failover")
	}

	// Both endpoints fail over asynchronously; poll until the secondary relay
	// is actually used and the primary has drained.
	settled := time.Now().Add(30 * time.Second)
	for time.Now().Before(settled) {
		if srv2.ClientCount() > 0 && srv1.ClientCount() == 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if srv2.ClientCount() == 0 {
		t.Fatal("secondary relay has no clients after failover")
	}
	if srv1.ClientCount() != 0 {
		t.Fatalf("primary relay still has %d clients", srv1.ClientCount())
	}
	conn2, err := epA.Connect(ctx, secB.Public())
	if err != nil {
		t.Fatalf("connect after failover: %v", err)
	}
	defer conn2.CloseWithError(0, "")
	echoRoundTrip(t, ctx, conn2, "fresh after failover")
}

// TestRestartingFrame verifies the relay can advise clients it is restarting,
// causing them to reconnect.
func TestRestartingFrame(t *testing.T) {
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

	epA, err := iron.NewEndpoint(ctx, secA, "", iron.WithRelayURLs("http://"+relayAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer epA.Close()
	epB, err := iron.NewEndpoint(ctx, secB, "", iron.WithRelayURLs("http://"+relayAddr), iron.WithSkipAnnounce())
	if err != nil {
		t.Fatal(err)
	}
	defer epB.Close()
	if err := epB.SetAnnouncedAddrs([]*net.UDPAddr{{IP: net.IPv4(127, 0, 0, 1), Port: 1}}); err != nil {
		t.Fatal(err)
	}
	go serveEcho(ctx, epB, make(chan error, 1))

	if got := srv.ClientCount(); got != 2 {
		t.Fatalf("expected 2 clients before restart, got %d", got)
	}

	// Advise a restart with a small reconnect delay; clients reconnect.
	srv.Restarting(100*time.Millisecond, 10*time.Second)

	deadline := time.Now().Add(30 * time.Second)
	reconnected := false
	for time.Now().Before(deadline) {
		if srv.ClientCount() == 2 {
			reconnected = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !reconnected {
		t.Fatal("clients did not reconnect after restart advisory")
	}

	// And connections still work after the restart.
	conn, err := epA.Connect(ctx, secB.Public())
	if err != nil {
		t.Fatalf("connect after restart: %v", err)
	}
	defer conn.CloseWithError(0, "")
	echoRoundTrip(t, ctx, conn, "after restart")
}
