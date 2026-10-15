package vat

import (
	"strings"
	"time"
)

var euCountries = []string{
	"BE", "BG", "CZ", "DK", "DE", "EE", "EL", "ES", "FR", "HR", "IE",
	"IT", "CY", "LV", "LT", "LU", "HU", "MT", "NL", "AT", "PL", "PT",
	"RO", "SI", "SK", "FI", "SE", //  "UK",
}

// reference:
// http://ec.europa.eu/taxation_customs/resources/documents/taxation/vat/how_vat_works/rates/vat_rates_en.pdf
// last updated: Jan 1st, 2017

type Rates struct {
	Since     time.Time
	Countries map[string]float64
}

var standardRates = []Rates{
	{
		Since: since(2015, 3, 6),
		Countries: map[string]float64{
			"BE": 21,
			"BG": 20,
			"CZ": 21,
			"DK": 25,
			"DE": 19,
			"EE": 20,
			"EL": 23,
			"ES": 21,
			"FR": 20,
			"HR": 25,
			"IE": 23,
			"IT": 22,
			"CY": 19,
			"LV": 21,
			"LT": 21,
			"LU": 17,
			"HU": 27,
			"MT": 18,
			"NL": 21,
			"AT": 20,
			"PL": 23,
			"PT": 23,
			"RO": 24,
			"SI": 22,
			"SK": 20,
			"FI": 24,
			"SE": 25,
			// "UK": 20,
		},
	},
	{
		Since: since(2017, 1, 1),
		Countries: map[string]float64{
			"EL": 24,
			"RO": 19,
		},
	},
	{
		Since: since(2020, 7, 1),
		Countries: map[string]float64{
			"DE": 16,
		},
	},
	{
		Since: since(2021, 1, 1),
		Countries: map[string]float64{
			"DE": 19,
		},
	},

	// add new rates at bottom ...
}

// StandardRate returns VAT rate in EU country at time.Now()
func StandardRate(countryCode string) (rate float64, ok bool) {
	return StandardRateAtDate(countryCode, time.Now())
}

// StandardRateAtDate returns VAT rate in EU country at given date
func StandardRateAtDate(countryCode string, date time.Time) (rate float64, ok bool) {
	return find(standardRates, strings.ToUpper(countryCode), date)
}

// find finds rate in given []Rate slice
func find(rates []Rates, countryCode string, date time.Time) (rate float64, ok bool) {
	// TODO: order by Since field
	for i := len(rates) - 1; i >= 0; i-- {
		if date.After(rates[i].Since) {
			if rate, ok := rates[i].Countries[countryCode]; ok {
				return rate, true
			}
		}
	}
	return 0, false
}

// Countries returns a list of all EU countries
func Countries() []string {
	return euCountries
}

// IsEUCountry returns true if countryCode is EU country
func IsEUCountry(countryCode string) bool {
	for _, c := range euCountries {
		if c == strings.ToUpper(countryCode) {
			return true
		}
	}
	return false
}

// since is a helper returning time.Time for Year, Month, Day
func since(year, month, day int) time.Time {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}
