package stun

import (
	"net"
	"testing"
)

func TestBindingRoundTrip(t *testing.T) {
	id, err := NewTransactionID()
	if err != nil {
		t.Fatal(err)
	}

	req := EncodeBindingRequest(id)
	if !IsBindingRequest(req) {
		t.Fatal("request not recognized as a binding request")
	}
	gotType, gotID, err := ParseHeader(req)
	if err != nil {
		t.Fatal(err)
	}
	if gotType != bindingRequest || gotID != id {
		t.Fatalf("header mismatch: type=%d id=%x", gotType, gotID)
	}

	observed := &net.UDPAddr{IP: net.IPv4(100, 64, 3, 14), Port: 51000}
	resp := EncodeXORMappedAddress(id, observed)
	if !IsBindingResponse(resp) {
		t.Fatal("response not recognized as a binding response")
	}
	if _, gotID, err := ParseHeader(resp); err != nil || gotID != id {
		t.Fatalf("response transaction id mismatch: %x %v", gotID, err)
	}

	addr, err := ParseXORMappedAddress(resp, id)
	if err != nil {
		t.Fatal(err)
	}
	if !addr.IP.Equal(observed.IP) || addr.Port != observed.Port {
		t.Fatalf("decoded %v, want %v", addr, observed)
	}
}

func TestBindingIPv6(t *testing.T) {
	id, err := NewTransactionID()
	if err != nil {
		t.Fatal(err)
	}
	observed := &net.UDPAddr{IP: net.ParseIP("2001:db8::42"), Port: 60000}
	resp := EncodeXORMappedAddress(id, observed)
	addr, err := ParseXORMappedAddress(resp, id)
	if err != nil {
		t.Fatal(err)
	}
	if !addr.IP.Equal(observed.IP) || addr.Port != observed.Port {
		t.Fatalf("decoded %v, want %v", addr, observed)
	}
}

func TestParseHeaderRejectsGarbage(t *testing.T) {
	if IsBindingRequest([]byte{0, 0, 0, 0}) {
		t.Fatal("short message accepted")
	}
	bad := EncodeBindingRequest([12]byte{})
	bad[4] = 0 // clobber the magic cookie
	if IsBindingResponse(bad) {
		t.Fatal("bad magic cookie accepted")
	}
	if _, err := ParseXORMappedAddress(bad, [12]byte{}); err == nil {
		t.Fatal("expected error for message without the attribute")
	}
}
