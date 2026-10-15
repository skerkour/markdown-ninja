//go:build arm64 && goexperiment.simd

package blake3

import (
	"simd/archsimd"
	"unsafe"
)

// This file is the experimental, cgo-free, assembly-free SIMD acceleration of
// BLAKE3's per-chunk compression for arm64 (NEON), written with Go's
// experimental simd/archsimd intrinsics. It only builds on arm64 under
// GOEXPERIMENT=simd; the generic build (blake3_generic.go) keeps every other
// configuration scalar.
//
// NEON registers are 128-bit, so we hash 4 full 1024-byte chunks at once — one
// chunk per uint32 lane (Uint32x4). The 16 compression state words become 16
// vectors; the quarter-round mixing is pure elementwise Add/Xor, with the
// byte-aligned 16/8-bit rotations done as a single VTBL (LookupOrZero) and the
// 12/7-bit rotations as a shift-or (VUSHL; archsimd exposes no immediate-shift
// or VSRI form). The message vectors and state vectors are named locals and
// the 7 rounds are fully unrolled, so the compiler keeps all 32 vectors in
// NEON registers. The per-block message is loaded straight off the chunk bytes
// (little-endian words) and transposed into the lane-major layout with the
// standard NEON 4x4 transpose (TRN1/TRN2). Results are bit-identical to the
// scalar path.

// simdLanes is the number of chunks (or output blocks) per batch.
const simdLanes = 4

// fillChunkCVs hashes simdLanes chunks per NEON batch, synchronously on the
// calling goroutine. All work is stack-scoped, so nothing allocates; the SIMD
// batch loop handles the bulk and a scalar loop finishes any remainder.
func fillChunkCVs(data []byte, cvs [][8]uint32, base uint64, key [8]uint32, flags uint32) {
	n := len(cvs)
	batch := 0
	for ; batch+simdLanes <= n; batch += simdLanes {
		compressChunksLanes(data, batch, base, cvs, key, flags)
	}
	for ; batch < n; batch++ {
		cvs[batch] = compressChunkCV(data, batch, base, key, flags)
	}
}

// fillChunkCV15 computes the chaining value of a full chunk after its first 15
// blocks (the inputCV of its CHUNK_END block) for the last chunk's output node.
// The NEON kernel has no partial-block mode, so this uses the scalar kernel.
// Feeding all 16 blocks compresses the first 15 and leaves the last in the
// buffer, exactly like a full chunkState.update.
func fillChunkCV15(data []byte, cvs [][8]uint32, base uint64, key [8]uint32, flags uint32) {
	cs := newChunkState(key, base, flags)
	cs.update(data[:16*blockLen])
	cvs[0] = cs.cv
}

// compressOutputs fills out with the extendable root output, simdLanes blocks
// per NEON batch. Each output block uses a distinct counter (the per-lane
// counter vectors) against the same message block and input CV.
func compressOutputs(out []byte, cv *[8]uint32, block *[16]uint32, blkLen, flags uint32, start uint64) {
	for len(out) >= simdLanes*blockLen {
		compressOutputsLanes(out, cv, block, blkLen, flags, start)
		out = out[simdLanes*blockLen:]
		start += simdLanes
	}
	compressOutputsScalar(out, cv, block, blkLen, flags, start)
}

// rotr16Tbl and rotr8Tbl are the VTBL index vectors that rotate each 32-bit
// lane right by 16/8 bits (byte-aligned rotations, so a single TBL does the
// job). They are kept as plain byte arrays and materialized into vectors by the
// compiler inside the kernels. rotr8Tbl is {1,2,3,0,...}: rotr8 of a 32-bit
// word moves its least-significant byte to the most-significant position.
var (
	rotr16Tbl = [16]uint8{2, 3, 0, 1, 6, 7, 4, 5, 10, 11, 8, 9, 14, 15, 12, 13}
	rotr8Tbl  = [16]uint8{1, 2, 3, 0, 5, 6, 7, 4, 9, 10, 11, 8, 13, 14, 15, 12}
)

// shiftCounts holds the per-lane signed shift counts for the 12/7-bit right
// rotations. Because archsimd's ShiftAllRight/Left lower to the register-form
// VUSHL, the shift amount must live in a vector register; materializing it
// inside the rotate (MOVI+VMOV+VDUP per use) forces the compiler to spill the
// value being rotated. Hoisting these four vectors as loop-invariant gVec
// parameters keeps them in registers so each rotate is exactly two VUSHL + one
// VORR with no spills. Negative counts shift right (rotr12 = x>>12 | x<<20,
// rotr7 = x>>7 | x<<25).
var shiftCounts = [4][4]int32{
	{-12, -12, -12, -12},
	{20, 20, 20, 20},
	{-7, -7, -7, -7},
	{25, 25, 25, 25},
}

// rotr16 and rotr8 rotate each 32-bit lane right by 16/8 bits with a single
// VTBL (byte table lookup) instead of shift+or.
func rotr16(x archsimd.Uint32x4) archsimd.Uint32x4 {
	return x.ReshapeToUint8s().LookupOrZero(archsimd.LoadUint8x16Array(&rotr16Tbl)).ReshapeToUint32s()
}

func rotr8(x archsimd.Uint32x4) archsimd.Uint32x4 {
	return x.ReshapeToUint8s().LookupOrZero(archsimd.LoadUint8x16Array(&rotr8Tbl)).ReshapeToUint32s()
}

// rotr12x and rotr7x rotate each 32-bit lane right by 12/7 bits using the
// loop-invariant shift count vectors passed in (n12/p20 and n7/p25), so the
// compiler keeps the counts in registers across the whole round loop.
func rotr12x(x archsimd.Uint32x4, n12, p20 archsimd.Int32x4) archsimd.Uint32x4 {
	return x.Shift(n12).Or(x.Shift(p20))
}

func rotr7x(x archsimd.Uint32x4, n7, p25 archsimd.Int32x4) archsimd.Uint32x4 {
	return x.Shift(n7).Or(x.Shift(p25))
}

// gVec is the BLAKE3 quarter round on simdLanes lanes at once. The rotations
// are right by 16/12/8/7, matching the scalar RotateLeft32(x, -k). n12/p20 and
// n7/p25 are the loop-invariant 12/7-bit shift count vectors.
func gVec(va, vb, vc, vd, mx, my archsimd.Uint32x4, n12, p20, n7, p25 archsimd.Int32x4) (archsimd.Uint32x4, archsimd.Uint32x4, archsimd.Uint32x4, archsimd.Uint32x4) {
	va = va.Add(vb).Add(mx)
	vd = rotr16(vd.Xor(va))
	vc = vc.Add(vd)
	vb = rotr12x(vb.Xor(vc), n12, p20)
	va = va.Add(vb).Add(my)
	vd = rotr8(vd.Xor(va))
	vc = vc.Add(vd)
	vb = rotr7x(vb.Xor(vc), n7, p25)
	return va, vb, vc, vd
}

// transpose4 returns the columns of the 4×4 matrix whose rows are r0..r3:
// result k = {r0[k], r1[k], r2[k], r3[k]}. Two rounds of TRN1/TRN2 (the
// standard NEON 4×4 transpose), expressed with archsimd intrinsics — the same
// construction as chacha20's transpose4Neon, with no scalar gather.
func transpose4(r0, r1, r2, r3 archsimd.Uint32x4) (archsimd.Uint32x4, archsimd.Uint32x4, archsimd.Uint32x4, archsimd.Uint32x4) {
	u0 := r0.ConcatEven(r1)
	u1 := r0.ConcatOdd(r1)
	u2 := r2.ConcatEven(r3)
	u3 := r2.ConcatOdd(r3)
	return u0.ConcatEven(u2), u1.ConcatEven(u3), u0.ConcatOdd(u2), u1.ConcatOdd(u3)
}

// compressChunksLanes hashes the simdLanes full 1024-byte chunks at indices
// base..base+simdLanes-1 (lane j = chunk base+j) and writes their chaining
// values into cvs. counterBase is the BLAKE3 chunk counter of data[0].
//
// The 16 message vectors and 16 state vectors are held as distinct named
// locals (no indexed arrays) and the 7 rounds are fully unrolled with static
// message indices, so the Go compiler can keep all 32 vectors in NEON
// registers instead of spilling the arrays to memory.
func compressChunksLanes(data []byte, base int, counterBase uint64, cvs [][8]uint32, key [8]uint32, flags uint32) {
	// Per-lane chunk counters (chunk index), split into low/high 32 bits.
	var ctrLo, ctrHi [simdLanes]uint32
	for j := 0; j < simdLanes; j++ {
		c := counterBase + uint64(base+j)
		ctrLo[j] = uint32(c)
		ctrHi[j] = uint32(c >> 32)
	}
	vCtrLo := archsimd.LoadUint32x4(ctrLo[:])
	vCtrHi := archsimd.LoadUint32x4(ctrHi[:])
	vBlockLen := archsimd.BroadcastUint32x4(blockLen)

	// Chaining value and IV, one broadcast vector per word.
	cv0 := archsimd.BroadcastUint32x4(key[0])
	cv1 := archsimd.BroadcastUint32x4(key[1])
	cv2 := archsimd.BroadcastUint32x4(key[2])
	cv3 := archsimd.BroadcastUint32x4(key[3])
	cv4 := archsimd.BroadcastUint32x4(key[4])
	cv5 := archsimd.BroadcastUint32x4(key[5])
	cv6 := archsimd.BroadcastUint32x4(key[6])
	cv7 := archsimd.BroadcastUint32x4(key[7])
	iv0 := archsimd.BroadcastUint32x4(iv[0])
	iv1 := archsimd.BroadcastUint32x4(iv[1])
	iv2 := archsimd.BroadcastUint32x4(iv[2])
	iv3 := archsimd.BroadcastUint32x4(iv[3])

	// Loop-invariant 12/7-bit shift count vectors (see shiftCounts).
	n12 := archsimd.LoadInt32x4Array(&shiftCounts[0])
	p20 := archsimd.LoadInt32x4Array(&shiftCounts[1])
	n7 := archsimd.LoadInt32x4Array(&shiftCounts[2])
	p25 := archsimd.LoadInt32x4Array(&shiftCounts[3])

	// A full chunk is 16 blocks of 64 bytes; block 0 carries CHUNK_START and
	// block 15 carries CHUNK_END, mirroring chunkState.update + output().
	for b := 0; b < 16; b++ {
		// Load each chunk's 16 message words directly as vectors — on a
		// little-endian arch the chunk bytes are the LE words — and transpose
		// 4 chunks at a time so m[i] lane j = word i of chunk j.
		w0 := unsafe.Slice((*uint32)(unsafe.Pointer(&data[(base+0)*chunkLen+b*blockLen])), 16)
		w1 := unsafe.Slice((*uint32)(unsafe.Pointer(&data[(base+1)*chunkLen+b*blockLen])), 16)
		w2 := unsafe.Slice((*uint32)(unsafe.Pointer(&data[(base+2)*chunkLen+b*blockLen])), 16)
		w3 := unsafe.Slice((*uint32)(unsafe.Pointer(&data[(base+3)*chunkLen+b*blockLen])), 16)
		m0, m1, m2, m3 := transpose4(
			archsimd.LoadUint32x4(w0[0:]),
			archsimd.LoadUint32x4(w1[0:]),
			archsimd.LoadUint32x4(w2[0:]),
			archsimd.LoadUint32x4(w3[0:]),
		)
		m4, m5, m6, m7 := transpose4(
			archsimd.LoadUint32x4(w0[4:]),
			archsimd.LoadUint32x4(w1[4:]),
			archsimd.LoadUint32x4(w2[4:]),
			archsimd.LoadUint32x4(w3[4:]),
		)
		m8, m9, m10, m11 := transpose4(
			archsimd.LoadUint32x4(w0[8:]),
			archsimd.LoadUint32x4(w1[8:]),
			archsimd.LoadUint32x4(w2[8:]),
			archsimd.LoadUint32x4(w3[8:]),
		)
		m12, m13, m14, m15 := transpose4(
			archsimd.LoadUint32x4(w0[12:]),
			archsimd.LoadUint32x4(w1[12:]),
			archsimd.LoadUint32x4(w2[12:]),
			archsimd.LoadUint32x4(w3[12:]),
		)

		fl := flags
		if b == 0 {
			fl |= flagChunkStart
		}
		if b == 15 {
			fl |= flagChunkEnd
		}

		v0, v1, v2, v3 := cv0, cv1, cv2, cv3
		v4, v5, v6, v7 := cv4, cv5, cv6, cv7
		v8, v9, v10, v11 := iv0, iv1, iv2, iv3
		v12, v13, v14, v15 := vCtrLo, vCtrHi, vBlockLen, archsimd.BroadcastUint32x4(fl)

		// 7 unrolled rounds; each line is one gVec with the static message
		// schedule index.
		// round 0
		v0, v4, v8, v12 = gVec(v0, v4, v8, v12, m0, m1, n12, p20, n7, p25)
		v1, v5, v9, v13 = gVec(v1, v5, v9, v13, m2, m3, n12, p20, n7, p25)
		v2, v6, v10, v14 = gVec(v2, v6, v10, v14, m4, m5, n12, p20, n7, p25)
		v3, v7, v11, v15 = gVec(v3, v7, v11, v15, m6, m7, n12, p20, n7, p25)
		v0, v5, v10, v15 = gVec(v0, v5, v10, v15, m8, m9, n12, p20, n7, p25)
		v1, v6, v11, v12 = gVec(v1, v6, v11, v12, m10, m11, n12, p20, n7, p25)
		v2, v7, v8, v13 = gVec(v2, v7, v8, v13, m12, m13, n12, p20, n7, p25)
		v3, v4, v9, v14 = gVec(v3, v4, v9, v14, m14, m15, n12, p20, n7, p25)
		// round 1
		v0, v4, v8, v12 = gVec(v0, v4, v8, v12, m2, m6, n12, p20, n7, p25)
		v1, v5, v9, v13 = gVec(v1, v5, v9, v13, m3, m10, n12, p20, n7, p25)
		v2, v6, v10, v14 = gVec(v2, v6, v10, v14, m7, m0, n12, p20, n7, p25)
		v3, v7, v11, v15 = gVec(v3, v7, v11, v15, m4, m13, n12, p20, n7, p25)
		v0, v5, v10, v15 = gVec(v0, v5, v10, v15, m1, m11, n12, p20, n7, p25)
		v1, v6, v11, v12 = gVec(v1, v6, v11, v12, m12, m5, n12, p20, n7, p25)
		v2, v7, v8, v13 = gVec(v2, v7, v8, v13, m9, m14, n12, p20, n7, p25)
		v3, v4, v9, v14 = gVec(v3, v4, v9, v14, m15, m8, n12, p20, n7, p25)
		// round 2
		v0, v4, v8, v12 = gVec(v0, v4, v8, v12, m3, m4, n12, p20, n7, p25)
		v1, v5, v9, v13 = gVec(v1, v5, v9, v13, m10, m12, n12, p20, n7, p25)
		v2, v6, v10, v14 = gVec(v2, v6, v10, v14, m13, m2, n12, p20, n7, p25)
		v3, v7, v11, v15 = gVec(v3, v7, v11, v15, m7, m14, n12, p20, n7, p25)
		v0, v5, v10, v15 = gVec(v0, v5, v10, v15, m6, m5, n12, p20, n7, p25)
		v1, v6, v11, v12 = gVec(v1, v6, v11, v12, m9, m0, n12, p20, n7, p25)
		v2, v7, v8, v13 = gVec(v2, v7, v8, v13, m11, m15, n12, p20, n7, p25)
		v3, v4, v9, v14 = gVec(v3, v4, v9, v14, m8, m1, n12, p20, n7, p25)
		// round 3
		v0, v4, v8, v12 = gVec(v0, v4, v8, v12, m10, m7, n12, p20, n7, p25)
		v1, v5, v9, v13 = gVec(v1, v5, v9, v13, m12, m9, n12, p20, n7, p25)
		v2, v6, v10, v14 = gVec(v2, v6, v10, v14, m14, m3, n12, p20, n7, p25)
		v3, v7, v11, v15 = gVec(v3, v7, v11, v15, m13, m15, n12, p20, n7, p25)
		v0, v5, v10, v15 = gVec(v0, v5, v10, v15, m4, m0, n12, p20, n7, p25)
		v1, v6, v11, v12 = gVec(v1, v6, v11, v12, m11, m2, n12, p20, n7, p25)
		v2, v7, v8, v13 = gVec(v2, v7, v8, v13, m5, m8, n12, p20, n7, p25)
		v3, v4, v9, v14 = gVec(v3, v4, v9, v14, m1, m6, n12, p20, n7, p25)
		// round 4
		v0, v4, v8, v12 = gVec(v0, v4, v8, v12, m12, m13, n12, p20, n7, p25)
		v1, v5, v9, v13 = gVec(v1, v5, v9, v13, m9, m11, n12, p20, n7, p25)
		v2, v6, v10, v14 = gVec(v2, v6, v10, v14, m15, m10, n12, p20, n7, p25)
		v3, v7, v11, v15 = gVec(v3, v7, v11, v15, m14, m8, n12, p20, n7, p25)
		v0, v5, v10, v15 = gVec(v0, v5, v10, v15, m7, m2, n12, p20, n7, p25)
		v1, v6, v11, v12 = gVec(v1, v6, v11, v12, m5, m3, n12, p20, n7, p25)
		v2, v7, v8, v13 = gVec(v2, v7, v8, v13, m0, m1, n12, p20, n7, p25)
		v3, v4, v9, v14 = gVec(v3, v4, v9, v14, m6, m4, n12, p20, n7, p25)
		// round 5
		v0, v4, v8, v12 = gVec(v0, v4, v8, v12, m9, m14, n12, p20, n7, p25)
		v1, v5, v9, v13 = gVec(v1, v5, v9, v13, m11, m5, n12, p20, n7, p25)
		v2, v6, v10, v14 = gVec(v2, v6, v10, v14, m8, m12, n12, p20, n7, p25)
		v3, v7, v11, v15 = gVec(v3, v7, v11, v15, m15, m1, n12, p20, n7, p25)
		v0, v5, v10, v15 = gVec(v0, v5, v10, v15, m13, m3, n12, p20, n7, p25)
		v1, v6, v11, v12 = gVec(v1, v6, v11, v12, m0, m10, n12, p20, n7, p25)
		v2, v7, v8, v13 = gVec(v2, v7, v8, v13, m2, m6, n12, p20, n7, p25)
		v3, v4, v9, v14 = gVec(v3, v4, v9, v14, m4, m7, n12, p20, n7, p25)
		// round 6
		v0, v4, v8, v12 = gVec(v0, v4, v8, v12, m11, m15, n12, p20, n7, p25)
		v1, v5, v9, v13 = gVec(v1, v5, v9, v13, m5, m0, n12, p20, n7, p25)
		v2, v6, v10, v14 = gVec(v2, v6, v10, v14, m1, m9, n12, p20, n7, p25)
		v3, v7, v11, v15 = gVec(v3, v7, v11, v15, m8, m6, n12, p20, n7, p25)
		v0, v5, v10, v15 = gVec(v0, v5, v10, v15, m14, m10, n12, p20, n7, p25)
		v1, v6, v11, v12 = gVec(v1, v6, v11, v12, m2, m12, n12, p20, n7, p25)
		v2, v7, v8, v13 = gVec(v2, v7, v8, v13, m3, m4, n12, p20, n7, p25)
		v3, v4, v9, v14 = gVec(v3, v4, v9, v14, m7, m13, n12, p20, n7, p25)

		cv0 = v0.Xor(v8)
		cv1 = v1.Xor(v9)
		cv2 = v2.Xor(v10)
		cv3 = v3.Xor(v11)
		cv4 = v4.Xor(v12)
		cv5 = v5.Xor(v13)
		cv6 = v6.Xor(v14)
		cv7 = v7.Xor(v15)
	}

	// Scatter lane j of each CV word back to chunk base+j.
	var lane [simdLanes]uint32
	cv0.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		cvs[base+j][0] = lane[j]
	}
	cv1.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		cvs[base+j][1] = lane[j]
	}
	cv2.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		cvs[base+j][2] = lane[j]
	}
	cv3.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		cvs[base+j][3] = lane[j]
	}
	cv4.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		cvs[base+j][4] = lane[j]
	}
	cv5.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		cvs[base+j][5] = lane[j]
	}
	cv6.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		cvs[base+j][6] = lane[j]
	}
	cv7.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		cvs[base+j][7] = lane[j]
	}
}

// compressOutputsLanes computes simdLanes 64-byte root output blocks starting
// at block start. The message block and input CV are constant across lanes;
// only the counter varies. As with compressChunksLanes, the message and state
// vectors are named locals and the rounds are fully unrolled.
func compressOutputsLanes(out []byte, cv *[8]uint32, block *[16]uint32, blkLen, flags uint32, start uint64) {
	var ctrLo, ctrHi [simdLanes]uint32
	for j := 0; j < simdLanes; j++ {
		c := start + uint64(j)
		ctrLo[j] = uint32(c)
		ctrHi[j] = uint32(c >> 32)
	}
	vCtrLo := archsimd.LoadUint32x4(ctrLo[:])
	vCtrHi := archsimd.LoadUint32x4(ctrHi[:])
	vBlockLen := archsimd.BroadcastUint32x4(blkLen)
	vFlags := archsimd.BroadcastUint32x4(flags)

	cv0 := archsimd.BroadcastUint32x4(cv[0])
	cv1 := archsimd.BroadcastUint32x4(cv[1])
	cv2 := archsimd.BroadcastUint32x4(cv[2])
	cv3 := archsimd.BroadcastUint32x4(cv[3])
	cv4 := archsimd.BroadcastUint32x4(cv[4])
	cv5 := archsimd.BroadcastUint32x4(cv[5])
	cv6 := archsimd.BroadcastUint32x4(cv[6])
	cv7 := archsimd.BroadcastUint32x4(cv[7])
	iv0 := archsimd.BroadcastUint32x4(iv[0])
	iv1 := archsimd.BroadcastUint32x4(iv[1])
	iv2 := archsimd.BroadcastUint32x4(iv[2])
	iv3 := archsimd.BroadcastUint32x4(iv[3])

	m0 := archsimd.BroadcastUint32x4(block[0])
	m1 := archsimd.BroadcastUint32x4(block[1])
	m2 := archsimd.BroadcastUint32x4(block[2])
	m3 := archsimd.BroadcastUint32x4(block[3])
	m4 := archsimd.BroadcastUint32x4(block[4])
	m5 := archsimd.BroadcastUint32x4(block[5])
	m6 := archsimd.BroadcastUint32x4(block[6])
	m7 := archsimd.BroadcastUint32x4(block[7])
	m8 := archsimd.BroadcastUint32x4(block[8])
	m9 := archsimd.BroadcastUint32x4(block[9])
	m10 := archsimd.BroadcastUint32x4(block[10])
	m11 := archsimd.BroadcastUint32x4(block[11])
	m12 := archsimd.BroadcastUint32x4(block[12])
	m13 := archsimd.BroadcastUint32x4(block[13])
	m14 := archsimd.BroadcastUint32x4(block[14])
	m15 := archsimd.BroadcastUint32x4(block[15])

	// Loop-invariant 12/7-bit shift count vectors (see shiftCounts).
	n12 := archsimd.LoadInt32x4Array(&shiftCounts[0])
	p20 := archsimd.LoadInt32x4Array(&shiftCounts[1])
	n7 := archsimd.LoadInt32x4Array(&shiftCounts[2])
	p25 := archsimd.LoadInt32x4Array(&shiftCounts[3])

	v0, v1, v2, v3 := cv0, cv1, cv2, cv3
	v4, v5, v6, v7 := cv4, cv5, cv6, cv7
	v8, v9, v10, v11 := iv0, iv1, iv2, iv3
	v12, v13, v14, v15 := vCtrLo, vCtrHi, vBlockLen, vFlags

	// 7 unrolled rounds; the message schedule is the same as the chunk kernel.
	// round 0
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, m0, m1, n12, p20, n7, p25)
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, m2, m3, n12, p20, n7, p25)
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, m4, m5, n12, p20, n7, p25)
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, m6, m7, n12, p20, n7, p25)
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, m8, m9, n12, p20, n7, p25)
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, m10, m11, n12, p20, n7, p25)
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, m12, m13, n12, p20, n7, p25)
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, m14, m15, n12, p20, n7, p25)
	// round 1
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, m2, m6, n12, p20, n7, p25)
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, m3, m10, n12, p20, n7, p25)
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, m7, m0, n12, p20, n7, p25)
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, m4, m13, n12, p20, n7, p25)
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, m1, m11, n12, p20, n7, p25)
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, m12, m5, n12, p20, n7, p25)
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, m9, m14, n12, p20, n7, p25)
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, m15, m8, n12, p20, n7, p25)
	// round 2
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, m3, m4, n12, p20, n7, p25)
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, m10, m12, n12, p20, n7, p25)
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, m13, m2, n12, p20, n7, p25)
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, m7, m14, n12, p20, n7, p25)
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, m6, m5, n12, p20, n7, p25)
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, m9, m0, n12, p20, n7, p25)
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, m11, m15, n12, p20, n7, p25)
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, m8, m1, n12, p20, n7, p25)
	// round 3
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, m10, m7, n12, p20, n7, p25)
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, m12, m9, n12, p20, n7, p25)
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, m14, m3, n12, p20, n7, p25)
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, m13, m15, n12, p20, n7, p25)
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, m4, m0, n12, p20, n7, p25)
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, m11, m2, n12, p20, n7, p25)
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, m5, m8, n12, p20, n7, p25)
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, m1, m6, n12, p20, n7, p25)
	// round 4
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, m12, m13, n12, p20, n7, p25)
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, m9, m11, n12, p20, n7, p25)
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, m15, m10, n12, p20, n7, p25)
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, m14, m8, n12, p20, n7, p25)
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, m7, m2, n12, p20, n7, p25)
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, m5, m3, n12, p20, n7, p25)
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, m0, m1, n12, p20, n7, p25)
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, m6, m4, n12, p20, n7, p25)
	// round 5
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, m9, m14, n12, p20, n7, p25)
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, m11, m5, n12, p20, n7, p25)
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, m8, m12, n12, p20, n7, p25)
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, m15, m1, n12, p20, n7, p25)
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, m13, m3, n12, p20, n7, p25)
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, m0, m10, n12, p20, n7, p25)
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, m2, m6, n12, p20, n7, p25)
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, m4, m7, n12, p20, n7, p25)
	// round 6
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, m11, m15, n12, p20, n7, p25)
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, m5, m0, n12, p20, n7, p25)
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, m1, m9, n12, p20, n7, p25)
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, m8, m6, n12, p20, n7, p25)
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, m14, m10, n12, p20, n7, p25)
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, m2, m12, n12, p20, n7, p25)
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, m3, m4, n12, p20, n7, p25)
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, m7, m13, n12, p20, n7, p25)

	// Final output XOR: state[i] ^= state[i+8], state[i+8] ^= cv[i].
	v0 = v0.Xor(v8)
	v1 = v1.Xor(v9)
	v2 = v2.Xor(v10)
	v3 = v3.Xor(v11)
	v4 = v4.Xor(v12)
	v5 = v5.Xor(v13)
	v6 = v6.Xor(v14)
	v7 = v7.Xor(v15)
	v8 = v8.Xor(cv0)
	v9 = v9.Xor(cv1)
	v10 = v10.Xor(cv2)
	v11 = v11.Xor(cv3)
	v12 = v12.Xor(cv4)
	v13 = v13.Xor(cv5)
	v14 = v14.Xor(cv6)
	v15 = v15.Xor(cv7)

	// Transpose the word-major output vectors into block-major (block j's
	// words 4g..4g+3 in row j of group g) so each block is a contiguous store.
	b0w0, b1w0, b2w0, b3w0 := transpose4(v0, v1, v2, v3)
	b0w1, b1w1, b2w1, b3w1 := transpose4(v4, v5, v6, v7)
	b0w2, b1w2, b2w2, b3w2 := transpose4(v8, v9, v10, v11)
	b0w3, b1w3, b2w3, b3w3 := transpose4(v12, v13, v14, v15)

	storeBlock(out, 0, b0w0, b0w1, b0w2, b0w3)
	storeBlock(out, 64, b1w0, b1w1, b1w2, b1w3)
	storeBlock(out, 128, b2w0, b2w1, b2w2, b2w3)
	storeBlock(out, 192, b3w0, b3w1, b3w2, b3w3)
}

// storeBlock stores one 64-byte block (16 words across w0..w3) at out[off:].
func storeBlock(out []byte, off int, w0, w1, w2, w3 archsimd.Uint32x4) {
	w0.ReshapeToUint8s().Store(out[off : off+16])
	w1.ReshapeToUint8s().Store(out[off+16 : off+32])
	w2.ReshapeToUint8s().Store(out[off+32 : off+48])
	w3.ReshapeToUint8s().Store(out[off+48 : off+64])
}

// mergeParentCVs computes the parent chaining value of each pair (src[2i],
// src[2i+1]) into out[i], using the 4-lane NEON parent kernel in batches.
func mergeParentCVs(out, src [][8]uint32, key [8]uint32, flags uint32) {
	n := len(out)
	i := 0
	for ; i+simdLanes <= n; i += simdLanes {
		var left, right [simdLanes][8]uint32
		for j := 0; j < simdLanes; j++ {
			left[j] = src[2*(i+j)]
			right[j] = src[2*(i+j)+1]
		}
		cvs := compressParentsLanes(&left, &right, key, flags)
		for j := 0; j < simdLanes; j++ {
			out[i+j] = cvs[j]
		}
	}
	for ; i < n; i++ {
		out[i] = parentCV(src[2*i], src[2*i+1], key, flags)
	}
}

// compressParentsLanes computes the chaining value of 4 parent nodes in one
// NEON batch. Parent j's block is [left[j], right[j]]; the message is already
// lane-major (no transpose needed), the counter is 0, and the flags carry
// flagParent. Only the first 8 output words (the CV) are needed.
func compressParentsLanes(left, right *[simdLanes][8]uint32, key [8]uint32, flags uint32) [simdLanes][8]uint32 {
	// Lane-major message vectors: m[i] lane j = word i of parent j.
	var scratch [16][simdLanes]uint32
	for j := 0; j < simdLanes; j++ {
		for w := 0; w < 8; w++ {
			scratch[w][j] = left[j][w]
			scratch[w+8][j] = right[j][w]
		}
	}
	m0 := archsimd.LoadUint32x4(scratch[0][:])
	m1 := archsimd.LoadUint32x4(scratch[1][:])
	m2 := archsimd.LoadUint32x4(scratch[2][:])
	m3 := archsimd.LoadUint32x4(scratch[3][:])
	m4 := archsimd.LoadUint32x4(scratch[4][:])
	m5 := archsimd.LoadUint32x4(scratch[5][:])
	m6 := archsimd.LoadUint32x4(scratch[6][:])
	m7 := archsimd.LoadUint32x4(scratch[7][:])
	m8 := archsimd.LoadUint32x4(scratch[8][:])
	m9 := archsimd.LoadUint32x4(scratch[9][:])
	m10 := archsimd.LoadUint32x4(scratch[10][:])
	m11 := archsimd.LoadUint32x4(scratch[11][:])
	m12 := archsimd.LoadUint32x4(scratch[12][:])
	m13 := archsimd.LoadUint32x4(scratch[13][:])
	m14 := archsimd.LoadUint32x4(scratch[14][:])
	m15 := archsimd.LoadUint32x4(scratch[15][:])

	cv0 := archsimd.BroadcastUint32x4(key[0])
	cv1 := archsimd.BroadcastUint32x4(key[1])
	cv2 := archsimd.BroadcastUint32x4(key[2])
	cv3 := archsimd.BroadcastUint32x4(key[3])
	cv4 := archsimd.BroadcastUint32x4(key[4])
	cv5 := archsimd.BroadcastUint32x4(key[5])
	cv6 := archsimd.BroadcastUint32x4(key[6])
	cv7 := archsimd.BroadcastUint32x4(key[7])
	iv0 := archsimd.BroadcastUint32x4(iv[0])
	iv1 := archsimd.BroadcastUint32x4(iv[1])
	iv2 := archsimd.BroadcastUint32x4(iv[2])
	iv3 := archsimd.BroadcastUint32x4(iv[3])

	// Loop-invariant 12/7-bit shift count vectors (see shiftCounts).
	n12 := archsimd.LoadInt32x4Array(&shiftCounts[0])
	p20 := archsimd.LoadInt32x4Array(&shiftCounts[1])
	n7 := archsimd.LoadInt32x4Array(&shiftCounts[2])
	p25 := archsimd.LoadInt32x4Array(&shiftCounts[3])

	v0, v1, v2, v3 := cv0, cv1, cv2, cv3
	v4, v5, v6, v7 := cv4, cv5, cv6, cv7
	v8, v9, v10, v11 := iv0, iv1, iv2, iv3
	v12 := archsimd.BroadcastUint32x4(0)
	v13 := archsimd.BroadcastUint32x4(0)
	v14 := archsimd.BroadcastUint32x4(blockLen)
	v15 := archsimd.BroadcastUint32x4(flags | flagParent)

	// round 0
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, m0, m1, n12, p20, n7, p25)
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, m2, m3, n12, p20, n7, p25)
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, m4, m5, n12, p20, n7, p25)
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, m6, m7, n12, p20, n7, p25)
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, m8, m9, n12, p20, n7, p25)
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, m10, m11, n12, p20, n7, p25)
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, m12, m13, n12, p20, n7, p25)
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, m14, m15, n12, p20, n7, p25)
	// round 1
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, m2, m6, n12, p20, n7, p25)
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, m3, m10, n12, p20, n7, p25)
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, m7, m0, n12, p20, n7, p25)
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, m4, m13, n12, p20, n7, p25)
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, m1, m11, n12, p20, n7, p25)
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, m12, m5, n12, p20, n7, p25)
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, m9, m14, n12, p20, n7, p25)
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, m15, m8, n12, p20, n7, p25)
	// round 2
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, m3, m4, n12, p20, n7, p25)
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, m10, m12, n12, p20, n7, p25)
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, m13, m2, n12, p20, n7, p25)
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, m7, m14, n12, p20, n7, p25)
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, m6, m5, n12, p20, n7, p25)
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, m9, m0, n12, p20, n7, p25)
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, m11, m15, n12, p20, n7, p25)
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, m8, m1, n12, p20, n7, p25)
	// round 3
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, m10, m7, n12, p20, n7, p25)
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, m12, m9, n12, p20, n7, p25)
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, m14, m3, n12, p20, n7, p25)
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, m13, m15, n12, p20, n7, p25)
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, m4, m0, n12, p20, n7, p25)
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, m11, m2, n12, p20, n7, p25)
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, m5, m8, n12, p20, n7, p25)
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, m1, m6, n12, p20, n7, p25)
	// round 4
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, m12, m13, n12, p20, n7, p25)
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, m9, m11, n12, p20, n7, p25)
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, m15, m10, n12, p20, n7, p25)
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, m14, m8, n12, p20, n7, p25)
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, m7, m2, n12, p20, n7, p25)
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, m5, m3, n12, p20, n7, p25)
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, m0, m1, n12, p20, n7, p25)
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, m6, m4, n12, p20, n7, p25)
	// round 5
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, m9, m14, n12, p20, n7, p25)
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, m11, m5, n12, p20, n7, p25)
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, m8, m12, n12, p20, n7, p25)
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, m15, m1, n12, p20, n7, p25)
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, m13, m3, n12, p20, n7, p25)
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, m0, m10, n12, p20, n7, p25)
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, m2, m6, n12, p20, n7, p25)
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, m4, m7, n12, p20, n7, p25)
	// round 6
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, m11, m15, n12, p20, n7, p25)
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, m5, m0, n12, p20, n7, p25)
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, m1, m9, n12, p20, n7, p25)
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, m8, m6, n12, p20, n7, p25)
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, m14, m10, n12, p20, n7, p25)
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, m2, m12, n12, p20, n7, p25)
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, m3, m4, n12, p20, n7, p25)
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, m7, m13, n12, p20, n7, p25)

	// CV update: word i = v[i]^v[i+8], then scatter across the 4 parents.
	v0 = v0.Xor(v8)
	v1 = v1.Xor(v9)
	v2 = v2.Xor(v10)
	v3 = v3.Xor(v11)
	v4 = v4.Xor(v12)
	v5 = v5.Xor(v13)
	v6 = v6.Xor(v14)
	v7 = v7.Xor(v15)

	var lane [simdLanes]uint32
	var cvs [simdLanes][8]uint32
	v0.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		cvs[j][0] = lane[j]
	}
	v1.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		cvs[j][1] = lane[j]
	}
	v2.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		cvs[j][2] = lane[j]
	}
	v3.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		cvs[j][3] = lane[j]
	}
	v4.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		cvs[j][4] = lane[j]
	}
	v5.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		cvs[j][5] = lane[j]
	}
	v6.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		cvs[j][6] = lane[j]
	}
	v7.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		cvs[j][7] = lane[j]
	}
	return cvs
}
