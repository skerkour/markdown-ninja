package faststrings

import "unsafe"

const ascii_mask = 0x8080808080808080

// from https://blog.reyem.dev/post/simd_within_a_register_in_go_1_20/
func IsAscii(s string) bool {
	length := len(s)
	i := 0
	if length >= 8 {
		bytes := unsafe.StringData(s)
		for ; i < length-7; i += 8 {
			if (ascii_mask & getBytesUint64(bytes, i)) > 0 {
				return false
			}
		}
	}

	// check the last characters of the string
	for ; i < length; i++ {
		if 0x80&s[i] > 0 {
			return false
		}
	}
	return true
}

// return 8 bytes at offset as an uint64
func getBytesUint64(bytes *byte, offset int) uint64 {
	// add offset to the pointer and convert bytes to an int64
	data := *(*uint64)(unsafe.Add(unsafe.Pointer(bytes), offset))
	return data
}
