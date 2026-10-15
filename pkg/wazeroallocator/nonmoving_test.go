package wazeroallocator

import (
	"testing"

	"github.com/tetratelabs/wazero/experimental"
	"markdown.ninja/stdx/testify/require"
)

func TestNonMoving(t *testing.T) {
	tests := []struct {
		name string
		mem  experimental.LinearMemory
		cap  uint64
	}{
		{
			name: "native",
			mem:  NewNonMoving().Allocate(10, 20),
			cap:  pageSize,
		},
		// The non-slice allocators are available on all normal platforms. Rather than requiring qemu to test slice
		// allocator, we just go ahead and test it in addition to the native one. On platforms other than unix/windows,
		// it will test the same allocator twice, which is fine.
		{
			name: "slice",
			mem:  alloc(10, 20),
			cap:  20,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			mem := tc.mem
			defer mem.Free()

			buf := mem.Reallocate(5)
			require.Len(t, buf, 5)
			require.EqualValues(t, tc.cap, cap(buf))
			base := &buf[0]

			buf = mem.Reallocate(5)
			require.Len(t, buf, 5)
			require.Equal(t, base, &buf[0])

			buf = mem.Reallocate(10)
			require.Len(t, buf, 10)
			require.Equal(t, base, &buf[0])

			buf = mem.Reallocate(20)
			require.Len(t, buf, 20)
			require.Equal(t, base, &buf[0])
		})
	}
}
