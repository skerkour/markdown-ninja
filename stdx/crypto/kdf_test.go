package crypto

import (
	"bytes"
	"testing"
)

func TestKeySizes(t *testing.T) {
	if KeySize256 != 32 {
		t.Error("KeySize256 != 32")
	}
	if KeySize384 != 48 {
		t.Error("KeySize384 != 48")
	}
	if KeySize512 != 64 {
		t.Error("KeySize512 != 64")
	}
	if KeySize1024 != 128 {
		t.Error("KeySize1024 != 128")
	}
	if KeySize2048 != 256 {
		t.Error("KeySize2048 != 256")
	}
	if KeySize4096 != 512 {
		t.Error("KeySize4096 != 512")
	}
}

func TestDeriveKeyFromKeyKeyLen(t *testing.T) {
	info := []byte("com.bloom42.gobox")
	key := RandBytes(KeySize512)

	_, err := DeriveKeyFromKey(key, info, 128)
	if err == nil {
		t.Error("Accept invalid keyLen")
	}

	_, err = DeriveKeyFromKey(key, info, 65)
	if err == nil {
		t.Error("Accept invalid keyLen")
	}

	_, err = DeriveKeyFromKey(key, info, 0)
	if err == nil {
		t.Error("Accept invalid keyLen")
	}

	_, err = DeriveKeyFromKey(key, info, 1)
	if err != nil {
		t.Error("Reject valid keyLen")
	}

	_, err = DeriveKeyFromKey(key, info, 64)
	if err != nil {
		t.Error("Reject valid keyLen")
	}
}

func TestDeriveKeyFromKeyContext(t *testing.T) {
	info1 := []byte("com.bloom42.gobox1")
	info2 := []byte("com.bloom42.gobox2")
	key1 := RandBytes(KeySize512)
	key2 := RandBytes(KeySize512)

	subKey1, err := DeriveKeyFromKey(key1, info1, KeySize256)
	if err != nil {
		t.Error(err)
	}

	subKey2, err := DeriveKeyFromKey(key1, info2, KeySize256)
	if err != nil {
		t.Error(err)
	}

	if bytes.Equal(subKey1, subKey2) {
		t.Error("subKey1 and subKey2 are equal")
	}

	subKey3, err := DeriveKeyFromKey(key1, info1, KeySize256)
	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(subKey1, subKey3) {
		t.Error("subKey1 and subKey3 are different")
	}

	subKey4, err := DeriveKeyFromKey(key2, info1, KeySize256)
	if err != nil {
		t.Error(err)
	}

	if bytes.Equal(subKey1, subKey4) {
		t.Error("subKey1 and subKey4 are equal")
	}
}
