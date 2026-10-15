package chacha20_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"markdown.ninja/stdx/crypto/chacha20"
)

func toHex(bits []byte) string {
	return hex.EncodeToString(bits)
}

func fromHex(bits string) []byte {
	b, err := hex.DecodeString(bits)
	if err != nil {
		panic(err)
	}
	return b
}

func TestChaCha20(t *testing.T) {
	for i, v := range chacha20Vectors {
		if len(v.plaintext) == 0 {
			v.plaintext = make([]byte, len(v.ciphertext))
		}

		dst := make([]byte, len(v.ciphertext))

		// XORKeyStream(dst, v.plaintext, v.nonce, v.key)
		// if !bytes.Equal(dst, v.ciphertext) {
		// 	t.Errorf("Test %d: ciphertext mismatch:\n \t got:  %s\n \t want: %s", i, toHex(dst), toHex(v.ciphertext))
		// }

		c, err := chacha20.New(v.key, v.nonce)
		if err != nil {
			t.Errorf("plaintext: %s", toHex(v.plaintext))
			t.Errorf("nonce: %s", toHex(v.nonce))
			t.Fatal(err)
		}

		c.XORKeyStream(dst[:1], v.plaintext[:1])
		c.XORKeyStream(dst[1:], v.plaintext[1:])
		if !bytes.Equal(dst, v.ciphertext) {
			t.Errorf("Test %d: ciphertext mismatch:\n \t got:  %s\n \t want: %s", i, toHex(dst), toHex(v.ciphertext))
		}
	}
}

func TestXChaCha20(t *testing.T) {
	for i, v := range xchacha20Vectors {
		if len(v.plaintext) == 0 {
			v.plaintext = make([]byte, len(v.ciphertext))
		}

		dst := make([]byte, len(v.ciphertext))

		// XORKeyStream(dst, v.plaintext, v.nonce, v.key)
		// if !bytes.Equal(dst, v.ciphertext) {
		// 	t.Errorf("Test %d: ciphertext mismatch:\n \t got:  %s\n \t want: %s", i, toHex(dst), toHex(v.ciphertext))
		// }

		c, err := chacha20.NewX(v.key, v.nonce)
		if err != nil {
			t.Errorf("plaintext: %s", toHex(v.plaintext))
			t.Errorf("nonce: %s", toHex(v.nonce))
			t.Fatal(err)
		}

		c.XORKeyStream(dst[:1], v.plaintext[:1])
		c.XORKeyStream(dst[1:], v.plaintext[1:])
		if !bytes.Equal(dst, v.ciphertext) {
			t.Errorf("Test %d: ciphertext mismatch:\n \t got:  %s\n \t want: %s", i, toHex(dst), toHex(v.ciphertext))
		}
	}
}

var chacha20Vectors = []struct {
	key, nonce, plaintext, ciphertext []byte
}{
	{
		fromHex("0000000000000000000000000000000000000000000000000000000000000000"),
		fromHex("0000000000000000"),
		nil,
		fromHex("76b8e0ada0f13d90405d6ae55386bd28bdd219b8a08ded1aa836efcc8b770dc7da41597c5157488d7724e03fb8d84a376a43b8f41518a11cc387b669b2ee6586"),
	},
	// we only support 64-bit nonces
	// {
	// 	fromHex("0000000000000000000000000000000000000000000000000000000000000000"),
	// 	fromHex("000000000000000000000000"),
	// 	nil,
	// 	fromHex("76b8e0ada0f13d90405d6ae55386bd28bdd219b8a08ded1aa836efcc8b770dc7da41597c5157488d7724e03fb8d84a376a43b8f41518a11cc387b669b2ee6586"),
	// },
	// {
	// 	fromHex("0000000000000000000000000000000000000000000000000000000000000000"),
	// 	fromHex("000000000000000000000000000000000000000000000000"),
	// 	nil,
	// 	fromHex("bcd02a18bf3f01d19292de30a7a8fdaca4b65e50a6002cc72cd6d2f7c91ac3d5728f83e0aad2bfcf9abd2d2db58faedd65015dd83fc09b131e271043019e8e0f"),
	// },
}

var xchacha20Vectors = []struct {
	key, nonce, plaintext, ciphertext []byte
}{
	{
		fromHex("0000000000000000000000000000000000000000000000000000000000000000"),
		fromHex("000000000000000000000000000000000000000000000000"),
		nil,
		fromHex("bcd02a18bf3f01d19292de30a7a8fdaca4b65e50a6002cc72cd6d2f7c91ac3d5728f83e0aad2bfcf9abd2d2db58faedd65015dd83fc09b131e271043019e8e0f"),
	},
}
