package crypto

import (
	"errors"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/blake2b"
)

const (
	// KeySize128 is the size in bytes of a 128 bits key
	KeySize128 = 32
	// KeySize256 is the size in bytes of a 256 bits key
	KeySize256 = 32
	// KeySize384 is the size in bytes of a 384 bits key
	KeySize384 = 48
	// KeySize512 is the size in bytes of a 512 bits key
	KeySize512 = 64
	// KeySize1024 is the size in bytes of a 1024 bits key
	KeySize1024 = 128
	// KeySize2048 is the size in bytes of a 2048 bits key
	KeySize2048 = 256
	// KeySize4096 is the size in bytes of a 4096 bits key
	KeySize4096 = 512
)

// DeriveKeyFromPasswordParams describes the input parameters used by the Argon2id algorithm.
type DeriveKeyFromPasswordParams struct {
	// The amount of memory used by the algorithm (in kibibytes).
	Memory uint32

	// The number of iterations over the memory.
	Iterations uint32

	// The number of threads (or lanes) used by the algorithm.
	Parallelism uint8

	// Size of the generated key. 32 bytes or more is recommended.
	KeySize uint32
}

// DefaultDeriveKeyFromPasswordParams provides some sane default parameters for deriving keys passwords.
// You are encouraged to change the Memory, Iterations and Parallelism parameters
// to values appropriate for the environment that your code will be running in.
var DefaultDeriveKeyFromPasswordParams = DeriveKeyFromPasswordParams{
	Memory:      64 * 1024,
	Iterations:  5,
	Parallelism: 2,
	KeySize:     KeySize256,
}

// DeriveKeyFromPassword derives a key from a human provided password using the argon2id Key Derivation
// Function
func DeriveKeyFromPassword(password, salt []byte, params DeriveKeyFromPasswordParams) ([]byte, error) {
	key := argon2.IDKey(password, salt, params.Iterations, params.Memory, params.Parallelism, params.KeySize)
	if key == nil {
		return nil, errors.New("crypto: Deriving key from password")
	}
	return key, nil
}

// DeriveKeyFromKey derives a key from a high entropy key using the blake2b function
func DeriveKeyFromKey(key, info []byte, keySize uint8) ([]byte, error) {
	if keySize < 1 || keySize > 64 {
		return nil, errors.New("crypto: keySize must be between 1 and 64")
	}

	blake2bHash, err := blake2b.New(int(keySize), key)
	if err != nil {
		return nil, err
	}

	blake2bHash.Write(info)
	return blake2bHash.Sum(nil), nil
}
