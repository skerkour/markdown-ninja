package chacha

import "errors"

const (
	HChaCha20KeySize   = 32
	HChaCha20NonceSize = 16
)

var (
	ErrBadHChaCha20KeySize   = errors.New("chacha: bad HChaCha20 key size")
	ErrBadHChaCha20NonceSize = errors.New("chacha: bad HChaCha20 nonce size")
)

// HChaCha20 generates 32 pseudo-random bytes from a 128 bit nonce and a 256 bit secret key.
// It can be used as a key-derivation-function (KDF).
func HChaCha20(key, nonce []byte) ([]byte, error) {
	// This function is split into a wrapper so that the slice allocation will
	// be inlined, and depending on how the caller uses the return value, won't
	// escape to the heap.
	out := make([]byte, 32)
	return hChaCha20(out, key, nonce)
}
