package chacha20

// func XORKeyStream(key [32]byte, nonce [12]byte, counter uint32, dst, src []byte) {
// 	// load constant
// 	i0 := simd.BroadcastUint32s(0x61707865)
// 	i1 := simd.BroadcastUint32s(0x3320646e)
// 	i2 := simd.BroadcastUint32s(0x79622d32)
// 	i3 := simd.BroadcastUint32s(0x6b206574)

// 	// load key
// 	i4 := simd.BroadcastUint32s(binary.LittleEndian.Uint32(key[0:4]))
// 	i5 := simd.BroadcastUint32s(binary.LittleEndian.Uint32(key[4:8]))
// 	i6 := simd.BroadcastUint32s(binary.LittleEndian.Uint32(key[8:12]))
// 	i7 := simd.BroadcastUint32s(binary.LittleEndian.Uint32(key[12:16]))
// 	i8 := simd.BroadcastUint32s(binary.LittleEndian.Uint32(key[16:20]))
// 	i9 := simd.BroadcastUint32s(binary.LittleEndian.Uint32(key[20:24]))
// 	i10 := simd.BroadcastUint32s(binary.LittleEndian.Uint32(key[24:28]))
// 	i11 := simd.BroadcastUint32s(binary.LittleEndian.Uint32(key[28:32]))

// 	// load nonce
// 	i13 := simd.BroadcastUint32s(binary.LittleEndian.Uint32(nonce[0:4]))
// 	i14 := simd.BroadcastUint32s(binary.LittleEndian.Uint32(nonce[4:8]))
// 	i15 := simd.BroadcastUint32s(binary.LittleEndian.Uint32(nonce[8:12]))

// 	nLanes := i0.Len()

// 	for len(src) > 0 {
// 		var ct [16]uint32
// 		for i := 0; i < nLanes; i++ {
// 			ct[i] = counter + uint32(i)
// 		}

// 		var i12 simd.Uint32s
// 		switch nLanes {
// 		case 4:
// 			i12 = simd.LoadUint32s(ct[:4])
// 		case 8:
// 			i12 = simd.LoadUint32s(ct[:8])
// 		case 16:
// 			i12 = simd.LoadUint32s(ct[:16])
// 		default:
// 			i12 = simd.LoadUint32s(ct[:nLanes])
// 		}

// 		w0, w1, w2, w3, w4, w5, w6, w7, w8, w9, w10, w11, w12, w13, w14, w15 :=
// 			i0, i1, i2, i3, i4, i5, i6, i7, i8, i9, i10, i11, i12, i13, i14, i15

// 		for r := 0; r < 10; r++ {
// 			w0, w4, w8, w12 = quarterRound(w0, w4, w8, w12)
// 			w1, w5, w9, w13 = quarterRound(w1, w5, w9, w13)
// 			w2, w6, w10, w14 = quarterRound(w2, w6, w10, w14)
// 			w3, w7, w11, w15 = quarterRound(w3, w7, w11, w15)

// 			w0, w5, w10, w15 = quarterRound(w0, w5, w10, w15)
// 			w1, w6, w11, w12 = quarterRound(w1, w6, w11, w12)
// 			w2, w7, w8, w13 = quarterRound(w2, w7, w8, w13)
// 			w3, w4, w9, w14 = quarterRound(w3, w4, w9, w14)
// 		}

// 		w0 = w0.Add(i0)
// 		w1 = w1.Add(i1)
// 		w2 = w2.Add(i2)
// 		w3 = w3.Add(i3)
// 		w4 = w4.Add(i4)
// 		w5 = w5.Add(i5)
// 		w6 = w6.Add(i6)
// 		w7 = w7.Add(i7)
// 		w8 = w8.Add(i8)
// 		w9 = w9.Add(i9)
// 		w10 = w10.Add(i10)
// 		w11 = w11.Add(i11)
// 		w12 = w12.Add(i12)
// 		w13 = w13.Add(i13)
// 		w14 = w14.Add(i14)
// 		w15 = w15.Add(i15)

// 		var r0, r1, r2, r3, r4, r5, r6, r7 [16]uint32
// 		var r8, r9, r10, r11, r12, r13, r14, r15 [16]uint32

// 		w0.Store(r0[:nLanes])
// 		w1.Store(r1[:nLanes])
// 		w2.Store(r2[:nLanes])
// 		w3.Store(r3[:nLanes])
// 		w4.Store(r4[:nLanes])
// 		w5.Store(r5[:nLanes])
// 		w6.Store(r6[:nLanes])
// 		w7.Store(r7[:nLanes])
// 		w8.Store(r8[:nLanes])
// 		w9.Store(r9[:nLanes])
// 		w10.Store(r10[:nLanes])
// 		w11.Store(r11[:nLanes])
// 		w12.Store(r12[:nLanes])
// 		w13.Store(r13[:nLanes])
// 		w14.Store(r14[:nLanes])
// 		w15.Store(r15[:nLanes])

// 		var keystream [1024]byte

// 		for lane := 0; lane < nLanes && len(src) > 0; lane++ {
// 			off := lane * 64

// 			binary.LittleEndian.PutUint32(keystream[off:], r0[lane])
// 			binary.LittleEndian.PutUint32(keystream[off+4:], r1[lane])
// 			binary.LittleEndian.PutUint32(keystream[off+8:], r2[lane])
// 			binary.LittleEndian.PutUint32(keystream[off+12:], r3[lane])
// 			binary.LittleEndian.PutUint32(keystream[off+16:], r4[lane])
// 			binary.LittleEndian.PutUint32(keystream[off+20:], r5[lane])
// 			binary.LittleEndian.PutUint32(keystream[off+24:], r6[lane])
// 			binary.LittleEndian.PutUint32(keystream[off+28:], r7[lane])
// 			binary.LittleEndian.PutUint32(keystream[off+32:], r8[lane])
// 			binary.LittleEndian.PutUint32(keystream[off+36:], r9[lane])
// 			binary.LittleEndian.PutUint32(keystream[off+40:], r10[lane])
// 			binary.LittleEndian.PutUint32(keystream[off+44:], r11[lane])
// 			binary.LittleEndian.PutUint32(keystream[off+48:], r12[lane])
// 			binary.LittleEndian.PutUint32(keystream[off+52:], r13[lane])
// 			binary.LittleEndian.PutUint32(keystream[off+56:], r14[lane])
// 			binary.LittleEndian.PutUint32(keystream[off+60:], r15[lane])
// 		}

// 		// n := nLanes * 64
// 		// if len(src) < n {
// 		// 	n = len(src)
// 		// }
// 		// for i := 0; i < n; i++ {
// 		// 	dst[i] = src[i] ^ keystream[i]
// 		// }
// 		n := subtle.XORBytes(dst, src, keystream[:nLanes*64])
// 		src = src[n:]
// 		dst = dst[n:]
// 		counter += uint32(nLanes)
// 	}
// }

// func quarterRound(a, b, c, d simd.Uint32s) (simd.Uint32s, simd.Uint32s, simd.Uint32s, simd.Uint32s) {
// 	a = a.Add(b)
// 	d = d.Xor(a)
// 	d = d.RotateAllLeft(16)

// 	c = c.Add(d)
// 	b = b.Xor(c)
// 	b = b.RotateAllLeft(12)

// 	a = a.Add(b)
// 	d = d.Xor(a)
// 	d = d.RotateAllLeft(8)

// 	c = c.Add(d)
// 	b = b.Xor(c)
// 	b = b.RotateAllLeft(7)

// 	return a, b, c, d
// }
