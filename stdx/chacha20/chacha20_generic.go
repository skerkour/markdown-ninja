//go:build !arm64 && !amd64

package chacha20

// xorKeyStream is the portable scalar backend hook. It XORs src with the
// key stream generated from state and maintains leftover key stream state.
func (cipher *Cipher) xorKeyStream(dst, src []byte) {
	cipher.xorKeyStreamScalar(dst, src)
}
