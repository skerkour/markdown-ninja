package chacha20

import (
	"crypto/cipher"
	"crypto/subtle"
	"encoding/binary"
	"math/bits"
	"unsafe"
)

const blockSize = 64

// The "sigma" constant which is the value of the first row of ChaCha's state.
var constant = [4]uint32{
	0x61707865, // "expa"
	0x3320646e, // "nd 3"
	0x79622d32, // "2-by"
	0x6b206574, // "te k"
}

// Cipher is a stateful ChaCha20 stream cipher supporting both the original DJB
// layout (64-bit counter at words 12-13, 8-byte nonce at words 14-15) and the
// IETF / RFC 8439 layout (32-bit counter at word 12, 12-byte nonce at words
// 13-15). It implements cipher.Stream.
//
// It keeps the ChaCha20 state as a plain [16]uint32 so that different SIMD
// backends (4-block, 8-block, 16-block, ...) can each rebuild their own vector
// layout from it on demand, and a portable scalar backend can run on platforms
// where SIMD is unavailable.
type Cipher struct {
	// state is the 16-word ChaCha20 state: words 0-3 constants, 4-11 key,
	// 12-13 counter (word 12 only for the IETF layout), 13-15 nonce (words
	// 14-15 only for the DJB layout).
	state [16]uint32

	// leftover holds unused key stream from the last partial block, valid for
	// leftoverLen bytes at the front. Max 63 = one block minus one.
	leftover    [63]byte
	leftoverLen uint8
	// set to true if the IETF counter wrapped
	overflow bool
	// djb selects the original DJB layout: 64-bit counter spanning words 12-13
	// and an 8-byte nonce in words 14-15, instead of the IETF 32-bit counter
	// (word 12) and 12-byte nonce (words 13-15).
	djb bool
}

var _ cipher.Stream = (*Cipher)(nil)

// NewIetf creates a ChaCha20 (IETF) stream cipher with the given 32-byte key
// and 12-byte nonce.
func NewIetf(key [32]byte, nonce [12]byte) Cipher {
	var cipher Cipher

	// constant
	copy(cipher.state[:4], constant[:])
	// cipher.state[0] = constant0
	// cipher.state[1] = constant1
	// cipher.state[2] = constant2
	// cipher.state[3] = constant3

	// key
	for i := 0; i < 8; i++ {
		cipher.state[4+i] = binary.LittleEndian.Uint32(key[i*4 : i*4+4])
	}
	// cipher.state[4] = binary.LittleEndian.Uint32(key[0:4])
	// cipher.state[5] = binary.LittleEndian.Uint32(key[4:8])
	// cipher.state[6] = binary.LittleEndian.Uint32(key[8:12])
	// cipher.state[7] = binary.LittleEndian.Uint32(key[12:16])
	// cipher.state[8] = binary.LittleEndian.Uint32(key[16:20])
	// cipher.state[9] = binary.LittleEndian.Uint32(key[20:24])
	// cipher.state[10] = binary.LittleEndian.Uint32(key[24:28])
	// cipher.state[11] = binary.LittleEndian.Uint32(key[28:32])

	// nonce
	for i := 0; i < 3; i++ {
		cipher.state[13+i] = binary.LittleEndian.Uint32(nonce[i*4 : i*4+4])
	}
	// cipher.state[13] = binary.LittleEndian.Uint32(nonce[0:4])
	// cipher.state[14] = binary.LittleEndian.Uint32(nonce[4:8])
	// cipher.state[15] = binary.LittleEndian.Uint32(nonce[8:12])

	return cipher
}

// NewDjb creates a ChaCha20 (original DJB) stream cipher with the given
// 32-byte key and 8-byte nonce.
func NewDjb(key [32]byte, nonce [8]byte) Cipher {
	var cipher Cipher
	cipher.djb = true

	// constant
	copy(cipher.state[:4], constant[:])

	// key
	for i := 0; i < 8; i++ {
		cipher.state[4+i] = binary.LittleEndian.Uint32(key[i*4 : i*4+4])
	}

	// counter (words 12-13) stays at 0

	// nonce
	cipher.state[14] = binary.LittleEndian.Uint32(nonce[0:4])
	cipher.state[15] = binary.LittleEndian.Uint32(nonce[4:8])

	return cipher
}

// counter returns the current block counter: a 64-bit value spanning words
// 12-13 for the DJB layout, or the 32-bit word-12 counter for the IETF layout.
func (cipher *Cipher) counter() uint64 {
	if cipher.djb {
		return uint64(cipher.state[12]) | uint64(cipher.state[13])<<32
	}
	return uint64(cipher.state[12])
}

// setCounter writes the block counter to words 12-13 (DJB) or word 12 (IETF).
func (cipher *Cipher) setCounter(counter uint64) {
	cipher.state[12] = uint32(counter)
	if cipher.djb {
		cipher.state[13] = uint32(counter >> 32)
	}
}

// incCounter advances the block counter by one block.
func (cipher *Cipher) incCounter() {
	if cipher.djb {
		c := cipher.counter() + 1
		cipher.setCounter(c)
	} else {
		cipher.state[12]++
	}
}

// processTail advances the block counter by the number of whole blocks XORed
// (n/64) plus one more when n is a partial block, and stores the unused tail of
// that last partial block as leftover key stream. keystream holds a full SIMD
// batch of generated key stream and n is how many of its bytes were consumed.
//
// ctr is the DJB scalar counter (0 for the IETF layout); it is written back to
// state and returned, so callers can keep advancing it across batches. It is
// shared by the SIMD backends so the tail/leftover handling (including the
// bounds-check elision on cipher.leftover) lives in exactly one place.
func (cipher *Cipher) processTail(dst, src, keystream []byte, ctr uint64) uint64 {
	// min() removes bound checks
	n := min(len(src), len(keystream))
	subtle.XORBytes(dst, src, keystream[:n])

	blocks := uint64(n / 64)
	if n%64 != 0 {
		blocks++
	}
	if cipher.djb {
		ctr += blocks
		cipher.state[12] = uint32(ctr)
		cipher.state[13] = uint32(ctr >> 32)
	} else {
		cipher.state[12] += uint32(blocks)
	}

	if n%64 != 0 {
		// n%64 != 0 here, so 64-n%64 is already ≤ 63; min makes that
		// visible for bounds-check elision on cipher.leftover[:]
		leftoverLen := min(64-n%64, 63)
		copy(cipher.leftover[:leftoverLen], keystream[n:])
		cipher.leftoverLen = uint8(leftoverLen)
	} else {
		cipher.leftoverLen = 0
	}
	return ctr
}

// SetCounter sets the block counter (words 12-13 for the DJB layout, word 12
// for the IETF layout) and drops any buffered leftover key stream. It allows
// moving the counter forward or backward in the key stream.
func (cipher *Cipher) SetCounter(counter uint64) {
	cipher.setCounter(counter)
	cipher.leftoverLen = 0
	cipher.overflow = false
}

// XORKeyStream XORs each byte of src with the key stream and writes the result
// to dst. It maintains state across calls: leftover key stream from a previous
// call is consumed first, and any unused key stream from the final partial
// block is retained for the next call.
//
// dst and src must overlap entirely or not at all; len(dst) must be >=
// len(src).
func (cipher *Cipher) XORKeyStream(dst, src []byte) {
	if len(src) == 0 {
		return
	}
	if len(dst) < len(src) {
		panic("chacha20: output smaller than input")
	}
	if inexactOverlap(dst[:len(src)], src) {
		panic("chacha20: invalid buffer overlap")
	}

	// Drain leftover keystream from a previous call.
	if cipher.leftoverLen > 0 {
		// leftoverLen is at most len(leftover) == 63; min makes that fact
		// visible, so all leftover slicing below is bounds-check-free.
		leftoverLen := min(int(cipher.leftoverLen), len(cipher.leftover))
		n := min(leftoverLen, len(src))
		subtle.XORBytes(dst[:n], src[:n], cipher.leftover[:n])
		copy(cipher.leftover[:], cipher.leftover[n:leftoverLen])
		cipher.leftoverLen = uint8(leftoverLen - n)
		dst, src = dst[n:], src[n:]
		if len(src) == 0 {
			return
		}
	}

	// The DJB layout has a 64-bit counter, so it cannot overflow in practice;
	// only the IETF layout can exhaust its 32-bit counter.
	if !cipher.djb {
		numBlocks := (uint64(len(src)) + blockSize - 1) / blockSize
		if cipher.overflow || uint64(cipher.state[12])+numBlocks > 1<<32 {
			panic("chacha20: counter overflow")
		} else if uint64(cipher.state[12])+numBlocks == 1<<32 {
			cipher.overflow = true
		}
	}

	// Generate fresh keystream for the remaining input. xorKeyStream is
	// the platform-specific backend: SIMD where available, scalar elsewhere.
	cipher.xorKeyStream(dst, src)
}

// xorKeyStreamScalar is the portable backend. It XORs src with the key stream
// generated from state and returns the number of leftover key stream bytes
// (0..63) stored at leftover[:n].
func (cipher *Cipher) xorKeyStreamScalar(dst, src []byte) {
	for len(src) > 0 {
		block := cipher.chacha20Block()
		n := min(blockSize, len(src), len(dst))
		subtle.XORBytes(dst[:n], src[:n], block[:n])
		cipher.incCounter()

		if n < blockSize {
			copy(cipher.leftover[:], block[n:])
			cipher.leftoverLen = uint8(blockSize - n)
			return
		}
		dst, src = dst[n:], src[n:]
	}

	cipher.leftoverLen = 0
}

// chacha20Block computes one 64-byte key stream block for the state.
func (cipher *Cipher) chacha20Block() [64]byte {
	var w [16]uint32
	copy(w[:], cipher.state[:])

	for r := 0; r < 10; r++ {
		w[0], w[4], w[8], w[12] = quarterRound(w[0], w[4], w[8], w[12])
		w[1], w[5], w[9], w[13] = quarterRound(w[1], w[5], w[9], w[13])
		w[2], w[6], w[10], w[14] = quarterRound(w[2], w[6], w[10], w[14])
		w[3], w[7], w[11], w[15] = quarterRound(w[3], w[7], w[11], w[15])

		w[0], w[5], w[10], w[15] = quarterRound(w[0], w[5], w[10], w[15])
		w[1], w[6], w[11], w[12] = quarterRound(w[1], w[6], w[11], w[12])
		w[2], w[7], w[8], w[13] = quarterRound(w[2], w[7], w[8], w[13])
		w[3], w[4], w[9], w[14] = quarterRound(w[3], w[4], w[9], w[14])
	}

	var out [64]byte
	for i := 0; i < 16; i++ {
		binary.LittleEndian.PutUint32(out[i*4:], w[i]+cipher.state[i])
	}
	return out
}

func quarterRound(a, b, c, d uint32) (uint32, uint32, uint32, uint32) {
	a += b
	d ^= a
	d = bits.RotateLeft32(d, 16)
	c += d
	b ^= c
	b = bits.RotateLeft32(b, 12)
	a += b
	d ^= a
	d = bits.RotateLeft32(d, 8)
	c += d
	b ^= c
	b = bits.RotateLeft32(b, 7)
	return a, b, c, d
}

// inexactOverlap reports whether x and y share memory in a way that is not
// allowed for XORKeyStream: overlapping partially, but not identical pointers.
func inexactOverlap(x, y []byte) bool {
	if len(x) == 0 || len(y) == 0 || &x[0] == &y[0] {
		return false
	}
	return anyOverlap(x, y)
}

func anyOverlap(x, y []byte) bool {
	return len(x) > 0 && len(y) > 0 &&
		uintptr(unsafe.Pointer(&x[0]))+uintptr(len(x)) > uintptr(unsafe.Pointer(&y[0])) &&
		uintptr(unsafe.Pointer(&y[0]))+uintptr(len(y)) > uintptr(unsafe.Pointer(&x[0]))
}
