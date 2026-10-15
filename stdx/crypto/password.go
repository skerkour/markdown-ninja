package crypto

// Package argon2id provides a convience wrapper around Go's golang.org/x/crypto/argon2
// implementation, making it simpler to securely hash and verify passwords
// using Argon2.
//
// It enforces use of the Argon2id algorithm variant and cryptographically-secure
// random salts.

import (
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

var (
	// ErrInvalidPasswordHash in returned by ComparePasswordAndHash if the provided
	// hash isn't in the expected format.
	ErrInvalidPasswordHash = errors.New("crypto: hash is not in the correct format")

	// ErrIncompatiblePasswordHashVersion in returned by ComparePasswordAndHash if the
	// provided hash was created using a different version of Argon2.
	ErrIncompatiblePasswordHashVersion = errors.New("crypto: incompatible version of argon2")
)

// DefaultHashPasswordParams provides some sane default parameters for hashing passwords.
// You are encouraged to change the Memory, Iterations and Parallelism parameters
// to values appropriate for the environment that your code will be running in.
var DefaultHashPasswordParams = HashPasswordParams{
	Memory:      64 * 1024,
	Iterations:  3,
	Parallelism: 2,
	SaltLength:  KeySize512,
	KeyLength:   KeySize512,
}

// HashPasswordParams describes the input parameters used by the Argon2id algorithm. The
// Memory and Iterations parameters control the computational cost of hashing
// the password. The higher these figures are, the greater the cost of generating
// the hash and the longer the runtime. It also follows that the greater the cost
// will be for any attacker trying to guess the password. If the code is running
// on a machine with multiple cores, then you can decrease the runtime without
// reducing the cost by increasing the Parallelism parameter. This controls the
// number of threads that the work is spread across. Important note: Changing the
// value of the Parallelism parameter changes the hash output.
//
// For guidance and an outline process for choosing appropriate parameters see
// https://tools.ietf.org/html/draft-irtf-cfrg-argon2-04#section-4
type HashPasswordParams struct {
	// The amount of memory used by the algorithm (in kibibytes).
	Memory uint32

	// The number of iterations over the memory.
	Iterations uint32

	// The number of threads (or lanes) used by the algorithm.
	Parallelism uint8

	// Length of the random salt. 16 bytes is recommended for password hashing.
	SaltLength uint32

	// Length of the generated key. 32 bytes or more is recommended.
	KeyLength uint32
}

// HashPassword returns a Argon2id hash of a plain-text password using the
// provided algorithm parameters. The returned hash follows the format used by
// the Argon2 reference C implementation and contains the base64-encoded Argon2id d
// derived key prefixed by the salt and parameters. It looks like this:
//
//	$argon2id$v=19$m=65536,t=3,p=2$c29tZXNhbHQ$RdescudvJCsgt3ub+b+dWRWJTmaaJObG
func HashPassword(password []byte, params HashPasswordParams) (hash string) {
	salt := RandBytes(uint64(params.SaltLength))

	key := argon2.IDKey(password, salt, params.Iterations, params.Memory, params.Parallelism, params.KeyLength)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Key := base64.RawStdEncoding.EncodeToString(key)

	hash = fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, params.Memory, params.Iterations, params.Parallelism, b64Salt, b64Key)
	return hash
}

// VerifyPasswordHash performs a constant-time comparison between a
// plain-text password and Argon2id hash, using the parameters and salt
// contained in the hash. It returns true if they match, otherwise it returns
// false.
func VerifyPasswordHash(password []byte, hash string) bool {
	params, salt, key, err := decodePasswordHash(hash)
	if err != nil {
		return false
	}

	otherKey := argon2.IDKey(password, salt, params.Iterations, params.Memory, params.Parallelism, params.KeyLength)

	keyLen := int32(len(key))
	otherKeyLen := int32(len(otherKey))

	if subtle.ConstantTimeEq(keyLen, otherKeyLen) == 0 {
		return false
	}
	if subtle.ConstantTimeCompare(key, otherKey) == 1 {
		return true
	}
	return false
}

func decodePasswordHash(hash string) (params *HashPasswordParams, salt, key []byte, err error) {
	vals := strings.Split(hash, "$")
	if len(vals) != 6 {
		return nil, nil, nil, ErrInvalidPasswordHash
	}

	var version int
	_, err = fmt.Sscanf(vals[2], "v=%d", &version)
	if err != nil {
		return nil, nil, nil, err
	}
	if version != argon2.Version {
		return nil, nil, nil, ErrIncompatiblePasswordHashVersion
	}

	params = &HashPasswordParams{}
	_, err = fmt.Sscanf(vals[3], "m=%d,t=%d,p=%d", &params.Memory, &params.Iterations, &params.Parallelism)
	if err != nil {
		return nil, nil, nil, err
	}

	salt, err = base64.RawStdEncoding.DecodeString(vals[4])
	if err != nil {
		return nil, nil, nil, err
	}
	params.SaltLength = uint32(len(salt))

	key, err = base64.RawStdEncoding.DecodeString(vals[5])
	if err != nil {
		return nil, nil, nil, err
	}
	params.KeyLength = uint32(len(key))

	return params, salt, key, nil
}
