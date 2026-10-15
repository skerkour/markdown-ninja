package proto

import (
	"bytes"
	"net"
	"testing"
	"time"

	"markdown.ninja/stdx/iron/base"
)

// TestClientToRelayBatchRoundTrip verifies that a batch with packets of
// differing sizes round-trips through the tagged CBOR encoding.
func TestClientToRelayBatchRoundTrip(t *testing.T) {
	var id base.NodeID
	id[0] = 0xab

	msg := ClientToRelayBatch{
		Remote:  id,
		Ecn:     3,
		Packets: [][]byte{bytes.Repeat([]byte{0x01}, 1200), []byte("short"), bytes.Repeat([]byte{0x02}, 64)},
	}
	frame, err := Encode(msg)
	if err != nil {
		t.Fatal(err)
	}

	var val any
	if err := Unmarshal(frame, &val); err != nil {
		t.Fatal(err)
	}
	got, ok := val.(ClientToRelayBatch)
	if !ok {
		t.Fatalf("content = %T, want ClientToRelayBatch", val)
	}
	if got.Remote != id {
		t.Fatalf("remote = %x, want %x", got.Remote, id)
	}
	if got.Ecn != 3 {
		t.Fatalf("ecn = %d, want 3", got.Ecn)
	}
	if len(got.Packets) != 3 {
		t.Fatalf("got %d packets, want 3", len(got.Packets))
	}
	for i, p := range msg.Packets {
		if !bytes.Equal(got.Packets[i], p) {
			t.Fatalf("packet %d mismatch", i)
		}
	}
}

// TestRelayToClientBatchRoundTrip verifies the relay->client direction is a
// distinct type/tag.
func TestRelayToClientBatchRoundTrip(t *testing.T) {
	var id base.NodeID
	id[1] = 0xcd
	msg := RelayToClientBatch{Remote: id, Packets: [][]byte{[]byte("a"), []byte("bcd")}}
	frame, err := Encode(msg)
	if err != nil {
		t.Fatal(err)
	}
	var got RelayToClientBatch
	if err := Unmarshal(frame, &got); err != nil {
		t.Fatal(err)
	}
	if got.Remote != id || len(got.Packets) != 2 {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

// TestUnmarshalAny verifies decoding into any yields the concrete message value
// for registered tags, which is how dispatchers switch on the message kind.
func TestUnmarshalAny(t *testing.T) {
	ping, err := Encode(Ping{Nonce: [8]byte{9}})
	if err != nil {
		t.Fatal(err)
	}
	var val any
	if err := Unmarshal(ping, &val); err != nil {
		t.Fatal(err)
	}
	p, ok := val.(Ping)
	if !ok {
		t.Fatalf("content = %T, want Ping", val)
	}
	if p.Nonce != [8]byte{9} {
		t.Fatalf("nonce = %x", p.Nonce)
	}
}

// TestUnmarshalValidates verifies Unmarshal rejects a frame whose tag does not
// match the target type.
func TestUnmarshalValidates(t *testing.T) {
	ping, err := Encode(Ping{})
	if err != nil {
		t.Fatal(err)
	}
	var pong Pong
	if err := Unmarshal(ping, &pong); err == nil {
		t.Fatal("expected error decoding a Ping frame into a Pong")
	}
}

// TestHolePunchRoundTrip verifies hole punch request and hole punch messages,
// whose addresses travel as "ip:port" strings.
func TestHolePunchRoundTrip(t *testing.T) {
	var target base.NodeID
	target[1] = 0xbe
	addrs := []string{"8.8.8.8:5000", "[2001:db8::1]:9000"}

	req, err := Encode(HolePunchRequest{Target: target})
	if err != nil {
		t.Fatal(err)
	}
	var hpr HolePunchRequest
	if err := Unmarshal(req, &hpr); err != nil || hpr.Target != target {
		t.Fatalf("hole punch request round trip failed: %v", err)
	}

	frame, err := Encode(HolePunch{Target: target, Addrs: addrs})
	if err != nil {
		t.Fatal(err)
	}
	var hp HolePunch
	if err := Unmarshal(frame, &hp); err != nil {
		t.Fatal(err)
	}
	if hp.Target != target {
		t.Fatalf("target mismatch")
	}
	if len(hp.Addrs) != 2 || hp.Addrs[0] != addrs[0] || hp.Addrs[1] != addrs[1] {
		t.Fatalf("addrs = %v, want %v", hp.Addrs, addrs)
	}
}

// TestRestartingRoundTrip verifies the restart advisory durations.
func TestRestartingRoundTrip(t *testing.T) {
	frame, err := Encode(Restarting{ReconnectAfter: 1500 * time.Millisecond, TryFor: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	var r Restarting
	if err := Unmarshal(frame, &r); err != nil {
		t.Fatal(err)
	}
	if r.ReconnectAfter != 1500*time.Millisecond {
		t.Fatalf("reconnect_after = %v, want 1.5s", r.ReconnectAfter)
	}
	if r.TryFor != 10*time.Second {
		t.Fatalf("try_for = %v, want 10s", r.TryFor)
	}
}

// TestAuthFrames verifies the handshake messages round-trip.
func TestAuthFrames(t *testing.T) {
	challenge := [16]byte{1, 2, 3}
	ch, err := Encode(ServerHello{Challenge: challenge})
	if err != nil {
		t.Fatal(err)
	}
	var sc ServerHello
	if err := Unmarshal(ch, &sc); err != nil || sc.Challenge != challenge {
		t.Fatalf("challenge round trip failed: %v", err)
	}

	var id base.NodeID
	id[7] = 0x42
	var sig [64]byte
	sig[0] = 0xff
	addrs := []string{"203.0.113.9:1234"}
	auth, err := Encode(ClientHello{ID: id, Sig: sig, Addrs: addrs})
	if err != nil {
		t.Fatal(err)
	}
	var ca ClientHello
	if err := Unmarshal(auth, &ca); err != nil {
		t.Fatal(err)
	}
	if ca.ID != id || ca.Sig != sig || len(ca.Addrs) != 1 || ca.Addrs[0] != addrs[0] {
		t.Fatalf("client hello round trip failed")
	}

	fin, err := Encode(Finished{Result: true, Observed: net.IPv4(203, 0, 113, 9)})
	if err != nil {
		t.Fatal(err)
	}
	var f Finished
	if err := Unmarshal(fin, &f); err != nil {
		t.Fatal(err)
	}
	if !f.Result || !f.Observed.Equal(net.IPv4(203, 0, 113, 9)) {
		t.Fatalf("finished round trip failed: %+v", f)
	}
}

// TestInvalidFrames verifies that untagged or malformed data is rejected.
func TestInvalidFrames(t *testing.T) {
	var v any
	if err := Unmarshal(nil, &v); err == nil {
		t.Fatal("expected error for empty frame")
	}
	if err := Unmarshal([]byte{0xa1}, &v); err == nil {
		t.Fatal("expected error for untagged frame")
	}
	// Truncated tag head.
	if err := Unmarshal([]byte{0xd9, 0x10}, &v); err == nil {
		t.Fatal("expected error for truncated tag head")
	}
}

// TestAPIMessages verifies the HTTP directory API messages round-trip.
func TestAPIMessages(t *testing.T) {
	var id base.NodeID
	id[0] = 1

	lookup, err := Encode(APILookupRequest{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	var l APILookupRequest
	if err := Unmarshal(lookup, &l); err != nil || l.ID != id {
		t.Fatalf("lookup round trip failed: %v", err)
	}

	resp, err := Encode(APILookupResponse{Addrs: []string{"1.2.3.4:5000"}, Observed: "203.0.113.7"})
	if err != nil {
		t.Fatal(err)
	}
	var r APILookupResponse
	if err := Unmarshal(resp, &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Addrs) != 1 || r.Addrs[0] != "1.2.3.4:5000" || r.Observed != "203.0.113.7" {
		t.Fatalf("lookup resp round trip failed: %+v", r)
	}
}
