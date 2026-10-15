// Package blake3 implements the BLAKE3 cryptographic hash function, a pure-Go,
// cgo-free port of the official reference implementation
// (https://github.com/BLAKE3-team/BLAKE3/tree/master/reference_impl), verified
// against the official BLAKE3 test vectors.
//
// It provides the three BLAKE3 modes — unkeyed hashing (New, Sum256), keyed
// hashing / MAC (NewKeyed, KeyedHash) and key derivation (NewDeriveKey,
// DeriveKey) — as well as streaming Hasher that satisfies hash.Hash and a
// seekable extendable-output reader (Hasher.Xof, XofReader).
package blake3

import (
	"encoding/binary"
	"hash"
	"math/bits"
)

const (
	// blockLen is the size in bytes of one compression input block.
	blockLen = 64
	// chunkLen is the size in bytes of one chunk (16 blocks).
	chunkLen = 1024
	// keyLen is the size in bytes of a BLAKE3 key.
	keyLen = 32

	// Domain-separation flags.
	flagChunkStart        uint32 = 1 << 0
	flagChunkEnd          uint32 = 1 << 1
	flagParent            uint32 = 1 << 2
	flagRoot              uint32 = 1 << 3
	flagKeyedHash         uint32 = 1 << 4
	flagDeriveKeyContext  uint32 = 1 << 5
	flagDeriveKeyMaterial uint32 = 1 << 6
)

// iv is the BLAKE3 initial chaining value (the SHA-256 IV).
var iv = [8]uint32{
	0x6A09E667, 0xBB67AE85, 0x3C6EF372, 0xA54FF53A,
	0x510E527F, 0x9B05688C, 0x1F83D9AB, 0x5BE0CD19,
}

// Sum256 returns the 32-byte BLAKE3 digest of data.
func Sum256(data []byte) [32]byte {
	o := hashAll(data, iv, 0)
	var out [32]byte
	o.rootOutput(out[:])
	return out
}

// KeyedHash returns the 32-byte keyed BLAKE3 hash (MAC) of input under key.
func KeyedHash(key [keyLen]byte, input []byte) [32]byte {
	o := hashAll(input, keyWords(key[:]), flagKeyedHash)
	var out [32]byte
	o.rootOutput(out[:])
	return out
}

// DeriveKey derives a 32-byte subkey from keyMaterial, domain-separated by
// context. As with NewDeriveKey, context should be hardcoded, globally unique,
// and application-specific so that different applications cannot be tricked
// into using the same context string.
func DeriveKey(context string, keyMaterial []byte) [32]byte {
	o := hashAll(keyMaterial, deriveKeyIV(context), flagDeriveKeyMaterial)
	var out [32]byte
	o.rootOutput(out[:])
	return out
}

// gsc is the BLAKE3 quarter round on four named scalar words. Passing the four
// state words by value (rather than indexing a [16]uint32 array) lets the
// compiler keep them in registers.
func gsc(a, b, c, d, mx, my uint32) (uint32, uint32, uint32, uint32) {
	a = a + b + mx
	d = bits.RotateLeft32(d^a, -16)
	c = c + d
	b = bits.RotateLeft32(b^c, -12)
	a = a + b + my
	d = bits.RotateLeft32(d^a, -8)
	c = c + d
	b = bits.RotateLeft32(b^c, -7)
	return a, b, c, d
}

// compress runs the 7-round BLAKE3 compression and returns the 16-word state.
// The first 8 words are the chaining value; all 16 are used for output.
//
// The 16 state words and 16 message words are held as named locals and the 7
// rounds are fully unrolled with static message indices, so the compiler keeps
// everything in registers (the reference implementation does the same). This
// matters because compress is also the scalar kernel for the tree merge and the
// generic build.
func compress(cv *[8]uint32, block *[16]uint32, counter uint64, blkLen, flags uint32) [16]uint32 {
	m0 := block[0]
	m1 := block[1]
	m2 := block[2]
	m3 := block[3]
	m4 := block[4]
	m5 := block[5]
	m6 := block[6]
	m7 := block[7]
	m8 := block[8]
	m9 := block[9]
	m10 := block[10]
	m11 := block[11]
	m12 := block[12]
	m13 := block[13]
	m14 := block[14]
	m15 := block[15]

	v0, v1, v2, v3 := cv[0], cv[1], cv[2], cv[3]
	v4, v5, v6, v7 := cv[4], cv[5], cv[6], cv[7]
	v8, v9, v10, v11 := iv[0], iv[1], iv[2], iv[3]
	v12 := uint32(counter)
	v13 := uint32(counter >> 32)
	v14 := blkLen
	v15 := flags

	// 7 unrolled rounds; each line is one gsc with the static message schedule
	// index.
	// round 0
	v0, v4, v8, v12 = gsc(v0, v4, v8, v12, m0, m1)
	v1, v5, v9, v13 = gsc(v1, v5, v9, v13, m2, m3)
	v2, v6, v10, v14 = gsc(v2, v6, v10, v14, m4, m5)
	v3, v7, v11, v15 = gsc(v3, v7, v11, v15, m6, m7)
	v0, v5, v10, v15 = gsc(v0, v5, v10, v15, m8, m9)
	v1, v6, v11, v12 = gsc(v1, v6, v11, v12, m10, m11)
	v2, v7, v8, v13 = gsc(v2, v7, v8, v13, m12, m13)
	v3, v4, v9, v14 = gsc(v3, v4, v9, v14, m14, m15)
	// round 1
	v0, v4, v8, v12 = gsc(v0, v4, v8, v12, m2, m6)
	v1, v5, v9, v13 = gsc(v1, v5, v9, v13, m3, m10)
	v2, v6, v10, v14 = gsc(v2, v6, v10, v14, m7, m0)
	v3, v7, v11, v15 = gsc(v3, v7, v11, v15, m4, m13)
	v0, v5, v10, v15 = gsc(v0, v5, v10, v15, m1, m11)
	v1, v6, v11, v12 = gsc(v1, v6, v11, v12, m12, m5)
	v2, v7, v8, v13 = gsc(v2, v7, v8, v13, m9, m14)
	v3, v4, v9, v14 = gsc(v3, v4, v9, v14, m15, m8)
	// round 2
	v0, v4, v8, v12 = gsc(v0, v4, v8, v12, m3, m4)
	v1, v5, v9, v13 = gsc(v1, v5, v9, v13, m10, m12)
	v2, v6, v10, v14 = gsc(v2, v6, v10, v14, m13, m2)
	v3, v7, v11, v15 = gsc(v3, v7, v11, v15, m7, m14)
	v0, v5, v10, v15 = gsc(v0, v5, v10, v15, m6, m5)
	v1, v6, v11, v12 = gsc(v1, v6, v11, v12, m9, m0)
	v2, v7, v8, v13 = gsc(v2, v7, v8, v13, m11, m15)
	v3, v4, v9, v14 = gsc(v3, v4, v9, v14, m8, m1)
	// round 3
	v0, v4, v8, v12 = gsc(v0, v4, v8, v12, m10, m7)
	v1, v5, v9, v13 = gsc(v1, v5, v9, v13, m12, m9)
	v2, v6, v10, v14 = gsc(v2, v6, v10, v14, m14, m3)
	v3, v7, v11, v15 = gsc(v3, v7, v11, v15, m13, m15)
	v0, v5, v10, v15 = gsc(v0, v5, v10, v15, m4, m0)
	v1, v6, v11, v12 = gsc(v1, v6, v11, v12, m11, m2)
	v2, v7, v8, v13 = gsc(v2, v7, v8, v13, m5, m8)
	v3, v4, v9, v14 = gsc(v3, v4, v9, v14, m1, m6)
	// round 4
	v0, v4, v8, v12 = gsc(v0, v4, v8, v12, m12, m13)
	v1, v5, v9, v13 = gsc(v1, v5, v9, v13, m9, m11)
	v2, v6, v10, v14 = gsc(v2, v6, v10, v14, m15, m10)
	v3, v7, v11, v15 = gsc(v3, v7, v11, v15, m14, m8)
	v0, v5, v10, v15 = gsc(v0, v5, v10, v15, m7, m2)
	v1, v6, v11, v12 = gsc(v1, v6, v11, v12, m5, m3)
	v2, v7, v8, v13 = gsc(v2, v7, v8, v13, m0, m1)
	v3, v4, v9, v14 = gsc(v3, v4, v9, v14, m6, m4)
	// round 5
	v0, v4, v8, v12 = gsc(v0, v4, v8, v12, m9, m14)
	v1, v5, v9, v13 = gsc(v1, v5, v9, v13, m11, m5)
	v2, v6, v10, v14 = gsc(v2, v6, v10, v14, m8, m12)
	v3, v7, v11, v15 = gsc(v3, v7, v11, v15, m15, m1)
	v0, v5, v10, v15 = gsc(v0, v5, v10, v15, m13, m3)
	v1, v6, v11, v12 = gsc(v1, v6, v11, v12, m0, m10)
	v2, v7, v8, v13 = gsc(v2, v7, v8, v13, m2, m6)
	v3, v4, v9, v14 = gsc(v3, v4, v9, v14, m4, m7)
	// round 6
	v0, v4, v8, v12 = gsc(v0, v4, v8, v12, m11, m15)
	v1, v5, v9, v13 = gsc(v1, v5, v9, v13, m5, m0)
	v2, v6, v10, v14 = gsc(v2, v6, v10, v14, m1, m9)
	v3, v7, v11, v15 = gsc(v3, v7, v11, v15, m8, m6)
	v0, v5, v10, v15 = gsc(v0, v5, v10, v15, m14, m10)
	v1, v6, v11, v12 = gsc(v1, v6, v11, v12, m2, m12)
	v2, v7, v8, v13 = gsc(v2, v7, v8, v13, m3, m4)
	v3, v4, v9, v14 = gsc(v3, v4, v9, v14, m7, m13)

	// Final output XOR: state[i] ^= state[i+8], state[i+8] ^= cv[i].
	v0 ^= v8
	v1 ^= v9
	v2 ^= v10
	v3 ^= v11
	v4 ^= v12
	v5 ^= v13
	v6 ^= v14
	v7 ^= v15
	v8 ^= cv[0]
	v9 ^= cv[1]
	v10 ^= cv[2]
	v11 ^= cv[3]
	v12 ^= cv[4]
	v13 ^= cv[5]
	v14 ^= cv[6]
	v15 ^= cv[7]

	return [16]uint32{v0, v1, v2, v3, v4, v5, v6, v7, v8, v9, v10, v11, v12, v13, v14, v15}
}

func first8(s [16]uint32) [8]uint32 {
	return [8]uint32{s[0], s[1], s[2], s[3], s[4], s[5], s[6], s[7]}
}

// wordsFromLE decodes the little-endian bytes in b (zero-padded to 64 bytes)
// into 16 words.
func wordsFromLE(b []byte) [16]uint32 {
	var w [16]uint32
	var buf [blockLen]byte
	copy(buf[:], b)
	for i := range 16 {
		w[i] = binary.LittleEndian.Uint32(buf[i*4:])
	}
	return w
}

// outputFromBlock builds the output node for a single-block hash (len(data) <=
// blockLen) directly, without routing through the incremental chunk state. The
// block is decoded from data with the tail zero-padded, the initial chaining
// value is the mode key (no blocks compressed yet), and the flags carry both
// CHUNK_START and CHUNK_END since the single block is the whole chunk. This is
// the fast path for the small-input benchmarks.
func outputFromBlock(data []byte, key [8]uint32, flags uint32) output {
	var w [16]uint32
	for i := 0; i < len(data)/4; i++ {
		w[i] = binary.LittleEndian.Uint32(data[i*4:])
	}
	// Zero-fill the remaining words; partial final word handled below.
	for i := len(data) / 4; i < 16; i++ {
		w[i] = 0
	}
	// The final partial word, if any, is little-endian padded with zeros.
	if rem := len(data) % 4; rem != 0 {
		var last uint32
		base := len(data) - rem
		for i := 0; i < rem; i++ {
			last |= uint32(data[base+i]) << (8 * uint(i))
		}
		w[len(data)/4] = last
	}
	return output{
		inputCV:  key,
		block:    w,
		counter:  0,
		blockLen: uint32(len(data)),
		flags:    flags | flagChunkStart | flagChunkEnd,
	}
}

// stackCVs is the number of chunk chaining values processed per batch in the
// stack-buffered bulk paths (hashAll and Hasher.Write). Bounding the batch
// keeps the CV scratch buffer a small, stack-allocated array no matter how
// large the input is.
const stackCVs = 32

// xofBufLen is the length of the XofReader's cache buffer, one AVX-512 batch
// (16 output blocks) so extendable output uses the widest SIMD kernel where
// available; other backends process it in their own batch sizes.
const xofBufLen = 1024

// compressChunkCV computes the chaining value of the full 1024-byte chunk at
// index i (data[i*chunkLen:(i+1)*chunkLen]) with the given key and base flags.
// base is the BLAKE3 chunk counter of data[0]; the compression counter of chunk
// i is base+i. It is the scalar per-chunk kernel shared by the generic
// fillChunkCVs, the SIMD paths' remainder/fallback, and the chunk-level hashing
// in Write.
func compressChunkCV(data []byte, i int, base uint64, key [8]uint32, flags uint32) [8]uint32 {
	cs := newChunkState(key, base+uint64(i), flags)
	cs.update(data[i*chunkLen : (i+1)*chunkLen])
	return cs.output().chainingValue()
}

// fillChunkCVsScalar computes every chunk's chaining value with the scalar
// kernel. It is the default build's fillChunkCVs and the no-SIMD fallback for
// the SIMD builds.
func fillChunkCVsScalar(data []byte, cvs [][8]uint32, base uint64, key [8]uint32, flags uint32) {
	for i := range cvs {
		cvs[i] = compressChunkCV(data, i, base, key, flags)
	}
}

// fillChunkCVs computes the chaining value of each full 1024-byte chunk
// data[i*chunkLen:(i+1)*chunkLen] into cvs[i]. Its implementation is selected
// at build time: a pure-Go version (blake3_generic.go) and SIMD versions for
// amd64/arm64 under GOEXPERIMENT=simd (blake3_simd_*.go), gated at runtime on
// CPU features. All versions produce bit-identical results and allocate
// nothing (the caller supplies the scratch buffer).

// foldChunkCVs computes the chaining values of the whole chunks of data at
// chunk indices h.chunk.chunkCounter ..+whole-1 and folds each into the tree of
// h. data must hold exactly whole chunk-aligned chunks (its local chunk 0 is
// the global chunk h.chunk.chunkCounter). Processing in bounded batches keeps
// the CV scratch buffer a small stack-allocated array for arbitrarily large
// inputs; within each batch the complete subtrees are merged bottom-up with the
// SIMD parent kernel where available. After folding, h.chunk is reset to an
// empty chunk at the next index, ready to absorb the remainder.
func (h *Hasher) foldChunkCVs(data []byte, whole int) {
	start := h.chunk.chunkCounter
	for i := 0; i < whole; {
		m := min(whole-i, stackCVs)
		var buf [stackCVs][8]uint32
		fillChunkCVs(data[i*chunkLen:], buf[:m], start+uint64(i), h.key, h.flags)
		h.foldChunkCVRun(buf[:m], start+uint64(i))
		i += m
	}
	h.chunk = newChunkState(h.key, start+uint64(whole), h.flags)
}

// foldChunkCVRun folds len(cvs) consecutive chunk chaining values starting at
// global chunk counter start into the tree. The chunks are decomposed into
// complete binary subtrees aligned to the global counter (the left-balanced
// BLAKE3 tree); each subtree is merged bottom-up with mergeChunkCVs and pushed
// as a height-indexed subtree, so the only scalar parent compressions are the
// carry merges at subtree boundaries.
func (h *Hasher) foldChunkCVRun(cvs [][8]uint32, start uint64) {
	i := 0
	for i < len(cvs) {
		g := start + uint64(i)
		remaining := len(cvs) - i
		var hgt, size int
		if g == 0 {
			hgt = bits.Len(uint(remaining)) - 1
			size = 1 << uint(hgt)
		} else {
			hgt = bits.TrailingZeros64(g)
			if hgt > 62 {
				hgt = 62
			}
			size = 1 << uint(hgt)
			for size > remaining {
				size >>= 1
				hgt--
			}
		}
		cv := mergeChunkCVs(cvs[i:i+size], h.key, h.flags)
		h.pushSubtree(cv, hgt)
		i += size
	}
}

// mergeChunkCVs merges the leaf chaining values in cvs (a complete subtree, so
// len is a power of two) bottom-up into a single chaining value, using the SIMD
// parent kernel where available. It mutates cvs.
func mergeChunkCVs(cvs [][8]uint32, key [8]uint32, flags uint32) [8]uint32 {
	for len(cvs) > 1 {
		half := len(cvs) / 2
		mergeParentCVs(cvs[:half], cvs, key, flags)
		cvs = cvs[:half]
	}
	return cvs[0]
}

// putWords little-endian encodes the 16 compression output words into buf
// (which must be at least 64 bytes).
func putWords(buf []byte, words [16]uint32) {
	for i, w := range words {
		binary.LittleEndian.PutUint32(buf[i*4:], w)
	}
}

// compressOutputsScalar fills out with the extendable root output: each 64-byte
// block is a fresh ROOT compression with an incremented counter.
func compressOutputsScalar(out []byte, cv *[8]uint32, block *[16]uint32, blkLen, flags uint32, start uint64) {
	var buf [blockLen]byte
	for len(out) > 0 {
		words := compress(cv, block, start, blkLen, flags)
		putWords(buf[:], words)
		n := copy(out, buf[:])
		out = out[n:]
		start++
	}
}

// compressOutputs fills out with the extendable root output. Its implementation
// is selected at build time like fillChunkCVs; it processes whole output blocks
// in SIMD batches where available and falls back to compressOutputsScalar.

// hashAll computes the root output node for the whole input in one shot. For
// inputs larger than one chunk the independent per-chunk chaining values are
// computed (SIMD where available) and folded into the tree in bounded stack
// batches with SIMD subtree merges, so nothing allocates and the tree merge
// stays cheap. When the final chunk is exactly full it is hashed by the SIMD
// kernel too instead of the scalar incremental state. The result is
// byte-identical to the incremental Hasher.
func hashAll(data []byte, key [8]uint32, flags uint32) output {
	n := len(data)
	if n <= blockLen {
		return outputFromBlock(data, key, flags)
	}
	if n <= chunkLen {
		cs := newChunkState(key, 0, flags)
		cs.update(data)
		return cs.output()
	}
	nChunks := (n + chunkLen - 1) / chunkLen
	stack := Hasher{key: key, flags: flags, chunk: newChunkState(key, 0, flags)}

	var last output
	if n%chunkLen == 0 {
		// All chunks full. Hash every chunk with the SIMD kernel, fold the first
		// nChunks-1 into the tree, and build the last chunk's output node
		// directly from its CV after the first 15 blocks and its final block —
		// the inputCV of the CHUNK_END block — so no chunk is hashed twice.
		first := nChunks - 1
		stack.foldChunkCVs(data[:first*chunkLen], first)
		var cvs [1][8]uint32
		fillChunkCV15(data[first*chunkLen:], cvs[:], uint64(first), key, flags)
		last = output{
			inputCV:  cvs[0],
			block:    wordsFromLE(data[n-blockLen:]),
			counter:  uint64(first),
			blockLen: blockLen,
			flags:    flags | flagChunkEnd,
		}
	} else {
		stack.foldChunkCVs(data[:(nChunks-1)*chunkLen], nChunks-1)
		lastCS := newChunkState(key, uint64(nChunks-1), flags)
		lastCS.update(data[(nChunks-1)*chunkLen:])
		last = lastCS.output()
	}

	// Fold the occupied right-edge subtrees into the root node.
	for i := bits.TrailingZeros64(stack.stackLen); i < bits.Len64(stack.stackLen); i++ {
		if stack.hasSubtreeAtHeight(i) {
			last = parentOutput(stack.stack[i], last.chainingValue(), key, flags)
		}
	}
	return last
}

// output is a node (chunk or parent) ready to be finalized into a chaining
// value or into extendable root output bytes.
type output struct {
	inputCV  [8]uint32
	block    [16]uint32
	counter  uint64
	blockLen uint32
	flags    uint32
}

func (o output) chainingValue() [8]uint32 {
	return first8(compress(&o.inputCV, &o.block, o.counter, o.blockLen, o.flags))
}

// rootOutput fills out with the extendable root output (XOF). Each 64-byte
// output block uses a fresh ROOT compression with an incremented counter.
func (o output) rootOutput(out []byte) {
	compressOutputs(out, &o.inputCV, &o.block, o.blockLen, o.flags|flagRoot, 0)
}

// parentOutput builds the parent node merging two child chaining values. The
// input chaining value is the mode's key (iv in unkeyed mode, the key words in
// keyed / derive-key modes), and the mode's base flags are folded in alongside
// flagParent.
func parentOutput(left, right, key [8]uint32, flags uint32) output {
	var block [16]uint32
	copy(block[:8], left[:])
	copy(block[8:], right[:])
	return output{inputCV: key, block: block, blockLen: blockLen, flags: flags | flagParent}
}

// keyWords decodes a 32-byte key into the 8 little-endian words BLAKE3 uses as
// the initial chaining value in keyed and derive-key modes.
func keyWords(key []byte) [8]uint32 {
	var w [8]uint32
	for i := range 8 {
		w[i] = binary.LittleEndian.Uint32(key[i*4:])
	}
	return w
}

// deriveKeyIV computes the 32-byte context hash used as the initial chaining
// value (key) in key-derivation mode.
func deriveKeyIV(context string) [8]uint32 {
	ctx := Hasher{key: iv, flags: flagDeriveKeyContext, chunk: newChunkState(iv, 0, flagDeriveKeyContext)}
	ctx.Write([]byte(context))
	var contextKey [keyLen]byte
	ctx.finalize(contextKey[:])
	return keyWords(contextKey[:])
}

// chunkState accumulates one chunk (up to 1024 bytes) of input. flags carries
// the mode's base flags (0 unkeyed, flagKeyedHash, or flagDeriveKeyMaterial),
// folded into every block compression.
type chunkState struct {
	cv               [8]uint32
	chunkCounter     uint64
	flags            uint32
	block            [blockLen]byte
	blockLen         int
	blocksCompressed int
}

func newChunkState(key [8]uint32, counter uint64, flags uint32) chunkState {
	return chunkState{cv: key, chunkCounter: counter, flags: flags}
}

func (c *chunkState) len() int { return blockLen*c.blocksCompressed + c.blockLen }

func (c *chunkState) startFlag() uint32 {
	if c.blocksCompressed == 0 {
		return flagChunkStart
	}
	return 0
}

func (c *chunkState) update(input []byte) {
	for len(input) > 0 {
		// If the block buffer is full, compress it and clear it. More input is
		// coming, so this compression is not CHUNK_END.
		if c.blockLen == blockLen {
			w := wordsFromLE(c.block[:])
			c.cv = first8(compress(&c.cv, &w, c.chunkCounter, blockLen, c.flags|c.startFlag()))
			c.blocksCompressed++
			c.block = [blockLen]byte{}
			c.blockLen = 0
		}
		n := copy(c.block[c.blockLen:], input)
		c.blockLen += n
		input = input[n:]
	}
}

func (c *chunkState) output() output {
	return output{
		inputCV:  c.cv,
		block:    wordsFromLE(c.block[:]),
		counter:  c.chunkCounter,
		blockLen: uint32(c.blockLen),
		flags:    c.flags | c.startFlag() | flagChunkEnd,
	}
}

// Hasher is an incremental BLAKE3 hasher implementing hash.Hash. It is not
// safe for concurrent use. The zero value is not usable; obtain one from New,
// NewKeyed, or NewDeriveKey.
type Hasher struct {
	key      [8]uint32 // initial chaining value: iv (unkeyed) or the key words
	flags    uint32    // base flags: 0, flagKeyedHash, or flagDeriveKeyMaterial
	chunk    chunkState
	stack    [54][8]uint32 // space for 54 subtree chaining values: 2^54 * chunkLen = 2^64
	stackLen uint64        // bitmask: bit i set means a complete subtree of height i is in stack[i]
}

// ensure that Hasher implements hash.Hash at build time.
var _ hash.Hash = (*Hasher)(nil)

// newHasher builds a Hasher for the given mode (key = initial chaining value,
// flags = base domain-separation flags applied to every compression).
func newHasher(key [8]uint32, flags uint32) *Hasher {
	return &Hasher{key: key, flags: flags, chunk: newChunkState(key, 0, flags)}
}

// New returns a new streaming BLAKE3 hasher in the default (unkeyed) mode.
func New() *Hasher {
	return newHasher(iv, 0)
}

// NewKeyed returns a streaming BLAKE3 hasher in keyed mode (BLAKE3's keyed
// hashing / MAC construction). The 32-byte key replaces the IV as the initial
// chaining value and every compression carries the flagKeyedHash flag.
func NewKeyed(key [keyLen]byte) *Hasher {
	return newHasher(keyWords(key[:]), flagKeyedHash)
}

// NewDeriveKey returns a streaming BLAKE3 hasher in key-derivation mode for the
// given context string. Per the BLAKE3 spec this is a two-phase construction:
// the context string is first hashed with the flagDeriveKeyContext flag to
// obtain a 32-byte context key, which then becomes the initial chaining value
// for the key material, hashed with the flagDeriveKeyMaterial flag. Write the
// key material into the returned hasher; its (extendable) output is the derived
// key.
//
// The context string should be hardcoded, globally unique, and
// application-specific, as recommended by the BLAKE3 spec.
func NewDeriveKey(context string) *Hasher {
	return newHasher(deriveKeyIV(context), flagDeriveKeyMaterial)
}

// Size returns the default digest size in bytes (32).
func (h *Hasher) Size() int { return 32 }

// BlockSize returns the BLAKE3 block size in bytes (64).
func (h *Hasher) BlockSize() int { return blockLen }

// Reset restores the hasher to its initial state, preserving its mode (unkeyed,
// keyed, or derive-key).
func (h *Hasher) Reset() {
	h.chunk = newChunkState(h.key, 0, h.flags)
	h.stackLen = 0
}

func (h *Hasher) hasSubtreeAtHeight(i int) bool {
	return h.stackLen&(1<<uint(i)) != 0
}

// pushSubtree folds a complete subtree of the given height into the tree,
// merging it leftward with any already-complete subtrees of equal or greater
// height (the classic height-indexed stack of the reference implementation).
// A subtree of height h covers 2^h chunks.
func (h *Hasher) pushSubtree(cv [8]uint32, height int) {
	i := height
	for h.hasSubtreeAtHeight(i) {
		cv = parentCV(h.stack[i], cv, h.key, h.flags)
		i++
	}
	h.stack[i] = cv
	h.stackLen += 1 << uint(height)
}

// addChunkCV folds a single finished chunk's chaining value into the tree. It
// is a height-0 subtree push.
func (h *Hasher) addChunkCV(cv [8]uint32) {
	h.pushSubtree(cv, 0)
}

// parentCV compresses the parent node merging two child chaining values.
func parentCV(left, right, key [8]uint32, flags uint32) [8]uint32 {
	var block [16]uint32
	copy(block[:8], left[:])
	copy(block[8:], right[:])
	return first8(compress(&key, &block, 0, blockLen, flags|flagParent))
}

// flushChunk folds the completed current chunk into the tree and starts a
// fresh empty chunk for the next index.
func (h *Hasher) flushChunk() {
	cv := h.chunk.output().chainingValue()
	h.addChunkCV(cv)
	h.chunk = newChunkState(h.key, h.chunk.chunkCounter+1, h.flags)
}

// Write absorbs input into the hash state. It never returns an error and
// always consumes all of p.
//
// Once the current (partial) chunk is topped up, the remaining chunk-aligned
// input is hashed as whole chunks: the independent per-chunk chaining values
// are computed (SIMD where available) and folded into the tree in order, in
// bounded stack batches so nothing allocates. This keeps the streaming path as
// fast as the one-shot helpers.
func (h *Hasher) Write(p []byte) (int, error) {
	n := len(p)

	// Top up the current chunk so that the remainder of p is chunk-aligned.
	// If the chunk fills up, fold it into the tree.
	if h.chunk.len() != 0 {
		want := chunkLen - h.chunk.len()
		if want > len(p) {
			want = len(p)
		}
		h.chunk.update(p[:want])
		p = p[want:]
		if len(p) == 0 {
			return n, nil
		}
		h.flushChunk()
	}

	// Process whole chunks in bulk. h.chunk is empty here and the remaining p
	// starts exactly at chunk index h.chunk.chunkCounter. As in the reference
	// implementation, the final chunk is always left in the incremental state
	// (even when it is exactly full), so we process all but the last chunk.
	whole := (len(p) - 1) / chunkLen
	if whole > 0 {
		h.foldChunkCVs(p[:whole*chunkLen], whole)
		p = p[whole*chunkLen:]
	}

	// Buffer the trailing partial chunk, if any.
	h.chunk.update(p)
	return n, nil
}

// rootOutput computes the root output node without mutating the hasher.
// Starting with the output from the current chunk, the occupied right-edge
// subtrees (at the heights set in the stack bitmask) are folded in until the
// root output is reached.
func (h *Hasher) rootOutput() output {
	o := h.chunk.output()
	for i := bits.TrailingZeros64(h.stackLen); i < bits.Len64(h.stackLen); i++ {
		if h.hasSubtreeAtHeight(i) {
			o = parentOutput(h.stack[i], o.chainingValue(), h.key, h.flags)
		}
	}
	return o
}

// finalize computes the extendable output into out without mutating the
// hasher, so more data may still be written afterwards.
func (h *Hasher) finalize(out []byte) {
	h.rootOutput().rootOutput(out)
}

// Sum appends the 32-byte BLAKE3 digest of the data written so far to b.
func (h *Hasher) Sum(b []byte) []byte {
	var d [32]byte
	h.finalize(d[:])
	return append(b, d[:]...)
}
