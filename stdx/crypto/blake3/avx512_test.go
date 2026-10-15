//go:build amd64 && goexperiment.simd

package blake3

import (
	"bytes"
	"encoding/binary"
	"math/bits"
	"math/rand"
	"testing"

	"simd/archsimd"
)

// scalarRound512 is the scalar model of gVec512: the BLAKE3 quarter round on a
// 16-lane word-major layout, transcribed 1:1 from the kernel (VPRORVD == right
// rotate by the count). Used to validate the AVX-512 kernel's logic even where
// AVX-512 cannot execute. v and m are the state and message arrays, indexed by
// lane; mx/my are message-word indices.
func scalarRound512(v, m *[16][16]uint32, a, b, c, d, mx, my int) {
	vd := [16]uint32{}
	va := [16]uint32{}
	vc := [16]uint32{}
	vb := [16]uint32{}
	copy(vd[:], v[d][:])
	copy(va[:], v[a][:])
	copy(vc[:], v[c][:])
	copy(vb[:], v[b][:])

	mmx := m[mx]
	mmy := m[my]

	for j := 0; j < 16; j++ {
		va[j] = va[j] + vb[j] + mmx[j]
		vd[j] = bits.RotateLeft32(vd[j]^va[j], -16)
		vc[j] = vc[j] + vd[j]
		vb[j] = bits.RotateLeft32(vb[j]^vc[j], -12)
		va[j] = va[j] + vb[j] + mmy[j]
		vd[j] = bits.RotateLeft32(vd[j]^va[j], -8)
		vc[j] = vc[j] + vd[j]
		vb[j] = bits.RotateLeft32(vb[j]^vc[j], -7)
	}

	copy(v[a][:], va[:])
	copy(v[b][:], vb[:])
	copy(v[c][:], vc[:])
	copy(v[d][:], vd[:])
}

// scalarCompressChunks512 is the scalar model of compressChunksAvx512,
// transcribed 1:1 (same gather, schedule, rounds, CV update, scatter). It
// validates the AVX-512 kernel's lane/counter/schedule logic on any machine.
func scalarCompressChunks512(data []byte, base int, counterBase uint64, key [8]uint32, flags uint32) [16][8]uint32 {
	// cvs[i] = word i of the chaining value, lane j = chunk (base+j).
	var cvs [8][16]uint32
	for i := 0; i < 8; i++ {
		for j := 0; j < 16; j++ {
			cvs[i][j] = key[i]
		}
	}
	var ctrLo, ctrHi [16]uint32
	for j := 0; j < 16; j++ {
		c := counterBase + uint64(base+j)
		ctrLo[j] = uint32(c)
		ctrHi[j] = uint32(c >> 32)
	}

	// 7-round schedule (same as the kernel).
	var sched [7][16]int
	for i := 0; i < 16; i++ {
		sched[0][i] = i
	}
	perm := [16]int{2, 6, 3, 10, 7, 0, 4, 13, 1, 11, 12, 5, 9, 14, 15, 8}
	for r := 1; r < 7; r++ {
		for i := 0; i < 16; i++ {
			sched[r][i] = sched[r-1][perm[i]]
		}
	}
	cols := [8][4]int{{0, 4, 8, 12}, {1, 5, 9, 13}, {2, 6, 10, 14}, {3, 7, 11, 15}, {0, 5, 10, 15}, {1, 6, 11, 12}, {2, 7, 8, 13}, {3, 4, 9, 14}}

	// A full chunk is 16 blocks; block 0 has CHUNK_START, block 15 CHUNK_END.
	for b := 0; b < 16; b++ {
		var m [16][16]uint32
		for j := 0; j < 16; j++ {
			blk := data[(base+j)*chunkLen+b*blockLen:]
			for i := 0; i < 16; i++ {
				m[i][j] = binary.LittleEndian.Uint32(blk[i*4:])
			}
		}

		fl := flags
		if b == 0 {
			fl |= flagChunkStart
		}
		if b == 15 {
			fl |= flagChunkEnd
		}

		var v [16][16]uint32
		for i := 0; i < 8; i++ {
			v[i] = cvs[i]
		}
		for i := 0; i < 4; i++ {
			v[8+i] = [16]uint32{iv[i], iv[i], iv[i], iv[i], iv[i], iv[i], iv[i], iv[i], iv[i], iv[i], iv[i], iv[i], iv[i], iv[i], iv[i], iv[i]}
		}
		v[12] = ctrLo
		v[13] = ctrHi
		v[14] = [16]uint32{blockLen, blockLen, blockLen, blockLen, blockLen, blockLen, blockLen, blockLen, blockLen, blockLen, blockLen, blockLen, blockLen, blockLen, blockLen, blockLen}
		v[15] = [16]uint32{fl, fl, fl, fl, fl, fl, fl, fl, fl, fl, fl, fl, fl, fl, fl, fl}

		for r := 0; r < 7; r++ {
			for gi, c := range cols {
				sc := sched[r]
				scalarRound512(&v, &m, c[0], c[1], c[2], c[3], sc[gi*2], sc[gi*2+1])
			}
		}
		for i := 0; i < 8; i++ {
			cvs[i] = [16]uint32{
				v[i][0] ^ v[i+8][0], v[i][1] ^ v[i+8][1], v[i][2] ^ v[i+8][2], v[i][3] ^ v[i+8][3],
				v[i][4] ^ v[i+8][4], v[i][5] ^ v[i+8][5], v[i][6] ^ v[i+8][6], v[i][7] ^ v[i+8][7],
				v[i][8] ^ v[i+8][8], v[i][9] ^ v[i+8][9], v[i][10] ^ v[i+8][10], v[i][11] ^ v[i+8][11],
				v[i][12] ^ v[i+8][12], v[i][13] ^ v[i+8][13], v[i][14] ^ v[i+8][14], v[i][15] ^ v[i+8][15],
			}
		}
	}

	// Scatter: cvs[i][j] = word i of chunk (base+j).
	var out [16][8]uint32
	for i := 0; i < 8; i++ {
		for j := 0; j < 16; j++ {
			out[j][i] = cvs[i][j]
		}
	}
	return out
}

// TestAVX512LogicModel validates the AVX-512 kernel's exact algorithm (gather,
// schedule, 16-lane rounds, CV update, scatter) against the scalar chunk kernel
// via a 1:1 scalar transcription. This runs on any CPU, unlike the SIMD path.
func TestAVX512LogicModel(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	data := make([]byte, simdLanesAvx512*chunkLen)
	for i := range data {
		data[i] = byte(rng.Intn(256))
	}
	var key [8]uint32
	for i := range key {
		key[i] = rng.Uint32()
	}

	for _, counterBase := range []uint64{0, 3} {
		model := scalarCompressChunks512(data, 0, counterBase, key, 0)
		for j := 0; j < 16; j++ {
			want := compressChunkCV(data, j, counterBase, key, 0)
			if model[j] != want {
				t.Fatalf("base=%d chunk %d: model %x scalar %x", counterBase, j, model[j], want)
			}
		}
	}
}

// TestAVX512PathCrossCheck verifies that the AVX-512 lane kernels produce
// results identical to the scalar chunk kernel, and reports whether the
// runtime AVX-512 gate is active (so we know the SIMD path is really
// exercised). It also cross-checks against the AVX2 kernels.
func TestAVX512PathCrossCheck(t *testing.T) {
	if !archsimd.X86.AVX512() {
		t.Skip("host/emulated CPU lacks AVX-512; AVX-512 kernel not exercised")
	}
	rng := rand.New(rand.NewSource(7))
	data := make([]byte, simdLanesAvx512*chunkLen)
	for i := range data {
		data[i] = byte(rng.Intn(256))
	}
	var key [8]uint32
	for i := range key {
		key[i] = rng.Uint32()
	}

	// Chunk kernel vs scalar.
	var simdCV, scalarCV [simdLanesAvx512][8]uint32
	compressChunksAvx512(data, 0, 0, simdCV[:], key, 0, simdLanesAvx512, 16, true)
	for i := 0; i < simdLanesAvx512; i++ {
		scalarCV[i] = compressChunkCV(data, i, 0, key, 0)
	}
	for i := 0; i < simdLanesAvx512; i++ {
		if simdCV[i] != scalarCV[i] {
			t.Fatalf("chunk %d: SIMD %x scalar %x", i, simdCV[i], scalarCV[i])
		}
	}

	// Chunk kernel vs scalar, non-zero base counter.
	for i := 0; i < simdLanesAvx512; i++ {
		scalarCV[i] = compressChunkCV(data, i, 3, key, flagKeyedHash)
	}
	compressChunksAvx512(data, 0, 3, simdCV[:], key, flagKeyedHash, simdLanesAvx512, 16, true)
	for i := 0; i < simdLanesAvx512; i++ {
		if simdCV[i] != scalarCV[i] {
			t.Fatalf("keyed chunk %d: SIMD %x scalar %x", i, simdCV[i], scalarCV[i])
		}
	}

	// XOF kernel vs scalar.
	var cv [8]uint32 = key
	var blk [16]uint32
	for i := range blk {
		blk[i] = rng.Uint32()
	}
	var m [16][simdLanesAvx512]uint32
	for i := range blk {
		for j := 0; j < simdLanesAvx512; j++ {
			m[i][j] = blk[i]
		}
	}
	var simdOut, scalarOut [simdLanesAvx512 * blockLen]byte
	compressOutputsAvx512(simdOut[:], &cv, &m, blockLen, flagRoot, 7)
	compressOutputsScalar(scalarOut[:], &cv, &blk, blockLen, flagRoot, 7)
	if !bytes.Equal(simdOut[:], scalarOut[:]) {
		t.Fatal("compressOutputsAvx512 != scalar")
	}

	// XOF kernel vs AVX2 kernel (both must be bit-identical).
	if archsimd.X86.AVX2() {
		var avx2Out [simdLanesAvx512 * blockLen]byte
		compressOutputs(avx2Out[:], &cv, &blk, blockLen, flagRoot, 7)
		if !bytes.Equal(simdOut[:], avx2Out[:]) {
			t.Fatal("AVX-512 XOF != AVX2 XOF")
		}
	}
}

// TestTransposeVecs512 verifies that the vectorized 16x16 message transpose is
// an involution: applying it twice must reproduce the input exactly. The
// transpose carries a fixed lane permutation (like the reference), which is
// undone on the output side, so the involution property is what makes the
// per-chunk lanes come out in natural order. Combined with the end-to-end
// cross-check in TestAVX512PathCrossCheck this pins the transpose's
// correctness.
func TestTransposeVecs512(t *testing.T) {
	if !archsimd.X86.AVX512() {
		t.Skip("host/emulated CPU lacks AVX-512")
	}
	rng := rand.New(rand.NewSource(7))
	var in [16][16]uint32
	for i := range in {
		for j := range in[i] {
			in[i][j] = rng.Uint32()
		}
	}
	var v [16]archsimd.Uint32x16
	for i := 0; i < 16; i++ {
		v[i] = archsimd.LoadUint32x16(in[i][:])
	}
	a, b, c, d, e, f, g, h, i2, j2, k2, l2, m2, n2, o2, p2 :=
		transposeVecs512(v[0], v[1], v[2], v[3], v[4], v[5], v[6], v[7], v[8], v[9], v[10], v[11], v[12], v[13], v[14], v[15])
	r0, r1, r2, r3, r4, r5, r6, r7, r8, r9, r10, r11, r12, r13, r14, r15 :=
		transposeVecs512(a, b, c, d, e, f, g, h, i2, j2, k2, l2, m2, n2, o2, p2)
	outs := []*archsimd.Uint32x16{&r0, &r1, &r2, &r3, &r4, &r5, &r6, &r7, &r8, &r9, &r10, &r11, &r12, &r13, &r14, &r15}
	var back [16][16]uint32
	for i := 0; i < 16; i++ {
		outs[i].Store(back[i][:])
	}
	for i := 0; i < 16; i++ {
		if back[i] != in[i] {
			t.Fatalf("transpose not an involution at row %d", i)
		}
	}
}
