package jwt

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/mldsa"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"testing"
	"time"
)

func TestSignAndVerify_Ed25519(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.Public().(ed25519.PublicKey)

	header := Header{Typ: JWT, Alg: EdDSA, KID: "ed25519-key"}
	claims := map[string]any{"sub": "user123", "exp": time.Now().Add(time.Hour).Unix()}

	token, err := Sign(priv, &header, claims)
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}
	if parsedHeader.Alg != EdDSA {
		t.Fatalf("expected EdDSA, got %s", parsedHeader.Alg)
	}
	if parsedHeader.KID != "ed25519-key" {
		t.Fatalf("expected ed25519-key, got %s", parsedHeader.KID)
	}

	opts := VerifyOptions{Exp: true, Nbf: false, AllowedTimeDrift: time.Minute}
	result, err := ParseAndVerify[map[string]any](pub, parsedHeader, token, &opts)
	if err != nil {
		t.Fatal(err)
	}
	if result["sub"] != "user123" {
		t.Fatalf("expected user123, got %v", result["sub"])
	}
}

func TestSignAndVerify_Ed25519PrivateKey(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, map[string]any{"sub": "test"})
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ParseAndVerify[map[string]any](priv, parsedHeader, token, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSignAndVerify_ECDSAP256(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub := &priv.PublicKey

	header := Header{Typ: JWT, Alg: ES256}
	claims := map[string]any{"sub": "alice"}

	token, err := Sign(priv, &header, claims)
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}
	if parsedHeader.Alg != ES256 {
		t.Fatalf("expected ES256, got %s", parsedHeader.Alg)
	}

	result, err := ParseAndVerify[map[string]any](pub, parsedHeader, token, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result["sub"] != "alice" {
		t.Fatalf("expected alice, got %v", result["sub"])
	}
}

func TestSignAndVerify_ECDSAP384(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub := &priv.PublicKey

	header := Header{Typ: JWT, Alg: ES384}
	token, err := Sign(priv, &header, map[string]any{"sub": "bob"})
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}
	if parsedHeader.Alg != ES384 {
		t.Fatalf("expected ES384, got %s", parsedHeader.Alg)
	}

	_, err = ParseAndVerify[map[string]any](pub, parsedHeader, token, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSignAndVerify_ECDSAP521(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub := &priv.PublicKey

	header := Header{Typ: JWT, Alg: ES512}
	token, err := Sign(priv, &header, map[string]any{"sub": "carol"})
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}
	if parsedHeader.Alg != ES512 {
		t.Fatalf("expected ES512, got %s", parsedHeader.Alg)
	}

	_, err = ParseAndVerify[map[string]any](pub, parsedHeader, token, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSignAndVerify_HMAC(t *testing.T) {
	tests := []struct {
		alg Algorithm
		key []byte
	}{
		{HS256, make([]byte, 32)},
		{HS384, make([]byte, 48)},
		{HS512, make([]byte, 64)},
	}

	for _, tt := range tests {
		if _, err := rand.Read(tt.key); err != nil {
			t.Fatal(err)
		}

		header := Header{Typ: JWT, Alg: tt.alg}
		token, err := Sign([]byte(tt.key), &header, map[string]any{"sub": "test"})
		if err != nil {
			t.Fatalf("Sign(%s): %v", tt.alg, err)
		}

		parsedHeader, err := ParseHeader(token)
		if err != nil {
			t.Fatalf("ParseHeader(%s): %v", tt.alg, err)
		}

		_, err = ParseAndVerify[map[string]any]([]byte(tt.key), parsedHeader, token, nil)
		if err != nil {
			t.Fatalf("ParseAndVerify(%s): %v", tt.alg, err)
		}
	}
}

func TestSignAndVerify_RSAPublicKey(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pub := &priv.PublicKey

	header := Header{Typ: JWT, Alg: RS256}
	token, err := Sign(priv, &header, map[string]any{"sub": "rsa-user"})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	result, err := ParseAndVerify[map[string]any](pub, parsedHeader, token, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result["sub"] != "rsa-user" {
		t.Fatalf("expected rsa-user, got %v", result["sub"])
	}
}

func TestSignAndVerify_WithRegisteredClaims(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	claims := RegisteredClaims{
		SUB: "user1",
		ISS: "my-app",
		AUD: []string{"my-api"},
		EXP: time.Now().Add(time.Hour).Unix(),
		NBF: time.Now().Add(-time.Minute).Unix(),
		IAT: time.Now().Unix(),
		JTI: "unique-id-123",
	}

	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, claims)
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	opts := VerifyOptions{
		Exp:              true,
		Nbf:              true,
		Aud:              []string{"my-api"},
		Iss:              []string{"my-app"},
		AllowedTimeDrift: time.Minute,
	}

	result, err := ParseAndVerify[RegisteredClaims](priv, parsedHeader, token, &opts)
	if err != nil {
		t.Fatal(err)
	}
	if result.SUB != "user1" {
		t.Fatalf("expected user1, got %s", result.SUB)
	}
}

func TestSignAndVerify_ExpiredToken(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	claims := map[string]any{"exp": time.Now().Add(-time.Hour).Unix()}

	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, claims)
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	opts := VerifyOptions{Exp: true}
	_, err = ParseAndVerify[map[string]any](priv, parsedHeader, token, &opts)
	if err != ErrTokenExpired {
		t.Fatalf("expected ErrTokenExpired, got %v", err)
	}
}

func TestSignAndVerify_NotYetValid(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	claims := map[string]any{"nbf": time.Now().Add(time.Hour).Unix()}

	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, claims)
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	opts := VerifyOptions{Nbf: true}
	_, err = ParseAndVerify[map[string]any](priv, parsedHeader, token, &opts)
	if err != ErrTokenNotYetValid {
		t.Fatalf("expected ErrTokenNotYetValid, got %v", err)
	}
}

func TestSignAndVerify_BadAudience(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	claims := map[string]any{"aud": "wrong-aud"}

	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, claims)
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	opts := VerifyOptions{Aud: []string{"expected-aud"}}
	_, err = ParseAndVerify[map[string]any](priv, parsedHeader, token, &opts)
	if err != ErrInvalidAudience {
		t.Fatalf("expected ErrInvalidAudience, got %v", err)
	}
}

func TestSignAndVerify_BadIssuer(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	claims := map[string]any{"iss": "wrong-iss"}

	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, claims)
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	opts := VerifyOptions{Iss: []string{"expected-iss"}}
	_, err = ParseAndVerify[map[string]any](priv, parsedHeader, token, &opts)
	if err != ErrInvalidIssuer {
		t.Fatalf("expected ErrInvalidIssuer, got %v", err)
	}
}

func TestParseAndVerify_WrongKey(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, wrongPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, map[string]any{"sub": "test"})
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ParseAndVerify[map[string]any](wrongPriv, parsedHeader, token, nil)
	if err != ErrInvalidSignature {
		t.Fatalf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestSign_UnsupportedKeyType(t *testing.T) {
	header := Header{Typ: JWT, Alg: EdDSA}
	_, err := Sign("not-a-key", &header, map[string]any{})
	if err != ErrUnsupportedKeyType {
		t.Fatalf("expected ErrUnsupportedKeyType, got %v", err)
	}
}

func TestSign_WrongAlgorithm(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	header := Header{Typ: JWT, Alg: HS256}
	_, err = Sign(priv, &header, map[string]any{})
	if err != ErrInvalidAlgorithm {
		t.Fatalf("expected ErrInvalidAlgorithm, got %v", err)
	}
}

func TestVerify_UnsupportedKeyType(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseAndVerify[any]("not-a-key", parsedHeader, token, nil)
	if err != ErrUnsupportedKeyType {
		t.Fatalf("expected ErrUnsupportedKeyType, got %v", err)
	}
}

func TestParseAndVerify_RSAPSS(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	tests := []Algorithm{PS256, PS384, PS512}
	for _, alg := range tests {
		header := Header{Typ: JWT, Alg: alg}
		token, err := Sign(priv, &header, map[string]any{"sub": "pss"})
		if err != nil {
			t.Fatalf("Sign(%s): %v", alg, err)
		}

		parsedHeader, err := ParseHeader(token)
		if err != nil {
			t.Fatal(err)
		}

		_, err = ParseAndVerify[map[string]any](&priv.PublicKey, parsedHeader, token, nil)
		if err != nil {
			t.Fatalf("Verify(%s): %v", alg, err)
		}
	}
}

func TestAlgorithm_Helpers(t *testing.T) {
	if !HS256.isHMAC() {
		t.Fatal("HS256 should be HMAC")
	}
	if !HS384.isHMAC() {
		t.Fatal("HS384 should be HMAC")
	}
	if !HS512.isHMAC() {
		t.Fatal("HS512 should be HMAC")
	}
	if !RS256.isRSA() {
		t.Fatal("RS256 should be RSA")
	}
	if !RS384.isRSA() {
		t.Fatal("RS384 should be RSA")
	}
	if !RS512.isRSA() {
		t.Fatal("RS512 should be RSA")
	}
	if !PS256.isRSA() {
		t.Fatal("PS256 should be RSA")
	}
	if !PS384.isRSA() {
		t.Fatal("PS384 should be RSA")
	}
	if !PS512.isRSA() {
		t.Fatal("PS512 should be RSA")
	}
	if !ES256.isECDSA() {
		t.Fatal("ES256 should be ECDSA")
	}
	if !ES384.isECDSA() {
		t.Fatal("ES384 should be ECDSA")
	}
	if !ES512.isECDSA() {
		t.Fatal("ES512 should be ECDSA")
	}
	if EdDSA.isHMAC() || EdDSA.isRSA() || EdDSA.isECDSA() {
		t.Fatal("EdDSA should not match any category")
	}
}

func TestParseHeader_MissingTyp(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	header := Header{Alg: EdDSA}
	token, err := Sign(priv, &header, map[string]any{"sub": "test"})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseHeader(token)
	if err != nil {
		t.Fatalf("expected missing typ to be accepted, got %v", err)
	}
	if parsed.Alg != EdDSA {
		t.Fatalf("expected EdDSA, got %s", parsed.Alg)
	}
}

func TestParseHeader_NonJWTTyp(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	header := Header{Typ: "JOSE", Alg: EdDSA}
	token, err := Sign(priv, &header, map[string]any{"sub": "test"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseHeader(token)
	if err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken for non-JWT typ, got %v", err)
	}
}

func TestAudience_ArrayString(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	claims := map[string]any{"aud": []string{"api-a", "api-b"}}
	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, claims)
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	opts := VerifyOptions{Aud: []string{"api-b"}}
	_, err = ParseAndVerify[map[string]any](priv, parsedHeader, token, &opts)
	if err != nil {
		t.Fatalf("expected aud array to match, got %v", err)
	}
}

func TestAudience_ArrayStringNoMatch(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	claims := map[string]any{"aud": []string{"api-x", "api-y"}}
	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, claims)
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	opts := VerifyOptions{Aud: []string{"api-z"}}
	_, err = ParseAndVerify[map[string]any](priv, parsedHeader, token, &opts)
	if err != ErrInvalidAudience {
		t.Fatalf("expected ErrInvalidAudience, got %v", err)
	}
}

func TestIssuer_NonString(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	claims := map[string]any{"iss": 12345}
	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, claims)
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	opts := VerifyOptions{Iss: []string{"expected-iss"}}
	_, err = ParseAndVerify[map[string]any](priv, parsedHeader, token, &opts)
	if err != ErrInvalidIssuer {
		t.Fatalf("expected ErrInvalidIssuer for non-string iss, got %v", err)
	}
}

func TestTimestamp_NonIntegerExp(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	claims := map[string]any{"exp": 123456.789}
	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, claims)
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	opts := VerifyOptions{Exp: true}
	_, err = ParseAndVerify[map[string]any](priv, parsedHeader, token, &opts)
	if err != ErrMissingField {
		t.Fatalf("expected ErrMissingField for non-integer exp, got %v", err)
	}
}

func TestSignAndVerify_TimeDrift(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	opts := VerifyOptions{
		Exp:              true,
		AllowedTimeDrift: 10 * time.Second,
	}

	withinDrift := time.Now().Add(-5 * time.Second).Unix()

	claims := map[string]any{"exp": withinDrift}
	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, claims)
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ParseAndVerify[map[string]any](priv, parsedHeader, token, &opts)
	if err != nil {
		t.Fatalf("expected token within drift to be valid, got %v", err)
	}
}

func TestVerify_EdDSAPublicKeyWrongAlgorithm(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, map[string]any{"sub": "test"})
	if err != nil {
		t.Fatal(err)
	}
	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}
	parsedHeader.Alg = HS256
	_, err = ParseAndVerify[map[string]any](priv, parsedHeader, token, nil)
	if err != ErrInvalidAlgorithm {
		t.Fatalf("expected ErrInvalidAlgorithm, got %v", err)
	}
}

func TestVerify_ECDSAWrongAlgorithm(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub := &priv.PublicKey

	header := Header{Typ: JWT, Alg: ES256}
	token, err := Sign(priv, &header, map[string]any{"sub": "test"})
	if err != nil {
		t.Fatal(err)
	}
	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}
	parsedHeader.Alg = ES384
	_, err = ParseAndVerify[map[string]any](pub, parsedHeader, token, nil)
	if err != ErrInvalidAlgorithm {
		t.Fatalf("expected ErrInvalidAlgorithm for ECDSA wrong alg, got %v", err)
	}
}

func TestVerify_RSAWrongAlgorithm(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pub := &priv.PublicKey

	header := Header{Typ: JWT, Alg: RS256}
	token, err := Sign(priv, &header, map[string]any{"sub": "test"})
	if err != nil {
		t.Fatal(err)
	}
	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}
	parsedHeader.Alg = EdDSA
	_, err = ParseAndVerify[map[string]any](pub, parsedHeader, token, nil)
	if err != ErrInvalidAlgorithm {
		t.Fatalf("expected ErrInvalidAlgorithm for RSA wrong alg, got %v", err)
	}
}

func TestAudience_NonStringNonArray(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	claims := map[string]any{"aud": 123}
	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, claims)
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	opts := VerifyOptions{Aud: []string{"api"}}
	_, err = ParseAndVerify[map[string]any](priv, parsedHeader, token, &opts)
	if err != ErrInvalidAudience {
		t.Fatalf("expected ErrInvalidAudience for non-string non-array aud, got %v", err)
	}
}

func TestParseAndVerify_NilOpts(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, map[string]any{"sub": "test"})
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseAndVerify[map[string]any](priv, parsedHeader, token, nil)
	if err != nil {
		t.Fatalf("expected no error with nil opts, got %v", err)
	}
}

func TestParseAndVerify_NoValidationFlags(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, map[string]any{"sub": "test"})
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	opts := VerifyOptions{}
	_, err = ParseAndVerify[map[string]any](priv, parsedHeader, token, &opts)
	if err != nil {
		t.Fatalf("expected no error with empty opts (no validation flags), got %v", err)
	}
}

func TestVerify_UnsupportedKeyInSwitch(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, map[string]any{"sub": "test"})
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	type customKey struct{}
	var k customKey
	_, err = ParseAndVerify[any](k, parsedHeader, token, nil)
	if err != ErrUnsupportedKeyType {
		t.Fatalf("expected ErrUnsupportedKeyType, got %v", err)
	}
}

func TestSignAndVerify_NoOptsStructClaims(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	claims := RegisteredClaims{
		SUB: "user1",
		ISS: "my-app",
	}

	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, claims)
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	result, err := ParseAndVerify[RegisteredClaims](priv, parsedHeader, token, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.SUB != "user1" {
		t.Fatalf("expected user1, got %s", result.SUB)
	}
}

func TestParseAndVerify_RegisteredClaimsWithIAT(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	claims := RegisteredClaims{
		SUB: "user1",
		ISS: "my-app",
		AUD: []string{"my-api"},
		EXP: time.Now().Add(time.Hour).Unix(),
		IAT: time.Now().Unix(),
	}

	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, claims)
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	opts := VerifyOptions{
		Exp:              true,
		Iat:              true,
		Aud:              []string{"my-api"},
		Iss:              []string{"my-app"},
		AllowedTimeDrift: time.Minute,
	}

	result, err := ParseAndVerify[RegisteredClaims](priv, parsedHeader, token, &opts)
	if err != nil {
		t.Fatal(err)
	}
	if result.SUB != "user1" {
		t.Fatalf("expected user1, got %s", result.SUB)
	}
}

func TestSignAndVerify_IssuedInFuture(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	claims := map[string]any{"iat": time.Now().Add(time.Hour).Unix()}

	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, claims)
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	opts := VerifyOptions{Iat: true}
	_, err = ParseAndVerify[map[string]any](priv, parsedHeader, token, &opts)
	if err != ErrTokenIssuedInFuture {
		t.Fatalf("expected ErrTokenIssuedInFuture, got %v", err)
	}
}

func TestSignAndVerify_IssuedAtWithDrift(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	opts := VerifyOptions{
		Iat:              true,
		AllowedTimeDrift: 10 * time.Second,
	}

	withinDrift := time.Now().Add(5 * time.Second).Unix()

	claims := map[string]any{"iat": withinDrift}
	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, claims)
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ParseAndVerify[map[string]any](priv, parsedHeader, token, &opts)
	if err != nil {
		t.Fatalf("expected token within drift to be valid, got %v", err)
	}
}

func TestSignAndVerify_MissingIat(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	claims := map[string]any{"sub": "test"}

	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, claims)
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	opts := VerifyOptions{Iat: true}
	_, err = ParseAndVerify[map[string]any](priv, parsedHeader, token, &opts)
	if err != ErrMissingField {
		t.Fatalf("expected ErrMissingField for missing iat, got %v", err)
	}
}

func TestTimestamp_NonIntegerIat(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	claims := map[string]any{"iat": 123456.789}
	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, claims)
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	opts := VerifyOptions{Iat: true}
	_, err = ParseAndVerify[map[string]any](priv, parsedHeader, token, &opts)
	if err != ErrMissingField {
		t.Fatalf("expected ErrMissingField for non-integer iat, got %v", err)
	}
}

func TestSign_WrongAlgorithmECDSA(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	header := Header{Typ: JWT, Alg: HS256}
	_, err = Sign(priv, &header, map[string]any{})
	if err != ErrInvalidAlgorithm {
		t.Fatalf("expected ErrInvalidAlgorithm for ECDSA with HMAC alg, got %v", err)
	}
}

func TestSign_WrongAlgorithmRSA(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	header := Header{Typ: JWT, Alg: HS256}
	_, err = Sign(priv, &header, map[string]any{})
	if err != ErrInvalidAlgorithm {
		t.Fatalf("expected ErrInvalidAlgorithm for RSA with HMAC alg, got %v", err)
	}
}

func TestHashFunc_InvalidAlgorithm(t *testing.T) {
	_, err := hashFunc("INVALID")
	if err != ErrInvalidAlgorithm {
		t.Fatalf("expected ErrInvalidAlgorithm from hashFunc, got %v", err)
	}
}

func TestHashFuncCrypto_InvalidAlgorithm(t *testing.T) {
	_, err := hashFuncCrypto("INVALID")
	if err != ErrInvalidAlgorithm {
		t.Fatalf("expected ErrInvalidAlgorithm from hashFuncCrypto, got %v", err)
	}
}

func TestHmacVerify_InvalidAlgorithm(t *testing.T) {
	err := hmacVerify([]byte("message"), make([]byte, 32), []byte("sig"), "INVALID")
	if err != ErrInvalidAlgorithm {
		t.Fatalf("expected ErrInvalidAlgorithm from hmacVerify, got %v", err)
	}
}

func TestEcdsaVerify_InvalidAlgorithm(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	err = ecdsaVerify(&priv.PublicKey, []byte("msg"), []byte("sig"), "INVALID")
	if err != ErrInvalidAlgorithm {
		t.Fatalf("expected ErrInvalidAlgorithm from ecdsaVerify, got %v", err)
	}
}

func TestRsaVerify_InvalidAlgorithm(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	err = rsaVerify(&priv.PublicKey, []byte("msg"), []byte("sig"), "INVALID")
	if err != ErrInvalidAlgorithm {
		t.Fatalf("expected ErrInvalidAlgorithm from rsaVerify, got %v", err)
	}
}

func TestTimestampFromJson_NonFloat(t *testing.T) {
	// Test int64 path
	_, err := timestampFromJson(int64(123456))
	if err != nil {
		t.Fatalf("unexpected error for int64: %v", err)
	}

	// Test uint64 path
	_, err = timestampFromJson(uint64(123456))
	if err != nil {
		t.Fatalf("unexpected error for uint64: %v", err)
	}

	// Test int path
	_, err = timestampFromJson(123456)
	if err != nil {
		t.Fatalf("unexpected error for int: %v", err)
	}

	// Test non-numeric
	_, err = timestampFromJson("not-a-number")
	if err != ErrMissingField {
		t.Fatalf("expected ErrMissingField for non-numeric, got %v", err)
	}
}

func TestVerify_ECDSAWrongSignature(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	header := Header{Typ: JWT, Alg: ES256}
	token, err := Sign(priv, &header, map[string]any{"sub": "test"})
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	otherPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherPub := &otherPriv.PublicKey

	_, err = ParseAndVerify[map[string]any](otherPub, parsedHeader, token, nil)
	if err != ErrInvalidSignature {
		t.Fatalf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestVerify_HMACWrongSignature(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	wrongKey := make([]byte, 32)
	rand.Read(wrongKey)

	header := Header{Typ: JWT, Alg: HS256}
	token, err := Sign(key, &header, map[string]any{"sub": "test"})
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ParseAndVerify[map[string]any](wrongKey, parsedHeader, token, nil)
	if err != ErrInvalidSignature {
		t.Fatalf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestVerify_HMACWrongAlgorithm(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	header := Header{Typ: JWT, Alg: HS256}
	token, err := Sign(key, &header, map[string]any{"sub": "test"})
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}
	parsedHeader.Alg = EdDSA

	_, err = ParseAndVerify[map[string]any](key, parsedHeader, token, nil)
	if err != ErrInvalidAlgorithm {
		t.Fatalf("expected ErrInvalidAlgorithm, got %v", err)
	}
}

func TestVerify_Ed25519WrongAlgorithm(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, map[string]any{"sub": "test"})
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}
	parsedHeader.Alg = HS256

	_, err = ParseAndVerify[map[string]any](priv, parsedHeader, token, nil)
	if err != ErrInvalidAlgorithm {
		t.Fatalf("expected ErrInvalidAlgorithm, got %v", err)
	}
}

func TestParseAndVerify_RegisteredClaimsWithValidation(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	claims := RegisteredClaims{
		SUB: "user1",
		ISS: "my-app",
		AUD: []string{"my-api"},
		EXP: time.Now().Add(time.Hour).Unix(),
	}

	header := Header{Typ: JWT, Alg: EdDSA}
	token, err := Sign(priv, &header, claims)
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	opts := VerifyOptions{
		Exp: true,
		Aud: []string{"my-api"},
		Iss: []string{"my-app"},
	}

	result, err := ParseAndVerify[RegisteredClaims](priv, parsedHeader, token, &opts)
	if err != nil {
		t.Fatal(err)
	}
	if result.SUB != "user1" {
		t.Fatalf("expected user1, got %s", result.SUB)
	}
}

func TestVerify_RSAPrivateKeyWrongAlgorithm(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	header := Header{Typ: JWT, Alg: RS256}
	token, err := Sign(priv, &header, map[string]any{"sub": "test"})
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}
	parsedHeader.Alg = ES256

	_, err = ParseAndVerify[map[string]any](priv, parsedHeader, token, nil)
	if err != ErrInvalidAlgorithm {
		t.Fatalf("expected ErrInvalidAlgorithm for RSA priv key wrong alg, got %v", err)
	}
}

func TestIsSupported(t *testing.T) {
	all := []Algorithm{HS256, HS384, HS512, EdDSA, ES256, ES384, ES512, RS256, RS384, RS512, PS256, PS384, PS512, MLDSA44, MLDSA65, MLDSA87}
	for _, a := range all {
		if !a.isSupported() {
			t.Fatalf("expected %s to be supported", a)
		}
	}
	if Algorithm("none").isSupported() {
		t.Fatal("expected 'none' to be unsupported")
	}
	if Algorithm("unsupported").isSupported() {
		t.Fatal("expected 'unsupported' to be unsupported")
	}
}

func TestParseHeader_UnsupportedAlgorithm(t *testing.T) {
	tests := []struct {
		name string
		alg  string
	}{
		{"none algorithm", `{"alg":"none","typ":"JWT"}`},
		{"random algorithm", `{"alg":"UNSUPPORTED","typ":"JWT"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headerB64 := base64.RawURLEncoding.EncodeToString([]byte(tt.alg))
			token := headerB64 + ".aW52YWxpZFBheWxvYWQ.aW52YWxpZFNpZ25hdHVyZQ"
			_, err := ParseHeader(token)
			if err != ErrUnsupportedAlgorithm {
				t.Fatalf("expected ErrUnsupportedAlgorithm, got %v", err)
			}
		})
	}
}

func TestParseHeader_SupportedAlgorithm(t *testing.T) {
	for _, alg := range []Algorithm{HS256, ES256, RS256, EdDSA, PS256} {
		header := fmt.Sprintf(`{"alg":"%s","typ":"JWT"}`, alg)
		headerB64 := base64.RawURLEncoding.EncodeToString([]byte(header))
		token := headerB64 + ".aW52YWxpZFBheWxvYWQ.aW52YWxpZFNpZ25hdHVyZQ"
		parsed, err := ParseHeader(token)
		if err != nil {
			t.Fatalf("expected no error for %s, got %v", alg, err)
		}
		if parsed.Alg != alg {
			t.Fatalf("expected alg %s, got %s", alg, parsed.Alg)
		}
	}
}

// ── Malformed token / edge case tests ───────────────────────

func TestParseHeader_MalformedTokens(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{"empty string", ""},
		{"single dot", "."},
		{"only dots", "..."},
		{"no dots", "noDotsHere"},
		{"leading dot", ".a.b"},
		{"two parts only", "a.b"},
		{"trailing dot after claims", "a.b."},
		{"trailing double dot after header", "a.."},
		{"truncated after first dot", "a."},
		{"empty header", ".b.c"},
		{"empty claims", "a..c"},
		{"consecutive dots between header and claims", "ab...c"},
		{"valid base64, invalid json", "ISEh.YQ.YQ"},
		{"valid json, missing alg", "eyJ0eXAiOiJKV1QifQ.YQ.YQ"},
		{"valid json, alg is empty string", "eyJhbGciOiIifQ.YQ.YQ"},
		{"header base64 not 4-char aligned", "ab.cd.ef"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseHeader(tt.token)
			if err != ErrInvalidToken {
				t.Fatalf("expected ErrInvalidToken, got %v", err)
			}
		})
	}
}

func TestParseHeader_FourParts(t *testing.T) {
	_, err := ParseHeader("a.b.c.d")
	if err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken for extra dot, got %v", err)
	}
}

func TestParseHeader_InvalidBase64(t *testing.T) {
	_, err := ParseHeader("!!!not-base64!!.YQ.YQ")
	if err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken for invalid header base64, got %v", err)
	}
}

func TestParseAndVerify_MismatchedHeaderToken(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	header := Header{Typ: JWT, Alg: EdDSA}
	longToken, err := Sign(priv, &header, map[string]any{"sub": "test"})
	if err != nil {
		t.Fatal(err)
	}

	longParsed, err := ParseHeader(longToken)
	if err != nil {
		t.Fatal(err)
	}

	if longParsed.firstDotIndex <= 0 || longParsed.secondDotIndex <= 0 {
		t.Fatal("expected non-zero dot indices")
	}

	// use the long token's header with a shorter token — should not panic
	_, err = ParseAndVerify[any](priv, longParsed, "X.Y", nil)
	if err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken for mismatched header/token, got %v", err)
	}
}

func TestParseAndVerify_MismatchedHeaderTokenBoundary(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	header := Header{Typ: JWT, Alg: EdDSA}
	longToken, err := Sign(priv, &header, map[string]any{"sub": "test"})
	if err != nil {
		t.Fatal(err)
	}

	longParsed, err := ParseHeader(longToken)
	if err != nil {
		t.Fatal(err)
	}

	short := longToken[:longParsed.secondDotIndex]
	_, err = ParseAndVerify[any](priv, longParsed, short, nil)
	if err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken when token truncated at secondDotIndex, got %v", err)
	}
}

// ── ML-DSA tests ────────────────────────────────────────────

func TestSignAndVerify_MLDSA44(t *testing.T) {
	priv, err := mldsa.GenerateKey(mldsa.MLDSA44())
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.PublicKey()

	header := Header{Typ: JWT, Alg: MLDSA44, KID: "mldsa44-key"}
	claims := map[string]any{"sub": "user123"}

	token, err := Sign(priv, &header, claims)
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}
	if parsedHeader.Alg != MLDSA44 {
		t.Fatalf("expected MLDSA44, got %s", parsedHeader.Alg)
	}
	if parsedHeader.KID != "mldsa44-key" {
		t.Fatalf("expected mldsa44-key, got %s", parsedHeader.KID)
	}

	result, err := ParseAndVerify[map[string]any](pub, parsedHeader, token, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result["sub"] != "user123" {
		t.Fatalf("expected user123, got %v", result["sub"])
	}
}

func TestSignAndVerify_MLDSA65(t *testing.T) {
	priv, err := mldsa.GenerateKey(mldsa.MLDSA65())
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.PublicKey()

	header := Header{Typ: JWT, Alg: MLDSA65}
	token, err := Sign(priv, &header, map[string]any{"sub": "test"})
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ParseAndVerify[map[string]any](pub, parsedHeader, token, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSignAndVerify_MLDSA87(t *testing.T) {
	priv, err := mldsa.GenerateKey(mldsa.MLDSA87())
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.PublicKey()

	header := Header{Typ: JWT, Alg: MLDSA87}
	token, err := Sign(priv, &header, map[string]any{"sub": "test"})
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ParseAndVerify[map[string]any](pub, parsedHeader, token, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSignAndVerify_MLDSA_VerifyWithPrivateKey(t *testing.T) {
	priv, err := mldsa.GenerateKey(mldsa.MLDSA44())
	if err != nil {
		t.Fatal(err)
	}

	header := Header{Typ: JWT, Alg: MLDSA44}
	token, err := Sign(priv, &header, map[string]any{"sub": "test"})
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	// Verify using the private key (should extract the public key)
	_, err = ParseAndVerify[map[string]any](priv, parsedHeader, token, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSign_MLDSA_WrongAlgorithm(t *testing.T) {
	priv, err := mldsa.GenerateKey(mldsa.MLDSA44())
	if err != nil {
		t.Fatal(err)
	}

	header := Header{Typ: JWT, Alg: EdDSA}
	_, err = Sign(priv, &header, map[string]any{})
	if err != ErrInvalidAlgorithm {
		t.Fatalf("expected ErrInvalidAlgorithm, got %v", err)
	}
}

func TestVerify_MLDSA_WrongAlgorithm(t *testing.T) {
	priv, err := mldsa.GenerateKey(mldsa.MLDSA44())
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.PublicKey()

	header := Header{Typ: JWT, Alg: MLDSA44}
	token, err := Sign(priv, &header, map[string]any{"sub": "test"})
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader.Alg = EdDSA
	_, err = ParseAndVerify[map[string]any](pub, parsedHeader, token, nil)
	if err != ErrInvalidAlgorithm {
		t.Fatalf("expected ErrInvalidAlgorithm, got %v", err)
	}
}

func TestVerify_MLDSA_WrongKey(t *testing.T) {
	priv, err := mldsa.GenerateKey(mldsa.MLDSA44())
	if err != nil {
		t.Fatal(err)
	}
	wrongPriv, err := mldsa.GenerateKey(mldsa.MLDSA44())
	if err != nil {
		t.Fatal(err)
	}

	header := Header{Typ: JWT, Alg: MLDSA44}
	token, err := Sign(priv, &header, map[string]any{"sub": "test"})
	if err != nil {
		t.Fatal(err)
	}

	parsedHeader, err := ParseHeader(token)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ParseAndVerify[map[string]any](wrongPriv.PublicKey(), parsedHeader, token, nil)
	if err != ErrInvalidSignature {
		t.Fatalf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestParseHeader_MLDSA_Supported(t *testing.T) {
	for _, alg := range []Algorithm{MLDSA44, MLDSA65, MLDSA87} {
		header := fmt.Sprintf(`{"alg":"%s","typ":"JWT"}`, alg)
		headerB64 := base64.RawURLEncoding.EncodeToString([]byte(header))
		token := headerB64 + ".aW52YWxpZFBheWxvYWQ.aW52YWxpZFNpZ25hdHVyZQ"
		parsed, err := ParseHeader(token)
		if err != nil {
			t.Fatalf("expected no error for %s, got %v", alg, err)
		}
		if parsed.Alg != alg {
			t.Fatalf("expected alg %s, got %s", alg, parsed.Alg)
		}
	}
}
