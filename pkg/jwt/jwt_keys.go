package jwt

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha512"
	"crypto/subtle"
	"fmt"
	"hash"
	"time"

	"markdown.ninja/pkg/kms"
	"markdown.ninja/stdx/uuid"
)

// A JWT key as stored in DB
type jwtKey struct {
	ID                 uuid.UUID `db:"id"`
	CreatedAt          time.Time `db:"created_at"`
	UpdatedAt          time.Time `db:"updated_at"`
	Algorithm          Algorithm `db:"algorithm"`
	EncryptedSecretKey []byte    `db:"encrypted_secret_key"`
}

type newSigningKey struct {
	jwtKey  jwtKey
	hmacKey HmacSha512Key
}

type HmacSha512Key struct {
	algorithm Algorithm
	id        string
	secret    [32]byte
}

func (key *HmacSha512Key) Sign(message []byte) []byte {
	var signature [64]byte

	hmac := hmac.New(func() hash.Hash { return sha512.New() }, key.secret[:])
	hmac.Write(message)
	return hmac.Sum(signature[:0])
}

func (key *HmacSha512Key) Verify(message, signature []byte) bool {
	var hmacHash [64]byte

	hmac := hmac.New(func() hash.Hash { return sha512.New() }, key.secret[:])
	hmac.Write(message)
	hmac.Sum(hmacHash[:0])

	return subtle.ConstantTimeCompare(hmacHash[:], signature) == 1
}

func generateAndSaveNewJwtKey(ctx context.Context, store KeyStore, kms *kms.Kms) (newSigningKey, error) {
	keyId := uuid.NewV7()

	var secret [32]byte
	rand.Read(secret[:])

	encryptedSecret, err := kms.Encrypt(ctx, secret[:], keyId.Bytes())
	if err != nil {
		return newSigningKey{}, fmt.Errorf("error encrypting secret key: %w", err)
	}

	now := time.Now().UTC()
	jwtKey := jwtKey{
		ID:                 keyId,
		CreatedAt:          now,
		UpdatedAt:          now,
		Algorithm:          AlgorithmHS512,
		EncryptedSecretKey: encryptedSecret,
	}
	err = store.SetKey(ctx, jwtKey)
	if err != nil {
		return newSigningKey{}, fmt.Errorf("error saving new key: %w", err)
	}

	ret := newSigningKey{
		jwtKey: jwtKey,
		hmacKey: HmacSha512Key{
			algorithm: AlgorithmHS512,
			id:        jwtKey.ID.String(),
			secret:    secret,
		},
	}
	return ret, nil
}

func decryptJwtSecretKey(ctx context.Context, kms *kms.Kms, jwtKey jwtKey) (HmacSha512Key, error) {
	var err error
	ret := HmacSha512Key{
		algorithm: AlgorithmHS512,
		id:        jwtKey.ID.String(),
		secret:    [32]byte{},
	}

	secret, err := kms.Decrypt(ctx, jwtKey.EncryptedSecretKey, jwtKey.ID.Bytes())
	if err != nil {
		return ret, fmt.Errorf("error decrypting JWT secret key: %w", err)
	}

	copy(ret.secret[:], secret)

	return ret, nil
}

// type VerifyingKey interface {
// 	Verify(message, signature []byte) bool
// 	Algorithm() Algorithm
// 	ID() string
// }

// type Key interface {
// 	VerifyingKey
// 	Sign(message []byte) (signature []byte)
// }

// type HamcSha256Key struct {
// 	secret []byte
// }

// func (key *HamcSha256Key) Sign(message []byte) []byte {
// 	var signature [32]byte

// 	hmac := hmac.New(func() hash.Hash { return sha256.New() }, key.secret)
// 	hmac.Write(message)
// 	return hmac.Sum(signature[:0])
// }

// func (key *HamcSha256Key) Verify(message, signature []byte) bool {
// 	var hmacHash [32]byte

// 	hmac := hmac.New(func() hash.Hash { return sha256.New() }, key.secret)
// 	hmac.Write(message)
// 	hmac.Sum(hmacHash[:0])

// 	return subtle.ConstantTimeCompare(hmacHash[:], signature) == 1
// }

// type Ed25519VerifyingKey struct {
// 	keyID     string
// 	publicKey ed25519.PublicKey
// }

// func NewEd25519VerifyingKey(keyID string, publicKey []byte) (Ed25519VerifyingKey, error) {
// 	if len(publicKey) != ed25519.PublicKeySize {
// 		return Ed25519VerifyingKey{}, errors.New("invalid ed25519 public key size")
// 	}

// 	return Ed25519VerifyingKey{keyID: keyID, publicKey: ed25519.PublicKey(publicKey)}, nil
// }

// func (key Ed25519VerifyingKey) Algorithm() Algorithm {
// 	return AlgorithmEdDsa
// }

// func (key Ed25519VerifyingKey) ID() string {
// 	return key.keyID
// }

// func (key Ed25519VerifyingKey) Verify(message, signature []byte) bool {
// 	return ed25519.Verify(key.publicKey, message, signature)
// }
