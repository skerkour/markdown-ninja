package zign

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"markdown.ninja/stdx/crypto"
)

type SignInput struct {
	Filename string
	Reader   io.Reader
}

type SignOutput struct {
	Filename   string `json:"file"`
	HashSha256 string `json:"hash_sha256"`
	Signature  []byte `json:"signature"`
}

func Sign(encryptedBase64PrivateKey string, password string, input SignInput) (output SignOutput, err error) {
	res, err := SignMany(encryptedBase64PrivateKey, password, []SignInput{input})
	if err != nil {
		return
	}
	output = res[0]
	return
}

func SignMany(encryptedBase64PrivateKey string, password string, input []SignInput) (output []SignOutput, err error) {
	output = make([]SignOutput, len(input))

	privateKeyAndSalt, err := base64.StdEncoding.DecodeString(encryptedBase64PrivateKey)
	if err != nil {
		err = fmt.Errorf("zign.Sign: decoding encrypted private key: %w", err)
		return
	}

	privateKeyAndSaltLen := len(privateKeyAndSalt)
	if privateKeyAndSaltLen < SaltSize+crypto.Ed25519PrivateKeySize {
		err = errors.New("zign.Sign: private key is not valid")
		return
	}

	encryptedPrivateKey := privateKeyAndSalt[:len(privateKeyAndSalt)-SaltSize]
	salt := privateKeyAndSalt[len(encryptedPrivateKey):]

	encryptionKey, err := crypto.DeriveKeyFromPassword([]byte(password), salt, crypto.DefaultDeriveKeyFromPasswordParams)
	if err != nil {
		err = fmt.Errorf("zign.Sign: deriving encryption key from password: %w", err)
		return
	}

	privateKeyBytes, err := crypto.Decrypt(encryptionKey, encryptedPrivateKey, salt)
	if err != nil {
		err = fmt.Errorf("zign.Sign: decrypting private key: %w", err)
		return
	}
	defer crypto.Zeroize(privateKeyBytes)

	privateKey, err := crypto.NewEd25519PrivateKeyFromBytes(privateKeyBytes)
	if err != nil {
		err = fmt.Errorf("zign.Sign: parsing private key: %w", err)
		return
	}
	defer crypto.Zeroize(privateKey)

	for index, file := range input {
		var hash []byte
		var signature []byte

		hash, signature, err = hashAndSignFile(privateKey, file.Reader)
		if err != nil {
			return
		}

		output[index] = SignOutput{
			Filename:   file.Filename,
			HashSha256: hex.EncodeToString(hash),
			Signature:  signature,
		}
	}

	return
}

func hashAndSignFile(privateKey crypto.Ed25519PrivateKey, file io.Reader) (hash, signature []byte, err error) {
	hasher := sha256.New()
	var size int64

	size, err = io.Copy(hasher, file)
	if err != nil {
		err = fmt.Errorf("zign.Sign: hashing file %w", err)
		return
	}

	hash = hasher.Sum(nil)

	// size of an uint64 and hash
	sizeUint64 := uint64(size)
	message := bytes.NewBuffer(make([]byte, 0, 8+crypto.HashSize256))
	err = binary.Write(message, binary.BigEndian, sizeUint64)
	if err != nil {
		err = fmt.Errorf("zign.Sign: writing size: %w", err)
		return
	}

	_, err = message.Write(hash)
	if err != nil {
		err = fmt.Errorf("zign.Sign: writing hash: %w", err)
		return
	}

	signature, err = privateKey.Sign(rand.Reader, message.Bytes(), crypto.Ed25519SignerOpts)
	if err != nil {
		err = fmt.Errorf("zign.Sign: signing file: %w", err)
		return
	}

	return
}
