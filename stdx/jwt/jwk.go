package jwt

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/mldsa"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"math/big"
)

// JWK holds the decoded key and its JWK metadata.
// Returned by value to minimize allocations.
type JWK struct {
	// Key ID
	ID  string
	Alg Algorithm
	Key any
}

type JWKS struct {
	Keys []JWK `json:"keys"`
}

type jwkFields struct {
	KTY  string `json:"kty"`
	USE  string `json:"use,omitempty"`
	CRV  string `json:"crv,omitempty"`
	ALG  string `json:"alg,omitempty"`
	KID  string `json:"kid,omitempty"`
	X    string `json:"x,omitempty"`
	Y    string `json:"y,omitempty"`
	D    string `json:"d,omitempty"`
	N    string `json:"n,omitempty"`
	E    string `json:"e,omitempty"`
	P    string `json:"p,omitempty"`
	Q    string `json:"q,omitempty"`
	DP   string `json:"dp,omitempty"`
	DQ   string `json:"dq,omitempty"`
	QI   string `json:"qi,omitempty"`
	K    string `json:"k,omitempty"`
	PUB  string `json:"pub,omitempty"`
	PRIV string `json:"priv,omitempty"`
}

// MarshalJSON implements json.Marshaler for JWK.
// It serializes the key to RFC 7517 JSON.
func (jwk JWK) MarshalJSON() ([]byte, error) {
	capacity := 1600 // appropriate size for ML-DSA-44 keys
	switch key := jwk.Key.(type) {
	case *mldsa.PublicKey:
		capacity = key.Parameters().PublicKeySize() * 4 / 3
	case *mldsa.PrivateKey:
		capacity = key.PublicKey().Parameters().PublicKeySize() * 4 / 3
	}
	w := newJwkWriter(capacity)
	w.writeByte('{')

	switch key := jwk.Key.(type) {
	case ed25519.PrivateKey:
		if len(key) != ed25519.PrivateKeySize {
			return nil, ErrInvalidJWK
		}
		w.writeString("kty", "OKP")
		w.writeString("crv", "Ed25519")
		w.writeString("alg", string(EdDSA))
		w.writeBase64("x", key[ed25519.SeedSize:])
		w.writeBase64("d", key[:ed25519.SeedSize])

	case ed25519.PublicKey:
		if len(key) != ed25519.PublicKeySize {
			return nil, ErrInvalidJWK
		}
		w.writeString("kty", "OKP")
		w.writeString("crv", "Ed25519")
		w.writeString("alg", string(EdDSA))
		w.writeBase64("x", key)

	case *ecdsa.PrivateKey:
		crv, ecAlg, coordLen, err := curveInfo(key.Curve)
		if err != nil {
			return nil, err
		}
		w.writeString("kty", "EC")
		w.writeString("crv", crv)
		w.writeString("alg", string(ecAlg))
		w.writeBase64("x", bigIntFixed(key.X, coordLen))
		w.writeBase64("y", bigIntFixed(key.Y, coordLen))
		w.writeBase64("d", bigIntFixed(key.D, coordLen))

	case *ecdsa.PublicKey:
		crv, ecAlg, coordLen, err := curveInfo(key.Curve)
		if err != nil {
			return nil, err
		}
		w.writeString("kty", "EC")
		w.writeString("crv", crv)
		w.writeString("alg", string(ecAlg))
		w.writeBase64("x", bigIntFixed(key.X, coordLen))
		w.writeBase64("y", bigIntFixed(key.Y, coordLen))

	case *rsa.PrivateKey:
		if !jwk.Alg.isRSA() {
			return nil, ErrInvalidAlgorithm
		}
		key.Precompute()
		w.writeString("kty", "RSA")
		w.writeString("alg", string(jwk.Alg))
		w.writeBase64("n", key.N.Bytes())
		w.writeIntBase64("e", uint32(key.E))
		w.writeBase64("d", key.D.Bytes())
		if len(key.Primes) >= 2 {
			w.writeBase64("p", key.Primes[0].Bytes())
			w.writeBase64("q", key.Primes[1].Bytes())
		}
		if key.Precomputed.Dp != nil {
			w.writeBase64("dp", key.Precomputed.Dp.Bytes())
		}
		if key.Precomputed.Dq != nil {
			w.writeBase64("dq", key.Precomputed.Dq.Bytes())
		}
		if key.Precomputed.Qinv != nil {
			w.writeBase64("qi", key.Precomputed.Qinv.Bytes())
		}

	case *rsa.PublicKey:
		if !jwk.Alg.isRSA() {
			return nil, ErrInvalidAlgorithm
		}
		w.writeString("kty", "RSA")
		w.writeString("alg", string(jwk.Alg))
		w.writeBase64("n", key.N.Bytes())
		w.writeIntBase64("e", uint32(key.E))

	case *mldsa.PublicKey:
		alg := mldsaAlgFromParams(key.Parameters())
		w.writeString("kty", "AKP")
		w.writeString("alg", string(alg))
		w.writeBase64("pub", key.Bytes())

	case *mldsa.PrivateKey:
		publicKey := key.PublicKey()
		alg := mldsaAlgFromParams(publicKey.Parameters())
		w.writeString("kty", "AKP")
		w.writeString("alg", string(alg))
		w.writeBase64("pub", publicKey.Bytes())
		w.writeBase64("priv", key.Bytes())

	case []byte:
		if !jwk.Alg.isHMAC() {
			return nil, ErrInvalidAlgorithm
		}
		w.writeString("kty", "oct")
		w.writeString("alg", string(jwk.Alg))
		w.writeBase64("k", key)

	default:
		return nil, ErrUnsupportedKeyType
	}

	if jwk.ID != "" {
		w.writeString("kid", jwk.ID)
	}

	w.writeByte('}')
	return w.buf, nil
}

// UnmarshalJSON implements json.Unmarshaler for JWK.
// It decodes a JSON Web Key and populates j.Key, j.Alg, and j.ID.
func (jwk *JWK) UnmarshalJSON(data []byte) error {
	var raw jwkFields
	if err := json.Unmarshal(data, &raw); err != nil {
		return ErrInvalidJWK
	}
	parsed, err := parseJWK(&raw)
	if err != nil {
		return err
	}
	*jwk = parsed
	return nil
}

func parseJWK(raw *jwkFields) (JWK, error) {
	switch raw.KTY {
	case "OKP":
		x, err := base64.RawURLEncoding.DecodeString(raw.X)
		if err != nil {
			return JWK{}, ErrInvalidJWK
		}

		switch raw.CRV {
		case "Ed25519":
			jwk := JWK{
				Alg: EdDSA,
				ID:  raw.KID,
			}
			if raw.D != "" {
				d, err := base64.RawURLEncoding.DecodeString(raw.D)
				if err != nil || len(d) != ed25519.SeedSize || len(x) != ed25519.PublicKeySize {
					return JWK{}, ErrInvalidJWK
				}
				priv := make(ed25519.PrivateKey, ed25519.PrivateKeySize)
				copy(priv[:ed25519.SeedSize], d)
				copy(priv[ed25519.SeedSize:], x)
				jwk.Key = priv
			} else {
				if len(x) != ed25519.PublicKeySize {
					return JWK{}, ErrInvalidJWK
				}
				jwk.Key = ed25519.PublicKey(x)
			}
			return jwk, nil

		default:
			return JWK{}, ErrUnsupportedCurve
		}

	case "EC":
		x, err := base64.RawURLEncoding.DecodeString(raw.X)
		if err != nil {
			return JWK{}, ErrInvalidJWK
		}
		y, err := base64.RawURLEncoding.DecodeString(raw.Y)
		if err != nil {
			return JWK{}, ErrInvalidJWK
		}

		var curve elliptic.Curve
		var alg Algorithm
		switch raw.CRV {
		case "P-256":
			curve = elliptic.P256()
			alg = ES256
		case "P-384":
			curve = elliptic.P384()
			alg = ES384
		case "P-521":
			curve = elliptic.P521()
			alg = ES512
		default:
			return JWK{}, ErrUnsupportedCurve
		}

		result := JWK{
			Alg: alg,
			ID:  raw.KID,
		}
		xBig := new(big.Int).SetBytes(x)
		yBig := new(big.Int).SetBytes(y)

		if raw.D != "" {
			d, err := base64.RawURLEncoding.DecodeString(raw.D)
			if err != nil {
				return JWK{}, ErrInvalidJWK
			}
			dBig := new(big.Int).SetBytes(d)
			result.Key = &ecdsa.PrivateKey{
				PublicKey: ecdsa.PublicKey{
					Curve: curve,
					X:     xBig,
					Y:     yBig,
				},
				D: dBig,
			}
		} else {
			result.Key = &ecdsa.PublicKey{
				Curve: curve,
				X:     xBig,
				Y:     yBig,
			}
		}
		return result, nil

	case "RSA":
		n, err := base64.RawURLEncoding.DecodeString(raw.N)
		if err != nil {
			return JWK{}, ErrInvalidJWK
		}
		e, err := base64.RawURLEncoding.DecodeString(raw.E)
		if err != nil {
			return JWK{}, ErrInvalidJWK
		}

		alg := Algorithm(raw.ALG)
		if !alg.isRSA() {
			return JWK{}, ErrInvalidAlgorithm
		}

		result := JWK{
			Alg: alg,
			ID:  raw.KID,
		}
		nBig := new(big.Int).SetBytes(n)
		eBig := new(big.Int).SetBytes(e)

		if raw.D != "" {
			d, err := base64.RawURLEncoding.DecodeString(raw.D)
			if err != nil {
				return JWK{}, ErrInvalidJWK
			}
			dBig := new(big.Int).SetBytes(d)

			priv := &rsa.PrivateKey{
				PublicKey: rsa.PublicKey{
					N: nBig,
					E: int(eBig.Int64()),
				},
				D: dBig,
			}

			if raw.P != "" && raw.Q != "" {
				p, err := base64.RawURLEncoding.DecodeString(raw.P)
				if err != nil {
					return JWK{}, ErrInvalidJWK
				}
				q, err := base64.RawURLEncoding.DecodeString(raw.Q)
				if err != nil {
					return JWK{}, ErrInvalidJWK
				}
				priv.Primes = []*big.Int{
					new(big.Int).SetBytes(p),
					new(big.Int).SetBytes(q),
				}

				if raw.DP != "" {
					dp, _ := base64.RawURLEncoding.DecodeString(raw.DP)
					if dp != nil {
						priv.Precomputed.Dp = new(big.Int).SetBytes(dp)
					}
				}
				if raw.DQ != "" {
					dq, _ := base64.RawURLEncoding.DecodeString(raw.DQ)
					if dq != nil {
						priv.Precomputed.Dq = new(big.Int).SetBytes(dq)
					}
				}
				if raw.QI != "" {
					qi, _ := base64.RawURLEncoding.DecodeString(raw.QI)
					if qi != nil {
						priv.Precomputed.Qinv = new(big.Int).SetBytes(qi)
					}
				}
			}

			result.Key = priv
		} else {
			result.Key = &rsa.PublicKey{
				N: nBig,
				E: int(eBig.Int64()),
			}
		}
		return result, nil

	case "oct":
		k, err := base64.RawURLEncoding.DecodeString(raw.K)
		if err != nil {
			return JWK{}, ErrInvalidJWK
		}
		return JWK{
			Key: k,
			Alg: Algorithm(raw.ALG),
			ID:  raw.KID,
		}, nil

	case "AKP":
		alg := Algorithm(raw.ALG)
		params, err := mldsaParamsFromAlg(alg)
		if err != nil {
			return JWK{}, err
		}

		pubBytes, err := base64.RawURLEncoding.DecodeString(raw.PUB)
		if err != nil {
			return JWK{}, ErrInvalidJWK
		}
		publicKey, err := mldsa.NewPublicKey(params, pubBytes)
		if err != nil {
			return JWK{}, ErrInvalidJWK
		}

		if raw.PRIV != "" {
			seed, err := base64.RawURLEncoding.DecodeString(raw.PRIV)
			if err != nil || len(seed) != mldsa.PrivateKeySize {
				return JWK{}, ErrInvalidJWK
			}
			sk, err := mldsa.NewPrivateKey(params, seed)
			if err != nil {
				return JWK{}, ErrInvalidJWK
			}
			if !publicKey.Equal(sk.PublicKey()) {
				return JWK{}, ErrInvalidJWK
			}
			return JWK{Key: sk, Alg: alg, ID: raw.KID}, nil
		}

		return JWK{Key: publicKey, Alg: alg, ID: raw.KID}, nil

	default:
		return JWK{}, ErrUnsupportedKeyType
	}
}

// ── internal helpers ────────────────────────────────────────

type jwkWriter struct {
	buf    []byte
	hasKey bool
}

func newJwkWriter(capacity int) jwkWriter {
	return jwkWriter{
		buf:    make([]byte, 0, capacity),
		hasKey: false,
	}
}

func (w *jwkWriter) writeString(key, value string) {
	if w.hasKey {
		w.buf = append(w.buf, ',')
	}
	w.hasKey = true
	w.buf = append(w.buf, '"')
	w.buf = append(w.buf, key...)
	w.buf = append(w.buf, '"', ':')
	w.buf = append(w.buf, '"')
	w.buf = append(w.buf, value...)
	w.buf = append(w.buf, '"')
}

func (w *jwkWriter) writeBase64(key string, data []byte) {
	if w.hasKey {
		w.buf = append(w.buf, ',')
	}
	w.hasKey = true
	w.buf = append(w.buf, '"')
	w.buf = append(w.buf, key...)
	w.buf = append(w.buf, '"', ':')
	w.buf = append(w.buf, '"')
	w.buf = base64.RawURLEncoding.AppendEncode(w.buf, data)
	w.buf = append(w.buf, '"')
}

func (w *jwkWriter) writeByte(b byte) {
	w.buf = append(w.buf, b)
}

// Appends the given `uint32` encoded with Base64RawURL
func (w *jwkWriter) writeIntBase64(key string, v uint32) {
	var tmp [4]byte
	binary.BigEndian.PutUint32(tmp[:], v)
	// skip the trailing zeroes
	i := 0
	for i < 3 && tmp[i] == 0 {
		i++
	}

	if w.hasKey {
		w.buf = append(w.buf, ',')
	}
	w.hasKey = true
	w.buf = append(w.buf, '"')
	w.buf = append(w.buf, key...)
	w.buf = append(w.buf, '"', ':')
	w.buf = append(w.buf, '"')
	w.buf = base64.RawURLEncoding.AppendEncode(w.buf, tmp[i:])
	w.buf = append(w.buf, '"')
}

func bigIntFixed(x *big.Int, minLen int) []byte {
	b := x.Bytes()
	if len(b) >= minLen {
		return b
	}
	padded := make([]byte, minLen)
	copy(padded[minLen-len(b):], b)
	return padded
}

func curveInfo(curve elliptic.Curve) (crv string, alg Algorithm, coordLen int, err error) {
	switch curve {
	case elliptic.P256():
		return "P-256", ES256, 32, nil
	case elliptic.P384():
		return "P-384", ES384, 48, nil
	case elliptic.P521():
		return "P-521", ES512, 66, nil
	default:
		return "", "", 0, ErrUnsupportedCurve
	}
}

func mldsaParamsFromAlg(alg Algorithm) (params mldsa.Parameters, err error) {
	switch alg {
	case MLDSA44:
		params = mldsa.MLDSA44()
	case MLDSA65:
		params = mldsa.MLDSA65()
	case MLDSA87:
		params = mldsa.MLDSA87()
	default:
		err = ErrInvalidAlgorithm
	}
	return
}

func mldsaAlgFromParams(params mldsa.Parameters) Algorithm {
	switch params.PublicKeySize() {
	case mldsa.MLDSA44PublicKeySize:
		return MLDSA44
	case mldsa.MLDSA65PublicKeySize:
		return MLDSA65
	case mldsa.MLDSA87PublicKeySize:
		return MLDSA87
	default:
		return ""
	}
}
