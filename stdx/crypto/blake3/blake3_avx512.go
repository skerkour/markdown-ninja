//go:build amd64 && goexperiment.simd

package blake3

import (
	"encoding/binary"
	"simd/archsimd"
	"unsafe"
)

// This file is the experimental, cgo-free, assembly-free SIMD acceleration of
// BLAKE3's per-chunk compression for amd64 with AVX-512, written with Go's
// experimental simd/archsimd intrinsics. It only builds on amd64 under
// GOEXPERIMENT=simd, and the dispatch in blake3_simd_amd64.go gates it at
// runtime on archsimd.X86.AVX512(); CPUs without AVX-512 fall back to the AVX2
// or scalar paths.
//
// 512-bit registers hold 16 uint32 lanes, so we hash 16 full 1024-byte chunks
// at once — one chunk per lane. The 16 compression state words become 16
// vectors; the quarter-round mixing is pure elementwise Add/Xor, and each
// 16/12/8/7-bit right rotation is a single VPRORVD (Uint32x16.RotateRight
// against a broadcast count vector). The message and state vectors are named
// locals and the 7 rounds are fully unrolled. Like chacha20's AVX-512 core, the
// 16 state vectors and 4 rotation-count vectors occupy 20 of the 32 ZMM
// registers, leaving room for the message vectors and temporaries so the round
// loop spills as little as possible. The per-block message is gathered with a
// scalar transpose into the lane-major layout (m[i] lane j = word i of chunk
// j). Results are bit-identical to the scalar path.

// simdLanesAvx512 is the number of chunks (or output blocks) per AVX-512 batch.
const simdLanesAvx512 = 16

// rotCounts holds the broadcast rotation counts backing the four VPRORVD
// rotations of each quarter round. They are plain data (no SIMD instructions at
// init, so the package initializes on CPUs without AVX-512); the archsimd
// vectors are materialized lazily inside the AVX-512 kernels.
var rotCounts = [4][16]uint32{
	{16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16},
	{12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12},
	{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8},
	{7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7},
}

// permLo128Idx and permHi128Idx encode the two _mm512_shuffle_i32x4 stages
// (imm 0x88 / 0xdd) of the reference 16x16 transpose as VPERMT2D
// (ConcatPermute) index vectors over the concatenation of the two 512-bit
// sources. permLo128Idx selects {a.128[0], a.128[2], b.128[0], b.128[2]};
// permHi128Idx selects {a.128[1], a.128[3], b.128[1], b.128[3]}.
var (
	permLo128Idx = [16]uint32{
		0, 1, 2, 3, 8, 9, 10, 11, 16, 17, 18, 19, 24, 25, 26, 27,
	}
	permHi128Idx = [16]uint32{
		4, 5, 6, 7, 12, 13, 14, 15, 20, 21, 22, 23, 28, 29, 30, 31,
	}
)

// interleaveLo64/interleaveHi64 interleave the 64-bit lanes of x and y within
// each 128-bit subvector (VPUNPCKLQDQ / VPUNPCKHQDQ), through a 64-bit reshape
// so the archsimd op lowers to the right instruction.
func interleaveLo64(x, y archsimd.Uint32x16) archsimd.Uint32x16 {
	return x.ReshapeToUint64s().InterleaveLoGrouped(y.ReshapeToUint64s()).ReshapeToUint32s()
}

func interleaveHi64(x, y archsimd.Uint32x16) archsimd.Uint32x16 {
	return x.ReshapeToUint64s().InterleaveHiGrouped(y.ReshapeToUint64s()).ReshapeToUint32s()
}

// transposeVecs512 performs the reference 16x16 word-matrix transpose on 16
// 512-bit vectors (each input holds one row of 16 words), producing 16 vectors
// where output i holds word i across all 16 inputs. It mirrors the reference
// transpose_vecs_512: VPUNPCKLDQ/HDQ (32-bit lanes), then VPUNPCKLQDQ/HQDQ
// (64-bit lanes), then two 128-bit interleave stages (VSHUFI32X4, expressed
// here as VPERMT2D/ConcatPermute). Like the reference, the result carries a
// fixed lane permutation that is its own inverse, so applying it to the chunk
// outputs undoes the permutation applied to the messages.
func transposeVecs512(
	a, b, c, d, e, f, g, h, i, j, k, l, m, n, o, p archsimd.Uint32x16,
) (r0, r1, r2, r3, r4, r5, r6, r7, r8, r9, r10, r11, r12, r13, r14, r15 archsimd.Uint32x16) {
	lo128 := archsimd.LoadUint32x16Array(&permLo128Idx)
	hi128 := archsimd.LoadUint32x16Array(&permHi128Idx)

	// Interleave 32-bit lanes within each 128-bit subvector.
	ab0 := a.InterleaveLoGrouped(b)
	ab2 := a.InterleaveHiGrouped(b)
	cd0 := c.InterleaveLoGrouped(d)
	cd2 := c.InterleaveHiGrouped(d)
	ef0 := e.InterleaveLoGrouped(f)
	ef2 := e.InterleaveHiGrouped(f)
	gh0 := g.InterleaveLoGrouped(h)
	gh2 := g.InterleaveHiGrouped(h)
	ij0 := i.InterleaveLoGrouped(j)
	ij2 := i.InterleaveHiGrouped(j)
	kl0 := k.InterleaveLoGrouped(l)
	kl2 := k.InterleaveHiGrouped(l)
	mn0 := m.InterleaveLoGrouped(n)
	mn2 := m.InterleaveHiGrouped(n)
	op0 := o.InterleaveLoGrouped(p)
	op2 := o.InterleaveHiGrouped(p)

	// Interleave 64-bit lanes within each 128-bit subvector.
	abcd0 := interleaveLo64(ab0, cd0)
	abcd1 := interleaveHi64(ab0, cd0)
	abcd2 := interleaveLo64(ab2, cd2)
	abcd3 := interleaveHi64(ab2, cd2)
	efgh0 := interleaveLo64(ef0, gh0)
	efgh1 := interleaveHi64(ef0, gh0)
	efgh2 := interleaveLo64(ef2, gh2)
	efgh3 := interleaveHi64(ef2, gh2)
	ijkl0 := interleaveLo64(ij0, kl0)
	ijkl1 := interleaveHi64(ij0, kl0)
	ijkl2 := interleaveLo64(ij2, kl2)
	ijkl3 := interleaveHi64(ij2, kl2)
	mnop0 := interleaveLo64(mn0, op0)
	mnop1 := interleaveHi64(mn0, op0)
	mnop2 := interleaveLo64(mn2, op2)
	mnop3 := interleaveHi64(mn2, op2)

	// Interleave 128-bit lanes (first stage).
	abcdefgh0 := abcd0.ConcatPermute(efgh0, lo128)
	abcdefgh1 := abcd1.ConcatPermute(efgh1, lo128)
	abcdefgh2 := abcd2.ConcatPermute(efgh2, lo128)
	abcdefgh3 := abcd3.ConcatPermute(efgh3, lo128)
	abcdefgh4 := abcd0.ConcatPermute(efgh0, hi128)
	abcdefgh5 := abcd1.ConcatPermute(efgh1, hi128)
	abcdefgh6 := abcd2.ConcatPermute(efgh2, hi128)
	abcdefgh7 := abcd3.ConcatPermute(efgh3, hi128)
	ijklmnop0 := ijkl0.ConcatPermute(mnop0, lo128)
	ijklmnop1 := ijkl1.ConcatPermute(mnop1, lo128)
	ijklmnop2 := ijkl2.ConcatPermute(mnop2, lo128)
	ijklmnop3 := ijkl3.ConcatPermute(mnop3, lo128)
	ijklmnop4 := ijkl0.ConcatPermute(mnop0, hi128)
	ijklmnop5 := ijkl1.ConcatPermute(mnop1, hi128)
	ijklmnop6 := ijkl2.ConcatPermute(mnop2, hi128)
	ijklmnop7 := ijkl3.ConcatPermute(mnop3, hi128)

	// Interleave 128-bit lanes (final stage).
	r0 = abcdefgh0.ConcatPermute(ijklmnop0, lo128)
	r1 = abcdefgh1.ConcatPermute(ijklmnop1, lo128)
	r2 = abcdefgh2.ConcatPermute(ijklmnop2, lo128)
	r3 = abcdefgh3.ConcatPermute(ijklmnop3, lo128)
	r4 = abcdefgh4.ConcatPermute(ijklmnop4, lo128)
	r5 = abcdefgh5.ConcatPermute(ijklmnop5, lo128)
	r6 = abcdefgh6.ConcatPermute(ijklmnop6, lo128)
	r7 = abcdefgh7.ConcatPermute(ijklmnop7, lo128)
	r8 = abcdefgh0.ConcatPermute(ijklmnop0, hi128)
	r9 = abcdefgh1.ConcatPermute(ijklmnop1, hi128)
	r10 = abcdefgh2.ConcatPermute(ijklmnop2, hi128)
	r11 = abcdefgh3.ConcatPermute(ijklmnop3, hi128)
	r12 = abcdefgh4.ConcatPermute(ijklmnop4, hi128)
	r13 = abcdefgh5.ConcatPermute(ijklmnop5, hi128)
	r14 = abcdefgh6.ConcatPermute(ijklmnop6, hi128)
	r15 = abcdefgh7.ConcatPermute(ijklmnop7, hi128)
	return r0, r1, r2, r3, r4, r5, r6, r7, r8, r9, r10, r11, r12, r13, r14, r15
}

// bcast512 returns a vector with x in every lane (a single VPBROADCASTD).
func bcast512(x uint32) archsimd.Uint32x16 {
	return archsimd.BroadcastUint32x16(x)
}

// gVec512 is the BLAKE3 quarter round on 16 lanes at once. The rotations are
// right by 16/12/8/7, matching the scalar RotateLeft32(x, -k), each a single
// VPRORVD against the loop-invariant count vectors r16/r12/r8/r7.
//
// mx and my point at the lane-major message rows for the two message words of
// this column. The 16 state vectors plus the 4 rotation-count vectors already
// fill most of the 32 ZMM registers. The message is loaded with an array load
// (no bounds-checked slice header) written as the leftmost, single-use operand
// of the first VPADDD, so the compiler consumes it directly from the load
// without routing it through a spill slot and reloading it.
func gVec512(va, vb, vc, vd archsimd.Uint32x16, mx, my *[16]uint32, r16, r12, r8, r7 archsimd.Uint32x16) (archsimd.Uint32x16, archsimd.Uint32x16, archsimd.Uint32x16, archsimd.Uint32x16) {
	va = archsimd.LoadUint32x16Array(mx).Add(va).Add(vb)
	vd = vd.Xor(va).RotateRight(r16)
	vc = vc.Add(vd)
	vb = vb.Xor(vc).RotateRight(r12)
	va = archsimd.LoadUint32x16Array(my).Add(va).Add(vb)
	vd = vd.Xor(va).RotateRight(r8)
	vc = vc.Add(vd)
	vb = vb.Xor(vc).RotateRight(r7)
	return va, vb, vc, vd
}

// compressChunksAvx512 hashes the first `lanes` (1..simdLanesAvx512) full
// 1024-byte chunks at indices base..base+lanes-1 (lane j = chunk base+j) and
// writes their chaining values into cvs. counterBase is the BLAKE3 chunk
// counter of data[0]. The chunk at index base+lanes-1 is hashed for its first
// `blocks` blocks; all other chunks are hashed for all 16. endFlag marks which
// block carries CHUNK_END: blocks-1 when set (a full chunk), none when clear
// (computing the pre-final CV of a full chunk for its output node). The unused
// lanes are zero-filled and their outputs discarded, so partial batches work.
//
// The message data for the hashed blocks is transposed into the lane-major
// layout (t[b][i] lane j = word i of block b of chunk base+j) once, before the
// block loop, so the hot loop does no scalar gather. Within the loop the 16
// state vectors plus the 4 rotation-count vectors are the only long-lived
// vector values and the messages are read through gVec512's memory-operand
// loads, avoiding the state spills of materializing all 16 message vectors as
// registers.
func compressChunksAvx512(data []byte, base int, counterBase uint64, cvs [][8]uint32, key [8]uint32, flags uint32, lanes, blocks int, endFlag bool) {
	// Transpose the message data once: t[b][i][j] = little-endian word i of the
	// 64-byte block b of chunk base+j, for the first `lanes` chunks. The
	// remaining lanes stay zero, so the compression is well-defined on them.
	// Full batches use the vectorized 16x16 transpose; partial batches fall
	// back to the scalar gather (they are rare and small).
	var t [16][16][simdLanesAvx512]uint32
	if lanes == simdLanesAvx512 && blocks == 16 {
		for b := 0; b < blocks; b++ {
			off := (base * chunkLen) + b*blockLen
			va := archsimd.LoadUint32x16Array((*[16]uint32)(unsafe.Pointer(&data[off])))
			off += chunkLen
			vb := archsimd.LoadUint32x16Array((*[16]uint32)(unsafe.Pointer(&data[off])))
			off += chunkLen
			vc := archsimd.LoadUint32x16Array((*[16]uint32)(unsafe.Pointer(&data[off])))
			off += chunkLen
			vd := archsimd.LoadUint32x16Array((*[16]uint32)(unsafe.Pointer(&data[off])))
			off += chunkLen
			ve := archsimd.LoadUint32x16Array((*[16]uint32)(unsafe.Pointer(&data[off])))
			off += chunkLen
			vf := archsimd.LoadUint32x16Array((*[16]uint32)(unsafe.Pointer(&data[off])))
			off += chunkLen
			vg := archsimd.LoadUint32x16Array((*[16]uint32)(unsafe.Pointer(&data[off])))
			off += chunkLen
			vh := archsimd.LoadUint32x16Array((*[16]uint32)(unsafe.Pointer(&data[off])))
			off += chunkLen
			vi := archsimd.LoadUint32x16Array((*[16]uint32)(unsafe.Pointer(&data[off])))
			off += chunkLen
			vj := archsimd.LoadUint32x16Array((*[16]uint32)(unsafe.Pointer(&data[off])))
			off += chunkLen
			vk := archsimd.LoadUint32x16Array((*[16]uint32)(unsafe.Pointer(&data[off])))
			off += chunkLen
			vl := archsimd.LoadUint32x16Array((*[16]uint32)(unsafe.Pointer(&data[off])))
			off += chunkLen
			vm := archsimd.LoadUint32x16Array((*[16]uint32)(unsafe.Pointer(&data[off])))
			off += chunkLen
			vn := archsimd.LoadUint32x16Array((*[16]uint32)(unsafe.Pointer(&data[off])))
			off += chunkLen
			vo := archsimd.LoadUint32x16Array((*[16]uint32)(unsafe.Pointer(&data[off])))
			off += chunkLen
			vp := archsimd.LoadUint32x16Array((*[16]uint32)(unsafe.Pointer(&data[off])))
			wa, wb, wc, wd, we, wf, wg, wh, wi, wj, wk, wl, wm, wn, wo, wp :=
				transposeVecs512(va, vb, vc, vd, ve, vf, vg, vh, vi, vj, vk, vl, vm, vn, vo, vp)
			wa.StoreArray(&t[b][0])
			wb.StoreArray(&t[b][1])
			wc.StoreArray(&t[b][2])
			wd.StoreArray(&t[b][3])
			we.StoreArray(&t[b][4])
			wf.StoreArray(&t[b][5])
			wg.StoreArray(&t[b][6])
			wh.StoreArray(&t[b][7])
			wi.StoreArray(&t[b][8])
			wj.StoreArray(&t[b][9])
			wk.StoreArray(&t[b][10])
			wl.StoreArray(&t[b][11])
			wm.StoreArray(&t[b][12])
			wn.StoreArray(&t[b][13])
			wo.StoreArray(&t[b][14])
			wp.StoreArray(&t[b][15])
		}
	} else {
		for b := 0; b < blocks; b++ {
			for j := 0; j < lanes; j++ {
				blk := data[(base+j)*chunkLen+b*blockLen:]
				for i := 0; i < 16; i++ {
					t[b][i][j] = binary.LittleEndian.Uint32(blk[i*4:])
				}
			}
		}
	}

	// Per-lane chunk counters (chunk index), split into low/high 32 bits.
	var ctrLo, ctrHi [simdLanesAvx512]uint32
	for j := 0; j < lanes; j++ {
		c := counterBase + uint64(base+j)
		ctrLo[j] = uint32(c)
		ctrHi[j] = uint32(c >> 32)
	}
	vCtrLo := archsimd.LoadUint32x16(ctrLo[:])
	vCtrHi := archsimd.LoadUint32x16(ctrHi[:])
	vBlockLen := bcast512(blockLen)

	// Chaining value and IV, one broadcast vector per word.
	cv0 := bcast512(key[0])
	cv1 := bcast512(key[1])
	cv2 := bcast512(key[2])
	cv3 := bcast512(key[3])
	cv4 := bcast512(key[4])
	cv5 := bcast512(key[5])
	cv6 := bcast512(key[6])
	cv7 := bcast512(key[7])
	iv0 := bcast512(iv[0])
	iv1 := bcast512(iv[1])
	iv2 := bcast512(iv[2])
	iv3 := bcast512(iv[3])

	// VPRORVD count vectors, loop-invariant.
	r16 := archsimd.LoadUint32x16Array(&rotCounts[0])
	r12 := archsimd.LoadUint32x16Array(&rotCounts[1])
	r8 := archsimd.LoadUint32x16Array(&rotCounts[2])
	r7 := archsimd.LoadUint32x16Array(&rotCounts[3])

	// The per-block flag vector takes only three distinct values (chunk start,
	// middle blocks, chunk end), so broadcast each once and select in the loop.
	vFlagsStart := bcast512(flags | flagChunkStart)
	vFlagsMid := bcast512(flags)
	vFlagsEnd := bcast512(flags | flagChunkEnd)

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

		// 7 unrolled rounds; each line is one gVec512 with the static message
		// schedule index. The message operands are pointers into the
		// transposed buffer, read fresh per round.
		// round 0
		v0, v4, v8, v12 = gVec512(v0, v4, v8, v12, &t[b][0], &t[b][1], r16, r12, r8, r7)
		v1, v5, v9, v13 = gVec512(v1, v5, v9, v13, &t[b][2], &t[b][3], r16, r12, r8, r7)
		v2, v6, v10, v14 = gVec512(v2, v6, v10, v14, &t[b][4], &t[b][5], r16, r12, r8, r7)
		v3, v7, v11, v15 = gVec512(v3, v7, v11, v15, &t[b][6], &t[b][7], r16, r12, r8, r7)
		v0, v5, v10, v15 = gVec512(v0, v5, v10, v15, &t[b][8], &t[b][9], r16, r12, r8, r7)
		v1, v6, v11, v12 = gVec512(v1, v6, v11, v12, &t[b][10], &t[b][11], r16, r12, r8, r7)
		v2, v7, v8, v13 = gVec512(v2, v7, v8, v13, &t[b][12], &t[b][13], r16, r12, r8, r7)
		v3, v4, v9, v14 = gVec512(v3, v4, v9, v14, &t[b][14], &t[b][15], r16, r12, r8, r7)
		// round 1
		v0, v4, v8, v12 = gVec512(v0, v4, v8, v12, &t[b][2], &t[b][6], r16, r12, r8, r7)
		v1, v5, v9, v13 = gVec512(v1, v5, v9, v13, &t[b][3], &t[b][10], r16, r12, r8, r7)
		v2, v6, v10, v14 = gVec512(v2, v6, v10, v14, &t[b][7], &t[b][0], r16, r12, r8, r7)
		v3, v7, v11, v15 = gVec512(v3, v7, v11, v15, &t[b][4], &t[b][13], r16, r12, r8, r7)
		v0, v5, v10, v15 = gVec512(v0, v5, v10, v15, &t[b][1], &t[b][11], r16, r12, r8, r7)
		v1, v6, v11, v12 = gVec512(v1, v6, v11, v12, &t[b][12], &t[b][5], r16, r12, r8, r7)
		v2, v7, v8, v13 = gVec512(v2, v7, v8, v13, &t[b][9], &t[b][14], r16, r12, r8, r7)
		v3, v4, v9, v14 = gVec512(v3, v4, v9, v14, &t[b][15], &t[b][8], r16, r12, r8, r7)
		// round 2
		v0, v4, v8, v12 = gVec512(v0, v4, v8, v12, &t[b][3], &t[b][4], r16, r12, r8, r7)
		v1, v5, v9, v13 = gVec512(v1, v5, v9, v13, &t[b][10], &t[b][12], r16, r12, r8, r7)
		v2, v6, v10, v14 = gVec512(v2, v6, v10, v14, &t[b][13], &t[b][2], r16, r12, r8, r7)
		v3, v7, v11, v15 = gVec512(v3, v7, v11, v15, &t[b][7], &t[b][14], r16, r12, r8, r7)
		v0, v5, v10, v15 = gVec512(v0, v5, v10, v15, &t[b][6], &t[b][5], r16, r12, r8, r7)
		v1, v6, v11, v12 = gVec512(v1, v6, v11, v12, &t[b][9], &t[b][0], r16, r12, r8, r7)
		v2, v7, v8, v13 = gVec512(v2, v7, v8, v13, &t[b][11], &t[b][15], r16, r12, r8, r7)
		v3, v4, v9, v14 = gVec512(v3, v4, v9, v14, &t[b][8], &t[b][1], r16, r12, r8, r7)
		// round 3
		v0, v4, v8, v12 = gVec512(v0, v4, v8, v12, &t[b][10], &t[b][7], r16, r12, r8, r7)
		v1, v5, v9, v13 = gVec512(v1, v5, v9, v13, &t[b][12], &t[b][9], r16, r12, r8, r7)
		v2, v6, v10, v14 = gVec512(v2, v6, v10, v14, &t[b][14], &t[b][3], r16, r12, r8, r7)
		v3, v7, v11, v15 = gVec512(v3, v7, v11, v15, &t[b][13], &t[b][15], r16, r12, r8, r7)
		v0, v5, v10, v15 = gVec512(v0, v5, v10, v15, &t[b][4], &t[b][0], r16, r12, r8, r7)
		v1, v6, v11, v12 = gVec512(v1, v6, v11, v12, &t[b][11], &t[b][2], r16, r12, r8, r7)
		v2, v7, v8, v13 = gVec512(v2, v7, v8, v13, &t[b][5], &t[b][8], r16, r12, r8, r7)
		v3, v4, v9, v14 = gVec512(v3, v4, v9, v14, &t[b][1], &t[b][6], r16, r12, r8, r7)
		// round 4
		v0, v4, v8, v12 = gVec512(v0, v4, v8, v12, &t[b][12], &t[b][13], r16, r12, r8, r7)
		v1, v5, v9, v13 = gVec512(v1, v5, v9, v13, &t[b][9], &t[b][11], r16, r12, r8, r7)
		v2, v6, v10, v14 = gVec512(v2, v6, v10, v14, &t[b][15], &t[b][10], r16, r12, r8, r7)
		v3, v7, v11, v15 = gVec512(v3, v7, v11, v15, &t[b][14], &t[b][8], r16, r12, r8, r7)
		v0, v5, v10, v15 = gVec512(v0, v5, v10, v15, &t[b][7], &t[b][2], r16, r12, r8, r7)
		v1, v6, v11, v12 = gVec512(v1, v6, v11, v12, &t[b][5], &t[b][3], r16, r12, r8, r7)
		v2, v7, v8, v13 = gVec512(v2, v7, v8, v13, &t[b][0], &t[b][1], r16, r12, r8, r7)
		v3, v4, v9, v14 = gVec512(v3, v4, v9, v14, &t[b][6], &t[b][4], r16, r12, r8, r7)
		// round 5
		v0, v4, v8, v12 = gVec512(v0, v4, v8, v12, &t[b][9], &t[b][14], r16, r12, r8, r7)
		v1, v5, v9, v13 = gVec512(v1, v5, v9, v13, &t[b][11], &t[b][5], r16, r12, r8, r7)
		v2, v6, v10, v14 = gVec512(v2, v6, v10, v14, &t[b][8], &t[b][12], r16, r12, r8, r7)
		v3, v7, v11, v15 = gVec512(v3, v7, v11, v15, &t[b][15], &t[b][1], r16, r12, r8, r7)
		v0, v5, v10, v15 = gVec512(v0, v5, v10, v15, &t[b][13], &t[b][3], r16, r12, r8, r7)
		v1, v6, v11, v12 = gVec512(v1, v6, v11, v12, &t[b][0], &t[b][10], r16, r12, r8, r7)
		v2, v7, v8, v13 = gVec512(v2, v7, v8, v13, &t[b][2], &t[b][6], r16, r12, r8, r7)
		v3, v4, v9, v14 = gVec512(v3, v4, v9, v14, &t[b][4], &t[b][7], r16, r12, r8, r7)
		// round 6
		v0, v4, v8, v12 = gVec512(v0, v4, v8, v12, &t[b][11], &t[b][15], r16, r12, r8, r7)
		v1, v5, v9, v13 = gVec512(v1, v5, v9, v13, &t[b][5], &t[b][0], r16, r12, r8, r7)
		v2, v6, v10, v14 = gVec512(v2, v6, v10, v14, &t[b][1], &t[b][9], r16, r12, r8, r7)
		v3, v7, v11, v15 = gVec512(v3, v7, v11, v15, &t[b][8], &t[b][6], r16, r12, r8, r7)
		v0, v5, v10, v15 = gVec512(v0, v5, v10, v15, &t[b][14], &t[b][10], r16, r12, r8, r7)
		v1, v6, v11, v12 = gVec512(v1, v6, v11, v12, &t[b][2], &t[b][12], r16, r12, r8, r7)
		v2, v7, v8, v13 = gVec512(v2, v7, v8, v13, &t[b][3], &t[b][4], r16, r12, r8, r7)
		v3, v4, v9, v14 = gVec512(v3, v4, v9, v14, &t[b][7], &t[b][13], r16, r12, r8, r7)

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
	// The vectorized transpose permuted the message lanes, so undo it first
	// (the transpose is its own inverse, matching the reference) to restore
	// natural chunk order. The partial (scalar-transpose) path is already in
	// natural order.
	if lanes == simdLanesAvx512 && blocks == 16 {
		var z archsimd.Uint32x16
		c0, c1, c2, c3, c4, c5, c6, c7, _, _, _, _, _, _, _, _ :=
			transposeVecs512(cv0, cv1, cv2, cv3, cv4, cv5, cv6, cv7, z, z, z, z, z, z, z, z)
		cv0, cv1, cv2, cv3, cv4, cv5, cv6, cv7 = c0, c1, c2, c3, c4, c5, c6, c7
	}

	var lane [simdLanesAvx512]uint32
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

// compressOutputsAvx512 computes simdLanes512 64-byte root output blocks
// starting at block start. The message block and input CV are constant across
// lanes; only the counter varies. The message words are read through gVec512's
// memory-operand loads from the broadcast buffer m (built once by
// compressOutputs). As with compressChunksLanes512, the state vectors are named
// locals and the rounds are fully unrolled.
func compressOutputsAvx512(out []byte, cv *[8]uint32, m *[16][simdLanesAvx512]uint32, blkLen, flags uint32, start uint64) {
	var ctrLo, ctrHi [simdLanesAvx512]uint32
	for j := 0; j < simdLanesAvx512; j++ {
		c := start + uint64(j)
		ctrLo[j] = uint32(c)
		ctrHi[j] = uint32(c >> 32)
	}
	vCtrLo := archsimd.LoadUint32x16(ctrLo[:])
	vCtrHi := archsimd.LoadUint32x16(ctrHi[:])
	vBlockLen := bcast512(blkLen)
	vFlags := bcast512(flags)

	cv0 := bcast512(cv[0])
	cv1 := bcast512(cv[1])
	cv2 := bcast512(cv[2])
	cv3 := bcast512(cv[3])
	cv4 := bcast512(cv[4])
	cv5 := bcast512(cv[5])
	cv6 := bcast512(cv[6])
	cv7 := bcast512(cv[7])
	iv0 := bcast512(iv[0])
	iv1 := bcast512(iv[1])
	iv2 := bcast512(iv[2])
	iv3 := bcast512(iv[3])

	r16 := archsimd.LoadUint32x16Array(&rotCounts[0])
	r12 := archsimd.LoadUint32x16Array(&rotCounts[1])
	r8 := archsimd.LoadUint32x16Array(&rotCounts[2])
	r7 := archsimd.LoadUint32x16Array(&rotCounts[3])

	v0, v1, v2, v3 := cv0, cv1, cv2, cv3
	v4, v5, v6, v7 := cv4, cv5, cv6, cv7
	v8, v9, v10, v11 := iv0, iv1, iv2, iv3
	v12, v13, v14, v15 := vCtrLo, vCtrHi, vBlockLen, vFlags

	// 7 unrolled rounds; the message schedule is the same as the chunk kernel.
	// round 0
	v0, v4, v8, v12 = gVec512(v0, v4, v8, v12, &m[0], &m[1], r16, r12, r8, r7)
	v1, v5, v9, v13 = gVec512(v1, v5, v9, v13, &m[2], &m[3], r16, r12, r8, r7)
	v2, v6, v10, v14 = gVec512(v2, v6, v10, v14, &m[4], &m[5], r16, r12, r8, r7)
	v3, v7, v11, v15 = gVec512(v3, v7, v11, v15, &m[6], &m[7], r16, r12, r8, r7)
	v0, v5, v10, v15 = gVec512(v0, v5, v10, v15, &m[8], &m[9], r16, r12, r8, r7)
	v1, v6, v11, v12 = gVec512(v1, v6, v11, v12, &m[10], &m[11], r16, r12, r8, r7)
	v2, v7, v8, v13 = gVec512(v2, v7, v8, v13, &m[12], &m[13], r16, r12, r8, r7)
	v3, v4, v9, v14 = gVec512(v3, v4, v9, v14, &m[14], &m[15], r16, r12, r8, r7)
	// round 1
	v0, v4, v8, v12 = gVec512(v0, v4, v8, v12, &m[2], &m[6], r16, r12, r8, r7)
	v1, v5, v9, v13 = gVec512(v1, v5, v9, v13, &m[3], &m[10], r16, r12, r8, r7)
	v2, v6, v10, v14 = gVec512(v2, v6, v10, v14, &m[7], &m[0], r16, r12, r8, r7)
	v3, v7, v11, v15 = gVec512(v3, v7, v11, v15, &m[4], &m[13], r16, r12, r8, r7)
	v0, v5, v10, v15 = gVec512(v0, v5, v10, v15, &m[1], &m[11], r16, r12, r8, r7)
	v1, v6, v11, v12 = gVec512(v1, v6, v11, v12, &m[12], &m[5], r16, r12, r8, r7)
	v2, v7, v8, v13 = gVec512(v2, v7, v8, v13, &m[9], &m[14], r16, r12, r8, r7)
	v3, v4, v9, v14 = gVec512(v3, v4, v9, v14, &m[15], &m[8], r16, r12, r8, r7)
	// round 2
	v0, v4, v8, v12 = gVec512(v0, v4, v8, v12, &m[3], &m[4], r16, r12, r8, r7)
	v1, v5, v9, v13 = gVec512(v1, v5, v9, v13, &m[10], &m[12], r16, r12, r8, r7)
	v2, v6, v10, v14 = gVec512(v2, v6, v10, v14, &m[13], &m[2], r16, r12, r8, r7)
	v3, v7, v11, v15 = gVec512(v3, v7, v11, v15, &m[7], &m[14], r16, r12, r8, r7)
	v0, v5, v10, v15 = gVec512(v0, v5, v10, v15, &m[6], &m[5], r16, r12, r8, r7)
	v1, v6, v11, v12 = gVec512(v1, v6, v11, v12, &m[9], &m[0], r16, r12, r8, r7)
	v2, v7, v8, v13 = gVec512(v2, v7, v8, v13, &m[11], &m[15], r16, r12, r8, r7)
	v3, v4, v9, v14 = gVec512(v3, v4, v9, v14, &m[8], &m[1], r16, r12, r8, r7)
	// round 3
	v0, v4, v8, v12 = gVec512(v0, v4, v8, v12, &m[10], &m[7], r16, r12, r8, r7)
	v1, v5, v9, v13 = gVec512(v1, v5, v9, v13, &m[12], &m[9], r16, r12, r8, r7)
	v2, v6, v10, v14 = gVec512(v2, v6, v10, v14, &m[14], &m[3], r16, r12, r8, r7)
	v3, v7, v11, v15 = gVec512(v3, v7, v11, v15, &m[13], &m[15], r16, r12, r8, r7)
	v0, v5, v10, v15 = gVec512(v0, v5, v10, v15, &m[4], &m[0], r16, r12, r8, r7)
	v1, v6, v11, v12 = gVec512(v1, v6, v11, v12, &m[11], &m[2], r16, r12, r8, r7)
	v2, v7, v8, v13 = gVec512(v2, v7, v8, v13, &m[5], &m[8], r16, r12, r8, r7)
	v3, v4, v9, v14 = gVec512(v3, v4, v9, v14, &m[1], &m[6], r16, r12, r8, r7)
	// round 4
	v0, v4, v8, v12 = gVec512(v0, v4, v8, v12, &m[12], &m[13], r16, r12, r8, r7)
	v1, v5, v9, v13 = gVec512(v1, v5, v9, v13, &m[9], &m[11], r16, r12, r8, r7)
	v2, v6, v10, v14 = gVec512(v2, v6, v10, v14, &m[15], &m[10], r16, r12, r8, r7)
	v3, v7, v11, v15 = gVec512(v3, v7, v11, v15, &m[14], &m[8], r16, r12, r8, r7)
	v0, v5, v10, v15 = gVec512(v0, v5, v10, v15, &m[7], &m[2], r16, r12, r8, r7)
	v1, v6, v11, v12 = gVec512(v1, v6, v11, v12, &m[5], &m[3], r16, r12, r8, r7)
	v2, v7, v8, v13 = gVec512(v2, v7, v8, v13, &m[0], &m[1], r16, r12, r8, r7)
	v3, v4, v9, v14 = gVec512(v3, v4, v9, v14, &m[6], &m[4], r16, r12, r8, r7)
	// round 5
	v0, v4, v8, v12 = gVec512(v0, v4, v8, v12, &m[9], &m[14], r16, r12, r8, r7)
	v1, v5, v9, v13 = gVec512(v1, v5, v9, v13, &m[11], &m[5], r16, r12, r8, r7)
	v2, v6, v10, v14 = gVec512(v2, v6, v10, v14, &m[8], &m[12], r16, r12, r8, r7)
	v3, v7, v11, v15 = gVec512(v3, v7, v11, v15, &m[15], &m[1], r16, r12, r8, r7)
	v0, v5, v10, v15 = gVec512(v0, v5, v10, v15, &m[13], &m[3], r16, r12, r8, r7)
	v1, v6, v11, v12 = gVec512(v1, v6, v11, v12, &m[0], &m[10], r16, r12, r8, r7)
	v2, v7, v8, v13 = gVec512(v2, v7, v8, v13, &m[2], &m[6], r16, r12, r8, r7)
	v3, v4, v9, v14 = gVec512(v3, v4, v9, v14, &m[4], &m[7], r16, r12, r8, r7)
	// round 6
	v0, v4, v8, v12 = gVec512(v0, v4, v8, v12, &m[11], &m[15], r16, r12, r8, r7)
	v1, v5, v9, v13 = gVec512(v1, v5, v9, v13, &m[5], &m[0], r16, r12, r8, r7)
	v2, v6, v10, v14 = gVec512(v2, v6, v10, v14, &m[1], &m[9], r16, r12, r8, r7)
	v3, v7, v11, v15 = gVec512(v3, v7, v11, v15, &m[8], &m[6], r16, r12, r8, r7)
	v0, v5, v10, v15 = gVec512(v0, v5, v10, v15, &m[14], &m[10], r16, r12, r8, r7)
	v1, v6, v11, v12 = gVec512(v1, v6, v11, v12, &m[2], &m[12], r16, r12, r8, r7)
	v2, v7, v8, v13 = gVec512(v2, v7, v8, v13, &m[3], &m[4], r16, r12, r8, r7)
	v3, v4, v9, v14 = gVec512(v3, v4, v9, v14, &m[7], &m[13], r16, r12, r8, r7)

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
	var lane [simdLanesAvx512]uint32
	v0.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen:], lane[j])
	}
	v1.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+4:], lane[j])
	}
	v2.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+8:], lane[j])
	}
	v3.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+12:], lane[j])
	}
	v4.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+16:], lane[j])
	}
	v5.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+20:], lane[j])
	}
	v6.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+24:], lane[j])
	}
	v7.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+28:], lane[j])
	}
	v8.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+32:], lane[j])
	}
	v9.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+36:], lane[j])
	}
	v10.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+40:], lane[j])
	}
	v11.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+44:], lane[j])
	}
	v12.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+48:], lane[j])
	}
	v13.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+52:], lane[j])
	}
	v14.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+56:], lane[j])
	}
	v15.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		binary.LittleEndian.PutUint32(out[j*blockLen+60:], lane[j])
	}
}

// compressParentsAvx512 computes the chaining value of 16 parent nodes in one
// AVX-512 batch. Parent j's block is [left[j], right[j]]; the message is
// already lane-major (no transpose needed), the counter is 0, and the flags
// carry flagParent. Only the first 8 output words (the CV) are needed.
func compressParentsAvx512(left, right *[simdLanesAvx512][8]uint32, key [8]uint32, flags uint32) [simdLanesAvx512][8]uint32 {
	// Lane-major message buffer: m[i] lane j = word i of parent j. Passed as
	// pointers to gVec512 so the messages are read from memory per round.
	var scratch [16][simdLanesAvx512]uint32
	for j := 0; j < simdLanesAvx512; j++ {
		for w := 0; w < 8; w++ {
			scratch[w][j] = left[j][w]
			scratch[w+8][j] = right[j][w]
		}
	}

	cv0 := bcast512(key[0])
	cv1 := bcast512(key[1])
	cv2 := bcast512(key[2])
	cv3 := bcast512(key[3])
	cv4 := bcast512(key[4])
	cv5 := bcast512(key[5])
	cv6 := bcast512(key[6])
	cv7 := bcast512(key[7])
	iv0 := bcast512(iv[0])
	iv1 := bcast512(iv[1])
	iv2 := bcast512(iv[2])
	iv3 := bcast512(iv[3])

	// Loop-invariant VPRORVD count vectors.
	r16 := archsimd.LoadUint32x16Array(&rotCounts[0])
	r12 := archsimd.LoadUint32x16Array(&rotCounts[1])
	r8 := archsimd.LoadUint32x16Array(&rotCounts[2])
	r7 := archsimd.LoadUint32x16Array(&rotCounts[3])

	v0, v1, v2, v3 := cv0, cv1, cv2, cv3
	v4, v5, v6, v7 := cv4, cv5, cv6, cv7
	v8, v9, v10, v11 := iv0, iv1, iv2, iv3
	v12 := bcast512(0)
	v13 := bcast512(0)
	v14 := bcast512(blockLen)
	v15 := bcast512(flags | flagParent)

	// round 0
	v0, v4, v8, v12 = gVec512(v0, v4, v8, v12, &scratch[0], &scratch[1], r16, r12, r8, r7)
	v1, v5, v9, v13 = gVec512(v1, v5, v9, v13, &scratch[2], &scratch[3], r16, r12, r8, r7)
	v2, v6, v10, v14 = gVec512(v2, v6, v10, v14, &scratch[4], &scratch[5], r16, r12, r8, r7)
	v3, v7, v11, v15 = gVec512(v3, v7, v11, v15, &scratch[6], &scratch[7], r16, r12, r8, r7)
	v0, v5, v10, v15 = gVec512(v0, v5, v10, v15, &scratch[8], &scratch[9], r16, r12, r8, r7)
	v1, v6, v11, v12 = gVec512(v1, v6, v11, v12, &scratch[10], &scratch[11], r16, r12, r8, r7)
	v2, v7, v8, v13 = gVec512(v2, v7, v8, v13, &scratch[12], &scratch[13], r16, r12, r8, r7)
	v3, v4, v9, v14 = gVec512(v3, v4, v9, v14, &scratch[14], &scratch[15], r16, r12, r8, r7)
	// round 1
	v0, v4, v8, v12 = gVec512(v0, v4, v8, v12, &scratch[2], &scratch[6], r16, r12, r8, r7)
	v1, v5, v9, v13 = gVec512(v1, v5, v9, v13, &scratch[3], &scratch[10], r16, r12, r8, r7)
	v2, v6, v10, v14 = gVec512(v2, v6, v10, v14, &scratch[7], &scratch[0], r16, r12, r8, r7)
	v3, v7, v11, v15 = gVec512(v3, v7, v11, v15, &scratch[4], &scratch[13], r16, r12, r8, r7)
	v0, v5, v10, v15 = gVec512(v0, v5, v10, v15, &scratch[1], &scratch[11], r16, r12, r8, r7)
	v1, v6, v11, v12 = gVec512(v1, v6, v11, v12, &scratch[12], &scratch[5], r16, r12, r8, r7)
	v2, v7, v8, v13 = gVec512(v2, v7, v8, v13, &scratch[9], &scratch[14], r16, r12, r8, r7)
	v3, v4, v9, v14 = gVec512(v3, v4, v9, v14, &scratch[15], &scratch[8], r16, r12, r8, r7)
	// round 2
	v0, v4, v8, v12 = gVec512(v0, v4, v8, v12, &scratch[3], &scratch[4], r16, r12, r8, r7)
	v1, v5, v9, v13 = gVec512(v1, v5, v9, v13, &scratch[10], &scratch[12], r16, r12, r8, r7)
	v2, v6, v10, v14 = gVec512(v2, v6, v10, v14, &scratch[13], &scratch[2], r16, r12, r8, r7)
	v3, v7, v11, v15 = gVec512(v3, v7, v11, v15, &scratch[7], &scratch[14], r16, r12, r8, r7)
	v0, v5, v10, v15 = gVec512(v0, v5, v10, v15, &scratch[6], &scratch[5], r16, r12, r8, r7)
	v1, v6, v11, v12 = gVec512(v1, v6, v11, v12, &scratch[9], &scratch[0], r16, r12, r8, r7)
	v2, v7, v8, v13 = gVec512(v2, v7, v8, v13, &scratch[11], &scratch[15], r16, r12, r8, r7)
	v3, v4, v9, v14 = gVec512(v3, v4, v9, v14, &scratch[8], &scratch[1], r16, r12, r8, r7)
	// round 3
	v0, v4, v8, v12 = gVec512(v0, v4, v8, v12, &scratch[10], &scratch[7], r16, r12, r8, r7)
	v1, v5, v9, v13 = gVec512(v1, v5, v9, v13, &scratch[12], &scratch[9], r16, r12, r8, r7)
	v2, v6, v10, v14 = gVec512(v2, v6, v10, v14, &scratch[14], &scratch[3], r16, r12, r8, r7)
	v3, v7, v11, v15 = gVec512(v3, v7, v11, v15, &scratch[13], &scratch[15], r16, r12, r8, r7)
	v0, v5, v10, v15 = gVec512(v0, v5, v10, v15, &scratch[4], &scratch[0], r16, r12, r8, r7)
	v1, v6, v11, v12 = gVec512(v1, v6, v11, v12, &scratch[11], &scratch[2], r16, r12, r8, r7)
	v2, v7, v8, v13 = gVec512(v2, v7, v8, v13, &scratch[5], &scratch[8], r16, r12, r8, r7)
	v3, v4, v9, v14 = gVec512(v3, v4, v9, v14, &scratch[1], &scratch[6], r16, r12, r8, r7)
	// round 4
	v0, v4, v8, v12 = gVec512(v0, v4, v8, v12, &scratch[12], &scratch[13], r16, r12, r8, r7)
	v1, v5, v9, v13 = gVec512(v1, v5, v9, v13, &scratch[9], &scratch[11], r16, r12, r8, r7)
	v2, v6, v10, v14 = gVec512(v2, v6, v10, v14, &scratch[15], &scratch[10], r16, r12, r8, r7)
	v3, v7, v11, v15 = gVec512(v3, v7, v11, v15, &scratch[14], &scratch[8], r16, r12, r8, r7)
	v0, v5, v10, v15 = gVec512(v0, v5, v10, v15, &scratch[7], &scratch[2], r16, r12, r8, r7)
	v1, v6, v11, v12 = gVec512(v1, v6, v11, v12, &scratch[5], &scratch[3], r16, r12, r8, r7)
	v2, v7, v8, v13 = gVec512(v2, v7, v8, v13, &scratch[0], &scratch[1], r16, r12, r8, r7)
	v3, v4, v9, v14 = gVec512(v3, v4, v9, v14, &scratch[6], &scratch[4], r16, r12, r8, r7)
	// round 5
	v0, v4, v8, v12 = gVec512(v0, v4, v8, v12, &scratch[9], &scratch[14], r16, r12, r8, r7)
	v1, v5, v9, v13 = gVec512(v1, v5, v9, v13, &scratch[11], &scratch[5], r16, r12, r8, r7)
	v2, v6, v10, v14 = gVec512(v2, v6, v10, v14, &scratch[8], &scratch[12], r16, r12, r8, r7)
	v3, v7, v11, v15 = gVec512(v3, v7, v11, v15, &scratch[15], &scratch[1], r16, r12, r8, r7)
	v0, v5, v10, v15 = gVec512(v0, v5, v10, v15, &scratch[13], &scratch[3], r16, r12, r8, r7)
	v1, v6, v11, v12 = gVec512(v1, v6, v11, v12, &scratch[0], &scratch[10], r16, r12, r8, r7)
	v2, v7, v8, v13 = gVec512(v2, v7, v8, v13, &scratch[2], &scratch[6], r16, r12, r8, r7)
	v3, v4, v9, v14 = gVec512(v3, v4, v9, v14, &scratch[4], &scratch[7], r16, r12, r8, r7)
	// round 6
	v0, v4, v8, v12 = gVec512(v0, v4, v8, v12, &scratch[11], &scratch[15], r16, r12, r8, r7)
	v1, v5, v9, v13 = gVec512(v1, v5, v9, v13, &scratch[5], &scratch[0], r16, r12, r8, r7)
	v2, v6, v10, v14 = gVec512(v2, v6, v10, v14, &scratch[1], &scratch[9], r16, r12, r8, r7)
	v3, v7, v11, v15 = gVec512(v3, v7, v11, v15, &scratch[8], &scratch[6], r16, r12, r8, r7)
	v0, v5, v10, v15 = gVec512(v0, v5, v10, v15, &scratch[14], &scratch[10], r16, r12, r8, r7)
	v1, v6, v11, v12 = gVec512(v1, v6, v11, v12, &scratch[2], &scratch[12], r16, r12, r8, r7)
	v2, v7, v8, v13 = gVec512(v2, v7, v8, v13, &scratch[3], &scratch[4], r16, r12, r8, r7)
	v3, v4, v9, v14 = gVec512(v3, v4, v9, v14, &scratch[7], &scratch[13], r16, r12, r8, r7)

	// CV update: word i = v[i]^v[i+8], then scatter across the 16 parents.
	v0 = v0.Xor(v8)
	v1 = v1.Xor(v9)
	v2 = v2.Xor(v10)
	v3 = v3.Xor(v11)
	v4 = v4.Xor(v12)
	v5 = v5.Xor(v13)
	v6 = v6.Xor(v14)
	v7 = v7.Xor(v15)

	var lane [simdLanesAvx512]uint32
	var cvs [simdLanesAvx512][8]uint32
	v0.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		cvs[j][0] = lane[j]
	}
	v1.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		cvs[j][1] = lane[j]
	}
	v2.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		cvs[j][2] = lane[j]
	}
	v3.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		cvs[j][3] = lane[j]
	}
	v4.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		cvs[j][4] = lane[j]
	}
	v5.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		cvs[j][5] = lane[j]
	}
	v6.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		cvs[j][6] = lane[j]
	}
	v7.Store(lane[:])
	for j := 0; j < simdLanesAvx512; j++ {
		cvs[j][7] = lane[j]
	}
	return cvs
}
