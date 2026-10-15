package crypto

import (
	"testing"

	"golang.org/x/crypto/curve25519"
	"markdown.ninja/stdx/crypto/chacha20blake3"
)

func TestCurve25519EncryptDecrypt(t *testing.T) {
	message := []byte("this is a simple message")
	nonce := RandBytes(chacha20blake3.NonceSize)

	toPublicKey, toPrivateKey, err := GenerateCurve25519KeyPair()
	if err != nil {
		t.Error(err)
	}

	fromPublicKey, fromPrivateKey, err := GenerateCurve25519KeyPair()
	if err != nil {
		t.Error(err)
	}

	ciphertext, err := toPublicKey.Encrypt(fromPrivateKey, nonce, message)
	if err != nil {
		t.Error(err)
	}

	plaintext, err := toPrivateKey.Decrypt(fromPublicKey, nonce, ciphertext)
	if err != nil {
		t.Error(err)
	}

	if !ConstantTimeCompare(message, plaintext) {
		t.Errorf("Message (%s) and plaintext (%s) don't match", string(message), string(plaintext))
	}
}

func TestCurve25519EncryptDecryptEphemeral(t *testing.T) {
	message := []byte("this is a simple message")

	toPublicKey, toPrivateKey, err := GenerateCurve25519KeyPair()
	if err != nil {
		t.Error(err)
	}

	ciphertext, ephemeralPublicKey, err := toPublicKey.EncryptEphemeral(message)
	if err != nil {
		t.Error(err)
	}

	plaintext, err := toPrivateKey.DecryptEphemeral(ephemeralPublicKey, ciphertext)
	if err != nil {
		t.Error(err)
	}

	if !ConstantTimeCompare(message, plaintext) {
		t.Errorf("Message (%s) and plaintext (%s) don't match", string(message), string(plaintext))
	}
}

func TestCurve25519KeyExchanges(t *testing.T) {
	publicKey1, privateKey1, err := GenerateCurve25519KeyPair()
	if err != nil {
		t.Error(err)
	}

	publicKey2, privateKey2, err := GenerateCurve25519KeyPair()
	if err != nil {
		t.Error(err)
	}

	sharedSecret1, err := privateKey1.KeyExchange(publicKey2)
	if err != nil {
		t.Error(err)
	}

	sharedSecret2, err := privateKey2.KeyExchange(publicKey1)
	if err != nil {
		t.Error(err)
	}

	if !ConstantTimeCompare(sharedSecret1, sharedSecret2) {
		t.Error("SharedSecret1 != SharedSecret2")
	}
}

func TestCurve25519KeyExchange(t *testing.T) {
	publicKey1, privateKey1, err := GenerateCurve25519KeyPair()
	if err != nil {
		t.Error(err)
	}

	publicKey2, privateKey2, err := GenerateCurve25519KeyPair()
	if err != nil {
		t.Error(err)
	}

	sharedSecret1, err := privateKey1.KeyExchange(publicKey2)
	if err != nil {
		t.Error(err)
	}

	sharedSecret2, err := curve25519.X25519(privateKey1, publicKey2)
	if err != nil {
		t.Error(err)
	}

	if !ConstantTimeCompare(sharedSecret1, sharedSecret2) {
		t.Error("SharedSecret1 != SharedSecret2")
	}

	sharedSecret3, err := curve25519.X25519(privateKey2, publicKey1)
	if err != nil {
		t.Error(err)
	}

	if !ConstantTimeCompare(sharedSecret1, sharedSecret3) {
		t.Error("SharedSecret1 != SharedSecret3")
	}
}

func TestCurve25519PrivateKeyPublic(t *testing.T) {
	publicKey, privateKey, err := GenerateCurve25519KeyPair()
	if err != nil {
		t.Error(err)
	}

	publicKey2, err := privateKey.Public()
	if err != nil {
		t.Error(err)
	}

	if !ConstantTimeCompare(publicKey, publicKey2) {
		t.Error("publicKey != publicKey2")
	}
}

func TestCurve25519GenerateNonce(t *testing.T) {
	publicKey, _, err := GenerateCurve25519KeyPair()
	if err != nil {
		t.Error(err)
	}

	ephemeralPublicKey, _, err := GenerateCurve25519KeyPair()
	if err != nil {
		t.Error(err)
	}

	nonce, err := generateNonce(ephemeralPublicKey, publicKey)
	if err != nil {
		t.Error(err)
	}

	nonce2, err := generateNonce(ephemeralPublicKey, publicKey)
	if err != nil {
		t.Error(err)
	}

	if !ConstantTimeCompare(nonce, nonce2) {
		t.Error("nonce != nonce2")
	}
}
