//go:build amd64 && goexperiment.simd

package blake3

import (
	"encoding/binary"
	"simd/archsimd"
)

// This file is the experimental, cgo-free, assembly-free SIMD acceleration of
// BLAKE3's per-chunk compression for amd64 (AVX2), written with Go's
// experimental simd/archsimd intrinsics. It only builds on amd64 under
// GOEXPERIMENT=simd; the generic build (blake3_generic.go) keeps every other
// configuration scalar, and a runtime AVX2 check keeps CPUs without AVX2 on
// the scalar path.
//
// 256-bit registers hold 8 uint32 lanes, so we hash 8 full 1024-byte chunks at
// once — one chunk per lane. The 16 compression state words become 16 vectors;
// the quarter-round mixing is pure elementwise Add/Xor, with the byte-aligned
// 16/8-bit rotations done as a single VPSHUFB (PermuteOrZeroGrouped) and the
// 12/7-bit rotations as a shift-or (VPSRLD/VPSLLD).
//
// AVX2 has only 16 YMM registers while a BLAKE3 block wants 16 live state
// vectors and up to 16 message vectors, so the message words are never held as
// registers: the whole batch is transposed once into a lane-major stack buffer
// (m[i] lane j = word i of chunk base+j) and gVec reads its two message words
// straight from memory each round, like the C reference kernel. The 16 state
// vectors are the only long-lived vector values, so Go's register allocator
// does not have to spill state to make room for messages. Results are
// bit-identical to the scalar path.

// simdLanes is the number of chunks (or output blocks) per batch.
const simdLanes = 8

// fillChunkCVs hashes chunks per SIMD batch, synchronously on the calling
// goroutine. All work is stack-scoped, so nothing allocates. The AVX-512
// kernel (16 chunks/batch) is used when available, else the AVX2 kernel
// (8 chunks/batch), else the scalar path on CPUs without AVX2. The final
// partial batch (fewer chunks than the batch width) is still hashed by the SIMD
// kernel with zero-filled spare lanes, so even small counts of whole chunks use
// the SIMD path.
func fillChunkCVs(data []byte, cvs [][8]uint32, base uint64, key [8]uint32, flags uint32) {
	if !archsimd.X86.AVX2() {
		fillChunkCVsScalar(data, cvs, base, key, flags)
		return
	}
	n := len(cvs)
	batch := 0
	if archsimd.X86.AVX512() {
		for ; batch+simdLanesAvx512 <= n; batch += simdLanesAvx512 {
			compressChunksAvx512(data, batch, base, cvs, key, flags, simdLanesAvx512, 16, true)
		}
		if batch < n {
			// A tail smaller than one AVX-512 batch wastes most of the 16
			// lanes; route tails of up to 4 chunks to the 4-wide XMM kernel
			// (all of whose ops run on AVX-512 CPUs too).
			if rem := n - batch; rem <= 4 {
				compressChunksAvx4(data, batch, base, cvs, key, flags, rem)
			} else {
				compressChunksAvx512(data, batch, base, cvs, key, flags, rem, 16, true)
			}
			return
		}
		return
	}
	for ; batch+simdLanes <= n; batch += simdLanes {
		compressChunksAvx2(data, batch, base, cvs, key, flags, simdLanes, 16, true)
	}
	if batch < n {
		rem := n - batch
		if rem <= 4 {
			// A 1-4 chunk tail: use the 4-wide XMM kernel so the round loop
			// does not waste lanes on zero-filled data.
			compressChunksAvx4(data, batch, base, cvs, key, flags, rem)
		} else {
			compressChunksAvx2(data, batch, base, cvs, key, flags, rem, 16, true)
		}
	}
}

// fillChunkCV15 computes the chaining value of a full 1024-byte chunk after its
// first 15 blocks — the inputCV of the chunk's final (CHUNK_END) block, which
// is what the chunk's output node compresses. It lets hashAll build the last
// chunk's output node without hashing that chunk twice.
func fillChunkCV15(data []byte, cvs [][8]uint32, base uint64, key [8]uint32, flags uint32) {
	if archsimd.X86.AVX512() {
		compressChunksAvx512(data, 0, base, cvs, key, flags, 1, 15, false)
		return
	}
	if archsimd.X86.AVX2() {
		compressChunksAvx2(data, 0, base, cvs, key, flags, 1, 15, false)
		return
	}
	cs := newChunkState(key, base, flags)
	cs.update(data[:15*blockLen])
	cvs[0] = cs.cv
}

// compressOutputs fills out with the extendable root output, simdLanesAvx512
// blocks per AVX-512 batch (or simdLanes per AVX2 batch). Each output block
// uses a distinct counter (the per-lane counter vectors) against the same
// message block and input CV. The broadcast message buffer is built once per
// call (the block is constant across all batches) and shared by the kernels.
func compressOutputs(out []byte, cv *[8]uint32, block *[16]uint32, blkLen, flags uint32, start uint64) {
	if !archsimd.X86.AVX2() {
		compressOutputsScalar(out, cv, block, blkLen, flags, start)
		return
	}
	var m8 [16][simdLanes]uint32
	for i := 0; i < 16; i++ {
		for j := 0; j < simdLanes; j++ {
			m8[i][j] = block[i]
		}
	}
	if archsimd.X86.AVX512() {
		var m512 [16][simdLanesAvx512]uint32
		for i := 0; i < 16; i++ {
			for j := 0; j < simdLanesAvx512; j++ {
				m512[i][j] = block[i]
			}
		}
		for len(out) >= simdLanesAvx512*blockLen {
			compressOutputsAvx512(out, cv, &m512, blkLen, flags, start)
			out = out[simdLanesAvx512*blockLen:]
			start += simdLanesAvx512
		}
	}
	for len(out) >= simdLanes*blockLen {
		compressOutputsAvx2(out, cv, &m8, blkLen, flags, start)
		out = out[simdLanes*blockLen:]
		start += simdLanes
	}
	compressOutputsScalar(out, cv, block, blkLen, flags, start)
}

// rotr16Idx and rotr8Idx are the VPSHUFB index vectors that rotate each 32-bit
// element of a Uint8x32 right by 16/8 bits. They are kept as plain byte arrays
// (no SIMD instructions at init) and materialized into vectors by the compiler
// inside the kernels. rotr8Idx is {1,2,3,0,...}: rotr8 of a 32-bit word moves
// its least-significant byte to the most-significant position.
var (
	rotr16Idx = [32]int8{
		2, 3, 0, 1, 6, 7, 4, 5, 10, 11, 8, 9, 14, 15, 12, 13,
		2, 3, 0, 1, 6, 7, 4, 5, 10, 11, 8, 9, 14, 15, 12, 13,
	}
	rotr8Idx = [32]int8{
		1, 2, 3, 0, 5, 6, 7, 4, 9, 10, 11, 8, 13, 14, 15, 12,
		1, 2, 3, 0, 5, 6, 7, 4, 9, 10, 11, 8, 13, 14, 15, 12,
	}
)

// rotr16 and rotr8 rotate each 32-bit lane right by 16/8 bits with a single
// VPSHUFB (byte permute within each 4-byte group) instead of shift+or. The
// index vector is loaded as a memory operand of the VPSHUFB (the compiler
// keeps it out of registers), matching the reference implementation.
func rotr16(x archsimd.Uint32x8) archsimd.Uint32x8 {
	return x.ReshapeToUint8s().PermuteOrZeroGrouped(archsimd.LoadInt8x32Array(&rotr16Idx)).ReshapeToUint32s()
}

func rotr8(x archsimd.Uint32x8) archsimd.Uint32x8 {
	return x.ReshapeToUint8s().PermuteOrZeroGrouped(archsimd.LoadInt8x32Array(&rotr8Idx)).ReshapeToUint32s()
}

// rotr rotates each 32-bit lane right by k via shift+or for the non-byte
// aligned rotations (12 and 7), which have no single-instruction VPSHUFB.
func rotr(x archsimd.Uint32x8, k uint64) archsimd.Uint32x8 {
	return x.ShiftAllRight(k).Or(x.ShiftAllLeft(32 - k))
}

// The 128-bit (Uint32x4) rotation tables and helpers used by the 4-wide XMM
// kernel for partial batches (fewer than 8 chunks).
var (
	rotr16Idx4 = [16]int8{
		2, 3, 0, 1, 6, 7, 4, 5, 10, 11, 8, 9, 14, 15, 12, 13,
	}
	rotr8Idx4 = [16]int8{
		1, 2, 3, 0, 5, 6, 7, 4, 9, 10, 11, 8, 13, 14, 15, 12,
	}
)

func rotr16x4(x archsimd.Uint32x4) archsimd.Uint32x4 {
	return x.ReshapeToUint8s().PermuteOrZero(archsimd.LoadInt8x16Array(&rotr16Idx4)).ReshapeToUint32s()
}

func rotr8x4(x archsimd.Uint32x4) archsimd.Uint32x4 {
	return x.ReshapeToUint8s().PermuteOrZero(archsimd.LoadInt8x16Array(&rotr8Idx4)).ReshapeToUint32s()
}

func rotr4(x archsimd.Uint32x4, k uint64) archsimd.Uint32x4 {
	return x.ShiftAllRight(k).Or(x.ShiftAllLeft(32 - k))
}

// gVec4 is the BLAKE3 quarter round on 4 lanes at once (the XMM partial-batch
// kernel). As with gVec, the message is loaded with an array load written as
// the leftmost single-use operand of the first VPADDD so it flows directly
// from the load into the add without a spill-reload round trip.
func gVec4(va, vb, vc, vd archsimd.Uint32x4, mx, my *[4]uint32) (archsimd.Uint32x4, archsimd.Uint32x4, archsimd.Uint32x4, archsimd.Uint32x4) {
	va = archsimd.LoadUint32x4Array(mx).Add(va).Add(vb)
	vd = rotr16x4(vd.Xor(va))
	vc = vc.Add(vd)
	vb = rotr4(vb.Xor(vc), 12)
	va = archsimd.LoadUint32x4Array(my).Add(va).Add(vb)
	vd = rotr8x4(vd.Xor(va))
	vc = vc.Add(vd)
	vb = rotr4(vb.Xor(vc), 7)
	return va, vb, vc, vd
}

// gVec is the BLAKE3 quarter round on simdLanes lanes at once. The rotations
// are right by 16/12/8/7, matching the scalar RotateLeft32(x, -k).
//
// mx and my point at the lane-major message rows for the two message words of
// this column (mx = m[sched[i]], my = m[sched[i+1]]); the message is loaded
// from memory here rather than passed as vectors. AVX2 has only 16 YMM
// registers and the 16 compression-state vectors already fill them, so the
// messages are read as memory operands (folding into VPADDD where the compiler
// can) and re-materialized each round, exactly like the C reference kernel,
// instead of occupying registers that would force the state to spill.
func gVec(va, vb, vc, vd archsimd.Uint32x8, mx, my *[8]uint32) (archsimd.Uint32x8, archsimd.Uint32x8, archsimd.Uint32x8, archsimd.Uint32x8) {
	va = archsimd.LoadUint32x8Array(mx).Add(va).Add(vb)
	vd = rotr16(vd.Xor(va))
	vc = vc.Add(vd)
	vb = rotr(vb.Xor(vc), 12)
	va = archsimd.LoadUint32x8Array(my).Add(va).Add(vb)
	vd = rotr8(vd.Xor(va))
	vc = vc.Add(vd)
	vb = rotr(vb.Xor(vc), 7)
	return va, vb, vc, vd
}

// bcast returns a vector with x in every lane (a single VPBROADCASTD).
func bcast(x uint32) archsimd.Uint32x8 {
	return archsimd.BroadcastUint32x8(x)
}

// compressChunksAvx4 hashes the first `lanes` (1..4) full 1024-byte chunks at
// indices base..base+lanes-1 with the 4-wide XMM (Uint32x4) kernel. It is used
// for partial batches of 1-4 chunks, where running the 8-wide kernel would
// waste half the lanes (and thus half the round-loop work on zero-filled
// data). The message data is transposed into a lane-major stack buffer and read
// through gVec4's array loads, mirroring the 8-wide kernel.
func compressChunksAvx4(data []byte, base int, counterBase uint64, cvs [][8]uint32, key [8]uint32, flags uint32, lanes int) {
	// Transpose the message data once: t[b][i][j] = little-endian word i of the
	// 64-byte block b of chunk base+j, for the first `lanes` chunks.
	var t [16][16][4]uint32
	for b := 0; b < 16; b++ {
		for j := 0; j < lanes; j++ {
			blk := data[(base+j)*chunkLen+b*blockLen:]
			for i := 0; i < 16; i++ {
				t[b][i][j] = binary.LittleEndian.Uint32(blk[i*4:])
			}
		}
	}

	// Per-lane chunk counters (chunk index), split into low/high 32 bits.
	var ctrLo, ctrHi [4]uint32
	for j := 0; j < lanes; j++ {
		c := counterBase + uint64(base+j)
		ctrLo[j] = uint32(c)
		ctrHi[j] = uint32(c >> 32)
	}
	vCtrLo := archsimd.LoadUint32x4(ctrLo[:])
	vCtrHi := archsimd.LoadUint32x4(ctrHi[:])
	vBlockLen := archsimd.BroadcastUint32x4(blockLen)

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

	vFlagsStart := archsimd.BroadcastUint32x4(flags | flagChunkStart)
	vFlagsMid := archsimd.BroadcastUint32x4(flags)
	vFlagsEnd := archsimd.BroadcastUint32x4(flags | flagChunkEnd)

	for b := 0; b < 16; b++ {
		v15 := vFlagsMid
		if b == 0 {
			v15 = vFlagsStart
		} else if b == 15 {
			v15 = vFlagsEnd
		}

		v0, v1, v2, v3 := cv0, cv1, cv2, cv3
		v4, v5, v6, v7 := cv4, cv5, cv6, cv7
		v8, v9, v10, v11 := iv0, iv1, iv2, iv3
		v12, v13, v14 := vCtrLo, vCtrHi, vBlockLen

		// 7 unrolled rounds; each line is one gVec4 with the static message
		// schedule index.
		// round 0
		v0, v4, v8, v12 = gVec4(v0, v4, v8, v12, &t[b][0], &t[b][1])
		v1, v5, v9, v13 = gVec4(v1, v5, v9, v13, &t[b][2], &t[b][3])
		v2, v6, v10, v14 = gVec4(v2, v6, v10, v14, &t[b][4], &t[b][5])
		v3, v7, v11, v15 = gVec4(v3, v7, v11, v15, &t[b][6], &t[b][7])
		v0, v5, v10, v15 = gVec4(v0, v5, v10, v15, &t[b][8], &t[b][9])
		v1, v6, v11, v12 = gVec4(v1, v6, v11, v12, &t[b][10], &t[b][11])
		v2, v7, v8, v13 = gVec4(v2, v7, v8, v13, &t[b][12], &t[b][13])
		v3, v4, v9, v14 = gVec4(v3, v4, v9, v14, &t[b][14], &t[b][15])
		// round 1
		v0, v4, v8, v12 = gVec4(v0, v4, v8, v12, &t[b][2], &t[b][6])
		v1, v5, v9, v13 = gVec4(v1, v5, v9, v13, &t[b][3], &t[b][10])
		v2, v6, v10, v14 = gVec4(v2, v6, v10, v14, &t[b][7], &t[b][0])
		v3, v7, v11, v15 = gVec4(v3, v7, v11, v15, &t[b][4], &t[b][13])
		v0, v5, v10, v15 = gVec4(v0, v5, v10, v15, &t[b][1], &t[b][11])
		v1, v6, v11, v12 = gVec4(v1, v6, v11, v12, &t[b][12], &t[b][5])
		v2, v7, v8, v13 = gVec4(v2, v7, v8, v13, &t[b][9], &t[b][14])
		v3, v4, v9, v14 = gVec4(v3, v4, v9, v14, &t[b][15], &t[b][8])
		// round 2
		v0, v4, v8, v12 = gVec4(v0, v4, v8, v12, &t[b][3], &t[b][4])
		v1, v5, v9, v13 = gVec4(v1, v5, v9, v13, &t[b][10], &t[b][12])
		v2, v6, v10, v14 = gVec4(v2, v6, v10, v14, &t[b][13], &t[b][2])
		v3, v7, v11, v15 = gVec4(v3, v7, v11, v15, &t[b][7], &t[b][14])
		v0, v5, v10, v15 = gVec4(v0, v5, v10, v15, &t[b][6], &t[b][5])
		v1, v6, v11, v12 = gVec4(v1, v6, v11, v12, &t[b][9], &t[b][0])
		v2, v7, v8, v13 = gVec4(v2, v7, v8, v13, &t[b][11], &t[b][15])
		v3, v4, v9, v14 = gVec4(v3, v4, v9, v14, &t[b][8], &t[b][1])
		// round 3
		v0, v4, v8, v12 = gVec4(v0, v4, v8, v12, &t[b][10], &t[b][7])
		v1, v5, v9, v13 = gVec4(v1, v5, v9, v13, &t[b][12], &t[b][9])
		v2, v6, v10, v14 = gVec4(v2, v6, v10, v14, &t[b][14], &t[b][3])
		v3, v7, v11, v15 = gVec4(v3, v7, v11, v15, &t[b][13], &t[b][15])
		v0, v5, v10, v15 = gVec4(v0, v5, v10, v15, &t[b][4], &t[b][0])
		v1, v6, v11, v12 = gVec4(v1, v6, v11, v12, &t[b][11], &t[b][2])
		v2, v7, v8, v13 = gVec4(v2, v7, v8, v13, &t[b][5], &t[b][8])
		v3, v4, v9, v14 = gVec4(v3, v4, v9, v14, &t[b][1], &t[b][6])
		// round 4
		v0, v4, v8, v12 = gVec4(v0, v4, v8, v12, &t[b][12], &t[b][13])
		v1, v5, v9, v13 = gVec4(v1, v5, v9, v13, &t[b][9], &t[b][11])
		v2, v6, v10, v14 = gVec4(v2, v6, v10, v14, &t[b][15], &t[b][10])
		v3, v7, v11, v15 = gVec4(v3, v7, v11, v15, &t[b][14], &t[b][8])
		v0, v5, v10, v15 = gVec4(v0, v5, v10, v15, &t[b][7], &t[b][2])
		v1, v6, v11, v12 = gVec4(v1, v6, v11, v12, &t[b][5], &t[b][3])
		v2, v7, v8, v13 = gVec4(v2, v7, v8, v13, &t[b][0], &t[b][1])
		v3, v4, v9, v14 = gVec4(v3, v4, v9, v14, &t[b][6], &t[b][4])
		// round 5
		v0, v4, v8, v12 = gVec4(v0, v4, v8, v12, &t[b][9], &t[b][14])
		v1, v5, v9, v13 = gVec4(v1, v5, v9, v13, &t[b][11], &t[b][5])
		v2, v6, v10, v14 = gVec4(v2, v6, v10, v14, &t[b][8], &t[b][12])
		v3, v7, v11, v15 = gVec4(v3, v7, v11, v15, &t[b][15], &t[b][1])
		v0, v5, v10, v15 = gVec4(v0, v5, v10, v15, &t[b][13], &t[b][3])
		v1, v6, v11, v12 = gVec4(v1, v6, v11, v12, &t[b][0], &t[b][10])
		v2, v7, v8, v13 = gVec4(v2, v7, v8, v13, &t[b][2], &t[b][6])
		v3, v4, v9, v14 = gVec4(v3, v4, v9, v14, &t[b][4], &t[b][7])
		// round 6
		v0, v4, v8, v12 = gVec4(v0, v4, v8, v12, &t[b][11], &t[b][15])
		v1, v5, v9, v13 = gVec4(v1, v5, v9, v13, &t[b][5], &t[b][0])
		v2, v6, v10, v14 = gVec4(v2, v6, v10, v14, &t[b][1], &t[b][9])
		v3, v7, v11, v15 = gVec4(v3, v7, v11, v15, &t[b][8], &t[b][6])
		v0, v5, v10, v15 = gVec4(v0, v5, v10, v15, &t[b][14], &t[b][10])
		v1, v6, v11, v12 = gVec4(v1, v6, v11, v12, &t[b][2], &t[b][12])
		v2, v7, v8, v13 = gVec4(v2, v7, v8, v13, &t[b][3], &t[b][4])
		v3, v4, v9, v14 = gVec4(v3, v4, v9, v14, &t[b][7], &t[b][13])

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
	var lane [4]uint32
	cv0.Store(lane[:])
	for j := 0; j < lanes; j++ {
		cvs[base+j][0] = lane[j]
	}
	cv1.Store(lane[:])
	for j := 0; j < lanes; j++ {
		cvs[base+j][1] = lane[j]
	}
	cv2.Store(lane[:])
	for j := 0; j < lanes; j++ {
		cvs[base+j][2] = lane[j]
	}
	cv3.Store(lane[:])
	for j := 0; j < lanes; j++ {
		cvs[base+j][3] = lane[j]
	}
	cv4.Store(lane[:])
	for j := 0; j < lanes; j++ {
		cvs[base+j][4] = lane[j]
	}
	cv5.Store(lane[:])
	for j := 0; j < lanes; j++ {
		cvs[base+j][5] = lane[j]
	}
	cv6.Store(lane[:])
	for j := 0; j < lanes; j++ {
		cvs[base+j][6] = lane[j]
	}
	cv7.Store(lane[:])
	for j := 0; j < lanes; j++ {
		cvs[base+j][7] = lane[j]
	}
}

// compressChunksAvx2 hashes the first `lanes` (1..simdLanes) full 1024-byte
// chunks at indices base..base+lanes-1 (lane j = chunk base+j) and writes their
// chaining values into cvs. counterBase is the BLAKE3 chunk counter of data[0].
// The chunk at index base+lanes-1 is hashed for its first `blocks` blocks; all
// other chunks are hashed for all 16. endFlag marks which block carries
// CHUNK_END: blocks-1 when set (a full chunk), none when clear (computing the
// pre-final CV of a full chunk for its output node). The unused lanes are
// zero-filled and their outputs discarded, so partial batches work.
//
// The message data for the hashed blocks is transposed into the lane-major
// layout (t[b][i] lane j = word i of block b of chunk base+j) once, before the
// block loop, so the hot loop does no scalar gather. Within the loop the 16
// state vectors are the only live vector values (they fit the 16 YMM
// registers) and the messages are read through gVec's memory-operand loads, so
// the register pressure Go's allocator otherwise resolves by spilling state is
// largely avoided.
func compressChunksAvx2(data []byte, base int, counterBase uint64, cvs [][8]uint32, key [8]uint32, flags uint32, lanes, blocks int, endFlag bool) {
	// Transpose the message data once: t[b][i][j] = little-endian word i of the
	// 64-byte block b of chunk base+j, for the first `lanes` chunks. The
	// remaining lanes stay zero, so the compression is well-defined on them.
	var t [16][16][simdLanes]uint32
	for b := 0; b < 16; b++ {
		for j := 0; j < lanes; j++ {
			blk := data[(base+j)*chunkLen+b*blockLen:]
			for i := 0; i < 16; i++ {
				t[b][i][j] = binary.LittleEndian.Uint32(blk[i*4:])
			}
		}
	}

	// Per-lane chunk counters (chunk index), split into low/high 32 bits.
	var ctrLo, ctrHi [simdLanes]uint32
	for j := 0; j < lanes; j++ {
		c := counterBase + uint64(base+j)
		ctrLo[j] = uint32(c)
		ctrHi[j] = uint32(c >> 32)
	}
	vCtrLo := archsimd.LoadUint32x8(ctrLo[:])
	vCtrHi := archsimd.LoadUint32x8(ctrHi[:])
	vBlockLen := bcast(blockLen)

	// Chaining value and IV, one broadcast vector per word. These are built
	// once per call (the 8-scalar-store broadcast cost is amortized over the
	// whole batch) rather than re-broadcast in the loop.
	cv0 := bcast(key[0])
	cv1 := bcast(key[1])
	cv2 := bcast(key[2])
	cv3 := bcast(key[3])
	cv4 := bcast(key[4])
	cv5 := bcast(key[5])
	cv6 := bcast(key[6])
	cv7 := bcast(key[7])
	iv0 := bcast(iv[0])
	iv1 := bcast(iv[1])
	iv2 := bcast(iv[2])
	iv3 := bcast(iv[3])

	// The per-block flag vector takes only three distinct values (chunk start,
	// middle blocks, chunk end), so broadcast each once and select in the loop.
	vFlagsStart := bcast(flags | flagChunkStart)
	vFlagsMid := bcast(flags)
	vFlagsEnd := bcast(flags | flagChunkEnd)

	// A full chunk is 16 blocks of 64 bytes; block 0 carries CHUNK_START and
	// (for a full 16-block hash) block 15 carries CHUNK_END, mirroring
	// chunkState.update + output().
	for b := 0; b < blocks; b++ {
		v15 := vFlagsMid
		if b == 0 {
			v15 = vFlagsStart
		} else if endFlag && b == blocks-1 {
			v15 = vFlagsEnd
		}

		v0, v1, v2, v3 := cv0, cv1, cv2, cv3
		v4, v5, v6, v7 := cv4, cv5, cv6, cv7
		v8, v9, v10, v11 := iv0, iv1, iv2, iv3
		v12, v13, v14 := vCtrLo, vCtrHi, vBlockLen

		// 7 unrolled rounds; each line is one gVec with the static message
		// schedule index. The message operands are pointers into the
		// transposed buffer, read fresh per round.
		// round 0
		v0, v4, v8, v12 = gVec(v0, v4, v8, v12, &t[b][0], &t[b][1])
		v1, v5, v9, v13 = gVec(v1, v5, v9, v13, &t[b][2], &t[b][3])
		v2, v6, v10, v14 = gVec(v2, v6, v10, v14, &t[b][4], &t[b][5])
		v3, v7, v11, v15 = gVec(v3, v7, v11, v15, &t[b][6], &t[b][7])
		v0, v5, v10, v15 = gVec(v0, v5, v10, v15, &t[b][8], &t[b][9])
		v1, v6, v11, v12 = gVec(v1, v6, v11, v12, &t[b][10], &t[b][11])
		v2, v7, v8, v13 = gVec(v2, v7, v8, v13, &t[b][12], &t[b][13])
		v3, v4, v9, v14 = gVec(v3, v4, v9, v14, &t[b][14], &t[b][15])
		// round 1
		v0, v4, v8, v12 = gVec(v0, v4, v8, v12, &t[b][2], &t[b][6])
		v1, v5, v9, v13 = gVec(v1, v5, v9, v13, &t[b][3], &t[b][10])
		v2, v6, v10, v14 = gVec(v2, v6, v10, v14, &t[b][7], &t[b][0])
		v3, v7, v11, v15 = gVec(v3, v7, v11, v15, &t[b][4], &t[b][13])
		v0, v5, v10, v15 = gVec(v0, v5, v10, v15, &t[b][1], &t[b][11])
		v1, v6, v11, v12 = gVec(v1, v6, v11, v12, &t[b][12], &t[b][5])
		v2, v7, v8, v13 = gVec(v2, v7, v8, v13, &t[b][9], &t[b][14])
		v3, v4, v9, v14 = gVec(v3, v4, v9, v14, &t[b][15], &t[b][8])
		// round 2
		v0, v4, v8, v12 = gVec(v0, v4, v8, v12, &t[b][3], &t[b][4])
		v1, v5, v9, v13 = gVec(v1, v5, v9, v13, &t[b][10], &t[b][12])
		v2, v6, v10, v14 = gVec(v2, v6, v10, v14, &t[b][13], &t[b][2])
		v3, v7, v11, v15 = gVec(v3, v7, v11, v15, &t[b][7], &t[b][14])
		v0, v5, v10, v15 = gVec(v0, v5, v10, v15, &t[b][6], &t[b][5])
		v1, v6, v11, v12 = gVec(v1, v6, v11, v12, &t[b][9], &t[b][0])
		v2, v7, v8, v13 = gVec(v2, v7, v8, v13, &t[b][11], &t[b][15])
		v3, v4, v9, v14 = gVec(v3, v4, v9, v14, &t[b][8], &t[b][1])
		// round 3
		v0, v4, v8, v12 = gVec(v0, v4, v8, v12, &t[b][10], &t[b][7])
		v1, v5, v9, v13 = gVec(v1, v5, v9, v13, &t[b][12], &t[b][9])
		v2, v6, v10, v14 = gVec(v2, v6, v10, v14, &t[b][14], &t[b][3])
		v3, v7, v11, v15 = gVec(v3, v7, v11, v15, &t[b][13], &t[b][15])
		v0, v5, v10, v15 = gVec(v0, v5, v10, v15, &t[b][4], &t[b][0])
		v1, v6, v11, v12 = gVec(v1, v6, v11, v12, &t[b][11], &t[b][2])
		v2, v7, v8, v13 = gVec(v2, v7, v8, v13, &t[b][5], &t[b][8])
		v3, v4, v9, v14 = gVec(v3, v4, v9, v14, &t[b][1], &t[b][6])
		// round 4
		v0, v4, v8, v12 = gVec(v0, v4, v8, v12, &t[b][12], &t[b][13])
		v1, v5, v9, v13 = gVec(v1, v5, v9, v13, &t[b][9], &t[b][11])
		v2, v6, v10, v14 = gVec(v2, v6, v10, v14, &t[b][15], &t[b][10])
		v3, v7, v11, v15 = gVec(v3, v7, v11, v15, &t[b][14], &t[b][8])
		v0, v5, v10, v15 = gVec(v0, v5, v10, v15, &t[b][7], &t[b][2])
		v1, v6, v11, v12 = gVec(v1, v6, v11, v12, &t[b][5], &t[b][3])
		v2, v7, v8, v13 = gVec(v2, v7, v8, v13, &t[b][0], &t[b][1])
		v3, v4, v9, v14 = gVec(v3, v4, v9, v14, &t[b][6], &t[b][4])
		// round 5
		v0, v4, v8, v12 = gVec(v0, v4, v8, v12, &t[b][9], &t[b][14])
		v1, v5, v9, v13 = gVec(v1, v5, v9, v13, &t[b][11], &t[b][5])
		v2, v6, v10, v14 = gVec(v2, v6, v10, v14, &t[b][8], &t[b][12])
		v3, v7, v11, v15 = gVec(v3, v7, v11, v15, &t[b][15], &t[b][1])
		v0, v5, v10, v15 = gVec(v0, v5, v10, v15, &t[b][13], &t[b][3])
		v1, v6, v11, v12 = gVec(v1, v6, v11, v12, &t[b][0], &t[b][10])
		v2, v7, v8, v13 = gVec(v2, v7, v8, v13, &t[b][2], &t[b][6])
		v3, v4, v9, v14 = gVec(v3, v4, v9, v14, &t[b][4], &t[b][7])
		// round 6
		v0, v4, v8, v12 = gVec(v0, v4, v8, v12, &t[b][11], &t[b][15])
		v1, v5, v9, v13 = gVec(v1, v5, v9, v13, &t[b][5], &t[b][0])
		v2, v6, v10, v14 = gVec(v2, v6, v10, v14, &t[b][1], &t[b][9])
		v3, v7, v11, v15 = gVec(v3, v7, v11, v15, &t[b][8], &t[b][6])
		v0, v5, v10, v15 = gVec(v0, v5, v10, v15, &t[b][14], &t[b][10])
		v1, v6, v11, v12 = gVec(v1, v6, v11, v12, &t[b][2], &t[b][12])
		v2, v7, v8, v13 = gVec(v2, v7, v8, v13, &t[b][3], &t[b][4])
		v3, v4, v9, v14 = gVec(v3, v4, v9, v14, &t[b][7], &t[b][13])

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
	for j := 0; j < lanes; j++ {
		cvs[base+j][0] = lane[j]
	}
	cv1.Store(lane[:])
	for j := 0; j < lanes; j++ {
		cvs[base+j][1] = lane[j]
	}
	cv2.Store(lane[:])
	for j := 0; j < lanes; j++ {
		cvs[base+j][2] = lane[j]
	}
	cv3.Store(lane[:])
	for j := 0; j < lanes; j++ {
		cvs[base+j][3] = lane[j]
	}
	cv4.Store(lane[:])
	for j := 0; j < lanes; j++ {
		cvs[base+j][4] = lane[j]
	}
	cv5.Store(lane[:])
	for j := 0; j < lanes; j++ {
		cvs[base+j][5] = lane[j]
	}
	cv6.Store(lane[:])
	for j := 0; j < lanes; j++ {
		cvs[base+j][6] = lane[j]
	}
	cv7.Store(lane[:])
	for j := 0; j < lanes; j++ {
		cvs[base+j][7] = lane[j]
	}
}

// compressOutputsAvx2 computes simdLanes 64-byte root output blocks starting
// at block start. The message block and input CV are constant across lanes;
// only the counter varies. The message words are read through gVec's
// memory-operand loads from the broadcast buffer m (built once by
// compressOutputs), keeping the 16 state vectors the only live vector values.
func compressOutputsAvx2(out []byte, cv *[8]uint32, m *[16][simdLanes]uint32, blkLen, flags uint32, start uint64) {
	var ctrLo, ctrHi [simdLanes]uint32
	for j := 0; j < simdLanes; j++ {
		c := start + uint64(j)
		ctrLo[j] = uint32(c)
		ctrHi[j] = uint32(c >> 32)
	}
	vCtrLo := archsimd.LoadUint32x8(ctrLo[:])
	vCtrHi := archsimd.LoadUint32x8(ctrHi[:])
	vBlockLen := bcast(blkLen)
	vFlags := bcast(flags)

	cv0 := bcast(cv[0])
	cv1 := bcast(cv[1])
	cv2 := bcast(cv[2])
	cv3 := bcast(cv[3])
	cv4 := bcast(cv[4])
	cv5 := bcast(cv[5])
	cv6 := bcast(cv[6])
	cv7 := bcast(cv[7])
	iv0 := bcast(iv[0])
	iv1 := bcast(iv[1])
	iv2 := bcast(iv[2])
	iv3 := bcast(iv[3])

	v0, v1, v2, v3 := cv0, cv1, cv2, cv3
	v4, v5, v6, v7 := cv4, cv5, cv6, cv7
	v8, v9, v10, v11 := iv0, iv1, iv2, iv3
	v12, v13, v14, v15 := vCtrLo, vCtrHi, vBlockLen, vFlags

	// 7 unrolled rounds; the message schedule is the same as the chunk kernel.
	// round 0
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, &m[0], &m[1])
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, &m[2], &m[3])
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, &m[4], &m[5])
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, &m[6], &m[7])
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, &m[8], &m[9])
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, &m[10], &m[11])
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, &m[12], &m[13])
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, &m[14], &m[15])
	// round 1
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, &m[2], &m[6])
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, &m[3], &m[10])
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, &m[7], &m[0])
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, &m[4], &m[13])
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, &m[1], &m[11])
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, &m[12], &m[5])
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, &m[9], &m[14])
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, &m[15], &m[8])
	// round 2
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, &m[3], &m[4])
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, &m[10], &m[12])
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, &m[13], &m[2])
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, &m[7], &m[14])
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, &m[6], &m[5])
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, &m[9], &m[0])
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, &m[11], &m[15])
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, &m[8], &m[1])
	// round 3
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, &m[10], &m[7])
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, &m[12], &m[9])
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, &m[14], &m[3])
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, &m[13], &m[15])
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, &m[4], &m[0])
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, &m[11], &m[2])
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, &m[5], &m[8])
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, &m[1], &m[6])
	// round 4
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, &m[12], &m[13])
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, &m[9], &m[11])
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, &m[15], &m[10])
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, &m[14], &m[8])
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, &m[7], &m[2])
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, &m[5], &m[3])
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, &m[0], &m[1])
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, &m[6], &m[4])
	// round 5
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, &m[9], &m[14])
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, &m[11], &m[5])
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, &m[8], &m[12])
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, &m[15], &m[1])
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, &m[13], &m[3])
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, &m[0], &m[10])
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, &m[2], &m[6])
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, &m[4], &m[7])
	// round 6
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, &m[11], &m[15])
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, &m[5], &m[0])
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, &m[1], &m[9])
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, &m[8], &m[6])
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, &m[14], &m[10])
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, &m[2], &m[12])
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, &m[3], &m[4])
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, &m[7], &m[13])

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

	// Scatter the word-major output vectors into the block-major byte layout:
	// word i of block j lands at out[j*64+i*4].
	var lane [simdLanes]uint32
	v0.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen:], lane[j])
	}
	v1.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+4:], lane[j])
	}
	v2.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+8:], lane[j])
	}
	v3.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+12:], lane[j])
	}
	v4.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+16:], lane[j])
	}
	v5.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+20:], lane[j])
	}
	v6.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+24:], lane[j])
	}
	v7.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+28:], lane[j])
	}
	v8.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+32:], lane[j])
	}
	v9.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+36:], lane[j])
	}
	v10.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+40:], lane[j])
	}
	v11.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+44:], lane[j])
	}
	v12.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+48:], lane[j])
	}
	v13.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+52:], lane[j])
	}
	v14.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+56:], lane[j])
	}
	v15.Store(lane[:])
	for j := 0; j < simdLanes; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+60:], lane[j])
	}
}

// mergeParentCVs computes the parent chaining value of each pair (src[2i],
// src[2i+1]) into out[i], using the 8-lane AVX2 parent kernel in batches.
func mergeParentCVs(out, src [][8]uint32, key [8]uint32, flags uint32) {
	if !archsimd.X86.AVX2() {
		for i := 0; i < len(out); i++ {
			out[i] = parentCV(src[2*i], src[2*i+1], key, flags)
		}
		return
	}
	// Prefer the 16-lane AVX-512 parent kernel when available.
	if archsimd.X86.AVX512() {
		n := len(out)
		i := 0
		for ; i+simdLanesAvx512 <= n; i += simdLanesAvx512 {
			var left, right [simdLanesAvx512][8]uint32
			for j := 0; j < simdLanesAvx512; j++ {
				left[j] = src[2*(i+j)]
				right[j] = src[2*(i+j)+1]
			}
			cvs := compressParentsAvx512(&left, &right, key, flags)
			for j := 0; j < simdLanesAvx512; j++ {
				out[i+j] = cvs[j]
			}
		}
		for ; i < n; i++ {
			out[i] = parentCV(src[2*i], src[2*i+1], key, flags)
		}
		return
	}
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

// compressParentsLanes computes the chaining value of 8 parent nodes in one
// AVX2 batch. Parent j's block is [left[j], right[j]]; the message is already
// lane-major (no transpose needed), the counter is 0, and the flags carry
// flagParent. Only the first 8 output words (the CV) are needed.
func compressParentsLanes(left, right *[simdLanes][8]uint32, key [8]uint32, flags uint32) [simdLanes][8]uint32 {
	// Lane-major message buffer: m[i] lane j = word i of parent j. Passed as
	// pointers to gVec so the messages are read from memory per round.
	var scratch [16][simdLanes]uint32
	for j := 0; j < simdLanes; j++ {
		for w := 0; w < 8; w++ {
			scratch[w][j] = left[j][w]
			scratch[w+8][j] = right[j][w]
		}
	}

	cv0 := bcast(key[0])
	cv1 := bcast(key[1])
	cv2 := bcast(key[2])
	cv3 := bcast(key[3])
	cv4 := bcast(key[4])
	cv5 := bcast(key[5])
	cv6 := bcast(key[6])
	cv7 := bcast(key[7])
	iv0 := bcast(iv[0])
	iv1 := bcast(iv[1])
	iv2 := bcast(iv[2])
	iv3 := bcast(iv[3])

	v0, v1, v2, v3 := cv0, cv1, cv2, cv3
	v4, v5, v6, v7 := cv4, cv5, cv6, cv7
	v8, v9, v10, v11 := iv0, iv1, iv2, iv3
	v12 := bcast(0)
	v13 := bcast(0)
	v14 := bcast(blockLen)
	v15 := bcast(flags | flagParent)

	// round 0
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, &scratch[0], &scratch[1])
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, &scratch[2], &scratch[3])
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, &scratch[4], &scratch[5])
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, &scratch[6], &scratch[7])
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, &scratch[8], &scratch[9])
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, &scratch[10], &scratch[11])
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, &scratch[12], &scratch[13])
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, &scratch[14], &scratch[15])
	// round 1
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, &scratch[2], &scratch[6])
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, &scratch[3], &scratch[10])
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, &scratch[7], &scratch[0])
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, &scratch[4], &scratch[13])
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, &scratch[1], &scratch[11])
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, &scratch[12], &scratch[5])
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, &scratch[9], &scratch[14])
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, &scratch[15], &scratch[8])
	// round 2
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, &scratch[3], &scratch[4])
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, &scratch[10], &scratch[12])
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, &scratch[13], &scratch[2])
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, &scratch[7], &scratch[14])
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, &scratch[6], &scratch[5])
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, &scratch[9], &scratch[0])
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, &scratch[11], &scratch[15])
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, &scratch[8], &scratch[1])
	// round 3
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, &scratch[10], &scratch[7])
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, &scratch[12], &scratch[9])
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, &scratch[14], &scratch[3])
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, &scratch[13], &scratch[15])
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, &scratch[4], &scratch[0])
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, &scratch[11], &scratch[2])
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, &scratch[5], &scratch[8])
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, &scratch[1], &scratch[6])
	// round 4
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, &scratch[12], &scratch[13])
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, &scratch[9], &scratch[11])
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, &scratch[15], &scratch[10])
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, &scratch[14], &scratch[8])
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, &scratch[7], &scratch[2])
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, &scratch[5], &scratch[3])
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, &scratch[0], &scratch[1])
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, &scratch[6], &scratch[4])
	// round 5
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, &scratch[9], &scratch[14])
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, &scratch[11], &scratch[5])
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, &scratch[8], &scratch[12])
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, &scratch[15], &scratch[1])
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, &scratch[13], &scratch[3])
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, &scratch[0], &scratch[10])
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, &scratch[2], &scratch[6])
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, &scratch[4], &scratch[7])
	// round 6
	v0, v4, v8, v12 = gVec(v0, v4, v8, v12, &scratch[11], &scratch[15])
	v1, v5, v9, v13 = gVec(v1, v5, v9, v13, &scratch[5], &scratch[0])
	v2, v6, v10, v14 = gVec(v2, v6, v10, v14, &scratch[1], &scratch[9])
	v3, v7, v11, v15 = gVec(v3, v7, v11, v15, &scratch[8], &scratch[6])
	v0, v5, v10, v15 = gVec(v0, v5, v10, v15, &scratch[14], &scratch[10])
	v1, v6, v11, v12 = gVec(v1, v6, v11, v12, &scratch[2], &scratch[12])
	v2, v7, v8, v13 = gVec(v2, v7, v8, v13, &scratch[3], &scratch[4])
	v3, v4, v9, v14 = gVec(v3, v4, v9, v14, &scratch[7], &scratch[13])

	// CV update: word i = v[i]^v[i+8], then scatter across the 8 parents.
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
