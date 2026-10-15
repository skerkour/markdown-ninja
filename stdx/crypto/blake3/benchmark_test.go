package blake3_test

import (
	"strconv"
	"testing"

	zeeboblake3 "github.com/zeebo/blake3"
	lukeblake3 "lukechampine.com/blake3"

	"markdown.ninja/stdx/crypto/blake3"
)

// BenchmarkSum256 compares this implementation against the two hand-written
// assembly reference implementations (zeebo/blake3 and lukechampine/blake3)
// across input sizes, so the intrinsics-based kernel's throughput can be
// judged against the machine ceiling. Both references use AVX-512 where
// available and AVX2 otherwise.
func BenchmarkSum256(b *testing.B) {
	data := make([]byte, 1<<20)
	impls := []struct {
		name string
		fn   func([]byte) [32]byte
	}{
		{"stdx-go", blake3.Sum256},
		{"zeebo", zeeboblake3.Sum256},
		{"lukechampine", lukeblake3.Sum256},
	}
	for _, size := range []uint64{64, 4096, 64_000, 128_000} {
		b.Run(strconv.FormatUint(size, 10), func(b *testing.B) {
			in := data[:size]
			for _, impl := range impls {
				b.Run(impl.name, func(b *testing.B) {
					b.SetBytes(int64(size))
					b.ResetTimer()
					for b.Loop() {
						impl.fn(in)
					}
				})
			}
		})
	}
}

func BenchmarkHasherStream(b *testing.B) {
	data := make([]byte, 1<<20)
	var out [32]byte
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for b.Loop() {
		h := blake3.New()
		h.Write(data)
		h.Sum(out[:0])
	}
}

func BenchmarkKeyedHash(b *testing.B) {
	var key [32]byte
	data := make([]byte, 1<<20)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for b.Loop() {
		blake3.KeyedHash(key, data)
	}
}

func BenchmarkDeriveKey(b *testing.B) {
	data := make([]byte, 1<<20)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for b.Loop() {
		blake3.DeriveKey("stdx-go blake3 benchmark", data)
	}
}

func BenchmarkXof(b *testing.B) {
	h := blake3.New()
	h.Write([]byte("xof benchmark"))
	out := make([]byte, 1<<20)
	b.SetBytes(int64(len(out)))
	b.ResetTimer()
	for b.Loop() {
		r := h.Xof()
		for p := out; len(p) > 0; {
			n := len(p)
			if n > 64 {
				n = 64
			}
			r.Read(p[:n])
			p = p[n:]
		}
	}
}
