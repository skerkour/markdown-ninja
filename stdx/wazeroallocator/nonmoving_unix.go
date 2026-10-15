//go:build unix

package wazeroallocator

import (
	"fmt"
	"math"

	"github.com/tetratelabs/wazero/experimental"
	"golang.org/x/sys/unix"
)

var pageSize = uint64(unix.Getpagesize())

func alloc(_, max uint64) experimental.LinearMemory {
	// Round up to the page size because recommitting must be page-aligned.
	// In practice, the WebAssembly page size should be a multiple of the system
	// page size on most if not all platforms and rounding will never happen.
	rnd := pageSize - 1
	res := (max + rnd) &^ rnd

	if res > math.MaxInt {
		// This ensures int(max) overflows to a negative value,
		// and unix.Mmap returns EINVAL.
		res = math.MaxUint64
	}

	// Reserve max bytes of address space, to ensure we won't need to move it.
	// A protected, private, anonymous mapping should not commit memory.
	b, err := unix.Mmap(-1, 0, int(res), unix.PROT_NONE, unix.MAP_PRIVATE|unix.MAP_ANON)
	if err != nil {
		panic(fmt.Errorf("allocator_unix: failed to reserve memory: %w", err))
	}
	return &mmappedMemory{buf: b[:0]}
}

// The slice covers the entire mmapped memory:
//   - len(buf) is the already committed memory,
//   - cap(buf) is the reserved address space, which is max rounded up to a page.
type mmappedMemory struct {
	buf []byte
}

func (m *mmappedMemory) Reallocate(size uint64) []byte {
	com := uint64(len(m.buf))
	res := uint64(cap(m.buf))

	if com < size && size <= res {
		// Round up to the page size.
		rnd := pageSize - 1
		newCap := (size + rnd) &^ rnd

		// Commit additional memory up to new bytes.
		err := unix.Mprotect(m.buf[com:newCap], unix.PROT_READ|unix.PROT_WRITE)
		if err != nil {
			return nil
		}

		// Update committed memory.
		m.buf = m.buf[:newCap]
	}
	// Limit returned capacity because bytes beyond
	// len(m.buf) have not yet been committed.
	return m.buf[:size:len(m.buf)]
}

func (m *mmappedMemory) Free() {
	err := unix.Munmap(m.buf[:cap(m.buf)])
	if err != nil {
		panic(fmt.Errorf("allocator_unix: failed to release memory: %w", err))
	}
	m.buf = nil
}
