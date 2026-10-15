package iron_test

import (
	"context"
	"testing"
	"time"

	"markdown.ninja/stdx/iron"
	"markdown.ninja/stdx/iron/base"
)

// TestBackboneAcrossRelays verifies end-to-end that a peer on one relay is
// reachable from a peer on a different relay through the relay-to-relay
// backbone: the dialer asks its own relay, which broadcasts the lookup to its
// configured peer relay and forwards the packets over the backbone.
func TestBackboneAcrossRelays(t *testing.T) {
	const secret = "shared-backbone-secret"

	relay1Addr, srv1, stop1 := startRelay(t)
	defer stop1()
	relay2Addr, srv2, stop2 := startRelay(t)
	defer stop2()
	srv1.Secret = secret
	srv2.Secret = secret
	srv1.Self = "http://" + relay1Addr
	srv2.Self = "http://" + relay2Addr
	srv1.SetPeers([]string{"http://" + relay2Addr})
	srv2.SetPeers([]string{"http://" + relay1Addr})
	srv1.Start()
	srv2.Start()

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

	// A is WS-connected to relay1, B to relay2. Both are configured with both
	// relays, and the two relays are federated, so A's lookup reaches B.
	epA, err := iron.NewEndpoint(ctx, secA, "",
		iron.WithRelayURLs("http://"+relay1Addr, "http://"+relay2Addr))
	if err != nil {
		t.Fatal(err)
	}
	defer epA.Close()
	epB, err := iron.NewEndpoint(ctx, secB, "",
		iron.WithRelayURLs("http://"+relay2Addr, "http://"+relay1Addr))
	if err != nil {
		t.Fatal(err)
	}
	defer epB.Close()

	go serveEcho(ctx, epB, make(chan error, 1))

	// Force the relay path so the data (and B's replies) must traverse the
	// relay-to-relay backbone, not a direct upgrade.
	conn, err := epA.Connect(ctx, secB.Public(), iron.ConnectRelayOnly())
	if err != nil {
		t.Fatalf("connect via backbone: %v", err)
	}
	defer conn.CloseWithError(0, "")

	if conn.Path() != iron.PathRelay {
		t.Fatalf("backbone connection used %q path, want relay", conn.Path())
	}
	echoRoundTrip(t, ctx, conn, "backbone hello")
}
