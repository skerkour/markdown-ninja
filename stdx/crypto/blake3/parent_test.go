//go:build (arm64 || amd64) && goexperiment.simd

package blake3

import (
	"math/rand"
	"testing"
)

// TestParentKernelCrossCheck verifies the SIMD parent kernel (used by the
// tree-merge batching) produces results identical to the scalar parentCV, for
// the 4-lane NEON and 8-lane AVX2 backends.
func TestParentKernelCrossCheck(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	var left, right [simdLanes][8]uint32
	for i := range left {
		for w := range left[i] {
			left[i][w] = rng.Uint32()
			right[i][w] = rng.Uint32()
		}
	}
	var key [8]uint32 = iv

	got := compressParentsLanes(&left, &right, key, 0)
	for j := 0; j < simdLanes; j++ {
		want := parentCV(left[j], right[j], key, 0)
		if got[j] != want {
			t.Fatalf("parent %d: got %x want %x", j, got[j], want)
		}
	}

	// Exercise mergeParentCVs with a mixed-length run (SIMD batches + scalar
	// remainder) against the scalar loop.
	n := 13
	src := make([][8]uint32, 2*n)
	for i := range src {
		for w := range src[i] {
			src[i][w] = rng.Uint32()
		}
	}
	out := make([][8]uint32, n)
	mergeParentCVs(out, src, key, flagKeyedHash)
	for i := 0; i < n; i++ {
		want := parentCV(src[2*i], src[2*i+1], key, flagKeyedHash)
		if out[i] != want {
			t.Fatalf("merge parent %d: got %x want %x", i, out[i], want)
		}
	}
}

// TestMergeChunkCVsCrossCheck verifies the bottom-up subtree merge against
// building the same tree with the scalar parentCV.
func TestMergeChunkCVsCrossCheck(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	var key [8]uint32 = iv
	for _, n := range []int{2, 4, 8, 16, 32} {
		cvs := make([][8]uint32, n)
		for i := range cvs {
			for w := range cvs[i] {
				cvs[i][w] = rng.Uint32()
			}
		}
		got := mergeChunkCVs(append([][8]uint32{}, cvs...), key, 0)
		want := scalarMergeChunkCVs(cvs, key, 0)
		if got != want {
			t.Fatalf("n=%d: got %x want %x", n, got, want)
		}
	}
}

func scalarMergeChunkCVs(cvs [][8]uint32, key [8]uint32, flags uint32) [8]uint32 {
	for len(cvs) > 1 {
		half := len(cvs) / 2
		for i := 0; i < half; i++ {
			cvs[i] = parentCV(cvs[2*i], cvs[2*i+1], key, flags)
		}
		cvs = cvs[:half]
	}
	return cvs[0]
}
