package byteshex

import (
	"bytes"
	"encoding/hex"
	"fmt"
)

// Bytes is a simple []byte that encodes to hexadecimal when marshaling to JSONs
type Bytes []byte

var bytesHexNUll = []byte("null")

func (b Bytes) String() string {
	return hex.EncodeToString(b)
}

func (b Bytes) MarshalJSON() ([]byte, error) {
	if b == nil {
		return bytesHexNUll, nil
	}

	buffer := bytes.NewBuffer(make([]byte, 0, (2 + len(b)*2)))
	buffer.WriteRune('"')
	buffer.WriteString(hex.EncodeToString(b))
	buffer.WriteRune('"')
	return buffer.Bytes(), nil
}

func (b *Bytes) UnmarshalJSON(data []byte) (err error) {
	if data == nil || bytes.Equal(data, bytesHexNUll) {
		return nil
	}

	data = bytes.Trim(data, `"`)
	decodedData, err := hex.DecodeString(string(data))
	if err != nil {
		return
	}

	*b = decodedData
	return nil
}

func (b *Bytes) Scan(val any) error {
	switch v := val.(type) {
	case []byte:
		*b = v
		return nil
	case nil:
		return nil
	default:
		return fmt.Errorf("kernel.BytesHex: Unsupported type: %T", v)
	}
}
