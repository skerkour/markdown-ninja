// Package discovery defines the interfaces through which an iron endpoint
// finds and advertises the direct UDP addresses of peers, independently of
// the relay.
//
// A Discoverer lets a dialing endpoint look up a peer's direct addresses from
// some channel other than the relay endpoint store (e.g. future mDNS, a
// directory service, a DHT). An Announcer is the publish side: a place this
// endpoint advertises its own direct addresses so peers can find it.
//
// Announcer deliberately does not embed Discoverer: an endpoint may want to
// announce itself somewhere without discovering others there (and vice
// versa). The two roles are wired separately at endpoint construction
// (WithAnnouncers) and per connection (WithDiscoverers).
//
// This package currently defines only the contracts; concrete channels
// (relay, mDNS, ...) live elsewhere.
package discovery

import (
	"context"
	"net"

	"markdown.ninja/stdx/iron/base"
)

// Discoverer is a channel through which a peer's direct addresses are looked
// up. Lookup should return an empty slice (not an error) when the peer is not
// known; an empty result simply means "not found here".
type Discoverer interface {
	Lookup(ctx context.Context, peer base.NodeID) []*net.UDPAddr
	Close() error
}

// Announcer is a channel to which this endpoint publishes its direct
// addresses so peers can find it. It does not embed Discoverer: announcing
// and discovering are independent concerns.
type Announcer interface {
	Announce(addrs []*net.UDPAddr)
	Close() error
}
