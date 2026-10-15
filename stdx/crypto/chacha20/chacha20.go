// Package chacha20 implements the ChaCha20 / XChaCha20 stream chipher.
// Notice that one specific key-nonce combination must be unique for all time.
//
// There are three versions of ChaCha20:
// - ChaCha20 with a 64 bit nonce (en/decrypt up to 2^64 * 64 bytes for one key-nonce combination)
// - ChaCha20 with a 96 bit nonce (en/decrypt up to 2^32 * 64 bytes (~256 GB) for one key-nonce combination)
// - XChaCha20 with a 192 bit nonce (en/decrypt up to 2^64 * 64 bytes for one key-nonce combination)
package chacha20

import (
	"crypto/cipher"
	"errors"
)

const (
	KeySize   = 32
	NonceSize = 8
)

var (
	ErrBadKeyLength   = errors.New("chacha20: bad key length. 32 bytes required")
	ErrBadNonceLength = errors.New("chacha20: bad nonce length for ChaCha20. 8 bytes required")
)

type StreamCipher interface {
	cipher.Stream
	SetCounter(n uint32)
}
