package semver

import (
	"testing"
)

func TestIsValid(t *testing.T) {
	tests := []struct {
		version        string
		expectedResult bool
	}{
		{"1.0.0", true},
		{"2.0.0", true},
		{"100.0.0", true},
		{"1.0efds.0", false},
		{"1", true},
	}

	for _, test := range tests {
		if IsValid(test.version) != test.expectedResult {
			t.Errorf("got (%v) for (%s). Expected result: %v", IsValid(test.version), test.version, test.expectedResult)
		}
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		v              string
		w              string
		expectedResult int
	}{
		{
			"1.0.0",
			"1.1.1",
			-1,
		},
		{
			"1.0.0",
			"1.0.0",
			0,
		},
		{
			"2.0.0",
			"1.0.0",
			1,
		},
	}

	for _, test := range tests {
		if Compare(test.v, test.w) != test.expectedResult {
			t.Errorf("got (%d) for (%s, %s). Expected result: %d", Compare(test.v, test.w), test.v, test.w, test.expectedResult)
		}
	}
}
