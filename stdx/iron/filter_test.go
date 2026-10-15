package iron

import (
	"net"
	"reflect"
	"testing"
)

// fixedLocalNet is a deterministic interface snapshot used by the filter
// tests: a docker-style bridge (172.17.0.0/16), a 192.168.1.0/24 LAN, a
// link-local prefix and loopback, matching a host whose relay-observed
// address is 100.64.0.7 (CGNAT).
func fixedLocalNet() localNet {
	return localNet{
		addrs: []net.IP{
			net.IPv4(127, 0, 0, 1),
			net.IPv4(192, 168, 1, 5),
			net.IPv4(172, 17, 0, 7),
			net.ParseIP("fe80::1"),
		},
		nets: []net.IPNet{
			{IP: net.IPv4(127, 0, 0, 0), Mask: net.CIDRMask(8, 32)},
			{IP: net.IPv4(192, 168, 1, 0), Mask: net.CIDRMask(24, 32)},
			{IP: net.IPv4(172, 17, 0, 0), Mask: net.CIDRMask(16, 32)},
			{IP: net.ParseIP("fe80::"), Mask: net.CIDRMask(64, 128)},
		},
	}
}

func TestReachableIP(t *testing.T) {
	ln := fixedLocalNet()
	observed := net.IPv4(100, 64, 0, 7) // our relay-observed address (CGNAT)

	cases := []struct {
		name string
		ip   net.IP
		want bool
	}{
		{"same host via real interface", net.IPv4(172, 17, 0, 7), true},
		{"same LAN private", net.IPv4(192, 168, 1, 42), true},
		{"public", net.IPv4(8, 8, 8, 8), true},
		{"CGNAT equal to observed", net.IPv4(100, 64, 0, 7), false}, // our own NAT address
		{"CGNAT other subscriber", net.IPv4(100, 64, 9, 9), true},   // distinct observed addr
		{"loopback v4", net.IPv4(127, 0, 0, 1), false},
		{"loopback v6", net.ParseIP("::1"), false},
		{"cross-LAN private", net.IPv4(192, 168, 2, 9), false},
		{"rfc1918 10/8", net.IPv4(10, 1, 2, 3), false},
		{"ULA", net.ParseIP("fd00::1"), false},
		{"ipv6 link-local same link", net.ParseIP("fe80::42"), false}, // not dialable without a zone
		{"ipv6 link-local other link", net.ParseIP("fe80:0:0:1::1"), false},
		{"ipv4 link-local", net.IPv4(169, 254, 7, 1), false},
		{"multicast", net.IPv4(224, 0, 0, 1), false},
		{"unspecified", net.IPv4(0, 0, 0, 0), false},
		{"nil", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := reachableIP(tc.ip, ln, observed); got != tc.want {
				t.Fatalf("reachableIP(%v) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

func TestFilterCandidatesFor(t *testing.T) {
	ln := fixedLocalNet()
	observed := net.IPv4(100, 64, 0, 7)

	all := []*net.UDPAddr{
		{IP: net.IPv4(127, 0, 0, 1), Port: 1000},    // loopback: dropped (fallback added below)
		{IP: net.IPv4(172, 17, 0, 7), Port: 1000},   // same host: kept + loopback fallback
		{IP: net.IPv4(192, 168, 1, 42), Port: 1000}, // same LAN: kept
		{IP: net.IPv4(192, 168, 2, 9), Port: 1000},  // other LAN: dropped
		{IP: net.IPv4(8, 8, 8, 8), Port: 1000},      // public: kept
		{IP: net.IPv4(100, 64, 0, 7), Port: 1000},   // == observed: dropped
		{IP: net.IPv4(172, 17, 0, 7), Port: 1000},   // duplicate of kept: deduped
	}
	// Because 172.17.0.7 is strictly equal to one of our interface addresses,
	// the peer is the same host and 127.0.0.1 is tried first.
	want := []*net.UDPAddr{
		{IP: net.IPv4(127, 0, 0, 1), Port: 1000},
		{IP: net.IPv4(172, 17, 0, 7), Port: 1000},
		{IP: net.IPv4(192, 168, 1, 42), Port: 1000},
		{IP: net.IPv4(8, 8, 8, 8), Port: 1000},
	}
	if got := filterCandidatesFor(all, ln, observed); !reflect.DeepEqual(got, want) {
		t.Fatalf("filterCandidatesFor = %v, want %v", got, want)
	}
}

// TestFilterCandidatesForNoLoopbackFallback verifies that peers which are NOT
// the same host never get a loopback candidate appended.
func TestFilterCandidatesForNoLoopbackFallback(t *testing.T) {
	ln := fixedLocalNet()
	observed := net.IPv4(100, 64, 0, 7)

	// A LAN peer: same subnet, but no strictly-equal interface address.
	all := []*net.UDPAddr{
		{IP: net.IPv4(192, 168, 1, 42), Port: 2000},
		{IP: net.IPv4(8, 8, 8, 8), Port: 2000},
	}
	want := []*net.UDPAddr{
		{IP: net.IPv4(192, 168, 1, 42), Port: 2000},
		{IP: net.IPv4(8, 8, 8, 8), Port: 2000},
	}
	if got := filterCandidatesFor(all, ln, observed); !reflect.DeepEqual(got, want) {
		t.Fatalf("filterCandidatesFor = %v, want %v", got, want)
	}
}
