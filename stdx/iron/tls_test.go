package iron_test

import (
	"context"
	"crypto/tls"
	"regexp"
	"strings"
	"testing"
	"time"

	"markdown.ninja/stdx/iron"
	"markdown.ninja/stdx/iron/base"
)

// TestWithCustomCurvePreferences verifies that the KeyExchange groups from
// WithTLSConfig are used on both sides of a connection: two endpoints sharing
// a non-default group still connect directly and echo.
func TestWithCustomCurvePreferences(t *testing.T) {
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

	curves := []tls.CurveID{tls.X25519}
	epA, err := iron.NewEndpoint(ctx, secA, "",
		iron.WithRelayURLs("http://"+relayAddr),
		iron.WithTLSConfig(iron.TLSConfig{CurvePreferences: curves}))
	if err != nil {
		t.Fatal(err)
	}
	defer epA.Close()
	epB, err := iron.NewEndpoint(ctx, secB, "",
		iron.WithRelayURLs("http://"+relayAddr),
		iron.WithTLSConfig(iron.TLSConfig{CurvePreferences: curves}))
	if err != nil {
		t.Fatal(err)
	}
	defer epB.Close()
	go serveEcho(ctx, epB, make(chan error, 1))

	conn, err := epA.Connect(ctx, secB.Public())
	if err != nil {
		t.Fatalf("connect with custom curve preferences: %v", err)
	}
	defer conn.CloseWithError(0, "")
	echoRoundTrip(t, ctx, conn, "custom curves")
}

// TestMismatchedCurveNegotiationFails verifies the curve preferences actually
// gate the handshake: two endpoints with disjoint KeyExchange group sets fail
// to connect (no shared group to negotiate), on both the direct and relay
// paths.
func TestMismatchedCurveNegotiationFails(t *testing.T) {
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

	epA, err := iron.NewEndpoint(ctx, secA, "",
		iron.WithRelayURLs("http://"+relayAddr),
		iron.WithTLSConfig(iron.TLSConfig{CurvePreferences: []tls.CurveID{tls.X25519}}))
	if err != nil {
		t.Fatal(err)
	}
	defer epA.Close()
	epB, err := iron.NewEndpoint(ctx, secB, "",
		iron.WithRelayURLs("http://"+relayAddr),
		iron.WithTLSConfig(iron.TLSConfig{CurvePreferences: []tls.CurveID{tls.CurveP256}}))
	if err != nil {
		t.Fatal(err)
	}
	defer epB.Close()

	if conn, err := epA.Connect(ctx, secB.Public()); err == nil {
		conn.CloseWithError(0, "")
		t.Fatal("expected connect to fail with disjoint curve preferences")
	}
}

// negotiatedState inspects a connection's TLS state: it returns the negotiated
// ALPN protocol and the DNS subject alternative names of the peer certificate.
func negotiatedState(conn *iron.Connection) (alpn string, peerSANs []string) {
	st := conn.State().TLS
	if len(st.PeerCertificates) == 0 {
		return st.NegotiatedProtocol, nil
	}
	return st.NegotiatedProtocol, st.PeerCertificates[0].DNSNames
}

// TestHTTP3Masquerade verifies the hardened defaults: connections present as
// HTTP/3 (ALPN "h3") with a random, plausible SNI that never leaks the dialed
// node id, and the server certificate is minted to match that SNI.
func TestHTTP3Masquerade(t *testing.T) {
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

	epA, err := iron.NewEndpoint(ctx, secA, "",
		iron.WithRelayURLs("http://"+relayAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer epA.Close()
	epB, err := iron.NewEndpoint(ctx, secB, "",
		iron.WithRelayURLs("http://"+relayAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer epB.Close()
	go serveEcho(ctx, epB, make(chan error, 1))

	conn, err := epA.Connect(ctx, secB.Public())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.CloseWithError(0, "")
	echoRoundTrip(t, ctx, conn, "masquerade")

	alpn, peerSANs := negotiatedState(conn)
	if alpn != "h3" {
		t.Fatalf("negotiated ALPN = %q, want %q", alpn, "h3")
	}
	if len(peerSANs) != 1 {
		t.Fatalf("peer cert SANs = %v, want exactly the dialed SNI", peerSANs)
	}
	sni := peerSANs[0]
	if sni == secB.Public().String() {
		t.Fatalf("SNI leaked the node id: %q", sni)
	}
	if strings.Contains(sni, ".iron.invalid") {
		t.Fatalf("SNI uses the iron TLD: %q", sni)
	}
	generated := regexp.MustCompile(`^[a-z0-9-]+-[0-9a-f]{6}\.(com|net|org|io|co)$`)
	if !generated.MatchString(sni) {
		t.Fatalf("SNI %q does not look like the generated pool", sni)
	}
}

// TestCustomTLSMasquerade verifies that WithTLSConfig overrides the defaults:
// a custom ALPN and SNI pool are used on the wire and the server certificate
// matches the configured SNI.
func TestCustomTLSMasquerade(t *testing.T) {
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

	tlsCfg := iron.TLSConfig{
		ALPN:         []string{"iron-test/echo/1"},
		SNIHostnames: []string{"cdn.example.test", "edge.example.test"},
	}
	epA, err := iron.NewEndpoint(ctx, secA, "",
		iron.WithRelayURLs("http://"+relayAddr),
		iron.WithTLSConfig(tlsCfg))
	if err != nil {
		t.Fatal(err)
	}
	defer epA.Close()
	epB, err := iron.NewEndpoint(ctx, secB, "",
		iron.WithRelayURLs("http://"+relayAddr),
		iron.WithTLSConfig(tlsCfg))
	if err != nil {
		t.Fatal(err)
	}
	defer epB.Close()
	go serveEcho(ctx, epB, make(chan error, 1))

	conn, err := epA.Connect(ctx, secB.Public())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.CloseWithError(0, "")
	echoRoundTrip(t, ctx, conn, "custom masquerade")

	alpn, peerSANs := negotiatedState(conn)
	if alpn != "iron-test/echo/1" {
		t.Fatalf("negotiated ALPN = %q, want %q", alpn, "iron-test/echo/1")
	}
	if len(peerSANs) != 1 {
		t.Fatalf("peer cert SANs = %v, want exactly the configured SNI", peerSANs)
	}
	if !strings.HasPrefix(peerSANs[0], "cdn.example.test") &&
		!strings.HasPrefix(peerSANs[0], "edge.example.test") {
		t.Fatalf("peer cert SNI %q not from the configured pool", peerSANs[0])
	}
}

// TestMismatchedALPNFails verifies that endpoints must advertise a compatible
// ALPN list: disjoint NextProtos sets fail to negotiate on the direct path.
func TestMismatchedALPNFails(t *testing.T) {
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

	epA, err := iron.NewEndpoint(ctx, secA, "",
		iron.WithRelayURLs("http://"+relayAddr),
		iron.WithTLSConfig(iron.TLSConfig{ALPN: []string{"iron-test/a"}}))
	if err != nil {
		t.Fatal(err)
	}
	defer epA.Close()
	epB, err := iron.NewEndpoint(ctx, secB, "",
		iron.WithRelayURLs("http://"+relayAddr),
		iron.WithTLSConfig(iron.TLSConfig{ALPN: []string{"iron-test/b"}}))
	if err != nil {
		t.Fatal(err)
	}
	defer epB.Close()

	if conn, err := epA.Connect(ctx, secB.Public()); err == nil {
		conn.CloseWithError(0, "")
		t.Fatal("expected connect to fail with disjoint ALPN lists")
	}
}
