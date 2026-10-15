package chacha20

import (
	"simd/archsimd"
	"unsafe"
)

// rotl8Idx/rotl16Idx are the VTBL index vectors that rotate each 32-bit
// element of a Uint32x4 by 8/16 bits. On little-endian arm64 a single
// VUint8x16.LookupOrZero (VTBL) performs the rotation.
var (
	rotl8Idx  = archsimd.LoadUint8x16Array(&[16]uint8{3, 0, 1, 2, 7, 4, 5, 6, 11, 8, 9, 10, 15, 12, 13, 14})
	rotl16Idx = archsimd.LoadUint8x16Array(&[16]uint8{2, 3, 0, 1, 6, 7, 4, 5, 10, 11, 8, 9, 14, 15, 12, 13})
	shift12   = archsimd.LoadInt32x4Array(&[4]int32{12, 12, 12, 12})
	shift20   = archsimd.LoadInt32x4Array(&[4]int32{-20, -20, -20, -20})
	shift7    = archsimd.LoadInt32x4Array(&[4]int32{7, 7, 7, 7})
	shift25   = archsimd.LoadInt32x4Array(&[4]int32{-25, -25, -25, -25})

	// djbSeq2 offsets the two 64-bit counter lanes within a NEON batch.
	djbSeq2 = [2]uint64{0, 1}
)

// xorKeyStream is the arm64 SIMD backend hook. It XORs src with the key
// stream generated from state and maintains leftover key stream state.
func (cipher *Cipher) xorKeyStream(dst, src []byte) {
	if len(src) > 64 {
		cipher.xorKeyStreamNeon(dst, src)
	} else {
		cipher.xorKeyStreamScalar(dst, src)
	}
}

// xorKeyStreamNeon is the 4-block SIMD core. It XORs src with the key stream
// generated from state and returns the number of leftover key stream bytes
// (0..63) stored at leftover[:n].
//
// It processes 4 blocks (256 bytes) per iteration. If the input is not a
// multiple of 256 bytes, the final iteration produces the tail and any unused
// key stream of the last partial block is retained.
//
// For the IETF layout the block counter lives in word 12 and the counter
// vector is advanced with a single vector add per iteration. For the DJB
// layout the 64-bit counter spans words 12-13, so a scalar counter is kept and
// the low/high counter vectors are rebuilt from it each iteration.
func (cipher *Cipher) xorKeyStreamNeon(dst, src []byte) {
	// constant
	i0 := archsimd.BroadcastUint32x4(constant[0])
	i1 := archsimd.BroadcastUint32x4(constant[1])
	i2 := archsimd.BroadcastUint32x4(constant[2])
	i3 := archsimd.BroadcastUint32x4(constant[3])
	// key
	i4 := archsimd.BroadcastUint32x4(cipher.state[4])
	i5 := archsimd.BroadcastUint32x4(cipher.state[5])
	i6 := archsimd.BroadcastUint32x4(cipher.state[6])
	i7 := archsimd.BroadcastUint32x4(cipher.state[7])
	i8 := archsimd.BroadcastUint32x4(cipher.state[8])
	i9 := archsimd.BroadcastUint32x4(cipher.state[9])
	i10 := archsimd.BroadcastUint32x4(cipher.state[10])
	i11 := archsimd.BroadcastUint32x4(cipher.state[11])
	// word 13: nonce low for IETF, counter high for DJB
	i13 := archsimd.BroadcastUint32x4(cipher.state[13])
	// nonce
	i14 := archsimd.BroadcastUint32x4(cipher.state[14])
	i15 := archsimd.BroadcastUint32x4(cipher.state[15])

	counterIncrement := archsimd.BroadcastUint32x4(4)
	counter := archsimd.LoadUint32x4Array(&[4]uint32{cipher.state[12] + 0, cipher.state[12] + 1, cipher.state[12] + 2, cipher.state[12] + 3})

	var djbCounter uint64
	if cipher.djb {
		djbCounter = cipher.counter()
	}

	// len(dst) is not needed but it removes bound checks
	for len(src) >= 256 && len(dst) >= 256 {
		if cipher.djb {
			counter, i13 = djbCounterVectorsNeon(djbCounter)
		}
		w0, w1, w2, w3, w4, w5, w6, w7, w8, w9, w10, w11, w12, w13, w14, w15 :=
			chacha4BlocksNeon(i0, i1, i2, i3, i4, i5, i6, i7, i8, i9, i10, i11, counter, i13, i14, i15)

		srcPointer := unsafe.Pointer(&src[0])
		dstPointer := unsafe.Pointer(&dst[0])

		// xor source and store directly at destination
		xorStore(dstPointer, srcPointer, 0, w0)
		xorStore(dstPointer, srcPointer, 64, w1)
		xorStore(dstPointer, srcPointer, 128, w2)
		xorStore(dstPointer, srcPointer, 192, w3)

		xorStore(dstPointer, srcPointer, 16, w4)
		xorStore(dstPointer, srcPointer, 80, w5)
		xorStore(dstPointer, srcPointer, 144, w6)
		xorStore(dstPointer, srcPointer, 208, w7)

		xorStore(dstPointer, srcPointer, 32, w8)
		xorStore(dstPointer, srcPointer, 96, w9)
		xorStore(dstPointer, srcPointer, 160, w10)
		xorStore(dstPointer, srcPointer, 224, w11)

		xorStore(dstPointer, srcPointer, 48, w12)
		xorStore(dstPointer, srcPointer, 112, w13)
		xorStore(dstPointer, srcPointer, 176, w14)
		xorStore(dstPointer, srcPointer, 240, w15)

		src = src[256:]
		dst = dst[256:]
		if cipher.djb {
			djbCounter += 4
		} else {
			counter = counter.Add(counterIncrement)
		}
	}

	if !cipher.djb {
		cipher.state[12] = counter.GetElem(0)
	}

	// Tail: < 256 bytes. Generate 4 blocks of key stream into a local
	// buffer, XOR the consumed part, and keep the unused bytes of the last
	// partial block as leftover (<= 63).
	if len(src) > 0 {
		if cipher.djb {
			counter, i13 = djbCounterVectorsNeon(djbCounter)
		}
		var keystream [256]byte
		w0, w1, w2, w3, w4, w5, w6, w7, w8, w9, w10, w11, w12, w13, w14, w15 :=
			chacha4BlocksNeon(i0, i1, i2, i3, i4, i5, i6, i7, i8, i9, i10, i11, counter, i13, i14, i15)

		// block-major layout: block b words 4g..4g+3 at ks[b*64+g*16]
		w0.ReshapeToUint8s().Store(keystream[0:16])
		w1.ReshapeToUint8s().Store(keystream[64:80])
		w2.ReshapeToUint8s().Store(keystream[128:144])
		w3.ReshapeToUint8s().Store(keystream[192:208])

		w4.ReshapeToUint8s().Store(keystream[16:32])
		w5.ReshapeToUint8s().Store(keystream[80:96])
		w6.ReshapeToUint8s().Store(keystream[144:160])
		w7.ReshapeToUint8s().Store(keystream[208:224])

		w8.ReshapeToUint8s().Store(keystream[32:48])
		w9.ReshapeToUint8s().Store(keystream[96:112])
		w10.ReshapeToUint8s().Store(keystream[160:176])
		w11.ReshapeToUint8s().Store(keystream[224:240])

		w12.ReshapeToUint8s().Store(keystream[48:64])
		w13.ReshapeToUint8s().Store(keystream[112:128])
		w14.ReshapeToUint8s().Store(keystream[176:192])
		w15.ReshapeToUint8s().Store(keystream[240:256])

		cipher.processTail(dst, src, keystream[:], djbCounter)
		return
	}

	if cipher.djb {
		cipher.state[12] = uint32(djbCounter)
		cipher.state[13] = uint32(djbCounter >> 32)
	}
	cipher.leftoverLen = 0
}

// djbCounterVectorsNeon builds the word-12 (counter low) and word-13 (counter
// high) vectors for blocks c..c+3 in the DJB layout. The four 64-bit counters
// are computed in SIMD as two Uint64x2 vectors, split into low/high 32-bit
// words, and de-interleaved into the word-major layout NEON needs:
// loV = [lo(c)..lo(c+3)], hiV = [hi(c)..hi(c+3)]. The 64-bit add carries into
// the high word naturally, so no compare or branch is required.
func djbCounterVectorsNeon(c uint64) (loV, hiV archsimd.Uint32x4) {
	v0 := archsimd.BroadcastUint64x2(c).Add(archsimd.LoadUint64x2Array(&djbSeq2)) // [c, c+1]
	v1 := v0.Add(archsimd.BroadcastUint64x2(2))                                   // [c+2, c+3]
	a := v0.ReshapeToUint32s()                                                    // [lo(c), hi(c), lo(c+1), hi(c+1)]
	b := v1.ReshapeToUint32s()                                                    // [lo(c+2), hi(c+2), lo(c+3), hi(c+3)]
	return a.ConcatEven(b), a.ConcatOdd(b)                                        // VUZP1/VUZP2 -> low words, high words
}

// chacha4BlocksNeon computes 4 blocks of ChaCha20 key stream for the given
// counter (20 rounds + add-back) and transposes them into 16 block-major
// vectors. a[4g+b] holds words 4g..4g+3 of block b (b,g in 0..3), so block b's
// bytes are a[0*4+b], a[1*4+b], a[2*4+b], a[3*4+b] at byte offsets b*64+{0,16,32,48}.
func chacha4BlocksNeon(i0, i1, i2, i3, i4, i5, i6, i7, i8, i9, i10, i11, i12, i13, i14, i15 archsimd.Uint32x4) (w0, w1, w2, w3, w4, w5, w6, w7, w8, w9, w10, w11, w12, w13, w14, w15 archsimd.Uint32x4) {
	w0, w1, w2, w3, w4, w5, w6, w7, w8, w9, w10, w11, w12, w13, w14, w15 =
		i0, i1, i2, i3, i4, i5, i6, i7, i8, i9, i10, i11, i12, i13, i14, i15

	for r := 0; r < 10; r++ {
		w0, w4, w8, w12 = quarterRoundNeon(w0, w4, w8, w12)
		w1, w5, w9, w13 = quarterRoundNeon(w1, w5, w9, w13)
		w2, w6, w10, w14 = quarterRoundNeon(w2, w6, w10, w14)
		w3, w7, w11, w15 = quarterRoundNeon(w3, w7, w11, w15)

		w0, w5, w10, w15 = quarterRoundNeon(w0, w5, w10, w15)
		w1, w6, w11, w12 = quarterRoundNeon(w1, w6, w11, w12)
		w2, w7, w8, w13 = quarterRoundNeon(w2, w7, w8, w13)
		w3, w4, w9, w14 = quarterRoundNeon(w3, w4, w9, w14)
	}

	w0 = w0.Add(i0)
	w1 = w1.Add(i1)
	w2 = w2.Add(i2)
	w3 = w3.Add(i3)
	w4 = w4.Add(i4)
	w5 = w5.Add(i5)
	w6 = w6.Add(i6)
	w7 = w7.Add(i7)
	w8 = w8.Add(i8)
	w9 = w9.Add(i9)
	w10 = w10.Add(i10)
	w11 = w11.Add(i11)
	w12 = w12.Add(i12)
	w13 = w13.Add(i13)
	w14 = w14.Add(i14)
	w15 = w15.Add(i15)

	// transpose each word group into block-major vectors: group g output j is
	// block j's words 4g..4g+3.
	w0, w1, w2, w3 = transpose4Neon(w0, w1, w2, w3)
	w4, w5, w6, w7 = transpose4Neon(w4, w5, w6, w7)
	w8, w9, w10, w11 = transpose4Neon(w8, w9, w10, w11)
	w12, w13, w14, w15 = transpose4Neon(w12, w13, w14, w15)
	return
}

// xorStore XORs 16 bytes of keystream v with src[off:off+16] and stores the
// result into dst[off:off+16]. Caller guarantees a full 16-byte vector.
func xorStore(dst, src unsafe.Pointer, off int, v archsimd.Uint32x4) {
	v.Xor(archsimd.LoadUint32x4Array((*[4]uint32)(unsafe.Add(src, off)))).StoreArray((*[4]uint32)(unsafe.Add(dst, off)))
}

// The output stage can be sped up further with AArch64 structure loads/stores,
// which fold the 4x4 transpose into the memory access itself:
//
//	VLD4.4S {s0..s3}, [x_src], #64   // load 4 blocks, de-interleaved:
//	                                  //   s_i = word i across blocks 0..3
//	VEOR w0.16B, w0.16B, s0.16B      // (or keep keystream word-major and
//	VEOR w1.16B, w1.16B, s1.16B      //  XOR against s0..s3, one per word)
//	VEOR w2.16B, w2.16B, s2.16B
//	VEOR w3.16B, w3.16B, s3.16B
//	ST4.4S {w0..w3}, [x_dst], #64    // store interleaved by element:
//	                                  //   lane i writes block i's 4 words
//
// VST4 of the 4 word-major vectors w0..w3 (each holding one word across 4
// blocks) writes block 0's words 0-3, then block 1's words 0-3, etc. — i.e. it
// IS the transpose, fused into the store. So one word-group (4 words across 4
// blocks) costs 1 VLD4 + 4 VEOR + 1 VST4 = 6 instructions instead of the
// current 8 VUZP + 4 load + 4 EOR + 4 store = 20. Over the whole output that's
// ~24 vs ~80 instructions per 256-byte block (~3.3x fewer on the output stage).
//
// archsimd exposes no VLD4/VST4 (or any multi-vector interleave store), and the
// Go compiler will not synthesize them from this code, so this path requires a
// hand-written chacha20_arm64.s (or a linkname to one) and is out of scope here.
//
// transpose4Neon turns 4 word-major vectors (each holding word i of blocks
// counter..counter+3) into 4 block-major vectors (each holding 4 consecutive
// words of one block), using only VUZP1/VUZP2 (ConcatEven/ConcatOdd).
func transpose4Neon(a, b, c, d archsimd.Uint32x4) (archsimd.Uint32x4, archsimd.Uint32x4, archsimd.Uint32x4, archsimd.Uint32x4) {
	u0 := a.ConcatEven(b)
	u1 := a.ConcatOdd(b)
	u2 := c.ConcatEven(d)
	u3 := c.ConcatOdd(d)
	return u0.ConcatEven(u2), u1.ConcatEven(u3), u0.ConcatOdd(u2), u1.ConcatOdd(u3)
}

func quarterRoundNeon(a, b, c, d archsimd.Uint32x4) (archsimd.Uint32x4, archsimd.Uint32x4, archsimd.Uint32x4, archsimd.Uint32x4) {
	// unfortunately, as of now, RotateAllLeft is emulated on arm64 which leads to register pressure and
	// spilling on the stack be cause we are using 32 = 16 (initial state) + 16 (working state) + 6 ("constants")
	// registers which leads to performance

	// a += b; d ^= a; d <<<= 16
	a = a.Add(b)
	d = d.Xor(a)
	d = d.ReshapeToUint8s().LookupOrZero(rotl16Idx).ReshapeToUint32s()

	// c += d; b ^= c; b <<<= 12
	c = c.Add(d)
	b = b.Xor(c)
	b = b.Shift(shift12).Or(b.Shift(shift20))

	// a += b; d ^= a; d <<<= 8
	a = a.Add(b)
	d = d.Xor(a)
	d = d.ReshapeToUint8s().LookupOrZero(rotl8Idx).ReshapeToUint32s()

	// c += d; b ^= c; b <<<= 7
	c = c.Add(d)
	b = b.Xor(c)
	b = b.Shift(shift7).Or(b.Shift(shift25))

	return a, b, c, d
}
