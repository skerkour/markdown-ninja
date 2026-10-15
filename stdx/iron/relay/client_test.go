package relay

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"markdown.ninja/stdx/iron/base"
	"markdown.ninja/stdx/iron/proto"
)

// serverHandshake writes a ServerHello, reads a ClientHello, and answers with
// a Finished (Result=true) carrying the given observed IP. It mirrors the real
// relay's 3-message auth handshake.
func serverHandshake(t *testing.T, ws *websocket.Conn, context context.Context, observed net.IP) {
	t.Helper()
	if frame, err := proto.Encode(proto.ServerHello{Challenge: [16]byte{1}}); err == nil {
		if err := ws.Write(context, websocket.MessageBinary, frame); err != nil {
			return
		}
	}
	if _, _, err := ws.Read(context); err != nil {
		return
	}
	if frame, err := proto.Encode(proto.Finished{Result: true, Observed: observed}); err == nil {
		ws.Write(context, websocket.MessageBinary, frame)
	}
}

// miniRelay runs a websocket server that authenticates blindly and echoes
// whatever follows, enough for RelayConn to connect.
func miniRelay(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/relay", func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols:       []string{proto.RelayProtocol},
			InsecureSkipVerify: true,
		})
		if err != nil {
			return
		}
		ctx := r.Context()
		serverHandshake(t, ws, ctx, net.IPv4(127, 0, 0, 1))
		// echo binary messages back as-is (answering pings with pongs, like
		// the real relay)
		for {
			typ, msg, err := ws.Read(ctx)
			if err != nil {
				return
			}
			if typ != websocket.MessageBinary {
				continue
			}
			var msgVal any
			if err := proto.Unmarshal(msg, &msgVal); err == nil {
				if p, ok := msgVal.(proto.Ping); ok {
					if frame, err := proto.Encode(proto.Pong{Nonce: p.Nonce}); err == nil {
						ws.Write(ctx, websocket.MessageBinary, frame)
					}
					continue
				}
			}
			if err := ws.Write(ctx, websocket.MessageBinary, msg); err != nil {
				return
			}
		}
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { srv.Close() })
	return "http://" + ln.Addr().String()
}

// silentRelay authenticates the client and then goes completely silent (never
// pongs), so the client's keepalive watchdog should detect the dead relay.
func silentRelay(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/relay", func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols:       []string{proto.RelayProtocol},
			InsecureSkipVerify: true,
		})
		if err != nil {
			return
		}
		ctx := r.Context()
		serverHandshake(t, ws, ctx, net.IPv4(127, 0, 0, 1))
		// Never read or write again.
		<-ctx.Done()
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { srv.Close() })
	return "http://" + ln.Addr().String()
}

// TestWatchdogDetectsDeadRelay verifies the client's keepalive watchdog
// force-closes a relay that stops responding, surfacing it via Closed().
func TestWatchdogDetectsDeadRelay(t *testing.T) {
	url := silentRelay(t)
	secret, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	rc, err := Dial(context.Background(), url, secret,
		WithKeepalive(20*time.Millisecond, 80*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()

	select {
	case <-rc.Closed():
		// detected: good
	case <-time.After(3 * time.Second):
		t.Fatal("watchdog did not close the dead relay connection")
	}
}

// TestDialRejectsNonHTTP verifies only http/https relay URLs are accepted.
func TestDialRejectsNonHTTP(t *testing.T) {
	secret, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"ws://127.0.0.1:3333", "wss://relay.example", "ftp://relay.example", "not a url"} {
		if _, err := Dial(context.Background(), bad, secret); err == nil {
			t.Fatalf("Dial(%q) succeeded, want scheme error", bad)
		}
	}
}

func TestReadFromDeadline(t *testing.T) {
	url := miniRelay(t)
	secret, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	rc, err := Dial(context.Background(), url, secret)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()

	buf := make([]byte, 2048)

	if err := rc.SetReadDeadline(time.Now()); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, _, err := rc.ReadFrom(buf)
		done <- err
	}()
	select {
	case err := <-done:
		if err != os.ErrDeadlineExceeded {
			t.Fatalf("expected deadline exceeded, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ReadFrom did not honor SetReadDeadline")
	}
}

// TestReadFromBatch verifies that a batch frame is coalesced into a single
// queue entry and served to ReadFrom one packet at a time, preserving
// ordering across batches and honoring deadlines once a batch is exhausted.
func TestReadFromBatch(t *testing.T) {
	url := miniRelay(t)
	secret, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	rc, err := Dial(context.Background(), url, secret)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()

	buf := make([]byte, 2048)
	read := func() (string, error) {
		n, addr, err := rc.ReadFrom(buf)
		if err != nil {
			return "", err
		}
		if addr != (base.PeerAddr{ID: secret.Public()}) {
			return "", fmt.Errorf("unexpected peer addr %v", addr)
		}
		return string(buf[:n]), nil
	}

	// One batch frame holding three packets of differing sizes.
	rc.q <- &batchEntry{
		remote:  secret.Public(),
		packets: [][]byte{[]byte("AAAA"), []byte("BBBBB"), []byte("CC")},
	}
	for i, want := range []string{"AAAA", "BBBBB", "CC"} {
		got, err := read()
		if err != nil {
			t.Fatalf("packet %d: %v", i, err)
		}
		if got != want {
			t.Fatalf("packet %d: got %q, want %q", i, got, want)
		}
	}

	// Two back-to-back batches must be served in order.
	rc.q <- &batchEntry{remote: secret.Public(), packets: [][]byte{[]byte("DDDD"), []byte("EE")}}
	rc.q <- &batchEntry{remote: secret.Public(), packets: [][]byte{[]byte("FFFFF")}}
	for i, want := range []string{"DDDD", "EE", "FFFFF"} {
		got, err := read()
		if err != nil {
			t.Fatalf("cross-batch packet %d: %v", i, err)
		}
		if got != want {
			t.Fatalf("cross-batch packet %d: got %q, want %q", i, got, want)
		}
	}

	// The batch is exhausted: ReadFrom must now wait (deadline fires).
	if err := rc.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := rc.ReadFrom(buf); err != os.ErrDeadlineExceeded {
		t.Fatalf("expected deadline exceeded on empty queue, got %v", err)
	}
}

func TestConcurrentWriteTo(t *testing.T) {
	url := miniRelay(t)
	secret, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	rc, err := Dial(context.Background(), url, secret)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := rc.WriteTo([]byte("ping"), base.PeerAddr{ID: secret.Public()})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

// restartingRelay authenticates the client, then sends a Restarting advisory
// and closes the connection.
func restartingRelay(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/relay", func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols:       []string{proto.RelayProtocol},
			InsecureSkipVerify: true,
		})
		if err != nil {
			return
		}
		ctx := r.Context()
		serverHandshake(t, ws, ctx, net.IPv4(127, 0, 0, 1))
		// Advise a restart, then hang up.
		if frame, err := proto.Encode(proto.Restarting{ReconnectAfter: 20 * time.Millisecond, TryFor: time.Second}); err == nil {
			ws.Write(ctx, websocket.MessageBinary, frame)
		}
		ws.CloseNow()
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { srv.Close() })
	return "http://" + ln.Addr().String()
}

// TestRestartingReconnects verifies that a relay restart advisory causes the
// client to tear down its connection (after the advised delay) so the endpoint
// reconnects.
func TestRestartingReconnects(t *testing.T) {
	url := restartingRelay(t)
	secret, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	rc, err := Dial(context.Background(), url, secret)
	if err != nil {
		t.Fatal(err)
	}
	// The advisory says reconnect in 20ms; the client must not close before
	// then, and must close shortly after.
	select {
	case <-rc.Closed():
		t.Fatal("connection closed before the restart delay elapsed")
	case <-time.After(15 * time.Millisecond):
	}
	select {
	case <-rc.Closed():
		// closed after the advisory delay: good
	case <-time.After(2 * time.Second):
		t.Fatal("connection did not close after the restart advisory")
	}
}

// TestPingRTT verifies the relay round-trip time is tracked from pongs.
func TestPingRTT(t *testing.T) {
	url := miniRelay(t) // echoes pings back, so the watchdog gets pongs
	secret, err := base.NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	rc, err := Dial(context.Background(), url, secret,
		WithKeepalive(20*time.Millisecond, time.Second))
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if rtt := rc.PingRTT(); rtt > 0 && rtt < time.Second {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("did not observe a relay round-trip time")
}
