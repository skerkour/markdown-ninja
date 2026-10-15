package chacha20

import (
	"simd/archsimd"
	"unsafe"
)

// rotl16Idx and rotl8Idx are the VPSHUFB index vectors that rotate each 32-bit
// element of a Uint8x32 by 16/8 bits. They are kept as scalar data (not vector
// values, per the archsimd guidance against storing SIMD types in aggregates)
// and materialized as loop-invariant vector locals in xorKeyStreamAVX2.
var (
	rotl16Idx = [32]int8{
		2, 3, 0, 1, 6, 7, 4, 5, 10, 11, 8, 9, 14, 15, 12, 13,
		2, 3, 0, 1, 6, 7, 4, 5, 10, 11, 8, 9, 14, 15, 12, 13,
	}
	rotl8Idx = [32]int8{
		3, 0, 1, 2, 7, 4, 5, 6, 11, 8, 9, 10, 15, 12, 13, 14,
		3, 0, 1, 2, 7, 4, 5, 6, 11, 8, 9, 10, 15, 12, 13, 14,
	}

	// counterOffsets[k] is added to the counter word (element 0 of each 128-bit
	// lane of a column's words-12-15 vector) to yield the block counters 2k and
	// 2k+1 within a batch of 8. The base column 0 vector already holds c in the
	// low lane and c+1 in the high lane, so the other columns add 2k to both
	// lanes (mirroring two_AVX2 in chachaAVX2_amd64.s).
	counterOffsets = [4][8]uint32{
		{0, 0, 0, 0, 1, 0, 0, 0},
		{2, 0, 0, 0, 2, 0, 0, 0},
		{4, 0, 0, 0, 4, 0, 0, 0},
		{6, 0, 0, 0, 6, 0, 0, 0},
	}

	// counterIncrement advances all four column counters (word 12, element 0 of
	// each 128-bit lane) by one batch of 8 blocks.
	counterIncrement = [8]uint32{8, 0, 0, 0, 8, 0, 0, 0}

	// djbCntMask zeroes the nonce positions (elements 2,3,6,7) of a counter
	// vector before ORing in the nonce words.
	djbCntMask = [8]uint32{0xffffffff, 0xffffffff, 0, 0, 0xffffffff, 0xffffffff, 0, 0}
	// djbPermLo/djbPermHi are the VPERMD indices that pull the lo/hi counter
	// words of blocks {2k,2k+1} into elements 0/1 and 4/5 of each column
	// vector (from the reshaped [lo,hi,lo,hi,...] of 4 consecutive counters).
	djbPermLo = [8]uint32{0, 1, 0, 0, 2, 3, 0, 0}
	djbPermHi = [8]uint32{4, 5, 0, 0, 6, 7, 0, 0}
	// djbSeq4 offsets the four 64-bit counter lanes within a batch of 8.
	djbSeq4 = [4]uint64{0, 1, 2, 3}
)

// xorKeyStream is the amd64 SIMD backend hook. It XORs src with the key
// stream generated from state and maintains leftover key stream state.
//
// The 512-bit AVX-512 path is used when the CPU supports it, then the 256-bit
// AVX2 path when the CPU supports AVX2; otherwise the portable scalar backend
// runs.
func (cipher *Cipher) xorKeyStream(dst, src []byte) {
	switch {
	case archsimd.X86.AVX512() && len(src) > 256:
		cipher.xorKeyStreamAVX512(dst, src)
	case archsimd.X86.AVX2() && len(src) > 64:
		cipher.xorKeyStreamAVX2(dst, src)
	default:
		cipher.xorKeyStreamScalar(dst, src)
	}
}

// xorKeyStreamAVX2 is the 8-block SIMD core. It XORs src with the key stream
// generated from state and maintains leftover key stream state.
//
// It processes 8 blocks (512 bytes) per iteration using the column-major
// layout of chachaAVX2_amd64.s: each column k holds the 16 words of two
// blocks (block 2k in the low 128-bit lanes, block 2k+1 in the high lanes)
// across four Uint32x8 vectors, so one vectorized quarter round operates on
// four ChaCha columns at once. The 16/8-bit rotations are single VPSHUFB
// shuffles and the output transpose is fused into the load/xor/store. If the
// input is not a multiple of 512 bytes, the final iteration produces the tail
// and any unused key stream of the last partial block is retained.
//
// For the IETF layout the block counters live in word 12 and the c0w3..c3w3
// column vectors are advanced with a single vector add per iteration. For the
// DJB layout the 64-bit counter spans words 12-13, so a scalar counter is kept
// and the four column vectors are rebuilt from it each iteration.
func (cipher *Cipher) xorKeyStreamAVX2(dst, src []byte) {
	// initial state in column-major layout. s01/s23 are the two halves of the
	// 64-byte state; each column duplicates one 4-word group across its two
	// 128-bit lanes.
	s01 := archsimd.LoadUint32x8Array(&[8]uint32{
		cipher.state[0], cipher.state[1], cipher.state[2], cipher.state[3],
		cipher.state[4], cipher.state[5], cipher.state[6], cipher.state[7],
	})
	s23 := archsimd.LoadUint32x8Array(&[8]uint32{
		cipher.state[8], cipher.state[9], cipher.state[10], cipher.state[11],
		cipher.state[12], cipher.state[13], cipher.state[14], cipher.state[15],
	})

	c0w0 := s01.ConcatPermute128Scalars(0, 0, s01)
	c0w1 := s01.ConcatPermute128Scalars(1, 1, s01)
	c0w2 := s23.ConcatPermute128Scalars(0, 0, s23)

	// VPSHUFB rotation masks and the per-iteration counter increment, all
	// loop-invariant.
	rotl16 := archsimd.LoadInt8x32Array(&rotl16Idx)
	rotl8 := archsimd.LoadInt8x32Array(&rotl8Idx)
	counterInc := archsimd.LoadUint32x8Array(&counterIncrement)

	// The words-12-15 column vectors. For the IETF layout they are derived once
	// from s23 and advanced by a vector add per iteration; for the DJB layout
	// they are rebuilt from a scalar 64-bit counter each iteration.
	var (
		c0w3, c1w3, c2w3, c3w3 archsimd.Uint32x8
		c1w0, c1w1, c1w2       archsimd.Uint32x8
		c2w0, c2w1, c2w2       archsimd.Uint32x8
		c3w0, c3w1, c3w2       archsimd.Uint32x8
		djbCounter             uint64
		seq4v                  archsimd.Uint64x4
		one4v                  archsimd.Uint64x4
		nzVec                  archsimd.Uint32x8
		cntMask                archsimd.Uint32x8
		permLo                 archsimd.Uint32x8
		permHi                 archsimd.Uint32x8
	)
	if cipher.djb {
		djbCounter = cipher.counter()
		seq4v = archsimd.LoadUint64x4Array(&djbSeq4)
		one4v = archsimd.BroadcastUint64x4(4)
		nzVec = archsimd.LoadUint32x8Array(&[8]uint32{0, 0, cipher.state[14], cipher.state[15], 0, 0, cipher.state[14], cipher.state[15]})
		cntMask = archsimd.LoadUint32x8Array(&djbCntMask)
		permLo = archsimd.LoadUint32x8Array(&djbPermLo)
		permHi = archsimd.LoadUint32x8Array(&djbPermHi)
		c0w3, c1w3, c2w3, c3w3 = djbCounterColumnsAVX2Simd(djbCounter, seq4v, one4v, nzVec, cntMask, permLo, permHi)
		c1w0, c1w1, c1w2 = c0w0, c0w1, c0w2
		c2w0, c2w1, c2w2 = c0w0, c0w1, c0w2
		c3w0, c3w1, c3w2 = c0w0, c0w1, c0w2
	} else {
		c0w3 = s23.ConcatPermute128Scalars(1, 1, s23).Add(archsimd.LoadUint32x8Array(&counterOffsets[0]))
		c1w0, c1w1, c1w2 = c0w0, c0w1, c0w2
		c1w3 = c0w3.Add(archsimd.LoadUint32x8Array(&counterOffsets[1]))
		c2w0, c2w1, c2w2 = c0w0, c0w1, c0w2
		c2w3 = c0w3.Add(archsimd.LoadUint32x8Array(&counterOffsets[2]))
		c3w0, c3w1, c3w2 = c0w0, c0w1, c0w2
		c3w3 = c0w3.Add(archsimd.LoadUint32x8Array(&counterOffsets[3]))
	}

	// len(dst) >= 512 is not needed but it removes bound checks
	for len(src) >= 512 && len(dst) >= 512 {
		if cipher.djb {
			c0w3, c1w3, c2w3, c3w3 = djbCounterColumnsAVX2Simd(djbCounter, seq4v, one4v, nzVec, cntMask, permLo, permHi)
		}
		o0w0, o0w1, o0w2, o0w3, o1w0, o1w1, o1w2, o1w3, o2w0, o2w1, o2w2, o2w3, o3w0, o3w1, o3w2, o3w3 :=
			chacha8ColumnsAVX2(c0w0, c0w1, c0w2, c0w3, c1w0, c1w1, c1w2, c1w3, c2w0, c2w1, c2w2, c2w3, c3w0, c3w1, c3w2, c3w3, rotl16, rotl8)

		srcPointer := unsafe.Pointer(&src[0])
		dstPointer := unsafe.Pointer(&dst[0])

		// each column outputs two 64-byte blocks: block 2k and 2k+1
		xorStoreCol(dstPointer, srcPointer, 0, o0w0, o0w1, o0w2, o0w3)
		xorStoreCol(dstPointer, srcPointer, 128, o1w0, o1w1, o1w2, o1w3)
		xorStoreCol(dstPointer, srcPointer, 256, o2w0, o2w1, o2w2, o2w3)
		xorStoreCol(dstPointer, srcPointer, 384, o3w0, o3w1, o3w2, o3w3)

		src = src[512:]
		dst = dst[512:]
		if cipher.djb {
			djbCounter += 8
		} else {
			c0w3 = c0w3.Add(counterInc)
			c1w3 = c1w3.Add(counterInc)
			c2w3 = c2w3.Add(counterInc)
			c3w3 = c3w3.Add(counterInc)
		}
	}

	if !cipher.djb {
		cipher.state[12] = c0w3.GetLo().GetElem(0)
	}

	// Tail: < 512 bytes. Generate 8 blocks of key stream into a local
	// buffer, XOR the consumed part, and keep the unused bytes of the last
	// partial block as leftover (<= 63).
	if len(src) > 0 {
		if cipher.djb {
			c0w3, c1w3, c2w3, c3w3 = djbCounterColumnsAVX2Simd(djbCounter, seq4v, one4v, nzVec, cntMask, permLo, permHi)
		}
		var keystream [512]byte
		o0w0, o0w1, o0w2, o0w3, o1w0, o1w1, o1w2, o1w3, o2w0, o2w1, o2w2, o2w3, o3w0, o3w1, o3w2, o3w3 :=
			chacha8ColumnsAVX2(c0w0, c0w1, c0w2, c0w3, c1w0, c1w1, c1w2, c1w3, c2w0, c2w1, c2w2, c2w3, c3w0, c3w1, c3w2, c3w3, rotl16, rotl8)

		ks := unsafe.Pointer(&keystream[0])
		storeCol(ks, 0, o0w0, o0w1, o0w2, o0w3)
		storeCol(ks, 128, o1w0, o1w1, o1w2, o1w3)
		storeCol(ks, 256, o2w0, o2w1, o2w2, o2w3)
		storeCol(ks, 384, o3w0, o3w1, o3w2, o3w3)

		cipher.processTail(dst, src, keystream[:], djbCounter)
		return
	}

	if cipher.djb {
		cipher.state[12] = uint32(djbCounter)
		cipher.state[13] = uint32(djbCounter >> 32)
	}
	cipher.leftoverLen = 0
}

// djbCounterColumnsAVX2Simd builds the four column-major words-12-15 vectors
// (c0w3..c3w3) for blocks c..c+7 in the DJB layout using only AVX2 ops. The
// eight 64-bit counters are computed in SIMD as two Uint64x4 vectors and
// reshaped to [lo,hi,lo,hi,...]; each column then selects its two lo/hi word
// pairs with a VPERMD (Permute), zeroes the nonce positions, and ORs in the
// nonce words.
func djbCounterColumnsAVX2Simd(c uint64, seq4v, one4v archsimd.Uint64x4, nzVec, cntMask, permLo, permHi archsimd.Uint32x8) (c0w3, c1w3, c2w3, c3w3 archsimd.Uint32x8) {
	v0 := archsimd.BroadcastUint64x4(c).Add(seq4v) // [c, c+1, c+2, c+3]
	v1 := v0.Add(one4v)                            // [c+4, c+5, c+6, c+7]
	r0 := v0.ReshapeToUint32s()                    // [lo0,hi0,lo1,hi1, lo2,hi2,lo3,hi3]
	r1 := v1.ReshapeToUint32s()                    // [lo4,hi4,lo5,hi5, lo6,hi6,lo7,hi7]
	c0w3 = r0.Permute(permLo).And(cntMask).Or(nzVec)
	c1w3 = r0.Permute(permHi).And(cntMask).Or(nzVec)
	c2w3 = r1.Permute(permLo).And(cntMask).Or(nzVec)
	c3w3 = r1.Permute(permHi).And(cntMask).Or(nzVec)
	return
}

// chacha8ColumnsAVX2 computes 8 blocks of ChaCha20 key stream (20 rounds +
// add-back) on the column-major layout. The 16 initial column vectors are
// passed in and the compiler keeps them in the stack frame across the round
// loop, reloading them at the add-back. rotl16/rotl8 are the VPSHUFB rotation
// masks. The returned 16 vectors hold the add-back results in the same
// column-major layout.
func chacha8ColumnsAVX2(i0w0, i0w1, i0w2, i0w3, i1w0, i1w1, i1w2, i1w3, i2w0, i2w1, i2w2, i2w3, i3w0, i3w1, i3w2, i3w3 archsimd.Uint32x8, rotl16, rotl8 archsimd.Int8x32) (o0w0, o0w1, o0w2, o0w3, o1w0, o1w1, o1w2, o1w3, o2w0, o2w1, o2w2, o2w3, o3w0, o3w1, o3w2, o3w3 archsimd.Uint32x8) {
	// working state
	w0w0, w0w1, w0w2, w0w3, w1w0, w1w1, w1w2, w1w3, w2w0, w2w1, w2w2, w2w3, w3w0, w3w1, w3w2, w3w3 :=
		i0w0, i0w1, i0w2, i0w3, i1w0, i1w1, i1w2, i1w3, i2w0, i2w1, i2w2, i2w3, i3w0, i3w1, i3w2, i3w3

	for r := 0; r < 10; r++ {
		// column quarter rounds
		w0w0, w0w1, w0w2, w0w3 = quarterRoundCol(w0w0, w0w1, w0w2, w0w3, rotl16, rotl8)
		w1w0, w1w1, w1w2, w1w3 = quarterRoundCol(w1w0, w1w1, w1w2, w1w3, rotl16, rotl8)
		w2w0, w2w1, w2w2, w2w3 = quarterRoundCol(w2w0, w2w1, w2w2, w2w3, rotl16, rotl8)
		w3w0, w3w1, w3w2, w3w3 = quarterRoundCol(w3w0, w3w1, w3w2, w3w3, rotl16, rotl8)

		// rotate the rows so the next quarter rounds hit the diagonals
		w0w1 = w0w1.ConcatPermuteScalarsGrouped(1, 2, 3, 0, w0w1)
		w0w2 = w0w2.ConcatPermuteScalarsGrouped(2, 3, 0, 1, w0w2)
		w0w3 = w0w3.ConcatPermuteScalarsGrouped(3, 0, 1, 2, w0w3)
		w1w1 = w1w1.ConcatPermuteScalarsGrouped(1, 2, 3, 0, w1w1)
		w1w2 = w1w2.ConcatPermuteScalarsGrouped(2, 3, 0, 1, w1w2)
		w1w3 = w1w3.ConcatPermuteScalarsGrouped(3, 0, 1, 2, w1w3)
		w2w1 = w2w1.ConcatPermuteScalarsGrouped(1, 2, 3, 0, w2w1)
		w2w2 = w2w2.ConcatPermuteScalarsGrouped(2, 3, 0, 1, w2w2)
		w2w3 = w2w3.ConcatPermuteScalarsGrouped(3, 0, 1, 2, w2w3)
		w3w1 = w3w1.ConcatPermuteScalarsGrouped(1, 2, 3, 0, w3w1)
		w3w2 = w3w2.ConcatPermuteScalarsGrouped(2, 3, 0, 1, w3w2)
		w3w3 = w3w3.ConcatPermuteScalarsGrouped(3, 0, 1, 2, w3w3)

		// diagonal quarter rounds
		w0w0, w0w1, w0w2, w0w3 = quarterRoundCol(w0w0, w0w1, w0w2, w0w3, rotl16, rotl8)
		w1w0, w1w1, w1w2, w1w3 = quarterRoundCol(w1w0, w1w1, w1w2, w1w3, rotl16, rotl8)
		w2w0, w2w1, w2w2, w2w3 = quarterRoundCol(w2w0, w2w1, w2w2, w2w3, rotl16, rotl8)
		w3w0, w3w1, w3w2, w3w3 = quarterRoundCol(w3w0, w3w1, w3w2, w3w3, rotl16, rotl8)

		// inverse row rotation
		w0w1 = w0w1.ConcatPermuteScalarsGrouped(3, 0, 1, 2, w0w1)
		w0w2 = w0w2.ConcatPermuteScalarsGrouped(2, 3, 0, 1, w0w2)
		w0w3 = w0w3.ConcatPermuteScalarsGrouped(1, 2, 3, 0, w0w3)
		w1w1 = w1w1.ConcatPermuteScalarsGrouped(3, 0, 1, 2, w1w1)
		w1w2 = w1w2.ConcatPermuteScalarsGrouped(2, 3, 0, 1, w1w2)
		w1w3 = w1w3.ConcatPermuteScalarsGrouped(1, 2, 3, 0, w1w3)
		w2w1 = w2w1.ConcatPermuteScalarsGrouped(3, 0, 1, 2, w2w1)
		w2w2 = w2w2.ConcatPermuteScalarsGrouped(2, 3, 0, 1, w2w2)
		w2w3 = w2w3.ConcatPermuteScalarsGrouped(1, 2, 3, 0, w2w3)
		w3w1 = w3w1.ConcatPermuteScalarsGrouped(3, 0, 1, 2, w3w1)
		w3w2 = w3w2.ConcatPermuteScalarsGrouped(2, 3, 0, 1, w3w2)
		w3w3 = w3w3.ConcatPermuteScalarsGrouped(1, 2, 3, 0, w3w3)
	}

	// add-back
	o0w0 = w0w0.Add(i0w0)
	o0w1 = w0w1.Add(i0w1)
	o0w2 = w0w2.Add(i0w2)
	o0w3 = w0w3.Add(i0w3)
	o1w0 = w1w0.Add(i1w0)
	o1w1 = w1w1.Add(i1w1)
	o1w2 = w1w2.Add(i1w2)
	o1w3 = w1w3.Add(i1w3)
	o2w0 = w2w0.Add(i2w0)
	o2w1 = w2w1.Add(i2w1)
	o2w2 = w2w2.Add(i2w2)
	o2w3 = w2w3.Add(i2w3)
	o3w0 = w3w0.Add(i3w0)
	o3w1 = w3w1.Add(i3w1)
	o3w2 = w3w2.Add(i3w2)
	o3w3 = w3w3.Add(i3w3)
	return
}

// quarterRoundCol is the ChaCha20 quarter round on the column-major layout.
// Each 32-bit lane of the four vectors is one ChaCha column, so one call
// performs four quarter rounds. The 16/8-bit rotations are single VPSHUFB
// byte shuffles (the same trick as chachaAVX2_amd64.s); the 12/7-bit rotations
// use two shifts and an or, as AVX2 has no variable rotate.
func quarterRoundCol(a, b, c, d archsimd.Uint32x8, rotl16, rotl8 archsimd.Int8x32) (archsimd.Uint32x8, archsimd.Uint32x8, archsimd.Uint32x8, archsimd.Uint32x8) {
	// a += b; d ^= a; d <<<= 16
	a = a.Add(b)
	d = d.Xor(a)
	d = d.ReshapeToUint8s().PermuteOrZeroGrouped(rotl16).ReshapeToUint32s()

	// c += d; b ^= c; b <<<= 12
	c = c.Add(d)
	b = b.Xor(c)
	b = b.ShiftAllLeft(12).Or(b.ShiftAllRight(20))

	// a += b; d ^= a; d <<<= 8
	a = a.Add(b)
	d = d.Xor(a)
	d = d.ReshapeToUint8s().PermuteOrZeroGrouped(rotl8).ReshapeToUint32s()

	// c += d; b ^= c; b <<<= 7
	c = c.Add(d)
	b = b.Xor(c)
	b = b.ShiftAllLeft(7).Or(b.ShiftAllRight(25))

	return a, b, c, d
}

// xorStoreCol XORs 128 bytes of key stream held in the column vectors w0..w3
// (block 2k in the low 128-bit lanes, block 2k+1 in the high lanes) with
// src[off:off+128] and stores the result into dst[off:off+128].
//
// The ConcatPermute128Scalars call is the fused output transpose: it selects
// the low/high 128-bit lanes of the word-group vectors into block-major order
// before the XOR, so no separate transpose step is needed.
func xorStoreCol(dst, src unsafe.Pointer, off int, w0, w1, w2, w3 archsimd.Uint32x8) {
	t := w0.ConcatPermute128Scalars(0, 2, w1)
	t = t.Xor(archsimd.LoadUint32x8Array((*[8]uint32)(unsafe.Add(src, off))))
	t.StoreArray((*[8]uint32)(unsafe.Add(dst, off)))

	t = w2.ConcatPermute128Scalars(0, 2, w3)
	t = t.Xor(archsimd.LoadUint32x8Array((*[8]uint32)(unsafe.Add(src, off+32))))
	t.StoreArray((*[8]uint32)(unsafe.Add(dst, off+32)))

	t = w0.ConcatPermute128Scalars(1, 3, w1)
	t = t.Xor(archsimd.LoadUint32x8Array((*[8]uint32)(unsafe.Add(src, off+64))))
	t.StoreArray((*[8]uint32)(unsafe.Add(dst, off+64)))

	t = w2.ConcatPermute128Scalars(1, 3, w3)
	t = t.Xor(archsimd.LoadUint32x8Array((*[8]uint32)(unsafe.Add(src, off+96))))
	t.StoreArray((*[8]uint32)(unsafe.Add(dst, off+96)))
}

// storeCol stores 128 bytes of key stream held in the column vectors w0..w3
// (block 2k in the low 128-bit lanes, block 2k+1 in the high lanes) into
// ks[off:off+128].
func storeCol(ks unsafe.Pointer, off int, w0, w1, w2, w3 archsimd.Uint32x8) {
	w0.ConcatPermute128Scalars(0, 2, w1).StoreArray((*[8]uint32)(unsafe.Add(ks, off)))
	w2.ConcatPermute128Scalars(0, 2, w3).StoreArray((*[8]uint32)(unsafe.Add(ks, off+32)))
	w0.ConcatPermute128Scalars(1, 3, w1).StoreArray((*[8]uint32)(unsafe.Add(ks, off+64)))
	w2.ConcatPermute128Scalars(1, 3, w3).StoreArray((*[8]uint32)(unsafe.Add(ks, off+96)))
}
