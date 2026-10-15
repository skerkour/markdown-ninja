//go:build amd64

package chacha20

import (
	"simd/archsimd"
	"unsafe"
)

// Static arrays backing the VPROLVD count vectors and the VPERMI2D index
// vectors. They are plain data (no SIMD instruction at init, so the package
// initializes on CPUs without AVX-512/AVX2); the archsimd vectors are
// materialized lazily at use, i.e. only on the AVX-512 path, where the
// instructions are safe. The broadcast counts are kept in registers across the
// round loop (16 working vectors + 4 counts = 20 of the 32 ZMM registers,
// leaving room for temporaries, so the round loop does not spill).
var (
	rotlCounts = [4][16]uint32{
		{16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16},
		{12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12},
		{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8},
		{7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7},
	}

	// seq16 is the per-lane offset of the block counters within a batch of 16.
	seq16 = [16]uint32{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}

	// permuteABLanes02/13 gather the even/odd 128-bit lanes of the two sources,
	// and permuteHalvesLo/Hi select the lower/upper 256-bit halves of the two
	// sources. Indices 0-15 select the first source, 16-31 the second.
	permuteABLanes02 = [16]uint32{0, 1, 2, 3, 16, 17, 18, 19, 8, 9, 10, 11, 24, 25, 26, 27}
	permuteABLanes13 = [16]uint32{4, 5, 6, 7, 20, 21, 22, 23, 12, 13, 14, 15, 28, 29, 30, 31}
	permuteHalvesLo  = [16]uint32{0, 1, 2, 3, 4, 5, 6, 7, 16, 17, 18, 19, 20, 21, 22, 23}
	permuteHalvesHi  = [16]uint32{8, 9, 10, 11, 12, 13, 14, 15, 24, 25, 26, 27, 28, 29, 30, 31}
)

// xorKeyStreamAVX512 is the 16-block (1024 bytes) SIMD core. It XORs src with
// the key stream generated from state and maintains leftover key stream state.
//
// It processes 16 blocks per iteration. If the input is not a multiple of
// 1024 bytes, the final iteration produces the tail and any unused key stream
// of the last partial block is retained.
func (cipher *Cipher) xorKeyStreamAVX512(dst, src []byte) {
	// counter is kept as a 16-lane SIMD vector [c..c+15] and advanced in SIMD
	// per iteration, mirroring the AVX2 backend. For the DJB layout the
	// 64-bit counter spans words 12-13, so a scalar counter is kept and both
	// vectors are rebuilt from it each iteration.
	counterIncrement := archsimd.BroadcastUint32x16(16)
	counter := archsimd.BroadcastUint32x16(cipher.state[12]).Add(archsimd.LoadUint32x16Array(&seq16))

	// initial state
	i0 := archsimd.BroadcastUint32x16(cipher.state[0])
	i1 := archsimd.BroadcastUint32x16(cipher.state[1])
	i2 := archsimd.BroadcastUint32x16(cipher.state[2])
	i3 := archsimd.BroadcastUint32x16(cipher.state[3])
	i4 := archsimd.BroadcastUint32x16(cipher.state[4])
	i5 := archsimd.BroadcastUint32x16(cipher.state[5])
	i6 := archsimd.BroadcastUint32x16(cipher.state[6])
	i7 := archsimd.BroadcastUint32x16(cipher.state[7])
	i8 := archsimd.BroadcastUint32x16(cipher.state[8])
	i9 := archsimd.BroadcastUint32x16(cipher.state[9])
	i10 := archsimd.BroadcastUint32x16(cipher.state[10])
	i11 := archsimd.BroadcastUint32x16(cipher.state[11])
	i13 := archsimd.BroadcastUint32x16(cipher.state[13])
	i14 := archsimd.BroadcastUint32x16(cipher.state[14])
	i15 := archsimd.BroadcastUint32x16(cipher.state[15])

	var djbCounter uint64
	if cipher.djb {
		djbCounter = cipher.counter()
	}

	// len(dst) >= 1024 is not needed but it removes bound checks
	for len(src) >= 1024 && len(dst) >= 1024 {
		if cipher.djb {
			counter, i13 = djbCounterVectors512(djbCounter)
		}
		// 16 block-major vectors, block b in w[b]
		w0, w1, w2, w3, w4, w5, w6, w7, w8, w9, w10, w11, w12, w13, w14, w15 :=
			chacha16BlocksAVX512(i0, i1, i2, i3, i4, i5, i6, i7, i8, i9, i10, i11, counter, i13, i14, i15)

		srcPointer := unsafe.Pointer(&src[0])
		dstPointer := unsafe.Pointer(&dst[0])
		xorStoreBlock512(dstPointer, srcPointer, 0, w0)
		xorStoreBlock512(dstPointer, srcPointer, 64, w1)
		xorStoreBlock512(dstPointer, srcPointer, 128, w2)
		xorStoreBlock512(dstPointer, srcPointer, 192, w3)
		xorStoreBlock512(dstPointer, srcPointer, 256, w4)
		xorStoreBlock512(dstPointer, srcPointer, 320, w5)
		xorStoreBlock512(dstPointer, srcPointer, 384, w6)
		xorStoreBlock512(dstPointer, srcPointer, 448, w7)
		xorStoreBlock512(dstPointer, srcPointer, 512, w8)
		xorStoreBlock512(dstPointer, srcPointer, 576, w9)
		xorStoreBlock512(dstPointer, srcPointer, 640, w10)
		xorStoreBlock512(dstPointer, srcPointer, 704, w11)
		xorStoreBlock512(dstPointer, srcPointer, 768, w12)
		xorStoreBlock512(dstPointer, srcPointer, 832, w13)
		xorStoreBlock512(dstPointer, srcPointer, 896, w14)
		xorStoreBlock512(dstPointer, srcPointer, 960, w15)

		src = src[1024:]
		dst = dst[1024:]
		if cipher.djb {
			djbCounter += 16
		} else {
			counter = counter.Add(counterIncrement)
		}
	}

	if !cipher.djb {
		cipher.state[12] = counter.GetLo().GetLo().GetElem(0)
	}

	// Tail: < 1024 bytes. Generate 16 blocks of key stream into a local
	// buffer, XOR the consumed part, and keep the unused bytes of the last
	// partial block as leftover (<= 63).
	if len(src) > 0 {
		if cipher.djb {
			counter, i13 = djbCounterVectors512(djbCounter)
		}
		var keystream [1024]byte
		w0, w1, w2, w3, w4, w5, w6, w7, w8, w9, w10, w11, w12, w13, w14, w15 :=
			chacha16BlocksAVX512(i0, i1, i2, i3, i4, i5, i6, i7, i8, i9, i10, i11, counter, i13, i14, i15)

		ks := unsafe.Pointer(&keystream[0])
		storeBlock512(ks, 0, w0)
		storeBlock512(ks, 64, w1)
		storeBlock512(ks, 128, w2)
		storeBlock512(ks, 192, w3)
		storeBlock512(ks, 256, w4)
		storeBlock512(ks, 320, w5)
		storeBlock512(ks, 384, w6)
		storeBlock512(ks, 448, w7)
		storeBlock512(ks, 512, w8)
		storeBlock512(ks, 576, w9)
		storeBlock512(ks, 640, w10)
		storeBlock512(ks, 704, w11)
		storeBlock512(ks, 768, w12)
		storeBlock512(ks, 832, w13)
		storeBlock512(ks, 896, w14)
		storeBlock512(ks, 960, w15)

		cipher.processTail(dst, src, keystream[:], djbCounter)
		return
	}

	if cipher.djb {
		cipher.state[12] = uint32(djbCounter)
		cipher.state[13] = uint32(djbCounter >> 32)
	}
	cipher.leftoverLen = 0
}

// djbCounterVectors512 builds the word-12 (counter low) and word-13 (counter
// high) vectors for blocks c..c+15 in the DJB layout. The low words are
// affine in the lane index (broadcast + add); the high words are the broadcast
// high word plus 1 on the lanes whose low word wrapped past 2^32, detected
// with the unsigned VPCMPUD compare.
func djbCounterVectors512(c uint64) (loV, hiV archsimd.Uint32x16) {
	loBase := archsimd.BroadcastUint32x16(uint32(c))
	loV = loBase.Add(archsimd.LoadUint32x16Array(&seq16))
	hiV = archsimd.BroadcastUint32x16(uint32(c >> 32))
	// lanes where lo(c)+i wrapped: loBase > loV (unsigned). Mask32x16
	// ToInt32x16 sets all bits where set, so And with 1 yields the carry.
	carry := loBase.Greater(loV).ToInt32x16().AsUint32x16().And(archsimd.BroadcastUint32x16(1))
	hiV = hiV.Add(carry)
	return
}

// chacha16BlocksAVX512 computes 16 blocks of ChaCha20 key stream (20 rounds +
// add-back) and transposes them into 16 block-major vectors: w[b] holds the
// 16 words (64 bytes) of block b.
//
// The 16 initial-state vectors are passed in and spilled to the stack frame at
// entry (all 16 ZMM registers are needed by the working state), then reloaded
// at the add-back. The round loop itself spills nothing.
func chacha16BlocksAVX512(i0, i1, i2, i3, i4, i5, i6, i7, i8, i9, i10, i11, i12, i13, i14, i15 archsimd.Uint32x16) (w0, w1, w2, w3, w4, w5, w6, w7, w8, w9, w10, w11, w12, w13, w14, w15 archsimd.Uint32x16) {
	// working state
	w0, w1, w2, w3, w4, w5, w6, w7, w8, w9, w10, w11, w12, w13, w14, w15 =
		i0, i1, i2, i3, i4, i5, i6, i7, i8, i9, i10, i11, i12, i13, i14, i15

	// VPROLVD count vectors loaded once and kept in registers
	// across the round loop.
	rotl16 := archsimd.LoadUint32x16Array(&rotlCounts[0])
	rotl12 := archsimd.LoadUint32x16Array(&rotlCounts[1])
	rotl8 := archsimd.LoadUint32x16Array(&rotlCounts[2])
	rotl7 := archsimd.LoadUint32x16Array(&rotlCounts[3])

	for r := 0; r < 10; r++ {
		w0, w4, w8, w12 = quarterRoundAVX512(rotl16, rotl12, rotl8, rotl7, w0, w4, w8, w12)
		w1, w5, w9, w13 = quarterRoundAVX512(rotl16, rotl12, rotl8, rotl7, w1, w5, w9, w13)
		w2, w6, w10, w14 = quarterRoundAVX512(rotl16, rotl12, rotl8, rotl7, w2, w6, w10, w14)
		w3, w7, w11, w15 = quarterRoundAVX512(rotl16, rotl12, rotl8, rotl7, w3, w7, w11, w15)

		w0, w5, w10, w15 = quarterRoundAVX512(rotl16, rotl12, rotl8, rotl7, w0, w5, w10, w15)
		w1, w6, w11, w12 = quarterRoundAVX512(rotl16, rotl12, rotl8, rotl7, w1, w6, w11, w12)
		w2, w7, w8, w13 = quarterRoundAVX512(rotl16, rotl12, rotl8, rotl7, w2, w7, w8, w13)
		w3, w4, w9, w14 = quarterRoundAVX512(rotl16, rotl12, rotl8, rotl7, w3, w4, w9, w14)
	}

	// add back
	w0 = w0.Add(i0)
	w1 = w1.Add(i1)
	w2 = w2.Add(i2)
	w3 = w3.Add(i3)
	w4 = w4.Add(i4)
	w5 = w5.Add(i5)
	w6 = w6.Add(i6)
	w7 = w7.Add(i7)
	w8 = w8.Add(i8)
	w9 = w9.Add(i9)
	w10 = w10.Add(i10)
	w11 = w11.Add(i11)
	w12 = w12.Add(i12)
	w13 = w13.Add(i13)
	w14 = w14.Add(i14)
	w15 = w15.Add(i15)

	// transpose the word-major result into block-major vectors, in place.
	// stage 1: within-lane transpose per word group
	w0, w1, w2, w3 = transpose4AVX512(w0, w1, w2, w3)         // words 0-3
	w4, w5, w6, w7 = transpose4AVX512(w4, w5, w6, w7)         // words 4-7
	w8, w9, w10, w11 = transpose4AVX512(w8, w9, w10, w11)     // words 8-11
	w12, w13, w14, w15 = transpose4AVX512(w12, w13, w14, w15) // words 12-15

	// stage 2: cross-lane transpose. the k-th call gathers the four word groups
	// (stage-1 vectors w[k], w[k+4], w[k+8], w[k+12]) of blocks {k, k+4, k+8,
	// k+12} into the full 64 bytes of those blocks.
	w0, w4, w8, w12 = transpose4LanesAVX512(w0, w4, w8, w12)
	w1, w5, w9, w13 = transpose4LanesAVX512(w1, w5, w9, w13)
	w2, w6, w10, w14 = transpose4LanesAVX512(w2, w6, w10, w14)
	w3, w7, w11, w15 = transpose4LanesAVX512(w3, w7, w11, w15)
	return
}

// quarterRoundAVX512 is the ChaCha20 quarter round on the word-major layout.
// Each rotation is a single VPROLVD against a broadcast count vector, which
// needs no scratch register; with 16 working words + 4 count vectors in 20 of
// the 32 ZMM registers the round loop spills nothing. rotl16/rotl12/rotl8/
// rotl7 are the loop-invariant count vectors.
func quarterRoundAVX512(rotl16, rotl12, rotl8, rotl7, a, b, c, d archsimd.Uint32x16) (archsimd.Uint32x16, archsimd.Uint32x16, archsimd.Uint32x16, archsimd.Uint32x16) {
	// a += b; d ^= a; d <<<= 16
	a = a.Add(b)
	d = d.Xor(a)
	d = d.RotateLeft(rotl16)

	// c += d; b ^= c; b <<<= 12
	c = c.Add(d)
	b = b.Xor(c)
	b = b.RotateLeft(rotl12)

	// a += b; d ^= a; d <<<= 8
	a = a.Add(b)
	d = d.Xor(a)
	d = d.RotateLeft(rotl8)

	// c += d; b ^= c; b <<<= 7
	c = c.Add(d)
	b = b.Xor(c)
	b = b.RotateLeft(rotl7)

	return a, b, c, d
}

// transpose4AVX512 transposes one word group of 4 word-major vectors (each
// holding word 4g+j of the 16 blocks, one block per lane) into 4 vectors
// holding 4 consecutive words of each block, within each 128-bit lane.
// r[b] holds words 4g..4g+3 of block (4L+b) in lane L, for b, L in 0..3.
func transpose4AVX512(a, b, c, d archsimd.Uint32x16) (archsimd.Uint32x16, archsimd.Uint32x16, archsimd.Uint32x16, archsimd.Uint32x16) {
	t0 := a.InterleaveLoGrouped(b)
	t1 := a.InterleaveHiGrouped(b)
	t2 := c.InterleaveLoGrouped(d)
	t3 := c.InterleaveHiGrouped(d)
	return t0.ConcatPermuteScalarsGrouped(0, 1, 4, 5, t2),
		t0.ConcatPermuteScalarsGrouped(2, 3, 6, 7, t2),
		t1.ConcatPermuteScalarsGrouped(0, 1, 4, 5, t3),
		t1.ConcatPermuteScalarsGrouped(2, 3, 6, 7, t3)
}

// transpose4LanesAVX512 is the cross-lane stage of the output transpose. Each
// input holds, in its four 128-bit lanes, words of blocks {0,4,8,12}+m in
// lanes 0..3; it returns the 4x4 transpose at 128-bit granularity, so out[L]
// holds the 64 bytes of block 4L+m in word-group order.
func transpose4LanesAVX512(a, b, c, d archsimd.Uint32x16) (archsimd.Uint32x16, archsimd.Uint32x16, archsimd.Uint32x16, archsimd.Uint32x16) {
	lanes02 := archsimd.LoadUint32x16Array(&permuteABLanes02)
	lanes13 := archsimd.LoadUint32x16Array(&permuteABLanes13)
	halvesLo := archsimd.LoadUint32x16Array(&permuteHalvesLo)
	halvesHi := archsimd.LoadUint32x16Array(&permuteHalvesHi)

	ab02 := a.ConcatPermute(b, lanes02)        // [a.l0, b.l0, a.l2, b.l2]
	ab13 := a.ConcatPermute(b, lanes13)        // [a.l1, b.l1, a.l3, b.l3]
	cd02 := c.ConcatPermute(d, lanes02)        // [c.l0, d.l0, c.l2, d.l2]
	cd13 := c.ConcatPermute(d, lanes13)        // [c.l1, d.l1, c.l3, d.l3]
	return ab02.ConcatPermute(cd02, halvesLo), // [a.l0, b.l0, c.l0, d.l0]
		ab13.ConcatPermute(cd13, halvesLo), // [a.l1, b.l1, c.l1, d.l1]
		ab02.ConcatPermute(cd02, halvesHi), // [a.l2, b.l2, c.l2, d.l2]
		ab13.ConcatPermute(cd13, halvesHi) // [a.l3, b.l3, c.l3, d.l3]
}

// xorStoreBlock512 XORs 64 bytes of key stream (block b, held in v) with
// src[off:off+64] and stores the result into dst[off:off+64].
func xorStoreBlock512(dst, src unsafe.Pointer, offset int, v archsimd.Uint32x16) {
	v.Xor(archsimd.LoadUint32x16Array((*[16]uint32)(unsafe.Add(src, offset)))).StoreArray((*[16]uint32)(unsafe.Add(dst, offset)))
}

// storeBlock512 stores 64 bytes of key stream (block b, held in v) into
// ks[off:off+64].
func storeBlock512(ks unsafe.Pointer, offset int, v archsimd.Uint32x16) {
	v.StoreArray((*[16]uint32)(unsafe.Add(ks, offset)))
}
