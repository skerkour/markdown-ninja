// Package env provides environment variable unmarshalling into Go structs.
//
// Basic usage:
//
//	type Config struct {
//		Host string  `env:"HOST"`
//		Port int     `env:"PORT,default=8080"`
//		Debug bool   `env:"DEBUG"`
//	}
//
//	var cfg Config
//	err := env.Unmarshal(env.Load(), &cfg)
//
// Tag options:
//
//	env:"KEY"          — use KEY as the environment variable name
//	env:"KEY,required" — return an error if KEY is not set
//	env:"KEY,default=V" — use V as the fallback value
//	env:"-"             — skip the field entirely
//
// Without a tag, the field name is used as the key.
//
// Supported types:
//   - string, bool, all int/uint/float widths
//   - time.Duration
//   - []byte (base64-encoded)
//   - slices of any supported type or TextUnmarshaler elements
//   - structs implementing encoding.TextUnmarshaler or encoding.BinaryUnmarshaler
//     (e.g., net.IP, url.URL, netip.Addr)
//   - nested structs (value or pointer), with keys joined by "_"
//   - custom types with underlying supported types
//
// Pointer-to-struct fields are only initialized when at least one matching
// environment variable is found or a default tag exists in the subtree.
package env
