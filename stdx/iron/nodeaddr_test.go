package iron_test

import (
	"context"
	"net"
	"testing"
	"time"

	"markdown.ninja/stdx/iron"
	"markdown.ninja/stdx/iron/base"
)

func TestNodeAddrRoundTrip(t *testing.T) {
	secret, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	na := base.NodeAddr{
		ID: secret.Public(),
		Direct: []*net.UDPAddr{
			{IP: net.IPv4(192, 168, 1, 5), Port: 7000},
			{IP: net.ParseIP("2001:db8::1"), Port: 7000},
		},
		Relays: []string{"http://127.0.0.1:3333", "https://relay.example.com"},
	}

	s := na.String()
	got, err := base.ParseNodeAddr(s)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.ID != na.ID {
		t.Fatalf("id mismatch")
	}
	if len(got.Direct) != len(na.Direct) {
		t.Fatalf("direct addr count: got %d, want %d", len(got.Direct), len(na.Direct))
	}
	for i := range na.Direct {
		if !got.Direct[i].IP.Equal(na.Direct[i].IP) || got.Direct[i].Port != na.Direct[i].Port {
			t.Fatalf("direct[%d] mismatch: %v vs %v", i, got.Direct[i], na.Direct[i])
		}
	}
	if len(got.Relays) != len(na.Relays) {
		t.Fatalf("relay count: got %d, want %d", len(got.Relays), len(na.Relays))
	}
	for i := range na.Relays {
		if got.Relays[i] != na.Relays[i] {
			t.Fatalf("relay[%d] mismatch: %q vs %q", i, got.Relays[i], na.Relays[i])
		}
	}

	// Parse rejects garbage.
	if _, err := base.ParseNodeAddr("not base64!!"); err == nil {
		t.Fatal("expected error for invalid encoding")
	}
}

// TestConnectAddr verifies that a node can be dialed from only its NodeAddr,
// without any relay lookup: the embedded direct addresses are enough on the
// local network.
func TestConnectAddr(t *testing.T) {
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

	addr := epB.NodeAddr()
	if len(addr.Direct) == 0 {
		t.Fatal("listener endpoint announced no direct addresses")
	}

	// Give the announcement time to settle, then dial using only the address.
	time.Sleep(200 * time.Millisecond)
	secA, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	epA, err := iron.NewEndpoint(ctx, secA, "", iron.WithRelayURLs("http://"+relayAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer epA.Close()

	conn, err := epA.ConnectAddr(ctx, addr)
	if err != nil {
		t.Fatalf("connect addr: %v", err)
	}
	defer conn.CloseWithError(0, "")
	if conn.Path() != iron.PathDirect {
		t.Fatalf("expected direct path, got %q", conn.Path())
	}
	if id, err := epA.PeerID(conn); err != nil || id != secB.Public() {
		t.Fatalf("bad peer identity: %v %v", id, err)
	}
	echoRoundTrip(t, ctx, conn, "hello via node addr")
}

// TestNodeAddrRoundTripEndpoint verifies Endpoint.NodeAddr includes the relays.
func TestNodeAddrRelays(t *testing.T) {
	relayAddr, _, stopRelay := startRelay(t)
	defer stopRelay()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sec, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	ep, err := iron.NewEndpoint(ctx, sec, "",
		iron.WithRelayURLs("http://127.0.0.1:1111", "http://"+relayAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Close()

	na := ep.NodeAddr()
	if na.ID != sec.Public() {
		t.Fatal("node addr id mismatch")
	}
	if len(na.Relays) != 2 {
		t.Fatalf("expected 2 relays, got %d", len(na.Relays))
	}
}
