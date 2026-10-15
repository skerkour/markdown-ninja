package base32

import "encoding/base32"

const (
	Alphabet = "0123456789abcdefghjkmnpqrtuvwxyz"
)

var encoder = base32.NewEncoding(Alphabet).WithPadding(base32.NoPadding)

func EncodeToString(data []byte) string {
	return encoder.EncodeToString(data)
}

func DecodeString(input string) ([]byte, error) {
	return encoder.DecodeString(input)
}

func Decode(dst, src []byte) (n int, err error) {
	return encoder.Decode(dst, src)
}
