//go:build !amd64

package chacha20

import (
	"golang.org/x/crypto/chacha20"
)

// see also: https://cs.opensource.google/go/x/crypto/+/master:chacha20/chacha_generic.go

// XORKeyStream crypts bytes from src to dst using the given nonce and key.
// The length of the nonce determinds the version of ChaCha20:
// - 8 bytes:  ChaCha20 with a 64 bit nonce and a 2^64 * 64 byte period.
// - 12 bytes: ChaCha20 as defined in RFC 7539 and a 2^32 * 64 byte period.
// - 24 bytes: XChaCha20 with a 192 bit nonce and a 2^64 * 64 byte period.
// Src and dst may be the same slice but otherwise should not overlap.
// If len(dst) < len(src) this function panics.
// If the nonce is neither 64, 96 nor 192 bits long, this function panics.
// func XORKeyStream(dst, src, nonce, key []byte) {
// 	if runtime.GOARCH == "amd64" || runtime.GOARCH == "386" {
// 		chacha.XORKeyStream(dst, src, nonce, key, 20)
// 		return
// 	}

// 	chacha20.(dst, src, nonce, key, 20)
// }

// New returns a new cipher.Stream implementing a ChaCha20 version.
// The nonce must be unique for one key for all time.
// The length of the nonce determinds the version of ChaCha20:
// - 8 bytes:  ChaCha20 with a 64 bit nonce and a 2^64 * 64 byte period.
// - 12 bytes: ChaCha20 as defined in RFC 7539 and a 2^32 * 64 byte period.
// - 24 bytes: XChaCha20 with a 192 bit nonce and a 2^64 * 64 byte period.
// If the nonce is neither 64, 96 nor 192 bits long, a non-nil error is returned.
func New(key, nonce []byte) (StreamCipher, error) {
	// TODO: here we use a 96-bit nonce, because IETF's ChaCha20 use a 96-bit nonce,
	// but in the future we will want to use the original ChaCha20 with a 64-bit nonce
	// so for now, messages are limited to (64 B * 2^32-1) = ~256 GB
	var ietfNonce [12]byte

	if len(key) != KeySize {
		return nil, ErrBadKeyLength
	}
	if len(nonce) != NonceSize {
		return nil, ErrBadNonceLength
	}

	copy(ietfNonce[4:12], nonce[0:8])
	return chacha20.NewUnauthenticatedCipher(key, ietfNonce[:])
}
