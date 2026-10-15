package zign

import (
	"encoding/base64"
	"fmt"

	"markdown.ninja/stdx/crypto"
)

const SaltSize = crypto.KeySize256

func Init(password []byte) (encryptedAndEncodedPrivateKey string, encodedPublicKey string, err error) {
	publicKey, privateKey, err := crypto.GenerateEd25519KeyPair()
	if err != nil {
		err = fmt.Errorf("zign.Init: generating ed25519 keypair: %w", err)
		return
	}
	defer crypto.Zeroize(privateKey)

	salt := crypto.RandBytes(SaltSize)

	encryptionKey, err := crypto.DeriveKeyFromPassword(password, salt, crypto.DefaultDeriveKeyFromPasswordParams)
	if err != nil {
		err = fmt.Errorf("zign.Init: deriving encryption key from password: %w", err)
		return
	}

	encryptedPrivateKey, err := crypto.Encrypt(encryptionKey, privateKey.Bytes(), salt)
	if err != nil {
		err = fmt.Errorf("zign.Init: encrypting private key: %w", err)
		return
	}

	encryptedPrivateKeyAndSalt := append(encryptedPrivateKey, salt...)

	encryptedAndEncodedPrivateKey = base64.StdEncoding.EncodeToString(encryptedPrivateKeyAndSalt)
	encodedPublicKey = base64.StdEncoding.EncodeToString(publicKey.Bytes())

	return
}
