//go:build amd64

package chacha20

import (
	"bytes"
	"testing"

	"simd/archsimd"
)

// TestCipherDjbAVX512Direct drives the AVX-512 backend directly (bypassing the
// runtime feature dispatch) so the 8/16-block DJB counter paths are validated
// even when the dispatcher would pick another path for a given input size.
// Skipped where the feature is unavailable.
func TestCipherDjbAVX512Direct(t *testing.T) {
	if !archsimd.X86.AVX512() {
		t.Skip("AVX-512 not available")
	}
	key := testKey()
	nonce := testDjbNonce()

	counters := []uint64{0, 1, 0xffffff00, 0xffffffff, 0x1_00000000, 0x1_ffffff80, 0x1_fffffff8, 0xffffffff_fffffff0}
	sizes := []int{1, 63, 64, 65, 129, 256, 257, 511, 512, 513, 1023, 1024, 1025, 2048, 5000}

	for _, counter := range counters {
		for _, size := range sizes {
			pt := testPlaintext(size)

			c := NewDjb(key, nonce)
			c.SetCounter(counter)
			dst := make([]byte, size)
			c.xorKeyStreamAVX512(dst, pt)

			scalar := NewDjb(key, nonce)
			scalar.SetCounter(counter)
			ref := djbScalarXOR(scalar, pt)

			if !bytes.Equal(dst, ref) {
				t.Fatalf("AVX512/scalar mismatch: size=%d counter=%#x", size, counter)
			}
		}
	}
}

// TestCipherDjbAVX2Direct drives the AVX-256 backend directly, so the 8-block
// DJB counter path is validated even when the dispatcher would not select it.
// Skipped where AVX2 is unavailable.
func TestCipherDjbAVX2Direct(t *testing.T) {
	if !archsimd.X86.AVX2() {
		t.Skip("AVX2 not available")
	}
	key := testKey()
	nonce := testDjbNonce()

	counters := []uint64{0, 1, 0xffffff00, 0xffffffff, 0x1_00000000, 0x1_ffffff80, 0x1_fffffff8, 0xffffffff_fffffff0}
	sizes := []int{1, 63, 64, 65, 129, 255, 256, 257, 511, 512, 513, 1023, 1024, 2048, 5000}

	for _, counter := range counters {
		for _, size := range sizes {
			pt := testPlaintext(size)

			c := NewDjb(key, nonce)
			c.SetCounter(counter)
			dst := make([]byte, size)
			c.xorKeyStreamAVX2(dst, pt)

			scalar := NewDjb(key, nonce)
			scalar.SetCounter(counter)
			ref := djbScalarXOR(scalar, pt)

			if !bytes.Equal(dst, ref) {
				t.Fatalf("AVX2/scalar mismatch: size=%d counter=%#x", size, counter)
			}
		}
	}
}
