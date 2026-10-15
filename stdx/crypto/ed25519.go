package crypto

import (
	"crypto"
	"crypto/ed25519"
	"crypto/sha512"
	"errors"
	"io"
	"math/big"
)

const (
	// Ed25519PublicKeySize is the size, in bytes, of public keys as used in this package.
	Ed25519PublicKeySize = ed25519.PublicKeySize

	// Ed25519PrivateKeySize is the size, in bytes, of private keys as used in this package.
	Ed25519PrivateKeySize = ed25519.PrivateKeySize

	// Ed25519SignatureSize is the size, in bytes, of signatures generated and verified by this package.
	Ed25519SignatureSize = ed25519.SignatureSize

	// Ed25519SeedSize is the size, in bytes, of private key seeds. These are the private key representations used by RFC 8032.
	Ed25519SeedSize = ed25519.SeedSize

	// Ed25519SignerOpts must be used for `PrivateKey.Sign`
	Ed25519SignerOpts = crypto.Hash(0)
)

// Ed25519PublicKey is the type of Ed25519 public keys.
type Ed25519PublicKey ed25519.PublicKey

// Verify reports whether sig is a valid signature of message by publicKey.
// returns true if signature is valid. false otherwise.
func (publicKey Ed25519PublicKey) Verify(message, signature []byte) (bool, error) {
	if len(publicKey) != Ed25519PublicKeySize {
		return false, errors.New("crypto: Invalid Ed25519 public key size")
	}

	return ed25519.Verify(ed25519.PublicKey(publicKey), message, signature), nil
}

var curve25519P, _ = new(big.Int).SetString("57896044618658097711785492504343953926634992332820282019728792003956564819949", 10)

// ToCurve25519PublicKey returns the corresponding Curve25519 public key.
//
// See here for more details: https://blog.filippo.io/using-ed25519-keys-for-encryption
func (publicKey Ed25519PublicKey) ToCurve25519PublicKey() Curve25519PublicKey {
	// taken from https://github.com/FiloSottile/age/blob/master/internal/agessh/agessh.go#L179

	// ed25519.PublicKey is a little endian representation of the y-coordinate,
	// with the most significant bit set based on the sign of the x-coordinate.
	bigEndianY := make([]byte, Ed25519PublicKeySize)
	for i, b := range publicKey {
		bigEndianY[Ed25519PublicKeySize-i-1] = b
	}
	bigEndianY[0] &= 0b0111_1111

	// The Montgomery u-coordinate is derived through the bilinear map
	//
	//     u = (1 + y) / (1 - y)
	//
	// See https://blog.filippo.io/using-ed25519-keys-for-encryption.
	y := new(big.Int).SetBytes(bigEndianY)
	denom := big.NewInt(1)
	denom.ModInverse(denom.Sub(denom, y), curve25519P) // 1 / (1 - y)
	u := y.Mul(y.Add(y, big.NewInt(1)), denom)
	u.Mod(u, curve25519P)

	out := make([]byte, Curve25519PublicKeySize)
	uBytes := u.Bytes()
	for i, b := range uBytes {
		out[len(uBytes)-i-1] = b
	}

	return out
}

func (publicKey Ed25519PublicKey) Bytes() []byte {
	return publicKey[:]
}

func NewEd25519PublicKeyFromBytes(key []byte) (publicKey Ed25519PublicKey, err error) {
	if len(key) != Ed25519PublicKeySize {
		err = errors.New("crypto: Invalid Ed25519 public key size")
		return
	}
	publicKey = Ed25519PublicKey(key)
	return
}

// Ed25519PrivateKey is the type of Ed25519 private keys. It implements crypto.Signer.
type Ed25519PrivateKey ed25519.PrivateKey

// Sign signs the given message with priv.
// Ed25519 performs two passes over messages to be signed and therefore cannot
// handle pre-hashed messages. Thus opts.HashFunc() must return zero to
// indicate the message hasn't been hashed. This can be achieved by passing
// crypto.Ed25519SignerOpts as the value for opts.
func (privateKey Ed25519PrivateKey) Sign(rand io.Reader, message []byte, opts crypto.SignerOpts) (signature []byte, err error) {
	if len(privateKey) != Ed25519PrivateKeySize {
		return nil, errors.New("crpyto: Invalid Ed25519 private key size")
	}

	return ed25519.Sign(ed25519.PrivateKey(privateKey), message), nil
}

// ToCurve25519PrivateKey returns a corresponding Curve25519 private key.
//
// See here for more details: https://blog.filippo.io/using-ed25519-keys-for-encryption
func (privateKey Ed25519PrivateKey) ToCurve25519PrivateKey() Curve25519PrivateKey {
	// taken from https://github.com/FiloSottile/age/blob/292c3aaeea0695dbba356dfe18a70f10efb17d75/internal/agessh/agessh.go#L294
	h := sha512.New()
	h.Write(privateKey.Seed())
	out := h.Sum(nil)
	return out[:Curve25519PrivateKeySize]
}

// Public returns the Ed25519PublicKey corresponding to priv.
func (privateKey Ed25519PrivateKey) Public() Ed25519PublicKey {
	return Ed25519PublicKey(ed25519.PrivateKey(privateKey).Public().(ed25519.PublicKey))
}

// Seed returns the private key seed corresponding to priv. It is provided for interoperability
// with RFC 8032. RFC 8032's private keys correspond to seeds in this package.
func (privateKey Ed25519PrivateKey) Seed() []byte {
	return ed25519.PrivateKey(privateKey).Seed()
}

func (privateKey Ed25519PrivateKey) Bytes() []byte {
	return privateKey[:]
}

func NewEd25519PrivateKeyFromBytes(key []byte) (privateKey Ed25519PrivateKey, err error) {
	if len(key) != Ed25519PrivateKeySize {
		err = errors.New("crypto: Invalid Ed25519 private key size")
		return
	}
	privateKey = Ed25519PrivateKey(key)
	return
}

// GenerateEd25519KeyPair generates a public/private Ed25519 key pair
func GenerateEd25519KeyPair() (Ed25519PublicKey, Ed25519PrivateKey, error) {
	public, private, err := ed25519.GenerateKey(nil)
	return Ed25519PublicKey(public), Ed25519PrivateKey(private), err
}

// NewEd25519PrivateKeyFromSeed calculates a private key from a seed. It will panic if
// len(seed) is not SeedSize. This function is provided for interoperability
// with RFC 8032. RFC 8032's private keys correspond to seeds in this
// package.
func NewEd25519PrivateKeyFromSeed(seed []byte) (Ed25519PrivateKey, error) {
	if len(seed) != Ed25519SeedSize {
		return nil, errors.New("crypto: Invalid Ed25519 seed size")
	}

	private := ed25519.NewKeyFromSeed(seed)
	return Ed25519PrivateKey(private), nil
}
