package crypto

import (
	"regexp"
	"strings"
	"testing"
)

func TestHashPassword(t *testing.T) {
	hashRX, err := regexp.Compile(`^\$argon2id\$v=19\$m=65536,t=3,p=2\$[A-Za-z0-9+/]{86}\$[A-Za-z0-9+/]{86}$`)
	if err != nil {
		t.Fatal(err)
	}

	hash1 := HashPassword([]byte("pa$$word"), DefaultHashPasswordParams)

	if !hashRX.MatchString(hash1) {
		t.Errorf("hash %q not in correct format", hash1)
	}

	hash2 := HashPassword([]byte("pa$$word"), DefaultHashPasswordParams)

	if strings.Compare(hash1, hash2) == 0 {
		t.Error("hashes must be unique")
	}
}

func TestVerifyPasswordHash(t *testing.T) {
	hash := HashPassword([]byte("pa$$word"), DefaultHashPasswordParams)

	match := VerifyPasswordHash([]byte("pa$$word"), hash)

	if !match {
		t.Error("expected password and hash to match")
	}

	match = VerifyPasswordHash([]byte("otherPa$$word"), hash)

	if match {
		t.Error("expected password and hash to not match")
	}
}
