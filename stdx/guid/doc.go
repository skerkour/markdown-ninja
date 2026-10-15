// Package guid helps generate and parse GUIDs.
//
// GUIDs are similar to UUIDs (RFC 4122) but use are encoded using base32 to be more user-friendly.
//
// A GUID is a 16 byte (128 bit) array and is compatible with standard UUIDs which is handy if you
// need to interoperate with other systems.
// GUID are stored in databases as a 16 byte array and thus can leverage native UUID types for better
// performances.
//
//	go test -bench=.
//	pkg: github.com/skerkour/stdx-go/guid
//	BenchmarkNewRandomPool/goroutines-4-4           12655129                94.50 ns/op
//	BenchmarkNewRandomPool/goroutines-40-4          12680683                93.89 ns/op
//	BenchmarkNewRandomPool/goroutines-2000-4        12629418                94.45 ns/op
//	BenchmarkNewRandomPool/goroutines-4000-4        12702556                94.73 ns/op
//	BenchmarkNewRandomPool/goroutines-8000-4        12450429                95.09 ns/op
//	BenchmarkNewRandomReader/goroutines-40-4         6811862               154.2 ns/op
//	BenchmarkNewRandomReader/goroutines-2000-4       6851259               165.7 ns/op
//	BenchmarkNewRandomReader/goroutines-4000-4       7351102               158.5 ns/op
//	BenchmarkNewRandomReader/goroutines-8000-4       7219173               154.8 ns/op
//	BenchmarkNewRandomReader/goroutines-40000-4      7054268               159.7 ns/op
package guid
