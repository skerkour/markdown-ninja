//go:build (!amd64 && !386) || !gc || purego

package chacha

func hChaCha20(out, key, nonce []byte) ([]byte, error) {
	if len(key) != HChaCha20KeySize {
		return nil, ErrBadHChaCha20KeySize
	}
	if len(nonce) != HChaCha20NonceSize {
		return nil, ErrBadHChaCha20NonceSize
	}

	hChaCha20Generic((*[32]byte)(out), (*[16]byte)(nonce), (*[32]byte)(key))
	return out, nil
}
