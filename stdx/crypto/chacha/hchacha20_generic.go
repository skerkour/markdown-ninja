package chacha

import (
	"encoding/binary"
	"math/bits"
)

// calls to binary.LittleEndian.Uint32 and quarterRound are inlined
func hChaCha20Generic(out *[32]byte, nonce *[16]byte, key *[32]byte) {
	x0, x1, x2, x3 := chachaConstants[0], chachaConstants[1], chachaConstants[2], chachaConstants[3]
	x4 := binary.LittleEndian.Uint32(key[0:4])
	x5 := binary.LittleEndian.Uint32(key[4:8])
	x6 := binary.LittleEndian.Uint32(key[8:12])
	x7 := binary.LittleEndian.Uint32(key[12:16])
	x8 := binary.LittleEndian.Uint32(key[16:20])
	x9 := binary.LittleEndian.Uint32(key[20:24])
	x10 := binary.LittleEndian.Uint32(key[24:28])
	x11 := binary.LittleEndian.Uint32(key[28:32])
	x12 := binary.LittleEndian.Uint32(nonce[0:4])
	x13 := binary.LittleEndian.Uint32(nonce[4:8])
	x14 := binary.LittleEndian.Uint32(nonce[8:12])
	x15 := binary.LittleEndian.Uint32(nonce[12:16])

	for i := 0; i < 10; i++ {
		// Diagonal round.
		x0, x4, x8, x12 = quarterRound(x0, x4, x8, x12)
		x1, x5, x9, x13 = quarterRound(x1, x5, x9, x13)
		x2, x6, x10, x14 = quarterRound(x2, x6, x10, x14)
		x3, x7, x11, x15 = quarterRound(x3, x7, x11, x15)

		// Column round.
		x0, x5, x10, x15 = quarterRound(x0, x5, x10, x15)
		x1, x6, x11, x12 = quarterRound(x1, x6, x11, x12)
		x2, x7, x8, x13 = quarterRound(x2, x7, x8, x13)
		x3, x4, x9, x14 = quarterRound(x3, x4, x9, x14)
	}

	// _ = out[31] // bounds check elimination hint
	binary.LittleEndian.PutUint32(out[0:4], x0)
	binary.LittleEndian.PutUint32(out[4:8], x1)
	binary.LittleEndian.PutUint32(out[8:12], x2)
	binary.LittleEndian.PutUint32(out[12:16], x3)
	binary.LittleEndian.PutUint32(out[16:20], x12)
	binary.LittleEndian.PutUint32(out[20:24], x13)
	binary.LittleEndian.PutUint32(out[24:28], x14)
	binary.LittleEndian.PutUint32(out[28:32], x15)
}

func quarterRound(a, b, c, d uint32) (uint32, uint32, uint32, uint32) {
	a += b
	d ^= a
	d = bits.RotateLeft32(d, 16)
	c += d
	b ^= c
	b = bits.RotateLeft32(b, 12)
	a += b
	d ^= a
	d = bits.RotateLeft32(d, 8)
	c += d
	b ^= c
	b = bits.RotateLeft32(b, 7)
	return a, b, c, d
}
