package crypto

import (
	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/curve25519"
	"markdown.ninja/stdx/crypto/chacha20blake3"
)

const (
	// Curve25519PublicKeySize is the size, in bytes, of public keys as used in this package.
	Curve25519PublicKeySize = curve25519.PointSize

	// Curve25519PrivateKeySize is the size, in bytes, of private keys as used in this package.
	Curve25519PrivateKeySize = curve25519.ScalarSize

	// X25519SharedSecretSize is the size, in bytes of the shared secret of a x25519 key exchange
	X25519SharedSecretSize = 32
)

// GenerateCurve25519KeyPair generates a public/private Curve25519 key pair
func GenerateCurve25519KeyPair() (publicKey Curve25519PublicKey, privateKey Curve25519PrivateKey, err error) {
	privateKey = RandBytes(Curve25519PrivateKeySize)

	publicKey, err = privateKey.Public()
	return
}

// Curve25519PublicKey is the type of Curve25519 public keys.
type Curve25519PublicKey []byte

// KeyExchange performs a x25519 key exchange with the given private key
func (publicKey Curve25519PublicKey) KeyExchange(privateKey Curve25519PrivateKey) (sharedSecret []byte, err error) {
	sharedSecret, err = curve25519.X25519(privateKey, publicKey)
	return
}

// Encrypt performs a x25519 key exchange, and encrypt the message using `XChaCha20-Poly1305` with
// the shared secret as key and nonce as nonce.
func (publicKey Curve25519PublicKey) Encrypt(fromPrivateKey Curve25519PrivateKey, nonce []byte,
	message []byte) (ciphertext []byte, err error) {
	sharedSecret, err := publicKey.KeyExchange(fromPrivateKey)
	defer Zeroize(sharedSecret)
	if err != nil {
		return
	}

	cipher, err := chacha20blake3.New(sharedSecret)
	if err != nil {
		return
	}

	ciphertext = cipher.Seal(nil, nonce, message, nil)
	return
}

// EncryptEphemeral generates an ephemeral Curve25519KeyPair and `Encrypt` message using the public key,
// the ephemeral privateKey and `blake2b(size=AEADNonceSize, message=ephemeralPublicKey || publicKey)` as nonce
func (publicKey Curve25519PublicKey) EncryptEphemeral(message []byte) (ciphertext []byte, ephemeralPublicKey Curve25519PublicKey, err error) {
	ephemeralPublicKey, ephemeralPrivateKey, err := GenerateCurve25519KeyPair()
	defer Zeroize(ephemeralPrivateKey)
	if err != nil {
		return
	}

	nonce, err := generateNonce(ephemeralPublicKey, publicKey)
	if err != nil {
		return
	}

	ciphertext, err = publicKey.Encrypt(ephemeralPrivateKey, nonce, message)
	return
}

func generateNonce(ephemeralPublicKey, publicKey Curve25519PublicKey) (nonce []byte, err error) {
	var nonceMessage []byte

	nonceMessage = append(nonceMessage, []byte(ephemeralPublicKey)...)
	nonceMessage = append(nonceMessage, []byte(publicKey)...)
	hash, err := blake2b.New(chacha20blake3.NonceSize, nil)
	if err != nil {
		return
	}
	hash.Write(nonceMessage)
	nonce = hash.Sum(nil)
	return
}

// Curve25519PrivateKey is the type of Curve25519 private keys.
type Curve25519PrivateKey []byte

// Public returns the Curve25519PublicKey corresponding to privateKey.
func (privateKey Curve25519PrivateKey) Public() (publicKey Curve25519PublicKey, err error) {
	publicKey, err = curve25519.X25519(privateKey, curve25519.Basepoint)
	return
}

// KeyExchange performs a x25519 key exchange with the given public key
func (privateKey Curve25519PrivateKey) KeyExchange(publicKey Curve25519PublicKey) (sharedSecret []byte, err error) {
	sharedSecret, err = curve25519.X25519(privateKey, publicKey)
	return
}

// Decrypt performs a x25519 key exchange, and decrypt the ciphertext using `XChaCha20-Poly1305` with
// the shared secret as key and nonce as nonce.
func (privateKey Curve25519PrivateKey) Decrypt(fromPublicKey Curve25519PublicKey, nonce []byte, ciphertext []byte) (plaintext []byte, err error) {
	sharedSecret, err := privateKey.KeyExchange(fromPublicKey)
	defer Zeroize(sharedSecret)
	if err != nil {
		return
	}

	cipher, err := chacha20blake3.New(sharedSecret)
	if err != nil {
		return
	}

	plaintext, err = cipher.Open(nil, nonce, ciphertext, nil)
	return
}

// DecryptEphemeral generates a noce with `blake2b(size=AEADNonceSize, message=ephemeralPublicKey || privateKey.PublicKey())`
// and decrypt the `ciphertext` using `Decrypt`
func (privateKey Curve25519PrivateKey) DecryptEphemeral(ephemeralPublicKey Curve25519PublicKey, ciphertext []byte) (plaintext []byte, err error) {
	myPublicKey, err := privateKey.Public()
	if err != nil {
		return
	}

	nonce, err := generateNonce(ephemeralPublicKey, myPublicKey)
	if err != nil {
		return
	}

	return privateKey.Decrypt(ephemeralPublicKey, nonce, ciphertext)
}
