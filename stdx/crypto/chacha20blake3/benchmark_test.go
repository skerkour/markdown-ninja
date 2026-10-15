package chacha20blake3_test

import (
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"testing"

	"golang.org/x/crypto/chacha20"
	"golang.org/x/crypto/chacha20poly1305"
	"markdown.ninja/stdx/crypto/chacha20blake3"
	"markdown.ninja/stdx/crypto/xchacha20sha256"
)

var (
	BENCHMARKS = []int64{
		64,
		1024,
		16 * 1024,
		64 * 1024,
		1 * 1024 * 1024,
		10 * 1024 * 1024,
		// 100_000_000,
	}
)

// go test -benchmem -bench=. github.com/skerkour/stdx-go/crypto/chacha20blake3
func BenchmarkEncryptAEAD(b *testing.B) {
	associatedData := randBytes(b, 100)

	chaCha20Key := randBytes(b, chacha20.KeySize)
	xChaCha20Nonce := randBytes(b, chacha20.NonceSizeX)
	chaCha20Blake3Nonce := randBytes(b, chacha20blake3.NonceSize)

	for _, size := range BENCHMARKS {
		benchmarkEncrypt(b, size, "XChaCha20-Poly1305", newXChaCha20Poly1305Cipher(b, chaCha20Key), xChaCha20Nonce, associatedData)
		benchmarkEncrypt(b, size, "ChaCha20-BLAKE3", newChaCha20Blake3Cipher(b, chaCha20Key), chaCha20Blake3Nonce, associatedData)
		benchmarkEncrypt(b, size, "XChaCha20-SHA256", newXChaCha20Sha256Cipher(b, chaCha20Key), xChaCha20Nonce, associatedData)
	}
}

func BenchmarkDecryptAEAD(b *testing.B) {
	associatedData := randBytes(b, 100)

	chaCha20Key := randBytes(b, chacha20.KeySize)
	xChaCha20Nonce := randBytes(b, chacha20.NonceSizeX)
	chaCha20Blake3Nonce := randBytes(b, chacha20blake3.NonceSize)

	for _, size := range BENCHMARKS {
		benchmarkDecrypt(b, size, "XChaCha20-Poly1305", newXChaCha20Poly1305Cipher(b, chaCha20Key), xChaCha20Nonce, associatedData)
		benchmarkDecrypt(b, size, "ChaCha20-BLAKE3", newChaCha20Blake3Cipher(b, chaCha20Key), chaCha20Blake3Nonce, associatedData)
		benchmarkDecrypt(b, size, "XChaCha20-SHA256", newXChaCha20Sha256Cipher(b, chaCha20Key), xChaCha20Nonce, associatedData)
	}
}

func benchmarkEncrypt[C cipher.AEAD](b *testing.B, size int64, algorithm string, cipher C, nonce, associatedData []byte) {
	b.Run(fmt.Sprintf("%s-%s", bytesCount(size), algorithm), func(b *testing.B) {
		plaintext := randBytes(b, size)
		dst := make([]byte, 0, len(plaintext)+512)
		b.ReportAllocs()
		b.SetBytes(size)
		b.ResetTimer()
		for b.Loop() {
			cipher.Seal(dst, nonce, plaintext, associatedData)
		}
	})
}

func benchmarkDecrypt[C cipher.AEAD](b *testing.B, size int64, algorithm string, cipher C, nonce, associatedData []byte) {
	b.Run(fmt.Sprintf("%s-%s", bytesCount(size), algorithm), func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(size)
		plaintext := randBytes(b, size)
		cipherText := make([]byte, len(plaintext)+512)
		cipherText = cipher.Seal(cipherText, nonce, plaintext, associatedData)
		dst := make([]byte, len(cipherText))
		b.ResetTimer()
		for b.Loop() {
			cipher.Open(dst, nonce, cipherText, associatedData)
		}
	})
}

func newChaCha20Blake3Cipher(b *testing.B, key []byte) *chacha20blake3.ChaCha20Blake3 {
	cipher, err := chacha20blake3.New(key)
	if err != nil {
		b.Error(err)
	}

	return cipher
}

func newXChaCha20Sha256Cipher(b *testing.B, key []byte) *xchacha20sha256.XChaCha20Sha256 {
	cipher, err := xchacha20sha256.New(key)
	if err != nil {
		b.Error(err)
	}

	return cipher
}

func newXChaCha20Poly1305Cipher(b *testing.B, key []byte) cipher.AEAD {
	cipher, err := chacha20poly1305.NewX(key)
	if err != nil {
		b.Error(err)
	}

	return cipher
}

func randBytes(b *testing.B, n int64) []byte {
	buff := make([]byte, n)

	_, err := rand.Read(buff)
	if err != nil {
		b.Error(err)
	}

	return buff
}

func bytesCount(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.0f%ciB",
		float64(b)/float64(div), "KMGTPE"[exp])
}
