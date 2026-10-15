package base

import (
	"testing"
)

func TestHTTPRequestAuth(t *testing.T) {
	secret, err := NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	id := secret.Public()

	ts := int64(1700000000)
	body := []byte(`{"addrs":["192.168.1.5:7000"]}`)
	method := "POST"
	uri := "/endpoints"

	sig := SignHTTPRequest(secret, method, uri, body, ts)
	if !VerifyHTTPRequest(id, method, uri, body, ts, sig) {
		t.Fatal("valid signature rejected")
	}

	// Wrong node id.
	other, _ := NewNodeSecret()
	if VerifyHTTPRequest(other.Public(), method, uri, body, ts, sig) {
		t.Fatal("signature accepted for the wrong identity")
	}

	// Wrong method.
	if VerifyHTTPRequest(id, "GET", uri, body, ts, sig) {
		t.Fatal("signature accepted for the wrong method")
	}

	// Wrong URI.
	if VerifyHTTPRequest(id, method, "/other", body, ts, sig) {
		t.Fatal("signature accepted for the wrong uri")
	}

	// Tampered body.
	if VerifyHTTPRequest(id, method, uri, []byte(`{"addrs":["10.0.0.1:9"]}`), ts, sig) {
		t.Fatal("signature accepted for a tampered body")
	}

	// Wrong timestamp.
	if VerifyHTTPRequest(id, method, uri, body, ts+1, sig) {
		t.Fatal("signature accepted for the wrong timestamp")
	}

	// Truncated signature.
	if VerifyHTTPRequest(id, method, uri, body, ts, sig[:32]) {
		t.Fatal("truncated signature accepted")
	}
}

func TestAuthHeaderRoundTrip(t *testing.T) {
	secret, err := NewNodeSecret()
	if err != nil {
		t.Fatal(err)
	}
	id := secret.Public()
	ts := int64(1234)
	body := []byte("hello")

	sig := SignHTTPRequest(secret, "GET", "/endpoints/"+id.String(), body, ts)
	header := BuildAuthHeader(id, ts, sig)

	gotID, gotTS, gotSig, err := ParseAuthHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if gotID != id {
		t.Fatalf("id mismatch: %v vs %v", gotID, id)
	}
	if gotTS != ts {
		t.Fatalf("timestamp mismatch: %d vs %d", gotTS, ts)
	}
	if !VerifyHTTPRequest(gotID, "GET", "/endpoints/"+id.String(), body, gotTS, gotSig) {
		t.Fatal("round-tripped signature does not verify")
	}

	if _, _, _, err := ParseAuthHeader("Bearer token"); err == nil {
		t.Fatal("expected error for a non-iron authorization header")
	}
}
