package set

import (
	"testing"
)

func TestEqual(t *testing.T) {
	tests := []struct {
		s1       Set[int]
		s2       Set[int]
		expected bool
	}{
		{
			s1:       NewFromSlice([]int{1, 2, 3}),
			s2:       NewFromSlice([]int{1, 2, 3}),
			expected: true,
		},
		{
			s1:       New[int](),
			s2:       New[int](),
			expected: true,
		},
		{
			s1:       NewFromSlice([]int{1, 2, 3}),
			s2:       NewFromSlice([]int{1, 2}),
			expected: false,
		},
	}

	for _, test := range tests {
		result := test.s1.Equal(test.s2)
		if result != test.expected {
			t.Errorf("Invalid result for S1: %v / S2: %v. Expected: %v | Got %v", test.s1, test.s2, test.expected, result)
		}
	}
}
