package chacha

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"testing"
)

// Test that the out buffer is actually filled with some data to make sure that we do not pass the array
// by value instead of a reference to mutate it
func TestHChaCha20NotEmpty(t *testing.T) {
	// var out [32]byte
	var emptyOut [32]byte
	var key [32]byte
	var nonce [16]byte

	rand.Read(key[:])
	rand.Read(nonce[:])

	out, _ := HChaCha20(key[:], nonce[:])

	if bytes.Equal(out[:], emptyOut[:]) {
		fmt.Println("out", hex.Dump(out[:]))
		fmt.Println("emptyOut", hex.Dump(emptyOut[:]))
		t.Fatalf("out is empty")
	}
}

func TestHChaCha20(t *testing.T) {
	defer func(sse2, ssse3, avx, avx2 bool) {
		useSSE2, useSSSE3, useAVX, useAVX2 = sse2, ssse3, avx, avx2
	}(useSSE2, useSSSE3, useAVX, useAVX2)

	if useAVX2 {
		t.Log("AVX2 version")
		testHChaCha20(t)
		useAVX2 = false
	}
	if useAVX {
		t.Log("AVX version")
		testHChaCha20(t)
		useAVX = false
	}
	if useSSSE3 {
		t.Log("SSSE3 version")
		testHChaCha20(t)
		useSSSE3 = false
	}
	if useSSE2 {
		t.Log("SSE2 version")
		testHChaCha20(t)
		useSSE2 = false
	}
	t.Log("generic version")
	testHChaCha20(t)
}

func testHChaCha20(t *testing.T) {
	for i, v := range hChaCha20Vectors {
		var key [32]byte
		var nonce [16]byte
		copy(key[:], v.key)
		copy(nonce[:], v.nonce)

		out, _ := HChaCha20(key[:], nonce[:])
		if !bytes.Equal(out, v.keystream) {
			t.Errorf("Test %d: keystream mismatch:\n \t got:  %s\n \t want: %s", i, toHex(key[:]), toHex(v.keystream))
		}
	}
}

var hChaCha20Vectors = []struct {
	key, nonce, keystream []byte
}{
	{
		fromHex("0000000000000000000000000000000000000000000000000000000000000000"),
		fromHex("000000000000000000000000000000000000000000000000"),
		fromHex("1140704c328d1d5d0e30086cdf209dbd6a43b8f41518a11cc387b669b2ee6586"),
	},
	{
		fromHex("8000000000000000000000000000000000000000000000000000000000000000"),
		fromHex("000000000000000000000000000000000000000000000000"),
		fromHex("7d266a7fd808cae4c02a0a70dcbfbcc250dae65ce3eae7fc210f54cc8f77df86"),
	},
	{
		fromHex("0000000000000000000000000000000000000000000000000000000000000001"),
		fromHex("000000000000000000000000000000000000000000000002"),
		fromHex("e0c77ff931bb9163a5460c02ac281c2b53d792b1c43fea817e9ad275ae546963"),
	},
	{
		fromHex("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"),
		fromHex("000102030405060708090a0b0c0d0e0f1011121314151617"),
		fromHex("51e3ff45a895675c4b33b46c64f4a9ace110d34df6a2ceab486372bacbd3eff6"),
	},
	{
		// from draft-irtf-cfrg-xchacha-03
		fromHex("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"),
		fromHex("000000090000004a0000000031415927"),
		fromHex("82413b4227b27bfed30e42508a877d73a0f9e4d58a74a853c12ec41326d3ecdc"),
	},
}
