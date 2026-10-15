package chacha20

import (
	"bytes"
	"crypto/cipher"
	"testing"
)

// testDjbNonce returns the 8-byte nonce for the DJB tests.
func testDjbNonce() [8]byte {
	var nonce [8]byte
	for i := range nonce {
		nonce[i] = byte(255 - i*3)
	}
	return nonce
}

// djbToIetfNonce pads a DJB (8-byte) nonce to the IETF (12-byte) layout with
// four leading zero bytes. When the block counter is below 2^32, the DJB and
// IETF layouts produce identical states, so NewDjb(key, nonce).SetCounter(c)
// must match NewIetf(key, padded).SetCounter(c).
func djbToIetfNonce(nonce [8]byte) [12]byte {
	var ietf [12]byte
	copy(ietf[4:], nonce[:])
	return ietf
}

func TestCipherDjbRFC8439(t *testing.T) {
	// RFC 8439 section 2.4.2 (A.2 first test) in the DJB layout: the 8-byte
	// nonce is the last 8 bytes of the RFC's 12-byte nonce and the counter is 1.
	{
		key, _ := parseHex("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
		nonce, _ := parseHex("0000004a00000000")
		plaintext, _ := parseHex(
			"4c616469657320616e642047656e746c" +
				"656d656e206f662074686520636c6173" +
				"73206f66202739393a20496620492063" +
				"6f756c64206f6666657220796f75206f" +
				"6e6c79206f6e652074697020666f7220" +
				"746865206675747572652c2073756e73" +
				"637265656e20776f756c642062652069" +
				"742e")
		expected, _ := parseHex(
			"6e2e359a2568f98041ba0728dd0d6981" +
				"e97e7aec1d4360c20a27afccfd9fae0b" +
				"f91b65c5524733ab8f593dabcd62b357" +
				"1639d624e65152ab8f530c359f0861d8" +
				"07ca0dbf500d6a6156a38e088a22b65e" +
				"52bc514d16ccf806818ce91ab7793736" +
				"5af90bbf74a35be6b40b8eedf2785e42" +
				"874d")

		c := NewDjb([32]byte(key), [8]byte(nonce))
		c.SetCounter(1)
		got := xorOneShot(c, plaintext)
		if !bytes.Equal(got, expected) {
			t.Fatalf("RFC8439 2.4.2 (DJB) mismatch")
		}
	}

	// RFC 8439 A.2 test vector #1 in the DJB layout.
	{
		key, _ := parseHex("0000000000000000000000000000000000000000000000000000000000000000")
		nonce, _ := parseHex("0000000000000000")
		plaintext := make([]byte, 64)
		expected, _ := parseHex(
			"76b8e0ada0f13d90405d6ae55386bd28" +
				"bdd219b8a08ded1aa836efcc8b770dc7" +
				"da41597c5157488d7724e03fb8d84a37" +
				"6a43b8f41518a11cc387b669b2ee6586")

		c := NewDjb([32]byte(key), [8]byte(nonce))
		got := xorOneShot(c, plaintext)
		if !bytes.Equal(got, expected) {
			t.Fatalf("RFC8439 A.2 #1 (DJB) mismatch")
		}
	}

	// RFC 8439 A.2 test vector #2 in the DJB layout.
	{
		key, _ := parseHex("0000000000000000000000000000000000000000000000000000000000000001")
		nonce, _ := parseHex("0000000000000002")
		plaintext, _ := parseHex(
			"416e79207375626d697373696f6e2074" +
				"6f20746865204945544620696e74656e" +
				"6465642062792074686520436f6e7472" +
				"696275746f7220666f72207075626c69" +
				"636174696f6e20617320616c6c206f72" +
				"2070617274206f6620616e2049455446" +
				"20496e7465726e65742d447261667420" +
				"6f722052464320616e6420616e792073" +
				"746174656d656e74206d616465207769" +
				"7468696e2074686520636f6e74657874" +
				"206f6620616e20494554462061637469" +
				"7669747920697320636f6e7369646572" +
				"656420616e20224945544620436f6e74" +
				"7269627574696f6e222e205375636820" +
				"73746174656d656e747320696e636c75" +
				"6465206f72616c2073746174656d656e" +
				"747320696e2049455446207365737369" +
				"6f6e732c2061732077656c6c20617320" +
				"7772697474656e20616e6420656c6563" +
				"74726f6e696320636f6d6d756e696361" +
				"74696f6e73206d61646520617420616e" +
				"792074696d65206f7220706c6163652c" +
				"20776869636820617265206164647265" +
				"7373656420746f")
		expected, _ := parseHex(
			"a3fbf07df3fa2fde4f376ca23e82737041605d9f4f4f57bd8cff2c1d4b7955ec2a97948bd3722915c8f3d337f7d37005" +
				"0e9e96d647b7c39f56e031ca5eb6250d4042e02785ececfa4b4bb5e8ead0440e20b6e8db09d881a7c6132f420e527950" +
				"42bdfa7773d8a9051447b3291ce1411c680465552aa6c405b7764d5e87bea85ad00f8449ed8f72d0d662ab052691ca66" +
				"424bc86d2df80ea41f43abf937d3259dc4b2d0dfb48a6c9139ddd7f76966e928e635553ba76c5c879d7b35d49eb2e62b" +
				"0871cdac638939e25e8a1e0ef9d5280fa8ca328b351c3c765989cbcf3daa8b6ccc3aaf9f3979c92b3720fc88dc95ed84" +
				"a1be059c6499b9fda236e7e818b04b0bc39c1e876b193bfe5569753f88128cc08aaa9b63d1a16f80ef2554d7189c411f" +
				"5869ca52c5b83fa36ff216b9c1d30062bebcfd2dc5bce0911934fda79a86f6e698ced759c3ff9b6477338f3da4f9cd85" +
				"14ea9982ccafb341b2384dd902f3d1ab7ac61dd29c6f21ba5b862f3730e37cfdc4fd806c22f221")

		c := NewDjb([32]byte(key), [8]byte(nonce))
		c.SetCounter(1)
		got := xorOneShot(c, plaintext)
		if !bytes.Equal(got, expected) {
			t.Fatalf("RFC8439 A.2 #2 (DJB) mismatch")
		}
	}

	// RFC 8439 A.2 test vector #3 in the DJB layout.
	{
		key, _ := parseHex("1c9240a5eb55d38af333888604f6b5f0473917c1402b80099dca5cbc207075c0")
		nonce, _ := parseHex("0000000000000002")
		plaintext, _ := parseHex(
			"2754776173206272696c6c69672c2061" +
				"6e642074686520736c6974687920746f" +
				"7665730a446964206779726520616e64" +
				"2067696d626c6520696e207468652077" +
				"6162653a0a416c6c206d696d73792077" +
				"6572652074686520626f726f676f7665" +
				"732c0a416e6420746865206d6f6d6520" +
				"7261746873206f757467726162652e")
		expected, _ := parseHex(
			"62e6347f95ed87a45ffae7426f27a1df5fb69110044c0d73118effa95b01e5cf166d3df2d721caf9b21e5fb14c616871" +
				"fd84c54f9d65b283196c7fe4f60553ebf39c6402c42234e32a356b3e764312a61a5532055716ead6962568f87d3f3f77" +
				"04c6a8d1bcd1bf4d50d6154b6da731b187b58dfd728afa36757a797ac188d1")

		c := NewDjb([32]byte(key), [8]byte(nonce))
		c.SetCounter(42)
		got := xorOneShot(c, plaintext)
		if !bytes.Equal(got, expected) {
			t.Fatalf("RFC8439 A.2 #3 (DJB) mismatch")
		}
	}
}

func TestCipherDjbStreamInterface(t *testing.T) {
	var _ cipher.Stream = (*Cipher)(nil)
}

// djbScalarXOR is the portable reference: it runs the scalar backend on a copy
// of the cipher and returns the XORed output.
func djbScalarXOR(c Cipher, src []byte) []byte {
	dst := make([]byte, len(src))
	c.xorKeyStreamScalar(dst, src)
	return dst
}

func TestCipherDjbCrossCheckIetf(t *testing.T) {
	// For counters below 2^32 the DJB and IETF layouts share the same state,
	// so the DJB cipher must match NewIetf (already validated against
	// x/crypto) with the padded nonce.
	key := testKey()
	nonce := testDjbNonce()

	sizes := []int{0, 1, 2, 3, 63, 64, 65, 127, 128, 129, 255, 256, 257, 511, 512, 513, 1000, 4096}
	counters := []uint32{0, 1, 42, 1000}

	for _, size := range sizes {
		for _, counter := range counters {
			pt := testPlaintext(size)

			ietf := NewIetf(key, djbToIetfNonce(nonce))
			ietf.SetCounter(uint64(counter))
			ref := xorOneShot(ietf, pt)

			djb := NewDjb(key, nonce)
			djb.SetCounter(uint64(counter))
			got := xorOneShot(djb, pt)

			if !bytes.Equal(ref, got) {
				t.Fatalf("DJB/IETF mismatch: size=%d counter=%d", size, counter)
			}
		}
	}

	// The IETF reference panics on counter exhaustion, so the 0xffffffff
	// cross-check is limited to a single block.
	for _, size := range []int{1, 3, 63, 64} {
		pt := testPlaintext(size)

		ietf := NewIetf(key, djbToIetfNonce(nonce))
		ietf.SetCounter(0xffffffff)
		ref := xorOneShot(ietf, pt)

		djb := NewDjb(key, nonce)
		djb.SetCounter(0xffffffff)
		got := xorOneShot(djb, pt)

		if !bytes.Equal(ref, got) {
			t.Fatalf("DJB/IETF mismatch at counter wrap: size=%d", size)
		}
	}
}

func TestCipherDjbSimdVsScalar(t *testing.T) {
	// The strongest check of the per-architecture SIMD counter wiring: the
	// SIMD path (XORKeyStream on large inputs) must match the scalar backend
	// (xorKeyStreamScalar) for identical cipher state, including counters that
	// cross 2^32, 2^33 and 2^64 boundaries where the high counter word
	// increments mid-batch.
	key := testKey()
	nonce := testDjbNonce()

	// counters: near 2^32 (IETF would overflow here), crossing 2^33, carrying
	// within an 8/16-block batch (0x1_ffffffF8), crossing 2^64, and one that
	// wraps the full 64-bit counter (0xffffffff_fffffff8).
	counters := []uint64{
		0x00000000_ffffff00,
		0x00000000_fffffff8,
		0x00000000_ffffffff,
		0x00000001_00000000,
		0x00000001_ffffff80,
		0x00000001_fffffff8,
		0x00000001_fffffffe,
		0x00000002_00000000,
		0xffffffff_fffffff0,
		0xffffffff_fffffff8,
		0x00000080_00000000,
	}
	sizes := []int{1, 63, 64, 65, 127, 128, 129, 255, 256, 257, 511, 512, 513, 767, 1023, 1024, 1025, 2048, 5000}

	for _, counter := range counters {
		for _, size := range sizes {
			pt := testPlaintext(size)

			simd := NewDjb(key, nonce)
			simd.SetCounter(counter)
			dst := make([]byte, size)
			simd.XORKeyStream(dst, pt)

			scalar := NewDjb(key, nonce)
			scalar.SetCounter(counter)
			ref := djbScalarXOR(scalar, pt)

			if !bytes.Equal(dst, ref) {
				t.Fatalf("SIMD/scalar mismatch: size=%d counter=%#x", size, counter)
			}
		}
	}
}

func TestCipherDjbLeftoverMultiCall(t *testing.T) {
	key := testKey()
	nonce := testDjbNonce()
	pt := testPlaintext(300)
	ref := djbScalarXOR(NewDjb(key, nonce), pt)

	// partial block -> leftover, partially consume, then finish
	{
		c := NewDjb(key, nonce)
		buf := append([]byte(nil), pt...)
		c.XORKeyStream(buf[:10], buf[:10])
		c.XORKeyStream(buf[10:15], buf[10:15])
		c.XORKeyStream(buf[15:], buf[15:])
		if !bytes.Equal(buf, ref) {
			t.Fatal("partial leftover consumption")
		}
	}

	// partial block -> exactly exhaust leftover -> fresh blocks
	{
		c := NewDjb(key, nonce)
		buf := append([]byte(nil), pt...)
		c.XORKeyStream(buf[:3], buf[:3])
		c.XORKeyStream(buf[3:64], buf[3:64])
		c.XORKeyStream(buf[64:], buf[64:])
		if !bytes.Equal(buf, ref) {
			t.Fatal("exact leftover exhaustion")
		}
	}

	// multiple rounds of partial consumption
	{
		c := NewDjb(key, nonce)
		buf := append([]byte(nil), pt...)
		c.XORKeyStream(buf[:5], buf[:5])
		c.XORKeyStream(buf[5:12], buf[5:12])
		c.XORKeyStream(buf[12:20], buf[12:20])
		c.XORKeyStream(buf[20:], buf[20:])
		if !bytes.Equal(buf, ref) {
			t.Fatal("multiple partial leftover consumptions")
		}
	}

	// tiny chunks
	{
		c := NewDjb(key, nonce)
		buf := append([]byte(nil), pt...)
		for n := 0; n < 10; n++ {
			c.XORKeyStream(buf[n:n+1], buf[n:n+1])
		}
		c.XORKeyStream(buf[10:], buf[10:])
		if !bytes.Equal(buf, ref) {
			t.Fatal("tiny-chunk failed")
		}
	}
}

func TestCipherDjbLeftoverAcrossSimdBatches(t *testing.T) {
	// Multi-call streams whose chunk boundaries cross the SIMD batch sizes
	// (256 NEON, 512 AVX2, 1024 AVX-512) and the tail/leftover path.
	key := testKey()
	nonce := testDjbNonce()

	for _, size := range []int{256, 512, 1024, 1500, 3000} {
		pt := testPlaintext(size)
		ref := djbScalarXOR(NewDjb(key, nonce), pt)

		for _, a := range []int{0, 1, 63, 64, 65, 255, 256, 257} {
			if a > size {
				continue
			}
			for _, b := range []int{0, 1, 64, 128, 512} {
				if a+b > size {
					continue
				}
				c := NewDjb(key, nonce)
				buf := append([]byte(nil), pt...)
				off := 0
				for _, chunk := range []int{a, b, size - a - b} {
					if chunk == 0 {
						continue
					}
					c.XORKeyStream(buf[off:off+chunk], buf[off:off+chunk])
					off += chunk
				}
				if !bytes.Equal(buf, ref) {
					t.Fatalf("multi-call SIMD/tail mismatch: size=%d a=%d b=%d", size, a, b)
				}
			}
		}
	}
}

func TestCipherDjbHighCounterWriteBack(t *testing.T) {
	// Two sequential calls must produce a keystream identical to a single
	// scalar call, including when the second call starts with a high counter
	// word (state write-back of words 12-13).
	key := testKey()
	nonce := testDjbNonce()

	for _, counter := range []uint64{0x00000000_ffffff00, 0x00000001_ffffff80, 0xffffffff_fffffff0} {
		pt := testPlaintext(3000)
		ref := NewDjb(key, nonce)
		ref.SetCounter(counter)
		refDst := djbScalarXOR(ref, pt)

		c := NewDjb(key, nonce)
		c.SetCounter(counter)
		dst := make([]byte, len(pt))
		c.XORKeyStream(dst[:1000], pt[:1000])
		c.XORKeyStream(dst[1000:], pt[1000:])

		if !bytes.Equal(dst, refDst) {
			t.Fatalf("multi-call write-back mismatch: counter=%#x", counter)
		}
	}
}

func TestCipherDjbSetCounterRollback(t *testing.T) {
	key := testKey()
	nonce := testDjbNonce()
	pt := testPlaintext(200)
	ref := djbScalarXOR(NewDjb(key, nonce), pt)

	// encrypt 10 bytes, reset counter to 0, re-XOR those 10 bytes (back to
	// plaintext), then encrypt the rest.
	c := NewDjb(key, nonce)
	buf := append([]byte(nil), pt...)
	c.XORKeyStream(buf[:10], buf[:10])
	c.SetCounter(0)
	c.XORKeyStream(buf[:10], buf[:10])
	c.XORKeyStream(buf[10:], buf[10:])

	if !bytes.Equal(buf[:10], pt[:10]) {
		t.Fatal("rollback: first 10 bytes should be plaintext")
	}
	if !bytes.Equal(buf[10:], ref[10:]) {
		t.Fatal("rollback failed")
	}
}

func TestCipherDjbInPlace(t *testing.T) {
	key := testKey()
	nonce := testDjbNonce()
	pt := testPlaintext(1000)
	ref := djbScalarXOR(NewDjb(key, nonce), pt)

	c := NewDjb(key, nonce)
	buf := append([]byte(nil), pt...)
	c.XORKeyStream(buf, buf)
	if !bytes.Equal(buf, ref) {
		t.Fatal("in-place failed")
	}
}

func TestCipherDjbNoOverflowAt2_32(t *testing.T) {
	// The IETF layout panics when the 32-bit counter is exhausted; the DJB
	// 64-bit counter must not.
	key := testKey()
	nonce := testDjbNonce()
	pt := testPlaintext(64)

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("DJB panicked at 2^32: %v", r)
			}
		}()
		c := NewDjb(key, nonce)
		c.SetCounter(0xffffffff)
		c.XORKeyStream(make([]byte, 64), pt)
	}()
}

func TestCipherDjbUnaligned(t *testing.T) {
	key := testKey()
	nonce := testDjbNonce()

	for _, srcOff := range []int{1, 3, 7} {
		for _, dstOff := range []int{1, 5, 9} {
			size := 1024
			full := make([]byte, size+16)
			for i := range full {
				full[i] = byte(i*3 + 7)
			}
			src := full[srcOff : srcOff+size]
			ref := djbScalarXOR(NewDjb(key, nonce), src)

			dstFull := make([]byte, size+16)
			dst := dstFull[dstOff : dstOff+size]
			c := NewDjb(key, nonce)
			c.XORKeyStream(dst, src)

			if !bytes.Equal(ref, dst) {
				t.Fatalf("unaligned mismatch: srcOff=%d dstOff=%d", srcOff, dstOff)
			}
		}
	}
}

func TestCipherDjbZeroByteIntermediate(t *testing.T) {
	key := testKey()
	nonce := testDjbNonce()
	pt := testPlaintext(150)
	ref := djbScalarXOR(NewDjb(key, nonce), pt)

	c := NewDjb(key, nonce)
	buf := append([]byte(nil), pt...)
	c.XORKeyStream(buf[:10], buf[:10])
	c.XORKeyStream(nil, nil)
	c.XORKeyStream(buf[10:20], buf[10:20])
	c.XORKeyStream(nil, nil)
	c.XORKeyStream(buf[20:], buf[20:])
	if !bytes.Equal(buf, ref) {
		t.Fatal("zero-byte intermediate failed")
	}
}

func TestCipherDjbScalarOnly(t *testing.T) {
	// Force the scalar path (input < 64) and verify against itself.
	key := testKey()
	nonce := testDjbNonce()

	for _, size := range []int{1, 3, 63, 64} {
		pt := testPlaintext(size)
		c := NewDjb(key, nonce)
		got := xorOneShot(c, pt)
		ref := djbScalarXOR(NewDjb(key, nonce), pt)
		if !bytes.Equal(ref, got) {
			t.Fatalf("scalar mismatch: size=%d", size)
		}
	}
}
