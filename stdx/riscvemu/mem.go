package riscvemu

import "encoding/binary"

// PageSize is the guest page size (riscv64 Linux uses 4KiB).
const PageSize = 4096

// Mem is the guest physical memory, modelled as a sparse set of 4KiB pages.
// A nil/invalid page reads as zero and absorbs writes (fault-free execution),
// which is what simple full-system-style emulators do. Real fault semantics
// are not needed to run Go's runtime.
type Mem struct {
	pages map[uint64]*[PageSize]byte
	max   uint64 // highest byte address ever accessed, for range checks
}

func NewMem() *Mem {
	return &Mem{pages: make(map[uint64]*[PageSize]byte)}
}

// Page returns the page containing addr, materializing a zero page if needed.
func (m *Mem) Page(addr uint64) *[PageSize]byte {
	if addr > m.max {
		m.max = addr
	}
	idx := addr >> 12
	p := m.pages[idx]
	if p == nil {
		p = &[PageSize]byte{}
		m.pages[idx] = p
	}
	return p
}

// Write loads bytes into memory at addr.
func (m *Mem) Write(addr uint64, data []byte) {
	for i, b := range data {
		m.Page(addr + uint64(i))[(addr+uint64(i))&(PageSize-1)] = b
	}
}

// ReadU8 reads a byte.
func (m *Mem) ReadU8(a uint64) uint8 {
	return m.Page(a)[a&(PageSize-1)]
}

// ReadU16 reads a little-endian 16-bit value (may be unaligned).
func (m *Mem) ReadU16(a uint64) uint16 {
	p := m.Page(a)
	o := a & (PageSize - 1)
	if o+2 <= PageSize {
		return binary.LittleEndian.Uint16(p[o:])
	}
	return uint16(p[o]) | uint16(m.Page(a + 1)[0])<<8
}

// ReadU32 reads a little-endian 32-bit value (may be unaligned).
func (m *Mem) ReadU32(a uint64) uint32 {
	var b [4]byte
	for i := 0; i < 4; i++ {
		b[i] = m.Page(a + uint64(i))[(a+uint64(i))&(PageSize-1)]
	}
	return binary.LittleEndian.Uint32(b[:])
}

// ReadU64 reads a little-endian 64-bit value (may be unaligned).
func (m *Mem) ReadU64(a uint64) uint64 {
	var b [8]byte
	for i := 0; i < 8; i++ {
		b[i] = m.Page(a + uint64(i))[(a+uint64(i))&(PageSize-1)]
	}
	return binary.LittleEndian.Uint64(b[:])
}

// ReadBytes reads n bytes into a new slice.
func (m *Mem) ReadBytes(a uint64, n int) []byte {
	dst := make([]byte, n)
	for i := range dst {
		dst[i] = m.Page(a + uint64(i))[(a+uint64(i))&(PageSize-1)]
	}
	return dst
}

func (m *Mem) WriteU8(a uint64, v uint8) {
	m.Page(a)[a&(PageSize-1)] = v
}

func (m *Mem) WriteU16(a uint64, v uint16) {
	p := m.Page(a)
	o := a & (PageSize - 1)
	if o+2 <= PageSize {
		binary.LittleEndian.PutUint16(p[o:], v)
		return
	}
	m.Page(a)[o] = byte(v)
	m.Page(a + 1)[0] = byte(v >> 8)
}

func (m *Mem) WriteU32(a uint64, v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	m.Write(a, b[:])
}

func (m *Mem) WriteU64(a uint64, v uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	m.Write(a, b[:])
}

// DropPageRange removes a byte range from the page table (used by munmap).
func (m *Mem) DropPageRange(start, size uint64) {
	if size == 0 {
		return
	}
	first := start >> 12
	last := (start + size - 1) >> 12
	for i := first; i <= last; i++ {
		delete(m.pages, i)
	}
}
