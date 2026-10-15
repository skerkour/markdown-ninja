package zign

import (
	"bytes"
	"io"
	"testing"

	"markdown.ninja/stdx/crypto"
)

func TestSignAndVerifyInternal(t *testing.T) {
	randomData10M := crypto.RandBytes(10_000_00)

	testsVector := [][]byte{
		[]byte("Hello World"),
		{},
		randomData10M,
	}

	for _, data := range testsVector {
		dataReader := bytes.NewReader(data)

		publicKey, privateKey, err := crypto.GenerateEd25519KeyPair()
		if err != nil {
			t.Error(err)
		}

		hash, signature, err := hashAndSignFile(privateKey, dataReader)
		if err != nil {
			t.Error(err)
		}

		dataReader.Seek(0, io.SeekStart)

		verifyInput := VerifyInput{
			Reader:     dataReader,
			HashSha256: hash,
			Signature:  signature,
		}
		err = hashDataAndVerifySignature(publicKey, verifyInput)
		if err != nil {
			t.Error(err)
		}
	}
}

func TestSignAndVerifyIvalidHashInternal(t *testing.T) {
	randomData10M := crypto.RandBytes(10_000_00)

	testsVector := [][]byte{
		[]byte("Hello World"),
		{},
		randomData10M,
	}

	for index, data := range testsVector {
		dataReader := bytes.NewReader(data)

		publicKey, privateKey, err := crypto.GenerateEd25519KeyPair()
		if err != nil {
			t.Error(err)
		}

		hash, signature, err := hashAndSignFile(privateKey, dataReader)
		if err != nil {
			t.Error(err)
		}

		hash[1] += 1

		dataReader.Seek(0, io.SeekStart)

		verifyInput := VerifyInput{
			Reader:     dataReader,
			HashSha256: hash,
			Signature:  signature,
		}
		err = hashDataAndVerifySignature(publicKey, verifyInput)
		if err == nil {
			t.Errorf("verify accepting an invalid hash for test vector at index: %d", index)
		}
	}
}

func TestSignAndVerifyIvalidSignatureInternal(t *testing.T) {
	randomData10M := crypto.RandBytes(10_000_00)

	testsVector := [][]byte{
		[]byte("Hello World"),
		{},
		randomData10M,
	}

	for index, data := range testsVector {
		dataReader := bytes.NewReader(data)

		publicKey, privateKey, err := crypto.GenerateEd25519KeyPair()
		if err != nil {
			t.Error(err)
		}

		hash, signature, err := hashAndSignFile(privateKey, dataReader)
		if err != nil {
			t.Error(err)
		}

		signature[1] += 1

		dataReader.Seek(0, io.SeekStart)

		verifyInput := VerifyInput{
			Reader:     dataReader,
			HashSha256: hash,
			Signature:  signature,
		}
		err = hashDataAndVerifySignature(publicKey, verifyInput)
		if err == nil {
			t.Errorf("verify accepting an invalid signature for test vector at index: %d", index)
		}
	}
}
