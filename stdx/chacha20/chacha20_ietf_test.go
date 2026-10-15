package chacha20

import (
	"bytes"
	"testing"

	xchacha20 "golang.org/x/crypto/chacha20"
)

func testKey() [32]byte {
	var key [32]byte
	for i := range key {
		key[i] = byte(i)
	}
	return key
}

func testNonce() [12]byte {
	var nonce [12]byte
	for i := range nonce {
		nonce[i] = byte(i)
	}
	return nonce
}

func testPlaintext(n int) []byte {
	p := make([]byte, n)
	for i := range p {
		p[i] = byte(i % 251)
	}
	return p
}

func refXOR(t *testing.T, key, nonce []byte, counter uint32, src []byte) []byte {
	t.Helper()
	ref, err := xchacha20.NewUnauthenticatedCipher(key, nonce)
	if err != nil {
		t.Fatal(err)
	}
	ref.SetCounter(counter)
	dst := make([]byte, len(src))
	ref.XORKeyStream(dst, src)
	return dst
}

func xorOneShot(c Cipher, src []byte) []byte {
	dst := make([]byte, len(src))
	c.XORKeyStream(dst, src)
	return dst
}

func TestCipherIetfRFC8439(t *testing.T) {
	// RFC 8439 section 2.4.2 (A.2 first test).
	key, _ := parseHex("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	nonce, _ := parseHex("000000000000004a00000000")
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

	c := NewIetf([32]byte(key), [12]byte(nonce))
	c.SetCounter(1)
	got := xorOneShot(c, plaintext)
	if !bytes.Equal(got, expected) {
		t.Fatalf("RFC8439 2.4.2 mismatch")
	}
}

func parseHex(s string) ([]byte, error) {
	var out []byte
	for i := 0; i < len(s); i += 2 {
		hi := hexVal(s[i])
		lo := hexVal(s[i+1])
		out = append(out, hi<<4|lo)
	}
	return out, nil
}

func hexVal(b byte) byte {
	switch {
	case '0' <= b && b <= '9':
		return b - '0'
	case 'a' <= b && b <= 'f':
		return b - 'a' + 10
	case 'A' <= b && b <= 'F':
		return b - 'A' + 10
	}
	return 0
}

func TestCipherIetfCrossCheck(t *testing.T) {
	key := testKey()
	nonce := testNonce()

	sizes := []int{0, 1, 2, 3, 63, 64, 65, 127, 128, 129, 255, 256, 257, 511, 512, 513, 1000, 4096}
	counters := []uint32{0, 1, 1000}

	for _, size := range sizes {
		for _, counter := range counters {
			pt := testPlaintext(size)
			ref := refXOR(t, key[:], nonce[:], counter, pt)

			c := NewIetf(key, nonce)
			c.SetCounter(uint64(counter))
			got := xorOneShot(c, pt)
			if !bytes.Equal(ref, got) {
				t.Fatalf("cross-check mismatch: size=%d counter=%d", size, counter)
			}
		}
	}

	// High counters (near wrap): single block only, since x/crypto panics on
	// actual counter overflow.
	for _, size := range []int{1, 3, 63, 64} {
		for _, counter := range []uint32{0xfffffffe, 0xfffffffc} {
			pt := testPlaintext(size)
			ref := refXOR(t, key[:], nonce[:], counter, pt)

			c := NewIetf(key, nonce)
			c.SetCounter(uint64(counter))
			got := xorOneShot(c, pt)
			if !bytes.Equal(ref, got) {
				t.Fatalf("high-counter mismatch: size=%d counter=%d", size, counter)
			}
		}
	}
}

func TestCipherIetfLeftoverMultiCall(t *testing.T) {
	key := testKey()
	nonce := testNonce()
	pt := testPlaintext(300)
	ref := refXOR(t, key[:], nonce[:], 0, pt)

	// partial block -> leftover, partially consume, then finish
	{
		c := NewIetf(key, nonce)
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
		c := NewIetf(key, nonce)
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
		c := NewIetf(key, nonce)
		buf := append([]byte(nil), pt...)
		c.XORKeyStream(buf[:5], buf[:5])
		c.XORKeyStream(buf[5:12], buf[5:12])
		c.XORKeyStream(buf[12:20], buf[12:20])
		c.XORKeyStream(buf[20:], buf[20:])
		if !bytes.Equal(buf, ref) {
			t.Fatal("multiple partial leftover consumptions")
		}
	}
}

func TestCipherIetfLeftoverTinyChunks(t *testing.T) {
	key := testKey()
	nonce := testNonce()
	pt := testPlaintext(200)
	ref := refXOR(t, key[:], nonce[:], 0, pt)

	c := NewIetf(key, nonce)
	buf := append([]byte(nil), pt...)
	for n := 0; n < 10; n++ {
		c.XORKeyStream(buf[n:n+1], buf[n:n+1])
	}
	c.XORKeyStream(buf[10:], buf[10:])
	if !bytes.Equal(buf, ref) {
		t.Fatal("tiny-chunk failed")
	}
}

func TestCipherIetfLeftoverBoundary(t *testing.T) {
	key := testKey()
	nonce := testNonce()

	for _, size := range []int{63, 64, 65, 127, 128, 129, 191, 192, 193, 255, 256, 257} {
		pt := testPlaintext(size)
		ref := refXOR(t, key[:], nonce[:], 0, pt)

		for _, a := range []int{0, 1, 8, 31, 32, 33, 62, 63} {
			if a > size {
				continue
			}
			for _, b := range []int{0, 1, 7, 32, 63, 64} {
				if a+b > size {
					continue
				}
				c := NewIetf(key, nonce)
				buf := append([]byte(nil), pt...)
				off := 0
				for _, chunk := range []int{a, b, size - a - b} {
					c.XORKeyStream(buf[off:off+chunk], buf[off:off+chunk])
					off += chunk
				}
				if !bytes.Equal(buf, ref) {
					t.Fatalf("boundary failed: size=%d a=%d b=%d", size, a, b)
				}
			}
		}
	}
}

func TestCipherIetfZeroByteIntermediate(t *testing.T) {
	key := testKey()
	nonce := testNonce()
	pt := testPlaintext(150)
	ref := refXOR(t, key[:], nonce[:], 0, pt)

	c := NewIetf(key, nonce)
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

func TestCipherIetfSetCounterRollback(t *testing.T) {
	key := testKey()
	nonce := testNonce()
	pt := testPlaintext(200)
	ref := refXOR(t, key[:], nonce[:], 0, pt)

	// encrypt 10 bytes, reset counter to 0, re-XOR those 10 bytes (back to
	// plaintext), then encrypt the rest.
	c := NewIetf(key, nonce)
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

func TestCipherIetfInPlace(t *testing.T) {
	key := testKey()
	nonce := testNonce()
	pt := testPlaintext(1000)
	ref := refXOR(t, key[:], nonce[:], 0, pt)

	c := NewIetf(key, nonce)
	buf := append([]byte(nil), pt...)
	c.XORKeyStream(buf, buf)
	if !bytes.Equal(buf, ref) {
		t.Fatal("in-place failed")
	}
}

func TestCipherIetfScalar(t *testing.T) {
	// Force the scalar path (input < 128) and verify against the reference.
	key := testKey()
	nonce := testNonce()

	for _, size := range []int{1, 3, 63, 64, 65, 127} {
		pt := testPlaintext(size)
		ref := refXOR(t, key[:], nonce[:], 0, pt)
		c := NewIetf(key, nonce)
		got := xorOneShot(c, pt)
		if !bytes.Equal(ref, got) {
			t.Fatalf("scalar mismatch: size=%d", size)
		}
	}
}
