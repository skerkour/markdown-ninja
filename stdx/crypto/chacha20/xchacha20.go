package chacha20

import (
	"errors"

	"markdown.ninja/stdx/crypto/chacha"
)

const (
	NonceSizeX = 24
)

var (
	ErrBadNonceXLength = errors.New("chacha20: bad nonce length for XChaCha20. 24 bytes required")
)

// NewX returns a new instance of the XChaCha20 stream cipher.
// as of now we use the IETF chacha20 variant with 96-bit nonces
func NewX(key, nonce []byte) (StreamCipher, error) {
	// encryptionKey := make([]byte, 32)
	// chachaNonce := make([]byte, 12)

	if len(key) != KeySize {
		return nil, ErrBadKeyLength
	}
	if len(nonce) != NonceSizeX {
		return nil, ErrBadNonceXLength
	}

	// derive chacha's encryption key from the original key and the first 128 bits of the nonce
	chachaKey, _ := chacha.HChaCha20(key, nonce[0:16])
	// use the last 64 bits of the nonce as the nonce for chacha
	// copy(chachaNonce[4:12], nonce[16:24])
	return New(chachaKey, nonce[16:24])
}
