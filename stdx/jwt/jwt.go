// Package jwt provides functions for creating, parsing, and verifying JSON Web Tokens
// (JWT) as defined in RFC 7519, with support for JSON Web Keys (JWK) per RFC 7517.
//
// It supports EdDSA (Ed25519), ECDSA (P-256/P-384/P-521), RSA (PKCS1v15 and PSS),
// and HMAC (SHA-256/384/512) signing and verification.
//
// # Sign, ParseHeader, ParseAndVerify
//
// The following example demonstrates the complete JWT lifecycle: signing a token,
// inspecting its header, and verifying signature and claims.
//
//	import "markdown.ninja/stdx/jwt"
//
//	 // Generate or load a key pair (Ed25519 shown here).
//	 _, priv, _ := ed25519.GenerateKey(rand.Reader)
//	 pub := priv.Public().(ed25519.PublicKey)
//
//	 // Sign a token.
//	 header := jwt.Header{Typ: jwt.JWT, Alg: jwt.EdDSA, KID: "my-key"}
//	 claims := map[string]any{"sub": "user123", "exp": time.Now().Add(time.Hour).Unix()}
//	 token, err := jwt.Sign(priv, &header, claims)
//	 if err != nil { log.Fatal(err) }
//
//	 // ParseHeader extracts the header without verifying (useful for kid lookup).
//	 parsedHeader, err := jwt.ParseHeader(token)
//	 if err != nil { log.Fatal(err) }
//	 fmt.Println("Key ID:", parsedHeader.KID)
//
//	 // ParseAndVerify verifies the signature, validates claims, and decodes claims.
//	 opts := jwt.VerifyOptions{Exp: true, AllowedTimeDrift: time.Minute}
//	 result, err := jwt.ParseAndVerify[map[string]any](pub, &parsedHeader, token, &opts)
//	 if err != nil { log.Fatal(err) }
//	 fmt.Println("Subject:", result["sub"])
//
// # JWK
//
// Encode a Go crypto key to JWK JSON using json.Marshal,
// and parse it back using json.Unmarshal:
//
//	import "markdown.ninja/stdx/jwt"
//
//	 // Marshal a key to RFC 7517 JWK JSON.
//	 _, priv, _ := ed25519.GenerateKey(rand.Reader)
//	 jwk := jwt.JWK{Key: priv, Alg: jwt.EdDSA, ID: "my-key-id"}
//	 jwkJSON, err := json.Marshal(jwk)
//	 if err != nil { log.Fatal(err) }
//	 fmt.Println(string(jwkJSON))
//
//	 // Unmarshal JWK JSON back into a Go crypto key.
//	 var parsed jwt.JWK
//	 if err := json.Unmarshal(jwkJSON, &parsed); err != nil { log.Fatal(err) }
//	 fmt.Printf("Algorithm: %s, Key ID: %s\n", parsed.Alg, parsed.ID)
//	 _ = parsed.Key.(ed25519.PrivateKey)
//
//	 // Parse a JWK Set (JWKS):
//	 var set struct { Keys []jwt.JWK `json:"keys"` }
//	 if err := json.Unmarshal(jwksJSON, &set); err != nil { log.Fatal(err) }
package jwt

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/mldsa"
	cryptoRand "crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"hash"
	"slices"
	"strings"
	"time"
)

var (
	ErrUnsupportedKeyType   = errors.New("jwt: unsupported key type")
	ErrUnsupportedCurve     = errors.New("jwt: unsupported elliptic curve")
	ErrInvalidAlgorithm     = errors.New("jwt: invalid algorithm for key type")
	ErrInvalidJWK           = errors.New("jwt: invalid JWK")
	ErrMissingField         = errors.New("jwt: missing required JWT field")
	ErrInvalidSignature     = errors.New("jwt: invalid signature")
	ErrInvalidToken         = errors.New("jwt: invalid token format")
	ErrTokenExpired         = errors.New("jwt: token is expired")
	ErrTokenNotYetValid     = errors.New("jwt: token is not yet valid")
	ErrTokenIssuedInFuture  = errors.New("jwt: token was issued in the future")
	ErrInvalidAudience      = errors.New("jwt: invalid audience")
	ErrInvalidIssuer        = errors.New("jwt: invalid issuer")
	ErrKeyIsTooShort        = errors.New("jwt: key is too short")
	ErrUnsupportedAlgorithm = errors.New("jwt: unsupported algorithm")
)

// Algorithm identifies the signing/verification algorithm for a JWT.
type Algorithm string

const (
	// HMAC using SHA-256.
	HS256 Algorithm = "HS256"

	// HMAC using SHA-384.
	HS384 Algorithm = "HS384"

	// HMAC using SHA-512.
	HS512 Algorithm = "HS512"

	// Edwards-curve Digital Signature Algorithm (Ed25519).
	EdDSA Algorithm = "EdDSA"

	// ECDSA using P-256 and SHA-256.
	ES256 Algorithm = "ES256"

	// ECDSA using P-384 and SHA-384.
	ES384 Algorithm = "ES384"

	// ECDSA using P-521 and SHA-512.
	ES512 Algorithm = "ES512"

	// RSASSA-PKCS1-v1.5 with SHA-256.
	RS256 Algorithm = "RS256"

	// RSASSA-PKCS1-v1.5 with SHA-384.
	RS384 Algorithm = "RS384"

	// RSASSA-PKCS1-v1.5 with SHA-512.
	RS512 Algorithm = "RS512"

	// RSASSA-PSS with SHA-256.
	PS256 Algorithm = "PS256"

	// RSASSA-PSS with SHA-384.
	PS384 Algorithm = "PS384"

	// RSASSA-PSS with SHA-512.
	PS512 Algorithm = "PS512"

	// ML-DSA (Module-Lattice-Based Digital Signature Standard, FIPS 204).
	MLDSA44 Algorithm = "ML-DSA-44"
	MLDSA65 Algorithm = "ML-DSA-65"
	MLDSA87 Algorithm = "ML-DSA-87"
)

func (a Algorithm) isHMAC() bool {
	switch a {
	case HS256, HS384, HS512:
		return true
	}
	return false
}

func (a Algorithm) isRSA() bool {
	switch a {
	case RS256, RS384, RS512, PS256, PS384, PS512:
		return true
	}
	return false
}

func (a Algorithm) isECDSA() bool {
	switch a {
	case ES256, ES384, ES512:
		return true
	}
	return false
}

func (a Algorithm) isMLDSA() bool {
	switch a {
	case MLDSA44, MLDSA65, MLDSA87:
		return true
	}
	return false
}

func (a Algorithm) isSupported() bool {
	switch a {
	case HS256, HS384, HS512, EdDSA, ES256, ES384, ES512, RS256, RS384, RS512, PS256, PS384, PS512, MLDSA44, MLDSA65, MLDSA87:
		return true
	}
	return false
}

// TokenType represents the type of a JWT token.
type TokenType string

const JWT TokenType = "JWT"

// Header represents a JWT header.
type Header struct {
	Typ     TokenType `json:"typ"`
	Alg     Algorithm `json:"alg"`
	CTY     string    `json:"cty,omitempty"`
	JKU     string    `json:"jku,omitempty"`
	KID     string    `json:"kid,omitempty"`
	X5U     string    `json:"x5u,omitempty"`
	X5C     []string  `json:"x5c,omitempty"`
	X5T     string    `json:"x5t,omitempty"`
	X5TS256 string    `json:"x5t#S256,omitempty"`

	// field populated when parsing header to re-use when validating the token
	firstDotIndex  int `json:"-"`
	secondDotIndex int `json:"-"`
}

// The ClaimsValidator interface is used to avoid multiple unmarshalling when validating registered
// claims.
type ClaimsValidator interface {
	ValidateClaims(opts *VerifyOptions) error
}

// RegisteredClaims holds the standard JWT registered claim names.
// https://www.rfc-editor.org/rfc/rfc7519#section-4.1
type RegisteredClaims struct {
	ISS string   `json:"iss,omitempty"`
	SUB string   `json:"sub,omitempty"`
	AUD []string `json:"aud,omitempty"`
	EXP int64    `json:"exp,omitempty"`
	NBF int64    `json:"nbf,omitempty"`
	IAT int64    `json:"iat,omitempty"`
	JTI string   `json:"jti,omitempty"`
}

func (claims RegisteredClaims) ValidateClaims(opts *VerifyOptions) error {
	if opts == nil {
		return nil
	}

	now := time.Now().Unix()

	if opts.Exp {
		if claims.EXP == 0 {
			return ErrMissingField
		}
		if claims.EXP < now-int64(opts.AllowedTimeDrift.Seconds()) {
			return ErrTokenExpired
		}
	}

	if opts.Nbf {
		if claims.NBF == 0 {
			return ErrMissingField
		}
		if claims.NBF > now+int64(opts.AllowedTimeDrift.Seconds()) {
			return ErrTokenNotYetValid
		}
	}

	if opts.Iat {
		if claims.IAT == 0 {
			return ErrMissingField
		}
		if claims.IAT > now+int64(opts.AllowedTimeDrift.Seconds()) {
			return ErrTokenIssuedInFuture
		}
	}

	if len(opts.Aud) > 0 {
		if len(claims.AUD) == 0 {
			return ErrInvalidAudience
		}

		audFound := false
		for _, validAud := range opts.Aud {
			if slices.Contains(claims.AUD, validAud) {
				audFound = true
				break
			}
		}
		if !audFound {
			return ErrInvalidAudience
		}
	}

	if len(opts.Iss) > 0 {
		if claims.ISS == "" || !slices.Contains(opts.Iss, claims.ISS) {
			return ErrInvalidIssuer
		}
	}

	return nil
}

// VerifyOptions configures claim verification for ParseAndVerify.
type VerifyOptions struct {
	AllowedTimeDrift time.Duration
	Exp              bool
	// Not Before verification
	Nbf bool
	// Issued At verification
	Iat bool
	// Audience verification
	Aud []string
	// Issuer verification
	Iss []string
}

// Sign creates a signed JWT token string.
//
// Supported key types for signing:
//
//	ed25519.PrivateKey  -> EdDSA
//	*ecdsa.PrivateKey   -> ES256/ES384/ES512 (derived from curve)
//	[]byte              -> HS256/HS384/HS512 (must match header alg)
func Sign(keyAny any, header *Header, claims any) (string, error) {
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	buf := make([]byte, 0,
		base64.RawURLEncoding.EncodedLen(len(headerJSON))+
			base64.RawURLEncoding.EncodedLen(len(claimsJSON))+
			2+
			base64.RawURLEncoding.EncodedLen(512),
	)

	buf = base64.RawURLEncoding.AppendEncode(buf, headerJSON)
	buf = append(buf, '.')
	buf = base64.RawURLEncoding.AppendEncode(buf, claimsJSON)

	var signature []byte
	switch key := keyAny.(type) {
	case ed25519.PrivateKey:
		if header.Alg != EdDSA {
			return "", ErrInvalidAlgorithm
		}
		signature = ed25519.Sign(key, buf)

	case *ecdsa.PrivateKey:
		_, expectedAlg, _, err := curveInfo(key.Curve)
		if err != nil {
			return "", err
		}
		if header.Alg != expectedAlg {
			return "", ErrInvalidAlgorithm
		}
		signature, err = ecdsaSign(key, buf, header.Alg)
		if err != nil {
			return "", err
		}

	case *rsa.PrivateKey:
		if !header.Alg.isRSA() {
			return "", ErrInvalidAlgorithm
		}
		signature, err = rsaSign(key, buf, header.Alg)
		if err != nil {
			return "", err
		}

	case []byte:
		if !header.Alg.isHMAC() {
			return "", ErrInvalidAlgorithm
		}
		if len(key) < 32 {
			return "", ErrKeyIsTooShort
		}
		sig, err := hmacSign(buf, key, header.Alg)
		if err != nil {
			return "", err
		}
		signature = sig

	case *mldsa.PrivateKey:
		if !header.Alg.isMLDSA() {
			return "", ErrInvalidAlgorithm
		}
		signature, err = key.SignDeterministic(buf, &mldsa.Options{})
		if err != nil {
			return "", err
		}

	default:
		return "", ErrUnsupportedKeyType
	}

	buf = append(buf, '.')
	buf = base64.RawURLEncoding.AppendEncode(buf, signature)
	return string(buf), nil
}

// ParseHeader extracts and decodes the header from a JWT token without verifying.
// Use this to inspect the kid field before looking up the verification key.
func ParseHeader(token string) (Header, error) {
	firstDotIndex := strings.IndexByte(token, '.')
	if firstDotIndex <= 0 {
		return Header{}, ErrInvalidToken
	}

	secondDotIndex := strings.IndexByte(token[firstDotIndex+1:], '.')
	if secondDotIndex <= 0 {
		return Header{}, ErrInvalidToken
	}
	secondDotIndex += firstDotIndex + 1

	extraDotIndex := strings.IndexByte(token[secondDotIndex+1:], '.')
	if extraDotIndex >= 0 {
		return Header{}, ErrInvalidToken
	}

	headerBase64 := token[:firstDotIndex]
	headerJSON, err := base64.RawURLEncoding.DecodeString(headerBase64)
	if err != nil {
		return Header{}, ErrInvalidToken
	}

	var header Header
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return Header{}, ErrInvalidToken
	}

	if header.Alg == "" || (header.Typ != "" && header.Typ != JWT) {
		return Header{}, ErrInvalidToken
	}

	if !header.Alg.isSupported() {
		return Header{}, ErrUnsupportedAlgorithm
	}

	header.firstDotIndex = firstDotIndex
	header.secondDotIndex = secondDotIndex
	return header, nil
}

// ParseAndVerify verifies the JWT signature, validates claims per opts,
// and unmarshals the claims into dst (which must be a pointer).
//
// Supported key types for verification:
//
//	ed25519.PrivateKey / ed25519.PublicKey  -> EdDSA
//	*ecdsa.PrivateKey / *ecdsa.PublicKey    -> ES256/ES384/ES512
//	*rsa.PublicKey                          -> RS*/PS*
//	[]byte                                  -> HS*
func ParseAndVerify[C any](key any, header Header, token string, opts *VerifyOptions) (claims C, err error) {
	// ensure that the given header corresponds to the token
	if header.firstDotIndex == 0 || header.secondDotIndex == 0 || header.firstDotIndex >= header.secondDotIndex ||
		header.secondDotIndex >= len(token) || token[header.firstDotIndex] != '.' || token[header.secondDotIndex] != '.' {
		return claims, ErrInvalidToken
	}

	claimsBase64 := token[header.firstDotIndex+1 : header.secondDotIndex]
	signatureBase64 := token[header.secondDotIndex+1:]
	signedMessage := token[:header.secondDotIndex]

	rawSignature, err := base64.RawURLEncoding.DecodeString(signatureBase64)
	if err != nil {
		return claims, ErrInvalidToken
	}

	if err = verify(key, header.Alg, []byte(signedMessage), rawSignature); err != nil {
		return claims, err
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(claimsBase64)
	if err != nil {
		return claims, ErrInvalidToken
	}

	if opts != nil &&
		(opts.Exp || opts.Nbf || opts.Iat || len(opts.Aud) > 0 || len(opts.Iss) > 0) {
		if err = json.Unmarshal(claimsJSON, &claims); err != nil {
			return claims, ErrInvalidToken
		}

		// fast-path for when claims implement the ClaimsValidator interface
		// (e.g. when embedding RegisteredClaims)
		if claimsValidator, ok := any(claims).(ClaimsValidator); ok {
			err = claimsValidator.ValidateClaims(opts)
			return claims, err
		}

		// fast-path for when the input claim is a map[string]any
		if claimsMap, ok := any(claims).(map[string]any); ok {
			err = validateClaimsMap(claimsMap, opts)
			return claims, err
		}

		// slow-path for custom structures where we need to unmarshal 2 times
		var registeredClaims RegisteredClaims
		err = json.Unmarshal(claimsJSON, &registeredClaims)
		if err != nil {
			return claims, err
		}
		err = registeredClaims.ValidateClaims(opts)
		return claims, err
	}

	err = json.Unmarshal(claimsJSON, &claims)
	return claims, err
}

// ── signing / verification helpers ──────────────────────────

func hmacSign(message, key []byte, alg Algorithm) ([]byte, error) {
	h, err := hashFunc(alg)
	if err != nil {
		return nil, err
	}
	mac := hmac.New(h, key)
	mac.Write(message)
	return mac.Sum(nil), nil
}

func hmacVerify(message, key, sig []byte, alg Algorithm) error {
	expected, err := hmacSign(message, key, alg)
	if err != nil {
		return err
	}
	if !hmac.Equal(sig, expected) {
		return ErrInvalidSignature
	}
	return nil
}

func ecdsaSign(key *ecdsa.PrivateKey, message []byte, alg Algorithm) ([]byte, error) {
	h, err := hashFunc(alg)
	if err != nil {
		return nil, err
	}
	hash := h()
	hash.Write(message)
	digest := hash.Sum(nil)
	return ecdsa.SignASN1(cryptoRand.Reader, key, digest)
}

func rsaSign(key *rsa.PrivateKey, message []byte, alg Algorithm) ([]byte, error) {
	h, err := hashFunc(alg)
	if err != nil {
		return nil, err
	}
	hash := h()
	hash.Write(message)
	digest := hash.Sum(nil)

	cryptoHash, err := hashFuncCrypto(alg)
	if err != nil {
		return nil, err
	}

	switch alg {
	case RS256, RS384, RS512:
		return rsa.SignPKCS1v15(nil, key, cryptoHash, digest)
	case PS256, PS384, PS512:
		return rsa.SignPSS(cryptoRand.Reader, key, cryptoHash, digest, nil)
	}
	return nil, ErrInvalidAlgorithm
}

func ecdsaVerify(pub *ecdsa.PublicKey, message, sig []byte, alg Algorithm) error {
	h, err := hashFunc(alg)
	if err != nil {
		return err
	}
	hash := h()
	hash.Write(message)
	digest := hash.Sum(nil)
	if !ecdsa.VerifyASN1(pub, digest, sig) {
		return ErrInvalidSignature
	}
	return nil
}

func rsaVerify(pub *rsa.PublicKey, message, sig []byte, alg Algorithm) error {
	h, err := hashFunc(alg)
	if err != nil {
		return err
	}
	hash := h()
	hash.Write(message)
	digest := hash.Sum(nil)

	cryptoHash, err := hashFuncCrypto(alg)
	if err != nil {
		return err
	}

	switch alg {
	case RS256, RS384, RS512:
		return rsa.VerifyPKCS1v15(pub, cryptoHash, digest, sig)
	case PS256, PS384, PS512:
		return rsa.VerifyPSS(pub, cryptoHash, digest, sig, nil)
	}
	return ErrInvalidAlgorithm
}

func hashFunc(alg Algorithm) (func() hash.Hash, error) {
	switch alg {
	case HS256, ES256, RS256, PS256:
		return sha256.New, nil
	case HS384, ES384, RS384, PS384:
		return sha512.New384, nil
	case HS512, ES512, RS512, PS512:
		return sha512.New, nil
	}
	return nil, ErrInvalidAlgorithm
}

func hashFuncCrypto(alg Algorithm) (crypto.Hash, error) {
	switch alg {
	case RS256, PS256:
		return crypto.SHA256, nil
	case RS384, PS384:
		return crypto.SHA384, nil
	case RS512, PS512:
		return crypto.SHA512, nil
	}
	return 0, ErrInvalidAlgorithm
}

func verify(keyAny any, alg Algorithm, message, sig []byte) error {
	switch key := keyAny.(type) {
	case ed25519.PrivateKey:
		if alg != EdDSA {
			return ErrInvalidAlgorithm
		}
		if !ed25519.Verify(key.Public().(ed25519.PublicKey), message, sig) {
			return ErrInvalidSignature
		}
		return nil

	case ed25519.PublicKey:
		if alg != EdDSA {
			return ErrInvalidAlgorithm
		}
		if !ed25519.Verify(key, message, sig) {
			return ErrInvalidSignature
		}
		return nil

	case *ecdsa.PrivateKey:
		_, expectedAlg, _, err := curveInfo(key.Curve)
		if err != nil {
			return err
		}
		if alg != expectedAlg {
			return ErrInvalidAlgorithm
		}
		return ecdsaVerify(&key.PublicKey, message, sig, alg)

	case *ecdsa.PublicKey:
		_, expectedAlg, _, err := curveInfo(key.Curve)
		if err != nil {
			return err
		}
		if alg != expectedAlg {
			return ErrInvalidAlgorithm
		}
		return ecdsaVerify(key, message, sig, alg)

	case *rsa.PublicKey:
		if !alg.isRSA() {
			return ErrInvalidAlgorithm
		}
		return rsaVerify(key, message, sig, alg)

	case *rsa.PrivateKey:
		if !alg.isRSA() {
			return ErrInvalidAlgorithm
		}
		return rsaVerify(&key.PublicKey, message, sig, alg)

	case *mldsa.PrivateKey:
		if !alg.isMLDSA() {
			return ErrInvalidAlgorithm
		}
		if err := mldsa.Verify(key.PublicKey(), message, sig, &mldsa.Options{}); err != nil {
			return ErrInvalidSignature
		}
		return nil

	case *mldsa.PublicKey:
		if !alg.isMLDSA() {
			return ErrInvalidAlgorithm
		}
		if err := mldsa.Verify(key, message, sig, &mldsa.Options{}); err != nil {
			return ErrInvalidSignature
		}
		return nil

	case []byte:
		if !alg.isHMAC() {
			return ErrInvalidAlgorithm
		}

		return hmacVerify(message, key, sig, alg)

	default:
		return ErrUnsupportedKeyType
	}
}

func validateClaims(claimsJSON []byte, opts *VerifyOptions) error {
	raw := make(map[string]any, 4)
	if err := json.Unmarshal(claimsJSON, &raw); err != nil {
		return nil
	}

	return validateClaimsMap(raw, opts)
}

func validateClaimsMap(claims map[string]any, opts *VerifyOptions) error {
	var registeredClaims RegisteredClaims
	var err error

	if expRaw, ok := claims["exp"]; ok {
		registeredClaims.EXP, err = timestampFromJson(expRaw)
		if err != nil {
			return err
		}
	}

	if nbfRaw, ok := claims["nbf"]; ok {
		registeredClaims.NBF, err = timestampFromJson(nbfRaw)
		if err != nil {
			return err
		}
	}

	if iatRaw, ok := claims["iat"]; ok {
		registeredClaims.IAT, err = timestampFromJson(iatRaw)
		if err != nil {
			return err
		}
	}

	if issRaw, ok := claims["iss"]; ok {
		registeredClaims.ISS, ok = issRaw.(string)
		if !ok {
			return ErrInvalidIssuer
		}
	}

	if audRaw, ok := claims["aud"]; ok {
		switch audValue := audRaw.(type) {
		case []any:
			audStrings := make([]string, 0, len(audValue))
			for _, aud := range audValue {
				if audString, ok := aud.(string); ok {
					audStrings = append(audStrings, audString)
				} else {
					return ErrInvalidAudience
				}
			}
			registeredClaims.AUD = audStrings
		case string:
			registeredClaims.AUD = []string{audValue}
		default:
			return ErrInvalidAudience
		}
	}

	return registeredClaims.ValidateClaims(opts)
}

func timestampFromJson(v any) (int64, error) {
	switch n := v.(type) {
	case float64:
		if float64(int64(n)) != n {
			return 0, ErrMissingField
		}
		return int64(n), nil
	case int64:
		return n, nil
	case uint64:
		return int64(n), nil
	case int:
		return int64(n), nil
	}
	return 0, ErrMissingField
}
