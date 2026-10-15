package blake3_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"hash"
	"io"
	"math/rand"
	"os"
	"testing"

	zeebo "github.com/zeebo/blake3"
	"markdown.ninja/stdx/crypto/blake3"
)

var testVectors = func() (vecs struct {
	Key           string `json:"key"`
	ContextString string `json:"context_string"`
	Cases         []struct {
		InputLen  int    `json:"input_len"`
		Hash      string `json:"hash"`
		KeyedHash string `json:"keyed_hash"`
		DeriveKey string `json:"derive_key"`
	}
}) {
	data, err := os.ReadFile("testdata/test_vectors.json")
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(data, &vecs); err != nil {
		panic(err)
	}
	return
}()

var testInput = func() []byte {
	input := make([]byte, 1e6)
	for i := range input {
		input[i] = byte(i % 251)
	}
	return input
}()

func fromHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestVectors checks the unkeyed, keyed and derive-key modes against the
// official BLAKE3 test vectors, including the extended 131-byte outputs which
// exercise the XOF reader past its first output block.
func TestVectors(t *testing.T) {
	var key [32]byte
	copy(key[:], []byte(testVectors.Key))
	const ctx = "BLAKE3 2019-12-27 16:29:52 test vectors context"

	for i, vec := range testVectors.Cases {
		in := testInput[:vec.InputLen]

		// Unkeyed one-shot hash.
		if got := blake3.Sum256(in); !bytes.Equal(got[:], fromHex(t, vec.Hash)[:32]) {
			t.Errorf("case %d: Sum256 mismatch", i)
		}

		// Unkeyed extended output via Hasher.Xof.
		wantHash := fromHex(t, vec.Hash)
		h := blake3.New()
		h.Write(in)
		if got := h.Sum(nil); !bytes.Equal(got, wantHash[:32]) {
			t.Errorf("case %d: streaming Sum mismatch", i)
		}
		xof := make([]byte, len(wantHash))
		h.Xof().Read(xof)
		if !bytes.Equal(xof, wantHash) {
			t.Errorf("case %d: extended output mismatch", i)
		}

		// Keyed one-shot and streaming.
		if got := blake3.KeyedHash(key, in); !bytes.Equal(got[:], fromHex(t, vec.KeyedHash)[:32]) {
			t.Errorf("case %d: KeyedHash mismatch", i)
		}
		wantKeyed := fromHex(t, vec.KeyedHash)
		hk := blake3.NewKeyed(key)
		hk.Write(in)
		xof = make([]byte, len(wantKeyed))
		hk.Xof().Read(xof)
		if !bytes.Equal(xof, wantKeyed) {
			t.Errorf("case %d: keyed extended output mismatch", i)
		}

		// Derive-key one-shot and streaming.
		if got := blake3.DeriveKey(ctx, in); !bytes.Equal(got[:], fromHex(t, vec.DeriveKey)[:32]) {
			t.Errorf("case %d: DeriveKey mismatch", i)
		}
		wantDK := fromHex(t, vec.DeriveKey)
		hd := blake3.NewDeriveKey(ctx)
		hd.Write(in)
		xof = make([]byte, len(wantDK))
		hd.Xof().Read(xof)
		if !bytes.Equal(xof, wantDK) {
			t.Errorf("case %d: derive-key extended output mismatch", i)
		}
	}
}

func TestKnownDigests(t *testing.T) {
	if got := blake3.Sum256([]byte("abc")); got != [32]byte{0x64, 0x37, 0xb3, 0xac, 0x38, 0x46, 0x51, 0x33, 0xff, 0xb6, 0x3b, 0x75, 0x27, 0x3a, 0x8d, 0xb5, 0x48, 0xc5, 0x58, 0x46, 0x5d, 0x79, 0xdb, 0x03, 0xfd, 0x35, 0x9c, 0x6c, 0xd5, 0xbd, 0x9d, 0x85} {
		t.Errorf("Sum256(\"abc\") mismatch")
	}
	if got := blake3.Sum256(nil); got != [32]byte{0xaf, 0x13, 0x49, 0xb9, 0xf5, 0xf9, 0xa1, 0xa6, 0xa0, 0x40, 0x4d, 0xea, 0x36, 0xdc, 0xc9, 0x49, 0x9b, 0xcb, 0x25, 0xc9, 0xad, 0xc1, 0x12, 0xb7, 0xcc, 0x9a, 0x93, 0xca, 0xe4, 0x1f, 0x32, 0x62} {
		t.Errorf("Sum256(nil) mismatch")
	}
}

// TestStreamingEquivalence checks that writing data in arbitrary splits
// produces the same digest as hashing it in one shot.
func TestStreamingEquivalence(t *testing.T) {
	data := make([]byte, 1<<17)
	for i := range data {
		data[i] = byte(i * 7)
	}
	want := blake3.Sum256(data)

	splits := [][]int{
		{0},
		{1, 2, 3, 4},
		{64, 64},
		{1023, 1},
		{1024},
		{1025},
		{1 << 16},
	}
	for _, split := range splits {
		h := blake3.New()
		p := data
		for _, n := range split {
			if n > len(p) {
				n = len(p)
			}
			h.Write(p[:n])
			p = p[n:]
		}
		h.Write(p)
		if got := h.Sum(nil); !bytes.Equal(got, want[:]) {
			t.Errorf("split %v: mismatch", split)
		}
	}
}

// TestCrossCheckZeebo checks byte-for-byte agreement with the reference
// zeebo/blake3 implementation across all three modes and the XOF reader.
func TestCrossCheckZeebo(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	var key [32]byte
	for i := range key {
		key[i] = byte(rng.Intn(256))
	}
	const ctx = "stdx-go blake3 cross-check context"

	sizes := []int{0, 1, 32, 63, 64, 65, 255, 256, 1023, 1024, 1025, 2048, 2049, 4096, 8192, 65537, 1000000}
	for i := 0; i < 20; i++ {
		sizes = append(sizes, rng.Intn(1<<20))
	}

	for _, n := range sizes {
		data := make([]byte, n)
		for i := range data {
			data[i] = byte(rng.Intn(256))
		}

		// Unkeyed hash.
		if got, want := blake3.Sum256(data), zeebo.Sum256(data); got != want {
			t.Errorf("Sum256(%d): got %x want %x", n, got, want)
		}

		// Keyed hash.
		hk, err := zeebo.NewKeyed(key[:])
		if err != nil {
			t.Fatal(err)
		}
		hk.Write(data)
		if got, want := blake3.KeyedHash(key, data), [32]byte(hk.Sum(nil)); got != want {
			t.Errorf("KeyedHash(%d): got %x want %x", n, got, want)
		}

		// Derive key.
		derived := make([]byte, 32)
		zeebo.DeriveKey(ctx, data, derived)
		if got, want := blake3.DeriveKey(ctx, data), [32]byte(derived); got != want {
			t.Errorf("DeriveKey(%d): got %x want %x", n, got, want)
		}

		// Extended output.
		h := blake3.New()
		h.Write(data)
		z := zeebo.New()
		z.Write(data)
		wantXof := make([]byte, 131)
		z.Digest().Read(wantXof)
		gotXof := make([]byte, 131)
		h.Xof().Read(gotXof)
		if !bytes.Equal(gotXof, wantXof) {
			t.Errorf("XOF(%d): got %x want %x", n, gotXof, wantXof)
		}
	}
}

func TestXof(t *testing.T) {
	for _, n := range []int{0, 1, 63, 64, 65, 1000, 1 << 16} {
		data := make([]byte, n)
		for i := range data {
			data[i] = byte(i*3 + 1)
		}
		h := blake3.New()
		h.Write(data)

		// Reading in arbitrary sized chunks must match a single read.
		oneShot := make([]byte, 4096)
		h.Xof().Read(oneShot)
		var chunked bytes.Buffer
		io.CopyBuffer(&chunked, io.LimitReader(h.Xof(), 4096), make([]byte, 7))
		if !bytes.Equal(chunked.Bytes(), oneShot) {
			t.Errorf("XOF(%d): chunked read mismatch", n)
		}

		// The first 32 bytes of the XOF match Sum256.
		var sum [32]byte
		h.Xof().Read(sum[:])
		if got := h.Sum(nil); !bytes.Equal(got, sum[:]) {
			t.Errorf("XOF(%d): first 32 bytes != Sum", n)
		}
	}
}

func TestXofSeek(t *testing.T) {
	data := make([]byte, 2000)
	for i := range data {
		data[i] = byte(i)
	}
	h := blake3.New()
	h.Write(data)

	stream := func() []byte {
		out := make([]byte, 512)
		h.Xof().Read(out)
		return out
	}

	// A fresh reader must always start at the beginning.
	base := stream()

	// Seek to a middle position and compare with the reference stream.
	mid := make([]byte, 128)
	r := h.Xof()
	if pos, err := r.Seek(200, io.SeekStart); err != nil || pos != 200 {
		t.Fatalf("SeekStart(200): pos=%d err=%v", pos, err)
	}
	r.Read(mid)
	if !bytes.Equal(mid, base[200:328]) {
		t.Error("SeekStart result mismatch")
	}

	// SeekCurrent.
	r2 := h.Xof()
	if pos, err := r2.Seek(200, io.SeekStart); err != nil || pos != 200 {
		t.Fatalf("SeekStart(200): pos=%d err=%v", pos, err)
	}
	if pos, err := r2.Seek(50, io.SeekCurrent); err != nil || pos != 250 {
		t.Fatalf("SeekCurrent(50): pos=%d err=%v", pos, err)
	}
	buf := make([]byte, 64)
	r2.Read(buf)
	if !bytes.Equal(buf, base[250:314]) {
		t.Error("SeekCurrent result mismatch")
	}

	// SeekCurrent backwards within bounds. The 64-byte read above advanced the
	// position from 250 to 314.
	if pos, err := r2.Seek(-100, io.SeekCurrent); err != nil || pos != 214 {
		t.Fatalf("SeekCurrent(-100): pos=%d err=%v", pos, err)
	}
	r2.Read(buf)
	if !bytes.Equal(buf, base[214:278]) {
		t.Error("SeekCurrent backwards result mismatch")
	}

	// Re-seek and re-read must reproduce the same bytes.
	r3 := h.Xof()
	r3.Seek(0, io.SeekStart)
	r3.Seek(300, io.SeekStart)
	r3.Read(buf)
	r3.Seek(300, io.SeekStart)
	var buf2 [64]byte
	r3.Read(buf2[:])
	if !bytes.Equal(buf, buf2[:]) {
		t.Error("re-seek mismatch")
	}

	// Errors.
	if _, err := h.Xof().Seek(0, io.SeekEnd); err == nil {
		t.Error("SeekEnd should error")
	}
	if _, err := h.Xof().Seek(0, 42); err == nil {
		t.Error("invalid whence should error")
	}
	if _, err := h.Xof().Seek(-1, io.SeekStart); err == nil {
		t.Error("negative SeekStart should error")
	}
	if _, err := h.Xof().Seek(-1, io.SeekCurrent); err == nil {
		t.Error("negative SeekCurrent should error")
	}
}

// TestXofIndependence checks that multiple readers over the same hasher state
// are independent, and that creating a reader does not mutate the hasher.
func TestXofIndependence(t *testing.T) {
	h := blake3.New()
	h.Write([]byte("hello"))

	before := h.Xof()
	before2 := h.Xof()
	var a, b [32]byte
	before.Read(a[:])
	h.Xof().Read(b[:])
	if a != b {
		t.Error("two readers over the same state must produce identical streams")
	}

	// Writing more data after creating a reader changes future readers but
	// not the captured ones.
	h.Write([]byte(" world"))
	var c [32]byte
	h.Xof().Read(c[:])
	if a == c {
		t.Error("hasher was mutated by reader creation")
	}
	var a2 [32]byte
	before2.Read(a2[:])
	if a != a2 {
		t.Error("captured reader changed after parent write")
	}
}

// TestHashHashInterface exercises Hasher through the hash.Hash interface.
func TestHashHashInterface(t *testing.T) {
	var _ hash.Hash = blake3.New()

	data := []byte("interface test data")
	var h hash.Hash = blake3.New()
	if h.Size() != 32 || h.BlockSize() != 64 {
		t.Fatalf("Size=%d BlockSize=%d", h.Size(), h.BlockSize())
	}
	io.WriteString(h, "interface ")
	h.Write([]byte("test data"))
	if got, want := h.Sum(nil), blake3.Sum256(data); !bytes.Equal(got, want[:]) {
		t.Error("hash.Hash digest mismatch")
	}

	// Sum must not mutate the hasher.
	before := h.Sum(nil)
	if after := h.Sum(nil); !bytes.Equal(before, after) {
		t.Error("Sum is not idempotent")
	}

	// Reset returns to the empty state.
	h.Reset()
	if got, want := h.Sum(nil), blake3.Sum256(nil); !bytes.Equal(got, want[:]) {
		t.Error("Reset did not restore empty state")
	}
}

func TestNewDeriveKeyStreaming(t *testing.T) {
	// Writing the key material to a NewDeriveKey hasher must equal DeriveKey.
	const ctx = "test context"
	key := blake3.DeriveKey(ctx, []byte("key material"))
	h := blake3.NewDeriveKey(ctx)
	h.Write([]byte("key material"))
	if got, want := h.Sum(nil), key; !bytes.Equal(got, want[:]) {
		t.Error("NewDeriveKey + Write != DeriveKey")
	}
}
