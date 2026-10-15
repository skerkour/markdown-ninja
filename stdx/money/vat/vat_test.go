package vat

import (
	"context"
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	r, err := CheckVAT(context.Background(), "IE6388047V")
	if err != nil {
		t.Fatal(err)
	}

	if r.Valid == false {
		t.Error("Is actually valid vat number")
	}
	if r.CountryCode != "IE" {
		t.Error("Wrong country code")
	}
	if r.VATnumber != "6388047V" {
		t.Error("Wrong vat number")
	}
	if r.Name != "GOOGLE IRELAND LIMITED" {
		t.Error("Wrong name")
	}
	if r.Address != "3RD FLOOR, GORDON HOUSE, BARROW STREET, DUBLIN 4" {
		t.Errorf("Wrong address: %s", r.Address)
	}
}

func TestIsValidVAT(t *testing.T) {
	var tests = []struct {
		vatNumber string
		isValid   bool
	}{
		{"IE6388047V", true},
		{"", false},
		{"I", false},
		{"IE", false},
		{"IE1", false},
		{"LU 26375245", true}, // Amazon Europe Core Sarl
	}

	for _, tt := range tests {
		v, err := IsValidVAT(context.Background(), tt.vatNumber)
		if err == nil {
			if tt.isValid != v {
				t.Errorf("Expected %v for %v, got %v", tt.isValid, tt.vatNumber, v)
			}
		}
	}
}

func TestGetEnvelope(t *testing.T) {
	e, err := getEnvelope("IE6388047V") // Google Ireland
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e, "IE") {
		t.Error("Envelope is missing countryCode")
	}
	if !strings.Contains(e, "6388047V") {
		t.Error("Envelope is missing vatNumber")
	}

	if _, err := getEnvelope(""); err == nil {
		t.Error("Expected error for invalid vatNumber")
	}
	if _, err := getEnvelope("IE"); err == nil {
		t.Error("Expected error for invalid vatNumber")
	}
	if _, err := getEnvelope("IE1"); err != nil {
		t.Error("This should run the check, although it probably doesn't make sense")
	}
}

func TestSanitizeVatNumber(t *testing.T) {
	var tests = []struct {
		in  string
		out string
	}{
		{"IE 123", "IE123"},
		{" IE 123 ", "IE123"},
	}

	for _, tt := range tests {
		if tt.out != sanitizeVatNumber(tt.in) {
			t.Error("sanitize failed for", tt.in)
		}
	}
}
