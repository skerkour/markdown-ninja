package iron_test

import (
	"context"
	"testing"
	"time"

	"markdown.ninja/stdx/iron"
	"markdown.ninja/stdx/iron/base"
)

// TestRelayFreeDirect verifies that two endpoints created with no relay at all
// (no relay server running) connect directly via NodeAddr/ConnectAddr.
func TestRelayFreeDirect(t *testing.T) {
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

	// No relay is configured or running: endpoints are purely direct.
	epA, err := iron.NewEndpoint(ctx, secA, "")
	if err != nil {
		t.Fatal(err)
	}
	defer epA.Close()
	epB, err := iron.NewEndpoint(ctx, secB, "")
	if err != nil {
		t.Fatal(err)
	}
	defer epB.Close()
	go serveEcho(ctx, epB, make(chan error, 1))

	addr := epB.NodeAddr()
	if len(addr.Direct) == 0 {
		t.Fatal("relay-free listener endpoint has no direct addresses")
	}

	conn, err := epA.ConnectAddr(ctx, addr)
	if err != nil {
		t.Fatalf("connect addr: %v", err)
	}
	defer conn.CloseWithError(0, "")
	if conn.Path() != iron.PathDirect {
		t.Fatalf("expected direct path, got %q", conn.Path())
	}
	echoRoundTrip(t, ctx, conn, "relay-free direct hello")
}

// TestRelayFreeDiscoverer verifies a relay-free endpoint can connect via a
// custom Discoverer.
func TestRelayFreeDiscoverer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	secB, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	epB, err := iron.NewEndpoint(ctx, secB, "")
	if err != nil {
		t.Fatal(err)
	}
	defer epB.Close()
	go serveEcho(ctx, epB, make(chan error, 1))

	direct := epB.NodeAddr().Direct
	if len(direct) == 0 {
		t.Fatal("relay-free listener endpoint has no direct addresses")
	}
	d := newFakeDiscoverer()
	d.add(secB.Public(), direct...)

	secA, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	epA, err := iron.NewEndpoint(ctx, secA, "")
	if err != nil {
		t.Fatal(err)
	}
	defer epA.Close()

	conn, err := epA.Connect(ctx, secB.Public(), iron.WithDiscoverers(d))
	if err != nil {
		t.Fatalf("connect via discoverer: %v", err)
	}
	defer conn.CloseWithError(0, "")
	if conn.Path() != iron.PathDirect {
		t.Fatalf("expected direct path, got %q", conn.Path())
	}
	echoRoundTrip(t, ctx, conn, "relay-free discoverer hello")
}

// TestRelayFreeReconnect verifies that a relay-free direct connection
// re-establishes directly after a drop.
func TestRelayFreeReconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	secA, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	secB, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}

	epA, err := iron.NewEndpoint(ctx, secA, "")
	if err != nil {
		t.Fatal(err)
	}
	defer epA.Close()
	epB, err := iron.NewEndpoint(ctx, secB, "")
	if err != nil {
		t.Fatal(err)
	}
	defer epB.Close()

	// Keep accepting on B across reconnects.
	first := make(chan *iron.Connection, 1)
	go func() {
		for {
			conn, err := epB.Accept(ctx)
			if err != nil {
				return
			}
			select {
			case first <- conn:
			default:
			}
			go serveEchoConn(ctx, conn)
		}
	}()

	conn, err := epA.ConnectAddr(ctx, epB.NodeAddr())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.CloseWithError(0, "")
	if conn.Path() != iron.PathDirect {
		t.Fatalf("expected direct path, got %q", conn.Path())
	}
	echoRoundTrip(t, ctx, conn, "before drop")

	// Kill the direct connection from B's side; A's path watcher must
	// re-establish directly (there is no relay to fall back to).
	peerConn := <-first
	if err := peerConn.CloseWithError(0, "simulating drop"); err != nil {
		t.Fatal(err)
	}

	// Keep echoing until the connection recovers.
	deadline := time.Now().Add(20 * time.Second)
	recovered := false
	for time.Now().Before(deadline) {
		if err := echoRoundTripErr(ctx, conn, "after drop"); err == nil {
			recovered = true
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if !recovered {
		t.Fatal("relay-free connection did not re-establish after a drop")
	}
}

// TestRelayOnlyRequiresRelay verifies that WithRelayOnly without a configured
// relay is rejected.
func TestRelayOnlyRequiresRelay(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sec, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := iron.NewEndpoint(ctx, sec, "", iron.WithRelayOnly()); err == nil {
		t.Fatal("expected WithRelayOnly without a relay to fail")
	}
}

// TestRelayFreeAnnounce verifies that a relay-free endpoint still publishes to
// a registered announcer (announcement is independent of the relay).
func TestRelayFreeAnnounce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	sec, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	ann := newFakeAnnouncer()
	ep, err := iron.NewEndpoint(ctx, sec, "", iron.WithAnnouncers(ann))
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Close()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(ann.lastAnnounced()) > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("relay-free endpoint did not announce to its announcer")
}
