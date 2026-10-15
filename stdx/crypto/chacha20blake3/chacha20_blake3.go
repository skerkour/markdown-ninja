package chacha20blake3

import (
	"crypto/cipher"
	"crypto/subtle"
	"encoding/binary"
	"errors"

	"markdown.ninja/stdx/chacha20"
	"markdown.ninja/stdx/crypto/blake3"
)

const (
	KeySize   = 32
	NonceSize = 32
	TagSize   = 32

	errInvalidBlake3Key = "chacha20blake3: internal error: invalid BLAKE3 key"
)

var (
	ErrOpen           = errors.New("chacha20blake3: error decrypting ciphertext")
	ErrBadKeyLength   = errors.New("chacha20blake3: bad key length for ChaCha20-BLAKE3. 32 bytes required")
	ErrBadNonceLength = errors.New("chacha20blake3: bad nonce length for ChaCha20-BLAKE3. 24 bytes required")
)

type ChaCha20Blake3 struct {
	key [KeySize]byte
}

// ensure that ChaCha20Blake3 implements `cipher.AEAD` interface at build time
var _ cipher.AEAD = (*ChaCha20Blake3)(nil)

func New(key []byte) (*ChaCha20Blake3, error) {
	if len(key) != KeySize {
		return nil, ErrBadKeyLength
	}

	var ret ChaCha20Blake3
	copy(ret.key[:], key)

	return &ret, nil
}

func (*ChaCha20Blake3) NonceSize() int {
	return NonceSize
}

func (*ChaCha20Blake3) Overhead() int {
	return TagSize
}

func (cipher *ChaCha20Blake3) Seal(dst, nonce, plaintext, associatedData []byte) []byte {
	if len(nonce) != NonceSize {
		panic(ErrBadNonceLength)
	}

	var kdfOutput [72]byte
	kdf := blake3.NewKeyed(cipher.key)
	kdf.Write(nonce)
	kdf.Xof().Read(kdfOutput[:])

	chachaKey := kdfOutput[0:32]
	authenticationKey := kdfOutput[32:64]
	chachaNonce := kdfOutput[64:72]

	ret, out := sliceForAppend(dst, len(plaintext)+TagSize)
	ciphertext, tag := out[:len(plaintext)], out[len(plaintext):]

	var ietfNonce [12]byte
	copy(ietfNonce[4:], chachaNonce)

	chacha20Cipher := chacha20.NewIetf([32]byte(chachaKey), ietfNonce)
	chacha20Cipher.XORKeyStream(ciphertext, plaintext)

	macHasher := blake3.NewKeyed([32]byte(authenticationKey))
	macHasher.Write(associatedData)
	writeUint64LittleEndian(macHasher, uint64(len(associatedData)))
	macHasher.Write(ciphertext)
	writeUint64LittleEndian(macHasher, uint64(len(ciphertext)))
	macHasher.Xof().Read(tag[:])

	return ret
}

func (cipher *ChaCha20Blake3) Open(dst, nonce, ciphertext, associatedData []byte) ([]byte, error) {
	if len(nonce) != NonceSize {
		panic(ErrBadNonceLength)
	}

	tag := ciphertext[len(ciphertext)-TagSize:]
	ciphertext = ciphertext[:len(ciphertext)-TagSize]

	var kdfOutput [72]byte
	kdf := blake3.NewKeyed(cipher.key)
	kdf.Write(nonce)
	kdf.Xof().Read(kdfOutput[:])

	chachaKey := kdfOutput[0:32]
	authenticationKey := kdfOutput[32:64]
	chachaNonce := kdfOutput[64:72]

	var computedTag [TagSize]byte
	macHasher := blake3.NewKeyed([32]byte(authenticationKey[:]))
	macHasher.Write(associatedData)
	writeUint64LittleEndian(macHasher, uint64(len(associatedData)))
	macHasher.Write(ciphertext)
	writeUint64LittleEndian(macHasher, uint64(len(ciphertext)))
	macHasher.Xof().Read(computedTag[:])

	if subtle.ConstantTimeCompare(computedTag[:], tag) != 1 {
		return nil, ErrOpen
	}

	ret, plaintext := sliceForAppend(dst, len(ciphertext))

	var ietfNonce [12]byte
	copy(ietfNonce[4:], chachaNonce)

	chacha20Cipher := chacha20.NewIetf([32]byte(chachaKey), ietfNonce)
	chacha20Cipher.XORKeyStream(plaintext, ciphertext)

	return ret, nil
}

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

func writeUint64LittleEndian(p *blake3.Hasher, n uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], n)
	p.Write(buf[:])
}
