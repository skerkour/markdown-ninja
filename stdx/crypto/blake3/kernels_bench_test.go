//go:build arm64 && goexperiment.simd

package blake3

import "testing"

func BenchmarkKernelChunksLanes(b *testing.B) {
	data := make([]byte, simdLanes*chunkLen)
	var key [8]uint32 = iv
	cvs := make([][8]uint32, simdLanes)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for b.Loop() {
		compressChunksLanes(data, 0, 0, cvs, key, 0)
	}
}

func BenchmarkKernelOutputsLanes(b *testing.B) {
	var cv [8]uint32 = iv
	var blk [16]uint32
	out := make([]byte, simdLanes*blockLen)
	b.SetBytes(int64(len(out)))
	b.ResetTimer()
	for b.Loop() {
		compressOutputsLanes(out, &cv, &blk, blockLen, flagRoot, 0)
	}
}
