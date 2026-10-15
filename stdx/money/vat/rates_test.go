package vat

import (
	"testing"
)

func TestStandardRate(t *testing.T) {
	rate, ok := StandardRate("DE")
	if !ok {
		t.Error("should give rate for DE")
	}

	if rate != 19 {
		t.Error("wrong rate for DE")
	}

	_, ok = StandardRate("de")
	if !ok {
		t.Error("should give rate for DE")
	}

	_, ok = StandardRate("ZZ")
	if ok {
		t.Error("don't give rate for unknown countries")
	}
}

func TestStandardRateAtDate(t *testing.T) {
	rate, ok := StandardRateAtDate("EL", since(2017, 5, 15))
	if !ok {
		t.Error("should give rate for EL")
	}
	if rate != 24 {
		t.Errorf("wrong rate for EL since 2017-5-15: %v", rate)
	}

	rate, ok = StandardRateAtDate("EL", since(2016, 12, 30))
	if !ok {
		t.Error("should give rate for EL")
	}
	if rate != 23 {
		t.Errorf("wrong rate for EL since 2016-12-30: %v", rate)
	}

}

func TestIsEUCountry(t *testing.T) {
	var tests = []struct {
		countryCode string
		isEUCountry bool
	}{
		{"DE", true},
		{"AT", true},
		{"CH", false},
		{"XX", false},
	}

	for _, tt := range tests {
		if tt.isEUCountry != IsEUCountry(tt.countryCode) {
			t.Error("Country is not an eu country")
		}
	}
}
