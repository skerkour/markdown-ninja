package crypto

import "testing"

func TestHashSizes(t *testing.T) {
	if HashSize256 != 32 {
		t.Error("HashSize256 != 32")
	}
	if HashSize384 != 48 {
		t.Error("HashSize384 != 48")
	}
	if HashSize512 != 64 {
		t.Error("HashSize512 != 64")
	}
}

// func TestHash256(t *testing.T) {
// 	data := []byte("a random string")
// 	hash := Hash256(data)

// 	if len(hash) != int(HashSize256) {
// 		t.Error("len(hash) != HashSize256")
// 	}
// }

// func TestHash384(t *testing.T) {
// 	data := []byte("a random string")
// 	hash := Hash384(data)

// 	if len(hash) != int(HashSize384) {
// 		t.Error("len(hash) != HashSize384")
// 	}
// }

// func TestHash512(t *testing.T) {
// 	data := []byte("a random string")
// 	hash := Hash512(data)

// 	if len(hash) != int(HashSize512) {
// 		t.Error("len(hash) != HashSize512")
// 	}
// }
