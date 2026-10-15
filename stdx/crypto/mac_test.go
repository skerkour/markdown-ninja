package crypto

import (
	"bytes"
	"testing"
)

func TestMacLen(t *testing.T) {
	var err error
	data := []byte("com.bloom42.gobox")
	key := RandBytes(KeySize512)

	_, err = Mac(key, data, 128)
	if err == nil {
		t.Error("Accept invalid keyLen")
	}

	_, err = Mac(key, data, 65)
	if err == nil {
		t.Error("Accept invalid keyLen")
	}

	_, err = Mac(key, data, 0)
	if err == nil {
		t.Error("Accept invalid keyLen")
	}

	_, err = Mac(key, data, 1)
	if err != nil {
		t.Error("Reject valid keyLen")
	}

	_, err = Mac(key, data, 64)
	if err != nil {
		t.Error("Reject valid keyLen")
	}
}

func TestMac(t *testing.T) {
	var err error
	data1 := []byte("com.bloom42.gobox1")
	data2 := []byte("com.bloom42.gobox2")
	key1 := RandBytes(KeySize512)
	key2 := RandBytes(KeySize512)

	signature1, err := Mac(key1, data1, KeySize256)
	if err != nil {
		t.Error(err)
	}

	signature2, err := Mac(key1, data2, KeySize256)
	if err != nil {
		t.Error(err)
	}

	if bytes.Equal(signature1, signature2) {
		t.Error("signature1 and signature2 are equal")
	}

	signature3, err := Mac(key1, data1, KeySize256)
	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(signature1, signature3) {
		t.Error("signature1 and signature3 are different")
	}

	signature4, err := Mac(key2, data1, KeySize256)
	if err != nil {
		t.Error(err)
	}

	if bytes.Equal(signature1, signature4) {
		t.Error("subKey1 and signature4 are equal")
	}
}
