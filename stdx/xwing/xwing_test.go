package xwing

import (
	"bytes"
	"crypto/ecdh"
	"crypto/mlkem"
	"crypto/mlkem/mlkemtest"
	"encoding/hex"
	"testing"
)

func fromHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

func fromHex32(s string) (b [32]byte) {
	copy(b[:], fromHex(s))
	return
}

func encapsulateDerand(encapKey *EncapsulationKey, eseed []byte) (sharedSecret [SharedKeySize]byte, ciphertext []byte, err error) {
	publicKeyX25519 := encapKey.publicKeyX25519[:]

	x25519EphemeralKey, err := ecdh.X25519().NewPrivateKey(eseed[32:])
	if err != nil {
		return [SharedKeySize]byte{}, nil, err
	}
	x25519PeerKey, err := ecdh.X25519().NewPublicKey(publicKeyX25519)
	if err != nil {
		return [SharedKeySize]byte{}, nil, err
	}
	ciphertextX25519 := x25519EphemeralKey.PublicKey().Bytes()
	sharedSecretX25519, err := x25519EphemeralKey.ECDH(x25519PeerKey)
	if err != nil {
		return [SharedKeySize]byte{}, nil, err
	}

	sharedSecretMlkem, ciphertextMlkem, err := mlkemtest.Encapsulate768(encapKey.mlkemEncapsulationKey, eseed[:32])
	if err != nil {
		return [SharedKeySize]byte{}, nil, err
	}

	sharedSecret = combiner(sharedSecretMlkem, sharedSecretX25519, ciphertextX25519, publicKeyX25519)

	ciphertext = make([]byte, CiphertextSize)
	copy(ciphertext[:mlkem.CiphertextSize768], ciphertextMlkem)
	copy(ciphertext[mlkem.CiphertextSize768:], ciphertextX25519)

	return sharedSecret, ciphertext, nil
}

func TestConstants(t *testing.T) {
	if EncapsulationKeySize != 1216 {
		t.Errorf("EncapsulationKeySize = %d, want 1216", EncapsulationKeySize)
	}
	if CiphertextSize != 1120 {
		t.Errorf("CiphertextSize = %d, want 1120", CiphertextSize)
	}
	if SharedKeySize != 32 {
		t.Errorf("SharedKeySize = %d, want 32", SharedKeySize)
	}
	if SeedSize != 32 {
		t.Errorf("SeedSize = %d, want 32", SeedSize)
	}
}

func TestNewEncapsulationKeyFromBytesInvalidLength(t *testing.T) {
	_, err := NewEncapsulationKeyFromBytes(make([]byte, 0))
	if err == nil {
		t.Error("expected error for empty key")
	}
	_, err = NewEncapsulationKeyFromBytes(make([]byte, 100))
	if err == nil {
		t.Error("expected error for short key")
	}
}

func TestGenerateDecapsulationKey(t *testing.T) {
	decapKey := GenerateDecapsulationKey()
	if len(decapKey.Bytes()) != SeedSize {
		t.Errorf("Bytes() length = %d, want %d", len(decapKey.Bytes()), SeedSize)
	}
	zeroSeed := make([]byte, SeedSize)
	if bytes.Equal(decapKey.Bytes(), zeroSeed) {
		t.Error("generated seed is all zeros")
	}
}

func TestDecapsulationKeyEncapsulationKeyRoundTripBytes(t *testing.T) {
	decapKey := GenerateDecapsulationKey()

	seed := decapKey.Bytes()
	seedAgain := decapKey.Bytes()
	if !bytes.Equal(seed, seedAgain) {
		t.Error("Bytes() is not idempotent")
	}

	encapKey := decapKey.EncapsulationKey()
	encapKeyBytes := encapKey.Bytes()
	if len(encapKeyBytes) != EncapsulationKeySize {
		t.Errorf("EncapsulationKey.Bytes() length = %d, want %d", len(encapKeyBytes), EncapsulationKeySize)
	}

	encapKey2, err := NewEncapsulationKeyFromBytes(encapKeyBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encapKey2.Bytes(), encapKeyBytes) {
		t.Error("NewEncapsulationKeyFromBytes round-trip failed")
	}
}

func TestDecapsulateInvalidLength(t *testing.T) {
	decapKey := GenerateDecapsulationKey()
	_, err := decapKey.Decapsulate(make([]byte, 0))
	if err == nil {
		t.Error("expected error for empty ciphertext")
	}
	_, err = decapKey.Decapsulate(make([]byte, CiphertextSize+1))
	if err == nil {
		t.Error("expected error for long ciphertext")
	}
}

func TestNewDecapsulationKeyFromSeedDeterministic(t *testing.T) {
	seed := fromHex32("7f9c2ba4e88f827d616045507605853ed73b8093f6efbc88eb1a6eacfa66ef26")

	decapKey1 := NewDecapsulationKeyFromSeed(seed)
	decapKey2 := NewDecapsulationKeyFromSeed(seed)

	if !bytes.Equal(decapKey1.Bytes(), decapKey2.Bytes()) {
		t.Error("seeds differ")
	}
	if !bytes.Equal(decapKey1.EncapsulationKey().Bytes(), decapKey2.EncapsulationKey().Bytes()) {
		t.Error("encapsulation keys differ")
	}
}

func TestEncapsulateRoundTrip(t *testing.T) {
	decapKey := GenerateDecapsulationKey()

	encapKey := decapKey.EncapsulationKey()
	sharedSecret, ciphertext, err := encapKey.Encapsulate()
	if err != nil {
		t.Fatal(err)
	}

	decapsulatedSecret, err := decapKey.Decapsulate(ciphertext)
	if err != nil {
		t.Fatal(err)
	}

	if sharedSecret != decapsulatedSecret {
		t.Error("decapsulated shared secret does not match encapsulated shared secret")
	}
	if len(ciphertext) != CiphertextSize {
		t.Errorf("ciphertext length = %d, want %d", len(ciphertext), CiphertextSize)
	}
}

func TestVectorsDraft(t *testing.T) {
	type testVector struct {
		seed  string
		eseed string
		ss    string
	}

	vectors := []testVector{
		{
			seed:  "7f9c2ba4e88f827d616045507605853ed73b8093f6efbc88eb1a6eacfa66ef26",
			eseed: "3cb1eea988004b93103cfb0aeefd2a686e01fa4a58e8a3639ca8a1e3f9ae57e235b8cc873c23dc62b8d260169afa2f75ab916a58d974918835d25e6a435085b2",
			ss:    "d2df0522128f09dd8e2c92b1e905c793d8f57a54c3da25861f10bf4ca613e384",
		},
		{
			seed:  "badfd6dfaac359a5efbb7bcc4b59d538df9a04302e10c8bc1cbf1a0b3a5120ea",
			eseed: "17cda7cfad765f5623474d368ccca8af0007cd9f5e4c849f167a580b14aabdefaee7eef47cb0fca9767be1fda69419dfb927e9df07348b196691abaeb580b32d",
			ss:    "f2e86241c64d60f6649fbc6c5b7d17180b780a3f34355e64a85749949c45f150",
		},
		{
			seed:  "ef58538b8d23f87732ea63b02b4fa0f4873360e2841928cd60dd4cee8cc0d4c9",
			eseed: "22a96188d032675c8ac850933c7aff1533b94c834adbb69c6115bad4692d8619f90b0cdf8a7b9c264029ac185b70b83f2801f2f4b3f70c593ea3aeeb613a7f1b",
			ss:    "953f7f4e8c5b5049bdc771d1dffada0dd961477d1a2ae0988baa7ea6898d893f",
		},
	}

	for i, tv := range vectors {
		seed := fromHex32(tv.seed)
		eseed := fromHex(tv.eseed)
		var expectedSS [SharedKeySize]byte
		copy(expectedSS[:], fromHex(tv.ss))

		decapKey := NewDecapsulationKeyFromSeed(seed)

		if !bytes.Equal(decapKey.Bytes(), seed[:]) {
			t.Errorf("vector %d: seed mismatch", i)
		}

		encapKey := decapKey.EncapsulationKey()
		sharedSecret, ciphertext, err := encapsulateDerand(encapKey, eseed)
		if err != nil {
			t.Fatalf("vector %d: encapsulateDerand: %v", i, err)
		}

		if sharedSecret != expectedSS {
			t.Errorf("vector %d: encapsulate shared secret mismatch:\n  got:  %x\n  want: %x", i, sharedSecret, expectedSS)
		}

		decapsulatedSS, err := decapKey.Decapsulate(ciphertext)
		if err != nil {
			t.Fatalf("vector %d: Decapsulate: %v", i, err)
		}

		if decapsulatedSS != expectedSS {
			t.Errorf("vector %d: decapsulate shared secret mismatch:\n  got:  %x\n  want: %x", i, decapsulatedSS, expectedSS)
		}
	}
}

func TestRoundTripMany(t *testing.T) {
	for i := 0; i < 10; i++ {
		decapKey := GenerateDecapsulationKey()

		encapKey := decapKey.EncapsulationKey()
		sharedSecret, ciphertext, err := encapKey.Encapsulate()
		if err != nil {
			t.Fatal(err)
		}

		decapsulatedSS, err := decapKey.Decapsulate(ciphertext)
		if err != nil {
			t.Fatal(err)
		}

		if sharedSecret != decapsulatedSS {
			t.Errorf("iteration %d: decapsulated shared secret mismatch", i)
		}
	}
}

func TestWrongKeyProducesDifferentSecret(t *testing.T) {
	decapKeyA := GenerateDecapsulationKey()
	decapKeyB := GenerateDecapsulationKey()

	encapKeyA := decapKeyA.EncapsulationKey()
	sharedSecretA, ciphertext, err := encapKeyA.Encapsulate()
	if err != nil {
		t.Fatal(err)
	}

	sharedSecretB, err := decapKeyB.Decapsulate(ciphertext)
	if err != nil {
		t.Fatal(err)
	}

	if sharedSecretA == sharedSecretB {
		t.Error("decapsulation with wrong key should produce different secret")
	}
}

func TestTamperedCiphertext(t *testing.T) {
	decapKey := GenerateDecapsulationKey()

	encapKey := decapKey.EncapsulationKey()
	sharedSecret, ciphertext, err := encapKey.Encapsulate()
	if err != nil {
		t.Fatal(err)
	}

	ciphertext[0] ^= 0x80

	tamperedSS, err := decapKey.Decapsulate(ciphertext)
	if err != nil {
		t.Fatal(err)
	}

	if sharedSecret == tamperedSS {
		t.Error("tampered ciphertext should produce different secret")
	}
}

func TestCombinerIsDeterministic(t *testing.T) {
	ssM := bytes.Repeat([]byte{0x01}, 32)
	ssX := bytes.Repeat([]byte{0x02}, 32)
	ctX := bytes.Repeat([]byte{0x03}, 32)
	pkX := bytes.Repeat([]byte{0x04}, 32)

	result1 := combiner(ssM, ssX, ctX, pkX)
	result2 := combiner(ssM, ssX, ctX, pkX)

	if result1 != result2 {
		t.Error("combiner is not deterministic")
	}

	if len(result1) != SharedKeySize {
		t.Errorf("combiner output length = %d, want %d", len(result1), SharedKeySize)
	}
}

func TestXwingLabel(t *testing.T) {
	if len(xwingLabel) != 6 {
		t.Errorf("xwingLabel length = %d, want 6", len(xwingLabel))
	}
	expected := []byte("\\.//^\\")
	if !bytes.Equal(xwingLabel, expected) {
		t.Errorf("xwingLabel = %q, want %q", string(xwingLabel), string(expected))
	}
}

func TestDecapsulateWithZerosCiphertext(t *testing.T) {
	seed := fromHex32("7f9c2ba4e88f827d616045507605853ed73b8093f6efbc88eb1a6eacfa66ef26")
	decapKey := NewDecapsulationKeyFromSeed(seed)

	ciphertext := make([]byte, CiphertextSize)
	_, err := decapKey.Decapsulate(ciphertext)
	if err == nil {
		t.Error("expected error when decapsulating zero ciphertext")
	}
}

func TestEncapsulationKeyBytesReturnsCopy(t *testing.T) {
	decapKey := GenerateDecapsulationKey()

	encapKey := decapKey.EncapsulationKey()
	encapKeyBytes := encapKey.Bytes()
	encapKeyBytes[0] ^= 0xff

	if bytes.Equal(encapKeyBytes, encapKey.Bytes()) {
		t.Error("Bytes() should return a copy")
	}
}

func TestDecapsulationKeyBytesReturnsCopy(t *testing.T) {
	decapKey := GenerateDecapsulationKey()

	seed := decapKey.Bytes()
	seed[0] ^= 0xff

	if bytes.Equal(seed, decapKey.Bytes()) {
		t.Error("Bytes() should return a copy")
	}
}
