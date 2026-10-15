//go:build amd64 && goexperiment.simd

package blake3

import (
	"bytes"
	"math/rand"
	"testing"

	"simd/archsimd"
)

// TestAVX2PathCrossCheck verifies that the AVX2 lane kernels produce results
// identical to the scalar chunk kernel, and reports whether the runtime AVX2
// gate is active (so we know the SIMD path is really exercised).
func TestAVX2PathCrossCheck(t *testing.T) {
	if !archsimd.X86.AVX2() {
		t.Skip("host/emulated CPU lacks AVX2; SIMD kernel not exercised")
	}
	rng := rand.New(rand.NewSource(42))
	data := make([]byte, simdLanes*chunkLen)
	for i := range data {
		data[i] = byte(rng.Intn(256))
	}
	var key [8]uint32
	for i := range key {
		key[i] = rng.Uint32()
	}

	// Chunk kernel vs scalar.
	var simdCV, scalarCV [simdLanes][8]uint32
	compressChunksAvx2(data, 0, 0, simdCV[:], key, 0, simdLanes, 16, true)
	for i := 0; i < simdLanes; i++ {
		scalarCV[i] = compressChunkCV(data, i, 0, key, 0)
	}
	for i := 0; i < simdLanes; i++ {
		if simdCV[i] != scalarCV[i] {
			t.Fatalf("chunk %d: SIMD %x scalar %x", i, simdCV[i], scalarCV[i])
		}
	}

	// XOF kernel vs scalar.
	var cv [8]uint32 = key
	var blk [16]uint32
	for i := range blk {
		blk[i] = rng.Uint32()
	}
	var m [16][simdLanes]uint32
	for i := range blk {
		for j := 0; j < simdLanes; j++ {
			m[i][j] = blk[i]
		}
	}
	var simdOut, scalarOut [simdLanes * blockLen]byte
	compressOutputsAvx2(simdOut[:], &cv, &m, blockLen, flagRoot, 7)
	compressOutputsScalar(scalarOut[:], &cv, &blk, blockLen, flagRoot, 7)
	if !bytes.Equal(simdOut[:], scalarOut[:]) {
		t.Fatal("compressOutputsLanes != scalar")
	}

	// 4-wide XMM kernel vs scalar, for a partial batch of 4 chunks.
	var simdCV4, scalarCV4 [4][8]uint32
	compressChunksAvx4(data, 0, 0, simdCV4[:], key, 0, 4)
	for i := 0; i < 4; i++ {
		scalarCV4[i] = compressChunkCV(data, i, 0, key, 0)
	}
	for i := 0; i < 4; i++ {
		if simdCV4[i] != scalarCV4[i] {
			t.Fatalf("4-wide chunk %d: SIMD %x scalar %x", i, simdCV4[i], scalarCV4[i])
		}
	}
}
