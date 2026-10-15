package chacha20

import (
	"crypto/cipher"

	"markdown.ninja/stdx/crypto/chacha"
)

func New(key, nonce []byte) (cipher.Stream, error) {
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
	return chacha.NewCipher(ietfNonce, key, 20)
}
