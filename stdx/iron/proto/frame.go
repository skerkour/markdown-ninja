// Package proto defines the iron relay wire protocol.
//
// Every message is a single CBOR data item carrying a semantic tag (CBOR major
// type 6) whose number IS the message type, in the private/vendor range 4200+.
// A frame on the wire is therefore the tag head followed directly by the
// message content — no enclosing wrapper, no double encoding. Each Go message
// struct is registered with its tag number in a cbor.TagSet, so encoding
// emits the tag automatically and decoding validates it; decoding into
// cbor.Tag yields a typed *Msg for switch-style dispatch.
package proto

import (
	"net"
	"reflect"
	"time"

	"github.com/fxamacker/cbor/v2"
	"markdown.ninja/stdx/iron/base"
)

// Relay protocol versions negotiated via the WebSocket subprotocol.
const (
	RelayProtocol = "iron-relay"
	// RelayBackboneProtocol is the subprotocol a peer relay uses to establish
	// a relay-to-relay backbone link.
	RelayBackboneProtocol = "iron-relay-backbone"
)

// HandshakeDomainSep is the domain-separation string for the relay
// challenge signature.
const HandshakeDomainSep = "iron-relay handshake v1 challenge signature"

// Message tags. A tag's number is the message type; values are in the
// private/vendor CBOR tag range, above the IANA-registered tags.
const (
	MsgServerHello        = 4200 // relay->client   handshake step 1
	MsgClientHello        = 4201 // client->relay   handshake step 2
	MsgFinished           = 4202 // relay->client   handshake step 3
	MsgClientToRelayBatch = 4204 // client->relay
	MsgRelayToClientBatch = 4205 // relay->client
	MsgPing               = 4206
	MsgPong               = 4207
	MsgRestarting         = 4208
	MsgHolePunchRequest   = 4210 // client->relay
	MsgHolePunch          = 4211 // relay->client
	MsgRelayToRelayBatch  = 4212 // relay->relay  (backbone)
	MsgLocalAddrs         = 4213 // client->relay (refresh announced addresses)

	// HTTP directory API messages (POST /relay/api).
	MsgAPILookup     = 4301
	MsgAPILookupResp = 4302
	MsgHello         = 4303
)

// Size limits.
const (
	// MaxPacketSize is the maximum size of a single QUIC packet.
	MaxPacketSize = 64 * 1024
	// MaxEndpoints bounds the number of direct addresses a node may announce.
	MaxEndpoints = 32
)

// tagSet maps each message type to its Go struct. A cbor.TagSet only allows
// one tag number per Go type, so the two batch directions are distinct types
// (see ClientToRelayBatch / RelayToClientBatch).
var tagSet = func() cbor.TagSet {
	tagSet := cbor.NewTagSet()
	addToTagSet := func(opts cbor.TagOptions, t any, num uint64) {
		if err := tagSet.Add(opts, reflect.TypeOf(t), num); err != nil {
			panic("cbor: " + err.Error())
		}
	}
	tagOptions := cbor.TagOptions{EncTag: cbor.EncTagRequired, DecTag: cbor.DecTagRequired}
	addToTagSet(tagOptions, ServerHello{}, MsgServerHello)
	addToTagSet(tagOptions, ClientHello{}, MsgClientHello)
	addToTagSet(tagOptions, Finished{}, MsgFinished)
	addToTagSet(tagOptions, ClientToRelayBatch{}, MsgClientToRelayBatch)
	addToTagSet(tagOptions, RelayToClientBatch{}, MsgRelayToClientBatch)
	addToTagSet(tagOptions, Ping{}, MsgPing)
	addToTagSet(tagOptions, Pong{}, MsgPong)
	addToTagSet(tagOptions, Restarting{}, MsgRestarting)
	addToTagSet(tagOptions, HolePunchRequest{}, MsgHolePunchRequest)
	addToTagSet(tagOptions, HolePunch{}, MsgHolePunch)
	addToTagSet(tagOptions, RelayToRelayBatch{}, MsgRelayToRelayBatch)
	addToTagSet(tagOptions, LocalAddrs{}, MsgLocalAddrs)
	addToTagSet(tagOptions, APILookupRequest{}, MsgAPILookup)
	addToTagSet(tagOptions, APILookupResponse{}, MsgAPILookupResp)
	addToTagSet(tagOptions, Hello{}, MsgHello)
	return tagSet
}()

// Encoding/decoding modes are immutable and safe for concurrent use. Core
// Deterministic Encoding keeps map keys sorted and integers minimal, so the
// bytes are stable for a given message (useful for the signed HTTP API).
var (
	encMode cbor.EncMode
	decMode cbor.DecMode
)

func init() {
	em, err := cbor.CoreDetEncOptions().EncModeWithTags(tagSet)
	if err != nil {
		panic("proto: " + err.Error())
	}
	encMode = em
	dm, err := cbor.DecOptions{}.DecModeWithTags(tagSet)
	if err != nil {
		panic("proto: " + err.Error())
	}
	decMode = dm
}

// Message structs. Fields use integer map keys (keyasint) for compactness;
// byte arrays marshal as CBOR byte strings.

// The relay authentication handshake is exactly three messages:
//
//  1. ServerHello  (relay -> client): a challenge to sign.
//  2. ClientHello  (client -> relay): identity, signature, direct addresses.
//  3. Finished     (relay -> client): Result=true on success (with the
//     relay-observed address), or Result=false to deny.
//
// Announcing the client's direct addresses in ClientHello lets the relay tie
// them to the live connection and purge them the moment it disconnects.

// ServerHello opens the handshake with a challenge the client must sign.
type ServerHello struct {
	Challenge [16]byte `cbor:"0,keyasint"`
}

// ClientHello authenticates a client: its public identity, a signature over
// the blake3-derived handshake key, and the direct addresses it is reachable
// at (publishing them to the relay's directory).
type ClientHello struct {
	ID    base.NodeID `cbor:"0,keyasint"`
	Sig   [64]byte    `cbor:"1,keyasint"`
	Addrs []string    `cbor:"2,keyasint"`
}

// Finished is the relay's handshake answer. Result=true confirms
// authentication and, when available, carries the address the relay observes
// the client coming from. Result=false denies (the client is then closed).
type Finished struct {
	Result   bool   `cbor:"0,keyasint"`
	Observed net.IP `cbor:"1,keyasint,omitempty"`
}

// ClientToRelayBatch carries one or more QUIC packets (of possibly differing
// sizes) from a client to the relay for forwarding. Remote is the destination
// NodeID. When the destination is connected to a different relay, Relay holds
// that relay's URL and the relaying server forwards over the backbone.
type ClientToRelayBatch struct {
	Remote  base.NodeID `cbor:"0,keyasint"`
	Ecn     byte        `cbor:"1,keyasint"`
	Packets [][]byte    `cbor:"2,keyasint"`
	Relay   string      `cbor:"3,keyasint,omitempty"`
}

// RelayToClientBatch carries one or more QUIC packets from the relay to a
// client. Remote is the sender.
type RelayToClientBatch struct {
	Remote  base.NodeID `cbor:"0,keyasint"`
	Ecn     byte        `cbor:"1,keyasint"`
	Packets [][]byte    `cbor:"2,keyasint"`
}

// RelayToRelayBatch is forwarded between relays over the backbone link.
// Remote is the ultimate destination NodeID on the far relay; Sender is the
// originating NodeID, which the far relay tags onto the packets it delivers.
type RelayToRelayBatch struct {
	Remote  base.NodeID `cbor:"0,keyasint"`
	Sender  base.NodeID `cbor:"1,keyasint"`
	Ecn     byte        `cbor:"2,keyasint"`
	Packets [][]byte    `cbor:"3,keyasint"`
}

// LocalAddrs refreshes the announced direct addresses on an established relay
// connection (e.g. after STUN discovery or SetAnnouncedAddrs).
type LocalAddrs struct {
	Addrs []string `cbor:"0,keyasint"`
}

// Ping is a relay keepalive probe.
type Ping struct {
	Nonce [8]byte `cbor:"0,keyasint"`
}

// Pong answers a Ping, echoing its nonce.
type Pong struct {
	Nonce [8]byte `cbor:"0,keyasint"`
}

// Restarting advises the client that the relay is restarting: reconnect after
// ReconnectAfter (smearing the reconnect storm), trying for TryFor.
type Restarting struct {
	ReconnectAfter time.Duration `cbor:"0,keyasint"`
	TryFor         time.Duration `cbor:"1,keyasint"`
}

// HolePunchRequest asks the relay to coordinate a direct connection to Target.
type HolePunchRequest struct {
	Target base.NodeID `cbor:"0,keyasint"`
}

// HolePunch carries the other side's direct addresses ("ip:port" strings) so
// both ends can punch through their NATs simultaneously.
type HolePunch struct {
	Target base.NodeID `cbor:"0,keyasint"`
	Addrs  []string    `cbor:"1,keyasint"`
}

// HTTP directory API messages.

// APILookupRequest asks for another node's direct addresses.
type APILookupRequest struct {
	ID base.NodeID `cbor:"0,keyasint"`
}

// APILookupResponse is the answer to an APILookup: the peer's direct addresses, its
// observed public IP (empty when it is not connected to the relay) and the
// relays that reported the peer (the answering relay's own host first, then
// any of its peers that found the peer too).
type APILookupResponse struct {
	Addrs       []string `cbor:"0,keyasint"`
	Observed    string   `cbor:"1,keyasint,omitempty"`
	FoundRelays []string `cbor:"2,keyasint"`
}

// Hello invites a peer relay to establish the backbone connection. It is sent
// by the larger-address relay (which does not dial) to the smaller-address
// relay, so the smaller side can dial and a pair keeps exactly one connection.
type Hello struct {
	Self string `cbor:"0,keyasint"`
}

// Encode returns the CBOR encoding of a message: the tag head (the message
// type, taken from the registered TagSet) followed directly by the content.
func Encode(msg any) ([]byte, error) {
	return encMode.Marshal(msg)
}

// Unmarshal decodes a tagged frame into msg (a pointer to the message struct
// or a pointer to any). The frame's tag number is validated against msg's
// registered type (DecTagRequired), then the content is decoded exactly once.
// Decoding into an any yields the concrete message value for registered tags,
// which is how callers dispatch on the message kind.
func Unmarshal(data []byte, msg any) error {
	return decMode.Unmarshal(data, msg)
}
