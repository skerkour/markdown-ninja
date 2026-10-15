package crypto

// HashSize is the size of a hash, in bytes.
type HashSize uint32

const (
	// HashSize256 is the size in bytes of a 256 bits hash
	HashSize256 HashSize = 32
	// HashSize384 is the size in bytes of a 384 bits hash
	HashSize384 HashSize = 48
	// HashSize512 is the size in bytes of a 512 bits hash
	HashSize512 HashSize = 64
)

// // NewHashBlake2b returns a new `hash.Hash` computing the BLAKE2b checksum with a custom length.
// // size can be a value between 1 and 64.
// // It is highly recommended to use values equal or greater than 32.
// func NewHashBlake2b(size HashSize, key []byte) (hash.Hash, error) {
// 	return blake2b.New(int(size), key)
// }

// // Hash256 returns the BLAKE2b-256 checksum of the data.
// func HashBlake2b256(data []byte) []byte {
// 	sum := blake2b.Sum256(data)
// 	return sum[:]
// }

// // Hash384 returns the BLAKE2b-384 checksum of the data.
// func HashBlake2b384(data []byte) []byte {
// 	sum := blake2b.Sum384(data)
// 	return sum[:]
// }

// // Hash512 returns the BLAKE2b-512 checksum of the data.
// func HashBlake2b512(data []byte) []byte {
// 	sum := blake2b.Sum512(data)
// 	return sum[:]
// }

// // NewHashSha256Hash returns a new `hash.Hash` computing the SHA256 checksum.
// func NewHashSha256() hash.Hash {
// 	return sha256.New()
// }

// // HashSha256 returns the SHA256 checksum of the data.
// func HashSha256(data []byte) []byte {
// 	sum := sha256.Sum256(data)
// 	return sum[:]
// }
