//go:build amd64 && gc && !purego

package chacha

// This function is implemented in hchacha20_amd64.s
//
//go:noescape
func hChaCha20AVX(out *[32]byte, nonce *[16]byte, key *[32]byte)

// This function is implemented in hchacha20_amd64.s
//
//go:noescape
func hChaCha20SSE2(out *[32]byte, nonce *[16]byte, key *[32]byte)

// This function is implemented in hchacha20_amd64.s
//
//go:noescape
func hChaCha20SSSE3(out *[32]byte, nonce *[16]byte, key *[32]byte)

func hChaCha20(out, key, nonce []byte) ([]byte, error) {
	if len(key) != HChaCha20KeySize {
		return nil, ErrBadHChaCha20KeySize
	}
	if len(nonce) != HChaCha20NonceSize {
		return nil, ErrBadHChaCha20NonceSize
	}

	switch {
	case useAVX:
		hChaCha20AVX((*[32]byte)(out), (*[16]byte)(nonce), (*[32]byte)(key))
	case useSSSE3:
		hChaCha20SSSE3((*[32]byte)(out), (*[16]byte)(nonce), (*[32]byte)(key))
	case useSSE2:
		hChaCha20SSE2((*[32]byte)(out), (*[16]byte)(nonce), (*[32]byte)(key))
	default:
		hChaCha20Generic((*[32]byte)(out), (*[16]byte)(nonce), (*[32]byte)(key))
	}

	return out, nil
}
