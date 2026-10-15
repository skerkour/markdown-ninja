package vat

import (
	"time"
)

// GetApplicableTaxAtDate returns taxRate and if reverseCharge is applicable
// You should check the vatNumber with IsValidVAT(vatNumber) before
// passing a vatNumber to this function.
func GetApplicableTaxAtDate(countryCode, vatNumber string, date time.Time) (taxRate float64, reverseCharge bool, err error) {
	if vatNumber != "" {
		// eu business
		return 0, true, nil
	} else {
		// person from EU or rest of world?
		rate, ok := StandardRateAtDate(countryCode, date)
		if ok {
			// person from eu
			return rate, false, nil
		} else {
			// person from rest of world
			return 0, false, nil
		}
	}
}

// GetApplicableTax is a convenience func for GetApplicableTaxAtDate(...)
func GetApplicableTax(countryCode, vatNumber string) (taxRate float64, reverseCharge bool, err error) {
	return GetApplicableTaxAtDate(countryCode, vatNumber, time.Now())
}
