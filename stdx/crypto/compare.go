package crypto

import (
	"crypto/subtle"
)

// ConstantTimeCompare returns true if the 2 buffer are equals and false otherwise
func ConstantTimeCompare(x, y []byte) bool {
	res := subtle.ConstantTimeCompare(x, y)
	return res == 1
}
