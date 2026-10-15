package kernel

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"markdown.ninja/stdx/uuid"
)

const (
	AuthHttpHeader = "Authorization"

	EmailMaxLength            = 80
	TotpQrCodeSize            = 256
	TotpIssuer                = "Markdown Ninja"
	TotpQrCodeJPEGQuality int = 90

	MaxAssetSize = 200_000_000 // 200MB
)

type EmptyInput struct{}

type PaginatedResult[T any] struct {
	Data []T `json:"data"`
	// Total int64 `json:"total"`
}

////////////////////////////////////////////////////////////////////////////////////////////////////
// Entities
////////////////////////////////////////////////////////////////////////////////////////////////////

type Address struct {
	// Address line 1 (e.g., street, PO Box, or company name).
	Line1 string `json:"line1"`
	// Address line 2 (e.g., apartment, suite, unit, or building).
	Line2 string `json:"line2"`
	// ZIP or postal code.
	PostalCode string `json:"postal_code"`
	// City, district, suburb, town, or village.
	City string `json:"city"`
	// State, County, Region or Province
	State string `json:"state"`
	// Two-letter country code (ISO 3166-1 alpha-2).
	CountryCode string `json:"country_code"`
}

func (address *Address) Scan(val any) error {
	switch v := val.(type) {
	case []byte:
		json.Unmarshal(v, address)
		return nil
	case string:
		json.Unmarshal([]byte(v), address)
		return nil
	default:
		return fmt.Errorf("Address.Scan: Unsupported type: %T", v)
	}
}

func (address *Address) Value() (driver.Value, error) {
	return json.Marshal(address)
}

////////////////////////////////////////////////////////////////////////////////////////////////////
// Service
////////////////////////////////////////////////////////////////////////////////////////////////////

type InitData struct {
	StripePublicKey string `json:"stripe_public_key"`
	Country         string `json:"country"`
	ContactEamil    string `json:"contact_email"`
	Pricing         []Plan `json:"pricing"`
	// ChallengeSiteKey *string `json:"challenge_site_key"`
	Pingoo          InitDataPingoo `json:"pingoo"`
	WebsitesBaseUrl string         `json:"websites_base_url"`
}

type InitDataPingoo struct {
	AppID    string `json:"app_id"`
	Endpoint string `json:"endpoint"`
}

type DeleteBackgroundJobInput struct {
	JobID uuid.UUID `json:"id"`
}

type ChallengeSiteKey struct {
	SiteKey *string `json:"site_key"`
}
