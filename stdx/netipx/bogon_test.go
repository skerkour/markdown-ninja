package netipx

import (
	"net/netip"
	"testing"
)

func TestIsBogon(t *testing.T) {
	tests := []struct {
		IPAddress netip.Addr
		IsBogon   bool
	}{
		{netip.MustParseAddr("127.0.0.1"), true},
		{netip.MustParseAddr("0.0.0.0"), true},
		{netip.MustParseAddr("255.255.255.255"), true},
		{netip.MustParseAddr("104.16.132.229"), false},
	}

	for _, test := range tests {
		isBogon := IsBogon(test.IPAddress)
		if isBogon != test.IsBogon {
			t.Errorf("Invalid result for IP address %s. Got: %v | expected: %v", test.IPAddress, isBogon, test.IsBogon)
		}
	}
}
