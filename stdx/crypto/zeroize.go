package crypto

// Zeroize set all bytes of buffer to 0
func Zeroize(buffer []byte) {
	for i := range buffer {
		buffer[i] = 0
	}
}
