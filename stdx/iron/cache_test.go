package iron

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"markdown.ninja/stdx/iron/base"
)

func TestLookupCandidatesCache(t *testing.T) {
	oldTTL := candidateTTL
	candidateTTL = time.Hour // never expires within the test
	defer func() { candidateTTL = oldTTL }()

	var peer base.NodeID
	peer[0] = 1

	e := &Endpoint{peerAddrs: sync.Map{}}
	want := []*net.UDPAddr{{IP: net.IPv4(192, 168, 1, 5), Port: 7000}}
	e.peerAddrs.Store(peer, &candidateEntry{addrs: want, at: time.Now()})

	// A fresh cache hit returns the cached addresses without touching the relay.
	got, _, _, err := e.lookupCandidates(context.Background(), peer, connectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[0].IP.Equal(want[0].IP) || got[0].Port != 7000 {
		t.Fatalf("unexpected candidates: %v", got)
	}

	// forgetCandidates evicts, forcing a re-lookup on the next call.
	e.forgetCandidates(peer)
	if _, ok := e.peerAddrs.Load(peer); ok {
		t.Fatal("candidates not forgotten")
	}
}
