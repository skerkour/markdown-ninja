package jwt

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/mldsa"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestJWKRoundTrip_Ed25519PrivateKey(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	kid := "test-ed25519-priv"
	jsonData, err := json.Marshal(JWK{Key: priv, Alg: EdDSA, ID: kid})
	if err != nil {
		t.Fatal(err)
	}

	var result JWK
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatal(err)
	}

	if result.Alg != EdDSA {
		t.Fatalf("expected Alg EdDSA, got %s", result.Alg)
	}
	if result.ID != kid {
		t.Fatalf("expected KID %s, got %s", kid, result.ID)
	}

	parsed, ok := result.Key.(ed25519.PrivateKey)
	if !ok {
		t.Fatalf("expected ed25519.PrivateKey, got %T", result.Key)
	}
	if !parsed.Equal(priv) {
		t.Fatal("keys not equal")
	}
}

func TestJWKRoundTrip_Ed25519PublicKey(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	jsonData, err := json.Marshal(JWK{Key: pub, Alg: EdDSA})
	if err != nil {
		t.Fatal(err)
	}

	var result JWK
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatal(err)
	}

	if result.Alg != EdDSA {
		t.Fatalf("expected Alg EdDSA, got %s", result.Alg)
	}

	parsed, ok := result.Key.(ed25519.PublicKey)
	if !ok {
		t.Fatalf("expected ed25519.PublicKey, got %T", result.Key)
	}
	if len(parsed) == 0 {
		t.Fatal("empty public key")
	}
}

func TestJWKRoundTrip_ECDSAP256PrivateKey(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	jsonData, err := json.Marshal(JWK{Key: priv, Alg: ES256, ID: "p256"})
	if err != nil {
		t.Fatal(err)
	}

	var result JWK
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatal(err)
	}

	if result.Alg != ES256 {
		t.Fatalf("expected Alg ES256, got %s", result.Alg)
	}
	if result.ID != "p256" {
		t.Fatalf("expected KID p256, got %s", result.ID)
	}

	parsed, ok := result.Key.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatalf("expected *ecdsa.PrivateKey, got %T", result.Key)
	}
	if priv.D.Cmp(parsed.D) != 0 {
		t.Fatal("private keys not equal")
	}
}

func TestJWKRoundTrip_ECDSAP256PublicKey(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub := &priv.PublicKey

	jsonData, err := json.Marshal(JWK{Key: pub, Alg: ES256})
	if err != nil {
		t.Fatal(err)
	}

	var result JWK
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatal(err)
	}

	if result.Alg != ES256 {
		t.Fatalf("expected Alg ES256, got %s", result.Alg)
	}

	parsed, ok := result.Key.(*ecdsa.PublicKey)
	if !ok {
		t.Fatalf("expected *ecdsa.PublicKey, got %T", result.Key)
	}
	if parsed.X == nil || parsed.Y == nil {
		t.Fatal("empty public key coordinates")
	}
	_ = parsed
}

func TestJWKRoundTrip_ECDSAP384PrivateKey(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	jsonData, err := json.Marshal(JWK{Key: priv, Alg: ES384})
	if err != nil {
		t.Fatal(err)
	}

	var result JWK
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatal(err)
	}

	if result.Alg != ES384 {
		t.Fatalf("expected Alg ES384, got %s", result.Alg)
	}

	parsed, ok := result.Key.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatalf("expected *ecdsa.PrivateKey, got %T", result.Key)
	}
	if priv.D.Cmp(parsed.D) != 0 {
		t.Fatal("keys not equal")
	}
}

func TestJWKRoundTrip_ECDSAP521PrivateKey(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	jsonData, err := json.Marshal(JWK{Key: priv, Alg: ES512})
	if err != nil {
		t.Fatal(err)
	}

	var result JWK
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatal(err)
	}

	if result.Alg != ES512 {
		t.Fatalf("expected Alg ES512, got %s", result.Alg)
	}

	parsed, ok := result.Key.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatalf("expected *ecdsa.PrivateKey, got %T", result.Key)
	}
	if priv.D.Cmp(parsed.D) != 0 {
		t.Fatal("keys not equal")
	}
}

func TestJWKRoundTrip_RSAPrivateKey(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	jsonData, err := json.Marshal(JWK{Key: priv, Alg: RS256, ID: "rsa-2048"})
	if err != nil {
		t.Fatal(err)
	}

	var result JWK
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatal(err)
	}

	if result.Alg != RS256 {
		t.Fatalf("expected Alg RS256, got %s", result.Alg)
	}
	if result.ID != "rsa-2048" {
		t.Fatalf("expected KID rsa-2048, got %s", result.ID)
	}

	parsed, ok := result.Key.(*rsa.PrivateKey)
	if !ok {
		t.Fatalf("expected *rsa.PrivateKey, got %T", result.Key)
	}
	if priv.D.Cmp(parsed.D) != 0 {
		t.Fatal("keys not equal")
	}
	if len(parsed.Primes) == 0 {
		t.Fatal("missing prime factors")
	}
}

func TestJWKRoundTrip_RSAPublicKey(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pub := &priv.PublicKey

	jsonData, err := json.Marshal(JWK{Key: pub, Alg: PS384})
	if err != nil {
		t.Fatal(err)
	}

	var result JWK
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatal(err)
	}

	if result.Alg != PS384 {
		t.Fatalf("expected Alg PS384, got %s", result.Alg)
	}

	parsed, ok := result.Key.(*rsa.PublicKey)
	if !ok {
		t.Fatalf("expected *rsa.PublicKey, got %T", result.Key)
	}
	if parsed.N.Cmp(pub.N) != 0 || parsed.E != pub.E {
		t.Fatal("keys not equal")
	}
}

func TestJWKRoundTrip_HMAC(t *testing.T) {
	tests := []struct {
		alg Algorithm
		len int
	}{
		{HS256, 32},
		{HS384, 48},
		{HS512, 64},
	}

	for _, tt := range tests {
		key := make([]byte, tt.len)
		if _, err := rand.Read(key); err != nil {
			t.Fatal(err)
		}

		jsonData, err := json.Marshal(JWK{Key: []byte(key), Alg: tt.alg})
		if err != nil {
			t.Fatalf("json.Marshal(%s): %v", tt.alg, err)
		}

		var result JWK
		if err := json.Unmarshal(jsonData, &result); err != nil {
			t.Fatalf("json.Unmarshal(%s): %v", tt.alg, err)
		}

		if result.Alg != tt.alg {
			t.Fatalf("expected Alg %s, got %s", tt.alg, result.Alg)
		}

		parsed, ok := result.Key.([]byte)
		if !ok {
			t.Fatalf("expected []byte, got %T", result.Key)
		}
		if len(parsed) != tt.len {
			t.Fatalf("expected key length %d, got %d", tt.len, len(parsed))
		}
	}
}

func TestJWKRoundTrip_ECDSAalgIgnored(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	jsonData, err := json.Marshal(JWK{Key: priv, Alg: "WRONG_ALG"})
	if err != nil {
		t.Fatal(err)
	}

	var result JWK
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatal(err)
	}

	if result.Alg != ES256 {
		t.Fatalf("expected Alg ES256 (derived from curve), got %s", result.Alg)
	}
}

func TestJWKRoundTrip_EdDSAalgIgnored(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	jsonData, err := json.Marshal(JWK{Key: priv, Alg: "WRONG"})
	if err != nil {
		t.Fatal(err)
	}

	var result JWK
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatal(err)
	}

	if result.Alg != EdDSA {
		t.Fatalf("expected Alg EdDSA, got %s", result.Alg)
	}
}

// ── Error cases ─────────────────────────────────────────────

func TestJWKError_UnsupportedKeyType(t *testing.T) {
	_, err := json.Marshal(JWK{Key: "not-a-key", Alg: HS256})
	if !errors.Is(err, ErrUnsupportedKeyType) {
		t.Fatalf("expected ErrUnsupportedKeyType, got %v", err)
	}
}

func TestJWKError_InvalidEd25519Size(t *testing.T) {
	_, err := json.Marshal(JWK{Key: ed25519.PrivateKey{1, 2, 3}, Alg: EdDSA})
	if !errors.Is(err, ErrInvalidJWK) {
		t.Fatalf("expected ErrInvalidJWK, got %v", err)
	}
}

func TestJWKError_ECDSAwithP224(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	_, err = json.Marshal(JWK{Key: priv, Alg: ES256})
	if !errors.Is(err, ErrUnsupportedCurve) {
		t.Fatalf("expected ErrUnsupportedCurve, got %v", err)
	}
}

func TestJWKError_RSABadAlgorithm(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	_, err = json.Marshal(JWK{Key: priv, Alg: HS256})
	if !errors.Is(err, ErrInvalidAlgorithm) {
		t.Fatalf("expected ErrInvalidAlgorithm, got %v", err)
	}

	_, err = json.Marshal(JWK{Key: &priv.PublicKey, Alg: HS256})
	if !errors.Is(err, ErrInvalidAlgorithm) {
		t.Fatalf("expected ErrInvalidAlgorithm, got %v", err)
	}
}

func TestJWKError_HMACBadAlgorithm(t *testing.T) {
	key := make([]byte, 32)

	_, err := json.Marshal(JWK{Key: key, Alg: RS256})
	if !errors.Is(err, ErrInvalidAlgorithm) {
		t.Fatalf("expected ErrInvalidAlgorithm, got %v", err)
	}
}

func TestJWKError_ParseBadJSON(t *testing.T) {
	var jwk JWK
	err := json.Unmarshal([]byte("{bad json"), &jwk)
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("expected *json.SyntaxError, got %v (%T)", err, err)
	}
}

func TestJWKError_ParseUnknownKTY(t *testing.T) {
	var jwk JWK
	err := json.Unmarshal([]byte(`{"kty":"UNKNOWN"}`), &jwk)
	if err != ErrUnsupportedKeyType {
		t.Fatalf("expected ErrUnsupportedKeyType, got %v", err)
	}
}

func TestJWKError_ParseUnsupportedCurve(t *testing.T) {
	var jwk JWK
	err := json.Unmarshal([]byte(`{"kty":"EC","crv":"P-224","x":"","y":""}`), &jwk)
	if err != ErrUnsupportedCurve {
		t.Fatalf("expected ErrUnsupportedCurve, got %v", err)
	}
}

func TestJWKError_ParseBadBase64(t *testing.T) {
	var jwk JWK
	err := json.Unmarshal([]byte(`{"kty":"oct","k":"!!!not-base64!!!"}`), &jwk)
	if err != ErrInvalidJWK {
		t.Fatalf("expected ErrInvalidJWK, got %v", err)
	}
}

func TestJWKError_ParseMissingKTY(t *testing.T) {
	var jwk JWK
	err := json.Unmarshal([]byte(`{"alg":"RS256"}`), &jwk)
	if err != ErrUnsupportedKeyType {
		t.Fatalf("expected ErrUnsupportedKeyType, got %v", err)
	}
}

func TestJWKError_ParseNonRSAalg(t *testing.T) {
	data := []byte(`{"kty":"RSA","n":"AQAB","e":"AQAB","alg":"HS256"}`)
	var jwk JWK
	err := json.Unmarshal(data, &jwk)
	if err != ErrInvalidAlgorithm {
		t.Fatalf("expected ErrInvalidAlgorithm, got %v", err)
	}
}

// ── Known test vectors ──────────────────────────────────────

func TestJWKVector_Ed25519PublicKey(t *testing.T) {
	knownJWK := `{
		"kty":"OKP",
		"crv":"Ed25519",
		"x":"11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaNcAlUM",
		"alg":"EdDSA"
	}`

	var result JWK
	if err := json.Unmarshal([]byte(knownJWK), &result); err != nil {
		t.Fatal(err)
	}

	if result.Alg != EdDSA {
		t.Fatalf("expected Alg EdDSA, got %s", result.Alg)
	}

	pub, ok := result.Key.(ed25519.PublicKey)
	if !ok {
		t.Fatalf("expected ed25519.PublicKey, got %T", result.Key)
	}

	if len(pub) != ed25519.PublicKeySize {
		t.Fatalf("expected %d bytes, got %d", ed25519.PublicKeySize, len(pub))
	}
}

func TestJWKVector_Ed25519PrivateKey(t *testing.T) {
	knownJWK := `{
		"kty":"OKP",
		"crv":"Ed25519",
		"d":"nWGxne_9WmC6hEr0kuwsxERJxWl7M0ZQWPN7i_J2G6E",
		"x":"11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaNcAlUM",
		"alg":"EdDSA"
	}`

	var result JWK
	if err := json.Unmarshal([]byte(knownJWK), &result); err != nil {
		t.Fatal(err)
	}

	if result.Alg != EdDSA {
		t.Fatalf("expected Alg EdDSA, got %s", result.Alg)
	}

	priv, ok := result.Key.(ed25519.PrivateKey)
	if !ok {
		t.Fatalf("expected ed25519.PrivateKey, got %T", result.Key)
	}

	if len(priv) != ed25519.PrivateKeySize {
		t.Fatalf("expected %d bytes, got %d", ed25519.PrivateKeySize, len(priv))
	}
}

func TestJWKVector_ECDSAP256Public(t *testing.T) {
	knownJWK := `{
		"kty":"EC",
		"crv":"P-256",
		"x":"MKBCTNIcKUSDii11ySs3526iDZ8AiTo7Tu6KPAqv7D4",
		"y":"4Etl6SRW2YiLUrN5vfvVHuhp7x8PxltmWWlbbM4IFyM",
		"alg":"ES256"
	}`

	var result JWK
	if err := json.Unmarshal([]byte(knownJWK), &result); err != nil {
		t.Fatal(err)
	}

	if result.Alg != ES256 {
		t.Fatalf("expected Alg ES256, got %s", result.Alg)
	}

	pub, ok := result.Key.(*ecdsa.PublicKey)
	if !ok {
		t.Fatalf("expected *ecdsa.PublicKey, got %T", result.Key)
	}

	if pub.X == nil || pub.Y == nil {
		t.Fatal("empty coordinates")
	}
	if pub.Curve != elliptic.P256() {
		t.Fatal("wrong curve")
	}
}

func TestJWKVector_ECDSAP256Private(t *testing.T) {
	knownJWK := `{
		"kty":"EC",
		"crv":"P-256",
		"x":"MKBCTNIcKUSDii11ySs3526iDZ8AiTo7Tu6KPAqv7D4",
		"y":"4Etl6SRW2YiLUrN5vfvVHuhp7x8PxltmWWlbbM4IFyM",
		"d":"870MB6gfuTJ4HtUnUvYMyJpr5eUZNP4Bk43bVdj3eAE",
		"alg":"ES256"
	}`

	var result JWK
	if err := json.Unmarshal([]byte(knownJWK), &result); err != nil {
		t.Fatal(err)
	}

	if result.Alg != ES256 {
		t.Fatalf("expected Alg ES256, got %s", result.Alg)
	}

	priv, ok := result.Key.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatalf("expected *ecdsa.PrivateKey, got %T", result.Key)
	}

	if priv.D == nil {
		t.Fatal("missing private scalar")
	}
}

func TestJWKVector_RSA(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	jsonData, err := json.Marshal(JWK{Key: priv, Alg: RS256, ID: "test-rsa-kid"})
	if err != nil {
		t.Fatal(err)
	}
	var result JWK
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatal(err)
	}

	if result.Alg != RS256 {
		t.Fatalf("expected Alg RS256, got %s", result.Alg)
	}
	if result.ID != "test-rsa-kid" {
		t.Fatalf("expected KID test-rsa-kid, got %s", result.ID)
	}

	pub, ok := result.Key.(*rsa.PrivateKey)
	if !ok {
		t.Fatalf("expected *rsa.PrivateKey, got %T", result.Key)
	}
	if pub.N == nil || pub.D == nil {
		t.Fatal("empty RSA key")
	}
}

// ── MarshalJSON with kid ───────────────────────────────────

func TestJWKEncode_WithKID(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	jsonData, err := json.Marshal(JWK{Key: priv, Alg: EdDSA, ID: "my-key-1"})
	if err != nil {
		t.Fatal(err)
	}

	var raw struct {
		KID string `json:"kid"`
	}
	if err := json.Unmarshal(jsonData, &raw); err != nil {
		t.Fatal(err)
	}
	if raw.KID != "my-key-1" {
		t.Fatalf("expected kid my-key-1, got %s", raw.KID)
	}
}

func TestJWKEncode_WithoutKID(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	jsonData, err := json.Marshal(JWK{Key: priv, Alg: EdDSA})
	if err != nil {
		t.Fatal(err)
	}

	var raw struct {
		KID string `json:"kid,omitempty"`
	}
	if err := json.Unmarshal(jsonData, &raw); err != nil {
		t.Fatal(err)
	}
	if raw.KID != "" {
		t.Fatalf("expected empty kid, got %s", raw.KID)
	}
}

// ── Edge cases ──────────────────────────────────────────────

func TestJWKRoundTrip_Ed25519EmptyKID(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	jsonData, err := json.Marshal(JWK{Key: priv, Alg: EdDSA})
	if err != nil {
		t.Fatal(err)
	}

	var result JWK
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatal(err)
	}
	if result.ID != "" {
		t.Fatalf("expected empty KID, got %s", result.ID)
	}
}

func TestJWKParse_OctWithAlg(t *testing.T) {
	data := []byte(`{"kty":"oct","k":"Zm9vYmFy","alg":"HS256","kid":"sym-key"}`)
	var result JWK
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}

	if result.Alg != HS256 {
		t.Fatalf("expected HS256, got %s", result.Alg)
	}
	if result.ID != "sym-key" {
		t.Fatalf("expected sym-key, got %s", result.ID)
	}
	key, ok := result.Key.([]byte)
	if !ok {
		t.Fatalf("expected []byte, got %T", result.Key)
	}
	if string(key) != "foobar" {
		t.Fatalf("expected foobar, got %s", key)
	}
}

// ── JWKS (JSON Web Key Set) ─────────────────────────────────

func TestJWKSRoundTrip_MixedKeys(t *testing.T) {
	_, edPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ecPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rsaPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	hmacKey := make([]byte, 32)
	rand.Read(hmacKey)

	var raw struct {
		Keys []JWK `json:"keys"`
	}
	raw.Keys = []JWK{
		{Key: edPriv, Alg: EdDSA, ID: "ed25519-key"},
		{Key: ecPriv, Alg: ES256, ID: "ecdsa-key"},
		{Key: rsaPriv, Alg: RS256, ID: "rsa-key"},
		{Key: []byte(hmacKey), Alg: HS256, ID: "hmac-key"},
	}

	jwksJSON, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Keys []JWK `json:"keys"`
	}
	if err := json.Unmarshal(jwksJSON, &parsed); err != nil {
		t.Fatal(err)
	}
	keys := parsed.Keys

	if len(keys) != 4 {
		t.Fatalf("expected 4 keys, got %d", len(keys))
	}

	if keys[0].ID != "ed25519-key" || keys[0].Alg != EdDSA {
		t.Fatalf("unexpected key 0: Alg=%s ID=%s", keys[0].Alg, keys[0].ID)
	}
	if _, ok := keys[0].Key.(ed25519.PrivateKey); !ok {
		t.Fatalf("expected ed25519.PrivateKey, got %T", keys[0].Key)
	}

	if keys[1].ID != "ecdsa-key" || keys[1].Alg != ES256 {
		t.Fatalf("unexpected key 1: Alg=%s ID=%s", keys[1].Alg, keys[1].ID)
	}
	if _, ok := keys[1].Key.(*ecdsa.PrivateKey); !ok {
		t.Fatalf("expected *ecdsa.PrivateKey, got %T", keys[1].Key)
	}

	if keys[2].ID != "rsa-key" || keys[2].Alg != RS256 {
		t.Fatalf("unexpected key 2: Alg=%s ID=%s", keys[2].Alg, keys[2].ID)
	}
	if _, ok := keys[2].Key.(*rsa.PrivateKey); !ok {
		t.Fatalf("expected *rsa.PrivateKey, got %T", keys[2].Key)
	}

	if keys[3].ID != "hmac-key" || keys[3].Alg != HS256 {
		t.Fatalf("unexpected key 3: Alg=%s ID=%s", keys[3].Alg, keys[3].ID)
	}
	if _, ok := keys[3].Key.([]byte); !ok {
		t.Fatalf("expected []byte, got %T", keys[3].Key)
	}
}

func TestJWKS_EmptySet(t *testing.T) {
	var raw struct {
		Keys []JWK `json:"keys"`
	}
	if err := json.Unmarshal([]byte(`{"keys":[]}`), &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw.Keys) != 0 {
		t.Fatalf("expected 0 keys, got %d", len(raw.Keys))
	}
}

func TestJWKS_SingleKey(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	var raw struct {
		Keys []JWK `json:"keys"`
	}
	raw.Keys = []JWK{{Key: priv, Alg: EdDSA, ID: "single"}}
	jwksJSON, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Keys []JWK `json:"keys"`
	}
	if err := json.Unmarshal(jwksJSON, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(parsed.Keys))
	}
	if parsed.Keys[0].ID != "single" {
		t.Fatalf("expected ID single, got %s", parsed.Keys[0].ID)
	}
}

func TestJWKS_WithPublicKeys(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ecPriv, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	var raw struct {
		Keys []JWK `json:"keys"`
	}
	raw.Keys = []JWK{
		{Key: pub, Alg: EdDSA},
		{Key: &ecPriv.PublicKey, Alg: ES384},
	}
	jwksJSON, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Keys []JWK `json:"keys"`
	}
	if err := json.Unmarshal(jwksJSON, &parsed); err != nil {
		t.Fatal(err)
	}

	keys := parsed.Keys
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	if keys[0].Alg != EdDSA {
		t.Fatalf("expected EdDSA, got %s", keys[0].Alg)
	}
	if keys[1].Alg != ES384 {
		t.Fatalf("expected ES384, got %s", keys[1].Alg)
	}
}

// ── JWKS Error cases ────────────────────────────────────────

func TestJWKSError_BadJSON(t *testing.T) {
	var raw struct {
		Keys []JWK `json:"keys"`
	}
	err := json.Unmarshal([]byte(`{bad json`), &raw)
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("expected *json.SyntaxError, got %v (%T)", err, err)
	}
}

func TestJWKSError_NoKeysField(t *testing.T) {
	var raw struct {
		Keys []JWK `json:"keys"`
	}
	if err := json.Unmarshal([]byte(`{"kty":"RSA"}`), &raw); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(raw.Keys) != 0 {
		t.Fatalf("expected 0 keys, got %d", len(raw.Keys))
	}
}

func TestJWKSError_KeysNotArray(t *testing.T) {
	var raw struct {
		Keys []JWK `json:"keys"`
	}
	err := json.Unmarshal([]byte(`{"keys":"not-an-array"}`), &raw)
	var typeErr *json.UnmarshalTypeError
	if !errors.As(err, &typeErr) {
		t.Fatalf("expected *json.UnmarshalTypeError, got %v (%T)", err, err)
	}
}

func TestJWKSError_InvalidKeyInSet(t *testing.T) {
	jwksJSON := `{"keys":[
		{"kty":"OKP","crv":"Ed25519","x":"11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaNcAlUM"},
		{"kty":"UNKNOWN"}
	]}`

	var raw struct {
		Keys []JWK `json:"keys"`
	}
	err := json.Unmarshal([]byte(jwksJSON), &raw)
	if err != ErrUnsupportedKeyType {
		t.Fatalf("expected ErrUnsupportedKeyType, got %v", err)
	}
}

func TestJWKSError_BadBase64InSet(t *testing.T) {
	jwksJSON := `{"keys":[
		{"kty":"oct","k":"!!!not-base64!!!"}
	]}`

	var raw struct {
		Keys []JWK `json:"keys"`
	}
	err := json.Unmarshal([]byte(jwksJSON), &raw)
	if err != ErrInvalidJWK {
		t.Fatalf("expected ErrInvalidJWK, got %v", err)
	}
}

func TestJWKEncode_RSAPublicNoDuplicateN(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pub := &priv.PublicKey

	jsonData, err := json.Marshal(JWK{Key: pub, Alg: RS256})
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	pattern := `"n":"`
	for i := 0; i <= len(jsonData)-len(pattern); i++ {
		if string(jsonData[i:i+len(pattern)]) == pattern {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 n field, got %d in %s", count, string(jsonData))
	}
}

// ── AKP / ML-DSA JWK tests ─────────────────────────────────

func TestJWKRoundTrip_MLDSA44PublicKey(t *testing.T) {
	priv, err := mldsa.GenerateKey(mldsa.MLDSA44())
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.PublicKey()

	jsonData, err := json.Marshal(JWK{Key: pub, Alg: MLDSA44, ID: "mldsa44-pub"})
	if err != nil {
		t.Fatal(err)
	}

	var result JWK
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.Alg != MLDSA44 {
		t.Fatalf("expected Alg ML-DSA-44, got %s", result.Alg)
	}
	if result.ID != "mldsa44-pub" {
		t.Fatalf("expected kid mldsa44-pub, got %s", result.ID)
	}

	parsed, ok := result.Key.(*mldsa.PublicKey)
	if !ok {
		t.Fatalf("expected *mldsa.PublicKey, got %T", result.Key)
	}
	if !pub.Equal(parsed) {
		t.Fatal("public keys not equal")
	}
}

func TestJWKRoundTrip_MLDSA44PrivateKey(t *testing.T) {
	priv, err := mldsa.GenerateKey(mldsa.MLDSA44())
	if err != nil {
		t.Fatal(err)
	}

	jsonData, err := json.Marshal(JWK{Key: priv, Alg: MLDSA44, ID: "mldsa44-priv"})
	if err != nil {
		t.Fatal(err)
	}

	var result JWK
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.Alg != MLDSA44 {
		t.Fatalf("expected Alg ML-DSA-44, got %s", result.Alg)
	}
	if result.ID != "mldsa44-priv" {
		t.Fatalf("expected kid mldsa44-priv, got %s", result.ID)
	}

	parsed, ok := result.Key.(*mldsa.PrivateKey)
	if !ok {
		t.Fatalf("expected *mldsa.PrivateKey, got %T", result.Key)
	}
	if !priv.Equal(parsed) {
		t.Fatal("private keys not equal")
	}
}

func TestJWKRoundTrip_MLDSA65PrivateKey(t *testing.T) {
	priv, err := mldsa.GenerateKey(mldsa.MLDSA65())
	if err != nil {
		t.Fatal(err)
	}

	jsonData, err := json.Marshal(JWK{Key: priv, Alg: MLDSA65})
	if err != nil {
		t.Fatal(err)
	}

	var result JWK
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.Alg != MLDSA65 {
		t.Fatalf("expected Alg ML-DSA-65, got %s", result.Alg)
	}
}

func TestJWKRoundTrip_MLDSA87PrivateKey(t *testing.T) {
	priv, err := mldsa.GenerateKey(mldsa.MLDSA87())
	if err != nil {
		t.Fatal(err)
	}

	jsonData, err := json.Marshal(JWK{Key: priv, Alg: MLDSA87})
	if err != nil {
		t.Fatal(err)
	}

	var result JWK
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.Alg != MLDSA87 {
		t.Fatalf("expected Alg ML-DSA-87, got %s", result.Alg)
	}
}

func TestJWKRoundTrip_MLDSA44AllZeroSeed(t *testing.T) {
	// RFC 9964, Appendix A.1, Figure 1: all-zeros seed ML-DSA-44 key
	seed := make([]byte, 32)
	priv, err := mldsa.NewPrivateKey(mldsa.MLDSA44(), seed)
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.PublicKey()

	jsonData, err := json.Marshal(JWK{Key: priv, Alg: MLDSA44})
	if err != nil {
		t.Fatal(err)
	}

	// Check the JWK has the expected structure
	var raw struct {
		KTY  string `json:"kty"`
		ALG  string `json:"alg"`
		PUB  string `json:"pub"`
		PRIV string `json:"priv"`
	}
	if err := json.Unmarshal(jsonData, &raw); err != nil {
		t.Fatal(err)
	}

	if raw.KTY != "AKP" {
		t.Fatalf("expected kty AKP, got %s", raw.KTY)
	}
	if raw.ALG != "ML-DSA-44" {
		t.Fatalf("expected alg ML-DSA-44, got %s", raw.ALG)
	}
	if raw.PRIV != "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" {
		t.Fatalf("unexpected priv encoding: %s", raw.PRIV)
	}

	// Round-trip verify
	var result JWK
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	parsedPriv, ok := result.Key.(*mldsa.PrivateKey)
	if !ok {
		t.Fatalf("expected *mldsa.PrivateKey, got %T", result.Key)
	}
	if !priv.Equal(parsedPriv) {
		t.Fatal("private keys not equal")
	}
	if !pub.Equal(parsedPriv.PublicKey()) {
		t.Fatal("public keys not equal")
	}
}

func TestJWK_MarshalMLDSA_PrivateKey(t *testing.T) {
	priv, err := mldsa.GenerateKey(mldsa.MLDSA44())
	if err != nil {
		t.Fatal(err)
	}

	jsonData, err := json.Marshal(JWK{Key: priv, Alg: MLDSA44, ID: "mldsa-key"})
	if err != nil {
		t.Fatal(err)
	}

	var raw struct {
		KTY  string `json:"kty"`
		ALG  string `json:"alg"`
		KID  string `json:"kid"`
		PUB  string `json:"pub"`
		PRIV string `json:"priv"`
	}
	if err := json.Unmarshal(jsonData, &raw); err != nil {
		t.Fatal(err)
	}

	if raw.KTY != "AKP" {
		t.Fatalf("expected kty AKP, got %s", raw.KTY)
	}
	if raw.ALG != "ML-DSA-44" {
		t.Fatalf("expected alg ML-DSA-44, got %s", raw.ALG)
	}
	if raw.KID != "mldsa-key" {
		t.Fatalf("expected kid mldsa-key, got %s", raw.KID)
	}
	if raw.PUB == "" {
		t.Fatal("expected non-empty pub")
	}
	if raw.PRIV == "" {
		t.Fatal("expected non-empty priv")
	}
}

func TestJWKError_AKPBadAlg(t *testing.T) {
	var jwk JWK
	err := json.Unmarshal([]byte(`{"kty":"AKP","alg":"EdDSA","pub":"AAAA"}`), &jwk)
	if err != ErrInvalidAlgorithm {
		t.Fatalf("expected ErrInvalidAlgorithm, got %v", err)
	}
}

func TestJWKError_AKPBadPubBase64(t *testing.T) {
	var jwk JWK
	err := json.Unmarshal([]byte(`{"kty":"AKP","alg":"ML-DSA-44","pub":"!!!not-base64!!!"}`), &jwk)
	if err != ErrInvalidJWK {
		t.Fatalf("expected ErrInvalidJWK, got %v", err)
	}
}

func TestJWKError_AKPBadPrivBase64(t *testing.T) {
	var jwk JWK
	err := json.Unmarshal([]byte(`{"kty":"AKP","alg":"ML-DSA-44","pub":"AA","priv":"!!!not-base64!!!"}`), &jwk)
	if err != ErrInvalidJWK {
		t.Fatalf("expected ErrInvalidJWK, got %v", err)
	}
}

func TestJWKError_AKPPrivWrongSize(t *testing.T) {
	// Wrong size seed (16 bytes instead of 32)
	seed := make([]byte, 16)
	privBase64 := base64.RawURLEncoding.EncodeToString(seed)
	pub, err := mldsa.GenerateKey(mldsa.MLDSA44())
	if err != nil {
		t.Fatal(err)
	}
	pubBase64 := base64.RawURLEncoding.EncodeToString(pub.PublicKey().Bytes())
	jwkJSON := fmt.Sprintf(`{"kty":"AKP","alg":"ML-DSA-44","pub":"%s","priv":"%s"}`, pubBase64, privBase64)
	var jwk JWK
	err = json.Unmarshal([]byte(jwkJSON), &jwk)
	if err != ErrInvalidJWK {
		t.Fatalf("expected ErrInvalidJWK for wrong seed size, got %v", err)
	}
}

func TestJWKSError_AKPInSet(t *testing.T) {
	priv, err := mldsa.GenerateKey(mldsa.MLDSA44())
	if err != nil {
		t.Fatal(err)
	}

	var raw struct {
		Keys []JWK `json:"keys"`
	}
	raw.Keys = []JWK{
		{Key: priv, Alg: MLDSA44, ID: "mldsa-in-set"},
		{Key: priv.PublicKey(), Alg: MLDSA44, ID: "mldsa-pub-in-set"},
	}

	jwksJSON, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Keys []JWK `json:"keys"`
	}
	if err := json.Unmarshal(jwksJSON, &parsed); err != nil {
		t.Fatal(err)
	}

	if len(parsed.Keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(parsed.Keys))
	}
	if parsed.Keys[0].Alg != MLDSA44 {
		t.Fatalf("expected ML-DSA-44, got %s", parsed.Keys[0].Alg)
	}
	if parsed.Keys[1].Alg != MLDSA44 {
		t.Fatalf("expected ML-DSA-44, got %s", parsed.Keys[1].Alg)
	}
}
