// Package xwing implements the hybrid post-quantum Key Encapsulation
// Mechanism (KEM) X-Wing as specified in [draft-connolly-cfrg-xwing-kem](https://www.ietf.org/archive/id/draft-connolly-cfrg-xwing-kem-07.html)
// which combines X25519 and ML-KEM-768.
package xwing

import (
	"bytes"
	"crypto/ecdh"
	"crypto/mlkem"
	"crypto/rand"
	"crypto/sha3"
	"errors"
)

const (
	CiphertextSize       = mlkem.CiphertextSize768 + 32
	EncapsulationKeySize = mlkem.EncapsulationKeySize768 + 32
	SharedKeySize        = 32
	SeedSize             = 32
)

// X-Wing decapsulationKey (public) key
type DecapsulationKey struct {
	seed            [SeedSize]byte
	secretKeyMlkem  *mlkem.DecapsulationKey768
	secretKeyX25519 *ecdh.PrivateKey
	// the encapsulation (public) key
	encapKey EncapsulationKey
}

// X-Wing encapsulation (public) key
type EncapsulationKey struct {
	mlkemEncapsulationKey *mlkem.EncapsulationKey768
	publicKeyX25519       [32]byte
}

func GenerateDecapsulationKey() *DecapsulationKey {
	var seed [SeedSize]byte
	_, _ = rand.Read(seed[:])
	return NewDecapsulationKeyFromSeed(seed)
}

func NewDecapsulationKeyFromSeed(seed [32]byte) *DecapsulationKey {
	expanded := make([]byte, mlkem.SeedSize+32)
	shake256Xof := sha3.NewSHAKE256()
	shake256Xof.Write(seed[:])
	if _, err := shake256Xof.Read(expanded); err != nil {
		panic(err) // Should never happen
	}

	secretKeyMlkem, err := mlkem.NewDecapsulationKey768(expanded[:mlkem.SeedSize])
	if err != nil {
		panic(err) // Should never happen
	}
	publicKeyMlkem := secretKeyMlkem.EncapsulationKey()

	secretKeyX25519, err := ecdh.X25519().NewPrivateKey(expanded[mlkem.SeedSize:])
	if err != nil {
		panic(err) // Should never happen
	}
	publicKeyX25519 := secretKeyX25519.PublicKey().Bytes()

	decapKey := &DecapsulationKey{}
	copy(decapKey.seed[:], seed[:])
	decapKey.secretKeyMlkem = secretKeyMlkem
	decapKey.secretKeyX25519 = secretKeyX25519
	decapKey.encapKey.mlkemEncapsulationKey = publicKeyMlkem
	copy(decapKey.encapKey.publicKeyX25519[:], publicKeyX25519)

	return decapKey
}

func (decapKey *DecapsulationKey) Bytes() []byte {
	return bytes.Clone(decapKey.seed[:])
}

func (decapKey *DecapsulationKey) EncapsulationKey() *EncapsulationKey {
	return &decapKey.encapKey
}

func (decapKey *DecapsulationKey) Decapsulate(ciphertext []byte) ([SharedKeySize]byte, error) {
	if len(ciphertext) != CiphertextSize {
		return [SharedKeySize]byte{}, errors.New("xwing: invalid ciphertext length")
	}

	ciphertextMlkem := ciphertext[:mlkem.CiphertextSize768]
	ciphertextX25519 := ciphertext[mlkem.CiphertextSize768:]
	publicKeyX25519 := decapKey.encapKey.publicKeyX25519[:]

	sharedSecretMlkem, err := decapKey.secretKeyMlkem.Decapsulate(ciphertextMlkem)
	if err != nil {
		return [SharedKeySize]byte{}, err
	}

	peerKey, err := ecdh.X25519().NewPublicKey(ciphertextX25519)
	if err != nil {
		return [SharedKeySize]byte{}, err
	}
	sharedSecretX25519, err := decapKey.secretKeyX25519.ECDH(peerKey)
	if err != nil {
		return [SharedKeySize]byte{}, err
	}

	sharedSecret := combiner(sharedSecretMlkem, sharedSecretX25519, ciphertextX25519, publicKeyX25519)
	return sharedSecret, nil
}

func NewEncapsulationKeyFromBytes(encapKeyBytes []byte) (*EncapsulationKey, error) {
	if len(encapKeyBytes) != EncapsulationKeySize {
		return nil, errors.New("xwing: invalid encapsulation key size")
	}

	publicKeyMlkem := encapKeyBytes[:mlkem.EncapsulationKeySize768]
	publicKeyX25519 := encapKeyBytes[mlkem.EncapsulationKeySize768:]

	mlkemEncapsulationKey, err := mlkem.NewEncapsulationKey768(publicKeyMlkem)
	if err != nil {
		return nil, err
	}

	encapKey := &EncapsulationKey{
		mlkemEncapsulationKey: mlkemEncapsulationKey,
	}
	copy(encapKey.publicKeyX25519[:], publicKeyX25519)

	return encapKey, nil
}

func (encapKey *EncapsulationKey) Encapsulate() (sharedSecret [SharedKeySize]byte, ciphertext []byte, err error) {
	publicKeyX25519 := encapKey.publicKeyX25519[:]

	x25519EphemeralKey, err := ecdh.X25519().GenerateKey(nil)
	if err != nil {
		return [SharedKeySize]byte{}, nil, err
	}
	x25519PeerKey, err := ecdh.X25519().NewPublicKey(publicKeyX25519)
	if err != nil {
		return [SharedKeySize]byte{}, nil, err
	}

	ciphertextX25519 := x25519EphemeralKey.PublicKey().Bytes()
	sharedSecretX25519, err := x25519EphemeralKey.ECDH(x25519PeerKey)
	if err != nil {
		return [SharedKeySize]byte{}, nil, err
	}

	sharedSecretMlkem, ciphertextMlkem := encapKey.mlkemEncapsulationKey.Encapsulate()

	sharedSecret = combiner(sharedSecretMlkem, sharedSecretX25519, ciphertextX25519, publicKeyX25519)

	ciphertext = make([]byte, CiphertextSize)
	copy(ciphertext[:mlkem.CiphertextSize768], ciphertextMlkem)
	copy(ciphertext[mlkem.CiphertextSize768:], ciphertextX25519)

	return sharedSecret, ciphertext, nil
}

func (encapKey *EncapsulationKey) Bytes() []byte {
	return append(encapKey.mlkemEncapsulationKey.Bytes(), encapKey.publicKeyX25519[:]...)
}

var xwingLabel = []byte("\\.//^\\")

func combiner(sharedSecretMlkem, sharedSecretX25519, x25519Ciphertext, x25519PublicKey []byte) [SharedKeySize]byte {
	var sharedKey [SharedKeySize]byte

	hasher := sha3.New256()
	hasher.Write(sharedSecretMlkem)
	hasher.Write(sharedSecretX25519)
	hasher.Write(x25519Ciphertext)
	hasher.Write(x25519PublicKey)
	hasher.Write(xwingLabel)
	hasher.Sum(sharedKey[:0])

	return sharedKey
}
