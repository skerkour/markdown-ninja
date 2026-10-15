package crypto

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"math/big"
	"math/rand/v2"
)

// const (
// 	chaCha20RandSourceStateSize = 1000
// )

// type ChaCha20RandSource struct {
// 	chacha20Cipher cipher.Stream
// 	state          []byte
// }

// func NewChaCha20RandSource(seed [32]byte) *ChaCha20RandSource {

// 	state :=

// 	source := &ChaCha20RandSource{

// 	}
// }

// ensure that RandSource satisfies the rand.Source  interface
var _ rand.Source = (*RandSource)(nil)

type RandSource struct {
}

func NewRandomGenerator() *rand.Rand {
	source := RandSource{}
	return rand.New(source)
}

func (source RandSource) Uint64() uint64 {
	var buff [8]byte

	_, err := cryptorand.Reader.Read(buff[:])
	if err != nil {
		panic(err)
	}

	return binary.BigEndian.Uint64(buff[:])
}

// RandBytes returns securely generated random bytes.
// It will return an error if the system's secure random
// number generator fails to function correctly, in which
// case the caller should not continue.
func RandBytes(n uint64) []byte {
	b := make([]byte, n)

	// crypto/rand should never return an error
	// See https://github.com/golang/go/issues/66821
	_, err := cryptorand.Read(b)
	if err != nil {
		panic(err)
	}

	return b
}

// RandInt64Between returns a uniform random value in [min, max).
func RandInt64Between(min, max int64) int64 {
	// crypto/rand should never return an error
	// See https://github.com/golang/go/issues/66821
	n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(max-min))
	if err != nil {
		panic(err)
	}

	return n.Int64()
}
