// Package crypto provides a high level, secure, easy to use, and hard to misuse API to common
// cryptographic operations.
//
// # KDF
//
// KDF (Key Derivation Function) functions should be used to derives encryption keys from passwords
// or other keys.
//
// # Pasword
//
// Only th efunction `HashPassword` should be used for password hashing.
//
// # AEAD
//
// AEAD (Authenticated Encryption with Associated Data) is used for secret key (symmetric) cryptography.
//
// # Hash
//
// hash functions (`Hash{256,384,512}`, `NewHash`) should be used to hashs files or other kind of data.
// NOT FOR PASSWORD.
package crypto
