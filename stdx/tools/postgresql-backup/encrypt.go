package main

import (
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
	"markdown.ninja/stdx/xwing"
)

const (
	encryptionKeySize    = 32
	nonceSize            = 24
	ciphertextHeaderSize = xwing.CiphertextSize
)

func encrypt(plaintext []byte, publicKeyBase64 string) ([]byte, error) {
	encapKeyBytes, err := base64.StdEncoding.DecodeString(publicKeyBase64)
	if err != nil {
		return nil, err
	}

	encapKey, err := xwing.NewEncapsulationKeyFromBytes(encapKeyBytes)
	if err != nil {
		return nil, err
	}

	sharedSecret, ciphertextHeader, err := encapKey.Encapsulate()
	if err != nil {
		return nil, err
	}

	encryptionKey, err := deriveEncryptionKey(sharedSecret[:])
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	cipher, err := chacha20poly1305.NewX(encryptionKey[:])
	if err != nil {
		return nil, err
	}

	ciphertext := cipher.Seal(nil, nonce, plaintext, nil)

	result := make([]byte, 0, ciphertextHeaderSize+nonceSize+len(ciphertext))
	result = append(result, ciphertextHeader...)
	result = append(result, nonce...)
	result = append(result, ciphertext...)

	return result, nil
}

func decrypt(data []byte, privateKeyBase64 string) ([]byte, error) {
	seedBytes, err := base64.StdEncoding.DecodeString(privateKeyBase64)
	if err != nil {
		return nil, err
	}

	var seed [32]byte
	copy(seed[:], seedBytes)

	decapKey := xwing.NewDecapsulationKeyFromSeed(seed)

	if len(data) <= ciphertextHeaderSize+nonceSize {
		return nil, errors.New("invalid encrypted file: too short")
	}

	ciphertextHeader := data[:ciphertextHeaderSize]
	nonce := data[ciphertextHeaderSize : ciphertextHeaderSize+nonceSize]
	ciphertext := data[ciphertextHeaderSize+nonceSize:]

	sharedSecret, err := decapKey.Decapsulate(ciphertextHeader)
	if err != nil {
		return nil, fmt.Errorf("error decapsulating X-Wing ciphertext: %w", err)
	}

	encryptionKey, err := deriveEncryptionKey(sharedSecret[:])
	if err != nil {
		return nil, err
	}

	cipher, err := chacha20poly1305.NewX(encryptionKey[:])
	if err != nil {
		return nil, err
	}

	plaintext, err := cipher.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

func deriveEncryptionKey(sharedSecret []byte) ([32]byte, error) {
	var key [32]byte

	hkdfReader := hkdf.New(sha512.New, sharedSecret, nil, []byte("postgresql-backup"))
	if _, err := io.ReadFull(hkdfReader, key[:]); err != nil {
		return [32]byte{}, fmt.Errorf("hkdf: %w", err)
	}

	return key, nil
}
