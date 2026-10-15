package blake3

import (
	"errors"
	"io"
)

// XofReader produces a seekable stream of BLAKE3 extendable output (XOF)
// bytes. It implements io.Reader and io.Seeker. The stream is effectively
// unbounded: BLAKE3 can produce up to 2^64 output blocks.
//
// Obtaining the reader does not mutate the parent Hasher, so the two can be
// used independently. Multiple readers from the same Hasher are independent of
// one another.
type XofReader struct {
	n   output // the root output node, without the flagRoot bit
	off uint64 // current position within the output stream, in bytes

	// buf caches xofBufLen output blocks ahead of the current position so
	// small Read calls are served from memory instead of one compression each.
	// bufValid is true when buf holds blocks bufStart..bufStart+len(buf)/blockLen-1.
	buf      [xofBufLen]byte
	bufStart uint64 // block index of buf[0]
	bufValid bool
}

// ensure that XofReader implements io.Reader and io.Seeker at build time.
var (
	_ io.Reader = (*XofReader)(nil)
	_ io.Seeker = (*XofReader)(nil)
)

var (
	ErrSeekEnd      = errors.New("blake3: cannot seek from end of infinite output")
	ErrSeekInvalid  = errors.New("blake3: invalid whence")
	ErrSeekNegative = errors.New("blake3: seek position cannot be negative")
)

// Xof returns a reader over the extendable output (XOF) of the data written so
// far. The hasher is not modified, so more data may still be written after
// calling Xof.
func (h *Hasher) Xof() *XofReader {
	return &XofReader{n: h.rootOutput()}
}

// Read fills p with output bytes, advancing the stream position. It never
// returns an error and always consumes len(p) bytes.
func (r *XofReader) Read(p []byte) (int, error) {
	lenp := len(p)

	// Serve from the cache when it covers the current position.
	if r.bufValid && r.off >= r.bufStart*blockLen && r.off-r.bufStart*blockLen < uint64(len(r.buf)) {
		idx := r.off - r.bufStart*blockLen
		m := copy(p, r.buf[idx:])
		p = p[m:]
		r.off += uint64(m)
		if len(p) == 0 {
			return lenp, nil
		}
	}
	r.bufValid = false

	// Handle a position misaligned to a block boundary with a single
	// compression.
	if offInBlock := uint(r.off % blockLen); offInBlock != 0 {
		var buf [blockLen]byte
		compressOutputs(buf[:], &r.n.inputCV, &r.n.block, r.n.blockLen, r.n.flags|flagRoot, r.off/blockLen)
		m := copy(p, buf[offInBlock:])
		p = p[m:]
		r.off += uint64(m)
		if len(p) == 0 {
			return lenp, nil
		}
	}

	// Now block-aligned. If p fits in the cache, fill it ahead (amortizing the
	// per-block compression over the read) and serve from it; otherwise write
	// straight through in SIMD batches.
	if len(p) <= len(r.buf) {
		compressOutputs(r.buf[:], &r.n.inputCV, &r.n.block, r.n.blockLen, r.n.flags|flagRoot, r.off/blockLen)
		r.bufStart = r.off / blockLen
		r.bufValid = true
		copy(p, r.buf[:len(p)])
		r.off += uint64(len(p))
		return lenp, nil
	}
	compressOutputs(p, &r.n.inputCV, &r.n.block, r.n.blockLen, r.n.flags|flagRoot, r.off/blockLen)
	r.off += uint64(len(p))
	return lenp, nil
}

// Seek sets the stream position according to whence:
//
//	io.SeekStart   position = offset
//	io.SeekCurrent position = current + offset
//	io.SeekEnd     returns an error, since the output stream is unbounded
//
// A negative resulting position is an error. Seek returns the new position and
// nil on success.
func (r *XofReader) Seek(offset int64, whence int) (int64, error) {
	off := int64(r.off)
	switch whence {
	case io.SeekStart:
		off = offset
	case io.SeekCurrent:
		off += offset
	case io.SeekEnd:
		return 0, ErrSeekEnd
	default:
		return 0, ErrSeekInvalid
	}
	if off < 0 {
		return 0, ErrSeekNegative
	}
	r.off = uint64(off)
	r.bufValid = false
	return off, nil
}
