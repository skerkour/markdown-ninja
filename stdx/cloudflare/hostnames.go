package cloudflare

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// CustomHostnameStatus is the enumeration of valid state values in the CustomHostnameSSL.
type CustomHostnameStatus string

const (
	// PENDING status represents state of CustomHostname is pending.
	PENDING CustomHostnameStatus = "pending"
	// ACTIVE status represents state of CustomHostname is active.
	ACTIVE CustomHostnameStatus = "active"
	// MOVED status represents state of CustomHostname is moved.
	MOVED CustomHostnameStatus = "moved"
	// DELETED status represents state of CustomHostname is removed.
	DELETED CustomHostnameStatus = "deleted"
)

// CustomHostname represents a custom hostname in a zone.
type CustomHostname struct {
	ID                        string                                  `json:"id,omitempty"`
	Hostname                  string                                  `json:"hostname,omitempty"`
	CustomOriginServer        string                                  `json:"custom_origin_server,omitempty"`
	CustomOriginSNI           string                                  `json:"custom_origin_sni,omitempty"`
	SSL                       *CustomHostnameSSL                      `json:"ssl,omitempty"`
	CustomMetadata            CustomMetadata                          `json:"custom_metadata,omitempty"`
	Status                    CustomHostnameStatus                    `json:"status,omitempty"`
	VerificationErrors        []string                                `json:"verification_errors,omitempty"`
	OwnershipVerification     CustomHostnameOwnershipVerification     `json:"ownership_verification,omitempty"`
	OwnershipVerificationHTTP CustomHostnameOwnershipVerificationHTTP `json:"ownership_verification_http,omitempty"`
	CreatedAt                 *time.Time                              `json:"created_at,omitempty"`
}

// CustomHostnameSSL represents the SSL section in a given custom hostname.
type CustomHostnameSSL struct {
	ID                   string                          `json:"id,omitempty"`
	Status               string                          `json:"status,omitempty"`
	Method               string                          `json:"method,omitempty"`
	Type                 string                          `json:"type,omitempty"`
	Wildcard             *bool                           `json:"wildcard,omitempty"`
	CustomCertificate    string                          `json:"custom_certificate,omitempty"`
	CustomKey            string                          `json:"custom_key,omitempty"`
	CertificateAuthority string                          `json:"certificate_authority,omitempty"`
	Issuer               string                          `json:"issuer,omitempty"`
	SerialNumber         string                          `json:"serial_number,omitempty"`
	Settings             CustomHostnameSSLSettings       `json:"settings,omitempty"`
	Certificates         []CustomHostnameSSLCertificates `json:"certificates,omitempty"`
	// Deprecated: use ValidationRecords.
	// If there a single validation record, this will equal ValidationRecords[0] for backwards compatibility.
	SSLValidationRecord
	ValidationRecords []SSLValidationRecord `json:"validation_records,omitempty"`
	ValidationErrors  []SSLValidationError  `json:"validation_errors,omitempty"`
}

// CustomHostnameSSLSettings represents the SSL settings for a custom hostname.
type CustomHostnameSSLSettings struct {
	HTTP2         string   `json:"http2,omitempty"`
	HTTP3         string   `json:"http3,omitempty"`
	TLS13         string   `json:"tls_1_3,omitempty"`
	MinTLSVersion string   `json:"min_tls_version,omitempty"`
	Ciphers       []string `json:"ciphers,omitempty"`
	EarlyHints    string   `json:"early_hints,omitempty"`
}

// CustomHostnameOwnershipVerification represents ownership verification status of a given custom hostname.
type CustomHostnameOwnershipVerification struct {
	Type  string `json:"type,omitempty"`
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

// CustomHostnameSSLCertificates represent certificate properties like issuer, expires date and etc.
type CustomHostnameSSLCertificates struct {
	Issuer            string     `json:"issuer"`
	SerialNumber      string     `json:"serial_number"`
	Signature         string     `json:"signature"`
	ExpiresOn         *time.Time `json:"expires_on"`
	IssuedOn          *time.Time `json:"issued_on"`
	FingerprintSha256 string     `json:"fingerprint_sha256"`
	ID                string     `json:"id"`
}

// CustomHostnameOwnershipVerificationHTTP represents a response from the Custom Hostnames endpoints.
type CustomHostnameOwnershipVerificationHTTP struct {
	HTTPUrl  string `json:"http_url,omitempty"`
	HTTPBody string `json:"http_body,omitempty"`
}

// CustomMetadata defines custom metadata for the hostname. This requires logic to be implemented by Cloudflare to act on the data provided.
type CustomMetadata map[string]any

// SSLValidationError represents errors that occurred during SSL validation.
type SSLValidationError struct {
	Message string `json:"message,omitempty"`
}

// SSLValidationRecord displays Domain Control Validation tokens.
type SSLValidationRecord struct {
	CnameTarget string `json:"cname_target,omitempty"`
	CnameName   string `json:"cname,omitempty"`

	TxtName  string `json:"txt_name,omitempty"`
	TxtValue string `json:"txt_value,omitempty"`

	HTTPUrl  string `json:"http_url,omitempty"`
	HTTPBody string `json:"http_body,omitempty"`

	Emails []string `json:"emails,omitempty"`
}

// https://developers.cloudflare.com/api/operations/custom-hostname-for-a-zone-create-custom-hostname
func (client *Client) CreateCustomHostname(ctx context.Context, zone, hostname string) (hostnameID string, err error) {
	var res CustomHostname
	input := CustomHostname{
		Hostname: hostname,
		SSL: &CustomHostnameSSL{
			Method: "http",
			Type:   "dv",
			Settings: CustomHostnameSSLSettings{
				HTTP2:         "on",
				HTTP3:         "on",
				MinTLSVersion: "1.2",
				TLS13:         "on",
			},
		},
	}

	err = client.request(ctx, requestParams{
		Payload: input,
		Method:  http.MethodPost,
		URL:     fmt.Sprintf("/client/v4/zones/%s/custom_hostnames", zone),
	}, &res)
	if err != nil {
		return
	}

	hostnameID = res.ID

	return
}

// https://developers.cloudflare.com/api/operations/custom-hostname-for-a-zone-delete-custom-hostname-(-and-any-issued-ssl-certificates)
func (client *Client) DeleteCustomHostname(ctx context.Context, zone, hostnameID string) (err error) {
	err = client.request(ctx, requestParams{
		Method: http.MethodDelete,
		URL:    fmt.Sprintf("/client/v4/zones/%s/custom_hostnames/%s", zone, hostnameID),
	}, nil)

	return
}

// https://developers.cloudflare.com/api/operations/custom-hostname-for-a-zone-custom-hostname-details
func (client *Client) GetCustomHostnameDetails(ctx context.Context, zone, hostnameID string) (res CustomHostname, err error) {
	err = client.request(ctx, requestParams{
		Method: http.MethodGet,
		URL:    fmt.Sprintf("/client/v4/zones/%s/custom_hostnames/%s", zone, hostnameID),
	}, &res)
	return
}
