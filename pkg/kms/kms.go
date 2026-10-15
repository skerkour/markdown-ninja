package kms

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/fxamacker/cbor/v2"
	"golang.org/x/crypto/chacha20poly1305"
	"markdown.ninja/stdx/memorycache"
	"markdown.ninja/stdx/uuid"
)

type Provider string
type EncryptionAlgorithm uint8

const (
	ProviderScaleway Provider = "scaleway"
	ProviderConsole  Provider = "console"
)

type KmsService interface {
	EncryptDataKey(ctx context.Context, keyId string, plaintext []byte) ([]byte, error)
	DecryptDataKey(ctx context.Context, keyId string, ciphertext []byte) ([]byte, error)
}

type Ciphertext struct {
	Version    int32        `cbor:"version"`
	Ciphertext CiphertextV1 `cbor:"ciphertext"`
	// Ciphertext cbor.RawMessage `cbor:"ciphertext"`
}

type CiphertextV1 struct {
	// the ID of the key at the external KMS service
	KmsKeyID string `cbor:"kms_key_id"`

	// the unique ID of the data key. useful for in-memory caching so we don't need to call the
	// external KMS for every decryption. Note that data_key_id is unique.
	DataKeyID        uuid.UUID `cbor:"data_key_id"`
	EncryptedDataKey []byte    `cbor:"encrypted_data_key"`
	Nonce            []byte    `cbor:"nonce"`
	EncryptedData    []byte    `cbor:"encrypted_data"`
}

type Kms struct {
	kms         KmsService
	masterKeyID string
	// in-memory cache used to store data keys to avoid too many calls to the KMS service
	dataKeysCache *memorycache.Cache[string, []byte]
}

func New(kms KmsService, masterKeyID string) *Kms {
	dataKeysCache := memorycache.New(
		memorycache.WithCapacity[string, []byte](5_000),
		memorycache.WithTTL[string, []byte](1*time.Hour),
	)

	return &Kms{
		kms,
		masterKeyID,
		dataKeysCache,
	}
}

func (kms *Kms) Encrypt(ctx context.Context, data []byte, aad []byte) ([]byte, error) {
	var dataKey [chacha20poly1305.KeySize]byte
	var nonce [chacha20poly1305.NonceSizeX]byte

	rand.Read(dataKey[:])
	rand.Read(nonce[:])

	encryptedDataKey, err := kms.kms.EncryptDataKey(ctx, kms.masterKeyID, dataKey[:])
	if err != nil {
		return nil, fmt.Errorf("kms: error encrypting data key: %w", err)
	}

	// encrypt data
	dataKeyID := uuid.NewV7()

	finalAad := make([]byte, 0, len(aad)+len(encryptedDataKey)+len(kms.masterKeyID)+uuid.Size)
	finalAad = append(finalAad, aad...)
	finalAad = append(finalAad, []byte(kms.masterKeyID)...)
	finalAad = append(finalAad, dataKeyID.Bytes()...)
	finalAad = append(finalAad, encryptedDataKey...)

	cipher, err := chacha20poly1305.NewX(dataKey[:])
	if err != nil {
		return nil, fmt.Errorf("kms: error instantiating cipher: %w", err)
	}

	encryptedData := cipher.Seal(nil, nonce[:], data, finalAad)

	ciphertext := Ciphertext{
		Version: 1,
		Ciphertext: CiphertextV1{
			KmsKeyID:         kms.masterKeyID,
			DataKeyID:        dataKeyID,
			EncryptedDataKey: encryptedDataKey,
			Nonce:            nonce[:],
			EncryptedData:    encryptedData,
		},
	}

	ciphertextCbor, err := cbor.Marshal(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("kms: error encoding ciphertext to CBOR: %w", err)
	}

	kms.dataKeysCache.Set(dataKeyID.String(), dataKey[:], memorycache.DefaultTTL)

	return ciphertextCbor, nil
}

func (kms *Kms) Decrypt(ctx context.Context, ciphertextBytes []byte, aad []byte) ([]byte, error) {
	var ciphertext Ciphertext

	err := cbor.Unmarshal(ciphertextBytes, &ciphertext)
	if err != nil {
		return nil, fmt.Errorf("kms: error decoding CBOR ciphertext: %w", err)
	}

	dataKeyID := ciphertext.Ciphertext.DataKeyID
	dataKeyIDStr := dataKeyID.String()
	var dataKey []byte
	if cachedDataKey := kms.dataKeysCache.Get(dataKeyIDStr); cachedDataKey != nil {
		dataKey = cachedDataKey.Value()
	} else {
		dataKey, err = kms.kms.DecryptDataKey(ctx, kms.masterKeyID, ciphertext.Ciphertext.EncryptedDataKey)
		if err != nil {
			return nil, fmt.Errorf("kms: error decrypting data key: %w", err)
		}
		kms.dataKeysCache.Set(dataKeyIDStr, dataKey, memorycache.DefaultTTL)
	}

	// decrypt data
	finalAad := make([]byte, 0, len(aad)+len(ciphertext.Ciphertext.EncryptedDataKey)+len(kms.masterKeyID)+uuid.Size)
	finalAad = append(finalAad, aad...)
	finalAad = append(finalAad, []byte(kms.masterKeyID)...)
	finalAad = append(finalAad, dataKeyID.Bytes()...)
	finalAad = append(finalAad, ciphertext.Ciphertext.EncryptedDataKey...)

	cipher, err := chacha20poly1305.NewX(dataKey[:])
	if err != nil {
		return nil, fmt.Errorf("kms: error instantiating cipher: %w", err)
	}

	plaintext, err := cipher.Open(nil, ciphertext.Ciphertext.Nonce, ciphertext.Ciphertext.EncryptedData, finalAad)
	if err != nil {
		return nil, fmt.Errorf("kms: error decrypting data: %w", err)
	}

	return plaintext, nil
}
