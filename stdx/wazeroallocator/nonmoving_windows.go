//go:build windows

package wazeroallocator

import (
	"fmt"
	"math"
	"unsafe"

	"github.com/tetratelabs/wazero/experimental"
	"golang.org/x/sys/windows"
)

var pageSize = uint64(windows.Getpagesize())

func alloc(_, max uint64) experimental.LinearMemory {
	// Round up to the page size because recommitting must be page-aligned.
	// In practice, the WebAssembly page size should be a multiple of the system
	// page size on most if not all platforms and rounding will never happen.
	rnd := pageSize - 1
	res := (max + rnd) &^ rnd

	if res > math.MaxInt {
		// This ensures uintptr(max) overflows to a large value,
		// and windows.VirtualAlloc returns an error.
		res = math.MaxUint64
	}

	// Reserve max bytes of address space, to ensure we won't need to move it.
	// This does not commit memory.
	r, err := windows.VirtualAlloc(0, uintptr(res), windows.MEM_RESERVE, windows.PAGE_READWRITE)
	if err != nil {
		panic(fmt.Errorf("allocator_windows: failed to reserve memory: %w", err))
	}

	buf := unsafe.Slice((*byte)(unsafe.Pointer(r)), int(res))
	return &virtualMemory{buf: buf[:0], addr: r}
}

// The slice covers the entire mmapped memory:
//   - len(buf) is the already committed memory,
//   - cap(buf) is the reserved address space, which is max rounded up to a page.
type virtualMemory struct {
	buf  []byte
	addr uintptr
}

func (m *virtualMemory) Reallocate(size uint64) []byte {
	com := uint64(len(m.buf))
	res := uint64(cap(m.buf))

	if com < size && size <= res {
		// Round up to the page size.
		rnd := pageSize - 1
		newCap := (size + rnd) &^ rnd

		// Commit additional memory up to new bytes.
		_, err := windows.VirtualAlloc(m.addr, uintptr(newCap), windows.MEM_COMMIT, windows.PAGE_READWRITE)
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

func (m *virtualMemory) Free() {
	err := windows.VirtualFree(m.addr, 0, windows.MEM_RELEASE)
	if err != nil {
		panic(fmt.Errorf("allocator_windows: failed to release memory: %w", err))
	}
	m.addr = 0
}
