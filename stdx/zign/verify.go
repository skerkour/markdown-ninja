package zign

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"markdown.ninja/stdx/crypto"
)

type VerifyInput struct {
	Reader     io.Reader
	HashSha256 []byte
	Signature  []byte
}

func Verify(base64PublicKey string, input VerifyInput) (err error) {
	err = VerifyMany(base64PublicKey, []VerifyInput{input})
	if err != nil {
		return
	}

	return
}

func VerifyMany(base64PublicKey string, input []VerifyInput) (err error) {
	publicKeyBytes, err := base64.StdEncoding.DecodeString(base64PublicKey)
	if err != nil {
		err = fmt.Errorf("zign.Verify: decoding public key (%s): %w", base64PublicKey, err)
		return
	}

	publicKey, err := crypto.NewEd25519PublicKeyFromBytes(publicKeyBytes)
	if err != nil {
		err = fmt.Errorf("zign.Verify: parsing public key (%s): %w", base64PublicKey, err)
		return
	}

	for _, file := range input {
		err = hashDataAndVerifySignature(publicKey, file)
		if err != nil {
			return
		}
	}

	return
}

func hashDataAndVerifySignature(publicKey crypto.Ed25519PublicKey, file VerifyInput) (err error) {
	hasher := sha256.New()
	var size int64

	size, err = io.Copy(hasher, file.Reader)
	if err != nil {
		err = fmt.Errorf("zign.Verify: hashing file: %w", err)
		return
	}

	hash := hasher.Sum(nil)

	if !crypto.ConstantTimeCompare(hash, file.HashSha256) {
		err = errors.New("zign.Verify: hash is not valid")
		return
	}

	// size of an uint64 and hash
	sizeUint64 := uint64(size)
	message := bytes.NewBuffer(make([]byte, 0, 8+crypto.HashSize256))
	err = binary.Write(message, binary.BigEndian, sizeUint64)
	if err != nil {
		err = fmt.Errorf("zign.Verify: writing size: %w", err)
		return
	}

	_, err = message.Write(hash)
	if err != nil {
		err = fmt.Errorf("zign.Verify: writing hash: %w", err)
		return
	}

	verified := false
	verified, err = publicKey.Verify(message.Bytes(), file.Signature)
	if err != nil {
		err = fmt.Errorf("zign.Verify: verifying signature (%s): %w", base64.StdEncoding.EncodeToString(file.Signature), err)
		return
	}
	if verified {
		return
	}

	err = errors.New("zign.Verify: signature is not valid")

	return
}
