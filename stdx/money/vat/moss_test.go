package vat

import (
	"testing"
)

func TestGetApplicableTax(t *testing.T) {
	var tests = []struct {
		VATnumber     string
		CountryCode   string
		TaxRate       float64
		ReverseCharge bool
		Err           error
	}{
		// private customer within EU
		{"", "DE", 19, false, nil},
		// {"", "UK", 20, false, nil},

		// business customer within EU
		{"IE6388047V", "IE", 0, true, nil},

		// customer not in EU
		{"", "CH", 0, false, nil},
		{"", "AR", 0, false, nil},
	}

	for _, tt := range tests {
		taxRate, reverseCharge, err := GetApplicableTax(tt.CountryCode, tt.VATnumber)
		if err != tt.Err {
			t.Fatalf("Expected err %v, got %v for %+v\n", tt.Err, err, tt)
		}
		if taxRate != tt.TaxRate {
			t.Errorf("Expected tax rate %v, got %v for %+v\n", tt.TaxRate, taxRate, tt)
		}
		if reverseCharge != tt.ReverseCharge {
			t.Errorf("Expected reverse charge %v, got %v for %+v\n", tt.ReverseCharge, reverseCharge, tt)
		}
	}

}
