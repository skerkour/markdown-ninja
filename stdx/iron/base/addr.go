package base

import "net"

// PeerAddr is the net.Addr that identifies a remote iron endpoint within the
// relay transport: it is simply the peer's NodeID.
//
// quic-go only ever inspects an address through Network(), String() and
// equality, all of which are stable for this type, so the relay transport can
// tunnel QUIC packets for arbitrarily many peers over one socket while quic-go
// still routes each packet to the right connection (by connection ID).
type PeerAddr struct {
	ID NodeID
}

func (a PeerAddr) Network() string { return "iron-relay" }
func (a PeerAddr) String() string  { return a.ID.String() }

var _ net.Addr = PeerAddr{}
