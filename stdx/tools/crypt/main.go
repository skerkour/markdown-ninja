package main

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha3"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/term"
)

const (
	keyLength = 32

	kdfInfoXChaCha20Poly1305Key = "crypt XChaCha20-Poly1305 key"
	kdfInfoAesKey               = "crypt AES-256-GCM key"

	nonceSeedLength               = 32
	aesNonceLength                = 12
	xchacha20Poly1305NonceLength  = 24
	kdfInfoXChaCha20Poly1305Nonce = "crypt XChaCha20-Poly1305 nonce"
	kdfInfoAesNonce               = "crypt AES-256-GCM nonce"

	argon2SaltLength = 32
	argon2Iterations = 8
	argon2MemoryKB   = 1024 * 1024 // 1GiB
	// paralellism
	argon2Lanes       = 4
	kdfInfoArgon2Salt = "crypt Argon2 salt"
)

func main() {
	if len(os.Args) != 4 {
		printHelpAndExit(1)
	}

	action := os.Args[1]
	fileIn := os.Args[2]
	fileOut := os.Args[3]
	var err error

	confirmPassword := true
	fn := encrypt

	switch action {
	case "encrypt":
		// do nothing
	case "decrypt":
		confirmPassword = false
		fn = decrypt
	default:
		printHelpAndExit(1)
	}

	password, err := askForPassword(confirmPassword)
	if err != nil {
		log.Fatal(err)
	}

	err = processFile(password, fileIn, fileOut, fn)
	zeroize(password)
	if err != nil {
		log.Fatal(err)
	}
}

func processFile(password []byte, fileIn, fileOut string, fn func(password, input []byte) (output []byte, err error)) error {
	if fileIn == fileOut {
		return errors.New("input file can't be the same as output file")
	}

	dataIn, err := os.ReadFile(fileIn)
	if err != nil {
		return fmt.Errorf("error reading [%s]: %w", fileIn, err)
	}

	dataOut, err := fn(password, dataIn)
	if err != nil {
		return err
	}

	err = os.WriteFile(fileOut, dataOut, 0600)
	zeroize(dataIn)
	zeroize(dataOut)
	if err != nil {
		return fmt.Errorf("error writing to [%s]: %w", fileOut, err)
	}

	return nil
}

// returns [nonceSeed (32 bytes) || ciphertext]
// xchacha20Poly1305Nonce = KDF(nonceSeed, "...", 24)
// AesNonce = KDF(nonceSeed, "...", 12)
// argon2Salt = KDF(nonceSeed, "...", 32)
func encrypt(password, plaintext []byte) (ciphertext []byte, err error) {
	var nonceSeed [nonceSeedLength]byte
	rand.Read(nonceSeed[:])

	xchacha20Poly1305Nonce := deriveKey(nonceSeed[:], kdfInfoXChaCha20Poly1305Nonce, xchacha20Poly1305NonceLength)
	aesNonce := deriveKey(nonceSeed[:], kdfInfoAesNonce, aesNonceLength)
	argon2Salt := deriveKey(nonceSeed[:], kdfInfoArgon2Salt, argon2SaltLength)

	rootKey := argon2.IDKey(password, argon2Salt, argon2Iterations, argon2MemoryKB, uint8(argon2Lanes), keyLength)

	aesKey := deriveKey(rootKey, kdfInfoAesKey, keyLength)
	xchacha20Key := deriveKey(rootKey, kdfInfoXChaCha20Poly1305Key, keyLength)
	zeroize(rootKey)

	aesCipher, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("error instantiating AES cipher: %w", err)
	}
	aes256Gcm, err := cipher.NewGCM(aesCipher)
	if err != nil {
		return nil, fmt.Errorf("error instantiating AES-256-GCM cipher: %w", err)
	}

	compressedPlaintextBuffer, err := compressGzip(plaintext)
	if err != nil {
		return nil, fmt.Errorf("error compressing data: %w", err)
	}
	compressedPlaintext := compressedPlaintextBuffer.Bytes()

	ciphertextAes := aes256Gcm.Seal(compressedPlaintext[:0], aesNonce, compressedPlaintext, nil)
	zeroize(aesKey)

	xchacha20Poly1305Cipher, err := chacha20poly1305.NewX(xchacha20Key)
	if err != nil {
		return nil, fmt.Errorf("error instantiating XChaCha20-Poly1305 cipher: %w", err)
	}

	// additional space is reserved for the AEADs' authentication tags
	ciphertext = make([]byte, 0, nonceSeedLength+len(plaintext)+100)
	ciphertext = append(ciphertext, nonceSeed[:]...)

	ciphertext = xchacha20Poly1305Cipher.Seal(ciphertext, xchacha20Poly1305Nonce, ciphertextAes, nil)
	zeroize(xchacha20Key)
	zeroize(compressedPlaintext)

	return ciphertext, nil
}

func decrypt(password, ciphertext []byte) (plaintext []byte, err error) {
	nonceSeed := ciphertext[:nonceSeedLength]
	ciphertext = ciphertext[nonceSeedLength:]

	xchacha20Poly1305Nonce := deriveKey(nonceSeed, kdfInfoXChaCha20Poly1305Nonce, xchacha20Poly1305NonceLength)
	aesNonce := deriveKey(nonceSeed, kdfInfoAesNonce, aesNonceLength)
	argon2Salt := deriveKey(nonceSeed, kdfInfoArgon2Salt, argon2SaltLength)

	rootKey := argon2.IDKey(password, argon2Salt, argon2Iterations, argon2MemoryKB, uint8(argon2Lanes), keyLength)
	zeroize(argon2Salt)

	aesKey := deriveKey(rootKey, kdfInfoAesKey, keyLength)
	chacha20Key := deriveKey(rootKey, kdfInfoXChaCha20Poly1305Key, keyLength)
	zeroize(rootKey)

	xchacha20Poly1305Cipher, err := chacha20poly1305.NewX(chacha20Key)
	if err != nil {
		return nil, fmt.Errorf("error instantiating XChaCha20-Poly1305 cipher: %w", err)
	}

	ciphertextAes, err := xchacha20Poly1305Cipher.Open(ciphertext[:0], xchacha20Poly1305Nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("error decrypting data with XChaCha20-Poly1305: %w", err)
	}
	zeroize(chacha20Key)

	aesCipher, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("error instantiating AES cipher: %w", err)
	}
	aes256Gcm, err := cipher.NewGCM(aesCipher)
	if err != nil {
		return nil, fmt.Errorf("error instantiating AES-256-GCM cipher: %w", err)
	}

	compressedPlaintext, err := aes256Gcm.Open(ciphertextAes[:0], aesNonce, ciphertextAes, nil)
	if err != nil {
		return nil, fmt.Errorf("error decrypting data with AES-256-GCM: %w", err)
	}
	zeroize(aesKey)

	plaintext, err = decompressGzip(bytes.NewReader(compressedPlaintext))
	zeroize(compressedPlaintext)
	if err != nil {
		return nil, fmt.Errorf("error decomrpessing data: %w", err)
	}

	return plaintext, nil
}

func deriveKey(rootKey []byte, info string, length int64) []byte {
	out := make([]byte, length)

	hasher := sha3.NewSHAKE256()
	hasher.Write([]byte(rootKey))
	binary.Write(hasher, binary.LittleEndian, len(rootKey))
	hasher.Write([]byte(info))
	binary.Write(hasher, binary.LittleEndian, len(info))
	binary.Write(hasher, binary.LittleEndian, length)

	hasher.Read(out)
	return out
}

func printHelpAndExit(exitCode int) {
	fmt.Println("usage: crypt <encrypt|decrypt> <in> <out>")
	os.Exit(exitCode)
}

func zeroize(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func compressGzip(input []byte) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	gzipCompressor := gzip.NewWriter(&buf)
	if _, err := gzipCompressor.Write(input); err != nil {
		gzipCompressor.Close()
		return nil, err
	}
	if err := gzipCompressor.Close(); err != nil {
		return nil, err
	}
	return &buf, nil
}

func decompressGzip(comrpessedDataReader io.Reader) ([]byte, error) {
	gzipDecompressor, err := gzip.NewReader(comrpessedDataReader)
	if err != nil {
		return nil, err
	}
	defer gzipDecompressor.Close()
	return io.ReadAll(gzipDecompressor)
}

func askForPassword(confirmPassword bool) ([]byte, error) {
	// Prompt for password (no echo)
	os.Stderr.WriteString("Password: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	os.Stderr.WriteString("\n")
	if err != nil {
		return nil, fmt.Errorf("error reading password: %w", err)
	}
	if len(password) == 0 {
		return nil, errors.New("password is empty")
	}

	if confirmPassword {
		os.Stderr.WriteString("Confirm Password: ")
		passwordConfirmation, err := term.ReadPassword(int(os.Stdin.Fd()))
		os.Stderr.WriteString("\n")
		if err != nil {
			return nil, fmt.Errorf("error reading password confirmation: %w", err)
		}
		if !bytes.Equal(password, passwordConfirmation) {
			return nil, errors.New("passwords don't match")
		}
		zeroize(passwordConfirmation)
	}

	return password, nil
}
