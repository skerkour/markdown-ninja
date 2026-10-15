//go:build amd64 && goexperiment.simd

package blake3

import (
	"math/bits"
	"math/rand"
	"testing"
)

// TestParentKernelAvx512Model validates the AVX-512 parent kernel via its 1:1
// scalar transcription (gVec512 uses VPRORVD == right rotate), so the kernel's
// logic is verified even where AVX-512 cannot execute.
func TestParentKernelAvx512Model(t *testing.T) {
	rng := rand.New(rand.NewSource(8))
	var left, right [simdLanesAvx512][8]uint32
	for i := range left {
		for w := range left[i] {
			left[i][w] = rng.Uint32()
			right[i][w] = rng.Uint32()
		}
	}
	var key [8]uint32 = iv

	got := scalarCompressParentsAvx512(&left, &right, key, 0)
	for j := 0; j < simdLanesAvx512; j++ {
		want := parentCV(left[j], right[j], key, 0)
		if got[j] != want {
			t.Fatalf("parent %d: model %x scalar %x", j, got[j], want)
		}
	}
}

// scalarCompressParentsAvx512 is a 1:1 scalar transcription of
// compressParentsAvx512 (16-lane word-major rounds, right-rotate == VPRORVD).
func scalarCompressParentsAvx512(left, right *[simdLanesAvx512][8]uint32, key [8]uint32, flags uint32) [simdLanesAvx512][8]uint32 {
	var m [16][simdLanesAvx512]uint32
	for j := 0; j < simdLanesAvx512; j++ {
		for w := 0; w < 8; w++ {
			m[w][j] = left[j][w]
			m[w+8][j] = right[j][w]
		}
	}
	var v [16][simdLanesAvx512]uint32
	for i := 0; i < 8; i++ {
		for j := 0; j < simdLanesAvx512; j++ {
			v[i][j] = key[i]
		}
	}
	for i := 0; i < 4; i++ {
		for j := 0; j < simdLanesAvx512; j++ {
			v[8+i][j] = iv[i]
		}
	}
	for j := 0; j < simdLanesAvx512; j++ {
		v[12][j] = 0
		v[13][j] = 0
		v[14][j] = blockLen
		v[15][j] = flags | flagParent
	}

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

	for r := 0; r < 7; r++ {
		for gi, c := range cols {
			sc := sched[r]
			a, b, cc, d := c[0], c[1], c[2], c[3]
			mx, my := sc[gi*2], sc[gi*2+1]
			for j := 0; j < simdLanesAvx512; j++ {
				va := v[a][j]
				vb := v[b][j]
				vc := v[cc][j]
				vd := v[d][j]
				va = va + vb + m[mx][j]
				vd = bits.RotateLeft32(vd^va, -16)
				vc = vc + vd
				vb = bits.RotateLeft32(vb^vc, -12)
				va = va + vb + m[my][j]
				vd = bits.RotateLeft32(vd^va, -8)
				vc = vc + vd
				vb = bits.RotateLeft32(vb^vc, -7)
				v[a][j], v[b][j], v[cc][j], v[d][j] = va, vb, vc, vd
			}
		}
	}

	var out [simdLanesAvx512][8]uint32
	for i := 0; i < 8; i++ {
		for j := 0; j < simdLanesAvx512; j++ {
			out[j][i] = v[i][j] ^ v[i+8][j]
		}
	}
	return out
}
