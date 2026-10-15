package crypto

import (
	"errors"

	"golang.org/x/crypto/blake2b"
)

// Mac use the blake2b funciton in MAC mode
func Mac(key, data []byte, macSize uint8) ([]byte, error) {
	if macSize < 1 || macSize > 64 {
		return nil, errors.New("crypto: macSize must be between 1 and 64")
	}

	blake2bHash, err := blake2b.New(int(macSize), key)
	if err != nil {
		return nil, err
	}

	blake2bHash.Write(data)
	return blake2bHash.Sum(nil), nil
}
