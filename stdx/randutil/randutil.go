package randutil

import "math/rand/v2"

// RandAlphabet returns a buffer a size n filled with random values taken from alphabet
func RandAlphabet(randomGenerator *rand.Rand, alphabet []byte, n uint64) []byte {
	buffer := make([]byte, n)
	alphabetLen := int64(len(alphabet))

	for i := range buffer {
		buffer[i] = alphabet[randomGenerator.Int64N(alphabetLen)]
	}

	return buffer
}

// TODO: handle runes instead of bytes
func RandString(randomGenerator *rand.Rand, alphabet string, n uint64) string {
	buffer := make([]byte, n)
	alphabetLen := int64(len(alphabet))

	for i := range buffer {
		buffer[i] = alphabet[randomGenerator.Int64N(alphabetLen)]
	}

	return string(buffer)
}
