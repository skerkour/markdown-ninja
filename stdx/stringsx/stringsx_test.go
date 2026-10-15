package stringsx

import "testing"

func TestToAscii(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"héllo", "hello"},
		{"hello", "hello"},
		{"", ""},
		{"À@ö", "A@o"},
	}

	for _, test := range tests {
		got, err := ToAscii(test.input)
		if err != nil {
			t.Fatal(err)
		}
		if got != test.expected {
			t.Errorf("got: %s | expected: %s", got, test.expected)
		}
	}
}

func TestIsLower(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"héllo", true},
		{"hello", true},
		{"", true},
		{"Hello", false},
	}

	for _, test := range tests {
		got := IsLower(test.input)
		if got != test.expected {
			t.Errorf("got: %v | expected: %v", got, test.expected)
		}
	}
}

func TestIsUpper(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"héllo", false},
		{"hello", false},
		{"", true},
		{"Hello", true},
	}

	for _, test := range tests {
		got := IsUpper(test.input)
		if got != test.expected {
			t.Errorf("got: %v | expected: %v", got, test.expected)
		}
	}
}
