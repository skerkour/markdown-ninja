package faststrings

import "testing"

func TestIsAscii(t *testing.T) {
	tests := []struct {
		str     string
		isAscii bool
	}{
		{"", true},
		{"hello", true},
		{"hellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohello", true},
		{"héllollollollollollollollollollollollollollollollollollollollollollollollollollollollollollollollollollollo", false},
		{"hllollollollollollollollollollollollollollollollollollollollollollollollollollollollollollollollollollollö", false},
		{"hllollollollollollollollollollollollollollollollollolölollollollollollollollollollollollollollollollolloll", false},
	}

	for _, test := range tests {
		if IsAscii(test.str) != test.isAscii {
			t.Fatalf("got: %v | expected: %v | string: %s", IsAscii(test.str), test.isAscii, test.str)
		}
	}
}
