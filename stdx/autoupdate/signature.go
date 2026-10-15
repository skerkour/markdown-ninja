package autoupdate

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"markdown.ninja/stdx/byteshex"
	"markdown.ninja/stdx/crypto"
)

type SignInput struct {
	Filename string
	Reader   io.Reader
}

func GenerateSigningKeypair(password []byte) (encryptedAndEncodedPrivateKey string, encodedPublicKey string, err error) {
	publicKey, privateKey, err := crypto.GenerateEd25519KeyPair()
	if err != nil {
		err = fmt.Errorf("autoupdate.GenerateKeypair: error generating ed25519 keypair: %w", err)
		return
	}
	defer crypto.Zeroize(privateKey)

	salt := crypto.RandBytes(SaltSize)
	encryptionKey, err := crypto.DeriveKeyFromPassword(password, salt, crypto.DefaultDeriveKeyFromPasswordParams)
	if err != nil {
		err = fmt.Errorf("autoupdate.GenerateKeypair: error deriving encryption key from password: %w", err)
		return
	}

	encryptedPrivateKey, err := crypto.Encrypt(encryptionKey, privateKey.Bytes(), salt)
	if err != nil {
		err = fmt.Errorf("autoupdate.GenerateKeypair: error encrypting private key: %w", err)
		return
	}

	encryptedPrivateKeyAndSalt := append(encryptedPrivateKey, salt...)

	encryptedAndEncodedPrivateKey = base64.StdEncoding.EncodeToString(encryptedPrivateKeyAndSalt)
	encodedPublicKey = base64.StdEncoding.EncodeToString(publicKey.Bytes())

	return
}

func Sign(encryptedBase64PrivateKey string, password string, input SignInput) (output ReleaseFile, err error) {
	res, err := SignMany(encryptedBase64PrivateKey, password, []SignInput{input})
	if err != nil {
		return
	}
	output = res[0]
	return
}

func SignMany(encryptedBase64PrivateKey string, password string, input []SignInput) (output []ReleaseFile, err error) {
	output = make([]ReleaseFile, len(input))

	privateKeyAndSalt, err := base64.StdEncoding.DecodeString(encryptedBase64PrivateKey)
	if err != nil {
		err = fmt.Errorf("autoupdate.Sign: decoding encrypted private key: %w", err)
		return
	}

	privateKeyAndSaltLen := len(privateKeyAndSalt)
	if privateKeyAndSaltLen < SaltSize+crypto.Ed25519PrivateKeySize {
		err = errors.New("autoupdate.Sign: private key is not valid")
		return
	}

	encryptedPrivateKey := privateKeyAndSalt[:len(privateKeyAndSalt)-SaltSize]
	salt := privateKeyAndSalt[len(encryptedPrivateKey):]

	encryptionKey, err := crypto.DeriveKeyFromPassword([]byte(password), salt, crypto.DefaultDeriveKeyFromPasswordParams)
	if err != nil {
		err = fmt.Errorf("autoupdate.Sign: deriving encryption key from password: %w", err)
		return
	}

	privateKeyBytes, err := crypto.Decrypt(encryptionKey, encryptedPrivateKey, salt)
	if err != nil {
		err = fmt.Errorf("autoupdate.Sign: decrypting private key: %w", err)
		return
	}
	defer crypto.Zeroize(privateKeyBytes)

	privateKey, err := crypto.NewEd25519PrivateKeyFromBytes(privateKeyBytes)
	if err != nil {
		err = fmt.Errorf("autoupdate.Sign: parsing private key: %w", err)
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

		output[index] = ReleaseFile{
			Filename:  file.Filename,
			Sha256:    byteshex.Bytes(hash),
			Signature: signature,
		}
	}

	return
}

func hashAndSignFile(privateKey crypto.Ed25519PrivateKey, file io.Reader) (hash, signature []byte, err error) {
	hasher := sha256.New()
	var size int64

	size, err = io.Copy(hasher, file)
	if err != nil {
		err = fmt.Errorf("autoupdate.Sign: hashing file %w", err)
		return
	}

	hash = hasher.Sum(nil)

	// size of an uint64 and hash
	sizeUint64 := uint64(size)
	message := bytes.NewBuffer(make([]byte, 0, 8+crypto.HashSize256))
	err = binary.Write(message, binary.BigEndian, sizeUint64)
	if err != nil {
		err = fmt.Errorf("autoupdate.Sign: writing size: %w", err)
		return
	}

	_, err = message.Write(hash)
	if err != nil {
		err = fmt.Errorf("autoupdate.Sign: writing hash: %w", err)
		return
	}

	signature, err = privateKey.Sign(rand.Reader, message.Bytes(), crypto.Ed25519SignerOpts)
	if err != nil {
		err = fmt.Errorf("autoupdate.Sign: signing file: %w", err)
		return
	}

	return
}

type VerifyInput struct {
	Reader    io.Reader
	Sha256    []byte
	Signature []byte
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
		err = fmt.Errorf("autoupdate.VerifyMany: error decoding public key (%s): %w", base64PublicKey, err)
		return
	}

	publicKey, err := crypto.NewEd25519PublicKeyFromBytes(publicKeyBytes)
	if err != nil {
		err = fmt.Errorf("autoupdate.VerifyMany: error parsing public key (%s): %w", base64PublicKey, err)
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
		err = fmt.Errorf("autoupdate.Verify: hashing file: %w", err)
		return
	}

	hash := hasher.Sum(nil)

	if !crypto.ConstantTimeCompare(hash, file.Sha256) {
		err = errors.New("autoupdate.Verify: hash is not valid")
		return
	}

	// size of an uint64 and hash
	sizeUint64 := uint64(size)
	message := bytes.NewBuffer(make([]byte, 0, 8+crypto.HashSize256))
	err = binary.Write(message, binary.BigEndian, sizeUint64)
	if err != nil {
		err = fmt.Errorf("autoupdate.Verify: writing size: %w", err)
		return
	}

	_, err = message.Write(hash)
	if err != nil {
		err = fmt.Errorf("autoupdate.Verify: writing hash: %w", err)
		return
	}

	verified := false
	verified, err = publicKey.Verify(message.Bytes(), file.Signature)
	if err != nil {
		err = fmt.Errorf("autoupdate.Verify: verifying signature (%s): %w", base64.StdEncoding.EncodeToString(file.Signature), err)
		return
	}
	if verified {
		return
	}

	err = errors.New("autoupdate.Verify: signature is not valid")

	return
}
