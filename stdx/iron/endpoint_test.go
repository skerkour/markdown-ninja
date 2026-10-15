package iron_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/quic-go/quic-go"
	"github.com/zeebo/blake3"

	"markdown.ninja/stdx/iron"
	"markdown.ninja/stdx/iron/base"
	"markdown.ninja/stdx/iron/proto"
	"markdown.ninja/stdx/iron/relayserver"
)

func startRelay(t *testing.T) (string, *relayserver.Server, func()) {
	t.Helper()
	srv := relayserver.NewServer(nil)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	httpSrv := &http.Server{Handler: srv.Handler()}
	go func() { _ = httpSrv.Serve(l) }()
	serveUDP(t, srv, l.Addr().String())
	stop := func() {
		// Force-close hijacked websocket connections so clients observe the
		// outage (http.Server.Shutdown cannot see them).
		_ = srv.Close()
		_ = httpSrv.Shutdown(context.Background())
		_ = l.Close()
	}
	return l.Addr().String(), srv, stop
}

// serveUDP binds a UDP socket on addr for the relay's STUN responder, letting
// endpoints discover their public address for hole punching.
func serveUDP(t *testing.T, srv *relayserver.Server, addr string) {
	t.Helper()
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { udpConn.Close() })
	srv.ServeUDP(udpConn)
}

// restartableRelay runs a relay on a fixed address and can be stopped and
// restarted on the same address, to test endpoint reconnection. The same
// relayserver instance is reused across restarts.
func restartableRelay(t *testing.T) (addr string, stop func(), restart func()) {
	t.Helper()
	srv := relayserver.NewServer(nil)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr = l.Addr().String()
	httpSrv := &http.Server{Handler: srv.Handler()}
	go func() { _ = httpSrv.Serve(l) }()
	serveUDP(t, srv, addr)

	stop = func() {
		// Force-close hijacked websocket connections and the listener so the
		// endpoints actually observe the outage.
		_ = srv.Close()
		_ = httpSrv.Close()
		_ = l.Close()
	}
	restart = func() {
		l2, err := net.Listen("tcp", addr)
		if err != nil {
			t.Fatalf("re-listen on %s: %v", addr, err)
		}
		httpSrv2 := &http.Server{Handler: srv.Handler()}
		go func() { _ = httpSrv2.Serve(l2) }()
		t.Cleanup(func() {
			_ = httpSrv2.Close()
			_ = l2.Close()
		})
	}
	return addr, stop, restart
}

func newEndpointPair(t *testing.T, ctx context.Context, relayURL string) (*iron.Endpoint, *iron.Endpoint, *base.NodeSecret, *base.NodeSecret) {
	t.Helper()
	secA, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	secB, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	epA, err := iron.NewEndpoint(ctx, secA, "", iron.WithRelayURLs(relayURL), iron.WithLogger(testLogger(t)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { epA.Close() })
	epB, err := iron.NewEndpoint(ctx, secB, "", iron.WithRelayURLs(relayURL), iron.WithLogger(testLogger(t)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { epB.Close() })
	return epA, epB, secA, secB
}

func testLogger(t *testing.T) *slog.Logger {
	return slog.New(slog.NewTextHandler(testWriter{t}, nil))
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) { w.t.Log(string(p)); return len(p), nil }

// serveEcho accepts connections and echoes every stream back. It reports
// unexpected accept errors only; a canceled context is a clean shutdown.
func serveEcho(ctx context.Context, ep *iron.Endpoint, errCh chan<- error) {
	for {
		conn, err := ep.Accept(ctx)
		if err != nil {
			if ctx.Err() == nil {
				errCh <- err
			}
			return
		}
		go serveEchoConn(ctx, conn)
	}
}

func serveEchoConn(ctx context.Context, conn *iron.Connection) {
	for {
		st, err := conn.AcceptStream(ctx)
		if err != nil {
			return
		}
		go func(st *quic.Stream) {
			defer st.Close()
			_, _ = io.Copy(st, st)
		}(st)
	}
}

// echoRoundTrip writes msg on a fresh stream and reads the echo back.
func echoRoundTrip(t *testing.T, ctx context.Context, conn *iron.Connection, msg string) {
	t.Helper()
	if err := echoRoundTripErr(ctx, conn, msg); err != nil {
		t.Fatalf("echo round trip: %v", err)
	}
}

// echoRoundTripErr is echoRoundTrip that reports errors instead of failing the
// test, for retry loops.
func echoRoundTripErr(ctx context.Context, conn *iron.Connection, msg string) error {
	st, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return err
	}
	if _, err := st.Write([]byte(msg)); err != nil {
		return err
	}
	st.Close()
	got, err := io.ReadAll(st)
	if err != nil {
		return err
	}
	if string(got) != msg {
		return &echoMismatchError{want: msg, got: string(got)}
	}
	return nil
}

type echoMismatchError struct {
	want string
	got  string
}

func (e *echoMismatchError) Error() string {
	return "echo mismatch: got " + e.got
}

func waitForPath(t *testing.T, conn *iron.Connection, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if conn.Path() == want {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("connection did not switch to %q (still %q)", want, conn.Path())
}

// TestEchoDirect verifies two endpoints with reachable direct addresses
// connect over UDP, bypassing the relay.
func TestEchoDirect(t *testing.T) {
	relayAddr, _, stopRelay := startRelay(t)
	defer stopRelay()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	epA, epB, secA, secB := newEndpointPair(t, ctx, "http://"+relayAddr)
	errCh := make(chan error, 1)

	// Server side: accept, verify the client identity and the path, echo.
	accepted := make(chan *iron.Connection, 1)
	go func() {
		conn, err := epB.Accept(ctx)
		if err != nil {
			errCh <- err
			return
		}
		if id, err := epB.PeerID(conn); err != nil {
			errCh <- err
			return
		} else if id != secA.Public() {
			errCh <- &unexpectedPeerError{id: id}
			return
		}
		accepted <- conn
		st, err := conn.AcceptStream(ctx)
		if err != nil {
			errCh <- err
			return
		}
		if _, err := io.Copy(st, st); err != nil {
			errCh <- err
			return
		}
		st.Close()
		errCh <- nil
	}()

	conn, err := epA.Connect(ctx, secB.Public())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.CloseWithError(0, "")

	if id, err := epA.PeerID(conn); err != nil || id != secB.Public() {
		t.Fatalf("bad peer identity: %v %v", id, err)
	}
	if conn.Path() != iron.PathDirect {
		t.Fatalf("expected direct path, got %q", conn.Path())
	}
	acceptedConn := <-accepted
	if acceptedConn.Path() != iron.PathDirect {
		t.Fatalf("accepted side expected direct path, got %q", acceptedConn.Path())
	}

	echoRoundTrip(t, ctx, conn, "direct hello")
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

// TestEchoRelayFallback verifies that a peer whose direct address is
// unreachable still connects through the relay.
func TestEchoRelayFallback(t *testing.T) {
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
	epA, err := iron.NewEndpoint(ctx, secA, "", iron.WithRelayURLs("http://"+relayAddr))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { epA.Close() })
	// B publishes only an unreachable loopback port, so every direct dial
	// fails. Skip its auto-announcement, otherwise a lookup could sample the
	// real addresses before the override reaches the relay and connect
	// directly.
	epB, err := iron.NewEndpoint(ctx, secB, "", iron.WithRelayURLs("http://"+relayAddr), iron.WithSkipAnnounce())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { epB.Close() })
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
		t.Fatalf("expected relay path, got %q", conn.Path())
	}
	echoRoundTrip(t, ctx, conn, "relay hello")
}

// TestDirectFallbackRedial verifies that a dropped direct connection is
// transparently re-established over the relay.
func TestDirectFallbackRedial(t *testing.T) {
	relayAddr, _, stopRelay := startRelay(t)
	defer stopRelay()

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	epA, epB, _, secB := newEndpointPair(t, ctx, "http://"+relayAddr)

	// Single accept loop: remember the first accepted connection so the test
	// can kill it, and echo streams on every connection.
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

	conn, err := epA.Connect(ctx, secB.Public())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.CloseWithError(0, "")
	if conn.Path() != iron.PathDirect {
		t.Fatalf("expected direct path, got %q", conn.Path())
	}

	// Kill the direct connection from the peer's side.
	peerConn := <-first
	if err := peerConn.CloseWithError(0, "simulating direct drop"); err != nil {
		t.Fatal(err)
	}

	// The endpoint must silently fall back to the relay...
	waitForPath(t, conn, iron.PathRelay)
	// ...and keep serving new streams on the replacement connection.
	echoRoundTrip(t, ctx, conn, "after fallback")
}

// TestRelayRejectsBadSignature checks the relay's auth handshake refuses a
// client that cannot prove ownership of its claimed identity.
func TestRelayRejectsBadSignature(t *testing.T) {
	relayAddr, _, stopRelay := startRelay(t)
	defer stopRelay()
	relayURL := "http://" + relayAddr

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ws, _, err := websocket.Dial(ctx, relayURL+"/relay", &websocket.DialOptions{
		Subprotocols: []string{proto.RelayProtocol},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()

	typ, msg, err := ws.Read(ctx)
	if err != nil {
		t.Fatalf("expected challenge: %v", err)
	}
	if typ != websocket.MessageBinary {
		t.Fatal("expected binary challenge")
	}
	var ch proto.ServerHello
	if err := proto.Unmarshal(msg, &ch); err != nil {
		t.Fatal("expected server hello")
	}

	// Sign a *different* message so the signature does not verify.
	secret, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	var key [32]byte
	blake3.DeriveKey(proto.HandshakeDomainSep, ch.Challenge[:], key[:])
	key[0] ^= 0xff
	sig := secret.Sign(key[:])
	auth, err := proto.Encode(proto.ClientHello{ID: secret.Public(), Sig: sig})
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.Write(ctx, websocket.MessageBinary, auth); err != nil {
		t.Fatal(err)
	}

	// The relay must either send an unsuccessful Finished or close the
	// connection.
	typ, resp, err := ws.Read(ctx)
	if err != nil {
		return // closed: rejection confirmed
	}
	if typ != websocket.MessageBinary {
		t.Fatalf("unexpected frame type %d", typ)
	}
	var fin proto.Finished
	if err := proto.Unmarshal(resp, &fin); err != nil {
		t.Fatal(err)
	}
	if fin.Result {
		t.Fatalf("expected denial (Finished.Result=false)")
	}
}

type unexpectedPeerError struct {
	id base.NodeID
}

func (e *unexpectedPeerError) Error() string {
	return "unexpected peer identity: " + e.id.String()
}

// TestRelayReconnect verifies that endpoints reconnect to the relay after an
// outage and that new connections over the relay work again.
func TestRelayReconnect(t *testing.T) {
	relayAddr, stopRelay, restartRelay := restartableRelay(t)
	defer stopRelay()
	relayURL := "http://" + relayAddr

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
	epA, err := iron.NewEndpoint(ctx, secA, "", iron.WithRelayURLs(relayURL))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { epA.Close() })
	// B is reachable only via the relay.
	epB, err := iron.NewEndpoint(ctx, secB, "", iron.WithRelayURLs(relayURL), iron.WithSkipAnnounce())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { epB.Close() })
	if err := epB.SetAnnouncedAddrs([]*net.UDPAddr{{IP: net.IPv4(127, 0, 0, 1), Port: 1}}); err != nil {
		t.Fatal(err)
	}
	go serveEcho(ctx, epB, make(chan error, 1))

	connectAndEcho := func(msg string) {
		t.Helper()
		conn, err := epA.Connect(ctx, secB.Public())
		if err != nil {
			t.Fatalf("connect: %v", err)
		}
		defer conn.CloseWithError(0, "")
		if conn.Path() != iron.PathRelay {
			t.Fatalf("expected relay path, got %q", conn.Path())
		}
		echoRoundTrip(t, ctx, conn, msg)
	}

	// Baseline over the relay.
	connectAndEcho("before outage")

	// Kill the relay. The endpoints detect the outage and start reconnecting
	// with backoff (all attempts fail while the relay is down).
	stopRelay()

	// Wait until the endpoint actually observes the outage (its Connect starts
	// failing). Timing varies, especially under the race detector.
	outageObserved := false
	for i := 0; i < 30; i++ {
		checkCtx, checkCancel := context.WithTimeout(ctx, 250*time.Millisecond)
		_, err := epA.Connect(checkCtx, secB.Public())
		checkCancel()
		if err != nil {
			outageObserved = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !outageObserved {
		t.Fatal("endpoint did not observe the relay outage")
	}

	// Bring the relay back on the same address.
	restartRelay()

	// The endpoints reconnect with backoff; keep trying until a fresh
	// connection over the relay works again.
	deadline := time.Now().Add(30 * time.Second)
	for {
		conn, err := epA.Connect(ctx, secB.Public())
		if err == nil {
			path := conn.Path()
			if path != iron.PathRelay {
				conn.CloseWithError(0, "")
				t.Fatalf("expected relay path after reconnect, got %q", path)
			}
			conn.CloseWithError(0, "")
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("connection did not recover after relay restart: %v", err)
		}
		time.Sleep(250 * time.Millisecond)
	}

	connectAndEcho("after outage")
}
