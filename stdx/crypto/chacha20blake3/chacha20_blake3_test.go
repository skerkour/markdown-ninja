package chacha20blake3_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"markdown.ninja/stdx/crypto/chacha20blake3"
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

func TestVectorsChaCha20Blake3(t *testing.T) {
	for i, v := range chacha20Blake3Vectors {
		dst := make([]byte, len(v.plaintext)+chacha20blake3.TagSize)

		cipher, err := chacha20blake3.New(v.key)
		if err != nil {
			t.Errorf("plaintext: %s", toHex(v.plaintext))
			t.Errorf("nonce: %s", toHex(v.nonce))
			t.Fatal(err)
		}

		cipher.Seal(dst[:0], v.nonce, v.plaintext, v.additionalData)
		if !bytes.Equal(dst, v.ciphertext) {
			t.Errorf("Test %d: ciphertext mismatch:\ngot:  %s\nwant: %s", i, toHex(dst), toHex(v.ciphertext))
		}

		decryptedPlaintext, err := cipher.Open(nil, v.nonce, dst, v.additionalData)
		if err != nil {
			t.Errorf("Test %d: %v", i, err)
		}
		if !bytes.Equal(decryptedPlaintext, v.plaintext) {
			t.Errorf("Test %d: plaintext mismatch:\ngot:  %s\nwant: %s", i, toHex(decryptedPlaintext), toHex(v.plaintext))
		}
	}
}

// func TestBasicX(t *testing.T) {
// 	var key [chacha20blake3.KeySize]byte
// 	var nonce [chacha20blake3.NonceSizeX]byte

// 	originalPlaintext := []byte("Hello World")
// 	additionalData := []byte("!")

// 	rand.Read(key[:])
// 	rand.Read(nonce[:])

// 	cipher, _ := chacha20blake3.NewX(key[:])
// 	ciphertext := cipher.Seal(nil, nonce[:], originalPlaintext, additionalData)

// 	decryptedPlaintext, err := cipher.Open(nil, nonce[:], ciphertext, additionalData)
// 	if err != nil {
// 		t.Errorf("decrypting message: %s", err)
// 		return
// 	}

// 	if !bytes.Equal(decryptedPlaintext, originalPlaintext) {
// 		t.Errorf("original message (%s) != decrypted message (%s)", string(originalPlaintext), string(decryptedPlaintext))
// 		return
// 	}
// }

// func TestAdditionalDataX(t *testing.T) {
// 	var key [chacha20blake3.KeySize]byte
// 	var nonce [chacha20blake3.NonceSizeX]byte

// 	originalPlaintext := []byte("Hello World")
// 	additionalData := []byte("!")

// 	rand.Read(key[:])
// 	rand.Read(nonce[:])

// 	cipher, _ := chacha20blake3.NewX(key[:])
// 	ciphertext := cipher.Seal(nil, nonce[:], originalPlaintext, additionalData)

// 	_, err := cipher.Open(nil, nonce[:], ciphertext, []byte{})
// 	if !errors.Is(err, chacha20blake3.ErrOpen) {
// 		t.Errorf("expected error (%s) | got (%s)", chacha20blake3.ErrOpen, err)
// 		return
// 	}
// }

var chacha20Blake3Vectors = []struct {
	key            []byte
	nonce          []byte
	plaintext      []byte
	additionalData []byte
	ciphertext     []byte
}{
	{
		fromHex("0000000000000000000000000000000000000000000000000000000000000000"),
		fromHex("0000000000000000000000000000000000000000000000000000000000000000"),
		[]byte{},
		nil,
		fromHex("e074bcc1f324f0139dea37f8465aa7edf565f968aeae9bfa348c9a9c1c702ad2"),
	},
	{
		fromHex("0100000000000000000000000000000000000000000000000000000000000010"),
		fromHex("1000000000000000000000000000000000000000000000000000000000000001"),
		fromHex("4368614368613230"), // ChaCha20
		fromHex("424c414b4533"),     // BLAKE3
		fromHex("af4d5f3ac75f3753a764e5af1d3396f9f6f5b5ea94889665372f39a9ae7aa55aa3d77b69680bfe45"),
	},
	{
		fromHex("3eb02a239a2a66de159b9bb5486ccc10a6f63ddf5862ef076650513372353622"),
		fromHex("719d34360dcf03dc7af6a4d1d9fd311b035cbc148241f1419f166537a5552aec"),
		fromHex("b8f60975cd7057a003ac84df00d514624fe40cb7855c50dd6594f59b3a2580e5"),
		fromHex("c8d69ca92da6c5fd22f1805179fcd36cb7a9d45848fa346ba7118c2f34d23a48"),
		fromHex("42c948ee385574606ce91ed09f4ef744c69b7101ef682aee8acdd14fd827499eefe6f43193680e3685f3fe0a702a0c19ceb8d7b539a02edec99783fdb9816eb5"),
	},
}
