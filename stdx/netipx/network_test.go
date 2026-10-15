package netipx

import (
	"net/netip"
	"testing"
)

func TestAllIpsForNetwork(t *testing.T) {
	tests := []struct {
		network string
		ips     []string
	}{
		{"10.0.0.0/32", []string{"10.0.0.0"}},
		{"10.0.0.0/30", []string{"10.0.0.0", "10.0.0.1", "10.0.0.2", "10.0.0.3"}},
	}

	for _, test := range tests {
		network := netip.MustParsePrefix(test.network)
		ips := make([]netip.Addr, 0, len(test.ips))
		for _, ipStr := range test.ips {
			ip := netip.MustParseAddr(ipStr)
			ips = append(ips, ip)
		}

		i := 0
		for ip := range AllIpsForNetwork(network) {
			if ips[i].Compare(ip) != 0 {
				t.Fatalf("Got: %s | expected: %s | network: [%s]", ip, ips[i], network)
			}
			i += 1
		}

		if len(ips) != i {
			t.Fatalf("Got: %d ips | expected: %d ips | network: [%s]", i, len(ips), network)
		}
	}
}
