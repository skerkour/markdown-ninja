package uuid

import "bytes"

const Size = 16

func (uuid UUID) Equal(other UUID) bool {
	return bytes.Equal(uuid[:], other[:])
}

func (uuid UUID) IsNil() bool {
	return uuid.Equal(Nil)
}

func (uuid UUID) Bytes() []byte {
	return uuid[:]
}
