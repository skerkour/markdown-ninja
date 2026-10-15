package base

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"net"
)

// NodeAddr is the complete, shareable address of a node: its NodeID plus the
// direct UDP addresses and relay URLs it can be reached at. It round-trips
// through String/ParseNodeAddr, so it can be handed out of band (like a phone
// number) and dialed without any further discovery.
type NodeAddr struct {
	ID     NodeID
	Direct []*net.UDPAddr
	Relays []string
}

// String encodes the address in a compact, URL-safe form.
func (a NodeAddr) String() string {
	b := []byte{1} // version
	b = append(b, a.ID[:]...)
	b = binary.AppendUvarint(b, uint64(len(a.Direct)))
	for _, d := range a.Direct {
		b = appendAddr(b, d)
	}
	b = binary.AppendUvarint(b, uint64(len(a.Relays)))
	for _, r := range a.Relays {
		b = binary.AppendUvarint(b, uint64(len(r)))
		b = append(b, r...)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// ParseNodeAddr decodes the form produced by NodeAddr.String.
func ParseNodeAddr(s string) (NodeAddr, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return NodeAddr{}, err
	}
	b := raw
	if len(b) == 0 || b[0] != 1 {
		return NodeAddr{}, errors.New("unsupported node addr version")
	}
	b = b[1:]
	if len(b) < NodeIDLen {
		return NodeAddr{}, errors.New("short node addr")
	}
	var a NodeAddr
	copy(a.ID[:], b[:NodeIDLen])
	b = b[NodeIDLen:]

	n, nRead := binary.Uvarint(b)
	if nRead <= 0 || n > 32 {
		return NodeAddr{}, errors.New("bad direct address count")
	}
	b = b[nRead:]
	for i := uint64(0); i < n; i++ {
		addr, used, err := parseAddr(b)
		if err != nil {
			return NodeAddr{}, err
		}
		b = b[used:]
		a.Direct = append(a.Direct, addr)
	}

	n, nRead = binary.Uvarint(b)
	if nRead <= 0 || n > 32 {
		return NodeAddr{}, errors.New("bad relay count")
	}
	b = b[nRead:]
	for i := uint64(0); i < n; i++ {
		l, lRead := binary.Uvarint(b)
		if lRead <= 0 || l > 2048 || uint64(len(b)) < uint64(lRead)+l {
			return NodeAddr{}, errors.New("bad relay url")
		}
		b = b[lRead:]
		a.Relays = append(a.Relays, string(b[:l]))
		b = b[l:]
	}
	return a, nil
}

// appendAddr appends a single address: [1B family][4 or 16B ip][2B port BE].
func appendAddr(dst []byte, a *net.UDPAddr) []byte {
	if ip4 := a.IP.To4(); ip4 != nil {
		dst = append(dst, 4)
		dst = append(dst, ip4...)
	} else {
		ip6 := a.IP.To16()
		if ip6 == nil {
			ip6 = make(net.IP, 16)
		}
		dst = append(dst, 16)
		dst = append(dst, ip6...)
	}
	return append(dst, byte(a.Port>>8), byte(a.Port))
}

// parseAddr decodes a single address, returning the address and bytes consumed.
func parseAddr(b []byte) (*net.UDPAddr, int, error) {
	if len(b) < 3 {
		return nil, 0, errors.New("short address")
	}
	fam := b[0]
	var ip net.IP
	var n int
	switch fam {
	case 4:
		if len(b) < 7 {
			return nil, 0, errors.New("short ipv4 address")
		}
		ip = net.IP(append([]byte(nil), b[1:5]...))
		n = 5
	case 16:
		if len(b) < 19 {
			return nil, 0, errors.New("short ipv6 address")
		}
		ip = net.IP(append([]byte(nil), b[1:17]...))
		n = 17
	default:
		return nil, 0, errors.New("unknown address family")
	}
	port := int(b[n])<<8 | int(b[n+1])
	return &net.UDPAddr{IP: ip, Port: port}, n + 2, nil
}
