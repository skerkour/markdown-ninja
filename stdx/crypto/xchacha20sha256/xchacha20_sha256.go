package xchacha20sha256

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"hash"

	"markdown.ninja/stdx/crypto/chacha20"
)

const (
	KeySize   = 32
	NonceSize = 24
	TagSize   = 32

	encryptionKeyContext    = "xchacha2-sha256 2023-12-31 23:59:59:999 encryption-key"
	athenticationKeyContext = "xchacha2-sha256 2024-01-01 00:00:00:000 authentication-key"
)

var (
	ErrOpen = errors.New("xchacha20blake3: error decrypting ciphertext")
)

type XChaCha20Sha256 struct {
	key [KeySize]byte
}

func New(key []byte) (*XChaCha20Sha256, error) {
	ret := new(XChaCha20Sha256)
	copy(ret.key[:], key)
	return ret, nil
}

func (*XChaCha20Sha256) NonceSize() int {
	return NonceSize
}

func (*XChaCha20Sha256) Overhead() int {
	return TagSize
}

func (x *XChaCha20Sha256) Seal(dst, nonce, plaintext, additionalData []byte) []byte {
	ret, out := sliceForAppend(dst, len(plaintext)+TagSize)
	ciphertext, tag := out[:len(plaintext)], out[len(plaintext):]

	var authenticationKey [32]byte
	xchacha20Cipher, _ := chacha20.NewX(x.key[:], nonce)
	xchacha20Cipher.XORKeyStream(authenticationKey[:], authenticationKey[:])
	xchacha20Cipher.SetCounter(1)
	xchacha20Cipher.XORKeyStream(ciphertext, plaintext)

	macHasher := hmac.New(sha256.New, authenticationKey[:])
	macHasher.Write(additionalData)
	macHasher.Write(ciphertext)
	writeUint64(macHasher, len(additionalData))
	writeUint64(macHasher, len(ciphertext))
	macHasher.Sum(tag[:0])

	return ret
}

func (x *XChaCha20Sha256) Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error) {
	tag := ciphertext[len(ciphertext)-TagSize:]
	ciphertext = ciphertext[:len(ciphertext)-TagSize]

	var authenticationKey [32]byte
	xchacha20Cipher, _ := chacha20.NewX(x.key[:], nonce)
	xchacha20Cipher.XORKeyStream(authenticationKey[:], authenticationKey[:])
	xchacha20Cipher.SetCounter(1)

	var computedTag [TagSize]byte
	macHasher := hmac.New(sha256.New, authenticationKey[:])
	macHasher.Write(additionalData)
	macHasher.Write(ciphertext)
	writeUint64(macHasher, len(additionalData))
	writeUint64(macHasher, len(ciphertext))
	macHasher.Sum(computedTag[:0])

	ret, plaintext := sliceForAppend(dst, len(ciphertext))

	if subtle.ConstantTimeCompare(computedTag[:], tag) != 1 {
		for i := range plaintext {
			plaintext[i] = 0
		}
		return nil, ErrOpen
	}

	xchacha20Cipher.XORKeyStream(plaintext, ciphertext)

	return ret, nil
}

// func deriveKey(parentKey []byte, context string) (subKey [32]byte) {
// 	blake3.DeriveKey(subKey[:], context, parentKey)
// 	return subKey
// 	// hasher.Write(parentKey)
// 	// // hasher.Write(binary.LittleEndian.AppendUint64([]byte{}, uint64(len(nonce))))
// 	// // hasher.Write(binary.LittleEndian.AppendUint64([]byte{}, uint64(len(parentKey))))
// 	// return hasher.Sum(out)
// }

// sliceForAppend takes a slice and a requested number of bytes. It returns a
// slice with the contents of the given slice followed by that many bytes and a
// second slice that aliases into it and contains only the extra bytes. If the
// original slice has sufficient capacity then no allocation is performed.
func sliceForAppend(in []byte, n int) (head, tail []byte) {
	if total := len(in) + n; cap(in) >= total {
		head = in[:total]
	} else {
		head = make([]byte, total)
		copy(head, in)
	}
	tail = head[len(in):]
	return
}

func writeUint64(p hash.Hash, n int) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(n))
	p.Write(buf[:])
}
