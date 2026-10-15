package main

import (
	"bytes"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	type testCase struct {
		Password string
		Data     string
	}
	tests := []testCase{
		{
			"",
			"",
		},
		{
			"password",
			"",
		},
		{
			"",
			"data",
		},
		{
			"password",
			"data",
		},
		{
			"password",
			// echo -n 'data' | shasum -a 512, repeated
			"77c7ce9a5d86bb386d443bb96390faa120633158699c8844c30b13ab0bf92760b7e4416aea397db91b4ac0e5dd56b8ef7e4b066162ab1fdc088319ce6defc87677c7ce9a5d86bb386d443bb96390faa120633158699c8844c30b13ab0bf92760b7e4416aea397db91b4ac0e5dd56b8ef7e4b066162ab1fdc088319ce6defc87677c7ce9a5d86bb386d443bb96390faa120633158699c8844c30b13ab0bf92760b7e4416aea397db91b4ac0e5dd56b8ef7e4b066162ab1fdc088319ce6defc876",
		},
	}

	for i, test := range tests {
		ciphertext, err := encrypt([]byte(test.Password), []byte(test.Data))
		if err != nil {
			t.Fatalf("error encrypting data [%d]: %s", i, err)
		}
		if bytes.Equal(ciphertext, []byte(test.Data)) ||
			(len(test.Data) != 0 && bytes.Equal(ciphertext[:len(test.Data)], []byte(test.Data))) {
			t.Fatalf("ciphertext == data for %d", i)
		}

		plaintext, err := decrypt([]byte(test.Password), ciphertext)
		if err != nil {
			t.Fatalf("error decrypting data [%d]: %s", i, err)
		}

		ciphertext2 := bytes.Clone(ciphertext)
		_, err = decrypt([]byte(test.Password+"1"), ciphertext2)
		if err == nil {
			t.Fatalf("expected error when using invalid password decrypting data for [%d]", i)
		}

		if !bytes.Equal(plaintext, []byte(test.Data)) {
			t.Fatalf("data (%s) != decrypted plaintext (%s) for %d", test.Data, string(plaintext), i)
		}
	}
}
