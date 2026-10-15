//go:build goexperiment.simd

package chacha20

import (
	"bytes"
	"testing"

	chacha20std "golang.org/x/crypto/chacha20"
)

func testKeyNonce() ([32]byte, [12]byte) {
	var key [32]byte
	var nonce [12]byte
	for i := range key {
		key[i] = byte(i * 7)
	}
	for i := range nonce {
		nonce[i] = byte(255 - i*3)
	}
	return key, nonce
}

func TestXORKeyStream(t *testing.T) {
	key, nonce := testKeyNonce()

	sizes := []int{0, 1, 63, 64, 65, 127, 128, 129, 255, 256, 257, 383, 511, 512, 513, 1023, 1024, 4096}
	counters := []uint32{0, 1, 1000}

	for _, size := range sizes {
		for _, counter := range counters {
			src := make([]byte, size)
			for i := range src {
				src[i] = byte(i*13 + size)
			}

			ref, err := chacha20std.NewUnauthenticatedCipher(key[:], nonce[:])
			if err != nil {
				t.Fatal(err)
			}
			ref.SetCounter(counter)
			refDst := make([]byte, size)
			ref.XORKeyStream(refDst, src)

			cipher := NewIetf(key, nonce)
			cipher.SetCounter(uint64(counter))
			dst := make([]byte, size)
			cipher.XORKeyStream(dst, src)

			if !bytes.Equal(refDst, dst) {
				t.Fatalf("XORKeyStream mismatch: size=%d counter=%d", size, counter)
			}
		}
	}
}

func TestXORKeyStreamHighCounter(t *testing.T) {
	key, nonce := testKeyNonce()

	// near-wrap counters, single block each: the reference panics on actual
	// counter overflow, so wrap cases are tested one block at a time.
	counters := []uint32{0xffffffff - 1, 0xfffffffc, 0xfffffffe}
	for _, counter := range counters {
		src := make([]byte, 64)
		for i := range src {
			src[i] = byte(i*7 + 3)
		}

		ref, err := chacha20std.NewUnauthenticatedCipher(key[:], nonce[:])
		if err != nil {
			t.Fatal(err)
		}
		ref.SetCounter(counter)
		refDst := make([]byte, 64)
		ref.XORKeyStream(refDst, src)

		cipher := NewIetf(key, nonce)
		cipher.SetCounter(uint64(counter))
		dst := make([]byte, 64)
		cipher.XORKeyStream(dst, src)

		if !bytes.Equal(refDst, dst) {
			t.Fatalf("high-counter mismatch: counter=%d", counter)
		}
	}
}

func TestXORKeyStreamInPlace(t *testing.T) {
	key, nonce := testKeyNonce()

	sizes := []int{64, 255, 256, 257, 383, 511, 512, 513, 1024, 4096}
	for _, size := range sizes {
		ref, err := chacha20std.NewUnauthenticatedCipher(key[:], nonce[:])
		if err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, size)
		for i := range buf {
			buf[i] = byte(i*11 + size)
		}
		refDst := make([]byte, size)
		copy(refDst, buf)
		ref.XORKeyStream(refDst, refDst)

		cipher := NewIetf(key, nonce)
		dst := make([]byte, size)
		copy(dst, buf)
		cipher.XORKeyStream(dst, dst)

		if !bytes.Equal(refDst, dst) {
			t.Fatalf("in-place mismatch: size=%d", size)
		}
	}
}

func TestXORKeyStreamUnaligned(t *testing.T) {
	key, nonce := testKeyNonce()

	for _, srcOff := range []int{1, 3, 7} {
		for _, dstOff := range []int{1, 5, 9} {
			size := 1024
			full := make([]byte, size+16)
			for i := range full {
				full[i] = byte(i*3 + 7)
			}
			src := full[srcOff : srcOff+size]

			ref, err := chacha20std.NewUnauthenticatedCipher(key[:], nonce[:])
			if err != nil {
				t.Fatal(err)
			}
			refDst := make([]byte, size)
			ref.XORKeyStream(refDst, src)

			dstFull := make([]byte, size+16)
			dst := dstFull[dstOff : dstOff+size]
			cipher := NewIetf(key, nonce)
			cipher.XORKeyStream(dst, src)

			if !bytes.Equal(refDst, dst) {
				t.Fatalf("unaligned mismatch: srcOff=%d dstOff=%d", srcOff, dstOff)
			}
		}
	}
}

func TestXORKeyStreamAcrossCalls(t *testing.T) {
	key, nonce := testKeyNonce()

	// Split the input into chunks that deliberately cross 64-byte block
	// boundaries to exercise the leftover key stream path.
	chunks := []int{1, 63, 64, 65, 127, 128, 129, 300}
	size := 0
	for _, c := range chunks {
		size += c
	}
	src := make([]byte, size)
	for i := range src {
		src[i] = byte(i*5 + 1)
	}

	ref, err := chacha20std.NewUnauthenticatedCipher(key[:], nonce[:])
	if err != nil {
		t.Fatal(err)
	}
	refDst := make([]byte, size)
	ref.XORKeyStream(refDst, src)

	cipher := NewIetf(key, nonce)
	dst := make([]byte, size)
	off := 0
	for _, c := range chunks {
		cipher.XORKeyStream(dst[off:off+c], src[off:off+c])
		off += c
	}

	if !bytes.Equal(refDst, dst) {
		t.Fatalf("across-calls mismatch")
	}

	// SetCounter must drop any buffered leftover key stream: after a partial
	// block, resetting the counter to 0 and streaming again from the start
	// must produce the reference stream for the whole input.
	cipher = NewIetf(key, nonce)
	dst2 := make([]byte, size)
	cipher.XORKeyStream(dst2[:5], src[:5])
	cipher.SetCounter(0)
	cipher.XORKeyStream(dst2, src)
	if !bytes.Equal(refDst, dst2) {
		t.Fatalf("SetCounter leftover mismatch")
	}
}
