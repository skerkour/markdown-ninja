//go:build !goexperiment.simd

package blake3

// simdLanes is the number of chunks (or output blocks) processed per SIMD
// batch. The generic build has no SIMD backend, so it is 1 and both dispatch
// functions fall straight through to the scalar kernels.
const simdLanes = 1

// fillChunkCVs is the portable scalar backend hook. It computes each full
// chunk's chaining value with the scalar kernel.
func fillChunkCVs(data []byte, cvs [][8]uint32, base uint64, key [8]uint32, flags uint32) {
	fillChunkCVsScalar(data, cvs, base, key, flags)
}

// fillChunkCV15 is the portable scalar backend hook for computing the chaining
// value of a chunk after its first 15 blocks (the inputCV of its final block).
// Feeding all 16 blocks compresses the first 15 and leaves the last in the
// buffer, exactly like a full chunkState.update.
func fillChunkCV15(data []byte, cvs [][8]uint32, base uint64, key [8]uint32, flags uint32) {
	cs := newChunkState(key, base, flags)
	cs.update(data[:16*blockLen])
	cvs[0] = cs.cv
}

// compressOutputs is the portable scalar backend hook. It fills out with the
// extendable root output, one ROOT compression per 64-byte block.
func compressOutputs(out []byte, cv *[8]uint32, block *[16]uint32, blkLen, flags uint32, start uint64) {
	compressOutputsScalar(out, cv, block, blkLen, flags, start)
}

// mergeParentCVs computes the parent chaining value of each pair (src[2i],
// src[2i+1]) into out[i], with the scalar kernel.
func mergeParentCVs(out, src [][8]uint32, key [8]uint32, flags uint32) {
	for i := 0; i < len(out); i++ {
		out[i] = parentCV(src[2*i], src[2*i+1], key, flags)
	}
}
