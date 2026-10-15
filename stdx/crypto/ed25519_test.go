package crypto

import (
	"testing"
)

func TestGenerateEd25519Keypair(t *testing.T) {
	zeroPublicKey := make([]byte, Ed25519PublicKeySize)
	zeroPrivateKey := make([]byte, Ed25519PrivateKeySize)

	publicKey, privateKey, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Error(err)
	}

	if ConstantTimeCompare(zeroPrivateKey, privateKey) {
		t.Error("Generated private key is empty")
	}

	if ConstantTimeCompare(zeroPublicKey, publicKey) {
		t.Error("Generated public key is empty")
	}
}

func TestEd25519KeypairToCurve25519(t *testing.T) {
	ed25519PublicKey, ed25519PrivateKey, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Error(err)
	}

	curve25519PrivateKey := ed25519PrivateKey.ToCurve25519PrivateKey()

	curve25519PublicKey, err := curve25519PrivateKey.Public()
	if err != nil {
		t.Error(err)
	}

	curve25519PublicKey2 := ed25519PublicKey.ToCurve25519PublicKey()

	if !ConstantTimeCompare(curve25519PublicKey, curve25519PublicKey2) {
		t.Errorf("curve25519PublicKey (%x) != curve25519PublicKey2 (%x)", curve25519PublicKey, curve25519PublicKey2)
	}
}
