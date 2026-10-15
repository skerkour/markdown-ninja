package netipx

import (
	"iter"
	"net/netip"
)

func AllIpsForNetwork(network netip.Prefix) iter.Seq[netip.Addr] {
	return func(yield func(netip.Addr) bool) {
		ipAddress := network.Addr()
		for network.Contains(ipAddress) {
			if !yield(ipAddress) {
				return
			}
			ipAddress = ipAddress.Next()
		}
	}
}
