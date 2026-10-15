package netipx

import "net/netip"

// https://ipinfo.io/bogon
// https://ipgeolocation.io/resources/bogon.html
var bogonPrefixes = []netip.Prefix{
	// IPv4
	netip.MustParsePrefix("0.0.0.0/8"),          // "This" network
	netip.MustParsePrefix("10.0.0.0/8"),         // Private-use networks
	netip.MustParsePrefix("100.64.0.0/10"),      // Carrier-grade NAT
	netip.MustParsePrefix("127.0.0.0/8"),        // Loopback
	netip.MustParsePrefix("127.0.53.53/32"),     // Name collision occurrence
	netip.MustParsePrefix("169.254.0.0/16"),     // Link local
	netip.MustParsePrefix("172.16.0.0/12"),      // Private-use networks
	netip.MustParsePrefix("192.0.0.0/24"),       // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),       // TEST-NET-1
	netip.MustParsePrefix("192.168.0.0/16"),     // Private-use networks
	netip.MustParsePrefix("198.18.0.0/15"),      // Network interconnect device benchmark testing
	netip.MustParsePrefix("198.51.100.0/24"),    // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),     // TEST-NET-3
	netip.MustParsePrefix("224.0.0.0/4"),        // Multicast
	netip.MustParsePrefix("240.0.0.0/4"),        // Reserved for future use
	netip.MustParsePrefix("255.255.255.255/32"), // Limited broadcast

	// IPv6
	netip.MustParsePrefix("::/128"),        // Node-scope unicast unspecified address
	netip.MustParsePrefix("::1/128"),       // Node-scope unicast loopback address
	netip.MustParsePrefix("::ffff:0:0/96"), // IPv4-mapped addresses
	netip.MustParsePrefix("::/96"),         // IPv4-compatible addresses
	netip.MustParsePrefix("100::/64"),      // Remotely triggered black hole addresses
	netip.MustParsePrefix("2001:10::/28"),  // Overlay routable cryptographic hash identifiers (ORCHID)
	netip.MustParsePrefix("2001:db8::/32"), // Documentation prefix
	netip.MustParsePrefix("fc00::/7"),      // Unique local addresses (ULA)
	netip.MustParsePrefix("fe80::/10"),     // Link-local unicast
	netip.MustParsePrefix("fec0::/10"),     // Site-local unicast (deprecated)
	netip.MustParsePrefix("ff00::/8"),      // Multicast (Note: ff0e:/16 is global scope and may appear on the global internet.)

	// IPv6 Additional Bogon Ranges
	// These ranges aren't officially IPv6 bogon ranges - they're IPv6 representations of different IPv4 bogon ranges.
	netip.MustParsePrefix("2002::/24"),             // 6to4 bogon (0.0.0.0/8)
	netip.MustParsePrefix("2002:a00::/24"),         // 6to4 bogon (10.0.0.0/8)
	netip.MustParsePrefix("2002:7f00::/24"),        // 6to4 bogon (127.0.0.0/8)
	netip.MustParsePrefix("2002:a9fe::/32"),        // 6to4 bogon (169.254.0.0/16)
	netip.MustParsePrefix("2002:ac10::/28"),        // 6to4 bogon (172.16.0.0/12)
	netip.MustParsePrefix("2002:c000::/40"),        // 6to4 bogon (192.0.0.0/24)
	netip.MustParsePrefix("2002:c000:200::/40"),    // 6to4 bogon (192.0.2.0/24)
	netip.MustParsePrefix("2002:c0a8::/32"),        // 6to4 bogon (192.168.0.0/16)
	netip.MustParsePrefix("2002:c612::/31"),        // 6to4 bogon (198.18.0.0/15)
	netip.MustParsePrefix("2002:c633:6400::/40"),   // 6to4 bogon (198.51.100.0/24)
	netip.MustParsePrefix("2002:cb00:7100::/40"),   // 6to4 bogon (203.0.113.0/24)
	netip.MustParsePrefix("2002:e000::/20"),        // 6to4 bogon (224.0.0.0/4)
	netip.MustParsePrefix("2002:f000::/20"),        // 6to4 bogon (240.0.0.0/4)
	netip.MustParsePrefix("2002:ffff:ffff::/48"),   // 6to4 bogon (255.255.255.255/32)
	netip.MustParsePrefix("2001::/40"),             // Teredo bogon (0.0.0.0/8)
	netip.MustParsePrefix("2001:0:a00::/40"),       // Teredo bogon (10.0.0.0/8)
	netip.MustParsePrefix("2001:0:7f00::/40"),      // Teredo bogon (127.0.0.0/8)
	netip.MustParsePrefix("2001:0:a9fe::/48"),      // Teredo bogon (169.254.0.0/16)
	netip.MustParsePrefix("2001:0:ac10::/44"),      // Teredo bogon (172.16.0.0/12)
	netip.MustParsePrefix("2001:0:c000::/56"),      // Teredo bogon (192.0.0.0/24)
	netip.MustParsePrefix("2001:0:c000:200::/56"),  // Teredo bogon (192.0.2.0/24)
	netip.MustParsePrefix("2001:0:c0a8::/48"),      // Teredo bogon (192.168.0.0/16)
	netip.MustParsePrefix("2001:0:c612::/47"),      // Teredo bogon (198.18.0.0/15)
	netip.MustParsePrefix("2001:0:c633:6400::/56"), // Teredo bogon (198.51.100.0/24)
	netip.MustParsePrefix("2001:0:cb00:7100::/56"), // Teredo bogon (203.0.113.0/24)
	netip.MustParsePrefix("2001:0:e000::/36"),      // Teredo bogon (224.0.0.0/4)
	netip.MustParsePrefix("2001:0:f000::/36"),      // Teredo bogon (240.0.0.0/4)
	netip.MustParsePrefix("2001:0:ffff:ffff::/64"), // Teredo bogon (255.255.255.255/32)
}

var bogonIPSet = buildBogonIpSet()

func BogonPrefixes() []netip.Prefix {
	return bogonPrefixes
}

func IsBogon(ipAddress netip.Addr) bool {
	return bogonIPSet.Contains(ipAddress)
}

func IsPrefixBogon(prefix netip.Prefix) bool {
	return bogonIPSet.ContainsPrefix(prefix)
}

func buildBogonIpSet() *IPSet {
	var builder IPSetBuilder

	for _, bogonPrefix := range bogonPrefixes {
		builder.AddPrefix(bogonPrefix)
	}

	ret, err := builder.IPSet()
	if err != nil {
		panic(err)
	}

	return ret
}
