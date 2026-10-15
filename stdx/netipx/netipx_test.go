// Copyright 2020 The Inet.Af AUTHORS. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//lint:file-ignore U1000 allow unused code in tests for experiments.

package netipx

import (
	"bytes"
	"encoding"
	"flag"
	"net"
	"net/netip"
	"reflect"
	"testing"
)

type (
	IPPrefix = netip.Prefix
	IP       = netip.Addr
	IPPort   = netip.AddrPort
)

// IPv4 returns the IP of the IPv4 address a.b.c.d.
func IPv4(a, b, c, d uint8) IP {
	return netip.AddrFrom4([4]byte{a, b, c, d})
}

var long = flag.Bool("long", false, "run long tests")

func TestFromStdIP(t *testing.T) {
	mustFromStdIPMustPanic := func(std net.IP) (IP, bool) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected MustFromStdIP(%#v) to panic", std)
			}
		}()
		return MustFromStdIP(std), true
	}

	tests := []struct {
		name string
		fn   func(net.IP) (IP, bool)
		std  net.IP
		want IP
	}{
		{
			name: "v4",
			fn:   FromStdIP,
			std:  []byte{1, 2, 3, 4},
			want: IPv4(1, 2, 3, 4),
		},
		{
			name: "v6",
			fn:   FromStdIP,
			std:  net.ParseIP("::1"),
			want: netip.AddrFrom16([...]byte{15: 1}),
		},
		{
			name: "4in6-unmap",
			fn:   FromStdIP,
			std:  net.ParseIP("1.2.3.4"),
			want: IPv4(1, 2, 3, 4),
		},
		{
			name: "v4-raw",
			fn:   FromStdIPRaw,
			std:  net.ParseIP("1.2.3.4").To4(),
			want: IPv4(1, 2, 3, 4),
		},
		{
			name: "4in6-raw",
			fn:   FromStdIPRaw,
			std:  net.ParseIP("1.2.3.4"),
			want: netip.AddrFrom16([...]byte{10: 0xff, 11: 0xff, 12: 1, 13: 2, 14: 3, 15: 4}),
		},
		{
			name: "bad-raw",
			fn:   FromStdIPRaw,
			std:  net.IP{0xff},
		},
		{
			name: "bad-must-panic",
			fn:   mustFromStdIPMustPanic,
			std:  net.IP{0x00, 0x01, 0x02},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.fn(tt.std)
			if got != tt.want {
				t.Errorf("got (%#v, %v); want %#v", got, ok, tt.want)
			}
		})
	}
}

func TestFromStdAddr(t *testing.T) {
	tests := []struct {
		name   string
		std    net.IP
		port   int
		zone   string
		want   netip.AddrPort
		wantOK bool
	}{
		{
			name: "invalid IP",
			std:  net.IP{0xff},
		},
		{
			name: "invalid port",
			std:  net.IP{1, 2, 3, 4},
			port: -1,
		},
		{
			name:   "v4",
			std:    net.IP{1, 2, 3, 4},
			port:   8080,
			want:   netip.AddrPortFrom(netip.AddrFrom4([...]byte{1, 2, 3, 4}), 8080),
			wantOK: true,
		},
		{
			name: "v4 with zone",
			std:  net.IP{1, 2, 3, 4},
			port: 8080,
			zone: "foobar",
		},
		{
			name:   "v6",
			std:    net.ParseIP("fc::"),
			port:   8080,
			want:   netip.AddrPortFrom(netip.MustParseAddr("fc::"), 8080),
			wantOK: true,
		},
		{
			name:   "v6 with zone",
			std:    net.ParseIP("fc::"),
			port:   8080,
			zone:   "foobar",
			want:   netip.AddrPortFrom(netip.MustParseAddr("fc::").WithZone("foobar"), 8080),
			wantOK: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := FromStdAddr(tt.std, tt.port, tt.zone)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("FromStdAddr(%#v, %d, %q): got (%#v, %v); want (%#v, %t)", tt.std, tt.port, tt.zone, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestFromStdIPNet(t *testing.T) {
	tests := []struct {
		name string
		std  *net.IPNet
		want IPPrefix
	}{
		{
			name: "invalid IP",
			std: &net.IPNet{
				IP: net.IP{0xff},
			},
		},
		{
			name: "invalid mask",
			std: &net.IPNet{
				IP:   net.IPv6loopback,
				Mask: nil,
			},
		},
		{
			name: "non-contiguous mask",
			std: &net.IPNet{
				IP:   net.IPv4(192, 0, 2, 0).To4(),
				Mask: net.IPv4Mask(255, 0, 255, 0),
			},
		},
		{
			name: "IPv4",
			std: &net.IPNet{
				IP:   net.IPv4(192, 0, 2, 0).To4(),
				Mask: net.CIDRMask(24, 32),
			},
			want: mustIPPrefix("192.0.2.0/24"),
		},
		{
			name: "IPv6",
			std: &net.IPNet{
				IP:   net.ParseIP("2001:db8::"),
				Mask: net.CIDRMask(64, 128),
			},
			want: mustIPPrefix("2001:db8::/64"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := FromStdIPNet(tt.std)
			if !ok && got != (IPPrefix{}) {
				t.Fatalf("!ok but non-zero result")
			}

			if got != tt.want {
				t.Errorf("FromStdIPNet(%q) = %+v; want %+v", tt.std, got, tt.want)
			}
		})
	}
}

func TestParsePrefixOrAddr(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		want    netip.Addr
		wantErr bool
	}{
		{
			name:    "empty IP",
			s:       "",
			wantErr: true,
		},
		{
			name:    "invalid IP",
			s:       "192.168.",
			wantErr: true,
		},
		{
			name: "IPv4 address",
			s:    "127.0.0.1",
			want: netip.MustParseAddr("127.0.0.1"),
		},
		{
			name: "IPv6 address",
			s:    "2001:db8::",
			want: netip.MustParseAddr("2001:db8::"),
		},
		{
			name: "IPv4 prefix",
			s:    "192.0.2.1/32",
			want: netip.MustParseAddr("192.0.2.1"),
		},
		{
			name: "IPv6 prefix",
			s:    "2001:db8::68/96",
			want: netip.MustParseAddr("2001:db8::68"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePrefixOrAddr(tt.s)
			if err != nil {
				if tt.wantErr {
					return
				}
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("ParsePrefixOrAddr(%q) = %v; want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestAddrIPNet(t *testing.T) {
	tests := []struct {
		name string
		addr netip.Addr
		want *net.IPNet
	}{
		{
			name: "invalid IP",
			addr: netip.Addr{},
			want: &net.IPNet{},
		},
		{
			name: "IPv4",
			addr: mustIP("127.0.0.1"),
			want: &net.IPNet{
				IP:   net.IPv4(127, 0, 0, 1).To4(),
				Mask: net.IPv4Mask(255, 255, 255, 255),
			},
		},
		{
			name: "IPv6",
			addr: mustIP("2001:db8::"),
			want: &net.IPNet{
				IP:   net.ParseIP("2001:db8::"),
				Mask: net.CIDRMask(128, 128),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AddrIPNet(tt.addr)
			if got == nil {
				t.Fatalf("nil result")
			} else if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AddrIPNet(%q) = %+v; want %+v", tt.addr, got, tt.want)
			}
		})
	}
}

func TestPrefixLastIP(t *testing.T) {
	tests := []struct {
		name   string
		prefix netip.Prefix
		want   netip.Addr
	}{
		{
			name:   "invalid prefix",
			prefix: netip.Prefix{},
			want:   netip.Addr{},
		},
		{
			name:   "IPv4 /0",
			prefix: netip.MustParsePrefix("0.0.0.0/0"),
			want:   netip.MustParseAddr("255.255.255.255"),
		},
		{
			name:   "IPv4 /10",
			prefix: netip.MustParsePrefix("100.64.0.0/10"),
			want:   netip.MustParseAddr("100.127.255.255"),
		},
		{
			name:   "IPv4 /24",
			prefix: netip.MustParsePrefix("127.0.0.0/24"),
			want:   netip.MustParseAddr("127.0.0.255"),
		},
		{
			name:   "IPv4 /30",
			prefix: netip.MustParsePrefix("127.0.0.0/30"),
			want:   netip.MustParseAddr("127.0.0.3"),
		},
		{
			name:   "IPv4 /32",
			prefix: netip.MustParsePrefix("127.0.0.0/32"),
			want:   netip.MustParseAddr("127.0.0.0"),
		},
		{
			name:   "IPv6 /96",
			prefix: netip.MustParsePrefix("2001:db8::/96"),
			want:   netip.MustParseAddr("2001:db8::ffff:ffff"),
		},
		{
			name:   "IPv6 /125",
			prefix: netip.MustParsePrefix("2001:db8::/125"),
			want:   netip.MustParseAddr("2001:db8::7"),
		},
		{
			name:   "IPv6 /128",
			prefix: netip.MustParsePrefix("2001:db8::1/128"),
			want:   netip.MustParseAddr("2001:db8::1"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PrefixLastIP(tt.prefix)
			if got != tt.want {
				t.Errorf("PrefixLastIP(%q) = %v; want %v", tt.prefix, got, tt.want)
			}
		})
	}
}

type appendMarshaler interface {
	encoding.TextMarshaler
	AppendTo([]byte) []byte
}

// testAppendToMarshal tests that x's AppendTo and MarshalText methods yield the same results.
// x's MarshalText method must not return an error.
func testAppendToMarshal(t *testing.T, x appendMarshaler) {
	t.Helper()
	m, err := x.MarshalText()
	if err != nil {
		t.Fatalf("(%v).MarshalText: %v", x, err)
	}
	a := make([]byte, 0, len(m))
	a = x.AppendTo(a)
	if !bytes.Equal(m, a) {
		t.Errorf("(%v).MarshalText = %q, (%v).AppendTo = %q", x, m, x, a)
	}
}

var (
	mustIP       = netip.MustParseAddr
	mustIPPrefix = netip.MustParsePrefix
)

func mustIPs(strs ...string) []IP {
	var res []IP
	for _, s := range strs {
		res = append(res, mustIP(s))
	}
	return res
}

func BenchmarkBinaryMarshalRoundTrip(b *testing.B) {
	b.ReportAllocs()
	tests := []struct {
		name string
		ip   string
	}{
		{"ipv4", "1.2.3.4"},
		{"ipv6", "2001:db8::1"},
		{"ipv6+zone", "2001:db8::1%eth0"},
	}
	for _, tc := range tests {
		b.Run(tc.name, func(b *testing.B) {
			ip := mustIP(tc.ip)
			for b.Loop() {
				bt, err := ip.MarshalBinary()
				if err != nil {
					b.Fatal(err)
				}
				var ip2 IP
				if err := ip2.UnmarshalBinary(bt); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkStdIPv4(b *testing.B) {
	b.ReportAllocs()
	ips := []net.IP{}
	for b.Loop() {
		ip := net.IPv4(8, 8, 8, 8)
		ips = ips[:0]
		for i := 0; i < 100; i++ {
			ips = append(ips, ip)
		}
	}
}

func BenchmarkIPv4(b *testing.B) {
	b.ReportAllocs()
	ips := []IP{}
	for b.Loop() {
		ip := IPv4(8, 8, 8, 8)
		ips = ips[:0]
		for i := 0; i < 100; i++ {
			ips = append(ips, ip)
		}
	}
}

// ip4i was one of the possible representations of IP that came up in
// discussions, inlining IPv4 addresses, but having an "overflow"
// interface for IPv6 or IPv6 + zone. This is here for benchmarking.
type ip4i struct {
	ip4    [4]byte
	flags1 byte
	flags2 byte
	flags3 byte
	flags4 byte
	ipv6   interface{}
}

func newip4i_v4(a, b, c, d byte) ip4i {
	return ip4i{ip4: [4]byte{a, b, c, d}}
}

// BenchmarkIPv4_inline benchmarks the candidate representation, ip4i.
func BenchmarkIPv4_inline(b *testing.B) {
	b.ReportAllocs()
	ips := []ip4i{}
	for b.Loop() {
		ip := newip4i_v4(8, 8, 8, 8)
		ips = ips[:0]
		for i := 0; i < 100; i++ {
			ips = append(ips, ip)
		}
	}
}

func BenchmarkStdIPv6(b *testing.B) {
	b.ReportAllocs()
	ips := []net.IP{}
	for b.Loop() {
		ip := net.ParseIP("2001:db8::1")
		ips = ips[:0]
		for i := 0; i < 100; i++ {
			ips = append(ips, ip)
		}
	}
}

func BenchmarkIPv6(b *testing.B) {
	b.ReportAllocs()
	ips := []IP{}
	for b.Loop() {
		ip := mustIP("2001:db8::1")
		ips = ips[:0]
		for i := 0; i < 100; i++ {
			ips = append(ips, ip)
		}
	}
}

var parseBenchInputs = []struct {
	name string
	ip   string
}{
	{"v4", "192.168.1.1"},
	{"v6", "fd7a:115c:a1e0:ab12:4843:cd96:626b:430b"},
	{"v6_ellipsis", "fd7a:115c::626b:430b"},
	{"v6_v4", "::ffff:192.168.140.255"},
	{"v6_zone", "1:2::ffff:192.168.140.255%eth1"},
}

func pxv(cidrStrs ...string) (out []IPPrefix) {
	for _, s := range cidrStrs {
		out = append(out, mustIPPrefix(s))
	}
	return
}

func TestRangePrefixes(t *testing.T) {
	tests := []struct {
		from string
		to   string
		want []IPPrefix
	}{
		{"0.0.0.0", "255.255.255.255", pxv("0.0.0.0/0")},
		{"::", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", pxv("::/0")},
		{"10.0.0.0", "10.255.255.255", pxv("10.0.0.0/8")},
		{"10.0.0.0", "10.127.255.255", pxv("10.0.0.0/9")},
		{"0.0.0.4", "0.0.0.11", pxv(
			// 4 0100
			// 5 0101
			// 6 0110
			// 7 0111
			// 8 1000
			// 9 1001
			//10 1010
			//11 1011
			"0.0.0.4/30",
			"0.0.0.8/30",
		)},
		{"10.0.0.0", "11.10.255.255", pxv(
			"10.0.0.0/8",
			"11.0.0.0/13",
			"11.8.0.0/15",
			"11.10.0.0/16",
		)},
		{"1.2.3.5", "5.6.7.8", pxv(
			"1.2.3.5/32",
			"1.2.3.6/31",
			"1.2.3.8/29",
			"1.2.3.16/28",
			"1.2.3.32/27",
			"1.2.3.64/26",
			"1.2.3.128/25",
			"1.2.4.0/22",
			"1.2.8.0/21",
			"1.2.16.0/20",
			"1.2.32.0/19",
			"1.2.64.0/18",
			"1.2.128.0/17",
			"1.3.0.0/16",
			"1.4.0.0/14",
			"1.8.0.0/13",
			"1.16.0.0/12",
			"1.32.0.0/11",
			"1.64.0.0/10",
			"1.128.0.0/9",
			"2.0.0.0/7",
			"4.0.0.0/8",
			"5.0.0.0/14",
			"5.4.0.0/15",
			"5.6.0.0/22",
			"5.6.4.0/23",
			"5.6.6.0/24",
			"5.6.7.0/29",
			"5.6.7.8/32",
		)},
	}
	for _, tt := range tests {
		r := IPRangeFrom(mustIP(tt.from), mustIP(tt.to))
		got := r.Prefixes()
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("failed %s->%s. got:", tt.from, tt.to)
			for _, v := range got {
				t.Errorf("  %v", v)
			}
			t.Error("want:\n")
			for _, v := range tt.want {
				t.Errorf("  %v", v)
			}
		}
	}
}

func BenchmarkIPRangePrefixes(b *testing.B) {
	b.ReportAllocs()
	buf := make([]IPPrefix, 0, 50)
	r := IPRange{mustIP("1.2.3.5"), mustIP("5.6.7.8")}
	for b.Loop() {
		_ = r.AppendPrefixes(buf[:0])
	}
}

func TestParseIPRange(t *testing.T) {
	tests := []struct {
		in   string
		want interface{}
	}{
		{"", "no hyphen in range \"\""},
		{"foo-", `invalid From IP "foo" in range "foo-"`},
		{"1.2.3.4-foo", `invalid To IP "foo" in range "1.2.3.4-foo"`},
		{"1.2.3.4-5.6.7.8", IPRange{mustIP("1.2.3.4"), mustIP("5.6.7.8")}},
		{"1.2.3.4-0.1.2.3", "range 1.2.3.4 to 0.1.2.3 not valid"},
		{"::1-::5", IPRange{mustIP("::1"), mustIP("::5")}},
	}
	for _, tt := range tests {
		r, err := ParseIPRange(tt.in)
		var got interface{}
		if err != nil {
			got = err.Error()
		} else {
			got = r
		}
		if got != tt.want {
			t.Errorf("ParseIPRange(%q) = %v; want %v", tt.in, got, tt.want)
		}
		if err == nil {
			back := r.String()
			if back != tt.in {
				t.Errorf("input %q stringifies back as %q", tt.in, back)
			}
		}

		var r2 IPRange
		err = r2.UnmarshalText([]byte(tt.in))
		if err != nil {
			got = err.Error()
		} else {
			got = r2
		}
		if got != tt.want && tt.in != "" {
			t.Errorf("UnmarshalText(%q) = %v; want %v", tt.in, got, tt.want)
		}

		testAppendToMarshal(t, r)
	}
}

func TestIPRangeUnmarshalTextNonZero(t *testing.T) {
	r := MustParseIPRange("1.2.3.4-5.6.7.8")
	if err := r.UnmarshalText([]byte("1.2.3.4-5.6.7.8")); err == nil {
		t.Fatal("unmarshaled into non-empty IPPrefix")
	}
}

func TestIPRangeContains(t *testing.T) {
	type rtest struct {
		ip   IP
		want bool
	}
	tests := []struct {
		r      IPRange
		rtests []rtest
	}{
		{
			IPRangeFrom(mustIP("10.0.0.2"), mustIP("10.0.0.4")),
			[]rtest{
				{mustIP("10.0.0.1"), false},
				{mustIP("10.0.0.2"), true},
				{mustIP("10.0.0.3"), true},
				{mustIP("10.0.0.4"), true},
				{mustIP("10.0.0.5"), false},
				{IP{}, false},
				{mustIP("::"), false},
			},
		},
		{
			IPRangeFrom(mustIP("::1"), mustIP("::ffff")),
			[]rtest{
				{mustIP("::0"), false},
				{mustIP("::1"), true},
				{mustIP("::1%z"), false},
				{mustIP("::ffff"), true},
				{mustIP("1::"), false},
				{mustIP("0.0.0.1"), false},
				{IP{}, false},
			},
		},
		{
			IPRangeFrom(mustIP("10.0.0.2"), mustIP("::")), // invalid
			[]rtest{
				{mustIP("10.0.0.2"), false},
			},
		},
		{
			IPRange{},
			[]rtest{
				{IP{}, false},
			},
		},
	}
	for _, tt := range tests {
		for _, rt := range tt.rtests {
			got := tt.r.Contains(rt.ip)
			if got != rt.want {
				t.Errorf("Range(%v).Contains(%v) = %v; want %v", tt.r, rt.ip, got, rt.want)
			}
		}
	}
}

func TestIPRangeOverlaps(t *testing.T) {
	tests := []struct {
		r, o IPRange
		want bool
	}{
		{
			IPRange{},
			IPRange{},
			false,
		},
		{
			IPRange{mustIP("10.0.0.1"), mustIP("10.0.0.3")},
			IPRange{mustIP("10.0.0.3"), mustIP("10.0.0.4")},
			true, // overlaps on edge
		},
		{
			IPRange{mustIP("10.0.0.1"), mustIP("10.0.0.3")},
			IPRange{mustIP("10.0.0.2"), mustIP("10.0.0.4")},
			true, // overlaps in middle
		},
		{
			IPRange{mustIP("10.0.0.1"), mustIP("10.0.0.3")},
			IPRange{mustIP("10.0.0.4"), mustIP("10.0.0.4")},
			false, // doesn't overlap
		},
		{
			IPRange{mustIP("10.0.0.1"), mustIP("10.0.0.3")},
			IPRange{mustIP("10.0.0.0"), mustIP("10.0.0.5")},
			true, // one fully inside the other
		},
		{
			IPRange{mustIP("10.0.0.1"), mustIP("10.0.0.3")},
			IPRange{mustIP("::1"), mustIP("::2")},
			false,
		},
		{
			IPRange{mustIP("::"), mustIP("ff::")},
			IPRange{mustIP("cc::1"), mustIP("cc::2")},
			true,
		},
	}
	for _, tt := range tests {
		got := tt.r.Overlaps(tt.o)
		if got != tt.want {
			t.Errorf("Overlaps(%v, %v) = %v; want %v", tt.r, tt.o, got, tt.want)
		}
		got = tt.o.Overlaps(tt.r)
		if got != tt.want {
			t.Errorf("Overlaps(%v, %v) (reversed) = %v; want %v", tt.o, tt.r, got, tt.want)
		}
	}
}

func TestIPRangeValid(t *testing.T) {
	tests := []struct {
		r    IPRange
		want bool
	}{
		{IPRange{mustIP("10.0.0.0"), mustIP("10.0.0.255")}, true},
		{IPRange{mustIP("::1"), mustIP("::2")}, true},
		{IPRange{mustIP("::1%foo"), mustIP("::2%foo")}, true},

		{IPRange{mustIP("::1%foo"), mustIP("::2%bar")}, false}, // zones differ
		{IPRange{IP{}, IP{}}, false},                           // zero values
		{IPRange{mustIP("::2"), mustIP("::1")}, false},         // bad order
		{IPRange{mustIP("1.2.3.4"), mustIP("::1")}, false},     // family mismatch
	}
	for _, tt := range tests {
		got := tt.r.IsValid()
		if got != tt.want {
			t.Errorf("range %v to %v Valid = %v; want %v", tt.r.From(), tt.r.To(), got, tt.want)
		}
	}
}

func TestIPRangePrefix(t *testing.T) {
	tests := []struct {
		r    IPRange
		want IPPrefix
	}{
		{IPRange{mustIP("10.0.0.0"), mustIP("10.0.0.255")}, mustIPPrefix("10.0.0.0/24")},
		{IPRange{mustIP("10.0.0.0"), mustIP("10.0.0.254")}, IPPrefix{}},
		{IPRange{mustIP("fc00::"), AddrPrior(mustIP("fe00::"))}, mustIPPrefix("fc00::/7")},
	}
	for _, tt := range tests {
		got, ok := tt.r.Prefix()
		if ok != (got != IPPrefix{}) {
			t.Errorf("for %v, Prefix() results inconsistent: %v, %v", tt.r, got, ok)
		}
		if got != tt.want {
			t.Errorf("for %v, Prefix = %v; want %v", tt.r, got, tt.want)
		}
	}

	allocs := int(testing.AllocsPerRun(1000, func() {
		tt := tests[0]
		if _, ok := tt.r.Prefix(); !ok {
			t.Fatal("expected okay")
		}
	}))
	if allocs != 0 {
		t.Errorf("allocs = %v", allocs)
	}
}

func BenchmarkIPRangePrefix(b *testing.B) {
	b.ReportAllocs()
	r := IPRange{mustIP("10.0.0.0"), mustIP("10.0.0.255")}
	for b.Loop() {
		if _, ok := r.Prefix(); !ok {
			b.Fatal("expected a prefix")
		}
	}
}

var nextPriorTests = []struct {
	ip    IP
	next  IP
	prior IP
}{
	{mustIP("10.0.0.1"), mustIP("10.0.0.2"), mustIP("10.0.0.0")},
	{mustIP("10.0.0.255"), mustIP("10.0.1.0"), mustIP("10.0.0.254")},
	{mustIP("127.0.0.1"), mustIP("127.0.0.2"), mustIP("127.0.0.0")},
	{mustIP("254.255.255.255"), mustIP("255.0.0.0"), mustIP("254.255.255.254")},
	{mustIP("255.255.255.255"), IP{}, mustIP("255.255.255.254")},
	{mustIP("0.0.0.0"), mustIP("0.0.0.1"), IP{}},
	{mustIP("::"), mustIP("::1"), IP{}},
	{mustIP("::%x"), mustIP("::1%x"), IP{}},
	{mustIP("::1"), mustIP("::2"), mustIP("::")},
	{mustIP("ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff"), IP{}, mustIP("ffff:ffff:ffff:ffff:ffff:ffff:ffff:fffe")},
}

func TestAddrNextPrior(t *testing.T) {
	doNextPrior(t)

	for _, ip := range []IP{
		mustIP("0.0.0.0"),
		mustIP("::"),
	} {
		got := AddrPrior(ip)
		if got.IsValid() {
			t.Errorf("IP(%v).Prior = %v; want zero", ip, got)
		}
	}

	var allFF [16]byte
	for i := range allFF {
		allFF[i] = 0xff
	}

	for _, ip := range []IP{
		mustIP("255.255.255.255"),
		netip.AddrFrom16(allFF),
	} {
		got := ip.Next()
		if got.IsValid() {
			t.Errorf("IP(%v).Next = %v; want zero", ip, got)
		}
	}
}

func BenchmarkIPNextPrior(b *testing.B) {
	for b.Loop() {
		doNextPrior(b)
	}
}

func doNextPrior(t testing.TB) {
	for _, tt := range nextPriorTests {
		gnext, gprior := AddrNext(tt.ip), AddrPrior(tt.ip)
		if gnext != tt.next {
			t.Errorf("IP(%v).Next = %v; want %v", tt.ip, gnext, tt.next)
		}
		if gprior != tt.prior {
			t.Errorf("IP(%v).Prior = %v; want %v", tt.ip, gprior, tt.prior)
		}
		if AddrNext(tt.ip).IsValid() && AddrPrior(AddrNext(tt.ip)) != tt.ip {
			t.Errorf("IP(%v).Next.Prior = %v; want %v", tt.ip, AddrPrior(AddrNext(tt.ip)), tt.ip)
		}
		if AddrPrior(tt.ip).IsValid() && AddrNext(AddrPrior(tt.ip)) != tt.ip {
			t.Errorf("IP(%v).Prior.Next = %v; want %v", tt.ip, AddrNext(AddrPrior(tt.ip)), tt.ip)
		}
	}
}

// Sink variables are here to force the compiler to not elide
// seemingly useless work in benchmarks and allocation tests. If you
// were to just `_ = foo()` within a test function, the compiler could
// correctly deduce that foo() does nothing and doesn't need to be
// called. By writing results to a global variable, we hide that fact
// from the compiler and force it to keep the code under test.
var (
	sinkIP            IP
	sinkStdIP         net.IP
	sinkIPPort        IPPort
	sinkIPPrefix      IPPrefix
	sinkIPPrefixSlice []IPPrefix
	sinkIPRange       IPRange
	sinkIP16          [16]byte
	sinkIP4           [4]byte
	sinkBool          bool
	sinkString        string
	sinkBytes         []byte
	sinkUDPAddr       = &net.UDPAddr{IP: make(net.IP, 0, 16)}
)

func TestNoAllocs(t *testing.T) {
	// Wrappers that panic on error, to prove that our alloc-free
	// methods are returning successfully.
	panicIPOK := func(ip IP, ok bool) IP {
		if !ok {
			panic("not ok")
		}
		return ip
	}
	panicPfxOK := func(pfx IPPrefix, ok bool) IPPrefix {
		if !ok {
			panic("not ok")
		}
		return pfx
	}
	panicIPPOK := func(ipp IPPort, ok bool) IPPort {
		if !ok {
			panic("not ok")
		}
		return ipp
	}

	test := func(name string, f func()) {
		t.Run(name, func(t *testing.T) {
			n := testing.AllocsPerRun(1000, f)
			if n != 0 {
				t.Fatalf("allocs = %d; want 0", int(n))
			}
		})
	}

	// IP constructors
	test("FromStdIP", func() { sinkIP = panicIPOK(FromStdIP(net.IP([]byte{1, 2, 3, 4}))) })
	test("FromStdIPRaw", func() { sinkIP = panicIPOK(FromStdIPRaw(net.IP([]byte{1, 2, 3, 4}))) })

	// IPPort constructors
	test("FromStdAddr", func() {
		std := net.IP{1, 2, 3, 4}
		sinkIPPort = panicIPPOK(FromStdAddr(std, 5678, ""))
	})

	// IPPrefix constructors
	test("FromStdIPNet", func() {
		std := &net.IPNet{
			IP:   net.IP{1, 2, 3, 4},
			Mask: net.IPMask{255, 255, 0, 0},
		}
		sinkIPPrefix = panicPfxOK(FromStdIPNet(std))
	})

	// IPRange constructors
	test("IPRangeFrom", func() { sinkIPRange = IPRangeFrom(IPv4(1, 2, 3, 4), IPv4(4, 3, 2, 1)) })
	test("ParseIPRange", func() { sinkIPRange = MustParseIPRange("1.2.3.0-1.2.4.150") })

	// IPRange methods
	test("IPRange.IsZero", func() { sinkBool = MustParseIPRange("1.2.3.0-1.2.4.150").IsZero() })
	test("IPRange.IsValid", func() { sinkBool = MustParseIPRange("1.2.3.0-1.2.4.150").IsValid() })
	test("IPRange.Overlaps", func() {
		a := MustParseIPRange("1.2.3.0-1.2.3.150")
		b := MustParseIPRange("1.2.4.0-1.2.4.255")
		sinkBool = a.Overlaps(b)
	})
	test("IPRange.Prefix", func() {
		a := MustParseIPRange("1.2.3.0-1.2.3.255")
		sinkIPPrefix = panicPfxOK(a.Prefix())
	})
}

func TestComparePrefix(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		// sort by address family
		{"1.0.0.0/8", "1::/8", -1},
		{"1::/8", "1.0.0.0/8", 1},

		// then sort by bitlen
		{"1.0.0.0/7", "1.0.0.0/8", -1},
		{"1.0.0.0/8", "1.0.0.0/7", 1},
		{"1.0.0.0/8", "1.0.0.0/8", 0},

		// then sort by addr
		{"1.0.0.0/8", "1.0.0.1/8", -1},
		{"1.0.0.1/8", "1.0.0.0/8", 1},
	}
	for _, tt := range tests {
		if got := ComparePrefix(netip.MustParsePrefix(tt.a), netip.MustParsePrefix(tt.b)); got != tt.want {
			t.Errorf("f(%q, %q) = %v; want %v", tt.a, tt.b, got, tt.want)
		}
	}
}
